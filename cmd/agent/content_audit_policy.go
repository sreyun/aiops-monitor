package main

// Agent-side content-audit policy. The endpoint is the last trustworthy place
// to minimize sensitive data before transport and storage, so redaction is
// deliberately performed before events enter the collector buffer.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"aiops-monitor/shared"
)

const (
	contentBodyMetadata = "metadata"
	contentBodyRedacted = "redacted"
	contentBodyFull     = "full"
)

var defaultAuditRedactKeys = map[string]bool{
	"authorization": true, "proxy_authorization": true, "cookie": true, "set_cookie": true,
	"password": true, "passwd": true, "pwd": true, "secret": true, "client_secret": true,
	"token": true, "access_token": true, "refresh_token": true, "id_token": true,
	"api_key": true, "apikey": true, "access_key": true, "private_key": true,
	"credential": true, "credentials": true, "session": true, "session_id": true,
}

type auditRedactRule struct {
	label string
	re    *regexp.Regexp
}

var auditRedactRules = []auditRedactRule{
	{"llm_api_key", regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`)},
	{"aws_access_key", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{"private_key", regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)},
	{"jwt", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
	{"bearer_token", regexp.MustCompile(`(?i)\bBearer[ \t]+[A-Za-z0-9._~+/-]{12,}=*\b`)},
	{"email", regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)},
	{"cn_id", regexp.MustCompile(`\b[1-9]\d{5}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[0-9Xx]\b`)},
	{"cn_phone", regexp.MustCompile(`\b1[3-9]\d{9}\b`)},
}

// applyContentAuditPolicy returns false when an event is outside the configured
// host/path scope. Otherwise it enriches hashes/sizes and applies the selected
// body policy in-place.
func applyContentAuditPolicy(cfg SNIConfig, ev *shared.ContentAuditEvent) bool {
	if ev == nil {
		return false
	}
	host := normalizeAuditHost(ev.Host)
	if len(cfg.ContentAuditIncludeHosts) > 0 && !matchesAnyAuditPattern(host, cfg.ContentAuditIncludeHosts) {
		return false
	}
	if matchesAnyAuditPattern(host, cfg.ContentAuditExcludeHosts) ||
		matchesAnyAuditPattern(strings.ToLower(ev.Path), cfg.ContentAuditExcludePaths) {
		return false
	}

	mode := normalizeContentBodyMode(cfg.ContentAuditBodyMode)
	if strings.EqualFold(ev.Protocol, "tls") {
		mode = contentBodyMetadata
	}
	ev.BodyMode = mode
	ev.CaptureBackend = effectiveCaptureBackend(cfg.CaptureBackend)
	ev.ReqBytes = len([]byte(ev.Body))
	ev.RespBytes = len([]byte(ev.RespBody))
	ev.ReqSHA256 = auditBodyHash(ev.Body)
	ev.RespSHA256 = auditBodyHash(ev.RespBody)

	var labels []string
	var count int
	ev.Path, labels, count = redactAuditURL(ev.Path, labels, count)
	switch mode {
	case contentBodyMetadata:
		_, labels, count = redactAuditText(ev.Body, cfg.ContentAuditRedactKeys, labels, count)
		_, labels, count = redactAuditText(ev.RespBody, cfg.ContentAuditRedactKeys, labels, count)
		ev.Body, ev.RespBody = "", ""
	case contentBodyRedacted:
		ev.Body, labels, count = redactAuditText(ev.Body, cfg.ContentAuditRedactKeys, labels, count)
		ev.RespBody, labels, count = redactAuditText(ev.RespBody, cfg.ContentAuditRedactKeys, labels, count)
	case contentBodyFull:
		// Explicit break-glass mode: bodies remain unchanged. URL credentials are
		// still removed because they have no forensic value in cleartext.
	}
	ev.RedactionLabels = uniqueSortedStrings(labels)
	ev.RedactionCount = count
	return true
}

func normalizeContentBodyMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case contentBodyMetadata:
		return contentBodyMetadata
	case contentBodyFull:
		return contentBodyFull
	default:
		return contentBodyRedacted
	}
}

func auditBodyHash(body string) string {
	if body == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func normalizeAuditHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		return strings.Trim(h, "[]")
	}
	return strings.Trim(host, "[]")
}

func matchesAnyAuditPattern(value string, patterns []string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	for _, raw := range patterns {
		p := strings.ToLower(strings.TrimSpace(raw))
		switch {
		case p == "*":
			return true
		case strings.HasPrefix(p, "*."):
			suffix := strings.TrimPrefix(p, "*")
			if strings.HasSuffix(value, suffix) || value == strings.TrimPrefix(suffix, ".") {
				return true
			}
		case strings.HasSuffix(p, "*"):
			if strings.HasPrefix(value, strings.TrimSuffix(p, "*")) {
				return true
			}
		case value == p:
			return true
		}
	}
	return false
}

func redactAuditText(text string, extraKeys []string, labels []string, count int) (string, []string, int) {
	if text == "" {
		return text, labels, count
	}
	keys := make(map[string]bool, len(defaultAuditRedactKeys)+len(extraKeys))
	for k := range defaultAuditRedactKeys {
		keys[k] = true
	}
	for _, k := range extraKeys {
		k = normalizeAuditKey(k)
		if k != "" {
			keys[k] = true
		}
	}

	var doc any
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	if dec.Decode(&doc) == nil {
		var trailing any
		if err := dec.Decode(&trailing); err == io.EOF {
			doc, labels, count = redactAuditJSON(doc, keys, labels, count)
			if b, err := json.Marshal(doc); err == nil {
				return string(b), labels, count
			}
		}
	}
	return redactAuditString(text, labels, count)
}

func redactAuditJSON(v any, keys map[string]bool, labels []string, count int) (any, []string, int) {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			if keys[normalizeAuditKey(k)] {
				x[k] = "[REDACTED]"
				labels = append(labels, "credential_field")
				count++
				continue
			}
			x[k], labels, count = redactAuditJSON(value, keys, labels, count)
		}
	case []any:
		for i := range x {
			x[i], labels, count = redactAuditJSON(x[i], keys, labels, count)
		}
	case string:
		return redactAuditString(x, labels, count)
	}
	return v, labels, count
}

func normalizeAuditKey(k string) string {
	k = strings.ToLower(strings.TrimSpace(k))
	k = strings.ReplaceAll(k, "-", "_")
	return k
}

func redactAuditString(s string, labels []string, count int) (string, []string, int) {
	for _, rule := range auditRedactRules {
		n := 0
		s = rule.re.ReplaceAllStringFunc(s, func(string) string {
			n++
			return "[REDACTED:" + rule.label + "]"
		})
		if n > 0 {
			labels = append(labels, rule.label)
			count += n
		}
	}
	return s, labels, count
}

func redactAuditURL(raw string, labels []string, count int) (string, []string, int) {
	if raw == "" {
		return raw, labels, count
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil || u.RawQuery == "" {
		return raw, labels, count
	}
	q := u.Query()
	changed := false
	for key := range q {
		if defaultAuditRedactKeys[normalizeAuditKey(key)] ||
			strings.Contains(normalizeAuditKey(key), "signature") {
			q.Set(key, "[REDACTED]")
			labels = append(labels, "query_credential")
			count++
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
		return u.String(), labels, count
	}
	return raw, labels, count
}

func uniqueSortedStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
