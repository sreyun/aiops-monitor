//go:build windows

package main

import (
	"strings"
	"time"

	"aiops-monitor/shared"
)

const hypervCollectTimeout = 30 * time.Second

// hypervProbeScript reports whether this host is a Hyper-V host (the Get-VM
// cmdlet exists). Printed "yes" ⇒ available.
const hypervProbeScript = `if (Get-Command Get-VM -ErrorAction SilentlyContinue) { 'yes' }`

// hypervScript collects every guest in a SINGLE powershell process: Get-VM once,
// then network adapters and checkpoints each fetched with one pipeline and
// grouped by VMName (no per-VM N+1). IP / switch lists are emitted comma-joined
// as strings so the Go side never hits PS 5.1's "a single-element array property
// serializes as a scalar" JSON quirk. All health-relevant fields (State,
// ReplicationState/Health) are non-localized enums, so parsing is locale-safe.
const hypervScript = `$ErrorActionPreference='SilentlyContinue'
$ProgressPreference='SilentlyContinue'
$vms=@(Get-VM)
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
  $disks=@()
  foreach($d in @($vm.HardDrives)){
    $fs=0
    if($d.Path){ $fi=Get-Item -LiteralPath $d.Path -ErrorAction SilentlyContinue; if($fi){ $fs=[math]::Round($fi.Length/1GB,1) } }
    $disks+=[PSCustomObject]@{ Path=[string]$d.Path; ControllerType=[string]$d.ControllerType; ControllerNumber=[int]$d.ControllerNumber; ControllerLocation=[int]$d.ControllerLocation; FileSizeGB=[double]$fs }
  }
  [PSCustomObject]@{
    Name=$n
    Id=[string]$vm.Id.Guid
    State=[string]$vm.State
    Status=[string]$vm.Status
    CPUUsage=[double]$vm.CPUUsage
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

// hypervAvailable reports whether Hyper-V guest collection can run here.
func hypervAvailable() bool {
	out, _ := runCmdTimeout(8*time.Second, "powershell", "-NoProfile", "-NonInteractive", "-Command", hypervProbeScript)
	return strings.Contains(out, "yes")
}

// hypervCollect shells out to PowerShell and parses the guest inventory.
func hypervCollect() ([]shared.HyperVGuest, error) {
	out, err := runCmdTimeout(hypervCollectTimeout, "powershell", "-NoProfile", "-NonInteractive", "-Command", hypervScript)
	if err != nil {
		return nil, err
	}
	return parseHyperV(out)
}
