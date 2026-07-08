package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ---- user management (admin-only; enforced by routeAllowed) ----

// userView is the browser-safe projection of an account (no salt/hash/secret).
func userView(u AccountConfig) map[string]any {
	return map[string]any{
		"username": u.Username, "display_name": u.DisplayName,
		"email": u.Email, "role": u.Role, "mfa_enabled": u.MFAEnabled,
	}
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users := s.cfg.UsersList()
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, userView(u))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	uname := sanitizeUsername(req.Username)
	if uname == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "用户名仅限字母/数字/-_.，长度 2–32 位"})
		return
	}
	if len(strings.TrimSpace(req.Password)) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "密码至少 4 位"})
		return
	}
	if !validRole(req.Role) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "角色不合法"})
		return
	}
	email := strings.TrimSpace(req.Email)
	if email != "" && !validEmail(email) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "邮箱格式不合法"})
		return
	}
	if err := s.cfg.CreateUser(uname, req.Password, strings.TrimSpace(req.DisplayName), email, req.Role); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "创建用户：" + uname + "（" + req.Role + "）"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	var req struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if !validRole(req.Role) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "角色不合法"})
		return
	}
	email := strings.TrimSpace(req.Email)
	if email != "" && !validEmail(email) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "邮箱格式不合法"})
		return
	}
	if err := s.cfg.UpdateUserMeta(username, strings.TrimSpace(req.DisplayName), email, req.Role); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "修改用户：" + username + " → " + req.Role})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if cur, ok := s.currentUser(r); ok && cur.Username == username {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "不能删除当前登录的账户"})
		return
	}
	if err := s.cfg.DeleteUser(username); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.auth.clearUserSessions(username) // kick the removed user out
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "删除用户：" + username})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleResetUserPassword(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if len(strings.TrimSpace(req.Password)) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "密码至少 4 位"})
		return
	}
	if err := s.cfg.SetUserPassword(username, req.Password); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.auth.clearUserSessions(username) // force re-login with the new password
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "重置用户密码：" + username})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleResetUserMFA(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if err := s.cfg.SetUserMFA(username, false, ""); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "管理员解除用户两步验证：" + username})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
