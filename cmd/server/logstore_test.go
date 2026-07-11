package main

import (
	"strings"
	"testing"

	"aiops-monitor/shared"
)

func TestLogStoreSearch(t *testing.T) {
	ls := newLogStore()
	ls.ingest("h1", "web", []shared.LogLine{
		{Ts: 100, Source: "/var/log/a", Level: "ERROR", Message: "connection refused"},
		{Ts: 101, Source: "/var/log/a", Level: "info", Message: "started ok"},
		{Ts: 102, Source: "/var/log/a", Level: "warn", Message: "slow query"},
	})
	if ls.count() != 3 {
		t.Fatalf("count=%d, want 3", ls.count())
	}
	if r := ls.search("", "", "refused", 0, 10); len(r) != 1 || r[0].Message != "connection refused" {
		t.Fatalf("keyword search failed: %v", r)
	}
	// "ERROR" must normalize to "error".
	if r := ls.search("", "error", "", 0, 10); len(r) != 1 {
		t.Fatalf("expected 1 error line, got %d", len(r))
	}
	if r := ls.recentErrors(0, 10); len(r) != 2 { // error + warn
		t.Fatalf("recentErrors=%d, want 2", len(r))
	}
	if ls.errorCount(0) != 1 {
		t.Fatalf("errorCount=%d, want 1", ls.errorCount(0))
	}
	if all := ls.search("", "", "", 0, 10); all[0].Ts != 102 {
		t.Fatalf("expected newest-first, got Ts=%d", all[0].Ts)
	}
}

func TestHeuristicInspect(t *testing.T) {
	ctx := inspectionContext{
		OnlineHosts:   3,
		OfflineHosts:  []string{"db-01"},
		FiringAlerts:  []Alert{{Level: "critical", Hostname: "web-01", Message: "CPU 96%"}},
		BreachingSLOs: []SLOStatus{{SLO: SLO{Name: "API可用性", Target: 99.9}, SLI: 99.0}},
		HighUsage:     []string{"web-01 CPU 96%"},
		ErrorCount:    60,
	}
	summary, findings := heuristicInspect(ctx)
	if summary == "" {
		t.Fatal("empty summary")
	}
	var crit, warn int
	for _, f := range findings {
		switch f.Severity {
		case "critical":
			crit++
		case "warning":
			warn++
		}
	}
	if crit < 3 {
		t.Errorf("expected >=3 critical findings (offline+alert+errors>=50), got %d", crit)
	}
	if warn < 2 {
		t.Errorf("expected >=2 warning findings (slo+high-usage), got %d", warn)
	}
	// A healthy snapshot yields no findings.
	if s2, f2 := heuristicInspect(inspectionContext{OnlineHosts: 5}); len(f2) != 0 || s2 == "" {
		t.Errorf("healthy inspection should have no findings, got %d", len(f2))
	}
}

func TestHeuristicDiagnose(t *testing.T) {
	out := heuristicDiagnose(Incident{Type: "disk", Title: "disk full"}, "主机: web-01")
	if !strings.Contains(out, "清理") {
		t.Errorf("disk diagnosis should mention cleanup, got: %s", out)
	}
	// Unknown type still returns a sensible generic direction.
	if g := heuristicDiagnose(Incident{Type: ""}, ""); g == "" {
		t.Error("generic diagnosis must not be empty")
	}
}
