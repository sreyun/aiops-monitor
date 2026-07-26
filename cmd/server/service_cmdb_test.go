package main

import (
	"testing"
	"time"
)

func TestComputeServiceImpactOpenIncidents(t *testing.T) {
	im := newIncidentManager()
	_ = im.CreateManual("cpu.high", "critical", "h1", "web-01", "ops")
	cm := newChangeManager()
	ch, err := cm.Upsert(ChangeRecord{
		Title: "restart nginx", HostIDs: []string{"h1"}, StartedAt: time.Now().Unix(), Status: "completed",
	}, "ops")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{incidents: im, changes: cm}
	svc := BusinessService{ID: "svc_1", Name: "checkout", HostIDs: []string{"h1"}}
	imp := s.computeServiceImpact(svc)
	if len(imp.OpenIncidents) != 1 {
		t.Fatalf("open_incidents=%v", imp.OpenIncidents)
	}
	if len(imp.RecentChanges) < 1 || imp.RecentChanges[0].ID != ch.ID {
		t.Fatalf("recent_changes=%v", imp.RecentChanges)
	}
}

func TestChangeImpactMatchesService(t *testing.T) {
	svc := BusinessService{ID: "s1", Name: "billing", HostIDs: []string{"db-1"}}
	c := ChangeRecord{Title: "migrate", Services: []string{"billing"}}
	if !changeTouchesService(c, svc, map[string]bool{"db-1": true}) {
		t.Fatal("expected service name match")
	}
}
