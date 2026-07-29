/* ---------- Shared host tree picker (folder tree · hostname + IP) ---------- */
/* Depends on: esc, I18N, HOST_FOLDERS (from hosts.js), optional loadHostFolders */

(function (global) {
  "use strict";

  function t(k, fb) { return (typeof I18N !== "undefined" && I18N.t) ? I18N.t(k, fb) : fb; }
  function e(s) { return typeof esc === "function" ? esc(String(s ?? "")) : String(s ?? ""); }

  function hostIP(h) {
    if (!h) return "";
    return String(h.ip || h.agent_ip || h.primary_ip || "").trim();
  }
  function hostTitle(h) {
    const name = (h && (h.hostname || h.id)) || "";
    const ip = hostIP(h);
    return ip ? `${name} (${ip})` : name;
  }
  function hostOnline(h) {
    if (!h) return false;
    if (typeof h.online === "boolean") return h.online;
    return true;
  }

  function hostsByFolder(hosts) {
    const map = new Map();
    (hosts || []).forEach(h => {
      const fid = h.folder_id || "__ungrouped__";
      if (!map.has(fid)) map.set(fid, []);
      map.get(fid).push(h);
    });
    map.forEach(list => list.sort((a, b) => {
      const an = (a.hostname || a.id || "").toLowerCase();
      const bn = (b.hostname || b.id || "").toLowerCase();
      if (an !== bn) return an < bn ? -1 : 1;
      return hostIP(a).localeCompare(hostIP(b));
    }));
    return map;
  }

  function filterHost(h, q) {
    if (!q) return true;
    const hay = [h.id, h.hostname, hostIP(h), h.os, h.platform, h.category, h.folder_path]
      .filter(Boolean).join(" ").toLowerCase();
    return hay.includes(q);
  }

  function folderTree() {
    return (typeof HOST_FOLDERS !== "undefined" && HOST_FOLDERS && Array.isArray(HOST_FOLDERS.folders))
      ? HOST_FOLDERS.folders : [];
  }

  function collectFolderHostIds(node, byFolder, q, onlineOnly) {
    const ids = [];
    const walk = (n) => {
      (byFolder.get(n.id) || []).forEach(h => {
        if (!filterHost(h, q)) return;
        if (onlineOnly && !hostOnline(h)) return;
        if (h && h.id) ids.push(h.id);
      });
      (n.children || []).forEach(walk);
    };
    walk(node);
    return ids;
  }

  function folderNodeHTML(node, ctx, depth) {
    const byFolder = ctx.byFolder;
    const q = ctx.q;
    const selected = ctx.selected;
    const collapsed = ctx.collapsed;
    const mode = ctx.mode; // multi | single | target
    const kids = node.children || [];
    const own = (byFolder.get(node.id) || []).filter(h => filterHost(h, q));
    const childHTML = kids.map(c => folderNodeHTML(c, ctx, depth + 1)).join("");
    const hasHosts = own.length > 0 || childHTML;
    if (q && !hasHosts && !(node.name || "").toLowerCase().includes(q)) return "";
    const isCollapsed = collapsed.has(node.id);
    const hostIds = collectFolderHostIds(node, byFolder, q, mode === "multi" ? (ctx.onlineOnly !== false) : false);
    const pad = Math.min(depth, 6) * 14;
    let folderCtrl = "";
    if (mode === "multi") {
      const checkedN = hostIds.filter(id => selected.has(id)).length;
      const folderState = !hostIds.length ? "" : (checkedN === hostIds.length ? "checked" : (checkedN > 0 ? "data-indeterminate=\"1\"" : ""));
      folderCtrl = `<label class="hs-pick-folder-lab">
        <input type="checkbox" class="hs-pick-folder-cb" data-hp-folder="${e(node.id)}" ${folderState}>
        <span class="hs-pick-folder-name">${e(node.name || node.id)}</span>
        <span class="hs-pick-count">${own.length || hostIds.length}</span>
      </label>`;
    } else {
      // target / single: folder itself is a selectable target (folder:id)
      const val = `folder:${node.id}`;
      const on = ctx.targetValue === val || (mode === "single" && selected.has(node.id));
      folderCtrl = `<label class="hs-pick-folder-lab hs-pick-folder-target${on ? " is-on" : ""}">
        <input type="radio" class="hs-pick-folder-radio" name="${e(ctx.name)}" value="${e(val)}" ${on ? "checked" : ""}>
        <span class="hs-pick-folder-name">${e(node.name || node.id)}</span>
        <span class="hs-pick-count">${own.length || hostIds.length}</span>
      </label>`;
    }
    let html = `<div class="hs-pick-folder" style="padding-left:${pad}px">
      <button type="button" class="hs-pick-caret" data-hp-fold="${e(node.id)}" aria-expanded="${isCollapsed ? "false" : "true"}">${isCollapsed ? "▸" : "▾"}</button>
      ${folderCtrl}
    </div>`;
    if (!isCollapsed) {
      html += own.map(h => hostRowHTML(h, ctx, depth + 1)).join("");
      html += childHTML;
    }
    return html;
  }

  function hostRowHTML(h, ctx, depth) {
    const online = hostOnline(h);
    const ip = hostIP(h);
    const pad = Math.min(depth, 6) * 14;
    const mode = ctx.mode;
    if (mode === "multi") {
      const disabled = ctx.onlineOnly !== false && !online;
      return `<label class="hs-pick-row${disabled ? " off" : ""}${ctx.selected.has(h.id) && !disabled ? " is-on" : ""}" style="padding-left:${pad + 22}px" title="${e(hostTitle(h))}">
        <input type="checkbox" class="hs-pick-host" value="${e(h.id)}" ${disabled ? "disabled" : ""} ${ctx.selected.has(h.id) ? "checked" : ""}>
        <span class="hs-pick-name">${e(h.hostname || h.id)}</span>
        <span class="hs-pick-ip mono">${e(ip || "—")}</span>
        <span class="hs-pick-st ${online ? "ok" : ""}"><i class="hs-pick-dot" aria-hidden="true"></i>${online ? e(t("hs.online", "在线")) : e(t("hs.offline", "离线"))}</span>
      </label>`;
    }
    const val = `host:${h.id}`;
    const on = ctx.targetValue === val || (mode === "single" && ctx.selected.has(h.id));
    const disabled = ctx.onlineOnly === true && !online;
    return `<label class="hs-pick-row${disabled ? " off" : ""}${on && !disabled ? " is-on" : ""}" style="padding-left:${pad + 22}px" title="${e(hostTitle(h))}">
      <input type="radio" class="hs-pick-host-radio" name="${e(ctx.name)}" value="${e(mode === "target" ? val : h.id)}" ${disabled ? "disabled" : ""} ${on ? "checked" : ""}>
      <span class="hs-pick-name">${e(h.hostname || h.id)}</span>
      <span class="hs-pick-ip mono">${e(ip || "—")}</span>
      <span class="hs-pick-st ${online ? "ok" : ""}"><i class="hs-pick-dot" aria-hidden="true"></i>${online ? e(t("hs.online", "在线")) : e(t("hs.offline", "离线"))}</span>
    </label>`;
  }

  function categoryFallbackHTML(hosts, ctx) {
    const byCat = new Map();
    hosts.forEach(h => {
      const c = (h.category || "").trim() || t("hs.ungrouped", "未分组");
      if (!byCat.has(c)) byCat.set(c, []);
      byCat.get(c).push(h);
    });
    let body = "";
    [...byCat.keys()].sort().forEach(cat => {
      const list = byCat.get(cat);
      const fid = "cat:" + cat;
      const isCollapsed = ctx.collapsed.has(fid);
      const pad = 0;
      if (ctx.mode === "multi") {
        const ids = list.filter(h => hostOnline(h) || ctx.onlineOnly === false).map(h => h.id);
        const checkedN = ids.filter(id => ctx.selected.has(id)).length;
        const folderState = !ids.length ? "" : (checkedN === ids.length ? "checked" : (checkedN > 0 ? "data-indeterminate=\"1\"" : ""));
        body += `<div class="hs-pick-folder" style="padding-left:${pad}px">
          <button type="button" class="hs-pick-caret" data-hp-fold="${e(fid)}" aria-expanded="${isCollapsed ? "false" : "true"}">${isCollapsed ? "▸" : "▾"}</button>
          <label class="hs-pick-folder-lab">
            <input type="checkbox" class="hs-pick-folder-cb" data-hp-folder="${e(fid)}" data-hp-cat="${e(cat)}" ${folderState}>
            <span class="hs-pick-folder-name">${e(cat)}</span>
            <span class="hs-pick-count">${list.length}</span>
          </label>
        </div>`;
      } else {
        const val = `category:${cat}`;
        const on = ctx.targetValue === val;
        body += `<div class="hs-pick-folder">
          <button type="button" class="hs-pick-caret" data-hp-fold="${e(fid)}" aria-expanded="${isCollapsed ? "false" : "true"}">${isCollapsed ? "▸" : "▾"}</button>
          <label class="hs-pick-folder-lab hs-pick-folder-target${on ? " is-on" : ""}">
            <input type="radio" class="hs-pick-folder-radio" name="${e(ctx.name)}" value="${e(val)}" ${on ? "checked" : ""}>
            <span class="hs-pick-folder-name">${e(cat)}</span>
            <span class="hs-pick-count">${list.length}</span>
          </label>
        </div>`;
      }
      if (!isCollapsed) body += list.map(h => hostRowHTML(h, ctx, 1)).join("");
    });
    return body;
  }

  /**
   * opts: {
   *   id, name, mode: 'multi'|'target'|'single',
   *   hosts, selected: Set|string[], targetValue,
   *   collapsed: Set, q, onlineOnly, showAllOption, systemOptions: [{val,label}],
   *   compact
   * }
   */
  function renderHTML(opts) {
    opts = opts || {};
    const mode = opts.mode || "multi";
    const hosts = opts.hosts || [];
    const selected = opts.selected instanceof Set ? opts.selected : new Set(opts.selected || []);
    const collapsed = opts.collapsed instanceof Set ? opts.collapsed : new Set(opts.collapsed || []);
    const q = (opts.q || "").trim().toLowerCase();
    const treeId = opts.id || ("hpTree_" + Math.random().toString(36).slice(2, 8));
    const searchId = treeId + "_q";
    const name = opts.name || treeId;
    const folders = folderTree();
    const byFolder = hostsByFolder(hosts);
    const filtered = q ? hosts.filter(h => filterHost(h, q)) : hosts;
    const onlineN = filtered.filter(hostOnline).length;
    const selN = mode === "multi"
      ? [...selected].filter(id => filtered.some(h => h.id === id)).length
      : (opts.targetValue && opts.targetValue !== "all" ? 1 : 0);

    const ctx = {
      mode, byFolder, q, selected, collapsed, name,
      targetValue: opts.targetValue || "",
      onlineOnly: opts.onlineOnly,
    };

    let body = "";
    if (!hosts.length) {
      body = `<div class="hs-pick-empty">${e(t("hs.no_hosts", "暂无主机"))}</div>`;
    } else if (!filtered.length) {
      body = `<div class="hs-pick-empty">${e(t("hs.no_host_match", "无匹配主机"))}</div>`;
    } else if (folders.length) {
      body = folders.map(n => folderNodeHTML(n, ctx, 0)).join("");
      const ug = (byFolder.get("__ungrouped__") || []).filter(h => filterHost(h, q));
      if (ug.length) {
        body += folderNodeHTML({ id: "__ungrouped__", name: t("hs.ungrouped", "未分组"), children: [] }, ctx, 0);
      }
    } else {
      body = categoryFallbackHTML(filtered, ctx);
    }

    let quick = "";
    if (mode === "multi") {
      quick = `<div class="hs-pick-quick">
        <button type="button" class="btn sm ghost" data-hp-act="all-online">${e(t("hs.select_all_online", "全选在线"))}</button>
        <button type="button" class="btn sm ghost" data-hp-act="clear">${e(t("hs.clear_sel", "清空"))}</button>
        <span class="hs-pick-meta">${selN}/${onlineN || filtered.length}</span>
      </div>`;
    } else if (mode === "target") {
      const allOn = !opts.targetValue || opts.targetValue === "all";
      quick = `<div class="hs-pick-quick hs-pick-target-quick">
        <label class="hs-pick-chip${allOn ? " is-on" : ""}"><input type="radio" class="hs-pick-all-radio" name="${e(name)}" value="all" ${allOn ? "checked" : ""}> ${e(t("ui.all_hosts", "全部主机"))}</label>
        ${(opts.systemOptions || []).map(s => {
          const val = `system:${s.val}`;
          const on = opts.targetValue === val;
          return `<label class="hs-pick-chip${on ? " is-on" : ""}"><input type="radio" class="hs-pick-sys-radio" name="${e(name)}" value="${e(val)}" ${on ? "checked" : ""}> ${e(s.label)}</label>`;
        }).join("")}
        <span class="hs-pick-meta">${e(t("playbook.target_hint", "点分组或主机选定目标"))}</span>
      </div>`;
    }

    return `<div class="hs-pick-tree-wrap host-picker${opts.compact ? " is-compact" : ""}" data-hp-mode="${e(mode)}" data-hp-id="${e(treeId)}">
      <div class="hs-pick-tools">
        <input type="search" id="${e(searchId)}" class="hs-pick-search" value="${e(opts.q || "")}" placeholder="${e(t("hs.host_search_ph", "搜索主机名 / IP / 分组…"))}" autocomplete="off">
        ${quick}
      </div>
      <div class="hs-pick-tree" id="${e(treeId)}">${body}</div>
    </div>`;
  }

  function syncIndeterminate(root) {
    if (!root) return;
    root.querySelectorAll("input[data-indeterminate]").forEach(cb => { cb.indeterminate = true; });
  }

  function readMulti(root) {
    const set = new Set();
    if (!root) return set;
    root.querySelectorAll(".hs-pick-host:checked").forEach(cb => set.add(cb.value));
    return set;
  }

  function readTarget(root) {
    if (!root) return "all";
    const checked = root.querySelector('input[type="radio"]:checked');
    return checked ? checked.value : "all";
  }

  function bind(root, handlers) {
    handlers = handlers || {};
    if (!root || root._hpBound) return;
    root._hpBound = true;
    syncIndeterminate(root);

    root.addEventListener("click", ev => {
      const caret = ev.target.closest("[data-hp-fold]");
      if (caret) {
        ev.preventDefault();
        const id = caret.getAttribute("data-hp-fold");
        if (handlers.onToggleFold) handlers.onToggleFold(id);
        return;
      }
      const act = ev.target.closest("[data-hp-act]");
      if (act && handlers.onQuick) {
        handlers.onQuick(act.getAttribute("data-hp-act"));
      }
    });

    root.addEventListener("change", ev => {
      const tgel = ev.target;
      if (!tgel) return;
      if (tgel.classList.contains("hs-pick-folder-cb") && handlers.onFolderToggle) {
        const fid = tgel.getAttribute("data-hp-folder");
        handlers.onFolderToggle(fid, tgel.checked, tgel.getAttribute("data-hp-cat"));
        return;
      }
      if (tgel.classList.contains("hs-pick-host") && handlers.onHostToggle) {
        handlers.onHostToggle(tgel.value, tgel.checked);
        return;
      }
      if ((tgel.classList.contains("hs-pick-folder-radio") || tgel.classList.contains("hs-pick-host-radio")
        || tgel.classList.contains("hs-pick-all-radio") || tgel.classList.contains("hs-pick-sys-radio"))
        && handlers.onTargetChange) {
        handlers.onTargetChange(tgel.value);
        return;
      }
      if (tgel.classList.contains("hs-pick-search") && handlers.onSearch) {
        handlers.onSearch(tgel.value);
      }
    });

    root.addEventListener("input", ev => {
      if (ev.target && ev.target.classList.contains("hs-pick-search") && handlers.onSearch) {
        handlers.onSearch(ev.target.value);
      }
    });
  }

  /** Apply folder checkbox to selected set using current DOM host rows under folder. */
  function applyFolderCheck(root, folderId, checked) {
    if (!root) return;
    // Prefer hosts listed under this folder node in the rendered tree.
    const lab = root.querySelector(`.hs-pick-folder-cb[data-hp-folder="${CSS.escape(folderId)}"]`);
    if (!lab) return;
    const folderEl = lab.closest(".hs-pick-folder");
    if (!folderEl) return;
    // Collect until next sibling folder at same or shallower depth — simpler: all following host rows until next folder at pad<=current
    let el = folderEl.nextElementSibling;
    const myPad = parseInt((folderEl.style.paddingLeft || "0"), 10) || 0;
    while (el) {
      if (el.classList.contains("hs-pick-folder")) {
        const pad = parseInt((el.style.paddingLeft || "0"), 10) || 0;
        if (pad <= myPad) break;
      }
      if (el.classList.contains("hs-pick-row")) {
        const cb = el.querySelector(".hs-pick-host");
        if (cb && !cb.disabled) {
          cb.checked = checked;
          el.classList.toggle("is-on", checked);
        }
      }
      el = el.nextElementSibling;
    }
  }

  function labelForTarget(target, hosts) {
    const v = String(target || "all");
    if (!v || v === "all") return t("ui.all_hosts", "全部主机");
    if (v.startsWith("host:")) {
      const id = v.slice(5);
      const h = (hosts || []).find(x => x.id === id);
      return h ? hostTitle(h) : id;
    }
    if (v.startsWith("folder:")) {
      const id = v.slice(7);
      const paths = (typeof HOST_FOLDERS !== "undefined" && HOST_FOLDERS && HOST_FOLDERS.paths) ? HOST_FOLDERS.paths : {};
      return paths[id] || id;
    }
    if (v.startsWith("category:")) return v.slice(9) || t("section.uncategorized", "未分类");
    if (v.startsWith("system:")) return v.slice(7);
    return v;
  }

  function optionLabel(h) {
    return hostTitle(h);
  }

  /** Fill a <select> with hostname (ip) labels — for single-host dropdowns. */
  function fillHostSelect(sel, hosts, selectedId, opts) {
    if (!sel) return;
    opts = opts || {};
    const empty = opts.emptyLabel != null ? opts.emptyLabel : t("ui.select_host", "选择主机");
    const includeEmpty = opts.includeEmpty !== false;
    let html = includeEmpty ? `<option value="">${e(empty)}</option>` : "";
    (hosts || []).forEach(h => {
      const lab = optionLabel(h);
      html += `<option value="${e(h.id)}" ${selectedId === h.id ? "selected" : ""}>${e(lab)}</option>`;
    });
    sel.innerHTML = html;
  }

  global.HostPicker = {
    renderHTML,
    bind,
    readMulti,
    readTarget,
    syncIndeterminate,
    applyFolderCheck,
    hostsByFolder,
    collectFolderHostIds,
    filterHost,
    hostIP,
    hostTitle,
    hostOnline,
    labelForTarget,
    optionLabel,
    fillHostSelect,
    folderTree,
  };
})(typeof window !== "undefined" ? window : globalThis);
