package main

// NetFlow 流量异常告警。现有 NetFlow 只做采集/汇总/展示，无告警。这里补一个轻量的
// 每主机 EWMA 基线突增检测 + 采集器丢包检测，接入统一告警链路（去重/治理/SRE）。
// 数据源是 handleAgentNetFlow 上报时喂进来的窗口统计，不查 PG（tick 每 10s，不能压库）。

import (
	"fmt"
	"sync"
	"time"

	"aiops-monitor/shared"
)

const (
	nfSurgeRatio      = 3.0       // 当前 bps 超基线倍数即视为突增
	nfSurgeMinBps     = 1_000_000 // 低于 1Mbps 的不算突增（避免小流量噪声）
	nfSurgeMinSamples = 5         // 需先积累基线样本
	nfDropWarn        = 100       // 单窗口采集丢包阈值
)

type nfHostStat struct {
	hostname    string
	ip          string
	updatedAt   int64
	curBps      float64
	baselineBps float64
	samples     int
	surge       bool
	dropped     uint64
}

// nfStore 缓存每主机 NetFlow 窗口统计与 EWMA 基线。
type nfStore struct {
	mu   sync.RWMutex
	byID map[string]*nfHostStat
}

func newNFStore() *nfStore { return &nfStore{byID: map[string]*nfHostStat{}} }

// put 用一份 NetFlow 上报更新该主机的基线与突增标志。
func (ns *nfStore) put(hostID, hostname, ip string, rep shared.NetFlowReport) {
	if ns == nil || hostID == "" {
		return
	}
	win := rep.WindowSec
	if win <= 0 {
		win = 1
	}
	bps := float64(rep.Stats.TotalBytes*8) / float64(win)
	ns.mu.Lock()
	st := ns.byID[hostID]
	if st == nil {
		st = &nfHostStat{}
		ns.byID[hostID] = st
	}
	st.hostname, st.ip = hostname, ip
	st.updatedAt = time.Now().Unix()
	st.curBps = bps
	st.dropped = rep.Stats.DroppedPackets
	// surge 用"并入本样本之前"的基线判定，避免突增被自身稀释。
	st.surge = st.samples >= nfSurgeMinSamples && st.baselineBps > 0 &&
		bps > nfSurgeRatio*st.baselineBps && bps > nfSurgeMinBps
	if st.samples == 0 {
		st.baselineBps = bps
	} else {
		st.baselineBps = 0.8*st.baselineBps + 0.2*bps
	}
	st.samples++
	ns.mu.Unlock()
}

func (ns *nfStore) snapshot() map[string]nfHostStat {
	if ns == nil {
		return nil
	}
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	out := make(map[string]nfHostStat, len(ns.byID))
	for k, v := range ns.byID {
		out[k] = *v
	}
	return out
}

// EvaluateNetFlow 把流量突增/采集丢包转成标准 Alert。
func EvaluateNetFlow(ns *nfStore) []Alert {
	if ns == nil {
		return nil
	}
	var alerts []Alert
	now := time.Now().Unix()
	for hostID, st := range ns.snapshot() {
		if st.surge {
			alerts = append(alerts, Alert{
				HostID: hostID, Hostname: st.hostname, IP: st.ip,
				Level: "warning", Type: "netflow", Scope: "traffic/surge",
				Message:   Tz("alert.netflow_surge", hostOrName(hostID, st.hostname), humanRate(st.curBps), humanRate(st.baselineBps)),
				Value:     st.curBps,
				Timestamp: now,
			})
		}
		if st.dropped >= nfDropWarn {
			alerts = append(alerts, Alert{
				HostID: hostID, Hostname: st.hostname, IP: st.ip,
				Level: "warning", Type: "netflow", Scope: "collector/drops",
				Message:   Tz("alert.netflow_drops", hostOrName(hostID, st.hostname), int(st.dropped)),
				Value:     float64(st.dropped),
				Timestamp: now,
			})
		}
	}
	return alerts
}

func hostOrName(hostID, hostname string) string {
	if hostname != "" {
		return hostname
	}
	return hostID
}

// humanRate 把 bit/s 格式化为可读速率（base-1000，网络习惯）。
func humanRate(bps float64) string {
	units := []string{"b", "Kb", "Mb", "Gb", "Tb"}
	i := 0
	for bps >= 1000 && i < len(units)-1 {
		bps /= 1000
		i++
	}
	return fmt.Sprintf("%.1f %s", bps, units[i])
}
