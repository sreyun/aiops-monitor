package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// preserveAgentRedirect keeps POST (and other non-GET) methods + bodies across
// reverse-proxy redirects. Go's default Client converts 301/302 into GET and
// drops the body; behind openresty/nginx HTTP→HTTPS that turns
// POST /api/v1/agent/register into GET → "404 page not found", which is exactly
// the install self-test failure operators see when config.server is still http://.
//
// Cross-host redirects are rejected so a token/fingerprint in the body cannot be
// forwarded to an attacker-controlled Location. Same hostname with a scheme/port
// change (http://host → https://host) is allowed.
func preserveAgentRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fmt.Errorf("stopped after 5 redirects")
	}
	if len(via) == 0 {
		return nil
	}
	orig := via[0]
	if !sameAgentRedirectHost(orig.URL, req.URL) {
		return fmt.Errorf("redirect to different host blocked (%s → %s)", orig.URL.Host, req.URL.Host)
	}
	// Restore the original method if the default client downgraded POST→GET.
	if orig.Method != "" && req.Method != orig.Method {
		req.Method = orig.Method
		if orig.GetBody != nil {
			body, err := orig.GetBody()
			if err != nil {
				return err
			}
			req.Body = body
			req.GetBody = orig.GetBody
			req.ContentLength = orig.ContentLength
		}
		if ct := orig.Header.Get("Content-Type"); ct != "" {
			req.Header.Set("Content-Type", ct)
		}
	}
	return nil
}

func sameAgentRedirectHost(from, to *url.URL) bool {
	if from == nil || to == nil {
		return false
	}
	a := strings.ToLower(from.Hostname())
	b := strings.ToLower(to.Hostname())
	if a == "" || b == "" {
		return false
	}
	return a == b
}

// newAgentHTTPClient returns the shared agent→server client policy: report
// transport pool + POST-preserving redirects.
func newAgentHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		Transport:     reportTransport,
		CheckRedirect: preserveAgentRedirect,
	}
}

// applyAgentRedirectPolicy attaches POST-preserving redirects to an existing
// client (terminal / log / relay long-polls that share package-level vars).
func applyAgentRedirectPolicy(c *http.Client) {
	if c == nil {
		return
	}
	c.CheckRedirect = preserveAgentRedirect
}

// normalizeServersPreferHTTPS rewrites http://host (implicit :80) to https://host
// when the server issues a same-host HTTPS redirect. Streaming terminal/desktop
// TX uses io.Pipe bodies without GetBody, so POST-preserving redirects cannot
// replay them — the durable fix is to talk HTTPS from the first hop.
func normalizeServersPreferHTTPS(servers []ServerConfig, cfgPath string) []ServerConfig {
	if len(servers) == 0 {
		return servers
	}
	out := make([]ServerConfig, len(servers))
	copy(out, servers)
	changed := false
	for i := range out {
		raw := strings.TrimRight(strings.TrimSpace(out[i].Server), "/")
		up := probeUpgradeHTTPToHTTPS(raw)
		if up == "" || up == raw {
			continue
		}
		slog.Info("服务端强制 HTTPS，已改写上报地址", "from", raw, "to", up)
		out[i].Server = up
		changed = true
		if cfgPath != "" {
			upgradeConfigServerURL(cfgPath, raw, up, io.Discard)
		}
	}
	if changed {
		return out
	}
	return servers
}

// probeUpgradeHTTPToHTTPS returns https://host when http://host (port 80/empty)
// redirects to the same hostname over TLS. Non-80 ports (lab :8529) are left alone.
func probeUpgradeHTTPToHTTPS(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || !strings.EqualFold(u.Scheme, "http") {
		return raw
	}
	if p := u.Port(); p != "" && p != "80" {
		return raw
	}
	httpsBase := "https://" + u.Hostname()
	noFollow := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	for _, path := range []string{"/healthz", "/api/v1/agent/register", "/"} {
		req, err := http.NewRequest(http.MethodHead, strings.TrimRight(raw, "/")+path, nil)
		if err != nil {
			continue
		}
		resp, err := noFollow.Do(req)
		if err != nil {
			continue
		}
		loc := resp.Header.Get("Location")
		_ = resp.Body.Close()
		if resp.StatusCode >= 300 && resp.StatusCode < 400 && loc != "" {
			lu, err := url.Parse(loc)
			if err != nil {
				continue
			}
			if !lu.IsAbs() {
				base, _ := url.Parse(raw)
				lu = base.ResolveReference(lu)
			}
			if strings.EqualFold(lu.Scheme, "https") && strings.EqualFold(lu.Hostname(), u.Hostname()) {
				return httpsBase
			}
		}
	}
	req, err := http.NewRequest(http.MethodGet, httpsBase+"/healthz", nil)
	if err != nil {
		return raw
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return raw
	}
	_ = resp.Body.Close()
	if resp.StatusCode > 0 && resp.StatusCode < 500 {
		return httpsBase
	}
	return raw
}
