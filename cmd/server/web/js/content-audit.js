// content-audit.js — 跨平台 HTTP/LLM 内容审计面板。
// 支持端侧 metadata/redacted/full 数据策略；HTTPS 仅展示可见元数据，不暗示已解密。
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
  return `<div style="margin:0 0 10px;padding:9px 12px;border:1px solid var(--line);border-left:3px solid #e0a300;border-radius:8px;background:rgba(224,163,0,.08);font-size:12px;color:var(--muted)">${esc(I18N.t("ca.notice") || "⚠ 内容审计仅可在已授权网络启用。推荐使用端侧脱敏或仅元数据模式；完整正文可能包含 PII / prompt。HTTPS 正文仍需在 LLM Gateway/SDK 层审计；服务端记录按保留策略自动清理。")}</div>`;
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
    container.innerHTML = html + `<div class="empty-state">${I18N.t("ca.no_hosts") || "暂无有内容审计数据的主机（需启用 Agent 内容审计并产生匹配白名单的 HTTP 流量）"}</div>`;
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
  html += `<input type="search" id="caKw" class="nf-input" value="${esc(caKw)}" placeholder="${esc(I18N.t("ca.kw_ph") || "搜索关键字，或 provider:/model:/principal:/risk:")}">`;
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
  const rawFilter = caKw.trim();
  const advanced = /^(src_ip|dst_ip|host|method|protocol|backend|body_mode|provider|model|principal|decision|risk|sens):/i.test(rawFilter);
  const filter = rawFilter ? (advanced ? rawFilter : ("kw:" + rawFilter)) : "";
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
    container.innerHTML = `<div class="empty-state">${caSensOnly ? (I18N.t("ca.no_sens") || "无命中敏感数据的记录") : (I18N.t("ca.empty") || "暂无内容审计记录（请检查采集后端、权限、端口及域名白名单）")}</div>`;
    return;
  }
  const total = events.length;
  const llmCount = events.filter(e => e.is_llm).length;
  const sensitiveCount = events.filter(e => e.sensitive).length;
  const blockedCount = events.filter(e => /^(deny|block)$/i.test(e.policy_decision || "")).length;
  const tokenTotal = events.reduce((n, e) => n + Number(e.llm_input_tokens || 0) + Number(e.llm_output_tokens || 0), 0);
  caPage = tblClampPage(caPage, total, caSize);
  const pageEvents = events.slice((caPage - 1) * caSize, caPage * caSize);
  let html = `<div class="stats-grid" style="margin-bottom:12px">`;
  html += `<div class="stat-card"><div class="sv">${esc(String(total))}</div><div class="sk">${esc(I18N.t("ca.audit_events") || "审计事件")}</div><div class="sh">${esc(I18N.t("ca.query_window") || "当前查询窗口")}</div></div>`;
  html += `<div class="stat-card"><div class="sv ok">${esc(String(llmCount))}</div><div class="sk">${esc(I18N.t("ca.llm_calls") || "LLM 调用")}</div><div class="sh">${esc(I18N.t("ca.identified") || "结构化或端点识别")}</div></div>`;
  html += `<div class="stat-card"><div class="sv ${sensitiveCount ? "crit" : "ok"}">${esc(String(sensitiveCount))}</div><div class="sk">${esc(I18N.t("ca.sensitive_hits") || "敏感命中")}</div><div class="sh">${esc(I18N.t("ca.blocked") || "策略阻断")} ${esc(String(blockedCount))}</div></div>`;
  html += `<div class="stat-card"><div class="sv">${esc(tokenTotal.toLocaleString())}</div><div class="sk">${esc(I18N.t("ca.token_total") || "Token 总量")}</div><div class="sh">input + output</div></div>`;
  html += `</div><div class="nf-table-wrap"><table class="nf-flow-table">`;
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
    if (e.body_mode === "metadata") return `<span style="color:var(--muted)">仅元数据 · ${esc(String(e.req_bytes || 0))} B${e.req_sha256 ? " · SHA-256 " + esc(e.req_sha256.slice(0, 10)) + "…" : ""}</span>`;
    if (!e.method && e.status) return `<span style="color:var(--muted)">（请求未捕获·仅响应）</span>`;
    return `<span style="color:var(--muted)">—</span>`;
  };
  const responseCell = (e) => {
    if ((e.resp_body || "").length) return cell(e.resp_body, e.resp_truncated);
    if (e.body_mode === "metadata") return `<span style="color:var(--muted)">仅元数据 · ${esc(String(e.resp_bytes || 0))} B${e.resp_sha256 ? " · SHA-256 " + esc(e.resp_sha256.slice(0, 10)) + "…" : ""}</span>`;
    return `<span style="color:var(--muted)">—</span>`;
  };
  // data-ca-idx 指向「过滤后」列表下标，与 openCADetail 取数一致。
  const baseIdx = (caPage - 1) * caSize;
  pageEvents.forEach((e, i) => {
    const idx = baseIdx + i;
    html += `<tr class="ca-row"${e.sensitive ? ` style="background:rgba(224,77,90,.06)"` : ""} data-ca-idx="${idx}" title="${esc(I18N.t("ca.click_detail") || "点击查看详情")}">`;
    html += `<td style="white-space:nowrap">${esc(caTime(e.observed_at))}</td>`;
    html += `<td>${e.sensitive ? `<span style="display:inline-block;padding:1px 6px;border-radius:6px;background:#e04d5a;color:#fff;font-size:11px;white-space:nowrap" title="${esc(e.sensitive)}">⚠ ${esc(e.sensitive)}</span>` : `<span style="color:var(--muted)">—</span>`}</td>`;
    html += `<td class="nf-mono">${esc(e.src_ip || "")}</td>`;
    const decisionBadge = e.policy_decision ? ` · ${esc(e.policy_decision)}` : "";
    const llmBadge = e.is_llm ? `<div style="margin-top:3px"><span class="nf-proto nf-proto-tcp">LLM · ${esc(e.llm_provider || "compatible")}${e.llm_model ? " · " + esc(e.llm_model) : ""}${decisionBadge}</span></div>` : "";
    html += `<td class="nf-mono">${esc(e.host || e.dst_ip || "")}${e.dst_port ? `<span style="color:var(--muted)">:${e.dst_port}</span>` : ""}${e.path ? `<div style="color:var(--muted);font-size:11px;word-break:break-all;font-family:ui-monospace,monospace">${esc(e.path)}</div>` : ""}${llmBadge}</td>`;
    const methodBadge = e.protocol === "tls"
      ? `<span class="nf-proto" style="background:var(--bg3);color:var(--muted)">TLS 元数据</span>`
      : (e.method ? `<span class="nf-proto nf-proto-tcp">${esc(e.method)}</span>` : `<span class="nf-proto" style="background:var(--bg3);color:var(--muted)">仅响应</span>`);
    html += `<td>${methodBadge}<div style="margin-top:3px;color:var(--muted);font-size:10px">${esc(e.capture_backend || "legacy")} · ${esc(e.body_mode || "legacy")}</div></td>`;
    html += `<td class="nf-num">${e.status ? esc(String(e.status)) : "—"}</td>`;
    html += `<td>${promptCell(e)}</td>`;
    html += `<td>${responseCell(e)}</td>`;
    html += `</tr>`;
  });
  html += `</tbody></table></div>`;
  html += tblPager(total, caPage, caSize);
  container.innerHTML = html;
}

function caFilteredEvents() {
  const evs = caLastEvents || [];
  return caSensOnly ? evs.filter(e => e.sensitive) : evs;
}

function caKv(k, v) {
  return `<div class="ca-detail-kv"><span class="k">${esc(k)}</span><span class="v">${v}</span></div>`;
}

function caPreBlock(label, text, truncated) {
  const body = (text || "").length
    ? `<pre class="ca-detail-pre mono">${esc(text)}</pre>`
    : `<div class="ca-detail-empty">${esc(I18N.t("ca.no_content") || "无内容")}</div>`;
  const truncHint = truncated
    ? `<span class="ca-detail-trunc">[${esc(I18N.t("ca.truncated") || "已截断")}]</span>`
    : "";
  return `<div class="ca-detail-sec"><div class="ca-detail-sec-h">${esc(label)}${truncHint}</div>${body}</div>`;
}

// 打开内容审计详情弹窗：用列表缓存的完整字段，不再 2000 字截断。
function openCADetail(e) {
  if (!e) return;
  const titleEl = $("caDetailTitle");
  const bodyEl = $("caDetailBody");
  const mask = $("caDetailMask");
  if (!bodyEl || !mask) return;

  const dest = (e.host || e.dst_ip || "") + (e.dst_port ? (":" + e.dst_port) : "");
  const titleParts = [e.protocol === "tls" ? "TLS" : (e.method || (I18N.t("ca.resp_only") || "仅响应")), dest || "—", e.path || ""].filter(Boolean);
  if (titleEl) titleEl.textContent = titleParts.join(" ") || (I18N.t("ca.detail_title") || "内容审计详情");

  const endpoint = dest
    ? (e.path ? `${e.protocol === "tls" ? "https" : "http"}://${dest}${e.path}` : dest)
    : (e.path || "—");

  let meta = "";
  meta += caKv(I18N.t("ca.time") || "时间", esc(caTime(e.observed_at) || "—"));
  meta += caKv(I18N.t("netflow.src_ip") || "源IP", `<span class="nf-mono">${esc(e.src_ip || "—")}</span>`);
  meta += caKv(I18N.t("ca.dest") || "目的（域名/端点）", `<span class="nf-mono">${esc(endpoint)}</span>`);
  meta += caKv(I18N.t("ca.method") || "方法", e.protocol === "tls" ? "TLS ClientHello" : (e.method ? esc(e.method) : esc(I18N.t("ca.resp_only") || "仅响应")));
  meta += caKv("协议", esc(e.protocol || "unknown"));
  meta += caKv(I18N.t("ca.status") || "状态", e.status ? esc(String(e.status)) : "—");
  meta += caKv(I18N.t("ca.sensitive") || "敏感", e.sensitive
    ? `<span class="ca-sens-pill">⚠ ${esc(e.sensitive)}</span>`
    : `<span style="color:var(--muted)">—</span>`);
  meta += caKv(I18N.t("ca.ctype") || "类型", esc(e.ctype || "—"));
  meta += caKv(I18N.t("ca.resp_ctype") || "响应类型", esc(e.resp_ctype || "—"));
  meta += caKv("采集来源 / 正文策略", esc((e.capture_backend || "legacy") + " / " + (e.body_mode || "legacy")));
  meta += caKv("正文规模", `request ${esc(String(e.req_bytes || 0))} B · response ${esc(String(e.resp_bytes || 0))} B`);
  if (e.req_sha256 || e.resp_sha256) {
    meta += caKv("内容哈希", `<span class="nf-mono">req ${esc(e.req_sha256 || "—")}<br>resp ${esc(e.resp_sha256 || "—")}</span>`);
  }
  if (e.redaction_count) {
    meta += caKv("端侧脱敏", `${esc(String(e.redaction_count))} 处 · ${esc(e.redaction_labels || "—")}`);
  }
  if (e.principal_id || e.application_id) {
    meta += caKv("调用身份 / 应用", `${esc(e.principal_id || "—")} / ${esc(e.application_id || "—")}`);
  }
  if (e.request_id || e.trace_id) {
    meta += caKv("Request / Trace", `<span class="nf-mono">${esc(e.request_id || "—")}<br>${esc(e.trace_id || "—")}</span>`);
  }
  if (e.policy_decision || e.risk_labels) {
    meta += caKv("治理决策 / 风险", `${esc(e.policy_decision || "—")} / ${esc(e.risk_labels || "—")}`);
  }
  if (e.is_llm) {
    meta += caKv("LLM 提供方 / 模型", esc((e.llm_provider || "compatible") + (e.llm_model ? (" / " + e.llm_model) : "")));
    meta += caKv("LLM 操作", esc(e.llm_operation || "—") + (e.llm_stream ? " · stream" : ""));
    meta += caKv("内容规模", `prompt ${esc(String(e.llm_prompt_chars || 0))} chars · completion ${esc(String(e.llm_completion_chars || 0))} chars`);
    meta += caKv("Token 用量", `input ${esc(String(e.llm_input_tokens || 0))} · output ${esc(String(e.llm_output_tokens || 0))} · tool calls ${esc(String(e.llm_tool_calls || 0))}`);
    meta += caKv("端到端延迟", e.latency_ms ? `${esc(String(e.latency_ms))} ms` : "—");
  }

  bodyEl.innerHTML =
    `<div class="ca-detail-meta">${meta}</div>` +
    caPreBlock(I18N.t("ca.req_full") || "请求全文", e.body, e.req_truncated) +
    caPreBlock(I18N.t("ca.resp_full") || "响应全文", e.resp_body, e.resp_truncated);

  mask.classList.add("show");
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
  const lines = [`主机：${hn}　内容审计记录 ${evs.length} 条（其中敏感命中 ${sensN} 条）。记录可能采用仅元数据、端侧脱敏或完整正文策略：`];
  evs.slice(0, 30).forEach((e, i) => {
    lines.push(`\n[${i + 1}] ${e.src_ip || "?"} → ${e.host || e.dst_ip || "?"}${e.path || ""} ${e.method || ""} ${e.status || ""}${e.sensitive ? `　⚠敏感命中: ${e.sensitive}` : ""}`);
    if (e.body) lines.push(`  请求: ${String(e.body).slice(0, 800)}`);
    if (e.resp_body) lines.push(`  响应: ${String(e.resp_body).slice(0, 800)}`);
  });
  return lines.join("\n").slice(0, 14000);
}

// caOpenAI 打开 AI 面板研判内容审计；仅人工采纳/反馈后的结果进入学习闭环。
function caOpenAI() {
  if (typeof openAIAssist !== "function") { if (typeof toast === "function") toast(I18N.t("assist.unavailable", "AI 面板未就绪"), "err"); return; }
  if (!caLastEvents || !caLastEvents.length) { if (typeof toast === "function") toast(I18N.t("ca.empty", "暂无数据"), "err"); return; }
  openAIAssist({ task: "content_audit_diagnosis", mode: "analyze", title: I18N.t("ca.ai_title", "AI · 内容审计研判"), context: caToText() });
}

// 事件委托（CSP script-src 'self'，禁内联 onclick）。
safeAddEventListener("contentAuditPanel", "click", e => {
  const b = e.target.closest("[data-caact]");
  if (b) {
    // 刷新：连「有数据的主机」列表一起重拉（否则新产生审计的主机不会出现在下拉里）。
    if (b.dataset.caact === "refresh") { caDataHosts = null; renderContentAuditPanel(); }
    else if (b.dataset.caact === "ai") caOpenAI();
    return;
  }
  // 分页控件不打开详情
  if (e.target.closest(".tbl-pager")) return;
  const row = e.target.closest("tr.ca-row[data-ca-idx]");
  if (!row) return;
  const idx = parseInt(row.dataset.caIdx, 10);
  if (isNaN(idx)) return;
  const list = caFilteredEvents();
  openCADetail(list[idx]);
});

if (typeof window._pageRenderers === "undefined") window._pageRenderers = {};
// 每次进入「内容审计」子标签都重拉有数据的主机（数据集会随时间变化）。
window._pageRenderers["content-audit"] = function() { caDataHosts = null; renderContentAuditPanel(); };

})();
