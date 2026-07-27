package main

import (
	"encoding/json"
	"strings"
	"testing"

	"aiops-monitor/shared"
)

func TestParseChartRange(t *testing.T) {
	from, to, label := parseChartRange("24h", 6)
	if label != "24h" || to-from != 24*3600 {
		t.Fatalf("24h => %s %d", label, to-from)
	}
	from, to, label = parseChartRange("7d", 6)
	if label != "7d" || to-from != 7*24*3600 {
		t.Fatalf("7d => %s %d", label, to-from)
	}
	_, _, label = parseChartRange("", 6)
	if label != "6h" {
		t.Fatalf("default label=%s", label)
	}
}

func TestHostSamplesToChatChart(t *testing.T) {
	samples := make([]shared.Sample, 0, 20)
	for i := 0; i < 20; i++ {
		samples = append(samples, shared.Sample{
			Timestamp: int64(1000 + i*60),
			Metrics:   shared.Metrics{CPUPercent: float64(10 + i), MemPercent: float64(40 + i%5)},
		})
	}
	chart, stats := hostSamplesToChatChart(samples, []string{"cpu", "memory"}, "demo")
	series, _ := chart["series"].([]map[string]any)
	rows, _ := chart["samples"].([]map[string]any)
	if len(series) != 2 || len(rows) != 20 {
		t.Fatalf("series=%d rows=%d", len(series), len(rows))
	}
	if _, ok := stats["cpu"]; !ok {
		t.Fatal("missing cpu stats")
	}
}

func TestShowChartActionEnvelope(t *testing.T) {
	act := showChartAction("c1", "查看趋势", "CPU", map[string]any{"samples": []any{}}, map[string]any{"kind": "host_history"})
	if act["type"] != "show_chart" || act["id"] != "c1" {
		t.Fatalf("%#v", act)
	}
	dd := drillDownAction("下钻", "host_detail", map[string]any{"host_id": "h1"})
	if dd["target"] != "host_detail" || dd["host_id"] != "h1" {
		t.Fatalf("%#v", dd)
	}
	raw, _ := json.Marshal(capabilityResult{OK: true, UIActions: []map[string]any{act, dd}})
	if !strings.Contains(string(raw), `"_ui_actions"`) {
		t.Fatalf("missing ui actions: %s", raw)
	}
}

func TestNormalizeMetricKeys(t *testing.T) {
	got := normalizeMetricKeys("cpu,mem,disk", "cpu")
	if len(got) != 3 || got[1] != "memory" {
		t.Fatalf("%v", got)
	}
}
