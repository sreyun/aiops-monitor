package main

import "testing"

func TestPasswordPolicy(t *testing.T) {
	good := []string{"Abcd123!", "P@ssw0rd", "aB3$aB3$", "Zx9#mnop", "长密码Ab1!x"}
	for _, p := range good {
		if !validatePasswordStrength(p) {
			t.Errorf("should accept strong password %q", p)
		}
	}
	bad := map[string]string{
		"":           "empty",
		"Ab1!xy":     "too short (6)",
		"abcdefg1!":  "no uppercase",
		"ABCDEFG1!":  "no lowercase",
		"Abcdefgh!":  "no digit",
		"Abcdefg12":  "no special",
		"abcdefgh":   "only lowercase",
	}
	for p, why := range bad {
		if validatePasswordStrength(p) {
			t.Errorf("should reject %q (%s)", p, why)
		}
	}
}

func TestRecordingPersistence(t *testing.T) {
	dir := t.TempDir()
	m := newTermManager()
	m.recDir = dir
	arch := termArchive{
		info:      termSessionInfo{ID: "sess1", Hostname: "web-01", Operator: "alice", Frames: 2},
		recording: []termRecordFrame{{Ts: 1, Type: "output", Data: "aGk="}, {Ts: 2, Type: "input", Data: "eA=="}},
	}
	m.persistRecording(arch)

	// Read the frames straight back from the file.
	if got := m.readRecordingFile("sess1"); len(got) != 2 {
		t.Fatalf("readRecordingFile: expected 2 frames, got %d", len(got))
	}

	// A fresh manager (simulating a restart) indexes the persisted recording...
	m2 := newTermManager()
	m2.loadRecordings(dir)
	var found *termSessionInfo
	for _, s := range m2.listSessions() {
		if s.ID == "sess1" {
			cp := s
			found = &cp
		}
	}
	if found == nil {
		t.Fatal("loadRecordings should index the persisted session after restart")
	}
	if found.Frames != 2 || found.Active {
		t.Errorf("restored session info wrong: frames=%d active=%v", found.Frames, found.Active)
	}
	// ...and replay reads the frames lazily from the file.
	if got := m2.getRecording("sess1"); len(got) != 2 {
		t.Fatalf("getRecording after restart should read 2 frames from file, got %d", len(got))
	}
	// Unknown session → nil (no panic).
	if m2.getRecording("nope") != nil {
		t.Error("unknown session should return nil")
	}
}
