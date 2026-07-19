package main

import (
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
	Grid     DashGrid     `json:"grid"`
	Unit     string       `json:"unit,omitempty"` // percent|percentunit|bytes|Bps|s|ms|short|...（Grafana 单位串）
	Min      *float64     `json:"min,omitempty"`  // gauge/bargauge 量程
	Max      *float64     `json:"max,omitempty"`
	Decimals int          `json:"decimals,omitempty"`
	Text     string       `json:"text,omitempty"`     // type=text 的正文
	RawType  string       `json:"raw_type,omitempty"` // type=unsupported 时保留原 Grafana 类型
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
	cs.mu.Lock()
	now := time.Now().Unix()
	d.UpdatedAt = now
	if d.ID == "" {
		d.ID = genToken()[:8]
		d.CreatedAt = now
		cs.cfg.Dashboards = append(cs.cfg.Dashboards, d)
	} else {
		found := false
		for i := range cs.cfg.Dashboards {
			if cs.cfg.Dashboards[i].ID == d.ID {
				d.CreatedAt = cs.cfg.Dashboards[i].CreatedAt
				cs.cfg.Dashboards[i] = d
				found = true
				break
			}
		}
		if !found {
			d.CreatedAt = now
			cs.cfg.Dashboards = append(cs.cfg.Dashboards, d)
		}
	}
	cs.mu.Unlock()
	return d, cs.save()
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

// substituteVars 把表达式里的 $var / ${var} 替换为选中值，并处理 $__interval / $__range 内建量。
// vars 是「变量名 → 值」。多值变量的值应已由调用方组成 PromQL 正则（如 "a|b"）。
func substituteVars(expr string, vars map[string]string, stepSec, rangeSec int64) string {
	if expr == "" {
		return expr
	}
	return dashVarRe.ReplaceAllStringFunc(expr, func(m string) string {
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
