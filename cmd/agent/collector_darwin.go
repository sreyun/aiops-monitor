//go:build darwin

package main

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"aiops-monitor/shared"
)

// darwinCollector gathers base metrics with zero third-party dependencies:
// syscall.Statfs for disk, and the always-present sysctl/vm_stat/netstat/ps
// tools for everything else. It cross-compiles from any host (no cgo);
// runtime values should be spot-checked on a real Mac.
type darwinCollector struct {
	diskPath       string
	prevRx, prevTx uint64
	prevNetT       time.Time
	primed         bool
}

func newCollector(diskPath string) Collector {
	if diskPath == "" {
		diskPath = "/"
	}
	return &darwinCollector{diskPath: diskPath}
}

func (c *darwinCollector) Name() string    { return "darwin-sysctl" }
func (c *darwinCollector) Supported() bool { return true }

func (c *darwinCollector) Collect() (shared.Metrics, error) {
	now := time.Now()
	m := shared.Metrics{CPUCores: runtime.NumCPU()}

	m.CPUPercent = darwinCPU()

	total := sysctlUint("hw.memsize")
	used := darwinMemUsed()
	if used > total {
		used = total
	}
	m.MemTotal, m.MemUsed = total, used
	if total > 0 {
		m.MemPercent = round1(float64(used) / float64(total) * 100)
	}

	if st, su := darwinSwap(); st > 0 {
		m.SwapTotal, m.SwapUsed = st, su
		m.SwapPercent = round1(float64(su) / float64(st) * 100)
	}

	var fs syscall.Statfs_t
	if err := syscall.Statfs(c.diskPath, &fs); err == nil && fs.Blocks > 0 {
		bsize := uint64(fs.Bsize)
		dt := fs.Blocks * bsize
		free := fs.Bfree * bsize
		m.DiskTotal = dt
		m.DiskUsed = dt - free
		m.DiskPercent = round1(float64(dt-free) / float64(dt) * 100)
	}

	m.Disks = darwinDisks()

	if rx, tx, ok := darwinNet(); ok {
		if c.primed && !c.prevNetT.IsZero() {
			if el := now.Sub(c.prevNetT).Seconds(); el > 0 {
				m.NetSentRate = round1(rate(tx, c.prevTx, el))
				m.NetRecvRate = round1(rate(rx, c.prevRx, el))
			}
		}
		c.prevRx, c.prevTx = rx, tx
		c.prevNetT = now
	}

	m.NetConns = darwinTCPConns()
	m.Load1, m.Load5, m.Load15 = darwinLoad()
	m.ProcCount = darwinProcCount()
	m.ProcessNames = darwinProcNames()
	m.Uptime = darwinUptime()

	c.primed = true
	return m, nil
}

// ---- helpers (shell out to always-present macOS tools) ----

func run(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func sysctlUint(key string) uint64 {
	v, _ := strconv.ParseUint(strings.TrimSpace(run("sysctl", "-n", key)), 10, 64)
	return v
}

// darwinCPU parses `top -l 2` (two samples; the second reflects real usage).
func darwinCPU() float64 {
	out := run("top", "-l", "2", "-n", "0")
	idle := -1.0
	for _, ln := range strings.Split(out, "\n") {
		i := strings.Index(ln, "CPU usage:")
		if i < 0 {
			continue
		}
		j := strings.Index(ln, "idle")
		if j < 0 {
			continue
		}
		fields := strings.Fields(ln[:j])
		if len(fields) == 0 {
			continue
		}
		v := strings.TrimSuffix(fields[len(fields)-1], "%")
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			idle = f // keep the last (second sample)
		}
	}
	if idle < 0 {
		return 0
	}
	return round1(100 - idle)
}

func darwinMemUsed() uint64 {
	out := run("vm_stat")
	if out == "" {
		return 0
	}
	pageSize := uint64(4096)
	if i := strings.Index(out, "page size of "); i >= 0 {
		f := strings.Fields(out[i+len("page size of "):])
		if len(f) > 0 {
			if v, err := strconv.ParseUint(f[0], 10, 64); err == nil {
				pageSize = v
			}
		}
	}
	get := func(prefix string) uint64 {
		for _, ln := range strings.Split(out, "\n") {
			if strings.HasPrefix(ln, prefix) {
				v := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(ln, prefix)), ".")
				n, _ := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
				return n
			}
		}
		return 0
	}
	return (get("Pages active:") + get("Pages wired down:") + get("Pages occupied by compressor:")) * pageSize
}

func darwinSwap() (total, used uint64) {
	f := strings.Fields(run("sysctl", "-n", "vm.swapusage"))
	parseSize := func(tok string) uint64 {
		mult := 1.0
		switch {
		case strings.HasSuffix(tok, "G"):
			mult, tok = 1<<30, strings.TrimSuffix(tok, "G")
		case strings.HasSuffix(tok, "M"):
			mult, tok = 1<<20, strings.TrimSuffix(tok, "M")
		case strings.HasSuffix(tok, "K"):
			mult, tok = 1<<10, strings.TrimSuffix(tok, "K")
		}
		v, _ := strconv.ParseFloat(tok, 64)
		return uint64(v * mult)
	}
	for i := 0; i+2 < len(f); i++ {
		if f[i+1] != "=" {
			continue
		}
		switch f[i] {
		case "total":
			total = parseSize(f[i+2])
		case "used":
			used = parseSize(f[i+2])
		}
	}
	return total, used
}

// darwinNet sums Ibytes/Obytes per non-loopback interface (first row each).
func darwinNet() (rx, tx uint64, ok bool) {
	out := run("netstat", "-ibn")
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		return 0, 0, false
	}
	ib, ob := -1, -1
	for i, h := range strings.Fields(lines[0]) {
		switch h {
		case "Ibytes":
			ib = i
		case "Obytes":
			ob = i
		}
	}
	if ib < 0 || ob < 0 {
		return 0, 0, false
	}
	seen := map[string]bool{}
	for _, ln := range lines[1:] {
		f := strings.Fields(ln)
		if len(f) <= ob {
			continue
		}
		name := f[0]
		if strings.HasPrefix(name, "lo") || seen[name] {
			continue
		}
		seen[name] = true
		r, _ := strconv.ParseUint(f[ib], 10, 64)
		t, _ := strconv.ParseUint(f[ob], 10, 64)
		rx, tx = rx+r, tx+t
	}
	return rx, tx, true
}

func darwinTCPConns() int {
	n := 0
	for _, ln := range strings.Split(run("netstat", "-an", "-p", "tcp"), "\n") {
		if strings.Contains(ln, "ESTABLISHED") {
			n++
		}
	}
	return n
}

func darwinLoad() (l1, l5, l15 float64) {
	s := strings.NewReplacer("{", "", "}", "").Replace(run("sysctl", "-n", "vm.loadavg"))
	f := strings.Fields(s)
	if len(f) >= 3 {
		l1, _ = strconv.ParseFloat(f[0], 64)
		l5, _ = strconv.ParseFloat(f[1], 64)
		l15, _ = strconv.ParseFloat(f[2], 64)
	}
	return round2(l1), round2(l5), round2(l15)
}

func darwinProcCount() int {
	n := 0
	for _, ln := range strings.Split(run("ps", "-A", "-o", "pid="), "\n") {
		if strings.TrimSpace(ln) != "" {
			n++
		}
	}
	return n
}

// darwinProcNames returns unique process command names via ps, capped at 256.
func darwinProcNames() []string {
	out := run("ps", "-A", "-o", "comm=")
	if out == "" {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for _, ln := range strings.Split(out, "\n") {
		name := strings.TrimSpace(ln)
		if name == "" || seen[name] { continue }
		// ps comm may include full path; take basename
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		if name == "" || seen[name] { continue }
		seen[name] = true
		names = append(names, name)
		if len(names) >= 256 { break }
	}
	return names
}

func darwinUptime() uint64 {
	s := run("sysctl", "-n", "kern.boottime")
	i := strings.Index(s, "sec = ")
	if i < 0 {
		return 0
	}
	rest := s[i+len("sec = "):]
	j := strings.IndexAny(rest, ",}")
	if j < 0 {
		return 0
	}
	sec, err := strconv.ParseUint(strings.TrimSpace(rest[:j]), 10, 64)
	if err != nil {
		return 0
	}
	if now := uint64(time.Now().Unix()); now > sec {
		return now - sec
	}
	return 0
}

// darwinDisks returns usage for every /dev-backed volume via `df -kP`. It
// enumerates all disks dynamically (any number), de-duplicates by device (APFS
// shows several synthesized volumes per container) and tolerates mount points
// that contain spaces.
func darwinDisks() []shared.DiskInfo {
	lines := strings.Split(run("df", "-kP"), "\n")
	if len(lines) < 2 {
		return nil
	}
	seen := map[string]bool{}
	var res []shared.DiskInfo
	for _, ln := range lines[1:] {
		f := strings.Fields(ln)
		if len(f) < 6 || !strings.HasPrefix(f[0], "/dev/") || seen[f[0]] {
			continue
		}
		total, _ := strconv.ParseUint(f[1], 10, 64)
		used, _ := strconv.ParseUint(f[2], 10, 64)
		if total == 0 {
			continue
		}
		seen[f[0]] = true
		res = append(res, shared.DiskInfo{
			Path:    strings.Join(f[5:], " "), // columns 6..n are the mount point
			Total:   total * 1024,
			Used:    used * 1024,
			Percent: round1(float64(used) / float64(total) * 100),
		})
	}
	return res
}

func osVersion() string {
	if v := strings.TrimSpace(run("sw_vers", "-productVersion")); v != "" {
		return "macOS " + v
	}
	return "macOS"
}

func kernelVersion() string {
	return strings.TrimSpace(run("uname", "-r"))
}
