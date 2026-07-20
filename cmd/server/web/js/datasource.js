/* ---------- 数据源接入（Loki / Prometheus）---------- */
let LAST_DATASOURCES = [];

async function loadDataSources() {
  try {
    LAST_DATASOURCES = await fetch(`${API}/datasources`).then(r => r.json());
    if (!Array.isArray(LAST_DATASOURCES)) LAST_DATASOURCES = [];
  } catch (e) { LAST_DATASOURCES = []; }
  renderDataSources();
}

function renderDataSources() {
  const list = $("dataSourceList"), empty = $("dataSourceEmpty"), panel = $("dsQueryPanel");
  if (!list) return;
  if (!LAST_DATASOURCES.length) {
    list.innerHTML = "";
    if (empty) { empty.className = "ds-empty"; empty.style.display = ""; }
    if (panel) panel.style.display = "none";
    return;
  }
  if (empty) empty.style.display = "none";
  list.innerHTML = LAST_DATASOURCES.map(d => {
    const type = d.type === "loki" ? "loki" : "prom"; // 图标配色：指标类(prom/vm)同色，日志类(loki)另色
    const typeLabel = d.type === "loki" ? "LOKI" : d.type === "vm" ? "VM" : "PROM";
    const statusHTML = d.enabled !== false
      ? '<span class="ds-status on"><span class="ds-status-dot"></span>已启用</span>'
      : '<span class="ds-status off"><span class="ds-status-dot"></span>已停用</span>';
    const authInfo = d.auth_user ? `<span class="ds-auth">用户 ${esc(d.auth_user)}</span>` : "";
    return `<div class="ds-card ${type}${d.enabled === false ? " ds-off" : ""}" data-id="${esc(d.id)}">
      <div class="ds-type-icon ${type}">${typeLabel}</div>
      <div class="ds-info">
        <div class="ds-name">${esc(d.name)}</div>
        <div class="ds-url"><span>${esc(d.url)}</span>${authInfo}</div>
      </div>
      ${statusHTML}
      <div class="ds-actions">
        <button class="btn sm" data-dsact="test" data-id="${esc(d.id)}">测试</button>
        <button class="btn sm" data-dsact="edit" data-id="${esc(d.id)}">编辑</button>
        <button class="btn danger sm" data-dsact="del" data-id="${esc(d.id)}">删除</button>
      </div>
    </div>`;
  }).join("");
  // 即时查询面板的数据源下拉（仅启用项）
  const sel = $("dsQuerySource");
  if (sel) {
    const enabled = LAST_DATASOURCES.filter(d => d.enabled !== false);
    const prev = sel.value;
    sel.innerHTML = enabled.map(d => `<option value="${esc(d.id)}">${esc(d.name)}（${d.type}）</option>`).join("");
    if (prev && enabled.some(d => d.id === prev)) sel.value = prev;
    if (panel) panel.style.display = enabled.length ? "block" : "none";
  }
}

function openDataSourceModal(ds) {
  $("dataSourceModalTitle").textContent = ds ? "编辑数据源" : "添加数据源";
  $("dsId").value = ds ? ds.id : "";
  $("dsName").value = ds ? (ds.name || "") : "";
  $("dsType").value = ds ? ds.type : "prometheus";
  $("dsUrl").value = ds ? (ds.url || "") : "";
  $("dsAuthUser").value = ds ? (ds.auth_user || "") : "";
  $("dsAuthPass").value = ds ? (ds.auth_pass || "") : ""; // 脱敏值；留空=保持原密码
  $("dsEnabled").checked = ds ? ds.enabled !== false : true;
  const tr = $("dsTestResult"); if (tr) { tr.textContent = ""; tr.className = "ai-test-result"; }
  $("dataSourceMask").classList.add("show");
}

function collectDataSource() {
  return {
    id: $("dsId").value,
    name: $("dsName").value.trim(),
    type: $("dsType").value,
    url: $("dsUrl").value.trim(),
    auth_user: $("dsAuthUser").value.trim(),
    auth_pass: $("dsAuthPass").value,
    enabled: $("dsEnabled").checked,
  };
}

async function saveDataSource() {
  const ds = collectDataSource();
  if (!ds.name || !ds.url) { toast("名称和地址必填", "err"); return; }
  await withLoading("dsSaveBtn", async () => {
    try {
      const editing = !!ds.id;
      const url = editing ? `${API}/datasources/${encodeURIComponent(ds.id)}` : `${API}/datasources`;
      const r = await fetch(url, { method: editing ? "PUT" : "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(ds) });
      const j = await r.json().catch(() => ({}));
      if (r.ok) { toast("已保存", "ok"); $("dataSourceMask").classList.remove("show"); loadDataSources(); }
      else toast(j.error || "保存失败", "err");
    } catch (e) { toast("保存失败: " + e, "err"); }
  });
}

// 测试当前弹窗里填写的配置（编辑时密码留空则后端按 ID 还原原值）
async function testDataSourceConn() {
  const ds = collectDataSource();
  if (!ds.url) { toast("请先填写地址", "err"); return; }
  const el = $("dsTestResult");
  if (el) { el.textContent = "测试中…"; el.className = "ai-test-result"; }
  await withLoading("dsTestBtn", async () => {
    try {
      const r = await fetch(`${API}/datasources/test`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(ds) });
      const j = await r.json().catch(() => ({}));
      if (el) {
        if (j.ok) { el.textContent = "✓ 连接成功"; el.className = "ai-test-result ok"; }
        else { el.textContent = "✗ " + (j.error || "连接失败"); el.className = "ai-test-result err"; }
      }
    } catch (e) { if (el) { el.textContent = "✗ " + e; el.className = "ai-test-result err"; } }
  });
}

// 从列表直接测试已保存的数据源
async function testDataSourceById(id) {
  const ds = LAST_DATASOURCES.find(d => d.id === id);
  if (!ds) return;
  toast("测试中…", "ok");
  try {
    const r = await fetch(`${API}/datasources/test`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(ds) });
    const j = await r.json().catch(() => ({}));
    if (j.ok) toast("✓ " + ds.name + " 连接成功", "ok");
    else toast("✗ " + (j.error || "连接失败"), "err");
  } catch (e) { toast("✗ " + e, "err"); }
}

async function deleteDataSource(id) {
  if (!confirm("确定删除该数据源？")) return;
  try {
    const r = await fetch(`${API}/datasources/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (r.ok) { toast("已删除", "ok"); loadDataSources(); } else toast("删除失败", "err");
  } catch (e) { toast("删除失败: " + e, "err"); }
}

async function runDataSourceQuery() {
  const sel = $("dsQuerySource");
  const id = sel ? sel.value : "";
  const query = $("dsQueryText").value.trim();
  if (!id) { toast("请先添加并选择数据源", "err"); return; }
  if (!query) { toast("请输入查询语句", "err"); return; }
  const out = $("dsQueryResult");
  if (out) out.textContent = "查询中…";
  await withLoading("dsRunQueryBtn", async () => {
    try {
      const body = { query, limit: parseInt($("dsQueryLimit").value) || 0, since_min: parseInt($("dsQuerySince").value) || 0 };
      const r = await fetch(`${API}/datasources/${encodeURIComponent(id)}/query`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      const j = await r.json().catch(() => ({}));
      if (out) out.textContent = j.ok ? (j.result || "（无结果）") : ("查询失败: " + (j.error || "未知错误"));
    } catch (e) { if (out) out.textContent = "查询失败: " + e; }
  });
}

safeAddEventListener("addDataSourceBtn", "click", () => openDataSourceModal(null));
safeAddEventListener("dsSaveBtn", "click", saveDataSource);
safeAddEventListener("dsTestBtn", "click", testDataSourceConn);
safeAddEventListener("dsRunQueryBtn", "click", runDataSourceQuery);

// AI 辅助：根据自然语言生成 LogQL / PromQL（按所选数据源类型自动切换），可一键应用到查询框
safeAddEventListener("dsAIGenBtn", "click", () => {
  const sel = $("dsQuerySource");
  const ds = LAST_DATASOURCES.find(d => d.id === (sel && sel.value));
  if (!ds) { toast("请先添加并选择数据源", "err"); return; }
  const isLoki = ds.type === "loki";
  openAIAssist({
    task: isLoki ? "logql" : "promql",
    title: isLoki ? "AI 生成 LogQL" : "AI 生成 PromQL",
    mode: "generate",
    placeholder: isLoki ? "如：查询 nginx 最近的 5xx 错误日志" : "如：CPU 使用率超过 80% 的主机",
    context: `数据源：${ds.name}（类型 ${ds.type}，地址 ${ds.url}）`,
    applyLabel: "应用到查询框",
    applyTo: (code) => { const t = $("dsQueryText"); if (t) { t.value = code; t.focus(); } }
  });
});

// AI 辅助：解读当前查询结果（弹窗结果诊断）
safeAddEventListener("dsAIAnalyzeBtn", "click", () => {
  const res = $("dsQueryResult");
  const resText = res ? res.textContent.trim() : "";
  if (!resText || resText === "查询中…") { toast("请先运行查询，得到结果后再分析", "err"); return; }
  const sel = $("dsQuerySource");
  const ds = LAST_DATASOURCES.find(d => d.id === (sel && sel.value));
  const q = $("dsQueryText") ? $("dsQueryText").value.trim() : "";
  openAIAssist({
    task: "result_diagnosis",
    title: "AI 分析查询结果",
    mode: "analyze",
    context: `查询语句：\n${q}\n\n数据源类型：${ds ? ds.type : "未知"}\n\n查询结果（截断）：\n${resText.slice(0, 6000)}`
  });
});
safeAddEventListener("dataSourceList", "click", e => {
  const b = e.target.closest("[data-dsact]"); if (!b) return;
  const id = b.dataset.id;
  if (b.dataset.dsact === "edit") { const ds = LAST_DATASOURCES.find(d => d.id === id); if (ds) openDataSourceModal(ds); }
  else if (b.dataset.dsact === "del") deleteDataSource(id);
  else if (b.dataset.dsact === "test") testDataSourceById(id);
});
