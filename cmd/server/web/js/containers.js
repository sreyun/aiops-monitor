/* containers.js — 主机 Docker/Podman 容器资源视图 */

let CT_INVENTORIES = [];

async function loadContainersPanel() {
  const panel = $("containersPanel");
  if (!panel) return;
  panel.innerHTML = `<div class="hint">${esc(I18N.t("ui.loading", "加载中…"))}</div>`;
  try {
    const r = await fetch(`${API}/containers/list`, { credentials: "same-origin" });
    const j = await r.json().catch(() => ({}));
    if (!r.ok) throw new Error(j.error || `HTTP ${r.status}`);
    CT_INVENTORIES = j.inventories || [];
    renderContainersPanel();
  } catch (e) {
    panel.innerHTML = `<div class="hint">${esc(String(e.message || e))}</div>`;
  }
}

function renderContainersPanel() {
  const panel = $("containersPanel");
  if (!panel) return;
  if (!CT_INVENTORIES.length) {
    panel.innerHTML = `<div class="empty-line">${esc(I18N.t("containers.empty", "暂无容器数据。请确认主机已安装 Docker/Podman，并更新 Agent。"))}</div>`;
    return;
  }
  const allowWrite = typeof canWrite === "function" && canWrite();
  const rows = [];
  CT_INVENTORIES.forEach(inv => {
    (inv.containers || []).forEach(c => {
      const acts = allowWrite
        ? `<button type="button" class="btn sm" data-ct-act="start" data-host="${esc(inv.host_id)}" data-id="${esc(c.id)}" data-name="${esc(c.name)}">Start</button>
           <button type="button" class="btn sm" data-ct-act="stop" data-host="${esc(inv.host_id)}" data-id="${esc(c.id)}" data-name="${esc(c.name)}">Stop</button>
           <button type="button" class="btn sm" data-ct-act="restart" data-host="${esc(inv.host_id)}" data-id="${esc(c.id)}" data-name="${esc(c.name)}">Restart</button>
           <button type="button" class="btn sm" data-ct-log="1" data-host="${esc(inv.host_id)}" data-id="${esc(c.id)}" data-name="${esc(c.name)}">${esc(I18N.t("containers.logs", "日志"))}</button>`
        : `<button type="button" class="btn sm" data-ct-log="1" data-host="${esc(inv.host_id)}" data-id="${esc(c.id)}" data-name="${esc(c.name)}">${esc(I18N.t("containers.logs", "日志"))}</button>`;
      rows.push(`<tr>
        <td>${esc(inv.host_name || inv.host_id)}</td>
        <td class="mono">${esc(c.name || c.id)}</td>
        <td class="mono">${esc(c.image || "")}</td>
        <td>${esc(c.state || c.status || "")}</td>
        <td class="mono">${esc(c.ports || "—")}</td>
        <td>${acts}</td>
      </tr>`);
    });
  });
  panel.innerHTML = `<div class="section-title" style="margin-bottom:10px">
      <span data-i18n="nav.containers">容器</span>
      <span class="tag">${esc(I18N.t("containers.tag", "主机 Docker / Podman · 启停与日志"))}</span>
      <button type="button" class="btn sm" style="margin-left:auto" id="ctRefreshBtn">${esc(I18N.t("ui.refresh", "刷新"))}</button>
    </div>
    <div class="cfg-table-wrap"><table class="data-table" style="width:100%">
      <thead><tr>
        <th>${esc(I18N.t("containers.col_host", "主机"))}</th>
        <th>${esc(I18N.t("containers.col_name", "名称"))}</th>
        <th>${esc(I18N.t("containers.col_image", "镜像"))}</th>
        <th>${esc(I18N.t("containers.col_state", "状态"))}</th>
        <th>${esc(I18N.t("containers.col_ports", "端口"))}</th>
        <th>${esc(I18N.t("containers.col_actions", "操作"))}</th>
      </tr></thead>
      <tbody>${rows.join("") || `<tr><td colspan="6">${esc(I18N.t("containers.empty", "暂无容器"))}</td></tr>`}</tbody>
    </table></div>
    <div class="mask" id="ctLogMask" data-close>
      <div class="modal wide">
        <div class="modal-head"><h3 id="ctLogTitle">${esc(I18N.t("containers.logs", "容器日志"))}</h3>
          <button class="btn ghost close" data-close-btn aria-label="关闭">✕</button></div>
        <div class="modal-body"><pre id="ctLogBody" class="mono k8s-log-body"></pre></div>
      </div>
    </div>`;
  $("ctRefreshBtn")?.addEventListener("click", () => loadContainersPanel());
  panel.querySelectorAll("[data-ct-act]").forEach(b => {
    b.addEventListener("click", async () => {
      const act = b.getAttribute("data-ct-act");
      const host = b.getAttribute("data-host");
      const id = b.getAttribute("data-id");
      const name = b.getAttribute("data-name") || id;
      if (!confirm(I18N.t("containers.act_confirm", "确认对容器「{name}」执行 {action}？").replace("{name}", name).replace("{action}", act))) return;
      try {
        const r = await fetch(`${API}/containers/${encodeURIComponent(host)}/${encodeURIComponent(id)}/action`, {
          method: "POST", credentials: "same-origin",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ action: act, name }),
        });
        const j = await r.json().catch(() => ({}));
        if (!r.ok) throw new Error(j.error || `HTTP ${r.status}`);
        toast(I18N.t("containers.act_ok", "操作已提交"), "ok");
        setTimeout(() => loadContainersPanel(), 1200);
      } catch (e) { toast(String(e.message || e), "err"); }
    });
  });
  panel.querySelectorAll("[data-ct-log]").forEach(b => {
    b.addEventListener("click", async () => {
      const host = b.getAttribute("data-host");
      const id = b.getAttribute("data-id");
      const name = b.getAttribute("data-name") || id;
      const title = $("ctLogTitle");
      const body = $("ctLogBody");
      if (title) title.textContent = `${I18N.t("containers.logs", "日志")} · ${name}`;
      if (body) body.textContent = I18N.t("ui.loading", "加载中…");
      $("ctLogMask")?.classList.add("show");
      try {
        const r = await fetch(`${API}/containers/${encodeURIComponent(host)}/${encodeURIComponent(id)}/logs?tail=300`, { credentials: "same-origin" });
        const j = await r.json().catch(() => ({}));
        if (!r.ok) throw new Error(j.error || `HTTP ${r.status}`);
        if (body) body.textContent = j.log || "(empty)";
      } catch (e) {
        if (body) body.textContent = String(e.message || e);
      }
    });
  });
}

window._pageRenderers = window._pageRenderers || {};
window._pageRenderers.containers = loadContainersPanel;
