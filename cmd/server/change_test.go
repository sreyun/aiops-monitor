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
