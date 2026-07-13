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
