package main

import (
	"testing"
	"time"
)

func TestResolveOnCallUser_WeeklyRotation(t *testing.T) {
	sch := OnCallSchedule{
		ID:       "s1",
		Name:     "primary",
		Timezone: "UTC",
		Layers: []OnCallLayer{{
			Name: "L1", Rotation: "weekly", HandoffAt: "00:00",
			Members: []string{"alice", "bob"},
			StartAt: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC).Unix(), // Monday
		}},
	}
	// Same week as start → alice
	u := resolveOnCallUser(sch, time.Date(2026, 1, 6, 12, 0, 0, 0, time.UTC))
	if u != "alice" {
		t.Fatalf("week0 want alice got %q", u)
	}
	// Next week → bob
	u = resolveOnCallUser(sch, time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC))
	if u != "bob" {
		t.Fatalf("week1 want bob got %q", u)
	}
}

func TestOnCallPageAckCancelsEscalation(t *testing.T) {
	m := newOnCallManager()
	inc := Incident{ID: 42, Title: "t", Severity: "warning"}
	pol := EscalationPolicy{
		ID: "p1", Name: "default", Enabled: true,
		Steps: []EscalationStep{
			{AfterSec: 0, Target: EscalationTarget{Users: []string{"alice"}}},
			{AfterSec: 60, Target: EscalationTarget{Users: []string{"bob"}}},
		},
	}
	page := m.Start(inc, pol, "", []string{"alice"})
	if page.Status != "pending" {
		t.Fatalf("status=%s", page.Status)
	}
	m.AckByIncident(42, "alice")
	var got OnCallPage
	found := false
	for _, p := range m.List(false) {
		if p.ID == page.ID {
			got, found = p, true
			break
		}
	}
	if !found || got.Status != "acked" {
		t.Fatalf("expected acked, got %+v found=%v", got, found)
	}
	if len(m.DuePages(time.Now().Unix()+3600)) != 0 {
		t.Fatal("acked page should not be due")
	}
}
