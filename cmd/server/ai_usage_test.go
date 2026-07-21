package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEstimateAICost(t *testing.T) {
	cfg := AIConfig{InputPricePer1M: 1, OutputPricePer1M: 2}
	cost := estimateAICost(cfg, 0, 1_000_000, 0)
	if cost < 1.9 || cost > 2.1 {
		t.Fatalf("expected ~2 for 1M completion tokens, got %v", cost)
	}
	cost = estimateAICost(cfg, 0, 0, 500_000)
	if cost < 0.9 || cost > 1.1 {
		t.Fatalf("approx fallback expected ~1, got %v", cost)
	}
	if estimateAICost(AIConfig{}, 100, 100, 100) != 0 {
		t.Fatal("zero prices should yield zero cost")
	}
}

func TestParseTimeRangeQueryDefaults(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/ai/usage/history", nil)
	from, to := parseTimeRangeQuery(r, 24*time.Hour)
	if to <= from {
		t.Fatalf("to=%d from=%d", to, from)
	}
	span := to - from
	if span < 23*3600 || span > 25*3600 {
		t.Fatalf("unexpected span %d", span)
	}
	r2 := httptest.NewRequest(http.MethodGet, "/x?from=100&to=200", nil)
	f2, t2 := parseTimeRangeQuery(r2, time.Hour)
	if f2 != 100 || t2 != 200 {
		t.Fatalf("got from=%d to=%d", f2, t2)
	}
}

func TestAIStatsHubStillRecords(t *testing.T) {
	h := newAIStatsHub()
	h.record(aiCallStat{Ts: time.Now().Unix(), Task: "chat", Model: "m", LatencyMs: 10, OK: true, ApproxTokens: 5, CostEstimate: 0.01})
	snap := h.snapshot()
	if snap["total"].(int64) != 1 {
		t.Fatalf("total=%v", snap["total"])
	}
}
