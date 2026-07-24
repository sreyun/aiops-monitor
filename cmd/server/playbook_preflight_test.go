package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func newPreflightServer(t *testing.T) (*Server, *playbookManager) {
	t.Helper()
	cs, err := NewConfigStore(filepath.Join(t.TempDir(), "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	h := store.RegisterHost("h1", "node-1", "fp1")
	h.OS = "linux"
	h.Category = "prod"
	h.LastSeen = time.Now().Unix()
	pm := newPlaybookManager(cs)
	return &Server{store: store, cfg: cs, playbooks: pm}, pm
}

func TestPlaybookPreflightRiskAndRollbackCoverage(t *testing.T) {
	s, pm := newPreflightServer(t)
	pb, err := pm.Upsert(Playbook{
		Name: "restart",
		Strategy: PlaybookStrategy{
			MaxParallel: 2, AutoRollback: true,
		},
		Steps: []PlaybookStep{{
			Name: "restart service", Module: "service",
			Args:   map[string]string{"name": "nginx", "state": "restarted"},
			Target: "all", TimeoutSec: 30,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pf := s.buildPlaybookPreflight(pb)
	if !pf.Valid || pf.RiskLevel != "high" || !pf.RequiresApproval || pf.OnlineTargets != 1 {
		t.Fatalf("unexpected preflight: %+v", pf)
	}
	if len(pf.Warnings) == 0 {
		t.Fatal("auto-rollback change without rollback must warn")
	}
}

func TestHighRiskExecutionRequiresServerSideAcknowledgement(t *testing.T) {
	s, pm := newPreflightServer(t)
	pb, err := pm.Upsert(Playbook{
		Name: "restart",
		Steps: []PlaybookStep{{
			Name: "restart service", Module: "service",
			Args:   map[string]string{"name": "nginx", "state": "restarted"},
			Target: "all", TimeoutSec: 30,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/playbooks/"+pb.ID+"/execute", nil)
	req.SetPathValue("id", pb.ID)
	rw := httptest.NewRecorder()
	s.handleExecutePlaybook(rw, req)
	if rw.Code != http.StatusConflict {
		t.Fatalf("high-risk execution without acknowledgement = %d, want 409; body=%s", rw.Code, rw.Body.String())
	}
}
