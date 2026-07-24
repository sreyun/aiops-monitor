package main

// Phase 2 · 增量 2：TCP 流重组，拿完整多包请求 body + 响应(completion)全文。
// 被动抓包重组，尽力而为（非 RFC 严格）：按到达顺序 + TCP seq 去重重传/重叠；连接亲和（同连接
// 的包由同一 worker 处理，故本结构【单协程访问、无锁】）。内存严格封顶：每连接 req/resp 各有上限，
// 连接数上限 + 空闲驱逐。响应体按 Content-Length / chunked / SSE(text/event-stream) / 连接关闭 收全。

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
	"strconv"
	"strings"
	"time"

	"aiops-monitor/shared"
)

// ---- seq 环回安全比较（seq 在 2^32 处回绕）----
func seqLT(a, b uint32) bool { return int32(a-b) < 0 }
func seqLE(a, b uint32) bool { return int32(a-b) <= 0 }

// connKey 是一条 TCP 连接的规范键（无向：两端排序，双向映射到同一连接）。
type connKey struct {
	ipA, ipB     string
	portA, portB uint16
}

func makeConnKey(sIP string, sPort uint16, dIP string, dPort uint16) connKey {
	if sIP < dIP || (sIP == dIP && sPort <= dPort) {
		return connKey{sIP, dIP, sPort, dPort}
	}
	return connKey{dIP, sIP, dPort, sPort}
}

// dirStream 是连接一个方向的有界重组缓冲。前向乱序段先进入 pending，只有填平
// sequence gap 后才追加；首个载荷段之后到达的紧邻前段可安全 prepend。
type dirStream struct {
	haveBase   bool
	startSeq   uint32
	nextSeq    uint32
	buf        []byte
	pending    map[uint32][]byte
	pendingLen int
	truncated  bool
}

// appendSeg 去重/处理重叠并按序追加。内存总量（连续缓冲+乱序等待）不超过 cap。
func (d *dirStream) appendSeg(seq uint32, payload []byte, capacity int) {
	if len(payload) == 0 {
		return
	}
	end := seq + uint32(len(payload))
	if !d.haveBase {
		d.haveBase = true
		d.startSeq = seq
		d.nextSeq = seq
	}
	// 首次看到的 payload 可能不是该方向的首段；紧邻或重叠的更早段可 prepend。
	if seqLT(seq, d.startSeq) {
		if seqLT(end, d.startSeq) {
			d.truncated = true // 前方仍有 gap，不能伪造连续字节流
			return
		}
		prefixLen := int(d.startSeq - seq)
		if prefixLen > len(payload) {
			prefixLen = len(payload)
		}
		if n := d.prepend(payload[:prefixLen], capacity); n > 0 {
			d.startSeq -= uint32(n)
		}
		if seqLT(d.nextSeq, end) {
			skip := int(d.nextSeq - seq)
			d.appendContiguous(payload[skip:], capacity)
			d.drainPending(capacity)
		}
		return
	}

	if seqLT(seq, d.nextSeq) {
		if seqLE(end, d.nextSeq) {
			return // 完全重传
		}
		skip := d.nextSeq - seq
		if int(skip) < len(payload) {
			payload = payload[skip:]
		} else {
			return
		}
		seq = d.nextSeq
	}

	if seq != d.nextSeq {
		d.storePending(seq, payload, capacity)
		return
	}
	d.appendContiguous(payload, capacity)
	d.drainPending(capacity)
}

func (d *dirStream) appendContiguous(payload []byte, capacity int) {
	originalLen := len(payload)
	room := capacity - len(d.buf) - d.pendingLen
	if room < len(payload) {
		if room < 0 {
			room = 0
		}
		payload = payload[:room]
		d.truncated = true
	}
	d.buf = append(d.buf, payload...)
	// Advance over the observed segment even when storage is truncated; otherwise
	// every following packet would be retained as an artificial gap.
	d.nextSeq += uint32(originalLen)
}

func (d *dirStream) prepend(payload []byte, capacity int) int {
	if len(payload) == 0 {
		return 0
	}
	room := capacity - len(d.buf) - d.pendingLen
	if room < len(payload) {
		payload = payload[len(payload)-maxInt(room, 0):]
		d.truncated = true
	}
	if len(payload) == 0 {
		return 0
	}
	next := make([]byte, 0, len(payload)+len(d.buf))
	next = append(next, payload...)
	next = append(next, d.buf...)
	d.buf = next
	return len(payload)
}

func (d *dirStream) storePending(seq uint32, payload []byte, capacity int) {
	if d.pending == nil {
		d.pending = map[uint32][]byte{}
	}
	if existing, ok := d.pending[seq]; ok && len(existing) >= len(payload) {
		return
	}
	if old := d.pending[seq]; old != nil {
		d.pendingLen -= len(old)
		delete(d.pending, seq)
	}
	room := capacity - len(d.buf) - d.pendingLen
	if room <= 0 {
		d.truncated = true
		return
	}
	copyLen := len(payload)
	if copyLen > room {
		copyLen = room
		d.truncated = true
	}
	d.pending[seq] = append([]byte(nil), payload[:copyLen]...)
	d.pendingLen += copyLen
}

func (d *dirStream) drainPending(capacity int) {
	for {
		progressed := false
		for seq, payload := range d.pending {
			end := seq + uint32(len(payload))
			if seqLE(end, d.nextSeq) {
				delete(d.pending, seq)
				d.pendingLen -= len(payload)
				progressed = true
				break
			}
			if seqLT(d.nextSeq, seq) {
				continue
			}
			delete(d.pending, seq)
			d.pendingLen -= len(payload)
			skip := int(d.nextSeq - seq)
			d.appendContiguous(payload[skip:], capacity)
			progressed = true
			break
		}
		if !progressed {
			return
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// httpStream 是一条连接的 HTTP 请求/响应重组状态。
type httpStream struct {
	req, resp      dirStream
	srcIP          string // 客户端
	srcPort        uint16
	dstIP          string
	dstPort        uint16
	endpointsKnown bool
	lastSeen       int64
}

// reassembler 是一个 worker 独占的重组器（无锁）。
type reassembler struct {
	conns   map[connKey]*httpStream
	ports   []int
	reqCap  int
	respCap int
	maxConn int
	emit    func(shared.ContentAuditEvent)
}

func newReassembler(cfg SNIConfig, emit func(shared.ContentAuditEvent)) *reassembler {
	reqCap := cfg.ContentAuditMaxBody
	if reqCap <= 0 {
		reqCap = 4096
	}
	// 响应(completion)通常远大于请求，给更大上限（默认 256KB，随 req 上限放大）。
	respCap := reqCap * 64
	if respCap < 65536 {
		respCap = 65536
	}
	if respCap > 1<<20 {
		respCap = 1 << 20
	}
	return &reassembler{
		conns:   map[connKey]*httpStream{},
		ports:   cfg.ContentAuditPorts,
		reqCap:  reqCap,
		respCap: respCap,
		maxConn: 8192,
		emit:    emit,
	}
}

// feed 喂一个 TCP 段进重组器。dstPort∈审计端口=请求方向；srcPort∈审计端口=响应方向。
func (ra *reassembler) feed(info l4Info) {
	reqDir := contentPortMatch(ra.ports, info.dstPort)
	respDir := contentPortMatch(ra.ports, info.srcPort)
	if !reqDir && !respDir {
		return
	}
	key := makeConnKey(info.srcIP, info.srcPort, info.dstIP, info.dstPort)
	s := ra.conns[key]
	if s != nil && s.endpointsKnown {
		switch {
		case info.srcIP == s.srcIP && info.srcPort == s.srcPort:
			reqDir, respDir = true, false
		case info.srcIP == s.dstIP && info.srcPort == s.dstPort:
			reqDir, respDir = false, true
		}
	}
	// 端口白名单为空时两个方向都可能 true：以"载荷像 HTTP 请求"或"像响应"再分一次（尽力）。
	if reqDir && respDir {
		if bytes.HasPrefix(info.payload, []byte("HTTP/")) {
			reqDir = false
		} else if looksLikeHTTPRequest(info.payload) {
			respDir = false
		} else {
			return // 尚无方向证据，避免把响应续包拼进请求
		}
	}
	if s == nil {
		if len(ra.conns) >= ra.maxConn {
			ra.evictOldest()
		}
		s = &httpStream{}
		ra.conns[key] = s
	}
	s.lastSeen = time.Now().Unix()
	if reqDir {
		s.srcIP, s.srcPort, s.dstIP, s.dstPort, s.endpointsKnown =
			info.srcIP, info.srcPort, info.dstIP, info.dstPort, true
		s.req.appendSeg(info.seq, info.payload, ra.reqCap)
	} else {
		if !s.endpointsKnown {
			s.srcIP, s.srcPort, s.dstIP, s.dstPort, s.endpointsKnown =
				info.dstIP, info.dstPort, info.srcIP, info.srcPort, true
		}
		s.resp.appendSeg(info.seq, info.payload, ra.respCap)
	}

	// 连接结束(FIN/RST)：发射已有内容并回收。
	fin := info.tcpFlags&0x01 != 0 || info.tcpFlags&0x04 != 0
	if ra.tryEmit(key, s, fin) {
		return
	}
}

// tryEmit 判断请求+响应是否都收全（或被 fin 强制），是则发射并回收连接。
func (ra *reassembler) tryEmit(key connKey, s *httpStream, fin bool) bool {
	rh, reqOK := parseReqComplete(s.req.buf)
	if !reqOK && !fin {
		return false // 请求头都没齐，等
	}
	sh, respDone := parseRespComplete(s.resp.buf)
	if !respDone && !fin {
		return false // 响应还没收全，等（除非连接已结束）
	}
	// 到这：请求 OK 且（响应收全 或 fin）。构造事件。
	ev := shared.ContentAuditEvent{
		SrcIP: s.srcIP, DstIP: s.dstIP, DstPort: s.dstPort,
		Protocol: "http",
		Method:   rh.method, Host: rh.host, Path: rh.path, CType: rh.ctype,
		Body:          rh.body,
		Status:        sh.status,
		RespCType:     sh.ctype,
		RespBody:      sh.body,
		ReqTruncated:  s.req.truncated,
		RespTruncated: s.resp.truncated,
		Ts:            time.Now().Unix(),
	}
	if ev.Method == "" && ev.Status == 0 {
		delete(ra.conns, key)
		return true // 无有效内容，丢弃
	}
	ra.emit(ev)
	delete(ra.conns, key)
	return true
}

// sweepIdle 驱逐空闲连接（发射已有内容）。由 worker 周期调用。
func (ra *reassembler) sweepIdle(idleSec int64) {
	now := time.Now().Unix()
	for k, s := range ra.conns {
		if now-s.lastSeen > idleSec {
			ra.tryEmit(k, s, true) // 强制发射
			delete(ra.conns, k)    // tryEmit 里已删；双保险
		}
	}
}

func (ra *reassembler) evictOldest() {
	var oldK connKey
	var oldT int64 = 1<<62 - 1
	for k, s := range ra.conns {
		if s.lastSeen < oldT {
			oldT, oldK = s.lastSeen, k
		}
	}
	delete(ra.conns, oldK)
}

// ---- HTTP 请求/响应分帧 ----

type reqHead struct {
	method, path, host, ctype, body string
}
type respHead struct {
	status      int
	ctype, body string
}

var crlfcrlf = []byte("\r\n\r\n")

// splitHeadBody 找 \r\n\r\n，返回头(不含分隔)、体、是否找到头尾。
func splitHeadBody(buf []byte) (head, body []byte, ok bool) {
	i := bytes.Index(buf, crlfcrlf)
	if i < 0 {
		return nil, nil, false
	}
	return buf[:i], buf[i+4:], true
}

func headerVal(lines []string, name string) string {
	pfx := strings.ToLower(name) + ":"
	for _, ln := range lines {
		if strings.HasPrefix(strings.ToLower(ln), pfx) {
			return strings.TrimSpace(ln[len(pfx):])
		}
	}
	return ""
}

// parseReqComplete 解析请求；仅当 body 按 Content-Length 收全（或无 body / 已达 cap 截断）才 ok。
func parseReqComplete(buf []byte) (reqHead, bool) {
	if !looksLikeHTTPRequest(buf) {
		return reqHead{}, false
	}
	head, body, ok := splitHeadBody(buf)
	if !ok {
		return reqHead{}, false // 头没齐
	}
	lines := strings.Split(string(head), "\r\n")
	rl := strings.Fields(lines[0])
	if len(rl) < 2 {
		return reqHead{}, false
	}
	h := reqHead{method: rl[0], path: rl[1], host: headerVal(lines[1:], "Host"), ctype: headerVal(lines[1:], "Content-Type")}
	ce := headerVal(lines[1:], "Content-Encoding")
	te := strings.ToLower(headerVal(lines[1:], "Transfer-Encoding"))
	if strings.Contains(te, "chunked") {
		dec, done := dechunk(body)
		if done {
			dec = decodeContentEncoding(dec, ce)
		}
		h.body = string(cleanUTF8(dec))
		return h, done
	}
	cl, hasCL := parseContentLength(lines[1:])
	if !hasCL {
		h.body = string(cleanUTF8(body)) // 无 CL（如 GET）：现有 body 即全部
		return h, true
	}
	if len(body) < cl {
		return reqHead{}, false // body 未收全
	}
	h.body = string(cleanUTF8(decodeContentEncoding(body[:cl], ce)))
	return h, true
}

// parseRespComplete 解析响应；按 Content-Length / chunked / SSE / 连接关闭 判定收全。
func parseRespComplete(buf []byte) (respHead, bool) {
	if len(buf) < 12 || !bytes.HasPrefix(buf, []byte("HTTP/")) {
		return respHead{}, false
	}
	head, body, ok := splitHeadBody(buf)
	if !ok {
		return respHead{}, false
	}
	lines := strings.Split(string(head), "\r\n")
	sl := strings.Fields(lines[0])
	var h respHead
	if len(sl) >= 2 {
		h.status, _ = strconv.Atoi(sl[1])
	}
	if h.status >= 100 && h.status < 200 && h.status != 101 {
		if len(body) == 0 {
			return respHead{}, false
		}
		return parseRespComplete(body) // skip 100 Continue / 103 Early Hints
	}
	h.ctype = headerVal(lines[1:], "Content-Type")
	if h.status == 101 || h.status == 204 || h.status == 304 {
		return h, true // RFC-defined no-body responses
	}
	te := strings.ToLower(headerVal(lines[1:], "Transfer-Encoding"))
	ce := headerVal(lines[1:], "Content-Encoding") // gzip/deflate：收全后需先解压再清洗

	if strings.Contains(te, "chunked") {
		dec, done := dechunk(body)
		if done { // 只有整块收全才解压（gzip 半包解不出）
			dec = decodeContentEncoding(dec, ce)
		}
		h.body = string(cleanUTF8(dec))
		return h, done // chunked：收到 0\r\n\r\n 才算全
	}
	if cl, hasCL := parseContentLength(lines[1:]); hasCL {
		if len(body) < cl {
			return respHead{}, false
		}
		h.body = string(cleanUTF8(decodeContentEncoding(body[:cl], ce)))
		return h, true
	}
	// SSE / 无长度：body 即已收部分；是否"收全"由调用方的 fin/idle 决定（这里返回 done=false，
	// 让 tryEmit 在 fin/idle 时才落地）。但 SSE 见到 [DONE] 视为结束。
	h.body = string(cleanUTF8(decodeContentEncoding(body, ce)))
	if strings.Contains(strings.ToLower(h.ctype), "event-stream") && strings.Contains(string(body), "[DONE]") {
		return h, true
	}
	return h, false
}

func parseContentLength(lines []string) (int, bool) {
	v := headerVal(lines, "Content-Length")
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// dechunk 解 HTTP chunked 传输编码。返回解出的 body 与是否见到结束块(0\r\n\r\n)。
func dechunk(b []byte) ([]byte, bool) {
	var out []byte
	for {
		i := bytes.Index(b, []byte("\r\n"))
		if i < 0 {
			return out, false
		}
		sizeLine := string(b[:i])
		if semi := strings.IndexByte(sizeLine, ';'); semi >= 0 { // chunk extension
			sizeLine = sizeLine[:semi]
		}
		sz, err := strconv.ParseInt(strings.TrimSpace(sizeLine), 16, 64)
		if err != nil {
			return out, false
		}
		b = b[i+2:]
		if sz == 0 {
			return out, true // 结束块
		}
		if int64(len(b)) < sz {
			out = append(out, b...) // 未收全，先给已有
			return out, false
		}
		out = append(out, b[:sz]...)
		b = b[sz:]
		if len(b) >= 2 && b[0] == '\r' && b[1] == '\n' {
			b = b[2:]
		}
	}
}

// cleanUTF8 去除无效 UTF-8（body 可能非文本），保证 JSON 上报安全。
func cleanUTF8(b []byte) []byte {
	return []byte(strings.ToValidUTF8(string(b), ""))
}

const maxDecodedBody = 2 << 20 // 解压上限 2MB，防解压炸弹

// decodeContentEncoding 按 Content-Encoding 解压 gzip / deflate（仅标准库，零依赖）。
// 这是内容审计"看不到明文"的主因修复：绝大多数 HTTP API 响应是 gzip 压缩的，之前只解 chunked、
// 不解 Content-Encoding，压缩字节被 cleanUTF8 直接抹成空，导致响应体全空。br 等未知编码或解压
// 失败则原样返回（交给 cleanUTF8 兜底）。
func decodeContentEncoding(b []byte, enc string) []byte {
	enc = strings.ToLower(strings.TrimSpace(enc))
	if enc == "" || len(b) == 0 {
		return b
	}
	switch {
	case strings.Contains(enc, "gzip"):
		if r, err := gzip.NewReader(bytes.NewReader(b)); err == nil {
			defer r.Close()
			if out, err := io.ReadAll(io.LimitReader(r, maxDecodedBody)); err == nil && len(out) > 0 {
				return out
			}
		}
	case strings.Contains(enc, "deflate"):
		if r, err := zlib.NewReader(bytes.NewReader(b)); err == nil { // deflate 常为 zlib 包裹
			defer r.Close()
			if out, err := io.ReadAll(io.LimitReader(r, maxDecodedBody)); err == nil && len(out) > 0 {
				return out
			}
		}
		fr := flate.NewReader(bytes.NewReader(b)) // 再试裸 flate
		defer fr.Close()
		if out, err := io.ReadAll(io.LimitReader(fr, maxDecodedBody)); err == nil && len(out) > 0 {
			return out
		}
	}
	return b
}
