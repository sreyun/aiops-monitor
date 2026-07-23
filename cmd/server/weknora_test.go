package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeWeKnoraBaseURL(t *testing.T) {
	cases := map[string]string{
		"":                              "",
		"http://localhost:8080":         "http://localhost:8080/api/v1",
		"http://localhost:8080/":        "http://localhost:8080/api/v1",
		"http://localhost:8080/api/v1":  "http://localhost:8080/api/v1",
		"http://localhost:8080/api/v1/": "http://localhost:8080/api/v1",
	}
	for in, want := range cases {
		if got := normalizeWeKnoraBaseURL(in); got != want {
			t.Errorf("normalize(%q)=%q want %q", in, got, want)
		}
	}
}

func TestParseWeKnoraKBIDs(t *testing.T) {
	got := parseWeKnoraKBIDs("kb-1, kb-2;kb-1  kb-3")
	if len(got) != 3 || got[0] != "kb-1" || got[1] != "kb-2" || got[2] != "kb-3" {
		t.Fatalf("got=%v", got)
	}
}

func TestParseWeKnoraSearchResponseData(t *testing.T) {
	raw := []byte(`{"success":true,"data":[{"id":"c1","content":"磁盘清理流程","knowledge_title":"运维手册","score":0.91}]}`)
	chunks, err := parseWeKnoraSearchResponse(raw)
	if err != nil || len(chunks) != 1 || chunks[0].Content != "磁盘清理流程" {
		t.Fatalf("err=%v chunks=%+v", err, chunks)
	}
	text := formatWeKnoraChunks(chunks)
	if !strings.Contains(text, "运维手册") || !strings.Contains(text, "磁盘清理") {
		t.Fatalf("bad format: %s", text)
	}
}

func TestParseWeKnoraSearchResponseChunksAlias(t *testing.T) {
	raw := []byte(`{"chunks":[{"content":"FAQ 答案","knowledge_filename":"faq.pdf","score":0.5}]}`)
	chunks, err := parseWeKnoraSearchResponse(raw)
	if err != nil || len(chunks) != 1 {
		t.Fatalf("err=%v chunks=%+v", err, chunks)
	}
	if !strings.Contains(formatWeKnoraChunks(chunks), "faq.pdf") {
		t.Fatal("filename missing")
	}
}

func TestWeKnoraSearchHTTP(t *testing.T) {
	var gotKey, gotPath string
	var gotBody weknoraSearchReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-API-Key")
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": []map[string]any{
				{"content": "Nginx 502 排查：先看 upstream", "knowledge_title": "Web 故障手册", "score": 0.88},
			},
		})
	}))
	defer srv.Close()

	cfg := AIConfig{
		WeKnoraEnabled:          true,
		WeKnoraURL:              srv.URL,
		WeKnoraAPIKey:           "sk-test-key",
		WeKnoraKnowledgeBaseIDs: "kb-ops",
	}
	out, err := weknoraSearch(cfg, "Nginx 502", 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "sk-test-key" {
		t.Fatalf("X-API-Key=%q", gotKey)
	}
	if gotPath != "/api/v1/knowledge-search" {
		t.Fatalf("path=%q", gotPath)
	}
	if gotBody.Query != "Nginx 502" || gotBody.KnowledgeBaseID != "kb-ops" {
		t.Fatalf("body=%+v", gotBody)
	}
	if !strings.Contains(out, "Web 故障手册") || !strings.Contains(out, "upstream") {
		t.Fatalf("out=%s", out)
	}
}

func TestWeKnoraConfigured(t *testing.T) {
	if weknoraConfigured(AIConfig{}) {
		t.Fatal("empty should be false")
	}
	if !weknoraConfigured(AIConfig{WeKnoraEnabled: true, WeKnoraURL: "http://x", WeKnoraAPIKey: "k"}) {
		t.Fatal("want true")
	}
}

func TestSearchKnowledgeToolRegistered(t *testing.T) {
	h := &SreyunCore{tools: map[string]SreyunTool{}}
	h.registerTools()
	tool, ok := h.tools["search_knowledge"]
	if !ok || tool.Execute == nil || tool.Description == "" {
		t.Fatal("search_knowledge not registered properly")
	}
	p, _ := tool.Parameters["properties"].(map[string]any)
	if _, ok := p["query"]; !ok {
		t.Fatal("missing query param")
	}
}
