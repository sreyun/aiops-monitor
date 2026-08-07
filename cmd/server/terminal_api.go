package main

import "net/http"

// -----------------------------------------------------------------------
// Terminal enhancement handlers
// -----------------------------------------------------------------------

func (s *Server) handleListTerminalSessions(w http.ResponseWriter, r *http.Request) {
	sessions := s.term.listSessions()
	if u, ok := s.currentUser(r); ok && u.hostScopeRestricted() && roleRank(u.Role) < roleRank(RoleAdmin) {
		filtered := make([]termSessionInfo, 0, len(sessions))
		for _, sess := range sessions {
			if s.userCanAccessHost(u, sess.HostID) {
				filtered = append(filtered, sess)
			}
		}
		sessions = filtered
	}
	writeJSON(w, http.StatusOK, sessions)
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
	info, found := s.term.lookupSessionInfo(sid)
	if !found {
		// Orphan recording files without metadata must not leak to scoped users.
		if frames := s.term.getRecording(sid); frames != nil {
			u, ok := s.currentUser(r)
			if ok && u.hostScopeRestricted() && roleRank(u.Role) < roleRank(RoleAdmin) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "无权访问该主机（主机组/标签授权）"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"frames": frames, "count": len(frames)})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "terminal.session_not_found")})
		return
	}
	if info.HostID != "" && !s.requireHostAccess(w, r, info.HostID) {
		return
	}
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
	info, found := s.term.lookupSessionInfo(sid)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "terminal.session_not_found")})
		return
	}
	if info.HostID != "" && !s.requireHostAccess(w, r, info.HostID) {
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
	actor, ip := s.actorIP(r)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: actor, IP: ip, Message: Tz("log.observe_terminal", sid[:8])})
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
