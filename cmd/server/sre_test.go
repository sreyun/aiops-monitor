package main

import (
	"testing"

	"aiops-monitor/shared"
)

// --- Incident dedup + lifecycle ---

func TestIncidentDedupAndResolve(t *testing.T) {
	im := newIncidentManager()
	a := Alert{Type: "cpu", Level: "critical", HostID: "h1", Hostname: "web", Message: "CPU high"}
	key := "h1/cpu/"
	inc1 := im.OnAlertTransition(a, key, true)
	inc2 := im.OnAlertTransition(a, key, true) // same key must reuse
	if inc1 == 0 || inc2 == 0 || inc1 != inc2 {
		t.Fatalf("firing twice on the same key must reuse one incident")
	}
	if im.OpenCount() != 1 {
		t.Fatalf("expected 1 open incident, got %d", im.OpenCount())
	}
	if _, ok := im.Ack(inc1, "alice"); !ok {
		t.Fatal("ack failed")
	}
	im.OnAlertTransition(a, key, false) // recover
	if im.OpenCount() != 0 {
		t.Fatalf("recover must resolve the incident, open=%d", im.OpenCount())
	}
	got, _ := im.Get(inc1)
	if got.Status != "resolved" {
		t.Fatalf("status should be resolved, got %q", got.Status)
	}
}

// --- SLO math ---

func TestSLOBudget(t *testing.T) {
	cases := []struct {
		sli, target, budget, burn float64
	}{
		{100, 99, 100, 0},   // no bad → full budget
		{99.5, 99, 50, 0.5}, // half the allowance consumed
		{99, 99, 0, 1},      // exactly at target → budget gone
		{98, 99, 0, 2},      // over budget → clamped, burn 2x
		{100, 100, 100, 0},  // 100% target, perfect
	}
	for _, c := range cases {
		b, br := sloBudget(c.sli, c.target)
		if b != c.budget || br != c.burn {
			t.Errorf("sloBudget(%.2f,%.2f)=%.2f,%.2f; want %.2f,%.2f", c.sli, c.target, b, br, c.budget, c.burn)
		}
	}
}

func TestSLOComputeMetric(t *testing.T) {
	m := newSLOManager(nil)
	m.metricSamples = func(hostID string, fromTs int64) []shared.Sample {
		return []shared.Sample{
			{Timestamp: 100, Metrics: shared.Metrics{CPUPercent: 50}}, // good (<90)
			{Timestamp: 101, Metrics: shared.Metrics{CPUPercent: 95}}, // bad
			{Timestamp: 102, Metrics: shared.Metrics{CPUPercent: 80}}, // good
			{Timestamp: 103, Metrics: shared.Metrics{CPUPercent: 88}}, // good
		}
	}
	slo := SLO{SourceType: "metric", HostID: "h1", Metric: "cpu_percent", Comparator: "<", Threshold: 90, Target: 99, WindowDays: 30}
	st := m.computeStatus(slo, 200)
	if st.TotalEvents != 4 || st.GoodEvents != 3 {
		t.Fatalf("expected 3/4 good, got %d/%d", st.GoodEvents, st.TotalEvents)
	}
	if st.SLI != 75 {
		t.Fatalf("expected SLI 75, got %.2f", st.SLI)
	}
	if !st.Breaching {
		t.Fatalf("SLI 75 < target 99 must be breaching")
	}
}

func TestSLOComputeCheck(t *testing.T) {
	m := newSLOManager(nil)
	m.checkPoints = func(checkID string) []CheckPoint {
		return []CheckPoint{
			{Ts: 100, OK: true}, {Ts: 101, OK: false}, {Ts: 102, OK: true}, {Ts: 50, OK: false},
		}
	}
	slo := SLO{SourceType: "check", CheckID: "c1", Target: 99, WindowDays: 30}
	st := m.computeStatus(slo, 120) // window from = 120 - 30*86400, so all points included
	if st.TotalEvents != 4 || st.GoodEvents != 2 {
		t.Fatalf("expected 2/4 good, got %d/%d", st.GoodEvents, st.TotalEvents)
	}
}

// --- Remediation matching + guards ---

func TestRemediationMatch(t *testing.T) {
	m := newRemediationManager(nil)
	rule := RemediationRule{Enabled: true, PlaybookID: "pb1", MatchTypes: []string{"cpu"}, MinLevel: "warning"}
	if !m.matches(rule, Alert{Type: "cpu", Level: "critical"}) {
		t.Error("cpu/critical should match")
	}
	if m.matches(rule, Alert{Type: "memory", Level: "critical"}) {
		t.Error("memory should not match a cpu-only rule")
	}
	crit := rule
	crit.MinLevel = "critical"
	if crit.matchesLevelTest("warning") {
		t.Error("warning must not meet a critical min-level")
	}
	if !m.matches(crit, Alert{Type: "cpu", Level: "critical"}) {
		t.Error("cpu/critical should meet a critical min-level")
	}
	disabled := rule
	disabled.Enabled = false
	if m.matches(disabled, Alert{Type: "cpu", Level: "critical"}) {
		t.Error("disabled rule must never match")
	}
}

// helper for the test above (keeps intent explicit).
func (r RemediationRule) matchesLevelTest(level string) bool {
	return r.MinLevel == "" || levelRank(level) >= levelRank(r.MinLevel)
}

func TestRemediationCooldownAndApproval(t *testing.T) {
	m := newRemediationManager(nil)
	launched := 0
	m.getPlaybook = func(id string) (Playbook, bool) { return Playbook{ID: "pb1", Name: "restart"}, true }
	m.resolveHost = func(id string) *Host { return &Host{ID: "h1", Hostname: "web"} }
	m.trigger = func(pb Playbook, host *Host, op string, onDone func(ok bool)) int64 {
		launched++
		onDone(true)
		return int64(launched)
	}
	a := Alert{Type: "cpu", Level: "critical", HostID: "h1", Hostname: "web"}

	// Auto-run with a cooldown: first fires, second is suppressed.
	rule := RemediationRule{ID: "r1", Enabled: true, PlaybookID: "pb1", CooldownSec: 60}
	m.evaluateRule(rule, a, 0)
	m.evaluateRule(rule, a, 0)
	if launched != 1 {
		t.Fatalf("cooldown must suppress the 2nd run, launched=%d", launched)
	}
	sawCooldown := false
	for _, run := range m.Runs() {
		if run.Status == "skipped_cooldown" {
			sawCooldown = true
		}
	}
	if !sawCooldown {
		t.Fatal("expected a skipped_cooldown run record")
	}

	// Approval gate: queued, then approved → runs.
	rule2 := RemediationRule{ID: "r2", Enabled: true, PlaybookID: "pb1", RequireApproval: true}
	m.evaluateRule(rule2, a, 0)
	var pendingID int64
	for _, run := range m.Runs() {
		if run.Status == "pending_approval" {
			pendingID = run.ID
		}
	}
	if pendingID == 0 {
		t.Fatal("expected a pending_approval run")
	}
	before := launched
	if err := m.Approve(pendingID, "alice"); err != nil {
		t.Fatalf("approve failed: %v", err)
	}
	if launched != before+1 {
		t.Fatalf("approval must launch the playbook, launched %d→%d", before, launched)
	}
}

func TestRemediationRateLimit(t *testing.T) {
	m := newRemediationManager(nil)
	launched := 0
	m.getPlaybook = func(id string) (Playbook, bool) { return Playbook{ID: "pb1", Name: "x"}, true }
	m.resolveHost = func(id string) *Host { return &Host{ID: "h1", Hostname: "web"} }
	m.trigger = func(pb Playbook, host *Host, op string, onDone func(ok bool)) int64 { launched++; onDone(true); return 1 }
	// No cooldown, but max 2 per hour. Different hosts so cooldown-per-host never blocks.
	rule := RemediationRule{ID: "r1", Enabled: true, PlaybookID: "pb1", MaxPerHour: 2}
	for i := 0; i < 5; i++ {
		a := Alert{Type: "cpu", Level: "critical", HostID: "h" + string(rune('0'+i)), Hostname: "n"}
		m.evaluateRule(rule, a, 0)
	}
	if launched != 2 {
		t.Fatalf("rate limit should cap at 2, launched=%d", launched)
	}
}
