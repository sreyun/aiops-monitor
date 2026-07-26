package main

import (
	"context"
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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli, action, id)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return []byte("container_action 超时"), 1
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return []byte(msg), 1
	}
	return []byte(fmt.Sprintf("ok %s %s\n%s", action, id, strings.TrimSpace(string(out)))), 0
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
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, cli, "logs", "--tail", strconv.Itoa(tail), id).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return []byte("container_logs 超时"), 1
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	// Non-interactive: docker exec -i is not needed; avoid -t (TTY).
	cmd := exec.CommandContext(ctx, cli, "exec", id, "sh", "-c", cmdStr)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return []byte("container_exec 超时"), 1
	}
	if len(out) > 256*1024 {
		out = append(out[:256*1024], []byte("\n…[truncated]")...)
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return []byte(msg), 1
	}
	return out, 0
}
