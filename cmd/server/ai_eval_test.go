package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Wave 2/3 golden eval harness — offline, no network.
// Covers: skill packs, MCP scopes, write approval, assist verify parse, run feedback binding.

func TestEvalSkillPacksEmbedded(t *testing.T) {
	packs, err := listEmbeddedSkillPacks()
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) < 4 {
		t.Fatalf("expected >=4 packs, got %d", len(packs))
	}
	want := map[string]bool{"mysql": true, "postgres": true, "kubernetes": true, "network": true}
	for _, p := range packs {
		delete(want, p.ID)
		if p.Count < 3 {
			t.Fatalf("pack %s too small: %d", p.ID, p.Count)
		}
		pack, err := loadEmbeddedSkillPack(p.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, sk := range pack.Skills {
			if sk.Name == "" || sk.Trigger == "" || !strings.Contains(sk.Steps, "1)") {
				t.Fatalf("pack %s skill incomplete: %+v", p.ID, sk)
			}
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing packs: %v", want)
	}
}

func TestEvalMCPScopes(t *testing.T) {
	cfg := AIConfig{
		MCPEnabled: true,
		MCPToken:   "primary-secret",
		MCPScopedTokensJSON: `[
			{"name":"metrics-only","token":"m1","scopes":["metrics","alerts"]},
			{"name":"sql-only","token":"s1","scopes":["sql"]}
		]`,
	}
	ok, scopes, name := resolveMCPAuth(cfg, "primary-secret")
	if !ok || name != "primary" || !mcpScopesAllowAll(scopes) {
		t.Fatalf("primary auth failed: ok=%v name=%s scopes=%v", ok, name, scopes)
	}
	ok, scopes, name = resolveMCPAuth(cfg, "m1")
	if !ok || name != "metrics-only" {
		t.Fatalf("scoped auth failed: %v %s", ok, name)
	}
	if !mcpToolAllowedByScopes("query_metrics", scopes) || mcpToolAllowedByScopes("query_datasource", scopes) {
		t.Fatalf("metrics scope tool filter wrong: %v", scopes)
	}
	ok, scopes, _ = resolveMCPAuth(cfg, "s1")
	if !ok || !mcpToolAllowedByScopes("query_datasource", scopes) || mcpToolAllowedByScopes("search_logs", scopes) {
		t.Fatalf("sql scope filter wrong")
	}
	if ok, _, _ := resolveMCPAuth(cfg, "bad"); ok {
		t.Fatal("bad token must fail")
	}
}

func TestEvalWriteApprovalForced(t *testing.T) {
	s := &Server{aiGov: newAIGovHub()}
	h := &SreyunCore{s: s}
	msg, blocked := h.sreyunWriteBlocked("k8s_scale", "c/ns/web", map[string]any{
		"cluster_id": "c", "namespace": "ns", "name": "web", "replicas": 2.0,
	})
	if !blocked || !strings.Contains(msg, "approval_id") {
		t.Fatalf("expected forced approval, got blocked=%v msg=%s", blocked, msg)
	}
	hash := argsHashForApproval("k8s_scale", map[string]any{
		"cluster_id": "c", "namespace": "ns", "name": "web", "replicas": 2.0,
	})
	a := s.aiGov.issueWriteApproval("ops", "k8s_scale", hash, 60)
	msg, blocked = h.sreyunWriteBlocked("k8s_scale", "c/ns/web", map[string]any{
		"cluster_id": "c", "namespace": "ns", "name": "web", "replicas": 2.0,
		"approval_id": a.ID,
	})
	if blocked {
		t.Fatalf("valid approval should pass: %s", msg)
	}
}

func TestEvalAssistVerifyExtractAndSkip(t *testing.T) {
	lang, code := extractAssistCode("x\n```promql\nup{job=\"a\"}\n```\n")
	if lang != "promql" || !strings.Contains(code, "up{") {
		t.Fatalf("%s %s", lang, code)
	}
	v := assistVerifyResult{Skipped: true, Task: "promql", Summary: "未配置可用数据源，已跳过验证"}
	if !v.Skipped || v.OK {
		t.Fatal("skip contract")
	}
	if ds := parseDatasourceHint("数据源 id=abc123\n方言：PostgreSQL", ""); ds != "abc123" {
		t.Fatalf("parse ds hint: %q", ds)
	}
}

func TestEvalAIRunFeedbackBinding(t *testing.T) {
	s := &Server{assistStore: newAssistStore()}
	s.persistAIRun(AIRun{
		ID: "run_test1", Kind: "assist", Task: "pgsql",
		Input: "list", Answer: "```sql\nSELECT 1\n```", Actor: "u",
	})
	run, ok := s.lookupAIRun("run_test1")
	if !ok || run.Answer == "" {
		t.Fatal("lookup failed")
	}
	if _, ok := s.lookupAIRun("run_missing"); ok {
		t.Fatal("missing should fail")
	}
}

func TestEvalFallbackModelsParse(t *testing.T) {
	cfg := AIConfig{Model: "a", FallbackModels: "b, a, c"}
	var models []string
	for _, m := range strings.Split(cfg.FallbackModels, ",") {
		m = strings.TrimSpace(m)
		if m != "" && m != cfg.Model {
			models = append(models, m)
		}
	}
	if len(models) != 2 || models[0] != "b" || models[1] != "c" {
		t.Fatalf("%v", models)
	}
}

func TestEvalCopilotDutyContextShape(t *testing.T) {
	// Ensure skill pack JSON is valid for UI consumption.
	packs, _ := listEmbeddedSkillPacks()
	b, err := json.Marshal(map[string]any{"skill_packs": packs, "suggestions": []string{"x"}})
	if err != nil || !strings.Contains(string(b), "mysql") {
		t.Fatalf("marshal packs: %v %s", err, b)
	}
}

func TestEvalQuotaAndEmbedGates(t *testing.T) {
	if !embedReady(AIConfig{Enabled: true, EmbedAPIKey: "e", EmbedEndpoint: "http://e"}) {
		t.Fatal("embed-only")
	}
	if quotaTaskExempt(AIConfig{QuotaExemptTasks: "distill"}, "chat") {
		t.Fatal("chat not exempt")
	}
}
