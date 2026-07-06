package main

import "aiops-monitor/shared"

// Collector gathers base system metrics natively. Implementations are
// platform-specific and selected at build time via build tags:
//   - collector_linux.go   (procfs + syscall)
//   - collector_windows.go (Win32 API via syscall.NewLazyDLL)
//   - collector_darwin.go  (sysctl + system tools)
//   - collector_other.go   (stub; base metrics come from a core plugin such as
//                            plugins/core_metrics.py via psutil)
type Collector interface {
	Collect() (shared.Metrics, error)
	Supported() bool
	Name() string
}

// ---- rounding + rate helpers shared by every native collector ----

func round1(f float64) float64 { return float64(int64(f*10+0.5)) / 10 }
func round2(f float64) float64 { return float64(int64(f*100+0.5)) / 100 }

// rate computes a per-second rate from two monotonic counters. It returns 0
// on counter wrap (cur < prev) or a non-positive interval, so a freshly primed
// collector never emits a bogus spike.
func rate(cur, prev uint64, elapsed float64) float64 {
	if cur < prev || elapsed <= 0 {
		return 0
	}
	return float64(cur-prev) / elapsed
}
