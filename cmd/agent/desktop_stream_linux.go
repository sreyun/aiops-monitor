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

// Linux desktop: prefer ffmpeg x11grab; input via xdotool when available.

type linuxCapture struct {
	display        string
	w, h           int
	cropX, cropY   int
	monID          int
}

func openDeskCapture() (deskCapture, error) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0"
	}
	w, h := 1920, 1080
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
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		if _, err2 := exec.LookPath("import"); err2 != nil {
			return nil, fmt.Errorf("no graphical capture tool (need ffmpeg or ImageMagick import) and DISPLAY=%s", display)
		}
	}
	return &linuxCapture{display: display, w: w, h: h}, nil
}

func (c *linuxCapture) Size() (int, int) { return c.w, c.h }
func (c *linuxCapture) Close() error     { return nil }

func (c *linuxCapture) Capture() (image.Image, error) {
	grab := c.display
	if c.cropX != 0 || c.cropY != 0 {
		grab = fmt.Sprintf("%s+%d,%d", c.display, c.cropX, c.cropY)
	}
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		cmd := exec.Command("ffmpeg", "-loglevel", "error",
			"-f", "x11grab", "-video_size", fmt.Sprintf("%dx%d", c.w, c.h),
			"-i", grab, "-vframes", "1", "-f", "image2pipe", "-vcodec", "mjpeg", "-")
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("ffmpeg x11grab: %w", err)
		}
		img, err := jpeg.Decode(bytes.NewReader(out))
		if err != nil {
			return nil, err
		}
		b := img.Bounds()
		c.w, c.h = b.Dx(), b.Dy()
		return img, nil
	}
	cmd := exec.Command("import", "-display", c.display, "-window", "root", "png:-")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("import screenshot: %w", err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	c.w, c.h = b.Dx(), b.Dy()
	return img, nil
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

func deskKeyToVK(key, code string) int {
	// Reuse Windows VK numbers as a portable mapping table for linuxVKName.
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
		return string(rune(vk + 32)) // xdotool prefers lowercase
	}
	if vk >= '0' && vk <= '9' {
		return string(rune(vk))
	}
	return ""
}
