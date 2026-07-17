---
kind: design
name: 采用三类独立采集器架构（Redfish/NetFlow/包采集）并通过专用 HTTP 端点上报
source: session
category: adr
---

# 采用三类独立采集器架构（Redfish/NetFlow/包采集）并通过专用 HTTP 端点上报

_来源：4ba2fed → 27b8c5e 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
现有系统仅支持基础系统指标采集，需要扩展硬件健康监控和网络流量分析能力。三类数据源（服务器硬件、网络流、五元组包）在采集频率、协议格式、存储需求上差异巨大，无法复用现有的 10s 基础指标上报通道。

## 决策驱动
- 零 CGO 依赖约束
- 向后兼容（未配置不启动）
- 凭证安全（密码不落盘）
- 性能隔离（高频 NetFlow 不影响基础指标）

## 备选方案
- **统一采集器 + 单一上报通道** _（已否决）_ — 优点：代码复用度高，架构简单；缺点：不同频率的采集会互相阻塞；NetFlow 高吞吐会拖垮基础指标上报；难以实现内存上限保护
- **三类独立采集器 + 专用 HTTP POST 端点** — 优点：采集周期互不影响；NetFlow 可独立做内存聚合和背压；Agent 到 Server 复用 reportTransport 连接池和指纹认证；缺点：新增多个端点和 goroutine；Server 端需维护多套处理逻辑

## 决策
为 Redfish（POST /api/v1/agent/hardware）、NetFlow/Packet（POST /api/v1/agent/netflow）分别创建独立采集器和上报通道，每个 target 使用独立定时器，通过共享的 reportTransport 复用连接池和认证。

## 影响
Agent 侧新增三个 goroutine 管理各自生命周期；Server 端需实现 handleAgentHardware 和 handleAgentNetFlow 两个新 handler；但保证了采集稳定性，NetFlow 丢包不会影响基础监控。