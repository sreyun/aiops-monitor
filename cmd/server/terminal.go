package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Remote terminal relay.
//
// The agent has no inbound ports, so it *dials out*: it long-polls `wait` for a
// session, then opens two plain-HTTP streams — `rx` (server→agent keystrokes)
// and `tx` (agent→server shell output). The operator's browser speaks WebSocket
// to the server; the server relays bytes between the browser socket and the two
// agent streams. Agent terminal endpoints are gated by the machine fingerprint
// (bound at registration), not the install token.

type termSession struct {
	id        string
	hostID    string
	hostname  string
	operator  string
	mode      string // "" = interactive terminal, "exec" = one-shot playbook command
	command   string // the command to run when mode == "exec"
	toAgent   chan []byte   // browser keystrokes → agent (rx stream)
	toBrowser chan []byte   // agent shell output → browser
	agentUp   chan struct{} // closed once the agent attaches its tx stream
	done      chan struct{}
	upOnce    sync.Once
	doneOnce  sync.Once

	// --- terminal enhancements ---
	recording []termRecordFrame // session recording (timestamped I/O frames)
	observers map[*termObserver]struct{} // read-only watchers
	cmdBuffer string           // accumulates input for command-level audit
	lastCommand string         // last extracted command (for audit logging)
	createdAt int64            // session start time
	recMu     sync.Mutex       // protects recording + cmdBuffer + observers
}

func (s *termSession) markAgentUp() { s.upOnce.Do(func() { close(s.agentUp) }) }
func (s *termSession) close() {
	s.doneOnce.Do(func() {
		close(s.done)
		s.recMu.Lock()
		for obs := range s.observers {
			close(obs.done)
		}
		s.recMu.Unlock()
	})
}

// fanOut delivers a copy of output to all observers under recMu, so it can't race
// with add/removeObserver (which also hold recMu). Best-effort: drops on a slow
// observer rather than blocking the live session.
func (s *termSession) fanOut(b []byte) {
	s.recMu.Lock()
	for obs := range s.observers {
		select {
		case obs.ch <- b:
		default:
		}
	}
	s.recMu.Unlock()
}

// termRecordFrame is one timestamped frame in the session recording.
type termRecordFrame struct {
	Ts     int64  `json:"ts"`      // unix millisecond
	Type   string `json:"type"`    // "input" | "output"
	Data   string `json:"data"`    // base64-encoded raw bytes
}

// termObserver is a read-only watcher of a terminal session.
type termObserver struct {
	ch   chan []byte // receives a copy of toBrowser output
	done chan struct{}
}

// recordFrame appends a frame to the session recording (max 50k frames).
func (s *termSession) recordFrame(typ string, data []byte) {
	s.recMu.Lock()
	defer s.recMu.Unlock()
	if len(s.recording) > 50000 {
		return // cap to prevent unbounded memory
	}
	s.recording = append(s.recording, termRecordFrame{
		Ts:   time.Now().UnixMilli(),
		Type: typ,
		Data: base64.StdEncoding.EncodeToString(data),
	})
}

type termManager struct {
	mu       sync.Mutex
	sessions map[string]*termSession
	waiters  map[string]chan string // hostID -> a waiting agent poll
	archived []termArchive          // recordings of recently-ended sessions (for replay)
}

// termArchive keeps an ended session's recording so it can still be replayed
// after the live session has been removed.
type termArchive struct {
	info      termSessionInfo
	recording []termRecordFrame
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

func (m *termManager) create(hostID, hostname, operator string) *termSession {
	return m.createFull(hostID, hostname, operator, "", "")
}

// createExec makes a one-shot session that runs a single command via the agent's
// dedicated exec path (no interactive PTY, no sentinel) — reliable across shells
// and OSes, which the interactive-terminal approach was not (esp. Linux bash).
func (m *termManager) createExec(hostID, hostname, command string) *termSession {
	return m.createFull(hostID, hostname, "playbook-exec", "exec", command)
}

func (m *termManager) createFull(hostID, hostname, operator, mode, command string) *termSession {
	s := &termSession{
		id: termID(), hostID: hostID, hostname: hostname, operator: operator,
		mode: mode, command: command,
		toAgent: make(chan []byte, 64), toBrowser: make(chan []byte, 256),
		agentUp: make(chan struct{}), done: make(chan struct{}),
		observers: map[*termObserver]struct{}{},
		createdAt: time.Now().Unix(),
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
// remove deletes a session, first archiving its recording so an ended interactive
// session can still be replayed. Internal playbook-exec sessions are not archived.
func (m *termManager) remove(id string) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	delete(m.sessions, id)
	if ok && s.operator != "playbook-exec" {
		s.recMu.Lock()
		if len(s.recording) > 0 {
			rec := make([]termRecordFrame, len(s.recording))
			copy(rec, s.recording)
			m.archived = append(m.archived, termArchive{
				info: termSessionInfo{
					ID: s.id, HostID: s.hostID, Hostname: s.hostname,
					Operator: s.operator, CreatedAt: s.createdAt,
					Active: false, Frames: len(rec),
				},
				recording: rec,
			})
			if len(m.archived) > 20 { // keep only the most recent sessions
				m.archived = m.archived[len(m.archived)-20:]
			}
		}
		s.recMu.Unlock()
	}
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

// termFingerprintOKByHost verifies the agent-presented fingerprint against the
// fingerprint bound to hostID at registration (constant-time). The terminal
// reverse channel authenticates by fingerprint, not the install token, so
// rotating the token never breaks already-installed agents' terminals.
func (s *Server) termFingerprintOKByHost(hostID, fp string) bool {
	if hostID == "" || fp == "" {
		return false
	}
	host, ok := s.store.GetHost(hostID)
	if !ok || host.Fingerprint == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(fp), []byte(host.Fingerprint)) == 1
}

// termFingerprintOK is the request-flavored wrapper for handleAgentTermWait,
// which carries host + fp as query params.
func (s *Server) termFingerprintOK(r *http.Request) bool {
	return s.termFingerprintOKByHost(r.URL.Query().Get("host"), r.URL.Query().Get("fp"))
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

	// Look up hostname for audit log
	hostname := shortID(hostID)
	for _, h := range s.store.ListHosts() {
		if h.ID == hostID {
			hostname = h.Hostname
			break
		}
	}
	sess := s.term.create(hostID, hostname, s.clientIP(r))
	defer s.term.remove(sess.id)
	op := s.clientIP(r)
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: op, Host: hostname, Message: "打开远程终端 " + hostname})
	defer s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: op, Host: hostname, Message: "关闭远程终端 " + hostname})

	if !s.term.notifyAgent(hostID, sess.id) {
		_ = ws.WriteBinary([]byte("\r\n\x1b[31m✗ 无法建立终端会话——服务端未找到该主机的反向终端通道。\x1b[0m\r\n\r\n" +
			"常见原因与处理：\r\n" +
			"  1) \x1b[33mAgent 版本过旧\x1b[0m（旧版无反向终端通道）——请在该主机\x1b[36m重新执行安装命令升级到最新 Agent\x1b[0m；\r\n" +
			"  2) Agent 启动时\x1b[33m未采集到机器指纹\x1b[0m（指纹用于终端通道鉴权，检查 machine-id / MAC 是否可读）；\r\n" +
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
			// Record input for audit + replay; log completed commands
			if typ == 'i' {
				sess.recordFrame("input", payload)
				if cmd := sess.processCommandAudit(payload); cmd != "" {
					s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: op, Host: hostname, Message: "终端命令 [" + hostname + "]: " + cmd})
				}
			}
			select {
			case sess.toAgent <- termFrame(typ, payload):
			case <-sess.done:
				return
			}
		}
	}()
	// agent → browser (shell output) + recording + observers
	go func() {
		defer sess.close()
		for {
			select {
			case b := <-sess.toBrowser:
				sess.recordFrame("output", b)
				sess.fanOut(b) // deliver to observers under lock (avoids the map race → panic)
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
	if !s.termFingerprintOK(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized"})
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
		out := map[string]string{"session": sid}
		if sess := s.term.get(sid); sess != nil && sess.mode == "exec" {
			out["mode"] = "exec"
			out["command"] = sess.command
		}
		writeJSON(w, http.StatusOK, out)
	case <-time.After(25 * time.Second):
		writeJSON(w, http.StatusOK, map[string]string{})
	case <-r.Context().Done():
	}
}

// handleAgentTermRx streams operator keystrokes down to the agent (chunked).
func (s *Server) handleAgentTermRx(w http.ResponseWriter, r *http.Request) {
	sess := s.term.get(r.URL.Query().Get("session"))
	if sess == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session gone"})
		return
	}
	if !s.termFingerprintOKByHost(sess.hostID, r.URL.Query().Get("fp")) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "unauthorized"})
		return
	}
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "application/octet-stream")
	// Tell nginx (and other X-Accel-aware proxies) NOT to buffer this response —
	// keystrokes must reach the agent in real time. Saves the operator from having
	// to set `proxy_buffering off` for this stream.
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

// handleAgentTermTx receives the shell's output stream from the agent (chunked
// request body) and fans it to the browser.
func (s *Server) handleAgentTermTx(w http.ResponseWriter, r *http.Request) {
	sess := s.term.get(r.URL.Query().Get("session"))
	if sess == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session gone"})
		return
	}
	if !s.termFingerprintOKByHost(sess.hostID, r.URL.Query().Get("fp")) {
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

// ---- terminal enhancement: session list, replay, observer, command audit ----

// termSessionInfo is the JSON view of an active or recently-ended session.
type termSessionInfo struct {
	ID        string `json:"id"`
	HostID    string `json:"host_id"`
	Hostname  string `json:"hostname"`
	Operator  string `json:"operator"`
	CreatedAt int64  `json:"created_at"`
	Active    bool   `json:"active"`
	Observers int    `json:"observers"`
	Frames    int    `json:"frames"`
}

// listSessions returns all active terminal sessions.
func (m *termManager) listSessions() []termSessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]termSessionInfo, 0, len(m.sessions)+len(m.archived))
	for _, s := range m.sessions {
		if s.operator == "playbook-exec" {
			continue // hide internal playbook sessions from the session list
		}
		s.recMu.Lock()
		out = append(out, termSessionInfo{
			ID: s.id, HostID: s.hostID, Hostname: s.hostname,
			Operator: s.operator, CreatedAt: s.createdAt,
			Active: true, Observers: len(s.observers),
			Frames: len(s.recording),
		})
		s.recMu.Unlock()
	}
	// Archived (ended) sessions, newest first.
	for i := len(m.archived) - 1; i >= 0; i-- {
		out = append(out, m.archived[i].info)
	}
	return out
}

// getRecording returns the recorded frames for a session (for replay).
func (m *termManager) getRecording(sessionID string) []termRecordFrame {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[sessionID]; ok {
		s.recMu.Lock()
		out := make([]termRecordFrame, len(s.recording))
		copy(out, s.recording)
		s.recMu.Unlock()
		return out
	}
	for _, a := range m.archived {
		if a.info.ID == sessionID {
			out := make([]termRecordFrame, len(a.recording))
			copy(out, a.recording)
			return out
		}
	}
	return nil
}

// addObserver attaches a read-only observer to a session.
func (m *termManager) addObserver(sessionID string) (*termObserver, bool) {
	m.mu.Lock()
	s, ok := m.sessions[sessionID]
	m.mu.Unlock()
	if !ok {
		return nil, false
	}
	s.recMu.Lock()
	defer s.recMu.Unlock()
	obs := &termObserver{
		ch:   make(chan []byte, 128),
		done: make(chan struct{}),
	}
	s.observers[obs] = struct{}{}
	return obs, true
}

// removeObserver detaches an observer.
func (m *termManager) removeObserver(sessionID string, obs *termObserver) {
	m.mu.Lock()
	s, ok := m.sessions[sessionID]
	m.mu.Unlock()
	if !ok {
		return
	}
	s.recMu.Lock()
	delete(s.observers, obs)
	s.recMu.Unlock()
}

// processCommandAudit extracts commands from the input stream by detecting
// Enter (CR/LF) and returning the accumulated line for audit logging.
// This is best-effort — control characters, escape sequences, and multi-line
// commands may cause imperfect extraction, but it captures the common case.
// Returns the completed command string (empty if no command was completed).
func (s *termSession) processCommandAudit(payload []byte) string {
	s.recMu.Lock()
	defer s.recMu.Unlock()
	for _, b := range payload {
		if b == '\r' || b == '\n' {
			cmd := strings.TrimSpace(s.cmdBuffer)
			s.cmdBuffer = ""
			if cmd != "" && cmd[0] != 0x1b && cmd[0] != 0x03 {
				s.lastCommand = cmd
				return cmd
			}
		} else if b >= 0x20 && b < 0x7f {
			s.cmdBuffer += string(b)
		} else if b == 0x7f || b == 0x08 {
			if len(s.cmdBuffer) > 0 {
				s.cmdBuffer = s.cmdBuffer[:len(s.cmdBuffer)-1]
			}
		}
	}
	return ""
}

// getDecodedRecording returns the decoded output frames for replay/observe.
func (m *termManager) getDecodedRecording(sessionID string) [][]byte {
	frames := m.getRecording(sessionID) // covers both live and archived sessions
	out := make([][]byte, 0, len(frames))
	for _, f := range frames {
		if f.Type == "output" {
			if data, err := base64.StdEncoding.DecodeString(f.Data); err == nil {
				out = append(out, data)
			}
		}
	}
	return out
}
