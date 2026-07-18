package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// ============================================================================
// AI 技能库（自进化核心）
//
// 把散落的经验/诊断/解决记忆，定期提炼成命名化、带「触发条件 + 操作步骤」的可复用技能(SOP)，
// 检索后注入提示词，让 AI 直接复用被现实验证过的做法——这是比原始 RAG 更高阶的自我进化：
// 系统不只是「记住发生过什么」，而是「总结出该怎么做」，并在使用中不断强化 / 改写。
// 借鉴自 Nous Hermes Agent 的 skill 机制，落到本项目 Go + pgvector 的既有底座上。
// ============================================================================

const (
	skillDistillDupDist = 0.12 // 与既有技能语义距离 < 此值视为重复 → 覆盖改进而非新增
	skillRelevantDist   = 0.55 // 注入提示词时的相关性阈值，超过则不注入（避免噪声）
)

// distilledSkill 是 AI 提炼输出的单条技能。
type distilledSkill struct {
	Name    string `json:"name"`
	Trigger string `json:"trigger"`
	Steps   string `json:"steps"`
	Tags    string `json:"tags"`
}

// distillSkills 提炼主流程：取近期高价值经验 → AI 提炼成若干可复用技能 → 去重(覆盖改进)后入库。
// 由每日维护循环驱动，也可手动触发。返回新增技能数。
func (s *Server) distillSkills(lookbackDays int) (int, error) {
	if s.pg == nil {
		return 0, fmt.Errorf("PG 不可用")
	}
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.APIKey == "" {
		return 0, fmt.Errorf("AI 未配置")
	}
	if lookbackDays <= 0 {
		lookbackDays = 14
	}
	since := time.Now().Add(-time.Duration(lookbackDays) * 24 * time.Hour).Unix()
	mems := s.pg.memoriesForDistill(since, 40)
	if len(mems) < 3 { // 经验太少，不值得提炼
		return 0, nil
	}
	var corpus strings.Builder
	for i, m := range mems {
		fmt.Fprintf(&corpus, "[%d](%s) %s\n", i+1, m.Kind, trimLine(m.Content, 400))
	}
	sys := "你是资深 SRE 知识工程师。请从以下真实运维经验片段中，提炼出【可复用的运维技能(SOP)】。" +
		"每条技能要高度可复用、不绑定单次具体事件（把具体主机名/事件号抽象成通用条件）。" +
		"宁缺毋滥，只提炼有明确复用价值的，最多 6 条。" +
		"严格只输出一个 JSON 数组，每个元素为 {\"name\":\"技能名(简短祈使句)\",\"trigger\":\"何时适用(症状/场景)\"," +
		"\"steps\":\"分步操作(可含命令与判断)\",\"tags\":\"逗号分隔标签\"}；不要输出数组以外的任何文字。"
	out, err := aiComplete(cfg, sys, "运维经验片段：\n"+corpus.String())
	if err != nil {
		return 0, err
	}
	created := 0
	for _, sk := range parseDistilledSkills(out) {
		name, trigger, steps := strings.TrimSpace(sk.Name), strings.TrimSpace(sk.Trigger), strings.TrimSpace(sk.Steps)
		if name == "" || steps == "" {
			continue
		}
		emb := embedText(cfg, name+" "+trigger)
		if len(emb) == 0 {
			continue
		}
		if id, dup := s.pg.findSimilarSkill(emb, skillDistillDupDist); dup {
			// 已有相似技能 → 用新版覆盖（视为「用中改进」）并强化，不新增
			_ = s.pg.updateSkill(id, name, trigger, steps, emb)
			s.pg.recordSkillUse(id, true)
			continue
		}
		if _, err := s.pg.insertSkill(name, trigger, steps, sk.Tags, "distilled", emb); err == nil {
			created++
		}
	}
	if created > 0 {
		slog.Info("技能提炼完成", "新增技能", created, "候选经验", len(mems))
	}
	return created, nil
}

// parseDistilledSkills 容错解析 AI 输出的技能 JSON 数组（可能含代码块围栏或前后噪声）。
func parseDistilledSkills(text string) []distilledSkill {
	text = strings.TrimSpace(text)
	if i := strings.Index(text, "```"); i >= 0 { // 去代码块围栏
		text = text[i+3:]
		if j := strings.LastIndex(text, "```"); j >= 0 {
			text = text[:j]
		}
		if nl := strings.IndexByte(text, '\n'); nl >= 0 && nl < 12 { // 去掉 ```json 语言标记行
			text = text[nl+1:]
		}
	}
	l, r := strings.IndexByte(text, '['), strings.LastIndexByte(text, ']')
	if l < 0 || r <= l {
		return nil
	}
	var arr []distilledSkill
	if err := json.Unmarshal([]byte(text[l:r+1]), &arr); err != nil {
		return nil
	}
	return arr
}

// retrieveSkillsForPrompt 检索与当前任务最相关的技能，格式化为可注入提示词的文本，
// 同时异步记录一次使用（use_count++）。无技能 / 无相关匹配时返回空串。
func (s *Server) retrieveSkillsForPrompt(query string, topK int) string {
	if s.pg == nil {
		return ""
	}
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.APIKey == "" || strings.TrimSpace(query) == "" {
		return ""
	}
	if topK <= 0 {
		topK = 4
	}
	emb := embedText(cfg, query)
	if len(emb) == 0 {
		return ""
	}
	skills, err := s.pg.searchSkills(emb, topK)
	if err != nil || len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n【已掌握技能（历史提炼的可复用 SOP，相关时优先套用）】\n")
	ids := make([]int64, 0, len(skills))
	for _, sk := range skills {
		if sk.Distance > skillRelevantDist { // 只注入足够相关的
			continue
		}
		fmt.Fprintf(&b, "- 【%s】适用：%s\n  步骤：%s\n", sk.Name, trimLine(sk.Trigger, 120), trimLine(sk.Steps, 500))
		ids = append(ids, sk.ID)
	}
	if len(ids) == 0 {
		return ""
	}
	go func() {
		for _, id := range ids {
			s.pg.recordSkillUse(id, false)
		}
	}()
	return b.String()
}

// reinforceSkill 在事件解决 / 建议被采纳时，语义定位并强化最相关技能——技能层的学习闭环。异步。
func (s *Server) reinforceSkill(text string, factor float64) {
	if s.pg == nil {
		return
	}
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.APIKey == "" || strings.TrimSpace(text) == "" {
		return
	}
	go func() {
		if emb := embedText(cfg, text); len(emb) > 0 {
			s.pg.boostSkillNearest(emb, factor)
		}
	}()
}
