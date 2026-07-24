package main

// SNI + DNS 抓取器（平台无关部分：配置、聚合、flush）。Linux 默认使用 AF_PACKET
// 原生后端；Windows/macOS 使用外部 TShark 后端（Npcap/libpcap/BPF），避免把 CGO
// 和平台驱动耦合进 Agent。只读可见的 DNS/SNI 与明文 HTTP，不解密 TLS。

import (
	"log/slog"
	"sync"
	"time"

	"aiops-monitor/shared"
)

// SNIConfig 是 SNI/DNS 抓取 + 内容审计配置。
type SNIConfig struct {
	Enabled          bool   `json:"enabled"`
	Interface        string `json:"interface,omitempty"`           // TShark 可用接口名/编号；Linux native 空=所有网卡
	CaptureBackend   string `json:"capture_backend,omitempty"`     // auto | native | tshark
	TSharkPath       string `json:"tshark_path,omitempty"`         // 可选，留空自动发现
	MaxEntriesPerMin int    `json:"max_entries_per_min,omitempty"` // DNS/SNI 一分钟去重上限（默认 5000）
	TLSMetadataPorts []int  `json:"tls_metadata_ports,omitempty"`  // TLS ClientHello 元数据端口（默认 443/8443/9443）
	// ---- 明文 HTTP 内容审计（高敏感，默认关闭；需授权 + 履行告知义务）----
	ContentAudit                bool     `json:"content_audit,omitempty"`
	ContentAuditPorts           []int    `json:"content_audit_ports,omitempty"`              // 明文 HTTP 服务端口白名单
	ContentAuditMaxBody         int      `json:"content_audit_max_body,omitempty"`           // 请求正文截断上限
	ContentAuditBodyMode        string   `json:"content_audit_body_mode,omitempty"`          // metadata | redacted | full
	ContentAuditIncludeHosts    []string `json:"content_audit_include_hosts,omitempty"`      // 可选域名 allowlist，支持 *.example.com
	ContentAuditExcludeHosts    []string `json:"content_audit_exclude_hosts,omitempty"`      // 域名 denylist
	ContentAuditExcludePaths    []string `json:"content_audit_exclude_paths,omitempty"`      // 路径 denylist，支持 /health*
	ContentAuditRedactKeys      []string `json:"content_audit_redact_keys,omitempty"`        // 额外 JSON 敏感字段
	ContentAuditMaxEventsPerMin int      `json:"content_audit_max_events_per_min,omitempty"` // 每分钟事件硬上限
}

type sniCollector struct {
	cfg                  SNIConfig
	hostID               string
	fp                   string
	mu                   sync.Mutex
	seen                 map[string]shared.DNSMapEntry // "ip|domain" → entry，窗口内去重
	content              []shared.ContentAuditEvent    // 内容审计事件缓冲（窗口内，flush 清空）
	contentWindow        time.Time
	contentWindowN       int
	contentRateDropped   int64
	contentBufferDropped int64
}

func newSNICollector(cfg SNIConfig, hostID, fp string) *sniCollector {
	return &sniCollector{cfg: cfg, hostID: hostID, fp: fp, seen: map[string]shared.DNSMapEntry{}}
}

// snmpContentCap 是内容审计缓冲的硬上限（防慢 server 时无界增长；一条含 body 可能几 KB）。
const contentAuditCap = 2000

// addContent 在 Agent 端先执行 allow/deny、正文最小化与脱敏，再进入有界缓冲。
// 原始敏感正文不会在 metadata/redacted 模式下离开端点。
func (sc *sniCollector) addContent(ev shared.ContentAuditEvent) {
	if !applyContentAuditPolicy(sc.cfg, &ev) {
		return
	}
	sc.mu.Lock()
	now := time.Now()
	if sc.contentWindow.IsZero() || now.Sub(sc.contentWindow) >= time.Minute {
		sc.contentWindow = now
		sc.contentWindowN = 0
	}
	limit := sc.cfg.ContentAuditMaxEventsPerMin
	if limit <= 0 {
		limit = 2000
	}
	if sc.contentWindowN >= limit {
		sc.contentRateDropped++
		sc.mu.Unlock()
		return
	}
	sc.contentWindowN++
	if len(sc.content) < contentAuditCap {
		sc.content = append(sc.content, ev)
	} else {
		sc.contentBufferDropped++
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
	const (
		maxBatchEvents = 128
		maxBatchBytes  = 8 << 20
	)
	batches := 0
	for start := 0; start < len(evs); {
		end, size := start, 0
		for end < len(evs) && end-start < maxBatchEvents {
			evSize := len(evs[end].Body) + len(evs[end].RespBody) + len(evs[end].Path) + 1024
			if end > start && size+evSize > maxBatchBytes {
				break
			}
			size += evSize
			end++
		}
		reporter(shared.ContentAuditReport{
			HostID:      sc.hostID,
			Fingerprint: sc.fp,
			Timestamp:   time.Now().Unix(),
			Events:      evs[start:end],
		})
		batches++
		start = end
	}
	slog.Info("内容审计上报", "count", len(evs), "batches", batches)
}

func (sc *sniCollector) logContentDrops() {
	sc.mu.Lock()
	rateDropped, bufferDropped := sc.contentRateDropped, sc.contentBufferDropped
	sc.contentRateDropped, sc.contentBufferDropped = 0, 0
	sc.mu.Unlock()
	if rateDropped > 0 || bufferDropped > 0 {
		slog.Warn("内容审计事件被限流或缓冲丢弃",
			"rate_limited", rateDropped, "buffer_full", bufferDropped)
	}
}

// handleL4 处理一个已解析的四层信息：DNS 应答(UDP:53) → A 记录；TLS ClientHello(TCP) → SNI。
// 明文 HTTP 内容审计不在此处——由每 worker 独占的 reassembler(TCP 流重组)负责，见 collector_sni_linux.go。
func (sc *sniCollector) handleL4(info l4Info) {
	if len(info.payload) == 0 {
		return
	}
	// DNS 应答：UDP 源端口 53。
	if info.proto == 17 && info.srcPort == 53 {
		for _, d := range parseDNSResponse(info.payload) {
			sc.add(d)
		}
		return
	}
	// TLS ClientHello：TCP，载荷以 0x16(handshake) 开头，取 SNI，映射到目的 IP。
	if info.proto == 6 && len(info.payload) > 5 && info.payload[0] == 0x16 {
		if sni := parseTLSClientHelloSNI(info.payload); sni != "" {
			sc.observeSNI(info, sni)
		}
	}
}

func (sc *sniCollector) observeSNI(info l4Info, sni string) {
	sc.add(ipDomain{ip: info.dstIP, domain: sni, source: "sni"})
	if !sc.cfg.ContentAudit {
		return
	}
	sc.addContent(shared.ContentAuditEvent{
		SrcIP: info.srcIP, DstIP: info.dstIP, DstPort: info.dstPort,
		Protocol: "tls", Host: sni, CType: "application/tls-clienthello",
		Ts: time.Now().Unix(),
	})
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
