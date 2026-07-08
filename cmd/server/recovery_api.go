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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "recovery.invalid_email")})
		return
	}
	// Do NOT reveal whether the email matches — always return the same response.
	// Only actually send when the address matches some user's bound email.
	if user, found := s.cfg.UserByEmail(req.Email); found {
		cfg := s.cfg.Get()
		if cfg.SMTP.Enabled && cfg.SMTP.Host != "" {
			html := fmt.Sprintf(`<div style="font-family:sans-serif;padding:20px;max-width:500px;margin:0 auto">
  <h2>%s</h2>
  <p>%s</p>
  <p style="color:#888;font-size:13px">%s</p>
  <p style="color:#888;font-size:13px">%s</p>
</div>`, Tz("recovery.email_username_subject"), fmt.Sprintf(Tz("recovery.email_username_body"), user.Username), Tz("recovery.email_disclaimer"), fmt.Sprintf(Tz("recovery.email_time"), time.Now().Format("2006-01-02 15:04:05")))
			if err := sendEmail(cfg.SMTP, req.Email, Tz("recovery.email_username_subject"), html); err != nil {
				s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.username_email_failed", err.Error())})
				writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": Tr(r, "recovery.email_send_failed")})
				return
			}
			s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: Tz("log.username_email_sent")})
		}
	}
	// Always return ok to prevent email enumeration
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": Tr(r, "recovery.username_sent")})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "recovery.username_required")})
		return
	}
	user, found := s.cfg.UserByName(req.Username)
	// Uniform response regardless of whether the user exists / has an email, to
	// avoid username enumeration.
	if !found || user.Email == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": Tr(r, "recovery.code_sent")})
		return
	}
	cfg := s.cfg.Get()
	if !cfg.SMTP.Enabled || cfg.SMTP.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "recovery.no_smtp")})
		return
	}
	code, err := s.emailMgr.issueCode(user.Email, "reset_password")
	if err != nil {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": err.Error()})
		return
	}
	html := fmt.Sprintf(`<div style="font-family:sans-serif;padding:20px;max-width:500px;margin:0 auto">
  <h2>%s</h2>
  <p>%s</p>
  <p>%s</p>
  <p style="color:#888;font-size:13px">%s</p>
  <p style="color:#888;font-size:13px">%s</p>
</div>`, Tz("recovery.email_reset_subject"), fmt.Sprintf(Tz("recovery.email_reset_body"), code), Tz("recovery.email_reset_validity"), Tz("recovery.email_reset_disclaimer"), fmt.Sprintf(Tz("recovery.email_time"), time.Now().Format("2006-01-02 15:04:05")))
	if err := sendEmail(cfg.SMTP, user.Email, Tz("recovery.email_reset_subject"), html); err != nil {
		s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.reset_code_failed", err.Error())})
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": Tr(r, "recovery.email_send_failed")})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: Tz("log.reset_code_sent")})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": Tr(r, "recovery.code_sent")})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "recovery.invalid_params")})
		return
	}
	if len(strings.TrimSpace(req.NewPass)) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "recovery.password_too_short")})
		return
	}
	user, found := s.cfg.UserByName(req.Username)
	if !found || !strings.EqualFold(req.Email, user.Email) || user.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "recovery.user_email_mismatch")})
		return
	}
	if !s.emailMgr.verifyCode(user.Email, "reset_password", req.Code) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "recovery.code_invalid")})
		return
	}
	_ = s.cfg.SetUserPassword(user.Username, req.NewPass)
	s.auth.clearUserSessions(user.Username) // invalidate that user's old sessions
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.reset_password", user.Username)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": Tr(r, "recovery.password_reset")})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "recovery.no_email_bound")})
		return
	}
	if !acc.MFAEnabled {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": Tr(r, "recovery.mfa_not_enabled")})
		return
	}
	if req.Action == "send_code" {
		cfg := s.cfg.Get()
		if !cfg.SMTP.Enabled || cfg.SMTP.Host == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "recovery.no_smtp_short")})
			return
		}
		code, err := s.emailMgr.issueCode(acc.Email, "mfa_unbind")
		if err != nil {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": err.Error()})
			return
		}
		html := fmt.Sprintf(`<div style="font-family:sans-serif;padding:20px;max-width:500px;margin:0 auto">
  <h2>%s</h2>
  <p>%s</p>
  <p>%s</p>
  <p>%s</p>
  <p style="color:#888;font-size:13px">%s</p>
</div>`, Tz("recovery.email_mfa_title"), Tz("recovery.email_mfa_desc"), fmt.Sprintf(Tz("recovery.email_mfa_code"), code), Tz("recovery.email_mfa_validity"), Tz("recovery.email_mfa_disclaimer"))
		if err := sendEmail(cfg.SMTP, acc.Email, Tz("recovery.email_mfa_subject"), html); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": Tr(r, "recovery.email_send_error", err.Error())})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": Tr(r, "recovery.code_sent_ok")})
		return
	}
	if req.Action == "verify" {
		req.Code = strings.TrimSpace(req.Code)
		if len(req.Code) != 6 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "recovery.code_required")})
			return
		}
		if !s.emailMgr.verifyCode(acc.Email, "mfa_unbind", req.Code) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "recovery.code_invalid")})
			return
		}
	_ = s.cfg.SetUserMFA(acc.Username, false, "")
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.mfa_unbind", acc.Username)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": Tr(r, "recovery.mfa_disabled")})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "recovery.unknown_action")})
}
