package main

import (
	"testing"
)

func TestNormalizeDashPanelOptions(t *testing.T) {
	dec := 12
	o := DashPanelOptions{
		Sort:       "DESC",
		Limit:      500,
		Decimals:   &dec,
		Palette:    "classic",
		Legend:     "bottom",
		ChartStyle: "area",
		Colors:     []string{"#4c8dff", "not-a-color", "#22c55e", "var(--ok)"},
		Thresholds: []DashThreshold{
			{Value: 90, Color: "var(--crit)"},
			{Value: 0, Color: "#22c55e"},
			{Value: 75, Color: "red;evil"},
			{Value: 50, Color: "var(--warn)"},
		},
	}
	if err := normalizeDashPanelOptions(&o, "cpu"); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if o.Sort != "desc" {
		t.Fatalf("sort: %q", o.Sort)
	}
	if o.Limit != 200 {
		t.Fatalf("limit capped: %d", o.Limit)
	}
	if o.Decimals == nil || *o.Decimals != 10 {
		t.Fatalf("decimals capped: %v", o.Decimals)
	}
	// Colors accept HEX only; CSS tokens are for thresholds.
	if len(o.Colors) != 2 {
		t.Fatalf("colors cleaned: %v", o.Colors)
	}
	if len(o.Thresholds) != 3 {
		t.Fatalf("thresholds cleaned: %+v", o.Thresholds)
	}
	if o.Thresholds[0].Value != 0 || o.Thresholds[1].Value != 50 || o.Thresholds[2].Value != 90 {
		t.Fatalf("thresholds not sorted: %+v", o.Thresholds)
	}
}

func TestNormalizeDashPanelOptionsRejectsBadEnum(t *testing.T) {
	o := DashPanelOptions{Sort: "random"}
	if err := normalizeDashPanelOptions(&o, "x"); err == nil {
		t.Fatal("bad sort should fail")
	}
	o = DashPanelOptions{Palette: "neon"}
	if err := normalizeDashPanelOptions(&o, "x"); err == nil {
		t.Fatal("bad palette should fail")
	}
}

func TestNormalizeDashPanelOptionsLegendTop(t *testing.T) {
	o := DashPanelOptions{Legend: "TOP"}
	if err := normalizeDashPanelOptions(&o, "cpu"); err != nil {
		t.Fatalf("top legend should be accepted: %v", err)
	}
	if o.Legend != "top" {
		t.Fatalf("legend normalized: %q", o.Legend)
	}
	o = DashPanelOptions{Legend: "middle"}
	if err := normalizeDashPanelOptions(&o, "cpu"); err == nil {
		t.Fatal("invalid legend should fail")
	}
}

func TestMapGrafanaLegendPlacementTop(t *testing.T) {
	raw := `{
	  "title":"leg",
	  "panels":[{
	    "id":1,"type":"timeseries","title":"CPU","gridPos":{"x":0,"y":0,"w":12,"h":8},
	    "options":{"legend":{"displayMode":"list","placement":"top","showLegend":true}},
	    "targets":[{"expr":"up","legendFormat":"{{instance}}"}]
	  }]
	}`
	d, err := mapGrafanaDashboard([]byte(raw), "", "grafana:leg")
	if err != nil {
		t.Fatal(err)
	}
	if d.Panels[0].Options.Legend != "top" {
		t.Fatalf("grafana top placement → %q", d.Panels[0].Options.Legend)
	}
}

func TestMapGrafanaPanelOptionsThresholds(t *testing.T) {
	raw := `{
	  "title":"th",
	  "panels":[{
	    "id":1,"type":"gauge","title":"Disk","gridPos":{"x":0,"y":0,"w":6,"h":6},
	    "fieldConfig":{"defaults":{
	      "unit":"percent","decimals":2,
	      "color":{"mode":"thresholds"},
	      "thresholds":{"mode":"absolute","steps":[
	        {"color":"green"},
	        {"value":75,"color":"orange"},
	        {"value":90,"color":"red"}
	      ]}
	    }},
	    "options":{"legend":{"showLegend":false}},
	    "targets":[{"expr":"disk_used"}]
	  }]
	}`
	d, err := mapGrafanaDashboard([]byte(raw), "", "grafana:th")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Panels) != 1 {
		t.Fatalf("panels: %d", len(d.Panels))
	}
	p := d.Panels[0]
	if p.Options.Palette != "traffic" {
		t.Fatalf("palette: %q", p.Options.Palette)
	}
	if p.Options.Decimals == nil || *p.Options.Decimals != 2 {
		t.Fatalf("decimals: %v", p.Options.Decimals)
	}
	if len(p.Options.Thresholds) != 3 {
		t.Fatalf("thresholds (null base → 0): %+v", p.Options.Thresholds)
	}
	if p.Options.Thresholds[0].Value != 0 || p.Options.Thresholds[0].Color != "var(--ok)" {
		t.Fatalf("step0: %+v", p.Options.Thresholds[0])
	}
	if p.Options.Thresholds[1].Value != 75 || p.Options.Thresholds[1].Color != "var(--warn)" {
		t.Fatalf("step1: %+v", p.Options.Thresholds[1])
	}
	if p.Options.Thresholds[2].Value != 90 || p.Options.Thresholds[2].Color != "var(--crit)" {
		t.Fatalf("step2: %+v", p.Options.Thresholds[2])
	}
	if p.Options.Legend != "hidden" {
		t.Fatalf("legend: %q", p.Options.Legend)
	}
	if err := normalizeDashboard(&d); err != nil {
		t.Fatalf("normalize imported: %v", err)
	}
}

func TestDashChartsAssetPresent(t *testing.T) {
	// Ensure vendored ECharts + adapter are on disk for /js and /app.js bundling.
	for _, rel := range []string{
		"web/js/vendor/echarts.min.js",
		"web/js/dash_charts.js",
	} {
		b, err := webFS.ReadFile(rel)
		if err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
		if len(b) < 100 {
			t.Fatalf("%s too small", rel)
		}
	}
}
