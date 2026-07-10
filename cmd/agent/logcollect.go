package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"aiops-monitor/shared"
)

// Log collection — agent side.
//
// The agent tails the configured log files (--log-paths), classifies each new
// line's level heuristically, and forwards batches to the server every cycle.
// Only lines appended AFTER startup are sent (we seek to EOF first), so enabling
// collection never floods the server with historical logs. Rotation/truncation
// is detected (size < last offset) and re-read from the top.

var logCollectHTTP = &http.Client{Timeout: 20 * time.Second}

func (a *Agent) runLogCollectorFor(t *serverTarget) {
	if len(a.logPaths) == 0 || a.identity.Fingerprint == "" {
		return
	}
	slog.Info("日志采集已启用", "server", t.server, "文件数", len(a.logPaths))
	offsets := make(map[string]int64)
	for _, p := range a.logPaths { // seek to end so only NEW lines are forwarded
		if fi, err := os.Stat(p); err == nil {
			offsets[p] = fi.Size()
		}
	}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		var batch []shared.LogLine
		now := time.Now().Unix()
		for _, p := range a.logPaths {
			lines, newOff := readNewLines(p, offsets[p])
			offsets[p] = newOff
			for _, ln := range lines {
				batch = append(batch, shared.LogLine{Ts: now, Source: p, Level: classifyLogLevel(ln), Message: ln})
			}
			if len(batch) >= 500 {
				break
			}
		}
		if len(batch) > 500 {
			batch = batch[len(batch)-500:] // keep the most recent
		}
		if len(batch) > 0 {
			a.sendLogBatch(t, batch)
		}
	}
}

// readNewLines returns the lines appended after `off` plus the new offset.
// A file smaller than `off` is treated as rotated/truncated and re-read from 0;
// a very large gap only tails the last ~2MB to bound a single cycle.
func readNewLines(path string, off int64) ([]string, int64) {
	f, err := os.Open(path)
	if err != nil {
		return nil, off
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, off
	}
	size := fi.Size()
	if size < off {
		off = 0
	}
	if size <= off {
		return nil, size
	}
	start := off
	const maxRead = 2 << 20
	if size-start > maxRead {
		start = size - maxRead
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, size
	}
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		ln := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(ln) != "" {
			lines = append(lines, ln)
		}
	}
	return lines, size
}

func classifyLogLevel(line string) string {
	u := strings.ToUpper(line)
	switch {
	case strings.Contains(u, "ERROR") || strings.Contains(u, "FATAL") || strings.Contains(u, "PANIC") || strings.Contains(u, "CRITICAL") || strings.Contains(u, "[ERR"):
		return "error"
	case strings.Contains(u, "WARN"):
		return "warn"
	case strings.Contains(u, "DEBUG") || strings.Contains(u, "TRACE"):
		return "debug"
	default:
		return "info"
	}
}

func (a *Agent) sendLogBatch(t *serverTarget, lines []shared.LogLine) {
	body, _ := json.Marshal(shared.LogBatch{HostID: a.identity.HostID, Lines: lines})
	req, err := http.NewRequest(http.MethodPost, t.server+"/api/v1/agent/logs", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Fingerprint", a.identity.Fingerprint)
	if resp, err := logCollectHTTP.Do(req); err == nil {
		resp.Body.Close()
	}
}
