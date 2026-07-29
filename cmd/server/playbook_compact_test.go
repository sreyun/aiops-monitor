package main

import (
	"strings"
	"testing"
)

func TestCompactPlaybookExecutionTruncates(t *testing.T) {
	big := strings.Repeat("测", 3000) + strings.Repeat("x", 8000)
	e := PlaybookExecution{
		ID: 1, Status: "running",
		HostResults: map[string]HostExecResult{
			"h1": {
				Hostname: "a", Status: "running", Output: big,
				Steps: []StepResult{{Name: "inspect", Status: "success", Output: big}},
			},
		},
	}
	c := compactPlaybookExecution(e, 4096)
	hr := c.HostResults["h1"]
	if len(hr.Output) > 4200 {
		t.Fatalf("host output not truncated: %d", len(hr.Output))
	}
	if len(hr.Steps[0].Output) > 4200 {
		t.Fatalf("step output not truncated: %d", len(hr.Steps[0].Output))
	}
	// Original must stay intact (clone).
	if len(e.HostResults["h1"].Output) < 8000 {
		t.Fatal("compact mutated original execution")
	}
}

func TestSummarizePlaybookExecutionStrips(t *testing.T) {
	e := PlaybookExecution{
		HostResults: map[string]HostExecResult{
			"h1": {Output: "keep-me", Steps: []StepResult{{Output: "step-out"}}},
		},
	}
	s := summarizePlaybookExecution(e)
	if s.HostResults["h1"].Output != "" || s.HostResults["h1"].Steps[0].Output != "" {
		t.Fatal("summarize should strip outputs")
	}
}

func TestTruncatePlaybookStoreOutputHostInspect(t *testing.T) {
	big := strings.Repeat("a", 20000)
	out := truncatePlaybookStoreOutput("host_inspect", big)
	if len(out) > playbookHostInspectStore+200 {
		t.Fatalf("store preview too large: %d", len(out))
	}
	if !strings.Contains(out, "主机巡检") {
		t.Fatal("expected host_inspect store note")
	}
}

func TestPlaybookHeavyModule(t *testing.T) {
	if !playbookHeavyModule("host_inspect") || !playbookHasHeavySteps([]PlaybookStep{{Module: "host_security_scan"}}) {
		t.Fatal("heavy module detection failed")
	}
	if playbookHeavyModule("shell") {
		t.Fatal("shell should not be heavy")
	}
}
