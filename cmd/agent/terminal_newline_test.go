package main

import "testing"

func TestNormalizeOutputNewlines(t *testing.T) {
	var last bool
	got := normalizeOutputNewlines([]byte("a\nb\r\nc\n"), &last)
	want := "a\r\nb\r\nc\r\n"
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if last {
		t.Fatal("lastWasCR should be false after trailing LF")
	}

	last = true // previous chunk ended with CR
	got = normalizeOutputNewlines([]byte("\nOK"), &last)
	if string(got) != "\nOK" {
		t.Fatalf("CRLF split across chunks: got %q", got)
	}

	last = false
	got = normalizeOutputNewlines([]byte("already\r\nok"), &last)
	if string(got) != "already\r\nok" {
		t.Fatalf("should not double CR: %q", got)
	}
}

func TestNormalizeOutputNewlinesEmpty(t *testing.T) {
	var last bool
	if got := normalizeOutputNewlines(nil, &last); len(got) != 0 {
		t.Fatalf("nil in → %v", got)
	}
}
