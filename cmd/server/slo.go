package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"aiops-monitor/shared"
)

// ============================================================================
// SLO — Service Level Objectives + error budgets.
//
// An SLO measures an SLI (a "good ratio") over a rolling window and compares it
// to a target. Two SLI sources are supported with zero new dependencies:
//   - "check":  a synthetic check's up ratio (OK points / total points)
//   - "metric": the fraction of samples where a host metric stays in a good
//               band (e.g. cpu_percent < 90)
// The remaining error budget and burn rate are derived from the SLI, and a
// burned-through budget automatically opens an incident.
// ============================================================================

// SLO defines one objective.
type SLO struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Enabled    bool    `json:"enabled"`
	SourceType string  `json:"source_type"`          // check | metric
	CheckID    string  `json:"check_id,omitempty"`   // source_type=check
	HostID     string  `json:"host_id,omitempty"`    // source_type=metric
	Metric     string  `json:"metric,omitempty"`     // cpu_percent | mem_percent | disk_percent | load1 | ...
	Comparator string  `json:"comparator,omitempty"` // < | <= | > | >=  (defines the "good" band)
	Threshold  float64 `json:"threshold,omitempty"`
	Target     float64 `json:"target"`      // objective %, e.g. 99.9
	WindowDays int     `json:"window_days"` // rolling window
	CreatedAt  int64   `json:"created_at"`
	UpdatedAt  int64   `json:"updated_at"`
}

// SLOStatus is an SLO plus its computed health.
type SLOStatus struct {
	SLO
	SLI         float64 `json:"sli"`          // achieved good ratio over the window (%)
	ErrorBudget float64 `json:"error_budget"` // remaining budget (% of the allowance)
	BurnRate    float64 `json:"burn_rate"`    // consumed / allowed (>1 = over budget pace)
	GoodEvents  int64   `json:"good_events"`
	TotalEvents int64   `json:"total_events"`
	Breaching   bool    `json:"breaching"` // SLI < target
	Healthy     bool    `json:"healthy"`
}

// sampleMetric extracts a named metric from a sample; ok=false if unknown.
func sampleMetric(s shared.Sample, name string) (float64, bool) {
	switch name {
	case "cpu_percent":
		return s.CPUPercent, true
	case "mem_percent":
		return s.MemPercent, true
	case "disk_percent":
		return s.DiskPercent, true
	case "load1":
		return s.Load1, true
	case "load5":
		return s.Load5, true
	case "load15":
		return s.Load15, true
	case "diskio_util":
		return s.DiskIOUtilPercent, true
	case "net_recv_rate":
		return s.NetRecvRate, true
	case "net_sent_rate":
		return s.NetSentRate, true
	}
	return 0, false
}

// satisfies reports whether v is in the "good" band defined by comparator+threshold.
func satisfies(v float64, cmp string, thr float64) bool {
	switch cmp {
	case "<":
		return v < thr
	case "<=":
		return v <= thr
	case ">":
		return v > thr
	case ">=":
		return v >= thr
	case "==":
		return v == thr
	}
	return false
}

// sloBudget derives the remaining error budget (%) and burn rate from an SLI and
// target. allowed-bad = 100-target; consumed-bad = 100-sli. A 100% target means
// any bad event exhausts the budget.
func sloBudget(sli, target float64) (budgetRemaining, burnRate float64) {
	allowedBad := 100 - target
	actualBad := 100 - sli
	if allowedBad <= 0 {
		if actualBad <= 0 {
			return 100, 0
		}
		return 0, actualBad // effectively infinite pace; report the magnitude
	}
	budgetRemaining = (allowedBad - actualBad) / allowedBad * 100
	if budgetRemaining < 0 {
		budgetRemaining = 0
	}
	if budgetRemaining > 100 {
		budgetRemaining = 100
	}
	burnRate = actualBad / allowedBad
	return
}

// sloManager evaluates SLOs and raises incidents when a budget is exhausted.
type sloManager struct {
	mu  sync.Mutex
	cfg *ConfigStore
	// data sources (injected during wiring)
	metricSamples func(hostID string, fromTs int64) []shared.Sample
	checkPoints   func(checkID string) []CheckPoint
	incidents     *incidentManager
	burning       map[string]bool // sloID -> currently in a burn incident
}

func newSLOManager(cfg *ConfigStore) *sloManager {
	return &sloManager{cfg: cfg, burning: map[string]bool{}}
}

// computeStatus evaluates one SLO against its window.
func (m *sloManager) computeStatus(s SLO, now int64) SLOStatus {
	win := s.WindowDays
	if win < 1 {
		win = 30
	}
	fromTs := now - int64(win)*86400
	var good, total int64
	switch s.SourceType {
	case "check":
		if m.checkPoints != nil {
			for _, p := range m.checkPoints(s.CheckID) {
				if p.Ts < fromTs {
					continue
				}
				total++
				if p.OK {
					good++
				}
			}
		}
	case "metric":
		if m.metricSamples != nil {
			for _, smp := range m.metricSamples(s.HostID, fromTs) {
				v, ok := sampleMetric(smp, s.Metric)
				if !ok {
					continue
				}
				total++
				if satisfies(v, s.Comparator, s.Threshold) {
					good++
				}
			}
		}
	}
	sli := 100.0
	if total > 0 {
		sli = float64(good) / float64(total) * 100
	}
	budget, burn := sloBudget(sli, s.Target)
	return SLOStatus{
		SLO: s, SLI: sli, ErrorBudget: budget, BurnRate: burn,
		GoodEvents: good, TotalEvents: total,
		Breaching: total > 0 && sli < s.Target,
		Healthy:   total == 0 || sli >= s.Target,
	}
}

// Evaluate returns the current status of every SLO (for the API/UI).
func (m *sloManager) Evaluate() []SLOStatus {
	if m.cfg == nil {
		return nil
	}
	now := time.Now().Unix()
	slos := m.cfg.SLOs()
	out := make([]SLOStatus, 0, len(slos))
	for _, s := range slos {
		out = append(out, m.computeStatus(s, now))
	}
	return out
}

// EvaluateAndAlert is the ticker body: exhausting an SLO's error budget opens an
// incident (deduped per SLO); recovering budget resolves it.
func (m *sloManager) EvaluateAndAlert() {
	if m.cfg == nil || m.incidents == nil {
		return
	}
	now := time.Now().Unix()
	for _, s := range m.cfg.SLOs() {
		if !s.Enabled {
			continue
		}
		st := m.computeStatus(s, now)
		key := "slo/" + s.ID
		exhausted := st.TotalEvents > 0 && st.ErrorBudget <= 0
		m.mu.Lock()
		wasBurning := m.burning[s.ID]
		if exhausted && !wasBurning {
			m.burning[s.ID] = true
			m.mu.Unlock()
			m.incidents.raise(key, Tz("slo.incident_title", s.Name, st.SLI, s.Target),
				"critical", "slo", "", "", "slo")
		} else if !exhausted && wasBurning {
			delete(m.burning, s.ID)
			m.mu.Unlock()
			m.incidents.resolveByKey(key, Tz("slo.incident_recovered", s.Name))
		} else {
			m.mu.Unlock()
		}
	}
}

// validateSLO normalizes and checks an SLO before persisting.
func validateSLO(s *SLO) error {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return fmt.Errorf("%s", Tz("slo.name_required"))
	}
	if s.Target <= 0 || s.Target > 100 {
		return fmt.Errorf("%s", Tz("slo.bad_target"))
	}
	if s.WindowDays < 1 {
		s.WindowDays = 30
	}
	switch s.SourceType {
	case "check":
		if s.CheckID == "" {
			return fmt.Errorf("%s", Tz("slo.check_required"))
		}
	case "metric":
		if s.HostID == "" || s.Metric == "" {
			return fmt.Errorf("%s", Tz("slo.metric_required"))
		}
		if !satisfiesValidComparator(s.Comparator) {
			return fmt.Errorf("%s", Tz("slo.bad_comparator"))
		}
	default:
		return fmt.Errorf("%s", Tz("slo.bad_source"))
	}
	return nil
}

func satisfiesValidComparator(c string) bool {
	switch c {
	case "<", "<=", ">", ">=", "==":
		return true
	}
	return false
}
