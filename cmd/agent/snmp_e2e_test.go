package main

import (
	"net"
	"sort"
	"sync"
	"testing"
	"time"
)

// 真实 UDP 端到端：起一个模拟 SNMP 设备，用真实 snmpCollector 走完整 v2c 轮询链路
// （v2cExchanger → UDP 收发 → GET 系统组 → GETBULK 表遍历 → 速率计算）。

type testMIB struct {
	mu      sync.Mutex
	entries []mibEntry // 按 OID 升序
}

func (m *testMIB) get(oid []uint32) varbind {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if oidEqual(e.oid, oid) {
			return varbind{oid: oid, value: e.val}
		}
	}
	return varbind{oid: oid, value: snmpValue{Tag: tagNoSuchInstance, Exc: tagNoSuchInstance}}
}

func (m *testMIB) getNext(oid []uint32) (mibEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if oidCompare(e.oid, oid) > 0 {
			return e, true
		}
	}
	return mibEntry{}, false
}

func (m *testMIB) getBulk(reqOIDs []varbind, maxRep int) []varbind {
	var out []varbind
	cursors := make([][]uint32, len(reqOIDs))
	for i, vb := range reqOIDs {
		cursors[i] = vb.oid
	}
	for r := 0; r < maxRep; r++ {
		for c := range reqOIDs {
			e, ok := m.getNext(cursors[c])
			if !ok {
				out = append(out, varbind{oid: cursors[c], value: snmpValue{Tag: tagEndOfMibView, Exc: tagEndOfMibView}})
				continue
			}
			out = append(out, varbind{oid: e.oid, value: e.val})
			cursors[c] = e.oid
		}
	}
	return out
}

// advance 递增八位组计数器，模拟流量。
func (m *testMIB) advance(delta uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.entries {
		if oidHasPrefix(m.entries[i].oid, colIfHCInOctets) || oidHasPrefix(m.entries[i].oid, colIfHCOutOctets) {
			m.entries[i].val.Uint += delta
		}
	}
}

func buildTestMIB() *testMIB {
	octet := func(s string) snmpValue { return snmpValue{Tag: tagOctetString, Bytes: []byte(s)} }
	i := func(v int64) snmpValue { return snmpValue{Tag: tagInteger, Int: v} }
	g := func(v uint64) snmpValue { return snmpValue{Tag: tagGauge32, Uint: v} }
	c64 := func(v uint64) snmpValue { return snmpValue{Tag: tagCounter64, Uint: v} }

	e := []mibEntry{
		{[]uint32{1, 3, 6, 1, 2, 1, 1, 1, 0}, octet("Mock Switch v1")},                     // sysDescr
		{[]uint32{1, 3, 6, 1, 2, 1, 1, 3, 0}, snmpValue{Tag: tagTimeTicks, Uint: 8640000}}, // sysUpTime 24h
		{[]uint32{1, 3, 6, 1, 2, 1, 1, 5, 0}, octet("core-sw")},                            // sysName
	}
	for _, idx := range []uint32{1, 2} {
		e = append(e,
			mibEntry{withIndex(colIfDescr, idx), octet("GigabitEthernet0/" + string(rune('0'+idx)))},
			mibEntry{withIndex(colIfAdminStatus, idx), i(1)}, // up
			mibEntry{withIndex(colIfOperStatus, idx), i(1)},  // up
			mibEntry{withIndex(colIfName, idx), octet("Gi0/" + string(rune('0'+idx)))},
			mibEntry{withIndex(colIfHighSpeed, idx), g(100000)}, // 100 Gbps → 短间隔轮询速率仍远低于链路
			mibEntry{withIndex(colIfHCInOctets, idx), c64(1_000_000 * uint64(idx))},
			mibEntry{withIndex(colIfHCOutOctets, idx), c64(2_000_000 * uint64(idx))},
		)
	}
	sort.Slice(e, func(a, b int) bool { return oidCompare(e[a].oid, e[b].oid) < 0 })
	return &testMIB{entries: e}
}

func serveMockSNMP(t *testing.T, conn net.PacketConn, mib *testMIB) {
	buf := make([]byte, 65535)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		req := make([]byte, n)
		copy(req, buf[:n])
		_, community, p, err := parseV2CMessage(req)
		if err != nil {
			continue
		}
		var resp pdu
		resp.requestID = p.requestID
		switch p.pduType {
		case pduGet:
			for _, vb := range p.varbinds {
				resp.varbinds = append(resp.varbinds, mib.get(vb.oid))
			}
		case pduGetBulk:
			resp.varbinds = mib.getBulk(p.varbinds, p.errIndex) // errIndex 槽=max-repetitions
		}
		out := buildV2CMessage(community, encodeResponsePDU(resp))
		_, _ = conn.WriteTo(out, addr)
	}
}

func encodeResponsePDU(p pdu) []byte {
	var vbs []byte
	for _, vb := range p.varbinds {
		vbs = append(vbs, encodeVarbind(vb.oid, encodeValueTLV(vb.value))...)
	}
	body := concat(
		encodeInteger(int64(p.requestID)),
		encodeInteger(0),
		encodeInteger(0),
		encodeTLV(tagSequence, vbs),
	)
	return encodeTLV(pduResponse, body)
}

func encodeValueTLV(v snmpValue) []byte {
	switch v.Tag {
	case tagInteger:
		return encodeInteger(v.Int)
	case tagCounter32, tagGauge32, tagTimeTicks, tagCounter64:
		return encodeUnsigned(v.Tag, v.Uint)
	case tagOctetString:
		return encodeOctetString(v.Bytes)
	case tagOID:
		return encodeOID(v.OID)
	case tagIPAddress:
		return encodeTLV(tagIPAddress, v.Bytes)
	case tagNoSuchObject, tagNoSuchInstance, tagEndOfMibView:
		return []byte{v.Tag, 0x00}
	default:
		return encodeNull()
	}
}

func TestSNMPCollectorE2E(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer conn.Close()
	mib := buildTestMIB()
	go serveMockSNMP(t, conn, mib)

	port := conn.LocalAddr().(*net.UDPAddr).Port
	sc := newSNMPCollector(SNMPConfig{}, "agent1", "fp")
	target := SNMPTarget{Name: "sw1", IP: "127.0.0.1", Port: port, Version: "2c", Community: "public", TimeoutSec: 2, MaxRepetitions: 10}

	// 首轮：系统组 + 接口表，速率不可信（无基线）
	snap1 := sc.collectOne(target)
	if snap1.Error != "" {
		t.Fatalf("首轮采集失败: %s", snap1.Error)
	}
	if snap1.System.Descr != "Mock Switch v1" || snap1.System.Name != "core-sw" {
		t.Errorf("系统组解析错: %+v", snap1.System)
	}
	if len(snap1.Interfaces) != 2 {
		t.Fatalf("接口数 = %d, 期望 2", len(snap1.Interfaces))
	}
	if1 := snap1.Interfaces[0]
	if !if1.OperUp || if1.SpeedBps != 100000*1_000_000 || if1.Name == "" {
		t.Errorf("接口解析错: %+v", if1)
	}
	if if1.RateValid {
		t.Error("首轮不应有有效速率")
	}

	// 递增计数器 + 小间隔后第二轮：速率应有效
	time.Sleep(30 * time.Millisecond)
	mib.advance(1_000_000)
	snap2 := sc.collectOne(target)
	if snap2.Error != "" {
		t.Fatalf("第二轮采集失败: %s", snap2.Error)
	}
	var got bool
	for _, iface := range snap2.Interfaces {
		if iface.RateValid && iface.InBps > 0 {
			got = true
		}
	}
	if !got {
		t.Error("第二轮应至少一个接口有有效速率且 InBps>0")
	}
}
