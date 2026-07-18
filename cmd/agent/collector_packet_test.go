package main

import "testing"

// TestParseConntrackBytesPackets 锁定 flow 明细"字节/包始终为 0"的修复：此前 bytes/packets
// 的 case 只有注释、没有解析代码，即使 conntrack 行(开启 nf_conntrack_acct)带了值也被丢弃。
func TestParseConntrackBytesPackets(t *testing.T) {
	// 开启 acct 的典型行：orig + reply 两组方向元组各带一份 packets=/bytes=。
	line := "ipv4     2 tcp      6 431999 ESTABLISHED " +
		"src=10.0.0.1 dst=10.0.0.2 sport=443 dport=52341 packets=10 bytes=1500 " +
		"src=10.0.0.2 dst=10.0.0.1 sport=52341 dport=443 packets=8 bytes=1200 [ASSURED] mark=0 use=1"
	e, ok := parseConntrackLine(line)
	if !ok {
		t.Fatal("解析失败")
	}
	if e.bytes != 2700 {
		t.Errorf("bytes=%d, 期望 2700（双向 1500+1200 累加）", e.bytes)
	}
	if e.packets != 18 {
		t.Errorf("packets=%d, 期望 18（10+8）", e.packets)
	}
	if e.srcIP != "10.0.0.1" || e.dstIP != "10.0.0.2" || e.srcPort != 443 || e.dstPort != 52341 || e.proto != 6 {
		t.Errorf("五元组解析错: %+v", e)
	}

	// acct 关闭的行（无 packets=/bytes=）：五元组仍解析，流量为 0（合理降级，不 panic）。
	line2 := "ipv4     2 udp      17 29 src=10.0.0.5 dst=8.8.8.8 sport=1234 dport=53 [UNREPLIED] src=8.8.8.8 dst=10.0.0.5 sport=53 dport=1234 mark=0 use=1"
	e2, ok2 := parseConntrackLine(line2)
	if !ok2 || e2.srcIP != "10.0.0.5" || e2.dstPort != 53 || e2.proto != 17 {
		t.Errorf("无 acct 行五元组解析错: %+v", e2)
	}
	if e2.bytes != 0 || e2.packets != 0 {
		t.Errorf("无 acct 行流量应为 0, 得 bytes=%d packets=%d", e2.bytes, e2.packets)
	}
}
