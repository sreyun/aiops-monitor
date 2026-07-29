package sqltoolkit

import "testing"

func TestForbiddenWriteBlocksSleepAndCopy(t *testing.T) {
	if !ForbiddenWrite("SELECT pg_sleep(1)") {
		t.Fatal("pg_sleep must be forbidden")
	}
	if !ForbiddenWrite("COPY t TO STDOUT") {
		t.Fatal("COPY must be forbidden")
	}
	if !ForbiddenWrite("SELECT sleep(5)") {
		t.Fatal("sleep() must be forbidden")
	}
	if ForbiddenWrite("SELECT id FROM users WHERE id = 1") {
		t.Fatal("plain SELECT must be allowed")
	}
}

func TestForbiddenWriteBlocksSelectSideEffects(t *testing.T) {
	cases := []string{
		"SELECT pg_terminate_backend(123)",
		"SELECT pg_cancel_backend(pid) FROM pg_stat_activity",
		"WITH x AS (SELECT pg_terminate_backend(10)) SELECT * FROM x",
		"SELECT pg_reload_conf()",
		"SELECT pg_advisory_lock(42)",
		"SELECT lo_unlink(12345)",
		"SELECT id FROM t FOR UPDATE",
		"SELECT id FROM t FOR SHARE",
		"SELECT id FROM t LOCK IN SHARE MODE",
		"SELECT release_lock('x')",
	}
	for _, sql := range cases {
		if !ForbiddenWrite(sql) {
			t.Fatalf("must forbid side-effect SQL: %s", sql)
		}
	}
	if ForbiddenWrite("SELECT count(*) FROM pg_stat_activity") {
		t.Fatal("plain activity SELECT must remain allowed")
	}
}
