package main

import (
	"bytes"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"aiops-monitor/shared"
)

// validAgentToken constant-time compares an agent-presented token against the
// install token (never matches an empty configured token).
func validAgentToken(got, want string) bool {
	return want != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

//go:embed web
var webFS embed.FS

// Server wires the store, the operator-editable config and the notifier to
// HTTP handlers.
type Server struct {
	store     *Store
	cfg       *ConfigStore
	notifier  *Notifier
	auth      *Auth
	checks    *checkRunner
	term      *termManager  // remote terminal relay
	emailMgr  *emailManager // verification codes + reset tokens
	playbooks *playbookManager // automation playbooks + execution history
	distDir   string        // directory of downloadable agent binaries + plugins.zip
}

func NewServer(store *Store, cfg *ConfigStore, notifier *Notifier, distDir string, selfAddr string) *Server {
	return &Server{
		store: store, cfg: cfg, notifier: notifier, distDir: distDir,
		auth:     NewAuth(cfg),
		checks:   newCheckRunner(cfg, store, notifier, selfAddr),
		term:     newTermManager(),
		emailMgr: newEmailManager(),
		playbooks: newPlaybookManager(cfg),
	}
}

// Routes builds the HTTP handler using Go 1.22 method+path patterns.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/agent/register", s.handleRegister)
	mux.HandleFunc("POST /api/v1/agent/report", s.handleReport)
	// remote terminal: browser WebSocket (auth) + agent reverse streams (token)
	mux.HandleFunc("GET /api/v1/hosts/{id}/terminal", s.handleTerminal)
	mux.HandleFunc("GET /api/v1/agent/terminal/wait", s.handleAgentTermWait)
	mux.HandleFunc("GET /api/v1/agent/terminal/rx", s.handleAgentTermRx)
	mux.HandleFunc("POST /api/v1/agent/terminal/tx", s.handleAgentTermTx)
	mux.HandleFunc("GET /api/v1/hosts", s.handleHosts)
	mux.HandleFunc("GET /api/v1/hosts/{id}/metrics", s.handleHostMetrics)
	mux.HandleFunc("GET /api/v1/hosts/{id}/history", s.handleHostHistory)
	mux.HandleFunc("POST /api/v1/hosts/{id}/category", s.handleSetCategory)
	mux.HandleFunc("DELETE /api/v1/hosts/{id}", s.handleDeleteHost)
	mux.HandleFunc("GET /api/v1/alerts", s.handleAlerts)
	mux.HandleFunc("GET /api/v1/events", s.handleEvents)
	mux.HandleFunc("GET /api/v1/activity", s.handleActivity)
	mux.HandleFunc("GET /api/v1/summary", s.handleSummary)
	mux.HandleFunc("GET /api/v1/config", s.handleGetConfig)
	mux.HandleFunc("POST /api/v1/config", s.handleSetConfig)
	mux.HandleFunc("POST /api/v1/config/test", s.handleTestConfig)
	mux.HandleFunc("POST /api/v1/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/logout", s.handleLogout)
	mux.HandleFunc("GET /api/v1/me", s.handleMe)
	mux.HandleFunc("POST /api/v1/profile", s.handleSetProfile)
	mux.HandleFunc("POST /api/v1/password", s.handleSetPassword)
	mux.HandleFunc("POST /api/v1/mfa/setup", s.handleMFASetup)
	mux.HandleFunc("POST /api/v1/mfa/enable", s.handleMFAEnable)
	mux.HandleFunc("POST /api/v1/mfa/disable", s.handleMFADisable)
	mux.HandleFunc("POST /api/v1/mfa/unbind-via-email", s.handleMFAUnbindViaEmail)
	// Account recovery: public endpoints (no session required)
	mux.HandleFunc("POST /api/v1/account/recover-username", s.handleRecoverUsername)
	mux.HandleFunc("POST /api/v1/account/send-reset-code", s.handleSendResetCode)
	mux.HandleFunc("POST /api/v1/account/reset-password", s.handleResetPassword)
	// user management (RBAC; admin-only, enforced by routeAllowed)
	mux.HandleFunc("GET /api/v1/users", s.handleListUsers)
	mux.HandleFunc("POST /api/v1/users", s.handleCreateUser)
	mux.HandleFunc("POST /api/v1/users/{username}", s.handleUpdateUser)
	mux.HandleFunc("DELETE /api/v1/users/{username}", s.handleDeleteUser)
	mux.HandleFunc("POST /api/v1/users/{username}/reset-password", s.handleResetUserPassword)
	mux.HandleFunc("POST /api/v1/users/{username}/reset-mfa", s.handleResetUserMFA)
	mux.HandleFunc("GET /api/v1/checks", s.handleGetChecks)
	mux.HandleFunc("POST /api/v1/checks", s.handleUpsertCheck)
	mux.HandleFunc("POST /api/v1/checks/{id}/run", s.handleRunCheck)
	mux.HandleFunc("GET /api/v1/checks/{id}/history", s.handleCheckHistory)
	mux.HandleFunc("DELETE /api/v1/checks/{id}", s.handleDeleteCheck)
	// Playbooks (automation)
	mux.HandleFunc("GET /api/v1/playbooks", s.handleListPlaybooks)
	mux.HandleFunc("POST /api/v1/playbooks", s.handleUpsertPlaybook)
	mux.HandleFunc("DELETE /api/v1/playbooks/{id}", s.handleDeletePlaybook)
	mux.HandleFunc("POST /api/v1/playbooks/{id}/execute", s.handleExecutePlaybook)
	mux.HandleFunc("GET /api/v1/playbooks/executions", s.handleListExecutions)
	mux.HandleFunc("GET /api/v1/playbooks/executions/{id}", s.handleGetExecution)
	// Terminal enhancements
	mux.HandleFunc("GET /api/v1/terminal/sessions", s.handleListTerminalSessions)
	mux.HandleFunc("GET /api/v1/terminal/sessions/{id}/replay", s.handleTerminalReplay)
	mux.HandleFunc("GET /api/v1/terminal/sessions/{id}/observe", s.handleTerminalObserve)
	mux.HandleFunc("GET /api/v1/hosts/meta", s.handleHostsMeta)
	mux.HandleFunc("GET /api/v1/install/info", s.handleInstallInfo)
	mux.HandleFunc("POST /api/v1/install/reset-token", s.handleResetToken)
	mux.HandleFunc("GET /install.sh", s.handleInstallScript)
	mux.HandleFunc("GET /install.ps1", s.handleInstallScript)
	mux.HandleFunc("GET /uninstall.sh", s.handleUninstallScript)
	mux.HandleFunc("GET /uninstall.ps1", s.handleUninstallScript)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /", s.handleDashboard)
	// static assets (split css/js) served straight from the embedded web/ dir
	if sub, err := fs.Sub(webFS, "web"); err == nil {
		fsrv := http.FileServer(http.FS(sub))
		mux.Handle("GET /style.css", fsrv)
		mux.Handle("GET /app.js", fsrv)
		mux.Handle("GET /manifest.json", fsrv)
		mux.Handle("GET /icon.svg", fsrv)
		// Service Worker: needs Service-Worker-Allowed header for root scope control
		mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			w.Header().Set("Service-Worker-Allowed", "/")
			w.Header().Set("Cache-Control", "no-cache")
			data, err := webFS.ReadFile("web/sw.js")
			if err != nil {
				http.Error(w, "not found", 404)
				return
			}
			w.Write(data)
		})
	}
	// agent binaries + plugins.zip for the one-line install command
	if s.distDir != "" {
		mux.Handle("GET /dl/", http.StripPrefix("/dl/", http.FileServer(http.Dir(s.distDir))))
	}
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HostID   string `json:"host_id"`
		Hostname string `json:"hostname"`
		Token    string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if s.cfg.AgentTokenRequired() && !validAgentToken(req.Token, s.cfg.InstallToken()) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "无效或缺失的接入 Token"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"host_id":          req.HostID,
		"server_time_unix": time.Now().Unix(),
	})
}

// handleReport ingests a metrics report (base + custom + events) from an agent.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	var rep shared.Report
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if rep.HostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host_id required"})
		return
	}
	if s.cfg.AgentTokenRequired() && !validAgentToken(rep.Token, s.cfg.InstallToken()) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "无效或缺失的接入 Token"})
		return
	}
	h := s.store.Upsert(rep)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "host_id": h.ID})
}

func (s *Server) handleHosts(w http.ResponseWriter, r *http.Request) {
	hosts := s.store.ListHosts()
	now := time.Now().Unix()
	offline := int64(s.cfg.Thresholds().OfflineAfter.Seconds())

	type hostView struct {
		*Host
		Online bool `json:"online"`
	}
	views := make([]hostView, 0, len(hosts))
	for _, h := range hosts {
		if cat, ok := s.cfg.CategoryOverride(h.ID); ok {
			h.Category = cat // manual override wins over the agent-reported category
		}
		views = append(views, hostView{Host: h, Online: now-h.LastSeen <= offline})
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].Category != views[j].Category {
			return views[i].Category < views[j].Category
		}
		return views[i].Hostname < views[j].Hostname
	})
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) handleHostMetrics(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	samples, ok := s.store.GetSamples(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "host not found"})
		return
	}
	writeJSON(w, http.StatusOK, samples)
}

// handleHostHistory returns time-series data for a host within [from, to] range.
// Query params: from (unix timestamp), to (unix timestamp).
// Defaults: from = now - 24h, to = now.
func (s *Server) handleHostHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	now := time.Now().Unix()

	// Parse query parameters
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	var from, to int64
	if toStr != "" {
		var err error
		to, err = strconv.ParseInt(toStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid 'to' parameter"})
			return
		}
	} else {
		to = now
	}

	if fromStr != "" {
		var err error
		from, err = strconv.ParseInt(fromStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid 'from' parameter"})
			return
		}
	} else {
		from = now - 86400 // default: last 24 hours
	}

	if from >= to {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "'from' must be less than 'to'"})
		return
	}

	samples, ok := s.store.GetHistory(id, from, to)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "host not found"})
		return
	}

	writeJSON(w, http.StatusOK, samples)
}

// handleSetCategory sets (or clears, when empty) a manual category override.
func (s *Server) handleSetCategory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Category string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	cat := strings.TrimSpace(req.Category)
	_ = s.cfg.SetCategory(id, cat)
	msg := "设置主机分类：" + shortID(id) + " → " + cat
	if cat == "" {
		msg = "清除主机分类：" + shortID(id)
	}
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: s.clientIP(r), Message: msg})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "host_id": id, "category": cat})
}

func (s *Server) handleDeleteHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok := s.store.DeleteHost(id)
	_ = s.cfg.SetCategory(id, "") // drop any override for the removed host
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "host not found"})
		return
	}
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "删除主机 " + shortID(id)})
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted", "host_id": id})
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	alerts := Evaluate(s.store.ListHosts(), s.cfg.Thresholds())
	// stamp threshold alerts with their first-fired time (check alerts carry it already)
	since := s.notifier.ActiveSince()
	for i := range alerts {
		if t, ok := since[alertKey(alerts[i])]; ok {
			alerts[i].Since = t
		}
	}
	alerts = append(alerts, s.checks.DownAlerts()...)
	if alerts == nil {
		alerts = []Alert{}
	}
	writeJSON(w, http.StatusOK, alerts)
}

// handleEvents returns recent plugin-generated events (the Python/AI layer's findings).
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	events := s.store.RecentEvents()
	if events == nil {
		events = []storedEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

// handleActivity returns the unified activity log (operations + system + plugin).
func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	items := s.store.RecentActivity()
	if items == nil {
		items = []LogEntry{}
	}
	writeJSON(w, http.StatusOK, items)
}

// ---- custom checks ----

func (s *Server) handleGetChecks(w http.ResponseWriter, r *http.Request) {
	checks := s.cfg.Checks()
	st := s.checks.snapshot()
	out := make([]map[string]any, 0, len(checks)+1)

	// Built-in self health-check is always first
	selfEntry := map[string]any{
		"id": selfCheckID, "name": SelfCheckName, "type": "http",
		"target": "http://127.0.0.1:" + portFromAddr(s.checks.selfAddr) + "/healthz",
		"interval_sec": 30, "level": "critical", "enabled": true,
		"ok": true, "message": "", "checked_at": int64(0), "latency_ms": 0.0,
		"builtin": true,
	}
	if s2, ok := st[selfCheckID]; ok {
		selfEntry["ok"], selfEntry["message"], selfEntry["checked_at"], selfEntry["latency_ms"] = s2.OK, s2.Message, s2.CheckedAt, s2.LatencyMs
	}
	out = append(out, selfEntry)

	for _, c := range checks {
		m := map[string]any{
			"id": c.ID, "name": c.Name, "type": c.Type, "target": c.Target,
			"interval_sec": c.IntervalSec, "level": c.Level, "enabled": c.Enabled,
			"ok": true, "message": "", "checked_at": int64(0), "latency_ms": 0.0,
			"status_code": 0, "cert_days": -1, "loss_pct": -1.0,
		}
		if s2, ok := st[c.ID]; ok {
			m["ok"], m["message"], m["checked_at"], m["latency_ms"] = s2.OK, s2.Message, s2.CheckedAt, s2.LatencyMs
			m["status_code"], m["cert_days"], m["loss_pct"] = s2.StatusCode, s2.CertDays, s2.LossPct
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUpsertCheck(w http.ResponseWriter, r *http.Request) {
	var c CustomCheck
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	c.Name = strings.TrimSpace(c.Name)
	c.Target = strings.TrimSpace(c.Target)
	if c.Name == "" || c.Target == "" || (c.Type != "http" && c.Type != "tcp" && c.Type != "ping" && c.Type != "process") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "名称 / 目标 / 类型不合法"})
		return
	}
	if c.IntervalSec < 5 {
		c.IntervalSec = 30
	}
	if c.Level != "warning" && c.Level != "critical" {
		c.Level = "critical"
	}
	saved, err := s.cfg.UpsertCheck(c)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.checks.runNow(saved.ID)
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: s.clientIP(r), Message: "保存自定义监控：" + saved.Name})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID})
}

// handleRunCheck triggers one immediate probe of a check (fire-and-forget).
func (s *Server) handleRunCheck(w http.ResponseWriter, r *http.Request) {
	s.checks.runNow(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleCheckHistory returns a check's recorded trend series (latency / status /
// loss over time) for the history-curve view.
func (s *Server) handleCheckHistory(w http.ResponseWriter, r *http.Request) {
	pts := s.checks.HistoryOf(r.PathValue("id"))
	if pts == nil {
		pts = []CheckPoint{}
	}
	writeJSON(w, http.StatusOK, pts)
}

func (s *Server) handleDeleteCheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = s.cfg.DeleteCheck(id)
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "删除自定义监控 " + id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleHostsMeta returns minimal host info (id + hostname) for the process-check UI.
func (s *Server) handleHostsMeta(w http.ResponseWriter, r *http.Request) {
	hosts := s.store.ListHosts()
	type hostMeta struct {
		ID       string `json:"id"`
		Hostname string `json:"hostname"`
	}
	out := make([]hostMeta, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, hostMeta{ID: h.ID, Hostname: h.Hostname})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	hosts := s.store.ListHosts()
	now := time.Now().Unix()
	th := s.cfg.Thresholds()
	offline := int64(th.OfflineAfter.Seconds())

	online := 0
	for _, h := range hosts {
		if now-h.LastSeen <= offline {
			online++
		}
	}
	crit, warn := 0, 0
	for _, a := range append(Evaluate(hosts, th), s.checks.DownAlerts()...) {
		if a.Level == "critical" {
			crit++
		} else {
			warn++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_hosts":      len(hosts),
		"online_hosts":     online,
		"offline_hosts":    len(hosts) - online,
		"critical_alerts":  crit,
		"warning_alerts":   warn,
		"plugin_events":    len(s.store.RecentEvents()),
		"server_time_unix": now,
		"version":          appVersion,
		"terminal_enabled": s.cfg.TerminalEnabled(),
	})
}

// handleGetConfig returns the alert config with webhooks/secrets masked.
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	c := s.cfg.Get()
	c.Categories = nil
	c.Feishu.Webhook = maskSecret(c.Feishu.Webhook)
	c.Dingtalk.Webhook = maskSecret(c.Dingtalk.Webhook)
	c.Dingtalk.Secret = maskSecret(c.Dingtalk.Secret)
	c.SMTP.Password = maskSecret(c.SMTP.Password)
	// Never expose the password hash/salt or the MFA secret to the browser.
	c.Account.Salt, c.Account.Hash, c.Account.MFASecret = "", "", ""
	c.Users = nil // the user list (with hashes) is served via /api/v1/users, not here
	writeJSON(w, http.StatusOK, c)
}

// ---- user management (admin-only; enforced by routeAllowed) ----

// userView is the browser-safe projection of an account (no salt/hash/secret).
func userView(u AccountConfig) map[string]any {
	return map[string]any{
		"username": u.Username, "display_name": u.DisplayName,
		"email": u.Email, "role": u.Role, "mfa_enabled": u.MFAEnabled,
	}
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users := s.cfg.UsersList()
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, userView(u))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	uname := sanitizeUsername(req.Username)
	if uname == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "用户名仅限字母/数字/-_.，长度 2–32 位"})
		return
	}
	if len(strings.TrimSpace(req.Password)) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "密码至少 4 位"})
		return
	}
	if !validRole(req.Role) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "角色不合法"})
		return
	}
	email := strings.TrimSpace(req.Email)
	if email != "" && !validEmail(email) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "邮箱格式不合法"})
		return
	}
	if err := s.cfg.CreateUser(uname, req.Password, strings.TrimSpace(req.DisplayName), email, req.Role); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "创建用户：" + uname + "（" + req.Role + "）"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	var req struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if !validRole(req.Role) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "角色不合法"})
		return
	}
	email := strings.TrimSpace(req.Email)
	if email != "" && !validEmail(email) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "邮箱格式不合法"})
		return
	}
	if err := s.cfg.UpdateUserMeta(username, strings.TrimSpace(req.DisplayName), email, req.Role); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "修改用户：" + username + " → " + req.Role})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if cur, ok := s.currentUser(r); ok && cur.Username == username {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "不能删除当前登录的账户"})
		return
	}
	if err := s.cfg.DeleteUser(username); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.auth.clearUserSessions(username) // kick the removed user out
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "删除用户：" + username})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleResetUserPassword(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if len(strings.TrimSpace(req.Password)) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "密码至少 4 位"})
		return
	}
	if err := s.cfg.SetUserPassword(username, req.Password); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.auth.clearUserSessions(username) // force re-login with the new password
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "重置用户密码：" + username})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleResetUserMFA(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if err := s.cfg.SetUserMFA(username, false, ""); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "管理员解除用户两步验证：" + username})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	var in ServerConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	mergeSecrets(&in, s.cfg.Get())
	if err := s.cfg.Set(in); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// config changed: re-sync alert state so a newly configured webhook
	// immediately receives the currently-outstanding alerts.
	s.notifier.ResetState()
	go s.notifier.Trigger()
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: s.clientIP(r), Message: "更新告警配置"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleTestConfig(w http.ResponseWriter, r *http.Request) {
	var in ServerConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	mergeSecrets(&in, s.cfg.Get())
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: s.clientIP(r), Message: "发送告警测试消息"})
	if errs := s.notifier.SendTest(in); len(errs) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "errors": errs})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleInstallInfo returns the data the panel needs to render one-line install
// commands: the reachable server URL and the current install token.
func (s *Server) handleInstallInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"server_url":    serverURL(r),
		"token":         s.cfg.InstallToken(),
		"require_token": s.cfg.AgentTokenRequired(),
	})
}

func (s *Server) handleResetToken(w http.ResponseWriter, r *http.Request) {
	tok := s.cfg.ResetToken()
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "重置安装 Token"})
	writeJSON(w, http.StatusOK, map[string]string{"token": tok})
}

// handleInstallScript serves the platform install script (install.sh /
// install.ps1) with the server URL, token and category injected.
func (s *Server) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	// Do NOT fall back to the real install token when the query param is absent —
	// /install.sh is public, so injecting it would leak the token to anyone who
	// can reach the server. The dashboard always generates the command WITH the
	// token (from the authenticated /install/info), so legitimate installs carry it.
	token := sanitizeToken(r.URL.Query().Get("token"))
	// category & server are echoed into the shell/PowerShell install script inside
	// double quotes; sanitize so a crafted ?category= (or a forged X-Forwarded-Host
	// feeding serverURL) can't inject commands into the script a victim pipes to sh.
	category := sanitizeCategory(r.URL.Query().Get("category"))
	server := sanitizeServerURL(serverURL(r))
	var body string
	if strings.HasSuffix(r.URL.Path, ".ps1") {
		body = renderScript(installPs1Template, server, token, category)
	} else {
		body = renderScript(installShTemplate, server, token, category)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, body)
}

// handleUninstallScript serves the platform uninstall script (uninstall.sh /
// uninstall.ps1). These are static — no server URL / token needed.
func (s *Server) handleUninstallScript(w http.ResponseWriter, r *http.Request) {
	body := uninstallShTemplate
	if strings.HasSuffix(r.URL.Path, ".ps1") {
		body = uninstallPs1Template
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, body)
}

// clientIP extracts the operator's IP for the activity log.
// clientIP returns the request's client address for audit logs and login
// rate-limiting. Reverse-proxy headers (X-Real-IP / X-Forwarded-For) are honored
// ONLY when trust_proxy is enabled — otherwise they are attacker-forgeable and a
// directly-exposed server would let anyone reset their rate-limit bucket (and
// forge audit-log origins) by spoofing a header, so we use the raw connection
// address instead.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy() {
		if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
			return xr
		}
		if f := r.Header.Get("X-Forwarded-For"); f != "" {
			// Last hop is the address our trusted proxy actually saw (nginx appends
			// $remote_addr); the client-controlled prefix is not trusted.
			parts := strings.Split(f, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				return ip
			}
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// isHTTPS reports whether the request reached us over TLS, honoring the common
// reverse-proxy header. Used to set the Secure flag on the session cookie.
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// serverURL reconstructs the externally-reachable base URL from the request,
// honoring common reverse-proxy headers.
func serverURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = p
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	return scheme + "://" + host
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "ok", "time_unix": time.Now().Unix(),
	})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "dashboard not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// ---- secret masking helpers ----

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

// mergeSecrets keeps existing webhook/secret values when the incoming ones are
// blank or still masked, so the panel can submit without re-typing secrets.
func mergeSecrets(in *ServerConfig, old ServerConfig) {
	in.Feishu.Webhook = keepIfBlank(in.Feishu.Webhook, old.Feishu.Webhook)
	in.Dingtalk.Webhook = keepIfBlank(in.Dingtalk.Webhook, old.Dingtalk.Webhook)
	in.Dingtalk.Secret = keepIfBlank(in.Dingtalk.Secret, old.Dingtalk.Secret)
	in.SMTP.Password = keepIfBlank(in.SMTP.Password, old.SMTP.Password)
	if in.SMTP.FromName == "" {
		in.SMTP.FromName = old.SMTP.FromName
	}
}

func keepIfBlank(newv, oldv string) string {
	t := strings.TrimSpace(newv)
	if t == "" || strings.Contains(t, "****") {
		return oldv
	}
	return newv
}

// ---- install-script parameter sanitizers ----
// /install.sh and /install.ps1 are public and echo these query params into a
// shell/PowerShell script that a machine pipes straight to sh/iex. Any of them
// could otherwise carry quotes/`$`/backticks/`;` that break out of the quoted
// assignment and inject commands, so each is reduced to a safe charset. Real
// values (hex token, a URL, a category name) are unaffected.

func sanitizeToken(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 128 {
		s = s[:128]
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return -1
		}
	}, s)
}

func sanitizeCategory(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.', r == ' ':
			return r
		case unicode.Is(unicode.Han, r):
			return r
		default:
			return -1
		}
	}, strings.TrimSpace(s))
	if rs := []rune(s); len(rs) > 48 {
		s = string(rs[:48])
	}
	return s
}

func sanitizeServerURL(u string) string {
	u = strings.TrimSpace(u)
	if len(u) > 256 {
		u = u[:256]
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case strings.ContainsRune(":/._-", r):
			return r
		default:
			return -1
		}
	}, u)
}

// sanitizeUsername validates the login username: 2–32 chars of ASCII letters,
// digits, dot, dash or underscore. Returns "" when invalid.
func sanitizeUsername(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 || len(s) > 32 {
		return ""
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_'
		if !ok {
			return ""
		}
	}
	return s
}

// -----------------------------------------------------------------------
// Account recovery: username recovery + password reset via email.
// These endpoints are PUBLIC (no session) — they are gated by email
// verification codes and rate limiting instead.
// -----------------------------------------------------------------------

// handleRecoverUsername sends the account username to the given email address,
// but only if that email matches the configured account.
func (s *Server) handleRecoverUsername(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if !validEmail(req.Email) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "邮箱格式不合法"})
		return
	}
	// Do NOT reveal whether the email matches — always return the same response.
	// Only actually send when the address matches some user's bound email.
	if user, found := s.cfg.UserByEmail(req.Email); found {
		cfg := s.cfg.Get()
		if cfg.SMTP.Enabled && cfg.SMTP.Host != "" {
			html := fmt.Sprintf(`<div style="font-family:sans-serif;padding:20px;max-width:500px;margin:0 auto">
  <h2>AIOps Monitor — 账户用户名</h2>
  <p>您的登录用户名为：<b style="font-size:18px">%s</b></p>
  <p style="color:#888;font-size:13px">如非本人操作请忽略此邮件。</p>
  <p style="color:#888;font-size:13px">时间：%s</p>
</div>`, user.Username, time.Now().Format("2006-01-02 15:04:05"))
			if err := sendEmail(cfg.SMTP, req.Email, "AIOps Monitor — 用户名找回", html); err != nil {
				s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "用户名找回邮件发送失败：" + err.Error()})
				writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "邮件发送失败，请稍后重试"})
				return
			}
			s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: s.clientIP(r), Message: "用户名找回邮件已发送"})
		}
	}
	// Always return ok to prevent email enumeration
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "如该邮箱已绑定账户，用户名将通过邮件发送"})
}

// handleSendResetCode sends a 6-digit verification code to the email bound to
// the given username. The code is required for the subsequent password reset.
func (s *Server) handleSendResetCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请输入用户名"})
		return
	}
	user, found := s.cfg.UserByName(req.Username)
	// Uniform response regardless of whether the user exists / has an email, to
	// avoid username enumeration.
	if !found || user.Email == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "如该用户名存在且已绑定邮箱，验证码已发送"})
		return
	}
	cfg := s.cfg.Get()
	if !cfg.SMTP.Enabled || cfg.SMTP.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "系统未配置邮件服务，请联系管理员"})
		return
	}
	code, err := s.emailMgr.issueCode(user.Email, "reset_password")
	if err != nil {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": err.Error()})
		return
	}
	html := fmt.Sprintf(`<div style="font-family:sans-serif;padding:20px;max-width:500px;margin:0 auto">
  <h2>AIOps Monitor — 密码重置验证码</h2>
  <p>您的验证码为：<b style="font-size:24px;letter-spacing:4px;color:#4c8dff">%s</b></p>
  <p>验证码 10 分钟内有效，请尽快使用。</p>
  <p style="color:#888;font-size:13px">如非本人操作请忽略此邮件并建议修改密码。</p>
  <p style="color:#888;font-size:13px">时间：%s</p>
</div>`, code, time.Now().Format("2006-01-02 15:04:05"))
	if err := sendEmail(cfg.SMTP, user.Email, "AIOps Monitor — 密码重置验证码", html); err != nil {
		s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "密码重置验证码发送失败：" + err.Error()})
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "邮件发送失败，请稍后重试"})
		return
	}
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: s.clientIP(r), Message: "密码重置验证码已发送至绑定邮箱"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "如该用户名存在且已绑定邮箱，验证码已发送"})
}

// handleResetPassword resets the account password after verifying the email code.
func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Code     string `json:"code"`
		NewPass  string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	req.Code = strings.TrimSpace(req.Code)
	if req.Username == "" || !validEmail(req.Email) || len(req.Code) != 6 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "参数不完整或格式错误"})
		return
	}
	if len(strings.TrimSpace(req.NewPass)) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "新密码至少 4 位"})
		return
	}
	user, found := s.cfg.UserByName(req.Username)
	if !found || !strings.EqualFold(req.Email, user.Email) || user.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "用户名或邮箱不匹配"})
		return
	}
	if !s.emailMgr.verifyCode(user.Email, "reset_password", req.Code) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "验证码错误或已过期"})
		return
	}
	_ = s.cfg.SetUserPassword(user.Username, req.NewPass)
	s.auth.clearUserSessions(user.Username) // invalidate that user's old sessions
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "通过邮箱验证码重置密码：" + user.Username})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "密码已重置，请使用新密码登录"})
}

// handleMFAUnbindViaEmail disables MFA after verifying a code sent to the bound
// email. This is the recovery path when the operator lost their authenticator.
func (s *Server) handleMFAUnbindViaEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"` // "send_code" | "verify"
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	acc, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if acc.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "账户未绑定邮箱，无法通过邮箱解除 MFA"})
		return
	}
	if !acc.MFAEnabled {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "MFA 未启用"})
		return
	}
	if req.Action == "send_code" {
		cfg := s.cfg.Get()
		if !cfg.SMTP.Enabled || cfg.SMTP.Host == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "系统未配置邮件服务"})
			return
		}
		code, err := s.emailMgr.issueCode(acc.Email, "mfa_unbind")
		if err != nil {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": err.Error()})
			return
		}
		html := fmt.Sprintf(`<div style="font-family:sans-serif;padding:20px;max-width:500px;margin:0 auto">
  <h2>AIOps Monitor — 解除两步验证</h2>
  <p>您正在通过邮箱解除两步验证（MFA）绑定。</p>
  <p>验证码：<b style="font-size:24px;letter-spacing:4px;color:#4c8dff">%s</b></p>
  <p>验证码 10 分钟内有效，单次使用。</p>
  <p style="color:#888;font-size:13px">如非本人操作，请立即修改密码。</p>
</div>`, code)
		if err := sendEmail(cfg.SMTP, acc.Email, "AIOps Monitor — MFA 解除验证码", html); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "邮件发送失败：" + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "验证码已发送至绑定邮箱"})
		return
	}
	if req.Action == "verify" {
		req.Code = strings.TrimSpace(req.Code)
		if len(req.Code) != 6 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请输入 6 位验证码"})
			return
		}
		if !s.emailMgr.verifyCode(acc.Email, "mfa_unbind", req.Code) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "验证码错误或已过期"})
			return
		}
		_ = s.cfg.SetUserMFA(acc.Username, false, "")
		s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "通过邮箱验证码解除两步验证：" + acc.Username})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "两步验证已关闭"})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "未知操作类型"})
}

// -----------------------------------------------------------------------
// Playbook (automation) handlers
// -----------------------------------------------------------------------

func (s *Server) handleListPlaybooks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.playbooks.List())
}

func (s *Server) handleUpsertPlaybook(w http.ResponseWriter, r *http.Request) {
	var p Playbook
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	saved, err := s.playbooks.Upsert(p)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: s.clientIP(r), Message: "保存剧本：" + saved.Name})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID})
}

func (s *Server) handleDeletePlaybook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = s.playbooks.Delete(id)
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: "删除剧本 " + id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleExecutePlaybook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pb, ok := s.playbooks.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "剧本不存在"})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "目标主机为空或均已离线——请检查步骤的目标选择与主机在线状态"})
		return
	}
	targetList := make([]*Host, 0, len(targetSet))
	for _, h := range targetSet {
		targetList = append(targetList, h)
	}
	exec := s.playbooks.StartExecution(pb, s.clientIP(r), targetList)
	// Run each step on each host sequentially via the agent reverse terminal channel
	go s.runPlaybookExecution(pb, exec, targetList)
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: s.clientIP(r), Message: fmt.Sprintf("执行剧本「%s」于 %d 台主机", pb.Name, len(targetList))})
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
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: exec.Operator, Message: fmt.Sprintf("剧本「%s」执行完成： %s", pb.Name, status)})
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
	sess := s.term.create(h.ID, h.Hostname, "playbook-exec")
	defer s.term.remove(sess.id)
	// Critical: close the session when done so the agent's shell exits and the
	// agent returns to waiting for the next command. Without this the agent stays
	// stuck on this session and every subsequent step/execution fails to connect.
	defer sess.close()
	if !s.term.notifyAgent(h.ID, sess.id) {
		return "", fmt.Errorf("无法连接到主机 %s 的 Agent（可能有其它终端/剧本正在占用，或 Agent 版本过旧）", h.Hostname)
	}
	select {
	case <-sess.agentUp:
	case <-time.After(10 * time.Second):
		return "", fmt.Errorf("Agent 接入超时")
	case <-sess.done:
		return "", fmt.Errorf("会话已断开")
	}
	// Unique sentinel: after the command finishes, the shell runs "echo SENTINEL"
	// and the sentinel appears on its own line in the output. Detecting it means
	// the command is done. Using UnixNano ensures uniqueness across hosts/steps.
	sentinel := fmt.Sprintf("AIOPS_EXEC_DONE_%d", time.Now().UnixNano())
	// Send the command + Enter, then the sentinel echo + Enter. The shell
	// processes them sequentially, so the sentinel only appears after the
	// command completes.
	fullCmd := command + "\r" + "echo " + sentinel + "\r"
	select {
	case sess.toAgent <- termFrame('i', []byte(fullCmd)):
	case <-time.After(5 * time.Second):
		return "", fmt.Errorf("命令发送超时")
	}
	// Collect output until the sentinel appears as a standalone line or timeout.
	var output []byte
	timer := time.NewTimer(time.Duration(timeoutSec) * time.Second)
	defer timer.Stop()
	for {
		select {
		case b := <-sess.toBrowser:
			output = append(output, b...)
			if len(output) > 256*1024 {
				output = output[len(output)-256*1024:]
			}
			// Search for the sentinel at the beginning of a line (the echo
			// output), not inside the "echo SENTINEL" command echo.
			for _, sep := range []string{"\r\n" + sentinel, "\n" + sentinel} {
				if idx := bytes.Index(output, []byte(sep)); idx >= 0 {
					result := output[:idx]
					return cleanTerminalOutput(string(result)), nil
				}
			}
		case <-timer.C:
			select {
			case sess.toAgent <- termFrame('i', []byte{0x03}):
			default:
			}
			return cleanTerminalOutput(string(output)), fmt.Errorf("执行超时（%ds）", timeoutSec)
		case <-sess.done:
			return cleanTerminalOutput(string(output)), nil
		}
	}
}

// cleanTerminalOutput strips ANSI escape sequences, shell prompts, command
// echoes, and the sentinel echo from raw terminal output, returning clean
// text suitable for playbook execution results.
func cleanTerminalOutput(raw string) string {
	// 1. Strip ANSI escape sequences (CSI, OSC, and other ESC sequences)
	cleaned := stripANSIEscapes(raw)
	// 2. Normalize line endings
	cleaned = strings.ReplaceAll(cleaned, "\r\n", "\n")
	cleaned = strings.ReplaceAll(cleaned, "\r", "\n")
	// 3. Split into lines and filter
	lines := strings.Split(cleaned, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip empty lines at the beginning
		if len(result) == 0 && trimmed == "" {
			continue
		}
		// Skip shell prompts (Windows: "C:\...>", Linux: "user@host:~$")
		if isShellPrompt(trimmed) {
			continue
		}
		// Skip the sentinel echo command line
		if strings.HasPrefix(trimmed, "echo AIOPS_EXEC_DONE") {
			continue
		}
		result = append(result, line)
	}
	// 4. Trim trailing empty lines
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}
	return strings.Join(result, "\n")
}

// stripANSIEscapes removes ANSI/VT100 escape sequences from a string.
// Handles CSI (\x1b[...final), OSC (\x1b]...\x07 or \x1b\\), and bare ESC chars.
func stripANSIEscapes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) {
			switch s[i+1] {
			case '[': // CSI: ESC [ params... final(0x40-0x7e)
				i += 2
				for i < len(s) && s[i] >= 0x20 && s[i] <= 0x3f {
					i++ // parameter bytes
				}
				for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
					i++ // intermediate bytes
				}
				if i < len(s) && s[i] >= 0x40 && s[i] <= 0x7e {
					i++ // final byte
				}
			case ']': // OSC: ESC ] ... BEL or ST (ESC \)
				i += 2
				for i < len(s) {
					if s[i] == 0x07 { // BEL terminator
						i++
						break
					}
					if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' { // ST terminator
						i += 2
						break
					}
					i++
				}
			default: // Other ESC sequences (skip 2 bytes)
				i += 2
			}
			continue
		}
		// Skip non-printable control chars except \n, \r, \t
		if s[i] < 0x20 && s[i] != '\n' && s[i] != '\r' && s[i] != '\t' {
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// isShellPrompt returns true if the line looks like a shell prompt.
// Windows cmd.exe: "C:\Users\admin>"  |  PowerShell: "PS C:\...>"
// Linux/macOS: "user@host:~$"  |  root: "#"  |  sh: "$"
func isShellPrompt(line string) bool {
	// Windows cmd.exe prompt: drive letter + path + ">"
	if strings.HasSuffix(line, ">") && (strings.Contains(line, ":\\") || strings.Contains(line, ":/")) {
		return true
	}
	// PowerShell: "PS ...>"
	if strings.HasPrefix(line, "PS ") && strings.HasSuffix(line, ">") {
		return true
	}
	// Linux/macOS: "user@host:...$" or "user@host:...#"
	if (strings.HasSuffix(line, "$") || strings.HasSuffix(line, "#")) && strings.Contains(line, "@") {
		return true
	}
	// Bare root/sh prompt
	if line == "#" || line == "$" {
		return true
	}
	return false
}

func (s *Server) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.playbooks.ExecutionHistory())
}

func (s *Server) handleGetExecution(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	exec, ok := s.playbooks.GetExecution(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "执行记录不存在"})
		return
	}
	writeJSON(w, http.StatusOK, exec)
}

// -----------------------------------------------------------------------
// Terminal enhancement handlers
// -----------------------------------------------------------------------

func (s *Server) handleListTerminalSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.term.listSessions())
}

func (s *Server) handleTerminalReplay(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("id")
	frames := s.term.getRecording(sid)
	if frames == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "会话不存在或已结束"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"frames": frames, "count": len(frames)})
}

// handleTerminalObserve allows a second logged-in user to watch a live terminal
// session in read-only mode via WebSocket.
func (s *Server) handleTerminalObserve(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("id")
	if !s.cfg.TerminalEnabled() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "远程终端已被管理员禁用"})
		return
	}
	obs, ok := s.term.addObserver(sid)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "会话不存在"})
		return
	}
	defer s.term.removeObserver(sid, obs)
	ws, err := wsAccept(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "需要 WebSocket 升级"})
		return
	}
	defer ws.Close()
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: s.clientIP(r), Message: "旁观终端会话 " + sid[:8]})
	// Send recorded history first so the observer sees the full context
	for _, data := range s.term.getDecodedRecording(sid) {
		if err := ws.WriteBinary(data); err != nil {
			return
		}
	}
	// Then stream live output
	for {
		select {
		case b := <-obs.ch:
			if err := ws.WriteBinary(b); err != nil {
				return
			}
		case <-obs.done:
			return
		}
	}
}
