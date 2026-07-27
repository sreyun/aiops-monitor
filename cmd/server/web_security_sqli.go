package main

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Error-based SQL injection detection driven by sqlmap's DBMS fingerprint set.
//
// This is what makes the sqlmap feed worth downloading: the same regexes sqlmap
// uses to recognise a database error are applied here, so the built-in engine
// detects injection classes without Nuclei and without shipping (or running)
// sqlmap itself. A small baseline set is compiled in so the check still works
// on an air-gapped install that has never fetched a feed.

type sqlErrorSignature struct {
	DBMS string
	Re   *regexp.Regexp
}

type sqlErrorSignatureSet struct {
	Sigs   []sqlErrorSignature
	Source string // "builtin" or the file the signatures came from
}

func (s *sqlErrorSignatureSet) match(body string) (dbms string, ok bool) {
	if s == nil || body == "" {
		return "", false
	}
	for _, sig := range s.Sigs {
		if sig.Re.MatchString(body) {
			return sig.DBMS, true
		}
	}
	return "", false
}

// sqlmapErrorsXML mirrors the shape of sqlmap's data/xml/errors.xml.
type sqlmapErrorsXML struct {
	DBMS []struct {
		Value string `xml:"value,attr"`
		Error []struct {
			Regexp string `xml:"regexp,attr"`
		} `xml:"error"`
	} `xml:"dbms"`
}

// baselineSQLErrorPatterns is a conservative subset covering the databases we
// see in the field. Kept deliberately specific: a loose pattern like "error"
// would flag every application error page as a SQL injection.
var baselineSQLErrorPatterns = []struct{ DBMS, Pattern string }{
	{"MySQL", `SQL syntax.{0,80}MySQL`},
	{"MySQL", `Warning.{0,40}\Wmysqli?_`},
	{"MySQL", `valid MySQL result`},
	{"MySQL", `check the manual that (corresponds to|fits) your MySQL server version`},
	{"MariaDB", `MariaDB server version for the right syntax`},
	{"PostgreSQL", `PostgreSQL.{0,40}ERROR`},
	{"PostgreSQL", `Warning.{0,40}\Wpg_`},
	{"PostgreSQL", `valid PostgreSQL result`},
	{"PostgreSQL", `PG::SyntaxError:`},
	{"Microsoft SQL Server", `Driver.{0,40}SQL[-_ ]*Server`},
	{"Microsoft SQL Server", `Unclosed quotation mark after the character string`},
	{"Microsoft SQL Server", `Microsoft SQL Native Client error`},
	{"Microsoft SQL Server", `System\.Data\.SqlClient\.SqlException`},
	{"Oracle", `\bORA-\d{5}`},
	{"Oracle", `Oracle error`},
	{"Oracle", `quoted string not properly terminated`},
	{"SQLite", `SQLite/JDBCDriver`},
	{"SQLite", `SQLite\.Exception`},
	{"SQLite", `\[SQLITE_ERROR\]`},
	{"SQLite", `sqlite3\.OperationalError:`},
	{"IBM DB2", `CLI Driver.{0,40}DB2`},
	{"IBM DB2", `\bDB2 SQL error\b`},
	{"Sybase", `Warning.{0,40}\Wsybase_`},
	{"Informix", `Exception.{0,100}Informix`},
	{"Firebird", `Dynamic SQL Error`},
}

func builtinSQLErrorSignatures() *sqlErrorSignatureSet {
	set := &sqlErrorSignatureSet{Source: "builtin"}
	for _, p := range baselineSQLErrorPatterns {
		re, err := regexp.Compile("(?is)" + p.Pattern)
		if err != nil {
			continue
		}
		set.Sigs = append(set.Sigs, sqlErrorSignature{DBMS: p.DBMS, Re: re})
	}
	return set
}

// loadSQLErrorSignatures parses sqlmap's errors.xml out of an installed feed.
// Patterns that Go's RE2 cannot compile (sqlmap uses a few PCRE constructs) are
// skipped rather than failing the whole load.
func loadSQLErrorSignatures(feedDir string) (*sqlErrorSignatureSet, error) {
	path := filepath.Join(feedDir, "data", "xml", "errors.xml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc sqlmapErrorsXML
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	set := &sqlErrorSignatureSet{Source: path}
	seen := map[string]bool{}
	for _, d := range doc.DBMS {
		dbms := strings.TrimSpace(d.Value)
		if dbms == "" {
			dbms = "unknown"
		}
		for _, e := range d.Error {
			pat := strings.TrimSpace(e.Regexp)
			if pat == "" || seen[pat] {
				continue
			}
			seen[pat] = true
			re, err := regexp.Compile("(?is)" + pat)
			if err != nil {
				continue
			}
			set.Sigs = append(set.Sigs, sqlErrorSignature{DBMS: dbms, Re: re})
		}
	}
	if len(set.Sigs) == 0 {
		return nil, os.ErrNotExist
	}
	sort.SliceStable(set.Sigs, func(i, j int) bool { return set.Sigs[i].DBMS < set.Sigs[j].DBMS })
	return set, nil
}

var (
	sqlSigMu     sync.RWMutex
	sqlSigActive *sqlErrorSignatureSet
)

func setSQLErrorSignatures(set *sqlErrorSignatureSet) {
	if set == nil || len(set.Sigs) == 0 {
		return
	}
	sqlSigMu.Lock()
	sqlSigActive = set
	sqlSigMu.Unlock()
}

func currentSQLErrorSignatures() *sqlErrorSignatureSet {
	sqlSigMu.RLock()
	set := sqlSigActive
	sqlSigMu.RUnlock()
	if set != nil {
		return set
	}
	set = builtinSQLErrorSignatures()
	setSQLErrorSignatures(set)
	return set
}

// reloadSQLErrorSignatures swaps in the feed-provided set when one is installed.
// Called at boot and after every feed update so a refresh takes effect without
// restarting the server.
func (s *Server) reloadSQLErrorSignatures() {
	dir := s.feedSourcePath("sqlmap-signatures")
	if dir == "" {
		setSQLErrorSignatures(builtinSQLErrorSignatures())
		return
	}
	set, err := loadSQLErrorSignatures(dir)
	if err != nil {
		setSQLErrorSignatures(builtinSQLErrorSignatures())
		return
	}
	setSQLErrorSignatures(set)
}

// --- checks ---

const (
	// sqliMaxParams bounds the active probe: a scan must stay polite on targets
	// with dozens of query parameters.
	sqliMaxParams = 8
	// sqliBreakPayload is a single quote — it breaks a naive string literal.
	sqliBreakPayload = "'"
	// sqliRepairPayload closes the literal again. An error that appears with one
	// quote but disappears with two is the classic error-based confirmation and
	// filters out apps that simply reject quotes.
	sqliRepairPayload = "''"
)

// checkDBMSErrorLeak is passive: it looks for database errors already present in
// the baseline response. Costs nothing and catches leaky error pages.
func checkDBMSErrorLeak(base string, body []byte) []WebFinding {
	dbms, ok := currentSQLErrorSignatures().match(string(body))
	if !ok {
		return nil
	}
	return []WebFinding{builtinFinding("dbms-error-disclosure",
		"页面泄露数据库错误信息", "medium", base,
		"响应正文中包含 "+dbms+" 的原始错误信息。攻击者可据此推断数据库类型、表结构与注入点位置。",
		"关闭生产环境的详细错误回显，统一返回通用错误页，并将原始异常仅写入服务端日志",
		"disclosure", "info-leak", "sqlmap-signatures")}
}

// checkErrorBasedSQLi actively probes each query parameter. It only runs when
// the target URL already carries parameters — it never invents endpoints — and
// sends two harmless requests per parameter.
func checkErrorBasedSQLi(ctx context.Context, b *builtinScanContext, base string) []WebFinding {
	u, err := url.Parse(base)
	if err != nil {
		return nil
	}
	q := u.Query()
	if len(q) == 0 {
		return nil
	}
	names := make([]string, 0, len(q))
	for k := range q {
		names = append(names, k)
	}
	sort.Strings(names) // deterministic order, and a stable cap when truncating
	if len(names) > sqliMaxParams {
		names = names[:sqliMaxParams]
	}

	sigs := currentSQLErrorSignatures()
	var out []WebFinding
	for _, name := range names {
		if ctx.Err() != nil {
			break
		}
		orig := q.Get(name)

		broken := cloneQueryWith(q, name, orig+sqliBreakPayload)
		u.RawQuery = broken.Encode()
		_, body, err := b.do(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			continue
		}
		dbms, hit := sigs.match(string(body))
		if !hit {
			continue
		}

		repaired := cloneQueryWith(q, name, orig+sqliRepairPayload)
		u.RawQuery = repaired.Encode()
		_, body2, err := b.do(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			continue
		}
		if _, stillBroken := sigs.match(string(body2)); stillBroken {
			// The error survives a balanced quote, so it is input validation
			// noise rather than a broken SQL literal. Report it as a leak only.
			continue
		}
		out = append(out, builtinFinding("error-based-sqli",
			"报错型 SQL 注入", "critical", base,
			"参数 "+name+" 注入单引号后返回 "+dbms+" 语法错误，补全引号后错误消失，符合报错型 SQL 注入特征。",
			"使用参数化查询（预编译语句）替换字符串拼接；对输入做类型与白名单校验；关闭生产环境错误回显",
			"sqli", "injection", "sqlmap-signatures"))
	}
	u.RawQuery = q.Encode() // leave the caller's URL untouched
	return out
}

func cloneQueryWith(src url.Values, key, val string) url.Values {
	out := make(url.Values, len(src))
	for k, v := range src {
		out[k] = append([]string(nil), v...)
	}
	out.Set(key, val)
	return out
}
