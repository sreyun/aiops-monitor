package main

import (
	"net/http"
	"testing"
)

func TestRewriteProxyHeadersStripsForgedClientIP(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:18529/api/v1/login", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "127.0.0.1:8529"
	req.RemoteAddr = "203.0.113.10:54321"
	req.Header.Set("CF-Connecting-IP", "198.51.100.7") // attacker forge
	req.Header.Set("True-Client-IP", "198.51.100.8")
	req.Header.Set("X-Real-IP", "198.51.100.9")
	req.Header.Set("X-Forwarded-For", "198.51.100.10, 10.0.0.1")
	req.Header.Set("X-Forwarded-Proto", "https") // must not stick on plain HTTP

	rewriteProxyHeaders(req, "127.0.0.1:8529")

	if req.Host != "127.0.0.1:8529" {
		t.Fatalf("Host: got %q", req.Host)
	}
	if got := req.Header.Get("X-Forwarded-Host"); got != "127.0.0.1:8529" {
		t.Fatalf("X-Forwarded-Host: got %q", got)
	}
	if got := req.Header.Get("CF-Connecting-IP"); got != "" {
		t.Fatalf("CF-Connecting-IP must be stripped, got %q", got)
	}
	if got := req.Header.Get("True-Client-IP"); got != "" {
		t.Fatalf("True-Client-IP must be stripped, got %q", got)
	}
	if got := req.Header.Get("X-Real-IP"); got != "203.0.113.10" {
		t.Fatalf("X-Real-IP: got %q want peer", got)
	}
	if got := req.Header.Get("X-Forwarded-For"); got != "203.0.113.10" {
		t.Fatalf("X-Forwarded-For: got %q want peer only", got)
	}
	if got := req.Header.Get("X-Forwarded-Proto"); got != "http" {
		t.Fatalf("X-Forwarded-Proto: got %q want http (no client trust)", got)
	}
}

func TestPeerIPLoopback(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[::1]:9"
	if got := peerIP(req); got != "127.0.0.1" {
		t.Fatalf("got %q", got)
	}
}
