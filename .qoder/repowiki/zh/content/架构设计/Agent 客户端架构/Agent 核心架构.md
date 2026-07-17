# Agent 核心架构

<cite>
**本文引用的文件**   
- [cmd/agent/main.go](file://cmd/agent/main.go)
- [cmd/agent/reporter.go](file://cmd/agent/reporter.go)
- [cmd/agent/identity.go](file://cmd/agent/identity.go)
- [cmd/agent/hardware_aggregate.go](file://cmd/agent/hardware_aggregate.go)
- [cmd/agent/modules.go](file://cmd/agent/modules.go)
- [cmd/agent/plugins.go](file://cmd/agent/plugins.go)
- [cmd/agent/collector.go](file://cmd/agent/collector.go)
- [cmd/agent/collector_linux.go](file://cmd/agent/collector_linux.go)
- [cmd/agent/collector_redfish.go](file://cmd/agent/collector_redfish.go)
- [cmd/agent/collector_oceanstor.go](file://cmd/agent/collector_oceanstor.go)
- [cmd/agent/collector_netflow.go](file://cmd/agent/collector_netflow.go)
- [cmd/agent/collector_packet.go](file://cmd/agent/collector_packet.go)
- [cmd/agent/security_linux.go](file://cmd/agent/security_linux.go)
- [cmd/agent/tls.go](file://cmd/agent/tls.go)
- [cmd/agent/relay.go](file://cmd/agent/relay.go)
- [shared/wire.go](file://shared/wire.go)
- [config.example.json](file://config.example.json)
- [server_config.example.json](file://server_config.example.json)
- [README.md](file://README.md)
</cite>

## 更新摘要
**变更内容**   
- 增强身份管理：新增主机指纹协调机制，支持重装后自动认回既有身份
- 改进报告传输层：优化多服务器并发上报，增强错误处理和重试机制
- 新增硬件聚合功能：实现多采集器数据合并，避免告警抖动问题
- 扩展多采集器协调：支持Redfish BMC、OceanStor存储、NetFlow流量、五元组包采集
- 完善配置系统：新增各类采集器的独立配置项和参数

## 目录
1. [简介](#简介)
2. [项目结构](#项目结构)
3. [核心组件](#核心组件)
4. [架构总览](#架构总览)
5. [详细组件分析](#详细组件分析)
6. [依赖关系分析](#依赖关系分析)
7. [性能与并发特性](#性能与并发特性)
8. [配置与优先级规则](#配置与优先级规则)
9. [故障排查指南](#故障排查指南)
10. [结论](#结论)

## 简介
本文件面向 AIOps Monitor Agent 的核心架构，聚焦以下主题：Agent 启动流程、配置加载机制、多服务器并发上报、主机身份管理与持久化、信号处理与生命周期管理、安全环境检测、分类标签管理、日志采集与加密上报、中继模式（Relay）等。文档同时提供配置示例、命令行参数说明、默认值与优先级规则，以及常见问题的定位方法。

**更新** 本次更新重点增强了身份管理协调机制、报告传输层可靠性、硬件聚合功能和多采集器协调能力。

## 项目结构
Agent 位于 cmd/agent 下，采用"核心 + 平台特定实现 + 插件层 + 多采集器"的混合架构：
- 核心入口与生命周期：main.go
- 指标采集接口与平台实现：collector.go、collector_linux.go（Linux）、collector_windows.go、collector_darwin.go、collector_other.go
- 硬件采集器：collector_redfish.go（BMC/iDRAC/iLO）、collector_oceanstor.go（华为存储）
- 网络采集器：collector_netflow.go（NetFlow v5/v9接收）、collector_packet.go（五元组包采集）
- 硬件聚合器：hardware_aggregate.go（多采集器数据合并）
- 上报与多服务端广播：reporter.go
- 主机身份与指纹协调：identity.go
- 插件执行器：plugins.go
- 内置模块（Playbook 模块分发）：modules.go
- 安全环境检测（Linux）：security_linux.go
- TLS 客户端配置：tls.go
- 网关中继模式：relay.go
- 共享数据结构（与后端一致）：shared/wire.go

```mermaid
graph TB
subgraph "Agent 进程"
M["main.go<br/>启动/配置/信号"] --> R["reporter.go<br/>多目标上报/断路器"]
M --> I["identity.go<br/>主机ID/指纹协调"]
M --> HA["hardware_aggregate.go<br/>硬件数据聚合"]
M --> C["collector.go<br/>Collector 接口"]
C --> CL["collector_linux.go<br/>procfs/syscall 采集"]
M --> P["plugins.go<br/>插件发现与执行"]
M --> S["security_linux.go<br/>安全模块探测/修复建议"]
M --> T["tls.go<br/>TLS 客户端配置"]
M --> RL["relay.go<br/>网关中继模式"]
M --> MU["modules.go<br/>内置模块分发"]
R --> SH["shared/wire.go<br/>Report/Metrics/Event 数据模型"]
end
subgraph "硬件采集器"
RF["collector_redfish.go<br/>Redfish BMC采集"] --> HA
OS["collector_oceanstor.go<br/>OceanStor存储采集"] --> HA
end
subgraph "网络采集器"
NF["collector_netflow.go<br/>NetFlow接收器"] --> R
PK["collector_packet.go<br/>五元组包采集"] --> R
end
```

**图表来源**
- [cmd/agent/main.go:80-252](file://cmd/agent/main.go#L80-L252)
- [cmd/agent/reporter.go:268-328](file://cmd/agent/reporter.go#L268-L328)
- [cmd/agent/identity.go:30-48](file://cmd/agent/identity.go#L30-L48)
- [cmd/agent/hardware_aggregate.go:17-33](file://cmd/agent/hardware_aggregate.go#L17-L33)
- [cmd/agent/collector_redfish.go:95-143](file://cmd/agent/collector_redfish.go#L95-L143)
- [cmd/agent/collector_oceanstor.go:75-92](file://cmd/agent/collector_oceanstor.go#L75-L92)
- [cmd/agent/collector_netflow.go:167-200](file://cmd/agent/collector_netflow.go#L167-L200)
- [cmd/agent/collector_packet.go:29-56](file://cmd/agent/collector_packet.go#L29-L56)

章节来源
- [cmd/agent/main.go:80-252](file://cmd/agent/main.go#L80-L252)
- [shared/wire.go:120-139](file://shared/wire.go#L120-L139)

## 核心组件
- 配置与启动：解析配置文件与命令行参数，构建默认配置，应用 TLS 信任策略，选择 Relay 或正常模式，初始化主机 ID、采集器、插件运行器，注册信号处理器并进入主循环。
- 数据采集：通过 Collector 接口在 Linux 上直接读取 procfs/syscall，其他平台回退到 core 插件；支持磁盘 IO、网络速率、连接状态、负载、进程数、GPU 等指标。
- 硬件采集：Redfish BMC 采集器支持 Dell iDRAC、HP iLO、华为 iBMC 等厂商设备；OceanStor 存储采集器专门处理华为存储阵列。
- 网络采集：NetFlow 接收器支持 v5/v9 协议被动接收；五元组包采集器通过 nf_conntrack 增量采集。
- 硬件聚合：统一合并来自不同采集器的硬件快照，按 target_name 去重，避免服务端整体替换导致的告警抖动。
- 插件层：按周期并发执行 Python/shell 插件，输出自定义指标与事件，合并后随基础指标一起上报。
- 多服务端上报：单采集一次，广播至所有目标服务端；每个目标独立连接池、重试、熔断与 gzip 降级；403 自动重新注册。
- 主机身份：基于 OS machine-id + 首张非回环 MAC 生成机器指纹，持久化到 state_file，克隆场景自动识别并重建 host_id，支持重装后认回既有身份。
- 安全环境检测：启动时探测 kysec/SELinux/AppArmor/firewalld 等，根据 --security-mode 输出修复命令或临时切换宽容模式并定时恢复。
- 中继模式：以反向代理方式将本地请求转发到上游云监控中心，拦截安装脚本重写 SERVER 地址，注入共享密钥用于鉴权。
- 日志采集：可选监听若干日志路径，增量采集并按批次上报，支持 gzip+AES-GCM 加密（由服务端下发 log_key）。

**更新** 新增了硬件采集器、网络采集器和硬件聚合器组件，增强了身份管理的协调能力。

章节来源
- [cmd/agent/main.go:80-252](file://cmd/agent/main.go#L80-L252)
- [cmd/agent/reporter.go:268-328](file://cmd/agent/reporter.go#L268-L328)
- [cmd/agent/identity.go:30-48](file://cmd/agent/identity.go#L30-L48)
- [cmd/agent/hardware_aggregate.go:17-33](file://cmd/agent/hardware_aggregate.go#L17-L33)
- [cmd/agent/collector_redfish.go:95-143](file://cmd/agent/collector_redfish.go#L95-L143)
- [cmd/agent/collector_oceanstor.go:75-92](file://cmd/agent/collector_oceanstor.go#L75-L92)
- [cmd/agent/collector_netflow.go:167-200](file://cmd/agent/collector_netflow.go#L167-L200)
- [cmd/agent/collector_packet.go:29-56](file://cmd/agent/collector_packet.go#L29-L56)

## 架构总览
下图展示 Agent 从启动到周期性上报的主流程，包括配置加载、身份协调、安全检测、多目标并发上报与断路器保护，以及多采集器协调机制。

```mermaid
sequenceDiagram
participant U as "用户/系统"
participant M as "main.go"
participant I as "identity.go"
participant H as "hardware_aggregate.go"
participant S as "security_linux.go"
participant T as "tls.go"
participant A as "reporter.Agent"
participant RF as "collector_redfish.go"
participant OS as "collector_oceanstor.go"
participant NF as "collector_netflow.go"
participant PK as "collector_packet.go"
participant RT as "reporter.serverTarget"
U->>M : 启动进程
M->>M : 加载默认配置/配置文件/命令行参数
M->>T : configureServerTLS(skipVerify, ca_cert)
alt 启用中继模式
M->>RT : runRelay(...)
RT-->>U : 监听本地端口并反向代理
else 正常模式
M->>I : loadOrCreateHostID(state_file)
M->>A : NewAgent(servers, intervals, collector, runner, hostID, category)
A->>A : Run()
A->>A : reconcileIdentity() (身份协调)
A->>S : detectSecurityEnv() / getOSDist()
A->>H : newHardwareAggregator(hostID, fp, postHardwareReport)
opt 配置了 Redfish 采集器
A->>RF : newRedfishCollector(targets, hostID, fp).run(agg.submit)
end
opt 配置了 OceanStor 采集器
A->>OS : newOceanStorCollector(targets, hostID, fp).run(agg.submit)
end
opt 配置了 NetFlow 接收器
A->>NF : newNetflowReceiver(cfg, hostID, fp).run(postNetFlowReport)
end
opt 配置了五元组包采集器
A->>PK : newPacketCollector(cfg, hostID, fp).run(postNetFlowReport)
end
loop 每 report_interval
A->>A : Collect() + RunAll() (并发执行插件)
A->>RT : sendWithRetry(report) x N (并发)
RT-->>A : 成功/失败(403重注册/400禁用gzip/断路器)
end
end
U->>M : SIGINT/SIGTERM
M-->>U : 优雅退出
```

**图表来源**
- [cmd/agent/main.go:80-252](file://cmd/agent/main.go#L80-L252)
- [cmd/agent/reporter.go:364-451](file://cmd/agent/reporter.go#L364-L451)
- [cmd/agent/identity.go:30-48](file://cmd/agent/identity.go#L30-L48)
- [cmd/agent/hardware_aggregate.go:35-51](file://cmd/agent/hardware_aggregate.go#L35-L51)
- [cmd/agent/collector_redfish.go:145-197](file://cmd/agent/collector_redfish.go#L145-L197)
- [cmd/agent/collector_oceanstor.go:94-120](file://cmd/agent/collector_oceanstor.go#L94-L120)
- [cmd/agent/collector_netflow.go:202-263](file://cmd/agent/collector_netflow.go#L202-L263)
- [cmd/agent/collector_packet.go:58-113](file://cmd/agent/collector_packet.go#L58-L113)

## 详细组件分析

### 启动流程与生命周期
- 配置加载顺序：默认值 → 配置文件（config.json）→ 命令行参数覆盖。
- 安全环境检测：启动时探测操作系统与安全模块，必要时输出修复命令或切换到宽容模式并设置定时器自动恢复。
- 中继模式：若启用 --relay，则仅作为反向代理监听本地端口，不进入采集上报循环。
- 身份协调：在 Agent.Run() 开始时调用 reconcileIdentity()，向各服务端注册并获取规范 host_id，如果服务端返回不同的 ID，则更新本地 identity 并写回状态文件。
- 多采集器启动：根据配置动态启动硬件采集器（Redfish/OceanStor）、网络采集器（NetFlow/Packet），所有采集器都通过统一的聚合器或上报函数提交数据。
- 主循环：注册所有目标服务端（带指数退避），并行启动插件循环、终端通道、转发通道、日志采集通道；高频基础指标上报循环使用 defer/recover 保证异常不中断进程。
- 信号处理：捕获 SIGINT/SIGTERM，记录日志后退出。

**更新** 新增了身份协调和多采集器启动逻辑。

章节来源
- [cmd/agent/main.go:80-252](file://cmd/agent/main.go#L80-L252)
- [cmd/agent/reporter.go:364-451](file://cmd/agent/reporter.go#L364-L451)

### 配置加载机制与优先级
- 默认配置：包含 server、interval、plugin-interval、disk-path、plugins-dir、python、state-file、category、token、listen、log_encrypt 等。
- 配置文件：支持单 server 与多 servers 数组；当 servers 非空时优先于 server+token。
- 命令行参数：覆盖配置文件与默认值；支持 --config、--server、--interval、--plugin-interval、--plugins-dir、--python、--disk-path、--category、--token、--relay、--listen、--relay-secret、--log-paths、--log-encrypt、--tls-skip-verify、--ca-cert、--security-mode 等。
- TLS 信任：支持自定义 CA 证书与跳过校验（仅实验室/自签场景）。
- 新增采集器配置：支持 redfish_targets、oceanstor_targets、netflow、packet_capture 等独立配置项。

**更新** 新增了多种采集器的配置支持。

章节来源
- [cmd/agent/main.go:24-68](file://cmd/agent/main.go#L24-L68)
- [cmd/agent/tls.go:19-39](file://cmd/agent/tls.go#L19-L39)
- [config.example.json:1-16](file://config.example.json#L1-L16)

### 多服务器并发上报机制
- 目标隔离：每个 serverTarget 拥有独立的 http.Client（连接池隔离）、token、注册状态、重试与断路器。
- 并发广播：一次采集结果并发发送至所有目标，互不影响；任一目标失败不会阻塞其他目标。
- 重试与降级：同一周期内最多重试 3 次；遇到 400 且已压缩则禁用 gzip 并立即重试；遇到 403 则先重新注册再重试。
- 断路器：连续失败达到阈值打开断路器，暂停向该目标上报一段时间，并在半开时尝试恢复；打开时重置注册标记以便下次成功上报后重新注册。
- 事件去重：仅当全部目标均失败时才将事件重新入队，避免重复投递。
- 硬件上报专用通道：postHardwareReport 和 postNetFlowReport 使用独立的 HTTP POST 端点，不混入基础指标上报。

**更新** 新增了硬件和网络数据的专用上报通道。

```mermaid
classDiagram
class Agent {
-targets : []*serverTarget
-reportInterval : time.Duration
-pluginInterval : time.Duration
-collector : Collector
-plugins : *PluginRunner
-identity : Report
-httpc : *http.Client
-logPaths : []string
-logEncrypt : bool
-stateFile : string
-redfishTargets : []RedfishTarget
-oceanStorTargets : []OceanStorTarget
-netflowCfg : *NetFlowConfig
-packetCfg : *PacketConfig
+Run()
+reconcileIdentity()
+reportOnce()
+postHardwareReport()
+postNetFlowReport()
}
class serverTarget {
-server : string
-token : string
-httpc : *http.Client
-registered : bool
-canonicalHostID : string
-bo : backoff
-cb : circuitBreaker
-disableGzip : bool
-logKey : []byte
+register(base) bool
+send(rep) error
+sendWithRetry(rep) error
}
Agent --> serverTarget : "并发广播"
```

**图表来源**
- [cmd/agent/reporter.go:268-328](file://cmd/agent/reporter.go#L268-L328)
- [cmd/agent/reporter.go:657-723](file://cmd/agent/reporter.go#L657-L723)

章节来源
- [cmd/agent/reporter.go:268-328](file://cmd/agent/reporter.go#L268-L328)
- [cmd/agent/reporter.go:657-723](file://cmd/agent/reporter.go#L657-L723)

### 主机身份管理与协调
- 主机 ID：随机生成并持久化到 state_file；每次启动优先复用已有 ID。
- 防克隆指纹：结合 OS machine-id 与首张非回环 MAC 计算哈希指纹写入状态文件；若当前机器指纹与存储不一致，判定为克隆并重新生成 host_id，避免多机争用同一主机记录。
- 原子写：先写临时文件再 rename，防止崩溃导致状态损坏。
- 身份协调：reconcileIdentity() 在启动时向各服务端注册，如果服务端返回不同的 canonicalHostID，则更新本地 identity 并写回状态文件，确保重装后能认回既有身份。

**更新** 新增了身份协调机制，支持重装后自动认回既有身份。

```mermaid
flowchart TD
Start(["启动"]) --> LoadID["loadOrCreateHostID(state_file)"]
LoadID --> Reconcile["reconcileIdentity()"]
Reconcile --> Register{"向服务端注册"}
Register --> |成功| GetCanonical{"服务端返回规范ID?"}
GetCanonical --> |是| UpdateID["更新本地host_id并写回状态文件"]
GetCanonical --> |否| KeepLocal["保持本地host_id"]
Register --> |失败| SkipReconcile["跳过协调，继续运行"]
UpdateID --> End(["结束"])
KeepLocal --> End
SkipReconcile --> End
```

**图表来源**
- [cmd/agent/identity.go:30-48](file://cmd/agent/identity.go#L30-L48)
- [cmd/agent/identity.go:55-67](file://cmd/agent/identity.go#L55-L67)
- [cmd/agent/reporter.go:343-362](file://cmd/agent/reporter.go#L343-L362)

章节来源
- [cmd/agent/identity.go:30-48](file://cmd/agent/identity.go#L30-L48)
- [cmd/agent/identity.go:55-67](file://cmd/agent/identity.go#L55-L67)
- [cmd/agent/reporter.go:343-362](file://cmd/agent/reporter.go#L343-L362)

### 硬件聚合器与多采集器协调
- 聚合器设计：hardwareAggregator 维护按 target_name 分组的 HardwareSnapshot 映射，确保每个采集器的数据按目标唯一。
- 数据合并：每个采集器提交的数据都会合并到聚合器中，然后发送完整的快照集合给服务端。
- 避免告警抖动：由于服务端 hardwareStore.put 是整体替换操作，如果不合并会导致不同采集器的数据互相覆盖，产生告警 fire→resolve→fire 的抖动。
- 排序稳定：对 snapshots 按 TargetName 排序，确保上报内容稳定，便于比对和排错。

**更新** 新增硬件聚合器，解决多采集器数据冲突问题。

```mermaid
flowchart TD
RF["Redfish采集器<br/>snapshot1"] --> AGG["hardwareAggregator<br/>byTarget map"]
OS["OceanStor采集器<br/>snapshot2"] --> AGG
AGG --> Merge["按target_name合并"]
Merge --> Sort["按TargetName排序"]
Sort --> Post["postHardwareReport()"]
Post --> Server["服务端API"]
```

**图表来源**
- [cmd/agent/hardware_aggregate.go:17-51](file://cmd/agent/hardware_aggregate.go#L17-L51)

章节来源
- [cmd/agent/hardware_aggregate.go:17-51](file://cmd/agent/hardware_aggregate.go#L17-L51)

### 硬件采集器
#### Redfish 采集器
- 支持厂商：Dell iDRAC、HP iLO、华为 iBMC、Supermicro IPMI 等主流 BMC 设备。
- 兼容性：针对旧固件版本（TLS 1.0/1.1、RSA 密钥交换）进行特殊处理。
- 智能发现：自动发现 Systems 和 Chassis 路径，适配不同厂商的路径差异。
- 降频采集：固件清单、PCIe GPU、事件日志等采用降频采集 + 缓存策略。
- 错误分类：提供详细的错误信息分类和修复建议。

#### OceanStor 采集器
- 专用协议：华为 OceanStor 存储不支持 Redfish，使用 DeviceManager REST API。
- 会话管理：维护登录会话，自动处理 token 过期和重新登录。
- 数据映射：将华为特有的字段映射到标准的 HardwareSnapshot 结构。
- 健康状态：实现华为健康状态的精确映射，避免误报。

**更新** 新增两种硬件采集器，支持更多设备类型。

章节来源
- [cmd/agent/collector_redfish.go:95-143](file://cmd/agent/collector_redfish.go#L95-L143)
- [cmd/agent/collector_oceanstor.go:75-92](file://cmd/agent/collector_oceanstor.go#L75-L92)

### 网络采集器
#### NetFlow 接收器
- 双模式支持：被动接收（UDP监听）和主动采集（SNMP/REST轮询）。
- 协议兼容：支持 NetFlow v5 和 v9 协议，v9 需要模板解析。
- 内存聚合：5分钟滑动窗口聚合，按五元组 hash 统计 bytes/packets。
- 背压机制：内存上限保护（100K flows），超出后丢弃最小流量条目。

#### 五元组包采集器
- 零依赖方案：优先使用 /proc/net/nf_conntrack，无需额外依赖。
- 增量采集：通过快照差值计算增量 Flow 记录。
- 限速保护：支持 MaxPacketsPerMin 限速，避免资源耗尽。

**更新** 新增网络流量采集功能，支持网络监控需求。

章节来源
- [cmd/agent/collector_netflow.go:167-200](file://cmd/agent/collector_netflow.go#L167-L200)
- [cmd/agent/collector_packet.go:29-56](file://cmd/agent/collector_packet.go#L29-L56)

### 分类标签管理
- 分类字段：category 可在配置或命令行中设置，随 Report 一并上报，用于面板分组与过滤。
- 影响范围：分类不参与身份绑定，仅用于组织视图与筛选。

章节来源
- [cmd/agent/main.go:99-100](file://cmd/agent/main.go#L99-L100)
- [shared/wire.go:124-139](file://shared/wire.go#L124-L139)

### 安全环境检测逻辑（Linux）
- 检测项：kysec、SELinux、AppArmor、firewalld；同时识别发行版信息（ID/PrettyName/Version）。
- 行为：
  - auto：输出 enforcing 模块对应的修复命令。
  - permissive：自动切换为宽容模式，并设置定时器在指定时间后恢复 enforcing。
  - enforcing：恢复强制模式。
- 诊断：检查关键 /proc 路径是否可访问，提示权限不足时的可能原因与修复建议。

章节来源
- [cmd/agent/main.go:151-217](file://cmd/agent/main.go#L151-L217)
- [cmd/agent/security_linux.go:46-53](file://cmd/agent/security_linux.go#L46-L53)
- [cmd/agent/security_linux.go:143-165](file://cmd/agent/security_linux.go#L143-L165)
- [cmd/agent/security_linux.go:324-352](file://cmd/agent/security_linux.go#L324-L352)
- [cmd/agent/security_linux.go:294-322](file://cmd/agent/security_linux.go#L294-L322)

### 插件系统与自定义指标/事件
- 插件发现：扫描 plugins_dir，白名单允许 .py/.sh，忽略 SDK 与 dotfiles。
- 并发执行：限制最大并发子进程数量，超时控制，崩溃/超时不影响核心。
- 输出合并：基础指标（当原生不可用时回退）、自定义指标、事件列表合并后随 Report 上报。

章节来源
- [cmd/agent/plugins.go:62-100](file://cmd/agent/plugins.go#L62-L100)
- [cmd/agent/plugins.go:102-147](file://cmd/agent/plugins.go#L102-L147)
- [cmd/agent/reporter.go:423-439](file://cmd/agent/reporter.go#L423-L439)

### 中继模式（Relay）
- 功能：监听本地端口，反向代理到上游云监控中心；拦截安装脚本，重写 SERVER 指向本机，使内网机器无需直连云端。
- 安全：可选 relay_secret，注入 X-Relay-Secret 头供上游校验；对 Host 头进行严格清洗，防止命令注入。
- 传输：高 MaxIdleConnsPerHost 提升并发复用；短超时用于安装脚本拉取。

章节来源
- [cmd/agent/relay.go:31-89](file://cmd/agent/relay.go#L31-89)
- [cmd/agent/relay.go:136-189](file://cmd/agent/relay.go#L136-L189)

### 内置模块（Playbook 模块分发）
- 机制：服务端下发 modulePrefix+" "+JSON 封套命令，Agent 解析后调用内置模块（gather_facts/service/package/copy），跨系统一致执行。
- 优势：运维无需记忆各平台命令，统一通过 Playbook 编排执行。

章节来源
- [cmd/agent/modules.go:18-47](file://cmd/agent/modules.go#L18-L47)
- [cmd/agent/modules.go:49-66](file://cmd/agent/modules.go#L49-L66)
- [cmd/agent/modules.go:99-160](file://cmd/agent/modules.go#L99-L160)
- [cmd/agent/modules.go:162-239](file://cmd/agent/modules.go#L162-L239)
- [cmd/agent/modules.go:241-262](file://cmd/agent/modules.go#L241-L262)

## 依赖关系分析
- 数据契约：shared/wire.go 定义 Metrics/GPUInfo/ConnStat/DiskInfo/Report/Event/HardwareSnapshot/NetFlowReport 等结构，Agent 与后端共用，确保协议一致性。
- 采集器：collector.go 定义接口，Linux 实现直接读取 procfs/syscall，其他平台回退到 core 插件。
- 硬件采集：collector_redfish.go 和 collector_oceanstor.go 分别处理不同厂商的设备协议。
- 网络采集：collector_netflow.go 和 collector_packet.go 提供网络流量监控能力。
- 聚合器：hardware_aggregate.go 协调多个硬件采集器的数据输出。
- 上报层：reporter.go 聚合采集结果，并发广播至多个目标，具备重试、熔断、gzip 降级与注册态管理。
- 安全与 TLS：security_linux.go 负责安全模块探测与修复建议；tls.go 集中配置所有出站 HTTP 客户端的 TLS 信任策略。
- 中继：relay.go 作为反向代理，修改安装脚本并注入共享密钥。

**更新** 新增了硬件采集器、网络采集器和聚合器的依赖关系。

```mermaid
graph LR
SH["shared/wire.go"] --> COL["collector.go"]
COL --> CLX["collector_linux.go"]
SH --> REP["reporter.go"]
REP --> REL["relay.go"]
REP --> TLS["tls.go"]
REP --> PLG["plugins.go"]
REP --> MOD["modules.go"]
REP --> ID["identity.go"]
REP --> HA["hardware_aggregate.go"]
HA --> CRF["collector_redfish.go"]
HA --> COS["collector_oceanstor.go"]
REP --> CNF["collector_netflow.go"]
REP --> CPK["collector_packet.go"]
```

**图表来源**
- [shared/wire.go:120-139](file://shared/wire.go#L120-L139)
- [cmd/agent/collector.go:12-16](file://cmd/agent/collector.go#L12-L16)
- [cmd/agent/collector_linux.go:66-74](file://cmd/agent/collector_linux.go#L66-L74)
- [cmd/agent/reporter.go:268-328](file://cmd/agent/reporter.go#L268-L328)
- [cmd/agent/relay.go:31-89](file://cmd/agent/relay.go#L31-89)
- [cmd/agent/tls.go:47-73](file://cmd/agent/tls.go#L47-L73)
- [cmd/agent/plugins.go:45-55](file://cmd/agent/plugins.go#L45-L55)
- [cmd/agent/modules.go:18-47](file://cmd/agent/modules.go#L18-L47)
- [cmd/agent/identity.go:30-48](file://cmd/agent/identity.go#L30-L48)
- [cmd/agent/hardware_aggregate.go:17-33](file://cmd/agent/hardware_aggregate.go#L17-L33)
- [cmd/agent/collector_redfish.go:95-143](file://cmd/agent/collector_redfish.go#L95-L143)
- [cmd/agent/collector_oceanstor.go:75-92](file://cmd/agent/collector_oceanstor.go#L75-L92)
- [cmd/agent/collector_netflow.go:167-200](file://cmd/agent/collector_netflow.go#L167-L200)
- [cmd/agent/collector_packet.go:29-56](file://cmd/agent/collector_packet.go#L29-L56)

章节来源
- [shared/wire.go:120-139](file://shared/wire.go#L120-L139)
- [cmd/agent/collector.go:12-16](file://cmd/agent/collector.go#L12-L16)
- [cmd/agent/reporter.go:268-328](file://cmd/agent/reporter.go#L268-L328)

## 性能与并发特性
- 连接复用：reportTransport 复用连接，禁用 HTTP/2 以避免服务端重启导致的批量失败，缩短恢复时间。
- 并发上限：插件执行限制最大并发子进程数，避免资源抖动。
- 缓存策略：Linux 采集器缓存磁盘枚举与进程信息，降低频繁 I/O 开销；Redfish 采集器缓存固件清单、PCIe GPU、事件日志等。
- 重试与熔断：上报在同一周期内多次重试，配合断路器减少无效请求，提高外网稳定性。
- 压缩策略：大于阈值的 payload 才启用 gzip，遇 400 自动降级关闭压缩。
- 内存保护：NetFlow 聚合器有内存上限（100K flows）+ 背压丢弃计数。
- 降频采集：硬件采集器对低频数据进行缓存和降频采集，减少不必要的 API 调用。

**更新** 新增了硬件采集器的缓存策略和网络采集器的内存保护机制。

章节来源
- [cmd/agent/reporter.go:33-49](file://cmd/agent/reporter.go#L33-L49)
- [cmd/agent/plugins.go:116-116](file://cmd/agent/plugins.go#L116-L116)
- [cmd/agent/collector_linux.go:60-64](file://cmd/agent/collector_linux.go#L60-L64)
- [cmd/agent/reporter.go:213-253](file://cmd/agent/reporter.go#L213-L253)
- [cmd/agent/reporter.go:139-200](file://cmd/agent/reporter.go#L139-L200)
- [cmd/agent/collector_netflow.go:55-68](file://cmd/agent/collector_netflow.go#L55-L68)
- [cmd/agent/collector_redfish.go:103-142](file://cmd/agent/collector_redfish.go#L103-L142)

## 配置与优先级规则
- 优先级：命令行参数 > 配置文件 > 默认值。
- 多服务端：servers 数组非空时优先于 server+token。
- 关键参数：
  - --server/--interval/--plugin-interval/--plugins-dir/--python/--disk-path/--category/--token/--relay/--listen/--relay-secret/--config/--log-paths/--log-encrypt/--tls-skip-verify/--ca-cert/--security-mode
- 新增采集器配置：
  - redfish_targets: Redfish BMC 采集器配置数组
  - oceanstor_targets: OceanStor 存储采集器配置数组  
  - netflow: NetFlow 接收器配置对象
  - packet_capture: 五元组包采集器配置对象
- 配置文件示例：
  - 单服务端与多服务端并存，推荐生产使用 servers 数组。
- 服务端配置参考：server_config.example.json 包含告警、阈值、账户、转发等配置项。

**更新** 新增了多种采集器的配置选项。

章节来源
- [cmd/agent/main.go:24-68](file://cmd/agent/main.go#L24-L68)
- [cmd/agent/main.go:100-129](file://cmd/agent/main.go#L100-L129)
- [config.example.json:1-16](file://config.example.json#L1-L16)
- [server_config.example.json:1-36](file://server_config.example.json#L1-L36)
- [README.md:383-434](file://README.md#L383-L434)

## 故障排查指南
- 无法上报（403）：可能是 Token 失效或指纹未绑定，Agent 会自动重新注册；检查服务端 require_token 与 install_token 配置。
- 400 错误：疑似 gzip 被外网代理损坏，Agent 会禁用压缩并重试；检查中间代理是否破坏 Content-Encoding。
- 外网不稳定：断路器打开会暂停上报一段时间，等待半开探测；确认网络连通性与上游服务可用性。
- 安全模块拦截：查看启动日志中的安全模块检测结果与 /proc 路径访问诊断；按 auto 模式输出的修复命令操作，或使用 permissive 模式临时放行。
- 中继模式问题：确认 --listen 绑定地址与上游地址正确；检查 X-Relay-Secret 是否与上游 relay_secret 一致；安装脚本是否被正确改写 SERVER。
- 插件执行失败：检查插件输出是否为合法 JSON；确认 Python 解释器路径与插件扩展名在白名单内。
- 硬件采集失败：检查 Redfish/BMC 设备的网络连接、认证凭据、TLS 配置；查看具体的错误分类信息。
- NetFlow 接收失败：确认 UDP 端口未被占用，防火墙规则允许流量到达；检查 NetFlow 设备配置是否正确。
- 身份协调问题：查看 reconcileIdentity 相关日志，确认服务端返回的 canonicalHostID 是否符合预期。

**更新** 新增了硬件采集、网络采集和身份协调相关的故障排查指导。

章节来源
- [cmd/agent/reporter.go:213-253](file://cmd/agent/reporter.go#L213-L253)
- [cmd/agent/reporter.go:139-200](file://cmd/agent/reporter.go#L139-L200)
- [cmd/agent/reporter.go:452-567](file://cmd/agent/reporter.go#L452-L567)
- [cmd/agent/security_linux.go:294-322](file://cmd/agent/security_linux.go#L294-L322)
- [cmd/agent/relay.go:136-189](file://cmd/agent/relay.go#L136-L189)
- [cmd/agent/plugins.go:149-172](file://cmd/agent/plugins.go#L149-L172)
- [cmd/agent/collector_redfish.go:347-379](file://cmd/agent/collector_redfish.go#L347-L379)
- [cmd/agent/collector_netflow.go:202-263](file://cmd/agent/collector_netflow.go#L202-L263)
- [cmd/agent/reporter.go:343-362](file://cmd/agent/reporter.go#L343-L362)

## 结论
AIOps Monitor Agent 采用"核心 Go 采集 + Python 插件 + 多目标并发上报 + 多采集器协调"的混合架构，具备健壮的生命周期管理、灵活的安全环境适配、稳定的外网容错能力与开箱即用的中继模式。通过清晰的配置优先级、完善的身份持久化与指纹防克隆机制、增强的身份协调能力，以及硬件聚合器解决的多采集器数据冲突问题，Agent 能在复杂企业环境中稳定运行并提供高质量的可观测性数据。

**更新** 本次更新显著增强了 Agent 的身份管理能力、硬件监控能力和网络监控能力，使其能够更好地适应现代数据中心的多层次监控需求。