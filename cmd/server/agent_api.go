package main

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"log/slog"
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
	// Admission: a valid install token is required to register a NEW agent.
	// Once registered, the agent authenticates subsequent reports by fingerprint,
	// so rotating this token never disturbs already-installed agents.
	//
	// v5.2.6: Allow re-registration WITHOUT install token when the host is
	// already known (matching fingerprint in store). This is critical for
	// server restart recovery: if the DB was lost or the agent's config has
	// no token, the agent can still re-join by proving its machine fingerprint.
	// New agents (unknown host_id + unknown fingerprint) still require a token.
	if s.cfg.AgentTokenRequired() && !s.cfg.ValidInstallToken(req.Token) {
		// Check if this host already exists with a matching fingerprint
		existingHost, hostExists := s.store.GetHost(req.HostID)
		if !hostExists || existingHost.Fingerprint == "" || existingHost.Fingerprint != req.Fingerprint {
			// Unknown host or fingerprint doesn't match → require install token
			writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "agent.invalid_token")})
			return
		}
		// Known host with matching fingerprint → allow re-registration (server restart recovery)
		slog.Info("允许已知主机免Token重新注册（服务端重启恢复）", "host_id", req.HostID)
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
		// Gzip decompression failure — likely caused by proxy corruption
		// on external networks. Log the error so operators can diagnose.
		slog.Warn("Agent 上报 gzip 解压失败（可能外网代理损坏）",
			"remote", r.RemoteAddr, "content_encoding", r.Header.Get("Content-Encoding"),
			"content_length", r.ContentLength, "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	defer body.Close()

	var rep shared.Report
	if err := json.NewDecoder(body).Decode(&rep); err != nil {
		slog.Warn("Agent 上报 JSON 解析失败",
			"remote", r.RemoteAddr, "content_encoding", r.Header.Get("Content-Encoding"),
			"err", err)
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
