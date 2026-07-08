package main

import (
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
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
	log.Printf("内网机器安装命令: curl -fsSL http://<本机IP>%s/install.sh | sh", listenAddr)

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

// relayClient is used for the one-shot install-script fetch (small response,
// generated instantly — a 15s timeout is plenty).
var relayClient = &http.Client{Timeout: 15 * time.Second}

// serverLineRe matches the SERVER= / $Server = assignment line in install
// scripts, so the relay can rewrite the URL to its own address regardless of
// how the upstream rendered it (scheme, port, trailing slash, …).
var serverLineRe = regexp.MustCompile(`((?:SERVER|\$Server)\s*=\s*")[^"]+(")`)

// serveRelayInstallScript proxies an install.sh / install.ps1 request to the
// upstream cloud server, then rewrites the SERVER address in the response body
// to the relay's own address (derived from the request Host header). Internal
// machines thus auto-configure to connect through the relay.
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Relay: 读取安装脚本失败", http.StatusInternalServerError)
		return
	}

	// Rewrite SERVER="..." (and $Server = "...") to the relay's address.
	relayURL := "http://" + r.Host
	rewritten := serverLineRe.ReplaceAllString(string(body), "${1}"+relayURL+"${2}")

	// Copy content type + status.
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(rewritten)))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.WriteString(w, rewritten)
}
