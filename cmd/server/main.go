package main

import (
	"compress/gzip"
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
// The default "aiops" is a fallback for development builds; production builds
// inject the real Git tag at build time via ldflags:
//
//   go build -ldflags "-X main.appVersion=$(git describe --tags)" ./cmd/server ./cmd/agent
//
// or use the build script:  powershell -File build.ps1
//
// git describe --tags outputs tags like "v3.9.4" (already has the "v" prefix),
// so the frontend renders the value as-is without prepending another "v".
var appVersion = "aiops"

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
// securityHeadersMiddleware adds conservative hardening headers to every
// response (no MIME sniffing, no framing/clickjacking, no referrer leakage).
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
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
			strings.Contains(r.URL.Path, "/terminal") || // WS upgrade + streaming relays must not be buffered
			strings.Contains(r.URL.Path, "/forward") || // port forwarding streams must not be buffered
			strings.HasPrefix(r.URL.Path, "/proxy/") { // HTTP proxy tunnels must not be buffered
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
	addr := flag.String("addr", ":8529", Tz("server.flag_addr"))
	cfgPath := flag.String("config", "server_config.json", Tz("server.flag_config"))
	distDir := flag.String("dist", "", Tz("server.flag_dist"))
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	dist := resolveDist(*distDir)
	store := NewStore()
	cfg, err := NewConfigStore(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
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
			slog.Error(Tz("server.save_failed"), "err", err)
		} else {
			slog.Info(Tz("server.saved_exit"))
		}
		os.Exit(0)
	}()

	go notifier.Run(10 * time.Second)     // periodic alert evaluation + dedup push
	go server.checks.Run(5 * time.Second) // custom HTTP/TCP synthetic checks

	handler := securityHeadersMiddleware(corsMiddleware(gzipMiddleware(bodyLimitMiddleware(server.authMiddleware(server.Routes())))))
	srv := &http.Server{
		Addr:    *addr,
		Handler: handler,
		// ReadHeaderTimeout guards slow-header attacks while leaving request/
		// response bodies unbounded — the terminal relay streams for minutes and
		// the WebSocket is hijacked, so a fixed Read/WriteTimeout can't apply.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	slog.Info(Tz("server.started"))
	slog.Info(Tz("server.dashboard_url"), "url", "http://localhost"+*addr)
	slog.Info(Tz("server.api_url"), "url", "http://localhost"+*addr+"/api/v1/")
	slog.Info(Tz("server.config_file"), "path", *cfgPath)
	slog.Info(Tz("server.db_path"), "path", dbPathFor(*cfgPath), "note", Tz("server.db_note"))
	if hasAgentBinary(dist) {
		slog.Info(Tz("server.dist_dir"), "path", dist, "note", Tz("server.dist_ok"))
	} else {
		slog.Warn(Tz("server.dist_missing"))
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
