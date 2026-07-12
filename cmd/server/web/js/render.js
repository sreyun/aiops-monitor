/* ============================================================
   AIOps Monitor · render.js — 所有 DOM 渲染函数
   依赖：core.js（需先加载）
   ============================================================ */
"use strict";

/* ============================================================
   P0-3: 渲染性能优化 — 差量更新
   ============================================================ */
let HOST_DOM_CACHE = {}; // hostID -> { element, data }
function updateHostCard(h) {
  const existing = HOST_DOM_CACHE[h.id];
  if (!existing) return false;
  const el = existing.element;
  el.classList.toggle("online", !!h.online);
  el.classList.toggle("offline", !h.online);
  const dot = el.querySelector(".dot");
  if (dot) dot.className = "dot " + (h.online ? "on" : "off");
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

/* ---------- 渲染：KPI ---------- */
function renderCards(s) {
  const cardsEl = $("cards");
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

  cardsEl.querySelectorAll(".v[data-val]").forEach(el => {
    const goto = el.closest(".card")?.getAttribute("data-goto");
    const newVal = parseInt(el.dataset.val) || 0;
    const oldVal = prevVals[goto] !== undefined ? prevVals[goto] : newVal;
    if (oldVal !== newVal) animateValue(el, oldVal, newVal, 400);
  });

  const prevCrit = prevVals["alerts:"] !== undefined ? prevVals["alerts:"] : 0;
  if (s.critical_alerts > prevCrit) {
    const critCard = cardsEl.querySelector(".card.crit[data-goto='alerts:']");
    if (critCard) {
      critCard.classList.add("card-pulse");
      setTimeout(() => critCard.classList.remove("card-pulse"), 600);
    }
  }

  const ob = $("onboarding");
  if (ob) ob.style.display = s.total_hosts === 0 ? "block" : "none";
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
  const badge = $("statsHealthBadge");
  if (badge) badge.textContent = I18N.t("section.online_rate") + " " + rate + "%";
}

/* ---------- 渲染：告警 / 事件 ---------- */
const ALERT_TYPES = [
  {key:"", label:I18N.t("ui.all")}, {key:"cpu", label:"CPU"}, {key:"memory", label:I18N.t("ui.memory")},
  {key:"disk", label:I18N.t("ui.disk")}, {key:"gpu", label:"GPU"}, {key:"load", label:I18N.t("ui.load")},
  {key:"offline", label:I18N.t("ui.offline_status")}, {key:"check", label:I18N.t("ui.probe")}
];
const DIFF_GRACE_MS = 5000;
function diffUpdateList(container, items, rowFn, keyFn, emptyHTML) {
  if (!container) return;
  const now = Date.now();
  container.querySelectorAll(".row-leaving").forEach(el => {
    if (parseInt(el.dataset.leavingAt || "0") <= now) el.remove();
  });
  if (!items.length) {
    const leavingCount = container.querySelectorAll(".row-leaving").length;
    if (!leavingCount) {
      if (container.dataset.sig !== "empty") {
        container.innerHTML = emptyHTML;
        container.dataset.sig = "empty";
      }
    }
    return;
  }
  const sig = items.map(keyFn).join("\n");
  if (container.dataset.sig === sig) return;
  container.dataset.sig = sig;
  const existing = container.querySelectorAll("[data-key]");
  if (!existing.length) {
    container.innerHTML = items.map(rowFn).join("");
    return;
  }
  const oldMap = {};
  existing.forEach(el => { oldMap[el.dataset.key] = el; });
  const newKeys = items.map(keyFn);
  const newKeySet = {};
  newKeys.forEach(k => { newKeySet[k] = true; });
  existing.forEach(el => {
    if (!newKeySet[el.dataset.key] && !el.classList.contains("row-leaving")) {
      el.classList.add("row-leaving");
      el.dataset.leavingAt = String(now + DIFF_GRACE_MS);
    }
  });
  let refNode = null;
  for (let i = items.length - 1; i >= 0; i--) {
    const key = newKeys[i];
    let el = oldMap[key];
    if (el) {
      el.classList.remove("row-leaving", "row-enter");
      delete el.dataset.leavingAt;
    } else {
      el = document.createElement("div");
      el.innerHTML = rowFn(items[i]).trim();
      el = el.firstChild;
      el.classList.add("row-enter");
    }
    if (refNode && el.nextSibling !== refNode) {
      container.insertBefore(el, refNode);
    } else if (!refNode && el !== container.firstChild) {
      container.insertBefore(el, container.firstChild);
    }
    refNode = el;
  }
}
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
  const alertKey = a => {
    const type = a.type || "";
    const scope = a.scope || "";
    const id = a.host_id || "";
    if (type || scope || id) {
      return `${type}|${scope}|${a.hostname}|${id}`;
    }
    return `${a.hostname}|${a.message}|${a.level}`;
  };
  const row = a => {
    const dur = a.since ? I18N.t("section.duration") + " " + fmtDur(now - a.since) : "";
    const ipStr = a.ip ? `<span class="alert-ip mono">${esc(a.ip)}</span>` : "";
    const timeStr = a.timestamp ? `<span class="alert-time mono">${fmtDateTime(a.timestamp)}</span>` : "";
    const durSpan = a.since
      ? `<span class="src alert-dur" data-since="${a.since}" title="${I18N.t("section.first_fired")} ${fmtDateTime(a.since)}">${dur}</span>`
      : "";
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
  let filtered = alerts;
  if (ALERT_TYPE) filtered = filtered.filter(a => a.type === ALERT_TYPE);
  if (ALERT_SEARCH) filtered = filtered.filter(a => {
    const hay = ((a.hostname || "") + " " + (a.ip || "") + " " + (a.message || "")).toLowerCase();
    return hay.includes(ALERT_SEARCH.toLowerCase());
  });
  const empty = `<div class="empty-line">✅ ${I18N.t("empty.no_alerts")}</div>`;
  const filterEmpty = `<div class="empty-line">${I18N.t("empty.no_alerts_filtered")}</div>`;
  $("alerts").innerHTML = filtered.length ? filtered.map(row).join("") : (n ? filterEmpty : empty);
  diffUpdateList($("ovAlerts"), alerts.slice(0, 6), row, alertKey, empty);
  refreshAlertRowTimes($("ovAlerts"), now);
}

/* ---------- 概览：资源 TOP10 排行榜 ---------- */
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
  const hasGPU = live.some(h => { const gs = (h.latest.gpus || []); return gs.length > 0; });
  const diskMax = m => { const d = m.disks || []; return d.length ? Math.max(...d.map(x => x.percent)) : (m.disk_percent || 0); };
  const gpuMax = m => { const g = m.gpus || []; return g.length ? Math.max(...g.map(x => x.util_percent || 0)) : 0; };
  const netTotal = m => (m.net_sent_rate || 0) + (m.net_recv_rate || 0);
  const iopsTotal = m => (m.disk_read_iops || 0) + (m.disk_write_iops || 0);
  const gpuLive = live.filter(h => {
    const gs = (h.latest.gpus || []);
    return gs.length > 0;
  });
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
  const checksHtml = checkTopPanels();
  if (checksHtml) {
    html += `<div class="top-checks-panel">
      <div class="top-checks-title">${I18N.t("section.custom_monitor")}</div>
      ${checksHtml}
    </div>`;
  }
  el.innerHTML = html;
}
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

function applyLogFilters(items) {
  let filtered = items;
  if (LOG_KIND) filtered = filtered.filter(e => e.kind === LOG_KIND);
  if (LOG_LEVEL && LOG_LEVEL !== "all") filtered = filtered.filter(e => e.level === LOG_LEVEL);
  if (LOG_TIME_RANGE && LOG_TIME_RANGE !== "all") {
    const now = Math.floor(Date.now() / 1000);
    const hours = parseInt(LOG_TIME_RANGE);
    filtered = filtered.filter(e => (now - e.timestamp) <= hours * 3600);
  }
  return filtered.filter(e => e.actor !== I18N.t("notify.alert_engine"));
}

function exportLogsCSV() {
  const rows = applyLogFilters(LAST_LOG);
  if (!rows.length) { toast(I18N.t("empty.no_log_export"), "err"); return; }
  const escCsv = v => `"${String(v == null ? "" : v).replace(/"/g, '""')}"`;
  const lines = [I18N.t("section.csv_header")];
  rows.forEach(e => lines.push([fmtDateTime(e.timestamp), translateLogKind(e.kind), translateLogLevel(e.level), e.actor || "", e.host || "", e.message].map(escCsv).join(",")));
  const blob = new Blob(["\uFEFF" + lines.join("\r\n")], { type: "text/csv;charset=utf-8" });
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

function hostRow(h) {
  const m = h.latest || {};
  const disks = (Array.isArray(m.disks) ? m.disks : []).filter(d => !isSystemMount(d.path));
  const diskMax = disks.length ? Math.max(...disks.map(d => d.percent)) : (m.disk_percent || 0);
  const gpus = Array.isArray(m.gpus) ? m.gpus : [];
  const gpuMax = gpus.length ? Math.max(...gpus.map(g => g.util_percent || 0)) : null;
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
  const cats = [...new Set(hosts.map(h => h.category || I18N.t("section.uncategorized")))].sort();
  renderCatDropdown(cats);
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
  const byCat = {};
  pageHosts.forEach(h => { const c = h.category || I18N.t("section.uncategorized"); (byCat[c] = byCat[c] || []).push(h); });
  const render = isList ? hostRow : hostCard;
  const wrapCls = isList ? "host-list" : "grid";
  const newKey = pageHosts.map(h => h.id).join(",") + "|" + HOST_VIEW + "|" + HOST_PAGE + "|" + Object.keys(byCat).sort().join(",");
  if (LAST_RENDER_KEY === newKey && Object.keys(HOST_DOM_CACHE).length > 0) {
    pageHosts.forEach(h => updateHostCard(h));
    renderPager(pages, shown.length);
    return;
  }
  LAST_RENDER_KEY = newKey;
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

/* ---------- 自定义监控 ---------- */
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
function splitHostPort(t) {
  t = String(t || "");
  const i = t.lastIndexOf(":");
  if (i > 0) return { host: t.slice(0, i), port: t.slice(i + 1) };
  return { host: t, port: "" };
}
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
  if (check && check.type === "process") {
    const idx = check.target.indexOf("/");
    $("ckTarget").value = idx > 0 ? check.target.slice(idx + 1) : check.target;
  } else {
    $("ckTarget").value = check ? check.target : "";
  }
  $("ckInterval").value = check ? check.interval_sec : 30;
  $("ckLevel").value = check ? check.level : "critical";
  $("ckEnabled").checked = check ? check.enabled : true;
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

// 导出到 AIOps 命名空间
Object.assign(window.AIOps, {
  HOST_DOM_CACHE, updateHostCard, buildHostCache,
  renderCards, renderStatsHealth,
  ALERT_TYPES, DIFF_GRACE_MS, diffUpdateList, refreshAlertRowTimes, renderAlerts,
  renderTop, checkTopPanels, checkTopPanel,
  applyLogFilters, exportLogsCSV, renderLog, renderLogPager, setLogPageSize,
  hostCard, hostRow, renderHosts, renderPager,
  delHost, editCategory,
  checkTargetDisplay, splitHostPort, splitProcessTarget, cdItem, renderChecks,
  loadChecks, loadHostsMeta, updateCkTargetLabel, openCheckModal, populateHostSelect, saveCheck, delCheck
});