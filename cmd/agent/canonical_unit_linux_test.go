//go:build linux

package main

import "testing"

func TestCanonicalUnitConstants(t *testing.T) {
	if agentServiceName != "aiops-agent" {
		t.Fatalf("canonical unit = %q", agentServiceName)
	}
	if systemdUnitPath != "/etc/systemd/system/aiops-agent.service" {
		t.Fatalf("unit path = %q", systemdUnitPath)
	}
	foundLegacy := false
	for _, n := range legacyAgentServiceNames {
		if n == "aiops-monitor-agent" {
			foundLegacy = true
		}
	}
	if !foundLegacy {
		t.Fatal("legacy aiops-monitor-agent must remain in heal/update list")
	}
}
