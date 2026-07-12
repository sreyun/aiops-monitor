/* ============================================================
   AIOps Monitor · charts.js — Canvas 图表引擎
   依赖：core.js、render.js（需先加载）
   ============================================================ */
"use strict";

/* ---------- 主机趋势弹窗 ---------- */
let DETAIL_HOST_ID = '';
let DETAIL_TIME_RANGE = 24; // hours: 1, 24, 48, 168, 720
let DETAIL_CUSTOM = null;   // {from,to} unix seconds

function toLocalDatetimeValue(unixSec) {
  const d = new Date(unixSec * 1000);
  const p = n => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}

const CHART_SPANS = [
  [1, "time.1h"],
  [24, "time.24h"],
  [48, "48h"],
  [168, "7d"],
  [720, "time.30d"],
];
function renderChartControls(currentRange, prefix) {
  return CHART_SPANS.map(([h, key]) => {
    const lab = key === "48h" ? "48" + I18N.t("time.hour") : key === "7d" ? "7" + I18N.t("time.day") : I18N.t(key);
    return `<button class="chip-btn ${currentRange === h ? "active" : ""}" data-${prefix}="${h}">${lab}</button>`;
  }).join("");
}
async function openDetail(id, name) {
  DETAIL_HOST_ID = id;
  DETAIL_TIME_RANGE = 24;
  DETAIL_CUSTOM = null;
  $("detailTitle").textContent = name + " " + I18N.t("section.recent_trend");
  const body = $("detailBody");
  body.innerHTML = `<div class="empty-line">${I18N.t("ui.loading")}</div>`;
  $("detailMask").classList.add("show");
  await loadAndRenderCharts();
}

let DETAIL_CHARTS = {};

async function loadAndRenderCharts() {
  const body = $("detailBody");
  const now = Math.floor(Date.now() / 1000);
  const to = DETAIL_CUSTOM ? DETAIL_CUSTOM.to : now;
  const from = DETAIL_CUSTOM ? DETAIL_CUSTOM.from : now - DETAIL_TIME_RANGE * 3600;
  const spanH = Math.max(0, (to - from) / 3600);

  try {
    const samples = await fetch(`${API}/hosts/${encodeURIComponent(DETAIL_HOST_ID)}/history?from=${from}&to=${to}`).then(r => r.json());
    if (!Array.isArray(samples) || !samples.length) {
      body.innerHTML = `<div class="empty-line">${I18N.t("empty.no_history")}</div>`;
      return;
    }

    DETAIL_CHARTS = {};
    const gran = spanH <= 2 ? I18N.t("time.raw") : spanH <= 48 ? I18N.t("time.1m_agg") : I18N.t("time.5m_agg");
    const hasGPU = samples.some(s => Array.isArray(s.gpus) && s.gpus.length);
    const pct = v => v.toFixed(1) + '%';
    const wrap = id => `<div class="chart-wrap"><canvas id="${id}" width="1000" height="240"></canvas>` +
      `<button class="chart-enlarge" data-chart="${id}" title="${I18N.t('ui.zoom_preview')}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg></button></div>`;
    body.innerHTML = `
      <div class="chart-controls">
        ${renderChartControls(DETAIL_CUSTOM ? -1 : DETAIL_TIME_RANGE, "range")}
        <button class="chip-btn ${DETAIL_CUSTOM ? "active" : ""}" data-custom-toggle title="${I18N.t("time.custom_range") || "自定义时间范围"}">${I18N.t("time.custom") || "自定义"}</button>
        <span class="chart-custom-range" id="detailCustomPanel"${DETAIL_CUSTOM ? "" : " hidden"}>
          <input type="datetime-local" id="detailCustomFrom" class="dt-input" value="${toLocalDatetimeValue(from)}">
          <span class="dt-sep">→</span>
          <input type="datetime-local" id="detailCustomTo" class="dt-input" value="${toLocalDatetimeValue(to)}">
          <button class="chip-btn primary" data-custom-apply>${I18N.t("time.custom_apply") || "应用"}</button>
        </span>
      </div>
      <div class="chart-container">
        ${wrap('chartCPU')}${wrap('chartMem')}${wrap('chartLoad')}${wrap('chartDisk')}${hasGPU ? wrap('chartGPU') : ''}${wrap('chartNet')}${wrap('chartDiskIO')}${wrap('chartIOPS')}${wrap('chartProc')}
      </div>
      <div class="hint">${I18N.t("section.sample_points")}: ${samples.length} · ${I18N.t("section.granularity")}: ${gran}</div>
    `;

    DETAIL_CHARTS.chartCPU = createChart('chartCPU', samples,
      [{ key: 'cpu_percent', label: I18N.t("section.cpu_usage"), color: '#4c8dff', fmt: pct }], 0, 100, { title: I18N.t("section.cpu_usage") });
    DETAIL_CHARTS.chartMem = createChart('chartMem', samples,
      [{ key: 'mem_percent', label: I18N.t("section.mem_usage"), color: '#8b5cf6', fmt: pct }], 0, 100, { title: I18N.t("section.mem_usage") });
    DETAIL_CHARTS.chartLoad = createChart('chartLoad', samples, [
      { key: 'load1', label: I18N.t("section.load_1m_label"), color: '#4c8dff', fmt: v => v.toFixed(1) },
      { key: 'load5', label: I18N.t("section.load_5m_label"), color: '#f7b23b', fmt: v => v.toFixed(1) },
      { key: 'load15', label: I18N.t("section.load_15m_label"), color: '#f2545b', fmt: v => v.toFixed(1) },
    ], null, null, { title: I18N.t("section.load_avg") });

    let diskProto = [];
    samples.forEach(s => { if (Array.isArray(s.disks) && s.disks.length > diskProto.length) diskProto = s.disks; });
    const diskKeys = diskProto.map(d => d.path);
    const diskSeries = diskKeys.map((path, idx) => ({
      key: `disk_${idx}`, label: '磁盘 ' + path,
      color: ['#f7b23b', '#2fd07a', '#f2545b', '#43b6f0'][idx % 4], fmt: pct,
      transform: (s) => { const d = s.disks && s.disks[idx] ? s.disks[idx] : null; return d ? d.percent : null; }
    }));
    DETAIL_CHARTS.chartDisk = createChart('chartDisk', samples,
      diskSeries.length ? diskSeries : [{ key: 'disk_percent', label: I18N.t("section.root_partition"), color: '#f7b23b', fmt: pct }],
      0, 100, { title: I18N.t("section.disk_usage") });

    if (hasGPU) {
      const gpuNames = [];
      samples.forEach(s => (s.gpus || []).forEach((g, i) => { if (!gpuNames[i]) gpuNames[i] = g.name || ('GPU' + i); }));
      const gpuSeries = gpuNames.map((nm, idx) => ({
        key: `gpu_${idx}`, label: nm,
        color: ['#8b5cf6', '#43b6f0', '#2fd07a', '#f7b23b'][idx % 4], fmt: v => v.toFixed(0) + '%',
        transform: (s) => { const g = s.gpus && s.gpus[idx] ? s.gpus[idx] : null; return g ? (g.util_percent || 0) : null; }
      }));
      DETAIL_CHARTS.chartGPU = createChart('chartGPU', samples, gpuSeries, 0, 100, { title: I18N.t("section.gpu_usage") });
    }

    DETAIL_CHARTS.chartNet = createChart('chartNet', samples, [
      { key: 'net_recv_rate', label: I18N.t("section.net_recv"), color: '#2fd07a', fmt: fmtRate },
      { key: 'net_sent_rate', label: I18N.t("section.net_send"), color: '#43b6f0', fmt: fmtRate },
    ], null, null, { title: I18N.t("section.net_throughput") });

    DETAIL_CHARTS.chartDiskIO = createChart('chartDiskIO', samples, [
      { key: 'disk_read_rate', label: I18N.t("ui.disk_read"), color: '#2fd07a', fmt: fmtIORate },
      { key: 'disk_write_rate', label: I18N.t("ui.disk_write"), color: '#f7b23b', fmt: fmtIORate },
    ], null, null, { title: I18N.t("ui.disk_io") });

    DETAIL_CHARTS.chartIOPS = createChart('chartIOPS', samples, [
      { key: 'disk_read_iops', label: I18N.t("ui.disk_read_iops"), color: '#2fd07a', fmt: fmtIOPS },
      { key: 'disk_write_iops', label: I18N.t("ui.disk_write_iops"), color: '#f7b23b', fmt: fmtIOPS },
    ], null, null, { title: I18N.t("ui.disk_iops_title") });

    DETAIL_CHARTS.chartProc = createChart('chartProc', samples, [
      { key: 'proc_count', label: '进程数', color: '#8b5cf6', fmt: v => v.toFixed(0) },
    ], null, null, { title: '进程数趋势' });

  } catch (e) {
    body.innerHTML = `<div class="empty-line">加载失败: ${esc(e)}</div>`;
  }
}

function applyDetailCustomRange() {
  const fEl = $("detailCustomFrom"), tEl = $("detailCustomTo");
  if (!fEl || !tEl || !fEl.value || !tEl.value) { toast(I18N.t("time.custom_incomplete") || "请选择开始和结束时间", "warn"); return; }
  const from = Math.floor(new Date(fEl.value).getTime() / 1000);
  const to = Math.floor(new Date(tEl.value).getTime() / 1000);
  if (!Number.isFinite(from) || !Number.isFinite(to)) { toast(I18N.t("time.custom_invalid") || "时间格式无效", "err"); return; }
  if (to <= from) { toast(I18N.t("time.custom_order") || "结束时间必须晚于开始时间", "warn"); return; }
  if (to - from < 60) { toast(I18N.t("time.custom_tooshort") || "时间范围太短（至少 1 分钟）", "warn"); return; }
  DETAIL_CUSTOM = { from, to };
  loadAndRenderCharts();
}

// 详情弹窗事件委托
safeAddEventListener("detailBody", "click", e => {
  const en = e.target.closest(".chart-enlarge");
  if (en) { const ch = DETAIL_CHARTS[en.dataset.chart]; if (ch) openChartZoom(ch); return; }
  const tog = e.target.closest("[data-custom-toggle]");
  if (tog) {
    const panel = $("detailCustomPanel");
    if (panel) { panel.hidden = !panel.hidden; if (!panel.hidden) { const f = $("detailCustomFrom"); if (f) f.focus(); } }
    return;
  }
  if (e.target.closest("[data-custom-apply]")) { applyDetailCustomRange(); return; }
  const btn = e.target.closest(".chip-btn[data-range]");
  if (!btn) return;
  DETAIL_CUSTOM = null;
  DETAIL_TIME_RANGE = parseInt(btn.dataset.range);
  loadAndRenderCharts();
});

/* ---------- Canvas 折线图引擎 ---------- */
function chartTipEl() {
  let t = $("chartTip");
  if (!t) { t = document.createElement("div"); t.id = "chartTip"; t.className = "chart-tip"; document.body.appendChild(t); }
  return t;
}
function hideChartTip() { const t = $("chartTip"); if (t) t.style.display = "none"; }

function seriesVal(s, sample) {
  const v = s.transform ? s.transform(sample) : sample[s.key];
  return (v === null || v === undefined || isNaN(v)) ? null : v;
}

function smoothPath(ctx, pts) {
  if (pts.length < 2) return;
  ctx.beginPath();
  ctx.moveTo(pts[0].x, pts[0].y);
  for (let i = 1; i < pts.length - 1; i++) {
    const cx = (pts[i].x + pts[i + 1].x) / 2;
    const cy = (pts[i].y + pts[i + 1].y) / 2;
    ctx.quadraticCurveTo(pts[i].x, pts[i].y, cx, cy);
  }
  ctx.lineTo(pts[pts.length - 1].x, pts[pts.length - 1].y);
}

function drawChartEmpty(ctx, w, h, message) {
  ctx.clearRect(0, 0, w, h);
  const cssVar = name => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  const txtColor = cssVar("--muted") || "#8a95a8";
  const lineColor = cssVar("--line2") || "#2c3442";
  const cx = w / 2, cy = h / 2;
  ctx.strokeStyle = lineColor; ctx.lineWidth = 1.2; ctx.setLineDash([3, 4]); ctx.lineCap = "round";
  const iconPts = [{x: cx - 50, y: cy + 10}, {x: cx - 18, y: cy - 14}, {x: cx + 14, y: cy + 6}, {x: cx + 46, y: cy - 20}];
  ctx.beginPath(); ctx.moveTo(iconPts[0].x, iconPts[0].y);
  for (let i = 1; i < iconPts.length; i++) ctx.lineTo(iconPts[i].x, iconPts[i].y);
  ctx.stroke(); ctx.setLineDash([]);
  iconPts.forEach(p => { ctx.fillStyle = lineColor; ctx.beginPath(); ctx.arc(p.x, p.y, 2.5, 0, Math.PI * 2); ctx.fill(); });
  ctx.fillStyle = txtColor; ctx.font = "13px -apple-system, 'Segoe UI', 'PingFang SC', sans-serif"; ctx.textAlign = "center";
  ctx.fillText(message, cx, cy + 40);
}

function sizeChartCanvas(canvas, cssH) {
  const dpr = Math.min(window.devicePixelRatio || 1, 2);
  const cssW = Math.round(canvas.getBoundingClientRect().width) || 1000;
  canvas.style.height = cssH + "px";
  canvas.width = Math.max(1, Math.round(cssW * dpr));
  canvas.height = Math.max(1, Math.round(cssH * dpr));
  canvas.getContext("2d").setTransform(dpr, 0, 0, dpr, 0, 0);
  return { W: cssW, H: cssH, dpr };
}

function resizeAllCharts() {
  const states = [];
  for (const k in DETAIL_CHARTS) if (DETAIL_CHARTS[k]) states.push(DETAIL_CHARTS[k]);
  for (const k in (typeof CHK_CHARTS !== "undefined" ? CHK_CHARTS : {})) if (CHK_CHARTS[k]) states.push(CHK_CHARTS[k]);
  states.forEach(st => {
    if (!st.canvas || !st.canvas.isConnected) return;
    const d = sizeChartCanvas(st.canvas, st.cssH || 210);
    st.W = d.W; st.H = d.H; st.dpr = d.dpr;
    drawChart(st);
  });
}
let _chartResizeTimer = null;
window.addEventListener("resize", () => {
  clearTimeout(_chartResizeTimer);
  _chartResizeTimer = setTimeout(resizeAllCharts, 150);
});

function createChart(canvasId, allSamples, series, yMin = null, yMax = null, opts = {}) {
  const canvas = $(canvasId);
  if (!canvas) return null;
  const cssH = opts.isZoom ? 440 : 210;
  const dim = sizeChartCanvas(canvas, cssH);
  if (!allSamples || !allSamples.length) {
    drawChartEmpty(canvas.getContext("2d"), dim.W, dim.H, I18N.t("empty.no_trend_data") || "暂无趋势数据");
    return null;
  }
  const state = {
    canvas, ctx: canvas.getContext("2d"),
    W: dim.W, H: dim.H, dpr: dim.dpr, cssH,
    all: allSamples, series, yMin, yMax,
    title: opts.title || "", isZoom: !!opts.isZoom,
    i0: 0, i1: allSamples.length - 1,
    hover: -1, drag: false, downX: null, curX: null, moved: false,
    pad: { top: 22, right: 18, bottom: 28, left: 56 },
  };
  canvas._chart = state;
  drawChart(state);
  state._entranceStart = performance.now();
  state._entranceDur = 400;
  requestAnimationFrame(function entranceStep(now) {
    state._entranceP = Math.min(1, (now - state._entranceStart) / state._entranceDur);
    drawChart(state);
    if (state._entranceP < 1) requestAnimationFrame(entranceStep);
  });
  attachChartEvents(canvas);
  return state;
}

function drawChart(state) {
  const { ctx, canvas, series, pad } = state;
  ctx.setTransform(state.dpr || 1, 0, 0, state.dpr || 1, 0, 0);
  const w = state.W || canvas.width, h = state.H || canvas.height;
  const vis = state.all.slice(state.i0, state.i1 + 1);
  const n = vis.length;
  ctx.clearRect(0, 0, w, h);
  const cssVar = name => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  const gridColor = cssVar("--line2") || "rgba(43,53,71,.5)";
  const labelColor = cssVar("--muted") || "#8a95a8";
  const txtColor = cssVar("--txt") || "#e8eef6";
  let dMin = state.yMin !== null ? state.yMin : Infinity;
  let dMax = state.yMax !== null ? state.yMax : -Infinity;
  series.forEach(s => vis.forEach(sm => {
    const v = seriesVal(s, sm);
    if (v !== null) { dMin = Math.min(dMin, v); dMax = Math.max(dMax, v); }
  }));
  if (dMin === Infinity) dMin = 0;
  if (dMax === -Infinity) dMax = state.yMax !== null ? state.yMax : 100;
  if (state.yMin === null) dMin = Math.max(0, dMin * 0.92);
  if (state.yMax === null) dMax = dMax * 1.08 || 1;
  if (dMax <= dMin) dMax = dMin + 1;
  const yRange = dMax - dMin;
  ctx.font = "10.5px 'SF Mono', 'Cascadia Code', 'JetBrains Mono', Consolas, monospace";
  let maxLabelW = 0;
  for (let i = 0; i <= 4; i++) {
    const val = dMax - (yRange / 4) * i;
    const lab = series[0].fmt ? series[0].fmt(val) : val.toFixed(1);
    maxLabelW = Math.max(maxLabelW, ctx.measureText(lab).width);
  }
  pad.left = Math.max(56, Math.ceil(maxLabelW) + 14);
  const cw = w - pad.left - pad.right, ch = h - pad.top - pad.bottom;
  state.dataMin = dMin; state.dataMax = dMax; state._cw = cw; state._ch = ch; state._n = n;
  const xAt = i => pad.left + (n <= 1 ? 0 : (i / (n - 1)) * cw);
  const yAt = v => pad.top + ch - ((v - dMin) / yRange) * ch;
  ctx.strokeStyle = gridColor; ctx.lineWidth = 0.5; ctx.setLineDash([2, 4]);
  ctx.font = "10.5px 'SF Mono', 'Cascadia Code', 'JetBrains Mono', Consolas, monospace"; ctx.textAlign = "right";
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (ch / 4) * i;
    ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(w - pad.right, y); ctx.stroke();
    const val = dMax - (yRange / 4) * i;
    ctx.fillStyle = labelColor;
    const fmt = series[0].fmt;
    const label = fmt ? fmt(val) : val.toFixed(1);
    ctx.fillText(label, pad.left - 8, y + 4);
  }
  ctx.setLineDash([]);
  if (n >= 1) {
    const firstTs = vis[0].timestamp, span = vis[n - 1].timestamp - firstTs;
    ctx.textAlign = "center"; ctx.fillStyle = labelColor; ctx.font = "10.5px 'SF Mono', 'Cascadia Code', 'JetBrains Mono', Consolas, monospace";
    for (let i = 0; i <= 4; i++) {
      const x = pad.left + (cw / 4) * i;
      const d = new Date((firstTs + (span / 4) * i) * 1000);
      const lab = span > 172800
        ? `${d.getMonth() + 1}/${d.getDate()}`
        : `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
      ctx.fillText(lab, x, h - 8);
    }
  }
  series.forEach((s, sIdx) => {
    const pts = [];
    vis.forEach((sm, i) => { const v = seriesVal(s, sm); if (v !== null) pts.push({ x: xAt(i), y: yAt(v), val: v }); });
    if (pts.length >= 2) {
      ctx.save();
      ctx.strokeStyle = s.color; ctx.lineWidth = sIdx === 0 ? 2.2 : 1.8; ctx.lineJoin = "round"; ctx.lineCap = "round";
      if (pts.length > 12) { smoothPath(ctx, pts); } else { ctx.beginPath(); pts.forEach((p, i) => i ? ctx.lineTo(p.x, p.y) : ctx.moveTo(p.x, p.y)); }
      ctx.stroke();
      ctx.restore();
      const grad = ctx.createLinearGradient(0, pad.top, 0, pad.top + ch);
      grad.addColorStop(0, s.color + "35");
      grad.addColorStop(0.4, s.color + "15");
      grad.addColorStop(0.7, s.color + "06");
      grad.addColorStop(1, s.color + "01");
      ctx.fillStyle = grad;
      ctx.beginPath(); ctx.moveTo(pts[0].x, pad.top + ch);
      pts.forEach(p => ctx.lineTo(p.x, p.y));
      ctx.lineTo(pts[pts.length - 1].x, pad.top + ch); ctx.closePath(); ctx.fill();
    }
  });
  const legendY = pad.top + 4;
  let legendX = pad.left + 8;
  const legendItemWidth = 160;
  let legendBgW = 0, legendBgX0 = legendX;
  const legendLines = [];
  let curLine = { x: legendX, items: [] };
  series.forEach((s, sIdx) => {
    const pts = [];
    vis.forEach((sm, i) => { const v = seriesVal(s, sm); if (v !== null) pts.push({ x: xAt(i), y: yAt(v), val: v }); });
    const vals = pts.map(p => p.val);
    const cur = vals.length ? vals[vals.length - 1] : 0, peak = vals.length ? Math.max(...vals) : 0;
    const fmtV = v => s.fmt ? s.fmt(v) : v.toFixed(1);
    const labelText = `${s.label}  当前 ${fmtV(cur)} · 峰值 ${fmtV(peak)}`;
    if (curLine.x + legendItemWidth > w - pad.right && sIdx > 0) {
      legendLines.push(curLine);
      curLine = { x: pad.left + 8, items: [] };
    }
    curLine.items.push({ color: s.color, labelText, x: curLine.x });
    ctx.font = "10.5px -apple-system, 'Segoe UI', 'PingFang SC', sans-serif";
    curLine.x += ctx.measureText(labelText).width + 28;
    if (curLine.x + legendItemWidth > w - pad.right) {
      legendLines.push(curLine);
      curLine = { x: pad.left + 8, items: [] };
    }
  });
  if (curLine.items.length) legendLines.push(curLine);
  legendLines.forEach(line => {
    legendBgW = Math.max(legendBgW, line.x - legendBgX0);
  });
  if (legendLines.length) {
    const bgH = legendLines.length * 18 + 8;
    const panelColor = cssVar("--panel") || "rgba(17,22,33,.6)";
    ctx.fillStyle = panelColor + "99";
    const bgR = 6;
    ctx.beginPath(); ctx.roundRect(legendBgX0 - 4, legendY - 2, legendBgW + 20, bgH, bgR); ctx.fill();
  }
  let ly = legendY;
  legendLines.forEach(line => {
    let lx = line.items.length ? line.items[0].x : legendBgX0;
    line.items.forEach(item => {
      lx = item.x;
      ctx.fillStyle = item.color;
      ctx.beginPath(); ctx.roundRect(lx, ly, 10, 10, 3); ctx.fill();
      ctx.fillStyle = txtColor; ctx.font = "10.5px -apple-system, 'Segoe UI', 'PingFang SC', sans-serif"; ctx.textAlign = "left";
      ctx.fillText(item.labelText, lx + 14, ly + 9);
    });
    ly += 18;
  });
  if (state.drag && state.moved && state.downX !== null && state.curX !== null) {
    const x0 = Math.min(state.downX, state.curX), x1 = Math.max(state.downX, state.curX);
    ctx.fillStyle = "rgba(76,141,255,.12)"; ctx.fillRect(x0, pad.top, x1 - x0, ch);
    ctx.strokeStyle = "rgba(76,141,255,.5)"; ctx.lineWidth = 1; ctx.setLineDash([4, 4]); ctx.strokeRect(x0, pad.top, x1 - x0, ch); ctx.setLineDash([]);
  }
  if (state.hover >= state.i0 && state.hover <= state.i1 && !state.drag) {
    const li = state.hover - state.i0, x = xAt(li);
    ctx.strokeStyle = "rgba(200,210,230,.22)"; ctx.lineWidth = 0.8;
    ctx.setLineDash([3, 5]); ctx.beginPath(); ctx.moveTo(x, pad.top); ctx.lineTo(x, pad.top + ch); ctx.stroke(); ctx.setLineDash([]);
    series.forEach(s => {
      const v = seriesVal(s, vis[li]); if (v === null) return;
      const py = yAt(v);
      ctx.fillStyle = s.color + "25"; ctx.beginPath(); ctx.arc(x, py, 8, 0, Math.PI * 2); ctx.fill();
      ctx.fillStyle = s.color; ctx.beginPath(); ctx.arc(x, py, 3.5, 0, Math.PI * 2); ctx.fill();
      ctx.strokeStyle = "#fff"; ctx.lineWidth = 1.5;
      ctx.beginPath(); ctx.arc(x, py, 3.5, 0, Math.PI * 2); ctx.stroke();
    });
  }
}

function attachChartEvents(canvas) {
  if (canvas._evt) return;
  canvas._evt = true;
  const toX = e => {
    const st = canvas._chart;
    const r = canvas.getBoundingClientRect();
    if (!r.width) return 0;
    const W = (st && st.W) || r.width;
    return (e.clientX - r.left) * (W / r.width);
  };
  const localIdx = (st, x) => {
    const n = st._n; if (n <= 1) return 0;
    return Math.max(0, Math.min(n - 1, Math.round((x - st.pad.left) / st._cw * (n - 1))));
  };
  canvas.addEventListener("mousemove", e => {
    const st = canvas._chart; if (!st) return;
    const x = toX(e);
    if (st.drag) { st.curX = x; if (Math.abs(x - st.downX) > 4) st.moved = true; }
    const li = localIdx(st, x); st.hover = st.i0 + li;
    drawChart(st); showChartTip(st, e, li);
  });
  canvas.addEventListener("mousedown", e => { const st = canvas._chart; if (!st) return; st.drag = true; st.downX = toX(e); st.curX = st.downX; st.moved = false; });
  canvas.addEventListener("mouseup", e => {
    const st = canvas._chart; if (!st) return;
    if (st.drag && st.moved) {
      const a = localIdx(st, st.downX), b = localIdx(st, toX(e));
      const lo = Math.min(a, b), hi = Math.max(a, b);
      if (hi - lo >= 1) { const base = st.i0; st.i1 = base + hi; st.i0 = base + lo; }
    } else if (st.drag && !st.moved && !st.isZoom) { openChartZoom(st); }
    st.drag = false; st.downX = st.curX = null; st.moved = false; drawChart(st);
  });
  canvas.addEventListener("mouseleave", () => { const st = canvas._chart; if (!st) return; st.hover = -1; st.drag = false; st.moved = false; hideChartTip(); drawChart(st); });
  canvas.addEventListener("dblclick", () => { const st = canvas._chart; if (!st) return; st.i0 = 0; st.i1 = st.all.length - 1; st.hover = -1; hideChartTip(); drawChart(st); });
}

function showChartTip(state, e, li) {
  const vis = state.all.slice(state.i0, state.i1 + 1);
  const sm = vis[li]; if (!sm) { hideChartTip(); return; }
  const d = new Date(sm.timestamp * 1000);
  const time = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")} ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
  let rows = "";
  state.series.forEach(s => {
    const v = seriesVal(s, sm);
    const txt = v === null ? "—" : (s.fmt ? s.fmt(v) : v.toFixed(1));
    rows += `<div class="tip-r"><span class="tip-dot" style="background:${s.color}"></span><span>${esc(s.label)}</span><span class="tip-v">${esc(txt)}</span></div>`;
  });
  const t = chartTipEl();
  t.innerHTML = `<div class="tip-t">${time}</div>${rows}`;
  t.style.display = "block";
  let px = e.clientX + 14, py = e.clientY + 14;
  if (px + t.offsetWidth > window.innerWidth - 8) px = e.clientX - t.offsetWidth - 14;
  if (py + t.offsetHeight > window.innerHeight - 8) py = e.clientY - t.offsetHeight - 14;
  t.style.left = px + "px"; t.style.top = py + "px";
}

function openChartZoom(src) {
  hideChartTip();
  $("chartZoomTitle").textContent = (src.title || I18N.t("ui.trend")) + " · " + I18N.t("ui.zoom_preview");
  $("chartZoomMask").classList.add("show");
  const z = createChart("chartZoomCanvas", src.all, src.series, src.yMin, src.yMax, { title: src.title, isZoom: true });
  if (z) { z.i0 = src.i0; z.i1 = src.i1; drawChart(z); }
  DETAIL_CHARTS.__zoom = z;
}
function sparkBlock(title, series, color) {
  const last = series.length ? series[series.length - 1] : 0;
  return `<div class="field"><label>${title} · 当前 ${(last || 0).toFixed(1)}</label>
    <div class="spark">${sparkline(series, color)}</div></div>`;
}
function sparkline(series, color) {
  const w = 500, h = 46, n = series.length, max = 100;
  if (n < 2) return `<svg class="sparkline" viewBox="0 0 ${w} ${h}"></svg>`;
  const pts = series.map((v, i) => {
    const x = i / (n - 1) * w;
    const y = h - 2 - (Math.max(0, Math.min(v || 0, max)) / max) * (h - 4);
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  }).join(" ");
  const gid = "g" + Math.random().toString(36).slice(2, 7);
  return `<svg class="sparkline" viewBox="0 0 ${w} ${h}" preserveAspectRatio="none">
    <defs><linearGradient id="${gid}" x1="0" x2="0" y1="0" y2="1">
      <stop offset="0" stop-color="${color}" stop-opacity=".35"/><stop offset="1" stop-color="${color}" stop-opacity="0"/>
    </linearGradient></defs>
    <polygon points="0,${h} ${pts} ${w},${h}" fill="url(#${gid})"/>
    <polyline points="${pts}" fill="none" stroke="${color}" stroke-width="1.6"/></svg>`;
}

/* ---------- 自定义监控·历史曲线 ---------- */
let CHK_CHARTS = {};
let CHK_HIST = { id: "", name: "", type: "", range: 24 };

function openCheckHistory(id, name, type) {
  CHK_HIST = { id, name, type, range: 24 };
  $("checkHistTitle").textContent = name + " · 监控历史";
  $("checkHistMask").classList.add("show");
  loadCheckHistory();
}
async function loadCheckHistory() {
  const { id, name, type, range } = CHK_HIST;
  const body = $("checkHistBody");
  body.innerHTML = `<div class="empty-line">加载中…</div>`;
  const ctrl = renderChartControls(range, "crange");
  try {
    const all = await fetch(`${API}/checks/${encodeURIComponent(id)}/history`).then(r => r.json());
    const now = Math.floor(Date.now() / 1000);
    const from = range > 0 ? now - range * 3600 : 0;
    const pts = (Array.isArray(all) ? all : []).filter(p => p.timestamp >= from);
    if (!pts.length) {
      body.innerHTML = `<div class="chart-controls">${ctrl}</div><div class="empty-line">该时间范围暂无数据（检查运行一段时间后自动积累，重启后重新计）</div>`;
      return;
    }
    const samples = pts.map(p => ({ timestamp: p.timestamp, latency_ms: p.latency_ms, loss_pct: (typeof p.loss_pct === "number" ? p.loss_pct : null), ok: p.ok }));
    const isPing = type === "ping";
    const uptime = (pts.filter(p => p.ok).length / pts.length * 100).toFixed(1);
    const avgLat = (pts.reduce((s, p) => s + (p.latency_ms || 0), 0) / pts.length).toFixed(0);
    const span = pts.length > 1 ? fmtDur(pts[pts.length - 1].timestamp - pts[0].timestamp) : I18N.t("time.just_now");
    const wrap = cid => `<div class="chart-wrap"><canvas id="${cid}" width="1000" height="240"></canvas>` +
      `<button class="chart-enlarge" data-chart="${cid}" title="${I18N.t('ui.zoom_preview')}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg></button></div>`;
    body.innerHTML = `<div class="chart-controls">${ctrl}</div>
      <div class="chart-container">${wrap("chkLat")}${isPing ? wrap("chkLoss") : ""}</div>
      <div class="hint">采样 ${pts.length} 个 · 时间跨度 ${span} · 可用率 ${uptime}% · 平均延时 ${avgLat} ${I18N.t("unit.ms")} · 悬停查看数值，拖动框选放大，双击还原。</div>`;
    CHK_CHARTS = {};
    CHK_CHARTS.chkLat = createChart("chkLat", samples, [
      { key: "latency_ms", label: isPing ? I18N.t("form.avg_latency") : I18N.t("form.latency"), color: "#4c8dff", fmt: v => v.toFixed(0) + " " + I18N.t("unit.ms") },
    ], 0, null, { title: name + " · " + I18N.t("form.latency") + "(" + I18N.t("unit.ms") + ")" });
    if (isPing) {
      CHK_CHARTS.chkLoss = createChart("chkLoss", samples, [
        { key: "loss_pct", label: I18N.t("form.loss_rate"), color: "#f2545b", fmt: v => v.toFixed(0) + "%" },
      ], 0, 100, { title: name + " · 丢包率(%)" });
    }
  } catch (e) {
    body.innerHTML = `<div class="empty-line">加载失败: ${esc(e)}</div>`;
  }
}
safeAddEventListener("checkHistBody", "click", e => {
  const rb = e.target.closest(".chip-btn[data-crange]");
  if (rb) { CHK_HIST.range = parseInt(rb.dataset.crange); loadCheckHistory(); return; }
  const en = e.target.closest(".chart-enlarge"); if (!en) return;
  const ch = CHK_CHARTS[en.dataset.chart]; if (ch) openChartZoom(ch);
});

// 导出到 AIOps 命名空间
Object.assign(window.AIOps, {
  DETAIL_CHARTS, DETAIL_HOST_ID, DETAIL_TIME_RANGE, DETAIL_CUSTOM,
  CHART_SPANS, toLocalDatetimeValue, renderChartControls,
  openDetail, loadAndRenderCharts, applyDetailCustomRange,
  chartTipEl, hideChartTip, seriesVal, smoothPath, drawChartEmpty,
  sizeChartCanvas, resizeAllCharts, createChart, drawChart, attachChartEvents,
  showChartTip, openChartZoom, sparkBlock, sparkline,
  CHK_CHARTS, CHK_HIST, openCheckHistory, loadCheckHistory
});