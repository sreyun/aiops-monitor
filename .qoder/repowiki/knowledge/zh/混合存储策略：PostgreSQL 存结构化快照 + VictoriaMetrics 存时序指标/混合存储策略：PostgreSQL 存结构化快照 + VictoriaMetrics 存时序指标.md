---
kind: design
name: 混合存储策略：PostgreSQL 存结构化快照 + VictoriaMetrics 存时序指标
source: session
category: adr
---

# 混合存储策略：PostgreSQL 存结构化快照 + VictoriaMetrics 存时序指标

_来源：b8c1938 → a8b268a 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
硬件快照是低频更新的 JSON 文档，温度/功耗等数值指标是高频率时序数据，NetFlow 聚合数据也是时序格式，不同类型数据有不同的查询模式和存储需求。

## 决策驱动
- 查询模式差异
- 写入频率差异
- JSON 灵活性需求
- 时序聚合性能

## 备选方案
- **全部存入 PostgreSQL** _（已否决）_ — 优点：单一数据库、事务一致性好；缺点：时序查询性能差、JSON 列索引效率低、时间序列聚合慢
- **全部存入 VictoriaMetrics** _（已否决）_ — 优点：时序查询极致性能；缺点：不支持 JSON 文档、不适合结构化快照查询
- **PostgreSQL（快照/事件）+ VictoriaMetrics（数值指标）** — 优点：各取所长、查询性能最优、符合数据特征；缺点：双写复杂性、数据一致性需应用层保证

## 决策
Redfish 最新快照和硬件事件存入 PostgreSQL（hardware_snapshot JSONB 表 + hardware_events 事件表），温度/功耗等数值指标和 NetFlow 聚合数据写入 VictoriaMetrics，通过 host_id 关联。

## 影响
充分发挥了两种存储的优势，但需要应用层保证数据一致性；PostgreSQL 负责精确查询和文档存储，VictoriaMetrics 负责高效的时间序列聚合查询。