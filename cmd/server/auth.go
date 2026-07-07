package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookie = "aiops_session"
	sessionTTL    = 7 * 24 * time.Hour
)

// hashPassword returns a salted SHA-256 hash of the password (hex). Stdlib-only;
// adequate for a LAN ops dashboard behind a session cookie.
func hashPassword(pass, salt string) string {
	sum := sha256.Sum256([]byte(salt + ":" + pass))
	return hex.EncodeToString(sum[:])
}

type session struct {
	user    string
	expires time.Time
}

// Auth manages login sessions against the account in ConfigStore. Sessions
// are persisted through the embedded DB so a server restart keeps everyone
// logged in.
type Auth struct {
	cfg      *ConfigStore
	mu       sync.Mutex
	sessions map[string]session
	dirty    bool

	limMu    sync.Mutex
	loginHit map[string][]int64 // client ip -> recent FAILED attempt unix times
}

const (
	loginWindowSec   = 300 // sliding window for brute-force throttling
	loginMaxFailures = 8   // max failed attempts per IP per window
)

func NewAuth(cfg *ConfigStore) *Auth {
	return &Auth{cfg: cfg, sessions: map[string]session{}, loginHit: map[string][]int64{}}
}

// loginAllowed reports whether ip is under the failed-attempt threshold. It also
// prunes stale entries so the map stays bounded.
func (a *Auth) loginAllowed(ip string) bool {
	now := time.Now().Unix()
	cutoff := now - loginWindowSec
	a.limMu.Lock()
	defer a.limMu.Unlock()
	if len(a.loginHit) > 4096 { // safety valve against unbounded growth
		a.loginHit = map[string][]int64{}
	}
	kept := a.loginHit[ip][:0]
	for _, t := range a.loginHit[ip] {
		if t >= cutoff {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(a.loginHit, ip)
	} else {
		a.loginHit[ip] = kept
	}
	return len(kept) < loginMaxFailures
}

// loginFailed records one failed attempt for ip.
func (a *Auth) loginFailed(ip string) {
	now := time.Now().Unix()
	a.limMu.Lock()
	a.loginHit[ip] = append(a.loginHit[ip], now)
	a.limMu.Unlock()
}

func newSessionToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}

// CheckPassword reports whether user+pass match the configured account
// (constant-time on both fields). It does NOT issue a session — a second factor
// (TOTP) may still be required; the caller issues the session via issueSession.
func (a *Auth) CheckPassword(user, pass string) bool {
	acc := a.cfg.Account()
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(acc.Username)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(hashPassword(pass, acc.Salt)), []byte(acc.Hash)) == 1
	return userOK && passOK
}

func (a *Auth) validate(tok string) bool {
	if tok == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[tok]
	if !ok || time.Now().After(s.expires) {
		if ok {
			delete(a.sessions, tok)
		}
		return false
	}
	return true
}

func (a *Auth) ValidateRequest(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	return a.validate(c.Value)
}

func (a *Auth) Logout(tok string) {
	a.mu.Lock()
	delete(a.sessions, tok)
	a.dirty = true
	a.mu.Unlock()
}

// ClearSessions invalidates every active login session — used after a password
// change so any previously-issued (or stolen) cookie immediately stops working.
func (a *Auth) ClearSessions() {
	a.mu.Lock()
	a.sessions = map[string]session{}
	a.dirty = true
	a.mu.Unlock()
}

// issueSession creates and stores a fresh session token for user.
func (a *Auth) issueSession(user string) string {
	tok := newSessionToken()
	a.mu.Lock()
	a.sessions[tok] = session{user: user, expires: time.Now().Add(sessionTTL)}
	a.dirty = true
	a.mu.Unlock()
	return tok
}

// exportSessions / importSessions bridge the in-memory session table to the
// embedded DB snapshot (expired entries are skipped both ways).
func (a *Auth) exportSessions() map[string]dbSession {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]dbSession, len(a.sessions))
	now := time.Now()
	for tok, s := range a.sessions {
		if s.expires.After(now) {
			out[tok] = dbSession{User: s.user, Expires: s.expires.Unix()}
		}
	}
	return out
}

func (a *Auth) importSessions(in map[string]dbSession) {
	if len(in) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for tok, s := range in {
		exp := time.Unix(s.Expires, 0)
		if exp.After(now) {
			a.sessions[tok] = session{user: s.User, expires: exp}
		}
	}
}

func (a *Auth) consumeDirty() bool {
	a.mu.Lock()
	d := a.dirty
	a.dirty = false
	a.mu.Unlock()
	return d
}

// isPublicPath reports whether a request may proceed without a session:
// the dashboard shell + static assets, agent register/report, the install /
// uninstall scripts and downloads, and the login / me endpoints.
func isPublicPath(r *http.Request) bool {
	p := r.URL.Path
	switch p {
	case "/", "/healthz", "/style.css", "/app.js",
		"/install.sh", "/install.ps1", "/uninstall.sh", "/uninstall.ps1",
		"/api/v1/login", "/api/v1/me",
		"/api/v1/account/recover-username",
		"/api/v1/account/send-reset-code",
		"/api/v1/account/reset-password",
		"/api/v1/agent/register", "/api/v1/agent/report":
		return true
	}
	// Agent-facing terminal reverse channels are token-gated, not session-gated.
	if strings.HasPrefix(p, "/api/v1/agent/terminal/") {
		return true
	}
	return strings.HasPrefix(p, "/dl/")
}

// authMiddleware gates every non-public path on a valid session cookie.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r) || s.auth.ValidateRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	})
}

// ---- auth handlers ----

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"` // TOTP second factor (only when MFA is enabled)
	}
	ip := s.clientIP(r)
	if !s.auth.loginAllowed(ip) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "登录尝试过于频繁，请 5 分钟后再试"})
		return
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if !s.auth.CheckPassword(req.Username, req.Password) {
		s.auth.loginFailed(ip)
		s.store.AddLog(LogEntry{Kind: "系统", Level: "warning", Actor: ip, Message: "登录失败：" + req.Username})
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "用户名或密码错误"})
		return
	}
	// Password OK. If MFA is on, require a valid TOTP code as the second factor.
	// The requirement is revealed only AFTER the password checks out, so an
	// unauthenticated prober can't learn whether the account has MFA enabled.
	acc := s.cfg.Account()
	if acc.MFAEnabled {
		if strings.TrimSpace(req.Code) == "" {
			writeJSON(w, http.StatusOK, map[string]any{"mfa_required": true})
			return
		}
		if !totpVerify(acc.MFASecret, req.Code) {
			s.auth.loginFailed(ip)
			s.store.AddLog(LogEntry{Kind: "系统", Level: "warning", Actor: ip, Message: "动态验证码错误：" + req.Username})
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "动态验证码错误"})
			return
		}
	}
	tok := s.auth.issueSession(acc.Username)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL / time.Second),
	})
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: ip, Message: "登录成功：" + req.Username})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
	if !s.auth.ValidateRequest(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	a := s.cfg.Account()
	writeJSON(w, http.StatusOK, map[string]any{
		"username": a.Username, "display_name": a.DisplayName, "email": a.Email,
		"mfa_enabled": a.MFAEnabled,
	})
}

func (s *Server) handleSetProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	// Username is optional in the payload; when present it must be valid. Changing
	// it doesn't disturb the active session (sessions key off the token, not name).
	if strings.TrimSpace(req.Username) != "" {
		uname := sanitizeUsername(req.Username)
		if uname == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "用户名仅限字母/数字/-_.，长度 2–32 位"})
			return
		}
		_ = s.cfg.SetUsername(uname)
	}
	_ = s.cfg.SetProfile(strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Email))
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: s.clientIP(r), Message: "更新个人信息"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": s.cfg.Account().Username})
}

func (s *Server) handleSetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	acc := s.cfg.Account()
	if subtle.ConstantTimeCompare([]byte(hashPassword(req.Old, acc.Salt)), []byte(acc.Hash)) != 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "原密码错误"})
		return
	}
	if len(strings.TrimSpace(req.New)) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "新密码至少 4 位"})
		return
	}
	_ = s.cfg.SetPassword(req.New)
	// Invalidate all existing sessions so any old/stolen cookie dies with the old
	// password, then re-issue one for the current operator so they stay logged in.
	s.auth.ClearSessions()
	tok := s.auth.issueSession(acc.Username)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL / time.Second),
	})
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "修改登录密码"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- MFA (TOTP two-factor) ----

// handleMFASetup issues a fresh TOTP secret + provisioning URL for enrollment. It
// does NOT enable MFA — the client must prove one valid code via handleMFAEnable,
// so a mis-scanned secret can never lock the operator out.
func (s *Server) handleMFASetup(w http.ResponseWriter, r *http.Request) {
	secret := genTOTPSecret()
	if secret == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "生成密钥失败"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret":      secret,
		"otpauth_url": otpauthURL(s.cfg.Account().Username, secret),
	})
}

// handleMFAEnable turns MFA on after verifying the user can produce a current code
// for the freshly-issued secret (proves the authenticator app is set up).
func (s *Server) handleMFAEnable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Secret string `json:"secret"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if !totpVerify(req.Secret, req.Code) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "验证码不正确，请确认手机时间已同步后重试"})
		return
	}
	_ = s.cfg.SetMFA(true, strings.TrimSpace(req.Secret))
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "启用两步验证（TOTP）"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleMFADisable turns MFA off after re-verifying the account password, so a
// hijacked-but-unlocked session alone can't strip the second factor.
func (s *Server) handleMFADisable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	acc := s.cfg.Account()
	if subtle.ConstantTimeCompare([]byte(hashPassword(req.Password, acc.Salt)), []byte(acc.Hash)) != 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "密码不正确"})
		return
	}
	_ = s.cfg.SetMFA(false, "")
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "关闭两步验证（TOTP）"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
