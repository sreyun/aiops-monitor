---
kind: design
name: 采用三类独立采集器架构（Redfish/NetFlow/包采集）并复用 HTTP 上报通道
source: session
category: adr
---

# 采用三类独立采集器架构（Redfish/NetFlow/包采集）并复用 HTTP 上报通道

_来源：b8c1938 → a8b268a 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
现有系统仅支持基础系统指标采集，需要扩展硬件健康监控和网络流量分析能力。需要在不破坏现有 10s 基础指标上报机制的前提下，新增三类不同数据特征的采集器。

## 决策驱动
- 零 CGO 依赖限制
- 向后兼容要求
- 不同数据类型差异化存储需求
- Agent 端内存保护

## 备选方案
- **统一采集框架 + 插件化** _（已否决）_ — 优点：架构统一、易于扩展新采集器；缺点：实现复杂度高、引入额外抽象层、违反零 CGO 约束
- **三类独立 goroutine + 独立 HTTP POST 端点** — 优点：实现简单、互不影响、可独立配置周期、复用现有 reportTransport；缺点：代码分散、共享结构体需维护
- **外部采集器进程通过 gRPC 通信** _（已否决）_ — 优点：语言无关、资源隔离好；缺点：增加部署复杂度、gRPC 依赖、进程间通信开销

## 决策
在 Agent 中为 Redfish、NetFlow、包采集分别创建独立 goroutine，各自使用独立的 HTTP POST 端点（/agent/hardware、/agent/netflow），复用现有的 reportTransport 连接池和指纹认证机制。

## 影响
实现了三类采集器的解耦运行，但需要维护三个独立的错误处理和重试逻辑；NetFlow 和 Packet 共享同一端点通过 source 字段区分，减少了 API 数量。