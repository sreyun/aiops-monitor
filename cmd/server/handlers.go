package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
	apimon    *apiRunner       // API 性能监控：按业务系统批量探测接口
	term      *termManager     // remote terminal relay
	forward   *forwardManager  // port forwarding relay (TCP + HTTP proxy)
	emailMgr  *emailManager    // verification codes + reset tokens
	playbooks *playbookManager // automation playbooks + execution history
	push      *pushHub         // P3-1: WebSocket push hub for real-time updates
	// --- SRE workflow layer ---
	incidents   *incidentManager    // incident hub (alert/SLO/manual)
	remediation *remediationManager // closed-loop auto-remediation
	slos        *sloManager         // SLO + error budgets
	tickets     *ticketManager      // work orders
	logs        *logStore           // aggregated agent logs
	hw          *hardwareStore      // latest Redfish snapshots per host (feeds hardware alerts)
	hv          *hypervStore        // latest Hyper-V guest inventory per host (feeds VM alerts)
	snmp        *snmpStore          // latest SNMP device snapshots per host (feeds SNMP alerts)
	nf          *nfStore            // per-host NetFlow window stats + baseline (feeds traffic-anomaly alerts)
	ai          *aiManager          // AI inspection + diagnosis
	vm          *vmWriter           // optional VictoriaMetrics remote-write
	messages    *messageHub         // unified notification center (SRE/alert/AI feed)
	distDir     string              // directory of downloadable agent binaries + plugins.zip
	pg          *pgStore            // PostgreSQL persistence (optional, for pgvector/RAG)
	sreyun      *SreyunCore         // Sreyun Agent (autonomous SRE agent)
	// --- AI 记忆异步写入通道 ---
	memoryCh  chan memoryJob // 异步记忆写入队列
	memorySem chan struct{}  // Embedding API 并发信号量（最多 3 并发）
	memoryWg  sync.WaitGroup // 等待 worker 排空
}

func NewServer(store *Store, cfg *ConfigStore, notifier *Notifier, distDir string, selfAddr string) *Server {
	s := &Server{
		store: store, cfg: cfg, notifier: notifier, distDir: distDir,
		auth:        NewAuth(cfg),
		checks:      newCheckRunner(cfg, store, notifier, selfAddr),
		term:        newTermManager(),
		forward:     newForwardManager(cfg),
		emailMgr:    newEmailManager(),
		playbooks:   newPlaybookManager(cfg),
		push:        newPushHub(),
		incidents:   newIncidentManager(),
		remediation: newRemediationManager(cfg),
		slos:        newSLOManager(cfg),
		tickets:     newTicketManager(),
		logs:        newLogStore(),
		hw:          newHardwareStore(),
		hv:          newHypervStore(),
		snmp:        newSNMPStore(),
		nf:          newNFStore(),
		ai:          newAIManager(cfg),
		vm:          newVMWriter(cfg),
		messages:    newMessageHub(),
	}
	s.checks.vm = s.vm                                            // 拨测结果持久化到 VM（重启后仍可查历史趋势）
	s.apimon = newAPIRunner(s.checks, cfg, store, notifier, s.vm) // API 性能监控（复用高级探测引擎）
	s.wireSRE()
	// Restore persisted TCP forward rules (recreate listeners)
	s.forward.restoreRules(s)
	// Sreyun Agent 引擎是统一「AI 对话」的后端：无条件初始化（仅注册工具定义，很轻）。
	// 能否真正对话由请求时的 AI 配置（cfg.Enabled）决定——见 handleSreyunChat，
	// 未启用时优雅返回提示而非 503。此前 gated on SreyunEnabled&&Enabled 且仅在启动时
	// 判断，导致"配置完模型点 AI 对话仍 503"（s.sreyun 为 nil）。
	s.sreyun = newSreyunCore(s)
	// AI 记忆异步写入 worker pool：3 个 worker，并发上限 3
	s.memoryCh = make(chan memoryJob, 100)
	s.memorySem = make(chan struct{}, 3)
	s.startMemoryWorkers()
	// 记忆生命周期管理定时任务：每天执行衰减 + 清理 + 容量裁剪
	if s.pg != nil {
		go func() {
			// 启动后立即执行一次
			s.pg.decayOldMemories()
			s.pg.cleanupExpiredMemories()
			s.pg.capMemoriesByKind(2000) // 每种 kind 最多 2000 条
			s.pg.cleanupFlowRecords()    // 清理过期 Flow 记录
			// 每 24 小时执行一次
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				s.pg.decayOldMemories()
				s.pg.cleanupExpiredMemories()
				s.pg.capMemoriesByKind(2000)
				s.pg.cleanupFlowRecords()
				// 自进化：把近期高价值经验提炼成可复用技能(SOP)。放在维护循环里而非启动时，
				// 避免每次启动都触发 AI 调用；提炼失败/无新经验时静默跳过（distillSkills 内部已记日志）。
				_, _ = s.distillSkills(14)
			}
		}()
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
	// 重复主机（Agent 重装导致同一台机器出现多条记录）识别与清理
	mux.HandleFunc("GET /api/v1/hosts/duplicates", s.handleHostDuplicates)
	mux.HandleFunc("POST /api/v1/hosts/duplicates/cleanup", s.handleCleanupDuplicates)
	mux.HandleFunc("GET /api/v1/alerts", s.handleAlerts)
	mux.HandleFunc("GET /api/v1/alerts/history", s.handleAlertHistory)
	mux.HandleFunc("POST /api/v1/alerts/ack", s.handleAlertAck)
	mux.HandleFunc("POST /api/v1/alerts/silence", s.handleAlertSilence)
	mux.HandleFunc("GET /api/v1/alerts/governance", s.handleGetGovernance)
	mux.HandleFunc("POST /api/v1/alerts/governance", s.handleSetGovernance)
	mux.HandleFunc("POST /api/v1/alerts/clear", s.handleAlertClear)
	mux.HandleFunc("GET /api/v1/events", s.handleEvents)
	mux.HandleFunc("GET /api/v1/activity", s.handleActivity)
	mux.HandleFunc("GET /api/v1/summary", s.handleSummary)
	mux.HandleFunc("GET /api/v1/weather", s.handleWeather)
	mux.HandleFunc("GET /api/v1/config", s.handleGetConfig)
	mux.HandleFunc("POST /api/v1/config", s.handleSetConfig)
	mux.HandleFunc("POST /api/v1/config/test", s.handleTestConfig)
	mux.HandleFunc("POST /api/v1/login", s.handleLogin)
	// NOTE: POST /api/v1/login/sms-code removed — SMS login is not yet implemented.
	// Re-register when the SMS sending backend is wired.
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
	// API 性能监控：业务系统 + 接口批量监控
	mux.HandleFunc("GET /api/v1/apimon/systems", s.handleAPIMonOverview)
	mux.HandleFunc("POST /api/v1/apimon/systems", s.handleUpsertAPISystem)
	mux.HandleFunc("POST /api/v1/apimon/systems/{id}/run", s.handleRunAPISystem)
	mux.HandleFunc("DELETE /api/v1/apimon/systems/{id}", s.handleDeleteAPISystem)
	mux.HandleFunc("GET /api/v1/apimon/endpoints/{id}/history", s.handleAPIEndpointHistory)
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
	// Log aggregation (agent ingest is fingerprint-authed) + search + diagnosis
	mux.HandleFunc("POST /api/v1/agent/logs", s.handleAgentLogs)
	mux.HandleFunc("GET /api/v1/logs", s.handleSearchLogs)
	mux.HandleFunc("POST /api/v1/logs/diagnose", s.handleLogDiagnose)
	// Notification center (unified message feed)
	mux.HandleFunc("GET /api/v1/messages", s.handleListMessages)
	mux.HandleFunc("POST /api/v1/messages/read", s.handleMarkMessagesRead)
	mux.HandleFunc("POST /api/v1/messages/read-all", s.handleMarkAllMessagesRead)
	// AI: config + inspection + incident diagnosis
	mux.HandleFunc("GET /api/v1/ai/config", s.handleGetAIConfig)
	mux.HandleFunc("POST /api/v1/ai/config", s.handleSetAIConfig)
	mux.HandleFunc("POST /api/v1/ai/test", s.handleTestAIConfig)
	mux.HandleFunc("POST /api/v1/ai/test-embed", s.handleTestEmbedConfig)
	mux.HandleFunc("POST /api/v1/ai/terminal-access", s.handleAITerminalAccess)
	mux.HandleFunc("POST /api/v1/ai/chat", s.handleAIChat)
	mux.HandleFunc("POST /api/v1/ai/assist", s.handleAIAssist)                 // 全站「AI 辅助」按钮统一入口（任务化 SSE）
	mux.HandleFunc("POST /api/v1/ai/assist/feedback", s.handleAIAssistFeedback) // 采纳/评价 AI 辅助结果 → 学习闭环强化
	mux.HandleFunc("GET /api/v1/ai/duty-context", s.handleDutyContext)          // 值班晨报态势汇总（供前端流式生成）
	mux.HandleFunc("GET /api/v1/ai/skills", s.handleListSkills)                 // AI 技能库（自进化提炼产物）
	mux.HandleFunc("DELETE /api/v1/ai/skills/{id}", s.handleDeleteSkill)
	mux.HandleFunc("POST /api/v1/ai/skills/distill", s.handleDistillSkills) // 手动触发技能提炼
	mux.HandleFunc("POST /api/v1/mcp", s.handleMCP)                         // MCP server：外部 Agent 连接本平台只读运维工具（Bearer 鉴权，默认关，POST JSON-RPC）
	mux.HandleFunc("POST /api/v1/ai/models", s.handleAIModels)
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
	// Sreyun Agent — 自主运维 Agent
	mux.HandleFunc("POST /api/v1/hermes/chat", s.handleSreyunChat)
	mux.HandleFunc("GET /api/v1/hermes/suggestions", s.handleSreyunSuggestions)
	mux.HandleFunc("POST /api/v1/hermes/parse", s.handleSreyunParse)
	mux.HandleFunc("GET /api/v1/hermes/sessions", s.handleSreyunSessions)
	mux.HandleFunc("GET /api/v1/hermes/sessions/{id}", s.handleSreyunSession)
	mux.HandleFunc("POST /api/v1/hermes/sessions/{id}/undo", s.handleSreyunSessionUndo)
	mux.HandleFunc("GET /api/v1/hermes/rules", s.handleSreyunListRules)
	mux.HandleFunc("POST /api/v1/hermes/rules", s.handleSreyunUpsertRule)
	mux.HandleFunc("DELETE /api/v1/hermes/rules/{id}", s.handleSreyunDeleteRule)
	mux.HandleFunc("GET /api/v1/hermes/templates", s.handleSreyunListTemplates)
	mux.HandleFunc("POST /api/v1/hermes/templates", s.handleSreyunUpsertTemplate)
	mux.HandleFunc("DELETE /api/v1/hermes/templates/{id}", s.handleSreyunDeleteTemplate)
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
	// 端口范围批量组：整组删除 / 启停 / 复制 / 编辑（避免几百条逐条操作）
	mux.HandleFunc("DELETE /api/v1/forward/group/{gid}", s.handleForwardGroupDelete)
	mux.HandleFunc("PUT /api/v1/forward/group/{gid}/toggle", s.handleForwardGroupToggle)
	mux.HandleFunc("POST /api/v1/forward/group/{gid}/copy", s.handleForwardGroupCopy)
	mux.HandleFunc("PUT /api/v1/forward/group/{gid}/edit", s.handleForwardGroupEdit)
	mux.HandleFunc("GET /api/v1/forward/stats", s.handleForwardStats)
	mux.HandleFunc("GET /api/v1/forward/health", s.handleForwardHealth)
	// HTTP proxy shortcuts (saved configs)
	mux.HandleFunc("GET /api/v1/http-proxy", s.handleHTTPProxyList)
	mux.HandleFunc("POST /api/v1/http-proxy", s.handleHTTPProxyCreate)
	mux.HandleFunc("DELETE /api/v1/http-proxy/{id}", s.handleHTTPProxyDelete)
	mux.HandleFunc("PUT /api/v1/http-proxy/{id}", s.handleHTTPProxyEdit)
	mux.HandleFunc("PUT /api/v1/http-proxy/{id}/toggle", s.handleHTTPProxyToggle)
	mux.HandleFunc("POST /api/v1/http-proxy/{id}/copy", s.handleHTTPProxyCopy)
	// External data sources (Loki / Prometheus): AI query + log search + alert queries
	mux.HandleFunc("GET /api/v1/datasources", s.handleDataSourceList)
	mux.HandleFunc("POST /api/v1/datasources", s.handleDataSourceCreate)
	mux.HandleFunc("POST /api/v1/datasources/test", s.handleDataSourceTest)
	mux.HandleFunc("PUT /api/v1/datasources/{id}", s.handleDataSourceUpdate)
	mux.HandleFunc("DELETE /api/v1/datasources/{id}", s.handleDataSourceDelete)
	mux.HandleFunc("POST /api/v1/datasources/{id}/query", s.handleDataSourceQuery)
	mux.HandleFunc("GET /api/v1/datasources/{id}/labels", s.handleDataSourceLabels)
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
	// Hardware + NetFlow: agent ingest (fingerprint-gated)
	mux.HandleFunc("POST /api/v1/agent/hardware", s.handleAgentHardware)
	mux.HandleFunc("POST /api/v1/agent/netflow", s.handleAgentNetFlow)
	// SNMP: agent ingest (fingerprint-gated)
	mux.HandleFunc("POST /api/v1/agent/snmp", s.handleAgentSNMP)
	mux.HandleFunc("POST /api/v1/agent/snmp/trap", s.handleAgentSNMPTrap)
	// Hardware + NetFlow: frontend query
	mux.HandleFunc("GET /api/v1/hardware/health", s.handleHardwareHealth)
	mux.HandleFunc("GET /api/v1/hardware/history", s.handleHardwareHistory)
	mux.HandleFunc("GET /api/v1/hardware/events", s.handleHardwareEvents)
	mux.HandleFunc("DELETE /api/v1/hardware/{hostID}", s.handleDeleteHardware)
	// Hyper-V 虚拟机: agent ingest (fingerprint-gated) + frontend query
	mux.HandleFunc("POST /api/v1/agent/hyperv", s.handleAgentHyperV)
	mux.HandleFunc("GET /api/v1/hyperv/list", s.handleHyperVList)
	mux.HandleFunc("GET /api/v1/hyperv/events", s.handleHyperVEvents)
	mux.HandleFunc("DELETE /api/v1/hyperv/{hostID}", s.handleDeleteHyperV)
	mux.HandleFunc("GET /api/v1/netflow/summary", s.handleNetFlowSummary)
	mux.HandleFunc("GET /api/v1/netflow/flows", s.handleNetFlowFlows)
	mux.HandleFunc("GET /api/v1/netflow/packets", s.handleNetFlowPackets)
	// SNMP: frontend query
	mux.HandleFunc("GET /api/v1/snmp/list", s.handleSNMPList)
	mux.HandleFunc("GET /api/v1/snmp/interface-history", s.handleSNMPInterfaceHistory)
	mux.HandleFunc("GET /api/v1/snmp/traps", s.handleSNMPTraps)
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
		// /app.js: 把 web/js/ 下的 8 个源模块按依赖顺序拼成【单个脚本】返回。
		// 必须作为单脚本加载——整文件函数提升(hoisting)才生效；若用 8 个独立
		// <script> 标签，早模块顶层调用晚模块里定义的 helper/handler 会因
		// 每脚本独立提升而 ReferenceError。源码保持拆分(便于维护)，运行时=单文件。
		mux.HandleFunc("GET /app.js", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			for _, m := range []string{"core", "export", "duplicates", "overview", "hosts", "terminal", "settings", "nav", "sre", "ai-assist", "apimon", "governance", "datasource", "hardware", "hyperv", "netflow", "snmp", "init"} {
				b, err := webFS.ReadFile("web/js/" + m + ".js")
				if err != nil {
					http.Error(w, "js module missing: "+m, http.StatusInternalServerError)
					return
				}
				_, _ = w.Write(b)
				_, _ = w.Write([]byte("\n;\n")) // 模块间安全分隔（空语句），防 ASI 边界问题
			}
		})
		mux.Handle("GET /theme-init.js", fsrv) // 主题预置（外置内联脚本，配合 CSP 去 unsafe-inline）
		mux.Handle("GET /i18n-dashboard.js", fsrv)
		mux.Handle("GET /i18n-dashboard.en.js", fsrv)
		mux.Handle("GET /i18n-dashboard.zh-TW.js", fsrv)
		// P2-1: support split CSS/JS modules
		// 注意：不能 StripPrefix——文件在 web/js、web/css 子目录下，需保留前缀映射到子目录。
		mux.Handle("GET /css/", fsrv)
		mux.Handle("GET /js/", fsrv)
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
		mux.HandleFunc("GET /dl/", s.handleDownload)
	}
	return mux
}

// handleDownload serves agent binaries / plugins.zip from distDir with strong
// caching so re-installs and多机 installs don't re-download the full 7.5MB every
// time. http.ServeContent 负责 Range(断点续传)+If-None-Match/If-Modified-Since
// (条件 GET→304)，我们只需补上 ETag 与 Cache-Control：
//   - ETag=size-mtime 指纹：内容不变则客户端/CDN 命中 304，只回 header 不回 body。
//   - Cache-Control: public,max-age —— 让 CDN/relay 边缘缓存；用 max-age+ETag 而非
//     immutable，因为发版后同名 URL 内容会变，必须允许重新校验才能拿到新版 agent。
// gzip 中间件已对 /dl/ 前缀 bypass（二进制本就是压缩态，再 gzip 无益且破坏 Range）。
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/dl/")
	if name == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// 防目录穿越：Clean("/"+name) 消解 ../，再 Join 到 distDir。
	full := filepath.Join(s.distDir, filepath.Clean("/"+name))
	f, err := os.Open(full)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("ETag", fmt.Sprintf(`"%x-%x"`, fi.Size(), fi.ModTime().UnixNano()))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
