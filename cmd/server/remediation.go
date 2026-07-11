package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// Closed-loop auto-remediation — connect the alert engine to playbooks.
//
// When an alert fires the engine matches it against operator-defined rules; a
// matching rule triggers a playbook scoped to the affected host. Guards make it
// safe to run commands automatically: an optional human-approval gate, a
// per-host cooldown, and a per-rule hourly rate limit — so a flapping alert can
// never unleash a storm of remediation runs.
// ============================================================================

// RemediationRule maps a class of alerts to a playbook, with safety guards.
type RemediationRule struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Enabled       bool     `json:"enabled"`
	MatchTypes    []string `json:"match_types,omitempty"`    // alert types (cpu/memory/...); empty = any
	MinLevel      string   `json:"min_level,omitempty"`      // "" any | warning | critical
	MatchCategory string   `json:"match_category,omitempty"` // host category filter; empty = any
	PlaybookID    string   `json:"playbook_id"`
	// Guards
	RequireApproval bool `json:"require_approval"` // queue for operator approval instead of auto-running
	CooldownSec     int  `json:"cooldown_sec"`     // min seconds between runs for the same host
	MaxPerHour      int  `json:"max_per_hour"`     // per-rule hourly cap (0 = unlimited)
	CreatedAt       int64 `json:"created_at"`
	UpdatedAt       int64 `json:"updated_at"`
}

// RemediationRun records one (attempted) remediation.
type RemediationRun struct {
	ID           int64  `json:"id"`
	RuleID       string `json:"rule_id"`
	RuleName     string `json:"rule_name"`
	AlertKey     string `json:"alert_key"`
	AlertType    string `json:"alert_type"`
	HostID       string `json:"host_id"`
	Hostname     string `json:"hostname"`
	PlaybookID   string `json:"playbook_id"`
	PlaybookName string `json:"playbook_name"`
	// pending_approval | running | success | failed | skipped_cooldown | skipped_ratelimit | rejected | no_playbook
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
	ExecutionID int64  `json:"execution_id,omitempty"`
	IncidentID  int64  `json:"incident_id,omitempty"`
	CreatedAt   int64  `json:"created_at"`
	DecidedAt   int64  `json:"decided_at,omitempty"`
	DecidedBy   string `json:"decided_by,omitempty"`
}

const remediationRunCap = 300

func levelRank(l string) int {
	switch l {
	case "critical":
		return 2
	case "warning":
		return 1
	default:
		return 0
	}
}

// remediationManager evaluates rules and tracks runs. Playbook execution is done
// through a server-provided callback so this file stays free of HTTP/agent code.
type remediationManager struct {
	mu      sync.Mutex
	cfg     *ConfigStore
	runs    []RemediationRun
	nextID  int64
	lastRun map[string]int64   // ruleID|hostID -> last run unix (cooldown)
	hourly  map[string][]int64 // ruleID -> recent run unix times (rate limit)

	// Server-provided hooks (set during wiring).
	getPlaybook func(id string) (Playbook, bool)
	resolveHost func(id string) *Host
	category    func(hostID string) string
	// trigger runs the playbook on one host asynchronously, invokes onDone(ok)
	// when it finishes, and returns the playbook execution ID immediately.
	trigger    func(pb Playbook, host *Host, operator string, onDone func(ok bool)) int64
	onIncident func(incidentID int64, kind, actor, text string)
	// onNotify surfaces a remediation transition (awaiting approval / success /
	// failure) to the message center so operators are alerted out-of-band.
	onNotify func(level, title, body string, incidentID int64)
}

func newRemediationManager(cfg *ConfigStore) *remediationManager {
	return &remediationManager{
		cfg: cfg, nextID: 1,
		lastRun: map[string]int64{},
		hourly:  map[string][]int64{},
	}
}

// matches reports whether a rule applies to an alert on a host.
func (m *remediationManager) matches(r RemediationRule, a Alert) bool {
	if !r.Enabled || r.PlaybookID == "" {
		return false
	}
	if r.MinLevel != "" && levelRank(a.Level) < levelRank(r.MinLevel) {
		return false
	}
	if len(r.MatchTypes) > 0 {
		hit := false
		for _, t := range r.MatchTypes {
			if t == a.Type {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if r.MatchCategory != "" {
		cat := ""
		if m.category != nil {
			cat = m.category(a.HostID)
		}
		if cat != r.MatchCategory {
			return false
		}
	}
	return true
}

// OnAlert is the notifier hook for a firing alert: run every matching rule
// through its guards, then execute or queue for approval.
func (m *remediationManager) OnAlert(a Alert, incidentID int64) {
	if m.cfg == nil {
		return
	}
	for _, r := range m.cfg.RemediationRules() {
		if !m.matches(r, a) {
			continue
		}
		m.evaluateRule(r, a, incidentID)
	}
}

func (m *remediationManager) evaluateRule(r RemediationRule, a Alert, incidentID int64) {
	now := time.Now().Unix()
	m.mu.Lock()
	// Cooldown: same rule + host within the window.
	ck := r.ID + "|" + a.HostID
	if r.CooldownSec > 0 && now-m.lastRun[ck] < int64(r.CooldownSec) {
		m.recordLocked(r, a, incidentID, "skipped_cooldown", Tz("remediation.reason_cooldown", r.CooldownSec))
		m.mu.Unlock()
		return
	}
	// Rate limit: prune to last hour, then check the cap.
	if r.MaxPerHour > 0 {
		cut := now - 3600
		times := m.hourly[r.ID][:0]
		for _, t := range m.hourly[r.ID] {
			if t >= cut {
				times = append(times, t)
			}
		}
		m.hourly[r.ID] = times
		if len(times) >= r.MaxPerHour {
			m.recordLocked(r, a, incidentID, "skipped_ratelimit", Tz("remediation.reason_ratelimit", r.MaxPerHour))
			m.mu.Unlock()
			return
		}
	}
	pb, ok := m.getPlaybookSafe(r.PlaybookID)
	pbName := pb.Name
	if !ok {
		m.recordLocked(r, a, incidentID, "no_playbook", Tz("remediation.reason_no_playbook"))
		m.mu.Unlock()
		return
	}
	if r.RequireApproval {
		run := m.recordLocked(r, a, incidentID, "pending_approval", "")
		run.PlaybookName = pbName
		m.setPlaybookNameLocked(run.ID, pbName)
		m.mu.Unlock()
		if m.onIncident != nil && incidentID > 0 {
			m.onIncident(incidentID, "remediation", "auto",
				Tz("remediation.evt_pending", r.Name, pbName))
		}
		if m.onNotify != nil {
			m.onNotify("warning", "自动修复待审批："+r.Name,
				"修复剧本「"+pbName+"」已排队，等待人工审批，请在 SRE · 自动修复 页处理。", incidentID)
		}
		return
	}
	// Auto-run: reserve cooldown/rate-limit slots now to prevent double-fire.
	m.lastRun[ck] = now
	m.hourly[r.ID] = append(m.hourly[r.ID], now)
	run := m.recordLocked(r, a, incidentID, "running", "")
	m.setPlaybookNameLocked(run.ID, pbName)
	runID := run.ID
	m.mu.Unlock()
	m.launch(runID, pb, a.HostID, incidentID, r.Name)
}

// launch executes the playbook for a run (outside the lock).
func (m *remediationManager) launch(runID int64, pb Playbook, hostID string, incidentID int64, ruleName string) {
	host := (*Host)(nil)
	if m.resolveHost != nil {
		host = m.resolveHost(hostID)
	}
	if host == nil || m.trigger == nil {
		m.finish(runID, false, Tz("remediation.reason_host_gone"))
		return
	}
	execID := m.trigger(pb, host, Tz("remediation.actor"), func(ok bool) {
		m.finish(runID, ok, "")
	})
	m.mu.Lock()
	if run := m.findRun(runID); run != nil {
		run.ExecutionID = execID
	}
	m.mu.Unlock()
	if m.onIncident != nil && incidentID > 0 {
		m.onIncident(incidentID, "remediation", "auto",
			Tz("remediation.evt_triggered", ruleName, pb.Name, host.Hostname))
	}
}

// finish updates a run's terminal status once its playbook execution completes.
func (m *remediationManager) finish(runID int64, ok bool, reason string) {
	m.mu.Lock()
	run := m.findRun(runID)
	if run == nil {
		m.mu.Unlock()
		return
	}
	if ok {
		run.Status = "success"
	} else {
		run.Status = "failed"
		run.Reason = reason
	}
	incID := run.IncidentID
	name, host := run.PlaybookName, run.Hostname
	m.mu.Unlock()
	if m.onIncident != nil && incID > 0 {
		key := "remediation.evt_success"
		if !ok {
			key = "remediation.evt_failed"
		}
		m.onIncident(incID, "remediation", "auto", Tz(key, name, host))
	}
	if m.onNotify != nil {
		if ok {
			m.onNotify("success", "自动修复成功："+name, "主机 "+host+" 已成功执行修复剧本。", incID)
		} else {
			m.onNotify("critical", "自动修复失败："+name, "主机 "+host+"："+trimLine(reason, 160), incID)
		}
	}
}

// Approve runs a pending remediation; Reject discards it.
func (m *remediationManager) Approve(runID int64, actor string) error {
	m.mu.Lock()
	run := m.findRun(runID)
	if run == nil {
		m.mu.Unlock()
		return fmt.Errorf("%s", Tz("remediation.run_not_found"))
	}
	if run.Status != "pending_approval" {
		m.mu.Unlock()
		return fmt.Errorf("%s", Tz("remediation.not_pending"))
	}
	pb, ok := m.getPlaybookSafe(run.PlaybookID)
	if !ok {
		run.Status = "no_playbook"
		m.mu.Unlock()
		return fmt.Errorf("%s", Tz("remediation.reason_no_playbook"))
	}
	run.Status = "running"
	run.DecidedAt = time.Now().Unix()
	run.DecidedBy = actor
	// reserve guard slots on approval
	m.lastRun[run.RuleID+"|"+run.HostID] = time.Now().Unix()
	m.hourly[run.RuleID] = append(m.hourly[run.RuleID], time.Now().Unix())
	runID, hostID, incID, ruleName := run.ID, run.HostID, run.IncidentID, run.RuleName
	m.mu.Unlock()
	m.launch(runID, pb, hostID, incID, ruleName)
	return nil
}

func (m *remediationManager) Reject(runID int64, actor string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	run := m.findRun(runID)
	if run == nil {
		return fmt.Errorf("%s", Tz("remediation.run_not_found"))
	}
	if run.Status != "pending_approval" {
		return fmt.Errorf("%s", Tz("remediation.not_pending"))
	}
	run.Status = "rejected"
	run.DecidedAt = time.Now().Unix()
	run.DecidedBy = actor
	if m.onIncident != nil && run.IncidentID > 0 {
		m.onIncident(run.IncidentID, "remediation", actor, Tz("remediation.evt_rejected", run.RuleName))
	}
	return nil
}

// --- internal helpers (caller holds mu unless noted) ---

func (m *remediationManager) findRun(id int64) *RemediationRun {
	for i := range m.runs {
		if m.runs[i].ID == id {
			return &m.runs[i]
		}
	}
	return nil
}

func (m *remediationManager) getPlaybookSafe(id string) (Playbook, bool) {
	if m.getPlaybook == nil {
		return Playbook{}, false
	}
	return m.getPlaybook(id)
}

func (m *remediationManager) setPlaybookNameLocked(runID int64, name string) {
	if run := m.findRun(runID); run != nil {
		run.PlaybookName = name
	}
}

func (m *remediationManager) recordLocked(r RemediationRule, a Alert, incidentID int64, status, reason string) *RemediationRun {
	m.nextID++
	run := RemediationRun{
		ID: m.nextID, RuleID: r.ID, RuleName: r.Name,
		AlertKey: alertKey(a), AlertType: a.Type,
		HostID: a.HostID, Hostname: a.Hostname,
		PlaybookID: r.PlaybookID, Status: status, Reason: reason,
		IncidentID: incidentID, CreatedAt: time.Now().Unix(),
	}
	m.runs = append(m.runs, run)
	if len(m.runs) > remediationRunCap {
		m.runs = m.runs[len(m.runs)-remediationRunCap:]
	}
	return m.findRun(run.ID)
}

// Runs returns run history newest-first.
func (m *remediationManager) Runs() []RemediationRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RemediationRun, len(m.runs))
	copy(out, m.runs)
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// PendingCount returns how many runs await approval (for nav badges).
func (m *remediationManager) PendingCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for i := range m.runs {
		if m.runs[i].Status == "pending_approval" {
			n++
		}
	}
	return n
}

// validateRemediationRule normalizes and checks a rule before persisting.
func validateRemediationRule(r *RemediationRule) error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return fmt.Errorf("%s", Tz("remediation.name_required"))
	}
	if r.PlaybookID == "" {
		return fmt.Errorf("%s", Tz("remediation.playbook_required"))
	}
	if r.MinLevel != "" && r.MinLevel != "warning" && r.MinLevel != "critical" {
		return fmt.Errorf("%s", Tz("remediation.bad_level"))
	}
	if r.CooldownSec < 0 {
		r.CooldownSec = 0
	}
	if r.MaxPerHour < 0 {
		r.MaxPerHour = 0
	}
	return nil
}
