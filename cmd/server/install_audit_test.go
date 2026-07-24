package main

import (
	"strings"
	"testing"
)

func TestSanitizeAuditInstallOptions(t *testing.T) {
	got := sanitizeAuditInstallOptions(map[string]string{
		"sni_enabled":                      "false",
		"sni_interface":                    `eth0";reboot`,
		"content_audit":                    "true",
		"content_audit_ports":              "11434, 8000, 0, 65536, nope, 11434",
		"content_audit_max_body":           "999999",
		"capture_backend":                  "tshark",
		"content_audit_body_mode":          "metadata",
		"content_audit_include_hosts":      "*.Example.com,evil$host,*.Example.com",
		"content_audit_exclude_paths":      "/health*,/metrics*",
		"content_audit_max_events_per_min": "999999",
	})
	if !got.SNIEnabled || !got.ContentAudit {
		t.Fatal("content audit must imply collector enabled")
	}
	if got.SNIInterface != "eth0reboot" {
		t.Fatalf("interface not sanitized: %q", got.SNIInterface)
	}
	if got.ContentAuditPorts != "[11434,8000]" {
		t.Fatalf("ports = %q", got.ContentAuditPorts)
	}
	if got.ContentAuditMaxBody != 65536 {
		t.Fatalf("max body = %d", got.ContentAuditMaxBody)
	}
	if got.CaptureBackend != "tshark" || got.ContentAuditBodyMode != "metadata" {
		t.Fatalf("capture policy not sanitized: %+v", got)
	}
	if got.ContentAuditIncludeHosts != `["*.example.com","evilhost"]` {
		t.Fatalf("host patterns = %s", got.ContentAuditIncludeHosts)
	}
	if got.ContentAuditMaxEventsPerMin != 100000 {
		t.Fatalf("event limit = %d", got.ContentAuditMaxEventsPerMin)
	}
}

func TestRenderInstallAuditConfig(t *testing.T) {
	opts := installAuditOptions{
		SNIEnabled: true, SNIInterface: "eth0", ContentAudit: true,
		CaptureBackend: "tshark", ContentAuditPorts: "[11434,8000]", ContentAuditMaxBody: 8192,
		ContentAuditBodyMode: "redacted", ContentAuditIncludeHosts: `["*.example.com"]`,
		ContentAuditExcludePaths: `["/health*"]`, ContentAuditMaxEventsPerMin: 1200,
	}
	for name, tmpl := range map[string]string{"sh": installShTemplate, "ps1": installPs1Template} {
		out := renderScriptWithAudit(tmpl, "https://monitor.example", "tok", "prod", "", "[]", opts)
		for _, want := range []string{
			"enabled: true", "content_audit: true",
			"content_audit_ports: [11434,8000]", "content_audit_max_body: 8192",
			"capture_backend:", "tshark", "content_audit_body_mode:",
			"content_audit_include_hosts: [\"*.example.com\"]",
			"content_audit_max_events_per_min: 1200",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("%s installer missing %q", name, want)
			}
		}
		if strings.Contains(out, "__CONTENT_AUDIT") || strings.Contains(out, "__SNI_") || strings.Contains(out, "__CAPTURE_") {
			t.Errorf("%s installer has unresolved placeholders", name)
		}
	}
}
