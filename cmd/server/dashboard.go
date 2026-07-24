package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// 仪表盘（Dashboards）——自定义可视化 + 导入 Grafana 看板。
//
// 面板查询走 VM（Prometheus 兼容），复用前端 Canvas 图表引擎渲染。布局按 Grafana
// 24 栏 gridPos 忠实还原（导入的看板不走样），自定义编辑用「宽度栏数 + 高度 + 排序」。
// 模板变量（$job/$instance…）在查询前于服务端替换，label_values(...) 经 VM 标签 API 解析。
// ============================================================================

// Dashboard 是一个仪表盘。
type Dashboard struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
	Vars        []DashVar   `json:"vars,omitempty"`
	Panels      []DashPanel `json:"panels"`
	DataSource  string      `json:"datasource,omitempty"` // 看板级默认数据源 id（""=内置 VictoriaMetrics）
	Source      string      `json:"source,omitempty"`     // "" | "grafana:<id>" | "import" | "ai"
	Revision    int64       `json:"revision,omitempty"`   // 乐观锁版本；每次成功保存递增
	CreatedAt   int64       `json:"created_at"`
	UpdatedAt   int64       `json:"updated_at"`
}

// DashVar 是一个模板变量。
type DashVar struct {
	Name       string   `json:"name"`              // 变量名（不含 $）
	Label      string   `json:"label,omitempty"`   // 显示名
	Type       string   `json:"type"`              // query | custom | constant | textbox
	Query      string   `json:"query,omitempty"`   // type=query：label_values(metric,label) / label_values(label) / 原始 PromQL
	Options    []string `json:"options,omitempty"` // type=custom：候选值
	Current    string   `json:"current,omitempty"` // 当前选中值
	Multi      bool     `json:"multi,omitempty"`
	IncludeAll bool     `json:"include_all,omitempty"`
}

// DashPanel 是一个面板。
type DashPanel struct {
	ID         int          `json:"id"`
	Title      string       `json:"title"`
	Type       string       `json:"type"`                 // timeseries | stat | gauge | bargauge | table | text | logs | unsupported
	DataSource string       `json:"datasource,omitempty"` // 面板级数据源 id（覆盖看板默认；""=继承看板默认）
	Targets    []DashTarget `json:"targets,omitempty"`
	Grid       DashGrid     `json:"grid"`
	Unit       string       `json:"unit,omitempty"` // percent|percentunit|bytes|Bps|s|ms|short|...（Grafana 单位串）
	Min        *float64     `json:"min,omitempty"`  // gauge/bargauge 量程
	Max        *float64     `json:"max,omitempty"`
	Decimals   int          `json:"decimals,omitempty"`
	Text       string       `json:"text,omitempty"`     // type=text 的正文
	RawType    string       `json:"raw_type,omitempty"` // type=unsupported 时保留原 Grafana 类型
}

// DashTarget 是面板的一条查询目标。
type DashTarget struct {
	Expr   string `json:"expr"`
	Legend string `json:"legend,omitempty"` // 图例格式，支持 {{label}}
	RefID  string `json:"ref_id,omitempty"`
}

// DashGrid 是 24 栏网格坐标（同 Grafana）。
type DashGrid struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// ---- ConfigStore CRUD（无密钥字段，机制同 PromRules） ----

func (cs *ConfigStore) Dashboards() []Dashboard {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make([]Dashboard, len(cs.cfg.Dashboards))
	copy(out, cs.cfg.Dashboards)
	return out
}

func (cs *ConfigStore) DashboardByID(id string) (Dashboard, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, d := range cs.cfg.Dashboards {
		if d.ID == id {
			return d, true
		}
	}
	return Dashboard{}, false
}

func (cs *ConfigStore) UpsertDashboard(d Dashboard) (Dashboard, error) {
	if err := normalizeDashboard(&d); err != nil {
		return Dashboard{}, err
	}
	cs.mu.Lock()
	now := time.Now().Unix()
	d.UpdatedAt = now
	if d.ID == "" {
		d.ID = genToken()[:8]
		d.Revision = 1
		d.CreatedAt = now
		cs.cfg.Dashboards = append(cs.cfg.Dashboards, d)
	} else {
		found := false
		for i := range cs.cfg.Dashboards {
			if cs.cfg.Dashboards[i].ID == d.ID {
				d.CreatedAt = cs.cfg.Dashboards[i].CreatedAt
				d.Revision = cs.cfg.Dashboards[i].Revision + 1
				if d.Revision < 1 {
					d.Revision = 1
				}
				cs.cfg.Dashboards[i] = d
				found = true
				break
			}
		}
		if !found {
			d.Revision = 1
			d.CreatedAt = now
			cs.cfg.Dashboards = append(cs.cfg.Dashboards, d)
		}
	}
	cs.mu.Unlock()
	return d, cs.save()
}

const (
	maxDashboardPanels  = 120
	maxDashboardTargets = 12
	maxDashboardVars    = 30
	maxDashboardText    = 64 << 10
	maxDashboardExpr    = 16 << 10
)

var dashVarNameValid = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

// normalizeDashboard 是所有手工编辑、导入和 AI 生成看板共同经过的信任边界。
// 它既限制载荷/查询规模，也把网格与标识规整到前端可安全渲染的范围。
func normalizeDashboard(d *Dashboard) error {
	if d == nil {
		return fmt.Errorf("仪表盘不能为空")
	}
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" {
		return fmt.Errorf("仪表盘名称不能为空")
	}
	if len([]rune(d.Name)) > 120 {
		return fmt.Errorf("仪表盘名称不能超过 120 个字符")
	}
	d.Description = strings.TrimSpace(d.Description)
	if len([]rune(d.Description)) > 2000 {
		return fmt.Errorf("仪表盘描述不能超过 2000 个字符")
	}
	if len(d.Tags) > 20 {
		return fmt.Errorf("仪表盘标签不能超过 20 个")
	}
	tagSeen := map[string]bool{}
	tags := d.Tags[:0]
	for _, tag := range d.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || tagSeen[tag] {
			continue
		}
		if len([]rune(tag)) > 40 {
			return fmt.Errorf("仪表盘标签不能超过 40 个字符")
		}
		tagSeen[tag] = true
		tags = append(tags, tag)
	}
	d.Tags = tags
	if len(d.DataSource) > 128 || len(d.Source) > 256 {
		return fmt.Errorf("仪表盘数据源或来源字段过长")
	}
	if len(d.Vars) > maxDashboardVars {
		return fmt.Errorf("模板变量不能超过 %d 个", maxDashboardVars)
	}
	varSeen := map[string]bool{}
	for i := range d.Vars {
		v := &d.Vars[i]
		v.Name = strings.TrimSpace(v.Name)
		if !dashVarNameValid.MatchString(v.Name) {
			return fmt.Errorf("模板变量名 %q 无效：仅支持英文、数字和下划线，且不能以数字开头", v.Name)
		}
		if varSeen[v.Name] {
			return fmt.Errorf("模板变量名 %q 重复", v.Name)
		}
		varSeen[v.Name] = true
		switch v.Type {
		case "query", "custom", "constant", "textbox":
		default:
			return fmt.Errorf("模板变量 %q 的类型无效", v.Name)
		}
		if len(v.Query) > maxDashboardExpr || len(v.Current) > 4096 || len([]rune(v.Label)) > 80 {
			return fmt.Errorf("模板变量 %q 的内容过长", v.Name)
		}
		if len(v.Options) > 500 {
			return fmt.Errorf("模板变量 %q 的候选值不能超过 500 个", v.Name)
		}
		for _, option := range v.Options {
			if len(option) > 4096 {
				return fmt.Errorf("模板变量 %q 的候选值过长", v.Name)
			}
		}
	}
	if d.Panels == nil {
		d.Panels = []DashPanel{}
	}
	if len(d.Panels) > maxDashboardPanels {
		return fmt.Errorf("面板不能超过 %d 个", maxDashboardPanels)
	}
	allowedTypes := map[string]bool{
		"timeseries": true, "stat": true, "gauge": true, "bargauge": true,
		"table": true, "text": true, "logs": true, "unsupported": true,
		"piechart": true, "pie": true, "barchart": true, "bar": true,
		"histogram": true, "state-timeline": true, "statetimeline": true,
		"heatmap": true, "alertlist": true,
	}
	panelIDs := map[int]bool{}
	nextID := 1
	for _, p := range d.Panels {
		if p.ID >= nextID {
			nextID = p.ID + 1
		}
	}
	for i := range d.Panels {
		p := &d.Panels[i]
		if !allowedTypes[p.Type] {
			return fmt.Errorf("面板 %q 的类型 %q 不受支持", p.Title, p.Type)
		}
		if p.ID <= 0 || panelIDs[p.ID] {
			for panelIDs[nextID] {
				nextID++
			}
			p.ID = nextID
			nextID++
		}
		panelIDs[p.ID] = true
		p.Title = strings.TrimSpace(p.Title)
		if len([]rune(p.Title)) > 160 {
			return fmt.Errorf("面板标题不能超过 160 个字符")
		}
		if len(p.DataSource) > 128 || len(p.Unit) > 64 || len(p.RawType) > 128 {
			return fmt.Errorf("面板 %q 的配置字段过长", p.Title)
		}
		if len(p.Text) > maxDashboardText {
			return fmt.Errorf("文本面板 %q 不能超过 64 KiB", p.Title)
		}
		if len(p.Targets) > maxDashboardTargets {
			return fmt.Errorf("面板 %q 的查询不能超过 %d 条", p.Title, maxDashboardTargets)
		}
		for j := range p.Targets {
			t := &p.Targets[j]
			t.Expr = strings.TrimSpace(t.Expr)
			t.Legend = strings.TrimSpace(t.Legend)
			if len(t.Expr) > maxDashboardExpr {
				return fmt.Errorf("面板 %q 的查询表达式不能超过 16 KiB", p.Title)
			}
			if len([]rune(t.Legend)) > 256 || len(t.RefID) > 32 {
				return fmt.Errorf("面板 %q 的图例或引用 ID 过长", p.Title)
			}
		}
		if p.Type != "text" && p.Type != "alertlist" && p.Type != "unsupported" && len(p.Targets) == 0 {
			return fmt.Errorf("面板 %q 至少需要一条查询", p.Title)
		}
		p.Grid.X = max(0, min(23, p.Grid.X))
		p.Grid.Y = max(0, min(10000, p.Grid.Y))
		p.Grid.W = max(1, min(24, p.Grid.W))
		if p.Grid.X+p.Grid.W > 24 {
			p.Grid.X = 24 - p.Grid.W
		}
		p.Grid.H = max(1, min(48, p.Grid.H))
		if p.Min != nil && (math.IsNaN(*p.Min) || math.IsInf(*p.Min, 0)) {
			return fmt.Errorf("面板 %q 的最小值无效", p.Title)
		}
		if p.Max != nil && (math.IsNaN(*p.Max) || math.IsInf(*p.Max, 0)) {
			return fmt.Errorf("面板 %q 的最大值无效", p.Title)
		}
		if p.Min != nil && p.Max != nil && *p.Min >= *p.Max {
			return fmt.Errorf("面板 %q 的最小值必须小于最大值", p.Title)
		}
	}
	return nil
}

func (cs *ConfigStore) DeleteDashboard(id string) error {
	cs.mu.Lock()
	kept := cs.cfg.Dashboards[:0]
	for _, d := range cs.cfg.Dashboards {
		if d.ID != id {
			kept = append(kept, d)
		}
	}
	cs.cfg.Dashboards = kept
	cs.mu.Unlock()
	return cs.save()
}

// ---- 模板变量替换 ----

var dashVarRe = regexp.MustCompile(`\$(?:\{(\w+)\}|(\w+))`)

// normalizeDashQueryVars 把前端传来的变量值规范成可安全替换的 PromQL 片段：
// 「全部」/$__all/空 → .* ；逗号多值 → a|b。
func normalizeDashQueryVars(vars map[string]string) map[string]string {
	if len(vars) == 0 {
		return vars
	}
	out := make(map[string]string, len(vars))
	for k, v := range vars {
		v = strings.TrimSpace(v)
		switch {
		case v == "" || v == "$__all" || v == "All" || v == ".*":
			out[k] = ".*"
		case strings.Contains(v, ","):
			parts := strings.Split(v, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			out[k] = strings.Join(parts, "|")
		default:
			out[k] = v
		}
	}
	return out
}

// promoteLabelEqToRegex 把 label="a|b" / label=".*" 提升为 label=~"..."，
// 否则「全部」替换成 .* 后仍用 = 会匹配不到任何序列。
var labelEqNeedsRegex = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)="([^"]*(?:\||\.\*)[^"]*)"`)

func promoteLabelEqToRegex(expr string) string {
	if expr == "" || (!strings.Contains(expr, "|") && !strings.Contains(expr, ".*")) {
		return expr
	}
	return labelEqNeedsRegex.ReplaceAllString(expr, `${1}=~"${2}"`)
}

// substituteVars 把表达式里的 $var / ${var} 替换为选中值，并处理 $__interval / $__range 内建量。
// vars 是「变量名 → 值」。多值 / 全选会规范为正则并自动把 = 提升为 =~。
func substituteVars(expr string, vars map[string]string, stepSec, rangeSec int64) string {
	if expr == "" {
		return expr
	}
	vars = normalizeDashQueryVars(vars)
	out := dashVarRe.ReplaceAllStringFunc(expr, func(m string) string {
		sub := dashVarRe.FindStringSubmatch(m)
		name := sub[1]
		if name == "" {
			name = sub[2]
		}
		switch name {
		case "__interval", "__rate_interval":
			return durLabel(stepSec)
		case "__range", "__range_s":
			return durLabel(rangeSec)
		case "__interval_ms":
			return strconv.FormatInt(stepSec*1000, 10)
		}
		if v, ok := vars[name]; ok {
			return v
		}
		return m // 未知变量原样保留（避免破坏含 $ 的合法表达式）
	})
	return promoteLabelEqToRegex(out)
}

// durLabel 把秒转成 Prometheus 时长串（如 90 → "90s"，600 → "10m"，7200 → "2h"）。
func durLabel(sec int64) string {
	if sec <= 0 {
		sec = 60
	}
	switch {
	case sec%3600 == 0:
		return strconv.FormatInt(sec/3600, 10) + "h"
	case sec%60 == 0:
		return strconv.FormatInt(sec/60, 10) + "m"
	default:
		return strconv.FormatInt(sec, 10) + "s"
	}
}

// resolveVarValues 解析一个模板变量的候选值：custom 直接给候选，query 走 label_values 解析。
// labelValues 注入 VM 标签查询（(label, matchMetric) → 取值集）。
func resolveVarValues(v DashVar, labelValues func(label, match string) ([]string, bool)) []string {
	switch v.Type {
	case "custom":
		return v.Options
	case "constant", "textbox":
		if v.Current != "" {
			return []string{v.Current}
		}
		return nil
	case "query":
		if labelValues == nil {
			return nil
		}
		metric, label, ok := parseLabelValues(v.Query)
		if !ok {
			return nil
		}
		vals, _ := labelValues(label, metric)
		return vals
	}
	return nil
}

var labelValuesRe = regexp.MustCompile(`^\s*label_values\s*\(\s*(?:(.+?)\s*,\s*)?([a-zA-Z_][a-zA-Z0-9_]*)\s*\)\s*$`)

// parseLabelValues 解析 Grafana 的 label_values(metric, label) 或 label_values(label)。
// 返回 (matchSelector, label, ok)。metric 为空表示对全部序列取该标签值。
func parseLabelValues(q string) (metric, label string, ok bool) {
	m := labelValuesRe.FindStringSubmatch(strings.TrimSpace(q))
	if m == nil {
		return "", "", false
	}
	metric = strings.TrimSpace(m[1])
	label = strings.TrimSpace(m[2])
	// metric 可能是裸指标名（node_up）或选择器（node_up{job="x"}）；裸名转成选择器供 match[] 用。
	if metric != "" && !strings.Contains(metric, "{") && isBareMetric(metric) {
		metric = metric + `{}`
	}
	return metric, label, label != ""
}

func isBareMetric(s string) bool {
	for _, r := range s {
		if !(r == '_' || r == ':' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return s != ""
}

// dashVarMap 把变量的当前值汇成替换用的 map；多值/全选组成 PromQL 正则替换体。
func dashVarMap(vars []DashVar) map[string]string {
	out := make(map[string]string, len(vars))
	for _, v := range vars {
		val := v.Current
		if v.IncludeAll && (val == "" || val == "$__all" || val == "All") {
			val = ".*" // 全选 → 正则通配（配合 =~ 使用）
		}
		// 逗号分隔的多值 → a|b|c
		if strings.Contains(val, ",") {
			parts := strings.Split(val, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			val = strings.Join(parts, "|")
		}
		out[v.Name] = val
	}
	return out
}

// sortPanels 按 gridPos (y, x) 排序，保证网格渲染顺序与 Grafana 一致。
func sortPanels(panels []DashPanel) {
	sort.SliceStable(panels, func(i, j int) bool {
		if panels[i].Grid.Y != panels[j].Grid.Y {
			return panels[i].Grid.Y < panels[j].Grid.Y
		}
		return panels[i].Grid.X < panels[j].Grid.X
	})
}

// expandGrafanaClassicVars 把 Grafana 旧式 [[var]] 写成 $var，便于后续统一替换与 =~ 提升。
func expandGrafanaClassicVars(expr string) string {
	if expr == "" || !strings.Contains(expr, "[[") {
		return expr
	}
	re := regexp.MustCompile(`\[\[([a-zA-Z_][a-zA-Z0-9_]*)\]\]`)
	return re.ReplaceAllString(expr, `$$$1`)
}

// promoteTemplateVarEq 把 label="$var" / label="${var}" 提升为 =~。
// 「全部」时服务端会把变量换成 .*，若仍用 = 则永远匹配不到序列——这是 Grafana 导入看板空图的首要原因。
func promoteTemplateVarEq(expr string, varNames []string) string {
	if expr == "" {
		return expr
	}
	names := varNames
	if len(names) == 0 {
		names = []string{"instance", "host", "job", "node", "ident", "category", "device", "ifname"}
	}
	out := expr
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || strings.HasPrefix(name, "__") {
			continue
		}
		// label="$name" 或 label="${name}"（允许标签与 = 之间空白）
		re := regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*)\s*=\s*"(\$\{` + regexp.QuoteMeta(name) + `\}|\$` + regexp.QuoteMeta(name) + `)"`)
		out = re.ReplaceAllString(out, `${1}=~"${2}"`)
	}
	return out
}

func dashGridOverlap(a, b DashGrid) bool {
	if a.W <= 0 || a.H <= 0 || b.W <= 0 || b.H <= 0 {
		return false
	}
	return a.X < b.X+b.W && a.X+a.W > b.X && a.Y < b.Y+b.H && a.Y+a.H > b.Y
}

func panelsGridOverlap(panels []DashPanel) bool {
	for i := 0; i < len(panels); i++ {
		for j := i + 1; j < len(panels); j++ {
			if dashGridOverlap(panels[i].Grid, panels[j].Grid) {
				return true
			}
		}
	}
	return false
}

// resolvePanelOverlaps 在尽量保留原 x/w/h 的前提下，把重叠面板下推，修复 Grafana 折叠行展平或
// 短面板被前端抬高 h 后留下的叠层（表现为中间一排「挤成细条」）。
func resolvePanelOverlaps(panels []DashPanel) {
	sortPanels(panels)
	for i := range panels {
		for {
			moved := false
			for j := 0; j < i; j++ {
				if !dashGridOverlap(panels[i].Grid, panels[j].Grid) {
					continue
				}
				newY := panels[j].Grid.Y + panels[j].Grid.H
				if panels[i].Grid.Y < newY {
					panels[i].Grid.Y = newY
					moved = true
				}
			}
			if !moved {
				break
			}
		}
	}
}

// compactPanelGrid 消除垂直空洞（Grafana 折叠行展平后常见「断层」）：
// 按 y,x 顺序，把每个面板在保持 x/w/h 的前提下尽量上移到不与上方水平相交面板冲突的最低 y。
// 与 resolvePanelOverlaps 互补：后者只处理重叠下推，本函数关闭非重叠空洞。
func compactPanelGrid(panels []DashPanel) bool {
	if len(panels) == 0 {
		return false
	}
	sortPanels(panels)
	changed := false
	for i := range panels {
		g := &panels[i].Grid
		minY := 0
		for j := 0; j < i; j++ {
			o := panels[j].Grid
			// 水平方向有交集时，必须落在对方底边之下
			if g.X < o.X+o.W && g.X+g.W > o.X {
				if bottom := o.Y + o.H; bottom > minY {
					minY = bottom
				}
			}
		}
		if g.Y != minY {
			g.Y = minY
			changed = true
		}
	}
	return changed
}

var hostLegendRe = regexp.MustCompile(`\{\{\s*host\s*\}\}`)

// healImportedDashboard 固化看板常见缺陷：=~ 提升、经典变量语法、去掉图例中的主机 ID、
// 网格重叠下推、纵向空洞紧凑、unsupported 占位缩高。
// 返回是否发生了修改（供 GET 时惰性回写）。
func healImportedDashboard(d *Dashboard) bool {
	if d == nil {
		return false
	}
	changed := false
	varNames := make([]string, 0, len(d.Vars))
	for i := range d.Vars {
		v := &d.Vars[i]
		if v.Name != "" {
			varNames = append(varNames, v.Name)
		}
		if v.IncludeAll && (v.Current == "" || v.Current == "All") {
			v.Current = "$__all"
			changed = true
		}
	}
	for i := range d.Panels {
		p := &d.Panels[i]
		if p.Grid.W <= 0 {
			p.Grid.W = 12
			changed = true
		}
		if p.Grid.H <= 0 {
			p.Grid.H = 8
			changed = true
		}
		if p.Grid.W > 24 {
			p.Grid.W = 24
			changed = true
		}
		// 不支持的面板仍保留查询信息，但缩成矮条，避免 1860 类看板里大块「空壳」占位造成断层感。
		if p.Type == "unsupported" && p.Grid.H > 3 {
			p.Grid.H = 3
			changed = true
		}
		for j := range p.Targets {
			expr := p.Targets[j].Expr
			neu := promoteTemplateVarEq(expandGrafanaClassicVars(expr), varNames)
			if neu != expr {
				p.Targets[j].Expr = neu
				changed = true
			}
			// 去掉图例中的 {{host}}（主机 ID），已有 AI 看板打开时惰性修复
			leg := p.Targets[j].Legend
			if hostLegendRe.MatchString(leg) {
				if neuLeg := healAIDashLegend(leg); neuLeg != leg {
					p.Targets[j].Legend = neuLeg
					changed = true
				}
			}
		}
	}
	sortPanels(d.Panels)
	if panelsGridOverlap(d.Panels) {
		resolvePanelOverlaps(d.Panels)
		changed = true
	}
	if compactPanelGrid(d.Panels) {
		changed = true
	}
	return changed
}
