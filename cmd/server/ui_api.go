package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleHosts(w http.ResponseWriter, r *http.Request) {
	hosts := s.store.ListHosts()
	now := time.Now().Unix()
	offline := int64(s.cfg.Thresholds().OfflineAfter.Seconds())

	type hostView struct {
		*Host
		Online bool `json:"online"`
	}
	views := make([]hostView, 0, len(hosts))
	for _, h := range hosts {
		if cat, ok := s.cfg.CategoryOverride(h.ID); ok {
			h.Category = cat // manual override wins over the agent-reported category
		}
		views = append(views, hostView{Host: h, Online: now-h.LastSeen <= offline})
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].Category != views[j].Category {
			return views[i].Category < views[j].Category
		}
		return views[i].Hostname < views[j].Hostname
	})
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) handleHostMetrics(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	samples, ok := s.store.GetSamples(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "common.host_not_found")})
		return
	}
	writeJSON(w, http.StatusOK, samples)
}

// handleHostHistory returns time-series data for a host within [from, to] range.
// Query params: from (unix timestamp), to (unix timestamp).
// Defaults: from = now - 24h, to = now.
func (s *Server) handleHostHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	now := time.Now().Unix()

	// Parse query parameters
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	var from, to int64
	if toStr != "" {
		var err error
		to, err = strconv.ParseInt(toStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_to_param")})
			return
		}
	} else {
		to = now
	}

	if fromStr != "" {
		var err error
		from, err = strconv.ParseInt(fromStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_from_param")})
			return
		}
	} else {
		from = now - 86400 // default: last 24 hours
	}

	if from >= to {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.from_less_than_to")})
		return
	}

	samples, ok := s.store.GetHistory(id, from, to)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "common.host_not_found")})
		return
	}

	writeJSON(w, http.StatusOK, samples)
}

// handleSetCategory sets (or clears, when empty) a manual category override.
func (s *Server) handleSetCategory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Category string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	cat := strings.TrimSpace(req.Category)
	_ = s.cfg.SetCategory(id, cat)
	msg := Tz("log.set_category", shortID(id), cat)
	if cat == "" {
		msg = Tz("log.clear_category", shortID(id))
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: msg})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "host_id": id, "category": cat})
}

func (s *Server) handleDeleteHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok := s.store.DeleteHost(id)
	_ = s.cfg.SetCategory(id, "") // drop any override for the removed host
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "common.host_not_found")})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.delete_host", shortID(id))})
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted", "host_id": id})
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	alerts := Evaluate(s.store.ListHosts(), s.cfg.Thresholds())
	// stamp threshold alerts with their first-fired time (check alerts carry it already)
	since := s.notifier.ActiveSince()
	states := s.store.AlertStates()
	for i := range alerts {
		if t, ok := since[alertKey(alerts[i])]; ok {
			alerts[i].Since = t
		}
		alerts[i].Status = states[alertKey(alerts[i])]
	}
	alerts = append(alerts, s.checks.DownAlerts()...)
	// also attach status for check alerts
	for i := range alerts {
		if alerts[i].Status == "" {
			if st, ok := states[alertKey(alerts[i])]; ok {
				alerts[i].Status = st
			}
		}
	}
	if alerts == nil {
		alerts = []Alert{}
	}
	writeJSON(w, http.StatusOK, alerts)
}

// handleEvents returns recent plugin-generated events (the Python/AI layer's findings).
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	events := s.store.RecentEvents()
	if events == nil {
		events = []storedEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

// handleActivity returns the unified activity log (operations + system + plugin).
func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	items := s.store.RecentActivity()
	if items == nil {
		items = []LogEntry{}
	}
	writeJSON(w, http.StatusOK, items)
}

// handleHostsMeta returns minimal host info (id + hostname) for the process-check UI.
func (s *Server) handleHostsMeta(w http.ResponseWriter, r *http.Request) {
	hosts := s.store.ListHosts()
	type hostMeta struct {
		ID       string `json:"id"`
		Hostname string `json:"hostname"`
	}
	out := make([]hostMeta, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, hostMeta{ID: h.ID, Hostname: h.Hostname})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	hosts := s.store.ListHosts()
	now := time.Now().Unix()
	th := s.cfg.Thresholds()
	offline := int64(th.OfflineAfter.Seconds())

	online := 0
	for _, h := range hosts {
		if now-h.LastSeen <= offline {
			online++
		}
	}
	crit, warn := 0, 0
	for _, a := range append(Evaluate(hosts, th), s.checks.DownAlerts()...) {
		if a.Level == "critical" {
			crit++
		} else {
			warn++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_hosts":      len(hosts),
		"online_hosts":     online,
		"offline_hosts":    len(hosts) - online,
		"critical_alerts":  crit,
		"warning_alerts":   warn,
		"plugin_events":    len(s.store.RecentEvents()),
		"server_time_unix": now,
		"version":          appVersion,
		"terminal_enabled": s.cfg.TerminalEnabled(),
	})
}

// alertAckSilenceReq is the JSON body for ack/silence operations.
type alertAckSilenceReq struct {
	HostID string `json:"host_id"`
	Type   string `json:"type"`
	Scope  string `json:"scope"`
}

func (s *Server) handleAlertAck(w http.ResponseWriter, r *http.Request) {
	var req alertAckSilenceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	key := req.HostID + "/" + req.Type + "/" + req.Scope
	s.store.SetAlertState(key, "acknowledged")
	msg := Tz("log.alert_ack", shortID(req.HostID), req.Type)
	if req.Scope != "" {
		msg = Tz("log.alert_ack_scope", shortID(req.HostID), req.Type, req.Scope)
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: msg})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "key": key, "new_status": "acknowledged"})
}

func (s *Server) handleAlertSilence(w http.ResponseWriter, r *http.Request) {
	var req alertAckSilenceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	key := req.HostID + "/" + req.Type + "/" + req.Scope
	s.store.SetAlertState(key, "silenced")
	msg := Tz("log.alert_silence", shortID(req.HostID), req.Type)
	if req.Scope != "" {
		msg = Tz("log.alert_silence_scope", shortID(req.HostID), req.Type, req.Scope)
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: msg})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "key": key, "new_status": "silenced"})
}

func (s *Server) handleAlertClear(w http.ResponseWriter, r *http.Request) {
	var req alertAckSilenceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	key := req.HostID + "/" + req.Type + "/" + req.Scope
	s.store.ClearAlertState(key)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: Tz("log.alert_clear", shortID(req.HostID), req.Type)})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "key": key, "new_status": ""})
}
