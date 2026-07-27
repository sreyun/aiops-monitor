package main

import "testing"

func TestHostDistroRockyKylin(t *testing.T) {
	rocky9 := &Host{OS: "linux", Platform: "Rocky Linux 9.4 (Blue Onyx)"}
	d := hostDistro(rocky9)
	if d.ID != "rocky" || d.Family != "rhel" || d.Version != "9" {
		t.Fatalf("rocky9 profile = %+v", d)
	}
	rocky10 := &Host{OS: "linux", Platform: "Rocky Linux 10.0"}
	d = hostDistro(rocky10)
	if d.ID != "rocky" || d.Version != "10" {
		t.Fatalf("rocky10 profile = %+v", d)
	}
	kylin10 := &Host{OS: "linux", Platform: "Kylin Linux Advanced Server V10 (Sword)"}
	d = hostDistro(kylin10)
	if d.ID != "kylin" || d.Family != "kylin" || d.Version != "10" {
		t.Fatalf("kylin10 profile = %+v", d)
	}
	kylin11 := &Host{OS: "linux", Platform: "Kylin Linux Advanced Server V11"}
	d = hostDistro(kylin11)
	if d.ID != "kylin" || d.Version != "11" {
		t.Fatalf("kylin11 profile = %+v", d)
	}
}

func TestHostDistroMatrix(t *testing.T) {
	cases := []struct {
		platform   string
		wantID     string
		wantFamily string
		wantVer    string
	}{
		{"openEuler 22.03 LTS", "openeuler", "rhel", "22"},
		{"openEuler 24.03 LTS", "openeuler", "rhel", "24"},
		{"EulerOS 2.0 (SP10 x86_64)", "euleros", "rhel", "2"},
		{"EulerOS 2.5", "euleros", "rhel", "2"},
		{"EulerOS 3.0", "euleros", "rhel", "3"},
		{"Alibaba Cloud Linux 2.1903", "alinux", "rhel", "2"},
		{"Alibaba Cloud Linux 3.2104 LTS", "alinux", "rhel", "3"},
		{"Alibaba Cloud Linux 4", "alinux", "rhel", "4"},
		{"CentOS Linux 7 (Core)", "centos", "rhel", "7"},
		{"CentOS Linux 8", "centos", "rhel", "8"},
		{"Debian GNU/Linux 10 (buster)", "debian", "debian", "10"},
		{"Debian GNU/Linux 11 (bullseye)", "debian", "debian", "11"},
		{"Debian GNU/Linux 12 (bookworm)", "debian", "debian", "12"},
		{"Debian GNU/Linux 13 (trixie)", "debian", "debian", "13"},
		{"Kylin Linux Desktop V10", "kylin", "kylin", "10"},
		{"Kylin Linux Desktop V11", "kylin", "kylin", "11"},
	}
	for _, c := range cases {
		d := hostDistro(&Host{OS: "linux", Platform: c.platform})
		if d.ID != c.wantID || d.Family != c.wantFamily || d.Version != c.wantVer {
			t.Errorf("%q → %+v, want id=%s family=%s ver=%s", c.platform, d, c.wantID, c.wantFamily, c.wantVer)
		}
	}
}

func TestHostDistroWindowsMacOS(t *testing.T) {
	cases := []struct {
		os, platform, wantVer string
	}{
		{"windows", "Windows Server 2012 R2 (Build 9600)", "2012"},
		{"windows", "Windows Server 2016 (Build 14393)", "2016"},
		{"windows", "Windows Server 2019 (Build 17763)", "2019"},
		{"windows", "Windows Server 2022 (Build 20348)", "2022"},
		{"windows", "Windows Server 2025 (Build 26100)", "2025"},
		{"windows", "Windows 10 (Build 19045)", "10"},
		{"windows", "Windows 11 (Build 26100)", "11"},
		{"darwin", "macOS 12.7.6", "12"},
		{"darwin", "macOS 13.6", "13"},
		{"darwin", "macOS 14.5", "14"},
		{"darwin", "macOS 15.2", "15"},
	}
	for _, c := range cases {
		d := hostDistro(&Host{OS: c.os, Platform: c.platform})
		if d.Version != c.wantVer {
			t.Errorf("%s/%q version=%q want %q (profile=%+v)", c.os, c.platform, d.Version, c.wantVer, d)
		}
	}
}

func TestHostDistroTrustsGOOSOverPlatformText(t *testing.T) {
	// WSL / wine-like platform text must not flip a linux agent to windows.
	d := hostDistro(&Host{OS: "linux", Platform: "Ubuntu 22.04 on Windows Subsystem for Linux"})
	if d.GOOS != "linux" || d.ID != "ubuntu" || d.Family != "debian" {
		t.Fatalf("WSL-like host misclassified: %+v", d)
	}
	d = hostDistro(&Host{OS: "windows", Platform: "Windows Server 2022"})
	if d.GOOS != "windows" || d.Version != "2022" {
		t.Fatalf("windows host = %+v", d)
	}
}

func TestNormalizeHostDistroMajorNoVTokenSteal(t *testing.T) {
	if got := normalizeHostDistroMajor("openEuler 22.03 with libv10"); got != "22" {
		t.Fatalf("got %q, want 22", got)
	}
	if got := normalizeHostDistroMajor("Kylin Linux Advanced Server V10"); got != "10" {
		t.Fatalf("kylin V10 got %q", got)
	}
	if got := normalizeHostDistroMajor("V11"); got != "11" {
		t.Fatalf("leading V11 got %q", got)
	}
}

func TestMatchHostSystemSelector(t *testing.T) {
	rocky := &Host{OS: "linux", Platform: "Rocky Linux 9.3"}
	kylin := &Host{OS: "linux", Platform: "Kylin Linux Advanced Server V10"}
	ubuntu := &Host{OS: "linux", Platform: "Ubuntu 22.04.3 LTS"}
	win := &Host{OS: "windows", Platform: "Windows Server 2022"}
	oe := &Host{OS: "linux", Platform: "openEuler 22.03 LTS"}
	eu := &Host{OS: "linux", Platform: "EulerOS 2.0 (SP10)"}
	ali := &Host{OS: "linux", Platform: "Alibaba Cloud Linux 3"}
	mac := &Host{OS: "darwin", Platform: "macOS 15.1"}

	if !matchHostSystemSelector(rocky, "linux") || !matchHostSystemSelector(rocky, "rocky") || !matchHostSystemSelector(rocky, "rhel") {
		t.Fatal("rocky should match linux/rocky/rhel")
	}
	if matchHostSystemSelector(rocky, "kylin") || matchHostSystemSelector(rocky, "windows") {
		t.Fatal("rocky should not match kylin/windows")
	}
	if !matchHostSystemSelector(kylin, "kylin") || !matchHostSystemSelector(kylin, "linux") {
		t.Fatal("kylin should match kylin/linux")
	}
	if matchHostSystemSelector(kylin, "rocky") {
		t.Fatal("kylin should not match rocky")
	}
	if !matchHostSystemSelector(ubuntu, "linux") || !matchHostSystemSelector(ubuntu, "ubuntu") {
		t.Fatal("ubuntu should match linux/ubuntu")
	}
	if matchHostSystemSelector(ubuntu, "rocky") {
		t.Fatal("ubuntu should not match rocky")
	}
	if !matchHostSystemSelector(win, "windows") || matchHostSystemSelector(win, "linux") {
		t.Fatal("windows selector mismatch")
	}
	if !matchHostSystemSelector(oe, "openeuler") || !matchHostSystemSelector(oe, "openeuler:22") || matchHostSystemSelector(oe, "openeuler:24") {
		t.Fatal("openEuler version selector mismatch")
	}
	if !matchHostSystemSelector(eu, "euleros") || matchHostSystemSelector(eu, "openeuler") {
		t.Fatal("EulerOS must not be classified as openEuler")
	}
	if !matchHostSystemSelector(ali, "alinux") || !matchHostSystemSelector(ali, "alinux:3") || !matchHostSystemSelector(ali, "rhel") {
		t.Fatal("Alibaba Cloud Linux selector mismatch")
	}
	if !matchHostSystemSelector(mac, "macos") || !matchHostSystemSelector(mac, "macos:15") || matchHostSystemSelector(mac, "macos:14") {
		t.Fatal("macOS version selector mismatch")
	}
	if !matchHostSystemSelector(win, "windows:2022") || matchHostSystemSelector(win, "windows:2019") {
		t.Fatal("Windows Server year selector mismatch")
	}
	if !matchHostSystemSelector(rocky, "rocky:9") || matchHostSystemSelector(rocky, "rocky:10") {
		t.Fatal("rocky version selector mismatch")
	}
}

func TestPlaybookHostVarsDistro(t *testing.T) {
	h := &Host{ID: "h1", Hostname: "r9", OS: "linux", Platform: "Rocky Linux 9.4", IP: "10.0.0.1", Category: "db", Arch: "amd64"}
	vars := playbookHostVars(h)
	if vars["os"] != "linux" || vars["distro"] != "rocky" || vars["distro_version"] != "9" || vars["os_family"] != "rhel" {
		t.Fatalf("vars = %#v", vars)
	}
	if vars["arch"] != "amd64" {
		t.Fatalf("arch = %q", vars["arch"])
	}
	if !evalPlaybookWhen("{{distro}} == rocky", vars) {
		t.Fatal("distro == rocky should match")
	}
	if !evalPlaybookWhen("{{distro_version}} >= 9", vars) {
		t.Fatal("distro_version >= 9 should match")
	}
	if evalPlaybookWhen("{{distro}} == kylin", vars) {
		t.Fatal("rocky should not match distro == kylin")
	}
	kylin := playbookHostVars(&Host{OS: "linux", Platform: "Kylin Linux Advanced Server V11"})
	if !evalPlaybookWhen("{{distro}} == kylin", kylin) || !evalPlaybookWhen("{{distro_version}} >= 10", kylin) {
		t.Fatalf("kylin vars = %#v", kylin)
	}
	if !evalPlaybookWhen("{{platform}} contains V11", kylin) {
		t.Fatal("platform contains V11 should match")
	}
}

func TestResolveTargetsDistro(t *testing.T) {
	pm := &playbookManager{}
	hosts := []*Host{
		{ID: "1", OS: "linux", Platform: "Rocky Linux 10.0"},
		{ID: "2", OS: "linux", Platform: "Kylin Linux Advanced Server V10"},
		{ID: "3", OS: "linux", Platform: "Ubuntu 22.04"},
		{ID: "4", OS: "windows", Platform: "Windows 11"},
		{ID: "5", OS: "linux", Platform: "openEuler 24.03 LTS"},
		{ID: "6", OS: "linux", Platform: "EulerOS 3.0"},
	}
	got := pm.ResolveTargets("system:rocky", hosts)
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("system:rocky = %#v", got)
	}
	got = pm.ResolveTargets("system:kylin", hosts)
	if len(got) != 1 || got[0].ID != "2" {
		t.Fatalf("system:kylin = %#v", got)
	}
	got = pm.ResolveTargets("system:linux", hosts)
	if len(got) != 5 {
		t.Fatalf("system:linux count = %d, want 5", len(got))
	}
	got = pm.ResolveTargets("system:openeuler:24", hosts)
	if len(got) != 1 || got[0].ID != "5" {
		t.Fatalf("system:openeuler:24 = %#v", got)
	}
	got = pm.ResolveTargets("system:euleros", hosts)
	if len(got) != 1 || got[0].ID != "6" {
		t.Fatalf("system:euleros = %#v", got)
	}
	if !validPlaybookTarget("system:rocky") || !validPlaybookTarget("system:kylin") {
		t.Fatal("validPlaybookTarget should accept rocky/kylin")
	}
	if !validPlaybookTarget("system:openeuler:22") || !validPlaybookTarget("system:windows:2025") || !validPlaybookTarget("system:alinux") {
		t.Fatal("validPlaybookTarget should accept versioned / new distro selectors")
	}
}
