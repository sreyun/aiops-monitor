package main

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// AI governance fields live on AIConfig (see aiops.go). This file implements
// per-user daily quota tracking and AI write-tool audit trail.

type aiQuotaDay struct {
	Day   string // YYYY-MM-DD UTC
	Count int
}

type aiToolAuditEntry struct {
	Timestamp  int64  `json:"timestamp"`
	Actor      string `json:"actor"`
	Tool       string `json:"tool"`
	Action     string `json:"action"`
	HostID     string `json:"host_id,omitempty"`
	Approved   bool   `json:"approved"`
	Blocked    bool   `json:"blocked"`
	Detail     string `json:"detail,omitempty"`
	IncidentID int64  `json:"incident_id,omitempty"`
}

type aiGovHub struct {
	mu      sync.Mutex
	quota   map[string]aiQuotaDay // username → day count
	tools   []aiToolAuditEntry
	toolCap int
}

func newAIGovHub() *aiGovHub {
	return &aiGovHub{quota: map[string]aiQuotaDay{}, toolCap: 500}
}

func (h *aiGovHub) checkAndIncrQuota(user string, limit int) (ok bool, used, lim int) {
	if limit <= 0 {
		return true, 0, 0
	}
	if user == "" {
		user = "anonymous"
	}
	day := time.Now().UTC().Format("2006-01-02")
	h.mu.Lock()
	defer h.mu.Unlock()
	cur := h.quota[user]
	if cur.Day != day {
		cur = aiQuotaDay{Day: day, Count: 0}
	}
	if cur.Count >= limit {
		h.quota[user] = cur
		return false, cur.Count, limit
	}
	cur.Count++
	h.quota[user] = cur
	return true, cur.Count, limit
}

func (h *aiGovHub) recordTool(e aiToolAuditEntry) {
	if e.Timestamp == 0 {
		e.Timestamp = time.Now().Unix()
	}
	h.mu.Lock()
	h.tools = append([]aiToolAuditEntry{e}, h.tools...)
	if len(h.tools) > h.toolCap {
		h.tools = h.tools[:h.toolCap]
	}
	h.mu.Unlock()
}

func (h *aiGovHub) listTools(limit int) []aiToolAuditEntry {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.tools) < limit {
		limit = len(h.tools)
	}
	out := make([]aiToolAuditEntry, limit)
	copy(out, h.tools[:limit])
	return out
}

// redactAIText masks emails/phones/tokens in AI prompts/responses when enabled.
func redactAIText(s string, enabled bool) string {
	if !enabled || s == "" {
		return s
	}
	// Lightweight patterns — full DLP is out of scope; enough for governance demo.
	out := s
	out = strings.ReplaceAll(out, "@", "[at]")
	if len(out) > 8 {
		// mask long hex/base64-looking secrets
		var b strings.Builder
		run := 0
		for _, r := range out {
			if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
				run++
				if run > 12 {
					b.WriteByte('*')
					continue
				}
			} else {
				run = 0
			}
			b.WriteRune(r)
		}
		out = b.String()
	}
	return out
}

func (s *Server) handleListAIToolAudit(w http.ResponseWriter, r *http.Request) {
	if s.aiGov == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, s.aiGov.listTools(100))
}

// aiGovAllowRequest enforces daily quota for Assist/Sreyun chat entrypoints.
func (s *Server) aiGovAllowRequest(r *http.Request) (bool, string) {
	cfg := s.cfg.AIConfig()
	if cfg.DailyQuotaPerUser <= 0 {
		return true, ""
	}
	user := s.actorName(r)
	ok, used, lim := s.aiGov.checkAndIncrQuota(user, cfg.DailyQuotaPerUser)
	if !ok {
		return false, "AI 日配额已用尽（" + itoa(used) + "/" + itoa(lim) + "），请明日再试或联系管理员提高配额"
	}
	return true, ""
}
