package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func builtinIDs(fs []WebFinding) map[string]WebFinding {
	out := make(map[string]WebFinding, len(fs))
	for _, f := range fs {
		out[strings.TrimPrefix(f.TemplateID, "builtin/")] = f
	}
	return out
}

// TestBuiltinChecksFlagBareTarget covers the "no Nuclei, no templates" path: a
// plain HTTP app with no hardening at all must still yield the transport /
// header / cookie / method findings a commercial DAST is expected to report.
func TestBuiltinChecksFlagBareTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.Header().Set("Allow", "GET, POST, PUT, TRACE, OPTIONS")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Server", "nginx/1.18.0")
		w.Header().Set("Set-Cookie", "SESSIONID=abc; Path=/")
		w.Write([]byte("<html><body>hello</body></html>"))
	}))
	defer srv.Close()

	got := builtinIDs(runBuiltinWebChecks(context.Background(),
		WebScanTarget{BaseURL: srv.URL}, true, nil))

	for _, id := range []string{
		"plaintext-http", "missing-csp", "missing-xcto", "missing-referrer-policy",
		"clickjacking", "cookie-no-httponly", "cookie-no-samesite",
		"version-disclosure", "risky-http-methods",
	} {
		if _, ok := got[id]; !ok {
			t.Errorf("expected built-in finding %q, got %v", id, keysOf(got))
		}
	}
	// HSTS only makes sense over TLS — flagging it on http:// would be noise.
	if _, ok := got["missing-hsts"]; ok {
		t.Error("missing-hsts must not be reported for a plain-HTTP target")
	}
	// The Secure cookie flag is likewise only meaningful over HTTPS.
	if _, ok := got["cookie-no-secure"]; ok {
		t.Error("cookie-no-secure must not be reported for a plain-HTTP target")
	}
	if f := got["risky-http-methods"]; f.Severity != "high" {
		t.Errorf("PUT exposed: severity = %q, want high", f.Severity)
	}
}

// TestBuiltinChecksQuietOnHardenedTarget is the false-positive guard: a target
// that sets the expected headers must not be flagged for them.
func TestBuiltinChecksQuietOnHardenedTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=()")
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	got := builtinIDs(runBuiltinWebChecks(context.Background(),
		WebScanTarget{BaseURL: srv.URL}, true, nil))
	for _, id := range []string{
		"missing-csp", "missing-xcto", "missing-referrer-policy",
		"missing-permissions-policy", "clickjacking", "version-disclosure",
		"cookie-no-httponly", "risky-http-methods",
	} {
		if _, ok := got[id]; ok {
			t.Errorf("hardened target wrongly flagged with %q", id)
		}
	}
}

// TestBuiltinExposedPathsIgnoreSPAFallback locks the check that keeps the
// sensitive-path probe usable on single-page apps: an app that answers every
// unknown path with its HTML shell must not be reported as leaking .git/.env.
func TestBuiltinExposedPathsIgnoreSPAFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<html><head><title>App</title></head><body>ref: x = 1</body></html>"))
	}))
	defer srv.Close()

	got := builtinIDs(runBuiltinWebChecks(context.Background(),
		WebScanTarget{BaseURL: srv.URL}, true, nil))
	for _, id := range []string{"git-exposed", "env-exposed", "svn-exposed", "ds-store"} {
		if _, ok := got[id]; ok {
			t.Errorf("SPA fallback wrongly reported as %q", id)
		}
	}
}

// TestBuiltinExposedPathsDetectRealLeak is the positive counterpart: a genuinely
// served .git/HEAD and .env must be reported as critical.
func TestBuiltinExposedPathsDetectRealLeak(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.URL.Path {
		case "/.git/HEAD":
			w.Write([]byte("ref: refs/heads/main\n"))
		case "/.env":
			w.Write([]byte("DB_PASSWORD=s3cret\nAPI_KEY=abc\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	got := builtinIDs(runBuiltinWebChecks(context.Background(),
		WebScanTarget{BaseURL: srv.URL}, true, nil))
	for _, id := range []string{"git-exposed", "env-exposed"} {
		f, ok := got[id]
		if !ok {
			t.Fatalf("missing finding %q, got %v", id, keysOf(got))
		}
		if f.Severity != "critical" {
			t.Errorf("%s: severity = %q, want critical", id, f.Severity)
		}
	}
}

// TestBuiltinCORSReflectionWithCredentials covers the highest-impact CORS
// misconfiguration: reflecting an arbitrary Origin while allowing credentials.
func TestBuiltinCORSReflectionWithCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	got := builtinIDs(runBuiltinWebChecks(context.Background(),
		WebScanTarget{BaseURL: srv.URL}, true, nil))
	f, ok := got["cors-reflect-credentials"]
	if !ok {
		t.Fatalf("origin reflection with credentials not reported, got %v", keysOf(got))
	}
	if f.Severity != "high" {
		t.Errorf("severity = %q, want high", f.Severity)
	}
}

// TestBuiltinChecksRefusePrivateTargets guards the SSRF boundary: without
// allow_private the built-in engine must not touch internal addresses.
func TestBuiltinChecksRefusePrivateTargets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("built-in checks reached a private target with allow_private=false")
	}))
	defer srv.Close()

	out := runBuiltinWebChecks(context.Background(), WebScanTarget{BaseURL: srv.URL}, false, nil)
	if len(out) != 1 || !strings.HasSuffix(out[0].TemplateID, "unreachable") {
		t.Fatalf("want a single unreachable finding, got %+v", out)
	}
}

func keysOf(m map[string]WebFinding) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
