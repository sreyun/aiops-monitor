/* ========== API 性能监控 ==========
 * 按「业务系统」分组批量监控接口：聚合表(最新状态 / 本次·平均·P95 响应时间 /
 * 1h·24h 可用率 / 吞吐) + 历史曲线(复用拨测历史弹窗) + 异常告警。数据来自
 * GET /apimon/systems（实时状态由内存、聚合由 VM 现算），写入 VM 后重启不丢。
 */
let LAST_APIMON = { systems: [] };

async function loadAPIMon() {
  try {
    const data = await fetch(`${API}/apimon/systems`).then(r => r.json());
    LAST_APIMON = data && data.systems ? data : { systems: [] };
    renderAPIMon(LAST_APIMON);
  } catch (e) { /* ignore */ }
}

// 可用率着色：≥99.9 绿、≥99 黄、否则红；<0 表示暂无数据。
function apimonAvailClass(v) {
  if (v < 0) return "";
  if (v >= 99.9) return "ok";
  if (v >= 99) return "warn";
  return "crit";
}
function apiFmtPct(v) { return v < 0 ? "—" : v.toFixed(2) + "%"; }
function apiFmtMs(v) { return (!v || v <= 0) ? "—" : v.toFixed(0) + " ms"; }

function renderAPIMon(data) {
  const wrap = $("apimonSystems");
  if (!wrap) return;
  const systems = data.systems || [];
  if (!systems.length) {
    wrap.innerHTML = `<div class="empty-box">还没有 API 性能监控。点右上角「添加业务系统」把一个系统的多个接口批量纳入监控——自动统计可用率、平均 / P95 响应时间与吞吐，异常时按配置级别自动告警。</div>`;
    return;
  }
  // 顶部汇总条：业务系统 / 接口总数 / 当前异常 / 平均可用率(1h)
  const allEps = systems.flatMap(s => s.endpoints || []);
  const downCount = allEps.filter(e => e.down).length;
  const withAvail = allEps.filter(e => e.avail_1h >= 0);
  const avgAvail = withAvail.length ? withAvail.reduce((s, e) => s + e.avail_1h, 0) / withAvail.length : -1;
  const summary = `<div class="api-summary">
    <div class="card-stat"><div class="n">${systems.length}</div><div class="l">业务系统</div></div>
    <div class="card-stat"><div class="n">${allEps.length}</div><div class="l">监控接口</div></div>
    <div class="card-stat"><div class="n ${downCount ? "crit" : "ok"}">${downCount}</div><div class="l">当前异常</div></div>
    <div class="card-stat"><div class="n ${avgAvail >= 0 ? apimonAvailClass(avgAvail) : ""}">${apiFmtPct(avgAvail)}</div><div class="l">平均可用率 (1h)</div></div>
  </div>`;
  wrap.innerHTML = summary + systems.map(sys => {
    const eps = sys.endpoints || [];
    const downCount = eps.filter(e => e.down).length;
    const rows = eps.length ? eps.map(ep => {
      const dot = !ep.checked_at ? '<span class="sdot idle"></span>' : (ep.ok ? '<span class="sdot ok"></span>' : '<span class="sdot crit"></span>');
      const statusText = !ep.checked_at ? "未探测" : (ep.ok ? "正常" : "异常");
      return `<tr class="${ep.down ? "row-down" : ""}">
        <td><div class="api-ep-name">${esc(ep.name)}</div><div class="api-ep-url">${esc(ep.method || "GET")} ${esc(ep.url)}</div></td>
        <td>${dot}${statusText}${ep.status_code ? ` <span class="muted">${ep.status_code}</span>` : ""}</td>
        <td>${apiFmtMs(ep.latency_ms)}</td>
        <td>${apiFmtMs(ep.avg_ms)}</td>
        <td>${apiFmtMs(ep.p95_ms)}</td>
        <td class="${apimonAvailClass(ep.avail_1h)}">${apiFmtPct(ep.avail_1h)}</td>
        <td class="${apimonAvailClass(ep.avail_24h)}">${apiFmtPct(ep.avail_24h)}</td>
        <td>${ep.samples_1h > 0 ? ep.samples_1h.toFixed(0) : "—"}</td>
        <td><button class="mini-btn" data-aact="hist" data-ep="${esc(ep.id)}" data-name="${esc(ep.name)}">历史</button></td>
      </tr>`;
    }).join("") : `<tr><td colspan="9" class="muted">该业务系统暂无接口，点「编辑」添加。</td></tr>`;
    return `<div class="api-sys-card" data-sys="${esc(sys.id)}">
      <div class="api-sys-head">
        <div class="api-sys-title">${esc(sys.name)} <span class="tag">${eps.length} 接口 · 每 ${sys.interval_sec}s</span>${downCount ? `<span class="tag crit">${downCount} 异常</span>` : ""}${!sys.enabled ? '<span class="tag">已停用</span>' : ""}</div>
        <div class="api-sys-actions">
          <button class="mini-btn" data-aact="run" data-sys="${esc(sys.id)}">立即探测</button>
          <button class="mini-btn" data-aact="edit" data-sys="${esc(sys.id)}">编辑</button>
          <button class="mini-btn danger" data-aact="del" data-sys="${esc(sys.id)}">删除</button>
        </div>
      </div>
      <div class="api-table-wrap"><table class="api-table">
        <thead><tr><th>接口</th><th>最新状态</th><th>本次</th><th>平均(1h)</th><th>P95(1h)</th><th>可用率(1h)</th><th>可用率(24h)</th><th>吞吐(次/时)</th><th></th></tr></thead>
        <tbody>${rows}</tbody>
      </table></div>
    </div>`;
  }).join("");
}

function openAPISystemModal(sys) {
  $("apiSysId").value = sys ? sys.id : "";
  $("apiSysName").value = sys ? sys.name : "";
  $("apiSysInterval").value = sys ? sys.interval_sec : 60;
  $("apiSysLevel").value = sys ? sys.level : "critical";
  $("apiSysEnabled").checked = sys ? sys.enabled : true;
  $("apiSysModalTitle").textContent = sys ? "编辑业务系统" : "添加业务系统";
  // 回填公共请求头
  const commonHeadersText = (sys && sys.common_headers)
    ? Object.entries(sys.common_headers).map(([k, v]) => `${k}: ${v}`).join("\n")
    : "";
  $("apiSysCommonHeaders").value = commonHeadersText;
  // 回填公共请求体（必须回显，否则编辑保存会把公共体清零）
  $("apiSysCommonBody").value = (sys && sys.common_body) || "";
  const rows = $("apiEndpointRows");
  rows.innerHTML = "";
  const eps = (sys && sys.endpoints) || [];
  if (eps.length) eps.forEach(ep => addAPIEndpointRow(ep));
  else addAPIEndpointRow(null);
  $("apimonMask").classList.add("show");
}

// 追加一个接口编辑行（新增/编辑通用）。
function addAPIEndpointRow(ep) {
  const rows = $("apiEndpointRows");
  const div = document.createElement("div");
  div.className = "api-ep-row";
  const m = (ep && ep.method) || "GET";
  const methods = ["GET", "POST", "PUT", "DELETE", "HEAD", "PATCH"].map(x => `<option ${x === m ? "selected" : ""}>${x}</option>`).join("");
  const headersText = (ep && ep.headers) ? Object.entries(ep.headers).map(([k, v]) => `${k}: ${v}`).join("\n") : "";
  const commonHeaders = ($("apiSysCommonHeaders")?.value || "").trim();
  div.innerHTML = `
    <button class="api-ep-del" data-aact="ep-del" title="移除接口">✕</button>
    <div class="api-ep-grid">
      <input class="ep-name" placeholder="接口名称，如 登录" value="${esc((ep && ep.name) || "")}">
      <select class="ep-method sel">${methods}</select>
      <input class="ep-url" placeholder="https://api.example.com/v1/login" value="${esc((ep && ep.url) || "")}">
    </div>
    <div class="api-ep-grid2">
      <input class="ep-status" type="number" placeholder="期望状态码(默认<400)" value="${ep && ep.expect_status ? ep.expect_status : ""}">
      <input class="ep-keyword" placeholder="响应含关键字" value="${esc((ep && ep.expect_keyword) || "")}">
      <input class="ep-jsonpath" placeholder="JSON路径 如 code" value="${esc((ep && ep.json_path) || "")}">
      <input class="ep-jsonexpect" placeholder="JSON期望值 如 0" value="${esc((ep && ep.json_expect) || "")}">
    </div>
    <details class="api-ep-advanced">
      <summary>高级选项（请求头 / 请求体）${commonHeaders ? `<span class="hint">· 已继承系统级公共请求头</span>` : ""}</summary>
      <div class="api-ep-grid3">
        <textarea class="ep-headers" rows="2" placeholder="接口级请求头（覆盖同名的公共头），每行 Key: Value，如 Authorization: Bearer xxx">${esc(headersText)}</textarea>
        <textarea class="ep-body" rows="2" placeholder="请求体(可选，POST/PUT)">${esc((ep && ep.body) || "")}</textarea>
      </div>
    </details>`;
  rows.appendChild(div);
}

async function saveAPISystem() {
  const endpoints = [...document.querySelectorAll("#apiEndpointRows .api-ep-row")].map(row => {
    const g = s => row.querySelector(s);
    const headers = {};
    (g(".ep-headers").value || "").split("\n").forEach(line => { const i = line.indexOf(":"); if (i > 0) { const k = line.slice(0, i).trim(); if (k) headers[k] = line.slice(i + 1).trim(); } });
    return {
      name: g(".ep-name").value.trim(),
      url: g(".ep-url").value.trim(),
      method: g(".ep-method").value,
      expect_status: parseInt(g(".ep-status").value) || 0,
      expect_keyword: g(".ep-keyword").value.trim(),
      json_path: g(".ep-jsonpath").value.trim(),
      json_expect: g(".ep-jsonexpect").value.trim(),
      headers: headers,
      body: g(".ep-body").value,
      enabled: true
    };
  }).filter(ep => ep.name && ep.url);
  // 读取业务系统级公共请求头
  const commonHeaders = {};
  ($("apiSysCommonHeaders").value || "").split("\n").forEach(line => {
    const i = line.indexOf(":");
    if (i > 0) { const k = line.slice(0, i).trim(); if (k) commonHeaders[k] = line.slice(i + 1).trim(); }
  });
  const body = {
    id: $("apiSysId").value,
    name: $("apiSysName").value.trim(),
    interval_sec: Math.max(5, parseInt($("apiSysInterval").value) || 60),
    level: $("apiSysLevel").value,
    enabled: $("apiSysEnabled").checked,
    common_headers: commonHeaders,
    common_body: ($("apiSysCommonBody").value || "").trim(),
    endpoints: endpoints
  };
  if (!body.name) { toast("请填写业务系统名称", "err"); return; }
  if (!endpoints.length) { toast("请至少添加一个接口（需填接口名称与 URL）", "err"); return; }
  await withLoading("apiSysSaveBtn", async () => {
    try {
      const r = await fetch(`${API}/apimon/systems`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      if (r.ok) { toast("已保存", "ok"); $("apimonMask").classList.remove("show"); loadAPIMon(); }
      else { const j = await r.json().catch(() => ({})); toast("保存失败：" + (j.error || ""), "err"); }
    } catch (e) { toast("保存失败：" + e, "err"); }
  });
}

async function delAPISystem(id) {
  const sys = (LAST_APIMON.systems || []).find(s => s.id === id);
  if (!confirm(`确认删除业务系统「${sys ? sys.name : id}」及其全部接口监控？历史数据保留在 VM 中。`)) return;
  try { await fetch(`${API}/apimon/systems/${encodeURIComponent(id)}`, { method: "DELETE" }); toast("已删除", "ok"); loadAPIMon(); }
  catch (e) { toast("删除失败：" + e, "err"); }
}

function runAPISystem(id) {
  fetch(`${API}/apimon/systems/${encodeURIComponent(id)}/run`, { method: "POST" })
    .then(() => { toast("已触发探测", "ok"); setTimeout(loadAPIMon, 1800); })
    .catch(e => toast("触发失败：" + e, "err"));
}

// 接口性能历史：专用「组合曲线」弹窗——一屏三图：①响应时间分解(总/DNS/TCP/TLS/TTFB 同轴组合)
// ②可用性 ③响应体量。数据从 VM 全量回读（重启不丢），复用交互式图表引擎(悬停/框选放大/双击还原)。
let API_HIST = { id: "", name: "", range: 24 };
let API_HIST_CHARTS = {};

function openAPIHistory(id, name) {
  API_HIST = { id, name, range: 24 };
  $("apiHistTitle").textContent = name + " · 接口性能历史";
  $("apiHistMask").classList.add("show");
  loadAPIHistory();
}

function apiHistP95(vals) {
  if (!vals.length) return 0;
  const a = vals.slice().sort((x, y) => x - y);
  return a[Math.min(a.length - 1, Math.floor(a.length * 0.95))] || 0;
}

async function loadAPIHistory() {
  const { id, name, range } = API_HIST;
  const body = $("apiHistBody");
  body.innerHTML = `<div class="empty-line">${I18N.t("ui.loading", "加载中…")}</div>`;
  const now = Math.floor(Date.now() / 1000);
  const from = range > 0 ? now - range * 3600 : 0;
  const ctrl = renderChartControls(range, "arange");
  try {
    const sinceMin = range > 0 ? range * 60 : 43200; // 43200min=30d，range=0 视为「全部(近30天)」
    const all = await fetch(`${API}/apimon/endpoints/${encodeURIComponent(id)}/history?since_min=${sinceMin}`).then(r => r.json());
    const pts = (Array.isArray(all) ? all : []).filter(p => p.timestamp >= from);
    if (!pts.length) {
      body.innerHTML = `<div class="chart-controls">${ctrl}</div><div class="empty-line">该时间范围暂无数据（接口探测运行一段时间后自动积累，数据入 VM）。</div>`;
      return;
    }
    const samples = pts.map(p => ({
      timestamp: p.timestamp,
      latency_ms: p.latency_ms,
      dns_ms: p.dns_ms, tcp_ms: p.tcp_ms, tls_ms: p.tls_ms, ttfb_ms: p.ttfb_ms,
      online: p.ok ? 100 : 0,
      resp_kb: (p.resp_bytes || 0) / 1024,
    }));
    const uptime = (pts.filter(p => p.ok).length / pts.length * 100).toFixed(2);
    const avgLat = (pts.reduce((s, p) => s + (p.latency_ms || 0), 0) / pts.length).toFixed(0);
    const p95 = apiHistP95(pts.map(p => p.latency_ms || 0)).toFixed(0);
    const span = pts.length > 1 ? fmtDur(pts[pts.length - 1].timestamp - pts[0].timestamp) : I18N.t("time.just_now", "刚刚");
    const wrap = (cid, title) => `<div class="chart-wrap"><div class="chart-sub-title">${title}</div><canvas id="${cid}" width="1000" height="220"></canvas>` +
      `<button class="chart-enlarge" data-chart="${cid}" title="${I18N.t("ui.zoom_preview", "放大")}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg></button></div>`;
    body.innerHTML = `<div class="chart-controls">${ctrl}</div>
      <div class="api-hist-stat">
        <span class="ahs"><b class="${apimonAvailClass(parseFloat(uptime))}">${uptime}%</b><i>可用率</i></span>
        <span class="ahs"><b>${apiFmtMs(parseFloat(avgLat))}</b><i>平均延时</i></span>
        <span class="ahs"><b>${apiFmtMs(parseFloat(p95))}</b><i>P95 延时</i></span>
        <span class="ahs"><b>${pts.length}</b><i>采样 · 跨度 ${span}</i></span>
      </div>
      <div class="chart-container">
        ${wrap("apiHLat", "响应时间分解 · 总 / DNS / TCP / TLS / TTFB（ms）")}
        ${wrap("apiHAvail", "可用性 · 在线=100 / 离线=0")}
        ${wrap("apiHBytes", "响应体大小（KB）")}
      </div>
      <div class="hint">数据从 VictoriaMetrics 回读，重启不丢；曲线可悬停查看数值、拖动框选放大、双击还原。</div>`;
    API_HIST_CHARTS = {};
    API_HIST_CHARTS.apiHLat = createChart("apiHLat", samples, [
      { key: "latency_ms", label: "总延时", color: "#4c8dff", fmt: v => v.toFixed(0) + " ms" },
      { key: "dns_ms", label: "DNS", color: "#22c55e", fmt: v => v.toFixed(1) + " ms" },
      { key: "tcp_ms", label: "TCP", color: "#eab308", fmt: v => v.toFixed(1) + " ms" },
      { key: "tls_ms", label: "TLS", color: "#a855f7", fmt: v => v.toFixed(1) + " ms" },
      { key: "ttfb_ms", label: "TTFB", color: "#f97316", fmt: v => v.toFixed(1) + " ms" },
    ], 0, null, { title: name + " · 响应时间分解(ms)" });
    API_HIST_CHARTS.apiHAvail = createChart("apiHAvail", samples, [
      { key: "online", label: "在线", color: "#22c55e", fmt: v => (v >= 50 ? "在线" : "离线") },
    ], 0, 100, { title: name + " · 可用性" });
    API_HIST_CHARTS.apiHBytes = createChart("apiHBytes", samples, [
      { key: "resp_kb", label: "响应体", color: "#06b6d4", fmt: v => v.toFixed(1) + " KB" },
    ], 0, null, { title: name + " · 响应体大小(KB)" });
  } catch (e) {
    body.innerHTML = `<div class="empty-line">加载失败: ${esc(e)}</div>`;
  }
}

/* ---------- 事件绑定 ---------- */
safeAddEventListener("apimonAddBtn", "click", () => openAPISystemModal(null));
safeAddEventListener("apiSysSaveBtn", "click", saveAPISystem);
safeAddEventListener("apiAddEndpointBtn", "click", () => addAPIEndpointRow(null));
safeAddEventListener("apiEndpointRows", "click", e => {
  const del = e.target.closest('[data-aact="ep-del"]'); if (!del) return;
  const rows = $("apiEndpointRows");
  if (rows.querySelectorAll(".api-ep-row").length <= 1) { toast("至少保留一个接口", "err"); return; }
  del.closest(".api-ep-row").remove();
});
safeAddEventListener("apimonSystems", "click", e => {
  const btn = e.target.closest("[data-aact]"); if (!btn) return;
  const act = btn.dataset.aact;
  if (act === "hist") { openAPIHistory(btn.dataset.ep, btn.dataset.name); return; }
  const id = btn.dataset.sys; if (!id) return;
  if (act === "run") runAPISystem(id);
  else if (act === "edit") { const sys = (LAST_APIMON.systems || []).find(s => s.id === id); openAPISystemModal(sys); }
  else if (act === "del") delAPISystem(id);
});
// 接口历史弹窗：时间范围切换（快捷跨度）+ 图表放大委托
safeAddEventListener("apiHistBody", "click", e => {
  const rb = e.target.closest(".chip-btn[data-arange]");
  if (rb) { API_HIST.range = parseInt(rb.dataset.arange); loadAPIHistory(); return; }
  const en = e.target.closest(".chart-enlarge"); if (!en) return;
  const ch = API_HIST_CHARTS[en.dataset.chart]; if (ch) openChartZoom(ch);
});

// 把当前 API 业务监控快照汇总为纯文本，供 AI 分析（学习闭环：/ai/assist 自动沉淀记忆 + 👍/👎 强化）
function apimonToText() {
  const systems = (LAST_APIMON && LAST_APIMON.systems) || [];
  if (!systems.length) return "（当前没有任何 API 业务监控系统）";
  let totalEps = 0, totalDown = 0;
  const lines = [];
  systems.forEach(sys => {
    const eps = sys.endpoints || [];
    const down = eps.filter(e => e.down).length;
    totalEps += eps.length; totalDown += down;
    lines.push(`# 业务系统：${sys.name}（每 ${sys.interval_sec}s · 接口 ${eps.length} 个 · 异常 ${down} 个 · 级别 ${sys.level || "critical"}${sys.enabled === false ? " · 已停用" : ""}）`);
    eps.forEach(ep => {
      const st = !ep.checked_at ? "未探测" : (ep.ok ? "正常" : "异常");
      lines.push(`  - ${ep.name} [${ep.method || "GET"} ${ep.url}] 状态=${st}${ep.status_code ? " HTTP" + ep.status_code : ""} 本次=${apiFmtMs(ep.latency_ms)} 平均1h=${apiFmtMs(ep.avg_ms)} P95=${apiFmtMs(ep.p95_ms)} 可用率1h=${apiFmtPct(ep.avail_1h)} 24h=${apiFmtPct(ep.avail_24h)} 吞吐1h=${ep.samples_1h > 0 ? ep.samples_1h.toFixed(0) : "—"}`);
    });
  });
  const head = `业务系统 ${systems.length} 个 · 接口共 ${totalEps} 个 · 当前异常 ${totalDown} 个。\n`;
  return (head + lines.join("\n")).slice(0, 12000);
}

// 「🤖 AI 分析」：对当前所有业务系统接口的可用率/时延/异常做整体研判，结果自动进入 RAG 记忆闭环
safeAddEventListener("apimonAIBtn", "click", () => {
  if (typeof openAIAssist !== "function") { if (typeof toast === "function") toast(I18N.t("assist.unavailable", "AI 面板未就绪"), "err"); return; }
  openAIAssist({ task: "apimon_diagnosis", mode: "analyze", title: I18N.t("assist.title_apimon", "AI · API 业务监控分析"), context: apimonToText() });
});

// 仅在 API 性能监控视图可见时，每 15s 刷新一次聚合表（避免后台空拉）。
function apimonTick() {
  const v = document.getElementById("view-apimon");
  if (v && v.classList.contains("active")) loadAPIMon();
}
setInterval(apimonTick, 15000);
