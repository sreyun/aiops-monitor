package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
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
	`  "vars": [{"name":"instance","label":"实例","type":"query","query":"label_values(aiops_cpu_percent, instance)"}],` + "\n" +
	`  "panels": [{"title":"面板标题","type":"timeseries|stat|gauge|piechart|barchart|bargauge|histogram|state-timeline|heatmap|table|alertlist|text","unit":"percent|percentunit|bytes|Bps|s|ms|reqps|short|","w":12,"h":8,` + "\n" +
	`     "targets":[{"expr":"<PromQL>","legend":"{{标签}}"}]}]` + "\n" +
	"}\n" +
	"要求：① 只用【可用指标】/【本平台内置指标】里真实存在的指标名，不要臆造 node_* / node_exporter 指标；" +
	"② 计数器类指标配合 rate()/irate()；本平台 aiops_*_percent、aiops_load1/5/15 已是水位/瞬时值，【禁止】再套 rate()/irate()；" +
	"③ 用量用 percent/bytes 等合适单位（运行时间/时长用 s，字节用 bytes，速率用 Bps，请求率用 reqps，比率用 percentunit）；④ 每个面板给贴切标题；" +
	"⑤ 【充分利用组件库、避免千篇一律的 timeseries】：随时间变化的趋势用 timeseries；单个关键当前值(运行时间/在线数/总量)用 stat；" +
	"占比/利用率(0-100%)用 gauge(圆环)；构成占比(各状态/各分区/各类型)用 piechart；类别排行 top-N 用 barchart；" +
	"多实例同一指标横向对比用 bargauge；数值分布用 histogram；可用性/状态随时间(up/down)用 state-timeline；" +
	"多实例密度对比用 heatmap；明细清单用 table；平台当前告警用 alertlist(无需查询)。一个高质量看板应混用至少 4 种不同 type，切忌全是 timeseries。" +
	"⑥ 【美观布局】：整体按黄金信号分区，从上到下 = 顶部一行 stat 概览(每个 w=6、h=4，2~4 个并排铺满一行) → 中部 timeseries 趋势(w=12、h=7，两个一行) → " +
	"底部 piechart/barchart/gauge/table 等构成与明细(piechart/barchart/table 用 w=12、h=7；gauge 用 w=8、h=6；bargauge 用 w=12、h=6)；" +
	"绝不要让单个 stat/gauge 占满整行或使用过大高度(会出现大片空白)；同一行面板保持等高，栅格每行合计 w=24 铺满不留缝；piechart 的切片控制在 3~8 个(过多切片改用 barchart)；" +
	"⑦ 模板变量名必须用英文 ASCII（如 instance），中文只写在 label；表达式里用 $instance，标签值 instance=主机名、host=主机ID；" +
	"全局概览/排行类面板不要强制带实例过滤；按实例下钻的面板用 instance=~\"$instance\"（必须 =~，兼容「全部」）；" +
	"⑧ 【图例可读性】legend 只用人类可读标签：优先 \"{{instance}}\"（主机名）；多分类时用 \"{{category}} · {{instance}}\"；" +
	"严禁使用 \"{{host}}\"（主机 ID 为 32 位十六进制，图例/悬浮提示会刷屏且不可读）。无合适标签时 legend 可留空；" +
	"⑨ 面板数量控制在 6-12 个，覆盖核心黄金信号且类型丰富。只输出 JSON，不要额外解释。"

// aiopsBuiltinMetricsHint 给「优化看板」等未注入 VM 全量指标的路径用：避免 LLM 臆造 node_*。
const aiopsBuiltinMetricsHint = "【本平台内置主机指标（优先使用）】\n" +
	"aiops_cpu_percent, aiops_cpu_cores, aiops_mem_percent, aiops_mem_used_bytes, aiops_mem_total_bytes, " +
	"aiops_disk_percent, aiops_disk_used_bytes, aiops_disk_total_bytes, aiops_disk_vol_percent, " +
	"aiops_load1, aiops_load5, aiops_load15, aiops_net_sent_rate, aiops_net_recv_rate, aiops_net_conns, " +
	"aiops_uptime_seconds, aiops_proc_count, aiops_disk_io_util_percent。\n" +
	"标签：instance=主机名（图例用这个），host=主机ID（仅过滤，禁止进图例），可选 category。" +
	"示例：avg(aiops_cpu_percent)、topk(10, aiops_mem_percent)、" +
	"aiops_cpu_percent{instance=~\"$instance\"}（仅下钻面板，务必 =~）、legend 写 \"{{instance}}\" 或 \"{{category}} · {{instance}}\"。"

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

	// 变量名规范化：LLM 常写「实例」等中文名，但 substituteVars 只认 ASCII \w，会导致 $instance 无法替换、趋势图空数据。
	varRename := map[string]string{}
	seenVar := map[string]bool{}
	for _, v := range spec.Vars {
		raw := strings.TrimSpace(v.Name)
		if raw == "" {
			continue
		}
		nameASCII, label := normalizeDashVarName(raw, v.Label)
		if nameASCII != raw {
			varRename[raw] = nameASCII
			warns = append(warns, "模板变量「"+raw+"」已规范为 "+nameASCII)
		}
		if seenVar[nameASCII] {
			continue // 主机/实例/节点 都归一成 instance 时去重，避免下拉出现两个「实例」
		}
		seenVar[nameASCII] = true
		typ := v.Type
		switch typ {
		case "query", "custom", "constant", "textbox":
		default:
			typ = "query"
		}
		query := healDashVarQuery(v.Query, nameASCII)
		includeAll := typ == "query" || typ == "custom"
		d.Vars = append(d.Vars, DashVar{Name: nameASCII, Label: label, Type: typ, Query: query, Options: v.Options, IncludeAll: includeAll})
	}
	// 表达式里若用了 $instance 但未声明变量，自动补一个，避免应用后趋势图空数据。
	needInstance := false
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
		panel := DashPanel{ID: id, Title: strings.TrimSpace(p.Title), Type: typ, Unit: healAIDashUnit(p.Unit), Text: p.Text}
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
			expr = rewriteDashVarRefs(expr, varRename)
			expr = healAIDashExpr(expr)
			if strings.Contains(expr, "$instance") || strings.Contains(expr, "${instance}") {
				needInstance = true
			}
			legend := rewriteDashVarRefs(t.targetLegend(), varRename)
			// 图例里的 {{实例}} 等中文占位也改成 {{instance}}
			for old, neu := range varRename {
				legend = strings.ReplaceAll(legend, "{{"+old+"}}", "{{"+neu+"}}")
			}
			legend = healAIDashLegend(legend)
			panel.Targets = append(panel.Targets, DashTarget{Expr: expr, Legend: legend})
		}
		if typ != "text" && typ != "alertlist" && len(panel.Targets) == 0 {
			warns = append(warns, "面板「"+panel.Title+"」无有效查询，已跳过")
			continue
		}
		d.Panels = append(d.Panels, panel)
		id++
	}
	if needInstance {
		has := false
		for _, v := range d.Vars {
			if v.Name == "instance" {
				has = true
				break
			}
		}
		if !has {
			d.Vars = append([]DashVar{{
				Name: "instance", Label: "实例", Type: "query", IncludeAll: true,
				Query: "label_values(aiops_cpu_percent, instance)",
			}}, d.Vars...)
			warns = append(warns, "已自动补充 instance 模板变量")
		}
	}
	layoutAIDashPanels(d.Panels)
	return d, warns
}

// normalizeDashVarName 把中文/别名变量名收成 ASCII，供 $var 替换；中文挪到 label。
func normalizeDashVarName(name, label string) (ascii, outLabel string) {
	n := strings.TrimSpace(name)
	outLabel = strings.TrimSpace(label)
	switch strings.ToLower(n) {
	case "instance", "host", "job", "category", "ident", "device", "ip":
		if outLabel == "" && n != "instance" {
			outLabel = n
		}
		if outLabel == "" {
			outLabel = "实例"
		}
		return n, outLabel
	}
	// 常见中文/别名 → instance
	for _, alias := range []string{"实例", "主机", "主机名", "节点", "机器", "服务器"} {
		if n == alias {
			if outLabel == "" {
				outLabel = alias
			}
			return "instance", outLabel
		}
	}
	// 非 ASCII 或含非法字符：尽量落到 instance，避免 $变量 无法匹配
	ok := true
	for _, r := range n {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			ok = false
			break
		}
	}
	if !ok {
		if outLabel == "" {
			outLabel = n
		}
		return "instance", outLabel
	}
	if outLabel == "" {
		outLabel = n
	}
	return n, outLabel
}

func healDashVarQuery(q, varName string) string {
	q = strings.TrimSpace(q)
	if q == "" && varName == "instance" {
		return "label_values(aiops_cpu_percent, instance)"
	}
	// 把常见错误的 label_values(node_uname_info, instance) 换成平台真实指标
	low := strings.ToLower(q)
	if strings.Contains(low, "label_values") && strings.Contains(low, "node_uname") {
		return "label_values(aiops_cpu_percent, instance)"
	}
	return q
}

func rewriteDashVarRefs(expr string, rename map[string]string) string {
	if expr == "" || len(rename) == 0 {
		return expr
	}
	out := expr
	for old, neu := range rename {
		if old == neu {
			continue
		}
		out = strings.ReplaceAll(out, "${"+old+"}", "${"+neu+"}")
		out = strings.ReplaceAll(out, "$"+old, "$"+neu)
	}
	return out
}

// healAIDashExpr 纠正常见「优化后无数据」写法：臆造的 node_*、对水位指标误套 rate()、
// 下钻过滤写成 instance="$instance"（「全部」时 =".*" 匹配不到，需 =~）。
func healAIDashExpr(expr string) string {
	if expr == "" {
		return expr
	}
	out := expr
	replacements := []struct{ old, neu string }{
		{"node_load1", "aiops_load1"},
		{"node_load5", "aiops_load5"},
		{"node_load15", "aiops_load15"},
		{"cpu_usage_active", "aiops_cpu_percent"},
		{"mem_used_percent", "aiops_mem_percent"},
		{"disk_used_percent", "aiops_disk_percent"},
	}
	for _, r := range replacements {
		out = strings.ReplaceAll(out, r.old, r.neu)
	}
	// rate(aiops_xxx_percent{…}[…]) / irate(...) → 直接取水位指标（允许中间带标签选择器）
	gaugeRate := regexp.MustCompile(`(?i)\b(?:rate|irate)\s*\(\s*(aiops_(?:cpu|mem|disk|swap)(?:_vol)?_percent|aiops_load(?:1|5|15)|aiops_disk_io_util_percent|aiops_uptime_seconds)(\s*\{[^}]*\})?\s*\[[^\]]+\]\s*\)`)
	out = gaugeRate.ReplaceAllString(out, "$1$2")
	// instance="$instance" 等 → =~ ，兼容「全部」变成 .*
	out = promoteTemplateVarEq(out, nil)
	return out
}

// healAIDashLegend 去掉图例里的 {{host}}（主机 ID），优先保留主机名/分类，避免图例刷屏。
func healAIDashLegend(legend string) string {
	leg := strings.TrimSpace(legend)
	if leg == "" {
		return "{{instance}}"
	}
	hasHost := regexp.MustCompile(`\{\{\s*host\s*\}\}`).MatchString(leg)
	if !hasHost {
		return leg
	}
	hasInst := regexp.MustCompile(`\{\{\s*instance\s*\}\}`).MatchString(leg)
	hasCat := regexp.MustCompile(`\{\{\s*category\s*\}\}`).MatchString(leg)
	if !hasInst && !hasCat {
		return "{{instance}}"
	}
	// 去掉 {{host}} 及其两侧分隔符
	reHost := regexp.MustCompile(`\s*[-–—·|/:]?\s*\{\{\s*host\s*\}\}\s*[-–—·|/:]?\s*`)
	leg = reHost.ReplaceAllString(leg, " · ")
	leg = regexp.MustCompile(`(\s*·\s*)+`).ReplaceAllString(leg, " · ")
	leg = strings.Trim(leg, " ·\t\r\n")
	if leg == "" {
		if hasCat && hasInst {
			return "{{category}} · {{instance}}"
		}
		return "{{instance}}"
	}
	return leg
}

func healAIDashUnit(u string) string {
	u = strings.TrimSpace(u)
	switch strings.ToLower(u) {
	case "", "short", "none":
		return u
	case "%", "百分比", "percent", "pct":
		return "percent"
	case "ratio", "percentunit", "0-1":
		return "percentunit"
	case "byte", "bytes", "b", "字节":
		return "bytes"
	case "bps", "bytes/s", "b/s", "字节/秒":
		return "Bps"
	case "sec", "secs", "second", "seconds", "秒", "时长":
		return "s"
	case "millisecond", "milliseconds", "毫秒":
		return "ms"
	case "req/s", "qps", "rps":
		return "reqps"
	default:
		return u
	}
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
		if h < 5 || h > 10 {
			return 7
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
	case "stat":
		// 单个 stat 占满整行会大片留白；概览行通常 2~4 个并排
		if w > 12 {
			return 6
		}
	case "piechart", "barchart", "table", "heatmap", "timeseries", "state-timeline", "histogram":
		if w < 8 { // 可视化面板过窄会挤压图例/坐标轴，最低给到 8 栏（1/3 行）
			return 8
		}
	case "gauge":
		if w < 6 {
			return 6
		}
		if w > 12 {
			return 8
		}
	}
	return w
}

// layoutAIDashPanels 按「黄金信号分区」重排并紧凑落位，避免 AI 随意顺序导致 stat/趋势混杂、行内留白。
// 分区：stat 概览 → gauge → 趋势类 → 构成/排行 → 明细/其它；同行等高；每行尽量铺满 24 栏。
func layoutAIDashPanels(panels []DashPanel) {
	if len(panels) == 0 {
		return
	}
	sectionRank := func(t string) int {
		switch t {
		case "stat":
			return 0
		case "gauge":
			return 1
		case "timeseries", "state-timeline", "histogram", "heatmap":
			return 2
		case "piechart", "barchart", "bargauge":
			return 3
		default:
			return 4
		}
	}
	sort.SliceStable(panels, func(i, j int) bool {
		return sectionRank(panels[i].Type) < sectionRank(panels[j].Type)
	})
	normalizeAISectionWidths(panels)

	x, y, rowStart, rowH := 0, 0, 0, 0
	flushRow := func(end int) {
		if end <= rowStart {
			return
		}
		for j := rowStart; j < end; j++ {
			panels[j].Grid.H = rowH
			panels[j].Grid.Y = y
		}
		y += rowH
		x, rowH, rowStart = 0, 0, end
	}
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
			flushRow(i)
		}
		panels[i].Grid = DashGrid{X: x, Y: y, W: w, H: h}
		x += w
		if h > rowH {
			rowH = h
		}
	}
	flushRow(len(panels))
}

// normalizeAISectionWidths 让同类型连续段的宽度更整齐（如 3 个 stat → 各 8；4 个 → 各 6）。
func normalizeAISectionWidths(panels []DashPanel) {
	i := 0
	for i < len(panels) {
		typ := panels[i].Type
		j := i + 1
		for j < len(panels) && panels[j].Type == typ {
			j++
		}
		n := j - i
		if n >= 2 && (typ == "stat" || typ == "gauge") {
			// 优先整除 24：2→12、3→8、4→6；超过 4 个用 6 并由外层换行。
			w := 24 / n
			if n > 4 {
				w = 6
			}
			if w < 4 {
				w = 4
			}
			if typ == "gauge" && w > 12 {
				w = 8
			}
			extra := 24 - w*n
			if n > 4 || extra < 0 {
				extra = 0
			}
			for k := i; k < j; k++ {
				ww := w
				if extra > 0 {
					ww++
					extra--
				}
				if ww > 24 {
					ww = 24
				}
				panels[k].Grid.W = ww
				if typ == "stat" {
					panels[k].Grid.H = 4
				} else {
					panels[k].Grid.H = 6
				}
			}
		} else if typ == "timeseries" || typ == "piechart" || typ == "barchart" || typ == "table" {
			for k := i; k < j; k++ {
				if panels[k].Grid.W < 8 {
					panels[k].Grid.W = 12
				}
				if panels[k].Grid.H < 6 {
					panels[k].Grid.H = 7
				}
			}
		}
		i = j
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
		"面板用 PromQL 查询。禁止深度思考与思维链，直接输出最终 JSON。\n" + aiDashSchemaHint + "\n" + aiopsBuiltinMetricsHint
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
	cat := labels["category"]
	inst := labels["instance"]
	if inst == "" {
		inst = labels["hostname"]
	}
	if cat != "" && inst != "" {
		return cat + " · " + inst
	}
	if inst != "" {
		return inst
	}
	if cat != "" {
		return cat
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
	// 干跑：对各指标面板做一次即时查询；若全部无数据则拒绝应用，部分无数据则写入 warnings。
	vars := dashVarMap(d.Vars)
	var emptyTitles []string
	metricN := 0
	for _, p := range d.Panels {
		if p.Type == "text" || p.Type == "logs" || p.Type == "alertlist" || len(p.Targets) == 0 {
			continue
		}
		metricN++
		dsID := p.DataSource
		if dsID == "" {
			dsID = d.DataSource
		}
		if dsID == "" {
			dsID = cur.DataSource
		}
		expr := substituteVars(p.Targets[0].Expr, vars, 60, 3600)
		vec, ok := s.dashVector(dsID, expr)
		if !ok || len(vec) == 0 {
			title := p.Title
			if title == "" {
				title = p.Targets[0].Expr
			}
			emptyTitles = append(emptyTitles, title)
		}
	}
	if metricN > 0 && len(emptyTitles) == metricN {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":             false,
			"error":          "干跑校验失败：所有指标面板即时查询均无数据，未应用。请检查 PromQL / 数据源后重试。",
			"dry_run_empty":  emptyTitles,
			"warnings":       warns,
		})
		return
	}
	if len(emptyTitles) > 0 {
		preview := emptyTitles
		if len(preview) > 5 {
			preview = preview[:5]
		}
		warns = append(warns, fmt.Sprintf("干跑：%d 个面板即时无数据（%s）", len(emptyTitles), strings.Join(preview, "、")))
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": saved.ID, "panels": len(saved.Panels), "warnings": warns, "dry_run_empty": emptyTitles})
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
