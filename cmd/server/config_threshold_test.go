package main

import (
	"path/filepath"
	"testing"
)

// A fully-zeroed threshold set is healed entirely to the standard defaults.
func TestBackfillThresholdAllZero(t *testing.T) {
	var tc ThresholdConfig // all zero
	changed := backfillThresholdDefaults(&tc)
	if !changed {
		t.Fatal("expected changed=true for all-zero thresholds")
	}
	d := defaultThresholdConfig()
	if tc != d {
		t.Fatalf("all-zero should backfill to defaults\n got: %+v\nwant: %+v", tc, d)
	}
}

// Only the zero (unset) fields are healed; explicitly-set values are preserved.
func TestBackfillThresholdPartial(t *testing.T) {
	tc := ThresholdConfig{CPUWarn: 70, CPUCrit: 88, MemWarn: 75, MemCrit: 90,
		DiskWarn: 80, DiskCrit: 92, OfflineAfterSec: 45}
	// iops/gpu/load/proc/diskio left at 0 — the exact bug from the saved config.
	backfillThresholdDefaults(&tc)
	d := defaultThresholdConfig()
	if tc.CPUWarn != 70 || tc.CPUCrit != 88 || tc.MemWarn != 75 || tc.OfflineAfterSec != 45 {
		t.Errorf("explicitly-set values must be preserved, got %+v", tc)
	}
	if tc.IOPSWarn != d.IOPSWarn || tc.GPUWarn != d.GPUWarn || tc.LoadWarn != d.LoadWarn ||
		tc.ProcWarn != d.ProcWarn || tc.DiskIOWarn != d.DiskIOWarn {
		t.Errorf("zero fields must be healed to defaults, got %+v", tc)
	}
}

// Saving a config whose new-metric thresholds are 0 (blank form) persists the
// standard defaults instead of the meaningless zeros, and passes validation.
func TestSetBackfillsThresholds(t *testing.T) {
	cs, err := NewConfigStore(filepath.Join(t.TempDir(), "cfg.json"))
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}
	in := defaultServerConfig()
	in.Thresholds = ThresholdConfig{CPUWarn: 80, CPUCrit: 95, MemWarn: 85, MemCrit: 95,
		DiskWarn: 80, DiskCrit: 90, OfflineAfterSec: 60}
	// iops/gpu/load/proc/diskio deliberately 0 (as the browser form sent them).
	if err := cs.Set(in); err != nil {
		t.Fatalf("Set with zero new-metric thresholds should succeed, got: %v", err)
	}
	got := cs.Thresholds()
	d := defaultThresholdConfig()
	if got.IOPSWarn != d.IOPSWarn || got.GPUWarn != d.GPUWarn || got.LoadWarn != d.LoadWarn || got.ProcWarn != d.ProcWarn {
		t.Errorf("Set must backfill zero thresholds to defaults, got IOPS=%.0f GPU=%.0f Load=%.1f Proc=%.2f",
			got.IOPSWarn, got.GPUWarn, got.LoadWarn, got.ProcWarn)
	}
}
