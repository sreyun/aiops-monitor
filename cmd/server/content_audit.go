package main

// Phase 2 · 明文 HTTP 内容审计（服务端接收 + 查询）。agent 抓明文 HTTP 请求(method/path/Host/
// body 前缀)上报，落 PG 审计库。⚠ 高敏感：body 可能含用户发给大模型的 prompt(PII)。仅当 agent
// 显式开启 content_audit 时才有数据；服务端只做接收/存储/查询，保留期由 cleanupContentAudit 控制。

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"aiops-monitor/shared"
)

// handleAgentContentAudit 接收 agent 上报的内容审计事件（指纹校验，与其它 agent ingest 一致）。
func (s *Server) handleAgentContentAudit(w http.ResponseWriter, r *http.Request) {
	var rep shared.ContentAuditReport
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if rep.HostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host_id required"})
		return
	}
	fp := r.Header.Get("X-Agent-Fingerprint")
	if fp == "" {
		fp = r.URL.Query().Get("fp")
	}
	if !s.forwardFingerprintOKByHost(rep.HostID, fp) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "fingerprint mismatch"})
		return
	}
	// DLP 扫描：逐条查敏感数据(内置密钥/身份证/凭据 + 用户关键词)，命中即打标签 + 告警(去重防风暴)。
	hostname := rep.HostID
	if h := s.hostByID(rep.HostID); h != nil {
		hostname = h.Hostname
	}
	cfg := s.cfg.Get()
	labels := make([]string, len(rep.Events))
	for i := range rep.Events {
		ev := rep.Events[i]
		hits := scanSensitive(ev.Body+"\n"+ev.RespBody, cfg.ContentAuditSensitiveKeywords)
		if len(hits) == 0 {
			continue
		}
		labels[i] = strings.Join(hits, ", ")
		dest := ev.Host
		if dest == "" {
			dest = ev.DstIP
		}
		dest += ev.Path
		lvl := sensitiveSeverity(hits)
		a := Alert{
			HostID: rep.HostID, Hostname: hostname, IP: ev.SrcIP,
			Level: lvl, Type: "content_audit",
			Scope:     ev.SrcIP + "→" + dest,
			Message:   Tz("alert.content_sensitive", ev.SrcIP, dest, labels[i]),
			Timestamp: ev.Ts,
		}
		s.store.AddLog(LogEntry{Kind: KindSystem, Level: lvl, Actor: "内容审计DLP", Host: hostname, Message: a.Message})
		if cfg.AlertsEnabled && shouldAlertContent(a.HostID+"|"+a.Scope+"|"+labels[i], ev.Ts) {
			s.notifier.pushChannels(cfg, a, true)
			if lvl == "critical" && s.incidents != nil {
				s.incidents.OnAlertTransition(a, alertKey(a), true) // 密钥外泄转 Incident，走学习回路
			}
		}
	}
	if s.pg != nil && len(rep.Events) > 0 {
		s.pg.insertContentAudit(rep.HostID, rep.Events, labels)
		slog.Info("内容审计已存储", "host_id", rep.HostID, "events", len(rep.Events))
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleContentAuditHosts 只返回有内容审计记录的主机（供前端过滤掉无数据主机）。
func (s *Server) handleContentAuditHosts(w http.ResponseWriter, r *http.Request) {
	if s.pg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"hosts": []any{}})
		return
	}
	hosts, err := s.pg.getContentAuditHosts()
	if err != nil {
		slog.Warn("查询有内容审计的主机失败", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	s.annotateHostNames(hosts)
	writeJSON(w, http.StatusOK, map[string]any{"hosts": hosts})
}

// handleContentAudit 前端查询内容审计记录（最新在前，支持 filter）。
func (s *Server) handleContentAudit(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host")
	if hostID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host required"})
		return
	}
	if s.pg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"events": []any{}})
		return
	}
	limit := 200
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 1000 {
		limit = n
	}
	evs, err := s.pg.getContentAudit(hostID, r.URL.Query().Get("filter"), limit)
	if err != nil {
		slog.Warn("查询内容审计失败", "host", hostID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": evs})
}
