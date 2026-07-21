// content-audit.js — 明文 HTTP 内容审计面板（Phase 2）。并入「网络」父菜单的「内容审计」子标签。
// 展示 agent 抓到的明文 HTTP 请求：谁(源IP) 向 哪个端点(域名+路径) 发了什么(方法/内容/prompt)。
// ⚠ 高敏感：仅在 agent 显式开启 content_audit 时才有数据；面板顶部常驻合规提示。
(function() {
"use strict";

let caHost = "";
let caKw = "";
let caSearchT = null;
let caSensOnly = false;   // 只看命中敏感的
let caLastEvents = null;  // 上次加载的事件（供 AI 研判 + 客户端敏感过滤）
let caPage = 1, caSize = 20; // 明细分页（客户端）
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

  // host_id → hostname 映射（API 已 annotate；_cachedHosts 仅兜底）。
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
    const name = dh.hostname || h.hostname || dh.host_id;
    const sel = dh.host_id === caHost ? " selected" : "";
    html += `<option value="${esc(dh.host_id)}"${sel}>${esc(name)} · ${dh.events || 0}</option>`;
  });
  html += `</select>`;
  html += `<input type="search" id="caKw" class="nf-input" value="${esc(caKw)}" placeholder="${esc(I18N.t("ca.kw_ph") || "搜索 域名 / 路径 / 内容关键字")}">`;
  html += `<label class="nf-chk" style="display:inline-flex;align-items:center;gap:4px"><input type="checkbox" id="caSensOnly"${caSensOnly ? " checked" : ""}> ${esc(I18N.t("ca.sens_only") || "只看敏感")}</label>`;
  html += `<button class="nf-btn" data-caact="refresh">${I18N.t("common.refresh") || "刷新"}</button>`;
  html += `<button class="nf-btn nf-ai-btn" data-caact="ai" title="${esc(I18N.t("ca.ai_hint") || "AI 研判：是否有敏感数据外泄到大模型")}">🤖 ${esc(I18N.t("ai.analyze") || "AI 分析")}</button>`;
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
  const so = $("caSensOnly");
  so && so.addEventListener("change", function() {
    caSensOnly = this.checked;
    caPage = 1;
    // 客户端过滤即可（数据已在手），无需重查。
    if (caLastEvents) renderCA($("caBody"), caLastEvents);
  });
  // 分页控件（客户端）：上一页/下一页 + 每页条数。委托在 #caBody 上，renderCA 只换其 innerHTML，监听器常驻。
  const body = $("caBody");
  if (body) {
    body.addEventListener("click", function(e) {
      const b = e.target.closest("[data-pg]"); if (!b) return;
      if (b.dataset.pg === "prev") caPage--; else if (b.dataset.pg === "next") caPage++;
      if (caLastEvents) renderCA(body, caLastEvents);
    });
    body.addEventListener("change", function(e) {
      if (e.target.dataset && e.target.dataset.pg === "size") { caSize = +e.target.value || 20; caPage = 1; if (caLastEvents) renderCA(body, caLastEvents); }
    });
  }
}

window.loadContentAudit = function() {
  const host = caHost || ($("caHostSelect") || {}).value;
  if (!host) return;
  const body = $("caBody");
  if (body) body.innerHTML = `<div class="loading-dots">${I18N.t("common.loading") || "加载中..."}</div>`;
  const filter = caKw.trim() ? ("kw:" + caKw.trim()) : "";
  caPage = 1; // 新数据回到第一页
  fetch(`/api/v1/content-audit?host=${encodeURIComponent(host)}&filter=${encodeURIComponent(filter)}&limit=500`, { credentials: "same-origin" })
    .then(r => r.json())
    .then(d => { caLastEvents = d.events || []; renderCA(body, caLastEvents); })
    .catch(() => { if (body) body.innerHTML = `<div class="empty-state">${I18N.t("netflow.load_error") || "加载失败"}</div>`; });
};

function renderCA(container, events) {
  if (!container) return;
  if (caSensOnly) events = events.filter(e => e.sensitive); // 客户端"只看敏感"过滤
  if (events.length === 0) {
    container.innerHTML = `<div class="empty-state">${caSensOnly ? (I18N.t("ca.no_sens") || "无命中敏感数据的记录") : (I18N.t("ca.empty") || "暂无内容审计记录（需在 agent 配置 content_audit: true，且目标为明文 HTTP 流量）")}</div>`;
    return;
  }
  const total = events.length;
  caPage = tblClampPage(caPage, total, caSize);
  const pageEvents = events.slice((caPage - 1) * caSize, caPage * caSize);
  let html = `<div class="nf-table-wrap"><table class="nf-flow-table">`;
  html += `<thead><tr>`;
  html += `<th>${I18N.t("ca.time") || "时间"}</th>`;
  html += `<th>${I18N.t("ca.sensitive") || "敏感"}</th>`;
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
    return `<div class="ca-body-cell">${esc(s)}${(text || "").length > 2000 ? " …" : ""}${trunc ? `<span style="color:#e0a300"> [${esc(I18N.t("ca.truncated") || "已截断")}]</span>` : ""}</div>`;
  };
  // 仅响应行（抓包起于连接中途/请求丢包 → 只捕到响应）：方法/请求为空，标注清楚不是"没内容"的 bug。
  const promptCell = (e) => {
    if ((e.body || "").length) return cell(e.body, e.req_truncated);
    if (!e.method && e.status) return `<span style="color:var(--muted)">（请求未捕获·仅响应）</span>`;
    return `<span style="color:var(--muted)">—</span>`;
  };
  pageEvents.forEach(e => {
    html += `<tr${e.sensitive ? ` style="background:rgba(224,77,90,.06)"` : ""}>`;
    html += `<td style="white-space:nowrap">${esc(caTime(e.observed_at))}</td>`;
    html += `<td>${e.sensitive ? `<span style="display:inline-block;padding:1px 6px;border-radius:6px;background:#e04d5a;color:#fff;font-size:11px;white-space:nowrap" title="${esc(e.sensitive)}">⚠ ${esc(e.sensitive)}</span>` : `<span style="color:var(--muted)">—</span>`}</td>`;
    html += `<td class="nf-mono">${esc(e.src_ip || "")}</td>`;
    html += `<td class="nf-mono">${esc(e.host || e.dst_ip || "")}${e.dst_port ? `<span style="color:var(--muted)">:${e.dst_port}</span>` : ""}${e.path ? `<div style="color:var(--muted);font-size:11px;word-break:break-all;font-family:ui-monospace,monospace">${esc(e.path)}</div>` : ""}</td>`;
    html += `<td>${e.method ? `<span class="nf-proto nf-proto-tcp">${esc(e.method)}</span>` : `<span class="nf-proto" style="background:var(--bg3);color:var(--muted)">仅响应</span>`}</td>`;
    html += `<td class="nf-num">${e.status ? esc(String(e.status)) : "—"}</td>`;
    html += `<td>${promptCell(e)}</td>`;
    html += `<td>${cell(e.resp_body, e.resp_truncated)}</td>`;
    html += `</tr>`;
  });
  html += `</tbody></table></div>`;
  html += tblPager(total, caPage, caSize);
  container.innerHTML = html;
}

function caTime(s) {
  if (!s) return "";
  const d = new Date(s);
  return isNaN(d) ? "" : d.toLocaleString();
}

// caToText 把当前主机的内容审计记录汇成纯文本，供 AI 研判"是否有敏感数据外泄到大模型"。
function caToText() {
  const evs = caLastEvents || [];
  if (!evs.length) return "（当前主机暂无内容审计记录）";
  const nameMap = {};
  (window._cachedHosts || []).forEach(h => { nameMap[h.id] = h; });
  const fromAPI = (caDataHosts || []).find(h => h.host_id === caHost);
  const hn = (fromAPI && fromAPI.hostname) || (nameMap[caHost] || {}).hostname || caHost || "?";
  const sensN = evs.filter(e => e.sensitive).length;
  const lines = [`主机：${hn}　内容审计记录 ${evs.length} 条（其中内置规则命中敏感 ${sensN} 条）。含用户向 HTTP 端点(多为大模型)发的请求 prompt 与响应 completion：`];
  evs.slice(0, 30).forEach((e, i) => {
    lines.push(`\n[${i + 1}] ${e.src_ip || "?"} → ${e.host || e.dst_ip || "?"}${e.path || ""} ${e.method || ""} ${e.status || ""}${e.sensitive ? `　⚠敏感命中: ${e.sensitive}` : ""}`);
    if (e.body) lines.push(`  请求: ${String(e.body).slice(0, 800)}`);
    if (e.resp_body) lines.push(`  响应: ${String(e.resp_body).slice(0, 800)}`);
  });
  return lines.join("\n").slice(0, 14000);
}

// caOpenAI 打开 AI 面板研判内容审计（学习闭环自动复用：/ai/assist 沉淀记忆 + 👍/👎）。
function caOpenAI() {
  if (typeof openAIAssist !== "function") { if (typeof toast === "function") toast(I18N.t("assist.unavailable", "AI 面板未就绪"), "err"); return; }
  if (!caLastEvents || !caLastEvents.length) { if (typeof toast === "function") toast(I18N.t("ca.empty", "暂无数据"), "err"); return; }
  openAIAssist({ task: "content_audit_diagnosis", mode: "analyze", title: I18N.t("ca.ai_title", "AI · 内容审计研判"), context: caToText() });
}

// 事件委托（CSP script-src 'self'，禁内联 onclick）。
safeAddEventListener("contentAuditPanel", "click", e => {
  const b = e.target.closest("[data-caact]");
  if (!b) return;
  // 刷新：连「有数据的主机」列表一起重拉（否则新产生审计的主机不会出现在下拉里）。
  if (b.dataset.caact === "refresh") { caDataHosts = null; renderContentAuditPanel(); }
  else if (b.dataset.caact === "ai") caOpenAI();
});

if (typeof window._pageRenderers === "undefined") window._pageRenderers = {};
// 每次进入「内容审计」子标签都重拉有数据的主机（数据集会随时间变化）。
window._pageRenderers["content-audit"] = function() { caDataHosts = null; renderContentAuditPanel(); };

})();
