package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// containerCLI returns "docker" or "podman" if available.
func containerCLI() string {
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker"
	}
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman"
	}
	return ""
}

// moduleContainerAction start/stop/restart a container by id or name.
func moduleContainerAction(args map[string]string) ([]byte, int) {
	cli := containerCLI()
	if cli == "" {
		return []byte("未找到 docker 或 podman"), 1
	}
	action := strings.ToLower(strings.TrimSpace(args["action"]))
	id := strings.TrimSpace(args["id"])
	if id == "" {
		id = strings.TrimSpace(args["name"])
	}
	if id == "" {
		return []byte("container_action 缺少 id/name"), 1
	}
	switch action {
	case "start", "stop", "restart":
	default:
		return []byte("未知 action: " + action + "（start|stop|restart）"), 1
	}
	ctxTimeout := 60 * time.Second
	cmd := exec.Command(cli, action, id)
	done := make(chan struct {
		out []byte
		err error
	}, 1)
	go func() {
		out, err := cmd.CombinedOutput()
		done <- struct {
			out []byte
			err error
		}{out, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			msg := strings.TrimSpace(string(r.out))
			if msg == "" {
				msg = r.err.Error()
			}
			return []byte(msg), 1
		}
		return []byte(fmt.Sprintf("ok %s %s\n%s", action, id, strings.TrimSpace(string(r.out)))), 0
	case <-time.After(ctxTimeout):
		_ = cmd.Process.Kill()
		return []byte("container_action 超时"), 1
	}
}

// moduleContainerLogs returns recent container logs.
func moduleContainerLogs(args map[string]string) ([]byte, int) {
	cli := containerCLI()
	if cli == "" {
		return []byte("未找到 docker 或 podman"), 1
	}
	id := strings.TrimSpace(args["id"])
	if id == "" {
		id = strings.TrimSpace(args["name"])
	}
	if id == "" {
		return []byte("container_logs 缺少 id/name"), 1
	}
	tail := 200
	if t := strings.TrimSpace(args["tail"]); t != "" {
		if n, err := strconv.Atoi(t); err == nil && n > 0 && n <= 5000 {
			tail = n
		}
	}
	out, err := exec.Command(cli, "logs", "--tail", strconv.Itoa(tail), id).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return []byte(msg), 1
	}
	return out, 0
}

// moduleContainerExec runs a non-interactive short command inside a container.
// Args: id|name, command (shell string), optional timeout_sec (5~60, default 20).
func moduleContainerExec(args map[string]string) ([]byte, int) {
	cli := containerCLI()
	if cli == "" {
		return []byte("未找到 docker 或 podman"), 1
	}
	id := strings.TrimSpace(args["id"])
	if id == "" {
		id = strings.TrimSpace(args["name"])
	}
	if id == "" {
		return []byte("container_exec 缺少 id/name"), 1
	}
	cmdStr := strings.TrimSpace(args["command"])
	if cmdStr == "" {
		return []byte("container_exec 缺少 command"), 1
	}
	if len(cmdStr) > 2000 {
		return []byte("command 过长（≤2000）"), 1
	}
	timeoutSec := 20
	if t := strings.TrimSpace(args["timeout_sec"]); t != "" {
		if n, err := strconv.Atoi(t); err == nil && n >= 5 && n <= 60 {
			timeoutSec = n
		}
	}
	// Non-interactive: docker exec -i is not needed; avoid -t (TTY).
	cmd := exec.Command(cli, "exec", id, "sh", "-c", cmdStr)
	done := make(chan struct {
		out []byte
		err error
	}, 1)
	go func() {
		out, err := cmd.CombinedOutput()
		done <- struct {
			out []byte
			err error
		}{out, err}
	}()
	select {
	case r := <-done:
		out := r.out
		if len(out) > 256*1024 {
			out = append(out[:256*1024], []byte("\n…[truncated]")...)
		}
		if r.err != nil {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = r.err.Error()
			}
			return []byte(msg), 1
		}
		return out, 0
	case <-time.After(time.Duration(timeoutSec) * time.Second):
		_ = cmd.Process.Kill()
		return []byte("container_exec 超时"), 1
	}
}
