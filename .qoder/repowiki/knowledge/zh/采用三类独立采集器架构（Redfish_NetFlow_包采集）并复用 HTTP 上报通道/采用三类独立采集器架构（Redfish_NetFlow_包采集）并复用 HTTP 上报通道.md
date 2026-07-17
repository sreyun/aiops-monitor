---
kind: design
name: 采用三类独立采集器架构（Redfish/NetFlow/包采集）并复用 HTTP 上报通道
source: session
category: adr
---

# 采用三类独立采集器架构（Redfish/NetFlow/包采集）并复用 HTTP 上报通道

_来源：8be6209 → ccab58c 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
现有系统仅支持基础系统指标采集，需要扩展硬件健康监控和网络流量分析能力。需要在不破坏现有 10s 基础指标上报机制的前提下，新增硬件、网络两类高价值但数据特征差异巨大的采集场景。

## 决策驱动
- 零 CGO 依赖限制
- 向后兼容要求
- 不同采集器数据模型差异大
- Agent 资源占用可控

## 备选方案
- **统一采集框架 + 插件化** _（已否决）_ — 优点：架构整洁，新采集器开发成本低；缺点：需要重构现有代码，引入额外抽象层，违反零 CGO 约束
- **独立 goroutine + 独立 HTTP POST 端点** — 优点：最小侵入现有代码，各采集器互不影响，配置 omitempty 实现按需启动；缺点：多个 HTTP 连接，代码重复度略高
- **外部 sidecar 进程** _（已否决）_ — 优点：语言无关，隔离性好；缺点：增加部署复杂度，进程间通信开销

## 决策
在 Agent 中为 Redfish、NetFlow、包采集分别创建独立 goroutine，各自通过独立的 HTTP POST 端点（/agent/hardware、/agent/netflow）上报，复用现有的 reportTransport 连接池和指纹认证机制。

## 影响
实现了三类采集器的解耦，新增采集器无需修改核心逻辑；但需要维护三个不同的上报协议结构体（HardwareReport、NetFlowReport、FlowRecord）。配置项全部 omitempty 保证了未启用时不产生额外开销。