package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExternalMCPToolName(t *testing.T) {
	got := externalMCPToolName("Jira-Prod", "get_issue")
	if got != "ext_jira-prod_get_issue" {
		t.Fatalf("got %q", got)
	}
}

func TestMCPToolAllowedByClientPolicy(t *testing.T) {
	c := MCPClientConfig{}
	if mcpToolAllowedByClientPolicy("delete_issue", c) {
		t.Fatal("dangerous name should be blocked by default")
	}
	if !mcpToolAllowedByClientPolicy("search", c) {
		t.Fatal("safe name should be allowed")
	}
	c.ToolAllowlist = []string{"delete_issue"}
	if !mcpToolAllowedByClientPolicy("delete_issue", c) {
		t.Fatal("allowlist should override danger filter")
	}
	c.ToolBlocklist = []string{"search"}
	c.ToolAllowlist = nil
	if mcpToolAllowedByClientPolicy("search", c) {
		t.Fatal("blocklist should win")
	}
}

func TestMergeMCPClientsJSONPreservesAuth(t *testing.T) {
	saved := `[{"id":"a1","name":"A","enabled":true,"url":"http://x/mcp","headers":{"Authorization":"Bearer secret"}}]`
	incoming := `[{"id":"a1","name":"A","enabled":true,"url":"http://x/mcp","headers":{"Authorization":"****"}}]`
	out := mergeMCPClientsJSON(incoming, saved)
	list, err := parseMCPClientsJSON(out)
	if err != nil || len(list) != 1 {
		t.Fatalf("parse: %v %#v", err, list)
	}
	if list[0].Headers["Authorization"] != "Bearer secret" {
		t.Fatalf("auth not merged: %#v", list[0].Headers)
	}
}

func TestMaskMCPClientsJSONForAPI(t *testing.T) {
	raw := `[{"id":"a1","name":"A","url":"http://x","headers":{"Authorization":"Bearer secret","X-Debug":"1"}}]`
	masked := maskMCPClientsJSONForAPI(raw)
	list, err := parseMCPClientsJSON(masked)
	if err != nil || len(list) != 1 {
		t.Fatalf("parse: %v", err)
	}
	if list[0].Headers["Authorization"] != "****" {
		t.Fatalf("auth not masked: %#v", list[0].Headers)
	}
	if list[0].Headers["X-Debug"] != "1" {
		t.Fatalf("non-secret header changed: %#v", list[0].Headers)
	}
}

func TestMCPHTTPClientToolsListAndCall(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			ID     json.RawMessage `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"protocolVersion": mcpClientProtocolVersion, "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "mock"}},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{
					"tools": []map[string]any{
						{"name": "echo", "description": "echo text", "inputSchema": map[string]any{"type": "object"}},
						{"name": "delete_row", "description": "danger", "inputSchema": map[string]any{"type": "object"}},
					},
				},
			})
		case "tools/call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "hello-ext"}},
				},
			})
		default:
			http.Error(w, "unknown method "+req.Method, 400)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := MCPClientConfig{ID: "mock", Name: "mock", Enabled: true, URL: srv.URL, TimeoutSec: 5}
	tools, err := TestAndListTools(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 {
		t.Fatalf("tools=%d", len(tools))
	}
	blocked := 0
	for _, t0 := range tools {
		if t0.Blocked {
			blocked++
		}
	}
	if blocked != 1 {
		t.Fatalf("expected 1 blocked, got %d", blocked)
	}

	mgr := newMCPClientManager()
	cfg.SyncedTools = tools
	raw := encodeMCPClientsJSON([]MCPClientConfig{cfg})
	if err := mgr.Reload(raw); err != nil {
		t.Fatal(err)
	}
	out, err := mgr.Call(context.Background(), "mock", "echo", map[string]any{"q": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello-ext") {
		t.Fatalf("call result=%q", out)
	}
	if _, err := mgr.Call(context.Background(), "mock", "delete_row", nil); err == nil {
		t.Fatal("expected blocked tool error")
	}
}
