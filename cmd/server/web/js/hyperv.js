/* ============================================================================
 * Hyper-V 虚拟机页：按物理宿主机分组展示其下的 Guest 清单与资源，异常优先，
 * 并把每台 VM 关联回已纳管主机（可一键跳到该主机监控详情）。
 *
 * 复用 core.js 的 esc/fmtUptime/ago/toast、style.css 的 .badge.{ok,warn,crit}。
 * 事件全部委托（CSP script-src 'self'，禁内联 onclick）。
 * ========================================================================== */

const hvT = (k, fb) => I18N.t(k, fb);

let HV_INVENTORIES = [];        // [{host_id, host_name, guest_count, guests:[...], updated_at}]
const HV_FILTER = { q: "", status: "" };
const HV_TOGGLED = new Set();   // host_ids where the user flipped the default expand state

// hvExpanded decides whether a host section renders its VM table. Default: expand
// hosts with abnormal VMs or when the fleet is small; collapse healthy hosts in a
// large fleet (keeps the DOM light for hundreds of VMs). A user click flips that
// default; an active filter forces every section open so matches stay visible.
function hvExpanded(inv) {
  if (HV_FILTER.q || HV_FILTER.status) return true;
  const def = (inv.guests || []).some(hvAbnormal) || HV_INVENTORIES.length <= 6;
  return HV_TOGGLED.has(inv.host_id) ? !def : def;
}

async function loadHyperVPanel() {
  const container = $("hypervPanel");
  if (!container) return;
  container.innerHTML = `<div class="empty-line">${esc(hvT("hyperv.loading", "加载中…"))}</div>`;
  try {
    const d = await fetch(`${API}/hyperv/list`).then(r => r.json());
    HV_INVENTORIES = (d && d.inventories) ? d.inventories : [];
  } catch (e) {
    HV_INVENTORIES = [];
  }
  renderHyperVPanel();
}

/* ---------- 判定 ---------- */

// 与服务端 hypervGuestAbnormal 对齐：需要关注的 VM。
function hvAbnormal(g) {
  if (g.health === "Critical") return true;
  if (g.state !== "Running") return true;
  if (g.repl_health === "Warning" || g.repl_health === "Critical") return true;
  if ((g.cpu_usage || 0) >= 85) return true;
  const asg = g.mem_assigned_mb || 0, dem = g.mem_demand_mb || 0;
  if (asg > 0 && dem > 0 && dem / asg * 100 >= 90) return true;
  return false;
}

function hvStateBadge(g) {
  if (g.health === "Critical") return `<span class="badge crit">${esc(hvT("hyperv.st_critical", "严重"))}</span>`;
  switch (g.state) {
    case "Running": return `<span class="badge ok">${esc(hvT("hyperv.st_running", "运行中"))}</span>`;
    case "Off":     return `<span class="badge warn">${esc(hvT("hyperv.st_off", "已关机"))}</span>`;
    case "Paused":  return `<span class="badge warn">${esc(hvT("hyperv.st_paused", "已暂停"))}</span>`;
    case "Saved":   return `<span class="badge warn">${esc(hvT("hyperv.st_saved", "已保存"))}</span>`;
    default:        return `<span class="badge">${esc(g.state || hvT("hyperv.st_unknown", "未知"))}</span>`;
  }
}

/* ---------- 过滤 ---------- */

function hvMatches(g, q) {
  if (!q) return true;
  const hay = [g.name, g.state, (g.ip_addresses || []).join(" "), g.linked_host_name]
    .filter(Boolean).join(" ").toLowerCase();
  return q.toLowerCase().split(/\s+/).filter(Boolean).every(tok => hay.includes(tok));
}

function hvGuestVisible(g) {
  if (!hvMatches(g, HV_FILTER.q)) return false;
  switch (HV_FILTER.status) {
    case "running":    return g.state === "Running";
    case "notrunning": return g.state !== "Running";
    case "abnormal":   return hvAbnormal(g);
    default:           return true;
  }
}

/* ---------- 渲染 ---------- */

function hvGuestRow(g) {
  const cpu = g.state === "Running" ? Math.round(g.cpu_usage || 0) : null;
  const cpuCell = cpu === null ? "—"
    : `<span style="color:${usageColor(cpu)}">${cpu}%</span>`;
  let memCell = "—";
  if (g.state === "Running" && (g.mem_assigned_mb || 0) > 0) {
    const asg = Math.round(g.mem_assigned_mb), dem = Math.round(g.mem_demand_mb || 0);
    const pct = asg > 0 ? dem / asg * 100 : 0;
    memCell = `<span style="color:${usageColor(pct)}">${dem} / ${asg} MB</span>`;
  }
  const ip = (g.ip_addresses && g.ip_addresses.length) ? esc(g.ip_addresses.join(", ")) : "—";
  const uptime = (g.state === "Running" && g.uptime_sec) ? fmtUptime(g.uptime_sec) : "—";
  let repl = "—";
  if (g.repl_state && g.repl_state !== "Disabled") {
    const bad = g.repl_health === "Warning" || g.repl_health === "Critical";
    repl = `<span class="${bad ? "hv-bad" : ""}">${esc(g.repl_health || g.repl_state)}</span>`;
  }
  const linked = g.linked_host_id
    ? `<a class="hv-link" data-hvjump="${esc(g.linked_host_id)}" data-hvname="${esc(g.linked_host_name || g.name)}" title="${esc(hvT("hyperv.jump_title", "跳转到该主机监控详情"))}">${esc(g.linked_host_name || g.name)} ↗</a>`
    : `<span class="muted">—</span>`;
  return `<tr class="${hvAbnormal(g) ? "hv-row-bad" : ""}">
    <td>${esc(g.name)}</td>
    <td>${hvStateBadge(g)}</td>
    <td class="mono">${cpuCell}</td>
    <td class="mono">${memCell}</td>
    <td class="mono">${ip}</td>
    <td class="mono">${uptime}</td>
    <td>${repl}</td>
    <td>${linked}</td>
  </tr>`;
}

function hvHostSection(inv) {
  const guests = (inv.guests || []).slice().sort((a, b) => {
    const aa = hvAbnormal(a), ab = hvAbnormal(b);
    if (aa !== ab) return aa ? -1 : 1;
    return String(a.name).localeCompare(String(b.name));
  });
  const shown = guests.filter(hvGuestVisible);
  const running = guests.filter(g => g.state === "Running").length;
  const bad = guests.filter(hvAbnormal).length;
  const hostName = inv.host_name || inv.host_id;
  const updated = inv.updated_at ? ago(Math.floor(new Date(inv.updated_at).getTime() / 1000)) : "";
  const open = hvExpanded(inv);

  const head = `<div class="hv-host-head">
    <button class="hv-caret" data-hvtoggle="${esc(inv.host_id)}" title="${esc(hvT("hyperv.toggle", "展开/收起"))}" style="background:none;border:none;color:var(--muted);cursor:pointer;font-size:12px;padding:0 6px">${open ? "▾" : "▸"}</button>
    <a class="hv-link hv-host-name" data-hvjump="${esc(inv.host_id)}" data-hvname="${esc(hostName)}" title="${esc(hvT("hyperv.jump_title", "跳转到该主机监控详情"))}">🖥 ${esc(hostName)} ↗</a>
    <span class="hv-host-stat">${hvT("hyperv.total", "共")} ${guests.length} · ${hvT("hyperv.st_running", "运行中")} ${running}${bad ? ` · <span class="hv-bad">${hvT("hyperv.attention", "需关注")} ${bad}</span>` : ""}</span>
    ${updated ? `<span class="hv-host-upd muted">${esc(updated)}</span>` : ""}
  </div>`;

  if (!open) {
    return `<div class="hv-host">${head}</div>`; // collapsed: header only, table not built (DOM stays light)
  }
  if (!shown.length) {
    return `<div class="hv-host">${head}<div class="empty-line">${esc(hvT("hyperv.no_match", "无匹配虚拟机"))}</div></div>`;
  }
  const table = `<table class="hw-table hv-table">
    <thead><tr>
      <th>${hvT("hyperv.col_name", "虚拟机")}</th>
      <th>${hvT("hyperv.col_state", "状态")}</th>
      <th>CPU</th>
      <th>${hvT("hyperv.col_mem", "内存(需求/分配)")}</th>
      <th>${hvT("hyperv.col_ip", "IP 地址")}</th>
      <th>${hvT("hyperv.col_uptime", "运行时长")}</th>
      <th>${hvT("hyperv.col_repl", "复制")}</th>
      <th>${hvT("hyperv.col_linked", "关联主机")}</th>
    </tr></thead>
    <tbody>${shown.map(hvGuestRow).join("")}</tbody>
  </table>`;
  return `<div class="hv-host">${head}${table}</div>`;
}

function hvToolbar(totalHosts, totalVMs, totalBad) {
  return `<div class="hw-toolbar">
    <input id="hvSearch" class="input sm" type="text" placeholder="${esc(hvT("hyperv.search", "搜索 VM 名称 / IP / 状态…"))}" value="${esc(HV_FILTER.q)}" style="max-width:260px">
    <select id="hvStatusFilter" class="input sm" style="max-width:150px">
      <option value="">${esc(hvT("hyperv.f_all", "全部状态"))}</option>
      <option value="running"${HV_FILTER.status === "running" ? " selected" : ""}>${esc(hvT("hyperv.st_running", "运行中"))}</option>
      <option value="notrunning"${HV_FILTER.status === "notrunning" ? " selected" : ""}>${esc(hvT("hyperv.f_notrunning", "非运行"))}</option>
      <option value="abnormal"${HV_FILTER.status === "abnormal" ? " selected" : ""}>${esc(hvT("hyperv.attention", "需关注"))}</option>
    </select>
    <button data-hvrefresh="1" class="btn sm" title="${esc(hvT("hyperv.refresh", "刷新"))}">↻</button>
    <span class="hw-count muted">${hvT("hyperv.summary", "宿主机")} ${totalHosts} · ${hvT("hyperv.vm", "虚拟机")} ${totalVMs}${totalBad ? ` · <span class="hv-bad">${hvT("hyperv.attention", "需关注")} ${totalBad}</span>` : ""}</span>
  </div>`;
}

function renderHyperVPanel() {
  const container = $("hypervPanel");
  if (!container) return;
  if (!HV_INVENTORIES.length) {
    container.innerHTML = `<div class="empty-line">${esc(hvT("hyperv.empty", "暂无 Hyper-V 宿主机数据。请确认物理机已安装并更新 Agent（含 Hyper-V 采集），且该主机已启用 Hyper-V 角色。"))}</div>`;
    return;
  }
  let totalVMs = 0, totalBad = 0;
  HV_INVENTORIES.forEach(inv => {
    totalVMs += (inv.guests || []).length;
    totalBad += (inv.guests || []).filter(hvAbnormal).length;
  });
  const body = HV_INVENTORIES
    .slice()
    .sort((a, b) => (b.guests || []).filter(hvAbnormal).length - (a.guests || []).filter(hvAbnormal).length)
    .map(hvHostSection).join("");
  container.innerHTML = hvToolbar(HV_INVENTORIES.length, totalVMs, totalBad) + body;
}

/* ---------- 事件（全部委托） ---------- */

safeAddEventListener("hypervPanel", "click", e => {
  const refresh = e.target.closest("[data-hvrefresh]");
  if (refresh) { loadHyperVPanel(); return; }
  const toggle = e.target.closest("[data-hvtoggle]");
  if (toggle) {
    const id = toggle.dataset.hvtoggle;
    if (HV_TOGGLED.has(id)) HV_TOGGLED.delete(id); else HV_TOGGLED.add(id);
    renderHyperVPanel();
    return;
  }
  const j = e.target.closest("[data-hvjump]");
  if (j && typeof openDetail === "function") {
    openDetail(j.dataset.hvjump, j.dataset.hvname || j.dataset.hvjump);
  }
});

let HV_SEARCH_T = null;
safeAddEventListener("hypervPanel", "input", e => {
  if (e.target.id !== "hvSearch") return;
  clearTimeout(HV_SEARCH_T);
  const v = e.target.value;
  HV_SEARCH_T = setTimeout(() => {
    HV_FILTER.q = v;
    renderHyperVPanel();
    const s = $("hvSearch");
    if (s) { s.focus(); s.setSelectionRange(s.value.length, s.value.length); }
  }, 200);
});
safeAddEventListener("hypervPanel", "change", e => {
  if (e.target.id === "hvStatusFilter") { HV_FILTER.status = e.target.value; renderHyperVPanel(); }
});

// 供 nav.js 的 _pageRenderers 调用
if (typeof window._pageRenderers === "undefined") window._pageRenderers = {};
window._pageRenderers.hyperv = loadHyperVPanel;
