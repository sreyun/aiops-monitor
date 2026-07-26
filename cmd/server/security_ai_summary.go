package main

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// maybeHostSecurityAISummary optionally fills AISummary after a completed scan.
// Non-blocking: failures are logged and never change scan status.
func (s *Server) maybeHostSecurityAISummary(scan *HostScanResult) {
	if s == nil || scan == nil || scan.Status != "completed" {
		return
	}
	if !s.cfg.HostSecurity().AutoAISummary {
		return
	}
	ai := s.cfg.AIConfig()
	if !ai.Enabled || strings.TrimSpace(ai.Endpoint) == "" || strings.TrimSpace(ai.Model) == "" {
		return
	}
	scanID := scan.ID
	ctx := buildHostScanAIContext(scan)
	sys := buildAssistSystemPrompt("host_security_diagnosis", "")
	go func() {
		text, err := aiComplete(ai, sys, ctx)
		if err != nil {
			slog.Info("host security AI summary skipped", "scan_id", scanID, "err", err.Error())
			return
		}
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		if len(text) > 8000 {
			text = text[:8000]
		}
		now := time.Now().Unix()
		s.hostSec.mu.Lock()
		defer s.hostSec.mu.Unlock()
		for _, sc := range s.hostSec.scans {
			if sc != nil && sc.ID == scanID {
				sc.AISummary = text
				sc.AISummaryAt = now
				s.hostSec.rememberLastLocked(sc)
				s.hostSec.saveLocked()
				return
			}
		}
	}()
}

func (s *Server) maybeWebSecurityAISummary(scan *WebScanResult) {
	if s == nil || scan == nil || scan.Status != "completed" {
		return
	}
	if !s.cfg.WebSecurity().AutoAISummary {
		return
	}
	ai := s.cfg.AIConfig()
	if !ai.Enabled || strings.TrimSpace(ai.Endpoint) == "" || strings.TrimSpace(ai.Model) == "" {
		return
	}
	scanID := scan.ID
	ctx := buildWebScanAIContext(scan)
	sys := buildAssistSystemPrompt("web_vuln_diagnosis", "")
	go func() {
		text, err := aiComplete(ai, sys, ctx)
		if err != nil {
			slog.Info("web security AI summary skipped", "scan_id", scanID, "err", err.Error())
			return
		}
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		if len(text) > 8000 {
			text = text[:8000]
		}
		now := time.Now().Unix()
		s.webSec.mu.Lock()
		defer s.webSec.mu.Unlock()
		for _, sc := range s.webSec.scans {
			if sc != nil && sc.ID == scanID {
				sc.AISummary = text
				sc.AISummaryAt = now
				s.webSec.saveLocked()
				return
			}
		}
	}()
}

func buildHostScanAIContext(scan *HostScanResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "主机：%s (%s)\n风险：%s 评分：%d\n防火墙：%s CVE：%d 开放端口：%d\n",
		firstNonEmpty(scan.Hostname, scan.HostID), scan.HostID, scan.Risk, scan.Score,
		scan.Firewall, scan.CVECount, scan.PortCount)
	if hint := formatBaselineDiffHint(scan.BaselineDiff); hint != "" {
		fmt.Fprintf(&b, "基线对比：%s\n", hint)
	}
	fmt.Fprintf(&b, "摘要计数：%v\n", scan.Summary)
	n := 0
	for _, f := range scan.Findings {
		if f.Level != "critical" && f.Level != "high" && f.Level != "medium" {
			continue
		}
		fmt.Fprintf(&b, "- [%s/%s] %s %s\n", f.Level, f.Category, f.Title, firstNonEmpty(f.CVE, f.Package))
		n++
		if n >= 25 {
			break
		}
	}
	return b.String()
}

func buildWebScanAIContext(scan *WebScanResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "目标：%s %s\n摘要：%v\n", firstNonEmpty(scan.TargetName, scan.TargetID), scan.BaseURL, scan.Summary)
	if hint := formatBaselineDiffHint(scan.BaselineDiff); hint != "" {
		fmt.Fprintf(&b, "基线对比：%s\n", hint)
	}
	n := 0
	for _, f := range scan.Findings {
		sev := strings.ToLower(f.Severity)
		if sev != "critical" && sev != "high" && sev != "medium" {
			continue
		}
		fmt.Fprintf(&b, "- [%s] %s (%s) %s\n", f.Severity, firstNonEmpty(f.Name, f.TemplateID), f.TemplateID, firstNonEmpty(f.URL, f.MatchedAt))
		n++
		if n >= 25 {
			break
		}
	}
	return b.String()
}
