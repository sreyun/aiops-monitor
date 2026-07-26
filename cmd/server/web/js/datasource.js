/* ---------- 数据源接入（Loki / Prometheus / PostgreSQL / MySQL）---------- */
let LAST_DATASOURCES = [];
let DS_SQL_CONNS = [];

async function loadDataSources() {
  try {
    LAST_DATASOURCES = await fetch(`${API}/datasources`).then(r => r.json());
    if (!Array.isArray(LAST_DATASOURCES)) LAST_DATASOURCES = [];
  } catch (e) { LAST_DATASOURCES = []; }
  try {
    const j = await fetch(`${API}/sql/connections`).then(r => r.json()).catch(() => ({}));
    DS_SQL_CONNS = Array.isArray(j.connections) ? j.connections : [];
  } catch (e) { DS_SQL_CONNS = []; }
  renderDataSources();
}

function isSQLDSType(t) { return t === "postgres" || t === "mysql" || t === "postgresql"; }

function dsTypeBadge(t) {
  if (t === "loki") return { cls: "loki", label: "LOKI" };
  if (t === "postgres" || t === "postgresql") return { cls: "prom", label: "PG" };
  if (t === "mysql") return { cls: "prom", label: "MYSQL" };
  if (t === "vm") return { cls: "prom", label: "VM" };
  return { cls: "prom", label: "PROM" };
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
    const badge = dsTypeBadge(d.type);
    const statusHTML = d.enabled !== false
      ? '<span class="ds-status on"><span class="ds-status-dot"></span>已启用</span>'
      : '<span class="ds-status off"><span class="ds-status-dot"></span>已停用</span>';
    const authInfo = d.auth_user ? `<span class="ds-auth">用户 ${esc(d.auth_user)}</span>` : "";
    const urlShow = isSQLDSType(d.type)
      ? `${esc(d.url || "")}${d.port ? ":" + d.port : ""}${d.database ? "/" + esc(d.database) : ""}${d.sql_connection_id ? " · 关联连接" : ""}`
      : esc(d.url || "");
    return `<div class="ds-card ${badge.cls}${d.enabled === false ? " ds-off" : ""}" data-id="${esc(d.id)}">
      <div class="ds-type-icon ${badge.cls}">${badge.label}</div>
      <div class="ds-info">
        <div class="ds-name">${esc(d.name)}</div>
        <div class="ds-url"><span>${urlShow}</span>${authInfo}</div>
      </div>
      ${statusHTML}
      <div class="ds-actions">
        <button class="btn sm" data-dsact="test" data-id="${esc(d.id)}">测试</button>
        <button class="btn sm" data-dsact="edit" data-id="${esc(d.id)}">编辑</button>
        <button class="btn danger sm" data-dsact="del" data-id="${esc(d.id)}">删除</button>
      </div>
    </div>`;
  }).join("");
  const sel = $("dsQuerySource");
  if (sel) {
    const enabled = LAST_DATASOURCES.filter(d => d.enabled !== false);
    const prev = sel.value;
    sel.innerHTML = enabled.map(d => `<option value="${esc(d.id)}">${esc(d.name)}（${d.type}）</option>`).join("");
    if (prev && enabled.some(d => d.id === prev)) sel.value = prev;
    if (panel) panel.style.display = enabled.length ? "block" : "none";
  }
}

function syncDSTypeUI() {
  const ty = ($("dsType") && $("dsType").value) || "prometheus";
  const sql = isSQLDSType(ty);
  const extra = $("dsSQLExtraRow"); if (extra) extra.style.display = sql ? "" : "none";
  const link = $("dsSQLLinkRow"); if (link) link.style.display = sql ? "" : "none";
  const urlLbl = $("dsUrlLabel"); if (urlLbl) urlLbl.textContent = sql ? "主机地址" : "地址 URL";
  const url = $("dsUrl");
  if (url) url.placeholder = sql
    ? (ty === "mysql" ? "如 10.0.0.5 或 db.internal" : "如 10.0.0.5 或 postgres.internal")
    : "Prometheus http://prometheus:9090；VictoriaMetrics http://vm:8428；Loki http://loki:3100";
  const port = $("dsPort");
  if (port && !port.value) port.placeholder = ty === "mysql" ? "3306" : "5432";
  const connSel = $("dsSQLConnId");
  if (connSel && sql) {
    const want = ty === "mysql" ? "mysql" : "postgres";
    const list = DS_SQL_CONNS.filter(c => c.enabled !== false && (want === "mysql" ? (c.driver || "mysql") !== "postgres" : c.driver === "postgres"));
    const prev = connSel.value;
    connSel.innerHTML = `<option value="">不关联（使用上方主机凭证）</option>` +
      list.map(c => `<option value="${esc(c.id)}">[${esc(c.env || "prod")}] ${esc(c.name)} · ${esc(c.host)}:${c.port || ""}</option>`).join("");
    if (prev && list.some(c => c.id === prev)) connSel.value = prev;
  }
}

function openDataSourceModal(ds) {
  $("dataSourceModalTitle").textContent = ds ? "编辑数据源" : "添加数据源";
  $("dsId").value = ds ? ds.id : "";
  $("dsName").value = ds ? (ds.name || "") : "";
  $("dsType").value = ds ? (ds.type === "postgresql" ? "postgres" : ds.type) : "prometheus";
  $("dsUrl").value = ds ? (ds.url || "") : "";
  if ($("dsPort")) $("dsPort").value = ds && ds.port ? ds.port : "";
  if ($("dsDatabase")) $("dsDatabase").value = ds ? (ds.database || "") : "";
  $("dsAuthUser").value = ds ? (ds.auth_user || "") : "";
  $("dsAuthPass").value = ds ? (ds.auth_pass || "") : "";
  $("dsEnabled").checked = ds ? ds.enabled !== false : true;
  syncDSTypeUI();
  if ($("dsSQLConnId") && ds && ds.sql_connection_id) $("dsSQLConnId").value = ds.sql_connection_id;
  const tr = $("dsTestResult"); if (tr) { tr.textContent = ""; tr.className = "ai-test-result"; }
  $("dataSourceMask").classList.add("show");
}

function collectDataSource() {
  const type = $("dsType").value;
  const body = {
    id: $("dsId").value,
    name: $("dsName").value.trim(),
    type,
    url: $("dsUrl").value.trim(),
    auth_user: $("dsAuthUser").value.trim(),
    auth_pass: $("dsAuthPass").value,
    enabled: $("dsEnabled").checked,
  };
  if (isSQLDSType(type)) {
    body.port = parseInt(($("dsPort") && $("dsPort").value) || "0", 10) || 0;
    body.database = ($("dsDatabase") && $("dsDatabase").value.trim()) || "";
    body.sql_connection_id = ($("dsSQLConnId") && $("dsSQLConnId").value) || "";
  }
  return body;
}

async function saveDataSource() {
  const ds = collectDataSource();
  if (!ds.name) { toast("名称必填", "err"); return; }
  if (!isSQLDSType(ds.type) && !ds.url) { toast("名称和地址必填", "err"); return; }
  if (isSQLDSType(ds.type) && !ds.url && !ds.sql_connection_id) { toast("请填写主机或关联 SQL 连接", "err"); return; }
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

async function testDataSourceConn() {
  const ds = collectDataSource();
  if (!isSQLDSType(ds.type) && !ds.url) { toast("请先填写地址", "err"); return; }
  const el = $("dsTestResult");
  if (el) { el.textContent = "测试中…"; el.className = "ai-test-result"; }
  await withLoading("dsTestBtn", async () => {
    try {
      const r = await fetch(`${API}/datasources/test`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(ds) });
      const j = await r.json().catch(() => ({}));
      if (el) {
        if (j.ok) { el.textContent = "✓ 连接成功" + (j.version ? " · " + String(j.version).slice(0, 80) : ""); el.className = "ai-test-result ok"; }
        else { el.textContent = "✗ " + (j.error || "连接失败"); el.className = "ai-test-result err"; }
      }
    } catch (e) { if (el) { el.textContent = "✗ " + e; el.className = "ai-test-result err"; } }
  });
}

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
safeAddEventListener("dsType", "change", syncDSTypeUI);

safeAddEventListener("dsAIGenBtn", "click", () => {
  const sel = $("dsQuerySource");
  const ds = LAST_DATASOURCES.find(d => d.id === (sel && sel.value));
  if (!ds) { toast("请先添加并选择数据源", "err"); return; }
  const isLoki = ds.type === "loki";
  const isSQL = isSQLDSType(ds.type);
  const dialect = (ds.type === "mysql") ? "MySQL" : ((ds.type === "postgres" || ds.type === "postgresql") ? "PostgreSQL" : ds.type);
  openAIAssist({
    task: isLoki ? "logql" : (isSQL ? "pgsql" : "promql"),
    title: isLoki ? "AI 生成 LogQL" : (isSQL ? ("AI 生成 " + dialect + " SQL") : "AI 生成 PromQL"),
    mode: "generate",
    placeholder: isLoki ? "如：查询 nginx 最近的 5xx 错误日志" : (isSQL ? "如：查最近 20 条慢查询 / 列出无主键的表" : "如：CPU 使用率超过 80% 的主机"),
    context: `数据源：${ds.name}\n数据源 id=${ds.id}\n方言：${dialect}\n类型：${ds.type}\n地址：${ds.url || ""}${ds.database ? "\n数据库：" + ds.database : ""}\n仅生成只读 SELECT/WITH。`,
    datasource: ds.id,
    applyLabel: "应用到查询框",
    applyTo: (code) => { const t = $("dsQueryText"); if (t) { t.value = code; t.focus(); } }
  });
});

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
