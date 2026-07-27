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
// Install scripts (/install.sh, /install.ps1) are intercepted so SERVER= and
// embedded CONFIG_B64 point at the relay. Internal machines then download
// binaries and report metrics through the relay.
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
	proxy.FlushInterval = 100 * time.Millisecond
	proxy.Transport = relayTransport

	dlCache := newRelayDLCache(upstream, relaySecret)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/install.sh" || r.URL.Path == "/install.ps1" {
			serveRelayInstallScript(w, r, upstream, relaySecret)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/dl/") {
			if dlCache.serve(w, r) {
				return
			}
		}
		if relaySecret != "" {
			r.Header.Set("X-Relay-Secret", relaySecret)
		}
		proxy.ServeHTTP(w, r)
	})

	slog.Info("╔══════════════════════════════════════════════════════╗")
	slog.Info("║  AIOps Agent — 网关中继模式 (Relay)                    ║")
	slog.Info("║  监听: " + listenAddr + "  上游: " + upstream + "  ║")
	slog.Info("╚══════════════════════════════════════════════════════╝")
	relayPort := listenAddr
	if _, port, err := net.SplitHostPort(listenAddr); err == nil && port != "" {
		relayPort = ":" + port
	} else if !strings.HasPrefix(listenAddr, ":") {
		relayPort = ":" + listenAddr
	}
	slog.Info("内网机器安装命令", "cmd", "curl -fsSL http://<本机IP>"+relayPort+"/install.sh | sh")

	if listenAddr == "" || strings.HasPrefix(listenAddr, ":") ||
		strings.HasPrefix(listenAddr, "0.0.0.0:") {
		slog.Warn("⚠ 监听地址绑定到所有网卡——如不需外部访问，建议用 --listen 192.168.x.x:8529 绑定到内网IP")
	}

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Relay 启动失败: %v", err)
	}
}

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

var relayClient = &http.Client{Timeout: 30 * time.Second}

var serverLineRe = regexp.MustCompile(`((?:SERVER|\$Server)\s*=\s*")[^"]+(")`)

// Install scripts embed a full commented config.example.yaml reference; 1 MiB is safe.
const maxInstallScriptSize = 1 << 20

func sanitizeHost(h string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.' || r == ':' || r == '-' || r == '[' || r == ']':
			return r
		default:
			return -1
		}
	}, h)
}

func serveRelayInstallScript(w http.ResponseWriter, r *http.Request, upstream, relaySecret string) {
	upstreamURL := upstream + r.URL.Path
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), "GET", upstreamURL, nil)
	if err != nil {
		http.Error(w, "Relay: 构建请求失败", http.StatusInternalServerError)
		return
	}
	if relaySecret != "" {
		req.Header.Set("X-Relay-Secret", relaySecret)
	}

	resp, err := relayClient.Do(req)
	if err != nil {
		http.Error(w, "Relay: 无法连接上游服务端 ("+upstream+")", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxInstallScriptSize))
	if err != nil {
		http.Error(w, "Relay: 读取安装脚本失败", http.StatusInternalServerError)
		return
	}
	if len(body) >= maxInstallScriptSize {
		http.Error(w, "Relay: 安装脚本过大", http.StatusBadGateway)
		return
	}

	host := sanitizeHost(r.Host)
	if host == "" {
		http.Error(w, "Relay: 无效的 Host 头", http.StatusBadRequest)
		return
	}

	relayURL := "http://" + host
	rewritten := rewriteInstallScriptForRelay(string(body), relayURL)

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(rewritten)))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.WriteString(w, rewritten)
}

const relayDLCacheTTL = 10 * time.Minute

type relayDLCache struct {
	dir      string
	upstream string
	secret   string
	mu       sync.Mutex
	locks    map[string]*sync.Mutex
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

// pairKey collapses binary + .sha256 into one lock/generation so they cannot desync.
func relayDLPairKey(name string) string {
	return strings.TrimSuffix(name, ".sha256")
}

func (c *relayDLCache) serve(w http.ResponseWriter, r *http.Request) bool {
	name := path.Base(r.URL.Path)
	if name == "" || name == "." || name == "/" || strings.ContainsAny(name, `/\`) {
		return false
	}
	cf := filepath.Join(c.dir, name)
	pair := relayDLPairKey(name)

	if fi, err := os.Stat(cf); err == nil && !fi.IsDir() && time.Since(fi.ModTime()) < relayDLCacheTTL {
		// Also require sibling .sha256 (or binary) to be fresh when both exist.
		if c.pairFresh(pair) {
			http.ServeFile(w, r, cf)
			return true
		}
	}

	lk := c.lockFor(pair)
	lk.Lock()
	defer lk.Unlock()
	if fi, err := os.Stat(cf); err == nil && !fi.IsDir() && time.Since(fi.ModTime()) < relayDLCacheTTL && c.pairFresh(pair) {
		http.ServeFile(w, r, cf)
		return true
	}
	if err := c.fetchPair(pair); err != nil {
		slog.Warn("Relay /dl 缓存回源失败，回退直连代理", "file", name, "err", err)
		return false
	}
	slog.Info("Relay /dl 缓存已刷新", "pair", pair)
	http.ServeFile(w, r, cf)
	return true
}

func (c *relayDLCache) pairFresh(pair string) bool {
	bin := filepath.Join(c.dir, pair)
	sum := bin + ".sha256"
	fi1, err1 := os.Stat(bin)
	fi2, err2 := os.Stat(sum)
	if err1 != nil && err2 != nil {
		return false
	}
	now := time.Now()
	if err1 == nil && now.Sub(fi1.ModTime()) >= relayDLCacheTTL {
		return false
	}
	if err2 == nil && now.Sub(fi2.ModTime()) >= relayDLCacheTTL {
		return false
	}
	// If both exist, require mtimes within 30s (same generation).
	if err1 == nil && err2 == nil {
		d := fi1.ModTime().Sub(fi2.ModTime())
		if d < 0 {
			d = -d
		}
		if d > 30*time.Second {
			return false
		}
	}
	return true
}

func (c *relayDLCache) fetchPair(pair string) error {
	// Always refresh binary + checksum together when either is requested.
	paths := []string{"/dl/" + pair}
	if !strings.HasSuffix(pair, ".sha256") && !strings.HasSuffix(pair, ".zip") {
		paths = append(paths, "/dl/"+pair+".sha256")
	}
	var firstErr error
	for _, p := range paths {
		dst := filepath.Join(c.dir, path.Base(p))
		if err := c.fetch(p, dst); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			// .sha256 missing is non-fatal for plugins.zip etc.
			if strings.HasSuffix(p, ".sha256") {
				continue
			}
			return err
		}
	}
	return firstErr
}

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
