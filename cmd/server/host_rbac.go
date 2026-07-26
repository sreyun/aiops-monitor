package main

import (
	"net/http"
	"strings"
)

// Host/folder-scoped RBAC: empty AllowedFolderIDs means unrestricted (legacy).
// When set, the user may only see/act on hosts assigned to those folders
// (or nested under them). Admins always bypass.

func (u AccountConfig) hostScopeRestricted() bool {
	return len(u.AllowedFolderIDs) > 0 || len(u.AllowedHostIDs) > 0 || len(u.AllowedTags) > 0
}

func (s *Server) userCanAccessHost(u AccountConfig, hostID string) bool {
	if roleRank(u.Role) >= roleRank(RoleAdmin) {
		return true
	}
	if !u.hostScopeRestricted() {
		return true
	}
	for _, id := range u.AllowedHostIDs {
		if id == hostID {
			return true
		}
	}
	h, ok := s.store.GetHost(hostID)
	if !ok {
		return false
	}
	if len(u.AllowedTags) > 0 {
		cat := strings.TrimSpace(h.Category)
		for _, t := range u.AllowedTags {
			if strings.EqualFold(strings.TrimSpace(t), cat) {
				return true
			}
		}
	}
	if len(u.AllowedFolderIDs) == 0 {
		// Only host-id / tag rules applied above.
		return len(u.AllowedHostIDs) > 0 && containsStr(u.AllowedHostIDs, hostID)
	}
	assign := s.cfg.HostFolderAssign()
	folderID := assign[hostID]
	if folderID == "" {
		return false
	}
	allowed := map[string]bool{}
	for _, fid := range u.AllowedFolderIDs {
		allowed[fid] = true
		for _, child := range s.cfg.FolderDescendantIDs(fid) {
			allowed[child] = true
		}
	}
	return allowed[folderID]
}

func (s *Server) filterHostsForUser(r *http.Request, hosts []*Host) []*Host {
	u, ok := s.currentUser(r)
	if !ok || !u.hostScopeRestricted() || roleRank(u.Role) >= roleRank(RoleAdmin) {
		return hosts
	}
	out := make([]*Host, 0, len(hosts))
	for _, h := range hosts {
		if h != nil && s.userCanAccessHost(u, h.ID) {
			out = append(out, h)
		}
	}
	return out
}

func (s *Server) requireHostAccess(w http.ResponseWriter, r *http.Request, hostID string) bool {
	u, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}
	if s.userCanAccessHost(u, hostID) {
		return true
	}
	writeJSON(w, http.StatusForbidden, map[string]string{"error": "无权访问该主机（主机组/标签授权）"})
	return false
}

// filterInventoryRows keeps only inventory maps whose host_id the caller may access.
func (s *Server) filterInventoryRows(r *http.Request, rows []map[string]any) []map[string]any {
	u, ok := s.currentUser(r)
	if !ok || !u.hostScopeRestricted() || roleRank(u.Role) >= roleRank(RoleAdmin) {
		return rows
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		hid, _ := row["host_id"].(string)
		if hid != "" && s.userCanAccessHost(u, hid) {
			out = append(out, row)
		}
	}
	return out
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// HostFolderAssign returns a copy of hostID → folderID map.
func (cs *ConfigStore) HostFolderAssign() map[string]string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make(map[string]string, len(cs.cfg.HostFolderAssign))
	for k, v := range cs.cfg.HostFolderAssign {
		out[k] = v
	}
	return out
}

// FolderDescendantIDs returns all folder IDs under root (not including root).
func (cs *ConfigStore) FolderDescendantIDs(root string) []string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	n := findFolderNode(cs.cfg.HostFolders, root)
	if n == nil {
		return nil
	}
	var out []string
	var walk func([]HostFolderNode)
	walk = func(list []HostFolderNode) {
		for _, c := range list {
			out = append(out, c.ID)
			walk(c.Children)
		}
	}
	walk(n.Children)
	return out
}
