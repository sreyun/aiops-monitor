package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseNucleiJSONLMatcherName(t *testing.T) {
	raw := strings.Join([]string{
		`{"template-id":"http-missing-security-headers","info":{"name":"HTTP Missing Security Headers","severity":"info","description":"missing headers"},"type":"http","host":"http://example.com","matched-at":"http://example.com","matcher-name":"x-frame-options"}`,
		`{"template-id":"http-missing-security-headers","info":{"name":"HTTP Missing Security Headers","severity":"info","description":"missing headers"},"type":"http","host":"http://example.com","matched-at":"http://example.com","matcher-name":"content-security-policy"}`,
	}, "\n")
	out := parseNucleiJSONL(strings.NewReader(raw))
	if len(out) != 2 {
		t.Fatalf("findings=%d", len(out))
	}
	if out[0].MatcherName != "x-frame-options" || !strings.Contains(out[0].Name, "x-frame-options") {
		t.Fatalf("finding0=%+v", out[0])
	}
	if out[1].MatcherName != "content-security-policy" || !strings.Contains(out[1].Name, "content-security-policy") {
		t.Fatalf("finding1=%+v", out[1])
	}
	if out[0].Remediation == out[1].Remediation {
		t.Fatalf("expected distinct remediation per matcher, both=%q", out[0].Remediation)
	}
	if !strings.Contains(out[0].Remediation, "x-frame-options") {
		t.Fatalf("remediation missing header name: %q", out[0].Remediation)
	}
}

func TestUniqueRemediationTipsDedupesIdentical(t *testing.T) {
	same := "按模板「HTTP Missing Security Headers」(info) 的官方修复建议加固：升级组件、关闭暴露面或加强访问控制。"
	findings := []WebFinding{
		{Name: "HTTP Missing Security Headers [a]", MatcherName: "a", Remediation: same},
		{Name: "HTTP Missing Security Headers [b]", MatcherName: "b", Remediation: same},
		{Name: "HTTP Missing Security Headers [c]", MatcherName: "c", Remediation: same},
	}
	tips := uniqueRemediationTips(findings, 15)
	if len(tips) != 1 {
		t.Fatalf("expected body-dedupe to 1 tip, got %v", tips)
	}
}

func TestBuildWebScanReportDedupesRemediation(t *testing.T) {
	findings := []WebFinding{
		{Name: "HTTP Missing Security Headers [x-frame-options]", MatcherName: "x-frame-options", Severity: "info",
			Remediation: "在 HTTP 响应中配置安全头「x-frame-options」"},
		{Name: "HTTP Missing Security Headers [content-security-policy]", MatcherName: "content-security-policy", Severity: "info",
			Remediation: "在 HTTP 响应中配置安全头「content-security-policy」"},
		{Name: "HTTP Missing Security Headers [x-frame-options]", MatcherName: "x-frame-options", Severity: "info",
			Remediation: "在 HTTP 响应中配置安全头「x-frame-options」"},
	}
	rep := buildWebScanReport(WebScanTarget{Name: "demo", BaseURL: "http://example.com"}, findings)
	if len(rep.Remediation) != 2 {
		t.Fatalf("remediation=%v", rep.Remediation)
	}
}

func TestHumanizeNucleiErr(t *testing.T) {
	msg := humanizeNucleiErr("nuclei: [\x1b[1;31mFTL\x1b[0m] Could not run nuclei: no templates provided for scan", nil)
	if strings.Contains(msg, "\x1b") || strings.Contains(msg, "[1;31m") {
		t.Fatalf("ANSI not stripped: %q", msg)
	}
	if !strings.Contains(msg, "模板") {
		t.Fatalf("expected Chinese template hint, got %q", msg)
	}
}

func TestZhWebSecErr(t *testing.T) {
	got := zhWebSecErr("nuclei: [\x1b[31mFTL\x1b[0m] no templates provided for scan")
	if strings.Contains(got, "\x1b") || strings.Contains(strings.ToLower(got), "no templates provided") {
		t.Fatalf("raw English/ANSI leaked: %q", got)
	}
}

func TestBuildNucleiTemplateArgsTagsMapToDirs(t *testing.T) {
	root := t.TempDir()
	for _, sub := range []string{"http/misconfiguration", "http/exposures", "ssl", "http/exposed-panels"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(sub)), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	args := buildNucleiTemplateArgs(root, WebScanTarget{Tags: []string{"misconfig", "exposures"}})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "misconfiguration") || !strings.Contains(joined, "exposures") || !strings.Contains(joined, "ssl") {
		t.Fatalf("expected mapped dirs incl. ssl, got %v", args)
	}
	if strings.Contains(joined, "-tags") {
		t.Fatalf("should not need -tags when dirs mapped: %v", args)
	}
}

func TestNucleiTemplatesReady(t *testing.T) {
	root := t.TempDir()
	httpDir := filepath.Join(root, "http", "misconfiguration")
	if err := os.MkdirAll(httpDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if nucleiTemplatesReady(root) {
		t.Fatal("empty tree should not be ready")
	}
	for i := 0; i < 25; i++ {
		name := filepath.Join(httpDir, fmt.Sprintf("t%d.yaml", i))
		if err := os.WriteFile(name, []byte("id: t\n"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	if !nucleiTemplatesReady(root) {
		t.Fatal("expected ready after yaml files present")
	}
}

func TestAllocScanMetaReadable(t *testing.T) {
	m := newWebScanManager(t.TempDir(), 1)
	id, label, seq := m.allocScanMeta("芒果系统")
	if seq != 1 {
		t.Fatalf("seq=%d", seq)
	}
	if !strings.HasPrefix(id, "ws-001-") {
		t.Fatalf("id not readable: %s", id)
	}
	if !strings.Contains(label, "芒果系统") || !strings.Contains(label, "#001") {
		t.Fatalf("label=%q", label)
	}
}

func TestWebSecurityConfigDefaultsRaised(t *testing.T) {
	c := (WebSecurityConfig{}).withDefaults()
	if c.TimeoutSec != 900 || c.Concurrency != 25 || c.RateLimit != 120 || c.ScanConcurrency != 3 {
		t.Fatalf("defaults=%+v", c)
	}
}

func TestBuildNucleiRunArgsPerfFlags(t *testing.T) {
	cfg := WebSecurityConfig{Concurrency: 25, RateLimit: 120, Severity: "critical,high"}.withDefaults()
	args := buildNucleiRunArgs(cfg, WebScanTarget{BaseURL: "https://example.com"}, t.TempDir(), nil)
	joined := strings.Join(args, " ")
	for _, want := range []string{"-c 25", "-rate-limit 120", "-bulk-size", "-timeout 10", "-retries 1", "-nh", "-omit-raw", "-disable-update-check"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %v", want, args)
		}
	}
}

func TestBuildNucleiTemplateArgsSpecialtyTags(t *testing.T) {
	root := t.TempDir()
	vuln := filepath.Join(root, "http", "vulnerabilities")
	if err := os.MkdirAll(vuln, 0o750); err != nil {
		t.Fatal(err)
	}
	args := buildNucleiTemplateArgs(root, WebScanTarget{Tags: []string{"xss", "sqli"}})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "vulnerabilities") {
		t.Fatalf("expected vuln dir: %v", args)
	}
	if !strings.Contains(joined, "-tags") || !strings.Contains(joined, "xss") || !strings.Contains(joined, "sqli") {
		t.Fatalf("expected specialty -tags filter: %v", args)
	}
	// Full vulnerabilities pack should not add -tags narrowing.
	args2 := buildNucleiTemplateArgs(root, WebScanTarget{Tags: []string{"vulnerabilities", "xss"}})
	joined2 := strings.Join(args2, " ")
	if strings.Contains(joined2, "-tags") {
		t.Fatalf("full vuln pack should skip specialty -tags: %v", args2)
	}
}

func TestSetScanConcurrencyResizesSlots(t *testing.T) {
	m := newWebScanManager(t.TempDir(), 1)
	m.setScanConcurrency(4)
	m.concMu.Lock()
	got := m.maxConc
	m.concMu.Unlock()
	if got != 4 {
		t.Fatalf("maxConc=%d", got)
	}
	// Acquire up to 4 without blocking.
	for i := 0; i < 4; i++ {
		m.acquireScanSlot()
	}
	m.concMu.Lock()
	active := m.active
	m.concMu.Unlock()
	if active != 4 {
		t.Fatalf("active=%d", active)
	}
	for i := 0; i < 4; i++ {
		m.releaseScanSlot()
	}
}

func TestMigrateWebSecurityDefaultsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cs, err := NewConfigStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs.mu.Lock()
	cs.cfg.WebSecurity = WebSecurityConfig{
		TimeoutSec: 300, Concurrency: 10, RateLimit: 50, ScanConcurrency: 1,
	}
	cs.mu.Unlock()
	if !cs.migrateWebSecurityDefaultsOnce() {
		t.Fatal("expected migration")
	}
	got := cs.WebSecurity()
	if got.TimeoutSec != 900 || got.Concurrency != 25 || got.RateLimit != 120 || got.ScanConcurrency != 3 {
		t.Fatalf("migrated=%+v", got)
	}
	if cs.cfg.WebSecurity.DefaultsGen < webSecDefaultsGen {
		t.Fatalf("DefaultsGen=%d", cs.cfg.WebSecurity.DefaultsGen)
	}
	if cs.migrateWebSecurityDefaultsOnce() {
		t.Fatal("second migrate should be no-op")
	}
	// Intentional custom value after migrate must stick.
	if err := cs.SetWebSecurity(WebSecurityConfig{
		TimeoutSec: 600, Concurrency: 15, RateLimit: 80, ScanConcurrency: 1,
		DefaultsGen: webSecDefaultsGen,
	}); err != nil {
		t.Fatal(err)
	}
	got = cs.WebSecurity()
	if got.TimeoutSec != 600 || got.ScanConcurrency != 1 {
		t.Fatalf("custom after migrate overwritten: %+v", got)
	}
}
