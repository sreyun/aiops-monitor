---
kind: design
name: 存储层按数据类型拆分：PostgreSQL 存快照/事件/明细，VictoriaMetrics 存时序数值
source: session
category: adr
---

# 存储层按数据类型拆分：PostgreSQL 存快照/事件/明细，VictoriaMetrics 存时序数值

_来源：ccab58c → 4ba2fed 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
硬件监控涉及多种数据形态：Redfish 最新快照（JSONB）、状态变更事件（离散事件）、温度/功耗等数值指标（高频时序）、NetFlow 聚合指标（高基数维度）以及可选的 Flow 明细记录。

## 决策驱动
- 查询模式匹配存储特性
- 写入性能与压缩比
- 保留历史快照 vs 实时趋势的不同需求

## 备选方案
- **PostgreSQL JSONB + VictoriaMetrics 双写** — 优点：JSONB 适合不规则硬件快照和事件查询，VM 擅长时序聚合和范围查询，各司其职；缺点：双写增加复杂度，需维护两套索引策略
- **全部存入 PostgreSQL** _（已否决）_ — 优点：单存储简化运维；缺点：JSONB 查询高频时序数据性能差，无原生降采样能力
- **全部存入 VictoriaMetrics** _（已否决）_ — 优点：高性能时序写入；缺点：不支持 JSON 列、难以做复杂关联查询和精确筛选

## 决策
Redfish 最新快照存 PostgreSQL hardware_snapshot(JSONB) 表，状态变更存 hardware_events 表；温度/功耗/风扇 RPM 等数值指标写入 VictoriaMetrics 指标 aiops_hardware_*；NetFlow 聚合指标写入 aiops_netflow_*；Flow 明细可选存 flow_records 表并 7 天 TTL 清理。

## 影响
Server 端需实现双写逻辑；VM 指标命名约定 aiops_{hardware,netflow}_*；PG 表设计需考虑 UPSERT 主键 (host_id, target_name) 和索引优化。