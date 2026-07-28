package main

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type pgSlowEntry struct {
	SQL      string `json:"sql"`
	Duration int64  `json:"duration_ms"`
	At       int64  `json:"at"`
}

var (
	pgSlowMu   sync.Mutex
	pgSlowRing []pgSlowEntry
	pgSlowCap  = 50
)

func pgSlowThresholdMS() int64 {
	v := strings.TrimSpace(os.Getenv("AIOPS_PG_SLOW_MS"))
	if v == "" {
		return 500
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 500
	}
	return n
}

// observePGSlow records queries slower than AIOPS_PG_SLOW_MS (default 500).
func observePGSlow(label string, start time.Time) {
	ms := time.Since(start).Milliseconds()
	th := pgSlowThresholdMS()
	if th <= 0 || ms < th {
		return
	}
	if len(label) > 160 {
		label = label[:160] + "…"
	}
	slog.Warn("pg slow query", "sql", label, "ms", ms)
	e := pgSlowEntry{SQL: label, Duration: ms, At: time.Now().Unix()}
	pgSlowMu.Lock()
	pgSlowRing = append(pgSlowRing, e)
	if len(pgSlowRing) > pgSlowCap {
		pgSlowRing = pgSlowRing[len(pgSlowRing)-pgSlowCap:]
	}
	pgSlowMu.Unlock()
}

func recentPGSlowQueries(limit int) []pgSlowEntry {
	if limit <= 0 || limit > pgSlowCap {
		limit = pgSlowCap
	}
	pgSlowMu.Lock()
	defer pgSlowMu.Unlock()
	n := len(pgSlowRing)
	if n == 0 {
		return []pgSlowEntry{}
	}
	if limit > n {
		limit = n
	}
	out := make([]pgSlowEntry, limit)
	copy(out, pgSlowRing[n-limit:])
	return out
}

func (s *Server) handlePGSlowQueries(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	writeJSON(w, http.StatusOK, map[string]any{
		"threshold_ms": pgSlowThresholdMS(),
		"recent":       recentPGSlowQueries(limit),
	})
}

func (s *Server) handleNetflowQueueStats(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{}
	if s.pg != nil {
		out = s.pg.netflowQueueStats()
	}
	writeJSON(w, http.StatusOK, out)
}
