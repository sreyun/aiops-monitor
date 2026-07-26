package main

import (
	"testing"
	"time"
)

func TestChangeSoDBlocksSelfApprove(t *testing.T) {
	rec := ChangeRecord{Author: "alice", Status: ChangePendingApproval}
	if err := changeSoDAllows(rec, "approve", "alice", false); err == nil {
		t.Fatal("expected SoD error")
	}
	if err := changeSoDAllows(rec, "approve", "bob", false); err != nil {
		t.Fatal(err)
	}
	if err := changeSoDAllows(rec, "approve", "alice", true); err != nil {
		t.Fatal("break-glass should allow")
	}
}

func TestChangeTransitionSoD(t *testing.T) {
	m := newChangeManager()
	rec, err := m.Upsert(ChangeRecord{Title: "x", Author: "alice", Status: ChangePendingApproval}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.TransitionSoD(rec.ID, "approve", "alice", false, false)
	if err == nil {
		t.Fatal("self-approve should fail")
	}
	out, err := m.TransitionSoD(rec.ID, "approve", "bob", false, false)
	if err != nil || out.Approver != "bob" {
		t.Fatalf("err=%v out=%+v", err, out)
	}
}

func TestRemoteGateFreezeRequiresChange(t *testing.T) {
	cs := &ConfigStore{}
	now := time.Now().Unix()
	cs.cfg.ChangeWindows = []ChangeWindow{{
		ID: "w1", Name: "f", Start: now - 60, End: now + 3600,
		HostIDs: []string{"h1"}, Freeze: true,
	}}
	s := &Server{cfg: cs, changes: newChangeManager(), incidents: newIncidentManager()}
	ok, reason := s.remoteGateCheck("h1", "ops", false, false)
	if ok {
		t.Fatalf("expected deny during freeze, reason=%s", reason)
	}
	_, _ = s.changes.Upsert(ChangeRecord{
		Title: "fix", HostIDs: []string{"h1"}, Status: ChangeApproved, StartedAt: now,
	}, "ops")
	ok, _ = s.remoteGateCheck("h1", "ops", false, false)
	if !ok {
		t.Fatal("approved change should allow remote")
	}
}

func TestLoopForceRequiresAdminDefault(t *testing.T) {
	cs := &ConfigStore{}
	if !cs.loopForceRequiresAdmin() {
		t.Fatal("default should require admin")
	}
	cs.cfg.LoopForceAllowNonAdmin = true
	if cs.loopForceRequiresAdmin() {
		t.Fatal("explicit allow should disable admin requirement")
	}
}
