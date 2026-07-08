package main

import (
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
