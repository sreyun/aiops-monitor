---
kind: design
name: NetFlow 被动接收优先于主动采集，采用 5 分钟滑动窗口聚合 + 内存上限保护
source: session
category: adr
---

# NetFlow 被动接收优先于主动采集，采用 5 分钟滑动窗口聚合 + 内存上限保护

_来源：b6152eb → 451f07a 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
网络设备可能推送 NetFlow v5/v9 数据到 Agent，也可能需要 Agent 主动轮询 SNMP/REST 接口获取 Flow 统计，两种模式的数据源、协议、延迟特性差异大。

## 决策驱动
- 被动模式零侵入设备配置
- 高吞吐场景下的内存保护
- 可配置的聚合粒度

## 备选方案
- **先实现主动采集（SNMP/REST）** _（已否决）_ — 优点：无需设备侧配合，部署简单；缺点：轮询间隔受限于设备 API 限流；无法捕获瞬时突发流量；增加对第三方设备的适配成本
- **被动接收优先（UDP 监听）+ 主动采集作为 P1 补充** — 优点：被动模式由设备主动推送，无轮询开销；v5/v9 解析后写入 flowAggregator 内存窗口，5 分钟滑动窗口聚合 bytes/packets；内存上限 100K flows 保护 + dropped_packets 计数；后续可扩展 SNMP/REST 主动采集；缺点：需要设备侧配置将 Flow 推送到 Agent 端口；UDP 丢包需靠丢弃计数暴露问题

## 决策
P0 实现 NetFlow 被动接收：`net.ListenPacket("udp", ":2055")` 监听，v5 直接解析 48-byte flow records，v9 先缓存 Template FlowSet（ID=0）再用模板解码 Data FlowSet（ID=256+），结果写入 `flowAggregator` 内存窗口（5 分钟滑动窗口、最多 100K 条目、超出丢弃最小流量条目并告警），窗口到期 flush 为 `NetFlowReport` 通过 `/api/v1/agent/netflow` 上报。P1 再扩展 SNMP/REST 主动采集。

## 影响
被动模式下设备需提前配置 Flow 推送目标；聚合器内存上限防止 OOM 但会丢失低流量 flow；dropped_packets 指标用于观测丢包率；NetFlow 与 Packet 采集共享同一端点和 `FlowRecord` 结构体，通过 source 字段区分数据来源。