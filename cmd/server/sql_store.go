package main

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// MySQLConnection is a SQL toolkit endpoint (MySQL by default; Driver=postgres for read-only PG).
// Database is optional for MySQL: leave empty to work across all schemas on the instance.
type MySQLConnection struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Env         string `json:"env,omitempty"` // prod|staging|dev; empty is treated as prod
	Enabled     bool   `json:"enabled"`
	Driver      string `json:"driver,omitempty"` // mysql|postgres (default mysql)
	Host        string `json:"host"`
	Port        int    `json:"port,omitempty"`
	User        string `json:"user,omitempty"`
	Password    string `json:"password,omitempty"`     // encrypted / masked
	Database    string `json:"database,omitempty"`     // optional default schema
	TLS         string `json:"tls,omitempty"`          // true|false|skip-verify|preferred
	Params      string `json:"params,omitempty"`       // extra DSN query, e.g. charset=utf8mb4
	VersionHint string `json:"version_hint,omitempty"` // mysql57|mysql80|auto
	CreatedAt   int64  `json:"created_at,omitempty"`

	// SlowSQL configures automatic multi-database slow-query digests + rule advice (MySQL only).
	SlowSQL *SlowSQLMonitorConfig `json:"slow_sql,omitempty"`
}

// SlowSQLMonitorConfig controls scheduled collection from performance_schema.
type SlowSQLMonitorConfig struct {
	Enabled         bool              `json:"enabled"`
	Schedule        *PlaybookSchedule `json:"schedule,omitempty"`
	TopN            int               `json:"top_n,omitempty"`
	MinAvgLatencyMs float64           `json:"min_avg_latency_ms,omitempty"`
	ExcludeSchemas  []string          `json:"exclude_schemas,omitempty"`
	// AlertDisabled opts out of notifier/incident on slow-SQL reports (default: alerts on).
	AlertDisabled        bool    `json:"alert_disabled,omitempty"`
	AlertMinAvgLatencyMs float64 `json:"alert_min_avg_latency_ms,omitempty"`
}

func defaultSlowSQLExcludeSchemas() []string {
	return []string{"mysql", "information_schema", "performance_schema", "sys"}
}

func defaultSlowSQLMonitor() *SlowSQLMonitorConfig {
	return &SlowSQLMonitorConfig{
		Enabled: true,
		Schedule: &PlaybookSchedule{
			Enabled: true,
			Kind:    "daily",
			At:      "03:00",
		},
		TopN:            30,
		MinAvgLatencyMs: 100,
		ExcludeSchemas:  defaultSlowSQLExcludeSchemas(),
	}
}

func (c *SlowSQLMonitorConfig) withDefaults() *SlowSQLMonitorConfig {
	if c == nil {
		return defaultSlowSQLMonitor()
	}
	out := *c
	if out.TopN <= 0 {
		out.TopN = 30
	}
	if out.TopN > 100 {
		out.TopN = 100
	}
	if out.MinAvgLatencyMs <= 0 {
		out.MinAvgLatencyMs = 100
	}
	if len(out.ExcludeSchemas) == 0 {
		out.ExcludeSchemas = defaultSlowSQLExcludeSchemas()
	}
	if out.Schedule == nil {
		out.Schedule = &PlaybookSchedule{Enabled: true, Kind: "daily", At: "03:00"}
	} else if out.Schedule.Kind == "" && out.Schedule.Enabled {
		out.Schedule.Kind = "daily"
		if out.Schedule.At == "" {
			out.Schedule.At = "03:00"
		}
	}
	return &out
}

func maskMySQLConnection(c MySQLConnection) MySQLConnection {
	if c.Password != "" {
		c.Password = "****"
	}
	return c
}

func (cs *ConfigStore) ListMySQLConnections() []MySQLConnection {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make([]MySQLConnection, 0, len(cs.cfg.MySQLConnections))
	out = append(out, cs.cfg.MySQLConnections...)
	return out
}

func (cs *ConfigStore) GetMySQLConnection(id string) (MySQLConnection, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, c := range cs.cfg.MySQLConnections {
		if c.ID == id {
			return c, true
		}
	}
	return MySQLConnection{}, false
}

// mysqlConnReady returns a distinct error when the connection is missing or disabled.
func mysqlConnReady(c MySQLConnection, ok bool) error {
	if !ok {
		return fmt.Errorf("连接不存在：请重新选择数据库连接（慢 SQL 填入后若丢失，请在工作台顶部重选连接）")
	}
	if !c.Enabled {
		return fmt.Errorf("连接已停用：请在「连接管理」中启用该连接后再 EXPLAIN")
	}
	return nil
}

func (cs *ConfigStore) UpsertMySQLConnection(in MySQLConnection) (MySQLConnection, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Host = strings.TrimSpace(in.Host)
	in.Env = strings.ToLower(strings.TrimSpace(in.Env))
	in.Database = strings.TrimSpace(in.Database)
	in.Driver = driverOf(in)
	if in.Name == "" {
		return MySQLConnection{}, fmt.Errorf("name required")
	}
	if in.Host == "" {
		return MySQLConnection{}, fmt.Errorf("host required")
	}
	if in.Env != "" && in.Env != "prod" && in.Env != "staging" && in.Env != "dev" {
		return MySQLConnection{}, fmt.Errorf("env must be prod, staging or dev")
	}
	if in.Port <= 0 {
		if in.Driver == "postgres" {
			in.Port = 5432
		} else {
			in.Port = 3306
		}
	}
	if in.VersionHint == "" {
		in.VersionHint = "auto"
	}
	keepSecret := func(v, prev string) string {
		if v == "" || strings.Contains(v, "****") {
			return prev
		}
		return v
	}
	normalizeSlow := func(cur, prev *SlowSQLMonitorConfig) (*SlowSQLMonitorConfig, error) {
		if cur == nil {
			if prev != nil {
				return prev, nil
			}
			cur = defaultSlowSQLMonitor()
		} else {
			cur = cur.withDefaults()
		}
		if err := sanitizeSchedule(cur.Schedule); err != nil {
			return nil, fmt.Errorf("slow_sql.schedule: %w", err)
		}
		return cur, nil
	}
	cs.mu.Lock()
	if in.ID == "" {
		in.ID = termID()[:8]
		in.CreatedAt = time.Now().Unix()
		ss, err := normalizeSlow(in.SlowSQL, nil)
		if err != nil {
			cs.mu.Unlock()
			return MySQLConnection{}, err
		}
		in.SlowSQL = ss
		cs.cfg.MySQLConnections = append(cs.cfg.MySQLConnections, in)
		cs.mu.Unlock()
		return in, cs.save()
	}
	for i, c := range cs.cfg.MySQLConnections {
		if c.ID == in.ID {
			in.CreatedAt = c.CreatedAt
			in.Password = keepSecret(in.Password, c.Password)
			ss, err := normalizeSlow(in.SlowSQL, c.SlowSQL)
			if err != nil {
				cs.mu.Unlock()
				return MySQLConnection{}, err
			}
			in.SlowSQL = ss
			cs.cfg.MySQLConnections[i] = in
			cs.mu.Unlock()
			return in, cs.save()
		}
	}
	if in.CreatedAt == 0 {
		in.CreatedAt = time.Now().Unix()
	}
	ss, err := normalizeSlow(in.SlowSQL, nil)
	if err != nil {
		cs.mu.Unlock()
		return MySQLConnection{}, err
	}
	in.SlowSQL = ss
	cs.cfg.MySQLConnections = append(cs.cfg.MySQLConnections, in)
	cs.mu.Unlock()
	return in, cs.save()
}

// migrateMySQLSlowSQLDefaultsOnce enables SlowSQL monitor on legacy connections
// that predate the feature (SlowSQL == nil). Runs once per process start; persists when changed.
func (cs *ConfigStore) migrateMySQLSlowSQLDefaultsOnce() bool {
	cs.mu.Lock()
	changed := false
	for i := range cs.cfg.MySQLConnections {
		c := &cs.cfg.MySQLConnections[i]
		if driverOf(*c) == "postgres" {
			if c.SlowSQL != nil {
				c.SlowSQL = nil
				changed = true
			}
			continue
		}
		if c.SlowSQL != nil {
			continue
		}
		c.SlowSQL = defaultSlowSQLMonitor()
		changed = true
	}
	cs.mu.Unlock()
	if changed {
		_ = cs.save()
		slog.Info("mysql slow sql defaults migrated for legacy connections")
	}
	return changed
}

func (cs *ConfigStore) DeleteMySQLConnection(id string) error {
	cs.mu.Lock()
	kept := make([]MySQLConnection, 0, len(cs.cfg.MySQLConnections))
	found := false
	for _, c := range cs.cfg.MySQLConnections {
		if c.ID == id {
			found = true
			continue
		}
		kept = append(kept, c)
	}
	if !found {
		cs.mu.Unlock()
		return fmt.Errorf("connection not found")
	}
	cs.cfg.MySQLConnections = kept
	cs.mu.Unlock()
	return cs.save()
}
