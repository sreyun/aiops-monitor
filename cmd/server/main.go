package main

import (
	"compress/gzip"
	"flag"
	"log"
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
const appVersion = "1.1.0"

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
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
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
const maxBodyBytes = 2 << 20 // 2 MiB

// bodyLimitMiddleware wraps every request body in a MaxBytesReader so a
// malicious or buggy client can't stream an unbounded payload into memory.
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
type gzipResponseWriter struct {
	http.ResponseWriter
	gz    *gzip.Writer
	wrote bool
}

func (w *gzipResponseWriter) ensureHeader() {
	if w.wrote {
		return
	}
	w.wrote = true
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
	return w.gz.Write(b)
}

// gzipMiddleware compresses text/JSON responses for clients that accept gzip.
// At many-host scale the /hosts + /activity JSON polled every few seconds is the
// dominant bandwidth cost, and it compresses ~8-10x. WebSocket upgrades (the
// remote terminal) are skipped so hijacking still works.
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") ||
			strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") ||
			strings.Contains(r.URL.Path, "/terminal") { // WS upgrade + streaming relays must not be buffered
			next.ServeHTTP(w, r)
			return
		}
		gz := gzipWriterPool.Get().(*gzip.Writer)
		gz.Reset(w)
		defer func() { gz.Close(); gzipWriterPool.Put(gz) }()
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, gz: gz}, r)
	})
}

func main() {
	addr := flag.String("addr", ":8080", "监听地址，如 :8080 或 0.0.0.0:8080")
	cfgPath := flag.String("config", "server_config.json", "服务端配置文件路径（告警/阈值/分类）")
	distDir := flag.String("dist", "", "Agent 下载目录（含各平台二进制与 plugins.zip）；留空自动探测 ./dist 或程序所在目录")
	flag.Parse()

	dist := resolveDist(*distDir)
	store := NewStore()
	cfg := NewConfigStore(*cfgPath)
	notifier := NewNotifier(store, cfg)
	server := NewServer(store, cfg, notifier, dist, *addr)

	// embedded lightweight DB: restore state, then autosave + flush on exit
	db := NewDB(dbPathFor(*cfgPath), store, server.auth)
	db.Load()
	go db.AutoSave(15 * time.Second)
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		if err := db.Save(); err != nil {
			log.Printf("退出前落盘失败: %v", err)
		} else {
			log.Printf("状态已落盘,服务端退出。")
		}
		os.Exit(0)
	}()

	go notifier.Run(10 * time.Second)     // periodic alert evaluation + dedup push
	go server.checks.Run(5 * time.Second) // custom HTTP/TCP synthetic checks

	handler := corsMiddleware(gzipMiddleware(bodyLimitMiddleware(server.authMiddleware(server.Routes()))))
	srv := &http.Server{
		Addr:    *addr,
		Handler: handler,
		// ReadHeaderTimeout guards slow-header attacks while leaving request/
		// response bodies unbounded — the terminal relay streams for minutes and
		// the WebSocket is hijacked, so a fixed Read/WriteTimeout can't apply.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("AIOps Monitor 服务端已启动")
	log.Printf("  监控面板: http://localhost%s", *addr)
	log.Printf("  API 前缀: http://localhost%s/api/v1/", *addr)
	log.Printf("  配置文件: %s", *cfgPath)
	log.Printf("  数据库:   %s（内嵌轻量库,历史/日志/会话重启不丢）", dbPathFor(*cfgPath))
	if hasAgentBinary(dist) {
		log.Printf("  下载目录: %s（一键安装可用）", dist)
	} else {
		log.Printf("  警告: 未找到 Agent 下载文件，一键安装不可用；请用 -dist 指定含 aiops-agent* 与 plugins.zip 的目录")
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
