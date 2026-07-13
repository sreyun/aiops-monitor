package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// handleGetConfig returns the alert config with webhooks/secrets masked.
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	c := s.cfg.Get()
	c.Categories = nil
	c.Feishu.Webhook = maskSecret(c.Feishu.Webhook)
	c.Dingtalk.Webhook = maskSecret(c.Dingtalk.Webhook)
	c.Dingtalk.Secret = maskSecret(c.Dingtalk.Secret)
	c.CustomWebhook.URL = maskSecret(c.CustomWebhook.URL)
	c.SMTP.Password = maskSecret(c.SMTP.Password)
	c.SMS.SecretKey = maskSecret(c.SMS.SecretKey)
	c.SMS.AccessKey = maskSecret(c.SMS.AccessKey)
	c.VoiceCall.SecretKey = maskSecret(c.VoiceCall.SecretKey)
	c.VoiceCall.AccessKey = maskSecret(c.VoiceCall.AccessKey)
	c.AI.APIKey = maskSecret(c.AI.APIKey)       // AI provider credential
	c.PostgresDSN = maskSecret(c.PostgresDSN)   // DSN carries the PostgreSQL password
	c.InstallToken = maskSecret(c.InstallToken)                   // agent enrollment token — not for viewers
	c.RelaySecret = maskSecret(c.RelaySecret)                     // gateway relay shared secret
	c.CustomWebhook.Headers = maskSecret(c.CustomWebhook.Headers) // may carry auth tokens (e.g. X-Token)
	// Never expose the password hash/salt or the MFA secret to the browser.
	c.Account.Salt, c.Account.Hash, c.Account.MFASecret = "", "", ""
	c.Users = nil // the user list (with hashes) is served via /api/v1/users, not here
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	var in ServerConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
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
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: Tz("log.update_config")})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleTestConfig(w http.ResponseWriter, r *http.Request) {
	var in ServerConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	mergeSecrets(&in, s.cfg.Get())
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: Tz("log.test_alert")})
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
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: Tz("log.reset_token")})
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
	// Multi-server: the dashboard may pass a JSON array of {server,token} objects
	// so one agent pushes to multiple backends. Sanitized+re-serialized here so
	// a crafted payload can't inject shell/PowerShell metacharacters.
	serversJSON := sanitizeServersJSON(r.URL.Query().Get("servers_json"))
	// 日志采集路径（可选）：清洗为合法 JSON 数组注入生成的 config.json 的 log_paths
	logPaths := sanitizeLogPaths(r.URL.Query().Get("log_paths"))
	var body string
	if strings.HasSuffix(r.URL.Path, ".ps1") {
		body = renderScript(installPs1Template, server, token, category, serversJSON, logPaths)
	} else {
		body = renderScript(installShTemplate, server, token, category, serversJSON, logPaths)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, body)
}

// handleRelayInstallScript serves the gateway relay install script
// (install-relay.sh / install-relay.ps1) — same token/category sanitization as
// the regular install script, but uses the relay templates that configure the
// agent in --relay mode.
func (s *Server) handleRelayInstallScript(w http.ResponseWriter, r *http.Request) {
	token := sanitizeToken(r.URL.Query().Get("token"))
	category := sanitizeCategory(r.URL.Query().Get("category"))
	server := sanitizeServerURL(serverURL(r))
	var body string
	if strings.HasSuffix(r.URL.Path, ".ps1") {
		body = renderScript(relayInstallPs1Template, server, token, category, "", "")
	} else {
		body = renderScript(relayInstallShTemplate, server, token, category, "", "")
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
