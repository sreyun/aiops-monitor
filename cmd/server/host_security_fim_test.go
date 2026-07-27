package main

import (
	"strings"
	"testing"
)

func TestDiffHostFileInventoryBaseline(t *testing.T) {
	cur := []HostFileHash{
		{Path: "/etc/hosts", SHA256: "aaa", Mtime: 100},
		{Path: "/etc/shadow", SHA256: "bbb", Mtime: 100},
	}
	changes, baseline := diffHostFileInventory(nil, cur, nil)
	if !baseline {
		t.Fatal("expected baseline established")
	}
	if len(changes) != 0 {
		t.Fatalf("baseline should not emit changes: %+v", changes)
	}
}

func TestDiffHostFileInventoryAddedRemovedModified(t *testing.T) {
	prev := []HostFileHash{
		{Path: "/etc/hosts", SHA256: "aaa", Mtime: 1},
		{Path: "/etc/shadow", SHA256: "bbb", Mtime: 1},
		{Path: "/tmp/gone", SHA256: "ccc", Mtime: 1},
	}
	cur := []HostFileHash{
		{Path: "/etc/hosts", SHA256: "aaa2", Mtime: 2},
		{Path: "/etc/shadow", SHA256: "bbb", Mtime: 1},
		{Path: "/etc/newfile", SHA256: "ddd", Mtime: 2},
	}
	diffs := []hsAgentTextDiff{{
		Path: "/etc/hosts", OldSHA: "aaa", NewSHA: "aaa2",
		Diff: "--- a/hosts\n+++ b/hosts\n+evil\n",
	}}
	changes, baseline := diffHostFileInventory(prev, cur, diffs)
	if baseline {
		t.Fatal("should not be baseline")
	}
	byPath := map[string]HostFileChange{}
	for _, c := range changes {
		byPath[c.Path] = c
	}
	if byPath["/etc/hosts"].Change != "modified" || byPath["/etc/hosts"].Diff == "" {
		t.Fatalf("hosts: %+v", byPath["/etc/hosts"])
	}
	if byPath["/tmp/gone"].Change != "removed" {
		t.Fatalf("gone: %+v", byPath["/tmp/gone"])
	}
	if byPath["/etc/newfile"].Change != "added" {
		t.Fatalf("new: %+v", byPath["/etc/newfile"])
	}
	if _, ok := byPath["/etc/shadow"]; ok {
		t.Fatal("unchanged shadow should not appear")
	}
}

func TestDiffHostFileInventoryOldAgentNoInventory(t *testing.T) {
	prev := []HostFileHash{{Path: "/etc/hosts", SHA256: "aaa"}}
	changes, baseline := diffHostFileInventory(prev, nil, nil)
	if baseline {
		t.Fatal("empty cur is not a new baseline")
	}
	if len(changes) != 0 {
		t.Fatalf("old agent without inventory must not spam removals: %+v", changes)
	}
}

func TestFimFindingsSeverity(t *testing.T) {
	fs := fimFindingsFromChanges([]HostFileChange{
		{Path: "/etc/shadow", Change: "modified", OldSHA: "a", NewSHA: "b"},
		{Path: "/etc/hosts", Change: "modified", OldSHA: "a", NewSHA: "b"},
		{Path: "/opt/bin/foo", Change: "added", NewSHA: "c"},
	})
	if len(fs) != 3 {
		t.Fatalf("got %d", len(fs))
	}
	for _, f := range fs {
		if f.Category != "fim" {
			t.Fatalf("category=%s", f.Category)
		}
	}
	if fs[0].Level != "crit" { // shadow modified
		t.Fatalf("shadow level=%s", fs[0].Level)
	}
}

func TestFIMConfigDefaultsOn(t *testing.T) {
	c := HostSecurityConfig{}.withDefaults()
	if !c.fimEnabled() || !c.fimContentDiffEnabled() {
		t.Fatal("FIM should default on")
	}
	c.DisableFIM = true
	if c.fimEnabled() {
		t.Fatal("disable_fim should win")
	}
	c.DisableFIM = false
	c.DisableFIMContentDiff = true
	if c.fimContentDiffEnabled() {
		t.Fatal("disable_fim_content_diff should win")
	}
}

func TestScoreHostFindingsIncludesFIM(t *testing.T) {
	_, _, sum := scoreHostFindings([]HostFinding{
		{Level: "high", Category: "fim"},
		{Level: "medium", Category: "port"},
	})
	if sum["fim"] != 1 {
		t.Fatalf("summary fim=%v", sum)
	}
}

func TestFimNormalizePathCrossOS(t *testing.T) {
	if got := fimNormalizePath(`/etc/hosts`); got != "/etc/hosts" {
		t.Fatalf("unix path corrupted: %q", got)
	}
	if got := fimNormalizePath(`C:\Windows\System32\drivers\etc\hosts`); got != "C:/Windows/System32/drivers/etc/hosts" {
		t.Fatalf("windows path: %q", got)
	}
	prev := []HostFileHash{{Path: fimNormalizePath(`/etc/hosts`), SHA256: "aaa"}}
	cur := []HostFileHash{{Path: fimNormalizePath(`/etc/hosts`), SHA256: "bbb"}}
	changes, _ := diffHostFileInventory(prev, cur, nil)
	if len(changes) != 1 || changes[0].Change != "modified" {
		t.Fatalf("path normalize broke matching: %+v", changes)
	}
}

func TestSanitizeFileChangesCapsDiff(t *testing.T) {
	big := strings.Repeat("x", fimMaxStoredDiff+100)
	out := sanitizeFileChanges([]HostFileChange{{
		Path: "/etc/hosts", Change: "modified", Diff: big,
	}})
	if len(out) != 1 || !out[0].Truncated || len(out[0].Diff) != fimMaxStoredDiff {
		t.Fatalf("%+v len=%d", out, len(out[0].Diff))
	}
}
