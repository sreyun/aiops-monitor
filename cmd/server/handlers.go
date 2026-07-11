package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
)

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
	term      *termManager        // remote terminal relay
	forward   *forwardManager     // port forwarding relay (TCP + HTTP proxy)
	emailMgr  *emailManager       // verification codes + reset tokens
	playbooks *playbookManager    // automation playbooks + execution history
	push      *pushHub            // P3-1: WebSocket push hub for real-time updates
	// --- SRE workflow layer ---
	incidents   *incidentManager   // incident hub (alert/SLO/manual)
	remediation *remediationManager // closed-loop auto-remediation
	slos        *sloManager        // SLO + error budgets
	tickets     *ticketManager     // work orders
	logs        *logStore          // aggregated agent logs
	ai          *aiManager         // AI inspection + diagnosis
	vm          *vmWriter          // optional VictoriaMetrics remote-write
	messages    *messageHub        // unified notification center (SRE/alert/AI feed)
	distDir     string             // directory of downloadable agent binaries + plugins.zip
	pg          *pgStore           // PostgreSQL persistence (optional, for pgvector/RAG)
	hermes      *HermesCore        // Hermes Agent (autonomous SRE agent)
}

func NewServer(store *Store, cfg *ConfigStore, notifier *Notifier, distDir string, selfAddr string) *Server {
	s := &Server{
		store: store, cfg: cfg, notifier: notifier, distDir: distDir,
		auth:      NewAuth(cfg),
		checks:    newCheckRunner(cfg, store, notifier, selfAddr),
		term:      newTermManager(),
		forward:   newForwardManager(cfg),
		emailMgr:  newEmailManager(),
		playbooks: newPlaybookManager(cfg),
		push:      newPushHub(),
		incidents:   newIncidentManager(),
		remediation: newRemediationManager(cfg),
		slos:        newSLOManager(cfg),
		tickets:     newTicketManager(),
		logs:        newLogStore(),
		ai:          newAIManager(cfg),
		vm:          newVMWriter(cfg),
		messages:    newMessageHub(),
	}
	s.wireSRE()
	// Restore persisted TCP forward rules (recreate listeners)
	s.forward.restoreRules(s)
	// Hermes Agent: initialize if AI is configured
	if cfg := s.cfg.AIConfig(); cfg.HermesEnabled && cfg.Enabled {
		s.hermes = newHermesCore(s)
	}
	return s
}

// Routes builds the HTTP handler using Go 1.22 method+path patterns.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/agent/register", s.handleRegister)
	mux.HandleFunc("POST /api/v1/agent/report", s.handleReport)
	// terminal auth: secondary password + protocol agreement
	mux.HandleFunc("GET /api/user/terminal-password/status", s.handleTerminalPasswordStatus)
	mux.HandleFunc("POST /api/user/terminal-password/set", s.handleTerminalPasswordSet)
	mux.HandleFunc("POST /api/user/terminal-password/verify", s.handleTerminalPasswordVerify)
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
	mux.HandleFunc("POST /api/v1/alerts/ack", s.handleAlertAck)
	mux.HandleFunc("POST /api/v1/alerts/silence", s.handleAlertSilence)
	mux.HandleFunc("POST /api/v1/alerts/clear", s.handleAlertClear)
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
	mux.HandleFunc("POST /api/v1/account/init", s.handleAccountInit)
	mux.HandleFunc("POST /api/v1/mfa/setup", s.handleMFASetup)
	mux.HandleFunc("POST /api/v1/mfa/enable", s.handleMFAEnable)
	mux.HandleFunc("POST /api/v1/mfa/disable", s.handleMFADisable)
	mux.HandleFunc("POST /api/v1/mfa/unbind-via-email", s.handleMFAUnbindViaEmail)
	mux.HandleFunc("GET /api/v1/mfa/global", s.handleMFAGlobalGet)
	mux.HandleFunc("POST /api/v1/mfa/global", s.handleMFAGlobalSet)
	// Account recovery: public endpoints (no session required)
	// New dual-verification flow (email code + optional MFA TOTP)
	mux.HandleFunc("POST /api/v1/account/recover-send-code", s.handleRecoverSendCode)
	mux.HandleFunc("POST /api/v1/account/recover-verify", s.handleRecoverVerify)
	mux.HandleFunc("POST /api/v1/account/recover-verify-mfa", s.handleRecoverVerifyMFA)
	// Legacy/backward-compat endpoints
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
	// SRE workflow: incidents / auto-remediation / SLOs / work orders
	mux.HandleFunc("GET /api/v1/sre/overview", s.handleSREOverview)
	mux.HandleFunc("GET /api/v1/incidents", s.handleListIncidents)
	mux.HandleFunc("POST /api/v1/incidents", s.handleCreateIncident)
	mux.HandleFunc("GET /api/v1/incidents/{id}", s.handleGetIncident)
	mux.HandleFunc("POST /api/v1/incidents/{id}/ack", s.handleAckIncident)
	mux.HandleFunc("POST /api/v1/incidents/{id}/resolve", s.handleResolveIncident)
	mux.HandleFunc("POST /api/v1/incidents/{id}/comment", s.handleCommentIncident)
	mux.HandleFunc("POST /api/v1/incidents/{id}/ticket", s.handleEscalateIncident)
	mux.HandleFunc("GET /api/v1/remediation/rules", s.handleListRemediationRules)
	mux.HandleFunc("POST /api/v1/remediation/rules", s.handleUpsertRemediationRule)
	mux.HandleFunc("DELETE /api/v1/remediation/rules/{id}", s.handleDeleteRemediationRule)
	mux.HandleFunc("GET /api/v1/remediation/runs", s.handleListRemediationRuns)
	mux.HandleFunc("POST /api/v1/remediation/runs/{id}/approve", s.handleApproveRemediation)
	mux.HandleFunc("POST /api/v1/remediation/runs/{id}/reject", s.handleRejectRemediation)
	mux.HandleFunc("GET /api/v1/slos", s.handleListSLOs)
	mux.HandleFunc("POST /api/v1/slos", s.handleUpsertSLO)
	mux.HandleFunc("DELETE /api/v1/slos/{id}", s.handleDeleteSLO)
	mux.HandleFunc("GET /api/v1/tickets", s.handleListTickets)
	mux.HandleFunc("POST /api/v1/tickets", s.handleCreateTicket)
	mux.HandleFunc("GET /api/v1/tickets/{id}", s.handleGetTicket)
	mux.HandleFunc("POST /api/v1/tickets/{id}", s.handleUpdateTicket)
	mux.HandleFunc("POST /api/v1/tickets/{id}/comment", s.handleCommentTicket)
	mux.HandleFunc("DELETE /api/v1/tickets/{id}", s.handleDeleteTicket)
	// Log aggregation (agent ingest is fingerprint-authed) + search
	mux.HandleFunc("POST /api/v1/agent/logs", s.handleAgentLogs)
	mux.HandleFunc("GET /api/v1/logs", s.handleSearchLogs)
	// Notification center (unified message feed)
	mux.HandleFunc("GET /api/v1/messages", s.handleListMessages)
	mux.HandleFunc("POST /api/v1/messages/read", s.handleMarkMessagesRead)
	mux.HandleFunc("POST /api/v1/messages/read-all", s.handleMarkAllMessagesRead)
	// AI: config + inspection + incident diagnosis
	mux.HandleFunc("GET /api/v1/ai/config", s.handleGetAIConfig)
	mux.HandleFunc("POST /api/v1/ai/config", s.handleSetAIConfig)
	mux.HandleFunc("POST /api/v1/ai/test", s.handleTestAIConfig)
	mux.HandleFunc("POST /api/v1/ai/chat", s.handleAIChat)
	mux.HandleFunc("GET /api/v1/ai/models", s.handleAIModels)
	mux.HandleFunc("GET /api/v1/ai/inspections", s.handleListInspections)
	mux.HandleFunc("POST /api/v1/ai/inspect", s.handleRunInspection)
	mux.HandleFunc("POST /api/v1/incidents/{id}/diagnose", s.handleDiagnoseIncident)
	mux.HandleFunc("POST /api/v1/incidents/{id}/diagnose-chat", s.handleDiagnoseChatIncident)
	mux.HandleFunc("GET /api/v1/incidents/{id}/diagnose-chat", s.handleGetDiagnosisChatHistory)
	mux.HandleFunc("POST /api/v1/incidents/{id}/diagnosis-feedback", s.handleDiagnosisFeedback)
	// AI 经验规则库
	mux.HandleFunc("GET /api/v1/experience-rules", s.handleListExperienceRules)
	mux.HandleFunc("POST /api/v1/experience-rules", s.handleCreateExperienceRule)
	mux.HandleFunc("DELETE /api/v1/experience-rules/{id}", s.handleDeleteExperienceRule)
	// Hermes Agent — 自主运维 Agent
	mux.HandleFunc("POST /api/v1/hermes/chat", s.handleHermesChat)
	mux.HandleFunc("GET /api/v1/hermes/sessions", s.handleHermesSessions)
	mux.HandleFunc("GET /api/v1/hermes/sessions/{id}", s.handleHermesSession)
	mux.HandleFunc("GET /api/v1/hermes/rules", s.handleHermesListRules)
	mux.HandleFunc("POST /api/v1/hermes/rules", s.handleHermesUpsertRule)
	mux.HandleFunc("DELETE /api/v1/hermes/rules/{id}", s.handleHermesDeleteRule)
	mux.HandleFunc("GET /api/v1/hermes/templates", s.handleHermesListTemplates)
	mux.HandleFunc("POST /api/v1/hermes/templates", s.handleHermesUpsertTemplate)
	mux.HandleFunc("DELETE /api/v1/hermes/templates/{id}", s.handleHermesDeleteTemplate)
	// Terminal enhancements
	mux.HandleFunc("GET /api/v1/terminal/sessions", s.handleListTerminalSessions)
	mux.HandleFunc("GET /api/v1/terminal/sessions/{id}/replay", s.handleTerminalReplay)
	mux.HandleFunc("GET /api/v1/terminal/sessions/{id}/observe", s.handleTerminalObserve)
	// Port forwarding (TCP mapping + HTTP reverse proxy)
	mux.HandleFunc("GET /api/v1/forward", s.handleForwardList)
	mux.HandleFunc("POST /api/v1/forward", s.handleForwardCreate)
	mux.HandleFunc("DELETE /api/v1/forward/{id}", s.handleForwardDelete)
	mux.HandleFunc("PUT /api/v1/forward/{id}", s.handleForwardEdit)
	mux.HandleFunc("PUT /api/v1/forward/{id}/toggle", s.handleForwardToggle)
	mux.HandleFunc("POST /api/v1/forward/{id}/copy", s.handleForwardCopy)
	mux.HandleFunc("GET /api/v1/forward/stats", s.handleForwardStats)
	mux.HandleFunc("GET /api/v1/forward/health", s.handleForwardHealth)
	// HTTP proxy shortcuts (saved configs)
	mux.HandleFunc("GET /api/v1/http-proxy", s.handleHTTPProxyList)
	mux.HandleFunc("POST /api/v1/http-proxy", s.handleHTTPProxyCreate)
	mux.HandleFunc("DELETE /api/v1/http-proxy/{id}", s.handleHTTPProxyDelete)
	mux.HandleFunc("PUT /api/v1/http-proxy/{id}", s.handleHTTPProxyEdit)
	mux.HandleFunc("PUT /api/v1/http-proxy/{id}/toggle", s.handleHTTPProxyToggle)
	mux.HandleFunc("POST /api/v1/http-proxy/{id}/copy", s.handleHTTPProxyCopy)
	// HTTP proxy auth token for window.open() scenarios
	mux.HandleFunc("GET /api/v1/proxy-token", s.handleProxyToken)
	// HTTP proxy: support all methods (GET/POST/PUT/DELETE/PATCH)
	mux.HandleFunc("GET /proxy/{hostID}/{port}/{path...}", s.handleHTTPProxy)
	mux.HandleFunc("POST /proxy/{hostID}/{port}/{path...}", s.handleHTTPProxy)
	mux.HandleFunc("PUT /proxy/{hostID}/{port}/{path...}", s.handleHTTPProxy)
	mux.HandleFunc("DELETE /proxy/{hostID}/{port}/{path...}", s.handleHTTPProxy)
	mux.HandleFunc("PATCH /proxy/{hostID}/{port}/{path...}", s.handleHTTPProxy)
	// Port forwarding: agent reverse channel (fingerprint-gated, not session-gated)
	mux.HandleFunc("GET /api/v1/agent/forward/wait", s.handleAgentForwardWait)
	mux.HandleFunc("GET /api/v1/agent/forward/rx", s.handleAgentForwardRx)
	mux.HandleFunc("POST /api/v1/agent/forward/tx", s.handleAgentForwardTx)
	mux.HandleFunc("GET /api/v1/hosts/meta", s.handleHostsMeta)
	mux.HandleFunc("GET /api/v1/install/info", s.handleInstallInfo)
	mux.HandleFunc("POST /api/v1/install/reset-token", s.handleResetToken)
	mux.HandleFunc("GET /install.sh", s.handleInstallScript)
	mux.HandleFunc("GET /install.ps1", s.handleInstallScript)
	mux.HandleFunc("GET /install-relay.sh", s.handleRelayInstallScript)
	mux.HandleFunc("GET /install-relay.ps1", s.handleRelayInstallScript)
	mux.HandleFunc("GET /uninstall.sh", s.handleUninstallScript)
	mux.HandleFunc("GET /uninstall.ps1", s.handleUninstallScript)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	// P3-1: WebSocket push endpoint for real-time updates
	mux.HandleFunc("GET /ws/push", s.handlePushWS)
	mux.HandleFunc("GET /", s.handleDashboard)
	// static assets served from the embedded web/ dir
	if sub, err := fs.Sub(webFS, "web"); err == nil {
		fsrv := http.FileServer(http.FS(sub))
		mux.Handle("GET /style.css", fsrv)
		mux.Handle("GET /app.js", fsrv)
		mux.Handle("GET /theme-init.js", fsrv) // 主题预置（外置内联脚本，配合 CSP 去 unsafe-inline）
		mux.Handle("GET /i18n-dashboard.js", fsrv)
		mux.Handle("GET /i18n-dashboard.en.js", fsrv)
		mux.Handle("GET /i18n-dashboard.zh-TW.js", fsrv)
		// P2-1: support split CSS/JS modules
		mux.Handle("GET /css/", http.StripPrefix("/css/", http.FileServer(http.FS(sub))))
		mux.Handle("GET /js/", http.StripPrefix("/js/", http.FileServer(http.FS(sub))))
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
