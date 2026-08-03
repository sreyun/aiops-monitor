package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Trust / remote-gate knobs live on ServerConfig (zero-value defaults are safe).
// LoopForceAllowNonAdmin: default false → force=true requires admin.
// RemoteGateDisabled: default false → gate enabled.
// RemoteGateMode: empty → "freeze_or_highrisk".

const remoteGateModeFreezeOrHighRisk = "freeze_or_highrisk"

func (cs *ConfigStore) loopForceRequiresAdmin() bool {
	if cs == nil {
		return true
	}
	return !cs.cfg.LoopForceAllowNonAdmin
}

func (cs *ConfigStore) remoteGateEnabled() bool {
	if cs == nil {
		return true
	}
	return !cs.cfg.RemoteGateDisabled
}

func (cs *ConfigStore) remoteGateMode() string {
	if cs == nil {
		return remoteGateModeFreezeOrHighRisk
	}
	m := strings.TrimSpace(strings.ToLower(cs.cfg.RemoteGateMode))
	if m == "" {
		return remoteGateModeFreezeOrHighRisk
	}
	return m
}

func (s *Server) actorIsAdmin(r *http.Request) bool {
	if s == nil || s.cfg == nil {
		return false
	}
	u, ok := s.currentUser(r)
	if !ok {
		return false
	}
	return roleRank(u.Role) >= roleRank(RoleAdmin)
}

func (s *Server) actorIsAdminName(actor string) bool {
	if s == nil || s.cfg == nil {
		return false
	}
	return roleRank(s.cfg.RoleOf(actor)) >= roleRank(RoleAdmin)
}

// requireLoopForceAdmin rejects non-admin force=true when configured.
func (s *Server) requireLoopForceAdmin(w http.ResponseWriter, r *http.Request, force bool) bool {
	if !force || s == nil || s.cfg == nil || !s.cfg.loopForceRequiresAdmin() {
		return true
	}
	if s.actorIsAdmin(r) {
		return true
	}
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error": "force=true 仅管理员可用（loop_force_allow_non_admin=false）",
		"code":  "loop_force_admin_required",
	})
	if s.store != nil {
		s.store.AddLog(LogEntry{
			Kind: KindOperation, Level: "warning", Actor: s.actorName(r),
			Message: "闭环 force 被拒绝：非管理员",
		})
	}
	return false
}

// changeSoDAllows returns error if author self-approves (mirrors SQL proposer≠approver).
// Start/execute by the author after a third-party approval is allowed (common CM practice).
func changeSoDAllows(rec ChangeRecord, action, actor string, breakGlass bool) error {
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "approve" {
		return nil
	}
	author := strings.TrimSpace(rec.Author)
	actor = strings.TrimSpace(actor)
	if author == "" || actor == "" || !strings.EqualFold(author, actor) {
		return nil
	}
	if breakGlass {
		return nil
	}
	return fmt.Errorf("职责分离：作者 %s 不能审批自己的变更（管理员 break-glass 除外）", author)
}

// remoteGateCheck enforces freeze_or_highrisk: when freeze is active OR highRisk,
// require an approved/in_progress change on the host, or an open incident loop at/after approved.
func (s *Server) remoteGateCheck(hostID, actor string, highRisk bool, breakGlass bool) (allowed bool, reason string) {
	if s == nil || s.cfg == nil || !s.cfg.remoteGateEnabled() {
		return true, ""
	}
	if s.cfg.remoteGateMode() != remoteGateModeFreezeOrHighRisk {
		return true, ""
	}
	cat := ""
	if s.store != nil {
		if h, ok := s.store.GetHost(hostID); ok && h != nil {
			cat = h.Category
		}
	}
	_, freeze := s.cfg.activeFreezeWindow(hostID, cat, time.Now().Unix())
	if !freeze && !highRisk {
		return true, ""
	}
	if breakGlass {
		return true, "break_glass"
	}
	if s.changes != nil {
		for _, c := range s.changes.RelatedToHosts([]string{hostID}, time.Now().Unix()-14*24*3600) {
			st := normalizeChangeStatus(c.Status)
			if st == ChangeApproved || st == ChangeInProgress || st == ChangeScheduled {
				return true, fmt.Sprintf("change:%d", c.ID)
			}
		}
	}
	if s.incidents != nil {
		for _, inc := range s.incidents.List() {
			if inc.HostID != hostID || inc.Status == "resolved" {
				continue
			}
			if inc.Loop == nil {
				continue
			}
			switch strings.ToLower(inc.Loop.Stage) {
			case "approved", "verified", "promoted":
				return true, fmt.Sprintf("incident_loop:%d", inc.ID)
			}
		}
	}
	why := "高危远程操作"
	if freeze {
		why = "冻结窗内远程操作"
	}
	return false, why + "需已批准变更或事件闭环已批准（管理员可 break-glass）"
}

func (s *Server) hostRemoteHighRisk(hostID string) bool {
	if s == nil || s.incidents == nil || hostID == "" {
		return false
	}
	for _, inc := range s.incidents.List() {
		if inc.HostID == hostID && inc.Status != "resolved" && strings.EqualFold(inc.Severity, "critical") {
			return true
		}
	}
	return false
}

func (s *Server) enforceRemoteGate(w http.ResponseWriter, r *http.Request, hostID string, highRisk bool) bool {
	if !highRisk {
		highRisk = s.hostRemoteHighRisk(hostID) || r.URL.Query().Get("high_risk") == "1"
	}
	bg := s.actorIsAdmin(r) && (r.URL.Query().Get("break_glass") == "1" || r.Header.Get("X-Break-Glass") == "1")
	ok, reason := s.remoteGateCheck(hostID, s.actorName(r), highRisk, bg)
	if ok {
		if reason == "break_glass" && s.store != nil {
			label := s.hostLabelForID(hostID)
			s.addAuditLog(r, LogEntry{
				Kind: KindOperation, Level: "warning", Host: label,
				Message: "远程闸门紧急放行：" + label,
			})
		}
		return true
	}
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error": reason,
		"code":  "remote_gate_required",
	})
	return false
}

// findRemediationRun looks up a run in memory (best-effort).
func (s *Server) findRemediationRun(id int64) (RemediationRun, bool) {
	if s == nil || s.remediation == nil || id <= 0 {
		return RemediationRun{}, false
	}
	for _, r := range s.remediation.Runs() {
		if r.ID == id {
			return r, true
		}
	}
	return RemediationRun{}, false
}

// remoteSessionLinks picks the newest approved/in-progress change and open loop incident for a host.
func (s *Server) remoteSessionLinks(hostID string) (changeID, incidentID int64) {
	if s == nil || hostID == "" {
		return 0, 0
	}
	if s.changes != nil {
		for _, c := range s.changes.RelatedToHosts([]string{hostID}, time.Now().Unix()-14*24*3600) {
			st := normalizeChangeStatus(c.Status)
			if st == ChangeApproved || st == ChangeInProgress || st == ChangeScheduled {
				changeID = c.ID
				break
			}
		}
	}
	if s.incidents != nil {
		for _, inc := range s.incidents.List() {
			if inc.HostID != hostID || inc.Status == "resolved" || inc.Loop == nil {
				continue
			}
			switch strings.ToLower(inc.Loop.Stage) {
			case "approved", "verified", "promoted":
				incidentID = inc.ID
				return changeID, incidentID
			}
		}
	}
	return changeID, incidentID
}
