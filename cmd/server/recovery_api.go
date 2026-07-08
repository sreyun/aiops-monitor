package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// -----------------------------------------------------------------------
// Account recovery: username recovery + password reset via email.
// These endpoints are PUBLIC (no session) — they are gated by email
// verification codes and rate limiting instead.
// -----------------------------------------------------------------------

// handleRecoverUsername sends the account username to the given email address,
// but only if that email matches the configured account.
func (s *Server) handleRecoverUsername(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if !validEmail(req.Email) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "邮箱格式不合法"})
		return
	}
	// Do NOT reveal whether the email matches — always return the same response.
	// Only actually send when the address matches some user's bound email.
	if user, found := s.cfg.UserByEmail(req.Email); found {
		cfg := s.cfg.Get()
		if cfg.SMTP.Enabled && cfg.SMTP.Host != "" {
			html := fmt.Sprintf(`<div style="font-family:sans-serif;padding:20px;max-width:500px;margin:0 auto">
  <h2>AIOps Monitor — 账户用户名</h2>
  <p>您的登录用户名为：<b style="font-size:18px">%s</b></p>
  <p style="color:#888;font-size:13px">如非本人操作请忽略此邮件。</p>
  <p style="color:#888;font-size:13px">时间：%s</p>
</div>`, user.Username, time.Now().Format("2006-01-02 15:04:05"))
			if err := sendEmail(cfg.SMTP, req.Email, "AIOps Monitor — 用户名找回", html); err != nil {
				s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "用户名找回邮件发送失败：" + err.Error()})
				writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "邮件发送失败，请稍后重试"})
				return
			}
			s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: s.clientIP(r), Message: "用户名找回邮件已发送"})
		}
	}
	// Always return ok to prevent email enumeration
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "如该邮箱已绑定账户，用户名将通过邮件发送"})
}

// handleSendResetCode sends a 6-digit verification code to the email bound to
// the given username. The code is required for the subsequent password reset.
func (s *Server) handleSendResetCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请输入用户名"})
		return
	}
	user, found := s.cfg.UserByName(req.Username)
	// Uniform response regardless of whether the user exists / has an email, to
	// avoid username enumeration.
	if !found || user.Email == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "如该用户名存在且已绑定邮箱，验证码已发送"})
		return
	}
	cfg := s.cfg.Get()
	if !cfg.SMTP.Enabled || cfg.SMTP.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "系统未配置邮件服务，请联系管理员"})
		return
	}
	code, err := s.emailMgr.issueCode(user.Email, "reset_password")
	if err != nil {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": err.Error()})
		return
	}
	html := fmt.Sprintf(`<div style="font-family:sans-serif;padding:20px;max-width:500px;margin:0 auto">
  <h2>AIOps Monitor — 密码重置验证码</h2>
  <p>您的验证码为：<b style="font-size:24px;letter-spacing:4px;color:#4c8dff">%s</b></p>
  <p>验证码 10 分钟内有效，请尽快使用。</p>
  <p style="color:#888;font-size:13px">如非本人操作请忽略此邮件并建议修改密码。</p>
  <p style="color:#888;font-size:13px">时间：%s</p>
</div>`, code, time.Now().Format("2006-01-02 15:04:05"))
	if err := sendEmail(cfg.SMTP, user.Email, "AIOps Monitor — 密码重置验证码", html); err != nil {
		s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "密码重置验证码发送失败：" + err.Error()})
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "邮件发送失败，请稍后重试"})
		return
	}
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: s.clientIP(r), Message: "密码重置验证码已发送至绑定邮箱"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "如该用户名存在且已绑定邮箱，验证码已发送"})
}

// handleResetPassword resets the account password after verifying the email code.
func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Code     string `json:"code"`
		NewPass  string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	req.Code = strings.TrimSpace(req.Code)
	if req.Username == "" || !validEmail(req.Email) || len(req.Code) != 6 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "参数不完整或格式错误"})
		return
	}
	if len(strings.TrimSpace(req.NewPass)) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "新密码至少 4 位"})
		return
	}
	user, found := s.cfg.UserByName(req.Username)
	if !found || !strings.EqualFold(req.Email, user.Email) || user.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "用户名或邮箱不匹配"})
		return
	}
	if !s.emailMgr.verifyCode(user.Email, "reset_password", req.Code) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "验证码错误或已过期"})
		return
	}
	_ = s.cfg.SetUserPassword(user.Username, req.NewPass)
	s.auth.clearUserSessions(user.Username) // invalidate that user's old sessions
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "通过邮箱验证码重置密码：" + user.Username})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "密码已重置，请使用新密码登录"})
}

// handleMFAUnbindViaEmail disables MFA after verifying a code sent to the bound
// email. This is the recovery path when the operator lost their authenticator.
func (s *Server) handleMFAUnbindViaEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"` // "send_code" | "verify"
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	acc, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if acc.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "账户未绑定邮箱，无法通过邮箱解除 MFA"})
		return
	}
	if !acc.MFAEnabled {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "MFA 未启用"})
		return
	}
	if req.Action == "send_code" {
		cfg := s.cfg.Get()
		if !cfg.SMTP.Enabled || cfg.SMTP.Host == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "系统未配置邮件服务"})
			return
		}
		code, err := s.emailMgr.issueCode(acc.Email, "mfa_unbind")
		if err != nil {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": err.Error()})
			return
		}
		html := fmt.Sprintf(`<div style="font-family:sans-serif;padding:20px;max-width:500px;margin:0 auto">
  <h2>AIOps Monitor — 解除两步验证</h2>
  <p>您正在通过邮箱解除两步验证（MFA）绑定。</p>
  <p>验证码：<b style="font-size:24px;letter-spacing:4px;color:#4c8dff">%s</b></p>
  <p>验证码 10 分钟内有效，单次使用。</p>
  <p style="color:#888;font-size:13px">如非本人操作，请立即修改密码。</p>
</div>`, code)
		if err := sendEmail(cfg.SMTP, acc.Email, "AIOps Monitor — MFA 解除验证码", html); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "邮件发送失败：" + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "验证码已发送至绑定邮箱"})
		return
	}
	if req.Action == "verify" {
		req.Code = strings.TrimSpace(req.Code)
		if len(req.Code) != 6 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请输入 6 位验证码"})
			return
		}
		if !s.emailMgr.verifyCode(acc.Email, "mfa_unbind", req.Code) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "验证码错误或已过期"})
			return
		}
		_ = s.cfg.SetUserMFA(acc.Username, false, "")
		s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "通过邮箱验证码解除两步验证：" + acc.Username})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "两步验证已关闭"})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "未知操作类型"})
}
