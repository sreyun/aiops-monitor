// duplicates.js — 重复主机记录（Agent 重装换 ID）的提示与清理，主机页/硬件页共用。
//
// 成因与治本方案见服务端 store.go 的 CanonicalHostID：新版 Agent 会在启动时按机器
// 指纹认回原有 host_id，从根上不再产生重复。这里处理的是**存量**：升级之前已经攒下
// 的孤儿记录，以及尚未升级的老 Agent。
//
// 注意：平台里一切都按 host_id 存（VM 指标 host 标签、日志、告警、硬件快照/变更、
// Flow 明细），所以一条重复记录不只是列表里多一行，而是把这台机器的历史劈成了两半。

let DUP_STATE = { groups: [], stale_total: 0 };

const dupT = (k, fb) => I18N.t(k, fb);

// loadDuplicates 拉取重复分组；失败静默（这只是辅助信息，不该阻塞页面）。
async function loadDuplicates(onDone) {
  try {
    const d = await fetch(`${API}/hosts/duplicates`, { credentials: "same-origin" }).then(r => r.json());
    DUP_STATE = d || { groups: [], stale_total: 0 };
  } catch (e) {
    DUP_STATE = { groups: [], stale_total: 0 };
  }
  if (onDone) onDone(DUP_STATE);
}

// dupBannerHTML 只在**确有可清理项**时才返回内容：没有孤儿记录时不该常驻一条横幅。
function dupBannerHTML() {
  if (!DUP_STATE.stale_total) return "";
  return `<div class="hw-dup-bar">
    <span>⚠ ${esc(dupT("hardware.dup_found", "检测到重复主机记录"))}
      ${DUP_STATE.stale_total} ${esc(dupT("hardware.dup_stale_hint", "条可清理（Agent 重装会生成新的主机 ID，旧记录会一直残留）"))}</span>
    <button class="btn sm" data-dupact="detail">${esc(dupT("hardware.dup_detail", "查看"))}</button>
    <button class="btn sm danger" data-dupact="clean">${esc(dupT("hardware.dup_clean", "清理"))}</button>
  </div>`;
}

function dupLines() {
  return DUP_STATE.groups.map(g =>
    `${g.hostname}:\n` + g.hosts.map(h =>
      `  ${String(h.id).slice(0, 12)}… ${h.online ? "在线" : "离线"}` +
      `${h.current ? " ← 当前身份（保留）" : h.stale ? " ← 可清理" : " ← 保留（仍在上报）"}` +
      `  最后上报 ${new Date((h.last_seen || 0) * 1000).toLocaleString()}`
    ).join("\n")
  ).join("\n\n");
}

function dupShowDetail() {
  alert(dupT("hardware.dup_title", "重复主机记录（同一台物理机被登记了多次）") + "\n\n" + dupLines());
}

// dupCleanup 删除孤儿记录。删除**不可逆**，所以先把要删的东西原样摊开给用户看，
// 而不是只弹一句"确定吗"。
async function dupCleanup(onDone) {
  const msg = dupT("hardware.dup_confirm", "将删除以下已离线的重复主机记录（不影响当前在用的那条，也不会动仍在上报的主机）：")
    + "\n\n" + dupLines() + "\n\n" + dupT("hardware.dup_confirm2", "该操作不可撤销，确定继续？");
  if (!confirm(msg)) return;
  try {
    const r = await fetch(`${API}/hosts/duplicates/cleanup`, {
      method: "POST", credentials: "same-origin",
    }).then(r => r.json());
    toast(`${dupT("hardware.dup_cleaned", "已清理重复主机记录")} ${r.count || 0}`, "ok");
    window._cachedHosts = null; // 主机列表已变，强制重新拉取
    if (onDone) onDone();
  } catch (e) {
    toast(dupT("hardware.dup_clean_failed", "清理失败") + "：" + e.message, "err");
  }
}

// dupBindPanel 给一个容器绑定横幅上的两个按钮（容器内容会被重渲染，故用事件委托）。
function dupBindPanel(containerID, onDone) {
  safeAddEventListener(containerID, "click", e => {
    const b = e.target.closest("[data-dupact]");
    if (!b) return;
    if (b.dataset.dupact === "detail") dupShowDetail();
    else if (b.dataset.dupact === "clean") dupCleanup(onDone);
  });
}
