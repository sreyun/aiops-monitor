---
kind: design
name: 存储层按数据类型拆分：PostgreSQL 存结构化快照 + VictoriaMetrics 存时序指标
source: session
category: adr
---

# 存储层按数据类型拆分：PostgreSQL 存结构化快照 + VictoriaMetrics 存时序指标

_来源：8be6209 → ccab58c 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
新增的硬件快照和网络流量数据具有两种截然不同的访问模式：硬件状态需要精确查询最新值和历史事件，而温度/功耗/流量统计需要高效的时序范围聚合查询。

## 决策驱动
- 查询模式差异大
- 数据量级不同
- 历史保留策略不同
- 运维成本

## 备选方案
- **单一 PostgreSQL 存储** _（已否决）_ — 优点：运维简单，事务一致性强；缺点：时序聚合查询性能差，JSONB 列索引效率有限，不适合高频写入
- **单一 VictoriaMetrics 存储** _（已否决）_ — 优点：时序查询性能极佳，压缩率高；缺点：不支持复杂关联查询，JSON 字段表达能力弱，不适合结构化快照
- **PostgreSQL + VictoriaMetrics 混合** — 优点：PG 存结构化快照/事件（hardware_snapshot JSONB, hardware_events），VM 存数值指标（temperature/fan_rpm/power_watts/netflow_bytes/packets）；缺点：双存储运维复杂度，数据一致性需应用层保证

## 决策
Redfish 最新快照存入 PostgreSQL `hardware_snapshot` 表（JSONB 列），状态变更事件存入 `hardware_events` 表；温度/功耗/风扇 RPM 等数值指标写入 VictoriaMetrics；NetFlow 聚合指标同样写入 VM，可选的 Flow 明细记录存入 PG 的 `flow_records` 表并设置 7 天 TTL 自动清理。

## 影响
充分利用了 PG 的结构化查询能力和 VM 的时序聚合优势；但引入了双存储的数据同步复杂性，Server 端 handler 需要同时调用两个存储后端。