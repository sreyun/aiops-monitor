package main

// SNI/DNS 域名观测：接收 agent 上报的「目的 IP ↔ 真实域名」（来自明文 SNI 与 DNS 应答），
// 按主机缓存在内存里，供流量富化时把裸 IP 显示成用户实际访问的域名（如 api.openai.com）。
// agent 每 30s 重报，内存丢失可自愈，故不落 PG。带条数上限，防无界增长。

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"aiops-monitor/shared"
)

type dnsObserved struct {
	domain string
	source string // "dns" | "sni"
	at     int64
}

type dnsMapStore struct {
	mu     sync.RWMutex
	byHost map[string]map[string]dnsObserved // hostID → ip → 观测
}

// dnsObservations 是包级单例（与 flowEnrich 同风格，无需塞进 Server）。
var dnsObservations = &dnsMapStore{byHost: map[string]map[string]dnsObserved{}}

const dnsMapMaxPerHost = 20000 // 单主机 IP↔域名 上限

// put 合并一批观测到某主机。SNI 比 DNS 更贴近"用户实际请求的域名"，同 IP 有 SNI 时优先。
func (s *dnsMapStore) put(hostID string, entries []shared.DNSMapEntry) {
	if hostID == "" || len(entries) == 0 {
		return
	}
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.byHost[hostID]
	if m == nil {
		m = map[string]dnsObserved{}
		s.byHost[hostID] = m
	}
	for _, e := range entries {
		if e.IP == "" || e.Domain == "" {
			continue
		}
		cur, exists := m[e.IP]
		// 已有 SNI 观测时不被 DNS 覆盖；否则更新。
		if exists && cur.source == "sni" && e.Source != "sni" {
			continue
		}
		if !exists && len(m) >= dnsMapMaxPerHost {
			continue
		}
		m[e.IP] = dnsObserved{domain: e.Domain, source: e.Source, at: now}
	}
}

// lookup 查某主机某目的 IP 观测到的域名。
func (s *dnsMapStore) lookup(hostID, ip string) (string, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if m := s.byHost[hostID]; m != nil {
		if o, ok := m[ip]; ok {
			return o.domain, o.source, true
		}
	}
	return "", "", false
}

// handleAgentDNSMap 接收 agent 上报的域名观测（指纹校验，与其它 agent ingest 一致）。
func (s *Server) handleAgentDNSMap(w http.ResponseWriter, r *http.Request) {
	var rep shared.DNSMapReport
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if rep.HostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host_id required"})
		return
	}
	fp := r.Header.Get("X-Agent-Fingerprint")
	if fp == "" {
		fp = r.URL.Query().Get("fp")
	}
	if !s.forwardFingerprintOKByHost(rep.HostID, fp) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "fingerprint mismatch"})
		return
	}
	dnsObservations.put(rep.HostID, rep.Entries)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
