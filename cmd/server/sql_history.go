package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"aiops-monitor/cmd/server/sqltoolkit"
)

const sqlHistoryMaxPerUser = 50

var (
	reSQLStringSingle = regexp.MustCompile(`'([^']|\\')*'`)
	reSQLStringDouble = regexp.MustCompile(`"([^"]|\\")*"`)
	reSQLLongNumber   = regexp.MustCompile(`\b\d{4,}\b`)
)

// SQLQueryHistoryEntry is one desensitized analyze/query record for a user.
type SQLQueryHistoryEntry struct {
	ID           string `json:"id"`
	User         string `json:"user"`
	ConnectionID string `json:"connection_id,omitempty"`
	Kind         string `json:"kind"` // analyze|audit|explain|optimize|beautify
	SQL          string `json:"sql"`
	Score        *int   `json:"score,omitempty"`
	CreatedAt    int64  `json:"created_at"`
}

type sqlQueryHistoryManager struct {
	mu     sync.Mutex
	byUser map[string][]SQLQueryHistoryEntry
	dir    string
}

func newSQLQueryHistoryManager(dir string) *sqlQueryHistoryManager {
	m := &sqlQueryHistoryManager{byUser: map[string][]SQLQueryHistoryEntry{}, dir: dir}
	m.load()
	return m
}

func (m *sqlQueryHistoryManager) path() string {
	return filepath.Join(m.dir, "sql_query_history.json")
}

func (m *sqlQueryHistoryManager) load() {
	if m.dir == "" {
		return
	}
	b, err := os.ReadFile(m.path())
	if err != nil {
		return
	}
	var raw map[string][]SQLQueryHistoryEntry
	if json.Unmarshal(b, &raw) != nil {
		return
	}
	m.byUser = raw
}

func (m *sqlQueryHistoryManager) saveLocked() {
	if m.dir == "" {
		return
	}
	_ = os.MkdirAll(m.dir, 0o750)
	b, err := json.Marshal(m.byUser)
	if err != nil {
		return
	}
	tmp := m.path() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, m.path())
}

func desensitizeSQL(sql string) string {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return ""
	}
	sql = reSQLStringSingle.ReplaceAllString(sql, "'?'")
	sql = reSQLStringDouble.ReplaceAllString(sql, `"?"`)
	sql = reSQLLongNumber.ReplaceAllString(sql, "?")
	return truncateRunes(sql, 500)
}

func (m *sqlQueryHistoryManager) append(user, kind, connID, sqlText string, score *int) SQLQueryHistoryEntry {
	user = strings.TrimSpace(user)
	if user == "" || strings.TrimSpace(sqlText) == "" {
		return SQLQueryHistoryEntry{}
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "analyze"
	}
	entry := SQLQueryHistoryEntry{
		ID:           termID()[:12],
		User:         user,
		ConnectionID: strings.TrimSpace(connID),
		Kind:         kind,
		SQL:          desensitizeSQL(sqlText),
		Score:        score,
		CreatedAt:    time.Now().Unix(),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	list := append([]SQLQueryHistoryEntry{entry}, m.byUser[user]...)
	if len(list) > sqlHistoryMaxPerUser {
		list = list[:sqlHistoryMaxPerUser]
	}
	m.byUser[user] = list
	m.saveLocked()
	return entry
}

func (m *sqlQueryHistoryManager) list(user string, limit int) []SQLQueryHistoryEntry {
	user = strings.TrimSpace(user)
	if limit <= 0 {
		limit = sqlHistoryMaxPerUser
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	src := m.byUser[user]
	if len(src) > limit {
		src = src[:limit]
	}
	out := make([]SQLQueryHistoryEntry, len(src))
	copy(out, src)
	return out
}

func buildExplainDiffFromAnalysis(before, after *sqltoolkit.ExplainAnalysis) map[string]any {
	toMap := func(a *sqltoolkit.ExplainAnalysis) map[string]sqltoolkit.ExplainHit {
		out := map[string]sqltoolkit.ExplainHit{}
		if a == nil {
			return out
		}
		for _, h := range a.TableAccess {
			t := strings.TrimSpace(h.Table)
			if t != "" {
				out[t] = h
			}
		}
		return out
	}
	bm, am := toMap(before), toMap(after)
	tables := map[string]bool{}
	for t := range bm {
		tables[t] = true
	}
	for t := range am {
		tables[t] = true
	}
	names := make([]string, 0, len(tables))
	for t := range tables {
		names = append(names, t)
	}
	sort.Strings(names)
	type changeRow struct {
		Table  string `json:"table"`
		Field  string `json:"field"`
		Before string `json:"before,omitempty"`
		After  string `json:"after,omitempty"`
	}
	var changes []changeRow
	improved, regressed := 0, 0
	for _, t := range names {
		b, a := bm[t], am[t]
		add := func(field, bv, av string) {
			if bv == av {
				return
			}
			changes = append(changes, changeRow{Table: t, Field: field, Before: bv, After: av})
		}
		add("access_type", b.AccessType, a.AccessType)
		add("key", b.Key, a.Key)
		add("rows", fmt.Sprintf("%v", b.Rows), fmt.Sprintf("%v", a.Rows))
		add("full_scan_risk", fmt.Sprintf("%v", b.FullScanRisk), fmt.Sprintf("%v", a.FullScanRisk))
		if b.FullScanRisk && !a.FullScanRisk {
			improved++
		}
		if !b.FullScanRisk && a.FullScanRisk {
			regressed++
		}
		if (b.Key == "" || b.Key == "null") && a.Key != "" && a.Key != "null" {
			improved++
		}
	}
	summary := "EXPLAIN 对比："
	if len(changes) == 0 {
		summary += "执行计划无可见变化"
	} else {
		summary += fmt.Sprintf("%d 处差异", len(changes))
		if improved > 0 {
			summary += fmt.Sprintf("；改善信号 %d", improved)
		}
		if regressed > 0 {
			summary += fmt.Sprintf("；退化信号 %d", regressed)
		}
	}
	return map[string]any{
		"summary": summary, "changes": changes,
		"improved": improved, "regressed": regressed,
	}
}

func (s *Server) execDDLWithExplain(c MySQLConnection, ddl, verifySQL string, timeoutSec int) (map[string]any, error) {
	verifySQL = strings.TrimSpace(verifySQL)
	result := map[string]any{}
	var before *sqltoolkit.ExplainAnalysis
	if verifySQL != "" {
		if expl, err := mysqlExplain(c, verifySQL); err == nil {
			if a, ok := expl["analysis"].(*sqltoolkit.ExplainAnalysis); ok {
				before = a
				result["explain_before"] = a
			}
		} else {
			result["explain_before_error"] = err.Error()
		}
	}
	ddlResult, err := mysqlExecDDL(c, ddl, timeoutSec)
	if err != nil {
		return result, err
	}
	for k, v := range ddlResult {
		result[k] = v
	}
	if verifySQL != "" {
		if expl, err := mysqlExplain(c, verifySQL); err == nil {
			var after *sqltoolkit.ExplainAnalysis
			if a, ok := expl["analysis"].(*sqltoolkit.ExplainAnalysis); ok {
				after = a
				result["explain_after"] = a
			}
			result["explain_diff"] = buildExplainDiffFromAnalysis(before, after)
		} else {
			result["explain_after_error"] = err.Error()
		}
	}
	return result, nil
}

func (s *Server) recordSQLHistory(r *http.Request, kind, connID, sqlText string, score *int) {
	if s.sqlHistory == nil || strings.TrimSpace(sqlText) == "" {
		return
	}
	s.sqlHistory.append(s.actorName(r), kind, connID, sqlText, score)
}

func (s *Server) handleSQLQueryHistory(w http.ResponseWriter, r *http.Request) {
	if s.sqlHistory == nil {
		writeJSON(w, http.StatusOK, map[string]any{"history": []SQLQueryHistoryEntry{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": s.sqlHistory.list(s.actorName(r), sqlHistoryMaxPerUser)})
}

func (s *Server) handleAppendSQLQueryHistory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind         string `json:"kind"`
		ConnectionID string `json:"connection_id"`
		SQL          string `json:"sql"`
		Score        *int   `json:"score"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if s.sqlHistory == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "history unavailable"})
		return
	}
	entry := s.sqlHistory.append(s.actorName(r), req.Kind, req.ConnectionID, req.SQL, req.Score)
	writeJSON(w, http.StatusCreated, entry)
}
