package main

import (
	"fmt"
	"strings"
)

// User-selectable forecast models (at least three + auto).
// Internal engine names may be more specific (e.g. holt-winters-24); these are the
// stable IDs exposed to the UI.
const (
	fcModelAuto        = "auto"
	fcModelFlat        = "flat"
	fcModelDampedHolt  = "damped-holt"
	fcModelDrift       = "drift"
	fcModelHoltWinters = "holt-winters"
	fcModelSeasonal    = "seasonal"
	fcModelShapeReplay = "shape-replay"
)

type forecastModelInfo struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// forecastModelCatalog is the list shown to operators (plain-language labels).
func forecastModelCatalog() []forecastModelInfo {
	return []forecastModelInfo{
		{ID: fcModelAuto, Label: "智能匹配（推荐）", Description: "系统按当前数据形态自动选最合适的预测方式"},
		{ID: fcModelDampedHolt, Label: "平滑波动", Description: "适合 CPU、延时、连接数等上下抖动的指标"},
		{ID: fcModelDrift, Label: "一路涨/跌", Description: "适合磁盘占用、容量等持续上升或下降的指标"},
		{ID: fcModelHoltWinters, Label: "有规律起伏", Description: "适合流量、访问量等带日/周周期的指标"},
		{ID: fcModelFlat, Label: "基本不变", Description: "适合可用率等几乎稳定在某一水平的指标"},
	}
}

func normalizeForecastMethod(m string) string {
	m = strings.ToLower(strings.TrimSpace(m))
	switch m {
	case "", "auto", "smart", "default":
		return fcModelAuto
	case "flat", "constant", "mean":
		return fcModelFlat
	case "damped-holt", "holt", "holt-linear", "exponential":
		return fcModelDampedHolt
	case "drift", "linear", "trend":
		return fcModelDrift
	case "holt-winters", "hw", "winters":
		return fcModelHoltWinters
	case "seasonal", "seasonal-naive":
		return fcModelSeasonal
	case "shape-replay", "shape", "burst":
		return fcModelShapeReplay
	default:
		// Allow already-qualified engine names through (seasonal-24, holt-winters-12).
		if strings.HasPrefix(m, "seasonal-") || strings.HasPrefix(m, "holt-winters-") {
			return m
		}
		return fcModelAuto
	}
}

func forecastMethodLabel(m string) string {
	m = normalizeForecastMethod(m)
	switch {
	case m == fcModelAuto:
		return "智能匹配（推荐）"
	case m == fcModelFlat:
		return "基本不变"
	case m == fcModelDampedHolt:
		return "平滑波动"
	case m == fcModelDrift:
		return "一路涨/跌"
	case m == fcModelShapeReplay || m == "shape-replay":
		return "突发形态"
	case strings.HasPrefix(m, "holt-winters"):
		return "有规律起伏"
	case strings.HasPrefix(m, "seasonal"):
		return "有规律起伏"
	default:
		return m
	}
}

// suggestForecastMethod picks a default family from series morphology (used when method=auto
// before holdout tournament, and as a UI hint). Holdout may still override in auto mode.
func suggestForecastMethod(prof seriesProfile) string {
	switch {
	case prof.monotonicUp:
		return fcModelDrift
	case prof.bursty && !prof.jittery:
		return fcModelShapeReplay
	case prof.seasonStr >= 0.35 && prof.period >= 4:
		return fcModelHoltWinters
	case prof.jittery || prof.trendStr >= 0.35:
		return fcModelDampedHolt
	case prof.stationary:
		return fcModelFlat
	default:
		return fcModelDampedHolt
	}
}

// forceCandidateNames restricts the tournament to the user-selected model family.
func forceCandidateNames(prefer string, period, shapeP, n int, prof seriesProfile) []string {
	prefer = normalizeForecastMethod(prefer)
	if prefer == fcModelAuto {
		return nil
	}
	periodOr := func(fallback int) int {
		p := period
		if p < 4 {
			p = shapeP
		}
		if p < 4 {
			p = fallback
		}
		if p < 4 {
			p = 8
		}
		if p > n/2 && n >= 8 {
			p = n / 2
		}
		return p
	}
	switch {
	case prefer == fcModelFlat:
		return []string{fcModelFlat}
	case prefer == fcModelDampedHolt:
		return []string{fcModelDampedHolt}
	case prefer == fcModelDrift:
		return []string{fcModelDrift}
	case prefer == fcModelShapeReplay || prefer == "shape-replay":
		return []string{"shape-replay"}
	case prefer == fcModelHoltWinters || strings.HasPrefix(prefer, "holt-winters"):
		p := periodOr(fcMin(24, fcMax(6, n/3)))
		return []string{fmt.Sprintf("holt-winters-%d", p)}
	case prefer == fcModelSeasonal || strings.HasPrefix(prefer, "seasonal"):
		p := periodOr(fcMin(24, fcMax(6, n/3)))
		return []string{fmt.Sprintf("seasonal-%d", p)}
	default:
		return nil
	}
}
