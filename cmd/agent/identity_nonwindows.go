//go:build !windows

package main

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func machineIDFromOS() string {
	switch runtime.GOOS {
	case "linux":
		for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
			if b, err := os.ReadFile(p); err == nil {
				if s := strings.TrimSpace(string(b)); s != "" {
					return s
				}
			}
		}
	case "darwin":
		out, _ := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
		for _, ln := range strings.Split(string(out), "\n") {
			if strings.Contains(ln, "IOPlatformUUID") {
				if i := strings.Index(ln, `= "`); i >= 0 {
					rest := ln[i+3:]
					if j := strings.IndexByte(rest, '"'); j >= 0 {
						return rest[:j]
					}
				}
			}
		}
	}
	return ""
}
