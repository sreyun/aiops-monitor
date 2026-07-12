package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// Hermes Agent — 自主运维 Agent 引擎
//
// 三层解耦架构：
//   Layer 1 (本文件): 引擎核心 — 观察→推理→行动循环 + Function Calling
//   Layer 2 (pgstore): 规则库 + 提示模板 — 热加载，无需重启
//   Layer 3 (plugins): Python 动作插件 — 动态加载，上传即生效
//
// 复用现有基础设施：aiChat/streamChat (LLM 调用)、pgStore (RAG/持久化)、
// playbookManager (远程执行)、logStore (日志检索)、notifier (告警查询)
// ============================================================================

// HermesTool defines a callable function that the LLM can invoke.
type HermesTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Execute     func(args map[string]any) (string, error)
}

// HermesSession represents one conversation session.
type HermesSession struct {
	ID         int64
	IncidentID int64
	Messages   []map[string]string
	CreatedAt  int64
}

// HermesCore is the autonomous agent engine.
type HermesCore struct {
	s     *Server
	tools map[string]HermesTool
	// Cached config (hot-reloaded from PG)
	configMu        sync.RWMutex
	cachedRules     []hermesRule
	cachedTemplates []hermesTemplate
	lastLoad        time.Time
}

// newHermesCore creates and initializes the Hermes engine.
func newHermesCore(s *Server) *HermesCore {
	h := &HermesCore{s: s, tools: make(map[string]HermesTool)}
	h.registerTools()
	return h
}

// registerTools registers all built-in tools the LLM can call.
func (h *HermesCore) registerTools() {
	h.tools["query_metrics"] = HermesTool{
		Name:        "query_metrics",
		Description: "查询主机的实时性能指标，返回 CPU/内存/磁盘/负载/网络/IO 等",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "主机 ID"},
				"metric":  map[string]string{"type": "string", "description": "要查询的指标类别：cpu/memory/disk/load/network/io/all"},
			},
			"required": []string{"host_id"},
		},
		Execute: h.execQueryMetrics,
	}
	h.tools["search_logs"] = HermesTool{
		Name:        "search_logs",
		Description: "搜索主机日志，支持按级别和关键词过滤",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id":  map[string]string{"type": "string", "description": "主机 ID"},
				"level":    map[string]string{"type": "string", "description": "日志级别：error/warn/info"},
				"keyword":  map[string]string{"type": "string", "description": "搜索关键词"},
				"minutes":  map[string]any{"type": "integer", "description": "最近 N 分钟"},
			},
			"required": []string{"host_id"},
		},
		Execute: h.execSearchLogs,
	}
	h.tools["list_alerts"] = HermesTool{
		Name:        "list_alerts",
		Description: "获取当前活跃告警列表",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "主机 ID（可选，不传则返回全部）"},
			},
		},
		Execute: h.execListAlerts,
	}
	h.tools["search_similar_cases"] = HermesTool{
		Name:        "search_similar_cases",
		Description: "在历史案例库中搜索相似故障（RAG 向量检索）",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]string{"type": "string", "description": "描述故障现象的关键词或句子"},
			},
			"required": []string{"query"},
		},
		Execute: h.execSearchCases,
	}
	h.tools["run_diagnostic"] = HermesTool{
		Name:        "run_diagnostic",
		Description: "登录目标主机执行【只读】诊断命令做巡检排查（如 top/df/free/ps/ss/journalctl/cat 日志 等，可用管道过滤）。严格只读：禁止任何增、删、改、重启、写文件或隐藏操作，系统会强制拦截非白名单命令。需用户先在 AI 设置中开启「AI 终端巡检」权限。返回命令输出。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "主机 ID"},
				"command": map[string]string{"type": "string", "description": "只读诊断命令，如 'top -bn1 | head -20'、'df -h'、'journalctl -u nginx --no-pager | tail -50'"},
			},
			"required": []string{"host_id", "command"},
		},
		Execute: h.execDiagnostic,
	}
	h.tools["run_python_action"] = HermesTool{
		Name:        "run_python_action",
		Description: "执行 Python 插件中的自定义动作（如重启服务、清理缓存、扩缩容脚本等）",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action_name": map[string]string{"type": "string", "description": "动作名称，对应 plugins/ 下的 hermes_actions.py 中定义的函数"},
				"host_id":     map[string]string{"type": "string", "description": "目标主机 ID"},
				"args":        map[string]string{"type": "string", "description": "传递给动作的参数（JSON 字符串）"},
			},
			"required": []string{"action_name"},
		},
		Execute: h.execPythonAction,
	}
}

// --- Tool implementations ---

// resolveHostRef 按 host_id 精确匹配主机；失败则回退到主机名 / IP（忽略大小写，再退化到
// 主机名包含匹配），让 AI 即便把主机名或 IP 当作 host_id 传入也能命中正确主机。
func (h *HermesCore) resolveHostRef(ref string) *Host {
	ref = strings.TrimSpace(ref)
	if ref == "" || h.s == nil {
		return nil
	}
	if hst := h.s.hostByID(ref); hst != nil {
		return hst
	}
	if h.s.store == nil {
		return nil
	}
	hosts := h.s.store.ListHosts()
	low := strings.ToLower(ref)
	for _, hst := range hosts { // 精确匹配主机名 / IP
		if strings.ToLower(hst.Hostname) == low || strings.ToLower(hst.IP) == low {
			return hst
		}
	}
	for _, hst := range hosts { // 宽松：主机名包含
		if hst.Hostname != "" && strings.Contains(strings.ToLower(hst.Hostname), low) {
			return hst
		}
	}
	return nil
}

func (h *HermesCore) execQueryMetrics(args map[string]any) (string, error) {
	hostID, _ := args["host_id"].(string)
	metric, _ := args["metric"].(string)
	if metric == "" {
		metric = "all"
	}
	hst := h.resolveHostRef(hostID)
	if hst == nil {
		return fmt.Sprintf("未找到主机 %q（可查询的主机见系统提示中的主机清单）", hostID), nil
	}
	if hst.Latest == nil {
		return fmt.Sprintf("主机 %s 当前不在线或暂无指标数据", hst.Hostname), nil
	}
	m := hst.Latest
	var b strings.Builder
	fmt.Fprintf(&b, "主机 %s 指标：\n", hst.Hostname)
	if metric == "all" || metric == "cpu" {
		fmt.Fprintf(&b, "  CPU: %.1f%%\n", m.CPUPercent)
	}
	if metric == "all" || metric == "memory" {
		fmt.Fprintf(&b, "  内存: %.1f%% (%d/%d GB)", m.MemPercent, m.MemUsed/1073741824, m.MemTotal/1073741824)
		if m.SwapTotal > 0 {
			fmt.Fprintf(&b, " | SWAP %.1f%%", m.SwapPercent)
		}
		b.WriteByte('\n')
	}
	if metric == "all" || metric == "disk" {
		fmt.Fprintf(&b, "  磁盘: %.1f%%\n", m.DiskPercent)
		for _, d := range m.Disks {
			fmt.Fprintf(&b, "    %s: %.1f%%\n", d.Path, d.Percent)
		}
	}
	if metric == "all" || metric == "load" {
		fmt.Fprintf(&b, "  负载: %.2f/%.2f/%.2f | 进程数: %d\n", m.Load1, m.Load5, m.Load15, m.ProcCount)
	}
	if metric == "all" || metric == "network" {
		fmt.Fprintf(&b, "  网络: ↓%.1f ↑%.1f MB/s\n", m.NetRecvRate/1048576, m.NetSentRate/1048576)
	}
	if metric == "all" || metric == "io" {
		if m.DiskReadRate+m.DiskWriteRate > 0 {
			fmt.Fprintf(&b, "  磁盘IO: ↓%.1f ↑%.1f MB/s | IOPS r%.0f/w%.0f\n",
				m.DiskReadRate/1048576, m.DiskWriteRate/1048576, m.DiskReadIOPS, m.DiskWriteIOPS)
		}
	}
	if m.Uptime > 0 {
		fmt.Fprintf(&b, "  运行时间: %s\n", formatUptime(m.Uptime))
	}
	return b.String(), nil
}

func (h *HermesCore) execSearchLogs(args map[string]any) (string, error) {
	hostID, _ := args["host_id"].(string)
	level, _ := args["level"].(string)
	keyword, _ := args["keyword"].(string)
	minutes := 5
	if v, ok := args["minutes"].(float64); ok {
		minutes = int(v)
	}
	if h.s.logs == nil {
		return "日志存储不可用", nil
	}
	name := hostID
	if hst := h.resolveHostRef(hostID); hst != nil {
		hostID, name = hst.ID, hst.Hostname
	}
	since := time.Now().Unix() - int64(minutes*60)
	entries := h.s.logs.search(hostID, level, keyword, since, 10)
	if len(entries) == 0 {
		return fmt.Sprintf("主机 %s 最近 %d 分钟无匹配日志", name, minutes), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "主机 %s 最近 %d 分钟日志（%d 条）：\n", name, minutes, len(entries))
	for _, e := range entries {
		ts := time.Unix(e.Ts, 0).Format("15:04:05")
		fmt.Fprintf(&b, "  [%s %s] %s\n", ts, strings.ToUpper(e.Level), trimLine(e.Message, 200))
	}
	return b.String(), nil
}

func (h *HermesCore) execListAlerts(args map[string]any) (string, error) {
	hostID, _ := args["host_id"].(string)
	if h.s.notifier == nil {
		return "告警系统不可用", nil
	}
	if hostID != "" {
		if hst := h.resolveHostRef(hostID); hst != nil {
			hostID = hst.ID
		}
	}
	var matched []string
	for _, a := range h.s.notifier.ActiveAlerts() {
		if a.Status != "" {
			continue
		}
		if hostID != "" && a.HostID != hostID {
			continue
		}
		hn := a.Hostname
		if hn == "" {
			hn = a.HostID
		}
		matched = append(matched, fmt.Sprintf("  %s (%s, %.1f) — 主机 %s", a.Type, a.Level, a.Value, hn))
	}
	if len(matched) == 0 {
		return "当前无活跃告警", nil
	}
	if len(matched) > 10 {
		matched = matched[:10]
	}
	return "当前活跃告警：\n" + strings.Join(matched, "\n"), nil
}

func (h *HermesCore) execSearchCases(args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "请输入故障描述", nil
	}
	if h.s.pg == nil {
		return "RAG 向量存储不可用（需要 PostgreSQL + pgvector）", nil
	}
	cfg := h.s.cfg.AIConfig()
	if !cfg.Enabled || cfg.APIKey == "" {
		return "AI 未启用，无法进行向量检索", nil
	}
	emb := embedText(cfg, query)
	if len(emb) == 0 {
		return "向量嵌入失败", nil
	}
	cases, err := h.s.pg.searchSimilarCases(emb, 3)
	if err != nil || len(cases) == 0 {
		return "未找到相似历史案例", nil
	}
	var b strings.Builder
	b.WriteString("相似历史案例：\n")
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
		fmt.Fprintf(&b, "  %d. 相似度 %d%%%s: %s\n", i+1, sim, fb, trimLine(c.Summary, 250))
	}
	return b.String(), nil
}

// diagCommandAllowed 校验诊断命令是否为「只读命令 + 只读管道过滤」。
// 命令在被控端经 shell 执行，故：
//  1. 拒绝可用于注入 / 破坏 / 外联 / 重定向 / 子shell 的元字符（; & $ < > ` \ 换行 () {}）；
//  2. 允许用管道 | 串联，但逐段校验每段命令首词必须精确命中白名单（非松散前缀，dfoo 不算 df）。
// 返回 (是否放行, 拒绝原因)。
func diagCommandAllowed(command string) (bool, string) {
	cmdTrim := strings.TrimSpace(command)
	if cmdTrim == "" {
		return false, "请指定诊断命令"
	}
	if strings.ContainsAny(cmdTrim, ";&$<>\n\r\\(){}"+"`") {
		return false, "诊断命令含被禁止的字符（; & $ < > ` 等），仅允许只读命令与管道过滤"
	}
	allow := []string{
		"top", "df", "iostat", "vmstat", "mpstat", "sar", "pidstat", "netstat", "ss", "free",
		"ps", "uptime", "cat", "head", "tail", "grep", "egrep", "ls", "du", "lsof", "dmesg",
		"journalctl", "systemctl status", "docker ps", "docker logs", "docker stats",
		"kubectl get", "kubectl describe", "wc", "sort", "uniq", "cut", "tr", "nl", "tac",
		"column", "date", "hostname", "uname", "who", "w",
	}
	segOK := func(seg string) bool {
		seg = strings.ToLower(strings.TrimSpace(seg))
		for _, p := range allow {
			if seg == p || strings.HasPrefix(seg, p+" ") {
				return true
			}
		}
		return false
	}
	for _, seg := range strings.Split(cmdTrim, "|") {
		if !segOK(seg) {
			return false, fmt.Sprintf("诊断命令 %q 含非白名单命令，仅允许只读诊断命令（top/df/free/ps/ss/cat/grep/journalctl 等）及其管道过滤", command)
		}
	}
	return true, ""
}

func (h *HermesCore) execDiagnostic(args map[string]any) (string, error) {
	hostID, _ := args["host_id"].(string)
	command, _ := args["command"].(string)
	if command == "" {
		return "请指定诊断命令", nil
	}
	// 门控：AI 终端只读巡检为独立高风险授权，需用户在 AI 设置中显式开启（并已校验终端密码）后才可执行主机命令。
	if !h.s.cfg.AIConfig().HermesTerminalEnabled {
		return "AI 终端只读巡检权限未开启。如需让我登录终端做只读排查，请在「AI 设置 → AI 终端巡检」中开启（需输入终端连接密码）。", nil
	}
	// Use playbook mechanism to execute read-only diagnostic commands
	host := h.resolveHostRef(hostID)
	if host == nil {
		return fmt.Sprintf("未找到主机 %q", hostID), nil
	}
	hostID = host.ID
	// 安全校验：命令最终在被控端经 shell 执行，前缀白名单不足以防注入。详见 diagCommandAllowed。
	if ok, reason := diagCommandAllowed(command); !ok {
		return reason, nil
	}
	// Run via playbook executor (one-shot command)
	pb := Playbook{
		Name: "hermes-diagnostic",
		Steps: []PlaybookStep{{
			Name:       "diagnostic-cmd",
			Command:    command,
			Target:     "host:" + hostID,
			TimeoutSec: 15,
		}},
	}
	done := make(chan bool, 1)
	execID := h.s.triggerPlaybookOnHost(pb, host, "hermes", func(ok bool) {
		done <- ok
	})
	select {
	case ok := <-done:
		// Retrieve execution output
		if exec, found := h.s.playbooks.GetExecution(execID); found {
			if hr, ok2 := exec.HostResults[hostID]; ok2 && hr.Output != "" {
				return fmt.Sprintf("诊断命令输出：\n%s", hr.Output), nil
			}
		}
		if ok {
			return "诊断命令执行完成（无输出）", nil
		}
		return "诊断命令执行失败", nil
	case <-time.After(20 * time.Second):
		return "诊断命令执行超时", nil
	}
}

func (h *HermesCore) execPythonAction(args map[string]any) (string, error) {
	actionName, _ := args["action_name"].(string)
	hostID, _ := args["host_id"].(string)
	argStr, _ := args["args"].(string)
	if actionName == "" {
		return "请指定动作名称", nil
	}
	// run_python_action 属于「写操作」（重启服务 / 清理缓存 / 扩缩容等）。仅在显式开启
	// HermesAutoApprove（低风险自动执行）时才真正执行，否则挂起并返回需人工确认，
	// 避免 AI 未经批准擅自变更主机（此前该配置从未在执行路径生效，属安全缺口）。
	if !h.s.cfg.AIConfig().HermesAutoApprove {
		return fmt.Sprintf("动作 %q 属于高风险写操作，需人工确认。当前未开启「自动执行」(hermes_auto_approve)，已阻止自动执行；请操作员手动处置，或在 AI 设置中开启后重试。", actionName), nil
	}
	// 加 30s 超时，避免插件脚本卡死导致请求 goroutine 永久阻塞
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "plugins/hermes_actions.py", actionName, hostID, argStr)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("动作 %q 执行超时（30s）", actionName), nil
	}
	if err != nil {
		slog.Warn("hermes python action failed", "action", actionName, "err", err, "output", string(output))
		return fmt.Sprintf("动作 %q 执行失败：%v\n输出：%s", actionName, err, string(output)), nil
	}
	return string(output), nil
}

// --- Core loop: Observe → Reason → Act ---

// Chat runs a Hermes conversation turn with Function Calling support.
// If stream is true, it writes SSE events to w; otherwise returns the reply.
func (h *HermesCore) Chat(ctx context.Context, session *HermesSession, userMsg string, images []chatImage, stream bool, w http.ResponseWriter) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := h.s.cfg.AIConfig()
	if !cfg.Enabled || cfg.Endpoint == "" || cfg.Model == "" {
		return "", fmt.Errorf("AI 未配置或未启用")
	}

	// Build system prompt from cached templates + rules
	sys := h.buildSystemPrompt()

	// Build messages
	msgs := []map[string]string{{"role": "system", "content": sys}}
	msgs = append(msgs, session.Messages...)
	msgs = append(msgs, map[string]string{"role": "user", "content": userMsg})

	// Execute the observe→reason→act loop
	fullReply, err := h.runLoop(ctx, cfg, msgs, images, stream, w)
	if err != nil {
		return "", err
	}
	// Update session
	session.Messages = append(session.Messages,
		map[string]string{"role": "user", "content": userMsg},
		map[string]string{"role": "assistant", "content": fullReply},
	)
	// Persist to PG
	if h.s.pg != nil {
		raw, _ := json.Marshal(session.Messages)
		newID, err := h.s.pg.saveHermesSession(session.ID, raw, session.IncidentID)
		if err == nil && newID > 0 {
			session.ID = newID
		}
	}
	return fullReply, nil
}

// runLoop implements the core observe→reason→act loop with Function Calling.
// 每轮都以【非流式】方式调用 LLM，以便可靠解析 tool_calls：流式模式难以在 token 中途
// 判定是否为工具调用，且 streamChat 每次调用都会发送 [DONE]，会使前端在多轮工具调用中途
// 提前结束、看不到最终结论。面向用户只推送「思考文字 + 工具执行状态 + 最终结论」，
// 工具调用的原始 JSON 绝不下发到前端。max 5 turns to prevent infinite loops.
func (h *HermesCore) runLoop(ctx context.Context, cfg AIConfig, msgs []map[string]string, images []chatImage, stream bool, w http.ResponseWriter) (string, error) {
	flusher, _ := w.(http.Flusher)
	sendDelta := func(text string) {
		if !stream || w == nil || text == "" {
			return
		}
		fmt.Fprintf(w, "data: {\"delta\":%s}\n\n", jsonString(text))
		if flusher != nil {
			flusher.Flush()
		}
	}

	for turn := 0; turn < 5; turn++ {
		if err := ctx.Err(); err != nil { // 客户端已断开：停止后续 LLM 调用与工具执行，避免用户离开后仍在主机上跑命令
			return "", err
		}
		msgsWithTools := h.injectTools(msgs)
		reply, err := aiChatV(ctx, cfg, msgsWithTools, images) // 带 ctx（可中止）+ 图片（多模态）
		if err != nil {
			if stream && w != nil {
				fmt.Fprintf(w, "data: {\"error\":%s}\n\n", jsonString(err.Error()))
				if flusher != nil {
					flusher.Flush()
				}
			}
			return "", err
		}

		toolCalls := h.parseToolCalls(reply)
		if len(toolCalls) == 0 {
			// 无工具调用 = 最终结论。剥掉可能残留的 JSON/代码块后下发。
			final := stripToolCallJSON(reply)
			if final == "" {
				final = strings.TrimSpace(reply)
			}
			sendDelta(final)
			return final, nil
		}

		// 有工具调用：先把模型的「思考文字」（JSON 之前的自然语言）推给用户
		if think := stripToolCallJSON(reply); think != "" {
			sendDelta(think + "\n")
		}
		// 推送工具执行状态（不含 JSON）
		names := make([]string, 0, len(toolCalls))
		for _, tc := range toolCalls {
			names = append(names, tc.Name)
		}
		sendDelta("🔧 正在执行：" + strings.Join(names, "、") + " …\n")

		// 执行工具，汇总结果
		var toolResults strings.Builder
		for _, tc := range toolCalls {
			slog.Info("hermes tool call", "tool", tc.Name, "args", fmt.Sprintf("%v", tc.Args))
			tool, ok := h.tools[tc.Name]
			if !ok {
				toolResults.WriteString(fmt.Sprintf("[工具 %s 不存在]\n", tc.Name))
				continue
			}
			result, err := tool.Execute(tc.Args)
			if err != nil {
				toolResults.WriteString(fmt.Sprintf("[工具 %s 执行失败：%v]\n", tc.Name, err))
			} else {
				toolResults.WriteString(fmt.Sprintf("工具 %s 结果：\n%s\n", tc.Name, result))
			}
		}
		// 把模型本轮的工具调用 + 真实结果追加进上下文，供下一轮推理
		msgs = append(msgs,
			map[string]string{"role": "assistant", "content": reply},
			map[string]string{"role": "user", "content": fmt.Sprintf("工具执行结果：\n%s\n请根据以上真实结果继续分析；若信息足够，请直接用简洁中文给出最终结论，不要再输出 JSON。", toolResults.String())},
		)
	}
	// 达到最大轮次仍未收敛：强制「不再调用工具」再问一次，逼出最终结论并正常落库，
	// 避免既跑满工具又丢弃已获取信息、还不给用户任何结论。
	msgs = append(msgs, map[string]string{"role": "user", "content": "已达到工具调用次数上限。请不要再调用任何工具，直接基于以上已获取的真实信息，用简洁中文给出你的最终结论与处置建议。"})
	final, err := aiChatV(ctx, cfg, msgs, images) // 不注入工具定义，强制收敛为自然语言结论
	if err != nil {
		sendDelta("分析未能在限定轮次内收敛，请缩小问题范围后重试。")
		return "", err
	}
	final = stripToolCallJSON(final)
	if final == "" {
		final = "分析未能得出明确结论，请补充信息后重试。"
	}
	sendDelta(final)
	return final, nil
}

// toolCall represents a parsed tool call from the LLM response.
type toolCall struct {
	Name string
	Args map[string]any
}

// injectTools adds tool definitions to the last system message.
func (h *HermesCore) injectTools(msgs []map[string]string) []map[string]string {
	// Build tool definitions JSON（按工具名排序，保证注入顺序稳定，利于 Provider prompt 缓存与可复现）
	names := make([]string, 0, len(h.tools))
	for name := range h.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	var toolDefs []map[string]any
	for _, name := range names {
		t := h.tools[name]
		toolDefs = append(toolDefs, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			},
		})
	}
	toolsJSON, _ := json.Marshal(toolDefs)

	// Find the system message and append tool definitions
	result := make([]map[string]string, len(msgs))
	copy(result, msgs)
	for i, m := range result {
		if m["role"] == "system" {
			result[i] = map[string]string{
				"role":    "system",
				"content": m["content"] + "\n\n你可以使用以下工具来获取信息或执行操作。当需要调用工具时，请用以下 JSON 格式回复：\n```json\n{\"tool_calls\":[{\"name\":\"工具名\",\"args\":{参数}}]}\n```\n\n可用工具定义：\n" + string(toolsJSON),
			}
			break
		}
	}
	return result
}

// parseToolCalls extracts tool calls from the LLM response text.
func (h *HermesCore) parseToolCalls(text string) []toolCall {
	// Look for JSON tool_calls block in the response
	text = strings.TrimSpace(text)
	// Try to find ```json ... ``` block
	start := strings.Index(text, "```json")
	if start < 0 {
		// Try to find raw JSON
		start = strings.Index(text, `{"tool_calls"`)
		if start < 0 {
			return nil
		}
		// 按花括号配平提取完整 JSON（支持模型输出的多行美化 JSON，不再按首个换行截断）
		depth, endPos := 0, -1
		for i := start; i < len(text); i++ {
			switch text[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					endPos = i + 1
				}
			}
			if endPos >= 0 {
				break
			}
		}
		if endPos < 0 {
			return nil
		}
		return h.parseToolCallJSON(text[start:endPos])
	}
	start += 7 // skip ```json
	end := strings.Index(text[start:], "```")
	if end < 0 {
		return nil
	}
	jsonStr := strings.TrimSpace(text[start : start+end])
	return h.parseToolCallJSON(jsonStr)
}

func (h *HermesCore) parseToolCallJSON(jsonStr string) []toolCall {
	var wrapper struct {
		ToolCalls []struct {
			Name string         `json:"name"`
			Args map[string]any `json:"args"`
		} `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &wrapper); err != nil {
		slog.Warn("hermes failed to parse tool call JSON", "json", jsonStr, "err", err)
		return nil
	}
	var out []toolCall
	for _, tc := range wrapper.ToolCalls {
		out = append(out, toolCall{Name: tc.Name, Args: tc.Args})
	}
	return out
}

// stripToolCallJSON 去掉模型回复中的 ```代码块``` 与 {"tool_calls":...} JSON，
// 只保留自然语言（思考文字 / 最终结论），用于下发给前端展示——工具调用 JSON 对用户不可见。
func stripToolCallJSON(text string) string {
	t := text
	// 1) 去掉所有 ``` ... ``` 代码块（含 ```json）
	for {
		start := strings.Index(t, "```")
		if start < 0 {
			break
		}
		rest := t[start+3:]
		end := strings.Index(rest, "```")
		if end < 0 {
			t = t[:start] // 未闭合：删到结尾
			break
		}
		t = t[:start] + t[start+3+end+3:]
	}
	// 2) 去掉裸 {"tool_calls" ... } —— 按花括号配平找到该 JSON 的结束位置
	for {
		idx := strings.Index(t, "{\"tool_calls\"")
		if idx < 0 {
			// 兼容含空格的写法 {"tool_calls" 前可能有空白，宽松再找一次
			idx = indexToolCallsLoose(t)
			if idx < 0 {
				break
			}
		}
		depth, endPos := 0, -1
		for i := idx; i < len(t); i++ {
			switch t[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					endPos = i + 1
				}
			}
			if endPos >= 0 {
				break
			}
		}
		if endPos > 0 {
			t = t[:idx] + t[endPos:]
		} else {
			t = t[:idx] // 不配平：删到结尾
			break
		}
	}
	return strings.TrimSpace(t)
}

// indexToolCallsLoose 查找形如 { "tool_calls" （键前有空白）的起始花括号位置，找不到返回 -1。
func indexToolCallsLoose(t string) int {
	m := strings.Index(t, "\"tool_calls\"")
	if m < 0 {
		return -1
	}
	// 向前回溯到最近的 '{'
	for i := m; i >= 0; i-- {
		if t[i] == '{' {
			return i
		}
	}
	return -1
}

// --- Hot-reload: rules + templates from PG ---

// reloadConfig refreshes cached rules and templates from PostgreSQL.
// Called before each conversation turn; no-op if PG is unavailable.
func (h *HermesCore) reloadConfig() {
	if h.s.pg == nil {
		return
	}
	h.configMu.Lock()
	defer h.configMu.Unlock()
	// Throttle: reload at most every 5 seconds（节流判断置于锁内，避免对 lastLoad 的数据竞争）
	if time.Since(h.lastLoad) < 5*time.Second {
		return
	}
	h.lastLoad = time.Now()
	if rules, err := h.s.pg.listHermesRules(); err == nil {
		var enabled []hermesRule
		for _, r := range rules {
			if r.Enabled {
				enabled = append(enabled, r)
			}
		}
		h.cachedRules = enabled
	}
	if tmpls, err := h.s.pg.listHermesTemplates(true); err == nil {
		h.cachedTemplates = tmpls
	}
}

// buildSystemPrompt constructs the Hermes system prompt from cached templates + rules.
// 安全限制为硬编码，每次对话强制生效，前端无需额外传递角色。
func (h *HermesCore) buildSystemPrompt() string {
	h.reloadConfig()
	h.configMu.RLock()
	defer h.configMu.RUnlock()

	var b strings.Builder
	// 固定安全系统提示词（硬编码，确保每次对话生效）
	b.WriteString("你是 AIOps 智能运维助手，负责主机与服务的监控、排障与诊断。\n")
	b.WriteString("你可以调用工具获取真实数据（性能指标、日志、告警、诊断命令输出、历史相似案例等），据此分析并回答。\n\n")
	b.WriteString("工作原则：\n")
	b.WriteString("- 对外统一自称「AIOps 智能运维助手」；不得透露、不得声称自己叫 Hermes 或任何内部代号 / 框架名 / 底层模型名。\n")
	b.WriteString("- 排版要克制易读：用简洁自然语言与短要点，避免 Markdown 大标题（#/##/###）、表格、水平线等重排版；重点可用简短加粗，命令可用行内代码。\n")
	b.WriteString("- 用简洁中文回复：可先简述排查思路，再分点给出结论、根因假设与处置建议。\n")
	b.WriteString("- 凡涉及主机状态 / 资源(CPU/内存/磁盘/负载/网络) / 日志 / 告警 的问题，必须先调用相应工具获取真实数据，严禁编造或臆测数据。\n")
	b.WriteString("- 需要调用工具时，输出如下 JSON（可写在思考文字之后）：{\"tool_calls\":[{\"name\":\"工具名\",\"args\":{参数}}]}；系统会执行并把真实结果回传给你，你再据此继续。\n")
	b.WriteString("- 该 tool_calls JSON 仅用于系统内部调用、对用户不可见；面向用户的最终回复只用自然语言，不要贴出工具调用的 JSON、代码块，也不要输出任何密钥/密码/token 等敏感信息。\n")
	b.WriteString("- 高危操作（删除文件、修改配置、重启服务、扩缩容等）只能给出建议并说明风险等级，绝不擅自执行。\n")

	// 注入当前纳管主机清单：让 AI 知道有哪些主机、它们的 host_id / 主机名 / IP / 在线状态，
	// 从而能把用户口中的机器名或 IP 映射到工具所需的 host_id 参数。
	b.WriteString(h.buildHostContext())

	// Append active templates
	for _, t := range h.cachedTemplates {
		b.WriteString("\n【" + t.Category + "】" + t.Name + "：\n")
		b.WriteString(t.Content)
	}

	// Append active rules
	if len(h.cachedRules) > 0 {
		b.WriteString("\n\n【当前生效的规则】\n")
		for _, r := range h.cachedRules {
			fmt.Fprintf(&b, "- %s (优先级 %d)：%s\n", r.Name, r.Priority, r.Description)
		}
	}
	return b.String()
}

// buildHostContext 生成「当前纳管主机」清单文本，作为系统提示词的一部分注入。
// 让 AI 知道可查询哪些主机，并能将用户提到的主机名 / IP 映射到工具所需的 host_id。
func (h *HermesCore) buildHostContext() string {
	if h.s == nil || h.s.store == nil {
		return ""
	}
	hosts := h.s.store.ListHosts()
	if len(hosts) == 0 {
		return "\n\n【当前纳管主机】暂无已纳管主机。若用户询问主机相关问题，请如实说明当前没有可查询的主机。\n"
	}
	// 稳定排序：在线优先，其次按主机名，保证每次注入顺序一致
	sort.Slice(hosts, func(i, j int) bool {
		oi, oj := hermesHostOnline(hosts[i]), hermesHostOnline(hosts[j])
		if oi != oj {
			return oi
		}
		return hosts[i].Hostname < hosts[j].Hostname
	})
	now := time.Now().Unix()
	online := 0
	for _, hst := range hosts {
		if hermesHostOnline(hst) {
			online++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n\n【当前纳管主机】共 %d 台，在线 %d 台。调用工具（query_metrics / search_logs / list_alerts / run_diagnostic）时，请使用下表的 id 作为 host_id 参数；用户可能用主机名或 IP 指代主机，你需自行映射到对应 id：\n", len(hosts), online)
	const limit = 50
	for i, hst := range hosts {
		if i >= limit {
			fmt.Fprintf(&b, "…（其余 %d 台省略）\n", len(hosts)-limit)
			break
		}
		status := "离线"
		if hermesHostOnline(hst) {
			status = "在线"
		}
		fmt.Fprintf(&b, "- id=%s 主机名=%s IP=%s 状态=%s", hst.ID, hst.Hostname, hermesIPOr(hst.IP), status)
		if hst.OS != "" {
			fmt.Fprintf(&b, " 系统=%s", hst.OS)
		}
		if hst.Latest != nil {
			fmt.Fprintf(&b, " 当前CPU=%.0f%% 内存=%.0f%%", hst.Latest.CPUPercent, hst.Latest.MemPercent)
		}
		if hst.LastSeen > 0 {
			fmt.Fprintf(&b, " 最后上报=%ds前", now-hst.LastSeen)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// hermesHostOnline 判断主机是否在线（最近上报 ≤120s，与 forward.go 的离线判定一致）。
func hermesHostOnline(h *Host) bool {
	return h != nil && h.LastSeen > 0 && time.Now().Unix()-h.LastSeen <= 120
}

func hermesIPOr(ip string) string {
	if strings.TrimSpace(ip) == "" {
		return "未知"
	}
	return ip
}

// resolveSession 按 session_id 解析会话，作为多轮记忆的权威来源：
//   - sessionID>0：从 PostgreSQL 加载完整消息历史（刷新页面 / 切换会话后仍能延续）。
//   - sessionID==0：新建会话；若 PG 不可用或加载失败，用前端传入的 history 兜底，保证前后端状态一致。
func (h *HermesCore) resolveSession(sessionID int64, history []map[string]string) *HermesSession {
	sess := &HermesSession{ID: sessionID}
	if sessionID > 0 && h.s.pg != nil {
		if raw, err := h.s.pg.loadHermesSession(sessionID); err == nil && raw != nil {
			var msgs []map[string]string
			if json.Unmarshal(raw, &msgs) == nil {
				sess.Messages = msgs
				return sess
			}
		}
	}
	// 新会话，或 PG 不可用 / 未找到：用前端历史兜底
	if len(sess.Messages) == 0 && len(history) > 0 {
		sess.Messages = sanitizeHistory(history)
	}
	return sess
}

// sanitizeHistory 清洗前端传入的会话历史：仅保留合法的 user/assistant 非空消息，并限制最近 40 条。
func sanitizeHistory(history []map[string]string) []map[string]string {
	out := make([]map[string]string, 0, len(history))
	for _, m := range history {
		role, content := m["role"], m["content"]
		if (role == "user" || role == "assistant") && strings.TrimSpace(content) != "" {
			out = append(out, map[string]string{"role": role, "content": content})
		}
	}
	if len(out) > 40 {
		out = out[len(out)-40:]
	}
	return out
}