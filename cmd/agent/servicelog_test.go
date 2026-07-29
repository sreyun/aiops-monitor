package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotatingFileKeepsSevenGenerations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	// Tiny caps so the test stays fast: 7 files × 64 bytes.
	const maxBytes = 64
	const maxFiles = 7
	w := newRotatingFile(path, maxBytes, maxFiles)
	if w == nil {
		t.Fatal("newRotatingFile failed")
	}
	defer w.Close()

	payload := make([]byte, 40)
	for i := range payload {
		payload[i] = 'x'
	}
	// Enough writes to force more than maxFiles rotations.
	for i := 0; i < 40; i++ {
		if _, err := w.Write(payload); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	_ = w.Close()

	found := 0
	for _, name := range rotatingFileNames("agent.log", maxFiles) {
		p := filepath.Join(dir, name)
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		found++
		if st.Size() > maxBytes+16 { // allow UTF-8 BOM on first open of a generation
			t.Fatalf("%s size=%d exceeds cap", name, st.Size())
		}
	}
	if found != maxFiles {
		t.Fatalf("expected %d log files, found %d", maxFiles, found)
	}
	// No 8th backup.
	if _, err := os.Stat(filepath.Join(dir, "agent.log.7")); err == nil {
		t.Fatal("unexpected agent.log.7 retained")
	}
}

func TestRotatingFileNames(t *testing.T) {
	got := rotatingFileNames("agent.log", 7)
	if len(got) != 7 || got[0] != "agent.log" || got[6] != "agent.log.6" {
		t.Fatalf("got %#v", got)
	}
}
