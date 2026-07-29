/* 安全情报源 / 模板库管理
 *
 * 更新是异步作业：点击更新后立即返回，前端轮询进度。之前同步下载会在浏览器或
 * 反向代理侧超时，用户看到「更新失败」但服务端其实还在下载。
 *
 * 注意：web-security.js 运行在 IIFE 内，本模块通过 window.__wsFeedsHost 桥接
 * paint / fetch，否则「情报源」按钮点击会因 ReferenceError 完全无反应。
 */

let SF_STATE = null;      // 最近一次 /security/feeds 响应
let SF_OPEN = false;      // 面板是否展开
let SF_POLL = null;       // 轮询定时器
let SF_TESTING = false;   // 连通性测试中
let SF_TEST_RESULT = null;
let SF_LOAD_ERROR = "";   // 最近一次加载失败原因
let SF_PAINT_SIG = "";    // 上次 paint 签名，避免无变化重复重建

// 常用加速镜像：国内直连 GitHub 经常超时，给出可直接选用的预设而不是让用户去查。
const SF_MIRRORS = [
  { label: "不使用（直连 GitHub）", value: "" },
  { label: "ghfast.top", value: "https://ghfast.top/" },
  { label: "gh-proxy.com", value: "https://gh-proxy.com/" },
  { label: "ghproxy.net", value: "https://ghproxy.net/" },
];

function sfT(k, d) { return (typeof I18N !== "undefined" && I18N.t) ? I18N.t(k, d) : d; }
function sfEsc(s) { return (typeof esc === "function") ? esc(String(s == null ? "" : s)) : String(s == null ? "" : s); }

function sfHost() {
  return (typeof window !== "undefined" && window.__wsFeedsHost) ? window.__wsFeedsHost : null;
}
function sfPaint() {
  const h = sfHost();
  if (h && typeof h.paint === "function") {
    h.paint();
    return true;
  }
  if (typeof toast === "function") {
    toast(sfT("sf.host_missing", "情报源面板未能挂载到 Web 扫描页，请刷新后重试"), "err");
  }
  return false;
}
async function sfAPI(url, opts) {
  const h = sfHost();
  if (h && typeof h.fetchJSON === "function") return h.fetchJSON(url, opts);
  const r = await fetch(url, opts || {});
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error || ("HTTP " + r.status));
  return j;
}

function sfKindLabel(kind) {
  if (kind === "nuclei") return sfT("sf.kind_nuclei", "可执行模板");
  if (kind === "signature") return sfT("sf.kind_signature", "检测特征库");
  return sfT("sf.kind_knowledge", "知识/参考库");
}

function sfBytes(n) {
  n = Number(n) || 0;
  if (n <= 0) return "—";
  if (n < 1024 * 1024) return (n / 1024).toFixed(0) + " KB";
  if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + " MB";
  return (n / 1024 / 1024 / 1024).toFixed(2) + " GB";
}

function sfAgo(ts) {
  if (!ts) return sfT("sf.never", "从未更新");
  if (typeof ago === "function") return ago(ts);
  return new Date(ts * 1000).toLocaleString();
}

async function sfLoad() {
  SF_LOAD_ERROR = "";
  try {
    SF_STATE = await sfAPI(`${API}/security/feeds`);
  } catch (e) {
    SF_STATE = null;
    SF_LOAD_ERROR = String((e && e.message) || e || sfT("sf.load_failed", "加载失败"));
  }
  return SF_STATE;
}

function sfStateSig() {
  if (SF_LOAD_ERROR) return "err:" + SF_LOAD_ERROR;
  if (!SF_STATE) return "empty";
  const j = SF_STATE.job || {};
  const feeds = (SF_STATE.feeds || []).map(f =>
    `${f.id || ""}:${f.updated_at || 0}:${f.status || ""}:${f.size || 0}`
  ).join("|");
  return [
    j.running ? 1 : 0,
    j.progress == null ? "" : j.progress,
    j.updated_at || 0,
    j.message || "",
    SF_STATE.updated_at || 0,
    feeds
  ].join(";");
}

function sfPaintIfChanged(force) {
  const sig = sfStateSig();
  if (!force && sig === SF_PAINT_SIG) return false;
  SF_PAINT_SIG = sig;
  return sfPaint();
}

function sfScrollIntoView() {
  try {
    const panel = document.querySelector("#webSecurityPanel .sf-panel");
    if (panel && typeof panel.scrollIntoView === "function") {
      panel.scrollIntoView({ behavior: "smooth", block: "nearest" });
    }
  } catch (_) {}
}

/* ---------- 渲染 ---------- */

function sfPanelHTML() {
  if (!SF_OPEN) return "";
  if (!SF_STATE) {
    if (SF_LOAD_ERROR) {
      return `<div class="cfg-panel sec-cfg-panel sf-panel" id="sfPanel">
        <div class="cfg-panel-head"><div>
          <div class="cfg-panel-title">${sfEsc(sfT("sf.title", "威胁情报源 / 模板库"))}</div>
          <p class="cfg-panel-desc sf-load-err">${sfEsc(SF_LOAD_ERROR)}</p>
        </div></div>
        <div class="cfg-actions">
          <button class="btn primary" data-sf="retry">${sfEsc(sfT("common.retry", "重试"))}</button>
          <button class="btn ghost" data-sf="close">${sfEsc(sfT("common.cancel", "收起"))}</button>
        </div>
      </div>`;
    }
    return `<div class="cfg-panel sec-cfg-panel sf-panel" id="sfPanel"><div class="loading-dots">${sfEsc(sfT("common.loading", "加载中..."))}</div></div>`;
  }
  const cfg = SF_STATE.config || {};
  const job = SF_STATE.job;
  const admin = typeof isAdmin === "function" ? isAdmin() : false;

  return `<div class="cfg-panel sec-cfg-panel sf-panel" id="sfPanel">
    <div class="cfg-panel-head"><div>
      <div class="cfg-panel-title">${sfEsc(sfT("sf.title", "威胁情报源 / 模板库"))}</div>
      <p class="cfg-panel-desc">${sfEsc(sfT("sf.desc", "统一管理扫描所依赖的模板与特征库。Web 与主机安全共用同一套下载通道（代理 / 加速镜像），更新在后台执行，可随时关闭页面。"))}</p>
    </div></div>
    ${sfJobHTML(job)}
    <div class="sf-grid">
      ${sfNetworkCardHTML(cfg)}
      ${sfAutoCardHTML(cfg)}
    </div>
    <div class="sf-sources">
      <div class="sf-sources-head">
        <h4>${sfEsc(sfT("sf.sources", "情报源"))}</h4>
        <span class="ws-help">${sfEsc(sfT("sf.sources_help", "「可执行模板」直接参与扫描；「检测特征库」被内置引擎解析（如 sqlmap 报错指纹用于报错型注入检测）；「知识库」仅作为研判与复现参考，不产生告警。"))}</span>
      </div>
      <div class="sf-list">${(SF_STATE.sources || []).map(s => sfSourceRowHTML(s, admin)).join("") || `<div class="muted">${sfEsc(sfT("sf.no_sources", "暂无可用情报源"))}</div>`}</div>
    </div>
    <div class="cfg-actions">
      ${admin ? `<button class="btn primary" data-sf="save">${sfEsc(sfT("common.save", "保存"))}</button>
      <button class="btn" data-sf="update-all"${job && job.running ? " disabled" : ""}>${sfEsc(sfT("sf.update_all", "更新已启用的源"))}</button>` : ""}
      <button class="btn ghost" data-sf="close">${sfEsc(sfT("common.cancel", "收起"))}</button>
      <span class="cfg-status" id="sfStatus"></span>
    </div>
  </div>`;
}

function sfNetworkCardHTML(cfg) {
  const mirror = cfg.mirror_prefix || "";
  const known = SF_MIRRORS.some(m => m.value === mirror);
  const opts = SF_MIRRORS.map(m => `<option value="${sfEsc(m.value)}"${m.value === mirror ? " selected" : ""}>${sfEsc(m.label)}</option>`).join("")
    + (known ? "" : `<option value="${sfEsc(mirror)}" selected>${sfEsc(mirror)}</option>`);
  const test = SF_TEST_RESULT;
  let testHTML = "";
  if (SF_TESTING) {
    testHTML = `<span class="sf-test testing">${sfEsc(sfT("sf.testing", "测试中…"))}</span>`;
  } else if (test) {
    testHTML = test.ok
      ? `<span class="sf-test ok">${sfEsc(sfT("sf.test_ok", "连通正常"))} · ${Number(test.elapsed_ms) || 0}ms${test.latest_tag ? " · " + sfEsc(test.latest_tag) : ""}</span>`
      : `<span class="sf-test err">${sfEsc(test.error || (sfT("sf.test_fail", "连接失败") + " HTTP " + (test.status || "?")))}</span>`;
  }
  return `<div class="ws-cfg-card">
    <h4>${sfEsc(sfT("sf.net_title", "下载通道"))}</h4>
    <p class="ws-help">${sfEsc(sfT("sf.net_help", "服务端无法直连 GitHub 时，配置企业代理或加速镜像即可恢复更新。留空则沿用进程的 HTTP_PROXY 环境变量。"))}</p>
    ${SF_STATE.proxy_from_env ? `<p class="ws-help sf-env">${sfEsc(sfT("sf.env_proxy", "已检测到环境变量代理："))}<code>${sfEsc(SF_STATE.proxy_from_env)}</code></p>` : ""}
    <div class="field"><label>${sfEsc(sfT("sf.proxy", "代理地址"))}</label>
      <input id="sfProxy" class="mono" placeholder="http://user:pass@10.0.0.1:8080 或 socks5://10.0.0.1:1080" value="${sfEsc(cfg.proxy_url || "")}"></div>
    <div class="cfg-form-row">
      <div class="field"><label>${sfEsc(sfT("sf.mirror", "加速镜像"))}</label>
        <select id="sfMirror">${opts}</select></div>
      <div class="field"><label>${sfEsc(sfT("sf.timeout", "单次更新超时（秒）"))}</label>
        <input id="sfTimeout" type="number" min="120" max="7200" value="${sfEsc(cfg.timeout_sec || 1800)}"></div>
    </div>
    <label class="switch cfg-enable"><input type="checkbox" id="sfInsecure"${cfg.insecure_tls ? " checked" : ""}>
      <span>${sfEsc(sfT("sf.insecure", "跳过 TLS 证书校验（仅在使用解密型代理时勾选）"))}</span></label>
    <div class="sf-test-row">
      <button class="btn sm" data-sf="test"${SF_TESTING ? " disabled" : ""}>${sfEsc(sfT("sf.test", "测试连通性"))}</button>
      ${testHTML}
    </div>
  </div>`;
}

function sfAutoCardHTML(cfg) {
  return `<div class="ws-cfg-card">
    <h4>${sfEsc(sfT("sf.auto_title", "自动更新"))}</h4>
    <p class="ws-help">${sfEsc(sfT("sf.auto_help", "按周期在后台增量更新已启用的源。模板库体积较大，建议放在业务低峰期，并确保下载通道稳定。"))}</p>
    <label class="switch cfg-enable"><input type="checkbox" id="sfAuto"${cfg.auto_update ? " checked" : ""}>
      <span>${sfEsc(sfT("sf.auto_enable", "启用定期自动更新"))}</span></label>
    <div class="field"><label>${sfEsc(sfT("sf.interval", "更新间隔（小时）"))}</label>
      <input id="sfInterval" type="number" min="1" max="720" value="${sfEsc(cfg.interval_hours || 24)}"></div>
    <p class="ws-help">${sfEsc(sfT("sf.last_auto", "上次自动更新："))}${sfEsc(cfg.last_auto_run_sec ? sfAgo(cfg.last_auto_run_sec) : sfT("sf.never", "从未更新"))}</p>
  </div>`;
}

function sfSourceRowHTML(s, admin) {
  const stale = s.enabled && !s.updated_at;
  const statusHTML = s.error
    ? `<span class="sf-state err" title="${sfEsc(s.error)}">${sfEsc(s.error)}</span>`
    : s.updated_at
      ? `<span class="sf-state ok">${sfEsc(s.state_ref || "")} · ${sfEsc(sfAgo(s.updated_at))} · ${Number(s.files) || 0} ${sfEsc(sfT("sf.files", "个文件"))} · ${sfEsc(sfBytes(s.bytes))}</span>`
      : `<span class="sf-state">${sfEsc(sfT("sf.never", "从未更新"))}</span>`;
  return `<div class="sf-item${s.enabled ? " on" : ""}${stale ? " stale" : ""}">
    <label class="sf-item-pick">
      <input type="checkbox" data-sfsrc="${sfEsc(s.id)}"${s.enabled ? " checked" : ""}${admin ? "" : " disabled"}>
    </label>
    <div class="sf-item-main">
      <div class="sf-item-title">
        <b>${sfEsc(s.name)}</b>
        <span class="sf-kind sf-kind-${sfEsc(s.kind)}">${sfEsc(sfKindLabel(s.kind))}</span>
        <a class="sf-repo mono" href="https://github.com/${sfEsc(s.repo)}" target="_blank" rel="noopener noreferrer">${sfEsc(s.repo)}</a>
        ${s.license ? `<span class="sf-license">${sfEsc(s.license)}</span>` : ""}
      </div>
      <p class="sf-item-desc">${sfEsc(s.desc)}</p>
      ${statusHTML}
    </div>
    ${admin ? `<button class="btn sm ghost" data-sf="update-one" data-id="${sfEsc(s.id)}">${sfEsc(sfT("sf.update_one", "更新"))}</button>` : ""}
  </div>`;
}

function sfJobHTML(job) {
  if (!job) return "";
  const pct = job.total > 0 ? Math.round((job.done / job.total) * 100) : 0;
  const phase = {
    resolve: sfT("sf.phase_resolve", "解析版本"),
    download: sfT("sf.phase_download", "下载中"),
    extract: sfT("sf.phase_extract", "解压中"),
    publish: sfT("sf.phase_publish", "发布中"),
    done: sfT("sf.phase_done", "已完成"),
  }[job.phase] || job.phase || "";
  const failed = (job.results || []).filter(r => r.error).length;
  const failedHTML = failed
    ? ` · <span class="err">${failed} ${sfEsc(sfT("sf.job_failed", "个失败"))}</span>`
    : "";
  return `<div class="sf-job${job.running ? " running" : ""}">
    <div class="sf-job-head">
      <span class="sf-job-title">${sfEsc(job.running ? sfT("sf.job_running", "正在更新情报源") : sfT("sf.job_done", "上次更新结果"))}</span>
      <span class="sf-job-meta">${sfEsc(phase)}${job.current ? " · " + sfEsc(job.current) : ""} · ${Number(job.done) || 0}/${Number(job.total) || 0}${failedHTML}</span>
      ${job.running ? `<button class="btn sm ghost" data-sf="cancel">${sfEsc(sfT("sf.cancel", "取消"))}</button>` : ""}
    </div>
    <div class="sf-bar"><div class="sf-bar-fill" style="width:${pct}%"></div></div>
    ${job.error ? `<div class="sf-job-err">${sfEsc(job.error)}</div>` : ""}
    ${(job.log || []).length ? `<pre class="sf-log mono">${sfEsc((job.log || []).slice(-8).join("\n"))}</pre>` : ""}
  </div>`;
}

/* ---------- 交互 ---------- */

function sfCollectConfig() {
  const val = id => { const el = $(id); return el ? el.value : ""; };
  const chk = id => { const el = $(id); return !!(el && el.checked); };
  const sources = Array.from(document.querySelectorAll("[data-sfsrc]"))
    .filter(el => el.checked).map(el => el.dataset.sfsrc);
  return {
    sources,
    proxy_url: String(val("sfProxy") || "").trim(),
    mirror_prefix: String(val("sfMirror") || "").trim(),
    timeout_sec: parseInt(val("sfTimeout"), 10) || 1800,
    insecure_tls: chk("sfInsecure"),
    auto_update: chk("sfAuto"),
    interval_hours: parseInt(val("sfInterval"), 10) || 24,
  };
}

async function sfSave() {
  const st = $("sfStatus");
  if (st) st.textContent = sfT("common.saving", "保存中…");
  try {
    SF_STATE = await sfAPI(`${API}/security/feeds/config`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(sfCollectConfig()),
    });
    SF_LOAD_ERROR = "";
    if (typeof toast === "function") toast(sfT("sf.saved", "情报源配置已保存"), "ok");
    sfPaint();
  } catch (e) {
    if (st) st.textContent = "";
    if (typeof toast === "function") toast(String(e.message || e), "err");
  }
}

// sfTest probes with the *form* values, not the saved ones, so an operator can
// validate a proxy before committing it.
async function sfTest() {
  SF_TESTING = true; SF_TEST_RESULT = null;
  sfPaint();
  try {
    SF_TEST_RESULT = await sfAPI(`${API}/security/feeds/test`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(sfCollectConfig()),
    });
  } catch (e) {
    SF_TEST_RESULT = { ok: false, error: String(e.message || e) };
  }
  SF_TESTING = false;
  sfPaint();
}

async function sfUpdate(ids) {
  try {
    const body = ids && ids.length ? JSON.stringify({ sources: ids }) : "{}";
    await sfAPI(`${API}/security/feeds/update`, {
      method: "POST", headers: { "Content-Type": "application/json" }, body,
    });
    if (typeof toast === "function") toast(sfT("sf.job_started", "更新任务已启动，进度在面板中实时显示"), "ok");
    sfStartPoll();
  } catch (e) {
    if (typeof toast === "function") toast(String(e.message || e), "err");
  }
}

async function sfCancel() {
  try {
    await sfAPI(`${API}/security/feeds/cancel`, { method: "POST" });
    if (typeof toast === "function") toast(sfT("sf.cancelled", "已取消更新"), "ok");
  } catch (e) {
    if (typeof toast === "function") toast(String(e.message || e), "err");
  }
  sfStartPoll();
}

// Polling stops as soon as the job finishes so an idle dashboard makes no
// background requests. Signature skip avoids redundant full paints while progress stalls.
function sfStartPoll() {
  sfStopPoll();
  const tick = async () => {
    await sfLoad();
    const running = SF_STATE && SF_STATE.job && SF_STATE.job.running;
    if (SF_OPEN && document.querySelector("#view-web-security.active")) sfPaintIfChanged(false);
    if (!running) {
      sfStopPoll();
      // The template count in the engine bar only changes once a run finishes.
      try {
        const eng = await sfAPI(`${API}/security/web/engine?refresh=1`);
        const h = sfHost();
        if (h && typeof h.setEngine === "function") h.setEngine(eng);
      } catch (_) {}
      if (SF_OPEN) sfPaintIfChanged(true);
    }
  };
  tick();
  SF_POLL = setInterval(tick, 2500);
}

function sfStopPoll() {
  if (SF_POLL) { clearInterval(SF_POLL); SF_POLL = null; }
}

async function sfToggle() {
  SF_OPEN = !SF_OPEN;
  if (SF_OPEN) {
    const h = sfHost();
    if (h && typeof h.setShowCfg === "function") h.setShowCfg(false);
    SF_LOAD_ERROR = "";
    if (!sfPaint()) {
      SF_OPEN = false;
      return;
    }
    sfScrollIntoView();
    await sfLoad();
    if (SF_STATE && SF_STATE.job && SF_STATE.job.running) sfStartPoll();
  } else {
    sfStopPoll();
  }
  sfPaint();
  if (SF_OPEN) sfScrollIntoView();
}

function sfAction(act, el) {
  if (act === "close") { SF_OPEN = false; sfStopPoll(); return sfPaint(); }
  if (act === "retry") {
    return (async () => {
      await sfLoad();
      if (SF_STATE && SF_STATE.job && SF_STATE.job.running) sfStartPoll();
      sfPaint();
      sfScrollIntoView();
    })();
  }
  if (act === "save") return sfSave();
  if (act === "test") return sfTest();
  if (act === "cancel") return sfCancel();
  if (act === "update-all") return sfUpdate(null);
  if (act === "update-one") return sfUpdate([el && el.dataset.id].filter(Boolean));
}
