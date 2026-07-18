package main

import (
	"net"
	"testing"
)

// TestFlowEnrichPrivateSkip 锁定隐私关键行为：内网/保留地址必须被判为 private，
// enrichOne 会在做任何外部 DNS 查询【之前】就早退——内部 IP 绝不外发到 Cymru/DNS。
func TestFlowEnrichPrivateSkip(t *testing.T) {
	priv := []string{
		"10.0.0.5", "192.168.1.1", "172.16.0.1", "172.31.255.1",
		"127.0.0.1", "169.254.1.1", "100.64.0.1", "100.127.0.1",
		"::1", "fe80::1", "0.0.0.0", "224.0.0.1",
	}
	for _, s := range priv {
		if !isPrivateOrReserved(net.ParseIP(s)) {
			t.Errorf("%s 应判为内网/保留（不得外发富化）", s)
		}
	}
	pub := []string{"8.8.8.8", "1.1.1.1", "142.250.1.1", "223.5.5.5"}
	for _, s := range pub {
		if isPrivateOrReserved(net.ParseIP(s)) {
			t.Errorf("%s 应判为公网", s)
		}
	}
	// nil / 空 IP 也当作跳过，避免对垃圾数据发查询。
	if !isPrivateOrReserved(nil) {
		t.Error("nil IP 应判为跳过")
	}
	if got := reverseIPv4(net.ParseIP("8.8.4.4").To4()); got != "4.4.8.8" {
		t.Errorf("reverseIPv4 = %s, 期望 4.4.8.8", got)
	}
}
