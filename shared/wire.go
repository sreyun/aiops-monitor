// Package shared holds the wire types exchanged between the Go agent core
// and the Go backend. Keeping them in one place is exactly the "share code
// with the backend" benefit of the hybrid architecture: the collector, the
// reporter and the server all speak the same structs, so the contract can
// never drift.
package shared

// Metrics is one point-in-time snapshot of base system metrics.
// On Linux/Windows/macOS the agent core fills this natively (procfs+syscall /
// Win32 API / sysctl). On any other platform it can be supplied by a core
// plugin (e.g. psutil) behind the same Collector interface.
type Metrics struct {
	CPUPercent   float64    `json:"cpu_percent"`
	CPUCores     int        `json:"cpu_cores"`
	MemTotal     uint64     `json:"mem_total"`
	MemUsed      uint64     `json:"mem_used"`
	MemPercent   float64    `json:"mem_percent"`
	SwapTotal    uint64     `json:"swap_total"`
	SwapUsed     uint64     `json:"swap_used"`
	SwapPercent  float64    `json:"swap_percent"`
	DiskTotal    uint64     `json:"disk_total"`
	DiskUsed     uint64     `json:"disk_used"`
	DiskPercent  float64    `json:"disk_percent"`
	Disks        []DiskInfo `json:"disks,omitempty"` // per-volume usage for every local disk
	NetSentRate  float64    `json:"net_sent_rate"`
	NetRecvRate  float64    `json:"net_recv_rate"`
	NetConns     int        `json:"net_conns"`       // established TCP connections (兼容旧字段)
	Conns        []ConnStat `json:"conns,omitempty"` // per-proto/per-state socket 计数（TCP 各状态 + UDP 总数）
	Load1        float64    `json:"load1"`
	Load5        float64    `json:"load5"`
	Load15       float64    `json:"load15"`
	ProcCount    int        `json:"proc_count"`
	Uptime       uint64     `json:"uptime"`
	GPUs         []GPUInfo  `json:"gpus,omitempty"`          // per-GPU utilization / VRAM (best-effort, cross-platform)
	ProcessNames []string   `json:"process_names,omitempty"` // top process names for process-monitor checks
	// Disk IO: read/write rates (bytes/sec) and IO utilization percentage
	DiskReadRate      float64 `json:"disk_read_rate"`
	DiskWriteRate     float64 `json:"disk_write_rate"`
	DiskIOUtilPercent float64 `json:"disk_io_util_percent"`
	// Disk IOPS: read/write operations per second
	DiskReadIOPS  float64 `json:"disk_read_iops"`
	DiskWriteIOPS float64 `json:"disk_write_iops"`

	// ---- API 业务监控指标（由插件或外部系统上报，可选）----
	APIAvailPercent  float64 `json:"api_avail_percent,omitempty"`  // 接口可用率 %
	APIAvgRespMs     float64 `json:"api_avg_resp_ms,omitempty"`    // 平均响应时间 ms
	APIP95RespMs     float64 `json:"api_p95_resp_ms,omitempty"`    // P95 响应时间 ms
	APIThroughputRPS float64 `json:"api_throughput_rps,omitempty"` // 吞吐量 req/s

	// ---- 编排定时任务指标（由插件或外部系统上报，可选）----
	TaskFailCount  int     `json:"task_fail_count,omitempty"`  // 执行失败次数
	TaskTimeoutSec float64 `json:"task_timeout_sec,omitempty"` // 超时时长 s
}

// GPUInfo is per-GPU usage. Collection is best-effort and vendor-dependent:
// NVIDIA via nvidia-smi (Linux/Windows), AMD via sysfs (Linux), Apple/other via
// ioreg (macOS). Fields that a platform cannot supply are left zero.
type GPUInfo struct {
	Name        string  `json:"name"`
	UtilPercent float64 `json:"util_percent"`
	MemUsed     uint64  `json:"mem_used,omitempty"`
	MemFree     uint64  `json:"mem_free,omitempty"` // VRAM 空闲字节（total-used 或直接采集）
	MemTotal    uint64  `json:"mem_total,omitempty"`
	MemPercent  float64 `json:"mem_percent,omitempty"`
	Temp        float64 `json:"temp,omitempty"` // °C, 0 if unknown
}

// ConnStat is a per-protocol, per-state socket count, powering the connection /
// session-state trend charts. For TCP, State is a canonical state name
// (ESTABLISHED/TIME_WAIT/LISTEN/CLOSE_WAIT/SYN_SENT/...); for UDP (which is
// stateless) a single entry with State="" carries the total socket count.
type ConnStat struct {
	Proto string `json:"proto"`           // "tcp" | "udp"
	State string `json:"state,omitempty"` // TCP 状态名；UDP 为空
	Count int    `json:"count"`
}

// DiskInfo is per-volume disk usage. The agent enumerates every local disk:
// all fixed drives on Windows (C:, D:, …), real filesystem mounts on
// Linux/macOS. Path is the drive letter or mount point.
type DiskInfo struct {
	Path    string  `json:"path"`
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Percent float64 `json:"percent"`
}

// Sample is a Metrics snapshot stamped with the server receive time.
type Sample struct {
	Timestamp int64 `json:"timestamp"`
	Metrics
}

// LogLine is one collected log line from an agent's log sources.
type LogLine struct {
	Ts      int64  `json:"ts"`
	Source  string `json:"source"` // file path / "journald" / "docker:<name>"
	Level   string `json:"level"`  // error|warn|info|debug
	Message string `json:"message"`
}

// LogBatch is a batch of collected log lines POSTed by an agent. The agent
// authenticates via the X-Agent-Fingerprint header (like the terminal + forward
// channels), so no credential travels in the body.
type LogBatch struct {
	HostID string    `json:"host_id"`
	Lines  []LogLine `json:"lines"`
}

// Event is a discrete signal emitted by a plugin — this is the channel the
// Python plugin / AI / automation layer uses to raise findings (anomalies,
// service-down, predictions, remediation results...).
type Event struct {
	Timestamp int64  `json:"timestamp"`
	Level     string `json:"level"`  // info | warning | critical
	Source    string `json:"source"` // plugin name
	Message   string `json:"message"`
}

// Report is the payload the agent core POSTs each cycle. Base metrics come
// from the Go core; Custom gauges and Events come from the plugin layer.
// Category is an operator-defined group label (e.g. prod / db / office-endpoint)
// used by the dashboard to group and filter hosts.
type Report struct {
	HostID      string             `json:"host_id"`
	Hostname    string             `json:"hostname"`
	OS          string             `json:"os"`
	Platform    string             `json:"platform"` // OS / distribution version
	Arch        string             `json:"arch"`
	IP          string             `json:"ip,omitempty"`
	Kernel      string             `json:"kernel,omitempty"`
	Category    string             `json:"category,omitempty"`
	Token       string             `json:"token,omitempty"`       // install token (registration only)
	Fingerprint string             `json:"fingerprint,omitempty"` // machine fingerprint (machine-id+MAC), authenticates reports
	Metrics     Metrics            `json:"metrics"`
	Custom      map[string]float64 `json:"custom,omitempty"`
	Events      []Event            `json:"events,omitempty"`
}

// ============================================================================
// Redfish 硬件状态采集结构体
// ============================================================================

// HardwareSnapshot is one point-in-time snapshot of a Redfish-managed server.
type HardwareSnapshot struct {
	TargetName string           `json:"target_name"`
	TargetURL  string           `json:"target_url"`
	Timestamp  int64            `json:"timestamp"`
	Health     string           `json:"health"` // OK / Warning / Critical
	State      string           `json:"state"`  // Enabled / Disabled / ...
	System     RedfishSystem    `json:"system"` // 整机身份（厂商/型号/序列号/BIOS…）
	CPUs       []RedfishCPU     `json:"cpus"`
	GPUs       []RedfishGPU     `json:"gpus,omitempty"` // Processors 里 ProcessorType=GPU 的成员
	Memory     RedfishMemory    `json:"memory"`
	Storage    []RedfishStorage `json:"storage"`
	RAID       []RedfishRAID    `json:"raid,omitempty"` // Storage 成员里的 StorageControllers（RAID 卡）
	Enclosures []StorageEnclosure `json:"enclosures,omitempty"` // 磁盘框（OceanStor 等外置存储）
	Temps      []SensorReading  `json:"temps"`
	Fans       []FanReading     `json:"fans"`
	Power      RedfishPower     `json:"power"`
	Firmware   []FirmwareInfo   `json:"firmware,omitempty"` // 降频采集
	Events     []HardwareEvent  `json:"events,omitempty"`   // BMC SEL / 事件日志（最近若干条）
	Error      string           `json:"error,omitempty"`
}

// RedfishSystem is the chassis/system identity — who this machine actually is.
// Without it the UI can only show a BMC URL, so an operator staring at a fault
// can't tell an R730 from a TaiShan 200 without opening iDRAC/iBMC by hand.
type RedfishSystem struct {
	Manufacturer string `json:"manufacturer,omitempty"` // Dell Inc. / Huawei
	Model        string `json:"model,omitempty"`        // PowerEdge R740 / RH2288 V3 / TaiShan 200 (Model 2280)
	SKU          string `json:"sku,omitempty"`          // Dell 的 Service Tag 就在这里
	SerialNumber string `json:"serial_number,omitempty"`
	AssetTag     string `json:"asset_tag,omitempty"`
	HostName     string `json:"host_name,omitempty"` // BMC 视角的 OS 主机名
	BIOSVersion  string `json:"bios_version,omitempty"`
	PowerState   string `json:"power_state,omitempty"` // On / Off
	IndicatorLED string `json:"indicator_led,omitempty"`
	BMCModel     string `json:"bmc_model,omitempty"`    // iDRAC9 / iBMC
	BMCFirmware  string `json:"bmc_firmware,omitempty"` // Manager.FirmwareVersion
}

// HardwareEvent is one BMC log entry (Dell iDRAC SEL/LC log, Huawei iBMC event
// log). This is the ONLY source that says *which* component caused a fault and
// when — a Health=Critical rollup on its own tells an operator nothing.
type HardwareEvent struct {
	ID        string `json:"id,omitempty"`
	Created   string `json:"created,omitempty"`  // RFC3339 from the BMC
	Severity  string `json:"severity,omitempty"` // OK / Warning / Critical
	Message   string `json:"message"`
	MessageID string `json:"message_id,omitempty"` // e.g. "AMP0300" / "Alert.1.0.PowerSupply"
	// Component is the offending part, resolved from Links.OriginOfCondition or
	// the sensor/entry metadata — "PSU 2", "DIMM A3", "Disk 0:1:3", …
	Component  string `json:"component,omitempty"`
	SensorType string `json:"sensor_type,omitempty"`
	Resolved   bool   `json:"resolved,omitempty"`
}

type RedfishCPU struct {
	Name       string  `json:"name"`
	Model      string  `json:"model"`
	Cores      int     `json:"cores"`
	Threads    int     `json:"threads"`
	Health     string  `json:"health"`
	TempC      float64 `json:"temp_c,omitempty"`
	MaxFreqMHz int     `json:"max_freq_mhz,omitempty"`
}

// RedfishGPU is a GPU reported by the BMC (a Processors member whose
// ProcessorType is "GPU"). Distinct from Metrics.GPUs, which is the OS-side
// nvidia-smi view — this one works even when the host OS is down.
type RedfishGPU struct {
	Name         string `json:"name"`
	Model        string `json:"model,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Health       string `json:"health"`
	State        string `json:"state,omitempty"`
	MaxFreqMHz   int    `json:"max_freq_mhz,omitempty"`
}

// RedfishRAID is a storage/RAID controller (Storage member's StorageControllers).
type RedfishRAID struct {
	Name            string          `json:"name"`
	Model           string          `json:"model,omitempty"`
	Manufacturer    string          `json:"manufacturer,omitempty"`
	FirmwareVersion string          `json:"firmware_version,omitempty"`
	SpeedGbps       float64         `json:"speed_gbps,omitempty"`
	Health          string          `json:"health"`
	State           string          `json:"state,omitempty"`
	DriveCount      int             `json:"drive_count,omitempty"`
	SerialNumber    string          `json:"serial_number,omitempty"`
	CacheMB         float64         `json:"cache_mb,omitempty"`     // CacheSummary.TotalCacheSizeMiB
	CacheHealth     string          `json:"cache_health,omitempty"` // 掉电保护/BBU 状态
	Volumes         []RedfishVolume `json:"volumes,omitempty"`
}

// RedfishVolume is a logical RAID volume (Storage member's Volumes).
type RedfishVolume struct {
	Name       string  `json:"name"`
	RAIDType   string  `json:"raid_type,omitempty"` // RAID0 / RAID1 / RAID5…
	CapacityGB float64 `json:"capacity_gb,omitempty"`
	Health     string  `json:"health,omitempty"`
	State      string  `json:"state,omitempty"`
}

// StorageEnclosure is a disk enclosure (磁盘框). Populated by the OceanStor
// DeviceManager collector — OceanStor arrays expose no Redfish endpoint at all,
// so this never comes from a BMC.
type StorageEnclosure struct {
	Name         string  `json:"name"`
	Model        string  `json:"model,omitempty"`
	SerialNumber string  `json:"serial_number,omitempty"`
	Location     string  `json:"location,omitempty"`
	Type         string  `json:"type,omitempty"`
	Health       string  `json:"health"`
	State        string  `json:"state,omitempty"`
	TemperatureC float64 `json:"temperature_c,omitempty"`
}

type RedfishMemory struct {
	TotalGB float64      `json:"total_gb"`
	UsedGB  float64      `json:"used_gb,omitempty"`
	DIMMs   []MemoryDIMM `json:"dimms,omitempty"`
}

type MemoryDIMM struct {
	Name         string  `json:"name"`
	CapacityGB   float64 `json:"capacity_gb"`
	Type         string  `json:"type"` // DDR4 / DDR5
	SpeedMHz     int     `json:"speed_mhz"`
	Health       string  `json:"health"`
	Slot         string  `json:"slot,omitempty"`
	Manufacturer string  `json:"manufacturer,omitempty"`
	PartNumber   string  `json:"part_number,omitempty"`
	SerialNumber string  `json:"serial_number,omitempty"`
	RankCount    int     `json:"rank_count,omitempty"`
	State        string  `json:"state,omitempty"` // Enabled / Absent
}

type RedfishStorage struct {
	Name       string  `json:"name"`
	Model      string  `json:"model,omitempty"`
	CapacityGB float64 `json:"capacity_gb"`
	Health     string  `json:"health"`
	MediaType  string  `json:"media_type,omitempty"` // HDD / SSD / NVMe
	Protocol   string  `json:"protocol,omitempty"`   // SATA / SAS / NVMe
	Status     string  `json:"status,omitempty"`     // OK / Warning / Critical
	SMARTWarn  bool    `json:"smart_warn,omitempty"`
	// 定位与寿命：换盘要知道插在哪个槽位，SSD 还要知道还剩多少寿命。
	SerialNumber string  `json:"serial_number,omitempty"`
	Revision     string  `json:"revision,omitempty"` // 盘固件版本
	Location     string  `json:"location,omitempty"` // 槽位，如 "Bay 3" / "Disk 0:1:3"
	Manufacturer string  `json:"manufacturer,omitempty"`
	RotationRPM  int     `json:"rotation_rpm,omitempty"`
	LifeLeftPct  float64 `json:"life_left_pct,omitempty"` // SSD 剩余寿命；-1 = 未知
	SpeedGbps    float64 `json:"speed_gbps,omitempty"`
	HotspareType string  `json:"hotspare_type,omitempty"`
	State        string  `json:"state,omitempty"`
}

type SensorReading struct {
	Name          string  `json:"name"`
	Reading       float64 `json:"reading"`
	Unit          string  `json:"unit"`   // Celsius, etc.
	Status        string  `json:"status"` // OK / Warning / Critical
	UpperCaution  float64 `json:"upper_caution,omitempty"`
	UpperCritical float64 `json:"upper_critical,omitempty"`
}

type FanReading struct {
	Name   string `json:"name"`
	RPM    int    `json:"rpm"`
	Health string `json:"health"`
	Status string `json:"status,omitempty"`
}

type RedfishPower struct {
	Redundancy string       `json:"redundancy"` // Full / N+1 / NotRedundant
	PSUs       []PSUReading `json:"psus,omitempty"`
	TotalWatts float64      `json:"total_watts,omitempty"`
}

type PSUReading struct {
	Name             string  `json:"name"`
	InputWatts       float64 `json:"input_watts"`
	OutputWatts      float64 `json:"output_watts,omitempty"`
	Health           string  `json:"health"`
	State            string  `json:"state"`
	Model            string  `json:"model,omitempty"`
	Manufacturer     string  `json:"manufacturer,omitempty"`
	SerialNumber     string  `json:"serial_number,omitempty"`
	FirmwareVersion  string  `json:"firmware_version,omitempty"`
	CapacityWatts    float64 `json:"capacity_watts,omitempty"` // 额定功率
	LineInputVoltage float64 `json:"line_input_voltage,omitempty"`
	PowerSupplyType  string  `json:"psu_type,omitempty"` // AC / DC
}

type FirmwareInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// HardwareReport is the payload agents POST for Redfish hardware snapshots.
type HardwareReport struct {
	HostID      string             `json:"host_id"`
	Fingerprint string             `json:"fingerprint,omitempty"`
	Snapshots   []HardwareSnapshot `json:"snapshots"`
}

// ============================================================================
// Hyper-V 虚拟机采集结构体（宿主机上的 Guest VM 清单）
//
// 与硬件快照同属"一台主机一份、变化慢、要追踪变更"的清单类数据，因此走独立
// 上报通道（POST /api/v1/agent/hyperv）而非高频 metrics 热路径。数据由物理
// 宿主机上的 Windows agent 通过 PowerShell(Get-VM) 采集。
// ============================================================================

// HyperVGuest is one Hyper-V guest VM as seen from the physical host.
// Health 由 agent 侧从 State/Status/ReplicationHealth 归一而来（OK/Warning/
// Critical），让服务端告警评估无需重复解析厂商字符串。
type HyperVGuest struct {
	Name             string   `json:"name"`
	ID               string   `json:"id"`                  // VM GUID：稳定身份，用于变更追踪（改名也认得出是同一台）
	State            string   `json:"state"`               // Running / Off / Paused / Saved / Starting / ...
	Status           string   `json:"status,omitempty"`    // "Operating normally" / 降级/故障描述
	Health           string   `json:"health,omitempty"`    // OK / Warning / Critical（归一后）
	CPUUsage         float64  `json:"cpu_usage"`           // 宿主视角 CPU 占用 %
	ProcessorCount   int      `json:"processor_count,omitempty"`
	MemAssignedMB    float64  `json:"mem_assigned_mb,omitempty"`
	MemDemandMB      float64  `json:"mem_demand_mb,omitempty"`
	MemMaxMB         float64  `json:"mem_max_mb,omitempty"`
	DynamicMemEnabled bool    `json:"dynamic_mem_enabled,omitempty"` // 内存压力(需求/分配)只对动态内存 VM 有意义
	UptimeSec        int64    `json:"uptime_sec,omitempty"`
	Generation       int      `json:"generation,omitempty"`
	Version          string   `json:"version,omitempty"`
	IPAddresses      []string `json:"ip_addresses,omitempty"` // 由集成服务上报，Guest 运行时才有
	Switches         []string `json:"switches,omitempty"`     // 连接的虚拟交换机名
	VHDCount         int      `json:"vhd_count,omitempty"`
	CheckpointCount  int      `json:"checkpoint_count,omitempty"`
	ReplState        string   `json:"repl_state,omitempty"`  // Disabled / Enabled / ...
	ReplHealth       string   `json:"repl_health,omitempty"` // NotApplicable / Normal / Warning / Critical
}

// HyperVReport is the payload agents POST for the Hyper-V guest inventory of one
// physical host. Error carries a collection failure (e.g. Get-VM unavailable) so
// the server can surface it without overwriting the last good inventory.
type HyperVReport struct {
	HostID      string        `json:"host_id"`
	Fingerprint string        `json:"fingerprint,omitempty"`
	Timestamp   int64         `json:"timestamp"`
	HostName    string        `json:"host_name,omitempty"`
	Error       string        `json:"error,omitempty"`
	Guests      []HyperVGuest `json:"guests"`
}

// ============================================================================
// NetFlow / 五元组包采集结构体
// ============================================================================

// FlowRecord is one aggregated flow (five-tuple + bytes/packets).
type FlowRecord struct {
	SrcIP     string `json:"src_ip"`
	DstIP     string `json:"dst_ip"`
	SrcPort   uint16 `json:"src_port"`
	DstPort   uint16 `json:"dst_port"`
	Protocol  uint8  `json:"protocol"`
	Bytes     uint64 `json:"bytes"`
	Packets   uint64 `json:"packets"`
	FirstSeen int64  `json:"first_seen"`
	LastSeen  int64  `json:"last_seen"`
	TCPFlags  uint8  `json:"tcp_flags"`
	SrcAS     uint32 `json:"src_as,omitempty"`
	DstAS     uint32 `json:"dst_as,omitempty"`
	InputIf   uint32 `json:"input_if,omitempty"`
	OutputIf  uint32 `json:"output_if,omitempty"`
}

type NetFlowStats struct {
	TotalFlows     int    `json:"total_flows"`
	TotalBytes     uint64 `json:"total_bytes"`
	TotalPackets   uint64 `json:"total_packets"`
	DroppedPackets uint64 `json:"dropped_packets"`
	Sampled        bool   `json:"sampled,omitempty"`
}

// NetFlowReport is the payload agents POST for NetFlow/packet aggregated flows.
type NetFlowReport struct {
	HostID      string       `json:"host_id"`
	Fingerprint string       `json:"fingerprint,omitempty"`
	Source      string       `json:"source"` // "netflow" | "packet"
	Timestamp   int64        `json:"timestamp"`
	WindowSec   int          `json:"window_sec"`
	Flows       []FlowRecord `json:"flows"`
	Stats       NetFlowStats `json:"stats"`
}
