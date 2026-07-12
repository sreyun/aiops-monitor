/* ========== 告警治理 ==========
 * 静默规则 / 抑制规则 / 通知路由 三类规则的可视化管理。整体读取 GET /alerts/governance，
 * 编辑后整体提交 POST /alerts/governance（后端在 notify.go pushChannels 生效）。
 * 状态只在「加载」时来自 GOV；新增/删除直接操作 DOM，保存时整体从 DOM 采集，
 * 避免"新增一条就把其它卡片未保存的编辑重渲染丢掉"。
 */
let GOV = { silence_rules: [], inhibit_rules: [], routes: [] };
const GOV_CHANNELS = [["feishu", "飞书"], ["dingtalk", "钉钉"], ["email", "邮件"], ["webhook", "自定义 Webhook"]];
const GOV_WEEKDAYS = [["1", "一"], ["2", "二"], ["3", "三"], ["4", "四"], ["5", "五"], ["6", "六"], ["0", "日"]];

async function loadGovernance() {
  try {
    const g = await fetch(`${API}/alerts/governance`).then(r => r.json());
    GOV = { silence_rules: g.silence_rules || [], inhibit_rules: g.inhibit_rules || [], routes: g.routes || [] };
  } catch (e) { GOV = { silence_rules: [], inhibit_rules: [], routes: [] }; }
  renderGovernance();
}

// 匹配条件编辑块（主机子串 + 类型逗号 + 级别复选）。prefix 用于类名隔离（一个卡片内可有 src/tgt 两块）。
function govMatchEditor(prefix, m, label) {
  m = m || {};
  const types = (m.types || []).join(",");
  const lv = m.levels || [];
  return `<div class="gov-match">
    ${label ? `<div class="gov-match-label">${label}</div>` : ""}
    <input class="gm-${prefix}-host" placeholder="主机名/IP 子串（留空=全部主机）" value="${esc(m.host_pattern || "")}">
    <input class="gm-${prefix}-types" placeholder="类型逗号分隔，如 cpu,memory,offline（留空=全部）" value="${esc(types)}">
    <span class="gov-lv-group">
      <label class="gov-lv"><input type="checkbox" class="gm-${prefix}-warning" ${lv.includes("warning") ? "checked" : ""}> 警告</label>
      <label class="gov-lv"><input type="checkbox" class="gm-${prefix}-critical" ${lv.includes("critical") ? "checked" : ""}> 严重</label>
    </span>
  </div>`;
}

function govRuleHead(r) {
  return `<button class="api-ep-del" data-gact="del" title="删除规则">✕</button>
    <div class="gov-rule-top">
      <input class="gov-name" placeholder="规则名称" value="${esc(r.name || "")}">
      <label class="switch gov-en"><input type="checkbox" class="gov-enabled" ${r.enabled !== false ? "checked" : ""}> <span>启用</span></label>
    </div>`;
}

// —— 单卡片构建器（新增时 append 一张，不重渲染其它卡片）——
function govSilenceCard(r) {
  r = r || {};
  return `<div class="gov-rule" data-kind="silence">
    ${govRuleHead(r)}
    ${govMatchEditor("m", r.match, "命中以下告警时静默（不推送通知，仍记录 + 页面可见）")}
    <div class="gov-window">
      <span class="gov-win-label">生效时段</span>
      <input class="gov-tstart" type="time" value="${esc(r.time_start || "")}"> —
      <input class="gov-tend" type="time" value="${esc(r.time_end || "")}">
      <span class="gov-hint-inline">（留空=全天；支持跨天如 22:00→08:00 夜间静默）</span>
    </div>
    <div class="gov-weekdays"><span class="gov-win-label">星期</span>${GOV_WEEKDAYS.map(([v, t]) =>
    `<label class="gov-wd"><input type="checkbox" class="gov-wd-cb" value="${v}" ${(r.weekdays || []).includes(+v) ? "checked" : ""}> ${t}</label>`).join("")}<span class="gov-hint-inline">（留空=每天）</span></div>
  </div>`;
}
function govInhibitCard(r) {
  r = r || {};
  return `<div class="gov-rule" data-kind="inhibit">
    ${govRuleHead(r)}
    ${govMatchEditor("src", r.source, "当存在以下「源」告警活跃时…")}
    ${govMatchEditor("tgt", r.target, "…则抑制以下「目标」告警的通知")}
    <label class="switch gov-samehost"><input type="checkbox" class="gov-same-host" ${r.same_host !== false ? "checked" : ""}> <span>仅同主机抑制（推荐：如主机离线时抑制其自身 CPU/内存告警）</span></label>
  </div>`;
}
function govRouteCard(r) {
  r = r || {};
  return `<div class="gov-rule" data-kind="route">
    ${govRuleHead(r)}
    ${govMatchEditor("m", r.match, "命中以下告警时，仅发往勾选的渠道")}
    <div class="gov-channels">${GOV_CHANNELS.map(([v, t]) =>
    `<label class="gov-ch"><input type="checkbox" class="gov-ch-cb" value="${v}" ${(r.channels || []).includes(v) ? "checked" : ""}> ${t}</label>`).join("")}</div>
    <label class="switch gov-continue"><input type="checkbox" class="gov-cont" ${r.continue ? "checked" : ""}> <span>命中后继续匹配后续路由（默认命中即停）</span></label>
  </div>`;
}

const GOV_EMPTY = {
  govSilenceList: `<div class="gov-empty">暂无静默规则。例：给「测试环境主机」加全天静默，或给「警告级」加夜间静默。</div>`,
  govInhibitList: `<div class="gov-empty">暂无抑制规则。例：源=offline、目标=cpu,memory,disk、同主机 → 主机离线时不再为它刷一堆指标告警。</div>`,
  govRouteList: `<div class="gov-empty">暂无通知路由。未配置任何路由=全部启用渠道都发。例：严重→飞书+钉钉+邮件；警告→仅飞书。</div>`,
};

function renderGovernance() {
  const sil = $("govSilenceList"), inh = $("govInhibitList"), rt = $("govRouteList");
  if (!sil) return;
  sil.innerHTML = (GOV.silence_rules || []).map(govSilenceCard).join("") || GOV_EMPTY.govSilenceList;
  inh.innerHTML = (GOV.inhibit_rules || []).map(govInhibitCard).join("") || GOV_EMPTY.govInhibitList;
  rt.innerHTML = (GOV.routes || []).map(govRouteCard).join("") || GOV_EMPTY.govRouteList;
}

// 新增：清掉空占位后 append 一张空白卡片（保留其它卡片未保存的编辑）。
function govAppend(listId, html) {
  const list = $(listId);
  const empty = list.querySelector(".gov-empty");
  if (empty) empty.remove();
  list.insertAdjacentHTML("beforeend", html);
}

// 从一个匹配编辑块收集 AlertMatch。
function govCollectMatch(card, prefix) {
  const host = card.querySelector(`.gm-${prefix}-host`).value.trim();
  const typesRaw = card.querySelector(`.gm-${prefix}-types`).value.trim();
  const types = typesRaw ? typesRaw.split(",").map(s => s.trim()).filter(Boolean) : [];
  const levels = [];
  if (card.querySelector(`.gm-${prefix}-warning`).checked) levels.push("warning");
  if (card.querySelector(`.gm-${prefix}-critical`).checked) levels.push("critical");
  const m = {};
  if (host) m.host_pattern = host;
  if (types.length) m.types = types;
  if (levels.length) m.levels = levels;
  return m;
}

async function saveGovernance() {
  const g = { silence_rules: [], inhibit_rules: [], routes: [] };
  document.querySelectorAll('#govSilenceList .gov-rule').forEach(card => {
    g.silence_rules.push({
      name: card.querySelector(".gov-name").value.trim(),
      enabled: card.querySelector(".gov-enabled").checked,
      match: govCollectMatch(card, "m"),
      time_start: card.querySelector(".gov-tstart").value,
      time_end: card.querySelector(".gov-tend").value,
      weekdays: [...card.querySelectorAll(".gov-wd-cb:checked")].map(x => +x.value),
    });
  });
  document.querySelectorAll('#govInhibitList .gov-rule').forEach(card => {
    g.inhibit_rules.push({
      name: card.querySelector(".gov-name").value.trim(),
      enabled: card.querySelector(".gov-enabled").checked,
      source: govCollectMatch(card, "src"),
      target: govCollectMatch(card, "tgt"),
      same_host: card.querySelector(".gov-same-host").checked,
    });
  });
  document.querySelectorAll('#govRouteList .gov-rule').forEach(card => {
    g.routes.push({
      name: card.querySelector(".gov-name").value.trim(),
      enabled: card.querySelector(".gov-enabled").checked,
      match: govCollectMatch(card, "m"),
      channels: [...card.querySelectorAll(".gov-ch-cb:checked")].map(x => x.value),
      continue: card.querySelector(".gov-cont").checked,
    });
  });
  await withLoading("govSaveBtn", async () => {
    try {
      const r = await fetch(`${API}/alerts/governance`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(g) });
      if (r.ok) { toast("告警治理规则已保存并生效", "ok"); loadGovernance(); }
      else { const j = await r.json().catch(() => ({})); toast("保存失败：" + (j.error || ""), "err"); }
    } catch (e) { toast("保存失败：" + e, "err"); }
  });
}

/* ---------- 事件绑定 ---------- */
safeAddEventListener("govAddSilence", "click", () => govAppend("govSilenceList", govSilenceCard({ enabled: true })));
safeAddEventListener("govAddInhibit", "click", () => govAppend("govInhibitList", govInhibitCard({ enabled: true, same_host: true })));
safeAddEventListener("govAddRoute", "click", () => govAppend("govRouteList", govRouteCard({ enabled: true })));
safeAddEventListener("govSaveBtn", "click", saveGovernance);
// 删除规则（委托）：移除 DOM 卡片；列表空了补回占位提示。
["govSilenceList", "govInhibitList", "govRouteList"].forEach(listId => {
  safeAddEventListener(listId, "click", e => {
    const del = e.target.closest('[data-gact="del"]'); if (!del) return;
    del.closest(".gov-rule").remove();
    const list = $(listId);
    if (!list.querySelector(".gov-rule")) list.innerHTML = GOV_EMPTY[listId];
  });
});
