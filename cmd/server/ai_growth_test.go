package main

import "testing"

func TestTrackOpsPatternProposesAfterThree(t *testing.T) {
	s := &Server{}
	growthHub = newAIGrowthHub()
	for i := 0; i < 2; i++ {
		ok, _, _, _ := s.trackOpsPattern("u1", "oom-restart", "查日志→定位OOM→重启")
		if ok {
			t.Fatalf("should not propose on hit %d", i+1)
		}
	}
	ok, name, trigger, steps := s.trackOpsPattern("u1", "oom-restart", "查日志→定位OOM→重启")
	if !ok {
		t.Fatal("expected propose on 3rd hit")
	}
	if name == "" || trigger == "" || steps == "" {
		t.Fatalf("empty draft: %q %q %q", name, trigger, steps)
	}
}

func TestRecordForecastOutcomeThreshold(t *testing.T) {
	// Without PG/memory channel, rememberAI no-ops — just ensure no panic.
	s := &Server{}
	s.recordForecastOutcome("cpu", 50, 80, 15)
	s.recordForecastOutcome("cpu", 50, 52, 15) // within threshold
}

func TestFormatHorizon(t *testing.T) {
	if formatHorizon(120) != "2m" {
		t.Fatalf("got %s", formatHorizon(120))
	}
	if formatHorizon(7200) != "2h" {
		t.Fatalf("got %s", formatHorizon(7200))
	}
	if formatHorizon(3*86400) != "3d" {
		t.Fatalf("got %s", formatHorizon(3*86400))
	}
}
