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
let LOG_LEVEL = "";   // 日志级别筛选
let LOG_TIME_RANGE = "all"; // 日志时间范围
let CHECK_TYPE = "all"; // 监控类型筛选
let HOST_SORT = "name"; // 主机排序方式
let LAST_LOG = [];    // 最近一次日志数据
let HOST_SEARCH = ""; // 主机搜索关键词
let HOST_FILTER = "all"; // 主机状态筛选 all|online|offline
let HOST_PAGE = 1;    // 主机分页当前页
const HOST_PAGE_SIZE = 9;
let LAST_CHECKS = []; // 最近一次自定义监控数据
let HOST_META = [];   // 主机元数据（id + hostname）用于进程监控
let DEFAULT_EMPTY = null;
let APP_STARTED = false;
let PAUSED = false;   // 暂停自动刷新（查看时不跳动）
let LOG_PAGE = 1;     // 日志分页当前页
let LOG_PAGE_SIZE = 50; // 日志每页条数（10/30/50/100）
let CHECK_VIEW = "list"; // 自定义监控视图：list | pill
let TERMINAL_ENABLED = true; // 服务端是否开启远程终端
let TERM_WS = null;   // 当前终端 WebSocket

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
  return s < 60 ? `${s}秒前` : s < 3600 ? `${Math.floor(s / 60)}分钟前` : s < 86400 ? `${Math.floor(s / 3600)}小时前` : `${Math.floor(s / 86400)}天前`;
};
const fmtDur = sec => {
  const s = Math.max(0, Math.floor(sec));
  if (s < 60) return `${s}秒`;
  if (s < 3600) return `${Math.floor(s / 60)}分钟`;
  if (s < 86400) return `${Math.floor(s / 3600)}小时${Math.floor(s % 3600 / 60)}分`;
  return `${Math.floor(s / 86400)}天${Math.floor(s % 86400 / 3600)}小时`;
};
// 与 agent 端一致的系统目录过滤（前端再兜一道，防旧 agent / 持久化历史里残留 /boot、/System 盘）
const isSystemMount = p => {
  p = String(p || "");
  return p === "/boot" || p.startsWith("/boot/") || p === "/System" || p.startsWith("/System/");
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
  // 空态引导 & 版本号
  const ob = $("onboarding");
  if (ob) ob.style.display = s.total_hosts === 0 ? "block" : "none";
  const ver = $("verLabel");
  if (ver && s.version) ver.textContent = "v" + s.version;
  TERMINAL_ENABLED = s.terminal_enabled !== false;
}

/* ---------- 渲染：告警 / 事件 ---------- */
function renderAlerts(alerts) {
  const n = alerts.length;
  $("alertsCount").textContent = n; $("navAlerts").textContent = n; $("ovAlertsCount").textContent = n;
  const now = Math.floor(Date.now() / 1000);
  const row = a => {
    const dur = a.since ? `已持续 ${fmtDur(now - a.since)}` : "";
    return `<div class="row-item ${esc(a.level)}">
    <span class="badge ${esc(a.level)}">${a.level === "critical" ? "严重" : "警告"}</span>
    <strong>${esc(a.hostname)}</strong><span class="msg">${esc(a.message)}</span>
    ${dur ? `<span class="src" title="首次触发 ${fmtDateTime(a.since)}">${dur}</span>` : ""}</div>`;
  };
  const empty = `<div class="empty-line">✅ 暂无告警，一切正常</div>`;
  $("alerts").innerHTML = n ? alerts.map(row).join("") : empty;
  $("ovAlerts").innerHTML = n ? alerts.slice(0, 6).map(row).join("") : empty;
}

/* ---------- 概览：资源 TOP10 ---------- */
function renderTop(hosts) {
  const el = $("topPanels");
  if (!el) return;
  const live = hosts.filter(h => h.latest && h.online);
  if (!live.length) { el.innerHTML = ""; return; }
  const by = f => live.map(h => ({ id: h.id, name: h.hostname || h.id, v: f(h) || 0 }))
    .sort((a, b) => b.v - a.v).slice(0, 10);
  const panel = (title, items) => `<div class="top-panel"><div class="tp-title">${title}</div>` +
    (items.length ? items.map(it => `
      <div class="top-item" data-id="${esc(it.id)}" data-name="${esc(it.name)}" title="点击查看趋势">
        <span class="ti-name">${esc(it.name)}</span>
        <div class="ti-bar"><div class="ti-fill" style="width:${Math.min(it.v, 100)}%;background:${usageColor(it.v)}"></div></div>
        <span class="ti-val mono">${it.v.toFixed(1)}%</span>
      </div>`).join("") : `<div class="empty-line">暂无数据</div>`) + `</div>`;
  const gpuTop = by(h => {
    const gs = h.latest.gpus || [];
    return gs.length ? Math.max(...gs.map(g => g.util_percent || 0)) : 0;
  }).filter(it => it.v > 0);
  el.innerHTML =
    panel("CPU 占用 TOP10", by(h => h.latest.cpu_percent)) +
    panel("内存占用 TOP10", by(h => h.latest.mem_percent)) +
    panel("磁盘占用 TOP10（最高分区）", by(h => {
      const ds = h.latest.disks || [];
      return ds.length ? Math.max(...ds.map(d => d.percent)) : (h.latest.disk_percent || 0);
    })) +
    (gpuTop.length ? panel("GPU 占用 TOP10（最高显卡）", gpuTop) : "");
}

// applyLogFilters mirrors the log view's filter chain (类型/级别/时间/内部自检)，
// 供渲染与 CSV 导出共用，保证导出内容与所见一致。
function applyLogFilters(items) {
  let filtered = items;
  if (LOG_KIND) filtered = filtered.filter(e => e.kind === LOG_KIND);
  if (LOG_LEVEL && LOG_LEVEL !== "all") filtered = filtered.filter(e => e.level === LOG_LEVEL);
  if (LOG_TIME_RANGE && LOG_TIME_RANGE !== "all") {
    const now = Math.floor(Date.now() / 1000);
    const hours = parseInt(LOG_TIME_RANGE);
    filtered = filtered.filter(e => (now - e.timestamp) <= hours * 3600);
  }
  return filtered.filter(e => !(e.actor === "自监控" && e.host === "服务端"));
}

function exportLogsCSV() {
  const rows = applyLogFilters(LAST_LOG);
  if (!rows.length) { toast("当前筛选下没有日志可导出", "err"); return; }
  const escCsv = v => `"${String(v == null ? "" : v).replace(/"/g, '""')}"`;
  const lines = ["时间,类型,级别,来源,主机,内容"];
  rows.forEach(e => lines.push([fmtDateTime(e.timestamp), e.kind, e.level, e.actor || "", e.host || "", e.message].map(escCsv).join(",")));
  const blob = new Blob(["﻿" + lines.join("\r\n")], { type: "text/csv;charset=utf-8" });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = `aiops-logs-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-")}.csv`;
  a.click();
  URL.revokeObjectURL(a.href);
  toast(`已导出 ${rows.length} 条日志`, "ok");
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
  
  const filtered = applyLogFilters(items);
  const total = filtered.length;
  const pages = Math.max(1, Math.ceil(total / LOG_PAGE_SIZE));
  if (LOG_PAGE > pages) LOG_PAGE = pages;
  if (LOG_PAGE < 1) LOG_PAGE = 1;
  const pageItems = filtered.slice((LOG_PAGE - 1) * LOG_PAGE_SIZE, LOG_PAGE * LOG_PAGE_SIZE);
  $("log").innerHTML = pageItems.length ? pageItems.map(row).join("") : `<div class="empty-line">暂无日志</div>`;
  renderLogPager(pages, total);
  $("ovLog").innerHTML = n ? items.slice(0, 6).map(row).join("") : `<div class="empty-line">暂无活动</div>`;
}

function renderLogPager(pages, total) {
  const pager = $("logPager");
  if (!pager) return;
  if (total === 0) { pager.innerHTML = ""; return; }
  if (pages <= 1) { pager.innerHTML = `<span class="pinfo">共 ${total} 条</span>`; return; }
  let btns = `<button ${LOG_PAGE === 1 ? "disabled" : ""} data-lpg="prev">‹</button>`;
  for (let i = 1; i <= pages; i++) {
    if (i === 1 || i === pages || Math.abs(i - LOG_PAGE) <= 1) {
      btns += `<button class="${i === LOG_PAGE ? "active" : ""}" data-lpg="${i}">${i}</button>`;
    } else if (Math.abs(i - LOG_PAGE) === 2) {
      btns += `<span class="pinfo">…</span>`;
    }
  }
  btns += `<button ${LOG_PAGE === pages ? "disabled" : ""} data-lpg="next">›</button>`;
  btns += `<span class="pinfo">共 ${total} 条 · ${LOG_PAGE}/${pages} 页</span>`;
  pager.innerHTML = btns;
}

// 每页条数切换（10/30/50/100）
function setLogPageSize(v) {
  LOG_PAGE_SIZE = parseInt(v) || 50;
  LOG_PAGE = 1;
  renderLog(LAST_LOG);
}

/* ---------- 渲染：主机卡片 ---------- */
function hostCard(h) {
  const m = h.latest || {};
  const swap = (m.swap_total || 0) > 0
    ? bar("SWAP", m.swap_percent || 0, (m.swap_percent || 0).toFixed(1) + "% · " + fmtGB(m.swap_used || 0) + "/" + fmtGB(m.swap_total || 0) + "G")
    : "";
  const disks = (Array.isArray(m.disks) ? m.disks : []).filter(d => !isSystemMount(d.path));
  const disksHtml = disks.length
    ? disks.map(d => bar("磁盘 " + esc(d.path) + (d.percent >= 90 ? " ⚠" : ""), d.percent, d.percent.toFixed(1) + "% · " + fmtGB(d.used) + "/" + fmtGB(d.total) + "G")).join("")
    : bar("磁盘", m.disk_percent || 0, (m.disk_percent || 0).toFixed(1) + "% · " + fmtGB(m.disk_used || 0) + "/" + fmtGB(m.disk_total || 0) + "G");
  const gpus = Array.isArray(m.gpus) ? m.gpus : [];
  const gpusHtml = gpus.map(g => {
    const util = Math.max(0, Math.min(g.util_percent || 0, 100));
    const memTxt = (g.mem_total || 0) > 0 ? " · 显存 " + fmtGB(g.mem_used || 0) + "/" + fmtGB(g.mem_total || 0) + "G" : "";
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
    }).join("") + `<span class="chip-label">自定义指标（来自插件）</span></div>`;
  }
  const cat = h.category ? esc(h.category) : "未分类";
  const loadTitle = "系统负载 1 / 5 / 15 分钟" + (h.os === "windows" ? "（Windows 为近似值）" : "");
  const staleSec = Math.floor(Date.now() / 1000) - (h.last_seen || 0);
  const lastCell = !h.online
    ? `<span class="g offline-tag" title="最后上报 ${fmtDateTime(h.last_seen)}">⚠ 失联 ${ago(h.last_seen)}</span>`
    : staleSec > 15
      ? `<span class="g stale-tag" title="数据可能卡顿，最后上报 ${fmtDateTime(h.last_seen)}">⚠ 数据 ${ago(h.last_seen)}</span>`
      : `<span class="g">运行 ${fmtUptime(m.uptime || 0)}</span>`;
  return `<div class="host ${h.online ? "" : "offline"}" data-id="${esc(h.id)}" data-name="${esc(h.hostname || h.id)}" data-cat="${esc(h.category || "")}">
    <div class="host-head">
      <div class="host-name"><span class="dot ${h.online ? "on" : "off"}"></span>
        <div style="min-width:0">
          <div class="hn" data-act="detail" title="${esc(h.hostname || h.id)}">${esc(h.hostname || h.id)}</div>
          <div class="host-info">
            <div class="hi-row"><span class="hi-k">IP 地址</span><span class="hi-v mono">${h.ip ? esc(h.ip) : "—"}</span></div>
            <div class="hi-row"><span class="hi-k">操作系统</span><span class="hi-v" title="${esc(h.platform || "")}">${esc(h.platform || "—")}${h.arch ? " · " + esc(h.arch) : ""}</span></div>
            <div class="hi-row"><span class="hi-k">内核版本</span><span class="hi-v mono" title="${esc(h.kernel || "")}">${h.kernel ? esc(h.kernel) : "—"}</span></div>
          </div>
        </div>
      </div>
      <div class="host-tags">
        <span class="cat-badge" data-act="cat" title="点击修改分类">${cat}</span>
        <span class="os-badge">${esc((h.os || "?").toUpperCase())}</span>
        ${(h.online && TERMINAL_ENABLED) ? `<button class="term-btn" data-act="term" title="远程终端（经 Agent 反向连接，免开端口）"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg></button>` : ""}
        <button class="x-btn" data-act="del" title="删除主机">✕</button>
      </div>
    </div>
    ${bar("CPU", m.cpu_percent || 0, (m.cpu_percent || 0).toFixed(1) + "% · " + (m.cpu_cores || 0) + "核")}
    ${bar("内存", m.mem_percent || 0, (m.mem_percent || 0).toFixed(1) + "% · " + fmtGB(m.mem_used || 0) + "/" + fmtGB(m.mem_total || 0) + "G")}
    ${swap}
    ${disksHtml}
    ${gpusHtml}
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
      ${lastCell}
    </div>
  </div>`;
}

function renderHosts(hosts) {
  LAST_HOSTS = hosts;
  // 进程监控下拉所需的主机元数据直接从主机列表派生，省掉一条 /hosts/meta 轮询请求
  HOST_META = hosts.map(h => ({ id: h.id, hostname: h.hostname }));
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
  let shown = hosts.filter(h => {
    if (CUR_CAT && (h.category || "未分类") !== CUR_CAT) return false;
    if (HOST_FILTER === "online" && !h.online) return false;
    if (HOST_FILTER === "offline" && h.online) return false;
    if (HOST_SEARCH) {
      const hay = ((h.hostname || "") + " " + (h.ip || "") + " " + (h.platform || "") + " " + (h.kernel || "") + " " + (h.category || "")).toLowerCase();
      if (!hay.includes(HOST_SEARCH.toLowerCase())) return false;
    }
    return true;
  });
  
  // 排序
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

    // 组织图表：每个图表包裹在 .chart-wrap 内，右上角提供放大按钮
    DETAIL_CHARTS = {};
    const gran = DETAIL_TIME_RANGE <= 2 ? '原始精度 (≈5s)' : DETAIL_TIME_RANGE <= 48 ? '1 分钟聚合' : '5 分钟聚合';
    const hasGPU = samples.some(s => Array.isArray(s.gpus) && s.gpus.length);
    const pct = v => v.toFixed(1) + '%';
    const wrap = id => `<div class="chart-wrap"><canvas id="${id}" width="1000" height="230"></canvas>` +
      `<button class="chart-enlarge" data-chart="${id}" title="放大预览"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg></button></div>`;
    body.innerHTML = `
      <div class="chart-controls">
        <button class="chip-btn ${DETAIL_TIME_RANGE === 1 ? 'active' : ''}" data-range="1">1小时</button>
        <button class="chip-btn ${DETAIL_TIME_RANGE === 24 ? 'active' : ''}" data-range="24">24小时</button>
        <button class="chip-btn ${DETAIL_TIME_RANGE === 48 ? 'active' : ''}" data-range="48">48小时</button>
        <button class="chip-btn ${DETAIL_TIME_RANGE === 168 ? 'active' : ''}" data-range="168">7天</button>
      </div>
      <div class="chart-container">
        ${wrap('chartCPU')}${wrap('chartMem')}${wrap('chartDisk')}${hasGPU ? wrap('chartGPU') : ''}${wrap('chartNet')}
      </div>
      <div class="hint">采样点 ${samples.length} 个（粒度：${gran}）· 悬停查看数值 · 拖动框选放大区间 · 双击还原 · 点击图表或右上角按钮放大预览。历史已持久化，重启不丢。</div>
    `;

    DETAIL_CHARTS.chartCPU = createChart('chartCPU', samples,
      [{ key: 'cpu_percent', label: 'CPU 使用率', color: '#4c8dff', fmt: pct }], 0, 100, { title: 'CPU 使用率' });
    DETAIL_CHARTS.chartMem = createChart('chartMem', samples,
      [{ key: 'mem_percent', label: '内存使用率', color: '#8b5cf6', fmt: pct }], 0, 100, { title: '内存使用率' });

    // 磁盘：每个分区一条线。以「磁盘数最多」的样本为准，避免首个样本缺盘时丢失分区曲线
    let diskProto = [];
    samples.forEach(s => { if (Array.isArray(s.disks) && s.disks.length > diskProto.length) diskProto = s.disks; });
    const diskKeys = diskProto.map(d => d.path);
    const diskSeries = diskKeys.map((path, idx) => ({
      key: `disk_${idx}`, label: '磁盘 ' + path,
      color: ['#f7b23b', '#2fd07a', '#f2545b', '#43b6f0'][idx % 4], fmt: pct,
      transform: (s) => { const d = s.disks && s.disks[idx] ? s.disks[idx] : null; return d ? d.percent : null; }
    }));
    DETAIL_CHARTS.chartDisk = createChart('chartDisk', samples,
      diskSeries.length ? diskSeries : [{ key: 'disk_percent', label: '根分区', color: '#f7b23b', fmt: pct }],
      0, 100, { title: '磁盘使用率' });

    // GPU：每块显卡一条线（存在时才有该图）
    if (hasGPU) {
      const gpuNames = [];
      samples.forEach(s => (s.gpus || []).forEach((g, i) => { if (!gpuNames[i]) gpuNames[i] = g.name || ('GPU' + i); }));
      const gpuSeries = gpuNames.map((nm, idx) => ({
        key: `gpu_${idx}`, label: 'GPU ' + nm,
        color: ['#8b5cf6', '#43b6f0', '#2fd07a', '#f7b23b'][idx % 4], fmt: v => v.toFixed(0) + '%',
        transform: (s) => { const g = s.gpus && s.gpus[idx] ? s.gpus[idx] : null; return g ? (g.util_percent || 0) : null; }
      }));
      DETAIL_CHARTS.chartGPU = createChart('chartGPU', samples, gpuSeries, 0, 100, { title: 'GPU 使用率' });
    }

    DETAIL_CHARTS.chartNet = createChart('chartNet', samples, [
      { key: 'net_recv_rate', label: '网络接收', color: '#2fd07a', fmt: fmtRate },
      { key: 'net_sent_rate', label: '网络发送', color: '#43b6f0', fmt: fmtRate },
    ], null, null, { title: '网络吞吐' });

  } catch (e) {
    body.innerHTML = `<div class="empty-line">加载失败: ${esc(e)}</div>`;
  }
}

// 详情弹窗事件委托：放大按钮 + 时间范围切换
safeAddEventListener("detailBody", "click", e => {
  const en = e.target.closest(".chart-enlarge");
  if (en) { const ch = DETAIL_CHARTS[en.dataset.chart]; if (ch) openChartZoom(ch); return; }
  const btn = e.target.closest(".chip-btn[data-range]");
  if (!btn) return;
  DETAIL_TIME_RANGE = parseInt(btn.dataset.range);
  document.querySelectorAll("#detailBody .chip-btn").forEach(b => b.classList.toggle("active", b === btn));
  loadAndRenderCharts();
});

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

// createChart builds an interactive line chart on a canvas and returns its
// state. The state (samples/series/visible-window) lives on canvas._chart so a
// single set of event listeners always drives the current chart.
function createChart(canvasId, allSamples, series, yMin = null, yMax = null, opts = {}) {
  const canvas = $(canvasId);
  if (!canvas || !allSamples || !allSamples.length) return null;
  const state = {
    canvas, ctx: canvas.getContext("2d"),
    all: allSamples, series, yMin, yMax,
    title: opts.title || "", isZoom: !!opts.isZoom,
    i0: 0, i1: allSamples.length - 1,
    hover: -1, drag: false, downX: null, curX: null, moved: false,
    pad: { top: 22, right: 18, bottom: 28, left: 56 },
  };
  canvas._chart = state;
  drawChart(state);
  attachChartEvents(canvas);
  return state;
}

function drawChart(state) {
  const { ctx, canvas, series, pad } = state;
  const w = canvas.width, h = canvas.height;
  const cw = w - pad.left - pad.right, ch = h - pad.top - pad.bottom;
  const vis = state.all.slice(state.i0, state.i1 + 1);
  const n = vis.length;
  ctx.clearRect(0, 0, w, h);

  // Y range (fixed when yMin/yMax given, else padded auto-range)
  let dMin = state.yMin !== null ? state.yMin : Infinity;
  let dMax = state.yMax !== null ? state.yMax : -Infinity;
  series.forEach(s => vis.forEach(sm => {
    const v = seriesVal(s, sm);
    if (v !== null) { dMin = Math.min(dMin, v); dMax = Math.max(dMax, v); }
  }));
  if (dMin === Infinity) dMin = 0;
  if (dMax === -Infinity) dMax = state.yMax !== null ? state.yMax : 100;
  if (state.yMin === null) dMin = Math.max(0, dMin * 0.9);
  if (state.yMax === null) dMax = dMax * 1.1 || 1;
  if (dMax <= dMin) dMax = dMin + 1;
  const yRange = dMax - dMin;
  state.dataMin = dMin; state.dataMax = dMax; state._cw = cw; state._ch = ch; state._n = n;

  const xAt = i => pad.left + (n <= 1 ? 0 : (i / (n - 1)) * cw);
  const yAt = v => pad.top + ch - ((v - dMin) / yRange) * ch;

  // grid + y labels
  ctx.strokeStyle = "rgba(43,53,71,.7)"; ctx.lineWidth = 0.5;
  ctx.font = "11px monospace"; ctx.textAlign = "right";
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (ch / 4) * i;
    ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(w - pad.right, y); ctx.stroke();
    const val = dMax - (yRange / 4) * i;
    ctx.fillStyle = "#8a95a8";
    ctx.fillText(series[0].fmt ? series[0].fmt(val) : val.toFixed(1), pad.left - 8, y + 4);
  }
  // x time labels
  if (n >= 1) {
    const firstTs = vis[0].timestamp, span = vis[n - 1].timestamp - firstTs;
    ctx.textAlign = "center"; ctx.fillStyle = "#8a95a8";
    for (let i = 0; i <= 4; i++) {
      const x = pad.left + (cw / 4) * i;
      const d = new Date((firstTs + (span / 4) * i) * 1000);
      const lab = span > 172800
        ? `${d.getMonth() + 1}/${d.getDate()}`
        : `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
      ctx.fillText(lab, x, h - 9);
    }
  }
  // series lines + area + legend
  series.forEach((s, sIdx) => {
    const pts = [];
    vis.forEach((sm, i) => { const v = seriesVal(s, sm); if (v !== null) pts.push({ x: xAt(i), y: yAt(v), val: v }); });
    if (pts.length >= 2) {
      ctx.strokeStyle = s.color; ctx.lineWidth = 2; ctx.lineJoin = "round";
      ctx.beginPath(); pts.forEach((p, i) => i ? ctx.lineTo(p.x, p.y) : ctx.moveTo(p.x, p.y)); ctx.stroke();
      ctx.globalAlpha = 0.1; ctx.fillStyle = s.color;
      ctx.beginPath(); ctx.moveTo(pts[0].x, pad.top + ch);
      pts.forEach(p => ctx.lineTo(p.x, p.y));
      ctx.lineTo(pts[pts.length - 1].x, pad.top + ch); ctx.closePath(); ctx.fill();
      ctx.globalAlpha = 1;
    }
    const vals = pts.map(p => p.val);
    const cur = vals.length ? vals[vals.length - 1] : 0, peak = vals.length ? Math.max(...vals) : 0;
    const fmtV = v => s.fmt ? s.fmt(v) : v.toFixed(1);
    const ly = pad.top + 6 + sIdx * 17;
    ctx.fillStyle = s.color; ctx.fillRect(pad.left + 8, ly, 10, 10);
    ctx.fillStyle = "#e8eef6"; ctx.font = "11px sans-serif"; ctx.textAlign = "left";
    ctx.fillText(`${s.label}  当前 ${fmtV(cur)} · 峰值 ${fmtV(peak)}`, pad.left + 24, ly + 9);
  });

  // selection rectangle (during box-select drag)
  if (state.drag && state.moved && state.downX !== null && state.curX !== null) {
    const x0 = Math.min(state.downX, state.curX), x1 = Math.max(state.downX, state.curX);
    ctx.fillStyle = "rgba(76,141,255,.16)"; ctx.fillRect(x0, pad.top, x1 - x0, ch);
    ctx.strokeStyle = "rgba(76,141,255,.6)"; ctx.lineWidth = 1; ctx.strokeRect(x0, pad.top, x1 - x0, ch);
  }
  // crosshair + hover markers
  if (state.hover >= state.i0 && state.hover <= state.i1 && !state.drag) {
    const li = state.hover - state.i0, x = xAt(li);
    ctx.strokeStyle = "rgba(200,210,230,.35)"; ctx.lineWidth = 1;
    ctx.setLineDash([4, 4]); ctx.beginPath(); ctx.moveTo(x, pad.top); ctx.lineTo(x, pad.top + ch); ctx.stroke(); ctx.setLineDash([]);
    series.forEach(s => {
      const v = seriesVal(s, vis[li]); if (v === null) return;
      ctx.fillStyle = s.color; ctx.strokeStyle = "#0b0f17"; ctx.lineWidth = 2;
      ctx.beginPath(); ctx.arc(x, yAt(v), 3.5, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
    });
  }
}

// attachChartEvents wires pointer interaction once per canvas element; handlers
// read the live state from canvas._chart so a persistent canvas (the zoom modal)
// never accumulates duplicate listeners.
function attachChartEvents(canvas) {
  if (canvas._evt) return;
  canvas._evt = true;
  const toX = e => { const r = canvas.getBoundingClientRect(); return (e.clientX - r.left) * (canvas.width / r.width); };
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
  $("chartZoomTitle").textContent = (src.title || "趋势") + " · 放大预览";
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

/* ---------- 远程终端（经 Agent 反向通道，免开入站端口） ---------- */
let TERM_RESIZE = null;   // 窗口 resize 监听器引用
function openTerminal(id, name) {
  $("termTitle").textContent = name + " · 远程终端";
  const screen = $("termScreen");
  const vt = makeVT(screen);
  screen._vt = vt;
  setTermStatus("连接中…", "");
  $("termMask").classList.add("show");
  closeTerminalWS(); // 关掉可能残留的上一个会话
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const ws = new WebSocket(`${proto}//${location.host}/api/v1/hosts/${encodeURIComponent(id)}/terminal`);
  ws.binaryType = "arraybuffer";
  TERM_WS = ws;
  const doResize = () => { const s = vt.fit(); if (s && ws.readyState === 1) termResizeSend(ws, s.cols, s.rows); };
  ws.onopen = () => { setTermStatus("已连接", "on"); screen.focus(); requestAnimationFrame(doResize); };
  ws.onmessage = ev => {
    const text = (typeof ev.data === "string") ? ev.data : vt.dec.decode(new Uint8Array(ev.data), { stream: true });
    vt.feed(text);
  };
  ws.onclose = () => { setTermStatus("已断开", "off"); if (TERM_WS === ws) TERM_WS = null; };
  ws.onerror = () => setTermStatus("连接错误", "off");
  screen.onkeydown = e => termKeyDown(e, ws);
  screen.onpaste = e => {
    e.preventDefault();
    const t = (e.clipboardData || window.clipboardData).getData("text");
    if (t) termSend(ws, t);
  };
  if (TERM_RESIZE) window.removeEventListener("resize", TERM_RESIZE);
  TERM_RESIZE = () => doResize();
  window.addEventListener("resize", TERM_RESIZE);
}
// 发送窗口尺寸（帧首字节 'r'，负载 "colsxrows"）→ 服务端 → Agent → PTY
function termResizeSend(ws, cols, rows) {
  if (!ws || ws.readyState !== 1) return;
  const body = new TextEncoder().encode(cols + "x" + rows);
  const framed = new Uint8Array(body.length + 1);
  framed[0] = 0x72; // 'r'
  framed.set(body, 1);
  ws.send(framed);
}
function setTermStatus(txt, cls) {
  const s = $("termStatus"); if (s) { s.textContent = txt; s.className = "term-status" + (cls ? " " + cls : ""); }
}
function closeTerminalWS() { if (TERM_WS) { try { TERM_WS.close(); } catch (e) {} TERM_WS = null; } }
// 发送输入（帧首字节 'i' 标识 input）
function termSend(ws, str) {
  if (!ws || ws.readyState !== 1) return;
  const body = new TextEncoder().encode(str);
  const framed = new Uint8Array(body.length + 1);
  framed[0] = 0x69; // 'i'
  framed.set(body, 1);
  ws.send(framed);
}
function termKeyDown(e, ws) {
  e.stopPropagation(); // 阻止全局 Esc 关弹窗，让 Esc 等按键传给 shell
  const k = e.key;
  const ac = (($("termScreen")._vt || {}).appCursor) ? "\x1bO" : "\x1b["; // 应用光标模式(vim/less…)
  let seq = null;
  if (k === "Enter") seq = "\r";
  else if (k === "Backspace") seq = "\x7f";
  else if (k === "Tab") seq = "\t";
  else if (k === "Escape") seq = "\x1b";
  else if (k === "ArrowUp") seq = ac + "A";
  else if (k === "ArrowDown") seq = ac + "B";
  else if (k === "ArrowRight") seq = ac + "C";
  else if (k === "ArrowLeft") seq = ac + "D";
  else if (k === "Home") seq = "\x1b[H";
  else if (k === "End") seq = "\x1b[F";
  else if (k === "Delete") seq = "\x1b[3~";
  else if (k === "PageUp") seq = "\x1b[5~";
  else if (k === "PageDown") seq = "\x1b[6~";
  else if (e.ctrlKey && k.length === 1) {
    const c = k.toLowerCase().charCodeAt(0);
    if (c >= 97 && c <= 122) seq = String.fromCharCode(c - 96); // Ctrl+A..Z → 0x01..0x1A
  } else if (k.length === 1 && !e.metaKey) seq = k; // 可打印字符
  if (seq !== null) { e.preventDefault(); termSend(ws, seq); }
}
/* ---------- 阶段2：VT100 / xterm 子集终端仿真器 ----------
   支持屏幕缓冲 + 光标寻址(CUP/CUU…)、擦除(ED/EL)、SGR 颜色(16/256/RGB、粗体/下划线/反显)、
   滚动区(DECSTBM)、插入/删除行列、备用屏(?1049)、回滚缓冲，可跑 vim/top 等全屏程序。 */
const VT_PAL = [
  "#2b303b", "#ff6b72", "#4fd483", "#e8b84b", "#5b9bff", "#c88bf0", "#4fc3f0", "#c8ced8",
  "#5a6473", "#ff8f95", "#7ee6a5", "#ffd071", "#82b4ff", "#d9b3f7", "#8fd7f7", "#ffffff"
];
function vt256(n) {
  n = n | 0;
  if (n < 16) return VT_PAL[n] || null;
  if (n < 232) { n -= 16; const r = Math.floor(n / 36), g = Math.floor((n % 36) / 6), b = n % 6; const c = v => v ? 55 + v * 40 : 0; return `rgb(${c(r)},${c(g)},${c(b)})`; }
  const v = 8 + (n - 232) * 10; return `rgb(${v},${v},${v})`;
}
const vtEsc = s => s.replace(/[&<>]/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" }[c]));

function makeVT(screen) {
  const vt = {
    screen, dec: new TextDecoder("utf-8"),
    cols: 80, rows: 24, cx: 0, cy: 0,
    fg: null, bg: null, flags: 0,          // flags: 1 粗体 2 反显 4 下划线 8 弱化
    sCx: 0, sCy: 0, sFg: null, sBg: null, sFlags: 0,
    top: 0, bot: 23, wrapNext: false,
    grid: null, SB_MAX: 2000,
    altActive: false, savedGrid: null, savedPos: null,
    st: 0, parm: "", coll: "",             // 解析状态 0 ground 1 esc 2 csi 3 osc 4 charset 5 osc-st
    cursorVis: true, appCursor: false, raf: 0,
  };
  const clampX = x => Math.max(0, Math.min(vt.cols - 1, x));
  const clampY = y => Math.max(0, Math.min(vt.rows - 1, y));
  const blank = () => ({ c: " ", f: null, b: null, a: 0 });
  const newRow = () => { const r = new Array(vt.cols); for (let i = 0; i < vt.cols; i++) r[i] = blank(); return r; };
  function alloc() { vt.grid = []; for (let y = 0; y < vt.rows; y++) vt.grid.push(newRow()); }

  screen.innerHTML = "";
  const sb = document.createElement("div"); sb.className = "term-sb";
  const lv = document.createElement("div"); lv.className = "term-lv";
  screen.appendChild(sb); screen.appendChild(lv);
  alloc();

  function clearCell(cell) { cell.c = " "; cell.f = null; cell.b = vt.bg; cell.a = 0; }
  function scrollUp(n) {
    for (let i = 0; i < n; i++) {
      const removed = vt.grid.splice(vt.top, 1)[0];
      if (!vt.altActive && vt.top === 0) {
        const div = document.createElement("div"); div.className = "term-row"; div.innerHTML = renderRow(removed, -1);
        sb.appendChild(div);
        while (sb.childElementCount > vt.SB_MAX) sb.removeChild(sb.firstChild);
      }
      vt.grid.splice(vt.bot, 0, newRow());
    }
  }
  function scrollDown(n) { for (let i = 0; i < n; i++) { vt.grid.splice(vt.bot, 1); vt.grid.splice(vt.top, 0, newRow()); } }
  function lineFeed() { if (vt.cy === vt.bot) scrollUp(1); else if (vt.cy < vt.rows - 1) vt.cy++; }
  function revIndex() { if (vt.cy === vt.top) scrollDown(1); else if (vt.cy > 0) vt.cy--; }
  function putChar(ch) {
    if (vt.wrapNext) { vt.cx = 0; lineFeed(); vt.wrapNext = false; }
    const cell = vt.grid[vt.cy][vt.cx];
    cell.c = ch; cell.f = vt.fg; cell.b = vt.bg; cell.a = vt.flags;
    if (vt.cx + 1 >= vt.cols) vt.wrapNext = true; else vt.cx++;
  }
  function eraseInLine(m) {
    const row = vt.grid[vt.cy];
    if (m === 1) { for (let x = 0; x <= vt.cx; x++) clearCell(row[x]); }
    else if (m === 2) { for (let x = 0; x < vt.cols; x++) clearCell(row[x]); }
    else { for (let x = vt.cx; x < vt.cols; x++) clearCell(row[x]); }
  }
  function eraseDisplay(m) {
    if (m === 1) { for (let y = 0; y < vt.cy; y++) for (let x = 0; x < vt.cols; x++) clearCell(vt.grid[y][x]); eraseInLine(1); }
    else if (m === 2 || m === 3) { for (let y = 0; y < vt.rows; y++) for (let x = 0; x < vt.cols; x++) clearCell(vt.grid[y][x]); if (m === 3) sb.innerHTML = ""; }
    else { eraseInLine(0); for (let y = vt.cy + 1; y < vt.rows; y++) for (let x = 0; x < vt.cols; x++) clearCell(vt.grid[y][x]); }
  }
  function saveCursor() { vt.sCx = vt.cx; vt.sCy = vt.cy; vt.sFg = vt.fg; vt.sBg = vt.bg; vt.sFlags = vt.flags; }
  function restoreCursor() { vt.cx = clampX(vt.sCx); vt.cy = clampY(vt.sCy); vt.fg = vt.sFg; vt.bg = vt.sBg; vt.flags = vt.sFlags; }
  function enterAlt() { if (vt.altActive) return; vt.altActive = true; vt.savedGrid = vt.grid; vt.savedPos = { x: vt.cx, y: vt.cy }; alloc(); vt.cx = 0; vt.cy = 0; sb.style.display = "none"; }
  function exitAlt() { if (!vt.altActive) return; vt.altActive = false; vt.grid = vt.savedGrid; if (vt.savedPos) { vt.cx = clampX(vt.savedPos.x); vt.cy = clampY(vt.savedPos.y); } vt.top = 0; vt.bot = vt.rows - 1; sb.style.display = ""; }
  function fullReset() { vt.fg = vt.bg = null; vt.flags = 0; vt.top = 0; vt.bot = vt.rows - 1; if (vt.altActive) exitAlt(); alloc(); vt.cx = vt.cy = 0; vt.wrapNext = false; }

  function sgrExt(ps, i, isFg) {
    const mode = ps[i + 1]; let color = null, used = i;
    if (mode === 5) { color = vt256(ps[i + 2] || 0); used = i + 2; }
    else if (mode === 2) { color = `rgb(${ps[i + 2] || 0},${ps[i + 3] || 0},${ps[i + 4] || 0})`; used = i + 4; }
    if (color !== null) { if (isFg) vt.fg = color; else vt.bg = color; }
    return used;
  }
  function sgr(ps) {
    if (!ps.length) ps = [0];
    for (let i = 0; i < ps.length; i++) {
      const n = ps[i];
      if (n === 0) { vt.fg = vt.bg = null; vt.flags = 0; }
      else if (n === 1) vt.flags |= 1; else if (n === 2) vt.flags |= 8;
      else if (n === 4) vt.flags |= 4; else if (n === 7) vt.flags |= 2;
      else if (n === 22) vt.flags &= ~9; else if (n === 24) vt.flags &= ~4; else if (n === 27) vt.flags &= ~2;
      else if (n >= 30 && n <= 37) vt.fg = VT_PAL[n - 30];
      else if (n === 38) i = sgrExt(ps, i, true);
      else if (n === 39) vt.fg = null;
      else if (n >= 40 && n <= 47) vt.bg = VT_PAL[n - 40];
      else if (n === 48) i = sgrExt(ps, i, false);
      else if (n === 49) vt.bg = null;
      else if (n >= 90 && n <= 97) vt.fg = VT_PAL[8 + n - 90];
      else if (n >= 100 && n <= 107) vt.bg = VT_PAL[8 + n - 100];
    }
  }
  function setMode(ps, priv, on) {
    if (!priv) return;
    for (const n of ps) {
      if (n === 25) vt.cursorVis = on;
      else if (n === 1) vt.appCursor = on;
      else if (n === 47 || n === 1047 || n === 1049) { on ? enterAlt() : exitAlt(); }
    }
  }
  function csi(f) {
    const priv = vt.coll.indexOf("?") >= 0;
    const ps = vt.parm.split(";").map(x => x === "" ? 0 : parseInt(x, 10) || 0);
    const p0 = ps[0] || 0, row = () => vt.grid[vt.cy];
    switch (f) {
      case "A": vt.cy = Math.max(vt.top, vt.cy - Math.max(1, p0)); break;
      case "B": vt.cy = Math.min(vt.bot, vt.cy + Math.max(1, p0)); break;
      case "C": vt.cx = Math.min(vt.cols - 1, vt.cx + Math.max(1, p0)); vt.wrapNext = false; break;
      case "D": vt.cx = Math.max(0, vt.cx - Math.max(1, p0)); vt.wrapNext = false; break;
      case "E": vt.cx = 0; vt.cy = Math.min(vt.bot, vt.cy + Math.max(1, p0)); break;
      case "F": vt.cx = 0; vt.cy = Math.max(vt.top, vt.cy - Math.max(1, p0)); break;
      case "G": case "`": vt.cx = clampX((p0 || 1) - 1); vt.wrapNext = false; break;
      case "d": vt.cy = clampY((p0 || 1) - 1); break;
      case "H": case "f": vt.cy = clampY((ps[0] || 1) - 1); vt.cx = clampX((ps[1] || 1) - 1); vt.wrapNext = false; break;
      case "J": eraseDisplay(p0); break;
      case "K": eraseInLine(p0); break;
      case "m": sgr(ps); break;
      case "r": { const t = (ps[0] || 1) - 1, b = (ps[1] || vt.rows) - 1; if (t < b) { vt.top = clampY(t); vt.bot = clampY(b); vt.cx = 0; vt.cy = vt.top; } break; }
      case "s": saveCursor(); break;
      case "u": restoreCursor(); break;
      case "L": if (vt.cy >= vt.top && vt.cy <= vt.bot) for (let i = 0; i < Math.max(1, p0); i++) { vt.grid.splice(vt.bot, 1); vt.grid.splice(vt.cy, 0, newRow()); } break;
      case "M": if (vt.cy >= vt.top && vt.cy <= vt.bot) for (let i = 0; i < Math.max(1, p0); i++) { vt.grid.splice(vt.cy, 1); vt.grid.splice(vt.bot, 0, newRow()); } break;
      case "P": { const r = row(); for (let i = 0; i < Math.max(1, p0); i++) { r.splice(vt.cx, 1); r.push(blank()); } break; }
      case "@": { const r = row(); for (let i = 0; i < Math.max(1, p0); i++) { r.splice(vt.cx, 0, blank()); r.pop(); } break; }
      case "X": { const r = row(); for (let x = vt.cx; x < Math.min(vt.cols, vt.cx + Math.max(1, p0)); x++) clearCell(r[x]); break; }
      case "S": scrollUp(Math.max(1, p0)); break;
      case "T": scrollDown(Math.max(1, p0)); break;
      case "h": setMode(ps, priv, true); break;
      case "l": setMode(ps, priv, false); break;
    }
  }
  vt.feed = function (text) {
    for (let i = 0; i < text.length; i++) {
      const ch = text[i], code = text.charCodeAt(i);
      if (vt.st === 0) {
        if (code === 0x1b) { vt.st = 1; vt.parm = ""; vt.coll = ""; }
        else if (ch === "\r") { vt.cx = 0; vt.wrapNext = false; }
        else if (code === 10 || code === 11 || code === 12) lineFeed();
        else if (code === 8) { vt.cx = Math.max(0, vt.cx - 1); vt.wrapNext = false; }
        else if (code === 9) vt.cx = Math.min(vt.cols - 1, vt.cx - (vt.cx % 8) + 8);
        else if (code === 7) { /* BEL */ }
        else if (code >= 32) putChar(ch);
      } else if (vt.st === 1) {
        if (ch === "[") { vt.st = 2; vt.parm = ""; vt.coll = ""; }
        else if (ch === "]") { vt.st = 3; }
        else if (ch === "(" || ch === ")" || ch === "*" || ch === "+") vt.st = 4;
        else { if (ch === "M") revIndex(); else if (ch === "D") lineFeed(); else if (ch === "E") { vt.cx = 0; lineFeed(); } else if (ch === "7") saveCursor(); else if (ch === "8") restoreCursor(); else if (ch === "c") fullReset(); vt.st = 0; }
      } else if (vt.st === 2) {
        if (code >= 0x40 && code <= 0x7e) { csi(ch); vt.st = 0; }
        else if (ch === "?" || ch === ">" || ch === "=" || ch === "!") vt.coll += ch;
        else vt.parm += ch;
      } else if (vt.st === 3) { if (code === 7) vt.st = 0; else if (code === 0x1b) vt.st = 5; }
      else if (vt.st === 4) vt.st = 0;
      else if (vt.st === 5) vt.st = 0;
    }
    scheduleRender();
  };

  function cellStyle(cell) {
    let f = cell.f, b = cell.b; const a = cell.a;
    if (a & 2) { const t = f; f = b || "#05070b"; b = t || "#d6dde8"; }
    let s = "";
    if (f) s += "color:" + f + ";";
    if (b) s += "background:" + b + ";";
    if (a & 1) s += "font-weight:600;";
    if (a & 8) s += "opacity:.7;";
    if (a & 4) s += "text-decoration:underline;";
    return s;
  }
  function renderRow(rowCells, cursorX) {
    let end = -1;
    for (let x = rowCells.length - 1; x >= 0; x--) { const c = rowCells[x]; if (c.c !== " " || c.f || c.b || c.a) { end = x; break; } }
    if (cursorX >= 0 && cursorX > end) end = cursorX;
    let html = "", run = "", style = null;
    const flush = () => { if (run !== "") { html += style ? `<span style="${style}">${vtEsc(run)}</span>` : vtEsc(run); run = ""; } };
    for (let x = 0; x <= end; x++) {
      const cell = rowCells[x];
      if (x === cursorX) { flush(); style = null; html += `<span class="term-cursor">${vtEsc(cell.c === " " ? " " : cell.c)}</span>`; continue; }
      const st = cellStyle(cell);
      if (st !== style) { flush(); style = st; }
      run += cell.c;
    }
    flush();
    return html;
  }
  function render() {
    const focused = document.activeElement === screen;
    let html = "";
    for (let y = 0; y < vt.rows; y++) {
      const cx = (vt.cursorVis && focused && y === vt.cy) ? vt.cx : -1;
      html += `<div class="term-row">${renderRow(vt.grid[y], cx)}</div>`;
    }
    lv.innerHTML = html;
    screen.scrollTop = screen.scrollHeight;
  }
  function scheduleRender() {
    if (vt.pending) return;
    vt.pending = true;
    const run = () => { if (!vt.pending) return; vt.pending = false; render(); };
    requestAnimationFrame(run);       // 可见标签页：随帧渲染，流畅
    setTimeout(run, 120);             // 兜底：后台标签页 rAF 被暂停时仍能渲染
  }

  vt.fit = function () {
    const probe = document.createElement("span");
    probe.textContent = "MMMMMMMMMMMMMMMMMMMM";
    probe.style.cssText = "position:absolute;visibility:hidden;white-space:pre;left:-9999px";
    lv.appendChild(probe);
    const rect = probe.getBoundingClientRect();
    const cw = rect.width / 20, chh = rect.height;
    lv.removeChild(probe);
    if (!cw || !chh) return null;
    const cs = getComputedStyle(screen);
    const padX = parseFloat(cs.paddingLeft) + parseFloat(cs.paddingRight);
    const padY = parseFloat(cs.paddingTop) + parseFloat(cs.paddingBottom);
    const cols = Math.max(20, Math.floor((screen.clientWidth - padX) / cw));
    const rows = Math.max(6, Math.floor((screen.clientHeight - padY) / chh));
    if (cols !== vt.cols || rows !== vt.rows) {
      const old = vt.grid; vt.cols = cols; vt.rows = rows; vt.grid = [];
      for (let y = 0; y < rows; y++) { const r = newRow(); if (old && old[y]) for (let x = 0; x < Math.min(cols, old[y].length); x++) r[x] = old[y][x]; vt.grid.push(r); }
      vt.top = 0; vt.bot = rows - 1; vt.cx = clampX(vt.cx); vt.cy = clampY(vt.cy); vt.wrapNext = false;
      scheduleRender();
    }
    return { cols, rows };
  };
  return vt;
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
    label = "PowerShell 一条命令安装（无需管理员）";
    hint = "普通 PowerShell 即可；安装到 %LOCALAPPDATA%\\aiops-agent 并注册用户级开机自启。";
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
// 进程类目标形如 hostID/进程名，展示为「进程 @ 主机名」更友好。
function checkTargetDisplay(c) {
  if (c.type === "process") {
    const i = c.target.indexOf("/");
    if (i > 0) {
      const hid = c.target.slice(0, i), pname = c.target.slice(i + 1);
      const meta = HOST_META.find(h => h.id === hid);
      return pname + " @ " + (meta ? meta.hostname || hid.slice(0, 8) : hid.slice(0, 8));
    }
  }
  return c.target;
}
// TCP 目标拆分为 主机 / 端口（末个冒号分隔）
function splitHostPort(t) {
  t = String(t || "");
  const i = t.lastIndexOf(":");
  if (i > 0) return { host: t.slice(0, i), port: t.slice(i + 1) };
  return { host: t, port: "" };
}
// 进程目标 hostID/进程名 拆分，并把 hostID 解析为主机名
function splitProcessTarget(c) {
  const t = String(c.target || "");
  const i = t.indexOf("/");
  if (i > 0) {
    const hid = t.slice(0, i), proc = t.slice(i + 1);
    const meta = HOST_META.find(h => h.id === hid);
    return { proc, hostName: meta ? (meta.hostname || hid.slice(0, 8)) : hid.slice(0, 8) };
  }
  return { proc: t, hostName: "—" };
}
// 详情项：键 + 值 + 值配色
function cdItem(k, v, cls) {
  return `<div class="cd-item"><div class="cd-k">${k}</div><div class="cd-v ${cls || ""}" title="${esc(v)}">${esc(v)}</div></div>`;
}
function renderChecks(checks) {
  LAST_CHECKS = checks;
  const userChecks = checks.filter(c => !c.builtin);
  $("navChecks").textContent = userChecks.filter(c => !c.ok && c.checked_at).length || userChecks.length;
  const grid = $("checksGrid"), empty = $("checksEmpty");
  grid.className = "checks-list" + (CHECK_VIEW === "pill" ? " pill" : "");
  if (!userChecks.length && !checks.length) { grid.innerHTML = ""; empty.style.display = "block"; return; }
  empty.style.display = "none";

  // 应用类型筛选
  let shown = checks;
  if (CHECK_TYPE && CHECK_TYPE !== "all") shown = shown.filter(c => c.type === CHECK_TYPE);

  grid.innerHTML = shown.map(c => {
    const st = !c.enabled ? "unknown" : (c.checked_at ? (c.ok ? "up" : "down") : "unknown");
    const stText = !c.enabled ? "已停用" : (c.checked_at ? (c.ok ? "正常" : "异常") : "待检测");
    const typeText = c.type === "http" ? "HTTP" : c.type === "tcp" ? "TCP" : c.type === "ping" ? "Ping" : "进程";
    const builtin = c.builtin ? ' data-builtin="1"' : "";
    const histBtn = `<button class="mini-btn" data-cact="hist" title="历史曲线"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 3v18h18"/><path d="M7 13l3-3 3 2 5-6"/></svg></button>`;
    const actions = `<span class="ch-actions">${histBtn}${c.builtin ? "" : `
          <button class="mini-btn" data-cact="run" title="立即检测">▶</button>
          <button class="mini-btn" data-cact="edit" title="编辑">✎</button>
          <button class="mini-btn del" data-cact="del" title="删除">✕</button>`}</span>`;
    const builtinTag = c.builtin ? `<span class="type-badge" style="background:#1a3a2a;color:#5efa9e">内置</span>` : "";

    // 详情字段：按监控类型给出各自贴合的字段，三类监控信息量对齐
    const stCls = st === "up" ? "ok" : st === "down" ? "crit" : "muted";
    const lat = c.checked_at ? Math.round(c.latency_ms) + " ms" : "—";
    const latCls = c.checked_at ? "" : "muted";
    const detail = [];
    if (c.type === "http") {
      detail.push(cdItem("监控地址", checkTargetDisplay(c), "muted"));
      detail.push(cdItem("运行状态", stText, stCls));
      const code = c.status_code || 0;
      detail.push(cdItem("状态码", code ? String(code) : "—", code === 0 ? "muted" : code >= 400 ? "crit" : "ok"));
      detail.push(cdItem("响应延时", lat, latCls));
      if (typeof c.cert_days === "number" && c.cert_days >= 0) {
        const d = c.cert_days;
        detail.push(cdItem("证书剩余", d + " 天", d <= 7 ? "crit" : d <= 30 ? "warn" : "ok"));
      }
    } else if (c.type === "tcp") {
      const hp = splitHostPort(c.target);
      detail.push(cdItem("目标主机", hp.host || c.target, "muted"));
      detail.push(cdItem("端口", hp.port || "—", ""));
      detail.push(cdItem("连通状态", stText, stCls));
      detail.push(cdItem("连接延时", lat, latCls));
    } else if (c.type === "ping") {
      detail.push(cdItem("监控地址", c.target, "muted"));
      detail.push(cdItem("运行状态", stText, stCls));
      const loss = (typeof c.loss_pct === "number" && c.loss_pct >= 0) ? c.loss_pct : null;
      detail.push(cdItem("丢包率", loss === null ? "—" : Math.round(loss) + "%",
        loss === null ? "muted" : loss === 0 ? "ok" : loss >= 100 ? "crit" : "warn"));
      const hasRtt = c.checked_at && c.latency_ms > 0;
      detail.push(cdItem("平均延时", hasRtt ? Math.round(c.latency_ms) + " ms" : "—", hasRtt ? "" : "muted"));
    } else if (c.type === "process") {
      const pr = splitProcessTarget(c);
      detail.push(cdItem("进程名", pr.proc, ""));
      detail.push(cdItem("所在主机", pr.hostName, "muted"));
      detail.push(cdItem("运行状态", stText, stCls));
      detail.push(cdItem("检测耗时", lat, latCls));
    } else {
      detail.push(cdItem("监控地址", checkTargetDisplay(c), "muted"));
      detail.push(cdItem("运行状态", stText, stCls));
      detail.push(cdItem("延时", lat, latCls));
    }
    detail.push(cdItem("检测周期", "每 " + c.interval_sec + "s", "muted"));
    detail.push(cdItem("最近检测", c.checked_at ? ago(c.checked_at) : "尚未检测", "muted"));

    return `<div class="check-card" data-id="${esc(c.id)}"${builtin}>
      <div class="check-row-top">
        <span class="st-dot ${st}"></span>
        <span class="ch-name" title="${esc(c.name)}">${esc(c.name)}</span>
        <span class="type-badge">${typeText}</span>
        ${builtinTag}
        <span class="st-pill ${st}">${stText}</span>
        ${actions}
      </div>
      <div class="check-detail">${detail.join("")}</div>
      ${(!c.ok && c.checked_at) ? `<div class="check-err">${esc(c.message)}</div>` : ""}
    </div>`;
  }).join("");
}
// 列表 / 胶囊视图切换
function setCheckView(v) {
  CHECK_VIEW = v === "pill" ? "pill" : "list";
  try { localStorage.setItem("aiops_check_view", CHECK_VIEW); } catch (e) {}
  document.querySelectorAll("#checkViewToggle .vt-btn").forEach(b => b.classList.toggle("active", b.dataset.cview === CHECK_VIEW));
  renderChecks(LAST_CHECKS);
}
async function loadChecks() {
  try { renderChecks(await fetch(`${API}/checks`).then(r => r.json())); } catch (e) { /* ignore */ }
}

let CHK_CHARTS = {};
// 自定义监控·历史曲线：复用交互式图表引擎（悬停十字线 / 框选放大 / 双击还原 / 放大预览）
async function openCheckHistory(id, name, type) {
  const body = $("checkHistBody");
  $("checkHistTitle").textContent = name + " · 监控历史";
  body.innerHTML = `<div class="empty-line">加载中…</div>`;
  $("checkHistMask").classList.add("show");
  try {
    const pts = await fetch(`${API}/checks/${encodeURIComponent(id)}/history`).then(r => r.json());
    if (!Array.isArray(pts) || !pts.length) {
      body.innerHTML = `<div class="empty-line">暂无历史数据（检查运行一段时间后自动积累，重启后重新计）</div>`;
      return;
    }
    const samples = pts.map(p => ({ timestamp: p.timestamp, latency_ms: p.latency_ms, loss_pct: (typeof p.loss_pct === "number" ? p.loss_pct : null), ok: p.ok }));
    const isPing = type === "ping";
    const uptime = (pts.filter(p => p.ok).length / pts.length * 100).toFixed(1);
    const avgLat = (pts.reduce((s, p) => s + (p.latency_ms || 0), 0) / pts.length).toFixed(0);
    const span = pts.length > 1 ? fmtDur(pts[pts.length - 1].timestamp - pts[0].timestamp) : "刚开始";
    const wrap = cid => `<div class="chart-wrap"><canvas id="${cid}" width="1000" height="230"></canvas>` +
      `<button class="chart-enlarge" data-chart="${cid}" title="放大预览"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg></button></div>`;
    body.innerHTML = `<div class="chart-container">${wrap("chkLat")}${isPing ? wrap("chkLoss") : ""}</div>
      <div class="hint">采样 ${pts.length} 个 · 时间跨度 ${span} · 可用率 ${uptime}% · 平均延时 ${avgLat} ms · 悬停查看数值，拖动框选放大，双击还原。</div>`;
    CHK_CHARTS = {};
    CHK_CHARTS.chkLat = createChart("chkLat", samples, [
      { key: "latency_ms", label: isPing ? "平均延时" : "延时", color: "#4c8dff", fmt: v => v.toFixed(0) + " ms" },
    ], 0, null, { title: name + " · 延时(ms)" });
    if (isPing) {
      CHK_CHARTS.chkLoss = createChart("chkLoss", samples, [
        { key: "loss_pct", label: "丢包率", color: "#f2545b", fmt: v => v.toFixed(0) + "%" },
      ], 0, 100, { title: name + " · 丢包率(%)" });
    }
  } catch (e) {
    body.innerHTML = `<div class="empty-line">加载失败: ${esc(e)}</div>`;
  }
}
// 历史弹窗内图表放大委托
safeAddEventListener("checkHistBody", "click", e => {
  const en = e.target.closest(".chart-enlarge"); if (!en) return;
  const ch = CHK_CHARTS[en.dataset.chart]; if (ch) openChartZoom(ch);
});
async function loadHostsMeta() {
  try { HOST_META = await fetch(`${API}/hosts/meta`).then(r => r.json()); } catch (e) { /* ignore */ }
}
function updateCkTargetLabel() {
  const t = $("ckType").value;
  if (t === "process") {
    $("ckHostField").style.display = "block";
    $("ckTargetLabel").textContent = "进程名称";
    $("ckTarget").placeholder = "如 nginx, mysql, aiops-agent";
    return;
  }
  $("ckHostField").style.display = "none";
  if (t === "http") {
    $("ckTargetLabel").textContent = "URL 地址";
    $("ckTarget").placeholder = "https://example.com";
  } else if (t === "ping") {
    $("ckTargetLabel").textContent = "主机地址 / IP";
    $("ckTarget").placeholder = "如 8.8.8.8 或 example.com";
  } else {
    $("ckTargetLabel").textContent = "主机:端口";
    $("ckTarget").placeholder = "127.0.0.1:3306";
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
  refresh(); loadChecks();
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
async function refresh(force) {
  if (PAUSED && !force) return;
  try {
    const rs = await fetch(`${API}/summary`);
    if (rs.status === 401) { $("loginView").classList.add("show"); return; }
    const s = await rs.json();
    const [hosts, alerts, activity] = await Promise.all([
      fetch(`${API}/hosts`).then(r => r.json()),
      fetch(`${API}/alerts`).then(r => r.json()),
      fetch(`${API}/activity`).then(r => r.json())
    ]);
    renderCards(s); renderAlerts(alerts); renderLog(activity); renderHosts(hosts); renderTop(hosts);
    $("clock").textContent = new Date().toLocaleTimeString("zh-CN");
    $("pulse").className = "pulse";
  } catch (e) {
    $("clock").textContent = "连接失败";
    $("pulse").className = "pulse off";
  }
}

/* ---------- 事件绑定（委托） ---------- */
const groupsEl = $("groups");
if (groupsEl) {
  groupsEl.addEventListener("click", e => {
    const host = e.target.closest(".host"); if (!host) return;
    const act = e.target.closest("[data-act]"); if (!act) return;
    const { id, name, cat } = host.dataset;
    if (act.dataset.act === "detail") openDetail(id, name);
    else if (act.dataset.act === "cat") editCategory(id, cat);
    else if (act.dataset.act === "del") delHost(id, name);
    else if (act.dataset.act === "term") openTerminal(id, name);
  });
}

const catFilterEl = $("catFilter");
if (catFilterEl) {
  catFilterEl.addEventListener("change", e => { CUR_CAT = e.target.value; HOST_PAGE = 1; renderHosts(LAST_HOSTS); });
}

// 主机筛选和排序
function filterHosts(value) {
  HOST_FILTER = value;
  HOST_PAGE = 1;
  renderHosts(LAST_HOSTS);
}

function sortHosts(value) {
  HOST_SORT = value;
  HOST_PAGE = 1;
  renderHosts(LAST_HOSTS);
}

// 暂停 / 恢复自动刷新
function togglePause() {
  PAUSED = !PAUSED;
  const btn = $("pauseBtn");
  if (btn) { btn.classList.toggle("active", PAUSED); btn.title = PAUSED ? "已暂停自动刷新，点击继续" : "暂停自动刷新"; }
  $("pulse").className = PAUSED ? "pulse paused" : "pulse";
  toast(PAUSED ? "已暂停自动刷新" : "已恢复自动刷新", "ok");
  if (!PAUSED) refresh(true);
}

// 一键清理所有离线主机
async function purgeOffline() {
  const off = LAST_HOSTS.filter(h => !h.online);
  if (!off.length) { toast("当前没有离线主机", "ok"); return; }
  if (!confirm(`确认清理 ${off.length} 台离线主机？\n若其 Agent 仍在运行，约 60 秒后会重新出现。`)) return;
  let ok = 0;
  for (const h of off) {
    try { const r = await fetch(`${API}/hosts/${encodeURIComponent(h.id)}`, { method: "DELETE" }); if (r.ok) ok++; } catch (e) { /* skip */ }
  }
  toast(`已清理 ${ok} 台离线主机`, "ok");
  refresh(true);
}

// Helper function to safely add event listeners
function safeAddEventListener(id, event, handler) {
  const el = $(id);
  if (el) {
    el.addEventListener(event, handler);
  } else {
    console.warn(`Element with id "${id}" not found`);
  }
}

safeAddEventListener("settingsBtn", "click", openSettings);
safeAddEventListener("saveBtn", "click", saveSettings);
safeAddEventListener("testBtn", "click", testSettings);
safeAddEventListener("installBtn", "click", openInstall);
safeAddEventListener("resetTokenBtn", "click", resetToken);
safeAddEventListener("copyCmdBtn", "click", function() {
  copyWithFeedback(this, $("installCmd").textContent, "已复制安装命令");
});
// 点击命令区域本身也可复制
safeAddEventListener("installCmd", "click", function() {
  const sel = window.getSelection();
  sel.removeAllRanges();
  const range = document.createRange();
  range.selectNodeContents(this);
  sel.addRange(range);
});
safeAddEventListener("installCategory", "input", renderInstallCmd);
safeAddEventListener("osTabs", "click", e => {
  const t = e.target.closest(".tab"); if (!t) return;
  CUR_OS = t.dataset.os;
  document.querySelectorAll("#osTabs .tab").forEach(x => x.classList.toggle("active", x === t));
  renderInstallCmd();
});
safeAddEventListener("copyUninstallBtn", "click", function() {
  copyWithFeedback(this, $("uninstallCmd").textContent, "已复制卸载命令");
});

/* ---------- 侧栏导航：视图切换 + 收起 + 移动抽屉 ---------- */
const navItems = document.querySelectorAll(".nav-item");
// 页面头元信息：标题 + 副标题。副标题让顶栏页面头承载“页面语义”，
// 而非机械回显侧栏导航名，从根上消除“两个概览”的重复观感。
const PAGE_META = {
  overview: { title: "概览", sub: "集群资源、告警与活动总览" },
  hosts:    { title: "主机", sub: "所有上报主机的实时指标" },
  alerts:   { title: "告警", sub: "阈值与自定义监控告警" },
  checks:   { title: "监控", sub: "网站 HTTP / 端口 TCP / 主机 Ping / 进程存活 拨测" },
  log:      { title: "日志", sub: "操作、系统与插件事件流水" },
};
function switchView(view) {
  document.querySelectorAll(".view").forEach(v => v.classList.toggle("active", v.id === "view-" + view));
  navItems.forEach(n => n.classList.toggle("active", n.dataset.view === view));
  const meta = PAGE_META[view];
  if (meta) {
    const t = $("pageTitle"), s = $("pageSub");
    if (t) t.textContent = meta.title;
    if (s) s.textContent = meta.sub;
  }
  window.scrollTo(0, 0);
}
navItems.forEach(n => n.addEventListener("click", () => {
  switchView(n.dataset.view);
  const appEl = $("app");
  if (appEl) appEl.classList.remove("nav-open");
}));

// 汉堡：桌面收起/展开侧栏；移动端打开/关闭抽屉
safeAddEventListener("menuBtn", "click", () => {
  const appEl = $("app");
  if (!appEl) return;
  if (window.innerWidth <= 900) appEl.classList.toggle("nav-open");
  else appEl.classList.toggle("collapsed");
});
safeAddEventListener("backdrop", "click", () => {
  const appEl = $("app");
  if (appEl) appEl.classList.remove("nav-open");
});

// 日志类型筛选
safeAddEventListener("logFilter", "click", e => {
  const b = e.target.closest(".chip-btn"); if (!b) return;
  LOG_KIND = b.dataset.kind;
  LOG_PAGE = 1;
  document.querySelectorAll("#logFilter .chip-btn").forEach(x => x.classList.toggle("active", x === b));
  renderLog(LAST_LOG);
});

// 日志级别和时间范围筛选
function filterLogsByLevel(level) {
  LOG_LEVEL = level;
  LOG_PAGE = 1;
  renderLog(LAST_LOG);
}

function filterLogsByTime(range) {
  LOG_TIME_RANGE = range;
  LOG_PAGE = 1;
  renderLog(LAST_LOG);
}

// 日志分页点击
safeAddEventListener("logPager", "click", e => {
  const b = e.target.closest("button[data-lpg]"); if (!b) return;
  const pg = b.dataset.lpg;
  if (pg === "prev") LOG_PAGE--;
  else if (pg === "next") LOG_PAGE++;
  else LOG_PAGE = parseInt(pg);
  renderLog(LAST_LOG);
});

// 监控类型筛选
function filterChecks(type) {
  CHECK_TYPE = type;
  renderChecks(LAST_CHECKS);
}
// 弹窗关闭：点遮罩空白处 或 右上角 ✕
document.querySelectorAll(".mask").forEach(mk => mk.addEventListener("click", e => {
  if (e.target === mk || e.target.closest("[data-close-btn]")) {
    mk.classList.remove("show"); hideChartTip();
    if (mk.id === "termMask") closeTerminalWS();
  }
}));
document.addEventListener("keydown", e => {
  if (e.key === "Escape") {
    const hadTerm = $("termMask") && $("termMask").classList.contains("show");
    document.querySelectorAll(".mask.show").forEach(mk => mk.classList.remove("show"));
    hideChartTip();
    if (hadTerm) closeTerminalWS();
  }
});

// KPI 卡片点击 → 跳转对应视图（并按需过滤主机）
safeAddEventListener("cards", "click", e => {
  const c = e.target.closest(".card"); if (!c) return;
  const [view, filter] = (c.dataset.goto || "").split(":");
  if (view === "hosts") { HOST_FILTER = filter || "all"; HOST_PAGE = 1; renderHosts(LAST_HOSTS); }
  if (view) switchView(view);
});
// 主机搜索 + 分页
safeAddEventListener("hostSearch", "input", e => { HOST_SEARCH = e.target.value; HOST_PAGE = 1; renderHosts(LAST_HOSTS); });
safeAddEventListener("pager", "click", e => {
  const b = e.target.closest("button[data-pg]"); if (!b) return;
  const pg = b.dataset.pg;
  if (pg === "prev") HOST_PAGE--;
  else if (pg === "next") HOST_PAGE++;
  else HOST_PAGE = parseInt(pg);
  renderHosts(LAST_HOSTS);
});
// 自定义监控
safeAddEventListener("addCheckBtn", "click", () => openCheckModal(null));
safeAddEventListener("ckType", "change", updateCkTargetLabel);
safeAddEventListener("ckSaveBtn", "click", saveCheck);
safeAddEventListener("checksGrid", "click", e => {
  const card = e.target.closest(".check-card"); if (!card) return;
  const act = e.target.closest("[data-cact]"); if (!act) return;
  const id = card.dataset.id, check = LAST_CHECKS.find(c => c.id === id);
  const cact = act.dataset.cact;
  if (cact === "hist") { if (check) openCheckHistory(id, check.name, check.type); return; } // 历史对内置检查也开放
  if (card.dataset.builtin) return; // 内置检查仅可查看历史，无编辑/删除
  if (cact === "edit") openCheckModal(check);
  else if (cact === "del") delCheck(id);
  else if (cact === "run") {
    fetch(`${API}/checks/${encodeURIComponent(id)}/run`, { method: "POST" })
      .then(() => { toast("已触发检测，结果稍后刷新", "ok"); setTimeout(loadChecks, 1500); })
      .catch(e => toast("触发失败: " + e, "err"));
  }
});
// 概览 TOP5 点击 → 直达该主机趋势
safeAddEventListener("topPanels", "click", e => {
  const it = e.target.closest(".top-item"); if (!it) return;
  openDetail(it.dataset.id, it.dataset.name);
});
// 日志导出
safeAddEventListener("exportLogBtn", "click", exportLogsCSV);
// 暂停自动刷新 + 批量清理离线
safeAddEventListener("pauseBtn", "click", togglePause);
safeAddEventListener("purgeOfflineBtn", "click", purgeOffline);
// 个人信息
safeAddEventListener("profileBtn", "click", openProfile);
safeAddEventListener("pfSaveBtn", "click", saveProfile);
safeAddEventListener("pfPwdBtn", "click", changePassword);
safeAddEventListener("logoutBtn", "click", logout);
// 登录
safeAddEventListener("loginForm", "submit", async e => {
  e.preventDefault();
  const loginErrEl = $("loginErr");
  if (loginErrEl) loginErrEl.textContent = "";
  try {
    const r = await fetch(`${API}/login`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: $("loginUser").value.trim(), password: $("loginPass").value })
    });
    if (r.ok) { 
      setUser(await fetch(`${API}/me`).then(x => x.json())); 
      const loginViewEl = $("loginView");
      if (loginViewEl) loginViewEl.classList.remove("show"); 
      startApp(); 
    }
    else { 
      const j = await r.json(); 
      if (loginErrEl) loginErrEl.textContent = j.error || "登录失败"; 
    }
  } catch (err) { 
    if (loginErrEl) loginErrEl.textContent = "登录失败: " + err; 
  }
});

/* ---------- 布局宽度：标准（默认）/ 宽屏，记忆到 localStorage ---------- */
function widePref() { try { return localStorage.getItem("aiops_wide") === "1"; } catch (e) { return false; } }
function applyWidthMode() {
  const wide = widePref();
  const app = $("app"); if (app) app.classList.toggle("wide", wide);
  const btn = $("widthBtn");
  if (btn) { btn.classList.toggle("active", wide); btn.title = wide ? "当前：宽屏，点击切换标准" : "当前：标准，点击切换宽屏"; }
}
safeAddEventListener("widthBtn", "click", () => {
  const wide = widePref();
  try { localStorage.setItem("aiops_wide", wide ? "0" : "1"); } catch (e) {}
  applyWidthMode();
  toast(wide ? "已切换为标准布局" : "已切换为宽屏布局", "ok");
});

/* ---------- 自定义监控视图切换（列表 / 胶囊） ---------- */
safeAddEventListener("checkViewToggle", "click", e => {
  const b = e.target.closest(".vt-btn"); if (!b) return;
  setCheckView(b.dataset.cview);
});

// 读取本地偏好并应用（视图 / 布局宽度）
function initPrefs() {
  try { const cv = localStorage.getItem("aiops_check_view"); if (cv === "pill" || cv === "list") CHECK_VIEW = cv; } catch (e) {}
  document.querySelectorAll("#checkViewToggle .vt-btn").forEach(b => b.classList.toggle("active", b.dataset.cview === CHECK_VIEW));
  applyWidthMode();
}

initPrefs();
initAuth();
