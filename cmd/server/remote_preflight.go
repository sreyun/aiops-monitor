package main

import (
	"net/http"
	"strings"
	"time"
)

// RemotePreflight is the unified freeze/remote-gate chip for UI (terminal/desktop/SQL/playbook).
type RemotePreflight struct {
	HostID         string `json:"host_id"`
	FreezeActive   bool   `json:"freeze_active"`
	FreezeWindow   string `json:"freeze_window,omitempty"`
	FreezeNote     string `json:"freeze_note,omitempty"`
	HighRisk       bool   `json:"high_risk"`
	GateRequired   bool   `json:"gate_required"`
	GateAllowed    bool   `json:"gate_allowed"`
	GateReason     string `json:"gate_reason,omitempty"`
	ChangeID       int64  `json:"change_id,omitempty"`
	IncidentID     int64  `json:"incident_id,omitempty"`
	BreakGlassOK   bool   `json:"break_glass_ok"`
	RemoteGateMode string `json:"remote_gate_mode,omitempty"`
}

// GET /api/v1/hosts/{id}/remote-preflight
func (s *Server) handleRemotePreflight(w http.ResponseWriter, r *http.Request) {
	hostID := strings.TrimSpace(r.PathValue("id"))
	if hostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host id required"})
		return
	}
	writeJSON(w, http.StatusOK, s.computeRemotePreflight(r, hostID))
}

func (s *Server) computeRemotePreflight(r *http.Request, hostID string) RemotePreflight {
	out := RemotePreflight{
		HostID:         hostID,
		RemoteGateMode: s.cfg.remoteGateMode(),
	}
	actor := ""
	if r != nil {
		out.BreakGlassOK = s.actorIsAdmin(r)
		actor = s.actorName(r)
	}
	now := time.Now().Unix()
	cat := ""
	if s.store != nil {
		if h, ok := s.store.GetHost(hostID); ok && h != nil {
			cat = h.Category
		}
	}
	if s.cfg != nil {
		if w, ok := s.cfg.activeFreezeWindow(hostID, cat, now); ok {
			out.FreezeActive = true
			out.FreezeWindow = w.Name
			out.FreezeNote = w.Note
		}
	}
	out.HighRisk = s.hostRemoteHighRisk(hostID)
	out.ChangeID, out.IncidentID = s.remoteSessionLinks(hostID)
	allowed, reason := s.remoteGateCheck(hostID, actor, out.HighRisk, false)
	out.GateAllowed = allowed
	out.GateReason = reason
	out.GateRequired = !allowed
	if allowed && (out.FreezeActive || out.HighRisk) {
		out.GateReason = reason
	}
	return out
}
