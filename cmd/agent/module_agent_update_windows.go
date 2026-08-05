//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	// CREATE_BREAKAWAY_FROM_JOB — critical: SCM / service Job Objects kill
	// children when the service stops. Without breakaway the update helper dies
	// with the agent and the binary never swaps (classic "Windows no auto-update").
	// createNoWindow / createBreakawayJob also declared in desktop_session_windows.go.
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

// agentReplaceAndRestart cannot overwrite a running Windows PE. Stage stays as
// .new; a detached helper (prefer SYSTEM scheduled task, else breakaway process)
// stops the service/process, swaps files, and brings the agent back via
// --install-service or user-mode VBS/schtasks/--config.
func agentReplaceAndRestart(exe, staging, cfgPath string) error {
	ensureWindowsProcessPath()
	dir := filepath.Dir(exe)
	if strings.TrimSpace(cfgPath) == "" {
		cfgPath = resolveAgentConfigBesideExe(dir)
	}

	// Prefer SYSTEM-readable, space-free work dir. Per-user TEMP with spaces
	// breaks Scheduled Task -Argument quoting; SYSTEM often cannot read it.
	workDir := windowsUpdateWorkDir()
	_ = os.MkdirAll(workDir, 0o755)
	helper := filepath.Join(workDir, "aiops-agent-update-helper.ps1")
	logPath := filepath.Join(workDir, "aiops-agent-update.log")
	resultPath := filepath.Join(dir, "aiops-agent-update.result")
	altResult := filepath.Join(workDir, "aiops-agent-update.result")
	_ = os.Remove(logPath)
	_ = os.Remove(altResult)

	script := buildWindowsUpdateHelperScript(exe, staging, cfgPath, logPath, resultPath, altResult)
	if err := os.WriteFile(helper, []byte(script), 0o644); err != nil {
		return fmt.Errorf("write helper: %w", err)
	}
	_ = os.WriteFile(filepath.Join(dir, "aiops-agent-update-helper.ps1"), []byte(script), 0o644)

	ps := windowsPowerShellPath()
	// -Command & 'path' survives spaces better than -File for Task Scheduler.
	psArgs := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-Command", "& '" + strings.ReplaceAll(helper, "'", "''") + "'"}

	scheduled := false
	if err := scheduleWindowsUpdateTask(ps, psArgs); err == nil {
		scheduled = true
		_ = os.WriteFile(altResult, []byte("scheduled "+time.Now().Format(time.RFC3339)), 0o644)
	}
	// Task registration ≠ helper running. Verify, else fall through to breakaway.
	if scheduled && waitWindowsUpdateHelperAlive(logPath, altResult, resultPath, 6*time.Second) {
		return nil
	}
	if scheduled {
		_ = exec.Command(filepath.Join(windowsSystemRoot(), "System32", "schtasks.exe"),
			"/Delete", "/TN", "AIOpsAgentSelfUpdate", "/F").Run()
	}

	if err := startWindowsBreakaway(ps, psArgs, workDir); err != nil {
		if err2 := startWindowsCmdStart(ps, helper, workDir); err2 != nil {
			return fmt.Errorf("start update helper: schtasks/breakaway/cmd all failed: %v / %v", err, err2)
		}
	}
	_ = waitWindowsUpdateHelperAlive(logPath, altResult, resultPath, 4*time.Second)
	return nil
}

func windowsUpdateWorkDir() string {
	for _, d := range []string{
		filepath.Join(os.Getenv("ProgramData"), "aiops-agent-update"),
		filepath.Join(windowsSystemRoot(), "Temp", "aiops-agent-update"),
		filepath.Join(os.TempDir(), "aiops-agent-update"),
	} {
		if strings.TrimSpace(d) == "" {
			continue
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			continue
		}
		probe := filepath.Join(d, ".w")
		if err := os.WriteFile(probe, []byte("1"), 0o644); err != nil {
			continue
		}
		_ = os.Remove(probe)
		return d
	}
	return filepath.Join(os.TempDir(), "aiops-agent-update")
}

func waitWindowsUpdateHelperAlive(logPath, altResult, resultPath string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		for _, p := range []string{logPath, resultPath, altResult} {
			b, err := os.ReadFile(p)
			if err != nil || len(b) == 0 {
				continue
			}
			s := string(b)
			if strings.Contains(s, "helper start") || strings.HasPrefix(s, "running") ||
				strings.HasPrefix(s, "ok ") || strings.HasPrefix(s, "fail ") {
				return true
			}
		}
		if windowsUpdateHelperAlive() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func windowsUpdateHelperAlive(paths ...string) bool {
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && st.Size() > 0 {
			b, _ := os.ReadFile(p)
			s := string(b)
			if strings.Contains(s, "helper start") || strings.HasPrefix(s, "running") ||
				strings.HasPrefix(s, "ok ") || strings.HasPrefix(s, "fail ") {
				return true
			}
		}
	}
	out, err := exec.Command(windowsPowerShellPath(), "-NoProfile", "-NonInteractive", "-Command",
		`Get-CimInstance Win32_Process -ErrorAction SilentlyContinue | Where-Object { $_.CommandLine -match 'aiops-agent-update-helper\.ps1' } | Select-Object -First 1 -ExpandProperty ProcessId`).CombinedOutput()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func scheduleWindowsUpdateTask(ps string, psArgs []string) error {
	// Prefer Register-ScheduledTask (locale-safe dates) over schtasks /SD.
	// SYSTEM + Highest when elevated; fall back to current-user task.
	argLine := make([]string, 0, len(psArgs))
	for _, a := range psArgs {
		argLine = append(argLine, quoteWinArg(a))
	}
	arguments := strings.Join(argLine, " ")
	task := "AIOpsAgentSelfUpdate"
	psSchedule := fmt.Sprintf(`
$ErrorActionPreference='Stop'
$task='%s'
$exe='%s'
$arg='%s'
Unregister-ScheduledTask -TaskName $task -Confirm:$false -ErrorAction SilentlyContinue | Out-Null
$action = New-ScheduledTaskAction -Execute $exe -Argument $arg
# Far-future ONCE trigger satisfies older Windows; we only Start once (no double /Run).
$trigger = New-ScheduledTaskTrigger -Once -At ((Get-Date).AddYears(10))
$ok = $false
try {
  $prin = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
  Register-ScheduledTask -TaskName $task -Action $action -Trigger $trigger -Principal $prin -Force | Out-Null
  $ok = $true
} catch {
  try {
    Register-ScheduledTask -TaskName $task -Action $action -Trigger $trigger -Force | Out-Null
    $ok = $true
  } catch {
    throw $_
  }
}
if ($ok) {
  Start-ScheduledTask -TaskName $task -ErrorAction Stop
}
`, psSingleQuote(task), psSingleQuote(ps), psSingleQuote(arguments))

	cmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", psSchedule)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schedule task: %v (%s)", err, truncBytes(out, 300))
	}
	return nil
}

func quoteWinArg(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\"") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func startWindowsBreakaway(ps string, args []string, dir string) error {
	cmd := exec.Command(ps, args...)
	cmd.Dir = dir
	cmd.Env = enrichWindowsShellEnv(os.Environ())
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createBreakawayJob | detachedProcess | createNewProcessGroup | createNoWindow,
		HideWindow:    true,
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("breakaway start (%s): %w", ps, err)
	}
	// Detach from Go's wait; process continues independently.
	go func() { _ = cmd.Process.Release() }()
	return nil
}

func startWindowsCmdStart(ps, helper, dir string) error {
	cmdExe := windowsCmdPath()
	// start "" /b launches a new process not tied to our console/job as tightly.
	line := fmt.Sprintf(`start "" /b "%s" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%s"`, ps, helper)
	cmd := exec.Command(cmdExe, "/c", line)
	cmd.Dir = dir
	cmd.Env = enrichWindowsShellEnv(os.Environ())
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createBreakawayJob | createNewProcessGroup | createNoWindow,
		HideWindow:    true,
	}
	return cmd.Start()
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
func buildWindowsUpdateHelperScript(exe, staging, cfgPath, logPath, resultPath, altResult string) string {
	return fmt.Sprintf(`$ErrorActionPreference='Stop'
$log = '%s'
$resultPath = '%s'
$altResult = '%s'
$helperPid = $PID
function Write-Log($m){ try{ Add-Content -LiteralPath $log -Value ("[{0}] {1}" -f (Get-Date -Format o), $m) -Encoding UTF8 }catch{} }
function Write-Result($m){
  foreach ($p in @($resultPath, $altResult)) {
    if (-not $p) { continue }
    try { Set-Content -LiteralPath $p -Value $m -Encoding UTF8 } catch {}
  }
}
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
  Get-Process -Name $names -ErrorAction SilentlyContinue | Where-Object { $_.Id -ne $helperPid } | Stop-Process -Force -ErrorAction SilentlyContinue
  Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
    Where-Object {
      $_.ProcessId -ne $helperPid -and (
        $_.Name -match '^aiops-agent' -or ($_.ExecutablePath -and $_.ExecutablePath -match 'aiops-agent')
      ) -and (-not ($_.CommandLine -and $_.CommandLine -match 'aiops-agent-update-helper'))
    } |
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
  $vbs = Join-Path $Dir 'start-agent.vbs'
  if (Test-Path -LiteralPath $vbs) {
    Write-Log 'user-mode: start-agent.vbs'
    Start-Process -FilePath "$env:SystemRoot\System32\wscript.exe" -ArgumentList (@('"'+$vbs+'"')) -WorkingDirectory $Dir -WindowStyle Hidden
    Start-Sleep -Seconds 4
    if (Test-AgentRunning) { Write-Log 'user-mode VBS ok'; return $true }
  }
  foreach ($tn in @('AIOpsAgent','AIOpsAgentKeepalive','AIOps Agent')) {
    try {
      $out = & "$env:SystemRoot\System32\schtasks.exe" /Run /TN $tn 2>&1 | Out-String
      Write-Log ("schtasks /Run $tn: " + $out.Trim())
      Start-Sleep -Seconds 4
      if (Test-AgentRunning) { Write-Log ("user-mode schtasks ok: " + $tn); return $true }
    } catch {
      Write-Log ("schtasks $tn: " + $_.Exception.Message)
    }
  }
  Write-Log ('user-mode Start-Process --config ' + $Cfg)
  Start-Process -FilePath $Exe -ArgumentList @('--config', $Cfg) -WorkingDirectory $Dir -WindowStyle Hidden
  Start-Sleep -Seconds 4
  return (Test-AgentRunning)
}
function Restart-AgentService {
  param([string]$Exe,[string]$Cfg,[string]$Dir)
  $hasSvc = Test-AgentServicePresent
  Write-Log ("restart path hasService=$hasSvc cfg=$Cfg")
  if ($hasSvc -and $Cfg -and (Test-Path -LiteralPath $Cfg)) {
    Write-Log ("install-service with config: " + $Cfg)
    $p = Start-Process -FilePath $Exe -ArgumentList @('--install-service','--config', $Cfg) -WorkingDirectory $Dir -Wait -PassThru -WindowStyle Hidden
    if ($p -and $p.ExitCode -eq 0) {
      foreach ($name in (Get-AgentServiceNames)) {
        if (Wait-ServiceState $name 'Running' 45) {
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
      try {
        & "$env:SystemRoot\System32\sc.exe" start $name 2>$null | Out-Null
        Start-Service -Name $name -ErrorAction SilentlyContinue
      } catch {
        Write-Log ("Start-Service $name failed: " + $_.Exception.Message)
        continue
      }
      if (Wait-ServiceState $name 'Running' 45) { Write-Log ("Start-Service ok: " + $name); return $true }
    }
  }
  return (Restart-AgentUserMode -Exe $Exe -Cfg $Cfg -Dir $Dir)
}
try {
  Write-Log ("helper start pid=$helperPid")
  Write-Result ("running " + (Get-Date -Format o))
  # Let the module HTTP response finish before we stop the agent service.
  Start-Sleep -Seconds 3
  $exe = '%s'
  $new = '%s'
  $cfg = '%s'
  $dir = Split-Path -Parent $exe
  $bak = $exe + '.bak'
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
      try {
        & "$env:SystemRoot\System32\sc.exe" stop $name 2>$null | Out-Null
        Stop-Service -Name $name -Force -ErrorAction SilentlyContinue
      } catch {}
      [void](Wait-ServiceState $name 'Stopped' 40)
    }
  }
  Stop-AgentProcesses
  Start-Sleep -Milliseconds 1000
  # Second pass — service recovery may have respawned the old PE.
  Stop-AgentProcesses
  Start-Sleep -Milliseconds 500
  if (Test-Path -LiteralPath $exe) {
    try { Copy-Item -Force -LiteralPath $exe -Destination $bak } catch {
      Write-Log ("backup Copy-Item warning: " + $_.Exception.Message)
    }
  }
  $moved = $false
  for ($i=0; $i -lt 15; $i++) {
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
  $ver = ''
  try { $ver = (& $exe --version 2>$null | Out-String).Trim() } catch {}
  Write-Log ("update ok version=$ver")
  Write-Result ("ok " + (Get-Date -Format o) + " version=" + $ver)
} catch {
  Write-Log ("update failed: " + $_.Exception.Message)
  Write-Result ("fail " + $_.Exception.Message)
  try {
    $exe = '%s'
    $cfg = '%s'
    $dir = Split-Path -Parent $exe
    $bak = $exe + '.bak'
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
  # Keep helper script for a while on failure (operators read TEMP); delete on success.
  try {
    if (Test-Path -LiteralPath $altResult) {
      $t = Get-Content -LiteralPath $altResult -Raw -ErrorAction SilentlyContinue
      if ($t -match '^ok ') { Remove-Item -Force -ErrorAction SilentlyContinue $PSCommandPath }
    }
  } catch {}
  # Cleanup one-shot task (best-effort).
  try { & "$env:SystemRoot\System32\schtasks.exe" /Delete /TN 'AIOpsAgentSelfUpdate' /F 2>$null | Out-Null } catch {}
}
`, psSingleQuote(logPath), psSingleQuote(resultPath), psSingleQuote(altResult),
		psSingleQuote(exe), psSingleQuote(staging), psSingleQuote(cfgPath),
		psSingleQuote(exe), psSingleQuote(cfgPath))
}

func psSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
