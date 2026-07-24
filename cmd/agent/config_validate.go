package main

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

// normalizeAndValidateConfig fails closed on dangerous/broken agent
// configuration. In particular, an explicitly configured agent must never
// silently fall back to localhost after a YAML/JSON error.
func normalizeAndValidateConfig(cfg *config) error {
	if cfg == nil {
		return fmt.Errorf("配置为空")
	}
	if cfg.ReportInterval < 5 || cfg.ReportInterval > 3600 {
		return fmt.Errorf("report_interval 必须在 5..3600 秒之间")
	}
	if cfg.PluginInterval < 5 || cfg.PluginInterval > 86400 {
		return fmt.Errorf("plugin_interval 必须在 5..86400 秒之间")
	}
	if strings.TrimSpace(cfg.PluginsDir) == "" {
		cfg.PluginsDir = "plugins"
	}
	if strings.TrimSpace(cfg.StateFile) == "" {
		cfg.StateFile = "agent_state.json"
	}

	targets := cfg.Servers
	if len(targets) == 0 && strings.TrimSpace(cfg.Server) != "" {
		targets = []ServerConfig{{Server: cfg.Server, Token: cfg.Token}}
	}
	if len(targets) == 0 {
		return fmt.Errorf("至少配置一个 server 或 servers")
	}
	for i := range targets {
		targets[i].Server = strings.TrimRight(strings.TrimSpace(targets[i].Server), "/")
		u, err := url.Parse(targets[i].Server)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return fmt.Errorf("servers[%d].server 必须是无内嵌凭据的 http/https URL", i)
		}
	}
	if len(cfg.Servers) > 0 {
		cfg.Servers = targets
	} else {
		cfg.Server = targets[0].Server
	}

	if cfg.SNI != nil {
		cfg.SNI.CaptureBackend = strings.ToLower(strings.TrimSpace(cfg.SNI.CaptureBackend))
		if cfg.SNI.CaptureBackend == "" {
			cfg.SNI.CaptureBackend = "auto"
		}
		switch cfg.SNI.CaptureBackend {
		case "auto", "native", "tshark":
		default:
			return fmt.Errorf("sni_dns_capture.capture_backend 必须是 auto/native/tshark")
		}
		if cfg.SNI.CaptureBackend == "native" && runtime.GOOS != "linux" {
			return fmt.Errorf("native 抓包后端仅支持 Linux；%s 请使用 tshark", runtime.GOOS)
		}
		cfg.SNI.Interface = strings.TrimSpace(cfg.SNI.Interface)
		if len(cfg.SNI.Interface) > 128 || strings.ContainsAny(cfg.SNI.Interface, "\r\n\x00") {
			return fmt.Errorf("sni_dns_capture.interface 非法或过长")
		}
		cfg.SNI.TSharkPath = strings.TrimSpace(cfg.SNI.TSharkPath)
		if cfg.SNI.TSharkPath != "" {
			base := strings.ToLower(filepath.Base(cfg.SNI.TSharkPath))
			if base != "tshark" && base != "tshark.exe" {
				return fmt.Errorf("sni_dns_capture.tshark_path 必须指向 tshark 可执行文件")
			}
		}
		if cfg.SNI.MaxEntriesPerMin < 0 || cfg.SNI.MaxEntriesPerMin > 100000 {
			return fmt.Errorf("sni_dns_capture.max_entries_per_min 必须在 0..100000 之间")
		}
		if cfg.SNI.MaxEntriesPerMin == 0 {
			cfg.SNI.MaxEntriesPerMin = 5000
		}
		if len(cfg.SNI.TLSMetadataPorts) == 0 {
			cfg.SNI.TLSMetadataPorts = []int{443, 8443, 9443}
		}
		var err error
		if cfg.SNI.TLSMetadataPorts, err = normalizeAuditPorts(cfg.SNI.TLSMetadataPorts, 32); err != nil {
			return fmt.Errorf("tls_metadata_ports: %w", err)
		}
		if cfg.SNI.ContentAudit {
			cfg.SNI.Enabled = true
			if len(cfg.SNI.ContentAuditPorts) == 0 {
				cfg.SNI.ContentAuditPorts = []int{11434, 8000, 8080}
			}
			if cfg.SNI.ContentAuditPorts, err = normalizeAuditPorts(cfg.SNI.ContentAuditPorts, 32); err != nil {
				return fmt.Errorf("content_audit_ports: %w", err)
			}
			if cfg.SNI.ContentAuditMaxBody == 0 {
				cfg.SNI.ContentAuditMaxBody = 4096
			}
			if cfg.SNI.ContentAuditMaxBody < 1024 || cfg.SNI.ContentAuditMaxBody > 65536 {
				return fmt.Errorf("content_audit_max_body 必须在 1024..65536 字节之间")
			}
			cfg.SNI.ContentAuditBodyMode = normalizeContentBodyMode(cfg.SNI.ContentAuditBodyMode)
			if cfg.SNI.ContentAuditMaxEventsPerMin == 0 {
				cfg.SNI.ContentAuditMaxEventsPerMin = 2000
			}
			if cfg.SNI.ContentAuditMaxEventsPerMin < 1 || cfg.SNI.ContentAuditMaxEventsPerMin > 100000 {
				return fmt.Errorf("content_audit_max_events_per_min 必须在 1..100000 之间")
			}
			if cfg.SNI.ContentAuditIncludeHosts, err = normalizeAuditPatterns(cfg.SNI.ContentAuditIncludeHosts, 64, 253); err != nil {
				return fmt.Errorf("content_audit_include_hosts: %w", err)
			}
			if cfg.SNI.ContentAuditExcludeHosts, err = normalizeAuditPatterns(cfg.SNI.ContentAuditExcludeHosts, 64, 253); err != nil {
				return fmt.Errorf("content_audit_exclude_hosts: %w", err)
			}
			if len(cfg.SNI.ContentAuditExcludePaths) == 0 {
				cfg.SNI.ContentAuditExcludePaths = []string{"/health*", "/metrics*", "/ready*", "/live*"}
			}
			if cfg.SNI.ContentAuditExcludePaths, err = normalizeAuditPatterns(cfg.SNI.ContentAuditExcludePaths, 64, 512); err != nil {
				return fmt.Errorf("content_audit_exclude_paths: %w", err)
			}
			if cfg.SNI.ContentAuditRedactKeys, err = normalizeAuditRedactKeys(cfg.SNI.ContentAuditRedactKeys); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeAuditPorts(in []int, max int) ([]int, error) {
	seen := map[int]bool{}
	out := make([]int, 0, len(in))
	for _, p := range in {
		if p < 1 || p > 65535 {
			return nil, fmt.Errorf("含非法端口 %d", p)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
		if len(out) > max {
			return nil, fmt.Errorf("端口最多允许 %d 个", max)
		}
	}
	return out, nil
}

func normalizeAuditPatterns(in []string, maxCount, maxLen int) ([]string, error) {
	if len(in) > maxCount {
		return nil, fmt.Errorf("规则最多允许 %d 条", maxCount)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		p := strings.ToLower(strings.TrimSpace(raw))
		if p == "" {
			continue
		}
		if len(p) > maxLen || strings.ContainsAny(p, "\r\n\x00") {
			return nil, fmt.Errorf("规则非法或过长")
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, nil
}

func normalizeAuditRedactKeys(in []string) ([]string, error) {
	if len(in) > 64 {
		return nil, fmt.Errorf("content_audit_redact_keys 最多允许 64 个字段")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		k := normalizeAuditKey(raw)
		if k == "" {
			continue
		}
		if len(k) > 64 {
			return nil, fmt.Errorf("content_audit_redact_keys 含过长字段")
		}
		for _, r := range k {
			if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_') {
				return nil, fmt.Errorf("content_audit_redact_keys 含非法字段 %q", raw)
			}
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out, nil
}
