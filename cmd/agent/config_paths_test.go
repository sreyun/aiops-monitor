package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// A Windows service starts with CWD=C:\Windows\System32, so the installer's
// relative `state_file` / `plugins_dir` used to resolve outside the install dir:
// the identity landed in System32 and the freshly extracted plugins were never
// found. Every launch context must agree on the same absolute paths.
func TestResolveConfigRelativePathsAnchorsToConfigDir(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	c := defaultConfig()
	c.StateFile = "agent_state.json"
	c.PluginsDir = "plugins"
	resolveConfigRelativePaths(&c, cfgPath)

	if want := filepath.Join(dir, "agent_state.json"); c.StateFile != want {
		t.Fatalf("state_file = %q, want %q", c.StateFile, want)
	}
	if want := filepath.Join(dir, "plugins"); c.PluginsDir != want {
		t.Fatalf("plugins_dir = %q, want %q", c.PluginsDir, want)
	}
}

func TestResolveConfigRelativePathsKeepsAbsolute(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(t.TempDir(), "state.json")

	c := defaultConfig()
	c.StateFile = abs
	c.PluginsDir = ""
	resolveConfigRelativePaths(&c, filepath.Join(dir, "config.yaml"))

	if c.StateFile != abs {
		t.Fatalf("absolute state_file rewritten: %q", c.StateFile)
	}
	if c.PluginsDir != "" {
		t.Fatalf("empty plugins_dir must stay empty, got %q", c.PluginsDir)
	}
}

// Anchoring must not orphan hosts installed before the fix: their identity sits
// in the old working-directory-relative location, and minting a new host_id
// would split the host's history in two.
func TestMigrateLegacyStateFileAdoptsWorkingDirIdentity(t *testing.T) {
	t.Setenv("AIOPS_MACHINE_ID", "selftest-machine")
	legacyDir := t.TempDir()
	installDir := t.TempDir()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(legacyDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	legacy := filepath.Join(legacyDir, "agent_state.json")
	writeState(t, legacy, "legacy-host-id", machineFingerprint())

	got := loadOrCreateHostID(filepath.Join(installDir, "agent_state.json"))
	if got != "legacy-host-id" {
		t.Fatalf("host_id = %q, want the migrated legacy id", got)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy state file must be removed so a downgrade cannot report in parallel")
	}
	if id := readHostIDFromState(filepath.Join(installDir, "agent_state.json")); id != "legacy-host-id" {
		t.Fatalf("migrated state holds %q", id)
	}
}

func TestMigrateLegacyStateFileKeepsExistingIdentity(t *testing.T) {
	t.Setenv("AIOPS_MACHINE_ID", "selftest-machine")
	legacyDir := t.TempDir()
	installDir := t.TempDir()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(legacyDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	fp := machineFingerprint()
	writeState(t, filepath.Join(legacyDir, "agent_state.json"), "legacy-host-id", fp)
	current := filepath.Join(installDir, "agent_state.json")
	writeState(t, current, "current-host-id", fp)

	if got := loadOrCreateHostID(current); got != "current-host-id" {
		t.Fatalf("host_id = %q, want the already-anchored id", got)
	}
}

// A relative path used to guarantee a writable parent (the CWD). An anchored one
// does not, and a failed write silently minted a new host_id on every start.
func TestPersistHostIDCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "agent_state.json")
	persistHostID(path, "host-1", "fp-1")
	if id := readHostIDFromState(path); id != "host-1" {
		t.Fatalf("identity not persisted into a missing directory, got %q", id)
	}
}

func writeState(t *testing.T, path, id, fp string) {
	t.Helper()
	b, err := json.Marshal(map[string]string{"host_id": id, "fp": fp})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}
