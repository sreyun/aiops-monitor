---
kind: design
name: 时序指标落 VictoriaMetrics，结构化快照/事件落 PostgreSQL
source: session
category: adr
---

# 时序指标落 VictoriaMetrics，结构化快照/事件落 PostgreSQL

_来源：7051a1c → 8be6209 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
新采集器产生两类数据：Redfish 数值指标（温度/功耗/风扇 RPM）和 NetFlow 聚合指标（bytes/packets per 五元组），以及 Redfish 最新快照和硬件事件等结构化数据。需要选择合适的存储后端。

## 决策驱动
- 时序数据范围查询性能（24h/7d 趋势图）
- 结构化快照需要关联主机信息
- 零额外依赖（VM 可能已部署或可轻量集成）
- Flow 明细可选存储且需 TTL 清理

## 备选方案
- **全部落 PostgreSQL（timescaledb 扩展）** _（已否决）_ — 优点：单数据库运维、事务一致性；缺点：timescaledb 增加部署复杂度，高基数五元组维度下压缩效率不如专用 TSDB
- **全部落 VictoriaMetrics** _（已否决）_ — 优点：统一时序后端、高压缩比、原生支持 range 查询；缺点：结构化快照/事件用标签建模不够直观，JSON 查询不便
- **混合存储：数值指标→VictoriaMetrics，结构化快照/事件→PostgreSQL** — 优点：各司其职——VM 擅长时间序列聚合查询，PG 擅长结构化关联查询；VM 指标名遵循 aiops_* 前缀约定；缺点：双存储写入路径，需维护两套 schema

## 决策
Redfish 数值指标（aiops_hardware_temperature/fan_rpm/power_watts/health_score）和 NetFlow 聚合指标（aiops_netflow_bytes/packets/dropped）写入 VictoriaMetrics；Redfish 最新快照存入 hardware_snapshot 表（host_id+target_name 主键 UPSERT），状态变更事件存入 hardware_events 表；NetFlow Flow 明细可选写入 flow_records 表并配合 Server 定时任务执行 7 天 TTL 清理。

## 影响
Server 端 handlers 需同时调用 VM 写入器和 PG 写入器，增加一次请求的 I/O 路径；VM 指标命名约定需在文档中固化；flow_records 表的 7 天 TTL 清理任务需作为后台 goroutine 运行，需考虑清理期间的锁竞争。