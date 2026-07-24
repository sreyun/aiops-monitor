package main

import (
	"encoding/json"
	"strconv"
	"strings"
)

// installAuditOptions controls the cross-platform packet-observation block
// written by the one-line installer. Linux supports the native AF_PACKET
// backend; Windows/macOS use TShark over Npcap/libpcap/BPF.
type installAuditOptions struct {
	SNIEnabled                  bool
	SNIInterface                string
	CaptureBackend              string
	ContentAudit                bool
	ContentAuditPorts           string
	ContentAuditMaxBody         int
	ContentAuditBodyMode        string
	ContentAuditIncludeHosts    string
	ContentAuditExcludePaths    string
	ContentAuditMaxEventsPerMin int
}

// renderScript injects the server URL / token / category / serversJSON into an
// install template. Placeholders are used (not fmt) so the shell/PowerShell '%'
// and '$' characters pass through untouched. serversJSON is a pre-validated JSON
// array string (e.g. [{"server":"...","token":"..."}]); when empty the template
// falls back to the single server+token config.
func renderScript(tmpl, server, token, category, serversJSON, logPaths string) string {
	return renderScriptWithAudit(tmpl, server, token, category, serversJSON, logPaths, installAuditOptions{})
}

func renderScriptWithAudit(tmpl, server, token, category, serversJSON, logPaths string, audit installAuditOptions) string {
	if strings.TrimSpace(logPaths) == "" {
		logPaths = "[]" // 必须是合法 JSON 数组（同时是合法 YAML flow 序列），否则生成的 config.yaml 语法错误
	}
	if strings.TrimSpace(audit.ContentAuditPorts) == "" {
		audit.ContentAuditPorts = "[11434,8000,8080]"
	}
	if audit.ContentAuditMaxBody <= 0 {
		audit.ContentAuditMaxBody = 4096
	}
	if audit.CaptureBackend == "" {
		audit.CaptureBackend = "auto"
	}
	if audit.ContentAuditBodyMode == "" {
		audit.ContentAuditBodyMode = "redacted"
	}
	if audit.ContentAuditIncludeHosts == "" {
		audit.ContentAuditIncludeHosts = "[]"
	}
	if audit.ContentAuditExcludePaths == "" {
		audit.ContentAuditExcludePaths = `["/health*","/metrics*","/ready*","/live*"]`
	}
	if audit.ContentAuditMaxEventsPerMin <= 0 {
		audit.ContentAuditMaxEventsPerMin = 2000
	}
	return strings.NewReplacer(
		"__SERVER__", server,
		"__TOKEN__", token,
		"__CATEGORY__", category,
		"__SERVERS_JSON__", serversJSON,
		"__LOG_PATHS__", logPaths,
		"__SNI_ENABLED__", strconv.FormatBool(audit.SNIEnabled || audit.ContentAudit),
		"__SNI_INTERFACE__", audit.SNIInterface,
		"__CAPTURE_BACKEND__", audit.CaptureBackend,
		"__CONTENT_AUDIT__", strconv.FormatBool(audit.ContentAudit),
		"__CONTENT_AUDIT_PORTS__", audit.ContentAuditPorts,
		"__CONTENT_AUDIT_MAX_BODY__", strconv.Itoa(audit.ContentAuditMaxBody),
		"__CONTENT_AUDIT_BODY_MODE__", audit.ContentAuditBodyMode,
		"__CONTENT_AUDIT_INCLUDE_HOSTS__", audit.ContentAuditIncludeHosts,
		"__CONTENT_AUDIT_EXCLUDE_PATHS__", audit.ContentAuditExcludePaths,
		"__CONTENT_AUDIT_MAX_EVENTS__", strconv.Itoa(audit.ContentAuditMaxEventsPerMin),
	).Replace(tmpl)
}

// sanitizeLogPaths 把用户填写的日志路径（换行或逗号分隔）清洗为一个【合法 JSON 数组字符串】，
// 用于注入安装脚本生成的 config.yaml 的 log_paths 字段（JSON 数组同时是合法 YAML flow 序列）。
// 关键安全点：路径会被写进未加引号的 shell heredoc，若含 $ ` 等会被展开导致命令注入，
// 因此逐字符白名单（仅保留路径合法字符），再用 json.Marshal 正确转义。
func sanitizeLogPaths(raw string) string {
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' })
	var paths []string
	seen := map[string]bool{}
	for _, f := range fields {
		clean := strings.TrimSpace(strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
				return r
			case r == '/', r == '.', r == '_', r == '-', r == ':', r == '*', r == ' ', r == '\\':
				return r
			default:
				return -1 // 丢弃 $ ` " ; | & < > ( ) 等危险字符
			}
		}, strings.TrimSpace(f)))
		if clean == "" || seen[clean] {
			continue
		}
		if len(clean) > 256 {
			clean = clean[:256]
		}
		seen[clean] = true
		paths = append(paths, clean)
		if len(paths) >= 20 { // 上限 20 条，避免超长命令
			break
		}
	}
	if len(paths) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(paths)
	return string(b)
}

// sanitizeServersJSON parses a JSON array of {server,token} objects, sanitizes
// each URL, and re-serializes as compact JSON. Returns "" if input is empty or
// invalid — the install template then falls back to single-server config.
func sanitizeServersJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var entries []struct {
		Server string `json:"server"`
		Token  string `json:"token"`
	}
	if json.Unmarshal([]byte(raw), &entries) != nil || len(entries) == 0 {
		return ""
	}
	type clean struct {
		Server string `json:"server"`
		Token  string `json:"token,omitempty"`
	}
	out := make([]clean, 0, len(entries))
	for _, e := range entries {
		s := sanitizeServerURL(e.Server)
		if s == "" {
			continue
		}
		out = append(out, clean{Server: s, Token: sanitizeToken(e.Token)})
	}
	if len(out) == 0 {
		return ""
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// sanitizeAuditInstallOptions turns public install-script query parameters into
// a bounded, injection-safe configuration. Content capture implies the shared
// DNS/SNI collector on both native and TShark backends.
func sanitizeAuditInstallOptions(r map[string]string) installAuditOptions {
	on := func(v string) bool {
		v = strings.ToLower(strings.TrimSpace(v))
		return v == "1" || v == "true" || v == "yes" || v == "on"
	}
	iface := strings.Map(func(ch rune) rune {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9':
			return ch
		case ch == '-', ch == '_', ch == '.', ch == ':':
			return ch
		default:
			return -1
		}
	}, strings.TrimSpace(r["sni_interface"]))
	if len(iface) > 128 {
		iface = iface[:128]
	}

	seen := map[int]bool{}
	ports := make([]int, 0, 16)
	for _, raw := range strings.FieldsFunc(r["content_audit_ports"], func(ch rune) bool {
		return ch == ',' || ch == ';' || ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t'
	}) {
		p, err := strconv.Atoi(raw)
		if err != nil || p < 1 || p > 65535 || seen[p] {
			continue
		}
		seen[p] = true
		ports = append(ports, p)
		if len(ports) >= 32 {
			break
		}
	}
	if len(ports) == 0 {
		ports = []int{11434, 8000, 8080}
	}
	portJSON, _ := json.Marshal(ports)

	maxBody, _ := strconv.Atoi(strings.TrimSpace(r["content_audit_max_body"]))
	if maxBody == 0 {
		maxBody = 4096
	}
	if maxBody < 1024 {
		maxBody = 1024
	}
	if maxBody > 65536 {
		maxBody = 65536
	}
	backend := strings.ToLower(strings.TrimSpace(r["capture_backend"]))
	if backend != "native" && backend != "tshark" {
		backend = "auto"
	}
	bodyMode := strings.ToLower(strings.TrimSpace(r["content_audit_body_mode"]))
	if bodyMode != "metadata" && bodyMode != "full" {
		bodyMode = "redacted"
	}
	maxEvents, _ := strconv.Atoi(strings.TrimSpace(r["content_audit_max_events_per_min"]))
	if maxEvents <= 0 {
		maxEvents = 2000
	}
	if maxEvents > 100000 {
		maxEvents = 100000
	}
	includeHosts := sanitizeAuditPatternList(r["content_audit_include_hosts"], 64, 253)
	excludePaths := sanitizeAuditPatternList(r["content_audit_exclude_paths"], 64, 512)
	if strings.TrimSpace(r["content_audit_exclude_paths"]) == "" {
		excludePaths = `["/health*","/metrics*","/ready*","/live*"]`
	}
	contentAudit := on(r["content_audit"])
	return installAuditOptions{
		SNIEnabled:                  on(r["sni_enabled"]) || contentAudit,
		SNIInterface:                iface,
		CaptureBackend:              backend,
		ContentAudit:                contentAudit,
		ContentAuditPorts:           string(portJSON),
		ContentAuditMaxBody:         maxBody,
		ContentAuditBodyMode:        bodyMode,
		ContentAuditIncludeHosts:    includeHosts,
		ContentAuditExcludePaths:    excludePaths,
		ContentAuditMaxEventsPerMin: maxEvents,
	}
}

func sanitizeAuditPatternList(raw string, maxCount, maxLen int) string {
	fields := strings.FieldsFunc(raw, func(ch rune) bool {
		return ch == ',' || ch == ';' || ch == '\n' || ch == '\r'
	})
	out := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		clean := strings.ToLower(strings.TrimSpace(strings.Map(func(ch rune) rune {
			switch {
			case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9':
				return ch
			case ch == '.', ch == '-', ch == '_', ch == '*', ch == '/', ch == ':', ch == '?', ch == '=':
				return ch
			default:
				return -1
			}
		}, field)))
		if clean == "" || seen[clean] {
			continue
		}
		if len(clean) > maxLen {
			clean = clean[:maxLen]
		}
		seen[clean] = true
		out = append(out, clean)
		if len(out) >= maxCount {
			break
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// installShTemplate installs the agent on Linux / macOS. It works without root:
// as root it registers a systemd service, otherwise it installs under $HOME and
// starts in the background.
const installShTemplate = `#!/bin/sh
set -e
SERVER="__SERVER__"
TOKEN="__TOKEN__"
CATEGORY="__CATEGORY__"
if [ "$(id -u)" = "0" ]; then DIR="${AIOPS_DIR:-/opt/aiops-agent}"; else DIR="${AIOPS_DIR:-$HOME/.aiops-agent}"; fi

OS=$(uname -s)
ARCH=$(uname -m)
case "$OS" in
  Linux)
    case "$ARCH" in
      x86_64|amd64)   BIN="aiops-agent-linux-amd64" ;;
      aarch64|arm64)   BIN="aiops-agent-linux-arm64" ;;
      *)               BIN="aiops-agent-linux-amd64" ;;
    esac
    ;;
  Darwin)
    case "$ARCH" in
      arm64)           BIN="aiops-agent-darwin-arm64" ;;
      x86_64)          BIN="aiops-agent-darwin-amd64" ;;
      *)               BIN="aiops-agent-darwin-amd64" ;;
    esac
    ;;
  *) echo "unsupported OS: $OS"; exit 1 ;;
esac

echo "[AIOps] installing to $DIR (server $SERVER)"
mkdir -p "$DIR"
cd "$DIR"
# Download to a staging file, verify the server-published SHA-256, then replace
# atomically. A truncated/corrupted/mismatched download must never overwrite a
# working agent binary.
NEW=".aiops-agent.new"
curl -fSL --retry 3 --retry-delay 2 -C - "$SERVER/dl/$BIN" -o "$NEW" || curl -fsSL "$SERVER/dl/$BIN" -o "$NEW"
EXPECTED=$(curl -fsSL "$SERVER/dl/$BIN.sha256" | awk '{print $1}')
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "$NEW" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "$NEW" | awk '{print $1}')
else
  echo "[AIOps] ERROR: sha256sum/shasum not found; refusing an unverified install."
  rm -f "$NEW"
  exit 1
fi
if [ -z "$EXPECTED" ] || [ "$EXPECTED" != "$ACTUAL" ]; then
  echo "[AIOps] ERROR: agent SHA-256 verification failed."
  rm -f "$NEW"
  exit 1
fi
[ -f aiops-agent ] && cp -f aiops-agent aiops-agent.bak 2>/dev/null || true
mv -f "$NEW" aiops-agent
chmod +x aiops-agent
if curl -fsSL "$SERVER/dl/plugins.zip" -o plugins.zip 2>/dev/null; then
  command -v unzip >/dev/null 2>&1 && unzip -oq plugins.zip
  rm -f plugins.zip
fi
# config.example.yaml is written locally by the agent on first start
# (ensureConfigExample), so we don't fetch it here — that was a wasted 404.
# YAML is now the default/recommended config format. The JSON arrays injected
# below (servers / log_paths) are valid YAML flow syntax, so they drop straight in.
SERVERS_JSON='__SERVERS_JSON__'
if [ -n "$SERVERS_JSON" ]; then
  cat > config.yaml <<EOF
servers: $SERVERS_JSON
category: "$CATEGORY"
log_paths: __LOG_PATHS__
report_interval: 30
plugin_interval: 60
plugins_dir: "$DIR/plugins"
state_file: "$DIR/agent_state.json"
sni_dns_capture:
  enabled: __SNI_ENABLED__
  interface: "__SNI_INTERFACE__"
  capture_backend: "__CAPTURE_BACKEND__"
  max_entries_per_min: 5000
  tls_metadata_ports: [443,8443,9443]
  content_audit: __CONTENT_AUDIT__
  content_audit_ports: __CONTENT_AUDIT_PORTS__
  content_audit_max_body: __CONTENT_AUDIT_MAX_BODY__
  content_audit_body_mode: "__CONTENT_AUDIT_BODY_MODE__"
  content_audit_include_hosts: __CONTENT_AUDIT_INCLUDE_HOSTS__
  content_audit_exclude_paths: __CONTENT_AUDIT_EXCLUDE_PATHS__
  content_audit_max_events_per_min: __CONTENT_AUDIT_MAX_EVENTS__
EOF
else
  cat > config.yaml <<EOF
server: "$SERVER"
token: "$TOKEN"
category: "$CATEGORY"
log_paths: __LOG_PATHS__
report_interval: 30
plugin_interval: 60
plugins_dir: "$DIR/plugins"
state_file: "$DIR/agent_state.json"
sni_dns_capture:
  enabled: __SNI_ENABLED__
  interface: "__SNI_INTERFACE__"
  capture_backend: "__CAPTURE_BACKEND__"
  max_entries_per_min: 5000
  tls_metadata_ports: [443,8443,9443]
  content_audit: __CONTENT_AUDIT__
  content_audit_ports: __CONTENT_AUDIT_PORTS__
  content_audit_max_body: __CONTENT_AUDIT_MAX_BODY__
  content_audit_body_mode: "__CONTENT_AUDIT_BODY_MODE__"
  content_audit_include_hosts: __CONTENT_AUDIT_INCLUDE_HOSTS__
  content_audit_exclude_paths: __CONTENT_AUDIT_EXCLUDE_PATHS__
  content_audit_max_events_per_min: __CONTENT_AUDIT_MAX_EVENTS__
EOF
fi
# Verify config.yaml was written correctly — on some systems set -e causes the
# script to exit partway (e.g. plugins download failure) BEFORE reaching here,
# leaving config.yaml missing. The agent would then silently use the hardcoded
# default (localhost:8529). Catch this early so the user sees the real error.
if [ ! -s config.yaml ]; then
  echo "[AIOps] ERROR: config.yaml was not created! Installation incomplete."
  echo "[AIOps] This usually means a download step failed. Re-run the install command."
  exit 1
fi
# Restrict config.yaml to owner-only (contains tokens/secrets).
chmod 600 config.yaml 2>/dev/null || true
if [ "__SNI_ENABLED__" = "true" ] && { [ "__CAPTURE_BACKEND__" = "tshark" ] || [ "$OS" = "Darwin" ]; }; then
  if ! command -v tshark >/dev/null 2>&1 && [ ! -x /Applications/Wireshark.app/Contents/MacOS/tshark ]; then
    echo "[AIOps] WARNING: network content audit needs TShark on $OS."
    echo "[AIOps] Install Wireshark first; on macOS also install its ChmodBPF package."
  else
    echo "[AIOps] TShark dependency detected for cross-platform network audit."
  fi
fi
# Migrate: remove a stale config.json left by a pre-YAML install. The agent now
# prefers config.yaml, but leaving both would be confusing — drop the old one.
rm -f config.json 2>/dev/null || true
echo "[AIOps] config written: $DIR/config.yaml (server: $SERVER)"

if [ "$OS" = "Linux" ] && command -v systemctl >/dev/null 2>&1 && [ "$(id -u)" = "0" ]; then
  # Linux + root → systemd: auto-start on boot + auto-restart on crash/kill.
  cat > /etc/systemd/system/aiops-agent.service <<UNIT
[Unit]
Description=AIOps Monitor Agent
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
WorkingDirectory=$DIR
ExecStart=$DIR/aiops-agent --config $DIR/config.yaml
Restart=always
RestartSec=5
[Install]
WantedBy=multi-user.target
UNIT
  systemctl daemon-reload
  systemctl enable --now aiops-agent
  echo "[AIOps] systemd service started: aiops-agent (boot autostart + auto-restart)"
  # 麒麟/UOS 系统自动检测并配置 kysec 白名单
  if command -v kysec_adm &>/dev/null; then
    kysec_adm -a $DIR/aiops-agent 2>/dev/null && echo "[AIOps] kysec whitelist added: $DIR/aiops-agent" || true
  fi
  # SELinux: check and warn if enforcing
  if command -v getenforce &>/dev/null && [ "$(getenforce 2>/dev/null)" = "Enforcing" ]; then
    echo "[AIOps] WARNING: SELinux is enforcing. If agent data collection is blocked, run:"
    echo "  sudo setenforce 0  (temporary) or load a custom SELinux policy module."
  fi
elif [ "$OS" = "Darwin" ]; then
  # macOS → launchd. RunAtLoad starts it on boot/login; KeepAlive relaunches it
  # automatically if it ever exits or is killed. This fixes the previous macOS
  # behaviour (a one-off background process that never came back after a reboot).
  if [ "$(id -u)" = "0" ]; then PLIST_DIR="/Library/LaunchDaemons"; else PLIST_DIR="$HOME/Library/LaunchAgents"; fi
  mkdir -p "$PLIST_DIR"
  PLIST="$PLIST_DIR/com.aiops.agent.plist"
  cat > "$PLIST" <<PL
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.aiops.agent</string>
  <key>ProgramArguments</key>
  <array>
    <string>$DIR/aiops-agent</string>
    <string>--config</string>
    <string>$DIR/config.yaml</string>
  </array>
  <key>WorkingDirectory</key><string>$DIR</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>$DIR/agent.log</string>
  <key>StandardErrorPath</key><string>$DIR/agent.log</string>
</dict>
</plist>
PL
  # Strip the quarantine xattr: a curl-downloaded binary can carry
  # com.apple.quarantine, and after a reboot Gatekeeper blocks launchd from starting
  # a quarantined/unsigned binary — a prime cause of "monitoring dead after restart".
  xattr -dr com.apple.quarantine "$DIR/aiops-agent" 2>/dev/null || true
  UIDN=$(id -u)
  launchctl unload "$PLIST" 2>/dev/null || true
  if [ "$UIDN" = "0" ]; then
    # System LaunchDaemon: starts at boot regardless of login. Prefer the modern
    # bootstrap API, fall back to legacy load -w on older macOS.
    launchctl bootout system "$PLIST" 2>/dev/null || true
    launchctl bootstrap system "$PLIST" 2>/dev/null || launchctl load -w "$PLIST" 2>/dev/null || launchctl load "$PLIST" 2>/dev/null || true
    echo "[AIOps] launchd LaunchDaemon installed: com.aiops.agent (starts at boot + keepalive)"
  else
    # Per-user LaunchAgent: bootstrap + ENABLE so the enabled state survives a reboot
    # (load -w alone can lose it on newer macOS); kickstart starts it right now.
    launchctl bootout "gui/$UIDN" "$PLIST" 2>/dev/null || true
    launchctl bootstrap "gui/$UIDN" "$PLIST" 2>/dev/null || launchctl load -w "$PLIST" 2>/dev/null || launchctl load "$PLIST" 2>/dev/null || true
    launchctl enable "gui/$UIDN/com.aiops.agent" 2>/dev/null || true
    launchctl kickstart "gui/$UIDN/com.aiops.agent" 2>/dev/null || true
    echo "[AIOps] launchd LaunchAgent installed: com.aiops.agent (starts at login + keepalive)"
    echo "[AIOps] NOTE: a per-user agent starts only after LOGIN. For a headless Mac that"
    echo "[AIOps] must collect before anyone logs in, re-run the installer with sudo."
  fi
else
  # Fallback (non-root Linux without systemd): run now + a @reboot crontab entry
  # so it survives reboots. root+systemd is recommended for restart-on-crash too.
  pkill -f "$DIR/aiops-agent" 2>/dev/null || true
  nohup "$DIR/aiops-agent" --config "$DIR/config.yaml" > "$DIR/agent.log" 2>&1 &
  if command -v crontab >/dev/null 2>&1; then
    ( crontab -l 2>/dev/null | grep -v "$DIR/aiops-agent --config" ; \
      echo "@reboot $DIR/aiops-agent --config $DIR/config.yaml >> $DIR/agent.log 2>&1" ) | crontab - 2>/dev/null || true
    echo "[AIOps] started in background + @reboot autostart added (log: $DIR/agent.log)"
  else
    echo "[AIOps] started in background (log: $DIR/agent.log)"
  fi
fi
echo "[AIOps] done. Check the dashboard for this host."
`

// installPs1Template installs the agent on Windows, privilege-adaptive:
//   - Run ELEVATED (admin): installs under %ProgramData% and registers a
//     scheduled task running as SYSTEM at Highest run level (boot + 5-min
//     keepalive). SYSTEM has the privileges Get-VM needs, so Hyper-V guest
//     collection works. This is the mode Hyper-V hosts must use.
//   - Run NON-elevated: the classic per-user install under %LOCALAPPDATA%
//     (HKCU Run + 5-min keepalive), unchanged. No admin required, but it
//     cannot collect Hyper-V guests — the script says so and points at the
//     elevated re-run.
//
// config.yaml is UTF-8 (no BOM); the agent is launched via a hidden VBS
// supervisor that only starts it when not already running (no duplicates).
const installPs1Template = `$ErrorActionPreference = "Stop"
# Force TLS 1.2 before any download. Windows Server 2012/2016 default Invoke-WebRequest
# to TLS 1.0, which fails against a TLS1.2-only HTTPS server ("Could not create SSL/TLS
# secure channel") — a very common Windows install failure. Numeric 3072 = Tls12 avoids
# an enum-undefined error on older .NET where the Tls12 name isn't defined.
try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor 3072 } catch {}
$Server   = "__SERVER__"
$Token    = "__TOKEN__"
$Category = "__CATEGORY__"
$LogPaths = '__LOG_PATHS__'
$ServersJson = '__SERVERS_JSON__'
# Elevated installs run the agent as SYSTEM (needed for Hyper-V Get-VM) and live
# machine-wide under ProgramData; non-elevated installs stay per-user as before.
$IsAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltinRole]::Administrator)

# Hyper-V auto-elevation: Get-VM needs admin/SYSTEM, so a per-user install on a
# Hyper-V host silently collects ZERO VMs. When NOT elevated AND this is a Hyper-V
# host, relaunch the SAME install elevated via UAC — the elevated run installs as
# SYSTEM and collects Hyper-V. Non-Hyper-V hosts keep the no-admin per-user install
# untouched. The command is passed as -EncodedCommand (base64 UTF-16LE) to dodge all
# quoting pitfalls. If UAC is declined or unavailable (headless), we fall through to
# the per-user install below, so this can only help, never block.
if (-not $IsAdmin -and (Get-Command Get-VM -ErrorAction SilentlyContinue)) {
  Write-Host "[AIOps] Hyper-V host detected but PowerShell is not elevated."
  Write-Host "[AIOps] Requesting administrator rights (UAC) so Hyper-V VM collection works..."
  try {
    $q = "token=" + [Uri]::EscapeDataString([string]$Token)
    if ($Category) { $q += "&category=" + [Uri]::EscapeDataString([string]$Category) }
    if ($LogPaths -and $LogPaths -ne "[]") { $q += "&log_paths=" + [Uri]::EscapeDataString([string]$LogPaths) }
    if ($ServersJson) { $q += "&servers_json=" + [Uri]::EscapeDataString([string]$ServersJson) }
    $reinvoke = '[Net.ServicePointManager]::SecurityProtocol=[Net.ServicePointManager]::SecurityProtocol -bor 3072; irm "' + $Server + '/install.ps1?' + $q + '" | iex'
    $enc = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($reinvoke))
    Start-Process powershell -Verb RunAs -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-EncodedCommand',$enc -ErrorAction Stop
    Write-Host "[AIOps] Elevated installer launched in a new window (approve the UAC prompt). This non-admin window is done."
    return
  } catch {
    Write-Host "[AIOps] Elevation declined or unavailable; continuing with a per-user install."
    Write-Host "[AIOps] NOTE: Hyper-V VM collection stays OFF until you re-run this command in an ELEVATED PowerShell."
  }
}
if ($IsAdmin) { $Dir = Join-Path $env:ProgramData "aiops-agent" } else { $Dir = Join-Path $env:LOCALAPPDATA "aiops-agent" }

Write-Host "[AIOps] installing to $Dir (server $Server, admin=$IsAdmin)"
New-Item -ItemType Directory -Force $Dir | Out-Null

# Stop any prior instance BEFORE downloading. A running agent holds aiops-agent.exe
# locked, so downloading over it fails — on Server 2012 (no curl.exe) Invoke-WebRequest
# throws and $ErrorActionPreference=Stop aborts the whole install. This is the #1 cause
# of re-install failures, esp. on Hyper-V hosts that re-run the elevated install while
# the 5-min keepalive is running. Delete the task first so it can't relaunch mid-install,
# kill the process, then wait for the file handle to release. cmd /c avoids the PS 5.1
# NativeCommandError when the task doesn't exist yet.
cmd /c 'schtasks /Delete /TN "AIOpsAgent" /F 2>nul'
Get-Process aiops-agent -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 800

# Prefer curl.exe (bundled on Win10+): supports resume (-C -) + retry so a flaky
# link doesn't restart the whole 7.5MB. Fall back to Invoke-WebRequest on older OS.
$AgentExe = Join-Path $Dir "aiops-agent.exe"
$AgentNew = Join-Path $Dir ".aiops-agent.new.exe"
$AgentBak = Join-Path $Dir "aiops-agent.exe.bak"
$Curl = Get-Command curl.exe -ErrorAction SilentlyContinue
if ($Curl) {
  & curl.exe -fSL --retry 3 --retry-delay 2 -C - "$Server/dl/aiops-agent.exe" -o $AgentNew
  if ($LASTEXITCODE -ne 0) { & curl.exe -fsSL "$Server/dl/aiops-agent.exe" -o $AgentNew }
} else {
  Invoke-WebRequest "$Server/dl/aiops-agent.exe" -OutFile $AgentNew -UseBasicParsing
}
$Expected = ((Invoke-WebRequest "$Server/dl/aiops-agent.exe.sha256" -UseBasicParsing).Content -split '\s+')[0].Trim().ToLowerInvariant()
$Sha = [Security.Cryptography.SHA256]::Create()
$Stream = [IO.File]::OpenRead($AgentNew)
try { $Actual = ([BitConverter]::ToString($Sha.ComputeHash($Stream))).Replace("-","").ToLowerInvariant() }
finally { $Stream.Dispose(); $Sha.Dispose() }
if (-not $Expected -or $Expected -ne $Actual) {
  Remove-Item $AgentNew -Force -ErrorAction SilentlyContinue
  throw "Agent SHA-256 verification failed; existing binary was not replaced."
}
if (Test-Path $AgentBak) { Remove-Item $AgentBak -Force -ErrorAction SilentlyContinue }
if (Test-Path $AgentExe) { Move-Item $AgentExe $AgentBak -Force }
try { Move-Item $AgentNew $AgentExe -Force }
catch {
  if (Test-Path $AgentBak) { Move-Item $AgentBak $AgentExe -Force }
  throw
}
try {
  Invoke-WebRequest "$Server/dl/plugins.zip" -OutFile "$Dir\plugins.zip" -UseBasicParsing
  Expand-Archive -Path "$Dir\plugins.zip" -DestinationPath $Dir -Force
  Remove-Item "$Dir\plugins.zip" -Force
} catch { Write-Host "[AIOps] plugins skipped" }

$ServersJson = '__SERVERS_JSON__'
# YAML is the default config format. PowerShell has no YAML serializer, so build it
# by hand: scalar values are single-quoted (backslash-safe for Windows paths; any
# embedded single-quote is doubled per YAML rules), while the injected JSON arrays
# (servers / log_paths) are already valid YAML flow syntax and drop in as-is.
function Yq($s) { "'" + (([string]$s) -replace "'", "''") + "'" }
$LogPathsYaml = if ($LogPaths -and $LogPaths.Trim() -ne "") { $LogPaths } else { "[]" }
$PluginsDir = Join-Path $Dir "plugins"
$StateFile  = Join-Path $Dir "agent_state.json"
$lines = New-Object System.Collections.Generic.List[string]
if ($ServersJson -ne "") {
  $lines.Add("servers: $ServersJson")
} else {
  $lines.Add("server: " + (Yq $Server))
  $lines.Add("token: " + (Yq $Token))
}
$lines.Add("category: " + (Yq $Category))
$lines.Add("log_paths: $LogPathsYaml")
$lines.Add("report_interval: 30")
$lines.Add("plugin_interval: 60")
$lines.Add("plugins_dir: " + (Yq $PluginsDir))
$lines.Add("state_file: " + (Yq $StateFile))
$lines.Add("sni_dns_capture:")
$lines.Add("  enabled: __SNI_ENABLED__")
$lines.Add("  interface: " + (Yq "__SNI_INTERFACE__"))
$lines.Add("  capture_backend: " + (Yq "__CAPTURE_BACKEND__"))
$lines.Add("  max_entries_per_min: 5000")
$lines.Add("  tls_metadata_ports: [443,8443,9443]")
$lines.Add("  content_audit: __CONTENT_AUDIT__")
$lines.Add("  content_audit_ports: __CONTENT_AUDIT_PORTS__")
$lines.Add("  content_audit_max_body: __CONTENT_AUDIT_MAX_BODY__")
$lines.Add("  content_audit_body_mode: " + (Yq "__CONTENT_AUDIT_BODY_MODE__"))
$lines.Add("  content_audit_include_hosts: __CONTENT_AUDIT_INCLUDE_HOSTS__")
$lines.Add("  content_audit_exclude_paths: __CONTENT_AUDIT_EXCLUDE_PATHS__")
$lines.Add("  content_audit_max_events_per_min: __CONTENT_AUDIT_MAX_EVENTS__")
$cfg = ($lines -join ([char]10)) + ([char]10)
[System.IO.File]::WriteAllText("$Dir\config.yaml", $cfg, (New-Object System.Text.UTF8Encoding $false))
if ("__SNI_ENABLED__" -eq "true") {
  $TSharkCandidates = @(
    (Get-Command tshark.exe -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Source -ErrorAction SilentlyContinue),
    (Join-Path $env:ProgramFiles "Wireshark\tshark.exe")
  ) | Where-Object { $_ -and (Test-Path $_) }
  if (-not $TSharkCandidates) {
    Write-Warning "Network content audit needs Wireshark TShark and Npcap. Install both, then restart aiops-agent."
  } else {
    Write-Host "[AIOps] TShark/Npcap audit dependency detected." -ForegroundColor Green
  }
}
# Migrate: remove a stale config.json from a pre-YAML install (agent now prefers YAML).
Remove-Item "$Dir\config.json" -Force -ErrorAction SilentlyContinue

# User-level autostart + keepalive (no admin required).
# start-agent.vbs is a *supervisor*: it launches the agent ONLY if it is not
# already running, so neither the logon Run key nor the 5-minute keepalive task
# ever spawns a duplicate. Two triggers together mean the agent survives both a
# reboot (Run key at logon) and being stopped/killed (task relaunches within 5m).
$exe  = "$Dir\aiops-agent.exe"
$conf = "$Dir\config.yaml"
$vbs  = "$Dir\start-agent.vbs"
$runLine = 'CreateObject("WScript.Shell").Run """' + $exe + '"" --config ""' + $conf + '""", 0, False'
$vbsBody = @"
' AIOps agent supervisor — start the agent only if it is not already running.
Dim running : running = False
On Error Resume Next
Dim wmi : Set wmi = GetObject("winmgmts:{impersonationLevel=impersonate}!\\.\root\cimv2")
Dim procs : Set procs = wmi.ExecQuery("SELECT ProcessId FROM Win32_Process WHERE Name = 'aiops-agent.exe'")
If Not procs Is Nothing Then If procs.Count > 0 Then running = True
On Error GoTo 0
If Not running Then $runLine
"@
[System.IO.File]::WriteAllText($vbs, $vbsBody, (New-Object System.Text.UTF8Encoding $false))

# (Prior instance already stopped + task deleted before the download above.)
if ($IsAdmin) {
  # Elevated: run the keepalive task as SYSTEM at Highest run level so Get-VM
  # (Hyper-V guest collection) has the privileges it needs. Same proven 5-minute
  # schtasks keepalive as per-user mode, just under the SYSTEM account; schtasks
  # /Run starts it immediately. The HKCU Run key is irrelevant to SYSTEM, drop it.
  Remove-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "AIOpsAgent" -ErrorAction SilentlyContinue
  $trTask = 'wscript.exe \"' + $vbs + '\"'
  # Continue-guard: schtasks writing to stderr can raise a NativeCommandError under
  # $ErrorActionPreference=Stop (PS 5.1) and abort the install even on success.
  $eap = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
  schtasks /Create /TN "AIOpsAgent" /TR $trTask /SC MINUTE /MO 5 /RU SYSTEM /RL HIGHEST /F 2>$null | Out-Null
  schtasks /Run /TN "AIOpsAgent" 2>$null | Out-Null
  $ErrorActionPreference = $eap
  Write-Host "[AIOps] installed as SYSTEM (elevated), 5-min keepalive. Hyper-V collection enabled."
} else {
  # Non-elevated: classic per-user autostart (unchanged). Works without admin but
  # CANNOT collect Hyper-V guests -- Get-VM needs elevation.
  New-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "AIOpsAgent" -Value ('wscript.exe "' + $vbs + '"') -PropertyType String -Force | Out-Null
  $trTask = 'wscript.exe \"' + $vbs + '\"'
  $eap = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
  schtasks /Create /TN "AIOpsAgent" /TR $trTask /SC MINUTE /MO 5 /F 2>$null | Out-Null
  $ErrorActionPreference = $eap
  Start-Process "wscript.exe" -ArgumentList ('"' + $vbs + '"')
  Write-Host "[AIOps] installed (user-level, no admin). Check the dashboard."
  Write-Host "[AIOps] NOTE: Hyper-V VM collection needs admin. On a Hyper-V host, re-run this install command in an ELEVATED PowerShell."
}
`

// relayInstallShTemplate installs the agent in GATEWAY RELAY mode on Linux /
// macOS. The relay listens on a local port and reverse-proxies all requests to
// the cloud server — internal machines that can't reach the internet point their
// agents at this relay instead. Only the gateway machine needs internet access.
const relayInstallShTemplate = `#!/bin/sh
set -e
SERVER="__SERVER__"
LISTEN="${RELAY_LISTEN:-:8529}"
if [ "$(id -u)" = "0" ]; then DIR="${AIOPS_DIR:-/opt/aiops-agent}"; else DIR="${AIOPS_DIR:-$HOME/.aiops-agent}"; fi

OS=$(uname -s)
ARCH=$(uname -m)
case "$OS" in
  Linux)
    case "$ARCH" in
      x86_64|amd64)   BIN="aiops-agent-linux-amd64" ;;
      aarch64|arm64)   BIN="aiops-agent-linux-arm64" ;;
      *)               BIN="aiops-agent-linux-amd64" ;;
    esac
    ;;
  Darwin)
    case "$ARCH" in
      arm64)           BIN="aiops-agent-darwin-arm64" ;;
      x86_64)          BIN="aiops-agent-darwin-amd64" ;;
      *)               BIN="aiops-agent-darwin-amd64" ;;
    esac
    ;;
  *) echo "unsupported OS: $OS"; exit 1 ;;
esac

echo "[AIOps] installing relay to $DIR (upstream $SERVER)"
mkdir -p "$DIR"
cd "$DIR"
# resumable + retried download: on flaky/cross-border links, don't re-fetch the
# whole 7.5MB from scratch. -C - resumes a partial; on a complete file the server
# returns 416, so fall back to a plain full GET.
curl -fSL --retry 3 --retry-delay 2 -C - "$SERVER/dl/$BIN" -o aiops-agent || curl -fsSL "$SERVER/dl/$BIN" -o aiops-agent
chmod +x aiops-agent

cat > config.yaml <<EOF
relay: true
listen: "$LISTEN"
server: "$SERVER"
EOF
if [ ! -s config.yaml ]; then
  echo "[AIOps] ERROR: config.yaml was not created! Installation incomplete."
  exit 1
fi
rm -f config.json 2>/dev/null || true
echo "[AIOps] config written: $DIR/config.yaml (upstream: $SERVER, listen: $LISTEN)"

if command -v systemctl >/dev/null 2>&1 && [ "$(id -u)" = "0" ]; then
  cat > /etc/systemd/system/aiops-relay.service <<UNIT
[Unit]
Description=AIOps Monitor Relay
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
WorkingDirectory=$DIR
ExecStart=$DIR/aiops-agent --config $DIR/config.yaml
Restart=always
RestartSec=5
[Install]
WantedBy=multi-user.target
UNIT
  systemctl daemon-reload
  systemctl enable --now aiops-relay
  echo "[AIOps] relay systemd service started: aiops-relay (listen $LISTEN)"
else
  pkill -f "$DIR/aiops-agent.*relay" 2>/dev/null || true
  nohup "$DIR/aiops-agent" --config "$DIR/config.yaml" > "$DIR/relay.log" 2>&1 &
  echo "[AIOps] relay started in background (log: $DIR/relay.log)"
fi
RELAY_PORT="${LISTEN##*:}"
echo ""
echo "[AIOps] Relay ready! Internal machines install with:"
echo "  curl -fsSL http://<this-host-ip>:${RELAY_PORT}/install.sh | sh"
`

// relayInstallPs1Template installs the agent in GATEWAY RELAY mode on Windows.
const relayInstallPs1Template = `$ErrorActionPreference = "Stop"
# Force TLS 1.2 (numeric 3072) so downloads work on Server 2012/2016 which default to TLS 1.0.
try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor 3072 } catch {}
$Server = "__SERVER__"
$Listen = if ($env:RELAY_LISTEN) { $env:RELAY_LISTEN } else { ":8529" }
$Dir    = Join-Path $env:LOCALAPPDATA "aiops-agent"

Write-Host "[AIOps] installing relay to $Dir (upstream $Server)"
New-Item -ItemType Directory -Force $Dir | Out-Null
# Stop a prior relay/agent before downloading so the running exe doesn't hold the
# file locked (which would make Invoke-WebRequest throw and abort the install).
Get-Process aiops-agent -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 800
Invoke-WebRequest "$Server/dl/aiops-agent.exe" -OutFile "$Dir\aiops-agent.exe" -UseBasicParsing

# YAML is the default config format (single-quoted scalars are backslash-safe; any
# embedded single-quote is doubled per YAML rules). No YAML serializer in PowerShell.
$RelayLines = New-Object System.Collections.Generic.List[string]
$RelayLines.Add("relay: true")
$RelayLines.Add("listen: '" + (([string]$Listen) -replace "'", "''") + "'")
$RelayLines.Add("server: '" + (([string]$Server) -replace "'", "''") + "'")
$cfg = ($RelayLines -join ([char]10)) + ([char]10)
[System.IO.File]::WriteAllText("$Dir\config.yaml", $cfg, (New-Object System.Text.UTF8Encoding $false))
# Migrate: remove a stale config.json from a pre-YAML install (agent now prefers YAML).
Remove-Item "$Dir\config.json" -Force -ErrorAction SilentlyContinue

$exe  = "$Dir\aiops-agent.exe"
$conf = "$Dir\config.yaml"
$vbs  = "$Dir\start-relay.vbs"
$line = 'CreateObject("WScript.Shell").Run """' + $exe + '"" --config ""' + $conf + '""", 0, False'
[System.IO.File]::WriteAllText($vbs, $line, (New-Object System.Text.ASCIIEncoding))
New-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "AIOpsRelay" -Value ('wscript.exe "' + $vbs + '"') -PropertyType String -Force | Out-Null

Get-Process aiops-agent -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Process "wscript.exe" -ArgumentList ('"' + $vbs + '"')
$Port = $Listen -replace '.*:', ''
Write-Host "[AIOps] relay installed and started (listen $Listen)"
Write-Host "[AIOps] internal machines use: http://<this-host-ip>:$Port"
`

// uninstallShTemplate stops + removes the agent on Linux / macOS.
const uninstallShTemplate = `#!/bin/sh
if [ "$(id -u)" = "0" ]; then DIR="${AIOPS_DIR:-/opt/aiops-agent}"; else DIR="${AIOPS_DIR:-$HOME/.aiops-agent}"; fi
echo "[AIOps] uninstalling from $DIR"
if command -v systemctl >/dev/null 2>&1; then
  systemctl disable --now aiops-agent 2>/dev/null || true
  rm -f /etc/systemd/system/aiops-agent.service
  systemctl daemon-reload 2>/dev/null || true
fi
# launchd (macOS): remove both the per-user LaunchAgent and the root LaunchDaemon.
for PLIST in "$HOME/Library/LaunchAgents/com.aiops.agent.plist" "/Library/LaunchDaemons/com.aiops.agent.plist"; do
  if [ -f "$PLIST" ]; then
    launchctl unload "$PLIST" 2>/dev/null || true
    rm -f "$PLIST"
  fi
done
# Remove the @reboot crontab entry added by the non-root fallback install.
if command -v crontab >/dev/null 2>&1; then
  crontab -l 2>/dev/null | grep -v "$DIR/aiops-agent --config" | crontab - 2>/dev/null || true
fi
pkill -f "$DIR/aiops-agent" 2>/dev/null || true
rm -rf "$DIR"
echo "[AIOps] uninstalled. You may delete the host card in the dashboard."
`

// uninstallPs1Template stops + removes the agent on Windows (user-level).
// v5.2.6: Comprehensive rewrite to fix multiple uninstall failures.
// v5.2.9: Regression fixes:
//  1. Replace Get-CimInstance (unreliable CommandLine) with taskkill / Get-Process
//  2. Kill ALL wscript.exe instances (safe on uninstall — no other apps use it)
//  3. Kill ALL aiops-agent.exe instances by name
//  4. Add $ErrorActionPreference = "Continue" for error visibility
//  5. Longer retry delays (2/4/8s) and MoveFileEx for stubborn files
//  6. Explicitly delete VBS files before EXE to release Run registry triggers
const uninstallPs1Template = `$ErrorActionPreference = "Continue"
# Clean both install locations: per-user (%LOCALAPPDATA%) and elevated
# (%ProgramData%). Removing a SYSTEM install's task/dir needs an elevated shell.
$Dirs = @((Join-Path $env:LOCALAPPDATA "aiops-agent"), (Join-Path $env:ProgramData "aiops-agent"))
Write-Host "[AIOps] uninstalling ($($Dirs -join '; '))"

# An elevated (SYSTEM) install registers its keepalive task + files machine-wide.
# Removing a SYSTEM task and %ProgramData% needs admin: without it, schtasks /Delete
# is access-denied (silently), the task relaunches the agent within 5 min, and the
# file deletion below fails — the classic "uninstall didn't work". Warn up front.
$IsAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltinRole]::Administrator)
$ProgramDataDir = Join-Path $env:ProgramData "aiops-agent"
if (-not $IsAdmin -and (Test-Path $ProgramDataDir)) {
    Write-Host "[AIOps] WARNING: an elevated (SYSTEM) install exists at $ProgramDataDir."
    Write-Host "[AIOps] Its SYSTEM scheduled task CANNOT be removed without admin and will relaunch"
    Write-Host "[AIOps] the agent within 5 minutes. Re-run this uninstall in an ELEVATED PowerShell"
    Write-Host "[AIOps] (Run as Administrator) to fully remove it."
}

# Step 1: Remove ALL autostart entries (normal + relay modes)
Remove-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "AIOpsAgent" -ErrorAction SilentlyContinue
Remove-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "AIOpsRelay" -ErrorAction SilentlyContinue

# Step 2: Remove the keepalive scheduled task FIRST — otherwise it relaunches the
# agent within 5 minutes and the file deletion below fails ("can't uninstall").
# Delete both the current name and the legacy hyphenated one.
cmd /c 'schtasks /Delete /TN "AIOpsAgent" /F 2>nul'
cmd /c 'schtasks /Delete /TN "AIOps-Agent" /F 2>nul'

# Step 3: Kill ALL related processes — agent + VBS launcher
# v5.2.9: Use taskkill + Get-Process instead of Get-CimInstance.
# Get-CimInstance Win32_Process.CommandLine is unreliable (may be null)
# and silently fails to match, leaving wscript.exe running.
# On uninstall it is safe to kill ALL wscript.exe instances — no other
# common Windows application uses wscript.exe for persistent launchers.
& taskkill /F /IM aiops-agent.exe 2>$null | Out-Null
Get-Process aiops-agent -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
& taskkill /F /IM wscript.exe 2>$null | Out-Null
Get-Process wscript -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue

# Step 4: Wait for process handles to release (increased to 3s)
Start-Sleep -Seconds 3

# Step 5: Delete files with retry logic (handles stubborn file locks), for BOTH
# install locations. Delete VBS files FIRST -- removing them prevents wscript.exe
# from being relaunched by the Run registry.
$files = @("start-agent.vbs", "start-relay.vbs", "aiops-agent.exe", "config.yaml", "config.json", "agent_state.json", "agent.log", "plugins.zip")
foreach ($Dir in $Dirs) {
    foreach ($f in $files) {
        $path = Join-Path $Dir $f
        if (Test-Path $path) { Remove-Item $path -Force -ErrorAction SilentlyContinue }
    }
    $pluginsDir = Join-Path $Dir "plugins"
    if (Test-Path $pluginsDir) { Remove-Item -Recurse -Force $pluginsDir -ErrorAction SilentlyContinue }
    for ($i = 2; $i -le 8; $i *= 2) {
        if (Test-Path $Dir) { Remove-Item -Recurse -Force $Dir -ErrorAction SilentlyContinue }
        if (-not (Test-Path $Dir)) { break }
        Start-Sleep -Seconds $i
    }
}

# Last resort -- schedule deletion of any still-locked dir on next reboot.
$stuck = @($Dirs | Where-Object { Test-Path $_ })
if ($stuck.Count -gt 0) {
    Write-Host "[AIOps] scheduling cleanup on next reboot for: $($stuck -join '; ')"
    $bat = Join-Path $env:TEMP "aiops-uninstall.bat"
    $sb = New-Object System.Text.StringBuilder
    [void]$sb.AppendLine("@echo off")
    [void]$sb.AppendLine(":retry")
    [void]$sb.AppendLine("timeout /t 5 /nobreak >nul")
    foreach ($d in $stuck) { [void]$sb.AppendLine('rmdir /s /q "' + $d + '" 2>nul') }
    foreach ($d in $stuck) { [void]$sb.AppendLine('if exist "' + $d + '" goto retry') }
    [void]$sb.AppendLine('del "%~f0" 2>nul')
    [System.IO.File]::WriteAllText($bat, $sb.ToString(), (New-Object System.Text.ASCIIEncoding))
    New-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\RunOnce" -Name "AIOpsCleanup" -Value ("cmd.exe /c " + $bat) -PropertyType String -Force | Out-Null
    Write-Host "[AIOps] warning: some files could not be deleted. Cleanup will finish on next reboot."
} else {
    Write-Host "[AIOps] uninstalled. You may delete the host card in the dashboard."
}
`
