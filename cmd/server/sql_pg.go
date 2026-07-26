package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

func driverOf(c MySQLConnection) string {
	d := strings.ToLower(strings.TrimSpace(c.Driver))
	if d == "postgres" || d == "postgresql" || d == "pg" {
		return "postgres"
	}
	return "mysql"
}

func postgresDSN(c MySQLConnection) string {
	user := c.User
	if user == "" {
		user = "postgres"
	}
	port := c.Port
	if port <= 0 {
		port = 5432
	}
	db := c.Database
	if db == "" {
		db = "postgres"
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, c.Password),
		Host:   fmt.Sprintf("%s:%d", c.Host, port),
		Path:   "/" + db,
	}
	q := url.Values{}
	ssl := strings.ToLower(strings.TrimSpace(c.TLS))
	switch ssl {
	case "true", "require":
		q.Set("sslmode", "require")
	case "skip-verify", "prefer", "preferred":
		q.Set("sslmode", "prefer")
	default:
		q.Set("sslmode", "disable")
	}
	q.Set("connect_timeout", "8")
	if extras := strings.TrimSpace(c.Params); extras != "" {
		if ev, err := url.ParseQuery(extras); err == nil {
			for k, vs := range ev {
				for _, v := range vs {
					q.Set(k, v)
				}
			}
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func pgOpen(c MySQLConnection) (*sql.DB, error) {
	db, err := sql.Open("postgres", postgresDSN(c))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(2 * time.Minute)
	return db, nil
}

func pgPing(c MySQLConnection) (string, error) {
	db, err := pgOpen(c)
	if err != nil {
		return "", err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var ver string
	if err := db.QueryRowContext(ctx, "SELECT version()").Scan(&ver); err != nil {
		return "", err
	}
	return truncateRun(ver, 200), nil
}

func pgCollectProcesslist(c MySQLConnection) ([]SQLProcessRow, error) {
	db, err := pgOpen(c)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	q := `
SELECT pid, COALESCE(usename,''), COALESCE(client_addr::text,''), COALESCE(datname,''),
       COALESCE(state,''), COALESCE(EXTRACT(EPOCH FROM (now()-query_start))::bigint,0),
       COALESCE(wait_event_type,''), COALESCE(LEFT(query,500),'')
FROM pg_stat_activity
WHERE pid <> pg_backend_pid()
ORDER BY query_start NULLS LAST
LIMIT 200`
	rs, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := make([]SQLProcessRow, 0, 32)
	for rs.Next() {
		var row SQLProcessRow
		var waitEv string
		if err := rs.Scan(&row.ID, &row.User, &row.Host, &row.DB, &row.Command, &row.TimeSec, &waitEv, &row.Info); err != nil {
			continue
		}
		row.State = waitEv
		out = append(out, row)
	}
	return out, rs.Err()
}

func pgCollectLocks(c MySQLConnection) ([]SQLLockRow, error) {
	db, err := pgOpen(c)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	q := `
SELECT
  COALESCE(blocked.pid::text,''), COALESCE(blocked.pid,0), COALESCE(LEFT(blocked.query,400),''),
  COALESCE(blocking.pid::text,''), COALESCE(blocking.pid,0), COALESCE(LEFT(blocking.query,400),''),
  COALESCE(blocked.wait_event,''), COALESCE(a.locktype,''), COALESCE(a.mode,'')
FROM pg_catalog.pg_locks a
JOIN pg_stat_activity blocked ON blocked.pid = a.pid
JOIN pg_catalog.pg_locks b ON a.locktype = b.locktype AND a.database IS NOT DISTINCT FROM b.database
  AND a.relation IS NOT DISTINCT FROM b.relation AND a.page IS NOT DISTINCT FROM b.page
  AND a.tuple IS NOT DISTINCT FROM b.tuple AND a.virtualxid IS NOT DISTINCT FROM b.virtualxid
  AND a.transactionid IS NOT DISTINCT FROM b.transactionid AND a.classid IS NOT DISTINCT FROM b.classid
  AND a.objid IS NOT DISTINCT FROM b.objid AND a.objsubid IS NOT DISTINCT FROM b.objsubid
  AND a.pid <> b.pid
JOIN pg_stat_activity blocking ON blocking.pid = b.pid
WHERE NOT a.granted AND b.granted
LIMIT 100`
	rs, err := db.QueryContext(ctx, q)
	if err != nil {
		return []SQLLockRow{}, nil
	}
	defer rs.Close()
	out := make([]SQLLockRow, 0, 16)
	for rs.Next() {
		var row SQLLockRow
		if err := rs.Scan(&row.WaitingTrxID, &row.WaitingPID, &row.WaitingQuery,
			&row.BlockingTrxID, &row.BlockingPID, &row.BlockingQuery,
			&row.WaitStarted, &row.LockType, &row.LockMode); err != nil {
			continue
		}
		out = append(out, row)
	}
	return out, rs.Err()
}

func pgExplain(c MySQLConnection, sqlText string) (map[string]any, error) {
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		return nil, fmt.Errorf("sql required")
	}
	low := strings.ToLower(sqlText)
	if strings.HasPrefix(low, "explain") {
		// already explain
	} else if !(strings.HasPrefix(low, "select") || strings.HasPrefix(low, "with") ||
		strings.HasPrefix(low, "show") || strings.HasPrefix(low, "values")) {
		return nil, fmt.Errorf("PostgreSQL 只读：仅允许对 SELECT/WITH 做 EXPLAIN")
	} else {
		sqlText = "EXPLAIN (FORMAT TEXT) " + sqlText
	}
	db, err := pgOpen(c)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rs, err := db.QueryContext(ctx, sqlText)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var lines []string
	for rs.Next() {
		var line string
		if rs.Scan(&line) == nil {
			lines = append(lines, line)
		}
	}
	return map[string]any{
		"driver":  "postgres",
		"plan":    strings.Join(lines, "\n"),
		"rows":    lines,
		"readonly": true,
	}, rs.Err()
}

func pgSchemaHealth(c MySQLConnection) ([]SchemaHealthFinding, error) {
	db, err := pgOpen(c)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var out []SchemaHealthFinding
	// Tables without primary key
	rs, err := db.QueryContext(ctx, `
SELECT n.nspname, c.relname
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog','information_schema')
  AND NOT EXISTS (
    SELECT 1 FROM pg_constraint con
    WHERE con.conrelid = c.oid AND con.contype = 'p'
  )
LIMIT 50`)
	if err == nil {
		defer rs.Close()
		for rs.Next() {
			var schema, table string
			if rs.Scan(&schema, &table) == nil {
				out = append(out, SchemaHealthFinding{
					Level: "medium", Code: "no_pk", Schema: schema, Table: table,
					Title: "表缺少主键", Detail: schema + "." + table,
					Suggest: "为业务表补充 PRIMARY KEY，便于复制与在线 DDL",
				})
			}
		}
	}
	return out, nil
}
