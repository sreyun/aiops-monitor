package main

import (
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestLegacyWindowsAgentUpdateScriptUsesInstallService(t *testing.T) {
	cmd := legacyWindowsAgentUpdateScript("http://mon.example:8529", "aiops-agent.exe")
	const marker = "powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand "
	idx := strings.Index(cmd, marker)
	if idx < 0 {
		t.Fatalf("expected absolute powershell.exe EncodedCommand, got: %s", cmd[:min(120, len(cmd))])
	}
	if !strings.Contains(cmd, `%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe`) {
		t.Fatal("legacy windows update must use absolute powershell path")
	}
	raw, err := base64.StdEncoding.DecodeString(cmd[idx+len(marker):])
	if err != nil {
		t.Fatal(err)
	}
	u16 := make([]uint16, len(raw)/2)
	for i := range u16 {
		u16[i] = uint16(raw[i*2]) | uint16(raw[i*2+1])<<8
	}
	ps := string(utf16.Decode(u16))
	for _, want := range []string{"--install-service", "--config", "WorkingDirectory"} {
		if !strings.Contains(ps, want) {
			t.Fatalf("legacy windows script missing %q", want)
		}
	}
	if strings.Contains(ps, "Start-Process $Exe -WindowStyle Hidden") {
		t.Fatal("legacy script still starts agent without --config")
	}
}

func TestLegacyUnixAgentUpdateScriptPrefersInstallService(t *testing.T) {
	sh := legacyUnixAgentUpdateScript("http://mon.example:8529", "aiops-agent-linux-amd64", false)
	if !strings.Contains(sh, "--install-service") {
		t.Fatal("linux legacy restart missing --install-service")
	}
	if !strings.Contains(sh, "--config") {
		t.Fatal("linux legacy restart missing --config")
	}
}
