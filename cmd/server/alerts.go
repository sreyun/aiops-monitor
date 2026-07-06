package main

import (
	"fmt"
	"sort"
	"time"
)

// Thresholds define when a base metric is warning / critical. A production
// version would load these per-host / per-rule and require a sustained
// duration before firing. Plugin-generated findings arrive as Events instead
// and are surfaced separately from these threshold alerts.
type Thresholds struct {
	CPUWarn, CPUCrit   float64
	MemWarn, MemCrit   float64
	DiskWarn, DiskCrit float64
	OfflineAfter       time.Duration
}

func DefaultThresholds() Thresholds {
	return Thresholds{
		CPUWarn: 80, CPUCrit: 90,
		MemWarn: 80, MemCrit: 90,
		DiskWarn: 85, DiskCrit: 95,
		OfflineAfter: 30 * time.Second,
	}
}

// Alert is a single fired threshold condition on base metrics.
type Alert struct {
	HostID    string  `json:"host_id"`
	Hostname  string  `json:"hostname"`
	Level     string  `json:"level"`           // warning | critical
	Type      string  `json:"type"`            // cpu | memory | disk | offline | check
	Scope     string  `json:"scope,omitempty"` // sub-target (e.g. disk path) for per-item dedup
	Since     int64   `json:"since,omitempty"` // unix time the condition first fired (for duration display)
	Message   string  `json:"message"`
	Value     float64 `json:"value"`
	Timestamp int64   `json:"timestamp"`
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
				Level:     "critical",
				Type:      "offline",
				Message:   fmt.Sprintf("主机 %s 已失联 %d 秒", h.Hostname, now-h.LastSeen),
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
				HostID: h.ID, Hostname: h.Hostname, Level: lv, Type: "cpu",
				Message:   fmt.Sprintf("CPU 使用率 %.1f%%（空闲 %.1f%%）", m.CPUPercent, 100-m.CPUPercent),
				Value:     m.CPUPercent, Timestamp: now,
			})
		}
		if lv := classify(m.MemPercent, t.MemWarn, t.MemCrit); lv != "" {
			alerts = append(alerts, Alert{
				HostID: h.ID, Hostname: h.Hostname, Level: lv, Type: "memory",
				Message:   fmt.Sprintf("内存使用率 %.1f%%（剩余 %s）", m.MemPercent, fmtBytes(m.MemTotal-m.MemUsed)),
				Value:     m.MemPercent, Timestamp: now,
			})
		}
		if len(m.Disks) > 0 {
			for _, d := range m.Disks {
				if lv := classify(d.Percent, t.DiskWarn, t.DiskCrit); lv != "" {
					alerts = append(alerts, Alert{
						HostID: h.ID, Hostname: h.Hostname, Level: lv, Type: "disk", Scope: d.Path,
						Message:   fmt.Sprintf("磁盘 %s 使用率 %.1f%%（剩余 %s）", d.Path, d.Percent, fmtBytes(d.Total-d.Used)),
						Value:     d.Percent, Timestamp: now,
					})
				}
			}
		} else if lv := classify(m.DiskPercent, t.DiskWarn, t.DiskCrit); lv != "" {
			alerts = append(alerts, Alert{
				HostID: h.ID, Hostname: h.Hostname, Level: lv, Type: "disk",
				Message:   fmt.Sprintf("磁盘使用率 %.1f%%（剩余 %s）", m.DiskPercent, fmtBytes(m.DiskTotal-m.DiskUsed)),
				Value:     m.DiskPercent, Timestamp: now,
			})
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
		return fmt.Sprintf("%.1f GB", float64(b)/gb)
	case b >= mb:
		return fmt.Sprintf("%.0f MB", float64(b)/mb)
	default:
		return fmt.Sprintf("%d KB", b/1024)
	}
}
