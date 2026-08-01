package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPTrustProxyHeaders(t *testing.T) {
	srv := &Server{cfg: &ConfigStore{cfg: ServerConfig{TrustProxy: true}}}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.97.1:12345" // Docker bridge gateway
	req.Header.Set("X-Real-IP", "10.20.30.40")
	if got := srv.clientIP(req); got != "10.20.30.40" {
		t.Fatalf("X-Real-IP: got %q", got)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "192.168.97.1:12345"
	req2.Header.Set("X-Forwarded-For", "203.0.113.9, 192.168.97.1")
	if got := srv.clientIP(req2); got != "203.0.113.9" {
		t.Fatalf("X-Forwarded-For: got %q", got)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.RemoteAddr = "192.168.97.1:12345"
	req3.Header.Set("CF-Connecting-IP", "198.51.100.7")
	if got := srv.clientIP(req3); got != "198.51.100.7" {
		t.Fatalf("CF-Connecting-IP: got %q", got)
	}

	// Forged CF-Connecting-IP must not beat proxy-injected X-Real-IP (hostproxy / nginx).
	req4 := httptest.NewRequest(http.MethodGet, "/", nil)
	req4.RemoteAddr = "192.168.97.1:12345"
	req4.Header.Set("X-Real-IP", "10.20.30.40")
	req4.Header.Set("CF-Connecting-IP", "198.51.100.7")
	if got := srv.clientIP(req4); got != "10.20.30.40" {
		t.Fatalf("X-Real-IP must win over forged CF-Connecting-IP, got %q", got)
	}
}

func TestClientIPIgnoresHeadersWithoutTrustProxy(t *testing.T) {
	srv := &Server{cfg: &ConfigStore{cfg: ServerConfig{TrustProxy: false}}}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.97.1:12345"
	req.Header.Set("X-Real-IP", "10.20.30.40")
	if got := srv.clientIP(req); got != "192.168.97.1" {
		t.Fatalf("want gateway RemoteAddr, got %q", got)
	}
}

func TestSanitizeClientIP(t *testing.T) {
	if got := sanitizeClientIP("10.0.0.1:8080"); got != "10.0.0.1" {
		t.Fatalf("got %q", got)
	}
	if got := sanitizeClientIP("::1"); got != "127.0.0.1" {
		t.Fatalf("got %q", got)
	}
	if got := sanitizeClientIP("not-an-ip"); got != "" {
		t.Fatalf("got %q", got)
	}
}
