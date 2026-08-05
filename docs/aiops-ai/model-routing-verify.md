# 模型路由验证 SQL 指南

> 本文档提供一组可在平台「SQL 工作台」直接执行的查询，用于**验证模型路由是否如配置所愿工作**。
> 前提：`ai_call_events_p` 已包含 `route_reason` 列（迁移 v12+），且平台已产生若干 AI 调用记录。

`route_reason` 取值说明：

| 值 | 含义 |
|---|---|
| `task_models` | 命中 `TaskModelsJSON` 的任务级映射（最优先） |
| `cheap_model` | 轻量任务（`isCheapAITask`）走了 `CheapModel` |
| `primary` | 走主配置模型 `cfg.Model` |
| `fallback` | 主模型失败后故障转移到 `FallbackModels` |
| `''`（空） | 未记录（迁移前旧数据） |

## 1. 路由原因总览 —— 验证路由策略分布

```sql
SELECT COALESCE(NULLIF(route_reason,''),'(未记录)') AS route_reason,
       COUNT(*) AS calls,
       SUM(CASE WHEN NOT ok THEN 1 ELSE 0 END) AS failed,
       ROUND(SUM(cost_estimate)::numeric, 4) AS cost,
       ROUND(AVG(latency_ms)) AS avg_ms
FROM ai_call_events_p
WHERE ts >= EXTRACT(EPOCH FROM NOW() - INTERVAL '30 days')::bigint
GROUP BY 1 ORDER BY calls DESC;
```

**怎么读**：
- `task_models` 占比高 → 管理员手配的任务映射在生效。
- `cheap_model` 占比高 → 轻量任务（promql/logql/summarize…）确实走了便宜模型。
- `fallback` 出现 → 有主模型调用失败被转移，需排查主模型稳定性（见第 4 条）。

## 2. 核对"cheap 任务是否真的走了 cheap 模型"

```sql
-- 轻量任务里，本应走 CheapModel 的有多少反而走了主模型？
SELECT task, model,
       COUNT(*) AS calls,
       SUM(CASE WHEN NOT ok THEN 1 ELSE 0 END) AS failed,
       ROUND(SUM(cost_estimate)::numeric, 4) AS cost
FROM ai_call_events_p
WHERE ts >= EXTRACT(EPOCH FROM NOW() - INTERVAL '30 days')::bigint
  AND route_reason IN ('cheap_model','primary')
  AND task IN ('promql','logql','pgsql','summarize','translate','classify')
GROUP BY 1,2 ORDER BY calls DESC;
```

**怎么读**：若某 task 出现 `model != CheapModel` 且 `route_reason='primary'`，说明该任务没被 `isCheapAITask` 识别（或没配 CheapModel），成本比预期高——需要补 `TaskModelsJSON`。

## 3. 故障转移分析 —— 主模型是否频繁故障

```sql
-- 触发 fallback 的调用：用了哪些备用模型、失败率、成本
SELECT model, COUNT(*) AS fallback_calls,
       SUM(CASE WHEN NOT ok THEN 1 ELSE 0 END) AS still_failed,
       ROUND(SUM(cost_estimate)::numeric, 4) AS cost
FROM ai_call_events_p
WHERE route_reason = 'fallback'
  AND ts >= EXTRACT(EPOCH FROM NOW() - INTERVAL '30 days')::bigint
GROUP BY 1 ORDER BY fallback_calls DESC;

-- fallback 的失败率 vs 总体（fallback 仍失败说明备用模型也不稳）
SELECT 'fallback' AS bucket, COUNT(*), SUM(CASE WHEN NOT ok THEN 1 ELSE 0 END)::float/COUNT(*) AS fail_rate
FROM ai_call_events_p WHERE route_reason='fallback'
UNION ALL
SELECT 'overall', COUNT(*), SUM(CASE WHEN NOT ok THEN 1 ELSE 0 END)::float/COUNT(*)
FROM ai_call_events_p;
```

**怎么读**：`still_failed` 高说明 FallbackModels 里配置的备用模型也不可靠；fallback 失败率明显高于总体说明主链路存在系统性故障。

## 4. 路由 × 模型 × 任务 交叉 —— 定位异常路由

```sql
SELECT route_reason, model, task,
       COUNT(*) AS calls,
       ROUND(SUM(cost_estimate)::numeric, 4) AS cost
FROM ai_call_events_p
WHERE ts >= EXTRACT(EPOCH FROM NOW() - INTERVAL '7 days')::bigint
GROUP BY 1,2,3
HAVING COUNT(*) > 3
ORDER BY cost DESC;
```

**怎么读**：一眼看出"哪个路由原因 × 哪个模型 × 哪个任务"在烧钱或异常。若出现 `task_models` 却配了很贵的模型，就是配置问题。

## 5. 精确成本（exact）按路由原因拆分 —— 对账口径

```sql
SELECT COALESCE(NULLIF(route_reason,''),'(未记录)') AS route_reason,
       SUM(CASE WHEN usage_source='exact' THEN prompt_tokens+completion_tokens ELSE 0 END) AS exact_tokens,
       SUM(CASE WHEN usage_source<>'exact' THEN approx_tokens ELSE 0 END) AS approx_tokens,
       ROUND(SUM(CASE WHEN usage_source='exact' THEN cost_estimate ELSE 0 END)::numeric, 4) AS exact_cost
FROM ai_call_events_p
WHERE ts >= EXTRACT(EPOCH FROM NOW() - INTERVAL '30 days')::bigint
GROUP BY 1 ORDER BY exact_cost DESC;
```

**怎么读**：`exact_cost` 是可信的计费口径成本；若某路由原因 exact 成本异常高，优先排查。

---

## 附：数据从哪里来

- 每条 AI 调用在 `recordAICallActor` 落库时，由 `inferRouteReason(cfg, task, usedModel)` 推断 `route_reason`，写入 `ai_call_events` / `ai_call_events_p`（双表同事务）。
- 前端在「AI 统计」页已有「模型路由原因」区块展示同一数据（`GET /api/v1/ai/stats` 的 `by_route_reason`）。
- 由于历史数据无 `route_reason`（迁移 v12 前），验证前建议先观察**迁移后产生的新调用**。
