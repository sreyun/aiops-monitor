package main

import (
	"strings"
	"testing"

	"aiops-monitor/shared"
)

func TestResolveDesktopTarget(t *testing.T) {
	win := &Host{OS: "windows"}
	proto, port, _ := resolveDesktopTarget(win)
	if proto != "rdp" || port != 3389 {
		t.Fatalf("windows default: %s %d", proto, port)
	}
	mac := &Host{OS: "darwin"}
	proto, port, _ = resolveDesktopTarget(mac)
	if proto != "vnc" || port != 5900 {
		t.Fatalf("darwin default: %s %d", proto, port)
	}
	lin := &Host{OS: "linux", Desktop: &shared.DesktopInfo{RDP: true, Preferred: "rdp", PreferredPort: 3389, Ports: []int{3389}}}
	proto, port, listening := resolveDesktopTarget(lin)
	if proto != "rdp" || port != 3389 || !listening {
		t.Fatalf("linux xrdp: %s %d listening=%v", proto, port, listening)
	}
	vnc := &Host{OS: "linux", Desktop: &shared.DesktopInfo{VNC: true, Preferred: "vnc", PreferredPort: 5901, Ports: []int{5901}}}
	proto, port, listening = resolveDesktopTarget(vnc)
	if proto != "vnc" || port != 5901 || !listening {
		t.Fatalf("linux vnc: %s %d listening=%v", proto, port, listening)
	}
}

func TestBuildRDPFile(t *testing.T) {
	s := buildRDPFile("10.0.0.1:13389", "web01")
	if !strings.Contains(s, "full address:s:10.0.0.1:13389") || !strings.Contains(s, "prompt for credentials:i:1") {
		t.Fatalf("rdp content unexpected: %q", s)
	}
}

func TestSanitizeDesktopFilename(t *testing.T) {
	got := sanitizeDesktopFilename("web 01/生产")
	if got == "" || strings.ContainsAny(got, "/\\") {
		t.Fatalf("got %q", got)
	}
}
