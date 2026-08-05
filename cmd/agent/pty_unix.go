//go:build linux || darwin

package main

import (
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

// Unix pseudo-terminal (openpty) backing for the remote terminal — a real TTY so
// colours, line editing, job control and full-screen programs (vim/top) work.
// Pure syscall (no cgo, no third-party): open /dev/ptmx, unlock + name the slave
// (per-OS ioctls live in pty_linux.go / pty_darwin.go), then spawn the login
// shell with the slave as its controlling terminal.

type winsize struct {
	rows, cols, xpix, ypix uint16
}

func ioctl(fd, req, arg uintptr) syscall.Errno {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg)
	return e
}

func setWinsize(fd uintptr, cols, rows int) {
	ws := winsize{rows: uint16(rows), cols: uint16(cols)}
	ioctl(fd, ptyWinszReq, uintptr(unsafe.Pointer(&ws)))
}

type unixPTY struct {
	master *os.File
	cmd    *exec.Cmd
}

// newPTY opens a pty pair and starts the shell attached to it. Returns nil on any
// failure so the caller falls back to piped stdio.
func newPTY(cols, rows int) termShell {
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 30
	}
	master, slavePath, err := ptyOpen()
	if err != nil {
		return nil
	}
	slave, err := os.OpenFile(slavePath, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil
	}
	setWinsize(master.Fd(), cols, rows)

	// Build a proper shell environment — systemd/minimal contexts often lack
	// HOME/USER/SHELL, which causes "cd: HOME not set" and broken ~ expansion.
	// shellPath() never returns nologin (service accounts); -l sources profile.
	sh := shellPath()
	env := buildShellEnv()
	dir := interactiveShellDir()
	// Prefer login+interactive; fall back to interactive-only when -l is rejected
	// (some busybox ash builds) so the remote terminal still comes up.
	// On Linux root: wrap with nsenter into PID 1 mount ns so systemd
	// ProtectSystem cannot leave /etc read-only for the interactive shell.
	var cmd *exec.Cmd
	var usedNsenter bool
	for _, shArgs := range [][]string{{"-l", "-i"}, {"-i"}, {}} {
		name, args, viaNs := linuxInteractiveShellInvocation(sh, shArgs, dir)
		c := exec.Command(name, args...)
		c.Stdin, c.Stdout, c.Stderr = slave, slave, slave
		c.Env = env
		// When nsenter --wd= is set, Dir is unused by the child cwd; keep for non-nsenter.
		if !viaNs {
			c.Dir = dir
		}
		c.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
		if err := c.Start(); err != nil {
			slog.Warn("PTY shell 启动失败，尝试降级参数", "err", err, "bin", name, "args", args, "dir", dir, "nsenter", viaNs)
			continue
		}
		cmd = c
		usedNsenter = viaNs
		slog.Info("PTY shell 已启动", "bin", name, "shell", sh, "args", shArgs, "pid", c.Process.Pid, "dir", dir, "nsenter", viaNs)
		break
	}
	// nsenter itself may fail (no --wd support); fall back to a plain shell so the
	// terminal still opens (may remain sandboxed — banner warns when /etc is RO).
	if cmd == nil {
		for _, shArgs := range [][]string{{"-l", "-i"}, {"-i"}, {}} {
			name, args, _ := linuxInteractiveShellInvocationPlain(sh, shArgs)
			c := exec.Command(name, args...)
			c.Stdin, c.Stdout, c.Stderr = slave, slave, slave
			c.Env = env
			c.Dir = dir
			c.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
			if err := c.Start(); err != nil {
				slog.Warn("PTY plain shell 启动失败", "err", err, "bin", name, "args", args)
				continue
			}
			cmd = c
			usedNsenter = false
			slog.Info("PTY shell 已启动（无 nsenter 降级）", "bin", name, "shell", sh, "pid", c.Process.Pid)
			break
		}
	}
	_ = usedNsenter
	if cmd == nil {
		master.Close()
		slave.Close()
		return nil
	}
	slave.Close() // the child owns the slave now; the parent only needs the master
	return &unixPTY{master: master, cmd: cmd}
}

// ensureUTF8 is a no-op on Linux/macOS: the terminal already uses UTF-8 by
// default (or the exec session sets LANG=en_US.UTF-8). No conversion needed.
func ensureUTF8(b []byte) []byte { return b }

// ensureUTF8Hold is a no-op on Unix (already UTF-8); never holds trailing bytes.
func ensureUTF8Hold(data []byte) (out, hold []byte) { return data, nil }

func (u *unixPTY) Read(b []byte) (int, error)  { return u.master.Read(b) }
func (u *unixPTY) Write(b []byte) (int, error) { return u.master.Write(b) }
func (u *unixPTY) Resize(cols, rows int) error { setWinsize(u.master.Fd(), cols, rows); return nil }
func (u *unixPTY) Wait() error                 { return u.cmd.Wait() }
func (u *unixPTY) Close() error {
	if u.cmd.Process != nil {
		_ = u.cmd.Process.Kill()
	}
	return u.master.Close()
}
