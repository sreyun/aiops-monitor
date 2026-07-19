package main

import (
	"path/filepath"
	"testing"
)

// TestPromFingerprint 验证标签指纹与顺序无关、区分不同标签集。
func TestPromFingerprint(t *testing.T) {
	a := promFingerprint(map[string]string{"job": "mysql", "instance": "db1"})
	b := promFingerprint(map[string]string{"instance": "db1", "job": "mysql"})
	if a != b {
		t.Fatal("同标签集应同指纹（与顺序无关）")
	}
	if a == promFingerprint(map[string]string{"job": "mysql"}) {
		t.Fatal("不同标签集应不同指纹")
	}
}

// TestRenderRuleMessage 验证告警文案模板：{{label}}/{{value}} 替换 + 空模板默认。
func TestRenderRuleMessage(t *testing.T) {
	r := PromRule{Name: "JVM堆高", Message: "{{instance}} 堆使用率 {{value}}"}
	if got := renderRuleMessage(r, map[string]string{"instance": "app1"}, 0.95); got != "app1 堆使用率 0.95" {
		t.Fatalf("模板渲染错误: %q", got)
	}
	if renderRuleMessage(PromRule{Name: "X"}, map[string]string{"a": "b"}, 1) == "" {
		t.Error("空模板应给默认文案")
	}
}

// TestPromRulePersistence 验证规则落盘。
func TestPromRulePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg.json")
	cs, err := NewConfigStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := cs.UpsertPromRule(PromRule{Name: "mysql down", Expr: "mysql_up == 0", ForSec: 60, Level: "critical", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	cs2, _ := NewConfigStore(path, nil)
	got := cs2.PromRules()
	if len(got) != 1 || got[0].ID != saved.ID || got[0].Expr != "mysql_up == 0" || got[0].ForSec != 60 || got[0].Level != "critical" {
		t.Fatalf("规则未落盘: %+v", got)
	}
}
