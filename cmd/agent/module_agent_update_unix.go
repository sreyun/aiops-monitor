//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// agentReplaceAndRestart replaces the running binary with staging (Linux allows
// renaming over a busy executable) then schedules a detached service restart.
func agentReplaceAndRestart(exe, staging, cfgPath string) error {
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
	if strings.TrimSpace(cfgPath) == "" {
		cfgPath = resolveAgentConfigBesideExe(filepath.Dir(exe))
	}
	return scheduleAgentRestart(exe, cfgPath)
}

func scheduleAgentRestart(exe, cfgPath string) error {
	dir := filepath.Dir(exe)
	switch runtime.GOOS {
	case "linux":
		unit := detectLinuxAgentUnit()
		// Critical: the agent (and this helper, if spawned without nsenter) may run
		// inside a systemd ProtectSystem mount namespace where /etc is read-only.
		// Fresh curl|bash install works because it runs outside that ns; auto-upgrade
		// must nsenter into PID 1 before --install-service / unit rewrite.
		script := fmt.Sprintf(`
sleep 2
EXE=%s
DIR=%s
CFG=%s
UNIT=%s
RESTARTED=0

# Run a command in the host mount namespace when possible (escape ProtectSystem).
host_run() {
  if [ "$(id -u)" -eq 0 ] && command -v nsenter >/dev/null 2>&1 && [ -e /proc/1/ns/mnt ]; then
    nsenter -t 1 -m -u -i -n -- "$@"
  else
    "$@"
  fi
}

# Shared body: unlock units (must execute inside host mount ns).
UNLOCK_SH='for u in aiops-agent aiops-monitor-agent; do
  for base in /etc/systemd/system /run/systemd/system /lib/systemd/system /usr/lib/systemd/system; do
    rm -rf "$base/${u}.service.d" 2>/dev/null || true
  done
  f="/etc/systemd/system/${u}.service"
  [ -f "$f" ] || continue
  sed -i \
    -e "s/^User=.*/User=root/" \
    -e "s/^Group=.*/Group=root/" \
    -e "s/^ProtectHome=.*/ProtectHome=false/" \
    -e "s/^ProtectSystem=.*/ProtectSystem=false/" \
    -e "s/^PrivateTmp=.*/PrivateTmp=false/" \
    -e "s/^NoNewPrivileges=.*/NoNewPrivileges=false/" \
    -e "s|^Environment=HOME=.*|Environment=HOME=/root|" \
    -e "s|^Environment=USER=.*|Environment=USER=root|" \
    -e "s|^Environment=LOGNAME=.*|Environment=LOGNAME=root|" \
    -e "/^CapabilityBoundingSet=/d" \
    -e "/^ReadWritePaths=/d" \
    -e "/^ReadOnlyPaths=/d" \
    -e "/^InaccessiblePaths=/d" \
    -e "/^TemporaryFileSystem=/d" \
    "$f" 2>/dev/null || true
  grep -q "^User=root" "$f" 2>/dev/null || echo "User=root" >> "$f"
  grep -q "^ProtectHome=false" "$f" 2>/dev/null || echo "ProtectHome=false" >> "$f"
  grep -q "^ProtectSystem=false" "$f" 2>/dev/null || echo "ProtectSystem=false" >> "$f"
  grep -q "^PrivateTmp=false" "$f" 2>/dev/null || echo "PrivateTmp=false" >> "$f"
  grep -q "^NoNewPrivileges=false" "$f" 2>/dev/null || echo "NoNewPrivileges=false" >> "$f"
done
systemctl daemon-reload 2>/dev/null || true'

# 1) Prefer full --install-service from host ns (same as fresh reinstall).
if [ -n "$CFG" ] && [ -f "$CFG" ]; then
  if [ "$(id -u)" -eq 0 ]; then
    if host_run "$EXE" --install-service --config "$CFG" >/tmp/aiops-agent-update-install.log 2>&1; then
      RESTARTED=1
    fi
  elif command -v sudo >/dev/null 2>&1 && sudo -n true >/dev/null 2>&1; then
    if sudo -n nsenter -t 1 -m -u -i -n -- "$EXE" --install-service --config "$CFG" >/tmp/aiops-agent-update-install.log 2>&1 \
       || sudo -n "$EXE" --install-service --config "$CFG" >/tmp/aiops-agent-update-install.log 2>&1; then
      RESTARTED=1
    fi
  fi
fi

# 2) Fallback: unlock unit in host ns, then systemctl restart (keeps unit name).
if [ "$RESTARTED" -eq 0 ]; then
  if [ "$(id -u)" -eq 0 ]; then
    host_run sh -c "$UNLOCK_SH"
  elif command -v sudo >/dev/null 2>&1 && sudo -n true >/dev/null 2>&1; then
    sudo -n nsenter -t 1 -m -- sh -c "$UNLOCK_SH" 2>/dev/null \
      || sudo -n sh -c "$UNLOCK_SH" 2>/dev/null || true
  fi
  if systemctl restart "$UNIT" 2>/dev/null || systemctl restart aiops-agent 2>/dev/null || systemctl restart aiops-monitor-agent 2>/dev/null; then
    RESTARTED=1
  fi
fi

if [ "$RESTARTED" -eq 0 ]; then
  if [ -z "$CFG" ]; then
    for c in "$DIR/config.yaml" "$DIR/config.yml" "$HOME/.aiops-agent/config.yaml"; do
      [ -f "$c" ] && CFG="$c" && break
    done
  fi
  pkill -x aiops-agent 2>/dev/null || pkill -f '[/]aiops-agent( |$)' 2>/dev/null || true
  sleep 1
  if [ -n "$CFG" ]; then
    nohup "$EXE" --config "$CFG" >/dev/null 2>&1 &
  else
    nohup "$EXE" >/dev/null 2>&1 &
  fi
  sleep 1
  if pgrep -x aiops-agent >/dev/null 2>&1 || pgrep -f '[/]aiops-agent( |$)' >/dev/null 2>&1; then
    RESTARTED=1
  fi
fi
if [ "$RESTARTED" -eq 0 ]; then
  echo "agent restart failed" >&2
  exit 1
fi
`, shellQuote(exe), shellQuote(dir), shellQuote(cfgPath), shellQuote(unit))
		return startDetachedShell(script)
	case "darwin":
		script := fmt.Sprintf(`
sleep 2
EXE=%s
DIR=%s
CFG=%s
UIDN=$(id -u)
xattr -dr com.apple.quarantine "$EXE" 2>/dev/null || true
RESTARTED=0
if [ -n "$CFG" ] && [ -f "$CFG" ]; then
  if "$EXE" --install-service --config "$CFG" >/dev/null 2>&1; then
    RESTARTED=1
  fi
fi
# Always kickstart — --install-service bootstrap alone may not run the new binary.
for label in "system/com.aiops.monitor.agent" "system/com.aiops.agent" "gui/$UIDN/com.aiops.agent" "gui/$UIDN/com.aiops.monitor.agent"; do
  if launchctl kickstart -k "$label" 2>/dev/null; then RESTARTED=1; break; fi
done
if [ "$RESTARTED" -eq 0 ]; then
  if [ -z "$CFG" ]; then
    for c in "$DIR/config.yaml" "$DIR/config.yml" "$HOME/.aiops-agent/config.yaml"; do
      [ -f "$c" ] && CFG="$c" && break
    done
  fi
  pkill -x aiops-agent 2>/dev/null || true
  sleep 1
  if [ -n "$CFG" ]; then
    nohup "$EXE" --config "$CFG" >/dev/null 2>&1 &
  else
    nohup "$EXE" >/dev/null 2>&1 &
  fi
  sleep 1
  pgrep -x aiops-agent >/dev/null 2>&1 && RESTARTED=1
fi
if [ "$RESTARTED" -eq 0 ]; then
  echo "agent restart failed" >&2
  exit 1
fi
`, shellQuote(exe), shellQuote(dir), shellQuote(cfgPath))
		return startDetachedShell(script)
	default:
		return fmt.Errorf("restart not supported on %s", runtime.GOOS)
	}
}

// startDetachedShell runs the restart helper in a new session so systemd/launchd
// stopping the agent does not also kill the helper (Windows uses DETACHED_PROCESS).
func startDetachedShell(script string) error {
	cmd := exec.Command("sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start restart helper: %w", err)
	}
	return nil
}

func detectLinuxAgentUnit() string {
	// Prefer the canonical one-liner / current --install-service name.
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

// windowsPowerShellPath is a stub for non-Windows builds (CIM helpers compile
// everywhere but only run under GOOS=windows switches).
func windowsPowerShellPath() string { return "powershell" }
