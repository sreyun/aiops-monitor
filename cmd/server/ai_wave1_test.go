package main

import (
	"strings"
	"testing"
)

func TestEmbedReady(t *testing.T) {
	if embedReady(AIConfig{Enabled: true, APIKey: "k", Endpoint: "http://x"}) != true {
		t.Fatal("chat key+ep should be ready")
	}
	if embedReady(AIConfig{Enabled: true, EmbedAPIKey: "ek", EmbedEndpoint: "http://e"}) != true {
		t.Fatal("embed-only should be ready")
	}
	if embedReady(AIConfig{Enabled: true, EmbedAPIKey: "ek"}) {
		t.Fatal("embed key without endpoint should not be ready")
	}
	if embedReady(AIConfig{Enabled: false, APIKey: "k", Endpoint: "http://x"}) {
		t.Fatal("disabled should not be ready")
	}
}

func TestAssistStoreFeedbackBinding(t *testing.T) {
	st := newAssistStore()
	rec := st.put("pgsql", "list tables", "```sql\nSELECT 1\n```", "alice")
	if rec.ID == "" {
		t.Fatal("empty id")
	}
	got, ok := st.get(rec.ID)
	if !ok || got.Answer != rec.Answer || got.Input != "list tables" {
		t.Fatalf("get mismatch: %+v", got)
	}
	if _, ok := st.get("ast_nope"); ok {
		t.Fatal("missing id should fail")
	}
}

func TestWriteApprovalConsume(t *testing.T) {
	h := newAIGovHub()
	a := h.issueWriteApproval("bob", "k8s_scale", "hash1", 60)
	if a.ID == "" {
		t.Fatal("empty approval")
	}
	if !h.consumeWriteApproval(a.ID, "k8s_scale", "hash1") {
		t.Fatal("first consume should succeed")
	}
	if h.consumeWriteApproval(a.ID, "k8s_scale", "hash1") {
		t.Fatal("second consume must fail")
	}
}

func TestMCPRateLimit(t *testing.T) {
	h := newAIGovHub()
	for i := 0; i < 3; i++ {
		ok, _, _ := h.checkAndIncrMCPRate("tok", 3)
		if !ok {
			t.Fatalf("hit %d should be ok", i)
		}
	}
	ok, used, lim := h.checkAndIncrMCPRate("tok", 3)
	if ok || used != 3 || lim != 3 {
		t.Fatalf("expected deny used=%d lim=%d ok=%v", used, lim, ok)
	}
}

func TestExtractAssistCode(t *testing.T) {
	lang, code := extractAssistCode("前言\n```sql\nSELECT now();\n```\n后")
	if lang != "sql" || !strings.Contains(code, "SELECT now()") {
		t.Fatalf("lang=%q code=%q", lang, code)
	}
}

func TestQuotaTaskExempt(t *testing.T) {
	cfg := AIConfig{QuotaExemptTasks: "distill, embed_test"}
	if !quotaTaskExempt(cfg, "distill") || quotaTaskExempt(cfg, "chat") {
		t.Fatal("exempt parse failed")
	}
}

func TestArgsHashForApprovalStable(t *testing.T) {
	a := argsHashForApproval("k8s_scale", map[string]any{"cluster_id": "c1", "namespace": "ns", "name": "web", "replicas": 3.0})
	b := argsHashForApproval("k8s_scale", map[string]any{"cluster_id": "c1", "namespace": "ns", "name": "web", "replicas": 3.0})
	if a == "" || a != b {
		t.Fatalf("unstable hash %q vs %q", a, b)
	}
}
