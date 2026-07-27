package main

import (
	"strconv"
	"strings"
	"testing"
)

// TestFimFullScopeSummarizesOrdinaryChurn is the core guarantee of full-scope
// FIM: every changed file anywhere on disk is recorded, but only the
// security-relevant paths become individual findings — ordinary application
// churn collapses into one summary row instead of flooding the list.
func TestFimFullScopeSummarizesOrdinaryChurn(t *testing.T) {
	changes := []HostFileChange{
		{Path: "/etc/shadow", Change: "modified", Reason: "content", OldSHA: "aaa", NewSHA: "bbb"},
	}
	for i := 0; i < 200; i++ {
		changes = append(changes, HostFileChange{
			Path: "/var/lib/app/cache/" + strconv.Itoa(i) + ".dat", Change: "modified", Reason: "size",
			OldSize: 10, NewSize: 20,
		})
	}
	findings := fimFindingsFromChanges(changes)
	if len(findings) != 2 {
		t.Fatalf("want 1 individual finding + 1 summary, got %d", len(findings))
	}
	if findings[0].Level != "crit" || !strings.Contains(findings[0].Detail, "/etc/shadow") {
		t.Errorf("shadow change should be its own crit finding, got %+v", findings[0])
	}
	sum := findings[1]
	if sum.ID != "fim.summary" || sum.Level != "info" {
		t.Fatalf("want an info-level summary row, got %+v", sum)
	}
	if !strings.Contains(sum.Detail, "200") {
		t.Errorf("summary should account for all 200 churn entries, got %q", sum.Detail)
	}
	// The whole point of metadata-only monitoring: no file contents leak into
	// the summary of non-whitelisted paths.
	if strings.Contains(sum.Detail, "cache/1.dat") {
		t.Errorf("summary must not enumerate individual paths: %q", sum.Detail)
	}
}

// TestConvertAgentFileChangesDropsDiffWhenAuditDisabled verifies the content
// audit switch is enforced server-side too, not just on the agent.
func TestConvertAgentFileChangesDropsDiffWhenAuditDisabled(t *testing.T) {
	in := []hsAgentFileChange{{
		Path: `C:\Windows\System32\drivers\etc\hosts`, Change: "modified", Reason: "content",
		OldSHA: "AAA", NewSHA: "BBB", Diff: "- old line\n+ new line",
	}}

	with := convertAgentFileChanges(in, true)
	if len(with) != 1 || with[0].Diff == "" {
		t.Fatalf("content audit on: diff should be kept, got %+v", with)
	}
	if with[0].Path != "C:/Windows/System32/drivers/etc/hosts" {
		t.Errorf("path not normalized for cross-OS display: %q", with[0].Path)
	}
	if with[0].OldSHA != "aaa" || with[0].NewSHA != "bbb" {
		t.Errorf("hashes should be lower-cased, got %q/%q", with[0].OldSHA, with[0].NewSHA)
	}

	without := convertAgentFileChanges(in, false)
	if len(without) != 1 || without[0].Diff != "" {
		t.Fatalf("content audit off: diff must be dropped, got %+v", without)
	}
	// Metadata still flows through — the change itself is always recorded.
	if without[0].Change != "modified" || without[0].Reason != "content" {
		t.Errorf("metadata lost when diff dropped: %+v", without[0])
	}
}

// TestConvertAgentFileChangesRejectsGarbage guards against a compromised or
// buggy agent injecting rows the UI would render as real changes.
func TestConvertAgentFileChangesRejectsGarbage(t *testing.T) {
	out := convertAgentFileChanges([]hsAgentFileChange{
		{Path: "/etc/hosts", Change: "modified"},
		{Path: "/etc/hosts", Change: "removed"}, // duplicate path
		{Path: "   ", Change: "added"},          // empty path
		{Path: "/tmp/x", Change: "chmod"},       // unknown change kind
	}, true)
	if len(out) != 1 || out[0].Path != "/etc/hosts" || out[0].Change != "modified" {
		t.Fatalf("expected only the first valid change, got %+v", out)
	}
}

// TestApplyFIMScanArgsSanitizesOperatorInput covers the arg-encoding boundary:
// the agent splits these on commas, so an entry containing a separator would
// silently expand into bogus roots/excludes.
func TestApplyFIMScanArgsSanitizesOperatorInput(t *testing.T) {
	args := map[string]string{}
	applyFIMScanArgs(args, HostSecurityConfig{
		FIMScope:        "full",
		FIMRoots:        []string{"/etc", "/etc", "  ", "/opt,/evil"},
		FIMContentPaths: []string{"/etc/ssh/sshd_config", "bad\nvalue"},
	}.withDefaults())

	if args["fim_scope"] != "full" {
		t.Errorf("fim_scope = %q, want full", args["fim_scope"])
	}
	if got := args["fim_roots"]; got != "/etc" {
		t.Errorf("fim_roots = %q, want the deduped/sanitized single entry", got)
	}
	if got := args["fim_content_paths"]; got != "/etc/ssh/sshd_config" {
		t.Errorf("fim_content_paths = %q, newline entry should be dropped", got)
	}
	for _, k := range []string{"fim_max_files", "fim_max_changes", "fim_budget_sec"} {
		n, err := strconv.Atoi(args[k])
		if err != nil || n <= 0 {
			t.Errorf("%s = %q, want a positive limit so the agent walk is bounded", k, args[k])
		}
	}
}

// TestConvertAgentFIMStatsNormalizesMode keeps the UI's coverage line honest —
// an unknown mode from an old agent must not be shown as a verified full scan.
func TestConvertAgentFIMStatsNormalizesMode(t *testing.T) {
	if convertAgentFIMStats(nil) != nil {
		t.Fatal("nil stats should stay nil so the UI hides the coverage line")
	}
	st := convertAgentFIMStats(&hsAgentFIMStats{
		Mode: "SENSITIVE", Files: 10, Dirs: 2, Reported: 3, LimitHit: true,
		Error: strings.Repeat("e", 500),
	})
	if st.Mode != "sensitive" {
		t.Errorf("mode = %q, want sensitive", st.Mode)
	}
	if len(st.Error) > 210 { // 200 chars plus the truncation marker
		t.Errorf("agent error should be truncated, got %d chars", len(st.Error))
	}
	if !st.LimitHit {
		t.Error("limit_hit must survive so partial coverage is visible")
	}
}
