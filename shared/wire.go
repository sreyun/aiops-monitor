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
	CPUPercent  float64 `json:"cpu_percent"`
	CPUCores    int     `json:"cpu_cores"`
	MemTotal    uint64  `json:"mem_total"`
	MemUsed     uint64  `json:"mem_used"`
	MemPercent  float64 `json:"mem_percent"`
	SwapTotal   uint64  `json:"swap_total"`
	SwapUsed    uint64  `json:"swap_used"`
	SwapPercent float64 `json:"swap_percent"`
	DiskTotal   uint64     `json:"disk_total"`
	DiskUsed    uint64     `json:"disk_used"`
	DiskPercent float64    `json:"disk_percent"`
	Disks       []DiskInfo `json:"disks,omitempty"` // per-volume usage for every local disk
	NetSentRate float64    `json:"net_sent_rate"`
	NetRecvRate float64 `json:"net_recv_rate"`
	NetConns    int     `json:"net_conns"` // established TCP connections
	Load1       float64 `json:"load1"`
	Load5       float64 `json:"load5"`
	Load15      float64 `json:"load15"`
	ProcCount   int     `json:"proc_count"`
	Uptime      uint64  `json:"uptime"`
	GPUs        []GPUInfo `json:"gpus,omitempty"` // per-GPU utilization / VRAM (best-effort, cross-platform)
	ProcessNames []string `json:"process_names,omitempty"` // top process names for process-monitor checks
}

// GPUInfo is per-GPU usage. Collection is best-effort and vendor-dependent:
// NVIDIA via nvidia-smi (Linux/Windows), AMD via sysfs (Linux), Apple/other via
// ioreg (macOS). Fields that a platform cannot supply are left zero.
type GPUInfo struct {
	Name        string  `json:"name"`
	UtilPercent float64 `json:"util_percent"`
	MemUsed     uint64  `json:"mem_used,omitempty"`
	MemTotal    uint64  `json:"mem_total,omitempty"`
	MemPercent  float64 `json:"mem_percent,omitempty"`
	Temp        float64 `json:"temp,omitempty"` // °C, 0 if unknown
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
	HostID   string             `json:"host_id"`
	Hostname string             `json:"hostname"`
	OS       string             `json:"os"`
	Platform string             `json:"platform"` // OS / distribution version
	Arch     string             `json:"arch"`
	IP       string             `json:"ip,omitempty"`
	Kernel   string             `json:"kernel,omitempty"`
	Category string             `json:"category,omitempty"`
	Token    string             `json:"token,omitempty"` // install token (optional auth)
	Metrics  Metrics            `json:"metrics"`
	Custom   map[string]float64 `json:"custom,omitempty"`
	Events   []Event            `json:"events,omitempty"`
}
