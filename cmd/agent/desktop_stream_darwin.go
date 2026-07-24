//go:build darwin

package main

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log/slog"
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

// darwinInput drives the mouse via cliclick (osascript cannot move the cursor to
// absolute coordinates reliably) and the keyboard via osascript System Events
// (reliable, no external dependency, and honours modifier chords). Modifier
// state is tracked in Go because the browser sends discrete key up/down events
// while cliclick/osascript invocations are stateless one-shots.
type darwinInput struct {
	mu           sync.Mutex
	hasCliclick  bool
	lastX, lastY int
	mods         map[string]bool // command / shift / control / option
	warnedMouse  bool
}

func openDeskInput() (deskInput, error) {
	_, err := exec.LookPath("cliclick")
	di := &darwinInput{hasCliclick: err == nil, mods: map[string]bool{}}
	if err != nil {
		slog.Warn("macOS 未找到 cliclick，远程鼠标控制不可用（键盘仍可用）；建议安装: brew install cliclick")
	}
	return di, nil
}

func (i *darwinInput) Close() error { return nil }

func (i *darwinInput) MouseMove(x, y int) error {
	i.mu.Lock()
	i.lastX, i.lastY = x, y
	hc := i.hasCliclick
	i.mu.Unlock()
	if hc {
		return exec.Command("cliclick", fmt.Sprintf("m:%d,%d", x, y)).Run()
	}
	return nil
}

func (i *darwinInput) MouseButton(button int, down bool) error {
	i.mu.Lock()
	x, y := i.lastX, i.lastY
	hc := i.hasCliclick
	warned := i.warnedMouse
	i.warnedMouse = true
	i.mu.Unlock()
	if !hc {
		if !warned {
			slog.Warn("忽略鼠标点击：未安装 cliclick（brew install cliclick）")
		}
		return nil
	}
	switch button {
	case 2: // right — cliclick lacks down/up; emulate a click on release
		if !down {
			return exec.Command("cliclick", fmt.Sprintf("rc:%d,%d", x, y)).Run()
		}
		return nil
	case 3: // middle — unsupported by cliclick
		return nil
	default: // left: honour explicit down/up so drag works
		verb := "dd"
		if !down {
			verb = "du"
		}
		return exec.Command("cliclick", fmt.Sprintf("%s:%d,%d", verb, x, y)).Run()
	}
}

func (i *darwinInput) MouseWheel(delta int) error {
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

func (i *darwinInput) setMod(name string, down bool) error {
	i.mu.Lock()
	i.mods[name] = down
	i.mu.Unlock()
	return nil
}

// usingClause renders the currently-held modifiers as an AppleScript
// `using {command down, ...}` suffix (empty when none held).
func (i *darwinInput) usingClause() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	var parts []string
	for _, m := range []string{"command", "control", "option", "shift"} {
		if i.mods[m] {
			parts = append(parts, m+" down")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return " using {" + strings.Join(parts, ", ") + "}"
}

func (i *darwinInput) Key(vk int, down bool) error {
	// Modifiers only update state; the chord is applied when the base key fires.
	switch vk {
	case 0x10:
		return i.setMod("shift", down)
	case 0x11:
		return i.setMod("control", down)
	case 0x12:
		return i.setMod("option", down)
	case 0x5B, 0x5C:
		return i.setMod("command", down)
	}
	if !down {
		return nil // keystroke/key code is an atomic press; fire on key-down
	}
	name := darwinVKName(vk)
	if name == "" {
		return nil
	}
	using := i.usingClause()
	var script string
	if len(name) == 1 || name == " " {
		script = fmt.Sprintf(`tell application "System Events" to keystroke %q%s`, name, using)
	} else {
		script = fmt.Sprintf(`tell application "System Events" to key code %s%s`, name, using)
	}
	return exec.Command("osascript", "-e", script).Run()
}

func deskGOOS() string { return "darwin" }

var (
	darwinScrIdxOnce sync.Once
	darwinScrIdx     = -1
)

// darwinScreenCaptureIndex resolves the avfoundation device index of
// "Capture screen 0". This MUST be detected — on most Macs device 0 is the
// FaceTime camera, so a naive index would stream the webcam instead of the
// screen. Returns -1 when ffmpeg is missing or no screen device is found.
func darwinScreenCaptureIndex() int {
	darwinScrIdxOnce.Do(func() {
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			return
		}
		// list_devices prints to stderr; exits non-zero by design.
		out, _ := exec.Command("ffmpeg", "-hide_banner", "-f", "avfoundation",
			"-list_devices", "true", "-i", "").CombinedOutput()
		inVideo := false
		for _, ln := range strings.Split(string(out), "\n") {
			low := strings.ToLower(ln)
			if strings.Contains(low, "avfoundation video devices") {
				inVideo = true
				continue
			}
			if strings.Contains(low, "avfoundation audio devices") {
				inVideo = false
				continue
			}
			if !inVideo {
				continue
			}
			// e.g. "[AVFoundation ...] [1] Capture screen 0"
			if strings.Contains(low, "capture screen 0") {
				if l := strings.Index(ln, "["); l >= 0 {
					// find the last "[N]" token before the name
					rest := ln
					for {
						a := strings.Index(rest, "[")
						b := strings.Index(rest, "]")
						if a < 0 || b < a {
							break
						}
						tok := strings.TrimSpace(rest[a+1 : b])
						if n, err := strconv.Atoi(tok); err == nil {
							darwinScrIdx = n
						}
						rest = rest[b+1:]
					}
				}
			}
		}
	})
	return darwinScrIdx
}

func deskAVFScreenIndex() int { return darwinScreenCaptureIndex() }

func deskH264Usable() bool { return darwinScreenCaptureIndex() >= 0 }

func deskPreferredCodec() string {
	if deskH264Usable() {
		return "h264"
	}
	return ""
}

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
