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
		toAgent: make(chan []byte, 128), toBrowser: make(chan []byte, 64),
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
		_ = ws.WriteBinary([]byte("E" + Tz("desktop.no_channel")))
		return
	}
	go func() {
		select {
		case <-sess.agentUp:
		case <-time.After(35 * time.Second):
			msg, _ := json.Marshal(map[string]string{"error": Tz("desktop.timeout")})
			_ = ws.WriteBinary(append([]byte{'E'}, msg...))
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
			case 'P': // ping → pong to browser
				_ = ws.WriteBinary([]byte{'P'})
				continue
				case 'M', 'W', 'B', 'Q', 'N', 'C', 'f', 'u', 'e', 'd':
				// framed to agent
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

	// agent → browser (already browser-ready: [type:1][payload])
	go func() {
		defer sess.close()
		for {
			select {
			case b := <-sess.toBrowser:
				sess.touch()
				if len(b) > 0 {
					switch b[0] {
					case 'S':
						sess.recordFrame("meta", b[1:])
					case 'K':
						// sample ~2 fps into recording to bound size
						if time.Now().UnixMilli()%500 < 120 {
							sess.recordFrame("jpeg", b[1:])
						}
					case 'H':
						sess.recordFrame("h264", b[1:])
					}
				}
				if err := ws.WriteBinary(b); err != nil {
					return
				}
			case <-sess.done:
				return
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
	defer sess.close()

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
		select {
		case sess.toBrowser <- out:
		case <-sess.done:
			return
		}
	}
}
