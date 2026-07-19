package main

import (
	"strings"
	"testing"
)

// TestSLOPromqlGoodTotal 验证 promql 源的 good/total 计算：$window 替换、good/total 取数、
// 查询失败归零（不误报）、good 超 total 时钳制。
func TestSLOPromqlGoodTotal(t *testing.T) {
	m := newSLOManager(nil)
	s := SLO{
		SourceType: "promql",
		GoodQuery:  `sum(increase(http_total{code!~"5.."}[$window]))`,
		TotalQuery: `sum(increase(http_total[$window]))`,
		Target:     99,
	}

	// 正常：total=1000，good=990（good 查询含 code），并断言 $window 已被替换为具体窗口。
	m.promScalar = func(q string) (float64, bool) {
		if strings.Contains(q, "$window") {
			t.Fatalf("$window 未被替换: %s", q)
		}
		if strings.Contains(q, "code") {
			return 990, true
		}
		return 1000, true
	}
	if good, total := m.goodTotal(s, 10000, 10000-300); total != 1000 || good != 990 {
		t.Fatalf("promql good/total 计算错误: good=%d total=%d", good, total)
	}

	// 查询失败 → 归零，避免把无数据误判为全坏。
	m.promScalar = func(q string) (float64, bool) { return 0, false }
	if g, tt := m.goodTotal(s, 10000, 9700); g != 0 || tt != 0 {
		t.Fatalf("查询失败应归零: good=%d total=%d", g, tt)
	}

	// good > total 时应钳制到 total。
	m.promScalar = func(q string) (float64, bool) {
		if strings.Contains(q, "code") {
			return 1200, true
		}
		return 1000, true
	}
	if g, tt := m.goodTotal(s, 10000, 9700); g != tt || tt != 1000 {
		t.Fatalf("good 应被钳制到 total: good=%d total=%d", g, tt)
	}
}
