/* ============================================================
   AIOps Monitor · core.js — 全局变量、工具函数、路由、轮询、主题、通知
   加载顺序：必须在所有其他模块之前加载
   ============================================================ */
"use strict";

window.AIOps = window.AIOps || {};

/* ===== UI/UX 审查修复（5.6 弹窗语义角色 / 6.4 全局加载指示） ===== */
(function(){
  /* 6.4 全局请求加载指示：包装原生 fetch，任何请求进行中时显示顶部细进度条 */
  var _origFetch = window.fetch ? window.fetch.bind(window) : null;
  if (_origFetch) {
    var _pending = 0;
    var _bar = document.createElement("div");
    _bar.className = "loadbar";
    _bar.setAttribute("aria-hidden", "true");
    document.addEventListener("DOMContentLoaded", function(){ document.body.appendChild(_bar); });
    window.fetch = function() {
      _pending++; _bar.classList.add("active");
      return _origFetch.apply(window, arguments).finally(function(){
        _pending--; if (_pending <= 0) { _pending = 0; _bar.classList.remove("active"); }
      });
    };
  }
  /* 5.6 为所有弹窗补充语义角色（读屏支持），含动态创建的弹窗 */
  function enhanceModals(){
    document.querySelectorAll(".modal:not([role])").forEach(function(m){
      m.setAttribute("role", "dialog");
      m.setAttribute("aria-modal", "true");
    });
  }
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", enhanceModals);
  else enhanceModals();
  var _mo = window.MutationObserver && new MutationObserver(function(muts){
    muts.forEach(function(m){ if (m.addedNodes && m.addedNodes.length) enhanceModals(); });
  });
  if (_mo) _mo.observe(document.documentElement, { childList:true, subtree:true });
})();

// 防御性初始化：若 i18n-dashboard.js 加载失败，注入最小可用 I18N 对象，
// 避免 app.js 中大量顶层 I18N.t() 调用抛出 ReferenceError 导致整个脚本崩溃，
// 进而阻止 initAuth() 执行、登录界面无法显示。
if (typeof window.I18N === "undefined" || typeof window.I18N.t !== "function") {
  console.warn("[AIOps] I18N not loaded, installing fallback translator");
  window.I18N = {
    t: function(key, fallback) { return fallback || key; },
    applyTranslations: function() {},
    setLang: function() {},
    getLang: function() { return "zh-CN"; },
    syncLangButtons: function() {},
    supported: ["zh-CN"],
    init: function() {}
  };
}

const API = "/api/v1";

// Account password policy (mirrors the server): >=8 chars incl. upper/lower/digit/special.
function pwPolicyOK(pw){
  return typeof pw==="string" && pw.length>=8 && /[A-Z]/.test(pw) && /[a-z]/.test(pw) && /[0-9]/.test(pw) && /[^A-Za-z0-9]/.test(pw);
}

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
    () => toast(I18N.t("toast.copy_failed"), "err")
  );
}

/* ---------- 全局状态变量 ---------- */
let CUR_CATS = [];    // 当前分类多选筛选（空数组=全部）
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
let CHECK_VIEW = "pill"; // 自定义监控视图：pill(卡片,默认) | list(列表)
let HOST_VIEW = "card";  // 主机视图：card | list
let TERMINAL_ENABLED = true; // 服务端是否开启远程终端
let TERM_WS = null;   // 当前终端 WebSocket
let CONN_STATE = "connecting"; // connecting | connected | disconnected
let FIRST_LOAD = true;
let LAST_CATS_KEY = ""; // 用于检测分类列表是否变化
let LAST_RENDER_KEY = ""; // P0-3: 用于差量更新检测
let ALERT_TYPE = "";   // 告警类型筛选
let ALERT_SEARCH = ""; // 告警主机搜索
let LAST_ALERTS = [];

/* ---------- 工具函数 ---------- */
const $ = id => document.getElementById(id);
const esc = s => String(s == null ? "" : s).replace(/[&<>"]/g, c =>
  ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
// withLoading: disable button + show spinner during async operation, prevent duplicate submits
const _loadingBtns = new WeakSet();
function withLoading(btnId, fn) {
  const btn = typeof btnId === "string" ? $(btnId) : btnId;
  if (!btn) return fn();
  if (_loadingBtns.has(btn)) return Promise.resolve(); // already loading, skip
  _loadingBtns.add(btn);
  const orig = btn.innerHTML;
  const origDisabled = btn.disabled;
  btn.disabled = true;
  btn.style.opacity = "0.6";
  btn.style.pointerEvents = "none";
  btn.innerHTML = '<svg class="spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width:14px;height:14px;animation:spin .6s linear infinite"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>';
  return Promise.resolve(fn()).finally(() => {
    btn.innerHTML = orig;
    btn.disabled = origDisabled;
    btn.style.opacity = "";
    btn.style.pointerEvents = "";
    _loadingBtns.delete(btn);
  });
}
const fmtRate = b => b < 1024 ? b.toFixed(0) + " " + I18N.t("unit.bps")
  : b < 1048576 ? (b / 1024).toFixed(1) + " " + I18N.t("unit.kbps")
  : (b / 1048576).toFixed(2) + " " + I18N.t("unit.mbps");
const fmtIORate = b => b < 1024 ? b.toFixed(0) + " " + I18N.t("unit.bps")
  : b < 1048576 ? (b / 1024).toFixed(1) + " " + I18N.t("unit.kbps")
  : b < 1073741824 ? (b / 1048576).toFixed(1) + " " + I18N.t("unit.mbps")
  : (b / 1073741824).toFixed(2) + " " + I18N.t("unit.gbs");
const fmtIOPS = v => v < 1000 ? v.toFixed(0) : v < 10000 ? (v / 1000).toFixed(1) + I18N.t("unit.kilo") : (v / 1000).toFixed(0) + I18N.t("unit.kilo");
const fmtGB = b => (b / 1073741824).toFixed(1);
const fmtUptime = s => {
  const d = Math.floor(s / 86400), h = Math.floor(s % 86400 / 3600), m = Math.floor(s % 3600 / 60);
  return d > 0 ? `${d}${I18N.t("time.day")}${h}${I18N.t("time.hour")}` : h > 0 ? `${h}${I18N.t("time.hour")}${m}${I18N.t("time.min")}` : `${m}${I18N.t("time.minute")}`;
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
  return s < 60 ? `${s}${I18N.t("time.ago_sec")}` : s < 3600 ? `${Math.floor(s / 60)}${I18N.t("time.ago_min")}` : s < 86400 ? `${Math.floor(s / 3600)}${I18N.t("time.ago_hour")}` : `${Math.floor(s / 86400)}${I18N.t("time.ago_day")}`;
};
const fmtDur = sec => {
  const s = Math.max(0, Math.floor(sec));
  if (s < 60) return `${s}${I18N.t("time.sec")}`;
  if (s < 3600) return `${Math.floor(s / 60)}${I18N.t("time.minute")}`;
  if (s < 86400) return `${Math.floor(s / 3600)}${I18N.t("time.hour")}${Math.floor(s % 3600 / 60)}${I18N.t("time.min")}`;
  return `${Math.floor(s / 86400)}${I18N.t("time.day")}${Math.floor(s % 86400 / 3600)}${I18N.t("time.hour")}`;
};
// Translate log kind from English enum to display text
const translateLogKind = k => {
  if (k === "operation") return I18N.t("ui.operation");
  if (k === "system") return I18N.t("ui.system");
  if (k === "plugin") return I18N.t("section.op_sys_plugin_plugin");
  return k;
};
// Translate log level from English enum to display text
const translateLogLevel = lvl => {
  if (lvl === "info") return I18N.t("filter.info_level");
  if (lvl === "warning") return I18N.t("ui.warning");
  if (lvl === "critical") return I18N.t("ui.critical");
  return lvl;
};
// Translate execution status from English enum to display text
const translateExecStatus = s => {
  if (s === "running") return I18N.t("exec.status.running");
  if (s === "completed") return I18N.t("exec.status.completed");
  if (s === "failed") return I18N.t("exec.status.failed");
  if (s === "success") return I18N.t("exec.status.success");
  if (s === "timeout") return I18N.t("exec.status.timeout");
  if (s === "pending") return I18N.t("exec.status.pending");
  return s;
};
// Translate step status from English enum to display text
const translateStepStatus = s => {
  if (s === "running") return I18N.t("exec.step.running");
  if (s === "completed") return I18N.t("exec.step.completed");
  if (s === "failed") return I18N.t("exec.step.failed");
  if (s === "timeout") return I18N.t("exec.step.timeout");
  if (s === "pending") return I18N.t("exec.step.pending");
  return s;
};
// 与 agent 端一致的系统目录过滤（前端再兜一道，防旧 agent / 持久化历史里残留 /boot、/System 盘）
const isSystemMount = p => {
  p = String(p || "");
  return p === "/boot" || p.startsWith("/boot/") || p === "/System" || p.startsWith("/System/");
};

/* ============================================================
   P1-1: 主题切换
   ============================================================ */
function initTheme() {
  const saved = localStorage.getItem("aiops_theme") || "dark";
  document.documentElement.setAttribute("data-theme", saved);
  syncThemeIcons(saved);
}
function toggleTheme() {
  const cur = document.documentElement.getAttribute("data-theme") || "dark";
  const next = cur === "dark" ? "light" : "dark";
  document.documentElement.setAttribute("data-theme", next);
  localStorage.setItem("aiops_theme", next);
  syncThemeIcons(next);
  // 重绘所有已存在的 Canvas 图表，使其使用新的 CSS 变量颜色
  if (typeof DETAIL_CHARTS !== "undefined") {
    for (const key in DETAIL_CHARTS) { if (DETAIL_CHARTS[key] && key !== "__zoom") drawChart(DETAIL_CHARTS[key]); }
  }
  if (typeof CHK_CHARTS !== "undefined") {
    for (const key in CHK_CHARTS) { if (CHK_CHARTS[key]) drawChart(CHK_CHARTS[key]); }
  }
}
/* 同步当前主题图标 */
function syncThemeIcons(theme) {
  const ddDark = document.querySelector(".user-dropdown .icon-theme-dark");
  const ddLight = document.querySelector(".user-dropdown .icon-theme-light");
  if (ddDark && ddLight) {
    ddDark.style.display = theme === "dark" ? "" : "none";
    ddLight.style.display = theme === "light" ? "" : "none";
  }
}

/* ============================================================
   P0-4: 桌面通知 + 声音告警
   ============================================================ */
let NOTIF_PERMITTED = false;
let LAST_CRIT_COUNT = 0;
let NOTIF_SOUND_ENABLED = false;
function initNotifications() {
  if (!("Notification" in window)) return;
  NOTIF_SOUND_ENABLED = localStorage.getItem("aiops_sound") === "1";
  if (Notification.permission === "granted") {
    NOTIF_PERMITTED = true;
  }
}
function requestNotificationPermission() {
  if (!("Notification" in window)) { toast(I18N.t("toast.no_notif_support"), "err"); return; }
  Notification.requestPermission().then(p => {
    if (p === "granted") { NOTIF_PERMITTED = true; toast(I18N.t("toast.desktop_notif_on"), "ok"); }
    else { toast(I18N.t("toast.desktop_notif_denied"), "err"); }
  });
}
function notifyCriticalAlerts(critCount) {
  if (!NOTIF_PERMITTED || critCount <= LAST_CRIT_COUNT) { LAST_CRIT_COUNT = critCount; return; }
  const newAlerts = critCount - LAST_CRIT_COUNT;
  LAST_CRIT_COUNT = critCount;
  try {
    new Notification(I18N.t("misc.critical_alert_title"), {
      body: newAlerts + " " + I18N.t("misc.new_alerts_count") + " " + critCount + " " + I18N.t("misc.count_end"),
      icon: "/icon.svg",
      tag: "critical-alerts",
      renotify: true
    });
  } catch(e) {}
  // 可选声音提醒
  if (NOTIF_SOUND_ENABLED) {
    try {
      const ctx = new (window.AudioContext || window.webkitAudioContext)();
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.connect(gain); gain.connect(ctx.destination);
      osc.frequency.value = 880; osc.type = "sine";
      gain.gain.setValueAtTime(0.3, ctx.currentTime);
      gain.gain.exponentialRampToValueAtTime(0.01, ctx.currentTime + 0.5);
      osc.start(); osc.stop(ctx.currentTime + 0.5);
    } catch(e) {}
  }
}

/* ============================================================
   P1-4: 模态弹窗可访问性 — 焦点陷阱
   ============================================================ */
let FOCUS_TRAP = null;
function trapFocus(mask) {
  const focusable = mask.querySelectorAll('button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])');
  if (focusable.length === 0) return;
  const first = focusable[0], last = focusable[focusable.length - 1];
  first.focus();
  FOCUS_TRAP = function(e) {
    if (e.key === "Escape") { closeMask(mask); return; }
    if (e.key !== "Tab") return;
    if (e.shiftKey) {
      if (document.activeElement === first) { e.preventDefault(); last.focus(); }
    } else {
      if (document.activeElement === last) { e.preventDefault(); first.focus(); }
    }
  };
  mask.addEventListener("keydown", FOCUS_TRAP);
}
function releaseFocus(mask) {
  if (FOCUS_TRAP) { mask.removeEventListener("keydown", FOCUS_TRAP); FOCUS_TRAP = null; }
}
function closeMask(mask) {
  mask.classList.remove("show");
  releaseFocus(mask);
}
// P1-4: 统一模态弹窗打开函数（带焦点陷阱）
function openMask(mask) {
  if (typeof mask === "string") mask = $(mask);
  if (!mask) return;
  mask.classList.add("show");
  setTimeout(() => trapFocus(mask), 50);
}

/* ============================================================
   P2-4: 骨架屏
   ============================================================ */
function showSkeleton() {
  const cardsEl = $("cards");
  if (cardsEl) {
    cardsEl.innerHTML = Array(6).fill(0).map(() =>
      '<div class="skeleton skeleton-card"><div class="sk-icon skeleton"></div><div class="sk-lines"><div class="sk-line skeleton w60"></div><div class="sk-line skeleton w40"></div></div></div>'
    ).join("");
  }
  const groupsEl = $("groups");
  if (groupsEl) {
    groupsEl.innerHTML = '<div class="skeleton-grid">' + Array(6).fill(0).map(() =>
      '<div class="skeleton skeleton-host"></div>'
    ).join("") + '</div>';
  }
}

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

/* ---------- 数字滚动动画 ---------- */
function animateValue(el, from, to, duration = 400) {
  if (from === to) return;
  const start = performance.now();
  const step = (now) => {
    const p = Math.min((now - start) / duration, 1);
    const eased = 1 - Math.pow(1 - p, 3); // ease-out cubic
    el.textContent = Math.round(from + (to - from) * eased);
    if (p < 1) requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
}

/* ---------- 主循环 ---------- */
function updateFavicon(critCount) {
  const canvas = document.createElement("canvas");
  canvas.width = 16; canvas.height = 16;
  const ctx = canvas.getContext("2d");
  ctx.fillStyle = "#0a0d13"; ctx.fillRect(0, 0, 16, 16);
  ctx.fillStyle = "#4c8dff"; ctx.font = "bold 9px sans-serif"; ctx.textAlign = "center"; ctx.textBaseline = "middle";
  ctx.fillText("AI", 8, 8.5);
  if (critCount > 0) {
    ctx.fillStyle = "#ef4d5a"; ctx.beginPath(); ctx.arc(13, 3, 3.5, 0, Math.PI * 2); ctx.fill();
    ctx.fillStyle = "#fff"; ctx.font = "bold 5px sans-serif"; ctx.fillText(critCount > 9 ? "9+" : String(critCount), 13, 3.5);
  }
  let link = document.querySelector("link[rel=icon]");
  if (!link) { link = document.createElement("link"); link.rel = "icon"; document.head.appendChild(link); }
  link.href = canvas.toDataURL();
}

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
    // P0-#2: Connection state feedback
    if (FIRST_LOAD) { FIRST_LOAD = false; }
    if (CONN_STATE !== "connected") {
      if (CONN_STATE === "disconnected") toast(I18N.t("toast.reconnected"), "ok");
      CONN_STATE = "connected";
    }
    // Filter hosts by category for overview
    const filteredHosts = CUR_CATS.length > 0
      ? hosts.filter(h => CUR_CATS.includes(h.category || I18N.t("section.uncategorized")))
      : hosts;
    // Compute overview stats from filtered hosts
    if (CUR_CATS.length > 0) {
      let online = 0;
      filteredHosts.forEach(h => { if (h.online) online++; });
      const filteredAlerts = alerts.filter(a => !a.host_id || filteredHosts.some(h => h.id === a.host_id));
      s.total_hosts = filteredHosts.length;
      s.online_hosts = online;
      s.offline_hosts = filteredHosts.length - online;
      s.critical_alerts = filteredAlerts.filter(a => a.level === "critical").length;
      s.warning_alerts = filteredAlerts.filter(a => a.level !== "critical").length;
    }
    renderCards(s); renderStatsHealth(s); renderAlerts(alerts); renderLog(activity); renderHosts(hosts); renderTop(CUR_CATS.length > 0 ? filteredHosts : hosts);
    updateFavicon(s.critical_alerts || 0);
    notifyCriticalAlerts(s.critical_alerts || 0);
    const pulseEl = $("pulse"); if (pulseEl) pulseEl.className = "pulse";
  } catch (e) {
    const pulseEl = $("pulse"); if (pulseEl) pulseEl.className = "pulse off";
    if (CONN_STATE === "connected") {
      CONN_STATE = "disconnected";
      toast(I18N.t("ui.reconnecting"), "err");
    }
  }
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

// 暂停 / 恢复自动刷新
function togglePause() {
  PAUSED = !PAUSED;
  const btn = $("pauseBtn");
  if (btn) { btn.classList.toggle("active", PAUSED); btn.title = PAUSED ? I18N.t("toast.paused_click") : I18N.t("ui.pause_refresh"); }
  const pulseEl = $("pulse"); if (pulseEl) pulseEl.className = PAUSED ? "pulse paused" : "pulse";
  toast(PAUSED ? I18N.t("toast.paused") : I18N.t("toast.resumed"), "ok");
  if (!PAUSED) refresh(true);
}

// 一键清理所有离线主机
async function purgeOffline() {
  const off = LAST_HOSTS.filter(h => !h.online);
  if (!off.length) { toast(I18N.t("empty.no_offline_hosts"), "ok"); return; }
  if (!confirm(`确认清理 ${off.length} 台离线主机？\n若其 Agent 仍在运行，约 60 秒后会重新出现。`)) return;
  let ok = 0;
  for (const h of off) {
    try { const r = await fetch(`${API}/hosts/${encodeURIComponent(h.id)}`, { method: "DELETE" }); if (r.ok) ok++; } catch (e) { /* skip */ }
  }
  toast(I18N.t("toast.cleaned") + ok + I18N.t("toast.hosts_cleaned"), "ok");
  refresh(true);
}

/* ---------- 事件绑定（委托） ---------- */
const groupsEl = $("groups");

/* ---------- Multi-select category dropdown ---------- */
function getSelectedCats() {
  try { const s = localStorage.getItem("aiops_cats"); if (s) return JSON.parse(s); } catch (e) {}
  return [];
}
function setSelectedCats(arr) {
  CUR_CATS = arr;
  try { localStorage.setItem("aiops_cats", JSON.stringify(arr)); } catch (e) {}
}
function catCollapsed(cat) {
  try { const s = localStorage.getItem("aiops_collapsed"); if (s) { const arr = JSON.parse(s); return arr.includes(cat); } } catch (e) {}
  return false;
}
function toggleCatCollapse(cat) {
  let arr = [];
  try { const s = localStorage.getItem("aiops_collapsed"); if (s) arr = JSON.parse(s); } catch (e) {}
  const i = arr.indexOf(cat);
  if (i >= 0) arr.splice(i, 1); else arr.push(cat);
  try { localStorage.setItem("aiops_collapsed", JSON.stringify(arr)); } catch (e) {}
}
function renderCatDropdown(cats) {
  const wrap = $("catDropdownWrap") ? $("catDropdownWrap").parentElement : null;
  if (!wrap) return; // 分类筛选已移除
  if (wrap.querySelector(".cat-dropdown")) {
    updateCatDropdownOptions(cats);
    return;
  }
  const oldSel = $("catFilter");
  if (oldSel) oldSel.remove();
  wrap.innerHTML = `<div class="cat-dropdown" id="catDropdownWrap">
    <button class="cat-dd-btn" id="catDropdownBtn"><span id="catDropdownLabel">${I18N.t("ui.all_categories")}</span> <span class="dd-arrow">▾</span></button>
    <div class="cat-dd-menu" id="catDropdownMenu"></div>
  </div>`;
  updateCatDropdownOptions(cats);
  $("catDropdownBtn").addEventListener("click", e => {
    e.stopPropagation();
    $("catDropdownMenu").classList.toggle("show");
  });
  document.addEventListener("click", e => {
    const menu = $("catDropdownMenu");
    if (menu && !e.target.closest(".cat-dropdown")) menu.classList.remove("show");
  });
}
function updateCatDropdownOptions(cats) {
  const menu = $("catDropdownMenu");
  if (!menu) return;
  CUR_CATS = CUR_CATS.filter(c => cats.includes(c));
  const newKey = cats.join("\u0001");
  if (newKey === LAST_CATS_KEY) {
    menu.querySelectorAll("input").forEach(inp => {
      if (inp.value === "") inp.checked = CUR_CATS.length === 0;
      else inp.checked = CUR_CATS.includes(inp.value);
    });
    updateCatDropdownLabel();
    return;
  }
  LAST_CATS_KEY = newKey;
  menu.innerHTML = `<label class="cat-dd-opt"><input type="checkbox" value="" ${CUR_CATS.length === 0 ? "checked" : ""}> 全部分类</label>` +
    cats.map(c => `<label class="cat-dd-opt"><input type="checkbox" value="${esc(c)}" ${CUR_CATS.includes(c) ? "checked" : ""}> ${esc(c)}</label>`).join("");
  menu.querySelectorAll("input").forEach(inp => {
    inp.addEventListener("change", () => {
      if (inp.value === "") {
        if (inp.checked) {
          menu.querySelectorAll("input").forEach(x => { if (x !== inp) x.checked = false; });
          setSelectedCats([]);
        }
      } else {
        menu.querySelector('input[value=""]').checked = false;
        const selected = [...menu.querySelectorAll('input:checked')].map(x => x.value).filter(v => v !== "");
        setSelectedCats(selected);
      }
      HOST_PAGE = 1;
      updateCatDropdownLabel();
      renderHosts(LAST_HOSTS);
    });
  });
  updateCatDropdownLabel();
}
function updateCatDropdownLabel() {
  const label = $("catDropdownLabel");
  if (!label) return;
  const btn = $("catDropdownBtn");
  if (CUR_CATS.length === 0) {
    label.textContent = I18N.t("section.all_categories");
    if (btn) btn.classList.remove("filtered");
  } else {
    if (CUR_CATS.length <= 2) label.textContent = CUR_CATS.join(", ");
    else label.textContent = CUR_CATS.length + " 个分类";
    if (btn) btn.classList.add("filtered");
  }
}

/* ---------- Host filters ---------- */
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

/* ---------- 侧栏导航：视图切换 + 收起 + 移动抽屉 ---------- */
const navItems = document.querySelectorAll(".nav-item");
const PAGE_META = {
  overview: { title: I18N.t("ui.overview"), sub: I18N.t("section.overview_desc") },
  hosts:    { title: I18N.t("nav.hosts"), sub: I18N.t("section.hosts_desc") },
  alerts:   { title: I18N.t("ui.alerts"), sub: I18N.t("section.alerts_desc") },
  checks:   { title: I18N.t("ui.checks"), sub: I18N.t("section.checks_desc") },
  automation: { title: I18N.t("ui.automation"), sub: I18N.t("section.automation_desc") },
  forward:  { title: I18N.t("section.port_forward"), sub: I18N.t("section.forward_desc") },
  sre:      { title: I18N.t("section.sre"), sub: I18N.t("section.sre_desc") },
  logs:     { title: I18N.t("section.logs"), sub: I18N.t("section.logs_desc") },
  log:      { title: I18N.t("ui.log"), sub: I18N.t("section.log_desc") },
};
function rebuildPageMeta() {
  PAGE_META.overview   = { title: I18N.t("ui.overview"), sub: I18N.t("section.overview_desc") };
  PAGE_META.hosts      = { title: I18N.t("nav.hosts"), sub: I18N.t("section.hosts_desc") };
  PAGE_META.alerts     = { title: I18N.t("ui.alerts"), sub: I18N.t("section.alerts_desc") };
  PAGE_META.checks     = { title: I18N.t("ui.checks"), sub: I18N.t("section.checks_desc") };
  PAGE_META.automation = { title: I18N.t("ui.automation"), sub: I18N.t("section.automation_desc") };
  PAGE_META.forward    = { title: I18N.t("section.port_forward"), sub: I18N.t("section.forward_desc") };
  PAGE_META.log        = { title: I18N.t("ui.log"), sub: I18N.t("section.log_desc") };
}
function switchView(view) {
  document.querySelectorAll(".view").forEach(v => v.classList.toggle("active", v.id === "view-" + view));
  navItems.forEach(n => n.classList.toggle("active", n.dataset.view === view));
  const meta = PAGE_META[view];
  if (meta) {
    const t = $("pageTitle"), s = $("pageSub");
    if (t) t.textContent = meta.title;
    if (s) s.textContent = meta.sub;
  }
  if (view === "automation") loadPlaybooks();
  if (view === "forward") loadForwards();
  if (view === "sre") loadSRE();
  if (view === "logs") loadLogs();
  window.scrollTo(0, 0);
}

/* ---------- 读取本地偏好并应用 ---------- */
function initPrefs() {
  try { const cv = localStorage.getItem("aiops_check_view"); if (cv === "pill" || cv === "list") CHECK_VIEW = cv; } catch (e) {}
  try { const hv = localStorage.getItem("aiops_host_view"); if (hv === "list" || hv === "card") HOST_VIEW = hv; } catch (e) {}
  if (HOST_VIEW !== "list" && HOST_VIEW !== "card") HOST_VIEW = "card";
  document.querySelectorAll("#checkViewToggle .vt-btn").forEach(b => b.classList.toggle("active", b.dataset.cview === CHECK_VIEW));
  document.querySelectorAll("#hostViewToggle .vt-btn").forEach(b => b.classList.toggle("active", b.dataset.hview === HOST_VIEW));
}

/* ---------- Alert filter helpers ---------- */
function filterAlertsByType(type) {
  ALERT_TYPE = type;
  document.querySelectorAll("#alertFilter .chip-btn").forEach(b => b.classList.toggle("active", b.dataset.atype === type));
  renderAlerts(LAST_ALERTS);
}

/* ---------- 设置自定义监控视图 ---------- */
function setCheckView(view) {
  CHECK_VIEW = view;
  try { localStorage.setItem("aiops_check_view", view); } catch (e) {}
  document.querySelectorAll("#checkViewToggle .vt-btn").forEach(b => b.classList.toggle("active", b.dataset.cview === view));
  renderChecks(LAST_CHECKS);
}
function setHostView(view) {
  HOST_VIEW = view;
  try { localStorage.setItem("aiops_host_view", view); } catch (e) {}
  document.querySelectorAll("#hostViewToggle .vt-btn").forEach(b => b.classList.toggle("active", b.dataset.hview === view));
  HOST_PAGE = 1;
  renderHosts(LAST_HOSTS);
}

/* ============================================================
   P3-1: WebSocket 推送（替代轮询，带降级）
   ============================================================ */
let PUSH_WS = null;
let PUSH_CONNECTED = false;
let PUSH_RETRY = 0;

function initPushWS() {
  if (!window.WebSocket || (!window.isSecureContext && location.hostname !== "localhost")) return;
  try {
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    PUSH_WS = new WebSocket(proto + "//" + location.host + "/ws/push");
    PUSH_WS.onopen = () => {
      PUSH_CONNECTED = true;
      PUSH_RETRY = 0;
    };
    PUSH_WS.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data);
        if (msg.type === "summary" && msg.data) {
          renderCards(msg.data);
          updateFavicon(msg.data.critical_alerts || 0);
          notifyCriticalAlerts(msg.data.critical_alerts || 0);
        } else if (msg.type === "alerts" && msg.data) {
          renderAlerts(msg.data);
        } else if (msg.type === "hosts" && msg.data) {
          renderHosts(msg.data);
        }
      } catch(err) {}
    };
    PUSH_WS.onclose = () => {
      PUSH_CONNECTED = false;
      PUSH_RETRY++;
      if (PUSH_RETRY <= 10) {
        setTimeout(() => initPushWS(), Math.min(30000, 1000 * Math.pow(2, PUSH_RETRY)));
      }
    };
    PUSH_WS.onerror = () => { try { PUSH_WS.close(); } catch(e) {} };
  } catch(e) {}
}

/* ============================================================
   离线检测
   ============================================================ */
window.addEventListener("online", () => {
  toast(I18N.t("toast.network_recovered"), "ok");
  refresh(true);
});
window.addEventListener("offline", () => {
  toast(I18N.t("toast.network_disconnected"), "err");
});

/* ============================================================
   侧栏实时时钟
   ============================================================ */
function updateSideClock() {
  const el = $("sideClock");
  if (!el) return;
  const now = new Date();
  const pad = n => String(n).padStart(2, "0");
  el.textContent = `${now.getFullYear()}-${pad(now.getMonth()+1)}-${pad(now.getDate())} ${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`;
}

// 导出到 AIOps 命名空间
Object.assign(window.AIOps, {
  API, $, esc, withLoading, toast, icon, bar, animateValue,
  fmtRate, fmtIORate, fmtIOPS, fmtGB, fmtUptime, fmtDateTime, fmtDur,
  usageColor, ago, translateLogKind, translateLogLevel, translateExecStatus, translateStepStatus,
  isSystemMount, pwPolicyOK, copyToClipboard, copyWithFeedback,
  initTheme, toggleTheme, syncThemeIcons,
  initNotifications, requestNotificationPermission, notifyCriticalAlerts,
  trapFocus, releaseFocus, closeMask, openMask, showSkeleton,
  safeAddEventListener, refresh, updateFavicon, togglePause, purgeOffline,
  getSelectedCats, setSelectedCats, catCollapsed, toggleCatCollapse,
  renderCatDropdown, updateCatDropdownOptions, updateCatDropdownLabel,
  filterHosts, sortHosts, filterAlertsByType, filterLogsByLevel, filterLogsByTime,
  switchView, rebuildPageMeta, initPrefs, setCheckView, setHostView,
  initPushWS, updateSideClock,
  get CUR_CATS() { return CUR_CATS; }, set CUR_CATS(v) { CUR_CATS = v; },
  get LAST_HOSTS() { return LAST_HOSTS; }, set LAST_HOSTS(v) { LAST_HOSTS = v; },
  get PAUSED() { return PAUSED; }, set PAUSED(v) { PAUSED = v; },
  get CONN_STATE() { return CONN_STATE; }, set CONN_STATE(v) { CONN_STATE = v; },
  get FIRST_LOAD() { return FIRST_LOAD; }, set FIRST_LOAD(v) { FIRST_LOAD = v; },
  get TERMINAL_ENABLED() { return TERMINAL_ENABLED; }, set TERMINAL_ENABLED(v) { TERMINAL_ENABLED = v; },
});