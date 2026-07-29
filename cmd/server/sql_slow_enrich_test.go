package main

import (
	"strings"
	"testing"

	"aiops-monitor/cmd/server/sqltoolkit"
)

func TestSQLLikelyTruncated(t *testing.T) {
	full := "SELECT id FROM users WHERE status = 1"
	if sqlLikelyTruncated(full, 1024) {
		t.Fatal("complete short SQL should not look truncated")
	}
	cut := strings.Repeat("x", 1020) + " FROM"
	if !sqlLikelyTruncated(cut, 1024) {
		t.Fatal("near max_digest_length + trailing FROM should be truncated")
	}
	mid := "SELECT a FROM t WHERE id IN ( SELECT DISTINCTROW r2.taskid FROM"
	if !sqlLikelyTruncated(mid, 0) {
		t.Fatal("unbalanced paren / trailing FROM should be truncated")
	}
}

func TestShouldPreferRecoveredSQL(t *testing.T) {
	digest := "SELECT id FROM users WHERE cycle = ?"
	full := "SELECT id FROM users WHERE cycle = 'week'"
	if !shouldPreferRecoveredSQL(digest, full) {
		t.Fatal("should prefer recovered SQL that clears placeholders")
	}
	if shouldPreferRecoveredSQL(full, digest) {
		t.Fatal("should not prefer digest over full")
	}
	quotedDigest := "select tel,name from user where tel='?'"
	quotedFull := "select tel,name from user where tel='17301655949'"
	if !shouldPreferRecoveredSQL(quotedDigest, quotedFull) {
		t.Fatal("should prefer full SQL over DIGEST '?' literals")
	}
	if shouldPreferRecoveredSQL(quotedFull, quotedDigest) {
		t.Fatal("must not prefer DIGEST over full literals")
	}
	longer := digest + " AND status = 1 AND x = 2"
	if !shouldPreferRecoveredSQL(digest, longer) {
		t.Fatal("should prefer longer recovered text")
	}
	if shouldPreferRecoveredSQL(digest, "") {
		t.Fatal("empty recovered should be rejected")
	}
}

func TestPickLongestRecoveredSQL(t *testing.T) {
	best, ok := pickLongestRecoveredSQL([]sqlRecoverCandidate{
		{SQL: "SELECT 1", Source: "history"},
		{SQL: "SELECT 1 FROM t WHERE id=1 AND x=2", Source: "history_long"},
		{SQL: "SELECT 1 FROM t", Source: "current"},
	})
	if !ok || best.Source != "history_long" {
		t.Fatalf("want history_long longest, got %+v ok=%v", best, ok)
	}
	best2, ok2 := pickLongestRecoveredSQL([]sqlRecoverCandidate{
		{SQL: "SELECT a FROM t WHERE id = ?", Source: "digest"},
		{SQL: "SELECT a FROM t WHERE id = 1", Source: "slow_log"},
	})
	if !ok2 || best2.Source != "slow_log" {
		t.Fatalf("want slow_log clearing placeholders, got %+v", best2)
	}
}

func TestSQLDigestFulltextCachePreferLonger(t *testing.T) {
	dir := t.TempDir()
	c := newSQLDigestFulltextCache(dir)
	c.Put("conn1", "d1", "SELECT 1", "history")
	c.Put("conn1", "d1", "SELECT 1 FROM dual WHERE x=1", "history_long")
	e, ok := c.Get("conn1", "d1")
	if !ok || !strings.Contains(e.SQL, "FROM dual") {
		t.Fatalf("cache should keep longer SQL: %+v", e)
	}
	// shorter should not overwrite
	c.Put("conn1", "d1", "SELECT 1", "digest")
	e2, _ := c.Get("conn1", "d1")
	if !strings.Contains(e2.SQL, "FROM dual") {
		t.Fatalf("shorter must not overwrite: %+v", e2)
	}
	// reload from disk
	c2 := newSQLDigestFulltextCache(dir)
	e3, ok3 := c2.Get("conn1", "d1")
	if !ok3 || e3.SQL != e2.SQL {
		t.Fatalf("persist/reload failed: %+v", e3)
	}
}

func TestBuildSlowSQLPSLimitsRemedy(t *testing.T) {
	lim := buildSlowSQLPSLimits(1024, 1024)
	if lim == nil || lim.RemedySQL == "" {
		t.Fatal("low limits should include remedy SQL")
	}
	okLim := buildSlowSQLPSLimits(8192, 8192)
	if okLim == nil || okLim.RemedySQL != "" {
		t.Fatalf("high limits should not need remedy SQL: %+v", okLim)
	}
}

func TestInferSchemaFromSQLText(t *testing.T) {
	sql := "SELECT t.id FROM `erp_core`.`task` t JOIN `erp_core`.`task_rel` r ON r.taskid = t.id"
	got := inferSchemaFromSQLText(sql)
	if got != "erp_core" {
		t.Fatalf("schema=%q want erp_core", got)
	}
	shape := sqltoolkit.ExtractQueryShape(sql)
	if shape == nil || !shape.ParseOK {
		t.Fatalf("parse failed: %+v", shape)
	}
	if shape.DominantSchema() != "erp_core" {
		t.Fatalf("DominantSchema=%q", shape.DominantSchema())
	}
}

func TestMysqlConnReadyErrors(t *testing.T) {
	if err := mysqlConnReady(MySQLConnection{}, false); err == nil || !strings.Contains(err.Error(), "连接不存在") {
		t.Fatalf("want 连接不存在, got %v", err)
	}
	if err := mysqlConnReady(MySQLConnection{Enabled: false}, true); err == nil || !strings.Contains(err.Error(), "连接已停用") {
		t.Fatalf("want 连接已停用, got %v", err)
	}
	if err := mysqlConnReady(MySQLConnection{Enabled: true}, true); err != nil {
		t.Fatal(err)
	}
}
