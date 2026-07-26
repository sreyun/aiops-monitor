package main

import "testing"

func TestLatestDiagnosisGate(t *testing.T) {
	inc := Incident{ID: 1, Title: "x", Timeline: []IncidentEvent{
		{Kind: "ai_diagnosis", Text: "结论：CPU 高。置信度：低", Citations: nil},
	}}
	g := latestDiagnosisGate(inc)
	if g.OK {
		t.Fatal("expected gate fail without citations")
	}
	inc.Timeline[0].Citations = []RAGCitation{{Source: "metrics", Summary: "cpu=99"}}
	inc.Timeline[0].Text = "结论：CPU 高。置信度：高"
	g = latestDiagnosisGate(inc)
	if !g.OK || g.Citations != 1 {
		t.Fatalf("gate=%+v", g)
	}
	// Weak-only evidence must fail the strong-evidence gate even if citations exist.
	inc.Timeline[0].Citations = []RAGCitation{{Kind: "inspect", Source: "heartbeat", Summary: "ok"}}
	if diagnosisEvidenceOK(inc.Timeline[0].Citations) {
		t.Fatal("heartbeat-only should fail diagnosisEvidenceOK")
	}
}

func TestIncidentLoopSetAndGate(t *testing.T) {
	im := newIncidentManager()
	inc := im.CreateManual("cpu", "critical", "h1", "web-01", "alice")
	im.AddEventWithCitations(inc.ID, "ai_diagnosis", "AI", "诊断正文 置信度：高",
		[]RAGCitation{{Source: "metrics", Summary: "cpu"}})
	loop := IncidentLoopState{Stage: "dry_run_ok", DryRunOK: true}
	got, ok := im.SetLoop(inc.ID, loop)
	if !ok || got.Loop == nil || got.Loop.Stage != "dry_run_ok" {
		t.Fatalf("loop=%+v", got.Loop)
	}
	s := &Server{incidents: im}
	gate, err := s.diagnosisGateAllowsPropose(got, false)
	if err != nil || !gate.OK {
		t.Fatalf("gate err=%v %+v", err, gate)
	}
	_, err = s.diagnosisGateAllowsPropose(Incident{ID: 99}, false)
	if err == nil {
		t.Fatal("expected gate error without diagnosis")
	}
}
