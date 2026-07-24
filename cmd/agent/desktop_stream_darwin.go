//go:build darwin

package main

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// macOS: screencapture for frames; osascript / cliclick for input.
// Requires Screen Recording (+ Accessibility for input) in System Settings.

type darwinCapture struct {
	mu       sync.Mutex
	monitor  int // 1-based display id for screencapture -D; 0 = main
	w, h     int
	monitors []deskMonitorInfo
}

func openDeskCapture() (deskCapture, error) {
	if _, err := exec.LookPath("screencapture"); err != nil {
		return nil, fmt.Errorf("screencapture not found — grant Screen Recording to the Agent in System Settings → Privacy & Security")
	}
	c := &darwinCapture{monitor: 0}
	c.refreshMonitors()
	if err := c.CaptureProbe(); err != nil {
		return nil, fmt.Errorf("%v — open System Settings → Privacy & Security → Screen Recording and allow the Agent (or Terminal if running interactively)", err)
	}
	return c, nil
}

func (c *darwinCapture) CaptureProbe() error {
	_, err := c.Capture()
	return err
}

func (c *darwinCapture) refreshMonitors() {
	// system_profiler SPDisplaysDataType is heavy; use screencapture -l listing windows is wrong.
	// Fallback: probe displays 1..8 with a tiny capture, keep those that work.
	c.monitors = nil
	for id := 1; id <= 8; id++ {
		tmp := filepath.Join(os.TempDir(), fmt.Sprintf("aiops-desk-probe-%d.jpg", id))
		cmd := exec.Command("screencapture", "-x", "-D", strconv.Itoa(id), "-t", "jpg", tmp)
		if err := cmd.Run(); err != nil {
			_ = os.Remove(tmp)
			continue
		}
		f, err := os.Open(tmp)
		_ = os.Remove(tmp)
		if err != nil {
			continue
		}
		img, err := jpeg.Decode(f)
		f.Close()
		if err != nil {
			continue
		}
		b := img.Bounds()
		c.monitors = append(c.monitors, deskMonitorInfo{
			ID: id, Name: fmt.Sprintf("Display %d", id),
			Width: b.Dx(), Height: b.Dy(), Primary: id == 1,
		})
	}
	if len(c.monitors) == 0 {
		c.monitors = []deskMonitorInfo{{ID: 1, Name: "Main display", Width: 1920, Height: 1080, Primary: true}}
	}
	if c.monitor == 0 && len(c.monitors) > 0 {
		c.monitor = c.monitors[0].ID
		c.w, c.h = c.monitors[0].Width, c.monitors[0].Height
	}
}

func (c *darwinCapture) Size() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.w > 0 {
		return c.w, c.h
	}
	return 1920, 1080
}

func (c *darwinCapture) Close() error { return nil }

func (c *darwinCapture) Monitors() []deskMonitorInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]deskMonitorInfo, len(c.monitors))
	copy(out, c.monitors)
	return out
}

func (c *darwinCapture) SetMonitor(id int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.monitors {
		if m.ID == id {
			c.monitor = id
			c.w, c.h = m.Width, m.Height
			return nil
		}
	}
	c.monitor = id
	return nil
}

func (c *darwinCapture) Capture() (image.Image, error) {
	c.mu.Lock()
	mon := c.monitor
	c.mu.Unlock()
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("aiops-desk-%d-%d.jpg", os.Getpid(), mon))
	args := []string{"-x", "-t", "jpg"}
	if mon > 0 {
		args = append(args, "-D", strconv.Itoa(mon))
	}
	args = append(args, tmp)
	cmd := exec.Command("screencapture", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(tmp)
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("screencapture failed: %s", msg)
	}
	f, err := os.Open(tmp)
	if err != nil {
		return nil, err
	}
	img, err := jpeg.Decode(f)
	f.Close()
	_ = os.Remove(tmp)
	if err != nil {
		// some macOS versions write PNG despite -t jpg
		f2, err2 := os.Open(tmp)
		if err2 == nil {
			img, err = png.Decode(f2)
			f2.Close()
		}
		if err != nil {
			return nil, err
		}
	}
	b := img.Bounds()
	c.mu.Lock()
	c.w, c.h = b.Dx(), b.Dy()
	c.mu.Unlock()
	return img, nil
}

type darwinInput struct {
	hasCliclick bool
}

func openDeskInput() (deskInput, error) {
	_, err := exec.LookPath("cliclick")
	return &darwinInput{hasCliclick: err == nil}, nil
}

func (i *darwinInput) Close() error { return nil }

func (i *darwinInput) MouseMove(x, y int) error {
	if i.hasCliclick {
		return exec.Command("cliclick", fmt.Sprintf("m:%d,%d", x, y)).Run()
	}
	script := fmt.Sprintf(`tell application "System Events" to set position of mouse to {%d, %d}`, x, y)
	// System Events may not support set position; try cliclick-less AppleScript via CG via python — fallback no-op warn
	return exec.Command("osascript", "-e", script).Run()
}

func (i *darwinInput) MouseButton(button int, down bool) error {
	if i.hasCliclick {
		cmd := "dd"
		if !down {
			cmd = "du"
		}
		if button == 2 {
			if down {
				cmd = "kd:ctrl"
			} else {
				cmd = "ku:ctrl"
			}
			// right click: c:x,y with right — cliclick rc:x,y
		}
		if button == 2 && down {
			return exec.Command("cliclick", "rc:.").Run()
		}
		if button == 2 {
			return nil
		}
		return exec.Command("cliclick", cmd+":.").Run()
	}
	btn := "left"
	if button == 2 {
		btn = "right"
	}
	ev := "mouse down"
	if !down {
		ev = "mouse up"
	}
	script := fmt.Sprintf(`tell application "System Events" to %s %s`, ev, btn)
	return exec.Command("osascript", "-e", script).Run()
}

func (i *darwinInput) MouseWheel(delta int) error {
	if i.hasCliclick {
		// cliclick doesn't scroll well; use osascript
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
	dir := 1
	if delta < 0 {
		dir = -1
	}
	script := fmt.Sprintf(`tell application "System Events" to repeat %d times
scroll %d
end repeat`, n, dir)
	return exec.Command("osascript", "-e", script).Run()
}

func (i *darwinInput) Key(vk int, down bool) error {
	name := darwinVKName(vk)
	if name == "" {
		return nil
	}
	if !down {
		return nil // osascript keystroke is press; key down/up limited
	}
	script := fmt.Sprintf(`tell application "System Events" to keystroke %s`, name)
	if len(name) == 1 {
		script = fmt.Sprintf(`tell application "System Events" to keystroke %q`, name)
	} else {
		script = fmt.Sprintf(`tell application "System Events" to key code %s`, name)
	}
	return exec.Command("osascript", "-e", script).Run()
}

func deskGOOS() string { return "darwin" }

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
	}
	if len(code) == 4 && code[:3] == "Key" {
		return int(code[3])
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

// darwinVKName returns either a single character or a macOS key code number string.
func darwinVKName(vk int) string {
	switch vk {
	case 0x08:
		return "51" // delete (backspace)
	case 0x09:
		return "48"
	case 0x0D:
		return "36"
	case 0x1B:
		return "53"
	case 0x20:
		return " "
	case 0x25:
		return "123"
	case 0x26:
		return "126"
	case 0x27:
		return "124"
	case 0x28:
		return "125"
	case 0x2E:
		return "117"
	}
	if vk >= 'A' && vk <= 'Z' {
		return string(rune(vk + 32))
	}
	if vk >= '0' && vk <= '9' {
		return string(rune(vk))
	}
	return ""
}

func deskClipboardGet() (string, error) {
	out, err := exec.Command("pbpaste").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func deskClipboardSet(text string) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
