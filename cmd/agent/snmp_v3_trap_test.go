package main

import "testing"

// buildDeviceV3Trap 用 USM 原语在"设备侧"造一条 v3 trap（发送方是 authoritative engine）。
func buildDeviceV3Trap(t *testing.T, usr *usmUser, engineID []byte, boots, etime, msgID int32) []byte {
	t.Helper()
	eng := &engineEntry{id: engineID, boots: boots, time: etime}
	eng.deriveKeys(usr)
	vbList := encodeTLV(tagSequence, concat(
		encodeVarbind(oidSysUpTime0, encodeUnsigned(tagTimeTicks, 200)),
		encodeVarbind(oidSnmpTrapOID, encodeOID(oidLinkDown)),
	))
	trapPDU := encodeTLV(pduTrapV2, concat(encodeInteger(1), encodeInteger(0), encodeInteger(0), vbList))
	msg, err := usr.buildV3Message(eng, msgID, trapPDU)
	if err != nil {
		t.Fatalf("造 v3 trap 失败: %v", err)
	}
	return msg
}

func TestParseV3TrapAuthNoPriv(t *testing.T) {
	engineID := []byte{0x80, 0x00, 0x1f, 0x88, 0x01, 0x02, 0x03, 0x04}
	dev := &usmUser{name: "trapuser", authProto: "SHA", authPass: []byte("authpass123"), secLevel: 1}
	msg := buildDeviceV3Trap(t, dev, engineID, 5, 100, 42)

	// 接收端配同一 USM 用户 → 验签通过、解析成功
	tr := newSNMPTrapReceiver(SNMPConfig{TrapUsers: []SNMPTrapUser{
		{User: "trapuser", SecLevel: "authNoPriv", AuthProto: "SHA", AuthPass: "authpass123"},
	}}, "agent", "fp")
	tr.parseTrap(msg, "10.0.0.9", nil)
	if len(tr.batch) != 1 {
		t.Fatalf("v3 authNoPriv trap 应入队 1 条, 得 %d", len(tr.batch))
	}
	if tr.batch[0].Version != "3" || tr.batch[0].TrapOID != "1.3.6.1.6.3.1.1.5.3" {
		t.Errorf("v3 trap 解析错: %+v", tr.batch[0])
	}
	if tr.batch[0].Community != "trapuser" {
		t.Errorf("v3 trap 应回显 USM 用户名, 得 %q", tr.batch[0].Community)
	}

	// 错误口令 → HMAC 校验失败 → 丢弃
	trBad := newSNMPTrapReceiver(SNMPConfig{TrapUsers: []SNMPTrapUser{
		{User: "trapuser", SecLevel: "authNoPriv", AuthProto: "SHA", AuthPass: "WRONGPASS"},
	}}, "agent", "fp")
	trBad.parseTrap(msg, "10.0.0.9", nil)
	if len(trBad.batch) != 0 {
		t.Error("错误口令应 HMAC 校验失败并丢弃")
	}

	// 未配置该用户 → auth trap 无从验签 → 丢弃
	trNone := newSNMPTrapReceiver(SNMPConfig{}, "agent", "fp")
	trNone.parseTrap(msg, "10.0.0.9", nil)
	if len(trNone.batch) != 0 {
		t.Error("无匹配 USM 用户的 auth trap 应丢弃")
	}
}

func TestParseV3TrapAuthPriv(t *testing.T) {
	engineID := []byte{0x80, 0x00, 0x1f, 0x88, 0x0a, 0x0b, 0x0c, 0x0d}
	dev := &usmUser{
		name: "secu", authProto: "SHA", authPass: []byte("authpass123"),
		privProto: "AES", privPass: []byte("privpass123"), secLevel: 3,
	}
	msg := buildDeviceV3Trap(t, dev, engineID, 1, 50, 7)

	tr := newSNMPTrapReceiver(SNMPConfig{TrapUsers: []SNMPTrapUser{
		{User: "secu", SecLevel: "authPriv", AuthProto: "SHA", AuthPass: "authpass123", PrivProto: "AES", PrivPass: "privpass123"},
	}}, "agent", "fp")
	tr.parseTrap(msg, "10.0.0.11", nil)
	if len(tr.batch) != 1 {
		t.Fatalf("v3 authPriv trap 应解密并入队 1 条, 得 %d", len(tr.batch))
	}
	if tr.batch[0].TrapOID != "1.3.6.1.6.3.1.1.5.3" || tr.batch[0].Severity != "warning" {
		t.Errorf("v3 authPriv trap 解析错: %+v", tr.batch[0])
	}

	// 错误加密口令 → 解密失败 → 丢弃
	trBad := newSNMPTrapReceiver(SNMPConfig{TrapUsers: []SNMPTrapUser{
		{User: "secu", SecLevel: "authPriv", AuthProto: "SHA", AuthPass: "authpass123", PrivProto: "AES", PrivPass: "WRONGPRIV"},
	}}, "agent", "fp")
	trBad.parseTrap(msg, "10.0.0.11", nil)
	if len(trBad.batch) != 0 {
		t.Error("错误加密口令应解密失败并丢弃")
	}
}
