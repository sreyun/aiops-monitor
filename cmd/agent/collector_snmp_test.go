package main

import (
	"testing"
	"time"
)

// ---- PDU / v2c 消息往返 ----

func TestV2CMessageRoundTrip(t *testing.T) {
	reqID := int32(12345)
	oids := [][]uint32{oidSysDescr, oidSysUpTime}
	msg := buildV2CMessage("public", buildGet(reqID, oids))

	ver, community, p, err := parseV2CMessage(msg)
	if err != nil {
		t.Fatalf("parseV2CMessage: %v", err)
	}
	if ver != 1 {
		t.Errorf("version = %d, 期望 1(v2c)", ver)
	}
	if community != "public" {
		t.Errorf("community = %q", community)
	}
	if p.pduType != pduGet {
		t.Errorf("pduType = %#x, 期望 GET", p.pduType)
	}
	if p.requestID != reqID {
		t.Errorf("requestID = %d, 期望 %d", p.requestID, reqID)
	}
	if len(p.varbinds) != 2 {
		t.Fatalf("varbinds = %d, 期望 2", len(p.varbinds))
	}
	if !oidEqual(p.varbinds[0].oid, oidSysDescr) {
		t.Errorf("varbind[0] OID = %s", oidToString(p.varbinds[0].oid))
	}
}

func TestGetBulkBuildParse(t *testing.T) {
	// GETBULK 的 non-repeaters/max-repetitions 复用 error-status/error-index 槽
	msg := buildV2CMessage("c", buildGetBulk(7, 0, 10, [][]uint32{colIfHCInOctets}))
	_, _, p, err := parseV2CMessage(msg)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.pduType != pduGetBulk || p.requestID != 7 || p.errStatus != 0 || p.errIndex != 10 {
		t.Errorf("GETBULK 解析错: type=%#x id=%d nonRep=%d maxRep=%d", p.pduType, p.requestID, p.errStatus, p.errIndex)
	}
}

// ---- 内存 MIB 模拟 agent，验证 GETBULK 表遍历 ----

type mibEntry struct {
	oid []uint32
	val snmpValue
}

type mockAgent struct{ entries []mibEntry } // 必须按 OID 升序

func oidCompare(a, b []uint32) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

func (m *mockAgent) getNext(oid []uint32) *mibEntry {
	for i := range m.entries {
		if oidCompare(m.entries[i].oid, oid) > 0 {
			return &m.entries[i]
		}
	}
	return nil
}

func (m *mockAgent) get(oids [][]uint32) (pdu, error) {
	p := pdu{pduType: pduResponse}
	for _, o := range oids {
		val := snmpValue{Tag: tagNoSuchInstance, Exc: tagNoSuchInstance}
		for _, e := range m.entries {
			if oidEqual(e.oid, o) {
				val = e.val
				break
			}
		}
		p.varbinds = append(p.varbinds, varbind{oid: o, value: val})
	}
	return p, nil
}

func (m *mockAgent) getBulk(nonRep, maxRep int, oids [][]uint32) (pdu, error) {
	p := pdu{pduType: pduResponse}
	cursors := make([][]uint32, len(oids))
	copy(cursors, oids)
	for r := 0; r < maxRep; r++ { // repetition-major
		for c := range oids {
			next := m.getNext(cursors[c])
			if next == nil {
				p.varbinds = append(p.varbinds, varbind{oid: cursors[c], value: snmpValue{Tag: tagEndOfMibView, Exc: tagEndOfMibView}})
				continue
			}
			p.varbinds = append(p.varbinds, varbind{oid: next.oid, value: next.val})
			cursors[c] = next.oid
		}
	}
	return p, nil
}

func (m *mockAgent) close() {}

func withIndex(col []uint32, idx uint32) []uint32 {
	out := make([]uint32, len(col)+1)
	copy(out, col)
	out[len(col)] = idx
	return out
}

func TestWalkColumns(t *testing.T) {
	// 两台接口 ifIndex 1/2，两列 ifDescr + ifHCInOctets
	m := &mockAgent{entries: []mibEntry{
		{withIndex(colIfDescr, 1), snmpValue{Tag: tagOctetString, Bytes: []byte("eth0")}},
		{withIndex(colIfDescr, 2), snmpValue{Tag: tagOctetString, Bytes: []byte("eth1")}},
		{withIndex(colIfHCInOctets, 1), snmpValue{Tag: tagCounter64, Uint: 1000}},
		{withIndex(colIfHCInOctets, 2), snmpValue{Tag: tagCounter64, Uint: 2000}},
	}}
	table, err := walkColumns(m, [][]uint32{colIfDescr, colIfHCInOctets}, 10)
	if err != nil {
		t.Fatalf("walkColumns: %v", err)
	}
	if len(table) != 2 {
		t.Fatalf("接口数 = %d, 期望 2", len(table))
	}
	if v := table[1][oidToString(colIfDescr)]; v.String() != "eth0" {
		t.Errorf("if1 ifDescr = %q", v.String())
	}
	if v := table[2][oidToString(colIfHCInOctets)]; v.Uint != 2000 {
		t.Errorf("if2 ifHCInOctets = %d", v.Uint)
	}
	// 关键：col0(ifDescr) 越界后不得把 ifHCInOctets 值错记到 ifDescr 键下
	if v, ok := table[1][oidToString(colIfDescr)]; !ok || v.Uint == 1000 {
		t.Errorf("if1 ifDescr 被污染: %+v", v)
	}
}

// ---- 速率计算 ----

func TestCounterDelta(t *testing.T) {
	// 正常
	if d, ok := counterDelta(100, 500, true, 10, 0); !ok || d != 400 {
		t.Errorf("正常 delta = %d,%v", d, ok)
	}
	// 64 位 cur<prev → 判复位
	if _, ok := counterDelta(500, 100, true, 10, 0); ok {
		t.Error("64 位回退应判复位")
	}
	// 32 位回绕（合理速率内）
	prev := uint64(1<<32 - 100)
	cur := uint64(50) // 绕过 0，共传 150
	if d, ok := counterDelta(prev, cur, false, 10, 1_000_000_000); !ok || d != 150 {
		t.Errorf("32 位回绕 delta = %d,%v, 期望 150", d, ok)
	}
	// 小值回退且按回绕换算速率爆表（远超链路）→ 是复位不是回绕，判无效
	// prev=100,cur=50 → 若当回绕 delta≈2^32，换算速率天量，必判复位
	if _, ok := counterDelta(100, 50, false, 1, 1000 /*仅1kbps链路*/); ok {
		t.Error("小值回退换算速率超链路应判复位")
	}
}

func TestComputeRates(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	prev := ifCounterSample{ts: t0, inOctets: 0, outOctets: 0, is64: true}
	cur := ifCounterSample{ts: t0.Add(10 * time.Second), inOctets: 1_250_000, outOctets: 625_000, is64: true}
	r := computeRates(prev, cur, 100_000_000) // 100Mbps
	if !r.valid {
		t.Fatal("应有效")
	}
	// 1.25MB/10s = 125000 B/s = 1_000_000 bps
	if r.inBps != 1_000_000 {
		t.Errorf("inBps = %v, 期望 1000000", r.inBps)
	}
	if r.inUtil != 1.0 { // 1Mbps / 100Mbps = 1%
		t.Errorf("inUtil = %v, 期望 1.0", r.inUtil)
	}
	// discontinuity 变化 → 无效
	cur2 := cur
	cur2.discontinuity = 999
	if computeRates(prev, cur2, 100_000_000).valid {
		t.Error("discontinuity 变化应判无效")
	}
	// 间隔非正 → 无效
	if computeRates(cur, prev, 100_000_000).valid {
		t.Error("负间隔应判无效")
	}
}
