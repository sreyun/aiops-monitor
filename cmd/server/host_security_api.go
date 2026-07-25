package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func writeSecErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleHostSecurityScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HostIDs []string `json:"host_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeSecErr(w, http.StatusBadRequest, "bad json")
		return
	}
	if len(req.HostIDs) == 0 {
		writeSecErr(w, http.StatusBadRequest, "host_ids required")
		return
	}
	if len(req.HostIDs) > 50 {
		writeSecErr(w, http.StatusBadRequest, "too many hosts")
		return
	}
	op := s.actorName(r)
	type row struct {
		HostID string          `json:"host_id"`
		Scan   *HostScanResult `json:"scan,omitempty"`
		Error  string          `json:"error,omitempty"`
	}
	out := make([]row, 0, len(req.HostIDs))
	ids := make([]string, 0, len(req.HostIDs))
	for _, id := range req.HostIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			out = append(out, row{Error: "empty host_id"})
			continue
		}
		scan := s.beginHostSecurityScan(id, op, "manual")
		out = append(out, row{HostID: id, Scan: scan})
		if scan != nil && scan.Status == "running" {
			ids = append(ids, scan.ID)
		} else if scan != nil && scan.Error != "" {
			out[len(out)-1].Error = scan.Error
		}
	}
	// Finish asynchronously so HTTP is not blocked for agent+OSV duration.
	go s.finishHostSecurityScans(ids)
	s.store.AddLog(LogEntry{
		Kind: KindOperation, Level: "info", Actor: op, IP: s.clientIP(r),
		Message: "host security scan: " + strings.Join(req.HostIDs, ","),
	})
	writeJSON(w, http.StatusAccepted, map[string]any{"results": out, "async": true})
}

func (s *Server) handleHostSecurityScans(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
			if limit > 200 {
				limit = 200
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"scans": s.hostSec.list(limit)})
}

func (s *Server) handleHostSecurityScanGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	scan := s.hostSec.get(id)
	if scan == nil {
		writeSecErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.secFindings != nil && scan.HostID != "" && len(scan.Findings) > 0 {
		cp := *scan
		cp.Findings = mergeHostFindingStatus(s.secFindings, scan.HostID, scan.Findings)
		writeJSON(w, http.StatusOK, &cp)
		return
	}
	writeJSON(w, http.StatusOK, scan)
}

func (s *Server) handleHostSecuritySummary(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"hosts": s.hostSec.summary()})
}

func (s *Server) handleGetHostSecurityConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.HostSecurity()
	cfg.EnableClamAV = cfg.clamAVEnabled()
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleSetHostSecurityConfig(w http.ResponseWriter, r *http.Request) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeSecErr(w, http.StatusBadRequest, "bad json")
		return
	}
	cur := s.cfg.HostSecurity()
	c := cur
	b, _ := json.Marshal(raw)
	if err := json.Unmarshal(b, &c); err != nil {
		writeSecErr(w, http.StatusBadRequest, "bad json")
		return
	}
	// Only change ClamAV opt-out when the client explicitly sent a flag.
	if _, ok := raw["disable_clamav"]; ok {
		// decoded into c.DisableClamAV
	} else if v, ok := raw["enable_clamav"]; ok {
		var on bool
		_ = json.Unmarshal(v, &on)
		c.DisableClamAV = !on
	} else {
		c.DisableClamAV = cur.DisableClamAV
	}
	c.EnableClamAV = !c.DisableClamAV
	if err := s.cfg.SetHostSecurity(c); err != nil {
		writeSecErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.store.AddLog(LogEntry{
		Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: "updated host security config",
	})
	out := s.cfg.HostSecurity()
	out.EnableClamAV = out.clamAVEnabled()
	writeJSON(w, http.StatusOK, out)
}
