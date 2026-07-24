package main

import (
	"strings"
	"testing"

	"aiops-monitor/shared"
)

func TestClassifyLLMExchangeOpenAI(t *testing.T) {
	req := `{"model":"gpt-4.1","stream":true,"messages":[{"role":"user","content":"你好"}],"tools":[{"type":"function"}]}`
	resp := "data: " + `{"choices":[{"delta":{"content":"您好"}}]}` + "\n" +
		"data: " + `{"usage":{"prompt_tokens":12,"completion_tokens":3}}` + "\n" +
		"data: [DONE]\n"
	m, ok := classifyLLMExchange("api.openai.com", "/v1/chat/completions", req, resp)
	if !ok || m.Provider != "openai" || m.Model != "gpt-4.1" || !m.Stream {
		t.Fatalf("classification failed: %+v ok=%v", m, ok)
	}
	if m.PromptChars == 0 || m.CompletionChars == 0 {
		t.Fatalf("text dimensions missing: %+v", m)
	}
	if m.InputTokens != 12 || m.OutputTokens != 3 {
		t.Fatalf("usage wrong: %+v", m)
	}
}

func TestClassifyLLMExchangeOllama(t *testing.T) {
	m, ok := classifyLLMExchange("ollama.local:11434", "/api/chat",
		`{"model":"llama3","messages":[{"role":"user","content":"hello"}]}`,
		`{"model":"llama3","message":{"role":"assistant","content":"world"},"prompt_eval_count":8,"eval_count":2}`)
	if !ok || m.Provider != "ollama" || m.Model != "llama3" {
		t.Fatalf("classification failed: %+v", m)
	}
	if m.InputTokens != 8 || m.OutputTokens != 2 {
		t.Fatalf("ollama usage wrong: %+v", m)
	}
}

func TestClassifyLLMExchangeRejectsOrdinaryHTTP(t *testing.T) {
	if _, ok := classifyLLMExchange("example.com", "/api/users", `{}`, `{}`); ok {
		t.Fatal("ordinary HTTP must not be classified as LLM")
	}
}

func TestClassifyLLMTLSMetadataByKnownProviderHost(t *testing.T) {
	tests := []struct {
		host     string
		provider string
	}{
		{"api.openai.com", "openai"},
		{"eastus.openai.azure.com", "azure-openai"},
		{"api.anthropic.com", "anthropic"},
		{"generativelanguage.googleapis.com", "google-gemini"},
		{"api.deepseek.com", "deepseek"},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			m, ok := classifyLLMExchange(tt.host, "", "", "")
			if !ok || m.Provider != tt.provider || m.Operation != "tls-metadata" {
				t.Fatalf("TLS metadata classification for %s = %+v ok=%v", tt.host, m, ok)
			}
		})
	}
}

func TestEnrichLLMAuditEventPersistsTLSDimensions(t *testing.T) {
	ev := shared.ContentAuditEvent{
		Protocol: "tls",
		Host:     "api.openai.com",
	}
	enrichLLMAuditEvent(&ev)
	if ev.LLMProvider != "openai" || ev.LLMOperation != "tls-metadata" {
		t.Fatalf("TLS LLM dimensions not persisted: %+v", ev)
	}

	ev.LLMProvider = "private-provider"
	enrichLLMAuditEvent(&ev)
	if ev.LLMProvider != "private-provider" {
		t.Fatalf("explicit structured metadata was overwritten: %+v", ev)
	}

	oversized := shared.ContentAuditEvent{
		Host: "api.openai.com", Path: "/v1/responses",
		Body: `{"model":"` + strings.Repeat("m", 2048) + `"}`,
	}
	enrichLLMAuditEvent(&oversized)
	if len(oversized.LLMModel) != 256 {
		t.Fatalf("derived model was not bounded: %d", len(oversized.LLMModel))
	}
}
