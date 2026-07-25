package main

import "testing"

func TestImportBatchesMarksStuckRunning(t *testing.T) {
	m := newHostInspectManager()
	m.importBatches([]*hostInspectBatch{{
		ID: "insp-1", Status: "running", HostCount: 1,
		Items: []hostInspectItem{{HostID: "h1", Hostname: "a", Status: "running"}},
	}})
	got, ok := m.get("insp-1")
	if !ok {
		t.Fatal("batch missing")
	}
	if got.Status != "done" {
		t.Fatalf("status=%q want done", got.Status)
	}
	if got.Items[0].Status != "error" {
		t.Fatalf("item status=%q want error", got.Items[0].Status)
	}
	if got.FinishedAt == 0 {
		t.Fatal("finished_at should be set")
	}
}
