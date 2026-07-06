package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"strings"
)

// loadOrCreateHostID returns a stable per-host id, persisting a freshly
// generated one so the agent keeps the same identity across restarts.
func loadOrCreateHostID(path string) string {
	if b, err := os.ReadFile(path); err == nil {
		var s struct {
			HostID string `json:"host_id"`
		}
		if json.Unmarshal(b, &s) == nil && s.HostID != "" {
			return s.HostID
		}
	}
	id := randomID()
	if b, err := json.Marshal(map[string]string{"host_id": id}); err == nil {
		_ = os.WriteFile(path, b, 0o644)
	}
	return id
}

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "host-unknown"
	}
	return hex.EncodeToString(b)
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown-host"
	}
	return h
}

// primaryIP returns the host's primary non-loopback IPv4 address.
func primaryIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}
	return ""
}

// linuxPrettyName reads PRETTY_NAME from /etc/os-release (used by the Linux
// collector's osVersion; harmless no-op elsewhere).
func linuxPrettyName() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
		}
	}
	return ""
}
