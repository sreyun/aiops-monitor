package main

import (
	"testing"
	"time"
)

func TestRemotePreflightGateRequired(t *testing.T) {
	now := time.Now().Unix()
	cs := &ConfigStore{}
	cs.cfg.ChangeWindows = []ChangeWindow{{
		ID: "f1", Name: "night", Start: now - 60, End: now + 3600, Freeze: true,
	}}
	s := &Server{cfg: cs, store: NewStore(), changes: newChangeManager(), incidents: newIncidentManager()}
	pf := s.computeRemotePreflight(nil, "h1")
	if !pf.FreezeActive {
		t.Fatal("expected freeze_active")
	}
	if !pf.GateRequired || pf.GateAllowed {
		t.Fatalf("expected gate_required, allowed=%v reason=%s", pf.GateAllowed, pf.GateReason)
	}
}

func TestRemotePreflightBreakGlassQuery(t *testing.T) {
	now := time.Now().Unix()
	cs := &ConfigStore{}
	cs.cfg.ChangeWindows = []ChangeWindow{{
		ID: "f1", Name: "night", Start: now - 60, End: now + 3600, Freeze: true,
	}}
	// Seed an admin user so actorIsAdmin works via cookie-less request with forced role is hard;
	// without request, breakGlass stays false — just ensure no panic and gate still required.
	s := &Server{cfg: cs, store: NewStore(), changes: newChangeManager(), incidents: newIncidentManager()}
	pf := s.computeRemotePreflight(nil, "h1")
	if pf.GateAllowed {
		t.Fatal("nil request must not break-glass")
	}
}
