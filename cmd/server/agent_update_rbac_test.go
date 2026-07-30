package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aiops-monitor/shared"
)

func TestAgentUpdateStartRespectsHostScope(t *testing.T) {
	dir := t.TempDir()
	cfg, err := NewConfigStore(filepath.Join(dir, "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	s := &Server{
		cfg:          cfg,
		store:        store,
		auth:         NewAuth(cfg),
		agentUpdates: newAgentUpdateManager(),
		changes:      newChangeManager(),
	}

	_ = store.RegisterHost("host-a", "alpha", "fp-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	_ = store.RegisterHost("host-b", "beta", "fp-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	now := time.Now().Unix()
	for _, id := range []string{"host-a", "host-b"} {
		h, ok := store.GetHost(id)
		if !ok {
			t.Fatalf("missing host %s", id)
		}
		h.LastSeen = now
		h.AgentVersion = "0.1.0"
		h.OS = "linux"
		h.Arch = "amd64"
		h.ServerURL = "http://127.0.0.1:8520"
		_, _ = store.UpsertAuthenticated(shared.Report{
			HostID: id, Hostname: h.Hostname, Fingerprint: h.Fingerprint,
			Metrics: shared.Metrics{CPUPercent: 1},
		}, h.Fingerprint)
		// Refresh last_seen after upsert so resolveAgentUpdateTargets keeps them online.
		if hh, ok := store.GetHost(id); ok {
			hh.LastSeen = time.Now().Unix()
			hh.ServerURL = "http://127.0.0.1:8520"
			hh.AgentVersion = "0.1.0"
			hh.OS = "linux"
			hh.Arch = "amd64"
		}
	}

	salt := genToken()[:16]
	op := AccountConfig{
		Username: "scoped-op", DisplayName: "Scoped", Role: RoleOperator,
		Salt: salt, Hash: hashPassword("Passw0rd!", salt),
		AllowedHostIDs: []string{"host-a"},
	}
	cfg.cfg.Users = append(cfg.cfg.Users, op)
	cfg.cfg.RemoteGateDisabled = true // isolate host-scope RBAC from change/freeze gate
	_ = cfg.save()

	tok := s.auth.issueSession("scoped-op")
	withSession := func(req *http.Request) *http.Request {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
		return req
	}

	// Explicit out-of-scope host must not match.
	rr := httptest.NewRecorder()
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/v1/agents/update",
		strings.NewReader(`{"host_ids":["host-b"],"confirm":true}`)))
	req.Header.Set("Content-Type", "application/json")
	s.handleAgentUpdateStart(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("out-of-scope host_ids: want 400 got %d body=%s", rr.Code, rr.Body.String())
	}

	// all=true must only update hosts inside the caller's scope.
	rr = httptest.NewRecorder()
	req = withSession(httptest.NewRequest(http.MethodPost, "/api/v1/agents/update",
		strings.NewReader(`{"all":true,"confirm":true}`)))
	req.Header.Set("Content-Type", "application/json")
	s.handleAgentUpdateStart(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("scoped all=true: want 202 got %d body=%s", rr.Code, rr.Body.String())
	}
	var snap agentUpdateJob
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	for _, h := range snap.Hosts {
		if h.HostID == "host-b" && h.Status != "skipped" {
			t.Fatalf("scoped operator must not enqueue out-of-scope host-b: %+v", h)
		}
		if h.HostID == "host-a" && h.Status == "skipped" && strings.Contains(h.Message, "无权") {
			t.Fatalf("in-scope host-a should not be RBAC-skipped: %+v", h)
		}
	}
	foundA := false
	for _, h := range snap.Hosts {
		if h.HostID == "host-a" {
			foundA = true
		}
	}
	if !foundA {
		t.Fatalf("expected in-scope host-a in job hosts: %+v", snap.Hosts)
	}
}
