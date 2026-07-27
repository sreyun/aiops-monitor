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
// and starts the service again.
func agentReplaceAndRestart(exe, staging string) error {
	helper := filepath.Join(filepath.Dir(exe), "aiops-agent-update-helper.ps1")
	logPath := filepath.Join(filepath.Dir(exe), "aiops-agent-update.log")
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'
$log = '%s'
function Write-Log($m){ try{ Add-Content -Path $log -Value ("[{0}] {1}" -f (Get-Date -Format o), $m) }catch{} }
try {
  Start-Sleep -Seconds 3
  $exe = '%s'
  $new = '%s'
  $bak = $exe + '.bak'
  if (-not (Test-Path $new)) { throw "staging missing: $new" }
  Get-Service -Name 'AiopsMonitorAgent','AIOps-Agent' -ErrorAction SilentlyContinue | Stop-Service -Force -ErrorAction SilentlyContinue
  Get-Process -Name 'aiops-agent' -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
  Start-Sleep -Milliseconds 800
  if (Test-Path $exe) {
    Copy-Item -Force $exe $bak
  }
  Move-Item -Force $new $exe
  try { Unblock-File -Path $exe -ErrorAction SilentlyContinue } catch {}
  $started = $false
  foreach ($name in @('AiopsMonitorAgent','AIOps-Agent')) {
    $svc = Get-Service -Name $name -ErrorAction SilentlyContinue
    if ($svc) {
      Start-Service $name
      $started = $true
      break
    }
  }
  if (-not $started) {
    $vbs = Join-Path (Split-Path $exe) 'start-agent.vbs'
    if (Test-Path $vbs) { Start-Process wscript.exe -ArgumentList ('"'+$vbs+'"') -WindowStyle Hidden }
    else { Start-Process $exe -WindowStyle Hidden }
  }
  Write-Log 'update ok'
} catch {
  Write-Log ("update failed: " + $_.Exception.Message)
  try {
    $exe = '%s'
    $bak = $exe + '.bak'
    if ((Test-Path $bak) -and -not (Test-Path $exe)) { Copy-Item -Force $bak $exe }
    Get-Service -Name 'AiopsMonitorAgent' -ErrorAction SilentlyContinue | Start-Service -ErrorAction SilentlyContinue
  } catch {}
  exit 1
} finally {
  Remove-Item -Force -ErrorAction SilentlyContinue $PSCommandPath
}
`, psSingleQuote(logPath), psSingleQuote(exe), psSingleQuote(staging), psSingleQuote(exe))
	if err := os.WriteFile(helper, []byte(script), 0o644); err != nil {
		return fmt.Errorf("write helper: %w", err)
	}
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", helper)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008} // DETACHED_PROCESS
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start helper: %w", err)
	}
	return nil
}

func psSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
