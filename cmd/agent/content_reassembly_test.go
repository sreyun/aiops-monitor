package main

import (
	"testing"

	"aiops-monitor/shared"
)

func caSeg(sIP string, sPort uint16, dIP string, dPort uint16, seq uint32, payload string, fin bool) l4Info {
	var flags uint8
	if fin {
		flags = 0x01
	}
	return l4Info{proto: 6, srcIP: sIP, srcPort: sPort, dstIP: dIP, dstPort: dPort, seq: seq, tcpFlags: flags, payload: []byte(payload)}
}

// TestReassemblyMultiPacketReqResp：请求跨 2 段(Content-Length) + 响应(Content-Length)，
// 验证重组出完整请求 body 与响应 completion。
func TestReassemblyMultiPacketReqResp(t *testing.T) {
	var got []shared.ContentAuditEvent
	ras := newReassembler(SNIConfig{ContentAuditPorts: []int{11434}, ContentAuditMaxBody: 65536},
		func(e shared.ContentAuditEvent) { got = append(got, e) })

	client, server := "10.0.0.5", "10.0.0.9"
	reqHdr := "POST /api/chat HTTP/1.1\r\nHost: ollama.local\r\nContent-Type: application/json\r\nContent-Length: 18\r\n\r\n"
	p1 := reqHdr + `{"model":` // body 前半
	p2 := `"llama3"}`          // body 后半（总 body=18）
	ras.feed(caSeg(client, 40000, server, 11434, 1000, p1, false))
	ras.feed(caSeg(client, 40000, server, 11434, 1000+uint32(len(p1)), p2, false))

	// 响应（Content-Length=16）
	resp := "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 16\r\n\r\n" + `{"resp":"hello"}`
	ras.feed(caSeg(server, 11434, client, 40000, 5000, resp, false))

	if len(got) != 1 {
		t.Fatalf("应发射 1 条内容审计, 得 %d", len(got))
	}
	e := got[0]
	if e.Method != "POST" || e.Path != "/api/chat" || e.Host != "ollama.local" {
		t.Errorf("请求头重组错: %+v", e)
	}
	if e.Body != `{"model":"llama3"}` {
		t.Errorf("多包请求 body 重组错: %q", e.Body)
	}
	if e.Status != 200 || e.RespBody != `{"resp":"hello"}` {
		t.Errorf("响应(completion)重组错: status=%d body=%q", e.Status, e.RespBody)
	}
}

// TestReassemblyRetransDedup：重传的请求段不应导致 body 重复。
func TestReassemblyRetransDedup(t *testing.T) {
	var got []shared.ContentAuditEvent
	ras := newReassembler(SNIConfig{ContentAuditPorts: []int{80}, ContentAuditMaxBody: 65536},
		func(e shared.ContentAuditEvent) { got = append(got, e) })
	c, s := "1.1.1.1", "2.2.2.2"
	req := "POST /x HTTP/1.1\r\nHost: h\r\nContent-Length: 5\r\n\r\nhello"
	seg := caSeg(c, 5000, s, 80, 100, req, false)
	ras.feed(seg)
	ras.feed(seg) // 重传同一段
	// 响应触发发射
	ras.feed(caSeg(s, 80, c, 5000, 900, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok", false))
	if len(got) != 1 || got[0].Body != "hello" {
		t.Fatalf("重传去重失败: events=%d body=%q", len(got), func() string {
			if len(got) > 0 {
				return got[0].Body
			}
			return ""
		}())
	}
}

func TestReassemblyOutOfOrderSegments(t *testing.T) {
	for _, order := range []string{"later-first", "forward-gap"} {
		t.Run(order, func(t *testing.T) {
			var got []shared.ContentAuditEvent
			ras := newReassembler(SNIConfig{ContentAuditPorts: []int{8000}, ContentAuditMaxBody: 65536},
				func(e shared.ContentAuditEvent) { got = append(got, e) })
			c, s := "10.1.0.1", "10.1.0.2"
			full := "POST /v1/responses HTTP/1.1\r\nHost: llm\r\nContent-Length: 11\r\n\r\nhello world"
			p1, p2, p3 := full[:35], full[35:65], full[65:]
			base := uint32(1000)
			switch order {
			case "later-first":
				ras.feed(caSeg(c, 40000, s, 8000, base+uint32(len(p1)), p2+p3, false))
				ras.feed(caSeg(c, 40000, s, 8000, base, p1, false))
			case "forward-gap":
				ras.feed(caSeg(c, 40000, s, 8000, base, p1, false))
				ras.feed(caSeg(c, 40000, s, 8000, base+uint32(len(p1)+len(p2)), p3, false))
				ras.feed(caSeg(c, 40000, s, 8000, base+uint32(len(p1)), p2, false))
			}
			ras.feed(caSeg(s, 8000, c, 40000, 9000, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok", false))
			if len(got) != 1 || got[0].Body != "hello world" {
				t.Fatalf("out-of-order reassembly failed: %+v", got)
			}
		})
	}
}

func TestReassemblyKeepsDirectionWhenBothEndpointPortsAreAudited(t *testing.T) {
	var got []shared.ContentAuditEvent
	ras := newReassembler(SNIConfig{ContentAuditPorts: []int{8000, 9000}, ContentAuditMaxBody: 65536},
		func(e shared.ContentAuditEvent) { got = append(got, e) })
	c, s := "10.2.0.1", "10.2.0.2"
	req := "POST /v1/responses HTTP/1.1\r\nHost: llm\r\nContent-Length: 2\r\n\r\n{}"
	ras.feed(caSeg(c, 9000, s, 8000, 100, req, false))
	resp1 := "HTTP/1.1 200 OK\r\nContent-Length: 11\r\n\r\nhello "
	resp2 := "world"
	ras.feed(caSeg(s, 8000, c, 9000, 900, resp1, false))
	ras.feed(caSeg(s, 8000, c, 9000, 900+uint32(len(resp1)), resp2, false))
	if len(got) != 1 || got[0].RespBody != "hello world" {
		t.Fatalf("response continuation was assigned to wrong direction: %+v", got)
	}
}

func TestReassemblySkipsHTTP100Continue(t *testing.T) {
	var got []shared.ContentAuditEvent
	ras := newReassembler(SNIConfig{ContentAuditPorts: []int{8000}, ContentAuditMaxBody: 65536},
		func(e shared.ContentAuditEvent) { got = append(got, e) })
	c, s := "10.3.0.1", "10.3.0.2"
	req := "POST /v1/responses HTTP/1.1\r\nHost: llm\r\nContent-Length: 2\r\n\r\n{}"
	ras.feed(caSeg(c, 50000, s, 8000, 100, req, false))
	resp := "HTTP/1.1 100 Continue\r\n\r\n" +
		"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"
	ras.feed(caSeg(s, 8000, c, 50000, 900, resp, false))
	if len(got) != 1 || got[0].Status != 200 || got[0].RespBody != "ok" {
		t.Fatalf("interim response handling failed: %+v", got)
	}
}

// TestReassemblyChunkedSSE：chunked 响应去块 + SSE [DONE] 结束。
func TestReassemblyChunkedSSE(t *testing.T) {
	var got []shared.ContentAuditEvent
	ras := newReassembler(SNIConfig{ContentAuditPorts: []int{8000}, ContentAuditMaxBody: 65536},
		func(e shared.ContentAuditEvent) { got = append(got, e) })
	c, s := "10.0.0.1", "10.0.0.2"
	ras.feed(caSeg(c, 33333, s, 8000, 1, "POST /v1/chat/completions HTTP/1.1\r\nHost: llm\r\nContent-Length: 2\r\n\r\nhi", false))
	// chunked 响应："hello"+" world" 两块 + 结束块
	resp := "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\n" +
		"5\r\nhello\r\n6\r\n world\r\n0\r\n\r\n"
	ras.feed(caSeg(s, 8000, c, 33333, 1, resp, false))
	if len(got) != 1 {
		t.Fatalf("chunked 应发射 1 条, 得 %d", len(got))
	}
	if got[0].RespBody != "hello world" {
		t.Errorf("chunked 去块错: %q", got[0].RespBody)
	}
}

func TestReassemblyChunkedRequest(t *testing.T) {
	var got []shared.ContentAuditEvent
	ras := newReassembler(SNIConfig{ContentAuditPorts: []int{8000}, ContentAuditMaxBody: 65536},
		func(e shared.ContentAuditEvent) { got = append(got, e) })
	c, s := "10.0.0.1", "10.0.0.2"
	req := "POST /v1/responses HTTP/1.1\r\nHost: llm\r\nTransfer-Encoding: chunked\r\n\r\n" +
		"5\r\nhello\r\n6\r\n world\r\n0\r\n\r\n"
	ras.feed(caSeg(c, 33333, s, 8000, 1, req, false))
	ras.feed(caSeg(s, 8000, c, 33333, 1, "HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n", false))
	if len(got) != 1 || got[0].Body != "hello world" {
		t.Fatalf("chunked request decode failed: %+v", got)
	}
}

func TestDechunk(t *testing.T) {
	out, done := dechunk([]byte("4\r\nWiki\r\n5\r\npedia\r\n0\r\n\r\n"))
	if !done || string(out) != "Wikipedia" {
		t.Errorf("dechunk 错: %q done=%v", out, done)
	}
	// 未收到结束块 → done=false
	_, done2 := dechunk([]byte("4\r\nWiki\r\n"))
	if done2 {
		t.Error("未见结束块不应 done")
	}
}
