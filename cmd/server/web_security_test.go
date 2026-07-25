package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHumanizeNucleiErr(t *testing.T) {
	msg := humanizeNucleiErr("nuclei: [\x1b[1;31mFTL\x1b[0m] Could not run nuclei: no templates provided for scan", nil)
	if strings.Contains(msg, "\x1b") || strings.Contains(msg, "[1;31m") {
		t.Fatalf("ANSI not stripped: %q", msg)
	}
	if !strings.Contains(msg, "模板") {
		t.Fatalf("expected Chinese template hint, got %q", msg)
	}
}

func TestZhWebSecErr(t *testing.T) {
	got := zhWebSecErr("nuclei: [\x1b[31mFTL\x1b[0m] no templates provided for scan")
	if strings.Contains(got, "\x1b") || strings.Contains(strings.ToLower(got), "no templates provided") {
		t.Fatalf("raw English/ANSI leaked: %q", got)
	}
}

func TestBuildNucleiTemplateArgsTagsMapToDirs(t *testing.T) {
	root := t.TempDir()
	for _, sub := range []string{"http/misconfiguration", "http/exposures", "ssl", "http/exposed-panels"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(sub)), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	args := buildNucleiTemplateArgs(root, WebScanTarget{Tags: []string{"misconfig", "exposures"}})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "misconfiguration") || !strings.Contains(joined, "exposures") || !strings.Contains(joined, "ssl") {
		t.Fatalf("expected mapped dirs incl. ssl, got %v", args)
	}
	if strings.Contains(joined, "-tags") {
		t.Fatalf("should not need -tags when dirs mapped: %v", args)
	}
}

func TestNucleiTemplatesReady(t *testing.T) {
	root := t.TempDir()
	httpDir := filepath.Join(root, "http", "misconfiguration")
	if err := os.MkdirAll(httpDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if nucleiTemplatesReady(root) {
		t.Fatal("empty tree should not be ready")
	}
	for i := 0; i < 25; i++ {
		name := filepath.Join(httpDir, fmt.Sprintf("t%d.yaml", i))
		if err := os.WriteFile(name, []byte("id: t\n"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	if !nucleiTemplatesReady(root) {
		t.Fatal("expected ready after yaml files present")
	}
}

func TestAllocScanMetaReadable(t *testing.T) {
	m := newWebScanManager(t.TempDir(), 1)
	id, label, seq := m.allocScanMeta("芒果系统")
	if seq != 1 {
		t.Fatalf("seq=%d", seq)
	}
	if !strings.HasPrefix(id, "ws-001-") {
		t.Fatalf("id not readable: %s", id)
	}
	if !strings.Contains(label, "芒果系统") || !strings.Contains(label, "#001") {
		t.Fatalf("label=%q", label)
	}
}
