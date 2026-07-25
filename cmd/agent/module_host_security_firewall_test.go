package main

import "testing"

func TestParseMacOSFirewallState(t *testing.T) {
	if parseMacOSFirewallState("Firewall is enabled. (State = 1)") != "on" {
		t.Fatal("enabled")
	}
	if parseMacOSFirewallState("Firewall is disabled. (State = 0)") != "off" {
		t.Fatal("disabled")
	}
}

func TestParseUFWAndFirewalld(t *testing.T) {
	if parseUFWStatus("Status: active\nTo\tAction\n22\tALLOW") != "on" {
		t.Fatal("ufw on")
	}
	if parseUFWStatus("Status: inactive") != "off" {
		t.Fatal("ufw off")
	}
	if parseFirewalldState("running") != "on" || parseFirewalldState("not running") != "off" {
		t.Fatal("firewalld")
	}
}

func TestParseIptablesInput(t *testing.T) {
	off := "Chain INPUT (policy ACCEPT)\ntarget     prot opt source               destination\n"
	if parseIptablesInput(off) != "off" {
		t.Fatalf("want off, got %s", parseIptablesInput(off))
	}
	on := "Chain INPUT (policy DROP)\ntarget     prot opt source               destination\nACCEPT     tcp  --  0.0.0.0/0            0.0.0.0/0\n"
	if parseIptablesInput(on) != "on" {
		t.Fatal("drop policy")
	}
}

func TestParseWindowsFirewallState(t *testing.T) {
	allOn := `
Domain Profile Settings:
State                                 ON
Private Profile Settings:
State                                 ON
Public Profile Settings:
State                                 ON
`
	if parseWindowsFirewallState(allOn) != "on" {
		t.Fatal("all on")
	}
	mixed := `
Domain Profile Settings:
State                                 ON
Public Profile Settings:
State                                 OFF
`
	if parseWindowsFirewallState(mixed) != "partial" {
		t.Fatal("partial")
	}
}

func TestFirewallFindingsOff(t *testing.T) {
	fs := firewallFindings(hostSecFirewall{Status: "off", Engine: "ufw", Detail: "Status: inactive"})
	if len(fs) != 1 || fs[0].ID != "firewall_off" {
		t.Fatalf("%+v", fs)
	}
}
