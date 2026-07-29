package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aiops-monitor/cmd/server/sqltoolkit"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// sqlWorkbenchQueryResult is the workbench "Run" response: rows + timing split.
type sqlWorkbenchQueryResult struct {
	OK        bool             `json:"ok"`
	Driver    string           `json:"driver"`
	Schema    string           `json:"schema,omitempty"`
	Columns   []string         `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	RowCount  int              `json:"row_count"`
	Limit     int              `json:"limit"`
	Truncated bool             `json:"truncated"`
	ExecMs    int64            `json:"exec_ms"`  // until QueryContext returns (server accepted / started streaming)
	FetchMs   int64            `json:"fetch_ms"` // client-side row fetch / decode
	TotalMs   int64            `json:"total_ms"`
	Error     string           `json:"error,omitempty"`
}

func clampSQLQueryTimeout(sec int) time.Duration {
	if sec <= 0 {
		return 20 * time.Second
	}
	if sec > 60 {
		sec = 60
	}
	return time.Duration(sec) * time.Second
}

// handleSQLWorkbenchQuery runs a single read-only statement and returns rows + timings.
// POST /api/v1/sql/connections/{id}/query
func (s *Server) handleSQLWorkbenchQuery(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	c, ok := s.cfg.GetMySQLConnection(id)
	if err := mysqlConnReady(c, ok); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	var req struct {
		SQL        string `json:"sql"`
		Schema     string `json:"schema"`
		Database   string `json:"database"`
		Limit      int    `json:"limit"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	sqlText := strings.TrimSpace(req.SQL)
	if sqlText == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请先输入 SQL"})
		return
	}
	if sqltoolkit.HasUnboundPlaceholder(sqlText) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "SQL 仍含 ? / $n 占位符：请填入真实参数后再运行（运行不会用探测值替参）",
		})
		return
	}
	schema := strings.TrimSpace(firstNonEmpty(req.Schema, req.Database))
	if schema == "" {
		schema = inferSchemaFromSQLText(sqlText)
	}
	if schema == "" {
		schema = strings.TrimSpace(c.Database)
	}
	limit := clampSQLReadLimit(req.Limit)
	timeout := clampSQLQueryTimeout(req.TimeoutSec)

	var (
		res *sqlWorkbenchQueryResult
		err error
	)
	if driverOf(c) == "postgres" {
		res, err = pgWorkbenchQuery(c, schema, sqlText, limit, timeout)
	} else {
		if schema == "" {
			if dbs, e := mysqlListBusinessDatabases(c); e == nil && len(dbs) == 1 {
				schema = dbs[0]
			}
		}
		res, err = mysqlWorkbenchQuery(c, schema, sqlText, limit, timeout)
	}
	if err != nil {
		body := map[string]any{"error": err.Error(), "ok": false}
		if res != nil {
			if res.ExecMs > 0 {
				body["exec_ms"] = res.ExecMs
			}
			if res.FetchMs > 0 {
				body["fetch_ms"] = res.FetchMs
			}
			if res.TotalMs > 0 {
				body["total_ms"] = res.TotalMs
			}
			if res.Schema != "" {
				body["schema"] = res.Schema
			}
		}
		writeJSON(w, http.StatusBadGateway, body)
		return
	}
	s.recordSQLHistory(r, "query", id, sqlText, nil)
	writeJSON(w, http.StatusOK, res)
}

func mysqlWorkbenchQuery(c MySQLConnection, schema, sqlText string, limit int, timeout time.Duration) (*sqlWorkbenchQueryResult, error) {
	sqlText = strings.TrimSpace(sqlText)
	if !sqltoolkit.IsReadOnlyQuery(sqlText) || sqltoolkit.ForbiddenWrite(sqlText) {
		return nil, fmt.Errorf("仅允许单条只读 SELECT/WITH/SHOW/DESC")
	}
	kw := sqltoolkit.FirstKeyword(sqlText)
	if kw != "select" && kw != "with" && kw != "show" && kw != "desc" && kw != "describe" {
		return nil, fmt.Errorf("仅允许 SELECT/WITH/SHOW/DESC")
	}
	schema = strings.TrimSpace(schema)
	if schema != "" {
		if !reSafeIdent.MatchString(schema) {
			return nil, fmt.Errorf("非法库名")
		}
		c.Database = schema
	}
	execSQL := strings.TrimSuffix(sqlText, ";")
	wrapped := false
	if kw == "select" || kw == "with" {
		if !strings.Contains(strings.ToLower(sqltoolkit.StripCommentsAndStrings(execSQL)), "limit") {
			execSQL = fmt.Sprintf("SELECT * FROM (%s) AS _aiops_q LIMIT %d", execSQL, limit)
			wrapped = true
		}
	}

	totalStart := time.Now()
	db, err := mysqlOpenForRead(c, timeout+2*time.Second)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if schema != "" {
		if _, err := db.ExecContext(ctx, "USE `"+schema+"`"); err != nil {
			return nil, fmt.Errorf("切换库失败：%w", err)
		}
	}

	execStart := time.Now()
	rs, err := db.QueryContext(ctx, execSQL)
	execMs := time.Since(execStart).Milliseconds()
	if err != nil {
		return &sqlWorkbenchQueryResult{ExecMs: execMs, TotalMs: time.Since(totalStart).Milliseconds(), Schema: schema}, err
	}
	defer rs.Close()

	fetchStart := time.Now()
	cols, rows, err := scanSQLResultRows(rs, limit, true)
	fetchMs := time.Since(fetchStart).Milliseconds()
	if err != nil {
		return &sqlWorkbenchQueryResult{
			ExecMs: execMs, FetchMs: fetchMs, TotalMs: time.Since(totalStart).Milliseconds(), Schema: schema,
		}, err
	}
	truncated := len(rows) >= limit || wrapped && len(rows) >= limit
	return &sqlWorkbenchQueryResult{
		OK: true, Driver: "mysql", Schema: schema,
		Columns: cols, Rows: rows, RowCount: len(rows), Limit: limit, Truncated: truncated,
		ExecMs: execMs, FetchMs: fetchMs, TotalMs: time.Since(totalStart).Milliseconds(),
	}, nil
}

func pgWorkbenchQuery(c MySQLConnection, schema, sqlText string, limit int, timeout time.Duration) (*sqlWorkbenchQueryResult, error) {
	sqlText = strings.TrimSpace(sqlText)
	if !sqltoolkit.IsReadOnlyQuery(sqlText) || sqltoolkit.ForbiddenWrite(sqlText) {
		return nil, fmt.Errorf("仅允许单条只读 SELECT/WITH/VALUES")
	}
	kw := sqltoolkit.FirstKeyword(sqlText)
	if kw != "select" && kw != "with" && kw != "values" && kw != "show" && kw != "table" {
		return nil, fmt.Errorf("PostgreSQL 仅允许 SELECT/WITH/VALUES")
	}
	schema = strings.TrimSpace(schema)
	if schema != "" && !reSafeIdent.MatchString(schema) {
		return nil, fmt.Errorf("非法 schema 名")
	}
	execSQL := strings.TrimSuffix(sqlText, ";")
	wrapped := false
	if !strings.Contains(strings.ToLower(sqltoolkit.StripCommentsAndStrings(execSQL)), "limit") {
		execSQL = fmt.Sprintf("SELECT * FROM (%s) AS _aiops_q LIMIT %d", execSQL, limit)
		wrapped = true
	}

	totalStart := time.Now()
	db, err := pgOpen(c)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if schema != "" {
		if _, err := db.ExecContext(ctx, `SELECT set_config('search_path', $1, true)`, schema); err != nil {
			return nil, fmt.Errorf("设置 search_path 失败：%w", err)
		}
	}

	execStart := time.Now()
	rs, err := db.QueryContext(ctx, execSQL)
	execMs := time.Since(execStart).Milliseconds()
	if err != nil {
		return &sqlWorkbenchQueryResult{ExecMs: execMs, TotalMs: time.Since(totalStart).Milliseconds(), Schema: schema, Driver: "postgres"}, err
	}
	defer rs.Close()

	fetchStart := time.Now()
	cols, rows, err := scanSQLResultRows(rs, limit, false)
	fetchMs := time.Since(fetchStart).Milliseconds()
	if err != nil {
		return &sqlWorkbenchQueryResult{
			Driver: "postgres", Schema: schema,
			ExecMs: execMs, FetchMs: fetchMs, TotalMs: time.Since(totalStart).Milliseconds(),
		}, err
	}
	truncated := len(rows) >= limit || (wrapped && len(rows) >= limit)
	return &sqlWorkbenchQueryResult{
		OK: true, Driver: "postgres", Schema: schema,
		Columns: cols, Rows: rows, RowCount: len(rows), Limit: limit, Truncated: truncated,
		ExecMs: execMs, FetchMs: fetchMs, TotalMs: time.Since(totalStart).Milliseconds(),
	}, nil
}

func scanSQLResultRows(rs *sql.Rows, limit int, mysqlStyle bool) ([]string, []map[string]any, error) {
	cols, err := rs.Columns()
	if err != nil {
		return nil, nil, err
	}
	var rows []map[string]any
	for rs.Next() {
		if len(rows) >= limit {
			break
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			continue
		}
		m := make(map[string]any, len(cols))
		for i, col := range cols {
			if mysqlStyle {
				m[col] = stringifySQLVal(vals[i])
			} else {
				m[col] = normalizeSQLValue(vals[i])
			}
		}
		rows = append(rows, m)
	}
	return cols, rows, rs.Err()
}

// mysqlOpenForRead opens MySQL with an extended read timeout for workbench queries.
func mysqlOpenForRead(c MySQLConnection, readTimeout time.Duration) (*sql.DB, error) {
	if readTimeout < 5*time.Second {
		readTimeout = 20 * time.Second
	}
	user := c.User
	if user == "" {
		user = "root"
	}
	port := c.Port
	if port <= 0 {
		port = 3306
	}
	cfg := mysqldriver.NewConfig()
	cfg.User = user
	cfg.Passwd = c.Password
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", c.Host, port)
	cfg.DBName = c.Database
	cfg.ParseTime = true
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = readTimeout
	cfg.WriteTimeout = 5 * time.Second
	cfg.InterpolateParams = true
	cfg.Params = map[string]string{}
	if c.TLS != "" {
		cfg.TLSConfig = c.TLS
	}
	if c.Params != "" {
		if extra, err := url.ParseQuery(c.Params); err == nil {
			for k, vs := range extra {
				if len(vs) > 0 {
					cfg.Params[k] = vs[0]
				}
			}
		}
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(2 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
