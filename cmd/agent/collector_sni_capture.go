package main

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"time"

	"aiops-monitor/shared"
)

// effectiveCaptureBackend keeps the default dependency-free on Linux while
// selecting the mature libpcap/Npcap-based TShark path on desktop platforms.
func effectiveCaptureBackend(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "native":
		return "native"
	case "tshark":
		return "tshark"
	}
	if runtime.GOOS == "linux" {
		return "native"
	}
	return "tshark"
}

func (sc *sniCollector) run(ctx context.Context, reporter func(shared.DNSMapReport), contentReporter func(shared.ContentAuditReport)) {
	backend := effectiveCaptureBackend(sc.cfg.CaptureBackend)
	slog.Info("网络内容审计采集器启动",
		"backend", backend, "os", runtime.GOOS, "interface", sc.cfg.Interface,
		"content_audit", sc.cfg.ContentAudit, "body_mode", normalizeContentBodyMode(sc.cfg.ContentAuditBodyMode))

	if backend == "native" {
		if err := sc.runNative(ctx, reporter, contentReporter); err != nil && ctx.Err() == nil {
			slog.Error("原生网络内容审计采集器退出", "err", err)
		}
		return
	}

	backoff := 5 * time.Second
	for ctx.Err() == nil {
		err := sc.runTShark(ctx, reporter, contentReporter)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = fmt.Errorf("tshark 未返回错误但采集进程已退出")
		}
		slog.Error("TShark 内容审计后端退出，将自动重试",
			"err", err, "retry_after", backoff)
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < time.Minute {
			backoff *= 2
			if backoff > time.Minute {
				backoff = time.Minute
			}
		}
	}
}
