package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// host_inspect：跨平台深度主机巡检（对齐 linux_inspect.sh 的结构化报告思路）。
// 输出纯 JSON，供服务端存储与 Web 渲染。覆盖 Windows / Linux（含麒麟/UOS）/ macOS。

type inspectReport struct {
	Version        string            `json:"version"`
	Timestamp      string            `json:"timestamp"`
	ElapsedSeconds float64           `json:"elapsed_seconds"`
	Host           inspectHost       `json:"host"`
	Metrics        inspectMetrics    `json:"metrics"`
	Sections       []inspectSection  `json:"sections"`
	Findings       []inspectFinding  `json:"findings"`
	Result         inspectResult     `json:"result"`
	Thresholds     inspectThresholds `json:"thresholds"`
}

type inspectHost struct {
	Hostname   string `json:"hostname"`
	FQDN       string `json:"fqdn,omitempty"`
	IP         string `json:"ip"`
	OS         string `json:"os"`
	OSFamily   string `json:"os_family"`
	GOOS       string `json:"goos"`
	Kernel     string `json:"kernel"`
	Arch       string `json:"arch"`
	UptimeDays int    `json:"uptime_days"`
	VirtType   string `json:"virt_type,omitempty"`
	Firewall   string `json:"firewall,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
}

type inspectMetrics struct {
	CPUUsagePct    float64 `json:"cpu_usage_pct"`
	CPUCores       int     `json:"cpu_cores"`
	Load1m         float64 `json:"load_1m"`
	Load5m         float64 `json:"load_5m"`
	Load15m        float64 `json:"load_15m"`
	MemUsagePct    float64 `json:"mem_usage_pct"`
	SwapUsagePct   float64 `json:"swap_usage_pct"`
	DiskAlertCount int     `json:"disk_alert_count"`
	TCPConnections int     `json:"tcp_connections"`
	TCPListen      int     `json:"tcp_listen"`
	ProcessCount   int     `json:"process_count"`
	ZombieCount    int     `json:"zombie_count"`
}

type inspectThresholds struct {
	CPUWarn  float64 `json:"cpu_warn"`
	MemWarn  float64 `json:"mem_warn"`
	DiskWarn float64 `json:"disk_warn"`
	SwapWarn float64 `json:"swap_warn"`
	LoadMult float64 `json:"load_mult"`
}

type inspectSection struct {
	ID      string        `json:"id"`
	Title   string        `json:"title"`
	Status  string        `json:"status"` // ok|warn|crit|info|skip
	Summary string        `json:"summary,omitempty"`
	Items   []inspectItem `json:"items,omitempty"`
}

type inspectItem struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Status string `json:"status,omitempty"`
}

type inspectFinding struct {
	Level   string `json:"level"` // warn|crit
	Message string `json:"message"`
	Section string `json:"section,omitempty"`
}

type inspectResult struct {
	Warnings  int `json:"warnings"`
	Critical  int `json:"critical"`
	ExitCode  int `json:"exit_code"`
}

type inspectBuilder struct {
	rep inspectReport
}

func moduleHostInspect(_ map[string]string) ([]byte, int) {
	start := time.Now()
	b := &inspectBuilder{rep: inspectReport{
		Version:   "1.0",
		Timestamp: time.Now().Format(time.RFC3339),
		Thresholds: inspectThresholds{
			CPUWarn: 80, MemWarn: 85, DiskWarn: 80, SwapWarn: 50, LoadMult: 1.5,
		},
	}}
	b.collectHost()
	b.collectCPU()
	b.collectMem()
	b.collectDisk()
	b.collectNet()
	b.collectProcess()
	b.collectServices()
	b.collectSecurity()
	b.collectTime()
	b.finalize(start)
	out, err := json.Marshal(b.rep)
	if err != nil {
		return []byte(`{"error":"marshal failed"}`), 1
	}
	// 退出码与报告 result.exit_code 对齐：严重=2，警告=1，正常=0
	return out, b.rep.Result.ExitCode
}

func (b *inspectBuilder) addFinding(level, section, msg string) {
	b.rep.Findings = append(b.rep.Findings, inspectFinding{Level: level, Section: section, Message: msg})
	if level == "crit" {
		b.rep.Result.Critical++
	} else {
		b.rep.Result.Warnings++
	}
}

func (b *inspectBuilder) worst(a, c string) string {
	rank := map[string]int{"ok": 0, "info": 0, "skip": 0, "warn": 1, "crit": 2}
	if rank[c] > rank[a] {
		return c
	}
	return a
}

func (b *inspectBuilder) finalize(start time.Time) {
	b.rep.ElapsedSeconds = time.Since(start).Seconds()
	status := "ok"
	for _, s := range b.rep.Sections {
		status = b.worst(status, s.Status)
	}
	_ = status
	if b.rep.Result.Critical > 0 {
		b.rep.Result.ExitCode = 2
	} else if b.rep.Result.Warnings > 0 {
		b.rep.Result.ExitCode = 1
	}
}

func (b *inspectBuilder) collectHost() {
	hn, _ := os.Hostname()
	ips := localIPv4s()
	ip := ""
	if len(ips) > 0 {
		ip = ips[0]
	}
	family, pretty, kernel := detectOSFamily()
	uptimeDays := uptimeDays()
	tz := detectTimezone()
	virt := detectVirt()
	fw := detectFirewall()
	b.rep.Host = inspectHost{
		Hostname: hn, IP: ip, OS: pretty, OSFamily: family, GOOS: runtime.GOOS,
		Kernel: kernel, Arch: runtime.GOARCH, UptimeDays: uptimeDays,
		VirtType: virt, Firewall: fw, Timezone: tz,
	}
	items := []inspectItem{
		{Label: "主机名", Value: hn},
		{Label: "IP", Value: ip},
		{Label: "系统", Value: pretty},
		{Label: "系统族", Value: family},
		{Label: "内核", Value: kernel},
		{Label: "架构", Value: runtime.GOARCH},
		{Label: "运行天数", Value: fmt.Sprintf("%d", uptimeDays)},
		{Label: "虚拟化", Value: virt},
		{Label: "防火墙", Value: fw},
		{Label: "时区", Value: tz},
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{
		ID: "host", Title: "主机概览", Status: "ok", Summary: pretty + " · " + family, Items: items,
	})
}

func detectOSFamily() (family, pretty, kernel string) {
	kernel = runtime.GOOS + "/" + runtime.GOARCH
	switch runtime.GOOS {
	case "windows":
		family = "windows"
		pretty = "Windows"
		if out := cmdOut(3, "cmd", "/c", "ver"); out != "" {
			pretty = strings.TrimSpace(strings.ReplaceAll(out, "\r\n", " "))
		}
		if out := cmdOut(3, "cmd", "/c", "ver"); out != "" {
			kernel = strings.TrimSpace(out)
		}
		return
	case "darwin":
		family = "darwin"
		pretty = "macOS"
		if out := cmdOut(3, "sw_vers", "-productVersion"); out != "" {
			pretty = "macOS " + strings.TrimSpace(out)
		}
		if out := cmdOut(3, "uname", "-r"); out != "" {
			kernel = strings.TrimSpace(out)
		}
		return
	default:
		family = "linux"
		pretty = "Linux"
		if out := cmdOut(3, "uname", "-r"); out != "" {
			kernel = strings.TrimSpace(out)
		}
		id, idLike, ver, p := readOSRelease()
		if p != "" {
			pretty = p
		} else if id != "" {
			pretty = id + " " + ver
		}
		blob := strings.ToLower(id + " " + idLike + " " + p)
		switch {
		case strings.Contains(blob, "kylin") || strings.Contains(blob, "neokylin") || fileExists("/etc/kylin-release"):
			family = "kylin"
			if raw, err := os.ReadFile("/etc/kylin-release"); err == nil && pretty == "Linux" {
				pretty = strings.TrimSpace(string(raw))
			}
		case strings.Contains(blob, "uos") || strings.Contains(blob, "deepin"):
			family = "uos"
		case strings.Contains(blob, "rhel") || strings.Contains(blob, "centos") || strings.Contains(blob, "rocky") ||
			strings.Contains(blob, "alma") || strings.Contains(blob, "fedora") || strings.Contains(blob, "openeuler") ||
			strings.Contains(blob, "anolis") || strings.Contains(blob, "amzn"):
			family = "rhel"
		case strings.Contains(blob, "debian") || strings.Contains(blob, "ubuntu"):
			family = "debian"
		case strings.Contains(blob, "suse"):
			family = "suse"
		case strings.Contains(blob, "arch"):
			family = "arch"
		case strings.Contains(blob, "alpine"):
			family = "alpine"
		}
		return
	}
}

func readOSRelease() (id, idLike, ver, pretty string) {
	raw, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return
	}
	for _, ln := range strings.Split(string(raw), "\n") {
		k, v, ok := strings.Cut(ln, "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"`)
		switch k {
		case "ID":
			id = v
		case "ID_LIKE":
			idLike = v
		case "VERSION_ID":
			ver = v
		case "PRETTY_NAME":
			pretty = v
		}
	}
	return
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func uptimeDays() int {
	switch runtime.GOOS {
	case "linux":
		raw, err := os.ReadFile("/proc/uptime")
		if err != nil {
			return 0
		}
		parts := strings.Fields(string(raw))
		if len(parts) == 0 {
			return 0
		}
		sec, _ := strconv.ParseFloat(parts[0], 64)
		return int(sec / 86400)
	case "darwin":
		out := cmdOut(3, "sysctl", "-n", "kern.boottime")
		// { sec = 123, usec = 0 } ...
		if i := strings.Index(out, "sec = "); i >= 0 {
			rest := out[i+6:]
			n := 0
			fmt.Sscanf(rest, "%d", &n)
			if n > 0 {
				return int(time.Since(time.Unix(int64(n), 0)).Hours() / 24)
			}
		}
	case "windows":
		out := cmdOut(5, "cmd", "/c", "wmic os get lastbootuptime /value")
		for _, ln := range strings.Split(out, "\n") {
			ln = strings.TrimSpace(ln)
			if strings.HasPrefix(ln, "LastBootUpTime=") {
				v := strings.TrimPrefix(ln, "LastBootUpTime=")
				if len(v) >= 14 {
					t, err := time.Parse("20060102150405", v[:14])
					if err == nil {
						return int(time.Since(t).Hours() / 24)
					}
				}
			}
		}
	}
	return 0
}

func detectTimezone() string {
	if z, _ := time.Now().Zone(); z != "" {
		if runtime.GOOS == "linux" {
			if out := cmdOut(2, "timedatectl", "show", "-p", "Timezone", "--value"); out != "" {
				return strings.TrimSpace(out)
			}
			if raw, err := os.ReadFile("/etc/timezone"); err == nil {
				return strings.TrimSpace(string(raw))
			}
		}
		return z
	}
	return ""
}

func detectVirt() string {
	switch runtime.GOOS {
	case "linux":
		if out := cmdOut(2, "systemd-detect-virt"); out != "" && !strings.Contains(out, "none") {
			return strings.TrimSpace(out)
		}
		if raw, err := os.ReadFile("/sys/class/dmi/id/product_name"); err == nil {
			return strings.TrimSpace(string(raw))
		}
	case "windows":
		if out := cmdOut(3, "cmd", "/c", "wmic computersystem get model /value"); out != "" {
			for _, ln := range strings.Split(out, "\n") {
				if strings.HasPrefix(strings.TrimSpace(ln), "Model=") {
					return strings.TrimPrefix(strings.TrimSpace(ln), "Model=")
				}
			}
		}
	case "darwin":
		if out := cmdOut(2, "sysctl", "-n", "machdep.cpu.brand_string"); out != "" {
			return "apple/" + strings.TrimSpace(out)
		}
	}
	return "unknown"
}

func detectFirewall() string {
	switch runtime.GOOS {
	case "linux":
		if out := cmdOut(2, "systemctl", "is-active", "firewalld"); strings.TrimSpace(out) == "active" {
			return "firewalld:active"
		}
		if out := cmdOut(2, "systemctl", "is-active", "ufw"); strings.TrimSpace(out) == "active" {
			return "ufw:active"
		}
		if out := cmdOut(2, "ufw", "status"); strings.Contains(out, "Status: active") {
			return "ufw:active"
		}
		if fileExists("/usr/sbin/iptables") || fileExists("/sbin/iptables") {
			return "iptables:present"
		}
		return "unknown"
	case "windows":
		out := cmdOut(4, "cmd", "/c", "netsh advfirewall show allprofiles state")
		if strings.Contains(strings.ToLower(out), "on") {
			return "windows-firewall:on"
		}
		return "windows-firewall:check"
	case "darwin":
		out := cmdOut(2, "defaults", "read", "/Library/Preferences/com.apple.alf", "globalstate")
		switch strings.TrimSpace(out) {
		case "0":
			return "alf:off"
		case "1", "2":
			return "alf:on"
		}
	}
	return "unknown"
}

func (b *inspectBuilder) collectCPU() {
	cores := runtime.NumCPU()
	b.rep.Metrics.CPUCores = cores
	st := "ok"
	items := []inspectItem{{Label: "逻辑 CPU", Value: fmt.Sprintf("%d", cores)}}
	var load1, load5, load15, cpuPct float64

	switch runtime.GOOS {
	case "linux":
		if raw, err := os.ReadFile("/proc/loadavg"); err == nil {
			f := strings.Fields(string(raw))
			if len(f) >= 3 {
				load1, _ = strconv.ParseFloat(f[0], 64)
				load5, _ = strconv.ParseFloat(f[1], 64)
				load15, _ = strconv.ParseFloat(f[2], 64)
			}
		}
		cpuPct = sampleLinuxCPU()
	case "darwin":
		if out := cmdOut(3, "sysctl", "-n", "vm.loadavg"); out != "" {
			// { 1.2 1.1 1.0 }
			out = strings.Trim(out, "{} \n")
			f := strings.Fields(out)
			if len(f) >= 3 {
				load1, _ = strconv.ParseFloat(f[0], 64)
				load5, _ = strconv.ParseFloat(f[1], 64)
				load15, _ = strconv.ParseFloat(f[2], 64)
			}
		}
		if out := cmdOut(4, "ps", "-A", "-o", "%cpu"); out != "" {
			sum := 0.0
			for i, ln := range strings.Split(out, "\n") {
				if i == 0 {
					continue
				}
				v, err := strconv.ParseFloat(strings.TrimSpace(ln), 64)
				if err == nil {
					sum += v
				}
			}
			cpuPct = sum / float64(cores)
			if cpuPct > 100 {
				cpuPct = 100
			}
		}
	case "windows":
		if out := cmdOut(5, "cmd", "/c", "wmic cpu get loadpercentage /value"); out != "" {
			for _, ln := range strings.Split(out, "\n") {
				ln = strings.TrimSpace(ln)
				if strings.HasPrefix(ln, "LoadPercentage=") {
					cpuPct, _ = strconv.ParseFloat(strings.TrimPrefix(ln, "LoadPercentage="), 64)
				}
			}
		}
	}

	b.rep.Metrics.CPUUsagePct = round1(cpuPct)
	b.rep.Metrics.Load1m, b.rep.Metrics.Load5m, b.rep.Metrics.Load15m = load1, load5, load15
	items = append(items, inspectItem{Label: "CPU 使用率", Value: fmt.Sprintf("%.1f%%", cpuPct)})
	if load1 > 0 || runtime.GOOS != "windows" {
		items = append(items, inspectItem{Label: "Load 1/5/15", Value: fmt.Sprintf("%.2f / %.2f / %.2f", load1, load5, load15)})
	}
	warnLoad := float64(cores) * b.rep.Thresholds.LoadMult
	if cpuPct >= b.rep.Thresholds.CPUWarn+10 {
		st = "crit"
		b.addFinding("crit", "cpu", fmt.Sprintf("CPU 使用率过高: %.1f%%", cpuPct))
	} else if cpuPct >= b.rep.Thresholds.CPUWarn {
		st = "warn"
		b.addFinding("warn", "cpu", fmt.Sprintf("CPU 使用率偏高: %.1f%%", cpuPct))
	}
	if load1 >= warnLoad*1.2 && cores > 0 {
		st = b.worst(st, "crit")
		b.addFinding("crit", "cpu", fmt.Sprintf("1 分钟负载过高: %.2f (阈值≈%.1f)", load1, warnLoad))
	} else if load1 >= warnLoad && cores > 0 {
		st = b.worst(st, "warn")
		b.addFinding("warn", "cpu", fmt.Sprintf("1 分钟负载偏高: %.2f", load1))
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{ID: "cpu", Title: "CPU / 负载", Status: st, Items: items})
}

func sampleLinuxCPU() float64 {
	read := func() (idle, total uint64) {
		raw, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0, 0
		}
		line := strings.SplitN(string(raw), "\n", 2)[0]
		f := strings.Fields(line)
		if len(f) < 5 || f[0] != "cpu" {
			return 0, 0
		}
		var vals []uint64
		for _, x := range f[1:] {
			n, _ := strconv.ParseUint(x, 10, 64)
			vals = append(vals, n)
			total += n
		}
		if len(vals) > 3 {
			idle = vals[3]
		}
		return
	}
	i1, t1 := read()
	time.Sleep(200 * time.Millisecond)
	i2, t2 := read()
	if t2 <= t1 || i2 < i1 {
		return 0
	}
	dt, di := float64(t2-t1), float64(i2-i1)
	return (1 - di/dt) * 100
}

func (b *inspectBuilder) collectMem() {
	st := "ok"
	var memPct, swapPct float64
	var memTotal, memAvail, swapTotal, swapFree uint64
	items := []inspectItem{}

	switch runtime.GOOS {
	case "linux":
		raw, err := os.ReadFile("/proc/meminfo")
		if err == nil {
			kv := map[string]uint64{}
			for _, ln := range strings.Split(string(raw), "\n") {
				f := strings.Fields(ln)
				if len(f) < 2 {
					continue
				}
				n, _ := strconv.ParseUint(f[1], 10, 64)
				kv[strings.TrimSuffix(f[0], ":")] = n // kB
			}
			memTotal = kv["MemTotal"] * 1024
			memAvail = kv["MemAvailable"] * 1024
			if memAvail == 0 {
				memAvail = (kv["MemFree"] + kv["Buffers"] + kv["Cached"]) * 1024
			}
			swapTotal = kv["SwapTotal"] * 1024
			swapFree = kv["SwapFree"] * 1024
		}
	case "darwin":
		if out := cmdOut(2, "sysctl", "-n", "hw.memsize"); out != "" {
			memTotal, _ = strconv.ParseUint(strings.TrimSpace(out), 10, 64)
		}
		// rough: page size * free pages from vm_stat
		pageSize := uint64(4096)
		if out := cmdOut(2, "pagesize"); out != "" {
			if n, err := strconv.ParseUint(strings.TrimSpace(out), 10, 64); err == nil {
				pageSize = n
			}
		}
		freePages := uint64(0)
		for _, ln := range strings.Split(cmdOut(3, "vm_stat"), "\n") {
			if strings.Contains(ln, "Pages free") {
				f := strings.Fields(ln)
				if len(f) > 0 {
					freePages, _ = strconv.ParseUint(strings.TrimSuffix(f[len(f)-1], "."), 10, 64)
				}
			}
		}
		memAvail = freePages * pageSize
	case "windows":
		out := cmdOut(5, "cmd", "/c", "wmic OS get FreePhysicalMemory,TotalVisibleMemorySize /value")
		var freeKB, totalKB uint64
		for _, ln := range strings.Split(out, "\n") {
			ln = strings.TrimSpace(ln)
			if strings.HasPrefix(ln, "FreePhysicalMemory=") {
				freeKB, _ = strconv.ParseUint(strings.TrimPrefix(ln, "FreePhysicalMemory="), 10, 64)
			}
			if strings.HasPrefix(ln, "TotalVisibleMemorySize=") {
				totalKB, _ = strconv.ParseUint(strings.TrimPrefix(ln, "TotalVisibleMemorySize="), 10, 64)
			}
		}
		memTotal = totalKB * 1024
		memAvail = freeKB * 1024
	}

	if memTotal > 0 {
		used := memTotal - memAvail
		if used > memTotal {
			used = memTotal
		}
		memPct = float64(used) / float64(memTotal) * 100
		items = append(items,
			inspectItem{Label: "内存总量", Value: humanBytes(memTotal)},
			inspectItem{Label: "可用内存", Value: humanBytes(memAvail)},
			inspectItem{Label: "内存使用率", Value: fmt.Sprintf("%.1f%%", memPct)},
		)
	}
	if swapTotal > 0 {
		swapUsed := swapTotal - swapFree
		swapPct = float64(swapUsed) / float64(swapTotal) * 100
		items = append(items, inspectItem{Label: "Swap 使用率", Value: fmt.Sprintf("%.1f%%", swapPct)})
	}
	b.rep.Metrics.MemUsagePct = round1(memPct)
	b.rep.Metrics.SwapUsagePct = round1(swapPct)
	if memPct >= b.rep.Thresholds.MemWarn+10 {
		st = "crit"
		b.addFinding("crit", "mem", fmt.Sprintf("内存使用率过高: %.1f%%", memPct))
	} else if memPct >= b.rep.Thresholds.MemWarn {
		st = "warn"
		b.addFinding("warn", "mem", fmt.Sprintf("内存使用率偏高: %.1f%%", memPct))
	}
	if swapPct >= 80 {
		st = b.worst(st, "crit")
		b.addFinding("crit", "mem", fmt.Sprintf("Swap 使用率过高: %.1f%%", swapPct))
	} else if swapPct >= b.rep.Thresholds.SwapWarn {
		st = b.worst(st, "warn")
		b.addFinding("warn", "mem", fmt.Sprintf("Swap 使用率偏高: %.1f%%", swapPct))
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{ID: "mem", Title: "内存 / Swap", Status: st, Items: items})
}

func (b *inspectBuilder) collectDisk() {
	st := "ok"
	items := []inspectItem{}
	alert := 0
	type diskRow struct{ mount, fstype, size, used, avail, pct string }

	var rows []diskRow
	switch runtime.GOOS {
	case "windows":
		out := cmdOut(8, "cmd", "/c", "wmic logicaldisk where DriveType=3 get DeviceID,FreeSpace,Size /format:csv")
		for i, ln := range strings.Split(out, "\n") {
			ln = strings.TrimSpace(ln)
			if i == 0 || ln == "" || strings.HasPrefix(ln, "Node,") {
				continue
			}
			f := strings.Split(ln, ",")
			if len(f) < 4 {
				continue
			}
			id, freeS, sizeS := f[len(f)-3], f[len(f)-2], f[len(f)-1]
			free, _ := strconv.ParseUint(freeS, 10, 64)
			size, _ := strconv.ParseUint(sizeS, 10, 64)
			if size == 0 {
				continue
			}
			used := size - free
			pct := float64(used) / float64(size) * 100
			rows = append(rows, diskRow{mount: id, size: humanBytes(size), used: humanBytes(used), avail: humanBytes(free), pct: fmt.Sprintf("%.0f%%", pct)})
			ist := "ok"
			if pct >= b.rep.Thresholds.DiskWarn+10 {
				ist, st, alert = "crit", b.worst(st, "crit"), alert+1
				b.addFinding("crit", "disk", fmt.Sprintf("%s 磁盘使用率过高: %.0f%%", id, pct))
			} else if pct >= b.rep.Thresholds.DiskWarn {
				ist, st, alert = "warn", b.worst(st, "warn"), alert+1
				b.addFinding("warn", "disk", fmt.Sprintf("%s 磁盘使用率偏高: %.0f%%", id, pct))
			}
			items = append(items, inspectItem{Label: id, Value: fmt.Sprintf("%s 已用 / %s (%.0f%%)", humanBytes(used), humanBytes(size), pct), Status: ist})
		}
	default:
		args := []string{"-hP"}
		if runtime.GOOS == "linux" {
			args = []string{"-hPT"}
		}
		out := cmdOut(5, "df", args...)
		for i, ln := range strings.Split(out, "\n") {
			if i == 0 || strings.TrimSpace(ln) == "" {
				continue
			}
			f := strings.Fields(ln)
			if len(f) < 6 {
				continue
			}
			// Linux df -hPT: Filesystem Type Size Used Avail Use% Mounted
			// macOS df -hP: Filesystem Size Used Avail Capacity Mounted
			var fstype, size, used, avail, usep, mount string
			if runtime.GOOS == "linux" && len(f) >= 7 {
				fstype, size, used, avail, usep, mount = f[1], f[2], f[3], f[4], f[5], f[6]
			} else {
				size, used, avail, usep, mount = f[1], f[2], f[3], f[4], strings.Join(f[5:], " ")
			}
			if skipMount(mount, fstype) {
				continue
			}
			pctStr := strings.TrimSuffix(usep, "%")
			pct, _ := strconv.ParseFloat(pctStr, 64)
			ist := "ok"
			if pct >= b.rep.Thresholds.DiskWarn+10 {
				ist, st, alert = "crit", b.worst(st, "crit"), alert+1
				b.addFinding("crit", "disk", fmt.Sprintf("%s 磁盘使用率过高: %.0f%%", mount, pct))
			} else if pct >= b.rep.Thresholds.DiskWarn {
				ist, st, alert = "warn", b.worst(st, "warn"), alert+1
				b.addFinding("warn", "disk", fmt.Sprintf("%s 磁盘使用率偏高: %.0f%%", mount, pct))
			}
			items = append(items, inspectItem{
				Label: mount, Value: fmt.Sprintf("%s 已用 / %s · 可用 %s (%s)", used, size, avail, usep), Status: ist,
			})
			_ = rows
		}
	}
	b.rep.Metrics.DiskAlertCount = alert
	sum := fmt.Sprintf("%d 个挂载点", len(items))
	if alert > 0 {
		sum += fmt.Sprintf("，%d 个超阈值", alert)
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{ID: "disk", Title: "磁盘空间", Status: st, Summary: sum, Items: items})
}

func skipMount(mount, fstype string) bool {
	ft := strings.ToLower(fstype)
	if strings.HasPrefix(ft, "tmpfs") || ft == "devtmpfs" || ft == "squashfs" || ft == "overlay" || ft == "proc" || ft == "sysfs" {
		return true
	}
	if strings.HasPrefix(mount, "/snap") || strings.HasPrefix(mount, "/run") || strings.HasPrefix(mount, "/sys") || strings.HasPrefix(mount, "/proc") {
		return true
	}
	return false
}

func (b *inspectBuilder) collectNet() {
	st := "ok"
	items := []inspectItem{}
	listen := 0
	conns := 0

	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		var ips []string
		for _, a := range addrs {
			ips = append(ips, a.String())
		}
		if len(ips) == 0 {
			continue
		}
		items = append(items, inspectItem{Label: iface.Name, Value: strings.Join(ips, ", ")})
	}

	switch runtime.GOOS {
	case "windows":
		out := cmdOut(8, "cmd", "/c", "netstat -ano")
		for _, ln := range strings.Split(out, "\n") {
			u := strings.ToUpper(ln)
			if strings.Contains(u, "LISTENING") {
				listen++
			}
			if strings.Contains(u, "TCP") {
				conns++
			}
		}
	default:
		out := cmdOut(5, "ss", "-s")
		if out == "" {
			out = cmdOut(5, "netstat", "-an")
		}
		for _, ln := range strings.Split(out, "\n") {
			l := strings.ToLower(ln)
			if strings.Contains(l, "listen") {
				listen++
			}
			if strings.Contains(l, "estab") || strings.Contains(l, "established") {
				conns++
			}
		}
		if listen == 0 {
			if lo := cmdOut(5, "ss", "-lnt"); lo != "" {
				listen = maxInt(0, strings.Count(lo, "\n")-1)
			}
		}
	}
	b.rep.Metrics.TCPListen = listen
	b.rep.Metrics.TCPConnections = conns
	items = append(items,
		inspectItem{Label: "监听端口数(估)", Value: fmt.Sprintf("%d", listen)},
		inspectItem{Label: "TCP 连接(估)", Value: fmt.Sprintf("%d", conns)},
	)
	b.rep.Sections = append(b.rep.Sections, inspectSection{ID: "net", Title: "网络", Status: st, Items: items})
}

func (b *inspectBuilder) collectProcess() {
	st := "ok"
	items := []inspectItem{}
	procs, zombies := 0, 0
	switch runtime.GOOS {
	case "windows":
		out := cmdOut(8, "cmd", "/c", "tasklist /FO CSV /NH")
		procs = maxInt(0, strings.Count(out, "\n"))
	case "darwin":
		out := cmdOut(5, "ps", "-axo", "stat=")
		for _, ln := range strings.Split(out, "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			procs++
			if strings.HasPrefix(ln, "Z") {
				zombies++
			}
		}
	default:
		out := cmdOut(5, "ps", "-eo", "stat=")
		for _, ln := range strings.Split(out, "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			procs++
			if strings.Contains(ln, "Z") {
				zombies++
			}
		}
	}
	b.rep.Metrics.ProcessCount = procs
	b.rep.Metrics.ZombieCount = zombies
	items = append(items, inspectItem{Label: "进程数", Value: fmt.Sprintf("%d", procs)})
	items = append(items, inspectItem{Label: "僵尸进程", Value: fmt.Sprintf("%d", zombies)})
	if zombies > 20 {
		st = "crit"
		b.addFinding("crit", "proc", fmt.Sprintf("僵尸进程过多: %d", zombies))
	} else if zombies > 0 {
		st = "warn"
		b.addFinding("warn", "proc", fmt.Sprintf("存在僵尸进程: %d", zombies))
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{ID: "proc", Title: "进程", Status: st, Items: items})
}

func (b *inspectBuilder) collectServices() {
	st := "info"
	items := []inspectItem{}
	names := []string{"sshd", "ssh", "cron", "crond", "docker", "containerd", "nginx", "httpd", "mysqld", "postgresql"}
	switch runtime.GOOS {
	case "windows":
		for _, n := range []string{"Wuauserv", "EventLog", "Winmgmt", "Schedule"} {
			out := cmdOut(3, "sc", "query", n)
			status := "unknown"
			if strings.Contains(out, "RUNNING") {
				status = "running"
			} else if strings.Contains(out, "STOPPED") {
				status = "stopped"
			} else if strings.Contains(out, "1060") {
				status = "notfound"
			}
			items = append(items, inspectItem{Label: n, Value: status})
		}
	case "darwin":
		out := cmdOut(4, "launchctl", "list")
		for _, n := range []string{"com.openssh.sshd", "com.apple.cron"} {
			val := "notfound"
			if strings.Contains(out, n) {
				val = "loaded"
			}
			items = append(items, inspectItem{Label: n, Value: val})
		}
	default:
		for _, n := range names {
			val := "notfound"
			if out := cmdOut(2, "systemctl", "is-active", n); out != "" {
				val = strings.TrimSpace(out)
			}
			if val == "notfound" || val == "" {
				continue
			}
			items = append(items, inspectItem{Label: n, Value: val})
		}
	}
	if len(items) == 0 {
		st = "skip"
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{ID: "svc", Title: "关键服务", Status: st, Items: items})
}

func (b *inspectBuilder) collectSecurity() {
	st := "info"
	items := []inspectItem{
		{Label: "防火墙", Value: b.rep.Host.Firewall},
	}
	switch runtime.GOOS {
	case "linux":
		if out := cmdOut(3, "getenforce"); out != "" {
			items = append(items, inspectItem{Label: "SELinux", Value: strings.TrimSpace(out)})
		}
		failed := 0
		if out := cmdOut(5, "journalctl", "-n", "100", "--no-pager", "-u", "sshd"); out != "" {
			failed = strings.Count(strings.ToLower(out), "failed") + strings.Count(out, "Invalid user")
		}
		items = append(items, inspectItem{Label: "近期 SSH 失败关键词(估)", Value: fmt.Sprintf("%d", failed)})
		if failed > 30 {
			st = "warn"
			b.addFinding("warn", "sec", fmt.Sprintf("近期 SSH 认证失败较多: %d", failed))
		}
	case "windows":
		items = append(items, inspectItem{Label: "说明", Value: "请结合 Windows 安全事件日志复核"})
	case "darwin":
		items = append(items, inspectItem{Label: "说明", Value: "请结合 Console 认证日志复核"})
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{ID: "sec", Title: "安全", Status: st, Items: items})
}

func (b *inspectBuilder) collectTime() {
	st := "ok"
	now := time.Now().Format(time.RFC3339)
	items := []inspectItem{
		{Label: "本机时间", Value: now},
		{Label: "时区", Value: b.rep.Host.Timezone},
	}
	synced := "unknown"
	switch runtime.GOOS {
	case "linux":
		if out := cmdOut(2, "timedatectl", "show", "-p", "NTPSynchronized", "--value"); out != "" {
			synced = strings.TrimSpace(out)
			if synced == "no" {
				st = "warn"
				b.addFinding("warn", "time", "NTP 未同步")
			}
		}
	case "darwin":
		synced = "sntp/check"
	case "windows":
		if out := cmdOut(3, "w32tm", "/query", "/status"); strings.Contains(out, "Leap Indicator") {
			synced = "w32tm:ok"
		}
	}
	items = append(items, inspectItem{Label: "时间同步", Value: synced})
	b.rep.Sections = append(b.rep.Sections, inspectSection{ID: "time", Title: "时间同步", Status: st, Items: items})
}

func cmdOut(timeoutSec int, name string, args ...string) string {
	if timeoutSec <= 0 {
		timeoutSec = 5
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	return string(out)
}

func humanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
