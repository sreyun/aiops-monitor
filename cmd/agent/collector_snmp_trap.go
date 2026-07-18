package main

// SNMP Trap 接收器（占位）。完整实现（监听 UDP:162、v1/v2c 解析、周期 flush）在阶段 6
// 补齐；此处先给 stub 让启动挂钩先编译通过。

import (
	"log/slog"

	"aiops-monitor/shared"
)

type snmpTrapReceiver struct {
	cfg    SNMPConfig
	hostID string
	fp     string
}

func newSNMPTrapReceiver(cfg SNMPConfig, hostID, fp string) *snmpTrapReceiver {
	return &snmpTrapReceiver{cfg: cfg, hostID: hostID, fp: fp}
}

// run 阶段 6 实现：监听 :162、解析 v1/v2c trap、周期 flush 上报。
func (tr *snmpTrapReceiver) run(reporter func(shared.SNMPTrapReport)) {
	_ = reporter
	slog.Warn("SNMP Trap 接收器尚未实现（阶段 6）", "listen", tr.cfg.TrapListen)
}
