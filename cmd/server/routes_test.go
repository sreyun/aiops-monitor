package main

import "testing"

// TestRoutesRegister ensures ServeMux patterns do not conflict at registration
// time (Go 1.22+ panics on overlapping wildcards, e.g. {id}/preflight vs executions/{id}).
func TestRoutesRegister(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("Routes() panicked (likely ServeMux pattern conflict): %v", rec)
		}
	}()
	(&Server{}).Routes()
}
