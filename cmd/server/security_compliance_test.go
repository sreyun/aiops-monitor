package main

import "testing"

func hasControl(refs []ComplianceRef, fw, ctrl string) bool {
	for _, r := range refs {
		if r.Framework == fw && r.Control == ctrl {
			return true
		}
	}
	return false
}

// TestComplianceLongestPrefixWins guards the lookup rule: a specific finding ID
// must map to its own control, not to a broader family rule that also matches.
func TestComplianceLongestPrefixWins(t *testing.T) {
	refs := complianceRefsFor("account", "acct_pass_min_len")
	if !hasControl(refs, "PCI-DSS", "8.3.6") {
		t.Fatalf("acct_pass_min_len should map to PCI-DSS 8.3.6, got %+v", refs)
	}
	// The generic "acct_pass_max_days" rule must not leak into it.
	if hasControl(refs, "PCI-DSS", "8.3.9") {
		t.Errorf("acct_pass_min_len wrongly inherited the expiration control: %+v", refs)
	}
}

// TestComplianceUnknownFindingStaysUnmapped keeps the mapping honest: claiming a
// control for an unrecognized finding would fabricate audit evidence.
func TestComplianceUnknownFindingStaysUnmapped(t *testing.T) {
	if refs := complianceRefsFor("misc", "totally_unknown_finding"); len(refs) != 0 {
		t.Fatalf("unknown finding should stay unmapped, got %+v", refs)
	}
}

// TestSummarizeComplianceCountsDistinctOpenControls verifies the posture number
// shown to auditors: duplicates collapse, and closed/info findings don't count.
func TestSummarizeComplianceCountsDistinctOpenControls(t *testing.T) {
	findings := annotateCompliance([]HostFinding{
		{ID: "acct_empty_password", Category: "account", Level: "high"},
		{ID: "acct_empty_password", Category: "account", Level: "high"}, // same control twice
		{ID: "win_smb1", Category: "windows", Level: "medium"},
		{ID: "auditd_off", Category: "audit", Level: "medium", Status: "resolved"},
		{ID: "sysctl.ip_forward", Category: "kernel", Level: "info"},
	})
	sum := summarizeCompliance(findings)
	if sum["PCI-DSS"] != 2 { // 8.3.1 from the account finding + 2.2.4 from SMBv1
		t.Errorf("PCI-DSS failing controls = %d, want 2 (%v)", sum["PCI-DSS"], sum)
	}
	// auditd_off is resolved, so its PCI-DSS 10.2.1 must not be counted.
	for _, f := range findings {
		if f.ID == "auditd_off" && len(f.Compliance) == 0 {
			t.Error("resolved findings should still carry their mapping for display")
		}
	}
	if got := complianceFrameworks(sum); len(got) < 2 || got[0] > got[1] {
		t.Errorf("frameworks should be sorted for stable UI order, got %v", got)
	}
}

// TestWebOWASPClassification checks the buckets customers actually read the
// report by, including the CVE-template fallback.
func TestWebOWASPClassification(t *testing.T) {
	cases := []struct {
		finding WebFinding
		want    string
	}{
		{WebFinding{TemplateID: "builtin/plaintext-http", Tags: []string{"tls", "transport"}}, owaspA02},
		{WebFinding{TemplateID: "builtin/cors-reflect-origin", Tags: []string{"cors"}}, owaspA01},
		{WebFinding{TemplateID: "generic-sqli", Tags: []string{"sqli"}}, owaspA03},
		{WebFinding{TemplateID: "CVE-2021-44228", Name: "Log4j RCE"}, owaspA03},
		{WebFinding{TemplateID: "CVE-2019-0001", Name: "Some product flaw"}, owaspA06},
		{WebFinding{TemplateID: "builtin/version-disclosure", Tags: []string{"disclosure"}}, owaspA05},
		{WebFinding{TemplateID: "unknown-template"}, owaspA05},
	}
	for _, c := range cases {
		if got := webOWASPCategory(c.finding); got != c.want {
			t.Errorf("%s: OWASP = %q, want %q", c.finding.TemplateID, got, c.want)
		}
	}
}

// TestSummarizeWebOWASPSkipsClosedAndInfo mirrors the host-side rule so the two
// dashboards report posture consistently.
func TestSummarizeWebOWASPSkipsClosedAndInfo(t *testing.T) {
	findings := annotateWebFindings([]WebFinding{
		{TemplateID: "builtin/plaintext-http", Severity: "high", Tags: []string{"tls"}},
		{TemplateID: "builtin/missing-csp", Severity: "info", Tags: []string{"headers"}},
		{TemplateID: "generic-sqli", Severity: "critical", Tags: []string{"sqli"}, Status: "false_positive"},
	})
	sum := summarizeWebOWASP(findings)
	if len(sum) != 1 || sum[owaspA02] != 1 {
		t.Fatalf("OWASP summary = %v, want only one A02 entry", sum)
	}
	if comp := summarizeWebCompliance(findings); comp["OWASP"] != 1 {
		t.Errorf("web compliance summary = %v, want a single OWASP control", comp)
	}
}

// TestDedupeWebFindingsMergesEnginesAndSortsBySeverity covers the overlap
// between the built-in engine and Nuclei reporting the same issue.
func TestDedupeWebFindingsMergesEnginesAndSortsBySeverity(t *testing.T) {
	out := dedupeWebFindings([]WebFinding{
		{TemplateID: "t1", URL: "https://x/", Severity: "low"},
		{TemplateID: "t2", URL: "https://x/", Severity: "critical"},
		{TemplateID: "t1", URL: "https://x/", Severity: "low"}, // duplicate
	})
	if len(out) != 2 {
		t.Fatalf("dedupe kept %d findings, want 2", len(out))
	}
	if out[0].Severity != "critical" {
		t.Errorf("findings must be severity-sorted, got %q first", out[0].Severity)
	}
}
