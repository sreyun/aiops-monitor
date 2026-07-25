package main

import (
	"encoding/json"
	"testing"
)

func TestIngestPlaybookHostInspect(t *testing.T) {
	s := &Server{inspect: newHostInspectManager()}
	host := &Host{ID: "h1", Hostname: "web-01", OS: "linux", IP: "10.0.0.1"}
	out := `{"host":{"os_family":"linux"},"result":{"warnings":1,"critical":0}}`
	s.ingestPlaybookHostInspect("nightly", 42, host, "alice", out, 1200)
	s.inspect.finishPlaybookBatch(playbookInspectBatchID(42))

	b, ok := s.inspect.get(playbookInspectBatchID(42))
	if !ok {
		t.Fatal("batch missing")
	}
	if b.Source != "playbook: nightly" {
		t.Fatalf("source=%q", b.Source)
	}
	if b.Status != "done" {
		t.Fatalf("status=%q want done", b.Status)
	}
	if len(b.Items) != 1 {
		t.Fatalf("items=%d", len(b.Items))
	}
	if b.Items[0].Status != "warn" || b.Items[0].Warnings != 1 {
		t.Fatalf("item=%+v", b.Items[0])
	}
	if b.WarnCount != 1 || b.DoneCount != 1 {
		t.Fatalf("counts ok=%d warn=%d done=%d", b.OKCount, b.WarnCount, b.DoneCount)
	}
	var rep map[string]any
	if err := json.Unmarshal(b.Items[0].Report, &rep); err != nil {
		t.Fatal(err)
	}
}

func TestParseHostInspectOutputInvalidJSON(t *testing.T) {
	host := &Host{ID: "h2", Hostname: "x"}
	item := parseHostInspectOutput(host, "not json", 100)
	if item.Status != "error" {
		t.Fatalf("status=%q want error", item.Status)
	}
}
