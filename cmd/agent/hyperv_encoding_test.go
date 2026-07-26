package main

import "testing"

func TestSanitizeHyperVText(t *testing.T) {
	if got := sanitizeHyperVText("网络适配器"); got != "网络适配器" {
		t.Fatalf("keep chinese: %q", got)
	}
	if got := sanitizeHyperVText("\ufeff正常"); got != "正常" {
		t.Fatalf("trim bom: %q", got)
	}
	mojibake := "\uFFFD\uFFFD\uFFFD\uFFFD"
	if got := sanitizeHyperVText(mojibake); got != "" {
		t.Fatalf("drop pure mojibake: %q", got)
	}
	if got := sanitizeHyperVText("OK"); got != "OK" {
		t.Fatalf("keep ascii: %q", got)
	}
}

func TestParseHyperVUTF8Chinese(t *testing.T) {
	const in = `[{"Name":"vm1","State":"Running",
	  "IntegrationState":"正常",
	  "Nics":{"Name":"网络适配器","MAC":"00155D010203","Switch":"transfer","Status":"确定","Connected":true,"IP":"192.168.1.1"}}]`
	g, err := parseHyperV(in)
	if err != nil || len(g) != 1 {
		t.Fatalf("parse: %v %#v", err, g)
	}
	if g[0].IntegrationState != "正常" {
		t.Fatalf("integration = %q", g[0].IntegrationState)
	}
	if len(g[0].Nics) != 1 || g[0].Nics[0].Name != "网络适配器" {
		t.Fatalf("nic = %#v", g[0].Nics)
	}
}
