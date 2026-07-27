package main

import (
	"sort"
	"strings"
)

// Compliance mapping turns raw findings into audit-ready evidence: each finding
// carries the control it violates in the frameworks customers get audited
// against (CIS Benchmarks, 中国等保 2.0 三级, PCI-DSS v4, ISO/IEC 27001:2022).
//
// The mapping is intentionally conservative — a finding without a confident
// control match stays unmapped rather than claiming coverage it does not have.

// ComplianceRef is one control a finding maps to.
type ComplianceRef struct {
	Framework string `json:"framework"`
	Control   string `json:"control"`
	Title     string `json:"title,omitempty"`
}

type complianceRule struct {
	// prefix matches the finding ID from its start; the longest match wins.
	prefix string
	refs   []ComplianceRef
}

func cref(fw, ctrl, title string) ComplianceRef {
	return ComplianceRef{Framework: fw, Control: ctrl, Title: title}
}

// complianceRules maps finding-ID prefixes to controls. Ordered by specificity
// at lookup time, not here.
var complianceRules = []complianceRule{
	// --- identity & access ---
	{"acct_uid0_alias", []ComplianceRef{
		cref("CIS", "6.2.9", "Ensure root is the only UID 0 account"),
		cref("等保2.0", "8.1.4.2", "身份鉴别—唯一标识"),
		cref("PCI-DSS", "8.2.1", "Unique ID for each user"),
		cref("ISO27001", "A.5.16", "Identity management"),
	}},
	{"acct_dup_uid", []ComplianceRef{
		cref("CIS", "6.2.5", "Ensure no duplicate UIDs exist"),
		cref("等保2.0", "8.1.4.2", "身份鉴别—唯一标识"),
		cref("PCI-DSS", "8.2.1", "Unique ID for each user"),
	}},
	{"acct_empty_password", []ComplianceRef{
		cref("CIS", "6.2.1", "Ensure accounts use passwords"),
		cref("等保2.0", "8.1.4.2", "身份鉴别—口令复杂度"),
		cref("PCI-DSS", "8.3.1", "Strong authentication required"),
		cref("ISO27001", "A.5.17", "Authentication information"),
	}},
	{"acct_password_never_expires", []ComplianceRef{
		cref("CIS", "5.5.1.1", "Ensure password expiration is configured"),
		cref("等保2.0", "8.1.4.2", "身份鉴别—定期更换"),
		cref("PCI-DSS", "8.3.9", "Password change frequency"),
	}},
	{"acct_pass_max_days", []ComplianceRef{
		cref("CIS", "5.5.1.1", "Password expiration days"),
		cref("等保2.0", "8.1.4.2", "身份鉴别—定期更换"),
	}},
	{"acct_pass_min_len", []ComplianceRef{
		cref("CIS", "5.4.1", "Password creation requirements"),
		cref("等保2.0", "8.1.4.2", "身份鉴别—口令复杂度"),
		cref("PCI-DSS", "8.3.6", "Minimum password length"),
	}},
	{"acct_system_shell", []ComplianceRef{
		cref("CIS", "5.5.2", "Ensure system accounts are non-login"),
		cref("等保2.0", "8.1.4.3", "访问控制—最小权限"),
	}},
	{"win_pass_min_len", []ComplianceRef{
		cref("CIS", "1.1.4", "Minimum password length"),
		cref("PCI-DSS", "8.3.6", "Minimum password length"),
		cref("等保2.0", "8.1.4.2", "身份鉴别—口令复杂度"),
	}},
	{"win_no_lockout", []ComplianceRef{
		cref("CIS", "1.2.1", "Account lockout threshold"),
		cref("PCI-DSS", "8.3.4", "Limit repeated access attempts"),
		cref("等保2.0", "8.1.4.2", "身份鉴别—登录失败处理"),
	}},
	{"win_guest_enabled", []ComplianceRef{
		cref("CIS", "2.3.1.3", "Guest account status"),
		cref("等保2.0", "8.1.4.3", "访问控制—默认账户"),
	}},
	{"win_admin_sprawl", []ComplianceRef{
		cref("CIS", "2.2.2", "Administrators group membership"),
		cref("等保2.0", "8.1.4.3", "访问控制—最小权限"),
		cref("ISO27001", "A.8.2", "Privileged access rights"),
	}},

	// --- privilege escalation ---
	{"sudo_nopasswd", []ComplianceRef{
		cref("CIS", "5.2.3", "Ensure sudo requires authentication"),
		cref("等保2.0", "8.1.4.3", "访问控制—最小权限"),
		cref("ISO27001", "A.8.2", "Privileged access rights"),
	}},
	{"sudo_unrestricted", []ComplianceRef{
		cref("CIS", "5.2.2", "Restrict sudo command scope"),
		cref("等保2.0", "8.1.4.3", "访问控制—最小权限"),
	}},
	{"suid_interpreter", []ComplianceRef{
		cref("CIS", "6.1.13", "Audit SUID/SGID executables"),
		cref("等保2.0", "8.1.4.4", "入侵防范—最小安装"),
		cref("ISO27001", "A.8.19", "Software on operational systems"),
	}},
	{"suid_unexpected", []ComplianceRef{
		cref("CIS", "6.1.13", "Audit SUID/SGID executables"),
		cref("等保2.0", "8.1.4.4", "入侵防范—最小安装"),
	}},
	{"win_uac_off", []ComplianceRef{
		cref("CIS", "2.3.17.1", "UAC admin approval mode"),
		cref("等保2.0", "8.1.4.3", "访问控制—最小权限"),
	}},
	{"win_autologon", []ComplianceRef{
		cref("CIS", "18.9.28", "Do not store credentials"),
		cref("PCI-DSS", "8.3.1", "Strong authentication required"),
		cref("等保2.0", "8.1.4.2", "身份鉴别"),
	}},

	// --- remote access & network exposure ---
	{"ssh_root_login", []ComplianceRef{
		cref("CIS", "5.1.7", "Disable SSH root login"),
		cref("等保2.0", "8.1.4.3", "访问控制—远程管理"),
		cref("PCI-DSS", "2.2.7", "Secure administrative access"),
	}},
	{"ssh_password_auth", []ComplianceRef{
		cref("CIS", "5.1.16", "SSH authentication methods"),
		cref("等保2.0", "8.1.4.2", "身份鉴别—双因素"),
	}},
	{"cis_ssh_empty_passwords", []ComplianceRef{
		cref("CIS", "5.1.19", "Disallow empty passwords over SSH"),
		cref("PCI-DSS", "8.3.1", "Strong authentication required"),
	}},
	{"cis_ssh_max_auth", []ComplianceRef{
		cref("CIS", "5.1.10", "SSH MaxAuthTries"),
		cref("等保2.0", "8.1.4.2", "身份鉴别—登录失败处理"),
	}},
	{"cis_ssh_alive", []ComplianceRef{
		cref("CIS", "5.1.11", "SSH idle timeout"),
		cref("等保2.0", "8.1.4.3", "访问控制—会话超时"),
		cref("PCI-DSS", "8.2.8", "Idle session timeout"),
	}},
	{"cis_ssh_x11", []ComplianceRef{cref("CIS", "5.1.12", "SSH X11 forwarding")}},
	{"ssh_bruteforce", []ComplianceRef{
		cref("等保2.0", "8.1.4.4", "入侵防范—攻击检测"),
		cref("PCI-DSS", "8.3.4", "Limit repeated access attempts"),
		cref("ISO27001", "A.8.16", "Monitoring activities"),
	}},
	{"win_rdp_nla", []ComplianceRef{
		cref("CIS", "18.10.57", "Require NLA for RDP"),
		cref("等保2.0", "8.1.4.3", "访问控制—远程管理"),
	}},
	{"win_smb1", []ComplianceRef{
		cref("CIS", "18.4.3", "Disable SMBv1"),
		cref("等保2.0", "8.1.4.4", "入侵防范—漏洞修补"),
		cref("PCI-DSS", "2.2.4", "Disable insecure services"),
	}},
	{"win_llmnr", []ComplianceRef{cref("CIS", "18.6.4.1", "Turn off multicast name resolution")}},
	{"firewall", []ComplianceRef{
		cref("CIS", "3.5", "Host firewall configuration"),
		cref("等保2.0", "8.1.3.1", "边界防护—访问控制"),
		cref("PCI-DSS", "1.2.1", "Network security controls"),
		cref("ISO27001", "A.8.20", "Network security"),
	}},
	{"port.", []ComplianceRef{
		cref("CIS", "3.5", "Restrict listening services"),
		cref("等保2.0", "8.1.3.1", "边界防护—端口最小化"),
		cref("PCI-DSS", "1.2.5", "Ports and services inventory"),
	}},

	// --- kernel & platform hardening ---
	{"sysctl.", []ComplianceRef{
		cref("CIS", "3.3", "Network kernel parameters"),
		cref("等保2.0", "8.1.4.4", "入侵防范—安全配置"),
	}},
	{"cis_aslr", []ComplianceRef{cref("CIS", "1.5.3", "Ensure ASLR is enabled")}},
	{"cis_ip_forward", []ComplianceRef{cref("CIS", "3.1.1", "Disable IP forwarding")}},
	{"selinux_weak", []ComplianceRef{
		cref("CIS", "1.6.1.2", "Ensure SELinux/AppArmor is enforcing"),
		cref("等保2.0", "8.1.4.4", "入侵防范—强制访问控制"),
	}},
	{"perm.", []ComplianceRef{
		cref("CIS", "6.1", "System file permissions"),
		cref("等保2.0", "8.1.4.6", "数据完整性—访问权限"),
		cref("ISO27001", "A.8.3", "Information access restriction"),
	}},
	{"docker_sock_world", []ComplianceRef{
		cref("CIS Docker", "3.15", "Docker socket ownership"),
		cref("等保2.0", "8.1.4.3", "访问控制—最小权限"),
	}},
	{"docker_privileged", []ComplianceRef{
		cref("CIS Docker", "5.4", "Do not use privileged containers"),
		cref("等保2.0", "8.1.4.4", "入侵防范—最小安装"),
	}},

	// --- data protection ---
	{"win_bitlocker_off", []ComplianceRef{
		cref("CIS", "18.9.11", "BitLocker drive encryption"),
		cref("等保2.0", "8.1.4.6", "数据保密性—存储加密"),
		cref("PCI-DSS", "3.5.1", "Render PAN unreadable"),
		cref("ISO27001", "A.8.24", "Use of cryptography"),
	}},
	{"filevault_off", []ComplianceRef{
		cref("等保2.0", "8.1.4.6", "数据保密性—存储加密"),
		cref("ISO27001", "A.8.24", "Use of cryptography"),
	}},

	// --- malware & intrusion detection ---
	{"malware", []ComplianceRef{
		cref("等保2.0", "8.1.4.4", "恶意代码防范"),
		cref("PCI-DSS", "5.2.1", "Anti-malware deployed"),
		cref("ISO27001", "A.8.7", "Protection against malware"),
	}},
	{"clamav", []ComplianceRef{
		cref("等保2.0", "8.1.4.4", "恶意代码防范"),
		cref("PCI-DSS", "5.2.1", "Anti-malware deployed"),
	}},
	// A stale signature database fails the "keep current" control even when the
	// scanner itself is deployed and running clean.
	{"clamav_db_stale", []ComplianceRef{
		cref("PCI-DSS", "5.3.1", "Keep anti-malware current"),
		cref("等保2.0", "8.1.4.4", "恶意代码防范—特征库升级"),
		cref("ISO27001", "A.8.7", "Protection against malware"),
	}},
	{"clamav_db_age_unknown", []ComplianceRef{
		cref("PCI-DSS", "5.3.1", "Keep anti-malware current"),
		cref("等保2.0", "8.1.4.4", "恶意代码防范—特征库升级"),
	}},
	{"win_defender_off", []ComplianceRef{
		cref("CIS", "18.9.47", "Microsoft Defender Antivirus"),
		cref("等保2.0", "8.1.4.4", "恶意代码防范"),
		cref("PCI-DSS", "5.2.1", "Anti-malware deployed"),
	}},
	{"win_defender_stale", []ComplianceRef{
		cref("PCI-DSS", "5.3.1", "Keep anti-malware current"),
		cref("等保2.0", "8.1.4.4", "恶意代码防范—特征库升级"),
	}},
	{"ioc", []ComplianceRef{
		cref("等保2.0", "8.1.4.4", "入侵防范—攻击检测"),
		cref("ISO27001", "A.8.16", "Monitoring activities"),
	}},
	{"sip_disabled", []ComplianceRef{cref("等保2.0", "8.1.4.4", "入侵防范—系统完整性")}},
	{"mac_gatekeeper_off", []ComplianceRef{cref("等保2.0", "8.1.4.4", "入侵防范—可信程序")}},

	// --- integrity monitoring & audit ---
	{"fim.", []ComplianceRef{
		cref("CIS", "6.1", "Filesystem integrity"),
		cref("等保2.0", "8.1.4.6", "数据完整性—校验"),
		cref("PCI-DSS", "11.5.2", "Change-detection mechanism"),
		cref("ISO27001", "A.8.9", "Configuration management"),
	}},
	{"auditd_off", []ComplianceRef{
		cref("CIS", "4.1.1.1", "Ensure auditd is installed and enabled"),
		cref("等保2.0", "8.1.4.5", "安全审计—审计开启"),
		cref("PCI-DSS", "10.2.1", "Audit logs implemented"),
		cref("ISO27001", "A.8.15", "Logging"),
	}},
	{"time_sync_off", []ComplianceRef{
		cref("CIS", "2.1.1", "Time synchronization"),
		cref("等保2.0", "8.1.4.5", "安全审计—时钟同步"),
		cref("PCI-DSS", "10.6.1", "Time-synchronization"),
	}},

	// --- vulnerability & patch management ---
	{"cve", []ComplianceRef{
		cref("等保2.0", "8.1.4.4", "入侵防范—漏洞修补"),
		cref("PCI-DSS", "6.3.3", "Security patches installed"),
		cref("ISO27001", "A.8.8", "Technical vulnerabilities"),
	}},
	{"auto_updates_off", []ComplianceRef{
		cref("等保2.0", "8.1.4.4", "入侵防范—漏洞修补"),
		cref("PCI-DSS", "6.3.3", "Security patches installed"),
	}},
	{"win_reboot_pending", []ComplianceRef{cref("PCI-DSS", "6.3.3", "Security patches installed")}},
	{"win_psv2", []ComplianceRef{
		cref("CIS", "18.9.100", "Legacy scripting engines"),
		cref("等保2.0", "8.1.4.4", "入侵防范—最小安装"),
	}},
}

// complianceRefsFor returns the controls a finding violates. Longest matching
// prefix wins so a specific rule beats a family-wide fallback.
func complianceRefsFor(category, id string) []ComplianceRef {
	id = strings.TrimSpace(strings.ToLower(id))
	best := ""
	var refs []ComplianceRef
	for _, r := range complianceRules {
		p := strings.ToLower(r.prefix)
		if !strings.HasPrefix(id, p) {
			continue
		}
		if len(p) > len(best) {
			best, refs = p, r.refs
		}
	}
	if refs != nil {
		return refs
	}
	// Fall back to the category so every finding still carries audit context.
	for _, r := range complianceRules {
		if strings.EqualFold(r.prefix, category) {
			return r.refs
		}
	}
	return nil
}

// annotateCompliance stamps controls onto findings in place.
func annotateCompliance(findings []HostFinding) []HostFinding {
	for i := range findings {
		if len(findings[i].Compliance) > 0 {
			continue
		}
		findings[i].Compliance = complianceRefsFor(findings[i].Category, findings[i].ID)
	}
	return findings
}

// summarizeCompliance counts how many distinct controls are failing per
// framework. Only findings that are still open count against the posture.
func summarizeCompliance(findings []HostFinding) map[string]int {
	perFW := map[string]map[string]bool{}
	for _, f := range findings {
		switch f.Status {
		case "resolved", "false_positive":
			continue
		}
		if f.Level == "info" {
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

// complianceFrameworks lists the frameworks present, sorted for stable UI order.
func complianceFrameworks(sum map[string]int) []string {
	out := make([]string, 0, len(sum))
	for fw := range sum {
		out = append(out, fw)
	}
	sort.Strings(out)
	return out
}
