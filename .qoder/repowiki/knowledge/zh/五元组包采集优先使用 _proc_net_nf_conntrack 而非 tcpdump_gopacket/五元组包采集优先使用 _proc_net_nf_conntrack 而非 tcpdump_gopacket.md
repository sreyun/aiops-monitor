---
kind: design
name: 五元组包采集优先使用 /proc/net/nf_conntrack 而非 tcpdump/gopacket
source: session
category: adr
---

# 五元组包采集优先使用 /proc/net/nf_conntrack 而非 tcpdump/gopacket

_来源：791b7da → 9897838 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
需要在 Linux 主机上获取五元组级别的连接统计，有多种技术路径可选，但项目有严格的零 CGO 和轻量级约束。

## 决策驱动
- 零 CGO 依赖
- 最小二进制体积
- Linux 平台即可满足目标环境

## 备选方案
- **gopacket/afpacket 内核旁路抓包** _（已否决）_ — 优点：功能最全、跨平台、BPF 过滤灵活；缺点：CGO 依赖导致交叉编译复杂；二进制体积膨胀；权限要求高
- **tcpdump 子进程 + BPF 过滤** _（已否决）_ — 优点：跨平台、BPF 性能优秀；缺点：需 root 权限；stdout 解析不稳定；进程间通信开销
- **/proc/net/nf_conntrack 定时读取** — 优点：零依赖、纯 Go、无额外进程；内核态已做去重；Linux 默认启用；缺点：仅 Linux；仅已建立连接；大表时解析慢

## 决策
P0 采用 `/proc/net/nf_conntrack` 每 30s 读取一次，解析 ESTABLISHED 连接的五元组和字节数，与上次快照做差得到增量 FlowRecord；P1 再考虑 tcpdump 子进程作为备选方案。

## 影响
只能看到已建立的 TCP 连接，看不到 SYN 等握手过程；当 conntrack 表超过 100K 条时解析性能下降，需在后续版本加入采样或增量读取优化。