package main

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Thresholds define when a base metric is warning / critical. A production
// version would load these per-host / per-rule and require a sustained
// duration before firing. Plugin-generated findings arrive as Events instead
// and are surfaced separately from these threshold alerts.
//
// Three preset profiles are available (v5.4.1):
//   - ConservativeThresholds() — sensitive, for production-critical systems
//   - StandardThresholds()    — recommended default, balanced noise/sensitivity
//   - RelaxedThresholds()     — low-noise, for dev/staging environments
type Thresholds struct {
	CPUWarn, CPUCrit   float64
	MemWarn, MemCrit   float64
	DiskWarn, DiskCrit float64
	DiskIOWarn, DiskIOCrit float64
	IOPSWarn, IOPSCrit float64
	GPUWarn, GPUCrit   float64
	LoadWarn, LoadCrit float64 // 按 CPU 核心数倍率
	ProcWarn           float64 // 进程数突增/突降比例阈值
	OfflineAfter       time.Duration
}

// DefaultThresholds returns the Standard profile (recommended defaults).
// This is the alias used by the config store for new installations.
func DefaultThresholds() Thresholds {
	return StandardThresholds()
}

// ConservativeThresholds returns sensitive thresholds for production-critical
// systems where early warning is preferred over reducing noise.
func ConservativeThresholds() Thresholds {
	return Thresholds{
		CPUWarn: 70, CPUCrit: 85,
		MemWarn: 75, MemCrit: 90,
		DiskWarn: 75, DiskCrit: 85,
		DiskIOWarn: 70, DiskIOCrit: 85,
		IOPSWarn: 20000, IOPSCrit: 50000,
		GPUWarn: 70, GPUCrit: 85,
		LoadWarn: 2.0, LoadCrit: 4.0,
		ProcWarn: 0.3,
		OfflineAfter: 30 * time.Second,
	}
}

// StandardThresholds returns the recommended balanced thresholds for most
// deployments. This is the new default since v5.4.1.
func StandardThresholds() Thresholds {
	return Thresholds{
		CPUWarn: 80, CPUCrit: 95,
		MemWarn: 85, MemCrit: 95,
		DiskWarn: 80, DiskCrit: 90,
		DiskIOWarn: 80, DiskIOCrit: 95,
		IOPSWarn: 50000, IOPSCrit: 100000,
		GPUWarn: 80, GPUCrit: 95,
		LoadWarn: 4.0, LoadCrit: 8.0,
		ProcWarn: 0.5,
		OfflineAfter: 60 * time.Second,
	}
}

// RelaxedThresholds returns low-noise thresholds suitable for dev/staging
// environments where alert fatigue should be minimized.
func RelaxedThresholds() Thresholds {
	return Thresholds{
		CPUWarn: 90, CPUCrit: 98,
		MemWarn: 90, MemCrit: 98,
		DiskWarn: 90, DiskCrit: 97,
		DiskIOWarn: 90, DiskIOCrit: 98,
		IOPSWarn: 100000, IOPSCrit: 200000,
		GPUWarn: 90, GPUCrit: 98,
		LoadWarn: 6.0, LoadCrit: 12.0,
		ProcWarn: 0.8,
		OfflineAfter: 120 * time.Second,
	}
}

// Alert is a single fired threshold condition on base metrics.
type Alert struct {
	HostID    string  `json:"host_id"`
	Hostname  string  `json:"hostname"`
	IP        string  `json:"ip"`
	Level     string  `json:"level"`           // warning | critical
	Type      string  `json:"type"`            // cpu | memory | disk | diskio | iops | offline | check | load | gpu | proc
	Scope     string  `json:"scope,omitempty"` // sub-target (e.g. disk path) for per-item dedup
	Since     int64   `json:"since,omitempty"` // unix time the condition first fired (for duration display)
	Message   string  `json:"message"`
	Value     float64 `json:"value"`
	Timestamp int64   `json:"timestamp"`
	Status    string  `json:"status,omitempty"` // acknowledged | silenced | "" (active)
}

func classify(v, warn, crit float64) string {
	switch {
	case v >= crit:
		return "critical"
	case v >= warn:
		return "warning"
	default:
		return ""
	}
}

// Evaluate computes the current threshold-alert set from host state.
func Evaluate(hosts []*Host, t Thresholds) []Alert {
	now := time.Now().Unix()
	offlineSec := int64(t.OfflineAfter.Seconds())
	var alerts []Alert

	for _, h := range hosts {
		if now-h.LastSeen > offlineSec {
			alerts = append(alerts, Alert{
				HostID:    h.ID,
				Hostname:  h.Hostname,
				IP:        h.IP,
				Level:     "critical",
				Type:      "offline",
				Message:   Tz("alert.offline", h.Hostname, h.IP, now-h.LastSeen),
				Value:     float64(now - h.LastSeen),
				Timestamp: now,
			})
			continue
		}
		if h.Latest == nil {
			continue
		}
		m := h.Latest
		if lv := classify(m.CPUPercent, t.CPUWarn, t.CPUCrit); lv != "" {
			alerts = append(alerts, Alert{
				HostID: h.ID, Hostname: h.Hostname, IP: h.IP, Level: lv, Type: "cpu",
				Message:   Tz("alert.cpu_high", m.CPUPercent, 100-m.CPUPercent),
				Value:     m.CPUPercent, Timestamp: now,
			})
		}
		if lv := classify(m.MemPercent, t.MemWarn, t.MemCrit); lv != "" {
			alerts = append(alerts, Alert{
				HostID: h.ID, Hostname: h.Hostname, IP: h.IP, Level: lv, Type: "memory",
				Message:   Tz("alert.mem_high", m.MemPercent, fmtBytes(m.MemTotal-m.MemUsed)),
				Value:     m.MemPercent, Timestamp: now,
			})
		}
		if len(m.Disks) > 0 {
			for _, d := range m.Disks {
				if lv := classify(d.Percent, t.DiskWarn, t.DiskCrit); lv != "" {
					alerts = append(alerts, Alert{
						HostID: h.ID, Hostname: h.Hostname, IP: h.IP, Level: lv, Type: "disk", Scope: d.Path,
						Message:   Tz("alert.disk_path_high", d.Path, d.Percent, fmtBytes(d.Total-d.Used)),
						Value:     d.Percent, Timestamp: now,
					})
				}
			}
		} else if lv := classify(m.DiskPercent, t.DiskWarn, t.DiskCrit); lv != "" {
			alerts = append(alerts, Alert{
				HostID: h.ID, Hostname: h.Hostname, IP: h.IP, Level: lv, Type: "disk",
				Message:   Tz("alert.disk_high", m.DiskPercent, fmtBytes(m.DiskTotal-m.DiskUsed)),
				Value:     m.DiskPercent, Timestamp: now,
			})
		}
		// System load alert (5-min load exceeding core count × threshold)
		if m.CPUCores > 0 {
			loadWarn := float64(m.CPUCores) * t.LoadWarn
			loadCrit := float64(m.CPUCores) * t.LoadCrit
			if m.Load5 >= loadWarn {
				lv := "warning"
				if m.Load5 >= loadCrit {
					lv = "critical"
				}
				alerts = append(alerts, Alert{
					HostID: h.ID, Hostname: h.Hostname, IP: h.IP, Level: lv, Type: "load",
					Message:   Tz("alert.load_high", m.Load5, m.CPUCores, loadWarn),
					Value:     m.Load5, Timestamp: now,
				})
			}
		}
		// GPU alert (configurable thresholds)
		for _, g := range m.GPUs {
			util := g.UtilPercent
			if lv := classify(util, t.GPUWarn, t.GPUCrit); lv != "" {
				tempStr := ""
				if g.Temp > 0 {
					tempStr = Tz("alert.gpu_temp", int(g.Temp))
				}
				alerts = append(alerts, Alert{
					HostID: h.ID, Hostname: h.Hostname, IP: h.IP, Level: lv, Type: "gpu", Scope: g.Name,
					Message:   Tz("alert.gpu_high", g.Name, util, tempStr),
					Value:     util, Timestamp: now,
				})
			}
		}
		// Disk IO alert (>80% warning, >90% critical)
		if m.DiskIOUtilPercent > 0 {
			if lv := classify(m.DiskIOUtilPercent, t.DiskIOWarn, t.DiskIOCrit); lv != "" {
				alerts = append(alerts, Alert{
					HostID: h.ID, Hostname: h.Hostname, IP: h.IP, Level: lv, Type: "diskio",
					Message: Tz("alert.diskio_high", m.DiskIOUtilPercent,
						fmtRateBytes(m.DiskReadRate), fmtRateBytes(m.DiskWriteRate)),
					Value:     m.DiskIOUtilPercent, Timestamp: now,
				})
			}
		}
		// IOPS alert (>10000 warning, >20000 critical)
		totalIOPS := m.DiskReadIOPS + m.DiskWriteIOPS
		if totalIOPS > 0 {
			if lv := classify(totalIOPS, t.IOPSWarn, t.IOPSCrit); lv != "" {
				alerts = append(alerts, Alert{
					HostID: h.ID, Hostname: h.Hostname, IP: h.IP, Level: lv, Type: "iops",
					Message: Tz("alert.iops_high", totalIOPS, m.DiskReadIOPS, m.DiskWriteIOPS),
					Value:     totalIOPS, Timestamp: now,
				})
			}
		}
		// Process count anomaly: compare current proc count vs 1h baseline
		if m.ProcCount > 0 && t.ProcWarn > 0 && len(h.hist1m) > 0 {
			var sumProc float64
			for _, s := range h.hist1m {
				sumProc += float64(s.ProcCount)
			}
			baseline := sumProc / float64(len(h.hist1m))
			if baseline > 0 {
				change := math.Abs(float64(m.ProcCount)-baseline) / baseline
				if change >= t.ProcWarn {
					dir := "increase"
					if float64(m.ProcCount) < baseline { dir = "decrease" }
					alerts = append(alerts, Alert{
						HostID: h.ID, Hostname: h.Hostname, IP: h.IP, Level: "warning", Type: "proc",
						Message: Tz("alert.proc_anomaly", m.ProcCount, baseline, change*100, dir),
						Value:     change * 100, Timestamp: now,
					})
				}
			}
		}
	}

	sort.SliceStable(alerts, func(i, j int) bool {
		if alerts[i].Level != alerts[j].Level {
			return alerts[i].Level == "critical"
		}
		return alerts[i].Hostname < alerts[j].Hostname
	})
	return alerts
}

// fmtBytes renders a byte count as a human-readable amount for alert messages.
func fmtBytes(b uint64) string {
	const gb, mb = 1 << 30, 1 << 20
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1fG", float64(b)/gb)
	case b >= mb:
		return fmt.Sprintf("%.0fM", float64(b)/mb)
	default:
		return fmt.Sprintf("%dK", b/1024)
	}
}

// fmtRateBytes renders a bytes/sec rate as human-readable (e.g. "12.3 MB/s").
func fmtRateBytes(bps float64) string {
	switch {
	case bps >= 1e9:
		return fmt.Sprintf("%.1f GB/s", bps/1e9)
	case bps >= 1e6:
		return fmt.Sprintf("%.1f MB/s", bps/1e6)
	case bps >= 1e3:
		return fmt.Sprintf("%.0f KB/s", bps/1e3)
	default:
		return fmt.Sprintf("%.0f B/s", bps)
	}
}
