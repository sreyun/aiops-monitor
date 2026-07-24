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
let _aiAssistState = {
  task: "", mode: "analyze", context: "", applyTo: null, lastAnswer: "", lastInput: "",
  busy: false, abort: null, history: [], followup: false, runId: 0, timer: null,
  ragCitations: [], title: "", feedbackOpen: false
};

// i18n 小助手：有 I18N 取译文，否则回退中文默认（面板在多语言下就地翻译）
function tA(k, f) { return (window.I18N && I18N.t) ? I18N.t(k, f) : f; }

function trapAIDialogFocus(e, root) {
  if (!root || e.key !== "Tab") return;
  const items = Array.from(root.querySelectorAll(
    'button:not([disabled]),textarea:not([disabled]),input:not([disabled]),select:not([disabled]),a[href],[tabindex]:not([tabindex="-1"])'
  )).filter(el => el.offsetParent !== null);
  if (!items.length) return;
  const first = items[0], last = items[items.length - 1], active = document.activeElement;
  if (!root.contains(active)) {
    e.preventDefault();
    first.focus();
  } else if (e.shiftKey && active === first) {
    e.preventDefault();
    last.focus();
  } else if (!e.shiftKey && active === last) {
    e.preventDefault();
    first.focus();
  }
}

function ensureAIAssistPanel() {
  let mask = document.getElementById("aiAssistMask");
  if (mask) return mask;
  mask = document.createElement("div");
  mask.id = "aiAssistMask";
  mask.className = "ai-assist-mask";
  mask.innerHTML = `
    <div class="ai-assist-panel" role="dialog" aria-modal="true" aria-labelledby="aiAssistTitle">
      <div class="ai-assist-head">
        <span class="ai-assist-icon">🤖</span>
        <span class="ai-assist-title" id="aiAssistTitle" data-i18n="assist.title">AI 辅助</span>
        <button class="ai-assist-close" id="aiAssistClose" data-i18n-title="assist.close" title="关闭">×</button>
      </div>
      <div class="ai-assist-input-row" id="aiAssistInputRow" style="display:none">
        <textarea class="ai-assist-input" id="aiAssistInput" rows="2" data-i18n-placeholder="assist.input_ph" placeholder="用一句话描述你的需求…"></textarea>
        <button class="btn primary sm" id="aiAssistRun" data-i18n="assist.run">生成</button>
      </div>
      <div class="ai-assist-status" id="aiAssistStatus" role="status" aria-live="polite" aria-atomic="true"></div>
      <div class="ai-assist-body" id="aiAssistBody"></div>
      <div class="ai-assist-feedback" id="aiAssistFeedback" style="display:none">
        <label for="aiAssistFeedbackReason">${esc(tA("assist.unhelpful_reason", "请说明不准确、缺失或不可执行之处"))}</label>
        <textarea id="aiAssistFeedbackReason" rows="2" maxlength="2000" placeholder="例如：引用了不存在的指标；未考虑 Windows 主机；处置步骤有风险…"></textarea>
        <div class="ai-assist-feedback-actions">
          <button class="btn sm" id="aiAssistFeedbackCancel" type="button">取消</button>
          <button class="btn danger sm" id="aiAssistFeedbackSend" type="button">提交差评</button>
        </div>
      </div>
      <div class="ai-assist-actions" id="aiAssistActions">
        <button class="btn sm danger" id="aiAssistStop" data-i18n="assist.stop" style="display:none" title="停止生成">⏹ 停止</button>
        <button class="btn sm ai-assist-fb" id="aiAssistUp" data-i18n-title="assist.fb_up" title="这次结果有用（强化记忆）" style="display:none">👍</button>
        <button class="btn sm ai-assist-fb" id="aiAssistDown" data-i18n-title="assist.fb_down" title="这次结果没用" style="display:none">👎</button>
        <button class="btn primary sm" id="aiAssistApply" data-i18n="assist.apply_to_input" style="display:none">应用</button>
        <button class="btn sm" id="aiAssistCopy" data-i18n="assist.copy" style="display:none">复制</button>
        <span class="ai-assist-export" id="aiAssistExportWrap" style="display:none">
          <select id="aiAssistExportFormat" aria-label="导出格式">
            <option value="markdown">Markdown</option>
            <option value="excel">表格 Excel</option>
            <option value="word">Word</option>
            <option value="pdf">PDF</option>
          </select>
          <button class="btn sm" id="aiAssistExport" type="button">导出</button>
        </span>
        <button class="btn sm" id="aiAssistRegen" data-i18n="assist.regen" style="display:none">重新生成</button>
      </div>
    </div>`;
  document.body.appendChild(mask);
  mask.addEventListener("click", e => { if (e.target === mask) closeAIAssist(); });
  document.getElementById("aiAssistClose").addEventListener("click", closeAIAssist);
  document.getElementById("aiAssistRun").addEventListener("click", () => submitAIAssist());
  document.getElementById("aiAssistRegen").addEventListener("click", () => { _aiAssistState.followup = false; runAIAssist(); });
  document.getElementById("aiAssistCopy").addEventListener("click", () => { if (_aiAssistState.lastAnswer && typeof copyText === "function") copyText(_aiAssistState.lastAnswer); });
  document.getElementById("aiAssistApply").addEventListener("click", async () => {
    if (!_aiAssistState.applyTo) return;
    const btn = document.getElementById("aiAssistApply");
    const code = extractFirstCodeBlock(_aiAssistState.lastAnswer) || (_aiAssistState.lastAnswer || "").trim();
    btn.disabled = true;
    try {
      const applied = await _aiAssistState.applyTo(code);
      if (applied === false) return;
      await sendAssistFeedback("applied"); // 只有应用真正成功后才学习
      closeAIAssist();
    } catch (e) {
      if (typeof toast === "function") toast(tA("assist.apply_failed", "应用失败") + "：" + String(e), "err");
    } finally {
      btn.disabled = false;
    }
  });
  document.getElementById("aiAssistUp").addEventListener("click", async () => {
    const result = await sendAssistFeedback("helpful");
    if (result === false) return;
    if (typeof toast === "function") {
      toast(result.learning_queued
        ? tA("assist.fb_marked_helpful", "已标记为有用 👍，AI 会记住")
        : tA("assist.fb_recorded_no_memory", "反馈已记录；持久记忆不可用，本次未进入跨会话学习"), result.learning_queued ? "ok" : "warn");
    }
    markAssistFeedbackDone();
  });
  document.getElementById("aiAssistDown").addEventListener("click", openAssistFeedback);
  document.getElementById("aiAssistFeedbackCancel").addEventListener("click", closeAssistFeedback);
  document.getElementById("aiAssistFeedbackSend").addEventListener("click", submitAssistUnhelpful);
  document.getElementById("aiAssistExport").addEventListener("click", exportCurrentAIAssist);
  document.getElementById("aiAssistStop").addEventListener("click", () => stopAIAssist());
  document.getElementById("aiAssistInput").addEventListener("keydown", e => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); submitAIAssist(); }
  });
  document.addEventListener("keydown", e => {
    if (!mask.classList.contains("show")) return;
    if (e.key === "Escape") closeAIAssist();
    else trapAIDialogFocus(e, mask.querySelector(".ai-assist-panel"));
  });
  if (window.I18N && I18N.applyTranslations) I18N.applyTranslations(mask); // 面板文案按当前语言就地翻译
  return mask;
}

function openAIAssist(opts) {
  opts = opts || {};
  if (_aiAssistState.abort) { try { _aiAssistState.abort.abort(); } catch (e) {} }
  if (_aiAssistState.timer) { clearTimeout(_aiAssistState.timer); _aiAssistState.timer = null; }
  _aiAssistState.runId++;
  _aiAssistState.returnFocus = document.activeElement;
  const mask = ensureAIAssistPanel();
  _aiAssistState.task = opts.task || "generic";
  _aiAssistState.context = opts.context || "";
  _aiAssistState.applyTo = (typeof opts.applyTo === "function") ? opts.applyTo : null;
  _aiAssistState.lastAnswer = "";
  _aiAssistState.history = [];
  _aiAssistState.followup = false;
  _aiAssistState.ragCitations = [];
  _aiAssistState.feedbackOpen = false;
  _aiAssistState.mode = opts.mode || (_aiAssistState.applyTo ? "generate" : "analyze");
  _aiAssistState.title = opts.title || tA("assist.title", "AI 辅助");
  document.getElementById("aiAssistTitle").textContent = _aiAssistState.title;
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
  document.getElementById("aiAssistExportWrap").style.display = "none";
  document.getElementById("aiAssistActions").style.display = "none";
  document.getElementById("aiAssistFeedback").style.display = "none";
  setAssistStatus("");
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
  const runId = ++_aiAssistState.runId;
  const body = document.getElementById("aiAssistBody");
  const inputEl = document.getElementById("aiAssistInput");
  const userInput = (_aiAssistState.mode === "generate") ? (inputEl.value || "").trim() : "";
  if (_aiAssistState.mode === "generate" && !userInput) { inputEl.focus(); return; }
  _aiAssistState.lastInput = userInput; // 供采纳/评价反馈语义定位记忆
  _aiAssistState.busy = true;
  _aiAssistState.lastAnswer = "";
  _aiAssistState.ragCitations = [];
  document.getElementById("aiAssistCopy").style.display = "none";
  document.getElementById("aiAssistApply").style.display = "none";
  document.getElementById("aiAssistRegen").style.display = "none";
  document.getElementById("aiAssistUp").style.display = "none";
  document.getElementById("aiAssistDown").style.display = "none";
  document.getElementById("aiAssistActions").style.display = "flex";
  document.getElementById("aiAssistStop").style.display = "";
  document.getElementById("aiAssistRun").disabled = true;
  setAssistStatus(tA("assist.preparing", "正在准备上下文与检索依据…"));
  body.innerHTML = `<div class="ai-thinking"><span class="ai-thinking-dots"><span></span><span></span><span></span></span> <span class="ai-thinking-text">${esc(tA("assist.thinking", "正在思考…"))}</span></div>`;
  let answer = "", reasoning = "", raf = null, ragHint = "";
  const paint = (streaming) => {
    if (runId !== _aiAssistState.runId) return;
    const hint = ragHint ? `<div class="ai-rag-hint">${esc(ragHint)}</div>` : "";
    const citations = renderAssistCitations(_aiAssistState.ragCitations);
    const rendered = renderAIMarkdown(answer);
    body.innerHTML = hint + renderReasoningBlock(reasoning, streaming) +
      (streaming
        ? `<div class="ai-stream-body">${rendered}<span class="ai-stream-cursor">▍</span></div>`
        : (rendered || `<div class='ai-assist-hint'>${esc(tA("assist.empty_reply", "（空回复）"))}</div>`)) + citations;
    body.scrollTop = body.scrollHeight;
  };
  const schedule = () => { if (raf) return; raf = requestAnimationFrame(() => { raf = null; paint(true); }); };
  try {
    _aiAssistState.abort = (typeof AbortController !== "undefined") ? new AbortController() : null;
    _aiAssistState.timer = setTimeout(() => {
      try { if (runId === _aiAssistState.runId && _aiAssistState.abort) _aiAssistState.abort.abort(); } catch (e) {}
    }, 180000); // 覆盖 fetch + 完整流读取，而不是仅覆盖响应头
    const r = await fetch(`${API}/ai/assist`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      signal: _aiAssistState.abort ? _aiAssistState.abort.signal : undefined,
      body: JSON.stringify({ task: _aiAssistState.task, input: userInput, context: _aiAssistState.context })
    });
    if (!r.ok) throw new Error("HTTP " + r.status);
    setAssistStatus(tA("assist.streaming", "正在流式生成，可随时停止…"));
    await readSSEStream(r,
      (d, full) => { if (runId !== _aiAssistState.runId) return; answer = full; schedule(); },
      (err) => {
        if (runId !== _aiAssistState.runId) return;
        if (raf) { cancelAnimationFrame(raf); raf = null; }
        body.innerHTML = `<div class="ai-assist-err">✗ ${esc(err)}</div>`;
        setAssistStatus(tA("assist.failed", "生成失败"));
        if (/AI 未配置|未启用/.test(String(err || "")) && typeof promptOpenAIConfig === "function") promptOpenAIConfig(err);
      },
      (full) => {
        if (runId !== _aiAssistState.runId) return;
        if (raf) { cancelAnimationFrame(raf); raf = null; }
        answer = full || answer; _aiAssistState.lastAnswer = answer; paint(false);
        setAssistStatus(tA("assist.completed", "生成完成，可继续追问、导出或反馈"));
        onAIAssistDone();
      },
      null,
      (meta) => {
        if (runId !== _aiAssistState.runId) return;
        if (!meta) return;
        if (Array.isArray(meta.citations)) _aiAssistState.ragCitations = meta.citations.slice(0, 20);
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
      (rd, fullR) => { if (runId !== _aiAssistState.runId) return; reasoning = fullR; schedule(); }
    );
  } catch (e) {
    if (runId !== _aiAssistState.runId) return;
    if (raf) { cancelAnimationFrame(raf); raf = null; }
    if (_aiAssistState.abort && e && e.name === "AbortError") {
      body.innerHTML = `<div class="ai-assist-hint">⏹ ${esc(tA("assist.stopped", "已停止生成"))}</div>${answer ? renderAIMarkdown(answer) : ""}`;
      setAssistStatus(tA("assist.stopped", "已停止生成"));
      if (answer) { _aiAssistState.lastAnswer = answer; onAIAssistDone(); }
    } else {
      const msg = String(e);
      const tip = /abort|timeout|Timeout|Failed to fetch|network/i.test(msg)
        ? tA("assist.timeout_hint", "请求超时或网络中断，请检查 AI 服务后重试")
        : tA("assist.request_failed", "请求失败") + "：" + msg;
      body.innerHTML = `<div class="ai-assist-err">✗ ${esc(tip)}</div>`;
      setAssistStatus(tA("assist.failed", "生成失败"));
      if (/AI 未配置|未启用/.test(msg) && typeof promptOpenAIConfig === "function") promptOpenAIConfig(msg);
    }
  } finally {
    if (_aiAssistState.timer) { clearTimeout(_aiAssistState.timer); _aiAssistState.timer = null; }
    if (runId !== _aiAssistState.runId) return;
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
  document.getElementById("aiAssistExportWrap").style.display = has ? "inline-flex" : "none";
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
  const runId = ++_aiAssistState.runId;
  const inputEl = document.getElementById("aiAssistInput");
  const runBtn = document.getElementById("aiAssistRun");
  const body = document.getElementById("aiAssistBody");
  const q = (inputEl.value || "").trim();
  if (!q) { inputEl.focus(); return; }
  inputEl.value = "";
  _aiAssistState.busy = true;
  setAssistStatus(tA("assist.streaming", "正在流式生成，可随时停止…"));
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
    if (runId !== _aiAssistState.runId) return;
    const rendered = renderAIMarkdown(answer);
    aEl.innerHTML = renderReasoningBlock(reasoning, streaming) +
      (streaming
        ? `<div class="ai-stream-body">${rendered}<span class="ai-stream-cursor">▍</span></div>`
        : (rendered || `<div class='ai-assist-hint'>${esc(tA("assist.empty_reply", "（空回复）"))}</div>`));
    body.scrollTop = body.scrollHeight;
  };
  const schedule = () => { if (raf) return; raf = requestAnimationFrame(() => { raf = null; paint(true); }); };
  try {
    _aiAssistState.abort = (typeof AbortController !== "undefined") ? new AbortController() : null;
    _aiAssistState.timer = setTimeout(() => {
      try { if (runId === _aiAssistState.runId && _aiAssistState.abort) _aiAssistState.abort.abort(); } catch (e) {}
    }, 180000);
    const r = await fetch(`${API}/ai/assist`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      signal: _aiAssistState.abort ? _aiAssistState.abort.signal : undefined,
      body: JSON.stringify({ task: _aiAssistState.task, input: q, context: _aiAssistState.context, history: _aiAssistState.history })
    });
    if (!r.ok) throw new Error("HTTP " + r.status);
    await readSSEStream(r,
      (d, full) => { if (runId !== _aiAssistState.runId) return; answer = full; schedule(); },
      (err) => {
        if (runId !== _aiAssistState.runId) return;
        if (raf) { cancelAnimationFrame(raf); raf = null; }
        aEl.innerHTML = `<div class="ai-assist-err">✗ ${esc(err)}</div>`;
        setAssistStatus(tA("assist.failed", "生成失败"));
      },
      (full) => {
        if (runId !== _aiAssistState.runId) return;
        if (raf) { cancelAnimationFrame(raf); raf = null; }
        answer = full || answer; paint(false);
        _aiAssistState.lastInput = q;
        _aiAssistState.lastAnswer = answer;
        _aiAssistState.history.push({ role: "user", content: q }, { role: "assistant", content: answer });
        if (_aiAssistState.history.length > 20) _aiAssistState.history = _aiAssistState.history.slice(-20);
        setAssistStatus(tA("assist.completed", "生成完成，可继续追问、导出或反馈"));
        onFollowupDone(aEl);
      },
      null,
      (meta) => {
        if (runId === _aiAssistState.runId && meta && Array.isArray(meta.citations)) {
          _aiAssistState.ragCitations = meta.citations.slice(0, 20);
        }
      },
      null,
      (rd, fullR) => { if (runId !== _aiAssistState.runId) return; reasoning = fullR; schedule(); }
    );
  } catch (e) {
    if (runId !== _aiAssistState.runId) return;
    if (raf) { cancelAnimationFrame(raf); raf = null; }
    if (_aiAssistState.abort && e && e.name === "AbortError") {
      aEl.innerHTML = answer ? renderAIMarkdown(answer) : `<div class="ai-assist-hint">⏹ ${esc(tA("assist.stopped", "已停止生成"))}</div>`;
      if (answer) {
        _aiAssistState.lastInput = q;
        _aiAssistState.lastAnswer = answer;
        onFollowupDone(aEl);
      }
      setAssistStatus(tA("assist.stopped", "已停止生成"));
    } else {
      aEl.innerHTML = `<div class="ai-assist-err">✗ ${esc(tA("assist.request_failed", "请求失败"))}：${esc(String(e))}</div>`;
      setAssistStatus(tA("assist.failed", "生成失败"));
    }
  } finally {
    if (_aiAssistState.timer) { clearTimeout(_aiAssistState.timer); _aiAssistState.timer = null; }
    if (runId !== _aiAssistState.runId) return;
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
  document.getElementById("aiAssistExportWrap").style.display = has ? "inline-flex" : "none";
  document.getElementById("aiAssistActions").style.display = has ? "flex" : "none";
  (aEl || document).querySelectorAll(".ai-code-copy").forEach(b => b.onclick = () => {
    const code = b.closest(".ai-code-wrap") ? b.closest(".ai-code-wrap").querySelector("code").textContent : "";
    if (typeof copyText === "function") copyText(code);
  });
}

function setAssistStatus(text) {
  const el = document.getElementById("aiAssistStatus");
  if (!el) return;
  el.textContent = text || "";
  el.style.display = text ? "" : "none";
}

let _aiFeedbackReasonPromise = null;

// 全站需要短文本输入的流程使用非阻塞、可访问的对话框，替代 window.prompt。
// 返回 string；取消时返回 null。原始回答不会放进 DOM 属性或遥测。
function requestAIFeedbackReason(opts) {
  opts = opts || {};
  if (_aiFeedbackReasonPromise) {
    const existing = document.getElementById("aiFeedbackReasonInput");
    if (existing) existing.focus();
    return _aiFeedbackReasonPromise;
  }
  const previousFocus = document.activeElement;
  const mask = document.createElement("div");
  mask.className = "ai-assist-mask show ai-feedback-prompt-mask";
  mask.innerHTML = `
    <div class="ai-feedback-prompt-panel" role="dialog" aria-modal="true" aria-labelledby="aiFeedbackReasonTitle" aria-describedby="aiFeedbackReasonHelp">
      <div class="ai-assist-head">
        <span class="ai-assist-icon" aria-hidden="true">🧭</span>
        <span class="ai-assist-title" id="aiFeedbackReasonTitle"></span>
        <button class="ai-assist-close" id="aiFeedbackReasonClose" type="button" aria-label="关闭">×</button>
      </div>
      <div class="ai-feedback-prompt-body">
        <p id="aiFeedbackReasonHelp"></p>
        <label for="aiFeedbackReasonInput" id="aiFeedbackReasonLabel"></label>
        <textarea id="aiFeedbackReasonInput" rows="4" maxlength="2000"></textarea>
        <div class="ai-feedback-prompt-error" id="aiFeedbackReasonError" role="status" aria-live="polite"></div>
      </div>
      <div class="ai-assist-actions">
        <button class="btn sm" id="aiFeedbackReasonCancel" type="button"></button>
        <button class="btn danger sm" id="aiFeedbackReasonSubmit" type="button"></button>
      </div>
    </div>`;
  document.body.appendChild(mask);
  const title = mask.querySelector("#aiFeedbackReasonTitle");
  const help = mask.querySelector("#aiFeedbackReasonHelp");
  const label = mask.querySelector("#aiFeedbackReasonLabel");
  let input = mask.querySelector("#aiFeedbackReasonInput");
  const error = mask.querySelector("#aiFeedbackReasonError");
  const cancelBtn = mask.querySelector("#aiFeedbackReasonCancel");
  const submitBtn = mask.querySelector("#aiFeedbackReasonSubmit");
  if (opts.singleLine || opts.inputType) {
    const oneLine = document.createElement("input");
    oneLine.id = "aiFeedbackReasonInput";
    oneLine.type = ["text", "password", "url", "number"].includes(opts.inputType) ? opts.inputType : "text";
    if (opts.autocomplete) oneLine.autocomplete = opts.autocomplete;
    if (opts.min != null) oneLine.min = String(opts.min);
    if (opts.max != null) oneLine.max = String(opts.max);
    if (opts.step != null) oneLine.step = String(opts.step);
    input.replaceWith(oneLine);
    input = oneLine;
  }
  title.textContent = opts.title || tA("assist.improve_title", "帮助 AI 持续改进");
  help.textContent = opts.message || tA("assist.unhelpful_reason", "请说明不准确、缺失或不可执行之处；该反馈会形成避坑经验。");
  label.textContent = opts.label || tA("assist.unhelpful_reason_label", "具体问题");
  input.placeholder = opts.placeholder || tA("assist.unhelpful_example", "例如：引用了不存在的指标；处置步骤缺少回滚方案…");
  if (input.tagName === "TEXTAREA") input.rows = Math.max(1, Math.min(10, Number(opts.rows) || 4));
  const maxChars = Math.max(1, Math.min(16000, Number(opts.maxLength) || 2000));
  if (input.tagName === "TEXTAREA" || input.type !== "number") input.maxLength = maxChars;
  input.value = opts.defaultValue == null ? "" : String(opts.defaultValue).slice(0, maxChars);
  cancelBtn.textContent = opts.cancelLabel || tA("assist.cancel", "取消");
  submitBtn.textContent = opts.submitLabel || tA("assist.submit_feedback", "提交反馈");
  if (opts.danger === false) submitBtn.classList.replace("danger", "primary");

  _aiFeedbackReasonPromise = new Promise(resolve => {
    let finished = false;
    const finish = value => {
      if (finished) return;
      finished = true;
      document.removeEventListener("keydown", onKey, true);
      mask.remove();
      _aiFeedbackReasonPromise = null;
      if (previousFocus && typeof previousFocus.focus === "function" && document.contains(previousFocus)) {
        setTimeout(() => previousFocus.focus(), 0);
      }
      resolve(value);
    };
    const submit = () => {
      const reason = (input.value || "").trim();
      if (!reason && opts.required !== false) {
        error.textContent = opts.requiredMessage || tA("assist.reason_required", "请填写具体原因，便于形成可复用的避坑经验");
        input.focus();
        return;
      }
      finish(reason);
    };
    const onKey = e => {
      if (e.key === "Escape") {
        e.preventDefault();
        e.stopImmediatePropagation();
        finish(null);
      } else if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
        e.preventDefault();
        submit();
      } else {
        trapAIDialogFocus(e, mask.querySelector(".ai-feedback-prompt-panel"));
      }
    };
    mask.querySelector("#aiFeedbackReasonClose").addEventListener("click", () => finish(null));
    mask.querySelector("#aiFeedbackReasonCancel").addEventListener("click", () => finish(null));
    mask.querySelector("#aiFeedbackReasonSubmit").addEventListener("click", submit);
    mask.addEventListener("click", e => { if (e.target === mask) finish(null); });
    input.addEventListener("input", () => { error.textContent = ""; });
    document.addEventListener("keydown", onKey, true);
    setTimeout(() => input.focus(), 20);
  });
  return _aiFeedbackReasonPromise;
}

// 语义化别名：供 URL、拓扑等非反馈场景复用同一套无障碍交互。
function requestAITextInput(opts) {
  return requestAIFeedbackReason(opts || {});
}

function renderAssistCitations(citations) {
  const items = Array.isArray(citations) ? citations.filter(c => c && c.title).slice(0, 12) : [];
  if (!items.length) return "";
  return `<details class="ai-evidence"><summary>依据与记忆来源 · ${items.length}</summary><div class="ai-evidence-list">` +
    items.map(c => `<span class="ai-evidence-item"><b>${esc(c.kind || "source")}</b>${esc(c.title || "")}</span>`).join("") +
    `</div></details>`;
}

function openAssistFeedback() {
  const box = document.getElementById("aiAssistFeedback");
  const input = document.getElementById("aiAssistFeedbackReason");
  if (!box || !input) return;
  box.style.display = "";
  _aiAssistState.feedbackOpen = true;
  input.value = "";
  setTimeout(() => input.focus(), 20);
}

function closeAssistFeedback() {
  const box = document.getElementById("aiAssistFeedback");
  if (box) box.style.display = "none";
  _aiAssistState.feedbackOpen = false;
}

async function submitAssistUnhelpful() {
  const input = document.getElementById("aiAssistFeedbackReason");
  const reason = (input && input.value || "").trim();
  if (!reason) {
    if (typeof toast === "function") toast(tA("assist.reason_required", "请填写具体原因，便于形成可复用的避坑经验"), "err");
    if (input) input.focus();
    return;
  }
  const btn = document.getElementById("aiAssistFeedbackSend");
  if (btn) btn.disabled = true;
  try {
    const result = await sendAssistFeedback("unhelpful", reason);
    if (result === false) return;
    if (typeof toast === "function") {
      toast(result.learning_queued
        ? tA("assist.fb_marked_unhelpful", "已记录问题与避坑经验 👎")
        : tA("assist.fb_recorded_no_memory", "反馈已记录；持久记忆不可用，本次未进入跨会话学习"), result.learning_queued ? "ok" : "warn");
    }
    closeAssistFeedback();
    markAssistFeedbackDone();
  } finally {
    if (btn) btn.disabled = false;
  }
}

// 采纳/评价反馈回传后端。只有成功写入闭环才返回 true，避免网络失败却提示“已学习”。
async function sendAssistFeedback(action, reason) {
  const st = _aiAssistState;
  if (!st.lastAnswer) return false;
  try {
    const r = await fetch(`${API}/ai/assist/feedback`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ task: st.task, input: st.lastInput || "", answer: st.lastAnswer || "", action, reason: reason || "" })
    });
    const j = await r.json().catch(() => ({}));
    if (!r.ok) {
      if (typeof toast === "function") toast(j.error || "反馈失败", "err");
      return false;
    }
    return j;
  } catch (e) {
    if (typeof toast === "function") toast(tA("assist.feedback_failed", "反馈未保存，请检查网络后重试"), "err");
    return false;
  }
}

function parseAssistMarkdownTables(text) {
  const lines = String(text || "").split(/\r?\n/);
  const sections = [];
  for (let i = 0; i + 1 < lines.length; i++) {
    if (!lines[i].includes("|") || !/^\s*\|?\s*:?-{3,}/.test(lines[i + 1])) continue;
    const split = line => line.trim().replace(/^\||\|$/g, "").split("|").map(s => s.replace(/\\\|/g, "|").trim());
    const columns = split(lines[i]);
    const rows = [];
    i += 2;
    while (i < lines.length && lines[i].includes("|") && lines[i].trim()) {
      const row = split(lines[i]);
      while (row.length < columns.length) row.push("");
      rows.push(row.slice(0, columns.length));
      i++;
    }
    if (columns.length && rows.length) sections.push({ title: "结构化结果 " + (sections.length + 1), columns, rows });
  }
  return sections;
}

function exportCurrentAIAssist() {
  if (!_aiAssistState.lastAnswer || typeof exportModel !== "function") {
    if (typeof toast === "function") toast(tA("assist.no_export", "暂无可导出的 AI 结果"), "warn");
    return;
  }
  const fmtEl = document.getElementById("aiAssistExportFormat");
  const fmt = fmtEl ? fmtEl.value : "markdown";
  const citations = (_aiAssistState.ragCitations || []).filter(c => c && c.title);
  const sections = parseAssistMarkdownTables(_aiAssistState.lastAnswer);
  if (citations.length) {
    sections.push({
      title: "依据与来源", columns: ["类型", "来源", "标题"],
      rows: citations.map(c => [c.kind || "", c.source || "", c.title || ""])
    });
  }
  const model = {
    title: _aiAssistState.title || "AI 诊断报告",
    subtitle: "AIOps Monitor · " + new Date().toLocaleString(),
    summaryTitle: "报告信息",
    meta: [
      ["AI 任务", _aiAssistState.task || "generic"],
      ["生成时间", new Date().toLocaleString()],
      ["知识依据", citations.length ? citations.length + " 条" : "本次未命中可展示依据"]
    ],
    narrativeTitle: "AI 分析与建议",
    narrative: _aiAssistState.lastAnswer,
    sections,
    orientation: sections.some(s => (s.columns || []).length > 4) ? "landscape" : "portrait",
    footer: "AI 结果仅作为运维决策辅助；高风险操作须经人工验证与审批。"
  };
  const ok = exportModel(model, fmt, _aiAssistState.title || "AI诊断报告");
  if (ok === false && typeof toast === "function") toast(tA("assist.popup_blocked", "浏览器拦截了导出窗口，请允许弹窗后重试"), "warn");
}

// 评价一次后隐藏 👍/👎，避免重复提交
function markAssistFeedbackDone() {
  const up = document.getElementById("aiAssistUp"), down = document.getElementById("aiAssistDown");
  if (up) up.style.display = "none";
  if (down) down.style.display = "none";
}

function closeAIAssist() {
  _aiAssistState.runId++;
  if (_aiAssistState.abort) { try { _aiAssistState.abort.abort(); } catch (e) {} }
  if (_aiAssistState.timer) { clearTimeout(_aiAssistState.timer); _aiAssistState.timer = null; }
  const mask = document.getElementById("aiAssistMask");
  if (mask) mask.classList.remove("show");
  _aiAssistState.busy = false;
  _aiAssistState.abort = null;
  closeAssistFeedback();
  setAssistStatus("");
  const back = _aiAssistState.returnFocus;
  if (back && typeof back.focus === "function" && document.contains(back)) setTimeout(() => back.focus(), 0);
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
