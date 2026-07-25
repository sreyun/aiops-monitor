package main

import "testing"

func TestContainerActionValidation(t *testing.T) {
	if _, code := moduleContainerAction(map[string]string{}); code == 0 {
		t.Fatal("missing args should fail")
	}
	// Without docker/podman, should fail with clear message
	out, code := moduleContainerAction(map[string]string{"action": "start", "id": "abc"})
	if code == 0 {
		t.Fatalf("expected fail without runtime, got %s", out)
	}
}

func TestContainerLogsValidation(t *testing.T) {
	if _, code := moduleContainerLogs(map[string]string{}); code == 0 {
		t.Fatal("missing id should fail")
	}
}
