//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"aiops-monitor/shared"
)

// windowsCollector reads base metrics through the Win32 API via
// syscall.NewLazyDLL — no cgo, no third-party dependency, mirroring the
// zero-dependency spirit of the Linux procfs collector.
var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	modpsapi    = syscall.NewLazyDLL("psapi.dll")
	modiphlpapi = syscall.NewLazyDLL("iphlpapi.dll")

	procGetSystemTimes       = modkernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx = modkernel32.NewProc("GlobalMemoryStatusEx")
	procGetDiskFreeSpaceExW  = modkernel32.NewProc("GetDiskFreeSpaceExW")
	procGetTickCount64       = modkernel32.NewProc("GetTickCount64")
	procGetLogicalDrives     = modkernel32.NewProc("GetLogicalDriveStringsW")
	procGetDriveType         = modkernel32.NewProc("GetDriveTypeW")
	procEnumProcesses        = modpsapi.NewProc("EnumProcesses")
	procGetIfTable           = modiphlpapi.NewProc("GetIfTable")
	procGetTcpTable          = modiphlpapi.NewProc("GetTcpTable")
	procRtlGetVersion        = syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion")
)

type filetime struct {
	low  uint32
	high uint32
}

func (f filetime) u64() uint64 { return uint64(f.high)<<32 | uint64(f.low) }

// memoryStatusEx mirrors Win32 MEMORYSTATUSEX (64 bytes).
type memoryStatusEx struct {
	length     uint32
	memoryLoad uint32
	totalPhys  uint64
	availPhys  uint64
	totalPage  uint64
	availPage  uint64
	totalVirt  uint64
	availVirt  uint64
	availExt   uint64
}

type windowsCollector struct {
	diskPath                       string
	prevIdle, prevKernel, prevUser uint64
	prevRx, prevTx                 uint64
	prevNetT                       time.Time
	load1, load5, load15           float64
	lastT                          time.Time
	primed                         bool
}

func newCollector(diskPath string) Collector {
	if diskPath == "" {
		diskPath = defaultDiskPath()
	}
	return &windowsCollector{diskPath: diskPath}
}

func (c *windowsCollector) Name() string    { return "windows-winapi" }
func (c *windowsCollector) Supported() bool { return true }

func (c *windowsCollector) Collect() (shared.Metrics, error) {
	now := time.Now()
	m := shared.Metrics{CPUCores: runtime.NumCPU()}

	// ---- CPU: GetSystemTimes (kernel time already includes idle) ----
	var idle, kernel, user filetime
	if r, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	); r != 0 {
		i, k, u := idle.u64(), kernel.u64(), user.u64()
		if c.primed {
			idleD := i - c.prevIdle
			totalD := (k - c.prevKernel) + (u - c.prevUser)
			if totalD > 0 {
				m.CPUPercent = round1(float64(totalD-idleD) / float64(totalD) * 100)
			}
		}
		c.prevIdle, c.prevKernel, c.prevUser = i, k, u
	}

	// ---- Memory + page file (Windows "swap") ----
	var msx memoryStatusEx
	msx.length = uint32(unsafe.Sizeof(msx))
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&msx))); r != 0 {
		m.MemTotal = msx.totalPhys
		m.MemUsed = msx.totalPhys - msx.availPhys
		if msx.totalPhys > 0 {
			m.MemPercent = round1(float64(m.MemUsed) / float64(msx.totalPhys) * 100)
		}
		// The commit limit beyond physical RAM approximates the page file.
		if msx.totalPage > msx.totalPhys {
			swapTotal := msx.totalPage - msx.totalPhys
			usedCommit := msx.totalPage - msx.availPage
			usedPhys := msx.totalPhys - msx.availPhys
			var swapUsed uint64
			if usedCommit > usedPhys {
				swapUsed = usedCommit - usedPhys
			}
			if swapUsed > swapTotal {
				swapUsed = swapTotal
			}
			m.SwapTotal = swapTotal
			m.SwapUsed = swapUsed
			if swapTotal > 0 {
				m.SwapPercent = round1(float64(swapUsed) / float64(swapTotal) * 100)
			}
		}
	}

	// ---- Disk: GetDiskFreeSpaceExW ----
	if p, err := syscall.UTF16PtrFromString(c.diskPath); err == nil {
		var freeAvail, total, totalFree uint64
		if r, _, _ := procGetDiskFreeSpaceExW.Call(
			uintptr(unsafe.Pointer(p)),
			uintptr(unsafe.Pointer(&freeAvail)),
			uintptr(unsafe.Pointer(&total)),
			uintptr(unsafe.Pointer(&totalFree)),
		); r != 0 && total > 0 {
			m.DiskTotal = total
			m.DiskUsed = total - totalFree
			m.DiskPercent = round1(float64(total-totalFree) / float64(total) * 100)
		}
	}

	// ---- All local fixed disks (C:, D:, …) ----
	m.Disks = enumWindowsDisks()

	// ---- Network: GetIfTable, byte counters diffed over time ----
	if rx, tx, ok := readIfTable(); ok {
		if c.primed && !c.prevNetT.IsZero() {
			if elapsed := now.Sub(c.prevNetT).Seconds(); elapsed > 0 {
				m.NetSentRate = round1(rate(tx, c.prevTx, elapsed))
				m.NetRecvRate = round1(rate(rx, c.prevRx, elapsed))
			}
		}
		c.prevRx, c.prevTx = rx, tx
		c.prevNetT = now
	}

	// ---- Established TCP connections ----
	m.NetConns = countTCPConns()

	// ---- Process count ----
	m.ProcCount = countProcs()

	// ---- Uptime: GetTickCount64 (ms) ----
	if r, _, _ := procGetTickCount64.Call(); r != 0 {
		m.Uptime = uint64(r) / 1000
	}

	// ---- Load average: Windows has none, approximate via EWMA of busy cores ----
	c.updateLoad(m.CPUPercent, m.CPUCores, now)
	m.Load1, m.Load5, m.Load15 = round2(c.load1), round2(c.load5), round2(c.load15)

	c.lastT = now
	c.primed = true
	return m, nil
}

// updateLoad maintains a Unix-like 1/5/15-minute load approximation. Windows
// exposes no run-queue load average, so we treat "busy cores" (cpu% * cores)
// as the instantaneous load and smooth it with an exponentially weighted
// moving average whose decay matches the sampling interval.
func (c *windowsCollector) updateLoad(cpuPercent float64, cores int, now time.Time) {
	instant := cpuPercent / 100.0 * float64(cores)
	if !c.primed || c.lastT.IsZero() {
		c.load1, c.load5, c.load15 = instant, instant, instant
		return
	}
	dt := now.Sub(c.lastT).Seconds()
	if dt <= 0 {
		return
	}
	c.load1 = ewma(c.load1, instant, dt, 60)
	c.load5 = ewma(c.load5, instant, dt, 300)
	c.load15 = ewma(c.load15, instant, dt, 900)
}

func ewma(prev, x, dt, tau float64) float64 {
	alpha := 1 - math.Exp(-dt/tau)
	return prev + alpha*(x-prev)
}

// readIfTable sums per-interface byte counters from the classic MIB_IFTABLE,
// skipping the software loopback. Fields are read at fixed offsets to avoid
// any struct-layout ambiguity. Counters are 32-bit; rate() tolerates wrap.
func readIfTable() (rx, tx uint64, ok bool) {
	var size uint32
	procGetIfTable.Call(0, uintptr(unsafe.Pointer(&size)), 0) // size probe
	if size == 0 {
		return 0, 0, false
	}
	buf := make([]byte, size)
	if r, _, _ := procGetIfTable.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		0,
	); r != 0 { // NO_ERROR == 0
		return 0, 0, false
	}
	const (
		rowSize        = 860
		offType        = 512
		offInOctets    = 552
		offOutOctets   = 576
		ifTypeLoopback = 24
	)
	n := binary.LittleEndian.Uint32(buf[0:])
	for i := 0; i < int(n); i++ {
		base := 4 + i*rowSize
		if base+offOutOctets+4 > len(buf) {
			break
		}
		if binary.LittleEndian.Uint32(buf[base+offType:]) == ifTypeLoopback {
			continue
		}
		rx += uint64(binary.LittleEndian.Uint32(buf[base+offInOctets:]))
		tx += uint64(binary.LittleEndian.Uint32(buf[base+offOutOctets:]))
	}
	return rx, tx, true
}

// countTCPConns counts established IPv4 TCP connections via GetTcpTable.
// MIB_TCPROW is 20 bytes with dwState first; MIB_TCP_STATE_ESTAB == 5.
func countTCPConns() int {
	var size uint32
	procGetTcpTable.Call(0, uintptr(unsafe.Pointer(&size)), 0)
	if size == 0 {
		return 0
	}
	buf := make([]byte, size)
	if r, _, _ := procGetTcpTable.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		0,
	); r != 0 {
		return 0
	}
	const (
		rowSize    = 20
		stateEstab = 5
	)
	n := binary.LittleEndian.Uint32(buf[0:])
	count := 0
	for i := 0; i < int(n); i++ {
		base := 4 + i*rowSize
		if base+4 > len(buf) {
			break
		}
		if binary.LittleEndian.Uint32(buf[base:]) == stateEstab {
			count++
		}
	}
	return count
}

// countProcs returns the number of running processes via EnumProcesses,
// growing the id buffer until it is not saturated.
func countProcs() int {
	size := 1024
	for {
		ids := make([]uint32, size)
		var needed uint32
		if r, _, _ := procEnumProcesses.Call(
			uintptr(unsafe.Pointer(&ids[0])),
			uintptr(size*4),
			uintptr(unsafe.Pointer(&needed)),
		); r == 0 {
			return 0
		}
		got := int(needed / 4)
		if got < size || size >= 1<<20 {
			return got
		}
		size *= 2
	}
}

// enumWindowsDisks returns usage for every fixed (local) drive. The logical
// drive strings come back as a double-NUL-terminated "C:\<NUL>D:\<NUL><NUL>"
// list; each fixed drive (DRIVE_FIXED) is measured with GetDiskFreeSpaceExW.
func enumWindowsDisks() []shared.DiskInfo {
	buf := make([]uint16, 256)
	r, _, _ := procGetLogicalDrives.Call(uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	if r == 0 || int(r) > len(buf) {
		return nil
	}
	const driveFixed = 3 // DRIVE_FIXED
	var out []shared.DiskInfo
	start := 0
	for i := 0; i < int(r); i++ {
		if buf[i] != 0 {
			continue
		}
		if i > start {
			root := buf[start : i+1] // include the terminating NUL for the Win32 calls
			if dt, _, _ := procGetDriveType.Call(uintptr(unsafe.Pointer(&root[0]))); dt == driveFixed {
				if di, ok := winDiskUsage(&root[0], syscall.UTF16ToString(buf[start:i])); ok {
					out = append(out, di)
				}
			}
		}
		start = i + 1
	}
	return out
}

func winDiskUsage(rootPtr *uint16, label string) (shared.DiskInfo, bool) {
	var freeAvail, total, totalFree uint64
	if r, _, _ := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(rootPtr)),
		uintptr(unsafe.Pointer(&freeAvail)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&totalFree)),
	); r == 0 || total == 0 {
		return shared.DiskInfo{}, false
	}
	used := total - totalFree
	return shared.DiskInfo{
		Path:    strings.TrimSuffix(label, "\\"),
		Total:   total,
		Used:    used,
		Percent: round1(float64(used) / float64(total) * 100),
	}, true
}

// osVersionInfo mirrors Win32 RTL_OSVERSIONINFOW.
type osVersionInfo struct {
	dwOSVersionInfoSize uint32
	dwMajorVersion      uint32
	dwMinorVersion      uint32
	dwBuildNumber       uint32
	dwPlatformId        uint32
	szCSDVersion        [128]uint16
}

func winVersion() (major, minor, build uint32) {
	var vi osVersionInfo
	vi.dwOSVersionInfoSize = uint32(unsafe.Sizeof(vi))
	procRtlGetVersion.Call(uintptr(unsafe.Pointer(&vi)))
	return vi.dwMajorVersion, vi.dwMinorVersion, vi.dwBuildNumber
}

func osVersion() string {
	maj, _, build := winVersion()
	name := "Windows"
	if maj == 10 && build >= 22000 {
		name = "Windows 11"
	} else if maj == 10 {
		name = "Windows 10"
	}
	return fmt.Sprintf("%s (Build %d)", name, build)
}

func kernelVersion() string {
	maj, min, build := winVersion()
	return fmt.Sprintf("%d.%d.%d", maj, min, build)
}
