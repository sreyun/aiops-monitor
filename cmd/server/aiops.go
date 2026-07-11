package main

import (
	"bufio"
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
	// Hermes Agent 配置
	HermesEnabled     bool `json:"hermes_enabled,omitempty"`      // 启用 Hermes 自主 Agent
	HermesAutoApprove bool `json:"hermes_auto_approve,omitempty"` // 低风险操作自动执行
}

// aiProviderType classifies the AI endpoint so the request/response format can be
// chosen automatically.
type aiProviderType int

const (
	aiProvOpenAI         aiProviderType = iota // OpenAI-compatible chat/completions (default)
	aiProvBailianNative                        // Bailian native text-generation/generation
	aiProvAnthropic                            // Anthropic Messages API (coding.dashscope.aliyuncs.com/apps/anthropic)
)

// isBailianEndpoint reports whether the endpoint targets Alibaba Bailian (DashScope).
func isBailianEndpoint(endpoint string) bool {
	return strings.Contains(endpoint, "dashscope.aliyuncs.com")
}

// normalizeEndpoint auto-corrects common endpoint mistakes and classifies the
// provider type so the caller can pick the right request/response format.
//
// Classification rules:
//   - Bailian native text-generation  → aiProvBailianNative
//   - Bailian /apps/anthropic          → aiProvAnthropic
//   - Bailian everything else          → aiProvOpenAI (compatible-mode)
//   - Non-Bailian                      → aiProvOpenAI
func normalizeEndpoint(endpoint string) (string, aiProviderType) {
	ep := strings.TrimRight(endpoint, "/")
	if !isBailianEndpoint(ep) {
		// Non-Bailian: if it's an OpenAI-compatible base URL without /chat/completions, append it.
		if !strings.HasSuffix(ep, "/chat/completions") && !strings.Contains(ep, "/chat/completions?") {
			if strings.HasSuffix(ep, "/v1") || strings.HasSuffix(ep, "/v1/") {
				ep += "/chat/completions"
			}
		}
		return ep, aiProvOpenAI
	}
	// Bailian Anthropic-compatible endpoint (Claude models via Anthropic Messages API)
	if strings.Contains(ep, "/apps/anthropic") || strings.Contains(ep, "/anthropic/") {
		// Anthropic endpoints DON'T use /chat/completions — they use the Messages API directly.
		return ep, aiProvAnthropic
	}
	// Bailian native text-generation API
	if strings.Contains(ep, "/api/v1/services/") || strings.Contains(ep, "/text-generation/") {
		return ep, aiProvBailianNative
	}
	// Bailian compatible-mode: ensure /chat/completions suffix
	if !strings.HasSuffix(ep, "/chat/completions") && !strings.Contains(ep, "/chat/completions?") {
		ep += "/chat/completions"
	}
	return ep, aiProvOpenAI
}

// aiChat calls an OpenAI-compatible, Bailian-native, or Anthropic-compatible
// chat/completions endpoint with a full message list (multi-turn, stdlib only).
// On a non-200 it surfaces a snippet of the provider's error body so the caller
// (e.g. the config test) can show WHY it failed.
func aiChat(cfg AIConfig, messages []map[string]string) (string, error) {
	if cfg.Endpoint == "" || cfg.Model == "" {
		return "", fmt.Errorf("AI Endpoint 或模型名未配置，请先在「AI 设置」中填写并保存")
	}

	ep, prov := normalizeEndpoint(cfg.Endpoint)

	var reqBody map[string]any
	var extraHeaders map[string]string

	switch prov {
	case aiProvBailianNative:
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
	case aiProvAnthropic:
		// Anthropic Messages API format.
		// system prompt is a top-level field, not a message role.
		var sys string
		var userMsgs []map[string]string
		for _, m := range messages {
			if m["role"] == "system" && sys == "" {
				sys = m["content"]
			} else {
				userMsgs = append(userMsgs, m)
			}
		}
		reqBody = map[string]any{
			"model":      cfg.Model,
			"max_tokens": 1024,
			"messages":   userMsgs,
		}
		if sys != "" {
			reqBody["system"] = sys
		}
		extraHeaders = map[string]string{
			"x-api-key":         cfg.APIKey,
			"anthropic-version": "2023-06-01",
		}
	default: // aiProvOpenAI
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
	if prov == aiProvAnthropic {
		// Anthropic uses x-api-key header instead of Authorization: Bearer
		for k, v := range extraHeaders {
			req.Header.Set(k, v)
		}
	} else if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	if prov == aiProvBailianNative {
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
		switch resp.StatusCode {
		case 404:
			if isBailianEndpoint(cfg.Endpoint) {
				return "", fmt.Errorf("HTTP 404：百炼 API 端点不存在。请确认 Endpoint 格式：\n"+
					"  OpenAI 兼容：https://dashscope.aliyuncs.com/compatible-mode/v1\n"+
					"  百炼原生 Text Gen：https://dashscope.aliyuncs.com/api/v1/services/aigc/text-generation/generation\n"+
					"  Anthropic 兼容：https://coding.dashscope.aliyuncs.com/apps/anthropic\n"+
					"当前端点：%s", cfg.Endpoint)
			}
			return "", fmt.Errorf("HTTP 404：API 端点不存在，请检查 Endpoint 地址是否正确（当前：%s）", cfg.Endpoint)
		case 401, 403:
			if prov == aiProvAnthropic {
				return "", fmt.Errorf("HTTP %d：认证失败，Anthropic 兼容端点请确认 API Key 是否正确（使用 x-api-key 头）", resp.StatusCode)
			}
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

	// Parse response according to provider type.
	switch prov {
	case aiProvBailianNative:
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

	case aiProvAnthropic:
		// Anthropic response: {"content":[{"type":"text","text":"..."}],"role":"assistant",...}
		var out struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Role string `json:"role"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return "", fmt.Errorf("解析 Anthropic 响应失败：%v", err)
		}
		// Collect text blocks from content array
		var texts []string
		for _, c := range out.Content {
			if c.Type == "text" && c.Text != "" {
				texts = append(texts, c.Text)
			}
		}
		if len(texts) == 0 {
			return "", fmt.Errorf("Anthropic API 返回空结果")
		}
		return strings.TrimSpace(strings.Join(texts, "\n")), nil

	default: // aiProvOpenAI
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
}

// aiComplete is the single-turn (system + user) convenience wrapper around aiChat.
func aiComplete(cfg AIConfig, system, user string) (string, error) {
	return aiChat(cfg, []map[string]string{
		{"role": "system", "content": system},
		{"role": "user", "content": user},
	})
}

// streamChat calls the AI provider with streaming enabled (SSE) and writes each
// token chunk to the ResponseWriter as an SSE "data:" event. For providers that
// do not support streaming (Anthropic), it falls back to aiChat and sends the
// whole reply as a single SSE event. Returns the accumulated full reply text.
// The caller must set the proper headers (Content-Type: text/event-stream,
// Cache-Control: no-cache, Connection: keep-alive) before calling this function.
func streamChat(w http.ResponseWriter, cfg AIConfig, messages []map[string]string) (string, error) {
	if cfg.Endpoint == "" || cfg.Model == "" {
		fmt.Fprintf(w, "data: {\"error\":\"AI 未配置\"}\n\n")
		return "", nil
	}

	ep, prov := normalizeEndpoint(cfg.Endpoint)

	// Anthropic-compatible endpoints don't support SSE streaming in the same way;
	// fall back to a single-chunk response.
	if prov == aiProvAnthropic {
		reply, err := aiChat(cfg, messages)
		if err != nil {
			fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", escapeSSE(err.Error()))
			return "", nil
		}
		fmt.Fprintf(w, "data: {\"delta\":%s}\n\n", jsonString(reply))
		fmt.Fprintf(w, "data: [DONE]\n\n")
		return reply, nil
	}

	// Build streaming request body
	var reqBody map[string]any
	var extraHeaders map[string]string

	switch prov {
	case aiProvBailianNative:
		reqBody = map[string]any{
			"model": cfg.Model,
			"input": map[string]any{
				"messages": messages,
			},
			"parameters": map[string]any{
				"temperature":        0.2,
				"result_format":      "message",
				"incremental_output": true,
			},
		}
	default: // aiProvOpenAI
		reqBody = map[string]any{
			"model":       cfg.Model,
			"messages":    messages,
			"temperature": 0.2,
			"stream":      true,
		}
	}

	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequest(http.MethodPost, ep, bytes.NewReader(b))
	if err != nil {
		fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", escapeSSE(err.Error()))
		return "", nil
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	if prov == aiProvBailianNative {
		req.Header.Set("X-DashScope-SSE", "enable")
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", escapeSSE(err.Error()))
		return "", nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 600))
		fmt.Fprintf(w, "data: {\"error\":\"HTTP %d: %s\"}\n\n", resp.StatusCode, escapeSSE(strings.TrimSpace(string(body))))
		return "", nil
	}

	// Parse SSE stream line by line, accumulating the full reply
	var fullReply strings.Builder
	flusher, _ := w.(http.Flusher)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			return fullReply.String(), nil
		}
		// Extract delta content from the chunk
		delta := parseStreamDelta(data, prov)
		if delta == "" {
			continue
		}
		fullReply.WriteString(delta)
		fmt.Fprintf(w, "data: {\"delta\":%s}\n\n", jsonString(delta))
		if flusher != nil {
			flusher.Flush()
		}
	}
	// If scanner ended without [DONE], send a done marker
	fmt.Fprintf(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
	return fullReply.String(), nil
}

// parseStreamDelta extracts the content delta from a single SSE chunk.
func parseStreamDelta(data string, prov aiProviderType) string {
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Output struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		} `json:"output"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return ""
	}
	// OpenAI-compatible format: choices[0].delta.content
	if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
		return chunk.Choices[0].Delta.Content
	}
	// Bailian native format: output.choices[0].message.content
	if len(chunk.Output.Choices) > 0 && chunk.Output.Choices[0].Message.Content != "" {
		return chunk.Output.Choices[0].Message.Content
	}
	return ""
}

// escapeSSE escapes special characters for safe SSE data field output.
func escapeSSE(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// jsonString marshals a string as a JSON string (with quotes).
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
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

// embedText calls the Bailian DashScope Embedding V2 API to convert text into a
// 1536-dimensional vector. Returns nil on any error (caller falls back gracefully).
// API: https://dashscope.aliyuncs.com/api/v1/services/embeddings/text-embedding/text-embedding
func embedText(cfg AIConfig, text string) []float64 {
	if !cfg.Enabled || cfg.APIKey == "" {
		return nil
	}
	// Only Bailian supports embedding V2; other providers are skipped silently.
	if !isBailianEndpoint(cfg.Endpoint) {
		return nil
	}
	if text = strings.TrimSpace(text); text == "" {
		return nil
	}
	// Truncate to avoid exceeding the model's input limit (text-embedding-v2: 2048 tokens).
	if len([]rune(text)) > 3000 {
		text = string([]rune(text)[:3000])
	}
	reqBody := map[string]any{
		"model": "text-embedding-v2",
		"input": map[string]any{
			"texts": []string{text},
		},
		"parameters": map[string]string{
			"text_type": "query",
		},
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequest(http.MethodPost,
		"https://dashscope.aliyuncs.com/api/v1/services/embeddings/text-embedding/text-embedding",
		bytes.NewReader(body))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var result struct {
		Output struct {
			Embeddings []struct {
				TextIndex int       `json:"text_index"`
				Embedding []float64 `json:"embedding"`
			} `json:"embeddings"`
		} `json:"output"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	if len(result.Output.Embeddings) == 0 {
		return nil
	}
	return result.Output.Embeddings[0].Embedding
}
