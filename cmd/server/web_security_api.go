package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleListWebTargets(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.WebSecurity()
	out := make([]WebScanTarget, 0, len(cfg.Targets))
	for _, t := range cfg.Targets {
		out = append(out, maskWebTarget(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": out})
}

func (s *Server) handleUpsertWebTarget(w http.ResponseWriter, r *http.Request) {
	var t WebScanTarget
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeSecErr(w, http.StatusBadRequest, "bad json")
		return
	}
	if id := strings.TrimSpace(r.PathValue("id")); id != "" {
		t.ID = id
	}
	saved, err := s.cfg.UpsertWebTarget(t)
	if err != nil {
		writeSecErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.store.AddLog(LogEntry{
		Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: "upsert web scan target: " + saved.Name,
	})
	writeJSON(w, http.StatusOK, maskWebTarget(saved))
}

func (s *Server) handleDeleteWebTarget(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if !s.cfg.DeleteWebTarget(id) {
		writeSecErr(w, http.StatusNotFound, "not found")
		return
	}
	s.store.AddLog(LogEntry{
		Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: "delete web scan target: " + id,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleWebTargetScan(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if _, ok := s.cfg.GetWebTarget(id); !ok {
		writeSecErr(w, http.StatusNotFound, "not found")
		return
	}
	scan := s.beginWebScan(id, s.actorName(r), "manual")
	if scan.Status == "failed" && (scan.ID == "ws-busy" || scan.ID == "ws-queue") {
		writeSecErr(w, http.StatusConflict, scan.Error)
		return
	}
	if scan.Status != "failed" {
		go s.completeWebScan(scan.ID)
	}
	s.store.AddLog(LogEntry{
		Kind: KindOperation, Level: "info", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: "web security scan: " + id,
	})
	writeJSON(w, http.StatusAccepted, map[string]any{"scan": scan, "async": true})
}

func (s *Server) handleWebScans(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
			if limit > 200 {
				limit = 200
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"scans": s.webSec.list(limit)})
}

func (s *Server) handleWebScanGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	scan := s.webSec.get(id)
	if scan == nil {
		writeSecErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.secFindings != nil && scan.TargetID != "" && len(scan.Findings) > 0 {
		cp := *scan
		cp.Findings = mergeWebFindingStatus(s.secFindings, scan.TargetID, scan.Findings)
		writeJSON(w, http.StatusOK, &cp)
		return
	}
	writeJSON(w, http.StatusOK, scan)
}

func (s *Server) handleGetWebSecurityConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.WebSecurity()
	cfg.Targets = nil
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleWebEngineStatus(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("refresh") == "1"
	writeJSON(w, http.StatusOK, s.collectWebEngineStatus(force))
}

func (s *Server) handleWebEngineRefresh(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.WebSecurity()
	bin := cfg.NucleiPath
	if bin == "" {
		bin = "nuclei"
	}
	dir := s.resolveNucleiTemplatesDir(cfg)
	home, _ := os.UserHomeDir()
	homeTpl := filepath.Join(home, "nuclei-templates")
	// Force reinstall: clear home cache then publish into persisted data dir.
	_ = os.RemoveAll(homeTpl)
	if err := downloadNucleiTemplatesHome(bin, homeTpl, 15*time.Minute); err != nil {
		writeSecErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := publishNucleiTemplates(homeTpl, dir); err != nil {
		writeSecErr(w, http.StatusBadGateway, err.Error())
		return
	}
	webEngineCacheMu.Lock()
	webEngineCacheAt = time.Time{}
	webEngineCacheMu.Unlock()
	st := s.collectWebEngineStatus(true)
	s.store.AddLog(LogEntry{
		Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: "refresh nuclei templates",
	})
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleSetWebSecurityConfig(w http.ResponseWriter, r *http.Request) {
	var in WebSecurityConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeSecErr(w, http.StatusBadRequest, "bad json")
		return
	}
	cur := s.cfg.WebSecurity()
	if len(in.Targets) == 0 {
		in.Targets = cur.Targets
	}
	if err := s.cfg.SetWebSecurity(in); err != nil {
		writeSecErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.store.AddLog(LogEntry{
		Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: "updated web security config",
	})
	out := s.cfg.WebSecurity()
	out.Targets = nil
	writeJSON(w, http.StatusOK, out)
}
