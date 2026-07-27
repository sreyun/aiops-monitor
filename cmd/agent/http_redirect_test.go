package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPreserveAgentRedirectKeepsPOSTBody(t *testing.T) {
	var gotMethod string
	var gotBody []byte
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotBody, _ = io.ReadAll(r.Body)
		if r.URL.Path != "/api/v1/agent/register" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "host_id": "h1"})
	}))
	defer final.Close()

	// Simulate openresty: plain HTTP 301 → final URL (same hostname, different port).
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+r.URL.RequestURI(), http.StatusMovedPermanently)
	}))
	defer redir.Close()

	payload := []byte(`{"host_id":"h1","hostname":"n","token":"tok","fingerprint":"fp"}`)
	client := newAgentHTTPClient(10 * time.Second)
	resp, err := client.Post(redir.URL+"/api/v1/agent/register", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s method=%s", resp.StatusCode, b, gotMethod)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("redirect must preserve POST, got %s", gotMethod)
	}
	if !bytes.Contains(gotBody, []byte(`"fingerprint":"fp"`)) {
		t.Fatalf("body lost across redirect: %s", gotBody)
	}
}

func TestDefaultClientLosesPOSTOn301(t *testing.T) {
	// Lock in the Go default-client footgun so we never "fix" it by reverting
	// CheckRedirect. If this starts failing, Go changed redirect semantics.
	var gotMethod string
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer final.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+r.URL.Path, http.StatusMovedPermanently)
	}))
	defer redir.Close()

	resp, err := http.Post(redir.URL+"/x", "application/json", strings.NewReader(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotMethod != http.MethodGet {
		t.Fatalf("expected default client to convert POST→GET, got %s", gotMethod)
	}
}

func TestProbeUpgradeSkipsNonDefaultHTTPPort(t *testing.T) {
	if got := probeUpgradeHTTPToHTTPS("http://192.168.1.10:8529"); got != "http://192.168.1.10:8529" {
		t.Fatalf("lab port must stay http: %q", got)
	}
	if got := probeUpgradeHTTPToHTTPS("https://aiops.example.com"); got != "https://aiops.example.com" {
		t.Fatalf("https must stay: %q", got)
	}
}
