# Redfish硬件采集器

<cite>
**本文引用的文件**   
- [collector_redfish.go](file://cmd/agent/collector_redfish.go)
- [wire.go](file://shared/wire.go)
- [main.go](file://cmd/agent/main.go)
- [handlers.go](file://cmd/server/handlers.go)
- [hardware_netflow.go](file://cmd/server/hardware_netflow.go)
- [reporter.go](file://cmd/agent/reporter.go)
- [collector_netflow.go](file://cmd/agent/collector_netflow.go)
- [collector_packet.go](file://cmd/agent/collector_packet.go)
- [config.example.json](file://config.example.json)
- [hardware.js](file://cmd/server/web/js/hardware.js)
- [style.css](file://cmd/server/web/style.css)
- [i18n-dashboard.js](file://cmd/server/web/i18n-dashboard.js)
- [collector_linux.go](file://cmd/agent/collector_linux.go)
- [infra.go](file://cmd/agent/infra.go)
- [pgstore.go](file://cmd/server/pgstore.go)
</cite>

## 更新摘要
**已进行的更改**
- **新增设备身份发现**：完整支持制造商、型号、序列号、SKU、资产标签等整机身份信息采集
- **GPU/加速器卡分离识别**：从Processors集合中智能区分CPU与GPU/加速卡，避免混入CPU列表
- **内存DIMM详细信息采集**：完整的DIMM插槽信息（容量、类型、速率、健康状态、物理位置）
- **存储子系统增强**：RAID控制器、逻辑卷、SMART预测故障检测、SSD剩余寿命监控
- **电源供应器监控**：PSU输入输出功耗、冗余状态、额定功率、电压监控
- **BMC事件日志系统**：300秒轮询间隔、多厂商路径发现、40条限制、组件归因解析
- **前端展示增强**：专业硬件面板、交互式卡片设计、实时数据更新、多语言支持

## 目录
1. [简介](#简介)
2. [项目结构](#项目结构)
3. [核心组件](#核心组件)
4. [架构总览](#架构总览)
5. [v6.5.0重大功能增强](#v650重大功能增强)
6. [详细组件分析](#详细组件分析)
7. [依赖关系分析](#依赖关系分析)
8. [性能与容量规划](#性能与容量规划)
9. [故障排查指南](#故障排查指南)
10. [结论](#结论)
11. [附录：API 定义](#附录api-定义)

## 简介
本文件聚焦于 AIOps 监控系统中"Redfish 硬件采集器"的端到端实现，涵盖 Agent 侧 Redfish 客户端、共享数据模型、Server 侧接收与查询接口，以及与 NetFlow/五元组包采集的协同。**v6.5.0重大更新**：新增了完整的设备身份发现、GPU/加速器卡分离识别、内存DIMM详细信息采集、存储子系统增强、电源供应器监控、BMC事件日志系统等核心功能改进，为运维人员提供了全方位的硬件观测能力。

文档面向运维与研发人员，既提供高层架构说明，也给出代码级流程与关键设计权衡。通过HTTP连接稳定性修复、增强的密码解析机制、TLS兼容性支持和全面的错误分类系统，显著提升了生产环境中BMC连接的可靠性和用户体验。

## 项目结构
围绕 Redfish 硬件采集的关键路径如下：
- Agent 侧
  - Redfish 采集器：定时轮询 BMC/iDRAC/iLO 等 Redfish 端点，聚合 CPU/内存/存储/温度/风扇/电源/固件等信息，生成快照并上报。
  - 配置注入：通过配置文件中的 redfish_targets 字段启用。
- 共享数据模型
  - 统一数据结构（HardwareSnapshot、HardwareReport 等）在 shared 包中定义，确保 Agent 与 Server 契约一致。
- Server 侧
  - 接收 Agent 上报的硬件快照，持久化到 PostgreSQL，并将数值指标写入 VictoriaMetrics；同时暴露前端查询接口。
- **前端展示层**
  - 硬件监控面板：基于 JavaScript 的动态渲染，支持交互式卡片展示
  - 专业UI设计：深色主题、响应式布局、多语言支持
  - 实时数据更新：自动刷新硬件状态信息

```mermaid
graph TB
subgraph "Agent"
RF["Redfish 采集器<br/>collector_redfish.go"]
CFG["配置加载<br/>cmd/agent/main.go"]
SHARED["共享模型<br/>shared/wire.go"]
REP["硬件报告上报<br/>cmd/agent/reporter.go"]
LINUX["Linux采集器<br/>collector_linux.go"]
end
subgraph "Server"
HND["路由注册<br/>cmd/server/handlers.go"]
HW["硬件处理<br/>cmd/server/hardware_netflow.go"]
PG["PostgreSQL<br/>pgstore.go"]
VM["VictoriaMetrics"]
WEB["Web服务器<br/>静态资源"]
end
subgraph "前端展示"
JS["硬件面板JS<br/>hardware.js"]
CSS["样式设计<br/>style.css"]
I18N["国际化<br/>i18n-dashboard.js"]
UI["用户界面<br/>硬件卡片"]
end
CFG --> RF
RF --> SHARED
RF --> REP
REP --> |"POST /api/v1/agent/hardware"| HND
HND --> HW
HW --> PG
HW --> VM
WEB --> JS
WEB --> CSS
WEB --> I18N
JS --> |"GET /api/v1/hardware/health"| HND
JS --> UI
LINUX --> SHARED
```

**图表来源** 
- [collector_redfish.go:1-126](file://cmd/agent/collector_redfish.go#L1-L126)
- [main.go:223-233](file://cmd/agent/main.go#L223-L233)
- [wire.go:144-237](file://shared/wire.go#L144-L237)
- [handlers.go:290-298](file://cmd/server/handlers.go#L290-L298)
- [hardware_netflow.go:19-90](file://cmd/server/hardware_netflow.go#L19-L90)
- [reporter.go:609-644](file://cmd/agent/reporter.go#L609-L644)
- [hardware.js:1-230](file://cmd/server/web/js/hardware.js#L1-L230)
- [style.css:2808-2839](file://cmd/server/web/style.css#L2808-L2839)
- [i18n-dashboard.js:450-472](file://cmd/server/web/i18n-dashboard.js#L450-472)
- [collector_linux.go:83-98](file://cmd/agent/collector_linux.go#L83-L98)

**章节来源**
- [collector_redfish.go:1-126](file://cmd/agent/collector_redfish.go#L1-L126)
- [main.go:223-233](file://cmd/agent/main.go#L223-L233)
- [wire.go:144-237](file://shared/wire.go#L144-L237)
- [handlers.go:290-298](file://cmd/server/handlers.go#L290-L298)
- [hardware_netflow.go:19-90](file://cmd/server/hardware_netflow.go#L19-L90)

## 核心组件
- RedfishTarget：描述一个 BMC/iDRAC/iLO 目标（名称、URL、认证、TLS 策略、采集间隔）。
- redfishCollector：管理多个目标的独立 goroutine 与定时器，负责鉴权、HTTP 请求、JSON 解析、错误退避、快照合并与上报。
- HardwareSnapshot/HardwareReport：Redfish 快照与上报载荷的数据模型。
- vmHardwareMetrics：将健康分数、温度、风扇转速、功耗等指标写入时序库。
- handleAgentHardware/handleHardwareHealth/handleHardwareHistory：服务端接收与查询接口。
- postHardwareReport：增强的硬件报告上报函数，包含详细的诊断日志和响应体捕获。
- **前端硬件面板**：基于 JavaScript 的动态渲染引擎，支持交互式硬件状态展示。

**章节来源**
- [collector_redfish.go:17-126](file://cmd/agent/collector_redfish.go#L17-L126)
- [wire.go:144-237](file://shared/wire.go#L144-L237)
- [hardware_netflow.go:19-158](file://cmd/server/hardware_netflow.go#L19-L158)
- [reporter.go:609-644](file://cmd/agent/reporter.go#L609-L644)
- [hardware.js:1-230](file://cmd/server/web/js/hardware.js#L1-L230)

## 架构总览
下图展示从 Agent 采集到 Server 落库与查询的完整链路，以及三类采集器（Redfish、NetFlow、五元组包）的统一上报通道，**新增前端展示层的完整集成**。

```mermaid
sequenceDiagram
participant RF as "Redfish 采集器"
participant AG as "Agent 主进程"
participant REP as "硬件报告上报器"
participant SV as "Server 路由"
participant HW as "硬件处理器"
participant PG as "PostgreSQL"
participant VM as "VictoriaMetrics"
participant WEB as "Web服务器"
participant UI as "前端硬件面板"
RF->>RF : "按 interval_sec 定时轮询 Redfish 端点"
RF->>AG : "生成 HardwareReport"
AG->>REP : "调用 postHardwareReport()"
REP->>SV : "POST /api/v1/agent/hardware (携带指纹)"
Note over REP,SV : "v6.2.5 : 捕获响应体(≤512字节)用于诊断"
SV->>HW : "handleAgentHardware()"
HW->>PG : "upsert 硬件快照 + 事件"
HW->>VM : "push 健康/温度/风扇/功耗指标"
SV-->>REP : "200 OK"
REP->>REP : "记录INFO级别成功日志"
UI->>SV : "GET /api/v1/hardware/health?host=..."
SV->>HW : "获取最新硬件快照"
HW->>PG : "查询数据库"
PG-->>HW : "返回硬件数据"
HW-->>SV : "格式化响应"
SV-->>UI : "JSON格式硬件数据"
UI->>UI : "渲染硬件卡片界面"
Note over RF,VM : "NetFlow/五元组包采集器使用 /api/v1/agent/netflow 上报"
```

**图表来源** 
- [collector_redfish.go:56-126](file://cmd/agent/collector_redfish.go#L56-L126)
- [reporter.go:609-644](file://cmd/agent/reporter.go#L609-L644)
- [handlers.go:290-298](file://cmd/server/handlers.go#L290-L298)
- [hardware_netflow.go:19-90](file://cmd/server/hardware_netflow.go#L19-L90)
- [hardware.js:23-43](file://cmd/server/web/js/hardware.js#L23-L43)

## v6.5.0重大功能增强

### 设备身份发现系统
v6.5.0版本新增了完整的设备身份发现功能，能够准确识别服务器的制造商、型号、序列号、SKU和服务标签等关键身份信息。

#### 身份信息采集
```go
// System overview（含整机身份：厂商/型号/序列号/BIOS）
var sys struct {
    Status           redfishStatus `json:"Status"`
    Manufacturer     string        `json:"Manufacturer"`
    Model            string        `json:"Model"`
    SKU              string        `json:"SKU"`
    SerialNumber     string        `json:"SerialNumber"`
    PartNumber       string        `json:"PartNumber"`
    AssetTag         string        `json:"AssetTag"`
    HostName         string        `json:"HostName"`
    BiosVersion      string        `json:"BiosVersion"`
    PowerState       string        `json:"PowerState"`
    IndicatorLED     string        `json:"IndicatorLED"`
}
```

#### 厂商适配处理
- **华为 iBMC(RH2288 V3 / TaiShan 200)**：序列号通常只填在 Chassis 的 SerialNumber，System.SerialNumber 为空
- **Dell iDRAC**：Service Tag 放在 SKU 字段
- **兼容策略**：两边都兜底，确保序列号完整性

**章节来源**
- [collector_redfish.go:371-422](file://cmd/agent/collector_redfish.go#L371-L422)

### GPU/加速器卡分离识别
v6.5.0实现了从 Processors 集合中智能区分 CPU 与 GPU/加速器卡的功能，避免了之前 GPU 被误认为 CPU 的问题。

#### 处理器类型识别
```go
// Processors 集合里同时挂 CPU 与 GPU/加速卡，按 ProcessorType 分流，
// 否则 GPU 会被当成 CPU 混进 CPU 列表（且 GPU 信息完全看不到）。
if strings.EqualFold(p.ProcessorType, "GPU") || strings.EqualFold(p.ProcessorType, "Accelerator") {
    snap.GPUs = append(snap.GPUs, shared.RedfishGPU{
        Name:         p.Name,
        Model:        p.Model,
        Manufacturer: p.Manufacturer,
        Health:       p.Status.Health,
        State:        p.Status.State,
        MaxFreqMHz:   p.MaxSpeedMHz,
    })
    continue
}
snap.CPUs = append(snap.CPUs, shared.RedfishCPU{
    Name:       p.Name,
    Model:      p.Model,
    Cores:      p.TotalCores,
    Threads:    p.TotalThreads,
    Health:     p.Status.Health,
    MaxFreqMHz: p.MaxSpeedMHz,
})
```

#### GPU信息展示
前端支持完整的GPU信息展示，包括名称、型号、制造商、最大频率、状态和健康情况。

**章节来源**
- [collector_redfish.go:456-478](file://cmd/agent/collector_redfish.go#L456-L478)
- [hardware.js:351-357](file://cmd/server/web/js/hardware.js#L351-L357)

### 内存DIMM详细信息采集
v6.5.0增强了内存DIMM的详细信息采集，包括物理槽位、容量、类型、速率、健康状态等完整信息。

#### DIMM信息采集
```go
// Memory DIMMs (lower frequency: every 5 min)
var mems struct {
    Members []struct {
        ODataID string `json:"@odata.id"`
    } `json:"Members"`
}
for _, m := range mems.Members {
    var dimm struct {
        Name              string        `json:"Name"`
        CapacityMiB       float64       `json:"CapacityMiB"`
        MemoryDeviceType  string        `json:"MemoryDeviceType"`
        OperatingSpeedMhz int           `json:"OperatingSpeedMhz"`
        AllowedSpeedsMHz  []int         `json:"AllowedSpeedsMHz"`
        Status            redfishStatus `json:"Status"`
        DeviceLocator     string        `json:"DeviceLocator"`
        Manufacturer      string        `json:"Manufacturer"`
        PartNumber        string        `json:"PartNumber"`
        SerialNumber      string        `json:"SerialNumber"`
        RankCount         int           `json:"RankCount"`
    }
    // 空槽位也会作为成员返回（State=Absent / 容量 0）。把它们混进列表会让
    // "24 条内存" 里一半是幻影，异常计数也跟着虚高。
    if strings.EqualFold(dimm.Status.State, "Absent") || dimm.CapacityMiB <= 0 {
        continue
    }
    // DeviceLocator 才是机箱丝印上的槽位（A1/DIMM010 J10），Id 多是 "DIMM.Socket.A1"
    slot := strings.TrimSpace(dimm.DeviceLocator)
    if slot == "" {
        slot = dimm.Id
    }
}
```

#### DIMM信息展示
前端支持完整的DIMM信息表格，包括插槽位置、容量、类型、速率、制造商、部件号、序列号和状态。

**章节来源**
- [collector_redfish.go:480-534](file://cmd/agent/collector_redfish.go#L480-L534)
- [hardware.js:359-371](file://cmd/server/web/js/hardware.js#L359-L371)

### 存储子系统增强
v6.5.0大幅增强了存储子系统的监控能力，包括RAID控制器、逻辑卷、SMART预测故障检测和SSD剩余寿命监控。

#### RAID控制器采集
```go
// StorageControllers（RAID 卡）
StorageControllers []struct {
    Name            string        `json:"Name"`
    Model           string        `json:"Model"`
    Manufacturer    string        `json:"Manufacturer"`
    FirmwareVersion string        `json:"FirmwareVersion"`
    SerialNumber    string        `json:"SerialNumber"`
    SpeedGbps       float64       `json:"SpeedGbps"`
    Status          redfishStatus `json:"Status"`
    CacheSummary    struct {
        TotalCacheSizeMiB float64       `json:"TotalCacheSizeMiB"`
        PersistentCacheSizeMiB float64  `json:"PersistentCacheSizeMiB"`
        Status            redfishStatus `json:"Status"`
    } `json:"CacheSummary"`
}
```

#### SMART预测故障检测
```go
// SMART 预测故障
FailurePredicted bool          `json:"FailurePredicted"`
// SSD 剩余寿命；Redfish 里 null 表示未知，用指针区分 null 与 0%。
PredictedMediaLifeLeftPercent *float64 `json:"PredictedMediaLifeLeftPercent"`

// 此前 SMARTWarn 从未被赋值，前端却按它标红——盘的预测故障永远看不到。
SMARTWarn:    drv.FailurePredicted,
LifeLeftPct:  life,
```

#### 逻辑卷监控
```go
// 逻辑卷（RAID 组）：盘好不代表卷好——降级的 RAID5 里每块盘都可能是 OK
const vols = (sd.raid || []).flatMap(r => (r.volumes || []).map(v => ({ ctl: r.name, v })))
html += hwSection(hwT("hardware.volumes", "逻辑卷"), vols.length,
    hwTable([hwT("hardware.raid", "RAID / 存储控制器"), hwT("hardware.name", "名称"), 
             hwT("hardware.raid_level", "RAID 级别"), hwT("hardware.capacity", "容量"), 
             hwT("hardware.status", "状态"), hwT("hardware.health", "健康")],
        vols.map(({ ctl, v }) => `<tr class="${hwBadCls(v.health)}">
            <td>${esc(ctl)}</td><td>${esc(v.name)}</td>
            <td>${esc(hwDash(v.raid_type))}</td><td>${v.capacity_gb ? v.capacity_gb.toFixed(0) + "GB" : "-"}</td>
            <td>${esc(hwEnum("state", v.state) || "-")}</td><td>${hwSevChip(v.health)}</td></tr>`)));
```

**章节来源**
- [collector_redfish.go:536-653](file://cmd/agent/collector_redfish.go#L536-L653)
- [hardware.js:396-415](file://cmd/server/web/js/hardware.js#L396-L415)

### 电源供应器监控
v6.5.0实现了完整的电源供应器监控，包括输入输出功耗、冗余状态、额定功率和电压监控。

#### PSU信息采集
```go
// DMTF Redfish Power schema 的属性名是 **PowerSupplies** 与 **Redundancy**
// （"PowerSupply" 只是类型名，不是属性名）。此前写成 PowerSupply /
// PowerSupplyRedundancy，导致所有厂商的 PSU 一律解析不出来 → 前端电源区永不渲染。
PowerSupplies []struct {
    Name              string        `json:"Name"`
    PowerInputWatts   float64       `json:"PowerInputWatts"`
    PowerOutputWatts  float64       `json:"PowerOutputWatts"`
    PowerCapacityWatts float64      `json:"PowerCapacityWatts"`
    LineInputVoltage  float64       `json:"LineInputVoltage"`
    PowerSupplyType   string        `json:"PowerSupplyType"`
    Model             string        `json:"Model"`
    Manufacturer      string        `json:"Manufacturer"`
    SerialNumber      string        `json:"SerialNumber"`
    FirmwareVersion   string        `json:"FirmwareVersion"`
    Status            redfishStatus `json:"Status"`
} `json:"PowerSupplies"`
Redundancy []struct {
    Mode string `json:"Mode"`
} `json:"Redundancy"`
```

#### PSU信息展示
前端支持完整的电源信息表格，包括名称、型号、输入输出功率、额定功率、输入电压、序列号和状态。

**章节来源**
- [collector_redfish.go:694-750](file://cmd/agent/collector_redfish.go#L694-L750)
- [hardware.js:417-429](file://cmd/server/web/js/hardware.js#L417-L429)

### BMC事件日志系统
v6.5.0新增了完整的BMC事件日志系统，支持300秒轮询间隔、多厂商路径发现和40条限制。

#### 事件日志采集
```go
// selInterval throttles event-log polling. Entries only appear on real faults,
// while a full SEL fetch is one of the heaviest Redfish calls there is (old
// iDRAC8 / RH2288 V3 firmware can take seconds) — polling it every 30s would
// tax the BMC for nothing.
const selInterval = 300

// hwEventCap bounds how many BMC log entries ride along in each snapshot.
// A Dell SEL holds ~500 entries and the LC log thousands; shipping them all on
// every poll would bloat the report and the JSONB row for no operational gain.
const hwEventCap = 40
```

#### 多厂商路径发现
```go
// logServicePaths returns candidate LogService Entries endpoints, discovered
// rather than hardcoded because the naming differs per vendor:
//   - Dell iDRAC7/8/9:  Managers/iDRAC.Embedded.1/LogServices/Sel  (+ /Lclog)
//   - Huawei iBMC (RH2288 V3 / TaiShan 200): Managers/1/LogServices/Log
//   - Some models expose it under Systems/<id>/LogServices/... instead
func (rc *redfishCollector) logServicePaths(client *http.Client, t RedfishTarget, password, sysPath string) []string {
    // 优先级：SEL > 事件日志 > 其它。Dell 的 Lclog 有几千条配置变更噪声，
    // SEL 才是硬件故障；华为 iBMC 只有 Log。
    rank := func(p string) int {
        lp := strings.ToLower(p)
        switch {
        case strings.Contains(lp, "sel"):
            return 0
        case strings.Contains(lp, "eventlog"), strings.HasSuffix(lp, "/log"):
            return 1
        case strings.Contains(lp, "lclog"), strings.Contains(lp, "lifecycle"):
            return 2
        }
        return 3
    }
}
```

#### 组件归因解析
```go
// collectEvents pulls the most recent BMC log entries and resolves each one to
// the component that triggered it.
func (rc *redfishCollector) collectEvents(client *http.Client, t RedfishTarget, password, sysPath string) []shared.HardwareEvent {
    comp := strings.TrimSpace(m.Oem.Huawei.EventSubject)
    if comp == "" {
        comp = componentFromODataID(m.Links.OriginOfCondition.ODataID)
    }
    if comp == "" && len(m.MessageArgs) > 0 {
        // Dell SEL 的 MessageArgs[0] 常就是部件名（"PSU 2" / "DIMM_A3"）。
        comp = strings.TrimSpace(m.MessageArgs[0])
    }
    if comp == "" && m.SensorType != "" {
        comp = m.SensorType
        if m.SensorNumber != nil {
            comp = fmt.Sprintf("%s #%d", m.SensorType, *m.SensorNumber)
        }
    }
}
```

**章节来源**
- [collector_redfish.go:752-1022](file://cmd/agent/collector_redfish.go#L752-L1022)
- [hardware.js:302-314](file://cmd/server/web/js/hardware.js#L302-L314)

## 详细组件分析

### Redfish 采集器（Agent 侧）
- 运行模型
  - 每个目标独立 goroutine + 独立定时器，最小采集间隔 30s。
  - 启动即采集一次，随后按周期执行。
- 鉴权与安全
  - 支持 Basic Auth；密码通过环境变量读取，不落盘。
  - **新增**：增强的TLS兼容性支持，专门针对Dell iDRAC 7/8、HP iLO 3/4、Supermicro IPMI等遗留BMC固件。
  - **新增**：可选跳过 TLS 证书校验（仅内网/自签场景），并在日志中记录TLS验证状态。
- 采集端点与频率
  - **新增**：厂商无关的路径发现机制，自动发现Systems和Chassis路径，替代硬编码的"/redfish/v1/Systems/1"。
  - Systems/1、Processors、Memory、Storage、Chassis/Thermal、Chassis/Power、UpdateService/FirmwareInventory。
  - 固件清单降频采集（约每小时），其余多为 60s 级别。
- 错误与退避
  - **新增**：全面的错误分类系统，提供中文诊断提示。
  - 连续失败 3 次后退避 5 分钟，降低 BMC 压力。
- 快照合并与上报
  - 内存维护最新快照列表，每次上报包含所有目标最新快照。

**更新** 新增了以下关键功能：

#### HTTP连接稳定性修复
```go
// redfishTransport creates an http.Transport configured for BMC compatibility.
// DisableKeepAlives is set because Dell iDRAC / HP iLO HTTP implementations
// send stale data on idle connections, causing Go's HTTP client to log
// "Unsolicited response received on idle HTTP channel". Each Redfish request
// is independent (30-60s apart), so connection reuse provides no benefit.
func redfishTransport(skipVerify bool) *http.Transport {
    return &http.Transport{
        TLSClientConfig:   redfishTLSConfig(skipVerify),
        DisableKeepAlives: true,
    }
}
```

#### 增强的密码解析机制
```go
// resolvePassword returns the effective password for this target.
// Priority: environment variable (password_env) > direct field (password).
// Logs diagnostics when the password appears empty.
func (t RedfishTarget) resolvePassword() string {
    pw := ""
    if t.PasswordEnv != "" {
        pw = os.Getenv(t.PasswordEnv)
        if pw == "" {
            slog.Warn("Redfish 密码环境变量为空",
                "target", t.Name, "env", t.PasswordEnv,
                "hint", "systemd 服务不继承用户环境变量，请在 .service 文件中设置 EnvironmentFile 或使用 password 字段")
        }
    }
    if pw == "" && t.Password != "" {
        pw = t.Password
    }
    if pw == "" {
        slog.Error("Redfish 密码为空，认证将失败",
            "target", t.Name,
            "password_env", t.PasswordEnv,
            "has_password_field", t.Password != "",
            "fix", "1) 设置环境变量并配置 EnvironmentFile，或 2) 在 config.json 中添加 password 字段")
    }
    return pw
}
```

#### 增强的TLS兼容性支持
```go
// redfishTLSConfig returns a tls.Config tuned for BMC/iDRAC/iLO compatibility.
// Old firmware (Dell iDRAC 7/8, HP iLO 3/4, Supermicro IPMI) often only supports
// TLS 1.0/1.1 and RSA key-exchange cipher suites that Go 1.22+ no longer offers
// by default. This config explicitly enables those legacy options so the handshake
// can succeed. BMC devices are internal-network only, so the reduced crypto
// requirements are acceptable.
func redfishTLSConfig(skipVerify bool) *tls.Config {
    // Start with all ID-based cipher suites (Go default set)
    cipherIDs := make([]uint16, 0, 32)
    for _, cs := range tls.CipherSuites() {
        cipherIDs = append(cipherIDs, cs.ID)
    }
    // Append insecure suites required by legacy BMC firmware:
    //   - RSA key exchange (TLS_RSA_WITH_AES_*_CBC_SHA)
    //   - 3DES suites
    for _, cs := range tls.InsecureCipherSuites() {
        cipherIDs = append(cipherIDs, cs.ID)
    }
    return &tls.Config{
        MinVersion:         tls.VersionTLS10, // allow TLS 1.0 for old iDRAC/iLO
        CipherSuites:       cipherIDs,
        InsecureSkipVerify: skipVerify,
    }
}
```

#### 厂商无关的路径发现机制
```go
// discoverSystemPath queries /redfish/v1/Systems and returns the first
// member's @odata.id. This handles vendor-specific system IDs:
//   - Dell iDRAC:   /redfish/v1/Systems/System.Embedded.1
//   - HP iLO:       /redfish/v1/Systems/1
//   - Supermicro:   /redfish/v1/Systems/1
//   - Lenovo XCC:   /redfish/v1/Systems/1
func (rc *redfishCollector) discoverSystemPath(client *http.Client, t RedfishTarget) (string, error) {
    password := t.resolvePassword()
    var col struct {
        Members []struct {
            ODataID string `json:"@odata.id"`
        } `json:"Members"`
    }
    if err := rc.rfGet(client, t.URL, t.Username, password, "/redfish/v1/Systems", &col); err != nil {
        return "", fmt.Errorf("discover Systems collection: %w", err)
    }
    if len(col.Members) == 0 {
        return "", fmt.Errorf("Systems collection is empty")
    }
    path := col.Members[0].ODataID
    slog.Info("Redfish System 路径已发现", "target", t.Name, "path", path)
    return path, nil
}
```

#### 全面的错误分类与中文诊断
```go
// classifyError returns a human-readable hint for common Redfish errors.
func classifyError(err error) string {
    msg := err.Error()
    switch {
    case containsAny(msg, "handshake failure", "tls: "):
        return "（TLS 握手失败：已启用 TLS 1.0+ 兼容模式，若仍失败请检查 BMC 固件版本是否过低，或尝试升级 iDRAC/iLO 固件）"
    case containsAny(msg, "x509", "certificate"):
        return "（TLS 证书错误：请在配置中设置 skip_tls_verify=true）"
    case containsAny(msg, "connection refused", "connect: "):
        return "（连接被拒绝：请检查 BMC 地址和端口是否正确，以及防火墙是否放行）"
    case containsAny(msg, "no such host", "lookup"):
        return "（DNS 解析失败：请检查 BMC 地址是否可达）"
    case containsAny(msg, "timeout", "deadline exceeded"):
        return "（连接超时：BMC 可能不可达或网络不通）"
    case containsAny(msg, "HTTP 401", "HTTP 403"):
        return "（认证失败：请检查 username 和 password_env 环境变量是否正确）"
    default:
        return ""
    }
}
```

#### v6.2.5 增强的硬件报告诊断功能
```go
// postHardwareReport sends a Redfish hardware snapshot to all server targets.
func (a *Agent) postHardwareReport(rep shared.HardwareReport) {
    body, err := json.Marshal(rep)
    if err != nil {
        slog.Warn("硬件上报序列化失败", "err", err)
        return
    }
    fp := a.identity.Fingerprint
    for _, t := range a.targets {
        go func(tgt *serverTarget) {
            req, err := http.NewRequest("POST", tgt.server+"/api/v1/agent/hardware", bytes.NewReader(body))
            if err != nil {
                return
            }
            req.Header.Set("Content-Type", "application/json")
            if fp != "" {
                req.Header.Set("X-Agent-Fingerprint", fp)
            }
            resp, err := tgt.httpc.Do(req)
            if err != nil {
                slog.Warn("硬件上报失败", "server", tgt.server, "err", err)
                return
            }
            defer resp.Body.Close()
            if resp.StatusCode >= 300 {
                // 读取响应体以便诊断拒绝原因（如 fingerprint mismatch）
                respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
                slog.Warn("硬件上报被拒", "server", tgt.server, "status", resp.StatusCode,
                    "host_id", rep.HostID, "snapshots", len(rep.Snapshots), "body", string(respBody))
            } else {
                slog.Info("硬件上报成功", "server", tgt.server, "host_id", rep.HostID,
                    "snapshots", len(rep.Snapshots))
            }
        }(t)
    }
}
```

```mermaid
classDiagram
class RedfishTarget {
+string Name
+string URL
+string Username
+string PasswordEnv
+string Password
+bool SkipTLSVerify
+int IntervalSec
+resolvePassword() string
}
class redfishCollector {
-targets []RedfishTarget
-hostID string
-fp string
-httpc *http.Client
-snapshots []HardwareSnapshot
-lastFW map[string]int64
-lastSEL map[string]int64
-selCache map[string][]HardwareEvent
-logPath map[string]string
-systemPath map[string]string
-chassisPath map[string]string
+run(reporter)
-pollLoop(target, reporter)
-collectOne(target) HardwareSnapshot
-storeAndReport(target, snap, reporter)
-rfGetRaw(client, base, user, pass, path, dst) error
-discoverSystemPath(client, target) string
-getChassisPath(client, target, sysPath) string
-classifyError(err) string
}
class HardwareSnapshot {
+string TargetName
+string TargetURL
+int64 Timestamp
+string Health
+string State
+[]RedfishCPU CPUs
+[]RedfishGPU GPUs
+RedfishMemory Memory
+[]RedfishStorage Storage
+[]RedfishRAID RAID
+[]SensorReading Temps
+[]FanReading Fans
+RedfishPower Power
+[]FirmwareInfo Firmware
+[]HardwareEvent Events
+string Error
}
class Agent {
+postHardwareReport(HardwareReport)
+postNetFlowReport(NetFlowReport)
}
redfishCollector --> RedfishTarget : "管理"
redfishCollector --> HardwareSnapshot : "生成/缓存"
Agent --> redfishCollector : "启动和管理"
```

**图表来源** 
- [collector_redfish.go:17-126](file://cmd/agent/collector_redfish.go#L17-L126)
- [wire.go:144-237](file://shared/wire.go#L144-L237)
- [reporter.go:609-644](file://cmd/agent/reporter.go#L609-L644)

**章节来源**
- [collector_redfish.go:56-126](file://cmd/agent/collector_redfish.go#L56-L126)
- [collector_redfish.go:129-391](file://cmd/agent/collector_redfish.go#L129-L391)
- [collector_redfish.go:393-429](file://cmd/agent/collector_redfish.go#L393-L429)
- [collector_redfish.go:162-259](file://cmd/agent/collector_redfish.go#L162-L259)
- [collector_redfish.go:261-293](file://cmd/agent/collector_redfish.go#L261-L293)
- [reporter.go:609-644](file://cmd/agent/reporter.go#L609-L644)
- [wire.go:144-237](file://shared/wire.go#L144-L237)

### Linux CPU采集器（CPU百分比计算修复）
**重要修复**：解决了CPU百分比计算中的关键bug，防止因内核iowait回退导致的uint64下溢和负值CPU使用率。

#### CPU计算逻辑修复
```go
// v5.4.0: Track permission errors across all collection points for diagnostics.
var permErrors []string

if ct, err := readCPUTimes(); err == nil {
    if c.primed && ct.total > c.prevCPU.total {
        totalDelta := ct.total - c.prevCPU.total
        // idle 里折算了 iowait，而内核文档明确 iowait **可以回退**（CPU 热插拔同理）。
        // 直接相减会 uint64 下溢成天文数字 → totalDelta-idleDelta 再次回绕 →
        // CPU% 变成 -9.2e17 这种脏数据，污染图表/告警/AI 基线。故双向夹紧。
        idleDelta := uint64(0)
        if ct.idle > c.prevCPU.idle {
            idleDelta = ct.idle - c.prevCPU.idle
        }
        if idleDelta > totalDelta {
            idleDelta = totalDelta
        }
        m.CPUPercent = round1(float64(totalDelta-idleDelta) / float64(totalDelta) * 100)
    }
    c.prevCPU = ct
}
```

#### 修复原理
- **问题根源**：内核文档明确指出iowait计数器可以回退（例如CPU热插拔时）
- **原始缺陷**：直接相减会导致uint64下溢，产生天文数字
- **修复方案**：双向夹紧idleDelta，确保其不超过totalDelta
- **影响范围**：防止CPU使用率出现负值（如-9.2e17），保护图表、告警和AI基线数据质量

**章节来源**
- [collector_linux.go:83-98](file://cmd/agent/collector_linux.go#L83-L98)
- [collector_linux.go:261-286](file://cmd/agent/collector_linux.go#L261-L286)

### 共享数据模型（Agent ↔ Server）
- HardwareSnapshot：单台服务器在某时间点的硬件快照，包含 CPU、内存、存储、传感器、风扇、电源、固件与健康状态。
- HardwareReport：Agent 上报的批量快照载体，附带主机标识与指纹。
- FlowRecord/NetFlowReport：用于 NetFlow 与五元组包采集的聚合记录与上报载体（与 Redfish 同属"硬件/网络"观测面）。

**章节来源**
- [wire.go:144-237](file://shared/wire.go#L144-L237)
- [wire.go:243-279](file://shared/wire.go#L243-L279)

### Server 端硬件处理与查询
- 接收与校验
  - POST /api/v1/agent/hardware：校验 JSON、HostID、X-Agent-Fingerprint 指纹匹配。
- 持久化与指标写入
  - 将快照 upsert 到 PostgreSQL；对非 OK 的健康状态插入硬件事件。
  - 将健康分数、温度、风扇 RPM、功耗等指标推送到 VictoriaMetrics。
- 查询接口
  - GET /api/v1/hardware/health：返回某主机最新快照。
  - GET /api/v1/hardware/history：基于 PromQL 查询历史趋势（温度/功率/风扇/健康分）。

```mermaid
sequenceDiagram
participant AG as "Agent"
participant REP as "硬件报告上报器"
participant SV as "Server 路由"
participant HW as "硬件处理器"
participant PG as "PostgreSQL"
participant VM as "VictoriaMetrics"
AG->>REP : "组装 HardwareReport"
REP->>SV : "POST /api/v1/agent/hardware"
Note over REP,SV : "v6.2.5 : 捕获响应体用于诊断"
SV->>HW : "handleAgentHardware()"
HW->>PG : "upsert 快照 + 事件"
HW->>VM : "push 健康/温度/风扇/功耗"
SV-->>REP : "200 OK"
REP->>REP : "记录INFO级别成功日志"
AG->>SV : "GET /api/v1/hardware/history?host=&metric=&range="
SV->>VM : "PromQL 查询"
VM-->>SV : "时序点集"
SV-->>AG : "points[]"
```

**图表来源** 
- [reporter.go:609-644](file://cmd/agent/reporter.go#L609-L644)
- [handlers.go:290-298](file://cmd/server/handlers.go#L290-L298)
- [hardware_netflow.go:19-158](file://cmd/server/hardware_netflow.go#L19-L158)

**章节来源**
- [handlers.go:290-298](file://cmd/server/handlers.go#L290-L298)
- [hardware_netflow.go:19-158](file://cmd/server/hardware_netflow.go#L19-L158)

### 三类采集器协同（Redfish / NetFlow / 五元组包）
- NetFlow 接收器
  - UDP 监听 v5/v9，模板缓存，窗口聚合，周期性 flush 上报。
- 五元组包采集
  - Linux 下读取 nf_conntrack，增量 diff 生成流记录，限速输出。
- 统一上报
  - 均通过 /api/v1/agent/netflow 上报，Server 侧写入 VM 与可选 PG。

```mermaid
flowchart TD
Start(["开始"]) --> Mode{"采集类型?"}
Mode --> |Redfish| RF["轮询 Redfish 端点<br/>生成 HardwareSnapshot"]
Mode --> |NetFlow| NF["UDP 接收 v5/v9<br/>模板缓存+聚合"]
Mode --> |Packet| PC["读取 nf_conntrack<br/>增量 diff 生成流"]
RF --> Report["组装 HardwareReport"]
NF --> NFR["组装 NetFlowReport"]
PC --> NFR
Report --> PostH["POST /api/v1/agent/hardware<br/>v6.2.5: 增强诊断日志"]
NFR --> PostN["POST /api/v1/agent/netflow"]
PostH --> End(["完成"])
PostN --> End
```

**图表来源** 
- [collector_redfish.go:56-126](file://cmd/agent/collector_redfish.go#L56-L126)
- [reporter.go:609-644](file://cmd/agent/reporter.go#L609-L644)
- [collector_netflow.go:192-263](file://cmd/agent/collector_netflow.go#L192-L263)
- [collector_packet.go:58-113](file://cmd/agent/collector_packet.go#L58-L113)
- [handlers.go:290-298](file://cmd/server/handlers.go#L290-L298)

**章节来源**
- [collector_netflow.go:192-263](file://cmd/agent/collector_netflow.go#L192-L263)
- [collector_packet.go:58-113](file://cmd/agent/collector_packet.go#L58-L113)
- [handlers.go:290-298](file://cmd/server/handlers.go#L290-L298)

## 依赖关系分析
- Agent 侧
  - collector_redfish.go 依赖 shared/wire.go 的数据模型。
  - main.go 将 redfishTargets 注入 Agent，驱动采集器运行。
  - reporter.go 提供增强的硬件报告上报功能，包含详细的诊断日志。
  - collector_linux.go 提供Linux平台的基础系统指标采集。
- Server 侧
  - handlers.go 注册 /api/v1/agent/hardware 与 /api/v1/agent/netflow 路由。
  - hardware_netflow.go 实现具体处理逻辑，依赖 pgStore 与 vmWriter。
  - pgstore.go 提供硬件相关的数据库操作，包括快照存储、事件记录和查询。
- **前端展示层**
  - hardware.js 依赖 Web API 接口获取硬件数据。
  - style.css 提供硬件面板的专业样式设计。
  - i18n-dashboard.js 提供多语言支持。

```mermaid
graph LR
CR["collector_redfish.go"] --> SW["shared/wire.go"]
AM["cmd/agent/main.go"] --> CR
REP["cmd/agent/reporter.go"] --> SW
REP --> |"增强诊断日志"| SV["cmd/server/handlers.go"]
HG["cmd/server/handlers.go"] --> HN["cmd/server/hardware_netflow.go"]
HN --> SW
HN --> PS["cmd/server/pgstore.go"]
LNX["collector_linux.go"] --> SW
HWJS["hardware.js"] --> HG
STYLE["style.css"] --> HWJS
I18N["i18n-dashboard.js"] --> HWJS
```

**图表来源** 
- [collector_redfish.go:1-16](file://cmd/agent/collector_redfish.go#L1-L16)
- [wire.go:1-10](file://shared/wire.go#L1-L10)
- [main.go:223-233](file://cmd/agent/main.go#L223-L233)
- [reporter.go:609-644](file://cmd/agent/reporter.go#L609-L644)
- [handlers.go:290-298](file://cmd/server/handlers.go#L290-L298)
- [hardware_netflow.go:1-13](file://cmd/server/hardware_netflow.go#L1-L13)
- [pgstore.go:1277-1370](file://cmd/server/pgstore.go#L1277-L1370)
- [hardware.js:1-230](file://cmd/server/web/js/hardware.js#L1-L230)
- [style.css:2808-2839](file://cmd/server/web/style.css#L2808-L2839)
- [i18n-dashboard.js:450-472](file://cmd/server/web/i18n-dashboard.js#L450-472)
- [collector_linux.go:1-20](file://cmd/agent/collector_linux.go#L1-L20)

**章节来源**
- [collector_redfish.go:1-16](file://cmd/agent/collector_redfish.go#L1-L16)
- [wire.go:1-10](file://shared/wire.go#L1-L10)
- [main.go:223-233](file://cmd/agent/main.go#L223-L233)
- [reporter.go:609-644](file://cmd/agent/reporter.go#L609-L644)
- [handlers.go:290-298](file://cmd/server/handlers.go#L290-L298)
- [hardware_netflow.go:1-13](file://cmd/server/hardware_netflow.go#L1-L13)
- [pgstore.go:1277-1370](file://cmd/server/pgstore.go#L1277-L1370)

## 性能与容量规划
- Agent 侧
  - 每目标独立 goroutine 与定时器，避免相互阻塞；连续失败退避降低 BMC 压力。
  - 固件清单降频采集，减少冗余请求。
  - **新增**：路径发现结果缓存，避免重复查询。
  - **新增**：HTTP连接禁用KeepAlive，防止Dell iDRAC系统的陈旧数据问题。
  - **v6.2.5增强**：硬件报告上报采用并发处理，每个目标独立goroutine，避免相互阻塞。
  - **CPU计算优化**：修复后的CPU百分比计算避免了无意义的重算和数据污染。
  - **事件日志优化**：300秒轮询间隔和40条限制，平衡了数据采集频率和BMC负载。
- Server 侧
  - 指纹校验前置，拒绝非法上报。
  - 数值指标走 VM，明细可落 PG，兼顾查询性能与成本。
  - **新增**：硬件快照UPSERT机制，避免覆盖上次有效数据。
  - **新增**：事件去重机制，仅在状态变化时记录事件。
- **前端展示层**
  - 客户端缓存机制，减少重复API请求。
  - 懒加载技术，仅在用户交互时加载详细信息。
  - 虚拟滚动支持，优化大量硬件数据的渲染性能。
- 容量建议
  - 根据目标数量与采集间隔估算 BMC 并发与带宽。
  - 结合 VM 标签维度（host/target/sensor/fan_name）评估查询负载。
  - 考虑前端并发请求限制，合理设置刷新频率。
  - **新增**：事件日志40条限制，避免单次快照过大影响传输效率。

## 故障排查指南
- 无法连接 BMC
  - 检查 URL、用户名/密码环境变量、TLS 策略（是否 skip_verify）、网络连通性。
  - **新增**：查看TLS兼容性日志，确认是否启用了legacy固件支持。
  - **新增**：检查HTTP连接日志，确认DisableKeepAlives配置是否生效。
- 频繁失败与退避
  - 关注连续失败计数与 5 分钟退避日志；确认 BMC 服务可用性与限流策略。
- 指纹校验失败
  - 核对 X-Agent-Fingerprint 或 fp 参数是否与主机注册信息一致。
  - **v6.2.5增强**：查看硬件上报被拒的详细响应体内容（最多512字节），获取具体的错误信息。
- 无历史数据
  - 确认 VM 已启用且可访问；检查 metric 名称与标签是否正确。
- 端口冲突（NetFlow）
  - 默认监听 :2055，若被占用需调整 listen 地址。
- **新增**：路径发现失败
  - 检查BMC是否支持标准Redfish API，确认/redfish/v1/Systems端点可访问。
- **新增**：TLS握手失败
  - 查看详细的中文错误提示，确认是否需要升级BMC固件或调整TLS配置。
- **新增**：密码认证失败
  - 检查password_env环境变量是否正确设置，或确认password字段配置。
  - systemd服务环境下建议使用EnvironmentFile或直接password字段。
- **v6.2.5新增**：硬件报告诊断
  - 查看INFO级别的"硬件上报成功"日志，确认上报是否成功。
  - 对于失败的报告，检查WARN级别的详细错误信息，包括响应体内容。
  - 重点关注"fingerprint mismatch"错误，确认Agent与服务端的指纹绑定状态。
- **CPU计算相关故障**
  - **新增**：如果观察到CPU使用率为负值或异常大的数值，可能是内核iowait回退导致的计算问题，现已通过双向夹紧机制修复。
  - 检查Linux内核版本和CPU热插拔配置，确认是否存在频繁的CPU状态变化。
- **前端相关故障**
  - 硬件面板无法加载：检查浏览器控制台是否有JavaScript错误。
  - 数据不更新：确认网络连接正常，检查API响应状态码。
  - 界面显示异常：清除浏览器缓存，检查CSS加载是否正常。
  - 多语言切换无效：确认i18n资源文件正确加载。
- **新增**：设备身份识别问题
  - 检查BMC固件版本是否支持完整的Redfish属性，确认Manufacturer、Model、SerialNumber等字段是否正确填充。
  - 对于华为设备，确认序列号是否在Chassis而非System中。
- **新增**：GPU识别问题
  - 检查ProcessorType字段是否正确识别为"GPU"或"Accelerator"。
  - 确认GPU信息没有被错误地归类到CPU列表中。
- **新增**：内存DIMM采集问题
  - 检查DeviceLocator字段是否正确映射到物理槽位。
  - 确认空槽位（Absent状态）已被正确过滤。
- **新增**：存储子系统问题
  - 检查RAID控制器信息是否正确采集，确认StorageControllers字段存在。
  - 验证SMART预测故障字段（FailurePredicted）是否正确解析。
  - 确认SSD剩余寿命字段（PredictedMediaLifeLeftPercent）不为null。
- **新增**：电源监控问题
  - 检查PowerSupplies字段是否正确解析（注意属性名大小写）。
  - 确认冗余状态（Redundancy）字段是否正确提取。
  - 验证PSU输入输出功率数据是否完整。
- **新增**：BMC事件日志问题
  - 检查事件日志路径发现逻辑，确认SEL/LogServices端点可访问。
  - 验证事件组件归因解析是否正确（MessageArgs、Links.OriginOfCondition等）。
  - 确认事件日志采集间隔（300秒）和限制（40条）配置合理。

**章节来源**
- [collector_redfish.go:62-101](file://cmd/agent/collector_redfish.go#L62-L101)
- [collector_redfish.go:261-293](file://cmd/agent/collector_redfish.go#L261-L293)
- [hardware_netflow.go:19-90](file://cmd/server/hardware_netflow.go#L19-L90)
- [collector_netflow.go:203-216](file://cmd/agent/collector_netflow.go#L203-216)
- [reporter.go:609-644](file://cmd/agent/reporter.go#L609-L644)
- [hardware.js:1-230](file://cmd/server/web/js/hardware.js#L1-L230)
- [collector_linux.go:83-98](file://cmd/agent/collector_linux.go#L83-L98)
- [collector_redfish.go:752-1022](file://cmd/agent/collector_redfish.go#L752-L1022)

## 结论
Redfish 硬件采集器以"轻量 Agent + 统一模型 + 双后端（PG+VM）+ 现代化前端"的方式，实现了跨厂商 BMC 的标准化硬件观测。**v6.5.0重大更新**：新增了完整的设备身份发现、GPU/加速器卡分离识别、内存DIMM详细信息采集、存储子系统增强、电源供应器监控、BMC事件日志系统等核心功能改进，为运维人员提供了全方位的硬件观测能力。配合 NetFlow 与五元组包采集，形成"硬件/网络"一体化监控面。设计上强调稳定性（退避/超时/鉴权）、可扩展（多目标/多协议）与可观测（指标/事件/历史/可视化）。通过HTTP连接稳定性修复、增强的密码解析机制、TLS兼容性支持和全面的错误分类系统，显著提升了系统在复杂生产环境中的稳定性和用户体验。v6.2.5版本进一步增强硬件报告诊断功能，提供更详细的错误上下文信息和成功操作日志，大幅提升了问题排查效率。

## 附录：API 定义
- 接收端点
  - POST /api/v1/agent/hardware
    - 入参：HardwareReport（含 host_id、fingerprint、snapshots[]）
    - 鉴权：X-Agent-Fingerprint 或 fp 查询参数
    - 响应：{status:"ok"}
    - **v6.2.5增强**：失败时返回详细的错误响应体（最多512字节），便于诊断指纹不匹配和认证问题
  - POST /api/v1/agent/netflow
    - 入参：NetFlowReport（含 host_id、source、flows[]、stats）
    - 鉴权：同上
    - 响应：{status:"ok"}
- 查询端点
  - GET /api/v1/hardware/health?host=...
    - 返回：最新快照列表
    - **前端集成**：供硬件面板实时显示设备状态
  - GET /api/v1/hardware/history?host=&metric=&range=[target]
    - 返回：时序点集 points[]
  - GET /api/v1/hardware/events?host=&limit=&target=...
    - 返回：平台侧记录的硬件状态变化事件
  - GET /api/v1/netflow/summary?host=&range=&dimension=&top=...
    - 返回：Top-N 聚合结果
  - GET /api/v1/netflow/flows?host=&limit=&filter=...
    - 返回：Flow 明细
  - GET /api/v1/netflow/packets?host=&range=...
    - 返回：包统计时序点集

**章节来源**
- [handlers.go:290-298](file://cmd/server/handlers.go#L290-L298)
- [hardware_netflow.go:95-277](file://cmd/server/hardware_netflow.go#L95-L277)
- [reporter.go:609-644](file://cmd/agent/reporter.go#L609-L644)
- [hardware.js:23-43](file://cmd/server/web/js/hardware.js#L23-L43)
- [hardware_netflow.go:137-158](file://cmd/server/hardware_netflow.go#L137-L158)