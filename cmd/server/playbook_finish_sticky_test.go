package main

import "testing"

func TestFinishExecutionCancelWins(t *testing.T) {
	pm := newPlaybookManager(nil)
	pb := Playbook{ID: "pb1", Name: "t", Steps: []PlaybookStep{{Name: "s", Command: "true", Target: "host:h1"}}}
	hosts := []*Host{{ID: "h1", Hostname: "h1"}}
	exec := pm.StartExecution(pb, "tester", hosts)

	pm.FinishExecution(exec.ID, "cancelled")
	pm.UpdateHostResult(exec.ID, "h1", HostExecResult{Hostname: "h1", Status: "success", Output: "late"})
	pm.FinishExecution(exec.ID, "completed")

	got, ok := pm.GetExecution(exec.ID)
	if !ok {
		t.Fatal("execution missing")
	}
	if got.Status != "cancelled" {
		t.Fatalf("status=%q want cancelled (sticky)", got.Status)
	}
	hr := got.HostResults["h1"]
	if hr.Status == "success" {
		t.Fatalf("host result overwritten to success after cancel")
	}
}

func TestUpdateHostResultIgnoresAfterHostCancelled(t *testing.T) {
	pm := newPlaybookManager(nil)
	pb := Playbook{ID: "pb1", Name: "t", Steps: []PlaybookStep{{Name: "s", Command: "true", Target: "host:h1"}}}
	hosts := []*Host{{ID: "h1", Hostname: "h1"}}
	exec := pm.StartExecution(pb, "tester", hosts)
	pm.UpdateHostResult(exec.ID, "h1", HostExecResult{Hostname: "h1", Status: "cancelled", Reason: "cancelled"})
	pm.UpdateHostResult(exec.ID, "h1", HostExecResult{Hostname: "h1", Status: "success", Output: "late"})

	got, _ := pm.GetExecution(exec.ID)
	if got.HostResults["h1"].Status != "cancelled" {
		t.Fatalf("host status=%q want cancelled", got.HostResults["h1"].Status)
	}
}
