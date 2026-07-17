---
kind: design
name: 混合存储架构：PostgreSQL + VictoriaMetrics 分工
source: session
category: adr
---

# 混合存储架构：PostgreSQL + VictoriaMetrics 分工

_来源：6438b62 → b8c1938 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
新增的硬件快照、硬件事件、NetFlow 聚合数据和 Flow 明细具有不同的查询模式和生命周期特征，需要选择合适的存储后端。

## 决策驱动
- 查询性能
- 存储成本
- 运维复杂度
- 数据一致性

## 备选方案
- **单一 PostgreSQL 存储所有数据** _（已否决）_ — 优点：运维简单、事务一致性强；缺点：时序数据查询性能差、JSONB 列膨胀严重、高基数维度查询慢
- **单一 VictoriaMetrics 存储所有数据** _（已否决）_ — 优点：时序查询高性能、压缩比好；缺点：不支持结构化关联查询、JSON 字段查询能力弱、无事务保证
- **PostgreSQL（结构化快照/事件）+ VictoriaMetrics（时序指标）** — 优点：各司其职、查询性能最优、符合数据特征；缺点：双存储运维复杂度、数据同步一致性需考虑

## 决策
PostgreSQL 存储 hardware_snapshot（最新快照 JSONB）、hardware_events（离散事件）、可选的 flow_records（7天 TTL 清理）；VictoriaMetrics 存储 aiops_hardware_* 和 aiops_netflow_* 数值指标；Server 端 handler 负责将同一份数据写入两个存储。

## 影响
需要维护两套存储的 schema 和索引；硬件快照的 JSONB 字段需要定期归档或清理；Flow 明细表必须实现定时清理任务避免无限增长。