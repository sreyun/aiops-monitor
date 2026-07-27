package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneExpiredUnixMap(t *testing.T) {
	now := time.Now().Unix()
	m := map[string]int64{
		"a": now - 10,
		"b": now + 60,
		"c": now - 1,
	}
	pruneExpiredUnixMap(m, now, 4096, func(v int64) int64 { return v })
	if _, ok := m["a"]; ok {
		t.Fatal("expired a should be pruned")
	}
	if _, ok := m["c"]; ok {
		t.Fatal("expired c should be pruned")
	}
	if _, ok := m["b"]; !ok {
		t.Fatal("live b should remain")
	}
}

func TestPruneExpiredUnixMapCap(t *testing.T) {
	now := time.Now().Unix()
	m := map[string]int64{}
	for i := 0; i < 20; i++ {
		m[string(rune('a'+i))] = now + 60
	}
	pruneExpiredUnixMap(m, now, 5, func(v int64) int64 { return v })
	if len(m) > 5 {
		t.Fatalf("want <=5 got %d", len(m))
	}
}

func TestAuditExportAllows(t *testing.T) {
	cfg := AuditExportConfig{MinLevel: "warning", Kinds: []string{"operation", "terminal"}}
	if auditExportAllows(cfg, LogEntry{Kind: "system", Level: "warning"}) {
		t.Fatal("kind system should be filtered")
	}
	if !auditExportAllows(cfg, LogEntry{Kind: "operation", Level: "warning"}) {
		t.Fatal("operation warning should pass")
	}
	if auditExportAllows(cfg, LogEntry{Kind: "operation", Level: "info"}) {
		t.Fatal("info below min_level should fail")
	}
}

func TestValidateAuditExportConfig(t *testing.T) {
	if err := validateAuditExportConfig(AuditExportConfig{Enabled: true}); err == nil {
		t.Fatal("enabled without destination should fail")
	}
	if err := validateAuditExportConfig(AuditExportConfig{Enabled: true, WebhookURL: "not-a-url"}); err == nil {
		t.Fatal("bad webhook should fail")
	}
	if err := validateAuditExportConfig(AuditExportConfig{Enabled: true, SyslogAddr: "127.0.0.1:514"}); err != nil {
		t.Fatal(err)
	}
}

func TestForbidEmptyHostIDForScoped(t *testing.T) {
	cfg, err := NewConfigStore(filepath.Join(t.TempDir(), "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	salt := genToken()[:16]
	cfg.cfg.Users = append(cfg.cfg.Users, AccountConfig{
		Username: "scoped", Role: RoleOperator,
		Salt: salt, Hash: hashPassword("Passw0rd!", salt),
		AllowedHostIDs: []string{"host-a"},
	})
	h := &SreyunCore{s: &Server{cfg: cfg, store: NewStore()}}
	if msg := h.forbidToolHostAccess("scoped", "query_metrics", map[string]any{}); msg == "" {
		t.Fatal("scoped query_metrics without host_id must deny")
	}
	if msg := h.forbidToolHostAccess("scoped", "list_alerts", map[string]any{}); msg != "" {
		t.Fatalf("list_alerts empty host should allow: %s", msg)
	}
}

func TestWriteAPIErrorIncludesRequestID(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxKeyRequestID{}, "rid-test-1"))
	writeAPIError(rr, req, http.StatusForbidden, "forbidden", "nope")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status %d", rr.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "forbidden" || body["error"] != "nope" {
		t.Fatalf("body=%v", body)
	}
	if body["request_id"] != "rid-test-1" {
		t.Fatalf("request_id=%q", body["request_id"])
	}
}
