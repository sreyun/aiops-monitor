package sqltoolkit

import "strings"

// HasDigestPlaceholders reports MySQL performance_schema DIGEST_TEXT style
// placeholders that still need real literals:
//   - unbound ? / $n (prepared-style)
//   - string literals that are exactly "?" (DIGEST encodes every string as '?')
//
// Note: a real query comparing to the character "?" is rare; treating it as a
// digest marker is the correct default for slow-SQL recovery.
func HasDigestPlaceholders(sql string) bool {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return false
	}
	if sqlHasUnboundPlaceholder(sql) {
		return true
	}
	return hasDigestQuotedPlaceholders(sql)
}

// hasDigestQuotedPlaceholders walks the SQL and returns true when a string
// literal's entire content is a single "?".
func hasDigestQuotedPlaceholders(sql string) bool {
	runes := []rune(sql)
	n := len(runes)
	i := 0
	for i < n {
		r := runes[i]
		switch {
		case r == '-' && i+1 < n && runes[i+1] == '-':
			for i < n && runes[i] != '\n' {
				i++
			}
		case r == '#':
			for i < n && runes[i] != '\n' {
				i++
			}
		case r == '/' && i+1 < n && runes[i+1] == '*':
			i += 2
			for i+1 < n && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			if i+1 < n {
				i += 2
			}
		case r == '\'' || r == '"':
			q := r
			i++
			start := i
			for i < n {
				if runes[i] == '\\' && i+1 < n {
					i += 2
					continue
				}
				if runes[i] == q {
					content := string(runes[start:i])
					i++
					if q == '\'' && i < n && runes[i] == '\'' {
						i++
						continue
					}
					if content == "?" {
						return true
					}
					break
				}
				i++
			}
		case r == '`':
			i++
			for i < n {
				if runes[i] == '`' {
					i++
					if i < n && runes[i] == '`' {
						i++
						continue
					}
					break
				}
				i++
			}
		default:
			i++
		}
	}
	return false
}

// SubstituteDigestQuotedPlaceholders replaces DIGEST-style '?' / "?" string
// literals with NULL so EXPLAIN can run when full SQL_TEXT was not recovered.
func SubstituteDigestQuotedPlaceholders(sql string) (string, bool) {
	runes := []rune(sql)
	n := len(runes)
	if n == 0 {
		return sql, false
	}
	var b strings.Builder
	b.Grow(n)
	changed := false
	i := 0
	copySpan := func(from, to int) {
		for ; from < to; from++ {
			b.WriteRune(runes[from])
		}
	}
	for i < n {
		r := runes[i]
		switch {
		case r == '-' && i+1 < n && runes[i+1] == '-':
			start := i
			for i < n && runes[i] != '\n' {
				i++
			}
			copySpan(start, i)
		case r == '#':
			start := i
			for i < n && runes[i] != '\n' {
				i++
			}
			copySpan(start, i)
		case r == '/' && i+1 < n && runes[i+1] == '*':
			start := i
			i += 2
			for i+1 < n && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			if i+1 < n {
				i += 2
			}
			copySpan(start, i)
		case r == '\'' || r == '"':
			q := r
			start := i
			i++
			contentStart := i
			for i < n {
				if runes[i] == '\\' && i+1 < n {
					i += 2
					continue
				}
				if runes[i] == q {
					content := string(runes[contentStart:i])
					i++
					if q == '\'' && i < n && runes[i] == '\'' {
						i++
						continue
					}
					if content == "?" {
						b.WriteString("NULL")
						changed = true
					} else {
						copySpan(start, i)
					}
					break
				}
				i++
			}
		case r == '`':
			start := i
			i++
			for i < n {
				if runes[i] == '`' {
					i++
					if i < n && runes[i] == '`' {
						i++
						continue
					}
					break
				}
				i++
			}
			copySpan(start, i)
		default:
			b.WriteRune(r)
			i++
		}
	}
	if !changed {
		return sql, false
	}
	return b.String(), true
}
