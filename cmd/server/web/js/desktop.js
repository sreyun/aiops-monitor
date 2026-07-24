/* ---------- 远程桌面（RDP / VNC 反向隧道 + 本机客户端） ---------- */
let DESKTOP_AUTH_PENDING = null; // unused; action lives on TERM_AUTH_PENDING

function openDesktop(id, name) {
  if (!DESKTOP_ENABLED) {
    toast(I18N.t("desktop.disabled"), "err");
    return;
  }
  if (TERM_AUTH_CHECKING) return;
  // Reuse terminal secondary auth; branch in proceedToTerminal via action flag.
  TERM_AUTH_PENDING = { id, name, action: "desktop" };
  checkTerminalAccess();
}

async function doOpenDesktop(id, name) {
  const mask = $("desktopMask");
  const body = $("desktopBody");
  const title = $("desktopTitle");
  if (title) title.textContent = (name || id) + " · " + I18N.t("desktop.title");
  if (body) body.innerHTML = `<div class="empty-line">${I18N.t("ui.loading")}</div>`;
  if (mask) mask.classList.add("show");
  try {
    const r = await fetch(`${API}/hosts/${encodeURIComponent(id)}/desktop`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: "{}"
    });
    const data = await r.json().catch(() => ({}));
    if (r.status === 403 && data.code === "terminal_verify_required") {
      if (mask) mask.classList.remove("show");
      TERM_AUTH_PENDING = { id, name, action: "desktop" };
      TERM_AUTH_VERIFIED = false;
      showTermVerify();
      return;
    }
    if (!r.ok) {
      if (body) body.innerHTML = `<div class="empty-line err">${esc(data.error || I18N.t("toast.update_failed2"))}</div>`;
      return;
    }
    renderDesktopPanel(data);
  } catch (e) {
    if (body) body.innerHTML = `<div class="empty-line err">${esc(String(e))}</div>`;
  }
}

function renderDesktopPanel(d) {
  const body = $("desktopBody");
  if (!body) return;
  const proto = (d.protocol || "vnc").toUpperCase();
  const listenHint = d.service_listening
    ? I18N.t("desktop.service_up")
    : I18N.t("desktop.service_maybe_down");
  const steps = d.protocol === "rdp"
    ? `<ol class="desktop-steps">
        <li>${I18N.t("desktop.step_rdp_1")}</li>
        <li>${I18N.t("desktop.step_rdp_2")}</li>
        <li>${I18N.t("desktop.step_rdp_3")}</li>
      </ol>`
    : `<ol class="desktop-steps">
        <li>${I18N.t("desktop.step_vnc_1")}</li>
        <li>${I18N.t("desktop.step_vnc_2")}</li>
        <li>${I18N.t("desktop.step_vnc_3")}</li>
      </ol>`;
  const actions = d.protocol === "rdp"
    ? `<button class="btn primary" type="button" data-desktop-act="rdp">${I18N.t("desktop.download_rdp")}</button>
       <button class="btn" type="button" data-desktop-act="copy">${I18N.t("desktop.copy_addr")}</button>`
    : `<button class="btn primary" type="button" data-desktop-act="vnc">${I18N.t("desktop.open_vnc")}</button>
       <button class="btn" type="button" data-desktop-act="copy">${I18N.t("desktop.copy_addr")}</button>`;
  body.innerHTML = `
    <div class="desktop-panel">
      <div class="desktop-badge">${esc(proto)} · :${d.target_port} → ${esc(d.connect_addr || "")}</div>
      <div class="desktop-meta">
        <div><span class="k">${I18N.t("desktop.connect_addr")}</span><code class="mono" id="desktopAddr">${esc(d.connect_addr || "")}</code></div>
        <div><span class="k">${I18N.t("desktop.listen_addr")}</span><code class="mono">${esc(d.listen_addr || "")}</code></div>
        <div class="desktop-hint">${esc(listenHint)}</div>
      </div>
      ${steps}
      <div class="desktop-actions">${actions}
        <button class="btn" type="button" data-desktop-act="close-fwd" title="${I18N.t("desktop.close_fwd")}">${I18N.t("desktop.close_fwd")}</button>
      </div>
      <div class="desktop-note">${I18N.t("desktop.note")}</div>
    </div>`;
  body._desktopData = d;
  if (!body.dataset.bound) {
    body.dataset.bound = "1";
    body.addEventListener("click", onDesktopAction);
  }
}

async function onDesktopAction(e) {
  const btn = e.target.closest("[data-desktop-act]");
  if (!btn) return;
  const act = btn.getAttribute("data-desktop-act");
  const d = $("desktopBody") && $("desktopBody")._desktopData;
  if (!d) return;
  if (act === "copy") {
    copyToClipboard(d.connect_addr || "").then(
      () => toast(I18N.t("toast.copied"), "ok"),
      () => toast(I18N.t("toast.copy_failed"), "err")
    );
    return;
  }
  if (act === "rdp" && d.rdp_file) {
    const blob = new Blob([d.rdp_file], { type: "application/x-rdp" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = d.rdp_filename || "remote.rdp";
    document.body.appendChild(a);
    a.click();
    setTimeout(() => { URL.revokeObjectURL(a.href); a.remove(); }, 1000);
    toast(I18N.t("desktop.rdp_saved"), "ok");
    return;
  }
  if (act === "vnc" && d.vnc_url) {
    window.location.href = d.vnc_url;
    return;
  }
  if (act === "close-fwd" && d.forward_id) {
    try {
      const r = await fetch(`${API}/forward/${encodeURIComponent(d.forward_id)}`, { method: "DELETE", credentials: "include" });
      if (r.ok) {
        toast(I18N.t("desktop.fwd_closed"), "ok");
        const mask = $("desktopMask");
        if (mask) mask.classList.remove("show");
      } else toast(I18N.t("toast.delete_failed"), "err");
    } catch (err) { toast(String(err), "err"); }
  }
}

function closeDesktopMask() {
  const mask = $("desktopMask");
  if (mask) mask.classList.remove("show");
}
