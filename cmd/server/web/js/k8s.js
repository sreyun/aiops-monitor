/* k8s.js — 服务端直连 Kubernetes 资源视图 */

let K8S_CLUSTERS = [];
let K8S_TAB = "overview";
let K8S_SCALE_CTX = null;

function k8sClusterId() {
  return ($("k8sClusterSel")?.value || "").trim();
}

function k8sNamespace() {
  return ($("k8sNsSel")?.value || "").trim();
}

function k8sSetStatus(text, kind) {
  const el = $("k8sStatusChip");
  if (!el) return;
  el.textContent = text || "";
  el.classList.remove("ok", "warn", "crit");
  if (kind) el.classList.add(kind === "err" ? "crit" : kind);
}

async function k8sFetch(path, opts) {
  const r = await fetch(`${API}${path}`, Object.assign({ credentials: "same-origin" }, opts || {}));
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error || (`HTTP ${r.status}`));
  return j;
}

async function loadK8sClusters() {
  const j = await k8sFetch("/k8s/clusters");
  K8S_CLUSTERS = j.clusters || [];
  const sel = $("k8sClusterSel");
  if (!sel) return;
  const prev = sel.value;
  if (!K8S_CLUSTERS.length) {
    sel.innerHTML = `<option value="">${esc(I18N.t("k8s.no_cluster", "尚未配置集群"))}</option>`;
    return;
  }
  sel.innerHTML = K8S_CLUSTERS.map(c =>
    `<option value="${esc(c.id)}" ${c.enabled ? "" : "disabled"}>${esc(c.name)}${c.enabled ? "" : " (disabled)"}</option>`
  ).join("");
  if (prev && K8S_CLUSTERS.some(c => c.id === prev)) sel.value = prev;
}

async function loadK8sNamespaces() {
  const id = k8sClusterId();
  const sel = $("k8sNsSel");
  if (!sel) return;
  const keep = sel.value;
  sel.innerHTML = `<option value="">${esc(I18N.t("k8s.all_ns", "全部命名空间"))}</option>`;
  if (!id) return;
  try {
    const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/namespaces`);
    (j.items || []).forEach(ns => {
      const o = document.createElement("option");
      o.value = ns.name || "";
      o.textContent = ns.name || "";
      sel.appendChild(o);
    });
    if (keep) sel.value = keep;
  } catch (_) { /* ignore */ }
}

function switchK8sTab(tab) {
  K8S_TAB = tab || "overview";
  document.querySelectorAll("#k8sInnerTabs .tab").forEach(b => b.classList.toggle("active", b.dataset.k8sTab === K8S_TAB));
  renderK8sPanel();
}

async function renderK8sPanel() {
  const panel = $("k8sPanel");
  if (!panel) return;
  const id = k8sClusterId();
  if (!id) {
    panel.innerHTML = `<div class="hint">${esc(I18N.t("k8s.hint_add", "请管理员在「集群配置」中添加 Kubernetes 集群（API Server + Token 或 kubeconfig）。"))}</div>`;
    k8sSetStatus(I18N.t("k8s.status_none", "未选择集群"), "warn");
    if (K8S_TAB === "config" && typeof isAdmin === "function" && isAdmin()) {
      panel.innerHTML = renderK8sConfigForm();
      wireK8sConfigForm();
    }
    return;
  }
  if (K8S_TAB === "config") {
    panel.innerHTML = renderK8sConfigForm();
    wireK8sConfigForm();
    return;
  }
  panel.innerHTML = `<div class="hint">${esc(I18N.t("sec.loading", "加载中…"))}</div>`;
  const nsQ = k8sNamespace() ? `?namespace=${encodeURIComponent(k8sNamespace())}` : "";
  try {
    if (K8S_TAB === "overview") {
      const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/overview`);
      const ver = (j.version && (j.version.gitVersion || j.version.major)) || "—";
      k8sSetStatus(I18N.t("k8s.status_ok", "已连接") + " · " + ver, "ok");
      panel.innerHTML = `<div class="k8s-kpi-row">
        <div class="k8s-kpi"><div class="k8s-kpi-l">Nodes</div><div class="k8s-kpi-v">${esc(String((j.nodes||{}).ready||0))}<span>/${esc(String((j.nodes||{}).total||0))}</span></div></div>
        <div class="k8s-kpi"><div class="k8s-kpi-l">Pods</div><div class="k8s-kpi-v">${esc(String((j.pods||{}).running||0))}<span>/${esc(String((j.pods||{}).total||0))}</span></div></div>
        <div class="k8s-kpi"><div class="k8s-kpi-l">Deployments</div><div class="k8s-kpi-v">${esc(String((j.deployments||{}).total||0))}</div></div>
        <div class="k8s-kpi"><div class="k8s-kpi-l">Version</div><div class="k8s-kpi-v mono" style="font-size:14px">${esc(String(ver))}</div></div>
      </div>`;
      return;
    }
    if (K8S_TAB === "nodes") {
      const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/nodes`);
      k8sSetStatus(I18N.t("k8s.status_ok", "已连接"), "ok");
      panel.innerHTML = k8sTable(
        [I18N.t("k8s.col_name", "名称"), I18N.t("k8s.col_ready", "状态"), "IP", I18N.t("k8s.col_linked_host", "关联主机")],
        (j.items || []).map(n => [n.name, n.ready, n.internal_ip || "—", n.linked_host_name || n.linked_host_id || "—"])
      );
      return;
    }
    if (K8S_TAB === "pods") {
      const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/pods${nsQ}`);
      k8sSetStatus(I18N.t("k8s.status_ok", "已连接"), "ok");
      const rows = (j.items || []).map(p => {
        const logBtn = `<button type="button" class="btn sm" data-k8s-log="${esc(p.namespace)}|${esc(p.name)}">${esc(I18N.t("k8s.view_log", "日志"))}</button>`;
        return [p.namespace, p.name, p.phase, p.node || "—", p.linked_host_name || p.linked_host_id || "—", p.ip || "—", logBtn];
      });
      panel.innerHTML = k8sTable(
        [I18N.t("k8s.col_ns", "命名空间"), I18N.t("k8s.col_name", "名称"), I18N.t("k8s.col_phase", "Phase"), "Node", I18N.t("k8s.col_linked_host", "关联主机"), "IP", ""],
        rows, true
      );
      panel.querySelectorAll("[data-k8s-log]").forEach(b => {
        b.addEventListener("click", () => {
          const [ns, name] = (b.getAttribute("data-k8s-log") || "").split("|");
          openK8sPodLog(ns, name);
        });
      });
      return;
    }
    if (K8S_TAB === "deployments") {
      const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/deployments${nsQ}`);
      k8sSetStatus(I18N.t("k8s.status_ok", "已连接"), "ok");
      const allowMutate = (typeof canWrite === "function" && canWrite()) || (typeof isAdmin === "function" && isAdmin());
      const rows = (j.items || []).map(d => {
        const acts = allowMutate
          ? `<button type="button" class="btn sm" data-k8s-scale="${esc(d.namespace)}|${esc(d.name)}|${esc(String(d.replicas||0))}">Scale</button>
             <button type="button" class="btn sm" data-k8s-restart="${esc(d.namespace)}|${esc(d.name)}">Restart</button>`
          : "—";
        return [d.namespace, d.name, `${d.ready||0}/${d.replicas||0}`, d.available||0, acts];
      });
      panel.innerHTML = k8sTable(
        [I18N.t("k8s.col_ns", "命名空间"), I18N.t("k8s.col_name", "名称"), I18N.t("k8s.col_replicas", "Ready/Desired"), I18N.t("k8s.col_available", "Available"), I18N.t("k8s.col_actions", "操作")],
        rows, true
      );
      panel.querySelectorAll("[data-k8s-scale]").forEach(b => {
        b.addEventListener("click", () => {
          const [ns, name, reps] = (b.getAttribute("data-k8s-scale") || "").split("|");
          openK8sScale(ns, name, parseInt(reps, 10) || 0);
        });
      });
      panel.querySelectorAll("[data-k8s-restart]").forEach(b => {
        b.addEventListener("click", async () => {
          const [ns, name] = (b.getAttribute("data-k8s-restart") || "").split("|");
          if (!confirm(I18N.t("k8s.restart_confirm", "确认对 Deployment 执行 rollout restart？").replace("{name}", name))) return;
          try {
            await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/deployments/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/restart`, {
              method: "POST", headers: { "Content-Type": "application/json" }, body: "{}",
            });
            toast(I18N.t("k8s.restart_ok", "已触发 Restart"), "ok");
            renderK8sPanel();
          } catch (e) { toast(String(e.message || e), "err"); }
        });
      });
      return;
    }
    if (K8S_TAB === "events") {
      const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/events${nsQ}`);
      k8sSetStatus(I18N.t("k8s.status_ok", "已连接"), "ok");
      panel.innerHTML = k8sTable(
        [I18N.t("k8s.col_ns", "命名空间"), "Type", "Reason", "Object", I18N.t("k8s.col_message", "消息"), "Count"],
        (j.items || []).map(e => [e.namespace || "—", e.type, e.reason, e.object || "—", e.message || "", e.count || 0])
      );
      return;
    }
  } catch (e) {
    k8sSetStatus(I18N.t("k8s.status_err", "连接失败"), "err");
    panel.innerHTML = `<div class="hint">${esc(String(e.message || e))}</div>`;
  }
}

function k8sTable(headers, rows, htmlCells) {
  if (!rows.length) return `<div class="hint">${esc(I18N.t("k8s.empty", "暂无数据"))}</div>`;
  const th = headers.map(h => `<th>${esc(h)}</th>`).join("");
  const tr = rows.map(r => `<tr>${r.map((c, i) => {
    const last = i === r.length - 1 && htmlCells;
    return `<td>${last ? c : esc(String(c ?? ""))}</td>`;
  }).join("")}</tr>`).join("");
  return `<div class="cfg-table-wrap"><table class="data-table" style="width:100%"><thead><tr>${th}</tr></thead><tbody>${tr}</tbody></table></div>`;
}

async function openK8sPodLog(ns, name) {
  const id = k8sClusterId();
  if (!id || !ns || !name) return;
  const title = $("k8sLogTitle");
  const body = $("k8sLogBody");
  if (title) title.textContent = `Pod ${ns}/${name}`;
  if (body) body.textContent = I18N.t("sec.loading", "加载中…");
  $("k8sLogMask")?.classList.add("show");
  try {
    const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/pods/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/log?tail=300`);
    if (body) body.textContent = j.log || "(empty)";
  } catch (e) {
    if (body) body.textContent = String(e.message || e);
  }
}

function openK8sScale(ns, name, current) {
  K8S_SCALE_CTX = { ns, name };
  const hint = $("k8sScaleHint");
  if (hint) hint.textContent = `${ns}/${name} · ${I18N.t("k8s.current_replicas", "当前副本")} ${current}`;
  if ($("k8sScaleReplicas")) $("k8sScaleReplicas").value = String(current);
  $("k8sScaleMask")?.classList.add("show");
}

function renderK8sConfigForm() {
  if (typeof isAdmin === "function" && !isAdmin()) {
    return `<div class="hint">${esc(I18N.t("toast.admin_only", "仅管理员可操作"))}</div>`;
  }
  const list = K8S_CLUSTERS.map(c => `<tr>
    <td>${esc(c.name)}</td>
    <td class="mono">${esc(c.api_server || (c.kubeconfig_yaml ? "kubeconfig" : "—"))}</td>
    <td>${c.enabled ? "✓" : "—"}</td>
    <td>
      <button type="button" class="btn sm" data-k8s-edit="${esc(c.id)}">${esc(I18N.t("ui.edit", "编辑"))}</button>
      <button type="button" class="btn sm" data-k8s-test="${esc(c.id)}">${esc(I18N.t("k8s.test", "连通测试"))}</button>
      <button type="button" class="btn sm danger" data-k8s-del="${esc(c.id)}">${esc(I18N.t("ui.delete", "删除"))}</button>
    </td>
  </tr>`).join("");
  return `<div class="cfg-panel">
    <div class="cfg-panel-title">${esc(I18N.t("k8s.tab_config", "集群配置"))}</div>
    <p class="field-hint">${esc(I18N.t("k8s.config_hint", "推荐使用只读 ServiceAccount Token；Scale/Restart 需额外 patch 权限。密钥留空表示保持原值。"))}</p>
    <div class="cfg-table-wrap" style="margin-bottom:14px"><table class="data-table" style="width:100%">
      <thead><tr><th>${esc(I18N.t("k8s.col_name", "名称"))}</th><th>Endpoint</th><th>${esc(I18N.t("k8s.col_enabled", "启用"))}</th><th></th></tr></thead>
      <tbody>${list || `<tr><td colspan="4">${esc(I18N.t("k8s.empty", "暂无数据"))}</td></tr>`}</tbody>
    </table></div>
    <div class="cfg-form" id="k8sCfgForm">
      <input type="hidden" id="k8sCfgId" value="">
      <div class="cfg-form-row">
        <div class="field"><label>${esc(I18N.t("k8s.col_name", "名称"))}</label><input id="k8sCfgName" type="text" autocomplete="off"></div>
        <div class="field cfg-field-switch"><label class="switch"><input type="checkbox" id="k8sCfgEnabled" checked><span>${esc(I18N.t("k8s.col_enabled", "启用"))}</span></label></div>
      </div>
      <div class="field"><label>API Server</label><input id="k8sCfgAPI" class="mono" type="url" placeholder="https://kubernetes.default.svc:443" autocomplete="off"></div>
      <div class="field"><label>Token</label><input id="k8sCfgToken" class="mono" type="password" placeholder="${esc(I18N.t("sec.oidc_secret_ph", "留空保持原值"))}" autocomplete="new-password"></div>
      <div class="field"><label>CA Cert (PEM)</label><textarea id="k8sCfgCA" class="mono" rows="3" spellcheck="false"></textarea></div>
      <label class="switch mb"><input type="checkbox" id="k8sCfgInsecure"><span>${esc(I18N.t("k8s.insecure", "跳过 TLS 校验（仅内网临时）"))}</span></label>
      <div class="field"><label>Kubeconfig YAML（可选，优先于上方字段）</label><textarea id="k8sCfgKube" class="mono" rows="5" spellcheck="false" placeholder="粘贴 kubeconfig；已配置时显示 ****"></textarea></div>
      <div class="field"><label>${esc(I18N.t("k8s.default_ns", "默认命名空间（空=全部）"))}</label><input id="k8sCfgNS" class="mono" type="text" placeholder="default"></div>
      <div class="cfg-actions">
        <button type="button" class="btn" id="k8sCfgReset">${esc(I18N.t("ui.reset", "重置"))}</button>
        <button type="button" class="btn primary" id="k8sCfgSave">${esc(I18N.t("settings.save", "保存"))}</button>
      </div>
    </div>
  </div>`;
}

function wireK8sConfigForm() {
  const fill = (c) => {
    c = c || {};
    if ($("k8sCfgId")) $("k8sCfgId").value = c.id || "";
    if ($("k8sCfgName")) $("k8sCfgName").value = c.name || "";
    if ($("k8sCfgEnabled")) $("k8sCfgEnabled").checked = c.enabled !== false;
    if ($("k8sCfgAPI")) $("k8sCfgAPI").value = c.api_server || "";
    if ($("k8sCfgToken")) $("k8sCfgToken").value = "";
    if ($("k8sCfgCA")) $("k8sCfgCA").value = c.ca_cert || "";
    if ($("k8sCfgInsecure")) $("k8sCfgInsecure").checked = !!c.insecure_skip_tls;
    if ($("k8sCfgKube")) $("k8sCfgKube").value = "";
    if ($("k8sCfgNS")) $("k8sCfgNS").value = c.default_namespace || "";
  };
  $("k8sCfgReset")?.addEventListener("click", () => fill(null));
  $("k8sCfgSave")?.addEventListener("click", async () => {
    const body = {
      id: ($("k8sCfgId")?.value || "").trim(),
      name: ($("k8sCfgName")?.value || "").trim(),
      enabled: !!$("k8sCfgEnabled")?.checked,
      api_server: ($("k8sCfgAPI")?.value || "").trim(),
      token: ($("k8sCfgToken")?.value || "").trim(),
      ca_cert: ($("k8sCfgCA")?.value || "").trim(),
      insecure_skip_tls: !!$("k8sCfgInsecure")?.checked,
      kubeconfig_yaml: ($("k8sCfgKube")?.value || "").trim(),
      default_namespace: ($("k8sCfgNS")?.value || "").trim(),
    };
    try {
      const path = body.id ? `/k8s/clusters/${encodeURIComponent(body.id)}` : "/k8s/clusters";
      const method = body.id ? "PUT" : "POST";
      await k8sFetch(path, { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      toast(I18N.t("toast.saved", "已保存"), "ok");
      await loadK8sPage();
    } catch (e) { toast(String(e.message || e), "err"); }
  });
  document.querySelectorAll("[data-k8s-edit]").forEach(b => {
    b.addEventListener("click", () => {
      const c = K8S_CLUSTERS.find(x => x.id === b.getAttribute("data-k8s-edit"));
      fill(c);
    });
  });
  document.querySelectorAll("[data-k8s-test]").forEach(b => {
    b.addEventListener("click", async () => {
      try {
        const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(b.getAttribute("data-k8s-test"))}/test`, { method: "POST", body: "{}" });
        const ver = j.version?.gitVersion || "ok";
        toast(I18N.t("k8s.test_ok", "连通成功") + " · " + ver, "ok");
      } catch (e) { toast(String(e.message || e), "err"); }
    });
  });
  document.querySelectorAll("[data-k8s-del]").forEach(b => {
    b.addEventListener("click", async () => {
      if (!confirm(I18N.t("k8s.del_confirm", "确定删除该集群配置？"))) return;
      try {
        await k8sFetch(`/k8s/clusters/${encodeURIComponent(b.getAttribute("data-k8s-del"))}`, { method: "DELETE" });
        toast(I18N.t("toast.deleted", "已删除"), "ok");
        await loadK8sPage();
      } catch (e) { toast(String(e.message || e), "err"); }
    });
  });
}

async function loadK8sPage() {
  try {
    await loadK8sClusters();
    await loadK8sNamespaces();
    await renderK8sPanel();
  } catch (e) {
    k8sSetStatus(I18N.t("k8s.status_err", "连接失败"), "err");
    const panel = $("k8sPanel");
    if (panel) panel.innerHTML = `<div class="hint">${esc(String(e.message || e))}</div>`;
  }
}

window._pageRenderers = window._pageRenderers || {};
window._pageRenderers.k8s = loadK8sPage;

document.addEventListener("DOMContentLoaded", () => {
  document.querySelectorAll("#k8sInnerTabs .tab").forEach(b => {
    b.addEventListener("click", () => switchK8sTab(b.dataset.k8sTab));
  });
  $("k8sRefreshBtn")?.addEventListener("click", () => loadK8sPage());
  $("k8sClusterSel")?.addEventListener("change", async () => {
    await loadK8sNamespaces();
    await renderK8sPanel();
  });
  $("k8sNsSel")?.addEventListener("change", () => renderK8sPanel());
  $("k8sScaleConfirm")?.addEventListener("click", async () => {
    const id = k8sClusterId();
    if (!id || !K8S_SCALE_CTX) return;
    const replicas = parseInt($("k8sScaleReplicas")?.value || "0", 10);
    if (!confirm(I18N.t("k8s.scale_confirm_q", "确认将 {name} 调整为 {n} 副本？")
      .replace("{name}", K8S_SCALE_CTX.name).replace("{n}", String(replicas)))) return;
    try {
      await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/deployments/${encodeURIComponent(K8S_SCALE_CTX.ns)}/${encodeURIComponent(K8S_SCALE_CTX.name)}/scale`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ replicas }),
      });
      toast(I18N.t("k8s.scale_ok", "Scale 已提交"), "ok");
      $("k8sScaleMask")?.classList.remove("show");
      renderK8sPanel();
    } catch (e) { toast(String(e.message || e), "err"); }
  });
});
