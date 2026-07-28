package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleMetricsForecastMultiSeries(t *testing.T) {
	srv, _ := newTestServer(t)
	now := time.Now().Unix()
	step := int64(60)
	mk := func(base float64, drift float64) [][2]float64 {
		pts := make([][2]float64, 0, 40)
		for i := 0; i < 40; i++ {
			pts = append(pts, [2]float64{
				float64(now - int64(40-i)*step),
				base + float64(i)*drift,
			})
		}
		return pts
	}
	body := map[string]any{
		"series": []map[string]any{
			{"name": "cpu_percent", "points": mk(20, 0.4)},
			{"name": "mem_percent", "points": mk(40, 0.2)},
			{"name": "net_recv", "points": mk(1e6, 1e3)},
		},
		"horizon_sec": 20 * step,
		"step":        step,
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/forecast", bytes.NewReader(raw))
	rr := httptest.NewRecorder()
	srv.handleMetricsForecast(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var res struct {
		OK     bool `json:"ok"`
		Series []struct {
			Name   string       `json:"name"`
			Kind   string       `json:"kind"`
			Points [][2]float64 `json:"points"`
		} `json:"series"`
		Meta forecastMeta `json:"meta"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected ok meta=%+v", res.Meta)
	}
	var histN, fcN int
	for _, s := range res.Series {
		switch s.Kind {
		case "history":
			histN++
		case "forecast":
			fcN++
			if len(s.Points) < 2 {
				t.Fatalf("forecast %q too short", s.Name)
			}
			// Future points should extend past now
			lastTS := int64(s.Points[len(s.Points)-1][0])
			if lastTS <= now {
				t.Fatalf("forecast %q lastTS=%d not after now=%d", s.Name, lastTS, now)
			}
		}
	}
	if histN < 3 || fcN < 3 {
		t.Fatalf("want ≥3 history+forecast pairs, got hist=%d fc=%d total=%d", histN, fcN, len(res.Series))
	}
	if res.Meta.NowTS <= 0 {
		t.Fatalf("missing now_ts in meta: %+v", res.Meta)
	}
}

func TestHandleMetricsForecastCapsSeries(t *testing.T) {
	srv, _ := newTestServer(t)
	now := time.Now().Unix()
	step := int64(30)
	series := make([]map[string]any, 0, metricsForecastMaxSeries+4)
	for n := 0; n < metricsForecastMaxSeries+4; n++ {
		pts := make([][2]float64, 0, 20)
		for i := 0; i < 20; i++ {
			pts = append(pts, [2]float64{float64(now - int64(20-i)*step), float64(10 + n + i)})
		}
		series = append(series, map[string]any{"name": fmt.Sprintf("s%d", n), "points": pts})
	}
	raw, _ := json.Marshal(map[string]any{"series": series, "step": step})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/forecast", bytes.NewReader(raw))
	rr := httptest.NewRecorder()
	srv.handleMetricsForecast(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var res struct {
		Series []struct {
			Kind string `json:"kind"`
		} `json:"series"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &res)
	hist := 0
	for _, s := range res.Series {
		if s.Kind == "history" {
			hist++
		}
	}
	if hist > metricsForecastMaxSeries {
		t.Fatalf("history series not capped: %d > %d", hist, metricsForecastMaxSeries)
	}
}

func TestHandleMetricsForecastRejectsEmpty(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/forecast", bytes.NewReader([]byte(`{"series":[]}`)))
	rr := httptest.NewRecorder()
	srv.handleMetricsForecast(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

func TestInferStepMedian(t *testing.T) {
	pts := [][2]float64{{0, 1}, {15, 2}, {30, 3}, {45, 4}, {60, 5}}
	if s := inferStep(pts); s != 15 {
		t.Fatalf("inferStep=%d want 15", s)
	}
}
