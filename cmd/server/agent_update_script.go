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
	// Restart: prefer --install-service / service managers so desktop worker stays
	// supervised; fall back to pkill+nohup with --config for crontab installs.
	restart := `
CFG=""
for c in "$DIR/config.yaml" "$DIR/config.yml" "$HOME/.aiops-agent/config.yaml"; do
  [ -f "$c" ] && CFG="$c" && break
done
RESTARTED=0
host_run() {
  if [ "$(id -u)" -eq 0 ] && command -v nsenter >/dev/null 2>&1 && [ -e /proc/1/ns/mnt ]; then
    nsenter -t 1 -m -u -i -n -- "$@"
  else
    "$@"
  fi
}
# As root: rewrite unit from host mount ns (User=root, unlock Protect*).
if command -v systemctl >/dev/null 2>&1 && [ -n "$CFG" ] && [ "$(id -u)" -eq 0 ]; then
  if host_run "$DIR/aiops-agent" --install-service --config "$CFG" >/dev/null 2>&1; then
    RESTARTED=1
  fi
fi
if [ "$RESTARTED" -eq 0 ] && command -v systemctl >/dev/null 2>&1 && [ "$(id -u)" -eq 0 ]; then
  host_run sh -c 'for u in aiops-agent aiops-monitor-agent; do
    rm -rf /etc/systemd/system/${u}.service.d /run/systemd/system/${u}.service.d 2>/dev/null || true
    f=/etc/systemd/system/${u}.service; [ -f "$f" ] || continue
    sed -i -e "s/^User=.*/User=root/" -e "s/^ProtectHome=.*/ProtectHome=false/" \
      -e "s/^ProtectSystem=.*/ProtectSystem=false/" -e "s/^PrivateTmp=.*/PrivateTmp=false/" \
      -e "s/^NoNewPrivileges=.*/NoNewPrivileges=false/" -e "/^CapabilityBoundingSet=/d" "$f" 2>/dev/null || true
    grep -q "^ProtectSystem=false" "$f" || echo "ProtectSystem=false" >> "$f"
    grep -q "^User=root" "$f" || echo "User=root" >> "$f"
  done; systemctl daemon-reload' 2>/dev/null || true
  if systemctl restart aiops-monitor-agent 2>/dev/null || systemctl restart aiops-agent 2>/dev/null; then
    RESTARTED=1
  fi
fi
if [ "$RESTARTED" -eq 0 ]; then
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
CFG=""
for c in "$DIR/config.yaml" "$DIR/config.yml" "$HOME/.aiops-agent/config.yaml"; do
  [ -f "$c" ] && CFG="$c" && break
done
UIDN=$(id -u)
xattr -dr com.apple.quarantine aiops-agent 2>/dev/null || true
RESTARTED=0
if [ -n "$CFG" ]; then
  "$DIR/aiops-agent" --install-service --config "$CFG" >/dev/null 2>&1 || true
fi
for label in "system/com.aiops.monitor.agent" "system/com.aiops.agent" "gui/$UIDN/com.aiops.agent" "gui/$UIDN/com.aiops.monitor.agent"; do
  if launchctl kickstart -k "$label" 2>/dev/null; then RESTARTED=1; break; fi
done
if [ "$RESTARTED" -eq 0 ]; then
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
	// Keep as one encoded command; restart must use --install-service / --config
	// for service installs, and VBS/schtasks/Start-Process --config for per-user
	// one-liner installs. Bare Start-Process $Exe (no args) breaks terminal+desktop.
	ps := fmt.Sprintf(`$ErrorActionPreference='Stop'
try{[Net.ServicePointManager]::SecurityProtocol=[Net.ServicePointManager]::SecurityProtocol -bor 3072}catch{}
$Server='%s'; $Bin='%s'
$exeNames=@('aiops-agent.exe','aiops-agent-windows-amd64.exe','aiops-agent-windows-arm64.exe','aiops-agent-windows-amd64-win2012.exe')
$cands=@((Join-Path $env:ProgramFiles 'AIOps Agent'),(Join-Path ${env:ProgramFiles(x86)} 'AIOps Agent'),(Join-Path $env:LOCALAPPDATA 'aiops-agent'),(Join-Path $env:ProgramData 'aiops-agent'),(Join-Path $env:ProgramData 'AIOps Agent'))
$Dir=$null; $Exe=$null
foreach($d in $cands){
  foreach($n in $exeNames){ $p=Join-Path $d $n; if(Test-Path -LiteralPath $p){ $Dir=$d; $Exe=$p; break } }
  if($Exe){ break }
}
if(-not $Exe){ throw 'agent exe not found' }
$New=Join-Path $Dir '.aiops-agent.new.exe'
$Cfg=$null
foreach($n in @('config.yaml','config.yml','config.json')){ $c=Join-Path $Dir $n; if(Test-Path -LiteralPath $c){ $Cfg=$c; break } }
Invoke-WebRequest "$Server/dl/$Bin" -OutFile $New -UseBasicParsing
$Expected=((Invoke-WebRequest "$Server/dl/$Bin.sha256" -UseBasicParsing).Content -split '\s+')[0].Trim().ToLowerInvariant()
$Sha=[Security.Cryptography.SHA256]::Create(); $Stream=[IO.File]::OpenRead($New)
try{ $Actual=([BitConverter]::ToString($Sha.ComputeHash($Stream))).Replace('-','').ToLowerInvariant() } finally { $Stream.Dispose(); $Sha.Dispose() }
if(-not $Expected -or $Expected -ne $Actual){ Remove-Item $New -Force; throw 'SHA-256 mismatch' }
try{ $probe=& $New --version 2>&1 | Out-String; if(-not $probe){ throw 'empty --version' } }catch{ Remove-Item $New -Force -ErrorAction SilentlyContinue; throw ("staging not runnable: "+$_.Exception.Message) }
$procNames=@('aiops-agent','aiops-agent-windows-amd64','aiops-agent-windows-arm64','aiops-agent-windows-amd64-win2012')
$svcNames=@('AiopsMonitorAgent','AIOps-Agent','AIOpsAgent')
foreach($name in $svcNames){
  try{ & "$env:SystemRoot\System32\sc.exe" stop $name 2>$null | Out-Null }catch{}
  Get-Service $name -ErrorAction SilentlyContinue | Stop-Service -Force -ErrorAction SilentlyContinue
}
Get-Process -Name $procNames -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1
Get-Process -Name $procNames -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
if(Test-Path -LiteralPath $Exe){ Copy-Item -LiteralPath $Exe -Destination ($Exe+'.bak') -Force -ErrorAction SilentlyContinue }
$moved=$false
for($i=0;$i -lt 12;$i++){
  try{ Move-Item -Force -LiteralPath $New -Destination $Exe; $moved=$true; break }catch{
    try{ Copy-Item -Force -LiteralPath $New -Destination $Exe; Remove-Item -Force -LiteralPath $New -ErrorAction SilentlyContinue; $moved=$true; break }catch{}
    Start-Sleep -Seconds 1
    Get-Process -Name $procNames -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
  }
}
if(-not $moved){ throw 'replace binary failed (file lock)' }
try{ Unblock-File -Path $Exe -ErrorAction SilentlyContinue }catch{}
function Test-Running { if(Get-Process -Name $procNames -ErrorAction SilentlyContinue){ return $true }; foreach($n in $svcNames){ $s=Get-Service $n -ErrorAction SilentlyContinue; if($s -and $s.Status -eq 'Running'){ return $true } }; return $false }
$ok=$false
$hasSvc=$false
foreach($n in $svcNames){ if(Get-Service $n -ErrorAction SilentlyContinue){ $hasSvc=$true; break } }
if($hasSvc -and $Cfg){
  $p=Start-Process -FilePath $Exe -ArgumentList @('--install-service','--config',$Cfg) -WorkingDirectory $Dir -Wait -PassThru -WindowStyle Hidden
  if($p -and $p.ExitCode -eq 0){ $ok=$true }
  if(-not $ok){ foreach($n in $svcNames){ $svc=Get-Service $n -ErrorAction SilentlyContinue; if($svc){ try{ Start-Service $n; Start-Sleep 2; if(Test-Running){ $ok=$true; break } }catch{} } } }
}
if(-not $ok){
  $vbs=Join-Path $Dir 'start-agent.vbs'
  if(Test-Path -LiteralPath $vbs){ Start-Process wscript.exe -ArgumentList ('"'+$vbs+'"') -WorkingDirectory $Dir -WindowStyle Hidden }
  else {
    foreach($tn in @('AIOpsAgent','AIOpsAgentKeepalive','AIOps Agent')){ try{ & "$env:SystemRoot\System32\schtasks.exe" /Run /TN $tn 2>$null | Out-Null }catch{} }
    if($Cfg){ Start-Process -FilePath $Exe -ArgumentList @('--config',$Cfg) -WorkingDirectory $Dir -WindowStyle Hidden }
    else { throw 'restart failed: no service and no config.yaml beside agent' }
  }
}
Start-Sleep -Seconds 4
if(-not (Test-Running)){ throw 'agent not running after update' }
Write-Output ('legacy agent update ok sha='+$Actual)
`,
		strings.ReplaceAll(server, "'", "''"),
		strings.ReplaceAll(bin, "'", "''"),
	)
	// Prefer absolute powershell so LocalSystem / thin PATH still works when this
	// string is executed via cmd /c (agent runShellCommand expands %SystemRoot%).
	return `%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand ` + psEncodedCommand(ps)
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
