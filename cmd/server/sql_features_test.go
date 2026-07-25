package main

import (
	"strings"
	"testing"

	"aiops-monitor/cmd/server/sqltoolkit"
)

func TestDesensitizeSQL(t *testing.T) {
	in := "SELECT * FROM users WHERE email = 'secret@example.com' AND id = 12345678"
	out := desensitizeSQL(in)
	if strings.Contains(out, "secret@example.com") {
		t.Fatalf("email not redacted: %s", out)
	}
	if !strings.Contains(out, "'?'") {
		t.Fatalf("expected literal placeholder: %s", out)
	}
	if strings.Contains(out, "12345678") {
		t.Fatalf("long number not redacted: %s", out)
	}
}

func TestBuildExplainDiffFromAnalysis(t *testing.T) {
	before := &sqltoolkit.ExplainAnalysis{
		TableAccess: []sqltoolkit.ExplainHit{
			{Table: "users", AccessType: "ALL", Key: "", Rows: 10000, FullScanRisk: true},
		},
	}
	after := &sqltoolkit.ExplainAnalysis{
		TableAccess: []sqltoolkit.ExplainHit{
			{Table: "users", AccessType: "ref", Key: "idx_email", Rows: 12, FullScanRisk: false},
		},
	}
	diff := buildExplainDiffFromAnalysis(before, after)
	sum, _ := diff["summary"].(string)
	if !strings.Contains(sum, "差异") {
		t.Fatalf("summary=%q diff=%#v", sum, diff)
	}
	if imp, _ := diff["improved"].(int); imp < 1 {
		t.Fatalf("expected improvement, got %#v", diff)
	}
}

func TestSQLQueryHistoryPerUser(t *testing.T) {
	dir := t.TempDir()
	m := newSQLQueryHistoryManager(dir)
	score := 88
	e1 := m.append("alice", "analyze", "c1", "SELECT 1", &score)
	_ = m.append("bob", "audit", "", "SELECT 2", nil)
	if e1.User != "alice" {
		t.Fatalf("entry=%#v", e1)
	}
	if len(m.list("alice", 10)) != 1 || len(m.list("bob", 10)) != 1 {
		t.Fatal("history isolation failed")
	}
}

func TestHostFindingKeyStable(t *testing.T) {
	k1 := hostFindingKey("h1", HostFinding{Category: "cve", CVE: "CVE-2024-1", Title: "x"})
	k2 := hostFindingKey("h1", HostFinding{Category: "cve", CVE: "CVE-2024-1", Title: "y"})
	if k1 != k2 {
		t.Fatalf("keys differ: %q vs %q", k1, k2)
	}
}

func TestWebFindingKey(t *testing.T) {
	k := webFindingKey("t1", "tpl-x", "https://a.example/x")
	if !strings.HasPrefix(k, "web:t1:tpl-x:") {
		t.Fatalf("bad key %q", k)
	}
}

func TestSecurityFindingStatusUpdate(t *testing.T) {
	dir := t.TempDir()
	m := newSecurityFindingManager(dir)
	st, err := m.upsert("host:h1:cve:CVE-1", "host", findingStatusAck, "", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != findingStatusAck {
		t.Fatalf("status=%q", st.Status)
	}
	if _, err := m.upsert("host:h1:cve:CVE-1", "host", "invalid", "", "bob"); err == nil {
		t.Fatal("expected invalid status error")
	}
}

func TestDiscoverAutoTopologyComposeRule(t *testing.T) {
	edges := discoverAutoTopologyEdgesFromContainers([]containerInvTest{
		{hostID: "h1", names: []string{"myapp_web_1", "myapp_db_1", "standalone"}},
	})
	found := false
	for _, e := range edges {
		if e.From == "svc:compose:myapp" && e.To == "host:h1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("compose edge missing: %#v", edges)
	}
}

type containerInvTest struct {
	hostID string
	names  []string
}

func discoverAutoTopologyEdgesFromContainers(invs []containerInvTest) []TopologyEdge {
	seen := map[string]bool{}
	var edges []TopologyEdge
	add := func(from, to, kind, note string) {
		from = normalizeTopoRef(from)
		to = normalizeTopoRef(to)
		key := from + "|" + to + "|" + kind
		if seen[key] {
			return
		}
		seen[key] = true
		edges = append(edges, TopologyEdge{From: from, To: to, Kind: kind, Note: note})
	}
	for _, inv := range invs {
		projects := map[string]string{}
		for _, name := range inv.names {
			add("container:"+name, "host:"+inv.hostID, "runs_on", "")
			if m := reComposeProject.FindStringSubmatch(name); len(m) == 2 {
				projects[m[1]] = inv.hostID
			}
		}
		for proj, hostID := range projects {
			add("svc:compose:"+proj, "host:"+hostID, "runs_on", "")
		}
	}
	return edges
}
