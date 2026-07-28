//go:build windows

package main

import (
	"strings"
	"testing"
)

func TestBuildWindowsUpdateHelperScriptPrefersServiceConfig(t *testing.T) {
	script := buildWindowsUpdateHelperScript(
		`C:\Program Files\AIOps Agent\aiops-agent.exe`,
		`C:\Program Files\AIOps Agent\.aiops-agent.new.exe`,
		`C:\Program Files\AIOps Agent\config.yaml`,
		`C:\Program Files\AIOps Agent\aiops-agent-update.log`,
	)
	for _, want := range []string{
		"--install-service",
		"--config",
		"AiopsMonitorAgent",
		"WorkingDirectory",
		"agent failed to restart after binary replace",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("helper script missing %q", want)
		}
	}
	// Legacy bug: bare Start-Process $exe without config broke terminal/desktop.
	if strings.Contains(script, "Start-Process $exe -WindowStyle Hidden") {
		t.Fatal("helper still has bare Start-Process without --config")
	}
	if !strings.Contains(script, "refusing bare Start-Process") {
		t.Fatal("helper must refuse config-less restart")
	}
	if !strings.Contains(script, "staging --version") {
		t.Fatal("helper must probe staging binary before swap")
	}
	if !strings.Contains(script, "Move-Item attempt") {
		t.Fatal("helper must retry Move-Item under AV locks")
	}
	// Pre-swap failures must not restore a leftover .bak over a still-good PE.
	if !strings.Contains(script, "$swapped = $false") || !strings.Contains(script, "$swapped = $true") {
		t.Fatal("helper must track $swapped around Move-Item")
	}
	if !strings.Contains(script, "$swapped -or -not (Test-Path -LiteralPath $exe)") {
		t.Fatal("helper must restore .bak only after swap or when exe is missing")
	}
	if strings.Contains(script, "elseif ((Test-Path -LiteralPath $bak))") {
		t.Fatal("helper must not unconditionally restore .bak whenever it exists")
	}
}
