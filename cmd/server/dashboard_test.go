package main

import (
	"strings"
	"testing"
)

const sampleGrafanaJSON = `{
  "title": "Node 概览",
  "tags": ["prod","node"],
  "templating": { "list": [
    { "name": "instance", "type": "query", "query": "label_values(node_uname_info, instance)", "current": {"value": "10.0.0.1"}, "multi": true, "includeAll": true },
    { "name": "env", "type": "custom", "current": {"value": ["a","b"]}, "options": [{"value":"$__all"},{"value":"a"},{"value":"b"}] },
    { "name": "ds", "type": "datasource", "query": "prometheus" }
  ]},
  "panels": [
    { "id": 1, "type": "timeseries", "title": "CPU", "gridPos": {"x":0,"y":0,"w":12,"h":8},
      "fieldConfig": {"defaults": {"unit": "percent"}},
      "targets": [ {"expr": "100 - avg(rate(node_cpu_seconds_total{mode=\"idle\",instance=\"$instance\"}[5m]))*100", "legendFormat": "{{instance}}"} ] },
    { "id": 2, "type": "graph", "title": "内存", "gridPos": {"x":12,"y":0,"w":12,"h":8},
      "yaxes": [ {"format": "bytes"} ],
      "targets": [ {"expr": "node_memory_MemAvailable_bytes{instance=\"$instance\"}"} ] },
    { "id": 3, "type": "stat", "title": "在线", "gridPos": {"x":0,"y":8,"w":6,"h":2},
      "targets": [ {"expr": "up{instance=\"$instance\"}"} ] },
    { "id": 6, "type": "stat", "title": "负载", "gridPos": {"x":6,"y":8,"w":6,"h":2},
      "targets": [ {"expr": "node_load1{instance=\"[[instance]]\"}"} ] },
    { "id": 10, "type": "row", "title": "分组", "panels": [
      { "id": 4, "type": "gauge", "title": "磁盘", "gridPos": {"x":0,"y":10,"w":6,"h":6},
        "fieldConfig": {"defaults": {"unit": "percentunit", "max": 1}},
        "targets": [ {"expr": "disk_used_ratio"} ] }
    ]},
    { "id": 5, "type": "nodeGraph", "title": "占比", "gridPos": {"x":6,"y":10,"w":6,"h":4},
      "targets": [ {"expr": "sum by (job)(up)"} ] }
  ]
}`

func TestNormalizeDashboardAllowsLargeGrafanaTemplates(t *testing.T) {
	// Node Exporter Full (1860) expands to ~120–140 panels; previously capped at 120.
	d := Dashboard{Name: "large"}
	for i := 0; i < 140; i++ {
		d.Panels = append(d.Panels, DashPanel{
			ID: i + 1, Title: "p", Type: "stat",
			Grid:    DashGrid{X: 0, Y: i * 4, W: 6, H: 4},
			Targets: []DashTarget{{Expr: "up"}},
		})
	}
	if err := normalizeDashboard(&d); err != nil {
		t.Fatalf("140 面板应通过校验（兼容 Grafana 1860）：%v", err)
	}
}

func TestMapGrafanaEmptyTargetsBecomeUnsupported(t *testing.T) {
	raw := `{
	  "title":"mixed",
	  "panels":[
	    {"id":1,"type":"timeseries","title":"OK","gridPos":{"x":0,"y":0,"w":12,"h":8},
	     "targets":[{"expr":"up"}]},
	    {"id":2,"type":"timeseries","title":"NoExpr","gridPos":{"x":12,"y":0,"w":12,"h":8},
	     "targets":[{"expr":""}]},
	    {"id":3,"type":"text","title":"Note","gridPos":{"x":0,"y":8,"w":24,"h":3},
	     "content":"hi"}
	  ]
	}`
	d, err := mapGrafanaDashboard([]byte(raw), "", "grafana:test")
	if err != nil {
		t.Fatal(err)
	}
	if err := normalizeDashboard(&d); err != nil {
		t.Fatalf("含无 PromQL 面板的看板仍应可导入：%v", err)
	}
	by := map[string]DashPanel{}
	for _, p := range d.Panels {
		by[p.Title] = p
	}
	if by["NoExpr"].Type != "unsupported" {
		t.Fatalf("无 expr 的 timeseries 应降级 unsupported，实为 %q", by["NoExpr"].Type)
	}
	if by["OK"].Type != "timeseries" || by["Note"].Type != "text" {
		t.Fatalf("正常面板被误伤: %+v", by)
	}
}

func TestUpsertDashboardRevisionConflict(t *testing.T) {
	dir := t.TempDir()
	cs := &ConfigStore{path: dir + "/cfg.json", cfg: ServerConfig{}}
	saved, err := cs.UpsertDashboard(Dashboard{Name: "r", Panels: []DashPanel{
		{Title: "a", Type: "stat", Grid: DashGrid{W: 6, H: 4}, Targets: []DashTarget{{Expr: "up"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 1 {
		t.Fatalf("revision=%d", saved.Revision)
	}
	next := saved
	next.Name = "r2"
	if _, err := cs.UpsertDashboardIfRevision(next, saved.Revision); err != nil {
		t.Fatal(err)
	}
	next.Name = "r3"
	if _, err := cs.UpsertDashboardIfRevision(next, saved.Revision); !errDashboardRevisionConflict(err) {
		t.Fatalf("期望 revision conflict，实为 %v", err)
	}
}

func TestMapGrafanaDashboard(t *testing.T) {
	d, err := mapGrafanaDashboard([]byte(sampleGrafanaJSON), "", "grafana:1860")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Node 概览" {
		t.Fatalf("名称错误: %q", d.Name)
	}
	if d.Source != "grafana:1860" {
		t.Fatalf("来源错误: %q", d.Source)
	}
	// 变量：query + custom 保留，datasource 跳过
	if len(d.Vars) != 2 {
		t.Fatalf("变量数应为 2（query+custom，跳过 datasource），实为 %d", len(d.Vars))
	}
	if d.Vars[0].Name != "instance" || d.Vars[0].Type != "query" || !d.Vars[0].Multi || !d.Vars[0].IncludeAll {
		t.Fatalf("query 变量映射错误: %+v", d.Vars[0])
	}
	if _, label, ok := parseLabelValues(d.Vars[0].Query); !ok || label != "instance" {
		t.Fatalf("label_values 解析错误: q=%q", d.Vars[0].Query)
	}
	if d.Vars[1].Type != "custom" || len(d.Vars[1].Options) != 2 { // $__all 被剔除
		t.Fatalf("custom 变量映射错误: %+v", d.Vars[1])
	}
	// 面板：row 展平 → 6 个（含新增 load stat）
	if len(d.Panels) != 6 {
		t.Fatalf("面板数应为 6（row 展平），实为 %d", len(d.Panels))
	}
	byID := map[int]DashPanel{}
	for _, p := range d.Panels {
		byID[p.ID] = p
	}
	if byID[2].Type != "timeseries" || byID[2].Unit != "bytes" {
		t.Fatalf("旧 graph 应映射为 timeseries + yaxes 单位 bytes: %+v", byID[2])
	}
	if byID[1].Type != "timeseries" || byID[1].Unit != "percent" || len(byID[1].Targets) != 1 || byID[1].Targets[0].Legend != "{{instance}}" {
		t.Fatalf("timeseries 面板映射错误: %+v", byID[1])
	}
	// 导入时固化 =~（含经典 [[instance]]）
	if byID[1].Targets[0].Expr != `100 - avg(rate(node_cpu_seconds_total{mode="idle",instance=~"$instance"}[5m]))*100` {
		t.Fatalf("导入应把 instance=\"$instance\" 提升为 =~: %q", byID[1].Targets[0].Expr)
	}
	if byID[6].Targets[0].Expr != `node_load1{instance=~"$instance"}` {
		t.Fatalf("经典 [[instance]] 应展开并提升 =~: %q", byID[6].Targets[0].Expr)
	}
	if byID[4].Type != "gauge" || byID[4].Unit != "percentunit" || byID[4].Max == nil || *byID[4].Max != 1 {
		t.Fatalf("嵌套 gauge 映射错误: %+v", byID[4])
	}
	if byID[5].Type != "nodegraph" {
		t.Fatalf("nodeGraph 应映射为 nodegraph（前端即将支持占位）: %+v", byID[5])
	}
	if byID[1].Grid.W != 12 || byID[3].Grid.W != 6 {
		t.Fatalf("gridPos 宽度未保留")
	}
	// Grafana 过矮 KPI（h=2）导入后抬到 ≥3；过高则压到紧凑高度
	if byID[3].Grid.H < 3 || byID[3].Grid.H > 5 {
		t.Fatalf("stat 高度应在 3~5，实为 %d", byID[3].Grid.H)
	}
}

func TestHealImportedDashboardOverlap(t *testing.T) {
	d := Dashboard{
		Panels: []DashPanel{
			{ID: 1, Title: "A", Grid: DashGrid{X: 0, Y: 0, W: 12, H: 3}},
			{ID: 2, Title: "B", Grid: DashGrid{X: 0, Y: 2, W: 12, H: 3}}, // 与 A 重叠
		},
	}
	if !healImportedDashboard(&d) {
		t.Fatal("重叠布局应被修复并标记 changed")
	}
	if panelsGridOverlap(d.Panels) {
		t.Fatalf("修复后仍重叠: %+v %+v", d.Panels[0].Grid, d.Panels[1].Grid)
	}
	if d.Panels[1].Grid.Y < d.Panels[0].Grid.Y+d.Panels[0].Grid.H {
		t.Fatalf("B 应被下推到 A 下方: A=%+v B=%+v", d.Panels[0].Grid, d.Panels[1].Grid)
	}
}

func TestHealImportedDashboardShortStat(t *testing.T) {
	d := Dashboard{
		Panels: []DashPanel{
			{ID: 1, Title: "在线主机数", Type: "stat", Grid: DashGrid{X: 0, Y: 0, W: 6, H: 2}, Targets: []DashTarget{{Expr: "up"}}},
			{ID: 2, Title: "CPU", Type: "stat", Grid: DashGrid{X: 6, Y: 0, W: 6, H: 2}, Targets: []DashTarget{{Expr: "cpu"}}},
			{ID: 3, Title: "趋势", Type: "timeseries", Grid: DashGrid{X: 0, Y: 4, W: 12, H: 8}, Targets: []DashTarget{{Expr: "x"}}},
		},
	}
	if !healImportedDashboard(&d) {
		t.Fatal("过矮 KPI 应被抬高")
	}
	for _, p := range d.Panels {
		if p.Type == "stat" && (p.Grid.H < 3 || p.Grid.H > 5) {
			t.Fatalf("stat「%s」高度应在 3~5，实为 %d", p.Title, p.Grid.H)
		}
	}
	if panelsGridOverlap(d.Panels) {
		t.Fatal("抬高 KPI 后不应重叠")
	}
	var trend DashPanel
	for _, p := range d.Panels {
		if p.Title == "趋势" {
			trend = p
		}
	}
	if trend.Grid.Y < 3 {
		t.Fatalf("趋势面板应被下推到 KPI 下方，y=%d", trend.Grid.Y)
	}
}

func TestCompactPanelGridRemovesGaps(t *testing.T) {
	// 模拟 Grafana 折叠行：顶部面板 y=0，折叠区内嵌面板仍保留绝对 y=80 → 大片断层
	panels := []DashPanel{
		{ID: 1, Title: "top-a", Grid: DashGrid{X: 0, Y: 0, W: 12, H: 8}},
		{ID: 2, Title: "top-b", Grid: DashGrid{X: 12, Y: 0, W: 12, H: 8}},
		{ID: 3, Title: "nested", Grid: DashGrid{X: 0, Y: 80, W: 24, H: 8}},
		{ID: 4, Title: "far", Grid: DashGrid{X: 0, Y: 200, W: 12, H: 6}},
		{ID: 5, Title: "far-b", Grid: DashGrid{X: 12, Y: 200, W: 12, H: 6}},
	}
	if !compactPanelGrid(panels) {
		t.Fatal("应消除垂直空洞")
	}
	byID := map[int]DashPanel{}
	for _, p := range panels {
		byID[p.ID] = p
	}
	if byID[1].Grid.Y != 0 || byID[2].Grid.Y != 0 {
		t.Fatalf("顶部行应留在 y=0: %+v %+v", byID[1].Grid, byID[2].Grid)
	}
	if byID[3].Grid.Y != 8 {
		t.Fatalf("嵌套面板应上移到 y=8，实为 %d", byID[3].Grid.Y)
	}
	if byID[4].Grid.Y != 16 || byID[5].Grid.Y != 16 {
		t.Fatalf("远端行应紧贴上一行: %+v %+v", byID[4].Grid, byID[5].Grid)
	}
	if panelsGridOverlap(panels) {
		t.Fatal("紧凑后不应重叠")
	}
}

func TestHealImportedDashboardShrinksUnsupported(t *testing.T) {
	d := Dashboard{
		Panels: []DashPanel{
			{ID: 1, Type: "timeseries", Grid: DashGrid{X: 0, Y: 0, W: 12, H: 8}},
			{ID: 2, Type: "unsupported", RawType: "nodeGraph", Grid: DashGrid{X: 0, Y: 40, W: 12, H: 10}},
		},
	}
	if !healImportedDashboard(&d) {
		t.Fatal("应标记 changed（缩高 + 紧凑）")
	}
	var u DashPanel
	for _, p := range d.Panels {
		if p.ID == 2 {
			u = p
		}
	}
	if u.Grid.H != 3 {
		t.Fatalf("unsupported 高度应缩为 3，实为 %d", u.Grid.H)
	}
	if u.Grid.Y != 8 {
		t.Fatalf("紧凑后 unsupported 应在 timeseries 下方 y=8，实为 %d", u.Grid.Y)
	}
}

func TestLayoutAIDashPanelsSections(t *testing.T) {
	panels := []DashPanel{
		{ID: 1, Type: "timeseries", Title: "cpu", Grid: DashGrid{W: 12, H: 7}},
		{ID: 2, Type: "stat", Title: "up", Grid: DashGrid{W: 6, H: 4}},
		{ID: 3, Type: "stat", Title: "procs", Grid: DashGrid{W: 6, H: 4}},
		{ID: 4, Type: "gauge", Title: "mem", Grid: DashGrid{W: 8, H: 6}},
		{ID: 5, Type: "table", Title: "disk", Grid: DashGrid{W: 12, H: 7}},
	}
	layoutAIDashPanels(panels)
	if panels[0].Type != "stat" || panels[1].Type != "stat" {
		t.Fatalf("stat 应排在最前: %s %s", panels[0].Type, panels[1].Type)
	}
	if panels[0].Grid.Y != 0 || panels[1].Grid.Y != 0 {
		t.Fatalf("两个 stat 应同行: %+v %+v", panels[0].Grid, panels[1].Grid)
	}
	if panels[0].Grid.W+panels[1].Grid.W != 24 {
		t.Fatalf("stat 行应铺满 24，实为 %d+%d", panels[0].Grid.W, panels[1].Grid.W)
	}
	// 单个 gauge 独占整行，禁止被后续 timeseries 填半行
	if panels[2].Type != "gauge" || panels[2].Grid.Y != panels[0].Grid.H || panels[2].Grid.W != 24 {
		t.Fatalf("gauge 应独占整行铺满: %+v", panels[2].Grid)
	}
	if panels[3].Type != "timeseries" || panels[3].Grid.Y != panels[2].Grid.Y+panels[2].Grid.H || panels[3].Grid.W != 24 {
		t.Fatalf("timeseries 应在 gauge 下方整行: %+v", panels[3].Grid)
	}
	if panelsGridOverlap(panels) || aiDashLayoutNeedsTidy(panels) {
		t.Fatal("AI 布局不应重叠或残留错位")
	}
}

func TestLayoutAIDashPanelsNoCrossSectionPack(t *testing.T) {
	// 模拟「应用优化」后常见结构：4 KPI + 奇数趋势 + 对比图 + 单 gauge + 表
	panels := []DashPanel{
		{ID: 1, Type: "stat", Title: "a", Grid: DashGrid{W: 6, H: 4}},
		{ID: 2, Type: "stat", Title: "b", Grid: DashGrid{W: 6, H: 4}},
		{ID: 3, Type: "stat", Title: "c", Grid: DashGrid{W: 6, H: 4}},
		{ID: 4, Type: "stat", Title: "d", Grid: DashGrid{W: 6, H: 4}},
		{ID: 5, Type: "stat", Title: "e", Grid: DashGrid{W: 6, H: 4}}, // 5 KPI → 3+2，禁止 4+1 孤儿
		{ID: 6, Type: "timeseries", Title: "t1", Grid: DashGrid{W: 12, H: 8}},
		{ID: 7, Type: "timeseries", Title: "t2", Grid: DashGrid{W: 12, H: 8}},
		{ID: 8, Type: "timeseries", Title: "t3", Grid: DashGrid{W: 12, H: 8}}, // 奇数 → 末行 w=24
		{ID: 9, Type: "barchart", Title: "top", Grid: DashGrid{W: 12, H: 8}},
		{ID: 10, Type: "gauge", Title: "io", Grid: DashGrid{W: 12, H: 6}},
		{ID: 11, Type: "table", Title: "detail", Grid: DashGrid{W: 12, H: 7}},
	}
	layoutAIDashPanels(panels)

	// 分区顺序
	wantTypes := []string{"stat", "stat", "stat", "stat", "stat", "gauge", "timeseries", "timeseries", "timeseries", "barchart", "table"}
	for i, typ := range wantTypes {
		if panels[i].Type != typ {
			t.Fatalf("panels[%d] type=%s want %s", i, panels[i].Type, typ)
		}
	}
	// KPI：3+2 两行均铺满（紧凑行高 h=4）
	if panels[0].Grid.Y != 0 || panels[2].Grid.Y != 0 || panels[0].Grid.W != 8 || panels[0].Grid.H != 4 {
		t.Fatalf("首行 3 KPI 应各 w=8 h=4: %+v", panels[0].Grid)
	}
	if panels[3].Grid.Y != 4 || panels[4].Grid.Y != 4 || panels[3].Grid.W+panels[4].Grid.W != 24 {
		t.Fatalf("次行 2 KPI 应铺满: %+v %+v", panels[3].Grid, panels[4].Grid)
	}
	// gauge 不得与 timeseries 同行
	if panels[5].Type != "gauge" || panels[5].Grid.W != 24 {
		t.Fatalf("gauge 应整行: %+v", panels[5].Grid)
	}
	if panels[6].Grid.Y <= panels[5].Grid.Y {
		t.Fatalf("timeseries 不得与 gauge 同 y")
	}
	// 3 条趋势：两行半对 + 末行整宽
	if panels[6].Grid.W != 12 || panels[7].Grid.W != 12 || panels[6].Grid.Y != panels[7].Grid.Y {
		t.Fatalf("前两条趋势应并排: %+v %+v", panels[6].Grid, panels[7].Grid)
	}
	if panels[8].Grid.W != 24 || panels[8].Grid.Y != panels[6].Grid.Y+panels[6].Grid.H {
		t.Fatalf("奇数趋势末行应整宽: %+v", panels[8].Grid)
	}
	if aiDashLayoutNeedsTidy(panels) || panelsGridOverlap(panels) {
		t.Fatal("重排后仍有布局缺陷")
	}
}

func TestAISplitRowCountsAvoidOrphan(t *testing.T) {
	got := aiSplitRowCounts(5, 4)
	if len(got) != 2 || got[0] != 3 || got[1] != 2 {
		t.Fatalf("5 KPI max4 应为 [3,2]，实为 %v", got)
	}
	got = aiSplitRowCounts(3, 2)
	if len(got) != 2 || got[0] != 2 || got[1] != 1 {
		t.Fatalf("3 charts max2 应为 [2,1]，实为 %v", got)
	}
}

func TestLayoutAIDashPanelsGoldenSignalBoard(t *testing.T) {
	// 与「主机黄金信号总览」当前面板类型一致（应用优化后常见错位样本）
	titles := []string{
		"在线主机数", "集群 CPU 均值", "集群内存均值", "集群磁盘均值",
		"CPU 利用率与单核负载趋势", "磁盘读写吞吐趋势", "内存与 Swap 使用率趋势",
		"CPU 高水位主机 Top10", "网络收发吞吐趋势", "磁盘卷水位横向对比",
		"内存高水位主机 Top10", "磁盘 IO 利用率水位", "磁盘卷容量明细",
	}
	types := []string{
		"stat", "stat", "stat", "stat",
		"timeseries", "timeseries", "timeseries",
		"barchart", "timeseries", "bargauge",
		"barchart", "gauge", "table",
	}
	panels := make([]DashPanel, len(types))
	for i := range types {
		panels[i] = DashPanel{ID: i + 1, Type: types[i], Title: titles[i], Grid: DashGrid{W: 12, H: 8}}
	}
	layoutAIDashPanels(panels)
	if panels[0].Type != "stat" || panels[3].Grid.Y != 0 || panels[0].Grid.W != 6 {
		t.Fatalf("4 KPI 应同行各 w=6: %+v", panels[0].Grid)
	}
	if panels[4].Type != "gauge" || panels[4].Grid.W != 24 || panels[4].Grid.Y != 4 {
		t.Fatalf("gauge 应独占第二行: type=%s %+v", panels[4].Type, panels[4].Grid)
	}
	ts := 0
	for _, p := range panels {
		if p.Type == "timeseries" {
			ts++
			if p.Grid.Y < 9 {
				t.Fatalf("timeseries 不得压到 gauge 行: %+v", p.Grid)
			}
		}
	}
	if ts != 4 {
		t.Fatalf("timeseries count=%d", ts)
	}
	if aiDashLayoutNeedsTidy(panels) {
		t.Fatal("黄金信号样本重排后仍有布局缺陷")
	}
}

func TestPromoteTemplateVarEq(t *testing.T) {
	got := promoteTemplateVarEq(`up{job="$job",instance="${instance}"}`, []string{"job", "instance"})
	want := `up{job=~"$job",instance=~"${instance}"}`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if promoteTemplateVarEq(`up{instance=~"$instance"}`, []string{"instance"}) != `up{instance=~"$instance"}` {
		t.Fatal("已是 =~ 应保持不变")
	}
}

func TestSubstituteVarsAndDur(t *testing.T) {
	vars := map[string]string{"job": "web", "instance": "1.2.3.4"}
	got := substituteVars(`rate(x{job="$job",instance="${instance}"}[$__interval])`, vars, 60, 3600)
	want := `rate(x{job="web",instance="1.2.3.4"}[1m])`
	if got != want {
		t.Fatalf("变量替换错误:\n got=%s\nwant=%s", got, want)
	}
	// 未知变量原样保留
	if substituteVars("a$unknown", vars, 60, 3600) != "a$unknown" {
		t.Fatal("未知变量应原样保留")
	}
	// 「全部」/$__all → .*，并把 = 提升为 =~
	all := substituteVars(`aiops_cpu_percent{instance="$instance"}`, map[string]string{"instance": "$__all"}, 60, 3600)
	if all != `aiops_cpu_percent{instance=~".*"}` {
		t.Fatalf("$__all 应规范为 =~\".*\"，实为 %q", all)
	}
	empty := substituteVars(`aiops_cpu_percent{instance="$instance"}`, map[string]string{"instance": ""}, 60, 3600)
	if empty != `aiops_cpu_percent{instance=~".*"}` {
		t.Fatalf("空变量应规范为 =~\".*\"，实为 %q", empty)
	}
	for sec, exp := range map[int64]string{60: "1m", 90: "90s", 3600: "1h", 7200: "2h"} {
		if durLabel(sec) != exp {
			t.Fatalf("durLabel(%d)=%q，应为 %q", sec, durLabel(sec), exp)
		}
	}
}

func TestDashVarMap(t *testing.T) {
	m := dashVarMap([]DashVar{
		{Name: "a", Current: "x,y,z"},
		{Name: "b", Current: "$__all", IncludeAll: true},
	})
	if m["a"] != "x|y|z" {
		t.Fatalf("多值应组成正则 x|y|z，实为 %q", m["a"])
	}
	if m["b"] != ".*" {
		t.Fatalf("全选应为 .*，实为 %q", m["b"])
	}
}

func TestNormalizeDashboardTrustBoundary(t *testing.T) {
	d := Dashboard{
		Name: "  专家看板  ",
		Vars: []DashVar{{Name: "instance", Type: "query", Query: "label_values(up, instance)"}},
		Panels: []DashPanel{
			{ID: 1, Title: "A", Type: "timeseries", Grid: DashGrid{X: 22, Y: -5, W: 12, H: 99}, Targets: []DashTarget{{Expr: " up "}}},
			{ID: 1, Title: "B", Type: "text", Grid: DashGrid{}, Text: "<img src=x onerror=alert(1)>"},
		},
	}
	if err := normalizeDashboard(&d); err != nil {
		t.Fatal(err)
	}
	if d.Name != "专家看板" {
		t.Fatalf("名称应去空白，实为 %q", d.Name)
	}
	if d.Panels[0].Grid.X+d.Panels[0].Grid.W > 24 || d.Panels[0].Grid.Y < 0 || d.Panels[0].Grid.H > 48 {
		t.Fatalf("网格应被钳制: %+v", d.Panels[0].Grid)
	}
	if d.Panels[0].Targets[0].Expr != "up" {
		t.Fatalf("查询应去空白: %q", d.Panels[0].Targets[0].Expr)
	}
	if d.Panels[0].ID == d.Panels[1].ID || d.Panels[1].ID <= 0 {
		t.Fatalf("重复面板 ID 应被修复: %d %d", d.Panels[0].ID, d.Panels[1].ID)
	}
	// 服务端保留 Markdown 原文；真正 HTML 渲染由前端安全 Markdown 编码，不能在这里破坏用户文本。
	if d.Panels[1].Text == "" {
		t.Fatal("文本内容不应被静默丢弃")
	}
}

func TestNormalizeDashboardRejectsInvalidShape(t *testing.T) {
	d := Dashboard{Name: "x", Panels: []DashPanel{{ID: 1, Type: "shell", Grid: DashGrid{W: 12, H: 6}}}}
	if err := normalizeDashboard(&d); err != nil {
		t.Fatalf("未知面板类型应降级 unsupported，不应整板失败: %v", err)
	}
	if d.Panels[0].Type != "unsupported" || d.Panels[0].RawType != "shell" {
		t.Fatalf("未知类型应保留 raw_type: %+v", d.Panels[0])
	}
	d = Dashboard{Name: "x", Vars: []DashVar{{Name: "实例", Type: "query"}}, Panels: []DashPanel{}}
	if err := normalizeDashboard(&d); err == nil {
		t.Fatal("非 ASCII 模板变量名必须拒绝")
	}
}

func TestDashboardTextPanelUsesSafeMarkdownRenderer(t *testing.T) {
	dashboardJS, err := webFS.ReadFile("web/js/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(dashboardJS)
	if !strings.Contains(src, `renderAIMarkdown(p.text || "")`) {
		t.Fatal("文本组件必须经过安全 Markdown 渲染器")
	}
	if strings.Contains(src, `innerHTML = p.text`) || strings.Contains(src, `innerHTML=p.text`) {
		t.Fatal("文本组件不得把未可信内容直接写入 innerHTML")
	}

	sreJS, err := webFS.ReadFile("web/js/sre.js")
	if err != nil {
		t.Fatal(err)
	}
	md := string(sreJS)
	renderAt := strings.Index(md, "function renderAIMarkdown")
	if renderAt < 0 || !strings.Contains(md[renderAt:], "t=esc(t);") {
		t.Fatal("AI Markdown 必须在插入格式标签前先转义原始 HTML")
	}
}
