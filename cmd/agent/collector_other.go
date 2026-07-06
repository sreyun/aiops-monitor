//go:build !linux && !windows && !darwin

package main

import (
	"runtime"

	"aiops-monitor/shared"
)

// stubCollector is the fallback for platforms without a native collector.
// Base metrics are then expected from a core plugin such as
// plugins/core_metrics.py (psutil), merged in behind the same Collector
// interface. Linux, Windows and macOS all have native collectors.
type stubCollector struct{}

func newCollector(_ string) Collector { return stubCollector{} }

func (stubCollector) Collect() (shared.Metrics, error) { return shared.Metrics{}, nil }
func (stubCollector) Supported() bool                  { return false }
func (stubCollector) Name() string                     { return "unsupported (use core plugin)" }

func osVersion() string     { return runtime.GOOS }
func kernelVersion() string { return "" }
