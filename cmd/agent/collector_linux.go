//go:build linux

package main

import (
	"bufio"
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"aiops-monitor/shared"
)

var errParse = errors.New("parse error")

type cpuTimes struct{ idle, total uint64 }
type netTotals struct{ rx, tx uint64 }

// linuxCollector reads base metrics straight from procfs and syscalls.
// This is the "high-frequency, close-to-the-system" core the hybrid design
// keeps in Go: no external dependency, tiny footprint, safe to poll often.
type linuxCollector struct {
	diskPath string
	prevCPU  cpuTimes
	prevNet  netTotals
	prevNetT time.Time
	primed   bool
}

func newCollector(diskPath string) Collector {
	if diskPath == "" {
		diskPath = "/"
	}
	return &linuxCollector{diskPath: diskPath}
}

func (c *linuxCollector) Name() string    { return "linux-procfs" }
func (c *linuxCollector) Supported() bool { return true }

func (c *linuxCollector) Collect() (shared.Metrics, error) {
	now := time.Now()
	m := shared.Metrics{CPUCores: runtime.NumCPU()}

	if ct, err := readCPUTimes(); err == nil {
		if c.primed && ct.total > c.prevCPU.total {
			totalDelta := ct.total - c.prevCPU.total
			idleDelta := ct.idle - c.prevCPU.idle
			m.CPUPercent = round1(float64(totalDelta-idleDelta) / float64(totalDelta) * 100)
		}
		c.prevCPU = ct
	}

	if mi, err := readMemInfo(); err == nil {
		if mi.memTotal > 0 {
			used := mi.memTotal - mi.memAvail
			m.MemTotal = mi.memTotal
			m.MemUsed = used
			m.MemPercent = round1(float64(used) / float64(mi.memTotal) * 100)
		}
		if mi.swapTotal > 0 {
			sused := mi.swapTotal - mi.swapFree
			m.SwapTotal = mi.swapTotal
			m.SwapUsed = sused
			m.SwapPercent = round1(float64(sused) / float64(mi.swapTotal) * 100)
		}
	}

	var st syscall.Statfs_t
	if err := syscall.Statfs(c.diskPath, &st); err == nil && st.Blocks > 0 {
		bsize := uint64(st.Bsize)
		total := st.Blocks * bsize
		free := st.Bfree * bsize
		used := total - free
		m.DiskTotal = total
		m.DiskUsed = used
		m.DiskPercent = round1(float64(used) / float64(total) * 100)
	}

	m.Disks = enumLinuxDisks()

	if nt, err := readNet(); err == nil {
		if c.primed && !c.prevNetT.IsZero() {
			if elapsed := now.Sub(c.prevNetT).Seconds(); elapsed > 0 {
				m.NetSentRate = round1(rate(nt.tx, c.prevNet.tx, elapsed))
				m.NetRecvRate = round1(rate(nt.rx, c.prevNet.rx, elapsed))
			}
		}
		c.prevNet = nt
		c.prevNetT = now
	}

	m.NetConns = countTCPConns()

	if l1, l5, l15, err := readLoadAvg(); err == nil {
		m.Load1, m.Load5, m.Load15 = l1, l5, l15
	}

	m.ProcCount = countProcs()
	if up, err := readUptime(); err == nil {
		m.Uptime = up
	}

	c.primed = true
	return m, nil
}

// ---- procfs helpers ----

func readCPUTimes() (cpuTimes, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuTimes{}, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 5 && fields[0] == "cpu" {
			var total, idle uint64
			for i := 1; i < len(fields); i++ {
				v, e := strconv.ParseUint(fields[i], 10, 64)
				if e != nil {
					continue
				}
				total += v
				if i == 4 || i == 5 { // idle + iowait
					idle += v
				}
			}
			return cpuTimes{idle: idle, total: total}, nil
		}
	}
	return cpuTimes{}, errParse
}

type memInfo struct{ memTotal, memAvail, swapTotal, swapFree uint64 }

func readMemInfo() (memInfo, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return memInfo{}, err
	}
	defer f.Close()
	var mi memInfo
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			mi.memTotal = parseMeminfoKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			mi.memAvail = parseMeminfoKB(line)
		case strings.HasPrefix(line, "SwapTotal:"):
			mi.swapTotal = parseMeminfoKB(line)
		case strings.HasPrefix(line, "SwapFree:"):
			mi.swapFree = parseMeminfoKB(line)
		}
	}
	return mi, nil
}

func parseMeminfoKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		v, _ := strconv.ParseUint(fields[1], 10, 64)
		return v * 1024 // kB -> bytes
	}
	return 0
}

func readNet() (netTotals, error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return netTotals{}, err
	}
	defer f.Close()
	var nt netTotals
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue // header lines
		}
		if strings.TrimSpace(line[:idx]) == "lo" {
			continue
		}
		fields := strings.Fields(line[idx+1:])
		if len(fields) >= 9 {
			rx, _ := strconv.ParseUint(fields[0], 10, 64) // recv bytes
			tx, _ := strconv.ParseUint(fields[8], 10, 64) // trans bytes
			nt.rx += rx
			nt.tx += tx
		}
	}
	return nt, nil
}

// countTCPConns counts established TCP connections from procfs (v4 + v6).
// The state column (index 3) is a hex code; 01 == TCP_ESTABLISHED.
func countTCPConns() int {
	n := 0
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		first := true
		for sc.Scan() {
			if first {
				first = false // skip header
				continue
			}
			fields := strings.Fields(sc.Text())
			if len(fields) >= 4 && fields[3] == "01" {
				n++
			}
		}
		f.Close()
	}
	return n
}

func readLoadAvg() (l1, l5, l15 float64, err error) {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, err
	}
	fields := strings.Fields(string(b))
	if len(fields) >= 3 {
		l1, _ = strconv.ParseFloat(fields[0], 64)
		l5, _ = strconv.ParseFloat(fields[1], 64)
		l15, _ = strconv.ParseFloat(fields[2], 64)
		return round2(l1), round2(l5), round2(l15), nil
	}
	return 0, 0, 0, errParse
}

func readUptime() (uint64, error) {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(b))
	if len(fields) >= 1 {
		v, e := strconv.ParseFloat(fields[0], 64)
		return uint64(v), e
	}
	return 0, errParse
}

func countProcs() int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		allDigit := true
		for _, r := range e.Name() {
			if r < '0' || r > '9' {
				allDigit = false
				break
			}
		}
		if allDigit {
			n++
		}
	}
	return n
}

// enumLinuxDisks returns usage for every real filesystem mount, de-duplicated
// by backing device and skipping pseudo filesystems.
func enumLinuxDisks() []shared.DiskInfo {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil
	}
	defer f.Close()
	seen := map[string]bool{}
	var out []shared.DiskInfo
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		dev, mount, fstype := fields[0], fields[1], fields[2]
		if !strings.HasPrefix(dev, "/dev/") || seen[dev] {
			continue
		}
		switch fstype {
		case "proc", "sysfs", "tmpfs", "devtmpfs", "cgroup", "cgroup2",
			"overlay", "squashfs", "autofs", "mqueue", "debugfs", "tracefs", "devpts":
			continue
		}
		var st syscall.Statfs_t
		if err := syscall.Statfs(mount, &st); err != nil || st.Blocks == 0 {
			continue
		}
		seen[dev] = true
		bsize := uint64(st.Bsize)
		total := st.Blocks * bsize
		used := total - st.Bfree*bsize
		out = append(out, shared.DiskInfo{
			Path:    unescapeMount(mount),
			Total:   total,
			Used:    used,
			Percent: round1(float64(used) / float64(total) * 100),
		})
	}
	return out
}

// unescapeMount decodes the octal escapes procfs uses inside mount points
// (space -> \040, tab -> \011, newline -> \012, backslash -> \134).
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(s)
}

func osVersion() string {
	if p := linuxPrettyName(); p != "" {
		return p
	}
	return "Linux"
}

func kernelVersion() string {
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
