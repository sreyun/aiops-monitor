package main

import (
	"math"
	"testing"
	"time"
)

func TestApplyForecastCalibrationBias(t *testing.T) {
	key := "test-calib-key"
	fcLearn.mu.Lock()
	fcLearn.Calibs[key] = &forecastCalibration{
		Key: key, BiasOffset: 10, Scale: 1, EvalCount: 3, MethodScores: map[string]float64{},
	}
	fcLearn.mu.Unlock()
	defer func() {
		fcLearn.mu.Lock()
		delete(fcLearn.Calibs, key)
		fcLearn.mu.Unlock()
	}()

	band := []forecastPoint{
		{TS: 1, Value: 100, Lo: 90, Hi: 110},
		{TS: 2, Value: 110, Lo: 100, Hi: 120},
	}
	out, cal := applyForecastCalibration(key, band, 100)
	if cal == nil {
		t.Fatal("expected calib")
	}
	if math.Abs(out[0].Value-110) > 0.01 {
		t.Fatalf("bias not applied: got %v", out[0].Value)
	}
	if math.Abs(out[1].Value-120) > 0.01 {
		t.Fatalf("bias+dev: got %v", out[1].Value)
	}
}

func TestLearnAdjustCandidateScorePrefersHistory(t *testing.T) {
	key := "test-score-key"
	fcLearn.mu.Lock()
	fcLearn.Calibs[key] = &forecastCalibration{
		Key: key, EvalCount: 5, Scale: 1,
		MethodScores: map[string]float64{"damped-holt": 80, "flat": 5},
	}
	fcLearn.mu.Unlock()
	defer func() {
		fcLearn.mu.Lock()
		delete(fcLearn.Calibs, key)
		fcLearn.mu.Unlock()
	}()

	holt := learnAdjustCandidateScore(key, "damped-holt", 1.0)
	flat := learnAdjustCandidateScore(key, "flat", 1.0)
	if holt >= flat {
		t.Fatalf("holt should be preferred (lower score): holt=%v flat=%v", holt, flat)
	}
}

func TestNearestPoint(t *testing.T) {
	pts := [][2]float64{{100, 1}, {200, 2}, {300, 3}}
	v, ok := nearestPoint(pts, 210, 50)
	if !ok || v != 2 {
		t.Fatalf("got %v ok=%v", v, ok)
	}
	_, ok = nearestPoint(pts, 1000, 50)
	if ok {
		t.Fatal("should reject far point")
	}
}

func TestSampleForecastBand(t *testing.T) {
	band := make([]forecastPoint, 100)
	for i := range band {
		band[i] = forecastPoint{TS: float64(i), Value: float64(i)}
	}
	got := sampleForecastBand(band, 10)
	if len(got) != 10 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].TS != 0 || got[9].TS != 99 {
		t.Fatalf("endpoints: %+v %+v", got[0], got[9])
	}
}

func TestForecastMetricKeyStable(t *testing.T) {
	a := forecastMetricKey("up", "host=a")
	b := forecastMetricKey("up", "host=a")
	c := forecastMetricKey("up", "host=b")
	if a != b || a == c {
		t.Fatalf("keys a=%s b=%s c=%s", a, b, c)
	}
}

func TestNoteForecastLedgerPersistsInMemory(t *testing.T) {
	s := &Server{}
	key := "ledger-test-" + time.Now().Format("150405")
	band := []forecastPoint{{TS: float64(time.Now().Unix() + 60), Value: 1, Lo: 0, Hi: 2}}
	s.noteForecastLedger(key, "flat", "up", band, time.Now().Unix(), 600, 15, 1)
	fcLearn.mu.Lock()
	defer fcLearn.mu.Unlock()
	found := false
	for _, L := range fcLearn.Ledgers {
		if L.Key == key && L.Expr == "up" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("ledger not recorded")
	}
}
