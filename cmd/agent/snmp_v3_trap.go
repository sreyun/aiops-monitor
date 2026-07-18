package main

// SNMPv3 trap / inform 接收（USM 验签 + 解密）。与轮询相反：发送方(被管设备)是
// authoritative engine，接收端按来信里的 engineID 本地化用户密钥来验签/解密。inform 需
// 回一个 authenticated Response 确认，否则发送方会重传，造成重复告警。复用 snmp_v3.go 的
// USM 原语（parseV3SecParams / locateAuthParams / computeAuth / decrypt / buildV3Message）。

import (
	"crypto/hmac"
	"errors"
	"log/slog"
	"net"

	"aiops-monitor/shared"
)

// matchTrapUser 按 userName 找配置的 v3 trap USM 用户（nil = 未配置）。
func (tr *snmpTrapReceiver) matchTrapUser(name string) *SNMPTrapUser {
	for i := range tr.cfg.TrapUsers {
		if tr.cfg.TrapUsers[i].User == name {
			return &tr.cfg.TrapUsers[i]
		}
	}
	return nil
}

// parseV3Trap 解析 v3 trap/inform：本地化密钥 → 按 msgFlags 验签/解密 → 解析 ScopedPDU。
func (tr *snmpTrapReceiver) parseV3Trap(data []byte, srcIP string, srcAddr net.Addr) {
	sp, msgData, err := parseV3SecParams(data)
	if err != nil {
		slog.Warn("v3 trap 安全参数解析失败", "src", srcIP, "err", err)
		return
	}
	userName, err := v3SecUserName(data)
	if err != nil {
		slog.Warn("v3 trap userName 解析失败", "src", srcIP, "err", err)
		return
	}
	authFlag := sp.flags&0x01 != 0
	privFlag := sp.flags&0x02 != 0

	var usr *usmUser
	var eng *engineEntry
	if u := tr.matchTrapUser(userName); u != nil {
		usr = u.toUSMUser()
		eng = &engineEntry{id: sp.engineID, boots: sp.boots, time: sp.time}
		eng.deriveKeys(usr) // 按发送方(authoritative) engineID 本地化认证/加密密钥
	}

	// 验签：需 auth 但无匹配用户/校验失败 → 丢弃（防伪造）。
	if authFlag {
		if usr == nil || usr.secLevel < 1 {
			slog.Warn("v3 trap 需验签但无匹配 USM 用户，丢弃", "src", srcIP, "user", userName)
			return
		}
		if !verifyV3TrapAuth(data, usr.authProto, eng.authKul) {
			slog.Warn("v3 trap HMAC 校验失败，丢弃", "src", srcIP, "user", userName)
			return
		}
	}

	// 解密（authPriv）。
	scoped := msgData
	if privFlag {
		if usr == nil || usr.secLevel < 3 {
			slog.Warn("v3 trap 需解密但用户非 authPriv，丢弃", "src", srcIP, "user", userName)
			return
		}
		tag, ct, _, e := readTLV(msgData)
		if e != nil || tag != tagOctetString {
			slog.Warn("v3 trap 密文封装异常，丢弃", "src", srcIP)
			return
		}
		pt, e := usr.decrypt(sp.boots, sp.time, eng.privKul, sp.privParams, ct)
		if e != nil {
			slog.Warn("v3 trap 解密失败", "src", srcIP, "err", e)
			return
		}
		scoped = pt
	}

	ev, pduType, reqID, rawVarbinds, ok := parseV3ScopedTrap(scoped, srcIP)
	if !ok {
		slog.Warn("v3 trap ScopedPDU 解析失败", "src", srcIP)
		return
	}
	ev.Community = userName // v3 无 community，借该字段展示 USM 用户名

	// inform：回 authenticated Response 确认，避免发送方重传 → 重复告警。
	if pduType == pduInform && usr != nil {
		tr.sendV3InformResponse(data, srcAddr, usr, eng, reqID, rawVarbinds)
	}
	tr.enqueue(ev, srcIP)
}

// parseV3ScopedTrap 解析 ScopedPDU：SEQUENCE{ contextEngineID, contextName, PDU }，
// PDU 为 SNMPv2-Trap(0xA7) 或 InformRequest(0xA6)。返回归一事件、PDU 类型、request-id、
// 原始 varbinds 编码（供 inform Response 回显）。
func parseV3ScopedTrap(scoped []byte, srcIP string) (shared.SNMPTrapEvent, byte, int32, []byte, bool) {
	var ev shared.SNMPTrapEvent
	_, body, _, err := readTLV(scoped)
	if err != nil {
		return ev, 0, 0, nil, false
	}
	_, _, r, err := readTLV(body) // contextEngineID
	if err != nil {
		return ev, 0, 0, nil, false
	}
	_, _, r, err = readTLV(r) // contextName
	if err != nil {
		return ev, 0, 0, nil, false
	}
	ptag, pc, _, err := readTLV(r) // PDU
	if err != nil || (ptag != pduTrapV2 && ptag != pduInform) {
		return ev, 0, 0, nil, false
	}
	p, err := parsePDU(ptag, pc)
	if err != nil {
		return ev, 0, 0, nil, false
	}
	ev = shared.SNMPTrapEvent{Version: "3", SourceIP: srcIP}
	fillTrapFromPDU(&ev, p)
	return ev, ptag, p.requestID, extractRawVarbinds(pc), true
}

// extractRawVarbinds 从 PDU 内容(request-id/error-status/error-index/varbinds)取出
// varbinds SEQUENCE 的完整编码，供 inform Response 原样回显（RFC 3416 要求回显绑定）。
func extractRawVarbinds(pduContent []byte) []byte {
	empty := encodeTLV(tagSequence, nil)
	r := pduContent
	for i := 0; i < 3; i++ { // 跳过 request-id / error-status / error-index
		_, _, rest, err := readTLV(r)
		if err != nil {
			return empty
		}
		r = rest
	}
	tag, content, _, err := readTLV(r)
	if err != nil || tag != tagSequence {
		return empty
	}
	return encodeTLV(tagSequence, content)
}

// verifyV3TrapAuth 校验报文 HMAC：拷贝报文 → authParams 清零 → 重算 → 常量时间比对。
func verifyV3TrapAuth(data []byte, authProto string, authKul []byte) bool {
	msgCopy := append([]byte(nil), data...)
	authP, err := locateAuthParams(msgCopy)
	if err != nil || len(authP) == 0 {
		return false
	}
	received := append([]byte(nil), authP...)
	for i := range authP {
		authP[i] = 0
	}
	mac := computeAuth(authProto, authKul, msgCopy)
	if len(mac) != len(received) {
		return false
	}
	return hmac.Equal(mac, received)
}

// sendV3InformResponse 回一个 authenticated Response 确认 inform。复用发送方(authoritative)
// engine 与已本地化的用户密钥，msgID 沿用来信，回显原始 varbinds。
func (tr *snmpTrapReceiver) sendV3InformResponse(reqMsg []byte, srcAddr net.Addr, usr *usmUser, eng *engineEntry, reqID int32, rawVarbinds []byte) {
	if tr.conn == nil || srcAddr == nil || eng == nil {
		return
	}
	msgID, err := v3MsgID(reqMsg)
	if err != nil {
		return
	}
	respPDU := encodeTLV(pduResponse, concat(
		encodeInteger(int64(reqID)),
		encodeInteger(0), // error-status
		encodeInteger(0), // error-index
		rawVarbinds,
	))
	msg, err := usr.buildV3Message(eng, msgID, respPDU)
	if err != nil {
		slog.Warn("v3 inform 响应构建失败", "err", err)
		return
	}
	if _, err := tr.conn.WriteTo(msg, srcAddr); err != nil {
		slog.Warn("v3 inform 响应发送失败", "err", err)
	}
}

// v3SecUserName 从 v3 报文的 USM 安全参数里取 msgUserName。
func v3SecUserName(data []byte) (string, error) {
	_, body, _, err := readTLV(data)
	if err != nil {
		return "", err
	}
	_, _, rest, err := readTLV(body) // version
	if err != nil {
		return "", err
	}
	_, _, rest, err = readTLV(rest) // HeaderData
	if err != nil {
		return "", err
	}
	tag, usmOctet, _, err := readTLV(rest) // msgSecurityParameters
	if err != nil || tag != tagOctetString {
		return "", errors.New("snmp v3: 无 secparams")
	}
	_, usmBody, _, err := readTLV(usmOctet) // USM SEQUENCE
	if err != nil {
		return "", err
	}
	r := usmBody
	for i := 0; i < 3; i++ { // 跳过 engineID / boots / time
		if _, _, r, err = readTLV(r); err != nil {
			return "", err
		}
	}
	_, name, _, err := readTLV(r) // userName
	if err != nil {
		return "", err
	}
	return string(name), nil
}

// v3MsgID 取 HeaderData 的第一个字段 msgID（inform Response 需沿用）。
func v3MsgID(data []byte) (int32, error) {
	_, body, _, err := readTLV(data)
	if err != nil {
		return 0, err
	}
	_, _, rest, err := readTLV(body) // version
	if err != nil {
		return 0, err
	}
	_, hdr, _, err := readTLV(rest) // HeaderData
	if err != nil {
		return 0, err
	}
	_, idc, _, err := readTLV(hdr) // msgID
	if err != nil {
		return 0, err
	}
	v, err := decodeInteger(idc)
	return int32(v), err
}
