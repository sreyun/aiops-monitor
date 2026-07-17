---
kind: design
name: 三类采集器（Redfish/NetFlow/包采集）采用独立 HTTP POST 端点上报，复用 reportTransport 连接池
source: session
category: adr
---

# 三类采集器（Redfish/NetFlow/包采集）采用独立 HTTP POST 端点上报，复用 reportTransport 连接池

_来源：7051a1c → 8be6209 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
系统需扩展硬件监控（Redfish）、网络流量（NetFlow v5/v9 + nf_conntrack）两类新采集能力，原有 10s 基础指标上报通道无法承载不同周期、不同结构的遥测数据。

## 决策驱动
- 三类采集器周期差异大（60s~3600s vs 10s）
- 数据结构差异大（HardwareSnapshot/NetFlowReport/FlowRecord）
- 零 CGO 依赖、复用现有认证与连接池
- 向后兼容：未配置时 goroutine 不启动

## 备选方案
- **复用现有 /agent/report 端点，通过 type 字段区分** _（已否决）_ — 优点：单一入口、无需新增路由；缺点：type 枚举膨胀、解析分支复杂、不同采集器周期混在一起调度难以优化
- **独立 HTTP POST 端点 + 独立 Transport** _（已否决）_ — 优点：各采集器独立超时/重试/连接池配置，互不影响；缺点：端口/路由增多
- **独立 HTTP POST 端点，复用 reportTransport 连接池与指纹认证** — 优点：共享底层连接池避免连接爆炸，复用认证逻辑，Agent 端只需新增三个 handler 注册；缺点：Server 端 handlers.go 需新增三个处理函数

## 决策
Agent 端新增 collector_redfish.go、collector_netflow.go、collector_packet.go 三个独立 goroutine，分别 POST 到 `/api/v1/agent/hardware`、`/api/v1/agent/netflow`（NetFlow 与 Packet 共享同一端点，通过 source 字段区分）；Server 端 handlers.go 新增 handleAgentHardware/handleAgentNetFlow 两个处理器，复用现有 reportTransport 连接池和指纹认证。

## 影响
Agent 端新增三个采集模块文件，Server 端新增两个上报处理器；NetFlow 与 Packet 共享端点简化了 Agent 侧实现但增加了 source 字段校验逻辑；所有新配置项使用 omitempty，未启用时不启动 goroutine，保证向后兼容。