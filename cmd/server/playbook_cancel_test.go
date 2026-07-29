package main

import (
	"testing"
	"time"
)

func TestCancelUnfinishedHosts(t *testing.T) {
	pm := newPlaybookManager(nil)
	pm.executions = []PlaybookExecution{{
		ID: 7, Status: "running", PlaybookID: "pb1",
		HostResults: map[string]HostExecResult{
			"a": {Hostname: "a", Status: "success", Steps: []StepResult{{Name: "s1", Status: "success"}}},
			"b": {Hostname: "b", Status: "running", Steps: []StepResult{{Name: "s1", Status: "running"}}},
			"c": {Hostname: "c", Status: "pending"},
		},
	}}
	pm.CancelUnfinishedHosts(7)
	hr := pm.executions[0].HostResults
	if hr["a"].Status != "success" {
		t.Fatalf("success host mutated: %s", hr["a"].Status)
	}
	if hr["b"].Status != "cancelled" || hr["b"].Reason != "cancelled" {
		t.Fatalf("running host not cancelled: %+v", hr["b"])
	}
	if hr["b"].Steps[0].Status != "cancelled" {
		t.Fatalf("running step not cancelled: %+v", hr["b"].Steps[0])
	}
	if hr["c"].Status != "cancelled" {
		t.Fatalf("pending host not cancelled: %+v", hr["c"])
	}
}

func TestAbortSessionsByExecID(t *testing.T) {
	m := newTermManager()
	s1 := m.createExecWithExecID("h1", "n1", "echo 1", 42)
	s2 := m.createExecWithExecID("h2", "n2", "echo 2", 42)
	s3 := m.createExecWithExecID("h3", "n3", "echo 3", 99)
	n := m.abortSessionsByExecID(42)
	if n != 2 {
		t.Fatalf("aborted=%d want 2", n)
	}
	if m.get(s1.id) != nil || m.get(s2.id) != nil {
		t.Fatal("sessions 42 should be removed")
	}
	if m.get(s3.id) == nil {
		t.Fatal("unrelated session removed")
	}
	if m.sessionAlive(s3.id) != true {
		t.Fatal("unrelated should stay alive")
	}
	s3.close()
	if m.sessionAlive(s3.id) {
		t.Fatal("closed session still alive")
	}
}

func TestRegisterPlaybookRunCancel(t *testing.T) {
	s := &Server{term: newTermManager(), playbooks: newPlaybookManager(nil)}
	ctx := s.registerPlaybookRun(11)
	s.term.createExecWithExecID("h1", "n1", "sleep 1", 11)
	signalled, n := s.signalPlaybookCancel(11)
	if !signalled || n != 1 {
		t.Fatalf("signalled=%v sessions=%d", signalled, n)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("context not cancelled")
	}
	s.unregisterPlaybookRun(11)
}
