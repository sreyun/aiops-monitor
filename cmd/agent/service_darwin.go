//go:build darwin

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// macOS always-on daemon: a root LaunchDaemon runs metrics/terminal/forward and
// supervises a remote-desktop worker launched *into the console user's GUI (Aqua)
// session* via `launchctl asuser`. Screen capture (screencapture) and input
// (cliclick/osascript) only work inside that GUI session ? a bare LaunchDaemon
// context cannot see the desktop.
//
// The worker runs within the GUI session (host_id stays readable). The binary
// still needs Screen Recording + Accessibility granted in
// System Settings ? Privacy & Security.

const agentServiceName = "com.aiops.monitor.agent"

const launchDaemonPlist = "/Library/LaunchDaemons/com.aiops.monitor.agent.plist"

func installAgentService(exePath, cfgPath string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("?? root ?????? sudo ?? --install-service")
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>--service</string>
		<string>--config</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>/var/log/aiops-monitor-agent.log</string>
	<key>StandardErrorPath</key>
	<string>/var/log/aiops-monitor-agent.log</string>
</dict>
</plist>
`, agentServiceName, exePath, cfgPath)

	if err := os.WriteFile(launchDaemonPlist, []byte(plist), 0o644); err != nil {
		return fmt.Errorf("?? LaunchDaemon plist ??: %w", err)
	}
	// bootout first (ignore errors) so a re-install reloads cleanly.
	_ = runSvcCmd("launchctl", "bootout", "system/"+agentServiceName)
	_ = runSvcCmd("launchctl", "unload", launchDaemonPlist)
	if err := runSvcCmd("launchctl", "bootstrap", "system", launchDaemonPlist); err != nil {
		// Fall back to legacy load on older macOS.
		if err2 := runSvcCmd("launchctl", "load", "-w", launchDaemonPlist); err2 != nil {
			return fmt.Errorf("launchctl ????: %v / %v", err, err2)
		}
	}
	return nil
}

func uninstallAgentService() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("?? root ?????? sudo ?? --uninstall-service")
	}
	_ = runSvcCmd("launchctl", "bootout", "system/"+agentServiceName)
	_ = runSvcCmd("launchctl", "unload", launchDaemonPlist)
	_ = os.Remove(launchDaemonPlist)
	return nil
}

// runAgentAsService is invoked by launchd (--service). Metrics run as root; the
// desktop channel is delegated to a GUI-session worker.
func runAgentAsService(agent *Agent, cfgPath string) error {
	agent.desktopDisabled = true
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("???????????: %w", err)
	}

	go agent.Run(ctx)
	slog.Info("Agent LaunchDaemon ???(root)", "config", cfgPath)
	superviseDarwinDesktopWorker(ctx, exe, cfgPath)
	return nil
}

func runDesktopWorker(agent *Agent) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	agent.RunDesktopOnly(ctx)
	return nil
}

// ---- supervisor ---------------------------------------------------------

func superviseDarwinDesktopWorker(ctx context.Context, exe, cfgPath string) {
	var worker *bgProc
	curUID := -1
	stopWorker := func() {
		if worker != nil {
			worker.kill()
			worker = nil
			curUID = -1
		}
	}
	defer stopWorker()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		uid, uname := consoleUser()
		if uid > 0 {
			if worker == nil || !worker.alive() || uid != curUID {
				stopWorker()
				w, err := spawnDarwinDesktopWorker(exe, cfgPath, uid)
				if err != nil {
					slog.Warn("?? macOS ?? worker ??", "err", err, "uid", uid, "user", uname)
				} else {
					worker = w
					curUID = uid
					slog.Info("?? GUI ??????? worker", "user", uname, "uid", uid)
				}
			}
		} else {
			stopWorker()
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func spawnDarwinDesktopWorker(exe, cfgPath string, uid int) (*bgProc, error) {
	args := []string{"asuser", strconv.Itoa(uid), exe, "--desktop-worker"}
	if cfgPath != "" {
		args = append(args, "--config", cfgPath)
	}
	cmd := exec.Command("launchctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return newBgProc(cmd), nil
}

// consoleUser returns the uid/name of the user owning the GUI console, or
// (-1,"") when only the login window is up (uid<500 / root / _windowserver).
func consoleUser() (int, string) {
	out, err := exec.Command("stat", "-f", "%Uu %Su", "/dev/console").Output()
	if err != nil {
		return -1, ""
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 2 {
		return -1, ""
	}
	uid, err := strconv.Atoi(fields[0])
	if err != nil {
		return -1, ""
	}
	name := fields[1]
	if uid < 500 || name == "root" || name == "_windowserver" || name == "" {
		return -1, ""
	}
	return uid, name
}
