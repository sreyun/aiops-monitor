/* k8s.js — 服务端直连 Kubernetes 资源视图（商用台账） */

let K8S_CLUSTERS = [];
let K8S_TAB = "overview";
let K8S_SCALE_CTX = null;
const K8S_FILTER = { q: "", phase: "all" }; // phase: all|running|pending|failed|other
let K8S_CACHE = { nodes: [], pods: [], deployments: [], events: [] };

const k8sT = (k, fb) => I18N.t(k, fb);

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

function k8sPhaseKey(phase) {
  const p = String(phase || "").toLowerCase();
  if (p === "running" || p === "succeeded") return "running";
  if (p === "pending" || p === "containercreating") return "pending";
  if (p === "failed" || p === "error" || p === "crashloopbackoff" || p === "unknown") return "failed";
  return "other";
}
function k8sPhaseBadge(phase) {
  const key = k8sPhaseKey(phase);
  const cls = { running: "ok", pending: "warn", failed: "crit", other: "info" }[key] || "info";
  const lab = {
    running: k8sT("k8s.phase_running", "Running"),
    pending: k8sT("k8s.phase_pending", "Pending"),
    failed: k8sT("k8s.phase_failed", "Failed"),
    other: phase || "—",
  }[key];
  return `<span class="badge ${cls}" title="${esc(phase || "")}">${esc(key === "other" ? (phase || "—") : lab)}</span>`;
}
function k8sReadyBadge(ready) {
  const ok = String(ready).toLowerCase() === "true" || ready === true || String(ready).toLowerCase() === "ready";
  const text = ok ? k8sT("k8s.ready_yes", "Ready") : k8sT("k8s.ready_no", "NotReady");
  return `<span class="badge ${ok ? "ok" : "crit"}">${esc(text)}</span>`;
}
function k8sEventTypeBadge(type) {
  const t = String(type || "").toLowerCase();
  if (t === "warning") return `<span class="badge warn">${esc(type)}</span>`;
  if (t === "normal") return `<span class="badge ok">${esc(type)}</span>`;
  return `<span class="badge">${esc(type || "—")}</span>`;
}

function k8sMatch(hay, q) {
  if (!q) return true;
  return typeof matchesSearchTokens === "function"
    ? matchesSearchTokens(hay, q)
    : String(hay).toLowerCase().includes(String(q).toLowerCase());
}

function k8sToolbarHTML(opts) {
  opts = opts || {};
  const showPhase = !!opts.phase;
  let html = `<div class="rtx-toolbar k8s-toolbar">
    <input type="search" id="k8sSearch" class="hw-search" placeholder="${esc(k8sT("k8s.search_ph", "搜索名称 / 命名空间 / 节点 / IP…"))}" value="${esc(K8S_FILTER.q)}" autocomplete="off">`;
  if (showPhase) {
    html += `<div class="select-wrap"><select id="k8sPhaseFilter">
      <option value="all"${K8S_FILTER.phase === "all" ? " selected" : ""}>${esc(k8sT("k8s.filter_all_phase", "全部 Phase"))}</option>
      <option value="running"${K8S_FILTER.phase === "running" ? " selected" : ""}>${esc(k8sT("k8s.phase_running", "Running"))}</option>
      <option value="pending"${K8S_FILTER.phase === "pending" ? " selected" : ""}>${esc(k8sT("k8s.phase_pending", "Pending"))}</option>
      <option value="failed"${K8S_FILTER.phase === "failed" ? " selected" : ""}>${esc(k8sT("k8s.phase_failed", "Failed"))}</option>
      <option value="other"${K8S_FILTER.phase === "other" ? " selected" : ""}>${esc(k8sT("k8s.phase_other", "其他"))}</option>
    </select></div>`;
  }
  if (opts.count != null) html += `<span class="rtx-count tag">${opts.count}</span>`;
  html += `</div>`;
  return html;
}

function k8sWireToolbar(refilter) {
  const search = $("k8sSearch");
  if (search) {
    search.addEventListener("input", () => {
      K8S_FILTER.q = search.value || "";
      refilter();
      const el = $("k8sSearch");
      if (el) { el.focus(); try { el.setSelectionRange(el.value.length, el.value.length); } catch (_) {} }
    });
  }
  const phase = $("k8sPhaseFilter");
  if (phase) {
    phase.addEventListener("change", () => {
      K8S_FILTER.phase = phase.value || "all";
      refilter();
    });
  }
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
    sel.innerHTML = `<option value="">${esc(k8sT("k8s.no_cluster", "尚未配置集群"))}</option>`;
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
  sel.innerHTML = `<option value="">${esc(k8sT("k8s.all_ns", "全部命名空间"))}</option>`;
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
  K8S_FILTER.q = "";
  K8S_FILTER.phase = "all";
  document.querySelectorAll("#k8sInnerTabs .tab").forEach(b => b.classList.toggle("active", b.dataset.k8sTab === K8S_TAB));
  renderK8sPanel();
}

function k8sTable(headers, rows, htmlCells) {
  if (!rows.length) {
    return `<div class="empty-state k8s-empty"><h4>${esc(k8sT("k8s.empty", "暂无数据"))}</h4>
      <p>${esc(k8sT("k8s.empty_hint", "尝试切换命名空间，或清空搜索条件。"))}</p></div>`;
  }
  const th = headers.map(h => `<th>${esc(h)}</th>`).join("");
  const tr = rows.map(r => `<tr>${r.map((c, i) => {
    const last = i === r.length - 1 && htmlCells;
    return `<td>${last ? c : esc(String(c ?? ""))}</td>`;
  }).join("")}</tr>`).join("");
  return `<div class="nf-table-wrap k8s-table-wrap"><table class="data-table k8s-table"><thead><tr>${th}</tr></thead><tbody>${tr}</tbody></table></div>`;
}

function paintK8sNodes() {
  const panel = $("k8sPanel");
  if (!panel) return;
  const q = K8S_FILTER.q;
  const items = (K8S_CACHE.nodes || []).filter(n => {
    const hay = [n.name, n.ready, n.internal_ip, n.linked_host_name, n.linked_host_id].join(" ");
    return k8sMatch(hay, q);
  });
  let body;
  if (!items.length) {
    body = `<div class="empty-state k8s-empty"><h4>${esc(k8sT("k8s.empty", "暂无数据"))}</h4><p>${esc(k8sT("k8s.empty_hint", "尝试切换命名空间，或清空搜索条件。"))}</p></div>`;
  } else {
    const th = [k8sT("k8s.col_name", "名称"), k8sT("k8s.col_ready", "状态"), k8sT("k8s.col_ip", "IP"), k8sT("k8s.col_linked_host", "关联主机")]
      .map(h => `<th>${esc(h)}</th>`).join("");
    const tr = items.map(n => `<tr>
      <td class="mono">${esc(n.name)}</td>
      <td>${k8sReadyBadge(n.ready)}</td>
      <td class="mono">${esc(n.internal_ip || "—")}</td>
      <td>${esc(n.linked_host_name || n.linked_host_id || "—")}</td>
    </tr>`).join("");
    body = `<div class="nf-table-wrap k8s-table-wrap"><table class="data-table k8s-table"><thead><tr>${th}</tr></thead><tbody>${tr}</tbody></table></div>`;
  }
  panel.innerHTML = k8sToolbarHTML({ count: `${items.length}/${K8S_CACHE.nodes.length}` }) + body;
  k8sWireToolbar(paintK8sNodes);
}

function paintK8sPods() {
  const panel = $("k8sPanel");
  if (!panel) return;
  const q = K8S_FILTER.q;
  const items = (K8S_CACHE.pods || []).filter(p => {
    if (K8S_FILTER.phase !== "all" && k8sPhaseKey(p.phase) !== K8S_FILTER.phase) return false;
    const hay = [p.namespace, p.name, p.phase, p.node, p.linked_host_name, p.linked_host_id, p.ip].join(" ");
    return k8sMatch(hay, q);
  });
  const th = [k8sT("k8s.col_ns", "命名空间"), k8sT("k8s.col_name", "名称"), k8sT("k8s.col_phase", "Phase"),
    k8sT("k8s.col_node", "节点"), k8sT("k8s.col_linked_host", "关联主机"), k8sT("k8s.col_ip", "IP"), k8sT("k8s.col_actions", "操作")]
    .map(h => `<th>${esc(h)}</th>`).join("");
  let body;
  if (!items.length) {
    body = `<div class="empty-state k8s-empty"><h4>${esc(k8sT("k8s.empty", "暂无数据"))}</h4><p>${esc(k8sT("k8s.empty_hint", "尝试切换命名空间，或清空搜索条件。"))}</p></div>`;
  } else {
    const allowMutate = (typeof canWrite === "function" && canWrite()) || (typeof isAdmin === "function" && isAdmin());
    const tr = items.map(p => {
      const acts = [`<button type="button" class="btn sm" data-k8s-log="${esc(p.namespace)}|${esc(p.name)}">${esc(k8sT("k8s.view_log", "日志"))}</button>`];
      if (allowMutate) {
        acts.push(`<button type="button" class="btn sm" data-k8s-exec="${esc(p.namespace)}|${esc(p.name)}">${esc(k8sT("k8s.act_exec", "Exec"))}</button>`);
        acts.push(`<button type="button" class="btn sm danger" data-k8s-delpod="${esc(p.namespace)}|${esc(p.name)}">${esc(k8sT("k8s.act_del_pod", "删除"))}</button>`);
      }
      return `<tr>
        <td class="mono">${esc(p.namespace)}</td>
        <td class="mono"><strong>${esc(p.name)}</strong></td>
        <td>${k8sPhaseBadge(p.phase)}</td>
        <td class="mono">${esc(p.node || "—")}</td>
        <td>${esc(p.linked_host_name || p.linked_host_id || "—")}</td>
        <td class="mono">${esc(p.ip || "—")}</td>
        <td><div class="k8s-actions">${acts.join("")}</div></td>
      </tr>`;
    }).join("");
    body = `<div class="nf-table-wrap k8s-table-wrap"><table class="data-table k8s-table"><thead><tr>${th}</tr></thead><tbody>${tr}</tbody></table></div>`;
  }
  const id = k8sClusterId();
  panel.innerHTML = k8sToolbarHTML({ phase: true, count: `${items.length}/${K8S_CACHE.pods.length}` }) +
    `<div style="margin:0 0 10px"><button type="button" class="btn sm ai-assist-btn" id="k8sPodsAI"><span class="ai-assist-btn-ic">🤖</span>${esc(k8sT("ai.analyze", "AI 分析"))}</button></div>` + body;
  k8sWireToolbar(paintK8sPods);
  panel.querySelectorAll("[data-k8s-log]").forEach(b => {
    b.addEventListener("click", () => {
      const [ns, name] = (b.getAttribute("data-k8s-log") || "").split("|");
      openK8sPodLog(ns, name);
    });
  });
  panel.querySelectorAll("[data-k8s-delpod]").forEach(b => {
    b.addEventListener("click", async () => {
      const [ns, name] = (b.getAttribute("data-k8s-delpod") || "").split("|");
      if (!confirm(k8sT("k8s.del_pod_confirm", "确认删除 Pod「{name}」？由控制器管理的 Pod 会被重建。").replace("{name}", name))) return;
      try {
        await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/pods/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`, { method: "DELETE" });
        toast(k8sT("k8s.del_pod_ok", "已删除 Pod"), "ok");
        renderK8sPanel();
      } catch (e) { toast(String(e.message || e), "err"); }
    });
  });
  panel.querySelectorAll("[data-k8s-exec]").forEach(b => {
    b.addEventListener("click", async () => {
      const [ns, name] = (b.getAttribute("data-k8s-exec") || "").split("|");
      const cmd = prompt(k8sT("k8s.exec_prompt", "在 Pod 内执行命令（需服务端 kubectl）"), "ps aux | head -n 20");
      if (cmd === null || !String(cmd).trim()) return;
      try {
        const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/pods/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/exec`, {
          method: "POST", headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ command: String(cmd).trim() }),
        });
        alert(j.output || "(empty)");
      } catch (e) { toast(String(e.message || e), "err"); }
    });
  });
  const ai = $("k8sPodsAI");
  if (ai) ai.addEventListener("click", () => k8sOpenOpsAI("pods"));
}

function paintK8sDeployments() {
  const panel = $("k8sPanel");
  if (!panel) return;
  const id = k8sClusterId();
  const q = K8S_FILTER.q;
  const allowMutate = (typeof canWrite === "function" && canWrite()) || (typeof isAdmin === "function" && isAdmin());
  const items = (K8S_CACHE.deployments || []).filter(d => {
    const hay = [d.namespace, d.name, String(d.replicas), String(d.ready), String(d.available)].join(" ");
    return k8sMatch(hay, q);
  });
  const th = [k8sT("k8s.col_ns", "命名空间"), k8sT("k8s.col_name", "名称"), k8sT("k8s.col_replicas", "Ready/Desired"),
    k8sT("k8s.col_available", "Available"), k8sT("k8s.col_actions", "操作")].map(h => `<th>${esc(h)}</th>`).join("");
  let body;
  if (!items.length) {
    body = `<div class="empty-state k8s-empty"><h4>${esc(k8sT("k8s.empty", "暂无数据"))}</h4><p>${esc(k8sT("k8s.empty_hint", "尝试切换命名空间，或清空搜索条件。"))}</p></div>`;
  } else {
    const tr = items.map(d => {
      const ready = d.ready || 0, desired = d.replicas || 0;
      const ok = desired > 0 && ready >= desired;
      const ratio = `<span class="badge ${ok ? "ok" : (ready === 0 ? "crit" : "warn")}">${esc(ready)}/${esc(desired)}</span>`;
      const acts = allowMutate
        ? `<div class="k8s-actions">
            <button type="button" class="btn sm" data-k8s-scale="${esc(d.namespace)}|${esc(d.name)}|${esc(String(desired))}">${esc(k8sT("k8s.act_scale", "扩缩容"))}</button>
            <button type="button" class="btn sm" data-k8s-restart="${esc(d.namespace)}|${esc(d.name)}">${esc(k8sT("k8s.act_restart", "重启"))}</button>
            <button type="button" class="btn sm" data-k8s-undo="${esc(d.namespace)}|${esc(d.name)}">${esc(k8sT("k8s.act_undo", "回滚"))}</button>
          </div>`
        : "—";
      return `<tr>
        <td class="mono">${esc(d.namespace)}</td>
        <td class="mono"><strong>${esc(d.name)}</strong></td>
        <td>${ratio}</td>
        <td>${esc(d.available || 0)}</td>
        <td>${acts}</td>
      </tr>`;
    }).join("");
    body = `<div class="nf-table-wrap k8s-table-wrap"><table class="data-table k8s-table"><thead><tr>${th}</tr></thead><tbody>${tr}</tbody></table></div>`;
  }
  panel.innerHTML = k8sToolbarHTML({ count: `${items.length}/${K8S_CACHE.deployments.length}` }) + body;
  k8sWireToolbar(paintK8sDeployments);
  panel.querySelectorAll("[data-k8s-scale]").forEach(b => {
    b.addEventListener("click", () => {
      const [ns, name, reps] = (b.getAttribute("data-k8s-scale") || "").split("|");
      openK8sScale(ns, name, parseInt(reps, 10) || 0);
    });
  });
  panel.querySelectorAll("[data-k8s-restart]").forEach(b => {
    b.addEventListener("click", async () => {
      const [ns, name] = (b.getAttribute("data-k8s-restart") || "").split("|");
      if (!confirm(k8sT("k8s.restart_confirm", "确认对 Deployment 执行 rollout restart？").replace("{name}", name))) return;
      try {
        await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/deployments/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/restart`, {
          method: "POST", headers: { "Content-Type": "application/json" }, body: "{}",
        });
        toast(k8sT("k8s.restart_ok", "已触发 Restart"), "ok");
        renderK8sPanel();
      } catch (e) { toast(String(e.message || e), "err"); }
    });
  });
  panel.querySelectorAll("[data-k8s-undo]").forEach(b => {
    b.addEventListener("click", async () => {
      const [ns, name] = (b.getAttribute("data-k8s-undo") || "").split("|");
      if (!confirm(k8sT("k8s.undo_confirm", "确认对 Deployment「{name}」执行 rollout undo？").replace("{name}", name))) return;
      try {
        await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/deployments/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/undo`, {
          method: "POST", headers: { "Content-Type": "application/json" }, body: "{}",
        });
        toast(k8sT("k8s.undo_ok", "已触发回滚"), "ok");
        renderK8sPanel();
      } catch (e) { toast(String(e.message || e), "err"); }
    });
  });
}

function k8sOpenOpsAI(kind) {
  if (typeof openAIAssist !== "function") { toast(k8sT("k8s.ai_unavailable", "AI 面板未就绪"), "err"); return; }
  const ctx = {
    cluster_id: k8sClusterId(),
    tab: kind || K8S_TAB,
    pods: (K8S_CACHE.pods || []).slice(0, 40),
    deployments: (K8S_CACHE.deployments || []).slice(0, 40),
    events: (K8S_CACHE.events || []).slice(0, 30),
  };
  openAIAssist({
    task: "k8s_ops_plan",
    title: k8sT("k8s.ai_title", "K8s 运维建议"),
    mode: "generate",
    context: JSON.stringify(ctx, null, 2),
    placeholder: k8sT("k8s.ai_ph", "例如：某 Deployment 副本不足，给出可执行动作 JSON"),
    applyLabel: k8sT("ai.apply_actions", "应用建议动作"),
    applyTo: async (text) => {
      if (typeof window.applyOpsActionPlan === "function") {
        return window.applyOpsActionPlan(text, { source: "k8s", refresh: () => renderK8sPanel() });
      }
      return false;
    },
  });
}

function paintK8sEvents() {
  const panel = $("k8sPanel");
  if (!panel) return;
  const q = K8S_FILTER.q;
  const items = (K8S_CACHE.events || []).filter(e => {
    const hay = [e.namespace, e.type, e.reason, e.object, e.message].join(" ");
    return k8sMatch(hay, q);
  });
  const th = [k8sT("k8s.col_ns", "命名空间"), k8sT("k8s.col_type", "类型"), k8sT("k8s.col_reason", "原因"),
    k8sT("k8s.col_object", "对象"), k8sT("k8s.col_message", "消息"), k8sT("k8s.col_count", "次数")]
    .map(h => `<th>${esc(h)}</th>`).join("");
  let body;
  if (!items.length) {
    body = `<div class="empty-state k8s-empty"><h4>${esc(k8sT("k8s.empty", "暂无数据"))}</h4><p>${esc(k8sT("k8s.empty_hint", "尝试切换命名空间，或清空搜索条件。"))}</p></div>`;
  } else {
    const tr = items.map(e => `<tr>
      <td class="mono">${esc(e.namespace || "—")}</td>
      <td>${k8sEventTypeBadge(e.type)}</td>
      <td>${esc(e.reason || "—")}</td>
      <td class="mono">${esc(e.object || "—")}</td>
      <td class="k8s-msg">${esc(e.message || "")}</td>
      <td>${esc(e.count || 0)}</td>
    </tr>`).join("");
    body = `<div class="nf-table-wrap k8s-table-wrap"><table class="data-table k8s-table"><thead><tr>${th}</tr></thead><tbody>${tr}</tbody></table></div>`;
  }
  panel.innerHTML = k8sToolbarHTML({ count: `${items.length}/${K8S_CACHE.events.length}` }) + body;
  k8sWireToolbar(paintK8sEvents);
}

async function renderK8sPanel() {
  const panel = $("k8sPanel");
  if (!panel) return;
  const id = k8sClusterId();
  if (!id) {
    panel.innerHTML = `<div class="empty-state"><h4>${esc(k8sT("k8s.no_cluster", "尚未配置集群"))}</h4>
      <p>${esc(k8sT("k8s.hint_add", "请管理员在「集群配置」中添加 Kubernetes 集群（API Server + Token 或 kubeconfig）。"))}</p></div>`;
    k8sSetStatus(k8sT("k8s.status_none", "未选择集群"), "warn");
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
  panel.innerHTML = `<div class="loading-dots">${esc(k8sT("sec.loading", "加载中…"))}</div>`;
  const nsQ = k8sNamespace() ? `?namespace=${encodeURIComponent(k8sNamespace())}` : "";
  try {
    if (K8S_TAB === "overview") {
      const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/overview`);
      const ver = (j.version && (j.version.gitVersion || j.version.major)) || "—";
      k8sSetStatus(k8sT("k8s.status_ok", "已连接") + " · " + ver, "ok");
      const nodes = j.nodes || {}, pods = j.pods || {}, deps = j.deployments || {};
      panel.innerHTML = `<div class="sec-stat-row k8s-kpi-row">
        <div class="sec-stat"><div class="sec-stat-n">${esc(String(nodes.ready || 0))}<span class="k8s-kpi-den">/${esc(String(nodes.total || 0))}</span></div><div class="sec-stat-l">${esc(k8sT("k8s.kpi_nodes", "节点 Ready"))}</div></div>
        <div class="sec-stat"><div class="sec-stat-n">${esc(String(pods.running || 0))}<span class="k8s-kpi-den">/${esc(String(pods.total || 0))}</span></div><div class="sec-stat-l">${esc(k8sT("k8s.kpi_pods", "Pod Running"))}</div></div>
        <div class="sec-stat"><div class="sec-stat-n">${esc(String(deps.total || 0))}</div><div class="sec-stat-l">${esc(k8sT("k8s.kpi_deployments", "Deployments"))}</div></div>
        <div class="sec-stat"><div class="sec-stat-n mono" style="font-size:15px">${esc(String(ver))}</div><div class="sec-stat-l">${esc(k8sT("k8s.kpi_version", "版本"))}</div></div>
      </div>
      <p class="ws-help" style="margin-top:4px">${esc(k8sT("k8s.overview_hint", "使用上方命名空间筛选，并在 Pods / Deployments / Events 标签中搜索与操作。"))}</p>`;
      return;
    }
    if (K8S_TAB === "nodes") {
      const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/nodes`);
      K8S_CACHE.nodes = j.items || [];
      k8sSetStatus(k8sT("k8s.status_ok", "已连接"), "ok");
      paintK8sNodes();
      return;
    }
    if (K8S_TAB === "pods") {
      const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/pods${nsQ}`);
      K8S_CACHE.pods = j.items || [];
      k8sSetStatus(k8sT("k8s.status_ok", "已连接"), "ok");
      paintK8sPods();
      return;
    }
    if (K8S_TAB === "deployments") {
      const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/deployments${nsQ}`);
      K8S_CACHE.deployments = j.items || [];
      k8sSetStatus(k8sT("k8s.status_ok", "已连接"), "ok");
      paintK8sDeployments();
      return;
    }
    if (K8S_TAB === "events") {
      const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/events${nsQ}`);
      K8S_CACHE.events = j.items || [];
      k8sSetStatus(k8sT("k8s.status_ok", "已连接"), "ok");
      paintK8sEvents();
      return;
    }
    if (K8S_TAB === "apply") {
      k8sSetStatus(k8sT("k8s.status_ok", "已连接"), "ok");
      paintK8sApply();
      return;
    }
  } catch (e) {
    k8sSetStatus(k8sT("k8s.status_err", "连接失败"), "err");
    panel.innerHTML = `<div class="empty-state"><h4>${esc(k8sT("k8s.status_err", "连接失败"))}</h4><p>${esc(String(e.message || e))}</p></div>`;
  }
}

function paintK8sApply() {
  const panel = $("k8sPanel");
  if (!panel) return;
  const allow = (typeof canWrite === "function" && canWrite()) || (typeof isAdmin === "function" && isAdmin());
  panel.innerHTML = `<div class="cfg-panel">
    <div class="cfg-panel-head"><div>
      <div class="cfg-panel-title">${esc(k8sT("k8s.apply_title", "YAML Apply / 创建命名空间"))}</div>
      <p class="cfg-panel-desc">${esc(k8sT("k8s.apply_hint", "粘贴多文档 YAML，经 kubectl apply 下发（需服务端安装 kubectl）。高危变更请先 Dry-run。集群「创建」指纳管已有 API；可在此创建 Namespace。"))}</p>
    </div></div>
    <div class="rtx-toolbar" style="margin-bottom:8px;gap:8px;flex-wrap:wrap">
      <input type="text" id="k8sApplyNs" class="hw-search" placeholder="${esc(k8sT("k8s.apply_ns_ph", "默认命名空间（可选）"))}" style="max-width:200px" value="${esc(k8sNamespace() || "")}">
      <input type="text" id="k8sNewNs" class="hw-search" placeholder="${esc(k8sT("k8s.new_ns_ph", "新建 Namespace 名"))}" style="max-width:180px">
      ${allow ? `<button type="button" class="btn sm" id="k8sCreateNsBtn">${esc(k8sT("k8s.create_ns", "创建 Namespace"))}</button>` : ""}
    </div>
    <textarea id="k8sApplyYAML" rows="16" class="mono" style="width:100%;padding:10px;border:1px solid var(--line);border-radius:8px;background:var(--panel2);font-size:12px" placeholder="apiVersion: v1&#10;kind: ConfigMap&#10;metadata:&#10;  name: demo"></textarea>
    <div style="margin-top:10px;display:flex;gap:8px;flex-wrap:wrap">
      ${allow ? `<button type="button" class="btn sm" id="k8sDryRunBtn">${esc(k8sT("k8s.dry_run", "Dry-run"))}</button>
      <button type="button" class="btn sm primary" id="k8sApplyBtn">${esc(k8sT("k8s.apply", "Apply"))}</button>` : `<span class="muted">${esc(k8sT("k8s.apply_readonly", "只读用户不可 Apply"))}</span>`}
    </div>
    <pre id="k8sApplyOut" class="mono" style="margin-top:10px;min-height:60px;max-height:260px;overflow:auto;padding:10px;border:1px solid var(--line);border-radius:8px;background:var(--panel2);font-size:12px;white-space:pre-wrap"></pre>
  </div>`;
  const run = async (dry) => {
    const id = k8sClusterId();
    const yaml = ($("k8sApplyYAML") || {}).value || "";
    const ns = (($("k8sApplyNs") || {}).value || "").trim();
    const out = $("k8sApplyOut");
    if (!yaml.trim()) { toast(k8sT("k8s.apply_empty", "请粘贴 YAML"), "err"); return; }
    if (!dry && !confirm(k8sT("k8s.apply_confirm", "确认 Apply 到集群？"))) return;
    if (out) out.textContent = k8sT("sec.loading", "加载中…");
    try {
      const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/apply`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ yaml, namespace: ns, dry_run: !!dry }),
      });
      if (out) out.textContent = j.output || "ok";
      toast(dry ? "Dry-run OK" : k8sT("k8s.apply_ok", "Apply 完成"), "ok");
    } catch (e) {
      if (out) out.textContent = String(e.message || e);
      toast(String(e.message || e), "err");
    }
  };
  const dryBtn = $("k8sDryRunBtn");
  if (dryBtn) dryBtn.onclick = () => run(true);
  const applyBtn = $("k8sApplyBtn");
  if (applyBtn) applyBtn.onclick = () => run(false);
  const nsBtn = $("k8sCreateNsBtn");
  if (nsBtn) nsBtn.onclick = async () => {
    const id = k8sClusterId();
    const name = (($("k8sNewNs") || {}).value || "").trim();
    if (!name) { toast(k8sT("k8s.new_ns_ph", "新建 Namespace 名"), "err"); return; }
    if (!confirm(k8sT("k8s.create_ns_confirm", "确认创建 Namespace？") + " " + name)) return;
    const out = $("k8sApplyOut");
    try {
      const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/namespaces`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      });
      if (out) out.textContent = j.output || "ok";
      toast(k8sT("k8s.create_ns_ok", "Namespace 已创建"), "ok");
      loadK8sNamespaces();
    } catch (e) {
      if (out) out.textContent = String(e.message || e);
      toast(String(e.message || e), "err");
    }
  };
}

async function openK8sPodLog(ns, name) {
  const id = k8sClusterId();
  if (!id || !ns || !name) return;
  const title = $("k8sLogTitle");
  const body = $("k8sLogBody");
  if (title) title.textContent = `Pod ${ns}/${name}`;
  if (body) body.textContent = k8sT("sec.loading", "加载中…");
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
  if (hint) hint.textContent = `${ns}/${name} · ${k8sT("k8s.current_replicas", "当前副本")} ${current}`;
  if ($("k8sScaleReplicas")) $("k8sScaleReplicas").value = String(current);
  $("k8sScaleMask")?.classList.add("show");
}

function renderK8sConfigForm() {
  if (typeof isAdmin === "function" && !isAdmin()) {
    return `<div class="hint">${esc(k8sT("toast.admin_only", "仅管理员可操作"))}</div>`;
  }
  const list = K8S_CLUSTERS.map(c => `<tr>
    <td>${esc(c.name)}</td>
    <td class="mono">${esc(c.api_server || (c.kubeconfig_yaml ? "kubeconfig" : "—"))}</td>
    <td>${c.enabled ? `<span class="badge ok">${esc(k8sT("k8s.col_enabled", "启用"))}</span>` : `<span class="badge">${esc(k8sT("k8s.off", "停用"))}</span>`}</td>
    <td>
      <div class="k8s-actions">
        <button type="button" class="btn sm" data-k8s-edit="${esc(c.id)}">${esc(k8sT("ui.edit", "编辑"))}</button>
        <button type="button" class="btn sm" data-k8s-test="${esc(c.id)}">${esc(k8sT("k8s.test", "连通测试"))}</button>
        <button type="button" class="btn sm danger" data-k8s-del="${esc(c.id)}">${esc(k8sT("ui.delete", "删除"))}</button>
      </div>
    </td>
  </tr>`).join("");
  return `<div class="cfg-panel k8s-cfg">
    <div class="cfg-panel-title">${esc(k8sT("k8s.tab_config", "集群配置"))}</div>
    <p class="field-hint">${esc(k8sT("k8s.config_hint", "推荐使用只读 ServiceAccount Token；Scale/Restart 需额外 patch 权限。密钥留空表示保持原值。"))}</p>
    <div class="nf-table-wrap k8s-table-wrap" style="margin-bottom:14px"><table class="data-table k8s-table">
      <thead><tr><th>${esc(k8sT("k8s.col_name", "名称"))}</th><th>${esc(k8sT("k8s.col_endpoint", "Endpoint"))}</th><th>${esc(k8sT("k8s.col_enabled", "启用"))}</th><th></th></tr></thead>
      <tbody>${list || `<tr><td colspan="4">${esc(k8sT("k8s.empty", "暂无数据"))}</td></tr>`}</tbody>
    </table></div>
    <div class="cfg-form" id="k8sCfgForm">
      <input type="hidden" id="k8sCfgId" value="">
      <div class="cfg-form-row">
        <div class="field"><label>${esc(k8sT("k8s.col_name", "名称"))}</label><input id="k8sCfgName" type="text" autocomplete="off"></div>
        <div class="field cfg-field-switch"><label class="switch"><input type="checkbox" id="k8sCfgEnabled" checked><span>${esc(k8sT("k8s.col_enabled", "启用"))}</span></label></div>
      </div>
      <div class="field"><label>API Server</label><input id="k8sCfgAPI" class="mono" type="url" placeholder="https://kubernetes.default.svc:443" autocomplete="off"></div>
      <div class="field"><label>Token</label><input id="k8sCfgToken" class="mono" type="password" placeholder="${esc(k8sT("sec.oidc_secret_ph", "留空保持原值"))}" autocomplete="new-password"></div>
      <div class="field"><label>CA Cert (PEM)</label><textarea id="k8sCfgCA" class="mono" rows="3" spellcheck="false"></textarea></div>
      <label class="switch mb"><input type="checkbox" id="k8sCfgInsecure"><span>${esc(k8sT("k8s.insecure", "跳过 TLS 校验（仅内网临时）"))}</span></label>
      <div class="field"><label>Kubeconfig YAML（可选，优先于上方字段）</label><textarea id="k8sCfgKube" class="mono" rows="5" spellcheck="false" placeholder="粘贴 kubeconfig；已配置时显示 ****"></textarea></div>
      <div class="field"><label>${esc(k8sT("k8s.default_ns", "默认命名空间（空=全部）"))}</label><input id="k8sCfgNS" class="mono" type="text" placeholder="default"></div>
      <div class="cfg-actions">
        <button type="button" class="btn" id="k8sCfgReset">${esc(k8sT("ui.reset", "重置"))}</button>
        <button type="button" class="btn primary" id="k8sCfgSave">${esc(k8sT("settings.save", "保存"))}</button>
      </div>
    </div>
  </div>`;
}

function wireK8sConfigForm() {
  const panel = $("k8sPanel");
  if (!panel) return;
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
  // Property handlers replace prior bindings when the form is re-rendered.
  const resetBtn = $("k8sCfgReset");
  if (resetBtn) resetBtn.onclick = () => fill(null);
  const saveBtn = $("k8sCfgSave");
  if (saveBtn) saveBtn.onclick = async () => {
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
      toast(k8sT("toast.saved", "已保存"), "ok");
      await loadK8sPage();
    } catch (e) { toast(String(e.message || e), "err"); }
  };
  panel.querySelectorAll("[data-k8s-edit]").forEach(b => {
    b.onclick = () => {
      const c = K8S_CLUSTERS.find(x => x.id === b.getAttribute("data-k8s-edit"));
      fill(c);
    };
  });
  panel.querySelectorAll("[data-k8s-test]").forEach(b => {
    b.onclick = async () => {
      try {
        const j = await k8sFetch(`/k8s/clusters/${encodeURIComponent(b.getAttribute("data-k8s-test"))}/test`, { method: "POST", body: "{}" });
        const ver = j.version?.gitVersion || "ok";
        toast(k8sT("k8s.test_ok", "连通成功") + " · " + ver, "ok");
      } catch (e) { toast(String(e.message || e), "err"); }
    };
  });
  panel.querySelectorAll("[data-k8s-del]").forEach(b => {
    b.onclick = async () => {
      if (!confirm(k8sT("k8s.del_confirm", "确定删除该集群配置？"))) return;
      try {
        await k8sFetch(`/k8s/clusters/${encodeURIComponent(b.getAttribute("data-k8s-del"))}`, { method: "DELETE" });
        toast(k8sT("toast.deleted", "已删除"), "ok");
        await loadK8sPage();
      } catch (e) { toast(String(e.message || e), "err"); }
    };
  });
}

async function loadK8sPage() {
  try {
    await loadK8sClusters();
    await loadK8sNamespaces();
    await renderK8sPanel();
  } catch (e) {
    k8sSetStatus(k8sT("k8s.status_err", "连接失败"), "err");
    const panel = $("k8sPanel");
    if (panel) panel.innerHTML = `<div class="empty-state"><h4>${esc(k8sT("k8s.status_err", "连接失败"))}</h4><p>${esc(String(e.message || e))}</p></div>`;
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
    if (!confirm(k8sT("k8s.scale_confirm_q", "确认将 {name} 调整为 {n} 副本？")
      .replace("{name}", K8S_SCALE_CTX.name).replace("{n}", String(replicas)))) return;
    try {
      await k8sFetch(`/k8s/clusters/${encodeURIComponent(id)}/deployments/${encodeURIComponent(K8S_SCALE_CTX.ns)}/${encodeURIComponent(K8S_SCALE_CTX.name)}/scale`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ replicas }),
      });
      toast(k8sT("k8s.scale_ok", "Scale 已提交"), "ok");
      $("k8sScaleMask")?.classList.remove("show");
      renderK8sPanel();
    } catch (e) { toast(String(e.message || e), "err"); }
  });
});
