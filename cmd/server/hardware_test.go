package main

import "testing"

// hardwareHealthScore must NOT report a score for empty/unknown health. The
// original code used a bare `switch` into a zero-valued `var score float64`, so a
// failed poll (Health == "") silently wrote 0 — which the series means as
// "Critical" — producing a phantom critical alert for a host that was merely
// unreachable.
func TestHardwareHealthScore(t *testing.T) {
	cases := []struct {
		health string
		want   float64
		wantOK bool
	}{
		{"OK", 2, true},
		{"Warning", 1, true},
		{"Critical", 0, true},
		{"", 0, false},        // 采集失败 → 不写指标，而不是当成 Critical
		{"Unknown", 0, false}, // 未知取值同理
		{"ok", 0, false},      // Redfish 健康值大小写敏感，不猜
	}
	for _, c := range cases {
		got, ok := hardwareHealthScore(c.health)
		if ok != c.wantOK {
			t.Errorf("hardwareHealthScore(%q) ok = %v, want %v", c.health, ok, c.wantOK)
		}
		if ok && got != c.want {
			t.Errorf("hardwareHealthScore(%q) = %v, want %v", c.health, got, c.want)
		}
	}
}
