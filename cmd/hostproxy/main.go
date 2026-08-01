// hostproxy is a tiny host-side reverse proxy for Docker Desktop / published-port
// setups where the container only sees the bridge gateway (e.g. 192.168.97.1)
// as RemoteAddr. It listens on the public port, forwards to the container's
// localhost-mapped port, and injects X-Real-IP / X-Forwarded-* from the real
// TCP peer so aiops-server (with trust_proxy) can record the visitor IP.
//
// Critical: preserve the browser Host (e.g. 127.0.0.1:8529). Rewriting Host to
// the upstream port (18529) breaks CSRF Origin checks (Origin :8529 ≠ Host :18529)
// and breaks logout / forecast / any cookie-authenticated POST.
//
// Critical: strip client-supplied CF-Connecting-IP / True-Client-IP / forged
// X-Real-IP / X-Forwarded-* before injecting the TCP peer. aiops-server's
// clientIP() prefers CF-Connecting-IP over X-Real-IP when TrustProxy is on —
// leaving a forged CF header would bypass login lockout and API rate limits.
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	listen := flag.String("listen", envOr("AIOPS_HTTP_LISTEN", ":8529"), "public listen address")
	target := flag.String("target", envOr("AIOPS_PROXY_TARGET", "http://127.0.0.1:18529"), "upstream container URL")
	flag.Parse()

	u, err := url.Parse(*target)
	if err != nil || u.Scheme == "" || u.Host == "" {
		log.Fatalf("invalid -target %q: %v", *target, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.FlushInterval = 50 * time.Millisecond
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		log.Printf("upstream error %s %s: %v", r.Method, r.URL.Path, e)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		clientHost := req.Host
		origDirector(req)
		rewriteProxyHeaders(req, clientHost)
	}

	srv := &http.Server{
		Addr:              *listen,
		Handler:           proxy,
		ReadHeaderTimeout: 15 * time.Second,
	}

	go func() {
		log.Printf("hostproxy listening on %s → %s (preserves Host, injects visitor X-Real-IP)", *listen, u.String())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

// rewriteProxyHeaders preserves browser Host and replaces client-IP / forwarded
// headers with values derived from the real TCP peer only.
func rewriteProxyHeaders(req *http.Request, clientHost string) {
	// Dial uses req.URL (upstream); keep browser Host for CSRF / cookies / absolute URLs.
	if clientHost != "" {
		req.Host = clientHost
		req.Header.Set("X-Forwarded-Host", clientHost)
	}

	// Drop hop-by-hop / forgeable identity headers before injecting trusted ones.
	// Especially CF-Connecting-IP: server clientIP() checks it before X-Real-IP.
	for _, h := range []string{
		"CF-Connecting-IP",
		"True-Client-IP",
		"X-Real-IP",
		"X-Forwarded-For",
		"X-Forwarded-Proto",
	} {
		req.Header.Del(h)
	}

	client := peerIP(req)
	if client == "" {
		return
	}
	req.Header.Set("X-Real-IP", client)
	req.Header.Set("X-Forwarded-For", client)
	req.Header.Set("X-Forwarded-Proto", forwardedProto(req))
}

func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	if host == "::1" {
		return "127.0.0.1"
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return ""
}

// forwardedProto reflects the scheme of the connection hostproxy itself
// terminated. Never trust a client-supplied X-Forwarded-Proto — attackers
// would otherwise force Secure session cookies on plain HTTP.
func forwardedProto(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
