package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestForecastModelCatalogHasAtLeastThreePlusAuto(t *testing.T) {
	cats := forecastModelCatalog()
	if len(cats) < 4 {
		t.Fatalf("want >=4 models (auto+3), got %d", len(cats))
	}
	ids := map[string]bool{}
	for _, c := range cats {
		ids[c.ID] = true
		if c.Label == "" || c.ID == "" {
			t.Fatalf("empty model entry: %+v", c)
		}
		blob := strings.ToLower(c.ID + " " + c.Label + " " + c.Description)
		if strings.Contains(blob, "hermes") {
			t.Fatalf("user-facing catalog must not mention hermes: %+v", c)
		}
	}
	for _, need := range []string{fcModelAuto, fcModelDampedHolt, fcModelDrift, fcModelHoltWinters} {
		if !ids[need] {
			t.Fatalf("missing model %s", need)
		}
	}
}

func TestNormalizeAndForceForecastMethod(t *testing.T) {
	if normalizeForecastMethod("") != fcModelAuto {
		t.Fatal("empty -> auto")
	}
	if normalizeForecastMethod("linear") != fcModelDrift {
		t.Fatal("linear -> drift")
	}
	forced := forceCandidateNames(fcModelHoltWinters, 0, 12, 48, seriesProfile{})
	if len(forced) != 1 || forced[0] != "holt-winters-12" {
		t.Fatalf("holt-winters force got %v", forced)
	}
	forced2 := forceCandidateNames(fcModelFlat, 0, 0, 20, seriesProfile{})
	if len(forced2) != 1 || forced2[0] != fcModelFlat {
		t.Fatalf("flat force got %v", forced2)
	}
}

func TestSuggestForecastMethodByProfile(t *testing.T) {
	if suggestForecastMethod(seriesProfile{monotonicUp: true}) != fcModelDrift {
		t.Fatal("monotonicUp -> drift")
	}
	if suggestForecastMethod(seriesProfile{stationary: true}) != fcModelFlat {
		t.Fatal("stationary -> flat")
	}
	if suggestForecastMethod(seriesProfile{jittery: true, trendStr: 0.5}) != fcModelDampedHolt {
		t.Fatal("jittery trend -> damped-holt")
	}
}

func TestAIConfigJSONNoHermesKeys(t *testing.T) {
	c := AIConfig{SreyunEnabled: true, SreyunAutoApprove: true, SreyunTerminalEnabled: true}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(strings.ToLower(s), "hermes") {
		t.Fatalf("marshaled AIConfig must not contain hermes: %s", s)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"ai_agent_enabled", "ai_auto_approve", "ai_terminal_enabled"} {
		if _, ok := raw[k]; !ok {
			t.Fatalf("missing key %s in %s", k, s)
		}
	}
	var c2 AIConfig
	if err := json.Unmarshal([]byte(`{"hermes_enabled":true,"hermes_auto_approve":true,"hermes_terminal_enabled":true}`), &c2); err != nil {
		t.Fatal(err)
	}
	if !c2.SreyunEnabled || !c2.SreyunAutoApprove || !c2.SreyunTerminalEnabled {
		t.Fatalf("legacy hermes_* unmarshal failed: %+v", c2)
	}
}

func TestLogEntryUsernameJSON(t *testing.T) {
	e := LogEntry{Kind: KindOperation, Level: "info", Actor: "admin", Username: "admin", IP: "1.2.3.4", Message: "x"}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["username"] != "admin" {
		t.Fatalf("username field missing: %s", b)
	}
}
