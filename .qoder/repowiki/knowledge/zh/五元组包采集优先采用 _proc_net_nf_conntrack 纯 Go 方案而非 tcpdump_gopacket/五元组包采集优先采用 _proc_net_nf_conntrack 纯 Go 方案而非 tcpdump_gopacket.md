---
kind: design
name: 五元组包采集优先采用 /proc/net/nf_conntrack 纯 Go 方案而非 tcpdump/gopacket
source: session
category: adr
---

# 五元组包采集优先采用 /proc/net/nf_conntrack 纯 Go 方案而非 tcpdump/gopacket

_来源：b6152eb → 451f07a 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
需要在 Linux 主机上采集五元组级别的包级流量，存在三种技术路线：读取内核 conntrack 表、fork tcpdump 子进程、使用 gopacket 的 afpacket。

## 决策驱动
- 零 CGO 依赖
- 跨平台兼容性
- 权限要求最低

## 备选方案
- **gopacket/afpacket** _（已否决）_ — 优点：功能最全、支持 BPF 过滤、跨平台；缺点：CGO 依赖导致编译体积增大、引入 C 库链接风险；不符合项目零 CGO 约束
- **tcpdump 子进程** _（已否决）_ — 优点：跨平台、BPF 过滤能力强；缺点：需要 root 权限运行；stdout 解析不稳定；进程管理复杂；违反零 CGO 外的额外二进制依赖原则
- **/proc/net/nf_conntrack 纯 Go 读取** — 优点：零依赖、无需特权、纯 Go 标准库实现；每 30s 快照差值计算增量 Flow；与 NetFlow 共享 `FlowRecord` 结构体；缺点：仅 Linux 可用；仅能看到已建立连接（ESTABLISHED），看不到 SYN/ACK 握手阶段；字节数估算精度有限

## 决策
P0 采用 `/proc/net/nf_conntrack` 方案：定时读取文件、解析每行提取 ipv4/tcp/udp 五元组 + ESTABLISHED 状态 + 字节数，与上一次快照做差得到增量 Flow 记录，格式统一为 `FlowRecord` 并通过 `/api/v1/agent/netflow` 上报（source="packet"）。P1 备选 tcpdump 子进程方案作为非 Linux 平台的回退。

## 影响
Linux 平台下零依赖、零特权即可运行；conntrack 表只反映已建连接，无法捕获握手阶段的 SYN 扫描等异常；bytes 字段是估算值，不适合精确计费但足以识别 Top-N 流量。