package main

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"aiops-monitor/shared"
)

// ---------------------------------------------------------------------------
// 硬件资产变更追溯
//
// hardware_snapshot 只保留每台设备的**最新**一份快照，所以"这块盘是什么时候换的"
// 事后完全无从查起。另一个极端——每轮采集都存一份整快照——在 30~60s 的采集周期下
// 会让表迅速膨胀，而其中 99% 是逐字重复的数据。
//
// 折中：每轮把新快照和上一份做**部件级 diff**，只有真的增/删/换才写一行。
// 一台稳定的机器一年也写不了几条，但换过的每块盘、每条内存都留得下痕迹。
// ---------------------------------------------------------------------------

// hwPart is one comparable component: identified by slot, compared by identity.
type hwPart struct {
	kind      string // disk / dimm / psu / ...
	component string // 槽位（换件后不变），用来判断"同一个位置"
	identity  string // 序列号+型号（换件后会变），用来判断"是不是同一个零件"
}

// hwPartsOf flattens a snapshot into comparable parts.
//
// component 用**槽位**而不是序列号：换盘时槽位不变、序列号变 → 判定为 replaced；
// 若用序列号当 key，一次换盘会变成 "removed + added" 两条，读起来完全看不出是同一个位置的更换。
func hwPartsOf(snap shared.HardwareSnapshot) []hwPart {
	var out []hwPart
	add := func(kind, comp, ident string) {
		comp = strings.TrimSpace(comp)
		if comp == "" {
			return // 没有稳定标识就无法可靠比对，宁可不记也不要制造假变更
		}
		out = append(out, hwPart{kind: kind, component: comp, identity: strings.TrimSpace(ident)})
	}

	for _, d := range snap.Storage {
		slot := d.Location
		if slot == "" {
			slot = d.Name
		}
		add("disk", slot, joinNonEmpty(d.SerialNumber, d.Model, fmt.Sprintf("%.0fGB", d.CapacityGB)))
	}
	for _, d := range snap.Memory.DIMMs {
		slot := d.Slot
		if slot == "" {
			slot = d.Name
		}
		add("dimm", slot, joinNonEmpty(d.SerialNumber, d.PartNumber, fmt.Sprintf("%.0fGB", d.CapacityGB)))
	}
	for _, p := range snap.Power.PSUs {
		add("psu", p.Name, joinNonEmpty(p.SerialNumber, p.Model))
	}
	for _, c := range snap.CPUs {
		add("cpu", c.Name, joinNonEmpty(c.Model, fmt.Sprintf("%dC", c.Cores)))
	}
	for _, g := range snap.GPUs {
		add("gpu", g.Name, joinNonEmpty(g.Model, g.Manufacturer))
	}
	for _, r := range snap.RAID {
		add("raid", r.Name, joinNonEmpty(r.SerialNumber, r.Model, r.FirmwareVersion))
	}
	for _, e := range snap.Enclosures {
		slot := e.Location
		if slot == "" {
			slot = e.Name
		}
		add("enclosure", slot, joinNonEmpty(e.SerialNumber, e.Model))
	}
	// 固件版本变化 = 升级痕迹，是运维排障时很关键的一条时间线
	for _, f := range snap.Firmware {
		add("firmware", f.Name, f.Version)
	}
	return out
}

func joinNonEmpty(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" && p != "-" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " / ")
}

// hwChange is one detected difference.
type hwChange struct {
	Kind      string
	Component string
	Action    string // added / removed / replaced
	Old, New  string
}

// diffHardware compares two snapshots and returns only real component changes.
func diffHardware(prev, cur shared.HardwareSnapshot) []hwChange {
	oldParts := map[string]hwPart{}
	for _, p := range hwPartsOf(prev) {
		oldParts[p.kind+"|"+p.component] = p
	}
	newParts := map[string]hwPart{}
	for _, p := range hwPartsOf(cur) {
		newParts[p.kind+"|"+p.component] = p
	}

	var out []hwChange
	for k, np := range newParts {
		op, existed := oldParts[k]
		switch {
		case !existed:
			out = append(out, hwChange{np.kind, np.component, "added", "", np.identity})
		case op.identity != np.identity:
			// 同一个槽位，零件身份变了 = 换件（或固件升级）
			action := "replaced"
			if np.kind == "firmware" {
				action = "changed"
			}
			out = append(out, hwChange{np.kind, np.component, action, op.identity, np.identity})
		}
	}
	for k, op := range oldParts {
		if _, still := newParts[k]; !still {
			out = append(out, hwChange{op.kind, op.component, "removed", op.identity, ""})
		}
	}
	// 输出稳定，便于测试与阅读
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Component < out[j].Component
	})
	return out
}

// recordHardwareChanges diffs against the previous snapshot and persists changes.
//
// 只在**已有**上一份快照时才比对：首次采集会把整机所有部件都算成 added，
// 那不是变更、是基线，记下来只会淹没真正的换件记录。
func (s *Server) recordHardwareChanges(hostID string, cur shared.HardwareSnapshot) {
	if s.pg == nil {
		return
	}
	prev, ok := s.pg.getHardwareSnapshotDecoded(hostID, cur.TargetName)
	if !ok {
		return // 首次入库 = 建立基线，不产生变更记录
	}
	changes := diffHardware(prev, cur)
	if len(changes) == 0 {
		return
	}
	for _, c := range changes {
		s.pg.insertHardwareChange(hostID, cur.TargetName, c)
	}
	slog.Info("检测到硬件资产变更", "host_id", hostID, "target", cur.TargetName, "changes", len(changes))
}
