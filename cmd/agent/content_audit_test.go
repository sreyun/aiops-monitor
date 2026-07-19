package main

import (
	"strings"
	"testing"
)

func TestParseHTTPRequest(t *testing.T) {
	// 典型的内网大模型请求（Ollama /api/chat），首包含请求行+Host+prompt。
	req := "POST /api/chat HTTP/1.1\r\n" +
		"Host: ollama.local:11434\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: 60\r\n\r\n" +
		`{"model":"llama3","messages":[{"role":"user","content":"你好"}]}`
	ev, ok := parseHTTPRequest([]byte(req), 4096)
	if !ok {
		t.Fatal("应识别为 HTTP 请求")
	}
	if ev.Method != "POST" || ev.Path != "/api/chat" {
		t.Errorf("请求行错: method=%q path=%q", ev.Method, ev.Path)
	}
	if ev.Host != "ollama.local:11434" {
		t.Errorf("Host 错: %q", ev.Host)
	}
	if ev.CType != "application/json" {
		t.Errorf("Content-Type 错: %q", ev.CType)
	}
	if !strings.Contains(ev.Body, "llama3") || !strings.Contains(ev.Body, "你好") {
		t.Errorf("body/prompt 未提取: %q", ev.Body)
	}

	// body 截断到 maxBody
	ev2, _ := parseHTTPRequest([]byte(req), 10)
	if len([]byte(ev2.Body)) > 10 {
		t.Errorf("body 未按 maxBody 截断: %d 字节", len(ev2.Body))
	}

	// 非 HTTP（TLS 握手 / 随机数据）→ 拒绝
	if _, ok := parseHTTPRequest([]byte{0x16, 0x03, 0x01, 0x00, 0x05}, 4096); ok {
		t.Error("TLS 握手不该被当 HTTP 请求")
	}
	if _, ok := parseHTTPRequest([]byte("just some random tcp bytes"), 4096); ok {
		t.Error("随机数据不该被当 HTTP 请求")
	}
}

func TestContentPortMatch(t *testing.T) {
	if !contentPortMatch(nil, 11434) {
		t.Error("空白名单应对所有端口放行")
	}
	if !contentPortMatch([]int{80, 11434}, 11434) {
		t.Error("命中端口应放行")
	}
	if contentPortMatch([]int{80, 443}, 11434) {
		t.Error("未命中端口应拒绝")
	}
}
