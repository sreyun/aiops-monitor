package main

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestSSRFBlockedIP(t *testing.T) {
	cases := []struct {
		ip      string
		strict  bool
		blocked bool
		desc    string
	}{
		{"169.254.169.254", false, true, "AWS/GCP/Azure 元数据"},
		{"169.254.170.2", false, true, "AWS ECS 元数据"},
		{"100.100.100.200", false, true, "阿里云元数据"},
		{"fd00:ec2::254", false, true, "AWS IPv6 元数据"},
		{"169.254.1.23", false, true, "链路本地"},
		{"8.8.8.8", false, false, "公网默认放行"},
		{"1.1.1.1", true, false, "公网严格模式也放行"},
		{"10.0.0.5", false, false, "内网默认放行（保留内网监控/自建 LLM）"},
		{"192.168.1.10", false, false, "内网默认放行"},
		{"127.0.0.1", false, false, "环回默认放行"},
		{"10.0.0.5", true, true, "内网严格模式拒绝"},
		{"192.168.1.10", true, true, "内网严格模式拒绝"},
		{"127.0.0.1", true, true, "环回严格模式拒绝"},
		{"172.16.5.5", true, true, "RFC1918 严格模式拒绝"},
	}
	for _, c := range cases {
		got, why := ssrfBlockedIP(net.ParseIP(c.ip), c.strict)
		if got != c.blocked {
			t.Errorf("%s: ssrfBlockedIP(%s, strict=%v)=%v want %v (why=%q)", c.desc, c.ip, c.strict, got, c.blocked, why)
		}
	}
	if b, _ := ssrfBlockedIP(nil, false); !b {
		t.Error("无法解析的 IP(nil) 应被拒绝")
	}
}

// TestGuardedClientBlocksMetadata 验证带防护的 client 在 connect 前就拒绝元数据地址（快速失败，不挂起）。
func TestGuardedClientBlocksMetadata(t *testing.T) {
	c := newGuardedHTTPClient(2 * time.Second)
	_, err := c.Get("http://169.254.169.254/latest/meta-data/iam/")
	if err == nil {
		t.Fatal("应拒绝连接云元数据地址")
	}
	if !strings.Contains(err.Error(), "SSRF") {
		t.Errorf("错误信息应含 SSRF 提示，实际：%v", err)
	}
}
