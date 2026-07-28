package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

const metricsForecastMaxSeries = 12

// metricsForecastReq accepts already-aligned sample series (host/AI/SNMP/…).
// Unlike /dashboards/query-forecast it does not require PromQL.
type metricsForecastReq struct {
	Series     []metricsForecastIn `json:"series"`
	HorizonSec int64               `json:"horizon_sec"` // 0 = equal to history span
	Step       int64               `json:"step"`        // 0 = auto from median delta
}

type metricsForecastIn struct {
	Name   string       `json:"name"`
	Points [][2]float64 `json:"points"` // [[tsSec, val], ...]
}

// handleMetricsForecast POST /api/v1/metrics/forecast
// Multi-series forecast for any Canvas/combo chart (CPU+mem+net, token cost, …).
func (s *Server) handleMetricsForecast(w http.ResponseWriter, r *http.Request) {
	var req metricsForecastReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if len(req.Series) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 series"})
		return
	}
	if len(req.Series) > metricsForecastMaxSeries {
		req.Series = req.Series[:metricsForecastMaxSeries]
	}

	out := make([]forecastSeriesOut, 0, len(req.Series)*2)
	meta := forecastMeta{Mode: "forecast", OK: false}
	var bestMAPE float64 = 1e9
	bestMethod := ""
	var nowTS int64
	anyOK := false

	for i, in := range req.Series {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = fmt.Sprintf("系列%d", i+1)
		}
		pts := in.Points
		if len(pts) < 8 {
			continue
		}
		// History passthrough (caller may already have history drawn; still return for completeness)
		out = append(out, forecastSeriesOut{
			Name: name, Kind: "history", Points: pts,
		})
		step := req.Step
		if step <= 0 {
			step = inferStep(pts)
		}
		span := int64(pts[len(pts)-1][0] - pts[0][0])
		if span < step*8 {
			continue
		}
		horizon := req.HorizonSec
		if horizon <= 0 {
			horizon = span
		}
		if horizon > span*4 {
			horizon = span * 4
		}
		if horizon > 90*24*3600 {
			horizon = 90 * 24 * 3600
		}
		fromTS := int64(pts[len(pts)-1][0])
		if fromTS > nowTS {
			nowTS = fromTS
		}
		learnKey := forecastMetricKey("metrics", name)
		fc, mape, r2, method, errMsg := robustForecastWithKey(pts, fromTS, horizon, step, learnKey)
		if errMsg != "" || len(fc) == 0 {
			continue
		}
		last := pts[len(pts)-1]
		fc, method, _ = s.finalizeForecastWithLearning(learnKey, method, "", fc, fromTS, horizon, step, last[1])
		anyOK = true
		if mape < bestMAPE {
			bestMAPE, bestMethod = mape, method
			meta.R2 = r2
		}
		fcPts := make([][2]float64, 0, len(fc)+1)
		band := make([]forecastPoint, 0, len(fc)+1)
		// Bridge last history point
		fcPts = append(fcPts, last)
		band = append(band, forecastPoint{TS: last[0], Value: last[1], Lo: last[1], Hi: last[1]})
		for _, p := range fc {
			fcPts = append(fcPts, [2]float64{p.TS, p.Value})
			band = append(band, p)
		}
		out = append(out, forecastSeriesOut{
			Name: name + " · 预测", Kind: "forecast", Points: fcPts, Band: band,
		})
		meta.HorizonSec = horizon
		meta.Step = step
	}

	meta.NowTS = nowTS
	if anyOK {
		meta.OK = true
		meta.Method = bestMethod
		meta.MAPE = bestMAPE
		meta.Message = fmt.Sprintf("左=实时 · 右=预测（%s，MAPE≈%.1f%%，%d 序列）",
			bestMethod, bestMAPE, countForecastKinds(out))
	} else {
		meta.Message = "数据不足，暂无法预测（每条序列至少约 8 个采样点）"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     meta.OK,
		"series": out,
		"meta":   meta,
	})
}

func countForecastKinds(list []forecastSeriesOut) int {
	n := 0
	for _, s := range list {
		if s.Kind == "forecast" {
			n++
		}
	}
	return n
}

func inferStep(pts [][2]float64) int64 {
	if len(pts) < 2 {
		return 15
	}
	// Median positive delta
	deltas := make([]float64, 0, len(pts)-1)
	for i := 1; i < len(pts); i++ {
		d := pts[i][0] - pts[i-1][0]
		if d > 0 {
			deltas = append(deltas, d)
		}
	}
	if len(deltas) == 0 {
		return 15
	}
	sort.Float64s(deltas)
	med := int64(deltas[len(deltas)/2] + 0.5)
	if med < 1 {
		med = 15
	}
	if med > 3600 {
		med = 3600
	}
	return med
}
