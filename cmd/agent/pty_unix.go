//go:build linux || darwin

package main

import (
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
	cmd := exec.Command(shellPath(), "-l", "-i") // -l: login shell (sources /etc/profile)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.Env = buildShellEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true} // new session, slave becomes the controlling tty
	if err := cmd.Start(); err != nil {
		master.Close()
		slave.Close()
		return nil
	}
	slave.Close() // the child owns the slave now; the parent only needs the master
	return &unixPTY{master: master, cmd: cmd}
}

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
