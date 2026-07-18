/* ---------- 告警设置 ---------- */
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
    // SMS config
    const sms = c.sms || {};
    $("smsEnabled").checked = !!sms.enabled;
    $("smsProvider").value = sms.provider || "aliyun";
    $("smsAccessKey").value = sms.access_key || "";
    $("smsSecretKey").value = sms.secret_key || "";
    $("smsSignName").value = sms.sign_name || "";
    $("smsTemplateCode").value = sms.template_code || "";
    $("smsTemplateParam").value = sms.template_param || "";
    $("smsAppId").value = sms.app_id || "";
    $("smsSender").value = sms.sender || "";
    $("smsRegion").value = sms.region || "";
    $("smsPhones").value = (sms.phones || []).join(",");
    // VoiceCall config
    const vc = c.voice_call || {};
    $("voiceCallEnabled").checked = !!vc.enabled;
    $("voiceCallProvider").value = vc.provider || "aliyun";
    $("voiceCallAccessKey").value = vc.access_key || "";
    $("voiceCallSecretKey").value = vc.secret_key || "";
    $("voiceCallCalledNumbers").value = (vc.called_numbers || []).join(",");
    $("voiceCallTtsCode").value = vc.tts_code || "";
    $("voiceCallTtsParam").value = vc.tts_param || "";
    $("voiceCallAppId").value = vc.app_id || "";
    $("voiceCallDisplayNbr").value = vc.display_nbr || "";
    $("voiceCallRegion").value = vc.region || "";
    // Threshold display: treat 0 / null / undefined as "unset" → show the standard
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
    // 拨测监控
    $("checkPingLossWarn").value = td(t.check_ping_loss_warn, 10); $("checkPingLossCrit").value = td(t.check_ping_loss_crit, 30);
    $("checkPingLatencyWarn").value = td(t.check_ping_latency_warn, 100); $("checkPingLatencyCrit").value = td(t.check_ping_latency_crit, 500);
    $("checkTCPTimeoutWarn").value = td(t.check_tcp_timeout_warn, 1000); $("checkTCPTimeoutCrit").value = td(t.check_tcp_timeout_crit, 5000);
    $("checkHTTPRespWarn").value = td(t.check_http_resp_warn, 1000); $("checkHTTPRespCrit").value = td(t.check_http_resp_crit, 5000);
    $("checkHTTPStatusWarn").value = td(t.check_http_status_warn, 1); $("checkHTTPStatusCrit").value = td(t.check_http_status_crit, 5);
    $("checkProcFailWarn").value = td(t.check_proc_fail_warn, 1); $("checkProcFailCrit").value = td(t.check_proc_fail_crit, 3);
    $("checkUDPTimeoutWarn").value = td(t.check_udp_timeout_warn, 1000); $("checkUDPTimeoutCrit").value = td(t.check_udp_timeout_crit, 5000);
    // API 业务监控
    $("apiAvailWarn").value = td(t.api_avail_warn, 99); $("apiAvailCrit").value = td(t.api_avail_crit, 95);
    $("apiAvgRespWarn").value = td(t.api_avg_resp_warn, 500); $("apiAvgRespCrit").value = td(t.api_avg_resp_crit, 2000);
    $("apiP95RespWarn").value = td(t.api_p95_resp_warn, 1000); $("apiP95RespCrit").value = td(t.api_p95_resp_crit, 5000);
    $("apiThroughputWarn").value = td(t.api_throughput_warn, 100); $("apiThroughputCrit").value = td(t.api_throughput_crit, 10);
    // 编排定时任务
    $("taskFailWarn").value = td(t.task_fail_warn, 1); $("taskFailCrit").value = td(t.task_fail_crit, 5);
    $("taskTimeoutWarn").value = td(t.task_timeout_warn, 60); $("taskTimeoutCrit").value = td(t.task_timeout_crit, 300);
    // 端口转发监控
    $("forwardConnWarn").value = td(t.forward_conn_warn, 200); $("forwardConnCrit").value = td(t.forward_conn_crit, 280);
    $("forwardBwWarn").value = td(t.forward_bw_warn, 80); $("forwardBwCrit").value = td(t.forward_bw_crit, 95);
    $("forwardErrWarn").value = td(t.forward_err_warn, 5); $("forwardErrCrit").value = td(t.forward_err_crit, 15);
    $("forwardLatWarn").value = td(t.forward_lat_warn, 1000); $("forwardLatCrit").value = td(t.forward_lat_crit, 5000);
    // SNMP 网络设备
    $("snmpIfUtilWarn").value = td(t.snmp_if_util_warn, 80); $("snmpIfUtilCrit").value = td(t.snmp_if_util_crit, 95);
    $("snmpIfErrWarn").value = td(t.snmp_if_err_warn, 1); $("snmpIfErrCrit").value = td(t.snmp_if_err_crit, 10);

    // Reset to first tab
    switchNotifyTab("tab-feishu");

    $("settingsMask").classList.add("show");
  } catch (e) { toast(I18N.t("toast.read_config_failed") + e, "err"); }
}

// ---- 告警阈值 Tab（已从「告警设置」弹窗独立出来，隶属「告警」模块）----
// 阈值输入框（同 ID）现位于 #view-thresholds。加载：拉全量配置回填字段；
// 保存：拉全量配置 → 仅覆盖 thresholds → 回存，从而不触碰 webhook/smtp 等其它设置
// （脱敏密钥原样回传，由后端按「掩码=保持原值」逻辑保留）。
async function loadThresholds() {
  try {
    const c = await fetch(`${API}/config`).then(r => r.json());
    const t = c.thresholds || {};
    const td = (v, def) => (v == null || v === 0 || isNaN(v)) ? def : v;
    $("cpuWarn").value = td(t.cpu_warn, 80); $("cpuCrit").value = td(t.cpu_crit, 95);
    $("memWarn").value = td(t.mem_warn, 85); $("memCrit").value = td(t.mem_crit, 95);
    $("diskWarn").value = td(t.disk_warn, 80); $("diskCrit").value = td(t.disk_crit, 90);
    $("diskioWarn").value = td(t.diskio_warn, 80); $("diskioCrit").value = td(t.diskio_crit, 95);
    $("iopsWarn").value = td(t.iops_warn, 50000); $("iopsCrit").value = td(t.iops_crit, 100000);
    $("gpuWarn").value = td(t.gpu_warn, 80); $("gpuCrit").value = td(t.gpu_crit, 95);
    $("gpuTempWarn").value = td(t.gpu_temp_warn, 85); $("gpuTempCrit").value = td(t.gpu_temp_crit, 95);
    $("gpuMemWarn").value = td(t.gpu_mem_warn, 90); $("gpuMemCrit").value = td(t.gpu_mem_crit, 97);
    $("loadWarn").value = td(t.load_warn, 4.0); $("loadCrit").value = td(t.load_crit, 8.0);
    $("procWarn").value = td(t.proc_warn, 0.5);
    $("connWarn").value = td(t.conn_warn, 5000); $("connCrit").value = td(t.conn_crit, 10000);
    $("offlineSec").value = td(t.offline_after_sec, 60);
    // 拨测监控阈值
    $("checkPingLossWarn").value = td(t.check_ping_loss_warn, 10); $("checkPingLossCrit").value = td(t.check_ping_loss_crit, 30);
    $("checkPingLatencyWarn").value = td(t.check_ping_latency_warn, 100); $("checkPingLatencyCrit").value = td(t.check_ping_latency_crit, 500);
    $("checkTCPTimeoutWarn").value = td(t.check_tcp_timeout_warn, 1000); $("checkTCPTimeoutCrit").value = td(t.check_tcp_timeout_crit, 5000);
    $("checkHTTPRespWarn").value = td(t.check_http_resp_warn, 1000); $("checkHTTPRespCrit").value = td(t.check_http_resp_crit, 5000);
    $("checkHTTPStatusWarn").value = td(t.check_http_status_warn, 1); $("checkHTTPStatusCrit").value = td(t.check_http_status_crit, 5);
    $("checkProcFailWarn").value = td(t.check_proc_fail_warn, 1); $("checkProcFailCrit").value = td(t.check_proc_fail_crit, 3);
    $("checkUDPTimeoutWarn").value = td(t.check_udp_timeout_warn, 1000); $("checkUDPTimeoutCrit").value = td(t.check_udp_timeout_crit, 5000);
    // API 业务监控阈值
    $("apiAvailWarn").value = td(t.api_avail_warn, 99); $("apiAvailCrit").value = td(t.api_avail_crit, 95);
    $("apiAvgRespWarn").value = td(t.api_avg_resp_warn, 500); $("apiAvgRespCrit").value = td(t.api_avg_resp_crit, 2000);
    $("apiP95RespWarn").value = td(t.api_p95_resp_warn, 1000); $("apiP95RespCrit").value = td(t.api_p95_resp_crit, 5000);
    $("apiThroughputWarn").value = td(t.api_throughput_warn, 100); $("apiThroughputCrit").value = td(t.api_throughput_crit, 10);
    // 编排定时任务阈值
    $("taskFailWarn").value = td(t.task_fail_warn, 1); $("taskFailCrit").value = td(t.task_fail_crit, 5);
    $("taskTimeoutWarn").value = td(t.task_timeout_warn, 60); $("taskTimeoutCrit").value = td(t.task_timeout_crit, 300);
    // 端口转发监控阈值
    $("forwardConnWarn").value = td(t.forward_conn_warn, 200); $("forwardConnCrit").value = td(t.forward_conn_crit, 280);
    $("forwardBwWarn").value = td(t.forward_bw_warn, 80); $("forwardBwCrit").value = td(t.forward_bw_crit, 95);
    $("forwardErrWarn").value = td(t.forward_err_warn, 5); $("forwardErrCrit").value = td(t.forward_err_crit, 15);
    $("forwardLatWarn").value = td(t.forward_lat_warn, 1000); $("forwardLatCrit").value = td(t.forward_lat_crit, 5000);
    // SNMP 网络设备
    $("snmpIfUtilWarn").value = td(t.snmp_if_util_warn, 80); $("snmpIfUtilCrit").value = td(t.snmp_if_util_crit, 95);
    $("snmpIfErrWarn").value = td(t.snmp_if_err_warn, 1); $("snmpIfErrCrit").value = td(t.snmp_if_err_crit, 10);
  } catch (e) { toast(I18N.t("toast.read_config_failed") + e, "err"); }
}
async function saveThresholds() {
  await withLoading("saveThresholdsBtn", async () => {
    try {
      const c = await fetch(`${API}/config`).then(r => r.json()); // 全量配置（密钥已脱敏，回存时后端按原值保留）
      const num = id => parseFloat($(id).value) || 0;
      c.thresholds = {
        cpu_warn: num("cpuWarn"), cpu_crit: num("cpuCrit"),
        mem_warn: num("memWarn"), mem_crit: num("memCrit"),
        disk_warn: num("diskWarn"), disk_crit: num("diskCrit"),
        diskio_warn: num("diskioWarn"), diskio_crit: num("diskioCrit"),
        iops_warn: num("iopsWarn"), iops_crit: num("iopsCrit"),
        gpu_warn: num("gpuWarn"), gpu_crit: num("gpuCrit"),
        gpu_temp_warn: num("gpuTempWarn"), gpu_temp_crit: num("gpuTempCrit"),
        gpu_mem_warn: num("gpuMemWarn"), gpu_mem_crit: num("gpuMemCrit"),
        load_warn: num("loadWarn"), load_crit: num("loadCrit"),
        proc_warn: num("procWarn"),
        conn_warn: Math.round(num("connWarn")), conn_crit: Math.round(num("connCrit")),
        offline_after_sec: Math.round(num("offlineSec")),
        // 拨测监控
        check_ping_loss_warn: num("checkPingLossWarn"), check_ping_loss_crit: num("checkPingLossCrit"),
        check_ping_latency_warn: num("checkPingLatencyWarn"), check_ping_latency_crit: num("checkPingLatencyCrit"),
        check_tcp_timeout_warn: num("checkTCPTimeoutWarn"), check_tcp_timeout_crit: num("checkTCPTimeoutCrit"),
        check_http_resp_warn: num("checkHTTPRespWarn"), check_http_resp_crit: num("checkHTTPRespCrit"),
        check_http_status_warn: Math.round(num("checkHTTPStatusWarn")), check_http_status_crit: Math.round(num("checkHTTPStatusCrit")),
        check_proc_fail_warn: Math.round(num("checkProcFailWarn")), check_proc_fail_crit: Math.round(num("checkProcFailCrit")),
        check_udp_timeout_warn: num("checkUDPTimeoutWarn"), check_udp_timeout_crit: num("checkUDPTimeoutCrit"),
        // API 业务监控
        api_avail_warn: num("apiAvailWarn"), api_avail_crit: num("apiAvailCrit"),
        api_avg_resp_warn: num("apiAvgRespWarn"), api_avg_resp_crit: num("apiAvgRespCrit"),
        api_p95_resp_warn: num("apiP95RespWarn"), api_p95_resp_crit: num("apiP95RespCrit"),
        api_throughput_warn: num("apiThroughputWarn"), api_throughput_crit: num("apiThroughputCrit"),
        // 编排定时任务
        task_fail_warn: Math.round(num("taskFailWarn")), task_fail_crit: Math.round(num("taskFailCrit")),
        task_timeout_warn: num("taskTimeoutWarn"), task_timeout_crit: num("taskTimeoutCrit"),
        // 端口转发监控
        forward_conn_warn: Math.round(num("forwardConnWarn")), forward_conn_crit: Math.round(num("forwardConnCrit")),
        forward_bw_warn: num("forwardBwWarn"), forward_bw_crit: num("forwardBwCrit"),
        forward_err_warn: num("forwardErrWarn"), forward_err_crit: num("forwardErrCrit"),
        forward_lat_warn: num("forwardLatWarn"), forward_lat_crit: num("forwardLatCrit"),
        // SNMP 网络设备
        snmp_if_util_warn: num("snmpIfUtilWarn"), snmp_if_util_crit: num("snmpIfUtilCrit"),
        snmp_if_err_warn: num("snmpIfErrWarn"), snmp_if_err_crit: num("snmpIfErrCrit")
      };
      const r = await fetch(`${API}/config`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(c) });
      if (r.ok) toast("告警阈值已保存，即时生效", "ok");
      else toast(I18N.t("toast.save_failed"), "err");
    } catch (e) { toast(I18N.t("toast.save_failed2") + e, "err"); }
  });
}

// Tab switching for notification channels
function switchNotifyTab(tabId) {
  document.querySelectorAll("#notifyTabs .tab").forEach(btn => btn.classList.toggle("active", btn.dataset.tab === tabId));
  document.querySelectorAll("#settingsMask .tab-panel").forEach(p => p.classList.toggle("active", p.id === tabId));
}

function collectSettings() {
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
    sms: {
      enabled: $("smsEnabled").checked,
      provider: $("smsProvider").value,
      access_key: $("smsAccessKey").value.trim(),
      secret_key: $("smsSecretKey").value,
      sign_name: $("smsSignName").value.trim(),
      template_code: $("smsTemplateCode").value.trim(),
      template_param: $("smsTemplateParam").value.trim(),
      app_id: $("smsAppId").value.trim(),
      sender: $("smsSender").value.trim(),
      region: $("smsRegion").value.trim(),
      phones: ($("smsPhones").value || "").split(",").map(s => s.trim()).filter(Boolean)
    },
    voice_call: {
      enabled: $("voiceCallEnabled").checked,
      provider: $("voiceCallProvider").value,
      access_key: $("voiceCallAccessKey").value.trim(),
      secret_key: $("voiceCallSecretKey").value,
      called_numbers: ($("voiceCallCalledNumbers").value || "").split(",").map(s => s.trim()).filter(Boolean),
      tts_code: $("voiceCallTtsCode").value.trim(),
      tts_param: $("voiceCallTtsParam").value.trim(),
      app_id: $("voiceCallAppId").value.trim(),
      display_nbr: $("voiceCallDisplayNbr").value.trim(),
      region: $("voiceCallRegion").value.trim()
    }
    // 注意：告警阈值由独立的「告警阈值」Tab（saveThresholds）管理，此处不再序列化 thresholds，
    // 否则保存告警通知设置会用一份不完整的阈值覆盖掉 check_*/GPU 温度·显存/连接数 等字段（被后端
    // 零值回填成默认值 → 静默丢失用户自定义阈值）。saveSettings 改为「拉全量→仅覆盖通知字段→回存」。
  };
}
async function saveSettings() {
  await withLoading("saveBtn", async () => {
    try {
      // 拉全量配置，仅覆盖告警通知字段后回存，避免清空 thresholds 等由其它 Tab 管理的设置。
      const full = await fetch(`${API}/config`).then(r => r.json());
      Object.assign(full, collectSettings());
      const r = await fetch(`${API}/config`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(full) });
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

/* ---------- 安装 Agent ---------- */
let INSTALL = { server_url: "", token: "" };
let CUR_OS = "linux";
let RELAY_MODE = false;
let MULTI_SERVER_MODE = false;
let TOKEN_REVEALED = false; // Token 脱敏状态
// Windows Server 2012 R2 / 2016 default to TLS 1.0. The install.ps1 template enables
// TLS 1.2 internally, but that runs too late: the outer `irm` that DOWNLOADS the
// script fails first against a TLS1.2-only HTTPS server ("未能创建 SSL/TLS 安全通道").
// So the one-liner must enable TLS 1.2 itself, before irm. Numeric 3072 = Tls12,
// -bor keeps existing protocols; using the number avoids an enum-undefined error on
// older .NET where the [Net.SecurityProtocolType]::Tls12 name isn't defined.
const PS_TLS12 = "[Net.ServicePointManager]::SecurityProtocol=[Net.ServicePointManager]::SecurityProtocol -bor 3072; ";
function maskToken(t) {
  if (!t) return "";
  if (TOKEN_REVEALED) return t;
  if (t.length <= 8) return "••••••••";
  return t.slice(0, 4) + "••••••••" + t.slice(-4);
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
  // 日志采集（可选）：把用户填写的路径（换行/逗号分隔）拼进安装命令，服务端写入 config.json 的 log_paths
  const lp = (($("installLogPaths") && $("installLogPaths").value) || "").trim();
  if (lp) q += "&log_paths=" + encodeURIComponent(lp);
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
    cmd = `${PS_TLS12}irm "${server}/install.ps1?${q}" | iex`;
    label = I18N.t("install.powershell_cmd");
    hint = "普通 PowerShell 即可（命令已内置 TLS 1.2，兼容 Windows Server 2012 R2）；安装到 %LOCALAPPDATA%\\AIOps-agent 并注册用户级开机自启。";
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
    ? `${PS_TLS12}irm "${server}/uninstall.ps1" | iex`
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
    gatewayCmd = `${PS_TLS12}irm "${server}/install-relay.ps1?${q}" | iex`;
    internalCmd = `${PS_TLS12}irm "${relay}/install.ps1?${q}" | iex`;
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
    ? `${PS_TLS12}irm "${server}/uninstall.ps1" | iex`
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

/* ---------- 自定义监控 ---------- */
// 进程类目标形如 hostID/进程名，展示为「进程 @ 主机名」更友好。
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
// TCP 目标拆分为 主机 / 端口（末个冒号分隔）
function splitHostPort(t) {
  t = String(t || "");
  const i = t.lastIndexOf(":");
  if (i > 0) return { host: t.slice(0, i), port: t.slice(i + 1) };
  return { host: t, port: "" };
}
// 进程目标 hostID/进程名 拆分，并把 hostID 解析为主机名
function splitProcessTarget(c) {
  const t = String(c.target || "");
  const i = t.indexOf("/");
  if (i > 0) {
    const hid = t.slice(0, i), proc = t.slice(i + 1);
    const meta = HOST_META.find(h => h.id === hid);
    return { proc, hostName: meta ? (meta.hostname || hid.slice(0, 8)) : hid.slice(0, 8) };
  }
  return { proc: t, hostName: "—" };
}
// 详情项：键 + 值 + 值配色
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

  // 应用类型筛选 + 关键字搜索
  let shown = checks;
  if (CHECK_TYPE && CHECK_TYPE !== "all") shown = shown.filter(c => c.type === CHECK_TYPE);
  if (CHECK_SEARCH) { const q = CHECK_SEARCH.toLowerCase(); shown = shown.filter(c => ((c.name || "") + " " + (c.target || "")).toLowerCase().includes(q)); }

  grid.innerHTML = shown.map(c => {
    const st = !c.enabled ? "unknown" : (c.checked_at ? (c.ok ? "up" : "down") : "unknown");
    const stText = !c.enabled ? I18N.t("ui.disabled_status") : (c.checked_at ? (c.ok ? I18N.t("ui.normal") : I18N.t("ui.abnormal")) : I18N.t("ui.pending"));
    const typeText = c.type === "http" ? "HTTP" : c.type === "tcp" ? "TCP" : c.type === "ping" ? "Ping" : I18N.t("ui.process");
    const builtin = c.builtin ? ' data-builtin="1"' : "";
    const histBtn = `<button class="mini-btn" data-cact="hist" title="${I18N.t('ui.history_chart')}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 3v18h18"/><path d="M7 13l3-3 3 2 5-6"/></svg></button>`;
    const actions = `<span class="ch-actions">${histBtn}${c.builtin ? "" : `
          <button class="mini-btn" data-cact="run" title="${I18N.t('ui.check_now')}">▶</button>
          <button class="mini-btn" data-cact="edit" title="${I18N.t('ui.edit')}">✎</button>
          <button class="mini-btn del" data-cact="del" title="${I18N.t('ui.delete')}">✕</button>`}</span>`;
    const builtinTag = c.builtin ? `<span class="type-badge" style="background:var(--ok-soft);color:var(--ok-txt)">${I18N.t("ui.builtin")}</span>` : "";

    // 详情字段：按监控类型给出各自贴合的字段，三类监控信息量对齐
    const stCls = st === "up" ? "ok" : st === "down" ? "crit" : "muted";
    const lat = c.checked_at ? Math.round(c.latency_ms) + " ms" : "—";
    const latCls = c.checked_at ? "" : "muted";
    const detail = [];
    if (c.type === "http") {
      detail.push(cdItem(I18N.t("form.check_url"), checkTargetDisplay(c), "muted"));
      detail.push(cdItem(I18N.t("form.run_status"), stText, stCls));
      const code = c.status_code || 0;
      detail.push(cdItem(I18N.t("form.status_code"), code ? String(code) : "—", code === 0 ? "muted" : code >= 400 ? "crit" : "ok"));
      detail.push(cdItem(I18N.t("form.response_latency"), lat, latCls));
      if (typeof c.cert_days === "number" && c.cert_days >= 0) {
        const d = c.cert_days;
        detail.push(cdItem(I18N.t("form.cert_remaining"), d + I18N.t("time.days"), d <= 7 ? "crit" : d <= 30 ? "warn" : "ok"));
      }
    } else if (c.type === "tcp") {
      const hp = splitHostPort(c.target);
      detail.push(cdItem(I18N.t("form.target"), hp.host || c.target, "muted"));
      detail.push(cdItem(I18N.t("form.port"), hp.port || "—", ""));
      detail.push(cdItem(I18N.t("form.connect_status"), stText, stCls));
      detail.push(cdItem(I18N.t("form.connect_latency"), lat, latCls));
    } else if (c.type === "ping") {
      detail.push(cdItem(I18N.t("form.check_url"), c.target, "muted"));
      detail.push(cdItem(I18N.t("form.run_status"), stText, stCls));
      const loss = (typeof c.loss_pct === "number" && c.loss_pct >= 0) ? c.loss_pct : null;
      detail.push(cdItem(I18N.t("form.loss_rate"), loss === null ? "—" : Math.round(loss) + "%",
        loss === null ? "muted" : loss === 0 ? "ok" : loss >= 100 ? "crit" : "warn"));
      const hasRtt = c.checked_at && c.latency_ms > 0;
      detail.push(cdItem(I18N.t("form.avg_latency"), hasRtt ? Math.round(c.latency_ms) + " ms" : "—", hasRtt ? "" : "muted"));
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
// 列表 / 胶囊视图切换
function setCheckView(v) {
  CHECK_VIEW = v === "pill" ? "pill" : "list";
  try { localStorage.setItem("aiops_check_view", CHECK_VIEW); } catch (e) {}
  document.querySelectorAll("#checkViewToggle .vt-btn").forEach(b => b.classList.toggle("active", b.dataset.cview === CHECK_VIEW));
  renderChecks(LAST_CHECKS);
}
// 主机 卡片 / 列表 视图切换
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

// 把当前拨测监控快照汇总为纯文本，供 AI 分析（学习闭环：/ai/assist 自动沉淀记忆 + 👍/👎 强化）
function checksToText() {
  const checks = LAST_CHECKS || [];
  if (!checks.length) return "（当前没有任何拨测监控项）";
  let down = 0, disabled = 0;
  const lines = checks.map(c => {
    if (c.enabled === false) disabled++;
    const st = !c.enabled ? "已停用" : (c.checked_at ? (c.ok ? "正常" : "异常") : "未探测");
    if (c.enabled && c.checked_at && !c.ok) down++;
    const typeText = c.type === "http" ? "HTTP" : c.type === "tcp" ? "TCP" : c.type === "ping" ? "Ping" : c.type === "process" ? "进程" : (c.type || "");
    let extra = "";
    if (c.type === "http") extra = ` 状态码=${c.status_code || "—"}${(typeof c.cert_days === "number" && c.cert_days >= 0) ? " 证书剩余=" + c.cert_days + "天" : ""}`;
    else if (c.type === "ping" && typeof c.loss_pct === "number" && c.loss_pct >= 0) extra = ` 丢包=${Math.round(c.loss_pct)}%`;
    const lat = c.checked_at ? Math.round(c.latency_ms) + "ms" : "—";
    const err = (c.enabled && c.checked_at && !c.ok && c.message) ? " 错误=" + c.message : "";
    return `- [${typeText}] ${c.name} 目标=${checkTargetDisplay(c)} 状态=${st} 时延=${lat}${extra} 间隔=${c.interval_sec}s${err}`;
  });
  const head = `拨测项共 ${checks.length} 个 · 异常 ${down} 个 · 停用 ${disabled} 个。\n`;
  return (head + lines.join("\n")).slice(0, 12000);
}

// 「🤖 AI 分析」：对当前所有拨测项的可用性/时延/证书/丢包做整体研判，结果自动进入 RAG 记忆闭环
safeAddEventListener("checksAIBtn", "click", () => {
  if (typeof openAIAssist !== "function") { if (typeof toast === "function") toast(I18N.t("assist.unavailable", "AI 面板未就绪"), "err"); return; }
  openAIAssist({ task: "checks_diagnosis", mode: "analyze", title: I18N.t("assist.title_checks", "AI · 拨测监控分析"), context: checksToText() });
});

let CHK_CHARTS = {};
let CHK_HIST = { id: "", name: "", type: "", range: 1, custom: null }; // range=小时数，默认 1h；custom={from,to}
// 自定义监控·历史曲线：复用交互式图表引擎，支持按时间范围筛选 + 自定义绝对区间（与主机趋势图一致）
function openCheckHistory(id, name, type) {
  CHK_HIST = { id, name, type, range: 1, custom: null };
  $("checkHistTitle").textContent = name + " · 监控历史";
  $("checkHistMask").classList.add("show");
  loadCheckHistory();
}
async function loadCheckHistory() {
  const { id, name, type, range, custom } = CHK_HIST;
  const body = $("checkHistBody");
  body.innerHTML = `<div class="empty-line">加载中…</div>`;
  const now = Math.floor(Date.now() / 1000);
  const from = custom ? custom.from : (range > 0 ? now - range * 3600 : 0);
  const to = custom ? custom.to : now;
  // 快捷跨度按钮 + 自定义绝对区间（与主机趋势图一致）
  const ctrl = `${renderChartControls(custom ? -1 : range, "crange")}
    <button class="chip-btn ${custom ? "active" : ""}" data-chk-custom-toggle title="${I18N.t("time.custom_range") || "自定义时间范围"}">${I18N.t("time.custom") || "自定义"}</button>
    <span class="chart-custom-range" id="chkCustomPanel"${custom ? "" : " hidden"}>
      <input type="datetime-local" id="chkCustomFrom" class="dt-input" value="${toLocalDatetimeValue(from > 0 ? from : now - 3600)}">
      <span class="dt-sep">→</span>
      <input type="datetime-local" id="chkCustomTo" class="dt-input" value="${toLocalDatetimeValue(to)}">
      <button class="chip-btn primary" data-chk-custom-apply>${I18N.t("time.custom_apply") || "应用"}</button>
    </span>`;
  try {
    const all = await fetch(`${API}/${CHK_HIST.base || "checks"}/${encodeURIComponent(id)}/history`).then(r => r.json());
    const pts = (Array.isArray(all) ? all : []).filter(p => p.timestamp >= from && (custom ? p.timestamp <= to : true));
    if (!pts.length) {
      body.innerHTML = `<div class="chart-controls">${ctrl}</div><div class="empty-line">该时间范围暂无数据（检查运行一段时间后自动积累，重启后重新计）</div>`;
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
      <div class="hint">采样 ${pts.length} 个 · 时间跨度 ${span} · 可用率 ${uptime}% · 平均延时 ${avgLat} ${I18N.t("unit.ms")} · 悬停查看数值，拖动框选放大，双击还原。</div>`;
    CHK_CHARTS = {};
    CHK_CHARTS.chkLat = createChart("chkLat", samples, [
      { key: "latency_ms", label: isPing ? I18N.t("form.avg_latency") : I18N.t("form.latency"), color: "#4c8dff", fmt: v => v.toFixed(0) + " " + I18N.t("unit.ms") },
    ], 0, null, { title: name + " · " + I18N.t("form.latency") + "(" + I18N.t("unit.ms") + ")" });
    if (isPing) {
      CHK_CHARTS.chkLoss = createChart("chkLoss", samples, [
        { key: "loss_pct", label: I18N.t("form.loss_rate"), color: "#f2545b", fmt: v => v.toFixed(0) + "%" },
      ], 0, 100, { title: name + " · 丢包率(%)" });
    }
  } catch (e) {
    body.innerHTML = `<div class="empty-line">加载失败: ${esc(e)}</div>`;
  }
}
// 历史弹窗：时间范围切换（快捷/自定义）+ 图表放大委托
safeAddEventListener("checkHistBody", "click", e => {
  const tog = e.target.closest("[data-chk-custom-toggle]");
  if (tog) { const p = $("chkCustomPanel"); if (p) { p.hidden = !p.hidden; if (!p.hidden) { const f = $("chkCustomFrom"); if (f) f.focus(); } } return; }
  if (e.target.closest("[data-chk-custom-apply]")) { applyChkCustomRange(); return; }
  const rb = e.target.closest(".chip-btn[data-crange]");
  if (rb) { CHK_HIST.custom = null; CHK_HIST.range = parseInt(rb.dataset.crange); loadCheckHistory(); return; }
  const en = e.target.closest(".chart-enlarge"); if (!en) return;
  const ch = CHK_CHARTS[en.dataset.chart]; if (ch) openChartZoom(ch);
});
// 读取两个 datetime-local 输入，校验后按自定义绝对区间重新拉取（与主机趋势图一致）
function applyChkCustomRange() {
  const fEl = $("chkCustomFrom"), tEl = $("chkCustomTo");
  if (!fEl || !tEl || !fEl.value || !tEl.value) { toast(I18N.t("time.custom_incomplete") || "请选择开始和结束时间", "warn"); return; }
  const from = Math.floor(new Date(fEl.value).getTime() / 1000);
  const to = Math.floor(new Date(tEl.value).getTime() / 1000);
  if (!Number.isFinite(from) || !Number.isFinite(to)) { toast(I18N.t("time.custom_invalid") || "时间格式无效", "err"); return; }
  if (to <= from) { toast(I18N.t("time.custom_order") || "结束时间必须晚于开始时间", "warn"); return; }
  if (to - from < 60) { toast(I18N.t("time.custom_tooshort") || "时间范围太短（至少 1 分钟）", "warn"); return; }
  CHK_HIST.custom = { from, to };
  loadCheckHistory();
}
async function loadHostsMeta() {
  try { HOST_META = await fetch(`${API}/hosts/meta`).then(r => r.json()); } catch (e) { /* ignore */ }
}
function updateCkTargetLabel() {
  const t = $("ckType").value;
  const adv = $("ckAdvancedWrap"); if (adv) adv.style.display = (t === "http") ? "" : "none"; // 高级模式仅 HTTP
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
  } else if (t === "udp") {
    $("ckTargetLabel").textContent = I18N.t("form.host_port");
    $("ckTarget").placeholder = "127.0.0.1:53";
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
  // HTTP 高级模式回填
  $("ckAdvanced").checked = !!(check && check.advanced);
  $("ckMethod").value = (check && check.method) || "GET";
  $("ckExpectStatus").value = (check && check.expect_status) ? check.expect_status : "";
  $("ckHeaders").value = (check && check.headers) ? Object.entries(check.headers).map(([k, v]) => `${k}: ${v}`).join("\n") : "";
  $("ckBody").value = (check && check.body) || "";
  $("ckExpectKeyword").value = (check && check.expect_keyword) || "";
  $("ckKeywordRegex").checked = !!(check && check.keyword_is_regex);
  $("ckJsonPath").value = (check && check.json_path) || "";
  $("ckJsonExpect").value = (check && check.json_expect) || "";
  $("ckCertWarnDays").value = (check && check.cert_warn_days) ? check.cert_warn_days : "";
  $("ckAdvancedBody").style.display = $("ckAdvanced").checked ? "" : "none";
  // Populate host select for process type
  populateHostSelect(check);
  updateCkTargetLabel();
  $("checkMask").classList.add("show");
}
function populateHostSelect(check) {
  const sel = $("ckHost");
  sel.innerHTML = `<option value="">-- 选择主机 --</option>` + HOST_META.map(h =>
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
  if (type === "http" && $("ckAdvanced").checked) { // HTTP 高级模式字段
    body.advanced = true;
    body.method = $("ckMethod").value;
    const hs = {};
    ($("ckHeaders").value || "").split("\n").forEach(line => { const i = line.indexOf(":"); if (i > 0) { const k = line.slice(0, i).trim(); if (k) hs[k] = line.slice(i + 1).trim(); } });
    body.headers = hs;
    body.body = $("ckBody").value;
    body.expect_status = parseInt($("ckExpectStatus").value) || 0;
    body.expect_keyword = $("ckExpectKeyword").value.trim();
    body.keyword_is_regex = $("ckKeywordRegex").checked;
    body.json_path = $("ckJsonPath").value.trim();
    body.json_expect = $("ckJsonExpect").value.trim();
    body.cert_warn_days = parseInt($("ckCertWarnDays").value) || 0;
  }
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

/* ---------- 账户 / 个人信息 ---------- */
let CUR_ROLE = "";
const roleLabel = r => ({ admin: I18N.t("ui.admin"), operator: I18N.t("ui.operator"), viewer: I18N.t("ui.readonly") }[r] || r || "");
const canWrite = () => CUR_ROLE === "operator" || CUR_ROLE === "admin";
const isAdmin = () => CUR_ROLE === "admin";
function setUser(me) {
  const name = me.display_name || me.username || I18N.t("ui.user");
  const initial = (name[0] || "A");
  const roleLabels = { admin: "管理员", operator: "操作员", viewer: "查看者" };
  // 顶栏按钮
  var el = $("userName"); if (el) el.textContent = name;
  el = $("userAvatar"); if (el) el.textContent = initial;
  // 下拉菜单大图
  el = $("userNameLg"); if (el) el.textContent = name;
  el = $("userAvatarLg"); if (el) el.textContent = initial;
  el = $("userRoleLg"); if (el) el.textContent = roleLabels[me.role] || me.role || "—";
  if (me.role) {
    CUR_ROLE = me.role;
    document.body.dataset.role = me.role;
  }
}
// fetchWithTimeout wraps fetch with an AbortController timeout so mobile
// browsers on slow/unstable networks don't hang indefinitely. Returns the
// Response or throws an AbortError / network error.
function fetchWithTimeout(url, opts, timeoutMs) {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), timeoutMs || 15000);
  return fetch(url, Object.assign({}, opts, { signal: ctrl.signal })).finally(() => clearTimeout(timer));
}
async function initAuth() {
  try {
    const r = await fetchWithTimeout(`${API}/me`, {}, 10000);
    if (r.ok) {
      const me = await r.json();
      setUser(me);
      $("loginView").classList.remove("show");
      startApp();
      // v5.4.0: force password change if admin reset was used
      if (me.must_change_password) {
        // 强制进入「安全初始化」弹窗：需修改用户名 + 密码后方可进入控制台
        setTimeout(() => openInitSetup(), 300);
      }
    }
    else { $("loginView").classList.add("show"); }
  } catch (e) {
    // Network error on initial auth check — show login with a friendly hint
    // instead of a raw "Failed to fetch" that confuses mobile users.
    $("loginView").classList.add("show");
    const loginErrEl = $("loginErr");
    if (loginErrEl) loginErrEl.textContent = I18N.t("toast.network_check_failed");
  }
}
/* ---------- 消息中心（顶栏铃铛 + 未读徽标 + 下拉面板） ---------- */
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
  // P1-2: 差异化轮询频率 — 按当前视图 + 标签页可见性调整刷新间隔
  const POLL_BASE = 5000;
  let pollTimer = null;
  function schedulePoll() {
    if (pollTimer) clearTimeout(pollTimer);
    const view = document.querySelector(".view.active")?.id.replace("view-", "") || "overview";
    const intervals = { overview: 5000, hosts: 5000, checks: 10000, alerts: 5000, automation: 15000, forward: 15000, log: 10000 };
    let interval = intervals[view] || POLL_BASE;
    // 后台标签页降频至 15s，减少不必要的网络请求和 DOM 渲染
    if (document.visibilityState === "hidden") interval = Math.max(interval, 15000);
    pollTimer = setTimeout(() => { refresh(); loadChecks(); if (document.querySelector("#view-forward.active")) loadForwards(); schedulePoll(); }, interval);
  }
  schedulePoll();
  // 视图切换时立即调整轮询频率
  document.querySelectorAll(".nav-item").forEach(n => n.addEventListener("click", () => setTimeout(schedulePoll, 100)));
  // 标签页可见性变化时重排轮询
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") { refresh(true); schedulePoll(); }
  });
  // P3-1: 初始化 WebSocket 推送（带降级到轮询）
  initPushWS();
}
// 首次登录 · 安全初始化：强制修改用户名 + 密码的专用弹窗（替代直接打开个人信息页）。
// 弹窗带 data-forced，无法通过 ESC / 点遮罩 / ✕ 关闭；完成后会话重签并刷新进入。
async function openInitSetup() {
  try {
    const me = await fetch(`${API}/me`).then(r => r.json()).catch(() => ({}));
    const u = $("initUser"); if (u) u.value = me.username || "";
    const p = $("initPass"); if (p) p.value = "";
    const p2 = $("initPass2"); if (p2) p2.value = "";
    const err = $("initErr"); if (err) { err.textContent = ""; err.style.display = "none"; }
    const mask = $("initSetupMask"); if (mask) mask.classList.add("show");
    if (u) setTimeout(() => u.focus(), 60);
  } catch (e) { toast(I18N.t("toast.read_failed2") + e, "err"); }
}
async function submitInitSetup() {
  const err = $("initErr");
  const showErr = (m) => { if (err) { err.textContent = m; err.style.display = "block"; } else toast(m, "err"); };
  if (err) { err.textContent = ""; err.style.display = "none"; }
  const uname = ($("initUser").value || "").trim();
  const pw = $("initPass").value || "";
  const pw2 = $("initPass2").value || "";
  if (!uname) { showErr(I18N.t("init.err_username", "请输入登录用户名")); return; }
  if (!pw) { showErr(I18N.t("init.err_password", "请输入新密码")); return; }
  if (pw !== pw2) { showErr(I18N.t("init.err_mismatch", "两次输入的密码不一致")); return; }
  if (pw.length < 8) { showErr(I18N.t("auth.password_policy", "密码需至少 8 位，含大小写字母、数字和特殊字符")); return; }
  await withLoading($("initSubmitBtn"), async () => {
    try {
      const r = await fetch(`${API}/account/init`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: uname, password: pw })
      });
      const j = await r.json().catch(() => ({}));
      if (r.ok) {
        const mask = $("initSetupMask"); if (mask) mask.classList.remove("show");
        // 后端已清除会话并要求重新登录（relogin:true）：不再进入控制台，
        // 而是提示并跳转到登录页，强制用新的用户名/密码重新登录。
        toast(I18N.t("init.relogin", "初始化完成，请用新的用户名和密码重新登录"), "ok");
        setTimeout(() => location.reload(), 1000);
      } else {
        showErr(j.error || I18N.t("toast.save_failed"));
      }
    } catch (e) { showErr(I18N.t("toast.save_failed2") + e); }
  });
}
async function openProfile(tab) {
  try {
    const me = await fetch(`${API}/me`).then(r => r.json());
    $("pfUsername").value = me.username || "";
    $("pfDisplay").value = me.display_name || "";
    $("pfEmail").value = me.email || "";
    $("pfPhone").value = me.phone || "";
    $("pfOld").value = ""; $("pfNew").value = "";
    setUser(me); // 用最新 /me 刷新顶栏与 CUR_ROLE（角色可能已变更）
    // 清空各 Tab 内联错误
    ["pfProfileErr", "pfPwdErr", "pfTermPwdErr"].forEach(id => { const e = $(id); if (e) { e.textContent = ""; e.style.display = "none"; } });
    renderMfaState(!!me.mfa_enabled);
    // v5.3.0: 加载终端密码状态
    loadTermPwdStatus();
    $("profileMask").classList.add("show");
    // 切换到底层请求指定的 Tab（默认「个人信息」）；非管理员无法进入用户管理
    const target = (tab === "users" && !isAdmin()) ? "info" : (tab || "info");
    switchProfileTab(target);
  } catch (e) { toast(I18N.t("toast.read_failed2") + e, "err"); }
}
let PROFILE_TAB = "info";
let PROFILE_USERS_LOADED = false;
async function switchProfileTab(tab) {
  PROFILE_TAB = tab;
  document.querySelectorAll("#profileTabs .tab").forEach(b => b.classList.toggle("active", b.dataset.ptab === tab));
  document.querySelectorAll("#profileMask .tab-panel").forEach(p => p.classList.toggle("active", p.id === "tab-profile-" + tab));
  // 用户管理 Tab：首次进入时按需独立加载（保持其它 Tab 状态不重渲染）
  if (tab === "users" && isAdmin() && !PROFILE_USERS_LOADED) {
    PROFILE_USERS_LOADED = true;
    await loadUsers();
  }
}
async function saveProfile() {
  const errEl = $("pfProfileErr");
  if (errEl) { errEl.textContent = ""; errEl.style.display = "none"; }
  try {
    const uname = $("pfUsername").value.trim();
    const r = await fetch(`${API}/profile`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: uname, display_name: $("pfDisplay").value.trim(), email: $("pfEmail").value.trim(), phone: $("pfPhone").value.trim() })
    });
    const j = await r.json().catch(() => ({}));
    if (r.ok) { toast(I18N.t("toast.profile_saved"), "ok"); setUser({ display_name: $("pfDisplay").value.trim(), username: j.username || uname }); }
    else if (errEl) { errEl.textContent = j.error || I18N.t("toast.save_failed"); errEl.style.display = "block"; }
    else toast(j.error || I18N.t("toast.save_failed"), "err");
  } catch (e) { toast(I18N.t("toast.save_failed2") + e, "err"); }
}
async function changePassword() {
  const errEl = $("pfPwdErr");
  if (errEl) { errEl.textContent = ""; errEl.style.display = "none"; }
  if (!$("pfOld").value || !$("pfNew").value) {
    if (errEl) { errEl.textContent = I18N.t("valid.fill_passwords"); errEl.style.display = "block"; }
    else toast(I18N.t("valid.fill_passwords"), "err");
    return;
  }
  if (!pwPolicyOK($("pfNew").value)) {
    if (errEl) { errEl.textContent = I18N.t("auth.password_policy"); errEl.style.display = "block"; }
    else toast(I18N.t("auth.password_policy"), "err");
    return;
  }
  try {
    const r = await fetch(`${API}/password`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ old: $("pfOld").value, new: $("pfNew").value })
    });
    const j = await r.json();
    if (r.ok) { toast(I18N.t("toast.password_changed"), "ok"); $("pfOld").value = ""; $("pfNew").value = ""; }
    else if (errEl) { errEl.textContent = j.error || I18N.t("toast.update_failed"); errEl.style.display = "block"; }
    else toast(j.error || I18N.t("toast.update_failed"), "err");
  } catch (e) { toast(I18N.t("toast.update_failed2") + e, "err"); }
}

/* ===================== v5.3.0: 终端密码管理（个人信息页） ===================== */
let TERM_PWD_CHANGE_SHOWING = false;

async function loadTermPwdStatus() {
  try {
    const r = await fetch("/api/user/terminal-password/status", { credentials: "include" });
    const j = await r.json().catch(() => ({}));
    const valEl = $("pfTermPwdStatusVal");
    if (valEl) {
      if (j.has_password) {
        valEl.textContent = I18N.t("term_auth.password_set");
        valEl.className = "term-pwd-status-val set";
      } else {
        valEl.textContent = I18N.t("term_auth.no_password_set");
        valEl.className = "term-pwd-status-val unset";
      }
    }
  } catch (e) { /* 静默失败 */ }
}

function toggleTermPwdChange() {
  TERM_PWD_CHANGE_SHOWING = !TERM_PWD_CHANGE_SHOWING;
  const authField = $("pfTermPwdAuthField");
  const newField = $("pfTermPwdNewField");
  const errEl = $("pfTermPwdErr");
  const btn = $("pfTermPwdBtn");

  if (TERM_PWD_CHANGE_SHOWING) {
    // 显示修改表单
    $("pfTermPwdAuth").value = "";
    $("pfTermPwdNew").value = "";
    if (errEl) { errEl.textContent = ""; errEl.style.display = "none"; }
    if (authField) authField.style.display = "block";
    if (newField) newField.style.display = "block";
    if (btn) btn.textContent = I18N.t("ui.cancel");
    // 根据 MFA 状态调整验证字段标签
    const authLabel = $("pfTermPwdAuthLabel");
    if (authLabel) {
      authLabel.textContent = MFA_ENABLED ? I18N.t("term_auth.mfa_code") : I18N.t("term_auth.current_password");
    }
    $("pfTermPwdAuth").placeholder = MFA_ENABLED ? I18N.t("mfa.code_6") : "";
    $("pfTermPwdAuth").maxLength = MFA_ENABLED ? 6 : 524288;
  } else {
    // 隐藏修改表单
    if (authField) authField.style.display = "none";
    if (newField) newField.style.display = "none";
    if (errEl) { errEl.textContent = ""; errEl.style.display = "none"; }
    if (btn) btn.textContent = I18N.t("term_auth.change_password_btn");
  }
}

async function submitTermPwdChange() {
  if (!TERM_PWD_CHANGE_SHOWING) {
    toggleTermPwdChange();
    return;
  }
  const code = $("pfTermPwdAuth").value.trim();
  const newPwd = $("pfTermPwdNew").value.trim();
  const errEl = $("pfTermPwdErr");

  if (!code || !newPwd) {
    if (errEl) { errEl.textContent = I18N.t("term_auth.fill_verify_password"); errEl.style.display = "block"; }
    return;
  }

  try {
    const r = await fetch("/api/user/terminal-password/set", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password: newPwd, code: code })
    });
    const j = await r.json().catch(() => ({}));

    if (r.ok) {
      toast(I18N.t("term_auth.changed_ok"), "ok");
      toggleTermPwdChange(); // 收起表单
      loadTermPwdStatus();   // 刷新状态
    } else {
      if (j.mfa_required) {
        // 修改时需要 MFA，但未提供
        if (errEl) { errEl.textContent = I18N.t("term_auth.enter_mfa_code"); errEl.style.display = "block"; }
        return;
      }
      if (errEl) { errEl.textContent = j.error || I18N.t("toast.update_failed"); errEl.style.display = "block"; }
    }
  } catch (e) {
    if (errEl) { errEl.textContent = I18N.t("toast.network_error"); errEl.style.display = "block"; }
  }
}

/* ===================== 两步验证（TOTP / Google Authenticator） ===================== */
let MFA_ENABLED = false;
function renderMfaState(enabled) {
  MFA_ENABLED = enabled;
  const st = $("mfaState"), chk = $("mfaToggleChk");
  if (st) { st.textContent = enabled ? I18N.t("toast.enabled") : I18N.t("toast.disabled"); st.className = "mfa-state " + (enabled ? "on" : "off"); }
  if (chk) { chk.checked = enabled; }
}
async function openMfaSetup(forced) {
  const body = $("mfaBody");
  $("mfaTitle").textContent = forced ? I18N.t("ui.mfa_required") : I18N.t("ui.enable_mfa");
  body.innerHTML = `<div class="empty-line">正在生成密钥…</div>`;
  $("mfaMask").classList.add("show");
  let data;
  try { data = await fetch(`${API}/mfa/setup`, { method: "POST" }).then(r => r.json()); }
  catch (e) { body.innerHTML = `<div class="empty-line">生成失败：${esc(e)}</div>`; return; }
  const secret = data.secret || "", qrURI = data.qr_datauri || "";
  const grp = secret.replace(/(.{4})/g, "$1 ").trim();
  body.innerHTML = `
    ${forced ? `<div class="mfa-desc" style="margin-bottom:10px;color:var(--warn-txt,#f2c078)">管理员已启用全局两步验证策略，请完成绑定后登录。</div>` : ""}
    <ol class="mfa-steps">
      <li>打开 <b>Google Authenticator</b>（或任意 TOTP 应用），扫描二维码；无法扫码时可手动输入下方密钥。</li>
      <li>输入应用当前显示的 6 位动态口令，点「确认启用」。</li>
    </ol>
    <div class="mfa-qr" id="mfaQr"></div>
    <div class="mfa-secret">${I18N.t("mfa.secret_label")}　<code class="mono" id="mfaSecret">${esc(grp)}</code><button class="btn ghost sm" id="mfaCopy" type="button">${I18N.t("mfa.copy_btn")}</button></div>
    <div class="field"><label>${I18N.t("form.totp_code")}</label><input type="text" id="mfaCode" inputmode="numeric" maxlength="6" placeholder="${I18N.t('mfa.code_6')}" autocomplete="one-time-code"></div>
    <div class="login-err" id="mfaErr"></div>
    <div class="mfa-foot"><button class="btn primary" id="mfaConfirm" type="button">${I18N.t("mfa.confirm_enable")}</button></div>`;
  if (qrURI) $("mfaQr").innerHTML = `<img src="${esc(qrURI)}" alt="MFA QR Code" class="qr-img">`;
  else $("mfaQr").innerHTML = `<div class="mfa-desc">二维码不可用，请在应用中手动输入上方密钥。</div>`;
  $("mfaCopy").onclick = () => { try { navigator.clipboard.writeText(secret); toast(I18N.t("toast.secret_copied"), "ok"); } catch (_) { } };
  $("mfaConfirm").onclick = async () => {
    const errEl = $("mfaErr"); errEl.textContent = "";
    const code = $("mfaCode").value.trim();
    if (code.length !== 6) { errEl.textContent = I18N.t("valid.enter_totp"); return; }
    const r = await fetch(`${API}/mfa/enable`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ secret, code }) });
    const j = await r.json().catch(() => ({}));
    if (r.ok) {
      toast(I18N.t("toast.mfa_enabled"), "ok");
      $("mfaMask").classList.remove("show");
      if (forced) {
        // Global MFA enforcement: complete login after enrollment.
        setUser(await fetch(`${API}/me`).then(x => x.json()));
        const lv = $("loginView"); if (lv) lv.classList.remove("show");
        startApp();
      } else { renderMfaState(true); }
    }
    else errEl.textContent = j.error || I18N.t("toast.enable_failed");
  };
  setTimeout(() => { const el = $("mfaCode"); if (el) el.focus(); }, 60);
}
function openMfaDisable() {
  const body = $("mfaBody");
  $("mfaTitle").textContent = I18N.t("ui.disable_mfa");
  body.innerHTML = `
    <div class="mfa-desc" style="margin-bottom:14px">关闭后，登录将不再需要动态口令。请选择验证方式：</div>
    <div class="field"><label>${I18N.t("form.password")}</label><input type="password" id="mfaPass" autocomplete="current-password"></div>
    <div class="login-err" id="mfaErr"></div>
    <div class="mfa-foot">
      <button class="btn danger" id="mfaConfirmOff" type="button">${I18N.t("mfa.disable_pwd")}</button>
      <button class="btn" id="mfaEmailUnbind" type="button">${I18N.t("mfa.email_unbind_btn")}</button>
    </div>`;
  $("mfaMask").classList.add("show");
  $("mfaConfirmOff").onclick = async () => {
    const errEl = $("mfaErr"); errEl.textContent = "";
    const r = await fetch(`${API}/mfa/disable`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ password: $("mfaPass").value }) });
    const j = await r.json().catch(() => ({}));
    if (r.ok) { toast(I18N.t("toast.mfa_disabled"), "ok"); $("mfaMask").classList.remove("show"); renderMfaState(false); }
    else errEl.textContent = j.error || I18N.t("toast.disable_failed");
  };
  $("mfaEmailUnbind").onclick = () => openMfaEmailUnbind();
  setTimeout(() => { const el = $("mfaPass"); if (el) el.focus(); }, 60);
}

/* ---------- 通过邮箱验证码解除 MFA ---------- */
function openMfaEmailUnbind() {
  const body = $("mfaBody");
  $("mfaTitle").textContent = I18N.t("ui.unbind_mfa_email");
  body.innerHTML = `
    <div class="mfa-desc" style="margin-bottom:14px">系统将向已绑定邮箱发送 6 位验证码，验证通过后关闭两步验证。</div>
    <div class="login-err" id="mfaErr"></div>
    <div class="mfa-foot">
      <button class="btn primary" id="mfaSendCode" type="button">${I18N.t("mfa.send_code_btn")}</button>
      <span style="flex:1"></span>
    </div>
    <div class="field" id="mfaCodeRow" style="display:none">
      <label>${I18N.t("form.email_code")}</label>
      <input type="text" id="mfaEmailCode" inputmode="numeric" maxlength="6" placeholder="${I18N.t('mfa.code_6_v2')}" autocomplete="one-time-code">
    </div>
    <div class="mfa-foot" id="mfaVerifyRow" style="display:none">
      <button class="btn danger" id="mfaConfirmEmailUnbind" type="button">${I18N.t("mfa.confirm_unbind")}</button>
    </div>`;
  $("mfaMask").classList.add("show");
  $("mfaSendCode").onclick = async () => {
    const errEl = $("mfaErr"); errEl.textContent = "";
    const r = await fetch(`${API}/mfa/unbind-via-email`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ action: "send_code" }) });
    const j = await r.json().catch(() => ({}));
    if (r.ok) {
      toast(I18N.t("toast.code_sent"), "ok");
      $("mfaSendCode").textContent = I18N.t("ui.resend");
      $("mfaSendCode").disabled = true;
      setTimeout(() => { const b = $("mfaSendCode"); if (b) { b.disabled = false; } }, 60000);
      $("mfaCodeRow").style.display = "";
      $("mfaVerifyRow").style.display = "";
      setTimeout(() => { const el = $("mfaEmailCode"); if (el) el.focus(); }, 60);
    } else {
      errEl.textContent = j.error || I18N.t("toast.send_failed");
    }
  };
  $("mfaConfirmEmailUnbind").onclick = async () => {
    const errEl = $("mfaErr"); errEl.textContent = "";
    const code = $("mfaEmailCode").value.trim();
    if (code.length !== 6) { errEl.textContent = I18N.t("valid.enter_code"); return; }
    const r = await fetch(`${API}/mfa/unbind-via-email`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ action: "verify", code }) });
    const j = await r.json().catch(() => ({}));
    if (r.ok) { toast(I18N.t("toast.mfa_unbind_email"), "ok"); $("mfaMask").classList.remove("show"); renderMfaState(false); }
    else errEl.textContent = j.error || I18N.t("toast.unbind_failed");
  };
}

/* ---------- 用户管理（管理员）---------- */
async function openUsers() {
  // 用户管理已并入「个人信息」四 Tab 布局中的「用户管理」分页
  openProfile("users");
}
async function loadUsers() {
  // Fetch global MFA policy status
  try {
    const gm = await fetch(`${API}/mfa/global`).then(r => r.json());
    const chk = $("globalMfaChk");
    if (chk) { chk.checked = !!gm.mfa_required; chk.disabled = false; }
  } catch (_) { /* non-admin or error — switch stays disabled */ }
  const list = $("usersList");
  list.innerHTML = `<div class="empty-line">加载中…</div>`;
  let users;
  try { users = await fetch(`${API}/users`).then(r => r.json()); }
  catch (e) { list.innerHTML = `<div class="empty-line">加载失败: ${esc(e)}</div>`; return; }
  if (!Array.isArray(users) || !users.length) { list.innerHTML = `<div class="empty-line">${I18N.t("empty.no_users")}</div>`; return; }
  list.innerHTML = users.map(u => `
    <div class="user-row" data-name="${esc(u.username)}">
      <div class="user-info">
        <div class="user-main"><span class="user-name">${esc(u.username)}</span>
          <span class="role-badge role-${esc(u.role)}">${roleLabel(u.role)}</span>
          ${u.mfa_enabled ? `<span class="user-mfa" title="${I18N.t('mfa.enabled_badge')}">${I18N.t('mfa.enabled_badge')}</span>` : ""}</div>
        <div class="user-sub">${esc(u.display_name || "—")}${u.email ? " · " + esc(u.email) : ""}</div>
      </div>
      <div class="user-acts">
        <button class="btn ghost sm" data-act="edit">${I18N.t("ui.edit")}</button>
        <button class="btn ghost sm" data-act="pwd">${I18N.t("ui.reset_password")}</button>
        ${u.mfa_enabled ? `<button class="btn ghost sm" data-act="mfa">${I18N.t("ui.unbind_mfa")}</button>` : ""}
        <button class="btn ghost sm ubtn-del" data-act="del">${I18N.t("ui.delete")}</button>
      </div>
    </div>`).join("");
}
function openUserEdit(user) {
  const isNew = !user;
  $("userEditTitle").textContent = isNew ? I18N.t("ui.new_user") : I18N.t("ui.edit_user") + user.username;
  const roleOpts = ["admin", "operator", "viewer"].map(r => `<option value="${r}" ${user && user.role === r ? "selected" : ""}>${roleLabel(r)}</option>`).join("");
  $("userEditBody").innerHTML = `
    ${isNew ? `<div class="field"><label>${I18N.t("form.username")}</label><input type="text" id="ueName" placeholder="${I18N.t('form.username_format')}"></div>
    <div class="field"><label>${I18N.t("form.initial_password")}</label><input type="password" id="uePass"></div>` : ""}
    <div class="field"><label>${I18N.t("form.display_name")}</label><input type="text" id="ueDisplay" value="${user ? esc(user.display_name || "") : ""}" placeholder="${I18N.t('form.hint_display_name')}"></div>
    <div class="field"><label>${I18N.t("form.email_optional")}</label><input type="text" id="ueEmail" value="${user ? esc(user.email || "") : ""}" placeholder="name@example.com"></div>
    <div class="field"><label>${I18N.t("form.role")}</label><div class="select-wrap"><select id="ueRole">${roleOpts}</select></div></div>
    <div class="login-err" id="ueErr"></div>
    <div class="mfa-foot"><button class="btn primary" id="ueSave" type="button">${isNew ? I18N.t("ui.create_user") : I18N.t("ui.save")}</button></div>`;
  $("userEditMask").classList.add("show");
  $("ueSave").onclick = async () => {
    const errEl = $("ueErr"); errEl.textContent = "";
    const body = { display_name: $("ueDisplay").value.trim(), email: $("ueEmail").value.trim(), role: $("ueRole").value };
    let r;
    if (isNew) {
      body.username = $("ueName").value.trim();
      body.password = $("uePass").value;
      r = await fetch(`${API}/users`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    } else {
      r = await fetch(`${API}/users/${encodeURIComponent(user.username)}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    }
    const j = await r.json().catch(() => ({}));
    if (r.ok) { toast(isNew ? I18N.t("toast.user_created") : I18N.t("toast.saved"), "ok"); $("userEditMask").classList.remove("show"); loadUsers(); }
    else errEl.textContent = j.error || I18N.t("toast.operation_failed");
  };
}
async function usersAction(name, act) {
  if (act === "del") {
    // 两步确认：防止误删敏感操作
    if (!confirm(`⚠ 确定删除用户「${name}」？\n\n该操作不可撤销，该用户的所有会话将立即失效。\n如需继续，请点击「确定」。`)) return;
    const r = await fetch(`${API}/users/${encodeURIComponent(name)}`, { method: "DELETE" });
    const j = await r.json().catch(() => ({}));
    if (r.ok) { toast(I18N.t("toast.user_deleted"), "ok"); loadUsers(); } else toast(j.error || I18N.t("toast.delete_failed"), "err");
  } else if (act === "pwd") {
    const pass = prompt(`为「${name}」设置新密码（至少 8 位）：`);
    if (pass == null) return;
    if (!pwPolicyOK(pass.trim())) { toast(I18N.t("auth.password_policy"), "err"); return; }
    const r = await fetch(`${API}/users/${encodeURIComponent(name)}/reset-password`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ password: pass }) });
    const j = await r.json().catch(() => ({}));
    if (r.ok) toast(I18N.t("toast.password_reset"), "ok"); else toast(j.error || I18N.t("toast.reset_failed"), "err");
  } else if (act === "mfa") {
    if (!confirm(`确定解除「${name}」的两步验证绑定？`)) return;
    const r = await fetch(`${API}/users/${encodeURIComponent(name)}/reset-mfa`, { method: "POST" });
    const j = await r.json().catch(() => ({}));
    if (r.ok) { toast(I18N.t("toast.mfa_unbound"), "ok"); loadUsers(); } else toast(j.error || I18N.t("toast.operation_failed"), "err");
  }
}

/* ---------- 账户找回：用户名 / 密码 ---------- */
// New dual-verification flow (email code + optional MFA TOTP)
function openRecoverUser(e) { if (e) e.preventDefault(); showRecoverFlow('recover_username'); }
function openRecoverPass(e) { if (e) e.preventDefault(); showRecoverFlow('recover_password'); }

function showRecoverFlow(purpose) {
  const body = $("recoverBody");
  $("recoverTitle").textContent = I18N.t("recover.title");
  const label = purpose === 'recover_username' ? I18N.t("login.forgot_user") : I18N.t("login.forgot_pass");
  body.innerHTML = `
    <div class="mfa-desc" style="margin-bottom:14px">${I18N.t("recover.enter_email_desc")}</div>
    <div class="field"><label>${I18N.t("form.email")}</label><input type="text" id="rcEmail" placeholder="name@example.com" autocomplete="email"></div>
    <div class="login-err" id="rcErr"></div>
    <div class="mfa-foot"><button class="btn primary" id="rcAction" type="button">${I18N.t("mfa.send_code_btn")}</button></div>`;
  $("recoverMask").classList.add("show");

  $("rcAction").onclick = async () => {
    const errEl = $("rcErr"); errEl.textContent = "";
    const email = $("rcEmail").value.trim();
    if (!email) { errEl.textContent = I18N.t("valid.enter_email"); return; }
    try {
      const r = await fetch(`${API}/account/recover-send-code`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, purpose })
      });
      const j = await r.json().catch(() => ({}));
      if (r.ok) {
        toast(j.message || I18N.t("toast.code_sent"), "ok");
        showRecoverStep2(purpose, email);
      } else {
        errEl.textContent = j.error || I18N.t("toast.send_failed");
      }
    } catch (e) { errEl.textContent = I18N.t("toast.send_failed2") + e; }
  };
  setTimeout(() => { const el = $("rcEmail"); if (el) el.focus(); }, 60);
}

function showRecoverStep2(purpose, email) {
  const body = $("recoverBody");
  body.innerHTML = `
    <div class="mfa-desc" style="margin-bottom:14px">${I18N.t("recover.enter_code_desc")}</div>
    <div class="field" style="margin-bottom:8px"><label style="font-size:11px;color:var(--muted2)">${I18N.t("form.email")}：${esc(email)}</label></div>
    <div class="field"><label>${I18N.t("form.email_code")}</label><input type="text" id="rcCode" inputmode="numeric" maxlength="6" placeholder="${I18N.t('mfa.code_6')}" autocomplete="one-time-code"></div>
    <div class="login-err" id="rcErr"></div>
    <div class="mfa-foot" style="justify-content:space-between">
      <button class="btn" id="rcResend" type="button">${I18N.t("recover.resend_code")}</button>
      <button class="btn primary" id="rcAction" type="button">${I18N.t("recover.verify_code_btn")}</button>
    </div>`;

  $("rcResend").onclick = () => showRecoverFlow(purpose);
  $("rcAction").onclick = async () => {
    const errEl = $("rcErr"); errEl.textContent = "";
    const code = $("rcCode").value.trim();
    if (code.length !== 6) { errEl.textContent = I18N.t("valid.enter_code"); return; }
    try {
      const r = await fetch(`${API}/account/recover-verify`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, code, purpose })
      });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) { errEl.textContent = j.error || I18N.t("toast.verify_failed"); return; }
      if (j.mfa_required) {
        showRecoverStepMFA(purpose, email, code);
      } else {
        showRecoverResult(purpose, j);
      }
    } catch (e) { errEl.textContent = I18N.t("toast.send_failed2") + e; }
  };
  setTimeout(() => { const el = $("rcCode"); if (el) el.focus(); }, 60);
}

function showRecoverStepMFA(purpose, email, code) {
  const body = $("recoverBody");
  body.innerHTML = `
    <div class="mfa-desc" style="margin-bottom:14px">${I18N.t("recover.enter_totp_desc")}</div>
    <div class="field"><label>${I18N.t("recover.totp_code")}</label><input type="text" id="rcTOTP" inputmode="numeric" maxlength="6" placeholder="${I18N.t('recover.totp_placeholder')}" autocomplete="one-time-code"></div>
    <div class="login-err" id="rcErr"></div>
    <div class="mfa-foot" style="justify-content:space-between">
      <button class="btn" id="rcBack" type="button">${I18N.t("ui.back")}</button>
      <button class="btn primary" id="rcAction" type="button">${I18N.t("recover.verify_totp_btn")}</button>
    </div>`;

  $("rcBack").onclick = () => showRecoverStep2(purpose, email);
  $("rcAction").onclick = async () => {
    const errEl = $("rcErr"); errEl.textContent = "";
    const totp = $("rcTOTP").value.trim();
    if (totp.length !== 6) { errEl.textContent = I18N.t("valid.enter_totp"); return; }
    try {
      const r = await fetch(`${API}/account/recover-verify-mfa`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, code, totp_code: totp, purpose })
      });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) { errEl.textContent = j.error || I18N.t("toast.verify_failed"); return; }
      showRecoverResult(purpose, j);
    } catch (e) { errEl.textContent = I18N.t("toast.send_failed2") + e; }
  };
  setTimeout(() => { const el = $("rcTOTP"); if (el) el.focus(); }, 60);
}

function showRecoverResult(purpose, result) {
  const body = $("recoverBody");
  if (purpose === 'recover_username') {
    toast(I18N.t("toast.username_recovered"), "ok");
    body.innerHTML = `
      <div class="mfa-desc" style="margin-bottom:14px">${I18N.t("recover.username_recovered")}</div>
      <div class="field"><input type="text" value="${esc(result.username)}" readonly style="font-weight:700;font-size:16px;text-align:center;cursor:pointer" data-act="copy-input" title="${I18N.t('toast.copied')}"></div>
      <div class="mfa-foot"><button class="btn primary" id="rcClose" type="button">${I18N.t("recover.back_to_login")}</button></div>`;
    $("rcClose").onclick = () => $("recoverMask").classList.remove("show");
  } else {
    showSetNewPassword(result.reset_token);
  }
}

function showSetNewPassword(token) {
  const body = $("recoverBody");
  body.innerHTML = `
    <div class="mfa-desc" style="margin-bottom:14px">${I18N.t("recover.enter_new_password")}</div>
    <div class="field"><label>${I18N.t("form.new_password_min4")}</label><input type="password" id="rcNewPass" placeholder="${I18N.t('form.new_password')}"></div>
    <div class="field"><label>${I18N.t('profile.confirm_password') || I18N.t('form.new_password')}</label><input type="password" id="rcNewPass2" placeholder="${I18N.t('form.new_password')}"></div>
    <div class="login-err" id="rcErr"></div>
    <div class="mfa-foot"><button class="btn danger" id="rcReset" type="button">${I18N.t("recover.reset_password_btn")}</button></div>`;

  $("rcReset").onclick = async () => {
    const errEl = $("rcErr"); errEl.textContent = "";
    const p1 = $("rcNewPass").value;
    const p2 = $("rcNewPass2").value;
    if (!pwPolicyOK(p1)) { errEl.textContent = I18N.t("auth.password_policy"); return; }
    if (p1 !== p2) { errEl.textContent = I18N.t("auth.password_mismatch"); return; }
    try {
      const r = await fetch(`${API}/account/reset-password`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reset_token: token, new_password: p1 })
      });
      const j = await r.json().catch(() => ({}));
      if (r.ok) {
        body.innerHTML = `
          <div class="mfa-desc" style="margin-bottom:14px;color:var(--ok);font-weight:600">✓ ${j.message || I18N.t("toast.password_reset2")}</div>
          <div class="mfa-foot"><button class="btn primary" id="rcClose" type="button">${I18N.t("recover.back_to_login")}</button></div>`;
        $("rcClose").onclick = () => $("recoverMask").classList.remove("show");
        toast(j.message || I18N.t("toast.password_reset2"), "ok");
      } else {
        errEl.textContent = j.error || I18N.t("toast.reset_failed");
      }
    } catch (e) { errEl.textContent = I18N.t("toast.reset_failed2") + e; }
  };
  setTimeout(() => { const el = $("rcNewPass"); if (el) el.focus(); }, 60);
}

async function logout() {
  try { await fetch(`${API}/logout`, { method: "POST" }); } catch (e) {}
  location.reload();
}

