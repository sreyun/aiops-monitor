package main

import (
	"testing"
	"time"
)

func TestPercentileAndEffectMTTR(t *testing.T) {
	sorted := []int64{10, 20, 30, 40, 100}
	if percentileSorted(sorted, 0.5) != 30 {
		t.Fatalf("p50=%d", percentileSorted(sorted, 0.5))
	}
	s := &Server{incidents: newIncidentManager()}
	a := s.incidents.CreateManual("a", "critical", "h1", "h1", "ops")
	s.incidents.Resolve(a.ID, "ops")
	now := time.Now().Unix()
	list := s.incidents.Export()
	for i := range list {
		if list[i].ID == a.ID {
			list[i].CreatedAt = now - 300
			list[i].ResolvedAt = now
			list[i].Status = "resolved"
		}
	}
	s.incidents.Import(list)
	rep := s.computeSREEffect(30)
	if rep.ResolvedCount < 1 || rep.MTTRP50Sec != 300 {
		t.Fatalf("effect=%+v", rep)
	}
}

func TestServiceImpactTouches(t *testing.T) {
	svc := BusinessService{ID: "s1", Name: "checkout", HostIDs: []string{"h1"}}
	c := ChangeRecord{Title: "deploy", HostIDs: []string{"h1"}, StartedAt: 1}
	if !changeTouchesService(c, svc, map[string]bool{"h1": true}) {
		t.Fatal("expected touch")
	}
}

func TestChangeFailureRateAndNoise(t *testing.T) {
	im := newIncidentManager()
	cm := newChangeManager()
	now := time.Now().Unix()
	// Resolved then reopen same key → noise
	r1 := im.CreateManual("noise.key", "warning", "h1", "h1", "ops")
	im.Resolve(r1.ID, "ops")
	_ = im.CreateManual("noise.key", "warning", "h1", "h1", "ops")
	list := im.Export()
	for i := range list {
		list[i].CreatedAt = now - 100
		if list[i].Status == "resolved" {
			list[i].ResolvedAt = now - 50
			list[i].Key = "noise.key"
		} else {
			list[i].Key = "noise.key"
		}
	}
	im.Import(list)

	okCh, err := cm.Upsert(ChangeRecord{
		Title: "good", Status: ChangeCompleted, StartedAt: now - 10, EndedAt: now, CreatedAt: now - 20,
	}, "ops")
	if err != nil {
		t.Fatal(err)
	}
	failCh, err := cm.Upsert(ChangeRecord{
		Title: "bad", Status: ChangeRolledBack, StartedAt: now - 10, EndedAt: now, CreatedAt: now - 20,
	}, "ops")
	if err != nil {
		t.Fatal(err)
	}
	_ = okCh
	_ = failCh

	s := &Server{incidents: im, changes: cm}
	rep := s.computeSREEffect(14)
	if rep.ChangeCount < 2 || rep.ChangeFailedCount < 1 || rep.ChangeFailureRate <= 0 {
		t.Fatalf("cfr=%+v", rep)
	}
	if rep.AlertReopenKeys < 1 {
		t.Fatalf("noise reopen missing: %+v", rep)
	}
}

func TestChangeIsFailedLinkedIncident(t *testing.T) {
	now := time.Now().Unix()
	inc := Incident{ID: 9, CreatedAt: now + 60}
	c := ChangeRecord{
		Status: ChangeCompleted, StartedAt: now, LinkedIncidentIDs: []int64{9},
	}
	if !changeIsFailed(c, []Incident{inc}) {
		t.Fatal("expected linked incident within 24h to count as failure")
	}
}

func TestFallbackModelList(t *testing.T) {
	cfg := AIConfig{Model: "a", FallbackModels: "b, a, c"}
	got := fallbackModelList(cfg)
	if len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Fatalf("got=%v", got)
	}
}
