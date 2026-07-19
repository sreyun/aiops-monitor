package main

// 内容审计 DLP：扫描审计到的明文请求/响应，命中敏感数据(密钥/私钥/身份证/凭据/自定义关键词)即
// 打标签 + 告警。用于"是否有敏感数据外泄到大模型"。零依赖，纯 regexp/字符串。

import (
	"regexp"
	"strings"
	"sync"
)

type sensRule struct {
	label string
	re    *regexp.Regexp
}

// sensitiveRules 内置敏感数据正则（保守，尽量少误报）。Go regexp 无 lookaround，用 \b 定边界。
var sensitiveRules = []sensRule{
	{"LLM/OpenAI 密钥", regexp.MustCompile(`sk-[A-Za-z0-9_\-]{20,}`)},
	{"AWS AccessKey", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"私钥", regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)},
	{"JWT", regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`)},
	{"身份证号", regexp.MustCompile(`\b[1-9]\d{5}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[0-9Xx]\b`)},
	{"手机号", regexp.MustCompile(`\b1[3-9]\d{9}\b`)},
	{"凭据字段", regexp.MustCompile(`(?i)"(?:password|passwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key)"\s*:\s*"[^"]{3,}"`)},
}

// scanSensitive 扫描文本，返回命中的敏感类别标签(去重)。keywords 为用户自定义敏感词(大小写不敏感子串)。
func scanSensitive(text string, keywords []string) []string {
	if text == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(lbl string) {
		if !seen[lbl] {
			seen[lbl] = true
			out = append(out, lbl)
		}
	}
	for _, r := range sensitiveRules {
		if r.re.MatchString(text) {
			add(r.label)
		}
	}
	if len(keywords) > 0 {
		lower := strings.ToLower(text)
		for _, kw := range keywords {
			kw = strings.TrimSpace(kw)
			if kw != "" && strings.Contains(lower, strings.ToLower(kw)) {
				add("关键词:" + kw)
			}
		}
	}
	return out
}

// sensitiveSeverity：命中密钥/私钥/凭据 → critical(密钥外泄很严重)；其余(PII/关键词) → warning。
func sensitiveSeverity(hits []string) string {
	for _, h := range hits {
		if strings.Contains(h, "密钥") || strings.Contains(h, "私钥") || strings.Contains(h, "凭据") ||
			strings.Contains(h, "Key") || strings.Contains(h, "JWT") {
			return "critical"
		}
	}
	return "warning"
}

// ---- 告警去重：DLP 命中可能成批，同 (host|src|dst|labels) 在窗口内只告一次，防告警风暴 ----
var (
	contentAlertMu   sync.Mutex
	contentAlertSeen = map[string]int64{}
)

const contentAlertWindowSec = 300

// shouldAlertContent 在窗口内对同一告警键去重；now 由调用方传入(避免本文件直接取时间不便测试)。
func shouldAlertContent(key string, now int64) bool {
	contentAlertMu.Lock()
	defer contentAlertMu.Unlock()
	if last, ok := contentAlertSeen[key]; ok && now-last < contentAlertWindowSec {
		return false
	}
	contentAlertSeen[key] = now
	// 顺带清理过期键，防无界增长。
	if len(contentAlertSeen) > 10000 {
		for k, t := range contentAlertSeen {
			if now-t >= contentAlertWindowSec {
				delete(contentAlertSeen, k)
			}
		}
	}
	return true
}
