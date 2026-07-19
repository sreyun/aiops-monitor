package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---- API 性能监控 HTTP 端点 ----

// handleAPIMonOverview 返回所有业务系统 + 接口，合并「实时状态」(内存)与「聚合指标」(VM 现算)，
// 供前端一张聚合表直接渲染（最新状态 / 平均·P95 响应时间 / 1h·24h 可用率 / 吞吐）。
func (s *Server) handleAPIMonOverview(w http.ResponseWriter, r *http.Request) {
	systems := s.cfg.APISystems()
	st := s.apimon.statusSnapshot()
	down := s.apimon.downSnapshot()
	var agg map[string]apiAggregate
	if s.vm != nil {
		agg = s.vm.queryAPIAggregate()
	}

	out := make([]map[string]any, 0, len(systems))
	for _, sys := range systems {
		eps := make([]map[string]any, 0, len(sys.Endpoints))
		for _, ep := range sys.Endpoints {
			m := map[string]any{
				"id": ep.ID, "name": ep.Name, "url": ep.URL, "method": ep.Method,
				"enabled": ep.Enabled, "headers": ep.Headers, "body": ep.Body,
				"expect_status": ep.ExpectStatus, "expect_keyword": ep.ExpectKeyword,
				"json_path": ep.JSONPath, "json_expect": ep.JSONExpect,
				// 实时状态（默认值 = 尚未探测）
				"ok": true, "message": "", "latency_ms": 0.0, "status_code": 0,
				"cert_days": -1, "resp_bytes": int64(0), "checked_at": int64(0),
				// VM 聚合（-1 = 暂无数据）
				"avg_ms": 0.0, "p95_ms": 0.0, "avail_1h": -1.0, "avail_24h": -1.0, "samples_1h": 0.0,
				"down": false, "down_since": int64(0),
			}
			if s2, ok := st[ep.ID]; ok {
				m["ok"], m["message"], m["latency_ms"], m["status_code"] = s2.OK, s2.Message, s2.LatencyMs, s2.StatusCode
				m["cert_days"], m["resp_bytes"], m["checked_at"] = s2.CertDays, s2.RespBytes, s2.CheckedAt
			}
			if a, ok := agg[ep.ID]; ok {
				m["avg_ms"], m["p95_ms"], m["samples_1h"] = a.AvgMs, a.P95Ms, a.Samples1h
				m["avail_1h"], m["avail_24h"] = a.Avail1h, a.Avail24h
			}
			if ds, ok := down[ep.ID]; ok {
				m["down"], m["down_since"] = true, ds
			}
			eps = append(eps, m)
		}
		out = append(out, map[string]any{
			"id": sys.ID, "name": sys.Name, "interval_sec": sys.IntervalSec,
			"level": sys.Level, "enabled": sys.Enabled, "created_at": sys.CreatedAt,
			"common_headers": sys.CommonHeaders, // 回显系统级公共请求头（此前遗漏→编辑时清空→保存被清零）
			"common_body":    sys.CommonBody,    // 回显系统级公共请求体（同理必须回显，否则编辑即清零）
			"endpoints":      eps,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"systems": out})
}

// handleUpsertAPISystem 新增/更新一个业务系统（含其接口列表），保存后立即探测一次。
func (s *Server) handleUpsertAPISystem(w http.ResponseWriter, r *http.Request) {
	var sys APISystem
	if err := json.NewDecoder(r.Body).Decode(&sys); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	sys.Name = strings.TrimSpace(sys.Name)
	if sys.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "业务系统名称不能为空"})
		return
	}
	if sys.IntervalSec < 5 {
		sys.IntervalSec = 60
	}
	if sys.Level != "warning" && sys.Level != "critical" {
		sys.Level = "critical"
	}
	// 清洗接口：去空名/空 URL，规整方法
	cleaned := make([]APIEndpoint, 0, len(sys.Endpoints))
	for _, ep := range sys.Endpoints {
		ep.Name = strings.TrimSpace(ep.Name)
		ep.URL = strings.TrimSpace(ep.URL)
		if ep.Name == "" || ep.URL == "" {
			continue
		}
		ep.Method = strings.ToUpper(strings.TrimSpace(ep.Method))
		cleaned = append(cleaned, ep)
	}
	sys.Endpoints = cleaned
	saved, err := s.cfg.UpsertAPISystem(sys)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.apimon.runNow(saved.ID)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: "保存 API 监控业务系统：" + saved.Name})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID})
}

func (s *Server) handleDeleteAPISystem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = s.cfg.DeleteAPISystem(id)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: "删除 API 监控业务系统：" + id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleRunAPISystem 立即探测某业务系统的全部接口（fire-and-forget）。
func (s *Server) handleRunAPISystem(w http.ResponseWriter, r *http.Request) {
	s.apimon.runNow(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleAPIEndpointHistory 返回某接口从 VM 读取的历史序列（延迟/状态随时间）。
func (s *Server) handleAPIEndpointHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var pts []APIHistPoint
	if s.vm != nil && s.vm.enabled() {
		to := time.Now().Unix()
		from := to - 24*3600 // 默认最近 24h
		if m := r.URL.Query().Get("since_min"); m != "" {
			if v, _ := strconv.Atoi(m); v > 0 {
				from = to - int64(v)*60
			}
		}
		pts = s.vm.queryAPIHistory(id, from, to)
	}
	if pts == nil {
		pts = []APIHistPoint{}
	}
	writeJSON(w, http.StatusOK, pts)
}
