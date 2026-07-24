package main

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"aiops-monitor/shared"
)

// llmAuditMeta is derived at query time from captured HTTP exchanges. It adds
// useful LLM governance dimensions without duplicating or further retaining
// prompt/completion content.
type llmAuditMeta struct {
	Provider        string
	Model           string
	Operation       string
	Stream          bool
	PromptChars     int
	CompletionChars int
	InputTokens     int
	OutputTokens    int
	ToolCalls       int
}

// enrichLLMAuditEvent persists body-independent LLM dimensions before insert,
// so indexed provider/model filters also work for passive TLS metadata events.
// Explicit Gateway/SDK dimensions always win over heuristic classification.
func enrichLLMAuditEvent(ev *shared.ContentAuditEvent) {
	if ev == nil {
		return
	}
	m, ok := classifyLLMExchange(ev.Host, ev.Path, ev.Body, ev.RespBody)
	if !ok {
		return
	}
	if ev.LLMProvider == "" {
		ev.LLMProvider = m.Provider
	}
	if ev.LLMModel == "" {
		ev.LLMModel = m.Model
	}
	if ev.LLMOperation == "" {
		ev.LLMOperation = m.Operation
	}
	if !ev.LLMStream {
		ev.LLMStream = m.Stream
	}
	if ev.InputTokens == 0 {
		ev.InputTokens = m.InputTokens
	}
	if ev.OutputTokens == 0 {
		ev.OutputTokens = m.OutputTokens
	}
	if ev.ToolCalls == 0 {
		ev.ToolCalls = m.ToolCalls
	}
	// Heuristic values are derived after zero-trust ingest normalization, so
	// apply the same storage bounds here instead of trusting JSON model/usage.
	ev.LLMProvider = truncateAuditField(strings.ToLower(strings.TrimSpace(ev.LLMProvider)), 128)
	ev.LLMModel = truncateAuditField(strings.TrimSpace(ev.LLMModel), 256)
	ev.LLMOperation = truncateAuditField(strings.ToLower(strings.TrimSpace(ev.LLMOperation)), 128)
	for _, n := range []*int{&ev.InputTokens, &ev.OutputTokens, &ev.ToolCalls} {
		if *n < 0 || *n > 1_000_000_000 {
			*n = 0
		}
	}
}

func annotateLLMAuditEvent(ev map[string]any) {
	str := func(k string) string {
		v, _ := ev[k].(string)
		return v
	}
	meta, derived := classifyLLMExchange(str("host"), str("path"), str("body"), str("resp_body"))
	structured := str("llm_provider") != "" || str("llm_model") != "" || str("llm_operation") != ""
	if !derived && !structured {
		ev["is_llm"] = false
		return
	}
	ev["is_llm"] = true
	setStringIfEmpty(ev, "llm_provider", meta.Provider)
	setStringIfEmpty(ev, "llm_model", meta.Model)
	setStringIfEmpty(ev, "llm_operation", meta.Operation)
	if _, exists := ev["llm_stream"]; !exists || ev["llm_stream"] == false {
		ev["llm_stream"] = meta.Stream
	}
	ev["llm_prompt_chars"] = meta.PromptChars
	ev["llm_completion_chars"] = meta.CompletionChars
	setIntIfZero(ev, "llm_input_tokens", meta.InputTokens)
	setIntIfZero(ev, "llm_output_tokens", meta.OutputTokens)
	setIntIfZero(ev, "llm_tool_calls", meta.ToolCalls)
}

func setStringIfEmpty(ev map[string]any, key, value string) {
	if current, _ := ev[key].(string); current == "" && value != "" {
		ev[key] = value
	}
}

func setIntIfZero(ev map[string]any, key string, value int) {
	if value == 0 {
		return
	}
	switch current := ev[key].(type) {
	case int:
		if current == 0 {
			ev[key] = value
		}
	case int64:
		if current == 0 {
			ev[key] = value
		}
	case float64:
		if current == 0 {
			ev[key] = value
		}
	default:
		ev[key] = value
	}
}

func classifyLLMExchange(host, path, reqBody, respBody string) (llmAuditMeta, bool) {
	h, p := strings.ToLower(host), strings.ToLower(path)
	var m llmAuditMeta
	switch {
	case strings.Contains(p, "/api/chat"):
		m.Provider, m.Operation = "ollama", "chat"
	case strings.Contains(p, "/api/generate"):
		m.Provider, m.Operation = "ollama", "generate"
	case strings.Contains(p, "/v1/messages"):
		m.Provider, m.Operation = "anthropic", "messages"
	case strings.Contains(p, "/v1/chat/completions"):
		m.Provider, m.Operation = "openai-compatible", "chat.completions"
	case strings.Contains(p, "/v1/responses"):
		m.Provider, m.Operation = "openai-compatible", "responses"
	case strings.Contains(p, "/v1/completions"):
		m.Provider, m.Operation = "openai-compatible", "completions"
	case strings.Contains(p, "/v1/embeddings"):
		m.Provider, m.Operation = "openai-compatible", "embeddings"
	default:
		switch {
		case strings.Contains(h, "openai."),
			strings.Contains(h, "anthropic."),
			strings.Contains(h, "openai.azure."),
			strings.Contains(h, "generativelanguage.googleapis."),
			strings.Contains(h, "cohere."),
			strings.Contains(h, "mistral."),
			strings.Contains(h, "deepseek."),
			strings.Contains(h, "groq."),
			strings.Contains(h, "together."),
			strings.Contains(h, "api.x.ai"):
			m.Operation = "tls-metadata"
		default:
			return m, false
		}
	}
	switch {
	case strings.Contains(h, "azure") && strings.Contains(h, "openai"):
		m.Provider = "azure-openai"
	case strings.Contains(h, "openai."):
		m.Provider = "openai"
	case strings.Contains(h, "anthropic."):
		m.Provider = "anthropic"
	case strings.Contains(h, "ollama"):
		m.Provider = "ollama"
	case strings.Contains(h, "generativelanguage.googleapis"):
		m.Provider = "google-gemini"
	case strings.Contains(h, "cohere."):
		m.Provider = "cohere"
	case strings.Contains(h, "mistral."):
		m.Provider = "mistral"
	case strings.Contains(h, "deepseek."):
		m.Provider = "deepseek"
	case strings.Contains(h, "groq."):
		m.Provider = "groq"
	case strings.Contains(h, "together."):
		m.Provider = "together"
	case strings.Contains(h, "api.x.ai"):
		m.Provider = "xai"
	}

	if req := firstJSONDocument(reqBody); req != nil {
		m.Model, _ = req["model"].(string)
		m.Stream, _ = req["stream"].(bool)
		m.PromptChars = textChars(req, map[string]bool{
			"messages": true, "prompt": true, "input": true, "instructions": true,
		})
		m.ToolCalls += countArrayKey(req, "tool_calls")
	}
	for _, resp := range jsonDocuments(respBody) {
		if m.Model == "" {
			m.Model, _ = resp["model"].(string)
		}
		m.CompletionChars += textChars(resp, map[string]bool{
			"choices": true, "message": true, "delta": true, "content": true,
			"text": true, "response": true, "completion": true, "output": true,
		})
		m.ToolCalls += countArrayKey(resp, "tool_calls")
		in, out := tokenUsage(resp)
		if in > m.InputTokens {
			m.InputTokens = in
		}
		if out > m.OutputTokens {
			m.OutputTokens = out
		}
	}
	return m, true
}

func firstJSONDocument(body string) map[string]any {
	docs := jsonDocuments(body)
	if len(docs) == 0 {
		return nil
	}
	return docs[0]
}

func jsonDocuments(body string) []map[string]any {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	var one map[string]any
	if json.Unmarshal([]byte(body), &one) == nil {
		return []map[string]any{one}
	}
	var out []map[string]any
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if line == "" || line == "[DONE]" {
			continue
		}
		var doc map[string]any
		if json.Unmarshal([]byte(line), &doc) == nil {
			out = append(out, doc)
		}
	}
	return out
}

func textChars(v any, allowed map[string]bool) int {
	var walk func(any, bool) int
	walk = func(x any, inAllowed bool) int {
		switch t := x.(type) {
		case string:
			if inAllowed {
				return utf8.RuneCountInString(t)
			}
		case []any:
			n := 0
			for _, item := range t {
				n += walk(item, inAllowed)
			}
			return n
		case map[string]any:
			n := 0
			for k, item := range t {
				n += walk(item, inAllowed || allowed[strings.ToLower(k)])
			}
			return n
		}
		return 0
	}
	return walk(v, false)
}

func countArrayKey(v any, key string) int {
	switch t := v.(type) {
	case []any:
		n := 0
		for _, item := range t {
			n += countArrayKey(item, key)
		}
		return n
	case map[string]any:
		n := 0
		for k, item := range t {
			if strings.EqualFold(k, key) {
				if arr, ok := item.([]any); ok {
					n += len(arr)
				}
			}
			n += countArrayKey(item, key)
		}
		return n
	}
	return 0
}

func tokenUsage(v map[string]any) (int, int) {
	usage, _ := v["usage"].(map[string]any)
	num := func(keys ...string) int {
		for _, k := range keys {
			if n, ok := usage[k].(float64); ok {
				return int(n)
			}
			if n, ok := v[k].(float64); ok { // Ollama prompt_eval_count/eval_count
				return int(n)
			}
		}
		return 0
	}
	return num("prompt_tokens", "input_tokens", "prompt_eval_count"),
		num("completion_tokens", "output_tokens", "eval_count")
}
