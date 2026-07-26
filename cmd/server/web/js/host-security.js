// host-security.js — 安全中心 · 主机安全
(function () {
"use strict";

let hsSummary = [];
let hsScans = [];
let hsHosts = [];
let hsSelected = null;
let hsBusy = false;
let hsPollTimer = null;
let hsCfg = null;
let hsShowCfg = false;
let hsPendingFilter = null; // { level: open|critical|high } from security overview

const hsT = (k, fb) => I18N.t(k, fb);
function hsEsc(s) { return typeof esc === "function" ? esc(String(s ?? "")) : String(s ?? ""); }

function hsLevelLabel(level) {
  const m = {
    crit: hsT("hs.level_crit", "危急"), critical: hsT("hs.level_crit", "危急"),
    high: hsT("hs.level_high", "高危"), medium: hsT("hs.level_medium", "中危"),
    low: hsT("hs.level_low", "低危"), info: hsT("hs.level_info", "信息"),
  };
  return m[String(level || "").toLowerCase()] || (level || "—");
}
function hsLevelBadge(level) {
  const m = { crit: "crit", critical: "crit", high: "high", medium: "warn", low: "info", info: "info" };
  const cls = m[String(level || "").toLowerCase()] || "info";
  return `<span class="badge ${cls}">${hsEsc(hsLevelLabel(level))}</span>`;
}
function hsStatusLabel(st) {
  const m = {
    running: hsT("hs.status_running", "进行中"),
    completed: hsT("hs.status_completed", "已完成"),
    failed: hsT("hs.status_failed", "失败"),
  };
  return m[st] || st || "—";
}
function hsStatusBadge(st) {
  const m = { running: "info", completed: "ok", failed: "crit" };
  return `<span class="badge ${m[st] || "info"}">${hsEsc(hsStatusLabel(st))}</span>`;
}
function hsClamLabel(v) {
  const m = {
    available: hsT("hs.clam_available", "已启用"),
    unavailable: hsT("hs.clam_unavailable", "未检测到"),
    disabled: hsT("hs.clam_disabled", "已关闭"),
    error: hsT("hs.clam_error", "异常"),
  };
  return m[v] || v || "—";
}
function hsFwLabel(v) {
  const m = {
    on: hsT("hs.fw_on", "已开启"),
    off: hsT("hs.fw_off", "已关闭"),
    partial: hsT("hs.fw_partial", "部分关闭"),
    unknown: hsT("hs.fw_unknown", "未知"),
  };
  return m[String(v || "").toLowerCase()] || (v ? String(v) : hsT("hs.fw_unknown", "未知"));
}
function hsFwBadge(row, opts) {
  const st = String((row && row.firewall) || "").toLowerCase();
  if (!st) return `<span class="muted">${hsEsc(hsT("hs.ports_unknown", "需重新扫描"))}</span>`;
  const cls = { on: "ok", off: "crit", partial: "high", unknown: "info" }[st] || "info";
  const eng = (row && row.firewall_engine) || "";
  const tip = [hsFwLabel(st) + (eng ? ` · ${eng}` : ""), (row && row.firewall_detail) || ""].filter(Boolean).join(" — ");
  const showEng = opts && opts.engine && eng;
  return `<span class="badge ${cls}" title="${hsEsc(tip)}">${hsEsc(hsFwLabel(st))}</span>${showEng ? `<div class="mono muted hs-fw-eng">${hsEsc(eng)}</div>` : ""}`;
}
function hsFmtTime(ts) {
  if (!ts) return "—";
  try { return new Date(ts * 1000).toLocaleString(); } catch (_) { return "—"; }
}
function hsScanLabel(s) {
  if (!s) return "—";
  if (s.label) return s.label;
  const name = s.hostname || s.host_id || "扫描";
  return `${name} · ${hsFmtTime(s.started_at)}`;
}
function hsScanIdShort(id) {
  const s = String(id || "");
  if (s.length <= 22) return s;
  return s.slice(0, 18) + "…";
}
function hsCatLabel(cat) {
  const m = {
    hardening: hsT("hs.cat_hardening", "加固"),
    malware: hsT("hs.cat_malware", "恶意软件"),
    ioc: hsT("hs.cat_ioc", "威胁迹象"),
    cve: "CVE",
    port: hsT("hs.cat_port", "端口"),
  };
  return m[cat] || cat || "—";
}
async function hsUpdateFindingStatus(finding, status) {
  if (!hsSelected || !finding) return;
  try {
    const r = await fetch(`${API}/security/findings/status`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        scope: "host", host_id: hsSelected.host_id, status,
        finding: {
          id: finding.id, category: finding.category, cve: finding.cve, title: finding.title,
          detail: finding.detail || "", package: finding.package || "",
        },
      }),
    });
    const j = await r.json().catch(() => ({}));
    if (!r.ok) throw new Error(j.error || "更新失败");
    finding.status = status;
    hsPaintDetail(hsSelected);
    toast(hsT("hs.finding_updated", "状态已更新"), "ok");
  } catch (e) { toast(e.message || String(e), "err"); }
}
function hsFindingStatusControls(f) {
  const st = f.status || "open";
  const opts = ["open", "ack", "false_positive", "resolved"];
  return `<select class="hs-finding-status" data-hsfid="${hsEsc(f.id || "")}" data-hscat="${hsEsc(f.category || "")}" data-hsdetail="${hsEsc(f.detail || "")}" data-hspkg="${hsEsc(f.package || "")}" style="font-size:11px;height:26px">
    ${opts.map(o => `<option value="${o}"${o === st ? " selected" : ""}>${o}</option>`).join("")}
  </select>`;
}
function hsPortRiskCls(risk) {
  const m = { crit: "crit", critical: "crit", high: "high", medium: "warn" };
  return m[String(risk || "").toLowerCase()] || "info";
}
/** Deduplicate open_ports by proto+port (legacy scans may still have dual-stack dupes). */
function hsUniquePorts(ports) {
  const map = new Map();
  (ports || []).forEach(p => {
    if (!p || !p.port) return;
    const key = `${(p.proto || "tcp").toLowerCase()}|${p.port}`;
    const prev = map.get(key);
    if (!prev) { map.set(key, Object.assign({}, p)); return; }
    const rank = r => ({ crit: 3, high: 2, medium: 1 }[r] || 0);
    if (rank(p.risk) > rank(prev.risk)) prev.risk = p.risk;
    if (p.public) prev.public = true;
    if (!prev.process && p.process) prev.process = p.process;
    if (!prev.service && p.service) prev.service = p.service;
    if (p.public || (!prev.public && p.addr)) prev.addr = p.addr || prev.addr;
  });
  return [...map.values()].sort((a, b) => {
    const rank = r => ({ crit: 3, high: 2, medium: 1 }[r] || 0);
    return rank(b.risk) - rank(a.risk) || a.port - b.port;
  });
}
/** Compact open-port summary for tables; clickable to open port detail. */
function hsPortsCell(row, compact) {
  const scanId = row.scan_id || row.id || "";
  const ports = hsUniquePorts(row.open_ports || []);
  const sample = row.port_sample || [];
  const risky = ports.filter(p => p.risk === "crit" || p.risk === "high" || p.risk === "medium");
  const riskyN = risky.length || row.risky_port_count || 0;
  const total = ports.length || (row.port_count != null ? row.port_count : sample.length);
  if (!total && !sample.length) {
    return `<span class="muted">${hsEsc(hsT("hs.ports_unknown", "需重新扫描"))}</span>`;
  }
  const tipPorts = (risky.length ? risky : ports).slice(0, 12).map(p =>
    `${p.proto || "tcp"} ${p.addr || "*"}:${p.port}${p.service ? " " + p.service : ""}`
  ).join("\n");
  const tip = tipPorts || String(total);
  const riskBit = riskyN
    ? `<em class="hs-port-risk ${riskyN >= 3 ? "crit" : "high"}">${riskyN}</em>`
    : `<em class="hs-port-risk ok">0</em>`;
  let chips = "";
  const show = (risky.length ? risky : ports).slice(0, compact ? 2 : 3);
  if (show.length) {
    chips = `<span class="hs-port-chips">${show.map(p =>
      `<span class="hs-port-chip ${hsPortRiskCls(p.risk)}">${hsEsc(String(p.port))}</span>`
    ).join("")}${total > show.length ? `<span class="hs-port-chip more">+${total - show.length}</span>` : ""}</span>`;
  }
  const inner = `<span class="hs-ports-meta"><strong>${total}</strong>${riskBit}</span>${chips}`;
  if (!scanId) return `<div class="hs-ports-cell" title="${hsEsc(tip)}">${inner}</div>`;
  return `<button type="button" class="hs-ports-cell hs-ports-link" data-hs-ports="${hsEsc(scanId)}" title="${hsEsc(hsT("hs.ports_click", "点击查看开放端口明细") + "\n" + tip)}">${inner}</button>`;
}
function hsNoticeHTML() {
  return `<div class="sec-notice sec-notice-slim">${hsEsc(hsT("hs.notice", "Agent 采集加固 / 端口 / 防火墙 / ClamAV；服务端匹配 OSV CVE。导出含端口与防火墙明细。"))}</div>`;
}
function hsScoreCell(score, risk) {
  const n = score == null ? "—" : String(score);
  const cls = (risk === "crit" || risk === "critical") ? "crit"
    : (risk === "high") ? "high"
    : (risk === "medium") ? "med" : "ok";
  return `<div class="hs-score-cell ${cls}"><strong>${hsEsc(n)}</strong></div>`;
}
function hsExportMenuHTML(disabled) {
  const dis = disabled ? " disabled" : "";
  return `<div class="exp-dd">
    <button class="btn" data-hs="export-toggle" aria-haspopup="true"${dis}>${hsEsc(hsT("hs.export", "导出报告"))}</button>
    <div class="exp-dd-menu" id="hsExportMenu" role="menu">
      <button class="exp-dd-opt" role="menuitem" data-hsexport="pdf"><span>${hsEsc(hsT("hs.export_pdf", "PDF 报告"))}</span><span class="exp-dd-ext">${hsEsc(hsT("hs.export_pdf_tip", "打印"))}</span></button>
      <button class="exp-dd-opt" role="menuitem" data-hsexport="word"><span>${hsEsc(hsT("hs.export_word", "Word 文档"))}</span><span class="exp-dd-ext">.docx</span></button>
      <button class="exp-dd-opt" role="menuitem" data-hsexport="html"><span>${hsEsc(hsT("hs.export_html", "HTML 网页"))}</span><span class="exp-dd-ext">.html</span></button>
      <button class="exp-dd-opt" role="menuitem" data-hsexport="markdown"><span>${hsEsc(hsT("hs.export_md", "Markdown"))}</span><span class="exp-dd-ext">.md</span></button>
      <button class="exp-dd-opt" role="menuitem" data-hsexport="excel"><span>${hsEsc(hsT("hs.export_excel", "Excel 表格"))}</span><span class="exp-dd-ext">.xlsx</span></button>
      <button class="exp-dd-opt" role="menuitem" data-hsexport="json"><span>${hsEsc(hsT("hs.export_json", "JSON 原始数据"))}</span><span class="exp-dd-ext">.json</span></button>
    </div>
  </div>`;
}
function hsWeekdayOpts(selected) {
  const days = [
    [0, hsT("hs.wd_sun", "周日")], [1, hsT("hs.wd_mon", "周一")], [2, hsT("hs.wd_tue", "周二")],
    [3, hsT("hs.wd_wed", "周三")], [4, hsT("hs.wd_thu", "周四")], [5, hsT("hs.wd_fri", "周五")],
    [6, hsT("hs.wd_sat", "周六")],
  ];
  return days.map(([v, lab]) =>
    `<option value="${v}"${Number(selected) === Number(v) ? " selected" : ""}>${hsEsc(lab)}</option>`
  ).join("");
}

async function hsFetchJSON(url, opts) {
  const r = await fetch(url, Object.assign({ credentials: "same-origin" }, opts || {}));
  let d = null;
  try { d = await r.json(); } catch (_) { d = null; }
  if (!r.ok) throw new Error((d && d.error) || r.statusText || ("HTTP " + r.status));
  return d;
}

function hsFindingMatchesFilter(f, level) {
  if (!f) return false;
  const st = String(f.status || "open").toLowerCase();
  if (st === "resolved" || st === "false_positive" || st === "ack" || st === "accepted") return false;
  const lv = String(f.level || "").toLowerCase();
  if (level === "critical") return lv === "critical" || lv === "crit";
  if (level === "high") return lv === "high";
  return lv === "critical" || lv === "crit" || lv === "high"; // open = crit+high pending
}

function hsPendingBannerHTML() {
  if (!hsPendingFilter || !hsPendingFilter.level) return "";
  const label = hsPendingFilter.level === "critical"
    ? hsT("hs.filter_crit", "仅显示开放危急项")
    : hsPendingFilter.level === "high"
      ? hsT("hs.filter_high", "仅显示开放高危项")
      : hsT("hs.filter_open", "仅显示开放危急/高危项");
  return `<div class="sec-notice sec-notice-slim" id="hsPendingBanner">${hsEsc(label)}
    <button type="button" class="btn sm ghost" data-hs="clear-filter">${hsEsc(hsT("hs.clear_filter", "清除筛选"))}</button></div>`;
}

async function renderHostSecurity() {
  const el = $("hostSecurityPanel");
  if (!el) return;
  if (typeof window.secConsumePendingFilter === "function") {
    const f = window.secConsumePendingFilter("host-security");
    if (f) hsPendingFilter = f;
  }
  el.innerHTML = `<div class="loading-dots">${hsT("common.loading", "加载中...")}</div>`;
  try {
    let hosts = [];
    try {
      hosts = typeof fetchHostsList === "function"
        ? await fetchHostsList({ maxAgeMs: 20000 })
        : (await hsFetchJSON(`${API}/hosts`) || []);
    } catch (_) {
      hosts = (window._cachedHosts && window._cachedHosts.length) ? window._cachedHosts : [];
    }
    hsHosts = Array.isArray(hosts) ? hosts : [];
    const [sum, scans, cfg] = await Promise.all([
      hsFetchJSON(`${API}/security/host/summary`),
      hsFetchJSON(`${API}/security/host/scans?limit=40`),
      hsFetchJSON(`${API}/security/host/config`).catch(() => null),
    ]);
    hsSummary = sum.hosts || [];
    hsScans = scans.scans || [];
    hsCfg = cfg;
    if (hsPendingFilter && hsPendingFilter.level && !hsSelected) {
      const hit = (hsScans || []).find(s => s.status === "completed" && (s.findings || []).some(f => hsFindingMatchesFilter(f, hsPendingFilter.level)));
      if (hit) hsSelected = hit;
    }
    paintHostSecurity();
    hsMaybePoll();
  } catch (err) {
    el.innerHTML = `<div class="empty-state"><h4>${hsEsc(hsT("hs.load_failed", "加载失败"))}</h4><p>${hsEsc(err.message || err)}</p></div>`;
  }
}

function paintHostSecurity() {
  const el = $("hostSecurityPanel");
  if (!el) return;
  const online = hsHosts.filter(h => h.online !== false);
  const crit = hsSummary.filter(h => h.risk === "critical" || h.risk === "crit").length;
  const high = hsSummary.filter(h => h.risk === "high").length;
  const running = hsScans.filter(s => s.status === "running").length;

  let html = `<div class="sec-shell hs-shell">`;
  html += hsNoticeHTML();
  html += hsPendingBannerHTML();
  html += `<div class="sec-metrics">
    <div class="sec-metric"><b>${hsSummary.length}</b><span>${hsEsc(hsT("hs.stat_scanned", "已扫描"))}</span></div>
    <div class="sec-metric crit"><b>${crit}</b><span>${hsEsc(hsT("hs.stat_crit", "危急"))}</span></div>
    <div class="sec-metric high"><b>${high}</b><span>${hsEsc(hsT("hs.stat_high", "高危"))}</span></div>
    <div class="sec-metric"><b>${running}</b><span>${hsEsc(hsT("hs.stat_running", "进行中"))}</span></div>
  </div>`;

  html += `<div class="sec-toolbar hs-toolbar">
    <div class="hs-host-pick">
      <label>${hsEsc(hsT("hs.host", "主机"))}</label>
      <select id="hsHostSelect" class="nf-select" multiple size="3">`;
  if (!online.length && !hsHosts.length) {
    html += `<option value="" disabled>${hsEsc(hsT("hs.no_hosts", "暂无主机"))}</option>`;
  } else {
    (online.length ? online : hsHosts).forEach(h => {
      const off = h.online === false ? ` (${hsT("hs.offline", "离线")})` : "";
      html += `<option value="${hsEsc(h.id)}">${hsEsc(h.hostname || h.id)}${hsEsc(off)}</option>`;
    });
  }
  html += `</select></div>
    <div class="sec-toolbar-actions">
      <button class="btn primary" data-hs="scan" ${hsBusy ? "disabled" : ""}>${hsEsc(hsT("hs.scan", "扫描选中"))}</button>
      <button class="btn" data-hs="refresh">${hsEsc(hsT("common.refresh", "刷新"))}</button>
      ${typeof isAdmin === "function" && isAdmin() ? `<button class="btn" data-hs="cfg">${hsEsc(hsT("hs.config", "定时"))}</button>` : ""}
      <button class="btn nf-ai-btn" data-hs="ai-diag" title="${hsEsc(hsT("hs.ai_diag_tip", "研判风险、优先级与疑似误报"))}">${hsEsc(hsT("hs.ai_diag", "AI 研判"))}</button>
      <button class="btn nf-ai-btn" data-hs="ai-rem" title="${hsEsc(hsT("hs.ai_rem_tip", "生成可确认执行的修复动作计划"))}">${hsEsc(hsT("hs.ai_rem", "AI 修复"))}</button>
      ${hsExportMenuHTML(false)}
    </div>
  </div>`;

  if (hsShowCfg && hsCfg) {
    const schedHosts = new Set(hsCfg.host_ids || []);
    const sch = hsCfg.schedule || {};
    let hostOpts = "";
    (hsHosts || []).forEach(h => {
      // Empty host_ids means "all online"; leave none selected so re-save keeps that meaning.
      const sel = schedHosts.size > 0 && schedHosts.has(h.id) ? " selected" : "";
      const off = h.online === false ? ` (${hsT("hs.offline", "离线")})` : "";
      hostOpts += `<option value="${hsEsc(h.id)}"${sel}>${hsEsc(h.hostname || h.id)}${hsEsc(off)}</option>`;
    });
    html += `<div class="cfg-panel sec-cfg-panel">
      <div class="cfg-panel-head"><div class="cfg-panel-title">${hsEsc(hsT("hs.config_title", "主机扫描调度"))}</div>
        <span class="tag">${hsEsc(hsT("hs.admin_hint", "写入需管理员"))}</span></div>
      <div class="cfg-form">
        <p class="ws-help">${hsEsc(hsT("hs.cfg_edit_help", "可随时修改并再次保存：定时开关、ClamAV、周期与纳入调度的主机列表。"))}</p>
        <label class="switch cfg-enable"><input type="checkbox" id="hsCfgEnabled"${hsCfg.enabled ? " checked" : ""}><span>${hsEsc(hsT("hs.cfg_enabled", "启用定时扫描"))}</span></label>
        <label class="switch cfg-enable"><input type="checkbox" id="hsCfgClam"${hsCfg.enable_clamav !== false ? " checked" : ""}><span>${hsEsc(hsT("hs.cfg_clam", "尝试使用 ClamAV"))}</span></label>
        <label class="switch cfg-enable"><input type="checkbox" id="hsCfgAISummary"${hsCfg.auto_ai_summary ? " checked" : ""}><span>${hsEsc(hsT("hs.cfg_ai_summary", "扫描完成后自动 AI 摘要"))}</span></label>
        <div class="cfg-form-row">
          <div class="field"><label>${hsEsc(hsT("hs.cfg_kind", "周期"))}</label>
            <div class="select-wrap"><select id="hsCfgKind">
              <option value="interval"${sch.kind === "interval" ? " selected" : ""}>${hsEsc(hsT("hs.kind_interval", "间隔"))}</option>
              <option value="daily"${sch.kind === "daily" ? " selected" : ""}>${hsEsc(hsT("hs.kind_daily", "每天"))}</option>
              <option value="weekly"${!sch.kind || sch.kind === "weekly" ? " selected" : ""}>${hsEsc(hsT("hs.kind_weekly", "每周"))}</option>
            </select></div></div>
          <div class="field"><label>${hsEsc(hsT("hs.cfg_at", "时间 HH:MM / 间隔分钟"))}</label>
            <input id="hsCfgAt" value="${hsEsc(sch.at || sch.interval_min || "03:30")}"></div>
          <div class="field"><label>${hsEsc(hsT("hs.cfg_weekday", "星期（仅每周）"))}</label>
            <div class="select-wrap"><select id="hsCfgWeekday">${hsWeekdayOpts(sch.weekday != null ? sch.weekday : 0)}</select></div></div>
        </div>
        <div class="field"><label>${hsEsc(hsT("hs.cfg_hosts", "定时扫描主机（可多选，不选表示全部在线主机）"))}</label>
          <select id="hsCfgHosts" class="nf-select" multiple size="5" style="min-width:100%;min-height:96px">${hostOpts}</select>
          <p class="ws-help">${hsEsc(hsT("hs.cfg_hosts_help", "修改后点保存即可生效；与上方「扫描选中主机」的临时选择相互独立。"))}</p>
        </div>
        <div class="cfg-actions"><button class="btn primary" data-hs="save-cfg">${hsEsc(hsT("common.save", "保存修改"))}</button>
          <button class="btn" data-hs="cfg">${hsEsc(hsT("common.cancel", "收起"))}</button>
          <span class="cfg-status" id="hsCfgStatus"></span></div>
      </div>
    </div>`;
  }

  html += `<div class="hs-layout"><div class="hs-col-main">`;
  html += `<div class="cfg-panel hs-panel sec-panel">
    <div class="sec-panel-head">
      <div class="cfg-panel-title">${hsEsc(hsT("hs.summary", "主机风险汇总"))}</div>
      <span class="sec-panel-meta">${hsSummary.length}</span>
    </div>`;
  if (!hsSummary.length) {
    html += `<div class="sec-empty">
      <div class="sec-empty-ico" aria-hidden="true"></div>
      <h4>${hsEsc(hsT("hs.summary_empty_title", "暂无扫描结果"))}</h4>
      <p>${hsEsc(hsT("hs.summary_empty", "选择在线主机后点击「扫描选中」，结果将出现在此。"))}</p>
    </div>`;
  } else {
    html += `<div class="nf-table-wrap hs-table-wrap"><table class="data-table hs-table hs-table-compact"><thead><tr>
      <th>${hsEsc(hsT("hs.host", "主机"))}</th>
      <th class="col-score">${hsEsc(hsT("hs.score", "分"))}</th>
      <th class="col-risk">${hsEsc(hsT("hs.risk", "风险"))}</th>
      <th>${hsEsc(hsT("hs.ports", "端口"))}</th>
      <th>${hsEsc(hsT("hs.firewall", "防火墙"))}</th>
      <th class="col-num">CVE</th>
      <th class="col-time">${hsEsc(hsT("hs.time", "时间"))}</th>
    </tr></thead><tbody>`;
    hsSummary.forEach(h => {
      const active = hsSelected && hsSelected.id === h.scan_id ? " active-row" : "";
      const osLine = [h.distro, h.os].filter(Boolean).join(" · ");
      html += `<tr class="sec-row${active}" data-scan="${hsEsc(h.scan_id)}">
        <td><div class="hs-host-name">${hsEsc(h.hostname || h.host_id)}</div>
          ${osLine ? `<div class="hs-host-sub muted">${hsEsc(osLine)}</div>` : ""}</td>
        <td class="col-score">${hsScoreCell(h.score, h.risk)}</td>
        <td class="col-risk">${hsLevelBadge(h.risk)}</td>
        <td>${hsPortsCell(h, true)}</td>
        <td>${hsFwBadge(h)}</td>
        <td class="col-num mono">${h.cve_count ?? 0}</td>
        <td class="col-time mono muted">${hsEsc(hsFmtTime(h.finished_at))}</td>
      </tr>`;
    });
    html += `</tbody></table></div>`;
  }
  html += `</div>`;

  html += `<div class="cfg-panel hs-panel sec-panel">
    <div class="sec-panel-head">
      <div class="cfg-panel-title">${hsEsc(hsT("hs.history", "扫描历史"))}</div>
      <span class="sec-panel-meta">${Math.min(hsScans.length, 25)}${hsScans.length > 25 ? "+" : ""}</span>
    </div>`;
  if (!hsScans.length) {
    html += `<div class="sec-empty slim"><p>${hsEsc(hsT("hs.history_empty", "尚无历史记录"))}</p></div>`;
  } else {
    html += `<div class="nf-table-wrap hs-table-wrap"><table class="data-table hs-table hs-table-compact"><thead><tr>
      <th>${hsEsc(hsT("hs.batch", "批次"))}</th>
      <th>${hsEsc(hsT("hs.host", "主机"))}</th>
      <th class="col-risk">${hsEsc(hsT("hs.status", "状态"))}</th>
      <th class="col-score">${hsEsc(hsT("hs.score", "分"))}</th>
      <th>${hsEsc(hsT("hs.ports", "端口"))}</th>
      <th class="col-time">${hsEsc(hsT("hs.time", "时间"))}</th>
      <th></th>
    </tr></thead><tbody>`;
    hsScans.slice(0, 25).forEach(s => {
      const active = hsSelected && hsSelected.id === s.id ? " active-row" : "";
      const done = s.status === "completed";
      const cancelBtn = s.status === "running"
        ? `<button type="button" class="btn sm danger" data-hs-cancel="${hsEsc(s.id)}">${hsEsc(hsT("hs.cancel_scan", "取消"))}</button>`
        : "";
      html += `<tr class="sec-row${active}" data-scan="${hsEsc(s.id)}" title="${hsEsc(s.id)}">
        <td><div class="sec-batch">${hsEsc(hsScanLabel(s))}</div>
          <div class="mono muted sec-batch-id">${hsEsc(hsScanIdShort(s.id))}</div></td>
        <td><div class="hs-host-name">${hsEsc(s.hostname || s.host_id)}</div>
          ${done ? hsLevelBadge(s.risk) : ""}</td>
        <td class="col-risk">${hsStatusBadge(s.status)}</td>
        <td class="col-score">${done ? hsScoreCell(s.score, s.risk) : `<span class="muted">—</span>`}</td>
        <td>${done ? hsPortsCell(s, true) : `<span class="muted">—</span>`}</td>
        <td class="col-time mono muted">${hsEsc(hsFmtTime(s.finished_at || s.started_at))}</td>
        <td>${cancelBtn}</td>
      </tr>`;
    });
    html += `</tbody></table></div>`;
  }
  html += `</div></div>
    <div class="hs-col-side"><div id="hsDetail" class="cfg-panel hs-panel hs-detail sec-panel"></div></div>
  </div></div>`;
  el.innerHTML = html;

  el.querySelectorAll("[data-hs]").forEach(b => b.addEventListener("click", e => {
    e.stopPropagation();
    hsAction(b.dataset.hs);
  }));
  el.querySelectorAll("[data-hsexport]").forEach(b => b.addEventListener("click", e => {
    e.stopPropagation();
    document.querySelectorAll("#hsExportMenu.show").forEach(m => m.classList.remove("show"));
    hsDoExport(b.dataset.hsexport);
  }));
  el.querySelectorAll("tr[data-scan]").forEach(tr => tr.addEventListener("click", () => hsLoadDetail(tr.dataset.scan)));
  el.querySelectorAll("[data-hs-cancel]").forEach(btn => btn.addEventListener("click", async e => {
    e.preventDefault();
    e.stopPropagation();
    try {
      await hsFetchJSON(`${API}/security/host/scans/${encodeURIComponent(btn.dataset.hsCancel)}/cancel`, { method: "POST" });
      toast(hsT("hs.cancel_ok", "已取消扫描"), "ok");
      renderHostSecurity();
    } catch (err) { toast(err.message || String(err), "err"); }
  }));
  el.querySelectorAll("[data-hs-ports]").forEach(btn => btn.addEventListener("click", e => {
    e.preventDefault();
    e.stopPropagation();
    hsLoadDetail(btn.dataset.hsPorts, { focus: "ports" });
  }));
  if (hsSelected) hsPaintDetail(hsSelected);
  else {
    const box = $("hsDetail");
    if (box) {
      box.innerHTML = `<div class="sec-empty hs-detail-empty">
        <div class="sec-empty-ico" aria-hidden="true"></div>
        <h4>${hsEsc(hsT("hs.pick_scan_title", "选择一条扫描记录"))}</h4>
        <p>${hsEsc(hsT("hs.pick_scan_hint", "点击左侧主机或历史批次，查看防火墙、开放端口与风险明细，并导出报告。"))}</p>
      </div>`;
    }
  }
}

function hsAction(act) {
  if (act === "refresh") return renderHostSecurity();
  if (act === "scan") return hsRunScan();
  if (act === "ai" || act === "ai-diag") return hsAI("diagnosis");
  if (act === "ai-rem") return hsAI("remediation");
  if (act === "clear-filter") { hsPendingFilter = null; return paintHostSecurity(); }
  if (act === "export-toggle") {
    const menu = $("hsExportMenu");
    if (menu) menu.classList.toggle("show");
    return;
  }
  if (act === "cfg") { hsShowCfg = !hsShowCfg; return paintHostSecurity(); }
  if (act === "save-cfg") return hsSaveCfg();
}

document.addEventListener("click", () => {
  document.querySelectorAll("#hsExportMenu.show, #wsExportMenu.show").forEach(m => m.classList.remove("show"));
});

async function hsSaveCfg() {
  const status = $("hsCfgStatus");
  const kind = ($("hsCfgKind") && $("hsCfgKind").value) || "weekly";
  const atRaw = ($("hsCfgAt") && $("hsCfgAt").value || "").trim();
  const schedule = { enabled: !!($("hsCfgEnabled") && $("hsCfgEnabled").checked), kind };
  if (kind === "interval") schedule.interval_min = Math.max(15, parseInt(atRaw, 10) || 1440);
  else schedule.at = atRaw || "03:30";
  if (kind === "weekly") {
    schedule.weekday = parseInt(($("hsCfgWeekday") && $("hsCfgWeekday").value) || "0", 10) || 0;
  }
  const hostSel = $("hsCfgHosts");
  const hostIds = hostSel ? [...hostSel.selectedOptions].map(o => o.value).filter(Boolean) : ((hsCfg && hsCfg.host_ids) || []);
  const body = {
    enabled: schedule.enabled,
    enable_clamav: !!($("hsCfgClam") && $("hsCfgClam").checked),
    auto_ai_summary: !!($("hsCfgAISummary") && $("hsCfgAISummary").checked),
    osv_url: (hsCfg && hsCfg.osv_url) || "",
    timeout_sec: (hsCfg && hsCfg.timeout_sec) || 180,
    host_ids: hostIds,
    schedule,
  };
  try {
    hsCfg = await hsFetchJSON(`${API}/security/host/config`, {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
    });
    if (status) { status.textContent = hsT("common.save", "保存") + hsT("toast.ok_suffix", "成功"); status.className = "cfg-status ok"; }
    if (typeof toast === "function") toast(hsT("toast.saved", "已保存"), "ok");
  } catch (e) {
    if (status) { status.textContent = e.message; status.className = "cfg-status err"; }
    if (typeof toast === "function") toast(String(e.message || e), "err");
  }
}

async function hsRunScan() {
  const sel = $("hsHostSelect");
  if (!sel) return;
  const ids = [...sel.selectedOptions].map(o => o.value).filter(Boolean);
  if (!ids.length) {
    if (typeof toast === "function") toast(hsT("hs.pick_host", "请选择主机"), "err");
    return;
  }
  hsBusy = true;
  paintHostSecurity();
  try {
    const d = await hsFetchJSON(`${API}/security/host/scan`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ host_ids: ids }),
    });
    const first = (d.results || []).find(x => x.scan);
    if (first && first.scan) hsSelected = first.scan;
    if (typeof toast === "function") toast(hsT("hs.scan_started", "扫描已启动"), "ok");
  } catch (e) {
    if (typeof toast === "function") toast(String(e.message || e), "err");
  } finally {
    hsBusy = false;
    await renderHostSecurity();
  }
}

function hsSoftRefresh() {
  const sel = $("hsHostSelect");
  const selected = sel ? [...sel.selectedOptions].map(o => o.value) : [];
  paintHostSecurity();
  const sel2 = $("hsHostSelect");
  if (sel2 && selected.length) {
    [...sel2.options].forEach(o => { o.selected = selected.includes(o.value); });
  }
  if (hsSelected) hsPaintDetail(hsSelected);
}

function hsMaybePoll() {
  if (hsPollTimer) { clearInterval(hsPollTimer); hsPollTimer = null; }
  const running = (hsScans || []).some(s => s.status === "running") || (hsSelected && hsSelected.status === "running");
  if (!running) return;
  hsPollTimer = setInterval(async () => {
    if (!document.querySelector("#view-host-security.active")) {
      clearInterval(hsPollTimer); hsPollTimer = null; return;
    }
    try {
      hsScans = (await hsFetchJSON(`${API}/security/host/scans?limit=40`)).scans || [];
      hsSummary = (await hsFetchJSON(`${API}/security/host/summary`)).hosts || [];
      if (hsSelected && hsSelected.id) {
        hsSelected = await hsFetchJSON(`${API}/security/host/scans/` + encodeURIComponent(hsSelected.id));
      }
      hsSoftRefresh();
      if (!(hsScans || []).some(s => s.status === "running")) {
        clearInterval(hsPollTimer); hsPollTimer = null;
      }
    } catch (_) {}
  }, 2500);
}

async function hsLoadDetail(id, opts) {
  if (!id) return;
  try {
    hsSelected = await hsFetchJSON(`${API}/security/host/scans/` + encodeURIComponent(id));
    hsPaintDetail(hsSelected, opts);
    document.querySelectorAll("#hostSecurityPanel tr.sec-row").forEach(tr => {
      tr.classList.toggle("active-row", tr.dataset.scan === id);
    });
  } catch (e) {
    if (typeof toast === "function") toast(String(e.message || e), "err");
  }
}

function hsPaintDetail(scan, opts) {
  const box = $("hsDetail");
  if (!box || !scan) return;
  if (scan.status === "running") {
    box.innerHTML = `<div class="cfg-panel-title">${hsEsc(hsScanLabel(scan))}</div>
      <div class="hs-progress"><div class="hs-progress-bar"></div></div>
      <p class="ws-help">${hsEsc(hsT("hs.scanning", "扫描进行中…"))}</p>`;
    return;
  }
  const ports = hsUniquePorts(scan.open_ports || []);
  const riskyN = ports.filter(p => p.risk === "crit" || p.risk === "high" || p.risk === "medium").length;
  const portTotal = ports.length || scan.port_count || 0;
  const osLine = [scan.os, scan.distro].filter(Boolean).join(" / ");
  const sum = scan.summary || {};
  const canExport = scan.status === "completed";
  let html = `<div class="cfg-panel-head hs-detail-head"><div>
      <div class="cfg-panel-title">${hsEsc(hsT("hs.detail", "扫描详情"))} · ${hsEsc(scan.hostname || scan.host_id)}</div>
      <p class="cfg-panel-desc">${hsEsc(hsScanLabel(scan))} · <code class="mono muted">${hsEsc(hsScanIdShort(scan.id))}</code>${osLine ? ` · ${hsEsc(osLine)}` : ""}</p>
    </div>
    <div class="hs-detail-actions">
      ${hsStatusBadge(scan.status)}
      ${canExport ? `<button class="btn sm nf-ai-btn" data-hs="ai-diag">${hsEsc(hsT("hs.ai_diag", "AI 研判"))}</button>
      <button class="btn sm nf-ai-btn" data-hs="ai-rem">${hsEsc(hsT("hs.ai_rem", "AI 修复"))}</button>
      <button class="btn sm primary" data-hs="export-toggle">${hsEsc(hsT("hs.export", "导出报告"))}</button>` : ""}
    </div></div>`;
  if (scan.error) html += `<div class="sec-error-box">${hsEsc(scan.error)}</div>`;
  if (scan.baseline_diff) {
    const d = scan.baseline_diff;
    html += `<div class="hint" style="margin:8px 0">较上次：新增 <b>${d.added || 0}</b> · 消失 <b>${d.removed || 0}</b> · 恶化 <b>${d.worsened || 0}</b> · 缓解 <b>${d.improved || 0}</b></div>`;
  }
  if (scan.ai_summary) {
    html += `<div class="sec-remediation" style="margin:8px 0"><div class="cfg-panel-title">${hsEsc(hsT("hs.ai_summary", "AI 摘要"))}</div>
      <pre class="mono" style="white-space:pre-wrap;margin:0;font-size:12px">${hsEsc(scan.ai_summary)}</pre></div>`;
  }
  html += `<div class="sec-metrics compact hs-detail-kpis">
    <div class="sec-metric"><b>${scan.score ?? "—"}</b><span>${hsEsc(hsT("hs.score", "安全分"))}</span></div>
    <div class="sec-metric"><b>${portTotal}</b><span>${hsEsc(hsT("hs.ports_open", "开放端口"))}</span></div>
    <div class="sec-metric ${riskyN ? "high" : ""}"><b>${riskyN}</b><span>${hsEsc(hsT("hs.risky_ports", "高危端口"))}</span></div>
    <div class="sec-metric"><b>${scan.cve_count || 0}</b><span>CVE</span></div>
    <div class="sec-metric crit"><b>${sum.crit || 0}</b><span>${hsEsc(hsT("hs.level_crit", "危急"))}</span></div>
  </div>`;
  html += `<div class="hs-fw-panel">
    <div class="hs-fw-head"><span class="cfg-panel-title">${hsEsc(hsT("hs.firewall", "防火墙"))}</span> ${hsFwBadge(scan, { engine: true })}
      <span class="tag">${hsEsc(hsClamLabel(scan.clamav))}</span> ${hsLevelBadge(scan.risk)}</div>
    <p class="ws-help">${hsEsc(scan.firewall_detail || hsT("hs.fw_no_detail", "暂无防火墙引擎原始输出；可重新扫描以刷新状态。"))}</p>
  </div>`;
  if (ports.length) {
    html += `<div class="hs-port-panel" id="hsPortPanel"><div class="cfg-panel-title">${hsEsc(hsT("hs.ports_detail", "开放端口明细"))} <span class="tag">${ports.length}</span></div>
      <p class="ws-help">${hsEsc(hsT("hs.ports_help", "对外绑定（0.0.0.0 / ::）的数据库、远程桌面、缓存等高危服务会标红并计入评分。同端口 IPv4/IPv6 已合并展示。"))}</p>
      <div class="nf-table-wrap hs-table-wrap"><table class="data-table hs-table"><thead><tr>
        <th>${hsEsc(hsT("hs.port", "端口"))}</th><th>${hsEsc(hsT("hs.proto", "协议"))}</th>
        <th>${hsEsc(hsT("hs.bind", "绑定"))}</th><th>${hsEsc(hsT("hs.service", "服务"))}</th>
        <th>${hsEsc(hsT("hs.process", "进程"))}</th><th>${hsEsc(hsT("hs.risk", "风险"))}</th>
      </tr></thead><tbody>`;
    ports.slice(0, 200).forEach(p => {
      html += `<tr>
        <td class="mono"><strong>${p.port}</strong></td>
        <td class="mono">${hsEsc(p.proto || "tcp")}</td>
        <td class="mono">${hsEsc(p.addr || "*")}${p.public ? ` <span class="badge warn">${hsEsc(hsT("hs.bind_public", "对外"))}</span>` : ""}</td>
        <td>${hsEsc(p.service || "—")}</td>
        <td class="mono muted">${hsEsc(p.process || "—")}</td>
        <td>${p.risk ? hsLevelBadge(p.risk) : `<span class="badge ok">${hsEsc(hsT("hs.level_info", "信息"))}</span>`}</td>
      </tr>`;
    });
    html += `</tbody></table></div></div>`;
  } else if (opts && opts.focus === "ports") {
    html += `<div class="hs-port-panel" id="hsPortPanel"><div class="empty-line">${hsEsc(hsT("hs.ports_unknown", "需重新扫描后可查看端口明细"))}</div></div>`;
  }
  if ((scan.remediation || []).length) {
    html += `<div class="sec-remediation"><div class="cfg-panel-title">${hsEsc(hsT("hs.remediation", "修复建议"))}</div><ul>`;
    const seenTips = new Set();
    scan.remediation.forEach(t => {
      const key = String(t || "").trim().toLowerCase();
      if (!key || seenTips.has(key)) return;
      seenTips.add(key);
      html += `<li>${hsEsc(t)}</li>`;
    });
    html += `</ul></div>`;
  }
  const allFindings = scan.findings || [];
  const filterLevel = hsPendingFilter && hsPendingFilter.level;
  const shownIdx = [];
  allFindings.forEach((f, idx) => {
    if (filterLevel && !hsFindingMatchesFilter(f, filterLevel)) return;
    shownIdx.push(idx);
  });
  html += `<div class="cfg-panel-title">${hsEsc(hsT("hs.findings", "风险明细"))} <span class="tag">${filterLevel ? shownIdx.length + "/" + allFindings.length : allFindings.length}</span></div>`;
  html += `<div class="nf-table-wrap hs-table-wrap"><table class="data-table hs-table"><thead><tr>
    <th>${hsEsc(hsT("hs.level", "级别"))}</th><th>${hsEsc(hsT("hs.category", "类别"))}</th>
    <th>${hsEsc(hsT("hs.title", "标题"))}</th><th>CVE</th><th>${hsEsc(hsT("hs.suggest", "建议"))}</th>
    <th>${hsEsc(hsT("hs.finding_status", "状态"))}</th>
    <th>${hsEsc(hsT("hs.finding_ai", "AI"))}</th>
  </tr></thead><tbody>`;
  shownIdx.slice(0, 200).forEach(idx => {
    const f = allFindings[idx];
    html += `<tr>
      <td>${hsLevelBadge(f.level)}</td>
      <td><span class="tag">${hsEsc(hsCatLabel(f.category))}</span></td>
      <td>${hsEsc(f.title)}<div class="field-hint">${hsEsc(f.detail || "")}</div></td>
      <td class="mono">${hsEsc(f.cve || f.id || "")}</td>
      <td>${hsEsc(f.suggest || "")}</td>
      <td>${hsFindingStatusControls(f)}</td>
      <td><button type="button" class="btn sm nf-ai-btn" data-hs-finding="${idx}" title="${hsEsc(hsT("hs.ai_finding_tip", "针对本条给出研判与修复建议"))}">${hsEsc(hsT("hs.ai_finding", "建议"))}</button></td>
    </tr>`;
  });
  if (!shownIdx.length) {
    html += `<tr><td colspan="7" class="empty-line">${hsEsc(filterLevel ? hsT("hs.no_filtered", "当前筛选下无待处置项") : hsT("hs.no_findings", "未发现风险项"))}</td></tr>`;
  }
  html += `</tbody></table></div>`;
  box.innerHTML = html;
  box.querySelectorAll(".hs-finding-status").forEach(sel => {
    sel.addEventListener("change", () => {
      const fid = sel.dataset.hsfid || "";
      const cat = sel.dataset.hscat || "";
      const detail = sel.dataset.hsdetail || "";
      const pkg = sel.dataset.hspkg || "";
      const finding = (scan.findings || []).find(x =>
        (x.id || "") === fid &&
        (x.category || "") === cat &&
        (x.detail || "") === detail &&
        (x.package || "") === pkg
      ) || (scan.findings || []).find(x => (x.id || "") === fid && (x.category || "") === cat);
      if (finding) hsUpdateFindingStatus(finding, sel.value);
    });
  });
  box.querySelectorAll("[data-hs-finding]").forEach(b => b.addEventListener("click", e => {
    e.stopPropagation();
    const idx = parseInt(b.dataset.hsFinding, 10);
    hsAIFinding(scan, idx);
  }));
  box.querySelectorAll("[data-hs]").forEach(b => b.addEventListener("click", e => {
    e.stopPropagation();
    hsAction(b.dataset.hs);
  }));
  const expBtn = document.querySelector("#hostSecurityPanel [data-hs=\"export-toggle\"]");
  if (expBtn) expBtn.disabled = !canExport;
  if (opts && opts.focus === "ports") {
    const panel = $("hsPortPanel") || box;
    try { panel.scrollIntoView({ behavior: "smooth", block: "start" }); } catch (_) {}
    panel.classList.add("hs-port-panel-focus");
    setTimeout(() => panel.classList.remove("hs-port-panel-focus"), 1600);
  }
}

function hsBuildReportModel(scan) {
  const sum = scan.summary || {};
  const ports = hsUniquePorts(scan.open_ports || []);
  const riskyN = ports.filter(p => p.risk === "crit" || p.risk === "high" || p.risk === "medium").length
    || scan.risky_port_count || 0;
  const portCount = ports.length || scan.port_count || 0;
  const fwText = hsFwLabel(scan.firewall) + (scan.firewall_engine ? ` (${scan.firewall_engine})` : "");
  const osLine = [scan.os, scan.distro].filter(Boolean).join(" / ") || "—";
  const narrative = [
    `# ${hsT("hs.report_exec", "执行摘要")}`,
    "",
    hsT("hs.report_exec_body", "主机「{host}」安全扫描已完成，安全分 {score}，风险等级 {risk}。防火墙：{fw}；ClamAV：{clam}；软件包 {pkgs} 个，匹配 CVE {cves} 条；开放端口 {ports} 个，其中高危 {risky} 个。")
      .replace("{host}", scan.hostname || scan.host_id)
      .replace("{score}", String(scan.score ?? "—"))
      .replace("{risk}", hsLevelLabel(scan.risk))
      .replace("{fw}", fwText)
      .replace("{clam}", hsClamLabel(scan.clamav))
      .replace("{pkgs}", String(scan.pkg_count || 0))
      .replace("{cves}", String(scan.cve_count || 0))
      .replace("{ports}", String(portCount))
      .replace("{risky}", String(riskyN)),
    "",
    `# ${hsT("hs.remediation", "修复建议")}`,
    ...((scan.remediation || []).length ? scan.remediation.map(t => `- ${t}`) : [`- ${hsT("hs.no_remediation", "暂无额外修复建议")}`]),
  ].join("\n");
  const findingRows = (scan.findings || []).map(f => [
    hsLevelLabel(f.level), hsCatLabel(f.category),
    f.title || "", f.cve || f.id || "", f.detail || "", f.suggest || "",
  ]);
  const fwDetail = String(scan.firewall_detail || "").trim();
  const fwRows = [
    [hsT("hs.firewall", "防火墙状态"), hsFwLabel(scan.firewall)],
    [hsT("hs.fw_engine", "防火墙引擎"), scan.firewall_engine || "—"],
    [hsT("hs.fw_detail", "引擎详情"), fwDetail || hsT("hs.fw_no_detail", "暂无原始输出")],
  ];
  const portRows = ports.map(p => [
    String(p.port),
    p.proto || "tcp",
    p.addr || "*",
    p.public ? hsT("hs.bind_public", "对外") : hsT("hs.bind_local", "本机/内网"),
    p.service || "—",
    p.process || "—",
    p.risk ? hsLevelLabel(p.risk) : hsT("hs.level_info", "信息"),
  ]);
  return {
    title: hsT("hs.report_title", "主机安全扫描报告") + " — " + (scan.hostname || scan.host_id),
    subtitle: hsT("hs.report_sub", "生成时间") + " " + new Date().toLocaleString() + " · " + hsScanLabel(scan),
    summaryTitle: hsT("hs.report_meta", "报告摘要"),
    narrativeTitle: hsT("hs.report_analysis", "分析结论与建议"),
    meta: [
      [hsT("hs.batch", "扫描批次"), hsScanLabel(scan)],
      [hsT("hs.host", "主机"), scan.hostname || scan.host_id || ""],
      [hsT("hs.os", "系统"), osLine],
      [hsT("hs.score", "安全分"), String(scan.score ?? "—")],
      [hsT("hs.risk", "风险等级"), hsLevelLabel(scan.risk)],
      [hsT("hs.firewall", "防火墙"), fwText],
      ["ClamAV", hsClamLabel(scan.clamav)],
      ["CVE", String(scan.cve_count || 0)],
      [hsT("hs.pkgs", "软件包"), String(scan.pkg_count || 0)],
      [hsT("hs.ports_open", "开放端口"), String(portCount)],
      [hsT("hs.risky_ports", "高危端口"), String(riskyN)],
      [hsT("hs.status", "状态"), hsStatusLabel(scan.status)],
      [hsT("hs.time", "完成时间"), hsFmtTime(scan.finished_at)],
    ],
    kpis: [
      [hsT("hs.score", "安全分"), String(scan.score ?? "—")],
      [hsT("hs.ports_open", "开放端口"), String(portCount)],
      [hsT("hs.risky_ports", "高危端口"), String(riskyN)],
      [hsT("hs.level_crit", "危急"), String(sum.crit || 0)],
      [hsT("hs.level_high", "高危"), String(sum.high || 0)],
      ["CVE", String(scan.cve_count || 0)],
    ],
    narrative,
    sections: [
      {
        title: hsT("hs.report_fw_sec", "防火墙状态"),
        columns: [hsT("hs.report_item", "项"), hsT("hs.report_value", "值")],
        rows: fwRows,
      },
      {
        title: hsT("hs.ports_detail", "开放端口明细"),
        columns: [
          hsT("hs.port", "端口"), hsT("hs.proto", "协议"), hsT("hs.bind", "绑定"),
          hsT("hs.bind_scope", "暴露面"), hsT("hs.service", "服务"),
          hsT("hs.process", "进程"), hsT("hs.risk", "风险"),
        ],
        rows: portRows.length
          ? portRows
          : [[hsT("hs.ports_unknown", "需重新扫描后可查看端口明细"), "", "", "", "", "", ""]],
      },
      {
        title: hsT("hs.findings", "风险明细"),
        columns: [
          hsT("hs.level", "级别"), hsT("hs.category", "类别"), hsT("hs.title", "标题"),
          "CVE", hsT("hs.detail_col", "详情"), hsT("hs.suggest", "建议"),
        ],
        rows: findingRows.length ? findingRows : [[hsT("hs.no_findings", "未发现风险项"), "", "", "", "", ""]],
      },
    ],
    footer: hsT("hs.report_footer", "本报告由 AIOps Monitor 安全中心自动生成，仅供运维处置参考，不替代专业渗透测试。"),
    orientation: "landscape",
    rawJSON: {
      report_type: "host_security",
      generated_at: new Date().toISOString(),
      scan_id: scan.id,
      label: hsScanLabel(scan),
      host_id: scan.host_id,
      hostname: scan.hostname,
      os: scan.os,
      distro: scan.distro,
      score: scan.score,
      risk: scan.risk,
      status: scan.status,
      clamav: scan.clamav,
      firewall: scan.firewall,
      firewall_engine: scan.firewall_engine,
      firewall_detail: scan.firewall_detail,
      cve_count: scan.cve_count,
      pkg_count: scan.pkg_count,
      port_count: portCount,
      risky_port_count: riskyN,
      open_ports: ports,
      remediation: scan.remediation || [],
      findings: scan.findings || [],
      summary: sum,
      started_at: scan.started_at,
      finished_at: scan.finished_at,
    },
  };
}

async function hsEnsureSelectedScan() {
  if (!hsSelected || !hsSelected.id) return null;
  if (hsSelected.status !== "completed") return null;
  // Re-fetch full scan so export always includes open_ports / firewall_detail.
  try {
    hsSelected = await hsFetchJSON(`${API}/security/host/scans/` + encodeURIComponent(hsSelected.id));
  } catch (_) { /* use cached */ }
  return hsSelected && hsSelected.status === "completed" ? hsSelected : null;
}

async function hsDoExport(fmt) {
  const scan = await hsEnsureSelectedScan();
  if (!scan) {
    if (typeof toast === "function") toast(hsT("hs.pick_scan", "请先选择一条已完成的扫描结果"), "err");
    return;
  }
  try {
    const model = hsBuildReportModel(scan);
    const ok = await exportModel(model, fmt, "主机安全报告_" + (scan.hostname || scan.host_id));
    if (ok === false && typeof toast === "function") toast(hsT("hs.export_popup", "浏览器拦截了导出窗口，请允许弹窗后重试"), "err");
    else if (fmt !== "pdf" && typeof toast === "function") toast(hsT("toast.exported", "已导出"), "ok");
  } catch (e) {
    if (typeof toast === "function") toast(hsT("hs.export_fail", "导出失败") + "：" + (e.message || e), "err");
  }
}

function hsAIContext(scan, maxFindings) {
  const model = hsBuildReportModel(scan);
  const hostId = scan.host_id || "";
  return {
    hostId,
    text: (model.narrative + "\n\n" + JSON.stringify({
      host_id: hostId,
      hostname: scan.hostname,
      os: scan.os,
      distro: scan.distro,
      score: scan.score,
      risk: scan.risk,
      clamav: scan.clamav,
      firewall: scan.firewall,
      meta: model.meta,
      findings: (scan.findings || []).slice(0, maxFindings || 40),
    }, null, 2)).slice(0, 14000),
  };
}

function hsAI(kind) {
  if (!hsSelected || hsSelected.status === "running") {
    if (typeof toast === "function") toast(hsT("hs.pick_scan", "请先选择一条已完成的扫描结果"), "err");
    return;
  }
  if (hsSelected.status !== "completed" && hsSelected.status !== "failed") {
    if (typeof toast === "function") toast(hsT("hs.pick_scan", "请先选择一条已完成的扫描结果"), "err");
    return;
  }
  const mode = kind === "remediation" ? "remediation" : "diagnosis";
  const { hostId, text } = hsAIContext(hsSelected, 40);
  if (typeof openAIAssist !== "function") return;
  if (mode === "remediation") {
    openAIAssist({
      task: "host_security_remediation", mode: "analyze",
      title: hsT("hs.ai_rem_title", "AI · 主机安全修复计划"),
      context: text,
      applyLabel: hsT("ai.apply_actions", "应用建议动作"),
      applyTo: async (code) => {
        if (typeof window.applyOpsActionPlan !== "function") return false;
        return window.applyOpsActionPlan(code, {
          source: "host-security",
          hostId,
          refresh: () => renderHostSecurity(),
        });
      },
    });
    return;
  }
  openAIAssist({
    task: "host_security_diagnosis", mode: "analyze",
    title: hsT("hs.ai_diag_title", "AI · 主机安全研判"),
    context: text,
    hint: hsT("hs.ai_diag_hint", "正在研判整体风险、优先级与疑似误报…"),
  });
}

function hsAIFinding(scan, idx) {
  if (!scan || typeof openAIAssist !== "function") return;
  const findings = scan.findings || [];
  const f = findings[idx];
  if (!f) {
    if (typeof toast === "function") toast(hsT("hs.finding_missing", "未找到该风险项"), "err");
    return;
  }
  const ctx = {
    host_id: scan.host_id,
    hostname: scan.hostname,
    os: scan.os,
    distro: scan.distro,
    score: scan.score,
    risk: scan.risk,
    finding: f,
    peers_same_category: findings.filter(x => x !== f && x.category === f.category).slice(0, 5).map(x => ({
      level: x.level, id: x.id, title: x.title, cve: x.cve,
    })),
  };
  openAIAssist({
    task: "host_security_finding", mode: "analyze",
    title: hsT("hs.ai_finding_title", "AI · 单条风险建议") + " · " + (f.title || f.id || "").slice(0, 40),
    context: JSON.stringify(ctx, null, 2).slice(0, 12000),
    hint: hsT("hs.ai_finding_hint", "正在分析本条 finding 的真伪、影响与修复步骤…"),
    applyLabel: hsT("hs.ai_apply_status", "按建议更新状态"),
    applyTo: async (text) => {
      const low = String(text || "").toLowerCase();
      let status = "";
      if (/\bfalse[_\s-]?positive\b/.test(low) || low.includes("误报")) status = "false_positive";
      else if (/\bresolved\b/.test(low) || low.includes("已修复") || low.includes("可关闭")) status = "resolved";
      else if (/\back\b/.test(low) || low.includes("已知接受") || low.includes("暂时接受")) status = "ack";
      if (!status) {
        if (typeof toast === "function") toast(hsT("hs.ai_status_unclear", "未从回复中识别到明确状态建议，请手动选择"), "warn");
        return false;
      }
      await hsUpdateFindingStatus(f, status);
      return true;
    },
  });
}

window._pageRenderers = window._pageRenderers || {};
window._pageRenderers["host-security"] = renderHostSecurity;
})();
