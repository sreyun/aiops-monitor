---
kind: design
name: 采用三类独立采集器架构（Redfish/NetFlow/Packet）
source: session
category: adr
---

# 采用三类独立采集器架构（Redfish/NetFlow/Packet）

_来源：791b7da → 9897838 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
现有系统仅支持基础系统指标采集，需要扩展硬件健康监控和网络流量分析能力。三类数据源（硬件状态、网络流、包级五元组）在采集频率、协议格式、存储需求上差异巨大，不适合混入现有的 10s 基础指标上报通道。

## 决策驱动
- 零 CGO 依赖限制
- 不同采集频率隔离
- 向后兼容不破坏现有上报通道
- 内存峰值保护

## 备选方案
- **统一采集器 + 单一上报通道** _（已否决）_ — 优点：代码复用度高，结构简洁；缺点：不同周期冲突；高吞吐 NetFlow 会阻塞基础指标；违反零 CGO 约束时难以隔离
- **三类独立采集器 + 独立 HTTP POST 端点** — 优点：采集周期互不影响；NetFlow 高吞吐不影响基础指标；错误隔离；符合零 CGO 要求；缺点：Agent 端 goroutine 数量增加；Server 端需维护多套 handler

## 决策
在 Agent 端新增三个独立 goroutine：collector_redfish.go（REST 轮询）、collector_netflow.go（UDP 监听+聚合）、collector_packet.go（nf_conntrack 读取），各自通过独立的 HTTP POST 端点（/agent/hardware、/agent/netflow）上报到 Server，与现有 /agent/report 完全解耦。

## 影响
Agent 进程复杂度上升但故障域隔离；Server 端需新增 hardware_snapshot/hardware_events/flow_records 三张 PG 表及 VictoriaMetrics 时序写入；前端新增「硬件监控」「网络流量」两个 Tab；后续 P2/P3 可渐进扩展而不影响已上线功能。