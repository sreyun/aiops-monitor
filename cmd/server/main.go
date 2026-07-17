package main

import (
	"compress/gzip"
	"context"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// appVersion is shown in the dashboard sidebar and the summary API.
// The default "AIOps" is a fallback for development builds; production builds
// inject the real Git tag at build time via ldflags:
//
//	go build -ldflags "-X main.appVersion=$(git describe --tags)" ./cmd/server ./cmd/agent
//
// or use the build script:  powershell -File build.ps1
//
// git describe --tags outputs tags like "v3.9.4" (already has the "v" prefix),
// so the frontend renders the value as-is without prepending another "v".
var appVersion = "AIOps"

// resolveDist finds the directory that holds the downloadable agent binaries
// (+ plugins.zip). It tries the -dist flag, ./dist, then the server executable's
// own dir and its dist/ subdir — so the one-line install works whether the
// server is launched from the repo root or from bin/.
func resolveDist(flagVal string) string {
	var candidates []string
	if flagVal != "" {
		candidates = append(candidates, flagVal)
	}
	candidates = append(candidates, "dist")
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(d, "dist"), d)
	}
	for _, c := range candidates {
		if hasAgentBinary(c) {
			return c
		}
	}
	if flagVal != "" {
		return flagVal
	}
	return "dist"
}

func hasAgentBinary(dir string) bool {
	if dir == "" {
		return false
	}
	for _, n := range []string{
		"aiops-agent.exe",
		"aiops-agent-linux-amd64", "aiops-agent-linux-arm64",
		"aiops-agent-darwin-arm64", "aiops-agent-darwin-amd64",
	} {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			return true
		}
	}
	return false
}

// corsMiddleware allows the dashboard (or external tools) to call the API
// cross-origin and short-circuits preflight OPTIONS requests.
// When CORSOrigins is configured, only matching Origin headers are echoed;
// otherwise the legacy wildcard "*" is used for backward compatibility.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origins := s.cfg.CORSOrigins()
		if len(origins) > 0 {
			origin := r.Header.Get("Origin")
			if origin != "" {
				for _, o := range origins {
					if strings.TrimSpace(o) == origin {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Set("Vary", "Origin")
						break
					}
				}
			}
			// Origin absent or not in whitelist → no CORS headers → browser blocks.
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// maxBodyBytes caps request bodies to blunt memory-exhaustion via oversized
// JSON. Reports (metrics + up to 256 process names + disks + GPUs) fit easily.
// Forwarding proxy requests need a larger limit (up to 100MB for file uploads).
const maxBodyBytes = 100 << 20 // 100 MiB

// bodyLimitMiddleware wraps every request body in a MaxBytesReader so a
// malicious or buggy client can't stream an unbounded payload into memory.
// securityHeadersMiddleware adds conservative hardening headers to every
// response (no MIME sniffing, no framing/clickjacking, no referrer leakage).
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		// Content-Security-Policy: defense-in-depth. script-src is 'self' ONLY —
		// all inline on*= handlers were refactored to delegated listeners and the
		// theme-init inline script was externalised (/theme-init.js), so even a
		// stored-XSS payload cannot execute inline JS. style-src keeps 'unsafe-inline'
		// (inline style= attributes are pervasive and low-risk — no script execution).
		// The policy also blocks plugins, base-tag/form hijacking, framing
		// (clickjacking), and cross-origin exfiltration (connect/img/font = self).
		// Skipped for /proxy/ — those responses are arbitrary target-host web apps
		// that must keep their own CSP/resources.
		if !strings.HasPrefix(r.URL.Path, "/proxy/") {
			h.Set("Content-Security-Policy",
				"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
					"img-src 'self' data:; font-src 'self' data:; connect-src 'self'; "+
					"object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		}
		next.ServeHTTP(w, r)
	})
}

func bodyLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// gzipWriterPool reuses gzip.Writer instances across requests to avoid per-
// request allocation under the many-host polling load.
var gzipWriterPool = sync.Pool{New: func() any { return gzip.NewWriter(nil) }}

// gzipResponseWriter transparently compresses the response body. It strips any
// Content-Length (now wrong post-compression) and advertises gzip on the first
// write.
//
// SSE 例外：当 handler 把 Content-Type 设为 text/event-stream 时，本 writer 切换为
// passthrough（直写底层、不压缩）。原因是 gzip.Writer 会把每个 data: 帧压进内部缓冲，
// 直到 Close 才吐给客户端——这会彻底破坏「逐字流式」，让整段 AI 回复一次性到达。
type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wrote       bool
	passthrough bool // true = 命中 SSE：绕过 gzip，直写底层并逐帧 Flush
}

func (w *gzipResponseWriter) ensureHeader() {
	if w.wrote {
		return
	}
	w.wrote = true
	// 流式响应（SSE）必须逐帧实时下发，不能经 gzip 缓冲。
	if strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		w.passthrough = true
		return
	}
	h := w.Header()
	h.Del("Content-Length")
	h.Set("Content-Encoding", "gzip")
	h.Add("Vary", "Accept-Encoding")
}
func (w *gzipResponseWriter) WriteHeader(code int) {
	// 101/204/304 carry no compressible body — pass through untouched.
	if code == http.StatusSwitchingProtocols || code == http.StatusNoContent || code == http.StatusNotModified {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.ensureHeader()
	w.ResponseWriter.WriteHeader(code)
}
func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if w.passthrough {
		return w.ResponseWriter.Write(b)
	}
	return w.gz.Write(b)
}

// Flush 实现 http.Flusher —— 这是 SSE 逐字流式能工作的关键：
//  1. gzipResponseWriter 仅内嵌 http.ResponseWriter 接口（该接口不含 Flush），若不显式
//     实现，handler 里的 `w.(http.Flusher)` 断言就会失败、所有 flush 沦为空操作，数据全被
//     憋到 handler 返回。这正是此前 AI 会话/诊断「不逐字」的根因。
//  2. 压缩响应必须先 flush gzip 缓冲、再 flush 底层 writer，否则压缩字节滞留在 gzip 内部；
//     SSE passthrough 则直接 flush 底层。
func (w *gzipResponseWriter) Flush() {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if !w.passthrough {
		_ = w.gz.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// gzipMiddleware compresses text/JSON responses for clients that accept gzip.
// At many-host scale the /hosts + /activity JSON polled every few seconds is the
// dominant bandwidth cost, and it compresses ~8-10x. WebSocket upgrades (the
// remote terminal) are skipped so hijacking still works.
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") ||
			strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") ||
			strings.Contains(r.URL.Path, "/terminal") || // WS upgrade + streaming relays must not be buffered
			strings.Contains(r.URL.Path, "/forward") || // port forwarding streams must not be buffered
			strings.HasPrefix(r.URL.Path, "/proxy/") { // HTTP proxy tunnels must not be buffered
			next.ServeHTTP(w, r)
			return
		}
		gz := gzipWriterPool.Get().(*gzip.Writer)
		gz.Reset(w)
		gzw := &gzipResponseWriter{ResponseWriter: w, gz: gz}
		// 仅当确实用 gzip 压过内容才写 gzip 尾（Close）。SSE passthrough 与空响应跳过，
		// 否则会往流式响应尾部追加乱码字节 / 往 204 等空响应硬塞一段空 gzip。
		defer func() {
			if gzw.wrote && !gzw.passthrough {
				gz.Close()
			}
			gzipWriterPool.Put(gz)
		}()
		next.ServeHTTP(gzw, r)
	})
}

// mustOpenPG connects to PostgreSQL, retrying briefly so a docker-compose cold
// start (PG still initializing behind its healthcheck) doesn't abort the boot.
// There is no embedded fallback: after the retry window a connection failure is
// fatal, by design — the platform stores all relational state in PostgreSQL.
func mustOpenPG(dsn string) *pgStore {
	const attempts = 10
	var lastErr error
	for i := 0; i < attempts; i++ {
		p, err := openPGStore(dsn)
		if err == nil {
			return p
		}
		lastErr = err
		slog.Warn("PostgreSQL 连接未就绪，重试中…", "attempt", i+1, "max", attempts, "err", err)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("PostgreSQL 连接失败（已重试 %d 次），服务终止：%v", attempts, lastErr)
	return nil
}

func main() {
	addr := flag.String("addr", ":8529", Tz("server.flag_addr"))
	cfgPath := flag.String("config", "server_config.json", Tz("server.flag_config"))
	distDir := flag.String("dist", "", Tz("server.flag_dist"))
	// v5.4.0: admin password reset
	resetAdmin := flag.Bool("reset-admin", false, "Reset the first admin user's password to a random value and print it to console, then exit")
	resetAdminAPI := flag.String("reset-admin-api", "", "Start a local HTTP API on 127.0.0.1:PORT for admin password reset (e.g. -reset-admin-api=:9999)")
	flag.Parse()

	// Handle admin password reset flags before any server logic
	if *resetAdmin {
		runResetAdmin(*cfgPath)
		return
	}
	if *resetAdminAPI != "" {
		runResetAdminAPI(*cfgPath, *resetAdminAPI)
		return
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	dist := resolveDist(*distDir)
	store := NewStore()

	// Storage is unified on PostgreSQL (all relational data) + VictoriaMetrics (all
	// time-series). The embedded aiops.db single-file store is fully retired — both
	// backends are REQUIRED and the server refuses to start without them, so state
	// can never silently land in a local file.
	dsn := strings.TrimSpace(os.Getenv("AIOPS_POSTGRES_DSN"))
	if dsn == "" {
		log.Fatal("AIOPS_POSTGRES_DSN 未配置：本平台已统一使用 PostgreSQL + VictoriaMetrics 存储，内置数据库已停用。请在环境变量中配置 PostgreSQL DSN（参见 docker-compose.yml）")
	}
	if strings.TrimSpace(os.Getenv("AIOPS_VM_URL")) == "" {
		log.Fatal("AIOPS_VM_URL 未配置：时序数据（指标/趋势）已统一写入 VictoriaMetrics。请在环境变量中配置 VM 地址（参见 docker-compose.yml）")
	}
	// Connect to PostgreSQL with a bounded retry so a docker-compose cold start (PG
	// still initializing) doesn't abort the boot; after the window it is fatal —
	// there is no local fallback.
	pg := mustOpenPG(dsn)
	slog.Info("PostgreSQL 已连接：配置 / 用户 / 审计 / 事件 / 工单 / 会话统一持久化到 PG")
	store.BindPG(pg) // audit log + plugin events → PG
	if secretEncryptionEnabled() {
		slog.Info("配置密钥落库加密已启用（AIOPS_SECRET_KEY）：MFA/SMTP/AI/webhook 等密钥 AES-256-GCM 静态加密")
	} else {
		slog.Warn("未设置 AIOPS_SECRET_KEY：配置中的密钥以明文存库，建议设置以启用静态加密")
	}

	cfg, err := NewConfigStore(*cfgPath, pg)
	if err != nil {
		log.Fatal(err)
	}
	notifier := NewNotifier(store, cfg)
	server := NewServer(store, cfg, notifier, dist, *addr)
	notifier.forward = server.forward
	notifier.hw = server.hw // 硬件异常接入统一告警链路（去重/推送与 CPU、磁盘等一致）

	server.term.loadRecordings(recordingsDirFor(*cfgPath)) // terminal replays survive restart (file-backed)
	server.term.pg = pg                                    // 终端会话录制永久留存到 PG（入库审计，不受内存 100 条上限影响）
	server.bindPG(pg)                                      // load + periodically persist incidents / work orders / sessions

	go notifier.Run(10 * time.Second)           // periodic alert evaluation + dedup push
	go server.checks.Run(5 * time.Second)       // custom HTTP/TCP synthetic checks
	go server.apimon.Run(5 * time.Second)       // API 性能监控：按业务系统批量探测接口
	go server.runScheduler(30 * time.Second)    // timed playbook triggers (interval/daily/weekly)
	go server.runSLOEvaluator(60 * time.Second) // SLO error-budget evaluation → burn incidents
	go server.ai.runInspectionLoop()            // scheduled AI/heuristic health inspection
	go server.vm.run()                          // optional VictoriaMetrics remote-write pump

	handler := securityHeadersMiddleware(server.corsMiddleware(gzipMiddleware(bodyLimitMiddleware(server.authMiddleware(server.Routes())))))
	srv := &http.Server{
		Addr:    *addr,
		Handler: handler,
		// ReadHeaderTimeout guards slow-header attacks while leaving request/
		// response bodies unbounded — the terminal relay streams for minutes and
		// the WebSocket is hijacked, so a fixed Read/WriteTimeout can't apply.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown: on SIGINT/SIGTERM, stop accepting new connections,
	// drain active HTTP requests (up to 30s), flush PostgreSQL state, then exit.
	// This replaces the old os.Exit(0) approach which bypassed defer cleanup
	// and forcibly dropped active connections.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		slog.Info("收到停止信号，正在优雅关闭…")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Warn("HTTP 服务关闭异常", "err", err)
		}
		// Final flush of all relational state to PostgreSQL, then close cleanly.
		server.pgFlush(pg, true)
		pg.close()
		os.Exit(0)
	}()

	slog.Info(Tz("server.started"))
	slog.Info(Tz("server.dashboard_url"), "url", "http://localhost"+*addr)
	slog.Info(Tz("server.api_url"), "url", "http://localhost"+*addr+"/api/v1/")
	slog.Info(Tz("server.config_file"), "path", *cfgPath)
	slog.Info("存储后端", "relational", "PostgreSQL", "timeseries", "VictoriaMetrics", "note", "内置 aiops.db 已停用")
	if hasAgentBinary(dist) {
		slog.Info(Tz("server.dist_dir"), "path", dist, "note", Tz("server.dist_ok"))
	} else {
		slog.Warn(Tz("server.dist_missing"))
	}
	// TLS / HTTPS: when a cert+key pair is provided, serve over TLS so agent↔server
	// and browser↔server traffic (login credentials, session cookie, agent
	// fingerprint, terminal I/O) is encrypted. When enabled, isHTTPS(r) becomes true
	// for direct connections, so the session cookie's Secure flag is set automatically.
	// Without it the server still serves plain HTTP (intended only behind a
	// TLS-terminating reverse proxy) and warns loudly.
	certFile := strings.TrimSpace(os.Getenv("AIOPS_TLS_CERT"))
	keyFile := strings.TrimSpace(os.Getenv("AIOPS_TLS_KEY"))
	if certFile != "" && keyFile != "" {
		slog.Info("已启用 TLS/HTTPS（加密传输）", "cert", certFile)
		if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
		return
	}
	slog.Warn("未配置 TLS（AIOPS_TLS_CERT/AIOPS_TLS_KEY）：以明文 HTTP 提供服务。生产环境请启用 TLS，或置于 HTTPS 终止代理之后，否则登录凭据/会话/终端数据将明文传输")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
