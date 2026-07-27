package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIsNonInteractiveShell(t *testing.T) {
	cases := map[string]bool{
		"/sbin/nologin":     true,
		"/usr/sbin/nologin": true,
		"/bin/false":        true,
		"/usr/bin/false":    true,
		"/bin/true":         true,
		"/bin/sync":         true,
		"/bin/bash":         false,
		"/bin/sh":           false,
		"/usr/bin/zsh":      false,
		"":                  true,
		"nologin":           true,
	}
	for path, want := range cases {
		if got := isNonInteractiveShell(path); got != want {
			t.Fatalf("%q → %v, want %v", path, got, want)
		}
	}
}

func TestShellPathRejectsNologinEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell selection")
	}
	t.Setenv("SHELL", "/usr/sbin/nologin")
	got := shellPath()
	if isNonInteractiveShell(got) {
		t.Fatalf("shellPath returned non-interactive %q under SHELL=nologin", got)
	}
	if !shellExists(got) && got != "/bin/sh" {
		t.Fatalf("shellPath returned missing binary %q", got)
	}
}

func TestShellPathRejectsFalseShellEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell selection")
	}
	t.Setenv("SHELL", "/bin/false")
	got := shellPath()
	if got == "/bin/false" || isNonInteractiveShell(got) {
		t.Fatalf("shellPath returned %q under SHELL=/bin/false", got)
	}
}

func TestBuildShellEnvForcesInteractiveSHELL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell env")
	}
	t.Setenv("SHELL", "/sbin/nologin")
	env := buildShellEnv()
	var shell string
	for _, e := range env {
		if strings.HasPrefix(e, "SHELL=") {
			shell = strings.TrimPrefix(e, "SHELL=")
			break
		}
	}
	if shell == "" {
		t.Fatal("SHELL missing from buildShellEnv")
	}
	if isNonInteractiveShell(shell) {
		t.Fatalf("buildShellEnv kept non-interactive SHELL=%q", shell)
	}
}

func TestSetEnvKey(t *testing.T) {
	env := []string{"PATH=/bin", "SHELL=/sbin/nologin"}
	env = setEnvKey(env, "SHELL", "/bin/bash")
	env = setEnvKey(env, "HOME", "/root")
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "SHELL=/bin/bash") {
		t.Fatal(joined)
	}
	if strings.Contains(joined, "SHELL=/sbin/nologin") {
		t.Fatal("old SHELL not replaced")
	}
	if !strings.Contains(joined, "HOME=/root") {
		t.Fatal(joined)
	}
}

func TestPasswdShellForUIDSelf(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no /etc/passwd")
	}
	if _, err := os.Stat("/etc/passwd"); err != nil {
		t.Skip("no /etc/passwd")
	}
	sh := passwdShellForUID(os.Getuid())
	// May be empty in exotic environments; when present must be a path-like string.
	if sh != "" && !strings.Contains(sh, string(filepath.Separator)) && sh[0] != '/' {
		t.Fatalf("unexpected passwd shell %q", sh)
	}
}
