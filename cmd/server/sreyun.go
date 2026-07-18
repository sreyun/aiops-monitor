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

	"aiops-monitor/shared"
)

// ============================================================================
// Sreyun Agent — 自主运维 Agent 引擎
//
// 三层解耦架构：
//   Layer 1 (本文件): 引擎核心 — 观察→推理→行动循环 + Function Calling
//   Layer 2 (pgstore): 规则库 + 提示模板 — 热加载，无需重启
//   Layer 3 (plugins): Python 动作插件 — 动态加载，上传即生效
//
// 复用现有基础设施：aiChat/streamChat (LLM 调用)、pgStore (RAG/持久化)、
// playbookManager (远程执行)、logStore (日志检索)、notifier (告警查询)
// ============================================================================

// SreyunTool defines a callable function that the LLM can invoke.
type SreyunTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Execute     func(args map[string]any) (string, error)
}

// SreyunSession represents one conversation session.
type SreyunSession struct {
	ID         int64
	IncidentID int64
	Messages   []map[string]string
	CreatedAt  int64
	// 上下文压缩缓存：Summary 为早期对话的滚动摘要，SummarizedCount 为已被摘要覆盖的消息数，
	// 用于增量压缩（只摘「新变旧」的段落）。为内存缓存，重启后丢失会自动重建，不影响正确性。
	Summary         string
	SummarizedCount int
}

// SreyunCore is the autonomous agent engine.
type SreyunCore struct {
	s     *Server
	tools map[string]SreyunTool
	ctx   context.Context
	// Cached config (hot-reloaded from PG)
	configMu        sync.RWMutex
	cachedRules     []sreyunRule
	cachedTemplates []sreyunTemplate
	lastLoad        time.Time
	// P1-2: 缓存工具定义 JSON，工具注册后不再变化，避免每轮重建
	cachedToolPrompt string
	// P3-1: 缓存原生 Function Calling 工具定义数组
	cachedNativeToolDefs []map[string]any
}

// newSreyunCore creates and initializes the Sreyun engine.
func newSreyunCore(s *Server) *SreyunCore {
	h := &SreyunCore{s: s, tools: make(map[string]SreyunTool)}
	h.registerTools()
	return h
}

// registerTools registers all built-in tools the LLM can call.
func (h *SreyunCore) registerTools() {
	h.tools["query_metrics"] = SreyunTool{
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
	h.tools["search_logs"] = SreyunTool{
		Name:        "search_logs",
		Description: "搜索主机日志，支持按级别和关键词过滤",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "主机 ID"},
				"level":   map[string]string{"type": "string", "description": "日志级别：error/warn/info"},
				"keyword": map[string]string{"type": "string", "description": "搜索关键词"},
				"minutes": map[string]any{"type": "integer", "description": "最近 N 分钟"},
			},
			"required": []string{"host_id"},
		},
		Execute: h.execSearchLogs,
	}
	h.tools["list_alerts"] = SreyunTool{
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
	h.tools["search_similar_cases"] = SreyunTool{
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
	h.tools["run_diagnostic"] = SreyunTool{
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
	h.tools["run_python_action"] = SreyunTool{
		Name:        "run_python_action",
		Description: "执行 Python 插件中的自定义动作（如重启服务、清理缓存、扩缩容脚本等）",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action_name": map[string]string{"type": "string", "description": "动作名称，对应 plugins/ 下的 sreyun_actions.py 中定义的函数"},
				"host_id":     map[string]string{"type": "string", "description": "目标主机 ID"},
				"args":        map[string]string{"type": "string", "description": "传递给动作的参数（JSON 字符串）"},
			},
			"required": []string{"action_name"},
		},
		Execute: h.execPythonAction,
	}
	h.tools["list_datasources"] = SreyunTool{
		Name:        "list_datasources",
		Description: "列出已接入的外部数据源（Loki 日志 / Prometheus 指标）及其 id/名称/类型。查询前先用它确认有哪些数据源可用。",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Execute:     h.execListDataSources,
	}
	h.tools["query_datasource"] = SreyunTool{
		Name:        "query_datasource",
		Description: "直接查询外部数据源做分析排查：Prometheus 传 PromQL（如 up、node_load1、rate(http_requests_total[5m])）；Loki 传 LogQL（如 {job=\"nginx\"} |= \"error\"）。先用 list_datasources 拿到数据源 id 或名称。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"datasource": map[string]string{"type": "string", "description": "数据源 id 或名称"},
				"query":      map[string]string{"type": "string", "description": "PromQL（Prometheus）或 LogQL（Loki）查询语句"},
				"limit":      map[string]any{"type": "integer", "description": "Loki 返回日志行数上限（默认 100）"},
				"since_min":  map[string]any{"type": "integer", "description": "Loki 查询最近 N 分钟（默认 60）"},
			},
			"required": []string{"datasource", "query"},
		},
		Execute: h.execQueryDataSource,
	}
	// --- New tools for enhanced AI capabilities ---
	h.tools["list_recent_changes"] = SreyunTool{
		Name:        "list_recent_changes",
		Description: "查询主机最近 N 小时的指标变化趋势（CPU/内存/磁盘增长率），帮助判断是否恶化中。返回结构化 JSON 包含趋势方向（上升/下降/稳定）和变化幅度。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "主机 ID"},
				"hours":   map[string]any{"type": "integer", "description": "查询最近 N 小时（默认 6）"},
			},
			"required": []string{"host_id"},
		},
		Execute: h.execListRecentChanges,
	}
	h.tools["check_host_health"] = SreyunTool{
		Name:        "check_host_health",
		Description: "综合评估主机健康状态：聚合当前指标、活跃告警、日志异常，返回 healthy/degraded/critical 分级结论和详细分析。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "主机 ID"},
			},
			"required": []string{"host_id"},
		},
		Execute: h.execCheckHostHealth,
	}

	// ---- 硬件 / 流量：此前 AI 完全看不到这两块数据 ----

	h.tools["query_hardware"] = SreyunTool{
		Name: "query_hardware",
		Description: "查询主机的服务器硬件状态（Redfish/BMC 或 OceanStor 存储）：整机健康、厂商型号序列号、" +
			"CPU/内存/硬盘/RAID卡/GPU/电源/风扇/温度/磁盘框的逐部件明细，并列出当前所有异常部件。" +
			"排查硬件故障、确认配置、做资产核对时用这个。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "主机 ID"},
				"section": map[string]string{"type": "string",
					"description": "要看的部分：summary(默认,概览+异常部件)/disk/memory/cpu/gpu/raid/psu/fan/temp/enclosure/firmware/all"},
			},
			"required": []string{"host_id"},
		},
		Execute: h.execQueryHardware,
	}

	h.tools["query_hardware_events"] = SreyunTool{
		Name: "query_hardware_events",
		Description: "查询 BMC 自身的硬件事件日志（Dell iDRAC 的 SEL / 华为 iBMC 事件 / OceanStor 当前告警）。" +
			"**这是唯一能定位到「是哪个部件、什么时候出的问题」的数据源** —— 整机健康只说好坏，不说是谁。" +
			"硬件报错但不知道换哪个件时，必须查这个。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "主机 ID"},
				"limit":   map[string]string{"type": "integer", "description": "返回条数，默认 20"},
			},
			"required": []string{"host_id"},
		},
		Execute: h.execQueryHardwareEvents,
	}

	h.tools["query_hardware_history"] = SreyunTool{
		Name: "query_hardware_history",
		Description: "查询硬件指标的历史趋势（温度/风扇转速/功耗/健康分），数据来自时序库，可长期回溯。" +
			"判断「是不是一直这样」「什么时候开始变热的」「功耗有没有异常爬升」时用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "主机 ID"},
				"metric":  map[string]string{"type": "string", "description": "temperature / fan_rpm / power / health_score"},
				"range":   map[string]string{"type": "string", "description": "时间范围，如 1h/6h/24h/7d/30d，默认 24h"},
			},
			"required": []string{"host_id", "metric"},
		},
		Execute: h.execQueryHardwareHistory,
	}

	h.tools["query_hardware_changes"] = SreyunTool{
		Name: "query_hardware_changes",
		Description: "查询硬件资产变更历史：哪块盘/哪条内存/哪个电源什么时候被换过、加过、拔掉过，以及固件版本变更。" +
			"排查「故障是不是换件之后开始的」「这台机器动过什么」时用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "主机 ID"},
				"limit":   map[string]string{"type": "integer", "description": "返回条数，默认 30"},
			},
			"required": []string{"host_id"},
		},
		Execute: h.execQueryHardwareChanges,
	}

	h.tools["query_netflow"] = SreyunTool{
		Name: "query_netflow",
		Description: "查询主机的网络流量 Top-N 排行（按对端 IP / 端口 / 协议聚合），数据来自永久保留的 Flow 明细。" +
			"排查带宽被谁占了、有无异常外联、某个时间段在跟谁通信时用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id":   map[string]string{"type": "string", "description": "主机 ID"},
				"dimension": map[string]string{"type": "string", "description": "聚合维度：dst_ip(默认)/src_ip/dst_port/src_port/protocol"},
				"range":     map[string]string{"type": "string", "description": "时间范围，如 1h/24h/7d，默认 1h"},
				"top":       map[string]string{"type": "integer", "description": "返回前 N 名，默认 10"},
			},
			"required": []string{"host_id"},
		},
		Execute: h.execQueryNetFlow,
	}

	h.tools["query_hyperv"] = SreyunTool{
		Name: "query_hyperv",
		Description: "查询某台物理宿主机上的 Hyper-V 虚拟机清单与状态：每台 VM 的运行/关机/暂停、CPU/内存占用、" +
			"IP 地址、运行时长、复制健康，异常 VM 摆在最前；并能识别 VM 是否对应到一台已纳管主机。" +
			"排查「宿主机上哪台虚拟机挂了/在抢资源」「某业务 VM 跑在哪台物理机上」时用这个。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "物理宿主机 ID"},
				"vm_name": map[string]string{"type": "string", "description": "可选：只看指定名称的虚拟机"},
			},
			"required": []string{"host_id"},
		},
		Execute: h.execQueryHyperV,
	}
}

// resolveDataSource matches a configured data source by id, then by name (case-insensitive).
func (h *SreyunCore) resolveDataSource(ref string) (DataSource, bool) {
	ref = strings.TrimSpace(ref)
	if ds, ok := h.s.cfg.GetDataSource(ref); ok {
		return ds, true
	}
	low := strings.ToLower(ref)
	for _, d := range h.s.cfg.ListDataSources() {
		if strings.ToLower(d.Name) == low {
			return d, true
		}
	}
	return DataSource{}, false
}

func (h *SreyunCore) execListDataSources(args map[string]any) (string, error) {
	list := h.s.cfg.ListDataSources()
	if len(list) == 0 {
		return "（未接入任何数据源，请先在「数据源」页添加 Loki / Prometheus）", nil
	}
	var sb strings.Builder
	for _, d := range list {
		status := "启用"
		if !d.Enabled {
			status = "停用"
		}
		fmt.Fprintf(&sb, "- id=%s 名称=%s 类型=%s 状态=%s\n", d.ID, d.Name, d.Type, status)
	}
	return sb.String(), nil
}

func (h *SreyunCore) execQueryDataSource(args map[string]any) (string, error) {
	ref, _ := args["datasource"].(string)
	query, _ := args["query"].(string)
	if strings.TrimSpace(ref) == "" || strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("datasource 和 query 必填")
	}
	ds, ok := h.resolveDataSource(ref)
	if !ok {
		return "", fmt.Errorf("未找到数据源 %q，请先用 list_datasources 确认可用数据源", ref)
	}
	if !ds.Enabled {
		return "", fmt.Errorf("数据源 %s 已停用", ds.Name)
	}
	limit, sinceMin := 0, 0
	if v, ok := args["limit"].(float64); ok {
		limit = int(v)
	}
	if v, ok := args["since_min"].(float64); ok {
		sinceMin = int(v)
	}
	return queryDataSource(ds, query, limit, sinceMin)
}

// --- Tool implementations ---

// resolveHostRef 按 host_id 精确匹配主机；失败则回退到主机名 / IP（忽略大小写，再退化到
// 主机名包含匹配），让 AI 即便把主机名或 IP 当作 host_id 传入也能命中正确主机。
func (h *SreyunCore) resolveHostRef(ref string) *Host {
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

func (h *SreyunCore) execQueryMetrics(args map[string]any) (string, error) {
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

func (h *SreyunCore) execSearchLogs(args map[string]any) (string, error) {
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

func (h *SreyunCore) execListAlerts(args map[string]any) (string, error) {
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

func (h *SreyunCore) execSearchCases(args map[string]any) (string, error) {
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
	// 同时检索「诊断案例库」与「通用 AI 记忆库」（对话 / 文件 / URL / 历史全都沉淀在此），
	// 让每次交互积累的知识都能被后续对话复用——真正的自我进化。
	cases, _ := h.s.pg.searchSimilarCases(emb, 3)
	mems, _ := h.s.pg.searchMemory(emb, 3)
	if len(cases) == 0 && len(mems) == 0 {
		return "未找到相似历史案例或记忆", nil
	}
	var b strings.Builder
	if len(cases) > 0 {
		b.WriteString("相似历史诊断案例：\n")
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
	}
	if len(mems) > 0 {
		b.WriteString("相关历史记忆（对话 / 文件 / URL）：\n")
		for i, m := range mems {
			sim := int((1.0 - m.Distance) * 100)
			if sim < 0 {
				sim = 0
			}
			fmt.Fprintf(&b, "  %d. [%s] 相似度 %d%%: %s\n", i+1, m.Kind, sim, trimLine(m.Content, 250))
		}
	}
	return b.String(), nil
}

// diagCommandAllowed 校验诊断命令是否为「只读命令 + 只读管道过滤」。
// 命令在被控端经 shell 执行，故：
//  1. 拒绝可用于注入 / 破坏 / 外联 / 重定向 / 子shell 的元字符（; & $ < > ` \ 换行 () {}）；
//  2. 允许用管道 | 串联，但逐段校验每段命令首词必须精确命中白名单（非松散前缀，dfoo 不算 df）。
//
// 返回 (是否放行, 拒绝原因)。
func diagCommandAllowed(command string) (bool, string) {
	cmdTrim := strings.TrimSpace(command)
	if cmdTrim == "" {
		return false, "请指定诊断命令"
	}
	if strings.ContainsAny(cmdTrim, ";&$<>\n\r\\(){}" + "`") {
		return false, "诊断命令含被禁止的字符（; & $ < > ` 等），仅允许只读命令与管道过滤"
	}
	allow := []string{
		"top", "df", "iostat", "vmstat", "mpstat", "sar", "pidstat", "netstat", "ss", "free",
		"ps", "uptime", "cat", "head", "tail", "grep", "egrep", "ls", "du", "lsof", "dmesg",
		"journalctl", "systemctl status", "docker ps", "docker logs", "docker stats",
		"kubectl get", "kubectl describe", "wc", "sort", "uniq", "cut", "tr", "nl", "tac",
		"column", "date", "hostname", "uname", "who", "w",
	}
	// P2-6: 敏感路径黑名单，防止通过 cat/grep 等读取敏感文件
	deniedPaths := []string{
		"/etc/shadow", "/etc/gshadow", "/etc/master.passwd",
		".ssh/", ".gnupg/", ".aws/", ".kube/config",
		"/etc/sudoers", "/root/.bash_history",
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
		// P2-6: 检查管道每段是否访问敏感路径
		segLower := strings.ToLower(seg)
		for _, dp := range deniedPaths {
			if strings.Contains(segLower, dp) {
				return false, fmt.Sprintf("诊断命令包含敏感路径 %q，已拦截", dp)
			}
		}
	}
	return true, ""
}

func (h *SreyunCore) execDiagnostic(args map[string]any) (string, error) {
	hostID, _ := args["host_id"].(string)
	command, _ := args["command"].(string)
	if command == "" {
		return "请指定诊断命令", nil
	}
	// 门控：AI 终端只读巡检为独立高风险授权，需用户在 AI 设置中显式开启（并已校验终端密码）后才可执行主机命令。
	if !h.s.cfg.AIConfig().SreyunTerminalEnabled {
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
		Name: "sreyun-diagnostic",
		Steps: []PlaybookStep{{
			Name:       "diagnostic-cmd",
			Command:    command,
			Target:     "host:" + hostID,
			TimeoutSec: 15,
		}},
	}
	done := make(chan bool, 1)
	execID := h.s.triggerPlaybookOnHost(pb, host, "sreyun", func(ok bool) {
		done <- ok
	})
	// h.ctx 是共享可变字段，仅 Chat() 内赋值；非会话路径调用时可能为 nil 或已取消，
	// 这里取本地副本并兜底 Background()，避免对 nil ctx 调 Done() 引发 panic（与 execPythonAction 一致）。
	dctx := h.ctx
	if dctx == nil {
		dctx = context.Background()
	}
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
	case <-dctx.Done():
		return "诊断命令已被客户端取消", nil
	}
}

func (h *SreyunCore) execPythonAction(args map[string]any) (string, error) {
	actionName, _ := args["action_name"].(string)
	hostID, _ := args["host_id"].(string)
	argStr, _ := args["args"].(string)
	if actionName == "" {
		return "请指定动作名称", nil
	}
	// run_python_action 属于「写操作」（重启服务 / 清理缓存 / 扩缩容等）。仅在显式开启
	// SreyunAutoApprove（低风险自动执行）时才真正执行，否则挂起并返回需人工确认，
	// 避免 AI 未经批准擅自变更主机（此前该配置从未在执行路径生效，属安全缺口）。
	if !h.s.cfg.AIConfig().SreyunAutoApprove {
		return fmt.Sprintf("动作 %q 属于高风险写操作，需人工确认。当前未开启「自动执行」(hermes_auto_approve)，已阻止自动执行；请操作员手动处置，或在 AI 设置中开启后重试。", actionName), nil
	}
	// 加 30s 超时，避免插件脚本卡死导致请求 goroutine 永久阻塞
	parentCtx := h.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "plugins/sreyun_actions.py", actionName, hostID, argStr)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("动作 %q 执行超时（30s）", actionName), nil
	}
	if err != nil {
		slog.Warn("sreyun python action failed", "action", actionName, "err", err, "output", string(output))
		return fmt.Sprintf("动作 %q 执行失败：%v\n输出：%s", actionName, err, string(output)), nil
	}
	return string(output), nil
}

// trendArrow returns a direction indicator for a numeric change.
func trendArrow(delta float64) string {
	if delta > 2 {
		return "↑"
	}
	if delta < -2 {
		return "↓"
	}
	return "→"
}

// execListRecentChanges queries historical samples and computes per-metric trends.
func (h *SreyunCore) execListRecentChanges(args map[string]any) (string, error) {
	hostID, _ := args["host_id"].(string)
	hours := 6
	if v, ok := args["hours"].(float64); ok && v > 0 {
		hours = int(v)
	}
	if hostID == "" {
		return "请指定 host_id", nil
	}
	name := hostID
	if hst := h.resolveHostRef(hostID); hst != nil {
		hostID, name = hst.ID, hst.Hostname
	}
	now := time.Now().Unix()
	from := now - int64(hours)*3600
	var samples []shared.Sample
	if h.s.vm != nil && h.s.vm.enabled() {
		samples, _ = h.s.vm.queryHistory(hostID, from, now)
	}
	if len(samples) == 0 {
		samples, _ = h.s.store.GetHistory(hostID, from, now)
	}
	if len(samples) < 2 {
		return fmt.Sprintf("主机 %s 最近 %d 小时历史数据不足（仅 %d 个样本），无法计算趋势", name, hours, len(samples)), nil
	}
	// Split into 3 windows: early / mid / late for trend direction detection.
	n := len(samples)
	third := n / 3
	if third < 1 {
		third = 1
	}
	early := samples[:third]
	late := samples[n-third:]
	avg := func(ss []shared.Sample, field func(shared.Sample) float64) float64 {
		if len(ss) == 0 {
			return 0
		}
		var sum float64
		for _, s := range ss {
			sum += field(s)
		}
		return sum / float64(len(ss))
	}
	cpuEarly := avg(early, func(s shared.Sample) float64 { return s.CPUPercent })
	cpuLate := avg(late, func(s shared.Sample) float64 { return s.CPUPercent })
	memEarly := avg(early, func(s shared.Sample) float64 { return s.MemPercent })
	memLate := avg(late, func(s shared.Sample) float64 { return s.MemPercent })
	diskEarly := avg(early, func(s shared.Sample) float64 { return s.DiskPercent })
	diskLate := avg(late, func(s shared.Sample) float64 { return s.DiskPercent })
	loadEarly := avg(early, func(s shared.Sample) float64 { return s.Load1 })
	loadLate := avg(late, func(s shared.Sample) float64 { return s.Load1 })

	type metricTrend struct {
		Name  string  `json:"metric"`
		Early float64 `json:"early_avg"`
		Late  float64 `json:"late_avg"`
		Delta float64 `json:"delta"`
		Arrow string  `json:"trend"`
	}
	trends := []metricTrend{
		{"CPU使用率(%)", cpuEarly, cpuLate, cpuLate - cpuEarly, trendArrow(cpuLate - cpuEarly)},
		{"内存使用率(%)", memEarly, memLate, memLate - memEarly, trendArrow(memLate - memEarly)},
		{"磁盘使用率(%)", diskEarly, diskLate, diskLate - diskEarly, trendArrow(diskLate - diskEarly)},
		{"负载(Load1)", loadEarly, loadLate, loadLate - loadEarly, trendArrow((loadLate - loadEarly) * 10)},
	}
	out, _ := json.Marshal(map[string]any{
		"host":    name,
		"hours":   hours,
		"samples": n,
		"trends":  trends,
	})
	return string(out), nil
}

// execCheckHostHealth performs a comprehensive health assessment of a host.
func (h *SreyunCore) execCheckHostHealth(args map[string]any) (string, error) {
	hostID, _ := args["host_id"].(string)
	if hostID == "" {
		return "请指定 host_id", nil
	}
	name := hostID
	if hst := h.resolveHostRef(hostID); hst != nil {
		hostID, name = hst.ID, hst.Hostname
	}
	host := h.s.hostByID(hostID)
	if host == nil {
		return fmt.Sprintf("未找到主机 %q", hostID), nil
	}
	// Determine online status.
	now := time.Now().Unix()
	offlineSec := int64(h.s.cfg.Thresholds().OfflineAfter.Seconds())
	online := now-host.LastSeen <= offlineSec

	type healthResult struct {
		Host      string   `json:"host"`
		Status    string   `json:"status"` // healthy | degraded | critical
		Online    bool     `json:"online"`
		Issues    []string `json:"issues,omitempty"`
		Metrics   string   `json:"metrics_summary"`
		Alerts    int      `json:"active_alerts"`
		ErrorLogs int      `json:"recent_errors"`
	}
	result := healthResult{Host: name, Online: online}

	if !online {
		result.Status = "critical"
		result.Issues = append(result.Issues, "主机离线")
		out, _ := json.Marshal(result)
		return string(out), nil
	}
	if host.Latest == nil {
		result.Status = "degraded"
		result.Issues = append(result.Issues, "无指标数据")
		out, _ := json.Marshal(result)
		return string(out), nil
	}
	m := host.Latest
	result.Metrics = fmt.Sprintf("CPU %.1f%% | 内存 %.1f%% | 磁盘 %.1f%% | Load %.2f/%.2f | 进程 %d",
		m.CPUPercent, m.MemPercent, m.DiskPercent, m.Load1, m.Load5, m.ProcCount)

	// Check thresholds.
	th := h.s.cfg.Thresholds()
	var score int
	if m.CPUPercent >= th.CPUCrit {
		result.Issues = append(result.Issues, fmt.Sprintf("CPU 使用率严重过高 %.1f%% (阈值 %.0f%%)", m.CPUPercent, th.CPUCrit))
		score += 3
	} else if m.CPUPercent >= th.CPUWarn {
		result.Issues = append(result.Issues, fmt.Sprintf("CPU 使用率偏高 %.1f%% (阈值 %.0f%%)", m.CPUPercent, th.CPUWarn))
		score++
	}
	if m.MemPercent >= th.MemCrit {
		result.Issues = append(result.Issues, fmt.Sprintf("内存使用率严重过高 %.1f%% (阈值 %.0f%%)", m.MemPercent, th.MemCrit))
		score += 3
	} else if m.MemPercent >= th.MemWarn {
		result.Issues = append(result.Issues, fmt.Sprintf("内存使用率偏高 %.1f%% (阈值 %.0f%%)", m.MemPercent, th.MemWarn))
		score++
	}
	if m.DiskPercent >= th.DiskCrit {
		result.Issues = append(result.Issues, fmt.Sprintf("磁盘使用率严重过高 %.1f%% (阈值 %.0f%%)", m.DiskPercent, th.DiskCrit))
		score += 3
	} else if m.DiskPercent >= th.DiskWarn {
		result.Issues = append(result.Issues, fmt.Sprintf("磁盘使用率偏高 %.1f%% (阈值 %.0f%%)", m.DiskPercent, th.DiskWarn))
		score++
	}
	if m.Load1 > float64(m.CPUCores)*2 {
		result.Issues = append(result.Issues, fmt.Sprintf("负载过高 Load1=%.2f (%d核)", m.Load1, m.CPUCores))
		score += 2
	}

	// Count active alerts for this host.
	if h.s.notifier != nil {
		for _, a := range h.s.notifier.ActiveAlerts() {
			if a.HostID == hostID && a.Status == "" {
				result.Alerts++
			}
		}
	}
	if result.Alerts > 0 {
		score += result.Alerts
	}

	// Check recent error logs.
	if h.s.logs != nil {
		errs := h.s.logs.recentErrors(now-1800, 50)
		for _, e := range errs {
			if e.HostID == hostID {
				result.ErrorLogs++
			}
		}
	}
	if result.ErrorLogs > 5 {
		score++
	}

	// Determine status based on cumulative score.
	switch {
	case score >= 5:
		result.Status = "critical"
	case score >= 2:
		result.Status = "degraded"
	default:
		result.Status = "healthy"
		if len(result.Issues) == 0 {
			result.Issues = append(result.Issues, "各项指标正常")
		}
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

// --- Core loop: Observe → Reason → Act ---

// Chat runs a Sreyun conversation turn with Function Calling support.
// If stream is true, it writes SSE events to w; otherwise returns the reply.
func (h *SreyunCore) Chat(ctx context.Context, session *SreyunSession, userMsg string, images []chatImage, stream bool, w http.ResponseWriter) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	h.ctx = ctx
	cfg := h.s.cfg.AIConfig()
	if !cfg.Enabled || cfg.Endpoint == "" || cfg.Model == "" {
		return "", fmt.Errorf("AI 未配置或未启用")
	}

	// Build system prompt from cached templates + rules
	sys := h.buildSystemPrompt()

	// RAG: 检索历史记忆注入 system prompt，让 Agent 能跨会话复用已有知识
	// Token 预算管理：动态裁剪 RAG 记忆，确保 system prompt 不超过 8000 token
	ragText := h.s.retrieveMemoryForPrompt("chat", userMsg, 8)
	sysBudget := 8000 - estimateTokens(sys)
	if sysBudget < 500 {
		sysBudget = 500 // 最低保留 500 token 给 RAG
	}
	ragTokens := estimateTokens(ragText)
	if ragTokens > sysBudget {
		// 截断 RAG 文本以符合预算
		ragRunes := []rune(ragText)
		for len(ragRunes) > 100 && estimateTokens(string(ragRunes)) > sysBudget {
			ragRunes = ragRunes[:len(ragRunes)*sysBudget/ragTokens]
		}
		ragText = string(ragRunes) + "\n…(RAG 记忆已截断以符合 token 预算)"
	}
	sys += ragText
	sys += h.s.retrieveSkillsForPrompt(userMsg, 4) // 注入已提炼的可复用技能(SOP)，让 Agent 直接套用被验证的做法

	// Build messages
	msgs := []map[string]string{{"role": "system", "content": sys}}
	// 上下文压缩：历史超预算时，把较旧轮次用 AI 摘成要点、保留最近若干轮原文——替代此前的朴素
	// 字符串截断，避免丢失早期关键上下文。摘要增量缓存在 session 上，只对「新变旧」的段落重算。
	compressed, newSummary, newCount := compressHistory(cfg, session.Messages, 12, session.Summary, session.SummarizedCount)
	session.Summary, session.SummarizedCount = newSummary, newCount
	msgs = append(msgs, compressed...)
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
		newID, err := h.s.pg.saveSreyunSession(session.ID, raw, session.IncidentID)
		if err == nil && newID > 0 {
			session.ID = newID
		}
	}
	// 向量化本轮交互 → 永久入库沉淀为 RAG 记忆
	if strings.TrimSpace(fullReply) != "" {
		go h.s.rememberAI("chat", fmt.Sprintf("sreyun:%d", session.ID),
			"【用户】\n"+userMsg+"\n\n【AI】\n"+fullReply)
	}
	return fullReply, nil
}

// runLoop implements the core observe→reason→act loop with Function Calling.
// 每轮都以【非流式】方式调用 LLM，以便可靠解析 tool_calls：流式模式难以在 token 中途
// 判定是否为工具调用，且 streamChat 每次调用都会发送 [DONE]，会使前端在多轮工具调用中途
// 提前结束、看不到最终结论。面向用户只推送「思考文字 + 工具执行状态 + 最终结论」，
// 工具调用的原始 JSON 绝不下发到前端。max 5 turns to prevent infinite loops.
func (h *SreyunCore) runLoop(ctx context.Context, cfg AIConfig, msgs []map[string]string, images []chatImage, stream bool, w http.ResponseWriter) (string, error) {
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
	// sendReasoning 下发推理模型思维链增量（独立 {"reasoning":...} 帧）。前端收进「思考过程」
	// 折叠区，与最终答案分离——既消除首字前的长静默，又不污染正文。
	sendReasoning := func(text string) {
		if !stream || w == nil || text == "" {
			return
		}
		fmt.Fprintf(w, "data: {\"reasoning\":%s}\n\n", jsonString(text))
		if flusher != nil {
			flusher.Flush()
		}
	}
	// sendTool 以独立 SSE 帧下发工具执行状态（state: run/ok/err），前端渲染为可实时更新的
	// 「工具调用」状态 chip；刻意与 delta 正文分离，既让用户看到实时进度，又不污染最终回答。
	// P3-3: 增加 info 字段，携带工具参数（run 时）或结果摘要（ok 时），供前端展示推理链路。
	sendTool := func(name, state string, info map[string]string) {
		if !stream || w == nil {
			return
		}
		extra := ""
		for k, v := range info {
			extra += fmt.Sprintf(",%s:%s", jsonString(k), jsonString(v))
		}
		fmt.Fprintf(w, "data: {\"tool\":{\"name\":%s,\"state\":%s%s}}\n\n", jsonString(name), jsonString(state), extra)
		if flusher != nil {
			flusher.Flush()
		}
	}

	for turn := 0; turn < 5; turn++ {
		if err := ctx.Err(); err != nil { // 客户端已断开：停止后续 LLM 调用与工具执行，避免用户离开后仍在主机上跑命令
			return "", err
		}

		// P3-1: 检测 Provider 类型，决定使用原生 Function Calling 还是文本注入
		_, prov := normalizeEndpoint(cfg.Endpoint)
		var callMsgs []map[string]string
		var nativeTools []map[string]any
		if prov != aiProvAnthropic && len(h.tools) > 0 {
			// OpenAI 兼容 Provider：使用原生 Function Calling（更可靠）
			callMsgs = msgs
			// 确保 nativeToolDefs 已缓存
			if h.cachedNativeToolDefs == nil {
				h.injectTools(msgs) // 副作用：缓存 nativeToolDefs
			}
			nativeTools = h.cachedNativeToolDefs
		} else {
			// Anthropic 或无工具：使用文本注入（兼容回退）
			callMsgs = h.injectTools(msgs)
		}

		// 真流式：仅 OpenAI 兼容 + 已开启 stream 时启用——content 逐字回调下发（实现主会话
		// 逐字输出），tool_calls 结构化累积；原生 FC 下二者分离，不会把工具 JSON 泄漏给用户。
		// Anthropic / 文本注入 / 非流式请求仍走可靠的非流式 aiChatV。
		var reply string
		var nativeCalls []nativeToolCall
		var err error
		streamedContent := false
		if stream && w != nil && prov != aiProvAnthropic && len(h.tools) > 0 {
			reply, nativeCalls, err = aiChatVStream(ctx, cfg, callMsgs, images, nativeTools,
				func(delta string) {
					streamedContent = true
					sendDelta(delta)
				},
				sendReasoning, // 思维链增量 → 独立通道
			)
		} else {
			reply, nativeCalls, err = aiChatV(ctx, cfg, callMsgs, images, nativeTools) // 带 ctx（可中止）+ 图片（多模态）
		}
		if err != nil {
			if stream && w != nil {
				fmt.Fprintf(w, "data: {\"error\":%s}\n\n", jsonString(err.Error()))
				if flusher != nil {
					flusher.Flush()
				}
			}
			return "", err
		}

		// P3-1: 优先使用原生 tool_calls，无则回退到文本解析
		var toolCalls []toolCall
		if len(nativeCalls) > 0 {
			for _, nc := range nativeCalls {
				toolCalls = append(toolCalls, toolCall{Name: nc.Name, Args: nc.Args})
			}
		} else {
			toolCalls = h.parseToolCalls(reply)
		}
		if len(toolCalls) == 0 {
			// 无工具调用 = 最终结论。剥掉可能残留的 JSON/代码块后下发。
			final := stripToolCallJSON(reply)
			if final == "" {
				final = strings.TrimSpace(reply)
			}
			// 流式模式下 content 已被逐字送达，末尾不再整段重发（否则前端会看到重复）。
			if !streamedContent {
				sendDelta(final)
			}
			return final, nil
		}

		// P3-3: 将模型的「思考文字」作为思维链下发。
		// 流式模式下推理旁白已随 content 逐字送达，此处跳过 blockquote 以免重复。
		if think := stripToolCallJSON(reply); think != "" && !streamedContent {
			sendDelta("\n> \U0001f9e0 " + strings.ReplaceAll(think, "\n", "\n> ") + "\n\n")
		}
		// 逐个工具「执行中 → 完成/失败」以独立 tool 帧实时下发
		var toolResults strings.Builder
		for _, tc := range toolCalls {
			slog.Info("sreyun tool call", "tool", tc.Name, "args", fmt.Sprintf("%v", tc.Args))
			tool, ok := h.tools[tc.Name]
			if !ok {
				sendTool(tc.Name, "err", nil)
				toolResults.WriteString(fmt.Sprintf("[工具 %s 不存在]\n", tc.Name))
				continue
			}
			argsInfo := map[string]string{}
			if hostID, _ := tc.Args["host_id"].(string); hostID != "" {
				argsInfo["target"] = hostID
			}
			if cmd, _ := tc.Args["command"].(string); cmd != "" {
				argsInfo["detail"] = cmd
			} else if q, _ := tc.Args["query"].(string); q != "" {
				argsInfo["detail"] = q
			} else if m, _ := tc.Args["metric"].(string); m != "" {
				argsInfo["detail"] = m
			}
			sendTool(tc.Name, "run", argsInfo)
			result, err := tool.Execute(tc.Args)
			if err != nil {
				sendTool(tc.Name, "err", nil)
				toolResults.WriteString(fmt.Sprintf("[工具 %s 执行失败：%v]\n", tc.Name, err))
			} else {
				summary := strings.ReplaceAll(strings.TrimSpace(result), "\n", " ")
				if len([]rune(summary)) > 120 {
					summary = string([]rune(summary)[:120]) + "…"
				}
				sendTool(tc.Name, "ok", map[string]string{"summary": summary})
				truncResult := result
				if len([]rune(truncResult)) > 4000 {
					truncResult = string([]rune(truncResult)[:4000]) + "\n…(工具结果已截断)"
				}
				toolResults.WriteString(fmt.Sprintf("工具 %s 结果：\n%s\n", tc.Name, truncResult))
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
	final, _, err := aiChatV(ctx, cfg, msgs, images, nil) // 不注入工具定义，强制收敛为自然语言结论
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
// P1-2: 工具定义 JSON 缓存于 h.cachedToolPrompt，仅首次调用时构建。
// P3-1: 同时缓存原生 Function Calling 格式的工具定义。
func (h *SreyunCore) injectTools(msgs []map[string]string) []map[string]string {
	if h.cachedToolPrompt == "" {
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
		h.cachedToolPrompt = "\n\n你可以使用以下工具来获取信息或执行操作。当需要调用工具时，请用以下 JSON 格式回复：\n```json\n{\"tool_calls\":[{\"name\":\"工具名\",\"args\":{参数}}]}\n```\n\n可用工具定义：\n" + string(toolsJSON)
		// P3-1: 缓存原生 Function Calling 工具定义（复用同一份排序后的 defs）
		h.cachedNativeToolDefs = toolDefs
	}

	// Find the system message and append tool definitions
	result := make([]map[string]string, len(msgs))
	copy(result, msgs)
	for i, m := range result {
		if m["role"] == "system" {
			result[i] = map[string]string{
				"role":    "system",
				"content": m["content"] + h.cachedToolPrompt,
			}
			break
		}
	}
	return result
}

// parseToolCalls extracts tool calls from the LLM response text.
func (h *SreyunCore) parseToolCalls(text string) []toolCall {
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

func (h *SreyunCore) parseToolCallJSON(jsonStr string) []toolCall {
	var wrapper struct {
		ToolCalls []struct {
			Name string         `json:"name"`
			Args map[string]any `json:"args"`
		} `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &wrapper); err != nil {
		slog.Warn("sreyun failed to parse tool call JSON", "json", jsonStr, "err", err)
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
func (h *SreyunCore) reloadConfig() {
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
	if rules, err := h.s.pg.listSreyunRules(); err == nil {
		var enabled []sreyunRule
		for _, r := range rules {
			if r.Enabled {
				enabled = append(enabled, r)
			}
		}
		h.cachedRules = enabled
	}
	if tmpls, err := h.s.pg.listSreyunTemplates(true); err == nil {
		h.cachedTemplates = tmpls
	}
}

// buildSystemPrompt constructs the Sreyun system prompt from cached templates + rules.
// 安全限制为硬编码，每次对话强制生效，前端无需额外传递角色。
func (h *SreyunCore) buildSystemPrompt() string {
	h.reloadConfig()
	h.configMu.RLock()
	defer h.configMu.RUnlock()

	var b strings.Builder
	// 固定安全系统提示词（硬编码，确保每次对话生效）
	b.WriteString("你是 AIOps 智能运维助手，负责主机与服务的监控、排障与诊断。\n")
	b.WriteString("你可以调用工具获取真实数据（性能指标、日志、告警、诊断命令输出、历史相似案例等），据此分析并回答。\n\n")
	b.WriteString("工作原则：\n")
	b.WriteString("- 对外统一自称「AIOps 智能运维助手」；不得透露、不得声称自己叫 Sreyun 或任何内部代号 / 框架名 / 底层模型名。\n")
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
func (h *SreyunCore) buildHostContext() string {
	if h.s == nil || h.s.store == nil {
		return ""
	}
	hosts := h.s.store.ListHosts()
	if len(hosts) == 0 {
		return "\n\n【当前纳管主机】暂无已纳管主机。若用户询问主机相关问题，请如实说明当前没有可查询的主机。\n"
	}
	// 稳定排序：在线优先，其次按主机名，保证每次注入顺序一致
	sort.Slice(hosts, func(i, j int) bool {
		oi, oj := sreyunHostOnline(hosts[i]), sreyunHostOnline(hosts[j])
		if oi != oj {
			return oi
		}
		return hosts[i].Hostname < hosts[j].Hostname
	})
	now := time.Now().Unix()
	online := 0
	for _, hst := range hosts {
		if sreyunHostOnline(hst) {
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
		if sreyunHostOnline(hst) {
			status = "在线"
		}
		fmt.Fprintf(&b, "- id=%s 主机名=%s IP=%s 状态=%s", hst.ID, hst.Hostname, sreyunIPOr(hst.IP), status)
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

// sreyunHostOnline 判断主机是否在线（最近上报 ≤120s，与 forward.go 的离线判定一致）。
func sreyunHostOnline(h *Host) bool {
	return h != nil && h.LastSeen > 0 && time.Now().Unix()-h.LastSeen <= 120
}

func sreyunIPOr(ip string) string {
	if strings.TrimSpace(ip) == "" {
		return "未知"
	}
	return ip
}

// resolveSession 按 session_id 解析会话，作为多轮记忆的权威来源：
//   - sessionID>0：从 PostgreSQL 加载完整消息历史（刷新页面 / 切换会话后仍能延续）。
//   - sessionID==0：新建会话；若 PG 不可用或加载失败，用前端传入的 history 兜底，保证前后端状态一致。
func (h *SreyunCore) resolveSession(sessionID int64, history []map[string]string) *SreyunSession {
	sess := &SreyunSession{ID: sessionID}
	if sessionID > 0 && h.s.pg != nil {
		if raw, err := h.s.pg.loadSreyunSession(sessionID); err == nil && raw != nil {
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

// estimateTokens estimates the token count of a mixed Chinese/English text.
// Chinese characters typically use ~1.5 tokens each; ASCII ~0.5 tokens each.
func estimateTokens(text string) int {
	tokens := 0
	for _, r := range text {
		if r > 127 {
			tokens += 2 // CJK / non-ASCII: ~1.5-2 tokens
		} else {
			tokens++ // ASCII: 1 token per char (conservative)
		}
	}
	// Rough approximation: 1 token ≈ 3-4 chars for English, 1-2 chars for Chinese
	return tokens * 2 / 3
}

