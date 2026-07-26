package main

import (
	"testing"

	"aiops-monitor/shared"
)

func TestEnforceContentAuditPolicy(t *testing.T) {
	ev := &shared.ContentAuditEvent{
		PolicyDecision: "deny",
		Body:           "secret-prompt",
		RespBody:       "model-out",
		BodyMode:       "full",
	}
	enforceContentAuditPolicy(ev)
	if ev.Body != "[blocked by policy]" || ev.RespBody != "[blocked by policy]" {
		t.Fatalf("body not redacted: %#v %#v", ev.Body, ev.RespBody)
	}
	if !isContentPolicyBlocked(ev.PolicyDecision) {
		t.Fatal("expected blocked")
	}
}

func TestOpenAPIEndpointURLs(t *testing.T) {
	eps := []APIEndpoint{
		{URL: "https://a.example/v1/users"},
		{URL: "https://a.example/v1/users?x=1"},
		{URL: "https://a.example/v1/orders"},
		{URL: "not-a-url"},
	}
	out := openAPIEndpointURLs(eps, 80)
	if len(out) != 2 {
		t.Fatalf("got %v", out)
	}
}
