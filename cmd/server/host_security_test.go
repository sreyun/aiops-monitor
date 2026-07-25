package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestScoreHostFindings(t *testing.T) {
	score, risk, sum := scoreHostFindings([]HostFinding{
		{Level: "crit"},
		{Level: "high"},
		{Level: "medium"},
	})
	if score >= 100 || score <= 0 {
		t.Fatalf("unexpected score %d", score)
	}
	if risk != "critical" {
		t.Fatalf("risk=%s", risk)
	}
	if sum["crit"] != 1 || sum["high"] != 1 {
		t.Fatalf("summary=%v", sum)
	}
}

func TestParseNucleiJSONL(t *testing.T) {
	raw := `{"template-id":"tech-detect","info":{"name":"Tech Detect","severity":"info","description":"d","remediation":"r"},"matched-at":"https://example.com","host":"https://example.com","type":"http"}
{"template-id":"cve-demo","info":{"name":"Demo CVE","severity":"high"},"matched-at":"https://example.com/x"}
`
	fs := parseNucleiJSONL(strings.NewReader(raw))
	if len(fs) != 2 {
		t.Fatalf("got %d findings", len(fs))
	}
	if fs[1].Severity != "high" || fs[1].TemplateID != "cve-demo" {
		t.Fatalf("%+v", fs[1])
	}
	if fs[0].Remediation != "r" {
		t.Fatalf("remediation=%q", fs[0].Remediation)
	}
}

func TestAssertURLAllowedBlocksPrivate(t *testing.T) {
	if err := assertURLAllowed("http://127.0.0.1/", false); err == nil {
		t.Fatal("expected block")
	}
	if err := assertURLAllowed("http://127.0.0.1/", true); err != nil {
		t.Fatal(err)
	}
}

func TestQueryOSVBatchMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []any{
				map[string]any{
					"vulns": []any{
						map[string]any{
							"id":      "OSV-1",
							"summary": "test vuln",
							"database_specific": map[string]any{
								"severity": "HIGH",
								"cve_id":   "CVE-2024-0001",
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	pkgs := []hsAgentPkg{{Name: "openssl", Version: "1.1.1", Ecosystem: "Debian"}}
	fs, err := queryOSVBatch(context.Background(), srv.URL, pkgs, "debian", "dpkg")
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 || fs[0].CVE != "CVE-2024-0001" || fs[0].Level != "high" {
		t.Fatalf("%+v", fs)
	}
}

func TestHostSecScheduleInterval(t *testing.T) {
	m := newHostSecurityManager(t.TempDir())
	sc := &PlaybookSchedule{Enabled: true, Kind: "interval", IntervalMin: 5}
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	if !hostSecScheduleDue(sc, m, now) {
		t.Fatal("first fire expected")
	}
	if hostSecScheduleDue(sc, m, now.Add(2*time.Minute)) {
		t.Fatal("should not fire within interval")
	}
	if !hostSecScheduleDue(sc, m, now.Add(5*time.Minute)) {
		t.Fatal("should fire after interval")
	}
}

func TestSanitizeWebTargetRejectsBadScheme(t *testing.T) {
	t0 := WebScanTarget{Name: "x", BaseURL: "ftp://example.com"}
	if err := sanitizeWebTarget(&t0, false); err == nil {
		t.Fatal("expected error")
	}
	t1 := WebScanTarget{Name: "ok", BaseURL: "https://example.com", Tags: []string{"misconfig"}}
	if err := sanitizeWebTarget(&t1, false); err != nil {
		t.Fatal(err)
	}
}

func TestSanitizeWebTargetPrivateRequiresGlobal(t *testing.T) {
	t0 := WebScanTarget{Name: "local", BaseURL: "http://127.0.0.1/", AllowPrivate: true}
	if err := sanitizeWebTarget(&t0, false); err == nil {
		t.Fatal("per-target allow_private must not bypass global gate")
	}
	if err := sanitizeWebTarget(&t0, true); err != nil {
		t.Fatal(err)
	}
}

func TestMaskAuthHeader(t *testing.T) {
	got := maskAuthHeader("Cookie: session=secret")
	if got != "Cookie: ********" {
		t.Fatalf("got %q", got)
	}
}

func TestClamAVDefaultEnabled(t *testing.T) {
	c := HostSecurityConfig{}.withDefaults()
	if !c.clamAVEnabled() {
		t.Fatal("default should enable ClamAV attempt")
	}
	c.DisableClamAV = true
	if c.clamAVEnabled() {
		t.Fatal("disable_clamav should win")
	}
}
