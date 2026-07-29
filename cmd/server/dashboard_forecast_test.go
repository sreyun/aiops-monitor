package main

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestHoltLinearForecastBasic(t *testing.T) {
	now := time.Now().Unix()
	step := int64(60)
	hist := make([][2]float64, 0, 40)
	for i := 0; i < 40; i++ {
		hist = append(hist, [2]float64{float64(now - int64(40-i)*step), 10 + float64(i)*0.5})
	}
	band, mape, r2, method, errMsg := holtLinearForecast(hist, now, 10*step, step)
	if errMsg != "" {
		t.Fatalf("unexpected err: %s", errMsg)
	}
	if method == "" {
		t.Fatalf("empty method")
	}
	if len(band) < 8 {
		t.Fatalf("band len=%d", len(band))
	}
	if band[0].Hi < band[0].Value || band[0].Lo > band[0].Value {
		t.Fatalf("band invalid: %+v", band[0])
	}
	// Rising series → last forecast should stay above early history (damped, not explode)
	if band[len(band)-1].Value <= hist[0][1] {
		t.Fatalf("forecast not rising: last=%v firstHist=%v method=%s", band[len(band)-1].Value, hist[0][1], method)
	}
	if mape < 0 || mape > 500 {
		t.Fatalf("mape out of range: %v", mape)
	}
	if r2 < 0 || r2 > 1.0001 {
		t.Fatalf("r2 out of range: %v", r2)
	}
}

func TestRobustForecastPercentDoesNotExceed100(t *testing.T) {
	now := time.Now().Unix()
	step := int64(15)
	hist := make([][2]float64, 0, 60)
	for i := 0; i < 60; i++ {
		// Rising CPU% that used to overshoot past 100 via Holt
		v := 40 + float64(i)*0.8 + math.Sin(float64(i)/3)*5
		hist = append(hist, [2]float64{float64(now - int64(60-i)*step), v})
	}
	band, _, _, method, errMsg := robustForecast(hist, now, 60*step, step)
	if errMsg != "" {
		t.Fatalf("err: %s", errMsg)
	}
	for _, p := range band {
		if p.Value > 100.5 {
			t.Fatalf("percent forecast >100: v=%v method=%s", p.Value, method)
		}
		if p.Hi > 105 {
			t.Fatalf("percent band hi too high: hi=%v", p.Hi)
		}
	}
}

func TestRobustForecastDoesNotExplodeOnSpike(t *testing.T) {
	now := time.Now().Unix()
	step := int64(15)
	hist := make([][2]float64, 0, 80)
	for i := 0; i < 80; i++ {
		v := 100.0 + math.Sin(float64(i)/5)*10
		if i == 70 {
			v = 800 // single spike that used to derail linear Holt
		}
		hist = append(hist, [2]float64{float64(now - int64(80-i)*step), v})
	}
	band, _, _, method, errMsg := robustForecast(hist, now, 40*step, step)
	if errMsg != "" {
		t.Fatalf("err: %s", errMsg)
	}
	last := band[len(band)-1].Value
	if last > 400 {
		t.Fatalf("forecast exploded after spike: last=%v method=%s", last, method)
	}
	if last < 0 {
		t.Fatalf("negative forecast on non-neg series: %v", last)
	}
}

func TestRobustForecastFlatNoisy(t *testing.T) {
	now := time.Now().Unix()
	step := int64(15)
	hist := make([][2]float64, 0, 60)
	for i := 0; i < 60; i++ {
		// Zero-mean noise around 50 — should prefer flat-ish forecast
		v := 50 + float64((i*17)%7) - 3
		hist = append(hist, [2]float64{float64(now - int64(60-i)*step), v})
	}
	band, _, _, _, errMsg := robustForecast(hist, now, 20*step, step)
	if errMsg != "" {
		t.Fatalf("err: %s", errMsg)
	}
	avg := 0.0
	for _, p := range band {
		avg += p.Value
	}
	avg /= float64(len(band))
	if avg < 30 || avg > 70 {
		t.Fatalf("noisy series forecast drifted too far: avg=%v", avg)
	}
}

func TestHoltLinearForecastInsufficient(t *testing.T) {
	hist := [][2]float64{{1, 1}, {2, 2}, {3, 3}}
	_, _, _, _, errMsg := holtLinearForecast(hist, 10, 60, 15)
	if errMsg == "" {
		t.Fatal("expected insufficient data message")
	}
	if !strings.Contains(errMsg, "数据不足") {
		t.Fatalf("msg=%q", errMsg)
	}
}

func TestForecastFutureStartsAfterNow(t *testing.T) {
	now := time.Now().Unix()
	step := int64(60)
	hist := make([][2]float64, 0, 30)
	for i := 0; i < 30; i++ {
		hist = append(hist, [2]float64{float64(now - int64(30-i)*step), 20 + float64(i)})
	}
	band, _, _, _, errMsg := robustForecast(hist, now, 10*step, step)
	if errMsg != "" {
		t.Fatal(errMsg)
	}
	if band[0].TS <= float64(now) {
		t.Fatalf("forecast must start after now: first=%v now=%v", band[0].TS, now)
	}
}

func TestShiftPoints(t *testing.T) {
	pts := [][2]float64{{100, 1}, {200, 2}}
	out := shiftPoints(pts, 50)
	if out[0][0] != 150 || out[1][0] != 250 {
		t.Fatalf("shift failed: %+v", out)
	}
	if out[0][1] != 1 {
		t.Fatalf("value mutated: %+v", out)
	}
}

func TestComputePoPChange(t *testing.T) {
	cur := [][2]float64{{1, 120}, {2, 140}}
	prev := [][2]float64{{1, 100}, {2, 100}}
	pct, ok := computePoPChange(cur, prev)
	if !ok {
		t.Fatal("expected ok")
	}
	if math.Abs(pct-30) > 0.01 {
		t.Fatalf("pct=%v want 30", pct)
	}
}

func TestForecastCrossThreshold(t *testing.T) {
	now := time.Now().Unix()
	step := int64(60)
	hist := make([][2]float64, 0, 30)
	for i := 0; i < 30; i++ {
		hist = append(hist, [2]float64{float64(now - int64(30-i)*step), float64(i) * 2})
	}
	cross, ok := forecastCrossThreshold(hist, 65, step)
	if !ok || cross <= now {
		t.Fatalf("cross=%d ok=%v lastHist=%.0f", cross, ok, hist[len(hist)-1][1])
	}
}

func TestRobustForecastSeasonalNotFlat(t *testing.T) {
	now := time.Now().Unix()
	step := int64(60)
	hist := make([][2]float64, 0, 96)
	for i := 0; i < 96; i++ {
		// 清晰周期 + 缓升，不应再被压成水平线
		v := 50 + 18*math.Sin(2*math.Pi*float64(i)/12) + float64(i)*0.05
		hist = append(hist, [2]float64{float64(now - int64(96-i)*step), v})
	}
	band, _, _, method, errMsg := robustForecast(hist, now, 24*step, step)
	if errMsg != "" {
		t.Fatal(errMsg)
	}
	if method == "flat" {
		t.Fatalf("季节序列不应选 flat，method=%s", method)
	}
	mn, mx := band[0].Value, band[0].Value
	for _, p := range band {
		if p.Value < mn {
			mn = p.Value
		}
		if p.Value > mx {
			mx = p.Value
		}
	}
	if mx-mn < 6 {
		t.Fatalf("季节预测波动过小（近似直线）: range=%v method=%s", mx-mn, method)
	}
}

func TestRobustForecastBurstyHasVariation(t *testing.T) {
	now := time.Now().Unix()
	step := int64(15)
	hist := make([][2]float64, 0, 80)
	for i := 0; i < 80; i++ {
		v := 5.0
		if i%10 == 0 {
			v = 120 + float64(i%3)*20 // 周期性尖刺，模拟磁盘 IO
		} else if i%10 == 1 {
			v = 40
		}
		hist = append(hist, [2]float64{float64(now - int64(80-i)*step), v})
	}
	band, _, _, method, errMsg := robustForecast(hist, now, 30*step, step)
	if errMsg != "" {
		t.Fatal(errMsg)
	}
	if method == "flat" {
		t.Fatalf("突发序列不应选 flat，method=%s", method)
	}
	var mean, ss float64
	mn, mx := band[0].Value, band[0].Value
	for _, p := range band {
		mean += p.Value
		if p.Value < mn {
			mn = p.Value
		}
		if p.Value > mx {
			mx = p.Value
		}
	}
	mean /= float64(len(band))
	for _, p := range band {
		d := p.Value - mean
		ss += d * d
	}
	std := math.Sqrt(ss / float64(len(band)))
	// 平滑后振幅会低于原始尖刺，但仍应保留可见起伏（不能是水平线）
	if std < 4 || (mx-mn) < 12 {
		t.Fatalf("突发序列预测波动过小: std=%v range=%v method=%s", std, mx-mn, method)
	}
}

func TestRobustForecastJitteryNotSawtooth(t *testing.T) {
	now := time.Now().Unix()
	step := int64(15)
	hist := make([][2]float64, 0, 90)
	for i := 0; i < 90; i++ {
		// 模拟连接数：慢变水位 + 强高频采样噪声（旧 shape-replay 会复刻成锯齿带）
		v := 550 + 80*math.Sin(float64(i)/18) + float64((i*37)%11-5)*18
		hist = append(hist, [2]float64{float64(now - int64(90-i)*step), v})
	}
	band, _, _, method, errMsg := robustForecast(hist, now, 36*step, step)
	if errMsg != "" {
		t.Fatal(errMsg)
	}
	if method == "shape-replay" {
		t.Fatalf("高频抖动序列不应使用 shape-replay，method=%s", method)
	}
	pred := make([]float64, len(band))
	for i, p := range band {
		pred[i] = p.Value
	}
	if forecastJitterPenalty(pred) > 0.35 {
		t.Fatalf("预测仍呈锯齿: penalty=%.2f method=%s", forecastJitterPenalty(pred), method)
	}
	// 不应出现假趋势顶满后长时间水平封顶
	topFlat := 0
	for i := 1; i < len(band); i++ {
		if math.Abs(band[i].Value-band[i-1].Value) < 1e-6 && band[i].Value > 900 {
			topFlat++
		}
	}
	if topFlat > len(band)/3 {
		t.Fatalf("预测疑似触顶平台: topFlat=%d/%d method=%s", topFlat, len(band), method)
	}
}

func TestRobustForecastShortSpanAllowed(t *testing.T) {
	now := time.Now().Unix()
	step := int64(30)
	hist := make([][2]float64, 0, 10)
	for i := 0; i < 10; i++ {
		hist = append(hist, [2]float64{float64(now - int64(10-i)*step), 40 + float64(i)})
	}
	band, _, _, method, errMsg := robustForecast(hist, now, 1800, step)
	if errMsg != "" {
		t.Fatalf("short span should forecast: %s", errMsg)
	}
	if len(band) < 2 {
		t.Fatalf("expected future points, method=%s", method)
	}
	if band[len(band)-1].TS <= float64(now) {
		t.Fatalf("forecast should extend past now")
	}
}

func TestRobustForecastMonotonicDiskPrefersDrift(t *testing.T) {
	now := time.Now().Unix()
	step := int64(300)
	hist := make([][2]float64, 0, 48)
	for i := 0; i < 48; i++ {
		// 磁盘占用缓慢单调上升（带极小噪声）
		v := 62.0 + float64(i)*0.35 + float64((i%5)-2)*0.02
		hist = append(hist, [2]float64{float64(now - int64(48-i)*step), v})
	}
	vals := make([]float64, len(hist))
	for i, p := range hist {
		vals[i] = p[1]
	}
	prof := profileSeries(vals)
	if !prof.monotonicUp {
		t.Fatalf("expected monotonicUp for disk-like series, got %+v", prof)
	}
	band, _, _, method, errMsg := robustForecast(hist, now, 24*step, step)
	if errMsg != "" {
		t.Fatal(errMsg)
	}
	if method == "flat" {
		t.Fatalf("单调磁盘序列不应选 flat，method=%s", method)
	}
	if len(band) < 4 {
		t.Fatal("empty band")
	}
	lastHist := hist[len(hist)-1][1]
	lastFC := band[len(band)-1].Value
	if lastFC <= lastHist+0.5 {
		t.Fatalf("存储外推应继续上升: hist=%.2f fc=%.2f method=%s", lastHist, lastFC, method)
	}
}
