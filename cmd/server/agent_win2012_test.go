package main

import "testing"

func TestHostNeedsWin2012Agent(t *testing.T) {
	cases := []struct {
		platform string
		want     bool
	}{
		{"Windows Server 2012 R2 (Build 9600)", true},
		{"Windows Server 2012 (Build 9200)", true},
		{"Windows 8.1 (Build 9600)", true},
		{"Windows Server 2016 (Build 14393)", false},
		{"Windows 10 (Build 19045)", false},
		{"Windows Server 2022 (Build 20348)", false},
	}
	for _, tc := range cases {
		h := &Host{OS: "windows", Platform: tc.platform, Arch: "amd64"}
		if got := hostNeedsWin2012Agent(h); got != tc.want {
			t.Fatalf("%q: got %v want %v", tc.platform, got, tc.want)
		}
	}
	cands := agentDistCandidatesForHost(&Host{OS: "windows", Platform: "Windows Server 2012 R2 (Build 9600)", Arch: "amd64"})
	if len(cands) == 0 || cands[0] != "aiops-agent-windows-amd64-win2012.exe" {
		t.Fatalf("win2012 candidates first=%v", cands)
	}
}
