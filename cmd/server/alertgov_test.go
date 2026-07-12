package main

import (
	"testing"
	"time"
)

func TestAlertMatch(t *testing.T) {
	a := Alert{Hostname: "web-prod-01", IP: "10.0.0.5", Type: "cpu", Level: "critical"}
	cases := []struct {
		name string
		m    AlertMatch
		want bool
	}{
		{"空=全匹配", AlertMatch{}, true},
		{"主机子串命中", AlertMatch{HostPattern: "web-prod"}, true},
		{"主机 IP 命中", AlertMatch{HostPattern: "10.0.0"}, true},
		{"主机不命中", AlertMatch{HostPattern: "db-"}, false},
		{"类型命中", AlertMatch{Types: []string{"mem", "cpu"}}, true},
		{"类型不命中", AlertMatch{Types: []string{"disk"}}, false},
		{"级别命中", AlertMatch{Levels: []string{"critical"}}, true},
		{"级别不命中", AlertMatch{Levels: []string{"warning"}}, false},
		{"多条件与", AlertMatch{HostPattern: "web", Types: []string{"cpu"}, Levels: []string{"critical"}}, true},
		{"多条件其一不满足", AlertMatch{HostPattern: "web", Levels: []string{"warning"}}, false},
	}
	for _, c := range cases {
		if got := c.m.matches(a); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestSilenceTimeWindow(t *testing.T) {
	// 夜间静默 22:00-08:00（跨天）
	r := SilenceRule{Enabled: true, TimeStart: "22:00", TimeEnd: "08:00"}
	at := func(h, m int) time.Time { return time.Date(2026, 7, 12, h, m, 0, 0, time.UTC) }
	if !r.activeNow(at(23, 0)) {
		t.Error("23:00 应在夜间窗口内")
	}
	if !r.activeNow(at(2, 30)) {
		t.Error("02:30 应在夜间窗口内")
	}
	if r.activeNow(at(12, 0)) {
		t.Error("12:00 不应在夜间窗口内")
	}
	if r.activeNow(at(8, 0)) {
		t.Error("08:00 为窗口右开边界，不应命中")
	}
	// 同日窗口 09:00-18:00
	r2 := SilenceRule{Enabled: true, TimeStart: "09:00", TimeEnd: "18:00"}
	if !r2.activeNow(at(10, 0)) || r2.activeNow(at(20, 0)) {
		t.Error("同日窗口判断错误")
	}
	// 无时段=全天
	if !(SilenceRule{Enabled: true}).activeNow(at(3, 0)) {
		t.Error("无时段应全天生效")
	}
	// 星期限定：2026-07-12 是周日(0)
	rWd := SilenceRule{Enabled: true, Weekdays: []int{1, 2}} // 周一/周二
	if rWd.activeNow(at(3, 0)) {
		t.Error("周日不在 周一/周二 列表，不应命中")
	}
}

func TestGovSilenced(t *testing.T) {
	g := AlertGovernance{SilenceRules: []SilenceRule{
		{Name: "静默测试机", Enabled: true, Match: AlertMatch{HostPattern: "test-"}},
		{Name: "已禁用", Enabled: false, Match: AlertMatch{}},
	}}
	now := time.Now()
	if ok, rule := govSilenced(g, Alert{Hostname: "test-01", Type: "cpu"}, now); !ok || rule != "静默测试机" {
		t.Fatalf("应被静默规则命中，got ok=%v rule=%q", ok, rule)
	}
	if ok, _ := govSilenced(g, Alert{Hostname: "prod-01"}, now); ok {
		t.Fatal("prod 主机不应被静默")
	}
}

func TestGovInhibited(t *testing.T) {
	// 主机离线 → 抑制同主机的 CPU/内存告警
	g := AlertGovernance{InhibitRules: []InhibitRule{{
		Name: "离线抑制指标", Enabled: true, SameHost: true,
		Source: AlertMatch{Types: []string{"offline"}},
		Target: AlertMatch{Types: []string{"cpu", "memory"}},
	}}}
	active := []Alert{{HostID: "h1", Type: "offline", Level: "critical"}}
	cpu := Alert{HostID: "h1", Type: "cpu", Level: "warning"}
	if ok, rule := govInhibited(g, cpu, active); !ok || rule != "离线抑制指标" {
		t.Fatalf("同主机 CPU 应被离线告警抑制，got ok=%v rule=%q", ok, rule)
	}
	// 不同主机不抑制
	cpuOther := Alert{HostID: "h2", Type: "cpu"}
	if ok, _ := govInhibited(g, cpuOther, active); ok {
		t.Fatal("不同主机不应被抑制")
	}
	// 源不活跃则不抑制
	if ok, _ := govInhibited(g, cpu, []Alert{{HostID: "h1", Type: "disk"}}); ok {
		t.Fatal("无离线源时不应抑制")
	}
	// 不被自己抑制（避免 target 匹配到自身活跃项）
	self := Alert{HostID: "h1", Type: "offline"}
	if ok, _ := govInhibited(AlertGovernance{InhibitRules: []InhibitRule{{
		Enabled: true, SameHost: true, Name: "x",
		Source: AlertMatch{Types: []string{"offline"}}, Target: AlertMatch{Types: []string{"offline"}},
	}}}, self, []Alert{self}); ok {
		t.Fatal("告警不应抑制自身")
	}
}

func TestGovRouteChannels(t *testing.T) {
	g := AlertGovernance{Routes: []NotifyRoute{
		{Name: "严重全发", Enabled: true, Match: AlertMatch{Levels: []string{"critical"}}, Channels: []string{"feishu", "dingtalk", "email"}},
		{Name: "警告仅飞书", Enabled: true, Match: AlertMatch{Levels: []string{"warning"}}, Channels: []string{"feishu"}},
	}}
	sel, routed := govRouteChannels(g, Alert{Level: "warning"})
	if !routed || !sel["feishu"] || sel["dingtalk"] {
		t.Fatalf("warning 应仅路由到飞书，got %v routed=%v", sel, routed)
	}
	sel2, routed2 := govRouteChannels(g, Alert{Level: "critical"})
	if !routed2 || !sel2["feishu"] || !sel2["dingtalk"] || !sel2["email"] {
		t.Fatalf("critical 应路由到三渠道，got %v", sel2)
	}
	// 无路由命中 → routed=false（调用方回退全部渠道）
	_, routed3 := govRouteChannels(AlertGovernance{}, Alert{Level: "critical"})
	if routed3 {
		t.Fatal("无路由配置时 routed 应为 false")
	}
}
