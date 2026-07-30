package main

import (
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// csrfOriginMiddleware rejects mutating API calls whose Origin/Referer does not
// match the request Host or an explicitly allowed CORS origin. Same-origin
// dashboard traffic always passes; cross-site form POSTs are blocked.
func (s *Server) csrfOriginMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
			next.ServeHTTP(w, r)
			return
		}
		p := r.URL.Path
		// Agent / public bootstrap paths authenticate by token/fingerprint, not cookie.
		if isPublicPath(r) || strings.HasPrefix(p, "/api/v1/agent/") ||
			strings.HasPrefix(p, "/proxy/") || strings.HasPrefix(p, "/dl/") ||
			p == "/api/v1/prom/write" || p == "/api/v1/mcp" ||
			p == "/api/v1/integrations/content-audit" ||
			strings.HasPrefix(p, "/api/v1/auth/oidc/") {
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(p, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if s.originAllowed(r) {
			next.ServeHTTP(w, r)
			return
		}
		slog.Warn("CSRF/Origin 校验拒绝", "method", r.Method, "path", p,
			"origin", r.Header.Get("Origin"), "referer", r.Header.Get("Referer"),
			"host", r.Host, "public_host", s.requestPublicHost(r))
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin not allowed"})
	})
}

func (s *Server) originAllowed(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	reqHost := s.requestPublicHost(r)
	if origin == "" {
		// Some same-origin navigations omit Origin; fall back to Referer host.
		ref := strings.TrimSpace(r.Header.Get("Referer"))
		if ref == "" {
			// Non-browser clients (curl/scripts) with session cookie: allow when
			// no Origin/Referer — CSRF requires a browser to inject Origin.
			return true
		}
		u, err := url.Parse(ref)
		if err != nil || u.Host == "" {
			return false
		}
		return hostMatches(u.Host, reqHost) || s.corsOriginListed(u.Scheme+"://"+u.Host)
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if hostMatches(u.Host, reqHost) {
		return true
	}
	return s.corsOriginListed(origin)
}

// requestPublicHost is the host the browser thinks it talked to. Behind a trusted
// reverse proxy (TrustProxy) prefer X-Forwarded-Host so CSRF Origin checks still
// match when the upstream container sees an internal listen address/port.
func (s *Server) requestPublicHost(r *http.Request) string {
	if s != nil && s.cfg != nil && s.cfg.TrustProxy() {
		if h := firstForwardedValue(r.Header.Get("X-Forwarded-Host")); h != "" {
			return h
		}
	}
	return r.Host
}

func hostMatches(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func (s *Server) corsOriginListed(origin string) bool {
	for _, o := range s.cfg.CORSOrigins() {
		if strings.TrimSpace(o) == origin {
			return true
		}
	}
	return false
}

// logProductionSecurityBaseline emits warnings (or fatals under AIOPS_STRICT_SECURITY)
// for insecure production defaults that block enterprise procurement.
func logProductionSecurityBaseline(cfg *ConfigStore) {
	strict := strings.EqualFold(os.Getenv("AIOPS_STRICT_SECURITY"), "1") ||
		strings.EqualFold(os.Getenv("AIOPS_STRICT_SECURITY"), "true")
	warnOrFatal := func(msg string) {
		if strict {
			slog.Error("安全基线未通过（AIOPS_STRICT_SECURITY）", "msg", msg)
			os.Exit(1)
		}
		slog.Warn("安全基线建议", "msg", msg)
	}
	if !secretEncryptionEnabled() {
		warnOrFatal("未设置 AIOPS_SECRET_KEY：配置密钥以明文存库，生产环境必须设置")
	}
	if dsn := os.Getenv("AIOPS_POSTGRES_DSN"); strings.Contains(strings.ToLower(dsn), "password=postgres") ||
		strings.Contains(dsn, "password=admin") || strings.Contains(dsn, ":postgres@") {
		warnOrFatal("PostgreSQL DSN 疑似使用默认口令，生产环境请更换强密码")
	}
	// Default admin/admin after first boot is a common POC leftover.
	if u, ok := cfg.UserByName("admin"); ok {
		if verifyPassword("admin", u.Salt, u.Hash) {
			warnOrFatal("默认管理员账号仍使用弱口令 admin，请立即修改")
		}
	}
	if len(cfg.CORSOrigins()) == 0 {
		slog.Info("CORS 白名单为空：跨域 API 不再回显 *，仅同源仪表盘可调用变更类接口")
	}
}
