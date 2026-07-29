package main

import (
	"regexp"
	"sort"
	"strings"
)

var (
	hermesWordRe = regexp.MustCompile(`(?i)\bhermes(?:\s+agent)?\b`)
	// Typical AIOps host IDs: 8–32 hex chars (case-insensitive). Used only as a last-resort scrub.
	hostIDHexRe = regexp.MustCompile(`\b[0-9a-fA-F]{12,32}\b`)
)

// hostDisplayLabel returns "hostname (ip)" for user-facing UI. Never returns a raw host ID.
func hostDisplayLabel(hostname, ip, id string) string {
	name := strings.TrimSpace(hostname)
	addr := strings.TrimSpace(ip)
	switch {
	case name != "" && addr != "":
		return name + " (" + addr + ")"
	case name != "":
		return name
	case addr != "":
		return addr
	default:
		_ = id // intentionally unused — never expose raw id
		return "未知主机"
	}
}

// hostDisplayLabelFromHost formats a *Host for UI.
func hostDisplayLabelFromHost(h *Host) string {
	if h == nil {
		return "未知主机"
	}
	return hostDisplayLabel(h.Hostname, h.IP, h.ID)
}

// buildHostLabelMap builds id → display label for redaction.
func (s *Server) buildHostLabelMap() map[string]string {
	out := map[string]string{}
	if s == nil || s.store == nil {
		return out
	}
	for _, h := range s.store.ListHosts() {
		if h == nil || h.ID == "" {
			continue
		}
		out[h.ID] = hostDisplayLabelFromHost(h)
	}
	return out
}

// redactUserFacingText replaces hermes branding and known host IDs for end-user copy.
func redactUserFacingText(text string, idToLabel map[string]string) string {
	if text == "" {
		return text
	}
	t := hermesWordRe.ReplaceAllString(text, "智能运维服务")
	t = strings.ReplaceAll(t, "hermes_auto_approve", "ai_auto_approve")
	t = strings.ReplaceAll(t, "reason=hermes_auto_approve", "reason=ai_auto_approve")
	if len(idToLabel) > 0 {
		// Replace longer IDs first to avoid partial clashes.
		ids := make([]string, 0, len(idToLabel))
		for id := range idToLabel {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return len(ids[i]) > len(ids[j]) })
		for _, id := range ids {
			if id == "" {
				continue
			}
			t = strings.ReplaceAll(t, id, idToLabel[id])
		}
	}
	return t
}

// (s *Server) redactUserFacing is a convenience wrapper.
func (s *Server) redactUserFacing(text string) string {
	return redactUserFacingText(text, s.buildHostLabelMap())
}

// (h *SreyunCore) redactUserFacing uses the parent server map when available.
func (h *SreyunCore) redactUserFacing(text string) string {
	if h == nil || h.s == nil {
		return redactUserFacingText(text, nil)
	}
	return h.s.redactUserFacing(text)
}

// (h *SreyunCore) hostLabelForID resolves a host_id to a safe display label.
func (h *SreyunCore) hostLabelForID(hostID string) string {
	hostID = strings.TrimSpace(hostID)
	if hostID == "" {
		return "未知主机"
	}
	if h != nil && h.s != nil && h.s.store != nil {
		if hh, ok := h.s.store.GetHost(hostID); ok && hh != nil {
			return hostDisplayLabelFromHost(hh)
		}
	}
	return "未知主机"
}
