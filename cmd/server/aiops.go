package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// AI layer — automated inspection + agent-style incident diagnosis.
//
// The LLM is a PLUGGABLE ENHANCEMENT ("埋点"): configure any OpenAI-compatible
// chat/completions endpoint and the inspector/diagnoser use it; leave it off and
// a built-in heuristic engine produces the same structured output, so the whole
// feature works out of the box with zero external dependencies.
// ============================================================================

// AIConfig configures the optional AI provider.
type AIConfig struct {
	Enabled            bool   `json:"enabled"`
	Endpoint           string `json:"endpoint"` // e.g. https://api.openai.com/v1/chat/completions
	APIKey             string `json:"api_key,omitempty"`
	Model              string `json:"model"`                // e.g. gpt-4o-mini / a local model name
	InspectIntervalMin int    `json:"inspect_interval_min"` // 0 = default 30
}

// isBailianEndpoint reports whether the endpoint targets Alibaba Bailian (DashScope).
func isBailianEndpoint(endpoint string) bool {
	return strings.Contains(endpoint, "dashscope.aliyuncs.com")
}

// normalizeEndpoint auto-corrects common endpoint mistakes:
// - Bailian compatible-mode without /chat/completions suffix → append it
// - Bailian native-mode (api/v1/services) → leave as-is for native handling
// Returns the (possibly corrected) endpoint and whether it's Bailian native mode.
func normalizeEndpoint(endpoint string) (string, bool) {
	ep := strings.TrimRight(endpoint, "/")
	if !isBailianEndpoint(ep) {
		// Non-Bailian: if it's an OpenAI-compatible base URL without /chat/completions, append it.
		if !strings.HasSuffix(ep, "/chat/completions") && !strings.Contains(ep, "/chat/completions?") {
			// Check if it looks like a base URL (ends with /v1 or similar)
			if strings.HasSuffix(ep, "/v1") || strings.HasSuffix(ep, "/v1/") {
				ep += "/chat/completions"
			}
		}
		return ep, false
	}
	// Bailian: check if it's the native API endpoint (not compatible-mode)
	if strings.Contains(ep, "/api/v1/services/") || strings.Contains(ep, "/text-generation/") {
		return ep, true // native mode
	}
	// Bailian compatible-mode: ensure /chat/completions suffix
	if !strings.HasSuffix(ep, "/chat/completions") && !strings.Contains(ep, "/chat/completions?") {
		ep += "/chat/completions"
	}
	return ep, false
}

// aiChat calls an OpenAI-compatible or Bailian-native chat/completions endpoint
// with a full message list (multi-turn, stdlib only). On a non-200 it surfaces a
// snippet of the provider's error body so the caller (e.g. the config test) can
// show WHY it failed.
func aiChat(cfg AIConfig, messages []map[string]string) (string, error) {
	if cfg.Endpoint == "" || cfg.Model == "" {
		return "", fmt.Errorf("AI Endpoint 或模型名未配置，请先在「AI 设置」中填写并保存")
	}

	ep, isBailianNative := normalizeEndpoint(cfg.Endpoint)

	var reqBody map[string]any
	if isBailianNative {
		// Bailian native API format: https://dashscope.aliyuncs.com/api/v1/services/aigc/text-generation/generation
		reqBody = map[string]any{
			"model": cfg.Model,
			"input": map[string]any{
				"messages": messages,
			},
			"parameters": map[string]any{
				"temperature":   0.2,
				"result_format": "message",
			},
		}
	} else {
		// OpenAI-compatible format
		reqBody = map[string]any{
			"model":       cfg.Model,
			"messages":    messages,
			"temperature": 0.2,
			"stream":      false,
		}
	}

	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequest(http.MethodPost, ep, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	// Bailian native API also accepts X-DashScope-SSE header control
	if isBailianNative {
		req.Header.Set("X-DashScope-SSE", "disable")
	}

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("网络请求失败：%v（请检查 Endpoint 地址是否正确）", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 600))
		msg := strings.TrimSpace(string(body))
		// Provide actionable hints for common status codes
		switch resp.StatusCode {
		case 404:
			if isBailianEndpoint(cfg.Endpoint) {
				return "", fmt.Errorf("HTTP 404：百炼 API 端点不存在。请确认 Endpoint 格式：\n"+
					"  兼容模式：https://dashscope.aliyuncs.com/compatible-mode/v1\n"+
					"  原生模式：https://dashscope.aliyuncs.com/api/v1/services/aigc/text-generation/generation\n"+
					"当前端点：%s", cfg.Endpoint)
			}
			return "", fmt.Errorf("HTTP 404：API 端点不存在，请检查 Endpoint 地址是否正确（当前：%s）", cfg.Endpoint)
		case 401, 403:
			return "", fmt.Errorf("HTTP %d：认证失败，请检查 API Key 是否正确且有效", resp.StatusCode)
		case 400:
			if msg != "" {
				return "", fmt.Errorf("HTTP 400：请求参数错误 — %s", trimLine(msg, 200))
			}
			return "", fmt.Errorf("HTTP 400：请求参数错误，请检查模型名称是否正确（当前：%s）", cfg.Model)
		default:
			if msg != "" {
				return "", fmt.Errorf("HTTP %d：%s", resp.StatusCode, trimLine(msg, 220))
			}
			return "", fmt.Errorf("HTTP %d：服务端返回异常状态码", resp.StatusCode)
		}
	}

	// Parse response — handle both Bailian native and OpenAI-compatible formats.
	if isBailianNative {
		// Bailian native response: {"output": {"choices": [{"message": {"content": "..."}}]}}
		var out struct {
			Output struct {
				Choices []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
			} `json:"output"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return "", fmt.Errorf("解析百炼响应失败：%v", err)
		}
		if len(out.Output.Choices) == 0 {
			return "", fmt.Errorf("百炼 API 返回空结果")
		}
		return strings.TrimSpace(out.Output.Choices[0].Message.Content), nil
	}

	// OpenAI-compatible response: {"choices": [{"message": {"content": "..."}}]}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("解析 AI 响应失败：%v", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("AI 服务返回空结果")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// aiComplete is the single-turn (system + user) convenience wrapper around aiChat.
func aiComplete(cfg AIConfig, system, user string) (string, error) {
	return aiChat(cfg, []map[string]string{
		{"role": "system", "content": system},
		{"role": "user", "content": user},
	})
}

// InspectionFinding is one item on an inspection report.
type InspectionFinding struct {
	Severity string `json:"severity"` // critical|warning|info
	Title    string `json:"title"`
	Detail   string `json:"detail,omitempty"`
}

// InspectionReport is one automated (or on-demand) health inspection.
type InspectionReport struct {
	ID         int64               `json:"id"`
	Ts         int64               `json:"ts"`
	Trigger    string              `json:"trigger"`               // scheduled|manual
	Source     string              `json:"source"`                // ai|heuristic
	Model      string              `json:"model,omitempty"`       // AI model used, or 启发式规则
	Context    string              `json:"context,omitempty"`     // human-readable "what was inspected"
	DurationMs int64               `json:"duration_ms,omitempty"` // how long this round took
	Summary    string              `json:"summary"`
	Findings   []InspectionFinding `json:"findings"`
}

// inspectionContext is the snapshot the AI/heuristic engine reasons over.
type inspectionContext struct {
	OnlineHosts   int
	OfflineHosts  []string
	FiringAlerts  []Alert
	BreachingSLOs []SLOStatus
	RecentErrors  []StoredLog
	ErrorCount    int
	WarnCount     int
	HighUsage     []string
}

const inspectionReportCap = 60

type aiManager struct {
	mu      sync.Mutex
	cfg     *ConfigStore
	reports []InspectionReport
	nextID  int64
	// injected data sources (set during wiring)
	snapshot    func() inspectionContext
	diagContext func(inc Incident) string
	onReport    func(rep InspectionReport) // notify hook: surface findings as messages
}

func newAIManager(cfg *ConfigStore) *aiManager { return &aiManager{cfg: cfg, nextID: 1} }

// heuristicInspect turns a snapshot into a summary + structured findings without
// any LLM — the reliable baseline the AI narrative sits on top of.
func heuristicInspect(ctx inspectionContext) (string, []InspectionFinding) {
	var f []InspectionFinding
	for _, hn := range ctx.OfflineHosts {
		f = append(f, InspectionFinding{"critical", "主机离线：" + hn, "该主机已失联，请检查网络连通与 Agent 进程。"})
	}
	for _, a := range ctx.FiringAlerts {
		sev := "warning"
		if a.Level == "critical" {
			sev = "critical"
		}
		f = append(f, InspectionFinding{sev, a.Hostname + " · " + a.Message, ""})
	}
	for _, s := range ctx.BreachingSLOs {
		f = append(f, InspectionFinding{"warning", "SLO 未达标：" + s.Name,
			fmt.Sprintf("SLI %.2f%% < 目标 %.2f%%，错误预算剩余 %.0f%%。", s.SLI, s.Target, s.ErrorBudget)})
	}
	for _, hu := range ctx.HighUsage {
		f = append(f, InspectionFinding{"warning", "资源高位：" + hu, ""})
	}
	if ctx.ErrorCount > 0 || ctx.WarnCount > 0 {
		sev := "info"
		if ctx.ErrorCount >= 50 {
			sev = "critical"
		} else if ctx.ErrorCount >= 10 {
			sev = "warning"
		}
		f = append(f, InspectionFinding{sev,
			fmt.Sprintf("近 30 分钟日志：error %d 条 · warn %d 条", ctx.ErrorCount, ctx.WarnCount),
			"可在「日志检索」按级别 + 主机定位错误起始时间与来源服务。"})
	}
	summary := fmt.Sprintf("在线 %d 台 · 离线 %d 台 · firing 告警 %d 条 · SLO 超标 %d 项 · 近 30 分钟 error %d/warn %d 条。",
		ctx.OnlineHosts, len(ctx.OfflineHosts), len(ctx.FiringAlerts), len(ctx.BreachingSLOs), ctx.ErrorCount, ctx.WarnCount)
	if len(f) == 0 {
		summary = "系统健康：本轮巡检未发现异常。"
	}
	return summary, f
}

// buildInspectionPrompt renders the snapshot as text for the LLM.
func buildInspectionPrompt(ctx inspectionContext) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("在线主机 %d 台。\n", ctx.OnlineHosts))
	if len(ctx.OfflineHosts) > 0 {
		b.WriteString("离线主机：" + strings.Join(ctx.OfflineHosts, "、") + "\n")
	}
	if len(ctx.FiringAlerts) > 0 {
		b.WriteString("正在触发的告警：\n")
		for _, a := range ctx.FiringAlerts {
			b.WriteString(fmt.Sprintf("  - [%s] %s · %s\n", a.Level, a.Hostname, a.Message))
		}
	}
	if len(ctx.BreachingSLOs) > 0 {
		b.WriteString("未达标 SLO：\n")
		for _, s := range ctx.BreachingSLOs {
			b.WriteString(fmt.Sprintf("  - %s: SLI %.2f%% / 目标 %.2f%%\n", s.Name, s.SLI, s.Target))
		}
	}
	if len(ctx.HighUsage) > 0 {
		b.WriteString("资源高位：" + strings.Join(ctx.HighUsage, "、") + "\n")
	}
	if len(ctx.RecentErrors) > 0 {
		b.WriteString(fmt.Sprintf("近期错误日志（%d 条，节选）：\n", ctx.ErrorCount))
		for i, e := range ctx.RecentErrors {
			if i >= 15 {
				break
			}
			b.WriteString("  - " + e.Hostname + ": " + trimLine(e.Message, 160) + "\n")
		}
	}
	if ctx.WarnCount > 0 {
		b.WriteString(fmt.Sprintf("近 30 分钟告警(warn)级日志 %d 条（可作为错误的前兆信号）。\n", ctx.WarnCount))
	}
	if b.Len() == 0 {
		b.WriteString("无异常指标。")
	}
	return b.String()
}

func trimLine(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// RunInspection performs one inspection (AI-enhanced when configured, heuristic
// otherwise) and stores the report.
func (m *aiManager) RunInspection(trigger string) InspectionReport {
	start := time.Now()
	ctx := inspectionContext{}
	if m.snapshot != nil {
		ctx = m.snapshot()
	}
	// Human-readable description of exactly what this round looked at — surfaced in
	// the report so operators can see the AI/heuristic actually ran over real data.
	inspectCtx := fmt.Sprintf("巡检范围：在线主机 %d 台 · 离线 %d 台 · firing 告警 %d 条 · SLO %d 项 · 近 30 分钟 error %d/warn %d 条 · 资源高位 %d 项。",
		ctx.OnlineHosts, len(ctx.OfflineHosts), len(ctx.FiringAlerts), len(ctx.BreachingSLOs), ctx.ErrorCount, ctx.WarnCount, len(ctx.HighUsage))
	summary, findings := heuristicInspect(ctx)
	source, model := "heuristic", "启发式规则"
	if cfg := m.cfg.AIConfig(); cfg.Enabled {
		sys := "你是资深 SRE 专家。根据以下系统巡检快照，用简洁中文给出整体健康研判、风险优先级与处置建议，控制在 200 字内。"
		if out, err := aiComplete(cfg, sys, buildInspectionPrompt(ctx)); err == nil && out != "" {
			summary = out
			source = "ai"
			model = cfg.Model
		}
	}
	rep := InspectionReport{
		Trigger: trigger, Source: source, Model: model, Context: inspectCtx,
		DurationMs: time.Since(start).Milliseconds(),
		Summary:   summary, Findings: findings, Ts: time.Now().Unix(),
	}
	m.mu.Lock()
	m.nextID++
	rep.ID = m.nextID
	m.reports = append(m.reports, rep)
	if len(m.reports) > inspectionReportCap {
		m.reports = m.reports[len(m.reports)-inspectionReportCap:]
	}
	m.mu.Unlock()
	if m.onReport != nil {
		m.onReport(rep)
	}
	return rep
}

// Reports returns inspection history newest-first.
func (m *aiManager) Reports() []InspectionReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]InspectionReport, len(m.reports))
	copy(out, m.reports)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// exportReports returns inspection history in chronological (storage) order for
// PG persistence.
func (m *aiManager) exportReports() []InspectionReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]InspectionReport, len(m.reports))
	copy(out, m.reports)
	return out
}

// importReports restores inspection history from PG on startup, resuming the ID
// sequence from the highest persisted ID so new reports never collide.
func (m *aiManager) importReports(reps []InspectionReport) {
	if len(reps) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(reps) > inspectionReportCap {
		reps = reps[len(reps)-inspectionReportCap:]
	}
	m.reports = reps
	for _, r := range reps {
		if r.ID > m.nextID {
			m.nextID = r.ID
		}
	}
}

// runInspectionLoop is the scheduled inspector.
func (m *aiManager) runInspectionLoop() {
	for {
		iv := 30
		if c := m.cfg.AIConfig(); c.InspectIntervalMin > 0 {
			iv = c.InspectIntervalMin
		}
		time.Sleep(time.Duration(iv) * time.Minute)
		m.RunInspection("scheduled")
	}
}

// diagHint returns a rule-of-thumb remediation direction per alert type.
func diagHint(typ string) string {
	m := map[string]string{
		"cpu":     "定位高 CPU 进程（top / pidstat）；排查失控进程或流量突增；必要时限流或扩容。",
		"memory":  "查看内存占用 TOP（free / ps aux --sort=-%mem）；排查内存泄漏或缓存膨胀；必要时重启相关服务或扩容。",
		"disk":    "定位大文件/目录（du -sh *）；清理日志与临时文件；排查写入是否激增；必要时扩容。",
		"diskio":  "定位高 IO 进程（iotop）；排查慢查询或大批量写入；考虑限速或迁移。",
		"iops":    "排查高频小 IO 来源（数据库/日志刷盘）；评估合并写入或换用更高 IOPS 存储。",
		"load":    "结合 CPU/IO/进程数判断瓶颈；定位 D 状态阻塞进程；排查下游依赖是否卡顿。",
		"offline": "检查主机网络连通与 Agent 进程是否存活；确认是否宕机或正在重启。",
		"gpu":     "查看 GPU 占用与温度（nvidia-smi）；排查失控训练/推理任务。",
		"proc":    "对比进程基线，定位异常拉起或退出的服务，检查是否 OOM/崩溃重启。",
	}
	if v, ok := m[typ]; ok {
		return v
	}
	return "结合指标趋势与错误日志定位异常起始时间，缩小到具体服务/进程后再处置。"
}

// heuristicDiagnose produces a rule-based diagnosis when no AI is configured.
func heuristicDiagnose(inc Incident, ctx string) string {
	var b strings.Builder
	b.WriteString("【启发式诊断 · 基于规则】\n\n根因方向：\n")
	b.WriteString(diagHint(inc.Type) + "\n")
	if ctx != "" {
		b.WriteString("\n采集到的上下文：\n" + ctx)
	}
	b.WriteString("\n提示：在「AI 巡检」页配置 AI Provider 后，可获得智能体级别的根因研判与处置编排。")
	return b.String()
}

// Diagnose returns a diagnosis for an incident (AI when configured, heuristic
// otherwise). The caller appends the result to the incident timeline.
func (m *aiManager) Diagnose(inc Incident) (string, string) {
	ctx := ""
	if m.diagContext != nil {
		ctx = m.diagContext(inc)
	}
	if cfg := m.cfg.AIConfig(); cfg.Enabled {
		sys := "你是资深 SRE 值班工程师。根据事件与主机上下文，给出：1) 按可能性排序的根因假设；2) 具体可执行的处置步骤。简洁中文，分点。"
		if out, err := aiComplete(cfg, sys, ctx); err == nil && out != "" {
			return out, "ai"
		}
	}
	return heuristicDiagnose(inc, ctx), "heuristic"
}
