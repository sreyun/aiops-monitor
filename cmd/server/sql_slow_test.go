package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestBuildSlowDigestQuery(t *testing.T) {
	q, args := buildSlowDigestQuery([]string{"mysql", "sys"}, 100, 30)
	if !strings.Contains(q, "events_statements_summary_by_digest") {
		t.Fatalf("query missing P_S table: %s", q)
	}
	if !strings.Contains(q, "SCHEMA_NAME NOT IN") {
		t.Fatalf("expected exclude filter: %s", q)
	}
	if len(args) != 4 { // 2 excludes + minAvg ps + topN
		t.Fatalf("args=%v", args)
	}
	if args[2].(float64) != 100*1e9 {
		t.Fatalf("min avg ps=%v", args[2])
	}
	if args[3].(int) != 30 {
		t.Fatalf("topN=%v", args[3])
	}
}

func TestDefaultSlowSQLMonitor(t *testing.T) {
	c := defaultSlowSQLMonitor()
	if !c.Enabled || c.TopN != 30 || c.MinAvgLatencyMs != 100 {
		t.Fatalf("%+v", c)
	}
	if c.Schedule == nil || !c.Schedule.Enabled || c.Schedule.Kind != "daily" || c.Schedule.At != "03:00" {
		t.Fatalf("schedule=%+v", c.Schedule)
	}
	if err := sanitizeSchedule(c.Schedule); err != nil {
		t.Fatal(err)
	}
}

func TestSlowSQLMonitorWithDefaults(t *testing.T) {
	c := (&SlowSQLMonitorConfig{Enabled: true, TopN: 200}).withDefaults()
	if c.TopN != 100 {
		t.Fatalf("topN cap=%d", c.TopN)
	}
	if c.MinAvgLatencyMs != 100 {
		t.Fatalf("minAvg=%v", c.MinAvgLatencyMs)
	}
}

func TestUpsertMySQLConnectionDefaultsSlowSQL(t *testing.T) {
	path := t.TempDir() + "/config.json"
	cs, err := NewConfigStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := cs.UpsertMySQLConnection(MySQLConnection{
		Name: "demo", Host: "127.0.0.1", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.SlowSQL == nil || !saved.SlowSQL.Enabled {
		t.Fatalf("expected default slow sql: %+v", saved.SlowSQL)
	}
	if saved.SlowSQL.Schedule == nil || saved.SlowSQL.Schedule.Kind != "daily" {
		t.Fatalf("schedule=%+v", saved.SlowSQL.Schedule)
	}
	// Update without slow_sql preserves previous.
	saved2, err := cs.UpsertMySQLConnection(MySQLConnection{
		ID: saved.ID, Name: "demo2", Host: "127.0.0.1", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved2.SlowSQL == nil || !saved2.SlowSQL.Enabled {
		t.Fatalf("preserved slow sql lost: %+v", saved2.SlowSQL)
	}
	// Explicit disable.
	off := defaultSlowSQLMonitor()
	off.Enabled = false
	off.Schedule.Enabled = false
	saved3, err := cs.UpsertMySQLConnection(MySQLConnection{
		ID: saved.ID, Name: "demo3", Host: "127.0.0.1", Enabled: true, SlowSQL: off,
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved3.SlowSQL.Enabled {
		t.Fatal("expected disabled")
	}
}

func TestSlowSQLScheduleDueDaily(t *testing.T) {
	m := newSlowSQLManager(t.TempDir())
	sc := &PlaybookSchedule{Enabled: true, Kind: "daily", At: "03:00"}
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.Local)
	if !slowSQLScheduleDue(sc, m, "c1", now) {
		t.Fatal("expected due at 03:00")
	}
	if slowSQLScheduleDue(sc, m, "c1", now) {
		t.Fatal("same day should not fire twice")
	}
	later := time.Date(2026, 7, 26, 3, 1, 0, 0, time.Local)
	if slowSQLScheduleDue(sc, m, "c1", later) {
		t.Fatal("wrong minute should not fire")
	}
}

func TestSlowSQLScheduleDueInterval(t *testing.T) {
	m := newSlowSQLManager(t.TempDir())
	sc := &PlaybookSchedule{Enabled: true, Kind: "interval", IntervalMin: 30}
	now := time.Now()
	if !slowSQLScheduleDue(sc, m, "c2", now) {
		t.Fatal("first interval run should be due")
	}
	if slowSQLScheduleDue(sc, m, "c2", now.Add(5*time.Minute)) {
		t.Fatal("should respect interval floor")
	}
	if !slowSQLScheduleDue(sc, m, "c2", now.Add(31*time.Minute)) {
		t.Fatal("expected due after interval")
	}
}

func TestHumanizeSlowSQLErr(t *testing.T) {
	msg := humanizeSlowSQLErr(fmt.Errorf("Error 1146: Table 'performance_schema.events_statements_summary_by_digest' doesn't exist"))
	if !strings.Contains(msg, "performance_schema") || !strings.Contains(msg, "GRANT") {
		t.Fatalf("%q", msg)
	}
}

func TestMysqlSystemSchema(t *testing.T) {
	if !mysqlSystemSchema("mysql") || !mysqlSystemSchema("SYS") {
		t.Fatal("system schemas")
	}
	if mysqlSystemSchema("app_db") {
		t.Fatal("app_db should not be system")
	}
}

func TestSanitizeFilePart(t *testing.T) {
	if sanitizeFilePart("ab/../c") != "ab____c" && sanitizeFilePart("ab/../c") == "" {
		t.Fatal(sanitizeFilePart("ab/../c"))
	}
	got := sanitizeFilePart("conn-1")
	if got != "conn-1" {
		t.Fatalf("%q", got)
	}
}
