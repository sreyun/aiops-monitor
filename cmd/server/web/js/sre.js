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
    renderPlaybooks(pbs || []);
  } catch (e) { console.warn("load playbooks:", e); }
}

function renderPlaybooks(pbs) {
  const list = $("playbookList"), empty = $("playbookEmpty");
  if (!list) return;
  if (empty) empty.style.display = pbs.length === 0 ? "" : "none";
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
    return `<div class="pb-step" data-idx="${i}">
      <div class="grid2">
        <div class="field"><label>${I18N.t("form.step_name")}</label><input type="text" class="pb-step-name" value="${esc(s.name||"")}" placeholder="${I18N.t('form.hint_step_name')}"></div>
        <div class="field"><label>${I18N.t("form.target")}</label><div class="select-wrap"><select class="pb-step-target" data-act-change="pb-target-preview">${tgtOpts}</select></div></div>
      </div>
      <div class="pb-target-preview" style="font-size:12px;color:var(--muted2);margin:-4px 0 4px"></div>
      <div class="field"><label>${I18N.t("form.command")}</label><textarea class="pb-step-cmd" rows="2" placeholder="${I18N.t('form.hint_command')}" spellcheck="false" style="resize:vertical;min-height:54px;line-height:1.5">${esc(s.command||"")}</textarea></div>
      <div class="grid2">
        <div class="field"><label>${I18N.t("form.timeout")}</label><input type="text" class="pb-step-timeout mono" value="${s.timeout_sec||30}" style="width:80px"></div>
        <div class="field"><label>${I18N.t("form.continue_err")}</label><label class="switch"><input type="checkbox" class="pb-step-cont" ${s.continue_on_error?"checked":""}> 继续下一步</label></div>
      </div>
      <button class="btn danger sm pb-step-del" type="button">${I18N.t("ui.delete_step")}</button>
    </div>`;
  }).join("");
  c.querySelectorAll(".pb-step-del").forEach(btn => {
    btn.onclick = () => { btn.closest(".pb-step").remove(); };
  });
  // Initialize previews
  c.querySelectorAll(".pb-step-target").forEach(sel => pbTargetPreview(sel));
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
    steps.push({
      name: el.querySelector(".pb-step-name").value.trim(),
      command: el.querySelector(".pb-step-cmd").value.trim(),
      target: el.querySelector(".pb-step-target").value,
      timeout_sec: parseInt(el.querySelector(".pb-step-timeout").value) || 30,
      continue_on_error: el.querySelector(".pb-step-cont").checked
    });
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
  $("execResultTitle").textContent = `${I18N.t("ui.execute")}${exec.status === "completed" ? I18N.t("ui.completed") : exec.status === "failed" ? I18N.t("ui.failed") : I18N.t("ui.running")}`;
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
const SRE_ALERT_TYPES = ["cpu","memory","disk","diskio","iops","gpu","load","proc","offline","check"];
const _sevCls = s => s==="critical"?"crit":s==="warning"?"warn":"info";
const _srcLabel = s => ({alert:"告警",slo:"SLO",manual:"手动"})[s]||esc(s);
const _incStatus = s => ({open:"进行中",acknowledged:"已确认",resolved:"已解决"})[s]||esc(s);
const _incStatusCls = s => s==="resolved"?"ok":s==="acknowledged"?"warn":"crit";
const _tlKind = k => ({created:"创建",fired:"触发",recovered:"恢复",acked:"确认",resolved:"解决",remediation:"自动修复",comment:"评论",escalated:"升级工单",note:"备注",ai_diagnosis:"🤖 AI 诊断"})[k]||k;
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
      <div class="ai-diagnosis-input">
        <textarea id="incDiagInput" rows="2" placeholder="追问 AI 细节、反驳结论、要求进一步排查…"></textarea>
        <button class="btn primary" id="incDiagSendBtn">发送</button>
      </div>
      <label class="ai-term-toggle" id="incTermToggle" style="margin-top:4px;font-size:12px;color:var(--muted);cursor:pointer;display:flex;align-items:center;gap:4px;user-select:none"><input type="checkbox" id="incTermCheck"> 包含终端操作上下文（分段摘要）</label>`;
    const acts=[];
    acts.push(`<button class="btn sm" data-iact="diagnose">🤖 AI 诊断</button>`);
    if (inc.status!=="resolved"){ acts.push(`<button class="btn sm" data-iact="ack">确认</button>`); acts.push(`<button class="btn sm" data-iact="resolve">解决</button>`); }
    if (!inc.ticket_id) acts.push(`<button class="btn sm" data-iact="escalate">升级工单</button>`);
    acts.push(`<div style="flex:1"></div><input type="text" id="incCommentInput" placeholder="添加评论…" style="flex:2;min-width:120px"><button class="btn primary sm" data-iact="comment">发送</button>`);
    const foot=$("incidentDetailFoot"); foot.innerHTML=acts.join("");
    foot.querySelectorAll("[data-iact]").forEach(b=>b.onclick=()=>incidentAction(inc.id,b.dataset.iact));
    // Wire up diagnosis chat
    window._incDiagId = inc.id;
    window._incDiagHistory = [];
    loadDiagnosisChatHistory(inc.id);
    $("incDiagSendBtn").onclick = () => sendDiagnosisChatMsg();
    $("incDiagInput").onkeydown = e => { if (e.key==="Enter" && !e.shiftKey){ e.preventDefault(); sendDiagnosisChatMsg(); } };
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
    else if (act==="diagnose"){ toast("AI 诊断中，请稍候…","ok"); await fetch(`${API}/incidents/${id}/diagnose`,{method:"POST"}); }
    else await fetch(`${API}/incidents/${id}/${act}`,{method:"POST"});
    openIncidentDetail(id); loadIncidents(); loadSREBadge();
  } catch(e){ toast("操作失败: "+e,"err"); }
}
// ---- AI 诊断多轮对话 ----
// readSSEStream reads a Server-Sent Events stream from a fetch response and
// calls onDelta for each token chunk, onError for errors, onResult for result
// metadata, and onDone when complete. Returns the accumulated full text.
async function readSSEStream(resp,onDelta,onError,onDone,onResult,onMeta){
  const reader=resp.body.getReader();
  const decoder=new TextDecoder();
  let buf="";
  let fullText="";
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
            if(j.delta){ fullText+=j.delta; if(onDelta) onDelta(j.delta,fullText); }
          } catch(e){ /* skip malformed chunks */ }
        }
      }
    }
  } finally { reader.releaseLock(); }
  if(onDone) onDone(fullText);
  return fullText;
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
  if(!hist.length){ el.innerHTML=`<div class="empty-line" style="padding:12px">点击上方「🤖 AI 诊断」获取初步研判，然后在此追问细节。</div>`; return; }
  el.innerHTML=hist.map((m,i)=>{
    const cls=m.role==="user"?"me":m.role==="assistant"?"ai":"sys";
    let fb="";
    if(m.role==="assistant" && m.content!=="思考中…"){
      fb=`<div class="ai-chat-fb"><button class="btn-tiny" data-fb="helpful" data-idx="${i}" title="有用">👍</button><button class="btn-tiny" data-fb="unhelpful" data-idx="${i}" title="无用">👎</button></div>`;
    }
    return `<div class="ai-chat-msg ${cls}">${esc(m.content)}${fb}</div>`;
  }).join("");
  // Wire feedback buttons
  el.querySelectorAll("[data-fb]").forEach(b=>b.onclick=()=>sendDiagnosisFeedback(parseInt(b.dataset.idx),b.dataset.fb==="helpful"));
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
  const msg=el.value.trim(); if(!msg) return;
  const chat=$("incDiagnosisChat");
  // Show user message immediately
  window._incDiagHistory.push({role:"user",content:msg});
  renderDiagnosisChat();
  el.value=""; el.disabled=true; $("incDiagSendBtn").disabled=true;
  // Add a placeholder for AI response
  window._incDiagHistory.push({role:"assistant",content:"思考中…"});
  renderDiagnosisChat();
  try {
    const r=await fetch(`${API}/incidents/${window._incDiagId}/diagnose-chat`,{
      method:"POST",headers:{"Content-Type":"application/json"},
      body:JSON.stringify({message:msg,history:window._incDiagHistory.filter(m=>m.content!=="思考中…").slice(0,-1),include_terminal:!!$("incTermCheck")?.checked,stream:true})
    });
    if(!r.ok){ throw new Error("HTTP "+r.status); }
    // SSE streaming: replace placeholder with incremental content
    let streamed=false;
    await readSSEStream(r,
      (delta,fullText)=>{
        if(!streamed){ window._incDiagHistory.pop(); streamed=true; }
        // Update last message in-place
        const last=window._incDiagHistory[window._incDiagHistory.length-1];
        if(last&&last.role==="assistant"){ last.content=fullText; }
        else { window._incDiagHistory.push({role:"assistant",content:fullText}); }
        renderDiagnosisChat();
      },
      (err)=>{
        window._incDiagHistory.pop();
        window._incDiagHistory.push({role:"assistant",content:"❌ "+err});
        renderDiagnosisChat();
      },
      (fullText)=>{
        if(!streamed){
          window._incDiagHistory.pop();
          window._incDiagHistory.push({role:"assistant",content:fullText||"（空回复）"});
        }
        renderDiagnosisChat();
      }
    );
  } catch(e){
    window._incDiagHistory.pop();
    window._incDiagHistory.push({role:"assistant",content:"❌ 网络错误: "+e});
    renderDiagnosisChat();
  }
  el.disabled=false; $("incDiagSendBtn").disabled=false; el.focus();
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
  $("rrLevel").value=r?(r.min_level||""):"critical"; $("rrCategory").value=r?(r.match_category||""):"";
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
// 日志检索分页状态
let LOG_PAGE = 1, LOG_PAGE_SIZE = 50, LOG_TOTAL = 0, LOG_PAGES = 1;
let LAST_LOG_STATS = null; // 缓存上次搜索的统计数据

async function loadLogs(){
  try { if (!SRE_HOSTS.length) SRE_HOSTS=(await fetch(`${API}/hosts`).then(r=>r.json()))||[]; } catch(e){}
  const hs=$("logHost");
  if (hs && hs.options.length<=1) hs.innerHTML=`<option value="">全部主机</option>`+SRE_HOSTS.map(h=>`<option value="${esc(h.id)}">${esc(h.hostname)}</option>`).join("");
  searchLogs();
}

async function searchLogs(page){
  if (page !== undefined) { LOG_PAGE = page; } else { LOG_PAGE = 1; }
  const host=$("logHost").value,level=$("logLevel").value,since=$("logSince").value,kw=$("logKeyword").value.trim();
  const qs=new URLSearchParams();
  if(host)qs.set("host",host); if(level)qs.set("level",level);
  if(since&&since!=="0")qs.set("since_min",since); if(kw)qs.set("q",kw);
  qs.set("page",String(LOG_PAGE)); qs.set("page_size",String(LOG_PAGE_SIZE));
  try {
    const resp=await fetch(`${API}/logs?${qs}`).then(r=>r.json());
    const items=resp.items||[]; LOG_TOTAL=resp.total||0; LOG_PAGES=resp.pages||1;
    LAST_LOG_STATS = resp.stats || null;

    // 渲染统计面板
    renderLogStats(resp.stats, resp.total);

    // 渲染日志列表
    const el=$("logResults");
    if(!items.length){ el.innerHTML=`<div class="empty-line">无匹配日志（被控端需以 --log-paths 指定采集文件）</div>`; renderLogPager(); return; }
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
    renderLogPager();
  } catch(e){ toast("检索失败: "+e,"err"); }
}

// 渲染日志统计面板
function renderLogStats(stats, total){
  const panel=$("logStatsPanel");
  if(!panel) return;
  if(!stats || !total){
    panel.innerHTML=""; panel.style.display="none";
    return;
  }
  panel.style.display="";
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

  // 按主机 Top 5
  let hostHTML="";
  if(topHosts.length){
    hostHTML='<div class="log-stat-row"><span class="log-stat-label">Top 主机：</span>';
    topHosts.forEach(h=>{
      hostHTML+=`<span class="log-stat-chip host" data-host="${esc(h.hostname)}">${esc(h.hostname)} <strong>${h.count}</strong></span>`;
    });
    hostHTML+='</div>';
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
  panel.querySelectorAll(".log-stat-chip.host").forEach(chip=>{
    chip.onclick=()=>{
      const hostSel=$("logHost");
      if(!hostSel) return;
      const hn=chip.dataset.host;
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
function renderLogPager(){
  const pager=$("logPager");
  if(!pager) return;
  if(LOG_TOTAL===0){ pager.innerHTML=""; return; }
  if(LOG_PAGES<=1){ pager.innerHTML=`<span class="pinfo">共 ${LOG_TOTAL} 条</span>`; return; }
  let btns=`<button ${LOG_PAGE===1?"disabled":""} data-lpg="prev">‹</button>`;
  for(let i=1;i<=LOG_PAGES;i++){
    if(i===1||i===LOG_PAGES||Math.abs(i-LOG_PAGE)<=1){
      btns+=`<button class="${i===LOG_PAGE?"active":""}" data-lpg="${i}">${i}</button>`;
    }else if(Math.abs(i-LOG_PAGE)===2){
      btns+=`<span class="pinfo">…</span>`;
    }
  }
  btns+=`<button ${LOG_PAGE===LOG_PAGES?"disabled":""} data-lpg="next">›</button>`;
  btns+=`<span class="pinfo">共 ${LOG_TOTAL} 条 · ${LOG_PAGE}/${LOG_PAGES} 页</span>`;
  pager.innerHTML=btns;

  // 绑定分页按钮事件
  pager.querySelectorAll("[data-lpg]").forEach(b=>{
    b.onclick=()=>{
      const v=b.dataset.lpg;
      if(v==="prev"){ if(LOG_PAGE>1) searchLogs(LOG_PAGE-1); }
      else if(v==="next"){ if(LOG_PAGE<LOG_PAGES) searchLogs(LOG_PAGE+1); }
      else{ const p=parseInt(v); if(p>0&&p<=LOG_PAGES) searchLogs(p); }
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
    <div class="log-diag-head"><span>🔍 诊断结果</span><button class="log-diag-close" onclick="$('logDiagResult').innerHTML=''">✕</button></div>
    <div class="log-diag-summary">${esc(rep.summary||"")}</div>
    ${findings?`<div class="ai-findings">${findings}</div>`:""}
    ${rep.context?`<div class="log-diag-ctx">${esc(rep.context)}</div>`:""}
  </div>`;
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
async function openAIConfig(){
  const tr=$("aiTestResult"); if(tr){ tr.textContent=""; tr.className="ai-test-result"; } // 清除上次遗留的测试结果
  try { const c=await fetch(`${API}/ai/config`).then(r=>r.json());
    $("aiEnabled").checked=!!c.enabled; $("aiEndpoint").value=c.endpoint||""; $("aiKey").value=c.api_key||""; $("aiModel").value=c.model||""; $("aiInterval").value=c.inspect_interval_min||30;
    AI_TERM_ENABLED=!!c.hermes_terminal_enabled; renderAITermState();
  } catch(e){}
  loadAIModels(); // 打开时按当前配置自动获取 provider 模型
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
  const body={enabled,endpoint,api_key:$("aiKey").value,model,inspect_interval_min:parseInt($("aiInterval").value)||30};
  const r=await fetch(`${API}/ai/config`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
  if(r.ok){ $("aiConfigMask").classList.remove("show"); toast("已保存","ok"); } else toast("保存失败","err");
}
// AI 连接测试：通过 SSE 流式验证 Provider 连通性，展示延迟 + 回复摘要
let _aiTestBusy=false;
async function testAIConfig(){
  if(_aiTestBusy) return; // 防重复点击
  const el=$("aiTestResult");
  const endpoint=$("aiEndpoint").value.trim(), model=$("aiModel").value.trim();
  if(!endpoint||!model){ if(el){ el.textContent="✗ 请先填写 Endpoint 和模型"; el.className="ai-test-result err"; } return; }
  _aiTestBusy=true;
  const testBtn=$("aiTestBtn"); if(testBtn) testBtn.disabled=true;
  if(el){ el.textContent="测试中…"; el.className="ai-test-result testing"; }
  const body={enabled:true,endpoint,api_key:$("aiKey").value,model};
  try{
    const r=await fetch(`${API}/ai/test`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
    if(!r.ok){ throw new Error("HTTP "+r.status); }
    // 读取 SSE 流式响应
    let resultMeta=null, reply="", error=null;
    await readSSEStream(r,
      (delta,fullText)=>{ reply=fullText; },  // 累积 delta（不逐字展示，仅收集结果）
      (err)=>{ error=err; },
      (fullText)=>{ if(!reply) reply=fullText; },
      (meta)=>{ resultMeta=meta; }  // 接收 result 元数据
    );
    if(!el) return;
    if(error){
      el.textContent="✗ "+error; el.className="ai-test-result err"; el.style.whiteSpace="pre-wrap";
      return;
    }
    // 优先使用 result 元数据展示
    if(resultMeta && resultMeta.ok){
      let extra="";
      if(resultMeta.provider_hint){
        const labels={openai:"OpenAI 兼容","bailian-compat":"百炼兼容",anthropic:"Anthropic"};
        extra=` · ${labels[resultMeta.provider_hint]||resultMeta.provider_hint}`;
      }
      el.textContent=`✓ 可用${extra} · ${resultMeta.latency_ms||0}ms · ${(resultMeta.reply||"").slice(0,48)}`; el.className="ai-test-result ok";
    } else if(reply){
      el.textContent=`✓ 可用 · ${reply.slice(0,48)}`; el.className="ai-test-result ok";
    } else {
      el.textContent="✗ 未收到有效回复"; el.className="ai-test-result err";
    }
  }catch(e){ if(el){ el.textContent="✗ 请求失败："+e; el.className="ai-test-result err"; } }
  finally{ _aiTestBusy=false; if(testBtn) testBtn.disabled=false; }
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
function renderAIMarkdown(raw){
  if(!raw) return "";
  // 1) 先抽出围栏代码块占位，避免其内部被当作 Markdown/HTML 处理
  const blocks=[];
  let t=raw.replace(/```[a-zA-Z0-9_+#-]*\n?([\s\S]*?)```/g,(m,code)=>{
    blocks.push(code.replace(/\n+$/,""));
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
    const ul=line.match(/^\s*[-*•·]\s+(.+)$/);
    const ol=line.match(/^\s*\d+[.)]\s+(.+)$/);
    if(ul){ if(!inList||listTag!=="ul"){ close(); html+="<ul>"; inList=true; listTag="ul"; } html+="<li>"+ul[1]+"</li>"; }
    else if(ol){ if(!inList||listTag!=="ol"){ close(); html+="<ol>"; inList=true; listTag="ol"; } html+="<li>"+ol[1]+"</li>"; }
    else { close(); html+=(line.trim()==="")?"":("<div>"+line+"</div>"); }
  }
  close();
  html=html.replace(/SNTLCB(\d+)SNTL/g,(m,i)=>"<pre class=\"ai-code\"><code>"+esc(blocks[+i])+"</code></pre>"); // 6) 还原代码块
  return html;
}
// AI 对话消息区：判断是否贴底（供流式时决定要不要自动滚动）
function aiChatStick(){ const log=$("aiChatLog"); return log ? (log.scrollHeight-log.scrollTop-log.clientHeight<80) : true; }
function aiChatToBottom(){ const log=$("aiChatLog"); if(log) log.scrollTop=log.scrollHeight; }
// 统一「AI 对话」——单窗口,后端走 Hermes 自主运维 Agent（能对话 + 自动调用工具,
// 不需要工具时自动退化成纯对话）。模型与 AI 设置共用同一套配置。
let AI_CHAT_SESSION=0;   // Hermes 服务端会话 id（0=新会话）
let AI_CHAT_HISTORY=[];  // 前端侧会话历史 {role,content}：兜底传后端 + 本地记忆
const AI_CHAT_INTRO=`<div class="ai-chat-msg sys">🤖 AI 助手已就绪（自主运维 Agent）。可以闲聊自检，也可直接描述问题让它自动排查，例如：<br>· "当前有哪些主机在线？"　·　"查询主机 web-01 的 CPU 使用率"<br>· "检查 nginx 服务状态"　·　"最近有什么告警？"<br>它会自动识别当前纳管主机并按需调用工具（查指标 / 日志 / 告警 / 诊断 / 修复）。</div>`;
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
}
function renderQueueHint(){
  const el=$("aiChatQueue"); if(!el) return;
  el.textContent=AI_CHAT_QUEUE.length?`⏳ 已排队 ${AI_CHAT_QUEUE.length} 条，将在当前回复完成后依次发送`:"";
}
async function sendAIChat(){
  const inp=$("aiChatInput"); if(!inp) return;
  const msg=inp.value.trim();
  const atts=AI_ATTACHMENTS.slice();
  if(!msg && !atts.length) return; // 无文本且无附件则不发
  if(_aiChatBusy){ // 忙时排队：完成后自动续发（可点终止清空排队）
    AI_CHAT_QUEUE.push({msg,atts});
    inp.value=""; AI_ATTACHMENTS=[]; renderAttachments(); renderQueueHint();
    return;
  }
  inp.value="";
  _aiChatBusy=true; _aiChatAborted=false; setAIChatBusyUI(true);
  _aiChatAbort=(typeof AbortController!=="undefined")?new AbortController():null;
  const imgN=atts.filter(a=>a.kind==="image").length, fileN=atts.filter(a=>a.kind==="file").length;
  const attNote=atts.length?`　<span class="ai-att-note">📎 ${imgN?imgN+" 图 ":""}${fileN?fileN+" 文件":""}</span>`:"";
  const log=$("aiChatLog");
  if(log){ const d=document.createElement("div"); d.className="ai-chat-msg me"; d.innerHTML=esc(msg||"（附件）")+attNote; log.appendChild(d); log.scrollTop=log.scrollHeight; }
  AI_CHAT_HISTORY.push({role:"user",content:msg||"（附件）"});
  AI_ATTACHMENTS=[]; renderAttachments();
  const pending=appendChatMsg("assistant","🤔 思考中…");
  let answer="";
  try{
    const images=atts.filter(a=>a.kind==="image").map(a=>({mime:a.mime,data:a.data}));
    const files=atts.filter(a=>a.kind==="file").map(a=>({name:a.name,text:a.text}));
    const r=await fetch(`${API}/hermes/chat`,{method:"POST",headers:{"Content-Type":"application/json"},
      signal:_aiChatAbort?_aiChatAbort.signal:undefined,
      body:JSON.stringify({message:msg,session_id:AI_CHAT_SESSION,history:AI_CHAT_HISTORY.slice(0,-1),images,files,stream:true})});
    if(!r.ok){ throw new Error("HTTP "+r.status); }
    let streamed=false;
    await readSSEStream(r,
      (delta,fullText)=>{
        const stick=aiChatStick();
        if(!streamed){ if(pending) pending.textContent=""; streamed=true; }
        answer=filterDisplayContent(fullText);
        if(pending) pending.innerHTML=renderAIMarkdown(answer)||"…";
        if(stick) aiChatToBottom();
      },
      (err)=>{ if(pending){ pending.textContent="✗ "+err; pending.classList.add("err"); } },
      (fullText)=>{
        if(!streamed&&pending){
          answer=filterDisplayContent(fullText||"");
          pending.innerHTML=renderAIMarkdown(answer)||"（空回复）";
        }
        aiChatToBottom();
      },
      null,
      (meta)=>{ if(meta&&meta.session_id){ AI_CHAT_SESSION=Number(meta.session_id); } }
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
  const bar=document.createElement("div"); bar.className="ai-msg-tools";
  const btn=document.createElement("button"); btn.textContent="复制"; btn.title="复制回复";
  btn.onclick=()=>{ copyText(rawText); btn.textContent="已复制"; setTimeout(()=>{ btn.textContent="复制"; },1200); };
  bar.appendChild(btn); div.appendChild(bar);
}
// 附件预览渲染（图片/文件 chip，可删除）
function renderAttachments(){
  const box=$("aiChatAttach"); if(!box) return;
  if(!AI_ATTACHMENTS.length){ box.innerHTML=""; box.style.display="none"; return; }
  box.style.display="flex";
  box.innerHTML=AI_ATTACHMENTS.map((a,i)=>`<span class="ai-attach-chip">${a.kind==="image"?"🖼️":"📄"} ${esc(a.name)}<button data-att="${i}" title="移除">✕</button></span>`).join("");
  box.querySelectorAll("[data-att]").forEach(b=>b.onclick=()=>{ AI_ATTACHMENTS.splice(parseInt(b.dataset.att),1); renderAttachments(); });
}
// 选择图片/文件：图片读为 base64（视觉），文本文件读为文本（注入上下文）
function onAIChatFiles(ev){
  const files=Array.from((ev.target&&ev.target.files)||[]);
  for(const f of files){
    if(f.type&&f.type.startsWith("image/")){
      if(AI_ATTACHMENTS.filter(a=>a.kind==="image").length>=4){ if(typeof toast==="function") toast("最多 4 张图片","err"); continue; }
      if(f.size>4*1024*1024){ if(typeof toast==="function") toast(`图片 ${f.name} 超过 4MB`,"err"); continue; }
      const rd=new FileReader();
      rd.onload=()=>{ const s=String(rd.result||""); const c=s.indexOf(","); AI_ATTACHMENTS.push({kind:"image",name:f.name,mime:f.type||"image/png",data:c>=0?s.slice(c+1):s}); renderAttachments(); };
      rd.readAsDataURL(f);
    } else {
      if(f.size>512*1024){ if(typeof toast==="function") toast(`文件 ${f.name} 超过 512KB，请上传关键片段`,"err"); continue; }
      const rd=new FileReader();
      rd.onload=()=>{ AI_ATTACHMENTS.push({kind:"file",name:f.name,text:String(rd.result||"")}); renderAttachments(); };
      rd.readAsText(f);
    }
  }
  if(ev.target) ev.target.value=""; // 允许重复选同一文件
}
safeAddEventListener("logSearchBtn","click",searchLogs);
safeAddEventListener("logKeyword","keydown",e=>{ if(e.key==="Enter") searchLogs(); });
safeAddEventListener("aiInspectBtn","click",runInspect);
safeAddEventListener("aiConfigBtn","click",openAIConfig);
safeAddEventListener("aiConfigSaveBtn","click",saveAIConfig);
safeAddEventListener("aiTestBtn","click",testAIConfig);
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
safeAddEventListener("aiChatAttachBtn","click",()=>{ const f=$("aiChatFile"); if(f) f.click(); });
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

