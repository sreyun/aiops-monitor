//go:build darwin

package main

import (
	"os"
	"syscall"
	"unsafe"
)

// macOS-specific pty open: grant + unlock the slave, then read its name via
// TIOCPTYGNAME into a 128-byte buffer.
const (
	tiocPTYGNAME = 0x40807453 // TIOCPTYGNAME (128-byte name)
	tiocPTYGRANT = 0x20007454 // TIOCPTYGRANT
	tiocPTYUNLK  = 0x20007452 // TIOCPTYUNLK
	ptyWinszReq  = 0x80087467 // TIOCSWINSZ
)

func ptyOpen() (*os.File, string, error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, "", err
	}
	if e := ioctl(m.Fd(), tiocPTYGRANT, 0); e != 0 {
		m.Close()
		return nil, "", e
	}
	if e := ioctl(m.Fd(), tiocPTYUNLK, 0); e != 0 {
		m.Close()
		return nil, "", e
	}
	var buf [128]byte
	if e := ioctl(m.Fd(), tiocPTYGNAME, uintptr(unsafe.Pointer(&buf[0]))); e != 0 {
		m.Close()
		return nil, "", e
	}
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return m, string(buf[:n]), nil
}
