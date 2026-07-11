package main

import "net/http"

// -----------------------------------------------------------------------
// Terminal enhancement handlers
// -----------------------------------------------------------------------

func (s *Server) handleListTerminalSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.term.listSessions())
}

func (s *Server) handleTerminalReplay(w http.ResponseWriter, r *http.Request) {
	// Replays contain the full shell I/O of a past session (potentially secrets
	// typed by another user), so require the same terminal secondary verification
	// the live shell enforces — not just the operator role.
	if verified, _ := s.auth.isTerminalVerified(r); !verified {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "terminal_auth.terminal_verify_required"), "code": "terminal_verify_required"})
		return
	}
	sid := r.PathValue("id")
	frames := s.term.getRecording(sid)
	if frames == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "terminal.session_not_found")})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"frames": frames, "count": len(frames)})
}

// handleTerminalObserve allows a second logged-in user to watch a live terminal
// session in read-only mode via WebSocket.
func (s *Server) handleTerminalObserve(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("id")
	if !s.cfg.TerminalEnabled() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "terminal.disabled")})
		return
	}
	// Observing streams another user's live shell output — gate on the terminal
	// secondary verification, same as opening a shell.
	if verified, _ := s.auth.isTerminalVerified(r); !verified {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "terminal_auth.terminal_verify_required"), "code": "terminal_verify_required"})
		return
	}
	obs, ok := s.term.addObserver(sid)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "terminal.session_not_found")})
		return
	}
	defer s.term.removeObserver(sid, obs)
	ws, err := wsAccept(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "terminal.ws_required")})
		return
	}
	defer ws.Close()
	// Record audit log with actual logged-in username
	user, _ := s.currentUser(r)
	actor := user.Username
	if actor == "" {
		actor = s.clientIP(r)
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: actor, Message: Tz("log.observe_terminal", sid[:8])})
	// Send recorded history first so the observer sees the full context
	for _, data := range s.term.getDecodedRecording(sid) {
		if err := ws.WriteBinary(data); err != nil {
			return
		}
	}
	// Then stream live output
	for {
		select {
		case b := <-obs.ch:
			if err := ws.WriteBinary(b); err != nil {
				return
			}
		case <-obs.done:
			return
		}
	}
}
