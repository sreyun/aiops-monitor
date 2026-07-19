package main

import (
	"strings"
	"testing"
	"time"
)

// TestCheckTimeout 验证 TCP/UDP/DNS 拨测的超时取值：优先 TimeoutSec，否则默认。
func TestCheckTimeout(t *testing.T) {
	if checkTimeout(CustomCheck{TimeoutSec: 3}, 5*time.Second) != 3*time.Second {
		t.Error("应采用 TimeoutSec")
	}
	if checkTimeout(CustomCheck{}, 5*time.Second) != 5*time.Second {
		t.Error("未设 TimeoutSec 应用默认 5s")
	}
}

// TestDNSThreshold 验证 DNS 解析延迟阈值：配置→内部映射、默认档非零、backfill 补默认（含 UDP 回归）。
func TestDNSThreshold(t *testing.T) {
	th := ThresholdConfig{CheckDNSTimeoutWarn: 300, CheckDNSTimeoutCrit: 1500}.toThresholds()
	if th.CheckDNSTimeoutWarn != 300 || th.CheckDNSTimeoutCrit != 1500 {
		t.Fatalf("DNS 阈值映射失败: warn=%v crit=%v", th.CheckDNSTimeoutWarn, th.CheckDNSTimeoutCrit)
	}
	if d := defaultThresholdConfig(); d.CheckDNSTimeoutWarn <= 0 || d.CheckDNSTimeoutCrit <= 0 {
		t.Fatal("默认档 DNS 阈值应非零")
	}
	var empty ThresholdConfig
	backfillThresholdDefaults(&empty)
	if empty.CheckDNSTimeoutWarn <= 0 || empty.CheckDNSTimeoutCrit <= 0 {
		t.Fatal("backfill 应补上 DNS 阈值默认")
	}
	if empty.CheckUDPTimeoutWarn <= 0 || empty.CheckUDPTimeoutCrit <= 0 {
		t.Fatal("backfill 应补上 UDP 阈值默认（回归此前遗漏）")
	}
}

// TestProbeDNS 验证 DNS 拨测：空目标/非法类型失败；localhost 离线可靠解析出 127.0.0.1；期望值断言。
func TestProbeDNS(t *testing.T) {
	cr := newCheckRunner(newTestConfigStore(t), NewStore(), nil, "")
	if ok, _ := cr.probeDNS(CustomCheck{Target: ""}, 5*time.Second); ok {
		t.Error("空域名应失败")
	}
	if ok, _ := cr.probeDNS(CustomCheck{Target: "example.com", DNSType: "ZZZ"}, 5*time.Second); ok {
		t.Error("非法记录类型应失败")
	}
	// localhost 的 A 记录解析在本机可靠（不依赖外网）
	ok, msg := cr.probeDNS(CustomCheck{Target: "localhost", DNSType: "A"}, 5*time.Second)
	if !ok || !strings.Contains(msg, "127.0.0.1") {
		t.Fatalf("localhost 应解析出 127.0.0.1, ok=%v msg=%s", ok, msg)
	}
	// 默认记录类型 = A（DNSType 留空）
	if ok, _ := cr.probeDNS(CustomCheck{Target: "localhost"}, 5*time.Second); !ok {
		t.Error("默认 A 记录应解析成功")
	}
	// 期望值不匹配应失败
	if ok, _ := cr.probeDNS(CustomCheck{Target: "localhost", DNSType: "A", ExpectKeyword: "9.9.9.9"}, 5*time.Second); ok {
		t.Error("期望值不匹配应失败")
	}
	// 目标@服务器 的解析：语法正确（用 localhost 不指定外部服务器仍应成功）
	if ok, _ := cr.probeDNS(CustomCheck{Target: "localhost", DNSType: "A", ExpectKeyword: "127.0.0.1"}, 5*time.Second); !ok {
		t.Error("期望值匹配应成功")
	}
}
