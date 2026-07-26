package main

import "testing"

func TestCitationIsStrong(t *testing.T) {
	if citationIsStrong(RAGCitation{Kind: "inspect", Source: "heartbeat", Summary: "ok"}) {
		t.Fatal("heartbeat-only inspect should not be strong")
	}
	if !citationIsStrong(RAGCitation{Kind: "metric", Source: "live", Summary: "cpu=90"}) {
		t.Fatal("metric citation should be strong")
	}
	if !citationIsStrong(RAGCitation{Kind: "note", Source: "metrics", Summary: "CPU 使用率"}) {
		t.Fatal("source/summary mentioning metrics/cpu should be strong")
	}
}

func TestDiagnosisEvidenceOK(t *testing.T) {
	if diagnosisEvidenceOK(nil) {
		t.Fatal("empty cites should fail")
	}
	if diagnosisEvidenceOK([]RAGCitation{{Kind: "inspect", Source: "hb"}}) {
		t.Fatal("single weak cite should fail")
	}
	if !diagnosisEvidenceOK([]RAGCitation{{Kind: "alert", Source: "cpu", Summary: "high"}}) {
		t.Fatal("one strong cite should pass")
	}
	weak := []RAGCitation{
		{Kind: "inspect", Source: "a"},
		{Kind: "inspect", Source: "b"},
		{Kind: "inspect", Source: "c"},
	}
	if !diagnosisEvidenceOK(weak) {
		t.Fatal("≥3 mixed cites should pass even if weak")
	}
}

func TestFilterMemoriesByScope(t *testing.T) {
	hits := []memoryHit{
		{ID: 1, Content: "global", ServiceID: "", Category: ""},
		{ID: 2, Content: "svc-a", ServiceID: "svc-a", Category: "web"},
		{ID: 3, Content: "svc-b", ServiceID: "svc-b", Category: "db"},
	}
	got := filterMemoriesByScope(hits, "svc-a", "web", 10)
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("got=%+v", got)
	}
	got = filterMemoriesByScope(hits, "", "", 1)
	if len(got) != 1 {
		t.Fatalf("limit without scope: %+v", got)
	}
}
