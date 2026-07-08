package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// newTestConfigStore builds a ConfigStore backed by a temp file so save() works
// without polluting the repo. The default config creates an admin/admin account.
func newTestConfigStore(t *testing.T) *ConfigStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test_config.json")
	cs, err := NewConfigStore(path)
	if err != nil {
		t.Fatalf("NewConfigStore failed: %v", err)
	}
	return cs
}

func TestNewAuth(t *testing.T) {
	cfg := newTestConfigStore(t)
	a := NewAuth(cfg)
	if a == nil {
		t.Fatal("NewAuth returned nil")
	}
	if a.cfg != cfg {
		t.Error("Auth.cfg not set")
	}
	if len(a.sessions) != 0 {
		t.Errorf("new auth should have no sessions, got %d", len(a.sessions))
	}
}

func TestCheckPassword(t *testing.T) {
	cfg := newTestConfigStore(t)
	a := NewAuth(cfg)
	// default account is admin/admin
	t.Run("correct password", func(t *testing.T) {
		acc, ok := a.CheckPassword("admin", "admin")
		if !ok {
			t.Fatal("CheckPassword rejected correct credentials")
		}
		if acc.Username != "admin" {
			t.Errorf("unexpected username: %s", acc.Username)
		}
	})
	t.Run("incorrect password", func(t *testing.T) {
		_, ok := a.CheckPassword("admin", "wrong")
		if ok {
			t.Error("CheckPassword accepted wrong password")
		}
	})
	t.Run("unknown user", func(t *testing.T) {
		_, ok := a.CheckPassword("ghost", "whatever")
		if ok {
			t.Error("CheckPassword accepted unknown user")
		}
	})
}

func TestLoginBruteForceProtection(t *testing.T) {
	cfg := newTestConfigStore(t)
	a := NewAuth(cfg)
	ip := "10.0.0.1"
	// loginAllowed starts true.
	if !a.loginAllowed(ip) {
		t.Fatal("loginAllowed should be true initially")
	}
	// Record failures up to the limit.
	for i := 0; i < loginMaxFailures; i++ {
		a.loginFailed(ip)
	}
	// After the limit, the IP is throttled.
	if a.loginAllowed(ip) {
		t.Errorf("loginAllowed should be false after %d failures", loginMaxFailures)
	}
}

func TestSessionValidation(t *testing.T) {
	cfg := newTestConfigStore(t)
	a := NewAuth(cfg)
	t.Run("valid token", func(t *testing.T) {
		tok := a.issueSession("admin")
		if !a.validate(tok) {
			t.Error("validate rejected a freshly issued token")
		}
	})
	t.Run("empty token", func(t *testing.T) {
		if a.validate("") {
			t.Error("validate accepted an empty token")
		}
	})
	t.Run("unknown token", func(t *testing.T) {
		if a.validate("nonexistent-token") {
			t.Error("validate accepted an unknown token")
		}
	})
	t.Run("logged out token", func(t *testing.T) {
		tok := a.issueSession("admin")
		a.Logout(tok)
		if a.validate(tok) {
			t.Error("validate accepted a logged-out token")
		}
	})
}

func TestClearSessions(t *testing.T) {
	cfg := newTestConfigStore(t)
	a := NewAuth(cfg)
	a.issueSession("admin")
	a.issueSession("admin")
	a.ClearSessions()
	if len(a.sessions) != 0 {
		t.Errorf("expected no sessions after ClearSessions, got %d", len(a.sessions))
	}
}

func TestClearUserSessions(t *testing.T) {
	cfg := newTestConfigStore(t)
	a := NewAuth(cfg)
	a.issueSession("alice")
	bobTok := a.issueSession("bob")
	a.clearUserSessions("alice")
	// alice's sessions gone, bob's untouched.
	if len(a.sessions) != 1 {
		t.Fatalf("expected 1 session remaining, got %d", len(a.sessions))
	}
	if !a.validate(bobTok) {
		t.Error("bob's session should still be valid")
	}
}

// newRoleRequest builds a request with the given method + path for RBAC checks.
func newRoleRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	return req
}

func TestRouteAllowed(t *testing.T) {
	s := &Server{}
	cases := []struct {
		name   string
		method string
		path   string
		role   string
		want   bool
	}{
		// own-account self-service: any logged-in role
		{"viewer can logout", "POST", "/api/v1/logout", RoleViewer, true},
		{"viewer can change password", "POST", "/api/v1/password", RoleViewer, true},
		{"viewer can setup mfa", "POST", "/api/v1/mfa/setup", RoleViewer, true},
		// reads: viewer+
		{"viewer can read hosts", "GET", "/api/v1/hosts", RoleViewer, true},
		{"viewer can read alerts", "GET", "/api/v1/alerts", RoleViewer, true},
		// writes (non-users): operator+
		{"viewer cannot set category", "POST", "/api/v1/hosts/h1/category", RoleViewer, false},
		{"operator can set category", "POST", "/api/v1/hosts/h1/category", RoleOperator, true},
		{"operator can delete host", "DELETE", "/api/v1/hosts/h1", RoleOperator, true},
		// user management: admin only
		{"operator cannot list users", "GET", "/api/v1/users", RoleOperator, false},
		{"admin can list users", "GET", "/api/v1/users", RoleAdmin, true},
		{"admin can create user", "POST", "/api/v1/users", RoleAdmin, true},
		{"operator cannot delete user", "DELETE", "/api/v1/users/alice", RoleOperator, false},
		{"admin can delete user", "DELETE", "/api/v1/users/alice", RoleAdmin, true},
		// terminal: operator+
		{"viewer cannot open terminal", "GET", "/api/v1/hosts/h1/terminal", RoleViewer, false},
		{"operator can open terminal", "GET", "/api/v1/hosts/h1/terminal", RoleOperator, true},
		// unknown role
		{"unknown role denied", "GET", "/api/v1/hosts", "weird", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := newRoleRequest(tc.method, tc.path)
			if got := s.routeAllowed(req, tc.role); got != tc.want {
				t.Errorf("routeAllowed(%s %s, role=%s) = %v, want %v",
					tc.method, tc.path, tc.role, got, tc.want)
			}
		})
	}
}

func TestValidateRequest(t *testing.T) {
	cfg := newTestConfigStore(t)
	a := NewAuth(cfg)
	t.Run("request with valid cookie", func(t *testing.T) {
		tok := a.issueSession("admin")
		req := newRoleRequest("GET", "/api/v1/hosts")
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
		if !a.ValidateRequest(req) {
			t.Error("ValidateRequest rejected a valid session cookie")
		}
	})
	t.Run("request without cookie", func(t *testing.T) {
		req := newRoleRequest("GET", "/api/v1/hosts")
		if a.ValidateRequest(req) {
			t.Error("ValidateRequest accepted a request without a cookie")
		}
	})
}
