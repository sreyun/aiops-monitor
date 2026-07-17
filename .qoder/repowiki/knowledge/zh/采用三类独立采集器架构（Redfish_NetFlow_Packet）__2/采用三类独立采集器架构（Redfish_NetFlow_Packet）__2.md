---
kind: design
name: 采用三类独立采集器架构（Redfish/NetFlow/Packet）
source: session
category: adr
---

# 采用三类独立采集器架构（Redfish/NetFlow/Packet）

_来源：a8b268a → 791b7da 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
需要扩展监控能力以覆盖硬件健康（服务器固件/温度/功耗）、网络流量（五元组 Flow）和包级统计，原有基础指标采集器无法满足这些异构数据源的需求。

## 决策驱动
- 零 CGO 依赖
- 复用现有 HTTP 上报通道
- 按数据类型分离存储后端

## 备选方案
- **单一通用采集器 + 插件系统** _（已否决）_ — 优点：统一入口、易于扩展新类型；缺点：实现复杂度高；不同协议差异大难以抽象
- **三类独立采集器 goroutine** — 优点：职责清晰、故障隔离、可独立配置周期；复用现有 reportTransport；缺点：代码分散在多个文件

## 决策
为 Redfish 硬件、NetFlow 流量、五元组包采集分别实现独立 goroutine 采集器，各自通过独立的 HTTP POST 端点上报，不混入现有 10s 基础指标周期。

## 影响
Agent 结构更清晰但文件增多；Server 需新增三个 handler 和两套存储后端（PostgreSQL + VictoriaMetrics）。各采集器故障互不影响，可按需启用。