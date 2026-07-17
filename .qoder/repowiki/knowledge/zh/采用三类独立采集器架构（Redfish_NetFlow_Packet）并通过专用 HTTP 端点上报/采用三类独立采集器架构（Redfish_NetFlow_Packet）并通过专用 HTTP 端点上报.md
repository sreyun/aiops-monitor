---
kind: design
name: 采用三类独立采集器架构（Redfish/NetFlow/Packet）并通过专用 HTTP 端点上报
source: session
category: adr
---

# 采用三类独立采集器架构（Redfish/NetFlow/Packet）并通过专用 HTTP 端点上报

_来源：b6152eb → 451f07a 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
AIOPS 监控需要同时采集硬件状态（Redfish）、网络流量（NetFlow v5/v9 + 五元组包）两类异构数据，它们的数据模型、采集频率、存储后端完全不同，无法复用现有 10s 基础指标上报通道。

## 决策驱动
- 零 CGO 依赖
- 向后兼容（未配置不启动）
- 凭证安全（密码仅环境变量读取）
- 性能保护（内存上限+背压）

## 备选方案
- **统一采集器 + 单一上报通道** _（已否决）_ — 优点：代码集中、结构简洁；缺点：不同采集周期互相干扰；NetFlow 的 UDP 监听与 HTTP POST 混在同一 goroutine 难以隔离故障；无法复用现有 reportTransport
- **三类独立采集器 + 专用 Agent 端点** — 优点：各采集器独立 goroutine + 独立定时器，互不影响；Agent → Server 使用独立 HTTP POST 端点（/api/v1/agent/hardware, /api/v1/agent/netflow），复用连接池和指纹认证；支持 omitempty 配置实现按需启用；缺点：新增多个文件、多套结构体定义；Server 端需维护多条处理链路

## 决策
在 Agent 中为 Redfish、NetFlow、Packet 三类采集器分别创建独立 goroutine，各自按配置的 interval 定时触发；通过独立的 HTTP POST 端点上报到 Server（hardware 走 PostgreSQL JSONB + VM 时序，netflow 走 VM + 可选 PG 明细），复用现有的 reportTransport 连接池和指纹认证机制。

## 影响
Agent 进程复杂度上升但故障域隔离良好；Server 端 handlers.go/pgstore.go 需扩展三条写入路径；前端新增「硬件监控」「网络流量」两个 Tab；所有新配置项 omitempty 保证向后兼容，未配置时对应 goroutine 不会启动。