package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestParseWeKnoraKBListResponse(t *testing.T) {
	raw := []byte(`{"success":true,"data":[
		{"id":"kb-a","name":"手册","is_temporary":false,"knowledge_count":3},
		{"id":"kb-tmp","name":"临时","is_temporary":true},
		{"id":"kb-b","name":"FAQ","type":"faq"}
	]}`)
	kbs, err := parseWeKnoraKBListResponse(raw)
	if err != nil || len(kbs) != 3 {
		t.Fatalf("err=%v n=%d", err, len(kbs))
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

func TestWeKnoraSearchAutoListAllKBs(t *testing.T) {
	var listHits, searchHits atomic.Int32
	var lastSearch weknoraSearchReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/knowledge-bases"):
			listHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": []map[string]any{
					{"id": "kb-1", "name": "手册 A", "is_temporary": false},
					{"id": "kb-2", "name": "手册 B", "is_temporary": false},
					{"id": "kb-tmp", "name": "临时", "is_temporary": true},
				},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/knowledge-search"):
			searchHits.Add(1)
			_ = json.NewDecoder(r.Body).Decode(&lastSearch)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": []map[string]any{
					{"id": "c1", "content": "来自库1", "knowledge_title": "doc1", "score": 0.9},
					{"id": "c2", "content": "来自库2", "knowledge_title": "doc2", "score": 0.8},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := AIConfig{
		WeKnoraEnabled: true,
		WeKnoraURL:     srv.URL,
		WeKnoraAPIKey:  "sk-all",
		// 故意留空：应自动 list 再带 knowledge_base_ids 检索
	}
	chunks, meta, err := weknoraSearchChunksMeta(cfg, "跨库查询", 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if listHits.Load() < 1 {
		t.Fatal("expected knowledge-bases list")
	}
	if searchHits.Load() < 1 {
		t.Fatal("expected knowledge-search")
	}
	if !meta.AutoListed || meta.KBCount != 2 {
		t.Fatalf("meta=%+v", meta)
	}
	if len(lastSearch.KnowledgeBaseIDs) != 2 {
		t.Fatalf("expected 2 kb ids in search, got %+v", lastSearch)
	}
	got := map[string]bool{}
	for _, id := range lastSearch.KnowledgeBaseIDs {
		got[id] = true
	}
	if !got["kb-1"] || !got["kb-2"] || got["kb-tmp"] {
		t.Fatalf("ids=%v", lastSearch.KnowledgeBaseIDs)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks=%d", len(chunks))
	}
}

func TestWeKnoraFanOutOnEmptyBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/hybrid-search") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": []map[string]any{
					{"id": "h1", "content": "hybrid hit", "knowledge_title": "hy", "score": 0.77},
				},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/knowledge-search") {
			var body weknoraSearchReq
			_ = json.NewDecoder(r.Body).Decode(&body)
			// 多库批量返回空；单库也空 → 走 hybrid
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []any{}})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := AIConfig{
		WeKnoraEnabled:          true,
		WeKnoraURL:              srv.URL,
		WeKnoraAPIKey:           "sk",
		WeKnoraKnowledgeBaseIDs: "kb-x,kb-y",
	}
	chunks, meta, err := weknoraSearchChunksMeta(cfg, "需要 hybrid", 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Strategy != "fanout" {
		t.Fatalf("strategy=%s", meta.Strategy)
	}
	if len(chunks) == 0 || !strings.Contains(chunks[0].Content, "hybrid") {
		t.Fatalf("chunks=%+v", chunks)
	}
}

func TestMergeWeKnoraChunks(t *testing.T) {
	a := []weknoraChunk{{ID: "1", Content: "a", Score: 0.5}, {ID: "2", Content: "b", Score: 0.9}}
	b := []weknoraChunk{{ID: "2", Content: "b-dup", Score: 0.95}, {ID: "3", Content: "c", Score: 0.7}}
	m := mergeWeKnoraChunks(a, b)
	if len(m) != 3 || m[0].ID != "2" {
		t.Fatalf("merged=%+v", m)
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
