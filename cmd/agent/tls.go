package main

import (
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
)

// buildServerTLS constructs the TLS config the agent uses for every HTTPS
// connection to the server. It supports a custom CA bundle (ca_cert) so a
// self-signed / private-CA server certificate can be trusted properly, plus an
// explicit tls_skip_verify escape hatch. Skip-verify disables authentication of
// the server and exposes the agent to man-in-the-middle attacks, so it is only
// for throwaway/lab use — a pinned CA is the correct choice for self-signed.
func buildServerTLS(skipVerify bool, caCertPath string) *tls.Config {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if caCertPath != "" {
		if pem, err := os.ReadFile(caCertPath); err != nil {
			slog.Error("读取 CA 证书失败，回退系统信任库", "path", caCertPath, "err", err)
		} else {
			pool := x509.NewCertPool()
			if pool.AppendCertsFromPEM(pem) {
				cfg.RootCAs = pool
				slog.Info("已加载自定义 CA 证书用于校验服务端 TLS", "path", caCertPath)
			} else {
				slog.Error("CA 证书解析失败（非 PEM 格式？）", "path", caCertPath)
			}
		}
	}
	if skipVerify {
		cfg.InsecureSkipVerify = true
		slog.Warn("已禁用服务端 TLS 证书校验（tls_skip_verify）——存在中间人风险，仅建议临时/内网自签场景临时使用")
	}
	return cfg
}

// configureServerTLS applies the server TLS config to every agent→server HTTP
// client/transport in one place: report, terminal (relay + long-poll), port
// forwarding, log ingest, and relay-mode upstream. It is called once at startup
// before any request is issued, so mutating these package-level clients is
// race-free. A no-op unless a CA cert or skip-verify is configured (default:
// standard verification against the system trust store).
func configureServerTLS(skipVerify bool, caCertPath string) {
	if !skipVerify && caCertPath == "" {
		return
	}
	cfg := buildServerTLS(skipVerify, caCertPath)

	// Transports that already exist with a custom *http.Transport: set TLS in place.
	reportTransport.TLSClientConfig = cfg
	if t, ok := termHTTP.Transport.(*http.Transport); ok {
		t.TLSClientConfig = cfg
	}
	relayTransport.TLSClientConfig = cfg

	// Clients that use the default transport (nil): give them one carrying the
	// TLS config. No ResponseHeaderTimeout — several of these are long-poll waits.
	shared := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:     cfg,
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   false,
	}
	forwardWaitHTTP.Transport = shared
	termWaitHTTP.Transport = shared
	logCollectHTTP.Transport = shared
	relayClient.Transport = shared
}
