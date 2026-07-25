package sqltoolkit

import (
	"fmt"
	"strings"
)

// AuditAST runs rules against a QueryShape (Vitess-derived).
func AuditAST(shape *QueryShape, d Dialect) []Finding {
	var findings []Finding
	add := func(id, level, title, detail, suggest string) {
		findings = append(findings, Finding{ID: id, Level: level, Title: title, Detail: detail, Rule: id, Suggest: suggest})
	}
	if shape == nil {
		return findings
	}
	if shape.MultiStmt {
		add("multi_stmt", "crit", "多语句", "检测到多个语句分隔符，存在注入/误执行风险", "每次只提交一条 SQL")
	}
	if shape.SelectStar {
		add("select_star", "warn", "SELECT *", "查出全部列增加 IO 与网络开销，也可能阻碍覆盖索引", "改为明确列清单")
	}
	for _, p := range shape.WherePreds {
		if p.LeadingLike {
			add("like_leading_wildcard", "warn", "LIKE 前导通配", "LIKE '%xxx' 通常无法使用 BTree 索引", "改为后缀模糊、全文索引或搜索引擎")
			break
		}
	}
	if shape.OrderByRand {
		add("order_by_rand", "crit", "ORDER BY RAND()", "会对结果集做昂贵排序，大数据量下极慢", "用随机主键区间或应用层抽样")
	}
	for _, p := range shape.WherePreds {
		if p.FuncWrapped {
			add("func_wrap_column", "warn", "条件中对列使用函数", "YEAR(col)/DATE(col) 等会使索引失效", "改为范围条件：col >= .. AND col < ..")
			break
		}
	}
	if shape.StmtType == "update" && !shape.HasWhere {
		add("update_no_where", "crit", "UPDATE 无 WHERE", "可能更新全表", "必须加 WHERE，或先 SELECT 确认影响行数")
	}
	if shape.StmtType == "delete" && !shape.HasWhere {
		add("delete_no_where", "crit", "DELETE 无 WHERE", "可能删除全表", "必须加 WHERE；生产建议先 EXPLAIN / 事务")
	}
	if (shape.StmtType == "select") && !shape.HasLimit {
		add("select_no_limit", "info", "SELECT 无 LIMIT", "大表全量返回可能拖垮客户端与连接", "开发调试请加 LIMIT；导出走批处理")
	}
	if shape.HasOr {
		add("or_in_where", "warn", "WHERE 使用 OR", "OR 常导致索引合并或回退全表扫描", "改为 UNION ALL / IN 列表，或复合索引覆盖")
	}
	if shape.HasNotIn {
		add("not_in", "warn", "NOT IN 子查询", "NOT IN 遇 NULL 语义陷阱且优化器支持弱", "改用 NOT EXISTS 或 LEFT JOIN ... IS NULL")
	}
	for _, p := range shape.WherePreds {
		if p.Kind == PredEqual && p.LitIsString && looksNumeric(p.Literal) {
			add("implicit_conversion", "warn", "可能的隐式类型转换",
				fmt.Sprintf("列 %s 与数字字符串字面量比较可能导致索引失效", p.Column),
				"两侧类型保持一致（去掉引号或显式 CAST）")
			break
		}
	}
	if shape.JoinMissingOn {
		add("join_no_on", "crit", "JOIN 缺少 ON/USING", "可能产生笛卡尔积", "为每个 JOIN 写明关联条件")
	}
	if shape.HasIndexHint {
		add("index_hint", "info", "使用了 INDEX Hint", "FORCE/USE INDEX 会降低优化器灵活性", "确认统计信息后再长期保留 Hint")
	}
	if d == DialectMySQL57 && shape.IsCTE {
		add("cte_57", "warn", "MySQL 5.7 不支持 CTE", "WITH（含 RECURSIVE）需 8.0+", "升级到 8.0 或改写为临时表/派生表")
	}
	if d == DialectMySQL57 && shape.HasWindow {
		add("window_57", "warn", "MySQL 5.7 不支持窗口函数", "OVER() 需 8.0+", "升级 8.0 或用用户变量/自连接改写")
	}
	if shape.IntoOutfile {
		add("into_outfile", "crit", "INTO OUTFILE/DUMPFILE", "写文件操作有安全风险，工具侧将拒绝连库执行", "去掉 INTO OUTFILE")
	}
	return findings
}

func looksNumeric(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r >= '0' && r <= '9' {
			continue
		}
		if i > 0 && (r == '.' || r == 'e' || r == 'E' || r == '-') {
			continue
		}
		return false
	}
	return true
}

// AuditMeta adds metadata-driven findings (implicit conversion vs column types, missing index on large tables).
func AuditMeta(shape *QueryShape, meta SchemaMeta, indexHints []IndexHint) []Finding {
	var findings []Finding
	if shape == nil || meta == nil {
		return findings
	}
	add := func(id, level, title, detail, suggest string) {
		findings = append(findings, Finding{ID: id, Level: level, Title: title, Detail: detail, Rule: id, Suggest: suggest})
	}
	for _, p := range shape.WherePreds {
		if p.Kind != PredEqual || !p.LitIsString || !looksNumeric(p.Literal) {
			continue
		}
		tm := resolveTableMeta(meta, shape, p.Table)
		if tm == nil {
			continue
		}
		col := findColumn(tm, p.Column)
		if col == nil {
			continue
		}
		if isNumericSQLType(col.DataType) {
			add("meta_implicit_conversion", "warn", "元数据确认：隐式类型转换",
				fmt.Sprintf("列 %s.%s 类型为 %s，却与字符串数字比较", tm.Name, col.Name, col.DataType),
				"去掉引号或 CAST 两侧类型一致")
		}
	}
	for _, h := range indexHints {
		if !h.Meta || len(h.Columns) == 0 {
			continue
		}
		tm := meta[normalizeIdent(h.Table)]
		if tm == nil {
			continue
		}
		if tm.TableRows >= 10000 {
			add("meta_missing_index_large", "warn", "大表缺少匹配索引",
				fmt.Sprintf("表 %s 约 %d 行，建议索引 (%v)", tm.Name, tm.TableRows, h.Columns),
				h.DDL)
		}
	}
	return findings
}

// AuditExplain converts EXPLAIN analysis into findings.
func AuditExplain(ex *ExplainAnalysis) []Finding {
	var findings []Finding
	if ex == nil {
		return findings
	}
	add := func(id, level, title, detail, suggest string) {
		findings = append(findings, Finding{ID: id, Level: level, Title: title, Detail: detail, Rule: id, Suggest: suggest})
	}
	for _, h := range ex.TableAccess {
		if h.FullScanRisk {
			add("explain_full_scan", "warn", "EXPLAIN：全表/全索引扫描风险",
				fmt.Sprintf("表 %s access_type=%s rows≈%.0f", h.Table, h.AccessType, h.Rows),
				"补充选择性 WHERE / 合适索引")
		}
		if h.UsingFilesort {
			add("explain_filesort", "info", "EXPLAIN：Using filesort",
				fmt.Sprintf("表 %s 可能额外排序", h.Table),
				"让 ORDER BY 列匹配索引前缀")
		}
		if h.UsingTemp {
			add("explain_temp_table", "warn", "EXPLAIN：Using temporary",
				fmt.Sprintf("表 %s 使用临时表", h.Table),
				"简化 GROUP BY / DISTINCT 或覆盖索引")
		}
		if h.Rows >= 100000 || (h.FullScanRisk && h.Filtered > 0 && h.Filtered < 10) {
			add("explain_high_rows", "warn", "EXPLAIN：高 rows / 低过滤率",
				fmt.Sprintf("表 %s rows≈%.0f filtered≈%.1f%%", h.Table, h.Rows, h.Filtered),
				"收紧谓词或增加高选择度索引")
		}
	}
	return dedupeFindings(findings)
}

func resolveTableMeta(meta SchemaMeta, shape *QueryShape, tableOrAlias string) *TableMeta {
	if tableOrAlias != "" {
		if tm := meta[normalizeIdent(tableOrAlias)]; tm != nil {
			return tm
		}
		for _, t := range shape.Tables {
			if equalIdent(t.Alias, tableOrAlias) || equalIdent(t.Name, tableOrAlias) {
				return meta[normalizeIdent(t.Name)]
			}
		}
	}
	if len(shape.Tables) == 1 {
		return meta[normalizeIdent(shape.Tables[0].Name)]
	}
	return nil
}

func findColumn(tm *TableMeta, name string) *ColumnMeta {
	for i := range tm.Columns {
		if equalIdent(tm.Columns[i].Name, name) {
			return &tm.Columns[i]
		}
	}
	return nil
}

func isNumericSQLType(t string) bool {
	switch normalizeIdent(t) {
	case "int", "integer", "bigint", "smallint", "tinyint", "mediumint",
		"decimal", "numeric", "float", "double", "real", "bit":
		return true
	default:
		return false
	}
}

func normalizeIdent(s string) string {
	return strings.ToLower(strings.Trim(s, "`"))
}

func equalIdent(a, b string) bool {
	return normalizeIdent(a) == normalizeIdent(b)
}

func dedupeFindings(in []Finding) []Finding {
	seen := map[string]bool{}
	out := make([]Finding, 0, len(in))
	for _, f := range in {
		key := f.ID + "|" + f.Detail
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	return out
}
