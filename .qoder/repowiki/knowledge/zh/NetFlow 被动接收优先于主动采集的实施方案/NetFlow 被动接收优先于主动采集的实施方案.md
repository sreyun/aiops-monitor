---
kind: design
name: NetFlow 被动接收优先于主动采集的实施方案
source: session
category: adr
---

# NetFlow 被动接收优先于主动采集的实施方案

_来源：791b7da → 9897838 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
网络设备（防火墙/交换机）推送 NetFlow 是标准做法，但某些场景下设备不支持或无法配置推送到 Agent。需要在实现顺序上权衡部署成本和覆盖范围。

## 决策驱动
- 部署成本最低
- 覆盖主流厂商设备
- 实现复杂度可控

## 备选方案
- **先实现 SNMP/REST 主动采集** _（已否决）_ — 优点：无需改造网络设备配置；缺点：需适配华为/H3C/Cisco 等多厂商 API；SNMP 性能差且易被限速；开发量大
- **先实现 UDP 被动接收（P0），再扩展主动采集（P1）** — 优点：直接利用设备原生 NetFlow 推送能力；纯 Go 标准库实现 v5/v9 解析；部署即插即用；缺点：需要先在防火墙上配置 flow export 指向 Agent IP:2055

## 决策
P0 阶段只实现 NetFlow v5/v9 被动接收模式，通过 `net.ListenPacket("udp", ":2055")` 监听，v9 额外缓存 Template FlowSet；P1 阶段再扩展 ActiveTargets 列表支持 SNMP/REST 主动轮询。

## 影响
部署时需确保网络设备能访问 Agent 的 2055 UDP 端口；v9 模板缓存增加了约 4KB 内存占用；P1 阶段需引入 gosnmp 第三方库（仍满足纯 Go 约束）。