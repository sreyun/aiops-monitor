package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// WebSocket 探测（协议扩展 F）——零依赖手写最小 WS 客户端。
//
// 对 ws:// / wss:// 端点做连通性探测：TCP/TLS 连接 + HTTP Upgrade 握手，校验 101 与
// Sec-WebSocket-Accept；若配置了消息(Body)，再发一帧文本、读一帧（可选关键字断言）。
// 返回与 HTTP 探测一致的 httpProbeResult，复用告警/存储/聚合链路。SSRF：拨号走
// ssrfDialControl，拦本地/元数据地址（端点由运维配置，仍保持与其它出站一致的守卫）。
// ============================================================================

const wsMagicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// probeWebSocket 执行一次 WebSocket 握手探测（可选一来一回消息）。
func probeWebSocket(ep APIEndpoint, commonHeaders map[string]string) httpProbeResult {
	res := httpProbeResult{certDays: -1}
	to := ep.TimeoutSec
	if to <= 0 {
		to = 10
	}
	u, err := url.Parse(strings.TrimSpace(ep.URL))
	if err != nil || (u.Scheme != "ws" && u.Scheme != "wss") {
		res.msg = "WebSocket URL 需以 ws:// 或 wss:// 开头"
		return res
	}
	host := u.Host
	if u.Port() == "" {
		if u.Scheme == "wss" {
			host = u.Hostname() + ":443"
		} else {
			host = u.Hostname() + ":80"
		}
	}
	deadline := time.Now().Add(time.Duration(to) * time.Second)
	start := time.Now()

	d := net.Dialer{Timeout: time.Duration(to) * time.Second, Control: ssrfDialControl}
	rawConn, err := d.Dial("tcp", host)
	if err != nil {
		res.msg = "连接失败：" + err.Error()
		res.totalMs = ms(time.Since(start))
		return res
	}
	defer rawConn.Close()
	res.tcpMs = ms(time.Since(start))

	var conn net.Conn = rawConn
	if u.Scheme == "wss" {
		tlsStart := time.Now()
		tc := tls.Client(rawConn, &tls.Config{ServerName: u.Hostname()})
		_ = tc.SetDeadline(deadline)
		if err := tc.Handshake(); err != nil {
			res.msg = "TLS 握手失败：" + err.Error()
			res.totalMs = ms(time.Since(start))
			return res
		}
		res.tlsMs = ms(time.Since(tlsStart))
		conn = tc
	}
	_ = conn.SetDeadline(deadline)

	// 生成 Sec-WebSocket-Key 并发起 Upgrade 握手
	keyRaw := make([]byte, 16)
	_, _ = rand.Read(keyRaw)
	key := base64.StdEncoding.EncodeToString(keyRaw)
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	var req strings.Builder
	fmt.Fprintf(&req, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n", path, u.Host, key)
	for k, v := range commonHeaders {
		if strings.TrimSpace(k) != "" {
			fmt.Fprintf(&req, "%s: %s\r\n", k, v)
		}
	}
	for k, v := range ep.Headers {
		if strings.TrimSpace(k) != "" {
			fmt.Fprintf(&req, "%s: %s\r\n", k, v)
		}
	}
	req.WriteString("\r\n")
	if _, err := conn.Write([]byte(req.String())); err != nil {
		res.msg = "发送握手失败：" + err.Error()
		res.totalMs = ms(time.Since(start))
		return res
	}

	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		res.msg = "读握手响应失败：" + err.Error()
		res.totalMs = ms(time.Since(start))
		return res
	}
	res.ttfbMs = ms(time.Since(start))
	res.code = parseWSStatus(statusLine)
	if res.code != 101 {
		res.msg = "握手未返回 101：" + strings.TrimSpace(statusLine)
		res.totalMs = ms(time.Since(start))
		return res
	}
	// 读响应头，取 Sec-WebSocket-Accept 并校验
	accept := ""
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if i := strings.Index(line, ":"); i > 0 && strings.EqualFold(strings.TrimSpace(line[:i]), "Sec-WebSocket-Accept") {
			accept = strings.TrimSpace(line[i+1:])
		}
	}
	if accept != wsAcceptKey(key) {
		res.msg = "Sec-WebSocket-Accept 校验失败（可能不是 WebSocket 端点）"
		res.totalMs = ms(time.Since(start))
		return res
	}

	// 可选：发一帧文本 + 读一帧（配置了 Body 才做），支持关键字断言
	if msg := strings.TrimSpace(ep.Body); msg != "" {
		if err := wsWriteText(conn, ep.Body); err != nil {
			res.msg = "发送消息帧失败：" + err.Error()
			res.totalMs = ms(time.Since(start))
			return res
		}
		payload, err := wsReadFrame(br)
		if err != nil {
			res.msg = "读消息帧失败：" + err.Error()
			res.totalMs = ms(time.Since(start))
			return res
		}
		res.bytes = int64(len(payload))
		if ep.ExpectKeyword != "" && !strings.Contains(string(payload), ep.ExpectKeyword) {
			res.msg = "响应帧未包含关键字：" + ep.ExpectKeyword
			res.totalMs = ms(time.Since(start))
			return res
		}
	}
	res.totalMs = ms(time.Since(start))
	res.ok = true
	res.msg = fmt.Sprintf("WebSocket 握手成功 · %.0fms", res.totalMs)
	return res
}

// wsAcceptKey 计算服务端应返回的 Sec-WebSocket-Accept（base64(sha1(key+GUID))）。
func wsAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + wsMagicGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// parseWSStatus 从状态行 "HTTP/1.1 101 Switching Protocols" 取状态码。
func parseWSStatus(line string) int {
	f := strings.Fields(line)
	if len(f) >= 2 {
		if c, err := strconv.Atoi(f[1]); err == nil {
			return c
		}
	}
	return 0
}

// wsWriteText 写一个客户端文本帧（RFC6455：客户端帧必须掩码）。
func wsWriteText(conn net.Conn, text string) error {
	payload := []byte(text)
	n := len(payload)
	header := []byte{0x81} // FIN=1, opcode=0x1(text)
	mask := make([]byte, 4)
	_, _ = rand.Read(mask)
	switch {
	case n < 126:
		header = append(header, byte(0x80|n))
	case n < 65536:
		header = append(header, 0x80|126, byte(n>>8), byte(n))
	default:
		header = append(header, 0x80|127, 0, 0, 0, 0, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	header = append(header, mask...)
	masked := make([]byte, n)
	for i := 0; i < n; i++ {
		masked[i] = payload[i] ^ mask[i%4]
	}
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(masked)
	return err
}

// wsReadFrame 读一个服务端数据帧的 payload（服务端帧不掩码；控制帧也照读，够连通性验证）。
func wsReadFrame(br *bufio.Reader) ([]byte, error) {
	if _, err := br.ReadByte(); err != nil { // b0：FIN + opcode（连通性探测不细分）
		return nil, err
	}
	b1, err := br.ReadByte()
	if err != nil {
		return nil, err
	}
	masked := b1&0x80 != 0
	n := int(b1 & 0x7f)
	if n == 126 {
		var ext [2]byte
		if _, err := io.ReadFull(br, ext[:]); err != nil {
			return nil, err
		}
		n = int(ext[0])<<8 | int(ext[1])
	} else if n == 127 {
		var ext [8]byte
		if _, err := io.ReadFull(br, ext[:]); err != nil {
			return nil, err
		}
		n = int(ext[4])<<24 | int(ext[5])<<16 | int(ext[6])<<8 | int(ext[7])
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(br, mask[:]); err != nil {
			return nil, err
		}
	}
	if n < 0 || n > 1<<20 { // 1MB 上限，防异常长度
		return nil, fmt.Errorf("帧过大")
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(br, payload); err != nil {
		return nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return payload, nil
}
