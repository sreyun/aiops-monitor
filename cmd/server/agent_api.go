package main

import (
	"encoding/json"
	"net/http"
	"time"

	"aiops-monitor/shared"
)

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HostID      string `json:"host_id"`
		Hostname    string `json:"hostname"`
		Token       string `json:"token"`
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	// Admission: a valid install token is required to register a new agent.
	// Once registered, the agent authenticates subsequent reports by fingerprint,
	// so rotating this token never disturbs already-installed agents.
	if s.cfg.AgentTokenRequired() && !s.cfg.ValidInstallToken(req.Token) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "无效或缺失的接入 Token"})
		return
	}
	if req.Fingerprint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "fingerprint required"})
		return
	}
	s.store.RegisterHost(req.HostID, req.Hostname, req.Fingerprint)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"host_id":          req.HostID,
		"server_time_unix": time.Now().Unix(),
	})
}

// handleReport ingests a metrics report (base + custom + events) from an agent.
// Authentication is by machine fingerprint (bound at registration), NOT by the
// install token — so rotating the token never breaks already-installed agents.
// Verification + upsert happen atomically inside Store.UpsertAuthenticated to
// avoid a TOCTOU window and double-lock overhead on the hot report path.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	var rep shared.Report
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if rep.HostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host_id required"})
		return
	}
	h, ok := s.store.UpsertAuthenticated(rep, rep.Fingerprint)
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "指纹鉴权失败：未注册或指纹不匹配"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "host_id": h.ID})
}
