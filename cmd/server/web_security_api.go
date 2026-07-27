package main

import (
	"encoding/json"
	"net/http"
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

// handleWebEngineRefresh starts a template refresh and returns straight away.
//
// It used to download the full template tree inside this request. On any link
// slower than a datacenter that outlives the browser and reverse-proxy read
// timeouts, so the operator saw "更新超时" while the download was in fact still
// running — and a second click then raced the first. The work now runs as a
// tracked feed job that the UI polls.
func (s *Server) handleWebEngineRefresh(w http.ResponseWriter, r *http.Request) {
	if s.feeds == nil {
		writeSecErr(w, http.StatusServiceUnavailable, "情报源模块未就绪")
		return
	}
	src, ok := feedSourceByID("nuclei-templates")
	if !ok {
		writeSecErr(w, http.StatusInternalServerError, "模板源未定义")
		return
	}
	job, err := s.feeds.runUpdate(s.cfg.SecurityFeeds(), []FeedSource{src}, s.actorName(r))
	if err != nil {
		writeSecErr(w, http.StatusConflict, err.Error())
		return
	}
	webEngineCacheMu.Lock()
	webEngineCacheAt = time.Time{}
	webEngineCacheMu.Unlock()
	s.store.AddLog(LogEntry{
		Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: "refresh nuclei templates",
	})
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleSetWebSecurityConfig(w http.ResponseWriter, r *http.Request) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeSecErr(w, http.StatusBadRequest, "bad json")
		return
	}
	cur := s.cfg.WebSecurity()
	in := cur
	b, _ := json.Marshal(raw)
	if err := json.Unmarshal(b, &in); err != nil {
		writeSecErr(w, http.StatusBadRequest, "bad json")
		return
	}
	// Preserve targets unless the client explicitly sent the field.
	if _, ok := raw["targets"]; !ok || len(in.Targets) == 0 {
		in.Targets = cur.Targets
	}
	// Preserve AutoAISummary when omitted (bool zero value would otherwise clear it).
	if _, ok := raw["auto_ai_summary"]; !ok {
		in.AutoAISummary = cur.AutoAISummary
	}
	if err := s.cfg.SetWebSecurity(in); err != nil {
		writeSecErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// Apply scan-slot concurrency immediately (was previously stuck at process start value).
	out := s.cfg.WebSecurity()
	if s.webSec != nil {
		s.webSec.setScanConcurrency(out.ScanConcurrency)
	}
	s.store.AddLog(LogEntry{
		Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: "updated web security config",
	})
	out.Targets = nil
	writeJSON(w, http.StatusOK, out)
}
