/* ---------- PWA: SW registration + Install prompt + Hash routing ---------- */
// P1-4: 全局 Escape 键关闭模态弹窗
document.addEventListener("keydown", e => {
  if (e.key === "Escape") {
    const masks = document.querySelectorAll(".mask.show:not([data-forced])");
    if (masks.length > 0) {
      // 只关闭最上层的弹窗
      const top = masks[masks.length - 1];
      closeMask(top);
    }
  }
});
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js", { scope: "/" }).then(reg => {
      if (reg) setInterval(() => reg.update().catch(() => {}), 60000); // 每分钟探测新版本
    }).catch(() => {});
  });
  // 新 SW 接管（clients.claim）时自动刷新一次，确保拿到最新 app.js —— 兜底防止
  // 旧 SW 缓存了坏版本（如某次拼接语法错误）把用户永久卡在坏页面（含登录页不显示）。
  // 用 controller 存否 + refreshing 双重守卫，避免首访无控制器时的刷新循环。
  let SW_REFRESHING = false;
  navigator.serviceWorker.addEventListener("controllerchange", () => {
    if (SW_REFRESHING || !navigator.serviceWorker.controller) return;
    SW_REFRESHING = true;
    location.reload();
  });
}
let DEFERRED_PROMPT = null;
window.addEventListener("beforeinstallprompt", e => {
  e.preventDefault();
  DEFERRED_PROMPT = e;
  setTimeout(() => {
    if (DEFERRED_PROMPT && !window.matchMedia("(display-mode: standalone)").matches) {
      toast(I18N.t("install.pwa_hint"), "ok");
    }
  }, 3000);
});
window.addEventListener("hashchange", () => {
  const h = location.hash.slice(1);
  if (h && ["overview", "hosts", "checks", "alerts", "automation", "log", "hardware", "netflow"].includes(h)) {
    switchView(h);
  }
});

// 语言就地切换：i18n-dashboard.js 已完成静态 [data-i18n] 文本替换，这里负责
// 重建 JS 动态生成的文案（页面标题/副标题、各视图列表、图表等），并保持当前视图/滚动/
// 已打开面板不变（不刷新页面）。
document.addEventListener("i18n:changed", () => {
  try { rebuildPageMeta(); } catch (e) {}
  const activeNav = document.querySelector(".nav-item.active");
  const view = activeNav ? activeNav.dataset.view : null;
  if (view && PAGE_META[view]) {
    const t = $("pageTitle"), s = $("pageSub");
    if (t) t.textContent = PAGE_META[view].title;
    if (s) s.textContent = PAGE_META[view].sub;
  }
  // 概览数据（卡片/健康/告警/活动/主机/TOP）强制重渲染
  try { refresh(true); } catch (e) {}
  // 当前视图专属的动态列表按需重载（模块级筛选/分页状态保持不变）
  try {
    if (view === "checks") loadChecks();
    else if (view === "automation") {
      const active = document.querySelector("#autoTabs .chip-btn.active");
      const tab = active && active.dataset.autotab ? active.dataset.autotab : "playbooks";
      if (tab === "inspect") loadHostInspect(); else loadPlaybooks();
    }
    else if (view === "forward") { loadForwards(); loadHttpProxies(); }
    else if (view === "hosts") loadHostsMeta();
  } catch (e) {}
  if (I18N.syncLangButtons) I18N.syncLangButtons();
});

initAuth();

/* ============================================================
   P3-1: WebSocket 推送（替代轮询，带降级）
   ============================================================ */
let PUSH_WS = null;
let PUSH_CONNECTED = false;
let PUSH_RETRY = 0;

function initPushWS() {
  // 仅在 HTTPS 或 localhost 下尝试 WebSocket
  if (!window.WebSocket || (!window.isSecureContext && location.hostname !== "localhost")) return;
  try {
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    PUSH_WS = new WebSocket(proto + "//" + location.host + "/ws/push");
    PUSH_WS.onopen = () => {
      PUSH_CONNECTED = true;
      PUSH_RETRY = 0;
    };
    PUSH_WS.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data);
        // 与轮询一致地按当前活动视图渲染，避免推送触发隐藏视图的全量重建。
        const activeView = document.querySelector(".view.active")?.id.replace("view-", "") || "overview";
        if (msg.type === "summary" && msg.data) {
          if (activeView === "overview") renderCards(msg.data);
          updateFavicon(msg.data.critical_alerts || 0);
          notifyCriticalAlerts(msg.data.critical_alerts || 0);
        } else if (msg.type === "alerts" && msg.data) {
          if (activeView === "overview" || activeView === "alerts") renderAlerts(msg.data);
        } else if (msg.type === "hosts" && msg.data) {
          if (activeView === "hosts") renderHosts(msg.data);
        }
      } catch(err) {}
    };
    PUSH_WS.onclose = () => {
      PUSH_CONNECTED = false;
      // 指数退避重连
      PUSH_RETRY++;
      if (PUSH_RETRY <= 10) {
        setTimeout(() => initPushWS(), Math.min(30000, 1000 * Math.pow(2, PUSH_RETRY)));
      }
    };
    PUSH_WS.onerror = () => { try { PUSH_WS.close(); } catch(e) {} };
  } catch(e) {}
}

/* ============================================================
   端口转发
   ============================================================ */
let LAST_FORWARDS = [];
let LAST_HTTP_PROXIES = [];
let FWD_MODE = "tcp"; // "tcp" | "http"

// 填充主机下拉选择框（同时填充创建弹窗 fwdHost 和编辑弹窗 fwdEditHost）
function populateForwardHosts() {
  const opts = LAST_HOSTS.map(h => `<option value="${h.id}">${esc(h.hostname)} (${esc(h.ip || "—")})</option>`).join("");
  const fh = $("fwdHost");
  if (fh) fh.innerHTML = opts;
  const efh = $("fwdEditHost");
  if (efh) efh.innerHTML = opts;
}

function short(id) { return id && id.length > 8 ? id.slice(0, 8) : id; }

async function loadForwards() {
  try {
    const res = await fetch("/api/v1/forward", { credentials: "include" });
    if (!res.ok) return;
    LAST_FORWARDS = await res.json();
    // Also load HTTP proxies
    try {
      const httpRes = await fetch("/api/v1/http-proxy", { credentials: "include" });
      if (httpRes.ok) LAST_HTTP_PROXIES = await httpRes.json();
    } catch(e) {}
    renderForwards();
  } catch(e) {}
}

// 把当前端口转发/代理快照汇总为纯文本供 AI 分析；仅人工采纳/反馈后的结果进入学习闭环。
function forwardsToText() {
  const tcp = LAST_FORWARDS || [];
  const http = LAST_HTTP_PROXIES || [];
  if (!tcp.length && !http.length) return "（当前没有任何端口转发/代理规则）";
  let off = 0, active = 0;
  const lines = [];
  tcp.forEach(f => {
    const en = f.enabled !== false;
    if (!en) off++;
    active += (f.sessions || 0);
    lines.push(`- [${f.protocol === "udp" ? "UDP" : "TCP"}] ${f.hostname} → :${f.target_port} 监听=${f.listen_addr}${f.remote_target ? " 跳板→" + f.remote_target : ""} ${en ? "启用" : "停用"} 活跃会话=${f.sessions || 0} 累计=${f.total_sessions || 0}`);
  });
  http.forEach(p => {
    const en = p.enabled !== false;
    if (!en) off++;
    active += (p.sessions || 0);
    lines.push(`- [HTTP] ${p.name || (p.hostname + ":" + p.target_port)} 目标=${p.hostname}:${p.target_port}${p.default_path || ""} ${en ? "启用" : "停用"} 活跃会话=${p.sessions || 0} 累计=${p.total_sessions || 0}`);
  });
  const head = `转发规则共 ${tcp.length + http.length} 条（TCP/UDP ${tcp.length} · HTTP ${http.length}） · 停用 ${off} 条 · 当前活跃会话合计 ${active}。\n`;
  return (head + lines.join("\n")).slice(0, 12000);
}

// 「🤖 AI 分析」：对当前所有端口转发/代理的连通性/会话/暴露面做整体研判，结果自动进入 RAG 记忆闭环
safeAddEventListener("fwdAIBtn", "click", () => {
  if (typeof openAIAssist !== "function") { if (typeof toast === "function") toast(I18N.t("assist.unavailable", "AI 面板未就绪"), "err"); return; }
  openAIAssist({ task: "forward_diagnosis", mode: "analyze", title: I18N.t("assist.title_forward", "AI · 端口转发分析"), context: forwardsToText() });
});

// 转发视图模式：list（默认）| card
let FORWARD_VIEW_MODE = (function () { try { return localStorage.getItem("aiops_fwd_view") === "list" ? "list" : "card"; } catch (e) { return "card"; } })(); // 默认卡片视图
let AUTOMATION_VIEW_MODE = (function () { try { return localStorage.getItem("aiops_pb_view") === "list" ? "list" : "card"; } catch (e) { return "card"; } })(); // 编排默认卡片视图

// 操作图标（统一的描边 SVG，使用 currentColor 跟随文字色）
const FWD_ICONS = {
  enable:  '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"/></svg>',
  disable: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M6 6h12v12H6z"/></svg>',
  copy:    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/></svg>',
  edit:    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4z"/></svg>',
  del:     '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2m2 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/></svg>',
  open:    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6M15 3h6v6M10 14L21 3"/></svg>',
  addr:    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/></svg>',
};

function switchForwardView(mode) {
  FORWARD_VIEW_MODE = mode;
  try { localStorage.setItem("aiops_fwd_view", mode); } catch (e) {}
  const wrap = $("forwardViewToggle");
  if (wrap) wrap.querySelectorAll(".vt-btn").forEach(b => b.classList.toggle("active", b.dataset.view === mode));
  renderForwards();
}

// 构建单条转发的操作按钮组
function fwdActionButtons(item) {
  const toggleIcon = item.enabled ? FWD_ICONS.disable : FWD_ICONS.enable;
  const toggleLabel = item.enabled ? I18N.t("ui.disable") : I18N.t("ui.enable");
  const primary = item.type === "http"
    ? `<button class="icon-btn" title="${I18N.t("ui.open")}" data-act="proxy-open" data-url="${esc(item.proxyUrl)}">${FWD_ICONS.open}</button>`
    : ""; // TCP：应用户要求移除「复制地址」按钮（列表/卡片里的监听地址仍可直接复制）
  return `
    <button class="icon-btn" title="${toggleLabel}" data-act="fwd-toggle" data-ftype="${esc(item.type)}" data-fid="${esc(item.id)}" data-enable="${item.enabled ? "0" : "1"}">${toggleIcon}</button>
    ${primary}
    <button class="icon-btn" title="${I18N.t("ui.copy")}" data-act="fwd-copy" data-ftype="${esc(item.type)}" data-fid="${esc(item.id)}">${FWD_ICONS.copy}</button>
    <button class="icon-btn" title="${I18N.t("ui.edit")}" data-act="fwd-edit" data-ftype="${esc(item.type)}" data-fid="${esc(item.id)}">${FWD_ICONS.edit}</button>
    <button class="icon-btn danger" title="${I18N.t("ui.delete")}" data-act="fwd-del" data-ftype="${esc(item.type)}" data-fid="${esc(item.id)}">${FWD_ICONS.del}</button>`;
}

// 将 TCP / HTTP 两条数据源统一为渲染模型
function collectForwardItems() {
  const items = [];
  (LAST_FORWARDS || []).forEach(f => {
    const jumpTag = f.remote_target ? ` · <span class="badge warn" title="跳板目标">⇢ ${esc(f.remote_target)}</span>` : "";
    items.push({
      type: "tcp", id: f.id,
      enabled: f.enabled !== false,
      badge: f.protocol === "udp" ? "UDP" : "TCP", badgeClass: "op",
      title: `${esc(f.hostname)} → :${f.target_port}`,
      sub: `${I18N.t("ui.listen_addr")} <code class="mono">${esc(f.listen_addr)}</code>${jumpTag} · ${f.sessions} ${I18N.t("ui.active_sessions")} · ${f.total_sessions || 0} ${I18N.t("ui.total_sessions")}`,
      listenAddr: f.listen_addr,
      groupID: f.group_id || "",           // 端口范围批量组（同组共享）
      targetPort: f.target_port,
      protocol: f.protocol === "udp" ? "udp" : "tcp",
      hostname: f.hostname,
    });
  });
  (LAST_HTTP_PROXIES || []).forEach(p => {
    const proxyUrl = `/proxy/${encodeURIComponent(p.host_id)}/${p.target_port}/${(p.default_path || "").replace(/^\//, "")}`;
    items.push({
      type: "http", id: p.id,
      enabled: p.enabled !== false,
      badge: "HTTP", badgeClass: "sys",
      title: esc(p.name || `${p.hostname}:${p.target_port}`) + (p.is_copy ? I18N.t("forward.copy_suffix") : ""),
      sub: `${esc(p.hostname)}:${p.target_port}${p.default_path ? " · " + esc(p.default_path) : ""} · ${p.sessions || 0} ${I18N.t("ui.active_sessions")} · ${p.total_sessions || 0} ${I18N.t("ui.total_sessions")}`,
      proxyUrl,
    });
  });
  if (FWD_SEARCH) { const q = FWD_SEARCH.toLowerCase(); return items.filter(it => ((it.title || "") + " " + (it.sub || "") + " " + (it.listenAddr || "") + " " + (it.proxyUrl || "")).toLowerCase().includes(q)); }
  return items;
}

// 把转发项按端口范围组（groupID）聚合：同组多条 → 一个组单元；其余 → 单条单元。
function fwdRenderUnits(items) {
  const groups = {}; const units = [];
  items.forEach(it => {
    if (it.groupID) {
      if (!groups[it.groupID]) { groups[it.groupID] = { group: true, gid: it.groupID, items: [] }; units.push(groups[it.groupID]); }
      groups[it.groupID].items.push(it);
    } else {
      units.push({ group: false, item: it });
    }
  });
  // 只有 1 条的“组”降级为单条显示
  return units.map(u => (u.group && u.items.length === 1) ? { group: false, item: u.items[0] } : u);
}
function fwdGroupMeta(u) {
  const its = u.items;
  const ports = its.map(x => x.targetPort).filter(Boolean).sort((a, b) => a - b);
  const enabledCount = its.filter(x => x.enabled).length;
  return {
    hostname: its[0].hostname, badge: its[0].badge, badgeClass: its[0].badgeClass,
    count: its.length, enabledCount, portMin: ports[0], portMax: ports[ports.length - 1],
    anyEnabled: enabledCount > 0,
  };
}
// 组操作按钮：整组启停 + 整组删除（一次操作整段端口范围）
function fwdGroupActions(u) {
  const m = fwdGroupMeta(u);
  const toggleIcon = m.anyEnabled ? FWD_ICONS.disable : FWD_ICONS.enable;
  const toggleLabel = m.anyEnabled ? "停用整组" : "启用整组";
  return `
    <button class="icon-btn" title="${toggleLabel}" data-act="fwd-group-toggle" data-gid="${esc(u.gid)}" data-enable="${m.anyEnabled ? "0" : "1"}">${toggleIcon}</button>
    <button class="icon-btn" title="复制整组（${m.count} 条）" data-act="fwd-group-copy" data-gid="${esc(u.gid)}" data-count="${m.count}">${FWD_ICONS.copy}</button>
    <button class="icon-btn" title="编辑整组" data-act="fwd-group-edit" data-gid="${esc(u.gid)}">${FWD_ICONS.edit}</button>
    <button class="icon-btn danger" title="删除整组（${m.count} 条）" data-act="fwd-group-del" data-gid="${esc(u.gid)}" data-count="${m.count}">${FWD_ICONS.del}</button>`;
}

function renderForwards() {
  const list = $("forwardList");
  const empty = $("forwardEmpty");
  if (!list || !empty) return;

  // 同步视图切换按钮的选中态到当前模式（覆盖初次加载时 HTML 静态 active 与持久化偏好不一致的情况）
  const vt = $("forwardViewToggle");
  if (vt) vt.querySelectorAll(".vt-btn").forEach(b => b.classList.toggle("active", b.dataset.view === FORWARD_VIEW_MODE));

  const items = collectForwardItems();
  const units = fwdRenderUnits(items);

  const cardHTML = u => {
    if (u.group) {
      const m = fwdGroupMeta(u);
      return `
      <div class="pb-card fwd-card fwd-group ${m.anyEnabled ? "" : "pb-off"}">
        <div class="pb-card-top">
          <div class="pb-card-title">
            <strong>${esc(m.hostname)} → :${m.portMin}-${m.portMax}</strong>
            <span class="pb-desc">端口范围组 · ${m.count} 条端口 · ${m.enabledCount} 启用</span>
          </div>
          <span class="fwd-status ${m.anyEnabled ? "on" : "off"}">${m.badge} 组</span>
        </div>
        <div class="pb-card-foot">
          <div class="pb-pills"><span class="badge ${m.badgeClass}">${m.badge}</span><span class="pb-pill">${m.count} 端口</span></div>
          <div class="fwd-actions">${fwdGroupActions(u)}</div>
        </div>
      </div>`;
    }
    const it = u.item;
    return `
      <div class="pb-card fwd-card ${it.enabled ? "" : "pb-off"}">
        <div class="pb-card-top">
          <div class="pb-card-title">
            <strong>${it.title}</strong>
            <span class="pb-desc">${it.sub}</span>
          </div>
          <span class="fwd-status ${it.enabled ? "on" : "off"}">${it.enabled ? I18N.t("ui.enabled") : I18N.t("ui.disabled")}</span>
        </div>
        <div class="pb-card-foot">
          <div class="pb-pills"><span class="badge ${it.badgeClass}">${it.badge}</span></div>
          <div class="fwd-actions">${fwdActionButtons(it)}</div>
        </div>
      </div>`;
  };
  const rowHTML = u => {
    if (u.group) {
      const m = fwdGroupMeta(u);
      return `
      <div class="fwd-row fwd-group ${m.anyEnabled ? "" : "fwd-off"}">
        <span class="badge ${m.badgeClass}">${m.badge}</span>
        <div class="fwd-main">
          <div class="fwd-title">${esc(m.hostname)} → :${m.portMin}-${m.portMax} <span class="pb-pill">${m.count} 端口</span></div>
          <div class="fwd-sub">端口范围组 · ${m.count} 条 · ${m.enabledCount} 启用</div>
        </div>
        <div class="fwd-actions">${fwdGroupActions(u)}</div>
      </div>`;
    }
    const it = u.item;
    return `
      <div class="fwd-row ${it.enabled ? "" : "fwd-off"}">
        <span class="badge ${it.badgeClass}">${it.badge}</span>
        <div class="fwd-main">
          <div class="fwd-title">${it.title}</div>
          <div class="fwd-sub">${it.sub}</div>
        </div>
        <div class="fwd-actions">${fwdActionButtons(it)}</div>
      </div>`;
  };

  if (FORWARD_VIEW_MODE === "card") {
    list.className = "fwd-list fwd-grid";
    list.innerHTML = units.map(cardHTML).join("");
  } else {
    list.className = "fwd-list";
    list.innerHTML = units.map(rowHTML).join("");
  }

  empty.style.display = items.length ? "none" : "";
}

function switchFwdMode(mode) {
  FWD_MODE = mode;
  document.querySelectorAll("#fwdModeTabs .fwd-mode-tab").forEach(btn => {
    btn.classList.toggle("active", btn.dataset.fwdmode === mode);
  });
  const tcpFields = $("fwdTcpFields");
  const httpFields = $("fwdHttpFields");
  const tcpFoot = $("fwdTcpFoot");
  const httpFoot = $("fwdHttpFoot");
  if (tcpFields) tcpFields.style.display = mode === "tcp" ? "" : "none";
  if (httpFields) httpFields.style.display = mode === "http" ? "" : "none";
  if (tcpFoot) tcpFoot.style.display = mode === "tcp" ? "" : "none";
  if (httpFoot) httpFoot.style.display = mode === "http" ? "" : "none";
}

function submitForward() {
  const hostID = $("fwdHost")?.value;
  const targetPort = parseInt($("fwdTargetPort")?.value || "0");
  if (!hostID || targetPort < 1 || targetPort > 65535) {
    toast(I18N.t("valid.fill_target_port"), "err");
    return;
  }
  if (FWD_MODE === "tcp") {
    createTcpForward(hostID, targetPort);
  } else {
    openHttpProxy(hostID, targetPort);
  }
}

async function createTcpForward(hostID, targetPort) {
  const localPort = parseInt($("fwdLocalPort")?.value || "0");
  const protocol = $("fwdProtocol")?.value || "tcp"; // tcp | udp
  const endPort = parseInt($("fwdTargetPortEnd")?.value || "0"); // > targetPort = 端口范围批量转发
  const remoteTarget = ($("fwdRemoteTarget")?.value || "").trim();
  const body = { host_id: hostID, target_port: targetPort, local_port: localPort, protocol };
  if (endPort > targetPort) body.target_port_end = endPort;
  if (remoteTarget) body.remote_target = remoteTarget;
  await withLoading("fwdSubmitBtn", async () => {
    try {
      const res = await fetch("/api/v1/forward", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(body)
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        toast(err.error || I18N.t("toast.create_failed"), "err");
        return;
      }
      const result = await res.json();
      if (result.count > 1) toast(`已批量创建 ${result.count} 条 ${protocol.toUpperCase()} 转发（端口 ${targetPort}-${endPort}）`, "ok");
      else toast(I18N.t("toast.forward_created") + result.listen_addr, "ok");
      closeForwardModal();
      loadForwards();
    } catch(e) {
      toast(I18N.t("toast.network_error2"), "err");
    }
  });
}

function openHttpProxy(hostID, targetPort) {
  const path = $("fwdHttpPath")?.value || "";
  const baseUrl = `/proxy/${encodeURIComponent(hostID)}/${encodeURIComponent(targetPort)}/${path.replace(/^\//, "")}`;
  openProxyUrl(baseUrl);
  closeForwardModal();
}

// openProxyUrl fetches a single-use proxy auth token (which the server
// also sets as a cookie), then opens the proxy URL in a new tab. The
// cookie is automatically carried by the new tab request, avoiding the
// "unauthorized" error that occurs when the session cookie is not sent.
async function openProxyUrl(baseUrl) {
  try {
    const res = await fetch("/api/v1/proxy-token", { credentials: "include" });
    if (!res.ok) { toast(I18N.t("toast.network_error2"), "err"); return; }
    // Token cookie is now set; open the URL — backend reads cookie or pt param.
    const data = await res.json();
    const sep = baseUrl.includes("?") ? "&" : "?";
    window.open(baseUrl + sep + "pt=" + encodeURIComponent(data.token), "_blank");
  } catch(e) {
    toast(I18N.t("toast.network_error2"), "err");
  }
}

async function saveHttpProxy() {
  const hostID = $("fwdHost")?.value;
  const targetPort = parseInt($("fwdTargetPort")?.value || "0");
  const name = $("fwdHttpName")?.value || "";
  const defaultPath = $("fwdHttpPath")?.value || "";
  if (!hostID || targetPort < 1 || targetPort > 65535) {
    toast(I18N.t("valid.fill_target_port"), "err");
    return;
  }
  await withLoading("fwdHttpSaveBtn", async () => {
    try {
      const res = await fetch("/api/v1/http-proxy", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ host_id: hostID, target_port: targetPort, name, default_path: defaultPath, enabled: true })
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        toast(err.error || I18N.t("toast.save_failed"), "err");
        return;
      }
      toast(I18N.t("toast.saved"), "ok");
      closeForwardModal();
      loadHttpProxies();
    } catch(e) {
      toast(I18N.t("toast.network_error2"), "err");
    }
  });
}

// 保存 HTTP 反向代理配置并立即在新标签打开（合并原「保存配置」+「打开链接」两个按钮）。
// 先同步 window.open 占位窗口，避免 save/token 两次 await 之后 window.open 被弹窗拦截。
async function saveAndOpenHttpProxy() {
  const hostID = $("fwdHost")?.value;
  const targetPort = parseInt($("fwdTargetPort")?.value || "0");
  const name = $("fwdHttpName")?.value || "";
  const defaultPath = $("fwdHttpPath")?.value || "";
  if (!hostID || targetPort < 1 || targetPort > 65535) {
    toast(I18N.t("valid.fill_target_port"), "err");
    return;
  }
  const win = window.open("", "_blank"); // 同步占位窗口，保住用户手势，规避弹窗拦截
  await withLoading("fwdHttpOpenBtn", async () => {
    try {
      const res = await fetch("/api/v1/http-proxy", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ host_id: hostID, target_port: targetPort, name, default_path: defaultPath, enabled: true })
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        if (win) win.close();
        toast(err.error || I18N.t("toast.save_failed"), "err");
        return;
      }
      toast(I18N.t("toast.saved"), "ok");
      loadHttpProxies();
      // 取一次性代理令牌（服务端同时下发 cookie），再把占位窗口导航到代理地址
      const tokRes = await fetch("/api/v1/proxy-token", { credentials: "include" });
      const baseUrl = `/proxy/${encodeURIComponent(hostID)}/${encodeURIComponent(targetPort)}/${defaultPath.replace(/^\//, "")}`;
      if (tokRes.ok) {
        const tok = await tokRes.json();
        const sep = baseUrl.includes("?") ? "&" : "?";
        const url = baseUrl + sep + "pt=" + encodeURIComponent(tok.token);
        if (win) win.location = url; else window.open(url, "_blank");
      } else if (win) {
        win.close();
      }
      closeForwardModal();
    } catch (e) {
      if (win) win.close();
      toast(I18N.t("toast.network_error2"), "err");
    }
  });
}

async function loadHttpProxies() {
  try {
    const res = await fetch("/api/v1/http-proxy", { credentials: "include" });
    if (!res.ok) return;
    LAST_HTTP_PROXIES = await res.json();
    renderForwards();
  } catch(e) {}
}

// 启用 / 停用某条转发（TCP 或 HTTP）
async function toggleForward(btn, type, id, enable) {
  // 注意：必须传入按钮元素本身。此前传的是委托事件的 currentTarget = document，
  // withLoading 内 `document.style.opacity=...` 会抛错（document 无 style），
  // 导致 fetch 根本没执行 —— 表现为「点开关无反应、状态无法切换」。
  await withLoading(btn, async () => {
    const url = type === "tcp"
      ? `/api/v1/forward/${id}/toggle`
      : `/api/v1/http-proxy/${id}/toggle`;
    try {
      const res = await fetch(url, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ enabled: enable })
      });
      if (!res.ok) { toast(I18N.t("toast.toggle_failed"), "err"); return; }
      toast(enable ? I18N.t("toast.enabled") : I18N.t("toast.disabled"), "ok");
      loadForwards();
    } catch(e) {
      toast(I18N.t("toast.network_error2"), "err");
    }
  })
}

// 整组启用 / 停用（端口范围批量组）
// 必须传入按钮元素本身（同 toggleForward）：委托事件的 currentTarget=document，
// withLoading 内 document.style.opacity 会抛错，导致 fetch 根本没执行 → 开关无反应。
async function toggleForwardGroup(btn, gid, enable) {
  await withLoading(btn, async () => {
    try {
      const res = await fetch(`/api/v1/forward/group/${encodeURIComponent(gid)}/toggle`, {
        method: "PUT", headers: { "Content-Type": "application/json" }, credentials: "include",
        body: JSON.stringify({ enabled: enable })
      });
      if (!res.ok) { toast(I18N.t("toast.toggle_failed"), "err"); return; }
      const j = await res.json().catch(() => ({}));
      toast((enable ? "已启用整组 " : "已停用整组 ") + (j.toggled || 0) + " 条", "ok");
      loadForwards();
    } catch (e) { toast(I18N.t("toast.network_error2"), "err"); }
  });
}
// 整组删除（端口范围批量组）——一次删完整段端口，免逐条删除
async function deleteForwardGroup(gid, count) {
  if (!confirm(`确认删除该端口范围组的全部 ${count || ""} 条转发规则？`)) return;
  try {
    const res = await fetch(`/api/v1/forward/group/${encodeURIComponent(gid)}`, { method: "DELETE", credentials: "include" });
    if (!res.ok) { toast(I18N.t("toast.delete_failed") || "删除失败", "err"); return; }
    const j = await res.json().catch(() => ({}));
    toast("已删除整组 " + (j.deleted || 0) + " 条转发", "ok");
    loadForwards();
  } catch (e) { toast(I18N.t("toast.network_error2"), "err"); }
}

// 复制（克隆）某条转发
async function copyForward(type, id) {
  const url = type === "tcp"
    ? `/api/v1/forward/${id}/copy`
    : `/api/v1/http-proxy/${id}/copy`;
  try {
    const res = await fetch(url, { method: "POST", credentials: "include" });
    if (!res.ok) { toast(I18N.t("toast.copy_failed"), "err"); return; }
    toast(I18N.t("toast.copied"), "ok");
    loadForwards();
  } catch(e) {
    toast(I18N.t("toast.network_error2"), "err");
  }
}

// 整组复制（端口范围批量组）
async function copyForwardGroup(gid, count) {
  try {
    const res = await fetch(`/api/v1/forward/group/${encodeURIComponent(gid)}/copy`, { method: "POST", credentials: "include" });
    if (!res.ok) { toast(I18N.t("toast.copy_failed"), "err"); return; }
    const j = await res.json().catch(() => ({}));
    toast("已复制整组 " + (j.copied || 0) + " 条转发", "ok");
    loadForwards();
  } catch (e) { toast(I18N.t("toast.network_error2"), "err"); }
}

// 整组编辑（端口范围批量组）——打开编辑弹窗，预填首条规则数据
function editForwardGroup(gid) {
  const rules = (LAST_FORWARDS || []).filter(f => f.group_id === gid);
  if (rules.length === 0) return;
  const ports = rules.map(r => r.target_port).filter(Boolean).sort((a, b) => a - b);
  const minP = ports[0], maxP = ports[ports.length - 1];
  const first = rules[0];
  $("fwdEditId").value = "";
  $("fwdEditType").value = "tcp";
  $("fwdEditGroupId").value = gid;
  populateForwardHosts();
  $("fwdEditHost").value = first.host_id;
  $("fwdEditPort").value = minP; // 起始端口（整段平移基准）
  $("fwdEditTcpField").style.display = "none"; // 组：本地端口镜像目标端口，无需单独填
  $("fwdEditHttpNameField").style.display = "none";
  $("fwdEditHttpPathField").style.display = "none";
  const hint = $("fwdEditGroupHint");
  if (hint) {
    hint.textContent = `端口范围组：共 ${rules.length} 条端口 ${minP}-${maxP}（${(first.protocol || "tcp").toUpperCase()}）。改「目标端口」为新起始端口即整段平移；改主机则整组切换；本地端口自动镜像。`;
    hint.style.display = "";
  }
  const title = document.querySelector("#fwdEditMask .modal-head h3");
  if (title) title.textContent = "编辑端口范围组";
  const mask = $("fwdEditMask");
  const backdrop = $("backdrop");
  if (mask) mask.classList.add("show");
  if (backdrop) backdrop.style.display = "";
}

function closeForwardModal() {
  const forwardMask = $("forwardMask");
  const backdrop = $("backdrop");
  if (forwardMask) forwardMask.classList.remove("show");
  if (backdrop) backdrop.style.display = "none";
}

// 删除某条转发（统一 TCP / HTTP）
async function deleteForward(type, id) {
  if (!confirm(I18N.t("valid.confirm_delete"))) return;
  const url = type === "tcp"
    ? `/api/v1/forward/${id}`
    : `/api/v1/http-proxy/${id}`;
  try {
    const res = await fetch(url, { method: "DELETE", credentials: "include" });
    if (res.ok) {
      toast(I18N.t("toast.deleted"), "ok");
      loadForwards();
    } else {
      toast(I18N.t("toast.delete_failed"), "err");
    }
  } catch(e) {
    toast(I18N.t("toast.network_error2"), "err");
  }
}

// 打开编辑弹窗并预填数据
function editForward(type, id) {
  const item = type === "tcp"
    ? (LAST_FORWARDS || []).find(f => f.id === id)
    : (LAST_HTTP_PROXIES || []).find(p => p.id === id);
  if (!item) return;
  $("fwdEditId").value = id;
  $("fwdEditType").value = type;
  $("fwdEditGroupId").value = ""; // 单条编辑，清空组 ID
  const gHint = $("fwdEditGroupHint"); if (gHint) gHint.style.display = "none"; // 单条编辑隐藏组提示
  const eTitle = document.querySelector("#fwdEditMask .modal-head h3"); if (eTitle) eTitle.textContent = "编辑转发";
  populateForwardHosts();
  $("fwdEditHost").value = item.host_id;
  $("fwdEditPort").value = item.target_port;
  if (type === "tcp") {
    $("fwdEditTcpField").style.display = "";
    $("fwdEditLocalPort").value = item.local_port || 0;
    $("fwdEditHttpNameField").style.display = "none";
    $("fwdEditHttpPathField").style.display = "none";
  } else {
    $("fwdEditTcpField").style.display = "none";
    $("fwdEditHttpNameField").style.display = "";
    $("fwdEditHttpPathField").style.display = "";
    $("fwdEditName").value = item.name || "";
    $("fwdEditPath").value = item.default_path || "";
  }
  const mask = $("fwdEditMask");
  const backdrop = $("backdrop");
  if (mask) mask.classList.add("show");
  if (backdrop) backdrop.style.display = "";
}

function closeForwardEditModal() {
  const mask = $("fwdEditMask");
  const backdrop = $("backdrop");
  if (mask) mask.classList.remove("show");
  if (backdrop) backdrop.style.display = "none";
}

// 保存编辑结果
async function saveForwardEdit() {
  const type = $("fwdEditType").value;
  const id = $("fwdEditId").value;
  const gid = $("fwdEditGroupId").value;
  const hostID = $("fwdEditHost").value;
  const targetPort = parseInt($("fwdEditPort").value || "0");
  if (!hostID || targetPort < 1 || targetPort > 65535) {
    toast(I18N.t("valid.fill_target_port"), "err");
    return;
  }
  // 整组编辑：走 group API
  if (gid) {
    const localPort = parseInt($("fwdEditLocalPort").value || "0");
    try {
      const res = await fetch(`/api/v1/forward/group/${encodeURIComponent(gid)}/edit`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ host_id: hostID, target_port: targetPort, local_port: localPort })
      });
      if (!res.ok) { toast(I18N.t("toast.edit_failed"), "err"); return; }
      const j = await res.json().catch(() => ({}));
      toast("已编辑整组 " + (j.edited || 0) + " 条转发", "ok");
      closeForwardEditModal();
      loadForwards();
    } catch (e) { toast(I18N.t("toast.network_error2"), "err"); }
    return;
  }
  if (type === "tcp") {
    const localPort = parseInt($("fwdEditLocalPort").value || "0");
    try {
      const res = await fetch(`/api/v1/forward/${id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ host_id: hostID, target_port: targetPort, local_port: localPort })
      });
      if (!res.ok) { toast(I18N.t("toast.edit_failed"), "err"); return; }
      toast(I18N.t("toast.edited"), "ok");
      closeForwardEditModal();
      loadForwards();
    } catch(e) { toast(I18N.t("toast.network_error2"), "err"); }
  } else {
    const name = $("fwdEditName").value || "";
    const defaultPath = $("fwdEditPath").value || "";
    // 保持当前启用状态，避免编辑后规则被意外禁用
    const cur = (LAST_HTTP_PROXIES || []).find(p => p.id === id);
    const keepEnabled = cur ? cur.enabled !== false : true;
    try {
      const res = await fetch(`/api/v1/http-proxy/${id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ host_id: hostID, target_port: targetPort, name, default_path: defaultPath, enabled: keepEnabled })
      });
      if (!res.ok) { toast(I18N.t("toast.edit_failed"), "err"); return; }
      toast(I18N.t("toast.edited"), "ok");
      closeForwardEditModal();
      loadForwards();
    } catch(e) { toast(I18N.t("toast.network_error2"), "err"); }
  }
}

// 绑定事件
safeAddEventListener("addForwardBtn", "click", () => {
  populateForwardHosts();
  switchFwdMode("tcp");
  const targetPort = $("fwdTargetPort");
  const localPort = $("fwdLocalPort");
  const httpPath = $("fwdHttpPath");
  const forwardMask = $("forwardMask");
  const backdrop = $("backdrop");
  if (targetPort) targetPort.value = "";
  if (localPort) localPort.value = "";
  if (httpPath) httpPath.value = "";
  if (forwardMask) forwardMask.classList.add("show");
  if (backdrop) backdrop.style.display = "";
  // 清空远程目标输入
  const remoteTarget = $("fwdRemoteTarget"); if (remoteTarget) remoteTarget.value = "";
});
safeAddEventListener("fwdSubmitBtn", "click", submitForward);
// 「保存并打开」：合并了原「保存配置」+「打开链接」（保存配置按钮已移除）
safeAddEventListener("fwdHttpOpenBtn", "click", saveAndOpenHttpProxy);
safeAddEventListener("fwdEditSaveBtn", "click", saveForwardEdit);
// Mode tab clicks
document.querySelectorAll("#fwdModeTabs .fwd-mode-tab").forEach(btn => {
  btn.addEventListener("click", () => switchFwdMode(btn.dataset.fwdmode));
});

// 复制文本到剪贴板
function copyText(text) {
  navigator.clipboard?.writeText(text).then(() => toast(I18N.t("toast.copied_detail") + text, "ok"));
}

/* ============================================================
   CSP 加固：把所有内联 on*= 事件处理器统一改为「事件委托」
   —— 目的有二：
   1) 允许在 CSP 中移除 script-src 'unsafe-inline'，即便出现 XSS 也无法执行内联脚本；
   2) 消除此前把主机名/代理 URL 直接拼进 onclick JS 字符串导致的 DOM XSS
      （esc() 不转义单引号，恶意主机名带 ' 即可越界注入）。
   约定：可点击元素用 data-act="动作"，附带的参数放 data-* 属性，经 dataset 读取，
   数据不再进入任何可执行上下文。
   ============================================================ */
document.addEventListener("click", e => {
  const el = e.target.closest("[data-act]");
  if (!el) return;
  switch (el.dataset.act) {
    case "install": openInstall(); break;
    case "ai-preset": setAIPreset(el.dataset.preset); break;
    case "fwd-view": switchForwardView(el.dataset.view); break;
    case "pb-view": switchAutomationView(el.dataset.view); break;
    case "term-observe": openTerminalObserve(el.dataset.sid, el.dataset.host); break;
    case "term-replay": openTerminalReplay(el.dataset.sid, el.dataset.host); break;
    case "proxy-open": openProxyUrl(el.dataset.url); break;
    case "fwd-toggle": toggleForward(el, el.dataset.ftype, el.dataset.fid, el.dataset.enable === "1"); break;
    case "fwd-copy": copyForward(el.dataset.ftype, el.dataset.fid); break;
    case "fwd-edit": editForward(el.dataset.ftype, el.dataset.fid); break;
    case "fwd-del": deleteForward(el.dataset.ftype, el.dataset.fid); break;
    case "fwd-group-toggle": toggleForwardGroup(el, el.dataset.gid, el.dataset.enable === "1"); break;
    case "fwd-group-copy": copyForwardGroup(el.dataset.gid, parseInt(el.dataset.count || "0")); break;
    case "fwd-group-edit": editForwardGroup(el.dataset.gid); break;
    case "fwd-group-del": deleteForwardGroup(el.dataset.gid, el.dataset.count); break;
    case "copy-input": navigator.clipboard?.writeText(el.value); toast(I18N.t("toast.copied"), "ok"); break;
  }
});
document.addEventListener("change", e => {
  const el = e.target.closest("[data-act-change]");
  if (!el) return;
  switch (el.dataset.actChange) {
    case "filter-hosts": filterHosts(el.value); break;
    case "sort-hosts": sortHosts(el.value); break;
    case "filter-logs-level": filterLogsByLevel(el.value); break;
    case "filter-logs-time": filterLogsByTime(el.value); break;
    case "log-page-size": setLogPageSize(el.value); break;
    case "filter-checks": filterChecks(el.value); break;
    case "pb-target-preview": pbTargetPreview(el); break;
    case "pb-module-change": pbModuleChange(el); break;
  }
});

/* ============================================================
   离线检测
   ============================================================ */
window.addEventListener("online", () => {
  toast(I18N.t("toast.network_recovered"), "ok");
  refresh(true);
});
window.addEventListener("offline", () => {
  toast(I18N.t("toast.network_disconnected"), "err");
});

/* ============================================================
   侧栏实时时钟
   ============================================================ */
function updateSideClock() {
  const el = $("sideClock");
  if (!el) return;
  const now = new Date();
  const pad = n => String(n).padStart(2, "0");
  el.textContent = `${now.getFullYear()}-${pad(now.getMonth()+1)}-${pad(now.getDate())} ${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`;
}
updateSideClock();
setInterval(updateSideClock, 1000);
