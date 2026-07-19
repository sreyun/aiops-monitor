package main

// SNI + DNS 抓取器（平台无关部分：配置、聚合、flush；原始套接字读取按平台拆到
// collector_sni_linux.go / collector_sni_other.go）。目的：把"目的 IP ↔ 真实域名"
// 抓出来上报，让流量页能显示用户实际访问的域名（如 api.openai.com / 内网 ollama）。
// 只读明文的 SNI 与 DNS，不解密任何内容。默认关闭，需显式开启且需 root/CAP_NET_RAW。

import (
	"log/slog"
	"sync"
	"time"

	"aiops-monitor/shared"
)

// SNIConfig 是 SNI/DNS 抓取 + 内容审计配置。
type SNIConfig struct {
	Enabled          bool   `json:"enabled"`
	Interface        string `json:"interface,omitempty"`           // 空=所有网卡
	MaxEntriesPerMin int    `json:"max_entries_per_min,omitempty"` // 一轮观测上限（默认 5000）
	// ---- Phase 2 · 明文 HTTP 内容审计（高敏感，默认关闭；需授权 + 履行告知义务）----
	ContentAudit        bool  `json:"content_audit,omitempty"`          // 开启明文 HTTP 请求内容审计
	ContentAuditPorts   []int `json:"content_audit_ports,omitempty"`    // 目的端口白名单（空=对所有 TCP 试探 HTTP）
	ContentAuditMaxBody int   `json:"content_audit_max_body,omitempty"` // body 截断上限(字节，默认 4096)
}

type sniCollector struct {
	cfg     SNIConfig
	hostID  string
	fp      string
	mu      sync.Mutex
	seen    map[string]shared.DNSMapEntry // "ip|domain" → entry，窗口内去重
	content []shared.ContentAuditEvent    // 内容审计事件缓冲（窗口内，flush 清空）
}

func newSNICollector(cfg SNIConfig, hostID, fp string) *sniCollector {
	return &sniCollector{cfg: cfg, hostID: hostID, fp: fp, seen: map[string]shared.DNSMapEntry{}}
}

// snmpContentCap 是内容审计缓冲的硬上限（防慢 server 时无界增长；一条含 body 可能几 KB）。
const contentAuditCap = 2000

// addContent 缓冲一条内容审计事件（受上限保护）。
func (sc *sniCollector) addContent(ev shared.ContentAuditEvent) {
	sc.mu.Lock()
	if len(sc.content) < contentAuditCap {
		sc.content = append(sc.content, ev)
	}
	sc.mu.Unlock()
}

// flushContent 排空内容审计缓冲 → ContentAuditReport 上报。
func (sc *sniCollector) flushContent(reporter func(shared.ContentAuditReport)) {
	sc.mu.Lock()
	if len(sc.content) == 0 {
		sc.mu.Unlock()
		return
	}
	evs := sc.content
	sc.content = nil
	sc.mu.Unlock()
	reporter(shared.ContentAuditReport{
		HostID:      sc.hostID,
		Fingerprint: sc.fp,
		Timestamp:   time.Now().Unix(),
		Events:      evs,
	})
	slog.Info("内容审计上报", "count", len(evs))
}

// handle 处理一个以太网帧：DNS 应答(UDP:53) → A 记录；TLS ClientHello(TCP) → SNI。
func (sc *sniCollector) handle(frame []byte) {
	info, ok := parseEthIPv4(frame)
	if !ok || len(info.payload) == 0 {
		return
	}
	// DNS 应答：UDP 源端口 53。
	if info.proto == 17 && info.srcPort == 53 {
		for _, d := range parseDNSResponse(info.payload) {
			sc.add(d)
		}
		return
	}
	if info.proto == 6 {
		// TLS ClientHello：载荷以 0x16(handshake) 开头，取 SNI，映射到目的 IP。
		if len(info.payload) > 5 && info.payload[0] == 0x16 {
			if sni := parseTLSClientHelloSNI(info.payload); sni != "" {
				sc.add(ipDomain{ip: info.dstIP, domain: sni, source: "sni"})
			}
			return
		}
		// 明文 HTTP 请求 → 内容审计（默认关闭；开启后按端口白名单）。取请求行+Host+body 前缀，
		// 主要用于审计"谁向哪个大模型端点发了什么 prompt"。加密流量到不了这里（那是 0x16 分支）。
		if sc.cfg.ContentAudit && contentPortMatch(sc.cfg.ContentAuditPorts, info.dstPort) {
			if ev, ok := parseHTTPRequest(info.payload, sc.cfg.ContentAuditMaxBody); ok {
				ev.SrcIP = info.srcIP
				ev.DstIP = info.dstIP
				ev.DstPort = info.dstPort
				ev.Ts = time.Now().Unix()
				sc.addContent(ev)
			}
		}
	}
}

func (sc *sniCollector) add(d ipDomain) {
	if d.ip == "" || d.domain == "" {
		return
	}
	cap := sc.cfg.MaxEntriesPerMin
	if cap <= 0 {
		cap = 5000
	}
	key := d.ip + "|" + d.domain
	sc.mu.Lock()
	if _, exists := sc.seen[key]; !exists && len(sc.seen) < cap {
		sc.seen[key] = shared.DNSMapEntry{IP: d.ip, Domain: d.domain, Source: d.source}
	}
	sc.mu.Unlock()
}

// flush 排空观测 → DNSMapReport 上报。
func (sc *sniCollector) flush(reporter func(shared.DNSMapReport)) {
	sc.mu.Lock()
	if len(sc.seen) == 0 {
		sc.mu.Unlock()
		return
	}
	entries := make([]shared.DNSMapEntry, 0, len(sc.seen))
	for _, e := range sc.seen {
		entries = append(entries, e)
	}
	sc.seen = map[string]shared.DNSMapEntry{}
	sc.mu.Unlock()

	reporter(shared.DNSMapReport{
		HostID:      sc.hostID,
		Fingerprint: sc.fp,
		Timestamp:   time.Now().Unix(),
		Entries:     entries,
	})
	slog.Info("SNI/DNS 域名观测上报", "count", len(entries))
}
