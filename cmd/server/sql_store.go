package main

import (
	"fmt"
	"strings"
	"time"
)

// MySQLConnection is a read-only MySQL endpoint for EXPLAIN / schema tooling.
type MySQLConnection struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Env         string `json:"env,omitempty"` // prod|staging|dev; empty is treated as prod
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port,omitempty"`
	User        string `json:"user,omitempty"`
	Password    string `json:"password,omitempty"` // encrypted / masked
	Database    string `json:"database,omitempty"`
	TLS         string `json:"tls,omitempty"`          // true|false|skip-verify|preferred
	Params      string `json:"params,omitempty"`       // extra DSN query, e.g. charset=utf8mb4
	VersionHint string `json:"version_hint,omitempty"` // mysql57|mysql80|auto
	CreatedAt   int64  `json:"created_at,omitempty"`
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

func (cs *ConfigStore) UpsertMySQLConnection(in MySQLConnection) (MySQLConnection, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Host = strings.TrimSpace(in.Host)
	in.Env = strings.ToLower(strings.TrimSpace(in.Env))
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
		in.Port = 3306
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
	cs.mu.Lock()
	if in.ID == "" {
		in.ID = termID()[:8]
		in.CreatedAt = time.Now().Unix()
		cs.cfg.MySQLConnections = append(cs.cfg.MySQLConnections, in)
		cs.mu.Unlock()
		return in, cs.save()
	}
	for i, c := range cs.cfg.MySQLConnections {
		if c.ID == in.ID {
			in.CreatedAt = c.CreatedAt
			in.Password = keepSecret(in.Password, c.Password)
			cs.cfg.MySQLConnections[i] = in
			cs.mu.Unlock()
			return in, cs.save()
		}
	}
	if in.CreatedAt == 0 {
		in.CreatedAt = time.Now().Unix()
	}
	cs.cfg.MySQLConnections = append(cs.cfg.MySQLConnections, in)
	cs.mu.Unlock()
	return in, cs.save()
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
