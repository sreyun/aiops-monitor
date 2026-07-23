package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// isPublicPath reports whether a request may proceed without a session:
// the dashboard shell + static assets, agent register/report, the install /
// uninstall scripts and downloads, and the login / me endpoints.
func isPublicPath(r *http.Request) bool {
	p := r.URL.Path
	switch p {
	case "/", "/healthz", "/style.css", "/app.js", "/theme-init.js", "/i18n-dashboard.js", "/i18n-dashboard.en.js", "/i18n-dashboard.zh-TW.js",
		"/sw.js", "/manifest.json", "/icon.svg", // PWA shell: SW must register on the pre-login page too
		"/install.sh", "/install.ps1", "/uninstall.sh", "/uninstall.ps1",
		"/api/v1/login", "/api/v1/me",
		"/api/v1/forward/health",
		"/api/v1/account/recover-send-code",
		"/api/v1/account/recover-verify",
		"/api/v1/account/recover-verify-mfa",
		"/api/v1/account/recover-username",
		"/api/v1/account/send-reset-code",
		"/api/v1/account/reset-password",
		"/api/v1/agent/register", "/api/v1/agent/report",
		"/api/v1/mcp", // MCP server：外部 Agent(如 Hermes Agent) 连接，在 handler 内做 Bearer Token 鉴权
		"/api/v1/prom/write", // Prometheus remote_write 接收：外部 exporter/telegraf/OTel 推送，在 handler 内做 Bearer 令牌鉴权
		"/api/v1/agent/logs": // fingerprint-gated log ingest (checked in the handler)
		return true
	}
	// Agent-facing hardware/netflow/hyperv/snmp ingest are fingerprint-gated, not
	// session-gated (the fingerprint is verified inside each handler).
	if p == "/api/v1/agent/hardware" || p == "/api/v1/agent/netflow" || p == "/api/v1/agent/hyperv" ||
		p == "/api/v1/agent/snmp" || p == "/api/v1/agent/snmp/trap" || p == "/api/v1/agent/dnsmap" ||
		p == "/api/v1/agent/content-audit" || p == "/api/v1/agent/probe-results" {
		return true
	}
	// 拆分后的前端静态模块（/js/*.js、/css/*）与 /app.js、/style.css 同属登录前外壳，需放行。
	if strings.HasPrefix(p, "/js/") || strings.HasPrefix(p, "/css/") {
		return true
	}
	// Agent-facing terminal reverse channels are token-gated, not session-gated.
	if strings.HasPrefix(p, "/api/v1/agent/terminal/") {
		return true
	}
	// Agent-facing port forwarding reverse channels are fingerprint-gated.
	if strings.HasPrefix(p, "/api/v1/agent/forward/") {
		return true
	}
	return strings.HasPrefix(p, "/dl/")
}

// currentUser resolves the logged-in user's account from the session cookie.
func (s *Server) currentUser(r *http.Request) (AccountConfig, bool) {
	name := s.auth.userForRequest(r)
	if name == "" {
		return AccountConfig{}, false
	}
	return s.cfg.UserByName(name)
}

// validatePasswordStrength enforces the account password policy: at least 8
// characters including an uppercase letter, a lowercase letter, a digit and a
// special (non-alphanumeric) character.
func validatePasswordStrength(pw string) bool {
	if len([]rune(pw)) < 8 {
		return false
	}
	var up, lo, dg, sp bool
	for _, c := range pw {
		switch {
		case c >= 'A' && c <= 'Z':
			up = true
		case c >= 'a' && c <= 'z':
			lo = true
		case c >= '0' && c <= '9':
			dg = true
		default:
			sp = true // any non-alphanumeric rune counts as a special character
		}
	}
	return up && lo && dg && sp
}

// routeAllowed enforces RBAC: any logged-in role may manage its own account;
// viewer is otherwise read-only; the remote terminal needs operator+; user
// management, admin ops, alert/AI settings writes need admin; every other write needs operator+.
func (s *Server) routeAllowed(r *http.Request, role string) bool {
	rank := roleRank(role)
	if rank == 0 {
		return false
	}
	p := r.URL.Path
	switch p { // own-account self-service: any logged-in role
	case "/api/v1/logout", "/api/v1/password", "/api/v1/profile", "/api/v1/account/init",
		"/api/v1/mfa/setup", "/api/v1/mfa/enable", "/api/v1/mfa/disable",
		"/api/v1/mfa/unbind-via-email":
		return true
	}
	if strings.HasPrefix(p, "/api/v1/users") || p == "/api/v1/mfa/global" || strings.HasPrefix(p, "/api/v1/admin/") { // user mgmt + admin ops: admin only
		return rank >= roleRank(RoleAdmin)
	}
	// 敏感系统配置：告警通道/阈值、AI Provider 设置及其连通性测试 —— 仅管理员可写。
	// GET 仍按下方 viewer+ 放行（密钥已脱敏），供界面回填与能力探测。
	switch p {
	case "/api/v1/config":
		if r.Method != http.MethodGet {
			return rank >= roleRank(RoleAdmin)
		}
	case "/api/v1/config/test",
		"/api/v1/ai/config",
		"/api/v1/ai/test",
		"/api/v1/ai/test-embed",
		"/api/v1/ai/test-rerank",
		"/api/v1/ai/test-weknora",
		"/api/v1/ai/models",
		"/api/v1/ai/terminal-access":
		if r.Method != http.MethodGet {
			return rank >= roleRank(RoleAdmin)
		}
	}
	if strings.Contains(p, "/terminal") || strings.HasPrefix(p, "/api/v1/forward") || strings.HasPrefix(p, "/proxy/") || p == "/api/v1/proxy-token" { // remote shell + port forwarding: operator+
		return rank >= roleRank(RoleOperator)
	}
	if r.Method == http.MethodGet { // reads: viewer+
		return rank >= roleRank(RoleViewer)
	}
	return rank >= roleRank(RoleOperator) // other writes/actions: operator+
}

// authMiddleware gates every non-public path on a valid session AND a sufficient
// role for the requested route.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// v5.4.1: verify relay shared secret when configured. Requests that
		// carry X-Relay-Secret must match the configured secret; requests
		// without the header are allowed (direct, not through relay).
		if relaySecret := s.cfg.RelaySecret(); relaySecret != "" {
			if hdr := r.Header.Get("X-Relay-Secret"); hdr != "" {
				if subtle.ConstantTimeCompare([]byte(hdr), []byte(relaySecret)) != 1 {
					s.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: s.clientIP(r), IP: s.clientIP(r), Message: Tz("log.relay_secret_mismatch")})
					writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "auth.relay_unauthorized")})
					return
				}
			}
		}
		if isPublicPath(r) {
			next.ServeHTTP(w, r)
			return
		}
		// HTTP proxy token auth: allows window.open in new tab without relying on
		// the session cookie (which may not be sent cross-context in some browsers).
		// Priority: cookie (set by handleProxyToken) > query param (fallback).
		if strings.HasPrefix(r.URL.Path, "/proxy/") {
			var tok string
			if c, err := r.Cookie("proxy_token"); err == nil && c.Value != "" {
				tok = c.Value
			} else if pt := r.URL.Query().Get("pt"); pt != "" {
				tok = pt
			}
			if tok != "" {
				if user := s.auth.validateProxyToken(tok); user != "" {
					// 纵深防御：代理令牌本就仅 operator+ 可签发，这里仍按令牌所属用户的
					// 当前角色复核 RBAC，防止签发后被降权的用户在令牌有效窗口内经 /proxy/ 越权。
					if s.routeAllowed(r, s.cfg.RoleOf(user)) {
						next.ServeHTTP(w, r)
						return
					}
					writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "auth.insufficient_permission")})
					return
				}
			}
		}
		name := s.auth.userForRequest(r)
		if name == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.unauthorized")})
			return
		}
		// Restricted sessions (global MFA enforcement) can only touch MFA endpoints.
		if s.auth.isRestricted(r) {
			p := r.URL.Path
			if p != "/api/v1/mfa/setup" && p != "/api/v1/mfa/enable" && p != "/api/v1/logout" {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "auth.mfa_required_first")})
				return
			}
		}
		if !s.routeAllowed(r, s.cfg.RoleOf(name)) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "auth.insufficient_permission")})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- auth handlers ----

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		LoginType string `json:"login_type"` // "username" (default), "phone"
		Code      string `json:"code"`       // TOTP second factor (only when MFA is enabled)
	}
	ip := s.clientIP(r)
	if !s.auth.loginAllowed(ip) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": Tr(r, "auth.too_many_attempts")})
		return
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	// Resolve and authenticate by login type
	var acc AccountConfig
	var authenticated bool
	switch req.LoginType {
	case "phone":
		acc, authenticated = s.authenticatePhoneLogin(w, r, req.Username, req.Password, ip)
	default:
		acc, authenticated = s.authenticateUsernameLogin(w, r, req.Username, req.Password, ip)
	}
	if !authenticated {
		return // error response already written by the authenticate* helper
	}
	// Credentials verified — proceed to MFA + session issuance.
	s.completeLogin(w, r, acc, req.Password, req.Code, ip)
}

// authenticateUsernameLogin verifies username+password via CheckPassword (which
// handles PBKDF2 upgrade). Returns the resolved account and whether authentication
// succeeded. On failure, writes the error response and returns false.
func (s *Server) authenticateUsernameLogin(w http.ResponseWriter, r *http.Request, username, password, ip string) (AccountConfig, bool) {
	acc, ok := s.auth.CheckPassword(username, password)
	if !ok {
		s.auth.loginFailed(ip)
		s.auth.loginAccountFailed(username)
		s.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: ip, IP: ip, Message: Tz("log.login_failed", username)})
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.invalid_credentials")})
		return acc, false
	}
	return acc, true
}

// authenticatePhoneLogin resolves an account by phone number and verifies the
// password separately (since CheckPassword uses UserByName). On failure, writes
// the error response and returns false.
func (s *Server) authenticatePhoneLogin(w http.ResponseWriter, r *http.Request, phone, password, ip string) (AccountConfig, bool) {
	acc, found := s.cfg.UserByPhone(phone)
	if !found {
		hashPassword(password, "dummy-salt-000000") // constant-ish timing
		s.auth.loginFailed(ip)
		s.auth.loginAccountFailed(phone)
		s.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: ip, IP: ip, Message: Tz("log.login_failed", "phone:"+phone)})
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.invalid_credentials")})
		return acc, false
	}
	if !verifyPassword(password, acc.Salt, acc.Hash) {
		s.auth.loginFailed(ip)
		s.auth.loginAccountFailed(acc.Username)
		s.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: ip, IP: ip, Message: Tz("log.login_failed", acc.Username)})
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.invalid_credentials")})
		return acc, false
	}
	if !s.auth.loginAccountAllowed(acc.Username) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": Tr(r, "auth.too_many_attempts")})
		return acc, false
	}
	return acc, true
}

// completeLogin handles the post-authentication phase: default-credential
// detection, MFA second factor, session issuance and the response.
func (s *Server) completeLogin(w http.ResponseWriter, r *http.Request, acc AccountConfig, password, code, ip string) {
	// v5.4.0: detect default admin/admin credentials and force a password
	// change on first login. Do not override an existing MustChangePassword
	// flag (it may already be set by the admin reset tool).
	if !acc.MustChangePassword && acc.Username == "admin" && password == "admin" {
		s.cfg.SetMustChangePassword(acc.Username)
		acc.MustChangePassword = true
		s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: ip, IP: ip, Message: Tz("log.default_credentials", acc.Username)})
	}
	// Password OK. If MFA is on, require a valid TOTP code as the second factor.
	// The requirement is revealed only AFTER the password checks out, so an
	// unauthenticated prober can't learn whether the account has MFA enabled.
	if acc.MFAEnabled {
		if strings.TrimSpace(code) == "" {
			writeJSON(w, http.StatusOK, map[string]any{"mfa_required": true})
			return
		}
		if !s.auth.verifyTOTPOnce(acc.Username, acc.MFASecret, code) {
			s.auth.loginFailed(ip)
			s.auth.loginAccountFailed(acc.Username)
			s.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: ip, IP: ip, Message: Tz("log.totp_failed", acc.Username)})
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.totp_error")})
			return
		}
	}
	// Credentials fully verified — clear the per-account failed-attempt counter.
	s.auth.loginAccountReset(acc.Username)
	// Global MFA policy: if admin has enabled MFARequired and this user hasn't
	// set up MFA yet, issue a restricted session and direct them to enroll.
	if s.cfg.MFARequired() && !acc.MFAEnabled {
		tok := s.auth.issueRestrictedSession(acc.Username)
		http.SetCookie(w, &http.Cookie{
			Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true,
			Secure:   s.isHTTPS(r),
			SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL / time.Second),
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"require_mfa_setup": true,
			"message":           Tr(r, "auth.global_mfa_required"),
		})
		return
	}
	tok := s.auth.issueSession(acc.Username)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true,
		Secure:   s.isHTTPS(r),
		SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL / time.Second),
	})
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: ip, IP: ip, Message: Tz("log.login_success", acc.Username)})
	resp := map[string]any{"ok": true}
	// v5.4.0: force password change if admin reset was used
	if acc.MustChangePassword {
		resp["must_change_password"] = true
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.auth.Logout(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleMe returns the current profile, or 401 if not logged in (the panel uses
// this to decide whether to show the login screen).
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	acc, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"username": acc.Username, "display_name": acc.DisplayName, "email": acc.Email, "phone": acc.Phone,
		"mfa_enabled": acc.MFAEnabled, "role": acc.Role,
		"must_change_password": acc.MustChangePassword,
	})
}

func (s *Server) handleSetProfile(w http.ResponseWriter, r *http.Request) {
	acc, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Phone       string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	name := acc.Username
	// Optional self-rename: validate, apply, then repoint the session so the
	// current cookie keeps working under the new username.
	if strings.TrimSpace(req.Username) != "" && req.Username != acc.Username {
		uname := sanitizeUsername(req.Username)
		if uname == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "auth.invalid_username_format")})
			return
		}
		if err := s.cfg.RenameUser(acc.Username, uname); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.auth.renameSessions(acc.Username, uname)
		name = uname
	}
	_ = s.cfg.SetUserProfile(name, strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Email), strings.TrimSpace(req.Phone))
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.actorName(r), IP: s.clientIP(r), Message: Tz("log.update_profile", name)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": name})
}

// ---- SMS verification code (reserved for future phone login) ----

// smsCodeEntry is a temporary in-memory store for SMS verification codes.
type smsCodeEntry struct {
	Code     string
	ExpireAt time.Time
}

var (
	smsCodeMu sync.Mutex
	smsCodes  = map[string]smsCodeEntry{} // phone -> entry
	smsLastMu sync.Mutex
	smsLast   = map[string]time.Time{} // phone -> last send time (rate limit)
)

// handleLoginSMSCode sends a 6-digit verification code to the given phone number.
// This is a reserved endpoint — SMS sending is not yet implemented.
func (s *Server) handleLoginSMSCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	phone := strings.TrimSpace(req.Phone)
	if phone == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "login.phone_required")})
		return
	}
	// Check if phone is registered
	_, found := s.cfg.UserByPhone(phone)
	if !found {
		// Don't reveal whether phone exists — use the same delay
		writeJSON(w, http.StatusOK, map[string]any{"message": Tr(r, "login.sms_sent")})
		return
	}
	// Rate limit: 60s between sends
	smsLastMu.Lock()
	last, exists := smsLast[phone]
	if exists && time.Since(last) < 60*time.Second {
		smsLastMu.Unlock()
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": Tr(r, "recovery.rate_limited")})
		return
	}
	smsLast[phone] = time.Now()
	smsLastMu.Unlock()
	// Generate 6-digit code using crypto/rand
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate code"})
		return
	}
	code := fmt.Sprintf("%06d", n.Int64())
	smsCodeMu.Lock()
	smsCodes[phone] = smsCodeEntry{Code: code, ExpireAt: time.Now().Add(5 * time.Minute)}
	smsCodeMu.Unlock()
	// TODO: Call actual SMS sending service here
	// sendSMS(phone, code)
	s.store.AddLog(LogEntry{Kind: KindSystem, Level: "info", Actor: s.actorName(r), IP: s.clientIP(r), Message: fmt.Sprintf("SMS code sent to %s (placeholder)", phone)})
	writeJSON(w, http.StatusOK, map[string]any{"message": Tr(r, "login.sms_sent")})
}

func (s *Server) handleSetPassword(w http.ResponseWriter, r *http.Request) {
	acc, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	var req struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if !verifyPassword(req.Old, acc.Salt, acc.Hash) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "auth.wrong_old_password")})
		return
	}
	if !validatePasswordStrength(strings.TrimSpace(req.New)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "auth.password_policy")})
		return
	}
	_ = s.cfg.SetUserPassword(acc.Username, req.New)
	// v5.4.0: clear MustChangePassword flag after a successful self-change
	s.cfg.ClearMustChangePassword(acc.Username)
	// Invalidate only THIS user's other sessions, then re-issue one for the current.
	s.auth.clearUserSessions(acc.Username)
	tok := s.auth.issueSession(acc.Username)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true,
		Secure:   s.isHTTPS(r),
		SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL / time.Second),
	})
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r), Message: Tz("log.change_password", acc.Username)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleAccountInit performs the forced first-login credential setup in one
// atomic step: the user picks a new username AND a new password without
// re-entering the old one. It is deliberately gated on MustChangePassword being
// set — i.e. it only works right after a forced-change login (default admin/admin
// or an admin password reset) and refuses once the flag is cleared. So it can
// never be abused during normal operation to bypass the old-password check in
// handleSetPassword.
func (s *Server) handleAccountInit(w http.ResponseWriter, r *http.Request) {
	acc, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	if !acc.MustChangePassword {
		// Forced-init window is closed: normal changes must go through
		// /profile + /password (which verify the current password).
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "auth.init_not_required")})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if !validatePasswordStrength(strings.TrimSpace(req.Password)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "auth.password_policy")})
		return
	}
	name := acc.Username
	// Optional self-rename: validate, apply, then repoint the session so the
	// current cookie keeps working under the new username.
	if strings.TrimSpace(req.Username) != "" && req.Username != acc.Username {
		uname := sanitizeUsername(req.Username)
		if uname == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "auth.invalid_username_format")})
			return
		}
		if err := s.cfg.RenameUser(acc.Username, uname); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.auth.renameSessions(acc.Username, uname)
		name = uname
	}
	_ = s.cfg.SetUserPassword(name, req.Password)
	s.cfg.ClearMustChangePassword(name)
	// Force a fresh re-login: invalidate ALL of this user's sessions (including the
	// current one) and clear the session cookie, so the user must sign in again
	// with the new credentials. This confirms they actually know the new password
	// and starts a clean session under the (possibly renamed) account.
	s.auth.clearUserSessions(name)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		Secure: s.isHTTPS(r), SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r), Message: Tz("log.change_password", name)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": name, "relogin": true})
}

// ---- MFA (TOTP two-factor) ----

// handleMFASetup issues a fresh TOTP secret + provisioning URL for the current
// user's enrollment. It does NOT enable MFA — the client must prove one valid
// code via handleMFAEnable, so a mis-scanned secret can never lock them out.
func (s *Server) handleMFASetup(w http.ResponseWriter, r *http.Request) {
	acc, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	secret := genTOTPSecret()
	if secret == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": Tr(r, "auth.gen_secret_failed")})
		return
	}
	uri := otpauthURL(acc.Username, secret)
	qr, err := genQRDataURI(uri)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": Tr(r, "auth.gen_qr_failed")})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret":      secret,
		"otpauth_url": uri,
		"qr_datauri":  qr,
	})
}

// handleMFAEnable turns the current user's MFA on after verifying they can
// produce a current code for the freshly-issued secret.
func (s *Server) handleMFAEnable(w http.ResponseWriter, r *http.Request) {
	acc, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	var req struct {
		Secret string `json:"secret"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if !totpVerify(req.Secret, req.Code) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "auth.totp_verify_failed")})
		return
	}
	_ = s.cfg.SetUserMFA(acc.Username, true, strings.TrimSpace(req.Secret))
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r), Message: Tz("log.enable_mfa", acc.Username)})
	// Upgrade a restricted session (global MFA enforcement) to a full session.
	s.auth.upgradeSession(r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- Global MFA policy ----

// handleMFAGlobalGet returns the current global MFA enforcement state.
func (s *Server) handleMFAGlobalGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"mfa_required": s.cfg.MFARequired(),
	})
}

// handleMFAGlobalSet toggles the global MFA enforcement policy (admin only).
func (s *Server) handleMFAGlobalSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Required bool `json:"required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if err := s.cfg.SetMFARequired(req.Required); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": Tr(r, "auth.save_failed")})
		return
	}
	action := Tz("log.global_mfa_off")
	if req.Required {
		action = Tz("log.global_mfa_on")
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r), Message: action})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mfa_required": req.Required})
}

// handleMFADisable turns the current user's MFA off after re-verifying their
// password, so a hijacked-but-unlocked session alone can't strip the factor.
func (s *Server) handleMFADisable(w http.ResponseWriter, r *http.Request) {
	acc, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if !verifyPassword(req.Password, acc.Salt, acc.Hash) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "auth.wrong_password")})
		return
	}
	_ = s.cfg.SetUserMFA(acc.Username, false, "")
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r), Message: Tz("log.disable_mfa", acc.Username)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
