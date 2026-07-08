package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ---- custom checks ----

func (s *Server) handleGetChecks(w http.ResponseWriter, r *http.Request) {
	checks := s.cfg.Checks()
	st := s.checks.snapshot()
	out := make([]map[string]any, 0, len(checks)+1)

	// Built-in self health-check is always first
	selfEntry := map[string]any{
		"id": selfCheckID, "name": SelfCheckName, "type": "http",
		"target": "http://127.0.0.1:" + portFromAddr(s.checks.selfAddr) + "/healthz",
		"interval_sec": 30, "level": "critical", "enabled": true,
		"ok": true, "message": "", "checked_at": int64(0), "latency_ms": 0.0,
		"builtin": true,
	}
	if s2, ok := st[selfCheckID]; ok {
		selfEntry["ok"], selfEntry["message"], selfEntry["checked_at"], selfEntry["latency_ms"] = s2.OK, s2.Message, s2.CheckedAt, s2.LatencyMs
	}
	out = append(out, selfEntry)

	for _, c := range checks {
		m := map[string]any{
			"id": c.ID, "name": c.Name, "type": c.Type, "target": c.Target,
			"interval_sec": c.IntervalSec, "level": c.Level, "enabled": c.Enabled,
			"ok": true, "message": "", "checked_at": int64(0), "latency_ms": 0.0,
			"status_code": 0, "cert_days": -1, "loss_pct": -1.0,
		}
		if s2, ok := st[c.ID]; ok {
			m["ok"], m["message"], m["checked_at"], m["latency_ms"] = s2.OK, s2.Message, s2.CheckedAt, s2.LatencyMs
			m["status_code"], m["cert_days"], m["loss_pct"] = s2.StatusCode, s2.CertDays, s2.LossPct
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUpsertCheck(w http.ResponseWriter, r *http.Request) {
	var c CustomCheck
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	c.Name = strings.TrimSpace(c.Name)
	c.Target = strings.TrimSpace(c.Target)
	if c.Name == "" || c.Target == "" || (c.Type != "http" && c.Type != "tcp" && c.Type != "ping" && c.Type != "process") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "名称 / 目标 / 类型不合法"})
		return
	}
	if c.IntervalSec < 5 {
		c.IntervalSec = 30
	}
	if c.Level != "warning" && c.Level != "critical" {
		c.Level = "critical"
	}
	saved, err := s.cfg.UpsertCheck(c)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.checks.runNow(saved.ID)
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: s.clientIP(r), Message: "保存自定义监控：" + saved.Name})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID})
}

// handleRunCheck triggers one immediate probe of a check (fire-and-forget).
func (s *Server) handleRunCheck(w http.ResponseWriter, r *http.Request) {
	s.checks.runNow(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleCheckHistory returns a check's recorded trend series (latency / status /
// loss over time) for the history-curve view.
func (s *Server) handleCheckHistory(w http.ResponseWriter, r *http.Request) {
	pts := s.checks.HistoryOf(r.PathValue("id"))
	if pts == nil {
		pts = []CheckPoint{}
	}
	writeJSON(w, http.StatusOK, pts)
}

func (s *Server) handleDeleteCheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = s.cfg.DeleteCheck(id)
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "删除自定义监控 " + id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
