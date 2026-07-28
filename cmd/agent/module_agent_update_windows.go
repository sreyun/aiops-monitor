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
  Get-Process -Name 'aiops-agent','aiops-agent-windows-amd64','aiops-agent-windows-arm64','aiops-agent-windows-amd64-win2012' -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
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
        if (Wait-ServiceState $name 'Running' 25) {
          Write-Log ("service running: " + $name)
          try {
            $ver = & $Exe --version 2>$null
            Write-Log ("post-restart version: " + $ver)
          } catch {}
          return $true
        }
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
    if (Wait-ServiceState $name 'Running' 25) { Write-Log ("Start-Service ok: " + $name); return $true }
  }
  # Last resort for non-service installs: REQUIRE --config.
  # Bare Start-Process defaults server to localhost and permanently breaks
  # remote terminal + desktop (the classic post-update Win10/11 failure).
  if (-not $Cfg -or -not (Test-Path -LiteralPath $Cfg)) {
    Write-Log 'FATAL: no config.yaml beside exe; refusing bare Start-Process'
    return $false
  }
  $args = @('--config', $Cfg)
  $vbs = Join-Path $Dir 'start-agent.vbs'
  if (Test-Path -LiteralPath $vbs) {
    Write-Log 'fallback start-agent.vbs'
    Start-Process wscript.exe -ArgumentList ('"'+$vbs+'"') -WorkingDirectory $Dir -WindowStyle Hidden
  } else {
    Write-Log ('fallback Start-Process args=' + ($args -join ' '))
    Start-Process -FilePath $Exe -ArgumentList $args -WorkingDirectory $Dir -WindowStyle Hidden
  }
  Start-Sleep -Seconds 3
  return [bool](Get-Process -Name 'aiops-agent','aiops-agent-windows-amd64','aiops-agent-windows-arm64','aiops-agent-windows-amd64-win2012' -ErrorAction SilentlyContinue)
}
try {
  Start-Sleep -Seconds 3
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
  foreach ($name in @('AiopsMonitorAgent','AIOps-Agent')) {
    $svc = Get-Service -Name $name -ErrorAction SilentlyContinue
    if ($svc) {
      try { Stop-Service -Name $name -Force -ErrorAction SilentlyContinue } catch {}
      [void](Wait-ServiceState $name 'Stopped' 20)
    }
  }
  Stop-AgentProcesses
  Start-Sleep -Milliseconds 800
  if (Test-Path -LiteralPath $exe) {
    Copy-Item -Force -LiteralPath $exe -Destination $bak
  }
  # Retry Move-Item — AV / indexer can briefly lock the PE.
  $moved = $false
  for ($i=0; $i -lt 8; $i++) {
    try {
      Move-Item -Force -LiteralPath $new -Destination $exe
      $moved = $true
      break
    } catch {
      Write-Log ("Move-Item attempt $($i+1) failed: " + $_.Exception.Message)
      Start-Sleep -Seconds 1
    }
  }
  if (-not $moved) { throw "Move-Item failed after retries" }
  $swapped = $true
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
`, psSingleQuote(logPath), psSingleQuote(exe), psSingleQuote(staging), psSingleQuote(cfgPath), psSingleQuote(exe), psSingleQuote(cfgPath))
}

func psSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
