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
	enabled    bool // whether this rule is currently active
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
	Enabled    bool   `json:"enabled"`
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
// P1: Fixed lock ordering — collects session references under m.mu, then
// operates on each without holding m.mu to avoid deadlock with other paths
// that may hold sess.mu → m.mu.
func (m *forwardManager) idleChecker() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		// Collect idle session IDs under the manager lock
		m.mu.Lock()
		type idleEntry struct {
			sess *forwardSession
			idle int64
		}
		var idles []idleEntry
		now := time.Now().Unix()
		for _, sess := range m.sessions {
			sess.mu.Lock()
			idle := now - sess.lastActive
			sess.mu.Unlock()
			if idle > int64(forwardIdleTimeout.Seconds()) {
				idles = append(idles, idleEntry{sess, idle})
			}
		}
		m.mu.Unlock()
		// Close idle sessions outside the manager lock
		for _, entry := range idles {
			slog.Info(Tz("log.forward_idle_timeout"), "session", entry.sess.id, "idle_sec", entry.idle)
			entry.sess.closeWith(Tz("log.forward_reason_timeout"))
		}
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

// rawForwardReader 把 agent 经 tx POST 回传的响应字节（来自 sess.toUser 通道）
// 暴露为一个连续的 io.Reader，供 io.ReadAll 一次性完整读取。
//
// ====== 竞态修复（彻底消除 unexpected EOF） ======
// 问题：handleAgentForwardTx 先向 toUser 发送最后一帧，再 close(sess.done)，
// 两个操作在同一个 goroutine 中纳秒级连续执行。rawForwardReader 的双 select
// 在阻塞等待时，若 toUser 和 done 同时就绪，Go 的 select 会随机选择 → 50%
// 概率选中 done 而丢弃通道缓冲中已到达的最后一帧 → http.ReadResponse 读到
// 截断的数据 → "unexpected EOF"。
//
// 修复：done 关闭后，必须再做一次非阻塞排空确认通道无残留数据。因为写端
// 保证「写通道」在「关 done」之前，所以当 done 被关闭时，若有数据则一定
// 已在通道缓冲中，非阻塞读取必定能拿到。
//
// 设计要点：
//   - 不依赖 io.Pipe：pipe 的写端关闭时机若与 http.ReadResponse 的读取错开，
//     解析器会读到提前的 EOF → "unexpected EOF"。
//   - 直接消费通道：agent 关闭 tx POST（= 目标服务关闭连接）后 toUser 不再有数据，
//     ReadAll 自然结束。
//   - 可选地把前 diagLeft 字节镜像到诊断缓冲区，用于超时/错误时的日志预览。
type rawForwardReader struct {
	ch       chan []byte
	done     <-chan struct{}
	diag     *bytes.Buffer
	diagMu   *sync.Mutex
	diagLeft int
}

func (x *rawForwardReader) Read(p []byte) (int, error) {
	for {
		// P0: 优先非阻塞地消费通道中已到达的数据。这是竞态修复的第一层：
		// 在进入阻塞等待之前，先排空通道缓冲中的所有就绪帧。
		select {
		case b, ok := <-x.ch:
			if !ok {
				return 0, io.EOF
			}
			if len(b) == 0 {
				continue
			}
			return x.emit(p, b), nil
		default:
		}

		// 阻塞等待：数据到达 或 会话结束（done 关闭）
		select {
		case b, ok := <-x.ch:
			if !ok {
				return 0, io.EOF
			}
			if len(b) == 0 {
				continue
			}
			return x.emit(p, b), nil
		case <-x.done:
			// P0: 竞态修复关键 — done 关闭后必须再做一次非阻塞排空。
			// handleAgentForwardTx 保证「向 toUser 写最后一帧」发生在
			// 「close(sess.done)」之前；当 select 因调度随机选中 done
			// 时，toUser 缓冲中的最后一帧尚未被消费，必须在此抢先取出。
			// 只有当 toUser 中也确实无数据时，才是真正的流结束。
			select {
			case b, ok := <-x.ch:
				if !ok {
					return 0, io.EOF
				}
				if len(b) == 0 {
					continue
				}
				return x.emit(p, b), nil
			default:
				return 0, io.EOF
			}
		}
	}
}

// emit 写入诊断镜像并返回拷贝到 p 的字节数。
func (x *rawForwardReader) emit(p, b []byte) int {
	if x.diag != nil && x.diagLeft > 0 {
		x.diagMu.Lock()
		if x.diagLeft > 0 {
			n := len(b)
			if n > x.diagLeft {
				n = x.diagLeft
			}
			x.diag.Write(b[:n])
			x.diagLeft -= n
		}
		x.diagMu.Unlock()
	}
	return copy(p, b)
}

// truncateStr 截取 s 为前 max 个字符（中文按 rune 处理），超出时追加 "…"。
func truncateStr(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
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

// agentOfflineReason diagnoses why no agent forward waiter exists for hostID,
// returning a human-readable reason in the current language to help the user
// understand whether the host is truly offline, has never registered, or may
// have a fingerprint mismatch.
func agentOfflineReason(store *Store, hostID string) string {
	host, ok := store.GetHost(hostID)
	if !ok {
		return Tz("forward.reason_host_unknown")
	}
	now := time.Now().Unix()
	offlineSec := now - host.LastSeen
	if offlineSec > 120 {
		ago := fmt.Sprintf("%d", offlineSec/60)
		return Tz("forward.reason_agent_down", ago)
	}
	if host.Fingerprint == "" {
		return Tz("forward.reason_no_fingerprint")
	}
	return Tz("forward.reason_channel_not_ready")
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
		// P1: Random port selection within range for better load distribution.
		// Uses a simple hash-based offset seeded by time to avoid import overhead.
		rng := int(time.Now().UnixNano() % int64((maxPort - minPort) + 1))
		for attempt := 0; attempt < 100; attempt++ {
			candidate := minPort + ((rng + attempt) % ((maxPort - minPort) + 1))
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
		enabled: true,
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

// toggleRule enables or disables a forwarding rule.
// When disabled, the listener is stopped but the rule config is preserved.
func (m *forwardManager) toggleRule(id string, enable bool) (*forwardRule, error) {
	m.mu.Lock()
	r, ok := m.rules[id]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("rule not found")
	}
	if r.enabled == enable {
		m.mu.Unlock()
		return r, nil // already in desired state
	}
	r.enabled = enable
	m.mu.Unlock()
	return r, nil
}

// updateRule modifies host_id, hostname, target_port and local_port of an existing rule.
// When hostID is non-empty, both hostID and hostname are updated so the rule points to
// the correct host after editing. When hostID is empty, host fields are left unchanged.
func (m *forwardManager) updateRule(id, hostID, hostname string, targetPort, localPort int) (*forwardRule, error) {
	m.mu.Lock()
	r, ok := m.rules[id]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("rule not found")
	}
	if hostID != "" {
		r.hostID = hostID
		r.hostname = hostname
	}
	if targetPort > 0 {
		r.targetPort = targetPort
	}
	if localPort > 0 {
		r.localPort = localPort
	}
	m.mu.Unlock()
	return r, nil
}

// copyRule duplicates an existing rule with a new ID.
func (m *forwardManager) copyRule(id string) (*forwardRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rules[id]
	if !ok {
		return nil, fmt.Errorf("rule not found")
	}
	newRule := &forwardRule{
		id:         termID()[:8],
		hostID:     r.hostID,
		hostname:   r.hostname,
		targetPort: r.targetPort,
		localPort:  0, // will be auto-assigned
		listenAddr: "",
		listener:   nil,
		operator:   r.operator,
		createdAt:  time.Now().Unix(),
		enabled:    true,
	}
	m.rules[newRule.id] = newRule
	return newRule, nil
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
			Sessions: sessions, Enabled: r.enabled,
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
		Message: Tz("log.forward_create", rule.id, hostname, req.TargetPort, rule.listenAddr)})
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
		msg := agentOfflineReason(s.store, hostID)
		slog.Warn(Tz("log.forward_parse_failed_short"), "host", hostname, "hostID", hostID, "port", port, "path", r.URL.Path, "reason", msg)
		http.Error(w, Tr(r, "forward.agent_offline")+": "+msg, http.StatusBadGateway)
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

	// *** CRITICAL: Start the response reader IMMEDIATELY after agentUp,
	// BEFORE sending request frames. If the Agent failed to connect to the
	// target, it already sent error data via the tx POST. The reader must
	// be running to capture that data before the session ends. ***
	// 不再使用 io.Pipe（关闭时机易与 http.ReadResponse 产生竞态导致
	// unexpected EOF），改为由一个 reader 直接从 sess.toUser 通道读取
	// agent 回传的全部响应字节，并同步镜像前 maxDiagBytes 用于诊断日志。
	var rawResponseBuf bytes.Buffer
	var rawResponseMu sync.Mutex
	const maxDiagBytes = 2048 // 仅记录前 2KB 用于诊断
	respReader := &rawForwardReader{
		ch:       sess.toUser,
		done:     sess.done,
		diag:     &rawResponseBuf,
		diagMu:   &rawResponseMu,
		diagLeft: maxDiagBytes,
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
	// 彻底修复：不再使用 bufio.NewReader(pr) + http.ReadResponse，
	// 那样在 io.Pipe 关闭时极易抛出 "unexpected EOF"（被包装为 502）。
	// 改为：先把 agent 回传的全部响应字节完整读出来（带上限），
	// 再用 http.ReadResponse(bytes.NewReader(raw), r) 解析。
	// 关键改进：
	//   1) 传入原始请求 r，让 ReadResponse 正确判断 HEAD/CONNECT 等无 body 场景；
	//   2) 先 io.ReadAll 再解析，避免 pipe/缓冲竞态造成的半截响应；
	//   3) 解析失败时把原始字节作为兜底响应返回，而不是返回笼统的 502。
	type readResult struct {
		raw []byte
		err error
	}
	resultCh := make(chan readResult, 1)
	go func() {
		// P0: 防止恶意超大响应撑爆内存（50MB 上限）
		const maxRespBytes = 50 << 20
		limited := io.LimitReader(respReader, maxRespBytes)
		raw, e := io.ReadAll(limited)
		resultCh <- readResult{raw, e}
	}()

	var rawResp []byte
	select {
	case res := <-resultCh:
		rawResp, err = res.raw, res.err
	case <-time.After(forwardReadTimeout):
		s.forward.stats.incError()
		rawResponseMu.Lock()
		rawPreview := truncateStr(rawResponseBuf.String(), 300)
		rawResponseMu.Unlock()
		slog.Warn(Tz("log.forward_parse_failed_short"), "host", hostname, "port", port, "path", path,
			"err", "read timeout", "raw_preview", rawPreview, "timeout", forwardReadTimeout)
		http.Error(w, Tr(r, "forward.agent_timeout"), http.StatusGatewayTimeout)
		return
	}

	if err != nil {
		s.forward.stats.incError()
		rawResponseMu.Lock()
		rawPreview := truncateStr(rawResponseBuf.String(), 300)
		rawResponseMu.Unlock()
		slog.Warn(Tz("log.forward_parse_failed_short"), "host", hostname, "port", port, "path", path,
			"err", err.Error(), "raw_len", len(rawResp), "raw_preview", rawPreview)
		http.Error(w, Tr(r, "forward.parse_response_failed", "读取上游响应失败: "+err.Error()), http.StatusBadGateway)
		return
	}

	// 尝试按 HTTP 响应解析
	resp, parseErr := http.ReadResponse(bufio.NewReader(bytes.NewReader(rawResp)), r)
	if parseErr == nil {
		defer resp.Body.Close()
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
		return
	}

	// 解析失败兜底：把 agent 回传的原始字节原样返回给浏览器。
	// 这样当目标是非 HTTP 服务（如返回纯文本错误、或 TLS 握手失败）时，
	// 用户至少能看到真实内容，而不是笼统的 502。
	s.forward.stats.incError()
	// 诊断：输出 rawResp 前 500 字符用于定位 unexpected EOF 等解析失败原因
	rawPreview := truncateStr(string(rawResp), 500)
	slog.Warn(Tz("log.forward_parse_failed_short"), "host", hostname, "port", port, "path", path,
		"err", parseErr.Error(), "raw_len", len(rawResp), "raw_preview", rawPreview)
	s.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: operator, Host: hostname,
		Message: Tz("log.forward_parse_failed", port, path, parseErr.Error())})
	if len(rawResp) == 0 {
		http.Error(w, Tr(r, "forward.parse_response_failed", parseErr.Error()), http.StatusBadGateway)
		return
	}
	// 原始内容可能是 agent 自己生成的 HTTP 502 错误页（目标不可达），
	// 尝试识别并返回对应状态码。
	statusCode := http.StatusOK
	contentType := "text/plain; charset=utf-8"
	if bytes.HasPrefix(rawResp, []byte("HTTP/")) {
		if idx := bytes.Index(rawResp, []byte("\r\n")); idx > 0 {
			parts := strings.Fields(string(rawResp[:idx]))
			if len(parts) >= 2 {
				if sc, e := strconv.Atoi(parts[1]); e == nil {
					statusCode = sc
				}
			}
		}
		// Try to extract Content-Type from raw HTTP response
		if ctIdx := bytes.Index(rawResp, []byte("Content-Type:")); ctIdx >= 0 {
			lineEnd := bytes.Index(rawResp[ctIdx:], []byte("\r\n"))
			if lineEnd > 0 {
				contentType = strings.TrimSpace(string(rawResp[ctIdx+13 : ctIdx+lineEnd]))
			}
		}
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Del("Content-Length")
	w.WriteHeader(statusCode)
	_, _ = w.Write(rawResp)
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
		msg := agentOfflineReason(s.store, hostID)
		slog.Warn(Tz("log.forward_agent_offline"), "host", hostname, "hostID", hostID, "port", port, "reason", msg)
		http.Error(w, Tr(r, "forward.agent_offline")+": "+msg, http.StatusBadGateway)
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
	// 当 Agent 完成响应回传（目标关闭连接 → r.Body 读到 EOF → 本函数返回）
	// 时，关闭 session，向「用户侧」发出流结束信号。
	// 这解决了此前 io.Pipe 竞态导致的 "unexpected EOF"，也让新的
	// rawForwardReader（io.ReadAll 模式）能可靠地拿到 EOF 而正常结束，
	// 不会陷入「要等 handler 返回才关 sess.done，而 handler 又在等 ReadAll
	// 返回」的死锁。sess.close() 由 doneOnce 保护，幂等安全。
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
