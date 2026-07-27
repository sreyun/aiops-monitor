//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// agentReplaceAndRestart cannot overwrite a running Windows PE. Stage stays as
// .new; a detached PowerShell helper stops the service/process, swaps files,
// and brings the agent back via --install-service (or Start-Service), never as a
// bare Start-Process without --config (that breaks terminal + desktop worker).
func agentReplaceAndRestart(exe, staging, cfgPath string) error {
	dir := filepath.Dir(exe)
	helper := filepath.Join(dir, "aiops-agent-update-helper.ps1")
	logPath := filepath.Join(dir, "aiops-agent-update.log")
	if strings.TrimSpace(cfgPath) == "" {
		cfgPath = resolveAgentConfigBesideExe(dir)
	}
	script := buildWindowsUpdateHelperScript(exe, staging, cfgPath, logPath)
	if err := os.WriteFile(helper, []byte(script), 0o644); err != nil {
		// Program Files can be momentarily locked; fall back to %TEMP%.
		helper = filepath.Join(os.TempDir(), "aiops-agent-update-helper.ps1")
		logPath = filepath.Join(os.TempDir(), "aiops-agent-update.log")
		script = buildWindowsUpdateHelperScript(exe, staging, cfgPath, logPath)
		if err2 := os.WriteFile(helper, []byte(script), 0o644); err2 != nil {
			return fmt.Errorf("write helper: %v / %v", err, err2)
		}
	}
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", helper)
	cmd.Dir = dir
	// DETACHED_PROCESS|CREATE_NEW_PROCESS_GROUP so the helper survives service stop.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | 0x00000200,
		HideWindow:    true,
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start helper: %w", err)
	}
	return nil
}

// buildWindowsUpdateHelperScript is split out for unit tests.
func buildWindowsUpdateHelperScript(exe, staging, cfgPath, logPath string) string {
	return fmt.Sprintf(`$ErrorActionPreference='Stop'
$log = '%s'
function Write-Log($m){ try{ Add-Content -Path $log -Value ("[{0}] {1}" -f (Get-Date -Format o), $m) }catch{} }
function Wait-ServiceState([string]$Name, [string]$Want, [int]$Seconds) {
  for ($i=0; $i -lt $Seconds; $i++) {
    $s = Get-Service -Name $Name -ErrorAction SilentlyContinue
    if ($s -and $s.Status -eq $Want) { return $true }
    Start-Sleep -Seconds 1
  }
  return $false
}
function Stop-AgentProcesses {
  Get-Process -Name 'aiops-agent','aiops-agent-windows-amd64','aiops-agent-windows-arm64' -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
}
function Restart-AgentService {
  param([string]$Exe,[string]$Cfg,[string]$Dir)
  # Preferred: refresh SCM ImagePath (--service --config) and start cleanly.
  # This is what keeps the interactive desktop worker + terminal channel healthy.
  if ($Cfg -and (Test-Path -LiteralPath $Cfg)) {
    Write-Log ("install-service with config: " + $Cfg)
    $p = Start-Process -FilePath $Exe -ArgumentList @('--install-service','--config', $Cfg) -WorkingDirectory $Dir -Wait -PassThru -WindowStyle Hidden
    if ($p.ExitCode -eq 0) {
      foreach ($name in @('AiopsMonitorAgent','AIOps-Agent')) {
        if (Wait-ServiceState $name 'Running' 20) { Write-Log ("service running: " + $name); return $true }
      }
    } else {
      Write-Log ("install-service exit=" + $p.ExitCode)
    }
  }
  foreach ($name in @('AiopsMonitorAgent','AIOps-Agent')) {
    $svc = Get-Service -Name $name -ErrorAction SilentlyContinue
    if (-not $svc) { continue }
    try {
      Start-Service -Name $name -ErrorAction Stop
    } catch {
      Write-Log ("Start-Service $name failed: " + $_.Exception.Message)
      continue
    }
    if (Wait-ServiceState $name 'Running' 20) { Write-Log ("Start-Service ok: " + $name); return $true }
  }
  # Last resort for non-service installs: keep --config + WorkingDirectory.
  # Do NOT start without config (defaults to localhost and breaks remote UI).
  $args = @()
  if ($Cfg -and (Test-Path -LiteralPath $Cfg)) { $args = @('--config', $Cfg) }
  $vbs = Join-Path $Dir 'start-agent.vbs'
  if (Test-Path -LiteralPath $vbs) {
    Write-Log 'fallback start-agent.vbs'
    Start-Process wscript.exe -ArgumentList ('"'+$vbs+'"') -WorkingDirectory $Dir -WindowStyle Hidden
  } else {
    Write-Log ('fallback Start-Process args=' + ($args -join ' '))
    if ($args.Count -gt 0) {
      Start-Process -FilePath $Exe -ArgumentList $args -WorkingDirectory $Dir -WindowStyle Hidden
    } else {
      Start-Process -FilePath $Exe -WorkingDirectory $Dir -WindowStyle Hidden
    }
  }
  Start-Sleep -Seconds 2
  return [bool](Get-Process -Name 'aiops-agent','aiops-agent-windows-amd64','aiops-agent-windows-arm64' -ErrorAction SilentlyContinue)
}
try {
  Start-Sleep -Seconds 3
  $exe = '%s'
  $new = '%s'
  $cfg = '%s'
  $dir = Split-Path -Parent $exe
  $bak = $exe + '.bak'
  if (-not (Test-Path -LiteralPath $new)) { throw "staging missing: $new" }
  if (-not $cfg) {
    foreach ($n in @('config.yaml','config.yml','config.json')) {
      $c = Join-Path $dir $n
      if (Test-Path -LiteralPath $c) { $cfg = $c; break }
    }
  }
  Write-Log ("update begin exe=$exe cfg=$cfg")
  foreach ($name in @('AiopsMonitorAgent','AIOps-Agent')) {
    $svc = Get-Service -Name $name -ErrorAction SilentlyContinue
    if ($svc) {
      try { Stop-Service -Name $name -Force -ErrorAction SilentlyContinue } catch {}
      [void](Wait-ServiceState $name 'Stopped' 15)
    }
  }
  Stop-AgentProcesses
  Start-Sleep -Milliseconds 800
  if (Test-Path -LiteralPath $exe) {
    Copy-Item -Force -LiteralPath $exe -Destination $bak
  }
  Move-Item -Force -LiteralPath $new -Destination $exe
  try { Unblock-File -Path $exe -ErrorAction SilentlyContinue } catch {}
  if (-not (Restart-AgentService -Exe $exe -Cfg $cfg -Dir $dir)) {
    throw 'agent failed to restart after binary replace'
  }
  Write-Log 'update ok'
} catch {
  Write-Log ("update failed: " + $_.Exception.Message)
  try {
    $exe = '%s'
    $cfg = '%s'
    $dir = Split-Path -Parent $exe
    $bak = $exe + '.bak'
    if ((Test-Path -LiteralPath $bak) -and -not (Test-Path -LiteralPath $exe)) {
      Copy-Item -Force -LiteralPath $bak -Destination $exe
    }
    [void](Restart-AgentService -Exe $exe -Cfg $cfg -Dir $dir)
  } catch {}
  exit 1
} finally {
  Remove-Item -Force -ErrorAction SilentlyContinue $PSCommandPath
}
`, psSingleQuote(logPath), psSingleQuote(exe), psSingleQuote(staging), psSingleQuote(cfgPath), psSingleQuote(exe), psSingleQuote(cfgPath))
}

func psSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
