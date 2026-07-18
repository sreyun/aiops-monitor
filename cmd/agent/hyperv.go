package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"aiops-monitor/shared"
)

// Hyper-V guest inventory collection.
//
// Runs only on Windows Hyper-V hosts (hypervAvailable gates the goroutine; the
// non-Windows stub returns false). Guest inventory is slow-changing list data,
// so it rides its own low-frequency channel (POST /api/v1/agent/hyperv) instead
// of the high-frequency base-metric report — same split as Redfish hardware.
//
// parseHyperV / splitComma / hypervHealth live here (cross-platform, pure) so
// they stay unit-testable on any OS; only the PowerShell exec is Windows-only.

// hypervRaw mirrors one object emitted by hypervScript's ConvertTo-Json. Field
// names match the PowerShell property names (json is case-insensitive). IP and
// Switches arrive comma-joined to dodge PS 5.1's "single-element array collapses
// to a scalar" JSON quirk.
type hypervRaw struct {
	Name            string
	Id              string
	State           string
	Status          string
	CPUUsage        float64
	ProcessorCount  int
	MemAssignedMB   float64
	MemDemandMB     float64
	MemMaxMB        float64
	DynamicMemoryEnabled bool
	UptimeSec       int64
	Generation      int
	Version         string
	IP              string
	Switches        string
	VHDCount        int
	CheckpointCount int
	ReplState       string
	ReplHealth      string
}

// parseHyperV parses hypervScript's JSON output into guests. It tolerates the
// three shapes PS 5.1 can emit: "[]"/null (no VMs), a bare "{...}" object (one
// VM — ConvertTo-Json drops the array brackets), and a normal "[...]" array.
func parseHyperV(out string) ([]shared.HyperVGuest, error) {
	s := strings.TrimSpace(out)
	if s == "" || s == "null" || s == "[]" {
		return nil, nil
	}
	if strings.HasPrefix(s, "{") { // single VM: wrap back into an array
		s = "[" + s + "]"
	}
	var raws []hypervRaw
	if err := json.Unmarshal([]byte(s), &raws); err != nil {
		return nil, err
	}
	guests := make([]shared.HyperVGuest, 0, len(raws))
	for _, r := range raws {
		if r.Name == "" {
			continue
		}
		g := shared.HyperVGuest{
			Name: r.Name, ID: r.Id, State: r.State, Status: r.Status,
			CPUUsage: r.CPUUsage, ProcessorCount: r.ProcessorCount,
			MemAssignedMB: r.MemAssignedMB, MemDemandMB: r.MemDemandMB, MemMaxMB: r.MemMaxMB,
			DynamicMemEnabled: r.DynamicMemoryEnabled,
			UptimeSec: r.UptimeSec, Generation: r.Generation, Version: r.Version,
			IPAddresses: splitComma(r.IP), Switches: splitComma(r.Switches),
			VHDCount: r.VHDCount, CheckpointCount: r.CheckpointCount,
			ReplState: r.ReplState, ReplHealth: r.ReplHealth,
		}
		g.Health = hypervHealth(r.State, r.ReplHealth)
		guests = append(guests, g)
	}
	return guests, nil
}

// splitComma turns a comma-joined field into a trimmed, empty-free slice.
func splitComma(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// hypervHealth normalizes a guest's health from its (non-localized) State enum
// and ReplicationHealth. Hyper-V's "*Critical" states (RunningCritical,
// OffCritical, …) mean the VM is in real trouble (e.g. its storage dropped).
// Off itself is NOT unhealthy here — an intentionally-stopped VM shouldn't read
// as a fault; the alert layer decides whether a stopped VM warrants a warning.
func hypervHealth(state, replHealth string) string {
	if strings.HasSuffix(state, "Critical") {
		return "Critical"
	}
	switch replHealth {
	case "Critical":
		return "Critical"
	case "Warning":
		return "Warning"
	}
	switch state {
	case "Running":
		return "OK"
	case "Paused", "Saved":
		return "Warning"
	}
	return ""
}

// runHyperVCollector periodically collects the guest inventory and posts it to
// all servers. On collection failure it still posts a report carrying Error so
// the server can surface "collection failed" without discarding the last good
// inventory.
func (a *Agent) runHyperVCollector() {
	interval := a.hypervInterval
	if interval < 30*time.Second {
		interval = 60 * time.Second
	}
	slog.Info("Hyper-V 采集器启动", "interval", interval)

	collectAndPost := func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Hyper-V 采集 panic 已恢复", "panic", r)
			}
		}()
		guests, err := hypervCollect()
		rep := shared.HyperVReport{
			HostID:      a.identity.HostID,
			Fingerprint: a.identity.Fingerprint,
			Timestamp:   time.Now().Unix(),
			HostName:    a.identity.Hostname,
			Guests:      guests,
		}
		if err != nil {
			rep.Error = err.Error()
			slog.Warn("Hyper-V 采集失败", "err", err)
		} else {
			slog.Info("Hyper-V 采集完成", "vms", len(guests))
		}
		a.postHyperVReport(rep)
	}

	collectAndPost() // report immediately on start
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		collectAndPost()
	}
}

// postHyperVReport sends the Hyper-V guest inventory to all server targets.
// Mirrors postHardwareReport: fingerprint in the X-Agent-Fingerprint header,
// no credential in the body.
func (a *Agent) postHyperVReport(rep shared.HyperVReport) {
	body, err := json.Marshal(rep)
	if err != nil {
		slog.Warn("Hyper-V 上报序列化失败", "err", err)
		return
	}
	fp := a.identity.Fingerprint
	for _, t := range a.targets {
		go func(tgt *serverTarget) {
			req, err := http.NewRequest("POST", tgt.server+"/api/v1/agent/hyperv", bytes.NewReader(body))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			if fp != "" {
				req.Header.Set("X-Agent-Fingerprint", fp)
			}
			resp, err := tgt.httpc.Do(req)
			if err != nil {
				slog.Warn("Hyper-V 上报失败", "server", tgt.server, "err", err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
				slog.Warn("Hyper-V 上报被拒", "server", tgt.server, "status", resp.StatusCode,
					"host_id", rep.HostID, "vms", len(rep.Guests), "body", string(respBody))
			} else {
				slog.Info("Hyper-V 上报成功", "server", tgt.server, "host_id", rep.HostID, "vms", len(rep.Guests))
			}
		}(t)
	}
}
