package main

import (
	"encoding/json"
	"testing"
)

func TestExtractUIActionsFromToolResult(t *testing.T) {
	raw := capabilityJSON(capabilityResult{
		OK:      true,
		Summary: "ok",
		UIActions: []map[string]any{
			openDashboardAction("d1", "CPU"),
			exportReportAction("报告", "## 正文"),
		},
	})
	acts := extractUIActionsFromToolResult(raw)
	if len(acts) != 2 {
		t.Fatalf("want 2 actions, got %d (%s)", len(acts), raw)
	}
	if acts[0]["type"] != "open_dashboard" || acts[0]["id"] != "d1" {
		t.Fatalf("unexpected open action: %#v", acts[0])
	}
	if acts[1]["type"] != "export_report" {
		t.Fatalf("unexpected export action: %#v", acts[1])
	}
	if extractUIActionsFromToolResult("not-json") != nil {
		t.Fatal("non-json should yield nil")
	}
	if extractUIActionsFromToolResult(`{"ok":true}`) != nil {
		t.Fatal("missing _ui_actions should yield nil/empty")
	}
}

func TestCapabilityJSONEnvelope(t *testing.T) {
	raw := capabilityJSON(capabilityResult{OK: false, Error: "看板不存在"})
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if m["ok"] != false || m["error"] != "看板不存在" {
		t.Fatalf("bad envelope: %v", m)
	}
}

func TestRegisterCapabilityTools(t *testing.T) {
	h := &SreyunCore{tools: map[string]SreyunTool{}}
	h.registerCapabilityTools()
	want := []string{
		"list_dashboards", "create_dashboard", "get_dashboard",
		"analyze_dashboard", "optimize_dashboard", "apply_dashboard_optimize",
		"run_assist_task", "diagnose_incident", "get_duty_context",
	}
	for _, name := range want {
		if _, ok := h.tools[name]; !ok {
			t.Fatalf("missing tool %s", name)
		}
	}
}

func TestValidAssistTaskNameForCapability(t *testing.T) {
	if !validAssistTaskName("host_security_diagnosis") {
		t.Fatal("expected valid")
	}
	if validAssistTaskName("Bad-Task") || validAssistTaskName("") {
		t.Fatal("expected invalid")
	}
}
