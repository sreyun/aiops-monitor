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
	monX, monY int
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
	// Probe displays 1..8 with a tiny capture, keep those that work.
	c.monitors = nil
	origins := darwinDisplayOrigins()
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
		ox, oy := 0, 0
		// Match CGDisplay bounds by size (screencapture -D order ≈ active display list).
		idx := len(c.monitors)
		if idx < len(origins) {
			ox, oy = origins[idx].x, origins[idx].y
		}
		c.monitors = append(c.monitors, deskMonitorInfo{
			ID: id, Name: fmt.Sprintf("Display %d", id),
			Width: b.Dx(), Height: b.Dy(), X: ox, Y: oy, Primary: id == 1,
		})
	}
	if len(c.monitors) == 0 {
		c.monitors = []deskMonitorInfo{{ID: 1, Name: "Main display", Width: 1920, Height: 1080, Primary: true}}
	}
	if c.monitor == 0 && len(c.monitors) > 0 {
		c.monitor = c.monitors[0].ID
		c.w, c.h = c.monitors[0].Width, c.monitors[0].Height
		c.monX, c.monY = c.monitors[0].X, c.monitors[0].Y
	}
}

type cgOrigin struct{ x, y, w, h int }

func darwinDisplayOrigins() []cgOrigin {
	py, err := exec.LookPath("python3")
	if err != nil {
		return nil
	}
	script := `
from Quartz import CGGetActiveDisplayList, CGDisplayBounds, CGMainDisplayID
max_n = 16
err, ids, count = CGGetActiveDisplayList(max_n, None, None)
if err != 0:
    raise SystemExit(0)
main = CGMainDisplayID()
# Primary first, then remaining in system order.
ordered = [d for d in ids[:count] if d == main] + [d for d in ids[:count] if d != main]
for d in ordered:
    b = CGDisplayBounds(d)
    print("%d,%d,%d,%d" % (int(b.origin.x), int(b.origin.y), int(b.size.width), int(b.size.height)))
`
	out, err := exec.Command(py, "-c", script).Output()
	if err != nil {
		return nil
	}
	var list []cgOrigin
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.Split(strings.TrimSpace(line), ",")
		if len(parts) != 4 {
			continue
		}
		x, _ := strconv.Atoi(parts[0])
		y, _ := strconv.Atoi(parts[1])
		w, _ := strconv.Atoi(parts[2])
		h, _ := strconv.Atoi(parts[3])
		list = append(list, cgOrigin{x, y, w, h})
	}
	return list
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
			c.monX, c.monY = m.X, m.Y
			return nil
		}
	}
	c.monitor = id
	return nil
}

func (c *darwinCapture) Origin() (x, y int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.monX, c.monY
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
	monX, monY   int
	mods         map[string]bool // command / shift / control / option
	warnedMouse  bool
}

func openDeskInput() (deskInput, error) {
	_, err := exec.LookPath("cliclick")
	di := &darwinInput{hasCliclick: err == nil, mods: map[string]bool{}}
	if err != nil {
		// Quartz via python3 is a no-extra-deps fallback on most Macs.
		if quartzMouseAvailable() {
			slog.Info("macOS 未找到 cliclick，将使用 Python/Quartz 注入鼠标（仍建议: brew install cliclick）")
		} else {
			slog.Warn("macOS 未找到 cliclick 且 Quartz 不可用，远程鼠标控制受限（键盘仍可用）；建议: brew install cliclick")
		}
	}
	return di, nil
}

func (i *darwinInput) Close() error { return nil }

func (i *darwinInput) SetOrigin(x, y int) {
	i.mu.Lock()
	i.monX, i.monY = x, y
	i.mu.Unlock()
}

func quartzMouseAvailable() bool {
	py, err := exec.LookPath("python3")
	if err != nil {
		return false
	}
	cmd := exec.Command(py, "-c", "import Quartz")
	return cmd.Run() == nil
}

func quartzMouse(action string, x, y, button int) error {
	py, err := exec.LookPath("python3")
	if err != nil {
		return err
	}
	// action: move | down | up | click
	script := `
import sys
from Quartz.CoreGraphics import (
    CGEventCreateMouseEvent, CGEventPost, CGEventCreateScrollWheelEvent,
    kCGEventMouseMoved, kCGEventLeftMouseDown, kCGEventLeftMouseUp,
    kCGEventRightMouseDown, kCGEventRightMouseUp,
    kCGEventOtherMouseDown, kCGEventOtherMouseUp,
    kCGMouseButtonLeft, kCGMouseButtonRight, kCGMouseButtonCenter,
    kCGHIDEventTap, kCGScrollEventUnitLine,
)
action, x, y, button = sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), int(sys.argv[4])
btn = kCGMouseButtonLeft
down_t, up_t = kCGEventLeftMouseDown, kCGEventLeftMouseUp
if button == 2:
    btn = kCGMouseButtonRight
    down_t, up_t = kCGEventRightMouseDown, kCGEventRightMouseUp
elif button == 3:
    btn = kCGMouseButtonCenter
    down_t, up_t = kCGEventOtherMouseDown, kCGEventOtherMouseUp
if action == "move":
    e = CGEventCreateMouseEvent(None, kCGEventMouseMoved, (x, y), btn)
    CGEventPost(kCGHIDEventTap, e)
elif action == "down":
    e = CGEventCreateMouseEvent(None, down_t, (x, y), btn)
    CGEventPost(kCGHIDEventTap, e)
elif action == "up":
    e = CGEventCreateMouseEvent(None, up_t, (x, y), btn)
    CGEventPost(kCGHIDEventTap, e)
elif action == "wheel":
    e = CGEventCreateScrollWheelEvent(None, kCGScrollEventUnitLine, 1, y)
    CGEventPost(kCGHIDEventTap, e)
`
	return exec.Command(py, "-c", script, action, fmt.Sprintf("%d", x), fmt.Sprintf("%d", y), fmt.Sprintf("%d", button)).Run()
}

func (i *darwinInput) MouseMove(x, y int) error {
	i.mu.Lock()
	ax, ay := i.monX+x, i.monY+y
	i.lastX, i.lastY = ax, ay
	hc := i.hasCliclick
	i.mu.Unlock()
	if hc {
		return exec.Command("cliclick", fmt.Sprintf("m:%d,%d", ax, ay)).Run()
	}
	return quartzMouse("move", ax, ay, 1)
}

func (i *darwinInput) MouseButton(button int, down bool) error {
	i.mu.Lock()
	x, y := i.lastX, i.lastY
	hc := i.hasCliclick
	warned := i.warnedMouse
	i.warnedMouse = true
	i.mu.Unlock()
	if hc {
		switch button {
		case 2:
			if !down {
				return exec.Command("cliclick", fmt.Sprintf("rc:%d,%d", x, y)).Run()
			}
			return nil
		case 3:
			// cliclick has no middle-click; use Quartz when available.
			return quartzMouse(map[bool]string{true: "down", false: "up"}[down], x, y, 3)
		default:
			verb := "dd"
			if !down {
				verb = "du"
			}
			return exec.Command("cliclick", fmt.Sprintf("%s:%d,%d", verb, x, y)).Run()
		}
	}
	if err := quartzMouse(map[bool]string{true: "down", false: "up"}[down], x, y, button); err != nil {
		if !warned {
			slog.Warn("忽略鼠标点击：未安装 cliclick 且 Quartz 注入失败（需辅助功能权限）", "err", err)
		}
		return err
	}
	return nil
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
	if err := quartzMouse("wheel", 0, dir*n, 0); err == nil {
		return nil
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
	case 0x10, 0xA0, 0xA1:
		return i.setMod("shift", down)
	case 0x11, 0xA2, 0xA3:
		return i.setMod("control", down)
	case 0x12, 0xA4, 0xA5:
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
	case 0x21:
		return "116" // page up
	case 0x22:
		return "121" // page down
	case 0x23:
		return "119" // end
	case 0x24:
		return "115" // home
	case 0x25:
		return "123"
	case 0x26:
		return "126"
	case 0x27:
		return "124"
	case 0x28:
		return "125"
	case 0x2D:
		return "114" // help/insert
	case 0x2E:
		return "117"
	case 0x70:
		return "122" // F1
	case 0x71:
		return "120"
	case 0x72:
		return "99"
	case 0x73:
		return "118"
	case 0x74:
		return "96"
	case 0x75:
		return "97"
	case 0x76:
		return "98"
	case 0x77:
		return "100"
	case 0x78:
		return "101"
	case 0x79:
		return "109"
	case 0x7A:
		return "103"
	case 0x7B:
		return "111"
	case 0xBD:
		return "-"
	case 0xBB:
		return "="
	case 0xDB:
		return "["
	case 0xDD:
		return "]"
	case 0xDC:
		return "\\"
	case 0xBA:
		return ";"
	case 0xDE:
		return "'"
	case 0xC0:
		return "`"
	case 0xBC:
		return ","
	case 0xBE:
		return "."
	case 0xBF:
		return "/"
	}
	if vk >= 'A' && vk <= 'Z' {
		return string(rune(vk + 32))
	}
	if vk >= '0' && vk <= '9' {
		return string(rune(vk))
	}
	return ""
}

func deskClipboardSupported() bool {
	_, err := exec.LookPath("pbpaste")
	return err == nil
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
