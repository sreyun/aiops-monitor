package main

import (
	"strings"
	"testing"

	"aiops-monitor/cmd/server/sqltoolkit"
)

func TestPgPeelExplain(t *testing.T) {
	inner, err := pgPeelExplain("EXPLAIN SELECT 1")
	if err != nil || !strings.EqualFold(strings.TrimSpace(inner), "SELECT 1") {
		t.Fatalf("simple peel: inner=%q err=%v", inner, err)
	}
	inner, err = pgPeelExplain("EXPLAIN (FORMAT JSON) SELECT id FROM t WHERE a = 1")
	if err != nil || !strings.Contains(strings.ToLower(inner), "select id from t") {
		t.Fatalf("format json peel: inner=%q err=%v", inner, err)
	}
	if _, err := pgPeelExplain("EXPLAIN ANALYZE SELECT 1"); err == nil {
		t.Fatal("expected EXPLAIN ANALYZE rejection")
	}
	if _, err := pgPeelExplain("EXPLAIN (ANALYZE, FORMAT JSON) SELECT 1"); err == nil {
		t.Fatal("expected EXPLAIN (ANALYZE) rejection")
	}
}

func TestAnalyzePGExplainJSON(t *testing.T) {
	raw := `[{
		"Plan": {
			"Node Type": "Seq Scan",
			"Relation Name": "orders",
			"Alias": "orders",
			"Startup Cost": 0.00,
			"Total Cost": 25.00,
			"Plan Rows": 1000,
			"Plan Width": 32
		}
	}]`
	a := analyzePGExplainJSON(raw)
	if a == nil || a.Summary == "" {
		t.Fatal("expected summary")
	}
	if a.FullScans < 1 {
		t.Fatalf("expected full scan count: %+v", a)
	}
}

func TestDataSourceToSQLConnPostgres(t *testing.T) {
	c, err := dataSourceToSQLConn(DataSource{
		Type: "postgres", URL: "db.internal", Port: 5432,
		AuthUser: "u", AuthPass: "p", Database: "app", TLS: "require",
	})
	if err != nil {
		t.Fatal(err)
	}
	if driverOf(c) != "postgres" || c.Host != "db.internal" || c.Database != "app" {
		t.Fatalf("conn=%+v", c)
	}
	dsn := postgresDSN(c)
	if !strings.Contains(dsn, "sslmode=require") || !strings.Contains(dsn, "app") {
		t.Fatalf("dsn=%s", dsn)
	}
}

func TestPGReadOnlyHelpers(t *testing.T) {
	if !sqltoolkit.IsReadOnlyQuery("WITH x AS (SELECT 1) SELECT * FROM x") {
		t.Fatal("CTE should be read-only")
	}
	if sqltoolkit.IsReadOnlyQuery("DELETE FROM t") {
		t.Fatal("DELETE must not be read-only")
	}
}
