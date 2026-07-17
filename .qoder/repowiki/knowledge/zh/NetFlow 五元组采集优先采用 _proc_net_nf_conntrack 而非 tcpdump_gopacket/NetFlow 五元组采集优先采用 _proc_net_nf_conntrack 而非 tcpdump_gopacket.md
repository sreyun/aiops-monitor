---
kind: design
name: NetFlow 五元组采集优先采用 /proc/net/nf_conntrack 而非 tcpdump/gopacket
source: session
category: adr
---

# NetFlow 五元组采集优先采用 /proc/net/nf_conntrack 而非 tcpdump/gopacket

_来源：8be6209 → ccab58c 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
需要采集主机级五元组流量统计，但项目有严格的零 CGO 约束且希望保持二进制体积最小化。

## 决策驱动
- 零 CGO 依赖
- 跨平台兼容性
- 性能开销最小化
- 权限要求最低

## 备选方案
- **/proc/net/nf_conntrack 定时读取** — 优点：纯 Go 标准库实现，零依赖，仅需 read 权限，Linux 内核态已聚合；缺点：仅 Linux 平台，只能看到已建立连接，无法捕获 SYN 等握手包
- **tcpdump 子进程 + BPF 过滤** _（已否决）_ — 优点：跨平台，能捕获所有数据包包括握手阶段；缺点：需要 root 权限，stdout 解析开销大，需管理子进程生命周期
- **gopacket/afpacket 直接抓包** _（已否决）_ — 优点：功能最完整，BPF 过滤高效；缺点：CGO 依赖，引入第三方库，二进制体积显著增大

## 决策
P0 阶段采用 /proc/net/nf_conntrack 方案，每 30s 读取一次并与上次快照做差计算增量 Flow；P1 阶段作为备选保留 tcpdump 子进程方案。

## 影响
避免了 CGO 依赖和 root 权限需求，但牺牲了非 Linux 平台支持和握手阶段流量可见性。nf_conntrack 只反映已建立连接，对短连接统计存在偏差。