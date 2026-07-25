package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func composeCLI() (bin string, argsPrefix []string) {
	// Prefer "docker compose" (v2 plugin), then standalone docker-compose, then podman-compose.
	if _, err := exec.LookPath("docker"); err == nil {
		if err := exec.Command("docker", "compose", "version").Run(); err == nil {
			return "docker", []string{"compose"}
		}
	}
	if _, err := exec.LookPath("docker-compose"); err == nil {
		return "docker-compose", nil
	}
	if _, err := exec.LookPath("podman-compose"); err == nil {
		return "podman-compose", nil
	}
	if _, err := exec.LookPath("podman"); err == nil {
		if err := exec.Command("podman", "compose", "version").Run(); err == nil {
			return "podman", []string{"compose"}
		}
	}
	return "", nil
}

// moduleComposeList lists compose projects on the host.
func moduleComposeList(args map[string]string) ([]byte, int) {
	bin, prefix := composeCLI()
	if bin == "" {
		return []byte("skip: 未找到 docker compose / podman-compose，跳过 Compose 列表\n"), 0
	}
	cmdArgs := append(append([]string{}, prefix...), "ls", "-a", "--format", "json")
	out, err := exec.Command(bin, cmdArgs...).CombinedOutput()
	if err != nil {
		// Older compose may not support ls --format json; fall back to plain ls.
		cmdArgs = append(append([]string{}, prefix...), "ls", "-a")
		out2, err2 := exec.Command(bin, cmdArgs...).CombinedOutput()
		if err2 != nil {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = err.Error()
			}
			return []byte(msg), 1
		}
		return []byte(`{"projects_text":` + strconv.Quote(string(out2)) + `,"cli":` + strconv.Quote(bin) + `}`), 0
	}
	text := strings.TrimSpace(string(out))
	// docker compose ls --format json may emit NDJSON (one object per line).
	projects := []json.RawMessage{}
	if strings.HasPrefix(text, "[") {
		_ = json.Unmarshal([]byte(text), &projects)
	} else {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			projects = append(projects, json.RawMessage(line))
		}
	}
	resp, _ := json.Marshal(map[string]any{"cli": bin, "projects": projects, "raw": text})
	return resp, 0
}

// moduleComposeAction runs compose up/down/ps/logs/pull/restart for a project or file.
// Args: action, project|name, file (compose yaml path), services (optional), timeout_sec.
func moduleComposeAction(args map[string]string) ([]byte, int) {
	bin, prefix := composeCLI()
	if bin == "" {
		return []byte("未找到 docker compose / docker-compose / podman-compose"), 1
	}
	action := strings.ToLower(strings.TrimSpace(args["action"]))
	switch action {
	case "up", "down", "ps", "logs", "pull", "restart", "stop", "start":
	default:
		return []byte("未知 action（up|down|ps|logs|pull|restart|stop|start）"), 1
	}
	project := strings.TrimSpace(args["project"])
	if project == "" {
		project = strings.TrimSpace(args["name"])
	}
	file := strings.TrimSpace(args["file"])
	if file != "" {
		if !filepath.IsAbs(file) {
			return []byte("file 必须是绝对路径"), 1
		}
		if st, err := os.Stat(file); err != nil || st.IsDir() {
			return []byte("compose 文件不存在: " + file), 1
		}
	}
	if project == "" && file == "" {
		return []byte("需要 project 或 file"), 1
	}

	cmdArgs := append([]string{}, prefix...)
	if file != "" {
		cmdArgs = append(cmdArgs, "-f", file)
	}
	if project != "" {
		cmdArgs = append(cmdArgs, "-p", project)
	}
	switch action {
	case "up":
		cmdArgs = append(cmdArgs, "up", "-d", "--remove-orphans")
	case "down":
		cmdArgs = append(cmdArgs, "down")
	case "ps":
		cmdArgs = append(cmdArgs, "ps", "-a")
	case "logs":
		tail := "200"
		if t := strings.TrimSpace(args["tail"]); t != "" {
			if n, err := strconv.Atoi(t); err == nil && n > 0 && n <= 5000 {
				tail = strconv.Itoa(n)
			}
		}
		cmdArgs = append(cmdArgs, "logs", "--tail", tail, "--no-color")
	case "pull":
		cmdArgs = append(cmdArgs, "pull")
	case "restart":
		cmdArgs = append(cmdArgs, "restart")
	case "stop":
		cmdArgs = append(cmdArgs, "stop")
	case "start":
		cmdArgs = append(cmdArgs, "start")
	}
	if svc := strings.TrimSpace(args["services"]); svc != "" {
		for _, s := range strings.Fields(strings.ReplaceAll(svc, ",", " ")) {
			if s != "" {
				cmdArgs = append(cmdArgs, s)
			}
		}
	}

	timeout := 180 * time.Second
	if t := strings.TrimSpace(args["timeout_sec"]); t != "" {
		if n, err := strconv.Atoi(t); err == nil && n >= 30 && n <= 600 {
			timeout = time.Duration(n) * time.Second
		}
	}
	cmd := exec.Command(bin, cmdArgs...)
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
		out := r.out
		if len(out) > 512*1024 {
			out = append(out[:512*1024], []byte("\n…[truncated]")...)
		}
		return out, 0
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return []byte(fmt.Sprintf("compose %s 超时", action)), 1
	}
}
