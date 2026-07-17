---
kind: design
name: 五元组包采集优先使用 /proc/net/nf_conntrack 而非 tcpdump/gopacket
source: session
category: adr
---

# 五元组包采集优先使用 /proc/net/nf_conntrack 而非 tcpdump/gopacket

_来源：a8b268a → 791b7da 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
需要在无 root 权限、无 CGO 的前提下获取主机进出流量的五元组统计。

## 决策驱动
- 零 CGO 约束
- 跨平台兼容性
- 资源占用

## 备选方案
- **gopacket/afpacket 内核抓包** _（已否决）_ — 优点：功能最全、精度最高；缺点：CGO 依赖、编译体积大、需 root
- **tcpdump 子进程 + BPF 过滤** _（已否决）_ — 优点：跨平台、BPF 精度高；缺点：需 root、stdout 解析不稳定、进程管理开销
- **/proc/net/nf_conntrack 定时读取** — 优点：纯 Go 标准库、零依赖、无需 root（只需 read 权限）；缺点：仅 Linux、仅已建连接、非实时

## 决策
P0 采用每 30s 读取 nf_conntrack 并与上次快照做差的方式估算增量流量，P1 再考虑 tcpdump 子进程作为备选。结果统一输出为 FlowRecord 格式复用 NetFlow 管道。

## 影响
只能看到 ESTABLISHED 状态的连接，无法捕获 SYN/ACK 握手阶段的短连接；但满足大多数流量分析需求且完全符合零 CGO 约束。