package sqltoolkit

import (
	"strings"
	"testing"
)

func TestExtractQueryShapeSelectStarLike(t *testing.T) {
	shape := ExtractQueryShape("SELECT * FROM users WHERE name LIKE '%ab%' AND status = 1")
	if !shape.ParseOK {
		t.Fatalf("parse: %s", shape.ParseError)
	}
	if !shape.SelectStar {
		t.Fatal("expected select star")
	}
	foundLike := false
	foundEq := false
	for _, p := range shape.WherePreds {
		if p.LeadingLike {
			foundLike = true
		}
		if p.Kind == PredEqual && strings.EqualFold(p.Column, "status") {
			foundEq = true
		}
	}
	if !foundLike || !foundEq {
		t.Fatalf("preds=%+v", shape.WherePreds)
	}
}

func TestAdviseIndexesSkipsCovering(t *testing.T) {
	shape := ExtractQueryShape("SELECT id FROM users WHERE status = 1 AND created_at > '2020-01-01'")
	meta := SchemaMeta{
		"users": &TableMeta{
			Name: "users", TableRows: 50000,
			Indexes: []IndexMeta{{Name: "idx_status_created", Columns: []string{"status", "created_at"}}},
		},
	}
	hints := AdviseIndexes(shape, meta)
	for _, h := range hints {
		if strings.EqualFold(h.Table, "users") {
			t.Fatalf("expected no hint when index covers, got %+v", h)
		}
	}
}

func TestAdviseIndexesSuggestsWhenMissing(t *testing.T) {
	shape := ExtractQueryShape("SELECT id FROM users WHERE status = 1")
	meta := SchemaMeta{"users": &TableMeta{Name: "users", TableRows: 100000}}
	hints := AdviseIndexes(shape, meta)
	if len(hints) == 0 {
		t.Fatal("expected index hint")
	}
	if !hints[0].Meta {
		t.Fatal("expected meta=true")
	}
}

func TestAnalyzeExplainPenalty(t *testing.T) {
	res := Analyze(AnalyzeInput{
		SQL:     "SELECT * FROM users WHERE status = 1",
		Dialect: DialectMySQL80,
		Explain: &ExplainAnalysis{
			Summary: "test",
			TableAccess: []ExplainHit{{
				Table: "users", AccessType: "ALL", Rows: 200000, Filtered: 5, FullScanRisk: true,
			}},
		},
	})
	if res.Score >= 100 {
		t.Fatalf("expected explain penalty, score=%d findings=%v", res.Score, res.Findings)
	}
	if res.Breakdown.ExplainPenalty <= 0 {
		t.Fatalf("breakdown=%+v", res.Breakdown)
	}
	if !res.Parsed {
		t.Fatal("expected parsed")
	}
}

func TestCTEOn57AST(t *testing.T) {
	r := Audit("WITH cte AS (SELECT 1 AS a) SELECT * FROM cte", DialectMySQL57)
	found := false
	for _, f := range r.Findings {
		if f.ID == "cte_57" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cte_57, findings=%v parsed=%v", r.Findings, r.Parsed)
	}
}
