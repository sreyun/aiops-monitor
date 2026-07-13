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
  wrap.innerHTML = systems.map(sys => {
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

// 接口历史：复用自定义监控的历史曲线弹窗（同为 CheckPoint 序列），仅切换取数端点。
function openAPIHistory(id, name) {
  CHK_HIST = { id, name, type: "http", range: 1, base: "apimon/endpoints" };
  $("checkHistTitle").textContent = name + " · 接口性能历史";
  $("checkHistMask").classList.add("show");
  loadCheckHistory();
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

// 仅在 API 性能监控视图可见时，每 15s 刷新一次聚合表（避免后台空拉）。
function apimonTick() {
  const v = document.getElementById("view-apimon");
  if (v && v.classList.contains("active")) loadAPIMon();
}
setInterval(apimonTick, 15000);
