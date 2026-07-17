// netflow.js — 网络流量面板 (Network Traffic Panel)
// Loaded as part of the unified app.js bundle.

(function() {
"use strict";

let nfCurrentHost = "";
let nfCurrentRange = "1h";

function renderNetFlowPanel() {
  const container = $("netflowPanel");
  if (!container) return;

  const hosts = (window._cachedHosts || []);
  if (hosts.length === 0) {
    container.innerHTML = `<div class="empty-state">${I18N.t("netflow.no_hosts") || "暂无主机"}</div>`;
    return;
  }

  // Build host selector + range selector + panels
  let html = `<div class="nf-toolbar">`;
  html += `<select id="nfHostSelect" class="nf-select">`;
  hosts.forEach(h => {
    if (h.online) {
      html += `<option value="${esc(h.id)}">${esc(h.hostname || h.id)}</option>`;
    }
  });
  html += `</select>`;
  html += `<select id="nfRangeSelect" class="nf-select">`;
  html += `<option value="1h">${I18N.t("netflow.last_1h") || "最近1小时"}</option>`;
  html += `<option value="6h">${I18N.t("netflow.last_6h") || "最近6小时"}</option>`;
  html += `<option value="24h">${I18N.t("netflow.last_24h") || "最近24小时"}</option>`;
  html += `<option value="7d">${I18N.t("netflow.last_7d") || "最近7天"}</option>`;
  html += `</select>`;
  html += `<button class="nf-btn" data-nfact="refresh">${I18N.t("common.refresh") || "刷新"}</button>`;
  html += `</div>`;

  html += `<div id="nfContent" class="nf-content">`;
  html += `<div id="nfSummary" class="nf-section"><h3>${I18N.t("netflow.top_talkers") || "流量排行"}</h3><div id="nfSummaryBody"></div></div>`;
  html += `<div id="nfFlows" class="nf-section"><h3>${I18N.t("netflow.flow_detail") || "Flow 明细"}</h3><div id="nfFlowsBody"></div></div>`;
  html += `</div>`;

  container.innerHTML = html;

  // Auto-select first host
  const sel = $("nfHostSelect");
  if (sel && sel.options.length > 0) {
    nfCurrentHost = sel.value;
  }
  sel && sel.addEventListener("change", function() { nfCurrentHost = this.value; loadNetFlowData(); });
  const rng = $("nfRangeSelect");
  rng && rng.addEventListener("change", function() { nfCurrentRange = this.value; loadNetFlowData(); });

  if (nfCurrentHost) loadNetFlowData();
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
    fetch(`/api/v1/netflow/summary?host=${encodeURIComponent(host)}&range=${range}&dimension=src_ip&top=10`, { credentials: "same-origin" }).then(r => r.json()),
    fetch(`/api/v1/netflow/flows?host=${encodeURIComponent(host)}&limit=100`, { credentials: "same-origin" }).then(r => r.json()),
  ]).then(([sumData, flowData]) => {
    renderNfSummary(summaryBody, sumData.summary || [], "src_ip");
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
  html += `</tr></thead><tbody>`;

  flows.forEach(f => {
    const protoName = protoNameMap(f.protocol);
    html += `<tr>`;
    html += `<td><span class="nf-badge nf-badge-${f.source}">${f.source}</span></td>`;
    html += `<td>${esc(f.src_ip || "")}</td>`;
    html += `<td>${f.src_port || ""}</td>`;
    html += `<td>${esc(f.dst_ip || "")}</td>`;
    html += `<td>${f.dst_port || ""}</td>`;
    html += `<td>${protoName}</td>`;
    html += `<td>${formatBytes(f.bytes || 0)}</td>`;
    html += `<td>${f.packets || 0}</td>`;
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

  let csv = "source,src_ip,src_port,dst_ip,dst_port,protocol,bytes,packets\n";
  flows.forEach(f => {
    csv += `${f.source},${f.src_ip},${f.src_port},${f.dst_ip},${f.dst_port},${f.protocol},${f.bytes},${f.packets}\n`;
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

function formatBytes(bytes) {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return (bytes / Math.pow(1024, i)).toFixed(1) + " " + units[i];
}

// Register with navigation
// 事件委托：CSP 为 script-src 'self'，内联 onclick 会被浏览器拦截；且这些函数在 IIFE 内、
// 不挂 window，内联写法即便没有 CSP 也会 ReferenceError。刷新/筛选/导出此前因此全是死按钮。
safeAddEventListener("netflowPanel", "click", e => {
  const b = e.target.closest("[data-nfact]");
  if (!b) return;
  if (b.dataset.nfact === "refresh") loadNetFlowData();
  else if (b.dataset.nfact === "filter") applyNfFilter();
  else if (b.dataset.nfact === "export") exportNfCSV();
});

if (typeof window._pageRenderers === "undefined") window._pageRenderers = {};
window._pageRenderers.netflow = renderNetFlowPanel;

})();
