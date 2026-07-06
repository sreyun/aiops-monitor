package main

import "strings"

// renderScript injects the server URL / token / category into an install
// template. Placeholders are used (not fmt) so the shell/PowerShell '%' and
// '$' characters pass through untouched.
func renderScript(tmpl, server, token, category string) string {
	return strings.NewReplacer(
		"__SERVER__", server,
		"__TOKEN__", token,
		"__CATEGORY__", category,
	).Replace(tmpl)
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
case "$OS" in
  Linux)  BIN="aiops-agent-linux-amd64" ;;
  Darwin) BIN="aiops-agent-darwin-arm64" ;;
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
cat > config.json <<EOF
{
  "server": "$SERVER",
  "token": "$TOKEN",
  "category": "$CATEGORY",
  "report_interval": 5,
  "plugin_interval": 15,
  "plugins_dir": "$DIR/plugins",
  "state_file": "$DIR/agent_state.json"
}
EOF

if command -v systemctl >/dev/null 2>&1 && [ "$(id -u)" = "0" ]; then
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
  echo "[AIOps] systemd service started: aiops-agent"
else
  pkill -f "$DIR/aiops-agent" 2>/dev/null || true
  nohup "$DIR/aiops-agent" --config "$DIR/config.json" > "$DIR/agent.log" 2>&1 &
  echo "[AIOps] started in background (log: $DIR/agent.log)"
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
$Dir      = Join-Path $env:LOCALAPPDATA "aiops-agent"

Write-Host "[AIOps] installing to $Dir (server $Server)"
New-Item -ItemType Directory -Force $Dir | Out-Null
Invoke-WebRequest "$Server/dl/aiops-agent.exe" -OutFile "$Dir\aiops-agent.exe" -UseBasicParsing
try {
  Invoke-WebRequest "$Server/dl/plugins.zip" -OutFile "$Dir\plugins.zip" -UseBasicParsing
  Expand-Archive -Path "$Dir\plugins.zip" -DestinationPath $Dir -Force
  Remove-Item "$Dir\plugins.zip" -Force
} catch { Write-Host "[AIOps] plugins skipped" }

$cfg = @{
  server = $Server
  token = $Token
  category = $Category
  report_interval = 5
  plugin_interval = 15
  plugins_dir = "$Dir\plugins"
  state_file = "$Dir\agent_state.json"
} | ConvertTo-Json
[System.IO.File]::WriteAllText("$Dir\config.json", $cfg, (New-Object System.Text.UTF8Encoding $false))

# 用户级开机自启（无需管理员）：HKCU Run + 隐藏窗口的 VBS 启动器
$exe  = "$Dir\aiops-agent.exe"
$conf = "$Dir\config.json"
$vbs  = "$Dir\start-agent.vbs"
$line = 'CreateObject("WScript.Shell").Run """' + $exe + '"" --config ""' + $conf + '""", 0, False'
[System.IO.File]::WriteAllText($vbs, $line, (New-Object System.Text.ASCIIEncoding))
New-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "AIOpsAgent" -Value ('wscript.exe "' + $vbs + '"') -PropertyType String -Force | Out-Null

Get-Process aiops-agent -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Process "wscript.exe" -ArgumentList ('"' + $vbs + '"')
Write-Host "[AIOps] installed and started (user-level autostart). Check the dashboard."
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
PLIST="$HOME/Library/LaunchAgents/com.aiops.agent.plist"
if [ -f "$PLIST" ]; then
  launchctl unload "$PLIST" 2>/dev/null || true
  rm -f "$PLIST"
fi
pkill -f "$DIR/aiops-agent" 2>/dev/null || true
rm -rf "$DIR"
echo "[AIOps] uninstalled. You may delete the host card in the dashboard."
`

// uninstallPs1Template stops + removes the agent on Windows (user-level).
const uninstallPs1Template = `$Dir = Join-Path $env:LOCALAPPDATA "aiops-agent"
Write-Host "[AIOps] uninstalling from $Dir"
Remove-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "AIOpsAgent" -ErrorAction SilentlyContinue
schtasks /Delete /TN "AIOps-Agent" /F 2>$null | Out-Null
Get-Process aiops-agent -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 500
Remove-Item -Recurse -Force $Dir -ErrorAction SilentlyContinue
Write-Host "[AIOps] uninstalled. You may delete the host card in the dashboard."
`
