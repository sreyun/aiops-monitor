package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
)

// ============================================================
// Port Forwarding API — Refactored Interface Layer
//
// This file defines the clean API contract for TCP and HTTP
// forwarding. It separates request/response types from the
// core forward.go implementation, making the interface easy
// to understand, document, and maintain.
// ============================================================

// --- TCP Forwarding API ---

// TCPForwardRequest creates a persistent TCP port mapping.
//
// Example (curl):
//   curl -X POST http://localhost:8529/api/v1/forward \
//     -H "Content-Type: application/json" \
//     -H "Cookie: aiops_session=<your-session>" \
//     -d '{"host_id":"abc123","target_port":3306,"local_port":13306}'
//
// This opens 0.0.0.0:13306 on the server (configurable via forward_listen),
// relaying all TCP traffic through the agent to localhost:3306 on the target host.
// Use any MySQL client to connect:
//   mysql -h 127.0.0.1 -P 13306 -u root -p
type TCPForwardRequest struct {
	HostID     string `json:"host_id"`               // Required: monitored host ID
	TargetPort int    `json:"target_port"`           // Required: port on the target host (1-65535)
	LocalPort  int    `json:"local_port,omitempty"`   // Optional: local listen port (0 = auto-assign)
}

// TCPForwardResponse is returned when a TCP forward rule is created.
type TCPForwardResponse struct {
	ID         string `json:"id"`          // Rule ID (use for deletion)
	HostID     string `json:"host_id"`
	Hostname   string `json:"hostname"`
	TargetPort int    `json:"target_port"`
	LocalPort  int    `json:"local_port"`
	ListenAddr string `json:"listen_addr"`  // e.g. "0.0.0.0:13306"
	Status     string `json:"status"`       // always "active"
	CreatedAt  int64  `json:"created_at"`
	Operator   string `json:"operator"`
	Sessions   int    `json:"sessions"`     // current active connections
}

// --- HTTP Forwarding API ---

// HTTPForwardUsage shows how to use the HTTP proxy endpoint.
//
// The HTTP proxy is stateless — no rule creation needed. Just
// send any HTTP request to the proxy URL:
//
//   GET  /proxy/{hostID}/{port}/{path}
//   POST /proxy/{hostID}/{port}/{path}
//   PUT  /proxy/{hostID}/{port}/{path}
//   DELETE /proxy/{hostID}/{port}/{path}
//
// Example (curl):
//   curl http://localhost:8529/proxy/abc123/8080/api/health
//   curl -X POST http://localhost:8529/proxy/abc123/3000/api/users \
//     -H "Content-Type: application/json" \
//     -d '{"name":"test"}'
//
// The proxy tunnels the request through the agent to
// localhost:{port} on the target host. WebSocket upgrades
// are also supported:
//   ws://localhost:8529/proxy/abc123/8080/ws

// ForwardStatsResponse exposes aggregate forwarding metrics.
type ForwardStatsResponse struct {
	ActiveSessions int   `json:"active_sessions"`
	TotalSessions  int64 `json:"total_sessions"`
	TotalBytes     int64 `json:"total_bytes"`
	Errors         int64 `json:"errors"`
	MaxSessions    int   `json:"max_sessions"`
}

// handleForwardStats returns aggregate forwarding statistics.
// GET /api/v1/forward/stats
func (s *Server) handleForwardStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, ForwardStatsResponse{
		ActiveSessions: int(atomic.LoadInt64(&s.forward.stats.ActiveSessions)),
		TotalSessions:  atomic.LoadInt64(&s.forward.stats.TotalSessions),
		TotalBytes:     atomic.LoadInt64(&s.forward.stats.TotalBytes),
		Errors:         atomic.LoadInt64(&s.forward.stats.Errors),
		MaxSessions:    maxForwardSessions,
	})
}

// handleForwardHealth checks if forwarding is available.
// GET /api/v1/forward/health
func (s *Server) handleForwardHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":     s.cfg.ForwardEnabled(),
		"max_body":    maxForwardBodySize,
		"max_session": maxForwardSessions,
	})
}

// --- HTTP Proxy Shortcuts API ---

// handleHTTPProxyList returns all saved HTTP proxy configurations.
// GET /api/v1/http-proxy
func (s *Server) handleHTTPProxyList(w http.ResponseWriter, r *http.Request) {
	proxies := s.cfg.ListHTTPProxies()
	writeJSON(w, http.StatusOK, proxies)
}

// handleHTTPProxyCreate creates a new HTTP proxy shortcut.
// POST /api/v1/http-proxy
func (s *Server) handleHTTPProxyCreate(w http.ResponseWriter, r *http.Request) {
	var req HTTPProxyConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if req.HostID == "" || req.TargetPort < 1 || req.TargetPort > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "forward.host_port_required")})
		return
	}
	// Lookup hostname
	for _, h := range s.store.ListHosts() {
		if h.ID == req.HostID {
			req.Hostname = h.Hostname
			break
		}
	}
	user, _ := s.currentUser(r)
	req.Operator = user.Username
	if req.Operator == "" {
		req.Operator = s.clientIP(r)
	}
	if err := s.cfg.AddHTTPProxy(req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// Return the created proxy with ID
	proxies := s.cfg.ListHTTPProxies()
	for _, p := range proxies {
		if p.HostID == req.HostID && p.TargetPort == req.TargetPort {
			writeJSON(w, http.StatusOK, p)
			return
		}
	}
	writeJSON(w, http.StatusOK, req)
}

// handleProxyToken generates a short-lived, single-use token for
// authenticating HTTP proxy requests opened via window.open().
// The token is returned as JSON AND set as a SameSite=Lax cookie so the
// new tab automatically carries it — no query-param gymnastics needed.
// GET /api/v1/proxy-token
func (s *Server) handleProxyToken(w http.ResponseWriter, r *http.Request) {
	user, ok := s.currentUser(r)
	if !ok || user.Username == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	tok := s.auth.generateProxyToken(user.Username)
	// Set as a short-lived SameSite=Lax cookie so the subsequent window.open
	// automatically carries it. Using raw header write to avoid any potential
	// interaction with gzip-wrapped ResponseWriter.
	ck := fmt.Sprintf("proxy_token=%s; Path=/; Max-Age=%d; SameSite=Lax", tok, int(proxyTokenTTL.Seconds()))
	w.Header().Add("Set-Cookie", ck)
	writeJSON(w, http.StatusOK, map[string]string{"token": tok})
}

// handleHTTPProxyDelete deletes an HTTP proxy shortcut.
// DELETE /api/v1/http-proxy/{id}
func (s *Server) handleHTTPProxyDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	if err := s.cfg.DeleteHTTPProxy(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
