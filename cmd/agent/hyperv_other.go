//go:build !windows

package main

import (
	"errors"

	"aiops-monitor/shared"
)

// Hyper-V is a Windows-only hypervisor; on other platforms collection is a no-op.
// hypervAvailable returning false means runHyperVCollector is never started.

var errHypervUnsupported = errors.New("Hyper-V 采集仅支持 Windows 宿主机")

func hypervAvailable() bool { return false }

func hypervCollect() ([]shared.HyperVGuest, hypervHostStats, error) {
	return nil, hypervHostStats{}, errHypervUnsupported
}
