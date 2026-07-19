package main

import "testing"

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
      "targets": [ {"expr": "100 - avg(rate(node_cpu_seconds_total{mode=\"idle\"}[5m]))*100", "legendFormat": "{{instance}}"} ] },
    { "id": 2, "type": "graph", "title": "内存", "gridPos": {"x":12,"y":0,"w":12,"h":8},
      "yaxes": [ {"format": "bytes"} ],
      "targets": [ {"expr": "node_memory_MemAvailable_bytes"} ] },
    { "id": 3, "type": "stat", "title": "在线", "gridPos": {"x":0,"y":8,"w":6,"h":4},
      "targets": [ {"expr": "up"} ] },
    { "id": 10, "type": "row", "title": "分组", "panels": [
      { "id": 4, "type": "gauge", "title": "磁盘", "gridPos": {"x":0,"y":12,"w":6,"h":6},
        "fieldConfig": {"defaults": {"unit": "percentunit", "max": 1}},
        "targets": [ {"expr": "disk_used_ratio"} ] }
    ]},
    { "id": 5, "type": "piechart", "title": "占比", "gridPos": {"x":6,"y":8,"w":6,"h":4},
      "targets": [ {"expr": "sum by (job)(up)"} ] }
  ]
}`

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
	// 面板：row 展平 → 5 个（timeseries/graph/stat/gauge/piechart）
	if len(d.Panels) != 5 {
		t.Fatalf("面板数应为 5（row 展平），实为 %d", len(d.Panels))
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
	if byID[4].Type != "gauge" || byID[4].Unit != "percentunit" || byID[4].Max == nil || *byID[4].Max != 1 {
		t.Fatalf("嵌套 gauge 映射错误: %+v", byID[4])
	}
	if byID[5].Type != "unsupported" || byID[5].RawType != "piechart" {
		t.Fatalf("piechart 应为 unsupported 占位: %+v", byID[5])
	}
	if byID[1].Grid.W != 12 || byID[3].Grid.W != 6 {
		t.Fatalf("gridPos 宽度未保留")
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
