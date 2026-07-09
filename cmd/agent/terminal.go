package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
// requests carry the machine fingerprint (bound at registration), NOT the
// install token — so rotating the token never breaks already-installed agents.
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

// runTerminalChannelFor runs a persistent reverse terminal channel for one
// server target. Each target gets its own goroutine so terminal sessions from
// different servers don't interfere. The fingerprint (machine-bound) is the
// same for all targets; each server independently verifies it.
func (a *Agent) runTerminalChannelFor(t *serverTarget) {
	if a.identity.Fingerprint == "" {
		slog.Warn("远程终端通道未启用：未采集到机器指纹", "server", t.server)
		return
	}
	slog.Info("远程终端通道已就绪，等待服务端呼叫…", "server", t.server)
	for {
		sid, mode, command, ok := a.termWait(t.server)
		if !ok {
			time.Sleep(3 * time.Second)
			continue
		}
		if sid == "" {
			continue // long-poll timeout, re-poll immediately
		}
		if mode == "exec" {
			go a.runExecSession(t.server, sid, command) // one-shot playbook command (no PTY)
		} else {
			go a.runTerminalSession(t.server, sid) // interactive terminal
		}
	}
}

func (a *Agent) termWait(server string) (sessionID, mode, command string, ok bool) {
	q := url.Values{"host": {a.identity.HostID}, "fp": {a.identity.Fingerprint}}
	resp, err := termWaitHTTP.Get(server + "/api/v1/agent/terminal/wait?" + q.Encode())
	if err != nil {
		return "", "", "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", "", false
	}
	var out struct {
		Session string `json:"session"`
		Mode    string `json:"mode"`
		Command string `json:"command"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Session, out.Mode, out.Command, true
}

// runExecSession runs a single playbook command via a one-shot child process
// (NOT an interactive PTY): far more reliable than the terminal + sentinel hack,
// especially on Linux bash where readline / prompts / login banners broke sentinel
// detection. It captures combined stdout+stderr, reports the exit code, streams
// the result up the tx channel, and ends — the agent returns to waiting at once.
func (a *Agent) runExecSession(server, sid, command string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("剧本命令会话异常已恢复", "session", sid, "panic", r)
		}
	}()
	if strings.TrimSpace(command) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// Prefix with chcp 65001 so cmd.exe and its built-in commands emit UTF-8
		// instead of the system ANSI code page (GBK on Chinese Windows). Without
		// this, any Chinese text in the command output is garbled.
		cmd = exec.CommandContext(ctx, "cmd", "/c", "chcp 65001 >nul && "+command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	// Set a UTF-8 locale so command output (including Chinese) is encoded as
	// UTF-8 on all platforms. On Windows, chcp 65001 handles the console code
	// page; PYTHONIOENCODING helps Python programs that read the env.
	cmd.Env = execEnv()
	out, err := cmd.CombinedOutput()
	exit := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		} else {
			exit = -1
			out = append(out, []byte("\n"+err.Error())...)
		}
	}
	// Fallback: some programs bypass chcp and emit bytes in the system ANSI
	// code page (e.g., a C program using printf with GBK literals). Convert any
	// non-UTF-8 bytes to UTF-8 via the Windows API (no-op on Linux/macOS).
	out = ensureUTF8(out)
	// The server detects completion by the tx body ending; the exit code is
	// appended on its own line so success/failure can be surfaced precisely.
	body := append(out, []byte(fmt.Sprintf("\n[AIOPS_EXIT]%d\n", exit))...)
	req, err := http.NewRequest("POST",
		server+"/api/v1/agent/terminal/tx?session="+sid+"&fp="+url.QueryEscape(a.identity.Fingerprint),
		bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if resp, err := termHTTP.Do(req); err == nil {
		resp.Body.Close()
	}
}

func (a *Agent) runTerminalSession(server, sid string) {
	// A terminal/playbook session must never crash the whole agent: recover any
	// panic so metrics reporting and future sessions keep working.
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("终端会话异常已恢复（不影响 Agent 运行）", "session", sid, "panic", r)
		}
	}()
	sh := startShell(120, 30)
	if sh == nil {
		return
	}
	slog.Info("远程终端会话开始", "session", sid)
	var once sync.Once
	closeAll := func() { once.Do(func() { _ = sh.Close() }) }
	fp := url.QueryEscape(a.identity.Fingerprint)

	// zmChan carries upload data received from the browser (via rx stream)
	// to the ZMODEM upload handler running in the tx goroutine.
	zmChan := make(chan []byte, 32)

	// tx: stream shell output up with ZMODEM detection + framing
	// Frame format: [type:1][len:4 BE][payload]
	//   'O' (0x4F) = normal PTY output
	//   'Z' (0x5A) = ZMODEM signal (JSON with filename/size)
	//   'D' (0x44) = download data chunk
	//   'E' (0x45) = transfer complete
	go func() {
		defer closeAll()
		pr, pw := io.Pipe()
		req, err := http.NewRequest("POST", server+"/api/v1/agent/terminal/tx?session="+sid+"&fp="+fp, pr)
		if err != nil {
			pw.Close()
			return
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		// Fire off the HTTP request in a goroutine; write to pw in the main goroutine.
		reqDone := make(chan error, 1)
		go func() {
			resp, doErr := termHTTP.Do(req)
			if doErr == nil {
				resp.Body.Close()
			}
			reqDone <- doErr
		}()

		// Write framed PTY output to pw, with ZMODEM interception.
		streamPTYFramed(sh, pw, zmChan)
		pw.Close()
		<-reqDone
	}()

	// rx: framed keystrokes / resize / upload from the server → the shell
	go func() {
		defer closeAll()
		resp, err := termHTTP.Get(server + "/api/v1/agent/terminal/rx?session=" + sid + "&fp=" + fp)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		readTermFrames(resp.Body, sh, zmChan)
	}()

	_ = sh.Wait()
	closeAll()
	slog.Info("远程终端会话结束", "session", sid)
}

// readTermFrames parses the rx stream: each frame is [type:1][len:2 BE][payload].
// type 'i' = input bytes, 'r' = resize ("colsxrows"),
// 'u' = upload data chunk, 'e' = end of upload.
func readTermFrames(r io.Reader, sh termShell, zmChan chan<- []byte) {
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
		case 'u':
			// Upload data chunk → forward to ZMODEM handler
			if len(payload) > 0 {
				select {
				case zmChan <- payload:
				default:
					// channel full, drop chunk (ZMODEM will retransmit)
				}
			}
		case 'e':
			// End of upload → signal ZMODEM handler
			select {
			case zmChan <- nil:
			default:
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

// execEnv returns the environment for playbook exec sessions. It inherits
// the agent's environment (PATH/HOME/…) and forces UTF-8 locale on all
// platforms so command output (including Chinese) is always UTF-8.
func execEnv() []string {
	env := os.Environ()
	if runtime.GOOS != "windows" {
		// Ensure UTF-8 locale on Linux/macOS: some minimal containers default to
		// the C locale which mangles non-ASCII output.
		env = append(env, "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	} else {
		// Windows: chcp 65001 sets the console code page, but Python and other
		// runtimes also check these env vars for UTF-8 I/O.
		env = append(env, "PYTHONIOENCODING=utf-8")
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

// ---- ZMODEM-aware PTY output stream ----

// writeFrame writes a framed message: [type:1][len:4 BE][payload].
func writeFrame(w io.Writer, typ byte, payload []byte) error {
	if len(payload) > 0x7FFFFFFF {
		payload = payload[:0x7FFFFFFF]
	}
	var hdr [5]byte
	hdr[0] = typ
	binary.BigEndian.PutUint32(hdr[1:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		_, err := w.Write(payload)
		return err
	}
	return nil
}

// streamPTYFramed reads from the PTY, detects ZMODEM headers, and writes
// framed data to w. Normal PTY output is written as 'O' frames. When ZMODEM
// is detected, the function runs the ZMODEM protocol and writes 'Z'/'D'/'E'
// frames. Upload data is received via zmChan.
//
// Uses a goroutine for PTY reading so the main loop can multiplex PTY data,
// upload data (zmChan), and a rz/sz detection timer via select.
func streamPTYFramed(ptyReader io.Reader, w io.Writer, zmChan <-chan []byte) {
	// PTY reader goroutine → channel (avoids blocking main loop when
	// remote rz is waiting for ZFILE and no PTY output is coming).
	type ptyData struct {
		data []byte
		err  error
	}
	ptyCh := make(chan ptyData, 1)
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := ptyReader.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				ptyCh <- ptyData{data, nil}
			}
			if err != nil {
				ptyCh <- ptyData{nil, err}
				return
			}
		}
	}()

	// State
	inZmodem := false
	var zmSession *ZmSession
	var uploadBuf []byte
	uploadReady := false

	// rz/sz detection: after ZRQINIT+ZRINIT, wait 2s for ZFILE from remote.
	// If ZFILE arrives → sz (download). If timer fires → rz (upload).
	zmNeedDetect := false
	zmDetectTimer := time.NewTimer(0)
	if !zmDetectTimer.Stop() {
		<-zmDetectTimer.C
	}

	// zmAccum accumulates partial ZMODEM frame data across PTY read chunks.
	// When parseZmFrame returns nil (incomplete frame), the remaining bytes
	// are saved here and prepended to the next chunk.
	var zmAccum []byte

	// processZmFrames parses and handles ZMODEM frames from a byte slice.
	// It also detects sz (ZFILE arriving while zmNeedDetect is true).
	processZmFrames := func(chunk []byte) {
		zmBuf := append(zmAccum, chunk...)
		zmAccum = nil
		for len(zmBuf) > 0 {
			frame, consumed, _ := parseZmFrame(zmBuf)
			if frame == nil {
				// Partial frame — save remaining bytes for next chunk
				zmAccum = append(zmAccum, zmBuf...)
				break
			}
			zmBuf = zmBuf[consumed:]

			// sz detection: ZFILE arrived while we're waiting to distinguish sz vs rz
			if zmNeedDetect && frame.Type == ZFILE {
				if !zmDetectTimer.Stop() {
					select {
					case <-zmDetectTimer.C:
					default:
					}
				}
				zmNeedDetect = false
				info := parseZFileInfo(frame.Data)
				fname := "download.dat"
				fsize := int64(0)
				if info != nil {
					if info.Name != "" {
						fname = info.Name
					}
					if info.Size > 0 {
						fsize = info.Size
					}
				}
				zmJSON := fmt.Sprintf(`{"type":"sz","filename":"%s","size":%d}`, fname, fsize)
				writeFrame(w, 'Z', []byte(zmJSON))
				slog.Info("ZMODEM检测为sz(下载)", "filename", fname, "size", fsize)
			}

			// Skip ZRQINIT — we already sent ZRINIT in the detection block
			if frame.Type == ZRQINIT {
				continue
			}

			responses := zmSession.HandleFrame(frame)
			for _, resp := range responses {
				if w2, ok := ptyReader.(io.Writer); ok {
					w2.Write(resp)
				}
			}

			// Check if download is complete
			if zmSession.State == zmIdle && zmSession.DataBuf.Len() > 0 {
				fname := "download.dat"
				fsize := int64(zmSession.DataBuf.Len())
				if zmSession.File != nil && zmSession.File.Name != "" {
					fname = zmSession.File.Name
				}
				slog.Info("ZMODEM下载完成", "filename", fname, "size", fsize)

				// 'Z' frame (type "sz") already sent at ZFILE detection time.
				// Now send the file data and completion marker.
				data := zmSession.DataBuf.Bytes()
				chunkSz := 32 << 10
				for offset := 0; offset < len(data); offset += chunkSz {
					end := offset + chunkSz
					if end > len(data) {
						end = len(data)
					}
					writeFrame(w, 'D', data[offset:end])
				}
				writeFrame(w, 'E', nil)

				inZmodem = false
				zmSession = nil
				zmAccum = nil
				uploadBuf = nil
				uploadReady = false
				break
			}
		}
	}

	for {
		// Drain zmChan: accumulate upload data
		drained := true
		for drained {
			select {
			case chunk := <-zmChan:
				if chunk == nil {
					uploadReady = true
				} else {
					uploadBuf = append(uploadBuf, chunk...)
				}
			default:
				drained = false
			}
		}

		// If upload data is ready and we're in ZMODEM mode (rz), send ZFILE to remote.
		if uploadReady && len(uploadBuf) > 0 && inZmodem && zmSession != nil && zmSession.State == zmInit {
			slog.Info("ZMODEM上传数据已就绪，开始上传协议", "size", len(uploadBuf))
			zmSession.UploadData = uploadBuf
			zmSession.File = &ZFileInfo{Name: "upload.dat", Size: int64(len(uploadBuf))}
			zmSession.State = zmFile
			if w2, ok := ptyReader.(io.Writer); ok {
				w2.Write(buildZfileFrame(zmSession.File.Name, zmSession.File.Size))
			}
			uploadBuf = nil
			uploadReady = false
		}

		// Wait for PTY data, upload data, or detection timer
		select {
		case pd := <-ptyCh:
			if pd.err != nil {
				if pd.err != io.EOF {
					slog.Warn("PTY读取错误", "err", pd.err)
				}
				return
			}
			chunk := pd.data

			if inZmodem {
				processZmFrames(chunk)
			} else {
				// Check for ZMODEM header
				if HasZmodemHeader(chunk) {
					idx := IndexZmodemHeader(chunk)
					if idx > 0 {
						writeFrame(w, 'O', chunk[:idx])
					}
					zmChunk := chunk[idx:]
					frame, _, _ := parseZmFrame(zmChunk)

					// Enter ZMODEM mode
					inZmodem = true
					zmSession = NewZmSession()
					zmSession.State = zmInit

					if frame != nil && frame.Type == ZRQINIT {
						// Always send ZRINIT immediately — both sz and rz need it.
						// Without ZRINIT, the remote side will timeout.
						if w2, ok := ptyReader.(io.Writer); ok {
							w2.Write(buildZrinitFrame())
						}
						slog.Info("ZMODEM握手检测到(ZRQINIT)，已发送ZRINIT，等待检测sz/rz")

						// Start 2-second timer to detect sz vs rz.
						// If ZFILE arrives within 2s → sz (download).
						// If timer fires → rz (upload).
						zmNeedDetect = true
						zmDetectTimer.Reset(2 * time.Second)

						// Process all frames in the chunk (including ZFILE if it's sz)
						processZmFrames(zmChunk)
					} else {
						slog.Info("ZMODEM握手检测到(非ZRQINIT)，开始文件传输")
						processZmFrames(zmChunk)
					}
				} else {
					// Normal output
					writeFrame(w, 'O', chunk)
				}
			}

		case <-zmDetectTimer.C:
			// Timer fired — no ZFILE within 2s → it's rz (upload)
			if zmNeedDetect {
				zmJSON := `{"type":"rz"}`
				writeFrame(w, 'Z', []byte(zmJSON))
				zmNeedDetect = false
				slog.Info("ZMODEM检测为rz(上传)，等待文件选择")
			}

		case chunk := <-zmChan:
			if chunk == nil {
				uploadReady = true
			} else {
				uploadBuf = append(uploadBuf, chunk...)
			}
		}
	}
}
