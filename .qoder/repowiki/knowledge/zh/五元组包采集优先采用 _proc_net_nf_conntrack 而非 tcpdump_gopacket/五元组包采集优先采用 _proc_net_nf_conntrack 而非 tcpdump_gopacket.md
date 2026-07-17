---
kind: design
name: 五元组包采集优先采用 /proc/net/nf_conntrack 而非 tcpdump/gopacket
source: session
category: adr
---

# 五元组包采集优先采用 /proc/net/nf_conntrack 而非 tcpdump/gopacket

_来源：b8c1938 → a8b268a 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
需要获取主机级别的五元组流量统计，但项目有严格的零 CGO 依赖约束，且需要跨平台考虑。

## 决策驱动
- 零 CGO 约束
- Linux 环境为主
- 轻量级实现
- 权限要求

## 备选方案
- **/proc/net/nf_conntrack 定时读取** — 优点：零依赖、无需 root、纯 Go 标准库；缺点：仅 Linux、仅已建连接、解析性能受限
- **tcpdump 子进程 + BPF 过滤** _（已否决）_ — 优点：跨平台、捕获所有包、BPF 高效过滤；缺点：需要 root 权限、stdout 解析开销大、进程管理复杂
- **gopacket/afpacket 内核态抓包** _（已否决）_ — 优点：功能最全、性能最好；缺点：CGO 依赖、编译体积大、跨平台兼容性差

## 决策
P0 阶段采用 /proc/net/nf_conntrack 每 30s 定时读取方案，与上一次快照做差计算增量 Flow 记录；P1 阶段再考虑 tcpdump 子进程方案作为备选。

## 影响
避免了 CGO 依赖和 root 权限要求，但只能统计已建立的连接，对短连接和 SYN 扫描等场景覆盖不足。