//go:build linux

package main

import (
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ensureLinuxAgentUnitPrivileges rewrites systemd units that still sandbox the
// agent or run it as a non-root user. Those configs make the remote terminal
// look "read-only" (vim E45 on /etc/*, ProtectHome blocking $HOME, etc.).
//
// Opt out of User=root escalation with an empty file:
//
//	/etc/aiops-agent/allow-nonroot
func ensureLinuxAgentUnitPrivileges() {
	if os.Geteuid() != 0 {
		slog.Warn("Agent 以非 root 运行：远程终端无法直接写入 /etc 等系统路径；请用 sudo 或重装（默认 root）",
			"uid", os.Geteuid(), "hint", "curl … | sudo bash  # 或显式 AIOPS_USER=root")
		return
	}
	allowNonRoot := false
	if _, err := os.Stat("/etc/aiops-agent/allow-nonroot"); err == nil {
		allowNonRoot = true
	}

	healed := false
	for _, name := range []string{"aiops-agent", "aiops-monitor-agent"} {
		path := "/etc/systemd/system/" + name + ".service"
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		next, changed := healLinuxUnitBody(string(body), allowNonRoot)
		if !changed {
			continue
		}
		if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
			slog.Warn("重写 systemd unit 失败", "unit", name, "err", err)
			continue
		}
		_ = os.RemoveAll("/etc/systemd/system/" + name + ".service.d")
		_ = os.RemoveAll("/run/systemd/system/" + name + ".service.d")
		_ = os.RemoveAll("/lib/systemd/system/" + name + ".service.d")
		_ = os.RemoveAll("/usr/lib/systemd/system/" + name + ".service.d")
		slog.Info("已重写 Agent systemd unit（解除沙箱 / 恢复 root 终端权限）", "unit", name)
		healed = true
	}
	if !healed {
		return
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	go func() {
		time.Sleep(800 * time.Millisecond)
		_ = exec.Command("systemctl", "restart", detectLinuxAgentUnit()).Run()
	}()
}

func healLinuxUnitBody(body string, allowNonRoot bool) (string, bool) {
	if !linuxUnitNeedsPrivilegeHeal(body, allowNonRoot) {
		return body, false
	}
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines)+8)
	var (
		hasProtectHome   bool
		hasProtectSystem bool
		hasPrivateTmp    bool
		hasNNP           bool
		hasUser          bool
		hasGroup         bool
		hasHOME          bool
		hasUSER          bool
		hasSHELL         bool
		inService        bool
	)
	skipPrefixes := []string{
		"CapabilityBoundingSet=",
		"ReadWritePaths=",
		"ReadOnlyPaths=",
		"InaccessiblePaths=",
		"TemporaryFileSystem=",
		"ProtectKernelTunables=",
		"ProtectKernelModules=",
		"ProtectControlGroups=",
		"RestrictSUIDSGID=",
		"SystemCallFilter=",
		"MemoryDenyWriteExecute=",
		"LockPersonality=",
		"RestrictRealtime=",
		"PrivateDevices=",
		"PrivateUsers=",
	}
	for _, ln := range lines {
		trim := strings.TrimSpace(ln)
		if trim == "[Service]" {
			inService = true
			out = append(out, ln)
			continue
		}
		if strings.HasPrefix(trim, "[") && trim != "[Service]" {
			if inService {
				out = appendUnlockDirectives(out, hasProtectHome, hasProtectSystem, hasPrivateTmp, hasNNP, hasUser, hasGroup, hasHOME, hasUSER, hasSHELL, allowNonRoot)
				inService = false
			}
			out = append(out, ln)
			continue
		}
		if !inService {
			out = append(out, ln)
			continue
		}
		skip := false
		for _, p := range skipPrefixes {
			if strings.HasPrefix(trim, p) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		switch {
		case strings.HasPrefix(trim, "User="):
			hasUser = true
			if allowNonRoot {
				out = append(out, ln)
			} else {
				out = append(out, "User=root")
			}
		case strings.HasPrefix(trim, "Group="):
			hasGroup = true
			if allowNonRoot {
				out = append(out, ln)
			} else {
				out = append(out, "Group=root")
			}
		case strings.HasPrefix(trim, "ProtectHome="):
			hasProtectHome = true
			out = append(out, "ProtectHome=false")
		case strings.HasPrefix(trim, "ProtectSystem="):
			hasProtectSystem = true
			out = append(out, "ProtectSystem=false")
		case strings.HasPrefix(trim, "PrivateTmp="):
			hasPrivateTmp = true
			out = append(out, "PrivateTmp=false")
		case strings.HasPrefix(trim, "NoNewPrivileges="):
			hasNNP = true
			out = append(out, "NoNewPrivileges=false")
		case strings.HasPrefix(trim, "Environment=HOME="):
			hasHOME = true
			if allowNonRoot {
				out = append(out, ln)
			} else {
				out = append(out, "Environment=HOME=/root")
			}
		case strings.HasPrefix(trim, "Environment=USER="):
			hasUSER = true
			if allowNonRoot {
				out = append(out, ln)
			} else {
				out = append(out, "Environment=USER=root")
			}
		case strings.HasPrefix(trim, "Environment=LOGNAME="):
			if allowNonRoot {
				out = append(out, ln)
			} else {
				out = append(out, "Environment=LOGNAME=root")
			}
		case strings.HasPrefix(trim, "Environment=SHELL="):
			hasSHELL = true
			out = append(out, ln)
		default:
			out = append(out, ln)
		}
	}
	if inService {
		out = appendUnlockDirectives(out, hasProtectHome, hasProtectSystem, hasPrivateTmp, hasNNP, hasUser, hasGroup, hasHOME, hasUSER, hasSHELL, allowNonRoot)
	}
	next := strings.Join(out, "\n")
	if !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	return next, next != body
}

func appendUnlockDirectives(out []string, hasProtectHome, hasProtectSystem, hasPrivateTmp, hasNNP, hasUser, hasGroup, hasHOME, hasUSER, hasSHELL, allowNonRoot bool) []string {
	if !hasUser && !allowNonRoot {
		out = append(out, "User=root")
	}
	if !hasGroup && !allowNonRoot {
		out = append(out, "Group=root")
	}
	if !hasSHELL {
		sh := "/bin/bash"
		if _, err := os.Stat(sh); err != nil {
			sh = "/bin/sh"
		}
		out = append(out, "Environment=SHELL="+sh)
	}
	if !hasHOME && !allowNonRoot {
		out = append(out, "Environment=HOME=/root")
	}
	if !hasUSER && !allowNonRoot {
		out = append(out, "Environment=USER=root", "Environment=LOGNAME=root")
	}
	if !hasProtectHome {
		out = append(out, "ProtectHome=false")
	}
	if !hasProtectSystem {
		out = append(out, "ProtectSystem=false")
	}
	if !hasPrivateTmp {
		out = append(out, "PrivateTmp=false")
	}
	if !hasNNP {
		out = append(out, "NoNewPrivileges=false")
	}
	return out
}

func linuxUnitNeedsPrivilegeHeal(body string, allowNonRoot bool) bool {
	checks := []string{
		"ProtectHome=true", "ProtectHome=read-only",
		"ProtectSystem=strict", "ProtectSystem=full", "ProtectSystem=true",
		"PrivateTmp=true", "NoNewPrivileges=true",
		"CapabilityBoundingSet=", "ReadWritePaths=", "ReadOnlyPaths=",
		"InaccessiblePaths=", "TemporaryFileSystem=",
	}
	for _, c := range checks {
		if strings.Contains(body, c) {
			return true
		}
	}
	if !strings.Contains(body, "ProtectHome=false") || !strings.Contains(body, "ProtectSystem=false") {
		return true
	}
	if !allowNonRoot {
		for _, ln := range strings.Split(body, "\n") {
			ln = strings.TrimSpace(ln)
			if strings.HasPrefix(ln, "User=") {
				u := strings.TrimSpace(strings.TrimPrefix(ln, "User="))
				if u != "" && u != "root" && u != "0" {
					return true
				}
			}
		}
	}
	return false
}
