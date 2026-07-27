package main

import "testing"

func TestCountAptUpgradable(t *testing.T) {
	out := `Listing... Done
WARNING: apt does not have a stable CLI interface. Use with caution in scripts.
bash/stable 5.2.15-2+b7 amd64 [upgradable from: 5.2.15-2+b6]
libc6/stable 2.36-9+deb12u10 amd64 [upgradable from: 2.36-9+deb12u9]
`
	if n := countAptUpgradable(out); n != 2 {
		t.Fatalf("apt count = %d, want 2", n)
	}
	if n := countAptUpgradable("Listing... Done\n"); n != 0 {
		t.Fatalf("empty apt = %d", n)
	}
}

func TestCountDnfCheckUpdate(t *testing.T) {
	out := `Last metadata expiration check: 0:12:34 ago on Mon 01 Jan 2024.
kernel.x86_64                     5.14.0-427.el9          updates
bash.x86_64                       5.1.8-9.el9             baseos
Obsoleting Packages
oldpkg.x86_64                     1.0-1.el9               updates
`
	if n := countDnfCheckUpdate(out); n != 3 {
		t.Fatalf("dnf count = %d, want 3 (kernel, bash, oldpkg)", n)
	}
	if n := countDnfCheckUpdate("Last metadata expiration check: now\n"); n != 0 {
		t.Fatalf("header-only dnf = %d", n)
	}
}

func TestCountZypperAndLines(t *testing.T) {
	z := `Repository 'oss' is up to date.
v | S | Name | Type | Version | Arch | Repository
--+---+---+---+---+---+---
v | security | openssl | package | 3.0 | x86_64 | Update
v | recommended | curl | package | 8.0 | x86_64 | Update
`
	if n := countZypperUpdates(z); n != 2 {
		t.Fatalf("zypper = %d, want 2", n)
	}
	if n := countNonEmptyLines("a\n\nb\n"); n != 2 {
		t.Fatalf("lines = %d, want 2", n)
	}
}
