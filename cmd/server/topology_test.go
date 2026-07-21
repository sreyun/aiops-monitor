package main

import (
	"strings"
	"testing"
)

func TestNormalizeTopoRef(t *testing.T) {
	cases := map[string]string{
		"host:abc": "host:abc",
		"abc":      "host:abc",
		"cat:DB":   "cat:DB",
		"svc:api":  "svc:api",
		"  ":       "",
	}
	for in, want := range cases {
		if got := normalizeTopoRef(in); got != want {
			t.Errorf("normalizeTopoRef(%q)=%q want %q", in, got, want)
		}
	}
}

func TestFormatTopologyRCASummary(t *testing.T) {
	s := formatTopologyRCASummary(TopologyRCA{
		HostID: "h1", Hostname: "web1", Category: "App",
		Upstream:   []TopologyNodeHit{{Ref: "svc:db"}},
		Downstream: []TopologyNodeHit{{Ref: "svc:gateway"}},
		RelatedHosts: []TopologyHostHit{
			{HostID: "h2", Hostname: "db1", Reason: "upstream"},
		},
		OpenIncidents: []TopologyIncHit{{ID: 9, Title: "disk full"}},
		Hints:         []string{"核对变更"},
	})
	for _, must := range []string{"web1", "上游", "下游", "关联主机", "未决事件", "核对变更"} {
		if !strings.Contains(s, must) {
			t.Errorf("summary missing %q:\n%s", must, s)
		}
	}
}

func TestRemediationProposeAndApprove(t *testing.T) {
	m := newRemediationManager(nil)
	launched := 0
	m.getPlaybook = func(id string) (Playbook, bool) {
		return Playbook{ID: id, Name: "fix-pb", Steps: []PlaybookStep{{Name: "n", Command: "echo ok"}}}, true
	}
	m.resolveHost = func(id string) *Host { return &Host{ID: "h1", Hostname: "web"} }
	m.trigger = func(pb Playbook, host *Host, op string, onDone func(ok bool)) int64 {
		launched++
		onDone(true)
		return int64(launched)
	}
	pb := Playbook{ID: "pb-prop", Name: "fix-pb", Steps: []PlaybookStep{{Name: "n", Command: "echo ok"}}}
	run, err := m.ProposeManual(pb, "h1", "web", 42, "提案测试", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "pending_approval" || run.RuleID != "" || run.IncidentID != 42 {
		t.Fatalf("unexpected run: %+v", run)
	}
	if err := m.Approve(run.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	if launched != 1 {
		t.Fatalf("expected launch once, got %d", launched)
	}
	// 提案不应占用规则冷却键造成副作用：再提案一次仍可批准
	run2, err := m.ProposeManual(pb, "h1", "web", 43, "提案2", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Approve(run2.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	if launched != 2 {
		t.Fatalf("second proposal should launch, got %d", launched)
	}
}

func TestAssistTaskPolicy_RemediationProposal(t *testing.T) {
	p := assistTaskPolicy("remediation_proposal")
	if p.Timeout <= 0 {
		t.Fatal("expected timeout")
	}
	sys := buildAssistSystemPrompt("remediation_proposal", "ctx")
	if !strings.Contains(sys, "一次性") && !strings.Contains(sys, "提案") {
		t.Fatalf("prompt=%s", trimLine(sys, 120))
	}
}
