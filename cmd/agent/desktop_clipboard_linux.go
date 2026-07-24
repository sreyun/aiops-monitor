//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func (c *linuxCapture) Monitors() []deskMonitorInfo {
	// xrandr --listmonitors → " 0: +*DP-1 1920/508x1080/286+0+0  DP-1"
	out, err := exec.Command("xrandr", "--listmonitors").Output()
	if err != nil {
		w, h := c.Size()
		return []deskMonitorInfo{{ID: 1, Name: "default", Width: w, Height: h, Primary: true}}
	}
	var list []deskMonitorInfo
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Monitors:") {
			continue
		}
		// "0: +*HDMI-1 1920/..." 
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		idStr := strings.TrimSuffix(parts[0], ":")
		id, _ := strconv.Atoi(idStr)
		id++ // 1-based
		geom := parts[2]
		primary := strings.Contains(parts[1], "*")
		name := parts[len(parts)-1]
		w, h, x, y := parseXrandrGeom(geom)
		if w == 0 {
			continue
		}
		list = append(list, deskMonitorInfo{ID: id, Name: name, Width: w, Height: h, X: x, Y: y, Primary: primary})
	}
	if len(list) == 0 {
		w, h := c.Size()
		return []deskMonitorInfo{{ID: 1, Name: "default", Width: w, Height: h, Primary: true}}
	}
	return list
}

func parseXrandrGeom(s string) (w, h, x, y int) {
	// 1920/508x1080/286+0+0
	main := strings.Split(s, "+")
	wh := strings.Split(main[0], "x")
	if len(wh) < 2 {
		return
	}
	wPart := strings.Split(wh[0], "/")[0]
	hPart := strings.Split(wh[1], "/")[0]
	w, _ = strconv.Atoi(wPart)
	h, _ = strconv.Atoi(hPart)
	if len(main) >= 3 {
		x, _ = strconv.Atoi(main[1])
		y, _ = strconv.Atoi(main[2])
	}
	return
}

func (c *linuxCapture) SetMonitor(id int) error {
	for _, m := range c.Monitors() {
		if m.ID == id {
			c.cropX, c.cropY = m.X, m.Y
			c.w, c.h = m.Width, m.Height
			c.monID = id
			c.outputName = m.Name
			return nil
		}
	}
	return fmt.Errorf("monitor %d not found", id)
}

func (c *linuxCapture) Origin() (x, y int) { return c.cropX, c.cropY }

func linuxWaylandSession() bool {
	return os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland"
}

func deskClipboardSupported() bool {
	if linuxWaylandSession() {
		if _, err := exec.LookPath("wl-paste"); err == nil {
			return true
		}
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return true
		}
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		return true
	}
	if _, err := exec.LookPath("xsel"); err == nil {
		return true
	}
	return false
}

func deskClipboardGet() (string, error) {
	if linuxWaylandSession() {
		if _, err := exec.LookPath("wl-paste"); err == nil {
			out, err := exec.Command("wl-paste", "-n").Output()
			return string(out), err
		}
	}
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0"
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command("xclip", "-selection", "clipboard", "-o")
		cmd.Env = append(os.Environ(), "DISPLAY="+display)
		out, err := cmd.Output()
		return string(out), err
	}
	if _, err := exec.LookPath("xsel"); err == nil {
		cmd := exec.Command("xsel", "--clipboard", "--output")
		cmd.Env = append(os.Environ(), "DISPLAY="+display)
		out, err := cmd.Output()
		return string(out), err
	}
	return "", fmt.Errorf("need wl-clipboard (Wayland) or xclip/xsel (X11)")
}

func deskClipboardSet(text string) error {
	if linuxWaylandSession() {
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd := exec.Command("wl-copy")
			cmd.Stdin = strings.NewReader(text)
			return cmd.Run()
		}
	}
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0"
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command("xclip", "-selection", "clipboard")
		cmd.Env = append(os.Environ(), "DISPLAY="+display)
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	if _, err := exec.LookPath("xsel"); err == nil {
		cmd := exec.Command("xsel", "--clipboard", "--input")
		cmd.Env = append(os.Environ(), "DISPLAY="+display)
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	return fmt.Errorf("need wl-clipboard (Wayland) or xclip/xsel (X11)")
}
