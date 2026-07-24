package main

import (
	"fmt"
	"log/slog"
	"strings"
)

// ============================================================================
// 学习闭环（self-evolution）
//
// 把「被采纳 / 执行成功 / 事件解决 / 用户 👍」等**真实结果**回流为记忆强化信号，与
// pgstore.decayOldMemories 的负向衰减对称，共同驱动 RAG 记忆随使用自我进化：
//
//   观察/生成(assist·diagnose) → 采纳/执行(apply·playbook·resolve) → 记录结果 + 强化促成它的记忆
//     → 检索时优先复用被验证有效的知识 → 下次生成/诊断更准 ↺
//
// 强化因子刻意温和（需多次累积才显著），避免单次反馈过度主导；上限见 memoryPriorityCap。
// ============================================================================

const (
	reinforceApplied  = 1.6 // AI 建议被采纳 / 应用到实际操作
	reinforceHelpful  = 1.4 // 用户显式 👍
	reinforceResolved = 1.5 // 诊断所属事件被最终解决（现实验证）
	reinforceSuccess  = 1.3 // 剧本 / 补救执行成功
	penalizeUnhelpful = 0.6 // 用户显式 👎
)

// reinforceMemory 语义强化：embed(text) 后定位最相近的一条 kind 记忆并调整其优先级。
// 用于 source 不唯一、需按内容定位的场景（如 AI 辅助采纳）。异步、尽力而为，绝不阻塞主流程。
func (s *Server) reinforceMemory(kind, text string, factor float64) {
	if s.pg == nil {
		return
	}
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.APIKey == "" || strings.TrimSpace(text) == "" {
		return
	}
	go func() {
		emb := embedText(cfg, text)
		if len(emb) == 0 {
			return
		}
		if id, ok := s.pg.boostNearestMemory(emb, kind, factor); ok {
			slog.Info("学习闭环·记忆强化", "kind", kind, "id", id, "factor", factor)
		}
	}()
}

// reinforceMemoryBySource 精确强化：按 kind+source 调整优先级（source 唯一时用，无需嵌入）。异步。
func (s *Server) reinforceMemoryBySource(kind, source string, factor float64) {
	if s.pg == nil {
		return
	}
	go func() {
		if n := s.pg.boostMemoryBySource(kind, source, factor); n > 0 {
			slog.Info("学习闭环·按来源强化记忆", "kind", kind, "source", source, "factor", factor, "affected", n)
		}
	}()
}

// rememberPlaybookOutcome 闭环 B：剧本执行完成后，把结果沉淀为经验记忆——成败统计 + 关键失败
// 输出 + 步骤命令，供后续「AI 生成剧本 / 事件诊断」检索复用。全部主机成功则额外强化该剧本经验，
// 使被现实验证有效的自动化做法在检索中上浮。
func (s *Server) rememberPlaybookOutcome(pb Playbook, exec *PlaybookExecution, status string) {
	if s.pg == nil {
		return
	}
	summary, _, failN := summarizePlaybookOutcome(pb, exec, status)
	s.rememberAI("experience", "playbook:"+pb.ID, summary)
	// 全部主机成功 → 强化该剧本经验（被现实验证有效），使其在后续 RAG 检索中上浮
	if status == "completed" && failN == 0 {
		s.reinforceMemoryBySource("experience", "playbook:"+pb.ID, reinforceSuccess)
	}
}

// summarizePlaybookOutcome 是纯函数：把一次剧本执行汇总为可入库检索的经验文本，并返回成败台数。
// 拆出为纯函数便于单测且无副作用。
func summarizePlaybookOutcome(pb Playbook, exec *PlaybookExecution, status string) (summary string, okN, failN int) {
	var failSample []string
	for _, r := range exec.HostResults {
		if r.Status == "success" {
			okN++
			continue
		}
		failN++
		for _, st := range r.Steps {
			if st.Status == "failed" && len(failSample) < 3 {
				failSample = append(failSample, fmt.Sprintf("[%s@%s] %s", st.Name, r.Hostname, trimLine(st.Output, 200)))
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "剧本「%s」执行%s：成功 %d 台 / 失败 %d 台。", pb.Name, status, okN, failN)
	if strings.TrimSpace(pb.Description) != "" {
		fmt.Fprintf(&b, "用途：%s。", pb.Description)
	}
	var cmds []string
	for _, stp := range pb.Steps {
		if strings.TrimSpace(stp.Command) != "" {
			cmds = append(cmds, stp.Name+": "+trimLine(stp.Command, 160))
		}
	}
	if len(cmds) > 0 {
		b.WriteString("\n步骤：\n" + strings.Join(cmds, "\n"))
	}
	if len(failSample) > 0 {
		b.WriteString("\n失败样本：\n" + strings.Join(failSample, "\n"))
	}
	return b.String(), okN, failN
}

func resolutionNoteFromIncident(inc Incident) string {
	for i := len(inc.Timeline) - 1; i >= 0; i-- {
		t := inc.Timeline[i]
		text := strings.TrimSpace(t.Text)
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "解决说明：") {
			return strings.TrimSpace(strings.TrimPrefix(text, "解决说明："))
		}
	}
	for i := len(inc.Timeline) - 1; i >= 0; i-- {
		t := inc.Timeline[i]
		if (t.Kind == "recovered" || t.Kind == "resolved") && strings.TrimSpace(t.Text) != "" {
			return strings.TrimSpace(t.Text)
		}
	}
	return ""
}

// learnFromResolution 闭环 C：事件解决后，沉淀结构化「结案经验」卡片，并强化促成解决的诊断记忆。
// note 为可选的解决说明（也可由 resolutionNoteFromIncident 从时间线提取）。
func (s *Server) learnFromResolution(inc Incident, note string) {
	if s.pg == nil {
		return
	}
	if strings.TrimSpace(note) == "" {
		note = resolutionNoteFromIncident(inc)
	}
	diag := latestTimelineText(inc, "ai_diagnosis")
	card := buildResolutionCardFromIncident(inc, note)
	card = s.enrichResolutionCardWithAI(card, diag)
	text := formatResolutionCard(card)
	if strings.TrimSpace(text) == "" {
		return
	}
	s.rememberAI("resolution", fmt.Sprintf("incident:%d", inc.ID), text)
	// 真实结案是比单次点赞更强的现实验证信号：提升该事件最新诊断案例。
	if s.pg != nil {
		_ = s.pg.updateDiagnosisFeedback(inc.ID, "helpful")
	}
	s.reinforceMemoryBySource("diagnosis", fmt.Sprintf("incident:%d", inc.ID), reinforceResolved)
	s.reinforceSkill(inc.Title+" "+inc.Type+" "+note, reinforceResolved)
	// 反馈驱动：被解决验证的结案卡尝试升格为可复用 Skill（去重合并，异步）
	s.promoteTextToSkill("resolution", fmt.Sprintf("incident:%d", inc.ID), text)
	// 结案时沉淀「问题+结论+文档标题」为已验证文档引用（非整库镜像）
	titles := extractDocTitlesFromText(text)
	s.persistAdoptedKnowledge(inc.Title+" "+inc.Type, text, fmt.Sprintf("knowledge:incident:%d", inc.ID), titles)
}

// correlateIncident 事件自动串联（知识复用闭环）：新事件创建时用 RAG 召回相似历史事件、已验证的
// 解决经验，并匹配已有的自动修复规则（含草稿），自动挂到事件时间线——让复现问题能立刻复用沉淀下来
// 的诊断脉络与处置，并提示"已有可启用的修复规则"。异步、尽力而为；无 pg/embedding 或无有价值命中
// 时静默跳过，绝不用空结果打扰时间线。
func (s *Server) correlateIncident(inc Incident) {
	if s.pg == nil {
		return
	}
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.APIKey == "" {
		return
	}
	query := strings.TrimSpace(inc.Title + " " + inc.Type + " " + inc.Hostname)
	if query == "" {
		return
	}
	const simCutoff = 0.4 // 余弦距离阈值：> 0.4（相似度 < 60%）视为不够相似，丢弃
	var lines []string
	if emb := embedText(cfg, query); len(emb) > 0 {
		// 相似历史诊断案例（已按用户 👍/👎 反馈重排，👎 的会被压低甚至挤出）
		if cases, err := s.pg.searchSimilarCases(emb, 4); err == nil {
			n := 0
			for _, c := range cases {
				if c.IncidentID == inc.ID || c.Distance > simCutoff {
					continue // 跳过自身与不够相似的
				}
				fb := ""
				if c.Feedback == "helpful" {
					fb = " 👍"
				} else if c.Feedback == "unhelpful" {
					fb = " 👎"
				}
				lines = append(lines, fmt.Sprintf("• 相似度%d%% · 事件#%d：%s%s",
					int((1-c.Distance)*100), c.IncidentID, trimLine(c.Summary, 160), fb))
				if n++; n >= 3 {
					break
				}
			}
		}
		// 已验证的解决经验（kind=resolution，来自事件解决闭环沉淀）
		if hits, err := s.pg.searchMemoryByKind(emb, "resolution", 2); err == nil {
			for _, h := range hits {
				if h.Distance <= simCutoff {
					lines = append(lines, "• 处置经验："+trimLine(h.Content, 160))
				}
			}
		}
	}
	// 已有的自动修复规则（含草稿）——提示可直接启用/复用，串起"诊断→自动修复"闭环
	for _, r := range s.matchingRemediationRules(inc) {
		state := "已启用，命中即触发"
		if !r.Enabled {
			state = "草稿·未启用，审核后可启用"
		}
		lines = append(lines, fmt.Sprintf("• 已有自动修复规则「%s」（%s）", r.Name, state))
	}
	if len(lines) == 0 {
		return
	}
	s.incidents.AddEvent(inc.ID, "correlation", "AI",
		"🔗 相似历史事件与处置参考（RAG 自动召回，仅供参考）\n"+strings.Join(lines, "\n"))
	s.store.MarkDirty()
}

// matchingRemediationRules 返回可能适用于该事件的自动修复规则（按告警类型/最低级别粗匹配，含草稿），
// 最多 3 条。用于事件自动串联时提示"已有规则可复用"。
func (s *Server) matchingRemediationRules(inc Incident) []RemediationRule {
	var out []RemediationRule
	for _, r := range s.cfg.RemediationRules() {
		if len(r.MatchTypes) > 0 {
			hit := false
			for _, t := range r.MatchTypes {
				if t == inc.Type {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		if r.MinLevel != "" && levelRank(inc.Severity) < levelRank(r.MinLevel) {
			continue
		}
		if out = append(out, r); len(out) >= 3 {
			break
		}
	}
	return out
}
