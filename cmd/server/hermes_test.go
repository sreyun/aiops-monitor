package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"aiops-monitor/shared"
)

// TestHermesHostContextAndToolLoop 端到端验证 Hermes AI 对话的核心修复：
//  1. 系统提示词自动注入当前纳管主机（id/主机名/IP/状态），且不再"禁止 JSON"、允许 tool_calls 协议；
//  2. 完整工具调用闭环：模型据主机清单发起 tool_call → query_metrics 执行 → 拿到真实指标 → 生成最终中文结论；
//  3. 多轮记忆：会话消息（user+assistant）被正确追加。
//
// 用 httptest 模拟一个 OpenAI 兼容的 /chat/completions 端点，无需真实 LLM 或 Docker。
func TestHermesHostContextAndToolLoop(t *testing.T) {
	var turn int
	var firstReqBody string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if turn == 0 {
			firstReqBody = string(body)
		}
		turn++
		var content string
		if turn == 1 {
			// 第一轮：模型据主机清单发起工具调用（host_id 取自注入的主机上下文）
			content = `{"tool_calls":[{"name":"query_metrics","args":{"host_id":"h-web01","metric":"cpu"}}]}`
		} else {
			// 第二轮：据工具真实结果给出自然语言结论
			content = "web-01 当前 CPU 使用率约 42%，负载正常，无需处理。"
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%s}}]}`, strconv.Quote(content))
	}))
	defer mock.Close()

	store := NewStore()
	hst := store.RegisterHost("h-web01", "web-01", "fp-test")
	hst.IP = "10.0.0.11"
	hst.OS = "linux"
	hst.Latest = &shared.Sample{Metrics: shared.Metrics{CPUPercent: 42, MemPercent: 63}}

	cfg := newTestConfigStore(t)
	if err := cfg.SetAIConfig(AIConfig{
		Enabled: true, Endpoint: mock.URL + "/v1", Model: "mock-model", APIKey: "test-key",
	}); err != nil {
		t.Fatalf("SetAIConfig: %v", err)
	}
	s := &Server{store: store, cfg: cfg}
	h := newHermesCore(s)

	// (1) 主机上下文注入 + JSON 禁令已移除
	sys := h.buildSystemPrompt()
	for _, want := range []string{"h-web01", "web-01", "10.0.0.11", "在线", "tool_calls", "当前纳管主机"} {
		if !strings.Contains(sys, want) {
			t.Fatalf("系统提示词缺少 %q：\n%s", want, sys)
		}
	}
	if strings.Contains(sys, "禁止输出任何 JSON") {
		t.Fatalf("系统提示词仍包含旧的 JSON 禁令，会阻止工具调用")
	}

	// (2) 工具调用闭环
	sess := &HermesSession{}
	reply, err := h.Chat(sess, "查询主机 web-01 的 CPU 使用率", false, nil)
	if err != nil {
		t.Fatalf("Chat 出错: %v", err)
	}
	if turn < 2 {
		t.Fatalf("预期发生工具调用（≥2 轮 LLM 调用），实际 %d 轮", turn)
	}
	if !strings.Contains(firstReqBody, "h-web01") {
		t.Fatalf("首轮 LLM 请求未携带主机清单上下文")
	}
	if !strings.Contains(reply, "CPU") {
		t.Fatalf("最终回复未包含预期内容: %q", reply)
	}
	if strings.Contains(reply, "tool_calls") {
		t.Fatalf("最终回复不应泄露 tool_calls JSON: %q", reply)
	}

	// (3) 多轮记忆：user + assistant 各一条
	if len(sess.Messages) != 2 || sess.Messages[0]["role"] != "user" || sess.Messages[1]["role"] != "assistant" {
		t.Fatalf("会话消息未正确追加: %+v", sess.Messages)
	}
	if sess.Messages[1]["content"] != reply {
		t.Fatalf("持久化的 assistant 消息应等于最终结论")
	}
}

// TestStripToolCallJSON 验证 tool_calls JSON / 代码块被剥离，只保留自然语言（对用户不可见 JSON）。
func TestStripToolCallJSON(t *testing.T) {
	cases := []struct{ in, want string }{
		{"让我查一下。\n{\"tool_calls\":[{\"name\":\"query_metrics\",\"args\":{\"host_id\":\"h1\"}}]}", "让我查一下。"},
		{"```json\n{\"tool_calls\":[]}\n```\n结论：一切正常", "结论：一切正常"},
		{"纯文本结论", "纯文本结论"},
		{"{\"tool_calls\":[{\"name\":\"list_alerts\",\"args\":{}}]}", ""},
	}
	for i, c := range cases {
		if got := stripToolCallJSON(c.in); got != c.want {
			t.Errorf("case %d: stripToolCallJSON(%q)=%q, want %q", i, c.in, got, c.want)
		}
	}
}

// TestSanitizeHistory 验证前端历史清洗：过滤非法角色 / 空内容。
func TestSanitizeHistory(t *testing.T) {
	in := []map[string]string{
		{"role": "user", "content": "hi"},
		{"role": "system", "content": "应被丢弃"},
		{"role": "assistant", "content": ""},
		{"role": "assistant", "content": "ok"},
	}
	out := sanitizeHistory(in)
	if len(out) != 2 || out[0]["content"] != "hi" || out[1]["content"] != "ok" {
		t.Fatalf("sanitizeHistory 结果异常: %+v", out)
	}
}

// TestHandleAIModelsLiveOnlyNoCurated 验证 /ai/models 只返回自动获取的实时模型（去重、provider=live），
// 且不再返回任何内置预设 / 精选模型。
func TestHandleAIModelsLiveOnlyNoCurated(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"id":"model-a"},{"id":"model-b"},{"id":"model-a"}]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mock.Close()

	s := &Server{store: NewStore(), cfg: newTestConfigStore(t)}

	req := httptest.NewRequest("POST", "/api/v1/ai/models",
		strings.NewReader(`{"endpoint":"`+mock.URL+`","api_key":"k"}`))
	w := httptest.NewRecorder()
	s.handleAIModels(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Models []struct {
			Value    string `json:"value"`
			Provider string `json:"provider"`
		} `json:"models"`
		LiveCount int `json:"live_count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// 去重后应为 2 个实时模型
	if resp.LiveCount != 2 || len(resp.Models) != 2 {
		t.Fatalf("期望 2 个实时模型，得到 live_count=%d models=%d: %s", resp.LiveCount, len(resp.Models), w.Body.String())
	}
	got := map[string]bool{}
	for _, m := range resp.Models {
		got[m.Value] = true
		if m.Provider != "live" {
			t.Errorf("模型 %s provider 应为 live，实为 %q", m.Value, m.Provider)
		}
	}
	if !got["model-a"] || !got["model-b"] {
		t.Fatalf("缺少自动获取的模型: %+v", resp.Models)
	}
	// 确认不再包含任何内置预设 / 精选模型
	for _, curated := range []string{"gpt-4o-mini", "gpt-4o", "qwen-plus", "qwen-max", "qwen-turbo", "deepseek-chat", "deepseek-reasoner", "claude-3-5-sonnet-20241022"} {
		if got[curated] {
			t.Errorf("不应再包含内置预设模型 %q", curated)
		}
	}
}
