package main

import (
	"testing"

	"aiops-monitor/shared"
)

func TestHypervKey(t *testing.T) {
	if got := hypervKey(shared.HyperVGuest{ID: "guid-1", Name: "web"}); got != "guid-1" {
		t.Errorf("with ID: got %q, want guid-1", got)
	}
	if got := hypervKey(shared.HyperVGuest{Name: "web"}); got != "name:web" {
		t.Errorf("without ID: got %q, want name:web", got)
	}
}

func TestDiffHyperVGuests(t *testing.T) {
	base := []shared.HyperVGuest{
		{ID: "a", Name: "web01", State: "Running"},
		{ID: "b", Name: "db01", State: "Running"},
	}

	// helper: index events by kind for assertions
	countKinds := func(chs []hypervChange) map[string]int {
		m := map[string]int{}
		for _, c := range chs {
			m[c.kind]++
		}
		return m
	}

	// added: base + a new VM.
	cur := append(append([]shared.HyperVGuest{}, base...), shared.HyperVGuest{ID: "c", Name: "app01", State: "Running"})
	if k := countKinds(diffHyperVGuests(base, cur)); k["vm_added"] != 1 || len(k) != 1 {
		t.Errorf("added: kinds = %v, want {vm_added:1}", k)
	}

	// removed: drop db01.
	cur = []shared.HyperVGuest{{ID: "a", Name: "web01", State: "Running"}}
	if k := countKinds(diffHyperVGuests(base, cur)); k["vm_removed"] != 1 || len(k) != 1 {
		t.Errorf("removed: kinds = %v, want {vm_removed:1}", k)
	}

	// state change Running→Off must be severity=warning (unexpected stop).
	cur = []shared.HyperVGuest{
		{ID: "a", Name: "web01", State: "Off"},
		{ID: "b", Name: "db01", State: "Running"},
	}
	chs := diffHyperVGuests(base, cur)
	if len(chs) != 1 || chs[0].kind != "state_change" || chs[0].severity != "warning" {
		t.Errorf("Running→Off: %+v, want 1 state_change/warning", chs)
	}

	// state change Off→Running is only info.
	prevOff := []shared.HyperVGuest{{ID: "a", Name: "web01", State: "Off"}}
	curOn := []shared.HyperVGuest{{ID: "a", Name: "web01", State: "Running"}}
	chs = diffHyperVGuests(prevOff, curOn)
	if len(chs) != 1 || chs[0].severity != "info" {
		t.Errorf("Off→Running: %+v, want 1 info", chs)
	}

	// rename with the SAME GUID must NOT be remove+add (GUID is identity).
	prev := []shared.HyperVGuest{{ID: "x", Name: "old-name", State: "Running"}}
	cur = []shared.HyperVGuest{{ID: "x", Name: "new-name", State: "Running"}}
	if chs := diffHyperVGuests(prev, cur); len(chs) != 0 {
		t.Errorf("rename same GUID: got %+v, want no changes", chs)
	}

	// identical inventories produce nothing.
	if chs := diffHyperVGuests(base, base); len(chs) != 0 {
		t.Errorf("no-op: got %+v", chs)
	}
}

func TestEvaluateHyperV(t *testing.T) {
	// empty store → no alerts
	if a := EvaluateHyperV(newHypervStore()); a != nil {
		t.Errorf("empty store: got %v, want nil", a)
	}

	// collection error → exactly one "collect" warning, guests not evaluated
	errStore := newHypervStore()
	errStore.put("h1", "host1", "10.0.0.1", []shared.HyperVGuest{{Name: "x", State: "Off"}}, "get-vm failed")
	ea := EvaluateHyperV(errStore)
	if len(ea) != 1 || ea[0].Level != "warning" || ea[0].Scope != "collect" || ea[0].Type != "hyperv" {
		t.Fatalf("collect error: %+v, want 1 warning/collect/hyperv", ea)
	}

	// mixed guests
	hs := newHypervStore()
	hs.put("h1", "host1", "10.0.0.1", []shared.HyperVGuest{
		{Name: "vmCrit", State: "Running", Health: "Critical"},                                 // critical, scope=name, no further
		{Name: "vmOff", State: "Off"},                                                          // warning /power
		{Name: "vmCPU", State: "Running", CPUUsage: 96},                                        // critical /cpu
		{Name: "vmMem", State: "Running", CPUUsage: 10, MemAssignedMB: 1000, MemDemandMB: 980}, // critical /mem (98%)
		{Name: "vmOK", State: "Running", CPUUsage: 10, MemAssignedMB: 1000, MemDemandMB: 100},  // healthy → no alert
	}, "")
	alerts := EvaluateHyperV(hs)

	byScope := map[string]string{}
	keys := map[string]bool{}
	for _, a := range alerts {
		if a.Type != "hyperv" || a.HostID != "h1" {
			t.Errorf("unexpected alert fields: %+v", a)
		}
		byScope[a.Scope] = a.Level
		k := alertKey(a)
		if keys[k] {
			t.Errorf("duplicate alertKey %q — sibling VMs/metrics would overwrite each other", k)
		}
		keys[k] = true
	}

	want := map[string]string{
		"vmCrit":      "critical",
		"vmOff/power": "warning",
		"vmCPU/cpu":   "critical",
		"vmMem/mem":   "critical",
	}
	for scope, lvl := range want {
		if byScope[scope] != lvl {
			t.Errorf("scope %q = %q, want %q (all: %v)", scope, byScope[scope], lvl, byScope)
		}
	}
	// vmCrit hit Health=Critical and must NOT also emit a /cpu or /mem alert.
	if _, ok := byScope["vmCrit/cpu"]; ok {
		t.Errorf("vmCrit should short-circuit after Critical health, got extra alerts: %v", byScope)
	}
	// vmOK must be silent.
	for scope := range byScope {
		if scope == "vmOK" || scope == "vmOK/cpu" || scope == "vmOK/mem" {
			t.Errorf("healthy VM produced alert %q", scope)
		}
	}
}
