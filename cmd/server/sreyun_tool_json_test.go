package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolResultJSONBoundedNeverMidCut(t *testing.T) {
	big := map[string]any{
		"items": strings.Repeat("x", 20000),
	}
	out := toolResultJSONBounded(big, 1000)
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("must be valid JSON, got %q err=%v", out, err)
	}
	if m["truncated"] != true {
		t.Fatalf("expected truncated notice: %s", out)
	}
}

func TestContainerInventorySummaryOmitsList(t *testing.T) {
	row := map[string]any{
		"host_id": "h1", "host_name": "web", "runtime": "docker",
		"container_count": 2, "updated_at": int64(1),
		"containers": []any{
			map[string]any{"name": "nginx", "state": "running"},
			map[string]any{"name": "redis", "state": "exited"},
		},
	}
	sum := containerInventorySummary(row)
	if _, ok := sum["containers"]; ok {
		t.Fatal("summary must not include full containers")
	}
	if sum["running"] != 1 || sum["exited"] != 1 {
		t.Fatalf("counts=%v", sum)
	}
}
