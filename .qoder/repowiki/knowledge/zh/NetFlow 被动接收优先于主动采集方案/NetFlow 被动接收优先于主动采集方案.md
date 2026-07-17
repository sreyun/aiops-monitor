---
kind: design
name: NetFlow 被动接收优先于主动采集方案
source: session
category: adr
---

# NetFlow 被动接收优先于主动采集方案

_来源：6438b62 → b8c1938 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
网络流量采集需要支持两种模式：被动接收（设备推送）和主动轮询（SNMP/REST）。需要在资源受限的 Agent 上选择最优实现路径。

## 决策驱动
- 零依赖
- 低 CPU 占用
- 跨平台兼容性

## 备选方案
- **gopacket/afpacket 内核级抓包** _（已否决）_ — 优点：功能最完整、BPF 过滤性能好；缺点：CGO 依赖、编译体积大、跨平台兼容性差
- **tcpdump 子进程 + BPF 过滤** _（已否决）_ — 优点：跨平台、BPF 过滤能力强；缺点：需要 root 权限、stdout 解析开销大、进程管理复杂
- **/proc/net/nf_conntrack 增量读取** — 优点：零依赖、纯 Go 实现、Linux 原生接口；缺点：仅 Linux 可用、只能看到已建立连接
- **NetFlow v5/v9 UDP 被动接收** — 优点：标准协议、网络设备原生支持、零额外部署；缺点：依赖上游设备正确配置 Flow 导出

## 决策
P0 优先实现 NetFlow v5/v9 被动接收（UDP :2055 监听）+ nf_conntrack 五元组快照差分；P1 再扩展 SNMP 主动采集和 tcpdump 备选方案。聚合器使用内存窗口按五元组 hash 聚合，设置 100K flows 上限和背压丢弃计数。

## 影响
被动模式无需额外安装软件但依赖网络设备配置；nf_conntrack 无法捕获未建立连接的流量；需要设计合理的滑动窗口大小和 flush 频率平衡内存与延迟。