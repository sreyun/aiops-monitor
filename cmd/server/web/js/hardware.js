// hardware.js — 硬件健康面板（Redfish/BMC）
// Loaded as part of the unified app.js bundle.
//
// 交互：卡片 / 列表自由切换 → 点任意一项打开详情弹窗（全量数据 + 重点突出 + 历史曲线）。
// 注意：CSP 为 script-src 'self'（无 unsafe-inline），**禁止内联 onclick**——一律事件委托。

let HW_RESULTS = [];                                   // [{host, snap}]
let HW_VIEW_MODE = localStorage.getItem("aiops_hw_view") || "card"; // card | list
let HW_CHARTS = {};                                    // 详情弹窗内的图表实例
let HW_CUR = null;                                     // 当前打开详情的项
let HW_HIST_RANGE = "6h";

/* ---------- 数据加载 ---------- */

// 自己拉主机列表，不依赖 window._cachedHosts —— 后者由异步 refresh 填充，
// 首屏点进来时常为 undefined 且此后无人重渲染，导致永远停在"暂无主机"。
async function loadHardwarePanel() {
  const container = $("hardwarePanel");
  if (!container) return;
  container.innerHTML = `<div class="empty-line">${I18N.t("ui.loading") || "加载中…"}</div>`;
  let hosts = [];
  try {
    hosts = (window._cachedHosts && window._cachedHosts.length) ? window._cachedHosts
          : (await fetch(`${API}/hosts`).then(r => r.json()) || []);
  } catch (e) { hosts = []; }
  if (!hosts.length) {
    container.innerHTML = `<div class="empty-line">${I18N.t("hardware.no_hosts") || "暂无主机"}</div>`;
    return;
  }
  // 不过滤离线主机：BMC 是带外通道，主机宕机时的硬件数据恰恰最有价值。
  const results = [];
  await Promise.all(hosts.map(h =>
    fetch(`${API}/hardware/health?host=${encodeURIComponent(h.id)}`)
      .then(r => r.json())
      .then(d => { (d.snapshots || []).forEach(s => results.push({ host: h, snap: s })); })
      .catch(() => {})
  ));
  HW_RESULTS = results;
  renderHardwarePanel();
}

/* ---------- 渲染 ---------- */

function hwHealthMeta(health) {
  if (health === "OK") return { cls: "ok", icon: "✓", label: health };
  if (health === "Warning") return { cls: "warn", icon: "⚠", label: health };
  if (health === "Critical") return { cls: "crit", icon: "✕", label: health };
  return { cls: "", icon: "?", label: health || "Unknown" };
}

// 汇总一台设备的"重点"：最高温、功耗、异常部件数
function hwSummary(sd) {
  const temps = sd.temps || [], fans = sd.fans || [], power = sd.power || {};
  const maxTemp = temps.length ? Math.max(...temps.map(t => t.reading || 0)) : 0;
  let bad = 0;
  const isBad = h => h === "Warning" || h === "Critical";
  temps.forEach(t => { if (isBad(t.status) || (t.upper_critical > 0 && t.reading >= t.upper_critical)) bad++; });
  fans.forEach(f => { if (isBad(f.health) || isBad(f.status)) bad++; });
  (power.psus || []).forEach(p => { if (isBad(p.health)) bad++; });
  (sd.storage || []).forEach(s => { if (isBad(s.health) || s.smart_warn) bad++; });
  ((sd.memory || {}).dimms || []).forEach(d => { if (isBad(d.health)) bad++; });
  (sd.cpus || []).forEach(c => { if (isBad(c.health)) bad++; });
  return { maxTemp, watts: power.total_watts || 0, bad, temps, fans, power };
}

function renderHardwarePanel() {
  const container = $("hardwarePanel");
  if (!container) return;
  if (!HW_RESULTS.length) {
    container.innerHTML = `<div class="empty-line">${I18N.t("hardware.no_data") || "暂无硬件数据（需在 Agent 配置 Redfish 目标）"}</div>`;
    return;
  }
  // 异常优先排序：Critical > Warning > OK，让最需要关注的排在最前
  const order = { Critical: 0, Warning: 1, OK: 2 };
  const items = HW_RESULTS.slice().sort((a, b) =>
    (order[a.snap.health] ?? 3) - (order[b.snap.health] ?? 3));
  container.innerHTML = HW_VIEW_MODE === "list" ? hwListHTML(items) : hwCardHTML(items);
}

function hwCardHTML(items) {
  return `<div class="hw-grid">` + items.map((it, i) => {
    const snap = it.snap, sd = snap.snapshot || {}, m = hwHealthMeta(snap.health), s = hwSummary(sd);
    const stat = (v, label) => v ? `<span class="hw-stat" title="${label}">${v}</span>` : "";
    return `<div class="hw-card" data-hwidx="${i}" role="button" tabindex="0">
      <div class="hw-card-header">
        <span class="hw-health-dot hw-${m.cls}">${m.icon}</span>
        <div class="hw-card-info">
          <div class="hw-card-name">${esc(snap.target_name || snap.target_url)}</div>
          <div class="hw-card-sub">${esc(it.host.hostname || it.host.id)} · ${esc(m.label)}</div>
        </div>
        ${s.bad > 0 ? `<span class="badge crit">${s.bad} 项异常</span>` : ""}
      </div>
      <div class="hw-quick-stats">
        ${stat((sd.memory || {}).total_gb ? (sd.memory.total_gb).toFixed(0) + "GB" : "", "内存")}
        ${stat((sd.cpus || []).length ? `${sd.cpus.length} × ${sd.cpus[0].cores || "?"}C` : "", "CPU")}
        ${stat(s.maxTemp ? s.maxTemp.toFixed(0) + "°C" : "", "最高温度")}
        ${stat(s.watts ? s.watts.toFixed(0) + "W" : "", "功耗")}
        ${stat((sd.storage || []).length ? sd.storage.length + "盘" : "", "存储")}
        ${stat(s.fans.length ? s.fans.length + "扇" : "", "风扇")}
      </div>
      <div class="hw-expand-hint">${I18N.t("hardware.open_detail") || "点击查看详情与历史曲线 →"}</div>
    </div>`;
  }).join("") + `</div>`;
}

function hwListHTML(items) {
  return `<div class="hw-list">` + items.map((it, i) => {
    const snap = it.snap, sd = snap.snapshot || {}, m = hwHealthMeta(snap.health), s = hwSummary(sd);
    return `<div class="hw-row" data-hwidx="${i}" role="button" tabindex="0">
      <span class="hw-health-dot hw-${m.cls}">${m.icon}</span>
      <div class="hw-row-id">
        <div class="hw-row-name">${esc(snap.target_name || snap.target_url)}</div>
        <div class="hw-row-sub">${esc(it.host.hostname || it.host.id)}</div>
      </div>
      <span class="badge ${m.cls === "ok" ? "ok" : m.cls === "warn" ? "warn" : "crit"}">${esc(m.label)}</span>
      ${s.bad > 0 ? `<span class="badge crit">${s.bad} 异常</span>` : `<span class="hw-row-cell">—</span>`}
      <span class="hw-row-cell mono">${s.maxTemp ? s.maxTemp.toFixed(0) + "°C" : "-"}</span>
      <span class="hw-row-cell mono">${s.watts ? s.watts.toFixed(0) + "W" : "-"}</span>
      <span class="hw-row-cell mono">${(sd.cpus || []).length}C / ${((sd.memory || {}).total_gb || 0).toFixed(0)}GB</span>
      <span class="hw-row-cell">${(sd.storage || []).length}盘 · ${s.fans.length}扇</span>
    </div>`;
  }).join("") + `</div>`;
}

/* ---------- 详情弹窗 ---------- */

function openHwDetail(idx) {
  const order = { Critical: 0, Warning: 1, OK: 2 };
  const items = HW_RESULTS.slice().sort((a, b) => (order[a.snap.health] ?? 3) - (order[b.snap.health] ?? 3));
  const it = items[idx];
  if (!it) return;
  HW_CUR = it;
  const snap = it.snap, sd = snap.snapshot || {}, m = hwHealthMeta(snap.health);
  $("hwDetailTitle").textContent = `${snap.target_name || snap.target_url} · ${it.host.hostname || it.host.id}`;
  $("hwDetailBody").innerHTML = hwDetailHTML(it, sd, m);
  $("hwDetailMask").classList.add("show");
  loadHwHistory(); // 异步填充历史曲线
}

function hwDetailHTML(it, sd, m) {
  const s = hwSummary(sd);
  const isBad = h => h === "Warning" || h === "Critical";
  const badCls = h => h === "Critical" ? "hw-crit-text" : h === "Warning" ? "hw-warn-text" : "";

  // ── 重点摘要条：健康 / 异常数 / 最高温 / 功耗 / 冗余 ──
  let html = `<div class="hw-kpis">
    <div class="hw-kpi hw-${m.cls}"><div class="hw-kpi-v">${m.icon} ${esc(m.label)}</div><div class="hw-kpi-k">整机健康</div></div>
    <div class="hw-kpi ${s.bad ? "hw-crit" : "hw-ok"}"><div class="hw-kpi-v">${s.bad}</div><div class="hw-kpi-k">异常部件</div></div>
    <div class="hw-kpi"><div class="hw-kpi-v">${s.maxTemp ? s.maxTemp.toFixed(0) + "°C" : "-"}</div><div class="hw-kpi-k">最高温度</div></div>
    <div class="hw-kpi"><div class="hw-kpi-v">${s.watts ? s.watts.toFixed(0) + "W" : "-"}</div><div class="hw-kpi-k">总功耗</div></div>
    <div class="hw-kpi"><div class="hw-kpi-v">${esc((sd.power || {}).redundancy || "-")}</div><div class="hw-kpi-k">电源冗余</div></div>
  </div>`;

  // ── 异常项置顶（重点突出）──
  const bads = [];
  (sd.temps || []).forEach(t => { if (isBad(t.status) || (t.upper_critical > 0 && t.reading >= t.upper_critical)) bads.push(["温度", t.name, `${t.reading}°C`, t.status]); });
  (sd.fans || []).forEach(f => { if (isBad(f.health) || isBad(f.status)) bads.push(["风扇", f.name, `${f.rpm} RPM`, f.health || f.status]); });
  ((sd.power || {}).psus || []).forEach(p => { if (isBad(p.health)) bads.push(["电源", p.name, `${p.input_watts}W`, p.health]); });
  (sd.storage || []).forEach(d => { if (isBad(d.health) || d.smart_warn) bads.push(["磁盘", d.name, d.smart_warn ? "SMART 预测故障" : `${(d.capacity_gb || 0).toFixed(0)}GB`, d.health]); });
  ((sd.memory || {}).dimms || []).forEach(d => { if (isBad(d.health)) bads.push(["内存", d.slot || d.name, `${(d.capacity_gb || 0).toFixed(0)}GB`, d.health]); });
  (sd.cpus || []).forEach(c => { if (isBad(c.health)) bads.push(["CPU", c.name, c.model || "", c.health]); });
  if (bads.length) {
    html += `<div class="hw-bad-box"><h4>⚠ 需要关注（${bads.length}）</h4><table class="hw-table">
      <tr><th>部件</th><th>名称</th><th>读数</th><th>状态</th></tr>` +
      bads.map(b => `<tr class="${badCls(b[3])}"><td>${esc(b[0])}</td><td>${esc(b[1])}</td><td>${esc(b[2])}</td><td>${esc(b[3])}</td></tr>`).join("") +
      `</table></div>`;
  }

  // ── 历史曲线 ──
  const wrap = id => `<div class="chart-wrap"><canvas id="${id}" width="1000" height="200"></canvas>
    <button class="chart-enlarge" data-hwchart="${id}" title="${I18N.t("ui.zoom_preview") || "放大预览"}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg></button></div>`;
  html += `<h4>历史曲线</h4>
    <div class="chart-controls">${["1h", "6h", "24h", "7d"].map(r =>
      `<button class="chip-btn ${HW_HIST_RANGE === r ? "active" : ""}" data-hwrange="${r}">${r}</button>`).join("")}</div>
    <div class="chart-container">${wrap("hwChartTemp")}${wrap("hwChartFan")}${wrap("hwChartPower")}${wrap("hwChartHealth")}</div>`;

  // ── 全量明细 ──
  const tbl = (title, head, rows) => rows.length
    ? `<h4>${title}</h4><table class="hw-table"><tr>${head.map(h => `<th>${h}</th>`).join("")}</tr>${rows.join("")}</table>` : "";

  html += tbl(`CPU (${(sd.cpus || []).length})`, ["名称", "型号", "核心/线程", "最大频率", "健康"],
    (sd.cpus || []).map(c => `<tr class="${badCls(c.health)}"><td>${esc(c.name)}</td><td>${esc(c.model)}</td><td>${c.cores}C/${c.threads}T</td><td>${c.max_freq_mhz ? c.max_freq_mhz + "MHz" : "-"}</td><td>${esc(c.health)}</td></tr>`));

  const mem = sd.memory || {};
  html += tbl(`内存 (${(mem.total_gb || 0).toFixed(0)}GB${mem.used_gb > 0 ? " / 已用 " + mem.used_gb.toFixed(0) + "GB" : ""})`,
    ["插槽", "容量", "类型", "速率", "健康"],
    (mem.dimms || []).map(d => `<tr class="${badCls(d.health)}"><td>${esc(d.slot || d.name)}</td><td>${(d.capacity_gb || 0).toFixed(0)}GB</td><td>${esc(d.type || "-")}</td><td>${d.speed_mhz ? d.speed_mhz + "MHz" : "-"}</td><td>${esc(d.health || "-")}</td></tr>`));

  html += tbl(`温度传感器 (${(sd.temps || []).length})`, ["传感器", "读数", "告警阈值", "严重阈值", "状态"],
    (sd.temps || []).map(t => {
      const cls = (t.upper_critical > 0 && t.reading >= t.upper_critical) ? "hw-crit-text"
                : (t.upper_caution > 0 && t.reading >= t.upper_caution) ? "hw-warn-text" : badCls(t.status);
      return `<tr class="${cls}"><td>${esc(t.name)}</td><td>${t.reading}°C</td><td>${t.upper_caution > 0 ? t.upper_caution + "°C" : "-"}</td><td>${t.upper_critical > 0 ? t.upper_critical + "°C" : "-"}</td><td>${esc(t.status)}</td></tr>`;
    }));

  html += tbl(`风扇 (${(sd.fans || []).length})`, ["名称", "RPM", "状态", "健康"],
    (sd.fans || []).map(f => `<tr class="${badCls(f.health) || badCls(f.status)}"><td>${esc(f.name)}</td><td>${f.rpm}</td><td>${esc(f.status || "-")}</td><td>${esc(f.health)}</td></tr>`));

  html += tbl(`存储 (${(sd.storage || []).length})`, ["名称", "型号", "类型", "容量", "SMART", "健康"],
    (sd.storage || []).map(d => `<tr class="${d.smart_warn ? "hw-crit-text" : badCls(d.health)}"><td>${esc(d.name)}</td><td>${esc(d.model || "-")}</td><td>${esc(d.media_type || d.protocol || "-")}</td><td>${(d.capacity_gb || 0).toFixed(0)}GB</td><td>${d.smart_warn ? "⚠ 预测故障" : "正常"}</td><td>${esc(d.health)}</td></tr>`));

  const psus = (sd.power || {}).psus || [];
  html += tbl(`电源 (${psus.length})`, ["PSU", "输入(W)", "输出(W)", "健康", "状态"],
    psus.map(p => `<tr class="${badCls(p.health)}"><td>${esc(p.name)}</td><td>${p.input_watts}W</td><td>${p.output_watts || "-"}W</td><td>${esc(p.health)}</td><td>${esc(p.state || "-")}</td></tr>`));

  html += tbl(`固件 (${(sd.firmware || []).length})`, ["名称", "版本"],
    (sd.firmware || []).map(f => `<tr><td>${esc(f.name)}</td><td>${esc(f.version)}</td></tr>`));

  // ── 元信息 ──
  const upd = it.snap.updated_at ? new Date(it.snap.updated_at).toLocaleString()
            : (sd.timestamp ? new Date(sd.timestamp * 1000).toLocaleString() : "-");
  html += `<div class="hint" style="margin-top:10px">BMC 地址 <code class="mono">${esc(it.snap.target_url || "-")}</code> · 运行状态 ${esc(sd.state || "-")} · 更新时间 ${esc(upd)}`;
  if (sd.error) html += ` · <span class="hw-crit-text">采集错误：${esc(sd.error)}</span>`;
  html += `</div>`;
  return html;
}

/* ---------- 历史曲线 ---------- */

async function loadHwHistory() {
  if (!HW_CUR) return;
  const hostID = HW_CUR.host.id, target = HW_CUR.snap.target_name || "";
  HW_CHARTS = {};
  const specs = [
    ["hwChartTemp", "temperature", "温度 (°C)", v => v.toFixed(0) + "°C"],
    ["hwChartFan", "fan_rpm", "风扇转速 (RPM)", v => v.toFixed(0)],
    ["hwChartPower", "power", "功耗 (W)", v => v.toFixed(0) + "W"],
    ["hwChartHealth", "health_score", "健康分 (2=OK/1=警告/0=严重)", v => v.toFixed(0)],
  ];
  await Promise.all(specs.map(async ([cid, metric, title, fmt]) => {
    try {
      const qs = new URLSearchParams({ host: hostID, metric, range: HW_HIST_RANGE });
      if (target) qs.set("target", target);
      const d = await fetch(`${API}/hardware/history?${qs}`).then(r => r.json());
      const series = hwParseSeries(d.points || []);
      if (!series.length) {
        const c = $(cid);
        if (c) drawChartEmpty(c.getContext("2d"), c.getBoundingClientRect().width || 1000, 200,
          "暂无历史数据（需等待采集积累）");
        return;
      }
      // 把多序列（每个传感器/风扇一条）对齐成 createChart 需要的 samples 结构
      const tsSet = new Set();
      series.forEach(s => s.pts.forEach(p => tsSet.add(p[0])));
      const samples = [...tsSet].sort((a, b) => a - b).map(ts => {
        const row = { timestamp: ts };
        series.forEach((s, i) => { const hit = s.pts.find(p => p[0] === ts); row["v" + i] = hit ? hit[1] : null; });
        return row;
      });
      const palette = ["#4c8dff", "#f7b23b", "#2fd07a", "#f2545b", "#8b5cf6", "#43b6f0", "#e06c9a", "#6ac4b8"];
      const defs = series.slice(0, 8).map((s, i) => ({
        key: "v" + i, label: s.name, color: palette[i % palette.length], fmt,
      }));
      HW_CHARTS[cid] = createChart(cid, samples, defs, null, null, { title });
    } catch (e) { /* 单图失败不影响其它图 */ }
  }));
}

// 把 Prometheus data.result 解析成 [{name, pts:[[tsSec, val]]}]
function hwParseSeries(points) {
  const out = [];
  (points || []).forEach(p => {
    if (!p || !p.values) return;
    const lbl = p.metric || {};
    const name = lbl.sensor || lbl.fan_name || lbl.target || "value";
    const pts = p.values.map(v => [Number(v[0]), parseFloat(v[1])]).filter(v => !isNaN(v[1]));
    if (pts.length) out.push({ name, pts });
  });
  return out;
}

/* ---------- 视图切换 ---------- */

function switchHwView(mode) {
  HW_VIEW_MODE = mode === "list" ? "list" : "card";
  try { localStorage.setItem("aiops_hw_view", HW_VIEW_MODE); } catch (e) {}
  document.querySelectorAll("#hwViewToggle .vt-btn").forEach(b =>
    b.classList.toggle("active", b.dataset.view === HW_VIEW_MODE));
  renderHardwarePanel();
}

/* ---------- 事件（全部委托，符合 CSP script-src 'self'） ---------- */

safeAddEventListener("hardwarePanel", "click", e => {
  const item = e.target.closest("[data-hwidx]");
  if (item) openHwDetail(parseInt(item.dataset.hwidx));
});
safeAddEventListener("hardwarePanel", "keydown", e => {
  if (e.key !== "Enter" && e.key !== " ") return;
  const item = e.target.closest("[data-hwidx]");
  if (item) { e.preventDefault(); openHwDetail(parseInt(item.dataset.hwidx)); }
});
safeAddEventListener("hwViewToggle", "click", e => {
  const b = e.target.closest("[data-view]");
  if (b) switchHwView(b.dataset.view);
});
safeAddEventListener("hwRefreshBtn", "click", loadHardwarePanel);
safeAddEventListener("hwDetailBody", "click", e => {
  const r = e.target.closest("[data-hwrange]");
  if (r) {
    HW_HIST_RANGE = r.dataset.hwrange;
    document.querySelectorAll("#hwDetailBody [data-hwrange]").forEach(b =>
      b.classList.toggle("active", b.dataset.hwrange === HW_HIST_RANGE));
    loadHwHistory();
    return;
  }
  const z = e.target.closest("[data-hwchart]");
  if (z) { const ch = HW_CHARTS[z.dataset.hwchart]; if (ch) openChartZoom(ch); }
});

// 供 nav.js 的 _pageRenderers 调用
if (typeof window._pageRenderers === "undefined") window._pageRenderers = {};
window._pageRenderers.hardware = loadHardwarePanel;
