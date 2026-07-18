package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aiops-monitor/shared"
)

// ---------------------------------------------------------------------------
// Sreyun SNMP / 网络设备工具
//
// 让 AI 会话能直接看到 SNMP 轮询到的交换机/路由器接口状态、带宽、错误率，以及收到的
// Trap 事件——网络设备装不了 agent，这几个工具是 AI 诊断网络问题的唯一数据入口。
// 输出面向 LLM：纯文本、结论前置、异常摆最前，不返回原始 JSON。
// ---------------------------------------------------------------------------

// snmpResolve 把 host 引用映射到 (id, name, snapshots)。内存没有就回落 PG。
func (h *SreyunCore) snmpResolve(args map[string]any) (string, string, []shared.SNMPSnapshot, string) {
	ref, _ := args["host_id"].(string)
	if ref == "" {
		return "", "", nil, "请指定 host_id"
	}
	hostID, name := ref, ref
	if hst := h.resolveHostRef(ref); hst != nil {
		hostID, name = hst.ID, hst.Hostname
	}
	snaps := h.s.snmp.snapsOf(hostID)
	if len(snaps) == 0 && h.s.pg != nil {
		if rows, err := h.s.pg.getSNMPSnapshots(hostID); err == nil {
			for _, r := range rows {
				if snap, ok := snmpSnapFromRow(r); ok {
					snaps = append(snaps, snap)
				}
			}
		}
	}
	if len(snaps) == 0 {
		return hostID, name, nil, fmt.Sprintf(
			"主机 %s 没有 SNMP 数据。可能原因：未配置 SNMP 轮询目标，或 Agent 未上报。", name)
	}
	return hostID, name, snaps, ""
}

func snmpSnapFromRow(r map[string]any) (shared.SNMPSnapshot, bool) {
	raw, ok := r["snapshot"]
	if !ok {
		return shared.SNMPSnapshot{}, false
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return shared.SNMPSnapshot{}, false
	}
	var snap shared.SNMPSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return shared.SNMPSnapshot{}, false
	}
	if dn, ok := r["device_name"].(string); ok && snap.TargetName == "" {
		snap.TargetName = dn
	}
	return snap, true
}

// execQuerySNMPMetric 概览每台被轮询设备的接口健康，异常接口摆最前。
func (h *SreyunCore) execQuerySNMPMetric(args map[string]any) (string, error) {
	_, name, snaps, errMsg := h.snmpResolve(args)
	if errMsg != "" {
		return errMsg, nil
	}
	deviceFilter, _ := args["device"].(string)
	deviceFilter = strings.ToLower(strings.TrimSpace(deviceFilter))

	var b strings.Builder
	fmt.Fprintf(&b, "主机 %s 的 SNMP 网络设备（%d 台）:\n", name, len(snaps))
	for _, snap := range snaps {
		if deviceFilter != "" && !strings.Contains(strings.ToLower(snap.TargetName), deviceFilter) {
			continue
		}
		if snap.Error != "" {
			fmt.Fprintf(&b, "- %s（%s）采集失败: %s\n", snap.TargetName, snap.TargetIP, snap.Error)
			continue
		}
		up, down := 0, 0
		var bad []string
		for _, iface := range snap.Interfaces {
			if iface.OperUp {
				up++
			} else {
				down++
			}
			switch {
			case iface.AdminStatus == 1 && !iface.OperUp:
				bad = append(bad, fmt.Sprintf("    %s: 链路 DOWN", iface.Name))
			case iface.RateValid:
				util := iface.InUtilPercent
				if iface.OutUtilPercent > util {
					util = iface.OutUtilPercent
				}
				if util >= snmpUtilWarn {
					bad = append(bad, fmt.Sprintf("    %s: 利用率 %.0f%%（in %s/s out %s/s）",
						iface.Name, util, humanRate(iface.InBps), humanRate(iface.OutBps)))
				}
				if e := iface.InErrPps + iface.OutErrPps + iface.InDiscardPps + iface.OutDiscardPps; e > snmpErrWarn {
					bad = append(bad, fmt.Sprintf("    %s: 错误/丢包 %.1f pps", iface.Name, e))
				}
			}
		}
		fmt.Fprintf(&b, "- %s（%s）%s，接口 %d 个（up %d / down %d），运行 %s\n",
			snap.TargetName, snap.TargetIP, snap.System.Name, len(snap.Interfaces), up, down, humanUptime(snap.System.UptimeSec))
		if len(bad) > 0 {
			fmt.Fprintf(&b, "  异常接口 %d 个:\n%s\n", len(bad), strings.Join(bad, "\n"))
		} else {
			b.WriteString("  异常接口: 无\n")
		}
	}
	return b.String(), nil
}

// execQueryInterfaceTraffic 列出各设备接口按带宽 Top-N，回答"哪个口最忙/带宽被谁占"。
func (h *SreyunCore) execQueryInterfaceTraffic(args map[string]any) (string, error) {
	_, name, snaps, errMsg := h.snmpResolve(args)
	if errMsg != "" {
		return errMsg, nil
	}
	top := 10
	if n, ok := args["top"].(float64); ok && n > 0 {
		top = int(n)
	}
	type ifRow struct {
		device, ifname string
		peak           float64
		in, out, util  float64
	}
	var rows []ifRow
	for _, snap := range snaps {
		for _, iface := range snap.Interfaces {
			if !iface.RateValid {
				continue
			}
			peak := iface.InBps
			if iface.OutBps > peak {
				peak = iface.OutBps
			}
			util := iface.InUtilPercent
			if iface.OutUtilPercent > util {
				util = iface.OutUtilPercent
			}
			rows = append(rows, ifRow{snap.TargetName, iface.Name, peak, iface.InBps, iface.OutBps, util})
		}
	}
	if len(rows) == 0 {
		return fmt.Sprintf("主机 %s 暂无有效的接口速率数据（首轮采集或未启用轮询）。", name), nil
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].peak > rows[j].peak })
	if len(rows) > top {
		rows = rows[:top]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "主机 %s 接口带宽 Top%d:\n", name, len(rows))
	for i, r := range rows {
		fmt.Fprintf(&b, "  %d. %s/%s — in %s/s, out %s/s, 利用率 %.0f%%\n",
			i+1, r.device, r.ifname, humanRate(r.in), humanRate(r.out), r.util)
	}
	return b.String(), nil
}

// execQueryTraps 查询最近收到的 SNMP Trap 事件。
func (h *SreyunCore) execQueryTraps(args map[string]any) (string, error) {
	ref, _ := args["host_id"].(string)
	if ref == "" {
		return "请指定 host_id", nil
	}
	hostID, name := ref, ref
	if hst := h.resolveHostRef(ref); hst != nil {
		hostID, name = hst.ID, hst.Hostname
	}
	if h.s.pg == nil {
		return "未配置 PostgreSQL，无法查询 Trap。", nil
	}
	limit := 30
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
	}
	rows, err := h.s.pg.getSNMPTraps(hostID, limit)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return fmt.Sprintf("主机 %s 最近没有收到 SNMP Trap。", name), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "主机 %s 最近 %d 条 SNMP Trap:\n", name, len(rows))
	for i, r := range rows {
		fmt.Fprintf(&b, "  %d. [%v] 来自 %v trapOID=%v @ %v\n",
			i+1, r["severity"], r["source_ip"], r["trap_oid"], r["received_at"])
	}
	return b.String(), nil
}

// humanUptime 把秒数格式化为可读运行时长。
func humanUptime(sec float64) string {
	if sec <= 0 {
		return "未知"
	}
	total := int(sec)
	d := total / 86400
	hh := (total % 86400) / 3600
	mm := (total % 3600) / 60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd %dh", d, hh)
	case hh > 0:
		return fmt.Sprintf("%dh %dm", hh, mm)
	default:
		return fmt.Sprintf("%dm", mm)
	}
}
