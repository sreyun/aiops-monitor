//go:build !linux

package main

import (
	"log/slog"
	"runtime"

	"aiops-monitor/shared"
)

// run: SNI/DNS 抓取依赖 Linux AF_PACKET 原始套接字，其它平台暂不支持（Windows 需 Npcap 驱动）。
func (sc *sniCollector) run(_ func(shared.DNSMapReport), _ func(shared.ContentAuditReport)) {
	slog.Info("SNI/DNS 抓取: 仅支持 Linux(AF_PACKET)，当前平台跳过", "os", runtime.GOOS)
}
