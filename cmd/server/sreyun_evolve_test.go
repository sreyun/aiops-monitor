package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestResolveUIView(t *testing.T) {
	v, ok := resolveUIView("安全中心")
	if !ok || v.View != "security-overview" {
		t.Fatalf("got %#v ok=%v", v, ok)
	}
	v, ok = resolveUIView("host-security")
	if !ok || v.Title == "" {
		t.Fatalf("host-security %#v", v)
	}
	if _, ok := resolveUIView("not-a-real-view"); ok {
		t.Fatal("expected miss")
	}
}

func TestNavigateViewAction(t *testing.T) {
	act := navigateViewAction("sre", "打开 SRE", "SRE 中枢")
	if act["type"] != "navigate_view" || act["view"] != "sre" {
		t.Fatalf("%#v", act)
	}
	tbl := showTableAction("t1", "表", "Demo", []string{"a"}, []map[string]any{{"a": 1}})
	logs := showLogsAction("l1", "日志", "Demo", []map[string]any{{"ts": "12:00", "line": "hi"}})
	raw, _ := json.Marshal(capabilityResult{OK: true, UIActions: []map[string]any{act, tbl, logs}})
	s := string(raw)
	for _, want := range []string{`"navigate_view"`, `"show_table"`, `"show_logs"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %s in %s", want, s)
		}
	}
}

func TestFindDashPanel(t *testing.T) {
	d := Dashboard{
		ID: "d1",
		Panels: []DashPanel{
			{ID: 3, Title: "CPU Usage", Type: "timeseries"},
			{ID: 7, Title: "Memory", Type: "stat"},
		},
	}
	p, ok := findDashPanel(d, 7, "")
	if !ok || p.Title != "Memory" {
		t.Fatalf("by id: %#v", p)
	}
	p, ok = findDashPanel(d, 0, "cpu")
	if !ok || p.ID != 3 {
		t.Fatalf("by title: %#v", p)
	}
}

func TestSecurityDefenseDedup(t *testing.T) {
	cfg := &ConfigStore{cfg: ServerConfig{AI: AIConfig{AutoDefendEnabled: true}}}
	s := &Server{store: NewStore(), cfg: cfg, incidents: newIncidentManager()}
	src := fmt.Sprintf("203.0.113.%d", time.Now().UnixNano()%200+1)
	r1 := s.recordSecurityDefense(securityDefenseInput{
		Kind: "login_bruteforce", Source: src, Target: "admin-dedup",
		Summary: "test bruteforce", CreateTicket: true, Actor: "test", Force: true,
	})
	if r1.Error != "" {
		t.Fatalf("first error: %s", r1.Error)
	}
	if r1.Data["incident_id"] == nil || r1.Data["incident_id"] == int64(0) {
		t.Fatalf("expected incident, got %#v", r1.Data)
	}
	r2 := s.recordSecurityDefense(securityDefenseInput{
		Kind: "login_bruteforce", Source: src, Target: "admin-dedup",
		Summary: "test bruteforce again", CreateTicket: true, Actor: "test",
	})
	if !r2.Skipped {
		t.Fatalf("expected dedup skip, got %#v", r2)
	}
}
