package main

import "net/http"

// -----------------------------------------------------------------------
// Terminal enhancement handlers
// -----------------------------------------------------------------------

func (s *Server) handleListTerminalSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.term.listSessions())
}

func (s *Server) handleTerminalReplay(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("id")
	frames := s.term.getRecording(sid)
	if frames == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "会话不存在或已结束"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"frames": frames, "count": len(frames)})
}

// handleTerminalObserve allows a second logged-in user to watch a live terminal
// session in read-only mode via WebSocket.
func (s *Server) handleTerminalObserve(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("id")
	if !s.cfg.TerminalEnabled() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "远程终端已被管理员禁用"})
		return
	}
	obs, ok := s.term.addObserver(sid)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	defer s.term.removeObserver(sid, obs)
	ws, err := wsAccept(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "需要 WebSocket 升级"})
		return
	}
	defer ws.Close()
	// 使用实际登录用户名记录审计日志
	user, _ := s.currentUser(r)
	actor := user.Username
	if actor == "" {
		actor = s.clientIP(r)
	}
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: actor, Message: "旁观终端会话 " + sid[:8]})
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
