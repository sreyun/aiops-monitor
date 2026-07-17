---
kind: design
name: 混合存储策略：PostgreSQL 存快照/事件 + VictoriaMetrics 存时序指标
source: session
category: adr
---

# 混合存储策略：PostgreSQL 存快照/事件 + VictoriaMetrics 存时序指标

_来源：4ba2fed → 27b8c5e 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
硬件监控需要两类截然不同的查询模式：Redfish 最新快照和状态变更事件适合关系型查询，而温度/功耗/NetFlow 等数值指标需要高效的时间范围聚合查询。

## 决策驱动
- 结构化查询能力（JSONB 列）
- 时序聚合性能
- 离散事件审计
- 存储成本优化

## 备选方案
- **全部存入 PostgreSQL** _（已否决）_ — 优点：单数据库运维，事务一致性好；缺点：时间范围聚合查询慢；大表膨胀严重；不适合高频时序写入
- **全部存入 VictoriaMetrics** _（已否决）_ — 优点：时序查询极快，压缩率高；缺点：不支持 JSON 结构查询；离散事件（如固件升级记录）建模困难；无 ACID 保障
- **PostgreSQL（快照/事件/可选明细）+ VictoriaMetrics（数值指标）** — 优点：各取所长：PG 存结构化数据和事件，VM 存高频时序；hardware_snapshot 用 JSONB 保留完整硬件信息；缺点：双存储运维复杂度；需要协调两次写入的事务边界

## 决策
Redfish 最新快照存 PostgreSQL `hardware_snapshot` 表（JSONB），状态变更存 `hardware_events` 表；温度/功耗等数值指标写 VictoriaMetrics；NetFlow Flow 明细可选存 PG 并带 7 天 TTL 清理。

## 影响
Server 端每次收到硬件快照需同时写 PG 和 VM；但获得了灵活的查询能力——既能查「某台主机当前 CPU 型号」，也能查「过去 24h 温度趋势」。