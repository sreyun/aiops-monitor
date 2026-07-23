package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ============================================================================
// WeKnora 外部文档 RAG
//
// 复杂运维文档（手册/Wiki/PDF）由 WeKnora 独立维护；本平台仅通过 API URL + API Key
// 调用 POST /api/v1/knowledge-search，把命中片段作为 Sreyun 工具 search_knowledge 注入诊断/对话。
// 认证头：X-API-Key（见 WeKnora 官方 API 文档）。
// ============================================================================

const weknoraSearchPath = "/knowledge-search"

// weknoraConfigured 判断是否已填写可用的 WeKnora 对接参数（开关 + URL + Key）。
func weknoraConfigured(cfg AIConfig) bool {
	return cfg.WeKnoraEnabled &&
		strings.TrimSpace(cfg.WeKnoraURL) != "" &&
		strings.TrimSpace(cfg.WeKnoraAPIKey) != ""
}

// normalizeWeKnoraBaseURL 归一化用户填写的 API 根地址为 …/api/v1。
// 接受 http://host:8080 或 http://host:8080/api/v1（尾斜杠可有可无）。
func normalizeWeKnoraBaseURL(raw string) string {
	u := strings.TrimRight(strings.TrimSpace(raw), "/")
	if u == "" {
		return ""
	}
	if strings.HasSuffix(u, "/api/v1") {
		return u
	}
	return u + "/api/v1"
}

// parseWeKnoraKBIDs 解析逗号/空白分隔的知识库 ID 列表。
func parseWeKnoraKBIDs(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// weknoraChunk 是 knowledge-search 返回的一条命中（兼容 data[] / chunks[] 字段名差异）。
type weknoraChunk struct {
	ID                 string  `json:"id"`
	Content            string  `json:"content"`
	KnowledgeID        string  `json:"knowledge_id"`
	KnowledgeTitle     string  `json:"knowledge_title"`
	KnowledgeFilename  string  `json:"knowledge_filename"`
	Score              float64 `json:"score"`
	ChunkIndex         int     `json:"chunk_index"`
	SourceDoc          string  `json:"source_doc"` // 部分实现别名
}

// weknoraSearchReq 对应 POST /knowledge-search 请求体。
type weknoraSearchReq struct {
	Query             string   `json:"query"`
	KnowledgeBaseID   string   `json:"knowledge_base_id,omitempty"`
	KnowledgeBaseIDs  []string `json:"knowledge_base_ids,omitempty"`
	KnowledgeIDs      []string `json:"knowledge_ids,omitempty"`
	TopK              int      `json:"top_k,omitempty"`
}

// weknoraSearch 调用 WeKnora 知识搜索，返回格式化文本（供工具/测试使用）。
func weknoraSearch(cfg AIConfig, query string, topK int, overrideKBIDs []string) (string, error) {
	chunks, err := weknoraSearchChunks(cfg, query, topK, overrideKBIDs)
	if err != nil {
		markWeKnoraFail(err)
		return "", err
	}
	markWeKnoraOK()
	return formatWeKnoraChunks(chunks), nil
}

func weknoraSearchChunks(cfg AIConfig, query string, topK int, overrideKBIDs []string) ([]weknoraChunk, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("查询不能为空")
	}
	base := normalizeWeKnoraBaseURL(cfg.WeKnoraURL)
	key := strings.TrimSpace(cfg.WeKnoraAPIKey)
	if base == "" || key == "" {
		return nil, fmt.Errorf("未配置 WeKnora URL 或 API Key")
	}
	if topK <= 0 {
		topK = 5
	}
	if topK > 20 {
		topK = 20
	}
	kbIDs := overrideKBIDs
	if len(kbIDs) == 0 {
		kbIDs = parseWeKnoraKBIDs(cfg.WeKnoraKnowledgeBaseIDs)
	}
	body := weknoraSearchReq{Query: query, TopK: topK}
	switch {
	case len(kbIDs) == 1:
		body.KnowledgeBaseID = kbIDs[0]
	case len(kbIDs) > 1:
		body.KnowledgeBaseIDs = kbIDs
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, base+weknoraSearchPath, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", key)
	resp, err := newGuardedHTTPClient(20 * time.Second).Do(req)
	if err != nil {
		return nil, fmt.Errorf("WeKnora 请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("WeKnora HTTP %d: %s", resp.StatusCode, trimLine(string(raw), 200))
	}
	return parseWeKnoraSearchResponse(raw)
}

// parseWeKnoraSearchResponse 兼容官方 {success,data:[]} 与社区 {chunks:[]} / {results:[]}。
func parseWeKnoraSearchResponse(raw []byte) ([]weknoraChunk, error) {
	var envelope struct {
		Success bool            `json:"success"`
		Data    []weknoraChunk  `json:"data"`
		Chunks  []weknoraChunk  `json:"chunks"`
		Results []weknoraChunk  `json:"results"`
		Total   int             `json:"total"`
		Error   string          `json:"error"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("解析 WeKnora 响应失败: %w", err)
	}
	if envelope.Error != "" || (envelope.Message != "" && len(envelope.Data) == 0 && len(envelope.Chunks) == 0 && len(envelope.Results) == 0) {
		msg := envelope.Error
		if msg == "" {
			msg = envelope.Message
		}
		return nil, fmt.Errorf("WeKnora: %s", msg)
	}
	switch {
	case len(envelope.Data) > 0:
		return envelope.Data, nil
	case len(envelope.Chunks) > 0:
		return envelope.Chunks, nil
	case len(envelope.Results) > 0:
		return envelope.Results, nil
	}
	return nil, nil
}

func formatWeKnoraChunks(chunks []weknoraChunk) string {
	if len(chunks) == 0 {
		return "未在 WeKnora 知识库中找到相关文档片段"
	}
	var b strings.Builder
	b.WriteString("WeKnora 知识库检索结果：\n")
	for i, c := range chunks {
		title := strings.TrimSpace(c.KnowledgeTitle)
		if title == "" {
			title = strings.TrimSpace(c.KnowledgeFilename)
		}
		if title == "" {
			title = strings.TrimSpace(c.SourceDoc)
		}
		if title == "" {
			title = "未命名文档"
		}
		score := ""
		if c.Score > 0 {
			score = fmt.Sprintf(" 相关度 %.2f", c.Score)
		}
		fmt.Fprintf(&b, "  %d. 【%s】%s\n     %s\n", i+1, title, score, trimLine(c.Content, 400))
	}
	return b.String()
}
