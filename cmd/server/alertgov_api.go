package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ---- 告警治理 HTTP 端点 ----

// handleGetGovernance 返回当前告警治理配置（静默/抑制/路由规则）。
func (s *Server) handleGetGovernance(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Governance())
}

// handleSetGovernance 整体替换告警治理配置（前端一次性提交全部规则）。
func (s *Server) handleSetGovernance(w http.ResponseWriter, r *http.Request) {
	var g AlertGovernance
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	// 清洗：规整规则名，丢弃无名的空规则
	sil := g.SilenceRules[:0]
	for _, x := range g.SilenceRules {
		x.Name = strings.TrimSpace(x.Name)
		if x.Name != "" {
			sil = append(sil, x)
		}
	}
	g.SilenceRules = sil
	inh := g.InhibitRules[:0]
	for _, x := range g.InhibitRules {
		x.Name = strings.TrimSpace(x.Name)
		if x.Name != "" {
			inh = append(inh, x)
		}
	}
	g.InhibitRules = inh
	rts := g.Routes[:0]
	for _, x := range g.Routes {
		x.Name = strings.TrimSpace(x.Name)
		if x.Name != "" {
			rts = append(rts, x)
		}
	}
	g.Routes = rts

	if err := s.cfg.SetGovernance(g); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: "更新告警治理规则（静默/抑制/路由）"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
