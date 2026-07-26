package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aiops-monitor/cmd/server/sqltoolkit"
)

func (s *Server) handleSQLBeautify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SQL     string `json:"sql"`
		Dialect string `json:"dialect"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	d := sqltoolkit.NormalizeDialect(req.Dialect)
	writeJSON(w, http.StatusOK, map[string]any{
		"sql":     sqltoolkit.Beautify(req.SQL, d),
		"dialect": d,
	})
}

func (s *Server) handleSQLAudit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SQL     string `json:"sql"`
		Dialect string `json:"dialect"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	d := sqltoolkit.NormalizeDialect(req.Dialect)
	res := sqltoolkit.Audit(req.SQL, d)
	writeJSON(w, http.StatusOK, map[string]any{
		"findings": res.Findings,
		"score":    res.Score,
		"dialect":  d,
	})
}

func (s *Server) handleSQLOptimize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SQL     string `json:"sql"`
		Dialect string `json:"dialect"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	d := sqltoolkit.NormalizeDialect(req.Dialect)
	res := sqltoolkit.Optimize(req.SQL, d)
	writeJSON(w, http.StatusOK, map[string]any{
		"rewritten_sql": res.RewrittenSQL,
		"suggestions":   res.Suggestions,
		"index_hints":   res.IndexHints,
		"dialect":       d,
	})
}

// handleSQLAnalyze: Vitess AST + optional metadata + EXPLAIN composite scoring.
func (s *Server) handleSQLAnalyze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SQL          string `json:"sql"`
		Dialect      string `json:"dialect"`
		ConnectionID string `json:"connection_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	d := sqltoolkit.NormalizeDialect(req.Dialect)
	in := sqltoolkit.AnalyzeInput{SQL: req.SQL, Dialect: d}

	connID := strings.TrimSpace(req.ConnectionID)
	if connID != "" {
		c, ok := s.cfg.GetMySQLConnection(connID)
		if !ok || !c.Enabled {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection not found or disabled"})
			return
		}
		if driverOf(c) == "postgres" {
			d = sqltoolkit.DialectPostgres
			in.Dialect = d
			kw := sqltoolkit.FirstKeyword(req.SQL)
			if kw == "select" || kw == "with" || kw == "explain" || kw == "values" {
				if expl, err := pgExplain(c, req.SQL); err == nil {
					if a, ok := expl["analysis"].(*sqltoolkit.ExplainAnalysis); ok {
						in.Explain = a
					}
				}
			}
		} else {
			if c.VersionHint == "mysql57" {
				d = sqltoolkit.DialectMySQL57
				in.Dialect = d
			} else if c.VersionHint == "mysql80" {
				d = sqltoolkit.DialectMySQL80
				in.Dialect = d
			}
			shape := sqltoolkit.ExtractQueryShape(req.SQL)
			if shape != nil && shape.ParseOK {
				if meta, err := mysqlFetchMetadata(c, shape.TableNames()); err == nil {
					in.Meta = meta
				}
			}
			kw := sqltoolkit.FirstKeyword(req.SQL)
			if kw == "select" || kw == "with" || kw == "explain" {
				if expl, err := mysqlExplain(c, req.SQL); err == nil {
					if a, ok := expl["analysis"].(*sqltoolkit.ExplainAnalysis); ok {
						in.Explain = a
					}
				}
			}
		}
	}

	res := sqltoolkit.Analyze(in)
	if strings.TrimSpace(req.SQL) != "" {
		sc := res.Score
		s.recordSQLHistory(r, "analyze", connID, req.SQL, &sc)
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleListMySQLConnections(w http.ResponseWriter, r *http.Request) {
	list := s.cfg.ListMySQLConnections()
	out := make([]MySQLConnection, 0, len(list))
	for _, c := range list {
		out = append(out, maskMySQLConnection(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": out})
}

func (s *Server) handleGetMySQLConnection(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	c, ok := s.cfg.GetMySQLConnection(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, maskMySQLConnection(c))
}

func (s *Server) handleUpsertMySQLConnection(w http.ResponseWriter, r *http.Request) {
	var in MySQLConnection
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if id := strings.TrimSpace(r.PathValue("id")); id != "" {
		in.ID = id
	}
	saved, err := s.cfg.UpsertMySQLConnection(in)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: "更新 MySQL 连接配置「" + saved.Name + "」"})
	writeJSON(w, http.StatusOK, maskMySQLConnection(saved))
}

func (s *Server) handleDeleteMySQLConnection(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	c, ok := s.cfg.GetMySQLConnection(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err := s.cfg.DeleteMySQLConnection(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: "删除 MySQL 连接配置「" + c.Name + "」"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleTestMySQLConnection(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	c, ok := s.cfg.GetMySQLConnection(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	var (
		ver string
		err error
	)
	if driverOf(c) == "postgres" {
		ver, err = pgPing(c)
	} else {
		ver, err = mysqlTestConnection(c)
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": ver, "driver": driverOf(c)})
}

func (s *Server) handleMySQLExplain(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	c, ok := s.cfg.GetMySQLConnection(id)
	if !ok || !c.Enabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found or disabled"})
		return
	}
	var req struct {
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if driverOf(c) == "postgres" {
		res, err := pgExplain(c, req.SQL)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
		return
	}
	res, err := mysqlExplain(c, req.SQL)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleMySQLSchema(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	c, ok := s.cfg.GetMySQLConnection(id)
	if !ok || !c.Enabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found or disabled"})
		return
	}
	database := strings.TrimSpace(r.URL.Query().Get("database"))
	if database == "" {
		database = strings.TrimSpace(r.URL.Query().Get("schema"))
	}
	table := strings.TrimSpace(r.URL.Query().Get("table"))
	if driverOf(c) == "postgres" {
		res, err := pgSchema(c, database, table)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
		return
	}
	res, err := mysqlSchema(c, database, table)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleSlowSQLRun(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection id required"})
		return
	}
	rep, err := s.runSlowSQLCollect(id, "manual")
	if err != nil {
		if strings.Contains(err.Error(), "正在进行中") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		if rep == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		// Collection failed but report persisted — return body for UI.
	}
	if rep != nil {
		s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.actorName(r), IP: s.clientIP(r),
			Message: fmt.Sprintf("慢 SQL 检查 conn=%s status=%s items=%d", rep.ConnectionName, rep.Status, rep.ItemCount)})
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleSlowSQLLatest(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if s.sqlSlow == nil {
		writeJSON(w, http.StatusOK, map[string]any{"report": nil})
		return
	}
	rep := s.sqlSlow.getLatest(id)
	if rep == nil {
		writeJSON(w, http.StatusOK, map[string]any{"report": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"report": rep})
}

func (s *Server) handleSlowSQLReports(w http.ResponseWriter, r *http.Request) {
	if s.sqlSlow == nil {
		writeJSON(w, http.StatusOK, map[string]any{"reports": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": s.sqlSlow.listLatest()})
}

// handleMySQLExecDDL executes a narrowly allowed index DDL. Production (and
// legacy connections with empty env) must carry an approved one-time ticket.
func (s *Server) handleMySQLExecDDL(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	c, ok := s.cfg.GetMySQLConnection(id)
	if !ok || !c.Enabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found or disabled"})
		return
	}
	var req struct {
		SQL        string `json:"sql"`
		TimeoutSec int    `json:"timeout_sec"`
		AllowExec  bool   `json:"allow_exec"`
		TicketID   string `json:"ticket_id"`
		VerifySQL  string `json:"verify_sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if driverOf(c) == "postgres" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "PostgreSQL 连接仅支持只读操作"})
		return
	}
	sqlText := strings.TrimSpace(req.SQL)
	if sqlText == "" || len(sqlText) > 16<<10 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sql 无效或过长"})
		return
	}
	if !sqltoolkit.IsAllowedIndexDDL(sqlText) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "仅允许 CREATE/ALTER 索引类 DDL"})
		return
	}
	if win, frozen := s.cfg.activeFreezeWindow("", "sql", time.Now().Unix()); frozen {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": fmt.Sprintf("当前处于 SQL 变更冻结窗「%s」，禁止直接执行 DDL", win.Name),
		})
		return
	}
	if ticketID := strings.TrimSpace(req.TicketID); ticketID != "" {
		s.executeSQLChangeRequest(w, r, ticketID, c.ID, sqlText, req.TimeoutSec, req.VerifySQL)
		return
	}
	if sqlConnectionEnv(c) == "prod" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "生产环境 DDL 必须提交、审批并使用变更单执行"})
		return
	}
	if !req.AllowExec {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "必须显式 allow_exec=true 才可执行 DDL"})
		return
	}
	res, err := s.execDDLWithExplain(c, sqlText, req.VerifySQL, req.TimeoutSec)
	if err != nil {
		s.store.AddLog(LogEntry{Kind: KindOperation, Level: "error", Actor: s.actorName(r), IP: s.clientIP(r),
			Message: fmt.Sprintf("SQL DDL 失败 conn=%s: %v", c.Name, err)})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("SQL DDL 执行 conn=%s sql=%s", c.Name, truncateRunes(sqlText, 200))})
	writeJSON(w, http.StatusOK, res)
}
