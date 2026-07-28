/* dash_charts.js — ECharts adapter for commercial BI dashboard panels.
 * Depends on global `echarts` (loaded from /js/vendor/echarts.min.js before /app.js).
 * Hosts page keeps Canvas createChart; dashboard timeseries/pie/bar/gauge/heatmap use this.
 */
(function (global) {
  "use strict";

  const PALETTES = {
    classic: ["#4c8dff", "#22c55e", "#f59e0b", "#ef4d5a", "#a855f7", "#06b6d4", "#eab308", "#ec4899", "#14b8a6", "#f97316"],
    warm: ["#ef4444", "#f97316", "#f59e0b", "#eab308", "#fb7185", "#fdba74", "#fcd34d", "#dc2626"],
    cool: ["#0ea5e9", "#06b6d4", "#14b8a6", "#3b82f6", "#6366f1", "#8b5cf6", "#22d3ee", "#38bdf8"],
    traffic: ["#22c55e", "#eab308", "#f59e0b", "#ef4444", "#16a34a", "#ca8a04", "#ea580c", "#b91c1c"],
    mono: ["#1e293b", "#334155", "#475569", "#64748b", "#94a3b8", "#cbd5e1", "#0f172a", "#78716c"]
  };

  const _inst = new WeakMap(); // HTMLElement → echarts.Instance

  function cssVar(name, fallback) {
    try {
      const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
      return v || fallback;
    } catch (e) { return fallback; }
  }

  function themeColors() {
    return {
      txt: cssVar("--txt", "#1e293b"),
      muted: cssVar("--muted", "#64748b"),
      line: cssVar("--line2", "#e2e8f0"),
      card: cssVar("--card", "#ffffff"),
      accent: cssVar("--accent", "#4c8dff"),
      ok: cssVar("--ok", "#22c55e"),
      warn: cssVar("--warn", "#f59e0b"),
      crit: cssVar("--crit", "#ef4444")
    };
  }

  function resolveColor(c, t) {
    if (!c) return t.accent;
    if (c === "var(--ok)") return t.ok;
    if (c === "var(--warn)") return t.warn;
    if (c === "var(--crit)") return t.crit;
    if (c === "var(--accent)") return t.accent;
    if (c === "var(--txt)") return t.txt;
    if (c === "var(--muted)") return t.muted;
    return c;
  }

  function paletteColors(opts) {
    const o = opts || {};
    if (o.palette === "custom" && Array.isArray(o.colors) && o.colors.length) return o.colors.slice();
    return (PALETTES[o.palette] || PALETTES.classic).slice();
  }

  function colorAt(opts, i) {
    const arr = paletteColors(opts);
    return arr[i % arr.length];
  }

  function panelOpts(p) {
    return (p && p.options) || {};
  }

  function effectiveDecimals(p) {
    const o = panelOpts(p);
    if (o.decimals != null && o.decimals !== "") return +o.decimals;
    if (p && p.decimals != null && p.decimals !== 0) return +p.decimals;
    return null;
  }

  function sortItems(items, sort) {
    const s = (sort || "desc").toLowerCase();
    if (s === "none") return items.slice();
    const out = items.slice();
    out.sort((a, b) => s === "asc" ? (a.val - b.val) : (b.val - a.val));
    return out;
  }

  function applyLimit(items, limit, fallback) {
    const n = limit > 0 ? limit : fallback;
    return n > 0 ? items.slice(0, n) : items;
  }

  function thresholdColor(v, thresholds, unit, min, max, fallback, thresholdMode) {
    const t = themeColors();
    let cmp = +v;
    // percentage mode: steps are 0–100 of the [min,max] (or unit scale) range
    if (String(thresholdMode || "").toLowerCase() === "percentage") {
      let pct = null;
      if (unit === "percent") pct = v;
      else if (unit === "percentunit") pct = v * 100;
      else if (max != null && min != null && max > min) pct = (v - min) / (max - min) * 100;
      if (pct != null) cmp = pct;
    }
    const steps = (thresholds || []).map(x => ({
      value: +x.value,
      color: resolveColor(x.color, t)
    })).filter(x => !isNaN(x.value)).sort((a, b) => a.value - b.value);
    if (!steps.length) {
      let pct = null;
      if (unit === "percent") pct = v;
      else if (unit === "percentunit") pct = v * 100;
      else if (max != null && min != null && max > min) pct = (v - min) / (max - min) * 100;
      if (pct == null) return fallback || t.accent;
      return pct >= 90 ? t.crit : pct >= 75 ? t.warn : t.ok;
    }
    let col = steps[0].color;
    for (const s of steps) {
      if (cmp >= s.value) col = s.color;
    }
    return col;
  }

  function legendOpt(mode, t) {
    const m = (mode || "bottom").toLowerCase();
    if (m === "hidden") return { show: false };
    const isRight = m === "right";
    const isTop = m === "top";
    // 顶部/底部横排可滚动；右侧竖排。顶部图例与标题区对齐，避免压住曲线。
    return {
      show: true,
      type: "scroll",
      orient: isRight ? "vertical" : "horizontal",
      left: isRight ? undefined : "center",
      right: isRight ? 2 : undefined,
      top: isRight ? "middle" : (isTop ? 4 : undefined),
      bottom: (!isRight && !isTop) ? 2 : undefined,
      width: isRight ? 100 : "92%",
      padding: isTop ? [2, 10, 0, 10] : [2, 8],
      itemWidth: 10,
      itemHeight: 8,
      itemGap: isRight ? 8 : 12,
      textStyle: {
        color: t.muted,
        fontSize: 11,
        overflow: "truncate",
        width: isRight ? 78 : 132
      },
      pageIconColor: t.muted,
      pageIconInactiveColor: t.line,
      pageTextStyle: { color: t.muted, fontSize: 10 },
      pageIconSize: 10,
      pageButtonItemGap: 4,
      pageButtonGap: 6,
      inactiveColor: t.line,
      tooltip: { show: true }
    };
  }

  /** Timeseries grid gutters reserved for legend + dataZoom slider. */
  function timeseriesGrid(leg, axisHidden) {
    const m = (leg || "bottom").toLowerCase();
    const top = m === "top" ? 40 : 28;
    const right = m === "right" ? 112 : 12;
    // bottom: room for x-axis + optional bottom legend + slider
    let bottom = 36;
    if (m === "bottom") bottom = 54;
    else if (m === "hidden") bottom = 34;
    return {
      left: axisHidden ? 8 : 12,
      right,
      top,
      bottom,
      containLabel: true
    };
  }

  /** Shared tooltip — always confined so panels with overflow:hidden never clip tips. */
  function tipBase(t, extra) {
    return Object.assign({
      backgroundColor: t.card,
      borderColor: t.line,
      textStyle: { color: t.txt, fontSize: 12 },
      confine: true,
      appendToBody: true,
      extraCssText: "max-width:280px;white-space:normal;word-break:break-word;z-index:9999;"
    }, extra || {});
  }

  /** Shared cartesian grid — containLabel + minimum gutters (never paint under legend/axis). */
  function gridSafe(extra) {
    return Object.assign({
      left: 8, right: 10, top: 24, bottom: 36,
      containLabel: true
    }, extra || {});
  }

  function ensureInst(el) {
    if (!global.echarts) {
      console.warn("[dash_charts] echarts not loaded");
      return null;
    }
    let chart = _inst.get(el);
    if (chart && !chart.isDisposed()) return chart;
    chart = global.echarts.init(el, null, { renderer: "canvas" });
    _inst.set(el, chart);
    return chart;
  }

  function dispose(el) {
    if (!el) return;
    const chart = _inst.get(el);
    if (chart && !chart.isDisposed()) {
      try { chart.dispose(); } catch (e) {}
    }
    _inst.delete(el);
  }

  function resize(el) {
    const chart = _inst.get(el);
    if (chart && !chart.isDisposed()) {
      try { chart.resize(); } catch (e) {}
    }
  }

  function resizeAll(root) {
    const scope = root || document;
    scope.querySelectorAll(".dash-echart").forEach(resize);
  }

  function fmtAxis(v, unit, decimals, fmtUnitFn) {
    if (typeof fmtUnitFn === "function") return fmtUnitFn(v, unit, decimals);
    if (v == null || isNaN(v)) return "-";
    if (decimals != null && !isNaN(decimals)) return (+v).toFixed(+decimals);
    return String(v);
  }

  /** @param {HTMLElement} el
   *  @param {object} cfg { type, panel, series|items|matrix, from, to, fmtUnit }
   */
  function render(el, cfg) {
    if (!el) return null;
    const chart = ensureInst(el);
    if (!chart) return null;
    const t = themeColors();
    const p = cfg.panel || {};
    let o = panelOpts(p);
    // Short panels: hide legend so series/axes never compete for space.
    if (el.clientHeight > 0 && el.clientHeight < 160 && o.legend !== "hidden") {
      o = Object.assign({}, o, { legend: "hidden" });
    }
    const dec = effectiveDecimals(p);
    const fmt = (v) => fmtAxis(v, p.unit, dec, cfg.fmtUnit);
    const type = (cfg.type || p.type || "timeseries").toLowerCase();

    let option;
    if (type === "timeseries") option = buildTimeseries(cfg, p, o, t, fmt);
    else if (type === "piechart" || type === "pie") option = buildPie(cfg, p, o, t, fmt);
    else if (type === "barchart" || type === "bar") option = buildBar(cfg, p, o, t, fmt);
    else if (type === "bargauge") option = buildBarGauge(cfg, p, o, t, fmt);
    else if (type === "gauge") option = buildGauge(cfg, p, o, t, fmt);
    else if (type === "histogram") option = buildHistogram(cfg, p, o, t, fmt);
    else if (type === "heatmap") option = buildHeatmap(cfg, p, o, t, fmt);
    else if (type === "candlestick") option = buildCandlestick(cfg, p, o, t, fmt);
    else if (type === "radar") option = buildRadar(cfg, p, o, t, fmt);
    else if (type === "sankey") option = buildSankey(cfg, p, o, t, fmt);
    else if (type === "stat") option = buildStatSpark(cfg, p, o, t, fmt);
    else option = buildTimeseries(cfg, p, o, t, fmt);

    chart.setOption(option, true);
    requestAnimationFrame(() => { try { chart.resize(); } catch (e) {} });
    return chart;
  }

  function markLines(thresholds, t) {
    const steps = (thresholds || []).filter(x => x && !isNaN(+x.value));
    if (!steps.length) return undefined;
    return {
      silent: true,
      symbol: "none",
      lineStyle: { type: "dashed", width: 1 },
      data: steps.map(s => ({
        yAxis: +s.value,
        lineStyle: { color: resolveColor(s.color, t) },
        label: { formatter: String(s.value), color: t.muted, fontSize: 10 }
      }))
    };
  }

  function buildTimeseries(cfg, p, o, t, fmt) {
    const collected = cfg.series || [];
    const style = (o.chart_style || (o.draw_style === "bars" ? "bar" : "line")).toLowerCase();
    const stacked = !!o.stacked;
    const smooth = !!o.smooth;
    const showPoints = !!o.show_points || o.draw_style === "points";
    const lineW = o.line_width != null ? +o.line_width : 2;
    const fillOp = o.fill_opacity != null ? Math.max(0, Math.min(1, +o.fill_opacity / 100)) : null;
    const pointSz = o.point_size != null ? +o.point_size : 4;
    const axisHidden = o.axis_placement === "hidden";
    const colors = paletteColors(o);
    const compareColors = ["#94a3b8", "#a78bfa", "#67e8f9", "#fdba74"];
    const series = [];
    const legendNames = [];
    let histIdx = 0;
    // Boundary between realtime (left) and forecast (right)
    let nowTsSec = 0, fcMaxTsSec = 0, histMinTs = 0;
    collected.forEach(c => {
      const kind = (c.kind || "history").toLowerCase();
      const pts = c.points || [];
      if ((!kind || kind === "history") && pts.length) {
        const first = +pts[0][0], last = +pts[pts.length - 1][0];
        if (!histMinTs || first < histMinTs) histMinTs = first;
        if (last > nowTsSec) nowTsSec = last;
      }
      if (kind === "forecast") {
        pts.forEach(pt => { const x = +pt[0]; if (x > fcMaxTsSec) fcMaxTsSec = x; });
        (c.band || []).forEach(pt => { const x = +(pt.ts || pt[0] || 0); if (x > fcMaxTsSec) fcMaxTsSec = x; });
      }
    });
    // 仅在确有「现在之后」的预测点时启用中轴拆分；预测关闭时绝不预留未来空白轴
    const hasForecast = fcMaxTsSec > nowTsSec + 1;
    if (hasForecast && cfg.nowTs) nowTsSec = Math.max(nowTsSec, +cfg.nowTs);
    let xMinMs = null, xMaxMs = null;
    if (hasForecast && nowTsSec > 0) {
      const histSpan = histMinTs > 0 ? Math.max(60, nowTsSec - histMinTs) : Math.max(60, fcMaxTsSec - nowTsSec);
      const fcSpan = Math.max(60, fcMaxTsSec - nowTsSec);
      const half = Math.max(histSpan, fcSpan);
      xMinMs = (nowTsSec - half) * 1000;
      xMaxMs = (nowTsSec + half) * 1000;
    }

    collected.forEach((c, i) => {
      const kind = (c.kind || c.role || "history").toLowerCase();
      const isForecast = kind === "forecast";
      const isCompare = kind === "compare_pop" || kind === "compare_yoy" || kind === "compare";
      let col = colors[histIdx % colors.length];
      if (isForecast) {
        const fi = collected.filter(x => x.kind === "forecast").indexOf(c);
        col = colors[(fi >= 0 ? fi : 0) % colors.length] || t.accent;
      }
      if (isCompare) col = compareColors[(kind === "compare_yoy" ? 1 : 0) % compareColors.length];
      if (!isForecast && !isCompare) histIdx++;

      // Confidence band — 不进图例，避免「置信带/区间」刷屏
      if (isForecast && Array.isArray(c.band) && c.band.length) {
        const loData = c.band.map(pt => [Math.round((+pt.ts || +pt[0] || 0) * 1000), +pt.lo]);
        const spanData = c.band.map(pt => [Math.round((+pt.ts || +pt[0] || 0) * 1000), Math.max(0, (+pt.hi) - (+pt.lo))]);
        const bandKey = "_fcband_" + i;
        series.push({
          name: bandKey + "_lo",
          type: "line",
          data: loData,
          lineStyle: { opacity: 0 },
          stack: "fcband_" + i,
          symbol: "none",
          silent: true,
          tooltip: { show: false },
          legendHoverLink: false,
          areaStyle: { opacity: 0 },
          z: 1
        });
        series.push({
          name: bandKey + "_hi",
          type: "line",
          data: spanData,
          lineStyle: { opacity: 0 },
          stack: "fcband_" + i,
          symbol: "none",
          silent: true,
          tooltip: { show: false },
          legendHoverLink: false,
          areaStyle: { color: col, opacity: 0.1 },
          z: 1
        });
      }

      const data = (c.points || []).map(pt => [Math.round(pt[0] * 1000), pt[1]]);
      let displayName = c.name || ("s" + i);
      if (isForecast) {
        // 「Eason · 预测」→ 简洁「预测」；多序列时保留前缀
        const base = String(displayName).replace(/\s*·\s*预测\s*$/, "").trim();
        const fcCount = collected.filter(x => x.kind === "forecast").length;
        displayName = fcCount <= 1 ? "预测" : (base + " · 预测");
      }
      if (!isForecast || !String(c.name || "").match(/置信|区间/)) {
        if (!isForecast || legendNames.indexOf(displayName) < 0) {
          if (!(String(displayName).indexOf("_fcband_") === 0)) legendNames.push(displayName);
        }
      }
      const base = {
        name: displayName,
        type: style === "bar" && !isForecast ? "bar" : "line",
        data,
        smooth: style === "bar" || isForecast ? false : smooth,
        showSymbol: showPoints && !isForecast,
        symbolSize: pointSz,
        connectNulls: !!o.span_nulls,
        sampling: isForecast ? undefined : "lttb",
        emphasis: { focus: "series" },
        itemStyle: {
          color: col,
          opacity: isCompare ? 0.75 : 1,
          borderType: isForecast && style === "bar" ? "dashed" : undefined
        },
        lineStyle: {
          width: isForecast ? Math.max(2, lineW) : (isCompare ? 1.6 : lineW),
          color: col,
          type: isForecast ? "dashed" : "solid",
          opacity: isCompare ? 0.7 : 1
        },
        stack: stacked && !isForecast && !isCompare ? "total" : undefined,
        z: isForecast ? 6 : (isCompare ? 2 : 3),
        _kind: kind
      };
      if (!isForecast && !isCompare && (style === "area" || (stacked && style !== "bar") || (fillOp != null && fillOp > 0 && style !== "bar"))) {
        base.areaStyle = { opacity: fillOp != null ? fillOp : (stacked ? 0.45 : 0.12) };
      }
      // Attach now-line + left/right shade once on the first history series
      if (histIdx === 1 && !isForecast && !isCompare) {
        const marks = [];
        if (o.thresholds && o.thresholds.length) {
          const ml = markLines(o.thresholds, t);
          if (ml && ml.data) marks.push(...ml.data);
        }
        if (hasForecast && nowTsSec > 0) {
          const rightEdge = xMaxMs ? xMaxMs / 1000 : fcMaxTsSec;
          const leftEdge = xMinMs ? xMinMs / 1000 : histMinTs;
          marks.push({
            xAxis: nowTsSec * 1000,
            label: {
              formatter: "现在",
              color: t.txt,
              fontSize: 11,
              fontWeight: 600,
              backgroundColor: t.card,
              padding: [2, 6],
              borderRadius: 3,
              position: "insideEndTop"
            },
            lineStyle: { color: t.crit || "#ef4444", type: "solid", width: 2, opacity: 0.9 }
          });
          base.markArea = {
            silent: true,
            data: [
              [
                { xAxis: leftEdge * 1000, itemStyle: { color: "rgba(34, 197, 94, 0.04)" } },
                { xAxis: nowTsSec * 1000 }
              ],
              [
                { xAxis: nowTsSec * 1000, itemStyle: { color: "rgba(99, 102, 241, 0.07)" } },
                { xAxis: rightEdge * 1000 }
              ]
            ]
          };
        }
        if (marks.length) {
          base.markLine = {
            silent: true,
            symbol: "none",
            data: marks
          };
        }
      }
      series.push(base);
    });
    const yMin = p.min != null ? +p.min : undefined;
    const yMax = p.max != null ? +p.max : undefined;
    const leg = (o.legend || "bottom").toLowerCase();
    const sliderBottom = leg === "bottom" ? 30 : 6;
    const legCfg = legendOpt(leg, t);
    if (legCfg.show !== false && legendNames.length) {
      legCfg.data = legendNames.filter(n => n && String(n).indexOf("_fcband_") !== 0);
    }
    return {
      color: colors,
      animation: false,
      grid: timeseriesGrid(leg, axisHidden),
      tooltip: tipBase(t, {
        trigger: "axis",
        formatter: (params) => {
          if (!Array.isArray(params) || !params.length) return "";
          const head = params[0].axisValueLabel || "";
          const axisMs = Array.isArray(params[0].value) ? +params[0].value[0] : +params[0].axisValue;
          const isFuture = hasForecast && nowTsSec > 0 && axisMs > nowTsSec * 1000;
          let html = head + (isFuture ? ' <span class="dash-tip-fc">预测</span>' : ' <span class="dash-tip-cmp">历史</span>') + "<br/>";
          params.forEach(pr => {
            if (!pr || (pr.seriesName && String(pr.seriesName).indexOf("_fcband_") >= 0)) return;
            const v = Array.isArray(pr.value) ? pr.value[1] : pr.value;
            if (v == null || v === "-") return;
            html += `${pr.marker}${pr.seriesName}: <b>${fmt(v)}</b><br/>`;
          });
          return html;
        }
      }),
      legend: legCfg,
      dataZoom: [
        { type: "inside", throttle: 50, xAxisIndex: 0 },
        { type: "slider", height: 14, bottom: sliderBottom, borderColor: t.line, textStyle: { color: t.muted, fontSize: 10 } }
      ],
      xAxis: {
        type: "time",
        min: xMinMs != null ? xMinMs : undefined,
        max: xMaxMs != null ? xMaxMs : undefined,
        axisLine: { lineStyle: { color: t.line } },
        axisLabel: { color: t.muted, fontSize: 10, hideOverlap: true },
        splitLine: { show: false }
      },
      yAxis: {
        type: "value",
        show: !axisHidden,
        position: o.axis_placement === "right" ? "right" : "left",
        min: yMin,
        max: yMax,
        scale: yMin == null && yMax == null,
        axisLine: { show: false },
        axisLabel: { color: t.muted, fontSize: 10, formatter: (v) => fmt(v), hideOverlap: true },
        splitLine: { lineStyle: { color: t.line, type: "dashed" } }
      },
      series
    };
  }

  function buildPie(cfg, p, o, t, fmt) {
    let items = (cfg.items || []).map((it, i) => ({
      name: it.lbl || it.name || ("#" + (i + 1)),
      value: Math.max(0, +it.val),
      itemStyle: { color: it.col || colorAt(o, i) }
    })).filter(it => it.value > 0);
    items = applyLimit(sortItems(items.map(it => ({ ...it, val: it.value })), o.sort || "desc").map(it => ({ name: it.name, value: it.val, itemStyle: it.itemStyle })), o.limit, 12);
    const legMode = (o.legend || "right").toLowerCase();
    let center = ["50%", "50%"];
    let radius = ["42%", "68%"];
    if (legMode === "right") { center = ["36%", "50%"]; }
    else if (legMode === "top") { center = ["50%", "58%"]; radius = ["38%", "62%"]; }
    else if (legMode === "bottom") { center = ["50%", "44%"]; radius = ["38%", "62%"]; }
    else if (legMode === "hidden") { center = ["50%", "50%"]; radius = ["44%", "70%"]; }
    return {
      color: paletteColors(o),
      animationDuration: 300,
      tooltip: tipBase(t, { trigger: "item", valueFormatter: (v) => fmt(v) }),
      legend: legendOpt(legMode, t),
      series: [{
        type: "pie",
        radius,
        center,
        avoidLabelOverlap: true,
        itemStyle: { borderRadius: 4, borderColor: t.card, borderWidth: 2 },
        label: { show: false },
        data: items
      }]
    };
  }

  function buildBar(cfg, p, o, t, fmt) {
    let items = (cfg.items || []).map((it, i) => ({
      lbl: it.lbl || it.name || ("#" + (i + 1)),
      val: +it.val,
      col: it.col || colorAt(o, i)
    }));
    items = applyLimit(sortItems(items, o.sort || "desc"), o.limit, 16);
    return {
      color: paletteColors(o),
      animationDuration: 300,
      grid: gridSafe({ top: 20, bottom: items.length > 8 ? 48 : 28 }),
      tooltip: tipBase(t, { trigger: "axis", valueFormatter: (v) => fmt(v) }),
      legend: { show: false },
      xAxis: {
        type: "category",
        data: items.map(it => it.lbl),
        axisLabel: { color: t.muted, fontSize: 10, rotate: items.length > 8 ? 30 : 0, interval: 8, hideOverlap: true },
        axisLine: { lineStyle: { color: t.line } }
      },
      yAxis: {
        type: "value",
        min: p.min != null ? +p.min : undefined,
        max: p.max != null ? +p.max : undefined,
        axisLabel: { color: t.muted, fontSize: 10, formatter: (v) => fmt(v), hideOverlap: true },
        splitLine: { lineStyle: { color: t.line, type: "dashed" } }
      },
      series: [{
        type: "bar",
        data: items.map(it => ({ value: it.val, itemStyle: { color: it.col, borderRadius: [4, 4, 0, 0] } })),
        barMaxWidth: 36
      }]
    };
  }

  function buildBarGauge(cfg, p, o, t, fmt) {
    let items = (cfg.items || []).map((it, i) => ({
      lbl: it.lbl || ("#" + (i + 1)),
      val: +it.val
    }));
    items = applyLimit(sortItems(items, o.sort || "desc"), o.limit, 16);
    const min = p.min != null ? +p.min : 0;
    const max = p.max != null ? +p.max : (p.unit === "percent" ? 100 : (p.unit === "percentunit" ? 1 : Math.max(...items.map(it => it.val), 1) * 1.1));
    return {
      animationDuration: 300,
      grid: gridSafe({ left: 12, right: 48, top: 8, bottom: 8 }),
      tooltip: tipBase(t, { trigger: "axis", valueFormatter: (v) => fmt(v) }),
      xAxis: { type: "value", min, max, show: false },
      yAxis: {
        type: "category",
        data: items.map(it => it.lbl).reverse(),
        axisLabel: { color: t.muted, fontSize: 11, width: 88, overflow: "truncate" },
        axisLine: { show: false },
        axisTick: { show: false }
      },
      series: [{
        type: "bar",
        data: items.map(it => ({
          value: it.val,
          itemStyle: { color: thresholdColor(it.val, o.thresholds, p.unit, min, max, colorAt(o, 0), o.threshold_mode), borderRadius: 4 }
        })).reverse(),
        barMaxWidth: 18,
        label: { show: true, position: "right", color: t.txt, fontSize: 11, formatter: (p2) => fmt(p2.value) }
      }]
    };
  }

  function buildGauge(cfg, p, o, t, fmt) {
    const items = applyLimit((cfg.items || []).map((it, i) => ({
      lbl: it.lbl || ("#" + (i + 1)),
      val: +it.val
    })), o.limit, 9);
    const min = p.min != null ? +p.min : 0;
    const max = p.max != null ? +p.max : (p.unit === "percent" ? 100 : (p.unit === "percentunit" ? 1 : Math.max(...items.map(it => it.val), 1) * 1.1));
    // Multi-gauge: use one chart with multiple gauge series arranged, or single if one item.
    if (items.length <= 1) {
      const v = items[0] ? items[0].val : 0;
      const col = thresholdColor(v, o.thresholds, p.unit, min, max, t.accent);
      return singleGaugeOption(v, items[0] && items[0].lbl, min, max, col, t, fmt, o);
    }
    // Fall back: render first as primary gauge + note (multi handled by caller splitting cells)
    const v = items[0].val;
    const col = thresholdColor(v, o.thresholds, p.unit, min, max, t.accent);
    return singleGaugeOption(v, items[0].lbl, min, max, col, t, fmt, o);
  }

  function singleGaugeOption(v, name, min, max, col, t, fmt, o) {
    const steps = (o.thresholds || []).map(s => ({
      value: ((+s.value - min) / ((max - min) || 1)) * 100,
      color: resolveColor(s.color, t)
    })).filter(s => !isNaN(s.value)).sort((a, b) => a.value - b.value);
    let axisColors = [[0.75, t.ok], [0.9, t.warn], [1, t.crit]];
    if (steps.length) {
      axisColors = steps.map(s => [Math.max(0, Math.min(1, s.value / 100)), s.color]);
      if (axisColors[axisColors.length - 1][0] < 1) axisColors.push([1, axisColors[axisColors.length - 1][1]]);
    }
    // 聚合序列常见空图例 → "value"；面板标题已足够，仪表下不再重复
    const rawName = String(name || "").trim();
    const showTitle = !!(rawName && !/^value$/i.test(rawName) && !/^aiops_/i.test(rawName) && rawName !== "#1");
    const span = (max - min) || 1;
    // 少刻度 + hideOverlap，避免小面板上 0/20/…/100 叠成一团
    const splitNumber = span <= 1 ? 2 : (span <= 100 ? 4 : 5);
    return {
      series: [{
        type: "gauge",
        min, max,
        center: ["50%", "58%"],
        radius: "88%",
        startAngle: 210,
        endAngle: -30,
        splitNumber,
        progress: { show: true, width: 12, itemStyle: { color: col } },
        axisLine: { lineStyle: { width: 12, color: axisColors } },
        axisTick: { show: false },
        splitLine: { show: false },
        axisLabel: {
          color: t.muted,
          fontSize: 9,
          distance: 6,
          hideOverlap: true,
          formatter: (x) => {
            if (!isFinite(x)) return "";
            // 两端刻度优先，中间过密时由 hideOverlap 裁切
            return fmt(x);
          }
        },
        pointer: { show: true, length: "52%", width: 4, itemStyle: { color: col } },
        anchor: { show: true, size: 8, itemStyle: { color: col } },
        title: {
          show: showTitle,
          offsetCenter: [0, "78%"],
          color: t.muted,
          fontSize: 11,
          overflow: "truncate",
          width: 120
        },
        detail: {
          valueAnimation: true,
          formatter: (x) => fmt(x),
          color: t.txt,
          fontSize: 20,
          offsetCenter: [0, showTitle ? "42%" : "48%"]
        },
        data: [{ value: v, name: showTitle ? rawName : "" }]
      }]
    };
  }

  function buildHistogram(cfg, p, o, t, fmt) {
    const bins = cfg.bins || [];
    return {
      color: paletteColors(o),
      grid: gridSafe({ top: 16, bottom: 36 }),
      tooltip: tipBase(t, { trigger: "axis" }),
      xAxis: {
        type: "category",
        data: bins.map(b => b.lbl),
        axisLabel: { color: t.muted, fontSize: 10, rotate: bins.length > 6 ? 30 : 0, hideOverlap: true },
        axisLine: { lineStyle: { color: t.line } }
      },
      yAxis: {
        type: "value",
        axisLabel: { color: t.muted, fontSize: 10, hideOverlap: true },
        splitLine: { lineStyle: { color: t.line, type: "dashed" } }
      },
      series: [{
        type: "bar",
        data: bins.map((b, i) => ({ value: b.count, itemStyle: { color: colorAt(o, i), borderRadius: [3, 3, 0, 0] } })),
        barMaxWidth: 40
      }]
    };
  }

  function buildHeatmap(cfg, p, o, t, fmt) {
    // cfg.matrix: { xLabels[], yLabels[], data: [[xIdx,yIdx,val], ...] }
    const m = cfg.matrix || { xLabels: [], yLabels: [], data: [] };
    const vals = m.data.map(d => d[2]);
    const mn = vals.length ? Math.min(...vals) : 0;
    const mx = vals.length ? Math.max(...vals) : 1;
    return {
      animation: false,
      tooltip: tipBase(t, {
        position: "top",
        formatter: (p2) => {
          const d = p2.data || [];
          return `${m.yLabels[d[1]] || ""}<br>${m.xLabels[d[0]] || ""}: <b>${fmt(d[2])}</b>`;
        }
      }),
      grid: gridSafe({ left: 12, right: 12, top: 12, bottom: 48 }),
      xAxis: { type: "category", data: m.xLabels, axisLabel: { color: t.muted, fontSize: 9, hideOverlap: true }, splitArea: { show: false } },
      yAxis: { type: "category", data: m.yLabels, axisLabel: { color: t.muted, fontSize: 10, width: 70, overflow: "truncate" }, splitArea: { show: false } },
      visualMap: {
        min: mn, max: mx, calculable: false, orient: "horizontal", left: "center", bottom: 4,
        itemWidth: 10, itemHeight: 100,
        inRange: { color: o.palette === "warm" ? ["#fff7ed", "#fdba74", "#ea580c", "#7f1d1d"] : ["#eff6ff", "#93c5fd", "#2563eb", "#1e3a8a"] },
        textStyle: { color: t.muted, fontSize: 10 }
      },
      series: [{
        type: "heatmap",
        data: m.data,
        emphasis: { itemStyle: { shadowBlur: 6, shadowColor: "rgba(0,0,0,0.25)" } }
      }]
    };
  }

  function buildStatSpark(cfg, p, o, t, fmt) {
    const pts = cfg.points || [];
    const data = pts.map(pt => [Math.round(pt[0] * 1000), pt[1]]);
    const last = pts.length ? pts[pts.length - 1][1] : 0;
    const col = thresholdColor(last, o.thresholds, p.unit, p.min, p.max, colorAt(o, 0), o.threshold_mode);
    const series = [{
      type: "line",
      data,
      smooth: true,
      showSymbol: false,
      lineStyle: { width: 2, color: col },
      areaStyle: { color: col, opacity: 0.15 }
    }];
    const fc = cfg.forecastPoints || cfg.forecast || [];
    if (fc.length) {
      series.push({
        type: "line",
        data: fc.map(pt => [Math.round((Array.isArray(pt) ? pt[0] : pt.ts) * 1000), Array.isArray(pt) ? pt[1] : pt.value]),
        smooth: false,
        showSymbol: false,
        lineStyle: { width: 2, color: col, type: "dashed", opacity: 0.85 }
      });
    }
    return {
      grid: { left: 2, right: 2, top: 4, bottom: 2 },
      xAxis: { type: "time", show: false },
      yAxis: { type: "value", show: false, min: "dataMin", max: "dataMax", scale: true },
      series
    };
  }

  function buildCandlestick(cfg, p, o, t, fmt) {
    const data = cfg.ohlc || [];
    return {
      animation: false,
      grid: gridSafe({ top: 20, bottom: 44 }),
      tooltip: tipBase(t, { trigger: "axis" }),
      dataZoom: [{ type: "inside" }, { type: "slider", height: 14, bottom: 22, borderColor: t.line }],
      xAxis: { type: "time", axisLabel: { color: t.muted, fontSize: 10, hideOverlap: true }, axisLine: { lineStyle: { color: t.line } } },
      yAxis: {
        type: "value", scale: true,
        axisLabel: { color: t.muted, fontSize: 10, formatter: (v) => fmt(v), hideOverlap: true },
        splitLine: { lineStyle: { color: t.line, type: "dashed" } }
      },
      series: [{
        type: "candlestick",
        data,
        itemStyle: {
          color: t.crit, color0: t.ok,
          borderColor: t.crit, borderColor0: t.ok
        }
      }]
    };
  }

  function buildRadar(cfg, p, o, t, fmt) {
    const items = cfg.items || [];
    const maxV = Math.max(1, ...items.map(it => +it.val || 0));
    const indicator = items.map(it => ({ name: it.name || it.lbl || "-", max: p.max != null ? +p.max : maxV * 1.15 }));
    const colors = paletteColors(o);
    const leg = (o.legend || "hidden").toLowerCase();
    // radar：底部图例易与轴线重叠，统一改顶部并下移雷达中心
    const radarLeg = leg === "bottom" ? "top" : leg;
    return {
      color: colors,
      animationDuration: 300,
      tooltip: tipBase(t, { trigger: "item" }),
      legend: legendOpt(radarLeg, t),
      radar: {
        indicator,
        center: radarLeg === "top" ? ["50%", "56%"] : (radarLeg === "right" ? ["42%", "52%"] : ["50%", "52%"]),
        radius: radarLeg === "top" || radarLeg === "right" ? "56%" : "62%",
        axisName: { color: t.muted, fontSize: 11 },
        splitLine: { lineStyle: { color: t.line } },
        splitArea: { areaStyle: { color: ["transparent", "rgba(127,127,127,0.04)"] } }
      },
      series: [{
        type: "radar",
        data: [{
          value: items.map(it => +it.val || 0),
          name: p.title || "指标",
          areaStyle: { opacity: 0.2 },
          lineStyle: { width: 2 },
          itemStyle: { color: colors[0] }
        }]
      }]
    };
  }

  function buildSankey(cfg, p, o, t, fmt) {
    const nodes = (cfg.sankey && cfg.sankey.nodes) || [];
    const links = (cfg.sankey && cfg.sankey.links) || [];
    const colors = paletteColors(o);
    return {
      color: colors,
      animationDuration: 300,
      tooltip: tipBase(t, {
        trigger: "item",
        formatter: (params) => {
          if (params.dataType === "edge") return `${params.data.source} → ${params.data.target}<br/>${fmt(params.data.value)}`;
          return params.name;
        }
      }),
      series: [{
        type: "sankey",
        left: 8, right: 8, top: 12, bottom: 12,
        data: nodes,
        links,
        emphasis: { focus: "adjacency" },
        lineStyle: { color: "gradient", curveness: 0.5 },
        label: { color: t.txt, fontSize: 11, overflow: "truncate", width: 80 }
      }]
    };
  }

  // Public helpers shared with dashboard.js
  global.DashCharts = {
    PALETTES,
    render,
    dispose,
    resize,
    resizeAll,
    paletteColors,
    colorAt,
    sortItems,
    applyLimit,
    thresholdColor,
    panelOpts,
    effectiveDecimals,
    themeColors,
    resolveColor
  };
})(typeof window !== "undefined" ? window : globalThis);
