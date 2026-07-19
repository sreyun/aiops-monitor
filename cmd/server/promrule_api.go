package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ---- 指标告警规则 HTTP 端点 ----

func (s *Server) handleListPromRules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"rules": s.cfg.PromRules()})
}

func (s *Server) handleUpsertPromRule(w http.ResponseWriter, r *http.Request) {
	var rule PromRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Expr = strings.TrimSpace(rule.Expr)
	if rule.Name == "" || rule.Expr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "名称与表达式不能为空"})
		return
	}
	if rule.Level != "warning" && rule.Level != "critical" {
		rule.Level = "warning"
	}
	if rule.ForSec < 0 {
		rule.ForSec = 0
	}
	saved, err := s.cfg.UpsertPromRule(rule)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: "保存指标告警规则：" + saved.Name})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID})
}

func (s *Server) handleDeletePromRule(w http.ResponseWriter, r *http.Request) {
	_ = s.cfg.DeletePromRule(r.PathValue("id"))
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: "删除指标告警规则：" + r.PathValue("id")})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleTestPromRule 立即评估表达式返回命中序列数 + 样例（供编辑时验证 PromQL）。
func (s *Server) handleTestPromRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Expr string `json:"expr"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if strings.TrimSpace(req.Expr) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "表达式为空"})
		return
	}
	count, samples, ok := s.promrules.evalPreview(req.Expr)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "查询失败（检查 VM 是否启用、表达式是否合法）"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": count, "samples": samples})
}
