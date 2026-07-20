package main

import (
	"testing"

	"aiops-monitor/shared"
)

// TestSNMPStorePutMergeByIP locks in the rename-safe merge: successive polls for
// different devices on the same agent must accumulate, and a same-IP rename must
// replace in place (not grow a duplicate).
func TestSNMPStorePutMergeByIP(t *testing.T) {
	ss := newSNMPStore()
	ss.put("h1", "agent1", "10.0.0.9", []shared.SNMPSnapshot{
		{TargetName: "sw-core", TargetIP: "10.0.0.1", Reachable: true},
	})
	ss.put("h1", "agent1", "10.0.0.9", []shared.SNMPSnapshot{
		{TargetName: "sw-access", TargetIP: "10.0.0.2", Reachable: true},
	})
	snaps := ss.snapsOf("h1")
	if len(snaps) != 2 {
		t.Fatalf("sibling devices: got %d, want 2", len(snaps))
	}

	// Rename sw-core → core-sw (same IP): still one row for that IP, new name.
	ss.put("h1", "agent1", "10.0.0.9", []shared.SNMPSnapshot{
		{TargetName: "core-sw", TargetIP: "10.0.0.1", Reachable: true},
	})
	snaps = ss.snapsOf("h1")
	if len(snaps) != 2 {
		t.Fatalf("after rename: got %d devices, want 2 (no duplicate)", len(snaps))
	}
	var foundCore, foundOld bool
	for _, s := range snaps {
		if s.TargetIP == "10.0.0.1" {
			if s.TargetName != "core-sw" {
				t.Errorf("renamed device name = %q, want core-sw", s.TargetName)
			}
			foundCore = true
		}
		if s.TargetName == "sw-core" {
			foundOld = true
		}
	}
	if !foundCore {
		t.Fatal("missing renamed device 10.0.0.1 / core-sw")
	}
	if foundOld {
		t.Fatal("old name sw-core must not linger after IP-matched rename")
	}
}

func TestSNMPStoreMigrateOperSeen(t *testing.T) {
	ss := newSNMPStore()
	ss.operDownShouldAlert("h1|sw-old|1", true)
	ss.migrateDeviceKey("h1", "sw-old", "sw-new")
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if !ss.operSeen["h1|sw-new|1"] {
		t.Fatal("operSeen key must move to new device name")
	}
	if ss.operSeen["h1|sw-old|1"] {
		t.Fatal("old operSeen key must be removed")
	}
}

func TestSNMPStoreRemove(t *testing.T) {
	ss := newSNMPStore()
	ss.put("h1", "a", "", []shared.SNMPSnapshot{{TargetName: "a", TargetIP: "1.1.1.1"}})
	ss.put("h1", "a", "", []shared.SNMPSnapshot{{TargetName: "b", TargetIP: "2.2.2.2"}})
	ss.operDownShouldAlert("h1|a|3", true)
	ss.remove("h1", "a")
	snaps := ss.snapsOf("h1")
	if len(snaps) != 1 || snaps[0].TargetName != "b" {
		t.Fatalf("after remove: %+v", snaps)
	}
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.operSeen["h1|a|3"] {
		t.Fatal("operSeen for removed device must be cleared")
	}
}

func TestSNMPDeviceKey(t *testing.T) {
	if got := snmpDeviceKey(shared.SNMPSnapshot{TargetName: "n", TargetIP: "10.0.0.1"}); got != "ip:10.0.0.1" {
		t.Errorf("got %q", got)
	}
	if got := snmpDeviceKey(shared.SNMPSnapshot{TargetName: "n"}); got != "name:n" {
		t.Errorf("got %q", got)
	}
}
