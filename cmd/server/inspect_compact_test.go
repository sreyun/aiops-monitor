package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompactInspectBatchStripsReport(t *testing.T) {
	rep := json.RawMessage(`{"result":{"warnings":1,"critical":0},"findings":[{"level":"warn","message":"disk"}]}`)
	b := &hostInspectBatch{
		ID: "insp-1", Status: "done", HostCount: 1,
		Items: []hostInspectItem{{
			HostID: "h1", Hostname: "n1", Status: "warn", HasReport: true,
			Report:        append(json.RawMessage(nil), rep...),
			FindingsBrief: []hostInspectFindingBrief{{Level: "warn", Message: "disk"}},
		}},
	}
	c := compactInspectBatch(b)
	if c.Items[0].Report != nil {
		t.Fatal("compact should clear report")
	}
	if !c.Items[0].HasReport {
		t.Fatal("has_report should remain true")
	}
	if len(c.Items[0].FindingsBrief) != 1 {
		t.Fatal("findings_brief should be kept")
	}
	if b.Items[0].Report == nil {
		t.Fatal("original batch report mutated")
	}
}

func TestParseHostInspectOutputBrief(t *testing.T) {
	body := `{"host":{"os_family":"linux"},"result":{"warnings":2,"critical":0},"metrics":{"cpu_usage_pct":11.5,"mem_usage_pct":40},"findings":[{"level":"warn","message":"disk full"}]}`
	h := &Host{ID: "h1", Hostname: "n1", OS: "linux", IP: "1.2.3.4"}
	item := parseHostInspectOutput(h, body, 100)
	if item.Status != "warn" || !item.HasReport || item.CPUPct == nil || *item.CPUPct != 11.5 {
		t.Fatalf("unexpected item: %+v", item)
	}
	if len(item.FindingsBrief) != 1 || item.FindingsBrief[0].Message != "disk full" {
		t.Fatalf("brief findings: %+v", item.FindingsBrief)
	}
	if len(item.Report) < 10 {
		t.Fatal("full report missing")
	}
}

func TestTruncateUTF8(t *testing.T) {
	s := strings.Repeat("好", 100)
	out := truncateUTF8(s, 20)
	if len(out) > 40 {
		t.Fatalf("too long: %d", len(out))
	}
	if !strings.Contains(out, "truncated") {
		t.Fatal("missing truncated marker")
	}
}
