package main

import (
	"net/http/httptest"
	"testing"

	"aiops-monitor/shared"
)

// TestHyperVEndpointAuth locks in the auth posture: the agent ingest endpoint
// MUST be public (fingerprint-gated in the handler, no session) — otherwise
// authMiddleware 401s every agent report and no VM data is ever stored — while
// the query/delete endpoints MUST stay session-gated.
func TestHyperVEndpointAuth(t *testing.T) {
	for _, p := range []string{"/api/v1/agent/hyperv", "/api/v1/agent/hardware", "/api/v1/agent/netflow"} {
		if !isPublicPath(httptest.NewRequest("POST", p, nil)) {
			t.Errorf("%s must be public (agent ingest is fingerprint-gated); otherwise agents get 401", p)
		}
	}
	for _, p := range []string{"/api/v1/hyperv/list", "/api/v1/hyperv/events", "/api/v1/hyperv/h1"} {
		if isPublicPath(httptest.NewRequest("GET", p, nil)) {
			t.Errorf("%s must NOT be public — it exposes VM inventory to anonymous callers", p)
		}
	}
}

func TestHypervKey(t *testing.T) {
	if got := hypervKey(shared.HyperVGuest{ID: "guid-1", Name: "web"}); got != "guid-1" {
		t.Errorf("with ID: got %q, want guid-1", got)
	}
	if got := hypervKey(shared.HyperVGuest{Name: "web"}); got != "name:web" {
		t.Errorf("without ID: got %q, want name:web", got)
	}
}

func TestHypervAlertScope(t *testing.T) {
	withID := shared.HyperVGuest{ID: "guid-1", Name: "web"}
	if got := hypervAlertScope(withID); got != "guid-1" {
		t.Errorf("with ID: got %q, want guid-1", got)
	}
	noID := shared.HyperVGuest{Name: "web"}
	if got := hypervAlertScope(noID); got != "web" {
		t.Errorf("no ID: got %q, want web", got)
	}
}

func TestNormalizeHyperVGuests(t *testing.T) {
	in := []shared.HyperVGuest{
		{ID: "g1", Name: "old-name", State: "Off"},
		{ID: "g1", Name: "new-name", State: "Running"}, // same GUID → keep last
		{Name: "orphan"},
		{Name: ""}, // drop
	}
	out := normalizeHyperVGuests(in)
	if len(out) != 2 {
		t.Fatalf("got %d guests, want 2: %+v", len(out), out)
	}
	if out[0].ID != "g1" || out[0].Name != "new-name" || out[0].State != "Running" {
		t.Errorf("GUID dedupe kept wrong entry: %+v", out[0])
	}
	if out[1].Name != "orphan" {
		t.Errorf("orphan missing: %+v", out)
	}
}

// TestNormalizeHyperVGuestsDropsNameOrphan covers the 资源→虚拟机 duplicate bug: a VM
// reported once with its GUID and once name-only (legacy snapshot / rename ghost /
// same name as a physical host) must collapse to the single GUID entry.
func TestNormalizeHyperVGuestsDropsNameOrphan(t *testing.T) {
	in := []shared.HyperVGuest{
		{ID: "guid-a", Name: "SRV-01", State: "Running"},
		{Name: "SRV-01", State: "Off"}, // name-only twin → drop
		{Name: "solo"},                 // no GUID twin → keep
	}
	out := normalizeHyperVGuests(in)
	if len(out) != 2 {
		t.Fatalf("got %d guests, want 2: %+v", len(out), out)
	}
	if out[0].ID != "guid-a" || out[0].State != "Running" {
		t.Errorf("GUID entry should survive: %+v", out[0])
	}
	if out[1].Name != "solo" {
		t.Errorf("name-only without GUID twin should survive: %+v", out)
	}
}

// TestDedupHyperVRowGuests verifies legacy PG rows self-heal on read.
func TestDedupHyperVRowGuests(t *testing.T) {
	rows := []map[string]any{{
		"host_id": "h1",
		"guests": []any{
			map[string]any{"id": "guid-a", "name": "SRV-01"},
			map[string]any{"name": "SRV-01"},                 // name-only orphan → drop
			map[string]any{"id": "guid-a", "name": "SRV-01"}, // exact dup → drop
			map[string]any{"name": "solo"},
			map[string]any{"name": ""}, // empty → drop
		},
		"guest_count": 5,
	}}
	dedupHyperVRowGuests(rows)
	guests, _ := rows[0]["guests"].([]any)
	if len(guests) != 2 {
		t.Fatalf("got %d guests after dedup, want 2: %+v", len(guests), guests)
	}
	if rows[0]["guest_count"].(int) != 2 {
		t.Errorf("guest_count not updated: %v", rows[0]["guest_count"])
	}
}

// TestEvaluateHyperVRenameStableScope verifies that a rename with the same GUID
// keeps the same alert Scope (no twin alerts under old+new names).
func TestEvaluateHyperVRenameStableScope(t *testing.T) {
	hs := newHypervStore()
	hs.put("h1", "host1", "", []shared.HyperVGuest{
		{ID: "guid-a", Name: "web-old", State: "Running", CPUUsage: 96},
	}, "", 0, 0)
	a1 := EvaluateHyperV(hs)
	if len(a1) != 1 || a1[0].Scope != "guid-a/cpu" {
		t.Fatalf("before rename: %+v, want scope guid-a/cpu", a1)
	}

	hs.put("h1", "host1", "", []shared.HyperVGuest{
		{ID: "guid-a", Name: "web-new", State: "Running", CPUUsage: 96},
	}, "", 0, 0)
	a2 := EvaluateHyperV(hs)
	if len(a2) != 1 || a2[0].Scope != "guid-a/cpu" {
		t.Fatalf("after rename: %+v, want same scope guid-a/cpu", a2)
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
	errStore.put("h1", "host1", "10.0.0.1", []shared.HyperVGuest{{Name: "x", State: "Off"}}, "get-vm failed", 0, 0)
	ea := EvaluateHyperV(errStore)
	if len(ea) != 1 || ea[0].Level != "warning" || ea[0].Scope != "collect" || ea[0].Type != "hyperv" {
		t.Fatalf("collect error: %+v, want 1 warning/collect/hyperv", ea)
	}

	// mixed guests. vmOff must first be seen Running so its stop is an alarming
	// transition (Off since first seen would be a silent, intentionally-off VM).
	hs := newHypervStore()
	hs.put("h1", "host1", "10.0.0.1", []shared.HyperVGuest{{Name: "vmOff", State: "Running"}}, "", 0, 0)
	hs.put("h1", "host1", "10.0.0.1", []shared.HyperVGuest{
		{Name: "vmCrit", State: "Running", Health: "Critical"}, // critical, scope=name, no further
		{Name: "vmOff", State: "Off"},                          // warning /power (Running→Off)
		{Name: "vmCPU", State: "Running", CPUUsage: 96},        // critical /cpu
		{Name: "vmMem", State: "Running", CPUUsage: 10, MemAssignedMB: 1000, MemDemandMB: 980, DynamicMemEnabled: true}, // critical /mem (98%)
		{Name: "vmStatic", State: "Running", CPUUsage: 10, MemAssignedMB: 1000, MemDemandMB: 990},                       // static mem → NO mem alert
		{Name: "vmOK", State: "Running", CPUUsage: 10, MemAssignedMB: 1000, MemDemandMB: 100, DynamicMemEnabled: true},  // healthy → no alert
	}, "", 0, 0)
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
	// Static-memory VM must not trigger a mem-pressure alert even at 99% demand.
	if _, ok := byScope["vmStatic/mem"]; ok {
		t.Errorf("static-memory VM must not trigger mem alert: %v", byScope)
	}
	// Healthy / silent VMs must produce nothing.
	for scope := range byScope {
		for _, silent := range []string{"vmOK", "vmOK/cpu", "vmOK/mem", "vmStatic", "vmStatic/cpu"} {
			if scope == silent {
				t.Errorf("VM expected silent produced alert %q", scope)
			}
		}
	}
}

// TestEvaluateHyperVOffTransition verifies power alerts fire only on a
// Running→non-running transition (not for VMs off since first seen) and clear on
// recovery — the anti-noise rule for intentionally-stopped VMs.
func TestEvaluateHyperVOffTransition(t *testing.T) {
	hs := newHypervStore()

	// Off since first seen (e.g. a template / cold spare) → no alarm.
	hs.put("h1", "host1", "", []shared.HyperVGuest{{Name: "tmpl", State: "Off"}}, "", 0, 0)
	if a := EvaluateHyperV(hs); len(a) != 0 {
		t.Fatalf("always-off VM must not alarm: %+v", a)
	}

	// Runs, then stops → one power warning.
	hs.put("h1", "host1", "", []shared.HyperVGuest{{Name: "tmpl", State: "Running"}}, "", 0, 0)
	hs.put("h1", "host1", "", []shared.HyperVGuest{{Name: "tmpl", State: "Off"}}, "", 0, 0)
	a := EvaluateHyperV(hs)
	if len(a) != 1 || a[0].Scope != "tmpl/power" || a[0].Level != "warning" {
		t.Fatalf("Running→Off must alarm once at tmpl/power: %+v", a)
	}

	// Stays off across another report → still one alarm (sticky), not cleared.
	hs.put("h1", "host1", "", []shared.HyperVGuest{{Name: "tmpl", State: "Off"}}, "", 0, 0)
	if a := EvaluateHyperV(hs); len(a) != 1 {
		t.Fatalf("sticky while still off: %+v", a)
	}

	// Recovers to Running → alarm clears.
	hs.put("h1", "host1", "", []shared.HyperVGuest{{Name: "tmpl", State: "Running"}}, "", 0, 0)
	if a := EvaluateHyperV(hs); len(a) != 0 {
		t.Fatalf("recovered VM must clear: %+v", a)
	}
}
