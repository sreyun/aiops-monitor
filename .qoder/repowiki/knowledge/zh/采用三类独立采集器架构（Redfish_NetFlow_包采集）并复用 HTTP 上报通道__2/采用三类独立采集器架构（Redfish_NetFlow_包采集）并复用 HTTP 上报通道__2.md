---
kind: design
name: 采用三类独立采集器架构（Redfish/NetFlow/包采集）并复用 HTTP 上报通道
source: session
category: adr
---

# 采用三类独立采集器架构（Redfish/NetFlow/包采集）并复用 HTTP 上报通道

_来源：ccab58c → 4ba2fed 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
现有系统仅支持基础系统指标采集，需要扩展硬件健康监控（服务器厂商 Redfish API）、网络流量分析（NetFlow v5/v9 + 五元组包统计）能力。三类数据采集源差异大、周期不同，需统一接入现有 Agent-Server 架构。

## 决策驱动
- 零 CGO 依赖限制
- 向后兼容不破坏现有 10s 基础指标上报
- 凭证安全（密码不落盘）
- 性能保护（内存上限+背压）

## 备选方案
- **三类独立采集器 + 独立 HTTP POST 端点** — 优点：隔离故障域、各 target 独立定时器、复用 reportTransport 连接池和指纹认证；缺点：新增三个 Agent 端点和对应 Server handler
- **统一为单一 Report{} 结构混入现有上报** _（已否决）_ — 优点：改动最小；缺点：会污染基础指标通道、无法区分数据源、周期冲突

## 决策
在 cmd/agent 下新增 collector_redfish.go、collector_netflow.go、collector_packet.go 三个独立 goroutine 采集器，分别通过 POST /api/v1/agent/hardware 和 /api/v1/agent/netflow 两个独立端点上报，复用现有 reportTransport 连接池与指纹认证机制。

## 影响
Agent 新增三个独立采集模块但共享配置解析；Server 新增 handleAgentHardware/handleAgentNetFlow 处理器；NetFlow 与 Packet 共享同一端点通过 source 字段区分数据来源。