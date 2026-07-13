package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModuleGatherFacts(t *testing.T) {
	out, exit := moduleGatherFacts()
	if exit != 0 {
		t.Fatalf("gather_facts exit = %d, want 0", exit)
	}
	s := string(out)
	for _, key := range []string{"hostname=", "os=", "arch=", "cpus=", "ip=", "ips="} {
		if !strings.Contains(s, key) {
			t.Errorf("gather_facts output missing %q:\n%s", key, s)
		}
	}
}

func TestModuleCopy(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "sub", "cfg.conf") // parent dir must be auto-created
	content := "line1\nline2\n"
	out, exit := moduleCopy(map[string]string{"dest": dest, "content": content, "mode": "0600"})
	if exit != 0 {
		t.Fatalf("copy exit = %d, want 0 (%s)", exit, out)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != content {
		t.Errorf("content = %q, want %q", got, content)
	}
	// Missing dest → failure.
	if _, exit := moduleCopy(map[string]string{"content": "x"}); exit == 0 {
		t.Error("copy without dest should fail")
	}
}

func TestRunModuleDispatch(t *testing.T) {
	a := &Agent{}
	// Bad JSON → failure.
	if _, exit := a.runModule("{not json"); exit == 0 {
		t.Error("bad JSON payload should fail")
	}
	// Unknown module → failure.
	if _, exit := a.runModule(`{"module":"nope","args":{}}`); exit == 0 {
		t.Error("unknown module should fail")
	}
	// gather_facts dispatches and succeeds.
	if out, exit := a.runModule(`{"module":"gather_facts","args":{}}`); exit != 0 || !strings.Contains(string(out), "os=") {
		t.Errorf("gather_facts dispatch failed: exit=%d out=%s", exit, out)
	}
}

func TestModuleArgValidation(t *testing.T) {
	if _, exit := moduleService(map[string]string{}); exit == 0 {
		t.Error("service without name should fail")
	}
	if _, exit := modulePackage(map[string]string{}); exit == 0 {
		t.Error("package without name should fail")
	}
}
