package main

// SNMP 采集数据的服务端接入：Agent 上报接收（指纹校验）、VM 时序写入（基数封顶）、
// PG 快照存储、前端查询端点。风格对齐 hardware_netflow.go 的 handleAgentNetFlow /
// vmNetFlowMetrics / rollupNetFlow。

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"aiops-monitor/shared"
)

// snmpMaxIfaces 是单台设备一轮最多写入 VM 的接口数上限。接口是稳定基数（不像
// netflow 的 src_port 那样爆炸），但仍硬性封顶，守住"时序库成本由序列数决定"这条命。
const snmpMaxIfaces = 300

// ============================================================================
// Agent-facing ingest（指纹校验）
// ============================================================================

// handleAgentSNMP 接收 agent 轮询上报的 SNMP 设备指标。
func (s *Server) handleAgentSNMP(w http.ResponseWriter, r *http.Request) {
	var rep shared.SNMPReport
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if rep.HostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host_id required"})
		return
	}
	fp := r.Header.Get("X-Agent-Fingerprint")
	if fp == "" {
		fp = r.URL.Query().Get("fp")
	}
	if !s.forwardFingerprintOKByHost(rep.HostID, fp) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "fingerprint mismatch"})
		return
	}

	// 缓存最新快照供告警评估每轮复用（含采集失败的快照，用于报"采集失败"告警）。
	hostname, ip := rep.HostID, ""
	if h := s.hostByID(rep.HostID); h != nil {
		hostname, ip = h.Hostname, h.IP
	}
	s.snmp.put(rep.HostID, hostname, ip, rep.Snapshots)

	for _, snap := range rep.Snapshots {
		// 采集失败（超时/认证失败）时快照各字段是零值：只报警不落库/不写时序，
		// 否则会把上一份好数据覆盖成空白，接口瞬间全变 down。
		if snap.Error != "" {
			slog.Warn("SNMP 采集失败，保留上一份快照不覆盖", "host", rep.HostID, "device", snap.TargetName, "err", snap.Error)
			continue
		}
		s.vmSNMPMetrics(rep.HostID, snap)
		if s.pg != nil {
			s.pg.upsertSNMPSnapshot(rep.HostID, snap)
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ============================================================================
// VM 写入（基数封顶）
// ============================================================================

func (s *Server) vmSNMPMetrics(hostID string, snap shared.SNMPSnapshot) {
	if !s.vm.enabled() {
		return
	}
	for _, line := range rollupSNMP(hostID, snap) {
		s.vm.pushRawLine(line)
	}
}

// rollupSNMP 把一台设备一轮快照转成一组 BOUNDED 的 Prometheus 行。
// 抽成纯函数是为了能直接对"产出多少条序列"做断言——每接口固定条数、且接口数封顶。
// 注意：Prometheus 导入格式时间戳单位是毫秒（snap.Timestamp 是秒，须 *1000，
// 否则历史全写进 1970，见 hardware_netflow.go 同款注释）。
func rollupSNMP(hostID string, snap shared.SNMPSnapshot) []string {
	var out []string
	ts := snap.Timestamp * 1000
	host := lblEsc(hostID)
	device := lblEsc(snap.TargetName)

	reach := 0.0
	if snap.Reachable {
		reach = 1
	}
	out = append(out, fmt.Sprintf(`aiops_snmp_reachable{host="%s",device="%s"} %g %d`, host, device, reach, ts))
	if snap.System.UptimeSec > 0 {
		out = append(out, fmt.Sprintf(`aiops_snmp_sys_uptime{host="%s",device="%s"} %g %d`, host, device, snap.System.UptimeSec, ts))
	}

	ifaces := snap.Interfaces
	if len(ifaces) > snmpMaxIfaces {
		slog.Warn("SNMP 接口数超过 VM 写入上限，截断", "host", hostID, "device", snap.TargetName, "count", len(ifaces), "max", snmpMaxIfaces)
		ifaces = ifaces[:snmpMaxIfaces]
	}
	for _, iface := range ifaces {
		lbl := fmt.Sprintf(`host="%s",device="%s",ifindex="%d",ifname="%s"`, host, device, iface.Index, lblEsc(iface.Name))
		operUp := 0.0
		if iface.OperUp {
			operUp = 1
		}
		out = append(out, fmt.Sprintf(`aiops_snmp_if_oper_up{%s} %g %d`, lbl, operUp, ts))
		if iface.SpeedBps > 0 {
			out = append(out, fmt.Sprintf(`aiops_snmp_if_speed_bps{%s} %d %d`, lbl, iface.SpeedBps, ts))
		}
		if iface.RateValid {
			out = append(out,
				fmt.Sprintf(`aiops_snmp_if_in_bps{%s} %g %d`, lbl, iface.InBps, ts),
				fmt.Sprintf(`aiops_snmp_if_out_bps{%s} %g %d`, lbl, iface.OutBps, ts),
				fmt.Sprintf(`aiops_snmp_if_in_util{%s} %g %d`, lbl, iface.InUtilPercent, ts),
				fmt.Sprintf(`aiops_snmp_if_out_util{%s} %g %d`, lbl, iface.OutUtilPercent, ts),
				fmt.Sprintf(`aiops_snmp_if_in_err_pps{%s} %g %d`, lbl, iface.InErrPps, ts),
				fmt.Sprintf(`aiops_snmp_if_out_err_pps{%s} %g %d`, lbl, iface.OutErrPps, ts),
				fmt.Sprintf(`aiops_snmp_if_in_disc_pps{%s} %g %d`, lbl, iface.InDiscardPps, ts),
				fmt.Sprintf(`aiops_snmp_if_out_disc_pps{%s} %g %d`, lbl, iface.OutDiscardPps, ts))
		}
	}
	return out
}

// ============================================================================
// 前端查询端点
// ============================================================================

// handleSNMPList 返回一台主机（agent）下所有被轮询设备的最新快照。
func (s *Server) handleSNMPList(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host")
	if hostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host required"})
		return
	}
	if s.pg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"devices": []any{}})
		return
	}
	devices, err := s.pg.getSNMPSnapshots(hostID)
	if err != nil {
		slog.Warn("查询 SNMP 快照失败", "host", hostID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

// handleSNMPTraps 返回一台主机最近收到的 trap 事件。
func (s *Server) handleSNMPTraps(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host")
	if hostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host required"})
		return
	}
	if s.pg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"traps": []any{}})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	traps, err := s.pg.getSNMPTraps(hostID, limit)
	if err != nil {
		slog.Warn("查询 SNMP Trap 失败", "host", hostID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"traps": traps})
}

// handleSNMPInterfaceHistory 返回某设备某接口某指标的 VM 时序（供前端画曲线）。
func (s *Server) handleSNMPInterfaceHistory(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host")
	device := r.URL.Query().Get("device")
	metric := r.URL.Query().Get("metric") // in_bps/out_bps/in_util/out_util/oper_up ...
	ifname := r.URL.Query().Get("ifname")
	rangeStr := r.URL.Query().Get("range")
	if hostID == "" || metric == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host and metric required"})
		return
	}
	if !s.vm.enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"points": []any{}})
		return
	}
	from, to := parseTimeRange(rangeStr)
	promql := fmt.Sprintf(`aiops_snmp_if_%s{host="%s"`, metric, hostID)
	if device != "" {
		promql += fmt.Sprintf(`, device="%s"`, device)
	}
	if ifname != "" {
		promql += fmt.Sprintf(`, ifname="%s"`, ifname)
	}
	promql += "}"
	points := s.vm.queryRawRange(promql, from, to)
	writeJSON(w, http.StatusOK, map[string]any{"points": points})
}
