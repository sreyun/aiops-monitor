package main

import (
	"testing"
	"time"
)

func TestShouldAlertSecurityDedupe(t *testing.T) {
	secAlertMu.Lock()
	secAlertSeen = map[string]int64{}
	secAlertMu.Unlock()

	now := time.Now().Unix()
	if !shouldAlertSecurity("k1", now) {
		t.Fatal("first should alert")
	}
	if shouldAlertSecurity("k1", now+10) {
		t.Fatal("within window should suppress")
	}
	if !shouldAlertSecurity("k1", now+secAlertWindowSec+1) {
		t.Fatal("after window should alert again")
	}
	if !shouldAlertSecurity("k2", now) {
		t.Fatal("different key should alert")
	}
}

func TestFindingLevelRank(t *testing.T) {
	if findingLevelRank("critical") < findingLevelRank("high") {
		t.Fatal("crit should rank higher")
	}
}

func TestMigrateMySQLSlowSQLDefaultsOnce(t *testing.T) {
	path := t.TempDir() + "/config.json"
	cs, err := NewConfigStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs.mu.Lock()
	cs.cfg.MySQLConnections = []MySQLConnection{
		{ID: "a", Name: "legacy", Host: "127.0.0.1", Enabled: true},
		{ID: "b", Name: "newer", Host: "127.0.0.1", Enabled: true, SlowSQL: &SlowSQLMonitorConfig{Enabled: false}},
	}
	cs.mu.Unlock()
	if !cs.migrateMySQLSlowSQLDefaultsOnce() {
		t.Fatal("expected migration")
	}
	list := cs.ListMySQLConnections()
	var legacy, newer *MySQLConnection
	for i := range list {
		if list[i].ID == "a" {
			legacy = &list[i]
		}
		if list[i].ID == "b" {
			newer = &list[i]
		}
	}
	if legacy == nil || legacy.SlowSQL == nil || !legacy.SlowSQL.Enabled {
		t.Fatalf("legacy not migrated: %+v", legacy)
	}
	if newer == nil || newer.SlowSQL == nil || newer.SlowSQL.Enabled {
		t.Fatalf("newer should keep disabled: %+v", newer)
	}
	if cs.migrateMySQLSlowSQLDefaultsOnce() {
		t.Fatal("second migrate should be no-op")
	}
}

func TestAIGovPersistTools(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/ai_tool_audit.json"
	h := newAIGovHub()
	h.load(path)
	h.recordTool(aiToolAuditEntry{Actor: "u", Tool: "t", Action: "run", Approved: true})
	h2 := newAIGovHub()
	h2.load(path)
	list := h2.listTools(10)
	if len(list) != 1 || list[0].Tool != "t" {
		t.Fatalf("persist failed: %+v", list)
	}
}
