package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ---- 仪表盘 HTTP 端点 ----

func (s *Server) handleListDashboards(w http.ResponseWriter, r *http.Request) {
	// 列表只回元信息（不含面板体），减小载荷。
	type meta struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description,omitempty"`
		Tags        []string `json:"tags,omitempty"`
		Panels      int      `json:"panels"`
		Source      string   `json:"source,omitempty"`
		UpdatedAt   int64    `json:"updated_at"`
	}
	var out []meta
	for _, d := range s.cfg.Dashboards() {
		out = append(out, meta{d.ID, d.Name, d.Description, d.Tags, len(d.Panels), d.Source, d.UpdatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"dashboards": out})
}

func (s *Server) handleGetDashboard(w http.ResponseWriter, r *http.Request) {
	d, ok := s.cfg.DashboardByID(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "仪表盘不存在"})
		return
	}
	// 惰性修复历史导入：=~ 与布局重叠，回写一次后下次不再变。
	if healImportedDashboard(&d) {
		if saved, err := s.cfg.UpsertDashboard(d); err == nil {
			d = saved
		}
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleUpsertDashboard(w http.ResponseWriter, r *http.Request) {
	var d Dashboard
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if d.ID != "" && d.Revision > 0 {
		if current, ok := s.cfg.DashboardByID(d.ID); ok && current.Revision > 0 && current.Revision != d.Revision {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":            "该仪表盘已被其他操作更新，请刷新后合并修改",
				"current_revision": current.Revision,
				"updated_at":       current.UpdatedAt,
			})
			return
		}
	}
	saved, err := s.cfg.UpsertDashboard(d)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: "保存仪表盘：" + saved.Name})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": saved.ID, "revision": saved.Revision, "updated_at": saved.UpdatedAt,
	})
}

func (s *Server) handleDeleteDashboard(w http.ResponseWriter, r *http.Request) {
	_ = s.cfg.DeleteDashboard(r.PathValue("id"))
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.clientIP(r), Message: "删除仪表盘：" + r.PathValue("id")})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// panelQueryReq 是面板查询请求：表达式 + 时间范围 + 已选变量值。
type panelQueryReq struct {
	Expr       string            `json:"expr"`
	From       int64             `json:"from"`
	To         int64             `json:"to"`
	Step       int64             `json:"step"`
	Vars       map[string]string `json:"vars"`
	DataSource string            `json:"datasource"` // 数据源 id（""=内置 VM）
	Limit      int               `json:"limit"`      // 日志面板取行上限
}

func validatePanelQueryReq(req *panelQueryReq, withRange, logs bool) error {
	if req == nil {
		return fmt.Errorf("查询请求不能为空")
	}
	req.Expr = strings.TrimSpace(req.Expr)
	if req.Expr == "" {
		return fmt.Errorf("查询表达式不能为空")
	}
	if len(req.Expr) > maxDashboardExpr {
		return fmt.Errorf("查询表达式不能超过 16 KiB")
	}
	if len(req.DataSource) > 128 {
		return fmt.Errorf("数据源 ID 过长")
	}
	if len(req.Vars) > maxDashboardVars {
		return fmt.Errorf("模板变量不能超过 %d 个", maxDashboardVars)
	}
	for k, v := range req.Vars {
		if !dashVarNameValid.MatchString(k) || len(v) > 4096 {
			return fmt.Errorf("模板变量 %q 无效或值过长", k)
		}
	}
	if !withRange {
		return nil
	}
	now := time.Now().Unix()
	if req.To <= 0 {
		req.To = now
	}
	if req.From <= 0 {
		req.From = req.To - 3600
	}
	if req.To <= req.From {
		return fmt.Errorf("查询结束时间必须晚于开始时间")
	}
	maxRange := int64(90 * 24 * 3600)
	if logs {
		maxRange = 7 * 24 * 3600
	}
	if req.To-req.From > maxRange {
		return fmt.Errorf("查询时间范围过大，最大允许 %d 天", maxRange/(24*3600))
	}
	if req.To > now+300 {
		return fmt.Errorf("查询结束时间不能超过当前时间 5 分钟")
	}
	if req.Step < 0 {
		return fmt.Errorf("查询步长不能为负数")
	}
	if logs {
		if req.Limit <= 0 {
			req.Limit = 200
		}
		if req.Limit > 2000 {
			req.Limit = 2000
		}
	}
	return nil
}

func (s *Server) handleDashboardQuery(w http.ResponseWriter, r *http.Request) {
	var req panelQueryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if err := validatePanelQueryReq(&req, true, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !s.dashBackendReady(req.DataSource) {
		writeJSON(w, http.StatusOK, map[string]any{"series": []any{}, "available": false})
		return
	}
	rangeSec := req.To - req.From
	if req.Step <= 0 {
		req.Step = rangeSec / 300 // 约 300 个点
		if req.Step < 15 {
			req.Step = 15
		}
	}
	expr := substituteVars(req.Expr, req.Vars, req.Step, rangeSec)
	series, ok := s.dashRangeSeries(req.DataSource, expr, req.From, req.To, req.Step)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"series": []any{}, "available": true, "error": "查询失败（表达式或数据源）"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"series": series, "step": req.Step})
}

// handleDashboardQueryInstant 即时查询，供 stat/gauge/bargauge/table 取当前值。
func (s *Server) handleDashboardQueryInstant(w http.ResponseWriter, r *http.Request) {
	var req panelQueryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if err := validatePanelQueryReq(&req, false, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !s.dashBackendReady(req.DataSource) {
		writeJSON(w, http.StatusOK, map[string]any{"series": []any{}, "available": false})
		return
	}
	expr := substituteVars(req.Expr, req.Vars, 60, 3600)
	vec, ok := s.dashVector(req.DataSource, expr)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"series": []any{}, "available": true, "error": "查询失败（表达式或数据源）"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"series": vec})
}

// handleDashboardQueryLogs 日志面板查询（Loki 数据源，LogQL 区间）。
func (s *Server) handleDashboardQueryLogs(w http.ResponseWriter, r *http.Request) {
	var req panelQueryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if err := validatePanelQueryReq(&req, true, true); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ds, ok := s.cfg.GetDataSource(req.DataSource)
	if !ok || ds.Type != "loki" || !ds.Enabled {
		writeJSON(w, http.StatusOK, map[string]any{"lines": []any{}, "available": false})
		return
	}
	now := time.Now()
	endNs := now.UnixNano()
	if req.To > 0 {
		endNs = req.To * 1e9
	}
	startNs := now.Add(-time.Hour).UnixNano()
	if req.From > 0 {
		startNs = req.From * 1e9
	}
	logql := substituteVars(req.Expr, req.Vars, 60, 3600)
	lines, qok := dsLokiRange(ds, logql, startNs, endNs, req.Limit)
	if !qok {
		writeJSON(w, http.StatusOK, map[string]any{"lines": []any{}, "available": true, "error": "日志查询失败（LogQL 或 Loki）"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
}

// handleDashboardVarValues 解析一个模板变量的候选值（custom 直给 / query 走 label_values，按数据源）。
func (s *Server) handleDashboardVarValues(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DashVar
		DataSource string `json:"datasource"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if len(req.DataSource) > 128 || len(req.Query) > maxDashboardExpr || len(req.Options) > 500 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "模板变量请求过大或字段无效"})
		return
	}
	lv := func(label, match string) ([]string, bool) { return s.dashLabelValues(req.DataSource, label, match) }
	writeJSON(w, http.StatusOK, map[string]any{"values": resolveVarValues(req.DashVar, lv)})
}

var grafanaIDRe = regexp.MustCompile(`^\d+$`)

// handleImportGrafana 导入看板模板：从 grafana.com 按 ID 拉取，或解析粘贴/上传的 JSON
// （自动识别 Grafana / 夜莺 Nightingale 两种导出格式），映射后保存。
func (s *Server) handleImportGrafana(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GrafanaID string `json:"grafana_id"`
		JSON      string `json:"json"`
		Name      string `json:"name"`
		Format    string `json:"format"` // ""/auto | grafana | nightingale（留空则自动识别）
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	var raw []byte
	source := "import"
	format := strings.TrimSpace(req.Format)
	if strings.TrimSpace(req.JSON) != "" {
		raw = []byte(req.JSON)
		if format == "" || format == "auto" {
			format = detectTemplateFormat(raw)
		}
	} else {
		id := strings.TrimSpace(req.GrafanaID)
		if !grafanaIDRe.MatchString(id) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请填写 grafana.com 看板 ID（纯数字），或粘贴/上传 Grafana / 夜莺看板 JSON"})
			return
		}
		// grafana.com 官方看板下载端点（公网，SSRF 守卫放行公网 IP）。
		url := "https://grafana.com/api/dashboards/" + id + "/revisions/latest/download"
		client := newGuardedHTTPClient(20 * time.Second)
		resp, err := client.Get(url)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "拉取 grafana.com 失败：" + err.Error()})
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "grafana.com 返回 " + strconv.Itoa(resp.StatusCode) + "（检查看板 ID 是否存在）"})
			return
		}
		raw, err = io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 上限 8MB
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "读取响应失败：" + err.Error()})
			return
		}
		source = "grafana:" + id
		format = "grafana"
	}

	var d Dashboard
	var err error
	if format == "nightingale" {
		if source == "import" {
			source = "nightingale"
		}
		d, err = mapNightingaleDashboard(raw, req.Name, source)
	} else {
		d, err = mapGrafanaDashboard(raw, req.Name, source)
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	saved, err := s.cfg.UpsertDashboard(d)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	unsupported := 0
	for _, p := range saved.Panels {
		if p.Type == "unsupported" {
			unsupported++
		}
	}
	kind := "Grafana"
	if format == "nightingale" {
		kind = "夜莺"
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: "导入 " + kind + " 看板：" + saved.Name + "（" + strconv.Itoa(len(saved.Panels)) + " 面板）"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID, "name": saved.Name, "panels": len(saved.Panels), "unsupported": unsupported, "format": format})
}
