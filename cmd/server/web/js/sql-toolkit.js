/* ---------- SQL 工具（MySQL 美化 / 审核 / 优化 + 只读 EXPLAIN）---------- */
let SQL_CONNS = [];
let SQL_CHANGES = [];
let SQL_HISTORY = [];
let SQL_SCHEMA = { tables: [], table: "", columns: [] };
let SQL_VERIFY_SQL = "";
let SQL_LAST = { audit: null, optimize: null, explain: null, beautified: "" };

function sqlT(k, fb) {
  return (typeof I18N !== "undefined" && I18N.t) ? (I18N.t(k, fb) || fb) : fb;
}

async function loadSQLToolkit() {
  await Promise.all([loadSQLConnections(), loadSQLChangeRequests(), loadSQLHistory()]);
  renderSQLConnSelect();
  renderSQLHistory();
  const conn = $("sqlConnSel") && $("sqlConnSel").value;
  if (conn) loadSQLSchema(conn);
  const tab = document.querySelector("#sqlInnerTabs .tab.active");
  const name = tab && tab.dataset.sqlTab ? tab.dataset.sqlTab : "workbench";
  showSQLTab(name);
}

async function loadSQLConnections() {
  try {
    const j = await fetch(`${API}/sql/connections`).then(r => r.json());
    SQL_CONNS = Array.isArray(j.connections) ? j.connections : (Array.isArray(j) ? j : []);
  } catch (e) {
    SQL_CONNS = [];
  }
}

function renderSQLConnSelect() {
  const sel = $("sqlConnSel");
  if (!sel) return;
  const prev = sel.value;
  const enabled = SQL_CONNS.filter(c => c.enabled !== false);
  sel.innerHTML = `<option value="">${esc(sqlT("sql.no_conn", "不连库（仅离线）"))}</option>` +
    enabled.map(c => `<option value="${esc(c.id)}">[${esc(c.env || "prod")}] ${esc(c.name)} (${esc(c.host)}:${c.port || 3306})</option>`).join("");
  if (prev && enabled.some(c => c.id === prev)) sel.value = prev;
  if (!sel.dataset.sqlConnBound) {
    sel.dataset.sqlConnBound = "1";
    sel.addEventListener("change", () => {
      const id = sel.value;
      if (id) loadSQLSchema(id);
      else renderSQLSchemaEmpty();
    });
  }
}

function renderSQLSchemaEmpty() {
  const db = $("sqlSchemaDb"); if (db) db.textContent = "";
  const tb = $("sqlSchemaTables"); if (tb) tb.innerHTML = `<span class="hint">${esc(sqlT("sql.schema_pick_conn", "选择连接后浏览表"))}</span>`;
  const col = $("sqlSchemaColumns"); if (col) col.innerHTML = "";
}

async function loadSQLSchema(connId, table) {
  if (!connId) { renderSQLSchemaEmpty(); return; }
  try {
    const q = table ? `?table=${encodeURIComponent(table)}` : "";
    const j = await fetch(`${API}/sql/connections/${encodeURIComponent(connId)}/schema${q}`).then(r => r.json());
    if (table) {
      SQL_SCHEMA.table = table;
      SQL_SCHEMA.columns = Array.isArray(j.columns) ? j.columns : [];
      renderSQLSchemaColumns(j);
    } else {
      SQL_SCHEMA.tables = Array.isArray(j.tables) ? j.tables : [];
      SQL_SCHEMA.table = "";
      SQL_SCHEMA.columns = [];
      renderSQLSchemaTables(connId, j);
    }
  } catch (e) {
    renderSQLSchemaEmpty();
  }
}

function renderSQLSchemaTables(connId, j) {
  const db = $("sqlSchemaDb");
  if (db) db.textContent = j.database || "";
  const box = $("sqlSchemaTables");
  if (!box) return;
  const tables = Array.isArray(j.tables) ? j.tables : [];
  if (!tables.length) {
    box.innerHTML = `<span class="hint">${esc(sqlT("sql.schema_empty", "无表或无权查看"))}</span>`;
    const col = $("sqlSchemaColumns"); if (col) col.innerHTML = "";
    return;
  }
  box.innerHTML = tables.map(t =>
    `<button type="button" class="${SQL_SCHEMA.table === t ? "active" : ""}" data-sqlschema-table="${esc(t)}">${esc(t)}</button>`
  ).join("");
  box.querySelectorAll("[data-sqlschema-table]").forEach(btn => {
    btn.onclick = () => loadSQLSchema(connId, btn.dataset.sqlschemaTable);
  });
}

function renderSQLSchemaColumns(j) {
  const tb = $("sqlSchemaTables");
  if (tb) tb.querySelectorAll("[data-sqlschema-table]").forEach(b => {
    b.classList.toggle("active", b.dataset.sqlschemaTable === j.table);
  });
  const box = $("sqlSchemaColumns");
  if (!box) return;
  const cols = Array.isArray(j.columns) ? j.columns : [];
  if (!cols.length) { box.innerHTML = ""; return; }
  box.innerHTML = `<div class="hint">${esc(j.table || "")} · ${cols.length} cols</div>` +
    cols.map(c => `<div class="mono">${esc(c.Field || c.field || "?")} <span class="tag">${esc(c.Type || c.type || "")}</span></div>`).join("");
}

async function loadSQLHistory() {
  try {
    const j = await fetch(`${API}/sql/history`).then(r => r.json());
    SQL_HISTORY = Array.isArray(j.history) ? j.history : [];
  } catch (_) { SQL_HISTORY = []; }
}

function renderSQLHistory() {
  const box = $("sqlHistoryList");
  if (!box) return;
  if (!SQL_HISTORY.length) {
    box.innerHTML = `<span class="hint">${esc(sqlT("sql.history_empty", "暂无历史"))}</span>`;
    return;
  }
  box.innerHTML = SQL_HISTORY.slice(0, 20).map(h => {
    const when = h.created_at ? new Date(h.created_at * 1000).toLocaleString() : "";
    const score = h.score != null ? ` · ${h.score}` : "";
    return `<button type="button" class="sql-history-item" data-sqlhist="${esc(h.id)}">
      <span class="tag">${esc(h.kind || "query")}${score}</span>
      <div class="mono">${esc(h.sql || "")}</div>
      <div class="hint">${esc(when)}</div>
    </button>`;
  }).join("");
  box.querySelectorAll("[data-sqlhist]").forEach(btn => {
    btn.onclick = () => {
      const item = SQL_HISTORY.find(x => x.id === btn.dataset.sqlhist);
      if (!item) return;
      if (item.connection_id && $("sqlConnSel")) $("sqlConnSel").value = item.connection_id;
      setSQLText(item.sql || "");
      toast(sqlT("sql.history_reopened", "已重新打开"), "ok");
    };
  });
}

function pickVerifySQL() {
  const stored = (SQL_VERIFY_SQL || "").trim();
  if (stored && !/^\s*(create|alter|drop)\b/i.test(stored)) return stored;
  const cur = sqlText().trim();
  if (cur && !/^\s*(create|alter|drop)\b/i.test(cur)) return cur;
  return "";
}
window.pickVerifySQL = pickVerifySQL;

function showSQLTab(name) {
  document.querySelectorAll("#sqlInnerTabs .tab").forEach(t => t.classList.toggle("active", t.dataset.sqlTab === name));
  const wb = $("sqlWorkbench");
  const cm = $("sqlConnManage");
  const ch = $("sqlChangeManage");
  if (wb) wb.style.display = name === "workbench" ? "" : "none";
  if (cm) cm.style.display = name === "connections" ? "" : "none";
  if (ch) ch.style.display = name === "changes" ? "" : "none";
  if (name === "connections") renderSQLConnList();
  if (name === "changes") loadSQLChangeRequests().then(renderSQLChangeList);
}

async function loadSQLChangeRequests() {
  try {
    const r = await fetch(`${API}/sql/change-requests`);
    const j = await r.json().catch(() => ({}));
    SQL_CHANGES = r.ok && Array.isArray(j.change_requests) ? j.change_requests : [];
  } catch (_) {
    SQL_CHANGES = [];
  }
}

function sqlConnectionEnvironment(id) {
  const c = SQL_CONNS.find(x => x.id === id);
  return c && c.env ? c.env : "prod";
}
window.sqlConnectionEnvironment = sqlConnectionEnvironment;

async function submitSQLChangeRequest(connectionId, sql, reason) {
  const r = await fetch(`${API}/sql/change-requests`, {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ connection_id: connectionId, sql, reason: reason || "" })
  });
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error || "提交变更单失败");
  await loadSQLChangeRequests();
  renderSQLChangeList();
  return j;
}
window.submitSQLChangeRequest = submitSQLChangeRequest;

async function submitSQLChangeFromEditor() {
  const connectionId = $("sqlConnSel") && $("sqlConnSel").value;
  const sql = sqlText().trim();
  if (!connectionId || !sql) { toast("请选择连接并输入 DDL", "err"); return; }
  const reason = prompt("请输入变更原因（可选）", "") || "";
  try {
    const cr = await submitSQLChangeRequest(connectionId, sql, reason);
    toast(`变更单 ${cr.id.slice(0, 8)} 已提交`, "ok");
    showSQLTab("changes");
  } catch (e) { toast(e.message || String(e), "err"); }
}

function renderSQLChangeList() {
  const list = $("sqlChangeList");
  if (!list) return;
  if (!SQL_CHANGES.length) {
    list.innerHTML = `<div class="ds-empty">暂无 DDL 变更单</div>`;
    return;
  }
  const admin = typeof isAdmin === "function" && isAdmin();
  const writable = typeof canWrite === "function" && canWrite();
  list.innerHTML = SQL_CHANGES.map(cr => {
    const expires = cr.expires_at ? new Date(cr.expires_at * 1000).toLocaleString() : "—";
    const approve = admin && cr.status === "pending"
      ? `<button class="btn sm" data-sqlchange="approve" data-id="${esc(cr.id)}">批准</button>
         <button class="btn danger sm" data-sqlchange="reject" data-id="${esc(cr.id)}">驳回</button>` : "";
    const rejectApproved = admin && cr.status === "approved"
      ? `<button class="btn danger sm" data-sqlchange="reject" data-id="${esc(cr.id)}">撤销</button>` : "";
    const execute = writable && cr.status === "approved"
      ? `<button class="btn primary sm" data-sqlchange="execute" data-id="${esc(cr.id)}">执行一次</button>` : "";
    return `<div class="ds-card" data-id="${esc(cr.id)}">
      <div class="ds-type-icon">DDL</div>
      <div class="ds-info">
        <div class="ds-name">[${esc(cr.environment || "prod")}] ${esc(cr.connection_name || cr.connection_id)}
          <span class="tag">${esc(cr.status)}</span></div>
        <div class="ds-url"><span>${esc(cr.proposer || "")} · ${new Date(cr.created_at * 1000).toLocaleString()}</span>
          <span class="ds-auth">有效期 ${esc(expires)}</span></div>
        <pre class="mono sql-snippet">${esc(cr.sql || "")}</pre>
        ${cr.reason ? `<div class="hint">原因：${esc(cr.reason)}</div>` : ""}
        ${cr.error ? `<div class="hint" style="color:var(--danger)">失败：${esc(cr.error)}</div>` : ""}
      </div>
      <div class="ds-actions">${approve}${rejectApproved}${execute}</div>
    </div>`;
  }).join("");
}

async function actSQLChange(id, action) {
  const promptText = action === "execute" ? "确认执行该 DDL？审批票将立即且永久消耗。" :
    (action === "approve" ? "确认批准该 DDL 变更单？" : "确认驳回/撤销该变更单？");
  if (!confirm(promptText)) return;
  try {
    const verifySQL = action === "execute" ? pickVerifySQL() : "";
    const body = action === "execute" && verifySQL ? { verify_sql: verifySQL } : {};
    const r = await fetch(`${API}/sql/change-requests/${encodeURIComponent(id)}/${action}`, {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body)
    });
    const j = await r.json().catch(() => ({}));
    if (!r.ok) throw new Error(j.error || "操作失败");
    toast(action === "execute" ? "DDL 已执行" : "变更单已更新", "ok");
    if (action === "execute" && (j.result || j.explain_diff || j.explain_before)) {
      showSQLExplainDiffResult(j.result || j);
    }
    await loadSQLChangeRequests();
    renderSQLChangeList();
    const conn = $("sqlConnSel") && $("sqlConnSel").value;
    if (conn) loadSQLSchema(conn);
  } catch (e) { toast(e.message || String(e), "err"); }
}

function sqlDialect() {
  const el = $("sqlDialect");
  return el ? el.value : "mysql80";
}

function sqlText() {
  const el = $("sqlEditor");
  return el ? el.value : "";
}

function setSQLText(v) {
  const el = $("sqlEditor");
  if (el) el.value = v;
}
window.setSQLText = setSQLText;

async function runSQLAnalyze() {
  const sql = sqlText().trim();
  if (!sql) { toast(sqlT("sql.empty", "请先输入 SQL"), "err"); return; }
  await withLoading("sqlAnalyzeBtn", async () => {
    try {
      const body = { sql, dialect: sqlDialect() };
      const conn = $("sqlConnSel") && $("sqlConnSel").value;
      if (conn) body.connection_id = conn;
      const r = await fetch(`${API}/sql/analyze`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body)
      });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) { toast(j.error || "分析失败", "err"); return; }
      SQL_LAST.audit = { findings: j.findings, score: j.score };
      SQL_LAST.optimize = { rewritten_sql: j.rewritten_sql, suggestions: j.suggestions, index_hints: j.index_hints };
      SQL_LAST.explain = j.explain ? { analysis: j.explain } : null;
      SQL_LAST.analyze = j;
      if (/^\s*(select|with)\b/i.test(sql)) SQL_VERIFY_SQL = sql;
      renderSQLAnalyze(j);
      await loadSQLHistory();
      renderSQLHistory();
    } catch (e) { toast(String(e), "err"); }
  });
}

function renderExplainDiffHTML(payload) {
  if (!payload) return "";
  const diff = payload.explain_diff;
  const before = payload.explain_before;
  const after = payload.explain_after;
  if (!diff && !before && !after) return "";
  const changes = diff && Array.isArray(diff.changes) ? diff.changes : [];
  const rows = changes.map(c => `<tr>
    <td>${esc(c.table || "")}</td><td>${esc(c.field || "")}</td>
    <td>${esc(c.before || "—")}</td><td>${esc(c.after || "—")}</td>
  </tr>`).join("");
  const renderSide = (label, a) => {
    if (!a) return "";
    const hits = Array.isArray(a.table_access) ? a.table_access : [];
    const tr = hits.map(h => `<tr><td>${esc(h.table || "")}</td><td>${esc(h.access_type || "")}</td><td>${esc(h.key || "—")}</td><td>${esc(String(h.rows != null ? h.rows : ""))}</td></tr>`).join("");
    return `<div class="sql-opt-block"><div class="sql-opt-head">${esc(label)}</div>
      <div class="table-wrap"><table class="data sql-explain-table"><thead><tr><th>table</th><th>type</th><th>key</th><th>rows</th></tr></thead><tbody>${tr || "<tr><td colspan=4>—</td></tr>"}</tbody></table></div></div>`;
  };
  return `<div class="sql-explain-diff">
    <div class="sql-opt-head">DDL 后 EXPLAIN 对比</div>
    <div class="sql-explain-summary">${esc((diff && diff.summary) || "")}</div>
    ${rows ? `<div class="table-wrap"><table class="data sql-explain-table"><thead><tr><th>table</th><th>field</th><th>before</th><th>after</th></tr></thead><tbody>${rows}</tbody></table></div>` : ""}
    ${renderSide("Before", before)}${renderSide("After", after)}
  </div>`;
}

function showSQLExplainDiffResult(payload) {
  const merged = Object.assign({}, SQL_LAST.analyze || {}, {
    explain_before: payload.explain_before,
    explain_after: payload.explain_after,
    explain_diff: payload.explain_diff,
    explain: payload.explain_after || (SQL_LAST.analyze && SQL_LAST.analyze.explain),
  });
  SQL_LAST.analyze = merged;
  renderSQLAnalyze(merged);
  showSQLTab("workbench");
}
window.showSQLExplainDiffResult = showSQLExplainDiffResult;

function renderSQLAnalyze(j) {
  const box = $("sqlResultPanel");
  if (!box) return;
  const bd = j.score_breakdown || {};
  const findings = Array.isArray(j.findings) ? j.findings : [];
  const hints = Array.isArray(j.index_hints) ? j.index_hints : [];
  const findingRows = findings.map(f => {
    const lv = f.level || "info";
    return `<div class="sql-finding ${esc(lv)}">
      <div class="sql-finding-head"><span class="sql-lv">${esc(lv)}</span><strong>${esc(f.title || f.id)}</strong><code class="mono">${esc(f.id || "")}</code></div>
      <div class="sql-finding-detail">${esc(f.detail || "")}</div>
      ${f.suggest ? `<div class="sql-finding-suggest">${esc(f.suggest)}</div>` : ""}
    </div>`;
  }).join("") || `<div class="hint">${esc(sqlT("sql.no_findings", "未发现问题"))}</div>`;
  const hintRows = hints.map(h =>
    `<li><strong>${esc(h.table || "")}</strong> (${esc((h.columns || []).join(", "))})
     — ${esc(h.reason || "")}${h.meta ? ' <span class="tag">meta</span>' : ""}
     ${h.ddl ? `<pre class="mono sql-snippet">${esc(h.ddl)}</pre>` : ""}</li>`
  ).join("");
  let explainHTML = "";
  if (j.explain) {
    const a = j.explain;
    const hits = Array.isArray(a.table_access) ? a.table_access : [];
    const rows = hits.map(h => `<tr>
      <td>${esc(h.table || "")}</td><td>${esc(h.access_type || "")}</td><td>${esc(h.key || "—")}</td>
      <td>${esc(String(h.rows != null ? h.rows : ""))}</td>
      <td>${h.full_scan_risk ? "⚠" : (h.key ? "✓" : "")}</td>
    </tr>`).join("");
    explainHTML = `<div class="sql-opt-block"><div class="sql-opt-head">EXPLAIN</div>
      <div class="sql-explain-summary">${esc(a.summary || "")}</div>
      <div class="table-wrap"><table class="data sql-explain-table">
        <thead><tr><th>table</th><th>type</th><th>key</th><th>rows</th><th></th></tr></thead>
        <tbody>${rows || "<tr><td colspan=5>—</td></tr>"}</tbody>
      </table></div></div>`;
  }
  const rewritten = j.rewritten_sql || "";
  box.innerHTML = `
    <div class="sql-score">${esc(sqlT("sql.score", "综合分"))}: <b>${j.score != null ? j.score : "-"}</b>
      <span class="tag">${j.parsed ? "AST" : "regex"}</span>
      ${j.metadata_used ? '<span class="tag">meta</span>' : ""}
      ${j.explain_used ? '<span class="tag">EXPLAIN</span>' : ""}
    </div>
    <div class="sql-breakdown hint">static −${bd.static_penalty || 0} · meta −${bd.meta_penalty || 0} · explain −${bd.explain_penalty || 0}</div>
    ${j.parse_error ? `<div class="hint">parse: ${esc(j.parse_error)}</div>` : ""}
    <div class="sql-opt-block"><div class="sql-opt-head">${esc(sqlT("sql.findings", "Findings"))}</div>${findingRows}</div>
    <div class="sql-opt-block"><div class="sql-opt-head">${esc(sqlT("sql.index_hints", "索引提示"))}</div><ul class="sql-ul">${hintRows || "<li>—</li>"}</ul></div>
    ${explainHTML}
    ${renderExplainDiffHTML(j)}
    <div class="sql-opt-block">
      <div class="sql-opt-head">
        <span>${esc(sqlT("sql.rewritten", "改写建议"))}</span>
        <button type="button" class="btn sm" id="sqlCopyRewritten">${esc(sqlT("sql.copy", "复制"))}</button>
        <button type="button" class="btn sm" id="sqlApplyRewritten">${esc(sqlT("sql.apply", "应用到编辑器"))}</button>
      </div>
      <pre class="mono sql-rewritten" id="sqlRewrittenBody">${esc(rewritten || "—")}</pre>
    </div>`;
  const copyBtn = $("sqlCopyRewritten");
  if (copyBtn) copyBtn.onclick = () => {
    if (!rewritten) return;
    navigator.clipboard.writeText(rewritten).then(() => toast(sqlT("sql.copied", "已复制"), "ok")).catch(() => toast("复制失败", "err"));
  };
  const applyBtn = $("sqlApplyRewritten");
  if (applyBtn) applyBtn.onclick = () => { if (rewritten) { setSQLText(rewritten); toast(sqlT("sql.applied", "已应用"), "ok"); } };
}

async function runSQLBeautify() {
  const sql = sqlText().trim();
  if (!sql) { toast(sqlT("sql.empty", "请先输入 SQL"), "err"); return; }
  await withLoading("sqlBeautifyBtn", async () => {
    try {
      const r = await fetch(`${API}/sql/beautify`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sql, dialect: sqlDialect() })
      });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) { toast(j.error || "失败", "err"); return; }
      setSQLText(j.sql || "");
      SQL_LAST.beautified = j.sql || "";
      toast(sqlT("sql.beautified", "已美化"), "ok");
    } catch (e) { toast(String(e), "err"); }
  });
}

async function runSQLAudit() {
  const sql = sqlText().trim();
  if (!sql) { toast(sqlT("sql.empty", "请先输入 SQL"), "err"); return; }
  await withLoading("sqlAuditBtn", async () => {
    try {
      const r = await fetch(`${API}/sql/audit`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sql, dialect: sqlDialect() })
      });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) { toast(j.error || "失败", "err"); return; }
      SQL_LAST.audit = j;
      renderSQLFindings(j);
    } catch (e) { toast(String(e), "err"); }
  });
}

async function runSQLOptimize() {
  const sql = sqlText().trim();
  if (!sql) { toast(sqlT("sql.empty", "请先输入 SQL"), "err"); return; }
  await withLoading("sqlOptimizeBtn", async () => {
    try {
      const r = await fetch(`${API}/sql/optimize`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sql, dialect: sqlDialect() })
      });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) { toast(j.error || "失败", "err"); return; }
      SQL_LAST.optimize = j;
      renderSQLOptimize(j);
    } catch (e) { toast(String(e), "err"); }
  });
}

async function runSQLExplain(connId, sqlOverride) {
  const sql = (sqlOverride != null ? String(sqlOverride) : sqlText()).trim();
  const conn = connId || ($("sqlConnSel") && $("sqlConnSel").value);
  if (!conn) { toast(sqlT("sql.need_conn", "EXPLAIN 需要选择 MySQL 连接"), "err"); return; }
  if (!sql) { toast(sqlT("sql.empty", "请先输入 SQL"), "err"); return; }
  // Prefer SELECT/WITH for re-EXPLAIN after DDL (index DDL itself is not EXPLAIN-able).
  const explainSQL = /^\s*(create|alter)\b/i.test(sql) ? sqlText().trim() || sql : sql;
  await withLoading("sqlExplainBtn", async () => {
    try {
      const r = await fetch(`${API}/sql/connections/${encodeURIComponent(conn)}/explain`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sql: explainSQL })
      });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) { toast(j.error || "EXPLAIN 失败", "err"); return; }
      SQL_LAST.explain = j;
      renderSQLExplain(j);
    } catch (e) { toast(String(e), "err"); }
  });
}
window.runSQLExplain = runSQLExplain;

function renderSQLFindings(j) {
  const box = $("sqlResultPanel");
  if (!box) return;
  const findings = Array.isArray(j.findings) ? j.findings : [];
  const score = typeof j.score === "number" ? j.score : "-";
  const rows = findings.map(f => {
    const lv = f.level || "info";
    return `<div class="sql-finding ${esc(lv)}">
      <div class="sql-finding-head"><span class="sql-lv">${esc(lv)}</span><strong>${esc(f.title || f.id)}</strong><code class="mono">${esc(f.id || "")}</code></div>
      <div class="sql-finding-detail">${esc(f.detail || "")}</div>
      ${f.suggest ? `<div class="sql-finding-suggest">${esc(f.suggest)}</div>` : ""}
    </div>`;
  }).join("") || `<div class="hint">${esc(sqlT("sql.no_findings", "未发现问题"))}</div>`;
  box.innerHTML = `<div class="sql-score">${esc(sqlT("sql.score", "审核分"))}: <b>${score}</b></div>${rows}`;
}

function renderSQLOptimize(j) {
  const box = $("sqlResultPanel");
  if (!box) return;
  const sug = (Array.isArray(j.suggestions) ? j.suggestions : []).map(s =>
    `<li><strong>${esc(s.title || s.id || "")}</strong> — ${esc(s.detail || s.message || "")}${s.sql ? `<pre class="mono sql-snippet">${esc(s.sql)}</pre>` : ""}</li>`
  ).join("");
  const idx = (Array.isArray(j.index_hints) ? j.index_hints : []).map(h =>
    `<li>${esc(typeof h === "string" ? h : (h.hint || h.sql || JSON.stringify(h)))}</li>`
  ).join("");
  const rewritten = j.rewritten_sql || "";
  box.innerHTML = `
    <div class="sql-opt-block">
      <div class="sql-opt-head">
        <span>${esc(sqlT("sql.rewritten", "改写建议"))}</span>
        <button type="button" class="btn sm" id="sqlCopyRewritten">${esc(sqlT("sql.copy", "复制"))}</button>
        <button type="button" class="btn sm" id="sqlApplyRewritten">${esc(sqlT("sql.apply", "应用到编辑器"))}</button>
      </div>
      <pre class="mono sql-rewritten" id="sqlRewrittenBody">${esc(rewritten || sqlT("sql.no_rewrite", "（无静态改写，见下方建议）"))}</pre>
    </div>
    <div class="sql-opt-block"><div class="sql-opt-head">${esc(sqlT("sql.suggestions", "优化建议"))}</div><ul class="sql-ul">${sug || "<li>—</li>"}</ul></div>
    <div class="sql-opt-block"><div class="sql-opt-head">${esc(sqlT("sql.index_hints", "索引提示"))}</div><ul class="sql-ul">${idx || "<li>—</li>"}</ul></div>`;
  const copyBtn = $("sqlCopyRewritten");
  if (copyBtn) copyBtn.onclick = () => {
    if (!rewritten) return;
    navigator.clipboard.writeText(rewritten).then(() => toast(sqlT("sql.copied", "已复制"), "ok")).catch(() => toast("复制失败", "err"));
  };
  const applyBtn = $("sqlApplyRewritten");
  if (applyBtn) applyBtn.onclick = () => { if (rewritten) { setSQLText(rewritten); toast(sqlT("sql.applied", "已应用"), "ok"); } };
}

function renderSQLExplain(j) {
  const box = $("sqlResultPanel");
  if (!box) return;
  const a = j.analysis || {};
  const hits = Array.isArray(a.table_access) ? a.table_access : [];
  const rows = hits.map(h => `<tr>
    <td>${esc(h.table || "")}</td>
    <td>${esc(h.access_type || "")}</td>
    <td>${esc(h.key || "—")}</td>
    <td>${esc(h.possible_keys || "—")}</td>
    <td>${esc(String(h.rows != null ? h.rows : ""))}</td>
    <td>${esc(String(h.filtered != null ? h.filtered : ""))}</td>
    <td>${h.full_scan_risk ? "⚠" : (h.key ? "✓" : "")}</td>
  </tr>`).join("");
  box.innerHTML = `
    <div class="sql-explain-summary">${esc(a.summary || "")}</div>
    <div class="table-wrap"><table class="data sql-explain-table">
      <thead><tr><th>table</th><th>type</th><th>key</th><th>possible_keys</th><th>rows</th><th>filtered</th><th></th></tr></thead>
      <tbody>${rows || `<tr><td colspan="7">${esc(sqlT("sql.no_explain_rows", "无表访问节点"))}</td></tr>`}</tbody>
    </table></div>
    <details class="sql-raw"><summary>EXPLAIN JSON</summary><pre class="mono">${esc(typeof j.raw === "string" ? j.raw : JSON.stringify(j.explain_json, null, 2))}</pre></details>`;
}

function sqlAssistContext() {
  const parts = [
    `方言: ${sqlDialect()}`,
    `SQL:\n${sqlText().trim()}`
  ];
  if (SQL_LAST.analyze) {
    parts.push(`全面分析: ${JSON.stringify({
      score: SQL_LAST.analyze.score,
      score_breakdown: SQL_LAST.analyze.score_breakdown,
      findings: SQL_LAST.analyze.findings,
      index_hints: SQL_LAST.analyze.index_hints,
      explain: SQL_LAST.analyze.explain,
      metadata_used: SQL_LAST.analyze.metadata_used
    }).slice(0, 8000)}`);
  } else {
    if (SQL_LAST.audit) {
      parts.push(`审核分: ${SQL_LAST.audit.score}\nFindings: ${JSON.stringify(SQL_LAST.audit.findings || []).slice(0, 4000)}`);
    }
    if (SQL_LAST.optimize) {
      parts.push(`静态优化: ${JSON.stringify({
        rewritten_sql: SQL_LAST.optimize.rewritten_sql,
        suggestions: SQL_LAST.optimize.suggestions,
        index_hints: SQL_LAST.optimize.index_hints
      }).slice(0, 4000)}`);
    }
    if (SQL_LAST.explain && SQL_LAST.explain.analysis) {
      parts.push(`EXPLAIN: ${JSON.stringify(SQL_LAST.explain.analysis).slice(0, 4000)}`);
    }
  }
  return parts.join("\n\n");
}

function openSQLAI(task) {
  if (typeof openAIAssist !== "function") {
    toast(sqlT("assist.unavailable", "AI 面板未就绪"), "err");
    return;
  }
  const titles = {
    sql_beautify: sqlT("sql.ai_beautify", "AI · SQL 美化"),
    sql_audit: sqlT("sql.ai_audit", "AI · SQL 深度审核"),
    sql_optimize: sqlT("sql.ai_optimize", "AI · SQL 深度优化"),
    sql_remediation: sqlT("sql.ai_remediation", "AI · SQL 优化闭环")
  };
  const connId = $("sqlConnSel") && $("sqlConnSel").value;
  const isPlan = task === "sql_remediation";
  openAIAssist({
    task,
    title: titles[task] || "AI · SQL",
    mode: task === "sql_beautify" ? "generate" : "analyze",
    context: sqlAssistContext() + (connId ? `\n\nconnection_id=${connId}` : ""),
    applyLabel: isPlan ? sqlT("ai.apply_actions", "应用建议动作") : sqlT("sql.apply", "应用到编辑器"),
    applyTo: async (code) => {
      if (isPlan && typeof window.applyOpsActionPlan === "function") {
        const plan = typeof window.parseOpsActionPlan === "function" ? window.parseOpsActionPlan(code) : null;
        if (plan && Array.isArray(plan.actions) && plan.actions.length) {
          return window.applyOpsActionPlan(plan, {
            source: "sql",
            connectionId: connId,
            selectSQL: setSQLText,
            reExplainSQL: sqlText(),
            refresh: async () => { await runSQLAnalyze(); },
            onDDLResult: (res) => { if (typeof showSQLExplainDiffResult === "function") showSQLExplainDiffResult(res); },
          });
        }
      }
      if (code) { setSQLText(code); toast(sqlT("sql.applied", "已应用"), "ok"); }
      if (/^\s*(select|with)\b/i.test(code || "")) SQL_VERIFY_SQL = code;
      return true;
    }
  });
}

function renderSQLConnList() {
  const list = $("sqlConnList");
  if (!list) return;
  if (!SQL_CONNS.length) {
    list.innerHTML = `<div class="ds-empty"><span class="ds-empty-icon">🗄</span>${esc(sqlT("sql.conn_empty", "还没有 MySQL 连接。管理员可添加只读账号用于 EXPLAIN。"))}</div>`;
    return;
  }
  list.innerHTML = SQL_CONNS.map(c => {
    const status = c.enabled !== false
      ? '<span class="ds-status on"><span class="ds-status-dot"></span>启用</span>'
      : '<span class="ds-status off"><span class="ds-status-dot"></span>停用</span>';
    return `<div class="ds-card prom${c.enabled === false ? " ds-off" : ""}" data-id="${esc(c.id)}">
      <div class="ds-type-icon prom">SQL</div>
      <div class="ds-info">
        <div class="ds-name">${esc(c.name)}</div>
        <div class="ds-url"><span>${esc(c.user || "")}@${esc(c.host)}:${c.port || 3306}/${esc(c.database || "")}</span>
          <span class="ds-auth">${esc(c.env || "prod")} · ${esc(c.version_hint || "auto")}</span></div>
      </div>
      ${status}
      <div class="ds-actions admin-only">
        <button class="btn sm" data-sqlconn="test" data-id="${esc(c.id)}">${esc(sqlT("sql.test", "测试"))}</button>
        <button class="btn sm" data-sqlconn="edit" data-id="${esc(c.id)}">${esc(sqlT("ui.edit", "编辑"))}</button>
        <button class="btn danger sm" data-sqlconn="del" data-id="${esc(c.id)}">${esc(sqlT("ui.delete", "删除"))}</button>
      </div>
    </div>`;
  }).join("");
}

function openSQLConnModal(c) {
  $("sqlConnModalTitle").textContent = c ? sqlT("sql.edit_conn", "编辑连接") : sqlT("sql.add_conn", "添加连接");
  $("sqlConnId").value = c ? c.id : "";
  $("sqlConnName").value = c ? (c.name || "") : "";
  $("sqlConnEnv").value = c ? (c.env || "prod") : "prod";
  $("sqlConnHost").value = c ? (c.host || "") : "";
  $("sqlConnPort").value = c ? (c.port || 3306) : 3306;
  $("sqlConnUser").value = c ? (c.user || "") : "";
  $("sqlConnPass").value = c ? (c.password || "") : "";
  $("sqlConnDB").value = c ? (c.database || "") : "";
  $("sqlConnTLS").value = c ? (c.tls || "") : "";
  $("sqlConnParams").value = c ? (c.params || "") : "";
  $("sqlConnVer").value = c ? (c.version_hint || "auto") : "auto";
  $("sqlConnEnabled").checked = c ? c.enabled !== false : true;
  const tr = $("sqlConnTestResult"); if (tr) { tr.textContent = ""; tr.className = "ai-test-result"; }
  $("sqlConnMask").classList.add("show");
}

function collectSQLConn() {
  return {
    id: $("sqlConnId").value,
    name: $("sqlConnName").value.trim(),
    env: $("sqlConnEnv").value,
    host: $("sqlConnHost").value.trim(),
    port: parseInt($("sqlConnPort").value, 10) || 3306,
    user: $("sqlConnUser").value.trim(),
    password: $("sqlConnPass").value,
    database: $("sqlConnDB").value.trim(),
    tls: $("sqlConnTLS").value.trim(),
    params: $("sqlConnParams").value.trim(),
    version_hint: $("sqlConnVer").value,
    enabled: $("sqlConnEnabled").checked
  };
}

async function saveSQLConn() {
  const c = collectSQLConn();
  if (!c.name || !c.host) { toast(sqlT("sql.name_host_required", "名称和主机必填"), "err"); return; }
  await withLoading("sqlConnSaveBtn", async () => {
    try {
      const editing = !!c.id;
      const url = editing ? `${API}/sql/connections/${encodeURIComponent(c.id)}` : `${API}/sql/connections`;
      const r = await fetch(url, { method: editing ? "PUT" : "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(c) });
      const j = await r.json().catch(() => ({}));
      if (r.ok) {
        toast(sqlT("ui.saved", "已保存"), "ok");
        $("sqlConnMask").classList.remove("show");
        await loadSQLConnections();
        renderSQLConnSelect();
        renderSQLConnList();
      } else toast(j.error || "保存失败", "err");
    } catch (e) { toast(String(e), "err"); }
  });
}

async function testSQLConnById(id) {
  toast(sqlT("sql.testing", "测试中…"), "ok");
  try {
    const r = await fetch(`${API}/sql/connections/${encodeURIComponent(id)}/test`, { method: "POST" });
    const j = await r.json().catch(() => ({}));
    if (j.ok) toast("✓ " + (j.version || "ok"), "ok");
    else toast("✗ " + (j.error || "失败"), "err");
  } catch (e) { toast("✗ " + e, "err"); }
}

async function deleteSQLConn(id) {
  if (!confirm(sqlT("sql.confirm_del", "确定删除该连接？"))) return;
  try {
    const r = await fetch(`${API}/sql/connections/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (r.ok) {
      toast(sqlT("ui.deleted", "已删除"), "ok");
      await loadSQLConnections();
      renderSQLConnSelect();
      renderSQLConnList();
    } else toast("删除失败", "err");
  } catch (e) { toast(String(e), "err"); }
}

safeAddEventListener("sqlInnerTabs", "click", e => {
  const t = e.target.closest("[data-sql-tab]"); if (!t) return;
  showSQLTab(t.dataset.sqlTab);
});
safeAddEventListener("sqlAnalyzeBtn", "click", runSQLAnalyze);
safeAddEventListener("sqlBeautifyBtn", "click", runSQLBeautify);
safeAddEventListener("sqlAuditBtn", "click", runSQLAudit);
safeAddEventListener("sqlOptimizeBtn", "click", runSQLOptimize);
safeAddEventListener("sqlExplainBtn", "click", runSQLExplain);
safeAddEventListener("sqlSubmitChangeBtn", "click", submitSQLChangeFromEditor);
safeAddEventListener("sqlChangeRefreshBtn", "click", async () => { await loadSQLChangeRequests(); renderSQLChangeList(); });
safeAddEventListener("sqlChangeList", "click", e => {
  const b = e.target.closest("[data-sqlchange]"); if (!b) return;
  actSQLChange(b.dataset.id, b.dataset.sqlchange);
});
safeAddEventListener("sqlAIBeautifyBtn", "click", () => openSQLAI("sql_beautify"));
safeAddEventListener("sqlAIAuditBtn", "click", () => openSQLAI("sql_audit"));
safeAddEventListener("sqlAIOptimizeBtn", "click", () => openSQLAI("sql_remediation"));
safeAddEventListener("addSQLConnBtn", "click", () => openSQLConnModal(null));
safeAddEventListener("sqlConnSaveBtn", "click", saveSQLConn);
safeAddEventListener("sqlConnList", "click", e => {
  const b = e.target.closest("[data-sqlconn]"); if (!b) return;
  const id = b.dataset.id;
  if (b.dataset.sqlconn === "edit") {
    const c = SQL_CONNS.find(x => x.id === id);
    if (c) openSQLConnModal(c);
  } else if (b.dataset.sqlconn === "del") deleteSQLConn(id);
  else if (b.dataset.sqlconn === "test") testSQLConnById(id);
});

window._pageRenderers = window._pageRenderers || {};
window._pageRenderers["sql-toolkit"] = loadSQLToolkit;
