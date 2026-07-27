package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AuditExportConfig controls optional SIEM/SOC forwarding of operation audit entries.
type AuditExportConfig struct {
	Enabled    bool     `json:"enabled"`
	WebhookURL string   `json:"webhook_url,omitempty"`
	SyslogAddr string   `json:"syslog_addr,omitempty"` // host:port UDP
	Format     string   `json:"format,omitempty"`      // json | cef
	Kinds      []string `json:"kinds,omitempty"`       // optional allow-list of LogEntry.Kind
	MinLevel   string   `json:"min_level,omitempty"`   // info | warning | critical
}

func (cs *ConfigStore) AuditExport() AuditExportConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	c := cs.cfg.AuditExport
	if c.Kinds != nil {
		c.Kinds = append([]string(nil), c.Kinds...)
	}
	return c
}

func (cs *ConfigStore) SetAuditExport(c AuditExportConfig) error {
	cs.mu.Lock()
	c.Format = strings.ToLower(strings.TrimSpace(c.Format))
	if c.Format != "cef" {
		c.Format = "json"
	}
	c.MinLevel = strings.ToLower(strings.TrimSpace(c.MinLevel))
	switch c.MinLevel {
	case "", "info", "warning", "critical":
	default:
		c.MinLevel = "info"
	}
	cs.cfg.AuditExport = c
	cs.mu.Unlock()
	return cs.save()
}

func auditLevelRank(level string) int {
	switch strings.ToLower(level) {
	case "critical":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

func auditExportAllows(cfg AuditExportConfig, e LogEntry) bool {
	if cfg.MinLevel != "" && auditLevelRank(e.Level) < auditLevelRank(cfg.MinLevel) {
		return false
	}
	if len(cfg.Kinds) == 0 {
		return true
	}
	for _, k := range cfg.Kinds {
		if strings.EqualFold(strings.TrimSpace(k), e.Kind) {
			return true
		}
	}
	return false
}

func (s *Server) exportAuditEntry(e LogEntry) {
	cfg := s.cfg.AuditExport()
	if !cfg.Enabled {
		return
	}
	if !auditExportAllows(cfg, e) {
		return
	}
	if cfg.WebhookURL != "" {
		go pushAuditWebhook(cfg.WebhookURL, e)
	}
	if cfg.SyslogAddr != "" {
		go pushAuditSyslog(cfg.SyslogAddr, cfg.Format, e)
	}
}

func pushAuditWebhook(urlStr string, e LogEntry) {
	body, err := json.Marshal(map[string]any{
		"timestamp":  e.Timestamp,
		"kind":       e.Kind,
		"level":      e.Level,
		"actor":      e.Actor,
		"ip":         e.IP,
		"host":       e.Host,
		"host_id":    e.Host, // host field often carries hostname or id; SIEM joins on either
		"message":    e.Message,
		"source":     "aiops-monitor",
		"request_id": "", // filled when caller stamps via message prefix; reserved
	})
	if err != nil {
		return
	}
	// SSRF: audit webhook URL is operator-configured — block cloud metadata / link-local.
	ctxClient := newGuardedHTTPClient(5 * time.Second)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest(http.MethodPost, urlStr, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := ctxClient.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
			time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			continue
		}
		return
	}
	if lastErr != nil {
		slog.Warn("审计 Webhook 外发失败", "err", lastErr)
	}
}

func pushAuditSyslog(addr, format string, e LogEntry) {
	// Prefer TCP for reliability; fall back to UDP if TCP dial fails.
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		conn, err = net.DialTimeout("udp", addr, 2*time.Second)
		if err != nil {
			slog.Warn("审计 Syslog 连接失败", "addr", addr, "err", err)
			return
		}
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	var line string
	if format == "cef" {
		line = fmt.Sprintf("CEF:0|Sreyun|AIOps|1.0|audit|%s|%s|act=%s src=%s dst=%s msg=%s",
			escapeCEF(e.Kind), cefSeverity(e.Level), escapeCEF(e.Actor), escapeCEF(e.IP), escapeCEF(e.Host), escapeCEF(e.Message))
	} else {
		raw, _ := json.Marshal(e)
		line = string(raw)
	}
	_, _ = fmt.Fprintln(conn, line)
}

func cefSeverity(level string) string {
	switch strings.ToLower(level) {
	case "critical":
		return "8"
	case "warning":
		return "5"
	default:
		return "3"
	}
}

func escapeCEF(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `|`, `\|`)
	s = strings.ReplaceAll(s, `=`, `\=`)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 512 {
		s = s[:512]
	}
	return s
}

func validateAuditExportConfig(c AuditExportConfig) error {
	if !c.Enabled {
		return nil
	}
	if strings.TrimSpace(c.WebhookURL) == "" && strings.TrimSpace(c.SyslogAddr) == "" {
		return fmt.Errorf("启用审计外发时必须配置 webhook_url 或 syslog_addr")
	}
	if u := strings.TrimSpace(c.WebhookURL); u != "" {
		parsed, err := url.Parse(u)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("webhook_url 无效")
		}
	}
	if a := strings.TrimSpace(c.SyslogAddr); a != "" {
		if _, _, err := net.SplitHostPort(a); err != nil {
			return fmt.Errorf("syslog_addr 须为 host:port")
		}
	}
	return nil
}

func (s *Server) handleGetAuditExport(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.AuditExport())
}

func (s *Server) handleSetAuditExport(w http.ResponseWriter, r *http.Request) {
	var c AuditExportConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_json", Tr(r, "common.invalid_json"))
		return
	}
	if err := validateAuditExportConfig(c); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}
	if err := s.cfg.SetAuditExport(c); err != nil {
		writeAPIError(w, r, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r), Message: "更新审计外发配置"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
