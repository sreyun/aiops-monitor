//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

var (
	procEnumDisplayMonitors = modUser32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW     = modUser32.NewProc("GetMonitorInfoW")
)

type rectWin struct {
	Left, Top, Right, Bottom int32
}

type monitorInfoW struct {
	CbSize    uint32
	RcMonitor rectWin
	RcWork    rectWin
	DwFlags   uint32
}

const monitorinfoPrimary = 1

func (c *winCapture) Monitors() []deskMonitorInfo {
	var list []deskMonitorInfo
	cb := syscall.NewCallback(func(hMonitor, hdcMonitor, lprcMonitor uintptr, dwData uintptr) uintptr {
		var mi monitorInfoW
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		r, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
		if r == 0 {
			return 1
		}
		id := len(list) + 1
		w := int(mi.RcMonitor.Right - mi.RcMonitor.Left)
		h := int(mi.RcMonitor.Bottom - mi.RcMonitor.Top)
		list = append(list, deskMonitorInfo{
			ID: id, Name: fmt.Sprintf("Display %d", id),
			Width: w, Height: h, Primary: mi.DwFlags&monitorinfoPrimary != 0,
			X: int(mi.RcMonitor.Left), Y: int(mi.RcMonitor.Top),
		})
		return 1
	})
	procEnumDisplayMonitors.Call(0, 0, cb, 0)
	if len(list) == 0 {
		w, h := c.Size()
		list = []deskMonitorInfo{{ID: 1, Name: "Primary", Width: w, Height: h, Primary: true}}
	}
	return list
}

func (c *winCapture) SetMonitor(id int) error {
	mons := c.Monitors()
	for _, m := range mons {
		if m.ID == id {
			c.monX, c.monY = m.X, m.Y
			c.w, c.h = m.Width, m.Height
			c.monID = id
			return nil
		}
	}
	return fmt.Errorf("monitor %d not found", id)
}

func (c *winCapture) Origin() (int, int) { return c.monX, c.monY }

func deskClipboardSupported() bool { return true }

func deskClipboardGet() (string, error) {
	out, err := exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard -Raw").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

func deskClipboardSet(text string) error {
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "Set-Clipboard -Value $input")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
