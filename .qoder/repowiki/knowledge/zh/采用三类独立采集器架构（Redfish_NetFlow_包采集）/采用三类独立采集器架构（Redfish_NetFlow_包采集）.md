---
kind: design
name: 采用三类独立采集器架构（Redfish/NetFlow/包采集）
source: session
category: adr
---

# 采用三类独立采集器架构（Redfish/NetFlow/包采集）

_来源：6438b62 → b8c1938 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
现有系统仅支持基础系统指标采集，需要扩展硬件健康监控和网络流量分析能力。需要在不破坏现有 10s 基础指标上报通道的前提下，新增三类不同数据特征的采集器。

## 决策驱动
- 零 CGO 依赖
- 向后兼容
- 性能保护
- 凭证安全

## 备选方案
- **统一采集框架 + 插件机制** _（已否决）_ — 优点：架构统一、易于扩展新采集器；缺点：开发复杂度高、引入新的运行时开销、与现有代码耦合风险大
- **三类独立 goroutine 采集器 + 独立 HTTP POST 端点** — 优点：隔离故障域、各采集器可独立配置周期、复用现有 reportTransport 连接池、零侵入式扩展；缺点：代码分散在多个文件、共享结构体需维护

## 决策
在 Agent 中为 Redfish、NetFlow、包采集分别实现独立 goroutine，各自通过独立的 HTTP POST 端点（/agent/hardware、/agent/netflow）上报，复用现有的 reportTransport 连接池和指纹认证机制。

## 影响
Agent 进程复杂度增加但故障隔离良好；Server 端需新增对应 handler 和存储逻辑；三种采集器的错误处理和退避策略需保持一致的健壮性模式。