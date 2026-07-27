//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// agentReplaceAndRestart replaces the running binary with staging (Linux allows
// renaming over a busy executable) then schedules a detached service restart.
func agentReplaceAndRestart(exe, staging string) error {
	// Prefer atomic rename on the same filesystem; never write-through a live path
	// via copyFile (ETXTBSY / partial write risk).
	if err := os.Rename(staging, exe); err != nil {
		sameDirStaging := filepath.Join(filepath.Dir(exe), ".aiops-agent.replace"+exeSuffix())
		if staging != sameDirStaging {
			if err2 := copyFile(staging, sameDirStaging); err2 != nil {
				return fmt.Errorf("replace binary: stage copy: %v (rename: %v)", err2, err)
			}
			_ = os.Remove(staging)
			staging = sameDirStaging
		}
		if err2 := os.Rename(staging, exe); err2 != nil {
			return fmt.Errorf("replace binary: %v / %v", err, err2)
		}
	}
	_ = os.Chmod(exe, 0o755)
	return scheduleAgentRestart(exe)
}

func scheduleAgentRestart(exe string) error {
	dir := filepath.Dir(exe)
	switch runtime.GOOS {
	case "linux":
		unit := detectLinuxAgentUnit()
		script := fmt.Sprintf(`
sleep 2
EXE=%s
DIR=%s
UNIT=%s
RESTARTED=0
if systemctl restart "$UNIT" 2>/dev/null || systemctl restart aiops-agent 2>/dev/null || systemctl restart aiops-monitor-agent 2>/dev/null; then
  RESTARTED=1
fi
if [ "$RESTARTED" -eq 0 ]; then
  CFG=""
  for c in "$DIR/config.yaml" "$DIR/config.yml" "$HOME/.aiops-agent/config.yaml"; do
    [ -f "$c" ] && CFG="$c" && break
  done
  pkill -x aiops-agent 2>/dev/null || pkill -f '[/]aiops-agent( |$)' 2>/dev/null || true
  sleep 1
  if [ -n "$CFG" ]; then
    nohup "$EXE" --config "$CFG" >/dev/null 2>&1 &
  else
    nohup "$EXE" >/dev/null 2>&1 &
  fi
fi
`, shellQuote(exe), shellQuote(dir), shellQuote(unit))
		cmd := exec.Command("sh", "-c", script)
		return cmd.Start()
	case "darwin":
		script := fmt.Sprintf(`
sleep 2
EXE=%s
DIR=%s
UIDN=$(id -u)
xattr -dr com.apple.quarantine "$EXE" 2>/dev/null || true
RESTARTED=0
for label in "gui/$UIDN/com.aiops.agent" "system/com.aiops.agent" "system/com.aiops.monitor.agent"; do
  if launchctl kickstart -k "$label" 2>/dev/null; then RESTARTED=1; break; fi
done
if [ "$RESTARTED" -eq 0 ]; then
  CFG=""
  for c in "$DIR/config.yaml" "$DIR/config.yml" "$HOME/.aiops-agent/config.yaml"; do
    [ -f "$c" ] && CFG="$c" && break
  done
  pkill -x aiops-agent 2>/dev/null || true
  sleep 1
  if [ -n "$CFG" ]; then
    nohup "$EXE" --config "$CFG" >/dev/null 2>&1 &
  else
    nohup "$EXE" >/dev/null 2>&1 &
  fi
fi
`, shellQuote(exe), shellQuote(dir))
		cmd := exec.Command("sh", "-c", script)
		return cmd.Start()
	default:
		return fmt.Errorf("restart not supported on %s", runtime.GOOS)
	}
}

func detectLinuxAgentUnit() string {
	for _, u := range []string{"aiops-agent", "aiops-monitor-agent"} {
		out, err := exec.Command("systemctl", "is-active", u).CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) == "active" {
			return u
		}
	}
	if _, err := os.Stat("/etc/systemd/system/aiops-agent.service"); err == nil {
		return "aiops-agent"
	}
	if _, err := os.Stat("/etc/systemd/system/aiops-monitor-agent.service"); err == nil {
		return "aiops-monitor-agent"
	}
	return "aiops-agent"
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
