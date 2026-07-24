//go:build windows

package main

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"unsafe"
)

// Windows GDI screen capture + SendInput / SetCursorPos keyboard/mouse.
//
// When run as the SYSTEM desktop worker (deskFollowSecureDesktop), the capture
// and input threads follow the *input desktop* via OpenInputDesktop +
// SetThreadDesktop. This is what lets the operator see and control the lock
// screen / logon screen (Winsta0\Winlogon secure desktop) and UAC prompts —
// impossible from an ordinary user-session process.

var (
	modUser32                  = syscall.NewLazyDLL("user32.dll")
	modGdi32                   = syscall.NewLazyDLL("gdi32.dll")
	procGetSystemMetrics       = modUser32.NewProc("GetSystemMetrics")
	procGetDC                  = modUser32.NewProc("GetDC")
	procReleaseDC              = modUser32.NewProc("ReleaseDC")
	procCreateCompatibleDC     = modGdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = modGdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = modGdi32.NewProc("SelectObject")
	procBitBlt                 = modGdi32.NewProc("BitBlt")
	procDeleteObject           = modGdi32.NewProc("DeleteObject")
	procDeleteDC               = modGdi32.NewProc("DeleteDC")
	procGetDIBits              = modGdi32.NewProc("GetDIBits")
	procSendInput              = modUser32.NewProc("SendInput")
	procSetCursorPos           = modUser32.NewProc("SetCursorPos")
	procMouseEvent             = modUser32.NewProc("mouse_event")
	procKeybdEvent             = modUser32.NewProc("keybd_event")

	procOpenInputDesktop          = modUser32.NewProc("OpenInputDesktop")
	procSetThreadDesktop          = modUser32.NewProc("SetThreadDesktop")
	procCloseDesktop              = modUser32.NewProc("CloseDesktop")
	procGetUserObjectInformationW = modUser32.NewProc("GetUserObjectInformationW")
)

const (
	uoiName           = 2          // UOI_NAME
	deskDesiredAccess = 0x10000000 // GENERIC_ALL — capture + input on the desktop
)

// deskFollowSecureDesktop enables input-desktop following (worker mode only, so
// the widely-deployed foreground mode keeps its exact current behaviour).
// deskWorkerMode softens the initial capture probe (the first frame may land
// before we attach to the input desktop).
var (
	deskFollowSecureDesktop bool
	deskWorkerMode          bool
)

func desktopNameOf(h uintptr) string {
	var buf [256]uint16
	var needed uint32
	r, _, _ := procGetUserObjectInformationW.Call(
		h, uintptr(uoiName),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*2),
		uintptr(unsafe.Pointer(&needed)),
	)
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:])
}

// runDesktopWorker is the Windows entry point for the secure-desktop worker
// process (spawned by the service into the active console session). It runs ONLY
// the remote-desktop channel, with capture/input following the input desktop.
func runDesktopWorker(agent *Agent) error {
	deskWorkerMode = true
	deskFollowSecureDesktop = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		cancel()
	}()
	agent.RunDesktopOnly(ctx)
	return nil
}

const (
	smCXScreen     = 0
	smCYScreen     = 1
	srcCopy        = 0x00CC0020
	biRGB          = 0
	dibRGBColors   = 0
	mouseeventfMove = 0x0001
	mouseeventfLeftDown = 0x0002
	mouseeventfLeftUp = 0x0004
	mouseeventfRightDown = 0x0008
	mouseeventfRightUp = 0x0010
	mouseeventfMiddleDown = 0x0020
	mouseeventfMiddleUp = 0x0040
	mouseeventfWheel = 0x0800
	keyeventfKeyUp = 0x0002
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type winCapture struct {
	w, h        int
	monX, monY  int
	monID       int
	curDesk     uintptr // currently attached input desktop (worker mode)
	curDeskName string
	locked      bool // this goroutine's OS thread is locked for SetThreadDesktop
}

// ensureInputDesktop attaches the calling (capture) thread to the desktop that
// currently receives input. It re-attaches whenever the input desktop switches
// (Default ↔ Winlogon ↔ Screen-saver), which is what makes lock/logon screens
// visible. No-op unless running as the SYSTEM worker.
func (c *winCapture) ensureInputDesktop() {
	if !deskFollowSecureDesktop {
		return
	}
	if !c.locked {
		runtime.LockOSThread()
		c.locked = true
	}
	h, _, _ := procOpenInputDesktop.Call(0, 0, uintptr(deskDesiredAccess))
	if h == 0 {
		return // not permitted (not SYSTEM) — keep current desktop
	}
	name := desktopNameOf(h)
	if c.curDesk != 0 && name == c.curDeskName {
		_, _, _ = procCloseDesktop.Call(h)
		return
	}
	if r, _, _ := procSetThreadDesktop.Call(h); r == 0 {
		_, _, _ = procCloseDesktop.Call(h)
		return
	}
	old := c.curDesk
	c.curDesk = h
	c.curDeskName = name
	if old != 0 {
		_, _, _ = procCloseDesktop.Call(old)
	}
	slog.Info("桌面 worker 已附着输入桌面", "desktop", name)
}

func openDeskCapture() (deskCapture, error) {
	w, _, _ := procGetSystemMetrics.Call(smCXScreen)
	h, _, _ := procGetSystemMetrics.Call(smCYScreen)
	if w == 0 || h == 0 {
		return nil, fmt.Errorf("cannot read screen size (no interactive desktop? run Agent in a logged-on user session, not Session 0)")
	}
	c := &winCapture{w: int(w), h: int(h), monID: 1}
	mons := c.Monitors()
	for _, m := range mons {
		if m.Primary {
			_ = c.SetMonitor(m.ID)
			break
		}
	}
	// Probe raw GDI capture (no desktop-follow: that must happen on the capture
	// goroutine). In worker mode the first frame may precede desktop attach, so a
	// probe failure is only a warning; otherwise fail fast with a clear message.
	if _, err := c.captureGDI(); err != nil {
		if deskWorkerMode {
			slog.Warn("初始抓屏失败，进入输入桌面后将重试", "err", err)
		} else {
			return nil, fmt.Errorf("screen capture failed: %w (Agent must run in an interactive user session with an unlocked desktop; 若需锁屏/登录界面请以服务方式安装: aiops-agent --install-service)", err)
		}
	}
	return c, nil
}

func (c *winCapture) Size() (int, int) { return c.w, c.h }
func (c *winCapture) Close() error {
	if c.curDesk != 0 {
		_, _, _ = procCloseDesktop.Call(c.curDesk)
		c.curDesk = 0
	}
	return nil
}

func (c *winCapture) Capture() (image.Image, error) {
	c.ensureInputDesktop()
	return c.captureGDI()
}

func (c *winCapture) captureGDI() (image.Image, error) {
	hwnd := uintptr(0)
	hdc, _, _ := procGetDC.Call(hwnd)
	if hdc == 0 {
		return nil, fmt.Errorf("GetDC failed")
	}
	defer procReleaseDC.Call(hwnd, hdc)

	memDC, _, _ := procCreateCompatibleDC.Call(hdc)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(memDC)

	bmp, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(c.w), uintptr(c.h))
	if bmp == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap failed")
	}
	defer procDeleteObject.Call(bmp)

	old, _, _ := procSelectObject.Call(memDC, bmp)
	defer procSelectObject.Call(memDC, old)

	ret, _, _ := procBitBlt.Call(memDC, 0, 0, uintptr(c.w), uintptr(c.h), hdc, uintptr(c.monX), uintptr(c.monY), srcCopy)
	if ret == 0 {
		return nil, fmt.Errorf("BitBlt failed")
	}

	bi := bitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:       int32(c.w),
		Height:      -int32(c.h),
		Planes:      1,
		BitCount:    32,
		Compression: biRGB,
	}
	buf := make([]byte, c.w*c.h*4)
	n, _, _ := procGetDIBits.Call(hdc, bmp, 0, uintptr(c.h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), dibRGBColors)
	if n == 0 {
		return nil, fmt.Errorf("GetDIBits failed")
	}

	img := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
	for i := 0; i < c.w*c.h; i++ {
		off := i * 4
		img.Pix[off+0] = buf[off+2]
		img.Pix[off+1] = buf[off+1]
		img.Pix[off+2] = buf[off+0]
		img.Pix[off+3] = 255
	}
	_ = procSendInput // keep linked for future absolute mouse
	return img, nil
}

type winInput struct {
	curDesk     uintptr
	curDeskName string
	locked      bool
}

func openDeskInput() (deskInput, error) { return &winInput{}, nil }
func (i *winInput) Close() error {
	if i.curDesk != 0 {
		_, _, _ = procCloseDesktop.Call(i.curDesk)
		i.curDesk = 0
	}
	return nil
}

// ensureInputDesktop attaches the calling (input) thread to the current input
// desktop so SendInput/SetCursorPos reach the lock/logon/secure desktop.
func (i *winInput) ensureInputDesktop() {
	if !deskFollowSecureDesktop {
		return
	}
	if !i.locked {
		runtime.LockOSThread()
		i.locked = true
	}
	h, _, _ := procOpenInputDesktop.Call(0, 0, uintptr(deskDesiredAccess))
	if h == 0 {
		return
	}
	name := desktopNameOf(h)
	if i.curDesk != 0 && name == i.curDeskName {
		_, _, _ = procCloseDesktop.Call(h)
		return
	}
	if r, _, _ := procSetThreadDesktop.Call(h); r == 0 {
		_, _, _ = procCloseDesktop.Call(h)
		return
	}
	old := i.curDesk
	i.curDesk = h
	i.curDeskName = name
	if old != 0 {
		_, _, _ = procCloseDesktop.Call(old)
	}
}

func (i *winInput) MouseMove(x, y int) error {
	i.ensureInputDesktop()
	_, _, _ = procSetCursorPos.Call(uintptr(x), uintptr(y))
	return nil
}

func (i *winInput) MouseButton(button int, down bool) error {
	i.ensureInputDesktop()
	var flags uintptr
	switch button {
	case 2:
		if down {
			flags = mouseeventfRightDown
		} else {
			flags = mouseeventfRightUp
		}
	case 3:
		if down {
			flags = mouseeventfMiddleDown
		} else {
			flags = mouseeventfMiddleUp
		}
	default:
		if down {
			flags = mouseeventfLeftDown
		} else {
			flags = mouseeventfLeftUp
		}
	}
	_, _, _ = procMouseEvent.Call(flags, 0, 0, 0, 0)
	return nil
}

func (i *winInput) MouseWheel(delta int) error {
	i.ensureInputDesktop()
	_, _, _ = procMouseEvent.Call(mouseeventfWheel, 0, 0, uintptr(int32(delta)*120), 0)
	return nil
}

func (i *winInput) Key(vk int, down bool) error {
	i.ensureInputDesktop()
	var flags uintptr
	if !down {
		flags = keyeventfKeyUp
	}
	_, _, _ = procKeybdEvent.Call(uintptr(vk), 0, flags, 0)
	return nil
}

func deskGOOS() string { return "windows" }

func deskH264Usable() bool       { return ffmpegAvailable() }
func deskPreferredCodec() string { return "" } // GDI JPEG is fast on Windows
func deskAVFScreenIndex() int    { return -1 }

func deskKeyToVK(key, code string) int {
	switch code {
	case "Backspace":
		return 0x08
	case "Tab":
		return 0x09
	case "Enter":
		return 0x0D
	case "Escape":
		return 0x1B
	case "Space":
		return 0x20
	case "PageUp":
		return 0x21
	case "PageDown":
		return 0x22
	case "End":
		return 0x23
	case "Home":
		return 0x24
	case "ArrowLeft":
		return 0x25
	case "ArrowUp":
		return 0x26
	case "ArrowRight":
		return 0x27
	case "ArrowDown":
		return 0x28
	case "Delete":
		return 0x2E
	case "ShiftLeft", "ShiftRight":
		return 0x10
	case "ControlLeft", "ControlRight":
		return 0x11
	case "AltLeft", "AltRight":
		return 0x12
	case "MetaLeft", "MetaRight":
		return 0x5B
	}
	if len(code) == 4 && code[:3] == "Key" {
		c := code[3]
		if c >= 'A' && c <= 'Z' {
			return int(c)
		}
	}
	if len(code) == 6 && code[:5] == "Digit" {
		return int(code[5])
	}
	if len(key) == 1 {
		r := key[0]
		if r >= 'a' && r <= 'z' {
			return int(r - 32)
		}
		if r >= 'A' && r <= 'Z' {
			return int(r)
		}
		if r >= '0' && r <= '9' {
			return int(r)
		}
	}
	return 0
}
