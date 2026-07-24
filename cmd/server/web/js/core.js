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

/* ===== 树折叠：硬件/虚拟机等「左树 + 右详情」布局，一键收起左树给右侧内容腾空间 =====
   约定：容器加 .tree-wrap，左树加 .tree-pane，中间放一个 [data-tree-toggle="<存储键>"] 把手。
   折叠态记忆到 localStorage，跨视图/刷新保持。样式与点击逻辑集中在此，各视图只需按约定出 DOM。*/
(function(){
  var st = document.createElement("style");
  st.textContent =
    ".tree-wrap{position:relative}" +
    ".tree-toggle-btn{flex:0 0 16px;align-self:stretch;min-height:120px;border:1px solid var(--line);" +
      "background:var(--panel);border-radius:8px;cursor:pointer;color:var(--muted);display:flex;" +
      "align-items:center;justify-content:center;padding:0;font-size:13px;line-height:1;user-select:none;" +
      "transition:background .15s,color .15s}" +
    ".tree-toggle-btn:hover{color:var(--text);background:rgba(127,127,127,.12)}" +
    ".tree-wrap.tree-collapsed .tree-pane{display:none}" +
    // 窄屏单列布局折叠意义不大：隐藏把手并强制展开，避免出现难看的横条。
    "@media(max-width:960px){.tree-toggle-btn{display:none}.tree-wrap.tree-collapsed .tree-pane{display:block}}";
  (document.head || document.documentElement).appendChild(st);

  document.addEventListener("click", function(e){
    var btn = e.target && e.target.closest ? e.target.closest("[data-tree-toggle]") : null;
    if (!btn) return;
    var wrap = btn.closest(".tree-wrap");
    if (!wrap) return;
    var collapsed = wrap.classList.toggle("tree-collapsed");
    btn.textContent = collapsed ? "›" : "‹";
    btn.setAttribute("aria-expanded", collapsed ? "false" : "true");
    try { localStorage.setItem(btn.getAttribute("data-tree-toggle"), collapsed ? "1" : "0"); } catch(err){}
  });

  // 各视图渲染时读初始折叠态（避免首帧闪烁）。
  window.treeCollapsed = function(key){
    try { return localStorage.getItem(key) === "1"; } catch(err){ return false; }
  };
})();

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
let CUR_CATS = [];    // legacy（树筛选用 CUR_FOLDER）
let CUR_FOLDER = "";  // ""=全部, "__ungrouped__"=未分组, else folder id
try { CUR_FOLDER = localStorage.getItem("aiops_host_folder") || ""; } catch (e) {}
let HOST_FOLDERS = { folders: [], assign: {}, paths: {}, counts: {} };
let HOST_TREE_COLLAPSED = new Set();
try {
  const _htc = localStorage.getItem("aiops_host_tree_collapsed");
  if (_htc) JSON.parse(_htc).forEach(id => HOST_TREE_COLLAPSED.add(id));
} catch (e) {}
let LAST_HOSTS = [];  // 最近一次主机数据（供筛选切换时本地重渲染）
let LOG_KIND = "";    // 日志类型筛选（操作/系统/插件）
let LOG_LEVEL = "";   // 日志级别筛选
let LOG_SEARCH = ""; // 审计日志关键字搜索（内容/操作者/主机）
let LOG_TIME_RANGE = "all"; // 日志时间范围
let CHECK_TYPE = "all"; // 监控类型筛选
let HOST_SORT = "name"; // 主机排序方式
let LAST_LOG = [];    // 最近一次日志数据
let HOST_SEARCH = ""; // 主机搜索关键词
let HOST_FILTER = "all"; // 主机状态筛选 all|online|offline
let HOST_PAGE = 1;    // 主机分页当前页
const HOST_PAGE_SIZE = 9;
let LAST_CHECKS = []; // 最近一次自定义监控数据
let CHECK_SEARCH = "";   // 监控（拨测）搜索关键字
let PB_SEARCH = "";      // 编排（剧本）搜索关键字
let FWD_SEARCH = "";     // 转发搜索关键字
let LAST_PLAYBOOKS = []; // 最近一次剧本数据（供搜索就地过滤，避免每次输入都重新拉取）
let HOST_META = [];   // 主机元数据（id + hostname）用于进程监控
let DEFAULT_EMPTY = null;
let APP_STARTED = false;
let LOG_PAGE = 1;     // 日志分页当前页
let LOG_PAGE_SIZE = 50; // 日志每页条数（10/30/50/100）
let CHECK_VIEW = "pill"; // 自定义监控视图：pill(卡片,默认) | list(列表)
let HOST_VIEW = "card";  // 主机视图：card | list
let TERMINAL_ENABLED = true; // 服务端是否开启远程终端
let DESKTOP_ENABLED = true;  // 远程桌面（依赖端口转发）
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
  if (k === "terminal") return "终端";
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
  if (s === "success") return I18N.t("exec.status.success");
  if (s === "skipped") return I18N.t("ui.skipped", "已跳过");
  if (s === "rollback_success") return I18N.t("exec.step.rollback_success", "回滚成功");
  if (s === "rollback_failed") return I18N.t("exec.step.rollback_failed", "回滚失败");
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

// ---- 明细表通用分页（客户端）----
// tblClampPage：把页码夹到 [1, 总页数]。
function tblClampPage(page, total, size) { return Math.min(Math.max(1, page || 1), Math.max(1, Math.ceil((total || 0) / (size || 20)))); }
// tblPager：返回分页控件 HTML；点击「上一页/下一页」和切换「每页条数」由各表用 [data-pg] 事件委托处理。
function tblPager(total, page, size) {
  const pages = Math.max(1, Math.ceil((total || 0) / size));
  page = Math.min(Math.max(1, page), pages);
  const from = total ? (page - 1) * size + 1 : 0, to = Math.min(page * size, total);
  return `<div class="tbl-pager">
    <span class="tbl-pager-info">共 ${total} 条 · ${from}–${to}</span>
    <span class="tbl-pager-spacer"></span>
    <div class="select-wrap sm"><select class="tbl-pager-size" data-pg="size">${[10, 20, 50, 100].map(n => `<option value="${n}"${n === size ? " selected" : ""}>${n} 条/页</option>`).join("")}</select></div>
    <button class="tbl-pager-btn" data-pg="prev"${page <= 1 ? " disabled" : ""}>‹</button>
    <span class="tbl-pager-cur">${page} / ${pages}</span>
    <button class="tbl-pager-btn" data-pg="next"${page >= pages ? " disabled" : ""}>›</button>
  </div>`;
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
  // 更新指标数值（文本与 hostCard 保持一致，避免差量更新丢失核数/容量等信息）
  const m = h.latest || {};
  const patch = (key, pct, detail) => {
    const vEl = el.querySelector(`[data-metric=${key}]`);
    if (vEl) vEl.textContent = detail;
    const bEl = el.querySelector(`[data-bar=${key}]`);
    if (bEl) { bEl.style.width = Math.max(0, Math.min(pct || 0, 100)) + "%"; bEl.style.background = usageColor(pct); }
  };
  if (m.cpu_percent !== undefined) {
    patch("cpu", m.cpu_percent, (m.cpu_percent || 0).toFixed(1) + "% · " + (m.cpu_cores || 0) + I18N.t("ui.cores"));
  }
  if (m.mem_percent !== undefined) {
    patch("mem", m.mem_percent, (m.mem_percent || 0).toFixed(1) + "% · " + fmtGB(m.mem_used || 0) + "/" + fmtGB(m.mem_total || 0) + I18N.t("unit.gb"));
  }
  if (m.disk_percent !== undefined) {
    patch("disk", m.disk_percent, (m.disk_percent || 0).toFixed(1) + "% · " + fmtGB(m.disk_used || 0) + "/" + fmtGB(m.disk_total || 0) + I18N.t("unit.gb"));
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

function bar(label, percent, detail, key) {
  const p = Math.max(0, Math.min(percent || 0, 100));
  // key（cpu/mem/disk）用于差量更新时定位数值与进度条，避免每轮询全量重建主机卡片。
  const vAttr = key ? ` data-metric="${key}"` : "";
  const bAttr = key ? ` data-bar="${key}"` : "";
  return `<div class="metric"><div class="row"><span class="label">${label}</span><span class="val mono"${vAttr}>${detail}</span></div>
    <div class="bar"><div class="fill"${bAttr} style="width:${p}%;background:${usageColor(percent)}"></div></div></div>`;
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
