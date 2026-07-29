/* ============================================================================
 * Shared multi-series forecast helper for Canvas charts (hosts / AI cost / SNMP / …).
 * Left = realtime history, right = future forecast (dashed). Uses POST /metrics/forecast.
 * Supports AbortSignal + isCurrent() so stale responses never paint a new canvas.
 * ============================================================================ */
window._FC_ON = window._FC_ON || {};
const FC_MAX_SERIES = 12;

function isChartForecastOn(scope) {
  return !!(window._FC_ON && window._FC_ON[scope]);
}
function setChartForecastOn(scope, on) {
  window._FC_ON = window._FC_ON || {};
  window._FC_ON[scope] = !!on;
}
function forecastChipHTML(scope, on) {
  const active = on != null ? !!on : isChartForecastOn(scope);
  return `<button type="button" class="chip-btn${active ? " active" : ""}" data-chart-forecast="${esc(scope || "default")}" title="多序列趋势预测：左侧实时，右侧未来（虚线）">预测</button>`;
}

function _fcStillCurrent(opts) {
  if (!opts) return true;
  if (typeof opts.isCurrent === "function" && !opts.isCurrent()) return false;
  if (opts.signal && opts.signal.aborted) return false;
  return true;
}

/** Build request payload from Canvas samples + series defs (supports transform). */
function buildForecastRequestSeries(samples, seriesDefs) {
  const use = (seriesDefs || []).filter(s => !s.kind || s.kind === "history").slice(0, FC_MAX_SERIES);
  const out = [];
  for (const s of use) {
    const pts = [];
    for (const sm of samples || []) {
      let v;
      try { v = typeof seriesVal === "function" ? seriesVal(s, sm) : sm[s.key]; } catch (_) { v = null; }
      if (v == null || !isFinite(+v)) continue;
      const ts = sm.timestamp != null ? sm.timestamp : sm.ts;
      if (!ts) continue;
      pts.push([+ts, +v]);
    }
    if (pts.length >= 8) {
      out.push({ name: String(s.key || s.label || ("s" + out.length)), points: pts });
    }
  }
  return out;
}

/**
 * Enrich samples/series with forecast overlays.
 * @returns {{samples, series, nowTs, meta, stale?}}
 */
async function enrichSamplesWithForecast(samples, seriesDefs, opts) {
  opts = opts || {};
  const base = { samples: samples || [], series: seriesDefs || [], nowTs: 0, meta: null };
  if (!opts.forecast || !base.samples.length || !base.series.length) return base;
  if (!_fcStillCurrent(opts)) return Object.assign(base, { stale: true });
  const reqSeries = buildForecastRequestSeries(base.samples, base.series);
  if (!reqSeries.length) {
    return Object.assign(base, { meta: { ok: false, message: "采样点不足，暂无法预测" } });
  }
  let res;
  try {
    const fetchOpts = {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        series: reqSeries,
        horizon_sec: opts.horizonSec || 0,
        step: opts.step || 0
      })
    };
    if (opts.signal) fetchOpts.signal = opts.signal;
    const r = await fetch(`${API}/metrics/forecast`, fetchOpts);
    if (!_fcStillCurrent(opts)) return Object.assign(base, { stale: true });
    res = await r.json();
  } catch (e) {
    if (e && (e.name === "AbortError" || (opts.signal && opts.signal.aborted))) {
      return Object.assign(base, { stale: true });
    }
    return Object.assign(base, { meta: { ok: false, message: String(e) } });
  }
  if (!_fcStillCurrent(opts)) return Object.assign(base, { stale: true });
  if (!res || !res.series) {
    return Object.assign(base, { meta: (res && res.meta) || { ok: false, message: "预测失败" } });
  }
  const histDefs = (seriesDefs || []).filter(s => !s.kind || s.kind === "history");
  const tsMap = new Map();
  for (const sm of base.samples) {
    const ts = Math.round(sm.timestamp != null ? sm.timestamp : sm.ts);
    if (!ts) continue;
    tsMap.set(ts, Object.assign({}, sm, { timestamp: ts }));
  }
  const outSeries = histDefs.map(s => Object.assign({}, s, { kind: s.kind || "history" }));
  let nowTs = (res.meta && (res.meta.now_ts || res.meta.NowTS)) || 0;
  for (const fs of res.series) {
    if (fs.kind !== "forecast") continue;
    const baseName = String(fs.name || "").replace(/\s*·\s*预测$/, "");
    const hist = histDefs.find(s => String(s.key) === baseName || String(s.label) === baseName)
      || histDefs.find(s => (fs.name || "").indexOf(String(s.label || "")) === 0);
    const color = (hist && hist.color) || "#4c8dff";
    const fmt = hist && hist.fmt;
    const fcKey = "fc_" + (hist && hist.key ? hist.key : baseName || outSeries.length);
    for (const pt of (fs.points || [])) {
      const ts = Math.round(+pt[0]);
      let row = tsMap.get(ts);
      if (!row) { row = { timestamp: ts }; tsMap.set(ts, row); }
      row[fcKey] = +pt[1];
    }
    if (!nowTs && (fs.points || []).length) {
      nowTs = Math.round(+fs.points[0][0]);
    }
    outSeries.push({
      key: fcKey,
      label: (hist && hist.label ? hist.label : baseName) + " · 预测",
      color, fmt, dashed: true, kind: "forecast"
    });
  }
  if (!nowTs && base.samples.length) {
    nowTs = Math.round(base.samples[base.samples.length - 1].timestamp || base.samples[base.samples.length - 1].ts || 0);
  }
  const merged = [...tsMap.values()].sort((a, b) => a.timestamp - b.timestamp);
  return { samples: merged, series: outSeries, nowTs, meta: res.meta || null };
}

/**
 * One shared forecast for many chart series defs (host detail).
 * Dedupes by series key, caps at FC_MAX_SERIES, returns merged samples + full fc series map.
 */
async function enrichSharedForecast(samples, allSeriesDefs, opts) {
  opts = opts || {};
  const seen = new Set();
  const uniq = [];
  for (const s of allSeriesDefs || []) {
    if (!s || (s.kind && s.kind !== "history")) continue;
    const k = String(s.key || s.label || "");
    if (!k || seen.has(k)) continue;
    seen.add(k);
    uniq.push(s);
  }
  return enrichSamplesWithForecast(samples, uniq, Object.assign({}, opts, { forecast: true }));
}

/** Pick history + matching fc_* series for one chart from a shared enrich result. */
function sliceForecastForChart(enriched, chartSeriesDefs) {
  if (!enriched || enriched.stale) return null;
  const histDefs = (chartSeriesDefs || []).filter(s => !s.kind || s.kind === "history");
  const keys = new Set(histDefs.map(s => String(s.key)));
  const outSeries = [];
  for (const s of histDefs) outSeries.push(Object.assign({}, s, { kind: "history" }));
  for (const s of (enriched.series || [])) {
    if (s.kind !== "forecast") continue;
    const baseKey = String(s.key || "").replace(/^fc_/, "");
    if (keys.has(baseKey)) outSeries.push(s);
  }
  return {
    samples: enriched.samples,
    series: outSeries,
    nowTs: enriched.nowTs || 0,
    meta: enriched.meta
  };
}

/** createChart + optional forecast enrichment — never paints if stale. */
async function createChartWithForecast(canvasId, samples, series, yMin, yMax, opts) {
  opts = opts || {};
  const want = !!opts.forecast || isChartForecastOn(opts.forecastScope || "");
  let sm = samples, ser = series, nowTs = opts.nowTs || 0, meta = null;
  if (want && !opts.preEnriched) {
    const en = await enrichSamplesWithForecast(samples, series, {
      forecast: true,
      horizonSec: opts.horizonSec || 0,
      step: opts.step || 0,
      signal: opts.signal,
      isCurrent: opts.isCurrent
    });
    if (en.stale || !_fcStillCurrent(opts)) return null;
    sm = en.samples; ser = en.series; nowTs = en.nowTs || nowTs; meta = en.meta;
  } else if (opts.preEnriched) {
    sm = opts.preEnriched.samples || samples;
    ser = opts.preEnriched.series || series;
    nowTs = opts.preEnriched.nowTs || nowTs;
    meta = opts.preEnriched.meta;
  }
  if (!_fcStillCurrent(opts)) return null;
  const state = createChart(canvasId, sm, ser, yMin, yMax, Object.assign({}, opts, { nowTs }));
  if (state) state._fcMeta = meta;
  return state;
}

/**
 * Mount many Canvas charts that share the same samples timeline.
 * When forecast is on: one shared POST /metrics/forecast, then slice per chart.
 * loadOpts: { signal, isCurrent } from beginRangeLoad — stale responses never paint.
 */
async function mountChartsWithForecast(scope, specs, loadOpts) {
  loadOpts = loadOpts || {};
  const want = isChartForecastOn(scope);
  const isCurrent = typeof loadOpts.isCurrent === "function" ? loadOpts.isCurrent : () => true;
  const signal = loadOpts.signal;
  const out = {};
  const list = (specs || []).filter(sp => sp && sp.id);
  if (!list.length) return out;

  if (!want) {
    for (const sp of list) {
      if (!isCurrent()) return out;
      out[sp.id] = createChart(sp.id, sp.samples, sp.series, sp.yMin, sp.yMax, Object.assign({}, sp.opts || {}, { forecastScope: scope }));
    }
    return out;
  }

  // Shared enrich using the first chart's samples (all multi-chart views share one timeline).
  const samples = list[0].samples || [];
  const allSeries = [];
  for (const sp of list) {
    if (sp.series) allSeries.push(...sp.series);
  }
  const en = await enrichSharedForecast(samples, allSeries, {
    forecast: true,
    signal,
    isCurrent,
    horizonSec: loadOpts.horizonSec || 0,
    step: loadOpts.step || 0
  });
  if (!isCurrent() || (en && en.stale)) return out;

  for (const sp of list) {
    if (!isCurrent()) return out;
    const sliced = sliceForecastForChart(en, sp.series);
    const baseOpts = Object.assign({}, sp.opts || {}, { forecastScope: scope });
    if (sliced) {
      out[sp.id] = createChart(sp.id, sliced.samples, sliced.series, sp.yMin, sp.yMax,
        Object.assign(baseOpts, { nowTs: sliced.nowTs || 0 }));
    } else {
      out[sp.id] = createChart(sp.id, sp.samples, sp.series, sp.yMin, sp.yMax, baseOpts);
    }
  }
  return out;
}

// Global toggle chip handler — views listen for "chart-forecast-toggle".
document.addEventListener("click", (e) => {
  const btn = e.target && e.target.closest && e.target.closest("[data-chart-forecast]");
  if (!btn) return;
  e.preventDefault();
  const scope = btn.getAttribute("data-chart-forecast") || "default";
  const on = !isChartForecastOn(scope);
  setChartForecastOn(scope, on);
  btn.classList.toggle("active", on);
  document.dispatchEvent(new CustomEvent("chart-forecast-toggle", { detail: { scope, on } }));
});
