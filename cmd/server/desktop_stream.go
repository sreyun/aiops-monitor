package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Web remote desktop: browser WebSocket ↔ agent reverse channel (screen stream + input + files).
// Independent of TCP port-forward / local RDP-VNC clients.

const (
	deskIdleTimeout = 2 * time.Hour
	deskHardTimeout = 8 * time.Hour
)

type deskSession struct {
	id       string
	hostID   string
	hostname string
	operator string
	ip       string
	lang     string

	toAgent   chan []byte
	toBrowser chan []byte
	agentUp   chan struct{}
	done      chan struct{}
	upOnce    sync.Once
	doneOnce  sync.Once

	lastActive atomic.Int64 // unix nano; idle timeout
	createdAt  int64
	recording  []deskRecordFrame
	recMu      sync.Mutex
}

func (s *deskSession) markAgentUp() { s.upOnce.Do(func() { close(s.agentUp) }) }
func (s *deskSession) close() {
	s.doneOnce.Do(func() { close(s.done) })
}
func (s *deskSession) touch() { s.lastActive.Store(time.Now().UnixNano()) }

type deskManager struct {
	mu              sync.Mutex
	sessions        map[string]*deskSession
	waiters         map[string]chan string
	pendingSessions map[string][]string
	archived        []deskArchive
	recDir          string
}

func newDeskManager() *deskManager {
	return &deskManager{
		sessions:        map[string]*deskSession{},
		waiters:         map[string]chan string{},
		pendingSessions: map[string][]string{},
	}
}

func deskID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func deskFrame(typ byte, payload []byte) []byte {
	if len(payload) > 0xffff {
		payload = payload[:0xffff]
	}
	b := make([]byte, 3+len(payload))
	b[0] = typ
	binary.BigEndian.PutUint16(b[1:], uint16(len(payload)))
	copy(b[3:], payload)
	return b
}

func (m *deskManager) create(hostID, hostname, operator, ip, lang string) *deskSession {
	s := &deskSession{
		id: deskID(), hostID: hostID, hostname: hostname, operator: operator, ip: ip, lang: lang,
		toAgent: make(chan []byte, 128), toBrowser: make(chan []byte, 256),
		agentUp: make(chan struct{}), done: make(chan struct{}),
		createdAt: time.Now().Unix(),
	}
	s.touch()
	m.mu.Lock()
	m.sessions[s.id] = s
	m.mu.Unlock()
	return s
}

func (m *deskManager) get(id string) *deskSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

func (m *deskManager) remove(id string) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	delete(m.sessions, id)
	if ok {
		if q := m.pendingSessions[s.hostID]; len(q) > 0 {
			kept := make([]string, 0, len(q))
			for _, sid := range q {
				if sid != id {
					kept = append(kept, sid)
				}
			}
			if len(kept) == 0 {
				delete(m.pendingSessions, s.hostID)
			} else {
				m.pendingSessions[s.hostID] = kept
			}
		}
	}
	m.mu.Unlock()
	if ok {
		m.archiveSession(s)
		s.close()
	}
}

func (m *deskManager) notifyAgent(hostID, sessionID string) bool {
	m.mu.Lock()
	w := m.waiters[hostID]
	delete(m.waiters, hostID)
	if w == nil {
		m.pendingSessions[hostID] = append(m.pendingSessions[hostID], sessionID)
		m.mu.Unlock()
		return true
	}
	m.mu.Unlock()
	select {
	case w <- sessionID:
		return true
	default:
		m.mu.Lock()
		m.pendingSessions[hostID] = append(m.pendingSessions[hostID], sessionID)
		m.mu.Unlock()
		return true
	}
}

func (m *deskManager) registerWaiter(hostID string) chan string {
	ch := make(chan string, 1)
	m.mu.Lock()
	m.waiters[hostID] = ch
	m.mu.Unlock()
	return ch
}

func (m *deskManager) unregisterWaiter(hostID string, ch chan string) {
	m.mu.Lock()
	if m.waiters[hostID] == ch {
		delete(m.waiters, hostID)
	}
	m.mu.Unlock()
}

// handleOpenDesktop preflight: secondary auth + host online; browser then opens WS.
// POST /api/v1/hosts/{id}/desktop
func (s *Server) handleOpenDesktop(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.TerminalEnabled() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "desktop.disabled")})
		return
	}
	verified, hasPassword := s.auth.isTerminalVerified(r)
	if !verified {
		code := "terminal_verify_required"
		if !hasPassword {
			code = "terminal_password_not_set"
		}
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": Tr(r, "terminal_auth."+code),
			"code":  code,
		})
		return
	}
	hostID := r.PathValue("id")
	h := s.hostByID(hostID)
	if h == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "common.host_not_found")})
		return
	}
	offlineSec := int64(s.cfg.Thresholds().OfflineAfter.Seconds())
	if time.Now().Unix()-h.LastSeen > offlineSec {
		writeJSON(w, http.StatusConflict, map[string]string{"error": Tr(r, "desktop.host_offline")})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":       "web",
		"ws_path":    "/api/v1/hosts/" + hostID + "/desktop/ws",
		"host_id":    hostID,
		"hostname":   h.Hostname,
		"os":         h.OS,
		"platform":   h.Platform,
		"supported":  true,
		"file_xfer":  true,
		"idle_hours": int(deskIdleTimeout.Hours()),
	})
}

// handleDesktopWS upgrades to WebSocket and relays screen/input/file frames.
// GET /api/v1/hosts/{id}/desktop/ws
//
// Session lifetime mirrors the terminal channel:
//   - browser disconnect closes the session
//   - agent TX end closes after a short drain so the last error/meta frames
//     still reach the browser (avoids "已断开" wiping the real cause)
//   - idle / hard timeout / agent-wait timeout also close
func (s *Server) handleDesktopWS(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.TerminalEnabled() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "desktop.disabled")})
		return
	}
	verified, hasPassword := s.auth.isTerminalVerified(r)
	if !verified {
		code := "terminal_verify_required"
		if !hasPassword {
			code = "terminal_password_not_set"
		}
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": Tr(r, "terminal_auth."+code),
			"code":  code,
		})
		return
	}
	hostID := r.PathValue("id")
	h := s.hostByID(hostID)
	if h == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "common.host_not_found")})
		return
	}
	ws, err := wsAccept(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "terminal.ws_required")})
		return
	}
	defer ws.Close()

	operator, clientIP := s.actorIP(r)
	sess := s.desk.create(hostID, h.Hostname, operator, clientIP, langFromRequest(r))
	defer s.desk.remove(sess.id)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: operator, IP: clientIP, Host: h.Hostname, Message: Tz("log.open_desktop", h.Hostname)})
	defer s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: operator, IP: clientIP, Host: h.Hostname, Message: Tz("log.close_desktop", h.Hostname)})

	if !s.desk.notifyAgent(hostID, sess.id) {
		_ = ws.WriteBinary(append([]byte{'E'}, mustJSON(map[string]string{"error": Tz("desktop.no_channel")})...))
		return
	}
	// Queue "waiting" before any agent frames so the UI does not flash "connected".
	select {
	case sess.toBrowser <- append([]byte{'S'}, mustJSON(map[string]any{"phase": "waiting_agent", "w": 1280, "h": 720})...):
	default:
	}
	go func() {
		select {
		case <-sess.agentUp:
			select {
			case sess.toBrowser <- append([]byte{'S'}, mustJSON(map[string]any{"phase": "agent_up"})...):
			case <-sess.done:
			}
		case <-time.After(35 * time.Second):
			select {
			case sess.toBrowser <- append([]byte{'E'}, mustJSON(map[string]string{"error": Tz("desktop.timeout")})...):
			case <-sess.done:
			}
			// Let the writer flush the error frame before tearing the socket down.
			time.Sleep(300 * time.Millisecond)
			sess.close()
		case <-sess.done:
		}
	}()

	// Idle / hard timeout watchdog
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		deadline := time.Now().Add(deskHardTimeout)
		for {
			select {
			case <-t.C:
				last := time.Unix(0, sess.lastActive.Load())
				if time.Since(last) > deskIdleTimeout || time.Now().After(deadline) {
					select {
					case sess.toBrowser <- append([]byte{'E'}, mustJSON(map[string]string{"error": Tz("desktop.timeout")})...):
					default:
					}
					time.Sleep(200 * time.Millisecond)
					sess.close()
					return
				}
			case <-sess.done:
				return
			}
		}
	}()

	// browser → agent
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
			sess.touch()
			typ := data[0]
			payload := data[1:]
			switch typ {
			case 'P': // app-level ping → pong
				_ = ws.WriteBinary([]byte{'P'})
				continue
			case 'M', 'W', 'B', 'Q', 'N', 'C', 'f', 'u', 'e', 'd':
				// framed to agent below
			default:
				continue
			}
			for {
				chunk := payload
				if len(chunk) > 0xffff {
					chunk = chunk[:0xffff]
				}
				select {
				case sess.toAgent <- deskFrame(typ, chunk):
				case <-sess.done:
					return
				}
				if len(payload) <= 0xffff {
					break
				}
				payload = payload[0xffff:]
			}
		}
	}()

	// agent → browser. Drain remaining frames when done so error/meta is not lost.
	go func() {
		write := func(b []byte) bool {
			sess.touch()
			if len(b) > 0 {
				switch b[0] {
				case 'S':
					sess.recordFrame("meta", b[1:])
				case 'K':
					if time.Now().UnixMilli()%500 < 120 {
						sess.recordFrame("jpeg", b[1:])
					}
				case 'H':
					sess.recordFrame("h264", b[1:])
				}
			}
			return ws.WriteBinary(b) == nil
		}
		for {
			select {
			case b := <-sess.toBrowser:
				if !write(b) {
					sess.close()
					return
				}
			case <-sess.done:
				for {
					select {
					case b := <-sess.toBrowser:
						_ = write(b)
					default:
						return
					}
				}
			}
		}
	}()

	go func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := ws.WritePing(nil); err != nil {
					sess.close()
					return
				}
			case <-sess.done:
				return
			}
		}
	}()
	<-sess.done
}

func (s *Server) handleAgentDeskWait(w http.ResponseWriter, r *http.Request) {
	if !s.termFingerprintOK(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	host := r.URL.Query().Get("host")
	if host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.host_required")})
		return
	}
	s.desk.mu.Lock()
	if pending := s.desk.pendingSessions[host]; len(pending) > 0 {
		var sid string
		rest := pending
		for len(rest) > 0 {
			cand := rest[0]
			rest = rest[1:]
			if _, live := s.desk.sessions[cand]; live {
				sid = cand
				break
			}
		}
		if len(rest) == 0 {
			delete(s.desk.pendingSessions, host)
		} else {
			s.desk.pendingSessions[host] = rest
		}
		if sid != "" {
			sess := s.desk.sessions[sid]
			s.desk.mu.Unlock()
			out := map[string]string{"session": sid}
			if sess != nil && sess.lang != "" {
				out["lang"] = sess.lang
			}
			writeJSON(w, http.StatusOK, out)
			return
		}
	}
	s.desk.mu.Unlock()

	ch := s.desk.registerWaiter(host)
	defer s.desk.unregisterWaiter(host, ch)
	select {
	case sid := <-ch:
		if sess := s.desk.get(sid); sess != nil {
			out := map[string]string{"session": sid}
			if sess.lang != "" {
				out["lang"] = sess.lang
			}
			writeJSON(w, http.StatusOK, out)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{})
	case <-time.After(25 * time.Second):
		writeJSON(w, http.StatusOK, map[string]string{})
	case <-r.Context().Done():
	}
}

func (s *Server) handleAgentDeskRx(w http.ResponseWriter, r *http.Request) {
	sess := s.desk.get(r.URL.Query().Get("session"))
	if sess == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "common.session_gone")})
		return
	}
	if !s.termFingerprintOKByHost(sess.hostID, agentFP(r)) {
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
			sess.close()
			return
		}
	}
}

// Agent tx frames: [type:1][len:4 BE][payload] — relayed to browser as [type][payload].
// Unlike a naive "defer sess.close()", we give the browser writer a short grace
// period to flush the last meta/error/jpeg frames. Otherwise a one-shot
// deskSendError TX races with sess.done and the UI only sees "已断开".
func (s *Server) handleAgentDeskTx(w http.ResponseWriter, r *http.Request) {
	sess := s.desk.get(r.URL.Query().Get("session"))
	if sess == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "common.session_gone")})
		return
	}
	if !s.termFingerprintOKByHost(sess.hostID, agentFP(r)) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	sess.markAgentUp()
	defer func() {
		deadline := time.Now().Add(1500 * time.Millisecond)
		for time.Now().Before(deadline) {
			if len(sess.toBrowser) == 0 {
				time.Sleep(50 * time.Millisecond)
				if len(sess.toBrowser) == 0 {
					break
				}
				continue
			}
			time.Sleep(20 * time.Millisecond)
		}
		sess.close()
	}()

	var hdr [5]byte
	for {
		if _, err := io.ReadFull(r.Body, hdr[:]); err != nil {
			return
		}
		typ := hdr[0]
		payloadLen := int(binary.BigEndian.Uint32(hdr[1:]))
		if payloadLen > 16<<20 {
			return
		}
		payload := make([]byte, payloadLen)
		if payloadLen > 0 {
			if _, err := io.ReadFull(r.Body, payload); err != nil {
				return
			}
		}
		out := make([]byte, 1+len(payload))
		out[0] = typ
		copy(out[1:], payload)
		if !sess.enqueueBrowser(out) {
			return
		}
	}
}

// enqueueBrowser forwards an agent frame to the browser writer.
//
// Video frames ('K' JPEG / 'H' H264) are lossy: if the browser is slow and the
// queue is full we prefer the newest frame, but we ONLY ever drop a *stale video*
// frame to make room. Control/meta/error frames ('S','E','C','F','D',…) are never
// evicted — dropping them raced with deskSendError and left the UI with a bare
// WebSocket close ("已断开") instead of the real cause.
//
// Returns false only when the session is done (caller should stop relaying).
func (s *deskSession) enqueueBrowser(out []byte) bool {
	isVideo := len(out) > 0 && (out[0] == 'K' || out[0] == 'H')
	if !isVideo {
		// Control frames block until delivered (or session ends).
		select {
		case s.toBrowser <- out:
			return true
		case <-s.done:
			return false
		}
	}
	select {
	case s.toBrowser <- out:
		return true
	case <-s.done:
		return false
	default:
	}
	// Queue full: make room by discarding at most one *stale video* frame.
	select {
	case old := <-s.toBrowser:
		if len(old) > 0 && (old[0] == 'K' || old[0] == 'H') {
			select {
			case s.toBrowser <- out:
			case <-s.done:
				return false
			default:
			}
		} else {
			// Head was a control frame — put it back, drop the incoming video.
			select {
			case s.toBrowser <- old:
			case <-s.done:
				return false
			}
		}
	default:
	}
	return true
}
