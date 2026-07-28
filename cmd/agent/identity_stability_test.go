package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadOrCreateHostIDKeepsIDWhenMachineIDStable(t *testing.T) {
	t.Setenv("AIOPS_MACHINE_ID", "stable-machine-guid-001")
	dir := t.TempDir()
	path := filepath.Join(dir, "agent_state.json")

	id1 := loadOrCreateHostID(path)
	if id1 == "" {
		t.Fatal("empty host id")
	}
	// Simulate fingerprint formula / MAC-order change while MachineGuid (mid) stays.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]string
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	s["fp"] = "old-flapping-fingerprint"
	raw, _ := json.Marshal(s)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	id2 := loadOrCreateHostID(path)
	if id2 != id1 {
		t.Fatalf("host_id changed on fp flap: %q → %q (mid should keep identity)", id1, id2)
	}
	b2, _ := os.ReadFile(path)
	var s2 map[string]string
	_ = json.Unmarshal(b2, &s2)
	if s2["fp"] == "old-flapping-fingerprint" {
		t.Fatal("expected fp to be refreshed to current machineFingerprint")
	}
	if s2["mid"] != "stable-machine-guid-001" {
		t.Fatalf("mid not persisted: %q", s2["mid"])
	}
}

func TestLoadOrCreateHostIDLegacyFPFlapBackfillsMID(t *testing.T) {
	t.Setenv("AIOPS_MACHINE_ID", "legacy-upgrade-mid")
	dir := t.TempDir()
	path := filepath.Join(dir, "agent_state.json")
	// Legacy file: host_id + fp only (no mid), fp no longer matches.
	raw, _ := json.Marshal(map[string]string{
		"host_id": "keep-this-host",
		"fp":      "stale-mac-based-fp",
	})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	got := loadOrCreateHostID(path)
	if got != "keep-this-host" {
		t.Fatalf("got %q, want keep-this-host", got)
	}
}

func TestWaitHostIDFromStateDoesNotMint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing-state.json")
	if id := waitHostIDFromState(path, 300*time.Millisecond); id != "" {
		t.Fatalf("must not mint, got %q", id)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("waitHostIDFromState must not create the state file")
	}
}
