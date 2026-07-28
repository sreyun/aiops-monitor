package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// applyPGPoolSettings configures the sql.DB pool from env (defaults match historical values).
func applyPGPoolSettings(db interface {
	SetMaxOpenConns(int)
	SetMaxIdleConns(int)
	SetConnMaxLifetime(time.Duration)
	SetConnMaxIdleTime(time.Duration)
}) {
	maxOpen := envIntDefault("AIOPS_PG_MAX_OPEN", 200)
	maxIdle := envIntDefault("AIOPS_PG_MAX_IDLE", 50)
	lifeMin := envIntDefault("AIOPS_PG_CONN_LIFE_MIN", 30)
	idleMin := envIntDefault("AIOPS_PG_CONN_IDLE_MIN", 5)
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(time.Duration(lifeMin) * time.Minute)
	db.SetConnMaxIdleTime(time.Duration(idleMin) * time.Minute)
}

func envIntDefault(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// ensureTSPartitions creates monthly range partitions for a parent table
// partitioned by BIGINT unix ts. Parent must already exist.
func (p *pgStore) ensureTSPartitions(parent string, monthsAhead int) {
	if p == nil || p.db == nil || parent == "" {
		return
	}
	if monthsAhead < 1 {
		monthsAhead = 2
	}
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	for i := 0; i < monthsAhead+2; i++ {
		mStart := start.AddDate(0, i, 0)
		mEnd := mStart.AddDate(0, 1, 0)
		name := fmt.Sprintf("%s_%04d%02d", parent, mStart.Year(), int(mStart.Month()))
		if !isSafePartitionName(name, parent) {
			continue
		}
		fromTS, toTS := mStart.Unix(), mEnd.Unix()
		ddl := fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM (%d) TO (%d)`,
			quoteIdent(name), quoteIdent(parent), fromTS, toTS)
		if _, err := p.db.Exec(ddl); err != nil {
			slog.Debug("ensure partition", "table", name, "err", err)
		}
	}
}

func isSafePartitionName(name, parent string) bool {
	if !strings.HasPrefix(name, parent+"_") {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}

// migrateDualTrackPartitions creates partitioned twin tables for audit/events/ai_call.
func (p *pgStore) migrateDualTrackPartitions() {
	if p == nil || p.db == nil {
		return
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS audit_log_p (
			id BIGSERIAL,
			ts BIGINT NOT NULL,
			data JSONB NOT NULL,
			content_hash TEXT NOT NULL DEFAULT '',
			prev_hash TEXT NOT NULL DEFAULT '',
			chain_seq BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (id, ts)
		) PARTITION BY RANGE (ts)`,
		`CREATE TABLE IF NOT EXISTS audit_log_p_default PARTITION OF audit_log_p DEFAULT`,
		`CREATE INDEX IF NOT EXISTS audit_log_p_ts ON audit_log_p(ts DESC)`,
		`CREATE TABLE IF NOT EXISTS events_p (
			id BIGSERIAL,
			ts BIGINT NOT NULL,
			data JSONB NOT NULL,
			PRIMARY KEY (id, ts)
		) PARTITION BY RANGE (ts)`,
		`CREATE TABLE IF NOT EXISTS events_p_default PARTITION OF events_p DEFAULT`,
		`CREATE INDEX IF NOT EXISTS events_p_ts ON events_p(ts DESC)`,
		`CREATE TABLE IF NOT EXISTS ai_call_events_p (
			id BIGSERIAL,
			ts BIGINT NOT NULL,
			task TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			actor TEXT NOT NULL DEFAULT '',
			latency_ms BIGINT NOT NULL DEFAULT 0,
			ok BOOLEAN NOT NULL DEFAULT TRUE,
			error TEXT DEFAULT '',
			memory_hits INT DEFAULT 0,
			skill_hits INT DEFAULT 0,
			reply_chars INT DEFAULT 0,
			approx_tokens INT DEFAULT 0,
			prompt_tokens INT DEFAULT 0,
			completion_tokens INT DEFAULT 0,
			cost_estimate DOUBLE PRECISION DEFAULT 0,
			PRIMARY KEY (id, ts)
		) PARTITION BY RANGE (ts)`,
		`CREATE TABLE IF NOT EXISTS ai_call_events_p_default PARTITION OF ai_call_events_p DEFAULT`,
		`CREATE INDEX IF NOT EXISTS ai_call_events_p_ts ON ai_call_events_p(ts DESC)`,
		// hash-chain columns on legacy audit_log
		`ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS content_hash TEXT DEFAULT ''`,
		`ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS prev_hash TEXT DEFAULT ''`,
		`ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS chain_seq BIGINT DEFAULT 0`,
	}
	for _, s := range stmts {
		if _, err := p.db.Exec(s); err != nil {
			slog.Warn("dual-track partition migrate", "err", err)
		}
	}
	p.ensureTSPartitions("audit_log_p", 3)
	p.ensureTSPartitions("events_p", 3)
	p.ensureTSPartitions("ai_call_events_p", 3)
}
