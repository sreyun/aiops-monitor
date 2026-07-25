package sqltoolkit

import (
	"strings"
	"sync"

	"vitess.io/vitess/go/vt/sqlparser"
)

var (
	parserOnce sync.Once
	vitessP    *sqlparser.Parser
	parserErr  error
)

func getParser() (*sqlparser.Parser, error) {
	parserOnce.Do(func() {
		vitessP, parserErr = sqlparser.New(sqlparser.Options{})
	})
	return vitessP, parserErr
}

// ParseMySQL parses a single MySQL statement with Vitess sqlparser.
func ParseMySQL(sql string) (sqlparser.Statement, error) {
	p, err := getParser()
	if err != nil {
		return nil, err
	}
	sql = strings.TrimSpace(sql)
	sql = strings.TrimSuffix(sql, ";")
	return p.Parse(sql)
}

// ExtractQueryShape builds a QueryShape from SQL (Vitess AST). On parse failure,
// ParseOK=false and ParseError is set; MultiStmt is still detected via ';'.
func ExtractQueryShape(sql string) *QueryShape {
	raw := strings.TrimSpace(sql)
	shape := &QueryShape{}
	if raw == "" {
		shape.ParseError = "empty"
		return shape
	}
	trimmed := strings.TrimSuffix(raw, ";")
	if strings.Count(trimmed, ";") > 0 {
		shape.MultiStmt = true
	}
	stmt, err := ParseMySQL(raw)
	if err != nil {
		shape.ParseError = err.Error()
		return shape
	}
	shape.ParseOK = true
	fillShapeFromStmt(shape, stmt)
	return shape
}

func fillShapeFromStmt(shape *QueryShape, stmt sqlparser.Statement) {
	switch s := stmt.(type) {
	case *sqlparser.Select:
		shape.StmtType = "select"
		if s.With != nil {
			shape.IsCTE = true
		}
		if s.Limit != nil {
			shape.HasLimit = true
		}
		if s.Into != nil && (s.Into.Type == sqlparser.IntoOutfile || s.Into.Type == sqlparser.IntoDumpfile) {
			shape.IntoOutfile = true
		}
		if s.SelectExprs != nil {
			for _, e := range s.SelectExprs.Exprs {
				if _, ok := e.(*sqlparser.StarExpr); ok {
					shape.SelectStar = true
				}
			}
		}
		collectTables(shape, s.From)
		if s.Where != nil {
			shape.HasWhere = true
			walkExpr(shape, s.Where.Expr, false)
		}
		if s.GroupBy != nil {
			for _, e := range s.GroupBy.Exprs {
				if c := colNameOf(e); c != "" {
					shape.GroupCols = appendUnique(shape.GroupCols, c)
				}
			}
		}
		for _, o := range s.OrderBy {
			if o == nil {
				continue
			}
			if fe, ok := o.Expr.(*sqlparser.FuncExpr); ok && strings.EqualFold(fe.Name.String(), "rand") {
				shape.OrderByRand = true
			}
			if c := colNameOf(o.Expr); c != "" {
				shape.OrderCols = appendUnique(shape.OrderCols, c)
			}
		}
		if len(s.Windows) > 0 {
			shape.HasWindow = true
		}
		_ = sqlparser.Walk(func(node sqlparser.SQLNode) (kontinue bool, err error) {
			switch n := node.(type) {
			case *sqlparser.OverClause, *sqlparser.WindowSpecification:
				shape.HasWindow = true
			case *sqlparser.FuncExpr:
				// window-ish names often appear with OVER; also catch via OverClause
				_ = n
			}
			return true, nil
		}, s)
	case *sqlparser.Update:
		shape.StmtType = "update"
		if s.With != nil {
			shape.IsCTE = true
		}
		if s.Limit != nil {
			shape.HasLimit = true
		}
		collectTables(shape, s.TableExprs)
		if s.Where != nil {
			shape.HasWhere = true
			walkExpr(shape, s.Where.Expr, false)
		}
	case *sqlparser.Delete:
		shape.StmtType = "delete"
		if s.With != nil {
			shape.IsCTE = true
		}
		if s.Limit != nil {
			shape.HasLimit = true
		}
		collectTables(shape, s.TableExprs)
		if s.Where != nil {
			shape.HasWhere = true
			walkExpr(shape, s.Where.Expr, false)
		}
	case *sqlparser.Insert:
		shape.StmtType = "insert"
	case *sqlparser.Union:
		shape.StmtType = "select"
		if s.With != nil {
			shape.IsCTE = true
		}
		if s.Limit != nil {
			shape.HasLimit = true
		}
		if left, ok := s.Left.(*sqlparser.Select); ok {
			fillShapeFromStmt(shape, left)
		}
		if right, ok := s.Right.(*sqlparser.Select); ok {
			tmp := &QueryShape{}
			fillShapeFromStmt(tmp, right)
			mergeShapes(shape, tmp)
		}
	default:
		shape.StmtType = "other"
	}
}

func mergeShapes(dst, src *QueryShape) {
	if src.SelectStar {
		dst.SelectStar = true
	}
	if src.HasOr {
		dst.HasOr = true
	}
	dst.WherePreds = append(dst.WherePreds, src.WherePreds...)
	dst.Tables = append(dst.Tables, src.Tables...)
}

func collectTables(shape *QueryShape, exprs []sqlparser.TableExpr) {
	for _, te := range exprs {
		collectTableExpr(shape, te)
	}
}

func collectTableExpr(shape *QueryShape, te sqlparser.TableExpr) {
	switch t := te.(type) {
	case *sqlparser.AliasedTableExpr:
		if len(t.Hints) > 0 {
			shape.HasIndexHint = true
		}
		name, alias := tableNameOf(t)
		if name != "" {
			shape.Tables = append(shape.Tables, TableRef{Name: name, Alias: alias})
		}
	case *sqlparser.JoinTableExpr:
		shape.HasJoin = true
		if t.Condition == nil || (t.Condition.On == nil && len(t.Condition.Using) == 0) {
			// NATURAL / CROSS may omit ON; still flag missing ON for INNER/LEFT/RIGHT style joins
			jt := strings.ToLower(t.Join.ToString())
			if !strings.Contains(jt, "natural") && !strings.Contains(jt, "cross") {
				shape.JoinMissingOn = true
			}
		} else if t.Condition.On != nil {
			walkExpr(shape, t.Condition.On, true)
		}
		collectTableExpr(shape, t.LeftExpr)
		collectTableExpr(shape, t.RightExpr)
	case *sqlparser.ParenTableExpr:
		for _, e := range t.Exprs {
			collectTableExpr(shape, e)
		}
	}
}

func tableNameOf(t *sqlparser.AliasedTableExpr) (name, alias string) {
	alias = t.As.String()
	switch e := t.Expr.(type) {
	case sqlparser.TableName:
		name = e.Name.String()
	case *sqlparser.DerivedTable:
		name = ""
	}
	return name, alias
}

func walkExpr(shape *QueryShape, expr sqlparser.Expr, isJoin bool) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *sqlparser.AndExpr:
		walkExpr(shape, e.Left, isJoin)
		walkExpr(shape, e.Right, isJoin)
	case *sqlparser.OrExpr:
		shape.HasOr = true
		walkExpr(shape, e.Left, isJoin)
		walkExpr(shape, e.Right, isJoin)
	case *sqlparser.XorExpr:
		walkExpr(shape, e.Left, isJoin)
		walkExpr(shape, e.Right, isJoin)
	case *sqlparser.NotExpr:
		walkExpr(shape, e.Expr, isJoin)
	case *sqlparser.ComparisonExpr:
		if e.Operator == sqlparser.NotInOp {
			shape.HasNotIn = true
		}
		pred := predicateFromComparison(e)
		if pred.Column != "" {
			if isJoin {
				shape.JoinPreds = append(shape.JoinPreds, pred)
			} else {
				shape.WherePreds = append(shape.WherePreds, pred)
			}
		}
	case *sqlparser.BetweenExpr:
		col, tbl, wrapped := extractCol(e.Left)
		if col != "" {
			p := Predicate{Table: tbl, Column: col, Kind: PredRange, FuncWrapped: wrapped}
			if isJoin {
				shape.JoinPreds = append(shape.JoinPreds, p)
			} else {
				shape.WherePreds = append(shape.WherePreds, p)
			}
		}
	}
}

func predicateFromComparison(e *sqlparser.ComparisonExpr) Predicate {
	col, tbl, wrapped := extractCol(e.Left)
	if col == "" {
		col, tbl, wrapped = extractCol(e.Right)
	}
	p := Predicate{Table: tbl, Column: col, FuncWrapped: wrapped, Kind: PredOther}
	switch e.Operator {
	case sqlparser.EqualOp, sqlparser.NullSafeEqualOp:
		p.Kind = PredEqual
	case sqlparser.LessThanOp, sqlparser.GreaterThanOp, sqlparser.LessEqualOp, sqlparser.GreaterEqualOp:
		p.Kind = PredRange
	case sqlparser.LikeOp, sqlparser.NotLikeOp:
		p.Kind = PredLike
		if lit, ok := e.Right.(*sqlparser.Literal); ok {
			p.Literal = lit.Val
			p.LitIsString = lit.Type == sqlparser.StrVal
			if strings.HasPrefix(lit.Val, "%") {
				p.LeadingLike = true
			}
		}
	case sqlparser.InOp, sqlparser.NotInOp:
		p.Kind = PredEqual
	}
	if lit, ok := e.Right.(*sqlparser.Literal); ok && p.Kind == PredEqual {
		p.Literal = lit.Val
		p.LitIsString = lit.Type == sqlparser.StrVal
	}
	return p
}

func extractCol(expr sqlparser.Expr) (col, table string, funcWrapped bool) {
	switch e := expr.(type) {
	case *sqlparser.ColName:
		return e.Name.String(), e.Qualifier.Name.String(), false
	case *sqlparser.FuncExpr:
		funcWrapped = true
		for _, a := range e.Exprs {
			if c, t, _ := extractCol(a); c != "" {
				return c, t, true
			}
		}
	case *sqlparser.ConvertExpr:
		funcWrapped = true
		return extractCol(e.Expr)
	case *sqlparser.CastExpr:
		funcWrapped = true
		return extractCol(e.Expr)
	}
	return "", "", false
}

func colNameOf(expr sqlparser.Expr) string {
	c, _, _ := extractCol(expr)
	return c
}

func appendUnique(ss []string, x string) []string {
	for _, s := range ss {
		if strings.EqualFold(s, x) {
			return ss
		}
	}
	return append(ss, x)
}

// TableNames returns unique physical table names from the shape.
func (q *QueryShape) TableNames() []string {
	if q == nil {
		return nil
	}
	out := []string{}
	for _, t := range q.Tables {
		if t.Name == "" {
			continue
		}
		out = appendUnique(out, t.Name)
	}
	return out
}
