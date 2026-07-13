package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSubstitutePlaybookVars(t *testing.T) {
	vars := map[string]string{"os": "linux", "ip": "192.168.1.5"}
	cases := map[string]string{
		"echo {{os}}":            "echo linux",
		"{{ip}}:{{os}}":          "192.168.1.5:linux",
		"{{ os }} spaced":        "linux spaced",  // tolerant of inner spaces
		"unknown {{nope}} here":  "unknown  here", // unknown → empty
		"no placeholders at all": "no placeholders at all",
	}
	for in, want := range cases {
		if got := substitutePlaybookVars(in, vars); got != want {
			t.Errorf("substitute(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEvalPlaybookWhen(t *testing.T) {
	vars := map[string]string{"os": "linux", "flag": "false", "empty": ""}
	truthy := []string{"{{os}} == linux", "{{os}} != windows", "yes", "1", "anything"}
	falsy := []string{"{{os}} == windows", "{{os}} != linux", "{{flag}}", "{{empty}}", "false", "0", "no", "off", ""}
	for _, w := range truthy {
		if !evalPlaybookWhen(w, vars) {
			t.Errorf("evalPlaybookWhen(%q) = false, want true", w)
		}
	}
	for _, w := range falsy {
		if evalPlaybookWhen(w, vars) {
			t.Errorf("evalPlaybookWhen(%q) = true, want false", w)
		}
	}
}

func TestResolvePlaybookCommand_PerOS(t *testing.T) {
	step := PlaybookStep{
		Command:    "ip a",
		CommandWin: "ipconfig {{ip}}",
		CommandMac: "ifconfig",
	}
	vars := map[string]string{"ip": "10.0.0.1"}
	cases := []struct {
		os, want string
	}{
		{"linux", "ip a"},
		{"windows", "ipconfig 10.0.0.1"}, // per-OS override + var substitution
		{"darwin", "ifconfig"},
	}
	for _, c := range cases {
		got := resolvePlaybookCommand(step, &Host{OS: c.os}, vars)
		if got != c.want {
			t.Errorf("resolve(os=%s) = %q, want %q", c.os, got, c.want)
		}
	}
	// Empty per-OS override falls back to Command.
	bare := PlaybookStep{Command: "uptime"}
	if got := resolvePlaybookCommand(bare, &Host{OS: "windows"}, vars); got != "uptime" {
		t.Errorf("fallback = %q, want uptime", got)
	}
}

func TestBuildModuleCommand_Envelope(t *testing.T) {
	cmd := buildModuleCommand("service", map[string]string{"name": "{{svc}}", "state": "restarted"}, map[string]string{"svc": "nginx"})
	if !strings.HasPrefix(cmd, modulePrefix+" ") {
		t.Fatalf("command %q missing module prefix", cmd)
	}
	payload := strings.TrimSpace(strings.TrimPrefix(cmd, modulePrefix))
	var mc struct {
		Module string            `json:"module"`
		Args   map[string]string `json:"args"`
	}
	if err := json.Unmarshal([]byte(payload), &mc); err != nil {
		t.Fatalf("payload not valid JSON: %v (%q)", err, payload)
	}
	if mc.Module != "service" {
		t.Errorf("module = %q, want service", mc.Module)
	}
	if mc.Args["name"] != "nginx" { // var substituted inside args
		t.Errorf("args.name = %q, want nginx", mc.Args["name"])
	}
	if mc.Args["state"] != "restarted" {
		t.Errorf("args.state = %q, want restarted", mc.Args["state"])
	}
}

func TestResolvePlaybookCommand_ModuleTakesPriority(t *testing.T) {
	// A step with both a Command and a Module must route to the module envelope.
	step := PlaybookStep{Command: "should-be-ignored", Module: "gather_facts", Args: map[string]string{}}
	got := resolvePlaybookCommand(step, &Host{OS: "linux"}, map[string]string{})
	if !strings.HasPrefix(got, modulePrefix) {
		t.Errorf("module step resolved to %q, want module envelope", got)
	}
}
