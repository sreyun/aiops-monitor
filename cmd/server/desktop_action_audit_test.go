package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAuditDeskActionNoPasswordLeak(t *testing.T) {
	s := &Server{store: NewStore()}
	payload, _ := json.Marshal(map[string]any{
		"action": "type_text",
		"text":   "P@ssw0rd-超级机密",
		"enter":  true,
	})
	if !auditDeskAction(s, "alice", "1.2.3.4", "hmsrv18", payload) {
		t.Fatal("expected audit to accept type_text")
	}
	logs := s.store.RecentActivity()
	if len(logs) == 0 {
		t.Fatal("no log written")
	}
	msg := logs[0].Message
	if strings.Contains(msg, "P@ssw0rd") || strings.Contains(msg, "超级机密") {
		t.Fatalf("password leaked into audit log: %q", msg)
	}
	if !strings.Contains(msg, "type_text") || !strings.Contains(msg, "长度=") {
		t.Fatalf("unexpected audit message: %q", msg)
	}
}

func TestAuditDeskActionCAD(t *testing.T) {
	s := &Server{store: NewStore()}
	payload, _ := json.Marshal(map[string]any{"action": "cad"})
	if !auditDeskAction(s, "bob", "10.0.0.1", "host1", payload) {
		t.Fatal("cad should be accepted")
	}
	if auditDeskAction(s, "bob", "10.0.0.1", "host1", []byte(`{"action":"nope"}`)) {
		t.Fatal("unknown action should be rejected")
	}
}
