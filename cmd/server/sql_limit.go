package main

// clampSQLReadLimit bounds dashboard/datasource SELECT result caps.
// The value is embedded as a numeric SQL literal (never user text).
func clampSQLReadLimit(limit int) int {
	if limit <= 0 || limit > 2000 {
		return 200
	}
	return limit
}
