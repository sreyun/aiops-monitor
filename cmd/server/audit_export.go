package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// AuditExportConfig controls optional SIEM/SOC forwarding of operation audit entries.
type AuditExportConfig struct {
	Enabled    bool   `json:"enabled"`
	WebhookURL string `json:"webhook_url,omitempty"`
	SyslogAddr string `json:"syslog_addr,omitempty"` // host:port UDP
	Format     string `json:"format,omitempty"`      // json | cef
}

func (cs *ConfigStore) AuditExport() AuditExportConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.AuditExport
}

func (cs *ConfigStore) SetAuditExport(c AuditExportConfig) error {
	cs.mu.Lock()
	c.Format = strings.ToLower(strings.TrimSpace(c.Format))
	if c.Format != "cef" {
		c.Format = "json"
	}
	cs.cfg.AuditExport = c
	cs.mu.Unlock()
	return cs.save()
}

func (s *Server) exportAuditEntry(e LogEntry) {
	cfg := s.cfg.AuditExport()
	if !cfg.Enabled {
		return
	}
	if cfg.WebhookURL != "" {
		go pushAuditWebhook(cfg.WebhookURL, e)
	}
	if cfg.SyslogAddr != "" {
		go pushAuditSyslog(cfg.SyslogAddr, cfg.Format, e)
	}
}

func pushAuditWebhook(url string, e LogEntry) {
	body, err := json.Marshal(map[string]any{
		"timestamp": e.Timestamp,
		"kind":      e.Kind,
		"level":     e.Level,
		"actor":     e.Actor,
		"ip":        e.IP,
		"host":      e.Host,
		"message":   e.Message,
		"source":    "aiops-monitor",
	})
	if err != nil {
		return
	}
	ctxClient := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ctxClient.Do(req)
	if err != nil {
		slog.Warn("审计 Webhook 外发失败", "err", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		slog.Warn("审计 Webhook 外发非 2xx", "status", resp.StatusCode)
	}
}

func pushAuditSyslog(addr, format string, e LogEntry) {
	conn, err := net.DialTimeout("udp", addr, 2*time.Second)
	if err != nil {
		slog.Warn("审计 Syslog 连接失败", "addr", addr, "err", err)
		return
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	var line string
	if format == "cef" {
		line = fmt.Sprintf("CEF:0|Sreyun|AIOps|1.0|audit|%s|%s|act=%s src=%s msg=%s",
			escapeCEF(e.Kind), cefSeverity(e.Level), escapeCEF(e.Actor), escapeCEF(e.IP), escapeCEF(e.Message))
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

func (s *Server) handleGetAuditExport(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.AuditExport())
}

func (s *Server) handleSetAuditExport(w http.ResponseWriter, r *http.Request) {
	var c AuditExportConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if err := s.cfg.SetAuditExport(c); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r), Message: "更新审计外发配置"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
