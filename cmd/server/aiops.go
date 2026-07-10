package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// aiComplete calls an OpenAI-compatible chat completion endpoint (stdlib only).
func aiComplete(cfg AIConfig, system, user string) (string, error) {
	if cfg.Endpoint == "" || cfg.Model == "" {
		return "", fmt.Errorf("ai endpoint/model not configured")
	}
	reqBody := map[string]any{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0.2,
		"stream":      false,
	}
	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequest(http.MethodPost, cfg.Endpoint, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ai http %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("ai empty response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// InspectionFinding is one item on an inspection report.
type InspectionFinding struct {
	Severity string `json:"severity"` // critical|warning|info
	Title    string `json:"title"`
	Detail   string `json:"detail,omitempty"`
}

// InspectionReport is one automated (or on-demand) health inspection.
type InspectionReport struct {
	ID       int64               `json:"id"`
	Ts       int64               `json:"ts"`
	Trigger  string              `json:"trigger"` // scheduled|manual
	Source   string              `json:"source"`  // ai|heuristic
	Summary  string              `json:"summary"`
	Findings []InspectionFinding `json:"findings"`
}

// inspectionContext is the snapshot the AI/heuristic engine reasons over.
type inspectionContext struct {
	OnlineHosts   int
	OfflineHosts  []string
	FiringAlerts  []Alert
	BreachingSLOs []SLOStatus
	RecentErrors  []StoredLog
	ErrorCount    int
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
	if ctx.ErrorCount > 0 {
		sev := "info"
		if ctx.ErrorCount >= 50 {
			sev = "warning"
		}
		f = append(f, InspectionFinding{sev, fmt.Sprintf("近 30 分钟错误日志 %d 条", ctx.ErrorCount),
			"可在「日志检索」按 error 级别定位。"})
	}
	summary := fmt.Sprintf("在线 %d 台 · 离线 %d 台 · firing 告警 %d 条 · SLO 超标 %d 项 · 近期错误日志 %d 条。",
		ctx.OnlineHosts, len(ctx.OfflineHosts), len(ctx.FiringAlerts), len(ctx.BreachingSLOs), ctx.ErrorCount)
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
	ctx := inspectionContext{}
	if m.snapshot != nil {
		ctx = m.snapshot()
	}
	summary, findings := heuristicInspect(ctx)
	source := "heuristic"
	if cfg := m.cfg.AIConfig(); cfg.Enabled {
		sys := "你是资深 SRE 专家。根据以下系统巡检快照，用简洁中文给出整体健康研判、风险优先级与处置建议，控制在 200 字内。"
		if out, err := aiComplete(cfg, sys, buildInspectionPrompt(ctx)); err == nil && out != "" {
			summary = out
			source = "ai"
		}
	}
	rep := InspectionReport{Trigger: trigger, Source: source, Summary: summary, Findings: findings, Ts: time.Now().Unix()}
	m.mu.Lock()
	m.nextID++
	rep.ID = m.nextID
	m.reports = append(m.reports, rep)
	if len(m.reports) > inspectionReportCap {
		m.reports = m.reports[len(m.reports)-inspectionReportCap:]
	}
	m.mu.Unlock()
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
