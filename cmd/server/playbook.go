package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Playbook is an operator-defined automation: a sequence of shell commands run
// on a set of target hosts via the Agent reverse-terminal channel.
type Playbook struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Steps       []PlaybookStep    `json:"steps"`
	Schedule    *PlaybookSchedule `json:"schedule,omitempty"` // optional timed trigger
	CreatedAt   int64             `json:"created_at"`
	UpdatedAt   int64             `json:"updated_at"`
}

// PlaybookSchedule defines an optional timed trigger for a playbook. A minimal,
// dependency-free model covering the common cases (no full cron parser):
//   - kind="interval": run every IntervalMin minutes
//   - kind="daily":    run every day at At ("HH:MM", server local time)
//   - kind="weekly":   run every week on Weekday (0=Sun..6=Sat) at At
type PlaybookSchedule struct {
	Enabled     bool   `json:"enabled"`
	Kind        string `json:"kind"`                   // interval | daily | weekly
	IntervalMin int    `json:"interval_min,omitempty"` // kind=interval
	At          string `json:"at,omitempty"`           // "HH:MM" for daily/weekly
	Weekday     int    `json:"weekday,omitempty"`      // 0=Sun..6=Sat for weekly
}

// PlaybookStep is one command in a playbook. Target selectors:
// "all" = every online host; "category:xxx" = hosts in category xxx;
// "host:ID" = a single host by ID.
type PlaybookStep struct {
	Name        string `json:"name"`
	Command     string `json:"command"`
	Target      string `json:"target"` // "all" | "category:xxx" | "host:ID"
	TimeoutSec  int    `json:"timeout_sec"`
	ContinueErr bool   `json:"continue_on_error"`
}

// PlaybookExecution is one run of a playbook: tracks per-host status + output.
type PlaybookExecution struct {
	ID         int64                    `json:"id"`
	PlaybookID string                   `json:"playbook_id"`
	PlaybookName string                 `json:"playbook_name"`
	Operator   string                   `json:"operator"`
	StartTime  int64                    `json:"start_time"`
	EndTime    int64                    `json:"end_time,omitempty"`
	Status     string                   `json:"status"` // running | completed | failed | cancelled
	HostResults map[string]HostExecResult `json:"host_results"`
}

// HostExecResult tracks one host's execution outcome.
type HostExecResult struct {
	Hostname string `json:"hostname"`
	Status   string `json:"status"` // pending | running | success | failed | timeout
	Output   string `json:"output"`
	Steps    []StepResult `json:"steps"`
}

// StepResult is one step's outcome on one host.
type StepResult struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Output   string `json:"output"`
	Duration int64  `json:"duration_ms"`
}

// playbookManager stores playbooks and execution history in memory + config.
type playbookManager struct {
	mu         sync.Mutex
	cfg        *ConfigStore
	executions []PlaybookExecution
	nextExecID int64
	// --- scheduler bookkeeping (in-memory; resets on restart) ---
	lastCheck time.Time            // last scheduler tick, for daily/weekly windowing
	lastRun   map[string]time.Time // playbook ID -> last scheduled fire (interval baseline + dedup)
	schedBusy map[string]bool      // playbook ID -> a scheduled run is currently in flight
}

func newPlaybookManager(cfg *ConfigStore) *playbookManager {
	return &playbookManager{
		cfg: cfg, nextExecID: 1,
		lastRun:   map[string]time.Time{},
		schedBusy: map[string]bool{},
	}
}

// parseHHMM parses "HH:MM" (24h) into minutes-of-day; ok=false if malformed.
func parseHHMM(s string) (int, bool) {
	var h, m int
	if n, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &h, &m); err != nil || n != 2 {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// scheduledInstant returns today's date at the given "HH:MM" in now's location.
func scheduledInstant(now time.Time, hhmm string) (time.Time, bool) {
	mins, ok := parseHHMM(hhmm)
	if !ok {
		return time.Time{}, false
	}
	return time.Date(now.Year(), now.Month(), now.Day(), mins/60, mins%60, 0, 0, now.Location()), true
}

// sanitizeSchedule validates a schedule in place. A disabled schedule is accepted
// as-is; an enabled one must be well-formed for its kind.
func sanitizeSchedule(sc *PlaybookSchedule) error {
	if sc == nil || !sc.Enabled {
		return nil
	}
	switch sc.Kind {
	case "interval":
		if sc.IntervalMin < 1 {
			return fmt.Errorf("%s", Tz("playbook.sched_bad_interval"))
		}
	case "daily":
		if _, ok := parseHHMM(sc.At); !ok {
			return fmt.Errorf("%s", Tz("playbook.sched_bad_time"))
		}
	case "weekly":
		if sc.Weekday < 0 || sc.Weekday > 6 {
			return fmt.Errorf("%s", Tz("playbook.sched_bad_weekday"))
		}
		if _, ok := parseHHMM(sc.At); !ok {
			return fmt.Errorf("%s", Tz("playbook.sched_bad_time"))
		}
	default:
		return fmt.Errorf("%s", Tz("playbook.sched_bad_kind"))
	}
	return nil
}

// dueSchedules returns the playbooks whose schedule is due to fire at `now`,
// updating internal bookkeeping so each occurrence fires exactly once. Playbooks
// with a scheduled run already in flight are skipped to avoid pileup. The caller
// must clearSchedBusy(id) when each returned playbook's run finishes.
func (pm *playbookManager) dueSchedules(now time.Time) []Playbook {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	prevCheck := pm.lastCheck
	if prevCheck.IsZero() {
		prevCheck = now // first tick establishes a baseline; never fire retroactively
	}
	var due []Playbook
	for _, pb := range pm.cfg.Playbooks() {
		sc := pb.Schedule
		if sc == nil || !sc.Enabled || pm.schedBusy[pb.ID] {
			continue
		}
		fire := false
		switch sc.Kind {
		case "interval":
			if sc.IntervalMin >= 1 {
				last, seen := pm.lastRun[pb.ID]
				if !seen {
					pm.lastRun[pb.ID] = now // baseline; first fire one interval later
				} else if now.Sub(last) >= time.Duration(sc.IntervalMin)*time.Minute {
					fire = true
				}
			}
		case "daily":
			if inst, ok := scheduledInstant(now, sc.At); ok && inst.After(prevCheck) && !inst.After(now) {
				fire = true
			}
		case "weekly":
			if int(now.Weekday()) == sc.Weekday {
				if inst, ok := scheduledInstant(now, sc.At); ok && inst.After(prevCheck) && !inst.After(now) {
					fire = true
				}
			}
		}
		if fire {
			pm.lastRun[pb.ID] = now
			pm.schedBusy[pb.ID] = true
			due = append(due, pb)
		}
	}
	pm.lastCheck = now
	return due
}

// clearSchedBusy releases the in-flight guard for a playbook's scheduled run.
func (pm *playbookManager) clearSchedBusy(id string) {
	pm.mu.Lock()
	delete(pm.schedBusy, id)
	pm.mu.Unlock()
}

// List returns all playbooks from config.
func (pm *playbookManager) List() []Playbook {
	return pm.cfg.Playbooks()
}

// Get returns a playbook by ID.
func (pm *playbookManager) Get(id string) (Playbook, bool) {
	for _, p := range pm.cfg.Playbooks() {
		if p.ID == id {
			return p, true
		}
	}
	return Playbook{}, false
}

// Upsert creates or updates a playbook.
func (pm *playbookManager) Upsert(p Playbook) (Playbook, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return Playbook{}, fmt.Errorf("%s", Tz("playbook.name_required"))
	}
	for i := range p.Steps {
		p.Steps[i].Name = strings.TrimSpace(p.Steps[i].Name)
		p.Steps[i].Command = strings.TrimSpace(p.Steps[i].Command)
		if p.Steps[i].TimeoutSec < 5 {
			p.Steps[i].TimeoutSec = 30
		}
	}
	if len(p.Steps) == 0 {
		return Playbook{}, fmt.Errorf("%s", Tz("playbook.step_required"))
	}
	if err := sanitizeSchedule(p.Schedule); err != nil {
		return Playbook{}, err
	}
	now := time.Now().Unix()
	p.UpdatedAt = now
	if p.ID == "" {
		p.ID = genToken()[:8]
		p.CreatedAt = now
	}
	return pm.cfg.UpsertPlaybook(p)
}

// Delete removes a playbook by ID.
func (pm *playbookManager) Delete(id string) error {
	return pm.cfg.DeletePlaybook(id)
}

// ResolveTargets expands a target selector into a list of host IDs.
// Supported prefixes: "all" = every host; "category:xxx" = hosts in category xxx;
// "system:xxx" = hosts whose OS matches xxx (linux/macos/windows — macOS hosts
// have h.OS="darwin", mapped via the macos→darwin check below);
// "host:ID" = a single host by ID.
func (pm *playbookManager) ResolveTargets(target string, hosts []*Host) []*Host {
	target = strings.TrimSpace(target)
	var result []*Host
	switch {
	case target == "" || target == "all":
		for _, h := range hosts {
			result = append(result, h)
		}
	case strings.HasPrefix(target, "category:"):
		cat := target[len("category:"):]
		for _, h := range hosts {
			// Use the EFFECTIVE category: an operator-set override wins over the
			// agent-self-reported category, exactly as the host list display does.
			// Otherwise a host's playbook membership would be driven by whatever
			// category its (untrusted) agent chose to report.
			effective := h.Category
			if pm.cfg != nil {
				if ov, ok := pm.cfg.CategoryOverride(h.ID); ok {
					effective = ov
				}
			}
			if effective == cat || (effective == "" && cat == Tz("playbook.uncategorized")) {
				result = append(result, h)
			}
		}
	case strings.HasPrefix(target, "system:"):
		sys := strings.ToLower(target[len("system:"):])
		for _, h := range hosts {
			// Match by h.OS (runtime.GOOS: "linux"/"windows"/"darwin"), NOT
			// h.Platform (which is a version string like "Ubuntu 22.04").
			if strings.ToLower(h.OS) == sys ||
				(sys == "macos" && strings.ToLower(h.OS) == "darwin") {
				result = append(result, h)
			}
		}
	case strings.HasPrefix(target, "host:"):
		hid := target[len("host:"):]
		for _, h := range hosts {
			if h.ID == hid {
				result = append(result, h)
			}
		}
	}
	return result
}

// ExecutionHistory returns recent playbook executions.
func (pm *playbookManager) ExecutionHistory() []PlaybookExecution {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	out := make([]PlaybookExecution, len(pm.executions))
	copy(out, pm.executions)
	// reverse: newest first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// StartExecution creates a new execution record and returns it.
func (pm *playbookManager) StartExecution(pb Playbook, operator string, hosts []*Host) *PlaybookExecution {
	pm.mu.Lock()
	pm.nextExecID++
	exec := PlaybookExecution{
		ID:          pm.nextExecID,
		PlaybookID:  pb.ID,
		PlaybookName: pb.Name,
		Operator:    operator,
		StartTime:   time.Now().Unix(),
		Status:      "running",
		HostResults: map[string]HostExecResult{},
	}
	for _, h := range hosts {
		exec.HostResults[h.ID] = HostExecResult{
			Hostname: h.Hostname,
			Status:   "pending",
		}
	}
	pm.executions = append(pm.executions, exec)
	// Trim history to last 100 executions
	if len(pm.executions) > 100 {
		pm.executions = pm.executions[len(pm.executions)-100:]
	}
	pm.mu.Unlock()
	return &exec
}

// UpdateHostResult updates one host's result in an execution.
func (pm *playbookManager) UpdateHostResult(execID int64, hostID string, result HostExecResult) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for i := range pm.executions {
		if pm.executions[i].ID == execID {
			pm.executions[i].HostResults[hostID] = result
			return
		}
	}
}

// FinishExecution marks an execution as done.
func (pm *playbookManager) FinishExecution(execID int64, status string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for i := range pm.executions {
		if pm.executions[i].ID == execID {
			pm.executions[i].EndTime = time.Now().Unix()
			pm.executions[i].Status = status
			return
		}
	}
}

// GetExecution returns a specific execution by ID.
func (pm *playbookManager) GetExecution(id int64) (PlaybookExecution, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for _, e := range pm.executions {
		if e.ID == id {
			return e, true
		}
	}
	return PlaybookExecution{}, false
}
