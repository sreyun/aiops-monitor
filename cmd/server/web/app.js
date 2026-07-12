"use strict";

/* ============================================================
   AIOps Monitor · app.js — 主入口
   依赖：core.js → render.js → charts.js → terminal.js → auth.js → automation.js
   负责：告警设置、安装 Agent、自定义监控、消息中心、端口转发、事件委托
   ============================================================ */

/* ---------- 鍛婅璁剧疆 ---------- */
async function openSettings() {
  try {
    const c = await fetch(`${API}/config`).then(r => r.json());
    const t = c.thresholds || {};
    $("alertsEnabled").checked = !!c.alerts_enabled;
    $("feishuEnabled").checked = !!(c.feishu && c.feishu.enabled);
    $("feishuWebhook").value = (c.feishu && c.feishu.webhook) || "";
    $("dingEnabled").checked = !!(c.dingtalk && c.dingtalk.enabled);
    $("dingWebhook").value = (c.dingtalk && c.dingtalk.webhook) || "";
    $("dingSecret").value = (c.dingtalk && c.dingtalk.secret) || "";
    // Custom webhook
    const cw = c.custom_webhook || {};
    $("customWebhookEnabled").checked = !!cw.enabled;
    $("customWebhookURL").value = cw.url || "";
    $("customWebhookMethod").value = cw.method || "POST";
    $("customWebhookContentType").value = cw.content_type || "application/json";
    $("customWebhookHeaders").value = cw.headers || "";
    $("customWebhookBodyTemplate").value = cw.body_template || "";
    // SMTP email config
    const s = c.smtp || {};
    $("smtpEnabled").checked = !!s.smtp_enabled;
    $("smtpHost").value = s.smtp_host || "";
    $("smtpPort").value = s.smtp_port || "";
    $("smtpUsername").value = s.smtp_username || "";
    $("smtpPassword").value = s.smtp_password || "";
    $("smtpFromName").value = s.smtp_from_name || "";
    $("smtpTLS").checked = !!s.smtp_use_tls;
    // Threshold display: treat 0 / null / undefined as "unset" 鈫?show the standard
    // default. The backend also backfills these zeros, so display and storage stay
    // consistent, and every metric always shows a sane standard threshold.
    const td = (v, def) => (v == null || v === 0 || isNaN(v)) ? def : v;
    $("cpuWarn").value = td(t.cpu_warn, 80); $("cpuCrit").value = td(t.cpu_crit, 95);
    $("memWarn").value = td(t.mem_warn, 85); $("memCrit").value = td(t.mem_crit, 95);
    $("diskWarn").value = td(t.disk_warn, 80); $("diskCrit").value = td(t.disk_crit, 90);
    $("diskioWarn").value = td(t.diskio_warn, 80); $("diskioCrit").value = td(t.diskio_crit, 95);
    $("iopsWarn").value = td(t.iops_warn, 50000); $("iopsCrit").value = td(t.iops_crit, 100000);
    $("gpuWarn").value = td(t.gpu_warn, 80); $("gpuCrit").value = td(t.gpu_crit, 95);
    $("loadWarn").value = td(t.load_warn, 4.0); $("loadCrit").value = td(t.load_crit, 8.0);
    $("procWarn").value = td(t.proc_warn, 0.5);
    $("offlineSec").value = td(t.offline_after_sec, 60);

    // Reset to first tab
    switchNotifyTab("tab-feishu");

    $("settingsMask").classList.add("show");
  } catch (e) { toast(I18N.t("toast.read_config_failed") + e, "err"); }
}

// Tab switching for notification channels
function switchNotifyTab(tabId) {
  document.querySelectorAll("#notifyTabs .tab").forEach(btn => btn.classList.toggle("active", btn.dataset.tab === tabId));
  document.querySelectorAll("#settingsMask .tab-panel").forEach(p => p.classList.toggle("active", p.id === tabId));
}

function collectSettings() {
  const num = id => parseFloat($(id).value) || 0;
  return {
    alerts_enabled: $("alertsEnabled").checked,
    feishu: { enabled: $("feishuEnabled").checked, webhook: $("feishuWebhook").value.trim() },
    dingtalk: { enabled: $("dingEnabled").checked, webhook: $("dingWebhook").value.trim(), secret: $("dingSecret").value.trim() },
    custom_webhook: {
      enabled: $("customWebhookEnabled").checked,
      url: $("customWebhookURL").value.trim(),
      method: $("customWebhookMethod").value,
      content_type: $("customWebhookContentType").value.trim(),
      headers: $("customWebhookHeaders").value.trim(),
      body_template: $("customWebhookBodyTemplate").value.trim()
    },
    smtp: {
      smtp_enabled: $("smtpEnabled").checked,
      smtp_host: $("smtpHost").value.trim(),
      smtp_port: parseInt($("smtpPort").value) || 0,
      smtp_username: $("smtpUsername").value.trim(),
      smtp_password: $("smtpPassword").value,
      smtp_from_name: $("smtpFromName").value.trim(),
      smtp_use_tls: $("smtpTLS").checked
    },
    thresholds: {
      cpu_warn: num("cpuWarn"), cpu_crit: num("cpuCrit"),
      mem_warn: num("memWarn"), mem_crit: num("memCrit"),
      disk_warn: num("diskWarn"), disk_crit: num("diskCrit"),
      diskio_warn: num("diskioWarn"), diskio_crit: num("diskioCrit"),
      iops_warn: num("iopsWarn"), iops_crit: num("iopsCrit"),
      gpu_warn: num("gpuWarn"), gpu_crit: num("gpuCrit"),
      load_warn: num("loadWarn"), load_crit: num("loadCrit"),
      proc_warn: num("procWarn"),
      offline_after_sec: Math.round(num("offlineSec"))
    }
  };
}
async function saveSettings() {
  await withLoading("saveBtn", async () => {
    try {
      const r = await fetch(`${API}/config`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(collectSettings()) });
      if (r.ok) { toast(I18N.t("toast.config_saved"), "ok"); $("settingsMask").classList.remove("show"); } else { toast(I18N.t("toast.save_failed"), "err"); }
    } catch (e) { toast(I18N.t("toast.save_failed2") + e, "err"); }
  });
}
async function testSettings() {
  await withLoading("testBtn", async () => {
    try {
      const r = await fetch(`${API}/config/test`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(collectSettings()) });
      const j = await r.json();
      if (j.ok) toast(I18N.t("toast.test_sent"), "ok");
      else toast(I18N.t("toast.test_failed2") + (j.errors || []).join("; "), "err");
    } catch (e) { toast(I18N.t("toast.test_failed2") + e, "err"); }
  });
}

/* ---------- 瀹夎 Agent ---------- */
let INSTALL = { server_url: "", token: "" };
let CUR_OS = "linux";
let RELAY_MODE = false;
let MULTI_SERVER_MODE = false;
let TOKEN_REVEALED = false; // Token 鑴辨晱鐘舵€?
function maskToken(t) {
  if (!t) return "";
  if (TOKEN_REVEALED) return t;
  if (t.length <= 8) return "鈥⑩€⑩€⑩€⑩€⑩€⑩€⑩€?;
  return t.slice(0, 4) + "鈥⑩€⑩€⑩€⑩€⑩€⑩€⑩€? + t.slice(-4);
}
function updateTokenDisplay() {
  const el = $("installToken"); if (!el) return;
  el.value = maskToken(INSTALL.token || "");
  el.dataset.revealed = TOKEN_REVEALED ? "1" : "0";
}
async function openInstall() {
  try {
    INSTALL = await fetch(`${API}/install/info`).then(r => r.json());
    TOKEN_REVEALED = false;
    updateTokenDisplay();
    RELAY_MODE = false;
    MULTI_SERVER_MODE = false;
    const normalRadio = document.querySelector('input[name="installMode"][value="normal"]');
    if (normalRadio) normalRadio.checked = true;
    renderInstallCmd();
    $("installMask").classList.add("show");
  } catch (e) { toast(I18N.t("toast.read_install_failed") + e, "err"); }
}
function parseMultiServerList() {
  const text = ($("multiServerList") || {}).value || "";
  const lines = text.split("\n").map(l => l.trim()).filter(l => l);
  const servers = [];
  for (const line of lines) {
    const parts = line.split(/\s+/);
    const server = parts[0];
    const token = parts.slice(1).join(" ") || "";
    if (server) servers.push({ server, token });
  }
  return servers.length > 0 ? JSON.stringify(servers) : "";
}
function renderInstallCmd() {
  // Multi-server section visibility
  const msSection = $("multiServerSection");
  if (msSection) msSection.style.display = (MULTI_SERVER_MODE && !RELAY_MODE) ? "" : "none";
  // Relay mode: show gateway + internal commands, hide normal install
  if (RELAY_MODE) {
    $("normalInstallSection").style.display = "none";
    $("relaySection").style.display = "";
    renderRelayCmd();
    return;
  }
  $("normalInstallSection").style.display = "";
  $("relaySection").style.display = "none";
  const server = INSTALL.server_url || location.origin;
  const token = INSTALL.token || "";
  const cat = $("installCategory").value.trim();
  let q = "token=" + encodeURIComponent(token) + (cat ? "&category=" + encodeURIComponent(cat) : "");
  // Multi-server: append servers_json so the generated config.json uses a
  // servers array instead of a single server+token.
  let cmd, label, hint;
  if (MULTI_SERVER_MODE) {
    const sj = parseMultiServerList();
    if (sj) q += "&servers_json=" + encodeURIComponent(sj);
    label = I18N.t("install.multi_server");
    hint = I18N.t("install.multi_desc");
  }
  if (CUR_OS === "windows") {
    cmd = `irm "${server}/install.ps1?${q}" | iex`;
    label = I18N.t("install.powershell_cmd");
    hint = "鏅€?PowerShell 鍗冲彲锛涘畨瑁呭埌 %LOCALAPPDATA%\\AIOps-agent 骞舵敞鍐岀敤鎴风骇寮€鏈鸿嚜鍚€?;
  } else if (CUR_OS === "macos") {
    cmd = `curl -fsSL "${server}/install.sh?${q}" | sh`;
    label = I18N.t("install.terminal_one_line");
    hint = I18N.t("install.linux_detail");
  } else {
    cmd = `curl -fsSL "${server}/install.sh?${q}" | sudo sh`;
    label = I18N.t("install.linux_cmd");
    hint = I18N.t("install.linux_desc");
  }
  $("installCmd").textContent = cmd;
  $("cmdLabel").textContent = label;
  $("cmdHint").textContent = hint;
  $("uninstallCmd").textContent = (CUR_OS === "windows")
    ? `irm "${server}/uninstall.ps1" | iex`
    : `curl -fsSL "${server}/uninstall.sh" | ${CUR_OS === "macos" ? "sh" : "sudo sh"}`;
}
function renderRelayCmd() {
  const server = INSTALL.server_url || location.origin;
  const token = INSTALL.token || "";
  const cat = $("installCategory").value.trim();
  let q = "token=" + encodeURIComponent(token) + (cat ? "&category=" + encodeURIComponent(cat) : "");
  const gwIP = $("relayGatewayIP").value.trim() || I18N.t("install.gateway_ip");
  const relay = `http://${gwIP}:8529`;
  let gatewayCmd, internalCmd;
  if (CUR_OS === "windows") {
    gatewayCmd = `irm "${server}/install-relay.ps1?${q}" | iex`;
    internalCmd = `irm "${relay}/install.ps1?${q}" | iex`;
  } else if (CUR_OS === "macos") {
    gatewayCmd = `curl -fsSL "${server}/install-relay.sh?${q}" | sh`;
    internalCmd = `curl -fsSL "${relay}/install.sh?${q}" | sh`;
  } else {
    gatewayCmd = `curl -fsSL "${server}/install-relay.sh?${q}" | sudo sh`;
    internalCmd = `curl -fsSL "${relay}/install.sh?${q}" | sudo sh`;
  }
  $("relayGatewayCmd").textContent = gatewayCmd;
  $("relayInternalCmd").textContent = internalCmd;
  $("uninstallCmd").textContent = (CUR_OS === "windows")
    ? `irm "${server}/uninstall.ps1" | iex`
    : `curl -fsSL "${server}/uninstall.sh" | ${CUR_OS === "macos" ? "sh" : "sudo sh"}`;
}
async function resetToken() {
  if (!confirm(I18N.t("install.reset_warning"))) return;
  try {
    const j = await fetch(`${API}/install/reset-token`, { method: "POST" }).then(r => r.json());
    INSTALL.token = j.token; TOKEN_REVEALED = false; updateTokenDisplay(); renderInstallCmd();
    toast(I18N.t("toast.token_reset"), "ok");
  } catch (e) { toast(I18N.t("toast.reset_failed2") + e, "err"); }
}

/* ---------- 鑷畾涔夌洃鎺?---------- */
// 杩涚▼绫荤洰鏍囧舰濡?hostID/杩涚▼鍚嶏紝灞曠ず涓恒€岃繘绋?@ 涓绘満鍚嶃€嶆洿鍙嬪ソ銆?
function checkTargetDisplay(c) {
  if (c.type === "process") {
    const i = c.target.indexOf("/");
    if (i > 0) {
      const hid = c.target.slice(0, i), pname = c.target.slice(i + 1);
      const meta = HOST_META.find(h => h.id === hid);
      return pname + " @ " + (meta ? meta.hostname || hid.slice(0, 8) : hid.slice(0, 8));
    }
  }
  return c.target;
}
// TCP 鐩爣鎷嗗垎涓?涓绘満 / 绔彛锛堟湯涓啋鍙峰垎闅旓級
function splitHostPort(t) {
  t = String(t || "");
  const i = t.lastIndexOf(":");
  if (i > 0) return { host: t.slice(0, i), port: t.slice(i + 1) };
  return { host: t, port: "" };
}
// 杩涚▼鐩爣 hostID/杩涚▼鍚?鎷嗗垎锛屽苟鎶?hostID 瑙ｆ瀽涓轰富鏈哄悕
function splitProcessTarget(c) {
  const t = String(c.target || "");
  const i = t.indexOf("/");
  if (i > 0) {
    const hid = t.slice(0, i), proc = t.slice(i + 1);
    const meta = HOST_META.find(h => h.id === hid);
    return { proc, hostName: meta ? (meta.hostname || hid.slice(0, 8)) : hid.slice(0, 8) };
  }
  return { proc: t, hostName: "鈥? };
}
// 璇︽儏椤癸細閿?+ 鍊?+ 鍊奸厤鑹?
function cdItem(k, v, cls) {
  return `<div class="cd-item"><div class="cd-k">${k}</div><div class="cd-v ${cls || ""}" title="${esc(v)}">${esc(v)}</div></div>`;
}
function renderChecks(checks) {
  LAST_CHECKS = checks;
  const userChecks = checks.filter(c => !c.builtin);
  $("navChecks").textContent = userChecks.filter(c => !c.ok && c.checked_at).length || userChecks.length;
  const grid = $("checksGrid"), empty = $("checksEmpty");
  grid.className = "checks-list" + (CHECK_VIEW === "pill" ? " pill" : "");
  if (!userChecks.length && !checks.length) { grid.innerHTML = ""; empty.style.display = "block"; return; }
  empty.style.display = "none";

  // 搴旂敤绫诲瀷绛涢€?
  let shown = checks;
  if (CHECK_TYPE && CHECK_TYPE !== "all") shown = shown.filter(c => c.type === CHECK_TYPE);

  grid.innerHTML = shown.map(c => {
    const st = !c.enabled ? "unknown" : (c.checked_at ? (c.ok ? "up" : "down") : "unknown");
    const stText = !c.enabled ? I18N.t("ui.disabled_status") : (c.checked_at ? (c.ok ? I18N.t("ui.normal") : I18N.t("ui.abnormal")) : I18N.t("ui.pending"));
    const typeText = c.type === "http" ? "HTTP" : c.type === "tcp" ? "TCP" : c.type === "ping" ? "Ping" : I18N.t("ui.process");
    const builtin = c.builtin ? ' data-builtin="1"' : "";
    const histBtn = `<button class="mini-btn" data-cact="hist" title="${I18N.t('ui.history_chart')}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 3v18h18"/><path d="M7 13l3-3 3 2 5-6"/></svg></button>`;
    const actions = `<span class="ch-actions">${histBtn}${c.builtin ? "" : `
          <button class="mini-btn" data-cact="run" title="${I18N.t('ui.check_now')}">鈻?/button>
          <button class="mini-btn" data-cact="edit" title="${I18N.t('ui.edit')}">鉁?/button>
          <button class="mini-btn del" data-cact="del" title="${I18N.t('ui.delete')}">鉁?/button>`}</span>`;
    const builtinTag = c.builtin ? `<span class="type-badge" style="background:var(--ok-soft);color:var(--ok-txt)">${I18N.t("ui.builtin")}</span>` : "";

    // 璇︽儏瀛楁锛氭寜鐩戞帶绫诲瀷缁欏嚭鍚勮嚜璐村悎鐨勫瓧娈碉紝涓夌被鐩戞帶淇℃伅閲忓榻?
    const stCls = st === "up" ? "ok" : st === "down" ? "crit" : "muted";
    const lat = c.checked_at ? Math.round(c.latency_ms) + " ms" : "鈥?;
    const latCls = c.checked_at ? "" : "muted";
    const detail = [];
    if (c.type === "http") {
      detail.push(cdItem(I18N.t("form.check_url"), checkTargetDisplay(c), "muted"));
      detail.push(cdItem(I18N.t("form.run_status"), stText, stCls));
      const code = c.status_code || 0;
      detail.push(cdItem(I18N.t("form.status_code"), code ? String(code) : "鈥?, code === 0 ? "muted" : code >= 400 ? "crit" : "ok"));
      detail.push(cdItem(I18N.t("form.response_latency"), lat, latCls));
      if (typeof c.cert_days === "number" && c.cert_days >= 0) {
        const d = c.cert_days;
        detail.push(cdItem(I18N.t("form.cert_remaining"), d + I18N.t("time.days"), d <= 7 ? "crit" : d <= 30 ? "warn" : "ok"));
      }
    } else if (c.type === "tcp") {
      const hp = splitHostPort(c.target);
      detail.push(cdItem(I18N.t("form.target"), hp.host || c.target, "muted"));
      detail.push(cdItem(I18N.t("form.port"), hp.port || "鈥?, ""));
      detail.push(cdItem(I18N.t("form.connect_status"), stText, stCls));
      detail.push(cdItem(I18N.t("form.connect_latency"), lat, latCls));
    } else if (c.type === "ping") {
      detail.push(cdItem(I18N.t("form.check_url"), c.target, "muted"));
      detail.push(cdItem(I18N.t("form.run_status"), stText, stCls));
      const loss = (typeof c.loss_pct === "number" && c.loss_pct >= 0) ? c.loss_pct : null;
      detail.push(cdItem(I18N.t("form.loss_rate"), loss === null ? "鈥? : Math.round(loss) + "%",
        loss === null ? "muted" : loss === 0 ? "ok" : loss >= 100 ? "crit" : "warn"));
      const hasRtt = c.checked_at && c.latency_ms > 0;
      detail.push(cdItem(I18N.t("form.avg_latency"), hasRtt ? Math.round(c.latency_ms) + " ms" : "鈥?, hasRtt ? "" : "muted"));
    } else if (c.type === "process") {
      const pr = splitProcessTarget(c);
      detail.push(cdItem(I18N.t("form.process_name2"), pr.proc, ""));
      detail.push(cdItem(I18N.t("form.target_host2"), pr.hostName, "muted"));
      detail.push(cdItem(I18N.t("form.run_status"), stText, stCls));
      detail.push(cdItem(I18N.t("form.check_duration"), lat, latCls));
    } else {
      detail.push(cdItem(I18N.t("form.check_url"), checkTargetDisplay(c), "muted"));
      detail.push(cdItem(I18N.t("form.run_status"), stText, stCls));
      detail.push(cdItem(I18N.t("form.latency"), lat, latCls));
    }
    detail.push(cdItem(I18N.t("form.check_interval"), I18N.t("section.every") + c.interval_sec + "s", "muted"));
    detail.push(cdItem(I18N.t("form.last_check"), c.checked_at ? ago(c.checked_at) : I18N.t("ui.not_checked"), "muted"));

    return `<div class="check-card st-${st}" data-id="${esc(c.id)}"${builtin}>
      <div class="check-row-top">
        <span class="st-dot ${st}"></span>
        <span class="ch-name" title="${esc(c.name)}">${esc(c.name)}</span>
        <span class="type-badge t-${esc(c.type)}">${typeText}</span>
        ${builtinTag}
        <span class="st-pill ${st}">${stText}</span>
        ${actions}
      </div>
      <div class="check-detail">${detail.join("")}</div>
      ${(!c.ok && c.checked_at) ? `<div class="check-err">${esc(c.message)}</div>` : ""}
    </div>`;
  }).join("");
}
// 鍒楄〃 / 鑳跺泭瑙嗗浘鍒囨崲
function setCheckView(v) {
  CHECK_VIEW = v === "pill" ? "pill" : "list";
  try { localStorage.setItem("aiops_check_view", CHECK_VIEW); } catch (e) {}
  document.querySelectorAll("#checkViewToggle .vt-btn").forEach(b => b.classList.toggle("active", b.dataset.cview === CHECK_VIEW));
  renderChecks(LAST_CHECKS);
}
// 涓绘満 鍗＄墖 / 鍒楄〃 瑙嗗浘鍒囨崲
function setHostView(v) {
  HOST_VIEW = v === "list" ? "list" : "card";
  try { localStorage.setItem("aiops_host_view", HOST_VIEW); } catch (e) {}
  document.querySelectorAll("#hostViewToggle .vt-btn").forEach(b => b.classList.toggle("active", b.dataset.hview === HOST_VIEW));
  HOST_PAGE = 1;
  renderHosts(LAST_HOSTS);
}
async function loadChecks() {
  try { renderChecks(await fetch(`${API}/checks`).then(r => r.json())); } catch (e) { /* ignore */ }
}

let CHK_CHARTS = {};
let CHK_HIST = { id: "", name: "", type: "", range: 24 }; // range=灏忔椂鏁帮紝榛樿 24h
// 鑷畾涔夌洃鎺峰巻鍙叉洸绾匡細澶嶇敤浜や簰寮忓浘琛ㄥ紩鎿庯紝鏀寔鎸夋椂闂磋寖鍥寸瓫閫夛紙涓庝富鏈鸿秼鍔垮浘涓€鑷达級
function openCheckHistory(id, name, type) {
  CHK_HIST = { id, name, type, range: 24 };
  $("checkHistTitle").textContent = name + " 路 鐩戞帶鍘嗗彶";
  $("checkHistMask").classList.add("show");
  loadCheckHistory();
}
async function loadCheckHistory() {
  const { id, name, type, range } = CHK_HIST;
  const body = $("checkHistBody");
  body.innerHTML = `<div class="empty-line">鍔犺浇涓€?/div>`;
  const ctrl = renderChartControls(range, "crange");
  try {
    const all = await fetch(`${API}/checks/${encodeURIComponent(id)}/history`).then(r => r.json());
    const now = Math.floor(Date.now() / 1000);
    const from = range > 0 ? now - range * 3600 : 0;
    const pts = (Array.isArray(all) ? all : []).filter(p => p.timestamp >= from);
    if (!pts.length) {
      body.innerHTML = `<div class="chart-controls">${ctrl}</div><div class="empty-line">璇ユ椂闂磋寖鍥存殏鏃犳暟鎹紙妫€鏌ヨ繍琛屼竴娈垫椂闂村悗鑷姩绉疮锛岄噸鍚悗閲嶆柊璁★級</div>`;
      return;
    }
    const samples = pts.map(p => ({ timestamp: p.timestamp, latency_ms: p.latency_ms, loss_pct: (typeof p.loss_pct === "number" ? p.loss_pct : null), ok: p.ok }));
    const isPing = type === "ping";
    const uptime = (pts.filter(p => p.ok).length / pts.length * 100).toFixed(1);
    const avgLat = (pts.reduce((s, p) => s + (p.latency_ms || 0), 0) / pts.length).toFixed(0);
    const span = pts.length > 1 ? fmtDur(pts[pts.length - 1].timestamp - pts[0].timestamp) : I18N.t("time.just_now");
    const wrap = cid => `<div class="chart-wrap"><canvas id="${cid}" width="1000" height="240"></canvas>` +
      `<button class="chart-enlarge" data-chart="${cid}" title="${I18N.t('ui.zoom_preview')}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg></button></div>`;
    body.innerHTML = `<div class="chart-controls">${ctrl}</div>
      <div class="chart-container">${wrap("chkLat")}${isPing ? wrap("chkLoss") : ""}</div>
      <div class="hint">閲囨牱 ${pts.length} 涓?路 鏃堕棿璺ㄥ害 ${span} 路 鍙敤鐜?${uptime}% 路 骞冲潎寤舵椂 ${avgLat} ${I18N.t("unit.ms")} 路 鎮仠鏌ョ湅鏁板€硷紝鎷栧姩妗嗛€夋斁澶э紝鍙屽嚮杩樺師銆?/div>`;
    CHK_CHARTS = {};
    CHK_CHARTS.chkLat = createChart("chkLat", samples, [
      { key: "latency_ms", label: isPing ? I18N.t("form.avg_latency") : I18N.t("form.latency"), color: "#4c8dff", fmt: v => v.toFixed(0) + " " + I18N.t("unit.ms") },
    ], 0, null, { title: name + " 路 " + I18N.t("form.latency") + "(" + I18N.t("unit.ms") + ")" });
    if (isPing) {
      CHK_CHARTS.chkLoss = createChart("chkLoss", samples, [
        { key: "loss_pct", label: I18N.t("form.loss_rate"), color: "#f2545b", fmt: v => v.toFixed(0) + "%" },
      ], 0, 100, { title: name + " 路 涓㈠寘鐜?%)" });
    }
  } catch (e) {
    body.innerHTML = `<div class="empty-line">鍔犺浇澶辫触: ${esc(e)}</div>`;
  }
}
// 鍘嗗彶寮圭獥锛氭椂闂磋寖鍥村垏鎹?+ 鍥捐〃鏀惧ぇ濮旀墭
safeAddEventListener("checkHistBody", "click", e => {
  const rb = e.target.closest(".chip-btn[data-crange]");
  if (rb) { CHK_HIST.range = parseInt(rb.dataset.crange); loadCheckHistory(); return; }
  const en = e.target.closest(".chart-enlarge"); if (!en) return;
  const ch = CHK_CHARTS[en.dataset.chart]; if (ch) openChartZoom(ch);
});
async function loadHostsMeta() {
  try { HOST_META = await fetch(`${API}/hosts/meta`).then(r => r.json()); } catch (e) { /* ignore */ }
}
function updateCkTargetLabel() {
  const t = $("ckType").value;
  if (t === "process") {
    $("ckHostField").style.display = "block";
    $("ckTargetLabel").textContent = I18N.t("form.process_name");
    $("ckTarget").placeholder = I18N.t("form.hint_process");
    return;
  }
  $("ckHostField").style.display = "none";
  if (t === "http") {
    $("ckTargetLabel").textContent = I18N.t("form.url");
    $("ckTarget").placeholder = "https://example.com";
  } else if (t === "ping") {
    $("ckTargetLabel").textContent = I18N.t("form.host_addr");
    $("ckTarget").placeholder = I18N.t("form.hint_url");
  } else {
    $("ckTargetLabel").textContent = I18N.t("form.host_port");
    $("ckTarget").placeholder = "127.0.0.1:3306";
  }
}
function openCheckModal(check) {
  $("checkModalTitle").textContent = check ? I18N.t("ui.edit_check") : I18N.t("ui.add_check");
  $("ckId").value = check ? check.id : "";
  $("ckName").value = check ? check.name : "";
  $("ckType").value = check ? check.type : "http";
  // For process type, extract process name only (not "hostID/procName")
  if (check && check.type === "process") {
    const idx = check.target.indexOf("/");
    $("ckTarget").value = idx > 0 ? check.target.slice(idx + 1) : check.target;
  } else {
    $("ckTarget").value = check ? check.target : "";
  }
  $("ckInterval").value = check ? check.interval_sec : 30;
  $("ckLevel").value = check ? check.level : "critical";
  $("ckEnabled").checked = check ? check.enabled : true;
  // Populate host select for process type
  populateHostSelect(check);
  updateCkTargetLabel();
  $("checkMask").classList.add("show");
}
function populateHostSelect(check) {
  const sel = $("ckHost");
  sel.innerHTML = `<option value="">-- 閫夋嫨涓绘満 --</option>` + HOST_META.map(h =>
    `<option value="${esc(h.id)}" ${check && check.target.startsWith(h.id + "/") ? "selected" : ""}>${esc(h.hostname || h.id)}</option>`
  ).join("");
}
async function saveCheck() {
  let target = $("ckTarget").value.trim();
  const type = $("ckType").value;
  if (type === "process") {
    const hostId = $("ckHost").value;
    if (!hostId) { toast(I18N.t("valid.select_host"), "err"); return; }
    if (!target) { toast(I18N.t("valid.fill_process"), "err"); return; }
    target = hostId + "/" + target;
  }
  const body = {
    id: $("ckId").value,
    name: $("ckName").value.trim(),
    type: type,
    target: target,
    interval_sec: Math.max(5, parseInt($("ckInterval").value) || 30),
    level: $("ckLevel").value,
    enabled: $("ckEnabled").checked
  };
  if (!body.name || !body.target) { toast(I18N.t("valid.fill_name_target"), "err"); return; }
  await withLoading("ckSaveBtn", async () => {
    try {
      const r = await fetch(`${API}/checks`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      if (r.ok) { toast(I18N.t("toast.saved"), "ok"); $("checkMask").classList.remove("show"); loadChecks(); }
      else { const j = await r.json(); toast(I18N.t("toast.save_failed2") + (j.error || ""), "err"); }
    } catch (e) { toast(I18N.t("toast.save_failed2") + e, "err"); }
  });
}
async function delCheck(id) {
  if (!confirm(I18N.t("valid.confirm_delete_check"))) return;
  try {
    const r = await fetch(`${API}/checks/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (r.ok) { toast(I18N.t("toast.deleted"), "ok"); loadChecks(); } else { toast(I18N.t("toast.delete_failed"), "err"); }
  } catch (e) { toast(I18N.t("toast.deleted") + ": " + e, "err"); }
}

/* ---------- 娑堟伅涓績锛堥《鏍忛搩閾?+ 鏈寰芥爣 + 涓嬫媺闈㈡澘锛?---------- */
let MSG_POLL = null;
function msgEsc(s){return String(s==null?"":s).replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));}
function initMsgCenter() {
  const panel = $("notifPanel"), wrap = $("notifWrap");
  if (!$("notifBtn") || !panel) return;
  safeAddEventListener("notifBtn", "click", (e) => {
    e.stopPropagation();
    const open = panel.classList.toggle("show");
    if (open) loadMessages();
  });
  safeAddEventListener("notifReadAll", "click", async (e) => {
    e.stopPropagation();
    try { await fetch(`${API}/messages/read-all`, { method: "POST" }); } catch (_) {}
    loadMessages();
  });
  document.addEventListener("click", (e) => { if (wrap && !wrap.contains(e.target)) panel.classList.remove("show"); });
  loadMessages();
  if (MSG_POLL) clearInterval(MSG_POLL);
  MSG_POLL = setInterval(loadMessages, 20000);
}
async function loadMessages() {
  try {
    const data = await fetch(`${API}/messages?limit=50`).then(r => r.json());
    const msgs = data.messages || [];
    const unread = data.unread || 0;
    const badge = $("notifBadge");
    if (badge) {
      if (unread > 0) { badge.textContent = unread > 99 ? "99+" : unread; badge.style.display = ""; }
      else badge.style.display = "none";
    }
    renderMessages(msgs);
  } catch (_) {}
}
function renderMessages(msgs) {
  const list = $("notifList"), empty = $("notifEmpty");
  if (!list) return;
  if (empty) empty.style.display = msgs.length ? "none" : "";
  const pad = n => String(n).padStart(2, "0");
  list.innerHTML = msgs.map(m => {
    const t = new Date((m.ts || 0) * 1000);
    const ts = `${t.getMonth()+1}-${pad(t.getDate())} ${pad(t.getHours())}:${pad(t.getMinutes())}`;
    return `<div class="notif-item ${m.read ? "" : "unread"}" data-id="${m.id}" data-view="${msgEsc(m.view || "")}">
      <span class="notif-dot ${msgEsc(m.level || "info")}"></span>
      <div class="notif-body">
        <div class="notif-title">${msgEsc(m.title || "")}</div>
        ${m.body ? `<div class="notif-sub">${msgEsc(m.body)}</div>` : ""}
        <div class="notif-time">${ts}</div>
      </div></div>`;
  }).join("");
  list.querySelectorAll(".notif-item").forEach(el => {
    el.addEventListener("click", async () => {
      const id = parseInt(el.dataset.id, 10);
      const view = el.dataset.view;
      try { await fetch(`${API}/messages/read`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ ids: [id] }) }); } catch (_) {}
      const p = $("notifPanel"); if (p) p.classList.remove("show");
      if (view && typeof switchView === "function") switchView(view);
      loadMessages();
    });
  });
}
function startApp() {
  if (APP_STARTED) return;
  APP_STARTED = true;
  initTheme();
  initNotifications();
  initMsgCenter();
  showSkeleton();
  refresh(); loadChecks();
  // P1-2: 宸紓鍖栬疆璇㈤鐜?鈥?鎸夊綋鍓嶈鍥?+ 鏍囩椤靛彲瑙佹€ц皟鏁村埛鏂伴棿闅?
  const POLL_BASE = 3000;
  let pollTimer = null;
  function schedulePoll() {
    if (pollTimer) clearTimeout(pollTimer);
    const view = document.querySelector(".view.active")?.id.replace("view-", "") || "overview";
    const intervals = { overview: 3000, hosts: 5000, checks: 10000, alerts: 3000, automation: 15000, forward: 15000, log: 10000 };
    let interval = intervals[view] || POLL_BASE;
    // 鍚庡彴鏍囩椤甸檷棰戣嚦 15s锛屽噺灏戜笉蹇呰鐨勭綉缁滆姹傚拰 DOM 娓叉煋
    if (document.visibilityState === "hidden") interval = Math.max(interval, 15000);
    pollTimer = setTimeout(() => { refresh(); loadChecks(); if (document.querySelector("#view-forward.active")) loadForwards(); schedulePoll(); }, interval);
  }
  schedulePoll();
  // 瑙嗗浘鍒囨崲鏃剁珛鍗宠皟鏁磋疆璇㈤鐜?
  document.querySelectorAll(".nav-item").forEach(n => n.addEventListener("click", () => setTimeout(schedulePoll, 100)));
  // 鏍囩椤靛彲瑙佹€у彉鍖栨椂閲嶆帓杞
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") { refresh(true); schedulePoll(); }
  });
  // P3-1: 鍒濆鍖?WebSocket 鎺ㄩ€侊紙甯﹂檷绾у埌杞锛?
  initPushWS();
}
/* ---------- 涓诲惊鐜?---------- */
function updateFavicon(critCount) {
  const canvas = document.createElement("canvas");
  canvas.width = 16; canvas.height = 16;
  const ctx = canvas.getContext("2d");
  ctx.fillStyle = "#0a0d13"; ctx.fillRect(0, 0, 16, 16);
  ctx.fillStyle = "#4c8dff"; ctx.font = "bold 9px sans-serif"; ctx.textAlign = "center"; ctx.textBaseline = "middle";
  ctx.fillText("AI", 8, 8.5);
  if (critCount > 0) {
    ctx.fillStyle = "#ef4d5a"; ctx.beginPath(); ctx.arc(13, 3, 3.5, 0, Math.PI * 2); ctx.fill();
    ctx.fillStyle = "#fff"; ctx.font = "bold 5px sans-serif"; ctx.fillText(critCount > 9 ? "9+" : String(critCount), 13, 3.5);
  }
  let link = document.querySelector("link[rel=icon]");
  if (!link) { link = document.createElement("link"); link.rel = "icon"; document.head.appendChild(link); }
  link.href = canvas.toDataURL();
}
async function refresh(force) {
  if (PAUSED && !force) return;
  try {
    const rs = await fetch(`${API}/summary`);
    if (rs.status === 401) { $("loginView").classList.add("show"); return; }
    const s = await rs.json();
    const [hosts, alerts, activity] = await Promise.all([
      fetch(`${API}/hosts`).then(r => r.json()),
      fetch(`${API}/alerts`).then(r => r.json()),
      fetch(`${API}/activity`).then(r => r.json())
    ]);
    // P0-#2: Connection state feedback
    if (FIRST_LOAD) { FIRST_LOAD = false; }
    if (CONN_STATE !== "connected") {
      if (CONN_STATE === "disconnected") toast(I18N.t("toast.reconnected"), "ok");
      CONN_STATE = "connected";
    }
    // Filter hosts by category for overview
    const filteredHosts = CUR_CATS.length > 0
      ? hosts.filter(h => CUR_CATS.includes(h.category || I18N.t("section.uncategorized")))
      : hosts;
    // Compute overview stats from filtered hosts
    if (CUR_CATS.length > 0) {
      let online = 0;
      filteredHosts.forEach(h => { if (h.online) online++; });
      const filteredAlerts = alerts.filter(a => !a.host_id || filteredHosts.some(h => h.id === a.host_id));
      s.total_hosts = filteredHosts.length;
      s.online_hosts = online;
      s.offline_hosts = filteredHosts.length - online;
      s.critical_alerts = filteredAlerts.filter(a => a.level === "critical").length;
      s.warning_alerts = filteredAlerts.filter(a => a.level !== "critical").length;
    }
    renderCards(s); renderStatsHealth(s); renderAlerts(alerts); renderLog(activity); renderHosts(hosts); renderTop(CUR_CATS.length > 0 ? filteredHosts : hosts);
    updateFavicon(s.critical_alerts || 0);
    notifyCriticalAlerts(s.critical_alerts || 0);
    const pulseEl = $("pulse"); if (pulseEl) pulseEl.className = "pulse";
  } catch (e) {
    const pulseEl = $("pulse"); if (pulseEl) pulseEl.className = "pulse off";
    if (CONN_STATE === "connected") {
      CONN_STATE = "disconnected";
      toast(I18N.t("ui.reconnecting"), "err");
    }
  }
}

/* ---------- 浜嬩欢缁戝畾锛堝鎵橈級 ---------- */
const groupsEl = $("groups");
// 鎶樺彔鍔熻兘宸蹭复鏃跺仠鐢細绉婚櫎 group-head 鐐瑰嚮浜嬩欢濮旀墭涓殑 toggleCatCollapse 閫昏緫
if (groupsEl) {
  groupsEl.addEventListener("click", e => {
    const host = e.target.closest(".host"); if (!host) return;
    const act = e.target.closest("[data-act]");
    const { id, name, cat } = host.dataset;
    if (act) {
      if (act.dataset.act === "detail") openDetail(id, name);
      else if (act.dataset.act === "cat") editCategory(id, cat);
      else if (act.dataset.act === "del") delHost(id, name);
      else if (act.dataset.act === "term") openTerminal(id, name);
    } else {
      // 鐐瑰嚮涓绘満鍗＄墖/琛屽唴浠绘剰闈炴搷浣滄寜閽尯鍩燂紙杩涘害鏉°€佽礋杞姐€佸簳閮ㄧ瓑锛夆啋 鎵撳紑璇︽儏
      openDetail(id, name);
    }
  });
}

/* ---------- Multi-select category dropdown ---------- */
function getSelectedCats() {
  try { const s = localStorage.getItem("aiops_cats"); if (s) return JSON.parse(s); } catch (e) {}
  return [];
}
function setSelectedCats(arr) {
  CUR_CATS = arr;
  try { localStorage.setItem("aiops_cats", JSON.stringify(arr)); } catch (e) {}
}
function catCollapsed(cat) {
  try { const s = localStorage.getItem("aiops_collapsed"); if (s) { const arr = JSON.parse(s); return arr.includes(cat); } } catch (e) {}
  return false;
}
function toggleCatCollapse(cat) {
  let arr = [];
  try { const s = localStorage.getItem("aiops_collapsed"); if (s) arr = JSON.parse(s); } catch (e) {}
  const i = arr.indexOf(cat);
  if (i >= 0) arr.splice(i, 1); else arr.push(cat);
  try { localStorage.setItem("aiops_collapsed", JSON.stringify(arr)); } catch (e) {}
}
function renderCatDropdown(cats) {
  const wrap = $("catDropdownWrap") ? $("catDropdownWrap").parentElement : null;
  if (!wrap) return; // 鍒嗙被绛涢€夊凡绉婚櫎
  if (wrap.querySelector(".cat-dropdown")) {
    // Update options in existing dropdown
    updateCatDropdownOptions(cats);
    return;
  }
  // Remove native select
  const oldSel = $("catFilter");
  if (oldSel) oldSel.remove();
  wrap.innerHTML = `<div class="cat-dropdown" id="catDropdownWrap">
    <button class="cat-dd-btn" id="catDropdownBtn"><span id="catDropdownLabel">${I18N.t("ui.all_categories")}</span> <span class="dd-arrow">鈻?/span></button>
    <div class="cat-dd-menu" id="catDropdownMenu"></div>
  </div>`;
  updateCatDropdownOptions(cats);
  // Toggle menu
  $("catDropdownBtn").addEventListener("click", e => {
    e.stopPropagation();
    $("catDropdownMenu").classList.toggle("show");
  });
  document.addEventListener("click", e => {
    const menu = $("catDropdownMenu");
    if (menu && !e.target.closest(".cat-dropdown")) menu.classList.remove("show");
  });
}
function updateCatDropdownOptions(cats) {
  const menu = $("catDropdownMenu");
  if (!menu) return;
  // Clean stale selections
  CUR_CATS = CUR_CATS.filter(c => cats.includes(c));
  // P0-#1: Only rebuild DOM when category list actually changes
  const newKey = cats.join("\u0001");
  if (newKey === LAST_CATS_KEY) {
    // Same categories: just sync checkbox states without rebuilding DOM
    menu.querySelectorAll("input").forEach(inp => {
      if (inp.value === "") inp.checked = CUR_CATS.length === 0;
      else inp.checked = CUR_CATS.includes(inp.value);
    });
    updateCatDropdownLabel();
    return;
  }
  LAST_CATS_KEY = newKey;
  menu.innerHTML = `<label class="cat-dd-opt"><input type="checkbox" value="" ${CUR_CATS.length === 0 ? "checked" : ""}> 鍏ㄩ儴鍒嗙被</label>` +
    cats.map(c => `<label class="cat-dd-opt"><input type="checkbox" value="${esc(c)}" ${CUR_CATS.includes(c) ? "checked" : ""}> ${esc(c)}</label>`).join("");
  menu.querySelectorAll("input").forEach(inp => {
    inp.addEventListener("change", () => {
      if (inp.value === "") {
        if (inp.checked) {
          menu.querySelectorAll("input").forEach(x => { if (x !== inp) x.checked = false; });
          setSelectedCats([]);
        }
      } else {
        menu.querySelector('input[value=""]').checked = false;
        const selected = [...menu.querySelectorAll('input:checked')].map(x => x.value).filter(v => v !== "");
        setSelectedCats(selected);
      }
      HOST_PAGE = 1;
      updateCatDropdownLabel();
      renderHosts(LAST_HOSTS);
    });
  });
  updateCatDropdownLabel();
}
function updateCatDropdownLabel() {
  const label = $("catDropdownLabel");
  if (!label) return;
  const btn = $("catDropdownBtn");
  if (CUR_CATS.length === 0) {
    label.textContent = I18N.t("section.all_categories");
    if (btn) btn.classList.remove("filtered");
  } else {
    if (CUR_CATS.length <= 2) label.textContent = CUR_CATS.join(", ");
    else label.textContent = CUR_CATS.length + " 涓垎绫?;
    if (btn) btn.classList.add("filtered");
  }
}

/* ---------- Host filters ---------- */
function filterHosts(value) {
  HOST_FILTER = value;
  HOST_PAGE = 1;
  renderHosts(LAST_HOSTS);
}

function sortHosts(value) {
  HOST_SORT = value;
  HOST_PAGE = 1;
  renderHosts(LAST_HOSTS);
}

// 鏆傚仠 / 鎭㈠鑷姩鍒锋柊
function togglePause() {
  PAUSED = !PAUSED;
  const btn = $("pauseBtn");
  if (btn) { btn.classList.toggle("active", PAUSED); btn.title = PAUSED ? I18N.t("toast.paused_click") : I18N.t("ui.pause_refresh"); }
  const pulseEl = $("pulse"); if (pulseEl) pulseEl.className = PAUSED ? "pulse paused" : "pulse";
  toast(PAUSED ? I18N.t("toast.paused") : I18N.t("toast.resumed"), "ok");
  if (!PAUSED) refresh(true);
}

// 涓€閿竻鐞嗘墍鏈夌绾夸富鏈?
async function purgeOffline() {
  const off = LAST_HOSTS.filter(h => !h.online);
  if (!off.length) { toast(I18N.t("empty.no_offline_hosts"), "ok"); return; }
  if (!confirm(`纭娓呯悊 ${off.length} 鍙扮绾夸富鏈猴紵\n鑻ュ叾 Agent 浠嶅湪杩愯锛岀害 60 绉掑悗浼氶噸鏂板嚭鐜般€俙)) return;
  let ok = 0;
  for (const h of off) {
    try { const r = await fetch(`${API}/hosts/${encodeURIComponent(h.id)}`, { method: "DELETE" }); if (r.ok) ok++; } catch (e) { /* skip */ }
  }
  toast(I18N.t("toast.cleaned") + ok + I18N.t("toast.hosts_cleaned"), "ok");
  refresh(true);
}

// Helper function to safely add event listeners
function safeAddEventListener(id, event, handler) {
  const el = $(id);
  if (el) {
    el.addEventListener(event, handler);
  } else {
    console.warn(`Element with id "${id}" not found`);
  }
}

// 娉細settingsBtn / themeToggle / topbarThemeBtn 宸茬Щ鍏ュ彸涓婅鐢ㄦ埛涓嬫媺鑿滃崟
// 渚ф爮鑿滃崟
safeAddEventListener("saveBtn", "click", saveSettings);
safeAddEventListener("testBtn", "click", testSettings);
safeAddEventListener("installBtn", "click", openInstall);
safeAddEventListener("resetTokenBtn", "click", resetToken);
safeAddEventListener("tokenToggleBtn", "click", function() {
  TOKEN_REVEALED = !TOKEN_REVEALED;
  updateTokenDisplay();
  this.title = TOKEN_REVEALED ? I18N.t("ui.hide_token") : I18N.t("ui.show_token");
});
safeAddEventListener("copyCmdBtn", "click", function() {
  copyWithFeedback(this, $("installCmd").textContent, I18N.t("toast.copy_install"));
});
// 鐐瑰嚮鍛戒护鍖哄煙鏈韩涔熷彲澶嶅埗
safeAddEventListener("installCmd", "click", function() {
  const sel = window.getSelection();
  sel.removeAllRanges();
  const range = document.createRange();
  range.selectNodeContents(this);
  sel.addRange(range);
});
safeAddEventListener("installCategory", "input", renderInstallCmd);
safeAddEventListener("osTabs", "click", e => {
  const t = e.target.closest(".tab"); if (!t) return;
  CUR_OS = t.dataset.os;
  document.querySelectorAll("#osTabs .tab").forEach(x => x.classList.toggle("active", x === t));
  renderInstallCmd();
});
safeAddEventListener("copyUninstallBtn", "click", function() {
  copyWithFeedback(this, $("uninstallCmd").textContent, I18N.t("toast.copy_uninstall"));
});
// 瀹夎妯″紡鍒囨崲锛坮adio buttons锛?
document.querySelectorAll('input[name="installMode"]').forEach(r => {
  r.addEventListener("change", function() {
    RELAY_MODE = (this.value === "relay");
    MULTI_SERVER_MODE = (this.value === "multi");
    renderInstallCmd();
  });
});
// 澶氭湇鍔＄鎺ㄩ€佸垪琛ㄥ彉鏇?
safeAddEventListener("multiServerList", "input", renderInstallCmd);
safeAddEventListener("relayGatewayIP", "input", renderInstallCmd);
safeAddEventListener("copyRelayGatewayBtn", "click", function() {
  copyWithFeedback(this, $("relayGatewayCmd").textContent, I18N.t("toast.copy_relay_install"));
});
safeAddEventListener("copyRelayInternalBtn", "click", function() {
  copyWithFeedback(this, $("relayInternalCmd").textContent, I18N.t("toast.copy_intranet_install"));
});

// 鍛婅鎿嶄綔鎸夐挳浜嬩欢濮旀墭锛堢‘璁?/ 闈欓粯 / 娓呴櫎鐘舵€侊級
document.addEventListener("click", async function(e) {
  const btn = e.target.closest(".alert-action");
  if (!btn) return;
  e.preventDefault();
  e.stopPropagation();
  const action = btn.dataset.action;
  const body = JSON.stringify({
    host_id: btn.dataset.host || "",
    type: btn.dataset.type || "",
    scope: btn.dataset.scope || ""
  });
  try {
    const r = await fetch(`${API}/alerts/${action}`, { method: "POST", headers: { "Content-Type": "application/json" }, body });
    if (r.ok) {
      toast(action === "ack" ? I18N.t("toast.alert_ack") : action === "silence" ? I18N.t("toast.alert_silence") : I18N.t("toast.alert_cleared"), "ok");
      await refresh(true);
    } else {
      toast(I18N.t("toast.operation_failed"), "err");
    }
  } catch (e) { toast(I18N.t("toast.network_error"), "err"); }
});

/* ---------- 渚ф爮瀵艰埅锛氳鍥惧垏鎹?+ 鏀惰捣 + 绉诲姩鎶藉眽 ---------- */
const navItems = document.querySelectorAll(".nav-item");
// 椤甸潰澶村厓淇℃伅锛氭爣棰?+ 鍓爣棰樸€傚壇鏍囬璁╅《鏍忛〉闈㈠ご鎵胯浇鈥滈〉闈㈣涔夆€濓紝
// 鑰岄潪鏈烘鍥炴樉渚ф爮瀵艰埅鍚嶏紝浠庢牴涓婃秷闄も€滀袱涓瑙堚€濈殑閲嶅瑙傛劅銆?
const PAGE_META = {
  overview: { title: I18N.t("ui.overview"), sub: I18N.t("section.overview_desc") },
  hosts:    { title: I18N.t("nav.hosts"), sub: I18N.t("section.hosts_desc") },
  alerts:   { title: I18N.t("ui.alerts"), sub: I18N.t("section.alerts_desc") },
  checks:   { title: I18N.t("ui.checks"), sub: I18N.t("section.checks_desc") },
  automation: { title: I18N.t("ui.automation"), sub: I18N.t("section.automation_desc") },
  forward:  { title: I18N.t("section.port_forward"), sub: I18N.t("section.forward_desc") },
  sre:      { title: I18N.t("section.sre"), sub: I18N.t("section.sre_desc") },
  logs:     { title: I18N.t("section.logs"), sub: I18N.t("section.logs_desc") },
  log:      { title: I18N.t("ui.log"), sub: I18N.t("section.log_desc") },
};
// Rebuild the JS-baked page-meta strings in the current language (called on
// i18n:changed so titles/subtitles follow an in-place language switch).
function rebuildPageMeta() {
  PAGE_META.overview   = { title: I18N.t("ui.overview"), sub: I18N.t("section.overview_desc") };
  PAGE_META.hosts      = { title: I18N.t("nav.hosts"), sub: I18N.t("section.hosts_desc") };
  PAGE_META.alerts     = { title: I18N.t("ui.alerts"), sub: I18N.t("section.alerts_desc") };
  PAGE_META.checks     = { title: I18N.t("ui.checks"), sub: I18N.t("section.checks_desc") };
  PAGE_META.automation = { title: I18N.t("ui.automation"), sub: I18N.t("section.automation_desc") };
  PAGE_META.forward    = { title: I18N.t("section.port_forward"), sub: I18N.t("section.forward_desc") };
  PAGE_META.log        = { title: I18N.t("ui.log"), sub: I18N.t("section.log_desc") };
}
function switchView(view) {
  document.querySelectorAll(".view").forEach(v => v.classList.toggle("active", v.id === "view-" + view));
  navItems.forEach(n => n.classList.toggle("active", n.dataset.view === view));
  const meta = PAGE_META[view];
  if (meta) {
    const t = $("pageTitle"), s = $("pageSub");
    if (t) t.textContent = meta.title;
    if (s) s.textContent = meta.sub;
  }
  if (view === "automation") loadPlaybooks();
  if (view === "forward") loadForwards();
  if (view === "sre") loadSRE();
  if (view === "logs") loadLogs();
  window.scrollTo(0, 0);
}
navItems.forEach(n => n.addEventListener("click", () => {
  switchView(n.dataset.view);
  const appEl = $("app");
  if (appEl) appEl.classList.remove("nav-open");
}));

// Collapsible nav groups (Nightingale-style); collapsed state persists per group.
(function initNavGroups(){
  let collapsed = {};
  try { collapsed = JSON.parse(localStorage.getItem("aiops_nav_collapsed")||"{}"); } catch(e){}
  document.querySelectorAll(".nav-group").forEach(g => {
    const key = g.dataset.group;
    if (collapsed[key]) g.classList.add("collapsed");
    const label = g.querySelector(".nav-group-label");
    if (label) label.addEventListener("click", () => {
      g.classList.toggle("collapsed");
      collapsed[key] = g.classList.contains("collapsed");
      try { localStorage.setItem("aiops_nav_collapsed", JSON.stringify(collapsed)); } catch(e){}
    });
  });
})();

// 姹夊牎锛氭闈㈡敹璧?灞曞紑渚ф爮锛涚Щ鍔ㄧ鎵撳紑/鍏抽棴鎶藉眽
safeAddEventListener("menuBtn", "click", () => {
  const appEl = $("app");
  if (!appEl) return;
  if (window.innerWidth <= 900) appEl.classList.toggle("nav-open");
  else appEl.classList.toggle("collapsed");
});
safeAddEventListener("backdrop", "click", () => {
  const appEl = $("app");
  if (appEl) appEl.classList.remove("nav-open");
});

// 鏃ュ織绫诲瀷绛涢€?
safeAddEventListener("logFilter", "click", e => {
  const b = e.target.closest(".chip-btn"); if (!b) return;
  LOG_KIND = b.dataset.kind;
  LOG_PAGE = 1;
  document.querySelectorAll("#logFilter .chip-btn").forEach(x => x.classList.toggle("active", x === b));
  renderLog(LAST_LOG);
});

// 鍛婅绫诲瀷绛涢€?
safeAddEventListener("alertFilter", "click", e => {
  const b = e.target.closest(".chip-btn"); if (!b) return;
  filterAlertsByType(b.dataset.atype);
});
// 鍛婅鎼滅储
safeAddEventListener("alertSearch", "input", e => { ALERT_SEARCH = e.target.value; renderAlerts(LAST_ALERTS); });

// 鏃ュ織绾у埆鍜屾椂闂磋寖鍥寸瓫閫?
function filterLogsByLevel(level) {
  LOG_LEVEL = level;
  LOG_PAGE = 1;
  renderLog(LAST_LOG);
}

function filterLogsByTime(range) {
  LOG_TIME_RANGE = range;
  LOG_PAGE = 1;
  renderLog(LAST_LOG);
}

// 鏃ュ織鍒嗛〉鐐瑰嚮
safeAddEventListener("logPager", "click", e => {
  const b = e.target.closest("button[data-lpg]"); if (!b) return;
  const pg = b.dataset.lpg;
  if (pg === "prev") LOG_PAGE--;
  else if (pg === "next") LOG_PAGE++;
  else LOG_PAGE = parseInt(pg);
  renderLog(LAST_LOG);
});

// 鐩戞帶绫诲瀷绛涢€?
function filterChecks(type) {
  CHECK_TYPE = type;
  renderChecks(LAST_CHECKS);
}
// 寮圭獥鍏抽棴锛氱偣閬僵绌虹櫧澶?鎴?鍙充笂瑙?鉁?
document.querySelectorAll(".mask").forEach(mk => mk.addEventListener("click", e => {
  if (e.target === mk || e.target.closest("[data-close-btn]")) {
    if (mk.hasAttribute("data-forced")) return; // 寮哄埗寮圭獥锛堥娆″畨鍏ㄥ垵濮嬪寲锛夛細绂佹鐐归伄缃?鉁?鍏抽棴
    mk.classList.remove("show"); hideChartTip();
    if (mk.id === "termMask") { closeTerminalWS(); }
    if (mk.id === "termReplayMask") { closeReplay(); }
    if (mk.id === "termObserveMask") { closeObserveWS(); }
    if (mk.id === "termSessionsMask") { if (TERM_SESSIONS_TIMER) { clearInterval(TERM_SESSIONS_TIMER); TERM_SESSIONS_TIMER = null; } }
    // v5.3.0: 缁堢璁よ瘉寮圭獥鍏抽棴鏃舵竻鐞嗙姸鎬?
    if (mk.id === "termProtocolMask" || mk.id === "termSetPwdMask" || mk.id === "termVerifyMask" || mk.id === "termLockedMask") { cancelTermAuth(); }
  }
}));
document.addEventListener("keydown", e => {
  if (e.key === "Escape") {
    const hadTerm = $("termMask") && $("termMask").classList.contains("show");
    const hadReplay = $("termReplayMask") && $("termReplayMask").classList.contains("show");
    const hadObserve = $("termObserveMask") && $("termObserveMask").classList.contains("show");
    const hadSessions = $("termSessionsMask") && $("termSessionsMask").classList.contains("show");
    document.querySelectorAll(".mask.show:not([data-forced])").forEach(mk => mk.classList.remove("show"));
    hideChartTip();
    if (hadTerm) closeTerminalWS();
    if (hadReplay) closeReplay();
    if (hadObserve) closeObserveWS();
    if (hadSessions && TERM_SESSIONS_TIMER) { clearInterval(TERM_SESSIONS_TIMER); TERM_SESSIONS_TIMER = null; }
  }
});

// KPI 鍗＄墖鐐瑰嚮 鈫?璺宠浆瀵瑰簲瑙嗗浘锛堝苟鎸夐渶杩囨护涓绘満锛?
safeAddEventListener("cards", "click", e => {
  const c = e.target.closest(".card"); if (!c) return;
  const [view, filter] = (c.dataset.goto || "").split(":");
  if (view === "hosts") { HOST_FILTER = filter || "all"; HOST_PAGE = 1; renderHosts(LAST_HOSTS); }
  if (view) switchView(view);
});
// 涓绘満鎼滅储 + 鍒嗛〉
safeAddEventListener("hostSearch", "input", e => { HOST_SEARCH = e.target.value; HOST_PAGE = 1; renderHosts(LAST_HOSTS); });
safeAddEventListener("pager", "click", e => {
  const b = e.target.closest("button[data-pg]"); if (!b) return;
  const pg = b.dataset.pg;
  if (pg === "prev") HOST_PAGE--;
  else if (pg === "next") HOST_PAGE++;
  else HOST_PAGE = parseInt(pg);
  renderHosts(LAST_HOSTS);
});
// 鑷畾涔夌洃鎺?
safeAddEventListener("addCheckBtn", "click", () => openCheckModal(null));
safeAddEventListener("ckType", "change", updateCkTargetLabel);
safeAddEventListener("ckSaveBtn", "click", saveCheck);
safeAddEventListener("checksGrid", "click", e => {
  const card = e.target.closest(".check-card"); if (!card) return;
  const act = e.target.closest("[data-cact]"); if (!act) return;
  const id = card.dataset.id, check = LAST_CHECKS.find(c => c.id === id);
  const cact = act.dataset.cact;
  if (cact === "hist") { if (check) openCheckHistory(id, check.name, check.type); return; } // 鍘嗗彶瀵瑰唴缃鏌ヤ篃寮€鏀?
  if (card.dataset.builtin) return; // 鍐呯疆妫€鏌ヤ粎鍙煡鐪嬪巻鍙诧紝鏃犵紪杈?鍒犻櫎
  if (cact === "edit") openCheckModal(check);
  else if (cact === "del") delCheck(id);
  else if (cact === "run") {
    fetch(`${API}/checks/${encodeURIComponent(id)}/run`, { method: "POST" })
      .then(() => { toast(I18N.t("toast.check_triggered"), "ok"); setTimeout(loadChecks, 1500); })
      .catch(e => toast(I18N.t("toast.trigger_failed2") + e, "err"));
  }
});
// 姒傝 璧勬簮 TOP10 鐐瑰嚮锛氫富鏈鸿鎯?/ 鐩戞帶鍘嗗彶
safeAddEventListener("topPanels", "click", e => {
  // 琛岀偣鍑?鈫?涓绘満璇︽儏
  const row = e.target.closest(".top-item");
  if (row) { openDetail(row.dataset.id, row.dataset.name); return; }
  // 鐩戞帶鎺㈤拡鐐瑰嚮
  const chk = e.target.closest(".checks-item");
  if (chk) { openCheckHistory(chk.dataset.checkId, chk.dataset.checkName, chk.dataset.checkType); return; }
});
// 鏃ュ織瀵煎嚭
safeAddEventListener("exportLogBtn", "click", exportLogsCSV);
// 鏆傚仠鑷姩鍒锋柊 + 鎵归噺娓呯悊绂荤嚎
safeAddEventListener("pauseBtn", "click", togglePause);
safeAddEventListener("purgeOfflineBtn", "click", purgeOffline);
// ===== 椤舵爮鐢ㄦ埛鑿滃崟 =====
(function initUserDropdown() {
  const wrap = $("topbarUserWrap");
  const btn = $("profileBtn");
  if (!wrap || !btn) return;
  // 鐐瑰嚮澶村儚鍒囨崲涓嬫媺
  btn.addEventListener("click", function(e) {
    e.stopPropagation();
    wrap.classList.toggle("open");
  });
  // 鐐瑰嚮澶栭儴鍏抽棴
  document.addEventListener("click", function(e) {
    if (!wrap.contains(e.target)) wrap.classList.remove("open");
  });
  // ESC 鍏抽棴
  document.addEventListener("keydown", function(e) {
    if (e.key === "Escape") wrap.classList.remove("open");
  });
  // 涓婚鍒囨崲
  safeAddEventListener("ddThemeToggle", "click", function() { toggleTheme(); wrap.classList.remove("open"); });
  // 璇█鍒囨崲锛堟寔涔呭寲鍒?cookie锛屽氨鍦伴噸娓叉煋鎵€鏈夋枃妗堬紝涓嶅埛鏂伴〉闈級
  var userDropdown = $("userDropdown");
  if (userDropdown) {
    userDropdown.addEventListener("click", function(e) {
      var b = e.target.closest("[data-lang]");
      if (b) I18N.setLang(b.dataset.lang);
    });
  }
  // 椤舵爮璇█鍒囨崲鎸夐挳缁勶紙绠€ / 绻?/ EN锛?
  var tbLang = $("tbLang");
  if (tbLang) {
    tbLang.addEventListener("click", function(e) {
      var b = e.target.closest("[data-lang]");
      if (b) I18N.setLang(b.dataset.lang);
    });
  }
  // 鏍囪褰撳墠閫変腑鐨勮瑷€锛堟兜鐩栭《鏍忎笌涓嬫媺涓ゅ [data-lang] 鎺т欢锛?
  if (I18N.syncLangButtons) I18N.syncLangButtons();
  // 鍛婅璁剧疆
  safeAddEventListener("ddSettings", "click", function() { openSettings(); wrap.classList.remove("open"); });
  // 鍛婅璁剧疆寮圭獥鍐?Tab 鍒囨崲
  safeAddEventListener("notifyTabs", "click", function(e) {
    const tab = e.target.closest(".tab");
    if (tab && tab.dataset.tab) switchNotifyTab(tab.dataset.tab);
  });
  // 涓汉淇℃伅
  safeAddEventListener("ddProfile", "click", function() { openProfile(); wrap.classList.remove("open"); });
  // 閫€鍑虹櫥褰?
  safeAddEventListener("ddLogout", "click", function() { logout(); wrap.classList.remove("open"); });
  // 鍒濆鍖栦富棰樻爣绛?
})();
// 鏃х殑 profileBtn 鐩存帴鎵撳紑涓汉淇℃伅 鈥?宸茶涓婇潰鐨勪笅鎷夎彍鍗曟浛浠?
// #usersBtn 宸插簾寮冿紙鐢ㄦ埛绠＄悊骞跺叆涓汉淇℃伅鍥?Tab锛夛紝浠呬繚鐣?openUsers() 閲嶅畾鍚戝叆鍙?
safeAddEventListener("profileTabs", "click", function (e) {
  const t = e.target.closest(".tab");
  if (t && t.dataset.ptab) switchProfileTab(t.dataset.ptab);
});
safeAddEventListener("userAddBtn", "click", () => openUserEdit(null));
safeAddEventListener("globalMfaChk", "change", async () => {
  const chk = $("globalMfaChk");
  if (!chk) return;
  const required = chk.checked;
  chk.disabled = true;
  try {
    const r = await fetch(`${API}/mfa/global`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ required }) });
    const j = await r.json().catch(() => ({}));
    if (r.ok) toast(required ? I18N.t("toast.global_mfa_on") : I18N.t("toast.global_mfa_off"), "ok");
    else { toast(j.error || I18N.t("toast.operation_failed"), "err"); chk.checked = !required; }
  } catch (e) { toast(I18N.t("toast.network_error"), "err"); chk.checked = !required; }
  chk.disabled = false;
});
safeAddEventListener("usersList", "click", async e => {
  const btn = e.target.closest("[data-act]"); if (!btn) return;
  const row = e.target.closest(".user-row"); if (!row) return;
  const name = row.dataset.name, act = btn.dataset.act;
  if (act === "edit") {
    const users = await fetch(`${API}/users`).then(r => r.json()).catch(() => []);
    const u = Array.isArray(users) && users.find(x => x.username === name);
    if (u) openUserEdit(u);
  } else { usersAction(name, act); }
});
safeAddEventListener("pfSaveBtn", "click", saveProfile);
safeAddEventListener("pfPwdBtn", "click", changePassword);
// 棣栨鐧诲綍路瀹夊叏鍒濆鍖栧脊绐楋細鎻愪氦鎸夐挳 + 纭瀵嗙爜妗嗗洖杞︽彁浜?
safeAddEventListener("initSubmitBtn", "click", submitInitSetup);
safeAddEventListener("initPass2", "keydown", e => { if (e.key === "Enter") { e.preventDefault(); submitInitSetup(); } });
safeAddEventListener("pfTermPwdBtn", "click", submitTermPwdChange);
safeAddEventListener("mfaToggleChk", "change", () => {
  const chk = $("mfaToggleChk");
  if (chk) chk.checked = MFA_ENABLED; // revert immediately; renderMfaState will update on success
  MFA_ENABLED ? openMfaDisable() : openMfaSetup();
});
safeAddEventListener("logoutBtn", "click", logout);
// 鐧诲綍椤垫壘鍥炲叆鍙?
safeAddEventListener("forgotUserLink", "click", openRecoverUser);
safeAddEventListener("forgotPassLink", "click", openRecoverPass);


/* ---------- 甯冨眬瀹藉害鍒囨崲锛堝凡绉婚櫎锛岀敱榛樿鍊兼帶鍒讹級 ---------- */

/* ---------- 鑷畾涔夌洃鎺ц鍥惧垏鎹紙鍒楄〃 / 鑳跺泭锛?---------- */
safeAddEventListener("checkViewToggle", "click", e => {
  const b = e.target.closest(".vt-btn"); if (!b) return;
  setCheckView(b.dataset.cview);
});
safeAddEventListener("hostViewToggle", "click", e => {
  const b = e.target.closest(".vt-btn"); if (!b) return;
  setHostView(b.dataset.hview);
});

// 璇诲彇鏈湴鍋忓ソ骞跺簲鐢紙瑙嗗浘 / 甯冨眬瀹藉害锛?
function initPrefs() {
  try { const cv = localStorage.getItem("aiops_check_view"); if (cv === "pill" || cv === "list") CHECK_VIEW = cv; } catch (e) {}
  try { const hv = localStorage.getItem("aiops_host_view"); if (hv === "list" || hv === "card") HOST_VIEW = hv; } catch (e) {}
  // 榛樿鍗＄墖瑙嗗浘锛氬嵆浣?localStorage 鏃犲€间篃纭繚 HOST_VIEW 涓?"card"
  if (HOST_VIEW !== "list" && HOST_VIEW !== "card") HOST_VIEW = "card";
  document.querySelectorAll("#checkViewToggle .vt-btn").forEach(b => b.classList.toggle("active", b.dataset.cview === CHECK_VIEW));
  document.querySelectorAll("#hostViewToggle .vt-btn").forEach(b => b.classList.toggle("active", b.dataset.hview === HOST_VIEW));
}

initPrefs();
CUR_CATS = getSelectedCats();

/* ---------- P2-#18: Keyboard shortcuts 1-5 switch views ---------- */
document.addEventListener("keydown", e => {
  if (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA" || e.target.tagName === "SELECT") return;
  if (e.metaKey || e.ctrlKey || e.altKey) return;
  const views = ["overview", "hosts", "checks", "alerts", "automation", "log"];
  const idx = parseInt(e.key) - 1;
  if (idx >= 0 && idx < views.length) {
    e.preventDefault();
    switchView(views[idx]);
  }
});

/* ---------- Alert filter helpers ---------- */
function filterAlertsByType(type) {
  ALERT_TYPE = type;
  document.querySelectorAll("#alertFilter .chip-btn").forEach(b => b.classList.toggle("active", b.dataset.atype === type));
  renderAlerts(LAST_ALERTS);
}
let LAST_ALERTS = [];

// 自动化运维代码已提取至 automation.js

// 缁堢浼氳瘽绠＄悊 + 鍥炴斁 + 鏃佽
safeAddEventListener("termSessionsBtn", "click", openTerminalSessions);
// 缁堢浼氳瘽鎼滅储
safeAddEventListener("termSessionSearch", "input", e => {
  TERM_SEARCH = e.target.value;
  renderTerminalSessions(LAST_TERM_SESSIONS);
});
safeAddEventListener("replayPlayBtn", "click", () => { if (REPLAY && REPLAY.playing) pauseReplay(); else playReplay(); });
safeAddEventListener("replayProgressBg", "click", e => {
  const rect = e.currentTarget.getBoundingClientRect();
  const progress = (e.clientX - rect.left) / rect.width;
  seekReplay(Math.max(0, Math.min(1, progress)));
});
document.querySelectorAll(".replay-speed-btn").forEach(btn => {
  btn.addEventListener("click", () => setReplaySpeed(parseFloat(btn.dataset.speed)));
});

/* ---------- PWA: SW registration + Install prompt + Hash routing ---------- */
// P1-4: 鍏ㄥ眬 Escape 閿叧闂ā鎬佸脊绐?
document.addEventListener("keydown", e => {
  if (e.key === "Escape") {
    const masks = document.querySelectorAll(".mask.show:not([data-forced])");
    if (masks.length > 0) {
      // 鍙叧闂渶涓婂眰鐨勫脊绐?
      const top = masks[masks.length - 1];
      closeMask(top);
    }
  }
});
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js", { scope: "/" }).catch(() => {});
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
  if (h && ["overview", "hosts", "checks", "alerts", "automation", "log"].includes(h)) {
    switchView(h);
  }
});

// 璇█灏卞湴鍒囨崲锛歩18n-dashboard.js 宸插畬鎴愰潤鎬?[data-i18n] 鏂囨湰鏇挎崲锛岃繖閲岃礋璐?
// 閲嶅缓 JS 鍔ㄦ€佺敓鎴愮殑鏂囨锛堥〉闈㈡爣棰?鍓爣棰樸€佸悇瑙嗗浘鍒楄〃銆佸浘琛ㄧ瓑锛夛紝骞朵繚鎸佸綋鍓嶈鍥?婊氬姩/
// 宸叉墦寮€闈㈡澘涓嶅彉锛堜笉鍒锋柊椤甸潰锛夈€?
document.addEventListener("i18n:changed", () => {
  try { rebuildPageMeta(); } catch (e) {}
  const activeNav = document.querySelector(".nav-item.active");
  const view = activeNav ? activeNav.dataset.view : null;
  if (view && PAGE_META[view]) {
    const t = $("pageTitle"), s = $("pageSub");
    if (t) t.textContent = PAGE_META[view].title;
    if (s) s.textContent = PAGE_META[view].sub;
  }
  // 姒傝鏁版嵁锛堝崱鐗?鍋ュ悍/鍛婅/娲诲姩/涓绘満/TOP锛夊己鍒堕噸娓叉煋
  try { refresh(true); } catch (e) {}
  // 褰撳墠瑙嗗浘涓撳睘鐨勫姩鎬佸垪琛ㄦ寜闇€閲嶈浇锛堟ā鍧楃骇绛涢€?鍒嗛〉鐘舵€佷繚鎸佷笉鍙橈級
  try {
    if (view === "checks") loadChecks();
    else if (view === "automation") loadPlaybooks();
    else if (view === "forward") { loadForwards(); loadHttpProxies(); }
    else if (view === "hosts") loadHostsMeta();
  } catch (e) {}
  if (I18N.syncLangButtons) I18N.syncLangButtons();
});

initAuth();

/* ============================================================
   P3-1: WebSocket 鎺ㄩ€侊紙鏇夸唬杞锛屽甫闄嶇骇锛?
   ============================================================ */
let PUSH_WS = null;
let PUSH_CONNECTED = false;
let PUSH_RETRY = 0;

function initPushWS() {
  // 浠呭湪 HTTPS 鎴?localhost 涓嬪皾璇?WebSocket
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
        if (msg.type === "summary" && msg.data) {
          renderCards(msg.data);
          updateFavicon(msg.data.critical_alerts || 0);
          notifyCriticalAlerts(msg.data.critical_alerts || 0);
        } else if (msg.type === "alerts" && msg.data) {
          renderAlerts(msg.data);
        } else if (msg.type === "hosts" && msg.data) {
          renderHosts(msg.data);
        }
      } catch(err) {}
    };
    PUSH_WS.onclose = () => {
      PUSH_CONNECTED = false;
      // 鎸囨暟閫€閬块噸杩?
      PUSH_RETRY++;
      if (PUSH_RETRY <= 10) {
        setTimeout(() => initPushWS(), Math.min(30000, 1000 * Math.pow(2, PUSH_RETRY)));
      }
    };
    PUSH_WS.onerror = () => { try { PUSH_WS.close(); } catch(e) {} };
  } catch(e) {}
}

/* ============================================================
   绔彛杞彂
   ============================================================ */
let LAST_FORWARDS = [];
let LAST_HTTP_PROXIES = [];
let FWD_MODE = "tcp"; // "tcp" | "http"

// 濉厖涓绘満涓嬫媺閫夋嫨妗嗭紙鍚屾椂濉厖鍒涘缓寮圭獥 fwdHost 鍜岀紪杈戝脊绐?fwdEditHost锛?
function populateForwardHosts() {
  const opts = LAST_HOSTS.map(h => `<option value="${h.id}">${esc(h.hostname)} (${esc(h.ip || "鈥?)})</option>`).join("");
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

// 杞彂瑙嗗浘妯″紡锛歭ist锛堥粯璁わ級| card
let FORWARD_VIEW_MODE = "list";

// 鎿嶄綔鍥炬爣锛堢粺涓€鐨勬弿杈?SVG锛屼娇鐢?currentColor 璺熼殢鏂囧瓧鑹诧級
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
  const wrap = $("forwardViewToggle");
  if (wrap) wrap.querySelectorAll(".vt-btn").forEach(b => b.classList.toggle("active", b.dataset.view === mode));
  renderForwards();
}

// 鏋勫缓鍗曟潯杞彂鐨勬搷浣滄寜閽粍
function fwdActionButtons(item) {
  const toggleIcon = item.enabled ? FWD_ICONS.disable : FWD_ICONS.enable;
  const toggleLabel = item.enabled ? I18N.t("ui.disable") : I18N.t("ui.enable");
  const primary = item.type === "http"
    ? `<button class="icon-btn" title="${I18N.t("ui.open")}" data-act="proxy-open" data-url="${esc(item.proxyUrl)}">${FWD_ICONS.open}</button>`
    : ""; // TCP锛氬簲鐢ㄦ埛瑕佹眰绉婚櫎銆屽鍒跺湴鍧€銆嶆寜閽紙鍒楄〃/鍗＄墖閲岀殑鐩戝惉鍦板潃浠嶅彲鐩存帴澶嶅埗锛?
  return `
    <button class="icon-btn" title="${toggleLabel}" data-act="fwd-toggle" data-ftype="${esc(item.type)}" data-fid="${esc(item.id)}" data-enable="${item.enabled ? "0" : "1"}">${toggleIcon}</button>
    ${primary}
    <button class="icon-btn" title="${I18N.t("ui.copy")}" data-act="fwd-copy" data-ftype="${esc(item.type)}" data-fid="${esc(item.id)}">${FWD_ICONS.copy}</button>
    <button class="icon-btn" title="${I18N.t("ui.edit")}" data-act="fwd-edit" data-ftype="${esc(item.type)}" data-fid="${esc(item.id)}">${FWD_ICONS.edit}</button>
    <button class="icon-btn danger" title="${I18N.t("ui.delete")}" data-act="fwd-del" data-ftype="${esc(item.type)}" data-fid="${esc(item.id)}">${FWD_ICONS.del}</button>`;
}

// 灏?TCP / HTTP 涓ゆ潯鏁版嵁婧愮粺涓€涓烘覆鏌撴ā鍨?
function collectForwardItems() {
  const items = [];
  (LAST_FORWARDS || []).forEach(f => {
    items.push({
      type: "tcp", id: f.id,
      enabled: f.enabled !== false,
      badge: "TCP", badgeClass: "op",
      title: `${esc(f.hostname)} 鈫?:${f.target_port}`,
      sub: `${I18N.t("ui.listen_addr")} <code class="mono">${esc(f.listen_addr)}</code> 路 ${f.sessions} ${I18N.t("ui.active_sessions")}`,
      listenAddr: f.listen_addr,
    });
  });
  (LAST_HTTP_PROXIES || []).forEach(p => {
    const proxyUrl = `/proxy/${encodeURIComponent(p.host_id)}/${p.target_port}/${(p.default_path || "").replace(/^\//, "")}`;
    items.push({
      type: "http", id: p.id,
      enabled: p.enabled !== false,
      badge: "HTTP", badgeClass: "sys",
      title: esc(p.name || `${p.hostname}:${p.target_port}`) + (p.is_copy ? I18N.t("forward.copy_suffix") : ""),
      sub: `${esc(p.hostname)}:${p.target_port}${p.default_path ? " 路 " + esc(p.default_path) : ""}`,
      proxyUrl,
    });
  });
  return items;
}

function renderForwards() {
  const list = $("forwardList");
  const empty = $("forwardEmpty");
  if (!list || !empty) return;

  const items = collectForwardItems();

  if (FORWARD_VIEW_MODE === "card") {
    list.className = "fwd-list fwd-grid";
    // Reuse the automation playbook-card structure (pb-card*) so forward cards are
    // visually identical: title + sub top-left, status top-right, divider, footer
    // with a type pill and the action buttons.
    list.innerHTML = items.map(it => `
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
      </div>`).join("");
  } else {
    list.className = "fwd-list";
    list.innerHTML = items.map(it => `
      <div class="fwd-row ${it.enabled ? "" : "fwd-off"}">
        <span class="badge ${it.badgeClass}">${it.badge}</span>
        <div class="fwd-main">
          <div class="fwd-title">${it.title}</div>
          <div class="fwd-sub">${it.sub}</div>
        </div>
        <div class="fwd-actions">${fwdActionButtons(it)}</div>
      </div>`).join("");
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
  await withLoading("fwdSubmitBtn", async () => {
    try {
      const res = await fetch("/api/v1/forward", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ host_id: hostID, target_port: targetPort, local_port: localPort })
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        toast(err.error || I18N.t("toast.create_failed"), "err");
        return;
      }
      const result = await res.json();
      toast(I18N.t("toast.forward_created") + result.listen_addr, "ok");
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
    // Token cookie is now set; open the URL 鈥?backend reads cookie or pt param.
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

// 淇濆瓨 HTTP 鍙嶅悜浠ｇ悊閰嶇疆骞剁珛鍗冲湪鏂版爣绛炬墦寮€锛堝悎骞跺師銆屼繚瀛橀厤缃€?銆屾墦寮€閾炬帴銆嶄袱涓寜閽級銆?
// 鍏堝悓姝?window.open 鍗犱綅绐楀彛锛岄伩鍏?save/token 涓ゆ await 涔嬪悗 window.open 琚脊绐楁嫤鎴€?
async function saveAndOpenHttpProxy() {
  const hostID = $("fwdHost")?.value;
  const targetPort = parseInt($("fwdTargetPort")?.value || "0");
  const name = $("fwdHttpName")?.value || "";
  const defaultPath = $("fwdHttpPath")?.value || "";
  if (!hostID || targetPort < 1 || targetPort > 65535) {
    toast(I18N.t("valid.fill_target_port"), "err");
    return;
  }
  const win = window.open("", "_blank"); // 鍚屾鍗犱綅绐楀彛锛屼繚浣忕敤鎴锋墜鍔匡紝瑙勯伩寮圭獥鎷︽埅
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
      // 鍙栦竴娆℃€т唬鐞嗕护鐗岋紙鏈嶅姟绔悓鏃朵笅鍙?cookie锛夛紝鍐嶆妸鍗犱綅绐楀彛瀵艰埅鍒颁唬鐞嗗湴鍧€
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

// 鍚敤 / 鍋滅敤鏌愭潯杞彂锛圱CP 鎴?HTTP锛?
async function toggleForward(ev, type, id, enable) {
  await withLoading(ev.currentTarget, async () => {
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

// 澶嶅埗锛堝厠闅嗭級鏌愭潯杞彂
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

function closeForwardModal() {
  const forwardMask = $("forwardMask");
  const backdrop = $("backdrop");
  if (forwardMask) forwardMask.classList.remove("show");
  if (backdrop) backdrop.style.display = "none";
}

// 鍒犻櫎鏌愭潯杞彂锛堢粺涓€ TCP / HTTP锛?
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

// 鎵撳紑缂栬緫寮圭獥骞堕濉暟鎹?
function editForward(type, id) {
  const item = type === "tcp"
    ? (LAST_FORWARDS || []).find(f => f.id === id)
    : (LAST_HTTP_PROXIES || []).find(p => p.id === id);
  if (!item) return;
  $("fwdEditId").value = id;
  $("fwdEditType").value = type;
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

// 淇濆瓨缂栬緫缁撴灉
async function saveForwardEdit() {
  const type = $("fwdEditType").value;
  const id = $("fwdEditId").value;
  const hostID = $("fwdEditHost").value;
  const targetPort = parseInt($("fwdEditPort").value || "0");
  if (!hostID || targetPort < 1 || targetPort > 65535) {
    toast(I18N.t("valid.fill_target_port"), "err");
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
    // 淇濇寔褰撳墠鍚敤鐘舵€侊紝閬垮厤缂栬緫鍚庤鍒欒鎰忓绂佺敤
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

// 缁戝畾浜嬩欢
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
});
safeAddEventListener("fwdSubmitBtn", "click", submitForward);
// 銆屼繚瀛樺苟鎵撳紑銆嶏細鍚堝苟浜嗗師銆屼繚瀛橀厤缃€?銆屾墦寮€閾炬帴銆嶏紙淇濆瓨閰嶇疆鎸夐挳宸茬Щ闄わ級
safeAddEventListener("fwdHttpOpenBtn", "click", saveAndOpenHttpProxy);
safeAddEventListener("fwdEditSaveBtn", "click", saveForwardEdit);
// Mode tab clicks
document.querySelectorAll("#fwdModeTabs .fwd-mode-tab").forEach(btn => {
  btn.addEventListener("click", () => switchFwdMode(btn.dataset.fwdmode));
});

// 澶嶅埗鏂囨湰鍒板壀璐存澘
function copyText(text) {
  navigator.clipboard?.writeText(text).then(() => toast(I18N.t("toast.copied_detail") + text, "ok"));
}

/* ============================================================
   CSP 鍔犲浐锛氭妸鎵€鏈夊唴鑱?on*= 浜嬩欢澶勭悊鍣ㄧ粺涓€鏀逛负銆屼簨浠跺鎵樸€?
   鈥斺€?鐩殑鏈変簩锛?
   1) 鍏佽鍦?CSP 涓Щ闄?script-src 'unsafe-inline'锛屽嵆渚垮嚭鐜?XSS 涔熸棤娉曟墽琛屽唴鑱旇剼鏈紱
   2) 娑堥櫎姝ゅ墠鎶婁富鏈哄悕/浠ｇ悊 URL 鐩存帴鎷艰繘 onclick JS 瀛楃涓插鑷寸殑 DOM XSS
      锛坋sc() 涓嶈浆涔夊崟寮曞彿锛屾伓鎰忎富鏈哄悕甯?' 鍗冲彲瓒婄晫娉ㄥ叆锛夈€?
   绾﹀畾锛氬彲鐐瑰嚮鍏冪礌鐢?data-act="鍔ㄤ綔"锛岄檮甯︾殑鍙傛暟鏀?data-* 灞炴€э紝缁?dataset 璇诲彇锛?
   鏁版嵁涓嶅啀杩涘叆浠讳綍鍙墽琛屼笂涓嬫枃銆?
   ============================================================ */
document.addEventListener("click", e => {
  const el = e.target.closest("[data-act]");
  if (!el) return;
  switch (el.dataset.act) {
    case "install": openInstall(); break;
    case "ai-preset": setAIPreset(el.dataset.preset); break;
    case "fwd-view": switchForwardView(el.dataset.view); break;
    case "term-observe": openTerminalObserve(el.dataset.sid, el.dataset.host); break;
    case "term-replay": openTerminalReplay(el.dataset.sid, el.dataset.host); break;
    case "proxy-open": openProxyUrl(el.dataset.url); break;
    case "fwd-toggle": toggleForward(e, el.dataset.ftype, el.dataset.fid, el.dataset.enable === "1"); break;
    case "fwd-copy": copyForward(el.dataset.ftype, el.dataset.fid); break;
    case "fwd-edit": editForward(el.dataset.ftype, el.dataset.fid); break;
    case "fwd-del": deleteForward(el.dataset.ftype, el.dataset.fid); break;
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
  }
});

/* ============================================================
   绂荤嚎妫€娴?
   ============================================================ */
window.addEventListener("online", () => {
  toast(I18N.t("toast.network_recovered"), "ok");
  refresh(true);
});
window.addEventListener("offline", () => {
  toast(I18N.t("toast.network_disconnected"), "err");
});

/* ============================================================
   渚ф爮瀹炴椂鏃堕挓
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
