/* security-overview.js — 安全中心总览 KPI */

let SEC_OVERVIEW = null;
let SEC_OVERVIEW_POLL = null;

function secOvT(k, fb) {
  return (window.I18N && I18N.t) ? I18N.t(k, fb) : fb;
}

async function loadSecurityOverview() {
  const el = $("securityOverviewPanel");
  if (!el) return;
  el.innerHTML = `<div class="loading-dots">${secOvT("common.loading", "加载中…")}</div>`;
  try {
    const r = await fetch(`${API}/security/overview`, { credentials: "same-origin" });
    const d = await r.json();
    if (!r.ok) throw new Error((d && d.error) || r.statusText);
    SEC_OVERVIEW = d;
    paintSecurityOverview();
    secOverviewMaybePoll();
  } catch (e) {
    el.innerHTML = `<div class="empty-state"><h4>${esc(secOvT("sec.load_failed", "加载失败"))}</h4><p>${esc(String(e.message || e))}</p></div>`;
  }
}

function paintSecurityOverview() {
  const el = $("securityOverviewPanel");
  if (!el || !SEC_OVERVIEW) return;
  const d = SEC_OVERVIEW;
  const sched = d.schedule || {};
  const scans = d.scans || {};
  const hostSched = sched.host || {};
  const webSched = sched.web || {};
  const schedOk = !!sched.healthy;
  const schedLabel = schedOk
    ? secOvT("sec.ov_sched_ok", "定时任务配置正常")
    : secOvT("sec.ov_sched_warn", "定时任务需检查");

  el.innerHTML = `<div class="sec-shell sec-overview-shell">
    <p class="cfg-panel-desc">${esc(secOvT("sec.ov_desc", "跨主机安全与 Web 扫描的开放风险、调度健康与进行中任务一览。"))}</p>
    <div class="sec-metrics sec-overview-kpis">
      <div class="sec-metric crit"><b>${d.open_critical || 0}</b><span>${esc(secOvT("sec.ov_open_crit", "开放危急发现"))}</span></div>
      <div class="sec-metric high"><b>${d.open_high || 0}</b><span>${esc(secOvT("sec.ov_open_high", "开放高危发现"))}</span></div>
      <div class="sec-metric ${schedOk ? "ok" : "warn"}"><b>${schedOk ? "✓" : "!"}</b><span>${esc(schedLabel)}</span></div>
      <div class="sec-metric"><b>${scans.total_running || 0}</b><span>${esc(secOvT("sec.ov_running", "进行中扫描"))}</span></div>
      <div class="sec-metric ${(scans.total_stuck || 0) > 0 ? "crit" : ""}"><b>${scans.total_stuck || 0}</b><span>${esc(secOvT("sec.ov_stuck", "疑似卡住"))}</span></div>
    </div>
    <div class="sec-overview-grid">
      <div class="cfg-panel sec-overview-card">
        <div class="cfg-panel-title">${esc(secOvT("sec.section_vuln", "漏洞运营"))}</div>
        <ul class="sec-overview-list">
          <li><span>${esc(secOvT("sec.tab_host", "主机安全"))}</span><strong>${d.open_critical || 0}</strong> ${esc(secOvT("sec.ov_crit_short", "危急"))} · <strong>${scans.host_running || 0}</strong> ${esc(secOvT("sec.ov_running_short", "进行中"))}</li>
          <li><span>${esc(secOvT("sec.tab_web", "Web 扫描"))}</span><strong>${d.open_high || 0}</strong> ${esc(secOvT("sec.ov_high_short", "高危+"))} · <strong>${scans.web_running || 0}</strong> ${esc(secOvT("sec.ov_running_short", "进行中"))}</li>
        </ul>
        <div class="sec-overview-actions">
          <button type="button" class="btn sm" data-sec-goto="host-security">${esc(secOvT("sec.tab_host", "主机安全"))}</button>
          <button type="button" class="btn sm" data-sec-goto="web-security">${esc(secOvT("sec.tab_web", "Web 扫描"))}</button>
        </div>
      </div>
      <div class="cfg-panel sec-overview-card">
        <div class="cfg-panel-title">${esc(secOvT("sec.ov_schedule", "调度健康"))}</div>
        <ul class="sec-overview-list">
          <li><span>${esc(secOvT("sec.tab_host", "主机安全"))}</span>${hostSched.enabled ? esc(secOvT("sec.ov_sched_on", "已启用")) + (hostSched.kind ? ` (${esc(hostSched.kind)})` : "") : esc(secOvT("sec.ov_sched_off", "未启用"))}</li>
          <li><span>${esc(secOvT("sec.tab_web", "Web 扫描"))}</span>${webSched.scheduled_targets || 0}/${webSched.total_targets || 0} ${esc(secOvT("sec.ov_web_sched", "目标已排程"))}</li>
          <li><span>${esc(secOvT("sec.ov_stuck", "疑似卡住"))}</span>${scans.host_stuck || 0} ${esc(secOvT("sec.tab_host", "主机"))} · ${scans.web_stuck || 0} ${esc(secOvT("sec.tab_web", "Web"))}</li>
        </ul>
      </div>
    </div>
    <div class="sec-overview-actions">
      <button type="button" class="btn" id="secOverviewRefresh">${esc(secOvT("common.refresh", "刷新"))}</button>
    </div>
  </div>`;

  el.querySelectorAll("[data-sec-goto]").forEach(btn => {
    btn.addEventListener("click", () => {
      const v = btn.getAttribute("data-sec-goto");
      if (v && typeof switchView === "function") switchView(v);
    });
  });
  const refBtn = el.querySelector("#secOverviewRefresh");
  if (refBtn) refBtn.addEventListener("click", () => loadSecurityOverview());
}

function secOverviewMaybePoll() {
  if (SEC_OVERVIEW_POLL) clearInterval(SEC_OVERVIEW_POLL);
  const panel = $("view-security-overview");
  if (!panel || !panel.classList.contains("active")) return;
  const running = (SEC_OVERVIEW && SEC_OVERVIEW.scans && SEC_OVERVIEW.scans.total_running) || 0;
  if (running <= 0) return;
  SEC_OVERVIEW_POLL = setInterval(() => {
    const p = $("view-security-overview");
    if (!p || !p.classList.contains("active")) {
      clearInterval(SEC_OVERVIEW_POLL);
      SEC_OVERVIEW_POLL = null;
      return;
    }
    loadSecurityOverview();
  }, 8000);
}

window._pageRenderers = window._pageRenderers || {};
window._pageRenderers["security-overview"] = loadSecurityOverview;
