package main

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf16"
)

// buildLegacyAgentUpdateCommand returns a one-shot shell/PowerShell command that
// downloads /dl/$bin, verifies SHA-256, replaces the running binary, and restarts
// the agent service — without wiping config (unlike a full reinstall).
func buildLegacyAgentUpdateCommand(goos, serverURL, bin string, force bool) string {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	bin = strings.TrimSpace(bin)
	if serverURL == "" || bin == "" {
		return ""
	}
	_ = force
	switch strings.ToLower(goos) {
	case "linux", "darwin":
		return legacyUnixAgentUpdateScript(serverURL, bin, goos == "darwin")
	case "windows":
		return legacyWindowsAgentUpdateScript(serverURL, bin)
	default:
		return ""
	}
}

func legacyUnixAgentUpdateScript(server, bin string, darwin bool) string {
	// Restart: prefer service managers; fall back to pkill+nohup for crontab installs.
	// Never mask total failure with a bare `|| true` that still exits 0.
	restart := `
RESTARTED=0
if command -v systemctl >/dev/null 2>&1; then
  if systemctl restart aiops-agent 2>/dev/null || systemctl restart aiops-monitor-agent 2>/dev/null; then
    RESTARTED=1
  fi
fi
if [ "$RESTARTED" -eq 0 ]; then
  CFG=""
  for c in "$DIR/config.yaml" "$DIR/config.yml" "$HOME/.aiops-agent/config.yaml"; do
    [ -f "$c" ] && CFG="$c" && break
  done
  pkill -x aiops-agent 2>/dev/null || pkill -f '/aiops-agent( |$)' 2>/dev/null || true
  sleep 1
  if [ -n "$CFG" ]; then
    nohup "$DIR/aiops-agent" --config "$CFG" >/dev/null 2>&1 &
  else
    nohup "$DIR/aiops-agent" >/dev/null 2>&1 &
  fi
  sleep 1
  if pgrep -x aiops-agent >/dev/null 2>&1 || pgrep -f '/aiops-agent( |$)' >/dev/null 2>&1; then
    RESTARTED=1
  fi
fi
if [ "$RESTARTED" -eq 0 ]; then
  echo "restart failed (no systemd unit and relaunch unsuccessful)"; exit 1
fi
`
	if darwin {
		restart = `
RESTARTED=0
UIDN=$(id -u)
xattr -dr com.apple.quarantine aiops-agent 2>/dev/null || true
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
    nohup "$DIR/aiops-agent" --config "$CFG" >/dev/null 2>&1 &
  else
    nohup "$DIR/aiops-agent" >/dev/null 2>&1 &
  fi
  sleep 1
  pgrep -x aiops-agent >/dev/null 2>&1 && RESTARTED=1
fi
if [ "$RESTARTED" -eq 0 ]; then
  echo "restart failed (launchctl/nohup)"; exit 1
fi
`
	}
	return fmt.Sprintf(`set -e
SERVER=%q
BIN=%q
DIR=""
for d in /opt/aiops-agent "$HOME/.aiops-agent" /usr/local/aiops-agent; do
  if [ -x "$d/aiops-agent" ]; then DIR="$d"; break; fi
done
if [ -z "$DIR" ]; then
  EXE=$(command -v aiops-agent 2>/dev/null || true)
  if [ -n "$EXE" ] && [ -x "$EXE" ]; then DIR=$(dirname "$EXE"); fi
fi
if [ -z "$DIR" ] || [ ! -x "$DIR/aiops-agent" ]; then
  echo "agent binary not found under known install dirs"; exit 1
fi
cd "$DIR"
NEW=".aiops-agent.new"
rm -f "$NEW"
if command -v curl >/dev/null 2>&1; then
  curl -fSL --retry 3 -o "$NEW" "$SERVER/dl/$BIN"
  curl -fsSL -o ".aiops-agent.sha256" "$SERVER/dl/$BIN.sha256"
elif command -v wget >/dev/null 2>&1; then
  wget -q -O "$NEW" "$SERVER/dl/$BIN"
  wget -q -O ".aiops-agent.sha256" "$SERVER/dl/$BIN.sha256"
else
  echo "curl/wget required"; exit 1
fi
EXPECTED=$(awk '{print $1}' .aiops-agent.sha256 | tr 'A-F' 'a-f')
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "$NEW" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "$NEW" | awk '{print $1}')
else
  echo "sha256sum/shasum required"; exit 1
fi
if [ -z "$EXPECTED" ] || [ "$EXPECTED" != "$ACTUAL" ]; then
  echo "SHA-256 mismatch"; rm -f "$NEW"; exit 1
fi
cp -f aiops-agent aiops-agent.bak 2>/dev/null || true
mv -f "$NEW" aiops-agent
chmod +x aiops-agent
%s
echo "legacy agent update ok sha=$ACTUAL"
`, server, bin, restart)
}

func legacyWindowsAgentUpdateScript(server, bin string) string {
	ps := fmt.Sprintf(`$ErrorActionPreference='Stop'; try{[Net.ServicePointManager]::SecurityProtocol=[Net.ServicePointManager]::SecurityProtocol -bor 3072}catch{}; $Server='%s'; $Bin='%s'; $cands=@((Join-Path $env:ProgramFiles 'AIOps Agent'),(Join-Path $env:LOCALAPPDATA 'aiops-agent'),(Join-Path $env:ProgramData 'aiops-agent'),(Join-Path $env:ProgramData 'AIOps Agent')); $Dir=$null; foreach($d in $cands){ if(Test-Path (Join-Path $d 'aiops-agent.exe')){ $Dir=$d; break } }; if(-not $Dir){ throw 'agent exe not found' }; $Exe=Join-Path $Dir 'aiops-agent.exe'; $New=Join-Path $Dir '.aiops-agent.new.exe'; Invoke-WebRequest "$Server/dl/$Bin" -OutFile $New -UseBasicParsing; $Expected=((Invoke-WebRequest "$Server/dl/$Bin.sha256" -UseBasicParsing).Content -split '\s+')[0].Trim().ToLowerInvariant(); $Sha=[Security.Cryptography.SHA256]::Create(); $Stream=[IO.File]::OpenRead($New); try{ $Actual=([BitConverter]::ToString($Sha.ComputeHash($Stream))).Replace('-','').ToLowerInvariant() } finally { $Stream.Dispose(); $Sha.Dispose() }; if(-not $Expected -or $Expected -ne $Actual){ Remove-Item $New -Force; throw 'SHA-256 mismatch' }; Get-Service AiopsMonitorAgent -ErrorAction SilentlyContinue | Stop-Service -Force -ErrorAction SilentlyContinue; Get-Process aiops-agent -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue; Start-Sleep -Milliseconds 800; if(Test-Path $Exe){ Copy-Item $Exe ($Exe+'.bak') -Force -ErrorAction SilentlyContinue }; Move-Item $New $Exe -Force; try{ Unblock-File $Exe -ErrorAction SilentlyContinue }catch{}; $svc=Get-Service AiopsMonitorAgent -ErrorAction SilentlyContinue; if($svc){ Start-Service AiopsMonitorAgent } else { $vbs=Join-Path $Dir 'start-agent.vbs'; if(Test-Path $vbs){ Start-Process wscript.exe -ArgumentList ('"'+$vbs+'"') -WindowStyle Hidden } else { Start-Process $Exe -WindowStyle Hidden } }; Write-Output ('legacy agent update ok sha='+$Actual)`,
		strings.ReplaceAll(server, "'", "''"),
		strings.ReplaceAll(bin, "'", "''"),
	)
	return "powershell -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand " + psEncodedCommand(ps)
}

func psEncodedCommand(script string) string {
	u16 := utf16.Encode([]rune(script))
	raw := make([]byte, len(u16)*2)
	for i, v := range u16 {
		raw[i*2] = byte(v)
		raw[i*2+1] = byte(v >> 8)
	}
	return base64.StdEncoding.EncodeToString(raw)
}
