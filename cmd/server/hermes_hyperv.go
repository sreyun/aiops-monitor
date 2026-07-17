package main

import (
	"fmt"
	"sort"
	"strings"

	"aiops-monitor/shared"
)

// ---------------------------------------------------------------------------
// Hermes Hyper-V 工具
//
// 让 AI 能看到「某台物理宿主机下跑了哪些虚拟机、各自什么状态、吃多少资源」，并把
// 虚拟机对应回已纳管主机——这正是"我的机器都跑在 Hyper-V 里"的排障闭环需要的视角。
// 输出面向 LLM：紧凑纯文本，异常 VM 摆最前。
// ---------------------------------------------------------------------------

// hypervResolve maps a host reference to (id, name, guests, errMsg).
func (h *HermesCore) hypervResolve(args map[string]any) (string, string, []shared.HyperVGuest, string) {
	ref, _ := args["host_id"].(string)
	if ref == "" {
		return "", "", nil, "请指定 host_id（物理宿主机 ID）"
	}
	hostID, name := ref, ref
	if hst := h.resolveHostRef(ref); hst != nil {
		hostID, name = hst.ID, hst.Hostname
	}
	guests := h.s.hv.guestsOf(hostID)
	if len(guests) == 0 && h.s.pg != nil {
		// 内存里没有（服务端刚重启）就回落到 PG 最新清单。
		if g, ok := h.s.pg.getHyperVInventoryDecoded(hostID); ok {
			guests = g
		}
	}
	if len(guests) == 0 {
		return hostID, name, nil, fmt.Sprintf(
			"宿主机 %s 没有 Hyper-V 虚拟机数据。可能原因：该主机不是 Hyper-V 宿主、Agent 未上报，或需要更新 Agent。", name)
	}
	return hostID, name, guests, ""
}

// hypervGuestAbnormal reports whether a guest warrants operator attention.
func hypervGuestAbnormal(g shared.HyperVGuest) bool {
	switch {
	case g.Health == "Critical":
		return true
	case g.State != "Running":
		return true
	case g.ReplHealth == "Warning" || g.ReplHealth == "Critical":
		return true
	case g.CPUUsage >= hypervCPUWarn:
		return true
	case g.MemAssignedMB > 0 && g.MemDemandMB > 0 && g.MemDemandMB/g.MemAssignedMB*100 >= hypervMemWarn:
		return true
	}
	return false
}

func (h *HermesCore) execQueryHyperV(args map[string]any) (string, error) {
	_, name, guests, errMsg := h.hypervResolve(args)
	if errMsg != "" {
		return errMsg, nil
	}
	want := strings.ToLower(strings.TrimSpace(fmt.Sprint(args["vm_name"])))
	if want == "<nil>" {
		want = ""
	}

	// 关联索引：把 VM 映射回已纳管主机（名称优先，其次 IP）。
	byName := map[string]*Host{}
	byIP := map[string]*Host{}
	for _, hst := range h.s.store.ListHosts() {
		if hst.Hostname != "" {
			byName[strings.ToLower(hst.Hostname)] = hst
		}
		if hst.IP != "" {
			byIP[hst.IP] = hst
		}
	}
	linked := func(g shared.HyperVGuest) string {
		if m := byName[strings.ToLower(g.Name)]; m != nil {
			return m.Hostname
		}
		for _, ip := range g.IPAddresses {
			if m := byIP[ip]; m != nil {
				return m.Hostname
			}
		}
		return ""
	}

	running, abnormal := 0, 0
	for _, g := range guests {
		if g.State == "Running" {
			running++
		}
		if hypervGuestAbnormal(g) {
			abnormal++
		}
	}

	// 异常在前，其次按名称。
	gs := make([]shared.HyperVGuest, len(guests))
	copy(gs, guests)
	sort.SliceStable(gs, func(i, j int) bool {
		ai, aj := hypervGuestAbnormal(gs[i]), hypervGuestAbnormal(gs[j])
		if ai != aj {
			return ai
		}
		return gs[i].Name < gs[j].Name
	})

	var b strings.Builder
	fmt.Fprintf(&b, "宿主机 %s：共 %d 台虚拟机（运行 %d / 非运行 %d / 需关注 %d）\n",
		name, len(guests), running, len(guests)-running, abnormal)
	for _, g := range gs {
		if want != "" && strings.ToLower(g.Name) != want {
			continue
		}
		flag := "正常"
		if hypervGuestAbnormal(g) {
			flag = "⚠需关注"
		}
		fmt.Fprintf(&b, "- %s [%s] 状态=%s", g.Name, flag, g.State)
		if g.Health != "" && g.Health != "OK" {
			fmt.Fprintf(&b, " 健康=%s", g.Health)
		}
		if g.State == "Running" {
			fmt.Fprintf(&b, " CPU=%.0f%%", g.CPUUsage)
			if g.MemAssignedMB > 0 {
				fmt.Fprintf(&b, " 内存需求/分配=%.0f/%.0fMB", g.MemDemandMB, g.MemAssignedMB)
			}
			if g.UptimeSec > 0 {
				fmt.Fprintf(&b, " 运行=%.1fh", float64(g.UptimeSec)/3600)
			}
		}
		if len(g.IPAddresses) > 0 {
			fmt.Fprintf(&b, " IP=%s", strings.Join(g.IPAddresses, ","))
		}
		if g.ReplState != "" && g.ReplState != "Disabled" {
			fmt.Fprintf(&b, " 复制=%s/%s", g.ReplState, g.ReplHealth)
		}
		if l := linked(g); l != "" {
			fmt.Fprintf(&b, " 关联纳管主机=%s", l)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}
