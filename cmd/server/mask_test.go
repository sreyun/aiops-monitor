package main

import "testing"

func TestMaskHelpers(t *testing.T) {
	if got := maskEmail("alice@example.com"); got != "a***@example.com" {
		t.Fatalf("maskEmail=%q", got)
	}
	if got := maskPhone("13800138000"); got != "138****8000" {
		t.Fatalf("maskPhone=%q", got)
	}
	if got := maskSecret("abcdefghijklmnop"); !stringsHasStar(got) {
		t.Fatalf("maskSecret=%q", got)
	}
}

func stringsHasStar(s string) bool {
	for _, r := range s {
		if r == '*' {
			return true
		}
	}
	return false
}
