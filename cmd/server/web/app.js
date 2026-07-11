/* ============================================================
   AIOps Monitor · 前端逻辑
   数据源：/api/v1/{summary,hosts,alerts,events,config}
   3 秒轮询（P1-2: 已改为差异化轮询频率）；事件委托绑定，避免内联 onclick 的转义隐患。

   P2-1 模块拆分说明：
   本文件可按功能域拆分为多个模块（服务端已支持 /js/ 路由）：
   - js/app-core.js    : 全局变量、工具函数、路由、轮询、主题、通知
   - js/app-render.js  : renderCards, renderHosts, renderAlerts, renderLog, renderTop
   - js/app-chart.js   : createChart, drawChart, attachChartEvents（Canvas 图表引擎）
   - js/app-terminal.js: VT100 仿真器、远程终端、会话回放
   - js/app-auth.js    : initAuth, login, MFA, 用户管理
   - js/app-automation.js: 剧本编排、批量执行
   在 index.html 中按依赖顺序加载多个 <script> 标签即可。
   ============================================================ */
"use strict";

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
  for (const key in DETAIL_CHARTS) { if (DETAIL_CHARTS[key] && key !== "__zoom") drawChart(DETAIL_CHARTS[key]); }
  for (const key in CHK_CHARTS) { if (CHK_CHARTS[key]) drawChart(CHK_CHARTS[key]); }
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

/* ============================================================
   P0-3: 渲染性能优化 — 差量更新
   ============================================================ */
let HOST_DOM_CACHE = {}; // hostID -> { element, data }
function updateHostCard(h) {
  const existing = HOST_DOM_CACHE[h.id];
  if (!existing) return false; // 新主机，需全量重建
  const el = existing.element;
  // 更新在线状态 class（卡片 + 状态灯）
  el.classList.toggle("online", !!h.online);
  el.classList.toggle("offline", !h.online);
  const dot = el.querySelector(".dot");
  if (dot) dot.className = "dot " + (h.online ? "on" : "off");
  // 更新指标数值
  const m = h.latest || {};
  if (m.cpu_percent !== undefined) {
    const cpuEl = el.querySelector("[data-metric=cpu]");
    if (cpuEl) cpuEl.textContent = (m.cpu_percent || 0).toFixed(1) + "%";
    const cpuBar = el.querySelector("[data-bar=cpu]");
    if (cpuBar) { cpuBar.style.width = (m.cpu_percent || 0) + "%"; cpuBar.style.background = usageColor(m.cpu_percent); }
  }
  if (m.mem_percent !== undefined) {
    const memEl = el.querySelector("[data-metric=mem]");
    if (memEl) memEl.textContent = (m.mem_percent || 0).toFixed(1) + "%";
    const memBar = el.querySelector("[data-bar=mem]");
    if (memBar) { memBar.style.width = (m.mem_percent || 0) + "%"; memBar.style.background = usageColor(m.mem_percent); }
  }
  if (m.disk_percent !== undefined) {
    const diskEl = el.querySelector("[data-metric=disk]");
    if (diskEl) diskEl.textContent = (m.disk_percent || 0).toFixed(1) + "%";
    const diskBar = el.querySelector("[data-bar=disk]");
    if (diskBar) { diskBar.style.width = (m.disk_percent || 0) + "%"; diskBar.style.background = usageColor(m.disk_percent); }
  }
  existing.data = h;
  return true;
}
function buildHostCache() {
  HOST_DOM_CACHE = {};
  document.querySelectorAll(".host").forEach(el => {
    const id = el.dataset.id;
    if (id) HOST_DOM_CACHE[id] = { element: el, data: null };
  });
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

/* ---------- 渲染：KPI ---------- */
function renderCards(s) {
  const cardsEl = $("cards");
  // 从现有 DOM 读取上一次数值（用于动画起始值）
  const prevVals = {};
  cardsEl.querySelectorAll(".v[data-val]").forEach(el => {
    const k = el.closest(".card")?.getAttribute("data-goto");
    if (k) prevVals[k] = parseInt(el.dataset.val) || 0;
  });

  const card = (cls, ic, v, k, vcls, goto) =>
    `<div class="card ${cls}" data-goto="${goto}" title="${I18N.t('section.click_view')}"><div class="ic">${icon(ic)}</div><div class="txt"><div class="v mono ${vcls || ""}" data-val="${v}">${v}</div><div class="k">${k}</div></div></div>`;
  cardsEl.innerHTML =
    card("info", "host", s.total_hosts, I18N.t("ui.total_hosts"), "", "hosts:all") +
    card("ok", "on", s.online_hosts, I18N.t("ui.online"), "ok", "hosts:online") +
    card(s.offline_hosts > 0 ? "crit" : "", "off", s.offline_hosts, I18N.t("ui.offline"), s.offline_hosts > 0 ? "crit" : "", "hosts:offline") +
    card(s.critical_alerts > 0 ? "crit" : "ok", "crit", s.critical_alerts, I18N.t("ui.critical_alerts"), s.critical_alerts > 0 ? "crit" : "ok", "alerts:") +
    card(s.warning_alerts > 0 ? "warn" : "ok", "warn", s.warning_alerts, I18N.t("ui.warning"), s.warning_alerts > 0 ? "warn" : "ok", "alerts:") +
    card("info", "event", s.plugin_events || 0, I18N.t("ui.plugin_events"), s.plugin_events > 0 ? "info" : "", "log:");

  // 数值变化动画
  cardsEl.querySelectorAll(".v[data-val]").forEach(el => {
    const goto = el.closest(".card")?.getAttribute("data-goto");
    const newVal = parseInt(el.dataset.val) || 0;
    const oldVal = prevVals[goto] !== undefined ? prevVals[goto] : newVal;
    if (oldVal !== newVal) animateValue(el, oldVal, newVal, 400);
  });

  // 告警数量增加时触发脉冲动画
  const prevCrit = prevVals["alerts:"] !== undefined ? prevVals["alerts:"] : 0;
  if (s.critical_alerts > prevCrit) {
    const critCard = cardsEl.querySelector(".card.crit[data-goto='alerts:']");
    if (critCard) {
      critCard.classList.add("card-pulse");
      setTimeout(() => critCard.classList.remove("card-pulse"), 600);
    }
  }

  // 空态引导 & 版本号
  const ob = $("onboarding");
  if (ob) ob.style.display = s.total_hosts === 0 ? "block" : "none";
  // 版本号显示在 brand 副标题中
  const verSpan = document.querySelector(".brand .sub");
  if (verSpan && s.version && s.version !== "AIOps") {
    verSpan.textContent = s.version;
  }
  TERMINAL_ENABLED = s.terminal_enabled !== false;
}

/* ---------- 渲染：统计与健康小结 ---------- */
function renderStatsHealth(s) {
  const grid = $("statsGrid");
  if (!grid) return;
  const total = s.total_hosts || 0;
  const online = s.online_hosts || 0;
  const rate = total > 0 ? Math.round(online / total * 100) : 0;
  const allAlerts = (s.critical_alerts || 0) + (s.warning_alerts || 0);
  const healthy = (s.critical_alerts || 0) === 0;
  const sc = (cls, val, key, hint) =>
    `<div class="stat-card"><div class="sv ${cls}">${val}</div><div class="sk">${key}</div>${hint ? `<div class="sh">${hint}</div>` : ""}</div>`;
  grid.innerHTML =
    sc("", total, I18N.t("ui.total_hosts"), "") +
    sc(rate >= 80 ? "ok" : rate >= 50 ? "warn" : "crit", rate + "%", I18N.t("section.online_rate"), online + "/" + total + " " + I18N.t("ui.online")) +
    sc(healthy ? "ok" : "crit", healthy ? I18N.t("section.health_ok") : I18N.t("section.health_error"), I18N.t("section.health_status"), !healthy ? I18N.t("section.unprocessed_alerts") + ": " + (s.critical_alerts || 0) : "") +
    sc(allAlerts > 0 ? "warn" : "ok", allAlerts, I18N.t("section.total_alerts"), I18N.t("ui.critical_alerts") + ": " + (s.critical_alerts || 0) + " / " + I18N.t("ui.warning") + ": " + (s.warning_alerts || 0));
  // 更新节头徽章（在线率摘要）
  const badge = $("statsHealthBadge");
  if (badge) badge.textContent = I18N.t("section.online_rate") + " " + rate + "%";
}

/* ---------- 渲染：告警 / 事件 ---------- */
const ALERT_TYPES = [
  {key:"", label:I18N.t("ui.all")}, {key:"cpu", label:"CPU"}, {key:"memory", label:I18N.t("ui.memory")},
  {key:"disk", label:I18N.t("ui.disk")}, {key:"gpu", label:"GPU"}, {key:"load", label:I18N.t("ui.load")},
  {key:"offline", label:I18N.t("ui.offline_status")}, {key:"check", label:I18N.t("ui.probe")}
];
/*
 * diffUpdateList — 概览列表差量更新引擎
 * 避免每 3 秒轮询全量 innerHTML 重建 DOM 导致的闪烁和布局跳动。
 * 策略：
 *   1. 生成新数据的签名摘要，若与上次完全一致则跳过更新（最常见路径）
 *   2. 签名不同时，按 key 逐行比对：保留匹配行、插入新行、标记多余行为 leaving
 *   3. 空列表 / 首次渲染走 innerHTML 快速路径
 *   4. 延迟移除机制：即将消失的行不立即删除，而是标记 .row-leaving 并设置
 *      5 秒宽限期。若同一 key 在下一次轮询中重新出现，则取消移除并复用节点。
 *      这解决了服务端 Evaluate() 无状态评估导致指标在阈值边界波动时
 *      同一告警时有时无、DOM 节点反复增删的闪烁问题。
 * 注意：匹配行不做 innerHTML 替换——时间相关的动态文本
 * （如“已持续 3 分钟”）由 refreshAlertRowTimes() 单独更新 textContent
 */
const DIFF_GRACE_MS = 5000; // 延迟移除宽限期：5 秒（覆盖 2 个 3 秒轮询周期）
function diffUpdateList(container, items, rowFn, keyFn, emptyHTML) {
  if (!container) return;
  const now = Date.now();
  // 1. 清理已过期的 leaving 行
  container.querySelectorAll(".row-leaving").forEach(el => {
    if (parseInt(el.dataset.leavingAt || "0") <= now) el.remove();
  });
  // 2. 快速路径：空列表
  if (!items.length) {
    // 若仍有 leaving 行在宽限期内，暂不显示“空”消息
    const leavingCount = container.querySelectorAll(".row-leaving").length;
    if (!leavingCount) {
      if (container.dataset.sig !== "empty") {
        container.innerHTML = emptyHTML;
        container.dataset.sig = "empty";
      }
    }
    return;
  }
  // 3. 签名检查：数据未变则完全跳过 DOM 操作
  const sig = items.map(keyFn).join("\n");
  if (container.dataset.sig === sig) return;
  container.dataset.sig = sig;
  // 4. 首次渲染或容器为空（无 data-key 行且无 leaving 行）
  const existing = container.querySelectorAll("[data-key]");
  if (!existing.length) {
    container.innerHTML = items.map(rowFn).join("");
    return;
  }
  // 5. 差量更新：按 key 匹配复用 DOM 节点
  const oldMap = {};
  existing.forEach(el => { oldMap[el.dataset.key] = el; });
  const newKeys = items.map(keyFn);
  const newKeySet = {};
  newKeys.forEach(k => { newKeySet[k] = true; });
  // 5a. 标记不再存在的行为 leaving（不立即删除）
  existing.forEach(el => {
    if (!newKeySet[el.dataset.key] && !el.classList.contains("row-leaving")) {
      el.classList.add("row-leaving");
      el.dataset.leavingAt = String(now + DIFF_GRACE_MS);
    }
  });
  // 5b. 插入/更新新行
  let refNode = null;
  for (let i = items.length - 1; i >= 0; i--) {
    const key = newKeys[i];
    let el = oldMap[key];
    if (el) {
      // 匹配行：取消任何待移除状态，不做 innerHTML 替换
      el.classList.remove("row-leaving", "row-enter");
      delete el.dataset.leavingAt;
    } else {
      // 新行：创建并插入到正确位置
      el = document.createElement("div");
      el.innerHTML = rowFn(items[i]).trim();
      el = el.firstChild;
      el.classList.add("row-enter");
    }
    // 确保 DOM 顺序与数据顺序一致
    if (refNode && el.nextSibling !== refNode) {
      container.insertBefore(el, refNode);
    } else if (!refNode && el !== container.firstChild) {
      container.insertBefore(el, container.firstChild);
    }
    refNode = el;
  }
}
/* refreshAlertRowTimes — 轻量级更新告警行的时间相关文本
 * 仅通过 textContent 更新“已持续 X 分”和绝对时间显示，
 * 不触及 innerHTML，不触发 DOM 重建和重排。 */
function refreshAlertRowTimes(container, now) {
  if (!container) return;
  container.querySelectorAll(".alert-dur[data-since]").forEach(el => {
    const since = parseInt(el.dataset.since);
    if (since) el.textContent = I18N.t("section.duration") + " " + fmtDur(now - since);
  });
}
function renderAlerts(alerts) {
  LAST_ALERTS = alerts;
  const n = alerts.length;
  $("alertsCount").textContent = n; $("navAlerts").textContent = n; $("ovAlertsCount").textContent = n;
  const now = Math.floor(Date.now() / 1000);
  // alertKey 使用稳定身份字段：type + scope + hostname + host_id
  // 这些字段在告警的生命周期内不会变化，不受 message 文本变化
  // （如拨测错误详情不同）或 since 重置（告警闪烁后重新计时）影响。
  // 这确保 diffUpdateList 能正确复用 DOM 节点，避免不必要的增删。
  const alertKey = a => {
    const type = a.type || "";
    const scope = a.scope || "";
    const id = a.host_id || "";
    if (type || scope || id) {
      return `${type}|${scope}|${a.hostname}|${id}`;
    }
    // Fallback: 仅当 type/scope/host_id 均缺失时使用 message（不应发生）
    return `${a.hostname}|${a.message}|${a.level}`;
  };
  const row = a => {
    const dur = a.since ? I18N.t("section.duration") + " " + fmtDur(now - a.since) : "";
    const ipStr = a.ip ? `<span class="alert-ip mono">${esc(a.ip)}</span>` : "";
    const timeStr = a.timestamp ? `<span class="alert-time mono">${fmtDateTime(a.timestamp)}</span>` : "";
    // dur 包装在 .alert-dur[data-since] 中，供 refreshAlertRowTimes 轻量更新
    const durSpan = a.since
      ? `<span class="src alert-dur" data-since="${a.since}" title="${I18N.t("section.first_fired")} ${fmtDateTime(a.since)}">${dur}</span>`
      : "";
    // 告警状态标签与操作按钮
    let statusBadge = "", actions = "";
    const hid = esc(a.host_id || ""), atyp = esc(a.type || ""), asc = esc(a.scope || "");
    const actAttrs = `data-host="${hid}" data-type="${atyp}" data-scope="${asc}"`;
    if (a.status === "acknowledged") {
      statusBadge = `<span class="badge status-badge status-ack">${I18N.t("alert.acknowledged")}</span>`;
      actions = `<button class="alert-action" data-action="clear" ${actAttrs} title="${I18N.t("alert.clear_status")}">↩</button>`;
    } else if (a.status === "silenced") {
      statusBadge = `<span class="badge status-badge status-silence">${I18N.t("alert.silenced")}</span>`;
      actions = `<button class="alert-action" data-action="clear" ${actAttrs} title="${I18N.t("alert.clear_status")}">↩</button>`;
    } else {
      actions = `<button class="alert-action" data-action="ack" ${actAttrs} title="${I18N.t("alert.acknowledge")}">✔</button>` +
        `<button class="alert-action" data-action="silence" ${actAttrs} title="${I18N.t("alert.silence")}">🔇</button>`;
    }
    const statusClass = a.status ? ` status-${esc(a.status)}` : "";
    return `<div class="row-item ${esc(a.level)}${statusClass}" tabindex="0" data-key="${esc(alertKey(a))}">
    <span class="badge ${esc(a.level)}">${a.level === "critical" ? I18N.t("ui.critical") : a.level === "info" ? I18N.t("toast.recovered") : I18N.t("ui.warning")}</span>
    ${statusBadge}
    <strong>${esc(a.hostname)}</strong>${ipStr}<span class="msg">${esc(a.message)}</span>
    ${durSpan}
    ${timeStr}
    <span class="alert-actions">${actions}</span></div>`;
  };
  // Apply filters
  let filtered = alerts;
  if (ALERT_TYPE) filtered = filtered.filter(a => a.type === ALERT_TYPE);
  if (ALERT_SEARCH) filtered = filtered.filter(a => {
    const hay = ((a.hostname || "") + " " + (a.ip || "") + " " + (a.message || "")).toLowerCase();
    return hay.includes(ALERT_SEARCH.toLowerCase());
  });
  const empty = `<div class="empty-line">✅ ${I18N.t("empty.no_alerts")}</div>`;
  const filterEmpty = `<div class="empty-line">${I18N.t("empty.no_alerts_filtered")}</div>`;
  $("alerts").innerHTML = filtered.length ? filtered.map(row).join("") : (n ? filterEmpty : empty);
  // 概览页告警列表：差量更新，避免全量 innerHTML 重建导致闪烁
  diffUpdateList($("ovAlerts"), alerts.slice(0, 6), row, alertKey, empty);
  // 轻量级更新“已持续”相对时间文本（仅 textContent，不重建 DOM）
  refreshAlertRowTimes($("ovAlerts"), now);
}

/* ---------- 概览：资源 TOP10 排行榜（多面板横向条形图） ---------- */
function renderTop(hosts) {
  const el = $("topPanels");
  if (!el) return;
  const live = hosts.filter(h => h.latest && h.online);

  if (!live.length) {
    el.innerHTML = `<div class="top-empty">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="3" width="18" height="18" rx="2"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="9" y1="21" x2="9" y2="9"/></svg>
      <span>${I18N.t("empty.no_online_hosts")}</span>
    </div>`;
    return;
  }

  // GPU panel visibility: show if ANY host has GPU data (even if utilization
  // is 0% — idle GPUs should still appear). The old filter required
  // util_percent > 0 which caused the panel to flicker on/off as GPUs idled.
  const hasGPU = live.some(h => { const gs = (h.latest.gpus || []); return gs.length > 0; });
  const diskMax = m => { const d = m.disks || []; return d.length ? Math.max(...d.map(x => x.percent)) : (m.disk_percent || 0); };
  const gpuMax = m => { const g = m.gpus || []; return g.length ? Math.max(...g.map(x => x.util_percent || 0)) : 0; };
  const netTotal = m => (m.net_sent_rate || 0) + (m.net_recv_rate || 0);
  const iopsTotal = m => (m.disk_read_iops || 0) + (m.disk_write_iops || 0);

  // GPU host filter: include any host that has GPU data (gs.length > 0).
  // Hosts with GPUs always appear in the GPU panel regardless of current load —
  // this prevents the flickering behavior where hosts disappear when their GPUs
  // are idle (util_percent = 0) or when nvidia-smi briefly times out and the
  // cached data has empty gpus array for one cycle.
  const gpuLive = live.filter(h => {
    const gs = (h.latest.gpus || []);
    return gs.length > 0;
  });

  // 面板定义：[key, title, unit, valueFn, isPct, displayFn]
  const panels = [
    { key: "cpu", title: I18N.t("section.cpu_top10"), unit: "%", fn: m => m.cpu_percent || 0, isPct: true },
    ...(hasGPU ? [{ key: "gpu", title: I18N.t("section.gpu_top10"), unit: "%", fn: gpuMax, isPct: true, dataSource: gpuLive }] : []),
    { key: "mem", title: I18N.t("section.mem_top10"), unit: "%", fn: m => m.mem_percent || 0, isPct: true },
    { key: "disk", title: I18N.t("section.disk_top10"), unit: "%", fn: diskMax, isPct: true },
    { key: "diskio", title: I18N.t("section.diskio_top10"), unit: "%", fn: m => m.disk_io_util_percent || 0, isPct: true },
    { key: "iops", title: I18N.t("section.iops_top10"), unit: I18N.t("unit.iops"), fn: iopsTotal, isPct: false },
    { key: "net", title: I18N.t("section.net_top10"), unit: I18N.t("unit.mbps"), fn: netTotal, isPct: false },
    { key: "load", title: I18N.t("section.load_top10"), unit: "", fn: m => m.load5 || 0, isPct: false },
    { key: "proc", title: I18N.t("section.proc_top10"), unit: "", fn: m => m.proc_count || 0, isPct: false },
  ];

  const topN = (arr, fn, n) => arr.slice().sort((a, b) => fn(b.latest) - fn(a.latest)).slice(0, n);

  function renderPanel(panel) {
    const source = panel.dataSource || live;
    const sorted = topN(source, panel.fn, 10);
    if (!sorted.length) {
      return `<div class="top-panel">
        <div class="top-title">${esc(panel.title)}<span class="top-unit">${esc(panel.unit)}</span></div>
        <div class="top-empty">${I18N.t("empty.no_data")}</div>
      </div>`;
    }
    const maxVal = Math.max(1, ...sorted.map(h => panel.fn(h.latest)));
    const items = sorted.map((h, idx) => {
      const v = panel.fn(h.latest);
      const pct = panel.isPct ? v : Math.min(100, v / maxVal * 100);
      const width = Math.max(3, pct);
      const color = panel.key === "net" ? "var(--info)" : usageColor(panel.isPct ? v : pct);
      let disp;
      if (panel.isPct) disp = v.toFixed(1) + "%";
      else if (panel.key === "net") disp = fmtRate(v);
      else if (panel.key === "iops") disp = fmtIOPS(v) + " " + I18N.t("unit.iops");
      else if (panel.key === "load") disp = v.toFixed(2);
      else if (panel.key === "proc") disp = v.toFixed(0);
      else disp = v.toFixed(1);
      return `<div class="top-item" tabindex="0" data-id="${esc(h.id)}" data-name="${esc(h.hostname || h.id)}" title="${esc(h.hostname || h.id)} · ${esc(disp)}">
        <span class="top-name">${esc(h.hostname || h.id)}</span>
        <div class="top-bar"><div class="top-bar-fill" style="width:${width}%;background:${color}"></div></div>
        <span class="top-val mono">${esc(disp)}</span>
      </div>`;
    }).join("");
    return `<div class="top-panel">
      <div class="top-title">${esc(panel.title)}<span class="top-unit">${esc(panel.unit)}</span></div>
      ${items}
    </div>`;
  }

  let html = panels.map(renderPanel).join("");

  // 监控探针面板
  const checksHtml = checkTopPanels();
  if (checksHtml) {
    html += `<div class="top-checks-panel">
      <div class="top-checks-title">${I18N.t("section.custom_monitor")}</div>
      ${checksHtml}
    </div>`;
  }

  el.innerHTML = html;
}

// 监控 TOP10，顺序 Ping → TCP → HTTP → 进程；无该类型监控则该面板不显示
function checkTopPanels() {
  const checks = (Array.isArray(LAST_CHECKS) ? LAST_CHECKS : []).filter(c => !c.builtin);
  if (!checks.length) return "";
  const byType = t => checks.filter(c => c.type === t);
  const panels = checkTopPanel(I18N.t("section.ping_top10"), byType("ping"), false)
    + checkTopPanel(I18N.t("section.tcp_top10"), byType("tcp"), false)
    + checkTopPanel(I18N.t("section.http_top10"), byType("http"), false)
    + checkTopPanel(I18N.t("section.process_top10"), byType("process"), true);
  return panels ? `<div class="checks-row">${panels}</div>` : "";
}
function checkTopPanel(title, list, isProc) {
  if (!list.length) return "";
  const sorted = list.slice().sort((a, b) => {
    const ad = (!a.ok && a.checked_at) ? 1 : 0, bd = (!b.ok && b.checked_at) ? 1 : 0;
    if (ad !== bd) return bd - ad;
    return (b.latency_ms || 0) - (a.latency_ms || 0);
  }).slice(0, 10);
  const maxLat = Math.max(1, ...sorted.map(c => c.latency_ms || 0));
  const items = sorted.map(c => {
    const down = !c.ok && c.checked_at, unknown = !c.checked_at;
    let val, color, width;
    if (isProc) {
      val = down ? I18N.t("ui.abnormal") : unknown ? I18N.t("ui.pending") : I18N.t("ui.normal");
      color = down ? "var(--crit)" : unknown ? "var(--muted2)" : "var(--ok)";
      width = unknown ? 0 : 100;
    } else if (down) { val = I18N.t("ui.abnormal"); color = "var(--crit)"; width = 100; }
    else if (unknown) { val = I18N.t("ui.pending"); color = "var(--muted2)"; width = 0; }
    else {
      const lat = Math.round(c.latency_ms || 0);
      val = lat + " " + I18N.t("unit.ms"); color = lat >= 1000 ? "var(--crit)" : lat >= 300 ? "var(--warn)" : "var(--ok)";
      width = Math.min(100, (c.latency_ms || 0) / maxLat * 100);
    }
    return `<div class="checks-item" data-check-id="${esc(c.id)}" data-check-name="${esc(c.name)}" data-check-type="${esc(c.type)}" title="${I18N.t("section.click_history")}">
      <span class="checks-name">${esc(c.name)}</span>
      <div class="checks-bar"><div class="checks-bar-fill" style="width:${width}%;background:${color}"></div></div>
      <span class="checks-val mono" style="color:${color}">${val}</span>
    </div>`;
  }).join("");
  return `<div class="checks-panel"><div class="checks-title">${title}</div>${items}</div>`;
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
  // Filter out internal alert engine logs (actor="告警引擎" from backend)
  return filtered.filter(e => e.actor !== I18N.t("notify.alert_engine"));
}

function exportLogsCSV() {
  const rows = applyLogFilters(LAST_LOG);
  if (!rows.length) { toast(I18N.t("empty.no_log_export"), "err"); return; }
  const escCsv = v => `"${String(v == null ? "" : v).replace(/"/g, '""')}"`;
  const lines = [I18N.t("section.csv_header")];
  rows.forEach(e => lines.push([fmtDateTime(e.timestamp), translateLogKind(e.kind), translateLogLevel(e.level), e.actor || "", e.host || "", e.message].map(escCsv).join(",")));
  const blob = new Blob(["﻿" + lines.join("\r\n")], { type: "text/csv;charset=utf-8" });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = `AIOps-logs-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-")}.csv`;
  a.click();
  URL.revokeObjectURL(a.href);
  toast(`${I18N.t("toast.exported")} ${rows.length} ${I18N.t("time.records")}${I18N.t("ui.log")}`, "ok");
}

function renderLog(items) {
  LAST_LOG = items;
  const n = items.length;
  $("logCount").textContent = n; $("navLog").textContent = n;
  const kcls = k => k === "operation" ? "op" : k === "system" ? "sys" : "plg";
  const logKey = e => `${e.kind}|${e.message}|${e.level}|${e.timestamp||0}|${e.actor||""}|${e.host||""}`;
  const row = e => `<div class="row-item ${esc(e.level)}" data-key="${esc(logKey(e))}">
    <span class="kind ${kcls(e.kind)}">${esc(translateLogKind(e.kind))}</span>
    <span class="msg">${esc(e.message)}</span>
    <span class="src">${esc(e.actor || "")}${e.host ? " · " + esc(e.host) : ""}</span>
    <span class="log-time mono">${fmtDateTime(e.timestamp)}</span></div>`;
  
  const filtered = applyLogFilters(items);
  const total = filtered.length;
  const pages = Math.max(1, Math.ceil(total / LOG_PAGE_SIZE));
  if (LOG_PAGE > pages) LOG_PAGE = pages;
  if (LOG_PAGE < 1) LOG_PAGE = 1;
  const pageItems = filtered.slice((LOG_PAGE - 1) * LOG_PAGE_SIZE, LOG_PAGE * LOG_PAGE_SIZE);
  $("log").innerHTML = pageItems.length ? pageItems.map(row).join("") : `<div class="empty-line">${I18N.t("empty.no_logs")}</div>`;
  renderLogPager(pages, total);
}

function renderLogPager(pages, total) {
  const pager = $("logPager");
  if (!pager) return;
  if (total === 0) { pager.innerHTML = ""; return; }
  if (pages <= 1) { pager.innerHTML = `<span class="pinfo">${I18N.t("ui.matched")}${total} ${I18N.t("time.records")}</span>`; return; }
  let btns = `<button ${LOG_PAGE === 1 ? "disabled" : ""} data-lpg="prev">‹</button>`;
  for (let i = 1; i <= pages; i++) {
    if (i === 1 || i === pages || Math.abs(i - LOG_PAGE) <= 1) {
      btns += `<button class="${i === LOG_PAGE ? "active" : ""}" data-lpg="${i}">${i}</button>`;
    } else if (Math.abs(i - LOG_PAGE) === 2) {
      btns += `<span class="pinfo">…</span>`;
    }
  }
  btns += `<button ${LOG_PAGE === pages ? "disabled" : ""} data-lpg="next">›</button>`;
  btns += `<span class="pinfo">${I18N.t("ui.matched")}${total} ${I18N.t("time.records")} · ${LOG_PAGE}/${pages}${I18N.t("time.page_suffix")}</span>`;
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
let DETAIL_TIME_RANGE = 24; // hours: 1, 24, 48, 168, 720

// 统一的时间跨度控件渲染函数（主机图表和监控图表共用）
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
  $("detailTitle").textContent = name + " " + I18N.t("section.recent_trend");
  const body = $("detailBody");
  body.innerHTML = `<div class="empty-line">${I18N.t("ui.loading")}</div>`;
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
      body.innerHTML = `<div class="empty-line">${I18N.t("empty.no_history")}</div>`;
      return;
    }

    // 组织图表：每个图表包裹在 .chart-wrap 内，右上角提供放大按钮
    DETAIL_CHARTS = {};
    const gran = DETAIL_TIME_RANGE <= 2 ? I18N.t("time.raw") : DETAIL_TIME_RANGE <= 48 ? I18N.t("time.1m_agg") : I18N.t("time.5m_agg");
    const hasGPU = samples.some(s => Array.isArray(s.gpus) && s.gpus.length);
    const pct = v => v.toFixed(1) + '%';
    const wrap = id => `<div class="chart-wrap"><canvas id="${id}" width="1000" height="240"></canvas>` +
      `<button class="chart-enlarge" data-chart="${id}" title="${I18N.t('ui.zoom_preview')}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg></button></div>`;
    body.innerHTML = `
      <div class="chart-controls">
        ${renderChartControls(DETAIL_TIME_RANGE, "range")}
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

    // 系统负载组合曲线：load1 / load5 / load15 三条折线同一坐标系
    DETAIL_CHARTS.chartLoad = createChart('chartLoad', samples, [
      { key: 'load1', label: I18N.t("section.load_1m_label"), color: '#4c8dff', fmt: v => v.toFixed(1) },
      { key: 'load5', label: I18N.t("section.load_5m_label"), color: '#f7b23b', fmt: v => v.toFixed(1) },
      { key: 'load15', label: I18N.t("section.load_15m_label"), color: '#f2545b', fmt: v => v.toFixed(1) },
    ], null, null, { title: I18N.t("section.load_avg") });

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
      diskSeries.length ? diskSeries : [{ key: 'disk_percent', label: I18N.t("section.root_partition"), color: '#f7b23b', fmt: pct }],
      0, 100, { title: I18N.t("section.disk_usage") });

    // GPU：每块显卡一条线（存在时才有该图）
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

/* ---------- 远程终端（经 Agent 反向通道）· 多标签 ---------- */
let TERM_TABS = [];      // [{id, name, ws, vt, screenEl, tabEl, retry}]
let TERM_ACTIVE = -1;    // active tab index
let TERM_RESIZE = null;  // window resize listener

/* ---------- v5.3.0: 终端二次认证 ---------- */
let TERM_AUTH_VERIFIED = false;    // 当前会话是否已验证终端密码
let TERM_AUTH_CHECKING = false;    // 是否正在执行认证流程
let TERM_AUTH_PENDING = null;      // 待处理的终端打开请求 {id, name}

function openTerminal(id, name) {
  // 多会话支持：同一 hostID 可创建多个标签页，每个标签页拥有独立的 WebSocket 连接。
  // 如果已有该主机的标签页且处于 dock 收起状态，优先恢复而不是新建。
  const dockedIdx = TERM_TABS.findIndex(t => t.id === id && TERM_DOCK_IDS.has(t.id));
  if (dockedIdx >= 0) {
    TERM_DOCK_IDS.delete(id);
    const dockItem = $("termDock") && $("termDock").querySelector(`[data-tab-id="${CSS.escape(id)}"]`);
    if (dockItem) dockItem.remove();
    updateTermDock();
    switchTermTab(dockedIdx);
    $("termMask").classList.add("show");
    requestAnimationFrame(() => requestAnimationFrame(termRefit));
    return;
  }

  // v5.3.0: 终端二次认证流程
  if (TERM_AUTH_CHECKING) return; // 避免重复触发
  TERM_AUTH_PENDING = { id, name };
  checkTerminalAccess();
}

/* ---------- 终端右键菜单 ---------- */
let TERM_CMENU_EL = null;
function initTermContextMenu() {
  if (TERM_CMENU_EL) return;
  TERM_CMENU_EL = document.createElement("div");
  TERM_CMENU_EL.className = "term-cmenu";
  TERM_CMENU_EL.innerHTML = `
    <div class="term-cmenu-item" data-action="copy">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
      <span>复制</span><span class="cmenu-key">Ctrl+C</span>
    </div>
    <div class="term-cmenu-item" data-action="paste">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"/><rect x="8" y="2" width="8" height="4" rx="1" ry="1"/></svg>
      <span>粘贴</span><span class="cmenu-key">Ctrl+V</span>
    </div>
    <div class="term-cmenu-sep"></div>
    <div class="term-cmenu-item" data-action="reconnect">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21.5 2v6h-6M2.5 22v-6h6M2 11.5a10 10 0 0 1 18.8-4.3M22 12.5a10 10 0 0 1-18.8 4.2"/></svg>
      <span>重新连接</span>
    </div>
    <div class="term-cmenu-sep"></div>
    <div class="term-cmenu-item" data-action="clear">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9"/><path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/></svg>
      <span>清屏</span>
    </div>
  `;
  document.body.appendChild(TERM_CMENU_EL);
  // 点击菜单外部关闭
  document.addEventListener("click", (e) => {
    if (TERM_CMENU_EL && !TERM_CMENU_EL.contains(e.target)) {
      TERM_CMENU_EL.classList.remove("show");
    }
  });
  // Esc 关闭
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && TERM_CMENU_EL) {
      TERM_CMENU_EL.classList.remove("show");
    }
  });
  // 菜单项点击处理
  TERM_CMENU_EL.addEventListener("click", (e) => {
    const item = e.target.closest(".term-cmenu-item");
    if (!item || item.classList.contains("disabled")) return;
    const action = item.dataset.action;
    const tab = TERM_CMENU_EL._termTab;
    TERM_CMENU_EL.classList.remove("show");
    if (!tab) return;
    switch (action) {
      case "copy": {
        const sel = getSelectedTermText(tab);
        if (sel) copyToClipboard(sel).then(() => toast(I18N.t("toast.copied"), "ok"), () => toast(I18N.t("toast.copy_failed"), "err"));
        break;
      }
      case "paste": {
        if (navigator.clipboard && navigator.clipboard.readText) {
          navigator.clipboard.readText().then(t => {
            if (t && tab.ws && tab.ws.readyState === 1) termSend(tab.ws, t);
          }).catch(() => {});
        }
        // 聚焦输入框让用户手动粘贴
        if (tab.inputEl) tab.inputEl.focus({ preventScroll: true });
        break;
      }
      case "reconnect":
        if (tab.ws && tab.ws.readyState === 1) { toast(I18N.t("term.connected"), "info"); return; }
        reconnectTermTab(tab);
        break;
      case "clear":
        if (tab.vt && tab.vt.fullReset) {
          tab.vt.fullReset();
          tab.vt.render();
        }
        break;
    }
  });
}
function showTermContextMenu(tab, e) {
  initTermContextMenu();
  if (!TERM_CMENU_EL) return;
  e.preventDefault();
  e.stopPropagation();
  TERM_CMENU_EL._termTab = tab;
  // 更新菜单项状态
  const copyItem = TERM_CMENU_EL.querySelector('[data-action="copy"]');
  const reconnectItem = TERM_CMENU_EL.querySelector('[data-action="reconnect"]');
  const hasSelection = getSelectedTermText(tab).length > 0;
  if (copyItem) copyItem.classList.toggle("disabled", !hasSelection);
  const disconnected = !tab.ws || tab.ws.readyState !== 1;
  if (reconnectItem) reconnectItem.classList.toggle("disabled", !disconnected);
  // 定位
  TERM_CMENU_EL.style.display = "block";
  let x = e.clientX, y = e.clientY;
  const mw = TERM_CMENU_EL.offsetWidth || 160;
  const mh = TERM_CMENU_EL.offsetHeight || 150;
  if (x + mw > window.innerWidth) x = window.innerWidth - mw - 4;
  if (y + mh > window.innerHeight) y = window.innerHeight - mh - 4;
  if (x < 0) x = 4;
  if (y < 0) y = 4;
  TERM_CMENU_EL.style.left = x + "px";
  TERM_CMENU_EL.style.top = y + "px";
  TERM_CMENU_EL.classList.add("show");
}

/* ---------- v5.3.0: 终端二次认证流程 ---------- */
const TERM_PROTOCOL_KEY = "aiops_term_protocol_agreed";

// 实际执行终端打开（原 openTerminal 后半部分逻辑）
function doOpenTerminal(id, name) {
  const sameHostTabs = TERM_TABS.filter(t => t.hostId === id);
  const tabName = sameHostTabs.length > 0 ? `${name} (${sameHostTabs.length + 1})` : name;
  createTermTab(id, name, tabName);
}

// 终端访问权限检查：协议 → 密码状态 → 验证
async function checkTerminalAccess() {
  if (TERM_AUTH_CHECKING) return;
  TERM_AUTH_CHECKING = true;

  try {
    // 1. 检查协议是否已同意
    if (!localStorage.getItem(TERM_PROTOCOL_KEY)) {
      showTermProtocol();
      return;
    }

    // 2. 检查是否已设置密码
    const statusRes = await fetch("/api/user/terminal-password/status", { credentials: "include" });
    const status = await statusRes.json().catch(() => ({}));

    if (!status.has_password) {
      // 未设置密码，弹出设置窗口
      showTermSetPassword();
      return;
    }

    // 3. 检查当前会话是否已验证——以服务端会话状态为准：浏览器刷新后本地
    //    TERM_AUTH_VERIFIED 会被重置，但服务端 session 仍记得已验证，
    //    因此这里读取 status.verified 并同步本地标记，避免刷新后反复重输终端密码。
    if (status.verified) TERM_AUTH_VERIFIED = true;
    if (TERM_AUTH_VERIFIED) {
      proceedToTerminal();
      return;
    }

    // 需要验证密码
    showTermVerify();
  } catch (e) {
    TERM_AUTH_CHECKING = false;
    toast(I18N.t("toast.network_error"), "err");
  }
}

// 协议同意后继续流程
function onTermProtocolAgreed() {
  localStorage.setItem(TERM_PROTOCOL_KEY, "1");
  $("termProtocolMask").classList.remove("show");
  // 重置检查锁，让 checkTerminalAccess 能继续执行后续步骤
  TERM_AUTH_CHECKING = false;
  checkTerminalAccess();
}

// 显示协议弹窗
function showTermProtocol() {
  $("termProtocolAgree").checked = false;
  $("termProtocolContinue").disabled = true;
  $("termProtocolMask").classList.add("show");
}

// 显示密码设置弹窗
function showTermSetPassword() {
  $("termSetPwd").value = "";
  $("termSetPwd2").value = "";
  $("termSetPwdErr").textContent = "";
  $("termSetPwdErr").style.display = "none";
  $("termSetPwdMask").classList.add("show");
}

// 显示密码验证弹窗
function showTermVerify() {
  $("termVerifyPwd").value = "";
  $("termVerifyErr").textContent = "";
  $("termVerifyErr").style.display = "none";
  $("termAttemptsInfo").style.display = "none";
  $("termVerifyMask").classList.add("show");
  setTimeout(() => { const el = $("termVerifyPwd"); if (el) el.focus(); }, 100);
}

// 密码设置/验证完成后打开终端
function proceedToTerminal() {
  TERM_AUTH_CHECKING = false;
  if (!TERM_AUTH_PENDING) return;
  const { id, name } = TERM_AUTH_PENDING;
  TERM_AUTH_PENDING = null;
  doOpenTerminal(id, name);
}

// 取消终端认证流程
function cancelTermAuth() {
  TERM_AUTH_CHECKING = false;
  TERM_AUTH_PENDING = null;
  $("termProtocolMask").classList.remove("show");
  $("termSetPwdMask").classList.remove("show");
  $("termVerifyMask").classList.remove("show");
}

// 提交设置终端密码
async function submitTermSetPassword() {
  const pwd = $("termSetPwd").value;
  const pwd2 = $("termSetPwd2").value;
  const errEl = $("termSetPwdErr");

  if (!pwd || !pwd2) {
    errEl.textContent = I18N.t("valid.fill_password");
    errEl.style.display = "block";
    return;
  }
  if (pwd !== pwd2) {
    errEl.textContent = I18N.t("term_auth.password_mismatch");
    errEl.style.display = "block";
    return;
  }
  if (pwd.length < 8) {
    errEl.textContent = I18N.t("term_auth.password_too_short");
    errEl.style.display = "block";
    return;
  }

  try {
    const r = await fetch("/api/user/terminal-password/set", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password: pwd })
    });
    const j = await r.json().catch(() => ({}));
    if (r.ok) {
      TERM_AUTH_VERIFIED = true;
      $("termSetPwdMask").classList.remove("show");
      toast(I18N.t("term_auth.password_set_ok"), "ok");
      proceedToTerminal();
    } else {
      errEl.textContent = j.error || I18N.t("toast.save_failed");
      errEl.style.display = "block";
    }
  } catch (e) {
    errEl.textContent = I18N.t("toast.network_error");
    errEl.style.display = "block";
  }
}

// 提交验证终端密码
async function submitTermVerify() {
  const pwd = $("termVerifyPwd").value;
  const errEl = $("termVerifyErr");
  const attemptsEl = $("termAttemptsInfo");

  if (!pwd) {
    errEl.textContent = I18N.t("valid.enter_password");
    errEl.style.display = "block";
    return;
  }

  try {
    const r = await fetch("/api/user/terminal-password/verify", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password: pwd })
    });
    const j = await r.json().catch(() => ({}));

    if (r.ok) {
      TERM_AUTH_VERIFIED = true;
      $("termVerifyMask").classList.remove("show");
      toast(I18N.t("term_auth.password_verified"), "ok");
      proceedToTerminal();
    } else {
      if (j.locked) {
        // 锁定状态
        $("termVerifyMask").classList.remove("show");
        $("termLockedMask").classList.add("show");
        TERM_AUTH_CHECKING = false;
        TERM_AUTH_PENDING = null;
        return;
      }
      errEl.textContent = j.error || I18N.t("toast.verify_failed");
      errEl.style.display = "block";
      if (typeof j.remaining === "number" && j.remaining > 0) {
        attemptsEl.textContent = I18N.t("term_auth.remaining_attempts") + j.remaining;
        attemptsEl.style.display = "block";
      }
      $("termVerifyPwd").value = "";
      $("termVerifyPwd").focus();
    }
  } catch (e) {
    errEl.textContent = I18N.t("toast.network_error");
    errEl.style.display = "block";
  }
}

// 密码可见性切换
function toggleTermPwdVisibility(inputId, btnId) {
  const input = $(inputId);
  const btn = $(btnId);
  if (!input || !btn) return;
  const isPassword = input.type === "password";
  input.type = isPassword ? "text" : "password";
  btn.querySelector("svg").innerHTML = isPassword
    ? '<path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/>'
    : '<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>';
}

function createTermTab(id, name, tabName) {
  tabName = tabName || name;
  const screens = $("termScreens"), tabbar = $("termTabbar");
  const screen = document.createElement("pre");
  screen.className = "term-screen"; screen.tabIndex = 0; screen.spellcheck = false;
  screens.appendChild(screen);
  const tab = document.createElement("button");
  tab.className = "term-tab";
  tab.innerHTML = `<span>${esc(tabName)}</span><span class="term-tab-close" title="${I18N.t('ui.close_tab')}">×</span>`;
  tabbar.appendChild(tab);
  const vt = makeVT(screen);
  screen._vt = vt;
  /* 移动端虚拟键盘支持：在终端屏幕内注入隐藏 textarea 捕获输入。
     必须在 makeVT() 之后创建——makeVT 会 screen.innerHTML="" 清空子节点。
     <pre> 元素在移动端无法唤起虚拟键盘，<textarea> 可以。 */
  const input = document.createElement("textarea");
  input.className = "term-input";
  input.setAttribute("autocapitalize", "off");
  input.setAttribute("autocorrect", "off");
  input.setAttribute("autocomplete", "off");
  input.setAttribute("spellcheck", "false");
  input.setAttribute("aria-label", I18N.t("misc.terminal_input"));
  input.setAttribute("enterkeyhint", "enter");
  input.setAttribute("rows", "1");
  input.setAttribute("wrap", "off");
  input.readOnly = false;
  screen.appendChild(input);
  const tabObj = {id, hostId: id, name, tabName, ws: null, vt, screenEl: screen, tabEl: tab, inputEl: input, retry: 0, composing: false};
  TERM_TABS.push(tabObj);
  const idx = TERM_TABS.length - 1;
  tab.onclick = (e) => {
    if (e.target.classList.contains("term-tab-close")) { e.stopPropagation(); closeTermTab(TERM_TABS.indexOf(tabObj)); }
    else switchTermTab(TERM_TABS.indexOf(tabObj));
  };
  // 键盘事件绑定到隐藏 textarea（桌面+移动端统一入口）
  input.onkeydown = ev => { ev.stopPropagation(); termKeyDown(ev, tabObj); };
  // 粘贴
  input.onpaste = ev => {
    ev.preventDefault();
    const t = (ev.clipboardData || window.clipboardData).getData("text");
    if (t && tabObj.ws) termSend(tabObj.ws, t);
  };
  // screen 级粘贴兜底：textarea 未聚焦时也能接收粘贴
  screen.addEventListener("paste", ev => {
    if (document.activeElement === input) return;
    ev.preventDefault();
    input.focus({ preventScroll: true });
    const t = (ev.clipboardData || window.clipboardData).getData("text");
    if (t && tabObj.ws) termSend(tabObj.ws, t);
  });
  // input 事件：移动端虚拟键盘字符输入 + 桌面端可打印字符（termKeyDown 不再处理可打印字符）
  input.addEventListener("input", ev => {
    if (tabObj.composing || ev.isComposing) return; // IME 组合中，等 compositionend
    const text = input.value;
    if (text && tabObj.ws) termSend(tabObj.ws, text);
    input.value = "";
  });
  // IME 组合输入（中文/日文等输入法）
  input.addEventListener("compositionstart", () => { tabObj.composing = true; });
  input.addEventListener("compositionend", ev => {
    tabObj.composing = false;
    if (ev.data && tabObj.ws) termSend(tabObj.ws, ev.data);
    input.value = "";
  });
  // beforeinput 兜底：部分移动浏览器 keydown 不触发 Backspace，用 beforeinput 捕获
  input.addEventListener("beforeinput", ev => {
    if (tabObj.composing) return;
    if (ev.inputType === "deleteContentBackward") {
      ev.preventDefault();
      if (tabObj.ws) termSend(tabObj.ws, "\x7f");
    }
  });
  // mouseup 聚焦隐藏 textarea：在鼠标松开后聚焦，不干扰用户拖拽选区。
  // （mousedown 时 focus() 会让浏览器把 textarea 作为选区上下文，
  //  导致 window.getSelection().rangeCount 变为 0，选区不可见。）
  screen.addEventListener("mouseup", function(ev) {
    // 如果用户刚完成了一次拖拽选区（选中了文本），不要立即聚焦 textarea，
    // 否则会清除选区。仅当用户单纯点击（无选区变化）时聚焦。
    const sel = window.getSelection();
    if (sel && sel.toString().length > 0) return;
    if (document.activeElement !== input) {
      input.focus({ preventScroll: true });
    }
  });
  // 键盘事件委托：当 screen(pre) 被聚焦但 textarea 未聚焦时（例如用户
  // 点击终端后未选中文本），将 keydown 重定向到 textarea，确保 termKeyDown
  // 能够正确处理所有键盘输入。
  screen.addEventListener("keydown", function(ev) {
    if (document.activeElement !== input) {
      input.focus({ preventScroll: true });
      // 重新构造并分发事件到 textarea，让 input.onkeydown 处理
      const newEv = new KeyboardEvent("keydown", {
        key: ev.key, code: ev.code, keyCode: ev.keyCode, which: ev.which,
        ctrlKey: ev.ctrlKey, shiftKey: ev.shiftKey,
        altKey: ev.altKey, metaKey: ev.metaKey,
        repeat: ev.repeat, bubbles: true, cancelable: true
      });
      ev.preventDefault();
      ev.stopPropagation();
      input.dispatchEvent(newEv);
    }
  });
  // <pre> 被直接聚焦时（Tab 键导航），重定向到 textarea
  screen.addEventListener("focus", function() {
    if (input && document.activeElement !== input) input.focus({ preventScroll: true });
  });
  // 右键菜单（暂时禁用，待修复后重新启用）
  // screen.addEventListener("contextmenu", function(ev) {
  //   showTermContextMenu(tabObj, ev);
  // });
  // JS fallback for :focus-within — toggle .term-focused class on screen
  // This ensures cursor blink animation works on iOS Safari where :focus-within
  // may not trigger for opacity:0 elements
  input.addEventListener("focus", function() {
    screen.classList.add("term-focused");
  });
  input.addEventListener("blur", function() {
    screen.classList.remove("term-focused");
  });
  // Mobile keyboard viewport adaptation: when virtual keyboard appears,
  // adjust terminal height to keep cursor visible
  if (window.visualViewport) {
    const vpHandler = function() {
      const mask = $("termMask");
      if (mask && mask.classList.contains("show")) {
        const modal = mask.querySelector(".term-modal");
        if (modal) {
          modal.style.height = window.visualViewport.height + "px";
        }
      }
    };
    window.visualViewport.addEventListener("resize", vpHandler);
    window.visualViewport.addEventListener("scroll", vpHandler);
  }
  switchTermTab(idx);
  $("termMask").classList.remove("maximized");
  const mb = $("termMaxBtn"); if (mb) mb.title = I18N.t("ui.maximize_window");
  $("termMask").classList.add("show");
  connectTermWS(tabObj);
}

function connectTermWS(tab) {
  const screen = tab.screenEl, vt = tab.vt;
  setTermStatus(tab.retry > 0 ? `${I18N.t("misc.reconnecting")}(${tab.retry}/3)` : I18N.t("ui.connecting"), "");
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const ws = new WebSocket(`${proto}//${location.host}/api/v1/hosts/${encodeURIComponent(tab.id)}/terminal`);
  ws.binaryType = "arraybuffer";
  tab.ws = ws;
  const doResize = () => { const s = vt.fit(); if (s && ws.readyState === 1) termResizeSend(ws, s.cols, s.rows); };
  ws.onopen = () => { tab.retry = 0; setTermStatus(I18N.t("ui.connected"), "on");
    // 更新 dock 卡片状态
    const dockItem = $("termDock") && $("termDock").querySelector(`[data-tab-id="${CSS.escape(tab.id)}"]`);
    if (dockItem) { const dot = dockItem.querySelector(".dock-dot"); if (dot) { dot.className = "dock-dot on"; } }
    if (tab.inputEl) tab.inputEl.focus({ preventScroll: true }); else screen.focus(); requestAnimationFrame(doResize); };
  ws.onmessage = ev => {
    const data = new Uint8Array(ev.data);
    // Check for ZMODEM/file-transfer frame: [0xFF][0xFE][type][len:4 BE][payload]
    if (data.length >= 7 && data[0] === 0xFF && data[1] === 0xFE) {
      handleZmBrowserFrame(tab, data);
      return;
    }
    // Normal PTY output
    const text = (typeof ev.data === "string") ? ev.data : vt.dec.decode(data, { stream: true });
    vt.feed(text);
  };
  ws.onclose = () => {
    setTermStatus(I18N.t("ui.disconnected"), "off");
    if (tab.ws === ws) tab.ws = null;
    // 更新 dock 卡片状态
    const dockItem = $("termDock") && $("termDock").querySelector(`[data-tab-id="${CSS.escape(tab.id)}"]`);
    if (dockItem) { const dot = dockItem.querySelector(".dock-dot"); if (dot) { dot.className = "dock-dot off"; } }

  };
  ws.onerror = () => setTermStatus(I18N.t("ui.connect_error"), "off");
}

function switchTermTab(idx) {
  if (idx < 0 || idx >= TERM_TABS.length) return;
  TERM_ACTIVE = idx;
  TERM_TABS.forEach((t, i) => { t.tabEl.classList.toggle("active", i === idx); t.screenEl.classList.toggle("active", i === idx); });
  $("termTitle").textContent = (TERM_TABS[idx].tabName || TERM_TABS[idx].name) + " " + I18N.t("term.title");
  requestAnimationFrame(() => { const t = TERM_TABS[idx]; if (t && t.inputEl) t.inputEl.focus({ preventScroll: true }); else if (t) t.screenEl.focus(); });
  if (TERM_RESIZE) window.removeEventListener("resize", TERM_RESIZE);
  TERM_RESIZE = () => termRefit();
  window.addEventListener("resize", TERM_RESIZE);
}

function closeTermTab(idx) {
  if (idx < 0 || idx >= TERM_TABS.length) return;
  const tab = TERM_TABS[idx];
  if (tab.ws) { try { tab.ws.close(); } catch(e) {} }
  tab.screenEl.remove(); tab.tabEl.remove();
  // 清理对应的 dock 卡片
  TERM_DOCK_IDS.delete(tab.id);
  const dockItem = $("termDock") && $("termDock").querySelector(`[data-tab-id="${CSS.escape(tab.id)}"]`);
  if (dockItem) dockItem.remove();
  TERM_TABS.splice(idx, 1);
  if (TERM_ACTIVE >= TERM_TABS.length) TERM_ACTIVE = TERM_TABS.length - 1;
  if (TERM_ACTIVE >= 0) switchTermTab(TERM_ACTIVE);
  else { $("termMask").classList.remove("show"); if (TERM_RESIZE) { window.removeEventListener("resize", TERM_RESIZE); TERM_RESIZE = null; } }
  updateTermDock();
}

function closeAllTermTabs() {
  TERM_TABS.forEach(t => { if (t.ws) { try { t.ws.close(); } catch(e) {} } });
  TERM_TABS = []; TERM_ACTIVE = -1;
  const sc = $("termScreens"); if (sc) sc.innerHTML = "";
  const tb = $("termTabbar"); if (tb) tb.innerHTML = "";
  if (TERM_RESIZE) { window.removeEventListener("resize", TERM_RESIZE); TERM_RESIZE = null; }
  clearTermDock();
}

/* ---------- 终端重连 ---------- */
function reconnectTermTab(tab) {
  if (!tab) {
    if (TERM_ACTIVE < 0 || !TERM_TABS[TERM_ACTIVE]) return;
    tab = TERM_TABS[TERM_ACTIVE];
  }
  // 关闭旧连接
  if (tab.ws) { try { tab.ws.close(); } catch(e) {} tab.ws = null; }
  tab.retry = 0;
  connectTermWS(tab);
}

/* ---------- 文件上传/下载（按钮交互） ---------- */
function startTermFileUpload() {
  if (TERM_ACTIVE < 0 || !TERM_TABS[TERM_ACTIVE]) return;
  const tab = TERM_TABS[TERM_ACTIVE];
  if (!tab.ws || tab.ws.readyState !== 1) { toast(I18N.t("term.not_connected"), "err"); return; }
  // 先弹出目标目录输入（默认 /tmp/），文件名将在选择文件后自动拼接
  const targetDir = prompt("请输入远程目标目录（如 /tmp/ 或 /home/user/）：", "/tmp/");
  if (!targetDir || !targetDir.trim()) return;
  // 确保目录以 / 结尾
  const dir = targetDir.trim().replace(/\/+$/, "") + "/";
  // 弹出文件选择器
  const input = document.createElement("input");
  input.type = "file";
  input.style.position = "fixed";
  input.style.left = "-9999px";
  input.style.top = "-9999px";
  document.body.appendChild(input);
  input.onchange = async () => {
    const file = input.files[0];
    document.body.removeChild(input);
    if (!file) return;
    if (file.size > 100 * 1024 * 1024) {
      toast(I18N.t("term.file_too_large"), "err");
      return;
    }
    // 自动拼接目标目录 + 文件名
    const targetPath = dir + file.name;
    toast(I18N.t("term.uploading") + ": " + file.name + " → " + targetPath + " (" + formatZmSize(file.size) + ")", "info");
    try {
      // 发送上传元数据 'f' 帧
      const meta = JSON.stringify({ filename: file.name, size: file.size, target_path: targetPath });
      const metaBytes = new TextEncoder().encode(meta);
      const metaFrame = new Uint8Array(metaBytes.length + 1);
      metaFrame[0] = 0x66; // 'f'
      metaFrame.set(metaBytes, 1);
      tab.ws.send(metaFrame);
      // 确认 WebSocket 仍然连接
      if (tab.ws.readyState !== 1) { toast(I18N.t("term.upload_cancelled"), "err"); return; }
      // 分块发送文件数据
      const buf = await file.arrayBuffer();
      const data = new Uint8Array(buf);
      const chunkSize = 32 * 1024;
      let bytesSent = 0;
      for (let offset = 0; offset < data.length; offset += chunkSize) {
        const end = Math.min(offset + chunkSize, data.length);
        termSendUpload(tab.ws, data.slice(offset, end));
        bytesSent = end;
        // 每 128KB 让出主线程，避免阻塞 WebSocket 发送缓冲区
        if (offset % (chunkSize * 4) === 0 && offset > 0) {
          await new Promise(r => setTimeout(r, 0));
        }
      }
      // 等待最后一帧被 WebSocket 发送完毕
      await new Promise(r => setTimeout(r, 50));
      termSendEnd(tab.ws);
      tab._uploadTarget = targetPath;
    } catch (err) {
      toast(I18N.t("term.upload_failed") + ": " + err.message, "err");
    }
  };
  // 使用 setTimeout 确保 prompt 关闭后浏览器恢复用户手势上下文
  setTimeout(() => input.click(), 150);
}

function startTermFileDownload() {
  if (TERM_ACTIVE < 0 || !TERM_TABS[TERM_ACTIVE]) return;
  const tab = TERM_TABS[TERM_ACTIVE];
  if (!tab.ws || tab.ws.readyState !== 1) { toast(I18N.t("term.not_connected"), "err"); return; }
  const remotePath = prompt("请输入远程文件路径（如 /var/log/syslog）：", "/tmp/");
  if (!remotePath || !remotePath.trim()) return;
  toast(`正在请求下载: ${remotePath.trim()}`, "info");
  // 发送下载请求 'd' 帧
  const meta = JSON.stringify({ remote_path: remotePath.trim() });
  const metaBytes = new TextEncoder().encode(meta);
  const metaFrame = new Uint8Array(metaBytes.length + 1);
  metaFrame[0] = 0x64; // 'd'
  metaFrame.set(metaBytes, 1);
  tab.ws.send(metaFrame);
  // 准备接收下载数据
  tab.fileDownload = { filename: remotePath.split("/").pop() || "download.dat", chunks: [], received: 0 };
}

/* ---------- 终端收起到右下角 ---------- */
let TERM_DOCK_IDS = new Set();  // 收起的 tab id 集合

function minimizeTerminal() {
  if (TERM_TABS.length === 0) return;
  TERM_TABS.forEach(t => TERM_DOCK_IDS.add(t.id));
  const mask = $("termMask");
  if (mask) {
    const modal = mask.querySelector(".term-modal");
    if (modal) {
      modal.style.transition = "transform .2s ease, opacity .2s ease";
      modal.style.transform = "scale(.92) translateY(20px)";
      modal.style.opacity = "0";
      setTimeout(() => {
        mask.classList.remove("show", "maximized");
        modal.style.transition = "";
        modal.style.transform = "";
        modal.style.opacity = "";
      }, 200);
    } else {
      mask.classList.remove("show", "maximized");
    }
  }
  if (TERM_RESIZE) { window.removeEventListener("resize", TERM_RESIZE); TERM_RESIZE = null; }
  setTimeout(updateTermDock, 200);
}

function updateTermDock() {
  const dock = $("termDock"); if (!dock) return;
  // 移除已不存在的 tab 对应的卡片
  dock.querySelectorAll(".term-dock-item").forEach(el => {
    if (!TERM_TABS.find(t => t.id === el.dataset.tabId)) el.remove();
  });
  // 为每个收起的 tab 创建/更新卡片
  const docked = TERM_TABS.filter(t => TERM_DOCK_IDS.has(t.id));
  dock.style.display = docked.length > 0 ? "flex" : "none";
  docked.forEach(tab => {
    let item = dock.querySelector(`[data-tab-id="${CSS.escape(tab.id)}"]`);
    if (!item) {
      item = document.createElement("div");
      item.className = "term-dock-item";
      item.dataset.tabId = tab.id;
      item.innerHTML = `
        <span class="dock-dot"></span>
        <span class="dock-name"></span>
        <button class="dock-btn" title="${I18N.t('ui.expand_window')}" aria-label="${I18N.t('ui.expand_window')}">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 19V13a1 1 0 0 1 1-1h12"/><path d="M12 5l-5 7 5-7"/></svg>
        </button>
        <button class="dock-btn close" title="${I18N.t('ui.close_session')}" aria-label="${I18N.t('ui.close_session')}">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6L6 18M6 6l12 12"/></svg>
        </button>`;
      // 点击卡片主体（非按钮）也展开
      item.addEventListener("click", e => {
        if (e.target.closest(".dock-btn")) return;
        expandTermFromDock(tab.id);
      });
      item.addEventListener("dblclick", () => expandTermFromDock(tab.id));
      // 展开按钮
      item.querySelector(".dock-btn:not(.close)").addEventListener("click", e => {
        e.stopPropagation(); expandTermFromDock(tab.id);
      });
      // 关闭按钮
      item.querySelector(".dock-btn.close").addEventListener("click", e => {
        e.stopPropagation(); closeTermFromDock(tab.id);
      });
      dock.appendChild(item);
    }
    // 更新主机名 + tooltip
    const nameEl = item.querySelector(".dock-name");
    if (nameEl) {
      nameEl.textContent = tab.tabName || tab.name;
      item.title = (tab.tabName || tab.name) + " · " + I18N.t("ui.remote_terminal");
    }
    // 更新连接状态
    const dot = item.querySelector(".dock-dot");
    if (dot) {
      dot.className = "dock-dot";
      if (tab.ws && tab.ws.readyState === 1) dot.classList.add("on");
      else if (tab.ws && tab.ws.readyState === 3) dot.classList.add("off");
    }
  });
}

function expandTermFromDock(tabId) {
  const idx = TERM_TABS.findIndex(t => t.id === tabId);
  if (idx < 0) return;
  TERM_DOCK_IDS.delete(tabId);
  switchTermTab(idx);
  const mask = $("termMask");
  const modal = mask.querySelector(".term-modal");
  if (modal) {
    modal.style.transition = "transform .22s cubic-bezier(.34,1.56,.64,1), opacity .22s ease";
    modal.style.transform = "scale(.94)";
    modal.style.opacity = "0";
    mask.classList.add("show");
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        modal.style.transform = "scale(1)";
        modal.style.opacity = "1";
        setTimeout(() => {
          modal.style.transition = "";
          modal.style.transform = "";
          modal.style.opacity = "";
        }, 250);
      });
    });
  } else {
    mask.classList.add("show");
  }
  requestAnimationFrame(() => requestAnimationFrame(termRefit));
  updateTermDock();
}

function closeTermFromDock(tabId) {
  const idx = TERM_TABS.findIndex(t => t.id === tabId);
  if (idx < 0) return;
  const tab = TERM_TABS[idx];
  // Close WS without triggering switchTermTab (modal is hidden — minimized state)
  if (tab.ws) { try { tab.ws.close(); } catch(e) {} }
  tab.screenEl.remove();
  tab.tabEl.remove();
  TERM_DOCK_IDS.delete(tabId);
  // Animate dock card removal
  const item = $("termDock") && $("termDock").querySelector(`[data-tab-id="${CSS.escape(tabId)}"]`);
  if (item) {
    item.classList.add("removing");
    setTimeout(() => { item.remove(); updateTermDock(); }, 200);
  }
  // Remove from array — but DON'T call switchTermTab (modal is hidden)
  TERM_TABS.splice(idx, 1);
  if (TERM_ACTIVE >= TERM_TABS.length) TERM_ACTIVE = TERM_TABS.length - 1;
  // If no tabs left, clean up fully
  if (TERM_TABS.length === 0) {
    TERM_ACTIVE = -1;
    const mask = $("termMask");
    if (mask) mask.classList.remove("show", "maximized");
    if (TERM_RESIZE) { window.removeEventListener("resize", TERM_RESIZE); TERM_RESIZE = null; }
  }
  // Immediate dock update (the animated removal happens in 200ms)
  updateTermDock();
}

function clearTermDock() {
  TERM_DOCK_IDS.clear();
  const dock = $("termDock");
  if (dock) { dock.innerHTML = ""; dock.style.display = "none"; }
}

/* ---------- 终端会话管理（列表 / 回放 / 旁观） ---------- */
let TERM_SESSIONS_TIMER = null;

function openTerminalSessions() {
  $("termSessionsMask").classList.add("show");
  loadTerminalSessions();
  if (TERM_SESSIONS_TIMER) clearInterval(TERM_SESSIONS_TIMER);
  TERM_SESSIONS_TIMER = setInterval(loadTerminalSessions, 3000);
}

let LAST_TERM_SESSIONS = [];
let TERM_SEARCH = "";

function loadTerminalSessions() {
  const mask = $("termSessionsMask");
  if (!mask || !mask.classList.contains("show")) {
    if (TERM_SESSIONS_TIMER) { clearInterval(TERM_SESSIONS_TIMER); TERM_SESSIONS_TIMER = null; }
    return;
  }
  fetch(`${API}/terminal/sessions`).then(r => r.json()).then(sessions => {
    LAST_TERM_SESSIONS = sessions || [];
    renderTerminalSessions(LAST_TERM_SESSIONS);
  }).catch(e => console.warn("load sessions:", e));
}

function renderTerminalSessions(sessions) {
  const c = $("termSessionsList");
  if (!c) return;
  // 按搜索关键词过滤
  const q = TERM_SEARCH.trim().toLowerCase();
  const filtered = q ? sessions.filter(s => {
    return (s.operator || "").toLowerCase().includes(q) ||
           (s.hostname || "").toLowerCase().includes(q) ||
           (s.ip || "").toLowerCase().includes(q);
  }) : sessions;
  // 更新计数
  const cnt = $("termSessionCount");
  if (cnt) {
    cnt.textContent = q ? `${filtered.length}/${sessions.length} 条` : `${sessions.length} 条`;
  }
  if (filtered.length === 0) {
    c.innerHTML = `<div style="text-align:center; color:var(--muted2); padding:32px 0">${q ? I18N.t("empty.no_terminal_match") : I18N.t("empty.no_active_sessions")}</div>`;
    return;
  }
  c.innerHTML = filtered.map(s => {
    const t = new Date(s.created_at * 1000);
    const time = `${String(t.getHours()).padStart(2,'0')}:${String(t.getMinutes()).padStart(2,'0')}:${String(t.getSeconds()).padStart(2,'0')}`;
    const ipStr = s.ip ? ` · IP ${esc(s.ip)}` : "";
    return `<div class="term-session-item">
      <div class="term-session-info">
        <div class="term-session-host">${esc(s.hostname)}</div>
        <div class="term-session-meta">${I18N.t("section.operator")} <strong style="color:var(--accent-txt)">${esc(s.operator)}</strong>${ipStr}${I18N.t("section.start_label")}${time} · ${s.frames} ${I18N.t("ui.frames_recorded")}</div>
      </div>
      ${s.observers > 0 ? `<span class="term-session-badge observers">${s.observers} ${I18N.t("ui.observe")}</span>` : `<span class="term-session-badge">${I18N.t("ui.active")}</span>`}
      <div class="term-session-actions">
        <button class="btn sm" onclick="openTerminalObserve('${s.id}','${esc(s.hostname)}')">${I18N.t("ui.observe")}</button>
        <button class="btn sm" onclick="openTerminalReplay('${s.id}','${esc(s.hostname)}')">${I18N.t("ui.replay")}</button>
      </div>
    </div>`;
  }).join("");
}

/* ---------- 终端回放 ---------- */
let REPLAY = null; // {frames, idx, vt, timer, playing, speed}

function openTerminalReplay(sessionId, hostname) {
  fetch(`${API}/terminal/sessions/${encodeURIComponent(sessionId)}/replay`)
    .then(r => r.json())
    .then(data => {
      // Replay OUTPUT frames (shell output) + RESIZE frames (terminal dimension changes).
      // INPUT frames are excluded: the shell output already contains the command echo.
      const frames = (data.frames || []).filter(f => f.type === "output" || f.type === "resize");
      if (frames.length === 0) { toast(I18N.t("empty.no_recording"), "err"); return; }
      // 从第一个 resize 帧获取录制时的初始终端尺寸
      let initCols = 80, initRows = 24;
      for (const f of frames) {
        if (f.type === "resize") {
          try {
            const parts = atob(f.data).split("x");
            const c = parseInt(parts[0]), r = parseInt(parts[1]);
            if (c >= 20 && r >= 6) { initCols = c; initRows = r; }
          } catch (e) {}
          break;
        }
      }
      $("termReplayTitle").textContent = hostname + " " + I18N.t("term.replay_title");
      const screen = $("termReplayScreen");
      const vt = makeVT(screen);
      // 用录制时的终端尺寸初始化 VT，避免 80x24 默认值导致换行错位
      if (initCols !== 80 || initRows !== 24) {
        vt.resizeTo(initCols, initRows);
      }
      screen._vt = vt;
      REPLAY = {frames, idx: 0, vt, timer: null, playing: false, speed: 2};
      $("termReplayMask").classList.add("show");
      $("termSessionsMask").classList.remove("show");
      if (TERM_SESSIONS_TIMER) { clearInterval(TERM_SESSIONS_TIMER); TERM_SESSIONS_TIMER = null; }
      document.querySelectorAll(".replay-speed-btn").forEach(b => {
        b.classList.toggle("active", parseFloat(b.dataset.speed) === 2);
      });
      updateReplayProgress();
      playReplay();
    })
    .catch(e => toast(I18N.t("toast.load_replay_failed") + e, "err"));
}

function playReplay() {
  if (!REPLAY || REPLAY.playing) return;
  REPLAY.playing = true;
  const btn = $("replayPlayBtn"); if (btn) btn.textContent = "⏸";
  const st = $("replayStatus"); if (st) { st.textContent = I18N.t("ui.playing"); st.className = "term-status on"; }
  scheduleNextFrame();
}

function pauseReplay() {
  if (!REPLAY) return;
  REPLAY.playing = false;
  if (REPLAY.timer) { clearTimeout(REPLAY.timer); REPLAY.timer = null; }
  const btn = $("replayPlayBtn"); if (btn) btn.textContent = "▶";
  const st = $("replayStatus"); if (st) { st.textContent = I18N.t("ui.paused"); st.className = "term-status"; }
}

function scheduleNextFrame() {
  if (!REPLAY || !REPLAY.playing) return;
  if (REPLAY.idx >= REPLAY.frames.length) {
    REPLAY.playing = false;
    const btn = $("replayPlayBtn"); if (btn) btn.textContent = "▶";
    const st = $("replayStatus"); if (st) { st.textContent = I18N.t("ui.playback_done"); st.className = "term-status"; }
    updateReplayProgress();
    return;
  }
  const frame = REPLAY.frames[REPLAY.idx];
  const bytes = Uint8Array.from(atob(frame.data), c => c.charCodeAt(0));
  if (frame.type === "resize") {
    // resize 帧：解析 cols/rows 并调整 VT 网格，不 feed 文本
    const parts = new TextDecoder().decode(bytes).split("x");
    const c = parseInt(parts[0]), r = parseInt(parts[1]);
    if (c >= 20 && r >= 6) REPLAY.vt.resizeTo(c, r);
  } else {
    const text = REPLAY.vt.dec.decode(bytes, { stream: true });
    REPLAY.vt.feed(text);
  }
  REPLAY.idx++;
  updateReplayProgress();
  let delay = 0;
  if (REPLAY.idx < REPLAY.frames.length) {
    const next = REPLAY.frames[REPLAY.idx];
    delay = (next.ts - frame.ts) / REPLAY.speed;
    delay = Math.min(Math.max(delay, 1), 3000 / REPLAY.speed);
  }
  REPLAY.timer = setTimeout(scheduleNextFrame, delay);
}

function setReplaySpeed(speed) {
  if (!REPLAY) return;
  REPLAY.speed = speed;
  document.querySelectorAll(".replay-speed-btn").forEach(b => {
    b.classList.toggle("active", parseFloat(b.dataset.speed) === speed);
  });
}

function seekReplay(progress) {
  if (!REPLAY) return;
  pauseReplay();
  const targetIdx = Math.floor(progress * REPLAY.frames.length);
  // 从头回放：先用第一个 resize 帧确定初始尺寸
  let initCols = 80, initRows = 24;
  for (const f of REPLAY.frames) {
    if (f.type === "resize") {
      try {
        const parts = atob(f.data).split("x");
        const c = parseInt(parts[0]), r = parseInt(parts[1]);
        if (c >= 20 && r >= 6) { initCols = c; initRows = r; }
      } catch (e) {}
      break;
    }
  }
  const screen = $("termReplayScreen");
  const vt = makeVT(screen);
  if (initCols !== 80 || initRows !== 24) vt.resizeTo(initCols, initRows);
  screen._vt = vt;
  REPLAY.vt = vt;
  REPLAY.idx = 0;
  for (let i = 0; i < targetIdx; i++) {
    const frame = REPLAY.frames[i];
    const bytes = Uint8Array.from(atob(frame.data), c => c.charCodeAt(0));
    if (frame.type === "resize") {
      const parts = new TextDecoder().decode(bytes).split("x");
      const c = parseInt(parts[0]), r = parseInt(parts[1]);
      if (c >= 20 && r >= 6) vt.resizeTo(c, r);
    } else {
      const text = vt.dec.decode(bytes, { stream: true });
      vt.feed(text);
    }
  }
  REPLAY.idx = targetIdx;
  updateReplayProgress();
}

function updateReplayProgress() {
  if (!REPLAY) return;
  const pct = REPLAY.frames.length > 0 ? (REPLAY.idx / REPLAY.frames.length) * 100 : 0;
  const bar = $("replayProgress"); if (bar) bar.style.width = pct + "%";
  const time = $("replayTime"); if (time) time.textContent = `${REPLAY.idx} / ${REPLAY.frames.length} 帧`;
}

function closeReplay() { pauseReplay(); REPLAY = null; }

/* ---------- 终端只读旁观 ---------- */
let OBSERVE_WS = null;

function openTerminalObserve(sessionId, hostname) {
  const screen = $("termObserveScreen");
  const vt = makeVT(screen);
  screen._vt = vt;
  $("termObserveTitle").textContent = hostname + " " + I18N.t("term.observe_title");
  setObserveStatus(I18N.t("ui.connecting"), "");
  $("termObserveMask").classList.add("show");
  $("termSessionsMask").classList.remove("show");
  if (TERM_SESSIONS_TIMER) { clearInterval(TERM_SESSIONS_TIMER); TERM_SESSIONS_TIMER = null; }
  closeObserveWS();
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const ws = new WebSocket(`${proto}//${location.host}/api/v1/terminal/sessions/${encodeURIComponent(sessionId)}/observe`);
  ws.binaryType = "arraybuffer";
  OBSERVE_WS = ws;
  ws.onopen = () => setObserveStatus(I18N.t("ui.observing"), "on");
  ws.onmessage = ev => {
    const text = (typeof ev.data === "string") ? ev.data : vt.dec.decode(new Uint8Array(ev.data), { stream: true });
    vt.feed(text);
  };
  ws.onclose = () => setObserveStatus(I18N.t("ui.session_ended"), "off");
  ws.onerror = () => setObserveStatus(I18N.t("ui.connect_error"), "off");
}

function closeObserveWS() {
  if (OBSERVE_WS) { try { OBSERVE_WS.close(); } catch(e) {} OBSERVE_WS = null; }
}

function setObserveStatus(txt, cls) {
  const s = $("observeStatus"); if (s) { s.textContent = txt; s.className = "term-status" + (cls ? " " + cls : ""); }
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
// 重新测量终端并把新尺寸告知 PTY（放大/还原/窗口变化后调用）
function termRefit() {
  if (TERM_ACTIVE < 0 || !TERM_TABS[TERM_ACTIVE]) return;
  const tab = TERM_TABS[TERM_ACTIVE];
  if (tab.vt && tab.ws) { const s = tab.vt.fit(); if (s && tab.ws.readyState === 1) termResizeSend(tab.ws, s.cols, s.rows); }
}
// 放大 / 还原 终端窗口
safeAddEventListener("termMaxBtn", "click", () => {
  const mask = $("termMask"); if (!mask) return;
  const max = mask.classList.toggle("maximized");
  const btn = $("termMaxBtn"); if (btn) btn.title = max ? I18N.t("ui.restore_size") : I18N.t("ui.maximize_window");
  requestAnimationFrame(() => requestAnimationFrame(termRefit)); // 等布局稳定后再测量
});
// 收起到右下角
safeAddEventListener("termMinBtn", "click", () => {
  minimizeTerminal();
});
// 文件上传
safeAddEventListener("termUploadBtn", "click", () => startTermFileUpload());
// 文件下载
safeAddEventListener("termDownloadBtn", "click", () => startTermFileDownload());

/* ---------- v5.3.0: 终端二次认证弹窗事件绑定 ---------- */
// 协议弹窗：勾选启用继续按钮
safeAddEventListener("termProtocolAgree", "change", function() {
  $("termProtocolContinue").disabled = !this.checked;
});
// 协议弹窗：同意并继续
safeAddEventListener("termProtocolContinue", "click", onTermProtocolAgreed);
// 协议弹窗：关闭（取消）
$("termProtocolMask").addEventListener("click", function(e) {
  if (e.target === this || e.target.closest("[data-close-btn]")) {
    cancelTermAuth();
  }
});

// 密码设置弹窗：提交
safeAddEventListener("termSetPwdBtn", "click", submitTermSetPassword);
// 密码设置弹窗：取消
safeAddEventListener("termSetPwdCancel", "click", function() {
  $("termSetPwdMask").classList.remove("show");
  cancelTermAuth();
});
$("termSetPwdMask").addEventListener("click", function(e) {
  if (e.target === this || e.target.closest("[data-close-btn]")) {
    $("termSetPwdMask").classList.remove("show");
    cancelTermAuth();
  }
});
// 密码设置弹窗：回车提交
safeAddEventListener("termSetPwd", "keydown", function(e) { if (e.key === "Enter") submitTermSetPassword(); });
safeAddEventListener("termSetPwd2", "keydown", function(e) { if (e.key === "Enter") submitTermSetPassword(); });
// 密码设置弹窗：显示/隐藏密码
safeAddEventListener("termSetPwdToggle", "click", function() { toggleTermPwdVisibility("termSetPwd", "termSetPwdToggle"); });
safeAddEventListener("termSetPwd2Toggle", "click", function() { toggleTermPwdVisibility("termSetPwd2", "termSetPwd2Toggle"); });

// 密码验证弹窗：提交
safeAddEventListener("termVerifyBtn", "click", submitTermVerify);
// 密码验证弹窗：取消
safeAddEventListener("termVerifyCancel", "click", function() {
  $("termVerifyMask").classList.remove("show");
  cancelTermAuth();
});
$("termVerifyMask").addEventListener("click", function(e) {
  if (e.target === this || e.target.closest("[data-close-btn]")) {
    $("termVerifyMask").classList.remove("show");
    cancelTermAuth();
  }
});
// 密码验证弹窗：回车提交
safeAddEventListener("termVerifyPwd", "keydown", function(e) { if (e.key === "Enter") submitTermVerify(); });
// 密码验证弹窗：显示/隐藏密码
safeAddEventListener("termVerifyPwdToggle", "click", function() { toggleTermPwdVisibility("termVerifyPwd", "termVerifyPwdToggle"); });

// 锁定弹窗：关闭
$("termLockedMask").addEventListener("click", function(e) {
  if (e.target === this || e.target.closest("[data-close-btn]")) {
    $("termLockedMask").classList.remove("show");
    cancelTermAuth();
  }
});

function setTermStatus(txt, cls) {
  const s = $("termStatus"); if (s) { s.textContent = txt; s.className = "term-status" + (cls ? " " + cls : ""); }
  // 同步更新当前活动 tab 的 dock 卡片状态
  if (TERM_ACTIVE >= 0 && TERM_TABS[TERM_ACTIVE]) {
    const tab = TERM_TABS[TERM_ACTIVE];
    const item = $("termDock") && $("termDock").querySelector(`[data-tab-id="${CSS.escape(tab.id)}"]`);
    if (item) {
      const dot = item.querySelector(".dock-dot");
      if (dot) {
        dot.className = "dock-dot";
        if (cls === "on") dot.classList.add("on");
        else if (cls === "off") dot.classList.add("off");
      }
    }
  }
}
function closeTerminalWS() { closeAllTermTabs(); }
// 发送输入（帧首字节 'i' 标识 input）
function termSend(ws, str) {
  if (!ws || ws.readyState !== 1) return;
  const body = new TextEncoder().encode(str);
  const framed = new Uint8Array(body.length + 1);
  framed[0] = 0x69; // 'i'
  framed.set(body, 1);
  ws.send(framed);
}
// 发送上传数据块（帧首字节 'u'）
function termSendUpload(ws, chunk) {
  if (!ws || ws.readyState !== 1) return;
  const framed = new Uint8Array(chunk.length + 1);
  framed[0] = 0x75; // 'u'
  framed.set(chunk, 1);
  ws.send(framed);
}
// 发送上传结束信号（帧首字节 'e'）
function termSendEnd(ws) {
  if (!ws || ws.readyState !== 1) return;
  ws.send(new Uint8Array([0x65])); // 'e'
}
// ---- ZMODEM/文件传输 浏览器端帧处理 ----
// handleZmBrowserFrame 解析并处理来自 Agent 的 ZMODEM/文件传输帧。
// 帧格式: [0xFF][0xFE][type][len:4 BE][payload]
function handleZmBrowserFrame(tab, data) {
  const zmType = data[2];
  const zmLen = (data[3] << 24) | (data[4] << 16) | (data[5] << 8) | data[6];
  const zmPayload = data.slice(7, 7 + zmLen);
  switch (zmType) {
    case 0x5A: { // 'Z' — ZMODEM 信号
      const info = new TextDecoder().decode(zmPayload);
      let meta;
      try { meta = JSON.parse(info); } catch (e) { return; }
      if (meta.type === "sz") {
        // 下载：准备接收文件数据
        tab.zmDownload = { filename: meta.filename || "download.dat", size: meta.size || 0, chunks: [], received: 0 };
        toast(I18N.t("term.downloading") + ": " + tab.zmDownload.filename + " (" + formatZmSize(tab.zmDownload.size) + ")", "info");
      } else if (meta.type === "rz") {
        // 上传：弹出文件选择对话框
        showZmUploadDialog(tab);
      }
      break;
    }
    case 0x46: { // 'F' — 文件信息（按钮上传ACK或下载元数据）
      const info = new TextDecoder().decode(zmPayload);
      let meta;
      try { meta = JSON.parse(info); } catch (e) { return; }
      if (meta.type === "upload_ack") {
        if (meta.status === "ok") {
          toast(`上传完成: ${meta.filename || ""}`, "ok");
        } else {
          toast(`上传失败: ${meta.message || "未知错误"}`, "err");
        }
      } else if (meta.type === "download_meta") {
        // 下载元数据：准备接收文件数据
        tab.fileDownload = tab.fileDownload || {};
        tab.fileDownload.filename = meta.filename || "download.dat";
        tab.fileDownload.size = meta.size || 0;
        tab.fileDownload.chunks = [];
        tab.fileDownload.received = 0;
        toast(`正在下载: ${meta.filename} (${formatZmSize(meta.size)})`, "info");
      } else if (meta.type === "download_error") {
        toast(`下载失败: ${meta.message || "未知错误"}`, "err");
        tab.fileDownload = null;
      }
      break;
    }
    case 0x44: // 'D' — 下载数据块
      if (tab.zmDownload) {
        tab.zmDownload.chunks.push(zmPayload);
        tab.zmDownload.received += zmPayload.length;
      }
      if (tab.fileDownload) {
        tab.fileDownload.chunks.push(zmPayload);
        tab.fileDownload.received += zmPayload.length;
      }
      break;
    case 0x45: // 'E' — 传输完成
      if (tab.zmDownload) {
        const dl = tab.zmDownload;
        const blob = new Blob(dl.chunks);
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = dl.filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        setTimeout(() => URL.revokeObjectURL(url), 1000);
        toast(I18N.t("term.download_done") + ": " + dl.filename, "ok");
        tab.zmDownload = null;
      }
      if (tab.fileDownload) {
        const dl = tab.fileDownload;
        const blob = new Blob(dl.chunks);
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = dl.filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        setTimeout(() => URL.revokeObjectURL(url), 1000);
        toast(`下载完成: ${dl.filename}`, "ok");
        tab.fileDownload = null;
      }
      break;
  }
}
// showZmUploadDialog 弹出文件选择对话框，读取文件并通过 WebSocket 上传。
function showZmUploadDialog(tab) {
  const input = document.createElement("input");
  input.type = "file";
  input.style.display = "none";
  document.body.appendChild(input);
  input.onchange = async () => {
    const file = input.files[0];
    document.body.removeChild(input);
    if (!file) return;
    if (file.size > 100 * 1024 * 1024) {
      toast(I18N.t("term.file_too_large"), "err");
      return;
    }
    toast(I18N.t("term.uploading") + ": " + file.name + " (" + formatZmSize(file.size) + ")", "info");
    try {
      const buf = await file.arrayBuffer();
      const data = new Uint8Array(buf);
      const chunkSize = 32 * 1024; // 32KB chunks
      for (let offset = 0; offset < data.length; offset += chunkSize) {
        const end = Math.min(offset + chunkSize, data.length);
        termSendUpload(tab.ws, data.slice(offset, end));
      }
      termSendEnd(tab.ws);
    } catch (err) {
      toast(I18N.t("term.upload_failed") + ": " + err.message, "err");
    }
  };
  input.click();
}
// formatZmSize 格式化文件大小显示
function formatZmSize(bytes) {
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
  return (bytes / (1024 * 1024)).toFixed(1) + " MB";
}
// getSelectedTermText 获取当前终端屏幕内的选中文本。
// 聚焦在隐藏 textarea 时 window.getSelection().rangeCount 为 0，
// 因为浏览器为表单元素维护独立的选区上下文。此时临时 blur 再检查。
function getSelectedTermText(tab) {
  const sel = window.getSelection();
  if (!sel || sel.rangeCount === 0) {
    // 如果 textarea 聚焦导致 rangeCount=0，临时 blur 后再检查
    const ae = document.activeElement;
    if (ae && ae.classList.contains("term-input")) {
      ae.blur();
      try {
        const s2 = window.getSelection();
        if (s2 && s2.rangeCount > 0) {
          const t = s2.toString();
          if (t) return t;
        }
      } finally {
        ae.focus({ preventScroll: true });
      }
    }
    return "";
  }
  // 方法1：直接取全局选区文本（大多数情况足够）
  const text = sel.toString();
  if (text) return text;
  // 方法2：检查 range 是否落在当前 tab 的 screen 内
  if (!tab || !tab.screenEl) return "";
  for (let i = 0; i < sel.rangeCount; i++) {
    const rng = sel.getRangeAt(i);
    if (tab.screenEl.contains(rng.commonAncestorContainer)) {
      return rng.toString();
    }
  }
  return "";
}
// ---- 全局 copy 事件处理（终端文本复制）----
// 仅注册一次。当用户通过右键菜单或 Ctrl+C 触发 copy 事件时，
// 临时 blur 隐藏 textarea 让 window.getSelection() 可见，然后写入剪贴板。
(function() {
  document.addEventListener("copy", function(ev) {
    const activeTab = TERM_ACTIVE >= 0 ? TERM_TABS[TERM_ACTIVE] : null;
    if (!activeTab || !activeTab.screenEl) return;
    const mask = document.getElementById("termMask");
    if (!mask || !mask.classList.contains("show")) return;
    // 临时 blur textarea 使 pre 中的选区对 window.getSelection() 可见
    const ae = document.activeElement;
    const hasTermInput = ae && ae.classList.contains("term-input");
    if (hasTermInput) ae.blur();
    let sel = "";
    try {
      const s = window.getSelection();
      if (s && s.rangeCount > 0) {
        for (let i = 0; i < s.rangeCount; i++) {
          const rng = s.getRangeAt(i);
          if (activeTab.screenEl.contains(rng.commonAncestorContainer)) {
            sel = rng.toString();
            break;
          }
        }
      }
      if (!sel) sel = window.getSelection().toString();
    } finally {
      if (hasTermInput && ae) ae.focus({ preventScroll: true });
    }
    if (!sel) return;
    ev.preventDefault();
    ev.clipboardData.setData("text/plain", sel);
  });
})();

function termKeyDown(e, tab) {
  e.stopPropagation(); // 阻止全局 Esc 关弹窗，让 Esc 等按键传给 shell
  const ws = tab ? tab.ws : null;
  const k = e.key;
  const mod = e.ctrlKey || e.metaKey;

  // ====== P0: 剪贴板双向交互 ======

  // Ctrl+V / Cmd+V / Shift+Insert → 粘贴剪贴板内容
  // 关键：不调用 preventDefault()，让浏览器原生 paste 事件正常触发，
  // 由 input.onpaste / screen paste 兜底处理器捕获文本并发送。
  if ((mod && (k === "v" || k === "V")) || (e.shiftKey && k === "Insert")) {
    return; // 放行浏览器 paste，不发送 \x16
  }

  // Ctrl+C / Cmd+C → 有选区时复制，无选区时发送 SIGINT
  // Ctrl+Shift+C / Cmd+Shift+C / Ctrl+Insert → 强制复制
  if ((mod && (k === "c" || k === "C")) || (mod && k === "Insert")) {
    const shiftCopy = e.shiftKey;

    // 临时 blur textarea 让 document 选区对 window.getSelection() 可见
    const ae = document.activeElement;
    const hasTermInput = ae && ae.classList.contains("term-input");
    if (hasTermInput) ae.blur();

    let sel = "";
    try {
      const s = window.getSelection();
      if (s && s.rangeCount > 0) {
        for (let i = 0; i < s.rangeCount; i++) {
          const rng = s.getRangeAt(i);
          if (tab && tab.screenEl && tab.screenEl.contains(rng.commonAncestorContainer)) {
            sel = rng.toString();
            break;
          }
        }
      }
      if (!sel) sel = window.getSelection().toString();
    } finally {
      if (hasTermInput && ae) ae.focus({ preventScroll: true });
    }

    if (shiftCopy || sel) {
      e.preventDefault();
      if (sel) {
        // 优先使用 navigator.clipboard API（现代浏览器），fallback 到 execCommand
        if (navigator.clipboard && window.isSecureContext) {
          navigator.clipboard.writeText(sel).then(() => {}, () => {});
        } else {
          const ta = document.createElement("textarea");
          ta.value = sel;
          ta.style.cssText = "position:fixed;left:-9999px;top:-9999px;opacity:0";
          document.body.appendChild(ta);
          ta.focus(); ta.select();
          try { document.execCommand("copy"); } catch (_) {}
          document.body.removeChild(ta);
        }
      }
      return;
    }

    // 无选区 → 发送 SIGINT (\x03)
  }

  const ac = (tab && tab.vt && tab.vt.appCursor) ? "\x1bO" : "\x1b["; // 应用光标模式(vim/less…)
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
  }
  // 可打印字符不再由 keydown 处理——改由隐藏 textarea 的 input 事件统一处理。
  // 原因：移动端虚拟键盘的 keydown e.key 常为 "Unidentified"，不可靠；
  //       input 事件在所有平台都能正确获取实际输入文本。
  // 桌面端：keydown 不 preventDefault → 字符进入 textarea → input 事件发送 → 清空 textarea
  // 移动端：keydown 可能不识别 → 同样由 input 事件兜底发送
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
    // screen.contains 涵盖两种焦点来源：<pre> 自身聚焦（桌面直接 Tab）
    // 和隐藏 <textarea> 子元素聚焦（移动端虚拟键盘 / 桌面端统一输入入口）
    const focused = screen.contains(document.activeElement);
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

  // resizeTo — 重新分配网格到指定 cols/rows，保留已有内容
  vt.resizeTo = function(cols, rows) {
    cols = Math.max(20, cols); rows = Math.max(6, rows);
    if (cols === vt.cols && rows === vt.rows) return;
    const old = vt.grid;
    vt.cols = cols; vt.rows = rows; vt.grid = [];
    for (let y = 0; y < rows; y++) {
      const r = newRow();
      if (old && old[y]) for (let x = 0; x < Math.min(cols, old[y].length); x++) r[x] = old[y][x];
      vt.grid.push(r);
    }
    vt.top = 0; vt.bot = rows - 1; vt.cx = clampX(vt.cx); vt.cy = clampY(vt.cy); vt.wrapNext = false;
    scheduleRender();
  };

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
    vt.resizeTo(cols, rows);
    return { cols: vt.cols, rows: vt.rows };
  };
  vt.fullReset = fullReset;
  vt.render = render;
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
    // Custom webhook
    const cw = c.custom_webhook || {};
    $("customWebhookEnabled").checked = !!cw.enabled;
    $("customWebhookURL").value = cw.url || "";
    $("customWebhookMethod").value = cw.method || "POST";
    $("customWebhookContentType").value = cw.content_type || "application/json";
    $("customWebhookHeaders").value = cw.headers || "";
    $("customWebhookBodyTemplate").value = cw.body_template || "";
    // SMTP email config
    const s = c.smtp || {};
    $("smtpEnabled").checked = !!s.smtp_enabled;
    $("smtpHost").value = s.smtp_host || "";
    $("smtpPort").value = s.smtp_port || "";
    $("smtpUsername").value = s.smtp_username || "";
    $("smtpPassword").value = s.smtp_password || "";
    $("smtpFromName").value = s.smtp_from_name || "";
    $("smtpTLS").checked = !!s.smtp_use_tls;
    // Threshold display: treat 0 / null / undefined as "unset" → show the standard
    // default. The backend also backfills these zeros, so display and storage stay
    // consistent, and every metric always shows a sane standard threshold.
    const td = (v, def) => (v == null || v === 0 || isNaN(v)) ? def : v;
    $("cpuWarn").value = td(t.cpu_warn, 80); $("cpuCrit").value = td(t.cpu_crit, 95);
    $("memWarn").value = td(t.mem_warn, 85); $("memCrit").value = td(t.mem_crit, 95);
    $("diskWarn").value = td(t.disk_warn, 80); $("diskCrit").value = td(t.disk_crit, 90);
    $("diskioWarn").value = td(t.diskio_warn, 80); $("diskioCrit").value = td(t.diskio_crit, 95);
    $("iopsWarn").value = td(t.iops_warn, 50000); $("iopsCrit").value = td(t.iops_crit, 100000);
    $("gpuWarn").value = td(t.gpu_warn, 80); $("gpuCrit").value = td(t.gpu_crit, 95);
    $("loadWarn").value = td(t.load_warn, 4.0); $("loadCrit").value = td(t.load_crit, 8.0);
    $("procWarn").value = td(t.proc_warn, 0.5);
    $("offlineSec").value = td(t.offline_after_sec, 60);

    // Reset to first tab
    switchNotifyTab("tab-feishu");

    $("settingsMask").classList.add("show");
  } catch (e) { toast(I18N.t("toast.read_config_failed") + e, "err"); }
}

// Tab switching for notification channels
function switchNotifyTab(tabId) {
  document.querySelectorAll("#notifyTabs .tab").forEach(btn => btn.classList.toggle("active", btn.dataset.tab === tabId));
  document.querySelectorAll("#settingsMask .tab-panel").forEach(p => p.classList.toggle("active", p.id === tabId));
}

function collectSettings() {
  const num = id => parseFloat($(id).value) || 0;
  return {
    alerts_enabled: $("alertsEnabled").checked,
    feishu: { enabled: $("feishuEnabled").checked, webhook: $("feishuWebhook").value.trim() },
    dingtalk: { enabled: $("dingEnabled").checked, webhook: $("dingWebhook").value.trim(), secret: $("dingSecret").value.trim() },
    custom_webhook: {
      enabled: $("customWebhookEnabled").checked,
      url: $("customWebhookURL").value.trim(),
      method: $("customWebhookMethod").value,
      content_type: $("customWebhookContentType").value.trim(),
      headers: $("customWebhookHeaders").value.trim(),
      body_template: $("customWebhookBodyTemplate").value.trim()
    },
    smtp: {
      smtp_enabled: $("smtpEnabled").checked,
      smtp_host: $("smtpHost").value.trim(),
      smtp_port: parseInt($("smtpPort").value) || 0,
      smtp_username: $("smtpUsername").value.trim(),
      smtp_password: $("smtpPassword").value,
      smtp_from_name: $("smtpFromName").value.trim(),
      smtp_use_tls: $("smtpTLS").checked
    },
    thresholds: {
      cpu_warn: num("cpuWarn"), cpu_crit: num("cpuCrit"),
      mem_warn: num("memWarn"), mem_crit: num("memCrit"),
      disk_warn: num("diskWarn"), disk_crit: num("diskCrit"),
      diskio_warn: num("diskioWarn"), diskio_crit: num("diskioCrit"),
      iops_warn: num("iopsWarn"), iops_crit: num("iopsCrit"),
      gpu_warn: num("gpuWarn"), gpu_crit: num("gpuCrit"),
      load_warn: num("loadWarn"), load_crit: num("loadCrit"),
      proc_warn: num("procWarn"),
      offline_after_sec: Math.round(num("offlineSec"))
    }
  };
}
async function saveSettings() {
  await withLoading("saveBtn", async () => {
    try {
      const r = await fetch(`${API}/config`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(collectSettings()) });
      if (r.ok) { toast(I18N.t("toast.config_saved"), "ok"); $("settingsMask").classList.remove("show"); } else { toast(I18N.t("toast.save_failed"), "err"); }
    } catch (e) { toast(I18N.t("toast.save_failed2") + e, "err"); }
  });
}
async function testSettings() {
  await withLoading("testBtn", async () => {
    try {
      const r = await fetch(`${API}/config/test`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(collectSettings()) });
      const j = await r.json();
      if (j.ok) toast(I18N.t("toast.test_sent"), "ok");
      else toast(I18N.t("toast.test_failed2") + (j.errors || []).join("; "), "err");
    } catch (e) { toast(I18N.t("toast.test_failed2") + e, "err"); }
  });
}

/* ---------- 安装 Agent ---------- */
let INSTALL = { server_url: "", token: "" };
let CUR_OS = "linux";
let RELAY_MODE = false;
let MULTI_SERVER_MODE = false;
let TOKEN_REVEALED = false; // Token 脱敏状态
function maskToken(t) {
  if (!t) return "";
  if (TOKEN_REVEALED) return t;
  if (t.length <= 8) return "••••••••";
  return t.slice(0, 4) + "••••••••" + t.slice(-4);
}
function updateTokenDisplay() {
  const el = $("installToken"); if (!el) return;
  el.value = maskToken(INSTALL.token || "");
  el.dataset.revealed = TOKEN_REVEALED ? "1" : "0";
}
async function openInstall() {
  try {
    INSTALL = await fetch(`${API}/install/info`).then(r => r.json());
    TOKEN_REVEALED = false;
    updateTokenDisplay();
    RELAY_MODE = false;
    MULTI_SERVER_MODE = false;
    const normalRadio = document.querySelector('input[name="installMode"][value="normal"]');
    if (normalRadio) normalRadio.checked = true;
    renderInstallCmd();
    $("installMask").classList.add("show");
  } catch (e) { toast(I18N.t("toast.read_install_failed") + e, "err"); }
}
function parseMultiServerList() {
  const text = ($("multiServerList") || {}).value || "";
  const lines = text.split("\n").map(l => l.trim()).filter(l => l);
  const servers = [];
  for (const line of lines) {
    const parts = line.split(/\s+/);
    const server = parts[0];
    const token = parts.slice(1).join(" ") || "";
    if (server) servers.push({ server, token });
  }
  return servers.length > 0 ? JSON.stringify(servers) : "";
}
function renderInstallCmd() {
  // Multi-server section visibility
  const msSection = $("multiServerSection");
  if (msSection) msSection.style.display = (MULTI_SERVER_MODE && !RELAY_MODE) ? "" : "none";
  // Relay mode: show gateway + internal commands, hide normal install
  if (RELAY_MODE) {
    $("normalInstallSection").style.display = "none";
    $("relaySection").style.display = "";
    renderRelayCmd();
    return;
  }
  $("normalInstallSection").style.display = "";
  $("relaySection").style.display = "none";
  const server = INSTALL.server_url || location.origin;
  const token = INSTALL.token || "";
  const cat = $("installCategory").value.trim();
  let q = "token=" + encodeURIComponent(token) + (cat ? "&category=" + encodeURIComponent(cat) : "");
  // Multi-server: append servers_json so the generated config.json uses a
  // servers array instead of a single server+token.
  let cmd, label, hint;
  if (MULTI_SERVER_MODE) {
    const sj = parseMultiServerList();
    if (sj) q += "&servers_json=" + encodeURIComponent(sj);
    label = I18N.t("install.multi_server");
    hint = I18N.t("install.multi_desc");
  }
  if (CUR_OS === "windows") {
    cmd = `irm "${server}/install.ps1?${q}" | iex`;
    label = I18N.t("install.powershell_cmd");
    hint = "普通 PowerShell 即可；安装到 %LOCALAPPDATA%\\AIOps-agent 并注册用户级开机自启。";
  } else if (CUR_OS === "macos") {
    cmd = `curl -fsSL "${server}/install.sh?${q}" | sh`;
    label = I18N.t("install.terminal_one_line");
    hint = I18N.t("install.linux_detail");
  } else {
    cmd = `curl -fsSL "${server}/install.sh?${q}" | sudo sh`;
    label = I18N.t("install.linux_cmd");
    hint = I18N.t("install.linux_desc");
  }
  $("installCmd").textContent = cmd;
  $("cmdLabel").textContent = label;
  $("cmdHint").textContent = hint;
  $("uninstallCmd").textContent = (CUR_OS === "windows")
    ? `irm "${server}/uninstall.ps1" | iex`
    : `curl -fsSL "${server}/uninstall.sh" | ${CUR_OS === "macos" ? "sh" : "sudo sh"}`;
}
function renderRelayCmd() {
  const server = INSTALL.server_url || location.origin;
  const token = INSTALL.token || "";
  const cat = $("installCategory").value.trim();
  let q = "token=" + encodeURIComponent(token) + (cat ? "&category=" + encodeURIComponent(cat) : "");
  const gwIP = $("relayGatewayIP").value.trim() || I18N.t("install.gateway_ip");
  const relay = `http://${gwIP}:8529`;
  let gatewayCmd, internalCmd;
  if (CUR_OS === "windows") {
    gatewayCmd = `irm "${server}/install-relay.ps1?${q}" | iex`;
    internalCmd = `irm "${relay}/install.ps1?${q}" | iex`;
  } else if (CUR_OS === "macos") {
    gatewayCmd = `curl -fsSL "${server}/install-relay.sh?${q}" | sh`;
    internalCmd = `curl -fsSL "${relay}/install.sh?${q}" | sh`;
  } else {
    gatewayCmd = `curl -fsSL "${server}/install-relay.sh?${q}" | sudo sh`;
    internalCmd = `curl -fsSL "${relay}/install.sh?${q}" | sudo sh`;
  }
  $("relayGatewayCmd").textContent = gatewayCmd;
  $("relayInternalCmd").textContent = internalCmd;
  $("uninstallCmd").textContent = (CUR_OS === "windows")
    ? `irm "${server}/uninstall.ps1" | iex`
    : `curl -fsSL "${server}/uninstall.sh" | ${CUR_OS === "macos" ? "sh" : "sudo sh"}`;
}
async function resetToken() {
  if (!confirm(I18N.t("install.reset_warning"))) return;
  try {
    const j = await fetch(`${API}/install/reset-token`, { method: "POST" }).then(r => r.json());
    INSTALL.token = j.token; TOKEN_REVEALED = false; updateTokenDisplay(); renderInstallCmd();
    toast(I18N.t("toast.token_reset"), "ok");
  } catch (e) { toast(I18N.t("toast.reset_failed2") + e, "err"); }
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
    const stText = !c.enabled ? I18N.t("ui.disabled_status") : (c.checked_at ? (c.ok ? I18N.t("ui.normal") : I18N.t("ui.abnormal")) : I18N.t("ui.pending"));
    const typeText = c.type === "http" ? "HTTP" : c.type === "tcp" ? "TCP" : c.type === "ping" ? "Ping" : I18N.t("ui.process");
    const builtin = c.builtin ? ' data-builtin="1"' : "";
    const histBtn = `<button class="mini-btn" data-cact="hist" title="${I18N.t('ui.history_chart')}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 3v18h18"/><path d="M7 13l3-3 3 2 5-6"/></svg></button>`;
    const actions = `<span class="ch-actions">${histBtn}${c.builtin ? "" : `
          <button class="mini-btn" data-cact="run" title="${I18N.t('ui.check_now')}">▶</button>
          <button class="mini-btn" data-cact="edit" title="${I18N.t('ui.edit')}">✎</button>
          <button class="mini-btn del" data-cact="del" title="${I18N.t('ui.delete')}">✕</button>`}</span>`;
    const builtinTag = c.builtin ? `<span class="type-badge" style="background:var(--ok-soft);color:var(--ok-txt)">${I18N.t("ui.builtin")}</span>` : "";

    // 详情字段：按监控类型给出各自贴合的字段，三类监控信息量对齐
    const stCls = st === "up" ? "ok" : st === "down" ? "crit" : "muted";
    const lat = c.checked_at ? Math.round(c.latency_ms) + " ms" : "—";
    const latCls = c.checked_at ? "" : "muted";
    const detail = [];
    if (c.type === "http") {
      detail.push(cdItem(I18N.t("form.check_url"), checkTargetDisplay(c), "muted"));
      detail.push(cdItem(I18N.t("form.run_status"), stText, stCls));
      const code = c.status_code || 0;
      detail.push(cdItem(I18N.t("form.status_code"), code ? String(code) : "—", code === 0 ? "muted" : code >= 400 ? "crit" : "ok"));
      detail.push(cdItem(I18N.t("form.response_latency"), lat, latCls));
      if (typeof c.cert_days === "number" && c.cert_days >= 0) {
        const d = c.cert_days;
        detail.push(cdItem(I18N.t("form.cert_remaining"), d + I18N.t("time.days"), d <= 7 ? "crit" : d <= 30 ? "warn" : "ok"));
      }
    } else if (c.type === "tcp") {
      const hp = splitHostPort(c.target);
      detail.push(cdItem(I18N.t("form.target"), hp.host || c.target, "muted"));
      detail.push(cdItem(I18N.t("form.port"), hp.port || "—", ""));
      detail.push(cdItem(I18N.t("form.connect_status"), stText, stCls));
      detail.push(cdItem(I18N.t("form.connect_latency"), lat, latCls));
    } else if (c.type === "ping") {
      detail.push(cdItem(I18N.t("form.check_url"), c.target, "muted"));
      detail.push(cdItem(I18N.t("form.run_status"), stText, stCls));
      const loss = (typeof c.loss_pct === "number" && c.loss_pct >= 0) ? c.loss_pct : null;
      detail.push(cdItem(I18N.t("form.loss_rate"), loss === null ? "—" : Math.round(loss) + "%",
        loss === null ? "muted" : loss === 0 ? "ok" : loss >= 100 ? "crit" : "warn"));
      const hasRtt = c.checked_at && c.latency_ms > 0;
      detail.push(cdItem(I18N.t("form.avg_latency"), hasRtt ? Math.round(c.latency_ms) + " ms" : "—", hasRtt ? "" : "muted"));
    } else if (c.type === "process") {
      const pr = splitProcessTarget(c);
      detail.push(cdItem(I18N.t("form.process_name2"), pr.proc, ""));
      detail.push(cdItem(I18N.t("form.target_host2"), pr.hostName, "muted"));
      detail.push(cdItem(I18N.t("form.run_status"), stText, stCls));
      detail.push(cdItem(I18N.t("form.check_duration"), lat, latCls));
    } else {
      detail.push(cdItem(I18N.t("form.check_url"), checkTargetDisplay(c), "muted"));
      detail.push(cdItem(I18N.t("form.run_status"), stText, stCls));
      detail.push(cdItem(I18N.t("form.latency"), lat, latCls));
    }
    detail.push(cdItem(I18N.t("form.check_interval"), I18N.t("section.every") + c.interval_sec + "s", "muted"));
    detail.push(cdItem(I18N.t("form.last_check"), c.checked_at ? ago(c.checked_at) : I18N.t("ui.not_checked"), "muted"));

    return `<div class="check-card st-${st}" data-id="${esc(c.id)}"${builtin}>
      <div class="check-row-top">
        <span class="st-dot ${st}"></span>
        <span class="ch-name" title="${esc(c.name)}">${esc(c.name)}</span>
        <span class="type-badge t-${esc(c.type)}">${typeText}</span>
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
// 主机 卡片 / 列表 视图切换
function setHostView(v) {
  HOST_VIEW = v === "list" ? "list" : "card";
  try { localStorage.setItem("aiops_host_view", HOST_VIEW); } catch (e) {}
  document.querySelectorAll("#hostViewToggle .vt-btn").forEach(b => b.classList.toggle("active", b.dataset.hview === HOST_VIEW));
  HOST_PAGE = 1;
  renderHosts(LAST_HOSTS);
}
async function loadChecks() {
  try { renderChecks(await fetch(`${API}/checks`).then(r => r.json())); } catch (e) { /* ignore */ }
}

let CHK_CHARTS = {};
let CHK_HIST = { id: "", name: "", type: "", range: 24 }; // range=小时数，默认 24h
// 自定义监控·历史曲线：复用交互式图表引擎，支持按时间范围筛选（与主机趋势图一致）
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
// 历史弹窗：时间范围切换 + 图表放大委托
safeAddEventListener("checkHistBody", "click", e => {
  const rb = e.target.closest(".chip-btn[data-crange]");
  if (rb) { CHK_HIST.range = parseInt(rb.dataset.crange); loadCheckHistory(); return; }
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
    $("ckTargetLabel").textContent = I18N.t("form.process_name");
    $("ckTarget").placeholder = I18N.t("form.hint_process");
    return;
  }
  $("ckHostField").style.display = "none";
  if (t === "http") {
    $("ckTargetLabel").textContent = I18N.t("form.url");
    $("ckTarget").placeholder = "https://example.com";
  } else if (t === "ping") {
    $("ckTargetLabel").textContent = I18N.t("form.host_addr");
    $("ckTarget").placeholder = I18N.t("form.hint_url");
  } else {
    $("ckTargetLabel").textContent = I18N.t("form.host_port");
    $("ckTarget").placeholder = "127.0.0.1:3306";
  }
}
function openCheckModal(check) {
  $("checkModalTitle").textContent = check ? I18N.t("ui.edit_check") : I18N.t("ui.add_check");
  $("ckId").value = check ? check.id : "";
  $("ckName").value = check ? check.name : "";
  $("ckType").value = check ? check.type : "http";
  // For process type, extract process name only (not "hostID/procName")
  if (check && check.type === "process") {
    const idx = check.target.indexOf("/");
    $("ckTarget").value = idx > 0 ? check.target.slice(idx + 1) : check.target;
  } else {
    $("ckTarget").value = check ? check.target : "";
  }
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
    if (!hostId) { toast(I18N.t("valid.select_host"), "err"); return; }
    if (!target) { toast(I18N.t("valid.fill_process"), "err"); return; }
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
  if (!body.name || !body.target) { toast(I18N.t("valid.fill_name_target"), "err"); return; }
  await withLoading("ckSaveBtn", async () => {
    try {
      const r = await fetch(`${API}/checks`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      if (r.ok) { toast(I18N.t("toast.saved"), "ok"); $("checkMask").classList.remove("show"); loadChecks(); }
      else { const j = await r.json(); toast(I18N.t("toast.save_failed2") + (j.error || ""), "err"); }
    } catch (e) { toast(I18N.t("toast.save_failed2") + e, "err"); }
  });
}
async function delCheck(id) {
  if (!confirm(I18N.t("valid.confirm_delete_check"))) return;
  try {
    const r = await fetch(`${API}/checks/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (r.ok) { toast(I18N.t("toast.deleted"), "ok"); loadChecks(); } else { toast(I18N.t("toast.delete_failed"), "err"); }
  } catch (e) { toast(I18N.t("toast.deleted") + ": " + e, "err"); }
}

/* ---------- 账户 / 个人信息 ---------- */
let CUR_ROLE = "";
const roleLabel = r => ({ admin: I18N.t("ui.admin"), operator: I18N.t("ui.operator"), viewer: I18N.t("ui.readonly") }[r] || r || "");
const canWrite = () => CUR_ROLE === "operator" || CUR_ROLE === "admin";
const isAdmin = () => CUR_ROLE === "admin";
function setUser(me) {
  const name = me.display_name || me.username || I18N.t("ui.user");
  const initial = (name[0] || "A");
  const roleLabels = { admin: "管理员", operator: "操作员", viewer: "查看者" };
  // 顶栏按钮
  var el = $("userName"); if (el) el.textContent = name;
  el = $("userAvatar"); if (el) el.textContent = initial;
  // 下拉菜单大图
  el = $("userNameLg"); if (el) el.textContent = name;
  el = $("userAvatarLg"); if (el) el.textContent = initial;
  el = $("userRoleLg"); if (el) el.textContent = roleLabels[me.role] || me.role || "—";
  if (me.role) {
    CUR_ROLE = me.role;
    document.body.dataset.role = me.role;
  }
}
// fetchWithTimeout wraps fetch with an AbortController timeout so mobile
// browsers on slow/unstable networks don't hang indefinitely. Returns the
// Response or throws an AbortError / network error.
function fetchWithTimeout(url, opts, timeoutMs) {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), timeoutMs || 15000);
  return fetch(url, Object.assign({}, opts, { signal: ctrl.signal })).finally(() => clearTimeout(timer));
}
async function initAuth() {
  try {
    const r = await fetchWithTimeout(`${API}/me`, {}, 10000);
    if (r.ok) {
      const me = await r.json();
      setUser(me);
      $("loginView").classList.remove("show");
      startApp();
      // v5.4.0: force password change if admin reset was used
      if (me.must_change_password) {
        // 强制进入「安全初始化」弹窗：需修改用户名 + 密码后方可进入控制台
        setTimeout(() => openInitSetup(), 300);
      }
    }
    else { $("loginView").classList.add("show"); }
  } catch (e) {
    // Network error on initial auth check — show login with a friendly hint
    // instead of a raw "Failed to fetch" that confuses mobile users.
    $("loginView").classList.add("show");
    const loginErrEl = $("loginErr");
    if (loginErrEl) loginErrEl.textContent = I18N.t("toast.network_check_failed");
  }
}
/* ---------- 消息中心（顶栏铃铛 + 未读徽标 + 下拉面板） ---------- */
let MSG_POLL = null;
function msgEsc(s){return String(s==null?"":s).replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));}
function initMsgCenter() {
  const panel = $("notifPanel"), wrap = $("notifWrap");
  if (!$("notifBtn") || !panel) return;
  safeAddEventListener("notifBtn", "click", (e) => {
    e.stopPropagation();
    const open = panel.classList.toggle("show");
    if (open) loadMessages();
  });
  safeAddEventListener("notifReadAll", "click", async (e) => {
    e.stopPropagation();
    try { await fetch(`${API}/messages/read-all`, { method: "POST" }); } catch (_) {}
    loadMessages();
  });
  document.addEventListener("click", (e) => { if (wrap && !wrap.contains(e.target)) panel.classList.remove("show"); });
  loadMessages();
  if (MSG_POLL) clearInterval(MSG_POLL);
  MSG_POLL = setInterval(loadMessages, 20000);
}
async function loadMessages() {
  try {
    const data = await fetch(`${API}/messages?limit=50`).then(r => r.json());
    const msgs = data.messages || [];
    const unread = data.unread || 0;
    const badge = $("notifBadge");
    if (badge) {
      if (unread > 0) { badge.textContent = unread > 99 ? "99+" : unread; badge.style.display = ""; }
      else badge.style.display = "none";
    }
    renderMessages(msgs);
  } catch (_) {}
}
function renderMessages(msgs) {
  const list = $("notifList"), empty = $("notifEmpty");
  if (!list) return;
  if (empty) empty.style.display = msgs.length ? "none" : "";
  const pad = n => String(n).padStart(2, "0");
  list.innerHTML = msgs.map(m => {
    const t = new Date((m.ts || 0) * 1000);
    const ts = `${t.getMonth()+1}-${pad(t.getDate())} ${pad(t.getHours())}:${pad(t.getMinutes())}`;
    return `<div class="notif-item ${m.read ? "" : "unread"}" data-id="${m.id}" data-view="${msgEsc(m.view || "")}">
      <span class="notif-dot ${msgEsc(m.level || "info")}"></span>
      <div class="notif-body">
        <div class="notif-title">${msgEsc(m.title || "")}</div>
        ${m.body ? `<div class="notif-sub">${msgEsc(m.body)}</div>` : ""}
        <div class="notif-time">${ts}</div>
      </div></div>`;
  }).join("");
  list.querySelectorAll(".notif-item").forEach(el => {
    el.addEventListener("click", async () => {
      const id = parseInt(el.dataset.id, 10);
      const view = el.dataset.view;
      try { await fetch(`${API}/messages/read`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ ids: [id] }) }); } catch (_) {}
      const p = $("notifPanel"); if (p) p.classList.remove("show");
      if (view && typeof switchView === "function") switchView(view);
      loadMessages();
    });
  });
}
function startApp() {
  if (APP_STARTED) return;
  APP_STARTED = true;
  initTheme();
  initNotifications();
  initMsgCenter();
  showSkeleton();
  refresh(); loadChecks();
  // P1-2: 差异化轮询频率 — 按当前视图 + 标签页可见性调整刷新间隔
  const POLL_BASE = 3000;
  let pollTimer = null;
  function schedulePoll() {
    if (pollTimer) clearTimeout(pollTimer);
    const view = document.querySelector(".view.active")?.id.replace("view-", "") || "overview";
    const intervals = { overview: 3000, hosts: 5000, checks: 10000, alerts: 3000, automation: 15000, forward: 15000, log: 10000 };
    let interval = intervals[view] || POLL_BASE;
    // 后台标签页降频至 15s，减少不必要的网络请求和 DOM 渲染
    if (document.visibilityState === "hidden") interval = Math.max(interval, 15000);
    pollTimer = setTimeout(() => { refresh(); loadChecks(); if (document.querySelector("#view-forward.active")) loadForwards(); schedulePoll(); }, interval);
  }
  schedulePoll();
  // 视图切换时立即调整轮询频率
  document.querySelectorAll(".nav-item").forEach(n => n.addEventListener("click", () => setTimeout(schedulePoll, 100)));
  // 标签页可见性变化时重排轮询
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") { refresh(true); schedulePoll(); }
  });
  // P3-1: 初始化 WebSocket 推送（带降级到轮询）
  initPushWS();
}
// 首次登录 · 安全初始化：强制修改用户名 + 密码的专用弹窗（替代直接打开个人信息页）。
// 弹窗带 data-forced，无法通过 ESC / 点遮罩 / ✕ 关闭；完成后会话重签并刷新进入。
async function openInitSetup() {
  try {
    const me = await fetch(`${API}/me`).then(r => r.json()).catch(() => ({}));
    const u = $("initUser"); if (u) u.value = me.username || "";
    const p = $("initPass"); if (p) p.value = "";
    const p2 = $("initPass2"); if (p2) p2.value = "";
    const err = $("initErr"); if (err) { err.textContent = ""; err.style.display = "none"; }
    const mask = $("initSetupMask"); if (mask) mask.classList.add("show");
    if (u) setTimeout(() => u.focus(), 60);
  } catch (e) { toast(I18N.t("toast.read_failed2") + e, "err"); }
}
async function submitInitSetup() {
  const err = $("initErr");
  const showErr = (m) => { if (err) { err.textContent = m; err.style.display = "block"; } else toast(m, "err"); };
  if (err) { err.textContent = ""; err.style.display = "none"; }
  const uname = ($("initUser").value || "").trim();
  const pw = $("initPass").value || "";
  const pw2 = $("initPass2").value || "";
  if (!uname) { showErr(I18N.t("init.err_username", "请输入登录用户名")); return; }
  if (!pw) { showErr(I18N.t("init.err_password", "请输入新密码")); return; }
  if (pw !== pw2) { showErr(I18N.t("init.err_mismatch", "两次输入的密码不一致")); return; }
  if (pw.length < 8) { showErr(I18N.t("auth.password_policy", "密码需至少 8 位，含大小写字母、数字和特殊字符")); return; }
  await withLoading($("initSubmitBtn"), async () => {
    try {
      const r = await fetch(`${API}/account/init`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: uname, password: pw })
      });
      const j = await r.json().catch(() => ({}));
      if (r.ok) {
        const mask = $("initSetupMask"); if (mask) mask.classList.remove("show");
        setUser({ username: j.username || uname });
        toast(I18N.t("init.done", "安全初始化完成，正在进入…"), "ok");
        // 用户名/密码已更新、会话已重签，刷新以干净状态进入控制台。
        setTimeout(() => location.reload(), 500);
      } else {
        showErr(j.error || I18N.t("toast.save_failed"));
      }
    } catch (e) { showErr(I18N.t("toast.save_failed2") + e); }
  });
}
async function openProfile(tab) {
  try {
    const me = await fetch(`${API}/me`).then(r => r.json());
    $("pfUsername").value = me.username || "";
    $("pfDisplay").value = me.display_name || "";
    $("pfEmail").value = me.email || "";
    $("pfOld").value = ""; $("pfNew").value = "";
    setUser(me); // 用最新 /me 刷新顶栏与 CUR_ROLE（角色可能已变更）
    // 清空各 Tab 内联错误
    ["pfProfileErr", "pfPwdErr", "pfTermPwdErr"].forEach(id => { const e = $(id); if (e) { e.textContent = ""; e.style.display = "none"; } });
    renderMfaState(!!me.mfa_enabled);
    // v5.3.0: 加载终端密码状态
    loadTermPwdStatus();
    $("profileMask").classList.add("show");
    // 切换到底层请求指定的 Tab（默认「个人信息」）；非管理员无法进入用户管理
    const target = (tab === "users" && !isAdmin()) ? "info" : (tab || "info");
    switchProfileTab(target);
  } catch (e) { toast(I18N.t("toast.read_failed2") + e, "err"); }
}
let PROFILE_TAB = "info";
let PROFILE_USERS_LOADED = false;
async function switchProfileTab(tab) {
  PROFILE_TAB = tab;
  document.querySelectorAll("#profileTabs .tab").forEach(b => b.classList.toggle("active", b.dataset.ptab === tab));
  document.querySelectorAll("#profileMask .tab-panel").forEach(p => p.classList.toggle("active", p.id === "tab-profile-" + tab));
  // 用户管理 Tab：首次进入时按需独立加载（保持其它 Tab 状态不重渲染）
  if (tab === "users" && isAdmin() && !PROFILE_USERS_LOADED) {
    PROFILE_USERS_LOADED = true;
    await loadUsers();
  }
}
async function saveProfile() {
  const errEl = $("pfProfileErr");
  if (errEl) { errEl.textContent = ""; errEl.style.display = "none"; }
  try {
    const uname = $("pfUsername").value.trim();
    const r = await fetch(`${API}/profile`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: uname, display_name: $("pfDisplay").value.trim(), email: $("pfEmail").value.trim() })
    });
    const j = await r.json().catch(() => ({}));
    if (r.ok) { toast(I18N.t("toast.profile_saved"), "ok"); setUser({ display_name: $("pfDisplay").value.trim(), username: j.username || uname }); }
    else if (errEl) { errEl.textContent = j.error || I18N.t("toast.save_failed"); errEl.style.display = "block"; }
    else toast(j.error || I18N.t("toast.save_failed"), "err");
  } catch (e) { toast(I18N.t("toast.save_failed2") + e, "err"); }
}
async function changePassword() {
  const errEl = $("pfPwdErr");
  if (errEl) { errEl.textContent = ""; errEl.style.display = "none"; }
  if (!$("pfOld").value || !$("pfNew").value) {
    if (errEl) { errEl.textContent = I18N.t("valid.fill_passwords"); errEl.style.display = "block"; }
    else toast(I18N.t("valid.fill_passwords"), "err");
    return;
  }
  if (!pwPolicyOK($("pfNew").value)) {
    if (errEl) { errEl.textContent = I18N.t("auth.password_policy"); errEl.style.display = "block"; }
    else toast(I18N.t("auth.password_policy"), "err");
    return;
  }
  try {
    const r = await fetch(`${API}/password`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ old: $("pfOld").value, new: $("pfNew").value })
    });
    const j = await r.json();
    if (r.ok) { toast(I18N.t("toast.password_changed"), "ok"); $("pfOld").value = ""; $("pfNew").value = ""; }
    else if (errEl) { errEl.textContent = j.error || I18N.t("toast.update_failed"); errEl.style.display = "block"; }
    else toast(j.error || I18N.t("toast.update_failed"), "err");
  } catch (e) { toast(I18N.t("toast.update_failed2") + e, "err"); }
}

/* ===================== v5.3.0: 终端密码管理（个人信息页） ===================== */
let TERM_PWD_CHANGE_SHOWING = false;

async function loadTermPwdStatus() {
  try {
    const r = await fetch("/api/user/terminal-password/status", { credentials: "include" });
    const j = await r.json().catch(() => ({}));
    const valEl = $("pfTermPwdStatusVal");
    if (valEl) {
      if (j.has_password) {
        valEl.textContent = I18N.t("term_auth.password_set");
        valEl.className = "term-pwd-status-val set";
      } else {
        valEl.textContent = I18N.t("term_auth.no_password_set");
        valEl.className = "term-pwd-status-val unset";
      }
    }
  } catch (e) { /* 静默失败 */ }
}

function toggleTermPwdChange() {
  TERM_PWD_CHANGE_SHOWING = !TERM_PWD_CHANGE_SHOWING;
  const authField = $("pfTermPwdAuthField");
  const newField = $("pfTermPwdNewField");
  const errEl = $("pfTermPwdErr");
  const btn = $("pfTermPwdBtn");

  if (TERM_PWD_CHANGE_SHOWING) {
    // 显示修改表单
    $("pfTermPwdAuth").value = "";
    $("pfTermPwdNew").value = "";
    if (errEl) { errEl.textContent = ""; errEl.style.display = "none"; }
    if (authField) authField.style.display = "block";
    if (newField) newField.style.display = "block";
    if (btn) btn.textContent = I18N.t("ui.cancel");
    // 根据 MFA 状态调整验证字段标签
    const authLabel = $("pfTermPwdAuthLabel");
    if (authLabel) {
      authLabel.textContent = MFA_ENABLED ? I18N.t("term_auth.mfa_code") : I18N.t("term_auth.current_password");
    }
    $("pfTermPwdAuth").placeholder = MFA_ENABLED ? I18N.t("mfa.code_6") : "";
    $("pfTermPwdAuth").maxLength = MFA_ENABLED ? 6 : 524288;
  } else {
    // 隐藏修改表单
    if (authField) authField.style.display = "none";
    if (newField) newField.style.display = "none";
    if (errEl) { errEl.textContent = ""; errEl.style.display = "none"; }
    if (btn) btn.textContent = I18N.t("term_auth.change_password_btn");
  }
}

async function submitTermPwdChange() {
  if (!TERM_PWD_CHANGE_SHOWING) {
    toggleTermPwdChange();
    return;
  }
  const code = $("pfTermPwdAuth").value.trim();
  const newPwd = $("pfTermPwdNew").value.trim();
  const errEl = $("pfTermPwdErr");

  if (!code || !newPwd) {
    if (errEl) { errEl.textContent = I18N.t("term_auth.fill_verify_password"); errEl.style.display = "block"; }
    return;
  }

  try {
    const r = await fetch("/api/user/terminal-password/set", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password: newPwd, code: code })
    });
    const j = await r.json().catch(() => ({}));

    if (r.ok) {
      toast(I18N.t("term_auth.changed_ok"), "ok");
      toggleTermPwdChange(); // 收起表单
      loadTermPwdStatus();   // 刷新状态
    } else {
      if (j.mfa_required) {
        // 修改时需要 MFA，但未提供
        if (errEl) { errEl.textContent = I18N.t("term_auth.enter_mfa_code"); errEl.style.display = "block"; }
        return;
      }
      if (errEl) { errEl.textContent = j.error || I18N.t("toast.update_failed"); errEl.style.display = "block"; }
    }
  } catch (e) {
    if (errEl) { errEl.textContent = I18N.t("toast.network_error"); errEl.style.display = "block"; }
  }
}

/* ===================== 两步验证（TOTP / Google Authenticator） ===================== */
let MFA_ENABLED = false;
function renderMfaState(enabled) {
  MFA_ENABLED = enabled;
  const st = $("mfaState"), chk = $("mfaToggleChk");
  if (st) { st.textContent = enabled ? I18N.t("toast.enabled") : I18N.t("toast.disabled"); st.className = "mfa-state " + (enabled ? "on" : "off"); }
  if (chk) { chk.checked = enabled; }
}
async function openMfaSetup(forced) {
  const body = $("mfaBody");
  $("mfaTitle").textContent = forced ? I18N.t("ui.mfa_required") : I18N.t("ui.enable_mfa");
  body.innerHTML = `<div class="empty-line">正在生成密钥…</div>`;
  $("mfaMask").classList.add("show");
  let data;
  try { data = await fetch(`${API}/mfa/setup`, { method: "POST" }).then(r => r.json()); }
  catch (e) { body.innerHTML = `<div class="empty-line">生成失败：${esc(e)}</div>`; return; }
  const secret = data.secret || "", qrURI = data.qr_datauri || "";
  const grp = secret.replace(/(.{4})/g, "$1 ").trim();
  body.innerHTML = `
    ${forced ? `<div class="mfa-desc" style="margin-bottom:10px;color:var(--warn-txt,#f2c078)">管理员已启用全局两步验证策略，请完成绑定后登录。</div>` : ""}
    <ol class="mfa-steps">
      <li>打开 <b>Google Authenticator</b>（或任意 TOTP 应用），扫描二维码；无法扫码时可手动输入下方密钥。</li>
      <li>输入应用当前显示的 6 位动态口令，点「确认启用」。</li>
    </ol>
    <div class="mfa-qr" id="mfaQr"></div>
    <div class="mfa-secret">${I18N.t("mfa.secret_label")}　<code class="mono" id="mfaSecret">${esc(grp)}</code><button class="btn ghost sm" id="mfaCopy" type="button">${I18N.t("mfa.copy_btn")}</button></div>
    <div class="field"><label>${I18N.t("form.totp_code")}</label><input type="text" id="mfaCode" inputmode="numeric" maxlength="6" placeholder="${I18N.t('mfa.code_6')}" autocomplete="one-time-code"></div>
    <div class="login-err" id="mfaErr"></div>
    <div class="mfa-foot"><button class="btn primary" id="mfaConfirm" type="button">${I18N.t("mfa.confirm_enable")}</button></div>`;
  if (qrURI) $("mfaQr").innerHTML = `<img src="${esc(qrURI)}" alt="MFA QR Code" class="qr-img">`;
  else $("mfaQr").innerHTML = `<div class="mfa-desc">二维码不可用，请在应用中手动输入上方密钥。</div>`;
  $("mfaCopy").onclick = () => { try { navigator.clipboard.writeText(secret); toast(I18N.t("toast.secret_copied"), "ok"); } catch (_) { } };
  $("mfaConfirm").onclick = async () => {
    const errEl = $("mfaErr"); errEl.textContent = "";
    const code = $("mfaCode").value.trim();
    if (code.length !== 6) { errEl.textContent = I18N.t("valid.enter_totp"); return; }
    const r = await fetch(`${API}/mfa/enable`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ secret, code }) });
    const j = await r.json().catch(() => ({}));
    if (r.ok) {
      toast(I18N.t("toast.mfa_enabled"), "ok");
      $("mfaMask").classList.remove("show");
      if (forced) {
        // Global MFA enforcement: complete login after enrollment.
        setUser(await fetch(`${API}/me`).then(x => x.json()));
        const lv = $("loginView"); if (lv) lv.classList.remove("show");
        startApp();
      } else { renderMfaState(true); }
    }
    else errEl.textContent = j.error || I18N.t("toast.enable_failed");
  };
  setTimeout(() => { const el = $("mfaCode"); if (el) el.focus(); }, 60);
}
function openMfaDisable() {
  const body = $("mfaBody");
  $("mfaTitle").textContent = I18N.t("ui.disable_mfa");
  body.innerHTML = `
    <div class="mfa-desc" style="margin-bottom:14px">关闭后，登录将不再需要动态口令。请选择验证方式：</div>
    <div class="field"><label>${I18N.t("form.password")}</label><input type="password" id="mfaPass" autocomplete="current-password"></div>
    <div class="login-err" id="mfaErr"></div>
    <div class="mfa-foot">
      <button class="btn danger" id="mfaConfirmOff" type="button">${I18N.t("mfa.disable_pwd")}</button>
      <button class="btn" id="mfaEmailUnbind" type="button">${I18N.t("mfa.email_unbind_btn")}</button>
    </div>`;
  $("mfaMask").classList.add("show");
  $("mfaConfirmOff").onclick = async () => {
    const errEl = $("mfaErr"); errEl.textContent = "";
    const r = await fetch(`${API}/mfa/disable`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ password: $("mfaPass").value }) });
    const j = await r.json().catch(() => ({}));
    if (r.ok) { toast(I18N.t("toast.mfa_disabled"), "ok"); $("mfaMask").classList.remove("show"); renderMfaState(false); }
    else errEl.textContent = j.error || I18N.t("toast.disable_failed");
  };
  $("mfaEmailUnbind").onclick = () => openMfaEmailUnbind();
  setTimeout(() => { const el = $("mfaPass"); if (el) el.focus(); }, 60);
}

/* ---------- 通过邮箱验证码解除 MFA ---------- */
function openMfaEmailUnbind() {
  const body = $("mfaBody");
  $("mfaTitle").textContent = I18N.t("ui.unbind_mfa_email");
  body.innerHTML = `
    <div class="mfa-desc" style="margin-bottom:14px">系统将向已绑定邮箱发送 6 位验证码，验证通过后关闭两步验证。</div>
    <div class="login-err" id="mfaErr"></div>
    <div class="mfa-foot">
      <button class="btn primary" id="mfaSendCode" type="button">${I18N.t("mfa.send_code_btn")}</button>
      <span style="flex:1"></span>
    </div>
    <div class="field" id="mfaCodeRow" style="display:none">
      <label>${I18N.t("form.email_code")}</label>
      <input type="text" id="mfaEmailCode" inputmode="numeric" maxlength="6" placeholder="${I18N.t('mfa.code_6_v2')}" autocomplete="one-time-code">
    </div>
    <div class="mfa-foot" id="mfaVerifyRow" style="display:none">
      <button class="btn danger" id="mfaConfirmEmailUnbind" type="button">${I18N.t("mfa.confirm_unbind")}</button>
    </div>`;
  $("mfaMask").classList.add("show");
  $("mfaSendCode").onclick = async () => {
    const errEl = $("mfaErr"); errEl.textContent = "";
    const r = await fetch(`${API}/mfa/unbind-via-email`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ action: "send_code" }) });
    const j = await r.json().catch(() => ({}));
    if (r.ok) {
      toast(I18N.t("toast.code_sent"), "ok");
      $("mfaSendCode").textContent = I18N.t("ui.resend");
      $("mfaSendCode").disabled = true;
      setTimeout(() => { const b = $("mfaSendCode"); if (b) { b.disabled = false; } }, 60000);
      $("mfaCodeRow").style.display = "";
      $("mfaVerifyRow").style.display = "";
      setTimeout(() => { const el = $("mfaEmailCode"); if (el) el.focus(); }, 60);
    } else {
      errEl.textContent = j.error || I18N.t("toast.send_failed");
    }
  };
  $("mfaConfirmEmailUnbind").onclick = async () => {
    const errEl = $("mfaErr"); errEl.textContent = "";
    const code = $("mfaEmailCode").value.trim();
    if (code.length !== 6) { errEl.textContent = I18N.t("valid.enter_code"); return; }
    const r = await fetch(`${API}/mfa/unbind-via-email`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ action: "verify", code }) });
    const j = await r.json().catch(() => ({}));
    if (r.ok) { toast(I18N.t("toast.mfa_unbind_email"), "ok"); $("mfaMask").classList.remove("show"); renderMfaState(false); }
    else errEl.textContent = j.error || I18N.t("toast.unbind_failed");
  };
}

/* ---------- 用户管理（管理员）---------- */
async function openUsers() {
  // 用户管理已并入「个人信息」四 Tab 布局中的「用户管理」分页
  openProfile("users");
}
async function loadUsers() {
  // Fetch global MFA policy status
  try {
    const gm = await fetch(`${API}/mfa/global`).then(r => r.json());
    const chk = $("globalMfaChk");
    if (chk) { chk.checked = !!gm.mfa_required; chk.disabled = false; }
  } catch (_) { /* non-admin or error — switch stays disabled */ }
  const list = $("usersList");
  list.innerHTML = `<div class="empty-line">加载中…</div>`;
  let users;
  try { users = await fetch(`${API}/users`).then(r => r.json()); }
  catch (e) { list.innerHTML = `<div class="empty-line">加载失败: ${esc(e)}</div>`; return; }
  if (!Array.isArray(users) || !users.length) { list.innerHTML = `<div class="empty-line">${I18N.t("empty.no_users")}</div>`; return; }
  list.innerHTML = users.map(u => `
    <div class="user-row" data-name="${esc(u.username)}">
      <div class="user-info">
        <div class="user-main"><span class="user-name">${esc(u.username)}</span>
          <span class="role-badge role-${esc(u.role)}">${roleLabel(u.role)}</span>
          ${u.mfa_enabled ? `<span class="user-mfa" title="${I18N.t('mfa.enabled_badge')}">${I18N.t('mfa.enabled_badge')}</span>` : ""}</div>
        <div class="user-sub">${esc(u.display_name || "—")}${u.email ? " · " + esc(u.email) : ""}</div>
      </div>
      <div class="user-acts">
        <button class="btn ghost sm" data-act="edit">${I18N.t("ui.edit")}</button>
        <button class="btn ghost sm" data-act="pwd">${I18N.t("ui.reset_password")}</button>
        ${u.mfa_enabled ? `<button class="btn ghost sm" data-act="mfa">${I18N.t("ui.unbind_mfa")}</button>` : ""}
        <button class="btn ghost sm ubtn-del" data-act="del">${I18N.t("ui.delete")}</button>
      </div>
    </div>`).join("");
}
function openUserEdit(user) {
  const isNew = !user;
  $("userEditTitle").textContent = isNew ? I18N.t("ui.new_user") : I18N.t("ui.edit_user") + user.username;
  const roleOpts = ["admin", "operator", "viewer"].map(r => `<option value="${r}" ${user && user.role === r ? "selected" : ""}>${roleLabel(r)}</option>`).join("");
  $("userEditBody").innerHTML = `
    ${isNew ? `<div class="field"><label>${I18N.t("form.username")}</label><input type="text" id="ueName" placeholder="${I18N.t('form.username_format')}"></div>
    <div class="field"><label>${I18N.t("form.initial_password")}</label><input type="password" id="uePass"></div>` : ""}
    <div class="field"><label>${I18N.t("form.display_name")}</label><input type="text" id="ueDisplay" value="${user ? esc(user.display_name || "") : ""}" placeholder="${I18N.t('form.hint_display_name')}"></div>
    <div class="field"><label>${I18N.t("form.email_optional")}</label><input type="text" id="ueEmail" value="${user ? esc(user.email || "") : ""}" placeholder="name@example.com"></div>
    <div class="field"><label>${I18N.t("form.role")}</label><div class="select-wrap"><select id="ueRole">${roleOpts}</select></div></div>
    <div class="login-err" id="ueErr"></div>
    <div class="mfa-foot"><button class="btn primary" id="ueSave" type="button">${isNew ? I18N.t("ui.create_user") : I18N.t("ui.save")}</button></div>`;
  $("userEditMask").classList.add("show");
  $("ueSave").onclick = async () => {
    const errEl = $("ueErr"); errEl.textContent = "";
    const body = { display_name: $("ueDisplay").value.trim(), email: $("ueEmail").value.trim(), role: $("ueRole").value };
    let r;
    if (isNew) {
      body.username = $("ueName").value.trim();
      body.password = $("uePass").value;
      r = await fetch(`${API}/users`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    } else {
      r = await fetch(`${API}/users/${encodeURIComponent(user.username)}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    }
    const j = await r.json().catch(() => ({}));
    if (r.ok) { toast(isNew ? I18N.t("toast.user_created") : I18N.t("toast.saved"), "ok"); $("userEditMask").classList.remove("show"); loadUsers(); }
    else errEl.textContent = j.error || I18N.t("toast.operation_failed");
  };
}
async function usersAction(name, act) {
  if (act === "del") {
    // 两步确认：防止误删敏感操作
    if (!confirm(`⚠ 确定删除用户「${name}」？\n\n该操作不可撤销，该用户的所有会话将立即失效。\n如需继续，请点击「确定」。`)) return;
    const r = await fetch(`${API}/users/${encodeURIComponent(name)}`, { method: "DELETE" });
    const j = await r.json().catch(() => ({}));
    if (r.ok) { toast(I18N.t("toast.user_deleted"), "ok"); loadUsers(); } else toast(j.error || I18N.t("toast.delete_failed"), "err");
  } else if (act === "pwd") {
    const pass = prompt(`为「${name}」设置新密码（至少 8 位）：`);
    if (pass == null) return;
    if (!pwPolicyOK(pass.trim())) { toast(I18N.t("auth.password_policy"), "err"); return; }
    const r = await fetch(`${API}/users/${encodeURIComponent(name)}/reset-password`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ password: pass }) });
    const j = await r.json().catch(() => ({}));
    if (r.ok) toast(I18N.t("toast.password_reset"), "ok"); else toast(j.error || I18N.t("toast.reset_failed"), "err");
  } else if (act === "mfa") {
    if (!confirm(`确定解除「${name}」的两步验证绑定？`)) return;
    const r = await fetch(`${API}/users/${encodeURIComponent(name)}/reset-mfa`, { method: "POST" });
    const j = await r.json().catch(() => ({}));
    if (r.ok) { toast(I18N.t("toast.mfa_unbound"), "ok"); loadUsers(); } else toast(j.error || I18N.t("toast.operation_failed"), "err");
  }
}

/* ---------- 账户找回：用户名 / 密码 ---------- */
// New dual-verification flow (email code + optional MFA TOTP)
function openRecoverUser() { showRecoverFlow('recover_username'); }
function openRecoverPass() { showRecoverFlow('recover_password'); }

function showRecoverFlow(purpose) {
  const body = $("recoverBody");
  $("recoverTitle").textContent = I18N.t("recover.title");
  const label = purpose === 'recover_username' ? I18N.t("login.forgot_user") : I18N.t("login.forgot_pass");
  body.innerHTML = `
    <div class="mfa-desc" style="margin-bottom:14px">${I18N.t("recover.enter_email_desc")}</div>
    <div class="field"><label>${I18N.t("form.email")}</label><input type="text" id="rcEmail" placeholder="name@example.com" autocomplete="email"></div>
    <div class="login-err" id="rcErr"></div>
    <div class="mfa-foot"><button class="btn primary" id="rcAction" type="button">${I18N.t("mfa.send_code_btn")}</button></div>`;
  $("recoverMask").classList.add("show");

  $("rcAction").onclick = async () => {
    const errEl = $("rcErr"); errEl.textContent = "";
    const email = $("rcEmail").value.trim();
    if (!email) { errEl.textContent = I18N.t("valid.enter_email"); return; }
    try {
      const r = await fetch(`${API}/account/recover-send-code`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, purpose })
      });
      const j = await r.json().catch(() => ({}));
      if (r.ok) {
        toast(j.message || I18N.t("toast.code_sent"), "ok");
        showRecoverStep2(purpose, email);
      } else {
        errEl.textContent = j.error || I18N.t("toast.send_failed");
      }
    } catch (e) { errEl.textContent = I18N.t("toast.send_failed2") + e; }
  };
  setTimeout(() => { const el = $("rcEmail"); if (el) el.focus(); }, 60);
}

function showRecoverStep2(purpose, email) {
  const body = $("recoverBody");
  body.innerHTML = `
    <div class="mfa-desc" style="margin-bottom:14px">${I18N.t("recover.enter_code_desc")}</div>
    <div class="field" style="margin-bottom:8px"><label style="font-size:11px;color:var(--muted2)">${I18N.t("form.email")}：${esc(email)}</label></div>
    <div class="field"><label>${I18N.t("form.email_code")}</label><input type="text" id="rcCode" inputmode="numeric" maxlength="6" placeholder="${I18N.t('mfa.code_6')}" autocomplete="one-time-code"></div>
    <div class="login-err" id="rcErr"></div>
    <div class="mfa-foot" style="justify-content:space-between">
      <button class="btn" id="rcResend" type="button">${I18N.t("recover.resend_code")}</button>
      <button class="btn primary" id="rcAction" type="button">${I18N.t("recover.verify_code_btn")}</button>
    </div>`;

  $("rcResend").onclick = () => showRecoverFlow(purpose);
  $("rcAction").onclick = async () => {
    const errEl = $("rcErr"); errEl.textContent = "";
    const code = $("rcCode").value.trim();
    if (code.length !== 6) { errEl.textContent = I18N.t("valid.enter_code"); return; }
    try {
      const r = await fetch(`${API}/account/recover-verify`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, code, purpose })
      });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) { errEl.textContent = j.error || I18N.t("toast.verify_failed"); return; }
      if (j.mfa_required) {
        showRecoverStepMFA(purpose, email, code);
      } else {
        showRecoverResult(purpose, j);
      }
    } catch (e) { errEl.textContent = I18N.t("toast.send_failed2") + e; }
  };
  setTimeout(() => { const el = $("rcCode"); if (el) el.focus(); }, 60);
}

function showRecoverStepMFA(purpose, email, code) {
  const body = $("recoverBody");
  body.innerHTML = `
    <div class="mfa-desc" style="margin-bottom:14px">${I18N.t("recover.enter_totp_desc")}</div>
    <div class="field"><label>${I18N.t("recover.totp_code")}</label><input type="text" id="rcTOTP" inputmode="numeric" maxlength="6" placeholder="${I18N.t('recover.totp_placeholder')}" autocomplete="one-time-code"></div>
    <div class="login-err" id="rcErr"></div>
    <div class="mfa-foot" style="justify-content:space-between">
      <button class="btn" id="rcBack" type="button">${I18N.t("ui.back")}</button>
      <button class="btn primary" id="rcAction" type="button">${I18N.t("recover.verify_totp_btn")}</button>
    </div>`;

  $("rcBack").onclick = () => showRecoverStep2(purpose, email);
  $("rcAction").onclick = async () => {
    const errEl = $("rcErr"); errEl.textContent = "";
    const totp = $("rcTOTP").value.trim();
    if (totp.length !== 6) { errEl.textContent = I18N.t("valid.enter_totp"); return; }
    try {
      const r = await fetch(`${API}/account/recover-verify-mfa`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, code, totp_code: totp, purpose })
      });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) { errEl.textContent = j.error || I18N.t("toast.verify_failed"); return; }
      showRecoverResult(purpose, j);
    } catch (e) { errEl.textContent = I18N.t("toast.send_failed2") + e; }
  };
  setTimeout(() => { const el = $("rcTOTP"); if (el) el.focus(); }, 60);
}

function showRecoverResult(purpose, result) {
  const body = $("recoverBody");
  if (purpose === 'recover_username') {
    toast(I18N.t("toast.username_recovered"), "ok");
    body.innerHTML = `
      <div class="mfa-desc" style="margin-bottom:14px">${I18N.t("recover.username_recovered")}</div>
      <div class="field"><input type="text" value="${esc(result.username)}" readonly style="font-weight:700;font-size:16px;text-align:center;cursor:pointer" onclick="navigator.clipboard.writeText(this.value);toast(I18N.t('toast.copied'),'ok')" title="${I18N.t('toast.copied')}"></div>
      <div class="mfa-foot"><button class="btn primary" id="rcClose" type="button">${I18N.t("recover.back_to_login")}</button></div>`;
    $("rcClose").onclick = () => $("recoverMask").classList.remove("show");
  } else {
    showSetNewPassword(result.reset_token);
  }
}

function showSetNewPassword(token) {
  const body = $("recoverBody");
  body.innerHTML = `
    <div class="mfa-desc" style="margin-bottom:14px">${I18N.t("recover.enter_new_password")}</div>
    <div class="field"><label>${I18N.t("form.new_password_min4")}</label><input type="password" id="rcNewPass" placeholder="${I18N.t('form.new_password')}"></div>
    <div class="field"><label>${I18N.t('profile.confirm_password') || I18N.t('form.new_password')}</label><input type="password" id="rcNewPass2" placeholder="${I18N.t('form.new_password')}"></div>
    <div class="login-err" id="rcErr"></div>
    <div class="mfa-foot"><button class="btn danger" id="rcReset" type="button">${I18N.t("recover.reset_password_btn")}</button></div>`;

  $("rcReset").onclick = async () => {
    const errEl = $("rcErr"); errEl.textContent = "";
    const p1 = $("rcNewPass").value;
    const p2 = $("rcNewPass2").value;
    if (!pwPolicyOK(p1)) { errEl.textContent = I18N.t("auth.password_policy"); return; }
    if (p1 !== p2) { errEl.textContent = I18N.t("auth.password_mismatch"); return; }
    try {
      const r = await fetch(`${API}/account/reset-password`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reset_token: token, new_password: p1 })
      });
      const j = await r.json().catch(() => ({}));
      if (r.ok) {
        body.innerHTML = `
          <div class="mfa-desc" style="margin-bottom:14px;color:var(--ok);font-weight:600">✓ ${j.message || I18N.t("toast.password_reset2")}</div>
          <div class="mfa-foot"><button class="btn primary" id="rcClose" type="button">${I18N.t("recover.back_to_login")}</button></div>`;
        $("rcClose").onclick = () => $("recoverMask").classList.remove("show");
        toast(j.message || I18N.t("toast.password_reset2"), "ok");
      } else {
        errEl.textContent = j.error || I18N.t("toast.reset_failed");
      }
    } catch (e) { errEl.textContent = I18N.t("toast.reset_failed2") + e; }
  };
  setTimeout(() => { const el = $("rcNewPass"); if (el) el.focus(); }, 60);
}

async function logout() {
  try { await fetch(`${API}/logout`, { method: "POST" }); } catch (e) {}
  location.reload();
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

/* ---------- 事件绑定（委托） ---------- */
const groupsEl = $("groups");
// 折叠功能已临时停用：移除 group-head 点击事件委托中的 toggleCatCollapse 逻辑
if (groupsEl) {
  groupsEl.addEventListener("click", e => {
    const host = e.target.closest(".host"); if (!host) return;
    const act = e.target.closest("[data-act]");
    const { id, name, cat } = host.dataset;
    if (act) {
      if (act.dataset.act === "detail") openDetail(id, name);
      else if (act.dataset.act === "cat") editCategory(id, cat);
      else if (act.dataset.act === "del") delHost(id, name);
      else if (act.dataset.act === "term") openTerminal(id, name);
    } else {
      // 点击主机卡片/行内任意非操作按钮区域（进度条、负载、底部等）→ 打开详情
      openDetail(id, name);
    }
  });
}

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
    // Update options in existing dropdown
    updateCatDropdownOptions(cats);
    return;
  }
  // Remove native select
  const oldSel = $("catFilter");
  if (oldSel) oldSel.remove();
  wrap.innerHTML = `<div class="cat-dropdown" id="catDropdownWrap">
    <button class="cat-dd-btn" id="catDropdownBtn"><span id="catDropdownLabel">${I18N.t("ui.all_categories")}</span> <span class="dd-arrow">▾</span></button>
    <div class="cat-dd-menu" id="catDropdownMenu"></div>
  </div>`;
  updateCatDropdownOptions(cats);
  // Toggle menu
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
  // Clean stale selections
  CUR_CATS = CUR_CATS.filter(c => cats.includes(c));
  // P0-#1: Only rebuild DOM when category list actually changes
  const newKey = cats.join("\u0001");
  if (newKey === LAST_CATS_KEY) {
    // Same categories: just sync checkbox states without rebuilding DOM
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

// Helper function to safely add event listeners
function safeAddEventListener(id, event, handler) {
  const el = $(id);
  if (el) {
    el.addEventListener(event, handler);
  } else {
    console.warn(`Element with id "${id}" not found`);
  }
}

// 注：settingsBtn / themeToggle / topbarThemeBtn 已移入右上角用户下拉菜单
// 侧栏菜单
safeAddEventListener("saveBtn", "click", saveSettings);
safeAddEventListener("testBtn", "click", testSettings);
safeAddEventListener("installBtn", "click", openInstall);
safeAddEventListener("resetTokenBtn", "click", resetToken);
safeAddEventListener("tokenToggleBtn", "click", function() {
  TOKEN_REVEALED = !TOKEN_REVEALED;
  updateTokenDisplay();
  this.title = TOKEN_REVEALED ? I18N.t("ui.hide_token") : I18N.t("ui.show_token");
});
safeAddEventListener("copyCmdBtn", "click", function() {
  copyWithFeedback(this, $("installCmd").textContent, I18N.t("toast.copy_install"));
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
  copyWithFeedback(this, $("uninstallCmd").textContent, I18N.t("toast.copy_uninstall"));
});
// 安装模式切换（radio buttons）
document.querySelectorAll('input[name="installMode"]').forEach(r => {
  r.addEventListener("change", function() {
    RELAY_MODE = (this.value === "relay");
    MULTI_SERVER_MODE = (this.value === "multi");
    renderInstallCmd();
  });
});
// 多服务端推送列表变更
safeAddEventListener("multiServerList", "input", renderInstallCmd);
safeAddEventListener("relayGatewayIP", "input", renderInstallCmd);
safeAddEventListener("copyRelayGatewayBtn", "click", function() {
  copyWithFeedback(this, $("relayGatewayCmd").textContent, I18N.t("toast.copy_relay_install"));
});
safeAddEventListener("copyRelayInternalBtn", "click", function() {
  copyWithFeedback(this, $("relayInternalCmd").textContent, I18N.t("toast.copy_intranet_install"));
});

// 告警操作按钮事件委托（确认 / 静默 / 清除状态）
document.addEventListener("click", async function(e) {
  const btn = e.target.closest(".alert-action");
  if (!btn) return;
  e.preventDefault();
  e.stopPropagation();
  const action = btn.dataset.action;
  const body = JSON.stringify({
    host_id: btn.dataset.host || "",
    type: btn.dataset.type || "",
    scope: btn.dataset.scope || ""
  });
  try {
    const r = await fetch(`${API}/alerts/${action}`, { method: "POST", headers: { "Content-Type": "application/json" }, body });
    if (r.ok) {
      toast(action === "ack" ? I18N.t("toast.alert_ack") : action === "silence" ? I18N.t("toast.alert_silence") : I18N.t("toast.alert_cleared"), "ok");
      await refresh(true);
    } else {
      toast(I18N.t("toast.operation_failed"), "err");
    }
  } catch (e) { toast(I18N.t("toast.network_error"), "err"); }
});

/* ---------- 侧栏导航：视图切换 + 收起 + 移动抽屉 ---------- */
const navItems = document.querySelectorAll(".nav-item");
// 页面头元信息：标题 + 副标题。副标题让顶栏页面头承载“页面语义”，
// 而非机械回显侧栏导航名，从根上消除“两个概览”的重复观感。
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
// Rebuild the JS-baked page-meta strings in the current language (called on
// i18n:changed so titles/subtitles follow an in-place language switch).
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
navItems.forEach(n => n.addEventListener("click", () => {
  switchView(n.dataset.view);
  const appEl = $("app");
  if (appEl) appEl.classList.remove("nav-open");
}));

// Collapsible nav groups (Nightingale-style); collapsed state persists per group.
(function initNavGroups(){
  let collapsed = {};
  try { collapsed = JSON.parse(localStorage.getItem("aiops_nav_collapsed")||"{}"); } catch(e){}
  document.querySelectorAll(".nav-group").forEach(g => {
    const key = g.dataset.group;
    if (collapsed[key]) g.classList.add("collapsed");
    const label = g.querySelector(".nav-group-label");
    if (label) label.addEventListener("click", () => {
      g.classList.toggle("collapsed");
      collapsed[key] = g.classList.contains("collapsed");
      try { localStorage.setItem("aiops_nav_collapsed", JSON.stringify(collapsed)); } catch(e){}
    });
  });
})();

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

// 告警类型筛选
safeAddEventListener("alertFilter", "click", e => {
  const b = e.target.closest(".chip-btn"); if (!b) return;
  filterAlertsByType(b.dataset.atype);
});
// 告警搜索
safeAddEventListener("alertSearch", "input", e => { ALERT_SEARCH = e.target.value; renderAlerts(LAST_ALERTS); });

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
    if (mk.hasAttribute("data-forced")) return; // 强制弹窗（首次安全初始化）：禁止点遮罩/✕ 关闭
    mk.classList.remove("show"); hideChartTip();
    if (mk.id === "termMask") { closeTerminalWS(); }
    if (mk.id === "termReplayMask") { closeReplay(); }
    if (mk.id === "termObserveMask") { closeObserveWS(); }
    if (mk.id === "termSessionsMask") { if (TERM_SESSIONS_TIMER) { clearInterval(TERM_SESSIONS_TIMER); TERM_SESSIONS_TIMER = null; } }
    // v5.3.0: 终端认证弹窗关闭时清理状态
    if (mk.id === "termProtocolMask" || mk.id === "termSetPwdMask" || mk.id === "termVerifyMask" || mk.id === "termLockedMask") { cancelTermAuth(); }
  }
}));
document.addEventListener("keydown", e => {
  if (e.key === "Escape") {
    const hadTerm = $("termMask") && $("termMask").classList.contains("show");
    const hadReplay = $("termReplayMask") && $("termReplayMask").classList.contains("show");
    const hadObserve = $("termObserveMask") && $("termObserveMask").classList.contains("show");
    const hadSessions = $("termSessionsMask") && $("termSessionsMask").classList.contains("show");
    document.querySelectorAll(".mask.show:not([data-forced])").forEach(mk => mk.classList.remove("show"));
    hideChartTip();
    if (hadTerm) closeTerminalWS();
    if (hadReplay) closeReplay();
    if (hadObserve) closeObserveWS();
    if (hadSessions && TERM_SESSIONS_TIMER) { clearInterval(TERM_SESSIONS_TIMER); TERM_SESSIONS_TIMER = null; }
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
      .then(() => { toast(I18N.t("toast.check_triggered"), "ok"); setTimeout(loadChecks, 1500); })
      .catch(e => toast(I18N.t("toast.trigger_failed2") + e, "err"));
  }
});
// 概览 资源 TOP10 点击：主机详情 / 监控历史
safeAddEventListener("topPanels", "click", e => {
  // 行点击 → 主机详情
  const row = e.target.closest(".top-item");
  if (row) { openDetail(row.dataset.id, row.dataset.name); return; }
  // 监控探针点击
  const chk = e.target.closest(".checks-item");
  if (chk) { openCheckHistory(chk.dataset.checkId, chk.dataset.checkName, chk.dataset.checkType); return; }
});
// 日志导出
safeAddEventListener("exportLogBtn", "click", exportLogsCSV);
// 暂停自动刷新 + 批量清理离线
safeAddEventListener("pauseBtn", "click", togglePause);
safeAddEventListener("purgeOfflineBtn", "click", purgeOffline);
// ===== 顶栏用户菜单 =====
(function initUserDropdown() {
  const wrap = $("topbarUserWrap");
  const btn = $("profileBtn");
  if (!wrap || !btn) return;
  // 点击头像切换下拉
  btn.addEventListener("click", function(e) {
    e.stopPropagation();
    wrap.classList.toggle("open");
  });
  // 点击外部关闭
  document.addEventListener("click", function(e) {
    if (!wrap.contains(e.target)) wrap.classList.remove("open");
  });
  // ESC 关闭
  document.addEventListener("keydown", function(e) {
    if (e.key === "Escape") wrap.classList.remove("open");
  });
  // 主题切换
  safeAddEventListener("ddThemeToggle", "click", function() { toggleTheme(); wrap.classList.remove("open"); });
  // 语言切换（持久化到 cookie，就地重渲染所有文案，不刷新页面）
  var userDropdown = $("userDropdown");
  if (userDropdown) {
    userDropdown.addEventListener("click", function(e) {
      var b = e.target.closest("[data-lang]");
      if (b) I18N.setLang(b.dataset.lang);
    });
  }
  // 顶栏语言切换按钮组（简 / 繁 / EN）
  var tbLang = $("tbLang");
  if (tbLang) {
    tbLang.addEventListener("click", function(e) {
      var b = e.target.closest("[data-lang]");
      if (b) I18N.setLang(b.dataset.lang);
    });
  }
  // 标记当前选中的语言（涵盖顶栏与下拉两处 [data-lang] 控件）
  if (I18N.syncLangButtons) I18N.syncLangButtons();
  // 告警设置
  safeAddEventListener("ddSettings", "click", function() { openSettings(); wrap.classList.remove("open"); });
  // 告警设置弹窗内 Tab 切换
  safeAddEventListener("notifyTabs", "click", function(e) {
    const tab = e.target.closest(".tab");
    if (tab && tab.dataset.tab) switchNotifyTab(tab.dataset.tab);
  });
  // 个人信息
  safeAddEventListener("ddProfile", "click", function() { openProfile(); wrap.classList.remove("open"); });
  // 退出登录
  safeAddEventListener("ddLogout", "click", function() { logout(); wrap.classList.remove("open"); });
  // 初始化主题标签
})();
// 旧的 profileBtn 直接打开个人信息 — 已被上面的下拉菜单替代
// #usersBtn 已废弃（用户管理并入个人信息四 Tab），仅保留 openUsers() 重定向入口
safeAddEventListener("profileTabs", "click", function (e) {
  const t = e.target.closest(".tab");
  if (t && t.dataset.ptab) switchProfileTab(t.dataset.ptab);
});
safeAddEventListener("userAddBtn", "click", () => openUserEdit(null));
safeAddEventListener("globalMfaChk", "change", async () => {
  const chk = $("globalMfaChk");
  if (!chk) return;
  const required = chk.checked;
  chk.disabled = true;
  try {
    const r = await fetch(`${API}/mfa/global`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ required }) });
    const j = await r.json().catch(() => ({}));
    if (r.ok) toast(required ? I18N.t("toast.global_mfa_on") : I18N.t("toast.global_mfa_off"), "ok");
    else { toast(j.error || I18N.t("toast.operation_failed"), "err"); chk.checked = !required; }
  } catch (e) { toast(I18N.t("toast.network_error"), "err"); chk.checked = !required; }
  chk.disabled = false;
});
safeAddEventListener("usersList", "click", async e => {
  const btn = e.target.closest("[data-act]"); if (!btn) return;
  const row = e.target.closest(".user-row"); if (!row) return;
  const name = row.dataset.name, act = btn.dataset.act;
  if (act === "edit") {
    const users = await fetch(`${API}/users`).then(r => r.json()).catch(() => []);
    const u = Array.isArray(users) && users.find(x => x.username === name);
    if (u) openUserEdit(u);
  } else { usersAction(name, act); }
});
safeAddEventListener("pfSaveBtn", "click", saveProfile);
safeAddEventListener("pfPwdBtn", "click", changePassword);
// 首次登录·安全初始化弹窗：提交按钮 + 确认密码框回车提交
safeAddEventListener("initSubmitBtn", "click", submitInitSetup);
safeAddEventListener("initPass2", "keydown", e => { if (e.key === "Enter") { e.preventDefault(); submitInitSetup(); } });
safeAddEventListener("pfTermPwdBtn", "click", submitTermPwdChange);
safeAddEventListener("mfaToggleChk", "change", () => {
  const chk = $("mfaToggleChk");
  if (chk) chk.checked = MFA_ENABLED; // revert immediately; renderMfaState will update on success
  MFA_ENABLED ? openMfaDisable() : openMfaSetup();
});
safeAddEventListener("logoutBtn", "click", logout);
// 登录页找回入口
safeAddEventListener("forgotUserLink", "click", openRecoverUser);
safeAddEventListener("forgotPassLink", "click", openRecoverPass);

// 登录
safeAddEventListener("loginForm", "submit", async e => {
  e.preventDefault();
  const loginErrEl = $("loginErr");
  if (loginErrEl) loginErrEl.textContent = "";
  const submitBtn = e.target.querySelector('button[type="submit"]');
  await withLoading(submitBtn, async () => {
    try {
      const codeEl = $("loginCode");
      const r = await fetchWithTimeout(`${API}/login`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          username: $("loginUser").value.trim(),
          password: $("loginPass").value,
          code: codeEl ? codeEl.value.trim() : ""
        })
      }, 15000);
      const j = await r.json().catch(() => ({}));
      if (r.ok && j.mfa_required) {
        const f = $("loginCodeField"); if (f) f.style.display = "";
        if (codeEl) codeEl.focus();
        if (loginErrEl) loginErrEl.textContent = I18N.t("mfa.login_totp");
      }
      else if (r.ok && j.require_mfa_setup) {
        openMfaSetup(true);
      }
      else if (r.ok && j.must_change_password) {
        // v5.4.0: admin password was reset — force password change
        let user;
        try {
          user = await fetchWithTimeout(`${API}/me`, {}, 10000).then(x => x.json());
        } catch (_) {
          user = { username: $("loginUser").value.trim(), display_name: "" };
        }
        setUser(user);
        $("loginView").classList.remove("show");
        startApp();
        // 强制进入「安全初始化」弹窗：需修改用户名 + 密码后方可进入控制台
        setTimeout(() => openInitSetup(), 300);
      }
      else if (r.ok) {
        // Post-login /me fetch: wrap in try/catch so a transient network
        // hiccup doesn't leave the user stuck on the login page after
        // successful authentication.
        let user;
        try {
          user = await fetchWithTimeout(`${API}/me`, {}, 10000).then(x => x.json());
        } catch (_) {
          // Login succeeded but /me failed — proceed anyway, the next poll
          // will populate user info. Better than showing an error after
          // the user already typed their credentials correctly.
          user = { username: $("loginUser").value.trim(), display_name: "" };
        }
        setUser(user);
        const loginViewEl = $("loginView");
        if (loginViewEl) loginViewEl.classList.remove("show");
        startApp();
      }
      else {
        if (loginErrEl) loginErrEl.textContent = j.error || I18N.t("toast.login_failed");
      }
    } catch (err) {
      // Distinguish AbortError (timeout) from generic network errors so
      // mobile users see a helpful message instead of "TypeError: Failed to fetch".
      const msg = err.name === "AbortError"
        ? I18N.t("toast.login_timeout")
        : I18N.t("toast.login_network_error");
      if (loginErrEl) loginErrEl.textContent = msg;
    }
  });
});

/* ---------- 布局宽度切换（已移除，由默认值控制） ---------- */

/* ---------- 自定义监控视图切换（列表 / 胶囊） ---------- */
safeAddEventListener("checkViewToggle", "click", e => {
  const b = e.target.closest(".vt-btn"); if (!b) return;
  setCheckView(b.dataset.cview);
});
safeAddEventListener("hostViewToggle", "click", e => {
  const b = e.target.closest(".vt-btn"); if (!b) return;
  setHostView(b.dataset.hview);
});

// 读取本地偏好并应用（视图 / 布局宽度）
function initPrefs() {
  try { const cv = localStorage.getItem("aiops_check_view"); if (cv === "pill" || cv === "list") CHECK_VIEW = cv; } catch (e) {}
  try { const hv = localStorage.getItem("aiops_host_view"); if (hv === "list" || hv === "card") HOST_VIEW = hv; } catch (e) {}
  // 默认卡片视图：即使 localStorage 无值也确保 HOST_VIEW 为 "card"
  if (HOST_VIEW !== "list" && HOST_VIEW !== "card") HOST_VIEW = "card";
  document.querySelectorAll("#checkViewToggle .vt-btn").forEach(b => b.classList.toggle("active", b.dataset.cview === CHECK_VIEW));
  document.querySelectorAll("#hostViewToggle .vt-btn").forEach(b => b.classList.toggle("active", b.dataset.hview === HOST_VIEW));
}

initPrefs();
CUR_CATS = getSelectedCats();

/* ---------- P2-#18: Keyboard shortcuts 1-5 switch views ---------- */
document.addEventListener("keydown", e => {
  if (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA" || e.target.tagName === "SELECT") return;
  if (e.metaKey || e.ctrlKey || e.altKey) return;
  const views = ["overview", "hosts", "checks", "alerts", "automation", "log"];
  const idx = parseInt(e.key) - 1;
  if (idx >= 0 && idx < views.length) {
    e.preventDefault();
    switchView(views[idx]);
  }
});

/* ---------- Alert filter helpers ---------- */
function filterAlertsByType(type) {
  ALERT_TYPE = type;
  document.querySelectorAll("#alertFilter .chip-btn").forEach(b => b.classList.toggle("active", b.dataset.atype === type));
  renderAlerts(LAST_ALERTS);
}
let LAST_ALERTS = [];

/* ===================== 自动化运维：剧本编排 + 批量执行 ===================== */
let PB_HOSTS = []; // cached full host list for target selection
let PB_CATS = []; // cached unique categories

async function loadPlaybooks() {
  try {
    const [pbs, hosts] = await Promise.all([
      fetch(`${API}/playbooks`).then(r => r.json()),
      fetch(`${API}/hosts`).then(r => r.json())
    ]);
    PB_HOSTS = hosts || [];
    // Extract unique categories for target dropdown
    PB_CATS = [...new Set(PB_HOSTS.map(h => h.category || I18N.t("section.uncategorized")))].sort();
    // System types are hardcoded (linux/macos/windows) — do NOT extract from
    // h.platform (which is a version string like "Ubuntu 22.04"), use h.os
    // (runtime.GOOS: "linux"/"windows"/"darwin") for matching.
    renderPlaybooks(pbs || []);
  } catch (e) { console.warn("load playbooks:", e); }
}

function renderPlaybooks(pbs) {
  const list = $("playbookList"), empty = $("playbookEmpty");
  if (!list) return;
  if (empty) empty.style.display = pbs.length === 0 ? "" : "none";
  list.innerHTML = pbs.map(pb => {
    const stepCount = (pb.steps || []).length;
    const targets = [...new Set((pb.steps || []).map(s => s.target))];
    const sched = pb.schedule && pb.schedule.enabled;
    return `<div class="pb-card" data-id="${esc(pb.id)}">
      <div class="pb-card-top">
        <div class="pb-card-title">
          <strong>${esc(pb.name)}</strong>
          ${pb.description ? `<span class="pb-desc">${esc(pb.description)}</span>` : `<span class="pb-desc pb-desc-empty">暂无描述</span>`}
        </div>
        ${sched ? `<span class="pb-sched-badge" title="${I18N.t("playbook.sched_badge_title")}">⏱ ${esc(pbSchedLabel(pb.schedule))}</span>` : ""}
      </div>
      <div class="pb-card-foot">
        <div class="pb-pills">
          <span class="pb-pill">${stepCount} 步骤</span>
          <span class="pb-pill">${targets.length} 目标</span>
          <span class="pb-pill pb-pill-id mono">${esc(pb.id)}</span>
        </div>
        <div class="pb-actions">
          <button class="btn primary sm" data-pbact="exec">▶ ${I18N.t("ui.execute")}</button>
          <button class="btn sm" data-pbact="edit">${I18N.t("ui.edit")}</button>
          <button class="btn danger sm" data-pbact="del">${I18N.t("ui.delete")}</button>
        </div>
      </div>
    </div>`;
  }).join("");
}

function openPlaybookModal(pb) {
  $("playbookModalTitle").textContent = pb ? I18N.t("ui.edit_playbook") : I18N.t("ui.new_playbook");
  $("pbId").value = pb ? pb.id : "";
  $("pbName").value = pb ? pb.name : "";
  $("pbDesc").value = pb ? (pb.description || "") : "";
  const steps = pb ? pb.steps : [];
  renderPbSteps(steps.length > 0 ? steps : [{name:"",command:"",target:"all",timeout_sec:30,continue_on_error:false}]);
  // Populate the timed-trigger fields from the playbook's schedule (if any).
  const sc = (pb && pb.schedule) ? pb.schedule : null;
  $("pbSchedEnabled").checked = !!(sc && sc.enabled);
  $("pbSchedKind").value = (sc && sc.kind) || "interval";
  $("pbSchedInterval").value = (sc && sc.interval_min) || 60;
  $("pbSchedAt").value = (sc && sc.at) || "03:00";
  $("pbSchedWeekday").value = String((sc && typeof sc.weekday === "number") ? sc.weekday : 1);
  pbSchedRefresh();
  $("playbookMask").classList.add("show");
}

// Show/hide the schedule sub-fields based on the enable toggle + selected kind.
function pbSchedRefresh() {
  const on = $("pbSchedEnabled").checked;
  $("pbSchedFields").style.display = on ? "" : "none";
  const kind = $("pbSchedKind").value;
  $("pbSchedIntervalField").style.display = (kind === "interval") ? "" : "none";
  $("pbSchedAtField").style.display = (kind === "daily" || kind === "weekly") ? "" : "none";
  $("pbSchedWeekdayField").style.display = (kind === "weekly") ? "" : "none";
}

// Human-readable schedule summary for the playbook card badge.
function pbSchedLabel(sc) {
  if (!sc || !sc.enabled) return "";
  if (sc.kind === "interval") return `每 ${sc.interval_min} 分钟`;
  if (sc.kind === "daily") return `每天 ${sc.at}`;
  if (sc.kind === "weekly") { const wd = ["日","一","二","三","四","五","六"][sc.weekday] || ""; return `每周${wd} ${sc.at}`; }
  return "定时";
}

function renderPbSteps(steps) {
  const c = $("pbSteps");
  c.innerHTML = steps.map((s, i) => {
    const tgtOpts = buildTargetOptions(s.target);
    return `<div class="pb-step" data-idx="${i}">
      <div class="grid2">
        <div class="field"><label>${I18N.t("form.step_name")}</label><input type="text" class="pb-step-name" value="${esc(s.name||"")}" placeholder="${I18N.t('form.hint_step_name')}"></div>
        <div class="field"><label>${I18N.t("form.target")}</label><div class="select-wrap"><select class="pb-step-target" onchange="pbTargetPreview(this)">${tgtOpts}</select></div></div>
      </div>
      <div class="pb-target-preview" style="font-size:12px;color:var(--muted2);margin:-4px 0 4px"></div>
      <div class="field"><label>${I18N.t("form.command")}</label><textarea class="pb-step-cmd" rows="2" placeholder="${I18N.t('form.hint_command')}" spellcheck="false" style="resize:vertical;min-height:54px;line-height:1.5">${esc(s.command||"")}</textarea></div>
      <div class="grid2">
        <div class="field"><label>${I18N.t("form.timeout")}</label><input type="text" class="pb-step-timeout mono" value="${s.timeout_sec||30}" style="width:80px"></div>
        <div class="field"><label>${I18N.t("form.continue_err")}</label><label class="switch"><input type="checkbox" class="pb-step-cont" ${s.continue_on_error?"checked":""}> 继续下一步</label></div>
      </div>
      <button class="btn danger sm pb-step-del" type="button">${I18N.t("ui.delete_step")}</button>
    </div>`;
  }).join("");
  c.querySelectorAll(".pb-step-del").forEach(btn => {
    btn.onclick = () => { btn.closest(".pb-step").remove(); };
  });
  // Initialize previews
  c.querySelectorAll(".pb-step-target").forEach(sel => pbTargetPreview(sel));
}

// Build <option> list for target select: all / by category / by system / per host
function buildTargetOptions(selectedTarget) {
  const opts = [`<option value="all" ${selectedTarget==="all"?"selected":""}>${I18N.t("ui.all_hosts")}</option>`];
  // By category
  if (PB_CATS.length > 0) {
    opts.push(`<optgroup label="${I18N.t("section.by_category")}">`);
    PB_CATS.forEach(cat => {
      const val = `category:${cat}`;
      opts.push(`<option value="${esc(val)}" ${selectedTarget===val?"selected":""}>${esc(cat)}</option>`);
    });
    opts.push('</optgroup>');
  }
  // By system type — hardcoded to Linux/macOS/Windows (not dynamic from host
  // data, because h.platform is a version string, not an OS type).
  opts.push(`<optgroup label="${I18N.t("section.by_system")}">`);
  [{val:"linux",label:"Linux"},{val:"macos",label:"macOS"},{val:"windows",label:"Windows"}].forEach(s => {
    opts.push(`<option value="system:${s.val}" ${selectedTarget===`system:${s.val}`?"selected":""}>${s.label}</option>`);
  });
  opts.push('</optgroup>');
  // Per host
  if (PB_HOSTS.length > 0) {
    opts.push(`<optgroup label="${I18N.t("section.target_host")}">`);
    PB_HOSTS.forEach(h => {
      const val = `host:${h.id}`;
      opts.push(`<option value="${esc(val)}" ${selectedTarget===val?"selected":""}>${esc(h.hostname)}</option>`);
    });
    opts.push('</optgroup>');
  }
  return opts.join("");
}

// Preview matched host count when target changes
function pbTargetPreview(sel) {
  const step = sel.closest(".pb-step");
  if (!step) return;
  const preview = step.querySelector(".pb-target-preview");
  if (!preview) return;
  const target = sel.value;
  let count = 0;
  if (target === "all" || target === "") {
    count = PB_HOSTS.length;
  } else if (target.startsWith("category:")) {
    const cat = target.slice("category:".length);
    count = PB_HOSTS.filter(h => (h.category || I18N.t("section.uncategorized")) === cat).length;
  } else if (target.startsWith("system:")) {
    const sys = target.slice("system:".length);
    // Match by h.os (runtime.GOOS: "linux"/"windows"/"darwin"), not h.platform
    // (which is a version string). macOS hosts have h.os="darwin".
    count = PB_HOSTS.filter(h => {
      const os = (h.os || "").toLowerCase();
      return os === sys || (sys === "macos" && os === "darwin");
    }).length;
  } else if (target.startsWith("host:")) {
    count = 1;
  }
  preview.textContent = count > 0 ? `${I18N.t("ui.matched")} ${count} ${I18N.t("ui.hosts_matched")}` : I18N.t("empty.no_host_match2");
  preview.style.color = count > 0 ? "var(--ok, #31c46b)" : "var(--crit, #ff5b6e)";
}

function collectPlaybook() {
  const steps = [];
  document.querySelectorAll("#pbSteps .pb-step").forEach(el => {
    steps.push({
      name: el.querySelector(".pb-step-name").value.trim(),
      command: el.querySelector(".pb-step-cmd").value.trim(),
      target: el.querySelector(".pb-step-target").value,
      timeout_sec: parseInt(el.querySelector(".pb-step-timeout").value) || 30,
      continue_on_error: el.querySelector(".pb-step-cont").checked
    });
  });
  let schedule = null;
  if ($("pbSchedEnabled").checked) {
    const kind = $("pbSchedKind").value;
    schedule = { enabled: true, kind };
    if (kind === "interval") schedule.interval_min = parseInt($("pbSchedInterval").value) || 0;
    if (kind === "daily" || kind === "weekly") schedule.at = $("pbSchedAt").value.trim();
    if (kind === "weekly") schedule.weekday = parseInt($("pbSchedWeekday").value) || 0;
  }
  return { id: $("pbId").value, name: $("pbName").value.trim(), description: $("pbDesc").value.trim(), steps, schedule };
}

async function savePlaybook() {
  const pb = collectPlaybook();
  if (!pb.name) { toast(I18N.t("valid.fill_playbook_name"), "err"); return; }
  if (pb.steps.length === 0) { toast(I18N.t("valid.need_step"), "err"); return; }
  await withLoading("pbSaveBtn", async () => {
    try {
      const r = await fetch(`${API}/playbooks`, { method: "POST", headers: {"Content-Type":"application/json"}, body: JSON.stringify(pb) });
      const j = await r.json().catch(()=>({}));
      if (r.ok) { toast(I18N.t("toast.playbook_saved"), "ok"); $("playbookMask").classList.remove("show"); loadPlaybooks(); }
      else toast(j.error || I18N.t("toast.save_failed"), "err");
    } catch (e) { toast(I18N.t("toast.save_failed2") + e, "err"); }
  });
}

async function executePlaybook(id) {
  try {
    const r = await fetch(`${API}/playbooks/${encodeURIComponent(id)}/execute`, { method: "POST" });
    const j = await r.json().catch(()=>({}));
    if (r.ok) {
      toast(I18N.t("toast.playbook_started"), "ok");
      // Poll for result
      const execId = j.execution_id;
      pollExecution(execId, id);
    } else toast(j.error || I18N.t("toast.execute_failed"), "err");
  } catch (e) { toast(I18N.t("toast.execute_failed2") + e, "err"); }
}

async function pollExecution(execId, pbId) {
  $("execResultTitle").textContent = I18N.t("ui.running");
  $("execResultBody").innerHTML = `<div class="empty-line">${I18N.t("ui.executing")}</div>`;
  $("execResultMask").classList.add("show");
  for (let i = 0; i < 60; i++) {
    await new Promise(r => setTimeout(r, 2000));
    try {
      const exec = await fetch(`${API}/playbooks/executions/${execId}`).then(r => r.json());
      renderExecResult(exec);
      if (exec.status !== "running") break;
    } catch (e) {}
  }
}

function renderExecResult(exec) {
  $("execResultTitle").textContent = `${I18N.t("ui.execute")}${exec.status === "completed" ? I18N.t("ui.completed") : exec.status === "failed" ? I18N.t("ui.failed") : I18N.t("ui.running")}`;
  const rows = Object.entries(exec.host_results || {}).map(([hid, r]) => {
    const statusCls = r.status === "success" ? "ok" : r.status === "failed" ? "crit" : "warn";
    const steps = (r.steps || []).map(s => `<div class="exec-step ${s.status}"><span class="exec-step-name">${esc(s.name)}</span><span class="exec-step-status">${translateStepStatus(s.status)}</span><pre class="exec-step-out">${esc(s.output||"")}</pre></div>`).join("");
    return `<div class="exec-row">
      <div class="exec-row-head"><strong>${esc(r.hostname)}</strong> <span class="badge ${statusCls}">${translateExecStatus(r.status)}</span></div>
      <div class="exec-steps">${steps}</div>
    </div>`;
  }).join("");
  $("execResultBody").innerHTML = `<div class="exec-meta">${I18N.t("exec.operator")}: ${esc(exec.operator)} · ${I18N.t("exec.start_time")}: ${fmtDateTime(exec.start_time)}${exec.end_time?" · "+I18N.t("exec.end_time")+": "+fmtDateTime(exec.end_time):""} · ${I18N.t("exec.status_label")}: ${translateExecStatus(exec.status)}</div>${rows}`;
}

async function loadExecHistory() {
  try {
    const list = await fetch(`${API}/playbooks/executions`).then(r => r.json());
    const rows = (list || []).map(e => {
      const success = Object.values(e.host_results || {}).filter(r => r.status === "success").length;
      const total = Object.keys(e.host_results || {}).length;
      return `<div class="exec-hist-row" data-exec-id="${e.id}">
        <strong>${esc(e.playbook_name)}</strong>
        <span class="badge ${e.status === "completed" ? "ok" : e.status === "failed" ? "crit" : "warn"}">${translateExecStatus(e.status)}</span>
        <span class="mono" style="color:var(--muted)">${success}/${total} ${I18N.t("exec.success_count")}</span>
        <span class="mono" style="color:var(--muted)">${fmtDateTime(e.start_time)}</span>
        <span class="mono" style="color:var(--muted)">${esc(e.operator)}</span>
      </div>`;
    }).join("");
    $("execHistBody").innerHTML = rows || `<div class="empty-line">${I18N.t("empty.no_executions")}</div>`;
    $("execHistBody").querySelectorAll("[data-exec-id]").forEach(el => {
      el.onclick = async () => {
        const exec = await fetch(`${API}/playbooks/executions/${el.dataset.execId}`).then(r => r.json());
        renderExecResult(exec);
        $("execHistMask").classList.remove("show");
        $("execResultMask").classList.add("show");
      };
    });
    $("execHistMask").classList.add("show");
  } catch (e) { toast(I18N.t("toast.load_history_failed") + e, "err"); }
}

// Playbook event listeners
safeAddEventListener("addPlaybookBtn", "click", () => openPlaybookModal(null));
safeAddEventListener("pbAddStep", "click", () => {
  const c = $("pbSteps");
  const existing = Array.from(c.querySelectorAll(".pb-step")).map(el => ({
    name: el.querySelector(".pb-step-name").value, command: el.querySelector(".pb-step-cmd").value,
    target: el.querySelector(".pb-step-target").value, timeout_sec: parseInt(el.querySelector(".pb-step-timeout").value)||30,
    continue_on_error: el.querySelector(".pb-step-cont").checked
  }));
  existing.push({name:"",command:"",target:"all",timeout_sec:30,continue_on_error:false});
  renderPbSteps(existing);
});
safeAddEventListener("pbSaveBtn", "click", savePlaybook);
safeAddEventListener("pbSchedEnabled", "change", pbSchedRefresh);
safeAddEventListener("pbSchedKind", "change", pbSchedRefresh);
safeAddEventListener("pbHistoryBtn", "click", loadExecHistory);
safeAddEventListener("playbookList", "click", e => {
  const card = e.target.closest(".pb-card"); if (!card) return;
  const act = e.target.closest("[data-pbact]"); if (!act) return;
  const id = card.dataset.id;
  if (act.dataset.pbact === "exec") executePlaybook(id);
  else if (act.dataset.pbact === "edit") {
    fetch(`${API}/playbooks`).then(r=>r.json()).then(pbs => {
      const pb = pbs.find(p=>p.id===id); if (pb) openPlaybookModal(pb);
    });
  } else if (act.dataset.pbact === "del") {
    if (!confirm(I18N.t("valid.confirm_delete_playbook"))) return;
    fetch(`${API}/playbooks/${encodeURIComponent(id)}`, {method:"DELETE"}).then(()=>{toast(I18N.t("toast.deleted"),"ok");loadPlaybooks();});
  }
});

// ============ SRE 中枢：事件 / 自动修复 / SLO / 工单 ============
let SRE_TAB = "incidents";
let SRE_HOSTS = [], SRE_PLAYBOOKS = [], SRE_CHECKS = [], SRE_RULES = [], SRE_SLOS = [], SRE_TICKETS = [];
const SRE_ALERT_TYPES = ["cpu","memory","disk","diskio","iops","gpu","load","proc","offline","check"];
const _sevCls = s => s==="critical"?"crit":s==="warning"?"warn":"info";
const _srcLabel = s => ({alert:"告警",slo:"SLO",manual:"手动"})[s]||s;
const _incStatus = s => ({open:"进行中",acknowledged:"已确认",resolved:"已解决"})[s]||s;
const _incStatusCls = s => s==="resolved"?"ok":s==="acknowledged"?"warn":"crit";
const _tlKind = k => ({created:"创建",fired:"触发",recovered:"恢复",acked:"确认",resolved:"解决",remediation:"自动修复",comment:"评论",escalated:"升级工单",note:"备注",ai_diagnosis:"🤖 AI 诊断"})[k]||k;
const _runStatus = s => ({running:"执行中",success:"成功",failed:"失败",pending_approval:"待审批",skipped_cooldown:"冷却跳过",skipped_ratelimit:"限频跳过",rejected:"已拒绝",no_playbook:"无剧本"})[s]||s;
const _runCls = s => s==="success"?"ok":(s==="failed"||s==="no_playbook")?"crit":s==="pending_approval"?"warn":s.indexOf("skipped")===0||s==="rejected"?"warn":"info";
const _prioCls = p => p==="p1"?"crit":p==="p2"?"warn":"info";
const _tkStatusCls = s => (s==="resolved"||s==="closed")?"ok":s==="in_progress"?"warn":"info";

async function loadSRE(){
  try {
    const [hosts, pbs] = await Promise.all([
      fetch(`${API}/hosts`).then(r=>r.json()),
      fetch(`${API}/playbooks`).then(r=>r.json())
    ]);
    SRE_HOSTS = hosts||[]; SRE_PLAYBOOKS = pbs||[];
  } catch(e){}
  try { SRE_CHECKS = (await fetch(`${API}/checks`).then(r=>r.json()))||[]; } catch(e){ SRE_CHECKS=[]; }
  loadSRETab(SRE_TAB); loadSREBadge();
}
async function loadSREBadge(){
  try {
    const o = await fetch(`${API}/sre/overview`).then(r=>r.json());
    const b = $("navSre"), n = (o.open_incidents||0)+(o.pending_remediations||0);
    if (b){ b.textContent=n; b.style.display=n>0?"":"none"; }
  } catch(e){}
}
function switchSRETab(tab){
  SRE_TAB = tab;
  document.querySelectorAll("#sreTabs .chip-btn").forEach(b=>b.classList.toggle("active", b.dataset.sretab===tab));
  document.querySelectorAll(".sre-panel").forEach(p=>p.classList.toggle("active", p.id==="srePanel-"+tab));
  loadSRETab(tab);
}
function loadSRETab(tab){
  if (tab==="incidents") loadIncidents();
  else if (tab==="remediation") loadRemediation();
  else if (tab==="slo") loadSLOs();
  else if (tab==="tickets") loadTickets();
  else if (tab==="ai") loadInspections();
}

/* ---- 事件 ---- */
async function loadIncidents(){
  try {
    const list = await fetch(`${API}/incidents`).then(r=>r.json());
    const el = $("incidentList");
    if (!list||!list.length){ el.innerHTML=`<div class="empty-line">暂无事件</div>`; return; }
    el.innerHTML = list.map(i=>`<div class="sre-row" data-incident="${i.id}">
      <span class="badge ${_sevCls(i.severity)}">${esc(i.severity)}</span>
      <div class="sre-row-main"><div class="sre-row-title">${esc(i.title)}</div>
        <div class="sre-row-sub">#${i.id} · ${_srcLabel(i.source)}${i.hostname?" · "+esc(i.hostname):""} · ${fmtDateTime(i.created_at)}</div></div>
      <span class="badge ${_incStatusCls(i.status)}">${_incStatus(i.status)}</span></div>`).join("");
    el.querySelectorAll("[data-incident]").forEach(r=>r.onclick=()=>openIncidentDetail(r.dataset.incident));
  } catch(e){ toast("加载失败: "+e,"err"); }
}
async function openIncidentDetail(id){
  try {
    const inc = await fetch(`${API}/incidents/${id}`).then(r=>r.json());
    $("incidentDetailTitle").textContent = `#${inc.id} ${inc.title}`;
    const tl = (inc.timeline||[]).slice().reverse().map(e=>`<div class="tl-item">
      <div class="tl-dot ${_sevCls(inc.severity)}"></div>
      <div class="tl-body"><div class="tl-head"><b>${_tlKind(e.kind)}</b> <span class="tl-time">${fmtDateTime(e.ts)}</span>${e.actor?` · <span class="tl-actor">${esc(e.actor)}</span>`:""}</div>${e.text?`<div class="tl-text">${esc(e.text)}</div>`:""}</div></div>`).join("");
    $("incidentDetailBody").innerHTML = `<div class="sre-meta">
      <span class="badge ${_sevCls(inc.severity)}">${esc(inc.severity)}</span>
      <span class="badge ${_incStatusCls(inc.status)}">${_incStatus(inc.status)}</span>
      <span class="mono" style="color:var(--muted)">${_srcLabel(inc.source)}${inc.hostname?" · "+esc(inc.hostname):""}</span>
      ${inc.ticket_id?`<span class="mono" style="color:var(--muted)">🎫 工单 #${inc.ticket_id}</span>`:""}</div>
      <div class="subhead">时间线</div><div class="timeline">${tl||`<div class="empty-line">—</div>`}</div>`;
    const acts=[];
    acts.push(`<button class="btn sm" data-iact="diagnose">🤖 AI 诊断</button>`);
    if (inc.status!=="resolved"){ acts.push(`<button class="btn sm" data-iact="ack">确认</button>`); acts.push(`<button class="btn sm" data-iact="resolve">解决</button>`); }
    if (!inc.ticket_id) acts.push(`<button class="btn sm" data-iact="escalate">升级工单</button>`);
    acts.push(`<div style="flex:1"></div><input type="text" id="incCommentInput" placeholder="添加评论…" style="flex:2;min-width:120px"><button class="btn primary sm" data-iact="comment">发送</button>`);
    const foot=$("incidentDetailFoot"); foot.innerHTML=acts.join("");
    foot.querySelectorAll("[data-iact]").forEach(b=>b.onclick=()=>incidentAction(inc.id,b.dataset.iact));
    $("incidentDetailMask").classList.add("show");
  } catch(e){ toast("加载失败: "+e,"err"); }
}
async function incidentAction(id, act){
  try {
    if (act==="comment"){ const t=$("incCommentInput").value.trim(); if(!t)return;
      await fetch(`${API}/incidents/${id}/comment`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({text:t})}); }
    else if (act==="escalate"){ await fetch(`${API}/incidents/${id}/ticket`,{method:"POST"}); toast("已升级为工单","ok"); }
    else if (act==="diagnose"){ toast("AI 诊断中，请稍候…","ok"); await fetch(`${API}/incidents/${id}/diagnose`,{method:"POST"}); }
    else await fetch(`${API}/incidents/${id}/${act}`,{method:"POST"});
    openIncidentDetail(id); loadIncidents(); loadSREBadge();
  } catch(e){ toast("操作失败: "+e,"err"); }
}
function openNewIncident(){
  $("niTitle").value=""; $("niSeverity").value="warning";
  $("niHost").innerHTML=`<option value="">—</option>`+SRE_HOSTS.map(h=>`<option value="${esc(h.id)}">${esc(h.hostname)}</option>`).join("");
  $("newIncidentMask").classList.add("show");
}
async function saveNewIncident(){
  const title=$("niTitle").value.trim(); if(!title){ toast("请填写标题","err"); return; }
  await fetch(`${API}/incidents`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({title,severity:$("niSeverity").value,host_id:$("niHost").value})});
  $("newIncidentMask").classList.remove("show"); loadIncidents(); loadSREBadge(); toast("已保存","ok");
}

/* ---- 自动修复 ---- */
async function loadRemediation(){
  try {
    const [rules,runs] = await Promise.all([fetch(`${API}/remediation/rules`).then(r=>r.json()),fetch(`${API}/remediation/runs`).then(r=>r.json())]);
    SRE_RULES = rules||[]; renderRules(SRE_RULES); renderRuns(runs||[]);
  } catch(e){ toast("加载失败: "+e,"err"); }
}
function renderRules(rules){
  const el=$("remediationRuleList");
  if(!rules.length){ el.innerHTML=`<div class="empty-line">暂无修复规则</div>`; return; }
  el.innerHTML = rules.map(r=>{
    const pb=SRE_PLAYBOOKS.find(p=>p.id===r.playbook_id);
    const g=[]; if(r.require_approval)g.push("需审批"); if(r.cooldown_sec)g.push(`冷却${r.cooldown_sec}s`); if(r.max_per_hour)g.push(`≤${r.max_per_hour}/h`);
    const match=(r.match_types&&r.match_types.length?r.match_types.join("/"):"任意类型")+(r.min_level?` ≥${r.min_level}`:"");
    return `<div class="pb-card fwd-card ${r.enabled?"":"pb-off"}" data-rule="${esc(r.id)}">
      <div class="pb-card-top"><div class="pb-card-title"><strong>${esc(r.name)}</strong><span class="pb-desc">${esc(match)} → ${esc(pb?pb.name:r.playbook_id)}</span></div>
        <span class="fwd-status ${r.enabled?"on":"off"}">${r.enabled?"已启用":"已停用"}</span></div>
      <div class="pb-card-foot"><div class="pb-pills">${g.map(x=>`<span class="badge">${esc(x)}</span>`).join("")}</div>
        <div class="fwd-actions"><button class="btn sm" data-rract="edit">编辑</button><button class="btn danger sm" data-rract="del">删除</button></div></div></div>`;
  }).join("");
  el.querySelectorAll("[data-rule]").forEach(card=>card.querySelectorAll("[data-rract]").forEach(b=>b.onclick=e=>{ e.stopPropagation();
    const id=card.dataset.rule;
    if(b.dataset.rract==="edit") openRuleModal(SRE_RULES.find(x=>x.id===id));
    else if(confirm("确认删除该规则？")) fetch(`${API}/remediation/rules/${id}`,{method:"DELETE"}).then(()=>loadRemediation());
  }));
}
function renderRuns(runs){
  const el=$("remediationRunList");
  if(!runs.length){ el.innerHTML=`<div class="empty-line">暂无执行记录</div>`; return; }
  el.innerHTML = runs.map(r=>`<div class="sre-row">
    <span class="badge ${_runCls(r.status)}">${_runStatus(r.status)}</span>
    <div class="sre-row-main"><div class="sre-row-title">${esc(r.rule_name)} → ${esc(r.playbook_name||r.playbook_id)}</div>
      <div class="sre-row-sub">${esc(r.hostname)} · ${esc(r.alert_type)} · ${fmtDateTime(r.created_at)}${r.reason?" · "+esc(r.reason):""}</div></div>
    ${r.status==="pending_approval"?`<div class="fwd-actions"><button class="btn primary sm" data-run="${r.id}" data-runact="approve">批准</button><button class="btn danger sm" data-run="${r.id}" data-runact="reject">拒绝</button></div>`:""}</div>`).join("");
  el.querySelectorAll("[data-runact]").forEach(b=>b.onclick=async()=>{ await fetch(`${API}/remediation/runs/${b.dataset.run}/${b.dataset.runact}`,{method:"POST"}); loadRemediation(); loadSREBadge(); });
}
function openRuleModal(r){
  $("rrId").value=r?r.id:""; $("rrTitle").textContent=r?"编辑规则":"新建规则";
  $("rrName").value=r?r.name:""; $("rrEnabled").checked=r?r.enabled:true;
  $("rrLevel").value=r?(r.min_level||""):"critical"; $("rrCategory").value=r?(r.match_category||""):"";
  $("rrCooldown").value=r?r.cooldown_sec:300; $("rrMaxPerHour").value=r?r.max_per_hour:6; $("rrApproval").checked=r?r.require_approval:false;
  $("rrPlaybook").innerHTML=SRE_PLAYBOOKS.map(p=>`<option value="${esc(p.id)}" ${r&&r.playbook_id===p.id?"selected":""}>${esc(p.name)}</option>`).join("")||`<option value="">（请先创建剧本）</option>`;
  const sel=new Set(r?(r.match_types||[]):[]);
  $("rrTypes").innerHTML=SRE_ALERT_TYPES.map(t=>`<label class="chip-check"><input type="checkbox" value="${t}" ${sel.has(t)?"checked":""}> ${t}</label>`).join("");
  $("remediationRuleMask").classList.add("show");
}
async function saveRule(){
  const types=[...document.querySelectorAll("#rrTypes input:checked")].map(c=>c.value);
  const body={id:$("rrId").value,name:$("rrName").value.trim(),enabled:$("rrEnabled").checked,match_types:types,min_level:$("rrLevel").value,match_category:$("rrCategory").value.trim(),playbook_id:$("rrPlaybook").value,require_approval:$("rrApproval").checked,cooldown_sec:parseInt($("rrCooldown").value)||0,max_per_hour:parseInt($("rrMaxPerHour").value)||0};
  const r=await fetch(`${API}/remediation/rules`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
  const j=await r.json().catch(()=>({}));
  if(r.ok){ $("remediationRuleMask").classList.remove("show"); loadRemediation(); toast("已保存","ok"); } else toast(j.error||"保存失败","err");
}

/* ---- SLO ---- */
async function loadSLOs(){
  try { SRE_SLOS = (await fetch(`${API}/slos`).then(r=>r.json()))||[]; renderSLOs(SRE_SLOS); }
  catch(e){ toast("加载失败: "+e,"err"); }
}
function renderSLOs(list){
  const el=$("sloList");
  if(!list.length){ el.innerHTML=`<div class="empty-line">暂无 SLO</div>`; return; }
  el.innerHTML=list.map(s=>{
    const bCls=s.error_budget<=0?"crit":s.error_budget<30?"warn":"ok";
    const src=s.source_type==="check"?"拨测 up 率":`${s.metric} ${s.comparator} ${s.threshold}`;
    return `<div class="pb-card fwd-card ${s.enabled?"":"pb-off"}" data-slo="${esc(s.id)}">
      <div class="pb-card-top"><div class="pb-card-title"><strong>${esc(s.name)}</strong><span class="pb-desc">${esc(src)} · 目标 ${s.target}% · ${s.window_days}d</span></div>
        <span class="badge ${s.breaching?"crit":"ok"}">SLI ${s.sli.toFixed(2)}%</span></div>
      <div class="slo-budget"><div class="slo-budget-bar"><div class="slo-budget-fill ${bCls}" style="width:${Math.max(0,Math.min(100,s.error_budget))}%"></div></div>
        <div class="slo-budget-txt">错误预算 ${s.error_budget.toFixed(0)}% · 燃尽 ${s.burn_rate.toFixed(2)}× · 达标 ${s.good_events}/${s.total_events}</div></div>
      <div class="pb-card-foot"><div class="pb-pills">${s.breaching?`<span class="badge crit">超标</span>`:`<span class="badge ok">健康</span>`}${s.enabled?"":`<span class="badge">停用</span>`}</div>
        <div class="fwd-actions"><button class="btn sm" data-sloact="edit">编辑</button><button class="btn danger sm" data-sloact="del">删除</button></div></div></div>`;
  }).join("");
  el.querySelectorAll("[data-slo]").forEach(card=>card.querySelectorAll("[data-sloact]").forEach(b=>b.onclick=e=>{ e.stopPropagation();
    const id=card.dataset.slo;
    if(b.dataset.sloact==="edit") openSloModal(SRE_SLOS.find(x=>x.id===id));
    else if(confirm("确认删除该 SLO？")) fetch(`${API}/slos/${id}`,{method:"DELETE"}).then(()=>loadSLOs());
  }));
}
function sloSourceChange(){
  const src=$("sloSource").value;
  $("sloCheckField").style.display=src==="check"?"":"none";
  $("sloMetricFields").style.display=src==="metric"?"":"none";
}
function openSloModal(s){
  $("sloId").value=s?s.id:""; $("sloModalTitle").textContent=s?"编辑 SLO":"新建 SLO";
  $("sloName").value=s?s.name:""; $("sloEnabled").checked=s?s.enabled:true; $("sloSource").value=s?s.source_type:"check";
  $("sloCheck").innerHTML=SRE_CHECKS.map(c=>`<option value="${esc(c.id)}" ${s&&s.check_id===c.id?"selected":""}>${esc(c.name)}</option>`).join("")||`<option value="">（请先创建拨测）</option>`;
  $("sloHost").innerHTML=SRE_HOSTS.map(h=>`<option value="${esc(h.id)}" ${s&&s.host_id===h.id?"selected":""}>${esc(h.hostname)}</option>`).join("");
  if(s){ $("sloMetric").value=s.metric||"cpu_percent"; $("sloComparator").value=s.comparator||"<"; $("sloThreshold").value=s.threshold||90; } else { $("sloComparator").value="<"; $("sloThreshold").value=90; }
  $("sloTarget").value=s?s.target:99.9; $("sloWindow").value=s?s.window_days:30;
  sloSourceChange(); $("sloMask").classList.add("show");
}
async function saveSlo(){
  const src=$("sloSource").value;
  const body={id:$("sloId").value,name:$("sloName").value.trim(),enabled:$("sloEnabled").checked,source_type:src,target:parseFloat($("sloTarget").value)||99,window_days:parseInt($("sloWindow").value)||30};
  if(src==="check") body.check_id=$("sloCheck").value;
  else { body.host_id=$("sloHost").value; body.metric=$("sloMetric").value; body.comparator=$("sloComparator").value; body.threshold=parseFloat($("sloThreshold").value)||0; }
  const r=await fetch(`${API}/slos`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
  const j=await r.json().catch(()=>({}));
  if(r.ok){ $("sloMask").classList.remove("show"); loadSLOs(); toast("已保存","ok"); } else toast(j.error||"保存失败","err");
}

/* ---- 工单 ---- */
async function loadTickets(){
  try { SRE_TICKETS=(await fetch(`${API}/tickets`).then(r=>r.json()))||[]; renderTickets(SRE_TICKETS); }
  catch(e){ toast("加载失败: "+e,"err"); }
}
function renderTickets(list){
  const el=$("ticketList");
  if(!list.length){ el.innerHTML=`<div class="empty-line">暂无工单</div>`; return; }
  el.innerHTML=list.map(t=>`<div class="sre-row" data-ticket="${t.id}">
    <span class="badge ${_prioCls(t.priority)}">${esc((t.priority||"p3").toUpperCase())}</span>
    <div class="sre-row-main"><div class="sre-row-title">${esc(t.title)}</div>
      <div class="sre-row-sub">#${t.id}${t.assignee?" · @"+esc(t.assignee):""}${t.incident_id?" · 🔗事件#"+t.incident_id:""} · ${fmtDateTime(t.updated_at)}</div></div>
    <span class="badge ${_tkStatusCls(t.status)}">${esc(t.status)}</span></div>`).join("");
  el.querySelectorAll("[data-ticket]").forEach(row=>row.onclick=()=>openTicketModal(SRE_TICKETS.find(x=>x.id==row.dataset.ticket)));
}
function openTicketModal(t){
  $("ticketId").value=t?t.id:""; $("ticketModalTitle").textContent=t?`#${t.id} ${t.title}`:"新建工单";
  $("tkTitle").value=t?t.title:""; $("tkPriority").value=t?t.priority:"p3"; $("tkStatus").value=t?t.status:"open";
  $("tkAssignee").value=t?(t.assignee||""):""; $("tkDesc").value=t?(t.description||""):"";
  const cm=$("tkComments"),cf=$("tkCommentField");
  if(t){ cm.innerHTML=`<div class="subhead">评论</div>`+((t.comments||[]).map(c=>`<div class="tk-comment"><span class="tk-c-author">${esc(c.author)}</span> <span class="tk-c-time">${fmtDateTime(c.ts)}</span><div>${esc(c.text)}</div></div>`).join("")||`<div class="empty-line">—</div>`); cf.style.display=""; }
  else { cm.innerHTML=""; cf.style.display="none"; }
  $("ticketMask").classList.add("show");
}
async function saveTicket(){
  const id=$("ticketId").value;
  const body={title:$("tkTitle").value.trim(),priority:$("tkPriority").value,status:$("tkStatus").value,assignee:$("tkAssignee").value.trim(),description:$("tkDesc").value.trim()};
  if(!body.title){ toast("请填写标题","err"); return; }
  const r=await fetch(id?`${API}/tickets/${id}`:`${API}/tickets`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
  const j=await r.json().catch(()=>({}));
  if(r.ok){ $("ticketMask").classList.remove("show"); loadTickets(); loadSREBadge(); toast("已保存","ok"); } else toast(j.error||"保存失败","err");
}
async function addTicketComment(){
  const id=$("ticketId").value,t=$("tkCommentInput").value.trim(); if(!id||!t)return;
  await fetch(`${API}/tickets/${id}/comment`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({text:t})});
  $("tkCommentInput").value=""; const tk=await fetch(`${API}/tickets/${id}`).then(r=>r.json()); openTicketModal(tk); loadTickets();
}

document.querySelectorAll("#sreTabs .chip-btn").forEach(b=>b.addEventListener("click",()=>switchSRETab(b.dataset.sretab)));
safeAddEventListener("newIncidentBtn","click",openNewIncident);
safeAddEventListener("niSaveBtn","click",saveNewIncident);
safeAddEventListener("newRemediationBtn","click",()=>openRuleModal(null));
safeAddEventListener("rrSaveBtn","click",saveRule);
safeAddEventListener("newSloBtn","click",()=>openSloModal(null));
safeAddEventListener("sloSaveBtn","click",saveSlo);
safeAddEventListener("sloSource","change",sloSourceChange);
safeAddEventListener("newTicketBtn","click",()=>openTicketModal(null));
safeAddEventListener("tkSaveBtn","click",saveTicket);
safeAddEventListener("tkCommentBtn","click",addTicketComment);

/* ---- 日志检索 ---- */
const _logLvlCls = l => l==="error"?"crit":l==="warn"?"warn":"info";
async function loadLogs(){
  try { if (!SRE_HOSTS.length) SRE_HOSTS=(await fetch(`${API}/hosts`).then(r=>r.json()))||[]; } catch(e){}
  const hs=$("logHost");
  if (hs && hs.options.length<=1) hs.innerHTML=`<option value="">全部主机</option>`+SRE_HOSTS.map(h=>`<option value="${esc(h.id)}">${esc(h.hostname)}</option>`).join("");
  searchLogs();
}
async function searchLogs(){
  const host=$("logHost").value,level=$("logLevel").value,since=$("logSince").value,kw=$("logKeyword").value.trim();
  const qs=new URLSearchParams(); if(host)qs.set("host",host); if(level)qs.set("level",level); if(since&&since!=="0")qs.set("since_min",since); if(kw)qs.set("q",kw); qs.set("limit","500");
  try {
    const list=await fetch(`${API}/logs?${qs}`).then(r=>r.json());
    const el=$("logResults");
    if(!list||!list.length){ el.innerHTML=`<div class="empty-line">无匹配日志（被控端需以 --log-paths 指定采集文件）</div>`; return; }
    el.innerHTML=list.map(l=>`<div class="log-line ${_logLvlCls(l.level)}"><span class="log-ts mono">${fmtDateTime(l.ts)}</span><span class="log-lvl ${_logLvlCls(l.level)}">${esc(l.level)}</span><span class="log-host">${esc(l.hostname)}</span><span class="log-msg">${esc(l.message)}</span></div>`).join("");
  } catch(e){ toast("检索失败: "+e,"err"); }
}

/* ---- AI 巡检 ---- */
async function loadInspections(){
  try {
    const list=await fetch(`${API}/ai/inspections`).then(r=>r.json());
    const el=$("aiReportList");
    if(!list||!list.length){ el.innerHTML=`<div class="empty-line">暂无巡检报告，点「立即巡检」生成一次。</div>`; return; }
    el.innerHTML=list.map(rep=>{
      const f=(rep.findings||[]).map(x=>`<div class="ai-finding"><span class="badge ${_sevCls(x.severity)}">${esc(x.severity)}</span><div class="ai-f-body"><div class="ai-f-title">${esc(x.title)}</div>${x.detail?`<div class="ai-f-detail">${esc(x.detail)}</div>`:""}</div></div>`).join("");
      return `<div class="ai-report"><div class="ai-report-head"><span class="badge ${rep.source==="ai"?"info":""}">${rep.source==="ai"?"AI 研判":"启发式"}</span><span class="ai-report-trigger">${rep.trigger==="manual"?"手动":"定时"}</span><span class="mono" style="color:var(--muted)">${fmtDateTime(rep.ts)}</span></div>
        <div class="ai-summary">${esc(rep.summary)}</div>${f?`<div class="ai-findings">${f}</div>`:""}</div>`;
    }).join("");
  } catch(e){ toast("加载失败: "+e,"err"); }
}
async function runInspect(){ toast("巡检中…","ok"); try { await fetch(`${API}/ai/inspect`,{method:"POST"}); loadInspections(); } catch(e){ toast("巡检失败: "+e,"err"); } }
async function openAIConfig(){
  try { const c=await fetch(`${API}/ai/config`).then(r=>r.json());
    $("aiEnabled").checked=!!c.enabled; $("aiEndpoint").value=c.endpoint||""; $("aiKey").value=c.api_key||""; $("aiModel").value=c.model||""; $("aiInterval").value=c.inspect_interval_min||30;
  } catch(e){}
  $("aiConfigMask").classList.add("show");
}
async function saveAIConfig(){
  const body={enabled:$("aiEnabled").checked,endpoint:$("aiEndpoint").value.trim(),api_key:$("aiKey").value,model:$("aiModel").value.trim(),inspect_interval_min:parseInt($("aiInterval").value)||30};
  const r=await fetch(`${API}/ai/config`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
  if(r.ok){ $("aiConfigMask").classList.remove("show"); toast("已保存","ok"); } else toast("保存失败","err");
}
safeAddEventListener("logSearchBtn","click",searchLogs);
safeAddEventListener("logKeyword","keydown",e=>{ if(e.key==="Enter") searchLogs(); });
safeAddEventListener("aiInspectBtn","click",runInspect);
safeAddEventListener("aiConfigBtn","click",openAIConfig);
safeAddEventListener("aiConfigSaveBtn","click",saveAIConfig);

// 终端会话管理 + 回放 + 旁观
safeAddEventListener("termSessionsBtn", "click", openTerminalSessions);
// 终端会话搜索
safeAddEventListener("termSessionSearch", "input", e => {
  TERM_SEARCH = e.target.value;
  renderTerminalSessions(LAST_TERM_SESSIONS);
});
safeAddEventListener("replayPlayBtn", "click", () => { if (REPLAY && REPLAY.playing) pauseReplay(); else playReplay(); });
safeAddEventListener("replayProgressBg", "click", e => {
  const rect = e.currentTarget.getBoundingClientRect();
  const progress = (e.clientX - rect.left) / rect.width;
  seekReplay(Math.max(0, Math.min(1, progress)));
});
document.querySelectorAll(".replay-speed-btn").forEach(btn => {
  btn.addEventListener("click", () => setReplaySpeed(parseFloat(btn.dataset.speed)));
});

/* ---------- PWA: SW registration + Install prompt + Hash routing ---------- */
// P1-4: 全局 Escape 键关闭模态弹窗
document.addEventListener("keydown", e => {
  if (e.key === "Escape") {
    const masks = document.querySelectorAll(".mask.show:not([data-forced])");
    if (masks.length > 0) {
      // 只关闭最上层的弹窗
      const top = masks[masks.length - 1];
      closeMask(top);
    }
  }
});
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js", { scope: "/" }).catch(() => {});
  });
}
let DEFERRED_PROMPT = null;
window.addEventListener("beforeinstallprompt", e => {
  e.preventDefault();
  DEFERRED_PROMPT = e;
  setTimeout(() => {
    if (DEFERRED_PROMPT && !window.matchMedia("(display-mode: standalone)").matches) {
      toast(I18N.t("install.pwa_hint"), "ok");
    }
  }, 3000);
});
window.addEventListener("hashchange", () => {
  const h = location.hash.slice(1);
  if (h && ["overview", "hosts", "checks", "alerts", "automation", "log"].includes(h)) {
    switchView(h);
  }
});

// 语言就地切换：i18n-dashboard.js 已完成静态 [data-i18n] 文本替换，这里负责
// 重建 JS 动态生成的文案（页面标题/副标题、各视图列表、图表等），并保持当前视图/滚动/
// 已打开面板不变（不刷新页面）。
document.addEventListener("i18n:changed", () => {
  try { rebuildPageMeta(); } catch (e) {}
  const activeNav = document.querySelector(".nav-item.active");
  const view = activeNav ? activeNav.dataset.view : null;
  if (view && PAGE_META[view]) {
    const t = $("pageTitle"), s = $("pageSub");
    if (t) t.textContent = PAGE_META[view].title;
    if (s) s.textContent = PAGE_META[view].sub;
  }
  // 概览数据（卡片/健康/告警/活动/主机/TOP）强制重渲染
  try { refresh(true); } catch (e) {}
  // 当前视图专属的动态列表按需重载（模块级筛选/分页状态保持不变）
  try {
    if (view === "checks") loadChecks();
    else if (view === "automation") loadPlaybooks();
    else if (view === "forward") { loadForwards(); loadHttpProxies(); }
    else if (view === "hosts") loadHostsMeta();
  } catch (e) {}
  if (I18N.syncLangButtons) I18N.syncLangButtons();
});

initAuth();

/* ============================================================
   P3-1: WebSocket 推送（替代轮询，带降级）
   ============================================================ */
let PUSH_WS = null;
let PUSH_CONNECTED = false;
let PUSH_RETRY = 0;

function initPushWS() {
  // 仅在 HTTPS 或 localhost 下尝试 WebSocket
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
      // 指数退避重连
      PUSH_RETRY++;
      if (PUSH_RETRY <= 10) {
        setTimeout(() => initPushWS(), Math.min(30000, 1000 * Math.pow(2, PUSH_RETRY)));
      }
    };
    PUSH_WS.onerror = () => { try { PUSH_WS.close(); } catch(e) {} };
  } catch(e) {}
}

/* ============================================================
   端口转发
   ============================================================ */
let LAST_FORWARDS = [];
let LAST_HTTP_PROXIES = [];
let FWD_MODE = "tcp"; // "tcp" | "http"

// 填充主机下拉选择框（同时填充创建弹窗 fwdHost 和编辑弹窗 fwdEditHost）
function populateForwardHosts() {
  const opts = LAST_HOSTS.map(h => `<option value="${h.id}">${esc(h.hostname)} (${short(h.id)})</option>`).join("");
  const fh = $("fwdHost");
  if (fh) fh.innerHTML = opts;
  const efh = $("fwdEditHost");
  if (efh) efh.innerHTML = opts;
}

function short(id) { return id && id.length > 8 ? id.slice(0, 8) : id; }

async function loadForwards() {
  try {
    const res = await fetch("/api/v1/forward", { credentials: "include" });
    if (!res.ok) return;
    LAST_FORWARDS = await res.json();
    // Also load HTTP proxies
    try {
      const httpRes = await fetch("/api/v1/http-proxy", { credentials: "include" });
      if (httpRes.ok) LAST_HTTP_PROXIES = await httpRes.json();
    } catch(e) {}
    renderForwards();
  } catch(e) {}
}

// 转发视图模式：list（默认）| card
let FORWARD_VIEW_MODE = "list";

// 操作图标（统一的描边 SVG，使用 currentColor 跟随文字色）
const FWD_ICONS = {
  enable:  '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg>',
  disable: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M6 6h12v12H6z"/></svg>',
  copy:    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/></svg>',
  edit:    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4z"/></svg>',
  del:     '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2m2 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/></svg>',
  open:    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6M15 3h6v6M10 14L21 3"/></svg>',
  addr:    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/></svg>',
};

function switchForwardView(mode) {
  FORWARD_VIEW_MODE = mode;
  const wrap = $("forwardViewToggle");
  if (wrap) wrap.querySelectorAll(".vt-btn").forEach(b => b.classList.toggle("active", b.dataset.view === mode));
  renderForwards();
}

// 构建单条转发的操作按钮组
function fwdActionButtons(item) {
  const toggleIcon = item.enabled ? FWD_ICONS.disable : FWD_ICONS.enable;
  const toggleLabel = item.enabled ? I18N.t("ui.disable") : I18N.t("ui.enable");
  const primary = item.type === "http"
    ? `<button class="icon-btn" title="${I18N.t("ui.open")}" onclick="openProxyUrl('${item.proxyUrl}')">${FWD_ICONS.open}</button>`
    : `<button class="icon-btn" title="${I18N.t("ui.copy_addr")}" onclick="copyText('${esc(item.listenAddr)}')">${FWD_ICONS.addr}</button>`;
  return `
    <button class="icon-btn" title="${toggleLabel}" onclick="toggleForward('${item.type}','${esc(item.id)}',${!item.enabled})">${toggleIcon}</button>
    ${primary}
    <button class="icon-btn" title="${I18N.t("ui.copy")}" onclick="copyForward('${item.type}','${esc(item.id)}')">${FWD_ICONS.copy}</button>
    <button class="icon-btn" title="${I18N.t("ui.edit")}" onclick="editForward('${item.type}','${esc(item.id)}')">${FWD_ICONS.edit}</button>
    <button class="icon-btn danger" title="${I18N.t("ui.delete")}" onclick="deleteForward('${item.type}','${esc(item.id)}')">${FWD_ICONS.del}</button>`;
}

// 将 TCP / HTTP 两条数据源统一为渲染模型
function collectForwardItems() {
  const items = [];
  (LAST_FORWARDS || []).forEach(f => {
    items.push({
      type: "tcp", id: f.id,
      enabled: f.enabled !== false,
      badge: "TCP", badgeClass: "op",
      title: `${esc(f.hostname)} → :${f.target_port}`,
      sub: `${I18N.t("ui.listen_addr")} <code class="mono">${esc(f.listen_addr)}</code> · ${f.sessions} ${I18N.t("ui.active_sessions")}`,
      listenAddr: f.listen_addr,
    });
  });
  (LAST_HTTP_PROXIES || []).forEach(p => {
    const proxyUrl = `/proxy/${encodeURIComponent(p.host_id)}/${p.target_port}/${(p.default_path || "").replace(/^\//, "")}`;
    items.push({
      type: "http", id: p.id,
      enabled: p.enabled !== false,
      badge: "HTTP", badgeClass: "sys",
      title: esc(p.name || `${p.hostname}:${p.target_port}`) + (p.is_copy ? I18N.t("forward.copy_suffix") : ""),
      sub: `${esc(p.hostname)}:${p.target_port}${p.default_path ? " · " + esc(p.default_path) : ""}`,
      proxyUrl,
    });
  });
  return items;
}

function renderForwards() {
  const list = $("forwardList");
  const empty = $("forwardEmpty");
  if (!list || !empty) return;

  const items = collectForwardItems();

  if (FORWARD_VIEW_MODE === "card") {
    list.className = "fwd-list fwd-grid";
    // Reuse the automation playbook-card structure (pb-card*) so forward cards are
    // visually identical: title + sub top-left, status top-right, divider, footer
    // with a type pill and the action buttons.
    list.innerHTML = items.map(it => `
      <div class="pb-card fwd-card ${it.enabled ? "" : "pb-off"}">
        <div class="pb-card-top">
          <div class="pb-card-title">
            <strong>${it.title}</strong>
            <span class="pb-desc">${it.sub}</span>
          </div>
          <span class="fwd-status ${it.enabled ? "on" : "off"}">${it.enabled ? I18N.t("ui.enabled") : I18N.t("ui.disabled")}</span>
        </div>
        <div class="pb-card-foot">
          <div class="pb-pills"><span class="badge ${it.badgeClass}">${it.badge}</span></div>
          <div class="fwd-actions">${fwdActionButtons(it)}</div>
        </div>
      </div>`).join("");
  } else {
    list.className = "fwd-list";
    list.innerHTML = items.map(it => `
      <div class="fwd-row ${it.enabled ? "" : "fwd-off"}">
        <span class="badge ${it.badgeClass}">${it.badge}</span>
        <div class="fwd-main">
          <div class="fwd-title">${it.title}</div>
          <div class="fwd-sub">${it.sub}</div>
        </div>
        <div class="fwd-actions">${fwdActionButtons(it)}</div>
      </div>`).join("");
  }

  empty.style.display = items.length ? "none" : "";
}

function switchFwdMode(mode) {
  FWD_MODE = mode;
  document.querySelectorAll("#fwdModeTabs .fwd-mode-tab").forEach(btn => {
    btn.classList.toggle("active", btn.dataset.fwdmode === mode);
  });
  const tcpFields = $("fwdTcpFields");
  const httpFields = $("fwdHttpFields");
  const tcpFoot = $("fwdTcpFoot");
  const httpFoot = $("fwdHttpFoot");
  if (tcpFields) tcpFields.style.display = mode === "tcp" ? "" : "none";
  if (httpFields) httpFields.style.display = mode === "http" ? "" : "none";
  if (tcpFoot) tcpFoot.style.display = mode === "tcp" ? "" : "none";
  if (httpFoot) httpFoot.style.display = mode === "http" ? "" : "none";
}

function submitForward() {
  const hostID = $("fwdHost")?.value;
  const targetPort = parseInt($("fwdTargetPort")?.value || "0");
  if (!hostID || targetPort < 1 || targetPort > 65535) {
    toast(I18N.t("valid.fill_target_port"), "err");
    return;
  }
  if (FWD_MODE === "tcp") {
    createTcpForward(hostID, targetPort);
  } else {
    openHttpProxy(hostID, targetPort);
  }
}

async function createTcpForward(hostID, targetPort) {
  const localPort = parseInt($("fwdLocalPort")?.value || "0");
  await withLoading("fwdSubmitBtn", async () => {
    try {
      const res = await fetch("/api/v1/forward", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ host_id: hostID, target_port: targetPort, local_port: localPort })
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        toast(err.error || I18N.t("toast.create_failed"), "err");
        return;
      }
      const result = await res.json();
      toast(I18N.t("toast.forward_created") + result.listen_addr, "ok");
      closeForwardModal();
      loadForwards();
    } catch(e) {
      toast(I18N.t("toast.network_error2"), "err");
    }
  });
}

function openHttpProxy(hostID, targetPort) {
  const path = $("fwdHttpPath")?.value || "";
  const baseUrl = `/proxy/${encodeURIComponent(hostID)}/${encodeURIComponent(targetPort)}/${path.replace(/^\//, "")}`;
  openProxyUrl(baseUrl);
  closeForwardModal();
}

// openProxyUrl fetches a single-use proxy auth token (which the server
// also sets as a cookie), then opens the proxy URL in a new tab. The
// cookie is automatically carried by the new tab request, avoiding the
// "unauthorized" error that occurs when the session cookie is not sent.
async function openProxyUrl(baseUrl) {
  try {
    const res = await fetch("/api/v1/proxy-token", { credentials: "include" });
    if (!res.ok) { toast(I18N.t("toast.network_error2"), "err"); return; }
    // Token cookie is now set; open the URL — backend reads cookie or pt param.
    const data = await res.json();
    const sep = baseUrl.includes("?") ? "&" : "?";
    window.open(baseUrl + sep + "pt=" + encodeURIComponent(data.token), "_blank");
  } catch(e) {
    toast(I18N.t("toast.network_error2"), "err");
  }
}

async function saveHttpProxy() {
  const hostID = $("fwdHost")?.value;
  const targetPort = parseInt($("fwdTargetPort")?.value || "0");
  const name = $("fwdHttpName")?.value || "";
  const defaultPath = $("fwdHttpPath")?.value || "";
  if (!hostID || targetPort < 1 || targetPort > 65535) {
    toast(I18N.t("valid.fill_target_port"), "err");
    return;
  }
  await withLoading("fwdHttpSaveBtn", async () => {
    try {
      const res = await fetch("/api/v1/http-proxy", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ host_id: hostID, target_port: targetPort, name, default_path: defaultPath })
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        toast(err.error || I18N.t("toast.save_failed"), "err");
        return;
      }
      toast(I18N.t("toast.saved"), "ok");
      closeForwardModal();
      loadHttpProxies();
    } catch(e) {
      toast(I18N.t("toast.network_error2"), "err");
    }
  });
}

async function loadHttpProxies() {
  try {
    const res = await fetch("/api/v1/http-proxy", { credentials: "include" });
    if (!res.ok) return;
    LAST_HTTP_PROXIES = await res.json();
    renderForwards();
  } catch(e) {}
}

// 启用 / 停用某条转发（TCP 或 HTTP）
async function toggleForward(type, id, enable) {
  const url = type === "tcp"
    ? `/api/v1/forward/${id}/toggle`
    : `/api/v1/http-proxy/${id}/toggle`;
  try {
    const res = await fetch(url, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ enabled: enable })
    });
    if (!res.ok) { toast(I18N.t("toast.toggle_failed"), "err"); return; }
    toast(enable ? I18N.t("toast.enabled") : I18N.t("toast.disabled"), "ok");
    loadForwards();
  } catch(e) {
    toast(I18N.t("toast.network_error2"), "err");
  }
}

// 复制（克隆）某条转发
async function copyForward(type, id) {
  const url = type === "tcp"
    ? `/api/v1/forward/${id}/copy`
    : `/api/v1/http-proxy/${id}/copy`;
  try {
    const res = await fetch(url, { method: "POST", credentials: "include" });
    if (!res.ok) { toast(I18N.t("toast.copy_failed"), "err"); return; }
    toast(I18N.t("toast.copied"), "ok");
    loadForwards();
  } catch(e) {
    toast(I18N.t("toast.network_error2"), "err");
  }
}

function closeForwardModal() {
  const forwardMask = $("forwardMask");
  const backdrop = $("backdrop");
  if (forwardMask) forwardMask.classList.remove("show");
  if (backdrop) backdrop.style.display = "none";
}

// 删除某条转发（统一 TCP / HTTP）
async function deleteForward(type, id) {
  if (!confirm(I18N.t("valid.confirm_delete"))) return;
  const url = type === "tcp"
    ? `/api/v1/forward/${id}`
    : `/api/v1/http-proxy/${id}`;
  try {
    const res = await fetch(url, { method: "DELETE", credentials: "include" });
    if (res.ok) {
      toast(I18N.t("toast.deleted"), "ok");
      loadForwards();
    } else {
      toast(I18N.t("toast.delete_failed"), "err");
    }
  } catch(e) {
    toast(I18N.t("toast.network_error2"), "err");
  }
}

// 打开编辑弹窗并预填数据
function editForward(type, id) {
  const item = type === "tcp"
    ? (LAST_FORWARDS || []).find(f => f.id === id)
    : (LAST_HTTP_PROXIES || []).find(p => p.id === id);
  if (!item) return;
  $("fwdEditId").value = id;
  $("fwdEditType").value = type;
  populateForwardHosts();
  $("fwdEditHost").value = item.host_id;
  $("fwdEditPort").value = item.target_port;
  if (type === "tcp") {
    $("fwdEditTcpField").style.display = "";
    $("fwdEditLocalPort").value = item.local_port || 0;
    $("fwdEditHttpNameField").style.display = "none";
    $("fwdEditHttpPathField").style.display = "none";
  } else {
    $("fwdEditTcpField").style.display = "none";
    $("fwdEditHttpNameField").style.display = "";
    $("fwdEditHttpPathField").style.display = "";
    $("fwdEditName").value = item.name || "";
    $("fwdEditPath").value = item.default_path || "";
  }
  const mask = $("fwdEditMask");
  const backdrop = $("backdrop");
  if (mask) mask.classList.add("show");
  if (backdrop) backdrop.style.display = "";
}

function closeForwardEditModal() {
  const mask = $("fwdEditMask");
  const backdrop = $("backdrop");
  if (mask) mask.classList.remove("show");
  if (backdrop) backdrop.style.display = "none";
}

// 保存编辑结果
async function saveForwardEdit() {
  const type = $("fwdEditType").value;
  const id = $("fwdEditId").value;
  const hostID = $("fwdEditHost").value;
  const targetPort = parseInt($("fwdEditPort").value || "0");
  if (!hostID || targetPort < 1 || targetPort > 65535) {
    toast(I18N.t("valid.fill_target_port"), "err");
    return;
  }
  if (type === "tcp") {
    const localPort = parseInt($("fwdEditLocalPort").value || "0");
    try {
      const res = await fetch(`/api/v1/forward/${id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ host_id: hostID, target_port: targetPort, local_port: localPort })
      });
      if (!res.ok) { toast(I18N.t("toast.edit_failed"), "err"); return; }
      toast(I18N.t("toast.edited"), "ok");
      closeForwardEditModal();
      loadForwards();
    } catch(e) { toast(I18N.t("toast.network_error2"), "err"); }
  } else {
    const name = $("fwdEditName").value || "";
    const defaultPath = $("fwdEditPath").value || "";
    // 保持当前启用状态，避免编辑后规则被意外禁用
    const cur = (LAST_HTTP_PROXIES || []).find(p => p.id === id);
    const keepEnabled = cur ? cur.enabled !== false : true;
    try {
      const res = await fetch(`/api/v1/http-proxy/${id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ host_id: hostID, target_port: targetPort, name, default_path: defaultPath, enabled: keepEnabled })
      });
      if (!res.ok) { toast(I18N.t("toast.edit_failed"), "err"); return; }
      toast(I18N.t("toast.edited"), "ok");
      closeForwardEditModal();
      loadForwards();
    } catch(e) { toast(I18N.t("toast.network_error2"), "err"); }
  }
}

// 绑定事件
safeAddEventListener("addForwardBtn", "click", () => {
  populateForwardHosts();
  switchFwdMode("tcp");
  const targetPort = $("fwdTargetPort");
  const localPort = $("fwdLocalPort");
  const httpPath = $("fwdHttpPath");
  const forwardMask = $("forwardMask");
  const backdrop = $("backdrop");
  if (targetPort) targetPort.value = "";
  if (localPort) localPort.value = "";
  if (httpPath) httpPath.value = "";
  if (forwardMask) forwardMask.classList.add("show");
  if (backdrop) backdrop.style.display = "";
});
safeAddEventListener("fwdSubmitBtn", "click", submitForward);
safeAddEventListener("fwdHttpSaveBtn", "click", saveHttpProxy);
safeAddEventListener("fwdHttpOpenBtn", "click", () => {
  const hostID = $("fwdHost")?.value;
  const targetPort = parseInt($("fwdTargetPort")?.value || "0");
  if (hostID && targetPort > 0) openHttpProxy(hostID, targetPort);
});
safeAddEventListener("fwdEditSaveBtn", "click", saveForwardEdit);
// Mode tab clicks
document.querySelectorAll("#fwdModeTabs .fwd-mode-tab").forEach(btn => {
  btn.addEventListener("click", () => switchFwdMode(btn.dataset.fwdmode));
});

// 复制文本到剪贴板
function copyText(text) {
  navigator.clipboard?.writeText(text).then(() => toast(I18N.t("toast.copied_detail") + text, "ok"));
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
updateSideClock();
setInterval(updateSideClock, 1000);
