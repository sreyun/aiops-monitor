package main

import (
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"aiops-monitor/shared"
)

//go:embed web
var webFS embed.FS

// Server wires the store, the operator-editable config and the notifier to
// HTTP handlers.
type Server struct {
	store    *Store
	cfg      *ConfigStore
	notifier *Notifier
	auth     *Auth
	checks   *checkRunner
	distDir  string // directory of downloadable agent binaries + plugins.zip
}

func NewServer(store *Store, cfg *ConfigStore, notifier *Notifier, distDir string, selfAddr string) *Server {
	return &Server{
		store: store, cfg: cfg, notifier: notifier, distDir: distDir,
		auth:   NewAuth(cfg),
		checks: newCheckRunner(cfg, store, notifier, selfAddr),
	}
}

// Routes builds the HTTP handler using Go 1.22 method+path patterns.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/agent/register", s.handleRegister)
	mux.HandleFunc("POST /api/v1/agent/report", s.handleReport)
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
	mux.HandleFunc("GET /api/v1/checks", s.handleGetChecks)
	mux.HandleFunc("POST /api/v1/checks", s.handleUpsertCheck)
	mux.HandleFunc("DELETE /api/v1/checks/{id}", s.handleDeleteCheck)
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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
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
	if s.cfg.RequireToken() && rep.Token != s.cfg.InstallToken() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid token"})
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
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: clientIP(r), Message: msg})
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
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: clientIP(r), Message: "删除主机 " + shortID(id)})
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted", "host_id": id})
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	alerts := Evaluate(s.store.ListHosts(), s.cfg.Thresholds())
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
		}
		if s2, ok := st[c.ID]; ok {
			m["ok"], m["message"], m["checked_at"], m["latency_ms"] = s2.OK, s2.Message, s2.CheckedAt, s2.LatencyMs
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
	if c.Name == "" || c.Target == "" || (c.Type != "http" && c.Type != "tcp" && c.Type != "process") {
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
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: clientIP(r), Message: "保存自定义监控：" + saved.Name})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID})
}

func (s *Server) handleDeleteCheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = s.cfg.DeleteCheck(id)
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: clientIP(r), Message: "删除自定义监控 " + id})
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
	})
}

// handleGetConfig returns the alert config with webhooks/secrets masked.
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	c := s.cfg.Get()
	c.Categories = nil
	c.Feishu.Webhook = maskSecret(c.Feishu.Webhook)
	c.Dingtalk.Webhook = maskSecret(c.Dingtalk.Webhook)
	c.Dingtalk.Secret = maskSecret(c.Dingtalk.Secret)
	writeJSON(w, http.StatusOK, c)
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
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: clientIP(r), Message: "更新告警配置"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleTestConfig(w http.ResponseWriter, r *http.Request) {
	var in ServerConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	mergeSecrets(&in, s.cfg.Get())
	s.store.AddLog(LogEntry{Kind: "操作", Level: "info", Actor: clientIP(r), Message: "发送告警测试消息"})
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
		"require_token": s.cfg.RequireToken(),
	})
}

func (s *Server) handleResetToken(w http.ResponseWriter, r *http.Request) {
	tok := s.cfg.ResetToken()
	s.store.AddLog(LogEntry{Kind: "操作", Level: "warning", Actor: clientIP(r), Message: "重置安装 Token"})
	writeJSON(w, http.StatusOK, map[string]string{"token": tok})
}

// handleInstallScript serves the platform install script (install.sh /
// install.ps1) with the server URL, token and category injected.
func (s *Server) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = s.cfg.InstallToken()
	}
	category := r.URL.Query().Get("category")
	var body string
	if strings.HasSuffix(r.URL.Path, ".ps1") {
		body = renderScript(installPs1Template, serverURL(r), token, category)
	} else {
		body = renderScript(installShTemplate, serverURL(r), token, category)
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
func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
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
}

func keepIfBlank(newv, oldv string) string {
	t := strings.TrimSpace(newv)
	if t == "" || strings.Contains(t, "****") {
		return oldv
	}
	return newv
}
