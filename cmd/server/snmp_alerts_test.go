package main

import (
	"testing"

	"aiops-monitor/shared"
)

func findAlert(alerts []Alert, scopeSuffix string) *Alert {
	for i := range alerts {
		if len(alerts[i].Scope) >= len(scopeSuffix) && alerts[i].Scope[len(alerts[i].Scope)-len(scopeSuffix):] == scopeSuffix {
			return &alerts[i]
		}
	}
	return nil
}

func TestEvaluateSNMPInterfaceDownTransition(t *testing.T) {
	ss := newSNMPStore()
	upIf := shared.SNMPInterface{Index: 1, Name: "Gi0/1", AdminStatus: 1, OperStatus: 1, OperUp: true, RateValid: true}
	ss.put("agent1", "agent1", "10.0.0.9", []shared.SNMPSnapshot{
		{TargetName: "sw1", Reachable: true, Interfaces: []shared.SNMPInterface{upIf}},
	})
	// 首轮 up：不应有 down 告警，但要标记"曾 up"
	if a := findAlert(EvaluateSNMP(ss), "/status"); a != nil {
		t.Fatalf("接口 up 不应告警: %+v", a)
	}
	// 之后 down：admin-up 但 oper-down，且此前见过 up → 应告警 critical
	downIf := upIf
	downIf.OperStatus, downIf.OperUp = 2, false
	ss.put("agent1", "agent1", "10.0.0.9", []shared.SNMPSnapshot{
		{TargetName: "sw1", Reachable: true, Interfaces: []shared.SNMPInterface{downIf}},
	})
	a := findAlert(EvaluateSNMP(ss), "/status")
	if a == nil || a.Level != "critical" || a.Type != "snmp" {
		t.Fatalf("接口 down 应告警 critical/snmp: %+v", a)
	}
}

func TestEvaluateSNMPNeverUpNoAlert(t *testing.T) {
	ss := newSNMPStore()
	// 从未 up 过的 admin-up 口（如启用但闲置的端口）down 不告警，避免刷屏
	downIf := shared.SNMPInterface{Index: 2, Name: "Gi0/2", AdminStatus: 1, OperStatus: 2, OperUp: false}
	ss.put("a", "a", "", []shared.SNMPSnapshot{{TargetName: "sw1", Reachable: true, Interfaces: []shared.SNMPInterface{downIf}}})
	if a := findAlert(EvaluateSNMP(ss), "/status"); a != nil {
		t.Errorf("从未 up 的口 down 不应告警: %+v", a)
	}
}

func TestEvaluateSNMPUtilAndPollFail(t *testing.T) {
	ss := newSNMPStore()
	// 高利用率
	hot := shared.SNMPInterface{Index: 1, Name: "Gi0/1", AdminStatus: 1, OperStatus: 1, OperUp: true, RateValid: true, InUtilPercent: 97}
	ss.put("a", "a", "", []shared.SNMPSnapshot{{TargetName: "sw1", Reachable: true, Interfaces: []shared.SNMPInterface{hot}}})
	if a := findAlert(EvaluateSNMP(ss), "/util"); a == nil || a.Level != "critical" {
		t.Errorf("97%% 利用率应 critical: %+v", a)
	}
	// 采集失败
	ss.put("a", "a", "", []shared.SNMPSnapshot{{TargetName: "sw2", Error: "timeout"}})
	if a := findAlert(EvaluateSNMP(ss), "/poll"); a == nil || a.Level != "warning" {
		t.Errorf("采集失败应 warning: %+v", a)
	}
}

func TestEvaluateNetFlowSurge(t *testing.T) {
	ns := newNFStore()
	base := shared.NetFlowReport{WindowSec: 60, Stats: shared.NetFlowStats{TotalBytes: 60 * 1_000_000 / 8}} // ~1Mbps
	for i := 0; i < 6; i++ {
		ns.put("h1", "h1", "", base)
	}
	if EvaluateNetFlow(ns) != nil && len(EvaluateNetFlow(ns)) > 0 {
		t.Fatal("稳定基线不应突增告警")
	}
	// 10x 突增
	surge := shared.NetFlowReport{WindowSec: 60, Stats: shared.NetFlowStats{TotalBytes: 60 * 10_000_000 / 8}}
	ns.put("h1", "h1", "", surge)
	a := findAlert(EvaluateNetFlow(ns), "traffic/surge")
	if a == nil || a.Type != "netflow" {
		t.Fatalf("10x 突增应告警: %+v", a)
	}
}

func TestEvaluateNetFlowDrops(t *testing.T) {
	ns := newNFStore()
	ns.put("h1", "h1", "", shared.NetFlowReport{WindowSec: 60, Stats: shared.NetFlowStats{DroppedPackets: 500}})
	if a := findAlert(EvaluateNetFlow(ns), "collector/drops"); a == nil {
		t.Error("采集丢包 500 应告警")
	}
}
