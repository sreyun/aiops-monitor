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
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Steps       []PlaybookStep `json:"steps"`
	CreatedAt   int64          `json:"created_at"`
	UpdatedAt   int64          `json:"updated_at"`
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
}

func newPlaybookManager(cfg *ConfigStore) *playbookManager {
	return &playbookManager{cfg: cfg, nextExecID: 1}
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
		return Playbook{}, fmt.Errorf("剧本名称不能为空")
	}
	for i := range p.Steps {
		p.Steps[i].Name = strings.TrimSpace(p.Steps[i].Name)
		p.Steps[i].Command = strings.TrimSpace(p.Steps[i].Command)
		if p.Steps[i].TimeoutSec < 5 {
			p.Steps[i].TimeoutSec = 30
		}
	}
	if len(p.Steps) == 0 {
		return Playbook{}, fmt.Errorf("至少需要一个步骤")
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
// "system:xxx" = hosts whose platform matches xxx (linux/windows/darwin);
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
			if h.Category == cat || (h.Category == "" && cat == "未分类") {
				result = append(result, h)
			}
		}
	case strings.HasPrefix(target, "system:"):
		sys := strings.ToLower(target[len("system:"):])
		for _, h := range hosts {
			if strings.ToLower(h.Platform) == sys ||
				(sys == "macos" && strings.ToLower(h.Platform) == "darwin") {
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
