package main

import (
	"sync"
	"time"

	"aiops-monitor/shared"
)

// ---------------------------------------------------------------------------
// Hardware alerting
//
// Hardware alerts are driven by the BMC's OWN verdict (Redfish Health / Status
// and each sensor's UpperCaution / UpperCritical), not by fixed user thresholds:
// the BMC knows its chassis' real limits, which vary per vendor/model. That also
// keeps this out of the ThresholdConfig plumbing entirely.
//
// The latest snapshot per host is kept in memory (fed at ingest) so the notifier
// can re-evaluate every tick without hitting PG each time.
// ---------------------------------------------------------------------------

type hwHostEntry struct {
	hostname  string
	ip        string
	updatedAt int64
	snaps     []shared.HardwareSnapshot
}

// hardwareStore holds the most recent hardware snapshots per host.
type hardwareStore struct {
	mu         sync.RWMutex
	byID       map[string]hwHostEntry
	lastHealth map[string]string // hostID|target → last seen health, for event dedup
}

func newHardwareStore() *hardwareStore {
	return &hardwareStore{byID: map[string]hwHostEntry{}, lastHealth: map[string]string{}}
}

// healthChanged reports whether a target's health differs from the last report,
// so hardware events are recorded on TRANSITIONS only. Without this the events
// table grows forever (one row per poll) for any host stuck in Warning.
func (hs *hardwareStore) healthChanged(hostID, target, health string) bool {
	if hs == nil {
		return true
	}
	k := hostID + "|" + target
	hs.mu.Lock()
	defer hs.mu.Unlock()
	if hs.lastHealth[k] == health {
		return false
	}
	hs.lastHealth[k] = health
	return true
}

// put replaces a host's snapshots with the newest report.
func (hs *hardwareStore) put(hostID, hostname, ip string, snaps []shared.HardwareSnapshot) {
	if hs == nil || hostID == "" {
		return
	}
	cp := make([]shared.HardwareSnapshot, len(snaps))
	copy(cp, snaps)
	hs.mu.Lock()
	hs.byID[hostID] = hwHostEntry{hostname: hostname, ip: ip, updatedAt: time.Now().Unix(), snaps: cp}
	hs.mu.Unlock()
}

// snapsOf returns the latest snapshots for one host (nil when none).
func (hs *hardwareStore) snapsOf(hostID string) []shared.HardwareSnapshot {
	if hs == nil {
		return nil
	}
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	e, ok := hs.byID[hostID]
	if !ok {
		return nil
	}
	out := make([]shared.HardwareSnapshot, len(e.snaps))
	copy(out, e.snaps)
	return out
}

// snapshot returns a copy of every host's latest hardware entry.
func (hs *hardwareStore) snapshot() map[string]hwHostEntry {
	if hs == nil {
		return nil
	}
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	out := make(map[string]hwHostEntry, len(hs.byID))
	for k, v := range hs.byID {
		out[k] = v
	}
	return out
}

// hwLevel maps a Redfish Health/Status string to an alert level ("" = healthy /
// unknown → no alert). Redfish values are case-sensitive; we don't guess.
func hwLevel(s string) string {
	switch s {
	case "Critical":
		return "critical"
	case "Warning":
		return "warning"
	}
	return ""
}

// EvaluateHardware turns the latest hardware snapshots into threshold alerts so
// they flow through the normal notifier path (dedup + fire/resolve + push to
// Feishu/DingTalk/SMS/…) exactly like CPU/disk alerts.
//
// Scope is unique per sub-component (target/temp/<sensor>, target/fan/<name>, …)
// because alertKey = HostID/Type/Scope — sharing a scope would make sibling
// sensors overwrite each other.
func EvaluateHardware(hs *hardwareStore) []Alert {
	entries := hs.snapshot()
	if len(entries) == 0 {
		return nil
	}
	now := time.Now().Unix()
	var alerts []Alert

	for hostID, e := range entries {
		for _, snap := range e.snaps {
			target := snap.TargetName
			if target == "" {
				target = snap.TargetURL
			}
			add := func(level, scope, msg string, val float64) {
				alerts = append(alerts, Alert{
					HostID: hostID, Hostname: e.hostname, IP: e.ip,
					Level: level, Type: "hardware", Scope: scope,
					Message: msg, Value: val, Timestamp: now,
				})
			}

			// A failed poll means "BMC unreachable", NOT "hardware broken" — report it
			// as a collection warning instead of a phantom hardware fault, and don't
			// evaluate the (zeroed) fields below.
			if snap.Error != "" {
				add("warning", target+"/collect", Tz("alert.hw_collect_failed", target, snap.Error), 0)
				continue
			}

			// Overall chassis health
			if lv := hwLevel(snap.Health); lv != "" {
				add(lv, target, Tz("alert.hw_health", target, snap.Health), 0)
			}

			// Temperature sensors — prefer the BMC's own per-sensor thresholds.
			for _, t := range snap.Temps {
				lv := ""
				switch {
				case t.UpperCritical > 0 && t.Reading >= t.UpperCritical:
					lv = "critical"
				case t.UpperCaution > 0 && t.Reading >= t.UpperCaution:
					lv = "warning"
				default:
					lv = hwLevel(t.Status)
				}
				if lv != "" {
					add(lv, target+"/temp/"+t.Name,
						Tz("alert.hw_temp", target, t.Name, t.Reading), t.Reading)
				}
			}

			// Fans — a stopped fan (0 RPM) on an otherwise-present fan is critical.
			for _, f := range snap.Fans {
				lv := hwLevel(f.Health)
				if lv == "" {
					lv = hwLevel(f.Status)
				}
				if lv == "" && f.RPM == 0 && f.Health == "OK" {
					lv = "critical" // reports healthy but not spinning
				}
				if lv != "" {
					add(lv, target+"/fan/"+f.Name,
						Tz("alert.hw_fan", target, f.Name, f.RPM), float64(f.RPM))
				}
			}

			// Power supplies
			for _, p := range snap.Power.PSUs {
				if lv := hwLevel(p.Health); lv != "" {
					add(lv, target+"/psu/"+p.Name,
						Tz("alert.hw_psu", target, p.Name, p.Health), p.InputWatts)
				}
			}

			// Storage / SMART
			for _, d := range snap.Storage {
				lv := hwLevel(d.Health)
				if lv == "" {
					lv = hwLevel(d.Status)
				}
				if d.SMARTWarn {
					lv = "critical" // predicted failure — always escalate
				}
				if lv != "" {
					health := d.Health
					if d.SMARTWarn {
						health = "SMART FailurePredicted"
					}
					add(lv, target+"/disk/"+d.Name, Tz("alert.hw_disk", target, d.Name, health), 0)
				}
			}

			// Memory DIMMs
			for _, d := range snap.Memory.DIMMs {
				slot := d.Slot
				if slot == "" {
					slot = d.Name
				}
				if lv := hwLevel(d.Health); lv != "" {
					add(lv, target+"/dimm/"+slot, Tz("alert.hw_dimm", target, slot, d.Health), 0)
				}
			}

			// CPUs
			for _, c := range snap.CPUs {
				if lv := hwLevel(c.Health); lv != "" {
					add(lv, target+"/cpu/"+c.Name, Tz("alert.hw_cpu", target, c.Name, c.Health), 0)
				}
			}

			// GPU / 加速卡（BMC 带外视角，主机宕机也能看到）
			for _, g := range snap.GPUs {
				if lv := hwLevel(g.Health); lv != "" {
					add(lv, target+"/gpu/"+g.Name, Tz("alert.hw_gpu", target, g.Name, g.Health), 0)
				}
			}

			// RAID / HBA 控制器
			for _, rd := range snap.RAID {
				if lv := hwLevel(rd.Health); lv != "" {
					add(lv, target+"/raid/"+rd.Name, Tz("alert.hw_raid", target, rd.Name, rd.Health), 0)
				}
			}

			// 磁盘框（OceanStor 等外置存储）
			for _, e := range snap.Enclosures {
				if lv := hwLevel(e.Health); lv != "" {
					add(lv, target+"/enclosure/"+e.Name,
						Tz("alert.hw_enclosure", target, e.Name, e.Health), e.TemperatureC)
				}
			}
		}
	}
	return alerts
}
