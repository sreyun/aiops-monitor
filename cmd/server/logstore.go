package main

import (
	"strings"
	"sync"

	"aiops-monitor/shared"
)

// ============================================================================
// Log aggregation — the second observability pillar.
//
// Agents tail their configured log sources and POST batches here; the server
// keeps a capped in-memory ring that operators (and the AI inspector) can search
// by host / level / keyword / time. Logs are high-volume and ephemeral, so they
// are deliberately NOT persisted — after a restart they simply re-accumulate.
// ============================================================================

// StoredLog is one collected log line, enriched with the source hostname.
type StoredLog struct {
	Ts       int64  `json:"ts"`
	HostID   string `json:"host_id"`
	Hostname string `json:"hostname"`
	Source   string `json:"source"`
	Level    string `json:"level"`
	Message  string `json:"message"`
}

const logStoreCap = 50000

// logPersistCap bounds how many recent lines get written to PG. Memory keeps the
// full ring for search; persistence only needs a warm tail to survive restart,
// so the periodic blob stays small enough to avoid heavy WAL churn.
const logPersistCap = 8000

type logStore struct {
	mu   sync.Mutex
	logs []StoredLog
}

func newLogStore() *logStore { return &logStore{} }

// normalizeLevel collapses assorted level spellings to error|warn|info|debug.
func normalizeLevel(l string) string {
	switch strings.ToLower(strings.TrimSpace(l)) {
	case "error", "err", "fatal", "panic", "crit", "critical", "emerg", "alert":
		return "error"
	case "warn", "warning":
		return "warn"
	case "debug", "trace":
		return "debug"
	default:
		return "info"
	}
}

func (ls *logStore) ingest(hostID, hostname string, lines []shared.LogLine) {
	if len(lines) == 0 {
		return
	}
	ls.mu.Lock()
	for _, l := range lines {
		msg := l.Message
		if len(msg) > 4000 {
			msg = msg[:4000]
		}
		ls.logs = append(ls.logs, StoredLog{
			Ts: l.Ts, HostID: hostID, Hostname: hostname,
			Source: l.Source, Level: normalizeLevel(l.Level), Message: msg,
		})
	}
	if len(ls.logs) > logStoreCap {
		ls.logs = ls.logs[len(ls.logs)-logStoreCap:]
	}
	ls.mu.Unlock()
}

// search returns matching logs newest-first (up to limit).
func (ls *logStore) search(hostID, level, keyword string, since int64, limit int) []StoredLog {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	kw := strings.ToLower(strings.TrimSpace(keyword))
	ls.mu.Lock()
	defer ls.mu.Unlock()
	out := make([]StoredLog, 0, limit)
	for i := len(ls.logs) - 1; i >= 0 && len(out) < limit; i-- {
		l := ls.logs[i]
		if hostID != "" && l.HostID != hostID {
			continue
		}
		if level != "" && l.Level != level {
			continue
		}
		if since > 0 && l.Ts < since {
			continue
		}
		if kw != "" && !strings.Contains(strings.ToLower(l.Message), kw) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// recentErrors returns up to limit error/warn lines since a timestamp (AI input).
func (ls *logStore) recentErrors(since int64, limit int) []StoredLog {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	out := []StoredLog{}
	for i := len(ls.logs) - 1; i >= 0 && len(out) < limit; i-- {
		l := ls.logs[i]
		if l.Ts < since {
			continue
		}
		if l.Level == "error" || l.Level == "warn" {
			out = append(out, l)
		}
	}
	return out
}

// errorCount counts error lines since a timestamp (for AI/UI).
func (ls *logStore) errorCount(since int64) int {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	n := 0
	for _, l := range ls.logs {
		if l.Ts >= since && l.Level == "error" {
			n++
		}
	}
	return n
}

func (ls *logStore) count() int {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return len(ls.logs)
}

// export returns a snapshot copy of the most recent logPersistCap lines for PG
// persistence (chronological order).
func (ls *logStore) export() []StoredLog {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	src := ls.logs
	if len(src) > logPersistCap {
		src = src[len(src)-logPersistCap:]
	}
	out := make([]StoredLog, len(src))
	copy(out, src)
	return out
}

// importLogs restores the log buffer from PG on startup (capped to logStoreCap).
func (ls *logStore) importLogs(logs []StoredLog) {
	if len(logs) == 0 {
		return
	}
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if len(logs) > logStoreCap {
		logs = logs[len(logs)-logStoreCap:]
	}
	ls.logs = logs
}
