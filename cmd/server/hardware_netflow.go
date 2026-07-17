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
// Agent-facing ingest endpoints (fingerprint-gated)
// ============================================================================

// handleAgentHardware receives Redfish hardware snapshots from agents.
func (s *Server) handleAgentHardware(w http.ResponseWriter, r *http.Request) {
	var rep shared.HardwareReport
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		slog.Warn("硬件上报 JSON 解析失败", "err", err, "remote", r.RemoteAddr)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if rep.HostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host_id required"})
		return
	}

	// Fingerprint verification (same pattern as terminal/forward)
	fp := r.Header.Get("X-Agent-Fingerprint")
	if fp == "" {
		fp = r.URL.Query().Get("fp")
	}
	if !s.forwardFingerprintOKByHost(rep.HostID, fp) {
		slog.Warn("硬件上报指纹校验失败", "host_id", rep.HostID, "fp", fp, "remote", r.RemoteAddr)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "fingerprint mismatch"})
		return
	}

	// Store snapshots in PG (upsert)
	if s.pg != nil {
		for _, snap := range rep.Snapshots {
			s.pg.upsertHardwareSnapshot(rep.HostID, snap)

			// Write numeric metrics to VM
			s.vmHardwareMetrics(rep.HostID, snap)

			// Detect health change → hardware event
			if snap.Health != "" && snap.Health != "OK" {
				s.pg.insertHardwareEvent(rep.HostID, snap.TargetName, "health_change",
					strings.ToLower(snap.Health), fmt.Sprintf("健康状态: %s", snap.Health))
			}
		}
		slog.Info("硬件上报已存储", "host_id", rep.HostID, "snapshots", len(rep.Snapshots))
	} else {
		slog.Warn("硬件上报已接收但 PG 未配置，数据未持久化", "host_id", rep.HostID, "snapshots", len(rep.Snapshots))
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleAgentNetFlow receives aggregated NetFlow/packet flows from agents.
func (s *Server) handleAgentNetFlow(w http.ResponseWriter, r *http.Request) {
	var rep shared.NetFlowReport
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

	// Write aggregated metrics to VM
	s.vmNetFlowMetrics(rep.HostID, rep)

	// Optionally store flow details in PG (for detailed queries + CSV export)
	if s.pg != nil && len(rep.Flows) > 0 {
		s.pg.insertFlowRecords(rep.HostID, rep.Source, rep.Flows)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ============================================================================
// Frontend query endpoints
// ============================================================================

// handleHardwareHealth returns the latest hardware snapshot for a host.
func (s *Server) handleHardwareHealth(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host")
	if hostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host required"})
		return
	}

	if s.pg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"snapshots": []any{}})
		return
	}

	snapshots, err := s.pg.getHardwareSnapshots(hostID)
	if err != nil {
		slog.Warn("查询硬件快照失败", "host", hostID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": snapshots})
}

// handleHardwareHistory returns hardware metric history from VM.
func (s *Server) handleHardwareHistory(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host")
	metric := r.URL.Query().Get("metric") // temperature, power, fan_rpm, health_score
	rangeStr := r.URL.Query().Get("range") // e.g. "24h", "7d"

	if hostID == "" || metric == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host and metric required"})
		return
	}

	from, to := parseTimeRange(rangeStr)
	target := r.URL.Query().Get("target") // optional: specific Redfish target name

	if !s.vm.enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"points": []any{}})
		return
	}

	// Build PromQL query based on metric
	var promql string
	switch metric {
	case "temperature":
		promql = fmt.Sprintf(`aiops_hardware_temperature{host="%s"`, hostID)
	case "power":
		promql = fmt.Sprintf(`aiops_hardware_power_watts{host="%s"`, hostID)
	case "fan_rpm":
		promql = fmt.Sprintf(`aiops_hardware_fan_rpm{host="%s"`, hostID)
	case "health_score":
		promql = fmt.Sprintf(`aiops_hardware_health_score{host="%s"`, hostID)
	default:
		promql = fmt.Sprintf(`aiops_hardware_temperature{host="%s"`, hostID)
	}
	if target != "" {
		promql += fmt.Sprintf(`, target="%s"`, target)
	}
	promql += "}"

	points := s.vm.queryRawRange(promql, from, to)
	writeJSON(w, http.StatusOK, map[string]any{"points": points})
}

// handleNetFlowSummary returns Top-N aggregated flow data.
func (s *Server) handleNetFlowSummary(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host")
	rangeStr := r.URL.Query().Get("range")
	dimension := r.URL.Query().Get("dimension") // src_ip, dst_ip, src_port, dst_port, protocol
	topN := 20
	if n, err := strconv.Atoi(r.URL.Query().Get("top")); err == nil && n > 0 && n <= 100 {
		topN = n
	}

	if hostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host required"})
		return
	}

	from, to := parseTimeRange(rangeStr)

	if !s.vm.enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"summary": []any{}, "total_bytes": 0})
		return
	}

	if dimension == "" {
		dimension = "src_ip"
	}

	// Query VM for aggregated flow bytes by dimension
	promql := fmt.Sprintf(`sum by (%s) (aiops_netflow_bytes{host="%s"})`, dimension, hostID)
	points := s.vm.queryRawRange(promql, from, to)

	// Aggregate to top-N
	agg := make(map[string]uint64)
	for _, p := range points {
		if m, ok := p.(map[string]any); ok {
			label := ""
			if labels, ok := m["labels"].(map[string]any); ok {
				if v, ok := labels[dimension].(string); ok {
					label = v
				}
			}
			if val, ok := m["value"].(float64); ok {
				agg[label] += uint64(val)
			}
		}
	}

	type kv struct {
		Key   string `json:"key"`
		Bytes uint64 `json:"bytes"`
	}
	var sorted []kv
	for k, v := range agg {
		sorted = append(sorted, kv{k, v})
	}
	// Simple sort (top-N)
	for i := 0; i < len(sorted) && i < topN; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Bytes > sorted[i].Bytes {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	if len(sorted) > topN {
		sorted = sorted[:topN]
	}

	writeJSON(w, http.StatusOK, map[string]any{"summary": sorted})
}

// handleNetFlowFlows returns flow detail records from PG.
func (s *Server) handleNetFlowFlows(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host")
	if hostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host required"})
		return
	}

	if s.pg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"flows": []any{}})
		return
	}

	limit := 200
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 1000 {
		limit = n
	}
	filter := r.URL.Query().Get("filter") // e.g. "src_ip:10.0.0.0/8"

	flows, err := s.pg.getFlowRecords(hostID, filter, limit)
	if err != nil {
		slog.Warn("查询 Flow 明细失败", "host", hostID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"flows": flows})
}

// handleNetFlowPackets returns packet capture statistics.
func (s *Server) handleNetFlowPackets(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host")
	rangeStr := r.URL.Query().Get("range")

	if hostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host required"})
		return
	}

	from, to := parseTimeRange(rangeStr)

	if !s.vm.enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"points": []any{}})
		return
	}

	promql := fmt.Sprintf(`aiops_netflow_packets{host="%s", source="packet"}`, hostID)
	points := s.vm.queryRawRange(promql, from, to)
	writeJSON(w, http.StatusOK, map[string]any{"points": points})
}

// ============================================================================
// VM write helpers
// ============================================================================

func (s *Server) vmHardwareMetrics(hostID string, snap shared.HardwareSnapshot) {
	if !s.vm.enabled() {
		return
	}
	ts := snap.Timestamp
	target := snap.TargetName

	// Health score: OK=2, Warning=1, Critical=0
	var score float64
	switch snap.Health {
	case "OK":
		score = 2
	case "Warning":
		score = 1
	case "Critical":
		score = 0
	}

	s.vm.pushHardware(hostID, target, ts, "aiops_hardware_health_score", score)

	// Temperature sensors
	for _, t := range snap.Temps {
		s.vm.pushHardwareLabeled(hostID, target, ts, "aiops_hardware_temperature",
			t.Reading, "sensor", t.Name)
	}

	// Fan RPM
	for _, f := range snap.Fans {
		s.vm.pushHardwareLabeled(hostID, target, ts, "aiops_hardware_fan_rpm",
			float64(f.RPM), "fan_name", f.Name)
	}

	// Power watts
	if snap.Power.TotalWatts > 0 {
		s.vm.pushHardware(hostID, target, ts, "aiops_hardware_power_watts", snap.Power.TotalWatts)
	}
}

func (s *Server) vmNetFlowMetrics(hostID string, rep shared.NetFlowReport) {
	if !s.vm.enabled() {
		return
	}
	for _, f := range rep.Flows {
		labels := fmt.Sprintf(`host="%s",src_ip="%s",dst_ip="%s",src_port="%d",dst_port="%d",proto="%d",source="%s"`,
			hostID, f.SrcIP, f.DstIP, f.SrcPort, f.DstPort, f.Protocol, rep.Source)

		s.vm.pushRawLine(fmt.Sprintf("aiops_netflow_bytes{%s} %d %d", labels, f.Bytes, rep.Timestamp))
		s.vm.pushRawLine(fmt.Sprintf("aiops_netflow_packets{%s} %d %d", labels, f.Packets, rep.Timestamp))
	}
	if rep.Stats.DroppedPackets > 0 {
		s.vm.pushRawLine(fmt.Sprintf(`aiops_netflow_dropped{host="%s"} %d %d`,
			hostID, rep.Stats.DroppedPackets, rep.Timestamp))
	}
}

// ============================================================================
// Helpers
// ============================================================================

func parseTimeRange(rangeStr string) (from, to int64) {
	to = time.Now().Unix()
	from = to - 86400 // default 24h
	if rangeStr == "" {
		return
	}
	rangeStr = strings.TrimSpace(rangeStr)
	if strings.HasSuffix(rangeStr, "h") {
		if h, err := strconv.Atoi(strings.TrimSuffix(rangeStr, "h")); err == nil {
			from = to - int64(h)*3600
		}
	} else if strings.HasSuffix(rangeStr, "d") {
		if d, err := strconv.Atoi(strings.TrimSuffix(rangeStr, "d")); err == nil {
			from = to - int64(d)*86400
		}
	} else if strings.HasSuffix(rangeStr, "m") {
		if m, err := strconv.Atoi(strings.TrimSuffix(rangeStr, "m")); err == nil {
			from = to - int64(m)*60
		}
	}
	return
}
