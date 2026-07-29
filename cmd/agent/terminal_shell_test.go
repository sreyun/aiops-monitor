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

func TestSetEnvKeyWindowsPathCase(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows env key folding")
	}
	// Simulate LocalSystem: Environ returns Path=…, old code appended PATH=…
	env := []string{`Path=C:\Weird`, "SystemRoot=C:\\Windows"}
	env = setEnvKey(env, "Path", mergeWindowsPath(`C:\Windows`, getEnvKey(env, "Path")))
	var pathCount int
	var pathVal string
	for _, e := range env {
		if envEntryKey(e, "Path") {
			pathCount++
			pathVal = e[strings.IndexByte(e, '=')+1:]
		}
	}
	if pathCount != 1 {
		t.Fatalf("expected 1 Path entry, got %d in %v", pathCount, env)
	}
	if !strings.Contains(strings.ToLower(pathVal), `c:\windows\system32`) {
		t.Fatalf("System32 missing: %q", pathVal)
	}
	if strings.EqualFold(pathVal, `C:\Weird`) {
		t.Fatalf("stale Path kept: %q", pathVal)
	}
}

func TestEnrichWindowsShellEnvReplacesPathNotDuplicates(t *testing.T) {
	env := enrichWindowsShellEnv([]string{`Path=`, `PATH=/usr/bin:/bin`})
	var n int
	for _, e := range env {
		if envEntryKey(e, "Path") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("duplicate Path/PATH entries: %d in %v", n, env)
	}
	path := getEnvKey(env, "Path")
	if isUnixStylePathEnv(path) || !strings.Contains(strings.ToLower(path), "system32") {
		t.Fatalf("PATH not repaired: %q", path)
	}
}

func TestIsUnixStylePathEnv(t *testing.T) {
	if !isUnixStylePathEnv("/usr/local/sbin:/usr/bin:/bin") {
		t.Fatal("expected unix PATH detection")
	}
	if isUnixStylePathEnv(`C:\Windows\System32;C:\Windows`) {
		t.Fatal("windows PATH misclassified")
	}
	if isUnixStylePathEnv("") {
		t.Fatal("empty should be false")
	}
}

func TestMergeWindowsPath(t *testing.T) {
	got := mergeWindowsPath(`C:\Windows`, "/usr/bin:/bin")
	if !strings.Contains(strings.ToLower(got), `c:\windows\system32`) {
		t.Fatalf("System32 missing: %q", got)
	}
	if strings.Contains(got, "/usr") {
		t.Fatalf("unix PATH leaked: %q", got)
	}
	got2 := mergeWindowsPath(`C:\Windows`, `D:\tools;C:\Windows\System32`)
	if !strings.HasPrefix(strings.ToLower(got2), `c:\windows\system32`) {
		t.Fatalf("essential dirs should lead: %q", got2)
	}
	if !strings.Contains(strings.ToLower(got2), `d:\tools`) {
		t.Fatalf("existing entry dropped: %q", got2)
	}
}

func TestPipeLocalEcho(t *testing.T) {
	got := string(pipeLocalEcho([]byte("ipconfig\r")))
	if got != "ipconfig\r\n" {
		t.Fatalf("got %q", got)
	}
	got = string(pipeLocalEcho([]byte{0x08}))
	if got != "\b \b" {
		t.Fatalf("backspace echo %q", got)
	}
}

func TestEnrichWindowsShellEnv(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows env enrichment")
	}
	env := enrichWindowsShellEnv([]string{"PATH=/usr/bin:/bin"})
	path := getEnvKey(env, "Path")
	if isUnixStylePathEnv(path) || !strings.Contains(strings.ToLower(path), "system32") {
		t.Fatalf("PATH not repaired: %q", path)
	}
	if getEnvKey(env, "SystemRoot") == "" {
		t.Fatal("SystemRoot missing")
	}
	if getEnvKey(env, "ComSpec") == "" {
		t.Fatal("ComSpec missing")
	}
}

func TestBuildShellEnvWindowsHasSystem32(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows")
	}
	env := buildShellEnv()
	path := getEnvKey(env, "Path")
	if !strings.Contains(strings.ToLower(path), "system32") {
		t.Fatalf("buildShellEnv PATH missing System32: %q", path)
	}
}

func TestWindowsShellInitCmdNoNestedQuotes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows")
	}
	s := windowsShellInitCmd()
	if strings.Contains(s, `set "Path=`) || strings.Contains(s, `set "PATH=`) {
		t.Fatalf("nested quotes break cmd /K: %q", s)
	}
	if !strings.Contains(strings.ToLower(s), "system32") {
		t.Fatalf("missing System32: %q", s)
	}
}

func TestDirUsableForShell(t *testing.T) {
	if !dirUsableForShell(t.TempDir()) {
		t.Fatal("temp dir should be usable")
	}
	if dirUsableForShell("") || dirUsableForShell("/no/such/dir/aiops-xyz") {
		t.Fatal("missing dirs must be unusable")
	}
	blocked := filepath.Join(t.TempDir(), "nope")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if dirUsableForShell(blocked) {
		t.Fatal("file must not count as shell cwd")
	}
}

func TestInteractiveShellDirFallsBack(t *testing.T) {
	// With a normal home this returns home; with HOME pointing at a file, must not.
	f := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", f)
	t.Setenv("USERPROFILE", f)
	got := interactiveShellDir()
	if got == f {
		t.Fatalf("interactiveShellDir returned blocked home %q", got)
	}
	if got != "" && !dirUsableForShell(got) {
		t.Fatalf("fallback cwd not usable: %q", got)
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
