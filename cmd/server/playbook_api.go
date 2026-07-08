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
	// Only online hosts can run commands — an offline host has no agent to reach,
	// so including it would always fail the whole execution. Filter them out.
	offlineSec := int64(s.cfg.Thresholds().OfflineAfter.Seconds())
	nowUnix := time.Now().Unix()
	hosts := make([]*Host, 0)
	for _, h := range s.store.ListHosts() {
		if nowUnix-h.LastSeen <= offlineSec {
			hosts = append(hosts, h)
		}
	}
	// Resolve all unique target hosts across all steps
	targetSet := map[string]*Host{}
	for _, step := range pb.Steps {
		for _, h := range s.playbooks.ResolveTargets(step.Target, hosts) {
			targetSet[h.ID] = h
		}
	}
	if len(targetSet) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "playbook.no_target")})
		return
	}
	targetList := make([]*Host, 0, len(targetSet))
	for _, h := range targetSet {
		targetList = append(targetList, h)
	}
	exec := s.playbooks.StartExecution(pb, s.clientIP(r), targetList)
	// Run each step on each host sequentially via the agent reverse terminal channel
	go s.runPlaybookExecution(pb, exec, targetList)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.execute_playbook", pb.Name, len(targetList))})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "execution_id": exec.ID})
}

// runPlaybookExecution runs playbook steps on all target hosts in parallel.
// Each host gets a one-shot terminal session: send command, capture output, close.
func (s *Server) runPlaybookExecution(pb Playbook, exec *PlaybookExecution, hosts []*Host) {
	var wg sync.WaitGroup
	for _, h := range hosts {
		wg.Add(1)
		go func(h *Host) {
			defer wg.Done()
			result := HostExecResult{Hostname: h.Hostname, Status: "running"}
			for _, step := range pb.Steps {
				sr := StepResult{Name: step.Name, Status: "running"}
				start := time.Now()
				output, err := s.execCommandOnHost(h, step.Command, step.TimeoutSec)
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

// execCommandOnHost runs a single command on a host via the Agent reverse terminal
// channel. It creates a terminal session, sends the command followed by a unique
// sentinel echo, and waits for the sentinel to appear in the output — which means
// the command has finished. The output is then cleaned of ANSI escapes, command
// echoes, and shell prompts before being returned.
func (s *Server) execCommandOnHost(h *Host, command string, timeoutSec int) (string, error) {
	if timeoutSec < 5 {
		timeoutSec = 30
	}
	sess := s.term.createExec(h.ID, h.Hostname, command)
	defer s.term.remove(sess.id)
	defer sess.close()
	// Summon the agent, retrying across the brief gap where it re-registers its
	// long-poll waiter between sessions — the main cause of "part of the hosts fail
	// to connect" on multi-host runs over higher-latency links. Up to ~3s.
	notified := false
	for i := 0; i < 30; i++ {
		if s.term.notifyAgent(h.ID, sess.id) {
			notified = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !notified {
		return "", fmt.Errorf("%s", Tz("playbook.connect_failed", h.Hostname))
	}
	// The agent runs the command as a ONE-SHOT process (sh -c / cmd /c, no PTY) and
	// streams the combined output up the tx channel, ending it when the process
	// exits — so session `done` (tx EOF) means the command finished. This is far
	// more reliable than the old interactive-PTY + sentinel scheme, especially on
	// Linux. The exit code arrives as a trailing "[AIOPS_EXIT]<code>" marker.
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
			return parseExecOutput(output, true)
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
			return parseExecOutput(output, false)
		}
	}
}

// parseExecOutput splits the agent's exec result into clean output text and an
// error derived from the trailing "[AIOPS_EXIT]<code>" marker.
func parseExecOutput(output []byte, timedOut bool) (string, error) {
	s := string(output)
	if idx := strings.LastIndex(s, "[AIOPS_EXIT]"); idx >= 0 {
		code := 0
		fmt.Sscanf(strings.TrimSpace(s[idx+len("[AIOPS_EXIT]"):]), "%d", &code)
		body := strings.TrimRight(s[:idx], "\r\n")
		if code != 0 {
			return body, fmt.Errorf("%s", Tz("playbook.exit_code", code))
		}
		return body, nil
	}
	body := strings.TrimRight(s, "\r\n")
	if timedOut {
		return body, fmt.Errorf("%s", Tz("playbook.timeout"))
	}
	return body, fmt.Errorf("%s", Tz("playbook.abnormal"))
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
