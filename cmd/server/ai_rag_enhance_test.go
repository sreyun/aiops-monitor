package main

import (
	"strings"
	"testing"
)

func TestMemoryKindLabel(t *testing.T) {
	if memoryKindLabel("resolution") != "结案经验" {
		t.Fatal(memoryKindLabel("resolution"))
	}
	if memoryKindLabel("knowledge") != "已验证文档引用" {
		t.Fatal(memoryKindLabel("knowledge"))
	}
	if memoryKindLabel("pitfall") != "避坑·差评" {
		t.Fatal(memoryKindLabel("pitfall"))
	}
}

func TestExtractDocTitlesFromText(t *testing.T) {
	raw := "WeKnora 知识库检索结果：\n  1. 【运维手册】 相关度 0.91\n     内容…\n  2. 【FAQ】\n"
	got := extractDocTitlesFromText(raw)
	if len(got) < 2 || got[0] != "运维手册" || got[1] != "FAQ" {
		t.Fatalf("got=%v", got)
	}
}

func TestWeKnoraDegradedTip(t *testing.T) {
	weknoraFailAt.Store(0)
	weknoraOKAt.Store(0)
	weknoraLastErr.Store("")
	if weknoraDegradedTip() != "" {
		t.Fatal("want empty")
	}
	markWeKnoraFail(errString("connection refused"))
	tip := weknoraDegradedTip()
	if tip == "" || !strings.Contains(tip, "不可用") {
		t.Fatalf("tip=%q", tip)
	}
	markWeKnoraOK()
	if weknoraDegradedTip() != "" {
		t.Fatal("cleared after OK")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestDiagnosisOrchestrationHint(t *testing.T) {
	h := diagnosisOrchestrationHint()
	for _, want := range []string{"排查编排", "WeKnora", "现场"} {
		if !strings.Contains(h, want) {
			t.Fatalf("missing %q in %s", want, h)
		}
	}
}

func TestPersistAdoptedKnowledgeSkippedEmpty(t *testing.T) {
	s := &Server{}
	// no panic when pg/memory nil
	s.persistAdoptedKnowledge("", "", "x", nil)
	s.rememberPitfall("q", "a", "", "x") // empty reason → no-op
}
