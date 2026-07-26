package main

import "testing"

func TestAttachSlowSQLTrend(t *testing.T) {
	prev := &SlowSQLReport{
		ID: "p1", Status: "completed",
		Items: []SlowSQLItem{
			{Digest: "d1", SQL: "SELECT 1", AvgLatencyMs: 100},
			{Digest: "d2", SQL: "SELECT 2", AvgLatencyMs: 200},
		},
	}
	cur := &SlowSQLReport{
		ID: "c1", Status: "completed",
		Items: []SlowSQLItem{
			{Digest: "d1", SQL: "SELECT 1", AvgLatencyMs: 160}, // worse
			{Digest: "d3", SQL: "SELECT 3", AvgLatencyMs: 50},  // new
		},
	}
	attachSlowSQLTrend(prev, cur)
	if cur.Trend == nil {
		t.Fatal("expected trend")
	}
	if cur.Trend.NewDigests != 1 || cur.Trend.GoneDigests != 1 || cur.Trend.Worsened != 1 {
		t.Fatalf("trend=%+v", cur.Trend)
	}
	if cur.Items[0].Trend != "worse" || cur.Items[1].Trend != "new" {
		t.Fatalf("item trends=%q %q", cur.Items[0].Trend, cur.Items[1].Trend)
	}
}

func TestParseKillSQL(t *testing.T) {
	id, ok := parseKillSQL("KILL 123")
	if !ok || id != 123 {
		t.Fatalf("got %d %v", id, ok)
	}
	id, ok = parseKillSQL("KILL CONNECTION 9")
	if !ok || id != 9 {
		t.Fatalf("conn got %d %v", id, ok)
	}
	if _, ok := parseKillSQL("DROP TABLE t"); ok {
		t.Fatal("should reject")
	}
}
