//go:build linux

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Linux desktop: prefer ffmpeg x11grab; fallbacks: import / scrot / gnome-screenshot / grim (Wayland).
// Input via xdotool when available (otherwise view-only).

type linuxCapture struct {
	display      string
	w, h         int
	cropX, cropY int
	monID        int
	wayland      bool
}

func openDeskCapture() (deskCapture, error) {
	display := os.Getenv("DISPLAY")
	wayland := os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland"
	if display == "" && !wayland {
		display = ":0"
	}
	w, h := 1920, 1080
	if display != "" {
		if out, err := exec.Command("xdpyinfo", "-display", display).Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "dimensions:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						parts := strings.Split(fields[1], "x")
						if len(parts) == 2 {
							if ww, e1 := strconv.Atoi(parts[0]); e1 == nil {
								w = ww
							}
							if hh, e2 := strconv.Atoi(parts[1]); e2 == nil {
								h = hh
							}
						}
					}
				}
			}
		}
	}
	c := &linuxCapture{display: display, w: w, h: h, wayland: wayland}
	if _, err := c.Capture(); err != nil {
		hint := fmt.Sprintf("抓屏失败（DISPLAY=%q WAYLAND=%v）: %v", display, wayland, err)
		hint += "；请安装 ffmpeg（X11）或 grim（Wayland），并确保 Agent 运行在图形会话中"
		return nil, fmt.Errorf("%s", hint)
	}
	return c, nil
}

func (c *linuxCapture) Size() (int, int) { return c.w, c.h }
func (c *linuxCapture) Close() error     { return nil }

func (c *linuxCapture) Capture() (image.Image, error) {
	var lastErr error
	try := func(fn func() (image.Image, error)) (image.Image, error) {
		img, err := fn()
		if err != nil {
			lastErr = err
			return nil, err
		}
		b := img.Bounds()
		if b.Dx() > 0 && b.Dy() > 0 {
			c.w, c.h = b.Dx(), b.Dy()
		}
		return img, nil
	}

	if c.wayland {
		if _, err := exec.LookPath("grim"); err == nil {
			if img, err := try(c.captureGrim); err == nil {
				return img, nil
			}
		}
	}

	if c.display != "" {
		if _, err := exec.LookPath("ffmpeg"); err == nil {
			if img, err := try(c.captureFFmpeg); err == nil {
				return img, nil
			}
		}
		if _, err := exec.LookPath("import"); err == nil {
			if img, err := try(c.captureImport); err == nil {
				return img, nil
			}
		}
		if _, err := exec.LookPath("scrot"); err == nil {
			if img, err := try(c.captureScrot); err == nil {
				return img, nil
			}
		}
		if _, err := exec.LookPath("gnome-screenshot"); err == nil {
			if img, err := try(c.captureGnome); err == nil {
				return img, nil
			}
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no graphical capture tool (need ffmpeg/import/scrot/gnome-screenshot, or grim on Wayland)")
}

func (c *linuxCapture) captureFFmpeg() (image.Image, error) {
	grab := c.display
	if c.cropX != 0 || c.cropY != 0 {
		grab = fmt.Sprintf("%s+%d,%d", c.display, c.cropX, c.cropY)
	}
	cmd := exec.Command("ffmpeg", "-loglevel", "error",
		"-f", "x11grab", "-video_size", fmt.Sprintf("%dx%d", c.w, c.h),
		"-i", grab, "-vframes", "1", "-f", "image2pipe", "-vcodec", "mjpeg", "-")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg x11grab: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return jpeg.Decode(bytes.NewReader(out))
}

func (c *linuxCapture) captureImport() (image.Image, error) {
	cmd := exec.Command("import", "-display", c.display, "-window", "root", "png:-")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("import: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return png.Decode(bytes.NewReader(out))
}

func (c *linuxCapture) captureScrot() (image.Image, error) {
	tmp := "/tmp/aiops-desk-scrot.png"
	cmd := exec.Command("scrot", "-o", tmp)
	cmd.Env = append(os.Environ(), "DISPLAY="+c.display)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("scrot: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	raw, err := os.ReadFile(tmp)
	_ = os.Remove(tmp)
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(raw))
}

func (c *linuxCapture) captureGnome() (image.Image, error) {
	tmp := "/tmp/aiops-desk-gnome.png"
	cmd := exec.Command("gnome-screenshot", "-f", tmp)
	cmd.Env = append(os.Environ(), "DISPLAY="+c.display)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("gnome-screenshot: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	raw, err := os.ReadFile(tmp)
	_ = os.Remove(tmp)
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(raw))
}

func (c *linuxCapture) captureGrim() (image.Image, error) {
	cmd := exec.Command("grim", "-")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("grim: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	if img, err := png.Decode(bytes.NewReader(out)); err == nil {
		return img, nil
	}
	return jpeg.Decode(bytes.NewReader(out))
}

type linuxInput struct {
	display string
}

func openDeskInput() (deskInput, error) {
	if _, err := exec.LookPath("xdotool"); err != nil {
		return nil, fmt.Errorf("xdotool not found (required for mouse/keyboard)")
	}
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0"
	}
	return &linuxInput{display: display}, nil
}

func (i *linuxInput) Close() error { return nil }

func (i *linuxInput) env() []string {
	return append(os.Environ(), "DISPLAY="+i.display)
}

func (i *linuxInput) run(args ...string) error {
	cmd := exec.Command("xdotool", args...)
	cmd.Env = i.env()
	return cmd.Run()
}

func (i *linuxInput) MouseMove(x, y int) error {
	return i.run("mousemove", "--sync", strconv.Itoa(x), strconv.Itoa(y))
}

func (i *linuxInput) MouseButton(button int, down bool) error {
	b := button
	if b < 1 {
		b = 1
	}
	if down {
		return i.run("mousedown", strconv.Itoa(b))
	}
	return i.run("mouseup", strconv.Itoa(b))
}

func (i *linuxInput) MouseWheel(delta int) error {
	btn := "4"
	if delta < 0 {
		btn = "5"
	}
	n := delta
	if n < 0 {
		n = -n
	}
	if n == 0 {
		n = 1
	}
	if n > 5 {
		n = 5
	}
	for k := 0; k < n; k++ {
		if err := i.run("click", btn); err != nil {
			return err
		}
	}
	return nil
}

func (i *linuxInput) Key(vk int, down bool) error {
	name := linuxVKName(vk)
	if name == "" {
		return nil
	}
	if down {
		return i.run("keydown", name)
	}
	return i.run("keyup", name)
}

func deskGOOS() string { return "linux" }

func deskH264Usable() bool       { return ffmpegAvailable() }
func deskPreferredCodec() string { return "" } // x11grab JPEG is acceptable
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
	}
	if len(code) == 4 && code[:3] == "Key" {
		return int(code[3])
	}
	if len(code) == 6 && code[:5] == "Digit" {
		return int(code[5])
	}
	if len(key) == 1 {
		r := key[0]
		if r >= 'a' && r <= 'z' {
			return int(r - 32)
		}
		return int(r)
	}
	return 0
}

func linuxVKName(vk int) string {
	switch vk {
	case 0x08:
		return "BackSpace"
	case 0x09:
		return "Tab"
	case 0x0D:
		return "Return"
	case 0x1B:
		return "Escape"
	case 0x20:
		return "space"
	case 0x21:
		return "Page_Up"
	case 0x22:
		return "Page_Down"
	case 0x23:
		return "End"
	case 0x24:
		return "Home"
	case 0x25:
		return "Left"
	case 0x26:
		return "Up"
	case 0x27:
		return "Right"
	case 0x28:
		return "Down"
	case 0x2E:
		return "Delete"
	case 0x10:
		return "Shift_L"
	case 0x11:
		return "Control_L"
	case 0x12:
		return "Alt_L"
	}
	if vk >= 'A' && vk <= 'Z' {
		return string(rune(vk + 32))
	}
	if vk >= '0' && vk <= '9' {
		return string(rune(vk))
	}
	return ""
}
