/* ---------- 远程桌面：推流 · 多屏 · 剪贴板 · H264 · 拖拽 · 回放 ---------- */
let DESK_WS = null;
let DESK_HOST = null;
let DESK_META = { w: 1920, h: 1080, monitors: [], h264: false, viewOnly: false };
let DESK_QUALITY = { scale: 0.5, quality: 55, fps: 8, codec: "jpeg", monitor: 0 };
let DESK_DOWNLOAD = null;
let DESK_MSE = null; // { mediaSource, sourceBuffer, queue, video }
let DESK_GOT_FRAME = false;
let DESK_PHASE = "idle"; // idle|connecting|waiting_agent|streaming|error|closed

function openDesktop(id, name) {
  if (!DESKTOP_ENABLED) { toast(I18N.t("desktop.disabled"), "err"); return; }
  if (TERM_AUTH_CHECKING) return;
  TERM_AUTH_PENDING = { id, name, action: "desktop" };
  checkTerminalAccess();
}

async function doOpenDesktop(id, name) {
  const mask = $("desktopMask");
  const title = $("desktopTitle");
  if (title) title.textContent = (name || id) + " · " + I18N.t("desktop.title");
  if (mask) mask.classList.add("show");
  DESK_GOT_FRAME = false;
  DESK_PHASE = "connecting";
  DESK_META = { w: 1920, h: 1080, monitors: [], h264: false, viewOnly: false };
  DESK_QUALITY = { scale: 0.5, quality: 55, fps: 8, codec: "jpeg", monitor: 0 };
  renderDesktopShell(id, name);
  try {
    const r = await fetch(`${API}/hosts/${encodeURIComponent(id)}/desktop`, {
      method: "POST", credentials: "include",
      headers: { "Content-Type": "application/json" }, body: "{}"
    });
    const data = await r.json().catch(() => ({}));
    if (r.status === 403 && data.code === "terminal_verify_required") {
      closeDesktopMask();
      TERM_AUTH_PENDING = { id, name, action: "desktop" };
      TERM_AUTH_VERIFIED = false;
      showTermVerify();
      return;
    }
    if (!r.ok) {
      setDesktopStatus(esc(data.error || I18N.t("toast.update_failed2")), true);
      setDeskPlaceholder(I18N.t("desktop.error"), data.error || "");
      return;
    }
    connectDesktopWS(id, name);
  } catch (e) {
    setDesktopStatus(esc(String(e)), true);
    setDeskPlaceholder(I18N.t("desktop.error"), String(e));
  }
}

function renderDesktopShell(id, name) {
  const body = $("desktopBody");
  if (!body) return;
  body.innerHTML = `
    <div class="desk-layout">
      <div class="desk-main">
        <div class="desk-toolbar">
          <div class="desk-status-wrap">
            <span class="desk-dot" id="deskDot"></span>
            <span class="desk-status" id="deskStatus">${esc(I18N.t("desktop.connecting"))}</span>
          </div>
          <div class="desk-tools">
            <label class="desk-q-label"><span>${esc(I18N.t("desktop.monitor"))}</span>
              <select id="deskMonitor" class="desk-select"><option value="0">—</option></select>
            </label>
            <label class="desk-q-label"><span>${esc(I18N.t("desktop.quality"))}</span>
              <select id="deskQuality" class="desk-select">
                <option value="fast">${esc(I18N.t("desktop.q_fast"))}</option>
                <option value="balanced" selected>${esc(I18N.t("desktop.q_balanced"))}</option>
                <option value="clear">${esc(I18N.t("desktop.q_clear"))}</option>
              </select>
            </label>
            <label class="desk-q-label"><span>${esc(I18N.t("desktop.codec"))}</span>
              <select id="deskCodec" class="desk-select">
                <option value="jpeg" selected>JPEG</option>
                <option value="h264">H.264</option>
              </select>
            </label>
            <button type="button" class="btn sm" id="deskClipSend" title="${esc(I18N.t("desktop.clip_send"))}">📋</button>
            <button type="button" class="btn sm" id="deskFullscreen" title="${esc(I18N.t("desktop.fullscreen"))}">⛶</button>
            <button type="button" class="btn sm" id="deskSessions">${esc(I18N.t("desktop.sessions"))}</button>
            <button type="button" class="btn sm" id="deskDisconnect">${esc(I18N.t("desktop.disconnect"))}</button>
          </div>
        </div>
        <div class="desk-stage" id="deskStage">
          <div class="desk-placeholder" id="deskPlaceholder">
            <div class="desk-spinner" aria-hidden="true"></div>
            <div class="desk-ph-title" id="deskPhTitle">${esc(I18N.t("desktop.connecting"))}</div>
            <div class="desk-ph-sub" id="deskPhSub">${esc(I18N.t("desktop.wait_hint"))}</div>
          </div>
          <canvas id="deskCanvas" tabindex="0" style="display:none"></canvas>
          <video id="deskVideo" playsinline autoplay muted style="display:none"></video>
          <div class="desk-drop-hint" id="deskDropHint">${esc(I18N.t("desktop.drop_hint"))}</div>
        </div>
      </div>
      <aside class="desk-side" id="deskSide">
        <div class="desk-side-title">${esc(I18N.t("desktop.files"))}</div>
        <div class="desk-side-hint">${esc(I18N.t("desktop.files_hint"))}</div>
        <label class="desk-field">${esc(I18N.t("desktop.upload_path"))}
          <input type="text" id="deskUploadPath" class="desk-input" placeholder="C:\\Temp\\ 或 /tmp/" autocomplete="off">
        </label>
        <div class="desk-row">
          <input type="file" id="deskFileInput" hidden>
          <button type="button" class="btn primary sm" id="deskUploadBtn">${esc(I18N.t("desktop.upload"))}</button>
        </div>
        <label class="desk-field">${esc(I18N.t("desktop.download_path"))}
          <input type="text" id="deskDownloadPath" class="desk-input" placeholder="${esc(I18N.t("desktop.download_ph"))}" autocomplete="off">
        </label>
        <button type="button" class="btn sm" id="deskDownloadBtn">${esc(I18N.t("desktop.download"))}</button>
        <div class="desk-xfer" id="deskXferLog"></div>
        <div class="desk-side-title" style="margin-top:12px">${esc(I18N.t("desktop.clipboard"))}</div>
        <textarea id="deskClipBox" class="desk-clip" rows="4" placeholder="${esc(I18N.t("desktop.clip_ph"))}"></textarea>
        <button type="button" class="btn sm" id="deskClipApply">${esc(I18N.t("desktop.clip_to_remote"))}</button>
      </aside>
    </div>
    <div class="desk-replay" id="deskReplayPane" hidden>
      <div class="desk-replay-bar">
        <span id="deskReplayTitle">${esc(I18N.t("desktop.sessions"))}</span>
        <button type="button" class="btn sm" id="deskReplayClose">${esc(I18N.t("ui.close","关闭"))}</button>
      </div>
      <div id="deskSessionsList" class="desk-sessions-list"></div>
      <canvas id="deskReplayCanvas" style="max-width:100%;background:#000;display:none"></canvas>
    </div>`;
  if (!body.dataset.deskBound) {
    body.dataset.deskBound = "1";
    body.addEventListener("click", onDesktopUIClick);
    body.addEventListener("change", onDesktopUIChange);
  }
  const stage = $("deskStage");
  if (stage) {
    stage.dataset.dnd = "1";
    stage.ondragover = e => { e.preventDefault(); stage.classList.add("drag"); };
    stage.ondragleave = () => stage.classList.remove("drag");
    stage.ondrop = onDeskDrop;
  }
  DESK_HOST = { id, name };
  setDeskDot("connecting");
}

function setDeskDot(phase) {
  const dot = $("deskDot");
  if (!dot) return;
  dot.className = "desk-dot " + (phase || "");
}

function setDeskPlaceholder(title, sub) {
  const ph = $("deskPlaceholder");
  const t = $("deskPhTitle");
  const s = $("deskPhSub");
  if (ph) ph.style.display = "";
  if (t) t.textContent = title || "";
  if (s) s.textContent = sub || "";
}

function hideDeskPlaceholder() {
  const ph = $("deskPlaceholder");
  if (ph) ph.style.display = "none";
}

function setDesktopStatus(msg, isErr) {
  const el = $("deskStatus");
  if (!el) return;
  el.textContent = msg;
  el.classList.toggle("err", !!isErr);
  if (isErr) setDeskDot("error");
}

function qualityPreset(name) {
  if (name === "fast") return { scale: 0.35, quality: 40, fps: 10 };
  if (name === "clear") return { scale: 0.75, quality: 75, fps: 6 };
  return { scale: 0.5, quality: 55, fps: 8 };
}

function sendDeskQuality() {
  if (!DESK_WS || DESK_WS.readyState !== 1) return;
  const payload = new TextEncoder().encode(JSON.stringify(DESK_QUALITY));
  const buf = new Uint8Array(1 + payload.length);
  buf[0] = "Q".charCodeAt(0);
  buf.set(payload, 1);
  DESK_WS.send(buf);
}

function fillMonitorSelect(mons) {
  const sel = $("deskMonitor");
  if (!sel) return;
  sel.innerHTML = (mons || []).map(m =>
    `<option value="${m.id}">${esc(m.name || ("#" + m.id))} (${m.width}×${m.height})${m.primary ? " ★" : ""}</option>`
  ).join("") || `<option value="0">—</option>`;
  if (mons && mons.length && (!DESK_QUALITY.monitor || !mons.some(m => m.id === DESK_QUALITY.monitor))) {
    const p = mons.find(m => m.primary) || mons[0];
    DESK_QUALITY.monitor = p.id;
  }
  sel.value = String(DESK_QUALITY.monitor || (mons && mons[0] && mons[0].id) || 0);
}

function connectDesktopWS(id, name) {
  closeDesktopWS();
  DESK_GOT_FRAME = false;
  DESK_PHASE = "waiting_agent";
  setDesktopStatus(I18N.t("desktop.waiting_agent"), false);
  setDeskPlaceholder(I18N.t("desktop.waiting_agent"), I18N.t("desktop.wait_hint"));
  setDeskDot("waiting");
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const ws = new WebSocket(`${proto}//${location.host}/api/v1/hosts/${encodeURIComponent(id)}/desktop/ws`);
  ws.binaryType = "arraybuffer";
  DESK_WS = ws;
  const canvas = $("deskCanvas");
  // Decode one JPEG at a time and retain only the newest waiting frame. The old
  // implementation shared one object URL across every in-flight Image; a newer
  // frame revoked the URL still being decoded, while an older onload could revoke
  // the newer URL. At normal frame rates that race left the canvas at its default
  // 300×150 black surface even though the UI reported "Connected".
  // Prefer createImageBitmap(Blob) so decode does not depend on blob: URLs
  // (a CSP img-src without blob: previously blocked every frame and surfaced as
  // "无法解码的 JPEG 画面"). Fall back to Image + object URL when unavailable.
  let jpegPending = null;
  let jpegDecoding = false;
  let jpegDecodeFailures = 0;

  const drawNextJPEG = () => {
    if (jpegDecoding || !jpegPending || DESK_WS !== ws) return;
    const blob = jpegPending;
    jpegPending = null;
    jpegDecoding = true;
    const finish = () => {
      jpegDecoding = false;
      if (jpegPending && DESK_WS === ws) drawNextJPEG();
    };
    const fail = () => {
      jpegDecodeFailures++;
      if (jpegDecodeFailures === 3 && DESK_WS === ws) {
        DESK_PHASE = "error";
        setDesktopStatus(I18N.t("desktop.jpeg_decode_failed"), true);
        setDeskPlaceholder(I18N.t("desktop.error"), I18N.t("desktop.jpeg_decode_failed"));
        setDeskDot("error");
      }
      finish();
    };
    const paint = (src, w, h) => {
      const ctx = canvas && canvas.getContext("2d");
      if (ctx && w > 0 && h > 0 && DESK_WS === ws) {
        if (canvas.width !== w || canvas.height !== h) {
          canvas.width = w;
          canvas.height = h;
        }
        ctx.drawImage(src, 0, 0);
        if (typeof src.close === "function") {
          try { src.close(); } catch (e) {}
        }
        jpegDecodeFailures = 0;
        markDeskStreaming();
        showDeskCanvas(true);
      }
      finish();
    };
    if (typeof createImageBitmap === "function") {
      createImageBitmap(blob).then((bmp) => paint(bmp, bmp.width, bmp.height), fail);
      return;
    }
    const frameURL = URL.createObjectURL(blob);
    const img = new Image();
    img.onload = () => {
      URL.revokeObjectURL(frameURL);
      paint(img, img.naturalWidth, img.naturalHeight);
    };
    img.onerror = () => {
      URL.revokeObjectURL(frameURL);
      fail();
    };
    img.src = frameURL;
  };

  ws.onopen = () => {
    // Do NOT claim fully connected until we get agent meta / first frame.
    setDesktopStatus(I18N.t("desktop.waiting_agent"), false);
    sendDeskQuality();
  };
  ws.onclose = () => {
    const prev = DESK_PHASE;
    DESK_PHASE = "closed";
    // Keep the real agent/server error — do not overwrite with bare "已断开".
    if (prev !== "error" && prev !== "streaming") {
      setDesktopStatus(I18N.t("desktop.disconnected"), true);
      if (!DESK_GOT_FRAME) {
        setDeskPlaceholder(I18N.t("desktop.disconnected"), I18N.t("desktop.wait_hint"));
      }
    } else if (prev === "streaming") {
      setDesktopStatus(I18N.t("desktop.disconnected"), true);
    }
    setDeskDot("error");
    unbindDesktopInput(canvas);
    closeDeskMSE();
  };
  ws.onerror = () => {
    DESK_PHASE = "error";
    setDesktopStatus(I18N.t("desktop.error"), true);
    setDeskDot("error");
    setDeskPlaceholder(I18N.t("desktop.error"), I18N.t("desktop.wait_hint"));
  };
  ws.onmessage = (ev) => {
    if (!(ev.data instanceof ArrayBuffer) || ev.data.byteLength < 1) return;
    const u8 = new Uint8Array(ev.data);
    const typ = String.fromCharCode(u8[0]);
    const payload = u8.subarray(1);
    if (typ === "S") {
      try {
        const meta = JSON.parse(new TextDecoder().decode(payload));
        if (meta.phase === "waiting_agent") {
          DESK_PHASE = "waiting_agent";
          setDesktopStatus(I18N.t("desktop.waiting_agent"), false);
          setDeskPlaceholder(I18N.t("desktop.waiting_agent"), I18N.t("desktop.wait_hint"));
          setDeskDot("waiting");
          return;
        }
        if (meta.phase === "agent_up" && !DESK_GOT_FRAME) {
          setDesktopStatus(I18N.t("desktop.agent_up"), false);
          setDeskPlaceholder(I18N.t("desktop.agent_up"), I18N.t("desktop.streaming_hint"));
          setDeskDot("waiting");
        }
        if (meta.w) DESK_META.w = meta.w;
        if (meta.h) DESK_META.h = meta.h;
        if (meta.h264 != null) DESK_META.h264 = !!meta.h264;
        if (meta.view_only != null) DESK_META.viewOnly = !!meta.view_only;
        if (Array.isArray(meta.monitors)) {
          DESK_META.monitors = meta.monitors;
          fillMonitorSelect(meta.monitors);
        }
        const codecSel = $("deskCodec");
        if (codecSel) {
          // Reflect the agent's capability: when H.264 is unavailable (e.g. the
          // Windows secure-desktop worker forces GDI capture so the lock screen
          // isn't a black ffmpeg frame), force JPEG and disable the option so it
          // can't be re-selected back into a black stream.
          const h264opt = codecSel.querySelector('option[value="h264"]');
          if (meta.h264 === false) {
            codecSel.value = "jpeg";
            DESK_QUALITY.codec = "jpeg";
            if (h264opt) h264opt.disabled = true;
          } else if (h264opt) {
            h264opt.disabled = false;
          }
        }
        // Agent-preferred codec (e.g. macOS: continuous H.264 vastly outperforms
        // per-frame screencapture). Auto-switch once, before the first frame.
        if (meta.prefer === "h264" && meta.h264 && !DESK_GOT_FRAME && DESK_QUALITY.codec !== "h264") {
          DESK_QUALITY.codec = "h264";
          if (codecSel) codecSel.value = "h264";
          sendDeskQuality();
        }
        if (meta.error) {
          DESK_PHASE = "error";
          setDesktopStatus(meta.error, true);
          setDeskPlaceholder(I18N.t("desktop.error"), meta.error);
          setDeskDot("error");
        }
        if (DESK_META.viewOnly && DESK_PHASE !== "error") {
          setDesktopStatus(I18N.t("desktop.view_only"), false);
        }
      } catch (e) {}
      return;
    }
    if (typ === "K" && canvas) {
      // Blob copies the frame out of the WebSocket event buffer. Replacing the
      // pending blob intentionally drops stale frames so decode latency cannot
      // grow without bound on a busy/high-DPI desktop.
      jpegPending = new Blob([payload.slice()], { type: "image/jpeg" });
      drawNextJPEG();
      return;
    }
    if (typ === "H") {
      markDeskStreaming();
      showDeskCanvas(false);
      appendDeskH264(payload);
      return;
    }
    if (typ === "C") {
      try {
        const j = JSON.parse(new TextDecoder().decode(payload));
        const box = $("deskClipBox");
        if (box && j.text != null) box.value = j.text;
        if (j.text && navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(j.text).catch(() => {});
        }
      } catch (e) {}
      return;
    }
    if (typ === "F") { handleDeskFileInfo(payload); return; }
    if (typ === "D") { if (DESK_DOWNLOAD) DESK_DOWNLOAD.chunks.push(payload.slice(0)); return; }
    if (typ === "E") {
      if (payload.length > 0) {
        try {
          const j = JSON.parse(new TextDecoder().decode(payload));
          if (j.error) {
            const isWarn = j.level === "warn";
            setDesktopStatus(j.error, !isWarn);
            // Warn diagnostics (blank capture / no_frame watchdog) must still be
            // visible on the canvas — otherwise a black JPEG stream looks like a
            // successful connection with no explanation.
            if (!DESK_GOT_FRAME || isWarn) {
              setDeskPlaceholder(isWarn ? I18N.t("desktop.warn") : I18N.t("desktop.error"), j.error);
            }
            if (!isWarn) {
              setDeskDot("error");
              DESK_PHASE = "error";
            } else {
              setDeskDot("warn");
            }
            return;
          }
        } catch (e) {
          const msg = new TextDecoder().decode(payload);
          DESK_PHASE = "error";
          setDesktopStatus(msg, true);
          setDeskDot("error");
          if (!DESK_GOT_FRAME) setDeskPlaceholder(I18N.t("desktop.error"), msg);
          return;
        }
      }
      if (DESK_DOWNLOAD) finishDeskDownload();
    }
  };
}

function markDeskStreaming() {
  if (!DESK_GOT_FRAME) {
    DESK_GOT_FRAME = true;
    DESK_PHASE = "streaming";
    hideDeskPlaceholder();
    setDeskDot("on");
    const msg = DESK_META.viewOnly
      ? I18N.t("desktop.view_only")
      : I18N.t("desktop.connected");
    setDesktopStatus(msg, false);
    const canvas = $("deskCanvas");
    if (canvas) { canvas.focus(); bindDesktopInput(canvas); }
  }
}

function showDeskCanvas(useCanvas) {
  const canvas = $("deskCanvas");
  const video = $("deskVideo");
  if (canvas) canvas.style.display = useCanvas ? "block" : "none";
  if (video) video.style.display = useCanvas ? "none" : "block";
}

function closeDeskMSE() {
  if (DESK_MSE && DESK_MSE.mediaSource && DESK_MSE.mediaSource.readyState === "open") {
    try { DESK_MSE.mediaSource.endOfStream(); } catch (e) {}
  }
  DESK_MSE = null;
  const video = $("deskVideo");
  if (video) { video.removeAttribute("src"); video.load(); }
}

function appendDeskH264(chunk) {
  const video = $("deskVideo");
  if (!video || typeof MediaSource === "undefined") return;
  if (!DESK_MSE) {
    const ms = new MediaSource();
    DESK_MSE = { mediaSource: ms, sourceBuffer: null, queue: [], video };
    video.src = URL.createObjectURL(ms);
    ms.addEventListener("sourceopen", () => {
      try {
        const sb = ms.addSourceBuffer('video/mp4; codecs="avc1.42E01E"');
        DESK_MSE.sourceBuffer = sb;
        sb.mode = "sequence";
        sb.addEventListener("updateend", flushDeskMSE);
        flushDeskMSE();
      } catch (e) {
        setDesktopStatus(I18N.t("desktop.h264_unsupported") + ": " + e, true);
        DESK_QUALITY.codec = "jpeg";
        const cs = $("deskCodec"); if (cs) cs.value = "jpeg";
        sendDeskQuality();
      }
    });
  }
  DESK_MSE.queue.push(chunk.buffer.slice(chunk.byteOffset, chunk.byteOffset + chunk.byteLength));
  flushDeskMSE();
}

function flushDeskMSE() {
  const m = DESK_MSE;
  if (!m || !m.sourceBuffer || m.sourceBuffer.updating || !m.queue.length) return;
  try { m.sourceBuffer.appendBuffer(m.queue.shift()); } catch (e) { m.queue = []; }
}

function handleDeskFileInfo(payload) {
  let meta = {};
  try { meta = JSON.parse(new TextDecoder().decode(payload)); } catch (e) { return; }
  const log = $("deskXferLog");
  if (meta.type === "upload_ack") {
    const ok = meta.status === "ok";
    toast((ok ? I18N.t("desktop.upload_ok") : I18N.t("desktop.upload_fail")) + (meta.filename ? ": " + meta.filename : "") + (meta.message ? " — " + meta.message : ""), ok ? "ok" : "err");
    if (log) log.textContent = (ok ? "↑ OK " : "↑ ERR ") + (meta.filename || "");
    return;
  }
  if (meta.type === "download_meta" || meta.type === "download_start") {
    DESK_DOWNLOAD = { filename: meta.filename || "download.bin", size: meta.size || 0, chunks: [] };
    toast(I18N.t("desktop.downloading") + ": " + DESK_DOWNLOAD.filename, "info");
    if (log) log.textContent = "↓ " + DESK_DOWNLOAD.filename;
    return;
  }
  if (meta.type === "download_error") {
    toast(I18N.t("desktop.download_fail") + (meta.message ? ": " + meta.message : ""), "err");
    DESK_DOWNLOAD = null;
  }
}

function finishDeskDownload() {
  const dl = DESK_DOWNLOAD; DESK_DOWNLOAD = null;
  if (!dl) return;
  const blob = new Blob(dl.chunks);
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob); a.download = dl.filename;
  document.body.appendChild(a); a.click();
  setTimeout(() => { URL.revokeObjectURL(a.href); a.remove(); }, 1000);
  toast(I18N.t("desktop.download_ok") + ": " + dl.filename, "ok");
}

let _deskInputBound = false;
function bindDesktopInput(canvas) {
  if (!canvas || _deskInputBound) return;
  _deskInputBound = true;
  canvas.addEventListener("mousemove", onDeskMouseMove);
  canvas.addEventListener("mousedown", onDeskMouseDown);
  canvas.addEventListener("mouseup", onDeskMouseUp);
  canvas.addEventListener("contextmenu", onDeskContext);
  canvas.addEventListener("wheel", onDeskWheel, { passive: false });
  canvas.addEventListener("keydown", onDeskKeyDown);
  canvas.addEventListener("keyup", onDeskKeyUp);
}
function unbindDesktopInput(canvas) {
  if (!canvas || !_deskInputBound) return;
  _deskInputBound = false;
  canvas.removeEventListener("mousemove", onDeskMouseMove);
  canvas.removeEventListener("mousedown", onDeskMouseDown);
  canvas.removeEventListener("mouseup", onDeskMouseUp);
  canvas.removeEventListener("contextmenu", onDeskContext);
  canvas.removeEventListener("wheel", onDeskWheel);
  canvas.removeEventListener("keydown", onDeskKeyDown);
  canvas.removeEventListener("keyup", onDeskKeyUp);
}

function deskNormXY(ev, el) {
  const rect = el.getBoundingClientRect();
  const x = (ev.clientX - rect.left) / Math.max(1, rect.width);
  const y = (ev.clientY - rect.top) / Math.max(1, rect.height);
  return {
    x: Math.min(1, Math.max(0, x)) * (DESK_META.w || rect.width),
    y: Math.min(1, Math.max(0, y)) * (DESK_META.h || rect.height)
  };
}
function deskSendJSON(typ, obj) {
  if (!DESK_WS || DESK_WS.readyState !== 1) return;
  const payload = new TextEncoder().encode(JSON.stringify(obj));
  const buf = new Uint8Array(1 + payload.length);
  buf[0] = typ.charCodeAt(0); buf.set(payload, 1); DESK_WS.send(buf);
}
let _deskLastMove = 0;
function onDeskMouseMove(ev) {
  const now = Date.now(); if (now - _deskLastMove < 33) return; _deskLastMove = now;
  const p = deskNormXY(ev, ev.currentTarget);
  deskSendJSON("M", { x: p.x, y: p.y, action: "move", btn: 0 });
}
function onDeskMouseDown(ev) {
  ev.preventDefault(); ev.currentTarget.focus();
  const p = deskNormXY(ev, ev.currentTarget);
  const btn = ev.button === 2 ? 2 : ev.button === 1 ? 3 : 1;
  deskSendJSON("M", { x: p.x, y: p.y, action: "down", btn });
}
function onDeskMouseUp(ev) {
  ev.preventDefault();
  const p = deskNormXY(ev, ev.currentTarget);
  const btn = ev.button === 2 ? 2 : ev.button === 1 ? 3 : 1;
  deskSendJSON("M", { x: p.x, y: p.y, action: "up", btn });
}
function onDeskContext(ev) { ev.preventDefault(); }
function onDeskWheel(ev) { ev.preventDefault(); deskSendJSON("W", { delta: ev.deltaY > 0 ? -1 : 1 }); }
function onDeskKeyDown(ev) { ev.preventDefault(); deskSendJSON("B", { down: true, key: ev.key, code: ev.code, vk: 0 }); }
function onDeskKeyUp(ev) { ev.preventDefault(); deskSendJSON("B", { down: false, key: ev.key, code: ev.code, vk: 0 }); }

function onDesktopUIClick(e) {
  const t = e.target;
  if (t.id === "deskDisconnect" || t.closest("#deskDisconnect")) { closeDesktopMask(); return; }
  if (t.id === "deskFullscreen" || t.closest("#deskFullscreen")) {
    const stage = $("deskStage"); if (stage && stage.requestFullscreen) stage.requestFullscreen().catch(() => {}); return;
  }
  if (t.id === "deskUploadBtn" || t.closest("#deskUploadBtn")) { const inp = $("deskFileInput"); if (inp) inp.click(); return; }
  if (t.id === "deskDownloadBtn" || t.closest("#deskDownloadBtn")) { deskStartDownload(); return; }
  if (t.id === "deskClipApply" || t.closest("#deskClipApply") || t.id === "deskClipSend" || t.closest("#deskClipSend")) {
    deskPushClipboard(); return;
  }
  if (t.id === "deskSessions" || t.closest("#deskSessions")) { openDeskSessions(); return; }
  if (t.id === "deskReplayClose" || t.closest("#deskReplayClose")) {
    const p = $("deskReplayPane"); if (p) p.hidden = true; return;
  }
  const play = t.closest("[data-desk-replay]");
  if (play) { playDeskReplay(play.getAttribute("data-desk-replay")); }
}

function onDesktopUIChange(e) {
  if (e.target && e.target.id === "deskQuality") {
    const p = qualityPreset(e.target.value);
    DESK_QUALITY = { ...DESK_QUALITY, ...p };
    sendDeskQuality(); return;
  }
  if (e.target && e.target.id === "deskCodec") {
    DESK_QUALITY.codec = e.target.value === "h264" ? "h264" : "jpeg";
    if (DESK_QUALITY.codec === "jpeg") closeDeskMSE();
    sendDeskQuality(); return;
  }
  if (e.target && e.target.id === "deskMonitor") {
    DESK_QUALITY.monitor = parseInt(e.target.value, 10) || 0;
    deskSendJSON("N", { id: DESK_QUALITY.monitor });
    sendDeskQuality(); return;
  }
  if (e.target && e.target.id === "deskFileInput" && e.target.files && e.target.files[0]) {
    deskStartUpload(e.target.files[0]); e.target.value = "";
  }
}

function onDeskDrop(e) {
  e.preventDefault();
  const stage = $("deskStage"); if (stage) stage.classList.remove("drag");
  const f = e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files[0];
  if (f) deskStartUpload(f);
}

function deskPushClipboard() {
  const box = $("deskClipBox");
  const send = (text) => {
    if (!text) return;
    if (box) box.value = text;
    deskSendJSON("C", { text });
    toast(I18N.t("desktop.clip_sent"), "ok");
  };
  if (box && box.value) { send(box.value); return; }
  if (navigator.clipboard && navigator.clipboard.readText) {
    navigator.clipboard.readText().then(send).catch(() => toast(I18N.t("desktop.clip_fail"), "err"));
  }
}

async function deskStartUpload(file) {
  if (!DESK_WS || DESK_WS.readyState !== 1) { toast(I18N.t("desktop.not_connected"), "err"); return; }
  if (file.size > 100 * 1024 * 1024) { toast(I18N.t("term.file_too_large"), "err"); return; }
  let path = (($("deskUploadPath") && $("deskUploadPath").value) || "").trim();
  if (!path) path = file.name;
  else if (path.endsWith("/") || path.endsWith("\\")) path = path + file.name;
  const meta = new TextEncoder().encode(JSON.stringify({ filename: file.name, size: file.size, target_path: path }));
  const fbuf = new Uint8Array(1 + meta.length); fbuf[0] = "f".charCodeAt(0); fbuf.set(meta, 1); DESK_WS.send(fbuf);
  const chunkSize = 48 * 1024; let offset = 0;
  while (offset < file.size) {
    if (DESK_WS.readyState !== 1) { toast(I18N.t("desktop.upload_fail"), "err"); return; }
    const slice = file.slice(offset, offset + chunkSize);
    const ab = await slice.arrayBuffer();
    const u = new Uint8Array(ab);
    const buf = new Uint8Array(1 + u.length); buf[0] = "u".charCodeAt(0); buf.set(u, 1); DESK_WS.send(buf);
    offset += u.length;
  }
  DESK_WS.send(new Uint8Array(["e".charCodeAt(0)]));
  toast(I18N.t("desktop.uploading") + ": " + file.name, "info");
}

function deskStartDownload() {
  if (!DESK_WS || DESK_WS.readyState !== 1) { toast(I18N.t("desktop.not_connected"), "err"); return; }
  const path = (($("deskDownloadPath") && $("deskDownloadPath").value) || "").trim();
  if (!path) { toast(I18N.t("desktop.download_ph"), "err"); return; }
  const meta = new TextEncoder().encode(JSON.stringify({ remote_path: path }));
  const buf = new Uint8Array(1 + meta.length); buf[0] = "d".charCodeAt(0); buf.set(meta, 1); DESK_WS.send(buf);
}

async function openDeskSessions() {
  const pane = $("deskReplayPane");
  const list = $("deskSessionsList");
  if (!pane || !list) return;
  pane.hidden = false;
  list.innerHTML = I18N.t("ui.loading");
  try {
    const sessions = await fetch(`${API}/desktop/sessions`, { credentials: "include" }).then(r => r.json());
    if (!Array.isArray(sessions) || !sessions.length) {
      list.innerHTML = `<div class="empty-line">${esc(I18N.t("desktop.no_sessions"))}</div>`;
      return;
    }
    list.innerHTML = sessions.map(s => `
      <div class="desk-sess-row">
        <div><b>${esc(s.hostname || s.host_id)}</b> · ${esc(s.operator || "")} · ${s.frames || 0} ${esc(I18N.t("desktop.frames"))}
          ${s.active ? `<span class="tag">${esc(I18N.t("desktop.live"))}</span>` : ""}</div>
        <button type="button" class="btn sm" data-desk-replay="${esc(s.id)}">${esc(I18N.t("desktop.replay"))}</button>
      </div>`).join("");
  } catch (e) { list.innerHTML = `<div class="empty-line err">${esc(String(e))}</div>`; }
}

async function playDeskReplay(id) {
  const canvas = $("deskReplayCanvas");
  if (!canvas) return;
  canvas.style.display = "block";
  const ctx = canvas.getContext("2d");
  try {
    const data = await fetch(`${API}/desktop/sessions/${encodeURIComponent(id)}/replay`, { credentials: "include" }).then(r => r.json());
    const frames = data.frames || [];
    let i = 0;
    const tick = () => {
      if (i >= frames.length) return;
      const f = frames[i++];
      if (f.type === "jpeg" && f.data) {
        const bin = atob(f.data);
        const u8 = new Uint8Array(bin.length);
        for (let j = 0; j < bin.length; j++) u8[j] = bin.charCodeAt(j);
        const blob = new Blob([u8], { type: "image/jpeg" });
        const url = URL.createObjectURL(blob);
        const img = new Image();
        img.onload = () => {
          canvas.width = img.width; canvas.height = img.height;
          ctx.drawImage(img, 0, 0); URL.revokeObjectURL(url);
          setTimeout(tick, 120);
        };
        img.src = url;
      } else setTimeout(tick, 40);
    };
    tick();
  } catch (e) { toast(String(e), "err"); }
}

function closeDesktopWS() {
  if (DESK_WS) { try { DESK_WS.close(); } catch (e) {} DESK_WS = null; }
  unbindDesktopInput($("deskCanvas"));
  closeDeskMSE();
}

function closeDesktopMask() {
  closeDesktopWS();
  DESK_PHASE = "idle";
  DESK_GOT_FRAME = false;
  const mask = $("desktopMask");
  if (mask) mask.classList.remove("show");
}
