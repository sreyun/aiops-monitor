package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// AI Orchestrator（P2）：任务路由策略、统一调用日志、运行时统计。
// 不替代 Hermes 工具循环；先覆盖 /ai/assist 与共享观测点。
// ============================================================================

// aiTaskPolicy 描述某一 AI 任务的记忆种类与调用选项。
type aiTaskPolicy struct {
	MemKind        string
	DisableThink   bool
	Timeout        time.Duration // 0 = 用 streamChat 默认 120s
	RememberKind   string
	RememberSource string
}

// assistTaskPolicy 按 task 返回编排策略（路由 + 思考开关 + 超时）。
func assistTaskPolicy(task string) aiTaskPolicy {
	p := aiTaskPolicy{MemKind: "chat", RememberKind: "assist", RememberSource: "assist:" + task}
	switch task {
	case "audit_diagnosis", "result_diagnosis", "chart_analysis", "snmp_diagnosis", "trap_diagnosis",
		"hardware_diagnosis", "hyperv_diagnosis", "netflow_diagnosis", "checks_diagnosis",
		"forward_diagnosis", "apimon_diagnosis", "content_audit_diagnosis",
		"dashboard_analysis", "dashboard_optimize":
		p.MemKind = "diagnosis"
	}
	switch task {
	case "dashboard_prompt_optimize", "dashboard_optimize", "dashboard_analysis":
		p.DisableThink = true
		p.Timeout = 90 * time.Second
	case "logql", "promql", "playbook", "remediation_rule", "remediation_proposal":
		p.Timeout = 90 * time.Second
	}
	return p
}

// aiCallStat 单次 AI 调用观测样本（内存环形缓冲，供管理页仪表）。
type aiCallStat struct {
	Ts           int64  `json:"ts"`
	Task         string `json:"task"`
	Model        string `json:"model"`
	LatencyMs    int64  `json:"latency_ms"`
	OK           bool   `json:"ok"`
	Error        string `json:"error,omitempty"`
	MemHits      int    `json:"memory_hits"`
	SkillHits    int    `json:"skill_hits"`
	ReplyChars   int    `json:"reply_chars"`
	ApproxTokens int    `json:"approx_tokens"` // 按字符粗估，非 Provider 精确账单
}

type aiTaskAgg struct {
	Count int   `json:"count"`
	Fail  int   `json:"fail"`
	AvgMs int64 `json:"avg_ms"`
	sumMs int64
}

type aiStatsHub struct {
	mu         sync.Mutex
	recent     []aiCallStat
	cap        int
	total      int64
	fail       int64
	sumLatency int64
	sumTokens  int64
}

func newAIStatsHub() *aiStatsHub {
	return &aiStatsHub{cap: 200, recent: make([]aiCallStat, 0, 64)}
}

func (h *aiStatsHub) record(st aiCallStat) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.total++
	h.sumLatency += st.LatencyMs
	h.sumTokens += int64(st.ApproxTokens)
	if !st.OK {
		h.fail++
	}
	h.recent = append(h.recent, st)
	if len(h.recent) > h.cap {
		h.recent = h.recent[len(h.recent)-h.cap:]
	}
}

func (h *aiStatsHub) snapshot() map[string]any {
	if h == nil {
		return map[string]any{
			"total": 0, "fail": 0, "avg_latency_ms": 0, "fail_rate": 0,
			"approx_tokens_total": 0, "by_task": map[string]aiTaskAgg{}, "recent": []aiCallStat{},
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	avg := int64(0)
	if h.total > 0 {
		avg = h.sumLatency / h.total
	}
	failRate := 0.0
	if h.total > 0 {
		failRate = float64(h.fail) / float64(h.total)
	}
	byTask := map[string]*aiTaskAgg{}
	for _, r := range h.recent {
		m := byTask[r.Task]
		if m == nil {
			m = &aiTaskAgg{}
			byTask[r.Task] = m
		}
		m.Count++
		m.sumMs += r.LatencyMs
		if !r.OK {
			m.Fail++
		}
	}
	outByTask := map[string]aiTaskAgg{}
	for k, m := range byTask {
		if m.Count > 0 {
			m.AvgMs = m.sumMs / int64(m.Count)
		}
		outByTask[k] = aiTaskAgg{Count: m.Count, Fail: m.Fail, AvgMs: m.AvgMs}
	}
	recent := make([]aiCallStat, len(h.recent))
	copy(recent, h.recent)
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}
	if len(recent) > 30 {
		recent = recent[:30]
	}
	return map[string]any{
		"total":               h.total,
		"fail":                h.fail,
		"avg_latency_ms":      avg,
		"fail_rate":           failRate,
		"approx_tokens_total": h.sumTokens,
		"by_task":             outByTask,
		"recent":              recent,
	}
}

// recordAICall 统一观测入口（assist / chat / diagnose 等均可调用）。
func (s *Server) recordAICall(task, model string, latencyMs int64, ok bool, errStr string, memHits, skillHits int, reply string) {
	if s == nil || s.aiStats == nil {
		return
	}
	approx := estimateTokens(reply)
	s.aiStats.record(aiCallStat{
		Ts: time.Now().Unix(), Task: task, Model: model,
		LatencyMs: latencyMs, OK: ok, Error: trimLine(errStr, 200),
		MemHits: memHits, SkillHits: skillHits,
		ReplyChars: len([]rune(reply)), ApproxTokens: approx,
	})
	slog.Info("ai.call",
		"task", task, "model", model, "latency_ms", latencyMs,
		"ok", ok, "memory_hits", memHits, "skill_hits", skillHits,
		"approx_tokens", approx, "err", errStr)
}

// streamOrchestratedAssist：assist 统一编排 —— RAG 注入、策略应用、流式调用、统计与记忆沉淀。
func (s *Server) streamOrchestratedAssist(ctx context.Context, w http.ResponseWriter, cfg AIConfig, task, userMsg, contextText string, history []map[string]string) string {
	policy := assistTaskPolicy(task)
	sys := buildAssistSystemPrompt(task, contextText)
	ragQ := strings.TrimSpace(userMsg + " " + contextText)
	memText, memHits, degM := s.retrieveMemoryDetailed(policy.MemKind, ragQ, 6)
	skillText, skillNames, skillHits, degS := s.retrieveSkillsDetailed(ragQ, 4)
	sys += memText + skillText
	deg := degM
	if deg == "" {
		deg = degS
	}
	writeRAGMetaSSE(w, memHits, skillHits, deg, skillNames)

	if strings.TrimSpace(userMsg) == "" {
		userMsg = "请根据上述上下文进行分析并给出结论。"
	}
	msgs := []map[string]string{{"role": "system", "content": sys}}
	if n := len(history); n > 0 {
		start := 0
		if n > 20 {
			start = n - 20
		}
		for _, h := range history[start:] {
			role, content := h["role"], strings.TrimSpace(h["content"])
			if (role == "user" || role == "assistant") && content != "" {
				msgs = append(msgs, map[string]string{"role": role, "content": content})
			}
		}
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": userMsg})

	opts := aiCallOpts{DisableThinking: policy.DisableThink, Timeout: policy.Timeout}
	start := time.Now()
	reply, err := streamChatOpts(ctx, w, cfg, msgs, nil, opts)
	latency := time.Since(start).Milliseconds()
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	s.recordAICall(task, cfg.Model, latency, err == nil, errStr, memHits, skillHits, reply)

	if strings.TrimSpace(reply) != "" {
		go s.rememberAI(policy.RememberKind, policy.RememberSource,
			fmt.Sprintf("【AI 辅助·%s】\n%s\n\n【AI】\n%s", task, userMsg, reply))
	}
	return reply
}
