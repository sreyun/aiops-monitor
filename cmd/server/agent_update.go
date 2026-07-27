package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	agentUpdateMaxJobs     = 50
	agentUpdateConcurrency = 3
	agentUpdateTimeoutSec  = 180
	agentUpdateMaxRetries  = 1
	// Cooldown after enqueue so success-before-version-ack cannot storm.
	agentUpdateInFlightSec = 1800
)

type agentUpdateHostResult struct {
	HostID   string `json:"host_id"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	FromVer  string `json:"from_version,omitempty"`
	Status   string `json:"status"` // pending|running|success|failed|skipped
	Method   string `json:"method,omitempty"` // module|script|rollback
	Message  string `json:"message,omitempty"`
	Updated  int64  `json:"updated_at,omitempty"`
}

type agentUpdateJob struct {
	ID         string                   `json:"id"`
	CreatedAt  int64                    `json:"created_at"`
	Actor      string                   `json:"actor"`
	TargetVer  string                   `json:"target_version"`
	ServerURL  string                   `json:"server_url"`
	Force      bool                     `json:"force"`
	Rollback   bool                     `json:"rollback"`
	Status     string                   `json:"status"` // queued|running|done
	Hosts      []*agentUpdateHostResult `json:"hosts"`
	FinishedAt int64                    `json:"finished_at,omitempty"`
}

type agentUpdateManager struct {
	mu   sync.Mutex
	jobs map[string]*agentUpdateJob
	// inFlight prevents overlapping updates for the same host (manual + auto).
	inFlight map[string]int64
}

func newAgentUpdateManager() *agentUpdateManager {
	return &agentUpdateManager{
		jobs:     map[string]*agentUpdateJob{},
		inFlight: map[string]int64{},
	}
}

func (m *agentUpdateManager) put(job *agentUpdateJob) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs[job.ID] = job
	if len(m.jobs) > agentUpdateMaxJobs {
		var oldest string
		var oldestTS int64 = 1<<63 - 1
		for id, j := range m.jobs {
			if j.CreatedAt < oldestTS {
				oldestTS = j.CreatedAt
				oldest = id
			}
		}
		if oldest != "" && oldest != job.ID {
			delete(m.jobs, oldest)
		}
	}
}

func cloneAgentUpdateJob(j *agentUpdateJob) *agentUpdateJob {
	if j == nil {
		return nil
	}
	cp := *j
	if j.Hosts != nil {
		cp.Hosts = make([]*agentUpdateHostResult, len(j.Hosts))
		for i, h := range j.Hosts {
			if h == nil {
				continue
			}
			hc := *h
			cp.Hosts[i] = &hc
		}
	}
	return &cp
}

func (m *agentUpdateManager) snapshot(id string) *agentUpdateJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneAgentUpdateJob(m.jobs[id])
}

func (m *agentUpdateManager) list(limit int) []*agentUpdateJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*agentUpdateJob, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, cloneAgentUpdateJob(j))
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt > out[i].CreatedAt {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (m *agentUpdateManager) setJobStatus(job *agentUpdateJob, status string, finished bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job.Status = status
	if finished {
		job.FinishedAt = time.Now().Unix()
	}
}

func (m *agentUpdateManager) setHostResult(hr *agentUpdateHostResult, status, method, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hr.Status = status
	if method != "" {
		hr.Method = method
	}
	hr.Message = message
	hr.Updated = time.Now().Unix()
}

func (m *agentUpdateManager) tryMarkInFlight(hostID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().Unix()
	if ts, ok := m.inFlight[hostID]; ok && now-ts < agentUpdateInFlightSec {
		return false
	}
	m.inFlight[hostID] = now
	return true
}

func (m *agentUpdateManager) clearInFlight(hostID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.inFlight, hostID)
}

func (m *agentUpdateManager) refreshInFlight(hostID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inFlight[hostID] = time.Now().Unix()
}

// ---------- HTTP handlers ----------

func (s *Server) handleAgentDistManifest(w http.ResponseWriter, r *http.Request) {
	arts := s.listAgentDistManifest()
	writeJSON(w, http.StatusOK, map[string]any{
		"version":     appVersion,
		"comparable":  isComparableAgentVer(appVersion),
		"artifacts":   arts,
		"count":       len(arts),
	})
}

type agentUpdateRequest struct {
	HostIDs  []string `json:"host_ids"`
	Category string   `json:"category"`
	All      bool     `json:"all"`
	Force    bool     `json:"force"`
	Rollback bool     `json:"rollback"`
	Confirm  bool     `json:"confirm"`
}

func (s *Server) handleAgentUpdateStart(w http.ResponseWriter, r *http.Request) {
	if s.agentUpdates == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "agent update manager unavailable"})
		return
	}
	var req agentUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "confirm=true required for agent fleet update"})
		return
	}
	hosts := s.resolveAgentUpdateTargets(req)
	if len(hosts) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no matching hosts"})
		return
	}

	var allowed []*Host
	var skipped []*agentUpdateHostResult
	for _, h := range hosts {
		ok, reason := s.remoteGateCheck(h.ID, s.actorName(r), true, s.actorIsAdmin(r) && (r.URL.Query().Get("break_glass") == "1" || r.Header.Get("X-Break-Glass") == "1"))
		if !ok {
			skipped = append(skipped, &agentUpdateHostResult{
				HostID: h.ID, Hostname: h.Hostname, OS: h.OS, Arch: h.Arch,
				FromVer: h.AgentVersion, Status: "skipped", Message: reason,
			})
			continue
		}
		if !s.agentUpdates.tryMarkInFlight(h.ID) {
			skipped = append(skipped, &agentUpdateHostResult{
				HostID: h.ID, Hostname: h.Hostname, OS: h.OS, Arch: h.Arch,
				FromVer: h.AgentVersion, Status: "skipped", Message: "update already in progress (cooldown)",
			})
			continue
		}
		allowed = append(allowed, h)
	}
	if len(allowed) == 0 {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error":   "no hosts allowed (remote gate or in-flight)",
			"code":    "remote_gate_required",
			"skipped": skipped,
		})
		return
	}

	// Prefer each host's reported agent base (relay URL). Fallback for legacy
	// script path when a host has not reported server_url yet. Module updates may
	// proceed with an empty job base (agent uses its configured report URL).
	fallback := strings.TrimRight(s.serverURL(r), "/")
	if fallback == "" {
		fallback = s.agentPublicBaseURL()
	}
	needConcreteBase := false
	for _, h := range allowed {
		if agentReportedDownloadBase(h) == "" && !hostSupportsAgentUpdateModule(h) {
			needConcreteBase = true
			break
		}
	}
	if needConcreteBase && fallback == "" {
		for _, h := range allowed {
			s.agentUpdates.clearInFlight(h.ID)
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot resolve server URL for /dl downloads"})
		return
	}
	job := s.createAgentUpdateJob(allowed, s.actorName(r), fallback, req.Force, req.Rollback, skipped...)
	s.store.AddLog(LogEntry{
		Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("批量更新 Agent：job=%s hosts=%d skipped=%d target=%s rollback=%v",
			job.ID, len(allowed), len(skipped), job.TargetVer, req.Rollback),
	})
	writeJSON(w, http.StatusAccepted, s.agentUpdates.snapshot(job.ID))
}

func (s *Server) handleAgentUpdateJob(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	job := s.agentUpdates.snapshot(id)
	if job == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleAgentUpdateJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"jobs": s.agentUpdates.list(20)})
}

type agentAutoUpdatePolicyRequest struct {
	Enabled          bool     `json:"agent_auto_update"`
	Window           string   `json:"agent_auto_update_window"`
	ExemptCategories []string `json:"agent_auto_update_exempt_categories"`
	ExemptHosts      []string `json:"agent_auto_update_exempt_hosts"`
}

func (s *Server) handleAgentAutoUpdatePolicyGet(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"agent_auto_update":                   cfg.AgentAutoUpdate,
		"agent_auto_update_window":            cfg.AgentAutoUpdateWindow,
		"agent_auto_update_exempt_categories": cfg.AgentAutoUpdateExemptCategories,
		"agent_auto_update_exempt_hosts":      cfg.AgentAutoUpdateExemptHosts,
		"public_url":                          s.cfg.PublicURL(),
		"target_version":                      appVersion,
		"comparable":                          isComparableAgentVer(appVersion),
	})
}

func (s *Server) handleAgentAutoUpdatePolicySet(w http.ResponseWriter, r *http.Request) {
	var req agentAutoUpdatePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	cats := normalizeCSVList(req.ExemptCategories)
	hosts := normalizeCSVList(req.ExemptHosts)
	if err := s.cfg.SetAgentAutoUpdatePolicy(req.Enabled, req.Window, cats, hosts); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{
		Kind: KindOperation, Level: "info", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("Agent 自动更新策略：enabled=%v window=%q exempt_cat=%d exempt_hosts=%d",
			req.Enabled, strings.TrimSpace(req.Window), len(cats), len(hosts)),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func normalizeCSVList(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		for _, p := range strings.Split(s, ",") {
			p = strings.TrimSpace(p)
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) resolveAgentUpdateTargets(req agentUpdateRequest) []*Host {
	all := s.store.ListHosts()
	offlineSec := int64(s.cfg.Thresholds().OfflineAfter.Seconds())
	now := time.Now().Unix()
	var out []*Host
	want := map[string]bool{}
	for _, id := range req.HostIDs {
		want[strings.TrimSpace(id)] = true
	}
	cat := strings.TrimSpace(req.Category)
	for _, h := range all {
		if h == nil {
			continue
		}
		if now-h.LastSeen > offlineSec {
			continue
		}
		if req.All {
			out = append(out, h)
			continue
		}
		if len(want) > 0 && want[h.ID] {
			out = append(out, h)
			continue
		}
		if cat != "" && (h.Category == cat || strings.EqualFold(h.Category, cat)) {
			out = append(out, h)
		}
	}
	return out
}

func (s *Server) createAgentUpdateJob(hosts []*Host, actor, serverURL string, force, rollback bool, preSkipped ...*agentUpdateHostResult) *agentUpdateJob {
	job := &agentUpdateJob{
		ID:        fmt.Sprintf("au-%d", time.Now().UnixNano()),
		CreatedAt: time.Now().Unix(),
		Actor:     actor,
		TargetVer: appVersion,
		ServerURL: serverURL,
		Force:     force,
		Rollback:  rollback,
		Status:    "queued",
	}
	for _, h := range hosts {
		job.Hosts = append(job.Hosts, &agentUpdateHostResult{
			HostID: h.ID, Hostname: h.Hostname, OS: h.OS, Arch: h.Arch,
			FromVer: h.AgentVersion, Status: "pending",
		})
	}
	job.Hosts = append(job.Hosts, preSkipped...)
	s.agentUpdates.put(job)
	go s.runAgentUpdateJob(job)
	return job
}

func (s *Server) runAgentUpdateJob(job *agentUpdateJob) {
	s.agentUpdates.setJobStatus(job, "running", false)
	sem := make(chan struct{}, agentUpdateConcurrency)
	var wg sync.WaitGroup
	// Only execute pending hosts (skipped hosts from gate/in-flight stay skipped).
	for _, hr := range job.Hosts {
		hr := hr
		if hr.Status == "skipped" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			s.executeAgentUpdateHost(job, hr)
			// Release or refresh in-flight based on outcome / version ack.
			s.finishHostInFlight(job, hr)
		}()
	}
	wg.Wait()
	s.agentUpdates.setJobStatus(job, "done", true)
}

func (s *Server) finishHostInFlight(job *agentUpdateJob, hr *agentUpdateHostResult) {
	if hr == nil {
		return
	}
	switch hr.Status {
	case "skipped", "failed":
		s.agentUpdates.clearInFlight(hr.HostID)
	case "success":
		// Keep cooldown until the host actually reports the new version (or TTL).
		s.agentUpdates.refreshInFlight(hr.HostID)
		go s.waitVersionAckOrExpire(hr.HostID, job.TargetVer, job.Rollback)
	default:
		s.agentUpdates.clearInFlight(hr.HostID)
	}
}

func (s *Server) waitVersionAckOrExpire(hostID, target string, rollback bool) {
	if rollback || !isComparableAgentVer(target) {
		// Rollback / uncomparable: keep short cooldown only.
		time.Sleep(2 * time.Minute)
		s.agentUpdates.clearInFlight(hostID)
		return
	}
	deadline := time.Now().Add(time.Duration(agentUpdateInFlightSec) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Second)
		h := s.hostByID(hostID)
		if h == nil {
			continue
		}
		if !agentVersionBehind(h.AgentVersion, target) {
			s.agentUpdates.clearInFlight(hostID)
			return
		}
	}
	// TTL handled by tryMarkInFlight; leave stamp in place.
}

func (s *Server) executeAgentUpdateHost(job *agentUpdateJob, hr *agentUpdateHostResult) {
	s.agentUpdates.setHostResult(hr, "running", "", "")
	h := s.hostByID(hr.HostID)
	if h == nil {
		s.agentUpdates.setHostResult(hr, "failed", "", "host not found")
		return
	}
	goos, goarch := hostGOOSArch(h)
	distName, distOK := s.agentDistResolve(goos, goarch)
	if !job.Rollback && !distOK {
		s.agentUpdates.setHostResult(hr, "skipped", "", fmt.Sprintf("server dist missing binary for %s/%s (need aiops-agent.exe or aiops-agent-windows-amd64.exe under dist/)", goos, goarch))
		return
	}
	_ = distName
	if !job.Force && !job.Rollback && !agentVersionBehind(h.AgentVersion, job.TargetVer) {
		s.agentUpdates.setHostResult(hr, "skipped", "", "already up to date")
		return
	}

	var lastErr error
	var lastKind execKind
	var out string
	method := ""
	for attempt := 0; attempt <= agentUpdateMaxRetries; attempt++ {
		// Module path: only pass the agent-reported base (relay LAN URL). Never inject
		// cloud PublicURL — agents behind a gateway cannot reach it, and validateUpdateServerURL
		// would reject it against allowedBases. Empty server → agent uses configured report base.
		moduleBase := agentReportedDownloadBase(h)
		scriptBase := agentDownloadBase(h, job.ServerURL)
		if hostSupportsAgentUpdateModule(h) {
			args := map[string]string{
				"force": bool01(job.Force || job.Rollback),
			}
			if moduleBase != "" {
				args["server"] = moduleBase
			}
			if job.TargetVer != "" {
				args["version"] = job.TargetVer
			}
			// Tell agent which /dl name exists on this server (Windows has two aliases).
			if distOK && distName != "" {
				args["bin"] = distName
			}
			method = "module"
			if job.Rollback {
				args["rollback"] = "1"
				method = "rollback"
			}
			s.agentUpdates.setHostResult(hr, "running", method, "")
			out, lastKind, lastErr = s.runAgentModuleKind(h.ID, "agent_update", args, agentUpdateTimeoutSec)
			if lastErr != nil && !job.Rollback && shouldLegacyAgentUpdateFallback(out, lastErr) {
				if scriptBase == "" {
					lastErr = fmt.Errorf("legacy update needs a download base (host server_url or public_url)")
					lastKind = execExit
				} else {
					method = "script"
					s.agentUpdates.setHostResult(hr, "running", method, "")
					out, lastKind, lastErr = s.runLegacyAgentUpdateScriptKind(h, scriptBase, job.Force)
				}
			}
		} else {
			if job.Rollback {
				s.agentUpdates.setHostResult(hr, "skipped", "", "rollback requires agent_update module")
				return
			}
			if scriptBase == "" {
				s.agentUpdates.setHostResult(hr, "skipped", "", "no download base (host server_url or public_url)")
				return
			}
			method = "script"
			s.agentUpdates.setHostResult(hr, "running", method, "")
			out, lastKind, lastErr = s.runLegacyAgentUpdateScriptKind(h, scriptBase, job.Force)
		}
		if lastErr == nil {
			s.agentUpdates.setHostResult(hr, "success", method, truncateRun(out, 500))
			return
		}
		if attempt < agentUpdateMaxRetries && lastKind.retryable() {
			time.Sleep(3 * time.Second)
			continue
		}
		break
	}
	msg := ""
	if lastErr != nil {
		msg = lastErr.Error()
	}
	if out != "" {
		msg = truncateRun(msg+": "+out, 500)
	}
	s.agentUpdates.setHostResult(hr, "failed", method, msg)
}

// shouldLegacyAgentUpdateFallback switches to the install-style script when the
// agent does not understand the module envelope. Do not fall back on explicit
// agent_update failures (checksum, etc.).
func shouldLegacyAgentUpdateFallback(out string, err error) bool {
	combined := out
	if err != nil {
		combined += " " + err.Error()
	}
	if strings.Contains(combined, "未知模块") {
		return true
	}
	if strings.Contains(out, "agent_update:") {
		return false
	}
	low := strings.ToLower(combined)
	return strings.Contains(low, "not found") ||
		strings.Contains(low, "不是内部或外部命令") ||
		strings.Contains(low, "command not found") ||
		strings.Contains(low, "__aiops_module__")
}

func bool01(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// agentReportedDownloadBase is the URL the agent itself uses (relay or direct).
// Empty means the agent_update module should use its configured report base.
func agentReportedDownloadBase(h *Host) string {
	if h == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(h.ServerURL), "/")
}

// agentDownloadBase prefers the agent-reported base; falls back to job/dashboard
// URL for legacy script updates that must embed a concrete /dl URL.
func agentDownloadBase(h *Host, fallback string) string {
	if u := agentReportedDownloadBase(h); u != "" {
		return u
	}
	return strings.TrimRight(strings.TrimSpace(fallback), "/")
}

func (s *Server) runLegacyAgentUpdateScript(h *Host, serverURL string, force bool) (string, error) {
	out, kind, err := s.runLegacyAgentUpdateScriptKind(h, serverURL, force)
	if err != nil {
		return out, err
	}
	if kind != execOK {
		return out, fmt.Errorf("legacy update script failed")
	}
	return out, nil
}

func (s *Server) runLegacyAgentUpdateScriptKind(h *Host, serverURL string, force bool) (string, execKind, error) {
	goos, goarch := hostGOOSArch(h)
	bin, ok := s.agentDistResolve(goos, goarch)
	if !ok {
		if name, err := agentDistBinaryName(goos, goarch); err == nil {
			bin = name
		} else {
			return "", execExit, err
		}
	}
	cmd := buildLegacyAgentUpdateCommand(goos, serverURL, bin, force)
	if cmd == "" {
		return "", execExit, fmt.Errorf("no legacy update script for %s", goos)
	}
	out, kind, err := s.execCommandOnHost(h, cmd, agentUpdateTimeoutSec)
	if err != nil {
		return out, kind, err
	}
	if kind != execOK {
		return out, kind, fmt.Errorf("legacy update script failed")
	}
	return out, kind, nil
}

// maybeAutoUpdateHost enqueues a single-host update when policy is enabled.
func (s *Server) maybeAutoUpdateHost(hostID string) {
	if s == nil || s.cfg == nil || s.agentUpdates == nil || hostID == "" {
		return
	}
	h, ok := s.store.GetHost(hostID)
	if !ok || h == nil {
		return
	}
	cfg := s.cfg.Get()
	if !cfg.AgentAutoUpdate {
		return
	}
	if !agentAutoUpdateWindowOpen(cfg.AgentAutoUpdateWindow) {
		return
	}
	for _, id := range cfg.AgentAutoUpdateExemptHosts {
		if id == h.ID {
			return
		}
	}
	for _, cat := range cfg.AgentAutoUpdateExemptCategories {
		if cat != "" && (h.Category == cat || strings.EqualFold(h.Category, cat)) {
			return
		}
	}
	if !agentVersionBehind(h.AgentVersion, appVersion) {
		// Version caught up — release any leftover cooldown early.
		s.agentUpdates.clearInFlight(h.ID)
		return
	}
	goos, goarch := hostGOOSArch(h)
	if !s.agentDistHas(goos, goarch) {
		return
	}
	// Module-capable hosts can update with an empty job base (agent uses report URL).
	// Legacy script hosts still need a concrete download base.
	base := agentReportedDownloadBase(h)
	if base == "" && !hostSupportsAgentUpdateModule(h) {
		base = s.agentPublicBaseURL()
		if base == "" {
			return
		}
	}
	// Freeze-only for auto path: highRisk=false so default remote gate does not
	// require an approved change outside freeze windows (manual push stays high-risk).
	if ok, _ := s.remoteGateCheck(h.ID, "auto-update", false, false); !ok {
		return
	}
	if !s.agentUpdates.tryMarkInFlight(h.ID) {
		return
	}
	job := s.createAgentUpdateJob([]*Host{h}, "auto-update", base, false, false)
	if s.store != nil {
		s.store.AddLog(LogEntry{
			Kind: KindOperation, Level: "info", Actor: "auto-update", Host: h.ID,
			Message: fmt.Sprintf("自动入队 Agent 更新：job=%s from=%s target=%s", job.ID, h.AgentVersion, job.TargetVer),
		})
	}
}

// agentAutoUpdateWindowOpen parses "HH:MM-HH:MM" local time; empty = always open.
func agentAutoUpdateWindowOpen(window string) bool {
	window = strings.TrimSpace(window)
	if window == "" {
		return true
	}
	parts := strings.Split(window, "-")
	if len(parts) != 2 {
		return true
	}
	parse := func(s string) (int, bool) {
		s = strings.TrimSpace(s)
		var h, m int
		if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil {
			return 0, false
		}
		if h < 0 || h > 23 || m < 0 || m > 59 {
			return 0, false
		}
		return h*60 + m, true
	}
	start, ok1 := parse(parts[0])
	end, ok2 := parse(parts[1])
	if !ok1 || !ok2 {
		return true
	}
	now := time.Now()
	cur := now.Hour()*60 + now.Minute()
	if start <= end {
		return cur >= start && cur <= end
	}
	return cur >= start || cur <= end
}
