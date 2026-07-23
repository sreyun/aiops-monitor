/* ============================================================================
 * 全站统一「AI 辅助」悬浮面板（可复用组件）
 *
 * 一个面板服务所有模块：LogQL/PromQL 生成、剧本生成、图表数据分析、审计日志诊断、
 * 弹窗结果诊断等。后端统一走 POST /api/v1/ai/assist（任务化 SSE 流式）。
 * 复用 sre.js 的 readSSEStream / renderReasoningBlock / renderAIMarkdown / esc / copyText。
 *
 * 用法：openAIAssist({ task, title, mode, context, presetInput, placeholder, applyTo, applyLabel })
 *   - task:        logql | promql | playbook | chart_analysis | audit_diagnosis | result_diagnosis | generic
 *   - mode:        "generate"(带输入框，用户描述需求) | "analyze"(打开即分析给定 context)
 *   - context:     调用方整理好的上下文文本（可用标签 / 数据摘要 / 结果正文 / 审计条目）
 *   - applyTo:     可选 fn(text)——点「应用」时回调，参数为抽取的首个代码块(或全文)
 * ========================================================================== */
let _aiAssistState = { task: "", mode: "analyze", context: "", applyTo: null, lastAnswer: "", busy: false, abort: null, history: [], followup: false };

// i18n 小助手：有 I18N 取译文，否则回退中文默认（面板在多语言下就地翻译）
function tA(k, f) { return (window.I18N && I18N.t) ? I18N.t(k, f) : f; }

function ensureAIAssistPanel() {
  let mask = document.getElementById("aiAssistMask");
  if (mask) return mask;
  mask = document.createElement("div");
  mask.id = "aiAssistMask";
  mask.className = "ai-assist-mask";
  mask.innerHTML = `
    <div class="ai-assist-panel" role="dialog" aria-modal="true">
      <div class="ai-assist-head">
        <span class="ai-assist-icon">🤖</span>
        <span class="ai-assist-title" id="aiAssistTitle" data-i18n="assist.title">AI 辅助</span>
        <button class="ai-assist-close" id="aiAssistClose" data-i18n-title="assist.close" title="关闭">×</button>
      </div>
      <div class="ai-assist-input-row" id="aiAssistInputRow" style="display:none">
        <textarea class="ai-assist-input" id="aiAssistInput" rows="2" data-i18n-placeholder="assist.input_ph" placeholder="用一句话描述你的需求…"></textarea>
        <button class="btn primary sm" id="aiAssistRun" data-i18n="assist.run">生成</button>
      </div>
      <div class="ai-assist-body" id="aiAssistBody"></div>
      <div class="ai-assist-actions" id="aiAssistActions">
        <button class="btn sm danger" id="aiAssistStop" data-i18n="assist.stop" style="display:none" title="停止生成">⏹ 停止</button>
        <button class="btn sm ai-assist-fb" id="aiAssistUp" data-i18n-title="assist.fb_up" title="这次结果有用（强化记忆）" style="display:none">👍</button>
        <button class="btn sm ai-assist-fb" id="aiAssistDown" data-i18n-title="assist.fb_down" title="这次结果没用" style="display:none">👎</button>
        <button class="btn primary sm" id="aiAssistApply" data-i18n="assist.apply_to_input" style="display:none">应用</button>
        <button class="btn sm" id="aiAssistCopy" data-i18n="assist.copy" style="display:none">复制</button>
        <button class="btn sm" id="aiAssistRegen" data-i18n="assist.regen" style="display:none">重新生成</button>
      </div>
    </div>`;
  document.body.appendChild(mask);
  mask.addEventListener("click", e => { if (e.target === mask) closeAIAssist(); });
  document.getElementById("aiAssistClose").addEventListener("click", closeAIAssist);
  document.getElementById("aiAssistRun").addEventListener("click", () => submitAIAssist());
  document.getElementById("aiAssistRegen").addEventListener("click", () => { _aiAssistState.followup = false; runAIAssist(); });
  document.getElementById("aiAssistCopy").addEventListener("click", () => { if (_aiAssistState.lastAnswer && typeof copyText === "function") copyText(_aiAssistState.lastAnswer); });
  document.getElementById("aiAssistApply").addEventListener("click", () => {
    if (!_aiAssistState.applyTo) return;
    const code = extractFirstCodeBlock(_aiAssistState.lastAnswer) || (_aiAssistState.lastAnswer || "").trim();
    sendAssistFeedback("applied"); // 采纳即强化：应用视为最强正反馈
    _aiAssistState.applyTo(code);
    closeAIAssist();
  });
  document.getElementById("aiAssistUp").addEventListener("click", async () => {
    if (await sendAssistFeedback("helpful") === false) return;
    if (typeof toast === "function") toast(tA("assist.fb_marked_helpful", "已标记为有用 👍，AI 会记住"), "ok");
    markAssistFeedbackDone();
  });
  document.getElementById("aiAssistDown").addEventListener("click", async () => {
    if (await sendAssistFeedback("unhelpful") === false) return;
    if (typeof toast === "function") toast(tA("assist.fb_marked_unhelpful", "已标记为无用 👎"), "ok");
    markAssistFeedbackDone();
  });
  document.getElementById("aiAssistStop").addEventListener("click", () => stopAIAssist());
  document.getElementById("aiAssistInput").addEventListener("keydown", e => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); submitAIAssist(); }
  });
  document.addEventListener("keydown", e => { if (e.key === "Escape" && mask.classList.contains("show")) closeAIAssist(); });
  if (window.I18N && I18N.applyTranslations) I18N.applyTranslations(mask); // 面板文案按当前语言就地翻译
  return mask;
}

function openAIAssist(opts) {
  opts = opts || {};
  const mask = ensureAIAssistPanel();
  _aiAssistState.task = opts.task || "generic";
  _aiAssistState.context = opts.context || "";
  _aiAssistState.applyTo = (typeof opts.applyTo === "function") ? opts.applyTo : null;
  _aiAssistState.lastAnswer = "";
  _aiAssistState.history = [];
  _aiAssistState.followup = false;
  _aiAssistState.mode = opts.mode || (_aiAssistState.applyTo ? "generate" : "analyze");
  document.getElementById("aiAssistTitle").textContent = opts.title || tA("assist.title", "AI 辅助");
  const inputRow = document.getElementById("aiAssistInputRow");
  const inputEl = document.getElementById("aiAssistInput");
  const applyBtn = document.getElementById("aiAssistApply");
  applyBtn.textContent = opts.applyLabel || tA("assist.apply_to_input", "应用到输入框");
  document.getElementById("aiAssistRun").textContent = tA("assist.run", "生成"); // 复位（追问会改成「追问」）
  document.getElementById("aiAssistBody").innerHTML = `<div class="ai-assist-hint">${esc(opts.hint || (_aiAssistState.mode === "generate" ? tA("assist.hint_generate", "描述需求后点「生成」，AI 会给出结果并可一键应用。") : tA("assist.hint_analyze", "AI 正在分析…")))}</div>`;
  document.getElementById("aiAssistCopy").style.display = "none";
  document.getElementById("aiAssistRegen").style.display = "none";
  document.getElementById("aiAssistUp").style.display = "none";
  document.getElementById("aiAssistDown").style.display = "none";
  document.getElementById("aiAssistActions").style.display = "none";
  applyBtn.style.display = "none";
  mask.classList.add("show");
  if (_aiAssistState.mode === "generate") {
    inputRow.style.display = "flex";
    inputEl.placeholder = opts.placeholder || tA("assist.input_ph", "用一句话描述你的需求…");
    inputEl.value = opts.presetInput || opts.prefill || ""; // prefill 只填不跑；presetInput 填并直接生成
    setTimeout(() => inputEl.focus(), 50);
    if (opts.presetInput) runAIAssist(); // 已预置需求则直接生成
  } else {
    inputRow.style.display = "none";
    runAIAssist(); // analyze 模式打开即执行
  }
}

async function runAIAssist() {
  if (_aiAssistState.busy) return;
  const body = document.getElementById("aiAssistBody");
  const inputEl = document.getElementById("aiAssistInput");
  const userInput = (_aiAssistState.mode === "generate") ? (inputEl.value || "").trim() : "";
  if (_aiAssistState.mode === "generate" && !userInput) { inputEl.focus(); return; }
  _aiAssistState.lastInput = userInput; // 供采纳/评价反馈语义定位记忆
  _aiAssistState.busy = true;
  _aiAssistState.lastAnswer = "";
  document.getElementById("aiAssistCopy").style.display = "none";
  document.getElementById("aiAssistApply").style.display = "none";
  document.getElementById("aiAssistRegen").style.display = "none";
  document.getElementById("aiAssistUp").style.display = "none";
  document.getElementById("aiAssistDown").style.display = "none";
  document.getElementById("aiAssistActions").style.display = "flex";
  document.getElementById("aiAssistStop").style.display = "";
  document.getElementById("aiAssistRun").disabled = true;
  body.innerHTML = `<div class="ai-thinking"><span class="ai-thinking-dots"><span></span><span></span><span></span></span> <span class="ai-thinking-text">${esc(tA("assist.thinking", "正在思考…"))}</span></div>`;
  let answer = "", reasoning = "", raf = null, ragHint = "";
  const paint = (streaming) => {
    const hint = ragHint ? `<div class="ai-rag-hint">${esc(ragHint)}</div>` : "";
    body.innerHTML = hint + renderReasoningBlock(reasoning, streaming) +
      (streaming
        ? `<div class="ai-stream-body"><span class="ai-stream-text">${esc(answer)}</span><span class="ai-stream-cursor">▍</span></div>`
        : (renderAIMarkdown(answer) || `<div class='ai-assist-hint'>${esc(tA("assist.empty_reply", "（空回复）"))}</div>`));
    body.scrollTop = body.scrollHeight;
  };
  const schedule = () => { if (raf) return; raf = requestAnimationFrame(() => { raf = null; paint(true); }); };
  try {
    _aiAssistState.abort = (typeof AbortController !== "undefined") ? new AbortController() : null;
    const timer = setTimeout(() => { try { _aiAssistState.abort && _aiAssistState.abort.abort(); } catch (e) {} }, 180000); // 3min 超时
    const r = await fetch(`${API}/ai/assist`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      signal: _aiAssistState.abort ? _aiAssistState.abort.signal : undefined,
      body: JSON.stringify({ task: _aiAssistState.task, input: userInput, context: _aiAssistState.context })
    });
    clearTimeout(timer);
    if (!r.ok) throw new Error("HTTP " + r.status);
    await readSSEStream(r,
      (d, full) => { answer = full; schedule(); },
      (err) => {
        if (raf) { cancelAnimationFrame(raf); raf = null; }
        body.innerHTML = `<div class="ai-assist-err">✗ ${esc(err)}</div>`;
        if (/AI 未配置|未启用/.test(String(err || "")) && typeof promptOpenAIConfig === "function") promptOpenAIConfig(err);
      },
      (full) => { if (raf) { cancelAnimationFrame(raf); raf = null; } answer = full || answer; _aiAssistState.lastAnswer = answer; paint(false); onAIAssistDone(); },
      null,
      (meta) => {
        if (!meta) return;
        const parts = [];
        if (meta.degraded_tip) parts.push(meta.degraded_tip);
        else {
          if (meta.memory_hits > 0) parts.push("记忆 ×" + meta.memory_hits);
          if (meta.skill_hits > 0) {
            let sk = "技能 ×" + meta.skill_hits;
            if (Array.isArray(meta.skill_names) && meta.skill_names.length) {
              sk += "（" + meta.skill_names.slice(0, 4).join("、") + (meta.skill_names.length > 4 ? "…" : "") + "）";
            }
            parts.push(sk);
          }
        }
        if (parts.length) { ragHint = parts.join(" · "); schedule(); }
      },
      null,
      (rd, fullR) => { reasoning = fullR; schedule(); }
    );
  } catch (e) {
    if (raf) { cancelAnimationFrame(raf); raf = null; }
    if (_aiAssistState.abort && e && e.name === "AbortError") {
      body.innerHTML = `<div class="ai-assist-hint">⏹ ${esc(tA("assist.stopped", "已停止生成"))}${answer ? "<br><br>" + esc(answer) : ""}</div>`;
      if (answer) { _aiAssistState.lastAnswer = answer; onAIAssistDone(); }
    } else {
      const msg = String(e);
      const tip = /abort|timeout|Timeout|Failed to fetch|network/i.test(msg)
        ? tA("assist.timeout_hint", "请求超时或网络中断，请检查 AI 服务后重试")
        : tA("assist.request_failed", "请求失败") + "：" + msg;
      body.innerHTML = `<div class="ai-assist-err">✗ ${esc(tip)}</div>`;
      if (/AI 未配置|未启用/.test(msg) && typeof promptOpenAIConfig === "function") promptOpenAIConfig(msg);
    }
  } finally {
    _aiAssistState.busy = false; _aiAssistState.abort = null;
    document.getElementById("aiAssistRun").disabled = false;
    const stopBtn = document.getElementById("aiAssistStop");
    if (stopBtn) stopBtn.style.display = "none";
  }
}

function stopAIAssist() {
  if (_aiAssistState.abort) { try { _aiAssistState.abort.abort(); } catch (e) {} }
}

function onAIAssistDone() {
  const has = !!(_aiAssistState.lastAnswer && _aiAssistState.lastAnswer.trim());
  document.getElementById("aiAssistCopy").style.display = has ? "" : "none";
  document.getElementById("aiAssistRegen").style.display = (has && _aiAssistState.mode === "generate") ? "" : "none";
  document.getElementById("aiAssistApply").style.display = (has && _aiAssistState.applyTo) ? "" : "none";
  // 采纳/评价反馈（学习闭环）：有结果就展示 👍/👎
  document.getElementById("aiAssistUp").style.display = has ? "" : "none";
  document.getElementById("aiAssistDown").style.display = has ? "" : "none";
  document.getElementById("aiAssistActions").style.display = has ? "flex" : "none";
  // 渲染出的代码块复制按钮就地生效
  const body = document.getElementById("aiAssistBody");
  body.querySelectorAll(".ai-code-copy").forEach(b => b.onclick = () => {
    const code = b.closest(".ai-code-wrap") ? b.closest(".ai-code-wrap").querySelector("code").textContent : "";
    if (typeof copyText === "function") copyText(code);
  });
  // analyze 模式（诊断/分析类面板）出结果后开启多轮追问，形成「基于同一份数据的会话交流」
  if (has && _aiAssistState.mode === "analyze" && !_aiAssistState.followup) enableFollowupComposer(_aiAssistState.lastAnswer);
}

// submitAIAssist：输入框「发送」分发——首轮走 runAIAssist，进入会话后走 runAIFollowup（多轮追问）。
function submitAIAssist() {
  if (_aiAssistState.followup) runAIFollowup();
  else runAIAssist();
}

// enableFollowupComposer：analyze 首答后开启「继续追问」，并把首轮 Q&A 存入 history（供后端多轮）。
function enableFollowupComposer(firstAnswer) {
  _aiAssistState.followup = true;
  _aiAssistState.history = [
    { role: "user", content: "请根据上述上下文进行分析并给出结论。" },
    { role: "assistant", content: firstAnswer }
  ];
  const inputRow = document.getElementById("aiAssistInputRow");
  const inputEl = document.getElementById("aiAssistInput");
  const runBtn = document.getElementById("aiAssistRun");
  inputRow.style.display = "flex";
  inputEl.value = "";
  inputEl.placeholder = tA("assist.followup_ph", "继续追问（基于以上数据与结论）…");
  runBtn.textContent = tA("assist.followup_send", "追问");
  runBtn.disabled = false;
}

// runAIFollowup：基于同一份 context + 累积 history 的多轮追问；结果追加渲染，不覆盖前文。
async function runAIFollowup() {
  if (_aiAssistState.busy) return;
  const inputEl = document.getElementById("aiAssistInput");
  const runBtn = document.getElementById("aiAssistRun");
  const body = document.getElementById("aiAssistBody");
  const q = (inputEl.value || "").trim();
  if (!q) { inputEl.focus(); return; }
  inputEl.value = "";
  _aiAssistState.busy = true;
  runBtn.disabled = true;
  document.getElementById("aiAssistActions").style.display = "flex";
  const stopBtn = document.getElementById("aiAssistStop");
  if (stopBtn) stopBtn.style.display = "";
  document.getElementById("aiAssistUp").style.display = "none";
  document.getElementById("aiAssistDown").style.display = "none";
  const turn = document.createElement("div");
  turn.className = "ai-fu-turn";
  turn.style.cssText = "margin-top:14px;padding-top:12px;border-top:1px dashed var(--border,rgba(127,127,127,.28))";
  const qEl = document.createElement("div");
  qEl.className = "ai-fu-q";
  qEl.style.cssText = "font-weight:600;margin-bottom:6px";
  qEl.textContent = "🧑 " + q;
  const aEl = document.createElement("div");
  aEl.className = "ai-fu-a";
  aEl.innerHTML = `<div class="ai-thinking"><span class="ai-thinking-dots"><span></span><span></span><span></span></span> <span class="ai-thinking-text">${esc(tA("assist.thinking", "正在思考…"))}</span></div>`;
  turn.appendChild(qEl); turn.appendChild(aEl);
  body.appendChild(turn);
  body.scrollTop = body.scrollHeight;
  let answer = "", reasoning = "", raf = null;
  const paint = (streaming) => {
    aEl.innerHTML = renderReasoningBlock(reasoning, streaming) +
      (streaming
        ? `<div class="ai-stream-body"><span class="ai-stream-text">${esc(answer)}</span><span class="ai-stream-cursor">▍</span></div>`
        : (renderAIMarkdown(answer) || `<div class='ai-assist-hint'>${esc(tA("assist.empty_reply", "（空回复）"))}</div>`));
    body.scrollTop = body.scrollHeight;
  };
  const schedule = () => { if (raf) return; raf = requestAnimationFrame(() => { raf = null; paint(true); }); };
  try {
    _aiAssistState.abort = (typeof AbortController !== "undefined") ? new AbortController() : null;
    const r = await fetch(`${API}/ai/assist`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      signal: _aiAssistState.abort ? _aiAssistState.abort.signal : undefined,
      body: JSON.stringify({ task: _aiAssistState.task, input: q, context: _aiAssistState.context, history: _aiAssistState.history })
    });
    if (!r.ok) throw new Error("HTTP " + r.status);
    await readSSEStream(r,
      (d, full) => { answer = full; schedule(); },
      (err) => { if (raf) { cancelAnimationFrame(raf); raf = null; } aEl.innerHTML = `<div class="ai-assist-err">✗ ${esc(err)}</div>`; },
      (full) => {
        if (raf) { cancelAnimationFrame(raf); raf = null; }
        answer = full || answer; paint(false);
        _aiAssistState.lastInput = q;
        _aiAssistState.lastAnswer = answer;
        _aiAssistState.history.push({ role: "user", content: q }, { role: "assistant", content: answer });
        if (_aiAssistState.history.length > 20) _aiAssistState.history = _aiAssistState.history.slice(-20);
        onFollowupDone(aEl);
      },
      null, null, null,
      (rd, fullR) => { reasoning = fullR; schedule(); }
    );
  } catch (e) {
    if (raf) { cancelAnimationFrame(raf); raf = null; }
    if (!(_aiAssistState.abort && e && e.name === "AbortError")) aEl.innerHTML = `<div class="ai-assist-err">✗ ${esc(tA("assist.request_failed", "请求失败"))}：${esc(String(e))}</div>`;
  } finally {
    _aiAssistState.busy = false; _aiAssistState.abort = null;
    runBtn.disabled = false;
    const stopBtn = document.getElementById("aiAssistStop");
    if (stopBtn) stopBtn.style.display = "none";
    setTimeout(() => inputEl.focus(), 30);
  }
}

// onFollowupDone：追问出结果后，重新展示 👍/👎/复制（针对最新一轮）并激活代码块复制。
function onFollowupDone(aEl) {
  const has = !!(_aiAssistState.lastAnswer && _aiAssistState.lastAnswer.trim());
  document.getElementById("aiAssistCopy").style.display = has ? "" : "none";
  document.getElementById("aiAssistUp").style.display = has ? "" : "none";
  document.getElementById("aiAssistDown").style.display = has ? "" : "none";
  document.getElementById("aiAssistActions").style.display = has ? "flex" : "none";
  (aEl || document).querySelectorAll(".ai-code-copy").forEach(b => b.onclick = () => {
    const code = b.closest(".ai-code-wrap") ? b.closest(".ai-code-wrap").querySelector("code").textContent : "";
    if (typeof copyText === "function") copyText(code);
  });
}

// 采纳/评价反馈回传后端 → 学习闭环强化/惩罚对应记忆。尽力而为，失败静默。
async function sendAssistFeedback(action) {
  const st = _aiAssistState;
  if (!st.lastAnswer) return false;
  let reason = "";
  if (action === "unhelpful") {
    reason = prompt(typeof I18N !== "undefined" ? I18N.t("sre.unhelpful_reason", "请简要说明为何无用（将写入避坑记忆）：") : "请简要说明为何无用（将写入避坑记忆）：", "");
    if (reason === null) return false;
    reason = (reason || "").trim();
    if (!reason) {
      if (typeof toast === "function") toast(typeof I18N !== "undefined" ? I18N.t("sre.need_unhelpful_reason", "差评需填写原因") : "差评需填写原因", "err");
      return false;
    }
  }
  try {
    const r = await fetch(`${API}/ai/assist/feedback`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ task: st.task, input: st.lastInput || "", answer: st.lastAnswer || "", action, reason })
    });
    const j = await r.json().catch(() => ({}));
    if (!r.ok) {
      if (typeof toast === "function") toast(j.error || "反馈失败", "err");
      return false;
    }
  } catch (e) { /* 忽略：反馈非关键路径 */ }
  return true;
}
// 评价一次后隐藏 👍/👎，避免重复提交
function markAssistFeedbackDone() {
  const up = document.getElementById("aiAssistUp"), down = document.getElementById("aiAssistDown");
  if (up) up.style.display = "none";
  if (down) down.style.display = "none";
}

function closeAIAssist() {
  if (_aiAssistState.abort) { try { _aiAssistState.abort.abort(); } catch (e) {} }
  const mask = document.getElementById("aiAssistMask");
  if (mask) mask.classList.remove("show");
  _aiAssistState.busy = false;
}

// 从 Markdown 文本抽取首个代码块内容（用于「应用」到查询语句 / 剧本字段）
function extractFirstCodeBlock(text) {
  if (!text) return "";
  const m = text.match(/```[a-zA-Z0-9_+#-]*\n?([\s\S]*?)```/);
  return m ? m[1].replace(/\n+$/, "").trim() : "";
}

// 小工具：生成一个统一样式的「AI 辅助」按钮 HTML（供各模块内联使用）
function aiAssistBtnHTML(label, extraClass) {
  return `<button type="button" class="btn sm ai-assist-btn ${extraClass || ""}"><span class="ai-assist-btn-ic">🤖</span>${esc(label || "AI 辅助")}</button>`;
}
