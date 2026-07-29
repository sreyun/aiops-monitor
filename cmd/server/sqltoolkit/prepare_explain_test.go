package sqltoolkit

import (
	"strings"
	"testing"
)

func TestPrepareSQLForExplainPlaceholders(t *testing.T) {
	in := "SELECT id FROM users WHERE status = ? AND id > ? LIMIT ?"
	out, notes := PrepareSQLForExplain(in, DialectMySQL57)
	if !strings.Contains(out, "NULL") {
		t.Fatalf("expected NULL probes: %s", out)
	}
	if !strings.Contains(out, "LIMIT 100") {
		t.Fatalf("expected LIMIT 100: %s", out)
	}
	if strings.Contains(out, "?") {
		t.Fatalf("placeholder remains: %s", out)
	}
	if len(notes) == 0 {
		t.Fatal("expected notes")
	}
}

func TestPrepareSQLForExplainMisquotedIdents(t *testing.T) {
	in := `SELECT 'id', 'platform', ? AS 'origin', 'referer' FROM 'operation_log' WHERE 'operation_time' >= ? AND 'business_key' != ?`
	out, notes := PrepareSQLForExplain(in, DialectMySQL57)
	if strings.Contains(out, "FROM 'operation_log'") {
		t.Fatalf("table still single-quoted: %s", out)
	}
	if !strings.Contains(out, "FROM `operation_log`") {
		t.Fatalf("expected backtick table: %s", out)
	}
	if !strings.Contains(out, "`operation_time`") {
		t.Fatalf("expected backtick column: %s", out)
	}
	if strings.Contains(out, "?") {
		t.Fatalf("placeholder remains: %s", out)
	}
	if len(notes) < 1 {
		t.Fatalf("expected notes, got %#v sql=%s", notes, out)
	}
}

func TestPrepareSQLForExplainKeepsRealStringLiteral(t *testing.T) {
	in := "SELECT id FROM users WHERE name = 'alice' AND status = 1"
	out, notes := PrepareSQLForExplain(in, DialectMySQL80)
	if out != in {
		t.Fatalf("should leave normal SQL alone:\n got %q\nwant %q", out, in)
	}
	if len(notes) != 0 {
		t.Fatalf("unexpected notes: %#v", notes)
	}
}

func TestPrepareSQLForExplainPostgresDollar(t *testing.T) {
	in := `SELECT id FROM "users" WHERE id = $1`
	out, _ := PrepareSQLForExplain(in, DialectPostgres)
	if strings.Contains(out, "$1") {
		t.Fatalf("dollar placeholder remains: %s", out)
	}
	if !strings.Contains(out, "NULL") {
		t.Fatalf("expected NULL: %s", out)
	}
}

func TestPrepareSQLForExplainQuotedDateFormatAndCountStar(t *testing.T) {
	in := "SELECT `tenant_id`, `platform`, `times`, `referer` FROM (\n" +
		" SELECT `tenant_id`, `platform`, `operation_user_id`, `referer`, SUM ( `times` ) AS `times` FROM (\n" +
		"  SELECT `tenant_id`, `platform`, `operation_user_id`, `referer`, ? AS `times` FROM (\n" +
		"   SELECT `tenant_id`, `platform`, `operation_user_id`, `referer` FROM `operation_log`\n" +
		"   WHERE `tenant_id` IS NOT NULL AND `referer` IS NOT NULL AND `operation_user_id` IS NOT NULL\n" +
		"   AND `platform` = ? AND `DATE_FORMAT` ( `operation_time`, ? ) = ?\n" +
		"   GROUP BY `tenant_id`, `platform`, `operation_user_id`, `referer`\n" +
		"  ) `web_dedup`\n" +
		"  UNION ALL\n" +
		"  SELECT `tenant_id`, `platform`, ? AS `operation_user_id`, `referer`, COUNT ( * ) AS `times`\n" +
		"  FROM `operation_log`\n" +
		"  WHERE `tenant_id` IS NOT NULL AND `referer` IS NOT NULL AND `platform` = ?\n" +
		"  AND `DATE_FORMAT` ( `operation_time`, ? ) = ?\n" +
		"  GROUP BY `tenant_id`, `platform`, `referer`\n" +
		" ) `x`\n" +
		" GROUP BY `tenant_id`, `platform`, `operation_user_id`, `referer`\n" +
		") AS `op`"
	out, notes := PrepareSQLForExplain(in, DialectMySQL57)
	if strings.Contains(out, "`DATE_FORMAT`") || strings.Contains(out, "'DATE_FORMAT'") {
		t.Fatalf("DATE_FORMAT still quoted: %s", out)
	}
	if !strings.Contains(out, "DATE_FORMAT(") && !strings.Contains(out, "DATE_FORMAT (") {
		t.Fatalf("expected DATE_FORMAT call: %s", out)
	}
	if strings.Contains(out, "COUNT ( * )") || strings.Contains(out, "COUNT (*)") || strings.Contains(out, "COUNT( *)") {
		t.Fatalf("COUNT(*) still spaced: %s", out)
	}
	if !strings.Contains(out, "COUNT(*)") {
		t.Fatalf("expected COUNT(*): %s", out)
	}
	if strings.Contains(out, "SUM (") || strings.Contains(out, "SUM  (") {
		t.Fatalf("SUM still has space before '(': %s", out)
	}
	if !strings.Contains(out, "SUM(") {
		t.Fatalf("expected SUM(: %s", out)
	}
	if strings.Contains(out, "DATE_FORMAT (") {
		t.Fatalf("DATE_FORMAT still has space before '(': %s", out)
	}
	if strings.Contains(out, "?") {
		t.Fatalf("placeholder remains: %s", out)
	}
	if !strings.Contains(out, "'%Y-%m-%d'") {
		t.Fatalf("expected date format probe: %s", out)
	}
	if strings.Contains(strings.ToUpper(out), "DATE_FORMAT(") && strings.Contains(out, "DATE_FORMAT(`operation_time`, NULL)") {
		t.Fatalf("DATE_FORMAT still has NULL format: %s", out)
	}
	joined := strings.Join(notes, "|")
	if !strings.Contains(joined, "函数") && !strings.Contains(joined, "COUNT") {
		t.Fatalf("expected normalize notes, got %#v", notes)
	}
}

func TestUnquoteBuiltinKeepsUserFunction(t *testing.T) {
	in := "SELECT `my_udf` (a) FROM t WHERE id = 1"
	out, notes := PrepareSQLForExplain(in, DialectMySQL80)
	if !strings.Contains(out, "`my_udf`") {
		t.Fatalf("user function should stay quoted: %s notes=%v", out, notes)
	}
}

func TestPrepareSQLForExplainTightensBuiltinParens(t *testing.T) {
	in := "SELECT AVG ( x ), MAX ( y ), MIN ( z ), GROUP_CONCAT ( a ), CAST ( id AS UNSIGNED )\n" +
		"FROM t WHERE IFNULL ( a, 0 ) > 0 AND DATE_FORMAT ( ts, ? ) = ?"
	out, notes := PrepareSQLForExplain(in, DialectMySQL57)
	for _, bad := range []string{"AVG (", "MAX (", "MIN (", "GROUP_CONCAT (", "CAST (", "IFNULL (", "DATE_FORMAT ("} {
		if strings.Contains(out, bad) {
			t.Fatalf("still spaced %q in: %s", bad, out)
		}
	}
	for _, need := range []string{"AVG(", "MAX(", "MIN(", "GROUP_CONCAT(", "CAST(", "IFNULL(", "DATE_FORMAT("} {
		if !strings.Contains(out, need) {
			t.Fatalf("missing %q in: %s", need, out)
		}
	}
	if strings.Contains(out, "?") {
		t.Fatalf("placeholder remains: %s", out)
	}
	joined := strings.Join(notes, "|")
	if !strings.Contains(joined, "1630") && !strings.Contains(joined, "空格") {
		t.Fatalf("expected space-normalize note, got %#v", notes)
	}
	// String literal must keep spaces that look like calls
	lit := "SELECT 'SUM (x)' AS s FROM t"
	out2, _ := PrepareSQLForExplain(lit, DialectMySQL80)
	if !strings.Contains(out2, "'SUM (x)'") {
		t.Fatalf("literal corrupted: %s", out2)
	}
}

func TestPrepareSQLForExplainDoesNotTightenUnknownIdent(t *testing.T) {
	in := "SELECT my_col (x) FROM t WHERE id = 1"
	out, _ := PrepareSQLForExplain(in, DialectMySQL80)
	if !strings.Contains(out, "my_col (") {
		t.Fatalf("should not tighten unknown ident: %s", out)
	}
}
