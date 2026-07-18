package main

import "testing"

var (
	oidSysUpTime0  = []uint32{1, 3, 6, 1, 2, 1, 1, 3, 0}
	oidSnmpTrapOID = []uint32{1, 3, 6, 1, 6, 3, 1, 1, 4, 1, 0}
	oidLinkDown    = []uint32{1, 3, 6, 1, 6, 3, 1, 1, 5, 3}
	oidIfIndex1    = []uint32{1, 3, 6, 1, 2, 1, 2, 2, 1, 1, 2}
)

func TestNormalizeV1TrapOID(t *testing.T) {
	// generic 0..5 → 标准 trapOID
	if got := normalizeV1TrapOID("1.3.6.1.4.1.9", 2, 0); got != "1.3.6.1.6.3.1.1.5.3" {
		t.Errorf("linkDown(generic=2) → %s", got)
	}
	if got := normalizeV1TrapOID("1.3.6.1.4.1.9", 0, 0); got != "1.3.6.1.6.3.1.1.5.1" {
		t.Errorf("coldStart(generic=0) → %s", got)
	}
	// generic=6 enterpriseSpecific → enterprise.0.specific
	if got := normalizeV1TrapOID("1.3.6.1.4.1.9.9.41", 6, 5); got != "1.3.6.1.4.1.9.9.41.0.5" {
		t.Errorf("enterpriseSpecific → %s", got)
	}
}

func TestParseV2Trap(t *testing.T) {
	vbList := encodeTLV(tagSequence, concat(
		encodeVarbind(oidSysUpTime0, encodeUnsigned(tagTimeTicks, 12345)),
		encodeVarbind(oidSnmpTrapOID, encodeOID(oidLinkDown)),
		encodeVarbind(oidIfIndex1, encodeInteger(2)), // 负载 varbind
	))
	pduContent := concat(encodeInteger(1), encodeInteger(0), encodeInteger(0), vbList)
	ev := parseV2Trap(pduContent, "10.0.0.5", "public")

	if ev.Version != "2c" || ev.TrapOID != "1.3.6.1.6.3.1.1.5.3" {
		t.Fatalf("v2c trapOID 错: %+v", ev)
	}
	if ev.Severity != "warning" {
		t.Errorf("linkDown 应 warning, 得 %s", ev.Severity)
	}
	if ev.UptimeSec != 123.45 {
		t.Errorf("uptime = %v, 期望 123.45", ev.UptimeSec)
	}
	// sysUpTime/snmpTrapOID 不进 payload，只剩 ifIndex 一条
	if len(ev.Varbinds) != 1 || ev.Varbinds[0].Value != "2" {
		t.Errorf("payload varbind 错: %+v", ev.Varbinds)
	}
}

func TestParseTrapFullV2C(t *testing.T) {
	vbList := encodeTLV(tagSequence, concat(
		encodeVarbind(oidSysUpTime0, encodeUnsigned(tagTimeTicks, 100)),
		encodeVarbind(oidSnmpTrapOID, encodeOID(oidLinkDown)),
	))
	pdu := encodeTLV(pduTrapV2, concat(encodeInteger(1), encodeInteger(0), encodeInteger(0), vbList))
	msg := buildV2CMessage("public", pdu)

	tr := newSNMPTrapReceiver(SNMPConfig{}, "agent1", "fp")
	tr.parseTrap(msg, "10.0.0.9", nil)
	if len(tr.batch) != 1 {
		t.Fatalf("batch 应有 1 条, 得 %d", len(tr.batch))
	}
	if tr.batch[0].SourceIP != "10.0.0.9" || tr.batch[0].TrapOID != "1.3.6.1.6.3.1.1.5.3" {
		t.Errorf("parseTrap 结果错: %+v", tr.batch[0])
	}
}

func TestParseV1Trap(t *testing.T) {
	enterprise := []uint32{1, 3, 6, 1, 4, 1, 9}
	content := concat(
		encodeOID(enterprise),
		encodeTLV(tagIPAddress, []byte{10, 0, 0, 1}), // agent-addr
		encodeInteger(2),                             // generic = linkDown
		encodeInteger(0),                             // specific
		encodeUnsigned(tagTimeTicks, 500),            // timestamp
		encodeTLV(tagSequence, encodeVarbind(oidIfIndex1, encodeInteger(3))),
	)
	ev := parseV1Trap(content, "10.0.0.7", "public")
	if ev.Version != "1" || ev.TrapOID != "1.3.6.1.6.3.1.1.5.3" {
		t.Fatalf("v1 trapOID 错: %+v", ev)
	}
	if ev.Severity != "warning" || ev.GenericTrap != 2 {
		t.Errorf("v1 severity/generic 错: %+v", ev)
	}
	if ev.AgentAddr != "10.0.0.1" {
		t.Errorf("agent-addr = %s", ev.AgentAddr)
	}
	if ev.UptimeSec != 5 {
		t.Errorf("uptime = %v, 期望 5", ev.UptimeSec)
	}
}

func TestTrapCommunityWhitelist(t *testing.T) {
	vbList := encodeTLV(tagSequence, concat(
		encodeVarbind(oidSysUpTime0, encodeUnsigned(tagTimeTicks, 1)),
		encodeVarbind(oidSnmpTrapOID, encodeOID(oidLinkDown)),
	))
	pdu := encodeTLV(pduTrapV2, concat(encodeInteger(1), encodeInteger(0), encodeInteger(0), vbList))
	msg := buildV2CMessage("secret", pdu)

	tr := newSNMPTrapReceiver(SNMPConfig{TrapCommunities: []string{"public"}}, "a", "f")
	tr.parseTrap(msg, "1.2.3.4", nil) // community "secret" 不在白名单
	if len(tr.batch) != 0 {
		t.Errorf("白名单外 community 应丢弃, batch=%d", len(tr.batch))
	}
}
