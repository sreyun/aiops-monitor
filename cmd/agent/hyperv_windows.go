//go:build windows

package main

import (
	"strings"
	"time"
	"unsafe"

	"aiops-monitor/shared"
)

const hypervCollectTimeout = 30 * time.Second

// hypervProbeScript reports whether this host is a Hyper-V HOST. It checks for
// the vmms (Hyper-V Virtual Machine Management) service rather than the Get-VM
// cmdlet: Get-Command Get-VM triggers a lazy Hyper-V-module autoload that is slow
// on cold boot (esp. Windows Server 2012) and runs fresh in every probe process,
// so it kept losing a boot-time race and reporting "not a Hyper-V host". Querying
// the service is instant, autoloads nothing, and is only present on an actual
// Hyper-V host (a management-tools-only box has Get-VM but no vmms and no local
// VMs to collect). Printed "yes" ⇒ available.
const hypervProbeScript = `if (Get-Service -Name vmms -ErrorAction SilentlyContinue) { 'yes' }`

// hypervScript collects every guest in a SINGLE powershell process: Get-VM once,
// then network adapters and checkpoints each fetched with one pipeline and
// grouped by VMName (no per-VM N+1). IP / switch lists are emitted comma-joined
// as strings so the Go side never hits PS 5.1's "a single-element array property
// serializes as a scalar" JSON quirk. All health-relevant fields (State,
// ReplicationState/Health) are non-localized enums, so parsing is locale-safe.
const hypervScript = `$ErrorActionPreference='SilentlyContinue'
$ProgressPreference='SilentlyContinue'
try { $vms=@(Get-VM -ErrorAction Stop) } catch {
  $adm=$false
  try { $adm=([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltinRole]::Administrator) } catch {}
  ConvertTo-Json -InputObject @{'__hyperv_error__'=$true;'elevated'=$adm;'message'=[string]$_.Exception.Message} -Compress
  exit 0
}
# Per-VM CPU via locale-safe WMI perf counters. Get-VM.CPUUsage is frequently 0/stale
# (the "host CPU shows 0%" complaint); Get-Counter paths are LOCALIZED on non-English
# Windows (Chinese 2012 R2 breaks the English path), but WMI class/property names are
# not. "% Guest Run Time" per virtual processor, summed per VM: /hostLP = share of the
# whole host's CPU; /vCPU = the guest's own utilization (0~100). Falls back to
# $vm.CPUUsage per-VM when the perf class yields nothing, so it is never worse than before.
$hostLP=0
try { $hostLP=[int](Get-CimInstance Win32_ComputerSystem -ErrorAction Stop).NumberOfLogicalProcessors } catch {}
if($hostLP -le 0){ try { $hostLP=[int]$env:NUMBER_OF_PROCESSORS } catch {} }
if($hostLP -le 0){ $hostLP=1 }
$vpSum=@{}
try {
  foreach($o in @(Get-CimInstance -ClassName Win32_PerfFormattedData_HvStats_HyperVHypervisorVirtualProcessor -ErrorAction Stop)){
    $nm=[string]$o.Name
    if(-not $nm -or $nm -eq '_Total'){ continue }
    $vn=$nm -replace ':Hv VP \d+$',''
    if($vn -eq $nm){ continue }
    $vpSum[$vn]=[double]($vpSum[$vn]) + [double]$o.PercentGuestRunTime
  }
} catch {}
$nicMap=@{}; $net=@{}; $sw=@{}
foreach($a in ($vms | Get-VMNetworkAdapter)){
  $vn=$a.VMName
  $ipList=@($a.IPAddresses | Where-Object {$_})
  if(-not $nicMap.ContainsKey($vn)){ $nicMap[$vn]=@() }
  $nicMap[$vn]+=[PSCustomObject]@{ Name=[string]$a.Name; MAC=[string]$a.MacAddress; Switch=[string]$a.SwitchName; Status=[string]$a.Status; Connected=[bool]$a.SwitchName; IP=($ipList -join ',') }
  foreach($ip in $ipList){ $net[$vn]=@($net[$vn])+$ip }
  if($a.SwitchName){ $sw[$vn]=@($sw[$vn])+$a.SwitchName }
}
$cpMap=@{}
foreach($s in ($vms | Get-VMSnapshot)){
  $vn=$s.VMName
  if(-not $cpMap.ContainsKey($vn)){ $cpMap[$vn]=@() }
  $ct=''
  if($s.CreationTime){ $ct=$s.CreationTime.ToString('yyyy-MM-ddTHH:mm:ss') }
  $cpMap[$vn]+=[PSCustomObject]@{ Name=[string]$s.Name; Created=$ct; Parent=[string]$s.ParentSnapshotName }
}
$out=foreach($vm in $vms){
  $n=$vm.Name
  $cpuHost=[double]$vm.CPUUsage
  $cpuGuest=0.0
  if($vpSum.ContainsKey($n)){
    $g2=[double]$vpSum[$n]
    $cpuHost=[math]::Round($g2/$hostLP,1)
    $vc=[int]$vm.ProcessorCount
    if($vc -gt 0){ $cpuGuest=[math]::Round($g2/$vc,1) }
  }
  $disks=@()
  foreach($d in @($vm.HardDrives)){
    $fs=0
    if($d.Path){ $fi=Get-Item -LiteralPath $d.Path -ErrorAction SilentlyContinue; if($fi){ $fs=[math]::Round($fi.Length/1GB,1) } }
    $disks+=[PSCustomObject]@{ Path=[string]$d.Path; ControllerType=[string]$d.ControllerType; ControllerNumber=[int]$d.ControllerNumber; ControllerLocation=[int]$d.ControllerLocation; FileSizeGB=[double]$fs }
  }
  [PSCustomObject]@{
    Name=$n
    Id=[string]$vm.Id
    State=[string]$vm.State
    Status=[string]$vm.Status
    CPUUsage=[double]$cpuHost
    CPUGuestPct=[double]$cpuGuest
    ProcessorCount=[int]$vm.ProcessorCount
    MemAssignedMB=[double]([math]::Round($vm.MemoryAssigned/1MB))
    MemDemandMB=[double]([math]::Round($vm.MemoryDemand/1MB))
    MemStartupMB=[double]([math]::Round($vm.MemoryStartup/1MB))
    MemMinMB=[double]([math]::Round($vm.MemoryMinimum/1MB))
    MemMaxMB=[double]([math]::Round($vm.MemoryMaximum/1MB))
    DynamicMemoryEnabled=[bool]$vm.DynamicMemoryEnabled
    UptimeSec=[int64]$vm.Uptime.TotalSeconds
    Generation=[int]$vm.Generation
    Version=[string]$vm.Version
    IntegrationState=[string]$vm.IntegrationServicesState
    IP=(@($net[$n] | Where-Object {$_} | Select-Object -Unique) -join ',')
    Switches=(@($sw[$n] | Where-Object {$_} | Select-Object -Unique) -join ',')
    VHDCount=@($vm.HardDrives).Count
    CheckpointCount=@($cpMap[$n]).Count
    ReplState=[string]$vm.ReplicationState
    ReplHealth=[string]$vm.ReplicationHealth
    Nics=@($nicMap[$n] | Where-Object {$_})
    Disks=@($disks | Where-Object {$_})
    Checkpoints=@($cpMap[$n] | Where-Object {$_})
  }
}
if($out){ ConvertTo-Json -InputObject @($out) -Depth 6 -Compress } else { '[]' }`

// hypervAvailable reports whether Hyper-V guest collection can run here. The probe
// (a vmms service lookup) is instant, but keep a modest timeout so a momentarily
// wedged PowerShell at boot just fails this attempt; the caller re-probes with
// backoff so a transient failure never permanently disables collection.
func hypervAvailable() bool {
	out, _ := runCmdTimeout(15*time.Second, "powershell", "-NoProfile", "-NonInteractive", "-Command", hypervProbeScript)
	return strings.Contains(out, "yes")
}

// hypervCollect shells out to PowerShell and parses the guest inventory, plus the
// physical host's own memory (for the "宿主机名 · 可用/总内存" display).
func hypervCollect() ([]shared.HyperVGuest, hypervHostStats, error) {
	var hs hypervHostStats
	out, err := runCmdTimeout(hypervCollectTimeout, "powershell", "-NoProfile", "-NonInteractive", "-Command", hypervScript)
	if err != nil {
		return nil, hs, err
	}
	guests, perr := parseHyperV(out)
	if perr != nil {
		return nil, hs, perr
	}
	hs.TotalMemMB, hs.AvailMemMB = hostPhysMemMB()
	return guests, hs, nil
}

// hostPhysMemMB returns the physical host's total and available RAM in MB via
// GlobalMemoryStatusEx (memoryStatusEx / procGlobalMemoryStatusEx are shared with
// the base collector). Instant, no locale dependency.
func hostPhysMemMB() (total, avail float64) {
	var msx memoryStatusEx
	msx.length = uint32(unsafe.Sizeof(msx))
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&msx))); r != 0 {
		total = float64(msx.totalPhys) / (1024 * 1024)
		avail = float64(msx.availPhys) / (1024 * 1024)
	}
	return
}
