package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Built-in web checks run without Nuclei. They cover the passive/config layer
// that every commercial DAST reports on — TLS posture, security headers, cookie
// flags, CORS, HTTP methods and well-known sensitive path exposure — so a scan
// still produces actionable results when the Nuclei engine or its templates are
// unavailable, and so those classes are covered deterministically rather than
// depending on template availability.

const (
	builtinCheckTimeout   = 15 * time.Second
	builtinMaxBody        = 256 << 10
	builtinCertWarnDays   = 30
	builtinCertNoticeDays = 90
)

// builtinScanContext carries everything the checks need for one target.
type builtinScanContext struct {
	target       WebScanTarget
	allowPrivate bool
	authHeaders  []string
	client       *http.Client
}

func newBuiltinScanContext(t WebScanTarget, allowPrivate bool, authHeaders []string) *builtinScanContext {
	return &builtinScanContext{
		target:       t,
		allowPrivate: allowPrivate,
		authHeaders:  authHeaders,
		client: &http.Client{
			Timeout: builtinCheckTimeout,
			// Never auto-follow: a redirect target may resolve to a private
			// address, and header/cookie checks must observe the FIRST response.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (b *builtinScanContext) do(ctx context.Context, method, rawURL string, extra map[string]string) (*http.Response, []byte, error) {
	if err := assertURLAllowed(rawURL, b.allowPrivate); err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "AIOps-Monitor-WebScan/1.0")
	for _, h := range b.authHeaders {
		if i := strings.Index(h, ":"); i > 0 {
			req.Header.Set(strings.TrimSpace(h[:i]), strings.TrimSpace(h[i+1:]))
		}
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, builtinMaxBody))
	_ = resp.Body.Close()
	return resp, body, nil
}

func builtinFinding(id, name, severity, target, desc, remedy string, tags ...string) WebFinding {
	return WebFinding{
		TemplateID:  "builtin/" + id,
		Name:        name,
		Severity:    severity,
		URL:         target,
		MatchedAt:   target,
		Description: desc,
		Remediation: remedy,
		Type:        "http",
		Tags:        append([]string{"builtin"}, tags...),
	}
}

// runBuiltinWebChecks executes every built-in check against the target.
func runBuiltinWebChecks(ctx context.Context, t WebScanTarget, allowPrivate bool, authHeaders []string) []WebFinding {
	b := newBuiltinScanContext(t, allowPrivate, authHeaders)
	base := strings.TrimRight(strings.TrimSpace(t.BaseURL), "/")
	if base == "" {
		return nil
	}
	var out []WebFinding
	resp, body, err := b.do(ctx, http.MethodGet, base, nil)
	if err != nil {
		return []WebFinding{builtinFinding("unreachable", "目标不可达", "info", base,
			"内置检测无法访问目标："+err.Error(),
			"确认目标可从服务端出网访问、DNS 解析正常、且未被 WAF 阻断", "recon")}
	}
	out = append(out, checkSecurityHeaders(base, resp)...)
	out = append(out, checkCookieFlags(base, resp)...)
	out = append(out, checkInfoDisclosure(base, resp, body)...)
	out = append(out, checkTLSPosture(ctx, base)...)
	out = append(out, checkHTTPSRedirect(ctx, b, base)...)
	out = append(out, checkCORS(ctx, b, base)...)
	out = append(out, checkHTTPMethods(ctx, b, base)...)
	out = append(out, checkExposedPaths(ctx, b, base)...)
	out = append(out, checkDBMSErrorLeak(base, body)...)
	out = append(out, checkErrorBasedSQLi(ctx, b, base)...)
	return out
}

// --- security headers ---

type headerCheck struct {
	Header   string
	ID       string
	Name     string
	Severity string
	Desc     string
	Remedy   string
}

var securityHeaderChecks = []headerCheck{
	{"Content-Security-Policy", "missing-csp", "缺少 Content-Security-Policy", "medium",
		"未设置 CSP，浏览器无法限制脚本/资源来源，XSS 一旦发生即可完全利用。",
		"下发 Content-Security-Policy，至少限定 default-src 'self' 与 script-src，避免 unsafe-inline"},
	{"Strict-Transport-Security", "missing-hsts", "缺少 Strict-Transport-Security", "medium",
		"HTTPS 站点未启用 HSTS，用户首次访问仍可能被降级到 HTTP 并遭中间人劫持。",
		"响应头加入 Strict-Transport-Security: max-age=31536000; includeSubDomains"},
	{"X-Content-Type-Options", "missing-xcto", "缺少 X-Content-Type-Options", "low",
		"未禁止 MIME 嗅探，浏览器可能把上传文件当作脚本执行。",
		"响应头加入 X-Content-Type-Options: nosniff"},
	{"Referrer-Policy", "missing-referrer-policy", "缺少 Referrer-Policy", "low",
		"未限制 Referer，跳转到外部站点时可能泄露内部路径与令牌。",
		"响应头加入 Referrer-Policy: strict-origin-when-cross-origin"},
	{"Permissions-Policy", "missing-permissions-policy", "缺少 Permissions-Policy", "info",
		"未限制摄像头/麦克风/地理位置等浏览器特性的调用范围。",
		"按需下发 Permissions-Policy，例如 geolocation=(), camera=(), microphone=()"},
}

func checkSecurityHeaders(base string, resp *http.Response) []WebFinding {
	var out []WebFinding
	isHTTPS := strings.HasPrefix(strings.ToLower(base), "https://")
	for _, c := range securityHeaderChecks {
		if resp.Header.Get(c.Header) != "" {
			continue
		}
		// HSTS is meaningless (and ignored by browsers) over plain HTTP.
		if c.Header == "Strict-Transport-Security" && !isHTTPS {
			continue
		}
		out = append(out, builtinFinding(c.ID, c.Name, c.Severity, base, c.Desc, c.Remedy,
			"headers", "misconfig"))
	}
	// Clickjacking needs EITHER a frame-ancestors CSP directive or X-Frame-Options.
	csp := strings.ToLower(resp.Header.Get("Content-Security-Policy"))
	if resp.Header.Get("X-Frame-Options") == "" && !strings.Contains(csp, "frame-ancestors") {
		out = append(out, builtinFinding("clickjacking", "可被 iframe 嵌套（点击劫持）", "medium", base,
			"既未设置 X-Frame-Options，CSP 中也没有 frame-ancestors，页面可被第三方站点嵌入实施点击劫持。",
			"设置 X-Frame-Options: SAMEORIGIN，或在 CSP 中加入 frame-ancestors 'self'",
			"headers", "clickjacking"))
	}
	if v := strings.ToLower(resp.Header.Get("Access-Control-Allow-Origin")); v == "*" {
		out = append(out, builtinFinding("cors-wildcard", "CORS 允许任意来源", "low", base,
			"Access-Control-Allow-Origin: *，任何站点的脚本都可读取该响应。",
			"改为按白名单回显具体 Origin；若响应含敏感数据务必收敛",
			"cors", "misconfig"))
	}
	return out
}

// --- cookies ---

func checkCookieFlags(base string, resp *http.Response) []WebFinding {
	isHTTPS := strings.HasPrefix(strings.ToLower(base), "https://")
	var insecure, noHTTPOnly, noSameSite []string
	for _, c := range resp.Cookies() {
		if isHTTPS && !c.Secure {
			insecure = append(insecure, c.Name)
		}
		if !c.HttpOnly {
			noHTTPOnly = append(noHTTPOnly, c.Name)
		}
		// An absent SameSite attribute leaves the zero value, not
		// SameSiteDefaultMode — only Lax/Strict actually mitigate CSRF.
		if c.SameSite != http.SameSiteLaxMode && c.SameSite != http.SameSiteStrictMode {
			noSameSite = append(noSameSite, c.Name)
		}
	}
	var out []WebFinding
	if len(insecure) > 0 {
		out = append(out, builtinFinding("cookie-no-secure", "Cookie 缺少 Secure 属性", "medium", base,
			"HTTPS 站点下发的以下 Cookie 未设置 Secure，可能经明文 HTTP 泄露："+strings.Join(insecure, ", "),
			"为所有会话 Cookie 添加 Secure 属性", "cookie", "misconfig"))
	}
	if len(noHTTPOnly) > 0 {
		out = append(out, builtinFinding("cookie-no-httponly", "Cookie 缺少 HttpOnly 属性", "medium", base,
			"以下 Cookie 可被 JavaScript 读取，XSS 时会话可被直接窃取："+strings.Join(noHTTPOnly, ", "),
			"为会话类 Cookie 添加 HttpOnly 属性", "cookie", "misconfig"))
	}
	if len(noSameSite) > 0 {
		out = append(out, builtinFinding("cookie-no-samesite", "Cookie 缺少 SameSite 限制", "low", base,
			"以下 Cookie 未设置 SameSite=Lax/Strict，存在 CSRF 风险："+strings.Join(noSameSite, ", "),
			"设置 SameSite=Lax（或 Strict）；确需跨站时同时启用 Secure", "cookie", "csrf"))
	}
	return out
}

// --- information disclosure ---

var disclosureHeaders = []string{"Server", "X-Powered-By", "X-AspNet-Version", "X-AspNetMvc-Version", "X-Generator"}

func checkInfoDisclosure(base string, resp *http.Response, body []byte) []WebFinding {
	var leaks []string
	for _, h := range disclosureHeaders {
		v := strings.TrimSpace(resp.Header.Get(h))
		if v == "" {
			continue
		}
		// A bare product name without a version is low value for an attacker.
		if h == "Server" && !strings.ContainsAny(v, "0123456789") {
			continue
		}
		leaks = append(leaks, h+": "+v)
	}
	var out []WebFinding
	if len(leaks) > 0 {
		out = append(out, builtinFinding("version-disclosure", "响应头泄露组件版本", "low", base,
			"以下响应头暴露了中间件/框架版本，便于攻击者精确匹配已知漏洞："+strings.Join(leaks, "; "),
			"在反向代理层移除或改写 Server / X-Powered-By 等版本标识头",
			"disclosure", "fingerprint"))
	}
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "<title>index of /") || strings.Contains(lower, "directory listing for") {
		out = append(out, builtinFinding("directory-listing", "开启了目录列表", "medium", base,
			"响应内容为目录索引页面，会暴露站点文件结构与备份文件。",
			"关闭 autoindex/DirectoryIndex，或为目录提供默认页面",
			"disclosure", "misconfig"))
	}
	return out
}

// --- TLS ---

func checkTLSPosture(ctx context.Context, base string) []WebFinding {
	u, err := url.Parse(base)
	if err != nil || !strings.EqualFold(u.Scheme, "https") {
		if err == nil && strings.EqualFold(u.Scheme, "http") {
			return []WebFinding{builtinFinding("plaintext-http", "站点使用明文 HTTP", "high", base,
				"目标以 http:// 提供服务，凭据与会话在传输过程中完全可被窃听或篡改。",
				"部署 TLS 证书并将全站强制跳转到 HTTPS，同时启用 HSTS",
				"tls", "transport")}
		}
		return nil
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "443")
	}
	dialer := &net.Dialer{Timeout: builtinCheckTimeout}
	// InsecureSkipVerify is required to *inspect* a broken chain; validity is
	// then evaluated explicitly below instead of being silently accepted.
	conn, err := tls.DialWithDialer(dialer, "tcp", host, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         u.Hostname(),
		MinVersion:         tls.VersionTLS10,
	})
	if err != nil {
		return []WebFinding{builtinFinding("tls-handshake", "TLS 握手失败", "info", base,
			"无法完成 TLS 握手："+err.Error(),
			"检查证书链、SNI 配置与受支持的 TLS 版本", "tls")}
	}
	defer conn.Close()
	state := conn.ConnectionState()

	var out []WebFinding
	if state.Version < tls.VersionTLS12 {
		out = append(out, builtinFinding("tls-old-version", "启用了过时的 TLS 版本", "high", base,
			"协商到 "+tlsVersionName(state.Version)+"，该版本存在已知密码学缺陷且不符合 PCI-DSS 要求。",
			"仅保留 TLS 1.2/1.3，禁用 SSLv3/TLS 1.0/1.1", "tls", "transport"))
	}
	if len(state.PeerCertificates) > 0 {
		leaf := state.PeerCertificates[0]
		now := time.Now()
		switch {
		case now.After(leaf.NotAfter):
			out = append(out, builtinFinding("tls-cert-expired", "TLS 证书已过期", "critical", base,
				fmt.Sprintf("证书于 %s 过期（CN=%s）。浏览器会直接拦截访问。",
					leaf.NotAfter.Format("2006-01-02"), leaf.Subject.CommonName),
				"立即续期证书并配置自动续期（ACME/内部 CA 自动化）", "tls", "cert"))
		case now.AddDate(0, 0, builtinCertWarnDays).After(leaf.NotAfter):
			out = append(out, builtinFinding("tls-cert-expiring", "TLS 证书即将过期", "high", base,
				fmt.Sprintf("证书将于 %s 过期，剩余 %d 天。",
					leaf.NotAfter.Format("2006-01-02"), int(time.Until(leaf.NotAfter).Hours()/24)),
				"尽快续期并接入自动续期与到期告警", "tls", "cert"))
		case now.AddDate(0, 0, builtinCertNoticeDays).After(leaf.NotAfter):
			out = append(out, builtinFinding("tls-cert-expiry-notice", "TLS 证书 90 天内到期", "info", base,
				fmt.Sprintf("证书到期日 %s。", leaf.NotAfter.Format("2006-01-02")),
				"纳入证书生命周期管理，避免临期抢修", "tls", "cert"))
		}
		if err := leaf.VerifyHostname(u.Hostname()); err != nil {
			out = append(out, builtinFinding("tls-hostname-mismatch", "TLS 证书域名不匹配", "high", base,
				"证书不包含访问所用域名："+err.Error(),
				"签发覆盖该域名（或含对应 SAN）的证书", "tls", "cert"))
		}
		if verifyErr := verifyCertChain(state, u.Hostname()); verifyErr != nil {
			out = append(out, builtinFinding("tls-untrusted-chain", "TLS 证书链不受信任", "high", base,
				"证书链校验失败："+verifyErr.Error()+"（自签名或缺少中间证书）",
				"部署由受信任 CA 签发的证书，并补齐中间证书链", "tls", "cert"))
		}
		if leaf.PublicKeyAlgorithm == x509.RSA && leaf.SignatureAlgorithm == x509.SHA1WithRSA {
			out = append(out, builtinFinding("tls-weak-signature", "证书使用 SHA-1 签名", "high", base,
				"SHA-1 已被证明可碰撞，签名不再可信。", "改用 SHA-256 及以上签名算法重新签发", "tls", "cert"))
		}
	}
	return out
}

func verifyCertChain(state tls.ConnectionState, host string) error {
	if len(state.PeerCertificates) == 0 {
		return fmt.Errorf("无证书")
	}
	inter := x509.NewCertPool()
	for _, c := range state.PeerCertificates[1:] {
		inter.AddCert(c)
	}
	_, err := state.PeerCertificates[0].Verify(x509.VerifyOptions{
		DNSName:       host,
		Intermediates: inter,
	})
	// Hostname problems are reported separately; don't double-count them here.
	if _, ok := err.(x509.HostnameError); ok {
		return nil
	}
	return err
}

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	}
	return fmt.Sprintf("0x%04x", v)
}

func checkHTTPSRedirect(ctx context.Context, b *builtinScanContext, base string) []WebFinding {
	u, err := url.Parse(base)
	if err != nil || !strings.EqualFold(u.Scheme, "https") {
		return nil
	}
	plain := *u
	plain.Scheme = "http"
	if plain.Port() == "443" {
		plain.Host = plain.Hostname()
	}
	resp, _, err := b.do(ctx, http.MethodGet, plain.String(), nil)
	if err != nil {
		return nil // HTTP port closed is the desired state
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		if loc := resp.Header.Get("Location"); strings.HasPrefix(strings.ToLower(loc), "https://") {
			return nil
		}
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return []WebFinding{builtinFinding("no-https-redirect", "HTTP 未强制跳转 HTTPS", "medium", plain.String(),
			fmt.Sprintf("http:// 入口直接返回 %d 而非跳转 HTTPS，用户可能全程走明文。", resp.StatusCode),
			"在 80 端口配置 301 到 https://，并启用 HSTS 防止降级", "tls", "transport")}
	}
	return nil
}

// --- CORS ---

func checkCORS(ctx context.Context, b *builtinScanContext, base string) []WebFinding {
	probe := "https://aiops-cors-probe.example"
	resp, _, err := b.do(ctx, http.MethodGet, base, map[string]string{"Origin": probe})
	if err != nil {
		return nil
	}
	allow := strings.TrimSpace(resp.Header.Get("Access-Control-Allow-Origin"))
	creds := strings.EqualFold(strings.TrimSpace(resp.Header.Get("Access-Control-Allow-Credentials")), "true")
	if allow == "" {
		return nil
	}
	reflected := strings.EqualFold(allow, probe)
	switch {
	case reflected && creds:
		return []WebFinding{builtinFinding("cors-reflect-credentials", "CORS 回显任意 Origin 且允许携带凭据", "high", base,
			"服务端把任意 Origin 原样回显并设置 Access-Control-Allow-Credentials: true，任何第三方站点都能以受害者身份读取接口数据。",
			"改为服务端白名单校验 Origin，拒绝未知来源；确需凭据时禁止通配与回显",
			"cors", "misconfig")}
	case reflected:
		return []WebFinding{builtinFinding("cors-reflect-origin", "CORS 回显任意 Origin", "medium", base,
			"服务端把任意 Origin 原样回显到 Access-Control-Allow-Origin。",
			"改为白名单匹配后再回显", "cors", "misconfig")}
	case allow == "*" && creds:
		return []WebFinding{builtinFinding("cors-wildcard-credentials", "CORS 通配来源且允许凭据", "high", base,
			"Access-Control-Allow-Origin: * 与 Allow-Credentials: true 组合属于危险配置。",
			"禁用通配，改为白名单 Origin", "cors", "misconfig")}
	}
	return nil
}

// --- HTTP methods ---

var riskyMethods = map[string]string{
	"TRACE":   "可用于跨站追踪（XST），窃取 HttpOnly Cookie",
	"TRACK":   "IIS 的 TRACE 变体，同样可用于跨站追踪",
	"PUT":     "允许写入文件，可能导致 WebShell 上传",
	"DELETE":  "允许删除服务端资源",
	"CONNECT": "可能被滥用为代理隧道",
}

func checkHTTPMethods(ctx context.Context, b *builtinScanContext, base string) []WebFinding {
	resp, _, err := b.do(ctx, http.MethodOptions, base, nil)
	if err != nil {
		return nil
	}
	allow := resp.Header.Get("Allow")
	if allow == "" {
		allow = resp.Header.Get("Access-Control-Allow-Methods")
	}
	var risky []string
	for _, m := range strings.Split(allow, ",") {
		m = strings.ToUpper(strings.TrimSpace(m))
		if desc, ok := riskyMethods[m]; ok {
			risky = append(risky, m+"（"+desc+"）")
		}
	}
	if len(risky) == 0 {
		return nil
	}
	sev := "medium"
	if strings.Contains(strings.Join(risky, " "), "PUT") || strings.Contains(strings.Join(risky, " "), "DELETE") {
		sev = "high"
	}
	return []WebFinding{builtinFinding("risky-http-methods", "启用了高风险 HTTP 方法", sev, base,
		"OPTIONS 响应声明支持："+strings.Join(risky, "；"),
		"在中间件层仅放行 GET/POST/HEAD 等业务必需方法", "misconfig", "methods")}
}

// --- exposed sensitive paths ---

// exposedPath is matched on CONTENT, not status code: many sites return 200
// with an SPA shell for unknown paths, which would make a status-only check
// report false positives everywhere.
type exposedPath struct {
	Path     string
	Marker   string         // plain substring the real file always contains
	Pattern  *regexp.Regexp // used when a substring is too weak to be conclusive
	ID       string
	Name     string
	Severity string
	Desc     string
	Remedy   string
}

var exposedPaths = []exposedPath{
	{Path: "/.git/HEAD", Marker: "ref:", ID: "git-exposed", Name: "暴露 .git 目录", Severity: "critical",
		Desc:   "可通过 .git 目录还原完整源码与历史提交（含可能的密钥）。",
		Remedy: "在 Web 服务器中拒绝 /.git 路径，并从发布产物中剔除版本控制目录"},
	// A bare "=" would match almost any page, so require real KEY=VALUE lines.
	{Path: "/.env", Pattern: regexp.MustCompile(`(?m)^[A-Za-z_][A-Za-z0-9_]*[ \t]*=`),
		ID: "env-exposed", Name: "暴露 .env 配置文件", Severity: "critical",
		Desc:   ".env 通常包含数据库口令、密钥与第三方凭据。",
		Remedy: "立即下线该文件、轮换其中所有凭据，并禁止 Web 目录暴露点文件"},
	{Path: "/.svn/entries", Pattern: regexp.MustCompile(`(?m)\Adir\b|\A\d+\s`),
		ID: "svn-exposed", Name: "暴露 .svn 目录", Severity: "high",
		Desc: "可还原源码与目录结构。", Remedy: "拒绝 /.svn 路径访问"},
	{Path: "/server-status", Marker: "Apache Server Status", ID: "apache-status",
		Name: "暴露 Apache server-status", Severity: "medium",
		Desc: "页面泄露活动请求、客户端 IP 与内部路径。", Remedy: "限制 mod_status 仅本机访问"},
	{Path: "/actuator/env", Marker: "\"propertySources\"", ID: "actuator-env",
		Name: "暴露 Spring Actuator env", Severity: "critical",
		Desc:   "/actuator/env 会泄露全部配置项，常含明文口令。",
		Remedy: "关闭敏感 actuator 端点或加认证：management.endpoints.web.exposure.include"},
	{Path: "/actuator/heapdump", Marker: "JAVA PROFILE", ID: "actuator-heapdump",
		Name: "暴露 Spring Actuator heapdump", Severity: "critical",
		Desc: "heapdump 可离线提取内存中的凭据与会话。", Remedy: "禁用 heapdump 端点"},
	{Path: "/phpinfo.php", Marker: "phpinfo()", ID: "phpinfo",
		Name: "暴露 phpinfo 页面", Severity: "medium",
		Desc: "泄露 PHP 配置、路径与扩展信息。", Remedy: "删除调试用 phpinfo 页面"},
	{Path: "/.DS_Store", Marker: "Bud1", ID: "ds-store", Name: "暴露 .DS_Store", Severity: "low",
		Desc: "可还原目录文件名清单。", Remedy: "从发布产物中剔除 .DS_Store"},
	{Path: "/web.config", Marker: "<configuration", ID: "webconfig",
		Name: "暴露 web.config", Severity: "high",
		Desc: "IIS 配置文件可能包含连接串与密钥。", Remedy: "禁止直接下载 .config 文件"},
	{Path: "/.aws/credentials", Marker: "aws_access_key_id", ID: "aws-creds",
		Name: "暴露 AWS 凭据文件", Severity: "critical",
		Desc: "可直接接管云账号资源。", Remedy: "立即删除并轮换 AK/SK，检查云上审计日志"},
}

func checkExposedPaths(ctx context.Context, b *builtinScanContext, base string) []WebFinding {
	var out []WebFinding
	for _, p := range exposedPaths {
		if ctx.Err() != nil {
			break
		}
		resp, body, err := b.do(ctx, http.MethodGet, base+p.Path, nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		if len(body) == 0 {
			continue
		}
		// An HTML page at a non-HTML path is the SPA fallback, not the real file.
		if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
			continue
		}
		if p.Marker != "" && !strings.Contains(string(body), p.Marker) {
			continue
		}
		if p.Pattern != nil && !p.Pattern.Match(body) {
			continue
		}
		out = append(out, builtinFinding(p.ID, p.Name, p.Severity, base+p.Path, p.Desc, p.Remedy,
			"exposure", "disclosure"))
	}
	return out
}
