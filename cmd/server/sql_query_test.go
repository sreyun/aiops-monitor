package main

import (
	"strings"
	"testing"
	"time"

	"aiops-monitor/cmd/server/sqltoolkit"
)

func TestClampSQLQueryTimeout(t *testing.T) {
	if clampSQLQueryTimeout(0) != 20*time.Second {
		t.Fatal("default")
	}
	if clampSQLQueryTimeout(5) != 5*time.Second {
		t.Fatal("5s")
	}
	if clampSQLQueryTimeout(120) != 60*time.Second {
		t.Fatal("cap 60")
	}
}

func TestWorkbenchQueryRejectsPlaceholders(t *testing.T) {
	if !sqltoolkit.HasUnboundPlaceholder("SELECT * FROM t WHERE id = ?") {
		t.Fatal("expected placeholder")
	}
	if sqltoolkit.HasUnboundPlaceholder("SELECT * FROM user LIMIT 10") {
		t.Fatal("plain select")
	}
}

func TestMysqlWorkbenchQueryGuards(t *testing.T) {
	_, err := mysqlWorkbenchQuery(MySQLConnection{Host: "127.0.0.1", Port: 1}, "db", "DELETE FROM t", 50, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "只读") && !strings.Contains(err.Error(), "允许") {
		t.Fatalf("expected read-only guard, got %v", err)
	}
	_, err = mysqlWorkbenchQuery(MySQLConnection{Host: "127.0.0.1", Port: 1}, "bad-name!", "SELECT 1", 50, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "非法") {
		t.Fatalf("expected bad schema, got %v", err)
	}
}
