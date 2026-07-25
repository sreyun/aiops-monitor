package main

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleSecurityOverview(w http.ResponseWriter, r *http.Request) {
	hostCfg := s.cfg.HostSecurity()
	webCfg := s.cfg.WebSecurity()

	openCritical, openHigh := s.countOpenSecurityFindings()
	hostRunning, hostStuck := s.hostSec.scanActivity(hostCfg.TimeoutSec)
	webRunning, webStuck := s.webSec.scanActivity(webCfg.TimeoutSec)

	hostSched := scheduleHealthFromPlaybook(hostCfg.Enabled, hostCfg.Schedule)
	webScheduled := 0
	for _, t := range webCfg.Targets {
		if t.Enabled && t.Schedule != nil && t.Schedule.Enabled {
			webScheduled++
		}
	}
	webSched := map[string]any{
		"enabled":          webScheduled > 0,
		"scheduled_targets": webScheduled,
		"total_targets":    len(webCfg.Targets),
	}
	healthy := (!hostSched["enabled"].(bool) || hostSched["healthy"].(bool)) &&
		(webScheduled == 0 || webSched["enabled"].(bool))

	writeJSON(w, http.StatusOK, map[string]any{
		"open_critical": openCritical,
		"open_high":     openHigh,
		"schedule": map[string]any{
			"healthy": healthy,
			"host":    hostSched,
			"web":     webSched,
		},
		"scans": map[string]any{
			"host_running":  hostRunning,
			"web_running":   webRunning,
			"host_stuck":    hostStuck,
			"web_stuck":     webStuck,
			"total_running": hostRunning + webRunning,
			"total_stuck":   hostStuck + webStuck,
		},
	})
}

func scheduleHealthFromPlaybook(enabled bool, sc *PlaybookSchedule) map[string]any {
	out := map[string]any{
		"enabled": false,
		"healthy": true,
		"kind":    "",
	}
	if !enabled || sc == nil || !sc.Enabled {
		return out
	}
	out["enabled"] = true
	out["kind"] = sc.Kind
	switch sc.Kind {
	case "interval":
		if sc.IntervalMin < 15 {
			out["healthy"] = false
			out["detail"] = "interval below 15m minimum"
		}
	case "daily", "weekly":
		if _, ok := parseHHMM(sc.At); !ok {
			out["healthy"] = false
			out["detail"] = "invalid schedule time"
		}
	default:
		out["healthy"] = false
		out["detail"] = "unknown schedule kind"
	}
	return out
}

func (s *Server) countOpenSecurityFindings() (critical, high int) {
	s.hostSec.mu.Lock()
	for _, scan := range s.hostSec.lastByHost {
		if scan == nil || scan.Status != "completed" {
			continue
		}
		if scan.Summary != nil {
			critical += scan.Summary["critical"]
			high += scan.Summary["high"]
			continue
		}
		for _, f := range scan.Findings {
			switch strings.ToLower(f.Level) {
			case "critical", "crit":
				critical++
			case "high":
				high++
			}
		}
	}
	s.hostSec.mu.Unlock()

	latestWeb := map[string]*WebScanResult{}
	s.webSec.mu.Lock()
	for _, sc := range s.webSec.scans {
		if sc == nil || sc.Status != "completed" {
			continue
		}
		prev := latestWeb[sc.TargetID]
		if prev == nil || sc.FinishedAt > prev.FinishedAt {
			latestWeb[sc.TargetID] = sc
		}
	}
	for _, sc := range latestWeb {
		if sc.Summary != nil {
			critical += sc.Summary["critical"]
			high += sc.Summary["high"]
			continue
		}
		for _, f := range sc.Findings {
			if !findingOpen(f.Status) {
				continue
			}
			switch strings.ToLower(f.Severity) {
			case "critical", "crit":
				critical++
			case "high":
				high++
			}
		}
	}
	s.webSec.mu.Unlock()
	return critical, high
}

func findingOpen(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "resolved", "false_positive", "ack", "accepted":
		return false
	default:
		return true
	}
}

func (m *hostSecurityManager) scanActivity(timeoutSec int) (running, stuck int) {
	if timeoutSec <= 0 {
		timeoutSec = 180
	}
	grace := int64(60)
	limit := int64(timeoutSec) + grace
	now := time.Now().Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sc := range m.scans {
		if sc == nil || sc.Status != "running" {
			continue
		}
		running++
		if sc.StartedAt > 0 && now-sc.StartedAt > limit {
			stuck++
		}
	}
	return running, stuck
}

func (m *webScanManager) scanActivity(timeoutSec int) (running, stuck int) {
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	grace := int64(120)
	limit := int64(timeoutSec) + grace
	now := time.Now().Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sc := range m.scans {
		if sc == nil || sc.Status != "running" {
			continue
		}
		running++
		if sc.StartedAt > 0 && now-sc.StartedAt > limit {
			stuck++
		}
	}
	return running, stuck
}
