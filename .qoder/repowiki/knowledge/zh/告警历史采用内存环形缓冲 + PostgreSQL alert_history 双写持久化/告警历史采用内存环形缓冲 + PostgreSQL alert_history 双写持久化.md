---
kind: design
name: 告警历史采用内存环形缓冲 + PostgreSQL alert_history 双写持久化
source: session
category: adr
---

# 告警历史采用内存环形缓冲 + PostgreSQL alert_history 双写持久化

_来源：7051a1c → 8be6209 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
现有 `alerts.go` 的 `Evaluate()` 是纯快照函数，重启后活跃告警丢失；`notify.go` 仅在 activity log 记录触发/恢复事件，无独立告警事件表；`store.go` 的 `alertStates` 仅存 ack/silence 状态。需要为告警生命周期提供可查询、可恢复的历史记录。

## 决策驱动
- 服务重启后告警历史不丢失
- 查询性能（活跃+近期历史合并返回）
- 最小侵入现有 Evaluate/dispatch 流程

## 备选方案
- **仅内存存储（ring buffer cap=500）** _（已否决）_ — 优点：实现简单、零外部依赖、启动即恢复；缺点：进程崩溃/重启数据丢失，不符合生产持久化要求
- **独立 Kafka/RabbitMQ 事件流** _（已否决）_ — 优点：高吞吐、解耦、可回放；缺点：引入额外中间件运维成本，对当前规模过度设计
- **PostgreSQL alert_history 表 + 内存最近窗口** — 优点：利用已有 PG 实例、JSONB 灵活存储告警详情、索引支持按 key/fired_at 高效查询、启动时加载最近 500 条到内存供快速响应；缺点：写入有 DB 延迟，但告警事件非高频路径可接受

## 决策
在 store.go 新增 AlertRecord 结构体与内存环形缓冲（cap=500），在 pgstore.go 新增 alert_history 表（含 key、fired_at、resolved_at、data JSONB 列及索引），notify.go 的 dispatch() 中在 fire/resolve 转换时追加写入并维护 recordIDs 映射，handleAlerts 合并 Evaluate 实时结果与历史已恢复记录返回。

## 影响
服务重启后可恢复最近 500 条告警历史；查询活跃告警时需合并 Evaluate 快照与历史 resolved 记录，存在轻微合并逻辑复杂度；PG 写入成为告警路径的潜在瓶颈，但告警频率低，影响有限。后续需考虑历史数据清理策略（如超过 N 天归档）。