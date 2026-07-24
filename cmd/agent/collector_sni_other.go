//go:build !linux

package main

import (
	"context"
	"fmt"
	"runtime"

	"aiops-monitor/shared"
)

// Non-Linux systems use the TShark backend. Keeping an explicit native stub
// makes capture_backend=native fail clearly instead of silently doing nothing.
func (sc *sniCollector) runNative(_ context.Context, _ func(shared.DNSMapReport), _ func(shared.ContentAuditReport)) error {
	return fmt.Errorf("native 抓包后端不支持 %s；请使用 capture_backend: tshark", runtime.GOOS)
}
