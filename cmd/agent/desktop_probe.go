package main

import (
	"net"
	"runtime"
	"strconv"
	"sync"
	"time"

	"aiops-monitor/shared"
)

// desktopProbePorts are common RDP / VNC listen ports we check on localhost.
var desktopProbePorts = []int{3389, 5900, 5901, 5902, 33890}

var (
	desktopProbeMu    sync.Mutex
	desktopProbeCache *shared.DesktopInfo
	desktopProbeAt    time.Time
)

// probeDesktopServices checks whether common desktop remote ports are accepting
// connections on localhost. Cached briefly so the report loop stays cheap.
func probeDesktopServices() *shared.DesktopInfo {
	desktopProbeMu.Lock()
	if desktopProbeCache != nil && time.Since(desktopProbeAt) < 60*time.Second {
		c := *desktopProbeCache
		desktopProbeMu.Unlock()
		return &c
	}
	desktopProbeMu.Unlock()

	info := &shared.DesktopInfo{OS: runtime.GOOS}
	for _, p := range desktopProbePorts {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)), 200*time.Millisecond)
		if err != nil {
			continue
		}
		_ = conn.Close()
		info.Ports = append(info.Ports, p)
		if p == 3389 || p == 33890 {
			info.RDP = true
		}
		if p >= 5900 && p <= 5999 {
			info.VNC = true
		}
	}
	switch runtime.GOOS {
	case "windows":
		info.Preferred = "rdp"
		info.PreferredPort = 3389
	case "darwin":
		info.Preferred = "vnc"
		info.PreferredPort = 5900
	default:
		if info.RDP {
			info.Preferred = "rdp"
			info.PreferredPort = 3389
		} else {
			info.Preferred = "vnc"
			info.PreferredPort = 5900
		}
	}
	if len(info.Ports) > 0 {
		for _, p := range info.Ports {
			if info.Preferred == "rdp" && (p == 3389 || p == 33890) {
				info.PreferredPort = p
				break
			}
			if info.Preferred == "vnc" && p >= 5900 && p <= 5999 {
				info.PreferredPort = p
				break
			}
		}
	}

	desktopProbeMu.Lock()
	desktopProbeCache = info
	desktopProbeAt = time.Now()
	desktopProbeMu.Unlock()
	cp := *info
	return &cp
}
