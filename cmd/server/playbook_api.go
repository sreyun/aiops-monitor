package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// -----------------------------------------------------------------------
// Playbook (automation) handlers
// -----------------------------------------------------------------------

func (s *Server) handleListPlaybooks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.playbooks.List())
}

func (s *Server) handleUpsertPlaybook(w http.ResponseWriter, r *http.Request) {
	var p Playbook
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	saved, err := s.playbooks.Upsert(p)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: Tz("log.save_playbook", saved.Name)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID})
}

func (s *Server) handleDeletePlaybook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = s.playbooks.Delete(id)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.delete_playbook", id)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleExecutePlaybook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pb, ok := s.playbooks.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "playbook.not_found")})
		return
	}
	targetList := s.onlinePlaybookTargets(pb)
	if len(targetList) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "playbook.no_target")})
		return
	}
	exec := s.playbooks.StartExecution(pb, s.clientIP(r), targetList)
	// Run each step on each host sequentially via the agent reverse terminal channel
	go s.runPlaybookExecution(pb, exec, targetList)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.execute_playbook", pb.Name, len(targetList))})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "execution_id": exec.ID})
}

// onlinePlaybookTargets resolves the unique set of ONLINE target hosts across all
// of a playbook's steps. Offline hosts have no reachable agent, so including them
// would always fail — they are filtered out up front.
func (s *Server) onlinePlaybookTargets(pb Playbook) []*Host {
	offlineSec := int64(s.cfg.Thresholds().OfflineAfter.Seconds())
	nowUnix := time.Now().Unix()
	hosts := make([]*Host, 0)
	for _, h := range s.store.ListHosts() {
		if nowUnix-h.LastSeen <= offlineSec {
			hosts = append(hosts, h)
		}
	}
	targetSet := map[string]*Host{}
	for _, step := range pb.Steps {
		for _, h := range s.playbooks.ResolveTargets(step.Target, hosts) {
			targetSet[h.ID] = h
		}
	}
	targetList := make([]*Host, 0, len(targetSet))
	for _, h := range targetSet {
		targetList = append(targetList, h)
	}
	return targetList
}

// runScheduler is the timed-trigger loop: every tick it fires any playbooks whose
// schedule is due. It runs for the life of the process.
func (s *Server) runScheduler(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for now := range t.C {
		for _, pb := range s.playbooks.dueSchedules(now) {
			s.fireScheduledPlaybook(pb)
		}
	}
}

// fireScheduledPlaybook runs one scheduled execution, clearing the in-flight guard
// when it finishes so the next occurrence can fire.
func (s *Server) fireScheduledPlaybook(pb Playbook) {
	hosts := s.onlinePlaybookTargets(pb)
	if len(hosts) == 0 {
		s.playbooks.clearSchedBusy(pb.ID)
		s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: "scheduler", Message: Tz("log.sched_no_target", pb.Name)})
		return
	}
	exec := s.playbooks.StartExecution(pb, Tz("playbook.scheduler_actor"), hosts)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: "scheduler", Message: Tz("log.sched_fire", pb.Name, len(hosts))})
	go func() {
		s.runPlaybookExecution(pb, exec, hosts)
		s.playbooks.clearSchedBusy(pb.ID)
	}()
}

const (
	// execPickupTimeout bounds how long a summoned agent has to attach before we
	// declare a no-pickup. Covers the agent's ≤25s long-poll cycle plus network margin.
	execPickupTimeout = 40 * time.Second
	// playbookMaxAttempts is the total number of tries per step per host: 1 initial
	// + retries. Only infrastructure-class failures (no-pickup/timeout/abnormal) are
	// retried; a genuine non-zero command exit is never retried.
	playbookMaxAttempts = 3
	// playbookRetryBackoff is multiplied by the attempt number for a linear backoff
	// between retries, giving a briefly-unreachable agent time to recover.
	playbookRetryBackoff = 2 * time.Second
	// playbookMaxParallel caps how many hosts run concurrently so a large fleet
	// doesn't get summoned in one thundering herd.
	playbookMaxParallel = 30
)

// runPlaybookExecution runs playbook steps on all target hosts in parallel
// (bounded by playbookMaxParallel). Each host gets a one-shot terminal session
// per step; infrastructure-class failures are retried automatically.
func (s *Server) runPlaybookExecution(pb Playbook, exec *PlaybookExecution, hosts []*Host) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, playbookMaxParallel) // bound concurrent hosts (anti thundering-herd)
	for _, h := range hosts {
		wg.Add(1)
		go func(h *Host) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			result := HostExecResult{Hostname: h.Hostname, Status: "running"}
			for _, step := range pb.Steps {
				sr := StepResult{Name: step.Name, Status: "running"}
				start := time.Now()
				// Retry infrastructure-class failures (agent didn't pick up,
				// timeout, abnormal end) — the usual cause of "some nodes fail"
				// in large batches. A genuine non-zero command exit is NOT retried.
				var output string
				var kind execKind
				var err error
				for attempt := 1; attempt <= playbookMaxAttempts; attempt++ {
					output, kind, err = s.execCommandOnHost(h, step.Command, step.TimeoutSec)
					if err == nil {
						if attempt > 1 {
							output += "\n" + Tz("playbook.retry_recovered", attempt)
						}
						break
					}
					if !kind.retryable() {
						break // real command failure — retrying is pointless
					}
					if attempt < playbookMaxAttempts {
						time.Sleep(time.Duration(attempt) * playbookRetryBackoff)
						continue
					}
					output += "\n" + Tz("playbook.attempts_failed", attempt)
				}
				sr.Duration = time.Since(start).Milliseconds()
				if err != nil {
					sr.Status = "failed"
					sr.Output = output + "\n[error] " + err.Error()
					result.Status = "failed"
					result.Output += sr.Output + "\n"
					result.Steps = append(result.Steps, sr)
					if !step.ContinueErr {
						break
					}
				} else {
					sr.Status = "success"
					sr.Output = output
					result.Output += output + "\n"
					result.Steps = append(result.Steps, sr)
				}
			}
			if result.Status != "failed" {
				result.Status = "success"
			}
			s.playbooks.UpdateHostResult(exec.ID, h.ID, result)
		}(h)
	}
	wg.Wait()
	// Determine overall status
	allSuccess := true
	for _, r := range exec.HostResults {
		if r.Status != "success" {
			allSuccess = false
			break
		}
	}
	status := "completed"
	if !allSuccess {
		status = "failed"
	}
	s.playbooks.FinishExecution(exec.ID, status)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: exec.Operator, Message: Tz("log.playbook_done", pb.Name, status)})
}

// execKind classifies a single command run so the batch runner can decide
// whether a failure is worth retrying. A non-zero exit code is a genuine command
// failure (retrying is pointless); a timeout / no-pickup / abnormal end is an
// infrastructure hiccup (a retry often recovers it — the root cause of the
// "some nodes fail" complaint in large batches).
type execKind int

const (
	execOK       execKind = iota // ran, exit 0
	execExit                     // ran, non-zero exit — NOT retryable
	execTimeout                  // timed out with partial output — retryable
	execNoPickup                 // timed out with NO output: agent never picked up — retryable
	execAbnormal                 // session ended without an exit marker — retryable
)

// retryable reports whether a failure of this kind is an infrastructure issue
// worth re-attempting (as opposed to a real non-zero command exit).
func (k execKind) retryable() bool {
	return k == execTimeout || k == execNoPickup || k == execAbnormal
}

// execCommandOnHost runs a single command on a host via the Agent reverse terminal
// channel. It creates a one-shot exec session, summons the agent, and streams the
// combined output until the process exits (tx EOF → session done) or the timer
// fires. The outcome is classified via parseExecOutput.
func (s *Server) execCommandOnHost(h *Host, command string, timeoutSec int) (string, execKind, error) {
	if timeoutSec < 5 {
		timeoutSec = 30
	}
	sess := s.term.createExec(h.ID, h.Hostname, command)
	defer s.term.remove(sess.id)
	defer sess.close()
	// Summon the agent. notifyAgent now queues the session if the agent is
	// between polls (no active waiter), so it always succeeds immediately.
	// The agent will pick it up on its next long-poll cycle (up to 25s).
	s.term.notifyAgent(h.ID, sess.id)

	// Phase 1 — wait for the agent to actually attach (markAgentUp fires when the
	// agent opens its tx stream). If it never attaches within the pickup window,
	// this is a pure "agent didn't pick up" miss: return fast (execNoPickup) so the
	// batch runner can retry quickly, instead of blocking for the whole command
	// timeout. execPickupTimeout covers the agent's ≤25s long-poll cycle + margin.
	select {
	case <-sess.agentUp:
		// attached — proceed to stream output
	case <-time.After(execPickupTimeout):
		return "", execNoPickup, fmt.Errorf("%s", Tz("playbook.no_pickup"))
	case <-sess.done:
		return "", execAbnormal, fmt.Errorf("%s", Tz("playbook.abnormal"))
	}

	// Phase 2 — the agent runs the command as a ONE-SHOT process (sh -c / cmd /c,
	// no PTY) and streams combined output up the tx channel, ending it when the
	// process exits (tx EOF → session done). Because the agent has already
	// attached, the timer is the command's real budget (no poll-latency buffer
	// needed). The exit code arrives as a trailing "[AIOPS_EXIT]<code>" marker.
	var output []byte
	timer := time.NewTimer(time.Duration(timeoutSec) * time.Second)
	defer timer.Stop()
	for {
		select {
		case b := <-sess.toBrowser:
			output = append(output, b...)
			if len(output) > 512*1024 {
				output = output[len(output)-512*1024:]
			}
		case <-timer.C:
			out, kind, err := parseExecOutput(output, true)
			return out, kind, err
		case <-sess.done:
			draining := true
			for draining {
				select {
				case b := <-sess.toBrowser:
					output = append(output, b...)
				default:
					draining = false
				}
			}
			out, kind, err := parseExecOutput(output, false)
			return out, kind, err
		}
	}
}

// parseExecOutput splits the agent's exec result into clean output text and an
// error derived from the trailing "[AIOPS_EXIT]<code>" marker.
func parseExecOutput(output []byte, timedOut bool) (string, execKind, error) {
	s := string(output)
	if idx := strings.LastIndex(s, "[AIOPS_EXIT]"); idx >= 0 {
		code := 0
		fmt.Sscanf(strings.TrimSpace(s[idx+len("[AIOPS_EXIT]"):]), "%d", &code)
		body := strings.TrimRight(s[:idx], "\r\n")
		if code != 0 {
			return body, execExit, fmt.Errorf("%s", Tz("playbook.exit_code", code))
		}
		return body, execOK, nil
	}
	body := strings.TrimRight(s, "\r\n")
	if timedOut {
		// No exit marker + timed out. Empty output means the agent never picked
		// up the summoned session (a pure infrastructure miss, highly retryable);
		// partial output means the command was running but exceeded the timeout.
		if strings.TrimSpace(body) == "" {
			return body, execNoPickup, fmt.Errorf("%s", Tz("playbook.no_pickup"))
		}
		return body, execTimeout, fmt.Errorf("%s", Tz("playbook.timeout"))
	}
	return body, execAbnormal, fmt.Errorf("%s", Tz("playbook.abnormal"))
}

func (s *Server) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.playbooks.ExecutionHistory())
}

func (s *Server) handleGetExecution(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	exec, ok := s.playbooks.GetExecution(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "playbook.exec_not_found")})
		return
	}
	writeJSON(w, http.StatusOK, exec)
}
