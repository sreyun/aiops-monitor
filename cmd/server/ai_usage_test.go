package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
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

// TestAIUsageSlotConcurrency is the regression guard for billing attribution:
// 50 concurrent AI calls, each binding a usage slot, must each read back their
// OWN prompt/completion tokens — never another goroutine's. This is what
// prevents cost cross-talk (串号) that would break per-user billing.
func TestAIUsageSlotConcurrency(t *testing.T) {
	const n = 50
	var wg sync.WaitGroup
	errs := make(chan string, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			wantP, wantC := i*10+1, i*10+2

			ctx, id := withAIUsageSlot(context.Background())
			defer endAIUsageSlot(id)
			captureAIUsageCtx(ctx, wantP, wantC)

			gotP, gotC, ok := takeCapturedAIUsageCtx(ctx)
			if !ok {
				errs <- fmt.Sprintf("goroutine %d: no usage captured", i)
				return
			}
			if gotP != wantP || gotC != wantC {
				errs <- fmt.Sprintf("goroutine %d: cross-talk got p=%d c=%d want p=%d c=%d", i, gotP, gotC, wantP, wantC)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

// TestAIUsageSlotConsumerMarksExact verifies that a captured slot yields
// usage_source='exact' while a slot-less read falls back to estimated.
func TestAIUsageSlotConsumerMarksExact(t *testing.T) {
	s := &Server{aiStats: newAIStatsHub()}

	// With a bound slot -> exact.
	ctx, id := withAIUsageSlot(context.Background())
	captureAIUsageCtx(ctx, 100, 50)
	s.recordAICallActor(ctx, "chat", "m", "u", 10, true, "", 0, 0, "x")
	endAIUsageSlot(id)
	snap := s.aiStats.snapshot()
	recent := snap["recent"].([]aiCallStat)
	if len(recent) != 1 || recent[0].UsageSource != "exact" || recent[0].PromptTokens != 100 || recent[0].CompletionTokens != 50 {
		t.Fatalf("expected exact usage, got %+v", recent[0])
	}

	// No slot -> estimated.
	s2 := &Server{aiStats: newAIStatsHub()}
	s2.recordAICallActor(context.Background(), "chat", "m", "u", 10, true, "", 0, 0, "hello world")
	snap2 := s2.aiStats.snapshot()
	recent2 := snap2["recent"].([]aiCallStat)
	if len(recent2) != 1 || recent2[0].UsageSource != "estimated" {
		t.Fatalf("expected estimated, got %+v", recent2[0])
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
