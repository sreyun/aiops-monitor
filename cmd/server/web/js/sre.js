/* ===================== 自动化运维：剧本编排 + 批量执行 ===================== */
let PB_HOSTS = []; // cached full host list for target selection
let PB_CATS = []; // cached unique categories

async function loadPlaybooks() {
  try {
    const [pbs, hosts] = await Promise.all([
      fetch(`${API}/playbooks`).then(r => r.json()),
      fetch(`${API}/hosts`).then(r => r.json())
    ]);
    PB_HOSTS = hosts || [];
    // Extract unique categories for target dropdown
    PB_CATS = [...new Set(PB_HOSTS.map(h => h.category || I18N.t("section.uncategorized")))].sort();
    // System types are hardcoded (linux/macos/windows) — do NOT extract from
    // h.platform (which is a version string like "Ubuntu 22.04"), use h.os
    // (runtime.GOOS: "linux"/"windows"/"darwin") for matching.
    LAST_PLAYBOOKS = pbs || [];
    renderPlaybooks(LAST_PLAYBOOKS);
  } catch (e) { console.warn("load playbooks:", e); }
}

function switchAutomationView(mode) {
  AUTOMATION_VIEW_MODE = mode;
  try { localStorage.setItem("aiops_pb_view", mode); } catch (e) {}
  loadPlaybooks(); // 重新拉取并渲染（renderPlaybooks 内按模式设 className + 同步按钮态）
}

function renderPlaybooks(pbs) {
  const list = $("playbookList"), empty = $("playbookEmpty");
  if (!list) return;
  if (PB_SEARCH) { const q = PB_SEARCH.toLowerCase(); pbs = (pbs || []).filter(p => ((p.name || "") + " " + (p.description || "") + " " + (p.id || "")).toLowerCase().includes(q)); }
  if (empty) empty.style.display = pbs.length === 0 ? "" : "none";
  // 视图模式：卡片(默认) / 列表——复用同一 .pb-card 结构，列表态仅由 CSS 重排为紧凑单行，
  // 从而不改动 data-pbact 委托对 .pb-card[data-id] 的依赖。
  list.className = AUTOMATION_VIEW_MODE === "list" ? "pb-listmode" : "";
  const vt = $("playbookViewToggle");
  if (vt) vt.querySelectorAll(".vt-btn").forEach(b => b.classList.toggle("active", b.dataset.view === AUTOMATION_VIEW_MODE));
  list.innerHTML = pbs.map(pb => {
    const stepCount = (pb.steps || []).length;
    const targets = [...new Set((pb.steps || []).map(s => s.target))];
    const sched = pb.schedule && pb.schedule.enabled;
    return `<div class="pb-card" data-id="${esc(pb.id)}">
      <div class="pb-card-top">
        <div class="pb-card-title">
          <strong>${esc(pb.name)}</strong>
          ${pb.description ? `<span class="pb-desc">${esc(pb.description)}</span>` : `<span class="pb-desc pb-desc-empty">暂无描述</span>`}
        </div>
        ${sched ? `<span class="pb-sched-badge" title="${I18N.t("playbook.sched_badge_title")}">⏱ ${esc(pbSchedLabel(pb.schedule))}</span>` : ""}
      </div>
      <div class="pb-card-foot">
        <div class="pb-pills">
          <span class="pb-pill">${stepCount} 步骤</span>
          <span class="pb-pill">${targets.length} 目标</span>
          <span class="pb-pill pb-pill-id mono">${esc(pb.id)}</span>
        </div>
        <div class="pb-actions">
          <button class="btn primary sm" data-pbact="exec">▶ ${I18N.t("ui.execute")}</button>
          <button class="btn sm" data-pbact="edit">${I18N.t("ui.edit")}</button>
          <button class="btn danger sm" data-pbact="del">${I18N.t("ui.delete")}</button>
        </div>
      </div>
    </div>`;
  }).join("");
}

function openPlaybookModal(pb) {
  $("playbookModalTitle").textContent = pb ? I18N.t("ui.edit_playbook") : I18N.t("ui.new_playbook");
  $("pbId").value = pb ? pb.id : "";
  $("pbName").value = pb ? pb.name : "";
  $("pbDesc").value = pb ? (pb.description || "") : "";
  const steps = pb ? pb.steps : [];
  renderPbSteps(steps.length > 0 ? steps : [{name:"",command:"",target:"all",timeout_sec:30,continue_on_error:false}]);
  // Populate the timed-trigger fields from the playbook's schedule (if any).
  const sc = (pb && pb.schedule) ? pb.schedule : null;
  $("pbSchedEnabled").checked = !!(sc && sc.enabled);
  $("pbSchedKind").value = (sc && sc.kind) || "interval";
  $("pbSchedInterval").value = (sc && sc.interval_min) || 60;
  $("pbSchedAt").value = (sc && sc.at) || "03:00";
  $("pbSchedWeekday").value = String((sc && typeof sc.weekday === "number") ? sc.weekday : 1);
  pbSchedRefresh();
  $("playbookMask").classList.add("show");
}

// Show/hide the schedule sub-fields based on the enable toggle + selected kind.
function pbSchedRefresh() {
  const on = $("pbSchedEnabled").checked;
  $("pbSchedFields").style.display = on ? "" : "none";
  const kind = $("pbSchedKind").value;
  $("pbSchedIntervalField").style.display = (kind === "interval") ? "" : "none";
  $("pbSchedAtField").style.display = (kind === "daily" || kind === "weekly") ? "" : "none";
  $("pbSchedWeekdayField").style.display = (kind === "weekly") ? "" : "none";
}

// Human-readable schedule summary for the playbook card badge.
function pbSchedLabel(sc) {
  if (!sc || !sc.enabled) return "";
  if (sc.kind === "interval") return `每 ${sc.interval_min} 分钟`;
  if (sc.kind === "daily") return `每天 ${sc.at}`;
  if (sc.kind === "weekly") { const wd = ["日","一","二","三","四","五","六"][sc.weekday] || ""; return `每周${wd} ${sc.at}`; }
  return "定时";
}

function renderPbSteps(steps) {
  const c = $("pbSteps");
  c.innerHTML = steps.map((s, i) => {
    const tgtOpts = buildTargetOptions(s.target);
    const a = s.args || {};
    const mod = s.module || "";
    const av = (k) => esc(a[k] || "");
    const optSel = (v, cur) => (v === (cur || "") ? "selected" : "");
    return `<div class="pb-step" data-idx="${i}">
      <div class="grid2">
        <div class="field"><label>${I18N.t("form.step_name")}</label><input type="text" class="pb-step-name" value="${esc(s.name||"")}" placeholder="${I18N.t('form.hint_step_name')}"></div>
        <div class="field"><label>${I18N.t("form.target")}</label><div class="select-wrap"><select class="pb-step-target" data-act-change="pb-target-preview">${tgtOpts}</select></div></div>
      </div>
      <div class="pb-target-preview" style="font-size:12px;color:var(--muted2);margin:-4px 0 4px"></div>
      <div class="field"><label>类型</label><div class="select-wrap"><select class="pb-step-module" data-act-change="pb-module-change">
        <option value="" ${optSel("",mod)}>Shell 命令</option>
        <option value="gather_facts" ${optSel("gather_facts",mod)}>采集主机信息 · gather_facts</option>
        <option value="service" ${optSel("service",mod)}>服务管理 · service</option>
        <option value="package" ${optSel("package",mod)}>软件包 · package</option>
        <option value="copy" ${optSel("copy",mod)}>写入文件 · copy</option>
      </select></div></div>

      <div class="pb-mod pb-mod-shell" style="display:none">
        <div class="field"><label>${I18N.t("form.command")}</label><textarea class="pb-step-cmd" rows="2" placeholder="${I18N.t('form.hint_command')}" spellcheck="false" style="resize:vertical;min-height:54px;line-height:1.5">${esc(s.command||"")}</textarea></div>
        <details class="pb-adv"${(s.command_win||s.command_mac)?" open":""}><summary style="cursor:pointer;font-size:12px;color:var(--muted2);margin:2px 0 6px">分系统命令（留空则统一用上面的命令）</summary>
          <div class="field"><label>Windows 覆盖命令</label><textarea class="pb-step-cmdwin" rows="2" spellcheck="false" style="resize:vertical;min-height:44px" placeholder="仅 Windows 主机执行此命令">${esc(s.command_win||"")}</textarea></div>
          <div class="field"><label>macOS 覆盖命令</label><textarea class="pb-step-cmdmac" rows="2" spellcheck="false" style="resize:vertical;min-height:44px" placeholder="仅 macOS 主机执行此命令">${esc(s.command_mac||"")}</textarea></div>
        </details>
      </div>

      <div class="pb-mod pb-mod-gather_facts" style="display:none">
        <div style="font-size:12px;color:var(--muted2);margin:2px 0 8px;line-height:1.6">采集主机名、IP、架构、CPU 数（跨系统一致，替代 <code>ip a</code> / <code>ipconfig</code>）。建议配合下方「保存输出到变量」在后续步骤引用。</div>
      </div>

      <div class="pb-mod pb-mod-service" style="display:none">
        <div class="grid2">
          <div class="field"><label>服务名</label><input type="text" class="pb-arg-service-name" value="${av('name')}" placeholder="nginx"></div>
          <div class="field"><label>目标状态</label><div class="select-wrap"><select class="pb-arg-service-state">
            <option value="started" ${optSel('started',a.state)}>启动 started</option>
            <option value="stopped" ${optSel('stopped',a.state)}>停止 stopped</option>
            <option value="restarted" ${optSel('restarted',a.state)}>重启 restarted</option>
            <option value="reloaded" ${optSel('reloaded',a.state)}>重载 reloaded</option>
          </select></div></div>
        </div>
        <div class="field"><label>开机自启</label><div class="select-wrap"><select class="pb-arg-service-enabled">
          <option value="" ${optSel('',a.enabled)}>不修改</option>
          <option value="true" ${optSel('true',a.enabled)}>启用</option>
          <option value="false" ${optSel('false',a.enabled)}>禁用</option>
        </select></div></div>
      </div>

      <div class="pb-mod pb-mod-package" style="display:none">
        <div class="grid2">
          <div class="field"><label>包名</label><input type="text" class="pb-arg-package-name" value="${av('name')}" placeholder="nginx"></div>
          <div class="field"><label>操作</label><div class="select-wrap"><select class="pb-arg-package-state">
            <option value="present" ${optSel('present',a.state)}>安装 present</option>
            <option value="absent" ${optSel('absent',a.state)}>卸载 absent</option>
            <option value="latest" ${optSel('latest',a.state)}>安装/升级到最新 latest</option>
          </select></div></div>
        </div>
        <div style="font-size:12px;color:var(--muted2);margin:2px 0 8px">自动探测系统包管理器（apt/dnf/yum/apk/zypper/pacman · brew · choco/winget）。</div>
      </div>

      <div class="pb-mod pb-mod-copy" style="display:none">
        <div class="grid2">
          <div class="field"><label>目标路径</label><input type="text" class="pb-arg-copy-dest" value="${av('dest')}" placeholder="/etc/app/config.yml"></div>
          <div class="field"><label>权限（八进制）</label><input type="text" class="pb-arg-copy-mode mono" value="${av('mode')}" placeholder="0644" style="width:110px"></div>
        </div>
        <div class="field"><label>文件内容</label><textarea class="pb-arg-copy-content" rows="4" spellcheck="false" style="resize:vertical;min-height:70px">${esc(a.content||"")}</textarea></div>
      </div>

      <details class="pb-adv"${(s.when||s.register)?" open":""}><summary style="cursor:pointer;font-size:12px;color:var(--muted2);margin:2px 0 6px">条件与变量（选填）</summary>
        <div class="grid2">
          <div class="field"><label>when 条件</label><input type="text" class="pb-step-when" value="${esc(s.when||"")}" placeholder="如 {{os}} == linux；结果空/false/0 则跳过本步"></div>
          <div class="field"><label>保存输出到变量</label><input type="text" class="pb-step-register" value="${esc(s.register||"")}" placeholder="变量名 → 后续步骤用 {{变量名}} 引用"></div>
        </div>
      </details>

      <div class="grid2">
        <div class="field"><label>${I18N.t("form.timeout")}</label><input type="text" class="pb-step-timeout mono" value="${s.timeout_sec||30}" style="width:80px"></div>
        <div class="field"><label>${I18N.t("form.continue_err")}</label><label class="switch"><input type="checkbox" class="pb-step-cont" ${s.continue_on_error?"checked":""}> 继续下一步</label></div>
      </div>
      <label class="switch" style="display:flex;margin:2px 0 10px"><input type="checkbox" class="pb-step-ignore" ${s.ignore_exit?"checked":""}> 忽略非零退出码（grep 无匹配、diff 有差异等也算成功）</label>
      <button class="btn danger sm pb-step-del" type="button">${I18N.t("ui.delete_step")}</button>
    </div>`;
  }).join("");
  c.querySelectorAll(".pb-step-del").forEach(btn => {
    btn.onclick = () => { btn.closest(".pb-step").remove(); };
  });
  // Initialize previews + module visibility
  c.querySelectorAll(".pb-step-target").forEach(sel => pbTargetPreview(sel));
  c.querySelectorAll(".pb-step-module").forEach(sel => pbModuleChange(sel));
}

// Show only the argument block matching the step's selected type (module).
function pbModuleChange(sel) {
  const step = sel.closest(".pb-step");
  if (!step) return;
  step.querySelectorAll(".pb-mod").forEach(m => { m.style.display = "none"; });
  const show = step.querySelector(".pb-mod-" + (sel.value === "" ? "shell" : sel.value));
  if (show) show.style.display = "";
}

// Build <option> list for target select: all / by category / by system / per host
function buildTargetOptions(selectedTarget) {
  const opts = [`<option value="all" ${selectedTarget==="all"?"selected":""}>${I18N.t("ui.all_hosts")}</option>`];
  // By category
  if (PB_CATS.length > 0) {
    opts.push(`<optgroup label="${I18N.t("section.by_category")}">`);
    PB_CATS.forEach(cat => {
      const val = `category:${cat}`;
      opts.push(`<option value="${esc(val)}" ${selectedTarget===val?"selected":""}>${esc(cat)}</option>`);
    });
    opts.push('</optgroup>');
  }
  // By system type — hardcoded to Linux/macOS/Windows (not dynamic from host
  // data, because h.platform is a version string, not an OS type).
  opts.push(`<optgroup label="${I18N.t("section.by_system")}">`);
  [{val:"linux",label:"Linux"},{val:"macos",label:"macOS"},{val:"windows",label:"Windows"}].forEach(s => {
    opts.push(`<option value="system:${s.val}" ${selectedTarget===`system:${s.val}`?"selected":""}>${s.label}</option>`);
  });
  opts.push('</optgroup>');
  // Per host
  if (PB_HOSTS.length > 0) {
    opts.push(`<optgroup label="${I18N.t("section.target_host")}">`);
    PB_HOSTS.forEach(h => {
      const val = `host:${h.id}`;
      opts.push(`<option value="${esc(val)}" ${selectedTarget===val?"selected":""}>${esc(h.hostname)}</option>`);
    });
    opts.push('</optgroup>');
  }
  return opts.join("");
}

// Preview matched host count when target changes
function pbTargetPreview(sel) {
  const step = sel.closest(".pb-step");
  if (!step) return;
  const preview = step.querySelector(".pb-target-preview");
  if (!preview) return;
  const target = sel.value;
  let count = 0;
  if (target === "all" || target === "") {
    count = PB_HOSTS.length;
  } else if (target.startsWith("category:")) {
    const cat = target.slice("category:".length);
    count = PB_HOSTS.filter(h => (h.category || I18N.t("section.uncategorized")) === cat).length;
  } else if (target.startsWith("system:")) {
    const sys = target.slice("system:".length);
    // Match by h.os (runtime.GOOS: "linux"/"windows"/"darwin"), not h.platform
    // (which is a version string). macOS hosts have h.os="darwin".
    count = PB_HOSTS.filter(h => {
      const os = (h.os || "").toLowerCase();
      return os === sys || (sys === "macos" && os === "darwin");
    }).length;
  } else if (target.startsWith("host:")) {
    count = 1;
  }
  preview.textContent = count > 0 ? `${I18N.t("ui.matched")} ${count} ${I18N.t("ui.hosts_matched")}` : I18N.t("empty.no_host_match2");
  preview.style.color = count > 0 ? "var(--ok, #31c46b)" : "var(--crit, #ff5b6e)";
}

function collectPlaybook() {
  const steps = [];
  document.querySelectorAll("#pbSteps .pb-step").forEach(el => {
    const mod = el.querySelector(".pb-step-module").value;
    const step = {
      name: el.querySelector(".pb-step-name").value.trim(),
      target: el.querySelector(".pb-step-target").value,
      timeout_sec: parseInt(el.querySelector(".pb-step-timeout").value) || 30,
      continue_on_error: el.querySelector(".pb-step-cont").checked,
      ignore_exit: el.querySelector(".pb-step-ignore").checked,
      when: el.querySelector(".pb-step-when").value.trim(),
      register: el.querySelector(".pb-step-register").value.trim()
    };
    if (mod) {
      step.module = mod;
      step.args = collectModuleArgs(el, mod);
    } else {
      step.command = el.querySelector(".pb-step-cmd").value.trim();
      step.command_win = el.querySelector(".pb-step-cmdwin").value.trim();
      step.command_mac = el.querySelector(".pb-step-cmdmac").value.trim();
    }
    steps.push(step);
  });
  let schedule = null;
  if ($("pbSchedEnabled").checked) {
    const kind = $("pbSchedKind").value;
    schedule = { enabled: true, kind };
    if (kind === "interval") schedule.interval_min = parseInt($("pbSchedInterval").value) || 0;
    if (kind === "daily" || kind === "weekly") schedule.at = $("pbSchedAt").value.trim();
    if (kind === "weekly") schedule.weekday = parseInt($("pbSchedWeekday").value) || 0;
  }
  return { id: $("pbId").value, name: $("pbName").value.trim(), description: $("pbDesc").value.trim(), steps, schedule };
}

// Gather module-specific arguments from a step's form into an args object.
function collectModuleArgs(el, mod) {
  const args = {};
  const g = (cls) => { const n = el.querySelector(cls); return n ? n.value.trim() : ""; };
  if (mod === "service") {
    args.name = g(".pb-arg-service-name");
    args.state = g(".pb-arg-service-state");
    const en = g(".pb-arg-service-enabled"); if (en) args.enabled = en;
  } else if (mod === "package") {
    args.name = g(".pb-arg-package-name");
    args.state = g(".pb-arg-package-state");
  } else if (mod === "copy") {
    args.dest = g(".pb-arg-copy-dest");
    const cont = el.querySelector(".pb-arg-copy-content");
    args.content = cont ? cont.value : ""; // preserve exact content (no trim)
    const mode = g(".pb-arg-copy-mode"); if (mode) args.mode = mode;
  }
  // gather_facts takes no args
  return args;
}

async function savePlaybook() {
  const pb = collectPlaybook();
  if (!pb.name) { toast(I18N.t("valid.fill_playbook_name"), "err"); return; }
  if (pb.steps.length === 0) { toast(I18N.t("valid.need_step"), "err"); return; }
  await withLoading("pbSaveBtn", async () => {
    try {
      const r = await fetch(`${API}/playbooks`, { method: "POST", headers: {"Content-Type":"application/json"}, body: JSON.stringify(pb) });
      const j = await r.json().catch(()=>({}));
      if (r.ok) { toast(I18N.t("toast.playbook_saved"), "ok"); $("playbookMask").classList.remove("show"); loadPlaybooks(); }
      else toast(j.error || I18N.t("toast.save_failed"), "err");
    } catch (e) { toast(I18N.t("toast.save_failed2") + e, "err"); }
  });
}

async function executePlaybook(id) {
  try {
    const r = await fetch(`${API}/playbooks/${encodeURIComponent(id)}/execute`, { method: "POST" });
    const j = await r.json().catch(()=>({}));
    if (r.ok) {
      toast(I18N.t("toast.playbook_started"), "ok");
      // Poll for result
      const execId = j.execution_id;
      pollExecution(execId, id);
    } else toast(j.error || I18N.t("toast.execute_failed"), "err");
  } catch (e) { toast(I18N.t("toast.execute_failed2") + e, "err"); }
}

async function pollExecution(execId, pbId) {
  $("execResultTitle").textContent = I18N.t("ui.running");
  $("execResultBody").innerHTML = `<div class="empty-line">${I18N.t("ui.executing")}</div>`;
  $("execResultMask").classList.add("show");
  for (let i = 0; i < 60; i++) {
    await new Promise(r => setTimeout(r, 2000));
    try {
      const exec = await fetch(`${API}/playbooks/executions/${execId}`).then(r => r.json());
      renderExecResult(exec);
      if (exec.status !== "running") break;
    } catch (e) {}
  }
}

function renderExecResult(exec) {
  window._lastExecResult = exec; // 供「AI 复盘」按钮取用
  $("execResultTitle").textContent = `${I18N.t("ui.execute")}${exec.status === "completed" ? I18N.t("ui.completed") : exec.status === "failed" ? I18N.t("ui.failed") : I18N.t("ui.running")}`;
  // 有任何主机未成功 → 显示「AI 复盘」按钮（执行中不显示）
  const rb = $("execRetroBtn");
  if (rb) {
    const done = exec.status !== "running";
    const hasFail = exec.status === "failed" || Object.values(exec.host_results || {}).some(r => r.status !== "success");
    rb.style.display = (done && hasFail) ? "" : "none";
  }
  const rows = Object.entries(exec.host_results || {}).map(([hid, r]) => {
    const statusCls = r.status === "success" ? "ok" : r.status === "failed" ? "crit" : "warn";
    const steps = (r.steps || []).map(s => `<div class="exec-step ${s.status}"><span class="exec-step-name">${esc(s.name)}</span><span class="exec-step-status">${translateStepStatus(s.status)}</span><pre class="exec-step-out">${esc(s.output||"")}</pre></div>`).join("");
    return `<div class="exec-row">
      <div class="exec-row-head"><strong>${esc(r.hostname)}</strong> <span class="badge ${statusCls}">${translateExecStatus(r.status)}</span></div>
      <div class="exec-steps">${steps}</div>
    </div>`;
  }).join("");
  $("execResultBody").innerHTML = `<div class="exec-meta">${I18N.t("exec.operator")}: ${esc(exec.operator)} · ${I18N.t("exec.start_time")}: ${fmtDateTime(exec.start_time)}${exec.end_time?" · "+I18N.t("exec.end_time")+": "+fmtDateTime(exec.end_time):""} · ${I18N.t("exec.status_label")}: ${translateExecStatus(exec.status)}</div>${rows}`;
}

async function loadExecHistory() {
  try {
    const list = await fetch(`${API}/playbooks/executions`).then(r => r.json());
    const rows = (list || []).map(e => {
      const success = Object.values(e.host_results || {}).filter(r => r.status === "success").length;
      const total = Object.keys(e.host_results || {}).length;
      return `<div class="exec-hist-row" data-exec-id="${e.id}">
        <strong>${esc(e.playbook_name)}</strong>
        <span class="badge ${e.status === "completed" ? "ok" : e.status === "failed" ? "crit" : "warn"}">${translateExecStatus(e.status)}</span>
        <span class="mono" style="color:var(--muted)">${success}/${total} ${I18N.t("exec.success_count")}</span>
        <span class="mono" style="color:var(--muted)">${fmtDateTime(e.start_time)}</span>
        <span class="mono" style="color:var(--muted)">${esc(e.operator)}</span>
      </div>`;
    }).join("");
    $("execHistBody").innerHTML = rows || `<div class="empty-line">${I18N.t("empty.no_executions")}</div>`;
    $("execHistBody").querySelectorAll("[data-exec-id]").forEach(el => {
      el.onclick = async () => {
        const exec = await fetch(`${API}/playbooks/executions/${el.dataset.execId}`).then(r => r.json());
        renderExecResult(exec);
        $("execHistMask").classList.remove("show");
        $("execResultMask").classList.add("show");
      };
    });
    $("execHistMask").classList.add("show");
  } catch (e) { toast(I18N.t("toast.load_history_failed") + e, "err"); }
}

// Playbook event listeners
safeAddEventListener("addPlaybookBtn", "click", () => openPlaybookModal(null));
safeAddEventListener("pbAddStep", "click", () => {
  const c = $("pbSteps");
  const existing = Array.from(c.querySelectorAll(".pb-step")).map(el => ({
    name: el.querySelector(".pb-step-name").value, command: el.querySelector(".pb-step-cmd").value,
    target: el.querySelector(".pb-step-target").value, timeout_sec: parseInt(el.querySelector(".pb-step-timeout").value)||30,
    continue_on_error: el.querySelector(".pb-step-cont").checked
  }));
  existing.push({name:"",command:"",target:"all",timeout_sec:30,continue_on_error:false});
  renderPbSteps(existing);
});

// 把编辑器中的剧本对象整理为可读文本，供 AI 预检
function playbookToText(pb) {
  let s = `剧本名称：${pb.name || "(未命名)"}\n描述：${pb.description || "(无)"}\n步骤数：${(pb.steps || []).length}\n`;
  (pb.steps || []).forEach((st, i) => {
    s += `\n步骤${i + 1} [${st.name || "未命名"}] 目标=${st.target} 超时=${st.timeout_sec}s 失败继续=${st.continue_on_error ? "是" : "否"} 忽略退出码=${st.ignore_exit ? "是" : "否"}`;
    if (st.when) s += ` 前置条件=${st.when}`;
    if (st.register) s += ` 存变量=${st.register}`;
    if (st.module) s += `\n  模块：${st.module} 参数：${JSON.stringify(st.args || {})}`;
    else {
      if (st.command) s += `\n  命令(Linux/通用)：${st.command}`;
      if (st.command_win) s += `\n  命令(Windows)：${st.command_win}`;
      if (st.command_mac) s += `\n  命令(macOS)：${st.command_mac}`;
    }
  });
  return s;
}
// 把执行结果整理为聚焦失败的复盘文本
function execResultToText(exec) {
  let s = `剧本：${exec.playbook_name || ""}\n整体状态：${exec.status}\n操作者：${exec.operator || ""}\n`;
  Object.values(exec.host_results || {}).forEach(r => {
    s += `\n主机 ${r.hostname}（${r.status}）：`;
    (r.steps || []).forEach(st => {
      const out = (st.output || "").slice(0, 600);
      s += `\n  - 步骤[${st.name}] ${st.status}` + (st.status !== "success" && out ? `\n    输出：${out}` : "");
    });
  });
  return s.slice(0, 8000);
}
// AI 剧本预检：执行前审查命令的破坏性/幂等性/跨平台/防护缺失，给红黄绿评级
safeAddEventListener("pbPrecheckBtn", "click", () => {
  const pb = collectPlaybook();
  if (!pb.steps || !pb.steps.length) { toast("请先添加至少一个步骤再预检", "err"); return; }
  openAIAssist({
    task: "playbook_precheck",
    title: "AI 剧本预检 · 执行前风险审查",
    mode: "analyze",
    context: playbookToText(pb)
  });
});
// AI 执行复盘：对失败的执行定位根因 + 修复/重跑建议 + 剧本改进
safeAddEventListener("execRetroBtn", "click", () => {
  const exec = window._lastExecResult;
  if (!exec) { toast("暂无执行结果可复盘", "err"); return; }
  openAIAssist({
    task: "execution_retro",
    title: "AI 执行复盘 · 失败根因分析",
    mode: "analyze",
    context: execResultToText(exec)
  });
});

// AI 辅助：根据自然语言生成整份剧本（名称+描述+步骤），一键回填编辑器
safeAddEventListener("pbAIGenBtn", "click", () => {
  openAIAssist({
    task: "playbook",
    title: "AI 生成运维剧本",
    mode: "generate",
    placeholder: "如：滚动重启所有 nginx 主机上的 nginx 服务，任一失败则停止",
    prefill: ($("pbDesc") && $("pbDesc").value.trim()) || ($("pbName") && $("pbName").value.trim()) || "",
    applyLabel: "回填到编辑器",
    applyTo: (text) => {
      try {
        const jsonText = extractFirstCodeBlock(text) || text;
        const pb = JSON.parse(jsonText);
        pb.id = ""; // 作为新剧本回填，保存时另建
        openPlaybookModal(pb);
        if (typeof toast === "function") toast("已生成，请检查步骤与命令后保存", "ok");
      } catch (e) {
        if (typeof toast === "function") toast("AI 输出不是合法剧本 JSON，请查看后手动填写", "err");
      }
    }
  });
});
safeAddEventListener("pbSaveBtn", "click", savePlaybook);
safeAddEventListener("pbSchedEnabled", "change", pbSchedRefresh);
safeAddEventListener("pbSchedKind", "change", pbSchedRefresh);
safeAddEventListener("pbHistoryBtn", "click", loadExecHistory);
safeAddEventListener("playbookList", "click", e => {
  const card = e.target.closest(".pb-card"); if (!card) return;
  const act = e.target.closest("[data-pbact]"); if (!act) return;
  const id = card.dataset.id;
  if (act.dataset.pbact === "exec") executePlaybook(id);
  else if (act.dataset.pbact === "edit") {
    fetch(`${API}/playbooks`).then(r=>r.json()).then(pbs => {
      const pb = pbs.find(p=>p.id===id); if (pb) openPlaybookModal(pb);
    });
  } else if (act.dataset.pbact === "del") {
    if (!confirm(I18N.t("valid.confirm_delete_playbook"))) return;
    fetch(`${API}/playbooks/${encodeURIComponent(id)}`, {method:"DELETE"}).then(()=>{toast(I18N.t("toast.deleted"),"ok");loadPlaybooks();});
  }
});

// ============ SRE 中枢：事件 / 自动修复 / SLO / 工单 ============
let SRE_TAB = "incidents";
let SRE_HOSTS = [], SRE_PLAYBOOKS = [], SRE_CHECKS = [], SRE_RULES = [], SRE_SLOS = [], SRE_TICKETS = [];
const SRE_ALERT_TYPES = ["cpu","memory","disk","diskio","iops","gpu","load","proc","conn","hardware","offline","check"];
const _sevCls = s => s==="critical"?"crit":s==="warning"?"warn":"info";
const _srcLabel = s => ({alert:"告警",slo:"SLO",manual:"手动"})[s]||esc(s);
const _incStatus = s => ({open:"进行中",acknowledged:"已确认",resolved:"已解决"})[s]||esc(s);
const _incStatusCls = s => s==="resolved"?"ok":s==="acknowledged"?"warn":"crit";
const _tlKind = k => ({created:"创建",fired:"触发",recovered:"恢复",acked:"确认",resolved:"解决",remediation:"自动修复",comment:"评论",escalated:"升级工单",note:"备注",ai_diagnosis:"🤖 AI 诊断",correlation:"🔗 关联分析",ai_analysis:"🤖 AI 分析"})[k]||k;
const _runStatus = s => ({running:"执行中",success:"成功",failed:"失败",pending_approval:"待审批",skipped_cooldown:"冷却跳过",skipped_ratelimit:"限频跳过",rejected:"已拒绝",no_playbook:"无剧本"})[s]||s;
const _runCls = s => s==="success"?"ok":(s==="failed"||s==="no_playbook")?"crit":s==="pending_approval"?"warn":s.indexOf("skipped")===0||s==="rejected"?"warn":"info";
const _prioCls = p => p==="p1"?"crit":p==="p2"?"warn":"info";
const _tkStatusCls = s => (s==="resolved"||s==="closed")?"ok":s==="in_progress"?"warn":"info";

async function loadSRE(){
  try {
    const [hosts, pbs] = await Promise.all([
      fetch(`${API}/hosts`).then(r=>r.json()),
      fetch(`${API}/playbooks`).then(r=>r.json())
    ]);
    SRE_HOSTS = hosts||[]; SRE_PLAYBOOKS = pbs||[];
  } catch(e){}
  try { SRE_CHECKS = (await fetch(`${API}/checks`).then(r=>r.json()))||[]; } catch(e){ SRE_CHECKS=[]; }
  loadSRETab(SRE_TAB); loadSREBadge();
}
async function loadSREBadge(){
  try {
    const o = await fetch(`${API}/sre/overview`).then(r=>r.json());
    const b = $("navSre"), n = (o.open_incidents||0)+(o.pending_remediations||0);
    if (b){ b.textContent=n; b.style.display=n>0?"":"none"; }
  } catch(e){}
}
function switchSRETab(tab){
  SRE_TAB = tab;
  document.querySelectorAll("#sreTabs .chip-btn").forEach(b=>b.classList.toggle("active", b.dataset.sretab===tab));
  document.querySelectorAll(".sre-panel").forEach(p=>p.classList.toggle("active", p.id==="srePanel-"+tab));
  loadSRETab(tab);
}
function loadSRETab(tab){
  if (tab==="incidents") loadIncidents();
  else if (tab==="remediation") loadRemediation();
  else if (tab==="slo") loadSLOs();
  else if (tab==="tickets") loadTickets();
  else if (tab==="ai") loadInspections();
}

/* ---- 事件 ---- */
async function loadIncidents(){
  try {
    const list = await fetch(`${API}/incidents`).then(r=>r.json());
    const el = $("incidentList");
    if (!list||!list.length){ el.innerHTML=`<div class="empty-line">暂无事件</div>`; return; }
    el.innerHTML = list.map(i=>`<div class="sre-row" data-incident="${i.id}">
      <span class="badge ${_sevCls(i.severity)}">${esc(i.severity)}</span>
      <div class="sre-row-main"><div class="sre-row-title">${esc(i.title)}</div>
        <div class="sre-row-sub">#${i.id} · ${_srcLabel(i.source)}${i.hostname?" · "+esc(i.hostname):""} · ${fmtDateTime(i.created_at)}</div></div>
      <span class="badge ${_incStatusCls(i.status)}">${_incStatus(i.status)}</span></div>`).join("");
    el.querySelectorAll("[data-incident]").forEach(r=>r.onclick=()=>openIncidentDetail(r.dataset.incident));
  } catch(e){ toast("加载失败: "+e,"err"); }
}
async function openIncidentDetail(id){
  try {
    const inc = await fetch(`${API}/incidents/${id}`).then(r=>r.json());
    $("incidentDetailTitle").textContent = `#${inc.id} ${inc.title}`;
    const tl = (inc.timeline||[]).slice().reverse().map(e=>`<div class="tl-item">
      <div class="tl-dot ${_sevCls(inc.severity)}"></div>
      <div class="tl-body"><div class="tl-head"><b>${_tlKind(e.kind)}</b> <span class="tl-time">${fmtDateTime(e.ts)}</span>${e.actor?` · <span class="tl-actor">${esc(e.actor)}</span>`:""}</div>${e.text?`<div class="tl-text">${esc(e.text)}</div>`:""}</div></div>`).join("");
    $("incidentDetailBody").innerHTML = `<div class="sre-meta">
      <span class="badge ${_sevCls(inc.severity)}">${esc(inc.severity)}</span>
      <span class="badge ${_incStatusCls(inc.status)}">${_incStatus(inc.status)}</span>
      <span class="mono" style="color:var(--muted)">${_srcLabel(inc.source)}${inc.hostname?" · "+esc(inc.hostname):""}</span>
      ${inc.ticket_id?`<span class="mono" style="color:var(--muted)">🎫 工单 #${inc.ticket_id}</span>`:""}</div>
      <div class="subhead">时间线</div><div class="timeline">${tl||`<div class="empty-line">—</div>`}</div>
      <div class="subhead" style="margin-top:16px">🤖 AI 诊断对话</div>
      <div id="incDiagnosisChat" class="ai-diagnosis-chat"></div>
      <div id="incDiagAttach" style="display:none;flex-wrap:wrap;gap:4px;padding:4px 0"></div>
      <div class="ai-diagnosis-input">
        <textarea id="incDiagInput" rows="2" placeholder="追问 AI 细节、反驳结论、要求进一步排查…"></textarea>
        <button class="btn sm" id="incDiagAttachBtn" title="上传图片或文件" style="padding:4px 8px">📎</button>
        <button class="btn primary" id="incDiagSendBtn">发送</button>
        <input type="file" id="incDiagFile" multiple hidden>
      </div>
      <label class="ai-term-toggle" id="incTermToggle" style="margin-top:4px;font-size:12px;color:var(--muted);cursor:pointer;display:flex;align-items:center;gap:4px;user-select:none"><input type="checkbox" id="incTermCheck"> 包含终端操作上下文（分段摘要）</label>`;
    window._curIncident = inc; // 供「转自动化规则」等操作取用完整事件（含时间线诊断）
    const acts=[];
    acts.push(`<button class="btn sm" data-iact="diagnose">🤖 AI 诊断</button>`);
    // 有 AI 诊断结论时，可一键把处置建议固化为「自动修复规则草稿」（停用态，需人工审核启用）
    if ((inc.timeline||[]).some(e=>e.kind==="ai_diagnosis" && e.text)) {
      acts.push(`<button class="btn sm ai-assist-btn" data-iact="draft-rule" title="把诊断建议转成自动修复规则草稿，人工审核后启用"><span class="ai-assist-btn-ic">🤖</span>转自动化规则</button>`);
    }
    if (inc.status!=="resolved"){ acts.push(`<button class="btn sm" data-iact="ack">确认</button>`); acts.push(`<button class="btn sm" data-iact="resolve">解决</button>`); }
    if (!inc.ticket_id) acts.push(`<button class="btn sm" data-iact="escalate">升级工单</button>`);
    acts.push(`<div style="flex:1"></div><input type="text" id="incCommentInput" placeholder="添加评论…" style="flex:2;min-width:120px"><button class="btn primary sm" data-iact="comment">发送</button>`);
    const foot=$("incidentDetailFoot"); foot.innerHTML=acts.join("");
    foot.querySelectorAll("[data-iact]").forEach(b=>b.onclick=()=>incidentAction(inc.id,b.dataset.iact));
    // Wire up diagnosis chat
    window._incDiagId = inc.id;
    window._incDiagHistory = [];
    window._INC_DIAG_ATTACHMENTS = [];
    loadDiagnosisChatHistory(inc.id);
    $("incDiagSendBtn").onclick = () => sendDiagnosisChatMsg();
    $("incDiagInput").onkeydown = e => { if (e.key==="Enter" && !e.shiftKey){ e.preventDefault(); sendDiagnosisChatMsg(); } };
    $("incDiagAttachBtn").onclick = () => { const f=$("incDiagFile"); if(f) f.click(); };
    $("incDiagFile").onchange = onDiagChatFiles;
    renderDiagAttachments();
    $("incidentDetailMask").classList.add("show");
  } catch(e){ toast("加载失败: "+e,"err"); }
}
async function incidentAction(id, act){
  try {
    if (act==="comment"){ const t=$("incCommentInput").value.trim(); if(!t)return;
      await fetch(`${API}/incidents/${id}/comment`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({text:t})}); }
    else if (act==="escalate"){
      const r=await fetch(`${API}/incidents/${id}/ticket`,{method:"POST"});
      const tk=await r.json().catch(()=>({}));
      toast(`已升级为工单 #${tk.id||"?"}`,"ok");
    }
    else if (act==="diagnose"){
      toast("AI 诊断中，请稍候…","ok");
      const r=await fetch(`${API}/incidents/${id}/diagnose`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({stream:true})});
      if(r.ok && r.headers.get("content-type")?.includes("event-stream")){
        // SSE 流式响应：等待完成后再刷新详情
        await readSSEStream(r,()=>{},()=>{},()=>{});
      }
    }
    else if (act==="draft-rule"){ draftRemediationFromIncident(window._curIncident); return; } // 不走末尾刷新
    else await fetch(`${API}/incidents/${id}/${act}`,{method:"POST"});
    openIncidentDetail(id); loadIncidents(); loadSREBadge();
  } catch(e){ toast("操作失败: "+e,"err"); }
}

// 闭环：把事件的 AI 诊断建议转成「自动修复规则草稿」。组织上下文（事件+最新诊断+可用剧本）后
// 调用统一 /ai/assist（task=remediation_rule），AI 产出 {playbook?,rule} JSON 供人工确认后落地。
function draftRemediationFromIncident(inc){
  if(!inc){ toast("请重新打开事件详情后再试","err"); return; }
  let diag="";
  const tl=inc.timeline||[];
  for(let i=tl.length-1;i>=0;i--){ if(tl[i].kind==="ai_diagnosis" && tl[i].text){ diag=tl[i].text; break; } }
  if(!diag){ toast("请先运行「🤖 AI 诊断」，有诊断结论后再转规则","err"); return; }
  const pbs=(SRE_PLAYBOOKS||[]).map(p=>`- id=${p.id} 名称=${p.name}${p.description?" 用途="+p.description:""}`).join("\n")||"（暂无已保存剧本，请新建）";
  const ctx=`事件：${inc.title}\n告警类型：${inc.type||"(未知)"}\n级别：${inc.severity}\n主机：${inc.hostname||"(未知)"}\n\nAI 诊断结论：\n${diag}\n\n【可用剧本】\n${pbs}`;
  openAIAssist({
    task:"remediation_rule",
    title:"AI 转自动化规则 · 草稿（需人工审核后启用）",
    mode:"analyze",
    context:ctx,
    applyLabel:"创建为草稿规则",
    applyTo:(text)=>applyRemediationDraft(text)
  });
}
// 落地草稿：新建剧本(若需要) + 建「停用」规则(require_approval 默认 true)，双保险，绝不自动生效。
async function applyRemediationDraft(text){
  let draft;
  try { draft=JSON.parse(extractFirstCodeBlock(text)||text); }
  catch(e){ toast("AI 输出不是合法 JSON，请到「自动修复」手动创建规则","err"); return; }
  try {
    let playbookId=(draft.existing_playbook_id||"").trim();
    if(!playbookId && draft.playbook){
      const pb=draft.playbook; pb.id="";
      const r=await fetch(`${API}/playbooks`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(pb)});
      const j=await r.json().catch(()=>({}));
      if(!r.ok||!j.id) throw new Error(j.error||"创建修复剧本失败");
      playbookId=j.id;
    }
    if(!playbookId) throw new Error("AI 未给出可用剧本");
    const rule=draft.rule||{};
    rule.id=""; rule.playbook_id=playbookId;
    rule.enabled=false; // 关键：草稿默认「停用」，绝不自动触发；人工审核后手动启用即生效
    if(rule.require_approval===undefined) rule.require_approval=true; // 双保险：即便启用也先排队人工审批
    const rr=await fetch(`${API}/remediation/rules`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(rule)});
    const rj=await rr.json().catch(()=>({}));
    if(!rr.ok) throw new Error(rj.error||"创建规则失败");
    toast("✅ 已创建『停用』草稿规则，请在「自动修复」审核命令与匹配条件后再启用","ok");
    const m=$("incidentDetailMask"); if(m) m.classList.remove("show");
    if(typeof switchSRETab==="function"){ switchSRETab("remediation"); }
    else if(typeof loadRemediation==="function"){ loadRemediation(); }
  } catch(e){ toast("落地草稿失败："+e,"err"); }
}
// ---- AI 诊断多轮对话 ----
// readSSEStream reads a Server-Sent Events stream from a fetch response and
// calls onDelta for each token chunk, onError for errors, onResult for result
// metadata, and onDone when complete. Returns the accumulated full text.
async function readSSEStream(resp,onDelta,onError,onDone,onResult,onMeta,onTool,onReasoning){
  const reader=resp.body.getReader();
  const decoder=new TextDecoder();
  let buf="";
  let fullText="";
  let fullReasoning="";
  try {
    while(true){
      const {done,value}=await reader.read();
      if(done) break;
      buf+=decoder.decode(value,{stream:true});
      // Split by double newlines to get SSE events
      const parts=buf.split("\n\n");
      buf=parts.pop()||"";
      for(const p of parts){
        const lines=p.split("\n");
        for(const line of lines){
          if(!line.startsWith("data: ")) continue;
          const data=line.slice(6);
          if(data==="[DONE]"){ if(onDone) onDone(fullText); return fullText; }
          try {
            const j=JSON.parse(data);
            if(j.error){ if(onError) onError(j.error); return fullText; }
            if(j.session_id!==undefined){ if(onMeta) onMeta(j); continue; }
            if(j.result){ if(onResult) onResult(j.result); continue; }
            if(j.tool){ if(onTool) onTool(j.tool); continue; } // 工具执行状态帧（run/ok/err）
            if(j.reasoning!==undefined){ fullReasoning+=j.reasoning; if(onReasoning) onReasoning(j.reasoning,fullReasoning); continue; } // 推理模型思维链增量
            if(j.delta){ fullText+=j.delta; if(onDelta) onDelta(j.delta,fullText); }
          } catch(e){ /* skip malformed chunks */ }
        }
      }
    }
  } finally { reader.releaseLock(); }
  if(onDone) onDone(fullText);
  return fullText;
}
// 渲染「🧠 思考过程」可折叠区块：默认折叠、暗色弱化，与正文答案视觉分离。
// streaming=true 时自动展开并显示光标，便于用户实时看到推理；完成后可手动收起。
function renderReasoningBlock(reasoning,streaming){
  if(!reasoning) return "";
  const cursor=streaming?'<span class="ai-stream-cursor">▍</span>':"";
  return `<details class="ai-reasoning"${streaming?" open":""}><summary class="ai-reasoning-sum">🧠 思考过程</summary>`
    +`<div class="ai-reasoning-body">${esc(reasoning)}${cursor}</div></details>`;
}
async function loadDiagnosisChatHistory(incidentId){
  const el=$("incDiagnosisChat"); if(!el) return;
  try {
    const r=await fetch(`${API}/incidents/${incidentId}/diagnose-chat`);
    const j=await r.json();
    window._incDiagHistory = (j.history||[]).map(m=>({role:m.role,content:m.content}));
  } catch(e){ window._incDiagHistory=[]; }
  renderDiagnosisChat();
}
function renderDiagnosisChat(){
  const el=$("incDiagnosisChat"); if(!el) return;
  const hist=window._incDiagHistory||[];
  if(!hist.length){ el.innerHTML=`<div class="empty-line" style="padding:12px">点击下方「🤖 AI 诊断」获取初步研判，然后在此追问细节。</div>`; return; }
  el.innerHTML=hist.map((m,i)=>{
    const cls=m.role==="user"?"me":m.role==="assistant"?"ai":"sys";
    // 思维链折叠区（推理模型）：流式中展开、完成后收起；无思维链时返回空串
    const rb=(m.role==="assistant")?renderReasoningBlock(m._reasoning,!!m._streaming):"";
    let body;
    if(m.role==="assistant" && m._streaming && m._loading){
      // 等待 AI 响应：显示动态加载提示（此时可能已在流式接收思维链）
      body=rb+`<div class="ai-thinking"><span class="ai-thinking-dots"><span></span><span></span><span></span></span> <span class="ai-thinking-text">${esc(m.content||"正在分析…")}</span></div>`;
    } else if(m.role==="assistant" && m._streaming){
      // 流式中：显示纯文本 + 闪烁光标，避免未完成 Markdown 导致渲染抖动
      body=rb+`<span class="ai-stream-text">${esc(m.content||"")}</span><span class="ai-stream-cursor">▍</span>`;
    } else if(m.role==="assistant" && m.content!=="思考中…" && !m.content.startsWith("❌")){
      body=rb+renderAIMarkdown(filterDisplayContent(m.content||""));
    } else {
      body=esc(m.content);
    }
    let fb="";
    if(m.role==="assistant" && m.content!=="思考中…" && !m._streaming){
      fb=`<div class="ai-chat-fb"><button class="btn-tiny" data-fb="helpful" data-idx="${i}" title="有用">👍</button><button class="btn-tiny" data-fb="unhelpful" data-idx="${i}" title="无用">👎</button></div>`;
    }
    return `<div class="ai-chat-msg ${cls}">${body}${fb}</div>`;
  }).join("");
  // Wire feedback buttons
  el.querySelectorAll("[data-fb]").forEach(b=>b.onclick=()=>sendDiagnosisFeedback(parseInt(b.dataset.idx),b.dataset.fb==="helpful"));
  el.querySelectorAll(".ai-chat-msg.ai").forEach(d=>addCopyTool(d,d.textContent));
  el.scrollTop=el.scrollHeight;
}
async function sendDiagnosisFeedback(idx,helpful){
  if(!window._incDiagId) return;
  try {
    await fetch(`${API}/incidents/${window._incDiagId}/diagnosis-feedback`,{
      method:"POST",headers:{"Content-Type":"application/json"},
      body:JSON.stringify({message_index:idx,helpful})
    });
    toast(helpful?"已标记为有用 👍":"已标记为无用 👎","ok");
  } catch(e){ /* ignore */ }
}
async function sendDiagnosisChatMsg(){
  const el=$("incDiagInput"); if(!el) return;
  const msg=el.value.trim();
  const atts=(window._INC_DIAG_ATTACHMENTS||[]).slice();
  if(!msg && !atts.length) return;
  const chat=$("incDiagnosisChat");
  // Show user message immediately (with attachment note)
  const imgN=atts.filter(a=>a.kind==="image").length, fileN=atts.filter(a=>a.kind==="file").length;
  const attNote=atts.length?` 📎 ${imgN?imgN+" 图 ":""}${fileN?fileN+" 文件":""}`:"";
  window._incDiagHistory.push({role:"user",content:msg||"（附件）"+attNote});
  renderDiagnosisChat();
  el.value=""; el.disabled=true; $("incDiagSendBtn").disabled=true;
  window._INC_DIAG_ATTACHMENTS=[]; renderDiagAttachments();
  // Add a placeholder for AI response with animated loading
  const aiMsg={role:"assistant",content:"",_streaming:true,_loading:true};
  window._incDiagHistory.push(aiMsg);
  renderDiagnosisChat();
  // 动画加载提示
  const loadingPhrases=["🔍 正在分析事件上下文…","📊 检索历史相似案例…","🤖 AI 正在思考…"];
  let loadingIdx=0;
  const loadingTimer=setInterval(()=>{
    loadingIdx=(loadingIdx+1)%loadingPhrases.length;
    if(aiMsg._loading){ aiMsg.content=loadingPhrases[loadingIdx]; renderDiagnosisChat(); }
  },2000);
  try {
    const cleanHist=window._incDiagHistory.filter(m=>!m._streaming&&m.content!=="思考中…").map(m=>({role:m.role,content:m.content}));
    const images=atts.filter(a=>a.kind==="image").map(a=>({mime:a.mime,data:a.data}));
    const files=atts.filter(a=>a.kind==="file").map(a=>({name:a.name,text:a.text}));
    const r=await fetch(`${API}/incidents/${window._incDiagId}/diagnose-chat`,{
      method:"POST",headers:{"Content-Type":"application/json"},
      body:JSON.stringify({message:msg,history:cleanHist,include_terminal:!!$("incTermCheck")?.checked,stream:true,images,files})
    });
    if(!r.ok){ throw new Error("HTTP "+r.status); }
    // SSE streaming
    let renderThrottle=null;
    const throttledRender=()=>{
      if(renderThrottle) return;
      renderThrottle=requestAnimationFrame(()=>{ renderThrottle=null; renderDiagnosisChat(); });
    };
    await readSSEStream(r,
      (delta,fullText)=>{
        if(aiMsg._loading){ clearInterval(loadingTimer); aiMsg._loading=false; }
        aiMsg.content=fullText;
        throttledRender();
      },
      (err)=>{
        clearInterval(loadingTimer); aiMsg._loading=false; aiMsg._streaming=false;
        aiMsg.content="❌ "+err;
        renderDiagnosisChat();
      },
      (fullText)=>{
        clearInterval(loadingTimer); aiMsg._loading=false;
        aiMsg._streaming=false;
        aiMsg.content=fullText||aiMsg.content||"（空回复）";
        if(renderThrottle){ cancelAnimationFrame(renderThrottle); renderThrottle=null; }
        renderDiagnosisChat();
      },
      null, // onResult
      null, // onMeta
      null, // onTool
      (rd,fullReasoning)=>{ // 思维链增量：累积到 aiMsg._reasoning 并实时渲染
        if(aiMsg._loading){ clearInterval(loadingTimer); aiMsg._loading=false; }
        aiMsg._reasoning=fullReasoning;
        throttledRender();
      }
    );
  } catch(e){
    clearInterval(loadingTimer);
    aiMsg._loading=false; aiMsg._streaming=false;
    aiMsg.content="❌ 网络错误: "+e;
    renderDiagnosisChat();
  }
  el.disabled=false; $("incDiagSendBtn").disabled=false; el.focus();
}
// Req1: 诊断对话附件渲染与文件处理（复用主对话的附件逻辑）
function renderDiagAttachments(){
  const box=$("incDiagAttach"); if(!box) return;
  const atts=window._INC_DIAG_ATTACHMENTS||[];
  if(!atts.length){ box.innerHTML=""; box.style.display="none"; return; }
  box.style.display="flex";
  box.innerHTML=atts.map((a,i)=>`<span class="ai-attach-chip">${a.kind==="image"?"🖼️":"📄"} ${esc(a.name)}<button data-datt="${i}" title="移除">✕</button></span>`).join("");
  box.querySelectorAll("[data-datt]").forEach(b=>b.onclick=()=>{ window._INC_DIAG_ATTACHMENTS.splice(parseInt(b.dataset.datt),1); renderDiagAttachments(); });
}
function onDiagChatFiles(ev){
  const files=Array.from((ev.target&&ev.target.files)||[]);
  if(!window._INC_DIAG_ATTACHMENTS) window._INC_DIAG_ATTACHMENTS=[];
  for(const f of files){
    if(f.type&&f.type.startsWith("image/")){
      if(window._INC_DIAG_ATTACHMENTS.filter(a=>a.kind==="image").length>=4){ if(typeof toast==="function") toast("最多 4 张图片","err"); continue; }
      if(f.size>4*1024*1024){ if(typeof toast==="function") toast(`图片 ${f.name} 超过 4MB`,"err"); continue; }
      const rd=new FileReader();
      rd.onload=()=>{ const s=String(rd.result||""); const c=s.indexOf(","); window._INC_DIAG_ATTACHMENTS.push({kind:"image",name:f.name,mime:f.type||"image/png",data:c>=0?s.slice(c+1):s}); renderDiagAttachments(); };
      rd.readAsDataURL(f);
    } else if(_AI_PARSE_EXT.includes(_extOf(f.name))){
      if(f.size>10*1024*1024){ if(typeof toast==="function") toast(`文件 ${f.name} 超过 10MB`,"err"); continue; }
      parseDiagFileAttachment(f);
    } else {
      if(f.size>1024*1024){ if(typeof toast==="function") toast(`文件 ${f.name} 超过 1MB`,"err"); continue; }
      const rd=new FileReader();
      rd.onload=()=>{ window._INC_DIAG_ATTACHMENTS.push({kind:"file",name:f.name,text:String(rd.result||"")}); renderDiagAttachments(); };
      rd.readAsText(f);
    }
  }
  if(ev.target) ev.target.value="";
}
function parseDiagFileAttachment(f){
  const rd=new FileReader();
  rd.onload=async()=>{
    const s=String(rd.result||""); const c=s.indexOf(","); const b64=c>=0?s.slice(c+1):s;
    const ph={kind:"file",name:f.name,text:"（解析中…）"};
    if(!window._INC_DIAG_ATTACHMENTS) window._INC_DIAG_ATTACHMENTS=[];
    window._INC_DIAG_ATTACHMENTS.push(ph); renderDiagAttachments();
    try{
      const r=await fetch(`${API}/hermes/parse`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({name:f.name,mime:f.type||"",data:b64})});
      const j=await r.json().catch(()=>({}));
      if(!r.ok||j.error){ window._INC_DIAG_ATTACHMENTS=window._INC_DIAG_ATTACHMENTS.filter(a=>a!==ph); if(typeof toast==="function") toast(`解析 ${f.name} 失败`,"err"); renderDiagAttachments(); return; }
      ph.text=j.text||""; renderDiagAttachments();
      if(typeof toast==="function") toast(`已解析 ${f.name}（${j.chars||0} 字）`,"ok");
    }catch(e){ window._INC_DIAG_ATTACHMENTS=window._INC_DIAG_ATTACHMENTS.filter(a=>a!==ph); if(typeof toast==="function") toast(`解析 ${f.name} 失败`,"err"); renderDiagAttachments(); }
  };
  rd.readAsDataURL(f);
}
function openNewIncident(){
  $("niTitle").value=""; $("niSeverity").value="warning";
  $("niHost").innerHTML=`<option value="">—</option>`+SRE_HOSTS.map(h=>`<option value="${esc(h.id)}">${esc(h.hostname)}</option>`).join("");
  $("newIncidentMask").classList.add("show");
}
async function saveNewIncident(){
  const title=$("niTitle").value.trim(); if(!title){ toast("请填写标题","err"); return; }
  await fetch(`${API}/incidents`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({title,severity:$("niSeverity").value,host_id:$("niHost").value})});
  $("newIncidentMask").classList.remove("show"); loadIncidents(); loadSREBadge(); toast("已保存","ok");
}

/* ---- 自动修复 ---- */
async function loadRemediation(){
  try {
    const [rules,runs] = await Promise.all([fetch(`${API}/remediation/rules`).then(r=>r.json()),fetch(`${API}/remediation/runs`).then(r=>r.json())]);
    SRE_RULES = rules||[]; renderRules(SRE_RULES); renderRuns(runs||[]);
  } catch(e){ toast("加载失败: "+e,"err"); }
}
function renderRules(rules){
  const el=$("remediationRuleList");
  if(!rules.length){ el.innerHTML=`<div class="empty-line">暂无修复规则</div>`; return; }
  el.innerHTML = rules.map(r=>{
    const pb=SRE_PLAYBOOKS.find(p=>p.id===r.playbook_id);
    const g=[]; if(r.require_approval)g.push("需审批"); if(r.cooldown_sec)g.push(`冷却${r.cooldown_sec}s`); if(r.max_per_hour)g.push(`≤${r.max_per_hour}/h`);
    const match=(r.match_types&&r.match_types.length?r.match_types.join("/"):"任意类型")+(r.min_level?` ≥${r.min_level}`:"");
    return `<div class="pb-card fwd-card ${r.enabled?"":"pb-off"}" data-rule="${esc(r.id)}">
      <div class="pb-card-top"><div class="pb-card-title"><strong>${esc(r.name)}</strong><span class="pb-desc">${esc(match)} → ${esc(pb?pb.name:r.playbook_id)}</span></div>
        <span class="fwd-status ${r.enabled?"on":"off"}">${r.enabled?"已启用":"已停用"}</span></div>
      <div class="pb-card-foot"><div class="pb-pills">${g.map(x=>`<span class="badge">${esc(x)}</span>`).join("")}</div>
        <div class="fwd-actions"><button class="btn sm" data-rract="edit">编辑</button><button class="btn danger sm" data-rract="del">删除</button></div></div></div>`;
  }).join("");
  el.querySelectorAll("[data-rule]").forEach(card=>card.querySelectorAll("[data-rract]").forEach(b=>b.onclick=e=>{ e.stopPropagation();
    const id=card.dataset.rule;
    if(b.dataset.rract==="edit") openRuleModal(SRE_RULES.find(x=>x.id===id));
    else if(confirm("确认删除该规则？")) fetch(`${API}/remediation/rules/${id}`,{method:"DELETE"}).then(()=>loadRemediation());
  }));
}
function renderRuns(runs){
  const el=$("remediationRunList");
  if(!runs.length){ el.innerHTML=`<div class="empty-line">暂无执行记录</div>`; return; }
  el.innerHTML = runs.map(r=>`<div class="sre-row">
    <span class="badge ${_runCls(r.status)}">${_runStatus(r.status)}</span>
    <div class="sre-row-main"><div class="sre-row-title">${esc(r.rule_name)} → ${esc(r.playbook_name||r.playbook_id)}</div>
      <div class="sre-row-sub">${esc(r.hostname)} · ${esc(r.alert_type)} · ${fmtDateTime(r.created_at)}${r.reason?" · "+esc(r.reason):""}</div></div>
    ${r.status==="pending_approval"?`<div class="fwd-actions"><button class="btn primary sm" data-run="${r.id}" data-runact="approve">批准</button><button class="btn danger sm" data-run="${r.id}" data-runact="reject">拒绝</button></div>`:""}</div>`).join("");
  el.querySelectorAll("[data-runact]").forEach(b=>b.onclick=async()=>{ await fetch(`${API}/remediation/runs/${b.dataset.run}/${b.dataset.runact}`,{method:"POST"}); loadRemediation(); loadSREBadge(); });
}
function openRuleModal(r){
  $("rrId").value=r?r.id:""; $("rrTitle").textContent=r?"编辑规则":"新建规则";
  $("rrName").value=r?r.name:""; $("rrEnabled").checked=r?r.enabled:true;
  $("rrLevel").value=r?(r.min_level||""):"critical";
  { // 主机分类改为下拉选择：从当前纳管主机的分类去重生成选项（含已保存但当前无主机的分类）
    const cur=r?(r.match_category||""):"";
    const _hs=((typeof LAST_HOSTS!=="undefined"&&LAST_HOSTS)||[]);
    // 包含所有主机分类 + 操作系统类型（去重）
    const cats=[...new Set([..._hs.map(h=>h.category).filter(Boolean), ..._hs.map(h=>h.os).filter(Boolean)])];
    if(cur&&!cats.includes(cur)) cats.push(cur);
    $("rrCategory").innerHTML='<option value="">全部分类</option>'+cats.map(c=>'<option value="'+esc(c)+'">'+esc(c)+'</option>').join('');
    $("rrCategory").value=cur;
  }
  $("rrCooldown").value=r?r.cooldown_sec:300; $("rrMaxPerHour").value=r?r.max_per_hour:6; $("rrApproval").checked=r?r.require_approval:false;
  $("rrPlaybook").innerHTML=SRE_PLAYBOOKS.map(p=>`<option value="${esc(p.id)}" ${r&&r.playbook_id===p.id?"selected":""}>${esc(p.name)}</option>`).join("")||`<option value="">（请先创建剧本）</option>`;
  const sel=new Set(r?(r.match_types||[]):[]);
  $("rrTypes").innerHTML=SRE_ALERT_TYPES.map(t=>`<label class="chip-check"><input type="checkbox" value="${esc(t)}" ${sel.has(t)?"checked":""}> ${esc(t)}</label>`).join("");
  $("remediationRuleMask").classList.add("show");
}
async function saveRule(){
  const types=[...document.querySelectorAll("#rrTypes input:checked")].map(c=>c.value);
  const body={id:$("rrId").value,name:$("rrName").value.trim(),enabled:$("rrEnabled").checked,match_types:types,min_level:$("rrLevel").value,match_category:$("rrCategory").value.trim(),playbook_id:$("rrPlaybook").value,require_approval:$("rrApproval").checked,cooldown_sec:parseInt($("rrCooldown").value)||0,max_per_hour:parseInt($("rrMaxPerHour").value)||0};
  const r=await fetch(`${API}/remediation/rules`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
  const j=await r.json().catch(()=>({}));
  if(r.ok){ $("remediationRuleMask").classList.remove("show"); loadRemediation(); toast("已保存","ok"); } else toast(j.error||"保存失败","err");
}

/* ---- SLO ---- */
async function loadSLOs(){
  try { SRE_SLOS = (await fetch(`${API}/slos`).then(r=>r.json()))||[]; renderSLOs(SRE_SLOS); }
  catch(e){ toast("加载失败: "+e,"err"); }
}
function renderSLOs(list){
  const el=$("sloList");
  if(!list.length){ el.innerHTML=`<div class="empty-line">暂无 SLO</div>`; return; }
  el.innerHTML=list.map(s=>{
    const bCls=s.error_budget<=0?"crit":s.error_budget<30?"warn":"ok";
    const src=s.source_type==="check"?"拨测 up 率":`${s.metric} ${s.comparator} ${s.threshold}`;
    return `<div class="pb-card fwd-card ${s.enabled?"":"pb-off"}" data-slo="${esc(s.id)}">
      <div class="pb-card-top"><div class="pb-card-title"><strong>${esc(s.name)}</strong><span class="pb-desc">${esc(src)} · 目标 ${s.target}% · ${s.window_days}d</span></div>
        <span class="badge ${s.breaching?"crit":"ok"}">SLI ${s.sli.toFixed(2)}%</span></div>
      <div class="slo-budget"><div class="slo-budget-bar"><div class="slo-budget-fill ${bCls}" style="width:${Math.max(0,Math.min(100,s.error_budget))}%"></div></div>
        <div class="slo-budget-txt">错误预算 ${s.error_budget.toFixed(0)}% · 燃尽 ${s.burn_rate.toFixed(2)}× · 达标 ${s.good_events}/${s.total_events}</div></div>
      <div class="pb-card-foot"><div class="pb-pills">${s.breaching?`<span class="badge crit">超标</span>`:`<span class="badge ok">健康</span>`}${s.enabled?"":`<span class="badge">停用</span>`}</div>
        <div class="fwd-actions"><button class="btn sm" data-sloact="edit">编辑</button><button class="btn danger sm" data-sloact="del">删除</button></div></div></div>`;
  }).join("");
  el.querySelectorAll("[data-slo]").forEach(card=>card.querySelectorAll("[data-sloact]").forEach(b=>b.onclick=e=>{ e.stopPropagation();
    const id=card.dataset.slo;
    if(b.dataset.sloact==="edit") openSloModal(SRE_SLOS.find(x=>x.id===id));
    else if(confirm("确认删除该 SLO？")) fetch(`${API}/slos/${id}`,{method:"DELETE"}).then(()=>loadSLOs());
  }));
}
function sloSourceChange(){
  const src=$("sloSource").value;
  $("sloCheckField").style.display=src==="check"?"":"none";
  $("sloMetricFields").style.display=src==="metric"?"":"none";
}
function openSloModal(s){
  $("sloId").value=s?s.id:""; $("sloModalTitle").textContent=s?"编辑 SLO":"新建 SLO";
  $("sloName").value=s?s.name:""; $("sloEnabled").checked=s?s.enabled:true; $("sloSource").value=s?s.source_type:"check";
  $("sloCheck").innerHTML=SRE_CHECKS.map(c=>`<option value="${esc(c.id)}" ${s&&s.check_id===c.id?"selected":""}>${esc(c.name)}</option>`).join("")||`<option value="">（请先创建拨测）</option>`;
  $("sloHost").innerHTML=SRE_HOSTS.map(h=>`<option value="${esc(h.id)}" ${s&&s.host_id===h.id?"selected":""}>${esc(h.hostname)}</option>`).join("");
  if(s){ $("sloMetric").value=s.metric||"cpu_percent"; $("sloComparator").value=s.comparator||"<"; $("sloThreshold").value=s.threshold||90; } else { $("sloComparator").value="<"; $("sloThreshold").value=90; }
  $("sloTarget").value=s?s.target:99.9; $("sloWindow").value=s?s.window_days:30;
  sloSourceChange(); $("sloMask").classList.add("show");
}
async function saveSlo(){
  const src=$("sloSource").value;
  const body={id:$("sloId").value,name:$("sloName").value.trim(),enabled:$("sloEnabled").checked,source_type:src,target:parseFloat($("sloTarget").value)||99,window_days:parseInt($("sloWindow").value)||30};
  if(src==="check") body.check_id=$("sloCheck").value;
  else { body.host_id=$("sloHost").value; body.metric=$("sloMetric").value; body.comparator=$("sloComparator").value; body.threshold=parseFloat($("sloThreshold").value)||0; }
  const r=await fetch(`${API}/slos`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
  const j=await r.json().catch(()=>({}));
  if(r.ok){ $("sloMask").classList.remove("show"); loadSLOs(); toast("已保存","ok"); } else toast(j.error||"保存失败","err");
}

/* ---- 工单 ---- */
async function loadTickets(){
  try { SRE_TICKETS=(await fetch(`${API}/tickets`).then(r=>r.json()))||[]; renderTickets(SRE_TICKETS); }
  catch(e){ toast("加载失败: "+e,"err"); }
}
function renderTickets(list){
  const el=$("ticketList");
  if(!list.length){ el.innerHTML=`<div class="empty-line">暂无工单</div>`; return; }
  el.innerHTML=list.map(t=>`<div class="sre-row" data-ticket="${t.id}">
    <span class="badge ${_prioCls(t.priority)}">${esc((t.priority||"p3").toUpperCase())}</span>
    <div class="sre-row-main"><div class="sre-row-title">${esc(t.title)}</div>
      <div class="sre-row-sub">#${t.id}${t.assignee?" · @"+esc(t.assignee):""}${t.incident_id?" · 🔗事件#"+t.incident_id:""} · ${fmtDateTime(t.updated_at)}</div></div>
    <span class="badge ${_tkStatusCls(t.status)}">${esc(t.status)}</span></div>`).join("");
  el.querySelectorAll("[data-ticket]").forEach(row=>row.onclick=()=>openTicketModal(SRE_TICKETS.find(x=>x.id==row.dataset.ticket)));
}
function openTicketModal(t){
  $("ticketId").value=t?t.id:""; $("ticketModalTitle").textContent=t?`#${t.id} ${t.title}`:"新建工单";
  $("tkTitle").value=t?t.title:""; $("tkPriority").value=t?t.priority:"p3"; $("tkStatus").value=t?t.status:"open";
  $("tkAssignee").value=t?(t.assignee||""):""; $("tkDesc").value=t?(t.description||""):"";
  // Show linked incident info if present
  const incInfo=$("tkIncidentInfo");
  if(t && t.incident){
    const inc=t.incident;
    incInfo.innerHTML=`<div class="hint" style="margin-bottom:8px">🔗 关联事件：<a href="#" onclick="openIncidentDetail(${inc.id});return false" style="font-weight:600">#${inc.id} ${esc(inc.title)}</a> · <span class="badge ${_sevCls(inc.severity)}">${esc(inc.severity)}</span> · ${esc(inc.hostname||"")} · ${fmtDateTime(inc.created_at)}</div>`;
    incInfo.style.display="";
  } else if(t && t.incident_id){
    incInfo.innerHTML=`<div class="hint" style="margin-bottom:8px">🔗 关联事件：<a href="#" onclick="openIncidentDetail(${t.incident_id});return false" style="font-weight:600">#${t.incident_id}</a></div>`;
    incInfo.style.display="";
  } else { incInfo.style.display="none"; }
  const cm=$("tkComments"),cf=$("tkCommentField");
  if(t){ cm.innerHTML=`<div class="subhead">评论</div>`+((t.comments||[]).map(c=>`<div class="tk-comment"><span class="tk-c-author">${esc(c.author)}</span> <span class="tk-c-time">${fmtDateTime(c.ts)}</span><div>${esc(c.text)}</div></div>`).join("")||`<div class="empty-line">—</div>`); cf.style.display=""; }
  else { cm.innerHTML=""; cf.style.display="none"; }
  $("ticketMask").classList.add("show");
}
async function saveTicket(){
  const id=$("ticketId").value;
  const body={title:$("tkTitle").value.trim(),priority:$("tkPriority").value,status:$("tkStatus").value,assignee:$("tkAssignee").value.trim(),description:$("tkDesc").value.trim()};
  if(!body.title){ toast("请填写标题","err"); return; }
  const r=await fetch(id?`${API}/tickets/${id}`:`${API}/tickets`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
  const j=await r.json().catch(()=>({}));
  if(r.ok){ $("ticketMask").classList.remove("show"); loadTickets(); loadSREBadge(); toast("已保存","ok"); } else toast(j.error||"保存失败","err");
}
async function addTicketComment(){
  const id=$("ticketId").value,t=$("tkCommentInput").value.trim(); if(!id||!t)return;
  await fetch(`${API}/tickets/${id}/comment`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({text:t})});
  $("tkCommentInput").value=""; const tk=await fetch(`${API}/tickets/${id}`).then(r=>r.json()); openTicketModal(tk); loadTickets();
}

document.querySelectorAll("#sreTabs .chip-btn").forEach(b=>b.addEventListener("click",()=>switchSRETab(b.dataset.sretab)));
safeAddEventListener("newIncidentBtn","click",openNewIncident);
safeAddEventListener("niSaveBtn","click",saveNewIncident);
safeAddEventListener("newRemediationBtn","click",()=>openRuleModal(null));
safeAddEventListener("rrSaveBtn","click",saveRule);
safeAddEventListener("newSloBtn","click",()=>openSloModal(null));
safeAddEventListener("sloSaveBtn","click",saveSlo);
safeAddEventListener("sloSource","change",sloSourceChange);
safeAddEventListener("newTicketBtn","click",()=>openTicketModal(null));
safeAddEventListener("tkSaveBtn","click",saveTicket);
safeAddEventListener("tkCommentBtn","click",addTicketComment);

/* ---- 日志检索 ---- */
const _logLvlCls = l => l==="error"?"crit":l==="warn"?"warn":"info";
// 日志检索分页状态：与概览「操作日志」的 LOG_PAGE/LOG_PAGE_SIZE（core.js）完全独立，
// 独立命名 + 独立 #logsPager 元素 + 独立 renderLogsPager，避免两个日志视图互相干扰。
let LOGS_PAGE = 1, LOGS_PAGE_SIZE = 50, LOGS_TOTAL = 0, LOGS_PAGES = 1;
let LAST_LOG_STATS = null; // 缓存上次搜索的统计数据

async function loadLogs(){
  try { if (!SRE_HOSTS.length) SRE_HOSTS=(await fetch(`${API}/hosts`).then(r=>r.json()))||[]; } catch(e){}
  const hs=$("logHost");
  if (hs && hs.options.length<=1) hs.innerHTML=`<option value="">全部主机</option>`+SRE_HOSTS.map(h=>`<option value="${esc(h.id)}">${esc(h.hostname)}</option>`).join("");
  // 日志来源下拉：本地聚合 + 已接入且启用的 Loki 数据源
  const srcSel=$("logSource");
  if (srcSel) {
    const cur=srcSel.value;
    try {
      const ds=await fetch(`${API}/datasources`).then(r=>r.json());
      const loki=(Array.isArray(ds)?ds:[]).filter(d=>d.type==="loki" && d.enabled!==false);
      srcSel.innerHTML=`<option value="">本地聚合</option>`+loki.map(d=>`<option value="${esc(d.id)}">${esc(d.name)}（Loki）</option>`).join("");
      if (cur && loki.some(d=>d.id===cur)) srcSel.value=cur;
    } catch(e){}
    onLogSourceChange();
  }
  searchLogs();
}

// 切换日志来源：Loki 模式下隐藏主机/级别筛选（Loki 用自己的标签选择器），显示 Job 筛选
// 关键字框改为 LogQL 输入
function onLogSourceChange(){
  const loki=!!($("logSource") && $("logSource").value);
  const hw=$("logHostWrap"), lw=$("logLevelWrap"), kw=$("logKeyword");
  const jw=$("logJobWrap"), js=$("logJob");
  if (hw) hw.style.display=loki?"none":"";
  if (lw) lw.style.display=loki?"none":"";
  if (kw) {
    if (loki) { kw.placeholder='LogQL，如 {job="nginx"} |= "error"'; kw.style.width="360px"; }
    else {
      // I18N.t 在缺键时返回键名本身（真值），不能用 || 兜底，否则占位符会显示 "logs.keyword_ph"
      const ph=I18N.t("logs.keyword_ph");
      kw.placeholder=(ph && ph!=="logs.keyword_ph")?ph:"关键字…";
      kw.style.width="190px";
    }
  }
  // Job 筛选：仅 Loki 模式显示，切换时自动加载 job 列表并更新关键字框
  if (jw) {
    if (loki) {
      jw.style.display="";
      if (js) { js.value=""; loadLogJobs($("logSource").value); }
      onLogJobChange();
    } else {
      jw.style.display="none";
    }
  }
  const el=$("logResults"); if (el) el.innerHTML="";
  const sp=$("logStatsPanel"); if (sp) sp.style.display="none";
  const pg=$("logsPager"); if (pg) pg.innerHTML="";
}

// 从 Loki 数据源加载 job 标签值列表
async function loadLogJobs(dsId){
  const js=$("logJob");
  if (!js || !dsId) return;
  const cur=js.value;
  js.innerHTML='<option value="">全部 job</option><option value="">加载中…</option>';
  try {
    const resp=await fetch(`${API}/datasources/${encodeURIComponent(dsId)}/labels?label=job`).then(r=>r.json());
    const labels=(resp.ok && Array.isArray(resp.labels))?resp.labels:[];
    js.innerHTML='<option value="">全部 job</option>'+labels.map(v=>`<option value="${esc(v)}">${esc(v)}</option>`).join("");
    if (cur && labels.includes(cur)) js.value=cur;
  } catch(e) {
    js.innerHTML='<option value="">全部 job</option><option value="">加载失败，请手动输入</option>';
  }
}

// Job 筛选变更：自动更新 LogQL 关键字框中的 job 选择器
function onLogJobChange(){
  const js=$("logJob"), kw=$("logKeyword");
  if (!js || !kw) return;
  const job=js.value;
  if (job) {
    // 选中具体 job：更新关键字框为 {job="xxx"}
    kw.value=`{job="${job}"}`;
  } else {
    // 全部 job：匹配所有含 job 标签的日志流
    kw.value='{job=~"(.+)"}';
  }
}

// Loki 检索：把关键字框内容当 LogQL，经数据源查询接口直查，渲染成日志行
async function searchLokiLogs(dsId){
  const q=$("logKeyword").value.trim();
  const since=$("logSince").value;
  const el=$("logResults");
  if (!q) { if (el) el.innerHTML=`<div class="empty-line">请输入 LogQL，如 {job="nginx"} |= "error"</div>`; return; }
  if (el) el.innerHTML=`<div class="empty-line">检索中…</div>`;
  const sp=$("logStatsPanel"); if (sp) sp.style.display="none";
  const pg=$("logsPager"); if (pg) pg.innerHTML="";
  try {
    const body={ query:q, limit:300, since_min:(since && since!=="0")?parseInt(since):720 };
    const resp=await fetch(`${API}/datasources/${encodeURIComponent(dsId)}/query`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)}).then(r=>r.json());
    if (!resp.ok) { if (el) el.innerHTML=`<div class="empty-line">检索失败: ${esc(resp.error||"未知错误")}</div>`; return; }
    const lines=(resp.result||"").split("\n").filter(x=>x.trim());
    if (!lines.length || (lines.length===1 && lines[0].startsWith("（"))) { if (el) el.innerHTML=`<div class="empty-line">${esc(lines[0]||"无匹配日志")}</div>`; return; }
    el.innerHTML=lines.map(line=>{
      const m=line.match(/^(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d)\s+([\s\S]*)$/);
      const ts=m?m[1]:"", msg=m?m[2]:line;
      const lvl=/\b(error|err|fatal|panic|exception)\b/i.test(msg)?"error":/\b(warn|warning)\b/i.test(msg)?"warn":"info";
      return `<div class="log-line ${_logLvlCls(lvl)}">
        <span class="log-ts mono">${esc(ts)}</span>
        <span class="log-lvl ${_logLvlCls(lvl)}">${esc(lvl)}</span>
        <span class="log-msg">${esc(msg)}</span>
      </div>`;
    }).join("");
  } catch(e){ if (el) el.innerHTML=`<div class="empty-line">检索失败: ${esc(e)}</div>`; }
}

async function searchLogs(page){
  // Loki 数据源模式：走 LogQL 直查，不用本地聚合的分页/筛选
  const srcSel=$("logSource");
  if (srcSel && srcSel.value) { return searchLokiLogs(srcSel.value); }
  if (page !== undefined) { LOGS_PAGE = page; } else { LOGS_PAGE = 1; }
  const host=$("logHost").value,level=$("logLevel").value,since=$("logSince").value,kw=$("logKeyword").value.trim();
  const qs=new URLSearchParams();
  if(host)qs.set("host",host); if(level)qs.set("level",level);
  if(since&&since!=="0")qs.set("since_min",since); if(kw)qs.set("q",kw);
  qs.set("page",String(LOGS_PAGE)); qs.set("page_size",String(LOGS_PAGE_SIZE));
  try {
    const resp=await fetch(`${API}/logs?${qs}`).then(r=>r.json());
    const items=resp.items||[]; LOGS_TOTAL=resp.total||0; LOGS_PAGES=resp.pages||1;
    LAST_LOG_STATS = resp.stats || null;

    // 渲染统计面板
    renderLogStats(resp.stats, resp.total);

    // 渲染日志列表
    const el=$("logResults");
    if(!items.length){ el.innerHTML=`<div class="empty-line">无匹配日志（被控端需以 --log-paths 指定采集文件）</div>`; renderLogsPager(); return; }
    el.innerHTML=items.map(l=>`<div class="log-line ${_logLvlCls(l.level)}">
      <span class="log-ts mono">${fmtDateTime(l.ts)}</span>
      <span class="log-lvl ${_logLvlCls(l.level)}">${esc(l.level)}</span>
      <span class="log-host">${esc(l.hostname)}</span>
      <span class="log-msg">${esc(l.message)}</span>
      ${(l.level==="error"||l.level==="warn")?`<button class="log-diag-btn" data-log='${esc(JSON.stringify({ts:l.ts,hostname:l.hostname,host_id:l.host_id||"",level:l.level,message:l.message}))}' title="提交诊断">🔍</button>`:""}
    </div>`).join("");

    // 绑定单条日志诊断按钮
    el.querySelectorAll(".log-diag-btn").forEach(b=>{ b.onclick=function(e){ e.stopPropagation(); const d=JSON.parse(this.dataset.log); diagnoseLogLine(d); }; });

    // 渲染分页控件
    renderLogsPager();
  } catch(e){ toast("检索失败: "+e,"err"); }
}

// 渲染日志统计面板
function renderLogStats(stats, total){
  const panel=$("logStatsPanel");
  if(!panel) return;
  if(!stats || !total){
    // 空态也保留看板结构，避免用户以为功能缺失；并提示数据来源
    // 注意：.log-stats 默认 display:none，须显式设为可见值（""会回落到 CSS 的 none）
    panel.style.display="block";
    panel.innerHTML=`<div class="log-stats-bar"><div class="log-stats-left"><span class="log-stat-total">共 <strong>0</strong> 条</span><span class="log-stat-empty">暂无匹配日志——被控端需在安装时以 --log-paths 指定采集文件；或放宽上方筛选条件后重试</span></div></div>`;
    return;
  }
  panel.style.display="block"; // 显式可见（.log-stats 默认 display:none，""会回落到 none）
  const byLvl=stats.by_level||{};
  const topHosts=stats.top_hosts||[];
  const timeDist=stats.time_distribution||{};

  // 按级别统计
  let levelHTML="";
  ["error","warn","info","debug"].forEach(lv=>{
    const cnt=byLvl[lv]||0;
    if(cnt>0 || lv==="error" || lv==="warn"){
      levelHTML+=`<span class="log-stat-chip ${_logLvlCls(lv)}">${lv}: <strong>${cnt}</strong></span>`;
    }
  });

  // 按主机 Top 5 — 横向柱状图可视化
  let hostHTML="";
  if(topHosts.length){
    const maxCount=topHosts[0].count||1;
    const barColors=['#4c8dff','#06b6d4','#8b5cf6','#22c55e','#f59e0b'];
    hostHTML='<div class="log-stat-row"><span class="log-stat-label">Top 主机：</span><div class="log-top-host-bars">';
    topHosts.forEach((h,i)=>{
      const pct=Math.round((h.count/maxCount)*100);
      const color=barColors[i%barColors.length];
      hostHTML+=`<div class="log-top-host-item" data-host="${esc(h.hostname)}" title="${esc(h.hostname)}：${h.count} 条日志">
        <span class="log-top-host-name">${esc(h.hostname)}</span>
        <div class="log-top-host-track"><div class="log-top-host-fill" style="width:${pct}%;background:${color}"></div></div>
        <span class="log-top-host-count" style="color:${color}">${h.count}</span>
      </div>`;
    });
    hostHTML+='</div></div>';
  }

  // 时间分布
  const h1=timeDist["1h"]||0, h6=timeDist["6h"]||0, h24=timeDist["24h"]||0;
  const timeHTML=`<span class="log-stat-chip time">近1h: <strong>${h1}</strong></span><span class="log-stat-chip time">近6h: <strong>${h6}</strong></span><span class="log-stat-chip time">近24h: <strong>${h24}</strong></span>`;

  // 一键诊断按钮（error > 10 条且 since_min <= 30）
  const errCount=byLvl["error"]||0;
  const sinceVal=$("logSince").value;
  const showDiag=errCount>=10 && (sinceVal==="15"||sinceVal==="30"||sinceVal==="60"||!sinceVal||sinceVal==="0");
  const diagBtn=showDiag ? `<button class="btn warn sm" id="logDiagBtn" style="margin-left:auto">⚡ 一键诊断（${errCount} 条错误）</button>` : "";

  panel.innerHTML=`<div class="log-stats-bar">
    <div class="log-stats-left">
      <span class="log-stat-total">共 <strong>${total}</strong> 条</span>
      ${levelHTML}
    </div>
    ${diagBtn}
  </div>
  ${hostHTML}
  <div class="log-stat-row"><span class="log-stat-label">时间分布：</span>${timeHTML}</div>`;

  // 绑定 Top 主机点击筛选
  panel.querySelectorAll(".log-top-host-item").forEach(item=>{
    item.onclick=()=>{
      const hostSel=$("logHost");
      if(!hostSel) return;
      const hn=item.dataset.host;
      for(let i=0;i<hostSel.options.length;i++){
        if(hostSel.options[i].textContent===hn){ hostSel.value=hostSel.options[i].value; break; }
      }
      searchLogs(1);
    };
  });

  // 绑定一键诊断
  const diagBtnEl=$("logDiagBtn");
  if(diagBtnEl){
    diagBtnEl.onclick=()=>{
      const host=$("logHost").value, hostname=$("logHost").selectedOptions[0]?.textContent||"";
      const since=$("logSince").value;
      diagnoseBulkLogs(host, hostname, parseInt(since)||60);
    };
  }
}

// 渲染日志分页控件
function renderLogsPager(){
  const pager=$("logsPager");
  if(!pager) return;
  if(LOGS_TOTAL===0){ pager.innerHTML=`<span class="pinfo">共 0 条</span>`; return; }
  if(LOGS_PAGES<=1){ pager.innerHTML=`<span class="pinfo">共 ${LOGS_TOTAL} 条</span>`; return; }
  let btns=`<button ${LOGS_PAGE===1?"disabled":""} data-lpg="prev">‹</button>`;
  for(let i=1;i<=LOGS_PAGES;i++){
    if(i===1||i===LOGS_PAGES||Math.abs(i-LOGS_PAGE)<=1){
      btns+=`<button class="${i===LOGS_PAGE?"active":""}" data-lpg="${i}">${i}</button>`;
    }else if(Math.abs(i-LOGS_PAGE)===2){
      btns+=`<span class="pinfo">…</span>`;
    }
  }
  btns+=`<button ${LOGS_PAGE===LOGS_PAGES?"disabled":""} data-lpg="next">›</button>`;
  btns+=`<span class="pinfo">共 ${LOGS_TOTAL} 条 · ${LOGS_PAGE}/${LOGS_PAGES} 页</span>`;
  pager.innerHTML=btns;

  // 绑定分页按钮事件
  pager.querySelectorAll("[data-lpg]").forEach(b=>{
    b.onclick=()=>{
      const v=b.dataset.lpg;
      if(v==="prev"){ if(LOGS_PAGE>1) searchLogs(LOGS_PAGE-1); }
      else if(v==="next"){ if(LOGS_PAGE<LOGS_PAGES) searchLogs(LOGS_PAGE+1); }
      else{ const p=parseInt(v); if(p>0&&p<=LOGS_PAGES) searchLogs(p); }
    };
  });
}

// 一键诊断：批量错误日志
async function diagnoseBulkLogs(hostID, hostname, sinceMin){
  toast("正在诊断…","ok");
  try {
    const r=await fetch(`${API}/logs/diagnose`,{
      method:"POST",
      headers:{"Content-Type":"application/json"},
      body:JSON.stringify({host_id:hostID,hostname:hostname,since_min:sinceMin})
    });
    if(!r.ok){ toast("诊断请求失败: "+r.status,"err"); return; }
    const rep=await r.json();
    // 显示诊断结果
    showDiagnosisResult(rep);
  } catch(e){ toast("诊断失败: "+e,"err"); }
}

// 单条日志诊断
async function diagnoseLogLine(log){
  toast("正在诊断…","ok");
  try {
    const r=await fetch(`${API}/logs/diagnose`,{
      method:"POST",
      headers:{"Content-Type":"application/json"},
      body:JSON.stringify({
        host_id:log.host_id||"",
        hostname:log.hostname||"",
        since_min:30,
        single_log:`[${log.level}] ${log.hostname} ${fmtDateTime(log.ts)} ${log.message}`
      })
    });
    if(!r.ok){ toast("诊断请求失败: "+r.status,"err"); return; }
    const rep=await r.json();
    showDiagnosisResult(rep);
  } catch(e){ toast("诊断失败: "+e,"err"); }
}

// 显示诊断结果
function showDiagnosisResult(rep){
  const panel=$("logDiagResult");
  if(!panel) return;
  const findings=(rep.findings||[]).map(f=>`<div class="ai-finding"><span class="badge ${f.severity==="critical"?"crit":"warn"}">${esc(f.severity)}</span><div class="ai-f-body"><div class="ai-f-title">${esc(f.title)}</div>${f.detail?`<div class="ai-f-detail">${esc(f.detail)}</div>`:""}</div></div>`).join("");
  panel.innerHTML=`<div class="log-diag-card">
    <div class="log-diag-head"><span>🔍 诊断结果</span><button class="log-diag-close" title="关闭">✕</button></div>
    <div class="log-diag-summary">${esc(rep.summary||"")}</div>
    ${findings?`<div class="ai-findings">${findings}</div>`:""}
    ${rep.context?`<div class="log-diag-ctx">${esc(rep.context)}</div>`:""}
  </div>`;
  // CSP 禁内联 onclick：渲染后再绑定关闭（此前 onclick="..." 会被 script-src 'self' 拦截而失效）
  const closeBtn=panel.querySelector(".log-diag-close");
  if(closeBtn) closeBtn.onclick=()=>{ panel.innerHTML=""; };
  panel.scrollIntoView({behavior:"smooth",block:"nearest"});
}
/* ---- AI 巡检 ---- */
async function loadInspections(){
  try {
    const list=await fetch(`${API}/ai/inspections`).then(r=>r.json());
    const el=$("aiReportList");
    if(!list||!list.length){ el.innerHTML=`<div class="empty-line">暂无巡检报告，点「立即巡检」生成一次。</div>`; return; }
    el.innerHTML=list.map(rep=>{
      const f=(rep.findings||[]).map(x=>`<div class="ai-finding"><span class="badge ${_sevCls(x.severity)}">${esc(x.severity)}</span><div class="ai-f-body"><div class="ai-f-title">${esc(x.title)}</div>${x.detail?`<div class="ai-f-detail">${esc(x.detail)}</div>`:""}</div></div>`).join("");
      const meta=[rep.model?esc(rep.model):"",(typeof rep.duration_ms==="number"&&rep.duration_ms>=0)?rep.duration_ms+"ms":""].filter(Boolean).join(" · ");
      return `<div class="ai-report"><div class="ai-report-head"><span class="badge ${rep.source==="ai"?"info":""}">${rep.source==="ai"?"AI 研判":"启发式"}</span><span class="ai-report-trigger">${rep.trigger==="manual"?"手动":"定时"}</span>${meta?`<span class="mono" style="color:var(--muted2);font-size:11px">${meta}</span>`:""}<span class="mono" style="color:var(--muted);margin-left:auto">${fmtDateTime(rep.ts)}</span></div>
        ${rep.context?`<div class="ai-report-ctx">${esc(rep.context)}</div>`:""}
        <div class="ai-summary">${esc(rep.summary)}</div>${f?`<div class="ai-findings">${f}</div>`:""}</div>`;
    }).join("");
  } catch(e){ toast("加载失败: "+e,"err"); }
}
async function runInspect(){ toast("巡检中…","ok"); try { await fetch(`${API}/ai/inspect`,{method:"POST"}); loadInspections(); } catch(e){ toast("巡检失败: "+e,"err"); } }
// AI 技能库：查看/删除自进化提炼的技能，手动触发提炼
async function openSkills(){
  const m=$("skillsMask"); if(m) m.classList.add("show");
  await loadSkills();
}
async function loadSkills(){
  const body=$("skillsBody"); if(!body) return;
  body.innerHTML=`<div class="empty-line" style="padding:16px">加载中…</div>`;
  try{
    const skills=await fetch(`${API}/ai/skills`).then(r=>r.json());
    if(!skills||!skills.length){
      body.innerHTML=`<div class="empty-line" style="padding:20px">还没有技能。随着 AI 诊断 / 剧本执行 / 事件解决 的经验积累，系统每日会自动从中提炼可复用技能；也可点右上角「立即提炼」。</div>`;
      return;
    }
    body.innerHTML=`<div class="skill-list">`+skills.map(s=>{
      const succ=s.use_count>0?Math.round((s.success_count/s.use_count)*100):0;
      return `<div class="skill-card">
        <div class="skill-head"><b>${esc(s.name)}</b>
          <span class="skill-meta">用 ${s.use_count} · 成功 ${succ}% · 权重 ${(s.priority||1).toFixed(1)}${s.source==="manual"?" · 手工":""}</span>
          <button class="btn danger sm" data-skill-del="${s.id}">删除</button></div>
        <div class="skill-trigger">适用：${esc(s.trigger||"")}</div>
        <pre class="skill-steps">${esc(s.steps||"")}</pre>
        ${s.tags?`<div class="skill-tags">🏷️ ${esc(s.tags)}</div>`:""}
      </div>`;
    }).join("")+`</div>`;
    body.querySelectorAll("[data-skill-del]").forEach(b=>b.onclick=async()=>{
      if(!confirm("删除该技能？")) return;
      await fetch(`${API}/ai/skills/${b.dataset.skillDel}`,{method:"DELETE"});
      loadSkills();
    });
  }catch(e){ body.innerHTML=`<div class="empty-line" style="padding:16px">加载失败：${esc(String(e))}</div>`; }
}
async function distillSkillsNow(){
  toast("提炼中，请稍候…","ok");
  try{
    const j=await fetch(`${API}/ai/skills/distill`,{method:"POST"}).then(r=>r.json());
    if(j.ok) toast(`提炼完成，新增 ${j.created||0} 条技能`,"ok"); else toast("提炼失败："+(j.error||"未知"),"err");
    loadSkills();
  }catch(e){ toast("提炼失败："+e,"err"); }
}
// 值班晨报：拉取服务端态势汇总（未决事件/SLO/待审批修复/巡检）→ 走统一 /ai/assist 流式生成
async function genDutyReport(){
  let j;
  try { j = await fetch(`${API}/ai/duty-context`).then(r=>r.json()); }
  catch(e){ toast("获取运维态势失败："+e,"err"); return; }
  openAIAssist({
    task:"duty_report",
    title:"🌅 AI 值班晨报",
    mode:"analyze",
    context:(j&&j.context)?j.context:"（当前无态势数据）",
    hint:(j&&j.notable===false)?"当前态势平静，无未决事件/SLO超标/待审批修复。":"正在汇总今日运维态势…"
  });
}
async function openAIConfig(){
  const tr=$("aiChatTestResult"); if(tr){ tr.textContent=""; tr.className="ai-test-result"; }
  const er=$("aiEmbedTestResult"); if(er){ er.textContent=""; er.className="ai-test-result"; }
  try { const c=await fetch(`${API}/ai/config`).then(r=>r.json());
    $("aiEnabled").checked=!!c.enabled; $("aiEndpoint").value=c.endpoint||""; $("aiKey").value=c.api_key||""; $("aiModel").value=c.model||""; $("aiInterval").value=c.inspect_interval_min||30;
    $("embedEndpoint").value=c.embed_endpoint||""; $("embedKey").value=c.embed_api_key||""; $("embedModel").value=c.embed_model||""; $("embedDim").value=c.embed_dimensions||"";
    if($("rerankEndpoint")){ $("rerankEndpoint").value=c.rerank_endpoint||""; $("rerankKey").value=c.rerank_api_key||""; $("rerankModel").value=c.rerank_model||""; }
    if($("aiSelfVerify")) $("aiSelfVerify").checked=!!c.self_verify;
    if($("aiMoAModels")) $("aiMoAModels").value=c.moa_models||"";
    if($("mcpEnabled")) $("mcpEnabled").checked=!!c.mcp_enabled;
    if($("mcpToken")) $("mcpToken").value=c.mcp_token||"";
    AI_TERM_ENABLED=!!c.hermes_terminal_enabled; renderAITermState();
    // 更新向量化 / 重排模型卡片摘要
    updateEmbedCardSummary(); updateRerankCardSummary();
    // 向量化、重排模型默认折叠
    const body=$("embedCardBody"), arrow=$("embedCardArrow");
    if(body){ body.style.display="none"; }
    if(arrow){ arrow.classList.remove("open"); }
    const rbody=$("rerankCardBody"), rarrow=$("rerankCardArrow");
    if(rbody){ rbody.style.display="none"; }
    if(rarrow){ rarrow.classList.remove("open"); }
  } catch(e){}
  loadAIModels();
  $("aiConfigMask").classList.add("show");
}
// ===== AI 终端只读巡检权限：独立开关，开启需终端连接密码 =====
let AI_TERM_ENABLED=false;
function renderAITermState(){
  const lbl=$("aiTermStateLabel"), btn=$("aiTermToggleBtn"), row=$("aiTermPwRow"), msg=$("aiTermMsg");
  if(lbl){ lbl.textContent=AI_TERM_ENABLED?"已开启":"未开启"; lbl.className="ai-term-state "+(AI_TERM_ENABLED?"on":"off"); }
  if(btn){ btn.textContent=AI_TERM_ENABLED?"关闭":"开启"; }
  if(row) row.style.display="none";
  if(msg){ msg.textContent=""; msg.className="ai-term-msg"; }
}
function toggleAITerm(){
  if(AI_TERM_ENABLED){ aiTermSet(false,""); return; } // 关闭无需密码
  const row=$("aiTermPwRow"); if(row) row.style.display="flex";
  const pw=$("aiTermPw"); if(pw){ pw.value=""; setTimeout(()=>pw.focus(),50); }
}
function confirmAITerm(){
  const pw=$("aiTermPw"), msg=$("aiTermMsg"), password=pw?pw.value:"";
  if(!password){ if(msg){ msg.textContent="请输入终端连接密码"; msg.className="ai-term-msg err"; } return; }
  aiTermSet(true,password);
}
async function aiTermSet(enabled,password){
  const msg=$("aiTermMsg");
  try{
    const r=await fetch(`${API}/ai/terminal-access`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({enabled,password})});
    const j=await r.json().catch(()=>({}));
    if(!r.ok){ if(msg){ msg.textContent="✗ "+(j.error||("HTTP "+r.status)); msg.className="ai-term-msg err"; } return; }
    AI_TERM_ENABLED=!!j.enabled; renderAITermState();
    if(msg){ msg.textContent=AI_TERM_ENABLED?"✓ 已开启：AI 可执行只读终端巡检（仅查询，禁止任何增删改）":"已关闭 AI 终端巡检"; msg.className="ai-term-msg ok"; }
    if(typeof toast==="function") toast(AI_TERM_ENABLED?"已开启 AI 终端只读巡检":"已关闭 AI 终端巡检","ok");
  }catch(e){ if(msg){ msg.textContent="✗ 请求失败："+e; msg.className="ai-term-msg err"; } }
}
// 从当前表单 Endpoint+Key 自动获取该 Provider 的可用模型，填充自定义下拉（可搜索）；
// 获取不到时保留手动输入。不再内置任何预设模型。
let _aiModelsReq=0;
let AI_MODELS=[]; // 已获取的可选模型 [{value,label}]
async function loadAIModels(){
  const info=$("aiModelInfo");
  const ep=($("aiEndpoint").value||"").trim();
  const myReq=++_aiModelsReq;
  if(!ep){ AI_MODELS=[]; renderModelDropdown(); if(info) info.textContent="· 填入 Endpoint 后自动获取，或直接手动输入模型名"; return; }
  if(info) info.textContent="· 获取中…";
  try {
    const body={endpoint:ep,api_key:$("aiKey").value||""};
    const m=await fetch(`${API}/ai/models`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)}).then(r=>r.json());
    if(myReq!==_aiModelsReq) return; // 有更新的请求在途，丢弃过期结果
    AI_MODELS=(m&&Array.isArray(m.models))?m.models:[];
    renderModelDropdown();
    if(info) info.textContent=AI_MODELS.length
      ? `· 已获取 ${AI_MODELS.length} 个模型，点输入框展开选择 / 搜索 / 手动输入`
      : "· 未获取到模型，请检查 Endpoint/Key，或直接手动输入模型名";
  } catch(e){ if(myReq!==_aiModelsReq) return; if(info) info.textContent="· 获取失败，可手动输入模型名"; }
}
// 自定义模型下拉：始终显示全部已获取模型（可按输入内容过滤），点选填入输入框。
// 替代原生 <datalist>——原生下拉会按输入框【已有值】过滤，导致“提示 N 个却只显示 1 个”。
function renderModelDropdown(filter){
  const dd=$("aiModelDropdown"); if(!dd) return;
  const f=(filter||"").trim().toLowerCase();
  const list=AI_MODELS.filter(x=>!f || String(x.value).toLowerCase().includes(f) || String(x.label||"").toLowerCase().includes(f));
  if(!list.length){ dd.innerHTML=`<div class="ai-model-empty">${AI_MODELS.length?"无匹配模型":"暂无模型，填好 Endpoint+Key 后点刷新"}</div>`; return; }
  dd.innerHTML=list.map(x=>`<div class="ai-model-opt" data-val="${esc(x.value)}" title="${esc(x.value)}">${esc(x.label||x.value)}</div>`).join("");
  dd.querySelectorAll(".ai-model-opt").forEach(el=>el.onclick=()=>{ const t=$("aiModel"); if(t) t.value=el.dataset.val; hideModelDropdown(); });
}
function showModelDropdown(){ const dd=$("aiModelDropdown"); if(!dd) return; renderModelDropdown(); dd.style.display="block"; } // 展开显示全部（不按已选值过滤，正是修复点）
function hideModelDropdown(){ const dd=$("aiModelDropdown"); if(dd) dd.style.display="none"; }
function toggleModelDropdown(){ const dd=$("aiModelDropdown"); if(dd&&dd.style.display==="block") hideModelDropdown(); else showModelDropdown(); }
// AI 预设:仅设置 Endpoint（两种接口类型:OpenAI 兼容 / Anthropic，按端点自动识别）。
// 取消默认预设模型：切换 Provider 后清空模型，改由自动获取 / 搜索 / 手动输入。
function setAIPreset(type){
  const presets={
    "bailian":{endpoint:"https://dashscope.aliyuncs.com/compatible-mode/v1",label:"阿里云百炼（OpenAI 兼容）"},
    "openai":{endpoint:"https://api.openai.com/v1",label:"OpenAI"},
    "deepseek":{endpoint:"https://api.deepseek.com/v1",label:"DeepSeek"},
    "ollama":{endpoint:"http://localhost:11434/v1",label:"本地 Ollama"},
    "claude":{endpoint:"https://dashscope.aliyuncs.com/apps/anthropic",label:"Claude（百炼 Anthropic）"},
  };
  const p=presets[type]; if(!p) return;
  $("aiEndpoint").value=p.endpoint;
  $("aiModel").value=""; // 取消默认预设模型，切 Provider 后需重新获取/输入
  toast(`已设为 ${p.label} · 正在获取模型…`,"ok");
  loadAIModels(); // 选预设后自动获取该 provider 的模型
}
async function saveAIConfig(){
  const enabled=$("aiEnabled").checked, endpoint=$("aiEndpoint").value.trim(), model=$("aiModel").value.trim();
  if(enabled && (!endpoint || !model)){ toast("启用 AI 需填写 Endpoint 和模型","err"); return; } // 轻校验：启用却没填必填项
  const body={enabled,endpoint,api_key:$("aiKey").value,model,inspect_interval_min:parseInt($("aiInterval").value)||30,
    embed_endpoint:$("embedEndpoint").value.trim(),embed_api_key:$("embedKey").value,embed_model:$("embedModel").value.trim(),embed_dimensions:parseInt($("embedDim").value)||0,
    rerank_endpoint:($("rerankEndpoint")?.value||"").trim(),rerank_api_key:$("rerankKey")?.value||"",rerank_model:($("rerankModel")?.value||"").trim(),
    self_verify:$("aiSelfVerify")?.checked||false,moa_models:($("aiMoAModels")?.value||"").trim(),
    mcp_enabled:$("mcpEnabled")?.checked||false,mcp_token:($("mcpToken")?.value||"").trim()};
  const r=await fetch(`${API}/ai/config`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
  if(r.ok){ $("aiConfigMask").classList.remove("show"); toast("已保存","ok"); } else toast("保存失败","err");
}
// AI 对话模型连接测试：通过 SSE 流式验证 Provider 连通性，展示延迟 + 回复摘要
let _aiTestBusy=false;
async function testAIChatConfig(){
  if(_aiTestBusy) return;
  const el=$("aiChatTestResult");
  const endpoint=$("aiEndpoint").value.trim(), model=$("aiModel").value.trim();
  if(!endpoint||!model){ if(el){ el.textContent="✗ 请先填写 Endpoint 和模型"; el.className="ai-test-result err"; } return; }
  _aiTestBusy=true;
  const testBtn=$("aiChatTestBtn"); if(testBtn) testBtn.disabled=true;
  if(el){ el.textContent="对话模型 测试中…"; el.className="ai-test-result testing"; }
  const body={enabled:true,endpoint,api_key:$("aiKey").value,model};
  try{
    const r=await fetch(`${API}/ai/test`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
    if(!r.ok){ throw new Error("HTTP "+r.status); }
    let resultMeta=null, reply="", error=null;
    await readSSEStream(r,
      (delta,fullText)=>{ reply=fullText; },
      (err)=>{ error=err; },
      (fullText)=>{ if(!reply) reply=fullText; },
      (meta)=>{ resultMeta=meta; }
    );
    if(!el) return;
    if(error){
      el.textContent="✗ 对话模型 "+error; el.className="ai-test-result err"; el.style.whiteSpace="pre-wrap";
      return;
    }
    if(resultMeta && resultMeta.ok){
      let extra="";
      if(resultMeta.provider_hint){
        const labels={openai:"OpenAI 兼容","bailian-compat":"百炼兼容",anthropic:"Anthropic"};
        extra=` · ${labels[resultMeta.provider_hint]||resultMeta.provider_hint}`;
      }
      el.textContent=`✓ 对话模型可用${extra} · ${resultMeta.latency_ms||0}ms · ${(resultMeta.reply||"").slice(0,48)}`; el.className="ai-test-result ok";
    } else if(reply){
      el.textContent=`✓ 对话模型可用 · ${reply.slice(0,48)}`; el.className="ai-test-result ok";
    } else {
      el.textContent="✗ 对话模型 未收到有效回复"; el.className="ai-test-result err";
    }
  }catch(e){ if(el){ el.textContent="✗ 对话模型 请求失败："+e; el.className="ai-test-result err"; } }
  finally{ _aiTestBusy=false; if(testBtn) testBtn.disabled=false; }
}

// AI 向量化模型连接测试
let _aiEmbedTestBusy=false;
async function testAIEmbedConfig(){
  if(_aiEmbedTestBusy) return;
  const el=$("aiEmbedTestResult");
  _aiEmbedTestBusy=true;
  const testBtn=$("aiEmbedTestBtn"); if(testBtn) testBtn.disabled=true;
  if(el){ el.textContent="向量化模型 测试中…"; el.className="ai-test-result testing"; }
  const body={enabled:true,
    embed_endpoint:$("embedEndpoint").value.trim(),
    embed_api_key:$("embedKey").value,
    embed_model:$("embedModel").value.trim(),
    embed_dimensions:parseInt($("embedDim").value)||0,
    endpoint:$("aiEndpoint").value.trim(),
    api_key:$("aiKey").value
  };
  try{
    const r=await fetch(`${API}/ai/test-embed`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
    const j=await r.json().catch(()=>({}));
    if(!el) return;
    if(j.ok){
      el.textContent=`✓ 向量化模型可用 · ${j.latency_ms||0}ms · ${j.dimensions||0}维 · ${j.model||""}`; el.className="ai-test-result ok";
    } else {
      el.textContent="✗ 向量化模型 "+(j.error||"测试失败"); el.className="ai-test-result err";
    }
  }catch(e){ if(el){ el.textContent="✗ 向量化模型 请求失败："+e; el.className="ai-test-result err"; } }
  finally{ _aiEmbedTestBusy=false; if(testBtn) testBtn.disabled=false; }
}

// 折叠/展开向量化模型卡片
function toggleEmbedCard(){
  const body=$("embedCardBody"), arrow=$("embedCardArrow");
  if(!body) return;
  const isOpen=body.style.display!=="none";
  body.style.display=isOpen?"none":"block";
  if(arrow){ arrow.classList.toggle("open",!isOpen); }
}

// 更新向量化模型卡片折叠状态摘要
function updateEmbedCardSummary(){
  const summary=$("embedCardSummary"); if(!summary) return;
  const model=$("embedModel").value.trim();
  if(model){ summary.textContent=` · 已配置：${model}`; }
  else { summary.textContent=""; }
}

// 折叠/展开重排模型卡片
function toggleRerankCard(){
  const body=$("rerankCardBody"), arrow=$("rerankCardArrow");
  if(!body) return;
  const isOpen=body.style.display!=="none";
  body.style.display=isOpen?"none":"block";
  if(arrow){ arrow.classList.toggle("open",!isOpen); }
}
// 更新重排模型卡片折叠状态摘要
function updateRerankCardSummary(){
  const summary=$("rerankCardSummary"); if(!summary) return;
  const model=($("rerankModel")?.value||"").trim();
  summary.textContent=model?` · 已启用：${model}`:" · 未启用";
}

// 过滤 AI 输出中的敏感信息（密钥 / 密码 / token）。代码与命令予以保留、交由 Markdown 渲染
// 展示——工具调用 JSON 已在后端剥离，这里仅对结尾残留兜底，不再误删正文里的命令/代码。
function filterDisplayContent(text){
  if(!text) return text;
  let t=text;
  t=t.replace(/\{\s*"tool_calls"[\s\S]*?\}\s*$/g,''); // 兜底：结尾残留的 tool_calls JSON
  t=t.replace(/\b(sk-[a-zA-Z0-9_-]{20,})\b/g,'[已隐藏密钥]'); // API 密钥
  t=t.replace(/\b(api_key|apikey|secret|password|passwd|token)\s*[:=]\s*['"]?[^\s'"]+['"]?/gi,'$1=[已隐藏]');
  return t.trim();
}
// 轻量 Markdown 渲染：先转义 HTML 防 XSS，再套用有限格式（加粗/斜体/有序无序列表/换行）。
// 输入应为已经 filterDisplayContent 过滤的文本（代码块/密钥已剔除）。
// ===== 轻量语法高亮（CSP 安全·零依赖）：常见运维语言的 注释/字符串/数字/关键字 =====
const HL_KW = {
  python:"def class return if elif else for while import from as with try except finally raise in is and or not None True False lambda pass break continue yield assert del async await global nonlocal self print",
  py:"def class return if elif else for while import from as with try except finally raise in is and or not None True False lambda pass break continue yield del async await self",
  bash:"if then else elif fi for while do done case esac function in return export local echo cd exit set unset read source",
  sh:"if then else elif fi for while do done case esac function in return export local echo cd exit set unset read source",
  shell:"if then else elif fi for while do done case esac function in return export local echo cd exit",
  javascript:"function return if else for while const let var new class extends import export default async await try catch finally throw typeof instanceof in of null undefined true false this switch case break continue delete void",
  js:"function return if else for while const let var new class import export default async await try catch throw null undefined true false this switch case break continue",
  typescript:"function return if else for while const let var new class extends implements interface type enum import export default async await try catch finally throw typeof in of null undefined true false this public private protected readonly",
  ts:"function return if else for while const let var new class interface type import export async await try catch throw null undefined true false public private readonly",
  go:"func package import return if else for range var const type struct interface map chan go defer select switch case break continue fallthrough nil true false make new append len cap panic recover",
  sql:"select from where insert update delete into values set create table drop alter add index join left right inner outer full on group by order having limit offset as and or not null is distinct count sum avg min max like between union all",
  json:"true false null",
  yaml:"true false null yes no on off",
  yml:"true false null yes no on off",
  java:"public private protected class interface extends implements return if else for while new import package void int long double float boolean char String null true false this static final abstract try catch finally throw throws",
  c:"int char float double void long short unsigned signed return if else for while do struct union enum typedef const static sizeof break continue switch case default goto NULL",
  cpp:"int char float double void return if else for while class struct namespace using template typename const static public private protected virtual true false nullptr new delete this",
  rust:"fn let mut const struct enum impl trait pub use mod match if else for while loop return break continue self Self Some None Ok Err true false as ref move async await where",
};
const HL_LINE = { python:"#",py:"#",bash:"#",sh:"#",shell:"#",yaml:"#",yml:"#",toml:"#",ini:"#",conf:"#",sql:"--",javascript:"//",js:"//",typescript:"//",ts:"//",go:"//",java:"//",c:"//",cpp:"//",rust:"//" };
const HL_BLOCK = { javascript:1,js:1,typescript:1,ts:1,go:1,java:1,c:1,cpp:1,rust:1,css:1 };
function _hlEsc(s){ return String(s).replace(/[&<>]/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;"}[c])); }
function _hlReEsc(s){ return s.replace(/[.*+?^${}()|[\]\\]/g,"\\$&"); }
function highlightCode(code, lang){
  lang=String(lang||"").toLowerCase();
  const kw=new Set((HL_KW[lang]||"").split(/\s+/).filter(Boolean));
  const line=Object.prototype.hasOwnProperty.call(HL_LINE,lang)?HL_LINE[lang]:null;
  const block=!!HL_BLOCK[lang];
  const parts=[];
  if(block) parts.push("\\/\\*[\\s\\S]*?\\*\\/");
  if(line) parts.push(_hlReEsc(line)+"[^\\n]*");
  parts.push('"(?:\\\\.|[^"\\\\\\n])*"',"'(?:\\\\.|[^'\\\\\\n])*'","`(?:\\\\.|[^`\\\\])*`");
  parts.push("\\b\\d[\\d._]*\\b","[A-Za-z_$][A-Za-z0-9_$]*");
  const re=new RegExp(parts.join("|"),"g");
  let out="",last=0,m;
  while((m=re.exec(code))){
    out+=_hlEsc(code.slice(last,m.index));
    const tok=m[0]; last=m.index+tok.length;
    let cls="";
    if(block&&tok.startsWith("/*")) cls="tok-com";
    else if(line&&tok.startsWith(line)) cls="tok-com";
    else if(tok[0]==='"'||tok[0]==="'"||tok[0]==="`") cls="tok-str";
    else if(tok[0]>="0"&&tok[0]<="9") cls="tok-num";
    else if(kw.has(tok)) cls="tok-kw";
    out+=cls?`<span class="${cls}">${_hlEsc(tok)}</span>`:_hlEsc(tok);
  }
  out+=_hlEsc(code.slice(last));
  return out;
}
function renderAIMarkdown(raw){
  if(!raw) return "";
  // 1) 先抽出围栏代码块占位，避免其内部被当作 Markdown/HTML 处理
  const blocks=[];
  let t=raw.replace(/```([a-zA-Z0-9_+#-]*)\n?([\s\S]*?)```/g,(m,lang,code)=>{
    blocks.push({lang:(lang||"").toLowerCase(), code:code.replace(/\n+$/,"")});
    return "SNTLCB"+(blocks.length-1)+"SNTL";
  });
  t=esc(t); // 2) 转义 HTML，杜绝注入
  t=t.replace(/\[([^\]\n]+)\]\(([^)\n]+)\)/g,"$1"); // 3) 链接 → 仅保留文字（聊天气泡不放裸链接）
  t=t.replace(/`([^`\n]+)`/g,"<code>$1</code>"); // 行内代码（内容已转义）
  t=t.replace(/\*\*([^*\n]+)\*\*/g,"<strong>$1</strong>"); // 4) 加粗 / 斜体
  t=t.replace(/__([^_\n]+)__/g,"<strong>$1</strong>");
  t=t.replace(/(^|[^*])\*([^*\n]+)\*(?!\*)/g,"$1<em>$2</em>");
  const lines=t.split("\n"); // 5) 标题 / 引用 / 分割线 / 列表 / 段落
  let html="",inList=false,listTag="ul";
  const close=()=>{ if(inList){ html+="</"+listTag+">"; inList=false; } };
  for(const line of lines){
    if(line.indexOf("SNTLCB")>=0){ close(); html+=line; continue; } // 代码块占位
    if(/^\s*([-*_])\1{2,}\s*$/.test(line)){ close(); html+="<hr class='ai-hr'>"; continue; } // 分割线 --- *** ___
    const h=line.match(/^\s*(#{1,6})\s*(.*)$/); // 标题 → 样式化，绝不残留 ### 字面量
    if(h){ close(); const tx=h[2].trim(); if(tx) html+=`<div class="ai-h ai-h${Math.min(h[1].length,4)}">${tx}</div>`; continue; }
    const bq=line.match(/^\s*&gt;\s?(.*)$/); // 引用（esc 后 > 变 &gt;）
    if(bq){ close(); html+=`<blockquote class="ai-bq">${bq[1]}</blockquote>`; continue; }
    // P3 诊断置信度：整行以「置信度：高/中/低」起头时渲染为彩色徽章（容忍前置的 <strong>/*/> 标记）
    if(/^\s*(?:<[^>]+>|[*>\s])*置信度\s*[:：]/.test(line)){
      const cm=line.match(/置信度\s*[:：]\s*(?:<[^>]+>|[*\s])*(高|中|低)/);
      if(cm){ close(); const lv=cm[1]; const cls=lv==="高"?"high":(lv==="低"?"low":"mid");
        html+=`<div class="ai-confidence ${cls}"><span class="ai-conf-badge">🎯 置信度 ${lv}</span></div>`; continue; }
    }
    const ul=line.match(/^\s*[-*•·]\s+(.+)$/);
    const ol=line.match(/^\s*\d+[.)]\s+(.+)$/);
    if(ul){ if(!inList||listTag!=="ul"){ close(); html+="<ul>"; inList=true; listTag="ul"; } html+="<li>"+ul[1]+"</li>"; }
    else if(ol){ if(!inList||listTag!=="ol"){ close(); html+="<ol>"; inList=true; listTag="ol"; } html+="<li>"+ol[1]+"</li>"; }
    else { close(); html+=(line.trim()==="")?"":("<div>"+line+"</div>"); }
  }
  close();
  html=html.replace(/SNTLCB(\d+)SNTL/g,(m,i)=>{ const b=blocks[+i]||{code:""}; const lang=b.lang||"代码"; // 6) 还原代码块：语言标签 + 独立复制按钮
    return "<div class=\"ai-code-wrap\"><div class=\"ai-code-head\"><span class=\"ai-code-lang\">"+esc(lang)+"</span><button class=\"ai-code-copy\" type=\"button\" title=\"复制代码\">复制</button></div><pre class=\"ai-code\"><code>"+highlightCode(b.code,b.lang)+"</code></pre></div>"; });
  return html;
}
// AI 对话消息区：判断是否贴底（供流式时决定要不要自动滚动）
function aiChatStick(){ const log=$("aiChatLog"); return log ? (log.scrollHeight-log.scrollTop-log.clientHeight<80) : true; }
function aiChatToBottom(){ const log=$("aiChatLog"); if(log) log.scrollTop=log.scrollHeight; }
// 统一「AI 对话」——单窗口,后端走 Hermes 自主运维 Agent（能对话 + 自动调用工具,
// 不需要工具时自动退化成纯对话）。模型与 AI 设置共用同一套配置。
let AI_CHAT_SESSION=0;   // Hermes 服务端会话 id（0=新会话）
let AI_CHAT_HISTORY=[];  // 前端侧会话历史 {role,content}：兜底传后端 + 本地记忆
const AI_CHAT_INTRO=`<div class="ai-welcome"><div class="ai-welcome-icon">🤖</div><div class="ai-welcome-title">AI 运维助手已就绪</div><div class="ai-welcome-sub">描述问题即可自动排查——查指标 / 日志 / 告警 / 诊断 / 修复，并识别当前纳管主机；也可上传 📄 文档 / 🔗 网页辅助分析。</div></div><div id="aiChatSuggest" class="ai-suggest"></div>`;
function openAIChat(){
  newAIChat();
  $("aiChatMask").classList.add("show");
  loadAISessions();
  setTimeout(()=>{ const i=$("aiChatInput"); if(i) i.focus(); },80);
}
// 开新会话：清空会话 id / 历史 / 消息区
function newAIChat(){
  if(_aiChatBusy) stopAIChat(); // 开新会话前终止在途
  AI_CHAT_SESSION=0; AI_CHAT_HISTORY=[]; AI_ATTACHMENTS=[]; AI_CHAT_QUEUE=[];
  const log=$("aiChatLog"); if(log) log.innerHTML=AI_CHAT_INTRO;
  const sel=$("aiSessionSelect"); if(sel) sel.value="";
  renderAttachments(); renderQueueHint(); setAIChatBusyUI(false);
  loadAISuggestions(); // 拉取并渲染快捷问题/推荐 Prompt
}
// ===== 快捷问题 / 推荐 Prompt（结合当前告警/主机/日志的动态建议 + 能力示例，随机展示） =====
let AI_SUGGEST={dynamic:[],curated:[]};
function _aiShuffle(a){ a=a.slice(); for(let i=a.length-1;i>0;i--){ const j=Math.floor(Math.random()*(i+1)); const t=a[i]; a[i]=a[j]; a[j]=t; } return a; }
async function loadAISuggestions(){
  const box=$("aiChatSuggest"); if(!box) return;
  try{
    const r=await fetch(`${API}/hermes/suggestions`); if(!r.ok){ box.style.display="none"; return; }
    AI_SUGGEST=(await r.json())||{dynamic:[],curated:[]};
    renderAISuggest();
  }catch(e){ box.style.display="none"; }
}
function renderAISuggest(){
  const box=$("aiChatSuggest"); if(!box) return;
  const dyn=(AI_SUGGEST.dynamic||[]).slice(0,2);
  const need=Math.max(0,5-dyn.length);
  const cur=_aiShuffle(AI_SUGGEST.curated||[]).slice(0,need);
  const items=dyn.concat(cur);
  if(!items.length){ box.style.display="none"; return; }
  box.style.display="";
  box.innerHTML=`<div class="ai-suggest-head"><span>💡 试试这些问题</span><button class="ai-suggest-refresh" title="换一批推荐">↻ 换一批</button></div>`+
    `<div class="ai-suggest-chips">`+items.map(q=>`<button class="ai-suggest-chip" data-q="${esc(q)}">${esc(q)}</button>`).join("")+`</div>`;
  const rf=box.querySelector(".ai-suggest-refresh"); if(rf) rf.onclick=renderAISuggest;
  box.querySelectorAll(".ai-suggest-chip").forEach(b=>b.onclick=()=>{ const inp=$("aiChatInput"); if(inp) inp.value=b.dataset.q; sendAIChat(); });
}
// 加载历史会话列表到下拉选择器
async function loadAISessions(){
  const sel=$("aiSessionSelect"); if(!sel) return;
  try{
    const r=await fetch(`${API}/hermes/sessions`);
    if(!r.ok) return;
    const list=await r.json();
    sel.innerHTML=`<option value="">＋ 新会话</option>`+
      (Array.isArray(list)?list:[]).map(s=>{
        const cnt=s.msg_count?` (${s.msg_count})`:"";
        return `<option value="${s.id}">${esc((s.title||"会话")+cnt)}</option>`;
      }).join("");
    sel.value=AI_CHAT_SESSION?String(AI_CHAT_SESSION):"";
  }catch(e){ /* 无 PG / 接口不可用时静默 */ }
}
// 切换到某历史会话并恢复其消息
async function switchAISession(id){
  if(!id){ newAIChat(); return; }
  try{
    const r=await fetch(`${API}/hermes/sessions/${id}`);
    if(!r.ok) throw new Error("HTTP "+r.status);
    const j=await r.json();
    const msgs=(j.messages||[]).filter(m=>m&&(m.role==="user"||m.role==="assistant"));
    AI_CHAT_SESSION=Number(id);
    AI_CHAT_HISTORY=msgs.map(m=>({role:m.role,content:m.content}));
    const log=$("aiChatLog");
    if(log){
      log.innerHTML=msgs.length
        ? msgs.map(m=> m.role==="user"
            ? `<div class="ai-chat-msg me">${esc(m.content||"")}</div>`
            : `<div class="ai-chat-msg ai">${renderAIMarkdown(filterDisplayContent(m.content||""))}</div>`
          ).join("")
        : `<div class="ai-chat-msg sys">（空会话）</div>`;
      log.querySelectorAll(".ai-chat-msg.ai").forEach(d=>addCopyTool(d,d.textContent));
      log.scrollTop=log.scrollHeight;
    }
  }catch(e){ if(typeof toast==="function") toast("加载会话失败："+e,"err"); }
}
// 会话有更新后延迟刷新列表（合并短时间内多次更新）
let _aiSessTimer=null;
function refreshAISessionsSoon(){ if(_aiSessTimer) clearTimeout(_aiSessTimer); _aiSessTimer=setTimeout(loadAISessions,700); }
function appendChatMsg(role,text){
  const log=$("aiChatLog"); if(!log) return null;
  const div=document.createElement("div");
  div.className="ai-chat-msg "+(role==="user"?"me":role==="assistant"?"ai":"sys");
  div.textContent=text;
  log.appendChild(div); log.scrollTop=log.scrollHeight;
  return div;
}
let _aiChatBusy=false;
let _aiChatAbort=null;    // 当前请求的 AbortController
let _aiChatAborted=false; // 本次是否被用户终止
let AI_CHAT_QUEUE=[];     // 排队消息 {msg, atts}
let AI_ATTACHMENTS=[];    // 待发送附件：{kind:"image"|"file", name, mime, data(图片base64), text(文件文本)}
function setAIChatBusyUI(busy){
  const send=$("aiChatSendBtn"), stop=$("aiChatStopBtn");
  if(send) send.style.display=busy?"none":"";
  if(stop) stop.style.display=busy?"":"none";
  const log=$("aiChatLog"); if(log) log.classList.toggle("ai-streaming", busy); // 流式打字光标
}
// 输入框自增高（Claude 风：随内容增长，封顶 168px 后内部滚动）
function autoGrowAIInput(){ const t=$("aiChatInput"); if(!t) return; t.style.height="auto"; t.style.height=Math.min(t.scrollHeight,168)+"px"; }
function renderQueueHint(){
  const el=$("aiChatQueue"); if(!el) return;
  el.textContent=AI_CHAT_QUEUE.length?`⏳ 已排队 ${AI_CHAT_QUEUE.length} 条，将在当前回复完成后依次发送`:"";
}
async function sendAIChat(){
  const inp=$("aiChatInput"); if(!inp) return;
  const msg=inp.value.trim();
  const atts=AI_ATTACHMENTS.slice();
  if(!msg && !atts.length) return; // 无文本且无附件则不发
  { const _sg=$("aiChatSuggest"); if(_sg) _sg.style.display="none"; } // 发起对话后隐藏推荐问题
  if(_aiChatBusy){ // 忙时排队：完成后自动续发（可点终止清空排队）
    AI_CHAT_QUEUE.push({msg,atts});
    inp.value=""; AI_ATTACHMENTS=[]; renderAttachments(); renderQueueHint();
    return;
  }
  inp.value=""; autoGrowAIInput();
  _aiChatBusy=true; _aiChatAborted=false; setAIChatBusyUI(true);
  _aiChatAbort=(typeof AbortController!=="undefined")?new AbortController():null;
  const imgN=atts.filter(a=>a.kind==="image").length, fileN=atts.filter(a=>a.kind==="file").length;
  const attNote=atts.length?`　<span class="ai-att-note">📎 ${imgN?imgN+" 图 ":""}${fileN?fileN+" 文件":""}</span>`:"";
  const log=$("aiChatLog");
  if(log){ const d=document.createElement("div"); d.className="ai-chat-msg me"; d.innerHTML=esc(msg||"（附件）")+attNote; log.appendChild(d); log.scrollTop=log.scrollHeight; }
  AI_CHAT_HISTORY.push({role:"user",content:msg||"（附件）"});
  AI_ATTACHMENTS=[]; renderAttachments();
  const pending=appendChatMsg("assistant","");
  if(pending) pending.innerHTML='<div class="ai-thinking"><span class="ai-thinking-dots"><span></span><span></span><span></span></span> <span class="ai-thinking-text">正在思考…</span></div>';
  let answer="";
  try{
    const images=atts.filter(a=>a.kind==="image").map(a=>({mime:a.mime,data:a.data}));
    const files=atts.filter(a=>a.kind==="file").map(a=>({name:a.name,text:a.text}));
    const r=await fetch(`${API}/hermes/chat`,{method:"POST",headers:{"Content-Type":"application/json"},
      signal:_aiChatAbort?_aiChatAbort.signal:undefined,
      body:JSON.stringify({message:msg,session_id:AI_CHAT_SESSION,history:AI_CHAT_HISTORY.slice(0,-1),images,files,stream:true})});
    if(!r.ok){ throw new Error("HTTP "+r.status); }
    let streamed=false;
    let reasoning=""; // 推理模型思维链（独立于 answer，渲染到「思考过程」折叠区）
    // 工具调用状态 chip（run→ok/err）：与回答正文分离渲染，实时更新且不污染最终回答
    const toolStates=[];
    const toolTraceHTML=()=> toolStates.length ? '<div class="ai-tool-trace">'+toolStates.map(s=>{
      const ic = s.state==="run"
        ? '<svg class="ai-tool-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>'
        : (s.state==="ok" ? "✓" : "✗");
      return `<span class="ai-tool-chip ${s.state}">${ic}<span>${esc(s.name)}</span></span>`;
    }).join("")+'</div>' : "";
    // 流式渲染：使用 requestAnimationFrame 同步到显示刷新（≈16ms），消除 setTimeout 攒批延迟
    let streamRAF=null;
    const paintStream=()=>{
      if(!pending) return;
      pending.innerHTML=renderReasoningBlock(reasoning,true)+toolTraceHTML()
        +'<div class="ai-stream-body"><span class="ai-stream-text">'+esc(answer||"")+"</span><span class=\"ai-stream-cursor\">▍</span></div>";
    };
    const schedulePaint=()=>{
      if(streamRAF) return;
      streamRAF=requestAnimationFrame(()=>{ streamRAF=null; paintStream(); });
    };
    const paintFinal=()=>{
      if(streamRAF){ cancelAnimationFrame(streamRAF); streamRAF=null; }
      if(pending) pending.innerHTML=renderReasoningBlock(reasoning,false)+toolTraceHTML()+(renderAIMarkdown(answer)||(toolStates.length?"":"…"));
    };
    await readSSEStream(r,
      (delta,fullText)=>{
        const stick=aiChatStick();
        if(!streamed){ streamed=true; }
        answer=filterDisplayContent(fullText);
        schedulePaint();
        if(stick) aiChatToBottom();
      },
      (err)=>{ if(streamRAF){ cancelAnimationFrame(streamRAF); streamRAF=null; } if(pending){ pending.textContent="✗ "+err; pending.classList.add("err"); } },
      (fullText)=>{
        if(pending){
          answer=filterDisplayContent(fullText||answer||"");
          paintFinal();
        }
        aiChatToBottom();
      },
      null,
      (meta)=>{ if(meta&&meta.session_id){ AI_CHAT_SESSION=Number(meta.session_id); } },
      (t)=>{ // 工具状态帧：run 追加 chip，ok/err 更新最近的同名 run chip
        if(!t||!t.name) return;
        if(t.state==="run") toolStates.push({name:t.name,state:"run"});
        else { for(let i=toolStates.length-1;i>=0;i--){ if(toolStates[i].name===t.name&&toolStates[i].state==="run"){ toolStates[i].state=t.state; break; } } }
        if(pending && !streamed){ streamed=true; }
        schedulePaint();
        if(aiChatStick()) aiChatToBottom();
      },
      (rd,fullReasoning)=>{ // 思维链增量：累积并实时渲染到折叠区
        if(!streamed){ streamed=true; }
        reasoning=fullReasoning;
        schedulePaint();
        if(aiChatStick()) aiChatToBottom();
      }
    );
    if(answer){ AI_CHAT_HISTORY.push({role:"assistant",content:answer}); addCopyTool(pending,answer); }
    refreshAISessionsSoon();
  }catch(e){
    if(_aiChatAborted || (e&&e.name==="AbortError")){ if(pending){ pending.textContent="⏹ 已终止"; pending.className="ai-chat-msg sys"; } }
    else if(pending){ pending.textContent="✗ 请求失败："+e; pending.classList.add("err"); }
  }
  finally{
    _aiChatBusy=false; _aiChatAbort=null; setAIChatBusyUI(false);
    if(inp) inp.focus();
    if(!_aiChatAborted && AI_CHAT_QUEUE.length){ // 处理排队（终止时不自动续发）
      const next=AI_CHAT_QUEUE.shift(); renderQueueHint();
      const i=$("aiChatInput"); if(i) i.value=next.msg||"";
      AI_ATTACHMENTS=next.atts||[]; renderAttachments();
      setTimeout(sendAIChat,80);
    }
  }
}
// 终止：立即中止在途请求（后端 ctx 取消随即停止 LLM 调用与工具执行），并清空排队
function stopAIChat(){
  _aiChatAborted=true;
  if(_aiChatAbort){ try{ _aiChatAbort.abort(); }catch(e){} }
  AI_CHAT_QUEUE=[]; renderQueueHint();
}
// 撤销上一轮问答：移除末尾 user+assistant 气泡 + 本地历史 + 服务端会话截断，并回填到输入框
async function undoAIChat(){
  if(_aiChatBusy){ if(typeof toast==="function") toast("生成中，请先终止再撤销","err"); return; }
  const log=$("aiChatLog"); if(!log) return;
  let lastUser="";
  for(let i=AI_CHAT_HISTORY.length-1;i>=0;i--){ if(AI_CHAT_HISTORY[i].role==="user"){ lastUser=AI_CHAT_HISTORY[i].content; break; } }
  if(AI_CHAT_HISTORY.length && AI_CHAT_HISTORY[AI_CHAT_HISTORY.length-1].role==="assistant") AI_CHAT_HISTORY.pop();
  if(AI_CHAT_HISTORY.length && AI_CHAT_HISTORY[AI_CHAT_HISTORY.length-1].role==="user") AI_CHAT_HISTORY.pop();
  const bubbles=()=>Array.from(log.querySelectorAll(".ai-chat-msg")).filter(b=>!b.classList.contains("sys"));
  if(!bubbles().length){ if(typeof toast==="function") toast("没有可撤销的对话","err"); return; }
  const lastAi=[...bubbles()].reverse().find(b=>b.classList.contains("ai")); if(lastAi) lastAi.remove();
  const lastMe=[...bubbles()].reverse().find(b=>b.classList.contains("me")); if(lastMe) lastMe.remove();
  if(AI_CHAT_SESSION){ try{ await fetch(`${API}/hermes/sessions/${AI_CHAT_SESSION}/undo`,{method:"POST"}); }catch(e){} refreshAISessionsSoon(); }
  const inp=$("aiChatInput"); if(inp&&lastUser){ inp.value=lastUser; inp.focus(); }
}
function copyText(t){
  if(navigator.clipboard&&navigator.clipboard.writeText){ return navigator.clipboard.writeText(t).catch(()=>_fallbackCopy(t)); }
  _fallbackCopy(t);
}
function _fallbackCopy(t){ const ta=document.createElement("textarea"); ta.value=t; ta.style.position="fixed"; ta.style.opacity="0"; document.body.appendChild(ta); ta.select(); try{document.execCommand("copy");}catch(e){} ta.remove(); }
// 给一条 AI 回复挂上「复制」操作栏
function addCopyTool(div,rawText){
  if(!div) return;
  // 代码块独立复制（复制对应 <pre> 内容）
  div.querySelectorAll(".ai-code-copy").forEach(b=>{
    b.onclick=()=>{ const w=b.closest(".ai-code-wrap"); const c=w&&w.querySelector("pre code"); if(c){ copyText(c.textContent); b.textContent="已复制"; setTimeout(()=>b.textContent="复制",1200); } };
  });
  const bar=document.createElement("div"); bar.className="ai-msg-tools";
  const btn=document.createElement("button"); btn.textContent="复制"; btn.title="复制回复";
  btn.onclick=()=>{ copyText(rawText); btn.textContent="已复制"; setTimeout(()=>{ btn.textContent="复制"; },1200); };
  bar.appendChild(btn);
  const rebtn=document.createElement("button"); rebtn.textContent="重答"; rebtn.title="用上一条问题重新回答";
  rebtn.onclick=regenerateAIChat;
  bar.appendChild(rebtn);
  div.appendChild(bar);
}
// 重答：取最近一条用户提问重新发送（追加一轮新回答）
function regenerateAIChat(){
  if(_aiChatBusy){ if(typeof toast==="function") toast("生成中，请先终止再重答","err"); return; }
  let q=""; for(let i=AI_CHAT_HISTORY.length-1;i>=0;i--){ if(AI_CHAT_HISTORY[i].role==="user"){ q=AI_CHAT_HISTORY[i].content; break; } }
  if(!q){ if(typeof toast==="function") toast("暂无可重答的问题","err"); return; }
  const inp=$("aiChatInput"); if(inp){ inp.value=q; if(typeof autoGrowAIInput==="function") autoGrowAIInput(); }
  sendAIChat();
}
// 附件预览渲染（图片/文件 chip，可删除）
function renderAttachments(){
  const box=$("aiChatAttach"); if(!box) return;
  if(!AI_ATTACHMENTS.length){ box.innerHTML=""; box.style.display="none"; return; }
  box.style.display="flex";
  box.innerHTML=AI_ATTACHMENTS.map((a,i)=>`<span class="ai-attach-chip">${a.kind==="image"?"🖼️":"📄"} ${esc(a.name)}<button data-att="${i}" title="移除">✕</button></span>`).join("");
  box.querySelectorAll("[data-att]").forEach(b=>b.onclick=()=>{ AI_ATTACHMENTS.splice(parseInt(b.dataset.att),1); renderAttachments(); });
}
// 需服务端解析的二进制文档（其余文本文件前端直接读文本）
const _AI_PARSE_EXT=["docx","xlsx","pdf"];
function _extOf(name){ const i=String(name||"").lastIndexOf("."); return i>=0?name.slice(i+1).toLowerCase():""; }
// 选择图片/文件：图片读为 base64（视觉）；docx/xlsx/pdf 经服务端解析成文本；纯文本文件直接读文本。
function onAIChatFiles(ev){
  const files=Array.from((ev.target&&ev.target.files)||[]);
  for(const f of files){
    if(f.type&&f.type.startsWith("image/")){
      if(AI_ATTACHMENTS.filter(a=>a.kind==="image").length>=4){ if(typeof toast==="function") toast("最多 4 张图片","err"); continue; }
      if(f.size>4*1024*1024){ if(typeof toast==="function") toast(`图片 ${f.name} 超过 4MB`,"err"); continue; }
      const rd=new FileReader();
      rd.onload=()=>{ const s=String(rd.result||""); const c=s.indexOf(","); AI_ATTACHMENTS.push({kind:"image",name:f.name,mime:f.type||"image/png",data:c>=0?s.slice(c+1):s}); renderAttachments(); };
      rd.readAsDataURL(f);
    } else if(_AI_PARSE_EXT.includes(_extOf(f.name))){
      if(f.size>10*1024*1024){ if(typeof toast==="function") toast(`文件 ${f.name} 超过 10MB`,"err"); continue; }
      parseFileAttachment(f); // 二进制文档 → 服务端解析
    } else {
      if(f.size>1024*1024){ if(typeof toast==="function") toast(`文件 ${f.name} 超过 1MB，请上传关键片段`,"err"); continue; }
      const rd=new FileReader();
      rd.onload=()=>{ AI_ATTACHMENTS.push({kind:"file",name:f.name,text:String(rd.result||"")}); renderAttachments(); };
      rd.readAsText(f);
    }
  }
  if(ev.target) ev.target.value=""; // 允许重复选同一文件
}
// docx/xlsx/pdf → base64 → POST /hermes/parse → 提取文本作为附件
function parseFileAttachment(f){
  const rd=new FileReader();
  rd.onload=async()=>{
    const s=String(rd.result||""); const c=s.indexOf(","); const b64=c>=0?s.slice(c+1):s;
    const ph={kind:"file",name:f.name,text:"（解析中…）"};
    AI_ATTACHMENTS.push(ph); renderAttachments();
    try{
      const r=await fetch(`${API}/hermes/parse`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({name:f.name,mime:f.type||"",data:b64})});
      const j=await r.json().catch(()=>({}));
      if(!r.ok||j.error){ AI_ATTACHMENTS=AI_ATTACHMENTS.filter(a=>a!==ph); if(typeof toast==="function") toast(`解析 ${f.name} 失败：${(j&&j.error)||r.status}`,"err"); renderAttachments(); return; }
      ph.text=j.text||""; renderAttachments();
      if(typeof toast==="function") toast(`已解析 ${f.name}（${j.chars||0} 字${j.truncated?"，已截断":""}）`,"ok");
    }catch(e){ AI_ATTACHMENTS=AI_ATTACHMENTS.filter(a=>a!==ph); if(typeof toast==="function") toast(`解析 ${f.name} 失败`,"err"); renderAttachments(); }
  };
  rd.readAsDataURL(f);
}
// 识别 URL：抓取网页正文作为附件注入上下文
async function attachURL(){
  const u=(typeof prompt==="function")?prompt("输入要抓取的网页 URL（将提取正文注入对话）："):"";
  if(!u||!u.trim()) return;
  const ph={kind:"file",name:u.trim().slice(0,60),text:"（抓取中…）"};
  AI_ATTACHMENTS.push(ph); renderAttachments();
  try{
    const r=await fetch(`${API}/hermes/parse`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({url:u.trim()})});
    const j=await r.json().catch(()=>({}));
    if(!r.ok||j.error){ AI_ATTACHMENTS=AI_ATTACHMENTS.filter(a=>a!==ph); if(typeof toast==="function") toast(`抓取失败：${(j&&j.error)||r.status}`,"err"); renderAttachments(); return; }
    ph.text=`[来源 URL: ${u.trim()}]\n`+(j.text||""); renderAttachments();
    if(typeof toast==="function") toast(`已抓取（${j.chars||0} 字${j.truncated?"，已截断":""}）`,"ok");
  }catch(e){ AI_ATTACHMENTS=AI_ATTACHMENTS.filter(a=>a!==ph); if(typeof toast==="function") toast("抓取失败","err"); renderAttachments(); }
}
safeAddEventListener("logSearchBtn","click",searchLogs);
safeAddEventListener("logKeyword","keydown",e=>{ if(e.key==="Enter") searchLogs(); });
safeAddEventListener("logSource","change",()=>{ onLogSourceChange(); if(!$("logSource").value) searchLogs(); });
safeAddEventListener("logJob","change",()=>{ onLogJobChange(); });
safeAddEventListener("aiInspectBtn","click",runInspect);
safeAddEventListener("dutyReportBtn","click",genDutyReport);
safeAddEventListener("skillsBtn","click",openSkills);
safeAddEventListener("skillsDistillBtn","click",distillSkillsNow);
safeAddEventListener("aiConfigBtn","click",openAIConfig);
safeAddEventListener("aiConfigSaveBtn","click",saveAIConfig);
safeAddEventListener("aiChatTestBtn","click",testAIChatConfig);
safeAddEventListener("aiEmbedTestBtn","click",testAIEmbedConfig);
safeAddEventListener("embedCardHeader","click",toggleEmbedCard);
safeAddEventListener("rerankCardHeader","click",toggleRerankCard);
safeAddEventListener("aiModelRefreshBtn","click",loadAIModels);
safeAddEventListener("aiEndpoint","change",loadAIModels);
safeAddEventListener("aiKey","change",loadAIModels); // 填/改 API Key 后自动获取模型
safeAddEventListener("aiModelCaretBtn","click",toggleModelDropdown);
safeAddEventListener("aiModel","focus",showModelDropdown);
safeAddEventListener("aiModel","input",e=>{ renderModelDropdown(e.target.value); const dd=$("aiModelDropdown"); if(dd) dd.style.display="block"; });
document.addEventListener("click",e=>{ if(!e.target.closest || !e.target.closest(".ai-model-wrap")) hideModelDropdown(); });
safeAddEventListener("aiTermToggleBtn","click",toggleAITerm);
safeAddEventListener("aiTermConfirmBtn","click",confirmAITerm);
safeAddEventListener("aiTermCancelBtn","click",()=>{ const r=$("aiTermPwRow"); if(r) r.style.display="none"; const m=$("aiTermMsg"); if(m){ m.textContent=""; m.className="ai-term-msg"; } });
safeAddEventListener("aiTermPw","keydown",e=>{ if(e.key==="Enter"){ e.preventDefault(); confirmAITerm(); } });
safeAddEventListener("aiChatBtn","click",openAIChat);
safeAddEventListener("topAiBtn","click",openAIChat); // 顶栏 AI 对话入口（全局可达）
safeAddEventListener("aiChatSendBtn","click",sendAIChat);
safeAddEventListener("aiChatInput","keydown",e=>{ if(e.key==="Enter"&&!e.shiftKey){ e.preventDefault(); sendAIChat(); } });
safeAddEventListener("aiChatInput","input",autoGrowAIInput);
safeAddEventListener("aiChatLog","scroll",()=>{ const b=$("aiChatScrollBtn"); if(b) b.style.display=aiChatStick()?"none":"flex"; });
safeAddEventListener("aiChatScrollBtn","click",()=>{ aiChatToBottom(); const b=$("aiChatScrollBtn"); if(b) b.style.display="none"; });
safeAddEventListener("aiChatAttachBtn","click",()=>{ const f=$("aiChatFile"); if(f) f.click(); });
safeAddEventListener("aiChatUrlBtn","click",attachURL);
safeAddEventListener("aiChatFile","change",onAIChatFiles);
safeAddEventListener("aiChatStopBtn","click",stopAIChat);
safeAddEventListener("aiUndoBtn","click",undoAIChat);
safeAddEventListener("aiNewChatBtn","click",newAIChat);
safeAddEventListener("aiSessionSelect","change",e=>switchAISession(e.target.value));

// （原独立的 Hermes 对话已并入上方统一的「AI 对话」——单窗口即走 Hermes Agent。）

// 终端会话管理 + 回放 + 旁观
safeAddEventListener("termSessionsBtn", "click", openTerminalSessions);
// 终端会话搜索
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

