package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleDashboardServesV2ByDefault(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	s.handleDashboard(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	body := rr.Body.String()
	// Built SPA references /v2/ assets; placeholder mentions make web.
	if !strings.Contains(body, "/v2/") && !strings.Contains(body, "make web") && !strings.Contains(body, "root") {
		t.Fatalf("expected v2 shell html, got prefix %q", trimForTest(body, 120))
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type=%q", ct)
	}
}

func TestHandleDashboardLegacyFallback(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/?ui=legacy", nil)
	rr := httptest.NewRecorder()
	s.handleDashboard(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `id="app"`) {
		t.Fatalf("expected classic index.html shell")
	}
}

func trimForTest(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
