package main

import (
	"sort"
	"strings"
)

// OWASP Top 10 (2021) classification for web findings. Auditors and customers
// ask for results in Top-10 terms, so every finding is bucketed here rather
// than being left as an opaque template ID.

const (
	owaspA01 = "A01:2021 访问控制失效"
	owaspA02 = "A02:2021 加密机制失效"
	owaspA03 = "A03:2021 注入"
	owaspA04 = "A04:2021 不安全设计"
	owaspA05 = "A05:2021 安全配置错误"
	owaspA06 = "A06:2021 使用含已知漏洞的组件"
	owaspA07 = "A07:2021 身份识别和身份验证失败"
	owaspA08 = "A08:2021 软件和数据完整性失败"
	owaspA09 = "A09:2021 安全日志和监控失败"
	owaspA10 = "A10:2021 服务端请求伪造 (SSRF)"
)

// owaspKeywordRules maps a substring of the template ID / tags / name to a
// Top-10 bucket. Evaluated in order; first match wins, so put narrow rules first.
var owaspKeywordRules = []struct {
	keyword  string
	category string
}{
	{"ssrf", owaspA10},
	{"sqli", owaspA03}, {"sql-injection", owaspA03}, {"xss", owaspA03},
	{"rce", owaspA03}, {"command-injection", owaspA03}, {"ssti", owaspA03},
	{"xxe", owaspA03}, {"ldap-injection", owaspA03}, {"crlf", owaspA03},
	{"deserialization", owaspA08}, {"supply-chain", owaspA08}, {"subdomain-takeover", owaspA08},
	{"default-login", owaspA07}, {"weak-password", owaspA07}, {"brute", owaspA07},
	{"auth-bypass", owaspA07}, {"jwt", owaspA07}, {"session", owaspA07},
	{"lfi", owaspA01}, {"path-traversal", owaspA01}, {"idor", owaspA01},
	{"unauth", owaspA01}, {"cors", owaspA01}, {"clickjacking", owaspA01},
	{"tls", owaspA02}, {"ssl", owaspA02}, {"cert", owaspA02},
	{"cookie", owaspA02}, {"transport", owaspA02}, {"weak-cipher", owaspA02},
	// Keep this narrow: a bare "version" also appears in banner/disclosure
	// templates, which are configuration issues (A05), not vulnerable components.
	{"cve", owaspA06}, {"outdated", owaspA06}, {"vulnerable-version", owaspA06},
	{"logging", owaspA09}, {"log4j", owaspA06},
	{"exposure", owaspA05}, {"disclosure", owaspA05}, {"misconfig", owaspA05},
	{"headers", owaspA05}, {"panel", owaspA05}, {"listing", owaspA05},
	{"takeover", owaspA08}, {"csrf", owaspA01},
}

// builtinEnabled defaults ON so a fresh install produces results even before
// the Nuclei template tree is warmed up.
func (c WebSecurityConfig) builtinEnabled() bool { return !c.DisableBuiltin }

// webOWASPCategory classifies one finding.
func webOWASPCategory(f WebFinding) string {
	hay := strings.ToLower(strings.Join(append([]string{
		f.TemplateID, f.Name, f.Type, f.MatcherName,
	}, f.Tags...), " "))
	for _, r := range owaspKeywordRules {
		if strings.Contains(hay, r.keyword) {
			return r.category
		}
	}
	// Nuclei CVE templates are named CVE-YYYY-NNNN.
	if strings.Contains(hay, "cve-") {
		return owaspA06
	}
	return owaspA05 // configuration issues are the default bucket for DAST output
}

// owaspComplianceRefs maps a Top-10 bucket onto the audit frameworks.
func owaspComplianceRefs(cat string) []ComplianceRef {
	base := []ComplianceRef{{Framework: "OWASP", Control: strings.SplitN(cat, " ", 2)[0], Title: cat}}
	switch cat {
	case owaspA01:
		return append(base,
			cref("等保2.0", "8.1.4.3", "访问控制"),
			cref("PCI-DSS", "7.2.1", "Access control model"),
			cref("ISO27001", "A.8.3", "Information access restriction"))
	case owaspA02:
		return append(base,
			cref("等保2.0", "8.1.4.6", "数据保密性—传输加密"),
			cref("PCI-DSS", "4.2.1", "Strong cryptography in transit"),
			cref("ISO27001", "A.8.24", "Use of cryptography"))
	case owaspA03:
		return append(base,
			cref("等保2.0", "8.1.4.4", "入侵防范—数据有效性校验"),
			cref("PCI-DSS", "6.2.4", "Protect against common attacks"),
			cref("ISO27001", "A.8.28", "Secure coding"))
	case owaspA05:
		return append(base,
			cref("等保2.0", "8.1.4.4", "入侵防范—安全配置"),
			cref("PCI-DSS", "2.2.1", "Secure configuration standards"),
			cref("ISO27001", "A.8.9", "Configuration management"))
	case owaspA06:
		return append(base,
			cref("等保2.0", "8.1.4.4", "入侵防范—漏洞修补"),
			cref("PCI-DSS", "6.3.3", "Security patches installed"),
			cref("ISO27001", "A.8.8", "Technical vulnerabilities"))
	case owaspA07:
		return append(base,
			cref("等保2.0", "8.1.4.2", "身份鉴别"),
			cref("PCI-DSS", "8.3.1", "Strong authentication required"),
			cref("ISO27001", "A.5.17", "Authentication information"))
	case owaspA08:
		return append(base,
			cref("等保2.0", "8.1.4.6", "数据完整性"),
			cref("ISO27001", "A.8.9", "Configuration management"))
	case owaspA09:
		return append(base,
			cref("等保2.0", "8.1.4.5", "安全审计"),
			cref("PCI-DSS", "10.2.1", "Audit logs implemented"),
			cref("ISO27001", "A.8.15", "Logging"))
	case owaspA10:
		return append(base,
			cref("等保2.0", "8.1.4.4", "入侵防范"),
			cref("PCI-DSS", "6.2.4", "Protect against common attacks"))
	}
	return base
}

// annotateWebFindings stamps OWASP + compliance onto every finding in place.
func annotateWebFindings(findings []WebFinding) []WebFinding {
	for i := range findings {
		if findings[i].OWASP == "" {
			findings[i].OWASP = webOWASPCategory(findings[i])
		}
		if len(findings[i].Compliance) == 0 {
			findings[i].Compliance = owaspComplianceRefs(findings[i].OWASP)
		}
	}
	return findings
}

// summarizeWebOWASP counts open findings per Top-10 bucket.
func summarizeWebOWASP(findings []WebFinding) map[string]int {
	out := map[string]int{}
	for _, f := range findings {
		switch f.Status {
		case "resolved", "false_positive":
			continue
		}
		if f.Severity == "info" || f.OWASP == "" {
			continue
		}
		out[f.OWASP]++
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// summarizeWebCompliance counts distinct failing controls per framework.
func summarizeWebCompliance(findings []WebFinding) map[string]int {
	perFW := map[string]map[string]bool{}
	for _, f := range findings {
		switch f.Status {
		case "resolved", "false_positive":
			continue
		}
		if f.Severity == "info" {
			continue
		}
		for _, c := range f.Compliance {
			if perFW[c.Framework] == nil {
				perFW[c.Framework] = map[string]bool{}
			}
			perFW[c.Framework][c.Control] = true
		}
	}
	if len(perFW) == 0 {
		return nil
	}
	out := make(map[string]int, len(perFW))
	for fw, ctrls := range perFW {
		out[fw] = len(ctrls)
	}
	return out
}

// dedupeWebFindings drops duplicates when two engines report the same issue.
func dedupeWebFindings(in []WebFinding) []WebFinding {
	seen := map[string]bool{}
	out := make([]WebFinding, 0, len(in))
	for _, f := range in {
		key := strings.ToLower(strings.Join([]string{
			f.TemplateID, f.MatcherName, f.URL, f.MatchedAt,
		}, "|"))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	// findingLevelRank is "higher is worse", so sort descending to put the
	// findings that need action first.
	sort.SliceStable(out, func(i, j int) bool {
		return webSeverityRank(out[i].Severity) > webSeverityRank(out[j].Severity)
	})
	return out
}
