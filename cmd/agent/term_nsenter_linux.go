//go:build linux

package main

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// linuxInteractiveShellInvocation returns how to spawn an interactive shell that
// escapes the agent service's mount namespace (systemd ProtectSystem/PrivateTmp
// remount /etc and /tmp read-only even for User=root). When possible we nsenter
// into PID 1's namespaces so the remote terminal matches a normal root login.
func linuxInteractiveShellInvocation(sh string, shArgs []string, dir string) (name string, args []string, viaNsenter bool) {
	if os.Geteuid() != 0 {
		return sh, shArgs, false
	}
	nsenter, err := exec.LookPath("nsenter")
	if err != nil {
		return sh, shArgs, false
	}
	if _, err := os.Stat("/proc/1/ns/mnt"); err != nil {
		return sh, shArgs, false
	}
	// Skip when we already share PID 1's mount ns (no sandbox active).
	if sameMountNS(os.Getpid(), 1) && etcWritable() {
		return sh, shArgs, false
	}

	// Always enter host mount/uts/ipc/net ns as root — fixes "root but /etc is
	// read-only" under ProtectSystem=strict/full and PrivateTmp.
	// --wd is applied after setns in util-linux nsenter, so use the *host* path
	// without probing writability in the sandboxed agent namespace.
	args = []string{"-t", "1", "-m", "-u", "-i", "-n"}
	wd := strings.TrimSpace(dir)
	if wd == "" {
		wd = "/root"
	}
	args = append(args, "--wd="+wd)
	args = append(args, "--", sh)
	args = append(args, shArgs...)
	slog.Info("远程终端经 nsenter 进入宿主机挂载命名空间", "shell", sh, "wd", wd)
	return nsenter, args, true
}

func sameMountNS(pidA, pidB int) bool {
	a, err1 := os.Readlink(filepath.Join("/proc", strconv.Itoa(pidA), "ns", "mnt"))
	b, err2 := os.Readlink(filepath.Join("/proc", strconv.Itoa(pidB), "ns", "mnt"))
	return err1 == nil && err2 == nil && a == b
}

// pathWritable reports whether the current process can create/write under path.
// Used to detect systemd ProtectSystem remounting /etc read-only.
func pathWritable(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	if fi.IsDir() {
		f, err := os.CreateTemp(path, ".aiops-wtest-*")
		if err != nil {
			return false
		}
		name := f.Name()
		_ = f.Close()
		_ = os.Remove(name)
		return true
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func etcWritable() bool {
	return pathWritable("/etc")
}

func termPrivilegeDiag() string {
	var b strings.Builder
	b.WriteString("uid=")
	b.WriteString(strconv.Itoa(os.Geteuid()))
	b.WriteString(" euid_ok=")
	if os.Geteuid() == 0 {
		b.WriteString("yes")
	} else {
		b.WriteString("no")
	}
	b.WriteString(" /etc_writable=")
	if etcWritable() {
		b.WriteString("yes")
	} else {
		b.WriteString("no")
	}
	b.WriteString(" same_mnt_as_pid1=")
	if sameMountNS(os.Getpid(), 1) {
		b.WriteString("yes")
	} else {
		b.WriteString("no")
	}
	if home := userHomeDir(); home != "" {
		b.WriteString(" home_writable=")
		if pathWritable(home) {
			b.WriteString("yes")
		} else {
			b.WriteString("no")
		}
	}
	if target, err := os.Readlink("/etc/resolv.conf"); err == nil {
		b.WriteString(" resolv.conf->")
		b.WriteString(filepath.Clean(target))
	}
	return b.String()
}
