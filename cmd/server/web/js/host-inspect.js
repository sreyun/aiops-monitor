/* ===== 主机深度巡检（编排页 Tab · Web 报告） ===== */
let INSP_BATCHES = [];
let INSP_ACTIVE_ID = "";
let INSP_POLL = null;
let INSP_VIEW_ITEM = null; // {batchId, hostId}

function switchAutoTab(tab) {
  document.querySelectorAll("#autoTabs .chip-btn").forEach(b => b.classList.toggle("active", b.dataset.autotab === tab));
  document.querySelectorAll("#view-automation .sre-panel").forEach(p => p.classList.toggle("active", p.id === "autoPanel-" + tab));
  if (tab === "inspect") loadHostInspect();
  if (tab === "playbooks" && typeof loadPlaybooks === "function") loadPlaybooks();
}

document.querySelectorAll("#autoTabs .chip-btn").forEach(b => {
  b.addEventListener("click", () => switchAutoTab(b.dataset.autotab));
});

async function loadHostInspect() {
  renderInspHostPicker();
  try {
    INSP_BATCHES = await fetch(`${API}/host-inspect`).then(r => r.json()) || [];
  } catch (e) {
    INSP_BATCHES = [];
    console.warn("load host-inspect:", e);
  }
  renderInspBatches();
  if (INSP_ACTIVE_ID) {
    const b = INSP_BATCHES.find(x => x.id === INSP_ACTIVE_ID);
    if (b && b.status === "running") startInspPoll(INSP_ACTIVE_ID);
  }
}

function renderInspHostPicker() {
  const box = $("inspHostList");
  if (!box) return;
  const hosts = (typeof LAST_HOSTS !== "undefined" && LAST_HOSTS) ? LAST_HOSTS : [];
  if (!hosts.length) {
    box.innerHTML = `<div class="hint">${I18N.t("inspect.no_host_cache", "暂无主机列表，请先打开「主机」页加载数据")}</div>`;
    return;
  }
  box.innerHTML = hosts.map(h => {
    const online = !!h.online;
    return `<label class="insp-host-row ${online ? "" : "off"}">
      <input type="checkbox" class="insp-host-cb" value="${esc(h.id)}" ${online ? "" : "disabled"}>
      <span class="insp-host-name">${esc(h.hostname || h.id)}</span>
      <span class="insp-host-meta">${esc(h.os || "")} · ${esc(h.ip || "—")} · ${online ? I18N.t("status.online", "在线") : I18N.t("status.offline", "离线")}</span>
    </label>`;
  }).join("");
}

safeAddEventListener("inspSelectAll", "change", e => {
  const on = !!e.target.checked;
  document.querySelectorAll(".insp-host-cb:not(:disabled)").forEach(cb => { cb.checked = on; });
});

safeAddEventListener("inspRefreshBtn", "click", () => loadHostInspect());

safeAddEventListener("inspRunBtn", "click", async () => {
  const ids = [...document.querySelectorAll(".insp-host-cb:checked")].map(cb => cb.value);
  if (!ids.length) {
    toast(I18N.t("inspect.pick_hosts", "请先勾选要巡检的在线主机"), "warn");
    return;
  }
  try {
    const profile = ($("inspProfile") && $("inspProfile").value) || "standard";
    const timeout = profile === "deep" ? 300 : profile === "quick" ? 90 : 180;
    const r = await fetch(`${API}/host-inspect/run`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ host_ids: ids, timeout_sec: timeout, profile })
    });
    const data = await r.json();
    if (!r.ok) {
      toast(data.error || I18N.t("inspect.run_fail", "发起巡检失败"), "err");
      return;
    }
    toast(I18N.t("inspect.run_ok", "巡检已开始"), "ok");
    INSP_ACTIVE_ID = data.id;
    INSP_VIEW_ITEM = null;
    await loadHostInspect();
    startInspPoll(data.id);
  } catch (e) {
    toast(String(e), "err");
  }
});

function startInspPoll(id) {
  stopInspPoll();
  INSP_POLL = setInterval(async () => {
    try {
      const b = await fetch(`${API}/host-inspect/${encodeURIComponent(id)}`).then(r => r.json());
      const idx = INSP_BATCHES.findIndex(x => x.id === id);
      if (idx >= 0) INSP_BATCHES[idx] = b; else INSP_BATCHES.unshift(b);
      renderInspBatches();
      if (INSP_VIEW_ITEM && INSP_VIEW_ITEM.batchId === id) {
        const it = (b.items || []).find(x => x.host_id === INSP_VIEW_ITEM.hostId);
        if (it && it.report) showInspReport(b, it);
      }
      if (b.status === "done") stopInspPoll();
    } catch (e) { /* ignore transient */ }
  }, 2000);
}

function stopInspPoll() {
  if (INSP_POLL) { clearInterval(INSP_POLL); INSP_POLL = null; }
}

function inspStatusLabel(s) {
  return ({
    pending: I18N.t("inspect.st_pending", "等待"),
    running: I18N.t("inspect.st_running", "巡检中"),
    ok: I18N.t("inspect.st_ok", "正常"),
    warn: I18N.t("inspect.st_warn", "警告"),
    crit: I18N.t("inspect.st_crit", "严重"),
    error: I18N.t("inspect.st_error", "失败"),
    done: I18N.t("inspect.st_done", "完成")
  })[s] || s;
}

function renderInspBatches() {
  const list = $("inspBatchList");
  const empty = $("inspEmpty");
  const stats = $("inspStats");
  if (!list) return;
  if (!INSP_BATCHES.length) {
    list.innerHTML = "";
    if (empty) empty.style.display = "";
    if (stats) stats.innerHTML = "";
    if ($("inspReportView")) $("inspReportView").style.display = "none";
    return;
  }
  if (empty) empty.style.display = "none";

  const latest = INSP_BATCHES[0];
  if (stats && latest) {
    stats.innerHTML = `<div class="insp-stat-card"><b>${latest.host_count || 0}</b><span>${I18N.t("inspect.stat_hosts", "目标主机")}</span></div>
      <div class="insp-stat-card ok"><b>${latest.ok_count || 0}</b><span>${I18N.t("inspect.st_ok", "正常")}</span></div>
      <div class="insp-stat-card warn"><b>${latest.warn_count || 0}</b><span>${I18N.t("inspect.st_warn", "警告")}</span></div>
      <div class="insp-stat-card crit"><b>${latest.crit_count || 0}</b><span>${I18N.t("inspect.st_crit", "严重")}</span></div>
      <div class="insp-stat-card err"><b>${latest.err_count || 0}</b><span>${I18N.t("inspect.st_error", "失败")}</span></div>`;
  }

  list.innerHTML = INSP_BATCHES.map(b => {
    const when = b.started_at ? new Date(b.started_at * 1000).toLocaleString() : "";
    const items = (b.items || []).map(it => {
      const active = INSP_VIEW_ITEM && INSP_VIEW_ITEM.batchId === b.id && INSP_VIEW_ITEM.hostId === it.host_id ? "active" : "";
      return `<button type="button" class="insp-item ${it.status} ${active}" data-batch="${esc(b.id)}" data-host="${esc(it.host_id)}">
        <span class="insp-item-name">${esc(it.hostname)}</span>
        <span class="insp-item-meta">${esc(it.os_family || it.os || "")} · ${esc(it.ip || "")}</span>
        <span class="insp-badge ${it.status}">${inspStatusLabel(it.status)}</span>
        ${it.critical ? `<span class="insp-badge crit">${it.critical} crit</span>` : ""}
        ${it.warnings ? `<span class="insp-badge warn">${it.warnings} warn</span>` : ""}
      </button>`;
    }).join("");
    return `<div class="insp-batch">
      <div class="insp-batch-head">
        <strong>${esc(b.id)}</strong>
        <span class="insp-badge ${b.status}">${inspStatusLabel(b.status)}</span>
        <span class="hint">${esc(when)} · ${esc(b.operator || "")} · ${b.done_count || 0}/${b.host_count || 0}</span>
      </div>
      <div class="insp-items">${items}</div>
    </div>`;
  }).join("");

  list.querySelectorAll(".insp-item").forEach(btn => {
    btn.addEventListener("click", () => {
      const bid = btn.dataset.batch, hid = btn.dataset.host;
      const batch = INSP_BATCHES.find(x => x.id === bid);
      const item = batch && (batch.items || []).find(x => x.host_id === hid);
      if (!item) return;
      INSP_VIEW_ITEM = { batchId: bid, hostId: hid };
      renderInspBatches();
      showInspReport(batch, item);
    });
  });
}

function showInspReport(batch, item) {
  const view = $("inspReportView");
  if (!view) return;
  if (!item.report) {
    view.style.display = "";
    view.innerHTML = `<div class="insp-report-head"><h3>${esc(item.hostname)}</h3>
      <span class="insp-badge ${item.status}">${inspStatusLabel(item.status)}</span></div>
      <div class="hint">${esc(item.error || I18N.t("inspect.waiting_report", "报告生成中…"))}</div>`;
    return;
  }
  let rep = item.report;
  if (typeof rep === "string") {
    try { rep = JSON.parse(rep); } catch (e) { rep = null; }
  }
  if (!rep) {
    view.style.display = "";
    view.innerHTML = `<div class="hint">${I18N.t("inspect.bad_report", "报告解析失败")}</div>`;
    return;
  }
  view.style.display = "";
  const h = rep.host || {};
  const m = rep.metrics || {};
  const res = rep.result || {};
  const findings = (rep.findings || []).map(f =>
    `<li class="insp-finding ${f.level}"><b>${esc(f.level)}</b> ${esc(f.message)}</li>`
  ).join("") || `<li class="hint">${I18N.t("inspect.no_findings", "无告警项")}</li>`;

  const toc = (rep.sections || []).map(sec =>
    `<a href="#insp-sec-${esc(sec.id)}" class="insp-toc-a ${esc(sec.status || "ok")}">${esc(sec.title)}</a>`
  ).join("");

  view.innerHTML = `
    <div class="insp-report-head">
      <div>
        <h3>${esc(item.hostname || h.hostname || "")}</h3>
        <div class="hint">${esc(h.os || "")} · ${esc(h.os_family || "")} · ${esc(h.ip || item.ip || "")} · ${esc(h.kernel || "")}${h.fqdn ? " · " + esc(h.fqdn) : ""} · v${esc(rep.version || "")}</div>
      </div>
      <div class="insp-report-result">
        <span class="insp-badge ${item.status}">${inspStatusLabel(item.status)}</span>
        <span>${I18N.t("inspect.warnings", "警告")} ${res.warnings || 0}</span>
        <span>${I18N.t("inspect.critical", "严重")} ${res.critical || 0}</span>
        <span class="hint">${esc(rep.timestamp || "")}${rep.elapsed_seconds != null ? " · " + Number(rep.elapsed_seconds).toFixed(1) + "s" : ""}</span>
        <button type="button" class="btn sm ai-assist-btn" id="inspAIAnalyzeBtn" title="${I18N.t("inspect.ai_analyze_title", "把体检发现喂给 AI Assist 做研判")}">🤖 ${I18N.t("inspect.ai_analyze", "AI 分析")}</button>
      </div>
    </div>
    <div class="insp-metrics">
      <div><b>${m.cpu_usage_pct ?? "—"}%</b><span>CPU</span></div>
      <div><b>${m.mem_usage_pct ?? "—"}%</b><span>MEM</span></div>
      <div><b>${m.swap_usage_pct ?? "—"}%</b><span>SWAP</span></div>
      <div><b>${m.load_1m ?? "—"}</b><span>Load1</span></div>
      <div><b>${m.disk_alert_count ?? 0}</b><span>Disk⚠</span></div>
      <div><b>${m.inode_alert_count ?? 0}</b><span>Inode⚠</span></div>
      <div><b>${m.fd_usage_pct != null ? m.fd_usage_pct + "%" : "—"}</b><span>FD</span></div>
      <div><b>${m.process_count ?? "—"}</b><span>Procs</span></div>
      <div><b>${m.zombie_count ?? 0}</b><span>Zombie</span></div>
      <div><b>${m.d_state_count ?? 0}</b><span>D-State</span></div>
      <div><b>${m.tcp_listen ?? "—"}</b><span>Listen</span></div>
      <div><b>${m.tcp_close_wait ?? 0}</b><span>CloseWait</span></div>
      <div><b>${m.oom_count ?? 0}</b><span>OOM</span></div>
      <div><b>${m.container_count ?? 0}</b><span>Ctr</span></div>
      <div><b>${(m.ssl_expired || 0) + (m.ssl_expiring || 0)}</b><span>SSL⚠</span></div>
    </div>
    <div class="insp-toc">${toc}</div>
    <div class="insp-findings"><h4>${I18N.t("inspect.findings", "发现问题")}</h4><ul>${findings}</ul></div>
    <div class="insp-sections">${(rep.sections || []).map(sec => {
      const rows = (sec.items || []).map(it =>
        `<tr class="${esc(it.status || "")}"><td>${esc(it.label)}</td><td>${esc(it.value)}</td></tr>`
      ).join("");
      return `<div class="insp-sec ${esc(sec.status || "ok")}" id="insp-sec-${esc(sec.id)}">
        <div class="insp-sec-head"><span class="insp-badge ${esc(sec.status || "ok")}">${esc(sec.status || "ok")}</span>
          <h4>${esc(sec.title)}</h4>
          ${sec.summary ? `<span class="hint">${esc(sec.summary)}</span>` : ""}
        </div>
        <table class="insp-table"><tbody>${rows}</tbody></table>
      </div>`;
    }).join("")}</div>
  `;
  const aiBtn = view.querySelector("#inspAIAnalyzeBtn");
  if (aiBtn) {
    aiBtn.onclick = () => openInspectAIAssist(batch, item, rep);
  }
  view.scrollIntoView({ behavior: "smooth", block: "nearest" });
}

function openInspectAIAssist(batch, item, rep) {
  if (typeof openAIAssist !== "function") {
    if (typeof toast === "function") toast(I18N.t("assist.unavailable", "AI 面板未就绪"), "err");
    return;
  }
  const h = (rep && rep.host) || {};
  const m = (rep && rep.metrics) || {};
  const res = (rep && rep.result) || {};
  const findings = Array.isArray(rep && rep.findings) ? rep.findings.slice() : [];
  const rank = { critical: 0, error: 1, warn: 2, warning: 2, info: 3 };
  findings.sort((a, b) => (rank[String(a.level || "").toLowerCase()] ?? 9) - (rank[String(b.level || "").toLowerCase()] ?? 9));
  const findingLines = findings.slice(0, 24).map(f =>
    `- [${f.level || "?"}] ${f.message || f.title || ""}`
  ).join("\n") || "（无 findings）";
  const hostName = item.hostname || h.hostname || item.host_id || "";
  let ctx = [
    `主机：${hostName}`,
    `主机ID：${item.host_id || h.id || ""}`,
    `系统：${h.os || ""} ${h.os_family || ""} ${h.kernel || ""}`,
    `IP：${h.ip || item.ip || ""}`,
    `批次：${batch && batch.id ? batch.id : ""} · 状态：${item.status || ""}`,
    `结果：警告 ${res.warnings || 0} · 严重 ${res.critical || 0}`,
    `指标：CPU ${m.cpu_usage_pct ?? "—"}% · MEM ${m.mem_usage_pct ?? "—"}% · Load1 ${m.load_1m ?? "—"} · Disk⚠ ${m.disk_alert_count ?? 0} · OOM ${m.oom_count ?? 0}`,
    "",
    "【发现问题（优先严重项）】",
    findingLines
  ].join("\n");
  if (ctx.length > 12000) ctx = ctx.slice(0, 12000) + "\n…（已截断）";
  openAIAssist({
    task: "host_inspect_analysis",
    title: I18N.t("inspect.ai_title", "AI · 主机体检分析") + " · " + hostName,
    mode: "analyze",
    context: ctx,
    hint: I18N.t("inspect.ai_hint", "正在结合体检 findings 与指标生成研判…")
  });
}
