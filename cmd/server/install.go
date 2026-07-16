package main

import (
	"encoding/json"
	"strings"
)

// renderScript injects the server URL / token / category / serversJSON into an
// install template. Placeholders are used (not fmt) so the shell/PowerShell '%'
// and '$' characters pass through untouched. serversJSON is a pre-validated JSON
// array string (e.g. [{"server":"...","token":"..."}]); when empty the template
// falls back to the single server+token config.
func renderScript(tmpl, server, token, category, serversJSON, logPaths string) string {
	if strings.TrimSpace(logPaths) == "" {
		logPaths = "[]" // 必须是合法 JSON 数组，否则生成的 config.json 语法错误
	}
	return strings.NewReplacer(
		"__SERVER__", server,
		"__TOKEN__", token,
		"__CATEGORY__", category,
		"__SERVERS_JSON__", serversJSON,
		"__LOG_PATHS__", logPaths,
	).Replace(tmpl)
}

// sanitizeLogPaths 把用户填写的日志路径（换行或逗号分隔）清洗为一个【合法 JSON 数组字符串】，
// 用于注入安装脚本生成的 config.json 的 log_paths 字段。
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
curl -fsSL "$SERVER/dl/$BIN" -o aiops-agent
chmod +x aiops-agent
if curl -fsSL "$SERVER/dl/plugins.zip" -o plugins.zip 2>/dev/null; then
  command -v unzip >/dev/null 2>&1 && unzip -oq plugins.zip
  rm -f plugins.zip
fi
SERVERS_JSON='__SERVERS_JSON__'
if [ -n "$SERVERS_JSON" ]; then
  cat > config.json <<EOF
{
  "servers": $SERVERS_JSON,
  "category": "$CATEGORY",
  "log_paths": __LOG_PATHS__,
  "report_interval": 10,
  "plugin_interval": 15,
  "plugins_dir": "$DIR/plugins",
  "state_file": "$DIR/agent_state.json"
}
EOF
else
  cat > config.json <<EOF
{
  "server": "$SERVER",
  "token": "$TOKEN",
  "category": "$CATEGORY",
  "log_paths": __LOG_PATHS__,
  "report_interval": 10,
  "plugin_interval": 15,
  "plugins_dir": "$DIR/plugins",
  "state_file": "$DIR/agent_state.json"
}
EOF
fi

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
ExecStart=$DIR/aiops-agent --config $DIR/config.json
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
    <string>$DIR/config.json</string>
  </array>
  <key>WorkingDirectory</key><string>$DIR</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>$DIR/agent.log</string>
  <key>StandardErrorPath</key><string>$DIR/agent.log</string>
</dict>
</plist>
PL
  launchctl unload "$PLIST" 2>/dev/null || true
  launchctl load -w "$PLIST" 2>/dev/null || launchctl load "$PLIST"
  echo "[AIOps] launchd job installed: com.aiops.agent (autostart on boot + keepalive)"
else
  # Fallback (non-root Linux without systemd): run now + a @reboot crontab entry
  # so it survives reboots. root+systemd is recommended for restart-on-crash too.
  pkill -f "$DIR/aiops-agent" 2>/dev/null || true
  nohup "$DIR/aiops-agent" --config "$DIR/config.json" > "$DIR/agent.log" 2>&1 &
  if command -v crontab >/dev/null 2>&1; then
    ( crontab -l 2>/dev/null | grep -v "$DIR/aiops-agent --config" ; \
      echo "@reboot $DIR/aiops-agent --config $DIR/config.json >> $DIR/agent.log 2>&1" ) | crontab - 2>/dev/null || true
    echo "[AIOps] started in background + @reboot autostart added (log: $DIR/agent.log)"
  else
    echo "[AIOps] started in background (log: $DIR/agent.log)"
  fi
fi
echo "[AIOps] done. Check the dashboard for this host."
`

// installPs1Template installs the agent on Windows WITHOUT requiring admin:
// it installs under %LOCALAPPDATA%, writes config.json (UTF-8, no BOM) and
// registers a user-level autostart (HKCU Run) via a hidden VBS launcher.
const installPs1Template = `$ErrorActionPreference = "Stop"
$Server   = "__SERVER__"
$Token    = "__TOKEN__"
$Category = "__CATEGORY__"
$LogPaths = '__LOG_PATHS__'
$Dir      = Join-Path $env:LOCALAPPDATA "aiops-agent"

Write-Host "[AIOps] installing to $Dir (server $Server)"
New-Item -ItemType Directory -Force $Dir | Out-Null
Invoke-WebRequest "$Server/dl/aiops-agent.exe" -OutFile "$Dir\aiops-agent.exe" -UseBasicParsing
try {
  Invoke-WebRequest "$Server/dl/plugins.zip" -OutFile "$Dir\plugins.zip" -UseBasicParsing
  Expand-Archive -Path "$Dir\plugins.zip" -DestinationPath $Dir -Force
  Remove-Item "$Dir\plugins.zip" -Force
} catch { Write-Host "[AIOps] plugins skipped" }

$ServersJson = '__SERVERS_JSON__'
if ($ServersJson -ne "") {
  $cfg = @{
    servers = ($ServersJson | ConvertFrom-Json)
    category = $Category
    log_paths = @($LogPaths | ConvertFrom-Json)
    report_interval = 10
    plugin_interval = 15
    plugins_dir = "$Dir\plugins"
    state_file = "$Dir\agent_state.json"
  } | ConvertTo-Json -Depth 3
} else {
  $cfg = @{
    server = $Server
    token = $Token
    category = $Category
    log_paths = @($LogPaths | ConvertFrom-Json)
    report_interval = 10
    plugin_interval = 15
    plugins_dir = "$Dir\plugins"
    state_file = "$Dir\agent_state.json"
  } | ConvertTo-Json
}
[System.IO.File]::WriteAllText("$Dir\config.json", $cfg, (New-Object System.Text.UTF8Encoding $false))

# User-level autostart + keepalive (no admin required).
# start-agent.vbs is a *supervisor*: it launches the agent ONLY if it is not
# already running, so neither the logon Run key nor the 5-minute keepalive task
# ever spawns a duplicate. Two triggers together mean the agent survives both a
# reboot (Run key at logon) and being stopped/killed (task relaunches within 5m).
$exe  = "$Dir\aiops-agent.exe"
$conf = "$Dir\config.json"
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

# 1) Autostart at logon (survives reboot)
New-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "AIOpsAgent" -Value ('wscript.exe "' + $vbs + '"') -PropertyType String -Force | Out-Null

# 2) Keepalive: re-run the supervisor every 5 minutes so a stopped/killed agent
#    is relaunched. Current-user context — no admin. Escaped inner quotes so the
#    path survives even under usernames that contain spaces.
$trTask = 'wscript.exe \"' + $vbs + '\"'
schtasks /Create /TN "AIOpsAgent" /TR $trTask /SC MINUTE /MO 5 /F 2>$null | Out-Null

Get-Process aiops-agent -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Process "wscript.exe" -ArgumentList ('"' + $vbs + '"')
Write-Host "[AIOps] installed with autostart + 5-min keepalive (user-level). Check the dashboard."
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
curl -fsSL "$SERVER/dl/$BIN" -o aiops-agent
chmod +x aiops-agent

cat > config.json <<EOF
{
  "relay": true,
  "listen": "$LISTEN",
  "server": "$SERVER"
}
EOF

if command -v systemctl >/dev/null 2>&1 && [ "$(id -u)" = "0" ]; then
  cat > /etc/systemd/system/aiops-relay.service <<UNIT
[Unit]
Description=AIOps Monitor Relay
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
WorkingDirectory=$DIR
ExecStart=$DIR/aiops-agent --config $DIR/config.json
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
  nohup "$DIR/aiops-agent" --config "$DIR/config.json" > "$DIR/relay.log" 2>&1 &
  echo "[AIOps] relay started in background (log: $DIR/relay.log)"
fi
RELAY_PORT="${LISTEN##*:}"
echo ""
echo "[AIOps] Relay ready! Internal machines install with:"
echo "  curl -fsSL http://<this-host-ip>:${RELAY_PORT}/install.sh | sh"
`

// relayInstallPs1Template installs the agent in GATEWAY RELAY mode on Windows.
const relayInstallPs1Template = `$ErrorActionPreference = "Stop"
$Server = "__SERVER__"
$Listen = if ($env:RELAY_LISTEN) { $env:RELAY_LISTEN } else { ":8529" }
$Dir    = Join-Path $env:LOCALAPPDATA "aiops-agent"

Write-Host "[AIOps] installing relay to $Dir (upstream $Server)"
New-Item -ItemType Directory -Force $Dir | Out-Null
Invoke-WebRequest "$Server/dl/aiops-agent.exe" -OutFile "$Dir\aiops-agent.exe" -UseBasicParsing

$cfg = @{
  relay  = $true
  listen = $Listen
  server = $Server
} | ConvertTo-Json
[System.IO.File]::WriteAllText("$Dir\config.json", $cfg, (New-Object System.Text.UTF8Encoding $false))

$exe  = "$Dir\aiops-agent.exe"
$conf = "$Dir\config.json"
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
//   1. Replace Get-CimInstance (unreliable CommandLine) with taskkill / Get-Process
//   2. Kill ALL wscript.exe instances (safe on uninstall — no other apps use it)
//   3. Kill ALL aiops-agent.exe instances by name
//   4. Add $ErrorActionPreference = "Continue" for error visibility
//   5. Longer retry delays (2/4/8s) and MoveFileEx for stubborn files
//   6. Explicitly delete VBS files before EXE to release Run registry triggers
const uninstallPs1Template = `$ErrorActionPreference = "Continue"
$Dir = Join-Path $env:LOCALAPPDATA "aiops-agent"
Write-Host "[AIOps] uninstalling from $Dir"

# Step 1: Remove ALL autostart entries (normal + relay modes)
Remove-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "AIOpsAgent" -ErrorAction SilentlyContinue
Remove-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "AIOpsRelay" -ErrorAction SilentlyContinue

# Step 2: Remove the keepalive scheduled task FIRST — otherwise it relaunches the
# agent within 5 minutes and the file deletion below fails ("can't uninstall").
# Delete both the current name and the legacy hyphenated one.
schtasks /Delete /TN "AIOpsAgent" /F 2>$null | Out-Null
schtasks /Delete /TN "AIOps-Agent" /F 2>$null | Out-Null

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

# Step 5: Delete files with retry logic (handles stubborn file locks)
# Delete VBS files FIRST — they are the smallest and removing them
# prevents wscript.exe from being relaunched by the Run registry.
$files = @("start-agent.vbs", "start-relay.vbs", "aiops-agent.exe", "config.json", "agent_state.json", "agent.log", "plugins.zip")
foreach ($f in $files) {
    $path = Join-Path $Dir $f
    if (Test-Path $path) {
        Remove-Item $path -Force -ErrorAction SilentlyContinue
    }
}

# Remove plugins directory if present
$pluginsDir = Join-Path $Dir "plugins"
if (Test-Path $pluginsDir) {
    Remove-Item -Recurse -Force $pluginsDir -ErrorAction SilentlyContinue
}

# Second try: delete entire directory with longer retries (2/4/8s)
for ($i = 2; $i -le 8; $i *= 2) {
    if (Test-Path $Dir) {
        Remove-Item -Recurse -Force $Dir -ErrorAction SilentlyContinue
    }
    if (-not (Test-Path $Dir)) { break }
    Start-Sleep -Seconds $i
}

# v5.2.9: Last resort — schedule deletion on next reboot for stubborn files
if (Test-Path $Dir) {
    Write-Host "[AIOps] scheduling cleanup on next reboot for: $Dir"
    $tmpDir = Join-Path $env:TEMP "aiops-uninstall.bat"
    @"
@echo off
:retry
timeout /t 5 /nobreak >nul
rmdir /s /q "$Dir" 2>nul
if exist "$Dir" goto retry
del "%~f0" 2>nul
"@ | Out-File -FilePath $tmpDir -Encoding ASCII
    # Use RunOnce to execute cleanup batch on next login
    New-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\RunOnce" -Name "AIOpsCleanup" -Value "cmd.exe /c $tmpDir" -PropertyType String -Force | Out-Null
    Write-Host "[AIOps] warning: some files could not be deleted. Cleanup will finish on next reboot."
} else {
    Write-Host "[AIOps] uninstalled. You may delete the host card in the dashboard."
}
`
