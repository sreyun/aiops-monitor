package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
)

// Optional H.264 path via ffmpeg (libx264 fragmented MP4). Falls back to JPEG when unavailable.

type h264Pipe struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	once   sync.Once
}

func ffmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

func startH264Pipe(mon deskMonitorInfo, scale float64, fps int) (*h264Pipe, error) {
	if !ffmpegAvailable() {
		return nil, fmt.Errorf("ffmpeg not found")
	}
	if fps < 1 {
		fps = 8
	}
	if fps > 20 {
		fps = 20
	}
	if scale <= 0 || scale > 1 {
		scale = 0.5
	}
	w := int(float64(mon.Width) * scale)
	h := int(float64(mon.Height) * scale)
	if w%2 != 0 {
		w--
	}
	if h%2 != 0 {
		h--
	}
	if w < 16 {
		w = 16
	}
	if h < 16 {
		h = 16
	}
	vf := fmt.Sprintf("scale=%d:%d", w, h)
	args := []string{"-loglevel", "error", "-f"}
	switch runtime.GOOS {
	case "windows":
		args = append(args, "gdigrab", "-framerate", strconv.Itoa(fps), "-offset_x", strconv.Itoa(mon.X), "-offset_y", strconv.Itoa(mon.Y),
			"-video_size", fmt.Sprintf("%dx%d", mon.Width, mon.Height), "-i", "desktop")
	case "darwin":
		// avfoundation screen device index MUST be resolved — device 0 is usually
		// the FaceTime camera. base = index of "Capture screen 0"; add (ID-1) for
		// additional displays.
		base := deskAVFScreenIndex()
		if base < 0 {
			return nil, fmt.Errorf("avfoundation screen-capture device not found")
		}
		idx := base
		if mon.ID > 1 {
			idx = base + (mon.ID - 1)
		}
		args = append(args, "avfoundation", "-capture_cursor", "1", "-framerate", strconv.Itoa(fps), "-i", fmt.Sprintf("%d:none", idx))
	default:
		disp := os.Getenv("DISPLAY")
		if disp == "" {
			disp = ":0"
		}
		grab := fmt.Sprintf("%s+%d,%d", disp, mon.X, mon.Y)
		args = append(args, "x11grab", "-framerate", strconv.Itoa(fps), "-video_size", fmt.Sprintf("%dx%d", mon.Width, mon.Height), "-i", grab)
	}
	args = append(args,
		"-vf", vf,
		"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
		"-pix_fmt", "yuv420p", "-g", strconv.Itoa(fps),
		"-f", "mp4", "-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"pipe:1",
	)
	cmd := exec.Command("ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &h264Pipe{cmd: cmd, stdout: stdout}, nil
}

func (p *h264Pipe) Read(b []byte) (int, error) {
	return p.stdout.Read(b)
}

func (p *h264Pipe) Close() error {
	var err error
	p.once.Do(func() {
		if p.stdout != nil {
			_ = p.stdout.Close()
		}
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
			err = p.cmd.Wait()
		}
	})
	return err
}
