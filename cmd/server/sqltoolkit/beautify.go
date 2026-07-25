package sqltoolkit

import (
	"strings"
	"unicode"
)

// Beautify formats SQL for MySQL readability (keywords uppercased, clause breaks).
func Beautify(sql string, _ Dialect) string {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return ""
	}
	tokens := tokenizeSQL(sql)
	var b strings.Builder
	indent := 0
	newlineBefore := map[string]bool{
		"SELECT": true, "FROM": true, "WHERE": true, "GROUP": true, "ORDER": true,
		"HAVING": true, "LIMIT": true, "UNION": true, "VALUES": true, "SET": true,
		"INNER": true, "LEFT": true, "RIGHT": true, "CROSS": true, "JOIN": true,
		"ON": true, "AND": true, "OR": true, "WHEN": true, "ELSE": true, "END": true,
		"WITH": true,
	}
	i := 0
	for i < len(tokens) {
		tok := tokens[i]
		up := strings.ToUpper(tok)
		// GROUP BY / ORDER BY / INNER JOIN etc.
		if i+1 < len(tokens) {
			combo := up + " " + strings.ToUpper(tokens[i+1])
			switch combo {
			case "GROUP BY", "ORDER BY", "INNER JOIN", "LEFT JOIN", "RIGHT JOIN", "CROSS JOIN", "OUTER JOIN", "UNION ALL":
				if b.Len() > 0 {
					b.WriteByte('\n')
					b.WriteString(strings.Repeat("  ", indent))
				}
				b.WriteString(combo)
				i += 2
				continue
			}
		}
		if newlineBefore[up] && b.Len() > 0 {
			if up == "AND" || up == "OR" || up == "ON" || up == "WHEN" || up == "ELSE" {
				b.WriteByte('\n')
				b.WriteString(strings.Repeat("  ", indent+1))
			} else {
				b.WriteByte('\n')
				b.WriteString(strings.Repeat("  ", indent))
			}
		} else if b.Len() > 0 && !isPunct(tok) && !isPunctLast(&b) {
			b.WriteByte(' ')
		}
		if isKeyword(tok) {
			b.WriteString(strings.ToUpper(tok))
		} else {
			b.WriteString(tok)
		}
		if tok == "(" {
			indent++
		}
		if tok == ")" && indent > 0 {
			indent--
		}
		i++
	}
	out := b.String()
	// tidy spaces before commas / parens
	out = strings.ReplaceAll(out, " ,", ",")
	out = strings.ReplaceAll(out, "( ", "(")
	out = strings.ReplaceAll(out, " )", ")")
	return strings.TrimSpace(out) + "\n"
}

func isPunct(tok string) bool {
	return tok == "," || tok == "(" || tok == ")" || tok == ";" || tok == "."
}

func isPunctLast(b *strings.Builder) bool {
	s := b.String()
	if s == "" {
		return true
	}
	last := s[len(s)-1]
	return last == '(' || last == '.' || last == ' '
}

func tokenizeSQL(sql string) []string {
	var out []string
	runes := []rune(sql)
	n := len(runes)
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < n; {
		r := runes[i]
		// comments — keep as single token
		if i+1 < n && r == '-' && runes[i+1] == '-' {
			flush()
			start := i
			for i < n && runes[i] != '\n' {
				i++
			}
			out = append(out, string(runes[start:i]))
			continue
		}
		if r == '#' {
			flush()
			start := i
			for i < n && runes[i] != '\n' {
				i++
			}
			out = append(out, string(runes[start:i]))
			continue
		}
		if i+1 < n && r == '/' && runes[i+1] == '*' {
			flush()
			start := i
			i += 2
			for i+1 < n && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			if i+1 < n {
				i += 2
			}
			out = append(out, string(runes[start:i]))
			continue
		}
		if r == '\'' || r == '"' || r == '`' {
			flush()
			q := r
			start := i
			i++
			for i < n {
				if runes[i] == '\\' && i+1 < n {
					i += 2
					continue
				}
				if runes[i] == q {
					i++
					if q == '\'' && i < n && runes[i] == '\'' {
						i++
						continue
					}
					break
				}
				i++
			}
			out = append(out, string(runes[start:i]))
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			i++
			continue
		}
		if r == ',' || r == '(' || r == ')' || r == ';' || r == '.' {
			flush()
			out = append(out, string(r))
			i++
			continue
		}
		cur.WriteRune(r)
		i++
	}
	flush()
	return out
}
