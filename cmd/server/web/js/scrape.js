/* ========== 指标抓取（agentless exporter 摄入 Prometheus 生态） ==========
 * 服务端直接抓取 exporter / 应用 /metrics → 解析 Prometheus 文本 → 带标签样本 → VM。
 * 免装 agent，一段代码覆盖 JVM/MySQL/Redis/Kafka 等整个 exporter 生态。另含 remote_write
 * 接收面板（任何 exporter/telegraf/categraf/OTel 都能往这推）。
 */
let LAST_SCRAPES = { targets: [] };

async function loadScrapes() {
  try {
    const d = await fetch(`${API}/scrape-targets`).then(r => r.json());
    LAST_SCRAPES = d && d.targets ? d : { targets: [] };
    renderScrapes(LAST_SCRAPES);
  } catch (e) { /* ignore */ }
}

function scrapeStatDot(t) {
  if (!t.checked_at) return '<span class="sdot idle"></span>未抓取';
  return t.ok ? '<span class="sdot ok"></span>正常' : '<span class="sdot crit"></span>异常';
}

function renderScrapes(data) {
  const wrap = $("scrapeTargets");
  if (!wrap) return;
  const targets = data.targets || [];
  if (!targets.length) {
    wrap.innerHTML = `<div class="empty-box">还没有指标抓取目标。点右上角「添加抓取目标」把 exporter / 应用 /metrics 纳入采集——服务端 agentless 抓取（免装 agent），解析 Prometheus 文本入 VM，即刻覆盖 JVM / MySQL / Redis / Kafka 等整个 exporter 生态。</div>`;
    return;
  }
  wrap.innerHTML = targets.map(t => `
    <div class="api-sys-card">
      <div class="api-sys-head">
        <div class="api-sys-title">${esc(t.name)} <span style="font-weight:400; color:var(--muted)">${scrapeStatDot(t)}${t.checked_at ? ` · ${t.samples} 序列${t.latency_ms ? " · " + t.latency_ms.toFixed(0) + "ms" : ""}` : ""}</span>${!t.enabled ? '<span class="tag">已停用</span>' : ""}${t.msg ? `<span class="tag crit" title="${esc(t.msg)}">错误</span>` : ""}</div>
        <div class="api-sys-actions">
          <button class="mini-btn" data-sact="run" data-id="${esc(t.id)}" title="立即抓取">▶</button>
          <button class="mini-btn" data-sact="edit" data-id="${esc(t.id)}" title="编辑">✎</button>
          <button class="mini-btn del" data-sact="del" data-id="${esc(t.id)}" title="删除">✕</button>
        </div>
      </div>
      <div style="padding:10px 16px; color:var(--muted); font-size:12px; font-family:monospace; word-break:break-all">${esc(t.url)} · 每 ${t.interval_sec}s${t.labels && Object.keys(t.labels).length ? " · " + Object.entries(t.labels).map(([k, v]) => esc(k + "=" + v)).join(" ") : ""}</div>
    </div>`).join("");
}

// 每行 "Key: Value" 解析为对象
function scrapeParseKV(text) {
  const o = {};
  (text || "").split("\n").forEach(l => { const i = l.indexOf(":"); if (i > 0) { const k = l.slice(0, i).trim(); if (k) o[k] = l.slice(i + 1).trim(); } });
  return o;
}

function openScrapeModal(t) {
  $("scrapeId").value = t ? t.id : "";
  $("scrapeName").value = t ? t.name : "";
  $("scrapeUrl").value = t ? t.url : "";
  $("scrapeInterval").value = t ? t.interval_sec : 30;
  $("scrapeTimeout").value = t && t.timeout_sec ? t.timeout_sec : "";
  $("scrapeEnabled").checked = t ? t.enabled : true;
  $("scrapeLabels").value = (t && t.labels) ? Object.entries(t.labels).map(([k, v]) => `${k}: ${v}`).join("\n") : "";
  $("scrapeHeaders").value = (t && t.headers) ? Object.entries(t.headers).map(([k, v]) => `${k}: ${v}`).join("\n") : "";
  $("scrapeModalTitle").textContent = t ? "编辑抓取目标" : "添加抓取目标";
  $("scrapeMask").classList.add("show");
}

async function saveScrape() {
  const body = {
    id: $("scrapeId").value,
    name: $("scrapeName").value.trim(),
    url: $("scrapeUrl").value.trim(),
    interval_sec: Math.max(5, parseInt($("scrapeInterval").value) || 30),
    timeout_sec: parseInt($("scrapeTimeout").value) || 0,
    enabled: $("scrapeEnabled").checked,
    labels: scrapeParseKV($("scrapeLabels").value),
    headers: scrapeParseKV($("scrapeHeaders").value)
  };
  if (!body.name || !body.url) { toast("请填写名称与 URL", "err"); return; }
  await withLoading("scrapeSaveBtn", async () => {
    try {
      const r = await fetch(`${API}/scrape-targets`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      if (r.ok) { toast("已保存", "ok"); $("scrapeMask").classList.remove("show"); loadScrapes(); }
      else { const j = await r.json().catch(() => ({})); toast("保存失败：" + (j.error || ""), "err"); }
    } catch (e) { toast("保存失败：" + e, "err"); }
  });
}

async function delScrape(id) {
  const t = (LAST_SCRAPES.targets || []).find(x => x.id === id);
  if (!confirm(`确认删除抓取目标「${t ? t.name : id}」？`)) return;
  try { await fetch(`${API}/scrape-targets/${encodeURIComponent(id)}`, { method: "DELETE" }); toast("已删除", "ok"); loadScrapes(); }
  catch (e) { toast("删除失败：" + e, "err"); }
}

function runScrape(id) {
  fetch(`${API}/scrape-targets/${encodeURIComponent(id)}/run`, { method: "POST" })
    .then(() => { toast("已触发抓取", "ok"); setTimeout(loadScrapes, 1500); })
    .catch(e => toast("触发失败：" + e, "err"));
}

/* ---- remote_write 接收面板 ---- */
async function loadPromWrite() {
  const el = $("promWritePanel");
  if (!el) return;
  try {
    const d = await fetch(`${API}/prom/write-config`).then(r => r.json());
    const enabled = !!(d && d.token);
    const url = location.origin + (d.path || "/api/v1/prom/write");
    el.innerHTML = `
      <div class="field"><label>接收令牌（Bearer） <span class="tag">${enabled ? "已启用" : "空 = 禁用接收"}</span></label>
        <div class="token-gen-row"><input type="password" id="promWriteToken" autocomplete="off" placeholder="${enabled ? "已设置（点生成可重置）" : "点生成一串强随机令牌"}" value="${enabled ? "****" : ""}"><button class="btn sm" id="promGenBtn" type="button" title="生成高强度随机令牌">🎲 生成</button><button class="btn sm primary" id="promSaveBtn" type="button">保存</button></div>
      </div>
      <div class="hint" style="margin-top:8px">推送地址：<code>${esc(url)}</code>　鉴权头：<code>Authorization: Bearer &lt;令牌&gt;</code>。任何 exporter/telegraf/categraf/OTel Collector 配 remote_write 指向此地址即可 → 直落 VM（由 VM 解 protobuf+snappy）。</div>`;
  } catch (e) { el.innerHTML = `<div class="empty-line">加载失败：${esc(e)}</div>`; }
}
// 高强度随机令牌（避免与 sre.js 的 genStrongToken 同名冲突）
function genStrongTokenScrape(n) { const a = new Uint8Array(n || 32); (window.crypto || window.msCrypto).getRandomValues(a); let s = ""; for (let i = 0; i < a.length; i++) s += String.fromCharCode(a[i]); return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, ""); }

/* ---------- 事件绑定 ---------- */
safeAddEventListener("scrapeAddBtn", "click", () => openScrapeModal(null));
safeAddEventListener("scrapeSaveBtn", "click", saveScrape);
safeAddEventListener("scrapeTargets", "click", e => {
  const btn = e.target.closest("[data-sact]"); if (!btn) return;
  const id = btn.dataset.id, act = btn.dataset.sact;
  if (act === "run") runScrape(id);
  else if (act === "edit") openScrapeModal((LAST_SCRAPES.targets || []).find(x => x.id === id));
  else if (act === "del") delScrape(id);
});
safeAddEventListener("promWritePanel", "click", async e => {
  if (e.target.closest("#promGenBtn")) { const t = $("promWriteToken"); if (t) { t.type = "text"; t.value = genStrongTokenScrape(32); } return; }
  if (e.target.closest("#promSaveBtn")) {
    const t = $("promWriteToken"); if (!t) return;
    try {
      const r = await fetch(`${API}/prom/write-config`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ token: t.value }) });
      if (r.ok) { toast(t.value ? "已保存（记得填入客户端）" : "已禁用接收", "ok"); loadPromWrite(); } else toast("保存失败", "err");
    } catch (err) { toast("保存失败：" + err, "err"); }
  }
});

/* ========== 指标告警规则（PromQL 条件 → 统一告警通道 → incident/AI 研判） ========== */
let LAST_RULES = [];

async function loadRules() {
  try {
    const d = await fetch(`${API}/prom-rules`).then(r => r.json());
    LAST_RULES = (d && d.rules) || [];
    renderRules(LAST_RULES);
  } catch (e) { /* ignore */ }
}

function renderRules(rules) {
  const wrap = $("promRules");
  if (!wrap) return;
  if (!rules.length) {
    wrap.innerHTML = `<div class="empty-box">还没有指标告警规则。写一条 PromQL 条件（如 <code>mysql_up == 0</code>）即可对抓取 / 推送来的指标告警——命中后走统一告警通道，自动进 incident 与 AI 研判。支持 <code>{{标签}}</code> 文案模板。</div>`;
    return;
  }
  wrap.innerHTML = rules.map(r => `
    <div class="api-sys-card">
      <div class="api-sys-head">
        <div class="api-sys-title">${esc(r.name)} <span class="tag ${r.level === "critical" ? "crit" : ""}">${r.level === "critical" ? "严重" : "警告"}</span>${r.for_sec ? `<span class="tag">for ${r.for_sec}s</span>` : ""}${!r.enabled ? '<span class="tag">已停用</span>' : ""}</div>
        <div class="api-sys-actions">
          <button class="mini-btn" data-ract="edit" data-id="${esc(r.id)}" title="编辑">✎</button>
          <button class="mini-btn del" data-ract="del" data-id="${esc(r.id)}" title="删除">✕</button>
        </div>
      </div>
      <div style="padding:10px 16px; color:var(--muted); font-size:12px; font-family:monospace; word-break:break-all">${esc(r.expr)}${r.message ? `<span style="color:var(--txt2)"> · 文案：${esc(r.message)}</span>` : ""}</div>
    </div>`).join("");
}

function openRuleModal(r) {
  $("ruleId").value = r ? r.id : "";
  $("ruleName").value = r ? r.name : "";
  $("ruleExpr").value = r ? r.expr : "";
  $("ruleLevel").value = r ? r.level : "critical";
  $("ruleFor").value = r && r.for_sec ? r.for_sec : "";
  $("ruleMessage").value = r ? (r.message || "") : "";
  $("ruleEnabled").checked = r ? r.enabled : true;
  const tr = $("ruleTestResult"); if (tr) { tr.textContent = ""; tr.removeAttribute("style"); }
  $("ruleModalTitle").textContent = r ? "编辑指标告警规则" : "添加指标告警规则";
  $("ruleMask").classList.add("show");
}

async function saveRule() {
  const body = {
    id: $("ruleId").value,
    name: $("ruleName").value.trim(),
    expr: $("ruleExpr").value.trim(),
    level: $("ruleLevel").value,
    for_sec: Math.max(0, parseInt($("ruleFor").value) || 0),
    message: $("ruleMessage").value.trim(),
    enabled: $("ruleEnabled").checked
  };
  if (!body.name || !body.expr) { toast("请填写名称与表达式", "err"); return; }
  await withLoading("ruleSaveBtn", async () => {
    try {
      const r = await fetch(`${API}/prom-rules`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      if (r.ok) { toast("已保存", "ok"); $("ruleMask").classList.remove("show"); loadRules(); }
      else { const j = await r.json().catch(() => ({})); toast("保存失败：" + (j.error || ""), "err"); }
    } catch (e) { toast("保存失败：" + e, "err"); }
  });
}

async function testRule() {
  const expr = $("ruleExpr").value.trim();
  const el = $("ruleTestResult");
  if (!el) return;
  if (!expr) { el.textContent = "请先填写表达式"; el.style.color = "var(--crit)"; return; }
  el.textContent = "测试中…"; el.style.color = "var(--muted)";
  try {
    const j = await fetch(`${API}/prom-rules/test`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ expr }) }).then(r => r.json());
    if (j.ok) {
      el.style.color = "var(--ok)";
      el.textContent = j.count > 0 ? `✓ 当前命中 ${j.count} 组：${(j.samples || []).slice(0, 2).join("；")}` : "✓ 表达式合法，当前无命中（暂不告警）";
    } else { el.style.color = "var(--crit)"; el.textContent = "✗ " + (j.error || "测试失败"); }
  } catch (e) { el.style.color = "var(--crit)"; el.textContent = "✗ 请求失败：" + e; }
}

async function delRule(id) {
  const r = LAST_RULES.find(x => x.id === id);
  if (!confirm(`确认删除规则「${r ? r.name : id}」？`)) return;
  try { await fetch(`${API}/prom-rules/${encodeURIComponent(id)}`, { method: "DELETE" }); toast("已删除", "ok"); loadRules(); }
  catch (e) { toast("删除失败：" + e, "err"); }
}

safeAddEventListener("ruleAddBtn", "click", () => openRuleModal(null));
safeAddEventListener("ruleSaveBtn", "click", saveRule);
safeAddEventListener("ruleTestBtn", "click", testRule);
safeAddEventListener("promRules", "click", e => {
  const btn = e.target.closest("[data-ract]"); if (!btn) return;
  const id = btn.dataset.id, act = btn.dataset.ract;
  if (act === "edit") openRuleModal(LAST_RULES.find(x => x.id === id));
  else if (act === "del") delRule(id);
});

// 仅在指标抓取视图可见时每 15s 刷新一次
function scrapeTick() {
  const v = document.getElementById("view-scrape");
  if (v && v.classList.contains("active")) loadScrapes();
}
setInterval(scrapeTick, 15000);
