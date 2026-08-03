//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestLinuxUnitNeedsPrivilegeHeal(t *testing.T) {
	good := `[Service]
User=root
ProtectHome=false
ProtectSystem=false
PrivateTmp=false
NoNewPrivileges=false
`
	if linuxUnitNeedsPrivilegeHeal(good, false) {
		t.Fatal("good unit should not need heal")
	}
	badUser := `[Service]
User=alice
ProtectHome=false
ProtectSystem=false
PrivateTmp=false
NoNewPrivileges=false
`
	if !linuxUnitNeedsPrivilegeHeal(badUser, false) {
		t.Fatal("non-root User must heal")
	}
	if linuxUnitNeedsPrivilegeHeal(badUser, true) {
		t.Fatal("allow-nonroot + unlock should not heal")
	}
	if !linuxUnitNeedsPrivilegeHeal("ProtectSystem=strict\nProtectHome=false\n", false) {
		t.Fatal("ProtectSystem=strict must heal")
	}
}

func TestHealLinuxUnitBody(t *testing.T) {
	in := `[Unit]
Description=AIOps Agent
[Service]
Type=simple
User=ubuntu
Group=ubuntu
Environment=HOME=/home/ubuntu
Environment=USER=ubuntu
Environment=LOGNAME=ubuntu
ExecStart=/opt/aiops-agent/aiops-agent --config /opt/aiops-agent/config.yaml
ProtectHome=read-only
ProtectSystem=strict
PrivateTmp=true
NoNewPrivileges=true
CapabilityBoundingSet=CAP_NET_RAW
[Install]
WantedBy=multi-user.target
`
	out, changed := healLinuxUnitBody(in, false)
	if !changed {
		t.Fatal("expected change")
	}
	for _, want := range []string{
		"User=root",
		"Group=root",
		"ProtectHome=false",
		"ProtectSystem=false",
		"PrivateTmp=false",
		"NoNewPrivileges=false",
		"Environment=HOME=/root",
		"Environment=USER=root",
		"ExecStart=/opt/aiops-agent/aiops-agent --config /opt/aiops-agent/config.yaml",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "CapabilityBoundingSet=") {
		t.Fatal("CapabilityBoundingSet should be stripped")
	}
	if strings.Contains(out, "ProtectHome=read-only") || strings.Contains(out, "User=ubuntu") {
		t.Fatal("old sandbox/user lines must be gone")
	}
}
