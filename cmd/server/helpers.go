package main

import (
	"net"
	"net/http"
	"strings"
	"unicode"
)

// clientIP extracts the operator's IP for the activity log.
// clientIP returns the request's client address for audit logs and login
// rate-limiting. Reverse-proxy headers (X-Real-IP / X-Forwarded-For) are honored
// ONLY when trust_proxy is enabled — otherwise they are attacker-forgeable and a
// directly-exposed server would let anyone reset their rate-limit bucket (and
// forge audit-log origins) by spoofing a header, so we use the raw connection
// address instead.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy() {
		if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
			return xr
		}
		if f := r.Header.Get("X-Forwarded-For"); f != "" {
			// Last hop is the address our trusted proxy actually saw (nginx appends
			// $remote_addr); the client-controlled prefix is not trusted.
			parts := strings.Split(f, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				return ip
			}
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// isHTTPS reports whether the request reached us over TLS, honoring the common
// reverse-proxy header. Used to set the Secure flag on the session cookie.
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// serverURL reconstructs the externally-reachable base URL from the request,
// honoring common reverse-proxy headers.
func serverURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = p
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	return scheme + "://" + host
}

// ---- secret masking helpers ----

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

// mergeSecrets keeps existing webhook/secret values when the incoming ones are
// blank or still masked, so the panel can submit without re-typing secrets.
func mergeSecrets(in *ServerConfig, old ServerConfig) {
	in.Feishu.Webhook = keepIfBlank(in.Feishu.Webhook, old.Feishu.Webhook)
	in.Dingtalk.Webhook = keepIfBlank(in.Dingtalk.Webhook, old.Dingtalk.Webhook)
	in.Dingtalk.Secret = keepIfBlank(in.Dingtalk.Secret, old.Dingtalk.Secret)
	in.CustomWebhook.URL = keepIfBlank(in.CustomWebhook.URL, old.CustomWebhook.URL)
	in.SMTP.Password = keepIfBlank(in.SMTP.Password, old.SMTP.Password)
	if in.SMTP.FromName == "" {
		in.SMTP.FromName = old.SMTP.FromName
	}
	// Custom webhook headers may carry auth tokens and are masked in GET responses;
	// restore the stored value when the browser submits a blank/masked placeholder.
	in.CustomWebhook.Headers = keepIfBlank(in.CustomWebhook.Headers, old.CustomWebhook.Headers)
	// 短信 / 语音的 AccessKey + SecretKey 在 GET 里被 maskSecret 脱敏（如 LTAI****GHIJ）。
	// 表单回传脱敏串时必须还原为原值——否则「发送测试」或再次保存会拿脱敏串当真实凭证去做
	// ACS3-HMAC-SHA256 签名，导致阿里云返回 SignatureDoesNotMatch / InvalidAccessKeyId。
	in.SMS.AccessKey = keepIfBlank(in.SMS.AccessKey, old.SMS.AccessKey)
	in.SMS.SecretKey = keepIfBlank(in.SMS.SecretKey, old.SMS.SecretKey)
	in.VoiceCall.AccessKey = keepIfBlank(in.VoiceCall.AccessKey, old.VoiceCall.AccessKey)
	in.VoiceCall.SecretKey = keepIfBlank(in.VoiceCall.SecretKey, old.VoiceCall.SecretKey)
}

func keepIfBlank(newv, oldv string) string {
	t := strings.TrimSpace(newv)
	if t == "" || strings.Contains(t, "****") {
		return oldv
	}
	return newv
}

// smsSafeVar 清洗要塞进短信模板变量的文本，使其符合阿里云短信内容审核。
// 阿里云对变量内容有严格限制：不支持 emoji、换行、【】（签名专用）及多数特殊符号，
// 且单个变量长度有限——否则报 isv.UNSUPPORTED_SMS_CONTENT。这里：换行/制表→空格，
// 只保留 中文/字母/数字/常用标点，丢弃 emoji 等其它符号，折叠空白并截断到 45 字。
func smsSafeVar(s string) string {
	s = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ' ',
			r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			unicode.Is(unicode.Han, r):
			b.WriteRune(r)
		case strings.ContainsRune("，。：；、！？（）().,:;-/_%", r):
			b.WriteRune(r)
		default:
			// 丢弃 emoji / 其它特殊符号（如 ✅ ★ 【 】）
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ") // 折叠多余空白
	if rs := []rune(out); len(rs) > 45 {                 // 单变量保守长度上限
		out = string(rs[:45])
	}
	return out
}

// ---- install-script parameter sanitizers ----
// /install.sh and /install.ps1 are public and echo these query params into a
// shell/PowerShell script that a machine pipes straight to sh/iex. Any of them
// could otherwise carry quotes/`$`/backticks/`;` that break out of the quoted
// assignment and inject commands, so each is reduced to a safe charset. Real
// values (hex token, a URL, a category name) are unaffected.

func sanitizeToken(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 128 {
		s = s[:128]
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return -1
		}
	}, s)
}

func sanitizeCategory(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.', r == ' ':
			return r
		case unicode.Is(unicode.Han, r):
			return r
		default:
			return -1
		}
	}, strings.TrimSpace(s))
	if rs := []rune(s); len(rs) > 48 {
		s = string(rs[:48])
	}
	return s
}

func sanitizeServerURL(u string) string {
	u = strings.TrimSpace(u)
	if len(u) > 256 {
		u = u[:256]
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case strings.ContainsRune(":/._-", r):
			return r
		default:
			return -1
		}
	}, u)
}

// sanitizeUsername validates the login username: 2–32 chars of ASCII letters,
// digits, dot, dash or underscore. Returns "" when invalid.
func sanitizeUsername(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 || len(s) > 32 {
		return ""
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_'
		if !ok {
			return ""
		}
	}
	return s
}
