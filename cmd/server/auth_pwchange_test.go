package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestPasswordChangeSessionBlocksAPI(t *testing.T) {
	dir := t.TempDir()
	cfg, err := NewConfigStore(filepath.Join(dir, "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	auth := NewAuth(cfg)
	srv := &Server{cfg: cfg, store: store, auth: auth}

	// Issue a password-change-only session as the default admin would get.
	tok := auth.issuePasswordChangeSession("admin")
	mw := srv.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	// Hosts API must be forbidden.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("hosts with pw-change session: got %d, want 403", rr.Code)
	}

	// /api/v1/me must still work (SPA reads must_change_password).
	req = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
	rr = httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("me with pw-change session: got %d, want 200", rr.Code)
	}
}

func TestCompleteLoginDefaultAdminIssuesPwChangeSession(t *testing.T) {
	dir := t.TempDir()
	cfg, err := NewConfigStore(filepath.Join(dir, "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	auth := NewAuth(cfg)
	srv := &Server{cfg: cfg, store: store, auth: auth}

	acc, ok := cfg.UserByName("admin")
	if !ok {
		t.Fatal("default admin missing")
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", strings.NewReader(`{}`))
	srv.completeLogin(rr, req, acc, "admin", "", "127.0.0.1")
	if rr.Code != http.StatusOK {
		t.Fatalf("login status %d body %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["must_change_password"] != true {
		t.Fatalf("expected must_change_password=true, got %#v", resp)
	}
	cookie := rr.Result().Cookies()
	var sess string
	for _, c := range cookie {
		if c.Name == sessionCookie {
			sess = c.Value
		}
	}
	if sess == "" {
		t.Fatal("missing session cookie")
	}
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	req2.AddCookie(&http.Cookie{Name: sessionCookie, Value: sess})
	if !auth.isPasswordChangeOnly(req2) {
		t.Fatal("session must be password-change-only")
	}
}

func TestAPIRateLimitSkipAgentPaths(t *testing.T) {
	if !apiRateLimitSkip("/api/v1/agent/report") {
		t.Fatal("agent report must skip global rate limit")
	}
	if apiRateLimitSkip("/api/v1/hosts") {
		t.Fatal("hosts must be rate-limited")
	}
}
