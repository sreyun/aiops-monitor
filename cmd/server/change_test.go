package main

import (
	"testing"
	"time"
)

func TestChangeRecordRelatedToHosts(t *testing.T) {
	m := newChangeManager()
	now := time.Now().Unix()
	_, err := m.Upsert(ChangeRecord{
		Title: "deploy api", Kind: "deploy", HostIDs: []string{"h1", "h2"}, StartedAt: now,
	}, "ops")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = m.Upsert(ChangeRecord{
		Title: "other host", Kind: "config", HostIDs: []string{"h9"}, StartedAt: now,
	}, "ops")
	rels := m.RelatedToHosts([]string{"h1"}, now-60)
	if len(rels) != 1 || rels[0].Title != "deploy api" {
		t.Fatalf("related=%+v", rels)
	}
}

func TestActiveFreezeWindow(t *testing.T) {
	cs := &ConfigStore{}
	now := time.Now().Unix()
	cs.cfg.ChangeWindows = []ChangeWindow{{
		ID: "w1", Name: "freeze", Start: now - 60, End: now + 3600,
		HostIDs: []string{"h1"}, Freeze: true,
	}}
	w, ok := cs.activeFreezeWindow("h1", "", now)
	if !ok || !w.Freeze {
		t.Fatalf("expected freeze for h1, ok=%v w=%+v", ok, w)
	}
	if _, ok := cs.activeFreezeWindow("h2", "", now); ok {
		t.Fatal("h2 should not match host-scoped freeze")
	}
}

func TestChangeWindowRecurDaily(t *testing.T) {
	// Build a local time inside the daily window 22:00–06:00.
	base := time.Date(2026, 7, 25, 23, 30, 0, 0, time.Local)
	w := ChangeWindow{Recur: "daily", RecurStartHM: "22:00", RecurEndHM: "06:00", Freeze: true}
	if !changeWindowActiveAt(w, base.Unix()) {
		t.Fatal("23:30 should be inside overnight daily freeze")
	}
	day := time.Date(2026, 7, 25, 12, 0, 0, 0, time.Local)
	if changeWindowActiveAt(w, day.Unix()) {
		t.Fatal("12:00 should be outside overnight daily freeze")
	}
	abs := ChangeWindow{Start: base.Unix() - 10, End: base.Unix() + 10, Freeze: true}
	if !changeWindowActiveAt(abs, base.Unix()) {
		t.Fatal("absolute window should be active")
	}
}

func TestSkillMatchesScope(t *testing.T) {
	global := Skill{}
	if !skillMatchesScope(global, "svc1", "db") {
		t.Fatal("global skill should match any scope")
	}
	scoped := Skill{ServiceIDs: "svc1,svc2", Categories: "db"}
	if !skillMatchesScope(scoped, "svc1", "db") {
		t.Fatal("exact scope should match")
	}
	if skillMatchesScope(scoped, "svc9", "db") {
		t.Fatal("wrong service should not match")
	}
	if skillMatchesScope(scoped, "svc1", "web") {
		t.Fatal("wrong category should not match")
	}
}
