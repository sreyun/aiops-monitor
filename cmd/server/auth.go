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

// Auth manages login sessions (in memory) against the account in ConfigStore.
type Auth struct {
	cfg      *ConfigStore
	mu       sync.Mutex
	sessions map[string]session
}

func NewAuth(cfg *ConfigStore) *Auth {
	return &Auth{cfg: cfg, sessions: map[string]session{}}
}

func newSessionToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}

// Login verifies credentials and returns a fresh session token.
func (a *Auth) Login(user, pass string) (string, bool) {
	acc := a.cfg.Account()
	if user != acc.Username ||
		subtle.ConstantTimeCompare([]byte(hashPassword(pass, acc.Salt)), []byte(acc.Hash)) != 1 {
		return "", false
	}
	tok := newSessionToken()
	a.mu.Lock()
	a.sessions[tok] = session{user: user, expires: time.Now().Add(sessionTTL)}
	a.mu.Unlock()
	return tok, true
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
	a.mu.Unlock()
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
		"/api/v1/agent/register", "/api/v1/agent/report":
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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	tok, ok := s.auth.Login(req.Username, req.Password)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "用户名或密码错误"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL / time.Second),
	})
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: clientIP(r), Message: "登录成功：" + req.Username})
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
	})
}

func (s *Server) handleSetProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	_ = s.cfg.SetProfile(strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Email))
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: clientIP(r), Message: "更新个人信息"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: clientIP(r), Message: "修改登录密码"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
