package main

import (
	"testing"
	"time"

	"aiops-monitor/shared"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name       string
		v, w, c    float64
		want       string
	}{
		{"below warn", 50, 80, 90, ""},
		{"at warn", 80, 80, 90, "warning"},
		{"between warn and crit", 85, 80, 90, "warning"},
		{"at crit", 90, 80, 90, "critical"},
		{"above crit", 99, 80, 90, "critical"},
		{"zero value", 0, 80, 90, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(tc.v, tc.w, tc.c); got != tc.want {
				t.Errorf("classify(%v, %v, %v) = %q, want %q", tc.v, tc.w, tc.c, got, tc.want)
			}
		})
	}
}

func TestDefaultThresholds(t *testing.T) {
	th := DefaultThresholds()
	if th.CPUWarn != 80 || th.CPUCrit != 95 {
		t.Errorf("cpu thresholds wrong: warn=%v crit=%v", th.CPUWarn, th.CPUCrit)
	}
	if th.MemWarn != 85 || th.MemCrit != 95 {
		t.Errorf("mem thresholds wrong: warn=%v crit=%v", th.MemWarn, th.MemCrit)
	}
	if th.DiskWarn != 80 || th.DiskCrit != 90 {
		t.Errorf("disk thresholds wrong: warn=%v crit=%v", th.DiskWarn, th.DiskCrit)
	}
	if th.OfflineAfter != 60*time.Second {
		t.Errorf("offline after wrong: %v", th.OfflineAfter)
	}
	// Verify StandardThresholds is the same as DefaultThresholds
	st := StandardThresholds()
	if th != st {
		t.Errorf("DefaultThresholds() != StandardThresholds()")
	}
	// Verify ConservativeThresholds has tighter values
	ct := ConservativeThresholds()
	if ct.CPUWarn >= th.CPUWarn || ct.CPUCrit >= th.CPUCrit {
		t.Error("Conservative thresholds should be tighter than Standard")
	}
	// Verify RelaxedThresholds has looser values
	rt := RelaxedThresholds()
	if rt.CPUWarn <= th.CPUWarn || rt.CPUCrit <= th.CPUCrit {
		t.Error("Relaxed thresholds should be looser than Standard")
	}
}

// mkHost builds a Host with a Latest sample for alert tests.
func mkHost(id, name string, lastSeen int64, m shared.Metrics) *Host {
	return &Host{
		ID:       id,
		Hostname: name,
		LastSeen: lastSeen,
		Latest:   &shared.Sample{Timestamp: lastSeen, Metrics: m},
	}
}

func TestEvaluate(t *testing.T) {
	th := DefaultThresholds()
	now := time.Now().Unix()

	t.Run("empty hosts", func(t *testing.T) {
		alerts := Evaluate(nil, th)
		if len(alerts) != 0 {
			t.Errorf("expected no alerts for empty hosts, got %d", len(alerts))
		}
	})

	t.Run("host with no alerts", func(t *testing.T) {
		h := mkHost("h1", "node-1", now, shared.Metrics{
			CPUPercent: 10, MemPercent: 20, DiskPercent: 30, CPUCores: 4,
		})
		alerts := Evaluate([]*Host{h}, th)
		if len(alerts) != 0 {
			t.Errorf("expected no alerts, got %d", len(alerts))
		}
	})

	t.Run("cpu warning at 80 percent", func(t *testing.T) {
		h := mkHost("h1", "node-1", now, shared.Metrics{CPUPercent: 80, CPUCores: 4})
		alerts := Evaluate([]*Host{h}, th)
		if len(alerts) != 1 {
			t.Fatalf("expected 1 alert, got %d", len(alerts))
		}
		if alerts[0].Level != "warning" || alerts[0].Type != "cpu" {
			t.Errorf("wrong alert: %+v", alerts[0])
		}
	})

	t.Run("cpu critical at 95 percent", func(t *testing.T) {
		h := mkHost("h1", "node-1", now, shared.Metrics{CPUPercent: 95, CPUCores: 4})
		alerts := Evaluate([]*Host{h}, th)
		if len(alerts) != 1 {
			t.Fatalf("expected 1 alert, got %d", len(alerts))
		}
		if alerts[0].Level != "critical" || alerts[0].Type != "cpu" {
			t.Errorf("wrong alert: %+v", alerts[0])
		}
	})

	t.Run("memory critical", func(t *testing.T) {
		h := mkHost("h1", "node-1", now, shared.Metrics{MemPercent: 95, CPUCores: 4})
		alerts := Evaluate([]*Host{h}, th)
		found := false
		for _, a := range alerts {
			if a.Type == "memory" && a.Level == "critical" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected critical memory alert, got %+v", alerts)
		}
	})

	t.Run("disk warning per path", func(t *testing.T) {
		h := mkHost("h1", "node-1", now, shared.Metrics{
			CPUCores: 4,
			Disks: []shared.DiskInfo{
				{Path: "/data", Total: 100, Used: 88, Percent: 88}, // warning
				{Path: "/var", Total: 100, Used: 50, Percent: 50},  // ok
			},
		})
		alerts := Evaluate([]*Host{h}, th)
		if len(alerts) != 1 {
			t.Fatalf("expected 1 disk alert, got %d", len(alerts))
		}
		if alerts[0].Type != "disk" || alerts[0].Level != "warning" || alerts[0].Scope != "/data" {
			t.Errorf("wrong disk alert: %+v", alerts[0])
		}
	})

	t.Run("offline host", func(t *testing.T) {
		h := mkHost("h1", "node-1", now-int64(120*time.Second), shared.Metrics{CPUCores: 4})
		alerts := Evaluate([]*Host{h}, th)
		if len(alerts) != 1 {
			t.Fatalf("expected 1 alert, got %d", len(alerts))
		}
		if alerts[0].Type != "offline" || alerts[0].Level != "critical" {
			t.Errorf("wrong offline alert: %+v", alerts[0])
		}
	})

	t.Run("gpu alert above 80 percent", func(t *testing.T) {
		h := mkHost("h1", "node-1", now, shared.Metrics{
			CPUCores: 4,
			GPUs:     []shared.GPUInfo{{Name: "GPU0", UtilPercent: 85}},
		})
		alerts := Evaluate([]*Host{h}, th)
		found := false
		for _, a := range alerts {
			if a.Type == "gpu" && a.Level == "warning" && a.Scope == "GPU0" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected warning gpu alert, got %+v", alerts)
		}
	})

	t.Run("load alert exceeds cores times four", func(t *testing.T) {
		h := mkHost("h1", "node-1", now, shared.Metrics{CPUCores: 4, Load5: 20})
		alerts := Evaluate([]*Host{h}, th)
		found := false
		for _, a := range alerts {
			if a.Type == "load" {
				found = true
				if a.Level != "warning" && a.Level != "critical" {
					t.Errorf("load alert level unexpected: %s", a.Level)
				}
			}
		}
		if !found {
			t.Errorf("expected load alert, got %+v", alerts)
		}
	})
}

func TestEvaluateSortOrder(t *testing.T) {
	th := DefaultThresholds()
	now := time.Now().Unix()
	// Two hosts: "zeta" has a warning, "alpha" has a critical. Critical must
	// come first regardless of hostname; within the same level, alphabetical.
	hosts := []*Host{
		mkHost("h1", "zeta", now, shared.Metrics{CPUPercent: 85, CPUCores: 4}), // warning
		mkHost("h2", "alpha", now, shared.Metrics{CPUPercent: 95, CPUCores: 4}), // critical
	}
	alerts := Evaluate(hosts, th)
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alerts))
	}
	if alerts[0].Level != "critical" {
		t.Errorf("expected critical first, got level=%s host=%s", alerts[0].Level, alerts[0].Hostname)
	}
	if alerts[1].Level != "warning" {
		t.Errorf("expected warning second, got level=%s host=%s", alerts[1].Level, alerts[1].Hostname)
	}

	// Two warnings: alphabetical by hostname.
	hosts2 := []*Host{
		mkHost("h1", "zeta", now, shared.Metrics{CPUPercent: 85, CPUCores: 4}),
		mkHost("h2", "alpha", now, shared.Metrics{CPUPercent: 85, CPUCores: 4}),
	}
	alerts2 := Evaluate(hosts2, th)
	if len(alerts2) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alerts2))
	}
	if alerts2[0].Hostname != "alpha" {
		t.Errorf("expected alpha first (alphabetical), got %s", alerts2[0].Hostname)
	}
	if alerts2[1].Hostname != "zeta" {
		t.Errorf("expected zeta second (alphabetical), got %s", alerts2[1].Hostname)
	}
}
