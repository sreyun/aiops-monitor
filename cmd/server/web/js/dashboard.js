/* ========== 仪表盘（自定义 + 导入 Grafana，面板走 VictoriaMetrics） ==========
 * 列表 / 详情渲染 / 面板查询与绘制（时序/数值/仪表/条形/表格/文本/占位）/ 时间范围 /
 * 模板变量 / 尺寸编辑 / Grafana 导入。网格按 24 栏 gridPos 忠实还原，编辑用宽度栏数+高度+排序。
 */
const DASH_COLORS = ["#4c8dff", "#22c55e", "#f59e0b", "#ef4d5a", "#a855f7", "#06b6d4", "#eab308", "#ec4899", "#14b8a6", "#f97316"];
let DASH_LIST = [];
let CUR_DASH = null;               // 当前打开的完整仪表盘
let DASH_EDIT = false;             // 编辑模式
let DASH_RANGE = { hours: 1, custom: null };
let DASH_VARVALS = {};             // 变量名 → 选中值
let DASH_VAR_OPTIONS = {};         // 变量名 → 候选值列表
let DASH_CHART_ARGS = {};          // panelId → createChart 参数（供 resize 重绘）
let PANEL_TARGETS_DRAFT = [];      // 面板编辑中的查询行
let VARS_DRAFT = [];               // 变量编辑中的行
let DASH_DATASOURCES = [];         // 已配置的外部数据源（Prometheus / Loki）

// 数据源解析：面板级覆盖 > 看板级默认 > 内置 VM（""）
function resolveDS(panel) { return (panel && panel.datasource) || (CUR_DASH && CUR_DASH.datasource) || ""; }
function dsById(id) { return DASH_DATASOURCES.find(d => d.id === id); }
function dsLabel(id) { if (!id || id === "vm") return "内置 VM"; const d = dsById(id); return d ? d.name : id; }
// 生成数据源下拉 options（kinds: 指标面板=["prometheus"]含内置VM；日志面板=["loki"]）
function dsOptions(selected, kinds, withVM) {
  let html = withVM ? `<option value="" ${!selected || selected === "vm" ? "selected" : ""}>内置 VM（VictoriaMetrics）</option>` : "";
  DASH_DATASOURCES.filter(d => kinds.includes(d.type) && d.enabled !== false).forEach(d => {
    html += `<option value="${esc(d.id)}" ${d.id === selected ? "selected" : ""}>${esc(d.name)} · ${d.type}</option>`;
  });
  return html;
}
async function loadDashDatasources() {
  try { const r = await fetch(`${API}/datasources`).then(r => r.json()); DASH_DATASOURCES = Array.isArray(r) ? r : []; }
  catch (e) { DASH_DATASOURCES = []; }
}

/* ---------- 列表 ---------- */
async function loadDashboards() {
  showDashHome();
  await loadDashDatasources();
  try {
    const d = await fetch(`${API}/dashboards`).then(r => r.json());
    DASH_LIST = (d && d.dashboards) || [];
    renderDashList(DASH_LIST);
  } catch (e) { /* ignore */ }
}
function showDashHome() {
  const h = $("dashHome"), d = $("dashDetail");
  if (h) h.style.display = "";
  if (d) { d.style.display = "none"; d.innerHTML = ""; }
  CUR_DASH = null; DASH_EDIT = false; DASH_CHART_ARGS = {};
}
function renderDashList(list) {
  const wrap = $("dashList");
  if (!wrap) return;
  if (!list.length) {
    wrap.innerHTML = `<div class="empty-box">还没有仪表盘。点右上角「✨ AI 生成」用一句话生成，「导入 Grafana」按看板 ID 一键拉取（如 1860 Node Exporter Full），或「新建仪表盘」自定义面板 —— 面板查询直接走所选数据源。</div>`;
    return;
  }
  wrap.innerHTML = `<div class="dash-cards">` + list.map(d => {
    const isG = d.source && d.source.indexOf("grafana:") === 0;
    const isAI = d.source === "ai" || (d.source || "").indexOf("ai-analysis") === 0;
    return `<div class="dash-card" data-dash="${esc(d.id)}">
      <div class="dash-card-hd">
        <span class="dash-card-ic ${isAI ? "ai" : isG ? "gf" : ""}">${isAI ? "✨" : "▦"}</span>
        <div class="dash-card-name" title="${esc(d.name)}">${esc(d.name)}</div>
        <div class="dash-card-ops">
          <button class="mini-btn" data-dact="meta" data-id="${esc(d.id)}" title="编辑信息">✎</button>
          <button class="mini-btn del" data-dact="del" data-id="${esc(d.id)}" title="删除">✕</button>
        </div>
      </div>
      <div class="dash-card-desc ${d.description ? "" : "muted"}">${d.description ? esc(d.description) : "暂无描述"}</div>
      <div class="dash-card-ft">
        <span class="dash-card-badge">${d.panels} 面板</span>
        ${isAI ? '<span class="dash-card-badge ai">AI</span>' : isG ? '<span class="dash-card-badge gf">Grafana</span>' : ""}
        ${(d.tags || []).slice(0, 3).map(t => `<span class="dash-card-tag">${esc(t)}</span>`).join("")}
      </div>
    </div>`;
  }).join("") + `</div>`;
}

/* ---------- 打开 / 详情渲染 ---------- */
async function openDashboard(id) {
  try {
    CUR_DASH = await fetch(`${API}/dashboards/${encodeURIComponent(id)}`).then(r => r.json());
  } catch (e) { toast("加载失败：" + e, "err"); return; }
  if (!CUR_DASH || !CUR_DASH.id) { toast("仪表盘不存在", "err"); return; }
  DASH_EDIT = false;
  $("dashHome").style.display = "none";
  $("dashDetail").style.display = "";
  // 直接打开（AI 生成 / 消息中心 / 事件跳转）时也要确保数据源已加载，否则「数据源」下拉只有内置 VM，无法选择外部源。
  if (!DASH_DATASOURCES.length) await loadDashDatasources();
  await resolveDashVars();
  renderDashDetail();
}
// 解析模板变量候选值 + 默认选中
async function resolveDashVars() {
  DASH_VAR_OPTIONS = {}; DASH_VARVALS = {};
  for (const v of (CUR_DASH.vars || [])) {
    let opts = [];
    try {
      const r = await fetch(`${API}/dashboards/var-values`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(Object.assign({}, v, { datasource: (CUR_DASH && CUR_DASH.datasource) || "" })) }).then(r => r.json());
      opts = (r && r.values) || [];
    } catch (e) { /* ignore */ }
    if (v.include_all) opts = ["$__all", ...opts];
    DASH_VAR_OPTIONS[v.name] = opts;
    let cur = v.current || (opts.length ? opts[0] : "");
    if (cur === "$__all" || cur === "All") cur = "$__all";
    DASH_VARVALS[v.name] = cur;
  }
}
function dashRange() {
  if (DASH_RANGE.custom) return { from: DASH_RANGE.custom.from, to: DASH_RANGE.custom.to };
  const now = Math.floor(Date.now() / 1000);
  return { from: now - DASH_RANGE.hours * 3600, to: now };
}
function renderDashDetail() {
  const d = CUR_DASH, wrap = $("dashDetail");
  if (!wrap) return;
  DASH_CHART_ARGS = {};
  const ranges = [[1, "1h"], [6, "6h"], [24, "24h"], [72, "3d"], [168, "7d"]];
  const rangeChips = ranges.map(([h, l]) => `<button class="chip-btn ${!DASH_RANGE.custom && DASH_RANGE.hours === h ? "active" : ""}" data-drange="${h}">${l}</button>`).join("");
  const rng = dashRange();
  const varSel = (d.vars || []).map(v => {
    const opts = DASH_VAR_OPTIONS[v.name] || [];
    const cur = DASH_VARVALS[v.name];
    if (v.type === "textbox" || v.type === "constant") {
      return `<span class="dash-var"><label>${esc(v.label || v.name)}</label><input type="text" class="dt-input" data-dvar="${esc(v.name)}" value="${esc(cur || "")}" style="width:120px"></span>`;
    }
    const optsHtml = opts.map(o => `<option value="${esc(o)}" ${o === cur ? "selected" : ""}>${o === "$__all" ? "全部" : esc(o)}</option>`).join("");
    return `<span class="dash-var"><label>${esc(v.label || v.name)}</label><div class="select-wrap sm"><select data-dvar="${esc(v.name)}">${optsHtml || `<option value="">（无候选）</option>`}</select></div></span>`;
  }).join("");
  const srcBadge = (d.source && d.source.indexOf("grafana:") === 0) ? '<span class="dash-badge">Grafana</span>'
    : (d.source === "ai" || (d.source || "").indexOf("ai-analysis") === 0) ? '<span class="dash-badge ai">AI</span>' : "";
  wrap.innerHTML = `
    <div class="dash-bar">
      <div class="dash-bar-main">
        <button class="dash-icon-btn" id="dashBack" title="返回列表">←</button>
        <div class="dash-title-wrap"><span class="dash-title">${esc(d.name)}</span>${srcBadge}</div>
        <div class="dash-bar-actions">
          <button class="btn ghost sm" id="dashAnalyzeBtn" title="AI 解读当前看板数据">🔍 解读</button>
          <button class="btn ghost sm" id="dashOptimizeBtn" title="AI 评审并给优化建议">✨ 优化</button>
          <button class="btn ghost sm" id="dashTicketBtn" title="AI 研判 → 生成工单">🎫 工单</button>
          <span class="dash-sep"></span>
          ${DASH_EDIT
            ? `<button class="btn sm" id="dashAddPanel">+ 面板</button><button class="btn sm" id="dashEditVars">变量</button><button class="btn sm" id="dashEditMeta">信息</button><button class="btn sm" id="dashCancelEdit">退出</button><button class="btn primary sm" id="dashSaveBtn">保存</button>`
            : `<button class="btn primary sm" id="dashEditBtn">✎ 编辑</button>`}
        </div>
      </div>
      <div class="dash-bar-sub">
        <div class="dash-range">${rangeChips}<button class="chip-btn ${DASH_RANGE.custom ? "active" : ""}" id="dashCustomToggle">自定义</button><button class="chip-btn dash-refresh" id="dashRefresh" title="刷新">↻</button></div>
        <span class="chart-custom-range" id="dashCustomPanel"${DASH_RANGE.custom ? "" : " hidden"}>
          <input type="datetime-local" id="dashCustomFrom" class="dt-input" value="${toLocalDatetimeValue(rng.from)}">
          <span class="dt-sep">→</span>
          <input type="datetime-local" id="dashCustomTo" class="dt-input" value="${toLocalDatetimeValue(rng.to)}">
          <button class="chip-btn primary" id="dashCustomApply">应用</button>
        </span>
        <span class="dash-spacer"></span>
        <div class="dash-picker"><span class="dash-picker-lbl">数据源</span><div class="select-wrap sm"><select id="dashDSSelect">${dsOptions(d.datasource, ["prometheus"], true)}</select></div></div>
        <div class="dash-vars">${varSel}</div>
      </div>
    </div>
    <div class="dash-grid ${DASH_EDIT ? "editing" : ""}" id="dashGrid"></div>`;
  renderPanels();
}
function renderPanels() {
  const grid = $("dashGrid");
  if (!grid || !CUR_DASH) return;
  const panels = (CUR_DASH.panels || []).slice().sort((a, b) => (a.grid.y - b.grid.y) || (a.grid.x - b.grid.x));
  if (!panels.length) {
    grid.innerHTML = `<div class="empty-box" style="grid-column:span 24">还没有面板。${DASH_EDIT ? "点上方「+ 面板」添加。" : "点「编辑」进入编辑模式后添加面板。"}</div>`;
    return;
  }
  grid.innerHTML = panels.map(p => {
    const w = Math.max(1, Math.min(24, p.grid.w || 12));
    // 面板按 gridPos.h 占满对应行数（网格行高固定），使卡片填满其矩形区域 —— 消除
    // 「矮面板(如 stat)紧邻高面板(timeseries)时下方大片空白」的根因（原来靠内容撑高会留缝）。
    const h = Math.max(3, Math.min(48, p.grid.h || 8));
    const dsTag = p.datasource ? `<span class="dash-panel-ds" title="面板数据源">${esc(dsLabel(p.datasource))}</span>` : "";
    const edit = DASH_EDIT ? `<div class="panel-edit-actions">
        <button class="mini-btn" data-pact="up" data-id="${p.id}" title="上移">↑</button>
        <button class="mini-btn" data-pact="down" data-id="${p.id}" title="下移">↓</button>
        <button class="mini-btn" data-pact="edit" data-id="${p.id}" title="编辑">✎</button>
        <button class="mini-btn del" data-pact="del" data-id="${p.id}" title="删除">✕</button>
      </div>` : "";
    return `<div class="dash-panel dp-${esc(p.type)}" style="grid-column:span ${w}; grid-row:span ${h}" data-panel="${p.id}">
      <div class="dash-panel-head"><span class="dash-panel-title" title="${esc(p.title || "")}">${esc(p.title || "")}</span>${dsTag}${edit}</div>
      <div class="dash-panel-body" id="panelBody_${p.id}"></div>
    </div>`;
  }).join("");
  panels.forEach(loadPanel);
}

/* ---------- 面板查询与绘制 ---------- */
function panelVars() { return DASH_VARVALS; }
// panelBodyH：面板正文的实际内容高度（扣除内边距），供图表填满面板用。
function panelBodyH(el) {
  if (!el) return 160;
  const cs = getComputedStyle(el);
  const pad = (parseFloat(cs.paddingTop) || 0) + (parseFloat(cs.paddingBottom) || 0);
  return Math.max(0, el.clientHeight - pad);
}
async function loadPanel(p) {
  const body = document.getElementById("panelBody_" + p.id);
  if (!body) return;
  if (p.type === "text") { body.innerHTML = `<div class="dash-text">${p.text || ""}</div>`; return; }
  if (p.type === "unsupported") {
    body.innerHTML = `<div class="dash-unsupported">⚠ 暂不支持的面板类型${p.raw_type ? "：" + esc(p.raw_type) : ""}<div class="dash-unsupported-q">${(p.targets || []).map(t => esc(t.expr)).join("<br>") || "（无查询）"}</div></div>`;
    return;
  }
  if (!(p.targets || []).length) { body.innerHTML = `<div class="dash-empty">未配置查询</div>`; return; }
  body.innerHTML = `<div class="dash-empty">加载中…</div>`;
  const { from, to } = dashRange();
  if (p.type === "logs") await loadLogsPanel(p, body, from, to);
  else if (p.type === "timeseries") await loadTimeseriesPanel(p, body, from, to);
  else if (p.type === "stat") await loadStatPanel(p, body, from, to);
  else if (p.type === "gauge") await loadGaugePanel(p, body);
  else if (p.type === "piechart" || p.type === "pie") await loadPiePanel(p, body);
  else if (p.type === "barchart" || p.type === "bar") await loadBarPanel(p, body);
  else await loadInstantPanel(p, body); // bargauge / table
}
// 即时查询公共入口：返回序列数组，出错/无数据时写占位并返回 null。
async function instantQuery(p, body) {
  let res;
  try { res = await fetch(`${API}/dashboards/query-instant`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ expr: p.targets[0].expr, datasource: resolveDS(p), vars: panelVars() }) }).then(r => r.json()); }
  catch (e) { body.innerHTML = `<div class="dash-empty">查询失败</div>`; return null; }
  if (res && res.available === false) { body.innerHTML = `<div class="dash-empty">数据源不可用（${esc(dsLabel(resolveDS(p)))}）</div>`; return null; }
  const series = (res && res.series) || [];
  if (!series.length) { body.innerHTML = `<div class="dash-empty">无数据</div>`; return null; }
  return series;
}
function seriesVal2(s) { return +(s.Value !== undefined ? s.Value : s.value); }
function seriesLabels(s) { return s.Labels || s.labels || {}; }
// statColor：按阈值给颜色（percent / percentunit / 有量程的按占比；否则中性主色）。
function statColor(v, unit, min, max) {
  let pct = null;
  if (unit === "percent") pct = v;
  else if (unit === "percentunit") pct = v * 100;
  else if (max != null && min != null && max > min) pct = (v - min) / (max - min) * 100;
  if (pct == null) return "var(--accent)";
  return pct >= 90 ? "var(--crit)" : pct >= 75 ? "var(--warn)" : "var(--ok)";
}
async function loadLogsPanel(p, body, from, to) {
  let res;
  try {
    res = await fetch(`${API}/dashboards/query-logs`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ expr: p.targets[0].expr, from, to, limit: 200, datasource: resolveDS(p), vars: panelVars() }) }).then(r => r.json());
  } catch (e) { body.innerHTML = `<div class="dash-empty">日志查询失败</div>`; return; }
  if (res && res.available === false) { body.innerHTML = `<div class="dash-empty">该面板需选择一个 <b>Loki</b> 数据源</div>`; return; }
  const lines = (res && res.lines) || [];
  if (!lines.length) { body.innerHTML = `<div class="dash-empty">该范围无日志</div>`; return; }
  body.innerHTML = `<div class="dash-logs">${lines.map(l => `<div class="dash-log-row"><span class="dash-log-ts">${fmtLogTs(l.ts_ms)}</span><span class="dash-log-line">${esc(l.line || "")}</span></div>`).join("")}</div>`;
}
function fmtLogTs(ms) {
  if (!ms) return "";
  const d = new Date(ms);
  const p2 = n => String(n).padStart(2, "0");
  return `${p2(d.getMonth() + 1)}-${p2(d.getDate())} ${p2(d.getHours())}:${p2(d.getMinutes())}:${p2(d.getSeconds())}`;
}
async function loadTimeseriesPanel(p, body, from, to) {
  const collected = []; // { labels, legendFmt, points }
  let naOff = false;
  for (const t of p.targets) {
    let res;
    try {
      res = await fetch(`${API}/dashboards/query`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ expr: t.expr, from, to, datasource: resolveDS(p), vars: panelVars() }) }).then(r => r.json());
    } catch (e) { continue; }
    if (res && res.available === false) { naOff = true; break; }
    for (const s of (res && res.series || [])) {
      if (collected.length >= 24) break; // 上限，避免图例爆炸
      collected.push({ labels: s.labels || {}, legendFmt: t.legend, points: s.points || [] });
    }
  }
  if (naOff) { body.innerHTML = `<div class="dash-empty">数据源不可用（${esc(dsLabel(resolveDS(p)))}）—— 请在「数据源」配置或改选面板数据源</div>`; return; }
  if (!collected.length) { body.innerHTML = `<div class="dash-empty">该范围无数据</div>`; return; }
  const labels = dashLegends(collected);
  const defs = [], tsMap = new Map();
  collected.forEach((c, i) => {
    const key = "s" + i;
    defs.push({ key, label: labels[i], color: DASH_COLORS[i % DASH_COLORS.length], fmt: v => fmtUnit(v, p.unit) });
    for (const pt of c.points) {
      const ts = Math.round(pt[0]);
      let row = tsMap.get(ts); if (!row) { row = { timestamp: ts }; tsMap.set(ts, row); }
      row[key] = pt[1];
    }
  });
  const samples = [...tsMap.values()].sort((a, b) => a.timestamp - b.timestamp);
  const cid = "dashCanvas_" + p.id;
  body.innerHTML = `<div class="chart-wrap"><canvas id="${cid}"></canvas></div>`;
  // Y 轴自适应数据范围（不再强制 percent 面板 0~100），否则像 CPU 常年 3~7% 会贴底、曲线只占底部一条。
  // 高度：面板已按 gridPos.h 占满网格行高，图表填满正文。在 rAF 里测量——刚插入 DOM 时正文尚未完成
  // 布局(clientHeight≈0)会导致高度落到 90px 下限、曲线只占容器一半，故等一帧布局稳定后再测；
  // 测不到时按 gridPos.h 反推(行高24+间距8-头/内边距~52)兜底。不再传 title(面板头已有标题)。
  const drawTs = () => {
    if (!document.getElementById(cid)) return;
    let chartH = panelBodyH(body);
    if (chartH < 120) chartH = dashRowHeight(p.grid.h || 8);
    chartH = Math.max(90, chartH);
    const args = [cid, samples, defs, null, null, { cssH: chartH }];
    DASH_CHART_ARGS[p.id] = args;
    createChart.apply(null, args);
  };
  requestAnimationFrame(drawTs);
}
// dashRowHeight：按 gridPos 行数反推面板正文可用高度（网格行高 24 + 行间距 8，扣面板头+内边距 ~52）。
function dashRowHeight(h) { const n = Math.max(3, Math.min(48, h || 8)); return n * 24 + (n - 1) * 8 - 52; }
// loadInstantPanel 处理 bargauge（横向条）与 table。
async function loadInstantPanel(p, body) {
  const series = await instantQuery(p, body);
  if (!series) return;
  if (p.type === "bargauge") {
    const min = p.min != null ? p.min : 0;
    const max = p.max != null ? p.max : (p.unit === "percent" ? 100 : (p.unit === "percentunit" ? 1 : autoMax(series)));
    body.innerHTML = `<div class="dash-bars-h">` + series.slice(0, 16).map(s => {
      const v = seriesVal2(s);
      const pct = max > min ? Math.max(0, Math.min(100, (v - min) / (max - min) * 100)) : 0;
      const col = statColor(v, p.unit, min, max);
      const lbl = legendFor(p.targets[0].legend, seriesLabels(s));
      return `<div class="dash-bar-item"><div class="dash-bar-lbl" title="${esc(lbl)}">${esc(lbl)}</div><div class="dash-bar-track"><div class="dash-bar-fill" style="width:${pct}%; background:${col}"></div></div><div class="dash-bar-val">${fmtUnit(v, p.unit)}</div></div>`;
    }).join("") + `</div>`;
  } else { // table
    const rows = series.map(s => ({ lbl: legendFor(p.targets[0].legend, seriesLabels(s)), v: seriesVal2(s) }))
      .sort((a, b) => b.v - a.v).slice(0, 200);
    body.innerHTML = `<div class="dash-table-wrap"><table class="dash-table"><thead><tr><th>序列</th><th class="num">值</th></tr></thead><tbody>` +
      rows.map(r => `<tr><td title="${esc(r.lbl)}">${esc(r.lbl)}</td><td class="num">${fmtUnit(r.v, p.unit)}</td></tr>`).join("") +
      `</tbody></table></div>`;
  }
}
// loadStatPanel：大数值（取区间最后一点）+ 阈值配色 + 迷你趋势 sparkline + 说明。
async function loadStatPanel(p, body, from, to) {
  const t = p.targets[0];
  let res;
  try { res = await fetch(`${API}/dashboards/query`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ expr: t.expr, from, to, datasource: resolveDS(p), vars: panelVars() }) }).then(r => r.json()); }
  catch (e) { body.innerHTML = `<div class="dash-empty">查询失败</div>`; return; }
  if (res && res.available === false) { body.innerHTML = `<div class="dash-empty">数据源不可用（${esc(dsLabel(resolveDS(p)))}）</div>`; return; }
  const series = (res && res.series) || [];
  if (!series.length) { body.innerHTML = `<div class="dash-empty">无数据</div>`; return; }
  const s0 = series[0], pts = s0.points || [];
  const val = pts.length ? pts[pts.length - 1][1] : 0;
  const col = statColor(val, p.unit, p.min, p.max);
  const lbl = legendFor(t.legend, s0.labels || {});
  const spark = pts.length > 1 ? svgSparkline(pts.map(pt => pt[1]), col) : "";
  body.innerHTML = `<div class="dash-stat2">
      <div class="dash-stat-num" style="color:${col}">${fmtUnit(+val, p.unit)}</div>
      ${lbl ? `<div class="dash-stat-cap">${esc(lbl)}</div>` : ""}
      ${spark ? `<div class="dash-stat-spark">${spark}</div>` : ""}
    </div>`;
}
// loadGaugePanel：每个序列一个圆环仪表（径向进度），阈值配色，flex 自适应铺满。
async function loadGaugePanel(p, body) {
  const series = await instantQuery(p, body);
  if (!series) return;
  const min = p.min != null ? p.min : 0;
  const max = p.max != null ? p.max : (p.unit === "percent" ? 100 : (p.unit === "percentunit" ? 1 : autoMax(series)));
  body.innerHTML = `<div class="dash-gauges">` + series.slice(0, 9).map(s => {
    const v = seriesVal2(s);
    const pct = max > min ? (v - min) / (max - min) * 100 : 0;
    const col = statColor(v, p.unit, min, max);
    const lbl = legendFor(p.targets[0].legend, seriesLabels(s));
    return `<div class="dash-gauge-item">${svgGauge(pct, fmtUnit(v, p.unit), col)}<div class="dash-gauge-lbl" title="${esc(lbl)}">${esc(lbl)}</div></div>`;
  }).join("") + `</div>`;
}
// loadPiePanel：环形图（每序列一片）+ 侧栏图例。
async function loadPiePanel(p, body) {
  const series = await instantQuery(p, body);
  if (!series) return;
  const items = series.slice(0, 12).map((s, i) => ({ val: Math.max(0, seriesVal2(s)), lbl: legendFor(p.targets[0].legend, seriesLabels(s)), col: DASH_COLORS[i % DASH_COLORS.length] }));
  const total = items.reduce((a, b) => a + b.val, 0) || 1;
  body.innerHTML = `<div class="dash-pie"><div class="dash-pie-chart">${svgDonut(items, total)}</div>
    <div class="dash-pie-legend">${items.map(it => `<div class="dash-pie-li"><span class="dash-pie-dot" style="background:${it.col}"></span><span class="dash-pie-name" title="${esc(it.lbl)}">${esc(it.lbl)}</span><span class="dash-pie-val">${fmtUnit(it.val, p.unit)}</span></div>`).join("")}</div></div>`;
}
// loadBarPanel：纵向柱状图（每序列一柱），适合 top-N。
async function loadBarPanel(p, body) {
  const series = await instantQuery(p, body);
  if (!series) return;
  const items = series.map((s, i) => ({ val: seriesVal2(s), lbl: legendFor(p.targets[0].legend, seriesLabels(s)), col: DASH_COLORS[i % DASH_COLORS.length] }))
    .sort((a, b) => b.val - a.val).slice(0, 16);
  const mx = Math.max(...items.map(it => it.val), 0) || 1;
  body.innerHTML = `<div class="dash-bars">` + items.map(it => {
    const h = Math.max(2, it.val / mx * 100);
    return `<div class="dash-barcol" title="${esc(it.lbl)}：${fmtUnit(it.val, p.unit)}">
        <div class="dash-barcol-v">${fmtUnit(it.val, p.unit)}</div>
        <div class="dash-barcol-track"><div class="dash-barcol-bar" style="height:${h}%; background:${it.col}"></div></div>
        <div class="dash-barcol-lbl">${esc(it.lbl)}</div></div>`;
  }).join("") + `</div>`;
}
/* ---------- SVG 组件 ---------- */
// svgSparkline：迷你趋势线（填满宽度，用于 stat 背景趋势）。
function svgSparkline(vals, color) {
  const n = vals.length; if (n < 2) return "";
  const mn = Math.min(...vals), mx = Math.max(...vals), rng = (mx - mn) || 1, W = 100, H = 28;
  const pts = vals.map((v, i) => `${(i / (n - 1) * W).toFixed(2)},${(H - (v - mn) / rng * H).toFixed(2)}`).join(" ");
  return `<svg viewBox="0 0 ${W} ${H}" preserveAspectRatio="none" class="spark-svg"><polygon points="0,${H} ${pts} ${W},${H}" fill="${color}" opacity="0.12"/><polyline points="${pts}" fill="none" stroke="${color}" stroke-width="1.5" vector-effect="non-scaling-stroke"/></svg>`;
}
// svgGauge：圆环径向进度仪表 + 中心数值。
function svgGauge(pct, valueText, color) {
  const r = 42, C = 2 * Math.PI * r;
  const off = C * (1 - Math.max(0, Math.min(1, pct / 100)));
  return `<svg viewBox="0 0 100 100" class="gauge-svg">
    <circle cx="50" cy="50" r="${r}" fill="none" stroke="var(--line2)" stroke-width="9"/>
    <circle cx="50" cy="50" r="${r}" fill="none" stroke="${color}" stroke-width="9" stroke-linecap="round" stroke-dasharray="${C.toFixed(1)}" stroke-dashoffset="${off.toFixed(1)}" transform="rotate(-90 50 50)"/>
    <text x="50" y="50" text-anchor="middle" dominant-baseline="central" class="gauge-txt" fill="var(--txt)">${esc(valueText)}</text>
  </svg>`;
}
// svgDonut：环形图（各片按占比，rotate -90 从顶部起）。
function svgDonut(items, total) {
  const r = 34, sw = 22, C = 2 * Math.PI * r;
  let off = 0;
  const segs = items.filter(it => it.val > 0).map(it => {
    const frac = it.val / total;
    const seg = `<circle cx="50" cy="50" r="${r}" fill="none" stroke="${it.col}" stroke-width="${sw}" stroke-dasharray="${(frac * C).toFixed(2)} ${(C - frac * C).toFixed(2)}" stroke-dashoffset="${(-off * C).toFixed(2)}" transform="rotate(-90 50 50)"/>`;
    off += frac;
    return seg;
  }).join("");
  return `<svg viewBox="0 0 100 100" class="pie-svg">${segs}</svg>`;
}
function legendFor(fmt, labels) {
  if (fmt && fmt.trim()) return fmt.replace(/\{\{\s*(\w+)\s*\}\}/g, (m, k) => (labels[k] !== undefined ? labels[k] : ""));
  const name = labels.__name__ || "";
  const rest = Object.keys(labels).filter(k => k !== "__name__").map(k => `${k}=${labels[k]}`).join(",");
  return (name + (rest ? `{${rest}}` : "")) || "value";
}
// dashLegends：多序列图例去重可辨。若各序列图例已互不相同则原样用；否则改用「序列之间取值不同的
// 标签」重建（如 state / mountpoint / device），避免像网络连接数那样 8 条都显示同一个 instance。
function dashLegends(collected) {
  const raw = collected.map(c => legendFor(c.legendFmt, c.labels));
  if (new Set(raw).size === raw.length) return raw; // 已可区分
  const keys = new Set();
  collected.forEach(c => Object.keys(c.labels || {}).forEach(k => { if (k !== "__name__") keys.add(k); }));
  const varying = [...keys].filter(k => new Set(collected.map(c => (c.labels || {})[k] || "")).size > 1);
  return collected.map((c, i) => {
    if (varying.length) {
      const lbl = varying.map(k => (c.labels || {})[k]).filter(v => v !== undefined && v !== "").join(" · ");
      if (lbl) return lbl;
    }
    const nm = (c.labels || {}).__name__;
    return (nm || raw[i] || "series") + " #" + (i + 1);
  });
}
function autoMax(series) {
  let m = 0;
  for (const s of series) { const v = +(s.Value !== undefined ? s.Value : s.value); if (v > m) m = v; }
  return m > 0 ? m * 1.1 : 1;
}

/* ---------- 单位格式化 ---------- */
function fmtShort(v) {
  const a = Math.abs(v);
  if (a >= 1e12) return (v / 1e12).toFixed(2) + "T";
  if (a >= 1e9) return (v / 1e9).toFixed(2) + "G";
  if (a >= 1e6) return (v / 1e6).toFixed(2) + "M";
  if (a >= 1e3) return (v / 1e3).toFixed(2) + "K";
  return (Number.isInteger(v) ? v : v.toFixed(2)) + "";
}
function fmtBytes(v) {
  const a = Math.abs(v); const u = ["B", "KB", "MB", "GB", "TB", "PB"]; let i = 0; let n = v;
  while (Math.abs(n) >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return n.toFixed(i ? 2 : 0) + u[i];
}
// fmtDuration：秒 → 人类可读时长。>=1天显示 天+小时（运行时间等长时长换算为天），
// 分钟级显示 分+秒，亚秒显示毫秒。
function fmtDuration(v) {
  const neg = v < 0 ? "-" : "";
  const a = Math.abs(v);
  if (a < 1) return neg + (a * 1000).toFixed(0) + "ms";
  if (a < 60) return neg + (a < 10 ? a.toFixed(1) : a.toFixed(0)) + "s";
  if (a < 3600) { const m = Math.floor(a / 60), s = Math.round(a % 60); return neg + m + "m" + (s ? " " + s + "s" : ""); }
  if (a < 86400) { const h = Math.floor(a / 3600), m = Math.round((a % 3600) / 60); return neg + h + "h" + (m ? " " + m + "m" : ""); }
  const d = Math.floor(a / 86400), h = Math.round((a % 86400) / 3600); return neg + d + "天" + (h ? " " + h + "h" : "");
}
function fmtUnit(v, unit) {
  if (v === undefined || v === null || isNaN(v)) return "-";
  switch (unit) {
    case "percent": return v.toFixed(1) + "%";
    case "percentunit": return (v * 100).toFixed(1) + "%";
    case "bytes": return fmtBytes(v);
    case "Bps": return fmtBytes(v) + "/s";
    case "s": case "seconds": case "duration": return fmtDuration(v);
    case "ms": return v >= 1000 ? fmtDuration(v / 1000) : v.toFixed(0) + "ms";
    case "reqps": return fmtShort(v) + "/s";
    default: return fmtShort(v);
  }
}

/* ---------- resize 重绘 ---------- */
let DASH_RESIZE_T = null;
window.addEventListener("resize", () => {
  const v = document.getElementById("view-dashboards");
  if (!v || !v.classList.contains("active") || !CUR_DASH) return;
  clearTimeout(DASH_RESIZE_T);
  DASH_RESIZE_T = setTimeout(() => {
    for (const id in DASH_CHART_ARGS) { try { createChart.apply(null, DASH_CHART_ARGS[id]); } catch (e) {} }
  }, 250);
});

/* ---------- 详情事件委托 ---------- */
safeAddEventListener("dashDetail", "click", async e => {
  const t = e.target;
  if (t.closest("#dashBack")) { showDashHome(); loadDashboards(); return; }
  if (t.closest("#dashRefresh")) { renderPanels(); return; }
  if (t.closest("#dashEditBtn")) { DASH_EDIT = true; renderDashDetail(); return; }
  if (t.closest("#dashAnalyzeBtn")) { aiAnalyzeDash(); return; }
  if (t.closest("#dashOptimizeBtn")) { aiOptimizeDash(); return; }
  if (t.closest("#dashTicketBtn")) { aiTicketDash(); return; }
  if (t.closest("#dashCancelEdit")) { openDashboard(CUR_DASH.id); return; }
  if (t.closest("#dashSaveBtn")) { saveCurDash(); return; }
  if (t.closest("#dashAddPanel")) { openPanelEditor(null); return; }
  if (t.closest("#dashEditVars")) { openVarsEditor(); return; }
  if (t.closest("#dashEditMeta")) { openDashMeta(CUR_DASH); return; }
  if (t.closest("#dashCustomToggle")) { const pn = $("dashCustomPanel"); if (pn) pn.hidden = !pn.hidden; return; }
  if (t.closest("#dashCustomApply")) { applyDashCustom(); return; }
  const rc = t.closest("[data-drange]");
  if (rc) { DASH_RANGE = { hours: +rc.dataset.drange, custom: null }; renderDashDetail(); return; }
  const pa = t.closest("[data-pact]");
  if (pa) { handlePanelAction(pa.dataset.pact, +pa.dataset.id); return; }
});
safeAddEventListener("dashDetail", "change", e => {
  if (e.target.id === "dashDSSelect") {
    CUR_DASH.datasource = e.target.value;
    // 静默持久化数据源选择（否则刷新后回退），再重解析变量 + 重绘。
    fetch(`${API}/dashboards`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(CUR_DASH) }).catch(() => {});
    resolveDashVars().then(renderDashDetail);
    return;
  }
  const sel = e.target.closest("[data-dvar]");
  if (sel) { DASH_VARVALS[sel.dataset.dvar] = sel.value; renderPanels(); }
});
function applyDashCustom() {
  const f = $("dashCustomFrom"), tt = $("dashCustomTo");
  if (!f || !tt || !f.value || !tt.value) { toast("请选择起止时间", "warn"); return; }
  const from = Math.floor(new Date(f.value).getTime() / 1000), to = Math.floor(new Date(tt.value).getTime() / 1000);
  if (!(to > from)) { toast("结束时间必须晚于开始时间", "warn"); return; }
  DASH_RANGE = { hours: 0, custom: { from, to } }; renderDashDetail();
}
function handlePanelAction(act, pid) {
  const panels = CUR_DASH.panels;
  const idx = panels.findIndex(p => p.id === pid);
  if (idx < 0) return;
  if (act === "edit") { openPanelEditor(panels[idx]); return; }
  if (act === "del") { if (confirm("删除该面板？")) { panels.splice(idx, 1); renderPanels(); } return; }
  // 上/下移：交换网格 y（保序渲染）
  const sorted = panels.slice().sort((a, b) => (a.grid.y - b.grid.y) || (a.grid.x - b.grid.x));
  const si = sorted.findIndex(p => p.id === pid);
  const swap = act === "up" ? si - 1 : si + 1;
  if (swap < 0 || swap >= sorted.length) return;
  const a = sorted[si], b = sorted[swap];
  const ty = a.grid.y, tx = a.grid.x; a.grid.y = b.grid.y; a.grid.x = b.grid.x; b.grid.y = ty; b.grid.x = tx;
  renderPanels();
}

/* ---------- 面板编辑器 ---------- */
function openPanelEditor(p) {
  $("panelId").value = p ? p.id : "";
  $("panelTitle").value = p ? (p.title || "") : "";
  $("panelType").value = p ? p.type : "timeseries";
  $("panelW").value = p ? (p.grid.w || 12) : 12;
  $("panelH").value = p ? (p.grid.h || 8) : 8;
  $("panelUnit").value = p ? (p.unit || "") : "";
  $("panelMin").value = p && p.min != null ? p.min : "";
  $("panelMax").value = p && p.max != null ? p.max : "";
  $("panelText").value = p ? (p.text || "") : "";
  PANEL_TARGETS_DRAFT = p && p.targets ? p.targets.map(t => ({ expr: t.expr, legend: t.legend || "" })) : [{ expr: "", legend: "" }];
  renderPanelTargets();
  fillPanelDS(p ? p.type : "timeseries", p ? (p.datasource || "") : "");
  panelTypeToggle();
  $("panelEditTitle").textContent = p ? "编辑面板" : "添加面板";
  openMask("panelEditMask");
}
function fillPanelDS(type, selected) {
  const sel = $("panelDS");
  if (!sel) return;
  if (type === "logs") sel.innerHTML = dsOptions(selected, ["loki"], false) || `<option value="">（请先在「数据源」配置 Loki）</option>`;
  else sel.innerHTML = dsOptions(selected, ["prometheus"], true);
}
function renderPanelTargets() {
  const wrap = $("panelTargets");
  if (!wrap) return;
  wrap.innerHTML = PANEL_TARGETS_DRAFT.map((t, i) => `
    <div class="panel-target-row">
      <input type="text" class="mono" data-tgt-expr="${i}" placeholder="PromQL，如 rate(node_cpu_seconds_total[$__interval])" value="${esc(t.expr)}" style="flex:2">
      <input type="text" data-tgt-legend="${i}" placeholder="图例 {{instance}}（可空）" value="${esc(t.legend)}" style="flex:1">
      <button class="mini-btn del" data-tgt-del="${i}" title="删除">✕</button>
    </div>`).join("");
}
function panelTypeToggle() {
  const ty = $("panelType").value;
  $("panelTextRow").style.display = ty === "text" ? "" : "none";
  $("panelTargetsWrap").style.display = ty === "text" ? "none" : "";
  $("panelUnitRow").style.display = (ty === "text" || ty === "logs") ? "none" : "";
  const dsRow = $("panelDSRow"); if (dsRow) dsRow.style.display = ty === "text" ? "none" : "";
  fillPanelDS(ty, $("panelDS").value);
  const lbl = document.querySelector("#panelTargetsWrap > label");
  if (lbl) lbl.textContent = ty === "logs" ? "LogQL 查询（Loki 数据源）" : "查询（PromQL，可多条；支持 $变量 与 {{标签}} 图例）";
}
safeAddEventListener("panelType", "change", panelTypeToggle);
safeAddEventListener("panelAddTarget", "click", () => { PANEL_TARGETS_DRAFT.push({ expr: "", legend: "" }); renderPanelTargets(); });
safeAddEventListener("panelTargets", "click", e => {
  const del = e.target.closest("[data-tgt-del]");
  if (del) { syncPanelTargets(); PANEL_TARGETS_DRAFT.splice(+del.dataset.tgtDel, 1); if (!PANEL_TARGETS_DRAFT.length) PANEL_TARGETS_DRAFT.push({ expr: "", legend: "" }); renderPanelTargets(); }
});
function syncPanelTargets() {
  document.querySelectorAll("[data-tgt-expr]").forEach(el => { const i = +el.dataset.tgtExpr; if (PANEL_TARGETS_DRAFT[i]) PANEL_TARGETS_DRAFT[i].expr = el.value; });
  document.querySelectorAll("[data-tgt-legend]").forEach(el => { const i = +el.dataset.tgtLegend; if (PANEL_TARGETS_DRAFT[i]) PANEL_TARGETS_DRAFT[i].legend = el.value; });
}
safeAddEventListener("panelSave", "click", () => {
  syncPanelTargets();
  const ty = $("panelType").value;
  const title = $("panelTitle").value.trim();
  const targets = ty === "text" ? [] : PANEL_TARGETS_DRAFT.filter(t => t.expr.trim()).map(t => ({ expr: t.expr.trim(), legend: t.legend.trim() }));
  if (ty !== "text" && !targets.length) { toast(ty === "logs" ? "请填写 LogQL 查询" : "请至少填写一条 PromQL 查询", "err"); return; }
  if (ty === "logs" && !$("panelDS").value) { toast("日志面板需选择一个 Loki 数据源", "err"); return; }
  const min = $("panelMin").value.trim(), max = $("panelMax").value.trim();
  const panel = {
    id: $("panelId").value ? +$("panelId").value : nextPanelId(),
    title, type: ty, datasource: $("panelDS").value,
    grid: { x: 0, y: 9999, w: Math.max(1, Math.min(24, parseInt($("panelW").value) || 12)), h: Math.max(2, parseInt($("panelH").value) || 8) },
    unit: $("panelUnit").value,
    targets, text: $("panelText").value,
  };
  if (min !== "") panel.min = parseFloat(min);
  if (max !== "") panel.max = parseFloat(max);
  const panels = CUR_DASH.panels;
  const existing = panels.findIndex(p => p.id === panel.id);
  if (existing >= 0) { panel.grid = panels[existing].grid; panel.grid.w = Math.max(1, Math.min(24, parseInt($("panelW").value) || 12)); panel.grid.h = Math.max(2, parseInt($("panelH").value) || 8); panels[existing] = panel; }
  else { placeNewPanel(panel, panels); panels.push(panel); }
  closeMask($("panelEditMask"));
  renderPanels();
});
function nextPanelId() { let m = 0; (CUR_DASH.panels || []).forEach(p => { if (p.id > m) m = p.id; }); return m + 1; }
function placeNewPanel(panel, panels) {
  let maxY = 0; panels.forEach(p => { const b = p.grid.y + p.grid.h; if (b > maxY) maxY = b; });
  panel.grid.y = maxY; panel.grid.x = 0;
}

/* ---------- 变量编辑器 ---------- */
function openVarsEditor() {
  VARS_DRAFT = (CUR_DASH.vars || []).map(v => ({ ...v }));
  renderVarRows();
  openMask("varEditMask");
}
function renderVarRows() {
  const wrap = $("varList");
  if (!wrap) return;
  wrap.innerHTML = VARS_DRAFT.map((v, i) => `
    <div class="var-row">
      <input type="text" data-v-name="${i}" placeholder="变量名（不含$）" value="${esc(v.name || "")}" style="width:120px">
      <div class="select-wrap sm"><select data-v-type="${i}">
        <option value="query" ${v.type === "query" ? "selected" : ""}>query</option>
        <option value="custom" ${v.type === "custom" ? "selected" : ""}>custom</option>
        <option value="textbox" ${v.type === "textbox" ? "selected" : ""}>textbox</option>
        <option value="constant" ${v.type === "constant" ? "selected" : ""}>constant</option>
      </select></div>
      <input type="text" class="mono" data-v-query="${i}" placeholder="${v.type === "custom" ? "候选值：a,b,c" : v.type === "query" ? "label_values(node_uname_info, instance)" : "默认值"}" value="${esc(v.type === "custom" ? (v.options || []).join(",") : (v.query || v.current || ""))}" style="flex:1">
      <label class="mini-check" title="多选"><input type="checkbox" data-v-multi="${i}" ${v.multi ? "checked" : ""}>多</label>
      <label class="mini-check" title="含全部"><input type="checkbox" data-v-all="${i}" ${v.include_all ? "checked" : ""}>全</label>
      <button class="mini-btn del" data-v-del="${i}" title="删除">✕</button>
    </div>`).join("") || `<div class="dash-empty">还没有变量</div>`;
}
safeAddEventListener("varAdd", "click", () => { syncVarRows(); VARS_DRAFT.push({ name: "", type: "query", query: "" }); renderVarRows(); });
safeAddEventListener("varList", "click", e => {
  const del = e.target.closest("[data-v-del]");
  if (del) { syncVarRows(); VARS_DRAFT.splice(+del.dataset.vDel, 1); renderVarRows(); }
});
safeAddEventListener("varList", "change", e => { if (e.target.closest("[data-v-type]")) { syncVarRows(); renderVarRows(); } });
function syncVarRows() {
  VARS_DRAFT.forEach((v, i) => {
    const nm = document.querySelector(`[data-v-name="${i}"]`); if (nm) v.name = nm.value.trim();
    const ty = document.querySelector(`[data-v-type="${i}"]`); if (ty) v.type = ty.value;
    const q = document.querySelector(`[data-v-query="${i}"]`);
    if (q) { if (v.type === "custom") { v.options = q.value.split(",").map(s => s.trim()).filter(Boolean); v.query = ""; } else if (v.type === "query") { v.query = q.value.trim(); } else { v.current = q.value.trim(); } }
    const mu = document.querySelector(`[data-v-multi="${i}"]`); if (mu) v.multi = mu.checked;
    const al = document.querySelector(`[data-v-all="${i}"]`); if (al) v.include_all = al.checked;
  });
}
safeAddEventListener("varSave", "click", async () => {
  syncVarRows();
  CUR_DASH.vars = VARS_DRAFT.filter(v => v.name);
  closeMask($("varEditMask"));
  await resolveDashVars();
  renderDashDetail();
});

/* ---------- 仪表盘信息（新建 / 编辑元信息） ---------- */
function openDashMeta(d) {
  $("dashMetaId").value = d ? d.id : "";
  $("dashMetaName").value = d ? d.name : "";
  $("dashMetaDesc").value = d ? (d.description || "") : "";
  $("dashMetaTags").value = d && d.tags ? d.tags.join(",") : "";
  $("dashMetaTitle").textContent = d ? "编辑仪表盘信息" : "新建仪表盘";
  openMask("dashMetaMask");
}
safeAddEventListener("dashMetaSave", "click", async () => {
  const id = $("dashMetaId").value;
  const name = $("dashMetaName").value.trim();
  if (!name) { toast("请填写名称", "err"); return; }
  const tags = $("dashMetaTags").value.split(",").map(s => s.trim()).filter(Boolean);
  const desc = $("dashMetaDesc").value.trim();
  // 编辑当前打开的仪表盘元信息（在内存里改，随保存落盘）
  if (CUR_DASH && CUR_DASH.id === id && id) {
    CUR_DASH.name = name; CUR_DASH.description = desc; CUR_DASH.tags = tags;
    closeMask($("dashMetaMask"));
    if (DASH_EDIT) renderDashDetail(); else saveCurDash();
    return;
  }
  // 从列表编辑信息：合并进已存的完整对象后保存
  let base = { id, name, description: desc, tags, panels: [] };
  if (id) { try { base = await fetch(`${API}/dashboards/${encodeURIComponent(id)}`).then(r => r.json()); base.name = name; base.description = desc; base.tags = tags; } catch (e) {} }
  const r = await fetch(`${API}/dashboards`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(base) }).then(r => r.json());
  closeMask($("dashMetaMask"));
  if (r && r.ok) {
    toast("已保存", "ok");
    if (!id) { openDashboard(r.id).then(() => { DASH_EDIT = true; renderDashDetail(); }); }
    else loadDashboards();
  } else toast("保存失败：" + ((r && r.error) || ""), "err");
});
async function saveCurDash() {
  if (!CUR_DASH) return;
  await withLoading("dashSaveBtn", async () => {
    try {
      const r = await fetch(`${API}/dashboards`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(CUR_DASH) });
      if (r.ok) { toast("已保存", "ok"); DASH_EDIT = false; renderDashDetail(); }
      else { const j = await r.json().catch(() => ({})); toast("保存失败：" + (j.error || ""), "err"); }
    } catch (e) { toast("保存失败：" + e, "err"); }
  });
}

/* ---------- Grafana 导入 ---------- */
safeAddEventListener("dashImportBtn", "click", () => {
  $("dashImportId").value = ""; $("dashImportJson").value = ""; $("dashImportName").value = "";
  const fn = $("dashImportFileName"); if (fn) fn.textContent = "";
  const fmt = $("dashImportFormat"); if (fmt) fmt.value = "auto";
  const fi = $("dashImportFile"); if (fi) fi.value = "";
  openMask("dashImportMask");
});
safeAddEventListener("dashImportFileBtn", "click", () => { const fi = $("dashImportFile"); if (fi) fi.click(); });
safeAddEventListener("dashImportFile", "change", e => {
  const f = e.target.files && e.target.files[0];
  if (!f) return;
  if (f.size > 8 * 1024 * 1024) { toast("文件过大（上限 8MB）", "err"); return; }
  const reader = new FileReader();
  reader.onload = () => { $("dashImportJson").value = reader.result || ""; const fn = $("dashImportFileName"); if (fn) fn.textContent = "已载入：" + f.name; };
  reader.onerror = () => toast("读取文件失败", "err");
  reader.readAsText(f);
});
safeAddEventListener("dashImportSave", "click", async () => {
  const body = { grafana_id: $("dashImportId").value.trim(), json: $("dashImportJson").value.trim(), name: $("dashImportName").value.trim(), format: ($("dashImportFormat") || {}).value || "auto" };
  if (!body.grafana_id && !body.json) { toast("请填写 grafana.com 看板 ID，或粘贴 / 上传 JSON", "err"); return; }
  await withLoading("dashImportSave", async () => {
    try {
      const r = await fetch(`${API}/dashboards/import-grafana`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      const j = await r.json().catch(() => ({}));
      if (r.ok && j.ok) {
        closeMask($("dashImportMask"));
        const kind = j.format === "nightingale" ? "夜莺" : "Grafana";
        toast(`已从 ${kind} 导入「${j.name}」：${j.panels} 面板${j.unsupported ? "（" + j.unsupported + " 个类型不支持，已占位）" : ""}`, "ok");
        openDashboard(j.id);
      } else toast("导入失败：" + (j.error || r.status), "err");
    } catch (e) { toast("导入失败：" + e, "err"); }
  });
});

/* ---------- AI 闭环：生成 / 解读 / 优化 / 建工单 ---------- */
async function dashDigest() {
  try { return await fetch(`${API}/dashboards/${encodeURIComponent(CUR_DASH.id)}/digest`).then(r => r.json()); }
  catch (e) { return null; }
}
async function aiAnalyzeDash() {
  if (!CUR_DASH) return;
  const d = await dashDigest();
  if (!d || d.error) { toast("获取看板数据失败", "err"); return; }
  openAIAssist({ task: "dashboard_analysis", title: "🔍 AI 解读 · " + CUR_DASH.name, mode: "analyze", context: d.digest || "", hint: "AI 正在解读看板实时数据…" });
}
async function aiOptimizeDash() {
  if (!CUR_DASH) return;
  const d = await dashDigest();
  if (!d || d.error) { toast("获取看板数据失败", "err"); return; }
  const ctx = (d.structure || "") + "\n\n【实时近况】\n" + (d.digest || "");
  const dashId = CUR_DASH.id;
  openAIAssist({
    task: "dashboard_optimize", title: "✨ AI 优化 · " + CUR_DASH.name, mode: "analyze",
    context: ctx, hint: "AI 正在评审看板并给出优化建议…",
    applyLabel: "应用优化到看板",
    applyTo: async (code) => {
      // 用完整回复（而非仅首个代码块）：AI 可能在 json 前先给了 PromQL 代码块，只取首块会拿错内容；
      // 服务端 extractJSONObject 会优先定位 ```json 块，更稳。
      const answer = (typeof _aiAssistState !== "undefined" && _aiAssistState.lastAnswer) || code || "";
      if (!answer.trim()) { toast("请先等 AI 给出优化建议再应用", "err"); return; }
      toast("正在应用优化…", "ok");
      try {
        const j = await fetch(`${API}/dashboards/${encodeURIComponent(dashId)}/ai-apply`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ json: answer }) }).then(r => r.json());
        if (j.ok) { toast(`已应用优化：${j.panels} 面板`, "ok"); if (CUR_DASH && CUR_DASH.id === dashId) openDashboard(dashId); }
        else toast("应用失败：" + (j.error || "AI 未给出可应用的看板结构，请点「重新生成」重试"), "err");
      } catch (e) { toast("应用失败：" + e, "err"); }
    }
  });
}
async function aiTicketDash() {
  if (!CUR_DASH) return;
  await withLoading("dashTicketBtn", async () => {
    try {
      const j = await fetch(`${API}/dashboards/${encodeURIComponent(CUR_DASH.id)}/ai-ticket`, { method: "POST" }).then(r => r.json());
      if (j.ok && j.needed) toast(`已建工单 #${j.ticket_id}（${j.priority}）：${j.title}`, "ok");
      else if (j.ok && !j.needed) toast(j.message || "AI 研判当前无明显异常，未建工单", "ok");
      else toast("建工单失败：" + (j.error || ""), "err");
    } catch (e) { toast("建工单失败：" + e, "err"); }
  });
}
safeAddEventListener("dashAIBtn", "click", () => { $("dashAIPrompt").value = ""; $("dashAIName").value = ""; openMask("dashAIMask"); });
safeAddEventListener("dashAICreate", "click", async () => {
  const prompt = $("dashAIPrompt").value.trim();
  if (!prompt) { toast("请描述你想要的看板", "err"); return; }
  const name = $("dashAIName").value.trim();
  // 后台异步生成：立即返回，不阻塞 UI；完成后经消息中心（🔔）弹窗通知。
  try {
    const j = await fetch(`${API}/dashboards/ai-create`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ prompt, name }) }).then(r => r.json());
    if (j.ok && j.queued) {
      closeMask($("dashAIMask"));
      toast("已提交后台生成，完成后右上角 🔔 会通知并可点击查看", "ok");
    } else toast("提交失败：" + (j.error || ""), "err");
  } catch (e) { toast("提交失败：" + e, "err"); }
});

/* ---------- 列表事件 ---------- */
safeAddEventListener("dashCreateBtn", "click", () => openDashMeta(null));
safeAddEventListener("dashList", "click", e => {
  const btn = e.target.closest("[data-dact]");
  const card = e.target.closest("[data-dash]");
  if (btn) {
    const id = btn.dataset.id, act = btn.dataset.dact;
    if (act === "open") openDashboard(id);
    else if (act === "meta") { const d = DASH_LIST.find(x => x.id === id); openDashMeta(d); }
    else if (act === "del") delDashboard(id);
    return;
  }
  if (card) openDashboard(card.dataset.dash);
});
async function delDashboard(id) {
  const d = DASH_LIST.find(x => x.id === id);
  if (!confirm(`确认删除仪表盘「${d ? d.name : id}」？`)) return;
  try { await fetch(`${API}/dashboards/${encodeURIComponent(id)}`, { method: "DELETE" }); toast("已删除", "ok"); loadDashboards(); }
  catch (e) { toast("删除失败：" + e, "err"); }
}
