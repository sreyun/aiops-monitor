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
// and brings the agent back via --install-service (or user-mode VBS/schtasks /
// Start-Process --config). Never bare Start-Process without --config.
func agentReplaceAndRestart(exe, staging, cfgPath string) error {
	ensureWindowsProcessPath()
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
	ps := windowsPowerShellPath()
	cmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", helper)
	cmd.Dir = dir
	cmd.Env = enrichWindowsShellEnv(os.Environ())
	// DETACHED_PROCESS|CREATE_NEW_PROCESS_GROUP so the helper survives service stop.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | 0x00000200,
		HideWindow:    true,
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start helper (%s): %w", ps, err)
	}
	return nil
}

// windowsPowerShellPath returns an absolute powershell.exe path. LocalSystem
// services often lack System32\WindowsPowerShell on PATH, so bare "powershell"
// fails with "executable file not found in %PATH%".
func windowsPowerShellPath() string {
	root := windowsSystemRoot()
	candidates := []string{
		filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe"),
		filepath.Join(root, "SysWOW64", "WindowsPowerShell", "v1.0", "powershell.exe"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	if p, err := exec.LookPath("powershell.exe"); err == nil {
		return p
	}
	if p, err := exec.LookPath("powershell"); err == nil {
		return p
	}
	return candidates[0]
}

// buildWindowsUpdateHelperScript is split out for unit tests.
func buildWindowsUpdateHelperScript(exe, staging, cfgPath, logPath string) string {
	return fmt.Sprintf(`$ErrorActionPreference='Stop'
$log = '%s'
function Write-Log($m){ try{ Add-Content -LiteralPath $log -Value ("[{0}] {1}" -f (Get-Date -Format o), $m) -Encoding UTF8 }catch{} }
function Wait-ServiceState([string]$Name, [string]$Want, [int]$Seconds) {
  for ($i=0; $i -lt $Seconds; $i++) {
    $s = Get-Service -Name $Name -ErrorAction SilentlyContinue
    if ($s -and $s.Status -eq $Want) { return $true }
    Start-Sleep -Seconds 1
  }
  return $false
}
function Get-AgentServiceNames {
  return @('AiopsMonitorAgent','AIOps-Agent','AIOpsAgent')
}
function Test-AgentServicePresent {
  foreach ($name in (Get-AgentServiceNames)) {
    if (Get-Service -Name $name -ErrorAction SilentlyContinue) { return $true }
  }
  return $false
}
function Stop-AgentProcesses {
  $names = @('aiops-agent','aiops-agent-windows-amd64','aiops-agent-windows-arm64','aiops-agent-windows-amd64-win2012')
  Get-Process -Name $names -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
  # Also kill by path when renamed oddly.
  Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
    Where-Object { $_.Name -match '^aiops-agent' -or ($_.ExecutablePath -and $_.ExecutablePath -match 'aiops-agent') } |
    ForEach-Object { try { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue } catch {} }
}
function Test-AgentRunning {
  $names = @('aiops-agent','aiops-agent-windows-amd64','aiops-agent-windows-arm64','aiops-agent-windows-amd64-win2012')
  if (Get-Process -Name $names -ErrorAction SilentlyContinue) { return $true }
  foreach ($name in (Get-AgentServiceNames)) {
    $s = Get-Service -Name $name -ErrorAction SilentlyContinue
    if ($s -and $s.Status -eq 'Running') { return $true }
  }
  return $false
}
function Restart-AgentUserMode {
  param([string]$Exe,[string]$Cfg,[string]$Dir)
  if (-not $Cfg -or -not (Test-Path -LiteralPath $Cfg)) {
    Write-Log 'FATAL: no config beside exe; refusing bare Start-Process'
    return $false
  }
  # Match one-liner install: VBS supervisor + logon task + keepalive task.
  $vbs = Join-Path $Dir 'start-agent.vbs'
  if (Test-Path -LiteralPath $vbs) {
    Write-Log 'user-mode: start-agent.vbs'
    Start-Process -FilePath "$env:SystemRoot\System32\wscript.exe" -ArgumentList (@('"'+$vbs+'"')) -WorkingDirectory $Dir -WindowStyle Hidden
    Start-Sleep -Seconds 3
    if (Test-AgentRunning) { Write-Log 'user-mode VBS ok'; return $true }
  }
  foreach ($tn in @('AIOpsAgent','AIOpsAgentKeepalive','AIOps Agent')) {
    try {
      $out = & "$env:SystemRoot\System32\schtasks.exe" /Run /TN $tn 2>&1 | Out-String
      Write-Log ("schtasks /Run $tn: " + $out.Trim())
      Start-Sleep -Seconds 3
      if (Test-AgentRunning) { Write-Log ("user-mode schtasks ok: " + $tn); return $true }
    } catch {
      Write-Log ("schtasks $tn: " + $_.Exception.Message)
    }
  }
  Write-Log ('user-mode Start-Process --config ' + $Cfg)
  Start-Process -FilePath $Exe -ArgumentList @('--config', $Cfg) -WorkingDirectory $Dir -WindowStyle Hidden
  Start-Sleep -Seconds 3
  return (Test-AgentRunning)
}
function Restart-AgentService {
  param([string]$Exe,[string]$Cfg,[string]$Dir)
  $hasSvc = Test-AgentServicePresent
  Write-Log ("restart path hasService=$hasSvc cfg=$Cfg")
  # Service installs: refresh SCM ImagePath then start. Needs admin (LocalSystem ok).
  if ($hasSvc -and $Cfg -and (Test-Path -LiteralPath $Cfg)) {
    Write-Log ("install-service with config: " + $Cfg)
    $p = Start-Process -FilePath $Exe -ArgumentList @('--install-service','--config', $Cfg) -WorkingDirectory $Dir -Wait -PassThru -WindowStyle Hidden
    if ($p -and $p.ExitCode -eq 0) {
      foreach ($name in (Get-AgentServiceNames)) {
        if (Wait-ServiceState $name 'Running' 30) {
          Write-Log ("service running: " + $name)
          try { Write-Log ("post-restart version: " + (& $Exe --version 2>$null)) } catch {}
          return $true
        }
      }
    } else {
      $code = if ($p) { $p.ExitCode } else { 'null' }
      Write-Log ("install-service exit=" + $code)
    }
    foreach ($name in (Get-AgentServiceNames)) {
      $svc = Get-Service -Name $name -ErrorAction SilentlyContinue
      if (-not $svc) { continue }
      try { Start-Service -Name $name -ErrorAction Stop } catch {
        Write-Log ("Start-Service $name failed: " + $_.Exception.Message)
        continue
      }
      if (Wait-ServiceState $name 'Running' 30) { Write-Log ("Start-Service ok: " + $name); return $true }
    }
  }
  # Per-user / VBS installs (majority of Windows one-liner hosts): never require
  # --install-service (needs elevation and fails silently for non-admin).
  return (Restart-AgentUserMode -Exe $Exe -Cfg $Cfg -Dir $Dir)
}
try {
  Start-Sleep -Seconds 2
  $exe = '%s'
  $new = '%s'
  $cfg = '%s'
  $dir = Split-Path -Parent $exe
  $bak = $exe + '.bak'
  # Only restore .bak after a successful swap. Restoring on any pre-swap
  # failure (stale leftover .bak, Copy-Item refresh fail, Move-Item fail)
  # would silently downgrade a still-good current PE.
  $swapped = $false
  if (-not (Test-Path -LiteralPath $new)) { throw "staging missing: $new" }
  if (-not $cfg) {
    foreach ($n in @('config.yaml','config.yml','config.json')) {
      $c = Join-Path $dir $n
      if (Test-Path -LiteralPath $c) { $cfg = $c; break }
    }
  }
  Write-Log ("update begin exe=$exe cfg=$cfg")
  try {
    $probe = & $new --version 2>&1 | Out-String
    Write-Log ("staging --version: " + $probe.Trim())
  } catch {
    throw ("staging binary not runnable before swap: " + $_.Exception.Message)
  }
  foreach ($name in (Get-AgentServiceNames)) {
    $svc = Get-Service -Name $name -ErrorAction SilentlyContinue
    if ($svc) {
      try { Stop-Service -Name $name -Force -ErrorAction SilentlyContinue } catch {}
      [void](Wait-ServiceState $name 'Stopped' 25)
    }
  }
  Stop-AgentProcesses
  Start-Sleep -Milliseconds 800
  if (Test-Path -LiteralPath $exe) {
    try { Copy-Item -Force -LiteralPath $exe -Destination $bak } catch {
      Write-Log ("backup Copy-Item warning: " + $_.Exception.Message)
    }
  }
  # Retry Move-Item — AV / indexer can briefly lock the PE. Fall back to copy+delete.
  $moved = $false
  for ($i=0; $i -lt 10; $i++) {
    try {
      Move-Item -Force -LiteralPath $new -Destination $exe
      $moved = $true
      break
    } catch {
      Write-Log ("Move-Item attempt $($i+1) failed: " + $_.Exception.Message)
      try {
        Copy-Item -Force -LiteralPath $new -Destination $exe
        Remove-Item -Force -LiteralPath $new -ErrorAction SilentlyContinue
        $moved = $true
        Write-Log ("Copy-Item fallback ok on attempt $($i+1)")
        break
      } catch {
        Write-Log ("Copy-Item attempt $($i+1) failed: " + $_.Exception.Message)
      }
      Start-Sleep -Seconds 1
      Stop-AgentProcesses
    }
  }
  if (-not $moved) { throw "Move-Item/Copy-Item failed after retries" }
  $swapped = $true
  try { Unblock-File -Path $exe -ErrorAction SilentlyContinue } catch {}
  if (-not (Restart-AgentService -Exe $exe -Cfg $cfg -Dir $dir)) {
    throw 'agent failed to restart after binary replace'
  }
  Write-Log 'update ok'
  try {
    Set-Content -LiteralPath (Join-Path $dir 'aiops-agent-update.result') -Value ("ok " + (Get-Date -Format o)) -Encoding UTF8
  } catch {}
} catch {
  Write-Log ("update failed: " + $_.Exception.Message)
  try {
    Set-Content -LiteralPath (Join-Path (Split-Path -Parent '%s') 'aiops-agent-update.result') -Value ("fail " + $_.Exception.Message) -Encoding UTF8
  } catch {}
  try {
    $exe = '%s'
    $cfg = '%s'
    $dir = Split-Path -Parent $exe
    $bak = $exe + '.bak'
    # Restore only when this run swapped (new PE may be broken) or exe is gone.
    if ((Test-Path -LiteralPath $bak) -and ($swapped -or -not (Test-Path -LiteralPath $exe))) {
      Write-Log ("restoring backup (swapped=$swapped)")
      Copy-Item -Force -LiteralPath $bak -Destination $exe
    } else {
      Write-Log ("skip backup restore (swapped=$swapped exe_exists=$((Test-Path -LiteralPath $exe)))")
    }
    [void](Restart-AgentService -Exe $exe -Cfg $cfg -Dir $dir)
  } catch {}
  exit 1
} finally {
  Remove-Item -Force -ErrorAction SilentlyContinue $PSCommandPath
}
`, psSingleQuote(logPath), psSingleQuote(exe), psSingleQuote(staging), psSingleQuote(cfgPath), psSingleQuote(exe), psSingleQuote(exe), psSingleQuote(cfgPath))
}

func psSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
