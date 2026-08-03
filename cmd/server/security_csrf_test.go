package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOriginAllowedTrustProxyForwardedHost(t *testing.T) {
	srv := &Server{cfg: &ConfigStore{cfg: ServerConfig{TrustProxy: true}}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logout", nil)
	req.Host = "127.0.0.1:18529" // what the container sees without Host rewrite
	req.Header.Set("Origin", "http://127.0.0.1:8529")
	req.Header.Set("X-Forwarded-Host", "127.0.0.1:8529")
	if !srv.originAllowed(req) {
		t.Fatal("expected Origin to match X-Forwarded-Host under TrustProxy")
	}
}

func TestOriginAllowedRejectsMismatch(t *testing.T) {
	srv := &Server{cfg: &ConfigStore{cfg: ServerConfig{TrustProxy: false}}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logout", nil)
	req.Host = "127.0.0.1:18529"
	req.Header.Set("Origin", "http://127.0.0.1:8529")
	if srv.originAllowed(req) {
		t.Fatal("without TrustProxy, mismatched Origin/Host must fail")
	}
}
