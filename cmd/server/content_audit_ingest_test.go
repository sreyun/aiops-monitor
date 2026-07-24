package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aiops-monitor/shared"
)

func TestNormalizeContentAuditEventZeroTrust(t *testing.T) {
	ev := shared.ContentAuditEvent{
		SrcIP: "not-an-ip", DstIP: "2001:db8::1",
		Method:         strings.Repeat("x", 100),
		Host:           strings.Repeat("h", 700),
		Path:           strings.Repeat("p", 5000),
		Body:           strings.Repeat("a", 70<<10),
		RespBody:       strings.Repeat("b", (1<<20)+100),
		CaptureBackend: "shell", BodyMode: "metadata",
		ReqSHA256: "bad", RespSHA256: strings.Repeat("f", 64),
		RedactionCount:  -1,
		RedactionLabels: []string{"LLM_API_KEY", "bad label!", "llm_api_key"},
		Ts:              time.Now().Add(24 * time.Hour).Unix(),
	}
	normalizeContentAuditEvent(&ev, 0)
	if ev.SrcIP != "" || ev.DstIP != "2001:db8::1" {
		t.Fatalf("IP normalization failed: %+v", ev)
	}
	if len(ev.Method) != 16 || len(ev.Host) != 512 || len(ev.Path) != 4096 {
		t.Fatalf("bounded fields not enforced: method=%d host=%d path=%d", len(ev.Method), len(ev.Host), len(ev.Path))
	}
	if ev.Body != "" || ev.RespBody != "" {
		t.Fatal("metadata mode must fail closed and strip bodies")
	}
	if ev.CaptureBackend != "legacy" || ev.BodyMode != "metadata" {
		t.Fatalf("provenance normalization failed: %+v", ev)
	}
	if ev.ReqSHA256 == "" || ev.RespSHA256 == "" || ev.RedactionCount != 0 {
		t.Fatalf("hash/redaction validation failed: %+v", ev)
	}
	if len(ev.RedactionLabels) != 1 || ev.RedactionLabels[0] != "llm_api_key" {
		t.Fatalf("redaction labels not normalized: %#v", ev.RedactionLabels)
	}
}

func TestNormalizeTLSAuditEventAlwaysFailsClosed(t *testing.T) {
	ev := shared.ContentAuditEvent{
		Protocol:       "TLS",
		CaptureBackend: "tshark",
		BodyMode:       "full",
		Host:           "api.openai.com",
		Body:           "must-not-survive",
		RespBody:       "must-not-survive-either",
	}
	normalizeContentAuditEvent(&ev, time.Now().Unix())
	if ev.Protocol != "tls" || ev.BodyMode != "metadata" {
		t.Fatalf("TLS provenance was not normalized: %+v", ev)
	}
	if ev.Body != "" || ev.RespBody != "" || ev.ReqBytes == 0 || ev.RespBytes == 0 {
		t.Fatalf("TLS event did not fail closed while retaining dimensions: %+v", ev)
	}
}

func TestSensitiveSeverityUnderstandsAgentLabels(t *testing.T) {
	if got := sensitiveSeverity([]string{"端侧脱敏:llm_api_key"}); got != "critical" {
		t.Fatalf("agent-side key redaction severity = %s", got)
	}
	if got := sensitiveSeverity([]string{"端侧脱敏:email"}); got != "warning" {
		t.Fatalf("agent-side PII redaction severity = %s", got)
	}
}

func TestGatewayContentAuditRequiresDedicatedBearerToken(t *testing.T) {
	srv, _ := newTestServer(t)
	token := strings.Repeat("t", 32)
	t.Setenv("AIOPS_CONTENT_AUDIT_INGEST_TOKEN", token)
	payload, _ := json.Marshal(shared.ContentAuditReport{
		HostID: "llm-gateway-prod",
		Events: []shared.ContentAuditEvent{{
			Host: "api.openai.com", Path: "/v1/responses",
			LLMProvider: "openai", LLMModel: "gpt-5", BodyMode: "metadata",
		}},
	})
	call := func(auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/content-audit", bytes.NewReader(payload))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rr := httptest.NewRecorder()
		srv.handleGatewayContentAudit(rr, req)
		return rr
	}
	if rr := call("Bearer wrong"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := call("Bearer " + token); rr.Code != http.StatusAccepted {
		t.Fatalf("valid token status=%d body=%s", rr.Code, rr.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/content-audit",
		bytes.NewReader(append(payload, []byte(`{"extra":true}`)...)))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	srv.handleGatewayContentAudit(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestStructuredGatewayFieldsWinOverBodyDerivation(t *testing.T) {
	ev := map[string]any{
		"host": "gateway.internal", "path": "/v1/responses",
		"llm_provider": "azure-openai", "llm_model": "deployment-prod",
		"llm_operation": "responses", "llm_input_tokens": 123,
	}
	annotateLLMAuditEvent(ev)
	if ev["is_llm"] != true || ev["llm_provider"] != "azure-openai" || ev["llm_input_tokens"] != 123 {
		t.Fatalf("structured LLM metadata was overwritten: %#v", ev)
	}
}

func TestContentAuditQueryDisablesCaching(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content-audit?host=h1", nil)
	rr := httptest.NewRecorder()
	srv.handleContentAudit(rr, req)
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}
