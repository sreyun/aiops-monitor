// hardware.js — 硬件健康面板（Redfish/BMC）
// Loaded as part of the unified app.js bundle.
//
// 交互：卡片 / 列表自由切换 → 点任意一项打开详情弹窗（全量数据 + 重点突出 + 历史曲线）。
// 注意：CSP 为 script-src 'self'（无 unsafe-inline），**禁止内联 onclick**——一律事件委托。
//
// 覆盖机型：Dell R730/R740/R750/R760 (iDRAC8/9)、华为 RH2288 V3、TaiShan 200 (Model 2280)(iBMC)。
// 各家 Redfish 字段完整度不一，渲染一律"有才画、没有不画空表"。

let HW_RESULTS = [];                                   // [{host, snap}]
let HW_VIEW_MODE = localStorage.getItem("aiops_hw_view") || "card"; // card | list
let HW_CHARTS = {};                                    // 详情弹窗内的图表实例
let HW_CUR = null;                                     // 当前打开详情的项
let HW_HIST_RANGE = "6h";
let HW_HIST_SUMMARY = {};                              // 各指标历史概况（min/max/avg/最新），供导出补上"历史"数据
let HW_LOCAL_EVENTS = [];                              // 平台侧记录的状态变化（异步载入，导出时一并带上）
let HW_FILTER = { q: "", status: "all", fresh: "all" }; // 搜索 / 在线状态 / 数据新鲜度

/* ---------- i18n 小工具 ---------- */

const hwT = (k, fb) => I18N.t(k, fb);

// BMC 一律返回英文枚举（OK / Enabled / NotRedundant…）。直接渲染就会出现
// "中文界面里夹着英文" —— 这里统一过一层字典，查不到就原样回显（新固件可能
// 冒出字典里没有的值，回显总比显示 key 好）。
function hwEnum(ns, v) {
  if (v === undefined || v === null || v === "") return "";
  return I18N.t("hw." + ns + "." + v, v);
}

const hwHealthText = h => hwEnum("health", h || "Unknown");

/* ---------- 数据加载 ---------- */

// 自己拉主机列表，不依赖 window._cachedHosts —— 后者由异步 refresh 填充，
// 首屏点进来时常为 undefined 且此后无人重渲染，导致永远停在"暂无主机"。
async function loadHardwarePanel() {
  const container = $("hardwarePanel");
  if (!container) return;
  container.innerHTML = `<div class="empty-line">${esc(hwT("hardware.loading", "加载中…"))}</div>`;
  let hosts = [];
  try {
    hosts = (window._cachedHosts && window._cachedHosts.length) ? window._cachedHosts
          : (await fetch(`${API}/hosts`).then(r => r.json()) || []);
  } catch (e) { hosts = []; }
  if (!hosts.length) {
    container.innerHTML = `<div class="empty-line">${esc(hwT("hardware.no_hosts", "暂无主机"))}</div>`;
    return;
  }
  // 重复主机（Agent 重装会换新 host_id，同一台机器留下多条记录）单独提示，
  // 不静默过滤：删记录是不可逆的，得由用户确认。
  loadDuplicates(() => renderHardwarePanel());

  // 不过滤离线主机：BMC 是带外通道，主机宕机时的硬件数据恰恰最有价值。
  const results = [];
  await Promise.all(hosts.map(h =>
    fetch(`${API}/hardware/health?host=${encodeURIComponent(h.id)}`)
      .then(r => r.json())
      .then(d => { (d.snapshots || []).forEach(s => results.push({ host: h, snap: s, online: !!h.online })); })
      .catch(() => {})
  ));
  HW_RESULTS = results;
  renderHardwarePanel();
}

/* ---------- 健康判定 ---------- */

function hwHealthMeta(health) {
  if (health === "OK") return { cls: "ok", icon: "✓", label: hwHealthText(health) };
  if (health === "Warning") return { cls: "warn", icon: "⚠", label: hwHealthText(health) };
  if (health === "Critical") return { cls: "crit", icon: "✕", label: hwHealthText(health) };
  return { cls: "unknown", icon: "?", label: hwHealthText(health) };
}

const hwIsBad = h => h === "Warning" || h === "Critical";
const hwBadCls = h => h === "Critical" ? "hw-crit-text" : h === "Warning" ? "hw-warn-text" : "";
const hwTempOver = t => (t.upper_critical > 0 && t.reading >= t.upper_critical) ? "Critical"
                      : (t.upper_caution > 0 && t.reading >= t.upper_caution) ? "Warning" : "";

// 收集所有异常部件，并明确"是哪个部件、什么读数、什么状态"。
// 卡片上的异常计数与详情里的"需要关注"表用的是同一份数据，两处不会再对不上。
function hwBadParts(sd) {
  const out = [];
  const push = (kind, name, reading, status) => out.push({ kind, name, reading, status });

  (sd.temps || []).forEach(t => {
    const over = hwTempOver(t), st = over || (hwIsBad(t.status) ? t.status : "");
    if (st) push(hwT("hardware.temperature", "温度传感器"), t.name, `${t.reading}°C`, st);
  });
  (sd.fans || []).forEach(f => {
    const st = hwIsBad(f.health) ? f.health : (hwIsBad(f.status) ? f.status : "");
    if (st) push(hwT("hardware.fans", "风扇"), f.name, `${f.rpm} RPM`, st);
  });
  ((sd.power || {}).psus || []).forEach(p => {
    if (hwIsBad(p.health)) push(hwT("hardware.power_supply", "电源"), p.name, `${p.input_watts}W`, p.health);
  });
  (sd.storage || []).forEach(d => {
    if (hwIsBad(d.health) || d.smart_warn) {
      const nm = d.location ? `${d.name} (${d.location})` : d.name;
      push(hwT("hardware.storage", "存储"), nm,
        d.smart_warn ? hwT("hardware.smart_fail", "⚠ 预测故障") : `${(d.capacity_gb || 0).toFixed(0)}GB`,
        d.smart_warn ? "Critical" : d.health);
    }
  });
  ((sd.memory || {}).dimms || []).forEach(d => {
    if (hwIsBad(d.health)) push(hwT("hardware.memory", "内存"), d.slot || d.name, `${(d.capacity_gb || 0).toFixed(0)}GB`, d.health);
  });
  (sd.cpus || []).forEach(c => { if (hwIsBad(c.health)) push(hwT("hardware.cpu", "CPU"), c.name, c.model || "", c.health); });
  (sd.gpus || []).forEach(g => { if (hwIsBad(g.health)) push(hwT("hardware.gpu", "GPU / 加速卡"), g.name, g.model || "", g.health); });
  (sd.raid || []).forEach(r => { if (hwIsBad(r.health)) push(hwT("hardware.raid", "RAID / 存储控制器"), r.name, r.model || "", r.health); });
  (sd.enclosures || []).forEach(e => {
    if (hwIsBad(e.health)) push(hwT("hardware.enclosure", "磁盘框"), e.location || e.name,
      e.temperature_c ? e.temperature_c.toFixed(0) + "°C" : (e.model || ""), e.health);
  });
  (sd.raid || []).forEach(r => (r.volumes || []).forEach(v => {
    if (hwIsBad(v.health)) push(hwT("hardware.volumes", "逻辑卷"), `${r.name} / ${v.name}`, v.raid_type || "", v.health);
  }));
  return out;
}

// 汇总一台设备的"重点"：最高温、功耗、异常部件
function hwSummary(sd) {
  const temps = sd.temps || [], fans = sd.fans || [], power = sd.power || {};
  const maxTemp = temps.length ? Math.max(...temps.map(t => t.reading || 0)) : 0;
  const bads = hwBadParts(sd);
  // 最高温那颗传感器是否已越限——决定卡片上温度 chip 要不要标色
  let tempLvl = "";
  temps.forEach(t => {
    if ((t.reading || 0) !== maxTemp) return;
    const o = hwTempOver(t) || (hwIsBad(t.status) ? t.status : "");
    if (o === "Critical" || (o && tempLvl !== "Critical")) tempLvl = o;
  });
  return { maxTemp, tempLvl, watts: power.total_watts || 0, bad: bads.length, bads, temps, fans, power };
}

/* ---------- 渲染 ---------- */

// 异常优先排序：Critical > Warning > OK，让最需要关注的排在最前。
// 卡片/列表/详情三处必须用同一个顺序，否则 data-hwidx 会指错设备。
function hwSortedItems() {
  const order = { Critical: 0, Warning: 1, OK: 2 };
  return HW_RESULTS.slice().sort((a, b) =>
    (order[a.snap.health] ?? 3) - (order[b.snap.health] ?? 3));
}

// 快照更新时间（秒）。BMC 挂了时快照会停更，这个时间比主机在线状态更能反映
// "这份硬件数据还新不新"。
function hwUpdatedAt(it) {
  const t = it.snap.updated_at ? Date.parse(it.snap.updated_at) / 1000
          : ((it.snap.snapshot || {}).timestamp || 0);
  return t || 0;
}

// 搜索匹配范围覆盖运维实际会输入的东西：主机名、BMC 名/地址、厂商型号、序列号。
function hwMatchesQuery(it, q) {
  if (!q) return true;
  const sd = it.snap.snapshot || {}, sys = sd.system || {};
  const hay = [
    it.host.hostname, it.host.id, it.host.ip,
    it.snap.target_name, it.snap.target_url,
    sys.manufacturer, sys.model, sys.serial_number, sys.sku, sys.host_name,
  ].filter(Boolean).join(" ").toLowerCase();
  // 空格分隔的多个词按 AND 匹配，"dell r740" 才能精确缩小范围
  return q.toLowerCase().split(/\s+/).filter(Boolean).every(w => hay.includes(w));
}

function hwFilteredItems() {
  const now = Date.now() / 1000;
  const freshWindow = { "1h": 3600, "24h": 86400, "7d": 604800 }[HW_FILTER.fresh] || 0;
  return hwSortedItems().filter(it => {
    if (HW_FILTER.status === "online" && !it.online) return false;
    if (HW_FILTER.status === "offline" && it.online) return false;
    if (freshWindow) {
      const t = hwUpdatedAt(it);
      if (!t || now - t > freshWindow) return false;
    }
    return hwMatchesQuery(it, HW_FILTER.q);
  });
}

function hwToolbarHTML(shown, total) {
  const opt = (v, cur, label) => `<option value="${v}" ${v === cur ? "selected" : ""}>${esc(label)}</option>`;
  let h = `<div class="hw-toolbar">
    <input type="search" id="hwSearch" class="hw-search" value="${esc(HW_FILTER.q)}"
      placeholder="${esc(hwT("hardware.search_ph", "搜索主机名 / 型号 / 序列号 / BMC 地址"))}">
    <select id="hwStatusFilter" class="hw-sel">
      ${opt("all", HW_FILTER.status, hwT("hardware.status_all", "全部状态"))}
      ${opt("online", HW_FILTER.status, hwT("hardware.status_online", "仅在线"))}
      ${opt("offline", HW_FILTER.status, hwT("hardware.status_offline", "仅离线"))}
    </select>
    <select id="hwFreshFilter" class="hw-sel">
      ${opt("all", HW_FILTER.fresh, hwT("hardware.fresh_all", "不限更新时间"))}
      ${opt("1h", HW_FILTER.fresh, hwT("hardware.fresh_1h", "1 小时内更新"))}
      ${opt("24h", HW_FILTER.fresh, hwT("hardware.fresh_24h", "24 小时内更新"))}
      ${opt("7d", HW_FILTER.fresh, hwT("hardware.fresh_7d", "7 天内更新"))}
    </select>
    <span class="hw-count">${shown}/${total}</span>
  </div>`;

  // 重复主机提示（仅在确有可清理项时出现，避免常态化噪音横幅）
  h += dupBannerHTML();
  return h;
}

function renderHardwarePanel() {
  const container = $("hardwarePanel");
  if (!container) return;
  if (!HW_RESULTS.length) {
    container.innerHTML = hwToolbarHTML(0, 0) +
      `<div class="empty-line">${esc(hwT("hardware.no_data", "暂无硬件数据（需在 Agent 配置 Redfish 目标）"))}</div>`;
    return;
  }
  const items = hwFilteredItems();
  let body;
  if (!items.length) {
    body = `<div class="empty-line">${esc(hwT("hardware.no_match", "没有匹配的设备，试试放宽筛选条件"))}</div>`;
  } else {
    body = HW_VIEW_MODE === "list" ? hwListHTML(items) : hwCardHTML(items);
  }
  container.innerHTML = hwToolbarHTML(items.length, HW_RESULTS.length) + body;
}

// 机型一行：厂商 · 型号 · 序列号。Dell 的 Service Tag 落在 SKU，华为落在
// SerialNumber —— 采集端已归一，这里只管展示。
function hwModelLine(sysInfo) {
  const parts = [sysInfo.manufacturer, sysInfo.model].filter(Boolean);
  const sn = sysInfo.serial_number || sysInfo.sku;
  if (sn) parts.push("SN " + sn);
  return parts.join(" · ");
}

function hwCardHTML(items) {
  return `<div class="hw-grid">` + items.map((it, i) => {
    const snap = it.snap, sd = snap.snapshot || {}, m = hwHealthMeta(snap.health), s = hwSummary(sd);
    const sys = sd.system || {};
    const stat = (label, v, cls) => v ? `<span class="hw-stat ${cls || ""}"><span class="hw-stat-k">${esc(label)}</span>${esc(v)}</span>` : "";
    const cpus = sd.cpus || [], mem = sd.memory || {};
    const cpuTxt = cpus.length ? `${cpus.length} × ${cpus[0].cores || "?"}C` : "";
    const edge = m.cls === "crit" ? "hw-edge-crit" : m.cls === "warn" ? "hw-edge-warn" : "";
    const model = hwModelLine(sys);
    return `<div class="hw-card ${edge}" data-hwidx="${i}" role="button" tabindex="0">
      <div class="hw-card-header">
        <span class="hw-health-dot hw-${m.cls}" aria-hidden="true">${m.icon}</span>
        <div class="hw-card-info">
          <div class="hw-card-name">${esc(snap.target_name || snap.target_url)}</div>
          <div class="hw-card-sub">
            <span class="hw-dot ${it.online ? "on" : "off"}" title="${esc(it.online ? hwT("hardware.host_online", "主机在线") : hwT("hardware.host_offline", "主机离线"))}"></span>
            ${esc(it.host.hostname || it.host.id)} · ${esc(m.label)}
          </div>
          ${model ? `<div class="hw-card-model" title="${esc(model)}">${esc(model)}</div>` : ""}
        </div>
        ${s.bad > 0 ? `<span class="badge crit">${s.bad} ${esc(hwT("hardware.bad_count", "项异常"))}</span>` : ""}
        <button class="icon-btn danger hw-del-btn" data-hwdel="${esc(snap.target_name || "")}" data-hwhost="${esc(it.host.id)}" title="${esc(hwT("hardware.delete", "删除"))}" data-i18n-title="hardware.delete">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2m3 0v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6h14z"/></svg>
        </button>
      </div>
      <div class="hw-quick-stats">
        ${stat(hwT("hardware.cpu", "CPU"), cpuTxt)}
        ${stat(hwT("hardware.memory", "内存"), mem.total_gb ? mem.total_gb.toFixed(0) + "GB" : "")}
        ${stat(hwT("hardware.max_temp", "最高温度"), s.maxTemp ? s.maxTemp.toFixed(0) + "°C" : "",
               s.tempLvl === "Critical" ? "hw-stat-crit" : s.tempLvl === "Warning" ? "hw-stat-warn" : "")}
        ${stat(hwT("hardware.power", "功耗"), s.watts ? s.watts.toFixed(0) + "W" : "")}
        ${stat(hwT("hardware.storage", "存储"), (sd.storage || []).length ? String(sd.storage.length) : "")}
        ${stat(hwT("hardware.fans", "风扇"), s.fans.length ? String(s.fans.length) : "")}
        ${stat(hwT("hardware.gpu", "GPU / 加速卡"), (sd.gpus || []).length ? String(sd.gpus.length) : "")}
        ${stat(hwT("hardware.raid", "RAID / 存储控制器"), (sd.raid || []).length ? String(sd.raid.length) : "")}
        ${stat(hwT("hardware.enclosure", "磁盘框"), (sd.enclosures || []).length ? String(sd.enclosures.length) : "")}
      </div>
      <div class="hw-expand-hint">${esc(hwT("hardware.open_detail", "点击查看详情与历史曲线 →"))}</div>
    </div>`;
  }).join("") + `</div>`;
}

function hwListHTML(items) {
  return `<div class="hw-list">` + items.map((it, i) => {
    const snap = it.snap, sd = snap.snapshot || {}, m = hwHealthMeta(snap.health), s = hwSummary(sd);
    const sys = sd.system || {};
    const badgeCls = m.cls === "ok" ? "ok" : m.cls === "warn" ? "warn" : m.cls === "crit" ? "crit" : "";
    const sub = [it.host.hostname || it.host.id, sys.model].filter(Boolean).join(" · ");
    return `<div class="hw-row" data-hwidx="${i}" role="button" tabindex="0">
      <span class="hw-health-dot hw-${m.cls}" aria-hidden="true">${m.icon}</span>
      <div class="hw-row-id">
        <div class="hw-row-name">${esc(snap.target_name || snap.target_url)}</div>
        <div class="hw-row-sub" title="${esc(sub)}">
          <span class="hw-dot ${it.online ? "on" : "off"}"></span>${esc(sub)}
        </div>
      </div>
      <span class="badge ${badgeCls}">${esc(m.label)}</span>
      ${s.bad > 0 ? `<span class="badge crit">${s.bad} ${esc(hwT("hardware.bad_count", "项异常"))}</span>` : `<span class="hw-row-cell">—</span>`}
      <span class="hw-row-cell mono">${s.maxTemp ? s.maxTemp.toFixed(0) + "°C" : "-"}</span>
      <span class="hw-row-cell mono">${s.watts ? s.watts.toFixed(0) + "W" : "-"}</span>
      <span class="hw-row-cell mono">${(sd.cpus || []).length}C / ${((sd.memory || {}).total_gb || 0).toFixed(0)}GB</span>
      <span class="hw-row-cell">${(sd.storage || []).length} ${esc(hwT("hardware.disk_unit", "盘"))} · ${s.fans.length} ${esc(hwT("hardware.fans", "风扇"))}</span>
      <button class="icon-btn danger hw-del-btn" data-hwdel="${esc(snap.target_name || "")}" data-hwhost="${esc(it.host.id)}" title="${esc(hwT("hardware.delete", "删除"))}" data-i18n-title="hardware.delete">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2m3 0v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6h14z"/></svg>
      </button>
    </div>`;
  }).join("") + `</div>`;
}

/* ---------- 详情弹窗 ---------- */

function openHwDetail(idx) {
  // 必须和渲染时用的**同一个**列表：卡片上的 data-hwidx 是筛选后列表的下标，
  // 这里若用未筛选的列表会打开另一台设备的详情。
  const it = hwFilteredItems()[idx];
  if (!it) return;
  HW_CUR = it;
  const snap = it.snap, sd = snap.snapshot || {}, m = hwHealthMeta(snap.health);
  const sys = sd.system || {};
  const title = [snap.target_name || snap.target_url, it.host.hostname || it.host.id, sys.model]
    .filter(Boolean).join(" · ");
  $("hwDetailTitle").textContent = title;
  $("hwDetailBody").innerHTML = hwDetailHTML(it, sd, m);
  $("hwDetailMask").classList.add("show");
  loadHwHistory();     // 异步填充历史曲线
  loadHwLocalEvents(); // 异步填充平台侧记录的状态变化
}

// 区块骨架：没有数据就明说"该机型未上报"，而不是渲染一张空表头。
// 各家 BMC 暴露的部件差异很大（TaiShan 200 无 RAID 卡时就是没有），
// 空表头会让人误以为是采集坏了。
function hwSection(title, count, bodyHTML) {
  return `<div class="hw-sec">
    <div class="hw-sec-head"><h4>${esc(title)}</h4>${count !== null ? `<span class="hw-sec-count">${count}</span>` : ""}</div>
    ${bodyHTML || `<div class="hw-sec-empty">${esc(hwT("hardware.no_parts", "该机型未上报此类部件"))}</div>`}
  </div>`;
}

function hwTable(head, rows) {
  if (!rows.length) return "";
  return `<div class="hw-table-wrap"><table class="hw-table">
    <thead><tr>${head.map(h => `<th>${esc(h)}</th>`).join("")}</tr></thead>
    <tbody>${rows.join("")}</tbody></table></div>`;
}

const hwSevCls = s => s === "Critical" ? "crit" : s === "Warning" ? "warn" : s === "OK" ? "ok" : "unknown";
const hwSevChip = s => `<span class="hw-sev hw-${hwSevCls(s)}">${esc(hwHealthText(s))}</span>`;
const hwDash = v => (v === undefined || v === null || v === "") ? "-" : String(v);

// 容量格式化：≥1024GB 显示 TB（保留 3 位，如 40.035 TB），否则 GB；0/空返回空串（不占格）。
function hwFmtCap(gb) {
  if (!gb || gb <= 0) return "";
  return gb >= 1024 ? (gb / 1024).toFixed(3) + " TB" : gb.toFixed(0) + " GB";
}

function hwDetailHTML(it, sd, m) {
  const s = hwSummary(sd);
  const sys = sd.system || {};
  let html = "";

  // ── 采集错误置顶：BMC 都连不上时，下面所有"正常"都是上一份缓存，必须先说清楚 ──
  if (sd.error) {
    html += `<div class="hw-error-row">${esc(hwT("hardware.collect_error", "采集错误"))}：${esc(sd.error)}</div>`;
  }

  // ── 整机身份 ──
  const ident = [
    [hwT("hardware.vendor", "厂商"), sys.manufacturer],
    [hwT("hardware.model", "型号"), sys.model],
    [hwT("hardware.serial", "序列号"), sys.serial_number],
    // Dell 的 Service Tag 与序列号常常同值，同值就不重复占一格
    [hwT("hardware.service_tag", "服务标签"), (sys.sku && sys.sku !== sys.serial_number) ? sys.sku : ""],
    [hwT("hardware.asset_tag", "资产编号"), sys.asset_tag],
    // 存储阵列（OceanStor 等）专有字段：软件版本/补丁/总容量/已用/位置（服务器为空不显示）
    [hwT("hardware.sw_version", "软件版本"), sys.software_version],
    [hwT("hardware.patch_version", "补丁版本"), sys.patch_version],
    [hwT("hardware.total_capacity", "总容量"), hwFmtCap(sys.total_capacity_gb)],
    [hwT("hardware.used_capacity", "已用容量"), hwFmtCap(sys.used_capacity_gb)],
    [hwT("hardware.location", "设备位置"), sys.location],
    [hwT("hardware.os_hostname", "OS 主机名"), sys.host_name],
    [hwT("hardware.bios", "BIOS 版本"), sys.bios_version],
    [hwT("hardware.bmc", "BMC"), [sys.bmc_model, sys.bmc_firmware].filter(Boolean).join(" ")],
    [hwT("hardware.power_state", "电源状态"), hwEnum("power", sys.power_state)],
    [hwT("hardware.run_state", "运行状态"), hwEnum("state", sd.state)],
    [hwT("hardware.bmc_addr", "BMC 地址"), it.snap.target_url],
  ].filter(([, v]) => v);
  if (ident.length) {
    html += `<div class="hw-ident">` + ident.map(([k, v]) =>
      `<div class="hw-ident-item"><div class="hw-ident-k">${esc(k)}</div><div class="hw-ident-v">${esc(v)}</div></div>`
    ).join("") + `</div>`;
  }

  // ── 重点摘要条 ──
  html += `<div class="hw-kpis">
    <div class="hw-kpi hw-${m.cls}"><div class="hw-kpi-v">${m.icon} ${esc(m.label)}</div><div class="hw-kpi-k">${esc(hwT("hardware.overall_health", "整机健康"))}</div></div>
    <div class="hw-kpi ${s.bad ? "hw-crit" : "hw-ok"}"><div class="hw-kpi-v">${s.bad}</div><div class="hw-kpi-k">${esc(hwT("hardware.bad_parts", "异常部件"))}</div></div>
    <div class="hw-kpi ${s.tempLvl === "Critical" ? "hw-crit" : s.tempLvl === "Warning" ? "hw-warn" : ""}"><div class="hw-kpi-v">${s.maxTemp ? s.maxTemp.toFixed(0) + "°C" : "-"}</div><div class="hw-kpi-k">${esc(hwT("hardware.max_temp", "最高温度"))}</div></div>
    <div class="hw-kpi"><div class="hw-kpi-v">${s.watts ? s.watts.toFixed(0) + "W" : "-"}</div><div class="hw-kpi-k">${esc(hwT("hardware.total_power", "总功耗"))}</div></div>
    <div class="hw-kpi"><div class="hw-kpi-v">${esc(hwEnum("redundancy", (sd.power || {}).redundancy) || "-")}</div><div class="hw-kpi-k">${esc(hwT("hardware.power_redundancy", "电源冗余"))}</div></div>
  </div>`;

  // ── 异常项置顶（重点突出，明确到具体部件）──
  if (s.bads.length) {
    // 计数用半角括号：全角括号在英文界面里是突兀的中文标点
    html += `<div class="hw-bad-box"><h4>⚠ ${esc(hwT("hardware.needs_attention", "需要关注"))} (${s.bads.length})</h4>` +
      hwTable([hwT("hardware.part", "部件"), hwT("hardware.name", "名称"), hwT("hardware.reading", "读数"), hwT("hardware.status", "状态")],
        s.bads.map(b => `<tr class="${hwBadCls(b.status)}"><td>${esc(b.kind)}</td><td>${esc(b.name)}</td><td>${esc(b.reading)}</td><td>${hwSevChip(b.status)}</td></tr>`)) +
      `</div>`;
  } else if (!sd.error) {
    html += `<div class="hw-ok-box">✓ ${esc(hwT("hardware.all_normal", "全部部件正常"))}</div>`;
  }

  // ── BMC 事件日志：唯一能回答"是哪个部件触发的"的数据 ──
  const evs = sd.events || [];
  html += hwSection(hwT("hardware.events_bmc", "BMC 事件日志（SEL）"), evs.length ? evs.length : null,
    evs.length ? `<div class="hw-events-wrap">` + hwTable(
      [hwT("hardware.event_time", "时间"), hwT("hardware.event_severity", "级别"),
       hwT("hardware.event_component", "触发部件"), hwT("hardware.event_message", "事件内容")],
      evs.map(e => `<tr class="${hwBadCls(e.severity)}">
        <td class="mono">${esc(hwFmtTime(e.created))}</td>
        <td>${hwSevChip(e.severity)}</td>
        <td>${e.component ? `<span class="hw-comp">${esc(e.component)}</span>` : "-"}</td>
        <td>${esc(e.message)}${e.resolved ? ` <span class="hw-sev hw-ok">${esc(hwT("hardware.event_resolved", "已处理"))}</span>` : ""}</td>
      </tr>`)) + `</div><div class="hint">${esc(hwT("hardware.events_hint", "来自 BMC 自身的硬件事件记录，可定位到具体触发部件"))}</div>`
    : "");

  // ── 历史曲线 ──
  const wrap = id => `<div class="chart-wrap"><canvas id="${id}" width="1000" height="200"></canvas>
    <button class="chart-enlarge" data-hwchart="${id}" title="${esc(hwT("ui.zoom_preview", "放大预览"))}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg></button></div>`;
  html += `<div class="hw-sec"><div class="hw-sec-head"><h4>${esc(hwT("hardware.history", "历史曲线"))}</h4></div>
    <div class="chart-controls">${["1h", "6h", "24h", "7d"].map(r =>
      `<button class="chip-btn ${HW_HIST_RANGE === r ? "active" : ""}" data-hwrange="${r}">${r}</button>`).join("")}</div>
    <div class="chart-container">${wrap("hwChartTemp")}${wrap("hwChartFan")}${wrap("hwChartPower")}${wrap("hwChartHealth")}</div></div>`;

  html += hwDetailPartsHTML(sd);

  // ── 平台侧记录的状态变化（异步填充）──
  html += `<div class="hw-sec"><div class="hw-sec-head"><h4>${esc(hwT("hardware.events_local", "监控记录的状态变化"))}</h4></div>
    <div id="hwLocalEvents"><div class="hw-sec-empty">${esc(hwT("hardware.loading", "加载中…"))}</div></div></div>`;

  // ── 元信息 ──
  const upd = it.snap.updated_at ? new Date(it.snap.updated_at).toLocaleString()
            : (sd.timestamp ? new Date(sd.timestamp * 1000).toLocaleString() : "-");
  html += `<div class="hint" style="margin-top:10px">${esc(hwT("hardware.updated", "更新时间"))} ${esc(upd)}</div>`;
  return html;
}

// 全量部件明细。每个区块独立成段，缺数据就说明原因，绝不画空表头。
function hwDetailPartsHTML(sd) {
  let html = "";
  const mem = sd.memory || {};
  const psus = (sd.power || {}).psus || [];

  // CPU
  html += hwSection(hwT("hardware.cpu", "CPU"), (sd.cpus || []).length,
    hwTable([hwT("hardware.name", "名称"), hwT("hardware.model", "型号"), hwT("hardware.cores_threads", "核心/线程"),
             hwT("hardware.max_freq", "最大频率"), hwT("hardware.health", "健康")],
      (sd.cpus || []).map(c => `<tr class="${hwBadCls(c.health)}"><td>${esc(c.name)}</td><td>${esc(hwDash(c.model))}</td>
        <td>${c.cores || "?"}C / ${c.threads || "?"}T</td><td>${c.max_freq_mhz ? c.max_freq_mhz + "MHz" : "-"}</td>
        <td>${hwSevChip(c.health)}</td></tr>`)));

  // GPU / 加速卡（此前采集了但从未渲染）
  html += hwSection(hwT("hardware.gpu", "GPU / 加速卡"), (sd.gpus || []).length || null,
    hwTable([hwT("hardware.name", "名称"), hwT("hardware.model", "型号"), hwT("hardware.manufacturer", "制造商"),
             hwT("hardware.max_freq", "最大频率"), hwT("hardware.status", "状态"), hwT("hardware.health", "健康")],
      (sd.gpus || []).map(g => `<tr class="${hwBadCls(g.health)}"><td>${esc(g.name)}</td><td>${esc(hwDash(g.model))}</td>
        <td>${esc(hwDash(g.manufacturer))}</td><td>${g.max_freq_mhz ? g.max_freq_mhz + "MHz" : "-"}</td>
        <td>${esc(hwEnum("state", g.state) || "-")}</td><td>${hwSevChip(g.health)}</td></tr>`)));

  // 内存
  const memTitle = hwT("hardware.memory", "内存") +
    (mem.total_gb ? ` · ${mem.total_gb.toFixed(0)}GB` : "") +
    (mem.used_gb > 0 ? ` / ${hwT("hardware.used", "已用")} ${mem.used_gb.toFixed(0)}GB` : "");
  html += hwSection(memTitle, (mem.dimms || []).length,
    hwTable([hwT("hardware.slot", "插槽"), hwT("hardware.capacity", "容量"), hwT("hardware.type", "类型"),
             hwT("hardware.speed", "速率"), hwT("hardware.manufacturer", "制造商"), hwT("hardware.part_number", "部件号"),
             hwT("hardware.serial", "序列号"), hwT("hardware.health", "健康")],
      (mem.dimms || []).map(d => `<tr class="${hwBadCls(d.health)}"><td class="mono">${esc(hwDash(d.slot || d.name))}</td>
        <td>${(d.capacity_gb || 0).toFixed(0)}GB</td><td>${esc(hwDash(d.type))}</td>
        <td>${d.speed_mhz ? d.speed_mhz + "MHz" : "-"}</td><td>${esc(hwDash(d.manufacturer))}</td>
        <td class="mono">${esc(hwDash(d.part_number))}</td><td class="mono">${esc(hwDash(d.serial_number))}</td>
        <td>${hwSevChip(d.health)}</td></tr>`)));

  // 存储 / 硬盘
  html += hwSection(hwT("hardware.storage", "存储"), (sd.storage || []).length,
    hwTable([hwT("hardware.location", "槽位"), hwT("hardware.name", "名称"), hwT("hardware.model", "型号"),
             hwT("hardware.type", "类型"), hwT("hardware.capacity", "容量"), hwT("hardware.serial", "序列号"),
             hwT("hardware.disk_fw", "盘固件"), hwT("hardware.life_left", "剩余寿命"),
             hwT("hardware.smart", "SMART"), hwT("hardware.health", "健康")],
      (sd.storage || []).map(d => {
        const media = [d.media_type, d.protocol].filter(Boolean).join(" / ") ||
                      (d.rotation_rpm ? d.rotation_rpm + " RPM" : "");
        // life_left_pct: -1 = BMC 未提供该字段（多数 HDD 与老 iDRAC），不能显示成 0%
        const life = (d.life_left_pct >= 0) ? d.life_left_pct.toFixed(0) + "%" : "-";
        const lifeCls = (d.life_left_pct >= 0 && d.life_left_pct <= 10) ? "hw-crit-text"
                      : (d.life_left_pct >= 0 && d.life_left_pct <= 20) ? "hw-warn-text" : "";
        return `<tr class="${d.smart_warn ? "hw-crit-text" : hwBadCls(d.health)}">
          <td class="mono">${esc(hwDash(d.location))}</td><td>${esc(d.name)}</td><td>${esc(hwDash(d.model))}</td>
          <td>${esc(hwDash(media))}</td><td>${(d.capacity_gb || 0).toFixed(0)}GB</td>
          <td class="mono">${esc(hwDash(d.serial_number))}</td><td class="mono">${esc(hwDash(d.revision))}</td>
          <td class="${lifeCls}">${life}</td>
          <td>${d.smart_warn ? `<span class="hw-sev hw-crit">${esc(hwT("hardware.smart_fail", "⚠ 预测故障"))}</span>`
                             : `<span class="hw-sev hw-ok">${esc(hwT("hardware.smart_ok", "正常"))}</span>`}</td>
          <td>${hwSevChip(d.health)}</td></tr>`;
      })));

  // 磁盘框（OceanStor 等外置存储；服务器 BMC 不上报此项，故无数据时整段不渲染）
  html += hwSection(hwT("hardware.enclosure", "磁盘框"), (sd.enclosures || []).length || null,
    hwTable([hwT("hardware.name", "名称"), hwT("hardware.model", "型号"), hwT("hardware.location", "槽位"),
             hwT("hardware.serial", "序列号"), hwT("hardware.temperature", "温度传感器"),
             hwT("hardware.status", "状态"), hwT("hardware.health", "健康")],
      (sd.enclosures || []).map(e => `<tr class="${hwBadCls(e.health)}"><td>${esc(e.name)}</td><td>${esc(hwDash(e.model))}</td>
        <td class="mono">${esc(hwDash(e.location))}</td>
        <td class="mono">${esc(hwDash(e.serial_number))}</td>
        <td class="mono">${e.temperature_c ? e.temperature_c.toFixed(0) + "°C" : "-"}</td>
        <td>${esc(hwEnum("state", e.state) || "-")}</td><td>${hwSevChip(e.health)}</td></tr>`)));

  // RAID / 存储控制器（此前采集了但从未渲染）
  html += hwSection(hwT("hardware.raid", "RAID / 存储控制器"), (sd.raid || []).length || null,
    hwTable([hwT("hardware.name", "名称"), hwT("hardware.model", "型号"), hwT("hardware.firmware", "固件版本"),
             hwT("hardware.cache", "缓存"), hwT("hardware.drive_count", "挂载盘数"), hwT("hardware.speed", "速率"),
             hwT("hardware.health", "健康")],
      (sd.raid || []).map(r => `<tr class="${hwBadCls(r.health)}"><td>${esc(r.name)}</td><td>${esc(hwDash(r.model))}</td>
        <td class="mono">${esc(hwDash(r.firmware_version))}</td>
        <td>${r.cache_mb ? r.cache_mb.toFixed(0) + "MB" : "-"}</td><td>${hwDash(r.drive_count)}</td>
        <td>${r.speed_gbps ? r.speed_gbps + "Gbps" : "-"}</td><td>${hwSevChip(r.health)}</td></tr>`)));

  // 逻辑卷（RAID 组）：盘好不代表卷好——降级的 RAID5 里每块盘都可能是 OK
  const vols = (sd.raid || []).flatMap(r => (r.volumes || []).map(v => ({ ctl: r.name, v })));
  if (vols.length) {
    html += hwSection(hwT("hardware.volumes", "逻辑卷"), vols.length,
      hwTable([hwT("hardware.raid", "RAID / 存储控制器"), hwT("hardware.name", "名称"), hwT("hardware.raid_level", "RAID 级别"),
               hwT("hardware.capacity", "容量"), hwT("hardware.status", "状态"), hwT("hardware.health", "健康")],
        vols.map(({ ctl, v }) => `<tr class="${hwBadCls(v.health)}"><td>${esc(ctl)}</td><td>${esc(v.name)}</td>
          <td>${esc(hwDash(v.raid_type))}</td><td>${v.capacity_gb ? v.capacity_gb.toFixed(0) + "GB" : "-"}</td>
          <td>${esc(hwEnum("state", v.state) || "-")}</td><td>${hwSevChip(v.health)}</td></tr>`)));
  }

  // 电源
  html += hwSection(hwT("hardware.power_supply", "电源"), psus.length,
    hwTable([hwT("hardware.name", "名称"), hwT("hardware.model", "型号"), hwT("hardware.input_watts", "输入(W)"),
             hwT("hardware.output_watts", "输出(W)"), hwT("hardware.rated_watts", "额定功率"),
             hwT("hardware.input_voltage", "输入电压"), hwT("hardware.serial", "序列号"),
             hwT("hardware.status", "状态"), hwT("hardware.health", "健康")],
      psus.map(p => `<tr class="${hwBadCls(p.health)}"><td>${esc(p.name)}</td><td>${esc(hwDash(p.model))}</td>
        <td>${p.input_watts ? p.input_watts.toFixed(0) + "W" : "-"}</td>
        <td>${p.output_watts ? p.output_watts.toFixed(0) + "W" : "-"}</td>
        <td>${p.capacity_watts ? p.capacity_watts.toFixed(0) + "W" : "-"}</td>
        <td>${p.line_input_voltage ? p.line_input_voltage.toFixed(0) + "V" : "-"}</td>
        <td class="mono">${esc(hwDash(p.serial_number))}</td>
        <td>${esc(hwEnum("state", p.state) || "-")}</td><td>${hwSevChip(p.health)}</td></tr>`)));

  // 风扇
  html += hwSection(hwT("hardware.fans", "风扇"), (sd.fans || []).length,
    hwTable([hwT("hardware.name", "名称"), hwT("hardware.rpm", "转速"), hwT("hardware.status", "状态"), hwT("hardware.health", "健康")],
      (sd.fans || []).map(f => `<tr class="${hwBadCls(f.health) || hwBadCls(f.status)}"><td>${esc(f.name)}</td>
        <td class="mono">${f.rpm} RPM</td><td>${esc(hwEnum("state", f.status) || "-")}</td><td>${hwSevChip(f.health)}</td></tr>`)));

  // 温度传感器
  html += hwSection(hwT("hardware.temperature", "温度传感器"), (sd.temps || []).length,
    hwTable([hwT("hardware.sensor", "传感器"), hwT("hardware.reading", "读数"), hwT("hardware.caution_threshold", "告警阈值"),
             hwT("hardware.crit_threshold", "严重阈值"), hwT("hardware.status", "状态")],
      (sd.temps || []).map(t => {
        const over = hwTempOver(t);
        return `<tr class="${hwBadCls(over || t.status)}"><td>${esc(t.name)}</td><td class="mono">${t.reading}°C</td>
          <td class="mono">${t.upper_caution > 0 ? t.upper_caution + "°C" : "-"}</td>
          <td class="mono">${t.upper_critical > 0 ? t.upper_critical + "°C" : "-"}</td>
          <td>${hwSevChip(over || t.status)}</td></tr>`;
      })));

  // 固件
  html += hwSection(hwT("hardware.firmware", "固件版本"), (sd.firmware || []).length || null,
    hwTable([hwT("hardware.name", "名称"), hwT("hardware.version", "版本")],
      (sd.firmware || []).map(f => `<tr><td>${esc(f.name)}</td><td class="mono">${esc(f.version)}</td></tr>`)));

  return html;
}

// BMC 的时间戳格式各家不一（带/不带时区、偶尔是 "-- ::" 占位）。
// 解析不了就原样显示，总好过显示 "Invalid Date"。
function hwFmtTime(v) {
  if (!v) return "-";
  const d = new Date(v);
  return isNaN(d.getTime()) ? String(v) : d.toLocaleString();
}

/* ---------- 平台侧事件 ---------- */

async function loadHwLocalEvents() {
  const box = $("hwLocalEvents");
  HW_LOCAL_EVENTS = [];
  if (!box || !HW_CUR) return;
  try {
    const qs = new URLSearchParams({ host: HW_CUR.host.id, limit: "50" });
    const target = HW_CUR.snap.target_name || "";
    if (target) qs.set("target", target);
    const d = await fetch(`${API}/hardware/events?${qs}`).then(r => r.json());
    const evs = d.events || [];
    HW_LOCAL_EVENTS = evs;
    if (!evs.length) {
      box.innerHTML = `<div class="hw-sec-empty">${esc(hwT("hardware.no_events", "暂无事件记录"))}</div>`;
      return;
    }
    box.innerHTML = `<div class="hw-events-wrap">` + hwTable(
      [hwT("hardware.event_time", "时间"), hwT("hardware.event_severity", "级别"), hwT("hardware.event_message", "事件内容")],
      evs.map(e => {
        // 平台侧 severity 落库时是小写（critical/warning），与 Redfish 的
        // 首字母大写枚举不同，转一下才能命中同一套色板与字典。
        const sev = e.severity ? e.severity.charAt(0).toUpperCase() + e.severity.slice(1) : "";
        return `<tr class="${hwBadCls(sev)}"><td class="mono">${esc(hwFmtTime(e.created_at))}</td>
          <td>${sev ? hwSevChip(sev) : "-"}</td><td>${esc(e.message || e.event_type || "")}</td></tr>`;
      })) + `</div>`;
  } catch (e) {
    box.innerHTML = `<div class="hw-sec-empty">${esc(hwT("hardware.no_events", "暂无事件记录"))}</div>`;
  }
}

/* ---------- 导出（资产管理） ---------- */

// 把当前设备拍平成 export.js 的中性文档模型。刻意与弹窗里展示的内容一致——
// 用户看到什么就导出什么，不搞"界面一套、导出另一套"。
function hwExportModel(it) {
  const snap = it.snap, sd = snap.snapshot || {}, sys = sd.system || {};
  const m = hwHealthMeta(snap.health), s = hwSummary(sd);
  const mem = sd.memory || {}, psus = (sd.power || {}).psus || [];
  const D = hwDash;

  const model = {
    title: [snap.target_name || snap.target_url, it.host.hostname || it.host.id, sys.model].filter(Boolean).join(" · "),
    subtitle: `${hwT("hardware.updated", "更新时间")}: ${hwFmtTime(snap.updated_at) }`,
    meta: [],
    sections: [],
  };

  const meta = [
    [hwT("hardware.vendor", "厂商"), sys.manufacturer],
    [hwT("hardware.model", "型号"), sys.model],
    [hwT("hardware.serial", "序列号"), sys.serial_number],
    [hwT("hardware.service_tag", "服务标签"), (sys.sku && sys.sku !== sys.serial_number) ? sys.sku : ""],
    [hwT("hardware.asset_tag", "资产编号"), sys.asset_tag],
    [hwT("hardware.sw_version", "软件版本"), sys.software_version],
    [hwT("hardware.patch_version", "补丁版本"), sys.patch_version],
    [hwT("hardware.total_capacity", "总容量"), hwFmtCap(sys.total_capacity_gb)],
    [hwT("hardware.used_capacity", "已用容量"), hwFmtCap(sys.used_capacity_gb)],
    [hwT("hardware.location", "设备位置"), sys.location],
    [hwT("hardware.os_hostname", "OS 主机名"), sys.host_name],
    [hwT("hardware.bios", "BIOS 版本"), sys.bios_version],
    [hwT("hardware.bmc", "BMC"), [sys.bmc_model, sys.bmc_firmware].filter(Boolean).join(" ")],
    [hwT("hardware.power_state", "电源状态"), hwEnum("power", sys.power_state)],
    [hwT("hardware.run_state", "运行状态"), hwEnum("state", sd.state)],
    [hwT("hardware.bmc_addr", "BMC 地址"), snap.target_url],
    [hwT("hardware.overall_health", "整机健康"), m.label],
    [hwT("hardware.bad_parts", "异常部件"), String(s.bad)],
    [hwT("hardware.max_temp", "最高温度"), s.maxTemp ? s.maxTemp.toFixed(0) + "°C" : ""],
    [hwT("hardware.total_power", "总功耗"), s.watts ? s.watts.toFixed(0) + "W" : ""],
    [hwT("hardware.power_redundancy", "电源冗余"), hwEnum("redundancy", (sd.power || {}).redundancy)],
  ];
  if (sd.error) meta.push([hwT("hardware.collect_error", "采集错误"), sd.error]);
  model.meta = meta.filter(([, v]) => v);

  const add = (title, columns, rows) => { if (rows.length) model.sections.push({ title, columns, rows }); };

  add(hwT("hardware.needs_attention", "需要关注"),
    [hwT("hardware.part", "部件"), hwT("hardware.name", "名称"), hwT("hardware.reading", "读数"), hwT("hardware.status", "状态")],
    s.bads.map(b => [b.kind, b.name, b.reading, hwHealthText(b.status)]));

  add(hwT("hardware.events_bmc", "BMC 事件日志（SEL）"),
    [hwT("hardware.event_time", "时间"), hwT("hardware.event_severity", "级别"),
     hwT("hardware.event_component", "触发部件"), hwT("hardware.event_message", "事件内容")],
    (sd.events || []).map(e => [hwFmtTime(e.created), hwHealthText(e.severity), D(e.component), e.message || ""]));

  // 历史概况：网页有温度/风扇/功耗/健康的曲线图，导出此前完全没有——这里用 最小/最大/均值/最新
  // 概括当前所选时间范围（HW_HIST_RANGE），把"历史"补进导出。
  add(hwT("hardware.history_summary", "历史概况") + `（${HW_HIST_RANGE}）`,
    [hwT("hardware.metric", "指标"), hwT("hardware.min", "最小值"), hwT("hardware.max", "最大值"),
     hwT("hardware.avg", "均值"), hwT("hardware.latest", "最新")],
    Object.values(HW_HIST_SUMMARY).map(h => {
      const f = h.fmt || (v => v.toFixed(0));
      return [h.label, f(h.min), f(h.max), f(h.avg), h.latest != null ? f(h.latest) : "-"];
    }));

  add(hwT("hardware.cpu", "CPU"),
    [hwT("hardware.name", "名称"), hwT("hardware.model", "型号"), hwT("hardware.cores_threads", "核心/线程"),
     hwT("hardware.max_freq", "最大频率"), hwT("hardware.health", "健康")],
    (sd.cpus || []).map(c => [c.name, D(c.model), `${c.cores || "?"}C / ${c.threads || "?"}T`,
      c.max_freq_mhz ? c.max_freq_mhz + "MHz" : "-", hwHealthText(c.health)]));

  add(hwT("hardware.gpu", "GPU / 加速卡"),
    [hwT("hardware.name", "名称"), hwT("hardware.model", "型号"), hwT("hardware.manufacturer", "制造商"),
     hwT("hardware.max_freq", "最大频率"), hwT("hardware.status", "状态"), hwT("hardware.health", "健康")],
    (sd.gpus || []).map(g => [g.name, D(g.model), D(g.manufacturer),
      g.max_freq_mhz ? g.max_freq_mhz + "MHz" : "-", hwEnum("state", g.state) || "-", hwHealthText(g.health)]));

  add(hwT("hardware.memory", "内存"),
    [hwT("hardware.slot", "插槽"), hwT("hardware.capacity", "容量"), hwT("hardware.type", "类型"),
     hwT("hardware.speed", "速率"), hwT("hardware.manufacturer", "制造商"), hwT("hardware.part_number", "部件号"),
     hwT("hardware.serial", "序列号"), hwT("hardware.health", "健康")],
    (mem.dimms || []).map(d => [D(d.slot || d.name), (d.capacity_gb || 0).toFixed(0) + "GB", D(d.type),
      d.speed_mhz ? d.speed_mhz + "MHz" : "-", D(d.manufacturer), D(d.part_number), D(d.serial_number),
      hwHealthText(d.health)]));

  add(hwT("hardware.storage", "存储"),
    [hwT("hardware.location", "槽位"), hwT("hardware.name", "名称"), hwT("hardware.model", "型号"),
     hwT("hardware.type", "类型"), hwT("hardware.capacity", "容量"), hwT("hardware.serial", "序列号"),
     hwT("hardware.disk_fw", "盘固件"), hwT("hardware.life_left", "剩余寿命"),
     hwT("hardware.smart", "SMART"), hwT("hardware.health", "健康")],
    (sd.storage || []).map(d => [D(d.location), d.name, D(d.model),
      D([d.media_type, d.protocol].filter(Boolean).join(" / ")), (d.capacity_gb || 0).toFixed(0) + "GB",
      D(d.serial_number), D(d.revision), (d.life_left_pct >= 0) ? d.life_left_pct.toFixed(0) + "%" : "-",
      d.smart_warn ? hwT("hardware.smart_fail", "⚠ 预测故障") : hwT("hardware.smart_ok", "正常"),
      hwHealthText(d.health)]));

  add(hwT("hardware.enclosure", "磁盘框"),
    [hwT("hardware.name", "名称"), hwT("hardware.model", "型号"), hwT("hardware.location", "槽位"),
     hwT("hardware.serial", "序列号"), hwT("hardware.temperature", "温度传感器"),
     hwT("hardware.status", "状态"), hwT("hardware.health", "健康")],
    (sd.enclosures || []).map(e => [e.name, D(e.model), D(e.location), D(e.serial_number),
      e.temperature_c ? e.temperature_c.toFixed(0) + "°C" : "-", hwEnum("state", e.state) || "-",
      hwHealthText(e.health)]));

  add(hwT("hardware.raid", "RAID / 存储控制器"),
    [hwT("hardware.name", "名称"), hwT("hardware.model", "型号"), hwT("hardware.firmware", "固件版本"),
     hwT("hardware.cache", "缓存"), hwT("hardware.drive_count", "挂载盘数"), hwT("hardware.speed", "速率"),
     hwT("hardware.serial", "序列号"), hwT("hardware.health", "健康")],
    (sd.raid || []).map(r => [r.name, D(r.model), D(r.firmware_version),
      r.cache_mb ? r.cache_mb.toFixed(0) + "MB" : "-", D(r.drive_count),
      r.speed_gbps ? r.speed_gbps + "Gbps" : "-", D(r.serial_number), hwHealthText(r.health)]));

  add(hwT("hardware.volumes", "逻辑卷"),
    [hwT("hardware.raid", "RAID / 存储控制器"), hwT("hardware.name", "名称"), hwT("hardware.raid_level", "RAID 级别"),
     hwT("hardware.capacity", "容量"), hwT("hardware.status", "状态"), hwT("hardware.health", "健康")],
    (sd.raid || []).flatMap(r => (r.volumes || []).map(v => [r.name, v.name, D(v.raid_type),
      v.capacity_gb ? v.capacity_gb.toFixed(0) + "GB" : "-", hwEnum("state", v.state) || "-", hwHealthText(v.health)])));

  add(hwT("hardware.power_supply", "电源"),
    [hwT("hardware.name", "名称"), hwT("hardware.model", "型号"), hwT("hardware.input_watts", "输入(W)"),
     hwT("hardware.output_watts", "输出(W)"), hwT("hardware.rated_watts", "额定功率"),
     hwT("hardware.input_voltage", "输入电压"), hwT("hardware.serial", "序列号"),
     hwT("hardware.status", "状态"), hwT("hardware.health", "健康")],
    psus.map(p => [p.name, D(p.model), p.input_watts ? p.input_watts.toFixed(0) + "W" : "-",
      p.output_watts ? p.output_watts.toFixed(0) + "W" : "-", p.capacity_watts ? p.capacity_watts.toFixed(0) + "W" : "-",
      p.line_input_voltage ? p.line_input_voltage.toFixed(0) + "V" : "-", D(p.serial_number),
      hwEnum("state", p.state) || "-", hwHealthText(p.health)]));

  add(hwT("hardware.fans", "风扇"),
    [hwT("hardware.name", "名称"), hwT("hardware.rpm", "转速"), hwT("hardware.status", "状态"), hwT("hardware.health", "健康")],
    (sd.fans || []).map(f => [f.name, f.rpm + " RPM", hwEnum("state", f.status) || "-", hwHealthText(f.health)]));

  add(hwT("hardware.temperature", "温度传感器"),
    [hwT("hardware.sensor", "传感器"), hwT("hardware.reading", "读数"), hwT("hardware.caution_threshold", "告警阈值"),
     hwT("hardware.crit_threshold", "严重阈值"), hwT("hardware.status", "状态")],
    (sd.temps || []).map(t => [t.name, t.reading + "°C",
      t.upper_caution > 0 ? t.upper_caution + "°C" : "-", t.upper_critical > 0 ? t.upper_critical + "°C" : "-",
      hwHealthText(hwTempOver(t) || t.status)]));

  add(hwT("hardware.firmware", "固件版本"),
    [hwT("hardware.name", "名称"), hwT("hardware.version", "版本")],
    (sd.firmware || []).map(f => [f.name, f.version]));

  add(hwT("hardware.events_local", "监控记录的状态变化"),
    [hwT("hardware.event_time", "时间"), hwT("hardware.event_severity", "级别"), hwT("hardware.event_message", "事件内容")],
    HW_LOCAL_EVENTS.map(e => {
      const sev = e.severity ? e.severity.charAt(0).toUpperCase() + e.severity.slice(1) : "";
      return [hwFmtTime(e.created_at), sev ? hwHealthText(sev) : "-", e.message || e.event_type || ""];
    }));

  return model;
}

// 把导出模型拍平成纯文本，喂给 AI 诊断——与导出/网页同一份数据，保证"看到什么就分析什么"。
function hwModelToText(model) {
  let s = (model.title || "设备") + "\n";
  (model.meta || []).forEach(([k, v]) => { s += `${k}: ${v}\n`; });
  (model.sections || []).forEach(sec => {
    s += `\n【${sec.title}】\n`;
    if (sec.columns && sec.columns.length) s += sec.columns.join(" | ") + "\n";
    (sec.rows || []).forEach(r => { s += r.join(" | ") + "\n"; });
  });
  return s;
}

function hwToggleExportMenu(show) {
  const menu = $("hwExportMenu"), btn = $("hwExportBtn");
  if (!menu || !btn) return;
  const open = show === undefined ? !menu.classList.contains("show") : show;
  menu.classList.toggle("show", open);
  btn.setAttribute("aria-expanded", open ? "true" : "false");
}

function hwDoExport(fmt) {
  if (!HW_CUR) return;
  hwToggleExportMenu(false);
  try {
    const model = hwExportModel(HW_CUR);
    const sys = (HW_CUR.snap.snapshot || {}).system || {};
    const base = [hwT("hardware.export_prefix", "硬件资产"),
                  HW_CUR.snap.target_name || HW_CUR.host.id, sys.model].filter(Boolean).join("_");
    const ok = exportModel(model, fmt, base);
    if (!ok) {
      // PDF 走 window.open，被浏览器拦了必须说清楚，否则用户以为按钮坏了
      toast(hwT("hardware.export_popup_blocked", "导出失败：请允许本站弹出窗口后重试"), "err");
      return;
    }
    if (fmt !== "pdf") toast(hwT("toast.exported", "已导出"), "ok");
  } catch (e) {
    toast(hwT("hardware.export_failed", "导出失败") + "：" + e.message, "err");
  }
}

/* ---------- 历史曲线 ---------- */

async function loadHwHistory() {
  if (!HW_CUR) return;
  const hostID = HW_CUR.host.id, target = HW_CUR.snap.target_name || "";
  HW_CHARTS = {};
  HW_HIST_SUMMARY = {};
  const specs = [
    ["hwChartTemp", "temperature", hwT("hardware.temperature", "温度传感器") + " (°C)", v => v.toFixed(0) + "°C"],
    ["hwChartFan", "fan_rpm", hwT("hardware.fans", "风扇") + " (RPM)", v => v.toFixed(0)],
    ["hwChartPower", "power", hwT("hardware.power", "功耗") + " (W)", v => v.toFixed(0) + "W"],
    ["hwChartHealth", "health_score", hwT("hardware.health", "健康") + " (2=OK/1=Warning/0=Critical)", v => v.toFixed(0)],
  ];
  await Promise.all(specs.map(async ([cid, metric, title, fmt]) => {
    try {
      const qs = new URLSearchParams({ host: hostID, metric, range: HW_HIST_RANGE });
      if (target) qs.set("target", target);
      const d = await fetch(`${API}/hardware/history?${qs}`).then(r => r.json());
      const series = hwParseSeries(d.points || []);
      // 汇总该指标历史概况（min/max/avg/最新），供导出的「历史概况」段——补上导出此前缺失的历史数据
      const allVals = series.flatMap(s => s.pts.map(p => p[1])).filter(v => !isNaN(v));
      if (allVals.length) {
        const lastVals = series.map(s => (s.pts.length ? s.pts[s.pts.length - 1][1] : null)).filter(v => v != null && !isNaN(v));
        HW_HIST_SUMMARY[metric] = {
          label: title, fmt,
          min: Math.min(...allVals), max: Math.max(...allVals),
          avg: allVals.reduce((a, b) => a + b, 0) / allVals.length,
          latest: lastVals.length ? lastVals.reduce((a, b) => a + b, 0) / lastVals.length : null,
        };
      }
      if (!series.length) {
        const c = $(cid);
        if (c) drawChartEmpty(c.getContext("2d"), c.getBoundingClientRect().width || 1000, 200,
          hwT("hardware.no_history", "暂无历史数据（需等待采集积累）"));
        return;
      }
      // 把多序列（每个传感器/风扇一条）对齐成 createChart 需要的 samples 结构
      const tsSet = new Set();
      series.forEach(s => s.pts.forEach(p => tsSet.add(p[0])));
      const samples = [...tsSet].sort((a, b) => a - b).map(ts => {
        const row = { timestamp: ts };
        series.forEach((s, i) => { const hit = s.pts.find(p => p[0] === ts); row["v" + i] = hit ? hit[1] : null; });
        return row;
      });
      const palette = ["#4c8dff", "#f7b23b", "#2fd07a", "#f2545b", "#8b5cf6", "#43b6f0", "#e06c9a", "#6ac4b8"];
      const defs = series.slice(0, 8).map((s, i) => ({
        key: "v" + i, label: s.name, color: palette[i % palette.length], fmt,
      }));
      HW_CHARTS[cid] = createChart(cid, samples, defs, null, null, { title });
    } catch (e) { /* 单图失败不影响其它图 */ }
  }));
}

// 把 Prometheus data.result 解析成 [{name, pts:[[tsSec, val]]}]
function hwParseSeries(points) {
  const out = [];
  (points || []).forEach(p => {
    if (!p || !p.values) return;
    const lbl = p.metric || {};
    const name = lbl.sensor || lbl.fan_name || lbl.target || "value";
    const pts = p.values.map(v => [Number(v[0]), parseFloat(v[1])]).filter(v => !isNaN(v[1]));
    if (pts.length) out.push({ name, pts });
  });
  return out;
}

/* ---------- 视图切换 ---------- */

function switchHwView(mode) {
  HW_VIEW_MODE = mode === "list" ? "list" : "card";
  try { localStorage.setItem("aiops_hw_view", HW_VIEW_MODE); } catch (e) {}
  document.querySelectorAll("#hwViewToggle .vt-btn").forEach(b =>
    b.classList.toggle("active", b.dataset.view === HW_VIEW_MODE));
  renderHardwarePanel();
}

/* ---------- 事件（全部委托，符合 CSP script-src 'self'） ---------- */

safeAddEventListener("hardwarePanel", "click", e => {
  if (e.target.closest("[data-hwdel]")) return; // 删除按钮自己处理，不打开详情
  const item = e.target.closest("[data-hwidx]");
  if (item) openHwDetail(parseInt(item.dataset.hwidx));
});
safeAddEventListener("hardwarePanel", "keydown", e => {
  if (e.key !== "Enter" && e.key !== " ") return;
  const item = e.target.closest("[data-hwidx]");
  if (item) { e.preventDefault(); openHwDetail(parseInt(item.dataset.hwidx)); }
});
safeAddEventListener("hwViewToggle", "click", e => {
  const b = e.target.closest("[data-view]");
  if (b) switchHwView(b.dataset.view);
});
safeAddEventListener("hwRefreshBtn", "click", loadHardwarePanel);
safeAddEventListener("hardwarePanel", "click", e => {
  const delBtn = e.target.closest("[data-hwdel]");
  if (delBtn) {
    e.stopPropagation();
    const target = delBtn.dataset.hwdel;
    const hostID = delBtn.dataset.hwhost;
    if (!target || !hostID) return;
    if (!confirm(hwT("hardware.confirm_delete", "确定删除该硬件资产记录？删除后不可恢复。"))) return;
    fetch(`${API}/hardware/${encodeURIComponent(hostID)}?target=${encodeURIComponent(target)}`, { method: "DELETE" })
      .then(r => r.ok ? r.json() : Promise.reject(r.statusText))
      .then(() => { toast(hwT("toast.deleted", "已删除"), "ok"); loadHardwarePanel(); })
      .catch(err => toast(hwT("hardware.delete_failed", "删除失败") + ": " + err, "err"));
    return;
  }
});

/* ---------- 筛选 / 搜索 / 重复主机清理（工具栏是重渲染出来的，一律事件委托） ---------- */

let HW_SEARCH_T = null;
safeAddEventListener("hardwarePanel", "input", e => {
  if (e.target.id !== "hwSearch") return;
  // 防抖：每敲一个字就重建整个网格会明显卡顿
  clearTimeout(HW_SEARCH_T);
  const v = e.target.value;
  HW_SEARCH_T = setTimeout(() => {
    HW_FILTER.q = v;
    renderHardwarePanel();
    // 重渲染会丢焦点，搜索框必须还原焦点和光标位置，否则打不完一个词
    const s = $("hwSearch");
    if (s) { s.focus(); s.setSelectionRange(s.value.length, s.value.length); }
  }, 200);
});
safeAddEventListener("hardwarePanel", "change", e => {
  if (e.target.id === "hwStatusFilter") { HW_FILTER.status = e.target.value; renderHardwarePanel(); }
  else if (e.target.id === "hwFreshFilter") { HW_FILTER.fresh = e.target.value; renderHardwarePanel(); }
});

// 重复主机的提示/查看/清理逻辑在 duplicates.js 里，主机页与硬件页共用一份。
dupBindPanel("hardwarePanel", loadHardwarePanel);

// 导出下拉：按钮开合 + 选项点击 + 点外部/Esc 收起
// AI 诊断：把该设备完整硬件快照喂给 AI 分析整体运行状态，走 /ai/assist 流式 + 自动沉淀记忆(闭环)
safeAddEventListener("hwAIBtn", "click", () => {
  if (!HW_CUR) return;
  if (typeof openAIAssist !== "function") { toast(hwT("hardware.ai_unavailable", "AI 面板未就绪"), "err"); return; }
  const model = hwExportModel(HW_CUR);
  openAIAssist({
    task: "hardware_diagnosis",
    title: "🤖 AI 硬件诊断 · " + (model.title || "设备"),
    mode: "analyze",
    context: hwModelToText(model).slice(0, 14000)
  });
});
safeAddEventListener("hwExportBtn", "click", e => { e.stopPropagation(); hwToggleExportMenu(); });
safeAddEventListener("hwExportMenu", "click", e => {
  const o = e.target.closest("[data-hwexport]");
  if (o) hwDoExport(o.dataset.hwexport);
});
document.addEventListener("click", e => {
  // 点在下拉自身之内不收起（选项的 click 由上面的委托处理）
  if (!e.target.closest("#hwExportDD")) hwToggleExportMenu(false);
});
document.addEventListener("keydown", e => { if (e.key === "Escape") hwToggleExportMenu(false); });
safeAddEventListener("hwDetailBody", "click", e => {
  const r = e.target.closest("[data-hwrange]");
  if (r) {
    HW_HIST_RANGE = r.dataset.hwrange;
    document.querySelectorAll("#hwDetailBody [data-hwrange]").forEach(b =>
      b.classList.toggle("active", b.dataset.hwrange === HW_HIST_RANGE));
    loadHwHistory();
    return;
  }
  const z = e.target.closest("[data-hwchart]");
  if (z) { const ch = HW_CHARTS[z.dataset.hwchart]; if (ch) openChartZoom(ch); }
});

// 供 nav.js 的 _pageRenderers 调用
if (typeof window._pageRenderers === "undefined") window._pageRenderers = {};
window._pageRenderers.hardware = loadHardwarePanel;
