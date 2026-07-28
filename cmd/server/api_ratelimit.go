package main

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// Global API IP rate limit (sliding window). Complements login lockout and
// MCP/AI per-feature limits. Agent ingest + static downloads are excluded so
// high-frequency report/terminal reverse channels are not starved.
const (
	apiRateWindowSec = 60
	apiRateMaxPerIP  = 300 // dashboard/API bursts; agent paths bypass
)

type apiRateLimiter struct {
	mu   sync.Mutex
	hits map[string][]int64
}

func newAPIRateLimiter() *apiRateLimiter {
	return &apiRateLimiter{hits: make(map[string][]int64)}
}

func (l *apiRateLimiter) allow(ip string) bool {
	if l == nil || ip == "" {
		return true
	}
	now := time.Now().Unix()
	cut := now - apiRateWindowSec
	l.mu.Lock()
	defer l.mu.Unlock()
	arr := l.hits[ip]
	n := 0
	for _, t := range arr {
		if t > cut {
			arr[n] = t
			n++
		}
	}
	arr = arr[:n]
	if len(arr) >= apiRateMaxPerIP {
		l.hits[ip] = arr
		return false
	}
	l.hits[ip] = append(arr, now)
	// Opportunistic prune of idle keys when map grows large.
	if len(l.hits) > 10000 {
		for k, v := range l.hits {
			if len(v) == 0 || v[len(v)-1] < cut {
				delete(l.hits, k)
			}
		}
	}
	return true
}

func apiRateLimitSkip(path string) bool {
	if path == "/healthz" || path == "/" {
		return true
	}
	if strings.HasPrefix(path, "/dl/") || strings.HasPrefix(path, "/js/") ||
		strings.HasPrefix(path, "/css/") {
		return true
	}
	// Agent reverse channels + ingest — fingerprint/token gated, high volume.
	if strings.HasPrefix(path, "/api/v1/agent/") {
		return true
	}
	if path == "/api/v1/prom/write" || path == "/api/v1/mcp" ||
		path == "/api/v1/integrations/content-audit" {
		return true
	}
	if strings.HasPrefix(path, "/proxy/") {
		return true
	}
	return !strings.HasPrefix(path, "/api/")
}

func (s *Server) apiRateLimitMiddleware(next http.Handler) http.Handler {
	lim := newAPIRateLimiter()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiRateLimitSkip(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		ip := s.clientIP(r)
		if !lim.allow(ip) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": Tr(r, "auth.too_many_attempts"),
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
