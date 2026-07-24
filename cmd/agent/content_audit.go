package main

// Phase 2 · 明文 HTTP 内容审计（增量 1：单包解析）。从 TCP 明文载荷里解析 HTTP【请求】的
// 请求行(method/path) + Host + Content-Type + body 前缀。请求行与 Host 头几乎总在首包最前，
// 因此单包即可拿到"谁 POST 到哪个大模型端点、prompt 开头是什么"——先不做全流重组（增量 2）。
// ⚠ 高敏感：body 可能含用户发给大模型的 prompt(PII)。仅在 content_audit=true 时启用，默认关闭。

import (
	"bytes"
	"strings"

	"aiops-monitor/shared"
)

var httpMethods = [][]byte{
	[]byte("GET "), []byte("POST "), []byte("PUT "), []byte("DELETE "),
	[]byte("PATCH "), []byte("HEAD "), []byte("OPTIONS "),
}

// looksLikeHTTPRequest 判断载荷是否以 HTTP 请求方法开头（快速早退，避免对每个 TCP 包全解析）。
func looksLikeHTTPRequest(p []byte) bool {
	for _, m := range httpMethods {
		if len(p) >= len(m) && bytes.Equal(p[:len(m)], m) {
			return true
		}
	}
	return false
}

// parseHTTPRequest 从单个 TCP 载荷解析 HTTP 请求关键信息。maxBody 截断 body 前缀。
// 返回 ok=false 表示不是 HTTP 请求 / 解析失败。
func parseHTTPRequest(payload []byte, maxBody int) (shared.ContentAuditEvent, bool) {
	if !looksLikeHTTPRequest(payload) {
		return shared.ContentAuditEvent{}, false
	}
	var ev shared.ContentAuditEvent
	ev.Protocol = "http"
	head := payload
	var body []byte
	if sep := bytes.Index(payload, []byte("\r\n\r\n")); sep >= 0 {
		head = payload[:sep]
		body = payload[sep+4:]
	}
	lines := strings.Split(string(head), "\r\n")
	if len(lines) == 0 {
		return ev, false
	}
	// 请求行：METHOD SP PATH SP HTTP/x
	rl := strings.Fields(lines[0])
	if len(rl) < 2 {
		return ev, false
	}
	ev.Method = rl[0]
	ev.Path = rl[1]
	for _, ln := range lines[1:] {
		lower := strings.ToLower(ln)
		switch {
		case strings.HasPrefix(lower, "host:"):
			ev.Host = strings.TrimSpace(ln[len("host:"):])
		case strings.HasPrefix(lower, "content-type:"):
			ev.CType = strings.TrimSpace(ln[len("content-type:"):])
		}
	}
	if maxBody <= 0 {
		maxBody = 4096
	}
	if len(body) > 0 {
		if len(body) > maxBody {
			body = body[:maxBody]
		}
		// 去除不可打印字节的干扰（保留常见可读内容）；body 可能非 UTF-8，做保守清洗。
		ev.Body = strings.ToValidUTF8(string(body), "")
	}
	return ev, true
}

// contentPortMatch 判断目的端口是否在审计白名单（空白名单=对所有 TCP 试探 HTTP）。
func contentPortMatch(ports []int, dstPort uint16) bool {
	if len(ports) == 0 {
		return true
	}
	for _, p := range ports {
		if p == int(dstPort) {
			return true
		}
	}
	return false
}
