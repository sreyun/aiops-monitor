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
	hostname   string
	ip         string
	updatedAt  int64
	guests     []shared.HyperVGuest
	totalMemMB float64 // 物理宿主机总内存(MB)，采集成功时更新，用于「可用/总内存」显示
	availMemMB float64 // 物理宿主机可用内存(MB)
	lastError  string  // 最近一次采集失败原因（非空时 guests 为上一份好数据）
	// 关机告警只在 Running→非运行的**跳变**时触发（并保持到恢复），避免为"故意长期
	// 关机的模板机/备用机"刷屏。stateByVM 记录上一份各 VM 状态，alarmVM 记录"当前应
	// 就其非运行状态告警"的 VM（sticky：直到该 VM 回到 Running 或消失才清除）。
	stateByVM map[string]string
	alarmVM   map[string]bool
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
func (hs *hypervStore) put(hostID, hostname, ip string, guests []shared.HyperVGuest, errMsg string, totalMemMB, availMemMB float64) {
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
		// 宿主机内存随成功采集更新；采集失败时保留上一份（同 guests 的保留策略）。
		if totalMemMB > 0 {
			e.totalMemMB, e.availMemMB = totalMemMB, availMemMB
		}
		// 计算跳变告警集：VM 非运行(关机/暂停/保存)且【上一份是 Running】=崩溃式停机→告警；
		// 已在告警且仍未回到 Running 则保持(sticky)。回到 Running 或消失则自然清除(不在新集里)。
		newState := make(map[string]string, len(guests))
		newAlarm := make(map[string]bool)
		for _, g := range guests {
			k := hypervKey(g)
			newState[k] = g.State
			if g.State == "Off" || g.State == "Paused" || g.State == "Saved" {
				if e.stateByVM[k] == "Running" || e.alarmVM[k] {
					newAlarm[k] = true
				}
			}
		}
		e.stateByVM = newState
		e.alarmVM = newAlarm
	}
	hs.byID[hostID] = e
}

// hostMemOf returns the host's total/available RAM (MB) from the latest report,
// (0,0) when unknown. Used to annotate the inventory list with "可用/总内存".
func (hs *hypervStore) hostMemOf(hostID string) (total, avail float64) {
	if hs == nil {
		return 0, 0
	}
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	if e, ok := hs.byID[hostID]; ok {
		return e.totalMemMB, e.availMemMB
	}
	return 0, 0
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
			// Scope 用稳定身份（GUID 优先），改名不会拆成新旧两条告警互相并存。
			scopeBase := hypervAlertScope(g)

			// 健康严重（RunningCritical/存储掉线/复制 Critical）——最高优先，命中即不再评其余项。
			if g.Health == "Critical" {
				add("critical", scopeBase, Tz("alert.hyperv_health", name, g.State), 0)
				continue
			}

			// 状态：非运行（关机/暂停/保存）→ warning，但**仅在由 Running 跳变而来**时报
			// （见 put() 里的 alarmVM）。故意长期关机的模板机/备用机不会刷屏。notifier 去重：
			// 起止各推一次，VM 恢复 Running 后 alarmVM 清除、告警自动 resolve。
			switch g.State {
			case "Off", "Paused", "Saved":
				if e.alarmVM[hypervKey(g)] {
					add("warning", scopeBase+"/power", Tz("alert.hyperv_state", name, g.State), 0)
				}
			}

			// 复制健康 Warning
			if g.ReplHealth == "Warning" {
				add("warning", scopeBase+"/repl", Tz("alert.hyperv_repl", name, g.ReplHealth), 0)
			}

			// 资源阈值——只对运行中的 VM 评估（关机 VM 的 0 值无意义）。
			if g.State == "Running" {
				if lv := classify(g.CPUUsage, hypervCPUWarn, hypervCPUCrit); lv != "" {
					add(lv, scopeBase+"/cpu", Tz("alert.hyperv_cpu", name, g.CPUUsage), g.CPUUsage)
				}
				// 内存压力(需求/分配)只对**动态内存** VM 有意义：静态内存 VM 的 demand
				// 可能≈assigned，会误报接近 100%。非动态内存直接跳过。
				if g.DynamicMemEnabled && g.MemAssignedMB > 0 && g.MemDemandMB > 0 {
					memPct := g.MemDemandMB / g.MemAssignedMB * 100
					if lv := classify(memPct, hypervMemWarn, hypervMemCrit); lv != "" {
						add(lv, scopeBase+"/mem", Tz("alert.hyperv_mem", name, memPct), memPct)
					}
				}
			}
		}
	}
	return alerts
}
