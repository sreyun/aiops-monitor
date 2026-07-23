package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ResolutionCard 是运维结案经验的结构化卡片，写入 kind=resolution 记忆后供故障排查 RAG 召回。
// 字段尽量短、可检索；缺失项允许为空。
type ResolutionCard struct {
	IncidentID int64    `json:"incident_id"`
	Title      string   `json:"title"`
	AlertType  string   `json:"alert_type,omitempty"`
	Severity   string   `json:"severity,omitempty"`
	HostID     string   `json:"host_id,omitempty"`
	Hostname   string   `json:"hostname,omitempty"`
	Symptom    string   `json:"symptom,omitempty"`    // 现象
	Impact     string   `json:"impact,omitempty"`     // 影响面
	RootCause  string   `json:"root_cause,omitempty"` // 根因
	Steps      []string `json:"steps,omitempty"`      // 排查步骤
	Actions    []string `json:"actions,omitempty"`    // 处置动作
	Verify     string   `json:"verify,omitempty"`     // 验证方式
	Note       string   `json:"note,omitempty"`       // 人工解决说明
	Status     string   `json:"status"`               // verified|draft
}

// formatResolutionCard 把卡片渲染为可向量化、可人工阅读的检索文本。
func formatResolutionCard(c ResolutionCard) string {
	var b strings.Builder
	status := c.Status
	if status == "" {
		status = "verified"
	}
	fmt.Fprintf(&b, "【结案经验·%s】事件#%d %s\n", status, c.IncidentID, strings.TrimSpace(c.Title))
	var tags []string
	if c.AlertType != "" {
		tags = append(tags, "类型:"+c.AlertType)
	}
	if c.Severity != "" {
		tags = append(tags, "级别:"+c.Severity)
	}
	if c.Hostname != "" {
		tags = append(tags, "主机:"+c.Hostname)
	} else if c.HostID != "" {
		tags = append(tags, "主机:"+c.HostID)
	}
	if len(tags) > 0 {
		b.WriteString("标签：" + strings.Join(tags, " · ") + "\n")
	}
	writeCardField(&b, "现象", c.Symptom)
	writeCardField(&b, "影响面", c.Impact)
	writeCardField(&b, "根因", c.RootCause)
	writeCardList(&b, "排查步骤", c.Steps)
	writeCardList(&b, "处置动作", c.Actions)
	writeCardField(&b, "验证", c.Verify)
	writeCardField(&b, "解决说明", c.Note)
	return strings.TrimSpace(b.String())
}

func writeCardField(b *strings.Builder, label, v string) {
	v = strings.TrimSpace(v)
	if v == "" {
		return
	}
	fmt.Fprintf(b, "%s：%s\n", label, v)
}

func writeCardList(b *strings.Builder, label string, items []string) {
	var cleaned []string
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it != "" {
			cleaned = append(cleaned, it)
		}
	}
	if len(cleaned) == 0 {
		return
	}
	b.WriteString(label + "：\n")
	for i, it := range cleaned {
		fmt.Fprintf(b, "%d. %s\n", i+1, it)
	}
}

// buildResolutionCardFromIncident 用事件时间线 + 解决说明拼一张「模板结案卡」（不依赖 LLM）。
func buildResolutionCardFromIncident(inc Incident, note string) ResolutionCard {
	diag := latestTimelineText(inc, "ai_diagnosis")
	rem := latestTimelineText(inc, "remediation")
	card := ResolutionCard{
		IncidentID: inc.ID,
		Title:      inc.Title,
		AlertType:  inc.Type,
		Severity:   inc.Severity,
		HostID:     inc.HostID,
		Hostname:   inc.Hostname,
		Symptom:    inc.Title,
		Note:       strings.TrimSpace(note),
		Status:     "verified",
	}
	if diag != "" {
		card.RootCause = extractSection(diag, []string{"根因研判", "根因", "Root cause"}, 400)
		if card.RootCause == "" {
			card.RootCause = trimLine(diag, 400)
		}
		card.Steps = splitNumberedLines(extractSection(diag, []string{"关键证据", "证据", "Evidence"}, 600))
		card.Actions = splitNumberedLines(extractSection(diag, []string{"处置建议", "处置", "建议", "Actions"}, 600))
		if len(card.Actions) == 0 {
			card.Actions = splitNumberedLines(extractSection(diag, []string{"处置建议"}, 600))
		}
	}
	if rem != "" {
		card.Actions = append(card.Actions, trimLine(rem, 240))
	}
	if card.Note != "" && card.Verify == "" {
		card.Verify = "人工确认已解决：" + trimLine(card.Note, 160)
	}
	if card.Verify == "" {
		card.Verify = "告警恢复 / 事件状态变为已解决"
	}
	return card
}

func latestTimelineText(inc Incident, kind string) string {
	for i := len(inc.Timeline) - 1; i >= 0; i-- {
		if inc.Timeline[i].Kind == kind && strings.TrimSpace(inc.Timeline[i].Text) != "" {
			return inc.Timeline[i].Text
		}
	}
	return ""
}

// extractSection 从 Markdown/纯文本中抓取标题下的一节内容。
func extractSection(doc string, titles []string, maxRunes int) string {
	lines := strings.Split(doc, "\n")
	start := -1
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		trim = strings.TrimLeft(trim, "#* \t")
		for _, t := range titles {
			if strings.Contains(trim, t) {
				start = i + 1
				break
			}
		}
		if start >= 0 {
			break
		}
	}
	if start < 0 {
		return ""
	}
	var body []string
	for i := start; i < len(lines); i++ {
		trim := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trim, "## ") || strings.HasPrefix(trim, "# ") {
			break
		}
		// emoji heading lines like "## 🎯 根因"
		if strings.HasPrefix(trim, "##") {
			break
		}
		body = append(body, lines[i])
	}
	out := strings.TrimSpace(strings.Join(body, "\n"))
	if maxRunes > 0 {
		rs := []rune(out)
		if len(rs) > maxRunes {
			out = string(rs[:maxRunes]) + "…"
		}
	}
	return out
}

func splitNumberedLines(block string) []string {
	block = strings.TrimSpace(block)
	if block == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "-*• \t")
		line = stripListIndexPrefix(line)
		if line != "" {
			out = append(out, line)
		}
		if len(out) >= 8 {
			break
		}
	}
	return out
}

// stripListIndexPrefix removes leading "1." / "1、" / "1)" style markers.
func stripListIndexPrefix(line string) string {
	i := 0
	rs := []rune(line)
	for i < len(rs) && rs[i] >= '0' && rs[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(rs) {
		return strings.TrimSpace(line)
	}
	switch rs[i] {
	case '.', '、', ')', '）':
		i++
		for i < len(rs) && (rs[i] == ' ' || rs[i] == '\t') {
			i++
		}
		return string(rs[i:])
	default:
		return strings.TrimSpace(line)
	}
}

// enrichResolutionCardWithAI 尝试用 LLM 把结案卡补全为更干净的结构；失败则返回原卡。
func (s *Server) enrichResolutionCardWithAI(card ResolutionCard, diag string) ResolutionCard {
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.APIKey == "" {
		return card
	}
	payload, _ := json.Marshal(map[string]any{
		"title":     card.Title,
		"type":      card.AlertType,
		"severity":  card.Severity,
		"hostname":  card.Hostname,
		"note":      card.Note,
		"diagnosis": trimLine(diag, 2500),
	})
	msgs := []map[string]string{
		{
			"role": "system",
			"content": "你是 SRE 知识管理员。根据输入把故障结案经验整理成 JSON，只输出 JSON 对象，不要 markdown。" +
				`字段：symptom,impact,root_cause,steps(数组),actions(数组),verify。中文，简洁可执行。未知字段用空字符串或空数组。`,
		},
		{"role": "user", "content": string(payload)},
	}
	raw, err := aiChat(cfg, msgs)
	if err != nil || strings.TrimSpace(raw) == "" {
		return card
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	var parsed struct {
		Symptom   string   `json:"symptom"`
		Impact    string   `json:"impact"`
		RootCause string   `json:"root_cause"`
		Steps     []string `json:"steps"`
		Actions   []string `json:"actions"`
		Verify    string   `json:"verify"`
	}
	if json.Unmarshal([]byte(raw), &parsed) != nil {
		return card
	}
	if v := strings.TrimSpace(parsed.Symptom); v != "" {
		card.Symptom = v
	}
	if v := strings.TrimSpace(parsed.Impact); v != "" {
		card.Impact = v
	}
	if v := strings.TrimSpace(parsed.RootCause); v != "" {
		card.RootCause = v
	}
	if len(parsed.Steps) > 0 {
		card.Steps = parsed.Steps
	}
	if len(parsed.Actions) > 0 {
		card.Actions = parsed.Actions
	}
	if v := strings.TrimSpace(parsed.Verify); v != "" {
		card.Verify = v
	}
	return card
}
