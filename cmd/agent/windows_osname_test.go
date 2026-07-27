package main

import "testing"

func TestFormatWindowsOSName(t *testing.T) {
	cases := []struct {
		maj, min, build uint32
		pt              byte
		want            string
	}{
		{6, 2, 9200, verNTServer, "Windows Server 2012 (Build 9200)"},
		{6, 3, 9600, verNTServer, "Windows Server 2012 R2 (Build 9600)"},
		{10, 0, 14393, verNTServer, "Windows Server 2016 (Build 14393)"},
		{10, 0, 17763, verNTServer, "Windows Server 2019 (Build 17763)"},
		{10, 0, 20348, verNTServer, "Windows Server 2022 (Build 20348)"},
		{10, 0, 26100, verNTServer, "Windows Server 2025 (Build 26100)"},
		{10, 0, 19045, verNTWorkstation, "Windows 10 (Build 19045)"},
		{10, 0, 22631, verNTWorkstation, "Windows 11 (Build 22631)"},
		{10, 0, 26100, verNTWorkstation, "Windows 11 (Build 26100)"},
		{10, 0, 26200, verNTWorkstation, "Windows 11 (Build 26200)"},
		{6, 2, 9200, verNTWorkstation, "Windows 8 (Build 9200)"},
		// A promoted domain controller reports wProductType=2, not 3. Treating it
		// as a workstation would label every DC in the fleet "Windows 10/11".
		{10, 0, 17763, verNTDomainController, "Windows Server 2019 (Build 17763)"},
		{10, 0, 20348, verNTDomainController, "Windows Server 2022 (Build 20348)"},
		{6, 2, 9200, verNTDomainController, "Windows Server 2012 (Build 9200)"},
	}
	for _, c := range cases {
		got := formatWindowsOSName(c.maj, c.min, c.build, c.pt)
		if got != c.want {
			t.Errorf("format(%d.%d.%d pt=%d)=%q want %q", c.maj, c.min, c.build, c.pt, got, c.want)
		}
	}
}
