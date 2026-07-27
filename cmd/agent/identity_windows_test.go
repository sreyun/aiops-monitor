//go:build windows

package main

import (
	"strings"
	"testing"
)

func TestReadMachineGuidRegistry(t *testing.T) {
	id := readMachineGuidRegistry()
	if id == "" {
		// Extremely unusual on a real Windows host; still don't hard-fail CI VMs
		// that might strip Cryptography keys in locked-down images.
		t.Skip("MachineGuid empty on this host; skipping")
	}
	// GUID form: 8-4-4-4-12 hex with dashes.
	if !strings.Contains(id, "-") || len(id) < 32 {
		t.Fatalf("MachineGuid looks invalid: %q", id)
	}
	fp := machineFingerprint()
	if fp == "" {
		t.Fatal("fingerprint must be non-empty when MachineGuid is readable")
	}
}

func TestMachineIDFromOSPrefersRegistry(t *testing.T) {
	id := machineIDFromOS()
	if id == "" {
		t.Skip("no MachineGuid available")
	}
	reg := readMachineGuidRegistry()
	if reg != "" && id != reg {
		// reg.exe fallback may differ only in whitespace; normalize.
		if strings.TrimSpace(id) != strings.TrimSpace(reg) {
			t.Fatalf("machineIDFromOS=%q registry=%q", id, reg)
		}
	}
}
