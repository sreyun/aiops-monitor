package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// serviceLogMaxBytes caps a single log file before it is rotated to ".1".
// Two generations of 4 MiB is enough to cover a crash loop's worth of history
// without ever needing an operator to clean up the install directory.
const serviceLogMaxBytes = 4 << 20

// startServiceFileLog mirrors slog output into <dir>/<name>.
//
// A Windows service and the desktop worker it spawns have no console: the SCM
// discards stderr entirely. That turned every startup failure — unreachable
// server, rejected token, invalid config — into "the installer said done and
// nothing showed up in the dashboard", with nothing to look at on the machine.
// systemd and launchd already capture stderr, so this only ever adds a file.
func startServiceFileLog(dir, name string) {
	if dir == "" {
		return
	}
	w := newRotatingFile(filepath.Join(dir, name), serviceLogMaxBytes)
	if w == nil {
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, w), &slog.HandlerOptions{Level: slog.LevelInfo})))
}

// rotatingFile is a minimal size-capped writer. Log volume here is a handful of
// lines per report cycle, so a mutex around a plain file is plenty and avoids
// pulling in a logging dependency for what is purely a diagnostics aid.
type rotatingFile struct {
	mu   sync.Mutex
	path string
	max  int64
	f    *os.File
	n    int64
}

func newRotatingFile(path string, max int64) *rotatingFile {
	r := &rotatingFile{path: path, max: max}
	if err := r.open(); err != nil {
		return nil
	}
	return r
}

func (r *rotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	size := int64(0)
	if st, err := f.Stat(); err == nil {
		size = st.Size()
	}
	// slog writes UTF-8. Notepad on Chinese Windows Server assumes the ANSI code
	// page (GBK) for a BOM-less file and renders every log line as mojibake —
	// unhelpful when this file exists precisely to be read during an incident.
	if size == 0 {
		if n, err := f.Write([]byte{0xEF, 0xBB, 0xBF}); err == nil {
			size = int64(n)
		}
	}
	r.f, r.n = f, size
	return nil
}

func (r *rotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return len(p), nil // degraded: never fail a log write into a service crash
	}
	if r.n+int64(len(p)) > r.max {
		r.rotateLocked()
	}
	n, _ := r.f.Write(p)
	r.n += int64(n)
	// Never surface an error: this writer is half of an io.MultiWriter, and
	// failing here (disk full, ACL change) would also stop the stderr copy that
	// systemd/launchd rely on. Losing a diagnostics line beats losing logging.
	return len(p), nil
}

func (r *rotatingFile) rotateLocked() {
	_ = r.f.Close()
	r.f = nil
	_ = os.Remove(r.path + ".1")
	_ = os.Rename(r.path, r.path+".1")
	if err := r.open(); err != nil {
		r.f = nil
	}
}
