package main

// SNMP 设备告警。snmpStore 缓存每台主机（agent）下各被轮询设备的最新快照，
// 供 Notifier.tick() 每轮评估复用（避免每 10s 查 PG）。EvaluateSNMP 把接口 up/down、
// 带宽利用率、错误/丢包率、采集失败转成标准 Alert，走统一告警链路（去重/治理/SRE）。

import (
	"strconv"
	"sync"
	"time"

	"aiops-monitor/shared"
)

// SNMP 接口告警阈值（写死常量，绕开 ThresholdConfig，同 hardware/hyperv 的做法）。
const (
	snmpUtilWarn = 80.0 // 带宽利用率 %（越高越糟）
	snmpUtilCrit = 95.0
	snmpErrWarn  = 1.0 // 错误+丢包率 pps（越高越糟）
	snmpErrCrit  = 10.0
)

type snmpHostEntry struct {
	hostname  string
	ip        string
	updatedAt int64
	snaps     []shared.SNMPSnapshot
}

// snmpStore holds the most recent SNMP device snapshots per host (agent),
// plus a little transition state so a link-down only alerts for interfaces
// we have previously observed UP (avoids noise from enabled-but-idle ports).
type snmpStore struct {
	mu       sync.RWMutex
	byID     map[string]snmpHostEntry
	operSeen map[string]bool // hostID|device|ifIndex → 曾观测到 oper-up
}

func newSNMPStore() *snmpStore {
	return &snmpStore{byID: map[string]snmpHostEntry{}, operSeen: map[string]bool{}}
}

// put replaces a host's SNMP snapshots with the newest report.
func (ss *snmpStore) put(hostID, hostname, ip string, snaps []shared.SNMPSnapshot) {
	if ss == nil || hostID == "" {
		return
	}
	cp := make([]shared.SNMPSnapshot, len(snaps))
	copy(cp, snaps)
	ss.mu.Lock()
	ss.byID[hostID] = snmpHostEntry{hostname: hostname, ip: ip, updatedAt: time.Now().Unix(), snaps: cp}
	ss.mu.Unlock()
}

// snapsOf returns the latest SNMP snapshots for one host (nil when none).
func (ss *snmpStore) snapsOf(hostID string) []shared.SNMPSnapshot {
	if ss == nil {
		return nil
	}
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	e, ok := ss.byID[hostID]
	if !ok {
		return nil
	}
	out := make([]shared.SNMPSnapshot, len(e.snaps))
	copy(out, e.snaps)
	return out
}

// snapshot returns a copy of every host's latest SNMP entry.
func (ss *snmpStore) snapshot() map[string]snmpHostEntry {
	if ss == nil {
		return nil
	}
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	out := make(map[string]snmpHostEntry, len(ss.byID))
	for k, v := range ss.byID {
		out[k] = v
	}
	return out
}

// operDownShouldAlert records an interface's oper state and reports whether a
// DOWN should raise an alert (only if the interface was previously seen UP).
func (ss *snmpStore) operDownShouldAlert(key string, up bool) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if up {
		ss.operSeen[key] = true
		return false
	}
	return ss.operSeen[key]
}

// EvaluateSNMP turns the latest SNMP snapshots into threshold alerts so they
// flow through the normal notifier path (dedup + fire/resolve + push + 治理 + SRE)
// exactly like hardware/hyperv alerts.
//
// Scope 每子项唯一（device/iface/metric）——alertKey = HostID/Type/Scope，
// 同 scope 会让兄弟接口互相覆盖。
func EvaluateSNMP(ss *snmpStore, th Thresholds) []Alert {
	if ss == nil {
		return nil
	}
	// 可配阈值优先；未配置(0)回退到内置常量，兼容旧配置与预设档。
	utilWarn, utilCrit := th.SNMPIfUtilWarn, th.SNMPIfUtilCrit
	if utilWarn <= 0 {
		utilWarn = snmpUtilWarn
	}
	if utilCrit <= 0 {
		utilCrit = snmpUtilCrit
	}
	errWarn, errCrit := th.SNMPIfErrWarn, th.SNMPIfErrCrit
	if errWarn <= 0 {
		errWarn = snmpErrWarn
	}
	if errCrit <= 0 {
		errCrit = snmpErrCrit
	}
	var alerts []Alert
	now := time.Now().Unix()

	for hostID, e := range ss.snapshot() {
		for _, snap := range e.snaps {
			device := snap.TargetName
			add := func(level, scope, msg string, val float64) {
				alerts = append(alerts, Alert{
					HostID: hostID, Hostname: e.hostname, IP: e.ip,
					Level: level, Type: "snmp", Scope: scope,
					Message: msg, Value: val, Timestamp: now,
				})
			}

			// 采集失败：报一条 warning，不拿零值误判各接口 down。
			if snap.Error != "" {
				add("warning", device+"/poll", Tz("alert.snmp_poll_fail", device, snap.Error), 0)
				continue
			}

			for _, iface := range snap.Interfaces {
				ifKey := hostID + "|" + device + "|" + strconv.Itoa(int(iface.Index))

				// 接口链路 DOWN：admin-up 但 oper-down，且此前见过 up。
				if iface.AdminStatus == 1 && !iface.OperUp {
					if ss.operDownShouldAlert(ifKey, false) {
						add("critical", device+"/"+iface.Name+"/status",
							Tz("alert.snmp_if_down", device, iface.Name), float64(iface.OperStatus))
					}
					continue // down 时利用率/速率无意义
				}
				if iface.OperUp {
					ss.operDownShouldAlert(ifKey, true) // 标记见过 up
				}
				if !iface.RateValid {
					continue // 首轮/复位时速率不可信，不评估
				}

				// 带宽利用率（取进/出较大者）。
				util := iface.InUtilPercent
				if iface.OutUtilPercent > util {
					util = iface.OutUtilPercent
				}
				if lv := classify(util, utilWarn, utilCrit); lv != "" {
					add(lv, device+"/"+iface.Name+"/util",
						Tz("alert.snmp_if_util", device, iface.Name, util), util)
				}

				// 错误率 + 丢包率（合计）。
				errPps := iface.InErrPps + iface.OutErrPps + iface.InDiscardPps + iface.OutDiscardPps
				if lv := classify(errPps, errWarn, errCrit); lv != "" {
					add(lv, device+"/"+iface.Name+"/errors",
						Tz("alert.snmp_if_errors", device, iface.Name, errPps), errPps)
				}
			}
		}
	}
	return alerts
}
