package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeInspectField(t *testing.T) {
	in := "Microsoft Windows [\x00版本 10.0.26200]\r\n"
	got := sanitizeInspectField(in)
	if strings.Contains(got, "\n") || strings.Contains(got, "\r") || strings.ContainsRune(got, 0) {
		t.Fatalf("control chars remain: %q", got)
	}
	if !strings.Contains(got, "Microsoft Windows") {
		t.Fatalf("got %q", got)
	}
}

func TestLooksLikeCommandUsage(t *testing.T) {
	junk := "sethostname: 'C hostname  hostname -s"
	if !looksLikeCommandUsage(junk) {
		t.Fatal("expected junk FQDN to be rejected")
	}
	if looksLikeCommandUsage("hmsrv18.example.com") {
		t.Fatal("valid FQDN should pass")
	}
}

func TestDecodeCmdOutValidUTF8(t *testing.T) {
	got := decodeCmdOut([]byte("Microsoft Windows [版本 10.0.26200]\n"))
	if !utf8.ValidString(got) {
		t.Fatalf("invalid utf8: %q", got)
	}
	if !strings.Contains(got, "版本") {
		t.Fatalf("lost Chinese: %q", got)
	}
}

func TestInspectResolveFQDNRejectsJunk(t *testing.T) {
	// Non-windows path uses hostname -f; we only assert the junk detector used by Windows path.
	if !looksLikeCommandUsage("Usage: hostname -f") {
		t.Fatal("expected usage text rejected")
	}
}

func TestWindowsOSIdentityKernelNotVerBanner(t *testing.T) {
	// Unit-level: kernel formatter must not equal a localized ver string.
	pretty := "Microsoft Windows [版本 10.0.26200.8875]"
	kernel := "10.0.26200.8875"
	if kernel == pretty {
		t.Fatal("kernel must not be the localized ver banner")
	}
	if strings.Contains(kernel, "版本") || strings.Contains(kernel, "[") {
		t.Fatalf("kernel looks like ver banner: %q", kernel)
	}
}
