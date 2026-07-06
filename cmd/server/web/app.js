/* ============================================================
   AIOps Monitor · 前端逻辑
   数据源：/api/v1/{summary,hosts,alerts,events,config}
   3 秒轮询；事件委托绑定，避免内联 onclick 的转义隐患。
   ============================================================ */
"use strict";
const API = "/api/v1";

/* 复制到剪贴板（兼容 HTTP 环境） */
function copyToClipboard(text) {
  if (navigator.clipboard && window.isSecureContext) {
    return navigator.clipboard.writeText(text);
  }
  // Fallback: textarea + execCommand
  return new Promise((resolve, reject) => {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.cssText = "position:fixed;left:-9999px;top:-9999px;opacity:0";
    document.body.appendChild(ta);
    ta.select();
    try {
      document.execCommand("copy") ? resolve() : reject(new Error("execCommand failed"));
    } catch (e) { reject(e); }
    finally { document.body.removeChild(ta); }
  });
}
function copyWithFeedback(btn, text, okMsg) {
  copyToClipboard(text).then(
    () => { const old = btn.textContent; btn.textContent = "✓"; toast(okMsg, "ok"); setTimeout(() => btn.textContent = old, 1200); },
    () => toast("复制失败，请手动选择复制", "err")
  );
}
let CUR_CAT = "";     // 当前分类筛选
let LAST_HOSTS = [];  // 最近一次主机数据（供筛选切换时本地重渲染）
let LOG_KIND = "";    // 日志类型筛选（操作/系统/插件）
let LAST_LOG = [];    // 最近一次日志数据
let HOST_SEARCH = ""; // 主机搜索关键词
let HOST_FILTER = "all"; // 主机状态筛选 all|online|offline
let HOST_PAGE = 1;    // 主机分页当前页
const HOST_PAGE_SIZE = 9;
let LAST_CHECKS = []; // 最近一次自定义监控数据
let HOST_META = [];   // 主机元数据（id + hostname）用于进程监控
let DEFAULT_EMPTY = null;
let APP_STARTED = false;

/* ---------- 工具函数 ---------- */
const $ = id => document.getElementById(id);
const esc = s => String(s == null ? "" : s).replace(/[&<>"]/g, c =>
  ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
const fmtRate = b => b < 1024 ? b.toFixed(0) + " B/s"
  : b < 1048576 ? (b / 1024).toFixed(1) + " KB/s"
  : (b / 1048576).toFixed(2) + " MB/s";
const fmtGB = b => (b / 1073741824).toFixed(1);
const fmtUptime = s => {
  const d = Math.floor(s / 86400), h = Math.floor(s % 86400 / 3600), m = Math.floor(s % 3600 / 60);
  return d > 0 ? `${d}天${h}小时` : h > 0 ? `${h}小时${m}分` : `${m}分钟`;
};
const fmtDateTime = ts => {
  const d = new Date(ts * 1000);
  const Y = d.getFullYear();
  const M = String(d.getMonth() + 1).padStart(2, '0');
  const D = String(d.getDate()).padStart(2, '0');
  const h = String(d.getHours()).padStart(2, '0');
  const m = String(d.getMinutes()).padStart(2, '0');
  const s = String(d.getSeconds()).padStart(2, '0');
  return `${Y}-${M}-${D} ${h}:${m}:${s}`;
};
const usageColor = p => p >= 90 ? "var(--crit)" : p >= 80 ? "var(--warn)" : p >= 60 ? "var(--info)" : "var(--ok)";
const ago = ts => {
  const s = Math.max(0, Math.floor(Date.now() / 1000 - ts));
  return s < 60 ? `${s}秒前` : s < 3600 ? `${Math.floor(s / 60)}分钟前` : `${Math.floor(s / 3600)}小时前`;
};

function toast(msg, kind) {
  const t = $("toast");
  t.textContent = msg;
  t.className = "toast show " + (kind || "");
  clearTimeout(t._t);
  t._t = setTimeout(() => (t.className = "toast"), 2800);
}

function icon(name) {
  const p = {
    host: '<path d="M4 4h16v10H4z"/><path d="M2 20h20M8 14v6M16 14v6"/>',
    on:   '<circle cx="12" cy="12" r="9"/><path d="M9 12l2 2 4-4"/>',
    off:  '<circle cx="12" cy="12" r="9"/><path d="M8 12h8"/>',
    crit: '<path d="M12 3 2 20h20z"/><path d="M12 9v5M12 17v.4"/>',
    warn: '<circle cx="12" cy="12" r="9"/><path d="M12 8v5M12 16v.4"/>',
    event:'<path d="M4 5h16v14H4z"/><path d="M4 9h16M9 13h6"/>'
  }[name] || "";
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${p}</svg>`;
}

function bar(label, percent, detail) {
  const p = Math.max(0, Math.min(percent || 0, 100));
  return `<div class="metric"><div class="row"><span class="label">${label}</span><span class="val mono">${detail}</span></div>
    <div class="bar"><div class="fill" style="width:${p}%;background:${usageColor(percent)}"></div></div></div>`;
}

/* ---------- 渲染：KPI ---------- */
function renderCards(s) {
  const card = (cls, ic, v, k, vcls, goto) =>
    `<div class="card ${cls}" data-goto="${goto}" title="点击查看"><div class="ic">${icon(ic)}</div><div class="txt"><div class="v mono ${vcls || ""}">${v}</div><div class="k">${k}</div></div></div>`;
  $("cards").innerHTML =
    card("info", "host", s.total_hosts, "主机总数", "", "hosts:all") +
    card("ok", "on", s.online_hosts, "在线", "ok", "hosts:online") +
    card(s.offline_hosts > 0 ? "crit" : "", "off", s.offline_hosts, "离线", s.offline_hosts > 0 ? "crit" : "", "hosts:offline") +
    card(s.critical_alerts > 0 ? "crit" : "ok", "crit", s.critical_alerts, "严重告警", s.critical_alerts > 0 ? "crit" : "ok", "alerts:") +
    card(s.warning_alerts > 0 ? "warn" : "ok", "warn", s.warning_alerts, "警告", s.warning_alerts > 0 ? "warn" : "ok", "alerts:") +
    card("info", "event", s.plugin_events || 0, "插件发现", s.plugin_events > 0 ? "info" : "", "log:");
}

/* ---------- 渲染：告警 / 事件 ---------- */
function renderAlerts(alerts) {
  const n = alerts.length;
  $("alertsCount").textContent = n; $("navAlerts").textContent = n; $("ovAlertsCount").textContent = n;
  const row = a => `<div class="row-item ${esc(a.level)}">
    <span class="badge ${esc(a.level)}">${a.level === "critical" ? "严重" : "警告"}</span>
    <strong>${esc(a.hostname)}</strong><span class="msg">${esc(a.message)}</span></div>`;
  const empty = `<div class="empty-line">✅ 暂无阈值告警</div>`;
  $("alerts").innerHTML = n ? alerts.map(row).join("") : empty;
  $("ovAlerts").innerHTML = n ? alerts.slice(0, 6).map(row).join("") : empty;
}

function renderLog(items) {
  LAST_LOG = items;
  const n = items.length;
  $("logCount").textContent = n; $("navLog").textContent = n; $("ovLogCount").textContent = n;
  const kcls = k => k === "操作" ? "op" : k === "系统" ? "sys" : "plg";
  const row = e => `<div class="row-item ${esc(e.level)}">
    <span class="kind ${kcls(e.kind)}">${esc(e.kind)}</span>
    <span class="msg">${esc(e.message)}</span>
    <span class="src">${esc(e.actor || "")}${e.host ? " · " + esc(e.host) : ""}</span>
    <span class="log-time mono">${fmtDateTime(e.timestamp)}</span></div>`;
  const filtered = LOG_KIND ? items.filter(e => e.kind === LOG_KIND) : items;
  $("log").innerHTML = filtered.length ? filtered.map(row).join("") : `<div class="empty-line">暂无日志</div>`;
  $("ovLog").innerHTML = n ? items.slice(0, 6).map(row).join("") : `<div class="empty-line">暂无活动</div>`;
}

/* ---------- 渲染：主机卡片 ---------- */
function hostCard(h) {
  const m = h.latest || {};
  const swap = (m.swap_total || 0) > 0
    ? bar("SWAP", m.swap_percent || 0, (m.swap_percent || 0).toFixed(1) + "% · " + fmtGB(m.swap_used || 0) + "/" + fmtGB(m.swap_total || 0) + "G")
    : "";
  const disksHtml = (Array.isArray(m.disks) && m.disks.length)
    ? m.disks.map(d => bar("磁盘 " + esc(d.path), d.percent, d.percent.toFixed(1) + "% · " + fmtGB(d.used) + "/" + fmtGB(d.total) + "G")).join("")
    : bar("磁盘", m.disk_percent || 0, (m.disk_percent || 0).toFixed(1) + "% · " + fmtGB(m.disk_used || 0) + "/" + fmtGB(m.disk_total || 0) + "G");
  let chips = "";
  if (h.custom && Object.keys(h.custom).length) {
    chips = `<div class="chips">` + Object.entries(h.custom).sort().map(([k, v]) => {
      const isDown = /\.up$/.test(k) && v === 0;
      const num = Number.isInteger(v) ? v : v.toFixed(1);
      return `<span class="chip ${isDown ? "crit" : ""}">${esc(k)} <b>${num}</b></span>`;
    }).join("") + `<span class="chip-label">自定义指标（来自插件）</span></div>`;
  }
  const cat = h.category ? esc(h.category) : "未分类";
  const loadTitle = "系统负载 1 / 5 / 15 分钟" + (h.os === "windows" ? "（Windows 为近似值）" : "");
  return `<div class="host" data-id="${esc(h.id)}" data-name="${esc(h.hostname || h.id)}" data-cat="${esc(h.category || "")}">
    <div class="host-head">
      <div class="host-name"><span class="dot ${h.online ? "on" : "off"}"></span>
        <div style="min-width:0">
          <div class="hn" data-act="detail" title="${esc(h.hostname || h.id)}">${esc(h.hostname || h.id)}</div>
          <div class="os">${esc(h.platform || "")}${h.arch ? " · " + esc(h.arch) : ""}</div>
          <div class="meta">${h.ip ? "IP " + esc(h.ip) : ""}${h.kernel ? (h.ip ? " · " : "") + "内核 " + esc(h.kernel) : ""}</div>
        </div>
      </div>
      <div class="host-tags">
        <span class="cat-badge" data-act="cat" title="点击修改分类">${cat}</span>
        <span class="os-badge">${esc((h.os || "?").toUpperCase())}</span>
        <button class="x-btn" data-act="del" title="删除主机">✕</button>
      </div>
    </div>
    ${bar("CPU", m.cpu_percent || 0, (m.cpu_percent || 0).toFixed(1) + "% · " + (m.cpu_cores || 0) + "核")}
    ${bar("内存", m.mem_percent || 0, (m.mem_percent || 0).toFixed(1) + "% · " + fmtGB(m.mem_used || 0) + "/" + fmtGB(m.mem_total || 0) + "G")}
    ${swap}
    ${disksHtml}
    <div class="loadline" title="${loadTitle}">
      <div class="load-cell"><div class="lv mono">${(m.load1 || 0).toFixed(2)}</div><div class="lk">1 min</div></div>
      <div class="load-cell"><div class="lv mono">${(m.load5 || 0).toFixed(2)}</div><div class="lk">5 min</div></div>
      <div class="load-cell"><div class="lv mono">${(m.load15 || 0).toFixed(2)}</div><div class="lk">15 min</div></div>
    </div>
    ${chips}
    <div class="foot">
      <span class="g">↑<span class="mono">${fmtRate(m.net_sent_rate || 0)}</span> ↓<span class="mono">${fmtRate(m.net_recv_rate || 0)}</span></span>
      <span class="g">🔗<span class="mono">${m.net_conns || 0}</span> 连接</span>
      <span class="g">进程 <span class="mono">${m.proc_count || 0}</span></span>
      <span class="g">运行 ${fmtUptime(m.uptime || 0)}</span>
    </div>
  </div>`;
}

function renderHosts(hosts) {
  LAST_HOSTS = hosts;
  if (DEFAULT_EMPTY === null) DEFAULT_EMPTY = $("empty").innerHTML;
  $("hostsCount").textContent = hosts.length;
  $("navHosts").textContent = hosts.length;

  // 刷新分类下拉（保留当前选择）
  const cats = [...new Set(hosts.map(h => h.category || "未分类"))].sort();
  const sel = $("catFilter"), cur = sel.value;
  sel.innerHTML = `<option value="">全部分类</option>` + cats.map(c => `<option value="${esc(c)}">${esc(c)}</option>`).join("");
  sel.value = cats.includes(cur) ? cur : "";
  CUR_CAT = sel.value;

  const groupsEl = $("groups"), empty = $("empty"), pager = $("pager");
  // 过滤：分类 + 在线状态 + 搜索
  const q = HOST_SEARCH.trim().toLowerCase();
  const shown = hosts.filter(h => {
    if (CUR_CAT && (h.category || "未分类") !== CUR_CAT) return false;
    if (HOST_FILTER === "online" && !h.online) return false;
    if (HOST_FILTER === "offline" && h.online) return false;
    if (q) {
      const hay = ((h.hostname || "") + " " + (h.ip || "") + " " + (h.platform || "") + " " + (h.category || "")).toLowerCase();
      if (!hay.includes(q)) return false;
    }
    return true;
  });

  if (!hosts.length) { groupsEl.innerHTML = ""; pager.innerHTML = ""; empty.style.display = "block"; empty.innerHTML = DEFAULT_EMPTY; return; }
  if (!shown.length) { groupsEl.innerHTML = ""; pager.innerHTML = ""; empty.style.display = "block"; empty.textContent = "没有匹配的主机。"; return; }
  empty.style.display = "none";

  // 分页
  const total = shown.length, pages = Math.ceil(total / HOST_PAGE_SIZE);
  if (HOST_PAGE > pages) HOST_PAGE = pages;
  if (HOST_PAGE < 1) HOST_PAGE = 1;
  const pageHosts = shown.slice((HOST_PAGE - 1) * HOST_PAGE_SIZE, HOST_PAGE * HOST_PAGE_SIZE);

  // 当前页按分类分组
  const byCat = {};
  pageHosts.forEach(h => { const c = h.category || "未分类"; (byCat[c] = byCat[c] || []).push(h); });
  groupsEl.innerHTML = Object.keys(byCat).sort().map(cat => `
    <div class="group">
      <div class="group-head"><span class="dot-cat"></span><span class="cat">${esc(cat)}</span>
        <span class="count-pill">${byCat[cat].length}</span><span class="line"></span></div>
      <div class="grid">${byCat[cat].map(hostCard).join("")}</div>
    </div>`).join("");
  renderPager(pages, total);
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
  if (!confirm(`确认删除主机「${name}」？\n若该主机 Agent 仍在运行，约 60 秒后会重新出现。`)) return;
  try {
    const r = await fetch(`${API}/hosts/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (r.ok) { toast("已删除主机", "ok"); refresh(); } else { toast("删除失败", "err"); }
  } catch (e) { toast("删除失败: " + e, "err"); }
}
async function editCategory(id, cur) {
  const cat = prompt("设置主机分类（留空清除；服务端覆盖优先于 Agent 上报）：", cur || "");
  if (cat === null) return;
  try {
    const r = await fetch(`${API}/hosts/${encodeURIComponent(id)}/category`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ category: cat.trim() })
    });
    if (r.ok) { toast("已更新分类", "ok"); refresh(); } else { toast("更新失败", "err"); }
  } catch (e) { toast("更新失败: " + e, "err"); }
}

/* ---------- 主机趋势弹窗 ---------- */
let DETAIL_HOST_ID = '';
let DETAIL_TIME_RANGE = 24; // hours: 24, 48, 72
async function openDetail(id, name) {
  DETAIL_HOST_ID = id;
  DETAIL_TIME_RANGE = 24;
  $("detailTitle").textContent = name + " · 近期趋势";
  const body = $("detailBody");
  body.innerHTML = `<div class="empty-line">加载中…</div>`;
  $("detailMask").classList.add("show");
  await loadAndRenderCharts();
}

async function loadAndRenderCharts() {
  const body = $("detailBody");
  const now = Math.floor(Date.now() / 1000);
  const from = now - DETAIL_TIME_RANGE * 3600;

  try {
    const samples = await fetch(`${API}/hosts/${encodeURIComponent(DETAIL_HOST_ID)}/history?from=${from}&to=${now}`).then(r => r.json());
    if (!Array.isArray(samples) || !samples.length) {
      body.innerHTML = `<div class="empty-line">暂无历史数据（Agent 需运行至少几分钟才会积累数据）</div>`;
      return;
    }

    // Build UI with time range selector and charts
    let html = `
      <div class="chart-controls">
        <button class="chip-btn ${DETAIL_TIME_RANGE === 24 ? 'active' : ''}" data-range="24">24小时</button>
        <button class="chip-btn ${DETAIL_TIME_RANGE === 48 ? 'active' : ''}" data-range="48">48小时</button>
        <button class="chip-btn ${DETAIL_TIME_RANGE === 72 ? 'active' : ''}" data-range="72">72小时</button>
      </div>
      <div class="chart-container">
        <canvas id="chartCPU" width="600" height="180"></canvas>
        <canvas id="chartMem" width="600" height="180"></canvas>
        <canvas id="chartDisk" width="600" height="180"></canvas>
        <canvas id="chartNet" width="600" height="180"></canvas>
      </div>
      <div class="hint">采样点 ${samples.length} 个（自动选择最佳粒度：${DETAIL_TIME_RANGE <= 2 ? '原始精度 (~3s)' : DETAIL_TIME_RANGE <= 48 ? '1分钟聚合' : '5分钟聚合'}）。</div>
    `;
    body.innerHTML = html;

    // Render charts
    renderLineChart('chartCPU', samples, [
      { key: 'cpu_percent', label: 'CPU 使用率 (%)', color: '#3b82f6' },
    ], 0, 100);

    renderLineChart('chartMem', samples, [
      { key: 'mem_percent', label: '内存使用率 (%)', color: '#8b5cf6' },
    ], 0, 100);

    // Disk chart: show all disks
    const diskKeys = samples.length > 0 && samples[0].disks ? samples[0].disks.map(d => d.path) : [];
    const diskSeries = diskKeys.map((path, idx) => ({
      key: `disk_${idx}`,
      label: `磁盘 ${path}`,
      color: ['#f59e0b', '#10b981', '#ef4444', '#06b6d4'][idx % 4],
      transform: (s) => {
        const disk = s.disks && s.disks[idx] ? s.disks[idx] : null;
        return disk ? disk.percent : null;
      }
    }));
    if (diskSeries.length > 0) {
      renderLineChart('chartDisk', samples, diskSeries, 0, 100);
    } else {
      // Fallback to root disk
      renderLineChart('chartDisk', samples, [
        { key: 'disk_percent', label: '根分区使用率 (%)', color: '#f59e0b' },
      ], 0, 100);
    }

    renderLineChart('chartNet', samples, [
      { key: 'net_recv_rate', label: '网络接收', color: '#10b981', fmt: fmtRate },
      { key: 'net_sent_rate', label: '网络发送', color: '#06b6d4', fmt: fmtRate },
    ]);

  } catch (e) {
    body.innerHTML = `<div class="empty-line">加载失败: ${esc(e)}</div>`;
  }
}

// Time range selector event delegation
$("detailBody").addEventListener("click", e => {
  const btn = e.target.closest(".chip-btn[data-range]");
  if (!btn) return;
  DETAIL_TIME_RANGE = parseInt(btn.dataset.range);
  document.querySelectorAll("#detailBody .chip-btn").forEach(b => b.classList.toggle("active", b === btn));
  loadAndRenderCharts();
});

/* ---------- Canvas Chart Rendering ---------- */
function renderLineChart(canvasId, samples, series, yMin = null, yMax = null) {
  const canvas = $(canvasId);
  if (!canvas || !samples.length) return;

  const ctx = canvas.getContext('2d');
  const w = canvas.width;
  const h = canvas.height;
  const padding = { top: 20, right: 60, bottom: 30, left: 50 };
  const chartW = w - padding.left - padding.right;
  const chartH = h - padding.top - padding.bottom;

  // Clear canvas
  ctx.clearRect(0, 0, w, h);

  // Calculate Y range
  let dataMin = yMin !== null ? yMin : Infinity;
  let dataMax = yMax !== null ? yMax : -Infinity;
  series.forEach(s => {
    samples.forEach(sample => {
      let val = s.transform ? s.transform(sample) : sample[s.key];
      if (val !== null && val !== undefined) {
        dataMin = Math.min(dataMin, val);
        dataMax = Math.max(dataMax, val);
      }
    });
  });

  if (dataMin === Infinity) dataMin = 0;
  if (dataMax === -Infinity) dataMax = 100;
  if (yMin === null) dataMin = Math.max(0, dataMin * 0.9);
  if (yMax === null) dataMax = dataMax * 1.1;
  const yRange = dataMax - dataMin || 1;

  // Draw grid lines
  ctx.strokeStyle = '#e5e7eb';
  ctx.lineWidth = 0.5;
  for (let i = 0; i <= 4; i++) {
    const y = padding.top + (chartH / 4) * i;
    ctx.beginPath();
    ctx.moveTo(padding.left, y);
    ctx.lineTo(w - padding.right, y);
    ctx.stroke();

    // Y-axis labels
    const val = dataMax - (yRange / 4) * i;
    ctx.fillStyle = '#6b7280';
    ctx.font = '11px monospace';
    ctx.textAlign = 'right';
    ctx.fillText(series[0].fmt ? series[0].fmt(val) : val.toFixed(1), padding.left - 8, y + 4);
  }

  // X-axis time labels
  const firstTs = samples[0].timestamp;
  const lastTs = samples[samples.length - 1].timestamp;
  const timeSpan = lastTs - firstTs;
  ctx.textAlign = 'center';
  for (let i = 0; i <= 4; i++) {
    const x = padding.left + (chartW / 4) * i;
    const ts = firstTs + (timeSpan / 4) * i;
    const d = new Date(ts * 1000);
    const label = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
    ctx.fillStyle = '#6b7280';
    ctx.fillText(label, x, h - 8);
  }

  // Draw series
  series.forEach((s, sIdx) => {
    const points = [];
    samples.forEach((sample, idx) => {
      let val = s.transform ? s.transform(sample) : sample[s.key];
      if (val === null || val === undefined) return;
      const x = padding.left + (idx / (samples.length - 1)) * chartW;
      const y = padding.top + chartH - ((val - dataMin) / yRange) * chartH;
      points.push({ x, y, val });
    });

    if (points.length < 2) return;

    // Draw line
    ctx.strokeStyle = s.color;
    ctx.lineWidth = 2;
    ctx.lineJoin = 'round';
    ctx.beginPath();
    points.forEach((p, i) => {
      if (i === 0) ctx.moveTo(p.x, p.y);
      else ctx.lineTo(p.x, p.y);
    });
    ctx.stroke();

    // Draw area fill
    ctx.globalAlpha = 0.1;
    ctx.fillStyle = s.color;
    ctx.beginPath();
    ctx.moveTo(points[0].x, padding.top + chartH);
    points.forEach(p => ctx.lineTo(p.x, p.y));
    ctx.lineTo(points[points.length - 1].x, padding.top + chartH);
    ctx.closePath();
    ctx.fill();
    ctx.globalAlpha = 1.0;

    // Legend
    const legendY = padding.top + sIdx * 18;
    ctx.fillStyle = s.color;
    ctx.fillRect(w - padding.right + 8, legendY, 12, 12);
    ctx.fillStyle = '#374151';
    ctx.font = '12px sans-serif';
    ctx.textAlign = 'left';
    ctx.fillText(s.label, w - padding.right + 24, legendY + 11);
  });
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

/* ---------- 告警设置 ---------- */
async function openSettings() {
  try {
    const c = await fetch(`${API}/config`).then(r => r.json());
    const t = c.thresholds || {};
    $("alertsEnabled").checked = !!c.alerts_enabled;
    $("feishuEnabled").checked = !!(c.feishu && c.feishu.enabled);
    $("feishuWebhook").value = (c.feishu && c.feishu.webhook) || "";
    $("dingEnabled").checked = !!(c.dingtalk && c.dingtalk.enabled);
    $("dingWebhook").value = (c.dingtalk && c.dingtalk.webhook) || "";
    $("dingSecret").value = (c.dingtalk && c.dingtalk.secret) || "";
    $("cpuWarn").value = t.cpu_warn; $("cpuCrit").value = t.cpu_crit;
    $("memWarn").value = t.mem_warn; $("memCrit").value = t.mem_crit;
    $("diskWarn").value = t.disk_warn; $("diskCrit").value = t.disk_crit;
    $("offlineSec").value = t.offline_after_sec;
    $("settingsMask").classList.add("show");
  } catch (e) { toast("读取配置失败: " + e, "err"); }
}
function collectSettings() {
  const num = id => parseFloat($(id).value) || 0;
  return {
    alerts_enabled: $("alertsEnabled").checked,
    feishu: { enabled: $("feishuEnabled").checked, webhook: $("feishuWebhook").value.trim() },
    dingtalk: { enabled: $("dingEnabled").checked, webhook: $("dingWebhook").value.trim(), secret: $("dingSecret").value.trim() },
    thresholds: {
      cpu_warn: num("cpuWarn"), cpu_crit: num("cpuCrit"),
      mem_warn: num("memWarn"), mem_crit: num("memCrit"),
      disk_warn: num("diskWarn"), disk_crit: num("diskCrit"),
      offline_after_sec: Math.round(num("offlineSec"))
    }
  };
}
async function saveSettings() {
  try {
    const r = await fetch(`${API}/config`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(collectSettings()) });
    if (r.ok) { toast("配置已保存", "ok"); $("settingsMask").classList.remove("show"); } else { toast("保存失败", "err"); }
  } catch (e) { toast("保存失败: " + e, "err"); }
}
async function testSettings() {
  try {
    const r = await fetch(`${API}/config/test`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(collectSettings()) });
    const j = await r.json();
    if (j.ok) toast("测试消息已发送 ✅", "ok");
    else toast("测试失败: " + (j.errors || []).join("; "), "err");
  } catch (e) { toast("测试失败: " + e, "err"); }
}

/* ---------- 安装 Agent ---------- */
let INSTALL = { server_url: "", token: "" };
let CUR_OS = "linux";
async function openInstall() {
  try {
    INSTALL = await fetch(`${API}/install/info`).then(r => r.json());
    $("installToken").value = INSTALL.token || "";
    renderInstallCmd();
    $("installMask").classList.add("show");
  } catch (e) { toast("读取安装信息失败: " + e, "err"); }
}
function renderInstallCmd() {
  const server = INSTALL.server_url || location.origin;
  const token = INSTALL.token || "";
  const cat = $("installCategory").value.trim();
  const q = "token=" + encodeURIComponent(token) + (cat ? "&category=" + encodeURIComponent(cat) : "");
  let cmd, label, hint;
  if (CUR_OS === "windows") {
    cmd = `irm "${server}/install.ps1?${q}" | iex`;
    label = "PowerShell（管理员）一条命令安装";
    hint = "以管理员身份运行 PowerShell；自动下载到 C:\\aiops-agent 并注册开机自启任务。";
  } else if (CUR_OS === "macos") {
    cmd = `curl -fsSL "${server}/install.sh?${q}" | sh`;
    label = "终端一条命令安装";
    hint = "下载到 /opt/aiops-agent 并后台启动（系统级守护可加 sudo）。";
  } else {
    cmd = `curl -fsSL "${server}/install.sh?${q}" | sudo sh`;
    label = "一条命令安装（root / sudo）";
    hint = "自动下载、注册 systemd 服务并开机自启。";
  }
  $("installCmd").textContent = cmd;
  $("cmdLabel").textContent = label;
  $("cmdHint").textContent = hint;
  $("uninstallCmd").textContent = (CUR_OS === "windows")
    ? `irm "${server}/uninstall.ps1" | iex`
    : `curl -fsSL "${server}/uninstall.sh" | ${CUR_OS === "macos" ? "sh" : "sudo sh"}`;
}
async function resetToken() {
  if (!confirm("重置 Token 后，之前分发的安装命令将失效。确定重置？")) return;
  try {
    const j = await fetch(`${API}/install/reset-token`, { method: "POST" }).then(r => r.json());
    INSTALL.token = j.token; $("installToken").value = j.token; renderInstallCmd();
    toast("Token 已重置", "ok");
  } catch (e) { toast("重置失败: " + e, "err"); }
}

/* ---------- 自定义监控 ---------- */
function renderChecks(checks) {
  LAST_CHECKS = checks;
  const userChecks = checks.filter(c => !c.builtin);
  $("navChecks").textContent = userChecks.filter(c => !c.ok && c.checked_at).length || userChecks.length;
  const grid = $("checksGrid"), empty = $("checksEmpty");
  if (!userChecks.length && !checks.length) { grid.innerHTML = ""; empty.style.display = "block"; return; }
  empty.style.display = "none";
  grid.innerHTML = checks.map(c => {
    const st = !c.enabled ? "unknown" : (c.checked_at ? (c.ok ? "up" : "down") : "unknown");
    const stText = !c.enabled ? "已停用" : (c.checked_at ? (c.ok ? "正常" : "异常") : "待检测");
    const lat = c.checked_at ? ` · ${Math.round(c.latency_ms)}ms` : "";
    const builtin = c.builtin ? ' data-builtin="1"' : "";
    const actions = c.builtin ? '' : `<span class="ch-actions">
          <button class="mini-btn" data-cact="edit" title="编辑">✎</button>
          <button class="mini-btn del" data-cact="del" title="删除">✕</button>
        </span>`;
    const builtinTag = c.builtin ? `<span class="type-badge" style="background:#1a3a2a;color:#5efa9e">内置</span>` : "";
    return `<div class="check-card" data-id="${esc(c.id)}"${builtin}>
      <div class="ch-head">
        <span class="st-dot ${st}"></span>
        <span class="ch-name" title="${esc(c.name)}">${esc(c.name)}</span>
        ${actions}
      </div>
      <div class="ch-target" title="${esc(c.target)}">${esc(c.target)}</div>
      <div class="ch-meta">
        <span class="type-badge">${c.type === "http" ? "HTTP" : c.type === "tcp" ? "TCP" : "进程"}</span>
        ${builtinTag}
        <span>${stText}${lat}</span>
        <span>每 ${c.interval_sec}s</span>
        <span>${c.level === "critical" ? "严重" : "警告"}</span>
        ${c.checked_at ? `<span>${ago(c.checked_at)}</span>` : ""}
      </div>
      ${(!c.ok && c.checked_at) ? `<div class="ch-meta" style="color:#ffb0b4">${esc(c.message)}</div>` : ""}
    </div>`;
  }).join("");
}
async function loadChecks() {
  try { renderChecks(await fetch(`${API}/checks`).then(r => r.json())); } catch (e) { /* ignore */ }
}
async function loadHostsMeta() {
  try { HOST_META = await fetch(`${API}/hosts/meta`).then(r => r.json()); } catch (e) { /* ignore */ }
}
function updateCkTargetLabel() {
  const t = $("ckType").value;
  if (t === "process") {
    $("ckHostField").style.display = "block";
    $("ckTargetLabel").textContent = "进程名称";
    $("ckTarget").placeholder = "如 nginx, mysql, aiops-agent";
  } else {
    $("ckHostField").style.display = "none";
    $("ckTargetLabel").textContent = t === "http" ? "URL 地址" : "主机:端口";
    $("ckTarget").placeholder = t === "http" ? "https://example.com" : "127.0.0.1:3306";
  }
}
function openCheckModal(check) {
  $("checkModalTitle").textContent = check ? "编辑检查" : "添加检查";
  $("ckId").value = check ? check.id : "";
  $("ckName").value = check ? check.name : "";
  $("ckType").value = check ? check.type : "http";
  $("ckTarget").value = check ? check.target : "";
  $("ckInterval").value = check ? check.interval_sec : 30;
  $("ckLevel").value = check ? check.level : "critical";
  $("ckEnabled").checked = check ? check.enabled : true;
  // Populate host select for process type
  populateHostSelect(check);
  updateCkTargetLabel();
  $("checkMask").classList.add("show");
}
function populateHostSelect(check) {
  const sel = $("ckHost");
  sel.innerHTML = `<option value="">-- 选择主机 --</option>` + HOST_META.map(h =>
    `<option value="${esc(h.id)}" ${check && check.target.startsWith(h.id + "/") ? "selected" : ""}>${esc(h.hostname || h.id)}</option>`
  ).join("");
}
async function saveCheck() {
  let target = $("ckTarget").value.trim();
  const type = $("ckType").value;
  if (type === "process") {
    const hostId = $("ckHost").value;
    if (!hostId) { toast("请选择目标主机", "err"); return; }
    if (!target) { toast("请填写进程名称", "err"); return; }
    target = hostId + "/" + target;
  }
  const body = {
    id: $("ckId").value,
    name: $("ckName").value.trim(),
    type: type,
    target: target,
    interval_sec: Math.max(5, parseInt($("ckInterval").value) || 30),
    level: $("ckLevel").value,
    enabled: $("ckEnabled").checked
  };
  if (!body.name || !body.target) { toast("请填写名称和目标", "err"); return; }
  try {
    const r = await fetch(`${API}/checks`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    if (r.ok) { toast("已保存", "ok"); $("checkMask").classList.remove("show"); loadChecks(); }
    else { const j = await r.json(); toast("保存失败: " + (j.error || ""), "err"); }
  } catch (e) { toast("保存失败: " + e, "err"); }
}
async function delCheck(id) {
  if (!confirm("确认删除该监控检查？")) return;
  try {
    const r = await fetch(`${API}/checks/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (r.ok) { toast("已删除", "ok"); loadChecks(); } else { toast("删除失败", "err"); }
  } catch (e) { toast("删除失败: " + e, "err"); }
}

/* ---------- 账户 / 个人信息 ---------- */
function setUser(me) {
  const name = me.display_name || me.username || "用户";
  $("userName").textContent = name;
  $("userAvatar").textContent = (name[0] || "A");
}
async function initAuth() {
  try {
    const r = await fetch(`${API}/me`);
    if (r.ok) { setUser(await r.json()); $("loginView").classList.remove("show"); startApp(); }
    else { $("loginView").classList.add("show"); }
  } catch (e) { $("loginView").classList.add("show"); }
}
function startApp() {
  if (APP_STARTED) return;
  APP_STARTED = true;
  refresh(); loadChecks(); loadHostsMeta();
  setInterval(() => { refresh(); loadChecks(); }, 3000);
}
async function openProfile() {
  try {
    const me = await fetch(`${API}/me`).then(r => r.json());
    $("pfUsername").value = me.username || "";
    $("pfDisplay").value = me.display_name || "";
    $("pfEmail").value = me.email || "";
    $("pfOld").value = ""; $("pfNew").value = "";
    $("profileMask").classList.add("show");
  } catch (e) { toast("读取失败: " + e, "err"); }
}
async function saveProfile() {
  try {
    const r = await fetch(`${API}/profile`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ display_name: $("pfDisplay").value.trim(), email: $("pfEmail").value.trim() })
    });
    if (r.ok) { toast("资料已保存", "ok"); setUser({ display_name: $("pfDisplay").value.trim(), username: $("pfUsername").value }); }
    else toast("保存失败", "err");
  } catch (e) { toast("保存失败: " + e, "err"); }
}
async function changePassword() {
  if (!$("pfOld").value || !$("pfNew").value) { toast("请填写原密码和新密码", "err"); return; }
  try {
    const r = await fetch(`${API}/password`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ old: $("pfOld").value, new: $("pfNew").value })
    });
    const j = await r.json();
    if (r.ok) { toast("密码已修改", "ok"); $("pfOld").value = ""; $("pfNew").value = ""; }
    else toast(j.error || "修改失败", "err");
  } catch (e) { toast("修改失败: " + e, "err"); }
}
async function logout() {
  try { await fetch(`${API}/logout`, { method: "POST" }); } catch (e) {}
  location.reload();
}

/* ---------- 主循环 ---------- */
async function refresh() {
  try {
    const rs = await fetch(`${API}/summary`);
    if (rs.status === 401) { $("loginView").classList.add("show"); return; }
    const s = await rs.json();
    const [hosts, alerts, activity] = await Promise.all([
      fetch(`${API}/hosts`).then(r => r.json()),
      fetch(`${API}/alerts`).then(r => r.json()),
      fetch(`${API}/activity`).then(r => r.json())
    ]);
    renderCards(s); renderAlerts(alerts); renderLog(activity); renderHosts(hosts);
    $("clock").textContent = new Date().toLocaleTimeString("zh-CN");
    $("pulse").className = "pulse";
    loadHostsMeta(); // keep host meta fresh for process-check UI
  } catch (e) {
    $("clock").textContent = "连接失败";
    $("pulse").className = "pulse off";
  }
}

/* ---------- 事件绑定（委托） ---------- */
$("groups").addEventListener("click", e => {
  const host = e.target.closest(".host"); if (!host) return;
  const act = e.target.closest("[data-act]"); if (!act) return;
  const { id, name, cat } = host.dataset;
  if (act.dataset.act === "detail") openDetail(id, name);
  else if (act.dataset.act === "cat") editCategory(id, cat);
  else if (act.dataset.act === "del") delHost(id, name);
});
$("catFilter").addEventListener("change", e => { CUR_CAT = e.target.value; HOST_PAGE = 1; renderHosts(LAST_HOSTS); });
$("settingsBtn").addEventListener("click", openSettings);
$("saveBtn").addEventListener("click", saveSettings);
$("testBtn").addEventListener("click", testSettings);
$("installBtn").addEventListener("click", openInstall);
$("resetTokenBtn").addEventListener("click", resetToken);
$("copyCmdBtn").addEventListener("click", function() {
  copyWithFeedback(this, $("installCmd").textContent, "已复制安装命令");
});
// 点击命令区域本身也可复制
$("installCmd").addEventListener("click", function() {
  const sel = window.getSelection();
  sel.removeAllRanges();
  const range = document.createRange();
  range.selectNodeContents(this);
  sel.addRange(range);
});
$("installCategory").addEventListener("input", renderInstallCmd);
$("osTabs").addEventListener("click", e => {
  const t = e.target.closest(".tab"); if (!t) return;
  CUR_OS = t.dataset.os;
  document.querySelectorAll("#osTabs .tab").forEach(x => x.classList.toggle("active", x === t));
  renderInstallCmd();
});
$("copyUninstallBtn").addEventListener("click", function() {
  copyWithFeedback(this, $("uninstallCmd").textContent, "已复制卸载命令");
});

/* ---------- 侧栏导航：视图切换 + 收起 + 移动抽屉 ---------- */
const navItems = document.querySelectorAll(".nav-item");
function switchView(view) {
  document.querySelectorAll(".view").forEach(v => v.classList.toggle("active", v.id === "view-" + view));
  navItems.forEach(n => {
    const on = n.dataset.view === view;
    n.classList.toggle("active", on);
    if (on) $("pageTitle").textContent = n.querySelector("span").textContent;
  });
  window.scrollTo(0, 0);
}
navItems.forEach(n => n.addEventListener("click", () => {
  switchView(n.dataset.view);
  $("app").classList.remove("nav-open");
}));

// 汉堡：桌面收起/展开侧栏；移动端打开/关闭抽屉
$("menuBtn").addEventListener("click", () => {
  if (window.innerWidth <= 900) $("app").classList.toggle("nav-open");
  else $("app").classList.toggle("collapsed");
});
$("backdrop").addEventListener("click", () => $("app").classList.remove("nav-open"));

// 日志类型筛选
$("logFilter").addEventListener("click", e => {
  const b = e.target.closest(".chip-btn"); if (!b) return;
  LOG_KIND = b.dataset.kind;
  document.querySelectorAll("#logFilter .chip-btn").forEach(x => x.classList.toggle("active", x === b));
  renderLog(LAST_LOG);
});
// 弹窗关闭：点遮罩空白处 或 右上角 ✕
document.querySelectorAll(".mask").forEach(mk => mk.addEventListener("click", e => {
  if (e.target === mk || e.target.closest("[data-close-btn]")) mk.classList.remove("show");
}));
document.addEventListener("keydown", e => {
  if (e.key === "Escape") document.querySelectorAll(".mask.show").forEach(mk => mk.classList.remove("show"));
});

// KPI 卡片点击 → 跳转对应视图（并按需过滤主机）
$("cards").addEventListener("click", e => {
  const c = e.target.closest(".card"); if (!c) return;
  const [view, filter] = (c.dataset.goto || "").split(":");
  if (view === "hosts") { HOST_FILTER = filter || "all"; HOST_PAGE = 1; renderHosts(LAST_HOSTS); }
  if (view) switchView(view);
});
// 主机搜索 + 分页
$("hostSearch").addEventListener("input", e => { HOST_SEARCH = e.target.value; HOST_PAGE = 1; renderHosts(LAST_HOSTS); });
$("pager").addEventListener("click", e => {
  const b = e.target.closest("button[data-pg]"); if (!b) return;
  const pg = b.dataset.pg;
  if (pg === "prev") HOST_PAGE--;
  else if (pg === "next") HOST_PAGE++;
  else HOST_PAGE = parseInt(pg);
  renderHosts(LAST_HOSTS);
});
// 自定义监控
$("addCheckBtn").addEventListener("click", () => openCheckModal(null));
$("ckType").addEventListener("change", updateCkTargetLabel);
$("ckSaveBtn").addEventListener("click", saveCheck);
$("checksGrid").addEventListener("click", e => {
  const card = e.target.closest(".check-card"); if (!card) return;
  if (card.dataset.builtin) return; // built-in check, no actions
  const act = e.target.closest("[data-cact]"); if (!act) return;
  const id = card.dataset.id, check = LAST_CHECKS.find(c => c.id === id);
  if (act.dataset.cact === "edit") openCheckModal(check);
  else if (act.dataset.cact === "del") delCheck(id);
});
// 个人信息
$("profileBtn").addEventListener("click", openProfile);
$("pfSaveBtn").addEventListener("click", saveProfile);
$("pfPwdBtn").addEventListener("click", changePassword);
$("logoutBtn").addEventListener("click", logout);
// 登录
$("loginForm").addEventListener("submit", async e => {
  e.preventDefault();
  $("loginErr").textContent = "";
  try {
    const r = await fetch(`${API}/login`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: $("loginUser").value.trim(), password: $("loginPass").value })
    });
    if (r.ok) { setUser(await fetch(`${API}/me`).then(x => x.json())); $("loginView").classList.remove("show"); startApp(); }
    else { const j = await r.json(); $("loginErr").textContent = j.error || "登录失败"; }
  } catch (err) { $("loginErr").textContent = "登录失败: " + err; }
});

initAuth();
