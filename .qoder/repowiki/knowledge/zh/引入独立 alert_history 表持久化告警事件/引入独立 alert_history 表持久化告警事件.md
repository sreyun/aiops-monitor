---
kind: design
name: 引入独立 alert_history 表持久化告警事件
source: session
category: adr
---

# 引入独立 alert_history 表持久化告警事件

_来源：1517cdd → 7051a1c 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
现有 `alerts.go` 的 `Evaluate()` 是纯快照函数，仅返回当前满足阈值的告警；`ui_api.go` 的 `handleAlerts` 每次请求调用 `Evaluate()`，导致服务重启或告警恢复后历史告警丢失。`notify.go` 仅在 activity log 中记录触发/恢复事件，`store.go` 的 `alertStates` 只存 ack/silence 状态，没有独立的告警事件存储。

## 决策驱动
- 告警事件必须跨进程/重启持久化
- 区分活跃告警与历史告警的查询语义
- 最小侵入现有 Evaluate/dispatch 流程

## 备选方案
- **复用现有 alertStates 扩展字段** _（已否决）_ — 优点：无需新表、改动最小；缺点：alertStates 设计用于瞬时状态（ack/silence），不适合承载完整事件生命周期；无法高效按时间范围查询历史
- **使用消息队列 + 外部日志系统（如 Elasticsearch）** _（已否决）_ — 优点：海量告警可水平扩展；缺点：引入额外依赖和运维复杂度；当前规模下过度设计
- **新增独立 alert_history 表（PostgreSQL JSONB）** — 优点：追加写入无锁、启动时加载最近 N 条到内存、与现有 PG store 一致；JSONB 灵活容纳不同告警类型元数据；缺点：需要维护新表结构；长周期历史需考虑归档策略

## 决策
在 PostgreSQL 新建 `alert_history` 表（含 key/fired_at/resolved_at/data JSONB 列及索引），同时在内存中维护 cap=500 的环形缓冲；`notify.go` 的 `dispatch()` 在 fire/resolve 转换时写入/更新记录，`BindPG` 启动时加载最近 500 条到内存，API 层合并 Evaluate 实时结果与历史恢复记录返回。

## 影响
告警事件获得持久化能力，支持重启恢复和历史回溯；但长期运行后 alert_history 表会持续增长，后续需引入基于 fired_at 的分区或定期归档策略；内存缓冲 cap=500 限制了单实例可见的历史窗口。