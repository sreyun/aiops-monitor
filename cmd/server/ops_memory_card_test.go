package main

import (
	"strings"
	"testing"
)

func TestFormatResolutionCard(t *testing.T) {
	card := ResolutionCard{
		IncidentID: 42,
		Title:      "CPU 持续过高",
		AlertType:  "cpu",
		Severity:   "critical",
		Hostname:   "web-01",
		Symptom:    "CPU 使用率 > 90%",
		Impact:     "接口超时",
		RootCause:  "失控定时任务",
		Steps:      []string{"查看 top", "核对 crontab"},
		Actions:    []string{"停用异常任务", "限流"},
		Verify:     "CPU 回落至 40%",
		Note:       "运维手工处理",
		Status:     "verified",
	}
	out := formatResolutionCard(card)
	for _, want := range []string{
		"【结案经验·verified】事件#42",
		"类型:cpu",
		"主机:web-01",
		"现象：CPU 使用率 > 90%",
		"根因：失控定时任务",
		"排查步骤：",
		"1. 查看 top",
		"处置动作：",
		"验证：CPU 回落至 40%",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("format missing %q\n---\n%s", want, out)
		}
	}
}

func TestBuildResolutionCardFromIncident(t *testing.T) {
	inc := Incident{
		ID:       7,
		Title:    "磁盘将满",
		Type:     "disk",
		Severity: "warning",
		Hostname: "db-1",
		Timeline: []IncidentEvent{
			{Kind: "ai_diagnosis", Text: "## 🎯 根因研判\n日志目录未轮转。\n\n## 🔍 关键证据\n1. /var/log 占用 90%\n2. logrotate 停用\n\n## 🛠️ 处置建议\n1. 清理旧日志\n2. 恢复 logrotate\n"},
		},
	}
	card := buildResolutionCardFromIncident(inc, "已清理并恢复轮转")
	if card.IncidentID != 7 || card.AlertType != "disk" {
		t.Fatalf("basic fields: %+v", card)
	}
	if !strings.Contains(card.RootCause, "日志目录") && !strings.Contains(card.RootCause, "轮转") {
		t.Fatalf("root cause not extracted: %q", card.RootCause)
	}
	if len(card.Actions) == 0 {
		t.Fatalf("actions empty: %+v", card)
	}
	if card.Note != "已清理并恢复轮转" {
		t.Fatalf("note=%q", card.Note)
	}
	text := formatResolutionCard(card)
	if !strings.Contains(text, "【结案经验") {
		t.Fatalf("bad text: %s", text)
	}
}

func TestResolutionNoteFromIncident(t *testing.T) {
	inc := Incident{Timeline: []IncidentEvent{
		{Kind: "note", Text: "解决说明：扩容磁盘"},
		{Kind: "resolved", Text: ""},
	}}
	if got := resolutionNoteFromIncident(inc); got != "扩容磁盘" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractSection(t *testing.T) {
	doc := "## 🎯 根因研判\nAAA\n\n## 🛠️ 处置建议\n1. BBB\n2. CCC\n"
	got := extractSection(doc, []string{"根因研判"}, 100)
	if !strings.Contains(got, "AAA") {
		t.Fatalf("got %q", got)
	}
	acts := splitNumberedLines(extractSection(doc, []string{"处置建议"}, 200))
	if len(acts) < 2 || acts[0] != "BBB" {
		t.Fatalf("acts=%v", acts)
	}
}
