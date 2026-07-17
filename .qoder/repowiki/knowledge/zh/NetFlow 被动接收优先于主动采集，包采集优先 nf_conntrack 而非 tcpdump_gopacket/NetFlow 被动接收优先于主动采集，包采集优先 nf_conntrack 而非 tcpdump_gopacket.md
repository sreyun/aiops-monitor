---
kind: design
name: NetFlow 被动接收优先于主动采集，包采集优先 nf_conntrack 而非 tcpdump/gopacket
source: session
category: adr
---

# NetFlow 被动接收优先于主动采集，包采集优先 nf_conntrack 而非 tcpdump/gopacket

_来源：ccab58c → 4ba2fed 提交周期内记录的编码计划——内容为规划时意图，实现可能滞后或有出入。_

**状态：** accepted

## 背景
网络流量采集有两种路径：被动接收（设备推送 NetFlow 到 Agent UDP）和主动采集（轮询 SNMP/REST），包采集也有三种方案选择。需要在功能完整性、部署复杂度、跨平台兼容性之间权衡。

## 决策驱动
- 零 CGO 约束
- 部署简单性
- Linux 环境覆盖率
- 资源占用最小化

## 备选方案
- **NetFlow 被动接收 P0 + 主动采集 P1 延后** — 优点：被动模式无需设备侧额外配置，UDP 监听零开销，P0 即可上线；缺点：需要网络设备配合推送 NetFlow
- **同时实现被动+主动采集** _（已否决）_ — 优点：覆盖更多设备类型；缺点：SNMP/REST 适配工作量大，多协议维护成本高
- **包采集用 /proc/net/nf_conntrack P0 + tcpdump P1 备选** — 优点：nf_conntrack 纯 Go 读取零依赖，tcpdump 作为 Linux 缺失时的降级方案；缺点：nf_conntrack 仅 Linux 且只捕获已建立连接
- **包采集直接用 gopacket/afpacket** _（已否决）_ — 优点：功能最全、跨平台；缺点：CGO 依赖违反项目约束，二进制体积显著增大

## 决策
NetFlow 被动接收作为 P0 优先级实现（UDP 监听 + v5/v9 解析 + 内存聚合窗口），主动采集（SNMP/REST）延至 P1；包采集首选 /proc/net/nf_conntrack 定时快照差值方案，tcpdump 子进程作为 P1 备选，明确拒绝 gopacket/afpacket 因 CGO 依赖。

## 影响
P0 阶段只能捕获 Linux 上已建立的连接，无法看到新建连接过程；NetFlow 需要网络设备配置输出流；tcpdump 方案需要 root 权限运行。