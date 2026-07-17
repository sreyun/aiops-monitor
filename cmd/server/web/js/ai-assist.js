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
let _aiAssistState = { task: "", mode: "analyze", context: "", applyTo: null, lastAnswer: "", busy: false, abort: null };

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
        <span class="ai-assist-title" id="aiAssistTitle">AI 辅助</span>
        <button class="ai-assist-close" id="aiAssistClose" title="关闭">×</button>
      </div>
      <div class="ai-assist-input-row" id="aiAssistInputRow" style="display:none">
        <textarea class="ai-assist-input" id="aiAssistInput" rows="2" placeholder="用一句话描述你的需求…"></textarea>
        <button class="btn primary sm" id="aiAssistRun">生成</button>
      </div>
      <div class="ai-assist-body" id="aiAssistBody"></div>
      <div class="ai-assist-actions" id="aiAssistActions">
        <button class="btn sm ai-assist-fb" id="aiAssistUp" title="这次结果有用（强化记忆）" style="display:none">👍</button>
        <button class="btn sm ai-assist-fb" id="aiAssistDown" title="这次结果没用" style="display:none">👎</button>
        <button class="btn primary sm" id="aiAssistApply" style="display:none">应用</button>
        <button class="btn sm" id="aiAssistCopy" style="display:none">复制</button>
        <button class="btn sm" id="aiAssistRegen" style="display:none">重新生成</button>
      </div>
    </div>`;
  document.body.appendChild(mask);
  mask.addEventListener("click", e => { if (e.target === mask) closeAIAssist(); });
  document.getElementById("aiAssistClose").addEventListener("click", closeAIAssist);
  document.getElementById("aiAssistRun").addEventListener("click", () => runAIAssist());
  document.getElementById("aiAssistRegen").addEventListener("click", () => runAIAssist());
  document.getElementById("aiAssistCopy").addEventListener("click", () => { if (_aiAssistState.lastAnswer && typeof copyText === "function") copyText(_aiAssistState.lastAnswer); });
  document.getElementById("aiAssistApply").addEventListener("click", () => {
    if (!_aiAssistState.applyTo) return;
    const code = extractFirstCodeBlock(_aiAssistState.lastAnswer) || (_aiAssistState.lastAnswer || "").trim();
    sendAssistFeedback("applied"); // 采纳即强化：应用视为最强正反馈
    _aiAssistState.applyTo(code);
    closeAIAssist();
  });
  document.getElementById("aiAssistUp").addEventListener("click", () => { sendAssistFeedback("helpful"); if (typeof toast === "function") toast("已标记为有用 👍，AI 会记住", "ok"); markAssistFeedbackDone(); });
  document.getElementById("aiAssistDown").addEventListener("click", () => { sendAssistFeedback("unhelpful"); if (typeof toast === "function") toast("已标记为无用 👎", "ok"); markAssistFeedbackDone(); });
  document.getElementById("aiAssistInput").addEventListener("keydown", e => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); runAIAssist(); }
  });
  document.addEventListener("keydown", e => { if (e.key === "Escape" && mask.classList.contains("show")) closeAIAssist(); });
  return mask;
}

function openAIAssist(opts) {
  opts = opts || {};
  const mask = ensureAIAssistPanel();
  _aiAssistState.task = opts.task || "generic";
  _aiAssistState.context = opts.context || "";
  _aiAssistState.applyTo = (typeof opts.applyTo === "function") ? opts.applyTo : null;
  _aiAssistState.lastAnswer = "";
  _aiAssistState.mode = opts.mode || (_aiAssistState.applyTo ? "generate" : "analyze");
  document.getElementById("aiAssistTitle").textContent = opts.title || "AI 辅助";
  const inputRow = document.getElementById("aiAssistInputRow");
  const inputEl = document.getElementById("aiAssistInput");
  const applyBtn = document.getElementById("aiAssistApply");
  applyBtn.textContent = opts.applyLabel || "应用到输入框";
  document.getElementById("aiAssistBody").innerHTML = `<div class="ai-assist-hint">${esc(opts.hint || (_aiAssistState.mode === "generate" ? "描述需求后点「生成」，AI 会给出结果并可一键应用。" : "AI 正在分析…"))}</div>`;
  document.getElementById("aiAssistCopy").style.display = "none";
  document.getElementById("aiAssistRegen").style.display = "none";
  document.getElementById("aiAssistUp").style.display = "none";
  document.getElementById("aiAssistDown").style.display = "none";
  document.getElementById("aiAssistActions").style.display = "none";
  applyBtn.style.display = "none";
  mask.classList.add("show");
  if (_aiAssistState.mode === "generate") {
    inputRow.style.display = "flex";
    inputEl.placeholder = opts.placeholder || "用一句话描述你的需求…";
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
  document.getElementById("aiAssistActions").style.display = "none";
  document.getElementById("aiAssistRun").disabled = true;
  body.innerHTML = `<div class="ai-thinking"><span class="ai-thinking-dots"><span></span><span></span><span></span></span> <span class="ai-thinking-text">正在思考…</span></div>`;
  let answer = "", reasoning = "", raf = null;
  const paint = (streaming) => {
    body.innerHTML = renderReasoningBlock(reasoning, streaming) +
      (streaming
        ? `<div class="ai-stream-body"><span class="ai-stream-text">${esc(answer)}</span><span class="ai-stream-cursor">▍</span></div>`
        : (renderAIMarkdown(answer) || "<div class='ai-assist-hint'>（空回复）</div>"));
    body.scrollTop = body.scrollHeight;
  };
  const schedule = () => { if (raf) return; raf = requestAnimationFrame(() => { raf = null; paint(true); }); };
  try {
    _aiAssistState.abort = (typeof AbortController !== "undefined") ? new AbortController() : null;
    const r = await fetch(`${API}/ai/assist`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      signal: _aiAssistState.abort ? _aiAssistState.abort.signal : undefined,
      body: JSON.stringify({ task: _aiAssistState.task, input: userInput, context: _aiAssistState.context })
    });
    if (!r.ok) throw new Error("HTTP " + r.status);
    await readSSEStream(r,
      (d, full) => { answer = full; schedule(); },
      (err) => { if (raf) { cancelAnimationFrame(raf); raf = null; } body.innerHTML = `<div class="ai-assist-err">✗ ${esc(err)}</div>`; },
      (full) => { if (raf) { cancelAnimationFrame(raf); raf = null; } answer = full || answer; _aiAssistState.lastAnswer = answer; paint(false); onAIAssistDone(); },
      null, null, null,
      (rd, fullR) => { reasoning = fullR; schedule(); }
    );
  } catch (e) {
    if (raf) { cancelAnimationFrame(raf); raf = null; }
    if (!(_aiAssistState.abort && e && e.name === "AbortError")) body.innerHTML = `<div class="ai-assist-err">✗ 请求失败：${esc(String(e))}</div>`;
  } finally {
    _aiAssistState.busy = false; _aiAssistState.abort = null;
    document.getElementById("aiAssistRun").disabled = false;
  }
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
}

// 采纳/评价反馈回传后端 → 学习闭环强化/惩罚对应记忆。尽力而为，失败静默。
function sendAssistFeedback(action) {
  const st = _aiAssistState;
  if (!st.lastAnswer) return;
  try {
    fetch(`${API}/ai/assist/feedback`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ task: st.task, input: st.lastInput || "", answer: st.lastAnswer || "", action })
    });
  } catch (e) { /* 忽略：反馈非关键路径 */ }
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
