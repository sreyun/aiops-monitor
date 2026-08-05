# AI 闭环效果周报

> 目标：让“AI 闭环自动化”从功能清单变成**可证明的价值**——
> 每周一 08:00 自动向消息中心推送一份量化的 SRE 效果周报。

## 1. 已落地能力（v0.19.x 起）

`duty_report.go` 新增 `buildWeeklyEffectReport()`：

- 时间窗：近 7 天（`computeSREEffect(7)`）。
- 触发：每周一（服务器本地时区）早报生成后自动追加推送；
  无任何事件/AI/变更数据时跳过，避免空报告打扰。
- 内容字段（与 `GET /api/v1/sre/effect` 同源）：

| KPI | 字段 | 含义 |
|---|---|---|
| 闭环数/闭环率 | `closed_loop_count` / `closed_loop_rate` | verify_ok 的事件占比 |
| AI 验证通过率 | `verify_pass_rate` | 有 verify 的 runs 中 ok 占比 |
| AI 采纳率 | `ai_adoption_rate` | feedback ∈ {helpful, applied} 占比 |
| MTTR / MTTA | `mttr_p50/p75` / `mtta_p50` | 事件解决/响应时长 |
| 变更失败率 | `change_failure_rate` | rolled_back 或 24h 内复发占比 |
| 告警噪音比 | `alert_noise_ratio` | (reopen+flap)/resolved |
| Skill/记忆命中 | `skill_hit_runs` / `memory_hit_runs` | 学习资产复用情况 |

## 2. 如何开启

1. AI Provider 已配置并启用（`AI.enabled` + Endpoint + Model）。
2. 无需额外开关：周一 08:00 自动推送；也可用前端
   “生成值班早报”按钮手动触发查看。

## 3. 使用建议

- 试点团队每周一例会直接引用该周报作为“AI 投入产出”依据。
- 若 MTTR 改善不明显，重点看 `verify_pass_rate` 与 `closed_loop_rate`
  是否达标（POC 验收建议：闭环率 ≥40%、验证通过率 ≥60%、采纳率 ≥50%）。
- 周报文本会沉淀到 RAG 记忆（`effect:weekly`），长期形成团队运维知识资产。

## 4. 扩展方向

- 接入邮件/飞书/钉钉定时推送（复用 `notify.go` 通道）。
- 按团队/业务服务维度拆分（`business_services` 分组）。
- 生成 PDF/HTML 周报附件供审计归档。
