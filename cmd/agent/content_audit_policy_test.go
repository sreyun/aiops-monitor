package main

import (
	"strings"
	"testing"

	"aiops-monitor/shared"
)

func TestContentAuditPolicyRedactsBeforeBuffer(t *testing.T) {
	cfg := SNIConfig{
		ContentAuditBodyMode:     "redacted",
		ContentAuditIncludeHosts: []string{"*.example.com"},
		ContentAuditExcludePaths: []string{"/health*"},
		ContentAuditRedactKeys:   []string{"tenant_secret"},
	}
	ev := shared.ContentAuditEvent{
		Host:     "llm.example.com:8000",
		Path:     "/v1/chat/completions?api_key=visible",
		Body:     `{"model":"x","api_key":"sk-abcdefghijklmnopqrstuvwxyz","email":"user@example.com","tenant_secret":"abc"}`,
		RespBody: `{"authorization":"Bearer abcdefghijklmnop","content":"ok"}`,
	}
	if !applyContentAuditPolicy(cfg, &ev) {
		t.Fatal("allowed event was filtered")
	}
	for _, secret := range []string{"sk-abcdefghijklmnopqrstuvwxyz", "user@example.com", "abcdefghijklmnop", `"abc"`} {
		if strings.Contains(ev.Body+"\n"+ev.RespBody, secret) {
			t.Fatalf("secret %q survived redaction: %+v", secret, ev)
		}
	}
	if strings.Contains(ev.Path, "visible") || ev.RedactionCount < 4 {
		t.Fatalf("URL/body redaction incomplete: %+v", ev)
	}
	if ev.ReqBytes == 0 || ev.ReqSHA256 == "" || ev.BodyMode != contentBodyRedacted {
		t.Fatalf("audit metadata missing: %+v", ev)
	}

	blocked := shared.ContentAuditEvent{Host: "llm.example.com", Path: "/healthz"}
	if applyContentAuditPolicy(cfg, &blocked) {
		t.Fatal("excluded path must be filtered")
	}
	outside := shared.ContentAuditEvent{Host: "unrelated.test", Path: "/v1/chat"}
	if applyContentAuditPolicy(cfg, &outside) {
		t.Fatal("host outside allowlist must be filtered")
	}
}

func TestContentAuditPolicyMetadataSuppressesBodies(t *testing.T) {
	ev := shared.ContentAuditEvent{
		Host: "localhost", Path: "/api/chat",
		Body:     `{"token":"secret-value","prompt":"hello"}`,
		RespBody: `{"message":"world"}`,
	}
	if !applyContentAuditPolicy(SNIConfig{ContentAuditBodyMode: "metadata"}, &ev) {
		t.Fatal("metadata event filtered")
	}
	if ev.Body != "" || ev.RespBody != "" {
		t.Fatalf("metadata mode retained body: %+v", ev)
	}
	if ev.ReqBytes == 0 || ev.RespBytes == 0 || ev.ReqSHA256 == "" || ev.RespSHA256 == "" {
		t.Fatalf("metadata mode lost correlation fields: %+v", ev)
	}
	if ev.RedactionCount == 0 {
		t.Fatal("metadata mode should report detected credential suppression")
	}
}

func TestContentAuditRateLimit(t *testing.T) {
	sc := newSNICollector(SNIConfig{
		ContentAuditBodyMode:        "metadata",
		ContentAuditMaxEventsPerMin: 1,
	}, "h", "fp")
	sc.addContent(shared.ContentAuditEvent{Host: "localhost", Path: "/api/chat", Body: "one"})
	sc.addContent(shared.ContentAuditEvent{Host: "localhost", Path: "/api/chat", Body: "two"})
	if len(sc.content) != 1 || sc.contentRateDropped != 1 {
		t.Fatalf("rate limit not enforced: events=%d dropped=%d", len(sc.content), sc.contentRateDropped)
	}
}

func TestObserveSNICreatesMetadataOnlyAuditEvent(t *testing.T) {
	sc := newSNICollector(SNIConfig{
		CaptureBackend:           "tshark",
		ContentAudit:             true,
		ContentAuditBodyMode:     "full",
		ContentAuditIncludeHosts: []string{"*.openai.com"},
	}, "h", "fp")

	sc.observeSNI(l4Info{
		proto: 6, srcIP: "10.0.0.8", dstIP: "203.0.113.10",
		srcPort: 49152, dstPort: 443,
	}, "api.openai.com")

	if len(sc.content) != 1 {
		t.Fatalf("TLS SNI observation produced %d audit events, want 1", len(sc.content))
	}
	ev := sc.content[0]
	if ev.Protocol != "tls" || ev.BodyMode != contentBodyMetadata ||
		ev.CaptureBackend != "tshark" || ev.Host != "api.openai.com" {
		t.Fatalf("unexpected TLS audit metadata: %+v", ev)
	}
	if ev.Body != "" || ev.RespBody != "" {
		t.Fatalf("TLS metadata event must never contain bodies: %+v", ev)
	}
	if got := sc.seen["203.0.113.10|api.openai.com"]; got.Source != "sni" {
		t.Fatalf("SNI DNS mapping missing: %+v", sc.seen)
	}

	sc.observeSNI(l4Info{
		proto: 6, srcIP: "10.0.0.8", dstIP: "203.0.113.11", dstPort: 443,
	}, "unrelated.example")
	if len(sc.content) != 1 {
		t.Fatalf("host allowlist did not suppress unrelated TLS event: %+v", sc.content)
	}
}
