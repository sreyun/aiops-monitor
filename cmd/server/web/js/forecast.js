/* ============================================================================
 * Shared multi-series forecast helper for Canvas charts (hosts / AI cost / SNMP / …).
 * Left = realtime history, right = future forecast (dashed). Uses POST /metrics/forecast.
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
 * @returns {{samples, series, nowTs, meta}}
 */
async function enrichSamplesWithForecast(samples, seriesDefs, opts) {
  opts = opts || {};
  const base = { samples: samples || [], series: seriesDefs || [], nowTs: 0, meta: null };
  if (!opts.forecast || !base.samples.length || !base.series.length) return base;
  const reqSeries = buildForecastRequestSeries(base.samples, base.series);
  if (!reqSeries.length) {
    return Object.assign(base, { meta: { ok: false, message: "采样点不足，暂无法预测" } });
  }
  let res;
  try {
    res = await fetch(`${API}/metrics/forecast`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        series: reqSeries,
        horizon_sec: opts.horizonSec || 0,
        step: opts.step || 0
      })
    }).then(r => r.json());
  } catch (e) {
    return Object.assign(base, { meta: { ok: false, message: String(e) } });
  }
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
  let nowTs = (res.meta && res.meta.now_ts) || 0;
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
      if (ts > nowTs && hist) { /* keep now from meta */ }
    }
    if (!nowTs && (fs.points || []).length) {
      // first forecast point after bridge ≈ now
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

/** createChart + optional forecast enrichment */
async function createChartWithForecast(canvasId, samples, series, yMin, yMax, opts) {
  opts = opts || {};
  const want = !!opts.forecast || isChartForecastOn(opts.forecastScope || "");
  let sm = samples, ser = series, nowTs = opts.nowTs || 0, meta = null;
  if (want) {
    const en = await enrichSamplesWithForecast(samples, series, {
      forecast: true, horizonSec: opts.horizonSec || 0, step: opts.step || 0
    });
    sm = en.samples; ser = en.series; nowTs = en.nowTs || nowTs; meta = en.meta;
  }
  const state = createChart(canvasId, sm, ser, yMin, yMax, Object.assign({}, opts, { nowTs }));
  if (state) state._fcMeta = meta;
  return state;
}

/**
 * Mount many Canvas charts; when scope forecast is on, each gets multi-series forecast.
 * @param {string} scope
 * @param {Array<{id,samples,series,yMin?,yMax?,opts?}>} specs
 * @returns {Object<id, chartState>}
 */
async function mountChartsWithForecast(scope, specs) {
  const want = isChartForecastOn(scope);
  const out = {};
  for (const sp of specs || []) {
    if (!sp || !sp.id) continue;
    const opts = Object.assign({}, sp.opts || {}, { forecast: want, forecastScope: scope });
    if (want) {
      out[sp.id] = await createChartWithForecast(sp.id, sp.samples, sp.series, sp.yMin, sp.yMax, opts);
    } else {
      out[sp.id] = createChart(sp.id, sp.samples, sp.series, sp.yMin, sp.yMax, opts);
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
