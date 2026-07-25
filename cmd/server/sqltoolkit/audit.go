package sqltoolkit

import (
	"regexp"
	"strings"
)

var (
	reSelectStar      = regexp.MustCompile(`(?i)\bselect\s+\*`)
	reLikePrefixWild  = regexp.MustCompile(`(?i)\blike\s+'%`)
	reLikePrefixWild2 = regexp.MustCompile(`(?i)\blike\s+"%`)
	reOrderRand       = regexp.MustCompile(`(?i)\border\s+by\s+rand\s*\(`)
	reDateFuncCol     = regexp.MustCompile(`(?i)\b(where|and|or)\s+(year|month|day|date|date_format|from_unixtime)\s*\(`)
	reHasOn           = regexp.MustCompile(`(?i)\bon\b`)
	reUpdateNoWhere   = regexp.MustCompile(`(?i)^\s*update\b`)
	reDeleteNoWhere   = regexp.MustCompile(`(?i)^\s*delete\b`)
	reHasWhere        = regexp.MustCompile(`(?i)\bwhere\b`)
	reHasLimit        = regexp.MustCompile(`(?i)\blimit\b`)
	reOrInWhere       = regexp.MustCompile(`(?i)\bwhere\b[\s\S]{0,800}\bor\b`)
	reNotInSelect     = regexp.MustCompile(`(?i)\bnot\s+in\s*\(`)
	reSelectNoLimit   = regexp.MustCompile(`(?i)^\s*(with\b[\s\S]+?\)\s*)?select\b`)
	reLeadingOr       = regexp.MustCompile(`(?i)\bwhere\b[\s\S]{0,200}\bor\b`)
	reImplicitConv    = regexp.MustCompile(`(?i)\b(where|and|or)\s+([a-z0-9_` + "`" + `.]+)\s*=\s*'[0-9]+'`)
	reHintForce       = regexp.MustCompile(`(?i)\bforce\s+index\b|\buse\s+index\b`)
)

// Audit prefers Vitess AST rules; falls back to regex when parse fails.
func Audit(sql string, d Dialect) AuditResult {
	raw := strings.TrimSpace(sql)
	if raw == "" {
		return AuditResult{Findings: []Finding{{ID: "empty", Level: "crit", Title: "空 SQL", Detail: "未提供语句"}}, Score: 0}
	}
	shape := ExtractQueryShape(raw)
	if shape.ParseOK {
		findings := AuditAST(shape, d)
		return AuditResult{Findings: findings, Score: ScoreFindings(findings), Parsed: true}
	}
	res := AuditRegex(raw, d)
	res.Findings = append([]Finding{{
		ID: "parse_error", Level: "info", Title: "AST 解析失败，已回退正则审核",
		Detail: shape.ParseError, Rule: "parse_error",
	}}, res.Findings...)
	res.Score = ScoreFindings(res.Findings)
	res.Parsed = false
	return res
}

// AuditRegex is the legacy heuristic engine (fallback).
func AuditRegex(sql string, d Dialect) AuditResult {
	raw := strings.TrimSpace(sql)
	stripped := StripCommentsAndStrings(raw)
	compact := compactSpaces(stripped)
	var findings []Finding

	add := func(id, level, title, detail, suggest string) {
		findings = append(findings, Finding{ID: id, Level: level, Title: title, Detail: detail, Rule: id, Suggest: suggest})
	}

	if raw == "" {
		return AuditResult{Findings: []Finding{{ID: "empty", Level: "crit", Title: "空 SQL", Detail: "未提供语句"}}, Score: 0}
	}

	if strings.Count(strings.TrimSuffix(strings.TrimSpace(raw), ";"), ";") > 0 {
		add("multi_stmt", "crit", "多语句", "检测到多个语句分隔符，存在注入/误执行风险", "每次只提交一条 SQL")
	}

	if reSelectStar.MatchString(stripped) {
		add("select_star", "warn", "SELECT *", "查出全部列增加 IO 与网络开销，也可能阻碍覆盖索引", "改为明确列清单")
	}

	if reLikePrefixWild.MatchString(raw) || reLikePrefixWild2.MatchString(raw) {
		add("like_leading_wildcard", "warn", "LIKE 前导通配", "LIKE '%xxx' 通常无法使用 BTree 索引", "改为后缀模糊、全文索引或搜索引擎")
	}

	if reOrderRand.MatchString(stripped) {
		add("order_by_rand", "crit", "ORDER BY RAND()", "会对结果集做昂贵排序，大数据量下极慢", "用随机主键区间或应用层抽样")
	}

	if reDateFuncCol.MatchString(stripped) {
		add("func_wrap_column", "warn", "条件中对列使用函数", "YEAR(col)/DATE(col) 等会使索引失效", "改为范围条件：col >= .. AND col < ..")
	}

	kw := FirstKeyword(raw)
	if kw == "update" && reUpdateNoWhere.MatchString(compact) && !reHasWhere.MatchString(stripped) {
		add("update_no_where", "crit", "UPDATE 无 WHERE", "可能更新全表", "必须加 WHERE，或先 SELECT 确认影响行数")
	}
	if kw == "delete" && reDeleteNoWhere.MatchString(compact) && !reHasWhere.MatchString(stripped) {
		add("delete_no_where", "crit", "DELETE 无 WHERE", "可能删除全表", "必须加 WHERE；生产建议先 EXPLAIN / 事务")
	}

	if (kw == "select" || kw == "with") && reSelectNoLimit.MatchString(compact) && !reHasLimit.MatchString(stripped) {
		add("select_no_limit", "info", "SELECT 无 LIMIT", "大表全量返回可能拖垮客户端与连接", "开发调试请加 LIMIT；导出走批处理")
	}

	if reOrInWhere.MatchString(stripped) || reLeadingOr.MatchString(stripped) {
		add("or_in_where", "warn", "WHERE 使用 OR", "OR 常导致索引合并或回退全表扫描", "改为 UNION ALL / IN 列表，或复合索引覆盖")
	}

	if reNotInSelect.MatchString(stripped) {
		add("not_in", "warn", "NOT IN 子查询", "NOT IN 遇 NULL 语义陷阱且优化器支持弱", "改用 NOT EXISTS 或 LEFT JOIN ... IS NULL")
	}

	if reImplicitConv.MatchString(raw) {
		add("implicit_conversion", "warn", "可能的隐式类型转换", "数字列与字符串比较可能导致索引失效", "两侧类型保持一致（去掉引号或显式 CAST）")
	}

	if strings.Contains(strings.ToLower(stripped), " join ") {
		if !reHasOn.MatchString(stripped) && !strings.Contains(strings.ToLower(stripped), " using ") {
			add("join_no_on", "crit", "JOIN 缺少 ON/USING", "可能产生笛卡尔积", "为每个 JOIN 写明关联条件")
		}
	}

	if reHintForce.MatchString(stripped) {
		add("index_hint", "info", "使用了 INDEX Hint", "FORCE/USE INDEX 会降低优化器灵活性", "确认统计信息后再长期保留 Hint")
	}

	if d == DialectMySQL57 && regexp.MustCompile(`(?i)\bwith\b`).MatchString(stripped) && FirstKeyword(raw) == "with" {
		add("cte_57", "warn", "MySQL 5.7 不支持 CTE", "WITH（含 RECURSIVE）需 8.0+", "升级到 8.0 或改写为临时表/派生表")
	}
	if d == DialectMySQL57 && regexp.MustCompile(`(?i)\bover\s*\(`).MatchString(stripped) {
		add("window_57", "warn", "MySQL 5.7 不支持窗口函数", "OVER() 需 8.0+", "升级 8.0 或用用户变量/自连接改写")
	}

	if ForbiddenWrite(raw) && (kw == "select" || kw == "with" || kw == "explain") {
		if strings.Contains(strings.ToLower(stripped), "into outfile") || strings.Contains(strings.ToLower(stripped), "into dumpfile") {
			add("into_outfile", "crit", "INTO OUTFILE/DUMPFILE", "写文件操作有安全风险，工具侧将拒绝连库执行", "去掉 INTO OUTFILE")
		}
	}

	if findings == nil {
		findings = []Finding{}
	}
	return AuditResult{Findings: findings, Score: ScoreFindings(findings)}
}
