package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestServerURLFollowsBrowser locks the "follow the browser" behavior: the
// generated install / uninstall command must carry the exact address the admin
// used to reach the panel, never an auto-detected LAN/container IP.
//
// Regression guard for the docker case: the server used to scan network
// interfaces and substitute the first non-loopback IP when the admin browsed
// from localhost — inside a container that is the container's own docker-network
// address (e.g. 172.18.0.4), reachable by nobody. The command must instead
// reflect the request host (or X-Forwarded-Host behind a proxy).
func TestServerURLFollowsBrowser(t *testing.T) {
	srv, _ := newTestServer(t)

	check := func(name, host, xfHost, xfProto, want string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/install/info", nil)
		req.Host = host
		if xfHost != "" {
			req.Header.Set("X-Forwarded-Host", xfHost)
		}
		if xfProto != "" {
			req.Header.Set("X-Forwarded-Proto", xfProto)
		}
		if got := srv.serverURL(req); got != want {
			t.Errorf("%s: serverURL(host=%q, xfHost=%q, xfProto=%q) = %q, want %q",
				name, host, xfHost, xfProto, got, want)
		}
	}

	// A real address the admin browsed → used verbatim.
	check("lan ip host", "192.168.1.50:8529", "", "", "http://192.168.1.50:8529")
	// The bug: browsing localhost must NOT be rewritten to a guessed IP. It stays
	// localhost (predictable) — the admin uses a real address or sets public_url.
	check("localhost stays localhost", "localhost:8529", "", "", "http://localhost:8529")
	check("loopback stays loopback (no container IP)", "127.0.0.1:8529", "", "", "http://127.0.0.1:8529")
	// Behind a reverse proxy the forwarded headers describe the client-facing host.
	check("x-forwarded-host wins", "172.18.0.4:8529", "aiops.example.com", "https", "https://aiops.example.com")
	// Proxies may append a list; the first token is the client-facing hop.
	check("x-forwarded list takes first", "172.18.0.4:8529", "aiops.example.com, internal:8529", "https, http", "https://aiops.example.com")
}

// TestServerURLPublicURLOverride verifies an explicit public_url always wins —
// the reliable knob for reverse-proxy / stable-domain deployments.
func TestServerURLPublicURLOverride(t *testing.T) {
	t.Setenv("AIOPS_PUBLIC_URL", "https://mon.corp.local")
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/install/info", nil)
	req.Host = "localhost:8529" // even a loopback browse must yield the override
	if got := srv.serverURL(req); got != "https://mon.corp.local" {
		t.Errorf("public_url override: serverURL = %q, want %q", got, "https://mon.corp.local")
	}
}

func TestFirstForwardedValue(t *testing.T) {
	cases := map[string]string{
		"":                       "",
		"aiops.example.com":      "aiops.example.com",
		"a.example.com, b.local": "a.example.com",
		"  https , http ":        "https",
	}
	for in, want := range cases {
		if got := firstForwardedValue(in); got != want {
			t.Errorf("firstForwardedValue(%q) = %q, want %q", in, got, want)
		}
	}
}
