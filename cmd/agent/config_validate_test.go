package main

import (
	"testing"
)

func validAgentConfig() config {
	c := defaultConfig()
	c.Server = "https://monitor.example.com/"
	return c
}

func TestNormalizeAndValidateConfig(t *testing.T) {
	c := validAgentConfig()
	if err := normalizeAndValidateConfig(&c); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if c.Server != "https://monitor.example.com" {
		t.Fatalf("server not normalized: %q", c.Server)
	}

	bad := validAgentConfig()
	bad.Server = "ftp://monitor.example.com"
	if err := normalizeAndValidateConfig(&bad); err == nil {
		t.Fatal("non-http server must be rejected")
	}

	bad = validAgentConfig()
	bad.ReportInterval = 1
	if err := normalizeAndValidateConfig(&bad); err == nil {
		t.Fatal("unsafe report interval must be rejected")
	}
}

func TestNormalizeContentAuditLeastPrivilege(t *testing.T) {
	c := validAgentConfig()
	c.SNI = &SNIConfig{ContentAudit: true}
	if err := normalizeAndValidateConfig(&c); err != nil {
		t.Fatalf("content audit config rejected: %v", err)
	}
	if !c.SNI.Enabled {
		t.Fatal("content audit must imply SNI collector enabled")
	}
	if len(c.SNI.ContentAuditPorts) != 3 {
		t.Fatalf("empty all-port capture must become safe defaults: %v", c.SNI.ContentAuditPorts)
	}
	if c.SNI.ContentAuditMaxBody != 4096 {
		t.Fatalf("default max body = %d, want 4096", c.SNI.ContentAuditMaxBody)
	}
	if c.SNI.ContentAuditBodyMode != contentBodyRedacted {
		t.Fatalf("default body mode = %q, want redacted", c.SNI.ContentAuditBodyMode)
	}
	if c.SNI.ContentAuditMaxEventsPerMin != 2000 {
		t.Fatalf("default event limit = %d, want 2000", c.SNI.ContentAuditMaxEventsPerMin)
	}
	if c.SNI.CaptureBackend != "auto" || len(c.SNI.TLSMetadataPorts) != 3 {
		t.Fatalf("capture defaults not normalized: %+v", c.SNI)
	}
}
