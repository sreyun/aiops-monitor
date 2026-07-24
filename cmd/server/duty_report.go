package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ============================================================================
// 值班晨报（duty report）—— SRE 中枢的每日态势汇总闭环。
//
// 每天定时把当前运维态势（未决事件 / SLO 燃尽 / 待审批自动修复 / 最新 AI 巡检）汇总，交 AI
// 生成一份务实的值班晨报，推送到消息中心。态势平静（无任何需关注项）时跳过推送，不刷屏。
// 同一份态势汇总也供前端「生成值班晨报」按钮走 /ai/assist 流式查看，服务端/前端共用一处口径。
// ============================================================================

const dutyReportSystemPrompt = "你是 SRE 值班主管，正在给运维团队写一份值班晨报。请基于给定的运维态势，用简洁中文生成晨报：" +
	"① 开头一句话总体研判（今天要重点盯什么）；② 按优先级列出需处理事项（未决事件 / SLO 超标 / 待审批修复，" +
	"每项点明建议动作）；③ 若有 AI 巡检风险，点出关键项；④ 结尾一句值班提醒。控制在 300 字内，务实可执行、不客套、不臆造给定之外的信息。"

// buildDutyReportContext 汇总当前运维态势为可读文本。notable=false 表示一切平静
// （无未决事件 / 无 SLO 超标 / 无待审批修复 / 巡检无风险），定时任务据此跳过推送。
func (s *Server) buildDutyReportContext() (string, bool) {
	var b strings.Builder
	notable := false

	// 未决事件（open + acknowledged）
	var openIncs []Incident
	for _, inc := range s.incidents.List() {
		if inc.Status != "resolved" {
			openIncs = append(openIncs, inc)
		}
	}
	if len(openIncs) > 0 {
		notable = true
		fmt.Fprintf(&b, "【未决事件】共 %d 起：\n", len(openIncs))
		for i, inc := range openIncs {
			if i >= 10 {
				fmt.Fprintf(&b, "- …另有 %d 起未列出\n", len(openIncs)-10)
				break
			}
			fmt.Fprintf(&b, "- #%d [%s/%s] %s（主机 %s，状态 %s）\n",
				inc.ID, inc.Severity, inc.Type, trimLine(inc.Title, 80), inc.Hostname, inc.Status)
		}
	} else {
		b.WriteString("【未决事件】无\n")
	}

	// SLO 燃尽
	var breaching []SLOStatus
	for _, st := range s.slos.Evaluate() {
		if st.Enabled && st.Breaching {
			breaching = append(breaching, st)
		}
	}
	if len(breaching) > 0 {
		notable = true
		fmt.Fprintf(&b, "\n【SLO 超标】共 %d 项：\n", len(breaching))
		for _, st := range breaching {
			fmt.Fprintf(&b, "- %s：SLI %.2f%% < 目标 %.2f%%，错误预算剩余 %.0f%%，燃尽率 %.2f\n",
				st.Name, st.SLI, st.Target, st.ErrorBudget, st.BurnRate)
		}
	}

	// 待审批自动修复
	var pending []RemediationRun
	for _, run := range s.remediation.Runs() {
		if run.Status == "pending_approval" {
			pending = append(pending, run)
		}
	}
	if len(pending) > 0 {
		notable = true
		fmt.Fprintf(&b, "\n【待审批自动修复】共 %d 项：\n", len(pending))
		for i, run := range pending {
			if i >= 8 {
				break
			}
			fmt.Fprintf(&b, "- 规则「%s」→ 剧本「%s」（主机 %s，触发告警 %s）\n",
				run.RuleName, run.PlaybookName, run.Hostname, run.AlertType)
		}
	}

	// 最新 AI 巡检结论（Reports() 为最新在前）
	if reports := s.ai.Reports(); len(reports) > 0 {
		last := reports[0]
		crit, warn := 0, 0
		for _, f := range last.Findings {
			if f.Severity == "critical" {
				crit++
			} else if f.Severity == "warning" {
				warn++
			}
		}
		if crit+warn > 0 {
			notable = true
		}
		fmt.Fprintf(&b, "\n【最新 AI 巡检】发现 %d 严重 / %d 警告：%s\n", crit, warn, trimLine(last.Summary, 200))
	}

	return b.String(), notable
}

// generateDutyReport 汇总态势并调用 AI 生成晨报文本。返回 (晨报, 是否有值得关注的态势, error)。
func (s *Server) generateDutyReport() (string, bool, error) {
	ctx, notable := s.buildDutyReportContext()
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.Endpoint == "" || cfg.Model == "" {
		return "", notable, fmt.Errorf("AI 未配置或未启用")
	}
	report, err := aiComplete(cfg, dutyReportSystemPrompt, "运维态势：\n"+ctx)
	return report, notable, err
}

// durationUntilNext 返回从现在到下一个 hh:mm（服务器本地时间）的时长。
func durationUntilNext(hh, mm int) time.Duration {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}

// runDutyReportLoop 每天到点（默认 08:00 服务器本地时间）生成并推送值班晨报。
// AI 未启用或态势平静时跳过，避免无意义打扰；晨报同时沉淀为 RAG 记忆。
func (s *Server) runDutyReportLoop() {
	for {
		time.Sleep(durationUntilNext(8, 0))
		if c := s.cfg.AIConfig(); !c.Enabled {
			continue
		}
		report, notable, err := s.generateDutyReport()
		if err != nil || !notable || strings.TrimSpace(report) == "" {
			continue // 平静或失败：不推送
		}
		s.messages.push("ai", "info", "🌅 值班晨报", trimLine(report, 400), "sre", "")
		if s.shouldRememberUnverifiedAIOutput() {
			go s.rememberAI("duty_report", "duty:daily", report)
		}
	}
}

// handleDutyContext 返回当前运维态势汇总，供前端「生成值班晨报」按钮走 /ai/assist 流式生成。
// GET /api/v1/ai/duty-context
func (s *Server) handleDutyContext(w http.ResponseWriter, r *http.Request) {
	ctx, notable := s.buildDutyReportContext()
	writeJSON(w, http.StatusOK, map[string]any{"context": ctx, "notable": notable})
}
