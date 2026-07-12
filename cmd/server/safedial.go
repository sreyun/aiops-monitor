package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// ============================================================================
// SSRF 出站防护（仅用于「用户可影响 URL」的出站：AI Endpoint + 通知 Webhook）
//
// 关键设计约束：本工具本就设计为探测/对接内网服务（自定义拨测打 127.0.0.1、
// 监控内网 Redis/MySQL、对接自建 LLM 网关或内网告警 Webhook），因此**不能**对
// 出站做"一刀切封内网"。
//   · 默认（零误伤）：始终拒绝 **云元数据地址 + 链路本地(169.254/16, fe80::/10)**——
//     这类地址永远不是合法业务目标，却是 SSRF 窃取云 IAM 凭据的头号目标。
//   · 严格模式（AIOPS_SSRF_STRICT=true，可选）：额外拒绝 **环回 + RFC1918 私网 + ULA**，
//     适合明确不需要对接任何内网服务的强隔离部署。
//
// 拦截点用 net.Dialer.Control：在 DNS 解析之后、真正 connect 之前对**实际 IP**
// 校验，天然覆盖 30x 重定向与 DNS rebinding（每次真实连接都会过一遍）。
// ============================================================================

// cloudMetadataIPs 是各云厂商的实例元数据端点（拿到即等于拿到实例 IAM 凭据）。
var cloudMetadataIPs = []net.IP{
	net.ParseIP("169.254.169.254"), // AWS / GCP / Azure / OpenStack / 华为云
	net.ParseIP("169.254.170.2"),   // AWS ECS 任务元数据
	net.ParseIP("100.100.100.200"), // 阿里云
	net.ParseIP("fd00:ec2::254"),   // AWS IPv6 元数据
}

// ssrfStrict 缓存严格模式开关（启动时从环境读取一次；atomic 便于测试覆盖）。
var ssrfStrict atomic.Bool

func init() {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("AIOPS_SSRF_STRICT")))
	ssrfStrict.Store(v == "true" || v == "1" || v == "yes" || v == "on")
}

// ssrfBlockedIP 判断一个已解析的目标 IP 是否应被拒绝。strict 时额外拒绝环回/私网/ULA。
func ssrfBlockedIP(ip net.IP, strict bool) (bool, string) {
	if ip == nil {
		return true, "无法解析目标 IP"
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true, "链路本地地址（含云元数据 169.254.169.254）"
	}
	for _, m := range cloudMetadataIPs {
		if ip.Equal(m) {
			return true, "云实例元数据地址"
		}
	}
	if ip.IsUnspecified() || ip.IsMulticast() {
		return true, "未指定/组播地址"
	}
	if strict && (ip.IsLoopback() || ip.IsPrivate()) {
		return true, "环回/内网私有地址（严格模式）"
	}
	return false, ""
}

// ssrfDialControl 是 net.Dialer.Control 钩子：连接前对实际 IP 做 SSRF 校验。
func ssrfDialControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip := net.ParseIP(host)
	if blocked, why := ssrfBlockedIP(ip, ssrfStrict.Load()); blocked {
		return fmt.Errorf("SSRF 保护：拒绝连接 %s（%s）", host, why)
	}
	return nil
}

// newGuardedHTTPClient 返回一个带 SSRF 出站校验的 http.Client，用于 AI/Webhook 等
// 用户可影响 URL 的出站请求。DialContext 的 Control 会在每次真实连接（含重定向）前校验 IP。
func newGuardedHTTPClient(timeout time.Duration) *http.Client {
	d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second, Control: ssrfDialControl}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:           d.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}
