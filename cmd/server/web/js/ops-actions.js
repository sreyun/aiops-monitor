/* ============================================================================
 * 统一 Action Proposal：解析 AI 输出的 {summary, actions[]} → 确认 → 调用既有 mutate API → 回验
 * window.applyOpsActionPlan(textOrPlan, opts)
 *   opts: { source, refresh, connectionId, selectSQL, reExplainSQL, hostId, targetId, allowDDL }
 * ========================================================================== */
(function () {
"use strict";

function oaT(k, fb) { return (window.I18N && I18N.t) ? I18N.t(k, fb) : fb; }

function parseOpsActionPlan(text) {
  if (!text) return null;
  if (typeof text === "object" && text.actions) return text;
  let raw = String(text).trim();
  const fence = raw.match(/```(?:json)?\s*([\s\S]*?)```/i);
  if (fence) raw = fence[1].trim();
  // Fallback: first {...} containing "actions"
  if (raw[0] !== "{") {
    const i = raw.indexOf("{");
    const j = raw.lastIndexOf("}");
    if (i >= 0 && j > i) raw = raw.slice(i, j + 1);
  }
  try {
    const obj = JSON.parse(raw);
    if (!obj || !Array.isArray(obj.actions)) return null;
    return obj;
  } catch (_) {
    return null;
  }
}

function actionLabel(a) {
  const t = a && a.type || "?";
  const p = a && a.params || {};
  const tgt = a && a.target || {};
  const name = tgt.name || tgt.id || tgt.vm_id || tgt.host_id || "";
  switch (t) {
    case "hyperv_power": return `Hyper-V ${p.action || "power"} · ${name}`;
    case "hyperv_config": return `Hyper-V 配置 · ${name}`;
    case "container_action": return `容器 ${p.action || "action"} · ${name || tgt.id}`;
    case "container_exec": return `容器 exec · ${name || tgt.id}`;
    case "k8s_scale": return `K8s scale=${p.replicas} · ${tgt.namespace}/${name}`;
    case "k8s_restart": return `K8s restart · ${tgt.namespace}/${name}`;
    case "k8s_undo": return `K8s rollout undo · ${tgt.namespace}/${name}`;
    case "k8s_delete_pod": return `K8s delete pod · ${tgt.namespace}/${name}`;
    case "k8s_exec": return `K8s exec · ${tgt.namespace}/${name}`;
    case "host_playbook": return `主机剧本 · ${tgt.host_id || name} (${(p.steps || []).length} 步)`;
    case "sql_apply": return "SQL 改写 → 编辑器";
    case "sql_ddl": return "SQL DDL（索引）";
    default: return t + (name ? " · " + name : "");
  }
}

async function apiJSON(path, opts) {
  const r = await fetch(`${API}${path}`, Object.assign({ credentials: "same-origin" }, opts || {}));
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error || j.output || `HTTP ${r.status}`);
  return j;
}

async function runOneAction(a, opts) {
  const t = a.type;
  const p = a.params || {};
  const tgt = a.target || {};
  switch (t) {
    case "hyperv_power": {
      const host = tgt.host_id;
      const vm = tgt.vm_id || tgt.id || tgt.name;
      if (!host || !vm) throw new Error("hyperv_power 缺少 host_id/vm_id");
      return apiJSON(`/hyperv/${encodeURIComponent(host)}/guests/${encodeURIComponent(vm)}/power`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action: p.action || "restart", name: tgt.name || "" }),
      });
    }
    case "hyperv_config": {
      const host = tgt.host_id;
      const vm = tgt.vm_id || tgt.id || tgt.name;
      if (!host || !vm) throw new Error("hyperv_config 缺少 host_id/vm_id");
      const body = { name: tgt.name || "" };
      ["processor_count", "memory_mb", "memory_min_mb", "memory_max_mb"].forEach(k => {
        if (p[k] != null && p[k] !== "") body[k] = Number(p[k]);
      });
      if (typeof p.dynamic_memory === "boolean") body.dynamic_memory = p.dynamic_memory;
      return apiJSON(`/hyperv/${encodeURIComponent(host)}/guests/${encodeURIComponent(vm)}/config`, {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
      });
    }
    case "container_action": {
      const host = tgt.host_id;
      const id = tgt.id || tgt.container_id;
      if (!host || !id) throw new Error("container_action 缺少 host_id/id");
      return apiJSON(`/containers/${encodeURIComponent(host)}/${encodeURIComponent(id)}/action`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action: p.action || "restart" }),
      });
    }
    case "container_exec": {
      const host = tgt.host_id;
      const id = tgt.id || tgt.container_id;
      if (!host || !id || !p.command) throw new Error("container_exec 缺少 host_id/id/command");
      return apiJSON(`/containers/${encodeURIComponent(host)}/${encodeURIComponent(id)}/exec`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ command: p.command, timeout_sec: p.timeout_sec || 15 }),
      });
    }
    case "k8s_scale": {
      const cid = tgt.cluster_id;
      const ns = tgt.namespace;
      const name = tgt.name;
      if (!cid || !ns || !name) throw new Error("k8s_scale 缺少 cluster_id/namespace/name");
      return apiJSON(`/k8s/clusters/${encodeURIComponent(cid)}/deployments/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/scale`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ replicas: Number(p.replicas) }),
      });
    }
    case "k8s_restart": {
      const cid = tgt.cluster_id;
      const ns = tgt.namespace;
      const name = tgt.name;
      if (!cid || !ns || !name) throw new Error("k8s_restart 缺少 cluster_id/namespace/name");
      return apiJSON(`/k8s/clusters/${encodeURIComponent(cid)}/deployments/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/restart`, {
        method: "POST", headers: { "Content-Type": "application/json" }, body: "{}",
      });
    }
    case "k8s_undo": {
      const cid = tgt.cluster_id;
      const ns = tgt.namespace;
      const name = tgt.name;
      if (!cid || !ns || !name) throw new Error("k8s_undo 缺少 cluster_id/namespace/name");
      return apiJSON(`/k8s/clusters/${encodeURIComponent(cid)}/deployments/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/undo`, {
        method: "POST", headers: { "Content-Type": "application/json" }, body: "{}",
      });
    }
    case "k8s_delete_pod": {
      const cid = tgt.cluster_id;
      const ns = tgt.namespace;
      const name = tgt.name;
      if (!cid || !ns || !name) throw new Error("k8s_delete_pod 缺少 cluster_id/namespace/name");
      return apiJSON(`/k8s/clusters/${encodeURIComponent(cid)}/pods/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`, {
        method: "DELETE",
      });
    }
    case "k8s_exec": {
      const cid = tgt.cluster_id;
      const ns = tgt.namespace;
      const name = tgt.name;
      if (!cid || !ns || !name || !p.command) throw new Error("k8s_exec 缺少参数");
      return apiJSON(`/k8s/clusters/${encodeURIComponent(cid)}/pods/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/exec`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ command: p.command, timeout_sec: p.timeout_sec || 15 }),
      });
    }
    case "host_playbook": {
      const hostId = tgt.host_id || opts.hostId;
      const steps = Array.isArray(p.steps) ? p.steps : [];
      if (!hostId || !steps.length) throw new Error("host_playbook 缺少 host_id/steps");
      const pb = {
        id: "ai-remediation-" + Date.now(),
        name: p.name || ("AI 修复 · " + hostId),
        description: p.description || "AI 建议动作（一次性，需人工确认）",
        steps: steps.map((s, i) => ({
          name: s.name || ("step-" + (i + 1)),
          module: s.module || "",
          args: s.args || {},
          command: s.command || "",
          command_win: s.command_win || "",
          target: s.target || ("host:" + hostId),
          timeout_sec: s.timeout_sec || 60,
          continue_on_error: !!s.continue_on_error,
          ignore_exit: !!s.ignore_exit,
        })),
      };
      const created = await apiJSON("/playbooks", {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(pb),
      });
      const id = (created && (created.id || created.playbook_id)) || pb.id;
      return apiJSON(`/playbooks/${encodeURIComponent(id)}/execute`, {
        method: "POST", headers: { "X-AIOps-Risk-Accepted": "true" },
      });
    }
    case "sql_apply": {
      const sql = p.sql || p.rewritten || "";
      if (!sql) throw new Error("sql_apply 缺少 params.sql");
      if (typeof opts.selectSQL === "function") opts.selectSQL(sql);
      else if (typeof window.setSQLText === "function") window.setSQLText(sql);
      else throw new Error("SQL 编辑器未就绪");
      return { ok: true };
    }
    case "sql_ddl": {
      const conn = tgt.connection_id || opts.connectionId;
      const sql = p.sql || "";
      if (!conn || !sql) throw new Error("sql_ddl 缺少 connection_id/sql");
      const env = typeof window.sqlConnectionEnvironment === "function"
        ? window.sqlConnectionEnvironment(conn) : "prod";
      if (env === "prod") {
        if (typeof window.submitSQLChangeRequest === "function") {
          return window.submitSQLChangeRequest(conn, sql, p.reason || "AI SQL 优化建议");
        }
        return apiJSON("/sql/change-requests", {
          method: "POST", headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ connection_id: conn, sql, reason: p.reason || "AI SQL 优化建议" }),
        });
      }
      if (!opts.allowDDL) throw new Error("未确认允许执行 DDL");
      const verifySQL = opts.reExplainSQL || (p.verify_sql || "");
      return apiJSON(`/sql/connections/${encodeURIComponent(conn)}/exec-ddl`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sql, allow_exec: true, timeout_sec: p.timeout_sec || 30, verify_sql: verifySQL }),
      });
    }
    default:
      throw new Error("不支持的动作类型: " + t);
  }
}

async function verifyAction(a, opts) {
  const v = (a && a.verify) || "none";
  if (v === "refresh_inventory" || v === "refresh") {
    if (typeof opts.refresh === "function") await opts.refresh();
    return;
  }
  if (v === "rescans" || v === "rescan") {
    const hostId = (a.target && a.target.host_id) || opts.hostId;
    const targetId = (a.target && a.target.target_id) || opts.targetId;
    if (hostId) {
      await apiJSON("/security/host/scan", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ host_ids: [hostId] }),
      });
    } else if (targetId) {
      await apiJSON(`/security/web/targets/${encodeURIComponent(targetId)}/scan`, { method: "POST" });
    }
    if (typeof opts.refresh === "function") await opts.refresh();
    return;
  }
  if (v === "re_explain") {
    const conn = (a.target && a.target.connection_id) || opts.connectionId;
    const sql = opts.reExplainSQL || (a.params && (a.params.verify_sql || a.params.sql)) || "";
    if (conn && sql && typeof window.runSQLExplain === "function") {
      await window.runSQLExplain(conn, sql);
    } else if (typeof opts.refresh === "function") {
      await opts.refresh();
    }
  }
}

/**
 * @returns {Promise<boolean>} true if applied; false if cancelled / no actions
 */
async function applyOpsActionPlan(text, opts) {
  opts = opts || {};
  const plan = parseOpsActionPlan(text);
  if (!plan) {
    if (typeof toast === "function") toast(oaT("ops.plan_parse_fail", "未找到可解析的动作 JSON"), "err");
    return false;
  }
  const actions = plan.actions || [];
  if (!actions.length) {
    if (typeof toast === "function") toast(plan.summary || oaT("ops.plan_empty", "无可执行动作"), "ok");
    return false;
  }
  const lines = actions.map((a, i) => `${i + 1}. [${(a.risk || "medium").toUpperCase()}] ${actionLabel(a)}`).join("\n");
  const summary = plan.summary ? (plan.summary + "\n\n") : "";
  if (!confirm(summary + oaT("ops.confirm_apply", "确认按顺序执行以下建议动作？") + "\n\n" + lines)) {
    return false;
  }
  const hasDDL = actions.some(a => a.type === "sql_ddl");
  if (hasDDL) {
    const ok = confirm(oaT("ops.confirm_ddl", "其中包含 SQL DDL（索引变更）。生产环境将提交变更单，其他环境将直接执行，是否继续？"));
    if (!ok) return false;
    opts.allowDDL = true;
  }
  const high = actions.filter(a => String(a.risk || "").toLowerCase() === "high");
  if (high.length) {
    if (!confirm(oaT("ops.confirm_high", "含高危动作，再次确认继续？") + "\n" + high.map(actionLabel).join("\n"))) {
      return false;
    }
  }

  // Server-side whitelist + apply (prompt-injection hard gate). Client only handles sql_apply UI.
  try {
    const validated = await apiJSON("/ops/actions/validate", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ plan: { summary: plan.summary || "", actions } }),
    });
    const applied = await apiJSON("/ops/actions/apply", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        plan: validated.plan || { summary: plan.summary || "", actions },
        confirm: true,
        allow_ddl: !!opts.allowDDL,
        grant: validated.grant || "",
      }),
    });
    const results = (applied && applied.results) || [];
    let okN = 0;
    for (let i = 0; i < results.length; i++) {
      const r = results[i];
      const a = (validated.plan && validated.plan.actions && validated.plan.actions[i]) || actions[i] || {};
      if (r.client_side && a.type === "sql_apply" && r.sql) {
        try {
          await runOneAction(Object.assign({}, a, { params: Object.assign({}, a.params || {}, { sql: r.sql }) }), opts);
          okN++;
        } catch (e) {
          if (typeof toast === "function") toast(actionLabel(a) + ": " + (e.message || e), "err");
          return false;
        }
        continue;
      }
      if (!r.ok) {
        if (typeof toast === "function") toast(actionLabel(a) + ": " + (r.error || applied.error || "failed"), "err");
        return false;
      }
      okN++;
      if (a.type === "sql_ddl" && typeof opts.onDDLResult === "function" && r.output) {
        try { opts.onDDLResult(r.output); } catch (_) {}
      }
      try { await verifyAction(a, opts); } catch (ve) { console.warn("verify failed", ve); }
    }
    if (typeof toast === "function") {
      toast(oaT("ops.applied_n", "已执行 {n} 条动作").replace("{n}", String(okN)), "ok");
    }
    if (typeof opts.refresh === "function") {
      try { await opts.refresh(); } catch (_) {}
    }
    return true;
  } catch (e) {
    if (typeof toast === "function") toast(oaT("ops.plan_blocked", "动作计划被服务端拦截") + ": " + (e.message || e), "err");
    return false;
  }
}

window.parseOpsActionPlan = parseOpsActionPlan;
window.applyOpsActionPlan = applyOpsActionPlan;
})();
