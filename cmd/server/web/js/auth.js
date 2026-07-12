/* ============================================================
   AIOps Monitor · auth.js — 认证、用户管理、MFA、账户恢复、个人信息
   依赖：core.js（$, esc, toast, withLoading, pwPolicyOK, safeAddEventListener, AIOps 命名空间）
   加载顺序：在 core.js / render.js / charts.js / terminal.js 之后，automation.js 之前
   ============================================================ */
"use strict";

window.AIOps = window.AIOps || {};

/* ---------- 账户 / 角色 ---------- */
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

/* ---------- 个人信息 ---------- */
async function openProfile(tab) {
  try {
    const me = await fetch(`${API}/me`).then(r => r.json());
    $("pfUsername").value = me.username || "";
    $("pfDisplay").value = me.display_name || "";
    $("pfEmail").value = me.email || "";
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
      body: JSON.stringify({ username: uname, display_name: $("pfDisplay").value.trim(), email: $("pfEmail").value.trim() })
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

/* ---------- 登录表单处理 ---------- */
safeAddEventListener("loginForm", "submit", async e => {
  e.preventDefault();
  const loginErrEl = $("loginErr");
  if (loginErrEl) loginErrEl.textContent = "";
  const submitBtn = e.target.querySelector('button[type="submit"]');
  await withLoading(submitBtn, async () => {
    try {
      const codeEl = $("loginCode");
      const r = await fetchWithTimeout(`${API}/login`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          username: $("loginUser").value.trim(),
          password: $("loginPass").value,
          code: codeEl ? codeEl.value.trim() : ""
        })
      }, 15000);
      const j = await r.json().catch(() => ({}));
      if (r.ok && j.mfa_required) {
        const f = $("loginCodeField"); if (f) f.style.display = "";
        if (codeEl) codeEl.focus();
        if (loginErrEl) loginErrEl.textContent = I18N.t("mfa.login_totp");
      }
      else if (r.ok && j.require_mfa_setup) {
        openMfaSetup(true);
      }
      else if (r.ok && j.must_change_password) {
        // v5.4.0: admin password was reset — force password change
        let user;
        try {
          user = await fetchWithTimeout(`${API}/me`, {}, 10000).then(x => x.json());
        } catch (_) {
          user = { username: $("loginUser").value.trim(), display_name: "" };
        }
        setUser(user);
        $("loginView").classList.remove("show");
        startApp();
        // 强制进入「安全初始化」弹窗：需修改用户名 + 密码后方可进入控制台
        setTimeout(() => openInitSetup(), 300);
      }
      else if (r.ok) {
        // Post-login /me fetch: wrap in try/catch so a transient network
        // hiccup doesn't leave the user stuck on the login page after
        // successful authentication.
        let user;
        try {
          user = await fetchWithTimeout(`${API}/me`, {}, 10000).then(x => x.json());
        } catch (_) {
          // Login succeeded but /me failed — proceed anyway, the next poll
          // will populate user info. Better than showing an error after
          // the user already typed their credentials correctly.
          user = { username: $("loginUser").value.trim(), display_name: "" };
        }
        setUser(user);
        const loginViewEl = $("loginView");
        if (loginViewEl) loginViewEl.classList.remove("show");
        startApp();
      }
      else {
        if (loginErrEl) loginErrEl.textContent = j.error || I18N.t("toast.login_failed");
      }
    } catch (err) {
      // Distinguish AbortError (timeout) from generic network errors so
      // mobile users see a helpful message instead of "TypeError: Failed to fetch".
      const msg = err.name === "AbortError"
        ? I18N.t("toast.login_timeout")
        : I18N.t("toast.login_network_error");
      if (loginErrEl) loginErrEl.textContent = msg;
    }
  });
});

// 导出到 AIOps 命名空间
Object.assign(window.AIOps, {
  fetchWithTimeout, initAuth,
  setUser, roleLabel, canWrite, isAdmin,
  openInitSetup, submitInitSetup,
  openProfile, switchProfileTab, saveProfile, changePassword,
  loadTermPwdStatus, toggleTermPwdChange, submitTermPwdChange,
  renderMfaState, openMfaSetup, openMfaDisable, openMfaEmailUnbind,
  openUsers, loadUsers, openUserEdit, usersAction,
  openRecoverUser, openRecoverPass, showRecoverFlow,
  logout,
  get CUR_ROLE() { return CUR_ROLE; }, set CUR_ROLE(v) { CUR_ROLE = v; },
  get MFA_ENABLED() { return MFA_ENABLED; }, set MFA_ENABLED(v) { MFA_ENABLED = v; },
  get PROFILE_TAB() { return PROFILE_TAB; }, set PROFILE_TAB(v) { PROFILE_TAB = v; },
  get PROFILE_USERS_LOADED() { return PROFILE_USERS_LOADED; }, set PROFILE_USERS_LOADED(v) { PROFILE_USERS_LOADED = v; },
});