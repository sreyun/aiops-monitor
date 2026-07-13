package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookie = "aiops_session"
	sessionTTL    = 7 * 24 * time.Hour
)

// Password hashing — PBKDF2-HMAC-SHA256 (stdlib only, no external deps).
//
// Migrated from single-round salted SHA-256 (a fast hash trivially brute-forced
// on GPUs) to PBKDF2 with a high iteration count. Stored hashes are
// self-describing ("pbkdf2$sha256$<iter>$<hex>"); legacy 64-hex SHA-256 hashes
// are still accepted by verifyPassword and transparently upgraded on next login.
const pbkdf2Iter = 600000 // OWASP 2023 guidance for PBKDF2-HMAC-SHA256

// pbkdf2SHA256 implements PBKDF2 (RFC 8018 §5.2) over HMAC-SHA256.
func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hLen := prf.Size()
	numBlocks := (keyLen + hLen - 1) / hLen
	dk := make([]byte, 0, numBlocks*hLen)
	var idx [4]byte
	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(idx[:], uint32(block))
		prf.Write(idx[:])
		u := prf.Sum(nil)
		t := make([]byte, len(u))
		copy(t, u)
		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(nil)
			for i := range t {
				t[i] ^= u[i]
			}
		}
		dk = append(dk, t...)
	}
	return dk[:keyLen]
}

// hashPassword returns a PBKDF2-HMAC-SHA256 hash of pass, salted with salt. The
// result is self-describing so the iteration count can evolve over time.
func hashPassword(pass, salt string) string {
	dk := pbkdf2SHA256([]byte(pass), []byte(salt), pbkdf2Iter, 32)
	return "pbkdf2$sha256$" + strconv.Itoa(pbkdf2Iter) + "$" + hex.EncodeToString(dk)
}

// verifyPassword reports, in constant time, whether pass matches the stored hash
// for the given salt. It accepts both the current PBKDF2 format and the legacy
// single-round salted SHA-256 (bare 64-hex) format for a seamless migration.
func verifyPassword(pass, salt, stored string) bool {
	if strings.HasPrefix(stored, "pbkdf2$") {
		parts := strings.Split(stored, "$")
		if len(parts) != 4 || parts[1] != "sha256" {
			return false
		}
		iter, err := strconv.Atoi(parts[2])
		if err != nil || iter < 1 {
			return false
		}
		want, err := hex.DecodeString(parts[3])
		if err != nil || len(want) == 0 {
			return false
		}
		got := pbkdf2SHA256([]byte(pass), []byte(salt), iter, len(want))
		return subtle.ConstantTimeCompare(got, want) == 1
	}
	// Legacy salted SHA-256 (hex).
	sum := sha256.Sum256([]byte(salt + ":" + pass))
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(stored)) == 1
}

// isLegacyHash reports whether stored is an old single-round SHA-256 hash that
// should be upgraded to PBKDF2 on the next successful authentication.
func isLegacyHash(stored string) bool {
	return stored != "" && !strings.HasPrefix(stored, "pbkdf2$")
}

type session struct {
	user       string
	expires    time.Time
	restricted bool // true = only MFA setup/enable/logout endpoints allowed (global MFA enforcement)
	// v5.3.0: terminal secondary verification — true once verified in this session.
	terminalVerified bool
	// v5.5.0: last activity time — drives the sliding idle timeout (absolute cap
	// stays expires). Not persisted; imported sessions start fresh on restart.
	lastSeen time.Time
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

	// v5.3.0: terminal verification rate limiting
	termAttemptMu sync.Mutex
	termAttempts  map[string]int      // user -> consecutive failed terminal verify attempts
	termLocked    map[string]time.Time // user -> locked until

	// v5.5.0: per-account login throttle (independent of IP, blunts IP-rotating
	// distributed brute force) — shares limMu with loginHit.
	loginHitUser map[string][]int64 // username -> recent failed attempt unix times

	// v5.5.0: TOTP single-use — a code (by time-step) accepted once for a user
	// can't be replayed within the skew window.
	totpMu   sync.Mutex
	totpUsed map[string]int64 // "user:step" -> expiry unix
}

const (
	loginWindowSec   = 300 // sliding window for brute-force throttling
	loginMaxFailures = 8   // max failed attempts per IP per window
	proxyTokenTTL    = 60 * time.Second // HTTP proxy token lifetime

	// v5.3.0: terminal verification rate limiting
	termMaxAttempts = 3                // max failed terminal verify attempts
	termLockoutSec  = 300              // lockout duration (5 minutes)

	// v5.5.0: per-account login throttle + session idle expiry
	loginAccountWindowSec = 900             // 15-min sliding window per account
	loginAccountMaxFail   = 10              // max failed attempts per ACCOUNT per window
	sessionIdleTimeout    = 24 * time.Hour  // sliding idle expiry (absolute cap stays sessionTTL)
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
	return &Auth{cfg: cfg, sessions: map[string]session{}, proxyTokens: map[string]proxyToken{}, loginHit: map[string][]int64{}, loginHitUser: map[string][]int64{}, totpUsed: map[string]int64{}, termAttempts: map[string]int{}, termLocked: map[string]time.Time{}}
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

// loginAccountAllowed reports whether the account is under the per-account failed-
// attempt threshold. Independent of IP, so a botnet rotating source addresses
// can't exceed loginAccountMaxFail against one account per window. Case-insensitive.
func (a *Auth) loginAccountAllowed(user string) bool {
	key := strings.ToLower(strings.TrimSpace(user))
	if key == "" {
		return true
	}
	now := time.Now().Unix()
	cutoff := now - loginAccountWindowSec
	a.limMu.Lock()
	defer a.limMu.Unlock()
	if len(a.loginHitUser) > 4096 { // safety valve against unbounded growth
		a.loginHitUser = map[string][]int64{}
	}
	kept := a.loginHitUser[key][:0]
	for _, t := range a.loginHitUser[key] {
		if t >= cutoff {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(a.loginHitUser, key)
	} else {
		a.loginHitUser[key] = kept
	}
	return len(kept) < loginAccountMaxFail
}

// loginAccountFailed records one failed attempt against an account.
func (a *Auth) loginAccountFailed(user string) {
	key := strings.ToLower(strings.TrimSpace(user))
	if key == "" {
		return
	}
	now := time.Now().Unix()
	a.limMu.Lock()
	a.loginHitUser[key] = append(a.loginHitUser[key], now)
	a.limMu.Unlock()
}

// loginAccountReset clears an account's failed-attempt history after a success.
func (a *Auth) loginAccountReset(user string) {
	a.limMu.Lock()
	delete(a.loginHitUser, strings.ToLower(strings.TrimSpace(user)))
	a.limMu.Unlock()
}

// verifyTOTPOnce verifies a TOTP code AND enforces single-use: a code (identified
// by its 30s time-step) accepted once for a user can't be replayed within the
// ±1-step skew window. Blunts reuse of a phished/observed code across login,
// account recovery and terminal-password changes.
func (a *Auth) verifyTOTPOnce(user, secret, code string) bool {
	step, ok := totpMatchStep(secret, code)
	if !ok {
		return false
	}
	now := time.Now().Unix()
	key := strings.ToLower(user) + ":" + strconv.FormatInt(step, 10)
	a.totpMu.Lock()
	defer a.totpMu.Unlock()
	for k, exp := range a.totpUsed { // prune expired entries
		if now > exp {
			delete(a.totpUsed, k)
		}
	}
	if _, used := a.totpUsed[key]; used {
		return false // replay within the skew window
	}
	a.totpUsed[key] = now + 2*totpPeriod
	return true
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
		hashPassword(pass, "dummy-salt-000000") // constant-ish timing (runs the full KDF)
		return AccountConfig{}, false
	}
	if !verifyPassword(pass, acc.Salt, acc.Hash) {
		return AccountConfig{}, false
	}
	// Transparently upgrade a legacy SHA-256 hash to PBKDF2 now that we hold the
	// verified plaintext — existing users get the stronger hash without any
	// action on their part.
	if isLegacyHash(acc.Hash) {
		if err := a.cfg.upgradeLoginHash(user, pass); err == nil {
			if u, ok := a.cfg.UserByName(user); ok {
				acc = u
			}
		}
	}
	return acc, true
}

// sessionKey maps a raw session token to its storage key. Sessions are indexed
// by the SHA-256 of the token so neither memory nor the persisted DB ever holds
// a usable token — a leaked snapshot can't be replayed to hijack a session.
func sessionKey(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

func (a *Auth) validate(tok string) bool {
	if tok == "" {
		return false
	}
	key := sessionKey(tok)
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[key]
	if !ok {
		return false
	}
	now := time.Now()
	// Absolute expiry (hard cap) OR sliding idle timeout — either invalidates.
	// A zero lastSeen (e.g. a session imported before this field existed) is
	// treated as active so a restart doesn't force everyone to re-login.
	if now.After(s.expires) || (!s.lastSeen.IsZero() && now.Sub(s.lastSeen) > sessionIdleTimeout) {
		delete(a.sessions, key)
		a.dirty = true
		return false
	}
	s.lastSeen = now // slide the idle window on activity
	a.sessions[key] = s
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
	delete(a.sessions, sessionKey(tok))
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
	now := time.Now()
	a.mu.Lock()
	a.sessions[sessionKey(tok)] = session{user: user, expires: now.Add(sessionTTL), lastSeen: now}
	a.dirty = true
	a.mu.Unlock()
	return tok
}

// issueRestrictedSession creates a session that can only access MFA enrollment
// endpoints (/api/v1/mfa/setup, /api/v1/mfa/enable) and logout. Used when the
// global MFA policy forces a user to bind TOTP before they can use the system.
func (a *Auth) issueRestrictedSession(user string) string {
	tok := newSessionToken()
	now := time.Now()
	a.mu.Lock()
	a.sessions[sessionKey(tok)] = session{user: user, expires: now.Add(sessionTTL), restricted: true, lastSeen: now}
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
	s, ok := a.sessions[sessionKey(c.Value)]
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
	if s, ok := a.sessions[sessionKey(c.Value)]; ok {
		s.restricted = false
		a.sessions[sessionKey(c.Value)] = s
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
	s, ok := a.sessions[sessionKey(c.Value)]
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
			out[tok] = dbSession{User: s.user, Expires: s.expires.Unix(), TerminalVerified: s.terminalVerified, Restricted: s.restricted}
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
			a.sessions[tok] = session{user: s.User, expires: exp, terminalVerified: s.TerminalVerified, restricted: s.Restricted, lastSeen: now}
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
		"/api/v1/agent/logs": // fingerprint-gated log ingest (checked in the handler)
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
// management needs admin; every other write needs operator+.
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
	if strings.HasPrefix(p, "/api/v1/users") || p == "/api/v1/mfa/global" { // user management + global MFA: admin only
		return rank >= roleRank(RoleAdmin)
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
					s.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.relay_secret_mismatch")})
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

// isTerminalVerified reports whether the current session has passed terminal
// secondary verification. Returns false if the user has no terminal password
// set (which means verification is not required).
func (a *Auth) isTerminalVerified(r *http.Request) (verified bool, hasPassword bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false, false
	}
	name := a.userForRequest(r)
	if name == "" {
		return false, false
	}
	hasPassword = a.cfg.HasTerminalPassword(name)
	if !hasPassword {
		return true, false // no password set → no verification needed
	}
	a.mu.Lock()
	s, ok := a.sessions[sessionKey(c.Value)]
	a.mu.Unlock()
	if !ok {
		return false, true
	}
	return s.terminalVerified, true
}

// markTerminalVerified marks the current session as terminal-verified.
func (a *Auth) markTerminalVerified(r *http.Request) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return
	}
	a.mu.Lock()
	if s, ok := a.sessions[sessionKey(c.Value)]; ok {
		s.terminalVerified = true
		a.sessions[sessionKey(c.Value)] = s
		a.dirty = true
	}
	a.mu.Unlock()
}

// terminalAttemptAllowed checks rate limiting for terminal password verification.
// Returns (allowed, remaining attempts).
func (a *Auth) terminalAttemptAllowed(user string) (bool, int) {
	a.termAttemptMu.Lock()
	defer a.termAttemptMu.Unlock()
	if lockedUntil, ok := a.termLocked[user]; ok && time.Now().Before(lockedUntil) {
		return false, 0
	}
	attempts := a.termAttempts[user]
	if attempts >= termMaxAttempts {
		a.termLocked[user] = time.Now().Add(termLockoutSec * time.Second)
		delete(a.termAttempts, user)
		return false, 0
	}
	return true, termMaxAttempts - attempts
}

// terminalAttemptFailed records a failed terminal password attempt.
func (a *Auth) terminalAttemptFailed(user string) {
	a.termAttemptMu.Lock()
	a.termAttempts[user]++
	a.termAttemptMu.Unlock()
}

// terminalAttemptReset clears the attempt counter (on success or explicit reset).
func (a *Auth) terminalAttemptReset(user string) {
	a.termAttemptMu.Lock()
	delete(a.termAttempts, user)
	delete(a.termLocked, user)
	a.termAttemptMu.Unlock()
}

// ---- auth handlers ----

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		LoginType string `json:"login_type"` // "username" (default), "phone", "sms"
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
	// Resolve account by login type
	var acc AccountConfig
	var ok bool
	throttleKey := req.Username
	switch req.LoginType {
	case "phone":
		throttleKey = req.Username // phone number is in username field
		acc, ok = s.cfg.UserByPhone(req.Username)
		if !ok {
			// Dummy hash to blunt enumeration timing
			hashPassword(req.Password, "dummy-salt-000000")
		}
	case "sms":
		// Reserved: SMS verification code login — not yet implemented
		writeJSON(w, http.StatusOK, map[string]any{"sms_required": true, "message": Tr(r, "login.sms_not_available")})
		return
	default:
		// Default: username login
		throttleKey = req.Username
		acc, ok = s.auth.CheckPassword(req.Username, req.Password)
		if !ok {
			s.auth.loginFailed(ip)
			s.auth.loginAccountFailed(req.Username)
			s.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: ip, Message: Tz("log.login_failed", req.Username)})
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.invalid_credentials")})
			return
		}
		goto passwordOK
	}

	// Phone login: validate password ourselves (since CheckPassword uses UserByName)
	if req.LoginType == "phone" {
		if !ok {
			s.auth.loginFailed(ip)
			s.auth.loginAccountFailed(throttleKey)
			s.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: ip, Message: Tz("log.login_failed", "phone:"+req.Username)})
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.invalid_credentials")})
			return
		}
		if !verifyPassword(req.Password, acc.Salt, acc.Hash) {
			s.auth.loginFailed(ip)
			s.auth.loginAccountFailed(throttleKey)
			s.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: ip, Message: Tz("log.login_failed", acc.Username)})
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.invalid_credentials")})
			return
		}
		// Per-account throttle: use the resolved username
		if !s.auth.loginAccountAllowed(acc.Username) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": Tr(r, "auth.too_many_attempts")})
			return
		}
	}

passwordOK:
	// v5.4.0: detect default admin/admin credentials and force a password
	// change on first login. Do not override an existing MustChangePassword
	// flag (it may already be set by the admin reset tool).
	if !acc.MustChangePassword && acc.Username == "admin" && req.Password == "admin" {
		s.cfg.SetMustChangePassword(acc.Username)
		acc.MustChangePassword = true
		s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: ip, Message: Tz("log.default_credentials", acc.Username)})
	}
	// Password OK. If MFA is on, require a valid TOTP code as the second factor.
	// The requirement is revealed only AFTER the password checks out, so an
	// unauthenticated prober can't learn whether the account has MFA enabled.
	if acc.MFAEnabled {
		if strings.TrimSpace(req.Code) == "" {
			writeJSON(w, http.StatusOK, map[string]any{"mfa_required": true})
			return
		}
		if !s.auth.verifyTOTPOnce(acc.Username, acc.MFASecret, req.Code) {
			s.auth.loginFailed(ip)
			s.auth.loginAccountFailed(acc.Username)
			s.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: ip, Message: Tz("log.totp_failed", acc.Username)})
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
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: ip, Message: Tz("log.login_success", acc.Username)})
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
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: Tz("log.update_profile", name)})
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
	s.store.AddLog(LogEntry{Kind: KindSystem, Level: "info", Actor: s.clientIP(r), Message: fmt.Sprintf("SMS code sent to %s (placeholder)", phone)})
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
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL / time.Second),
	})
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.change_password", acc.Username)})
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
		Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.change_password", name)})
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
	if !verifyPassword(req.Password, acc.Salt, acc.Hash) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "auth.wrong_password")})
		return
	}
	_ = s.cfg.SetUserMFA(acc.Username, false, "")
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.disable_mfa", acc.Username)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
