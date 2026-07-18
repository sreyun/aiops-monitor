package main

import (
	"strings"
	"testing"

	"aiops-monitor/shared"
)

// SNMP 指标同 netflow 一样，成败标准是"序列数有上界"：接口数被 snmpMaxIfaces 封顶。

func mkSNMPSnap(nIf int) shared.SNMPSnapshot {
	snap := shared.SNMPSnapshot{
		TargetName: "sw1", TargetIP: "10.0.0.1", Timestamp: 1784000000,
		Reachable: true,
		System:    shared.SNMPSystem{Name: "sw1", UptimeSec: 12345},
	}
	for i := 0; i < nIf; i++ {
		snap.Interfaces = append(snap.Interfaces, shared.SNMPInterface{
			Index: uint32(i + 1), Name: "Gi0/" + string(rune('0'+i%10)),
			OperUp: true, SpeedBps: 1_000_000_000, RateValid: true,
			InBps: 1000, OutBps: 2000, InUtilPercent: 0.1,
		})
	}
	return snap
}

func TestRollupSNMPBoundsCardinality(t *testing.T) {
	lines := rollupSNMP("h1", mkSNMPSnap(1000)) // 远超上限
	// 上界 = 2(reachable+uptime) + snmpMaxIfaces * 每接口最多条数
	const perIf = 11 // oper_up + speed + 8 rate series（RateValid 时）
	max := 2 + snmpMaxIfaces*perIf
	if len(lines) > max {
		t.Errorf("SNMP rollup 序列数 %d 超上界 %d", len(lines), max)
	}
	// 时间戳必须是毫秒（*1000），不能写进 1970
	for _, l := range lines {
		if strings.Contains(l, " 1784000000\n") || strings.HasSuffix(l, " 1784000000") {
			t.Errorf("时间戳未转毫秒: %s", l)
		}
	}
}

func TestRollupSNMPFailedSnapshotSkipped(t *testing.T) {
	// 采集失败的快照（Error 非空）不应经 rollup（由 handleAgentSNMP 提前拦截）；
	// 这里验证 reachable=false 时至少 reachable 序列值为 0。
	snap := shared.SNMPSnapshot{TargetName: "sw1", Timestamp: 1784000000, Reachable: false}
	lines := rollupSNMP("h1", snap)
	if len(lines) == 0 || !strings.Contains(lines[0], "aiops_snmp_reachable") || !strings.Contains(lines[0], "} 0 ") {
		t.Errorf("unreachable 设备 reachable 序列应为 0: %v", lines)
	}
}
