// hardware.js — 硬件健康面板 (Hardware Health Panel)
// Loaded as part of the unified app.js bundle.

(function() {
"use strict";

// Render hardware health cards for all hosts with Redfish data.
function renderHardwarePanel() {
  const container = $("hardwarePanel");
  if (!container) return;

  const hosts = (window._cachedHosts || []);
  if (hosts.length === 0) {
    container.innerHTML = `<div class="empty-state">${I18N.t("hardware.no_hosts") || "暂无主机"}</div>`;
    return;
  }

  container.innerHTML = `<div class="loading-dots">${I18N.t("common.loading") || "加载中..."}</div>`;

  let pending = 0;
  const results = [];

  hosts.forEach(h => {
    if (!h.online) return;
    pending++;
    fetch(`/api/v1/hardware/health?host=${encodeURIComponent(h.id)}`, { credentials: "same-origin" })
      .then(r => r.json())
      .then(data => {
        if (data.snapshots && data.snapshots.length > 0) {
          results.push({ host: h, snapshots: data.snapshots });
        }
      })
      .catch(() => {})
      .finally(() => {
        pending--;
        if (pending === 0) renderHardwareCards(container, results);
      });
  });

  if (pending === 0) {
    container.innerHTML = `<div class="empty-state">${I18N.t("hardware.no_data") || "暂无硬件数据（需在 Agent 配置 Redfish 目标）"}</div>`;
  }
}

// ── Helper: format timestamp to local time string ──
function fmtTime(ts) {
  if (!ts) return "";
  const d = new Date(typeof ts === "number" && ts < 1e12 ? ts * 1000 : ts);
  return d.toLocaleString();
}

function renderHardwareCards(container, results) {
  if (results.length === 0) {
    container.innerHTML = `<div class="empty-state">${I18N.t("hardware.no_data") || "暂无硬件数据（需在 Agent 配置 Redfish 目标）"}</div>`;
    return;
  }

  let html = '<div class="hw-grid">';
  results.forEach(({ host, snapshots }) => {
    snapshots.forEach(snap => {
      const healthClass = snap.health === "OK" ? "hw-ok" : snap.health === "Warning" ? "hw-warn" : "hw-crit";
      const healthIcon = snap.health === "OK" ? "✓" : snap.health === "Warning" ? "⚠" : "✕";
      const snapData = snap.snapshot || {};
      const temps = snapData.temps || [];
      const fans = snapData.fans || [];
      const power = snapData.power || {};
      const cpus = snapData.cpus || [];
      const storage = snapData.storage || [];
      const memory = snapData.memory || {};
      const dimms = memory.dimms || [];

      // ── Card Header ──
      html += `<div class="hw-card" onclick="toggleHwDetail(this)">`;
      html += `<div class="hw-card-header">`;
      html += `<span class="hw-health-dot ${healthClass}">${healthIcon}</span>`;
      html += `<div class="hw-card-info">`;
      html += `<div class="hw-card-name">${esc(snap.target_name || snap.target_url)}</div>`;
      html += `<div class="hw-card-sub">${esc(host.hostname || host.id)} · ${esc(snap.health || "Unknown")}</div>`;
      html += `</div></div>`;

      // ── Quick Stats (摘要标签) ──
      html += `<div class="hw-quick-stats">`;
      if (memory.total_gb > 0) {
        html += `<span class="hw-stat" title="${I18N.t("hardware.memory") || "内存"}">${memory.total_gb.toFixed(0)}GB</span>`;
      }
      if (cpus.length > 0) {
        html += `<span class="hw-stat" title="CPU">${cpus.length} × ${cpus[0].cores || "?"}C</span>`;
      }
      if (temps.length > 0) {
        const maxTemp = Math.max(...temps.map(t => t.reading || 0));
        html += `<span class="hw-stat" title="${I18N.t("hardware.max_temp") || "最高温度"}">${maxTemp.toFixed(0)}°C</span>`;
      }
      if (power.total_watts > 0) {
        html += `<span class="hw-stat" title="${I18N.t("hardware.power") || "功耗"}">${power.total_watts.toFixed(0)}W</span>`;
      }
      if (storage.length > 0) {
        html += `<span class="hw-stat" title="${I18N.t("hardware.storage") || "存储"}">${storage.length}${I18N.t("hardware.disk_unit") || "盘"}</span>`;
      }
      if (fans.length > 0) {
        html += `<span class="hw-stat" title="${I18N.t("hardware.fans") || "风扇"}">${fans.length}</span>`;
      }
      html += `</div>`;

      // ── Expand hint ──
      html += `<div class="hw-expand-hint">${I18N.t("hardware.expand_hint") || "点击展开详情 ▼"}</div>`;

      // ══════════════════════════════════════════
      // Expandable detail section (点击展开/收起)
      // ══════════════════════════════════════════
      html += `<div class="hw-detail" style="display:none">`;

      // ── Metadata: timestamp + error ──
      if (snap.updated_at || snapData.timestamp) {
        const t = snap.updated_at ? new Date(snap.updated_at).toLocaleString() : fmtTime(snapData.timestamp);
        html += `<div class="hw-meta-row"><span class="hw-meta-label">${I18N.t("hardware.updated") || "更新时间"}</span> ${esc(t)}</div>`;
      }
      if (snapData.error) {
        html += `<div class="hw-meta-row hw-error-row"><span class="hw-meta-label">${I18N.t("hardware.error") || "错误"}</span> <span class="hw-crit-text">${esc(snapData.error)}</span></div>`;
      }

      // ── 1. CPU ──
      if (cpus.length > 0) {
        html += `<h4>CPU (${cpus.length})</h4>`;
        html += `<table class="hw-table"><tr><th>${I18N.t("hardware.name") || "名称"}</th><th>${I18N.t("hardware.model") || "型号"}</th><th>${I18N.t("hardware.cores") || "核心/线程"}</th><th>${I18N.t("hardware.max_freq") || "最大频率"}</th><th>${I18N.t("hardware.health") || "健康"}</th></tr>`;
        cpus.forEach(c => {
          const freq = c.max_freq_mhz ? `${c.max_freq_mhz}MHz` : "-";
          html += `<tr><td>${esc(c.name)}</td><td>${esc(c.model)}</td><td>${c.cores}C/${c.threads}T</td><td>${freq}</td><td>${esc(c.health)}</td></tr>`;
        });
        html += `</table>`;
      }

      // ── 2. Memory ──
      if (memory.total_gb > 0) {
        html += `<h4>${I18N.t("hardware.memory") || "内存"} (${memory.total_gb.toFixed(0)}GB`;
        if (memory.used_gb > 0) html += ` / ${I18N.t("hardware.used") || "已用"} ${memory.used_gb.toFixed(0)}GB`;
        html += `)</h4>`;
        if (dimms.length > 0) {
          html += `<table class="hw-table"><tr><th>${I18N.t("hardware.slot") || "插槽"}</th><th>${I18N.t("hardware.capacity") || "容量"}</th><th>${I18N.t("hardware.type") || "类型"}</th><th>${I18N.t("hardware.speed") || "速率"}</th><th>${I18N.t("hardware.health") || "健康"}</th></tr>`;
          dimms.forEach(d => {
            html += `<tr><td>${esc(d.slot || d.name)}</td><td>${(d.capacity_gb || 0).toFixed(0)}GB</td><td>${esc(d.type || "-")}</td><td>${d.speed_mhz ? d.speed_mhz + "MHz" : "-"}</td><td>${esc(d.health || "-")}</td></tr>`;
          });
          html += `</table>`;
        }
      }

      // ── 3. Temperature Sensors (all) ──
      if (temps.length > 0) {
        html += `<h4>${I18N.t("hardware.temperature") || "温度传感器"} (${temps.length})</h4>`;
        html += `<table class="hw-table"><tr><th>${I18N.t("hardware.sensor") || "传感器"}</th><th>${I18N.t("hardware.reading") || "读数"}</th><th>${I18N.t("hardware.caution_threshold") || "告警阈值"}</th><th>${I18N.t("hardware.status") || "状态"}</th></tr>`;
        temps.forEach(t => {
          const warn = t.upper_caution > 0 && t.reading > t.upper_caution ? " hw-warn-text" : "";
          const crit = t.upper_critical > 0 && t.reading > t.upper_critical ? " hw-crit-text" : "";
          const cls = crit || warn;
          const threshold = t.upper_caution > 0 ? `${t.upper_caution}°C` : "-";
          html += `<tr class="${cls}"><td>${esc(t.name)}</td><td>${t.reading}°C</td><td>${threshold}</td><td>${esc(t.status)}</td></tr>`;
        });
        html += `</table>`;
      }

      // ── 4. Fans (all) ──
      if (fans.length > 0) {
        html += `<h4>${I18N.t("hardware.fans") || "风扇"} (${fans.length})</h4>`;
        html += `<table class="hw-table"><tr><th>${I18N.t("hardware.name") || "名称"}</th><th>RPM</th><th>${I18N.t("hardware.status") || "状态"}</th><th>${I18N.t("hardware.health") || "健康"}</th></tr>`;
        fans.forEach(f => {
          html += `<tr><td>${esc(f.name)}</td><td>${f.rpm}</td><td>${esc(f.status || "-")}</td><td>${esc(f.health)}</td></tr>`;
        });
        html += `</table>`;
      }

      // ── 5. Storage ──
      if (storage.length > 0) {
        html += `<h4>${I18N.t("hardware.storage") || "存储"} (${storage.length})</h4>`;
        html += `<table class="hw-table"><tr><th>${I18N.t("hardware.name") || "名称"}</th><th>${I18N.t("hardware.model") || "型号"}</th><th>${I18N.t("hardware.type") || "类型"}</th><th>${I18N.t("hardware.capacity") || "容量"}</th><th>${I18N.t("hardware.health") || "健康"}</th></tr>`;
        storage.forEach(s => {
          const smartWarn = s.smart_warn ? " hw-crit-text" : "";
          html += `<tr class="${smartWarn}"><td>${esc(s.name)}</td><td>${esc(s.model || "-")}</td><td>${esc(s.media_type || s.protocol || "-")}</td><td>${(s.capacity_gb || 0).toFixed(0)}GB</td><td>${esc(s.health)}</td></tr>`;
        });
        html += `</table>`;
      }

      // ── 6. Power Supply ──
      if (power.psus && power.psus.length > 0) {
        html += `<h4>${I18N.t("hardware.power_supply") || "电源"}</h4>`;
        html += `<div class="hw-power-info">${I18N.t("hardware.redundancy") || "冗余"}: <strong>${esc(power.redundancy || "N/A")}</strong>`;
        if (power.total_watts > 0) html += ` · ${I18N.t("hardware.total_power") || "总功耗"}: <strong>${power.total_watts.toFixed(0)}W</strong>`;
        html += `</div>`;
        html += `<table class="hw-table"><tr><th>PSU</th><th>${I18N.t("hardware.input_watts") || "输入(W)"}</th><th>${I18N.t("hardware.output_watts") || "输出(W)"}</th><th>${I18N.t("hardware.health") || "健康"}</th><th>${I18N.t("hardware.status") || "状态"}</th></tr>`;
        power.psus.forEach(p => {
          html += `<tr><td>${esc(p.name)}</td><td>${p.input_watts}W</td><td>${p.output_watts || "-"}W</td><td>${esc(p.health)}</td><td>${esc(p.state || "-")}</td></tr>`;
        });
        html += `</table>`;
      }

      // ── 7. Firmware ──
      if (snapData.firmware && snapData.firmware.length > 0) {
        html += `<h4>${I18N.t("hardware.firmware") || "固件版本"} (${snapData.firmware.length})</h4>`;
        html += `<table class="hw-table"><tr><th>${I18N.t("hardware.name") || "名称"}</th><th>${I18N.t("hardware.version") || "版本"}</th></tr>`;
        snapData.firmware.forEach(f => {
          html += `<tr><td>${esc(f.name)}</td><td>${esc(f.version)}</td></tr>`;
        });
        html += `</table>`;
      }

      html += `</div></div>`; // hw-detail + hw-card
    });
  });
  html += '</div>';
  container.innerHTML = html;
}

// Toggle hardware detail expand/collapse
window.toggleHwDetail = function(card) {
  const detail = card.querySelector(".hw-detail");
  const hint = card.querySelector(".hw-expand-hint");
  if (detail) {
    const expanding = detail.style.display === "none";
    detail.style.display = expanding ? "block" : "none";
    card.classList.toggle("expanded");
    if (hint) hint.textContent = expanding
      ? (I18N.t("hardware.collapse_hint") || "点击收起 ▲")
      : (I18N.t("hardware.expand_hint") || "点击展开详情 ▼");
  }
};

// Register with navigation
if (typeof window._pageRenderers === "undefined") window._pageRenderers = {};
window._pageRenderers.hardware = renderHardwarePanel;

})();
