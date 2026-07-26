package main

import (
	"net"
	"testing"
)

func TestScoreIPv4PrefersLANOverAPIPA(t *testing.T) {
	lan := net.ParseIP("10.10.1.50").To4()
	apipa := net.ParseIP("169.254.33.136").To4()
	pub := net.ParseIP("203.0.113.8").To4()
	cgnat := net.ParseIP("100.64.1.2").To4()

	lanScore := scoreIPv4(lan, "Ethernet")
	apipaScore := scoreIPv4(apipa, "vEthernet (Default Switch)")
	pubScore := scoreIPv4(pub, "eth0")
	cgnatScore := scoreIPv4(cgnat, "eth0")
	dockerScore := scoreIPv4(lan, "docker0")

	if !(lanScore > apipaScore) {
		t.Fatalf("LAN score %d should beat APIPA %d", lanScore, apipaScore)
	}
	if !(pubScore > apipaScore) {
		t.Fatalf("public score %d should beat APIPA %d", pubScore, apipaScore)
	}
	if !(lanScore > cgnatScore) {
		t.Fatalf("LAN score %d should beat CGNAT %d", lanScore, cgnatScore)
	}
	if !(lanScore > dockerScore) {
		t.Fatalf("Ethernet LAN %d should beat docker0 %d", lanScore, dockerScore)
	}
}

func TestScoreIPv4LinkLocalHelper(t *testing.T) {
	ip := net.ParseIP("169.254.1.1").To4()
	if !ip.IsLinkLocalUnicast() {
		t.Fatal("expected Go to treat 169.254 as link-local")
	}
	if scoreIPv4(ip, "Ethernet") >= 0 {
		t.Fatalf("APIPA on Ethernet should still be heavily penalized, got %d", scoreIPv4(ip, "Ethernet"))
	}
}
