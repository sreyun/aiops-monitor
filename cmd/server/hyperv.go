package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aiops-monitor/shared"
)

// ============================================================================
// Hyper-V 虚拟机：agent 上报接收 + 前端查询 + 指标/变更/关联
//
// 数据模型与硬件资产同构：每台物理宿主机一份 guest 清单，慢变、要追踪变更。
// 走独立指纹鉴权通道，落 PG(JSONB) + 每 VM 数值指标进 VictoriaMetrics。
// ============================================================================

// handleAgentHyperV receives a physical host's Hyper-V guest inventory.
func (s *Server) handleAgentHyperV(w http.ResponseWriter, r *http.Request) {
	var rep shared.HyperVReport
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		slog.Warn("Hyper-V 上报 JSON 解析失败", "err", err, "remote", r.RemoteAddr)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if rep.HostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host_id required"})
		return
	}

	// Fingerprint verification (same pattern as hardware/terminal/forward).
	fp := r.Header.Get("X-Agent-Fingerprint")
	if fp == "" {
		fp = r.URL.Query().Get("fp")
	}
	if !s.forwardFingerprintOKByHost(rep.HostID, fp) {
		slog.Warn("Hyper-V 上报指纹校验失败", "host_id", rep.HostID, "fp", fp, "remote", r.RemoteAddr)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "fingerprint mismatch"})
		return
	}

	hostname, ip := rep.HostID, ""
	if h := s.hostByID(rep.HostID); h != nil {
		hostname, ip = h.Hostname, h.IP
	}
	if hostname == rep.HostID && rep.HostName != "" {
		hostname = rep.HostName
	}

	// 缓存最新清单（供告警评估每轮复用）。采集失败时 put 会保留上一份好数据、只记 lastError。
	s.hv.put(rep.HostID, hostname, ip, rep.Guests, rep.Error)

	if rep.Error != "" {
		slog.Warn("Hyper-V 采集失败，保留上一份清单不覆盖", "host_id", rep.HostID, "err", rep.Error)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	ts := rep.Timestamp
	if ts == 0 {
		ts = time.Now().Unix()
	}

	if s.pg != nil {
		// 变更必须在 upsert **之前**比对：upsert 会覆盖上一份，之后就没有旧值可比了。
		s.recordHyperVChanges(rep.HostID, rep.Guests)
		s.pg.upsertHyperVInventory(rep.HostID, hostname, rep.Guests)
	}

	// 每 VM 数值指标写入 VictoriaMetrics（趋势曲线 + 历史）。
	s.pushHyperVMetrics(rep.HostID, rep.Guests, ts)

	slog.Info("Hyper-V 上报已存储", "host_id", rep.HostID, "vms", len(rep.Guests))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleHyperVList returns Hyper-V inventories, grouped per physical host. With
// ?host= it returns one host; otherwise every host that reported guests. Each
// guest is enriched with linked_host_id/linked_host_name when it maps to a
// managed host (by name or IP) — that's the "my machines run in Hyper-V" bridge.
func (s *Server) handleHyperVList(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"inventories": []any{}})
		return
	}
	host := r.URL.Query().Get("host")
	var rows []map[string]any
	if host != "" {
		if inv, ok := s.pg.getHyperVInventory(host); ok {
			rows = []map[string]any{inv}
		}
	} else {
		var err error
		rows, err = s.pg.getAllHyperVInventories()
		if err != nil {
			slog.Warn("查询 Hyper-V 清单失败", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
	}
	s.enrichHyperVLinks(rows)
	writeJSON(w, http.StatusOK, map[string]any{"inventories": rows})
}

// handleHyperVEvents returns a host's VM change/state events, newest first.
func (s *Server) handleHyperVEvents(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host")
	if hostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host required"})
		return
	}
	if s.pg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"events": []any{}})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := s.pg.getHyperVEvents(hostID, limit)
	if err != nil {
		slog.Warn("查询 Hyper-V 事件失败", "host", hostID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

// handleDeleteHyperV removes a host's Hyper-V inventory (in-memory + PG).
func (s *Server) handleDeleteHyperV(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("hostID")
	if hostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hostID required"})
		return
	}
	s.hv.remove(hostID)
	if s.pg != nil {
		s.pg.deleteHyperVInventory(hostID)
	}
	slog.Info("删除 Hyper-V 清单", "host", hostID, "actor", s.clientIP(r))
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.delete_hyperv", hostID)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// enrichHyperVLinks annotates each guest in the rows with linked_host_id /
// linked_host_name when it corresponds to a managed host. Matches by hostname
// (case-insensitive) first, then by any of the guest's reported IPs.
func (s *Server) enrichHyperVLinks(rows []map[string]any) {
	if len(rows) == 0 {
		return
	}
	byName := map[string]*Host{}
	byIP := map[string]*Host{}
	for _, h := range s.store.ListHosts() {
		if h.Hostname != "" {
			byName[strings.ToLower(h.Hostname)] = h
		}
		if h.IP != "" {
			byIP[h.IP] = h
		}
	}
	for _, row := range rows {
		guests, _ := row["guests"].([]any)
		for _, gi := range guests {
			g, ok := gi.(map[string]any)
			if !ok {
				continue
			}
			var match *Host
			if name, _ := g["name"].(string); name != "" {
				match = byName[strings.ToLower(name)]
			}
			if match == nil {
				if ips, ok := g["ip_addresses"].([]any); ok {
					for _, ipi := range ips {
						if ip, _ := ipi.(string); ip != "" {
							if h := byIP[ip]; h != nil {
								match = h
								break
							}
						}
					}
				}
			}
			if match != nil {
				g["linked_host_id"] = match.ID
				g["linked_host_name"] = match.Hostname
			}
		}
	}
}

// pushHyperVMetrics writes per-VM numeric series to VictoriaMetrics. Labels are
// host + vm; the timestamp is milliseconds (Prometheus import format — the same
// ×1000 gotcha the hardware/netflow paths hit). All lines go out in a single POST.
func (s *Server) pushHyperVMetrics(hostID string, guests []shared.HyperVGuest, ts int64) {
	if !s.vm.enabled() || len(guests) == 0 {
		return
	}
	ms := ts * 1000
	host := lblEsc(hostID)
	var b strings.Builder
	for _, g := range guests {
		if g.Name == "" {
			continue
		}
		vm := lblEsc(g.Name)
		stateVal := 0.0
		if g.State == "Running" {
			stateVal = 1
		}
		fmt.Fprintf(&b, "aiops_hyperv_state{host=%q,vm=%q} %g %d\n", host, vm, stateVal, ms)
		fmt.Fprintf(&b, "aiops_hyperv_cpu_percent{host=%q,vm=%q} %g %d\n", host, vm, g.CPUUsage, ms)
		if g.MemAssignedMB > 0 {
			fmt.Fprintf(&b, "aiops_hyperv_mem_assigned_mb{host=%q,vm=%q} %g %d\n", host, vm, g.MemAssignedMB, ms)
		}
		if g.MemDemandMB > 0 {
			fmt.Fprintf(&b, "aiops_hyperv_mem_demand_mb{host=%q,vm=%q} %g %d\n", host, vm, g.MemDemandMB, ms)
		}
		if g.UptimeSec > 0 {
			fmt.Fprintf(&b, "aiops_hyperv_uptime_sec{host=%q,vm=%q} %d %d\n", host, vm, g.UptimeSec, ms)
		}
	}
	if b.Len() > 0 {
		s.vm.pushRawLine(strings.TrimRight(b.String(), "\n"))
	}
}

// recordHyperVChanges diffs the incoming guests against the stored inventory and
// persists VM add / remove / state-change events. Identity is the VM GUID (falls
// back to name) so a renamed VM isn't logged as remove+add.
//
// Only diffs when a previous inventory exists: the first report is a baseline,
// not a set of "added" events (mirrors recordHardwareChanges).
func (s *Server) recordHyperVChanges(hostID string, cur []shared.HyperVGuest) {
	if s.pg == nil {
		return
	}
	prev, ok := s.pg.getHyperVInventoryDecoded(hostID)
	if !ok {
		return // 首次入库 = 建立基线，不产生事件
	}
	for _, c := range diffHyperVGuests(prev, cur) {
		s.pg.insertHyperVEvent(hostID, c.vmName, c.vmID, c.kind, c.severity, c.message)
	}
}

// hypervChange is one detected inventory difference.
type hypervChange struct {
	vmName, vmID, kind, severity, message string
}

// diffHyperVGuests compares two guest lists (by GUID, name fallback) and returns
// add / remove / state-change events. Pure function so it's unit-testable without
// a database. Callers must have a previous baseline — a first-ever inventory is
// not diffed (that's handled by recordHyperVChanges returning early).
func diffHyperVGuests(prev, cur []shared.HyperVGuest) []hypervChange {
	prevByID := map[string]shared.HyperVGuest{}
	for _, g := range prev {
		prevByID[hypervKey(g)] = g
	}
	curByID := map[string]shared.HyperVGuest{}
	for _, g := range cur {
		curByID[hypervKey(g)] = g
	}

	var out []hypervChange
	for _, g := range cur {
		k := hypervKey(g)
		old, existed := curLookup(prevByID, k)
		if !existed {
			out = append(out, hypervChange{g.Name, g.ID, "vm_added", "info",
				fmt.Sprintf("发现新虚拟机 %s（%s）", g.Name, g.State)})
		} else if old.State != g.State {
			sev := "info"
			// 由运行转为非运行 = 值得关注（宕机/被停）。
			if old.State == "Running" && g.State != "Running" {
				sev = "warning"
			}
			out = append(out, hypervChange{g.Name, g.ID, "state_change", sev,
				fmt.Sprintf("虚拟机 %s 状态变化：%s → %s", g.Name, old.State, g.State)})
		}
	}
	for _, g := range prev {
		if _, still := curByID[hypervKey(g)]; !still {
			out = append(out, hypervChange{g.Name, g.ID, "vm_removed", "warning",
				fmt.Sprintf("虚拟机 %s 已移除或迁移", g.Name)})
		}
	}
	return out
}

func curLookup(m map[string]shared.HyperVGuest, k string) (shared.HyperVGuest, bool) {
	g, ok := m[k]
	return g, ok
}

// hypervKey identifies a guest across reports: prefer the stable GUID, fall back
// to the name so guests without a reported ID still diff sanely.
func hypervKey(g shared.HyperVGuest) string {
	if g.ID != "" {
		return g.ID
	}
	return "name:" + g.Name
}
