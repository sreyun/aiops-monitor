package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeadersAllowDesktopBlobURLs(t *testing.T) {
	handler := securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("missing Content-Security-Policy")
	}
	// Remote desktop draws JPEG via createObjectURL / createImageBitmap and plays
	// H.264 via MediaSource blob URLs. Without blob: those loads fail under CSP.
	for _, need := range []string{"img-src", "blob:", "media-src"} {
		if !strings.Contains(csp, need) {
			t.Fatalf("CSP missing %q: %s", need, csp)
		}
	}
	if !strings.Contains(csp, "img-src 'self' data: blob:") {
		t.Fatalf("img-src must allow blob: for desktop JPEG frames, got: %s", csp)
	}
	if !strings.Contains(csp, "media-src 'self' blob:") {
		t.Fatalf("media-src must allow blob: for desktop H.264, got: %s", csp)
	}
}

func TestSecurityHeadersSkipProxy(t *testing.T) {
	handler := securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/proxy/foo", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if csp := rec.Header().Get("Content-Security-Policy"); csp != "" {
		t.Fatalf("proxy responses must not get app CSP, got %q", csp)
	}
}
