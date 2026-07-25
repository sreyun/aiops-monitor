package sqltoolkit

import (
	"strings"
	"testing"
)

func TestBeautifyKeywords(t *testing.T) {
	out := Beautify("select a,b from t where id=1", DialectMySQL57)
	if !strings.Contains(out, "SELECT") || !strings.Contains(out, "FROM") {
		t.Fatalf("beautify: %q", out)
	}
}

func TestAuditSelectStarAndDelete(t *testing.T) {
	r := Audit("SELECT * FROM users WHERE name LIKE '%ab%'", DialectMySQL57)
	ids := map[string]bool{}
	for _, f := range r.Findings {
		ids[f.ID] = true
	}
	if !ids["select_star"] || !ids["like_leading_wildcard"] {
		t.Fatalf("findings=%v score=%d", ids, r.Score)
	}
	r2 := Audit("DELETE FROM users", DialectMySQL80)
	crit := false
	for _, f := range r2.Findings {
		if f.ID == "delete_no_where" {
			crit = true
		}
	}
	if !crit {
		t.Fatal("expected delete_no_where")
	}
}

func TestOptimizeAddsLimit(t *testing.T) {
	o := Optimize("SELECT id FROM t WHERE status=1", DialectMySQL80)
	if !strings.Contains(strings.ToUpper(o.RewrittenSQL), "LIMIT") {
		t.Fatalf("expected LIMIT in %q", o.RewrittenSQL)
	}
	if len(o.IndexHints) == 0 {
		t.Fatal("expected index hint")
	}
}

func TestIsReadOnlyAndForbidden(t *testing.T) {
	if !IsReadOnlyQuery("SELECT 1") || !IsReadOnlyQuery("EXPLAIN SELECT 1") {
		t.Fatal("readonly")
	}
	if IsReadOnlyQuery("UPDATE t SET a=1; SELECT 1") {
		t.Fatal("multi should fail readonly")
	}
	if !ForbiddenWrite("DELETE FROM t WHERE id=1") {
		t.Fatal("delete is write")
	}
	if ForbiddenWrite("SELECT id FROM t") {
		t.Fatal("select is not write")
	}
	if !IsAllowedIndexDDL("CREATE INDEX idx_a ON t(a)") {
		t.Fatal("create index should be allowed")
	}
	if !IsAllowedIndexDDL("ALTER TABLE t ADD INDEX idx_b (b);") {
		t.Fatal("alter add index should be allowed")
	}
	if IsAllowedIndexDDL("DROP INDEX idx_a ON t") || IsAllowedIndexDDL("CREATE TABLE t(a int)") {
		t.Fatal("destructive DDL must be rejected")
	}
}

func TestCTEOn57(t *testing.T) {
	r := Audit("WITH cte AS (SELECT 1 AS a) SELECT * FROM cte", DialectMySQL57)
	found := false
	for _, f := range r.Findings {
		if f.ID == "cte_57" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected cte_57")
	}
}
