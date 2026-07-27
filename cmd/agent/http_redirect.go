package main

import (
	"fmt"
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
