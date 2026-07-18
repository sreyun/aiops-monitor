/* ============================================================================
 * Hyper-V 虚拟机页 — 主从式目录树：
 *   左侧树  物理宿主机 ▸ 其下虚拟机（可折叠、可选中、异常优先、状态圆点）
 *   右侧详情 选中 VM 的完整信息（概览 / CPU / 内存 / 硬盘 / 网络 / 检查点）
 * VM 关联回已纳管主机可一键跳转。样式经 <style> 注入（CSP 允许 style 'unsafe-inline'），
 * 不依赖 style.css；事件全部委托（CSP script-src 'self'，禁内联 onclick）。
 * 对老 Agent 缺失的明细字段优雅降级（显示"需更新 Agent"）。
 * ========================================================================== */

const hvT = (k, fb) => I18N.t(k, fb);

let HV_INVENTORIES = [];               // [{host_id, host_name, guest_count, guests:[...], updated_at}]
const HV_FILTER = { q: "", status: "" };
const HV_COLLAPSED = new Set();        // host_ids collapsed in the tree (default expanded)
let HV_SELECTED = null;                // { host, vm } currently shown in the detail panel

/* ---------- 小工具 ---------- */

function hvVmKey(g) { return g.id || ("name:" + (g.name || "")); }

function hvFmtGB(gb) {
  gb = +gb || 0;
  if (gb >= 1024) return (gb / 1024).toFixed(1) + " TB";
  if (gb > 0 && gb < 1) return Math.round(gb * 1024) + " MB";
  return gb.toFixed(1) + " GB";
}

function hvMac(m) {
  if (!m) return "—";
  const h = String(m).replace(/[^0-9A-Fa-f]/g, "");
  return h.length === 12 ? h.match(/.{2}/g).join(":").toUpperCase() : String(m);
}

function hvAbnormal(g) {
  if (g.health === "Critical") return true;
  if (g.state !== "Running") return true;
  if (g.repl_health === "Warning" || g.repl_health === "Critical") return true;
  if ((g.cpu_usage || 0) >= 85) return true;
  const asg = g.mem_assigned_mb || 0, dem = g.mem_demand_mb || 0;
  if (g.dynamic_mem_enabled && asg > 0 && dem > 0 && dem / asg * 100 >= 90) return true;
  return false;
}

function hvDotClass(g) {
  if (g.health === "Critical") return "crit";
  if (g.state !== "Running") return "off";
  if (hvAbnormal(g)) return "warn";
  return "ok";
}

function hvStateText(g) {
  switch (g.state) {
    case "Running": return hvT("hyperv.st_running", "运行中");
    case "Off": return hvT("hyperv.st_off", "已关机");
    case "Paused": return hvT("hyperv.st_paused", "已暂停");
    case "Saved": return hvT("hyperv.st_saved", "已保存");
    default: return g.state || hvT("hyperv.st_unknown", "未知");
  }
}

function hvStateBadge(g) {
  if (g.health === "Critical") return `<span class="badge crit">${esc(hvT("hyperv.st_critical", "严重"))}</span>`;
  switch (g.state) {
    case "Running": return `<span class="badge ok">${esc(hvT("hyperv.st_running", "运行中"))}</span>`;
    case "Off": return `<span class="badge warn">${esc(hvT("hyperv.st_off", "已关机"))}</span>`;
    case "Paused": return `<span class="badge warn">${esc(hvT("hyperv.st_paused", "已暂停"))}</span>`;
    case "Saved": return `<span class="badge warn">${esc(hvT("hyperv.st_saved", "已保存"))}</span>`;
    default: return `<span class="badge">${esc(g.state || hvT("hyperv.st_unknown", "未知"))}</span>`;
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
    case "running": return g.state === "Running";
    case "notrunning": return g.state !== "Running";
    case "abnormal": return hvAbnormal(g);
    default: return true;
  }
}

/* ---------- 数据加载 ---------- */

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

/* ---------- 选中态 ---------- */

function hvFindSelected() {
  if (!HV_SELECTED) return null;
  const inv = HV_INVENTORIES.find(i => i.host_id === HV_SELECTED.host);
  if (!inv) return null;
  const g = (inv.guests || []).find(x => hvVmKey(x) === HV_SELECTED.vm);
  return g ? { inv, g } : null;
}

// 默认选中第一台异常 VM（否则第一台 VM），让详情不空。
function hvFirstSelectable() {
  let firstAny = null;
  for (const inv of HV_INVENTORIES) {
    for (const g of (inv.guests || [])) {
      if (!firstAny) firstAny = { host: inv.host_id, vm: hvVmKey(g) };
      if (hvAbnormal(g)) return { host: inv.host_id, vm: hvVmKey(g) };
    }
  }
  return firstAny;
}

function hvSelect(host, vm) {
  HV_SELECTED = { host, vm };
  const tree = $("hvTree");
  if (tree) tree.querySelectorAll(".hv-vm").forEach(el =>
    el.classList.toggle("selected", el.dataset.hvhost === host && el.dataset.hvvm === vm));
  renderHyperVDetailOnly();
}

/* ---------- 左侧树 ---------- */

function hvVmNode(inv, g) {
  const sel = HV_SELECTED && HV_SELECTED.host === inv.host_id && HV_SELECTED.vm === hvVmKey(g);
  const running = g.state === "Running";
  const mini = running
    ? `${Math.round(g.cpu_usage || 0)}% · ${Math.round(g.mem_assigned_mb || 0)}MB`
    : esc(hvStateText(g));
  return `<div class="hv-vm ${sel ? "selected" : ""} ${hvAbnormal(g) ? "bad" : ""}" data-hvhost="${esc(inv.host_id)}" data-hvvm="${esc(hvVmKey(g))}">
    <span class="hv-dot ${hvDotClass(g)}"></span>
    <span class="vname" title="${esc(g.name)}">${esc(g.name)}</span>
    <span class="vmini">${mini}</span>
  </div>`;
}

function hvHostNode(inv) {
  const guests = (inv.guests || []).slice().sort((a, b) => {
    const aa = hvAbnormal(a), ab = hvAbnormal(b);
    if (aa !== ab) return aa ? -1 : 1;
    return String(a.name).localeCompare(String(b.name));
  });
  const visible = guests.filter(hvGuestVisible);
  const running = guests.filter(g => g.state === "Running").length;
  const bad = guests.filter(hvAbnormal).length;
  const hostName = inv.host_name || inv.host_id;
  const filtering = HV_FILTER.q || HV_FILTER.status;
  const collapsed = HV_COLLAPSED.has(inv.host_id) && !filtering;

  const countTitle = `${guests.length} ${esc(hvT("hyperv.vm", "虚拟机"))} · ${running} ${esc(hvT("hyperv.st_running", "运行中"))}${bad ? ` · ${bad} ${esc(hvT("hyperv.attention", "需关注"))}` : ""}`;
  const head = `<div class="hv-hosthead" data-hvhosttoggle="${esc(inv.host_id)}">
    <span class="hv-caret2">${collapsed ? "▸" : "▾"}</span>
    <span class="name" title="${esc(hostName)}">🖥 ${esc(hostName)}</span>
    <span class="hv-hostcount${bad ? " bad" : ""}" title="${countTitle}">${guests.length}${bad ? `<span class="hc-bad">${bad}</span>` : ""}</span>
  </div>`;

  if (collapsed) return `<div class="hv-hostnode">${head}</div>`;
  const list = visible.length
    ? visible.map(g => hvVmNode(inv, g)).join("")
    : `<div class="hv-vm" style="cursor:default;color:var(--muted)"><span class="vname">${esc(hvT("hyperv.no_match", "无匹配虚拟机"))}</span></div>`;
  return `<div class="hv-hostnode">${head}<div class="hv-vmlist">${list}</div></div>`;
}

function hvToolbar(totalHosts, totalVMs, totalBad) {
  return `<div class="hw-toolbar" style="margin-bottom:10px">
    <input id="hvSearch" class="input sm" type="text" placeholder="${esc(hvT("hyperv.search", "搜索 VM 名称 / IP / 状态…"))}" value="${esc(HV_FILTER.q)}" style="flex:1;min-width:120px">
    <select id="hvStatusFilter" class="input sm" style="max-width:130px">
      <option value="">${esc(hvT("hyperv.f_all", "全部状态"))}</option>
      <option value="running"${HV_FILTER.status === "running" ? " selected" : ""}>${esc(hvT("hyperv.st_running", "运行中"))}</option>
      <option value="notrunning"${HV_FILTER.status === "notrunning" ? " selected" : ""}>${esc(hvT("hyperv.f_notrunning", "非运行"))}</option>
      <option value="abnormal"${HV_FILTER.status === "abnormal" ? " selected" : ""}>${esc(hvT("hyperv.attention", "需关注"))}</option>
    </select>
    <button data-hvrefresh="1" class="btn sm" title="${esc(hvT("hyperv.refresh", "刷新"))}">↻</button>
  </div>
  <div class="hv-summary muted">${hvT("hyperv.summary", "宿主机")} ${totalHosts} · ${hvT("hyperv.vm", "虚拟机")} ${totalVMs}${totalBad ? ` · <span style="color:var(--warn-txt)">${hvT("hyperv.attention", "需关注")} ${totalBad}</span>` : ""}</div>`;
}

/* ---------- 右侧详情 ---------- */

function hvKv(k, v) { return `<div class="hv-kv"><span class="k">${k}</span><span class="v">${v}</span></div>`; }

function hvMiniTable(headers, rows) {
  if (!rows.length) return "";
  return `<table class="hv-mini-table"><thead><tr>${headers.map(h => `<th>${h}</th>`).join("")}</tr></thead><tbody>${rows.map(r => `<tr>${r.map(c => `<td>${c}</td>`).join("")}</tr>`).join("")}</tbody></table>`;
}

function hvOverviewCard(inv, g) {
  const kv = [];
  kv.push(hvKv(hvT("hyperv.ov_state", "状态"), hvStateBadge(g)));
  if (g.health && g.health !== "OK") kv.push(hvKv(hvT("hyperv.ov_health", "健康"), esc(g.health)));
  kv.push(hvKv(hvT("hyperv.ov_uptime", "运行时长"), (g.state === "Running" && g.uptime_sec) ? esc(fmtUptime(g.uptime_sec)) : "—"));
  kv.push(hvKv(hvT("hyperv.ov_host", "宿主机"), `<a class="hv-link" data-hvjump="${esc(inv.host_id)}" data-hvname="${esc(inv.host_name || inv.host_id)}">${esc(inv.host_name || inv.host_id)} ↗</a>`));
  if (g.linked_host_id) kv.push(hvKv(hvT("hyperv.ov_linked", "关联纳管主机"), `<a class="hv-link" data-hvjump="${esc(g.linked_host_id)}" data-hvname="${esc(g.linked_host_name || g.name)}">${esc(g.linked_host_name || g.name)} ↗</a>`));
  kv.push(hvKv(hvT("hyperv.ov_gen", "代/版本"), `${g.generation ? ("Gen" + g.generation) : "—"}${g.version ? (" · " + esc(g.version)) : ""}`));
  if (g.integration_state) kv.push(hvKv(hvT("hyperv.ov_integration", "集成服务"), esc(g.integration_state)));
  if (g.repl_state && g.repl_state !== "Disabled") kv.push(hvKv(hvT("hyperv.ov_repl", "复制"), esc(g.repl_state + " / " + (g.repl_health || ""))));
  return `<div class="hv-card"><h4>${hvT("hyperv.overview", "概览")}</h4>${kv.join("")}</div>`;
}

function hvCpuCard(g) {
  const running = g.state === "Running";
  const cpu = running ? Math.round(g.cpu_usage || 0) : 0;
  return `<div class="hv-card"><h4>CPU</h4>
    ${bar(hvT("hyperv.cpu_host", "占宿主 CPU"), cpu, running ? cpu + "%" : "—")}
    ${hvKv(hvT("hyperv.vcpu", "vCPU 数"), g.processor_count || "—")}
  </div>`;
}

function hvMemCard(g) {
  const running = g.state === "Running";
  const asg = g.mem_assigned_mb || 0, dem = g.mem_demand_mb || 0;
  const pct = asg > 0 ? Math.round(dem / asg * 100) : 0;
  const barHtml = (running && g.dynamic_mem_enabled && asg > 0)
    ? bar(hvT("hyperv.mem_pressure", "内存压力(需求/分配)"), pct, `${Math.round(dem)} / ${Math.round(asg)} MB`)
    : "";
  const rangeKv = g.dynamic_mem_enabled
    ? hvKv(hvT("hyperv.mem_range", "动态范围"), `${Math.round(g.mem_min_mb || 0)} ~ ${Math.round(g.mem_max_mb || 0)} MB`)
    : hvKv(hvT("hyperv.mem_type", "内存类型"), hvT("hyperv.mem_static", "静态"));
  return `<div class="hv-card"><h4>${hvT("hyperv.memory", "内存")}</h4>
    ${barHtml}
    ${hvKv(hvT("hyperv.mem_assigned", "已分配"), running ? Math.round(asg) + " MB" : "—")}
    ${hvKv(hvT("hyperv.mem_demand", "需求"), running ? Math.round(dem) + " MB" : "—")}
    ${g.mem_startup_mb ? hvKv(hvT("hyperv.mem_startup", "启动内存"), Math.round(g.mem_startup_mb) + " MB") : ""}
    ${rangeKv}
  </div>`;
}

function hvDiskCard(g) {
  const disks = g.disks || [];
  let body;
  if (!disks.length) body = `<div class="hv-hint">${esc(hvT("hyperv.no_disk", "无磁盘明细（需更新 Agent 采集）"))}</div>`;
  else body = hvMiniTable(
    [hvT("hyperv.disk_path", "路径"), hvT("hyperv.disk_ctrl", "控制器"), hvT("hyperv.disk_size", "占用")],
    disks.map(d => [
      esc(d.path || "—"),
      esc((d.controller_type || "") + (d.controller_number != null ? ` ${d.controller_number}:${d.controller_location || 0}` : "")),
      d.file_size_gb ? esc(hvFmtGB(d.file_size_gb)) : "—",
    ]));
  return `<div class="hv-card hv-full"><h4>${hvT("hyperv.disk", "硬盘")} · ${disks.length || (g.vhd_count || 0)}</h4>${body}</div>`;
}

function hvNetCard(g) {
  const nics = g.nics || [];
  let body;
  if (!nics.length) {
    const ips = g.ip_addresses || [];
    body = ips.length
      ? hvKv("IP", esc(ips.join(", ")))
      : `<div class="hv-hint">${esc(hvT("hyperv.no_nic", "无网卡明细（需更新 Agent 采集）"))}</div>`;
  } else body = hvMiniTable(
    [hvT("hyperv.nic_name", "网卡"), "MAC", hvT("hyperv.nic_switch", "虚拟交换机"), hvT("hyperv.nic_status", "状态"), "IP"],
    nics.map(n => [
      esc(n.name || "—"),
      esc(hvMac(n.mac)),
      esc(n.switch || "—"),
      esc(n.status || (n.connected ? "Connected" : "—")),
      esc((n.ip_addresses || []).join(", ") || "—"),
    ]));
  const count = nics.length || ((g.ip_addresses || []).length ? 1 : 0);
  return `<div class="hv-card hv-full"><h4>${hvT("hyperv.network", "网络")} · ${count}</h4>${body}</div>`;
}

function hvCheckpointCard(g) {
  const cps = g.checkpoints || [];
  if (!cps.length) return "";
  const body = hvMiniTable(
    [hvT("hyperv.cp_name", "检查点"), hvT("hyperv.cp_created", "创建时间")],
    cps.map(c => [esc(c.name || "—"), esc((c.created || "").replace("T", " "))]));
  return `<div class="hv-card hv-full"><h4>${hvT("hyperv.checkpoints", "检查点")} · ${cps.length}</h4>${body}</div>`;
}

// hvQuickStats renders an at-a-glance chip row under the detail title.
function hvQuickStats(g) {
  const running = g.state === "Running";
  const chip = (k, v) => `<span class="hv-chip"><span class="ck">${k}</span><span class="cv">${v}</span></span>`;
  const chips = [
    chip("CPU", running ? Math.round(g.cpu_usage || 0) + "%" : "—"),
    chip(hvT("hyperv.memory", "内存"), running && g.mem_assigned_mb ? Math.round(g.mem_assigned_mb) + " MB" : "—"),
    chip("vCPU", g.processor_count || "—"),
    chip(hvT("hyperv.ov_uptime", "运行"), running && g.uptime_sec ? esc(fmtUptime(g.uptime_sec)) : "—"),
  ];
  const dc = (g.disks || []).length || g.vhd_count || 0;
  if (dc) chips.push(chip(hvT("hyperv.disk", "硬盘"), dc));
  const nc = (g.nics || []).length;
  if (nc) chips.push(chip(hvT("hyperv.network", "网卡"), nc));
  return `<div class="hv-chips">${chips.join("")}</div>`;
}

function hvDetailFor(inv, g) {
  const head = `<div class="hv-dhead">
    <span class="hv-dot ${hvDotClass(g)}"></span>
    <span class="title">${esc(g.name)}</span>
    ${hvStateBadge(g)}
    ${g.linked_host_id ? `<a class="hv-link" data-hvjump="${esc(g.linked_host_id)}" data-hvname="${esc(g.linked_host_name || g.name)}" style="margin-left:auto">${esc(hvT("hyperv.open_host", "打开纳管主机"))} ↗</a>` : ""}
  </div>`;
  const cards = [
    hvOverviewCard(inv, g), hvCpuCard(g), hvMemCard(g),
    hvDiskCard(g), hvNetCard(g), hvCheckpointCard(g),
  ].join("");
  return `<div class="hv-detailbox">${head}${hvQuickStats(g)}<div class="hv-cards">${cards}</div></div>`;
}

function hvDetail() {
  const sel = hvFindSelected();
  if (!sel) return `<div class="hv-detailbox"><div class="hv-empty">${esc(hvT("hyperv.pick", "从左侧选择一台虚拟机，查看其 CPU / 内存 / 硬盘 / 网络 详情"))}</div></div>`;
  return hvDetailFor(sel.inv, sel.g);
}

/* ---------- 渲染 ---------- */

function renderHyperVDetailOnly() {
  const d = $("hvDetail");
  if (d) d.innerHTML = hvDetail();
}

function renderHyperVPanel() {
  hvInjectStyles();
  const container = $("hypervPanel");
  if (!container) return;
  if (!HV_INVENTORIES.length) {
    container.innerHTML = `<div class="empty-line">${esc(hvT("hyperv.empty", "暂无 Hyper-V 宿主机数据。请确认物理机已安装并更新 Agent（含 Hyper-V 采集），且该主机已启用 Hyper-V 角色。"))}</div>`;
    return;
  }
  if (!hvFindSelected()) HV_SELECTED = hvFirstSelectable();

  let totalVMs = 0, totalBad = 0;
  HV_INVENTORIES.forEach(inv => {
    totalVMs += (inv.guests || []).length;
    totalBad += (inv.guests || []).filter(hvAbnormal).length;
  });
  const hosts = HV_INVENTORIES.slice().sort((a, b) =>
    (b.guests || []).filter(hvAbnormal).length - (a.guests || []).filter(hvAbnormal).length);
  const tree = hvToolbar(HV_INVENTORIES.length, totalVMs, totalBad) +
    `<div class="hv-treebox">${hosts.map(hvHostNode).join("")}</div>`;
  container.innerHTML = `<div class="hv-wrap"><div class="hv-tree" id="hvTree">${tree}</div><div class="hv-detail" id="hvDetail">${hvDetail()}</div></div>`;
}

/* ---------- 事件（全部委托） ---------- */

safeAddEventListener("hypervPanel", "click", e => {
  const refresh = e.target.closest("[data-hvrefresh]");
  if (refresh) { loadHyperVPanel(); return; }
  const jump = e.target.closest("[data-hvjump]");
  if (jump && typeof openDetail === "function") {
    openDetail(jump.dataset.hvjump, jump.dataset.hvname || jump.dataset.hvjump);
    return;
  }
  const htoggle = e.target.closest("[data-hvhosttoggle]");
  if (htoggle) {
    const id = htoggle.dataset.hvhosttoggle;
    if (HV_COLLAPSED.has(id)) HV_COLLAPSED.delete(id); else HV_COLLAPSED.add(id);
    renderHyperVPanel();
    return;
  }
  const vm = e.target.closest(".hv-vm[data-hvvm]");
  if (vm) { hvSelect(vm.dataset.hvhost, vm.dataset.hvvm); return; }
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

/* ---------- 样式（注入一次） ---------- */

function hvInjectStyles() {
  if (document.getElementById("hv-css")) return;
  const s = document.createElement("style");
  s.id = "hv-css";
  s.textContent = `
  .hv-wrap{display:flex;gap:14px;align-items:flex-start}
  .hv-tree{flex:0 0 340px;max-width:340px;min-width:250px}
  .hv-detail{flex:1 1 auto;min-width:0}
  @media(max-width:900px){.hv-wrap{flex-direction:column}.hv-tree{flex-basis:auto;max-width:none;width:100%}}
  .hv-summary{font-size:12px;margin:-4px 2px 10px}
  .hv-treebox{background:var(--panel);border:1px solid var(--line);border-radius:10px;overflow:hidden}
  .hv-hostnode{border-bottom:1px solid var(--line)}
  .hv-hostnode:last-child{border-bottom:none}
  .hv-hosthead{display:flex;align-items:center;gap:8px;padding:9px 10px;cursor:pointer;user-select:none}
  .hv-hosthead:hover{background:var(--panel2)}
  .hv-hosthead .name{font-weight:600;color:var(--txt);flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .hv-hostcount{display:inline-flex;align-items:center;gap:4px;font-size:11px;color:var(--txt2);background:var(--panel3);border:1px solid var(--line);border-radius:20px;padding:1px 8px;font-variant-numeric:tabular-nums;flex:0 0 auto}
  .hv-hostcount .hc-bad{color:var(--warn-txt);font-weight:600}
  .hv-hostcount .hc-bad::before{content:"⚠ "}
  .hv-caret2{color:var(--muted);font-size:11px;width:12px;text-align:center;flex:0 0 12px}
  .hv-vmlist{padding:2px 0 6px}
  .hv-vm{display:flex;align-items:center;gap:8px;padding:6px 12px 6px 28px;cursor:pointer;border-left:2px solid transparent}
  .hv-vm:hover{background:var(--panel2)}
  .hv-vm.selected{background:var(--accent-soft);border-left-color:var(--accent)}
  .hv-vm .vname{flex:1;color:var(--txt2);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:13px}
  .hv-vm.selected .vname{color:var(--txt);font-weight:600}
  .hv-vm.bad .vname{color:var(--warn-txt)}
  .hv-vm .vmini{font-size:11px;color:var(--muted);font-variant-numeric:tabular-nums;white-space:nowrap}
  .hv-dot{width:8px;height:8px;border-radius:50%;flex:0 0 8px}
  .hv-dot.ok{background:var(--ok)}.hv-dot.warn{background:var(--warn)}.hv-dot.crit{background:var(--crit)}.hv-dot.off{background:var(--muted)}
  .hv-detailbox{background:var(--panel);border:1px solid var(--line);border-radius:10px;padding:16px;min-height:200px}
  .hv-dhead{display:flex;align-items:center;gap:10px;flex-wrap:wrap;padding-bottom:12px;margin-bottom:14px;border-bottom:1px solid var(--line)}
  .hv-dhead .title{font-size:16px;font-weight:600;color:var(--txt)}
  .hv-chips{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:14px}
  .hv-chip{display:inline-flex;align-items:center;gap:6px;background:var(--panel2);border:1px solid var(--line);border-radius:8px;padding:5px 10px;font-size:12px}
  .hv-chip .ck{color:var(--muted)}
  .hv-chip .cv{color:var(--txt);font-weight:600;font-variant-numeric:tabular-nums}
  .hv-cards{display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:12px}
  .hv-card{background:var(--panel2);border:1px solid var(--line);border-radius:8px;padding:12px}
  .hv-card.hv-full{grid-column:1/-1}
  .hv-card h4{margin:0 0 10px;font-size:11px;color:var(--muted);font-weight:600;text-transform:uppercase;letter-spacing:.04em;display:flex;align-items:center;gap:6px}
  .hv-card h4::before{content:"";width:3px;height:11px;border-radius:2px;background:var(--accent);opacity:.7}
  .hv-kv{display:flex;justify-content:space-between;gap:10px;padding:3px 0;font-size:13px;border-bottom:1px dashed transparent}
  .hv-kv .k{color:var(--muted);white-space:nowrap}
  .hv-kv .v{color:var(--txt);text-align:right;font-variant-numeric:tabular-nums;word-break:break-word}
  .hv-mini-table{width:100%;border-collapse:collapse;font-size:12px}
  .hv-mini-table th{text-align:left;color:var(--muted);font-weight:500;padding:4px 10px 5px 0;border-bottom:1px solid var(--line);white-space:nowrap}
  .hv-mini-table td{padding:5px 10px 5px 0;color:var(--txt2);border-bottom:1px solid var(--line);word-break:break-all;vertical-align:top}
  .hv-mini-table tr:last-child td{border-bottom:none}
  .hv-hint,.hv-empty{color:var(--muted);font-size:12px}
  .hv-empty{padding:40px 12px;text-align:center;font-size:13px}
  .hv-link{color:var(--accent);cursor:pointer;text-decoration:none}
  .hv-link:hover{text-decoration:underline}`;
  document.head.appendChild(s);
}

/* 供 nav.js 的 _pageRenderers 调用 */
if (typeof window._pageRenderers === "undefined") window._pageRenderers = {};
window._pageRenderers.hyperv = loadHyperVPanel;
