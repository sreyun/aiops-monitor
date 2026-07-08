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
	user       string
	expires    time.Time
	restricted bool // true = only MFA setup/enable/logout endpoints allowed (global MFA enforcement)
}

// Auth manages login sessions against the account in ConfigStore. Sessions
// are persisted through the embedded DB so a server restart keeps everyone
// logged in.
type Auth struct {
	cfg      *ConfigStore
	mu       sync.Mutex
	sessions map[string]session
	dirty    bool

	proxyTokens   map[string]proxyToken // ephemeral tokens for HTTP proxy auth
	proxyTokenMu  sync.Mutex

	limMu    sync.Mutex
	loginHit map[string][]int64 // client ip -> recent FAILED attempt unix times
}

const (
	loginWindowSec   = 300 // sliding window for brute-force throttling
	loginMaxFailures = 8   // max failed attempts per IP per window
	proxyTokenTTL    = 60 * time.Second // HTTP proxy token lifetime
)

type proxyToken struct {
	user    string
	expires time.Time
}

// generateProxyToken creates a short-lived token for HTTP proxy URL auth.
func (a *Auth) generateProxyToken(user string) string {
	tok := newSessionToken()
	a.proxyTokenMu.Lock()
	a.proxyTokens[tok] = proxyToken{user: user, expires: time.Now().Add(proxyTokenTTL)}
	a.proxyTokenMu.Unlock()
	return tok
}

// validateProxyToken returns the username if the token is valid, or empty string.
func (a *Auth) validateProxyToken(tok string) string {
	a.proxyTokenMu.Lock()
	pt, ok := a.proxyTokens[tok]
	delete(a.proxyTokens, tok) // single-use
	a.proxyTokenMu.Unlock()
	if !ok || time.Now().After(pt.expires) {
		return ""
	}
	return pt.user
}

func NewAuth(cfg *ConfigStore) *Auth {
	return &Auth{cfg: cfg, sessions: map[string]session{}, proxyTokens: map[string]proxyToken{}, loginHit: map[string][]int64{}}
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

// CheckPassword verifies user+pass against the users table and returns the
// matched account. It does NOT issue a session — a second factor (TOTP) may
// still be required; the caller issues the session via issueSession. A dummy
// hash runs when the username is unknown to blunt enumeration timing.
func (a *Auth) CheckPassword(user, pass string) (AccountConfig, bool) {
	acc, found := a.cfg.UserByName(user)
	if !found {
		hashPassword(pass, "dummy-salt-000000") // constant-ish timing
		return AccountConfig{}, false
	}
	if subtle.ConstantTimeCompare([]byte(hashPassword(pass, acc.Salt)), []byte(acc.Hash)) != 1 {
		return AccountConfig{}, false
	}
	return acc, true
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

// issueRestrictedSession creates a session that can only access MFA enrollment
// endpoints (/api/v1/mfa/setup, /api/v1/mfa/enable) and logout. Used when the
// global MFA policy forces a user to bind TOTP before they can use the system.
func (a *Auth) issueRestrictedSession(user string) string {
	tok := newSessionToken()
	a.mu.Lock()
	a.sessions[tok] = session{user: user, expires: time.Now().Add(sessionTTL), restricted: true}
	a.dirty = true
	a.mu.Unlock()
	return tok
}

// isRestricted reports whether the current session is a restricted (MFA-enrollment-only) session.
func (a *Auth) isRestricted(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[c.Value]
	if !ok || time.Now().After(s.expires) {
		return false
	}
	return s.restricted
}

// upgradeSession lifts a restricted session to a full session (called after MFA enrollment).
func (a *Auth) upgradeSession(r *http.Request) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if s, ok := a.sessions[c.Value]; ok {
		s.restricted = false
		a.sessions[c.Value] = s
		a.dirty = true
	}
}

// userForRequest returns the username bound to a valid (unexpired) session
// cookie, or "" if there is none.
func (a *Auth) userForRequest(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[c.Value]
	if !ok || time.Now().After(s.expires) {
		return ""
	}
	return s.user
}

// clearUserSessions invalidates all sessions for one user — used after that user
// changes or resets their own password (doesn't disturb other users).
func (a *Auth) clearUserSessions(name string) {
	a.mu.Lock()
	for tok, s := range a.sessions {
		if s.user == name {
			delete(a.sessions, tok)
		}
	}
	a.dirty = true
	a.mu.Unlock()
}

// renameSessions repoints a user's active sessions to a new username after a
// self rename, so the current cookie keeps working.
func (a *Auth) renameSessions(oldName, newName string) {
	a.mu.Lock()
	for tok, s := range a.sessions {
		if s.user == oldName {
			s.user = newName
			a.sessions[tok] = s
		}
	}
	a.dirty = true
	a.mu.Unlock()
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
	case "/", "/healthz", "/style.css", "/app.js", "/i18n-dashboard.js",
		"/install.sh", "/install.ps1", "/uninstall.sh", "/uninstall.ps1",
		"/api/v1/login", "/api/v1/me",
		"/api/v1/forward/health",
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

// routeAllowed enforces RBAC: any logged-in role may manage its own account;
// viewer is otherwise read-only; the remote terminal needs operator+; user
// management needs admin; every other write needs operator+.
func (s *Server) routeAllowed(r *http.Request, role string) bool {
	rank := roleRank(role)
	if rank == 0 {
		return false
	}
	p := r.URL.Path
	switch p { // own-account self-service: any logged-in role
	case "/api/v1/logout", "/api/v1/password", "/api/v1/profile",
		"/api/v1/mfa/setup", "/api/v1/mfa/enable", "/api/v1/mfa/disable",
		"/api/v1/mfa/unbind-via-email":
		return true
	}
	if strings.HasPrefix(p, "/api/v1/users") || p == "/api/v1/mfa/global" { // user management + global MFA: admin only
		return rank >= roleRank(RoleAdmin)
	}
	if strings.Contains(p, "/terminal") || strings.HasPrefix(p, "/api/v1/forward") || strings.HasPrefix(p, "/proxy/") { // remote shell + port forwarding: operator+
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
		if isPublicPath(r) {
			next.ServeHTTP(w, r)
			return
		}
		// HTTP proxy token auth: allows window.open in new tab without relying on
		// the session cookie (which may not be sent cross-context in some browsers).
		if strings.HasPrefix(r.URL.Path, "/proxy/") {
			if tok := r.URL.Query().Get("pt"); tok != "" {
				if s.auth.validateProxyToken(tok) != "" {
					next.ServeHTTP(w, r)
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
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"` // TOTP second factor (only when MFA is enabled)
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
	acc, ok := s.auth.CheckPassword(req.Username, req.Password)
	if !ok {
		s.auth.loginFailed(ip)
		s.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: ip, Message: Tz("log.login_failed", req.Username)})
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.invalid_credentials")})
		return
	}
	// Password OK. If MFA is on, require a valid TOTP code as the second factor.
	// The requirement is revealed only AFTER the password checks out, so an
	// unauthenticated prober can't learn whether the account has MFA enabled.
	if acc.MFAEnabled {
		if strings.TrimSpace(req.Code) == "" {
			writeJSON(w, http.StatusOK, map[string]any{"mfa_required": true})
			return
		}
		if !totpVerify(acc.MFASecret, req.Code) {
			s.auth.loginFailed(ip)
			s.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: ip, Message: Tz("log.totp_failed", req.Username)})
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.totp_error")})
			return
		}
	}
	// Global MFA policy: if admin has enabled MFARequired and this user hasn't
	// set up MFA yet, issue a restricted session and direct them to enroll.
	if s.cfg.MFARequired() && !acc.MFAEnabled {
		tok := s.auth.issueRestrictedSession(acc.Username)
		http.SetCookie(w, &http.Cookie{
			Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true,
			Secure:   isHTTPS(r),
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
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL / time.Second),
	})
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: ip, Message: Tz("log.login_success", req.Username)})
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
	acc, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"username": acc.Username, "display_name": acc.DisplayName, "email": acc.Email,
		"mfa_enabled": acc.MFAEnabled, "role": acc.Role,
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
	_ = s.cfg.SetUserProfile(name, strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Email))
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: Tz("log.update_profile", name)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": name})
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
	if subtle.ConstantTimeCompare([]byte(hashPassword(req.Old, acc.Salt)), []byte(acc.Hash)) != 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "auth.wrong_old_password")})
		return
	}
	if len(strings.TrimSpace(req.New)) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "auth.password_too_short")})
		return
	}
	_ = s.cfg.SetUserPassword(acc.Username, req.New)
	// Invalidate only THIS user's other sessions, then re-issue one for the current.
	s.auth.clearUserSessions(acc.Username)
	tok := s.auth.issueSession(acc.Username)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL / time.Second),
	})
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.change_password", acc.Username)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.enable_mfa", acc.Username)})
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
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: action})
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
	if subtle.ConstantTimeCompare([]byte(hashPassword(req.Password, acc.Salt)), []byte(acc.Hash)) != 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "auth.wrong_password")})
		return
	}
	_ = s.cfg.SetUserMFA(acc.Username, false, "")
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.disable_mfa", acc.Username)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
