package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
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
			// 事件自动串联：RAG 召回相似历史事件 + 已验证处置 + 匹配的自动修复规则，挂到时间线
			go s.correlateIncident(inc)
			// 新事件存入 AI 记忆库，供跨会话 RAG 检索复用
			go s.rememberAI("alert", fmt.Sprintf("incident:%d", inc.ID),
				fmt.Sprintf("【新告警事件】%s\n严重程度：%s | 类型：%s | 主机：%s | 来源：%s",
					inc.Title, inc.Severity, inc.Type, inc.Hostname, inc.Source))
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
		// AI 巡检报告存入 AI 记忆库，供跨会话 RAG 检索复用
		go s.rememberAI("inspection", fmt.Sprintf("inspection:%d", rep.ID),
			fmt.Sprintf("【AI巡检报告】%s\n发现：%d 项严重 / %d 项警告\n%s",
				rep.Context, crit, warn, rep.Summary))
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
	// API 业务监控接口作为 SLI 源：历史从 VM 回读（重启不丢），OK 率即 SLI。
	s.slos.apiPoints = func(apiID string, fromTs int64) []APIHistPoint {
		if s.vm != nil && s.vm.enabled() {
			return s.vm.queryAPIHistory(apiID, fromTs, time.Now().Unix())
		}
		return nil
	}
	// PromQL 源：把抓取/推送入 VM 的任意指标（JVM/DB/中间件…）作为 SLI，good/total 由 PromQL 现算。
	s.slos.promScalar = func(q string) (float64, bool) {
		if s.vm != nil && s.vm.enabled() {
			return s.vm.vmQueryScalar(q)
		}
		return 0, false
	}
	s.slos.promRange = func(q string, from, to, step int64) ([]vmRangePoint, bool) {
		if s.vm != nil && s.vm.enabled() {
			return s.vm.vmQueryRange(q, from, to, step)
		}
		return nil, false
	}

	// The alert engine drives incidents + remediation on every fire/recover.
	s.notifier.incidents = s.incidents
	s.notifier.remediation = s.remediation

	// Terminal session end → extract output summary and save to AI memory for RAG.
	if s.term != nil {
		s.term.onArchive = func(info termSessionInfo, text string) {
			go s.rememberAI("terminal", info.HostID,
				fmt.Sprintf("【终端会话摘要】主机：%s | 操作者：%s\n%s",
					info.Hostname, info.Operator, text))
		}
	}

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

// annotateHostNames fills hostname/ip from the managed-host store onto list rows
// that only carry host_id (NetFlow / SNMP / content-audit host selectors). Without
// this the UI falls back to raw IDs whenever _cachedHosts hasn't been loaded yet.
func (s *Server) annotateHostNames(rows []map[string]any) {
	for _, row := range rows {
		id, _ := row["host_id"].(string)
		if id == "" {
			continue
		}
		if h := s.hostByID(id); h != nil {
			if h.Hostname != "" {
				row["hostname"] = h.Hostname
			}
			if h.IP != "" {
				row["ip"] = h.IP
			}
		}
	}
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
	// 事件自动诊断结果同样向量化入库，供后续 RAG 相似案例检索（此前仅手动诊断/诊断对话会向量化）。
	go s.saveDiagnosisEmbedding(inc.ID, inc, out)
	// 同时存入通用 AI 记忆库，供跨会话 RAG 检索复用
	go s.rememberAI("diagnosis", fmt.Sprintf("incident:%d", inc.ID), "【事件】"+inc.Title+"\n【自动诊断】"+out)
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

// checkSlowDegradation detects slow resource degradation: if CPU/memory/disk
// show an upward trend over the last 3 samples AND are approaching warning
// thresholds (>85% of threshold), raise a warning incident with AI analysis.
func (s *Server) checkSlowDegradation(hostID string) {
	samples, ok := s.store.GetSamples(hostID)
	if !ok || len(samples) < 3 {
		return
	}
	n := len(samples)
	s1, s2, s3 := samples[n-3], samples[n-2], samples[n-1]

	isTrending := func(v1, v2, v3 float64) bool {
		return v2 > v1 && v3 > v2
	}

	th := s.cfg.Thresholds()
	var issues []string

	if isTrending(s1.CPUPercent, s2.CPUPercent, s3.CPUPercent) && s3.CPUPercent >= th.CPUWarn*0.85 {
		issues = append(issues, fmt.Sprintf("CPU 持续上升 %.1f%%→%.1f%%→%.1f%%（接近阈值%.0f%%）",
			s1.CPUPercent, s2.CPUPercent, s3.CPUPercent, th.CPUWarn))
	}
	if isTrending(s1.MemPercent, s2.MemPercent, s3.MemPercent) && s3.MemPercent >= th.MemWarn*0.85 {
		issues = append(issues, fmt.Sprintf("内存持续上升 %.1f%%→%.1f%%→%.1f%%（接近阈值%.0f%%）",
			s1.MemPercent, s2.MemPercent, s3.MemPercent, th.MemWarn))
	}
	if isTrending(s1.DiskPercent, s2.DiskPercent, s3.DiskPercent) && s3.DiskPercent >= th.DiskWarn*0.85 {
		issues = append(issues, fmt.Sprintf("磁盘持续上升 %.1f%%→%.1f%%→%.1f%%（接近阈值%.0f%%）",
			s1.DiskPercent, s2.DiskPercent, s3.DiskPercent, th.DiskWarn))
	}

	if len(issues) == 0 {
		return
	}

	host := s.hostByID(hostID)
	if host == nil {
		return
	}

	title := fmt.Sprintf("[趋势预警] 主机 %s 资源缓慢恶化", host.Hostname)
	analysis := fmt.Sprintf("检测到以下资源呈持续上升趋势，可能在数小时内达到告警阈值：\n- %s\n建议：检查相关服务是否有内存泄漏、日志膨胀或异常负载增长。",
		strings.Join(issues, "\n- "))

	inc := s.incidents.CreateManual(title, "warning", hostID, host.Hostname, "AI趋势检测")
	if inc.ID > 0 {
		s.incidents.AddEvent(inc.ID, "ai_analysis", "AI", analysis)
		s.store.MarkDirty()
		go s.rememberAI("alert", fmt.Sprintf("degradation:%s", hostID),
			fmt.Sprintf("【趋势预警】%s\n%s", title, analysis))
	}
}

// actorName returns the operator identity for audit logs: the authenticated
// username when available, otherwise the resolved client IP. For callers that
// also need the IP separately, use actorIP directly.
func (s *Server) actorName(r *http.Request) string {
	actor, _ := s.actorIP(r)
	return actor
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
	// 可选解决说明（用于沉淀解决经验；缺省留空，向后兼容旧前端）
	var body struct {
		Note string `json:"note,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	inc, found := s.incidents.Resolve(id, s.actorName(r))
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": Tr(r, "incident.not_found")})
		return
	}
	s.store.MarkDirty()
	// 学习闭环 C：事件解决 → 沉淀「解决经验」记忆 + 强化促成解决的诊断记忆。异步、尽力而为。
	go s.learnFromResolution(inc, strings.TrimSpace(body.Note))
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
	// Append a more descriptive timeline entry with the ticket number
	s.incidents.AddEvent(inc.ID, "escalated", s.actorName(r),
		fmt.Sprintf("已升级为工单 #%d（优先级 %s）", tk.ID, strings.ToUpper(prio)))
	s.store.MarkDirty()
	// Push notification to message center
	s.messages.push("ticket", "info",
		fmt.Sprintf("事件 #%d 已升级为工单 #%d", inc.ID, tk.ID),
		fmt.Sprintf("事件：%s | 优先级：%s | 操作人：%s", inc.Title, strings.ToUpper(prio), s.actorName(r)),
		"sre", strconv.FormatInt(tk.ID, 10))
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
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.actorName(r), IP: s.clientIP(r), Message: Tz("log.remediation_saved", saved.Name)})
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
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.actorName(r), IP: s.clientIP(r), Message: Tz("log.slo_saved", saved.Name)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID})
}

func (s *Server) handleDeleteSLO(w http.ResponseWriter, r *http.Request) {
	_ = s.cfg.DeleteSLO(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleSLOTrend 返回某 SLO 在自定义 [from,to] 区间的状态 + SLI 趋势曲线（分桶现算），
// 使 SLO 在时间维度上与主机趋势图一致（快捷跨度 / 自定义绝对区间）。
// GET /api/v1/slos/{id}/trend?from=&to=（Unix 秒；缺省用该 SLO 的窗口天数回看到现在）。
func (s *Server) handleSLOTrend(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var slo *SLO
	for _, x := range s.cfg.SLOs() {
		if x.ID == id {
			sx := x
			slo = &sx
			break
		}
	}
	if slo == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "slo not found"})
		return
	}
	now := time.Now().Unix()
	win := slo.WindowDays
	if win < 1 {
		win = 30
	}
	from, to := now-int64(win)*86400, now
	if v := r.URL.Query().Get("from"); v != "" {
		if n, _ := strconv.ParseInt(v, 10, 64); n > 0 {
			from = n
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if n, _ := strconv.ParseInt(v, 10, 64); n > 0 {
			to = n
		}
	}
	if to <= from {
		to = from + 3600
	}
	trend := s.slos.sloTrend(*slo, from, to)
	if trend == nil {
		trend = []sloTrendPoint{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": s.slos.computeStatusRange(*slo, from, to),
		"trend":  trend, "from": from, "to": to,
	})
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
	// Enrich with linked incident info for traceability
	result := map[string]any{
		"id":          tk.ID,
		"title":       tk.Title,
		"description": tk.Description,
		"priority":    tk.Priority,
		"status":      tk.Status,
		"assignee":    tk.Assignee,
		"reporter":    tk.Reporter,
		"incident_id": tk.IncidentID,
		"comments":    tk.Comments,
		"created_at":  tk.CreatedAt,
		"updated_at":  tk.UpdatedAt,
	}
	if tk.IncidentID > 0 {
		if inc, found := s.incidents.Get(tk.IncidentID); found {
			result["incident"] = map[string]any{
				"id":         inc.ID,
				"title":      inc.Title,
				"severity":   inc.Severity,
				"status":     inc.Status,
				"hostname":   inc.Hostname,
				"created_at": inc.CreatedAt,
			}
		}
	}
	writeJSON(w, http.StatusOK, result)
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
		// Auto-resolve the linked incident when the ticket is resolved/closed.
		if tk.IncidentID > 0 {
			if inc, found := s.incidents.Get(tk.IncidentID); found && inc.Status != "resolved" {
				s.incidents.Resolve(tk.IncidentID, "工单 #"+strconv.FormatInt(tk.ID, 10)+" 已"+label)
				s.incidents.AddEvent(tk.IncidentID, "note", "system",
					fmt.Sprintf("关联工单 #%d 已%s，事件自动标记为已解决", tk.ID, label))
			}
		}
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
	if r.Header.Get("X-Log-Enc") != "" {
		// 加密上报：按上报指纹重新派生日志密钥 → AES-256-GCM 解密 + gzip 解压
		key := deriveLogKey(agentFP(r))
		if key == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "服务端未启用日志加密（未配置 AIOPS_SECRET_KEY）"})
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
			return
		}
		plain, err := openLog(key, raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "日志解密失败"})
			return
		}
		if err := json.Unmarshal(plain, &batch); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
			return
		}
	} else if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
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

// handleSearchLogs returns matching aggregated logs (host/level/keyword/time) with server-side pagination and stats.
func (s *Server) handleSearchLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var since int64
	if m := q.Get("since_min"); m != "" {
		if v, _ := strconv.Atoi(m); v > 0 {
			since = time.Now().Unix() - int64(v)*60
		}
	}
	page := 1
	if p := q.Get("page"); p != "" {
		if v, _ := strconv.Atoi(p); v > 0 {
			page = v
		}
	}
	pageSize := 50
	if ps := q.Get("page_size"); ps != "" {
		if v, _ := strconv.Atoi(ps); v > 0 && v <= 200 {
			pageSize = v
		}
	}
	items, total := s.logs.searchPage(q.Get("host"), q.Get("level"), q.Get("q"), since, page, pageSize)
	pages := 1
	if total > 0 {
		pages = (total + pageSize - 1) / pageSize
	}
	stats := s.logs.searchStats(q.Get("host"), q.Get("level"), q.Get("q"), since)
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     items,
		"total":     total,
		"pages":     pages,
		"page":      page,
		"page_size": pageSize,
		"stats":     stats,
	})
}

// handleLogDiagnose runs heuristic inspection against current log search context.
func (s *Server) handleLogDiagnose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HostID    string   `json:"host_id"`
		Hostname  string   `json:"hostname"`
		SinceMin  int      `json:"since_min"`
		ErrorLogs []string `json:"error_logs"`
		SingleLog string   `json:"single_log"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = struct {
			HostID    string   `json:"host_id"`
			Hostname  string   `json:"hostname"`
			SinceMin  int      `json:"since_min"`
			ErrorLogs []string `json:"error_logs"`
			SingleLog string   `json:"single_log"`
		}{}
	}

	since := int64(0)
	if req.SinceMin > 0 {
		since = time.Now().Unix() - int64(req.SinceMin)*60
	} else {
		since = time.Now().Unix() - 1800 // default 30 min
	}

	// Build inspection context from log search
	ctx := inspectionContext{}
	if req.HostID != "" {
		if h := s.hostByID(req.HostID); h != nil {
			ctx.OnlineHosts++
			if h.Latest != nil {
				ctx.HighUsage = append(ctx.HighUsage,
					fmt.Sprintf("%s CPU %.1f%% Mem %.1f%% Disk %.1f%%", h.Hostname, h.Latest.CPUPercent, h.Latest.MemPercent, h.Latest.DiskPercent))
			}
		}
	}
	ctx.RecentErrors = s.logs.recentErrors(since, 50)
	ctx.ErrorCount = s.logs.errorCount(since)

	// Run heuristic inspection
	summary, findings := heuristicInspect(ctx)

	// Build a report
	report := InspectionReport{
		ID:       atomic.AddInt64(&s.ai.nextID, 1),
		Ts:       time.Now().Unix(),
		Source:   "heuristic",
		Trigger:  "manual",
		Summary:  summary,
		Findings: findings,
		Context:  fmt.Sprintf("日志诊断：主机 %s，时间范围 %d 分钟", req.Hostname, req.SinceMin),
	}

	// If single log line provided, prepend it to the summary
	if req.SingleLog != "" {
		report.Summary = "单条日志诊断：" + req.SingleLog + "\n" + report.Summary
	}
	if len(req.ErrorLogs) > 0 {
		report.Context += fmt.Sprintf("，错误日志 %d 条", len(req.ErrorLogs))
	}

	s.ai.mu.Lock()
	s.ai.reports = append(s.ai.reports, report)
	if len(s.ai.reports) > 100 {
		s.ai.reports = s.ai.reports[len(s.ai.reports)-100:]
	}
	s.ai.mu.Unlock()

	// Also notify via message hub
	if s.ai.onReport != nil {
		s.ai.onReport(report)
	}

	actor, ip := s.actorIP(r)
	s.store.AddLog(LogEntry{
		Kind:    KindOperation,
		Level:   "info",
		Actor:   actor,
		IP:      ip,
		Message: fmt.Sprintf("日志诊断：主机 %s，结论 %s", req.Hostname, summary),
	})

	writeJSON(w, http.StatusOK, report)
}

// ----------------------------------------------------------------------------
// AI: config + inspection + diagnosis
// ----------------------------------------------------------------------------

func (s *Server) handleGetAIConfig(w http.ResponseWriter, r *http.Request) {
	c := s.cfg.AIConfig()
	if c.APIKey != "" {
		c.APIKey = "****" // never echo the key back to the browser
	}
	if c.EmbedAPIKey != "" {
		c.EmbedAPIKey = "****" // 嵌入 Key 同样不回显
	}
	if c.RerankAPIKey != "" {
		c.RerankAPIKey = "****" // rerank Key 同样不回显
	}
	if c.MCPToken != "" {
		c.MCPToken = "****" // MCP 令牌是密钥，同样不回显
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleSetAIConfig(w http.ResponseWriter, r *http.Request) {
	var c AIConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	// 启用 MCP Server 时强制强令牌：MCP 鉴权无登录节流，弱令牌可被在线暴力破解。脱敏占位(****)表示
	// 沿用已保存的令牌（此前已校验），不重复校验。
	if c.MCPEnabled && !strings.Contains(c.MCPToken, "****") && len(strings.TrimSpace(c.MCPToken)) < 16 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "启用 MCP Server 需设置至少 16 位的强随机访问令牌"})
		return
	}
	if err := s.cfg.SetAIConfig(c); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.actorName(r), IP: s.clientIP(r), Message: Tz("ai.config_saved")})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleTestAIConfig verifies the AI provider is reachable and actually returns a
// completion via SSE streaming, so operators can confirm endpoint/key/model BEFORE
// relying on it. POST /api/v1/ai/test — a masked/blank key means "use the currently-saved one".
func (s *Server) handleTestAIConfig(w http.ResponseWriter, r *http.Request) {
	var c AIConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if c.APIKey == "" || strings.Contains(c.APIKey, "****") {
		c.APIKey = s.cfg.AIConfig().APIKey // the browser never receives the real key
	}
	if strings.TrimSpace(c.Endpoint) == "" || strings.TrimSpace(c.Model) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "请先填写 Endpoint 和模型名称"})
		return
	}
	// Classify provider for targeted hints
	_, prov := normalizeEndpoint(c.Endpoint)
	hint := "openai"
	switch prov {
	case aiProvAnthropic:
		hint = "anthropic"
	default: // aiProvOpenAI
		if isBailianEndpoint(c.Endpoint) {
			hint = "bailian-compat"
		}
	}
	start := time.Now()

	// 统一使用流式 SSE 输出
	s.setupSSE(w)
	reply, err := streamChatFiltered(r.Context(), w, c, []map[string]string{
		{"role": "system", "content": "你是连通性自检助手，用一句话确认你已就绪。"},
		{"role": "user", "content": "请回复：AI 服务正常，已就绪。"},
	})
	latency := time.Since(start).Milliseconds()
	if err != nil {
		fmt.Fprintf(w, "data: {\"error\":%s,\"latency_ms\":%d,\"provider_hint\":%s}\n\n", jsonString(err.Error()), latency, jsonString(hint))
		fmt.Fprint(w, "data: [DONE]\n\n")
		return
	}
	// 发送结果元数据后结束
	meta, _ := json.Marshal(map[string]any{"ok": true, "reply": reply, "latency_ms": latency, "model": c.Model, "provider_hint": hint})
	fmt.Fprintf(w, "data: {\"result\":%s}\n\n", string(meta))
	fmt.Fprint(w, "data: [DONE]\n\n")
}

// handleTestEmbedConfig 测试向量化/嵌入模型连通性。
// POST /api/v1/ai/test-embed — 用一条简短文本调用 embedText，返回 ok + 延迟。
func (s *Server) handleTestEmbedConfig(w http.ResponseWriter, r *http.Request) {
	var c AIConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if c.EmbedAPIKey == "" || strings.Contains(c.EmbedAPIKey, "****") {
		c.EmbedAPIKey = s.cfg.AIConfig().EmbedAPIKey
		if c.EmbedAPIKey == "" {
			c.EmbedAPIKey = s.cfg.AIConfig().APIKey
		}
	}
	if strings.TrimSpace(c.EmbedEndpoint) == "" {
		c.EmbedEndpoint = s.cfg.AIConfig().EmbedEndpoint
		if c.EmbedEndpoint == "" {
			c.EmbedEndpoint = s.cfg.AIConfig().Endpoint
		}
	}
	if strings.TrimSpace(c.EmbedModel) == "" {
		c.EmbedModel = s.cfg.AIConfig().EmbedModel
	}
	if c.EmbedAPIKey == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "请先填写 API Key"})
		return
	}
	if strings.TrimSpace(c.EmbedModel) == "" && !isBailianEndpoint(c.EmbedEndpoint) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "请先填写嵌入模型名称"})
		return
	}
	c.Enabled = true
	start := time.Now()
	emb := embedText(c, "连通性测试")
	latency := time.Since(start).Milliseconds()
	if len(emb) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "向量化调用失败，请检查 Endpoint / Key / 模型名称", "latency_ms": latency})
		return
	}
	modelLabel := c.EmbedModel
	if modelLabel == "" {
		modelLabel = "自动"
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "latency_ms": latency, "dimensions": len(emb), "model": modelLabel})
}

// handleTestRerankConfig 测试重排(rerank)模型连通性。
// POST /api/v1/ai/test-rerank — 用一条 query + 两条候选调用 rerankDocuments，返回 ok + 延迟。
func (s *Server) handleTestRerankConfig(w http.ResponseWriter, r *http.Request) {
	var c AIConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	saved := s.cfg.AIConfig()
	// 脱敏/留空的 Key 用已保存值回填；Endpoint/Key 的「rerank→嵌入→主」兜底交给 rerankConfig。
	if c.RerankAPIKey == "" || strings.Contains(c.RerankAPIKey, "****") {
		c.RerankAPIKey = saved.RerankAPIKey
	}
	if c.EmbedAPIKey == "" || strings.Contains(c.EmbedAPIKey, "****") {
		c.EmbedAPIKey = saved.EmbedAPIKey
	}
	if c.APIKey == "" || strings.Contains(c.APIKey, "****") {
		c.APIKey = saved.APIKey
	}
	if strings.TrimSpace(c.RerankModel) == "" {
		c.RerankModel = saved.RerankModel
	}
	c.Enabled = true
	if _, _, _, ok := rerankConfig(c); !ok {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "请先填写 rerank 模型名称与 API Key（Endpoint / Key 可留空复用嵌入 / 主配置）"})
		return
	}
	start := time.Now()
	order := rerankDocuments(c, "数据库连接超时如何排查", []string{"数据库连接池耗尽导致超时", "今天午餐吃什么"}, 2)
	latency := time.Since(start).Milliseconds()
	if len(order) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "rerank 调用失败，请检查 Endpoint / Key / 模型名称", "latency_ms": latency})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "latency_ms": latency, "model": strings.TrimSpace(c.RerankModel)})
}

// handleAITerminalAccess 开启/关闭「AI 终端只读巡检」权限（独立开关）。
// 开启为高风险授权：必须当前用户已设终端连接密码并校验通过（复用终端二次密码机制 + 限流）；
// 关闭为安全方向，无需密码。开启后 AI 可执行【只读】诊断命令替代人工巡检，禁止任何增删改。
// POST /api/v1/ai/terminal-access  {enabled:bool, password?:string}
func (s *Server) handleAITerminalAccess(w http.ResponseWriter, r *http.Request) {
	acc, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": Tr(r, "auth.unauthorized")})
		return
	}
	var req struct {
		Enabled  bool   `json:"enabled"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if !req.Enabled { // 关闭：安全方向，直接关
		_ = s.cfg.SetSreyunTerminalEnabled(false)
		s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r), Message: "关闭 AI 终端只读巡检权限：" + acc.Username})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": false})
		return
	}
	// 开启：必须已设终端密码 + 校验通过
	if !s.cfg.HasTerminalPassword(acc.Username) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请先在「个人设置 → 终端安全」中设置终端连接密码，再开启 AI 终端巡检"})
		return
	}
	allowed, remaining := s.auth.terminalAttemptAllowed(acc.Username)
	if !allowed {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": Tr(r, "terminal_auth.locked"), "locked": true})
		return
	}
	if !s.cfg.VerifyTerminalPassword(acc.Username, req.Password) {
		s.auth.terminalAttemptFailed(acc.Username)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "终端密码错误", "remaining": remaining - 1})
		return
	}
	s.auth.terminalAttemptReset(acc.Username)
	if err := s.cfg.SetSreyunTerminalEnabled(true); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "保存失败"})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r), Message: "开启 AI 终端只读巡检权限（已校验终端密码）：" + acc.Username})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": true})
}

// fetchProviderModels 查询 OpenAI 兼容 provider 的 GET {base}/models（适用于
// OpenAI / DeepSeek / Ollama / 百炼兼容模式…），返回模型 ID 列表。任何失败都返回
// nil,调用方据此提示用户手动输入。Anthropic 无公开的 models 列表端点 → 返回 nil。
func fetchProviderModels(endpoint, apiKey string) []string {
	if strings.TrimSpace(endpoint) == "" {
		return nil
	}
	ep, prov := normalizeEndpoint(endpoint)
	if prov == aiProvAnthropic {
		return nil
	}
	base := ep
	if i := strings.LastIndex(base, "/chat/completions"); i >= 0 {
		base = base[:i]
	}
	base = strings.TrimRight(base, "/")
	req, err := http.NewRequest(http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil {
		return nil
	}
	var ids []string
	for _, m := range out.Data {
		id := strings.TrimSpace(m.ID)
		if id != "" && isLikelyChatModel(id) {
			ids = append(ids, id)
		}
	}
	return ids
}

// isLikelyChatModel 过滤掉明显非「对话(chat)」类模型（嵌入 / 语音 / 图像 / 重排 / 审核 / 视频等）。
// 这些模型不能用于 /chat/completions，用户从下拉里选中它们测试/对话会直接 404/400，
// 是"下拉选了模型却报 404"的根因。多模态对话模型（vl / audio / vision）予以保留。
func isLikelyChatModel(id string) bool {
	l := strings.ToLower(id)
	for _, bad := range []string{
		"embedding", "embed", "bge", "m3e", "gte-", "text2vec",
		"tts", "whisper", "transcrib", "text-to-speech", "speech-to", "asr",
		"dall-e", "dalle", "stable-diffusion", "flux", "cogview", "wanx", "midjourney", "kolors",
		"rerank", "moderation",
		"sora", "video",
	} {
		if strings.Contains(l, bad) {
			return false
		}
	}
	return true
}

// handleAIModels 返回模型下拉候选：仅自动获取已配置 provider 的实时模型列表
// （表单值优先，其次已保存配置；百炼兼容模式会返回 qwen-* 等）。
// 不再内置任何预设 / 精选模型；获取不到时返回空列表，前端提示手动输入模型名
// （Anthropic 无公开 /models 端点，也走手动输入）。
// POST /api/v1/ai/models  {endpoint?, api_key?}
func (s *Server) handleAIModels(w http.ResponseWriter, r *http.Request) {
	type modelSuggestion struct {
		Value    string `json:"value"`
		Label    string `json:"label"`
		Provider string `json:"provider"`
	}
	var c AIConfig
	_ = json.NewDecoder(r.Body).Decode(&c) // body 可选
	saved := s.cfg.AIConfig()
	if strings.TrimSpace(c.Endpoint) == "" {
		c.Endpoint = saved.Endpoint
	}
	if c.APIKey == "" || strings.Contains(c.APIKey, "****") {
		c.APIKey = saved.APIKey
	}

	// 自动获取：查询 provider 的 GET {base}/models，作为模型候选的唯一来源。
	seen := map[string]bool{}
	models := make([]modelSuggestion, 0, 16)
	for _, id := range fetchProviderModels(c.Endpoint, c.APIKey) {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, modelSuggestion{Value: id, Label: id, Provider: "live"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models, "live_count": len(models)})
}

// handleAIChat is a lightweight SRE-assistant chat over the configured provider so
// operators can interactively confirm the AI works and ask ops questions.
// POST /api/v1/ai/chat  {message, history:[{role,content}]}
func (s *Server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message    string `json:"message"`
		IncidentID int64  `json:"incident_id,omitempty"`
		Stream     bool   `json:"stream,omitempty"`
		History    []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"history,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "消息不能为空"})
		return
	}
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.Endpoint == "" || cfg.Model == "" {
		s.setupSSE(w)
		fmt.Fprint(w, "data: {\"error\":\"AI 未配置或未启用，请先在「AI 设置」填写并保存\"}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
		return
	}

	// 尽早建立 SSE 连接并 Flush，让前端立即显示「思考中」动画；
	// 后续的 system prompt 构建和 RAG 检索在 SSE 已建立后执行，不阻塞首屏。
	s.setupSSE(w)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Build system prompt. If incident_id is provided, inject full rich context
	// (metrics + alerts + logs + RAG + rules) just like buildIncidentDiagnosisPrompt.
	sys := "你是资深 SRE / 运维助手，用简洁中文回答监控、告警、排障、性能与自动化相关问题；无关问题礼貌拒答。"
	if req.IncidentID > 0 {
		if inc, found := s.incidents.Get(req.IncidentID); found {
			sys = s.buildIncidentDiagnosisPrompt(inc) + "\n\n你是资深 SRE / 运维助手，结合以上事件上下文回答操作员的提问，用简洁中文给出具体建议。"
			for _, e := range inc.Timeline {
				if e.Kind == "ai_diagnosis" && e.Text != "" {
					sys += "\n\n【已有 AI 诊断结论】\n" + e.Text
					break
				}
			}
		}
	}

	// RAG: 检索历史记忆注入 system prompt，让 AI 能跨会话复用已有知识
	// （embedText + PG 查询可能耗时 1-3s，已在 SSE 连接建立后执行）
	memKind := "chat"
	if req.IncidentID > 0 {
		memKind = "diagnosis"
	}
	sys += s.retrieveMemoryForPrompt(memKind, req.Message, 8)
	sys += s.retrieveSkillsForPrompt(req.Message, 4) // 注入已掌握技能(SOP)

	msgs := []map[string]string{{"role": "system", "content": sys}}
	// 上下文压缩：长历史摘要化 + 保留最近轮次，替代此前"硬截断最近 10 轮"（无状态：每次基于全量历史）
	histMsgs := make([]map[string]string, 0, len(req.History))
	for _, h := range req.History {
		if (h.Role == "user" || h.Role == "assistant") && strings.TrimSpace(h.Content) != "" {
			histMsgs = append(histMsgs, map[string]string{"role": h.Role, "content": h.Content})
		}
	}
	msgs = append(msgs, compactHistory(histMsgs, 8)...) // 无会话缓存入口：用无 LLM 的廉价压缩，避免每轮同步摘要
	msgs = append(msgs, map[string]string{"role": "user", "content": req.Message})

	reply, _ := streamChat(r.Context(), w, cfg, msgs, nil)
	// 向量化本轮交互 → 永久入库沉淀为 RAG 记忆
	if strings.TrimSpace(reply) != "" {
		go s.rememberAI("chat", "ai_chat", "【用户】\n"+req.Message+"\n\n【AI】\n"+reply)
	}
}

// handleAIAssist 是全站「AI 辅助」按钮的统一后端：按 task 选择专用系统提示词，注入调用方
// 提供的上下文与 RAG 历史记忆，复用 streamChat 流式（逐字 + 思维链）输出，并把本轮沉淀为记忆。
// 一个端点覆盖：LogQL/PromQL 生成、剧本生成、图表数据分析、审计日志诊断、弹窗结果诊断、通用问答。
// POST /api/v1/ai/assist  {task, input, context}
func (s *Server) handleAIAssist(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Task    string `json:"task"`
		Input   string `json:"input"`
		Context string `json:"context,omitempty"`
		History []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"history,omitempty"` // 多轮追问：前几轮 Q&A（基于同一份 context 的会话）
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	req.Task = strings.TrimSpace(req.Task)
	if strings.TrimSpace(req.Input) == "" && strings.TrimSpace(req.Context) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请提供需求描述或待分析内容"})
		return
	}
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.Endpoint == "" || cfg.Model == "" {
		s.setupSSE(w)
		fmt.Fprint(w, "data: {\"error\":\"AI 未配置或未启用，请先在「AI 设置」填写并保存\"}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
		return
	}
	// 尽早建连并 Flush，前端立即显示「思考中」；后续 prompt 构建 + RAG 检索不阻塞首屏。
	s.setupSSE(w)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	sys := buildAssistSystemPrompt(req.Task, req.Context)
	// 诊断/分析类任务注入历史记忆（跨会话复用既有排障经验）
	memKind := "chat"
	switch req.Task {
	case "audit_diagnosis", "result_diagnosis", "chart_analysis", "snmp_diagnosis", "trap_diagnosis",
		"hardware_diagnosis", "netflow_diagnosis", "checks_diagnosis", "forward_diagnosis", "apimon_diagnosis",
		"content_audit_diagnosis", "dashboard_analysis", "dashboard_optimize":
		memKind = "diagnosis"
	}
	sys += s.retrieveMemoryForPrompt(memKind, strings.TrimSpace(req.Input+" "+req.Context), 6)
	sys += s.retrieveSkillsForPrompt(strings.TrimSpace(req.Input+" "+req.Context), 4) // 注入已掌握技能(SOP)
	userMsg := strings.TrimSpace(req.Input)
	if userMsg == "" {
		userMsg = "请根据上述上下文进行分析并给出结论。"
	}
	msgs := []map[string]string{{"role": "system", "content": sys}}
	// 多轮追问：把前几轮 Q&A 拼进来（限长防膨胀）。系统提示词已带同一份数据上下文，
	// 于是后续追问都基于同一份流量/设备/日志数据展开，实现「基于同一数据的会话交流」。
	if n := len(req.History); n > 0 {
		start := 0
		if n > 20 {
			start = n - 20
		}
		for _, h := range req.History[start:] {
			if (h.Role == "user" || h.Role == "assistant") && strings.TrimSpace(h.Content) != "" {
				msgs = append(msgs, map[string]string{"role": h.Role, "content": h.Content})
			}
		}
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": userMsg})
	// 看板相关任务：关闭深度思考，避免推理模型长时间「想」导致超时。
	var reply string
	switch req.Task {
	case "dashboard_prompt_optimize", "dashboard_optimize", "dashboard_analysis":
		reply, _ = streamChatOpts(r.Context(), w, cfg, msgs, nil, aiCallOpts{DisableThinking: true})
	default:
		reply, _ = streamChat(r.Context(), w, cfg, msgs, nil)
	}
	if strings.TrimSpace(reply) != "" {
		go s.rememberAI("assist", "assist:"+req.Task, "【AI 辅助·"+req.Task+"】\n"+userMsg+"\n\n【AI】\n"+reply)
	}
}

// buildAssistSystemPrompt 为各类「AI 辅助」任务构造专用系统提示词。ctxText 是调用方（前端）
// 预先整理好的上下文文本（可用标签 / 数据摘要 / 结果正文 / 审计条目等），原样注入。
func buildAssistSystemPrompt(task, ctxText string) string {
	ctxBlock := ""
	if strings.TrimSpace(ctxText) != "" {
		ctxBlock = "\n\n【上下文】\n" + ctxText
	}
	switch task {
	case "logql":
		return "你是 Grafana Loki LogQL 专家。根据运维人员的自然语言需求，生成一条正确、高效的 LogQL 查询。" +
			"要求：① 先用一个 ```logql 代码块只放最终查询语句；② 再用一两句中文说明查询逻辑与关键点；" +
			"③ 必须使用上下文中列出的真实标签，不要臆造标签名；④ 善用标签选择器缩小范围后再做 |= / |~ 过滤与 | json / | logfmt 解析，避免全量扫描；" +
			"⑤ 如需统计，用 rate()/count_over_time() 等。若信息不足，指出还需要哪些标签。" + ctxBlock
	case "promql":
		return "你是 Prometheus PromQL 专家。根据运维人员的自然语言需求，生成一条正确、高效的 PromQL 查询。" +
			"要求：① 先用一个 ```promql 代码块只放最终查询语句；② 再用一两句中文说明；③ 优先使用上下文中列出的真实指标名与标签；" +
			"④ 计数器类指标记得配合 rate()/irate() 与时间窗口；聚合用 sum/avg by(...)；⑤ 阈值/比率类给出清晰表达式。若信息不足，指出还需要哪些指标。" + ctxBlock
	case "playbook":
		return "你是自动化运维专家。根据运维人员的描述，生成一个可直接导入本平台的「运维剧本」JSON。" +
			"严格输出一个 ```json 代码块，结构为：{\"name\":\"剧本名\",\"description\":\"用途\",\"steps\":[{" +
			"\"name\":\"步骤名\",\"command\":\"Linux/通用命令\",\"command_win\":\"Windows 覆盖命令(可选)\",\"target\":\"all|category:分类|system:linux|host:ID\"," +
			"\"timeout_sec\":30,\"continue_on_error\":false,\"ignore_exit\":false,\"register\":\"变量名(可选)\",\"when\":\"条件(可选)\"}]}。" +
			"要求：① 命令须安全、幂等、只读优先，破坏性操作需在 description 明确风险并默认 continue_on_error=false；" +
			"② 跨平台差异用 command_win / command_mac 覆盖；③ 需要引用上一步输出时用 register + {{变量名}}；④ 代码块后用中文简述每步意图与注意事项。" + ctxBlock
	case "chart_analysis":
		return "你是资深 SRE。以下是监控图表/指标的数据摘要。请：① 概述整体趋势与当前水位；② 指出异常点、突变、持续高位或逼近阈值的项；" +
			"③ 推断可能原因；④ 给出可执行的排查方向或处置建议。用简洁中文、分点作答，只依据给定数据，不要编造。" + ctxBlock
	case "dashboard_prompt_optimize":
		return "你是可观测性与监控看板设计专家。把运维人员的简短需求改写成【简洁、可直接用于生成看板】的需求描述。" +
			"禁止深度思考与长篇铺陈。要求（控制在 400 字以内）：\n" +
			"① 一句话点明主题与对象；② 用短列表给出 6~10 个关键指标/黄金信号；" +
			"③ 各用一词标注建议图型（timeseries/stat/gauge/piechart/barchart/table）；" +
			"④ 一句布局顺序（概览在上、明细在下）；⑤ 若需下钻，点名一个模板变量即可。\n" +
			"直接输出改写正文，不要 JSON、不要代码块、不要解释。若上下文有可用指标，优先用真实指标名。" + ctxBlock
	case "dashboard_analysis":
		return "你是资深 SRE。以下是一个监控仪表盘的实时数据摘要（各面板当前值）。请对该看板做健康研判：" +
			"① 总体健康结论（正常/需关注/告急）；② 逐项指出异常、逼近阈值、异常趋势的面板与数值；③ 推断可能根因与关联；" +
			"④ 给出可执行的处置建议与后续观察项；⑤ 若存在需要立即跟进的问题，明确点出并建议是否建工单。" +
			"用简洁中文分点作答，只依据给定数据，不臆测。" + ctxBlock
	case "dashboard_optimize":
		return "你是可观测性专家，正在评审并优化一个监控仪表盘。禁止深度思考与思维链，直接作答。下面给出现有结构与实时近况。请：\n" +
			"① 先用简洁中文分点说明优化要点（补哪些黄金信号面板、修正哪些查询/单位/图例、建议的告警或 SLO、布局改进；PromQL 用行内反引号，不要用代码块）；\n" +
			"② 然后输出【优化后的完整看板】为唯一的一个 ```json 代码块（供用户一键应用），结构：" +
			"{\"name\":\"看板名\",\"vars\":[{\"name\":\"实例\",\"type\":\"query|custom\",\"query\":\"label_values(<指标>,<标签>)\"}]," +
			"\"panels\":[{\"title\":\"标题\",\"type\":\"timeseries|stat|gauge|table|text\",\"unit\":\"percent|percentunit|bytes|Bps|s|ms|reqps|short|\"," +
			"\"w\":12,\"h\":8,\"targets\":[{\"expr\":\"<PromQL>\",\"legend\":\"{{标签}}\"}]}]}。\n" +
			"要求：保留仍合理的原面板并改进、补齐缺失的关键面板；只用现有指标名不臆造；w 为 1-24 栏宽、h 约 6-9；除该 json 块外不要出现其它代码块。" + ctxBlock
	case "audit_diagnosis":
		return "你是安全审计与运维合规专家。以下是平台审计日志片段。请：① 识别异常/高风险操作（越权、异常登录、批量删除、配置篡改、异地/异常时间访问等）；" +
			"② 归纳可疑模式与关联行为；③ 评估风险等级；④ 给出处置与加固建议。用简洁中文分点作答，严格基于给定日志，不臆测。" + ctxBlock
	case "result_diagnosis":
		return "你是资深 SRE 值班工程师。以下是某项操作/查询/巡检的执行结果。请：① 解读结果含义；② 判断是否异常及严重程度；" +
			"③ 分析可能原因；④ 给出下一步排查或处置建议。用简洁中文分点作答，只基于给定结果，信息不足时说明还需要什么。" + ctxBlock
	case "playbook_precheck":
		return "你是自动化运维安全审计专家。以下是一份【即将执行】的运维剧本（步骤/命令/目标/选项）。请在执行前做风险预检：\n" +
			"① 首行用【风险等级：红/黄/绿】给出总体评级（红=含破坏性或高危操作，需人工复核；黄=有注意事项；绿=安全可执行）；\n" +
			"② 逐步排查并指出问题：破坏性操作（rm -rf、dd、mkfs、fdisk、drop/truncate、shutdown/reboot、kill -9、iptables flush 等）、" +
			"非幂等风险（重复执行是否会累积副作用或损坏）、跨平台隐患（Linux/Windows/macOS 命令差异、是否缺 command_win/command_mac）、" +
			"缺失防护（高危步骤未设 continue_on_error、超时不合理、未用 when 做前置校验、未用 register 校验上一步）；\n" +
			"③ 给出可直接采纳的加固建议。用简洁中文分点作答，只依据给定内容，不臆测。" + ctxBlock
	case "execution_retro":
		return "你是资深 SRE 值班工程师，正在对一次【失败的剧本执行】做复盘。以下是各主机的分步执行结果与输出。请：\n" +
			"① 定位失败根因（命令本身错误 / 目标主机环境或权限或依赖缺失 / 超时 / 基础设施抖动等），并引用关键错误输出佐证；\n" +
			"② 区分「个别主机失败」与「普遍失败」，指出受影响范围；\n" +
			"③ 给出针对性修复步骤与重跑建议（是否可安全重试、需先修什么）；\n" +
			"④ 提出对该剧本的改进（补 when 校验 / 调整超时 / 补 command_win 覆盖 / 加 continue_on_error 等）。" +
			"用简洁中文分点作答，严格基于给定输出，不臆测。" + ctxBlock
	case "remediation_rule":
		return "你是 SRE 自动化编排专家。请把给定【事件 + AI 诊断结论】里的处置建议，固化为一条「告警条件 → 修复剧本」的" +
			"『自动修复规则草稿』。严格只输出一个 ```json 代码块，结构如下：\n" +
			"{\"playbook\":{\"name\":\"修复剧本名\",\"description\":\"用途与风险说明\",\"steps\":[{\"name\":\"步骤名\"," +
			"\"command\":\"Linux/通用命令\",\"command_win\":\"Windows 覆盖命令(可选)\",\"target\":\"all\"," +
			"\"timeout_sec\":30,\"continue_on_error\":false}]}," +
			"\"rule\":{\"name\":\"规则名\",\"match_types\":[\"事件的告警类型,如 cpu/memory/disk/load/proc/offline\"]," +
			"\"min_level\":\"warning 或 critical\",\"match_category\":\"主机分类(可选,空=任意)\"," +
			"\"require_approval\":true,\"cooldown_sec\":300,\"max_per_hour\":3},\"existing_playbook_id\":\"\"}\n" +
			"要求：① 若【可用剧本】列表里已有能解决该问题的，填其 existing_playbook_id 并整段省略 playbook 字段；否则新建 playbook。" +
			"② 修复命令务必安全、幂等，优先『先只读诊断确认、再谨慎处置』；凡含破坏性或有风险的操作，require_approval 必须为 true，" +
			"并在 description 明确标注风险。③ match_types 用事件的真实告警类型，min_level 不低于事件级别；" +
			"target 用 \"all\"（自动修复引擎会把剧本限定在触发告警的那台主机上执行）。④ 给合理的 cooldown_sec 与 max_per_hour 防止告警抖动引发修复风暴。" +
			"⑤ 代码块之后，用中文简述：这条规则在什么条件下、对哪些主机、做什么，以及主要风险点与为何建议人工审批。" + ctxBlock
	case "duty_report":
		return dutyReportSystemPrompt + ctxBlock
	case "content_audit_diagnosis":
		return "你是资深数据安全(DLP)与合规审计专家。以下是从局域网被动抓取的明文 HTTP 内容审计记录——用户向各端点" +
			"（多为大模型服务，如 OpenAI/内网 Ollama/vLLM）发送的请求 prompt 与收到的响应 completion，部分已被内置规则" +
			"标注命中敏感数据。请：\n" +
			"① 一句话研判整体数据外泄风险（低/中/高）；\n" +
			"② 逐条指出【敏感数据外泄】风险：密钥/私钥/凭据/身份证/手机号等 PII、商业机密、源代码/内部文档被贴进大模型，" +
			"标明是谁(源IP)、发给谁(端点)、泄露了什么，按严重度排序；\n" +
			"③ 评估合规影响（等保/个人信息保护法/GDPR 视角）；\n" +
			"④ 给出可执行处置建议（阻断/告警/教育/收敛敏感词规则/改用合规内网模型等）。" +
			"用简洁中文分点作答，只依据给定记录、不臆造；未见敏感外泄时明确说明「未见明显敏感数据外泄」。" +
			"注意：你分析的是审计数据本身，回答里【不要原样复述完整的密钥/密码等敏感值】，用脱敏描述。" + ctxBlock
	case "hardware_diagnosis":
		return "你是资深数据中心硬件运维专家。以下是一台设备（服务器 / 存储 / 磁盘柜等）的硬件快照（整机身份、健康、" +
			"异常部件、BMC 事件、CPU/内存/存储/磁盘框/RAID/逻辑卷/电源/风扇/温度/固件等）。请：\n" +
			"① 一句话总体研判该设备当前整体运行状态（健康/需关注/有故障）；\n" +
			"② 逐条指出异常或劣化的部件（故障、SMART 预测故障、寿命偏低、温度逼近阈值、电源/风扇/冗余异常、BMC 事件），" +
			"并按紧急程度排序；\n" +
			"③ 分析可能原因与潜在风险（如某盘将坏、散热不足、电源冗余丢失）；\n" +
			"④ 给出可执行的处置与维护建议（更换/巡检/固件升级/散热整改等）。" +
			"用简洁中文分点作答，只依据给定快照数据，不臆造；数据正常时也要明确说明「未见异常」。" + ctxBlock
	case "snmp_diagnosis":
		return "你是资深网络运维专家。以下是通过 SNMP 轮询到的网络设备（交换机/路由器/防火墙）快照：系统信息、" +
			"各接口的 up/down 状态、带宽利用率、进出速率、错误率与丢包率。请：\n" +
			"① 一句话总体研判该设备当前网络状态（正常/需关注/有故障）；\n" +
			"② 逐条指出异常接口（链路 DOWN、利用率过高濒临拥塞、错误/丢包率异常），按紧急程度排序；\n" +
			"③ 分析可能原因（物理链路故障、光模块劣化、环路/广播风暴、带宽瓶颈、双工不匹配等）；\n" +
			"④ 给出可执行排查与处置建议（查线/换模块/扩容/限速/排查环路等）。" +
			"用简洁中文分点作答，只依据给定数据，不臆造；正常时明确说明「未见异常」。" + ctxBlock
	case "trap_diagnosis":
		return "你是资深网络运维专家。以下是网络设备主动上报的 SNMP Trap 事件列表（含来源 IP、trapOID、严重度、时间、" +
			"变量绑定）。请：\n" +
			"① 一句话总体研判这批 Trap 反映的整体状况；\n" +
			"② 归类并解读关键事件（linkDown/linkUp、认证失败、冷/热启动、厂商私有告警等），指出其业务含义；\n" +
			"③ 关联分析（如同一设备反复 linkDown/Up 说明链路抖动，大量认证失败可能是攻击或配置错误）；\n" +
			"④ 给出处置与后续观测建议。" +
			"用简洁中文分点作答，只依据给定事件，不臆造。" + ctxBlock
	case "checks_diagnosis":
		return "你是资深 SRE 与网站/服务可用性专家。以下是本平台【合成拨测监控】的当前快照（网站 HTTP / 端口 TCP / 主机 Ping / 进程存活 等探测项的状态、时延、HTTP 状态码、证书剩余天数、丢包率、探测间隔与最近错误）。请：\n" +
			"① 一句话总体研判当前拨测面的健康度（正常/需关注/有故障），点明 DOWN/异常项数量；\n" +
			"② 逐条列出异常或劣化项（探测失败、时延偏高、HTTP 状态码异常、证书临近到期、Ping 丢包、进程缺失），按紧急程度排序并引用关键数据；\n" +
			"③ 分析可能原因（目标服务宕机 / 网络链路 / DNS / 证书未续期 / 进程被杀 等）；\n" +
			"④ 给出可执行的排查与处置建议。用简洁中文分点作答，只依据给定快照，不臆造；全部正常时也要明确说明「未见异常」。" + ctxBlock
	case "forward_diagnosis":
		return "你是资深网络与运维专家。以下是本平台【端口转发 / 反向代理】的当前快照（TCP/UDP 转发与 HTTP 代理的监听地址、目标主机与端口、启用状态、活跃/累计会话数、跳板目标等）。请：\n" +
			"① 一句话总体研判转发面的运行状态；\n" +
			"② 指出可疑或需关注项（本应启用却已停用的、活跃会话异常为 0 或异常偏高的、指向同一目标的重复转发、跳板链路、监听地址冲突或过度暴露风险）；\n" +
			"③ 分析可能影响（服务不可达、端口占用、会话堆积、安全暴露面）；\n" +
			"④ 给出优化与排查建议，并从最小化开放的角度提示安全暴露面。用简洁中文分点作答，只依据给定快照，不臆造；无明显问题时也要明确说明「未见异常」。" + ctxBlock
	case "apimon_diagnosis":
		return "你是资深 SRE 与 API 性能专家。以下是本平台【API 业务监控】的当前快照（按业务系统分组的接口：最新状态、本次/平均/P95 响应时间、1h/24h 可用率、吞吐、异常接口数）。请：\n" +
			"① 一句话总体研判各业务系统的健康与 SLA 达成情况；\n" +
			"② 逐条列出异常或劣化接口（DOWN、可用率跌破阈值、P95 或平均时延偏高、吞吐异常），按业务影响排序并引用关键指标；\n" +
			"③ 分析可能原因（后端故障 / 依赖变慢 / 限流 / 网络 / 证书或鉴权失败 等）；\n" +
			"④ 给出可执行的排查与优化建议，并指出应优先处置的接口。用简洁中文分点作答，只依据给定快照，不臆造；全部达标时也要明确说明「未见异常」。" + ctxBlock
	case "netflow_diagnosis":
		return "你是资深网络流量分析与安全专家。以下是某主机在选定时间窗内的 NetFlow/流量快照（按维度的 Top Talkers 流量排行 + Top Flow 明细：源/目的 IP:端口、协议、字节、包数、平均包长、时长）。请：\n" +
			"① 一句话总体研判该主机流量是否正常（正常/需关注/疑似异常）；\n" +
			"② 指出异常或可疑模式：单点大流量/带宽打满、疑似端口扫描（同源大量不同目的端口或目的 IP）、疑似 DDoS 或反射放大（海量小包、UDP 突增）、异常外联（可疑目的 IP/端口、非业务端口外发）、数据外泄迹象（大流量上行到陌生外部地址）；\n" +
			"③ 分析可能原因与风险；\n" +
			"④ 给出可执行的排查与处置建议（抓包定位、封禁/限速、核实对应进程与业务等）。用简洁中文分点作答，只依据给定快照，不臆造；未见异常时也要明确说明「未见异常」。" + ctxBlock
	default: // generic
		return "你是资深 SRE / 运维助手，用简洁中文帮助运维人员处理监控、告警、排障、性能、日志与自动化相关问题；无关问题礼貌拒答。" + ctxBlock
	}
}

// handleAIAssistFeedback 闭环 A：运维人员对某次 AI 辅助结果的处置（采纳/👍/👎）回流为记忆强化
// 信号——「用了才算数」。语义定位该次 assist 记忆并强化或惩罚，使被反复采纳的生成/建议在后续
// RAG 检索中上浮、被否定的下沉，实现自我进化。
// POST /api/v1/ai/assist/feedback  {task, input, answer, action: applied|helpful|unhelpful}
func (s *Server) handleAIAssistFeedback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Task   string `json:"task"`
		Input  string `json:"input"`
		Answer string `json:"answer"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	factor := reinforceHelpful
	switch req.Action {
	case "applied":
		factor = reinforceApplied
	case "unhelpful":
		factor = penalizeUnhelpful
	}
	// 用「需求 + 回答」语义定位最相近的一条 assist 记忆并调整其优先级
	if text := strings.TrimSpace(req.Input + " " + req.Answer); text != "" {
		s.reinforceMemory("assist", text, factor)
		// 采纳/好评时，同步强化最相关技能(SOP)；差评不惩罚技能（技能来自多经验提炼，单次差评不足为据）
		if factor > 1.0 {
			s.reinforceSkill(text, factor)
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleListSkills 列出已提炼的可复用技能(SOP)。GET /api/v1/ai/skills
func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusOK, []Skill{})
		return
	}
	skills, err := s.pg.listSkills()
	if err != nil || skills == nil {
		skills = []Skill{}
	}
	writeJSON(w, http.StatusOK, skills)
}

// handleDeleteSkill 删除一条技能。DELETE /api/v1/ai/skills/{id}
func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	if id, ok := sreParseID(r); ok && s.pg != nil {
		_ = s.pg.deleteSkill(id)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleDistillSkills 手动触发一次技能提炼（回看 30 天）。POST /api/v1/ai/skills/distill
func (s *Server) handleDistillSkills(w http.ResponseWriter, r *http.Request) {
	n, err := s.distillSkills(30)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "created": n})
}

func (s *Server) handleListInspections(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.ai.Reports())
}

func (s *Server) handleRunInspection(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.ai.RunInspection("manual"))
}

// handleDiagnoseIncident runs an AI (or heuristic) diagnosis and appends it to
// the incident timeline. Supports optional stream=true parameter for SSE streaming.
// POST /api/v1/incidents/{id}/diagnose  {stream?:bool}
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

	// Optional stream flag
	var req struct {
		Stream bool `json:"stream,omitempty"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	cfg := s.cfg.AIConfig()
	if cfg.Enabled && cfg.Endpoint != "" && cfg.Model != "" {
		// 尽早建立 SSE 连接并 Flush，让前端立即显示「思考中」动画；
		// 后续的 prompt 构建（含 embedText）和 RAG 检索在 SSE 已建立后执行。
		s.setupSSE(w)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// AI mode: use rich context (metrics + alerts + logs + RAG + rules)
		sys := s.buildIncidentDiagnosisPrompt(inc)
		for _, e := range inc.Timeline {
			if e.Kind == "ai_diagnosis" && e.Text != "" {
				sys += "\n\n【已有 AI 诊断结论】\n" + e.Text
				break
			}
		}
		// P3：要求结构化输出（根因/置信度/证据/处置），置信度用固定行便于前端识别并渲染徽章。
		userMsg := fmt.Sprintf(`请对事件 #%d 进行诊断分析，严格按以下 Markdown 结构输出（保留各节标题，用简洁中文）：

## 🎯 根因研判
最可能的根本原因（1-3 句）。信息不足时明确指出还需要哪些数据。

## 📊 置信度
另起一行，格式固定为「置信度：高」/「置信度：中」/「置信度：低」三者之一（单独成行），再用一句话说明依据。

## 🔍 关键证据
逐条列出支撑判断的具体指标/日志/告警，须引用上文真实数据，不得编造。

## 🛠️ 处置建议
按优先级给出可执行步骤（编号列表）。`, inc.ID)
		// RAG: 检索历史诊断记忆注入 system prompt（已在 SSE 连接建立后执行）
		// 用事件本身（标题/类型/主机）作为检索查询：既让记忆与技能两侧共用【同一次】embedding（命中
		// LRU 缓存只嵌入一次），又让召回真正贴合本事件——而非贴合几乎固定的诊断模板 userMsg。
		ragQuery := strings.TrimSpace(inc.Title + " " + inc.Type + " " + inc.Hostname)
		if ragQuery == "" {
			ragQuery = userMsg
		}
		sys += s.retrieveMemoryForPrompt("diagnosis", ragQuery, 8)
		sys += s.retrieveSkillsForPrompt(ragQuery, 4) // 注入已掌握技能(SOP)

		diagMsgs := []map[string]string{
			{"role": "system", "content": sys},
			{"role": "user", "content": userMsg},
		}
		// 诊断生成：配置了 MoA 则多模型集成研判，否则单模型；两者都流式且不发 [DONE]（末尾统一发）。
		var diag string
		if len(moaModelList(cfg)) > 1 {
			diag = aiChatMoAStream(r.Context(), w, cfg, diagMsgs)
		} else {
			diag, _ = streamChatNoDone(r.Context(), w, cfg, diagMsgs, nil)
		}
		// 自我校验（可选）：独立第二遍对照证据复核结论，流式续写到同一响应。
		verify := ""
		if cfg.SelfVerify && strings.TrimSpace(diag) != "" {
			verify = streamSelfVerify(r.Context(), w, cfg, sys, diag)
		}
		// 统一收尾：发送一次 [DONE]
		fmt.Fprint(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if diag != "" {
			full := diag
			if strings.TrimSpace(verify) != "" {
				full += "\n\n🔎 自我校验：\n" + verify
			}
			s.incidents.AddEvent(id, "ai_diagnosis", "AI", full)
			s.store.MarkDirty()
			go s.saveDiagnosisEmbedding(id, inc, full)
			go s.rememberAI("diagnosis", fmt.Sprintf("incident:%d", inc.ID), "【事件】"+inc.Title+"\n【诊断结论】"+full)
		}
		return
	}

	// Fallback to heuristic
	diag, source := s.ai.Diagnose(inc)
	s.incidents.AddEvent(id, "ai_diagnosis", "启发式", diag)
	s.store.MarkDirty()
	writeJSON(w, http.StatusOK, map[string]string{"diagnosis": diag, "source": source})
}

// setupSSE sets the standard headers for Server-Sent Events streaming.
// X-Accel-Buffering: no 关闭 nginx / 网关的响应缓冲，保证逐帧实时到达客户端；
// 缺此头时反代会攒批下发，表现为「不逐字、整段蹦出」。Content-Type 一旦为
// text/event-stream，gzipResponseWriter 会自动转 passthrough（见 main.go），不再压缩缓冲。
func (s *Server) setupSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

// handleDiagnoseChatIncident provides multi-turn AI diagnosis chat for an
// incident, carrying the full incident context as system prompt so the operator
// can ask follow-up questions, challenge conclusions, or request deeper analysis.
// POST /api/v1/incidents/{id}/diagnose-chat  {message, history:[{role,content}], include_terminal}
func (s *Server) handleDiagnoseChatIncident(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		Message string `json:"message"`
		Stream  bool   `json:"stream,omitempty"`
		History []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"history,omitempty"`
		IncludeTerminal bool `json:"include_terminal,omitempty"`
		// P3-Req1: 图片/文件附件，与主 AI 对话保持一致
		Images []struct {
			MIME string `json:"mime"`
			Data string `json:"data"` // base64（不含 data: 前缀）
		} `json:"images,omitempty"`
		Files []struct {
			Name string `json:"name"`
			Text string `json:"text"`
		} `json:"files,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if strings.TrimSpace(req.Message) == "" && len(req.Images) == 0 && len(req.Files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "消息不能为空"})
		return
	}
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.Endpoint == "" || cfg.Model == "" {
		// AI 未配置时也走 SSE，前端才能正确解析错误
		s.setupSSE(w)
		fmt.Fprint(w, "data: {\"error\":\"AI 未配置或未启用，请先在「AI 设置」填写并保存\"}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
		return
	}
	// 尽早建立 SSE 连接并 Flush，让前端立即显示「思考中」动画；
	// 后续的 prompt 构建（含 embedText）和 RAG 检索在 SSE 已建立后执行。
	s.setupSSE(w)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	// Build rich system prompt with full incident context
	sys := s.buildIncidentDiagnosisPrompt(inc)
	// Collect existing AI diagnosis from timeline as additional context
	for _, e := range inc.Timeline {
		if e.Kind == "ai_diagnosis" && e.Text != "" {
			sys += "\n\n【已有 AI 诊断结论】\n" + e.Text
			break // only the latest one
		}
	}
	// Optionally inject terminal operation summary (方案 A: 分段摘要注入)
	if req.IncludeTerminal && inc.HostID != "" {
		if termSummary := s.buildTerminalSummary(inc.HostID); termSummary != "" {
			sys += "\n\n【终端操作记录（分段摘要）】\n" + termSummary
		}
	}
	// RAG: 检索历史诊断记忆注入 system prompt
	sys += s.retrieveMemoryForPrompt("diagnosis", req.Message, 8)
	sys += s.retrieveSkillsForPrompt(req.Message, 4) // 注入已掌握技能(SOP)
	msgs := []map[string]string{{"role": "system", "content": sys}}
	// 上下文压缩：长历史摘要化 + 保留最近轮次，替代此前"硬截断最近 20 轮"
	histMsgs := make([]map[string]string, 0, len(req.History))
	for _, h := range req.History {
		if (h.Role == "user" || h.Role == "assistant") && strings.TrimSpace(h.Content) != "" {
			histMsgs = append(histMsgs, map[string]string{"role": h.Role, "content": h.Content})
		}
	}
	msgs = append(msgs, compactHistory(histMsgs, 10)...) // 无会话缓存入口：用无 LLM 的廉价压缩
	// Req1: 将上传文件文本注入用户消息，图片走多模态链路
	userMsg := req.Message
	for _, f := range req.Files {
		txt := strings.TrimSpace(f.Text)
		if txt == "" {
			continue
		}
		if len([]rune(txt)) > 8000 { // 限制单文件注入长度
			txt = string([]rune(txt)[:8000]) + "\n…（文件过长，已截断）"
		}
		name := f.Name
		if name == "" {
			name = "附件"
		}
		userMsg += fmt.Sprintf("\n\n【上传的文件：%s】\n%s", name, txt)
	}
	if strings.TrimSpace(userMsg) == "" && len(req.Images) > 0 {
		userMsg = "（上传了图片，请查看并分析）"
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": userMsg})

	// Req1: 解析图片为 chatImage 切片，传入 streamChat 多模态链路
	var images []chatImage
	for _, im := range req.Images {
		if strings.TrimSpace(im.Data) == "" {
			continue
		}
		images = append(images, chatImage{MIME: im.MIME, Data: im.Data})
		if len(images) >= 4 { // 最多 4 张，控制上下文与成本
			break
		}
	}

	// streamChat 成功时已发 [DONE]，失败时发 error 帧（前端 onError 即终止），故此处无需再补发 [DONE]
	// ——与 handleAIChat / handleAIAssist 保持一致。
	reply, _ := streamChat(r.Context(), w, cfg, msgs, images)
	if reply != "" {
		s.saveDiagnosisChatTurn(id, req.Message, reply)
		go s.saveDiagnosisEmbedding(id, inc, reply)
		go s.rememberAI("diagnosis", fmt.Sprintf("incident:%d", inc.ID), "【事件】"+inc.Title+"\n【诊断对话】"+req.Message+"\n【AI回复】"+reply)
	}
}

// buildIncidentDiagnosisPrompt constructs a system prompt with the incident's
// full context — metadata, timeline, host metrics, active alerts, recent logs —
// so the AI has all the information it needs to reason about the problem without
// the operator having to retype it. All data-source failures are non-fatal: they
// log a warning and skip the affected section rather than blocking the diagnosis.
func (s *Server) buildIncidentDiagnosisPrompt(inc Incident) string {
	var b strings.Builder
	b.WriteString("你是资深 SRE 值班工程师，正在协助排查一个线上事件。以下是该事件的完整上下文：\n\n")
	fmt.Fprintf(&b, "事件 #%d：%s\n", inc.ID, inc.Title)
	fmt.Fprintf(&b, "严重程度：%s | 状态：%s | 来源：%s\n", inc.Severity, inc.Status, inc.Source)
	if inc.Hostname != "" {
		fmt.Fprintf(&b, "关联主机：%s\n", inc.Hostname)
	}
	if inc.Type != "" {
		fmt.Fprintf(&b, "告警类型：%s\n", inc.Type)
	}
	if inc.Assignee != "" {
		fmt.Fprintf(&b, "指派人：%s\n", inc.Assignee)
	}
	fmt.Fprintf(&b, "创建时间：%s\n", time.Unix(inc.CreatedAt, 0).Format("2006-01-02 15:04:05"))
	// Timeline summary
	b.WriteString("\n事件时间线摘要：\n")
	for _, e := range inc.Timeline {
		ts := time.Unix(e.Ts, 0).Format("15:04:05")
		if e.Text != "" {
			fmt.Fprintf(&b, "  [%s] %s — %s: %s\n", ts, e.Kind, e.Actor, trimLine(e.Text, 200))
		} else {
			fmt.Fprintf(&b, "  [%s] %s — %s\n", ts, e.Kind, e.Actor)
		}
	}

	// --- 1. 实时指标快照 ---
	if inc.HostID != "" {
		if h := s.hostByID(inc.HostID); h != nil && h.Latest != nil {
			m := h.Latest
			b.WriteString("\n【当前主机指标】（最近一个采样点）\n")
			fmt.Fprintf(&b, "  CPU %.1f%% | 内存 %.1f%% (%d/%d GB) | 磁盘 %.1f%%",
				m.CPUPercent, m.MemPercent, m.MemUsed/1073741824, m.MemTotal/1073741824, m.DiskPercent)
			if m.SwapTotal > 0 {
				fmt.Fprintf(&b, " | SWAP %.1f%%", m.SwapPercent)
			}
			fmt.Fprintf(&b, "\n  Load %.2f/%.2f/%.2f | 进程 %d | 网络 ↓%.1f ↑%.1f MB/s",
				m.Load1, m.Load5, m.Load15, m.ProcCount,
				m.NetRecvRate/1048576, m.NetSentRate/1048576)
			if m.DiskReadRate+m.DiskWriteRate > 0 {
				fmt.Fprintf(&b, " | 磁盘IO ↓%.1f ↑%.1f MB/s IOPS r%.0f/w%.0f",
					m.DiskReadRate/1048576, m.DiskWriteRate/1048576,
					m.DiskReadIOPS, m.DiskWriteIOPS)
			}
			if m.Uptime > 0 {
				fmt.Fprintf(&b, " | 运行 %s", formatUptime(m.Uptime))
			}
			b.WriteByte('\n')
			// Per-disk details
			if len(m.Disks) > 0 {
				b.WriteString("  各磁盘：")
				for i, d := range m.Disks {
					if i > 0 {
						b.WriteString(" · ")
					}
					fmt.Fprintf(&b, "%s %.1f%%", d.Path, d.Percent)
				}
				b.WriteByte('\n')
			}
		}
	}

	// --- 2. 活跃告警 ---
	if inc.HostID != "" && s.notifier != nil {
		var hostAlerts []string
		for _, a := range s.notifier.ActiveAlerts() {
			if a.HostID == inc.HostID && a.Status == "" {
				hostAlerts = append(hostAlerts, fmt.Sprintf("%s (%s, %.1f)", a.Type, a.Level, a.Value))
			}
		}
		if len(hostAlerts) > 0 {
			if len(hostAlerts) > 10 {
				hostAlerts = hostAlerts[:10]
			}
			b.WriteString("\n【当前活跃告警】\n  ")
			b.WriteString(strings.Join(hostAlerts, " · "))
			b.WriteByte('\n')
		}
	}

	// --- 3. 近期日志摘要 ---
	if inc.HostID != "" && s.logs != nil {
		logSince := time.Now().Unix() - 300 // last 5 minutes
		errs := s.logs.search(inc.HostID, "error", "", logSince, 5)
		warns := s.logs.search(inc.HostID, "warn", "", logSince, 5)
		if len(errs) > 0 || len(warns) > 0 {
			b.WriteString("\n【最近 5 分钟日志摘要】\n")
			for _, e := range errs {
				ts := time.Unix(e.Ts, 0).Format("15:04:05")
				fmt.Fprintf(&b, "  [%s ERROR] %s\n", ts, trimLine(e.Message, 200))
			}
			for _, e := range warns {
				ts := time.Unix(e.Ts, 0).Format("15:04:05")
				fmt.Fprintf(&b, "  [%s WARN]  %s\n", ts, trimLine(e.Message, 160))
			}
		} else {
			b.WriteString("\n【最近 5 分钟日志摘要】\n  无 error/warn 日志。\n")
		}
	}

	// --- 4. RAG 相似历史案例检索 ---
	if s.pg != nil && inc.HostID != "" {
		cfg := s.cfg.AIConfig()
		if cfg.Enabled && cfg.APIKey != "" {
			// Build a concise summary for embedding
			summaryText := fmt.Sprintf("事件：%s。告警类型：%s。严重程度：%s。", inc.Title, inc.Type, inc.Severity)
			if emb := embedText(cfg, summaryText); len(emb) > 0 {
				if cases, err := s.pg.searchSimilarCases(emb, 3); err == nil && len(cases) > 0 {
					b.WriteString("\n【📚 相似历史案例】（RAG 检索）\n")
					for i, c := range cases {
						sim := int((1.0 - c.Distance) * 100)
						if sim < 0 {
							sim = 0
						}
						fb := ""
						if c.Feedback == "helpful" {
							fb = " 👍"
						} else if c.Feedback == "unhelpful" {
							fb = " 👎"
						}
						fmt.Fprintf(&b, "  案例 %d（相似度 %d%%%s）：%s\n", i+1, sim, fb, trimLine(c.Summary, 250))
					}
				}
			}
		}
	}

	// --- 5. 经验规则匹配 ---
	if s.pg != nil {
		if rules, err := s.pg.listExperienceRules(); err == nil && len(rules) > 0 {
			var matched []string
			for _, r := range rules {
				if r.Pattern == "" {
					continue
				}
				// Try to match pattern against incident title, type, or log messages
				target := inc.Title + " " + inc.Type
				if strings.Contains(strings.ToLower(target), strings.ToLower(r.Pattern)) {
					matched = append(matched, fmt.Sprintf("  • %s（%s）→ %s", r.Pattern, r.Severity, r.Conclusion))
					if len(matched) >= 5 {
						break
					}
				}
			}
			if len(matched) > 0 {
				b.WriteString("\n【📋 匹配经验规则】\n")
				for _, m := range matched {
					b.WriteString(m + "\n")
				}
			}
		}
	}

	b.WriteString("\n你的任务：根据以上上下文，回答操作员的追问。请用简洁中文，给出具体可执行的排查方向或处置建议。如果信息不足，明确指出还需要什么信息。")
	return b.String()
}

// formatUptime converts uptime in seconds to a human-readable string.
func formatUptime(seconds uint64) string {
	d := seconds / 86400
	h := (seconds % 86400) / 3600
	m := (seconds % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("%dd%dh", d, h)
	}
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// buildTerminalSummary finds the most recent terminal session for the given host,
// splits the output frames into 30-second windows, and returns a human-readable
// timeline summary. This is 方案 A: 分段摘要注入 — the AI sees compact operation
// history without the full raw terminal dump.
func (s *Server) buildTerminalSummary(hostID string) string {
	if s.term == nil {
		return ""
	}
	sessions := s.term.findSessionsByHost(hostID)
	if len(sessions) == 0 {
		return ""
	}
	// Pick the newest session (already sorted newest-first)
	best := sessions[0]
	frames := s.term.getRecording(best.ID)
	if len(frames) == 0 {
		return ""
	}
	// Group output frames into 30-second windows
	type window struct {
		startTs int64
		lines   []string
	}
	const windowSec = 30
	var windows []window
	var cur *window
	for _, f := range frames {
		if f.Type != "output" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(f.Data)
		if err != nil || len(data) == 0 {
			continue
		}
		text := stripANSI(string(data))
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		sec := f.Ts / 1000 // convert ms to seconds
		if cur == nil || sec-cur.startTs >= windowSec {
			windows = append(windows, window{startTs: sec})
			cur = &windows[len(windows)-1]
		}
		// Append non-empty lines from this frame
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				cur.lines = append(cur.lines, line)
			}
		}
	}
	if len(windows) == 0 {
		return ""
	}
	// Build summary: one line per window, with the most informative output line
	var b strings.Builder
	b.WriteString(fmt.Sprintf("主机 %s 最近一次终端会话（操作人：%s，%s）：\n",
		best.Hostname, best.Operator, time.Unix(best.CreatedAt, 0).Format("2006-01-02 15:04:05")))
	maxWindows := 20
	if len(windows) > maxWindows {
		windows = windows[len(windows)-maxWindows:] // keep the most recent
	}
	for _, w := range windows {
		ts := time.Unix(w.startTs, 0).Format("15:04:05")
		// Pick the first 2-3 meaningful lines as summary
		summary := ""
		for j, line := range w.lines {
			if j >= 3 {
				break
			}
			if len(line) > 150 {
				line = line[:150] + "…"
			}
			if summary != "" {
				summary += " | "
			}
			summary += line
		}
		if summary == "" {
			continue
		}
		fmt.Fprintf(&b, "  [%s] %s\n", ts, summary)
	}
	return b.String()
}

// stripANSI removes ANSI escape sequences and control characters from terminal
// output, leaving only printable text.
func stripANSI(s string) string {
	// Remove ANSI CSI sequences: ESC[ ... m (SGR), ESC[ ... J, ESC[ ... K, etc.
	var b strings.Builder
	b.Grow(len(s))
	inEscape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0x1b { // ESC
			inEscape = true
			continue
		}
		if inEscape {
			// CSI sequences end with a letter (m, J, K, H, etc.)
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				inEscape = false
			}
			continue
		}
		// Skip other control characters (except newline and tab)
		if c < 0x20 && c != '\n' && c != '\t' {
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// diagnosisChatMessage is a single turn in an incident diagnosis conversation.
type diagnosisChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Ts      int64  `json:"ts"`
}

// saveDiagnosisChatTurn persists a chat turn to PostgreSQL via kv_state so the
// conversation history survives restarts and accumulates over time.
func (s *Server) saveDiagnosisChatTurn(incidentID int64, userMsg, aiReply string) {
	if s.pg == nil {
		return
	}
	key := fmt.Sprintf("ai_diag_chat_%d", incidentID)
	now := time.Now().Unix()
	// Load existing history
	var history []diagnosisChatMessage
	if raw, _ := s.pg.loadKV(key); raw != nil {
		_ = json.Unmarshal(raw, &history)
	}
	history = append(history,
		diagnosisChatMessage{Role: "user", Content: userMsg, Ts: now},
		diagnosisChatMessage{Role: "assistant", Content: aiReply, Ts: now},
	)
	// Cap at 100 messages (50 turns) to avoid unbounded growth
	if len(history) > 100 {
		history = history[len(history)-100:]
	}
	raw, _ := json.Marshal(history)
	_ = s.pg.saveKV(key, raw)
}

// handleGetDiagnosisChatHistory returns the persisted chat history for an incident.
// GET /api/v1/incidents/{id}/diagnose-chat
func (s *Server) handleGetDiagnosisChatHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := sreParseID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	var history []diagnosisChatMessage
	if s.pg != nil {
		key := fmt.Sprintf("ai_diag_chat_%d", id)
		if raw, _ := s.pg.loadKV(key); raw != nil {
			_ = json.Unmarshal(raw, &history)
		}
	}
	if history == nil {
		history = []diagnosisChatMessage{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": history})
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

// saveDiagnosisEmbedding generates a vector embedding for the diagnosis summary
// and stores it in PG for future RAG retrieval. Runs async (best-effort, non-blocking).
func (s *Server) saveDiagnosisEmbedding(incidentID int64, inc Incident, reply string) {
	if s.pg == nil {
		return
	}
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.APIKey == "" {
		return
	}
	// Build a concise summary from the incident + diagnosis for embedding
	summary := fmt.Sprintf("事件：%s。告警类型：%s。严重程度：%s。诊断：%s",
		inc.Title, inc.Type, inc.Severity, trimLine(reply, 500))
	emb := embedText(cfg, summary)
	if len(emb) == 0 {
		return
	}
	if _, err := s.pg.insertDiagnosisEmbedding(incidentID, emb, summary, inc.Severity, inc.Type); err != nil {
		slog.Warn("保存诊断向量失败", "incident", incidentID, "err", err)
	}
}

// memoryJob 是一个待向量化并入库的 AI 记忆任务。
type memoryJob struct {
	kind    string
	source  string
	content string
}

// rememberAI 把一段 AI 相关文本（对话 / 文件 / URL / 多轮历史）推入异步写入队列，
// 由后台 worker pool 完成向量化 + 去重 + 入库。非阻塞，队列满时静默丢弃。
// 无 pgvector 或未配置嵌入时静默跳过。
func (s *Server) rememberAI(kind, source, content string) {
	if s.pg == nil || s.memoryCh == nil {
		return
	}
	content = strings.TrimSpace(content)
	if len([]rune(content)) < 12 { // 太短，无检索价值
		return
	}
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.APIKey == "" {
		return
	}
	// 非阻塞入队：队列满时丢弃，避免突发流量打爆内存
	select {
	case s.memoryCh <- memoryJob{kind: kind, source: source, content: content}:
	default:
		slog.Warn("AI 记忆队列已满，丢弃本次写入", "kind", kind)
	}
}

// startMemoryWorkers 启动 3 个后台 worker，从 memoryCh 批量拉取记忆任务，
// 通过 semaphore 控制并发（最多 3 个同时调用 Embedding API），失败重试一次后静默丢弃。
func (s *Server) startMemoryWorkers() {
	const workerCount = 3
	for i := 0; i < workerCount; i++ {
		s.memoryWg.Add(1)
		go func() {
			defer s.memoryWg.Done()
			for job := range s.memoryCh {
				s.processMemoryJob(job)
			}
		}()
	}
}

// processMemoryJob 执行单条记忆任务的向量化 + 去重 + 入库。
// 通过 memorySem 信号量限制并发，防止突发大量写入导致 API 限流。
// 增强：
//   - 超过 2000 字符的内容在入库前生成 AI 摘要（仅 chat/terminal kind）
//   - 去重阈值 0.12 cosine distance，重复时合并而非丢弃
func (s *Server) processMemoryJob(job memoryJob) {
	// 获取信号量（并发上限保护）
	s.memorySem <- struct{}{}
	defer func() { <-s.memorySem }()

	content := job.content

	// 记忆摘要压缩：超过 2000 字符的 chat/terminal 内容生成 AI 摘要
	if (job.kind == "chat" || job.kind == "terminal") && len([]rune(content)) > 2000 {
		content = s.generateMemorySummary(job.kind, content)
	}

	if len([]rune(content)) > 8000 { // 存储正文限长（~8000字符 ≈ 4000 token）
		content = string([]rune(content)[:8000]) + "…"
	}
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.APIKey == "" {
		return
	}
	emb := embedText(cfg, content)
	if len(emb) == 0 {
		return
	}
	// 去重检查：余弦距离 < 0.12（相似度 > 88%）视为重复，合并而非丢弃
	if dup, dupID, _ := s.pg.hasDuplicateMemory(emb, job.kind); dup {
		// 合并逻辑：将新内容追加到已有记忆
		appendContent := content
		if len([]rune(appendContent)) > 500 { // 合并时只取摘要部分
			appendContent = string([]rune(appendContent)[:500]) + "…"
		}
		if err := s.pg.mergeDuplicateMemory(dupID, appendContent, emb); err != nil {
			slog.Debug("AI 记忆合并失败，回退为跳过", "kind", job.kind, "err", err)
		} else {
			slog.Debug("AI 记忆重复，已合并到已有记录", "kind", job.kind, "source", job.source, "dup_id", dupID)
		}
		return
	}
	if err := s.pg.insertMemoryEmbedding(job.kind, job.source, content, emb, time.Now().Unix()); err != nil {
		slog.Warn("保存 AI 记忆向量失败", "kind", job.kind, "err", err)
	}
}

// generateMemorySummary 对长文本生成 200 字 AI 摘要，格式为「摘要 + 原文截断」。
// 仅对 chat 和 terminal kind 做摘要压缩，diagnosis 和 alert 保持原文。
func (s *Server) generateMemorySummary(kind, content string) string {
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.APIKey == "" {
		// AI 未配置，直接截断
		return string([]rune(content)[:2000]) + "…"
	}
	// 调用 AI 生成摘要
	msgs := []map[string]string{
		{"role": "system", "content": "用不超过200字概括以下运维" + kind + "内容的核心知识点，保留关键指标、结论和建议。直接输出摘要文本，不要加任何格式标记。"},
		{"role": "user", "content": content},
	}
	summary, err := aiChat(cfg, msgs)
	if err != nil || strings.TrimSpace(summary) == "" {
		return string([]rune(content)[:2000]) + "…"
	}
	summary = strings.TrimSpace(summary)
	// 格式：摘要在前 + 原文截断保留
	truncated := content
	if len([]rune(truncated)) > 4000 {
		truncated = string([]rune(truncated)[:4000]) + "…"
	}
	return "【摘要】" + summary + "\n【原文】" + truncated
}

// retrieveMemoryForPrompt 根据用户当前消息检索语义最相关的 Top-K 历史记忆，
// 返回可拼入 system prompt 的文本片段。无 pg / 无 embedding 配置时返回空串。
// preferKind 指定优先召回的记忆类型（如 "chat"、"diagnosis"），
// 检索策略为 2/3 优先 kind + 1/3 其他 kind，兼顾场景相关性与跨域知识。
func (s *Server) retrieveMemoryForPrompt(preferKind, userMsg string, topK int) string {
	if s.pg == nil {
		return ""
	}
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.APIKey == "" {
		return ""
	}
	if topK <= 0 {
		topK = 8
	}
	// 对用户消息做向量化（截断至 8000 字符，与 embedText 保持一致）
	query := userMsg
	if len([]rune(query)) > 8000 {
		query = string([]rune(query)[:8000])
	}
	emb := embedText(cfg, query)
	if len(emb) == 0 {
		return ""
	}
	// 配置了 rerank 时按 3×K 过取候选，交给 rerank 精排 —— 向量召回负责“找全”，rerank 负责“排准”。
	fetch := topK
	if _, _, _, ok := rerankConfig(cfg); ok {
		fetch = topK * 3
	}
	hits, err := s.pg.searchMemoryByKind(emb, preferKind, fetch)
	if err != nil || len(hits) == 0 {
		return ""
	}
	// rerank 精排：失败 / 未配置时静默回退到原向量顺序，仅取前 topK。
	if len(hits) > topK {
		docs := make([]string, len(hits))
		for i, h := range hits {
			docs[i] = h.Content
		}
		if order := rerankDocuments(cfg, query, docs, topK); len(order) > 0 {
			reordered := make([]memoryHit, 0, len(order))
			for _, i := range order {
				reordered = append(reordered, hits[i])
			}
			hits = reordered
		}
	}
	// 异步更新命中记忆的 last_hit_at（用于衰减策略判断）
	go func() {
		ids := make([]int64, len(hits))
		for i, h := range hits {
			ids[i] = h.ID
		}
		s.pg.touchMemoryHits(ids)
	}()
	var b strings.Builder
	b.WriteString("\n\n【历史记忆参考（RAG 检索，仅供参考，以实际数据为准）】\n")
	for i, h := range hits {
		if i >= topK {
			break
		}
		// 截断过长内容，避免撑爆 prompt token
		content := h.Content
		if len([]rune(content)) > 1500 {
			content = string([]rune(content)[:1500]) + "…"
		}
		fmt.Fprintf(&b, "[%d] (%s) %s\n", i+1, h.Kind, content)
	}
	return b.String()
}

// handleDiagnosisFeedback records user feedback on an AI diagnosis.
// POST /api/v1/incidents/{id}/diagnosis-feedback  {message_index, helpful}
func (s *Server) handleDiagnosisFeedback(w http.ResponseWriter, r *http.Request) {
	id, ok := sreParseID(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	var req struct {
		MessageIndex int  `json:"message_index"`
		Helpful      bool `json:"helpful"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	fb := "unhelpful"
	if req.Helpful {
		fb = "helpful"
	}
	if s.pg != nil {
		if err := s.pg.updateDiagnosisFeedback(id, fb); err != nil {
			slog.Warn("保存诊断反馈失败", "incident", id, "err", err)
		}
	}
	// 学习闭环：👍/👎 同步强化/惩罚该事件的诊断记忆（ai_memory），与 diagnosis_embeddings 双库一致，
	// 让反馈既影响相似案例检索、也影响通用记忆检索。
	factor := reinforceHelpful
	if !req.Helpful {
		factor = penalizeUnhelpful
	}
	s.reinforceMemoryBySource("diagnosis", fmt.Sprintf("incident:%d", id), factor)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleListExperienceRules returns all experience rules.
// GET /api/v1/experience-rules
func (s *Server) handleListExperienceRules(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusOK, []experienceRule{})
		return
	}
	rules, err := s.pg.listExperienceRules()
	if err != nil {
		writeJSON(w, http.StatusOK, []experienceRule{})
		return
	}
	if rules == nil {
		rules = []experienceRule{}
	}
	writeJSON(w, http.StatusOK, rules)
}

// handleCreateExperienceRule creates a new experience rule.
// POST /api/v1/experience-rules  {pattern, conclusion, severity, incident_id}
func (s *Server) handleCreateExperienceRule(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PostgreSQL 未配置"})
		return
	}
	var req experienceRule
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Pattern == "" || req.Conclusion == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pattern 和 conclusion 为必填项"})
		return
	}
	id, err := s.pg.insertExperienceRule(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "ok"})
}

// handleDeleteExperienceRule deletes an experience rule by ID.
// DELETE /api/v1/experience-rules/{id}
func (s *Server) handleDeleteExperienceRule(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PostgreSQL 未配置"})
		return
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	if err := s.pg.deleteExperienceRule(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ============================================================================
// Sreyun Agent — 自主运维 Agent 对话 + 规则/模板管理
// ============================================================================

// handleSreyunChat provides multi-turn Sreyun Agent conversation with
// Function Calling support. Supports SSE streaming via stream=true.
// POST /api/v1/hermes/chat
func (s *Server) handleSreyunChat(w http.ResponseWriter, r *http.Request) {
	if s.sreyun == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Sreyun Agent 未启用"})
		return
	}
	var req struct {
		Message    string              `json:"message"`
		SessionID  int64               `json:"session_id,omitempty"`
		IncidentID int64               `json:"incident_id,omitempty"`
		History    []map[string]string `json:"history,omitempty"`
		Images     []struct {
			MIME string `json:"mime"`
			Data string `json:"data"` // base64（不含 data: 前缀）
		} `json:"images,omitempty"`
		Files []struct {
			Name string `json:"name"`
			Text string `json:"text"`
		} `json:"files,omitempty"`
		Stream bool `json:"stream,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if strings.TrimSpace(req.Message) == "" && len(req.Images) == 0 && len(req.Files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "消息不能为空"})
		return
	}
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.Endpoint == "" || cfg.Model == "" {
		// 统一 AI 对话走 SSE：未启用时也发 SSE 错误帧，前端才能正确显示。
		s.setupSSE(w)
		fmt.Fprint(w, "data: {\"error\":\"AI 未配置或未启用，请先在「AI 设置」填写 Endpoint / Key / 模型并勾选启用后保存\"}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
		return
	}
	// 展开上传的文本文件到消息上下文（对所有模型有效）；图片走多模态（需视觉模型）
	msg := req.Message
	for _, f := range req.Files {
		txt := strings.TrimSpace(f.Text)
		if txt == "" {
			continue
		}
		if len([]rune(txt)) > 8000 { // 限制单文件注入长度，避免上下文爆炸
			txt = string([]rune(txt)[:8000]) + "\n…（文件过长，已截断）"
		}
		name := f.Name
		if name == "" {
			name = "附件"
		}
		msg += fmt.Sprintf("\n\n【用户上传的文件：%s】\n%s", name, txt)
	}
	if strings.TrimSpace(msg) == "" {
		msg = "（用户上传了图片，请查看并分析）"
	}
	var images []chatImage
	for _, im := range req.Images {
		if strings.TrimSpace(im.Data) == "" {
			continue
		}
		images = append(images, chatImage{MIME: im.MIME, Data: im.Data})
		if len(images) >= 4 { // 最多 4 张，控制上下文与成本
			break
		}
	}
	// 按 session_id 解析会话（多轮记忆 / 刷新恢复），前端 history 作为兜底
	session := s.sreyun.resolveSession(req.SessionID, req.History)
	session.IncidentID = req.IncidentID
	// 统一 AI 对话默认走 SSE 流式；传入请求 ctx，客户端断开时可及时中止工具循环
	s.setupSSE(w)
	// 立即 Flush，确保 SSE 响应头到达客户端，前端开始显示「思考中」动画；
	// 后续 Chat() 内的 RAG 检索（embedText + PG 查询）不会阻塞首屏。
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	final, _ := s.sreyun.Chat(r.Context(), session, msg, images, true, w)
	// 向量化本轮交互 → 永久入库沉淀为 RAG 记忆（对话 + 上传文件 / URL 正文均含在 msg 内；
	// 附件再各存一条便于精确召回）。多轮历史随每轮持续累积。异步、尽力而为、不阻塞响应。
	if strings.TrimSpace(final) != "" {
		go s.rememberAI("chat", fmt.Sprintf("session:%d", session.ID), "【用户】\n"+msg+"\n\n【AI】\n"+final)
	}
	for _, f := range req.Files {
		if strings.TrimSpace(f.Text) != "" {
			go s.rememberAI("file", f.Name, f.Text)
		}
	}
	// 回传（可能新建的）会话 id，供前端延续多轮对话 & 刷新后恢复；随后统一发送 [DONE]
	fmt.Fprintf(w, "data: {\"session_id\":%d}\n\n", session.ID)
	fmt.Fprint(w, "data: [DONE]\n\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// handleSreyunSessions lists recent Sreyun sessions.
// GET /api/v1/hermes/sessions
func (s *Server) handleSreyunSessions(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	sessions, err := s.pg.listSreyunSessions(20)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if sessions == nil {
		sessions = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, sessions)
}

// handleSreyunSession loads a single Sreyun session.
// GET /api/v1/hermes/sessions/{id}
func (s *Server) handleSreyunSession(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "PostgreSQL 未配置"})
		return
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	raw, err := s.pg.loadSreyunSession(id)
	if err != nil || raw == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	var msgs []map[string]string
	if err := json.Unmarshal(raw, &msgs); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "messages": msgs})
}

// handleSreyunSessionUndo 撤销会话最后一轮问答（删除末尾 assistant + user 各一条），
// 供前端「撤销」修正上次提问后重试。POST /api/v1/hermes/sessions/{id}/undo
func (s *Server) handleSreyunSessionUndo(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "messages": []any{}})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	raw, err := s.pg.loadSreyunSession(id)
	if err != nil || raw == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	var msgs []map[string]string
	if json.Unmarshal(raw, &msgs) != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "会话数据损坏"})
		return
	}
	if n := len(msgs); n > 0 && msgs[n-1]["role"] == "assistant" {
		msgs = msgs[:n-1]
	}
	if n := len(msgs); n > 0 && msgs[n-1]["role"] == "user" {
		msgs = msgs[:n-1]
	}
	out, _ := json.Marshal(msgs)
	if _, err := s.pg.saveSreyunSession(id, out, 0); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "messages": msgs})
}

// handleSreyunListRules returns all Sreyun rules.
// GET /api/v1/hermes/rules
func (s *Server) handleSreyunListRules(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	rules, err := s.pg.listSreyunRules()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if rules == nil {
		rules = []sreyunRule{}
	}
	writeJSON(w, http.StatusOK, rules)
}

// handleSreyunUpsertRule creates or updates a Sreyun rule.
// POST /api/v1/hermes/rules
func (s *Server) handleSreyunUpsertRule(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PostgreSQL 未配置"})
		return
	}
	var rule sreyunRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if rule.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "规则名称不能为空"})
		return
	}
	id, err := s.pg.upsertSreyunRule(rule)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// Trigger hot-reload
	if s.sreyun != nil {
		s.sreyun.reloadConfig()
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "ok"})
}

// handleSreyunDeleteRule deletes a Sreyun rule.
// DELETE /api/v1/hermes/rules/{id}
func (s *Server) handleSreyunDeleteRule(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PostgreSQL 未配置"})
		return
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	if err := s.pg.deleteSreyunRule(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if s.sreyun != nil {
		s.sreyun.reloadConfig()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleSreyunListTemplates returns all Sreyun templates.
// GET /api/v1/hermes/templates
func (s *Server) handleSreyunListTemplates(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	tmpls, err := s.pg.listSreyunTemplates(false)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if tmpls == nil {
		tmpls = []sreyunTemplate{}
	}
	writeJSON(w, http.StatusOK, tmpls)
}

// handleSreyunUpsertTemplate creates or updates a Sreyun template.
// POST /api/v1/hermes/templates
func (s *Server) handleSreyunUpsertTemplate(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PostgreSQL 未配置"})
		return
	}
	var tmpl sreyunTemplate
	if err := json.NewDecoder(r.Body).Decode(&tmpl); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if tmpl.Name == "" || tmpl.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "模板名称和内容不能为空"})
		return
	}
	id, err := s.pg.upsertSreyunTemplate(tmpl)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if s.sreyun != nil {
		s.sreyun.reloadConfig()
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "ok"})
}

// handleSreyunDeleteTemplate deletes a Sreyun template.
// DELETE /api/v1/hermes/templates/{id}
func (s *Server) handleSreyunDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PostgreSQL 未配置"})
		return
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_id")})
		return
	}
	if err := s.pg.deleteSreyunTemplate(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if s.sreyun != nil {
		s.sreyun.reloadConfig()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
