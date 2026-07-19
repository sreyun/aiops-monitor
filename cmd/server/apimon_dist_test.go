package main

import (
	"testing"

	"aiops-monitor/shared"
)

// TestDistScope 验证故障范围判定：全通=ok、全挂=global、部分挂=regional。
func TestDistScope(t *testing.T) {
	cases := []struct {
		total, ok int
		want      string
	}{
		{0, 0, "ok"}, {3, 3, "ok"}, {3, 0, "global"}, {3, 1, "regional"}, {2, 1, "regional"},
	}
	for _, c := range cases {
		if got := distScope(c.total, c.ok); got != c.want {
			t.Errorf("distScope(%d,%d)=%q want %q", c.total, c.ok, got, c.want)
		}
	}
}

// TestDistProbeManager 验证多点结果归集、聚合、过期剔除与范围转换。
func TestDistProbeManager(t *testing.T) {
	m := newDistProbeManager()
	now := int64(1000)
	m.ingest("h1", "北京", []shared.ProbeResult{{TaskID: "ep1", OK: true, Ts: now}})
	m.ingest("h2", "上海", []shared.ProbeResult{{TaskID: "ep1", OK: false, Ts: now}})
	agg := m.aggregate("ep1", now)
	if agg.Total != 2 || agg.OKCount != 1 || agg.Scope != "regional" {
		t.Fatalf("应 2点/1正常/regional，实际 %+v", agg)
	}
	// 过期点（超过 ttl 600s）应剔除
	if old := m.aggregate("ep1", now+700); old.Total != 0 || old.Scope != "ok" {
		t.Fatalf("过期点应剔除，实际 %+v", old)
	}
	// 范围转换：首次变化 true，相同 false，再变化 true
	if !m.scopeTransition("ep1", "regional") {
		t.Error("首次应判为变化")
	}
	if m.scopeTransition("ep1", "regional") {
		t.Error("相同范围不应重复告警")
	}
	if !m.scopeTransition("ep1", "global") {
		t.Error("范围变化应判为变化")
	}
}
