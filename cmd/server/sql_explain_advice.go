package main

import (
	"strings"

	"aiops-monitor/cmd/server/sqltoolkit"
)

// attachExplainAdvice enriches an EXPLAIN API payload with detailed analysis,
// index DDL suggestions and optimization tips (shown under EXPLAIN JSON in UI).
func attachExplainAdvice(out map[string]any, c MySQLConnection, schema, sqlText string, analysis *sqltoolkit.ExplainAnalysis) {
	if out == nil {
		return
	}
	d := dialectForConn(c)
	shape := sqltoolkit.ExtractQueryShape(sqlText)
	tables := []string{}
	if shape != nil {
		tables = shape.TableNames()
	}
	if len(tables) == 0 && analysis != nil {
		seen := map[string]bool{}
		for _, h := range analysis.TableAccess {
			t := strings.TrimSpace(h.Table)
			if t == "" {
				continue
			}
			key := strings.ToLower(t)
			if seen[key] {
				continue
			}
			seen[key] = true
			tables = append(tables, t)
		}
	}
	var meta sqltoolkit.SchemaMeta
	if driverOf(c) != "postgres" && len(tables) > 0 {
		if m, err := mysqlFetchMetadataInSchema(c, schema, tables); err == nil {
			meta = m
		}
	}
	report := sqltoolkit.BuildExplainReport(sqlText, d, analysis, meta)
	out["detail"] = report
	out["findings"] = report.Findings
	out["index_hints"] = report.IndexHints
	out["suggestions"] = report.Suggestions
	if report.RewrittenSQL != "" {
		out["rewritten_sql"] = report.RewrittenSQL
	}
}
