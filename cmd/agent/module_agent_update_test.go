package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAgentConfigBesideExe(t *testing.T) {
	dir := t.TempDir()
	if got := resolveAgentConfigBesideExe(dir); got != "" {
		t.Fatalf("empty dir → %q", got)
	}
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, []byte("server: http://x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := resolveAgentConfigBesideExe(dir)
	want, _ := filepath.Abs(cfg)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestAgentDistBinaryName(t *testing.T) {
	cases := []struct{ goos, goarch, want string }{
		{"linux", "amd64", "aiops-agent-linux-amd64"},
		{"linux", "arm64", "aiops-agent-linux-arm64"},
		{"darwin", "arm64", "aiops-agent-darwin-arm64"},
		{"windows", "amd64", "aiops-agent.exe"},
		{"windows", "arm64", "aiops-agent-windows-arm64.exe"},
	}
	for _, c := range cases {
		got, err := agentDistBinaryName(c.goos, c.goarch)
		if err != nil || got != c.want {
			t.Fatalf("%s/%s → %q (%v), want %q", c.goos, c.goarch, got, err, c.want)
		}
	}
	if _, err := agentDistBinaryName("linux", "386"); err == nil {
		t.Fatal("expected error for linux/386")
	}
}

func TestAgentUpdateBinCandidatesWindowsAliases(t *testing.T) {
	cands := agentUpdateBinCandidates("windows", "amd64", "aiops-agent-windows-amd64.exe")
	if len(cands) < 2 {
		t.Fatalf("cands=%v", cands)
	}
	if cands[0] != "aiops-agent-windows-amd64.exe" {
		t.Fatalf("preferred first: %v", cands)
	}
	foundExe := false
	for _, c := range cands {
		if c == "aiops-agent.exe" {
			foundExe = true
		}
	}
	if !foundExe {
		t.Fatalf("missing aiops-agent.exe alias: %v", cands)
	}
}

func TestAgentUpdateBinCandidatesEmptyPreferred(t *testing.T) {
	cands := agentUpdateBinCandidates("linux", "amd64", "")
	if len(cands) != 1 || cands[0] != "aiops-agent-linux-amd64" {
		t.Fatalf("empty preferred: %v", cands)
	}
	// filepath.Base("") is "."; must not become a download candidate.
	cands = agentUpdateBinCandidates("linux", "amd64", "   ")
	if len(cands) != 1 || cands[0] != "aiops-agent-linux-amd64" {
		t.Fatalf("whitespace preferred: %v", cands)
	}
}

func TestNormalizeAgentVer(t *testing.T) {
	if normalizeAgentVer("v0.19.3") != "0.19.3" {
		t.Fatal(normalizeAgentVer("v0.19.3"))
	}
}

func TestValidateUpdateServerURL(t *testing.T) {
	if err := validateUpdateServerURL("http://mon.example:8529", []string{"http://mon.example:8529"}); err != nil {
		t.Fatal(err)
	}
	if err := validateUpdateServerURL("http://evil.example", []string{"http://mon.example:8529"}); err == nil {
		t.Fatal("expected reject")
	}
	if err := validateUpdateServerURL("ftp://mon.example", nil); err == nil {
		t.Fatal("expected scheme reject")
	}
}
