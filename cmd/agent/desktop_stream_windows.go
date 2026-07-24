//go:build windows

package main

import (
	"fmt"
	"image"
	"syscall"
	"unsafe"
)

// Windows GDI screen capture + SendInput / SetCursorPos keyboard/mouse.

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
)

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
	w, h           int
	monX, monY     int
	monID          int
}

func openDeskCapture() (deskCapture, error) {
	w, _, _ := procGetSystemMetrics.Call(smCXScreen)
	h, _, _ := procGetSystemMetrics.Call(smCYScreen)
	if w == 0 || h == 0 {
		return nil, fmt.Errorf("cannot read screen size")
	}
	c := &winCapture{w: int(w), h: int(h), monID: 1}
	mons := c.Monitors()
	for _, m := range mons {
		if m.Primary {
			_ = c.SetMonitor(m.ID)
			break
		}
	}
	return c, nil
}

func (c *winCapture) Size() (int, int) { return c.w, c.h }
func (c *winCapture) Close() error     { return nil }

func (c *winCapture) Capture() (image.Image, error) {
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

type winInput struct{}

func openDeskInput() (deskInput, error) { return &winInput{}, nil }
func (i *winInput) Close() error        { return nil }

func (i *winInput) MouseMove(x, y int) error {
	_, _, _ = procSetCursorPos.Call(uintptr(x), uintptr(y))
	return nil
}

func (i *winInput) MouseButton(button int, down bool) error {
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
	_, _, _ = procMouseEvent.Call(mouseeventfWheel, 0, 0, uintptr(int32(delta)*120), 0)
	return nil
}

func (i *winInput) Key(vk int, down bool) error {
	var flags uintptr
	if !down {
		flags = keyeventfKeyUp
	}
	_, _, _ = procKeybdEvent.Call(uintptr(vk), 0, flags, 0)
	return nil
}

func deskGOOS() string { return "windows" }

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
