package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// ============================================================================
// 仪表盘 AI 闭环：自然语言生成看板 / 按事件生成分析看板 / 数据摘要 / 研判转工单。
//
// 生成类走 aiComplete（同步补全 → 校验 JSON → 落盘）；解读/优化类走统一 /ai/assist
// （流式 + RAG + 👍👎 学习闭环，见 buildAssistSystemPrompt 的 dashboard_analysis / _optimize）。
// ============================================================================

// aiDashSpec 是 AI 产出的看板结构（宽松版，供校验前反序列化）。字段刻意接受多种别名
// （expr/query、legend/legendFormat、w-h/gridPos、name/title），因为 LLM 常混入 Grafana
// 原生 JSON 的写法——若只认单一字段，别名写法会被整段忽略，导致「应用优化后看板为空」。
type aiDashSpec struct {
	Name   string        `json:"name"`
	Title  string        `json:"title"` // Grafana 顶层用 title
	Vars   []aiDashVar   `json:"vars"`
	Panels []aiDashPanel `json:"panels"`
}

type aiDashVar struct {
	Name    string   `json:"name"`
	Label   string   `json:"label"`
	Type    string   `json:"type"`
	Query   string   `json:"query"`
	Options []string `json:"options"`
}

type aiDashPanel struct {
	Title   string   `json:"title"`
	Type    string   `json:"type"`
	Unit    string   `json:"unit"`
	W       int      `json:"w"`
	H       int      `json:"h"`
	GridPos struct { // Grafana 原生布局
		W int `json:"w"`
		H int `json:"h"`
	} `json:"gridPos"`
	Text    string         `json:"text"`
	Targets []aiDashTarget `json:"targets"`
}

type aiDashTarget struct {
	Expr         string `json:"expr"`
	Query        string `json:"query"` // Grafana 目标常用 query 存 PromQL
	Legend       string `json:"legend"`
	LegendFormat string `json:"legendFormat"` // Grafana 图例字段
}

// specName 返回看板名（兼容 name / title）。
func (s aiDashSpec) specName() string {
	if n := strings.TrimSpace(s.Name); n != "" {
		return n
	}
	return strings.TrimSpace(s.Title)
}

// targetExpr / targetLegend 合并别名字段。
func (t aiDashTarget) targetExpr() string {
	if e := strings.TrimSpace(t.Expr); e != "" {
		return e
	}
	return strings.TrimSpace(t.Query)
}

func (t aiDashTarget) targetLegend() string {
	if l := strings.TrimSpace(t.Legend); l != "" {
		return l
	}
	return strings.TrimSpace(t.LegendFormat)
}

// unwrapDashboardJSON 解开 Grafana 导出格式的外层 {"dashboard":{...}}，只在内层含 panels、
// 而外层不含 panels 时才下钻，避免误伤本平台原生结构。
func unwrapDashboardJSON(js string) string {
	var probe map[string]json.RawMessage
	if json.Unmarshal([]byte(js), &probe) != nil {
		return js
	}
	if _, hasPanels := probe["panels"]; hasPanels {
		return js
	}
	inner, ok := probe["dashboard"]
	if !ok {
		return js
	}
	var innerProbe map[string]json.RawMessage
	if json.Unmarshal(inner, &innerProbe) == nil {
		if _, ok := innerProbe["panels"]; ok {
			return string(inner)
		}
	}
	return js
}

// decodeAIDashSpec 从 AI 回复原文解析看板规格：抽 JSON → 解外层 dashboard → 反序列化。
func decodeAIDashSpec(raw string) (aiDashSpec, bool) {
	js := extractJSONObject(raw)
	if js == "" {
		return aiDashSpec{}, false
	}
	js = unwrapDashboardJSON(js)
	var spec aiDashSpec
	if json.Unmarshal([]byte(js), &spec) != nil {
		return aiDashSpec{}, false
	}
	return spec, true
}

const aiDashSchemaHint = "严格只输出一个 JSON 对象（可放在 ```json 代码块里），结构如下：\n" +
	"{\n" +
	`  "name": "看板名称",` + "\n" +
	`  "vars": [{"name":"instance","type":"query","query":"label_values(<指标>, <标签>)"}],` + "\n" +
	`  "panels": [{"title":"面板标题","type":"timeseries|stat|gauge|piechart|barchart|bargauge|histogram|state-timeline|heatmap|table|alertlist|text","unit":"percent|percentunit|bytes|Bps|s|ms|reqps|short|","w":12,"h":8,` + "\n" +
	`     "targets":[{"expr":"<PromQL>","legend":"{{标签}}"}]}]` + "\n" +
	"}\n" +
	"要求：① 只用【可用指标】里真实存在的指标名，不要臆造；② 计数器类指标配合 rate()/irate() 与时间窗口；" +
	"③ 用量用 percent/bytes 等合适单位（运行时间/时长用 s，字节用 bytes，速率用 Bps，请求率用 reqps，比率用 percentunit）；④ 每个面板给贴切标题；" +
	"⑤ 【充分利用组件库、避免千篇一律的 timeseries】：随时间变化的趋势用 timeseries；单个关键当前值(运行时间/在线数/总量)用 stat；" +
	"占比/利用率(0-100%)用 gauge(圆环)；构成占比(各状态/各分区/各类型)用 piechart；类别排行 top-N 用 barchart；" +
	"多实例同一指标横向对比用 bargauge；数值分布用 histogram；可用性/状态随时间(up/down)用 state-timeline；" +
	"多实例密度对比用 heatmap；明细清单用 table；平台当前告警用 alertlist(无需查询)。一个高质量看板应混用至少 4 种不同 type，切忌全是 timeseries。" +
	"⑥ 【美观布局】：整体按黄金信号分区，从上到下 = 顶部一行 stat 概览(每个 w=6、h=4，2~4 个并排铺满一行) → 中部 timeseries 趋势(w=12、h=8，两个一行) → " +
	"底部 piechart/barchart/gauge/table 等构成与明细(piechart/barchart/table 用 w=12、h=8；gauge 用 w=8、h=7；bargauge 用 w=12、h=6)；" +
	"绝不要让单个 stat/gauge 占满整行或使用过大高度(会出现大片空白)；同一行面板保持等高，栅格每行合计 w=24 铺满不留缝；piechart 的切片控制在 3~8 个(过多切片改用 barchart)；" +
	"⑦ 若适合按实例/任务/接口下钻，加一个 query 型模板变量并在表达式里用 $变量；⑧ 面板数量控制在 6-12 个，覆盖核心黄金信号且类型丰富。只输出 JSON，不要额外解释。"

// extractJSONObject 从 AI 回复里抽出第一个 JSON 对象（优先 ```json 代码块，否则首个 { 到末个 }）。
func extractJSONObject(s string) string {
	if i := strings.Index(s, "```json"); i >= 0 {
		rest := s[i+7:]
		if j := strings.Index(rest, "```"); j >= 0 {
			return strings.TrimSpace(rest[:j])
		}
	}
	if i := strings.Index(s, "```"); i >= 0 { // 无语言标记的代码块
		rest := s[i+3:]
		if j := strings.Index(rest, "```"); j >= 0 {
			inner := strings.TrimSpace(rest[:j])
			if strings.HasPrefix(inner, "{") {
				return inner
			}
		}
	}
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return ""
}

// sanitizeAIDash 把 AI 产出的宽松结构校验/规整为内部 Dashboard（类型白名单、栏宽钳制、网格布局、丢空查询）。
func sanitizeAIDash(spec aiDashSpec, name, source string) (Dashboard, []string) {
	var warns []string
	d := Dashboard{Source: source}
	d.Name = strings.TrimSpace(name)
	if d.Name == "" {
		d.Name = spec.specName()
	}
	if d.Name == "" {
		d.Name = "AI 生成看板"
	}
	for _, v := range spec.Vars {
		if strings.TrimSpace(v.Name) == "" {
			continue
		}
		typ := v.Type
		switch typ {
		case "query", "custom", "constant", "textbox":
		default:
			typ = "query"
		}
		d.Vars = append(d.Vars, DashVar{Name: v.Name, Label: v.Label, Type: typ, Query: v.Query, Options: v.Options})
	}
	id := 1
	for _, p := range spec.Panels {
		typ := p.Type
		switch typ {
		case "timeseries", "stat", "gauge", "bargauge", "table", "text", "piechart", "barchart",
			"histogram", "state-timeline", "heatmap", "alertlist", "logs":
		case "pie":
			typ = "piechart"
		case "bar":
			typ = "barchart"
		case "statetimeline":
			typ = "state-timeline"
		default:
			typ = "timeseries"
		}
		panel := DashPanel{ID: id, Title: strings.TrimSpace(p.Title), Type: typ, Unit: p.Unit, Text: p.Text}
		w, h := p.W, p.H
		if w == 0 {
			w = p.GridPos.W
		}
		if h == 0 {
			h = p.GridPos.H
		}
		panel.Grid = DashGrid{W: aiPanelWidth(typ, w), H: aiPanelHeight(typ, h)}
		for _, t := range p.Targets {
			expr := t.targetExpr()
			if expr == "" {
				continue
			}
			panel.Targets = append(panel.Targets, DashTarget{Expr: expr, Legend: t.targetLegend()})
		}
		if typ != "text" && typ != "alertlist" && len(panel.Targets) == 0 {
			warns = append(warns, "面板「"+panel.Title+"」无有效查询，已跳过")
			continue
		}
		d.Panels = append(d.Panels, panel)
		id++
	}
	layoutAIDashPanels(d.Panels)
	return d, warns
}

// aiPanelHeight 按面板类型给出合理的行高（网格行数），避免 stat/gauge 等单值面板被撑成大空白框：
// stat 单个数字只需很矮，timeseries/table 需要较高。同时钳制 AI 乱给的极端值。
func aiPanelHeight(typ string, h int) int {
	switch typ {
	case "stat":
		if h < 3 || h > 5 {
			return 4
		}
	case "bargauge":
		if h < 3 || h > 8 {
			return 5
		}
	case "gauge":
		if h < 4 || h > 8 {
			return 6
		}
	case "text":
		if h < 2 || h > 6 {
			return 3
		}
	case "state-timeline", "histogram":
		if h < 3 || h > 10 {
			return 6
		}
	default: // timeseries / table / piechart / barchart / heatmap / alertlist / logs
		if h < 4 || h > 14 {
			return 8
		}
	}
	return h
}

// aiPanelWidth 按面板类型给出合理的栅格宽度（1-24），避免 piechart/barchart/table 等被 AI 给成
// 过窄导致图例/切片被挤压：单值 stat 允许窄（并排铺一行），可视化类保证足够宽度。
func aiPanelWidth(typ string, w int) int {
	if w < 1 || w > 24 {
		switch typ {
		case "stat", "gauge":
			return 6
		case "bargauge", "text":
			return 12
		default:
			return 12
		}
	}
	switch typ {
	case "piechart", "barchart", "table", "heatmap", "timeseries", "state-timeline", "histogram":
		if w < 8 { // 可视化面板过窄会挤压图例/坐标轴，最低给到 8 栏（1/3 行）
			return 8
		}
	case "gauge":
		if w < 6 {
			return 6
		}
	}
	return w
}

// layoutAIDashPanels 把面板按 24 栏从左到右流式排布，超宽换行，生成 gridPos（x,y 供排序）。
// 各面板保留自己的类型化高度（见 aiPanelHeight）：stat 矮、timeseries 高，不做整行拔高，
// 避免把 stat 撑成大空白框；行内同高由 aiPanelHeight + 提示词「同类同高」保证。
func layoutAIDashPanels(panels []DashPanel) {
	x, y, rowH := 0, 0, 0
	for i := range panels {
		w := panels[i].Grid.W
		if w < 1 || w > 24 {
			w = 12
		}
		h := panels[i].Grid.H
		if h < 2 {
			h = 8
		}
		if x+w > 24 {
			x = 0
			y += rowH
			rowH = 0
		}
		panels[i].Grid = DashGrid{X: x, Y: y, W: w, H: h}
		x += w
		if h > rowH {
			rowH = h
		}
	}
}

// generateDashboardViaAI 是生成主流程：汇集可用指标上下文 → aiComplete → 抽 JSON → 校验落盘。
func (s *Server) generateDashboardViaAI(userNeed, seedCtx, source string) (Dashboard, []string, error) {
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.Endpoint == "" || cfg.Model == "" {
		return Dashboard{}, nil, fmt.Errorf("AI 未配置或未启用，请先在「AI 设置」填写并保存")
	}
	metricsCtx := s.metricContextFor(userNeed + " " + seedCtx)
	sys := "你是可观测性与 Prometheus 专家，为运维平台生成监控仪表盘。平台指标存于 VictoriaMetrics（Prometheus 兼容），" +
		"面板用 PromQL 查询。禁止深度思考与思维链，直接输出最终 JSON。\n" + aiDashSchemaHint
	if metricsCtx != "" {
		sys += "\n\n【可用指标（节选）】\n" + metricsCtx
	}
	user := strings.TrimSpace(userNeed)
	if seedCtx != "" {
		user += "\n\n【补充上下文】\n" + seedCtx
	}
	// 看板生成是结构化 JSON 任务：关掉深度思考，避免 Qwen3/R1 等先「想」两分钟再超时。
	out, err := aiCompleteOpts(cfg, sys, user, aiCallOpts{DisableThinking: true})
	if err != nil {
		return Dashboard{}, nil, fmt.Errorf("AI 生成失败：%v", err)
	}
	spec, ok := decodeAIDashSpec(out)
	if !ok {
		return Dashboard{}, nil, fmt.Errorf("AI 未返回可解析的看板 JSON")
	}
	d, warns := sanitizeAIDash(spec, "", source)
	if len(d.Panels) == 0 {
		return Dashboard{}, warns, fmt.Errorf("AI 未生成任何有效面板")
	}
	saved, err := s.cfg.UpsertDashboard(d)
	if err != nil {
		return Dashboard{}, warns, err
	}
	return saved, warns, nil
}

// metricContextFor 取 VM 全部指标名，按与需求的词重合度打分挑选（上限 ~200），作为生成上下文。
func (s *Server) metricContextFor(need string) string {
	if s.vm == nil || !s.vm.enabled() {
		return ""
	}
	all, ok := s.vm.vmLabelValues("__name__", "")
	if !ok || len(all) == 0 {
		return ""
	}
	const cap = 200
	if len(all) <= cap {
		return strings.Join(all, ", ")
	}
	// 词重合打分：需求里的词作为子串命中指标名者优先
	toks := tokenize(need)
	type scored struct {
		name  string
		score int
	}
	var arr []scored
	for _, m := range all {
		lm := strings.ToLower(m)
		sc := 0
		for _, t := range toks {
			if strings.Contains(lm, t) {
				sc++
			}
		}
		arr = append(arr, scored{m, sc})
	}
	sort.SliceStable(arr, func(i, j int) bool { return arr[i].score > arr[j].score })
	out := make([]string, 0, cap)
	for i := 0; i < cap && i < len(arr); i++ {
		out = append(out, arr[i].name)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

func tokenize(s string) []string {
	var toks []string
	var cur strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			cur.WriteRune(r)
		} else {
			if cur.Len() >= 2 {
				toks = append(toks, cur.String())
			}
			cur.Reset()
		}
	}
	if cur.Len() >= 2 {
		toks = append(toks, cur.String())
	}
	return toks
}

// buildDashboardDigest 汇总看板各面板的当前值（即时查询），作为「AI 解读/优化/工单」的数据上下文。
func (s *Server) buildDashboardDigest(d Dashboard) string {
	var b strings.Builder
	b.WriteString("看板：" + d.Name + "\n")
	vars := dashVarMap(d.Vars)
	n := 0
	for _, p := range d.Panels {
		if n >= 40 { // 面板数量上限，防上下文膨胀
			break
		}
		if p.Type == "text" || p.Type == "logs" || len(p.Targets) == 0 {
			continue
		}
		dsID := p.DataSource
		if dsID == "" {
			dsID = d.DataSource
		}
		expr := substituteVars(p.Targets[0].Expr, vars, 60, 3600)
		vec, ok := s.dashVector(dsID, expr)
		title := p.Title
		if title == "" {
			title = p.Targets[0].Expr
		}
		if !ok || len(vec) == 0 {
			b.WriteString("- " + title + "：无数据\n")
			n++
			continue
		}
		parts := []string{}
		for i, se := range vec {
			if i >= 6 {
				parts = append(parts, "…")
				break
			}
			lbl := legendFromLabels(se.Labels)
			parts = append(parts, strings.TrimSpace(lbl+" "+fmtDigestVal(se.Value, p.Unit)))
		}
		unit := ""
		if p.Unit != "" {
			unit = "（" + p.Unit + "）"
		}
		b.WriteString("- " + title + unit + "：" + strings.Join(parts, "; ") + "\n")
		n++
	}
	return b.String()
}

func legendFromLabels(labels map[string]string) string {
	if labels == nil {
		return ""
	}
	if inst := labels["instance"]; inst != "" {
		return inst
	}
	if job := labels["job"]; job != "" {
		return job
	}
	if nm := labels["__name__"]; nm != "" {
		return nm
	}
	return ""
}

func fmtDigestVal(v float64, unit string) string {
	switch unit {
	case "percent":
		return fmt.Sprintf("%.1f%%", v)
	case "percentunit":
		return fmt.Sprintf("%.1f%%", v*100)
	case "bytes", "Bps":
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprintf("%.4g", v)
	}
}

// ---- HTTP 端点 ----

// handleAICreateDashboard 后台异步生成看板：立即返回 queued，生成过程（较慢的 LLM 调用）
// 放到 goroutine，完成/失败后经消息中心（顶栏 🔔）推送弹窗反馈，避免前端长时间卡顿。
func (s *Server) handleAICreateDashboard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
		Name   string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请描述你想要的看板内容"})
		return
	}
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.Endpoint == "" || cfg.Model == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "AI 未配置或未启用，请先在「AI 设置」填写并保存"})
		return
	}
	name := strings.TrimSpace(req.Name)
	actor := s.clientIP(r)
	go func() {
		defer func() { _ = recover() }()
		d, warns, err := s.generateDashboardViaAI(prompt, "", "ai")
		if err != nil {
			s.messages.push("ai", "warning", "AI 看板生成失败", err.Error(), "dashboards", "")
			return
		}
		if name != "" {
			d.Name = name
			d, _ = s.cfg.UpsertDashboard(d)
		}
		body := "共 " + itoa(len(d.Panels)) + " 面板，点击查看"
		if len(warns) > 0 {
			body += "（" + itoa(len(warns)) + " 处提示）"
		}
		s.messages.push("ai", "success", "AI 看板已生成："+d.Name, body, "dashboards", d.ID)
		s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: actor, Message: "AI 生成看板：" + d.Name + "（" + itoa(len(d.Panels)) + " 面板）"})
	}()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "queued": true})
}

// handleApplyDashOptimize 把 AI 优化产出的看板 JSON 应用到现有看板（保留 id / 数据源）。
func (s *Server) handleApplyDashOptimize(w http.ResponseWriter, r *http.Request) {
	cur, ok := s.cfg.DashboardByID(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "仪表盘不存在"})
		return
	}
	var req struct {
		JSON string `json:"json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	spec, ok := decodeAIDashSpec(req.JSON)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "未在 AI 回复中找到可解析的看板 JSON"})
		return
	}
	d, warns := sanitizeAIDash(spec, cur.Name, cur.Source)
	if len(d.Panels) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "AI 未给出有效面板，未应用"})
		return
	}
	// 原地更新：保留 id / 数据源 / 描述 / 标签（AI 若给了名则用新名，否则保留原名）
	d.ID = cur.ID
	d.DataSource = cur.DataSource
	d.Description = cur.Description
	d.Tags = cur.Tags
	if spec.specName() == "" {
		d.Name = cur.Name
	}
	saved, err := s.cfg.UpsertDashboard(d)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: "应用 AI 看板优化：" + saved.Name})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID, "panels": len(saved.Panels), "warnings": warns})
}

func (s *Server) handleAIDashboardFromIncident(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IncidentID int64 `json:"incident_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	inc := s.incidents.find(req.IncidentID)
	if inc == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "事件不存在"})
		return
	}
	title, hostname, hostID, typ, sev := inc.Title, inc.Hostname, inc.HostID, inc.Type, inc.Severity
	need := "为一个正在排障的运维事件生成【分析看板】，聚焦定位该事件根因所需的关键指标（黄金信号：饱和度/错误/延迟/流量，以及相关资源使用率）。"
	seed := "事件标题：" + title + "\n严重级别：" + sev
	if hostname != "" {
		seed += "\n受影响主机：" + hostname
		need += "尽量用模板变量或表达式聚焦到该主机（instance/hostname 相关标签）。"
	}
	if typ != "" {
		seed += "\n告警类型：" + typ
	}
	if hostID != "" {
		seed += "\n主机ID：" + hostID
	}
	d, warns, err := s.generateDashboardViaAI(need, seed, "ai-analysis:incident:"+itoa64(req.IncidentID))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// 命名 + 事件时间线留痕
	d.Name = "🔎 事件分析：" + title
	d, _ = s.cfg.UpsertDashboard(d)
	s.incidents.AddEvent(req.IncidentID, "note", "AI", "已生成分析看板「"+d.Name+"」用于排障")
	s.store.MarkDirty()
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.clientIP(r), Message: "AI 按事件生成分析看板：" + d.Name})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": d.ID, "name": d.Name, "panels": len(d.Panels), "warnings": warns})
}

// handleDashboardDigest 返回看板实时数据摘要，供前端作为 /ai/assist 解读/优化的上下文。
func (s *Server) handleDashboardDigest(w http.ResponseWriter, r *http.Request) {
	d, ok := s.cfg.DashboardByID(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "仪表盘不存在"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"digest": s.buildDashboardDigest(d), "structure": dashStructureText(d)})
}

// dashStructureText 把看板结构（面板/类型/查询/单位）转成文本，供「AI 优化」审阅。
func dashStructureText(d Dashboard) string {
	var b strings.Builder
	b.WriteString("看板结构：" + d.Name + "\n")
	if len(d.Vars) > 0 {
		var vs []string
		for _, v := range d.Vars {
			vs = append(vs, v.Name+"("+v.Type+")")
		}
		b.WriteString("模板变量：" + strings.Join(vs, ", ") + "\n")
	}
	for _, p := range d.Panels {
		b.WriteString("- [" + p.Type + "] " + p.Title)
		if p.Unit != "" {
			b.WriteString(" 单位=" + p.Unit)
		}
		b.WriteString("\n")
		for _, t := range p.Targets {
			b.WriteString("    " + t.Expr + "\n")
		}
	}
	return b.String()
}

// handleDashboardAITicket 基于看板实时研判生成工单草案（AI 给标题/优先级/摘要）并创建。
func (s *Server) handleDashboardAITicket(w http.ResponseWriter, r *http.Request) {
	d, ok := s.cfg.DashboardByID(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "仪表盘不存在"})
		return
	}
	cfg := s.cfg.AIConfig()
	if !cfg.Enabled || cfg.Endpoint == "" || cfg.Model == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "AI 未配置或未启用"})
		return
	}
	// 前端已带真实选中变量值的数据摘要优先（服务端摘要因 d.Vars.Current 为空、变量替换成空而查不到数据）。
	var req struct {
		Digest string `json:"digest"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	digest := strings.TrimSpace(req.Digest)
	if digest == "" {
		digest = s.buildDashboardDigest(d)
	}
	sys := "你是 SRE 值班工程师。基于以下监控看板的实时数据，判断是否存在需要跟进的问题，并产出一条【工单草案】。" +
		"严格只输出一个 JSON 对象：{\"needed\":true/false,\"title\":\"简明工单标题\",\"priority\":\"p1|p2|p3|p4\",\"summary\":\"问题摘要+建议处置（中文，可分点）\"}。" +
		"needed=false 表示当前无异常、无需建单。优先级：p1=严重故障影响服务，p2=重要异常需尽快处理，p3=一般问题，p4=优化项。只输出 JSON。"
	out, err := aiComplete(cfg, sys, digest)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "AI 研判失败：" + err.Error()})
		return
	}
	var draft struct {
		Needed   bool   `json:"needed"`
		Title    string `json:"title"`
		Priority string `json:"priority"`
		Summary  string `json:"summary"`
	}
	if js := extractJSONObject(out); js != "" {
		_ = json.Unmarshal([]byte(js), &draft)
	}
	if draft.Title == "" {
		draft.Title = "看板研判：" + d.Name
	}
	if !draft.Needed && draft.Summary == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "needed": false, "message": "AI 研判当前无明显异常，未创建工单。"})
		return
	}
	desc := draft.Summary + "\n\n———\n数据来源看板：" + d.Name + "（" + d.ID + "）\n\n" + digest
	tk, err := s.tickets.Create(Ticket{Title: draft.Title, Description: desc, Priority: draft.Priority}, s.actorName(r))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.store.MarkDirty()
	s.messages.push("ticket", "info", "AI 看板研判建单："+tk.Title, "优先级 "+tk.Priority, "sre", itoa64(tk.ID))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "needed": true, "ticket_id": tk.ID, "title": tk.Title, "priority": tk.Priority})
}

func itoa(n int) string     { return itoa64(int64(n)) }
func itoa64(n int64) string { return fmt.Sprintf("%d", n) }
