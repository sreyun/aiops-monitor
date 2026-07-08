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
	"sync"
	"time"
)

// Port forwarding relay — server side.
//
// This module is completely independent from terminal.go. It reuses the same
// architectural pattern (agent reverse channel + bidirectional HTTP streams +
// [type:1][len:2 BE][payload] framing) but with its own session manager, API
// endpoints, and handlers. The two modules share zero code paths.
//
// Two modes:
//   - TCP port mapping: the server opens a local TCP listener (127.0.0.1:port)
//     and relays each accepted connection through the agent to localhost:targetPort
//     on the monitored host.
//   - HTTP reverse proxy: the server handles HTTP requests at /proxy/{hostID}/{port}/...
//     and tunnels them through the agent to the target's HTTP service.

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
	mu         sync.Mutex
	lastActive int64 // unix seconds of last data transfer (for idle timeout)
}

func (s *forwardSession) markAgentUp() { s.upOnce.Do(func() { close(s.agentUp) }) }
func (s *forwardSession) close() {
	s.doneOnce.Do(func() { close(s.done) })
}
func (s *forwardSession) touch() {
	s.mu.Lock()
	s.lastActive = time.Now().Unix()
	s.mu.Unlock()
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
}

func newForwardManager() *forwardManager {
	fm := &forwardManager{
		rules:    map[string]*forwardRule{},
		sessions: map[string]*forwardSession{},
		waiters:  map[string]chan forwardWaitInfo{},
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
				slog.Info("转发会话空闲超时，自动关闭", "session", id, "idle_sec", idle)
				sess.close()
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

func (m *forwardManager) createSession(ruleID, hostID, hostname string, targetPort int, mode, operator string) *forwardSession {
	s := &forwardSession{
		id: termID(), ruleID: ruleID, hostID: hostID, hostname: hostname,
		targetPort: targetPort, mode: mode, operator: operator,
		toAgent: make(chan []byte, 64), toUser: make(chan []byte, 256),
		agentUp: make(chan struct{}), done: make(chan struct{}),
		lastActive: time.Now().Unix(),
	}
	m.mu.Lock()
	m.sessions[s.id] = s
	m.mu.Unlock()
	return s
}

func (m *forwardManager) getSession(id string) *forwardSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

func (m *forwardManager) removeSession(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
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

func (m *forwardManager) createRule(hostID, hostname string, targetPort, localPort int, operator string) (*forwardRule, error) {
	addr := "127.0.0.1:" + strconv.Itoa(localPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// fallback: auto-allocate
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, fmt.Errorf("无法监听端口: %w", err)
		}
	}
	localPort = ln.Addr().(*net.TCPAddr).Port
	r := &forwardRule{
		id: termID()[:8], hostID: hostID, hostname: hostname,
		targetPort: targetPort, localPort: localPort,
		listenAddr: "127.0.0.1:" + strconv.Itoa(localPort),
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
			sess.close()
			delete(m.sessions, sid)
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
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "端口转发已被管理员禁用"})
		return
	}
	var req struct {
		HostID     string `json:"host_id"`
		TargetPort int    `json:"target_port"`
		LocalPort  int    `json:"local_port"` // 0 = auto-allocate
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.HostID == "" || req.TargetPort < 1 || req.TargetPort > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host_id 和 target_port 必填"})
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
	rule, err := s.forward.createRule(req.HostID, hostname, req.TargetPort, req.LocalPort, operator)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// start accepting connections in the background
	go s.serveForwardListener(rule)
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: operator, Host: hostname,
		Message: fmt.Sprintf("创建端口转发 %s → %s:%d (本地 %s)", hostname, hostname, req.TargetPort, rule.listenAddr)})
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
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "转发规则不存在"})
		return
	}
	user, _ := s.currentUser(r)
	operator := s.clientIP(r)
	if user.Username != "" {
		operator = user.Username
	}
	s.forward.removeRule(id)
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: operator, Host: rule.hostname,
		Message: fmt.Sprintf("关闭端口转发 %s → :%d", rule.hostname, rule.targetPort)})
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
		go s.handleForwardTCPConn(rule, conn)
	}
}

// handleForwardTCPConn relays one user TCP connection through the agent.
func (s *Server) handleForwardTCPConn(rule *forwardRule, conn net.Conn) {
	defer conn.Close()
	sess := s.forward.createSession(rule.id, rule.hostID, rule.hostname, rule.targetPort, "tcp", rule.operator)
	defer s.forward.removeSession(sess.id)
	defer sess.close()

	// notify agent
	if !s.forward.notifyAgent(rule.hostID, forwardWaitInfo{sessionID: sess.id, targetPort: rule.targetPort, mode: "tcp"}) {
		return // agent not polling; connection dropped silently
	}
	// watchdog: if agent never attaches, don't hang
	go func() {
		select {
		case <-sess.agentUp:
		case <-time.After(10 * time.Second):
			sess.close()
		case <-sess.done:
		}
	}()

	// user → agent (read from TCP, send to toAgent channel as data frames)
	go func() {
		defer sess.close()
		buf := make([]byte, 16<<10)
		for {
			n, err := conn.Read(buf)
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
				// signal close to agent
				select {
				case sess.toAgent <- forwardFrame('c', nil):
				case <-sess.done:
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
				if _, err := conn.Write(b); err != nil {
					return
				}
			case <-sess.done:
				return
			}
		}
	}()

	<-sess.done
}

// ---- HTTP reverse proxy ----

// handleHTTPProxy tunnels an HTTP request through the agent to the target's
// HTTP service. The URL pattern is /proxy/{hostID}/{port}/{path...}.
func (s *Server) handleHTTPProxy(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.ForwardEnabled() {
		http.Error(w, "端口转发已被管理员禁用", http.StatusForbidden)
		return
	}
	hostID := r.PathValue("hostID")
	portStr := r.PathValue("port")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		http.Error(w, "无效的端口号", http.StatusBadRequest)
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

	sess := s.forward.createSession("", hostID, hostname, port, "http", operator)
	defer s.forward.removeSession(sess.id)
	defer sess.close()

	// notify agent
	if !s.forward.notifyAgent(hostID, forwardWaitInfo{sessionID: sess.id, targetPort: port, mode: "http"}) {
		http.Error(w, "Agent 未在线或未启用转发通道", http.StatusBadGateway)
		return
	}
	// wait for agent to attach
	select {
	case <-sess.agentUp:
	case <-time.After(10 * time.Second):
		http.Error(w, "Agent 接入超时", http.StatusGatewayTimeout)
		return
	case <-sess.done:
		http.Error(w, "转发会话已关闭", http.StatusBadGateway)
		return
	}

	// construct raw HTTP request bytes
	path := "/" + r.PathValue("path")
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	var reqBuf bytes.Buffer
	fmt.Fprintf(&reqBuf, "%s %s HTTP/1.1\r\n", r.Method, path)
	// set Host header to the target
	fmt.Fprintf(&reqBuf, "Host: localhost:%d\r\n", port)
	// copy selected headers
	for _, h := range []string{"Content-Type", "Content-Length", "Accept", "Accept-Encoding", "Authorization", "Cookie", "User-Agent"} {
		if v := r.Header.Get(h); v != "" {
			fmt.Fprintf(&reqBuf, "%s: %s\r\n", h, v)
		}
	}
	// add forwarding headers
	fmt.Fprintf(&reqBuf, "X-Forwarded-For: %s\r\n", s.clientIP(r))
	fmt.Fprintf(&reqBuf, "X-Forwarded-Proto: %s\r\n", schemeOf(r))
	fmt.Fprintf(&reqBuf, "X-Real-IP: %s\r\n", s.clientIP(r))
	reqBuf.WriteString("\r\n")
	// copy request body
	if r.Body != nil {
		io.Copy(&reqBuf, r.Body)
	}

	// send the request through the tunnel in chunks
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
			http.Error(w, "转发会话已断开", http.StatusBadGateway)
			return
		}
		data = data[len(chunk):]
	}
	// signal end of request
	select {
	case sess.toAgent <- forwardFrame('c', nil):
	case <-sess.done:
	}

	// read response from the agent via a pipe
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for {
			select {
			case b := <-sess.toUser:
				sess.touch()
				pw.Write(b)
			case <-sess.done:
				return
			}
		}
	}()

	resp, err := http.ReadResponse(bufio.NewReader(pr), nil)
	if err != nil {
		http.Error(w, "无法解析上游响应: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// copy response headers and body to the browser
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: operator, Host: hostname,
		Message: fmt.Sprintf("HTTP 代理 %s:%d %s %s → %d", hostname, port, r.Method, path, resp.StatusCode)})
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
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized"})
		return
	}
	host := r.URL.Query().Get("host")
	if host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host required"})
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
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session gone"})
		return
	}
	if !s.forwardFingerprintOKByHost(sess.hostID, r.URL.Query().Get("fp")) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized"})
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
			sess.close()
			return
		}
	}
}

// handleAgentForwardTx receives data from the agent (chunked request body)
// and fans it to the user connection.
func (s *Server) handleAgentForwardTx(w http.ResponseWriter, r *http.Request) {
	sess := s.forward.getSession(r.URL.Query().Get("session"))
	if sess == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session gone"})
		return
	}
	if !s.forwardFingerprintOKByHost(sess.hostID, r.URL.Query().Get("fp")) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized"})
		return
	}
	sess.markAgentUp()
	defer sess.close()
	buf := make([]byte, 16<<10)
	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			b := make([]byte, n)
			copy(b, buf[:n])
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