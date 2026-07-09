package main

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"aiops-monitor/shared"
)

// decompressBody transparently handles gzip Content-Encoding on request bodies.
// Go's http.Server does NOT auto-decompress request bodies (unlike responses).
// Since agent v5.1.0, the report payload may be gzip-compressed to save bandwidth.
// Returns the original r.Body when no compression is used (backward-compatible).
func decompressBody(r *http.Request) (io.ReadCloser, error) {
	if r.Header.Get("Content-Encoding") != "gzip" {
		return r.Body, nil
	}
	gr, err := gzip.NewReader(r.Body)
	if err != nil {
		return nil, err
	}
	return gr, nil
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	body, err := decompressBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	defer body.Close()

	var req struct {
		HostID      string `json:"host_id"`
		Hostname    string `json:"hostname"`
		Token       string `json:"token"`
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	// Admission: a valid install token is required to register a new agent.
	// Once registered, the agent authenticates subsequent reports by fingerprint,
	// so rotating this token never disturbs already-installed agents.
	if s.cfg.AgentTokenRequired() && !s.cfg.ValidInstallToken(req.Token) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "agent.invalid_token")})
		return
	}
	if req.Fingerprint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "agent.fingerprint_required")})
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
//
// Since v5.1.0, agents may gzip-compress the JSON body (Content-Encoding: gzip)
// to reduce bandwidth. This handler transparently decompresses when needed.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	body, err := decompressBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	defer body.Close()

	var rep shared.Report
	if err := json.NewDecoder(body).Decode(&rep); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if rep.HostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.host_required")})
		return
	}
	h, ok := s.store.UpsertAuthenticated(rep, rep.Fingerprint)
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "agent.fingerprint_failed")})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "host_id": h.ID})
}
