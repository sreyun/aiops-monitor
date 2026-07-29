package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleSlowSQLPSLimits(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	c, ok := s.cfg.GetMySQLConnection(id)
	if err := mysqlConnReady(c, ok); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if driverOf(c) == "postgres" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "performance_schema 限额仅适用于 MySQL"})
		return
	}
	db, err := mysqlOpenSlow(c)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	md, mt := mysqlPSTextLimits(ctx, db)
	lim := buildSlowSQLPSLimits(md, mt)
	writeJSON(w, http.StatusOK, lim)
}

// handleSlowSQLApplyPSLimits optionally runs SET PERSIST for digest/SQL_TEXT lengths.
// Requires explicit confirm=true; does not restart mysqld.
func (s *Server) handleSlowSQLApplyPSLimits(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	c, ok := s.cfg.GetMySQLConnection(id)
	if err := mysqlConnReady(c, ok); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if driverOf(c) == "postgres" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "仅 MySQL 支持"})
		return
	}
	var req struct {
		Confirm bool `json:"confirm"`
		Target  int  `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请设置 confirm=true 二次确认后再写入"})
		return
	}
	target := req.Target
	if target < 1024 {
		target = 8192
	}
	if target > 65535 {
		target = 65535
	}
	db, err := mysqlOpenSlow(c)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	notes := []string{}
	applyOne := func(name string) {
		q := fmt.Sprintf("SET PERSIST %s = %d", name, target)
		if _, err := db.ExecContext(ctx, q); err != nil {
			q2 := fmt.Sprintf("SET GLOBAL %s = %d", name, target)
			if _, err2 := db.ExecContext(ctx, q2); err2 != nil {
				notes = append(notes, name+": "+err2.Error())
				return
			}
			notes = append(notes, name+": 已 SET GLOBAL（无 PERSIST，重启可能丢失）")
			return
		}
		notes = append(notes, name+": 已 SET PERSIST")
	}
	applyOne("max_digest_length")
	applyOne("performance_schema_max_sql_text_length")
	md, mt := mysqlPSTextLimits(ctx, db)
	lim := buildSlowSQLPSLimits(md, mt)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warn", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("尝试写入 MySQL P_S 文本限额 conn=%s target=%d", c.Name, target)})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      len(notes) > 0,
		"notes":   notes,
		"limits":  lim,
		"restart": "多数版本需重启 mysqld 后 performance_schema 相关长度才会完全生效，然后请重新采集慢 SQL",
	})
}

// pickLongestRecoveredSQL is a pure helper for tests / shared preference logic.
func pickLongestRecoveredSQL(candidates []sqlRecoverCandidate) (sqlRecoverCandidate, bool) {
	var best sqlRecoverCandidate
	for _, c := range candidates {
		text := strings.TrimSpace(c.SQL)
		if text == "" {
			continue
		}
		if best.SQL == "" || len(text) > len(best.SQL) || shouldPreferRecoveredSQL(best.SQL, text) {
			best = sqlRecoverCandidate{SQL: text, Source: c.Source}
		}
	}
	if best.SQL == "" {
		return sqlRecoverCandidate{}, false
	}
	return best, true
}
