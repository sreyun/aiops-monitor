package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Remote terminal — agent side.
//
// The agent has no inbound ports, so it dials out: a persistent long-poll asks
// the server whether an operator opened a terminal; when one is, the agent opens
// two plain-HTTP streams — rx (framed keystrokes/resize down) and tx (shell
// output up) — and bridges them to a locally-spawned shell. All terminal
// requests carry the install token.
//
// The shell is a real pseudo-terminal where available (Windows ConPTY, Linux/
// macOS openpty) so interactive TTY features work; it falls back to piped stdio
// on platforms without a native PTY.

// termShell is a spawned interactive shell — a PTY master or a piped fallback.
type termShell interface {
	io.Reader                      // shell output
	Write(p []byte) (int, error)   // keystrokes to the shell
	Resize(cols, rows int) error   // window size (no-op for piped fallback)
	Wait() error                   // block until the shell exits
	Close() error                  // terminate + release
}

// newPTY is provided per-platform; it returns nil when no native PTY is
// available, so startShell falls back to piped stdio.
// (see pty_windows.go / pty_linux.go / pty_darwin.go / pty_other.go)

var (
	termHTTP = &http.Client{} // no timeout — rx/tx streams are long-lived
	// termWaitHTTP bounds the long-poll wait so a half-open network can't wedge
	// the poller forever (which would silently kill the terminal channel while
	// metrics keep reporting). Slightly above the server's 25s poll timeout.
	termWaitHTTP = &http.Client{Timeout: 35 * time.Second}
)

func (a *Agent) runTerminalChannel() {
	if a.identity.Token == "" {
		log.Printf("远程终端通道未启用：未提供安装 Token（--token）")
		return
	}
	log.Printf("远程终端通道已就绪，等待服务端呼叫…")
	for {
		sid, ok := a.termWait()
		if !ok {
			time.Sleep(3 * time.Second)
			continue
		}
		if sid == "" {
			continue // long-poll timeout, re-poll immediately
		}
		go a.runTerminalSession(sid)
	}
}

func (a *Agent) termWait() (sessionID string, ok bool) {
	q := url.Values{"host": {a.identity.HostID}, "token": {a.identity.Token}}
	resp, err := termWaitHTTP.Get(a.server + "/api/v1/agent/terminal/wait?" + q.Encode())
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var out struct {
		Session string `json:"session"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Session, true
}

func (a *Agent) runTerminalSession(sid string) {
	sh := startShell(120, 30)
	if sh == nil {
		return
	}
	log.Printf("远程终端会话开始: %s", sid)
	var once sync.Once
	closeAll := func() { once.Do(func() { _ = sh.Close() }) }
	tok := url.QueryEscape(a.identity.Token)

	// tx: stream shell output up (body ends when the shell exits)
	go func() {
		defer closeAll()
		req, err := http.NewRequest("POST", a.server+"/api/v1/agent/terminal/tx?session="+sid+"&token="+tok, sh)
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		if resp, err := termHTTP.Do(req); err == nil {
			resp.Body.Close()
		}
	}()

	// rx: framed keystrokes / resize from the server → the shell
	go func() {
		defer closeAll()
		resp, err := termHTTP.Get(a.server + "/api/v1/agent/terminal/rx?session=" + sid + "&token=" + tok)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		readTermFrames(resp.Body, sh)
	}()

	_ = sh.Wait()
	closeAll()
	log.Printf("远程终端会话结束: %s", sid)
}

// readTermFrames parses the rx stream: each frame is [type:1][len:2 BE][payload].
// type 'i' = input bytes, 'r' = resize ("colsxrows").
func readTermFrames(r io.Reader, sh termShell) {
	var hdr [3]byte
	for {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return
		}
		n := int(binary.BigEndian.Uint16(hdr[1:]))
		payload := make([]byte, n)
		if n > 0 {
			if _, err := io.ReadFull(r, payload); err != nil {
				return
			}
		}
		switch hdr[0] {
		case 'i':
			if _, err := sh.Write(payload); err != nil {
				return
			}
		case 'r':
			if cols, rows, ok := parseSize(string(payload)); ok {
				_ = sh.Resize(cols, rows)
			}
		}
	}
}

func parseSize(s string) (cols, rows int, ok bool) {
	i := strings.IndexByte(s, 'x')
	if i <= 0 {
		return 0, 0, false
	}
	c, e1 := strconv.Atoi(s[:i])
	rw, e2 := strconv.Atoi(s[i+1:])
	if e1 != nil || e2 != nil || c <= 0 || rw <= 0 || c > 1000 || rw > 1000 {
		return 0, 0, false
	}
	return c, rw, true
}

// shellPath returns the user's preferred shell, falling back to /bin/bash then /bin/sh.
func shellPath() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	for _, s := range []string{"/bin/bash", "/bin/zsh", "/bin/sh"} {
		if _, err := os.Stat(s); err == nil {
			return s
		}
	}
	return "/bin/sh"
}

// buildShellEnv returns a full environment for the spawned shell, filling in
// HOME/USER/PATH/SHELL when the parent process (e.g. systemd) lacks them.
// Without HOME, bash prints "cd: HOME not set" and can't resolve ~ paths.
func buildShellEnv() []string {
	env := os.Environ()
	has := func(key string) bool {
		for _, e := range env {
			if len(e) > len(key)+1 && e[:len(key)+1] == key+"=" {
				return true
			}
		}
		return false
	}
	if !has("HOME") {
		if u, err := user.Current(); err == nil && u.HomeDir != "" {
			env = append(env, "HOME="+u.HomeDir)
		} else if os.Getuid() == 0 {
			env = append(env, "HOME=/root")
		} else {
			env = append(env, "HOME=/tmp")
		}
	}
	if !has("USER") {
		if u, err := user.Current(); err == nil && u.Username != "" {
			env = append(env, "USER="+u.Username, "LOGNAME="+u.Username)
		} else if os.Getuid() == 0 {
			env = append(env, "USER=root", "LOGNAME=root")
		}
	}
	if !has("SHELL") {
		env = append(env, "SHELL="+shellPath())
	}
	if !has("PATH") {
		env = append(env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	}
	env = append(env, "TERM=xterm-256color")
	// Ensure UTF-8 locale on Linux/macOS so command output (including Chinese)
	// is encoded as UTF-8 rather than the legacy C locale.
	if runtime.GOOS != "windows" {
		env = append(env, "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	}
	return env
}

// startShell returns a native PTY shell if the platform supports one, else a
// piped-stdio fallback.
func startShell(cols, rows int) termShell {
	if sh := newPTY(cols, rows); sh != nil {
		return sh
	}
	return newPipeShell()
}

// ---- piped fallback (no PTY) ----

type pipeShell struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	out   *os.File
}

func newPipeShell() termShell {
	name, args := shellCommand()
	cmd := exec.Command(name, args...)
	cmd.Env = buildShellEnv()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		_ = stdin.Close()
		return nil
	}
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = pr.Close()
		_ = pw.Close()
		return nil
	}
	_ = pw.Close() // parent drops its write end so pr EOFs when the shell exits
	return &pipeShell{cmd: cmd, stdin: stdin, out: pr}
}

func (p *pipeShell) Read(b []byte) (int, error) { return p.out.Read(b) }
func (p *pipeShell) Write(b []byte) (int, error) {
	// No PTY → no kernel CR→LF translation, so map Enter (CR) to LF here.
	data := make([]byte, len(b))
	copy(data, b)
	for i := range data {
		if data[i] == '\r' {
			data[i] = '\n'
		}
	}
	return p.stdin.Write(data)
}
func (p *pipeShell) Resize(int, int) error { return nil } // piped shell has no window size
func (p *pipeShell) Wait() error           { return p.cmd.Wait() }
func (p *pipeShell) Close() error {
	_ = p.stdin.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return nil
}

// shellCommand picks the interactive shell per OS (used by the piped fallback).
// On Windows, /K chcp 65001 forces UTF-8 output so Chinese text is not garbled.
func shellCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		if c := os.Getenv("COMSPEC"); c != "" {
			return c, []string{"/K", "chcp 65001 >nul"}
		}
		return "cmd.exe", []string{"/K", "chcp 65001 >nul"}
	}
	return shellPath(), []string{"-l", "-i"} // -l: login shell
}
