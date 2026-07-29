package main

import (
	"testing"
)

func TestMapGrafanaFieldConfigFull(t *testing.T) {
	raw := `{
	  "title":"fc",
	  "panels":[{
	    "id":1,"type":"timeseries","title":"CPU","gridPos":{"x":0,"y":0,"w":12,"h":8},
	    "fieldConfig":{
	      "defaults":{
	        "unit":"percent","decimals":1,"min":0,"max":100,"noValue":"—",
	        "color":{"mode":"palette-classic"},
	        "thresholds":{"mode":"percentage","steps":[
	          {"color":"green"},
	          {"value":70,"color":"orange"},
	          {"value":90,"color":"red"}
	        ]},
	        "mappings":[
	          {"type":"value","options":{"0":{"text":"空闲","color":"green","index":0}}},
	          {"type":"range","options":{"from":90,"to":100,"result":{"text":"过高","color":"red","index":1}}},
	          {"type":"special","options":{"match":"null","result":{"text":"无数据","color":"text","index":2}}}
	        ],
	        "custom":{
	          "drawStyle":"line","lineInterpolation":"smooth","lineWidth":3,
	          "fillOpacity":25,"gradientMode":"opacity","showPoints":"always",
	          "pointSize":5,"spanNulls":true,"axisPlacement":"left",
	          "stacking":{"mode":"normal"}
	        }
	      },
	      "overrides":[{
	        "matcher":{"id":"byName","options":"idle"},
	        "properties":[
	          {"id":"unit","value":"short"},
	          {"id":"custom.lineWidth","value":1},
	          {"id":"color","value":{"mode":"fixed","fixedColor":"blue"}}
	        ]
	      }]
	    },
	    "targets":[{"expr":"aiops_cpu_percent"}]
	  },{
	    "id":2,"type":"candlestick","title":"K","gridPos":{"x":0,"y":8,"w":12,"h":8},
	    "targets":[{"expr":"aiops_cpu_percent"}]
	  },{
	    "id":3,"type":"nodeGraph","title":"Topo","gridPos":{"x":12,"y":8,"w":12,"h":8},
	    "targets":[{"expr":"up"}]
	  },{
	    "id":4,"type":"clock","title":"Clock","gridPos":{"x":0,"y":16,"w":6,"h":4}
	  }]
	}`
	d, err := mapGrafanaDashboard([]byte(raw), "", "grafana:fc")
	if err != nil {
		t.Fatal(err)
	}
	if err := normalizeDashboard(&d); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	by := map[string]DashPanel{}
	for _, p := range d.Panels {
		by[p.Title] = p
	}
	cpu := by["CPU"]
	if cpu.Options.ThresholdMode != "percentage" {
		t.Fatalf("threshold mode: %q", cpu.Options.ThresholdMode)
	}
	if !cpu.Options.Smooth || !cpu.Options.Stacked || !cpu.Options.ShowPoints || !cpu.Options.SpanNulls {
		t.Fatalf("custom flags: %+v", cpu.Options)
	}
	if cpu.Options.ChartStyle != "area" { // fillOpacity > 0 upgrades line→area
		t.Fatalf("chart_style: %q", cpu.Options.ChartStyle)
	}
	if cpu.Options.LineWidth == nil || *cpu.Options.LineWidth != 3 {
		t.Fatalf("lineWidth: %v", cpu.Options.LineWidth)
	}
	if cpu.Options.NoValue != "—" {
		t.Fatalf("noValue: %q", cpu.Options.NoValue)
	}
	if len(cpu.Options.Mappings) < 3 {
		t.Fatalf("mappings: %+v", cpu.Options.Mappings)
	}
	if len(cpu.Options.Overrides) != 1 || cpu.Options.Overrides[0].MatcherID != "byName" {
		t.Fatalf("overrides: %+v", cpu.Options.Overrides)
	}
	if by["K"].Type != "candlestick" {
		t.Fatalf("candlestick type: %q", by["K"].Type)
	}
	if by["Topo"].Type != "nodegraph" {
		t.Fatalf("nodegraph type: %q", by["Topo"].Type)
	}
	if by["Clock"].Type != "clock" {
		t.Fatalf("clock type: %q", by["Clock"].Type)
	}
}

func TestSanitizeAIDashNewTypesAndOptions(t *testing.T) {
	spec := aiDashSpec{
		Name: "AI",
		Panels: []aiDashPanel{
			{
				Title: "健康六维", Type: "radar", Unit: "percent", W: 8, H: 8,
				Options: DashPanelOptions{
					Palette: "cool",
					Thresholds: []DashThreshold{
						{Value: 0, Color: "var(--ok)"},
						{Value: 80, Color: "var(--warn)"},
					},
				},
				Targets: []aiDashTarget{{Expr: "aiops_cpu_percent", Legend: "{{instance}}"}},
			},
			{Title: "流量走向", Type: "sankey", W: 12, H: 10, Targets: []aiDashTarget{{Expr: "topk(5, aiops_net_sent_rate)"}}},
			{Title: "时钟", Type: "clock", W: 6, H: 4},
			{Title: "未知类型", Type: "magic-chart", Targets: []aiDashTarget{{Expr: "up"}}},
		},
	}
	d, warns := sanitizeAIDash(spec, "", "ai")
	_ = warns
	if err := normalizeDashboard(&d); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	types := map[string]string{}
	for _, p := range d.Panels {
		types[p.Title] = p.Type
	}
	if types["健康六维"] != "radar" {
		t.Fatalf("radar: %q", types["健康六维"])
	}
	if types["流量走向"] != "sankey" {
		t.Fatalf("sankey: %q", types["流量走向"])
	}
	if types["时钟"] != "clock" {
		t.Fatalf("clock: %q", types["时钟"])
	}
	if types["未知类型"] != "timeseries" {
		t.Fatalf("unknown fallback: %q", types["未知类型"])
	}
	var radar DashPanel
	for _, p := range d.Panels {
		if p.Title == "健康六维" {
			radar = p
		}
	}
	if radar.Options.Palette != "cool" {
		t.Fatalf("AI palette not preserved: %+v", radar.Options)
	}
	if len(radar.Options.Thresholds) != 0 {
		t.Fatalf("AI sanitize must strip thresholds by default, got %+v", radar.Options.Thresholds)
	}
	if radar.Grid.H != 8 {
		t.Fatalf("radar height default: %+v", radar.Grid)
	}
	// Width may be stretched by layoutAIDashPanels to fill the 24-col row.
	if w := aiPanelWidth("radar", 8); w != 8 {
		t.Fatalf("aiPanelWidth(radar,8)=%d", w)
	}
	if w := aiPanelWidth("sankey", 0); w != 12 {
		t.Fatalf("aiPanelWidth(sankey default)=%d", w)
	}
	if w := aiPanelWidth("nodegraph", 0); w != 16 {
		t.Fatalf("aiPanelWidth(nodegraph default)=%d", w)
	}
	if h := aiPanelHeight("clock", 0); h != 4 {
		t.Fatalf("aiPanelHeight(clock default)=%d", h)
	}
}

func TestNormalizeUnknownTypeBecomesUnsupported(t *testing.T) {
	d := Dashboard{
		Name: "x",
		Panels: []DashPanel{
			{ID: 1, Title: "weird", Type: "totally-unknown", Grid: DashGrid{W: 12, H: 8}},
		},
	}
	if err := normalizeDashboard(&d); err != nil {
		t.Fatalf("should coerce unknown: %v", err)
	}
	if d.Panels[0].Type != "unsupported" || d.Panels[0].RawType != "totally-unknown" {
		t.Fatalf("got %+v", d.Panels[0])
	}
}
