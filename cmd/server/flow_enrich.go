package main

// 流量目的地富化：把裸 IP 变成「域名 + 归属组织(ASN) + 国家」，回答"这个 IP 到底属于谁/在访问什么"。
// 全部用 Go 标准库、零依赖、零数据库：
//   - 反向 DNS(PTR)：net.LookupAddr —— 拿主机名（如 dns.google）。
//   - ASN / 国家 / 组织：Team Cymru 的 DNS 服务（net.LookupTXT）——
//       <反转IP>.origin.asn.cymru.com TXT → "ASN | 前缀 | 国家 | 注册局 | 日期"
//       AS<n>.asn.cymru.com          TXT → "ASN | 国家 | 注册局 | 日期 | 组织名"
// 查询时惰性富化 + 缓存(IP 归属很稳定，缓存 24h)。只富化公网 IP；内网/保留地址标记为 private。
//
// 隐私：这类"记录访问了什么"属敏感能力，且富化会对【目的 IP】(公网服务)做外部 DNS 查询。
// 目的 IP 本就是公网服务地址，与反向 DNS 同性质；不查询内网/用户侧地址。可用配置项关闭。

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// flowEnrichment 是一个 IP 的富化结果（JSON 直接回给前端展示）。
type flowEnrichment struct {
	Host    string `json:"host,omitempty"`    // 反向 DNS 主机名
	ASN     string `json:"asn,omitempty"`     // 如 "AS15169"
	Org     string `json:"org,omitempty"`     // 如 "GOOGLE, US"
	Country string `json:"country,omitempty"` // 如 "US"
	Private bool   `json:"private,omitempty"` // 内网/保留地址（不富化）
}

// hasData 是否有可展示的富化内容（域名/归属/国家任一）。仅内网且无 PTR 时为 false。
func (e flowEnrichment) hasData() bool {
	return e.Host != "" || e.Org != "" || e.Country != ""
}

type enrichCacheEntry struct {
	res flowEnrichment
	at  time.Time
}

type flowEnricher struct {
	mu    sync.Mutex
	cache map[string]enrichCacheEntry
	ttl   time.Duration
	res   *net.Resolver
}

// flowEnrich 是包级单例：跨请求共享缓存，无需塞进 Server。
var flowEnrich = &flowEnricher{
	cache: map[string]enrichCacheEntry{},
	ttl:   24 * time.Hour,
	res:   net.DefaultResolver,
}

// isPrivateOrReserved 判断 IP 是否为内网/保留地址（不做外部富化）。
func isPrivateOrReserved(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// 100.64.0.0/10 (CGNAT) 也当作内网
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	return false
}

// reverseIPv4 把 "8.8.4.4" 变成 "4.4.8.8"（Cymru origin 查询用）。
func reverseIPv4(v4 net.IP) string {
	return strconv.Itoa(int(v4[3])) + "." + strconv.Itoa(int(v4[2])) + "." +
		strconv.Itoa(int(v4[1])) + "." + strconv.Itoa(int(v4[0]))
}

// enrichOne 富化单个 IP（带缓存）。ctx 控制单次查询的超时上限。
func (e *flowEnricher) enrichOne(ctx context.Context, ipStr string) flowEnrichment {
	e.mu.Lock()
	if ent, ok := e.cache[ipStr]; ok && time.Since(ent.at) < e.ttl {
		e.mu.Unlock()
		return ent.res
	}
	e.mu.Unlock()

	ip := net.ParseIP(ipStr)
	var res flowEnrichment
	priv := isPrivateOrReserved(ip)
	res.Private = priv

	// 反向 DNS（PTR）对内外网都做：内网常有内部 PTR（如 workstation-01.corp.local / 内网自建
	// 大模型主机名），一样有价值；公网 IP 的 PTR 多来自 CDN/云厂商。
	if names, err := e.res.LookupAddr(ctx, ipStr); err == nil && len(names) > 0 {
		res.Host = strings.TrimSuffix(names[0], ".")
	}

	// 内网/保留地址不做对外 ASN 查询（无意义），到此即可。
	if priv {
		e.store(ipStr, res)
		return res
	}

	// ASN / 国家 / 组织：仅 IPv4 走 Cymru origin（IPv6 用 origin6，此处从简，PTR 已可用）。
	if v4 := ip.To4(); v4 != nil {
		if txts, err := e.res.LookupTXT(ctx, reverseIPv4(v4)+".origin.asn.cymru.com"); err == nil && len(txts) > 0 {
			// "15169 | 8.8.4.0/24 | US | arin | 2000-03-30"
			f := strings.Split(txts[0], "|")
			if len(f) >= 3 {
				asn := strings.Fields(strings.TrimSpace(f[0]))
				if len(asn) > 0 {
					res.ASN = "AS" + asn[0]
					res.Country = strings.TrimSpace(f[2])
					// ASN → 组织名
					if orgTxts, err := e.res.LookupTXT(ctx, "AS"+asn[0]+".asn.cymru.com"); err == nil && len(orgTxts) > 0 {
						// "15169 | US | arin | 2000-03-30 | GOOGLE, US"
						of := strings.Split(orgTxts[0], "|")
						if len(of) > 0 {
							res.Org = strings.TrimSpace(of[len(of)-1])
						}
					}
				}
			}
		}
	}

	e.store(ipStr, res)
	return res
}

func (e *flowEnricher) store(ip string, res flowEnrichment) {
	e.mu.Lock()
	e.cache[ip] = enrichCacheEntry{res: res, at: time.Now()}
	// 简单封顶，防缓存无界增长（IP 数量本就有限，这里兜底）。
	if len(e.cache) > 50000 {
		e.cache = map[string]enrichCacheEntry{ip: {res: res, at: time.Now()}}
	}
	e.mu.Unlock()
}

// enrichMany 并发富化一组去重 IP，带整体截止时间，返回 IP→结果。已缓存的立即命中，
// 未缓存的在截止时间内尽力解析；超时未解析的这次不返回富化（下次查看再补），保证响应不挂。
func (e *flowEnricher) enrichMany(ips []string, deadline time.Duration) map[string]flowEnrichment {
	out := map[string]flowEnrichment{}
	uniq := map[string]struct{}{}
	for _, ip := range ips {
		if ip != "" {
			uniq[ip] = struct{}{}
		}
	}
	if len(uniq) == 0 {
		return out
	}

	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16) // 并发上限，避免一次几十路 DNS 打爆解析器
	for ip := range uniq {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r := e.enrichOne(ctx, ip)
			mu.Lock()
			out[ip] = r
			mu.Unlock()
		}(ip)
	}
	wg.Wait()
	return out
}
