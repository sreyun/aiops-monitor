package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
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
	s       *Server
	tools   map[string]HermesTool
	mu      sync.Mutex
	session *HermesSession // current active session (simplified: single-session)
	// Cached config (hot-reloaded from PG)
	configMu    sync.RWMutex
	cachedRules     []hermesRule
	cachedTemplates []hermesTemplate
	lastLoad time.Time
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
		Description: "在目标主机上执行只读诊断命令（如 top、df、iostat、netstat 等）。返回命令输出。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "主机 ID"},
				"command": map[string]string{"type": "string", "description": "诊断命令，如 'top -bn1 | head -20'"},
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

func (h *HermesCore) execQueryMetrics(args map[string]any) (string, error) {
	hostID, _ := args["host_id"].(string)
	metric, _ := args["metric"].(string)
	if metric == "" {
		metric = "all"
	}
	hst := h.s.hostByID(hostID)
	if hst == nil || hst.Latest == nil {
		return fmt.Sprintf("主机 %s 不在线或暂无指标数据", hostID), nil
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
	since := time.Now().Unix() - int64(minutes*60)
	entries := h.s.logs.search(hostID, level, keyword, since, 10)
	if len(entries) == 0 {
		return fmt.Sprintf("主机 %s 最近 %d 分钟无匹配日志", hostID, minutes), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "主机 %s 最近 %d 分钟日志（%d 条）：\n", hostID, minutes, len(entries))
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
	var matched []string
	for _, a := range h.s.notifier.ActiveAlerts() {
		if a.Status != "" {
			continue
		}
		if hostID != "" && a.HostID != hostID {
			continue
		}
		matched = append(matched, fmt.Sprintf("  %s (%s, %.1f) — %s", a.Type, a.Level, a.Value, hostID))
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

func (h *HermesCore) execDiagnostic(args map[string]any) (string, error) {
	hostID, _ := args["host_id"].(string)
	command, _ := args["command"].(string)
	if command == "" {
		return "请指定诊断命令", nil
	}
	// Use playbook mechanism to execute read-only diagnostic commands
	host := h.s.hostByID(hostID)
	if host == nil {
		return fmt.Sprintf("主机 %s 不在线", hostID), nil
	}
	// Sanitize: only allow read-only commands
	cmdLower := strings.ToLower(strings.TrimSpace(command))
	allowed := false
	for _, prefix := range []string{"top", "df", "iostat", "netstat", "ss", "free", "ps", "uptime",
		"cat", "head", "tail", "grep", "find", "ls", "du", "lsof", "dmesg", "journalctl", "systemctl status",
		"docker ps", "docker logs", "kubectl get", "kubectl describe"} {
		if strings.HasPrefix(cmdLower, prefix) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Sprintf("诊断命令 '%s' 不在允许列表中（仅限只读诊断命令）", command), nil
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
	// Execute hermes_actions.py with the action name
	cmd := exec.Command("python3", "plugins/hermes_actions.py", actionName, hostID, argStr)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn("hermes python action failed", "action", actionName, "err", err, "output", string(output))
		return fmt.Sprintf("动作 '%s' 执行失败：%v\n输出：%s", actionName, err, string(output)), nil
	}
	return string(output), nil
}

// --- Core loop: Observe → Reason → Act ---

// Chat runs a Hermes conversation turn with Function Calling support.
// If stream is true, it writes SSE events to w; otherwise returns the reply.
func (h *HermesCore) Chat(session *HermesSession, userMsg string, stream bool, w http.ResponseWriter) (string, error) {
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
	fullReply, err := h.runLoop(cfg, msgs, stream, w)
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
// max 5 turns to prevent infinite loops.
func (h *HermesCore) runLoop(cfg AIConfig, msgs []map[string]string, stream bool, w http.ResponseWriter) (string, error) {
	for turn := 0; turn < 5; turn++ {
		// Call LLM with tools
		respText, toolCalls, err := h.callLLMWithTools(cfg, msgs, stream, w)
		if err != nil {
			return "", err
		}
		// If no tool calls, this is the final answer
		if len(toolCalls) == 0 {
			return respText, nil
		}
		// Execute tools and append results
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
		// Append tool results as a user message for the next LLM call
		msgs = append(msgs, map[string]string{
			"role": "user",
			"content": fmt.Sprintf("工具执行结果：\n%s\n请根据以上结果继续分析。", toolResults.String()),
		})
	}
	return "", fmt.Errorf("达到最大推理轮次，请简化问题重试")
}

// toolCall represents a parsed tool call from the LLM response.
type toolCall struct {
	Name string
	Args map[string]any
}

// callLLMWithTools calls the LLM and parses tool_calls from the response.
// For streaming, it accumulates the full response and sends deltas to w.
func (h *HermesCore) callLLMWithTools(cfg AIConfig, msgs []map[string]string, stream bool, w http.ResponseWriter) (string, []toolCall, error) {
	// Inject tool definitions into the system message
	msgsWithTools := h.injectTools(msgs)

	if stream && w != nil {
		// Stream mode: accumulate full text, then parse tool calls
		fullText, err := streamChat(w, cfg, msgsWithTools)
		if err != nil {
			return "", nil, err
		}
		tools := h.parseToolCalls(fullText)
		return fullText, tools, nil
	}
	// Non-stream mode
	reply, err := aiChat(cfg, msgsWithTools)
	if err != nil {
		return "", nil, err
	}
	tools := h.parseToolCalls(reply)
	return reply, tools, nil
}

// injectTools adds tool definitions to the last system message.
func (h *HermesCore) injectTools(msgs []map[string]string) []map[string]string {
	// Build tool definitions JSON
	var toolDefs []map[string]any
	for _, t := range h.tools {
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
		end := strings.Index(text[start:], "\n")
		if end < 0 {
			end = len(text) - start
		}
		return h.parseToolCallJSON(text[start : start+end])
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

// --- Hot-reload: rules + templates from PG ---

// reloadConfig refreshes cached rules and templates from PostgreSQL.
// Called before each conversation turn; no-op if PG is unavailable.
func (h *HermesCore) reloadConfig() {
	if h.s.pg == nil {
		return
	}
	// Throttle: reload at most every 5 seconds
	if time.Since(h.lastLoad) < 5*time.Second {
		return
	}
	h.configMu.Lock()
	defer h.configMu.Unlock()
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
func (h *HermesCore) buildSystemPrompt() string {
	h.reloadConfig()
	h.configMu.RLock()
	defer h.configMu.RUnlock()

	var b strings.Builder
	// Default system prompt
	b.WriteString("你是 Hermes，资深 SRE 自主运维 Agent。你具有以下能力：\n")
	b.WriteString("1. 观察：查询指标、搜索日志、列出告警\n")
	b.WriteString("2. 推理：分析故障根因、关联历史案例\n")
	b.WriteString("3. 行动：执行诊断命令、触发修复动作\n\n")
	b.WriteString("工作原则：\n")
	b.WriteString("- 先观察再推理，给出结论前至少调用一个工具验证\n")
	b.WriteString("- 用简洁中文回复，分点列出根因假设和处置建议\n")
	b.WriteString("- 需要执行操作时，说明风险等级并等待确认\n")

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

// ensureHermesSession gets or creates a Hermes session.
func (h *HermesCore) ensureHermesSession(incidentID int64) *HermesSession {
	if h.session != nil {
		return h.session
	}
	h.session = &HermesSession{IncidentID: incidentID}
	// Try to load from PG
	if h.s.pg != nil && h.session.ID > 0 {
		if raw, err := h.s.pg.loadHermesSession(h.session.ID); err == nil && raw != nil {
			var msgs []map[string]string
			if json.Unmarshal(raw, &msgs) == nil {
				h.session.Messages = msgs
			}
		}
	}
	return h.session
}