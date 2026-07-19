// content-audit.js — 明文 HTTP 内容审计面板（Phase 2）。并入「网络」父菜单的「内容审计」子标签。
// 展示 agent 抓到的明文 HTTP 请求：谁(源IP) 向 哪个端点(域名+路径) 发了什么(方法/内容/prompt)。
// ⚠ 高敏感：仅在 agent 显式开启 content_audit 时才有数据；面板顶部常驻合规提示。
(function() {
"use strict";

let caHost = "";
let caKw = "";
let caSearchT = null;
// 「只列有内容审计数据的主机」：从 /api/v1/content-audit/hosts 拉有审计记录的主机，
// 无数据的主机不进下拉。null=未加载；刷新/进入视图时重拉。
let caDataHosts = null;

// 常驻合规提示（内容审计涉及用户明文请求，属隐私敏感）。
function caNoticeHTML() {
  return `<div style="margin:0 0 10px;padding:9px 12px;border:1px solid var(--line);border-left:3px solid #e0a300;border-radius:8px;background:rgba(224,163,0,.08);font-size:12px;color:var(--muted)">${esc(I18N.t("ca.notice") || "⚠ 内容审计含用户明文请求（可能包含 PII / prompt）。仅可在你有授权的网络启用，并对用户履行告知义务；服务端记录保留 30 天后自动清理。")}</div>`;
}

function renderContentAuditPanel() {
  const container = $("contentAuditPanel");
  if (!container) return;

  // 先拉「有审计数据的主机」，再渲染面板——无数据主机不列进下拉。
  if (caDataHosts === null) {
    container.innerHTML = `<div class="loading-dots">${I18N.t("common.loading") || "加载中..."}</div>`;
    fetch(`/api/v1/content-audit/hosts`, { credentials: "same-origin" })
      .then(r => r.json())
      .then(d => { caDataHosts = d.hosts || []; renderContentAuditPanel(); })
      .catch(() => { caDataHosts = []; renderContentAuditPanel(); });
    return;
  }

  // host_id → hostname 映射（有数据主机只带 host_id，用 _cachedHosts 补展示名）。
  const nameMap = {};
  (window._cachedHosts || []).forEach(h => { nameMap[h.id] = h; });

  let html = caNoticeHTML();
  if (caDataHosts.length === 0) {
    container.innerHTML = html + `<div class="empty-state">${I18N.t("ca.no_hosts") || "暂无有内容审计数据的主机（需在 agent 配置 content_audit: true 并有明文 HTTP 流量）"}</div>`;
    return;
  }
  html += `<div class="nf-toolbar">`;
  html += `<select id="caHostSelect" class="nf-select">`;
  caDataHosts.forEach(dh => {
    const h = nameMap[dh.host_id] || {};
    const name = h.hostname || dh.host_id;
    const sel = dh.host_id === caHost ? " selected" : "";
    html += `<option value="${esc(dh.host_id)}"${sel}>${esc(name)} · ${dh.events || 0}</option>`;
  });
  html += `</select>`;
  html += `<input type="search" id="caKw" class="nf-input" value="${esc(caKw)}" placeholder="${esc(I18N.t("ca.kw_ph") || "搜索 域名 / 路径 / 内容关键字")}">`;
  html += `<button class="nf-btn" data-caact="refresh">${I18N.t("common.refresh") || "刷新"}</button>`;
  html += `</div>`;
  html += `<div id="caBody"></div>`;
  container.innerHTML = html;

  const sel = $("caHostSelect");
  if (sel && sel.options.length > 0) {
    if (![...sel.options].some(o => o.value === caHost)) caHost = sel.options[0].value;
    sel.value = caHost;
  }
  caBind();
  if (caHost) loadContentAudit();
}

function caBind() {
  const sel = $("caHostSelect");
  sel && sel.addEventListener("change", function() { caHost = this.value; loadContentAudit(); });
  const kw = $("caKw");
  if (kw) kw.addEventListener("input", function() {
    clearTimeout(caSearchT);
    const v = this.value;
    caSearchT = setTimeout(() => { caKw = v; loadContentAudit(); }, 300);
  });
}

window.loadContentAudit = function() {
  const host = caHost || ($("caHostSelect") || {}).value;
  if (!host) return;
  const body = $("caBody");
  if (body) body.innerHTML = `<div class="loading-dots">${I18N.t("common.loading") || "加载中..."}</div>`;
  const filter = caKw.trim() ? ("kw:" + caKw.trim()) : "";
  fetch(`/api/v1/content-audit?host=${encodeURIComponent(host)}&filter=${encodeURIComponent(filter)}&limit=200`, { credentials: "same-origin" })
    .then(r => r.json())
    .then(d => renderCA(body, d.events || []))
    .catch(() => { if (body) body.innerHTML = `<div class="empty-state">${I18N.t("netflow.load_error") || "加载失败"}</div>`; });
};

function renderCA(container, events) {
  if (!container) return;
  if (events.length === 0) {
    container.innerHTML = `<div class="empty-state">${I18N.t("ca.empty") || "暂无内容审计记录（需在 agent 配置 content_audit: true，且目标为明文 HTTP 流量）"}</div>`;
    return;
  }
  let html = `<div class="nf-table-wrap"><table class="nf-flow-table">`;
  html += `<thead><tr>`;
  html += `<th>${I18N.t("ca.time") || "时间"}</th>`;
  html += `<th>${I18N.t("netflow.src_ip") || "源IP"}</th>`;
  html += `<th>${I18N.t("ca.dest") || "目的（域名/端点）"}</th>`;
  html += `<th>${I18N.t("ca.method") || "方法"}</th>`;
  html += `<th>${I18N.t("ca.status") || "状态"}</th>`;
  html += `<th>${I18N.t("ca.req_body") || "请求（prompt）"}</th>`;
  html += `<th>${I18N.t("ca.resp_body") || "响应（completion）"}</th>`;
  html += `</tr></thead><tbody>`;
  const cell = (text, trunc) => {
    const s = (text || "").slice(0, 2000);
    if (!s) return `<span style="color:var(--muted)">—</span>`;
    return `<div style="max-height:160px;overflow:auto;white-space:pre-wrap;word-break:break-all;font-family:var(--mono,ui-monospace,monospace);font-size:12px">${esc(s)}${(text || "").length > 2000 ? " …" : ""}${trunc ? `<span style="color:#e0a300"> [${esc(I18N.t("ca.truncated") || "已截断")}]</span>` : ""}</div>`;
  };
  events.forEach(e => {
    html += `<tr>`;
    html += `<td style="white-space:nowrap">${esc(caTime(e.observed_at))}</td>`;
    html += `<td>${esc(e.src_ip || "")}</td>`;
    html += `<td>${esc(e.host || e.dst_ip || "")}${e.dst_port ? `<span style="color:var(--muted)">:${e.dst_port}</span>` : ""}${e.path ? `<div style="color:var(--muted);font-size:11px;word-break:break-all">${esc(e.path)}</div>` : ""}</td>`;
    html += `<td>${esc(e.method || "")}</td>`;
    html += `<td>${e.status ? esc(String(e.status)) : "—"}</td>`;
    html += `<td>${cell(e.body, e.req_truncated)}</td>`;
    html += `<td>${cell(e.resp_body, e.resp_truncated)}</td>`;
    html += `</tr>`;
  });
  html += `</tbody></table></div>`;
  container.innerHTML = html;
}

function caTime(s) {
  if (!s) return "";
  const d = new Date(s);
  return isNaN(d) ? "" : d.toLocaleString();
}

// 事件委托（CSP script-src 'self'，禁内联 onclick）。
safeAddEventListener("contentAuditPanel", "click", e => {
  const b = e.target.closest("[data-caact]");
  if (!b) return;
  // 刷新：连「有数据的主机」列表一起重拉（否则新产生审计的主机不会出现在下拉里）。
  if (b.dataset.caact === "refresh") { caDataHosts = null; renderContentAuditPanel(); }
});

if (typeof window._pageRenderers === "undefined") window._pageRenderers = {};
// 每次进入「内容审计」子标签都重拉有数据的主机（数据集会随时间变化）。
window._pageRenderers["content-audit"] = function() { caDataHosts = null; renderContentAuditPanel(); };

})();
