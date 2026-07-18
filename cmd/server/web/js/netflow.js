// netflow.js — 网络流量面板 (Network Traffic Panel)
// Loaded as part of the unified app.js bundle.

(function() {
"use strict";

let nfCurrentHost = "";
let nfCurrentRange = "1h";
let nfHostQuery = "";       // 主机搜索词
let nfDimension = "dst_ip"; // Top-N 聚合维度（后端支持多种，之前前端写死了）
let nfSearchT = null;
// 「只列有流量的主机」：从 /api/v1/netflow/hosts 拉在所选时间窗内产生过 flow 的主机，
// 按字节降序（大流量在前）。null=未加载。换时间范围/刷新/进入视图时重新拉。
let nfTrafficHosts = null;
let nfLastSummary = null;  // 上次加载的 Top-N 汇总（{rows,dimension}），供 AI 分析
let nfLastFlows = null;    // 上次加载的 Flow 明细，供 AI 分析

function renderNetFlowPanel() {
  const container = $("netflowPanel");
  if (!container) return;

  // 先拉「有流量的主机」列表，再渲染面板。避免把成百上千台无流量主机塞进下拉。
  if (nfTrafficHosts === null) {
    container.innerHTML = `<div class="loading-dots">${I18N.t("common.loading") || "加载中..."}</div>`;
    fetch(`/api/v1/netflow/hosts?range=${encodeURIComponent(nfCurrentRange)}`, { credentials: "same-origin" })
      .then(r => r.json())
      .then(d => { nfTrafficHosts = d.hosts || []; renderNetFlowPanel(); })
      .catch(() => { nfTrafficHosts = []; renderNetFlowPanel(); });
    return;
  }

  const q = nfHostQuery.trim().toLowerCase();
  // 主机名/IP 映射：有流量主机只带 host_id，用 _cachedHosts 补 hostname/ip 展示。
  const nameMap = {};
  (window._cachedHosts || []).forEach(h => { nameMap[h.id] = h; });

  let html = `<div class="nf-toolbar">`;
  html += `<input type="search" id="nfHostSearch" class="nf-input" value="${esc(nfHostQuery)}"
    placeholder="${esc(I18N.t("netflow.search_ph") || "搜索主机")}">`;
  html += `<select id="nfHostSelect" class="nf-select">`;
  let shown = 0;
  nfTrafficHosts.forEach(th => {
    const h = nameMap[th.host_id] || {};
    const name = h.hostname || th.host_id;
    const hay = `${name} ${th.host_id} ${h.ip || ""}`.toLowerCase();
    if (q && !q.split(/\s+/).every(w => hay.includes(w))) return;
    shown++;
    const sel = th.host_id === nfCurrentHost ? " selected" : "";
    // 下拉直接标出流量量级，一眼看出谁是大流量主机
    html += `<option value="${esc(th.host_id)}"${sel}>${esc(name)} · ${formatBytes(Number(th.bytes) || 0)}</option>`;
  });
  html += `</select>`;
  html += `<select id="nfRangeSelect" class="nf-select">`;
  [["1h", "最近1小时"], ["6h", "最近6小时"], ["24h", "最近24小时"], ["7d", "最近7天"]].forEach(([v, fb]) => {
    html += `<option value="${v}"${v === nfCurrentRange ? " selected" : ""}>${I18N.t("netflow.last_" + v) || fb}</option>`;
  });
  html += `</select>`;
  // 聚合维度：后端本来就支持，之前前端写死了 src_ip，等于把能力藏起来了
  html += `<select id="nfDimSelect" class="nf-select" title="${esc(I18N.t("netflow.dimension") || "聚合维度")}">`;
  [["dst_ip", "netflow.dst_ip", "目的IP"], ["src_ip", "netflow.src_ip", "源IP"],
   ["dst_port", "netflow.dst_port", "目的端口"], ["src_port", "netflow.src_port", "源端口"],
   ["protocol", "netflow.protocol", "协议"]].forEach(([v, k, fb]) => {
    html += `<option value="${v}"${v === nfDimension ? " selected" : ""}>${esc(I18N.t(k) || fb)}</option>`;
  });
  html += `</select>`;
  html += `<button class="nf-btn" data-nfact="refresh">${I18N.t("common.refresh") || "刷新"}</button>`;
  html += `<button class="nf-btn nf-ai-btn" data-nfact="ai" title="${esc(I18N.t("netflow.ai_hint", "AI 分析该主机流量并沉淀记忆"))}">🤖 ${esc(I18N.t("ai.analyze", "AI 分析"))}</button>`;
  html += `</div>`;

  if (nfTrafficHosts.length === 0) {
    container.innerHTML = html + `<div class="empty-state">${I18N.t("netflow.no_traffic_hosts") || "所选时间范围内没有产生流量的主机"}</div>`;
    nfBindToolbar();
    return;
  }
  if (shown === 0) {
    container.innerHTML = html + `<div class="empty-state">${I18N.t("empty.no_host_match2") || "没有匹配的主机"}</div>`;
    nfBindToolbar();
    return;
  }

  html += `<div id="nfContent" class="nf-content">`;
  html += `<div id="nfSummary" class="nf-section"><h3>${I18N.t("netflow.top_talkers") || "流量排行"}</h3><div id="nfSummaryBody"></div></div>`;
  html += `<div id="nfFlows" class="nf-section"><h3>${I18N.t("netflow.flow_detail") || "Flow 明细"}</h3><div id="nfFlowsBody"></div></div>`;
  html += `</div>`;

  container.innerHTML = html;

  // 之前选中的主机若还在筛选结果里就保持不变，否则退回第一个（流量最大的）——
  // 不然每次输入搜索词都会把选中的主机跳走。
  const sel = $("nfHostSelect");
  if (sel && sel.options.length > 0) {
    if (![...sel.options].some(o => o.value === nfCurrentHost)) nfCurrentHost = sel.options[0].value;
    sel.value = nfCurrentHost;
  }
  nfBindToolbar();
  if (nfCurrentHost) loadNetFlowData();
}

// nfBindToolbar 绑定工具栏事件。工具栏每次重渲染都会被替换掉，所以必须重新绑。
function nfBindToolbar() {
  const sel = $("nfHostSelect");
  sel && sel.addEventListener("change", function() { nfCurrentHost = this.value; loadNetFlowData(); });
  const rng = $("nfRangeSelect");
  rng && rng.addEventListener("change", function() {
    nfCurrentRange = this.value;
    nfTrafficHosts = null; // 换时间范围 → 重新拉「有流量的主机」（不同窗口主机集不同）
    renderNetFlowPanel();
  });
  const dim = $("nfDimSelect");
  dim && dim.addEventListener("change", function() { nfDimension = this.value; loadNetFlowData(); });

  const search = $("nfHostSearch");
  if (search) {
    search.addEventListener("input", function() {
      // 防抖 + 还原焦点：重渲染会让输入框失焦，否则一次只能敲一个字
      clearTimeout(nfSearchT);
      const v = this.value;
      nfSearchT = setTimeout(() => {
        nfHostQuery = v;
        renderNetFlowPanel();
        const s = $("nfHostSearch");
        if (s) { s.focus(); s.setSelectionRange(s.value.length, s.value.length); }
      }, 200);
    });
  }
}

window.loadNetFlowData = function() {
  const host = nfCurrentHost || ($("nfHostSelect") || {}).value;
  const range = nfCurrentRange || "1h";
  if (!host) return;

  const summaryBody = $("nfSummaryBody");
  const flowsBody = $("nfFlowsBody");
  if (summaryBody) summaryBody.innerHTML = `<div class="loading-dots">${I18N.t("common.loading") || "加载中..."}</div>`;
  if (flowsBody) flowsBody.innerHTML = "";

  // Fetch Top-N summary
  Promise.all([
    fetch(`/api/v1/netflow/summary?host=${encodeURIComponent(host)}&range=${range}&dimension=${encodeURIComponent(nfDimension)}&top=10`, { credentials: "same-origin" }).then(r => r.json()),
    fetch(`/api/v1/netflow/flows?host=${encodeURIComponent(host)}&limit=100`, { credentials: "same-origin" }).then(r => r.json()),
  ]).then(([sumData, flowData]) => {
    nfLastSummary = { rows: sumData.summary || [], dimension: sumData.dimension || nfDimension };
    nfLastFlows = flowData.flows || [];
    renderNfSummary(summaryBody, sumData.summary || [], sumData.dimension || nfDimension);
    renderNfFlows(flowsBody, flowData.flows || []);
  }).catch(() => {
    if (summaryBody) summaryBody.innerHTML = `<div class="empty-state">${I18N.t("netflow.load_error") || "加载失败"}</div>`;
  });
};

function renderNfSummary(container, summary, dimension) {
  if (!container) return;
  if (summary.length === 0) {
    container.innerHTML = `<div class="empty-state">${I18N.t("netflow.no_data") || "暂无流量数据"}</div>`;
    return;
  }

  // Render as horizontal bar chart
  const maxBytes = summary[0].bytes || 1;
  let html = `<table class="nf-summary-table">`;
  html += `<tr><th>${I18N.t("netflow." + dimension) || dimension}</th><th>${I18N.t("netflow.bytes") || "流量"}</th><th></th></tr>`;
  summary.forEach(item => {
    const pct = Math.max(2, (item.bytes / maxBytes) * 100);
    html += `<tr>`;
    html += `<td class="nf-label">${esc(item.key)}</td>`;
    html += `<td class="nf-value">${formatBytes(item.bytes)}</td>`;
    html += `<td class="nf-bar-cell"><div class="nf-bar" style="width:${pct}%"></div></td>`;
    html += `</tr>`;
  });
  html += `</table>`;
  container.innerHTML = html;
}

function renderNfFlows(container, flows) {
  if (!container) return;
  if (flows.length === 0) {
    container.innerHTML = `<div class="empty-state">${I18N.t("netflow.no_flows") || "暂无 Flow 记录"}</div>`;
    return;
  }

  let html = `<div class="nf-flows-toolbar">`;
  html += `<input id="nfFilterInput" type="text" class="nf-input" placeholder="${I18N.t("netflow.filter_placeholder") || "筛选: src_ip:10.0.0.1 或 dst_port:443"}">`;
  html += `<button class="nf-btn" data-nfact="filter">${I18N.t("netflow.filter") || "筛选"}</button>`;
  html += `<button class="nf-btn" data-nfact="export">${I18N.t("netflow.export_csv") || "导出 CSV"}</button>`;
  html += `</div>`;

  html += `<div class="nf-table-wrap"><table class="nf-flow-table">`;
  html += `<thead><tr>`;
  html += `<th>${I18N.t("netflow.source") || "来源"}</th>`;
  html += `<th>${I18N.t("netflow.src_ip") || "源IP"}</th>`;
  html += `<th>${I18N.t("netflow.src_port") || "源端口"}</th>`;
  html += `<th>${I18N.t("netflow.dst_ip") || "目的IP"}</th>`;
  html += `<th>${I18N.t("netflow.dst_port") || "目的端口"}</th>`;
  html += `<th>${I18N.t("netflow.proto") || "协议"}</th>`;
  html += `<th>${I18N.t("netflow.bytes") || "字节"}</th>`;
  html += `<th>${I18N.t("netflow.packets") || "包"}</th>`;
  html += `<th>${I18N.t("netflow.avg_pkt") || "平均包长"}</th>`;
  html += `<th>${I18N.t("netflow.duration") || "时长"}</th>`;
  html += `<th>${I18N.t("netflow.last_seen") || "最后活跃"}</th>`;
  html += `</tr></thead><tbody>`;

  flows.forEach(f => {
    const protoName = protoNameMap(f.protocol);
    const bytes = Number(f.bytes) || 0, pkts = Number(f.packets) || 0;
    const avgPkt = pkts > 0 ? Math.round(bytes / pkts) : 0; // 平均包长，辅助识别小包攻击/大流传输
    const dur = nfDurationSec(f);
    html += `<tr>`;
    html += `<td><span class="nf-badge nf-badge-${f.source}">${f.source}</span></td>`;
    html += `<td>${esc(f.src_ip || "")}</td>`;
    html += `<td>${f.src_port || ""}</td>`;
    html += `<td>${esc(f.dst_ip || "")}</td>`;
    html += `<td>${f.dst_port || ""}</td>`;
    html += `<td>${protoName}</td>`;
    html += `<td>${formatBytes(bytes)}</td>`;
    html += `<td>${pkts}</td>`;
    html += `<td>${avgPkt ? avgPkt + " B" : "-"}</td>`;
    html += `<td>${dur === "" ? "-" : dur + " s"}</td>`;
    html += `<td>${esc(nfShortTime(f.last_seen))}</td>`;
    html += `</tr>`;
  });
  html += `</tbody></table></div>`;
  container.innerHTML = html;

  // Store flows for CSV export
  window._nfFlowsCache = flows;
}

window.applyNfFilter = function() {
  const filter = ($("nfFilterInput") || {}).value || "";
  if (!filter) { loadNetFlowData(); return; }
  const host = nfCurrentHost || ($("nfHostSelect") || {}).value;
  if (!host) return;

  fetch(`/api/v1/netflow/flows?host=${encodeURIComponent(host)}&filter=${encodeURIComponent(filter)}&limit=200`, { credentials: "same-origin" })
    .then(r => r.json())
    .then(data => renderNfFlows($("nfFlowsBody"), data.flows || []))
    .catch(() => {});
};

window.exportNfCSV = function() {
  const flows = window._nfFlowsCache || [];
  if (flows.length === 0) return;

  let csv = "source,src_ip,src_port,dst_ip,dst_port,protocol,bytes,packets,first_seen,last_seen\n";
  flows.forEach(f => {
    csv += `${f.source},${f.src_ip},${f.src_port},${f.dst_ip},${f.dst_port},${f.protocol},${f.bytes},${f.packets},${f.first_seen || ""},${f.last_seen || ""}\n`;
  });

  const blob = new Blob([csv], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `netflow_flows_${new Date().toISOString().slice(0, 10)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
};

function protoNameMap(proto) {
  switch (proto) {
    case 1: return "ICMP";
    case 6: return "TCP";
    case 17: return "UDP";
    default: return proto;
  }
}

// nfDurationSec 计算一条 flow 的持续秒数（last_seen - first_seen），无效则返回 ""。
function nfDurationSec(f) {
  if (!f.first_seen || !f.last_seen) return "";
  const d = (new Date(f.last_seen) - new Date(f.first_seen)) / 1000;
  return (isNaN(d) || d < 0) ? "" : Math.round(d);
}

// nfShortTime 把 ISO 时间串格式化为本地时分秒；无效返回 ""。
function nfShortTime(s) {
  if (!s) return "";
  const d = new Date(s);
  return isNaN(d) ? "" : d.toLocaleTimeString();
}

function formatBytes(bytes) {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return (bytes / Math.pow(1024, i)).toFixed(1) + " " + units[i];
}

// nfDimLabel 把聚合维度 code 映射为可读标签（供 AI 上下文）。
function nfDimLabel(dim) {
  return ({ dst_ip: "目的IP", src_ip: "源IP", dst_port: "目的端口", src_port: "源端口", protocol: "协议" })[dim] || dim;
}

// netflowToText 把当前主机的流量快照（Top-N 汇总 + Top Flow 明细）汇总为纯文本，供 AI 分析。
function netflowToText() {
  const nameMap = {};
  (window._cachedHosts || []).forEach(h => { nameMap[h.id] = h; });
  const hn = (nameMap[nfCurrentHost] || {}).hostname || nfCurrentHost || "?";
  const sum = (nfLastSummary && nfLastSummary.rows) || [];
  const flows = nfLastFlows || [];
  if (!sum.length && !flows.length) return "（当前主机在所选时间范围内暂无流量数据）";
  const lines = [`主机：${hn}（${nfCurrentHost}）　时间范围：${nfCurrentRange}　聚合维度：${nfDimLabel(nfDimension)}`];
  if (sum.length) {
    lines.push(`\n# Top ${sum.length} ${nfDimLabel((nfLastSummary && nfLastSummary.dimension) || nfDimension)}（按流量降序）`);
    sum.forEach((it, i) => lines.push(`  ${i + 1}. ${it.key} = ${formatBytes(Number(it.bytes) || 0)}`));
  }
  if (flows.length) {
    const top = flows.slice().sort((a, b) => (Number(b.bytes) || 0) - (Number(a.bytes) || 0)).slice(0, 40);
    lines.push(`\n# Top ${top.length} Flow（共 ${flows.length} 条，按字节降序）`);
    top.forEach(f => {
      const pkts = Number(f.packets) || 0, bytes = Number(f.bytes) || 0;
      const avg = pkts > 0 ? Math.round(bytes / pkts) : 0;
      const dur = nfDurationSec(f);
      lines.push(`  - ${f.src_ip || "?"}:${f.src_port || "?"} → ${f.dst_ip || "?"}:${f.dst_port || "?"} ${protoNameMap(f.protocol)} ${formatBytes(bytes)} ${pkts}包 均包${avg}B 时长${dur === "" ? "-" : dur + "s"}`);
    });
  }
  return lines.join("\n").slice(0, 12000);
}

// nfOpenAI 打开 AI 面板对当前主机流量做整体研判（学习闭环自动复用：/ai/assist 沉淀记忆 + 👍/👎）。
function nfOpenAI() {
  if (typeof openAIAssist !== "function") { if (typeof toast === "function") toast(I18N.t("assist.unavailable", "AI 面板未就绪"), "err"); return; }
  if (!nfCurrentHost) { if (typeof toast === "function") toast(I18N.t("netflow.no_data", "暂无流量数据"), "err"); return; }
  openAIAssist({ task: "netflow_diagnosis", mode: "analyze", title: I18N.t("assist.title_netflow", "AI · 流量分析"), context: netflowToText() });
}

// Register with navigation
// 事件委托：CSP 为 script-src 'self'，内联 onclick 会被浏览器拦截；且这些函数在 IIFE 内、
// 不挂 window，内联写法即便没有 CSP 也会 ReferenceError。刷新/筛选/导出此前因此全是死按钮。
safeAddEventListener("netflowPanel", "click", e => {
  const b = e.target.closest("[data-nfact]");
  if (!b) return;
  // 刷新：连「有流量的主机」列表一起重拉（否则新上流量的主机不会出现在下拉里）
  if (b.dataset.nfact === "refresh") { nfTrafficHosts = null; renderNetFlowPanel(); }
  else if (b.dataset.nfact === "filter") applyNfFilter();
  else if (b.dataset.nfact === "export") exportNfCSV();
  else if (b.dataset.nfact === "ai") nfOpenAI();
});

if (typeof window._pageRenderers === "undefined") window._pageRenderers = {};
// 每次进入「网络」视图都重拉有流量的主机（时间窗内的流量主机集会变化）。
window._pageRenderers.netflow = function() { nfTrafficHosts = null; renderNetFlowPanel(); };

})();
