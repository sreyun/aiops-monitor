package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aiops-monitor/shared"
)

// ============================================================================
// SRE workflow layer — wiring + HTTP handlers for incidents, closed-loop
// auto-remediation, SLOs and work orders.
// ============================================================================

// wireSRE connects the SRE managers to the rest of the server (playbook
// execution, host lookup, metric/check history, incident timeline, the alert
// engine hook). Called once from NewServer.
func (s *Server) wireSRE() {
	// Auto-remediation needs to look up playbooks/hosts and actually run commands.
	s.remediation.getPlaybook = s.playbooks.Get
	s.remediation.resolveHost = s.hostByID
	s.remediation.category = s.effectiveCategory
	s.remediation.trigger = s.triggerPlaybookOnHost
	s.remediation.onIncident = s.incidents.AddEvent

	// Notification center: every raised / recovered incident becomes a message
	// with a deep-link into the SRE hub. New CRITICAL incidents also trigger an
	// automatic AI/heuristic diagnosis (broadening AI coverage) whose result is
	// appended to the incident timeline and surfaced as its own message.
	s.incidents.onChange = func(inc Incident, isNew bool) {
		ref := strconv.FormatInt(inc.ID, 10)
		if isNew {
			s.messages.push("incident", inc.Severity, "新事件："+inc.Title, incidentMsgBody(inc), "sre", ref)
			if inc.Severity == "critical" {
				go s.autoDiagnose(inc)
			}
		} else {
			s.messages.push("incident", "success", "事件已恢复："+inc.Title, "", "sre", ref)
		}
	}
	// Auto-remediation transitions (awaiting approval / success / failure) → message
	// center, so operators are alerted to pending approvals and outcomes out-of-band.
	s.remediation.onNotify = func(level, title, body string, incidentID int64) {
		ref := ""
		if incidentID > 0 {
			ref = strconv.FormatInt(incidentID, 10)
		}
		s.messages.push("remediation", level, title, body, "sre", ref)
	}
	// AI inspection: only surface a message when the round actually found risks,
	// so the scheduled healthy inspections don't spam the inbox.
	s.ai.onReport = func(rep InspectionReport) {
		crit, warn := 0, 0
		for _, f := range rep.Findings {
			if f.Severity == "critical" {
				crit++
			} else if f.Severity == "warning" {
				warn++
			}
		}
		if crit+warn == 0 {
			return
		}
		lvl := "warning"
		if crit > 0 {
			lvl = "critical"
		}
		s.messages.push("ai", lvl, fmt.Sprintf("AI 巡检发现 %d 项风险", crit+warn), trimLine(rep.Summary, 200), "sre", "")
	}

	// SLO evaluation needs metric + check history and can raise incidents.
	s.slos.incidents = s.incidents
	s.slos.metricSamples = func(hostID string, fromTs int64) []shared.Sample {
		now := time.Now().Unix()
		// Long SLO windows exceed the in-memory tiers, so read from VM (the
		// authoritative time-series store) when it's enabled.
		if s.vm.enabled() {
			if samples, ok := s.vm.queryHistory(hostID, fromTs, now); ok {
				return samples
			}
		}
		samples, _ := s.store.GetHistory(hostID, fromTs, now)
		return samples
	}
	s.slos.checkPoints = s.checks.HistoryOf

	// The alert engine drives incidents + remediation on every fire/recover.
	s.notifier.incidents = s.incidents
	s.notifier.remediation = s.remediation

	// AI inspection reasons over a live snapshot; diagnosis over incident context.
	s.ai.snapshot = func() inspectionContext {
		ic := inspectionContext{}
		th := s.cfg.Thresholds()
		offlineSec := int64(th.OfflineAfter.Seconds())
		now := time.Now().Unix()
		for _, h := range s.store.ListHosts() {
			if now-h.LastSeen > offlineSec {
				ic.OfflineHosts = append(ic.OfflineHosts, h.Hostname)
				continue
			}
			ic.OnlineHosts++
			if h.Latest != nil {
				if h.Latest.CPUPercent >= th.CPUCrit {
					ic.HighUsage = append(ic.HighUsage, fmt.Sprintf("%s CPU %.0f%%", h.Hostname, h.Latest.CPUPercent))
				}
				if h.Latest.MemPercent >= th.MemCrit {
					ic.HighUsage = append(ic.HighUsage, fmt.Sprintf("%s 内存 %.0f%%", h.Hostname, h.Latest.MemPercent))
				}
				if h.Latest.DiskPercent >= th.DiskCrit {
					ic.HighUsage = append(ic.HighUsage, fmt.Sprintf("%s 磁盘 %.0f%%", h.Hostname, h.Latest.DiskPercent))
				}
			}
		}
		ic.FiringAlerts = s.notifier.ActiveAlerts()
		for _, st := range s.slos.Evaluate() {
			if st.Enabled && st.Breaching {
				ic.BreachingSLOs = append(ic.BreachingSLOs, st)
			}
		}
		ic.RecentErrors = s.logs.recentErrors(now-1800, 30)
		ic.ErrorCount = s.logs.errorCount(now - 1800)
		ic.WarnCount = len(s.logs.search("", "warn", "", now-1800, 500))
		return ic
	}
	s.ai.diagContext = func(inc Incident) string {
		var b strings.Builder
		fmt.Fprintf(&b, "事件 #%d：%s（级别 %s，状态 %s，来源 %s）\n", inc.ID, inc.Title, inc.Severity, inc.Status, inc.Source)
		if inc.Hostname != "" {
			b.WriteString("主机：" + inc.Hostname + "\n")
		}
		if h := s.hostByID(inc.HostID); h != nil && h.Latest != nil {
			m := h.Latest
			fmt.Fprintf(&b, "当前指标：CPU %.1f%% · 内存 %.1f%% · 磁盘 %.1f%% · Load %.2f · 进程 %d\n",
				m.CPUPercent, m.MemPercent, m.DiskPercent, m.Load1, m.ProcCount)
		}
		logSince := time.Now().Unix() - 3600
		if inc.HostID != "" {
			errs := s.logs.search(inc.HostID, "error", "", logSince, 12)
			warns := s.logs.search(inc.HostID, "warn", "", logSince, 8)
			if len(errs) > 0 {
				fmt.Fprintf(&b, "近 1 小时该主机错误日志（%d 条节选）：\n", len(errs))
				for _, e := range errs {
					b.WriteString("  - " + trimLine(e.Message, 200) + "\n")
				}
			}
			if len(warns) > 0 {
				b.WriteString("近 1 小时该主机告警(warn)日志（节选）：\n")
				for _, e := range warns {
					b.WriteString("  - " + trimLine(e.Message, 160) + "\n")
				}
			}
			if len(errs) == 0 && len(warns) == 0 {
				b.WriteString("近 1 小时该主机无 error/warn 日志。\n")
			}
		} else {
			// 集群级事件（无特定主机）：附上跨主机近期错误日志，辅助根因关联。
			errs := s.logs.recentErrors(logSince, 12)
			if len(errs) > 0 {
				b.WriteString("近 1 小时集群错误日志（跨主机节选）：\n")
				for _, e := range errs {
					fmt.Fprintf(&b, "  - [%s] %s\n", e.Hostname, trimLine(e.Message, 180))
				}
			}
		}
		return b.String()
	}
}

func (s *Server) hostByID(id string) *Host {
	for _, h := range s.store.ListHosts() {
		if h.ID == id {
			return h
		}
	}
	return nil
}

// incidentMsgBody renders a compact one-line body for an incident notification.
func incidentMsgBody(inc Incident) string {
	b := "级别 " + inc.Severity + " · 来源 " + inc.Source
	if inc.Hostname != "" {
		b += " · 主机 " + inc.Hostname
	}
	return b
}

// autoDiagnose runs an AI (or heuristic) diagnosis for a freshly-raised critical
// incident, appends the result to its timeline, and surfaces it as a message —
// so serious incidents arrive pre-analysed without any operator action. This
// broadens AI coverage from on-demand to automatic. Best-effort: a panic in the
// provider path never affects the caller (runs in its own goroutine).
func (s *Server) autoDiagnose(inc Incident) {
	defer func() { _ = recover() }()
	out, kind := s.ai.Diagnose(inc)
	if out == "" {
		return
	}
	s.incidents.AddEvent(inc.ID, "note", "ai-"+kind, out)
	s.messages.push("ai", "info", "AI 诊断 · "+inc.Title, trimLine(out, 220), "sre", strconv.FormatInt(inc.ID, 10))
	s.store.MarkDirty()
}

func (s *Server) effectiveCategory(hostID string) string {
	if ov, ok := s.cfg.CategoryOverride(hostID); ok {
		return ov
	}
	if h := s.hostByID(hostID); h != nil {
		return h.Category
	}
	return ""
}

// actorName returns the acting operator's username, falling back to their IP.
func (s *Server) actorName(r *http.Request) string {
	if u, ok := s.currentUser(r); ok && u.Username != "" {
		return u.Username
	}
	return s.clientIP(r)
}

// triggerPlaybookOnHost runs a playbook against a single host asynchronously and
// reports success/failure via onDone. Returns the execution ID immediately.
func (s *Server) triggerPlaybookOnHost(pb Playbook, host *Host, operator string, onDone func(ok bool)) int64 {
	hosts := []*Host{host}
	exec := s.playbooks.StartExecution(pb, operator, hosts)
	go func() {
		s.runPlaybookExecution(pb, exec, hosts)
		ok := false
		if e, found := s.playbooks.GetExecution(exec.ID); found {
			ok = e.Status == "completed"
		}
		if onDone != nil {
			onDone(ok)
		}
	}()
	return exec.ID
}

// runSLOEvaluator periodically evaluates SLO error budgets and raises/resolves
// burn incidents.
func (s *Server) runSLOEvaluator(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		s.slos.EvaluateAndAlert()
	}
}

func sreParseID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil
}

// ----------------------------------------------------------------------------
// Incidents
// ----------------------------------------------------------------------------

func (s *Server) handleListIncidents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.incidents.List())
}

func (s *Server) handleGetIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := sreParseID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	inc, found := s.incidents.Get(id)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "incident.not_found")})
		return
	}
	writeJSON(w, http.StatusOK, inc)
}

func (s *Server) handleCreateIncident(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title    string `json:"title"`
		Severity string `json:"severity"`
		HostID   string `json:"host_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "incident.title_required")})
		return
	}
	hostname := ""
	if h := s.hostByID(in.HostID); h != nil {
		hostname = h.Hostname
	}
	inc := s.incidents.CreateManual(in.Title, in.Severity, in.HostID, hostname, s.actorName(r))
	s.store.MarkDirty()
	writeJSON(w, http.StatusOK, inc)
}

func (s *Server) handleAckIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := sreParseID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	inc, found := s.incidents.Ack(id, s.actorName(r))
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "incident.not_found")})
		return
	}
	s.store.MarkDirty()
	writeJSON(w, http.StatusOK, inc)
}

func (s *Server) handleResolveIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := sreParseID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	inc, found := s.incidents.Resolve(id, s.actorName(r))
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "incident.not_found")})
		return
	}
	s.store.MarkDirty()
	writeJSON(w, http.StatusOK, inc)
}

func (s *Server) handleCommentIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := sreParseID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "incident.comment_required")})
		return
	}
	inc, found := s.incidents.Comment(id, s.actorName(r), in.Text)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "incident.not_found")})
		return
	}
	s.store.MarkDirty()
	writeJSON(w, http.StatusOK, inc)
}

// handleEscalateIncident spins a work order off an incident and links them.
func (s *Server) handleEscalateIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := sreParseID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	inc, found := s.incidents.Get(id)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "incident.not_found")})
		return
	}
	prio := "p2"
	if inc.Severity == "critical" {
		prio = "p1"
	}
	tk, err := s.tickets.Create(Ticket{
		Title: inc.Title, Priority: prio, IncidentID: inc.ID,
		Description: Tz("ticket.from_incident", inc.ID),
	}, s.actorName(r))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.incidents.SetTicket(inc.ID, tk.ID, s.actorName(r))
	s.store.MarkDirty()
	writeJSON(w, http.StatusOK, tk)
}

// ----------------------------------------------------------------------------
// Remediation rules + runs
// ----------------------------------------------------------------------------

func (s *Server) handleListRemediationRules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.RemediationRules())
}

func (s *Server) handleUpsertRemediationRule(w http.ResponseWriter, r *http.Request) {
	var rule RemediationRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if err := validateRemediationRule(&rule); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	saved, err := s.cfg.UpsertRemediationRule(rule)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.actorName(r), Message: Tz("log.remediation_saved", saved.Name)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID})
}

func (s *Server) handleDeleteRemediationRule(w http.ResponseWriter, r *http.Request) {
	_ = s.cfg.DeleteRemediationRule(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleListRemediationRuns(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.remediation.Runs())
}

func (s *Server) handleApproveRemediation(w http.ResponseWriter, r *http.Request) {
	id, ok := sreParseID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	if err := s.remediation.Approve(id, s.actorName(r)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRejectRemediation(w http.ResponseWriter, r *http.Request) {
	id, ok := sreParseID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	if err := s.remediation.Reject(id, s.actorName(r)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----------------------------------------------------------------------------
// SLOs
// ----------------------------------------------------------------------------

func (s *Server) handleListSLOs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.slos.Evaluate())
}

func (s *Server) handleUpsertSLO(w http.ResponseWriter, r *http.Request) {
	var slo SLO
	if err := json.NewDecoder(r.Body).Decode(&slo); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if err := validateSLO(&slo); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	saved, err := s.cfg.UpsertSLO(slo)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.actorName(r), Message: Tz("log.slo_saved", saved.Name)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID})
}

func (s *Server) handleDeleteSLO(w http.ResponseWriter, r *http.Request) {
	_ = s.cfg.DeleteSLO(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----------------------------------------------------------------------------
// Tickets (work orders)
// ----------------------------------------------------------------------------

func (s *Server) handleListTickets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.tickets.List())
}

func (s *Server) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	id, ok := sreParseID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	tk, found := s.tickets.Get(id)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "ticket.not_found")})
		return
	}
	writeJSON(w, http.StatusOK, tk)
}

func (s *Server) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	var in Ticket
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	tk, err := s.tickets.Create(in, s.actorName(r))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.MarkDirty()
	s.messages.push("ticket", "info", "新工单："+tk.Title,
		fmt.Sprintf("优先级 %s · 状态 %s", tk.Priority, tk.Status), "sre", strconv.FormatInt(tk.ID, 10))
	writeJSON(w, http.StatusOK, tk)
}

func (s *Server) handleUpdateTicket(w http.ResponseWriter, r *http.Request) {
	id, ok := sreParseID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	var in Ticket
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	tk, err := s.tickets.Update(id, in, s.actorName(r))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.MarkDirty()
	// Only message on the meaningful terminal transitions (resolved/closed) to
	// keep the inbox low-noise on routine edits.
	if tk.Status == "resolved" || tk.Status == "closed" {
		label := "已解决"
		if tk.Status == "closed" {
			label = "已关闭"
		}
		s.messages.push("ticket", "success", "工单"+label+"："+tk.Title, "", "sre", strconv.FormatInt(tk.ID, 10))
	}
	writeJSON(w, http.StatusOK, tk)
}

func (s *Server) handleCommentTicket(w http.ResponseWriter, r *http.Request) {
	id, ok := sreParseID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	tk, err := s.tickets.Comment(id, s.actorName(r), in.Text)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.MarkDirty()
	writeJSON(w, http.StatusOK, tk)
}

func (s *Server) handleDeleteTicket(w http.ResponseWriter, r *http.Request) {
	id, ok := sreParseID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	s.tickets.Delete(id)
	s.store.MarkDirty()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----------------------------------------------------------------------------
// Logs
// ----------------------------------------------------------------------------

// handleAgentLogs ingests a batch of agent logs (fingerprint-authenticated).
func (s *Server) handleAgentLogs(w http.ResponseWriter, r *http.Request) {
	var batch shared.LogBatch
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if !s.forwardFingerprintOKByHost(batch.HostID, agentFP(r)) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	hostname := shortID(batch.HostID)
	if h := s.hostByID(batch.HostID); h != nil {
		hostname = h.Hostname
	}
	s.logs.ingest(batch.HostID, hostname, batch.Lines)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleSearchLogs returns matching aggregated logs (host/level/keyword/time).
func (s *Server) handleSearchLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var since int64
	if m := q.Get("since_min"); m != "" {
		if v, _ := strconv.Atoi(m); v > 0 {
			since = time.Now().Unix() - int64(v)*60
		}
	}
	limit := 500
	if l := q.Get("limit"); l != "" {
		if v, _ := strconv.Atoi(l); v > 0 {
			limit = v
		}
	}
	writeJSON(w, http.StatusOK, s.logs.search(q.Get("host"), q.Get("level"), q.Get("q"), since, limit))
}

// ----------------------------------------------------------------------------
// AI: config + inspection + diagnosis
// ----------------------------------------------------------------------------

func (s *Server) handleGetAIConfig(w http.ResponseWriter, r *http.Request) {
	c := s.cfg.AIConfig()
	if c.APIKey != "" {
		c.APIKey = "****" // never echo the key back to the browser
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleSetAIConfig(w http.ResponseWriter, r *http.Request) {
	var c AIConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if err := s.cfg.SetAIConfig(c); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.actorName(r), Message: Tz("ai.config_saved")})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleListInspections(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.ai.Reports())
}

func (s *Server) handleRunInspection(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.ai.RunInspection("manual"))
}

// handleDiagnoseIncident runs an AI (or heuristic) diagnosis and appends it to
// the incident timeline.
func (s *Server) handleDiagnoseIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := sreParseID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	inc, found := s.incidents.Get(id)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "incident.not_found")})
		return
	}
	diag, source := s.ai.Diagnose(inc)
	actor := "启发式"
	if source == "ai" {
		actor = "AI"
	}
	s.incidents.AddEvent(id, "ai_diagnosis", actor, diag)
	s.store.MarkDirty()
	writeJSON(w, http.StatusOK, map[string]string{"diagnosis": diag, "source": source})
}

// handleSREOverview returns badge counts for the navigation.
func (s *Server) handleSREOverview(w http.ResponseWriter, r *http.Request) {
	breaching := 0
	for _, st := range s.slos.Evaluate() {
		if st.Enabled && st.Breaching {
			breaching++
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{
		"open_incidents":       s.incidents.OpenCount(),
		"pending_remediations": s.remediation.PendingCount(),
		"open_tickets":         s.tickets.OpenCount(),
		"slo_breaching":        breaching,
	})
}
