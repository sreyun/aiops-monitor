/* ---------- 渲染：主机卡片 ---------- */
function hostCard(h) {
  const m = h.latest || {};
  const swap = (m.swap_total || 0) > 0
    ? bar(I18N.t("section.swap"), m.swap_percent || 0, (m.swap_percent || 0).toFixed(1) + "% · " + fmtGB(m.swap_used || 0) + "/" + fmtGB(m.swap_total || 0) + I18N.t("unit.gb"))
    : "";
  const disks = (Array.isArray(m.disks) ? m.disks : []).filter(d => !isSystemMount(d.path));
  const disksHtml = disks.length
    ? disks.map(d => bar(I18N.t("ui.disk_label") + " " + esc(d.path) + (d.percent >= 90 ? " ⚠" : ""), d.percent, d.percent.toFixed(1) + "% · " + fmtGB(d.used) + "/" + fmtGB(d.total) + I18N.t("unit.gb"))).join("")
    : bar(I18N.t("ui.disk"), m.disk_percent || 0, (m.disk_percent || 0).toFixed(1) + "% · " + fmtGB(m.disk_used || 0) + "/" + fmtGB(m.disk_total || 0) + I18N.t("unit.gb"), "disk");
  const gpus = Array.isArray(m.gpus) ? m.gpus : [];
  const gpusHtml = gpus.map(g => {
    const util = Math.max(0, Math.min(g.util_percent || 0, 100));
    const memTxt = (g.mem_total || 0) > 0 ? " · " + I18N.t("ui.gpu_mem_short") + " " + fmtGB(g.mem_used || 0) + "/" + fmtGB(g.mem_total || 0) + I18N.t("unit.gb") : "";
    const tempTxt = (g.temp || 0) > 0 ? " · " + Math.round(g.temp) + "℃" : "";
    const name = esc((g.name || "GPU").slice(0, 22));
    return `<div class="metric gpu"><div class="row"><span class="label">GPU ${name}</span>
      <span class="val mono">${(g.util_percent || 0).toFixed(0)}%${memTxt}${tempTxt}</span></div>
      <div class="bar"><div class="fill" style="width:${util}%;background:${usageColor(g.util_percent || 0)}"></div></div></div>`;
  }).join("");
  let chips = "";
  if (h.custom && Object.keys(h.custom).length) {
    chips = `<div class="chips">` + Object.entries(h.custom).sort().map(([k, v]) => {
      const isDown = /\.up$/.test(k) && v === 0;
      const num = Number.isInteger(v) ? v : v.toFixed(1);
      return `<span class="chip ${isDown ? "crit" : ""}">${esc(k)} <b>${num}</b></span>`;
    }).join("") + `<span class="chip-label">${I18N.t("section.custom_metrics")}</span></div>`;
  }
  const catLabel = (h.folder_path || h.category)
    ? esc(h.folder_path || h.category)
    : I18N.t("section.uncategorized");
  const loadTitle = I18N.t("section.load_avg") + (h.os === "windows" ? I18N.t("misc.windows_approx") : "");
  const lastCell = !h.online
    ? `<span class="g offline-tag" title="${I18N.t("section.last_seen")} ${fmtDateTime(h.last_seen)}">⚠ ${I18N.t("ui.offline_status")} ${ago(h.last_seen)}</span>`
    : h.stale
      ? `<span class="g stale-tag" title="${I18N.t("section.data_stale")}，${I18N.t("section.last_seen")} ${fmtDateTime(h.last_seen)}">⚠ ${I18N.t("ui.data")} ${ago(h.last_seen)}</span>`
      : `<span class="g">${I18N.t("ui.running")} ${fmtUptime(m.uptime || 0)}</span>`;
  return `<div class="host ${h.online ? "online" : "offline"}" tabindex="0" data-id="${esc(h.id)}" data-name="${esc(h.hostname || h.id)}" data-cat="${esc(h.category || "")}" data-folder="${esc(h.folder_id || "")}">
    <div class="host-head">
      <div class="host-name"><span class="dot ${h.online ? "on" : "off"}"></span>
        <div class="hn" data-act="detail" title="${esc(h.hostname || h.id)}">${esc(h.hostname || h.id)}</div>
      </div>
      <div class="host-tags">
        <span class="cat-badge" data-act="cat" title="${I18N.t('section.click_set_folder')}">${catLabel}</span>
        <span class="os-badge">${esc((h.os || "?").toUpperCase())}</span>
        ${(h.online && TERMINAL_ENABLED) ? `<button class="term-btn" data-act="term" title="${I18N.t('section.terminal_desc')}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg></button>` : ""}
        ${(h.online && DESKTOP_ENABLED) ? `<button class="term-btn desktop-btn" data-act="desktop" title="${I18N.t('desktop.btn_title')}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg></button>` : ""}
        <button class="x-btn" data-act="del" title="${I18N.t("ui.delete")}">✕</button>
      </div>
    </div>
    <div class="host-meta" title="${esc([h.ip, h.platform, h.arch].filter(Boolean).join(" · "))}">
      <span class="hm-ip mono">${h.ip ? esc(h.ip) : "—"}</span>
      <span class="hm-sep">·</span>
      <span class="hm-os">${esc(h.platform || "—")}${h.arch ? " · " + esc(h.arch) : ""}</span>
    </div>
    ${bar("CPU", m.cpu_percent || 0, (m.cpu_percent || 0).toFixed(1) + "% · " + (m.cpu_cores || 0) + I18N.t("ui.cores"), "cpu")}
    ${bar(I18N.t("ui.memory"), m.mem_percent || 0, (m.mem_percent || 0).toFixed(1) + "% · " + fmtGB(m.mem_used || 0) + "/" + fmtGB(m.mem_total || 0) + I18N.t("unit.gb"), "mem")}
    ${swap}
    ${disksHtml}
    ${gpusHtml}
    <div class="loadline" title="${loadTitle}">
      <div class="load-cell"><div class="lv mono">${(m.load1 || 0).toFixed(2)}</div><div class="lk">${I18N.t("section.load_1m")}</div></div>
      <div class="load-cell"><div class="lv mono">${(m.load5 || 0).toFixed(2)}</div><div class="lk">${I18N.t("section.load_5m")}</div></div>
      <div class="load-cell"><div class="lv mono">${(m.load15 || 0).toFixed(2)}</div><div class="lk">${I18N.t("section.load_15m")}</div></div>
    </div>
    ${chips}
    <div class="foot">
      <span class="g">↑<span class="mono">${fmtRate(m.net_sent_rate || 0)}</span> ↓<span class="mono">${fmtRate(m.net_recv_rate || 0)}</span></span>
      <span class="g">💾<span class="mono">${I18N.t("ui.disk_read")} ${fmtIORate(m.disk_read_rate || 0)}</span> <span class="mono">${I18N.t("ui.disk_write")} ${fmtIORate(m.disk_write_rate || 0)}</span></span>
      <span class="g">💿<span class="mono">${fmtIOPS((m.disk_read_iops || 0) + (m.disk_write_iops || 0))} ${I18N.t("unit.iops")}</span></span>
      <span class="g">🔗<span class="mono">${m.net_conns || 0}</span> ${I18N.t("section.connections")}</span>
      <span class="g">📊<span class="mono">${m.proc_count || 0}</span> ${I18N.t("section.processes")}</span>
      ${lastCell}
    </div>
  </div>`;
}

/* ---------- 渲染：主机列表行（列表视图） ---------- */
function hostRow(h) {
  const m = h.latest || {};
  const disks = (Array.isArray(m.disks) ? m.disks : []).filter(d => !isSystemMount(d.path));
  const diskMax = disks.length ? Math.max(...disks.map(d => d.percent)) : (m.disk_percent || 0);
  const gpus = Array.isArray(m.gpus) ? m.gpus : [];
  const gpuMax = gpus.length ? Math.max(...gpus.map(g => g.util_percent || 0)) : null;
  // Mini metric bar: label + progress bar + value
  const miniBar = (label, v) => {
    const pct = Math.max(0, Math.min(v || 0, 100));
    const color = usageColor(v || 0);
    return `<div class="hrow-mbar" title="${label} ${pct.toFixed(1)}%">
      <span class="hm-k">${label}</span>
      <div class="hm-track"><div class="hm-fill" style="width:${pct}%;background:${color}"></div></div>
      <span class="hm-v mono" style="color:${color}">${pct.toFixed(0)}%</span>
    </div>`;
  };
  const isStale = h.online && h.stale;
  const statusCls = !h.online ? "offline" : isStale ? "stale" : "online";
  const last = !h.online
    ? `<span class="hrow-status offline" title="${I18N.t("section.last_seen")} ${fmtDateTime(h.last_seen)}">⚠ ${I18N.t("ui.offline_status")} ${ago(h.last_seen)}</span>`
    : isStale
      ? `<span class="hrow-status stale" title="${I18N.t('section.data_stale')}">⚠ ${ago(h.last_seen)}</span>`
      : `<span class="hrow-status online">${I18N.t("ui.running")} ${fmtUptime(m.uptime || 0)}</span>`;
  const catLabel = (h.folder_path || h.category)
    ? esc(h.folder_path || h.category)
    : I18N.t("section.uncategorized");
  const termBtn = (h.online && TERMINAL_ENABLED)
    ? `<button class="term-btn" data-act="term" title="${I18N.t('ui.remote_terminal')}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg></button>`
    : "";
  const deskBtn = (h.online && DESKTOP_ENABLED)
    ? `<button class="term-btn desktop-btn" data-act="desktop" title="${I18N.t('desktop.btn_title')}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg></button>`
    : "";
  const loadStr = m.load1 !== undefined ? `${I18N.t("ui.load")} ${(m.load1||0).toFixed(2)} / ${(m.load5||0).toFixed(2)}` : "";
  const ipTitle = h.ip ? esc(h.ip) : "";
  return `<div class="host hrow ${statusCls}" tabindex="0" data-id="${esc(h.id)}" data-name="${esc(h.hostname || h.id)}" data-cat="${esc(h.category || "")}" data-folder="${esc(h.folder_id || "")}">
    <span class="hrow-dot ${h.online ? "on" : "off"}"></span>
    <div class="hrow-id">
      <div class="hrow-name" data-act="detail" title="${esc(h.hostname || h.id)}">${esc(h.hostname || h.id)}</div>
      <div class="hrow-sub" title="${ipTitle}">${h.ip ? `<span class="mono">${esc(h.ip)}</span>` : ""}${h.platform ? `<span class="hrow-sep">·</span>${esc(h.platform)}` : ""}</div>
    </div>
    <span class="os-badge">${esc((h.os || "?").toUpperCase())}</span>
    <span class="cat-badge" data-act="cat" title="${I18N.t('section.click_set_folder')}">${catLabel}</span>
    <div class="hrow-metrics">
      ${miniBar("CPU", m.cpu_percent)}${miniBar(I18N.t("ui.memory"), m.mem_percent)}${miniBar(I18N.t("ui.disk"), diskMax)}${gpuMax !== null ? miniBar("GPU", gpuMax) : ""}
    </div>
    <span class="hrow-net g">↑<span class="mono">${fmtRate(m.net_sent_rate || 0)}</span> ↓<span class="mono">${fmtRate(m.net_recv_rate || 0)}</span></span>
    ${loadStr ? `<span class="hrow-load mono">${loadStr}</span>` : ""}
    <span class="hrow-last">${last}</span>
    <span class="ch-actions hrow-actions">${termBtn}${deskBtn}<button class="mini-btn del" data-act="del" title="${I18N.t("ui.delete")}">✕</button></span>
  </div>`;
}

function setCurFolder(id) {
  CUR_FOLDER = id || "";
  try { localStorage.setItem("aiops_host_folder", CUR_FOLDER); } catch (e) {}
  HOST_PAGE = 1;
}

function setCurType(key) {
  CUR_TYPE = key || "";
  try { localStorage.setItem("aiops_host_type", CUR_TYPE); } catch (e) {}
  HOST_PAGE = 1;
}

function setHostTreeMode(mode) {
  HOST_TREE_MODE = mode === "type" ? "type" : "folder";
  try { localStorage.setItem("aiops_host_tree_mode", HOST_TREE_MODE); } catch (e) {}
  HOST_PAGE = 1;
}

function persistHostTreeCollapsed() {
  try { localStorage.setItem("aiops_host_tree_collapsed", JSON.stringify([...HOST_TREE_COLLAPSED])); } catch (e) {}
}

function hostFolderMatchSet(folderId) {
  if (!folderId) return null;
  if (folderId === "__ungrouped__") return new Set(["__ungrouped__"]);
  const ids = new Set();
  const walk = (nodes) => {
    for (const n of nodes || []) {
      if (n.id === folderId) {
        const collect = (x) => { ids.add(x.id); (x.children || []).forEach(collect); };
        collect(n);
        return true;
      }
      if (walk(n.children)) return true;
    }
    return false;
  };
  if (!walk(HOST_FOLDERS.folders || []) || ids.size === 0) {
    // Stale localStorage / deleted folder — clear filter instead of emptying the list.
    if (CUR_FOLDER === folderId) setCurFolder("");
    return null;
  }
  return ids;
}

function flattenHostFolders(folders, prefix) {
  const out = [];
  (folders || []).forEach(n => {
    const path = prefix ? prefix + " / " + n.name : n.name;
    out.push({ id: n.id, name: n.name, path });
    out.push(...flattenHostFolders(n.children || [], path));
  });
  return out;
}

function hostTypeKey(h) {
  const p = (h.platform || "").trim();
  if (p) return p;
  const os = (h.os || "").trim().toLowerCase();
  if (os === "windows") return "Windows";
  if (os === "darwin" || os === "macos") return "macOS";
  if (os === "linux") return "Linux";
  return os ? os : I18N.t("section.type_unknown");
}

function hostInFolderFilter(h, matchSet) {
  if (!matchSet) return true;
  const fid = h.folder_id || "__ungrouped__";
  return matchSet.has(fid);
}

function hostInTypeFilter(h) {
  if (!CUR_TYPE) return true;
  return hostTypeKey(h) === CUR_TYPE;
}

function currentHostsCrumb() {
  if (HOST_TREE_MODE === "type") {
    return CUR_TYPE
      ? I18N.t("section.type_tree") + " / " + CUR_TYPE
      : I18N.t("section.all_hosts_tree");
  }
  if (!CUR_FOLDER) return I18N.t("section.all_hosts_tree");
  if (CUR_FOLDER === "__ungrouped__") return I18N.t("section.uncategorized");
  const flat = flattenHostFolders(HOST_FOLDERS.folders || []);
  const cur = flat.find(x => x.id === CUR_FOLDER);
  return cur ? cur.path : CUR_FOLDER;
}

async function loadHostFolders() {
  try {
    const r = await fetch(`${API}/host-folders`);
    if (!r.ok) return;
    const data = await r.json();
    HOST_FOLDERS = {
      folders: data.folders || [],
      assign: data.assign || {},
      paths: data.paths || {},
      counts: data.counts || {}
    };
  } catch (e) {}
}

function folderMatchesTreeQ(n, q) {
  if (!q) return true;
  if ((n.name || "").toLowerCase().includes(q)) return true;
  return (n.children || []).some(c => folderMatchesTreeQ(c, q));
}

function hostTreeNodeHTML(n, depth, q) {
  if (q && !folderMatchesTreeQ(n, q)) return "";
  const cnt = (HOST_FOLDERS.counts && HOST_FOLDERS.counts[n.id]) || { total: 0, online: 0 };
  const sel = HOST_TREE_MODE === "folder" && CUR_FOLDER === n.id;
  const hasKids = (n.children || []).length > 0;
  const collapsed = !q && HOST_TREE_COLLAPSED.has(n.id);
  const canAdd = true; // nesting is unlimited (bounded only by a high safety cap)
  const pad = 4 + (depth - 1) * 10;
  let kids = "";
  if (hasKids && !collapsed) {
    // --gx positions the vertical guide line under this node's caret centre.
    kids = `<div class="htx-children" style="--gx:${pad + 6}px">${(n.children || []).map(c => hostTreeNodeHTML(c, depth + 1, q)).join("")}</div>`;
  }
  return `<div class="htx-folder" data-depth="${depth}">
    <div class="htx-node${sel ? " selected" : ""}${hasKids ? " has-kids" : ""}" data-folder-sel="${esc(n.id)}" data-ctx-folder="${esc(n.id)}" role="button" tabindex="0" style="padding-left:${pad}px">
      <span class="htx-caret${hasKids ? "" : " empty"}" data-folder-toggle="${esc(n.id)}" title="${hasKids ? I18N.t("section.folder_toggle") : ""}">${hasKids ? (collapsed ? "▸" : "▾") : ""}</span>
      <span class="htx-ico" aria-hidden="true"></span>
      <span class="htx-name" title="${esc(n.name)}">${esc(n.name)}</span>
      <span class="htx-count">${cnt.total || 0}</span>
      <span class="htx-acts">
        ${canAdd ? `<button type="button" class="htx-act htx-add" data-folder-add="${esc(n.id)}" title="${I18N.t("section.folder_add_child")}">+</button>` : ""}
        <button type="button" class="htx-act" data-folder-ren="${esc(n.id)}" title="${I18N.t("section.folder_rename")}">✎</button>
        <button type="button" class="htx-act danger" data-folder-del="${esc(n.id)}" title="${I18N.t("section.folder_delete")}">✕</button>
      </span>
    </div>
    ${kids}
  </div>`;
}

function hostTypeTreeHTML(q) {
  const hosts = LAST_HOSTS || [];
  const map = {};
  hosts.forEach(h => {
    const k = hostTypeKey(h);
    if (!map[k]) map[k] = { total: 0, online: 0 };
    map[k].total++;
    if (h.online) map[k].online++;
  });
  const keys = Object.keys(map).sort((a, b) => a.localeCompare(b));
  const filtered = q ? keys.filter(k => k.toLowerCase().includes(q)) : keys;
  const allCnt = hosts.length;
  const rows = filtered.map(k => {
    const sel = CUR_TYPE === k;
    return `<div class="htx-node${sel ? " selected" : ""}" data-type-sel="${esc(k)}" role="button" tabindex="0" style="padding-left:4px">
      <span class="htx-caret empty"></span>
      <span class="htx-ico htx-ico-type" aria-hidden="true"></span>
      <span class="htx-name" title="${esc(k)}">${esc(k)}</span>
      <span class="htx-count">${map[k].total}</span>
    </div>`;
  }).join("");
  return `<div class="htx-node htx-special${CUR_TYPE === "" ? " selected" : ""}" data-type-sel="" role="button" tabindex="0" style="padding-left:4px">
      <span class="htx-caret empty"></span>
      <span class="htx-ico htx-ico-all" aria-hidden="true"></span>
      <span class="htx-name">${I18N.t("section.all_hosts_tree")}</span>
      <span class="htx-count">${allCnt}</span>
    </div>
    <div class="htx-sep"></div>
    ${rows || `<div class="htx-empty">${I18N.t("section.type_empty_hint")}</div>`}`;
}

function hostAssetTreeHTML(q) {
  const allCnt = (LAST_HOSTS || []).length;
  const ug = (HOST_FOLDERS.counts && HOST_FOLDERS.counts.__ungrouped__) || { total: 0, online: 0 };
  const folders = HOST_FOLDERS.folders || [];
  const showSpecial = !q || I18N.t("section.all_hosts_tree").toLowerCase().includes(q)
    || I18N.t("section.uncategorized").toLowerCase().includes(q);
  return `${showSpecial ? `<div class="htx-node htx-special${CUR_FOLDER === "" ? " selected" : ""}" data-folder-sel="" role="button" tabindex="0" style="padding-left:4px">
        <span class="htx-caret empty"></span>
        <span class="htx-ico htx-ico-all" aria-hidden="true"></span>
        <span class="htx-name">${I18N.t("section.all_hosts_tree")}</span>
        <span class="htx-count">${allCnt}</span>
      </div>
      <div class="htx-node htx-special${CUR_FOLDER === "__ungrouped__" ? " selected" : ""}" data-folder-sel="__ungrouped__" data-ctx-folder="__ungrouped__" role="button" tabindex="0" style="padding-left:4px">
        <span class="htx-caret empty"></span>
        <span class="htx-ico htx-ico-none" aria-hidden="true"></span>
        <span class="htx-name">${I18N.t("section.uncategorized")}</span>
        <span class="htx-count">${ug.total || 0}</span>
      </div>
      <div class="htx-sep"></div>` : ""}
      ${folders.map(n => hostTreeNodeHTML(n, 1, q)).join("") || `<div class="htx-empty">${I18N.t("section.folder_empty_hint")}</div>`}`;
}

function hostTreeHTML() {
  const mode = HOST_TREE_MODE === "type" ? "type" : "folder";
  const q = (HOST_TREE_Q || "").trim().toLowerCase();
  const body = mode === "type" ? hostTypeTreeHTML(q) : hostAssetTreeHTML(q);
  return `<div class="htx-tabs">
      <button type="button" class="htx-tab${mode === "folder" ? " active" : ""}" data-tree-mode="folder">${I18N.t("section.asset_tree")}</button>
      <button type="button" class="htx-tab${mode === "type" ? " active" : ""}" data-tree-mode="type">${I18N.t("section.type_tree")}</button>
      <span class="htx-tab-tools">
        <button type="button" class="htx-tool-btn" data-folder-refresh title="${I18N.t("section.host_refresh")}">
          <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
        </button>
        ${mode === "folder" ? `<button type="button" class="htx-tool-btn htx-add-root" data-folder-add="" title="${I18N.t("section.folder_add_root")}">+</button>` : ""}
      </span>
    </div>
    <div class="htx-tree-search">
      <input type="search" id="hostTreeSearch" class="htx-tree-q" value="${esc(HOST_TREE_Q || "")}"
        placeholder="${esc(mode === "type" ? I18N.t("section.type_search_ph") : I18N.t("section.folder_search_ph"))}" autocomplete="off">
    </div>
    <div class="htx-scroll">${body}</div>`;
}

function renderHostTree() {
  const el = $("hostTree");
  if (!el) return;
  const focusId = document.activeElement && document.activeElement.id === "hostTreeSearch";
  const caret = focusId ? document.activeElement.selectionStart : null;
  el.innerHTML = hostTreeHTML();
  const layout = $("hostsLayout");
  if (layout && window.treeCollapsed) {
    const col = window.treeCollapsed("aiops_host_tree");
    layout.classList.toggle("tree-collapsed", !!col);
    const btn = layout.querySelector("[data-tree-toggle]");
    if (btn) {
      btn.textContent = col ? "›" : "‹";
      btn.setAttribute("aria-expanded", col ? "false" : "true");
    }
  }
  if (focusId) {
    const inp = el.querySelector("#hostTreeSearch");
    if (inp) {
      inp.focus();
      try { inp.setSelectionRange(caret, caret); } catch (e) {}
    }
  }
}

function hideHostTreeCtx() {
  const m = document.getElementById("htxCtxMenu");
  if (m) m.remove();
}

function showHostTreeCtx(x, y, folderId) {
  hideHostTreeCtx();
  if (HOST_TREE_MODE !== "folder") return;
  if (folderId === "__ungrouped__") return;
  const menu = document.createElement("div");
  menu.id = "htxCtxMenu";
  menu.className = "htx-ctx";
  menu.style.left = x + "px";
  menu.style.top = y + "px";
  menu.innerHTML = `
    <button type="button" class="htx-ctx-item" data-ctx="add" data-id="${esc(folderId || "")}">${I18N.t("section.ctx_create_node")}</button>
    ${folderId ? `<button type="button" class="htx-ctx-item" data-ctx="ren" data-id="${esc(folderId)}">${I18N.t("section.ctx_rename_node")}</button>
    <button type="button" class="htx-ctx-item danger" data-ctx="del" data-id="${esc(folderId)}">${I18N.t("section.ctx_delete_node")}</button>` : ""}`;
  document.body.appendChild(menu);
  const rect = menu.getBoundingClientRect();
  if (rect.right > window.innerWidth - 8) menu.style.left = Math.max(8, window.innerWidth - rect.width - 8) + "px";
  if (rect.bottom > window.innerHeight - 8) menu.style.top = Math.max(8, window.innerHeight - rect.height - 8) + "px";
  const close = (e) => {
    if (e && menu.contains(e.target)) return;
    hideHostTreeCtx();
    document.removeEventListener("mousedown", close, true);
  };
  setTimeout(() => document.addEventListener("mousedown", close, true), 0);
  menu.addEventListener("click", async (e) => {
    const item = e.target.closest("[data-ctx]");
    if (!item) return;
    const act = item.getAttribute("data-ctx");
    const id = item.getAttribute("data-id") || "";
    hideHostTreeCtx();
    if (act === "add") await hostFolderAdd(id);
    else if (act === "ren") await hostFolderRename(id);
    else if (act === "del") await hostFolderDelete(id);
  });
}

async function hostFolderAdd(parentId) {
  const flat = flattenHostFolders(HOST_FOLDERS.folders || []);
  const parent = parentId ? flat.find(x => x.id === parentId) : null;
  const name = await promptFolderName({
    title: parentId ? I18N.t("section.folder_add_child") : I18N.t("section.folder_add_root"),
    parentPath: parent ? parent.path : "",
    defaultValue: "",
    placeholder: I18N.t("section.folder_name_ph")
  });
  if (name === null || !String(name).trim()) return;
  try {
    const r = await fetch(`${API}/host-folders`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ parent_id: parentId || "", name: String(name).trim() })
    });
    if (!r.ok) {
      const e = await r.json().catch(() => ({}));
      toast(e.error || I18N.t("toast.update_failed2"), "err");
      return;
    }
    toast(I18N.t("toast.folder_saved"), "ok");
    if (parentId) HOST_TREE_COLLAPSED.delete(parentId);
    persistHostTreeCollapsed();
    await loadHostFolders();
    renderHosts(LAST_HOSTS);
  } catch (e) { toast(I18N.t("toast.update_failed") + e, "err"); }
}

async function hostFolderRename(id) {
  const flat = flattenHostFolders(HOST_FOLDERS.folders || []);
  const cur = flat.find(x => x.id === id);
  const parentPath = cur && cur.path.includes(" / ")
    ? cur.path.slice(0, cur.path.lastIndexOf(" / "))
    : "";
  const name = await promptFolderName({
    title: I18N.t("section.folder_rename"),
    parentPath,
    defaultValue: cur ? cur.name : "",
    placeholder: I18N.t("section.folder_name_ph")
  });
  if (name === null || !String(name).trim()) return;
  try {
    const r = await fetch(`${API}/host-folders/${encodeURIComponent(id)}`, {
      method: "PATCH", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: String(name).trim() })
    });
    if (!r.ok) {
      const e = await r.json().catch(() => ({}));
      toast(e.error || I18N.t("toast.update_failed2"), "err");
      return;
    }
    toast(I18N.t("toast.folder_saved"), "ok");
    await loadHostFolders();
    refresh();
  } catch (e) { toast(I18N.t("toast.update_failed") + e, "err"); }
}

/** 主机分组专用轻量弹窗（不用 AI 反馈那套大弹层） */
function promptFolderName(opts) {
  opts = opts || {};
  return new Promise(resolve => {
    const existing = document.getElementById("htxFolderDlgMask");
    if (existing) existing.remove();
    const mask = document.createElement("div");
    mask.id = "htxFolderDlgMask";
    mask.className = "mask htx-dlg-mask show";
    const parentPath = opts.parentPath || "";
    mask.innerHTML = `
      <div class="htx-dlg" role="dialog" aria-modal="true" aria-labelledby="htxDlgTitle">
        <div class="htx-dlg-head">
          <h3 id="htxDlgTitle">${esc(opts.title || I18N.t("section.folder_add_root"))}</h3>
          <button type="button" class="htx-dlg-x" data-htx-dlg="cancel" aria-label="${esc(I18N.t("ui.close","关闭"))}">✕</button>
        </div>
        <div class="htx-dlg-body">
          ${parentPath ? `<div class="htx-dlg-path" title="${esc(parentPath)}"><span class="htx-dlg-path-k">${esc(I18N.t("section.folder_parent"))}</span><span class="htx-dlg-path-v">${esc(parentPath)}</span></div>` : ""}
          <label class="htx-dlg-label" for="htxDlgInput">${esc(I18N.t("section.folder_name"))}</label>
          <input type="text" id="htxDlgInput" class="htx-dlg-input" maxlength="48"
            placeholder="${esc(opts.placeholder || I18N.t("section.folder_name_ph"))}"
            value="${esc(opts.defaultValue || "")}" autocomplete="off" spellcheck="false">
          <div class="htx-dlg-hint">${esc(I18N.t("section.folder_name_hint"))}</div>
          <div class="htx-dlg-err" id="htxDlgErr" hidden></div>
        </div>
        <div class="htx-dlg-foot">
          <button type="button" class="btn" data-htx-dlg="cancel">${esc(I18N.t("ui.cancel","取消"))}</button>
          <button type="button" class="btn primary" data-htx-dlg="ok">${esc(I18N.t("ui.save","保存"))}</button>
        </div>
      </div>`;
    document.body.appendChild(mask);
    const input = mask.querySelector("#htxDlgInput");
    const err = mask.querySelector("#htxDlgErr");
    let done = false;
    const finish = (v) => {
      if (done) return;
      done = true;
      document.removeEventListener("keydown", onKey, true);
      mask.remove();
      resolve(v);
    };
    const submit = () => {
      const v = (input.value || "").trim();
      if (!v) {
        err.hidden = false;
        err.textContent = I18N.t("section.folder_name_required");
        input.focus();
        return;
      }
      if (/[\\/]/.test(v)) {
        err.hidden = false;
        err.textContent = I18N.t("section.folder_name_slash");
        input.focus();
        return;
      }
      finish(v);
    };
    const onKey = (e) => {
      if (e.key === "Escape") { e.preventDefault(); finish(null); }
      else if (e.key === "Enter") { e.preventDefault(); submit(); }
    };
    mask.addEventListener("click", (e) => {
      const act = e.target.closest("[data-htx-dlg]");
      if (!act) {
        if (e.target === mask) finish(null);
        return;
      }
      if (act.getAttribute("data-htx-dlg") === "ok") submit();
      else finish(null);
    });
    document.addEventListener("keydown", onKey, true);
    setTimeout(() => { input.focus(); input.select(); }, 30);
  });
}

async function hostFolderDelete(id) {
  const flat = flattenHostFolders(HOST_FOLDERS.folders || []);
  const cur = flat.find(x => x.id === id);
  if (!confirm(I18N.t("section.folder_delete_confirm") + (cur ? cur.path : id))) return;
  try {
    const r = await fetch(`${API}/host-folders/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (!r.ok) {
      const e = await r.json().catch(() => ({}));
      toast(e.error || I18N.t("toast.delete_failed"), "err");
      return;
    }
    // Clear selection if the current folder is the deleted node or under it.
    const match = hostFolderMatchSet(id);
    if (CUR_FOLDER === id || (match && match.has(CUR_FOLDER))) setCurFolder("");
    toast(I18N.t("toast.folder_deleted"), "ok");
    await loadHostFolders();
    refresh();
  } catch (e) { toast(I18N.t("toast.deleted") + ": " + e, "err"); }
}

function bindHostTreeOnce() {
  const tree = $("hostTree");
  if (!tree || tree.dataset.bound) return;
  tree.dataset.bound = "1";
  tree.addEventListener("input", (e) => {
    if (e.target && e.target.id === "hostTreeSearch") {
      HOST_TREE_Q = e.target.value || "";
      renderHostTree();
    }
  });
  tree.addEventListener("contextmenu", (e) => {
    const node = e.target.closest("[data-ctx-folder]");
    if (!node || HOST_TREE_MODE !== "folder") return;
    e.preventDefault();
    showHostTreeCtx(e.clientX, e.clientY, node.getAttribute("data-ctx-folder") || "");
  });
  tree.addEventListener("click", async (e) => {
    const modeBtn = e.target.closest("[data-tree-mode]");
    if (modeBtn) {
      setHostTreeMode(modeBtn.getAttribute("data-tree-mode"));
      HOST_TREE_Q = "";
      renderHosts(LAST_HOSTS);
      return;
    }
    if (e.target.closest("[data-folder-refresh]")) {
      e.stopPropagation();
      if (typeof refresh === "function") refresh();
      return;
    }
    const add = e.target.closest("[data-folder-add]");
    if (add) { e.stopPropagation(); await hostFolderAdd(add.getAttribute("data-folder-add")); return; }
    const ren = e.target.closest("[data-folder-ren]");
    if (ren) { e.stopPropagation(); await hostFolderRename(ren.getAttribute("data-folder-ren")); return; }
    const del = e.target.closest("[data-folder-del]");
    if (del) { e.stopPropagation(); await hostFolderDelete(del.getAttribute("data-folder-del")); return; }
    const tog = e.target.closest("[data-folder-toggle]");
    if (tog && !tog.classList.contains("empty")) {
      e.stopPropagation();
      const id = tog.getAttribute("data-folder-toggle");
      if (HOST_TREE_COLLAPSED.has(id)) HOST_TREE_COLLAPSED.delete(id);
      else HOST_TREE_COLLAPSED.add(id);
      persistHostTreeCollapsed();
      renderHostTree();
      return;
    }
    const typeSel = e.target.closest("[data-type-sel]");
    if (typeSel) {
      setCurType(typeSel.getAttribute("data-type-sel") || "");
      renderHosts(LAST_HOSTS);
      return;
    }
    const sel = e.target.closest("[data-folder-sel]");
    if (sel) {
      setCurFolder(sel.getAttribute("data-folder-sel") || "");
      renderHosts(LAST_HOSTS);
    }
  });
}

function bindHostsToolbarOnce() {
  if (window._htxToolbarBound) return;
  window._htxToolbarBound = true;
  document.addEventListener("click", (e) => {
    const moreBtn = e.target.closest("#htxMoreBtn");
    const menu = $("htxMoreMenu");
    const wrap = $("htxMoreWrap");
    if (moreBtn && menu) {
      menu.hidden = !menu.hidden;
      return;
    }
    if (menu && wrap && !wrap.contains(e.target)) menu.hidden = true;
  });
}

function renderHosts(hosts) {
  LAST_HOSTS = hosts;
  HOST_META = hosts.map(h => ({ id: h.id, hostname: h.hostname }));
  window._cachedHosts = hosts;
  if (DEFAULT_EMPTY === null && $("empty")) DEFAULT_EMPTY = $("empty").innerHTML;
  const countEl = $("hostsCount");
  if (countEl) countEl.textContent = hosts.length;
  const navHosts = $("navHosts");
  if (navHosts) navHosts.textContent = hosts.length;

  bindHostTreeOnce();
  bindHostsToolbarOnce();
  renderHostTree();

  if (!LAST_RENDER_KEY) {
    try {
      const s = localStorage.getItem("aiops_collapsed");
      if (s) {
        const arr = JSON.parse(s);
        const cats = [...new Set(hosts.map(h => h.category || I18N.t("section.uncategorized")))];
        if (Array.isArray(arr) && arr.length > 0 && cats.length > 0 && cats.every(c => arr.includes(c))) {
          localStorage.removeItem("aiops_collapsed");
        }
      }
    } catch (e) {}
  }

  const groupsEl = $("groups"), empty = $("empty"), pager = $("pager");
  if (!groupsEl || !empty || !pager) return;
  const dupBar = $("hostDupBar");
  if (dupBar) dupBar.innerHTML = dupBannerHTML();
  const crumb = $("hostsCrumb");
  if (crumb) crumb.textContent = currentHostsCrumb();

  const matchSet = HOST_TREE_MODE === "folder" ? hostFolderMatchSet(CUR_FOLDER) : null;
  let shown = hosts.filter(h => {
    if (HOST_TREE_MODE === "type") {
      if (!hostInTypeFilter(h)) return false;
    } else if (!hostInFolderFilter(h, matchSet)) {
      return false;
    }
    if (HOST_FILTER === "online" && !h.online) return false;
    if (HOST_FILTER === "offline" && h.online) return false;
    if (HOST_SEARCH) {
      const hay = ((h.hostname || "") + " " + (h.ip || "") + " " + (h.platform || "") + " " + (h.kernel || "") + " " + (h.category || "") + " " + (h.folder_path || "")).toLowerCase();
      if (!hay.includes(HOST_SEARCH.toLowerCase())) return false;
    }
    return true;
  });

  if (HOST_SORT === "cpu") {
    shown.sort((a, b) => (b.latest?.cpu_percent || 0) - (a.latest?.cpu_percent || 0));
  } else if (HOST_SORT === "mem") {
    shown.sort((a, b) => (b.latest?.mem_percent || 0) - (a.latest?.mem_percent || 0));
  } else if (HOST_SORT === "recent") {
    shown.sort((a, b) => (b.last_seen || 0) - (a.last_seen || 0));
  } else {
    shown.sort((a, b) => (a.hostname || a.id).localeCompare(b.hostname || b.id));
  }

  if (countEl) countEl.textContent = shown.length;

  if (!hosts.length) { groupsEl.innerHTML = ""; pager.innerHTML = ""; empty.style.display = "block"; empty.innerHTML = DEFAULT_EMPTY; return; }
  if (!shown.length) { groupsEl.innerHTML = ""; pager.innerHTML = ""; empty.style.display = "block"; empty.textContent = I18N.t("empty.no_host_match"); return; }
  empty.style.display = "none";

  const isList = HOST_VIEW === "list";
  const isMobile = window.innerWidth <= 480;
  const PAGINATION_THRESHOLD = isMobile ? (isList ? 20 : 10) : (isList ? 50 : 30);
  const pageSize = isList ? 50 : HOST_PAGE_SIZE;
  const shouldPaginate = shown.length > PAGINATION_THRESHOLD;
  let pageHosts, pages;
  if (shouldPaginate) {
    pages = Math.ceil(shown.length / pageSize);
    if (HOST_PAGE > pages) HOST_PAGE = pages;
    if (HOST_PAGE < 1) HOST_PAGE = 1;
    pageHosts = shown.slice((HOST_PAGE - 1) * pageSize, HOST_PAGE * pageSize);
  } else {
    HOST_PAGE = 1; pages = 1;
    pageHosts = shown;
  }

  const render = isList ? hostRow : hostCard;
  const wrapCls = isList ? "host-list" : "grid";
  const filterKey = HOST_TREE_MODE === "type" ? ("t:" + CUR_TYPE) : ("f:" + CUR_FOLDER);
  const newKey = pageHosts.map(h => h.id).join(",") + "|" + HOST_VIEW + "|" + HOST_PAGE + "|" + filterKey + "|" + HOST_TREE_MODE;
  if (LAST_RENDER_KEY === newKey && Object.keys(HOST_DOM_CACHE).length > 0) {
    pageHosts.forEach(h => updateHostCard(h));
    renderPager(pages, shown.length);
    renderHostTree();
    return;
  }
  LAST_RENDER_KEY = newKey;

  // 选中节点下扁平展示（卡片/列表），不再按路径二次分组
  groupsEl.innerHTML = `<div class="group htx-flat"><div class="${wrapCls}">${pageHosts.map(render).join("")}</div></div>`;
  buildHostCache();
  renderPager(pages, shown.length);
}

function renderPager(pages, total) {
  const pager = $("pager");
  if (!pager) return;
  if (pages <= 1) { pager.innerHTML = `<span class="pinfo">${I18N.t("section.pager_total", "共")} ${total} ${I18N.t("section.pager_hosts", "台")}</span>`; return; }
  let btns = `<button ${HOST_PAGE === 1 ? "disabled" : ""} data-pg="prev">‹</button>`;
  for (let i = 1; i <= pages; i++) {
    if (i === 1 || i === pages || Math.abs(i - HOST_PAGE) <= 1) {
      btns += `<button class="${i === HOST_PAGE ? "active" : ""}" data-pg="${i}">${i}</button>`;
    } else if (Math.abs(i - HOST_PAGE) === 2) {
      btns += `<span class="pinfo">…</span>`;
    }
  }
  btns += `<button ${HOST_PAGE === pages ? "disabled" : ""} data-pg="next">›</button>`;
  btns += `<span class="pinfo">${I18N.t("section.pager_total", "共")} ${total} ${I18N.t("section.pager_hosts", "台")} · ${HOST_PAGE}/${pages}</span>`;
  pager.innerHTML = btns;
}

/* ---------- 主机操作 ---------- */
async function delHost(id, name) {
  if (!confirm(`${I18N.t("valid.confirm_delete_host_prefix")}${I18N.t("ui.delete")}「${name}」？\n若该主机 Agent 仍在运行，约 60 ${I18N.t("time.sec")}后会重新出现。`)) return;
  try {
    const r = await fetch(`${API}/hosts/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (r.ok) { toast(I18N.t("toast.host_deleted"), "ok"); refresh(); } else { toast(I18N.t("toast.delete_failed"), "err"); }
  } catch (e) { toast(I18N.t("toast.deleted") + ": " + e, "err"); }
}

async function editCategory(id, cur) {
  const flat = flattenHostFolders(HOST_FOLDERS.folders || []);
  const options = [{ id: "__ungrouped__", path: I18N.t("section.uncategorized") }]
    .concat(flat.map(f => ({ id: f.id, path: f.path })));
  const host = (LAST_HOSTS || []).find(h => h.id === id);
  const curFid = (host && host.folder_id) || "__ungrouped__";
  const folderId = await promptMoveFolder({
    hostname: (host && host.hostname) || id,
    options,
    currentId: curFid
  });
  if (folderId === null) return;
  try {
    const r = await fetch(`${API}/hosts/${encodeURIComponent(id)}/folder`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ folder_id: folderId })
    });
    if (r.ok) { toast(I18N.t("toast.category_updated"), "ok"); await loadHostFolders(); refresh(); }
    else toast(I18N.t("toast.update_failed2"), "err");
  } catch (e) { toast(I18N.t("toast.update_failed") + e, "err"); }
}

function promptMoveFolder(opts) {
  opts = opts || {};
  const options = opts.options || [];
  return new Promise(resolve => {
    const existing = document.getElementById("htxFolderDlgMask");
    if (existing) existing.remove();
    const mask = document.createElement("div");
    mask.id = "htxFolderDlgMask";
    mask.className = "mask htx-dlg-mask show";
    const optsHtml = options.map(o =>
      `<option value="${esc(o.id)}"${o.id === opts.currentId ? " selected" : ""}>${esc(o.path)}</option>`
    ).join("");
    mask.innerHTML = `
      <div class="htx-dlg" role="dialog" aria-modal="true" aria-labelledby="htxDlgTitle">
        <div class="htx-dlg-head">
          <h3 id="htxDlgTitle">${esc(I18N.t("section.set_folder"))}</h3>
          <button type="button" class="htx-dlg-x" data-htx-dlg="cancel" aria-label="${esc(I18N.t("ui.close","关闭"))}">✕</button>
        </div>
        <div class="htx-dlg-body">
          <div class="htx-dlg-path"><span class="htx-dlg-path-k">${esc(I18N.t("section.host_short"))}</span><span class="htx-dlg-path-v">${esc(opts.hostname || "")}</span></div>
          <label class="htx-dlg-label" for="htxDlgSelect">${esc(I18N.t("section.folder_name"))}</label>
          <select id="htxDlgSelect" class="htx-dlg-input htx-dlg-select">${optsHtml}</select>
          <div class="htx-dlg-hint">${esc(I18N.t("section.set_folder_hint"))}</div>
        </div>
        <div class="htx-dlg-foot">
          <button type="button" class="btn" data-htx-dlg="cancel">${esc(I18N.t("ui.cancel","取消"))}</button>
          <button type="button" class="btn primary" data-htx-dlg="ok">${esc(I18N.t("ui.save","保存"))}</button>
        </div>
      </div>`;
    document.body.appendChild(mask);
    const sel = mask.querySelector("#htxDlgSelect");
    let done = false;
    const finish = (v) => {
      if (done) return;
      done = true;
      document.removeEventListener("keydown", onKey, true);
      mask.remove();
      resolve(v);
    };
    const onKey = (e) => {
      if (e.key === "Escape") { e.preventDefault(); finish(null); }
      else if (e.key === "Enter") { e.preventDefault(); finish(sel.value); }
    };
    mask.addEventListener("click", (e) => {
      const act = e.target.closest("[data-htx-dlg]");
      if (!act) {
        if (e.target === mask) finish(null);
        return;
      }
      if (act.getAttribute("data-htx-dlg") === "ok") finish(sel.value);
      else finish(null);
    });
    document.addEventListener("keydown", onKey, true);
    setTimeout(() => sel.focus(), 30);
  });
}

/* ---------- 主机趋势弹窗 ---------- */
let DETAIL_HOST_ID = '';
let DETAIL_HOST_NAME = '';
let DETAIL_TIME_RANGE = 1; // hours: 1/3/6/12/24/72/168/336（默认 1 小时）
let DETAIL_CUSTOM = null;   // {from,to} unix seconds — set when a custom range is active
let DETAIL_SAMPLES = [];

// 把 unix 秒格式化为 <input type="datetime-local"> 需要的本地时间字符串 YYYY-MM-DDTHH:mm
function toLocalDatetimeValue(unixSec) {
  const d = new Date(unixSec * 1000);
  const p = n => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}

// 统一的时间跨度控件渲染函数（主机图表和监控图表共用）
// 快捷时间跨度（小时）：1/3/6/12 小时 + 1/3/7/14 天（+ 自定义，由各视图单独渲染）
const CHART_SPANS = [1, 3, 6, 12, 24, 72, 168, 336];
function chartSpanLabel(h) {
  return h < 24 ? h + I18N.t("time.hour") : (h / 24) + I18N.t("time.day");
}
function renderChartControls(currentRange, prefix) {
  return CHART_SPANS.map(h =>
    `<button class="chip-btn ${currentRange === h ? "active" : ""}" data-${prefix}="${h}">${chartSpanLabel(h)}</button>`
  ).join("");
}
async function openDetail(id, name) {
  DETAIL_HOST_ID = id;
  DETAIL_HOST_NAME = name || id;
  DETAIL_TIME_RANGE = 1;
  DETAIL_CUSTOM = null;
  $("detailTitle").textContent = name + " " + I18N.t("section.recent_trend");
  const body = $("detailBody");
  body.innerHTML = `<div class="empty-line">${I18N.t("ui.loading")}</div>`;
  $("detailMask").classList.add("show");
  await loadAndRenderCharts();
}

async function loadAndRenderCharts() {
  const body = $("detailBody");
  const now = Math.floor(Date.now() / 1000);
  const to = DETAIL_CUSTOM ? DETAIL_CUSTOM.to : now;
  const from = DETAIL_CUSTOM ? DETAIL_CUSTOM.from : now - DETAIL_TIME_RANGE * 3600;
  const spanH = Math.max(0, (to - from) / 3600); // effective window in hours

  // 取消上一轮懒加载观察，避免切时间范围后旧回调继续触发。
  if (DETAIL_CHART_IO) { try { DETAIL_CHART_IO.disconnect(); } catch (_) {} DETAIL_CHART_IO = null; }
  DETAIL_CHART_PENDING = {};

  try {
    const samples = await fetch(`${API}/hosts/${encodeURIComponent(DETAIL_HOST_ID)}/history?from=${from}&to=${to}`).then(r => r.json());
    if (!Array.isArray(samples) || !samples.length) {
      DETAIL_SAMPLES = [];
      body.innerHTML = `<div class="empty-line">${I18N.t("empty.no_history")}</div>`;
      return;
    }
    DETAIL_SAMPLES = samples;

    // 组织图表：每个图表包裹在 .chart-wrap 内，右上角提供放大按钮；真正绘制延后到可见时（懒加载）。
    DETAIL_CHARTS = {};
    const gran = spanH <= 2 ? I18N.t("time.raw") : spanH <= 48 ? I18N.t("time.1m_agg") : I18N.t("time.5m_agg");
    const hasGPU = samples.some(s => Array.isArray(s.gpus) && s.gpus.length);
    const hasConns = samples.some(s => Array.isArray(s.conns) && s.conns.length);
    const pct = v => v.toFixed(1) + '%';
    const wrap = id => `<div class="chart-wrap" data-lazy-chart="${id}"><canvas id="${id}" width="1000" height="240"></canvas>` +
      `<button class="chart-enlarge" data-chart="${id}" title="${I18N.t('ui.zoom_preview')}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg></button></div>`;
    body.innerHTML = `
      <div class="chart-controls">
        ${renderChartControls(DETAIL_CUSTOM ? -1 : DETAIL_TIME_RANGE, "range")}
        <button class="chip-btn ${DETAIL_CUSTOM ? "active" : ""}" data-custom-toggle title="${I18N.t("time.custom_range") || "自定义时间范围"}">${I18N.t("time.custom") || "自定义"}</button>
        <button class="chip-btn ai-assist-btn" id="detailAIBtn" title="${I18N.t("hosts.ai_analyze_title","用 AI 解读该主机近期指标趋势")}"><span class="ai-assist-btn-ic">🤖</span>${I18N.t("hosts.ai_analyze","AI 分析")}</button>
        <span class="chart-custom-range" id="detailCustomPanel"${DETAIL_CUSTOM ? "" : " hidden"}>
          <input type="datetime-local" id="detailCustomFrom" class="dt-input" value="${toLocalDatetimeValue(from)}">
          <span class="dt-sep">→</span>
          <input type="datetime-local" id="detailCustomTo" class="dt-input" value="${toLocalDatetimeValue(to)}">
          <button class="chip-btn primary" data-custom-apply>${I18N.t("time.custom_apply") || "应用"}</button>
        </span>
      </div>
      <div class="chart-container">
        ${wrap('chartCPU')}${wrap('chartMem')}${wrap('chartLoad')}${wrap('chartDisk')}${hasGPU ? wrap('chartGPU') + wrap('chartGPUTemp') + wrap('chartGPUMemPct') + wrap('chartGPUMem') : ''}${wrap('chartNet')}${hasConns ? wrap('chartConns') + wrap('chartConnStates') : ''}${wrap('chartDiskIO')}${wrap('chartIOPS')}${wrap('chartProc')}
      </div>
      <div class="hint">${I18N.t("section.sample_points")}: ${samples.length} · ${I18N.t("section.granularity")}: ${gran}</div>
    `;

    // 先只登记「如何画」；进入视口后再 createChart，避免一次同步创建十多张 Canvas 卡顿。
    const lazy = (id, series, yMin, yMax, title) => {
      DETAIL_CHART_PENDING[id] = { samples, series, yMin, yMax, title };
    };
    lazy('chartCPU',
      [{ key: 'cpu_percent', label: I18N.t("section.cpu_usage"), color: '#4c8dff', fmt: pct }], 0, 100, I18N.t("section.cpu_usage"));
    lazy('chartMem',
      [{ key: 'mem_percent', label: I18N.t("section.mem_usage"), color: '#8b5cf6', fmt: pct }], 0, 100, I18N.t("section.mem_usage"));
    lazy('chartLoad', [
      { key: 'load1', label: I18N.t("section.load_1m_label"), color: '#4c8dff', fmt: v => v.toFixed(1) },
      { key: 'load5', label: I18N.t("section.load_5m_label"), color: '#f7b23b', fmt: v => v.toFixed(1) },
      { key: 'load15', label: I18N.t("section.load_15m_label"), color: '#f2545b', fmt: v => v.toFixed(1) },
    ], null, null, I18N.t("section.load_avg"));

    let diskProto = [];
    samples.forEach(s => { if (Array.isArray(s.disks) && s.disks.length > diskProto.length) diskProto = s.disks; });
    const diskKeys = diskProto.map(d => d.path);
    const latestDisk = {};
    for (let i = samples.length - 1; i >= 0 && Object.keys(latestDisk).length < diskKeys.length; i--) {
      (samples[i].disks || []).forEach(d => { if (!(d.path in latestDisk)) latestDisk[d.path] = d; });
    }
    const _gb = b => b / 1073741824;
    const diskLabel = (path) => {
      const d = latestDisk[path];
      if (!d || !d.total) return '磁盘 ' + path;
      const used = _gb(d.used), tot = _gb(d.total);
      return `磁盘 ${path} · 已用 ${used.toFixed(0)}/${tot.toFixed(0)}GB · 剩 ${(tot - used).toFixed(0)}GB`;
    };
    const diskSeries = diskKeys.map((path, idx) => ({
      key: `disk_${idx}`, label: diskLabel(path),
      color: ['#f7b23b', '#2fd07a', '#f2545b', '#43b6f0', '#8b5cf6', '#e06c9a'][idx % 6], fmt: pct,
      transform: (s) => { const d = (s.disks || []).find(x => x.path === path); return d ? d.percent : null; }
    }));
    lazy('chartDisk',
      diskSeries.length ? diskSeries : [{ key: 'disk_percent', label: I18N.t("section.root_partition"), color: '#f7b23b', fmt: pct }],
      0, 100, I18N.t("section.disk_usage"));

    if (hasGPU) {
      const gpuNames = [];
      samples.forEach(s => (s.gpus || []).forEach((g, i) => { if (!gpuNames[i]) gpuNames[i] = g.name || ('GPU' + i); }));
      const gpalette = ['#8b5cf6', '#43b6f0', '#2fd07a', '#f7b23b', '#f2545b', '#e06c9a'];
      const gcolor = idx => gpalette[idx % gpalette.length];
      const gpuVal = (idx, field) => (s) => { const g = s.gpus && s.gpus[idx] ? s.gpus[idx] : null; return g ? (g[field] || 0) : null; };
      const gbUnit = I18N.t("unit.gb");
      const gpuBytesGB = (idx, field) => (s) => { const g = s.gpus && s.gpus[idx] ? s.gpus[idx] : null; return g ? (g[field] || 0) / 1073741824 : null; };
      lazy('chartGPU', gpuNames.map((nm, idx) => ({
        key: `gpu_${idx}`, label: nm, color: gcolor(idx), fmt: v => v.toFixed(0) + '%', transform: gpuVal(idx, 'util_percent')
      })), 0, 100, I18N.t("section.gpu_usage"));
      lazy('chartGPUTemp', gpuNames.map((nm, idx) => ({
        key: `gput_${idx}`, label: nm, color: gcolor(idx), fmt: v => v.toFixed(0) + '℃', transform: gpuVal(idx, 'temp')
      })), null, null, I18N.t("section.gpu_temp"));
      lazy('chartGPUMemPct', gpuNames.map((nm, idx) => ({
        key: `gpump_${idx}`, label: nm, color: gcolor(idx), fmt: v => v.toFixed(0) + '%', transform: gpuVal(idx, 'mem_percent')
      })), 0, 100, I18N.t("section.gpu_mem_pct"));
      const gpuMemSeries = [];
      gpuNames.forEach((nm, idx) => {
        gpuMemSeries.push({ key: `gpumu_${idx}`, label: `${nm} · ${I18N.t("section.gpu_mem_used")}`, color: gcolor(idx * 2), fmt: v => v.toFixed(1) + gbUnit, transform: gpuBytesGB(idx, 'mem_used') });
        gpuMemSeries.push({ key: `gpumf_${idx}`, label: `${nm} · ${I18N.t("section.gpu_mem_free")}`, color: gcolor(idx * 2 + 1), fmt: v => v.toFixed(1) + gbUnit, transform: gpuBytesGB(idx, 'mem_free') });
      });
      lazy('chartGPUMem', gpuMemSeries, null, null, I18N.t("section.gpu_vram"));
    }

    lazy('chartNet', [
      { key: 'net_recv_rate', label: I18N.t("section.net_recv"), color: '#2fd07a', fmt: fmtRate },
      { key: 'net_sent_rate', label: I18N.t("section.net_send"), color: '#43b6f0', fmt: fmtRate },
    ], null, null, I18N.t("section.net_throughput"));

    if (hasConns) {
      const sumProto = (s, proto) => Array.isArray(s.conns) ? s.conns.reduce((a, c) => c.proto === proto ? a + (c.count || 0) : a, 0) : null;
      lazy('chartConns', [
        { key: 'conn_tcp', label: 'TCP', color: '#43b6f0', fmt: v => v.toFixed(0), transform: (s) => sumProto(s, 'tcp') },
        { key: 'conn_udp', label: 'UDP', color: '#2fd07a', fmt: v => v.toFixed(0), transform: (s) => sumProto(s, 'udp') },
      ], null, null, I18N.t("section.conn_count"));
      const KEY_STATES = ['ESTABLISHED', 'TIME_WAIT', 'LISTEN', 'CLOSE_WAIT'];
      const stateSet = KEY_STATES.filter(st => samples.some(s => (s.conns || []).some(c => c.proto === 'tcp' && c.state === st)));
      const stateColors = { ESTABLISHED: '#4c8dff', TIME_WAIT: '#f7b23b', LISTEN: '#2fd07a', CLOSE_WAIT: '#f2545b' };
      const stateSeries = stateSet.map((st, idx) => ({
        key: `cst_${idx}`, label: st, color: stateColors[st] || '#8b5cf6', fmt: v => v.toFixed(0),
        transform: (s) => { if (!Array.isArray(s.conns)) return null; const c = s.conns.find(x => x.proto === 'tcp' && x.state === st); return c ? c.count : 0; }
      }));
      if (stateSeries.length) lazy('chartConnStates', stateSeries, null, null, I18N.t("section.conn_states"));
    }

    lazy('chartDiskIO', [
      { key: 'disk_read_rate', label: I18N.t("ui.disk_read"), color: '#2fd07a', fmt: fmtIORate },
      { key: 'disk_write_rate', label: I18N.t("ui.disk_write"), color: '#f7b23b', fmt: fmtIORate },
    ], null, null, I18N.t("ui.disk_io"));
    lazy('chartIOPS', [
      { key: 'disk_read_iops', label: I18N.t("ui.disk_read_iops"), color: '#2fd07a', fmt: fmtIOPS },
      { key: 'disk_write_iops', label: I18N.t("ui.disk_write_iops"), color: '#f7b23b', fmt: fmtIOPS },
    ], null, null, I18N.t("ui.disk_iops_title"));
    lazy('chartProc', [
      { key: 'proc_count', label: '进程数', color: '#8b5cf6', fmt: v => v.toFixed(0) },
    ], null, null, '进程数趋势');

    mountDetailLazyCharts(body);
  } catch (e) {
    body.innerHTML = `<div class="empty-line">加载失败: ${esc(e)}</div>`;
  }
}

let DETAIL_CHART_IO = null;
let DETAIL_CHART_PENDING = {};

/** 视口进入时才 createChart；首屏可见的图表立即绘制（无入场动画）。 */
function mountDetailLazyCharts(root) {
  const mountOne = (id) => {
    const spec = DETAIL_CHART_PENDING[id];
    if (!spec || DETAIL_CHARTS[id]) return;
    DETAIL_CHARTS[id] = createChart(id, spec.samples, spec.series, spec.yMin, spec.yMax, {
      title: spec.title, noEntrance: true
    });
    delete DETAIL_CHART_PENDING[id];
  };
  const wraps = root.querySelectorAll("[data-lazy-chart]");
  if (!("IntersectionObserver" in window)) {
    wraps.forEach(el => mountOne(el.dataset.lazyChart));
    return;
  }
  DETAIL_CHART_IO = new IntersectionObserver((entries) => {
    entries.forEach(en => {
      if (!en.isIntersecting) return;
      const id = en.target.dataset.lazyChart;
      mountOne(id);
      DETAIL_CHART_IO.unobserve(en.target);
    });
  }, { root: null, rootMargin: "120px 0px", threshold: 0.01 });
  wraps.forEach(el => {
    // 首屏已在视口内的直接绘制，其余交给观察器（滚动时再画）。
    const rect = el.getBoundingClientRect();
    if (rect.top < window.innerHeight + 80 && rect.bottom > -40) mountOne(el.dataset.lazyChart);
    else DETAIL_CHART_IO.observe(el);
  });
}

// 详情弹窗事件委托：放大按钮 + 时间范围切换
// 重复主机横幅的按钮（横幅是重渲染出来的，故走事件委托）。
// 清理后强制刷新主机列表：记录已被删掉，页面必须跟着更新。
dupBindPanel("hostDupBar", () => refresh());
// 首屏拉一次重复分组；有则在下一次渲染时显示横幅
loadDuplicates(() => {
  const bar = $("hostDupBar");
  if (bar) bar.innerHTML = dupBannerHTML();
});

safeAddEventListener("detailBody", "click", e => {
  const en = e.target.closest(".chart-enlarge");
  if (en) {
    const id = en.dataset.chart;
    // 懒加载尚未触发时，放大前先强制挂载该图。
    if (!DETAIL_CHARTS[id] && DETAIL_CHART_PENDING[id]) {
      const spec = DETAIL_CHART_PENDING[id];
      DETAIL_CHARTS[id] = createChart(id, spec.samples, spec.series, spec.yMin, spec.yMax, { title: spec.title, noEntrance: true });
      delete DETAIL_CHART_PENDING[id];
    }
    const ch = DETAIL_CHARTS[id];
    if (ch) openChartZoom(ch);
    return;
  }
  // AI 分析主机趋势
  if (e.target.closest("#detailAIBtn")) {
    analyzeHostDetailAI();
    return;
  }
  // 自定义时间范围：展开/收起面板
  const tog = e.target.closest("[data-custom-toggle]");
  if (tog) {
    const panel = $("detailCustomPanel");
    if (panel) { panel.hidden = !panel.hidden; if (!panel.hidden) { const f = $("detailCustomFrom"); if (f) f.focus(); } }
    return;
  }
  // 自定义时间范围：应用
  if (e.target.closest("[data-custom-apply]")) { applyDetailCustomRange(); return; }
  const btn = e.target.closest(".chip-btn[data-range]");
  if (!btn) return;
  DETAIL_CUSTOM = null; // 切回预设跨度（相对当前时间）
  DETAIL_TIME_RANGE = parseInt(btn.dataset.range);
  loadAndRenderCharts();
});

function analyzeHostDetailAI() {
  if (typeof openAIAssist !== "function") {
    if (typeof toast === "function") toast(I18N.t("assist.unavailable", "AI 面板未就绪"), "err");
    return;
  }
  const samples = DETAIL_SAMPLES || [];
  if (!samples.length) {
    if (typeof toast === "function") toast(I18N.t("empty.no_history", "暂无历史数据"), "err");
    return;
  }
  const first = samples[0], last = samples[samples.length - 1];
  const avg = (key) => {
    let s = 0, n = 0;
    samples.forEach(x => { if (typeof x[key] === "number") { s += x[key]; n++; } });
    return n ? (s / n) : 0;
  };
  const max = (key) => samples.reduce((m, x) => Math.max(m, typeof x[key] === "number" ? x[key] : 0), 0);
  const lines = [
    `主机：${DETAIL_HOST_NAME || DETAIL_HOST_ID}（id=${DETAIL_HOST_ID}）`,
    `样本数：${samples.length}，时间范围：约 ${((last.ts || last.timestamp || 0) - (first.ts || first.timestamp || 0)) / 3600} 小时`,
    `CPU：均值 ${avg("cpu_percent").toFixed(1)}% · 峰值 ${max("cpu_percent").toFixed(1)}% · 当前 ${(last.cpu_percent || 0).toFixed(1)}%`,
    `内存：均值 ${avg("mem_percent").toFixed(1)}% · 峰值 ${max("mem_percent").toFixed(1)}% · 当前 ${(last.mem_percent || 0).toFixed(1)}%`,
    `磁盘：均值 ${avg("disk_percent").toFixed(1)}% · 峰值 ${max("disk_percent").toFixed(1)}% · 当前 ${(last.disk_percent || 0).toFixed(1)}%`,
    `负载：当前 load1=${(last.load1 || 0).toFixed(2)} load5=${(last.load5 || 0).toFixed(2)} load15=${(last.load15 || 0).toFixed(2)}`,
  ];
  if (Array.isArray(last.gpus) && last.gpus.length) {
    lines.push("GPU：" + last.gpus.map(g => `${g.name || "GPU"} util=${(g.util_percent || 0).toFixed(0)}% mem=${(g.mem_percent || 0).toFixed(0)}%`).join("；"));
  }
  openAIAssist({
    task: "chart_analysis",
    title: "🤖 AI 主机分析 · " + (DETAIL_HOST_NAME || DETAIL_HOST_ID),
    mode: "analyze",
    context: lines.join("\n"),
    hint: I18N.t("hosts.ai_analyzing", "AI 正在解读主机近期指标…"),
  });
}

// 读取两个 datetime-local 输入，校验后按自定义绝对时间范围重新拉取并渲染
function applyDetailCustomRange() {
  const fEl = $("detailCustomFrom"), tEl = $("detailCustomTo");
  if (!fEl || !tEl || !fEl.value || !tEl.value) { toast(I18N.t("time.custom_incomplete") || "请选择开始和结束时间", "warn"); return; }
  const from = Math.floor(new Date(fEl.value).getTime() / 1000);
  const to = Math.floor(new Date(tEl.value).getTime() / 1000);
  if (!Number.isFinite(from) || !Number.isFinite(to)) { toast(I18N.t("time.custom_invalid") || "时间格式无效", "err"); return; }
  if (to <= from) { toast(I18N.t("time.custom_order") || "结束时间必须晚于开始时间", "warn"); return; }
  if (to - from < 60) { toast(I18N.t("time.custom_tooshort") || "时间范围太短（至少 1 分钟）", "warn"); return; }
  DETAIL_CUSTOM = { from, to };
  loadAndRenderCharts();
}

/* ---------- Canvas 折线图（交互：悬停十字线 + 数值气泡 / 框选放大 / 双击还原 / 点击放大预览） ---------- */
let DETAIL_CHARTS = {};

function chartTipEl() {
  let t = $("chartTip");
  if (!t) { t = document.createElement("div"); t.id = "chartTip"; t.className = "chart-tip"; document.body.appendChild(t); }
  return t;
}
function hideChartTip() { const t = $("chartTip"); if (t) t.style.display = "none"; }

function seriesVal(s, sample) {
  const v = s.transform ? s.transform(sample) : sample[s.key];
  return (v === null || v === undefined || isNaN(v)) ? null : v;
}

// smoothPath — 将折线数据点绘制为平滑的二次贝塞尔曲线
function smoothPath(ctx, pts) {
  if (pts.length < 2) return;
  ctx.beginPath();
  ctx.moveTo(pts[0].x, pts[0].y);
  for (let i = 1; i < pts.length - 1; i++) {
    const cx = (pts[i].x + pts[i + 1].x) / 2;
    const cy = (pts[i].y + pts[i + 1].y) / 2;
    ctx.quadraticCurveTo(pts[i].x, pts[i].y, cx, cy);
  }
  ctx.lineTo(pts[pts.length - 1].x, pts[pts.length - 1].y);
}

// drawChartEmpty — 在 Canvas 上绘制空状态插画
function drawChartEmpty(ctx, w, h, message) {
  ctx.clearRect(0, 0, w, h);
  const cssVar = name => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  const txtColor = cssVar("--muted") || "#8a95a8";
  const lineColor = cssVar("--line2") || "#2c3442";
  const cx = w / 2, cy = h / 2;

  // 淡色折线图标轮廓
  ctx.strokeStyle = lineColor; ctx.lineWidth = 1.2; ctx.setLineDash([3, 4]); ctx.lineCap = "round";
  const iconPts = [{x: cx - 50, y: cy + 10}, {x: cx - 18, y: cy - 14}, {x: cx + 14, y: cy + 6}, {x: cx + 46, y: cy - 20}];
  ctx.beginPath(); ctx.moveTo(iconPts[0].x, iconPts[0].y);
  for (let i = 1; i < iconPts.length; i++) ctx.lineTo(iconPts[i].x, iconPts[i].y);
  ctx.stroke(); ctx.setLineDash([]);

  // 数据点
  iconPts.forEach(p => { ctx.fillStyle = lineColor; ctx.beginPath(); ctx.arc(p.x, p.y, 2.5, 0, Math.PI * 2); ctx.fill(); });

  // 居中提示文字
  ctx.fillStyle = txtColor; ctx.font = "13px -apple-system, 'Segoe UI', 'PingFang SC', sans-serif"; ctx.textAlign = "center";
  ctx.fillText(message, cx, cy + 40);
}

// createChart builds an interactive line chart on a canvas and returns its
// state. The state (samples/series/visible-window) lives on canvas._chart so a
// single set of event listeners always drives the current chart.
// sizeChartCanvas makes a canvas crisp on HiDPI screens: the pixel buffer is
// scaled by devicePixelRatio while all chart code keeps working in CSS pixels.
// cssH fixes the display height so a chart looks right at any column width
// (full-width or the two-up grid). Returns the logical {W,H,dpr} to draw within.
function sizeChartCanvas(canvas, cssH) {
  const dpr = Math.min(window.devicePixelRatio || 1, 2); // cap at 2 to bound memory
  const cssW = Math.round(canvas.getBoundingClientRect().width) || 1000;
  canvas.style.height = cssH + "px";
  canvas.width = Math.max(1, Math.round(cssW * dpr));
  canvas.height = Math.max(1, Math.round(cssH * dpr));
  canvas.getContext("2d").setTransform(dpr, 0, 0, dpr, 0, 0);
  return { W: cssW, H: cssH, dpr };
}

// resizeAllCharts re-fits every live chart to its current column width (buffers
// are pinned at creation for HiDPI crispness, so a viewport resize needs a refit).
function resizeAllCharts() {
  const states = [];
  for (const k in DETAIL_CHARTS) if (DETAIL_CHARTS[k]) states.push(DETAIL_CHARTS[k]);
  for (const k in (typeof CHK_CHARTS !== "undefined" ? CHK_CHARTS : {})) if (CHK_CHARTS[k]) states.push(CHK_CHARTS[k]);
  states.forEach(st => {
    if (!st.canvas || !st.canvas.isConnected) return;
    const d = sizeChartCanvas(st.canvas, st.cssH || 210);
    st.W = d.W; st.H = d.H; st.dpr = d.dpr;
    drawChart(st);
  });
}
let _chartResizeTimer = null;
window.addEventListener("resize", () => {
  clearTimeout(_chartResizeTimer);
  _chartResizeTimer = setTimeout(resizeAllCharts, 150);
});

function createChart(canvasId, allSamples, series, yMin = null, yMax = null, opts = {}) {
  const canvas = $(canvasId);
  if (!canvas) return null;
  const cssH = opts.cssH || (opts.isZoom ? 440 : 210);
  const dim = sizeChartCanvas(canvas, cssH);
  if (!allSamples || !allSamples.length) {
    drawChartEmpty(canvas.getContext("2d"), dim.W, dim.H, I18N.t("empty.no_trend_data") || "暂无趋势数据");
    return null;
  }
  const state = {
    canvas, ctx: canvas.getContext("2d"),
    W: dim.W, H: dim.H, dpr: dim.dpr, cssH,
    all: allSamples, series, yMin, yMax,
    title: opts.title || "", isZoom: !!opts.isZoom,
    legendMode: opts.legendMode || "full", // full=主机详情；dash=看板精简图例
    i0: 0, i1: allSamples.length - 1,
    hover: -1, drag: false, downX: null, curX: null, moved: false,
    pad: { top: 22, right: 18, bottom: 28, left: 56 },
  };
  canvas._chart = state;

  // 默认直接绘制；详情弹窗等场景传 noEntrance 跳过入场动画，避免一次打开十多张图连环重绘卡顿。
  drawChart(state);
  if (!opts.noEntrance) {
    state._entranceStart = performance.now();
    state._entranceDur = 400;
    requestAnimationFrame(function entranceStep(now) {
      state._entranceP = Math.min(1, (now - state._entranceStart) / state._entranceDur);
      drawChart(state);
      if (state._entranceP < 1) requestAnimationFrame(entranceStep);
    });
  }

  attachChartEvents(canvas);
  return state;
}

function drawChart(state) {
  const { ctx, canvas, series, pad } = state;
  // Draw in CSS pixels; the buffer is dpr-scaled so lines/text are crisp on HiDPI.
  ctx.setTransform(state.dpr || 1, 0, 0, state.dpr || 1, 0, 0);
  const w = state.W || canvas.width, h = state.H || canvas.height;
  const vis = state.all.slice(state.i0, state.i1 + 1);
  const n = vis.length;
  ctx.clearRect(0, 0, w, h);

  // 使用 CSS 变量适配深色/浅色主题
  const cssVar = name => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  const gridColor = cssVar("--line2") || "rgba(43,53,71,.5)";
  const labelColor = cssVar("--muted") || "#8a95a8";
  const txtColor = cssVar("--txt") || "#e8eef6";
  const bgColor = cssVar("--bg") || "#0a0d13";

  // Y range (fixed when yMin/yMax given, else padded auto-range)
  let dMin = state.yMin !== null ? state.yMin : Infinity;
  let dMax = state.yMax !== null ? state.yMax : -Infinity;
  series.forEach(s => vis.forEach(sm => {
    const v = seriesVal(s, sm);
    if (v !== null) { dMin = Math.min(dMin, v); dMax = Math.max(dMax, v); }
  }));
  if (dMin === Infinity) dMin = 0;
  if (dMax === -Infinity) dMax = state.yMax !== null ? state.yMax : 100;
  // 自动范围：对 auto-range 做 8% padding（比原来的 10% 更紧凑）
  if (state.yMin === null) dMin = Math.max(0, dMin * 0.92);
  if (state.yMax === null) dMax = dMax * 1.08 || 1;
  if (dMax <= dMin) dMax = dMin + 1;
  const yRange = dMax - dMin;
  // Dynamic left padding: widen it to fit the Y-axis labels so long values
  // (network rates like "1.45 MB/s", disk IO/GB) are never clipped off the canvas
  // edge — the fixed 56px was too narrow for rate charts.
  ctx.font = "10.5px 'SF Mono', 'Cascadia Code', 'JetBrains Mono', Consolas, monospace";
  let maxLabelW = 0;
  for (let i = 0; i <= 4; i++) {
    const val = dMax - (yRange / 4) * i;
    const lab = series[0].fmt ? series[0].fmt(val) : val.toFixed(1);
    maxLabelW = Math.max(maxLabelW, ctx.measureText(lab).width);
  }
  pad.left = Math.max(56, Math.ceil(maxLabelW) + 14);
  const cw = w - pad.left - pad.right, ch = h - pad.top - pad.bottom;
  state.dataMin = dMin; state.dataMax = dMax; state._cw = cw; state._ch = ch; state._n = n;

  const xAt = i => pad.left + (n <= 1 ? 0 : (i / (n - 1)) * cw);
  const yAt = v => pad.top + ch - ((v - dMin) / yRange) * ch;

  // 网格 + Y 轴标签（5 条水平线，虚线样式）
  ctx.strokeStyle = gridColor; ctx.lineWidth = 0.5; ctx.setLineDash([2, 4]);
  ctx.font = "10.5px 'SF Mono', 'Cascadia Code', 'JetBrains Mono', Consolas, monospace"; ctx.textAlign = "right";
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (ch / 4) * i;
    ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(w - pad.right, y); ctx.stroke();
    const val = dMax - (yRange / 4) * i;
    ctx.fillStyle = labelColor;
    // 使用第一个 series 的 fmt 格式化 Y 轴标签，确保网络图正确显示速率单位
    const fmt = series[0].fmt;
    const label = fmt ? fmt(val) : val.toFixed(1);
    ctx.fillText(label, pad.left - 8, y + 4);
  }
  ctx.setLineDash([]);

  // 图表标题（左上角）：各图无独立标题元素，靠此标明本图指标——尤其 GPU 算力/温度/显存、
  // TCP/UDP 连接等同为「一堆同色系折线」的图，没有标题就分不清是什么指标。
  if (state.title) {
    ctx.textAlign = "left";
    ctx.fillStyle = txtColor;
    ctx.font = "600 11.5px -apple-system, 'Segoe UI', 'PingFang SC', sans-serif";
    ctx.fillText(state.title, pad.left, 14);
  }

  // X 轴时间标签
  if (n >= 1) {
    const firstTs = vis[0].timestamp, span = vis[n - 1].timestamp - firstTs;
    ctx.textAlign = "center"; ctx.fillStyle = labelColor; ctx.font = "10.5px 'SF Mono', 'Cascadia Code', 'JetBrains Mono', Consolas, monospace";
    for (let i = 0; i <= 4; i++) {
      const x = pad.left + (cw / 4) * i;
      const d = new Date((firstTs + (span / 4) * i) * 1000);
      const lab = span > 172800
        ? `${d.getMonth() + 1}/${d.getDate()}`
        : `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
      ctx.fillText(lab, x, h - 8);
    }
  }

  // 系列折线 + 渐变填充区域
  series.forEach((s, sIdx) => {
    const pts = [];
    vis.forEach((sm, i) => { const v = seriesVal(s, sm); if (v !== null) pts.push({ x: xAt(i), y: yAt(v), val: v }); });
    if (pts.length >= 2) {
      // 折线路径（数据点 > 12 时使用平滑贝塞尔曲线）
      ctx.save();
      ctx.strokeStyle = s.color; ctx.lineWidth = sIdx === 0 ? 2.2 : 1.8; ctx.lineJoin = "round"; ctx.lineCap = "round";
      if (pts.length > 12) { smoothPath(ctx, pts); } else { ctx.beginPath(); pts.forEach((p, i) => i ? ctx.lineTo(p.x, p.y) : ctx.moveTo(p.x, p.y)); }
      ctx.stroke();
      ctx.restore();

      // 半透明渐变填充区域（4 层渐变停止点，层次更丰富）
      const grad = ctx.createLinearGradient(0, pad.top, 0, pad.top + ch);
      grad.addColorStop(0, s.color + "35");
      grad.addColorStop(0.4, s.color + "15");
      grad.addColorStop(0.7, s.color + "06");
      grad.addColorStop(1, s.color + "01");
      ctx.fillStyle = grad;
      ctx.beginPath(); ctx.moveTo(pts[0].x, pad.top + ch);
      pts.forEach(p => ctx.lineTo(p.x, p.y));
      ctx.lineTo(pts[pts.length - 1].x, pad.top + ch); ctx.closePath(); ctx.fill();
    }
  });

  // 图例：主机详情用「名称 + 当前/峰值」；看板用短名单行，避免多序列把曲线区挤没。
  const dashLegend = state.legendMode === "dash";
  const maxLegendItems = dashLegend ? 8 : series.length;
  const legendY = pad.top + 2;
  let legendX = pad.left + 8;
  const legendItemWidth = dashLegend ? 96 : 160;

  let legendBgW = 0, legendBgX0 = legendX;
  const legendLines = [];
  let curLine = { x: legendX, items: [] };
  const truncLeg = (s, n) => {
    s = String(s || "");
    if (s.length <= n) return s;
    return s.slice(0, Math.max(1, n - 1)) + "…";
  };
  series.forEach((s, sIdx) => {
    if (sIdx >= maxLegendItems) return;
    const pts = [];
    vis.forEach((sm, i) => { const v = seriesVal(s, sm); if (v !== null) pts.push({ x: xAt(i), y: yAt(v), val: v }); });
    const vals = pts.map(p => p.val);
    const cur = vals.length ? vals[vals.length - 1] : 0, peak = vals.length ? Math.max(...vals) : 0;
    const fmtV = v => s.fmt ? s.fmt(v) : v.toFixed(1);
    let labelText;
    if (dashLegend) {
      labelText = truncLeg(s.label || ("#" + (sIdx + 1)), 18);
    } else {
      labelText = `${s.label}  当前 ${fmtV(cur)} · 峰值 ${fmtV(peak)}`;
    }

    // 看板：只排一行，放不下就停（后面用 +N）
    if (dashLegend && curLine.x + legendItemWidth > w - pad.right && curLine.items.length) {
      return;
    }
    if (!dashLegend && curLine.x + legendItemWidth > w - pad.right && sIdx > 0) {
      legendLines.push(curLine);
      curLine = { x: pad.left + 8, items: [] };
    }
    curLine.items.push({ color: s.color, labelText, x: curLine.x });
    ctx.font = "10.5px -apple-system, 'Segoe UI', 'PingFang SC', sans-serif";
    curLine.x += ctx.measureText(labelText).width + 28;
    if (!dashLegend && curLine.x + legendItemWidth > w - pad.right) {
      legendLines.push(curLine);
      curLine = { x: pad.left + 8, items: [] };
    }
  });
  if (dashLegend && series.length > curLine.items.length) {
    const more = `+${series.length - curLine.items.length}`;
    curLine.items.push({ color: labelColor, labelText: more, x: curLine.x });
    curLine.x += ctx.measureText(more).width + 20;
  }
  if (curLine.items.length) legendLines.push(curLine);

  legendLines.forEach(line => {
    legendBgW = Math.max(legendBgW, line.x - legendBgX0);
  });

  if (legendLines.length) {
    const bgH = legendLines.length * (dashLegend ? 15 : 18) + (dashLegend ? 2 : 8);
    ctx.fillStyle = cssVar("--panel") + "99" || "rgba(17,22,33,.6)";
    const bgR = 6;
    ctx.beginPath(); ctx.roundRect(legendBgX0 - 4, legendY - 2, Math.min(legendBgW + 20, cw + 8), bgH, bgR); ctx.fill();
  }

  let ly = legendY;
  legendLines.forEach(line => {
    let lx = line.items.length ? line.items[0].x : legendBgX0;
    line.items.forEach(item => {
      lx = item.x;
      ctx.fillStyle = item.color;
      ctx.beginPath(); ctx.roundRect(lx, ly, 10, 10, 3); ctx.fill();
      ctx.fillStyle = txtColor; ctx.font = "10.5px -apple-system, 'Segoe UI', 'PingFang SC', sans-serif"; ctx.textAlign = "left";
      ctx.fillText(item.labelText, lx + 14, ly + 9);
    });
    ly += dashLegend ? 15 : 18;
  });

  // 框选矩形
  if (state.drag && state.moved && state.downX !== null && state.curX !== null) {
    const x0 = Math.min(state.downX, state.curX), x1 = Math.max(state.downX, state.curX);
    ctx.fillStyle = "rgba(76,141,255,.12)"; ctx.fillRect(x0, pad.top, x1 - x0, ch);
    ctx.strokeStyle = "rgba(76,141,255,.5)"; ctx.lineWidth = 1; ctx.setLineDash([4, 4]); ctx.strokeRect(x0, pad.top, x1 - x0, ch); ctx.setLineDash([]);
  }

  // 十字线（更细、更淡，不干扰数据观察）
  if (state.hover >= state.i0 && state.hover <= state.i1 && !state.drag) {
    const li = state.hover - state.i0, x = xAt(li);
    ctx.strokeStyle = "rgba(200,210,230,.22)"; ctx.lineWidth = 0.8;
    ctx.setLineDash([3, 5]); ctx.beginPath(); ctx.moveTo(x, pad.top); ctx.lineTo(x, pad.top + ch); ctx.stroke(); ctx.setLineDash([]);
    // 悬停数据点（双层光晕 + 白色高光边缘）
    series.forEach(s => {
      const v = seriesVal(s, vis[li]); if (v === null) return;
      const py = yAt(v);
      // 外层光晕（增大半径至 8px）
      ctx.fillStyle = s.color + "25"; ctx.beginPath(); ctx.arc(x, py, 8, 0, Math.PI * 2); ctx.fill();
      // 内层光点
      ctx.fillStyle = s.color; ctx.beginPath(); ctx.arc(x, py, 3.5, 0, Math.PI * 2); ctx.fill();
      // 白色高光边缘
      ctx.strokeStyle = "#fff"; ctx.lineWidth = 1.5;
      ctx.beginPath(); ctx.arc(x, py, 3.5, 0, Math.PI * 2); ctx.stroke();
    });
  }
}

// attachChartEvents wires pointer interaction once per canvas element; handlers
// read the live state from canvas._chart so a persistent canvas (the zoom modal)
// never accumulates duplicate listeners.
function attachChartEvents(canvas) {
  if (canvas._evt) return;
  canvas._evt = true;
  // Map a pointer's clientX into the chart's CSS-pixel coordinate space (state.W),
  // which is what drawChart / pad.left / _cw work in. Using canvas.width (the
  // dpr-scaled backing buffer) here caused the crosshair to be offset by the
  // devicePixelRatio on HiDPI / zoomed displays — hovering snapped to the wrong point.
  const toX = e => {
    const st = canvas._chart;
    const r = canvas.getBoundingClientRect();
    if (!r.width) return 0;
    const W = (st && st.W) || r.width; // CSS-pixel width the chart was drawn with
    return (e.clientX - r.left) * (W / r.width);
  };
  const localIdx = (st, x) => {
    const n = st._n; if (n <= 1) return 0;
    return Math.max(0, Math.min(n - 1, Math.round((x - st.pad.left) / st._cw * (n - 1))));
  };
  canvas.addEventListener("mousemove", e => {
    const st = canvas._chart; if (!st) return;
    const x = toX(e);
    if (st.drag) { st.curX = x; if (Math.abs(x - st.downX) > 4) st.moved = true; }
    const li = localIdx(st, x); st.hover = st.i0 + li;
    drawChart(st); showChartTip(st, e, li);
  });
  canvas.addEventListener("mousedown", e => { const st = canvas._chart; if (!st) return; st.drag = true; st.downX = toX(e); st.curX = st.downX; st.moved = false; });
  canvas.addEventListener("mouseup", e => {
    const st = canvas._chart; if (!st) return;
    if (st.drag && st.moved) {
      const a = localIdx(st, st.downX), b = localIdx(st, toX(e));
      const lo = Math.min(a, b), hi = Math.max(a, b);
      if (hi - lo >= 1) { const base = st.i0; st.i1 = base + hi; st.i0 = base + lo; }
    } else if (st.drag && !st.moved && !st.isZoom) { openChartZoom(st); }
    st.drag = false; st.downX = st.curX = null; st.moved = false; drawChart(st);
  });
  canvas.addEventListener("mouseleave", () => { const st = canvas._chart; if (!st) return; st.hover = -1; st.drag = false; st.moved = false; hideChartTip(); drawChart(st); });
  canvas.addEventListener("dblclick", () => { const st = canvas._chart; if (!st) return; st.i0 = 0; st.i1 = st.all.length - 1; st.hover = -1; hideChartTip(); drawChart(st); });
}

function showChartTip(state, e, li) {
  const vis = state.all.slice(state.i0, state.i1 + 1);
  const sm = vis[li]; if (!sm) { hideChartTip(); return; }
  const d = new Date(sm.timestamp * 1000);
  const time = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")} ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
  let rows = "";
  state.series.forEach(s => {
    const v = seriesVal(s, sm);
    const txt = v === null ? "—" : (s.fmt ? s.fmt(v) : v.toFixed(1));
    rows += `<div class="tip-r"><span class="tip-dot" style="background:${s.color}"></span><span>${esc(s.label)}</span><span class="tip-v">${esc(txt)}</span></div>`;
  });
  const t = chartTipEl();
  t.innerHTML = `<div class="tip-t">${time}</div>${rows}`;
  t.style.display = "block";
  let px = e.clientX + 14, py = e.clientY + 14;
  if (px + t.offsetWidth > window.innerWidth - 8) px = e.clientX - t.offsetWidth - 14;
  if (py + t.offsetHeight > window.innerHeight - 8) py = e.clientY - t.offsetHeight - 14;
  t.style.left = px + "px"; t.style.top = py + "px";
}

// openChartZoom opens the enlarge modal, re-rendering the source chart on a
// larger canvas that keeps the source's current visible window and stays fully
// interactive (hover / box-zoom / dbl-click reset).
function openChartZoom(src) {
  hideChartTip();
  $("chartZoomTitle").textContent = (src.title || I18N.t("ui.trend")) + " · " + I18N.t("ui.zoom_preview");
  $("chartZoomMask").classList.add("show");
  const z = createChart("chartZoomCanvas", src.all, src.series, src.yMin, src.yMax, { title: src.title, isZoom: true });
  if (z) { z.i0 = src.i0; z.i1 = src.i1; drawChart(z); }
  DETAIL_CHARTS.__zoom = z;
}
function sparkBlock(title, series, color) {
  const last = series.length ? series[series.length - 1] : 0;
  return `<div class="field"><label>${title} · 当前 ${(last || 0).toFixed(1)}</label>
    <div class="spark">${sparkline(series, color)}</div></div>`;
}
function sparkline(series, color) {
  const w = 500, h = 46, n = series.length, max = 100;
  if (n < 2) return `<svg class="sparkline" viewBox="0 0 ${w} ${h}"></svg>`;
  const pts = series.map((v, i) => {
    const x = i / (n - 1) * w;
    const y = h - 2 - (Math.max(0, Math.min(v || 0, max)) / max) * (h - 4);
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  }).join(" ");
  const gid = "g" + Math.random().toString(36).slice(2, 7);
  return `<svg class="sparkline" viewBox="0 0 ${w} ${h}" preserveAspectRatio="none">
    <defs><linearGradient id="${gid}" x1="0" x2="0" y1="0" y2="1">
      <stop offset="0" stop-color="${color}" stop-opacity=".35"/><stop offset="1" stop-color="${color}" stop-opacity="0"/>
    </linearGradient></defs>
    <polygon points="0,${h} ${pts} ${w},${h}" fill="url(#${gid})"/>
    <polyline points="${pts}" fill="none" stroke="${color}" stroke-width="1.6"/></svg>`;
}
