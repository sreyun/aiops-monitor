package main

import (
	"sync"
	"time"

	"aiops-monitor/shared"
)

// ---------------------------------------------------------------------------
// Hyper-V 虚拟机告警
//
// 与硬件告警同构：最新一份 guest 清单缓存在内存（上报时写入），notifier 每轮
// 直接重评估，无需查 PG。判定来源以【非本地化的枚举】为准（State / Replication
// Health），资源类（CPU/内存）用写死的内置默认阈值——本功能不进 ThresholdConfig
// 配置管道（见 [[storage-pg-vm-mandatory]] 里硬件告警绕开阈值管道的同款理由）。
// ---------------------------------------------------------------------------

// 内置默认资源阈值（不暴露到设置页）。仅对 Running 的 VM 评估。
const (
	hypervCPUWarn = 85.0 // 宿主视角 CPU 占用 %
	hypervCPUCrit = 95.0
	hypervMemWarn = 90.0 // 内存需求/分配 百分比（动态内存压力）
	hypervMemCrit = 97.0
)

type hvHostEntry struct {
	hostname  string
	ip        string
	updatedAt int64
	guests    []shared.HyperVGuest
	lastError string // 最近一次采集失败原因（非空时 guests 为上一份好数据）
}

// hypervStore holds the most recent Hyper-V guest inventory per physical host.
type hypervStore struct {
	mu   sync.RWMutex
	byID map[string]hvHostEntry
}

func newHypervStore() *hypervStore {
	return &hypervStore{byID: map[string]hvHostEntry{}}
}

// put records the newest inventory for a host. On a collection error the previous
// guest list is PRESERVED (only lastError is updated) so a transient Get-VM
// failure never wipes good data or fabricates "all VMs removed" changes.
func (hs *hypervStore) put(hostID, hostname, ip string, guests []shared.HyperVGuest, errMsg string) {
	if hs == nil || hostID == "" {
		return
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	e := hs.byID[hostID]
	e.hostname, e.ip, e.updatedAt, e.lastError = hostname, ip, time.Now().Unix(), errMsg
	if errMsg == "" {
		cp := make([]shared.HyperVGuest, len(guests))
		copy(cp, guests)
		e.guests = cp
	}
	hs.byID[hostID] = e
}

// guestsOf returns the latest guests for one host (nil when none).
func (hs *hypervStore) guestsOf(hostID string) []shared.HyperVGuest {
	if hs == nil {
		return nil
	}
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	e, ok := hs.byID[hostID]
	if !ok {
		return nil
	}
	out := make([]shared.HyperVGuest, len(e.guests))
	copy(out, e.guests)
	return out
}

// snapshot returns a copy of every host's latest entry.
func (hs *hypervStore) snapshot() map[string]hvHostEntry {
	if hs == nil {
		return nil
	}
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	out := make(map[string]hvHostEntry, len(hs.byID))
	for k, v := range hs.byID {
		out[k] = v
	}
	return out
}

// remove drops a host's inventory from the in-memory store.
func (hs *hypervStore) remove(hostID string) {
	if hs == nil {
		return
	}
	hs.mu.Lock()
	delete(hs.byID, hostID)
	hs.mu.Unlock()
}

// EvaluateHyperV turns the latest guest inventories into alerts that flow through
// the normal notifier path (dedup + fire/resolve + push) exactly like CPU/disk/
// hardware alerts. Type is "hyperv"; Scope is unique per VM+aspect so sibling VMs
// and sibling metrics never overwrite each other (alertKey = HostID/Type/Scope).
func EvaluateHyperV(hs *hypervStore) []Alert {
	entries := hs.snapshot()
	if len(entries) == 0 {
		return nil
	}
	now := time.Now().Unix()
	var alerts []Alert

	for hostID, e := range entries {
		add := func(level, scope, msg string, val float64) {
			alerts = append(alerts, Alert{
				HostID: hostID, Hostname: e.hostname, IP: e.ip,
				Level: level, Type: "hyperv", Scope: scope,
				Message: msg, Value: val, Timestamp: now,
			})
		}

		// 采集失败：只报一条，不拿(可能过期的) guests 字段误判。
		if e.lastError != "" {
			add("warning", "collect", Tz("alert.hyperv_collect_failed", e.lastError), 0)
			continue
		}

		for _, g := range e.guests {
			name := g.Name
			if name == "" {
				continue
			}

			// 健康严重（RunningCritical/存储掉线/复制 Critical）——最高优先，命中即不再评其余项。
			if g.Health == "Critical" {
				add("critical", name, Tz("alert.hyperv_health", name, g.State), 0)
				continue
			}

			// 状态：非运行（关机/暂停/保存）→ warning。notifier 去重：起止各推一次，
			// 长期关机 = 一条 active 告警（可 acknowledge），不刷屏。
			switch g.State {
			case "Off", "Paused", "Saved":
				add("warning", name+"/power", Tz("alert.hyperv_state", name, g.State), 0)
			}

			// 复制健康 Warning
			if g.ReplHealth == "Warning" {
				add("warning", name+"/repl", Tz("alert.hyperv_repl", name, g.ReplHealth), 0)
			}

			// 资源阈值——只对运行中的 VM 评估（关机 VM 的 0 值无意义）。
			if g.State == "Running" {
				if lv := classify(g.CPUUsage, hypervCPUWarn, hypervCPUCrit); lv != "" {
					add(lv, name+"/cpu", Tz("alert.hyperv_cpu", name, g.CPUUsage), g.CPUUsage)
				}
				if g.MemAssignedMB > 0 && g.MemDemandMB > 0 {
					memPct := g.MemDemandMB / g.MemAssignedMB * 100
					if lv := classify(memPct, hypervMemWarn, hypervMemCrit); lv != "" {
						add(lv, name+"/mem", Tz("alert.hyperv_mem", name, memPct), memPct)
					}
				}
			}
		}
	}
	return alerts
}
