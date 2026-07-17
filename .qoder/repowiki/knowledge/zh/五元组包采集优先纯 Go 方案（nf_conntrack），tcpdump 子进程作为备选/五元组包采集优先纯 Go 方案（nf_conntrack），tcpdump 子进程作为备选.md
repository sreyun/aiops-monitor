---
kind: design
name: 五元组包采集优先纯 Go 方案（nf_conntrack），tcpdump 子进程作为备选
source: session
category: adr
---

# 五元组包采集优先纯 Go 方案（nf_conntrack），tcpdump 子进程作为备选

_来源：4ba2fed → 27b8c5e 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
需要在 Linux 服务器上采集五元组级别的包统计，有多种技术路线可选，但项目有「零 CGO」硬性约束。

## 决策驱动
- 零 CGO 依赖
- Linux 平台特性利用
- 实现复杂度
- 跨平台兼容性

## 备选方案
- **gopacket/afpacket（内核态抓包）** _（已否决）_ — 优点：功能最全，性能最好；缺点：CGO 依赖，违反零 CGO 约束；编译体积大
- **tcpdump 子进程 + BPF 过滤** — 优点：跨平台，BPF 精准过滤；缺点：需 root 权限；stdout 解析不稳定；进程管理复杂
- **/proc/net/nf_conntrack 读取（P0）+ tcpdump 备选（P1）** — 优点：P0 纯 Go 零依赖，利用 Linux conntrack 子系统；P1 提供降级方案；缺点：P0 仅 Linux 且只捕获已建立连接；P1 需要 root

## 决策
P0 通过定时读取 `/proc/net/net/nf_conntrack` 解析五元组，与上次快照做差得到增量；P1 用 exec.Command 启动 tcpdump 子进程作为跨平台备选。

## 影响
P0 方案在 Linux 上零依赖且轻量，但只能看到 ESTABLISHED 状态的连接；tcpdump 方案能抓到所有包但需要 root 权限和更复杂的进程管理。