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
	emailMgr  *emailManager       // verification codes + reset tokens
	playbooks *playbookManager    // automation playbooks + execution history
	push      *pushHub            // P3-1: WebSocket push hub for real-time updates
	distDir   string              // directory of downloadable agent binaries + plugins.zip
}

func NewServer(store *Store, cfg *ConfigStore, notifier *Notifier, distDir string, selfAddr string) *Server {
	return &Server{
		store: store, cfg: cfg, notifier: notifier, distDir: distDir,
		auth:      NewAuth(cfg),
		checks:    newCheckRunner(cfg, store, notifier, selfAddr),
		term:      newTermManager(),
		emailMgr:  newEmailManager(),
		playbooks: newPlaybookManager(cfg),
		push:      newPushHub(),
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
	mux.HandleFunc("GET /api/v1/mfa/global", s.handleMFAGlobalGet)
	mux.HandleFunc("POST /api/v1/mfa/global", s.handleMFAGlobalSet)
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
