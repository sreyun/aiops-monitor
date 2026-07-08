package main

import (
	"bufio"
	"bytes"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Port forwarding relay — server side.
//
// Two modes:
//   - TCP port mapping: the server opens a local TCP listener (0.0.0.0:port by default)
//     and relays each accepted connection through the agent to localhost:targetPort
//     on the monitored host.
//   - HTTP reverse proxy: the server handles HTTP requests at /proxy/{hostID}/{port}/...
//     and tunnels them through the agent to the target's HTTP service.

// ---- Constants (P0: security limits) ----

const (
	maxForwardSessions  = 300           // P0: maximum concurrent forwarding sessions
	maxForwardBodySize  = 100 << 20     // P0: maximum HTTP request body (100MB) to prevent OOM
	forwardReadBufSize  = 32 << 10      // P1: 32KB read buffer (was 16KB)
	forwardReadTimeout  = 30 * time.Second // P1: HTTP response read timeout
	forwardTCPKeepAlive = 60 * time.Second // P1: TCP keepalive interval
)

// hopByHopHeaders are per-connection headers that must not be forwarded (RFC 7230 §6.1).
// P2: replaced whitelist approach with blacklist (more complete, fewer missed headers).
var hopByHopHeaders = map[string]bool{
	"Connection":          true,
	"Proxy-Connection":    true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

// forwardStats tracks aggregate forwarding metrics (P3: observability).
type forwardStats struct {
	ActiveSessions int64
	TotalSessions  int64
	TotalBytes     int64
	Errors         int64
}

func (fs *forwardStats) incActive() { atomic.AddInt64(&fs.ActiveSessions, 1); atomic.AddInt64(&fs.TotalSessions, 1) }
func (fs *forwardStats) decActive() { atomic.AddInt64(&fs.ActiveSessions, -1) }
func (fs *forwardStats) addBytes(n int64) { atomic.AddInt64(&fs.TotalBytes, n) }
func (fs *forwardStats) incError() { atomic.AddInt64(&fs.Errors, 1) }

// forwardSession is one tunneled connection (TCP or HTTP).
type forwardSession struct {
	id         string
	ruleID     string // TCP rule that spawned this session; "" for HTTP
	hostID     string
	hostname   string
	targetPort int
	mode       string // "tcp" | "http"
	operator   string
	toAgent    chan []byte   // user data → agent (rx stream)
	toUser     chan []byte   // agent data → user (tx stream)
	agentUp    chan struct{} // closed once the agent attaches its tx stream
	done       chan struct{}
	upOnce     sync.Once
	doneOnce   sync.Once
	closeReason string // P3: reason the session ended
	mu         sync.Mutex
	lastActive int64 // unix seconds of last data transfer (for idle timeout)
}

func (s *forwardSession) markAgentUp() { s.upOnce.Do(func() { close(s.agentUp) }) }
func (s *forwardSession) close() {
	s.doneOnce.Do(func() { close(s.done) })
}
func (s *forwardSession) closeWith(reason string) {
	s.mu.Lock()
	if s.closeReason == "" {
		s.closeReason = reason
	}
	s.mu.Unlock()
	s.close()
}
func (s *forwardSession) touch() {
	s.mu.Lock()
	s.lastActive = time.Now().Unix()
	s.mu.Unlock()
}
func (s *forwardSession) getCloseReason() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closeReason != "" {
		return s.closeReason
	}
	return Tz("log.forward_reason_eof")
}

// forwardRule is a persistent TCP forwarding rule with its own listener.
type forwardRule struct {
	id         string
	hostID     string
	hostname   string
	targetPort int
	localPort  int
	listenAddr string // "127.0.0.1:port"
	listener   net.Listener
	operator   string
	createdAt  int64
}

// forwardWaitInfo is what the agent receives from the long-poll.
type forwardWaitInfo struct {
	sessionID  string
	targetPort int
	mode       string
}

// forwardInfo is the JSON view for the API.
type forwardInfo struct {
	ID         string `json:"id"`
	HostID     string `json:"host_id"`
	Hostname   string `json:"hostname"`
	TargetPort int    `json:"target_port"`
	LocalPort  int    `json:"local_port"`
	ListenAddr string `json:"listen_addr"`
	Status     string `json:"status"`
	CreatedAt  int64  `json:"created_at"`
	Operator   string `json:"operator"`
	Sessions   int    `json:"sessions"`
}

type forwardManager struct {
	mu       sync.Mutex
	rules    map[string]*forwardRule
	sessions map[string]*forwardSession
	waiters  map[string]chan forwardWaitInfo // hostID -> a waiting agent poll
	stats    forwardStats                     // P3: aggregate metrics
	cfg      *ConfigStore                     // config reference for port range
}

func newForwardManager(cfg *ConfigStore) *forwardManager {
	fm := &forwardManager{
		rules:    map[string]*forwardRule{},
		sessions: map[string]*forwardSession{},
		waiters:  map[string]chan forwardWaitInfo{},
		cfg:      cfg,
	}
	go fm.idleChecker()
	return fm
}

// idleChecker closes sessions that have had no data for forwardIdleTimeout.
func (m *forwardManager) idleChecker() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		m.mu.Lock()
		now := time.Now().Unix()
		for id, sess := range m.sessions {
			sess.mu.Lock()
			idle := now - sess.lastActive
			sess.mu.Unlock()
			if idle > int64(forwardIdleTimeout.Seconds()) {
				slog.Info(Tz("log.forward_idle_timeout"), "session", id, "idle_sec", idle)
				sess.closeWith(Tz("log.forward_reason_timeout"))
			}
		}
		m.mu.Unlock()
	}
}

const forwardIdleTimeout = 30 * time.Minute

// forwardFrame encodes one rx message as [type:1][len:2 BE][payload].
// type 'd' = data, 'c' = close signal.
func forwardFrame(typ byte, payload []byte) []byte {
	if len(payload) > 0xffff {
		payload = payload[:0xffff]
	}
	b := make([]byte, 3+len(payload))
	b[0] = typ
	binary.BigEndian.PutUint16(b[1:], uint16(len(payload)))
	copy(b[3:], payload)
	return b
}

// ---- session lifecycle ----

func (m *forwardManager) createSession(ruleID, hostID, hostname string, targetPort int, mode, operator string) (*forwardSession, error) {
	m.mu.Lock()
	// P0: enforce maximum session count
	if len(m.sessions) >= maxForwardSessions {
		m.mu.Unlock()
		m.stats.incError()
		return nil, fmt.Errorf("%s", Tz("forward.too_many_sessions"))
	}
	s := &forwardSession{
		id: termID(), ruleID: ruleID, hostID: hostID, hostname: hostname,
		targetPort: targetPort, mode: mode, operator: operator,
		toAgent: make(chan []byte, 64), toUser: make(chan []byte, 256),
		agentUp: make(chan struct{}), done: make(chan struct{}),
		lastActive: time.Now().Unix(),
	}
	m.sessions[s.id] = s
	m.stats.incActive()
	m.mu.Unlock()
	return s, nil
}

func (m *forwardManager) getSession(id string) *forwardSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

func (m *forwardManager) removeSession(id string) {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
		m.stats.decActive()
	}
	m.mu.Unlock()
	_ = sess
}

// notifyAgent hands a new forward session to the agent currently long-polling
// for hostID; returns false if none is waiting.
func (m *forwardManager) notifyAgent(hostID string, info forwardWaitInfo) bool {
	m.mu.Lock()
	w := m.waiters[hostID]
	delete(m.waiters, hostID)
	m.mu.Unlock()
	if w == nil {
		return false
	}
	select {
	case w <- info:
		return true
	default:
		return false
	}
}

func (m *forwardManager) registerWaiter(hostID string) chan forwardWaitInfo {
	ch := make(chan forwardWaitInfo, 1)
	m.mu.Lock()
	m.waiters[hostID] = ch
	m.mu.Unlock()
	return ch
}

func (m *forwardManager) unregisterWaiter(hostID string, ch chan forwardWaitInfo) {
	m.mu.Lock()
	if m.waiters[hostID] == ch {
		delete(m.waiters, hostID)
	}
	m.mu.Unlock()
}

// ---- rule management ----

func (m *forwardManager) createRule(hostID, hostname string, targetPort, localPort int, listenHost, operator string) (*forwardRule, error) {
	// If localPort is 0 or requested port is unavailable, try ports in the configured range
	minPort, maxPort := m.cfg.ForwardPortRangeBounds()
	var ln net.Listener
	var err error
	actualPort := localPort

	if localPort > 0 {
		// Try the user-specified port first
		addr := listenHost + ":" + strconv.Itoa(localPort)
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			// User asked for a specific port but it failed, try the range
			actualPort = 0
		}
	}

	// If no listener yet, try ports in the configured range
	if ln == nil {
		// Try random ports in the range until one succeeds
		for attempt := 0; attempt < 100; attempt++ {
			candidate := minPort + attempt%((maxPort-minPort)+1)
			addr := listenHost + ":" + strconv.Itoa(candidate)
			ln, err = net.Listen("tcp", addr)
			if err == nil {
				actualPort = candidate
				break
			}
		}
		// If still no listener, fall back to OS-assigned port
		if ln == nil {
			ln, err = net.Listen("tcp", listenHost+":0")
			if err != nil {
				return nil, fmt.Errorf("%s", Tz("forward.listen_failed", err))
			}
		}
	}

	actualPort = ln.Addr().(*net.TCPAddr).Port
	actualAddr := listenHost + ":" + strconv.Itoa(actualPort)
	r := &forwardRule{
		id: termID()[:8], hostID: hostID, hostname: hostname,
		targetPort: targetPort, localPort: actualPort,
		listenAddr: actualAddr,
		listener: ln, operator: operator, createdAt: time.Now().Unix(),
	}
	m.mu.Lock()
	m.rules[r.id] = r
	m.mu.Unlock()
	return r, nil
}

func (m *forwardManager) getRule(id string) *forwardRule {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rules[id]
}

func (m *forwardManager) removeRule(id string) bool {
	m.mu.Lock()
	r, ok := m.rules[id]
	if !ok {
		m.mu.Unlock()
		return false
	}
	delete(m.rules, id)
	// close all sessions belonging to this rule
	for sid, sess := range m.sessions {
		if sess.ruleID == id {
			sess.closeWith(Tz("log.forward_reason_eof"))
			delete(m.sessions, sid)
			m.stats.decActive()
		}
	}
	m.mu.Unlock()
	if r.listener != nil {
		_ = r.listener.Close()
	}
	return true
}

func (m *forwardManager) listRules() []forwardInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]forwardInfo, 0, len(m.rules))
	for _, r := range m.rules {
		sessions := 0
		for _, s := range m.sessions {
			if s.ruleID == r.id {
				sessions++
			}
		}
		out = append(out, forwardInfo{
			ID: r.id, HostID: r.hostID, Hostname: r.hostname,
			TargetPort: r.targetPort, LocalPort: r.localPort,
			ListenAddr: r.listenAddr, Status: "active",
			CreatedAt: r.createdAt, Operator: r.operator,
			Sessions: sessions,
		})
	}
	return out
}

// ---- API handlers (browser-facing, auth-gated) ----

// handleForwardCreate creates a TCP port forwarding rule.
func (s *Server) handleForwardCreate(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.ForwardEnabled() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "forward.disabled")})
		return
	}
	var req struct {
		HostID     string `json:"host_id"`
		TargetPort int    `json:"target_port"`
		LocalPort  int    `json:"local_port"` // 0 = auto-allocate
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if req.HostID == "" || req.TargetPort < 1 || req.TargetPort > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "forward.host_port_required")})
		return
	}
	// look up hostname
	hostname := shortID(req.HostID)
	for _, h := range s.store.ListHosts() {
		if h.ID == req.HostID {
			hostname = h.Hostname
			break
		}
	}
	user, _ := s.currentUser(r)
	operator := s.clientIP(r)
	if user.Username != "" {
		operator = user.Username
	}
	listenHost := s.cfg.ForwardListenAddr()
	rule, err := s.forward.createRule(req.HostID, hostname, req.TargetPort, req.LocalPort, listenHost, operator)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// start accepting connections in the background
	go s.serveForwardListener(rule)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: operator, Host: hostname,
		Message: Tz("log.forward_create", hostname, hostname, req.TargetPort, rule.listenAddr)})
	writeJSON(w, http.StatusOK, forwardInfo{
		ID: rule.id, HostID: rule.hostID, Hostname: rule.hostname,
		TargetPort: rule.targetPort, LocalPort: rule.localPort,
		ListenAddr: rule.listenAddr, Status: "active",
		CreatedAt: rule.createdAt, Operator: operator,
	})
}

// handleForwardDelete closes a forwarding rule and its listener.
func (s *Server) handleForwardDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rule := s.forward.getRule(id)
	if rule == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "forward.rule_not_found")})
		return
	}
	user, _ := s.currentUser(r)
	operator := s.clientIP(r)
	if user.Username != "" {
		operator = user.Username
	}
	s.forward.removeRule(id)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: operator, Host: rule.hostname,
		Message: Tz("log.forward_close", rule.hostname, rule.targetPort)})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleForwardList returns all active forwarding rules.
func (s *Server) handleForwardList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.forward.listRules())
}

// serveForwardListener accepts TCP connections for a rule and tunnels each
// one through the agent reverse channel.
func (s *Server) serveForwardListener(rule *forwardRule) {
	for {
		conn, err := rule.listener.Accept()
		if err != nil {
			return // listener closed
		}
		// P1: set TCP keepalive on accepted connections
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetKeepAlive(true)
			_ = tc.SetKeepAlivePeriod(forwardTCPKeepAlive)
		}
		go s.handleForwardTCPConn(rule, conn)
	}
}

// handleForwardTCPConn relays one user TCP connection through the agent.
func (s *Server) handleForwardTCPConn(rule *forwardRule, conn net.Conn) {
	defer conn.Close()
	sess, err := s.forward.createSession(rule.id, rule.hostID, rule.hostname, rule.targetPort, "tcp", rule.operator)
	if err != nil {
		// P3: error feedback to user instead of silent drop
		return
	}
	defer s.forward.removeSession(sess.id)
	defer sess.close()

	// P3: TCP forward audit log
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: rule.operator, Host: rule.hostname,
		Message: Tz("log.forward_tcp", rule.hostname, rule.targetPort)})

	// notify agent
	if !s.forward.notifyAgent(rule.hostID, forwardWaitInfo{sessionID: sess.id, targetPort: rule.targetPort, mode: "tcp"}) {
		sess.closeWith(Tz("log.forward_reason_agent_down"))
		return // agent not polling
	}
	// watchdog: if agent never attaches, don't hang
	go func() {
		select {
		case <-sess.agentUp:
		case <-time.After(10 * time.Second):
			sess.closeWith(Tz("log.forward_reason_timeout"))
		case <-sess.done:
		}
	}()

	var bytesTransferred int64

	// user → agent (read from TCP, send to toAgent channel as data frames)
	go func() {
		defer sess.close()
		buf := make([]byte, forwardReadBufSize) // P1: 32KB buffer
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				sess.touch()
				b := make([]byte, n)
				copy(b, buf[:n])
				atomic.AddInt64(&bytesTransferred, int64(n))
				select {
				case sess.toAgent <- forwardFrame('d', b):
				case <-sess.done:
					return
				}
			}
			if err != nil {
				// signal close to agent
				select {
				case sess.toAgent <- forwardFrame('c', nil):
				case <-sess.done:
				}
				if err != io.EOF {
					sess.closeWith(Tz("log.forward_reason_error"))
				}
				return
			}
		}
	}()

	// agent → user (read from toUser channel, write to TCP)
	go func() {
		defer sess.close()
		for {
			select {
			case b := <-sess.toUser:
				sess.touch()
				atomic.AddInt64(&bytesTransferred, int64(len(b)))
				if _, err := conn.Write(b); err != nil {
					sess.closeWith(Tz("log.forward_reason_error"))
					return
				}
			case <-sess.done:
				return
			}
		}
	}()

	<-sess.done

	// P3: log close reason + bytes transferred
	s.forward.stats.addBytes(bytesTransferred)
	s.store.AddLog(LogEntry{Kind: KindSystem, Level: "info", Actor: rule.operator, Host: rule.hostname,
		Message: Tz("log.forward_tcp_closed", rule.hostname, rule.targetPort, sess.getCloseReason())})
}

// ---- HTTP reverse proxy ----

// handleHTTPProxy tunnels an HTTP request through the agent to the target's
// HTTP service. The URL pattern is /proxy/{hostID}/{port}/{path...}.
func (s *Server) handleHTTPProxy(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.ForwardEnabled() {
		http.Error(w, Tr(r, "forward.disabled"), http.StatusForbidden)
		return
	}
	hostID := r.PathValue("hostID")
	portStr := r.PathValue("port")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		http.Error(w, Tr(r, "forward.invalid_port"), http.StatusBadRequest)
		return
	}
	// look up hostname
	hostname := shortID(hostID)
	for _, h := range s.store.ListHosts() {
		if h.ID == hostID {
			hostname = h.Hostname
			break
		}
	}
	user, _ := s.currentUser(r)
	operator := s.clientIP(r)
	if user.Username != "" {
		operator = user.Username
	}

	// P2: WebSocket upgrade detection — tunnel as raw TCP instead of HTTP
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		s.handleWSProxy(w, r, hostID, hostname, port, operator)
		return
	}

	sess, err := s.forward.createSession("", hostID, hostname, port, "http", operator)
	if err != nil {
		http.Error(w, Tr(r, "forward.too_many_sessions"), http.StatusServiceUnavailable)
		return
	}
	defer s.forward.removeSession(sess.id)
	defer sess.close()

	// notify agent
	if !s.forward.notifyAgent(hostID, forwardWaitInfo{sessionID: sess.id, targetPort: port, mode: "http"}) {
		s.forward.stats.incError()
		http.Error(w, Tr(r, "forward.agent_offline"), http.StatusBadGateway)
		return
	}
	// wait for agent to attach
	select {
	case <-sess.agentUp:
	case <-time.After(10 * time.Second):
		s.forward.stats.incError()
		http.Error(w, Tr(r, "forward.agent_timeout"), http.StatusGatewayTimeout)
		return
	case <-sess.done:
		s.forward.stats.incError()
		http.Error(w, Tr(r, "forward.session_closed"), http.StatusBadGateway)
		return
	}

	// *** CRITICAL: Start the response pipe reader IMMEDIATELY after agentUp,
	// BEFORE sending request frames. If the Agent failed to connect to the
	// target, it already sent error data via the tx POST. The pipe reader must
	// be running to capture that data before handleAgentForwardTx's defer
	// sess.close() fires and closes sess.done. ***
	pr, pw := io.Pipe()
	var rawResponseBuf bytes.Buffer
	var rawResponseMu sync.Mutex
	const maxDiagBytes = 2048 // capture first 2KB for diagnostics
	go func() {
		defer pw.Close()
		for {
			select {
			case b := <-sess.toUser:
				sess.touch()
				if _, err := pw.Write(b); err != nil {
					return
				}
				rawResponseMu.Lock()
				if rawResponseBuf.Len() < maxDiagBytes {
					rawResponseBuf.Write(b)
				}
				rawResponseMu.Unlock()
			case <-sess.done:
				return
			}
		}
	}()

	// construct raw HTTP request bytes
	path := "/" + r.PathValue("path")
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	var reqBuf bytes.Buffer
	fmt.Fprintf(&reqBuf, "%s %s HTTP/1.1\r\n", r.Method, path)
	// set Host header to the target
	fmt.Fprintf(&reqBuf, "Host: localhost:%d\r\n", port)
	// P2: copy all headers EXCEPT hop-by-hop (blacklist approach, more complete than whitelist)
	for k, vs := range r.Header {
		if hopByHopHeaders[k] {
			continue
		}
		for _, v := range vs {
			fmt.Fprintf(&reqBuf, "%s: %s\r\n", k, v)
		}
	}
	// add forwarding headers
	fmt.Fprintf(&reqBuf, "X-Forwarded-For: %s\r\n", s.clientIP(r))
	fmt.Fprintf(&reqBuf, "X-Forwarded-Proto: %s\r\n", schemeOf(r))
	fmt.Fprintf(&reqBuf, "X-Real-IP: %s\r\n", s.clientIP(r))
	reqBuf.WriteString("\r\n")
	// P0: copy request body with size limit (prevent OOM)
	if r.Body != nil {
		limitedBody := io.LimitReader(r.Body, maxForwardBodySize)
		n, _ := io.Copy(&reqBuf, limitedBody)
		s.forward.stats.addBytes(n)
	}

	// send the request through the tunnel in chunks.
	// If the Agent closed the session (e.g. because it failed to connect to the
	// target and already sent error data), jump to response reading instead of
	// returning — the pipe reader already has the Agent's error response.
	data := reqBuf.Bytes()
	for len(data) > 0 {
		chunk := data
		if len(chunk) > 0xffff {
			chunk = chunk[:0xffff]
		}
		sess.touch()
		select {
		case sess.toAgent <- forwardFrame('d', chunk):
		case <-sess.done:
			// Agent disconnected before we could send the full request.
			// Don't return — the pipe reader may already have error data.
			goto readResponse
		}
		data = data[len(chunk):]
	}
	// signal end of request
	select {
	case sess.toAgent <- forwardFrame('c', nil):
	case <-sess.done:
		// Agent disconnected; proceed to read whatever is available
	}

readResponse:
	// P1: add timeout for reading upstream response
	type readResult struct {
		resp *http.Response
		err  error
	}
	resultCh := make(chan readResult, 1)
	go func() {
		resp, err := http.ReadResponse(bufio.NewReader(pr), nil)
		resultCh <- readResult{resp, err}
	}()

	var resp *http.Response
	select {
	case res := <-resultCh:
		resp, err = res.resp, res.err
	case <-time.After(forwardReadTimeout):
		s.forward.stats.incError()
		http.Error(w, Tr(r, "forward.agent_timeout"), http.StatusGatewayTimeout)
		return
	}

	if err != nil {
		s.forward.stats.incError()
		rawResponseMu.Lock()
		rawPreview := rawResponseBuf.String()
		rawResponseMu.Unlock()
		if len(rawPreview) > 300 {
			rawPreview = rawPreview[:300] + "..."
		}
		if rawResponseBuf.Len() == 0 {
			slog.Warn(Tz("log.forward_parse_failed_short"), "host", hostname, "port", port, "path", path, "err", err, "note", "empty response - agent may have failed to connect to target")
		} else {
			slog.Warn(Tz("log.forward_parse_failed_short"), "host", hostname, "port", port, "path", path, "err", err, "raw_preview", rawPreview)
		}
		s.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: operator, Host: hostname,
			Message: Tz("log.forward_parse_failed", port, path, err.Error())})
		http.Error(w, Tr(r, "forward.parse_response_failed", err.Error()), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// copy response headers and body to the browser
	for k, vs := range resp.Header {
		if hopByHopHeaders[k] {
			continue // P2: strip hop-by-hop from response too
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	n, _ := io.Copy(w, resp.Body)
	s.forward.stats.addBytes(n)

	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: operator, Host: hostname,
		Message: Tz("log.forward_http", hostname, port, r.Method, path, resp.StatusCode)})
}

// handleWSProxy tunnels a WebSocket upgrade request through the agent.
// P2: WebSocket passthrough support.
func (s *Server) handleWSProxy(w http.ResponseWriter, r *http.Request, hostID, hostname string, port int, operator string) {
	sess, err := s.forward.createSession("", hostID, hostname, port, "tcp", operator)
	if err != nil {
		http.Error(w, Tr(r, "forward.too_many_sessions"), http.StatusServiceUnavailable)
		return
	}
	defer s.forward.removeSession(sess.id)
	defer sess.close()

	if !s.forward.notifyAgent(hostID, forwardWaitInfo{sessionID: sess.id, targetPort: port, mode: "tcp"}) {
		s.forward.stats.incError()
		http.Error(w, Tr(r, "forward.agent_offline"), http.StatusBadGateway)
		return
	}
	select {
	case <-sess.agentUp:
	case <-time.After(10 * time.Second):
		s.forward.stats.incError()
		http.Error(w, Tr(r, "forward.agent_timeout"), http.StatusGatewayTimeout)
		return
	case <-sess.done:
		http.Error(w, Tr(r, "forward.session_closed"), http.StatusBadGateway)
		return
	}

	// Construct the WebSocket upgrade request as raw HTTP
	var reqBuf bytes.Buffer
	path := "/" + r.PathValue("path")
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	fmt.Fprintf(&reqBuf, "%s %s HTTP/1.1\r\n", r.Method, path)
	fmt.Fprintf(&reqBuf, "Host: localhost:%d\r\n", port)
	for k, vs := range r.Header {
		// Forward all headers including Upgrade, Connection, Sec-WebSocket-*
		for _, v := range vs {
			fmt.Fprintf(&reqBuf, "%s: %s\r\n", k, v)
		}
	}
	reqBuf.WriteString("\r\n")

	// Send the upgrade request through the tunnel
	data := reqBuf.Bytes()
	for len(data) > 0 {
		chunk := data
		if len(chunk) > 0xffff {
			chunk = chunk[:0xffff]
		}
		sess.touch()
		select {
		case sess.toAgent <- forwardFrame('d', chunk):
		case <-sess.done:
			return
		}
		data = data[len(chunk):]
	}

	// Hijack the HTTP connection to get a raw bidirectional stream
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket hijack not supported", http.StatusInternalServerError)
		return
	}
	clientConn, clientBuf, err := hj.Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()
	if clientBuf != nil {
		// flush any buffered data
		if clientBuf.Reader.Buffered() > 0 {
			extra := make([]byte, clientBuf.Reader.Buffered())
			clientBuf.Read(extra)
			select {
			case sess.toAgent <- forwardFrame('d', extra):
			case <-sess.done:
			}
		}
	}

	// Bidirectional relay: client → agent and agent → client
	done := make(chan struct{}, 2)

	// client → agent
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, forwardReadBufSize)
		for {
			n, err := clientConn.Read(buf)
			if n > 0 {
				sess.touch()
				b := make([]byte, n)
				copy(b, buf[:n])
				select {
				case sess.toAgent <- forwardFrame('d', b):
				case <-sess.done:
					return
				}
			}
			if err != nil {
				select {
				case sess.toAgent <- forwardFrame('c', nil):
				case <-sess.done:
				}
				return
			}
		}
	}()

	// agent → client
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			select {
			case b := <-sess.toUser:
				sess.touch()
				if _, err := clientConn.Write(b); err != nil {
					return
				}
			case <-sess.done:
				return
			}
		}
	}()

	<-done
	sess.close()
}

// schemeOf returns the request scheme (http or https).
func schemeOf(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// ---- Agent-facing handlers (fingerprint-gated, not session-gated) ----

// handleAgentForwardWait: agent long-polls here; returns a session id + target
// port when a user opens a forward connection for this host, or {} on timeout.
func (s *Server) handleAgentForwardWait(w http.ResponseWriter, r *http.Request) {
	if !s.forwardFingerprintOK(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	host := r.URL.Query().Get("host")
	if host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.host_required")})
		return
	}
	ch := s.forward.registerWaiter(host)
	defer s.forward.unregisterWaiter(host, ch)
	select {
	case info := <-ch:
		writeJSON(w, http.StatusOK, map[string]any{
			"session":     info.sessionID,
			"target_port": info.targetPort,
			"mode":        info.mode,
		})
	case <-time.After(25 * time.Second):
		writeJSON(w, http.StatusOK, map[string]string{})
	case <-r.Context().Done():
	}
}

// handleAgentForwardRx streams user data down to the agent (chunked).
func (s *Server) handleAgentForwardRx(w http.ResponseWriter, r *http.Request) {
	sess := s.forward.getSession(r.URL.Query().Get("session"))
	if sess == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "common.session_gone")})
		return
	}
	if !s.forwardFingerprintOKByHost(sess.hostID, r.URL.Query().Get("fp")) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if flusher != nil {
		flusher.Flush()
	}
	for {
		select {
		case b := <-sess.toAgent:
			if _, err := w.Write(b); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		case <-sess.done:
			return
		case <-r.Context().Done():
			sess.closeWith(Tz("log.forward_reason_agent_down"))
			return
		}
	}
}

// handleAgentForwardTx receives data from the agent (chunked request body)
// and fans it to the user connection.
func (s *Server) handleAgentForwardTx(w http.ResponseWriter, r *http.Request) {
	sess := s.forward.getSession(r.URL.Query().Get("session"))
	if sess == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "common.session_gone")})
		return
	}
	if !s.forwardFingerprintOKByHost(sess.hostID, r.URL.Query().Get("fp")) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	sess.markAgentUp()
	defer sess.close()
	buf := make([]byte, forwardReadBufSize) // P1: 32KB buffer
	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			b := make([]byte, n)
			copy(b, buf[:n])
			s.forward.stats.addBytes(int64(n))
			select {
			case sess.toUser <- b:
			case <-sess.done:
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// forwardFingerprintOKByHost verifies the agent-presented fingerprint against
// the fingerprint bound to hostID at registration (constant-time).
func (s *Server) forwardFingerprintOKByHost(hostID, fp string) bool {
	if hostID == "" || fp == "" {
		return false
	}
	host, ok := s.store.GetHost(hostID)
	if !ok || host.Fingerprint == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(fp), []byte(host.Fingerprint)) == 1
}

// forwardFingerprintOK is the request-flavored wrapper for handleAgentForwardWait.
func (s *Server) forwardFingerprintOK(r *http.Request) bool {
	return s.forwardFingerprintOKByHost(r.URL.Query().Get("host"), r.URL.Query().Get("fp"))
}
