package main

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// handleListMessages returns the notification feed (newest-first) + unread count.
func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, _ := strconv.Atoi(l); v > 0 {
			limit = v
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"messages": s.messages.list(limit),
		"unread":   s.messages.unreadCount(),
	})
}

// handleMarkMessagesRead marks the given message IDs as read.
func (s *Server) handleMarkMessagesRead(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	s.messages.markRead(req.IDs)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "unread": s.messages.unreadCount()})
}

// handleMarkAllMessagesRead clears the whole unread badge.
func (s *Server) handleMarkAllMessagesRead(w http.ResponseWriter, r *http.Request) {
	s.messages.markAllRead()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "unread": 0})
}
