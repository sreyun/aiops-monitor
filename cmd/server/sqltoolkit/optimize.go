package sqltoolkit

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	reFromTable = regexp.MustCompile(`(?i)\bfrom\s+([a-zA-Z0-9_` + "`" + `.]+)`)
	reWhereEq   = regexp.MustCompile(`(?i)\bwhere\b([\s\S]+?)(?:\bgroup\b|\border\b|\blimit\b|$)`)
	reEqCol     = regexp.MustCompile(`(?i)([a-zA-Z0-9_` + "`" + `.]+)\s*=\s*(?:\?|'[^']*'|[0-9]+)`)
	reRangeCol  = regexp.MustCompile(`(?i)([a-zA-Z0-9_` + "`" + `.]+)\s*(?:>=|<=|>|<|between)`)
)

// Optimize produces static rewrite advice and index templates for MySQL.
func Optimize(sql string, d Dialect) OptimizeResult {
	shape := ExtractQueryShape(sql)
	return OptimizeWithMeta(sql, d, shape, nil)
}

// OptimizeWithMeta uses AST shape + optional SchemaMeta for SQLAdvisor-style hints.
func OptimizeWithMeta(sql string, d Dialect, shape *QueryShape, meta SchemaMeta) OptimizeResult {
	raw := strings.TrimSpace(sql)
	audit := Audit(raw, d)
	out := OptimizeResult{Suggestions: []Finding{}, IndexHints: []IndexHint{}}

	rewritten := Beautify(raw, d)
	changed := false

	selectStar := shape != nil && shape.SelectStar
	if !selectStar {
		selectStar = reSelectStar.MatchString(StripCommentsAndStrings(raw))
	}
	if selectStar {
		out.Suggestions = append(out.Suggestions, Finding{
			ID: "rewrite_select_star", Level: "warn", Title: "避免 SELECT *",
			Detail:  "请按业务列出所需列，便于覆盖索引与减少回表",
			Suggest: "SELECT col1, col2, ... FROM ...",
		})
	}

	stripped := StripCommentsAndStrings(raw)
	kw := FirstKeyword(raw)
	hasLimit := shape != nil && shape.HasLimit
	if !hasLimit {
		hasLimit = reHasLimit.MatchString(stripped)
	}
	if (kw == "select" || kw == "with" || (shape != nil && shape.StmtType == "select")) && !hasLimit {
		trim := strings.TrimSpace(raw)
		trim = strings.TrimSuffix(trim, ";")
		rewritten = Beautify(trim+"\nLIMIT 1000", d)
		changed = true
		out.Suggestions = append(out.Suggestions, Finding{
			ID: "add_limit", Level: "info", Title: "已建议补充 LIMIT 1000",
			Detail:  "防止误查大表；上线前按业务调整或删除",
			Suggest: "LIMIT 1000",
		})
	}

	// Prefer AST advisor; fall back to regex templates
	if shape != nil && shape.ParseOK {
		out.IndexHints = AdviseIndexes(shape, meta)
	}
	if len(out.IndexHints) == 0 {
		out.IndexHints = regexIndexHints(stripped)
	}
	for _, h := range out.IndexHints {
		out.Suggestions = append(out.Suggestions, Finding{
			ID: "index_template", Level: "info", Title: "索引建议",
			Detail:  fmt.Sprintf("表 %s 建议考虑索引 (%s)：%s", h.Table, strings.Join(h.Columns, ", "), h.Reason),
			Suggest: h.DDL,
		})
	}

	leadingLike := false
	if shape != nil {
		for _, p := range shape.WherePreds {
			if p.LeadingLike {
				leadingLike = true
				break
			}
		}
	}
	if leadingLike || reLikePrefixWild.MatchString(raw) || reLikePrefixWild2.MatchString(raw) {
		out.Suggestions = append(out.Suggestions, Finding{
			ID: "like_optimize", Level: "warn", Title: "前导模糊查询",
			Detail: "BTree 无法高效支持；可考虑倒排/全文（MySQL FULLTEXT）或外部搜索",
		})
	}

	if d == DialectMySQL80 {
		out.Suggestions = append(out.Suggestions, Finding{
			ID: "mysql80_features", Level: "info", Title: "MySQL 8.0 可用能力",
			Detail:  "可考虑 CTE、窗口函数、降序索引、不可见索引做灰度验证",
			Suggest: "CREATE INDEX ... (col DESC); / ALTER TABLE ... ALTER INDEX ... INVISIBLE",
		})
	} else {
		out.Suggestions = append(out.Suggestions, Finding{
			ID: "mysql57_compat", Level: "info", Title: "MySQL 5.7 兼容提示",
			Detail: "避免 CTE/窗口函数；派生表别名必填；注意 ONLY_FULL_GROUP_BY",
		})
	}

	for _, f := range audit.Findings {
		if f.Level == "info" {
			continue
		}
		dup := false
		for _, s := range out.Suggestions {
			if s.ID == f.ID || s.Title == f.Title {
				dup = true
				break
			}
		}
		if !dup {
			out.Suggestions = append(out.Suggestions, f)
		}
	}

	if changed {
		out.RewrittenSQL = rewritten
	} else {
		out.RewrittenSQL = Beautify(raw, d)
	}
	return out
}

func regexIndexHints(stripped string) []IndexHint {
	table := ""
	if m := reFromTable.FindStringSubmatch(stripped); len(m) > 1 {
		table = strings.Trim(m[1], "`")
		if i := strings.LastIndex(table, "."); i >= 0 {
			table = table[i+1:]
		}
	}
	eqCols := []string{}
	rangeCols := []string{}
	wherePart := stripped
	if m := reWhereEq.FindStringSubmatch(stripped); len(m) > 1 {
		wherePart = m[1]
	}
	for _, m := range reEqCol.FindAllStringSubmatch(wherePart, -1) {
		col := stripTablePrefix(m[1])
		if col != "" && !containsStr(eqCols, col) {
			eqCols = append(eqCols, col)
		}
	}
	for _, m := range reRangeCol.FindAllStringSubmatch(wherePart, -1) {
		col := stripTablePrefix(m[1])
		if col != "" && !containsStr(rangeCols, col) && !containsStr(eqCols, col) {
			rangeCols = append(rangeCols, col)
		}
	}
	idxCols := append(append([]string{}, eqCols...), rangeCols...)
	if len(idxCols) == 0 || table == "" {
		return nil
	}
	if len(idxCols) > 4 {
		idxCols = idxCols[:4]
	}
	ddl := fmt.Sprintf("ALTER TABLE `%s` ADD INDEX idx_%s (%s);",
		table, strings.Join(idxCols, "_"), quoteCols(idxCols))
	return []IndexHint{{
		Table: table, Columns: idxCols,
		Reason: "等值条件优先、范围条件靠后的联合索引模板（需结合基数与区分度验证）",
		DDL:    ddl,
	}}
}

func stripTablePrefix(col string) string {
	col = strings.Trim(col, "`")
	if i := strings.LastIndex(col, "."); i >= 0 {
		col = col[i+1:]
	}
	low := strings.ToLower(col)
	if low == "and" || low == "or" || low == "where" || low == "on" {
		return ""
	}
	return col
}

func quoteCols(cols []string) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = "`" + c + "`"
	}
	return strings.Join(parts, ", ")
}

func containsStr(ss []string, x string) bool {
	for _, s := range ss {
		if strings.EqualFold(s, x) {
			return true
		}
	}
	return false
}
