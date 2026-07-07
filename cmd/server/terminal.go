package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// Remote terminal relay.
//
// The agent has no inbound ports, so it *dials out*: it long-polls `wait` for a
// session, then opens two plain-HTTP streams — `rx` (server→agent keystrokes)
// and `tx` (agent→server shell output). The operator's browser speaks WebSocket
// to the server; the server relays bytes between the browser socket and the two
// agent streams. Agent terminal endpoints are gated by the install token.

type termSession struct {
	id        string
	hostID    string
	toAgent   chan []byte   // browser keystrokes → agent (rx stream)
	toBrowser chan []byte   // agent shell output → browser
	agentUp   chan struct{} // closed once the agent attaches its tx stream
	done      chan struct{}
	upOnce    sync.Once
	doneOnce  sync.Once
}

func (s *termSession) markAgentUp() { s.upOnce.Do(func() { close(s.agentUp) }) }
func (s *termSession) close()       { s.doneOnce.Do(func() { close(s.done) }) }

type termManager struct {
	mu       sync.Mutex
	sessions map[string]*termSession
	waiters  map[string]chan string // hostID -> a waiting agent poll
}

func newTermManager() *termManager {
	return &termManager{sessions: map[string]*termSession{}, waiters: map[string]chan string{}}
}

func termID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// termFrame encodes one rx message as [type:1][len:2 BE][payload] so the agent
// can parse input vs resize from the raw stream.
func termFrame(typ byte, payload []byte) []byte {
	if len(payload) > 0xffff {
		payload = payload[:0xffff]
	}
	b := make([]byte, 3+len(payload))
	b[0] = typ
	binary.BigEndian.PutUint16(b[1:], uint16(len(payload)))
	copy(b[3:], payload)
	return b
}

func (m *termManager) create(hostID string) *termSession {
	s := &termSession{
		id: termID(), hostID: hostID,
		toAgent: make(chan []byte, 64), toBrowser: make(chan []byte, 256),
		agentUp: make(chan struct{}), done: make(chan struct{}),
	}
	m.mu.Lock()
	m.sessions[s.id] = s
	m.mu.Unlock()
	return s
}
func (m *termManager) get(id string) *termSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}
func (m *termManager) remove(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

// notifyAgent hands a new sessionID to the agent currently long-polling for
// hostID; returns false if none is waiting.
func (m *termManager) notifyAgent(hostID, sessionID string) bool {
	m.mu.Lock()
	w := m.waiters[hostID]
	delete(m.waiters, hostID)
	m.mu.Unlock()
	if w == nil {
		return false
	}
	select {
	case w <- sessionID:
		return true
	default:
		return false
	}
}
func (m *termManager) registerWaiter(hostID string) chan string {
	ch := make(chan string, 1)
	m.mu.Lock()
	m.waiters[hostID] = ch
	m.mu.Unlock()
	return ch
}
func (m *termManager) unregisterWaiter(hostID string, ch chan string) {
	m.mu.Lock()
	if m.waiters[hostID] == ch {
		delete(m.waiters, hostID)
	}
	m.mu.Unlock()
}

// termTokenOK gates the agent-facing terminal endpoints on the install token
// (constant-time compare), independent of the optional require_token setting.
func (s *Server) termTokenOK(r *http.Request) bool {
	want := s.cfg.InstallToken()
	got := r.URL.Query().Get("token")
	return want != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// handleTerminal (browser side) upgrades to WebSocket and relays a shell session.
// Auth is enforced by authMiddleware (session cookie) before we get here.
func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("id")
	if !s.cfg.TerminalEnabled() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "远程终端已被管理员禁用"})
		return
	}
	ws, err := wsAccept(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "需要 WebSocket 升级"})
		return
	}
	defer ws.Close()

	sess := s.term.create(hostID)
	defer s.term.remove(sess.id)
	op := clientIP(r)
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: op, Host: shortID(hostID), Message: "打开远程终端 " + shortID(hostID)})
	defer s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: op, Host: shortID(hostID), Message: "关闭远程终端 " + shortID(hostID)})

	if !s.term.notifyAgent(hostID, sess.id) {
		_ = ws.WriteBinary([]byte("\r\n\x1b[31m✗ 无法建立终端会话——服务端未找到该主机的反向终端通道。\x1b[0m\r\n\r\n" +
			"常见原因与处理：\r\n" +
			"  1) \x1b[33mAgent 版本过旧\x1b[0m（旧版无反向终端通道）——请在该主机\x1b[36m重新执行安装命令升级到最新 Agent\x1b[0m；\r\n" +
			"  2) Agent 启动时\x1b[33m未携带正确的 --token\x1b[0m（现已强制校验）；\r\n" +
			"  3) 主机刚离线或 Agent 未运行。\r\n\r\n" +
			"升级后重新打开终端即可。\r\n"))
		return
	}
	// Watchdog: if the agent never attaches, don't hang the operator forever.
	go func() {
		select {
		case <-sess.agentUp:
		case <-time.After(10 * time.Second):
			_ = ws.WriteBinary([]byte("\r\n\x1b[31mAgent 未在预期时间内接入终端通道，已断开。\x1b[0m\r\n"))
			sess.close()
		case <-sess.done:
		}
	}()

	// browser → agent. The browser tags each WS message: byte 0 'i' = input,
	// 'r' = resize ("colsxrows"). We re-encode it as a self-delimiting frame
	// ([type:1][len:2 BE][payload]) so the agent can demultiplex input vs resize
	// off the raw rx byte stream regardless of HTTP chunk boundaries.
	go func() {
		defer sess.close()
		for {
			data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if len(data) == 0 {
				continue
			}
			typ, payload := byte('i'), data
			switch data[0] {
			case 'r':
				typ, payload = 'r', data[1:]
			case 'i':
				typ, payload = 'i', data[1:]
			}
			if len(payload) == 0 {
				continue
			}
			select {
			case sess.toAgent <- termFrame(typ, payload):
			case <-sess.done:
				return
			}
		}
	}()
	// agent → browser (shell output)
	go func() {
		defer sess.close()
		for {
			select {
			case b := <-sess.toBrowser:
				if err := ws.WriteBinary(b); err != nil {
					return
				}
			case <-sess.done:
				return
			}
		}
	}()
	<-sess.done
}

// handleAgentTermWait: agent long-polls here; returns a session id when the
// operator opens a terminal for this host, or {} on timeout (agent re-polls).
func (s *Server) handleAgentTermWait(w http.ResponseWriter, r *http.Request) {
	if !s.termTokenOK(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid token"})
		return
	}
	host := r.URL.Query().Get("host")
	if host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host required"})
		return
	}
	ch := s.term.registerWaiter(host)
	defer s.term.unregisterWaiter(host, ch)
	select {
	case sid := <-ch:
		writeJSON(w, http.StatusOK, map[string]string{"session": sid})
	case <-time.After(25 * time.Second):
		writeJSON(w, http.StatusOK, map[string]string{})
	case <-r.Context().Done():
	}
}

// handleAgentTermRx streams operator keystrokes down to the agent (chunked).
func (s *Server) handleAgentTermRx(w http.ResponseWriter, r *http.Request) {
	if !s.termTokenOK(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid token"})
		return
	}
	sess := s.term.get(r.URL.Query().Get("session"))
	if sess == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session gone"})
		return
	}
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "application/octet-stream")
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

// handleAgentTermTx receives the shell's output stream from the agent (chunked
// request body) and fans it to the browser.
func (s *Server) handleAgentTermTx(w http.ResponseWriter, r *http.Request) {
	if !s.termTokenOK(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid token"})
		return
	}
	sess := s.term.get(r.URL.Query().Get("session"))
	if sess == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session gone"})
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
			case sess.toBrowser <- b:
			case <-sess.done:
				return
			}
		}
		if err != nil {
			return
		}
	}
}
