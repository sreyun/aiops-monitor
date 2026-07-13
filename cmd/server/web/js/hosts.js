/* ---------- 渲染：主机卡片 ---------- */
function hostCard(h) {
  const m = h.latest || {};
  const swap = (m.swap_total || 0) > 0
    ? bar(I18N.t("section.swap"), m.swap_percent || 0, (m.swap_percent || 0).toFixed(1) + "% · " + fmtGB(m.swap_used || 0) + "/" + fmtGB(m.swap_total || 0) + I18N.t("unit.gb"))
    : "";
  const disks = (Array.isArray(m.disks) ? m.disks : []).filter(d => !isSystemMount(d.path));
  const disksHtml = disks.length
    ? disks.map(d => bar(I18N.t("ui.disk_label") + " " + esc(d.path) + (d.percent >= 90 ? " ⚠" : ""), d.percent, d.percent.toFixed(1) + "% · " + fmtGB(d.used) + "/" + fmtGB(d.total) + I18N.t("unit.gb"))).join("")
    : bar(I18N.t("ui.disk"), m.disk_percent || 0, (m.disk_percent || 0).toFixed(1) + "% · " + fmtGB(m.disk_used || 0) + "/" + fmtGB(m.disk_total || 0) + I18N.t("unit.gb"));
  const gpus = Array.isArray(m.gpus) ? m.gpus : [];
  const gpusHtml = gpus.map(g => {
    const util = Math.max(0, Math.min(g.util_percent || 0, 100));
    const memTxt = (g.mem_total || 0) > 0 ? " · " + I18N.t("ui.gpu_mem_short") + " " + fmtGB(g.mem_used || 0) + "/" + fmtGB(g.mem_total || 0) + I18N.t("unit.gb") : "";
    const tempTxt = (g.temp || 0) > 0 ? " · " + Math.round(g.temp) + "℃" : "";
    const name = esc((g.name || "GPU").slice(0, 22));
    return `<div class="metric gpu"><div class="row"><span class="label">GPU ${name}</span>
      <span class="val mono">${(g.util_percent || 0).toFixed(0)}%${memTxt}${tempTxt}</span></div>
      <div class="bar"><div class="fill" style="width:${util}%;background:${usageColor(g.util_percent || 0)}"></div></div></div>`;
  }).join("");
  let chips = "";
  if (h.custom && Object.keys(h.custom).length) {
    chips = `<div class="chips">` + Object.entries(h.custom).sort().map(([k, v]) => {
      const isDown = /\.up$/.test(k) && v === 0;
      const num = Number.isInteger(v) ? v : v.toFixed(1);
      return `<span class="chip ${isDown ? "crit" : ""}">${esc(k)} <b>${num}</b></span>`;
    }).join("") + `<span class="chip-label">${I18N.t("section.custom_metrics")}</span></div>`;
  }
  const cat = h.category ? esc(h.category) : I18N.t("section.uncategorized");
  const loadTitle = I18N.t("section.load_avg") + (h.os === "windows" ? I18N.t("misc.windows_approx") : "");
  const staleSec = Math.floor(Date.now() / 1000) - (h.last_seen || 0);
  const lastCell = !h.online
    ? `<span class="g offline-tag" title="${I18N.t("section.last_seen")} ${fmtDateTime(h.last_seen)}">⚠ ${I18N.t("ui.offline_status")} ${ago(h.last_seen)}</span>`
    : staleSec > 15
      ? `<span class="g stale-tag" title="${I18N.t("section.data_stale")}，${I18N.t("section.last_seen")} ${fmtDateTime(h.last_seen)}">⚠ ${I18N.t("ui.data")} ${ago(h.last_seen)}</span>`
      : `<span class="g">${I18N.t("ui.running")} ${fmtUptime(m.uptime || 0)}</span>`;
  return `<div class="host ${h.online ? "online" : "offline"}" tabindex="0" data-id="${esc(h.id)}" data-name="${esc(h.hostname || h.id)}" data-cat="${esc(h.category || "")}">
    <div class="host-head">
      <div class="host-name"><span class="dot ${h.online ? "on" : "off"}"></span>
        <div style="min-width:0; flex:1; overflow:hidden">
          <div class="hn" data-act="detail" title="${esc(h.hostname || h.id)}">${esc(h.hostname || h.id)}</div>
          <div class="host-info">
            <div class="hi-row"><span class="hi-k">${I18N.t("section.host_info")}</span><span class="hi-v">${h.ip ? `<span class="mono">${esc(h.ip)}</span>` : "—"}</span></div>
            <div class="hi-row"><span class="hi-k">${I18N.t("section.os")}</span><span class="hi-v" title="${esc(h.platform || "")}${h.arch ? " · " + esc(h.arch) : ""}">${esc(h.platform || "—")}${h.arch ? " <span class=\"hi-sep\">·</span> " + esc(h.arch) : ""}</span></div>
          </div>
        </div>
      </div>
      <div class="host-tags">
        <span class="cat-badge" data-act="cat" title="${I18N.t('section.click_set_category')}">${cat}</span>
        <span class="os-badge">${esc((h.os || "?").toUpperCase())}</span>
        ${(h.online && TERMINAL_ENABLED) ? `<button class="term-btn" data-act="term" title="${I18N.t('section.terminal_desc')}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg></button>` : ""}
        <button class="x-btn" data-act="del" title="${I18N.t("ui.delete")}">✕</button>
      </div>
    </div>
    ${bar("CPU", m.cpu_percent || 0, (m.cpu_percent || 0).toFixed(1) + "% · " + (m.cpu_cores || 0) + I18N.t("ui.cores"))}
    ${bar(I18N.t("ui.memory"), m.mem_percent || 0, (m.mem_percent || 0).toFixed(1) + "% · " + fmtGB(m.mem_used || 0) + "/" + fmtGB(m.mem_total || 0) + I18N.t("unit.gb"))}
    ${swap}
    ${disksHtml}
    ${gpusHtml}
    <div class="loadline" title="${loadTitle}">
      <div class="load-cell"><div class="lv mono">${(m.load1 || 0).toFixed(2)}</div><div class="lk">${I18N.t("section.load_1m")}</div></div>
      <div class="load-cell"><div class="lv mono">${(m.load5 || 0).toFixed(2)}</div><div class="lk">${I18N.t("section.load_5m")}</div></div>
      <div class="load-cell"><div class="lv mono">${(m.load15 || 0).toFixed(2)}</div><div class="lk">${I18N.t("section.load_15m")}</div></div>
    </div>
    ${chips}
    <div class="foot">
      <span class="g">↑<span class="mono">${fmtRate(m.net_sent_rate || 0)}</span> ↓<span class="mono">${fmtRate(m.net_recv_rate || 0)}</span></span>
      <span class="g">💾<span class="mono">${I18N.t("ui.disk_read")} ${fmtIORate(m.disk_read_rate || 0)}</span> <span class="mono">${I18N.t("ui.disk_write")} ${fmtIORate(m.disk_write_rate || 0)}</span></span>
      <span class="g">💿<span class="mono">${fmtIOPS((m.disk_read_iops || 0) + (m.disk_write_iops || 0))} ${I18N.t("unit.iops")}</span></span>
      <span class="g">🔗<span class="mono">${m.net_conns || 0}</span> ${I18N.t("section.connections")}</span>
      <span class="g">📊<span class="mono">${m.proc_count || 0}</span> ${I18N.t("section.processes")}</span>
      ${lastCell}
    </div>
  </div>`;
}

/* ---------- 渲染：主机列表行（列表视图） ---------- */
function hostRow(h) {
  const m = h.latest || {};
  const disks = (Array.isArray(m.disks) ? m.disks : []).filter(d => !isSystemMount(d.path));
  const diskMax = disks.length ? Math.max(...disks.map(d => d.percent)) : (m.disk_percent || 0);
  const gpus = Array.isArray(m.gpus) ? m.gpus : [];
  const gpuMax = gpus.length ? Math.max(...gpus.map(g => g.util_percent || 0)) : null;
  // Mini metric bar: label + progress bar + value
  const miniBar = (label, v) => {
    const pct = Math.max(0, Math.min(v || 0, 100));
    const color = usageColor(v || 0);
    return `<div class="hrow-mbar" title="${label} ${pct.toFixed(1)}%">
      <span class="hm-k">${label}</span>
      <div class="hm-track"><div class="hm-fill" style="width:${pct}%;background:${color}"></div></div>
      <span class="hm-v mono" style="color:${color}">${pct.toFixed(0)}%</span>
    </div>`;
  };
  const staleSec = Math.floor(Date.now() / 1000) - (h.last_seen || 0);
  const isStale = h.online && staleSec > 15;
  const statusCls = !h.online ? "offline" : isStale ? "stale" : "online";
  const last = !h.online
    ? `<span class="hrow-status offline" title="${I18N.t("section.last_seen")} ${fmtDateTime(h.last_seen)}">⚠ ${I18N.t("ui.offline_status")} ${ago(h.last_seen)}</span>`
    : isStale
      ? `<span class="hrow-status stale" title="${I18N.t('section.data_stale')}">⚠ ${ago(h.last_seen)}</span>`
      : `<span class="hrow-status online">${I18N.t("ui.running")} ${fmtUptime(m.uptime || 0)}</span>`;
  const cat = h.category ? esc(h.category) : I18N.t("section.uncategorized");
  const termBtn = (h.online && TERMINAL_ENABLED)
    ? `<button class="term-btn" data-act="term" title="${I18N.t('ui.remote_terminal')}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg></button>`
    : "";
  const loadStr = m.load1 !== undefined ? `${I18N.t("ui.load")} ${(m.load1||0).toFixed(2)} / ${(m.load5||0).toFixed(2)}` : "";
  return `<div class="host hrow ${statusCls}" tabindex="0" data-id="${esc(h.id)}" data-name="${esc(h.hostname || h.id)}" data-cat="${esc(h.category || "")}">
    <span class="hrow-dot ${h.online ? "on" : "off"}"></span>
    <div class="hrow-id">
      <div class="hrow-name" data-act="detail" title="${esc(h.hostname || h.id)}">${esc(h.hostname || h.id)}</div>
      <div class="hrow-sub">${h.ip ? `<span class="mono">${esc(h.ip)}</span>` : ""}${h.platform ? `<span class="hrow-sep">·</span>${esc(h.platform)}` : ""}</div>
    </div>
    <span class="os-badge">${esc((h.os || "?").toUpperCase())}</span>
    <span class="cat-badge" data-act="cat" title="${I18N.t('section.click_set_category')}">${cat}</span>
    <div class="hrow-metrics">
      ${miniBar("CPU", m.cpu_percent)}${miniBar(I18N.t("ui.memory"), m.mem_percent)}${miniBar(I18N.t("ui.disk"), diskMax)}${gpuMax !== null ? miniBar("GPU", gpuMax) : ""}
    </div>
    <span class="hrow-net g">↑<span class="mono">${fmtRate(m.net_sent_rate || 0)}</span> ↓<span class="mono">${fmtRate(m.net_recv_rate || 0)}</span></span>
    ${loadStr ? `<span class="hrow-load mono">${loadStr}</span>` : ""}
    <span class="hrow-last">${last}</span>
    <span class="ch-actions hrow-actions">${termBtn}<button class="mini-btn del" data-act="del" title="${I18N.t("ui.delete")}">✕</button></span>
  </div>`;
}

function renderHosts(hosts) {
  LAST_HOSTS = hosts;
  HOST_META = hosts.map(h => ({ id: h.id, hostname: h.hostname }));
  if (DEFAULT_EMPTY === null) DEFAULT_EMPTY = $("empty").innerHTML;
  $("hostsCount").textContent = hosts.length;
  $("navHosts").textContent = hosts.length;

  // Refresh multi-select category dropdown (preserve current selection)
  const cats = [...new Set(hosts.map(h => h.category || I18N.t("section.uncategorized")))].sort();
  renderCatDropdown(cats);

  // 安全网：仅在首次渲染时检查（LAST_RENDER_KEY 未设置），
  // 防止 localStorage 残留导致页面打开即全隐藏。
  // 用户交互时的折叠操作不受此限制。
  if (!LAST_RENDER_KEY) {
    try {
      const s = localStorage.getItem("aiops_collapsed");
      if (s) {
        const arr = JSON.parse(s);
        if (Array.isArray(arr) && arr.length > 0 && cats.length > 0 && cats.every(c => arr.includes(c))) {
          localStorage.removeItem("aiops_collapsed");
        }
      }
    } catch (e) {}
  }

  const groupsEl = $("groups"), empty = $("empty"), pager = $("pager");
  
  // Filter: multi-category + online status + search
  let shown = hosts.filter(h => {
    if (CUR_CATS.length > 0 && !CUR_CATS.includes(h.category || I18N.t("section.uncategorized"))) return false;
    if (HOST_FILTER === "online" && !h.online) return false;
    if (HOST_FILTER === "offline" && h.online) return false;
    if (HOST_SEARCH) {
      const hay = ((h.hostname || "") + " " + (h.ip || "") + " " + (h.platform || "") + " " + (h.kernel || "") + " " + (h.category || "")).toLowerCase();
      if (!hay.includes(HOST_SEARCH.toLowerCase())) return false;
    }
    return true;
  });
  
  // Sort
  if (HOST_SORT === "cpu") {
    shown.sort((a, b) => (b.latest?.cpu_percent || 0) - (a.latest?.cpu_percent || 0));
  } else if (HOST_SORT === "mem") {
    shown.sort((a, b) => (b.latest?.mem_percent || 0) - (a.latest?.mem_percent || 0));
  } else if (HOST_SORT === "recent") {
    shown.sort((a, b) => (b.last_seen || 0) - (a.last_seen || 0));
  } else {
    shown.sort((a, b) => (a.hostname || a.id).localeCompare(b.hostname || b.id));
  }

  if (!hosts.length) { groupsEl.innerHTML = ""; pager.innerHTML = ""; empty.style.display = "block"; empty.innerHTML = DEFAULT_EMPTY; return; }
  if (!shown.length) { groupsEl.innerHTML = ""; pager.innerHTML = ""; empty.style.display = "block"; empty.textContent = I18N.t("empty.no_host_match"); return; }
  empty.style.display = "none";

  // Pagination: lower threshold on mobile to reduce DOM nodes
  const isList = HOST_VIEW === "list";
  const isMobile = window.innerWidth <= 480;
  const PAGINATION_THRESHOLD = isMobile ? (isList ? 20 : 10) : (isList ? 50 : 30);
  const pageSize = isList ? 50 : HOST_PAGE_SIZE;
  const shouldPaginate = shown.length > PAGINATION_THRESHOLD;
  let pageHosts, pages;
  if (shouldPaginate) {
    pages = Math.ceil(shown.length / pageSize);
    if (HOST_PAGE > pages) HOST_PAGE = pages;
    if (HOST_PAGE < 1) HOST_PAGE = 1;
    pageHosts = shown.slice((HOST_PAGE - 1) * pageSize, HOST_PAGE * pageSize);
  } else {
    HOST_PAGE = 1; pages = 1;
    pageHosts = shown;
  }

  // Group by category on current page
  const byCat = {};
  pageHosts.forEach(h => { const c = h.category || I18N.t("section.uncategorized"); (byCat[c] = byCat[c] || []).push(h); });
  const render = isList ? hostRow : hostCard;
  const wrapCls = isList ? "host-list" : "grid";

  // P0-3: 差量更新 — 如果主机集合未变，仅更新卡片数据而非重建 DOM
  const newKey = pageHosts.map(h => h.id).join(",") + "|" + HOST_VIEW + "|" + HOST_PAGE + "|" + Object.keys(byCat).sort().join(",");
  if (LAST_RENDER_KEY === newKey && Object.keys(HOST_DOM_CACHE).length > 0) {
    pageHosts.forEach(h => updateHostCard(h));
    renderPager(pages, shown.length);
    return;
  }
  LAST_RENDER_KEY = newKey;

  // 折叠功能已临时停用：所有分组始终展开渲染
  groupsEl.innerHTML = Object.keys(byCat).sort().map(cat => {
    return `<div class="group">
      <div class="group-head" data-cat="${esc(cat)}">
        <span class="cat-toggle">▼</span>
        <span class="dot-cat"></span><span class="cat">${esc(cat)}</span>
        <span class="count-pill">${byCat[cat].length}</span><span class="line"></span>
      </div>
      <div class="${wrapCls}">${byCat[cat].map(render).join("")}</div>
    </div>`;
  }).join("");
  buildHostCache();
  renderPager(pages, shown.length);
}

function renderPager(pages, total) {
  const pager = $("pager");
  if (pages <= 1) { pager.innerHTML = `<span class="pinfo">共 ${total} 台</span>`; return; }
  let btns = `<button ${HOST_PAGE === 1 ? "disabled" : ""} data-pg="prev">‹</button>`;
  for (let i = 1; i <= pages; i++) {
    if (i === 1 || i === pages || Math.abs(i - HOST_PAGE) <= 1) {
      btns += `<button class="${i === HOST_PAGE ? "active" : ""}" data-pg="${i}">${i}</button>`;
    } else if (Math.abs(i - HOST_PAGE) === 2) {
      btns += `<span class="pinfo">…</span>`;
    }
  }
  btns += `<button ${HOST_PAGE === pages ? "disabled" : ""} data-pg="next">›</button>`;
  btns += `<span class="pinfo">共 ${total} 台 · ${HOST_PAGE}/${pages} 页</span>`;
  pager.innerHTML = btns;
}

/* ---------- 主机操作 ---------- */
async function delHost(id, name) {
  if (!confirm(`${I18N.t("valid.confirm_delete_host_prefix")}${I18N.t("ui.delete")}「${name}」？\n若该主机 Agent 仍在运行，约 60 ${I18N.t("time.sec")}后会重新出现。`)) return;
  try {
    const r = await fetch(`${API}/hosts/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (r.ok) { toast(I18N.t("toast.host_deleted"), "ok"); refresh(); } else { toast(I18N.t("toast.delete_failed"), "err"); }
  } catch (e) { toast(I18N.t("toast.deleted") + ": " + e, "err"); }
}
async function editCategory(id, cur) {
  const cat = prompt(I18N.t("section.set_category_desc"), cur || "");
  if (cat === null) return;
  try {
    const r = await fetch(`${API}/hosts/${encodeURIComponent(id)}/category`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ category: cat.trim() })
    });
    if (r.ok) { toast(I18N.t("toast.category_updated"), "ok"); refresh(); } else { toast(I18N.t("toast.update_failed2"), "err"); }
  } catch (e) { toast(I18N.t("toast.update_failed") + e, "err"); }
}

/* ---------- 主机趋势弹窗 ---------- */
let DETAIL_HOST_ID = '';
let DETAIL_TIME_RANGE = 1; // hours: 1/3/6/12/24/72/168/336（默认 1 小时）
let DETAIL_CUSTOM = null;   // {from,to} unix seconds — set when a custom range is active

// 把 unix 秒格式化为 <input type="datetime-local"> 需要的本地时间字符串 YYYY-MM-DDTHH:mm
function toLocalDatetimeValue(unixSec) {
  const d = new Date(unixSec * 1000);
  const p = n => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}

// 统一的时间跨度控件渲染函数（主机图表和监控图表共用）
// 快捷时间跨度（小时）：1/3/6/12 小时 + 1/3/7/14 天（+ 自定义，由各视图单独渲染）
const CHART_SPANS = [1, 3, 6, 12, 24, 72, 168, 336];
function chartSpanLabel(h) {
  return h < 24 ? h + I18N.t("time.hour") : (h / 24) + I18N.t("time.day");
}
function renderChartControls(currentRange, prefix) {
  return CHART_SPANS.map(h =>
    `<button class="chip-btn ${currentRange === h ? "active" : ""}" data-${prefix}="${h}">${chartSpanLabel(h)}</button>`
  ).join("");
}
async function openDetail(id, name) {
  DETAIL_HOST_ID = id;
  DETAIL_TIME_RANGE = 1;
  DETAIL_CUSTOM = null;
  $("detailTitle").textContent = name + " " + I18N.t("section.recent_trend");
  const body = $("detailBody");
  body.innerHTML = `<div class="empty-line">${I18N.t("ui.loading")}</div>`;
  $("detailMask").classList.add("show");
  await loadAndRenderCharts();
}

async function loadAndRenderCharts() {
  const body = $("detailBody");
  const now = Math.floor(Date.now() / 1000);
  const to = DETAIL_CUSTOM ? DETAIL_CUSTOM.to : now;
  const from = DETAIL_CUSTOM ? DETAIL_CUSTOM.from : now - DETAIL_TIME_RANGE * 3600;
  const spanH = Math.max(0, (to - from) / 3600); // effective window in hours

  try {
    const samples = await fetch(`${API}/hosts/${encodeURIComponent(DETAIL_HOST_ID)}/history?from=${from}&to=${to}`).then(r => r.json());
    if (!Array.isArray(samples) || !samples.length) {
      body.innerHTML = `<div class="empty-line">${I18N.t("empty.no_history")}</div>`;
      return;
    }

    // 组织图表：每个图表包裹在 .chart-wrap 内，右上角提供放大按钮
    DETAIL_CHARTS = {};
    const gran = spanH <= 2 ? I18N.t("time.raw") : spanH <= 48 ? I18N.t("time.1m_agg") : I18N.t("time.5m_agg");
    const hasGPU = samples.some(s => Array.isArray(s.gpus) && s.gpus.length);
    const hasConns = samples.some(s => Array.isArray(s.conns) && s.conns.length);
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
        ${wrap('chartCPU')}${wrap('chartMem')}${wrap('chartLoad')}${wrap('chartDisk')}${hasGPU ? wrap('chartGPU') + wrap('chartGPUTemp') + wrap('chartGPUMemPct') + wrap('chartGPUMem') : ''}${wrap('chartNet')}${hasConns ? wrap('chartConns') + wrap('chartConnStates') : ''}${wrap('chartDiskIO')}${wrap('chartIOPS')}${wrap('chartProc')}
      </div>
      <div class="hint">${I18N.t("section.sample_points")}: ${samples.length} · ${I18N.t("section.granularity")}: ${gran}</div>
    `;

    DETAIL_CHARTS.chartCPU = createChart('chartCPU', samples,
      [{ key: 'cpu_percent', label: I18N.t("section.cpu_usage"), color: '#4c8dff', fmt: pct }], 0, 100, { title: I18N.t("section.cpu_usage") });
    DETAIL_CHARTS.chartMem = createChart('chartMem', samples,
      [{ key: 'mem_percent', label: I18N.t("section.mem_usage"), color: '#8b5cf6', fmt: pct }], 0, 100, { title: I18N.t("section.mem_usage") });

    // 系统负载组合曲线：load1 / load5 / load15 三条折线同一坐标系
    DETAIL_CHARTS.chartLoad = createChart('chartLoad', samples, [
      { key: 'load1', label: I18N.t("section.load_1m_label"), color: '#4c8dff', fmt: v => v.toFixed(1) },
      { key: 'load5', label: I18N.t("section.load_5m_label"), color: '#f7b23b', fmt: v => v.toFixed(1) },
      { key: 'load15', label: I18N.t("section.load_15m_label"), color: '#f2545b', fmt: v => v.toFixed(1) },
    ], null, null, { title: I18N.t("section.load_avg") });

    // 磁盘：每个分区一条线（按 path 匹配，稳健于分区数/顺序变化）。以「磁盘数最多」的样本
    // 为准建分区列表；并用最近一个含容量的样本给每个分区标注「已用 / 共 / 剩余」明细。
    let diskProto = [];
    samples.forEach(s => { if (Array.isArray(s.disks) && s.disks.length > diskProto.length) diskProto = s.disks; });
    const diskKeys = diskProto.map(d => d.path);
    const latestDisk = {};
    for (let i = samples.length - 1; i >= 0 && Object.keys(latestDisk).length < diskKeys.length; i--) {
      (samples[i].disks || []).forEach(d => { if (!(d.path in latestDisk)) latestDisk[d.path] = d; });
    }
    const _gb = b => b / 1073741824;
    const diskLabel = (path) => {
      const d = latestDisk[path];
      if (!d || !d.total) return '磁盘 ' + path;
      const used = _gb(d.used), tot = _gb(d.total);
      return `磁盘 ${path} · 已用 ${used.toFixed(0)}/${tot.toFixed(0)}GB · 剩 ${(tot - used).toFixed(0)}GB`;
    };
    const diskSeries = diskKeys.map((path, idx) => ({
      key: `disk_${idx}`, label: diskLabel(path),
      color: ['#f7b23b', '#2fd07a', '#f2545b', '#43b6f0', '#8b5cf6', '#e06c9a'][idx % 6], fmt: pct,
      transform: (s) => { const d = (s.disks || []).find(x => x.path === path); return d ? d.percent : null; }
    }));
    DETAIL_CHARTS.chartDisk = createChart('chartDisk', samples,
      diskSeries.length ? diskSeries : [{ key: 'disk_percent', label: I18N.t("section.root_partition"), color: '#f7b23b', fmt: pct }],
      0, 100, { title: I18N.t("section.disk_usage") });

    // GPU：每块显卡一组曲线（存在时才有这些图）。核心算力使用率 / 显卡温度 / 显存占用率 /
    // 显存已用·空闲 —— 覆盖 GPU 深度指标。
    if (hasGPU) {
      const gpuNames = [];
      samples.forEach(s => (s.gpus || []).forEach((g, i) => { if (!gpuNames[i]) gpuNames[i] = g.name || ('GPU' + i); }));
      const gpalette = ['#8b5cf6', '#43b6f0', '#2fd07a', '#f7b23b', '#f2545b', '#e06c9a'];
      const gcolor = idx => gpalette[idx % gpalette.length];
      const gpuVal = (idx, field) => (s) => { const g = s.gpus && s.gpus[idx] ? s.gpus[idx] : null; return g ? (g[field] || 0) : null; };
      const gbUnit = I18N.t("unit.gb");
      const gpuBytesGB = (idx, field) => (s) => { const g = s.gpus && s.gpus[idx] ? s.gpus[idx] : null; return g ? (g[field] || 0) / 1073741824 : null; };

      // 核心算力使用率 %
      DETAIL_CHARTS.chartGPU = createChart('chartGPU', samples, gpuNames.map((nm, idx) => ({
        key: `gpu_${idx}`, label: nm, color: gcolor(idx), fmt: v => v.toFixed(0) + '%', transform: gpuVal(idx, 'util_percent')
      })), 0, 100, { title: I18N.t("section.gpu_usage") });

      // 显卡温度 ℃
      DETAIL_CHARTS.chartGPUTemp = createChart('chartGPUTemp', samples, gpuNames.map((nm, idx) => ({
        key: `gput_${idx}`, label: nm, color: gcolor(idx), fmt: v => v.toFixed(0) + '℃', transform: gpuVal(idx, 'temp')
      })), null, null, { title: I18N.t("section.gpu_temp") });

      // 显存占用率 %
      DETAIL_CHARTS.chartGPUMemPct = createChart('chartGPUMemPct', samples, gpuNames.map((nm, idx) => ({
        key: `gpump_${idx}`, label: nm, color: gcolor(idx), fmt: v => v.toFixed(0) + '%', transform: gpuVal(idx, 'mem_percent')
      })), 0, 100, { title: I18N.t("section.gpu_mem_pct") });

      // 显存已用 / 空闲（GB），每卡两条
      const gpuMemSeries = [];
      gpuNames.forEach((nm, idx) => {
        gpuMemSeries.push({ key: `gpumu_${idx}`, label: `${nm} · ${I18N.t("section.gpu_mem_used")}`, color: gcolor(idx * 2), fmt: v => v.toFixed(1) + gbUnit, transform: gpuBytesGB(idx, 'mem_used') });
        gpuMemSeries.push({ key: `gpumf_${idx}`, label: `${nm} · ${I18N.t("section.gpu_mem_free")}`, color: gcolor(idx * 2 + 1), fmt: v => v.toFixed(1) + gbUnit, transform: gpuBytesGB(idx, 'mem_free') });
      });
      DETAIL_CHARTS.chartGPUMem = createChart('chartGPUMem', samples, gpuMemSeries, null, null, { title: I18N.t("section.gpu_vram") });
    }

    DETAIL_CHARTS.chartNet = createChart('chartNet', samples, [
      { key: 'net_recv_rate', label: I18N.t("section.net_recv"), color: '#2fd07a', fmt: fmtRate },
      { key: 'net_sent_rate', label: I18N.t("section.net_send"), color: '#43b6f0', fmt: fmtRate },
    ], null, null, { title: I18N.t("section.net_throughput") });

    // 连接数（TCP/UDP 总数）+ 会话状态（TCP 各状态一条线）
    if (hasConns) {
      const sumProto = (s, proto) => Array.isArray(s.conns) ? s.conns.reduce((a, c) => c.proto === proto ? a + (c.count || 0) : a, 0) : null;
      DETAIL_CHARTS.chartConns = createChart('chartConns', samples, [
        { key: 'conn_tcp', label: 'TCP', color: '#43b6f0', fmt: v => v.toFixed(0), transform: (s) => sumProto(s, 'tcp') },
        { key: 'conn_udp', label: 'UDP', color: '#2fd07a', fmt: v => v.toFixed(0), transform: (s) => sumProto(s, 'udp') },
      ], null, null, { title: I18N.t("section.conn_count") });

      // 会话状态：收集出现过的 TCP 状态，按常见优先级排序后每状态一条线
      const STATE_ORDER = ['ESTABLISHED', 'TIME_WAIT', 'CLOSE_WAIT', 'LISTEN', 'SYN_SENT', 'SYN_RECV', 'FIN_WAIT1', 'FIN_WAIT2', 'LAST_ACK', 'CLOSING', 'CLOSE', 'OTHER'];
      const stateSet = [];
      samples.forEach(s => (s.conns || []).forEach(c => { if (c.proto === 'tcp' && c.state && !stateSet.includes(c.state)) stateSet.push(c.state); }));
      stateSet.sort((a, b) => { const ia = STATE_ORDER.indexOf(a), ib = STATE_ORDER.indexOf(b); return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib); });
      const stateColors = ['#4c8dff', '#f7b23b', '#f2545b', '#2fd07a', '#8b5cf6', '#43b6f0', '#e06c9a', '#e8a33d', '#6ac4b8', '#c77dff', '#9aa7bd', '#ff8a5b'];
      const stateSeries = stateSet.map((st, idx) => ({
        key: `cst_${idx}`, label: st, color: stateColors[idx % stateColors.length], fmt: v => v.toFixed(0),
        transform: (s) => { if (!Array.isArray(s.conns)) return null; const c = s.conns.find(x => x.proto === 'tcp' && x.state === st); return c ? c.count : 0; }
      }));
      if (stateSeries.length) DETAIL_CHARTS.chartConnStates = createChart('chartConnStates', samples, stateSeries, null, null, { title: I18N.t("section.conn_states") });
    }

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

// 详情弹窗事件委托：放大按钮 + 时间范围切换
safeAddEventListener("detailBody", "click", e => {
  const en = e.target.closest(".chart-enlarge");
  if (en) { const ch = DETAIL_CHARTS[en.dataset.chart]; if (ch) openChartZoom(ch); return; }
  // 自定义时间范围：展开/收起面板
  const tog = e.target.closest("[data-custom-toggle]");
  if (tog) {
    const panel = $("detailCustomPanel");
    if (panel) { panel.hidden = !panel.hidden; if (!panel.hidden) { const f = $("detailCustomFrom"); if (f) f.focus(); } }
    return;
  }
  // 自定义时间范围：应用
  if (e.target.closest("[data-custom-apply]")) { applyDetailCustomRange(); return; }
  const btn = e.target.closest(".chip-btn[data-range]");
  if (!btn) return;
  DETAIL_CUSTOM = null; // 切回预设跨度（相对当前时间）
  DETAIL_TIME_RANGE = parseInt(btn.dataset.range);
  loadAndRenderCharts();
});

// 读取两个 datetime-local 输入，校验后按自定义绝对时间范围重新拉取并渲染
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

/* ---------- Canvas 折线图（交互：悬停十字线 + 数值气泡 / 框选放大 / 双击还原 / 点击放大预览） ---------- */
let DETAIL_CHARTS = {};

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

// smoothPath — 将折线数据点绘制为平滑的二次贝塞尔曲线
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

// drawChartEmpty — 在 Canvas 上绘制空状态插画
function drawChartEmpty(ctx, w, h, message) {
  ctx.clearRect(0, 0, w, h);
  const cssVar = name => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  const txtColor = cssVar("--muted") || "#8a95a8";
  const lineColor = cssVar("--line2") || "#2c3442";
  const cx = w / 2, cy = h / 2;

  // 淡色折线图标轮廓
  ctx.strokeStyle = lineColor; ctx.lineWidth = 1.2; ctx.setLineDash([3, 4]); ctx.lineCap = "round";
  const iconPts = [{x: cx - 50, y: cy + 10}, {x: cx - 18, y: cy - 14}, {x: cx + 14, y: cy + 6}, {x: cx + 46, y: cy - 20}];
  ctx.beginPath(); ctx.moveTo(iconPts[0].x, iconPts[0].y);
  for (let i = 1; i < iconPts.length; i++) ctx.lineTo(iconPts[i].x, iconPts[i].y);
  ctx.stroke(); ctx.setLineDash([]);

  // 数据点
  iconPts.forEach(p => { ctx.fillStyle = lineColor; ctx.beginPath(); ctx.arc(p.x, p.y, 2.5, 0, Math.PI * 2); ctx.fill(); });

  // 居中提示文字
  ctx.fillStyle = txtColor; ctx.font = "13px -apple-system, 'Segoe UI', 'PingFang SC', sans-serif"; ctx.textAlign = "center";
  ctx.fillText(message, cx, cy + 40);
}

// createChart builds an interactive line chart on a canvas and returns its
// state. The state (samples/series/visible-window) lives on canvas._chart so a
// single set of event listeners always drives the current chart.
// sizeChartCanvas makes a canvas crisp on HiDPI screens: the pixel buffer is
// scaled by devicePixelRatio while all chart code keeps working in CSS pixels.
// cssH fixes the display height so a chart looks right at any column width
// (full-width or the two-up grid). Returns the logical {W,H,dpr} to draw within.
function sizeChartCanvas(canvas, cssH) {
  const dpr = Math.min(window.devicePixelRatio || 1, 2); // cap at 2 to bound memory
  const cssW = Math.round(canvas.getBoundingClientRect().width) || 1000;
  canvas.style.height = cssH + "px";
  canvas.width = Math.max(1, Math.round(cssW * dpr));
  canvas.height = Math.max(1, Math.round(cssH * dpr));
  canvas.getContext("2d").setTransform(dpr, 0, 0, dpr, 0, 0);
  return { W: cssW, H: cssH, dpr };
}

// resizeAllCharts re-fits every live chart to its current column width (buffers
// are pinned at creation for HiDPI crispness, so a viewport resize needs a refit).
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

  // 入场动画：首帧绘制后启动渐进揭示
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
  // Draw in CSS pixels; the buffer is dpr-scaled so lines/text are crisp on HiDPI.
  ctx.setTransform(state.dpr || 1, 0, 0, state.dpr || 1, 0, 0);
  const w = state.W || canvas.width, h = state.H || canvas.height;
  const vis = state.all.slice(state.i0, state.i1 + 1);
  const n = vis.length;
  ctx.clearRect(0, 0, w, h);

  // 使用 CSS 变量适配深色/浅色主题
  const cssVar = name => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  const gridColor = cssVar("--line2") || "rgba(43,53,71,.5)";
  const labelColor = cssVar("--muted") || "#8a95a8";
  const txtColor = cssVar("--txt") || "#e8eef6";
  const bgColor = cssVar("--bg") || "#0a0d13";

  // Y range (fixed when yMin/yMax given, else padded auto-range)
  let dMin = state.yMin !== null ? state.yMin : Infinity;
  let dMax = state.yMax !== null ? state.yMax : -Infinity;
  series.forEach(s => vis.forEach(sm => {
    const v = seriesVal(s, sm);
    if (v !== null) { dMin = Math.min(dMin, v); dMax = Math.max(dMax, v); }
  }));
  if (dMin === Infinity) dMin = 0;
  if (dMax === -Infinity) dMax = state.yMax !== null ? state.yMax : 100;
  // 自动范围：对 auto-range 做 8% padding（比原来的 10% 更紧凑）
  if (state.yMin === null) dMin = Math.max(0, dMin * 0.92);
  if (state.yMax === null) dMax = dMax * 1.08 || 1;
  if (dMax <= dMin) dMax = dMin + 1;
  const yRange = dMax - dMin;
  // Dynamic left padding: widen it to fit the Y-axis labels so long values
  // (network rates like "1.45 MB/s", disk IO/GB) are never clipped off the canvas
  // edge — the fixed 56px was too narrow for rate charts.
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

  // 网格 + Y 轴标签（5 条水平线，虚线样式）
  ctx.strokeStyle = gridColor; ctx.lineWidth = 0.5; ctx.setLineDash([2, 4]);
  ctx.font = "10.5px 'SF Mono', 'Cascadia Code', 'JetBrains Mono', Consolas, monospace"; ctx.textAlign = "right";
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (ch / 4) * i;
    ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(w - pad.right, y); ctx.stroke();
    const val = dMax - (yRange / 4) * i;
    ctx.fillStyle = labelColor;
    // 使用第一个 series 的 fmt 格式化 Y 轴标签，确保网络图正确显示速率单位
    const fmt = series[0].fmt;
    const label = fmt ? fmt(val) : val.toFixed(1);
    ctx.fillText(label, pad.left - 8, y + 4);
  }
  ctx.setLineDash([]);

  // X 轴时间标签
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

  // 系列折线 + 渐变填充区域
  series.forEach((s, sIdx) => {
    const pts = [];
    vis.forEach((sm, i) => { const v = seriesVal(s, sm); if (v !== null) pts.push({ x: xAt(i), y: yAt(v), val: v }); });
    if (pts.length >= 2) {
      // 折线路径（数据点 > 12 时使用平滑贝塞尔曲线）
      ctx.save();
      ctx.strokeStyle = s.color; ctx.lineWidth = sIdx === 0 ? 2.2 : 1.8; ctx.lineJoin = "round"; ctx.lineCap = "round";
      if (pts.length > 12) { smoothPath(ctx, pts); } else { ctx.beginPath(); pts.forEach((p, i) => i ? ctx.lineTo(p.x, p.y) : ctx.moveTo(p.x, p.y)); }
      ctx.stroke();
      ctx.restore();

      // 半透明渐变填充区域（4 层渐变停止点，层次更丰富）
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

  // 图例：水平排列在图表右上角区域，带半透明背景
  const legendY = pad.top + 4;
  let legendX = pad.left + 8;
  const legendItemWidth = 160; // 每个图例条目预估宽度

  // 图例分组半透明背景
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

  // 计算背景矩形宽度
  legendLines.forEach(line => {
    legendBgW = Math.max(legendBgW, line.x - legendBgX0);
  });

  // 绘制图例背景
  if (legendLines.length) {
    const bgH = legendLines.length * 18 + 8;
    ctx.fillStyle = cssVar("--panel") + "99" || "rgba(17,22,33,.6)";
    const bgR = 6;
    ctx.beginPath(); ctx.roundRect(legendBgX0 - 4, legendY - 2, legendBgW + 20, bgH, bgR); ctx.fill();
  }

  // 逐行绘制图例条目
  let ly = legendY;
  legendLines.forEach(line => {
    let lx = line.x_start || legendBgX0;
    // reset lx to where this line started
    lx = line.items.length ? line.items[0].x : lx;
    line.items.forEach(item => {
      lx = item.x;
      // 10×10 圆角色块
      ctx.fillStyle = item.color;
      ctx.beginPath(); ctx.roundRect(lx, ly, 10, 10, 3); ctx.fill();
      ctx.fillStyle = txtColor; ctx.font = "10.5px -apple-system, 'Segoe UI', 'PingFang SC', sans-serif"; ctx.textAlign = "left";
      ctx.fillText(item.labelText, lx + 14, ly + 9);
    });
    ly += 18;
  });

  // 框选矩形
  if (state.drag && state.moved && state.downX !== null && state.curX !== null) {
    const x0 = Math.min(state.downX, state.curX), x1 = Math.max(state.downX, state.curX);
    ctx.fillStyle = "rgba(76,141,255,.12)"; ctx.fillRect(x0, pad.top, x1 - x0, ch);
    ctx.strokeStyle = "rgba(76,141,255,.5)"; ctx.lineWidth = 1; ctx.setLineDash([4, 4]); ctx.strokeRect(x0, pad.top, x1 - x0, ch); ctx.setLineDash([]);
  }

  // 十字线（更细、更淡，不干扰数据观察）
  if (state.hover >= state.i0 && state.hover <= state.i1 && !state.drag) {
    const li = state.hover - state.i0, x = xAt(li);
    ctx.strokeStyle = "rgba(200,210,230,.22)"; ctx.lineWidth = 0.8;
    ctx.setLineDash([3, 5]); ctx.beginPath(); ctx.moveTo(x, pad.top); ctx.lineTo(x, pad.top + ch); ctx.stroke(); ctx.setLineDash([]);
    // 悬停数据点（双层光晕 + 白色高光边缘）
    series.forEach(s => {
      const v = seriesVal(s, vis[li]); if (v === null) return;
      const py = yAt(v);
      // 外层光晕（增大半径至 8px）
      ctx.fillStyle = s.color + "25"; ctx.beginPath(); ctx.arc(x, py, 8, 0, Math.PI * 2); ctx.fill();
      // 内层光点
      ctx.fillStyle = s.color; ctx.beginPath(); ctx.arc(x, py, 3.5, 0, Math.PI * 2); ctx.fill();
      // 白色高光边缘
      ctx.strokeStyle = "#fff"; ctx.lineWidth = 1.5;
      ctx.beginPath(); ctx.arc(x, py, 3.5, 0, Math.PI * 2); ctx.stroke();
    });
  }
}

// attachChartEvents wires pointer interaction once per canvas element; handlers
// read the live state from canvas._chart so a persistent canvas (the zoom modal)
// never accumulates duplicate listeners.
function attachChartEvents(canvas) {
  if (canvas._evt) return;
  canvas._evt = true;
  // Map a pointer's clientX into the chart's CSS-pixel coordinate space (state.W),
  // which is what drawChart / pad.left / _cw work in. Using canvas.width (the
  // dpr-scaled backing buffer) here caused the crosshair to be offset by the
  // devicePixelRatio on HiDPI / zoomed displays — hovering snapped to the wrong point.
  const toX = e => {
    const st = canvas._chart;
    const r = canvas.getBoundingClientRect();
    if (!r.width) return 0;
    const W = (st && st.W) || r.width; // CSS-pixel width the chart was drawn with
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

// openChartZoom opens the enlarge modal, re-rendering the source chart on a
// larger canvas that keeps the source's current visible window and stays fully
// interactive (hover / box-zoom / dbl-click reset).
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

