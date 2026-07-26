package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// SQLProcessRow is a read-only PROCESSLIST entry.
type SQLProcessRow struct {
	ID      int64  `json:"id"`
	User    string `json:"user,omitempty"`
	Host    string `json:"host,omitempty"`
	DB      string `json:"db,omitempty"`
	Command string `json:"command,omitempty"`
	TimeSec int64  `json:"time_sec"`
	State   string `json:"state,omitempty"`
	Info    string `json:"info,omitempty"`
}

// SQLLockRow is a simplified InnoDB lock / trx wait row.
type SQLLockRow struct {
	WaitingTrxID  string `json:"waiting_trx_id,omitempty"`
	WaitingPID    int64  `json:"waiting_pid,omitempty"`
	WaitingQuery  string `json:"waiting_query,omitempty"`
	BlockingTrxID string `json:"blocking_trx_id,omitempty"`
	BlockingPID   int64  `json:"blocking_pid,omitempty"`
	BlockingQuery string `json:"blocking_query,omitempty"`
	WaitStarted   string `json:"wait_started,omitempty"`
	LockedTable   string `json:"locked_table,omitempty"`
	LockMode      string `json:"lock_mode,omitempty"`
	LockType      string `json:"lock_type,omitempty"`
}

func (s *Server) handleMySQLProcesslist(w http.ResponseWriter, r *http.Request) {
	c, ok := s.cfg.GetMySQLConnection(strings.TrimSpace(r.PathValue("id")))
	if !ok || !c.Enabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found or disabled"})
		return
	}
	if driverOf(c) == "postgres" {
		rows, err := pgCollectProcesslist(c)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"processes": rows, "driver": "postgres"})
		return
	}
	rows, err := mysqlCollectProcesslist(c)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"processes": rows, "driver": "mysql"})
}

func (s *Server) handleMySQLLocks(w http.ResponseWriter, r *http.Request) {
	c, ok := s.cfg.GetMySQLConnection(strings.TrimSpace(r.PathValue("id")))
	if !ok || !c.Enabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found or disabled"})
		return
	}
	if driverOf(c) == "postgres" {
		rows, err := pgCollectLocks(c)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"locks": rows, "driver": "postgres"})
		return
	}
	rows, err := mysqlCollectLocks(c)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"locks": rows, "driver": "mysql"})
}

func mysqlCollectProcesslist(c MySQLConnection) ([]SQLProcessRow, error) {
	db, err := sql.Open("mysql", mysqlDSNSlow(c))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	q := `SELECT ID, USER, HOST, IFNULL(DB,''), COMMAND, TIME, IFNULL(STATE,''), IFNULL(INFO,'')
FROM information_schema.PROCESSLIST
ORDER BY TIME DESC
LIMIT 200`
	rs, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := make([]SQLProcessRow, 0, 32)
	for rs.Next() {
		var row SQLProcessRow
		var info sql.NullString
		if err := rs.Scan(&row.ID, &row.User, &row.Host, &row.DB, &row.Command, &row.TimeSec, &row.State, &info); err != nil {
			continue
		}
		if info.Valid {
			row.Info = truncateRun(info.String, 500)
		}
		out = append(out, row)
	}
	return out, rs.Err()
}

func mysqlCollectLocks(c MySQLConnection) ([]SQLLockRow, error) {
	db, err := sql.Open("mysql", mysqlDSNSlow(c))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	// MySQL 8+ data_lock_waits; fall back gracefully on older versions.
	q := `
SELECT
  COALESCE(r.trx_id,''), COALESCE(r.trx_mysql_thread_id,0), COALESCE(LEFT(r.trx_query,400),''),
  COALESCE(b.trx_id,''), COALESCE(b.trx_mysql_thread_id,0), COALESCE(LEFT(b.trx_query,400),''),
  COALESCE(CAST(w.requesting_engine_lock_id AS CHAR),''),
  COALESCE(CAST(w.blocking_engine_lock_id AS CHAR),'')
FROM performance_schema.data_lock_waits w
JOIN information_schema.innodb_trx r ON r.trx_id = w.requesting_engine_transaction_id
JOIN information_schema.innodb_trx b ON b.trx_id = w.blocking_engine_transaction_id
LIMIT 100`
	rs, err := db.QueryContext(ctx, q)
	if err != nil {
		// Older MySQL: try sys.innodb_lock_waits if present.
		q2 := `
SELECT
  COALESCE(waiting_trx_id,''), COALESCE(waiting_pid,0), COALESCE(LEFT(waiting_query,400),''),
  COALESCE(blocking_trx_id,''), COALESCE(blocking_pid,0), COALESCE(LEFT(blocking_query,400),''),
  COALESCE(wait_started,''), COALESCE(locked_table,''), COALESCE(lock_mode,''), COALESCE(lock_type,'')
FROM sys.innodb_lock_waits
LIMIT 100`
		rs2, err2 := db.QueryContext(ctx, q2)
		if err2 != nil {
			return []SQLLockRow{}, nil // empty rather than hard-fail: locks view may be unavailable
		}
		defer rs2.Close()
		out := make([]SQLLockRow, 0, 16)
		for rs2.Next() {
			var row SQLLockRow
			if err := rs2.Scan(&row.WaitingTrxID, &row.WaitingPID, &row.WaitingQuery,
				&row.BlockingTrxID, &row.BlockingPID, &row.BlockingQuery,
				&row.WaitStarted, &row.LockedTable, &row.LockMode, &row.LockType); err != nil {
				continue
			}
			out = append(out, row)
		}
		return out, rs2.Err()
	}
	defer rs.Close()
	out := make([]SQLLockRow, 0, 16)
	for rs.Next() {
		var row SQLLockRow
		var reqLock, blkLock string
		if err := rs.Scan(&row.WaitingTrxID, &row.WaitingPID, &row.WaitingQuery,
			&row.BlockingTrxID, &row.BlockingPID, &row.BlockingQuery, &reqLock, &blkLock); err != nil {
			continue
		}
		row.LockType = strings.TrimSpace(reqLock)
		out = append(out, row)
	}
	return out, rs.Err()
}

func mysqlKillSession(c MySQLConnection, pid int64) error {
	if pid <= 0 {
		return fmt.Errorf("invalid process id")
	}
	db, err := sql.Open("mysql", mysqlDSNSlow(c))
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = db.ExecContext(ctx, "KILL ?", pid)
	return err
}

func parseKillSQL(sqlText string) (int64, bool) {
	s := strings.TrimSpace(sqlText)
	s = strings.TrimSuffix(s, ";")
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return 0, false
	}
	if !strings.EqualFold(fields[0], "KILL") {
		return 0, false
	}
	// KILL [CONNECTION|QUERY] <id>
	idStr := fields[len(fields)-1]
	if len(fields) == 3 {
		kw := strings.ToUpper(fields[1])
		if kw != "CONNECTION" && kw != "QUERY" {
			return 0, false
		}
	} else if len(fields) != 2 {
		return 0, false
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
