// snmp.js — SNMP 网络设备面板（轮询接口状态/流量 + Trap 事件）
// Loaded as part of the unified app.js bundle. 复用 netflow 的 nf-* 样式与全局 helper。

(function() {
"use strict";

let snCurrentHost = "";
let snHostQuery = "";
let snShowOffline = false;
let snTab = "devices"; // devices | traps
let snSearchT = null;
let snDevices = [];    // 最近一次加载的设备快照
let snTraps = [];      // 最近一次加载的 trap
// 「只列有网络设备的主机」：从 /api/v1/snmp/hosts 拉有 SNMP 设备快照或 trap 的主机，
// 按设备数降序。null=未加载。刷新/进入视图时重新拉。
let snSNMPHosts = null;

function snFmtBps(bps) {
  bps = bps || 0;
  const u = ["bps", "Kbps", "Mbps", "Gbps", "Tbps"];
  let i = 0;
  while (bps >= 1000 && i < u.length - 1) { bps /= 1000; i++; }
  return bps.toFixed(1) + " " + u[i];
}
function snFmtSpeed(bps) {
  if (!bps) return "-";
  const u = ["bps", "Kbps", "Mbps", "Gbps", "Tbps"];
  let i = 0, v = bps;
  while (v >= 1000 && i < u.length - 1) { v /= 1000; i++; }
  return Math.round(v) + " " + u[i];
}
// snUtilCell：利用率迷你进度条 + 百分比（≥80 红 / ≥50 黄 / 其余蓝）。
function snUtilCell(u) {
  const col = u >= 80 ? "var(--crit)" : u >= 50 ? "var(--warn)" : "var(--accent)";
  return `<div class="sn-util"><div class="sn-util-bar"><div class="sn-util-fill" style="width:${Math.min(100, u)}%;background:${col}"></div></div><span class="sn-util-v">${u.toFixed(0)}%</span></div>`;
}

function renderSNMPPanel() {
  const container = $("snmpPanel");
  if (!container) return;

  // 先拉「有网络设备数据的主机」，再渲染；避免把无 SNMP 数据的主机塞进下拉。
  if (snSNMPHosts === null) {
    container.innerHTML = `<div class="loading-dots">${I18N.t("common.loading") || "加载中..."}</div>`;
    fetch(`/api/v1/snmp/hosts`, { credentials: "same-origin" })
      .then(r => r.json())
      .then(d => { snSNMPHosts = d.hosts || []; renderSNMPPanel(); })
      .catch(() => { snSNMPHosts = []; renderSNMPPanel(); });
    return;
  }

  const q = snHostQuery.trim().toLowerCase();
  // 有设备的主机只带 host_id，用 _cachedHosts 补 hostname/ip 展示。
  const nameMap = {};
  (window._cachedHosts || []).forEach(h => { nameMap[h.id] = h; });

  let html = `<div class="nf-toolbar">`;
  html += `<input type="search" id="snHostSearch" class="nf-input" value="${esc(snHostQuery)}"
    placeholder="${esc(I18N.t("netflow.search_ph") || "搜索主机")}">`;
  html += `<select id="snHostSelect" class="nf-select">`;
  let shown = 0;
  snSNMPHosts.forEach(sh => {
    const h = nameMap[sh.host_id] || {};
    const name = h.hostname || sh.host_id;
    const hay = `${name} ${sh.host_id} ${h.ip || ""}`.toLowerCase();
    if (q && !q.split(/\s+/).every(w => hay.includes(w))) return;
    shown++;
    const sel = sh.host_id === snCurrentHost ? " selected" : "";
    const dev = Number(sh.devices) || 0;
    // 下拉直接标出设备数，一眼看出哪些主机纳管了网络设备
    html += `<option value="${esc(sh.host_id)}"${sel}>${esc(name)}${dev ? " · " + dev + " " + (I18N.t("snmp.dev_unit") || "设备") : ""}</option>`;
  });
  html += `</select>`;
  // Tab: 设备 / Trap
  html += `<span class="sn-tabs">`;
  html += `<button class="nf-btn${snTab === "devices" ? " sn-active" : ""}" data-snact="tab-devices">${esc(I18N.t("snmp.tab_devices") || "设备与接口")}</button>`;
  html += `<button class="nf-btn${snTab === "traps" ? " sn-active" : ""}" data-snact="tab-traps">${esc(I18N.t("snmp.tab_traps") || "Trap 事件")}</button>`;
  html += `</span>`;
  html += `<button class="nf-btn" data-snact="refresh">${I18N.t("common.refresh") || "刷新"}</button>`;
  html += `<button class="nf-btn" data-snact="ai">${esc(I18N.t("snmp.ai_diagnose") || "🤖 AI 诊断")}</button>`;
  html += `</div>`;

  if (snSNMPHosts.length === 0) {
    container.innerHTML = html + `<div class="empty-state">${I18N.t("snmp.no_snmp_hosts") || "暂无纳管网络设备的主机（未配置 SNMP 轮询/Trap，或 Agent 未上报）"}</div>`;
    snBindToolbar();
    return;
  }
  if (shown === 0) {
    container.innerHTML = html + `<div class="empty-state">${I18N.t("empty.no_host_match2") || "没有匹配的主机"}</div>`;
    snBindToolbar();
    return;
  }

  html += `<div id="snContent" class="nf-content"><div id="snBody"></div></div>`;
  container.innerHTML = html;

  const sel = $("snHostSelect");
  if (sel && sel.options.length > 0) {
    if (![...sel.options].some(o => o.value === snCurrentHost)) snCurrentHost = sel.options[0].value;
    sel.value = snCurrentHost;
  }
  snBindToolbar();
  if (snCurrentHost) loadSNMPData();
}

function snBindToolbar() {
  const sel = $("snHostSelect");
  sel && sel.addEventListener("change", function() { snCurrentHost = this.value; loadSNMPData(); });
  const off = $("snShowOffline");
  off && off.addEventListener("change", function() { snShowOffline = this.checked; renderSNMPPanel(); });
  const search = $("snHostSearch");
  if (search) {
    search.addEventListener("input", function() {
      clearTimeout(snSearchT);
      const v = this.value;
      snSearchT = setTimeout(() => {
        snHostQuery = v;
        renderSNMPPanel();
        const s = $("snHostSearch");
        if (s) { s.focus(); s.setSelectionRange(s.value.length, s.value.length); }
      }, 200);
    });
  }
}

window.loadSNMPData = function() {
  const host = snCurrentHost || ($("snHostSelect") || {}).value;
  if (!host) return;
  const body = $("snBody");
  if (body) body.innerHTML = `<div class="loading-dots">${I18N.t("common.loading") || "加载中..."}</div>`;

  if (snTab === "traps") {
    fetch(`/api/v1/snmp/traps?host=${encodeURIComponent(host)}&limit=100`, { credentials: "same-origin" })
      .then(r => r.json())
      .then(data => { snTraps = data.traps || []; renderSNTraps(body, snTraps); })
      .catch(() => { if (body) body.innerHTML = `<div class="empty-state">${I18N.t("netflow.load_error") || "加载失败"}</div>`; });
  } else {
    fetch(`/api/v1/snmp/list?host=${encodeURIComponent(host)}`, { credentials: "same-origin" })
      .then(r => r.json())
      .then(data => { snDevices = data.devices || []; renderSNDevices(body, snDevices); })
      .catch(() => { if (body) body.innerHTML = `<div class="empty-state">${I18N.t("netflow.load_error") || "加载失败"}</div>`; });
  }
};

function renderSNDevices(container, devices) {
  if (!container) return;
  if (devices.length === 0) {
    container.innerHTML = `<div class="empty-state">${I18N.t("snmp.no_devices") || "暂无 SNMP 设备数据（未配置轮询目标或 Agent 未上报）"}</div>`;
    return;
  }
  let html = "";
  devices.forEach(d => {
    const snap = d.snapshot || {};
    const sys = snap.system || {};
    const ifs = snap.interfaces || [];
    const up = ifs.filter(i => i.oper_up).length, down = ifs.length - up;
    const reachable = d.reachable !== false;
    html += `<div class="sn-dev">`;
    html += `<div class="sn-dev-head">
      <div class="sn-dev-title">${esc(d.device_name || "设备")}${reachable ? `<span class="sn-pill ok">${I18N.t("snmp.reachable") || "可达"}</span>` : `<span class="sn-pill bad">${I18N.t("snmp.unreachable") || "不可达"}</span>`}</div>
      <div class="sn-dev-sub">${esc(d.device_ip || "")}${sys.name ? " · " + esc(sys.name) : ""}</div>
    </div>`;
    if (snap.error) {
      html += `<div class="empty-state">${esc(I18N.t("snmp.poll_error") || "采集失败")}: ${esc(snap.error)}</div></div>`;
      return;
    }
    html += `<div class="sn-stats">
      <span class="sn-stat"><b>${ifs.length}</b>${I18N.t("snmp.interfaces") || "接口"}</span>
      <span class="sn-stat ok"><b>${up}</b>UP</span>
      <span class="sn-stat ${down > 0 ? "bad" : ""}"><b>${down}</b>DOWN</span>
      <span class="sn-stat"><b>${snFmtUptime(sys.uptime_sec)}</b>${I18N.t("snmp.uptime") || "运行"}</span>
    </div>`;
    html += `<div class="nf-table-wrap"><table class="nf-flow-table"><thead><tr>`;
    ["snmp.if_name:接口", "snmp.if_status:状态", "snmp.if_speed:速率", "snmp.if_in:入向", "snmp.if_out:出向", "snmp.if_util:利用率", "snmp.if_err:错误/丢包"].forEach(kv => {
      const [k, fb] = kv.split(":");
      html += `<th>${esc(I18N.t(k) || fb)}</th>`;
    });
    html += `</tr></thead><tbody>`;
    // 异常接口排前面
    const sorted = ifs.slice().sort((a, b) => snIfBad(b) - snIfBad(a));
    sorted.forEach(i => {
      const util = Math.max(i.in_util_percent || 0, i.out_util_percent || 0);
      const err = (i.in_err_pps || 0) + (i.out_err_pps || 0) + (i.in_discard_pps || 0) + (i.out_discard_pps || 0);
      const isDown = i.admin_status === 1 && !i.oper_up;
      const rowCls = isDown ? " sn-row-crit" : (util >= 80 ? " sn-row-warn" : "");
      html += `<tr class="${rowCls.trim()}">`;
      html += `<td>${esc(i.name || ("if" + i.index))}${i.alias ? ` <span class="sn-dim">${esc(i.alias)}</span>` : ""}</td>`;
      html += `<td>${i.oper_up ? `<span class="sn-badge sn-up">UP</span>` : `<span class="sn-badge sn-down">DOWN</span>`}</td>`;
      html += `<td class="nf-mono">${snFmtSpeed(i.speed_bps)}</td>`;
      html += `<td class="nf-num">${i.rate_valid ? snFmtBps(i.in_bps) : "-"}</td>`;
      html += `<td class="nf-num">${i.rate_valid ? snFmtBps(i.out_bps) : "-"}</td>`;
      html += `<td>${i.rate_valid ? snUtilCell(util) : "-"}</td>`;
      html += `<td class="nf-num">${i.rate_valid ? err.toFixed(1) : "-"}</td>`;
      html += `</tr>`;
    });
    html += `</tbody></table></div></div>`;
  });
  container.innerHTML = html;
}

function snIfBad(i) {
  if (i.admin_status === 1 && !i.oper_up) return 3;
  const util = Math.max(i.in_util_percent || 0, i.out_util_percent || 0);
  if (util >= 80) return 2;
  if (((i.in_err_pps || 0) + (i.out_err_pps || 0)) > 1) return 1;
  return 0;
}

function renderSNTraps(container, traps) {
  if (!container) return;
  if (traps.length === 0) {
    container.innerHTML = `<div class="empty-state">${I18N.t("snmp.no_traps") || "暂无 Trap 事件"}</div>`;
    return;
  }
  let html = `<div class="nf-section"><h3>${I18N.t("snmp.tab_traps") || "Trap 事件"}</h3>`;
  html += `<div class="nf-table-wrap"><table class="nf-flow-table"><thead><tr>`;
  ["snmp.trap_time:时间", "snmp.trap_severity:级别", "snmp.trap_source:来源", "snmp.trap_oid:Trap OID"].forEach(kv => {
    const [k, fb] = kv.split(":");
    html += `<th>${esc(I18N.t(k) || fb)}</th>`;
  });
  html += `</tr></thead><tbody>`;
  traps.forEach(t => {
    const sev = t.severity || "info";
    const sevCls = sev === "critical" ? "sn-down" : (sev === "warning" ? "sn-warn" : "sn-up");
    html += `<tr>`;
    html += `<td>${esc(snFmtTime(t.received_at))}</td>`;
    html += `<td><span class="sn-badge ${sevCls}">${esc(sev)}</span></td>`;
    html += `<td>${esc(t.source_ip || "")}</td>`;
    html += `<td class="sn-oid">${esc(t.trap_oid || "")}</td>`;
    html += `</tr>`;
  });
  html += `</tbody></table></div></div>`;
  container.innerHTML = html;
}

function snFmtUptime(sec) {
  sec = sec || 0;
  if (sec <= 0) return "-";
  const d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}
function snFmtTime(t) {
  if (!t) return "";
  try { return new Date(t).toLocaleString(); } catch (e) { return String(t); }
}

// AI 诊断：把当前设备/Trap 数据整理成文本塞进 openAIAssist 的 context。
function snAIDiagnose() {
  const host = snCurrentHost || ($("snHostSelect") || {}).value;
  if (!host) return;
  if (snTab === "traps") {
    if (!snTraps.length) { alert(I18N.t("snmp.no_traps") || "暂无 Trap 事件"); return; }
    let ctx = `主机 ${host} 最近 SNMP Trap 事件（${snTraps.length} 条）:\n`;
    snTraps.slice(0, 80).forEach((t, i) => {
      ctx += `${i + 1}. [${t.severity}] 来自 ${t.source_ip} trapOID=${t.trap_oid} @ ${snFmtTime(t.received_at)}\n`;
    });
    openAIAssist({ task: "trap_diagnosis", title: I18N.t("snmp.ai_trap_title") || "🤖 AI Trap 诊断", mode: "analyze", context: ctx.slice(0, 14000) });
  } else {
    if (!snDevices.length) { alert(I18N.t("snmp.no_devices") || "暂无设备数据"); return; }
    let ctx = snDevicesToText(snDevices);
    openAIAssist({ task: "snmp_diagnosis", title: I18N.t("snmp.ai_dev_title") || "🤖 AI 网络设备诊断", mode: "analyze", context: ctx.slice(0, 14000) });
  }
}

function snDevicesToText(devices) {
  let out = `主机 SNMP 网络设备快照（${devices.length} 台）:\n`;
  devices.forEach(d => {
    const snap = d.snapshot || {}, sys = snap.system || {}, ifs = snap.interfaces || [];
    if (snap.error) { out += `- ${d.device_name}（${d.device_ip}）采集失败: ${snap.error}\n`; return; }
    const up = ifs.filter(i => i.oper_up).length;
    out += `- ${d.device_name}（${d.device_ip}）${sys.name || ""} 接口 ${ifs.length}（up ${up}/down ${ifs.length - up}）运行 ${snFmtUptime(sys.uptime_sec)}\n`;
    ifs.forEach(i => {
      const util = Math.max(i.in_util_percent || 0, i.out_util_percent || 0);
      const err = (i.in_err_pps || 0) + (i.out_err_pps || 0) + (i.in_discard_pps || 0) + (i.out_discard_pps || 0);
      const bad = snIfBad(i);
      if (bad > 0) {
        out += `    ${i.name}: ${i.oper_up ? "UP" : "DOWN"}, 利用率 ${util.toFixed(0)}%, in ${snFmtBps(i.in_bps)}, out ${snFmtBps(i.out_bps)}, 错误/丢包 ${err.toFixed(1)}pps\n`;
      }
    });
  });
  return out;
}

// 事件委托（CSP: script-src 'self'，内联 onclick 会被拦）。
safeAddEventListener("snmpPanel", "click", e => {
  const b = e.target.closest("[data-snact]");
  if (!b) return;
  const act = b.dataset.snact;
  // 刷新：连「有网络设备的主机」列表一起重拉（否则新纳管的设备/主机不会出现在下拉里）。
  if (act === "refresh") { snSNMPHosts = null; renderSNMPPanel(); }
  else if (act === "ai") snAIDiagnose();
  else if (act === "tab-devices") { snTab = "devices"; renderSNMPPanel(); }
  else if (act === "tab-traps") { snTab = "traps"; renderSNMPPanel(); }
});

if (typeof window._pageRenderers === "undefined") window._pageRenderers = {};
window._pageRenderers.snmp = renderSNMPPanel;

})();
