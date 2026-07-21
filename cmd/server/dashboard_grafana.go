package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ============================================================================
// 导入 Grafana 看板（务实子集）。
//
// 支持 timeseries/graph、stat/singlestat、gauge、bargauge、table、text 面板 + PromQL
// 查询目标 + 模板变量（label_values 解析）。无法映射的面板类型保留为 unsupported 占位
// （带原始查询），不静默丢弃。容忍多版本 schema：flat panels[] + 嵌套 row.panels + 旧 rows[]。
// ============================================================================

type grafanaDash struct {
	Title      string   `json:"title"`
	Tags       []string `json:"tags"`
	Templating struct {
		List []grafanaVar `json:"list"`
	} `json:"templating"`
	Panels []grafanaPanel `json:"panels"`
	Rows   []struct {
		Panels []grafanaPanel `json:"panels"`
	} `json:"rows"`
}

type grafanaVar struct {
	Name    string          `json:"name"`
	Label   string          `json:"label"`
	Type    string          `json:"type"`
	Query   json.RawMessage `json:"query"` // string 或 {query:...}
	Current struct {
		Value json.RawMessage `json:"value"` // string 或 []string
	} `json:"current"`
	Options []struct {
		Value string `json:"value"`
	} `json:"options"`
	Multi      bool `json:"multi"`
	IncludeAll bool `json:"includeAll"`
}

type grafanaPanel struct {
	ID      int      `json:"id"`
	Title   string   `json:"title"`
	Type    string   `json:"type"`
	GridPos DashGrid `json:"gridPos"`
	Targets []struct {
		Expr         string `json:"expr"`
		LegendFormat string `json:"legendFormat"`
		RefID        string `json:"refId"`
	} `json:"targets"`
	FieldConfig struct {
		Defaults struct {
			Unit     string   `json:"unit"`
			Min      *float64 `json:"min"`
			Max      *float64 `json:"max"`
			Decimals int      `json:"decimals"`
		} `json:"defaults"`
	} `json:"fieldConfig"`
	Yaxes []struct {
		Format string `json:"format"`
	} `json:"yaxes"`
	Format  string         `json:"format"`
	Content string         `json:"content"`
	Panels  []grafanaPanel `json:"panels"` // 折叠 row 内嵌面板
}

// mapGrafanaDashboard 把 Grafana 看板 JSON 映射为内部 Dashboard。
func mapGrafanaDashboard(raw []byte, nameOverride, source string) (Dashboard, error) {
	// grafana.com 下载多为裸看板模型；也可能包一层 {"dashboard": {...}}。
	var probe struct {
		Dashboard json.RawMessage `json:"dashboard"`
	}
	body := raw
	if json.Unmarshal(raw, &probe) == nil && len(probe.Dashboard) > 0 {
		body = probe.Dashboard
	}
	var g grafanaDash
	if err := json.Unmarshal(body, &g); err != nil {
		return Dashboard{}, fmt.Errorf("解析 Grafana JSON 失败：%v", err)
	}

	d := Dashboard{Source: source, Tags: g.Tags}
	d.Name = strings.TrimSpace(nameOverride)
	if d.Name == "" {
		d.Name = strings.TrimSpace(g.Title)
	}
	if d.Name == "" {
		d.Name = "导入的看板"
	}

	// 模板变量
	for _, gv := range g.Templating.List {
		dv, ok := mapGrafanaVar(gv)
		if ok {
			d.Vars = append(d.Vars, dv)
		}
	}

	// 面板：flat + 嵌套 row + 旧 rows[]
	var flat []grafanaPanel
	flat = append(flat, flattenPanels(g.Panels)...)
	for _, r := range g.Rows {
		flat = append(flat, flattenPanels(r.Panels)...)
	}
	nextID := 1
	for _, gp := range flat {
		p := mapGrafanaPanel(gp)
		if p.ID == 0 {
			p.ID = nextID
		}
		if p.ID >= nextID {
			nextID = p.ID + 1
		}
		d.Panels = append(d.Panels, p)
	}
	if len(d.Panels) == 0 {
		return Dashboard{}, fmt.Errorf("未从该看板解析到任何面板")
	}
	sortPanels(d.Panels)
	healImportedDashboard(&d)
	return d, nil
}

// flattenPanels 展开 row 型面板的内嵌 panels（折叠行），返回可渲染面板列表。
func flattenPanels(panels []grafanaPanel) []grafanaPanel {
	var out []grafanaPanel
	for _, p := range panels {
		if p.Type == "row" {
			if len(p.Panels) > 0 {
				out = append(out, flattenPanels(p.Panels)...)
			}
			continue // 行标题本身不渲染为面板
		}
		out = append(out, p)
	}
	return out
}

func mapGrafanaVar(gv grafanaVar) (DashVar, bool) {
	switch gv.Type {
	case "query", "custom", "constant", "textbox":
	default:
		return DashVar{}, false // datasource/adhoc/interval 等跳过
	}
	dv := DashVar{
		Name: gv.Name, Label: gv.Label, Type: gv.Type,
		Multi: gv.Multi, IncludeAll: gv.IncludeAll,
		Query:   rawQueryString(gv.Query),
		Current: rawCurrentValue(gv.Current.Value),
	}
	for _, o := range gv.Options {
		if o.Value != "" && o.Value != "$__all" {
			dv.Options = append(dv.Options, o.Value)
		}
	}
	return dv, gv.Name != ""
}

func mapGrafanaPanel(gp grafanaPanel) DashPanel {
	p := DashPanel{
		ID: gp.ID, Title: gp.Title, Grid: gp.GridPos, Text: gp.Content,
		Min: gp.FieldConfig.Defaults.Min, Max: gp.FieldConfig.Defaults.Max,
		Decimals: gp.FieldConfig.Defaults.Decimals,
	}
	if p.Grid.W == 0 {
		p.Grid.W = 12
	}
	if p.Grid.H == 0 {
		p.Grid.H = 8
	}
	// 单位：新版 fieldConfig → 旧 graph yaxes → 旧 singlestat/gauge format
	p.Unit = gp.FieldConfig.Defaults.Unit
	if p.Unit == "" && len(gp.Yaxes) > 0 {
		p.Unit = gp.Yaxes[0].Format
	}
	if p.Unit == "" {
		p.Unit = gp.Format
	}
	// 目标（仅保留含 PromQL expr 的）
	for _, t := range gp.Targets {
		if strings.TrimSpace(t.Expr) == "" {
			continue
		}
		p.Targets = append(p.Targets, DashTarget{Expr: t.Expr, Legend: t.LegendFormat, RefID: t.RefID})
	}
	p.Type = mapGrafanaPanelType(gp.Type)
	// 已知类型但没有任何 PromQL 目标（如纯 text 之外的空面板）→ 若非 text 也无 expr，仍按其类型渲染空态。
	if p.Type == "unsupported" {
		p.RawType = gp.Type
	}
	return p
}

func mapGrafanaPanelType(t string) string {
	switch t {
	case "timeseries", "graph", "graph-old":
		return "timeseries"
	case "stat", "singlestat":
		return "stat"
	case "gauge":
		return "gauge"
	case "bargauge":
		return "bargauge"
	case "piechart", "grafana-piechart-panel", "piechart-old":
		return "piechart"
	case "barchart":
		return "barchart"
	case "histogram":
		return "histogram"
	case "state-timeline", "status-history":
		return "state-timeline"
	case "heatmap":
		return "heatmap"
	case "alertlist":
		return "alertlist"
	case "logs":
		return "logs"
	case "table", "table-old":
		return "table"
	case "text":
		return "text"
	default:
		return "unsupported"
	}
}

// rawQueryString 把变量 query（string 或 {query:...} 对象）取成字符串。
func rawQueryString(r json.RawMessage) string {
	if len(r) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(r, &s) == nil {
		return s
	}
	var obj struct {
		Query string `json:"query"`
	}
	if json.Unmarshal(r, &obj) == nil {
		return obj.Query
	}
	return ""
}

// rawCurrentValue 把变量 current.value（string 或 []string）取成字符串（多值用逗号连接）。
func rawCurrentValue(r json.RawMessage) string {
	if len(r) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(r, &s) == nil {
		return s
	}
	var arr []string
	if json.Unmarshal(r, &arr) == nil {
		return strings.Join(arr, ",")
	}
	return ""
}
