package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const sqlmapErrorsXMLFixture = `<?xml version="1.0" encoding="UTF-8"?>
<root>
    <dbms value="MySQL">
        <error regexp="SQL syntax.*?MySQL"/>
        <error regexp="Unknown column '[^ ]+' in 'field list'"/>
        <error regexp="(?=lookahead-is-pcre-only)MySQL"/>
    </dbms>
    <dbms value="PostgreSQL">
        <error regexp="PostgreSQL.*?ERROR"/>
    </dbms>
</root>`

func writeSqlmapFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	xmlDir := filepath.Join(dir, "data", "xml")
	if err := os.MkdirAll(xmlDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xmlDir, "errors.xml"), []byte(sqlmapErrorsXMLFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestLoadSQLErrorSignaturesSkipsUncompilablePatterns: sqlmap ships a handful of
// PCRE constructs Go's RE2 cannot parse. One bad pattern must not cost us the
// whole signature set.
func TestLoadSQLErrorSignaturesSkipsUncompilablePatterns(t *testing.T) {
	set, err := loadSQLErrorSignatures(writeSqlmapFixture(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(set.Sigs) != 3 {
		t.Fatalf("loaded %d signatures, want 3 compilable ones", len(set.Sigs))
	}
	if dbms, ok := set.match("You have an error in your SQL syntax; check the MySQL server"); !ok || dbms != "MySQL" {
		t.Errorf("MySQL error not matched: dbms=%q ok=%v", dbms, ok)
	}
	if _, ok := set.match("PostgreSQL query failed: ERROR: syntax error at or near"); !ok {
		t.Error("PostgreSQL error not matched")
	}
	if _, ok := set.match("everything is fine"); ok {
		t.Error("a clean page was matched as a DBMS error")
	}
}

func TestLoadSQLErrorSignaturesMissingFile(t *testing.T) {
	if _, err := loadSQLErrorSignatures(t.TempDir()); err == nil {
		t.Fatal("expected an error when errors.xml is absent")
	}
}

// TestBaselineSignaturesCoverCommonEngines keeps the compiled-in fallback
// useful: an air-gapped install still detects the databases we actually meet.
func TestBaselineSignaturesCoverCommonEngines(t *testing.T) {
	set := builtinSQLErrorSignatures()
	cases := map[string]string{
		"MySQL":                `You have an error in your SQL syntax; check the manual that corresponds to your MySQL server version`,
		"PostgreSQL":           `Warning: pg_query(): Query failed`,
		"Microsoft SQL Server": `Unclosed quotation mark after the character string ''.`,
		"Oracle":               `ORA-01756: quoted string not properly terminated`,
		"SQLite":               `sqlite3.OperationalError: near "'": syntax error`,
	}
	for wantDBMS, body := range cases {
		got, ok := set.match(body)
		if !ok {
			t.Errorf("%s error not detected by baseline set", wantDBMS)
			continue
		}
		if got != wantDBMS {
			t.Errorf("body attributed to %q, want %q", got, wantDBMS)
		}
	}
	// Ordinary application text must stay quiet — a false SQLi report is worse
	// than a missed one for triage load.
	for _, benign := range []string{
		"An error occurred, please try again later.",
		"<h1>500 Internal Server Error</h1>",
		"user not found",
	} {
		if dbms, ok := set.match(benign); ok {
			t.Errorf("benign page %q flagged as %s", benign, dbms)
		}
	}
}

func TestCheckDBMSErrorLeakIsPassiveAndSpecific(t *testing.T) {
	setSQLErrorSignatures(builtinSQLErrorSignatures())
	leaky := []byte("Fatal error: ORA-00933: SQL command not properly ended")
	out := checkDBMSErrorLeak("https://x.example.com", leaky)
	if len(out) != 1 {
		t.Fatalf("expected one finding, got %d", len(out))
	}
	if out[0].TemplateID != "builtin/dbms-error-disclosure" || out[0].Severity != "medium" {
		t.Errorf("unexpected finding shape: %+v", out[0])
	}
	if got := webOWASPCategory(out[0]); got != owaspA05 {
		t.Errorf("error disclosure classified as %q, want A05 (misconfiguration)", got)
	}
	if len(checkDBMSErrorLeak("https://x.example.com", []byte("<html>ok</html>"))) != 0 {
		t.Error("clean page produced a leak finding")
	}
}

// TestErrorBasedSQLiConfirmsWithQuoteBalance is the core of the check: an error
// that appears on one quote and disappears on two is a broken SQL literal.
func TestErrorBasedSQLiConfirmsWithQuoteBalance(t *testing.T) {
	setSQLErrorSignatures(builtinSQLErrorSignatures())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		switch {
		case id == "1''":
			w.Write([]byte("<html>no results</html>"))
		case len(id) > 1 && id[len(id)-1] == '\'':
			w.Write([]byte("You have an error in your SQL syntax; check the manual that corresponds to your MySQL server version"))
		default:
			w.Write([]byte("<html>ok</html>"))
		}
	}))
	defer srv.Close()

	base := srv.URL + "/item?id=1"
	b := newBuiltinScanContext(WebScanTarget{BaseURL: base}, true, nil)
	out := checkErrorBasedSQLi(context.Background(), b, base)
	if len(out) != 1 {
		t.Fatalf("expected one SQLi finding, got %d: %+v", len(out), out)
	}
	if out[0].TemplateID != "builtin/error-based-sqli" || out[0].Severity != "critical" {
		t.Errorf("unexpected finding: %+v", out[0])
	}
	if got := webOWASPCategory(out[0]); got != owaspA03 {
		t.Errorf("SQLi classified as %q, want A03 injection", got)
	}
}

// TestErrorBasedSQLiIgnoresAlwaysErroringParam: an app that rejects quotes
// outright errors on both probes. That is input validation noise, not injection.
func TestErrorBasedSQLiIgnoresAlwaysErroringParam(t *testing.T) {
	setSQLErrorSignatures(builtinSQLErrorSignatures())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("SQL syntax error near MySQL — rejected"))
	}))
	defer srv.Close()

	base := srv.URL + "/item?id=1"
	b := newBuiltinScanContext(WebScanTarget{BaseURL: base}, true, nil)
	if out := checkErrorBasedSQLi(context.Background(), b, base); len(out) != 0 {
		t.Fatalf("always-erroring param reported as SQLi: %+v", out)
	}
}

// TestErrorBasedSQLiSkipsParameterlessTargets: the check probes existing
// parameters only — it must never invent endpoints on a plain URL.
func TestErrorBasedSQLiSkipsParameterlessTargets(t *testing.T) {
	setSQLErrorSignatures(builtinSQLErrorSignatures())
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	base := srv.URL + "/"
	b := newBuiltinScanContext(WebScanTarget{BaseURL: base}, true, nil)
	if out := checkErrorBasedSQLi(context.Background(), b, base); len(out) != 0 {
		t.Errorf("findings on a parameterless URL: %+v", out)
	}
	if hits != 0 {
		t.Errorf("sent %d requests to a parameterless target, want 0", hits)
	}
}

// TestErrorBasedSQLiCapsParameterFanout keeps the probe polite on wide URLs.
func TestErrorBasedSQLiCapsParameterFanout(t *testing.T) {
	setSQLErrorSignatures(builtinSQLErrorSignatures())
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	base := srv.URL + "/s?a=1&b=2&c=3&d=4&e=5&f=6&g=7&h=8&i=9&j=10&k=11&l=12"
	b := newBuiltinScanContext(WebScanTarget{BaseURL: base}, true, nil)
	checkErrorBasedSQLi(context.Background(), b, base)
	// One request per probed parameter; the confirmation request only fires
	// after a match, and this target never errors.
	if hits > sqliMaxParams {
		t.Errorf("sent %d requests, want at most %d", hits, sqliMaxParams)
	}
}

// TestErrorBasedSQLiRespectsPrivateTargetGuard: the SSRF boundary applies to the
// active probe too, not just the passive checks.
func TestErrorBasedSQLiRespectsPrivateTargetGuard(t *testing.T) {
	setSQLErrorSignatures(builtinSQLErrorSignatures())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("SQLi probe reached a private target with allow_private=false")
	}))
	defer srv.Close()

	base := srv.URL + "/item?id=1"
	b := newBuiltinScanContext(WebScanTarget{BaseURL: base}, false, nil)
	if out := checkErrorBasedSQLi(context.Background(), b, base); len(out) != 0 {
		t.Errorf("findings produced despite the private-target guard: %+v", out)
	}
}

// TestCurrentSignaturesFallBackToBaseline: the check must work before any feed
// has ever been downloaded.
func TestCurrentSignaturesFallBackToBaseline(t *testing.T) {
	sqlSigMu.Lock()
	sqlSigActive = nil
	sqlSigMu.Unlock()
	set := currentSQLErrorSignatures()
	if set == nil || len(set.Sigs) == 0 {
		t.Fatal("no signatures available without a feed")
	}
	if set.Source != "builtin" {
		t.Errorf("Source = %q, want builtin", set.Source)
	}
}
