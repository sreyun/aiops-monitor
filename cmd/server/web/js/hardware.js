// hardware.js — 硬件健康面板 (Hardware Health Panel)
// Loaded as part of the unified app.js bundle.

(function() {
"use strict";

// Render hardware health cards for all hosts with Redfish data.
function renderHardwarePanel() {
  const container = $("hardwarePanel");
  if (!container) return;

  // Get all hosts and fetch hardware data for each
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
      const temps = (snapData.temps || []).slice(0, 4);
      const fans = (snapData.fans || []).slice(0, 4);
      const power = snapData.power || {};
      const cpus = snapData.cpus || [];
      const storage = snapData.storage || [];

      html += `<div class="hw-card" onclick="toggleHwDetail(this)">`;
      html += `<div class="hw-card-header">`;
      html += `<span class="hw-health-dot ${healthClass}">${healthIcon}</span>`;
      html += `<div class="hw-card-info">`;
      html += `<div class="hw-card-name">${esc(snap.target_name || snap.target_url)}</div>`;
      html += `<div class="hw-card-sub">${esc(host.hostname || host.id)} · ${esc(snap.health || "Unknown")}</div>`;
      html += `</div></div>`;

      // Quick stats
      html += `<div class="hw-quick-stats">`;
      if (temps.length > 0) {
        const maxTemp = Math.max(...temps.map(t => t.reading || 0));
        html += `<span class="hw-stat" title="${I18N.t("hardware.max_temp") || "最高温度"}">${maxTemp.toFixed(0)}°C</span>`;
      }
      if (power.total_watts > 0) {
        html += `<span class="hw-stat" title="${I18N.t("hardware.power") || "功耗"}">${power.total_watts.toFixed(0)}W</span>`;
      }
      if (cpus.length > 0) {
        html += `<span class="hw-stat" title="CPU">${cpus.length} × ${cpus[0].cores || "?"}C</span>`;
      }
      html += `</div>`;

      // Expandable detail
      html += `<div class="hw-detail" style="display:none">`;

      // CPU section
      if (cpus.length > 0) {
        html += `<h4>CPU</h4><table class="hw-table"><tr><th>${I18N.t("hardware.name") || "名称"}</th><th>${I18N.t("hardware.model") || "型号"}</th><th>${I18N.t("hardware.cores") || "核心"}</th><th>${I18N.t("hardware.health") || "健康"}</th></tr>`;
        cpus.forEach(c => {
          html += `<tr><td>${esc(c.name)}</td><td>${esc(c.model)}</td><td>${c.cores}C/${c.threads}T</td><td>${esc(c.health)}</td></tr>`;
        });
        html += `</table>`;
      }

      // Temperature section
      if (temps.length > 0) {
        html += `<h4>${I18N.t("hardware.temperature") || "温度传感器"}</h4><table class="hw-table"><tr><th>${I18N.t("hardware.sensor") || "传感器"}</th><th>${I18N.t("hardware.reading") || "读数"}</th><th>${I18N.t("hardware.status") || "状态"}</th></tr>`;
        temps.forEach(t => {
          const warn = t.upper_caution > 0 && t.reading > t.upper_caution ? " hw-warn-text" : "";
          html += `<tr class="${warn}"><td>${esc(t.name)}</td><td>${t.reading}°C</td><td>${esc(t.status)}</td></tr>`;
        });
        html += `</table>`;
      }

      // Fan section
      if (fans.length > 0) {
        html += `<h4>${I18N.t("hardware.fans") || "风扇"}</h4><table class="hw-table"><tr><th>${I18N.t("hardware.name") || "名称"}</th><th>RPM</th><th>${I18N.t("hardware.health") || "健康"}</th></tr>`;
        fans.forEach(f => {
          html += `<tr><td>${esc(f.name)}</td><td>${f.rpm}</td><td>${esc(f.health)}</td></tr>`;
        });
        html += `</table>`;
      }

      // Storage section
      if (storage.length > 0) {
        html += `<h4>${I18N.t("hardware.storage") || "存储"}</h4><table class="hw-table"><tr><th>${I18N.t("hardware.name") || "名称"}</th><th>${I18N.t("hardware.type") || "类型"}</th><th>${I18N.t("hardware.capacity") || "容量"}</th><th>${I18N.t("hardware.health") || "健康"}</th></tr>`;
        storage.forEach(s => {
          const smartWarn = s.smart_warn ? " hw-crit-text" : "";
          html += `<tr class="${smartWarn}"><td>${esc(s.name)}</td><td>${esc(s.media_type || s.protocol)}</td><td>${(s.capacity_gb || 0).toFixed(0)}GB</td><td>${esc(s.health)}</td></tr>`;
        });
        html += `</table>`;
      }

      // Power section
      if (power.psus && power.psus.length > 0) {
        html += `<h4>${I18N.t("hardware.power_supply") || "电源"}</h4>`;
        html += `<div class="hw-power-info">${I18N.t("hardware.redundancy") || "冗余"}: ${esc(power.redundancy || "N/A")}</div>`;
        html += `<table class="hw-table"><tr><th>PSU</th><th>${I18N.t("hardware.input_watts") || "输入(W)"}</th><th>${I18N.t("hardware.health") || "健康"}</th></tr>`;
        power.psus.forEach(p => {
          html += `<tr><td>${esc(p.name)}</td><td>${p.input_watts}W</td><td>${esc(p.health)}</td></tr>`;
        });
        html += `</table>`;
      }

      // Firmware section
      if (snapData.firmware && snapData.firmware.length > 0) {
        html += `<h4>${I18N.t("hardware.firmware") || "固件版本"}</h4><table class="hw-table"><tr><th>${I18N.t("hardware.name") || "名称"}</th><th>${I18N.t("hardware.version") || "版本"}</th></tr>`;
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
  if (detail) {
    detail.style.display = detail.style.display === "none" ? "block" : "none";
    card.classList.toggle("expanded");
  }
};

// Register with navigation
if (typeof window._pageRenderers === "undefined") window._pageRenderers = {};
window._pageRenderers.hardware = renderHardwarePanel;

})();
