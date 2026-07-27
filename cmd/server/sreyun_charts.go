package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"aiops-monitor/shared"
)

// registerChartTools wires in-chat chart / stat / drill-down tools for Hermes.
func (h *SreyunCore) registerChartTools() {
	h.tools["render_chart"] = SreyunTool{
		Name: "render_chart",
		Description: "在 AI 对话中生成并展示趋势图表（前端内嵌 Canvas）。" +
			"用户要看 CPU/内存/磁盘/负载/网络 趋势、对比曲线时优先调用。" +
			"source=host 时用主机历史指标；source=promql 时用 PromQL 区间查询。" +
			"不要在回复里粘贴大段原始采样 JSON，图表由 UI 动作展示。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source":     map[string]string{"type": "string", "description": "host | promql，默认 host"},
				"host_id":    map[string]string{"type": "string", "description": "主机 ID / 主机名 / IP（source=host）"},
				"metrics":    map[string]string{"type": "string", "description": "逗号分隔：cpu,memory,disk,load,network,io；默认 cpu"},
				"expr":       map[string]string{"type": "string", "description": "PromQL（source=promql）"},
				"datasource": map[string]string{"type": "string", "description": "数据源 ID，留空=内置 VM"},
				"range":      map[string]string{"type": "string", "description": "时间范围，如 1h/6h/24h/7d，默认 6h"},
				"title":      map[string]string{"type": "string", "description": "图表标题"},
			},
		},
		Execute: h.execRenderChart,
	}
	h.tools["query_metric_range"] = SreyunTool{
		Name:        "query_metric_range",
		Description: "查询主机指定指标的时间序列摘要（点数/最小/最大/平均/最新），并在对话中渲染趋势图。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "主机 ID / 主机名 / IP"},
				"metric":  map[string]string{"type": "string", "description": "cpu|memory|disk|load|network|io，默认 cpu"},
				"range":   map[string]string{"type": "string", "description": "1h/6h/24h/7d，默认 6h"},
			},
			"required": []string{"host_id"},
		},
		Execute: h.execQueryMetricRange,
	}
	h.tools["query_promql_range"] = SreyunTool{
		Name:        "query_promql_range",
		Description: "执行 PromQL 区间查询并在对话中渲染图表。适合自定义指标、多序列对比、跨主机聚合。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expr":       map[string]string{"type": "string", "description": "PromQL 表达式"},
				"datasource": map[string]string{"type": "string", "description": "数据源 ID，留空=内置 VM"},
				"range":      map[string]string{"type": "string", "description": "1h/6h/24h/7d，默认 6h"},
				"title":      map[string]string{"type": "string", "description": "图表标题"},
			},
			"required": []string{"expr"},
		},
		Execute: h.execQueryPromqlRange,
	}
	h.tools["show_instant_stat"] = SreyunTool{
		Name:        "show_instant_stat",
		Description: "在对话中展示大数字指标卡（可选迷你趋势）。适合「当前 CPU 多少」「内存占用」等瞬时值展示。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "主机 ID / 主机名 / IP"},
				"metric":  map[string]string{"type": "string", "description": "cpu|memory|disk|load，默认 cpu"},
				"range":   map[string]string{"type": "string", "description": "用于 sparkline 的范围，默认 1h"},
			},
			"required": []string{"host_id"},
		},
		Execute: h.execShowInstantStat,
	}
	h.tools["analyze_metric_trend"] = SreyunTool{
		Name: "analyze_metric_trend",
		Description: "分析主机近期指标趋势（早/晚窗口对比）并生成趋势图 + 下钻入口。" +
			"用户说「分析趋势」「有没有异常波动」「过去几小时资源变化」时优先调用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "主机 ID / 主机名 / IP"},
				"hours":   map[string]any{"type": "number", "description": "回溯小时数，默认 6，最大 72"},
				"metrics": map[string]string{"type": "string", "description": "逗号分隔指标，默认 cpu,memory,disk,load"},
			},
			"required": []string{"host_id"},
		},
		Execute: h.execAnalyzeMetricTrend,
	}
}

var chatChartColors = []string{
	"#4c8dff", "#22c55e", "#f59e0b", "#ef4d5a", "#a855f7",
	"#06b6d4", "#eab308", "#ec4899", "#14b8a6", "#f97316",
}

func parseChartRange(raw string, defHours int) (from, to int64, label string) {
	to = time.Now().Unix()
	if defHours <= 0 {
		defHours = 6
	}
	hours := defHours
	s := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case s == "" || s == "6h":
		hours = 6
		label = "6h"
	case strings.HasSuffix(s, "h"):
		var n int
		if _, err := fmt.Sscanf(s, "%dh", &n); err == nil && n > 0 {
			hours = n
			label = fmt.Sprintf("%dh", n)
		}
	case strings.HasSuffix(s, "d"):
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err == nil && n > 0 {
			hours = n * 24
			label = fmt.Sprintf("%dd", n)
		}
	case strings.HasSuffix(s, "m"):
		var n int
		if _, err := fmt.Sscanf(s, "%dm", &n); err == nil && n > 0 {
			from = to - int64(n)*60
			label = fmt.Sprintf("%dm", n)
			return from, to, label
		}
	default:
		var n float64
		if _, err := fmt.Sscanf(s, "%f", &n); err == nil && n > 0 {
			hours = int(n)
			label = fmt.Sprintf("%dh", hours)
		}
	}
	if hours < 1 {
		hours = 1
	}
	if hours > 168 {
		hours = 168
	}
	if label == "" {
		label = fmt.Sprintf("%dh", hours)
	}
	from = to - int64(hours)*3600
	return from, to, label
}

func normalizeMetricKeys(raw string, def string) []string {
	if strings.TrimSpace(raw) == "" {
		raw = def
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		k := strings.ToLower(strings.TrimSpace(p))
		switch k {
		case "mem", "memory", "ram":
			k = "memory"
		case "cpu", "disk", "load", "network", "net", "io":
			if k == "net" {
				k = "network"
			}
		case "all":
			for _, x := range []string{"cpu", "memory", "disk", "load"} {
				if !seen[x] {
					seen[x] = true
					out = append(out, x)
				}
			}
			continue
		default:
			if k == "" {
				continue
			}
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		out = []string{"cpu"}
	}
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

func metricLabel(key string) string {
	switch key {
	case "cpu":
		return "CPU %"
	case "memory":
		return "内存 %"
	case "disk":
		return "磁盘 %"
	case "load":
		return "Load1"
	case "network":
		return "网络 MB/s"
	case "io":
		return "磁盘 IO MB/s"
	default:
		return key
	}
}

func sampleMetricValue(s shared.Sample, key string) (float64, bool) {
	switch key {
	case "cpu":
		return s.CPUPercent, true
	case "memory":
		return s.MemPercent, true
	case "disk":
		return s.DiskPercent, true
	case "load":
		return s.Load1, true
	case "network":
		return (s.NetRecvRate + s.NetSentRate) / 1048576, true
	case "io":
		return (s.DiskReadRate + s.DiskWriteRate) / 1048576, true
	default:
		return 0, false
	}
}

func (h *SreyunCore) loadHostSamples(hostID string, from, to int64) []shared.Sample {
	var samples []shared.Sample
	if h.s != nil && h.s.vm != nil && h.s.vm.enabled() {
		samples, _ = h.s.vm.queryHistory(hostID, from, to)
	}
	if len(samples) == 0 && h.s != nil && h.s.store != nil {
		samples, _ = h.s.store.GetHistory(hostID, from, to)
	}
	return samples
}

func downsampleSamples(samples []shared.Sample, maxPts int) []shared.Sample {
	if maxPts < 30 {
		maxPts = 30
	}
	n := len(samples)
	if n <= maxPts {
		return samples
	}
	out := make([]shared.Sample, 0, maxPts)
	step := float64(n-1) / float64(maxPts-1)
	for i := 0; i < maxPts; i++ {
		idx := int(math.Round(float64(i) * step))
		if idx >= n {
			idx = n - 1
		}
		out = append(out, samples[idx])
	}
	return out
}

func hostSamplesToChatChart(samples []shared.Sample, metrics []string, title string) (chart map[string]any, stats map[string]any) {
	samples = downsampleSamples(samples, 180)
	seriesDefs := make([]map[string]any, 0, len(metrics))
	for i, m := range metrics {
		seriesDefs = append(seriesDefs, map[string]any{
			"key":   "s" + fmt.Sprint(i),
			"label": metricLabel(m),
			"color": chatChartColors[i%len(chatChartColors)],
		})
	}
	rows := make([]map[string]any, 0, len(samples))
	statAcc := map[string]*struct{ min, max, sum float64; n int }{}
	for _, m := range metrics {
		statAcc[m] = &struct{ min, max, sum float64; n int }{min: math.Inf(1), max: math.Inf(-1)}
	}
	for _, s := range samples {
		row := map[string]any{"timestamp": s.Timestamp}
		for i, m := range metrics {
			v, ok := sampleMetricValue(s, m)
			if !ok {
				continue
			}
			row["s"+fmt.Sprint(i)] = round3(v)
			acc := statAcc[m]
			if v < acc.min {
				acc.min = v
			}
			if v > acc.max {
				acc.max = v
			}
			acc.sum += v
			acc.n++
		}
		rows = append(rows, row)
	}
	yMax := 100.0
	for _, m := range metrics {
		if m == "load" || m == "network" || m == "io" {
			yMax = 0
			break
		}
	}
	chart = map[string]any{
		"samples": rows,
		"series":  seriesDefs,
		"title":   title,
	}
	if yMax > 0 {
		chart["y_min"] = 0
		chart["y_max"] = yMax
	}
	stats = map[string]any{}
	for _, m := range metrics {
		acc := statAcc[m]
		if acc.n == 0 {
			continue
		}
		stats[m] = map[string]any{
			"min": round3(acc.min),
			"max": round3(acc.max),
			"avg": round3(acc.sum / float64(acc.n)),
			"n":   acc.n,
		}
	}
	return chart, stats
}

func promMatrixToChatChart(series []promMatrix, title string, maxSeries int) map[string]any {
	if maxSeries <= 0 {
		maxSeries = 6
	}
	if len(series) > maxSeries {
		series = series[:maxSeries]
	}
	// Build union of timestamps
	tsSet := map[int64]struct{}{}
	for _, s := range series {
		for _, p := range s.Points {
			tsSet[int64(p[0])] = struct{}{}
		}
	}
	tsList := make([]int64, 0, len(tsSet))
	for ts := range tsSet {
		tsList = append(tsList, ts)
	}
	sort.Slice(tsList, func(i, j int) bool { return tsList[i] < tsList[j] })
	// Downsample timestamps
	if len(tsList) > 180 {
		step := float64(len(tsList)-1) / 179.0
		reduced := make([]int64, 0, 180)
		for i := 0; i < 180; i++ {
			idx := int(math.Round(float64(i) * step))
			if idx >= len(tsList) {
				idx = len(tsList) - 1
			}
			reduced = append(reduced, tsList[idx])
		}
		tsList = reduced
	}
	seriesDefs := make([]map[string]any, 0, len(series))
	lookups := make([]map[int64]float64, len(series))
	for i, s := range series {
		lbl := chartLegendFromLabels(s.Labels)
		if lbl == "" {
			lbl = fmt.Sprintf("series-%d", i+1)
		}
		seriesDefs = append(seriesDefs, map[string]any{
			"key":   "s" + fmt.Sprint(i),
			"label": lbl,
			"color": chatChartColors[i%len(chatChartColors)],
		})
		m := make(map[int64]float64, len(s.Points))
		for _, p := range s.Points {
			m[int64(p[0])] = p[1]
		}
		lookups[i] = m
	}
	rows := make([]map[string]any, 0, len(tsList))
	for _, ts := range tsList {
		row := map[string]any{"timestamp": ts}
		for i := range series {
			if v, ok := lookups[i][ts]; ok {
				row["s"+fmt.Sprint(i)] = round3(v)
			}
		}
		rows = append(rows, row)
	}
	return map[string]any{
		"samples": rows,
		"series":  seriesDefs,
		"title":   title,
	}
}

func chartLegendFromLabels(labels map[string]string) string {
	if labels == nil {
		return ""
	}
	for _, k := range []string{"instance", "host", "hostname", "job", "device", "mountpoint", "name"} {
		if v := strings.TrimSpace(labels[k]); v != "" {
			return v
		}
	}
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		if k == "__name__" {
			continue
		}
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, ",")
}

func round3(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*1000) / 1000
}

func chartID() string {
	t := genToken()
	if len(t) > 10 {
		return "c" + t[:10]
	}
	return "c" + t
}

func (h *SreyunCore) execRenderChart(args map[string]any) (string, error) {
	source, _ := args["source"].(string)
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		source = "host"
	}
	title, _ := args["title"].(string)
	rangeRaw, _ := args["range"].(string)
	from, to, rangeLabel := parseChartRange(rangeRaw, 6)

	if source == "promql" {
		expr, _ := args["expr"].(string)
		ds, _ := args["datasource"].(string)
		return h.renderPromqlChart(expr, ds, from, to, rangeLabel, title)
	}

	hostID, _ := args["host_id"].(string)
	metricsRaw, _ := args["metrics"].(string)
	return h.renderHostChart(hostID, metricsRaw, from, to, rangeLabel, title)
}

func (h *SreyunCore) execQueryMetricRange(args map[string]any) (string, error) {
	hostID, _ := args["host_id"].(string)
	metric, _ := args["metric"].(string)
	rangeRaw, _ := args["range"].(string)
	from, to, rangeLabel := parseChartRange(rangeRaw, 6)
	return h.renderHostChart(hostID, metric, from, to, rangeLabel, "")
}

func (h *SreyunCore) execQueryPromqlRange(args map[string]any) (string, error) {
	expr, _ := args["expr"].(string)
	ds, _ := args["datasource"].(string)
	title, _ := args["title"].(string)
	rangeRaw, _ := args["range"].(string)
	from, to, rangeLabel := parseChartRange(rangeRaw, 6)
	return h.renderPromqlChart(expr, ds, from, to, rangeLabel, title)
}

func (h *SreyunCore) renderHostChart(hostRef, metricsRaw string, from, to int64, rangeLabel, title string) (string, error) {
	hst := h.resolveHostRef(hostRef)
	if hst == nil {
		return capabilityJSON(capabilityResult{OK: false, Error: fmt.Sprintf("未找到主机 %q", hostRef)}), nil
	}
	metrics := normalizeMetricKeys(metricsRaw, "cpu")
	samples := h.loadHostSamples(hst.ID, from, to)
	if len(samples) < 2 {
		return capabilityJSON(capabilityResult{
			OK:    false,
			Error: fmt.Sprintf("主机 %s 在 %s 内历史样本不足（%d）", hst.Hostname, rangeLabel, len(samples)),
		}), nil
	}
	if strings.TrimSpace(title) == "" {
		title = fmt.Sprintf("%s · %s", hst.Hostname, rangeLabel)
	}
	chart, stats := hostSamplesToChatChart(samples, metrics, title)
	id := chartID()
	actions := []map[string]any{
		showChartAction(id, "查看趋势图 · "+hst.Hostname, title, chart, map[string]any{
			"kind": "host_history", "host_id": hst.ID, "metrics": strings.Join(metrics, ","), "range": rangeLabel,
		}),
		drillDownAction("打开主机详情 · "+hst.Hostname, "host_detail", map[string]any{
			"host_id": hst.ID, "host_name": hst.Hostname,
		}),
		drillDownAction("拓宽到 24h 再看", "prompt", map[string]any{
			"prompt": fmt.Sprintf("请用图表展示主机 %s 最近 24 小时的 %s 趋势", hst.Hostname, strings.Join(metrics, "/")),
		}),
	}
	sum := fmt.Sprintf("已生成「%s」趋势图（%d 点，指标 %s）", title, len(samples), strings.Join(metrics, ","))
	return capabilityJSON(capabilityResult{
		OK:      true,
		Summary: sum,
		Data: map[string]any{
			"chart_id": id,
			"host_id":  hst.ID,
			"hostname": hst.Hostname,
			"range":    rangeLabel,
			"metrics":  metrics,
			"points":   len(samples),
			"stats":    stats,
		},
		UIActions: actions,
	}), nil
}

func (h *SreyunCore) renderPromqlChart(expr, dsID string, from, to int64, rangeLabel, title string) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return capabilityJSON(capabilityResult{OK: false, Error: "expr 必填"}), nil
	}
	if h.s == nil {
		return capabilityJSON(capabilityResult{OK: false, Error: "服务不可用"}), nil
	}
	step := (to - from) / 180
	if step < 15 {
		step = 15
	}
	series, ok := h.s.dashRangeSeries(strings.TrimSpace(dsID), expr, from, to, step)
	if !ok || len(series) == 0 {
		return capabilityJSON(capabilityResult{OK: false, Error: "PromQL 区间查询无数据或数据源不可用"}), nil
	}
	if strings.TrimSpace(title) == "" {
		title = trimLine(expr, 48) + " · " + rangeLabel
	}
	chart := promMatrixToChatChart(series, title, 6)
	id := chartID()
	actions := []map[string]any{
		showChartAction(id, "查看 PromQL 图表", title, chart, map[string]any{
			"kind": "promql", "expr": expr, "datasource": dsID, "range": rangeLabel,
		}),
		drillDownAction("拓宽到 24h", "prompt", map[string]any{
			"prompt": fmt.Sprintf("请用 PromQL `%s` 绘制最近 24 小时趋势图", expr),
		}),
	}
	return capabilityJSON(capabilityResult{
		OK:      true,
		Summary: fmt.Sprintf("已生成 PromQL 图表「%s」（%d 条序列）", title, len(series)),
		Data: map[string]any{
			"chart_id": id,
			"expr":     expr,
			"range":    rangeLabel,
			"series_n": len(series),
		},
		UIActions: actions,
	}), nil
}

func (h *SreyunCore) execShowInstantStat(args map[string]any) (string, error) {
	hostRef, _ := args["host_id"].(string)
	metric, _ := args["metric"].(string)
	rangeRaw, _ := args["range"].(string)
	hst := h.resolveHostRef(hostRef)
	if hst == nil {
		return capabilityJSON(capabilityResult{OK: false, Error: fmt.Sprintf("未找到主机 %q", hostRef)}), nil
	}
	keys := normalizeMetricKeys(metric, "cpu")
	key := keys[0]
	var value float64
	ok := false
	if hst.Latest != nil {
		value, ok = sampleMetricValue(*hst.Latest, key)
	}
	from, to, rangeLabel := parseChartRange(rangeRaw, 1)
	samples := h.loadHostSamples(hst.ID, from, to)
	spark := make([][2]float64, 0, 60)
	if len(samples) > 0 {
		ds := downsampleSamples(samples, 60)
		for _, s := range ds {
			if v, good := sampleMetricValue(s, key); good {
				spark = append(spark, [2]float64{float64(s.Timestamp), round3(v)})
				if !ok {
					value, ok = v, true
				}
			}
		}
	}
	if !ok {
		return capabilityJSON(capabilityResult{OK: false, Error: "暂无该指标数据"}), nil
	}
	unit := "%"
	if key == "load" {
		unit = ""
	}
	if key == "network" || key == "io" {
		unit = "MB/s"
	}
	thresholds := map[string]float64{"warn": 75, "crit": 90}
	if key == "load" {
		cores := 1.0
		if hst.Latest != nil && hst.Latest.CPUCores > 0 {
			cores = float64(hst.Latest.CPUCores)
		}
		thresholds = map[string]float64{"warn": cores * 0.7, "crit": cores}
	}
	title := fmt.Sprintf("%s · %s", hst.Hostname, metricLabel(key))
	id := chartID()
	actions := []map[string]any{
		showStatAction(id, "查看 "+metricLabel(key), title, round3(value), unit, spark, thresholds),
		drillDownAction("查看 "+rangeLabel+" 趋势图", "prompt", map[string]any{
			"prompt": fmt.Sprintf("请用图表展示主机 %s 最近 %s 的 %s 趋势", hst.Hostname, rangeLabel, key),
		}),
		drillDownAction("打开主机详情", "host_detail", map[string]any{
			"host_id": hst.ID, "host_name": hst.Hostname,
		}),
	}
	return capabilityJSON(capabilityResult{
		OK:      true,
		Summary: fmt.Sprintf("%s 当前 %s = %.2f%s", hst.Hostname, metricLabel(key), value, unit),
		Data: map[string]any{
			"host_id": hst.ID, "metric": key, "value": round3(value), "unit": unit,
		},
		UIActions: actions,
	}), nil
}

func (h *SreyunCore) execAnalyzeMetricTrend(args map[string]any) (string, error) {
	hostRef, _ := args["host_id"].(string)
	metricsRaw, _ := args["metrics"].(string)
	hours := 6.0
	if v, ok := args["hours"].(float64); ok && v > 0 {
		hours = v
	}
	if hours > 72 {
		hours = 72
	}
	hst := h.resolveHostRef(hostRef)
	if hst == nil {
		return capabilityJSON(capabilityResult{OK: false, Error: fmt.Sprintf("未找到主机 %q", hostRef)}), nil
	}
	metrics := normalizeMetricKeys(metricsRaw, "cpu,memory,disk,load")
	from := time.Now().Unix() - int64(hours)*3600
	to := time.Now().Unix()
	rangeLabel := fmt.Sprintf("%.0fh", hours)
	samples := h.loadHostSamples(hst.ID, from, to)
	if len(samples) < 2 {
		return capabilityJSON(capabilityResult{
			OK:    false,
			Error: fmt.Sprintf("主机 %s 最近 %s 历史不足", hst.Hostname, rangeLabel),
		}), nil
	}
	title := fmt.Sprintf("%s 趋势分析 · %s", hst.Hostname, rangeLabel)
	chart, stats := hostSamplesToChatChart(samples, metrics, title)

	// early/late window deltas for narrative
	n := len(samples)
	third := n / 3
	if third < 1 {
		third = 1
	}
	early, late := samples[:third], samples[n-third:]
	avgWin := func(ss []shared.Sample, key string) float64 {
		var sum float64
		var c int
		for _, s := range ss {
			if v, ok := sampleMetricValue(s, key); ok {
				sum += v
				c++
			}
		}
		if c == 0 {
			return 0
		}
		return sum / float64(c)
	}
	type trendRow struct {
		Metric string  `json:"metric"`
		Early  float64 `json:"early_avg"`
		Late   float64 `json:"late_avg"`
		Delta  float64 `json:"delta"`
		Trend  string  `json:"trend"`
	}
	trends := make([]trendRow, 0, len(metrics))
	notable := make([]string, 0)
	for _, m := range metrics {
		e, l := avgWin(early, m), avgWin(late, m)
		d := l - e
		row := trendRow{Metric: metricLabel(m), Early: round3(e), Late: round3(l), Delta: round3(d), Trend: trendArrow(d)}
		if m == "load" {
			row.Trend = trendArrow(d * 10)
		}
		trends = append(trends, row)
		if math.Abs(d) >= 5 || (m == "load" && math.Abs(d) >= 0.5) {
			notable = append(notable, fmt.Sprintf("%s %s%.1f", metricLabel(m), row.Trend, d))
		}
	}
	id := chartID()
	actions := []map[string]any{
		showChartAction(id, "查看趋势图 · "+hst.Hostname, title, chart, map[string]any{
			"kind": "host_history", "host_id": hst.ID, "metrics": strings.Join(metrics, ","), "range": rangeLabel,
		}),
		drillDownAction("打开主机详情下钻", "host_detail", map[string]any{
			"host_id": hst.ID, "host_name": hst.Hostname,
		}),
		drillDownAction("只看 CPU 曲线", "prompt", map[string]any{
			"prompt": fmt.Sprintf("请用图表展示主机 %s 最近 %s 的 CPU 趋势", hst.Hostname, rangeLabel),
		}),
		drillDownAction("对比内存与磁盘", "prompt", map[string]any{
			"prompt": fmt.Sprintf("请用图表对比主机 %s 最近 %s 的内存和磁盘使用率", hst.Hostname, rangeLabel),
		}),
	}
	sum := fmt.Sprintf("已完成 %s 近 %s 趋势分析", hst.Hostname, rangeLabel)
	if len(notable) > 0 {
		sum += "；显著变化：" + strings.Join(notable, "，")
	} else {
		sum += "；整体波动平稳"
	}
	return capabilityJSON(capabilityResult{
		OK:      true,
		Summary: sum,
		Data: map[string]any{
			"chart_id": id,
			"host_id":  hst.ID,
			"hostname": hst.Hostname,
			"range":    rangeLabel,
			"trends":   trends,
			"stats":    stats,
			"points":   len(samples),
		},
		UIActions: actions,
	}), nil
}
