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
