package main

import (
	"testing"
	"time"
)

func TestFireScheduledPlaybookRequiresApprovalForChangeModules(t *testing.T) {
	s, _ := newTestServer(t)
	pb, err := s.playbooks.Upsert(Playbook{
		Name: "restart-nginx",
		Steps: []PlaybookStep{{
			Name: "restart", Module: "service", Target: "all", TimeoutSec: 30,
			Args: map[string]string{"name": "nginx", "state": "restarted"},
		}},
		Schedule: &PlaybookSchedule{Enabled: true, Kind: "interval", IntervalMin: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	h := s.store.RegisterHost("h1", "n1", "fp-h1")
	h.OS = "linux"
	h.LastSeen = time.Now().Unix()

	s.fireScheduledPlaybook(pb)

	list := s.playbooks.ExecutionHistory()
	if len(list) == 0 {
		t.Fatal("expected pending execution")
	}
	if list[0].Status != "pending_approval" {
		t.Fatalf("status=%q want pending_approval", list[0].Status)
	}
	if list[0].Trigger != "schedule" {
		t.Fatalf("trigger=%q", list[0].Trigger)
	}
}

func TestFireScheduledPlaybookReadonlyRunsImmediately(t *testing.T) {
	s, _ := newTestServer(t)
	pb, err := s.playbooks.Upsert(Playbook{
		Name: "facts",
		Steps: []PlaybookStep{{
			Name: "facts", Module: "gather_facts", Target: "all", TimeoutSec: 30,
		}},
		Schedule: &PlaybookSchedule{Enabled: true, Kind: "interval", IntervalMin: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	h := s.store.RegisterHost("h1", "n1", "fp-h1")
	h.OS = "linux"
	h.LastSeen = time.Now().Unix()

	s.fireScheduledPlaybook(pb)

	list := s.playbooks.ExecutionHistory()
	if len(list) == 0 {
		t.Fatal("expected execution")
	}
	if list[0].Status == "pending_approval" {
		t.Fatal("readonly schedule must not require approval")
	}
}
