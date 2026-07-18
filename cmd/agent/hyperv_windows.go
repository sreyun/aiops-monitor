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
$net=@{}
$sw=@{}
foreach($a in ($vms | Get-VMNetworkAdapter)){
  if($a.IPAddresses){ foreach($ip in $a.IPAddresses){ if($ip){ $net[$a.VMName]=@($net[$a.VMName])+$ip } } }
  if($a.SwitchName){ $sw[$a.VMName]=@($sw[$a.VMName])+$a.SwitchName }
}
$cp=@{}
foreach($s in ($vms | Get-VMSnapshot)){ $cp[$s.VMName]=[int]$cp[$s.VMName]+1 }
$out=foreach($vm in $vms){
  $n=$vm.Name
  [PSCustomObject]@{
    Name=$n
    Id=[string]$vm.Id.Guid
    State=[string]$vm.State
    Status=[string]$vm.Status
    CPUUsage=[double]$vm.CPUUsage
    ProcessorCount=[int]$vm.ProcessorCount
    MemAssignedMB=[double]([math]::Round($vm.MemoryAssigned/1MB))
    MemDemandMB=[double]([math]::Round($vm.MemoryDemand/1MB))
    MemMaxMB=[double]([math]::Round($vm.MemoryMaximum/1MB))
    DynamicMemoryEnabled=[bool]$vm.DynamicMemoryEnabled
    UptimeSec=[int64]$vm.Uptime.TotalSeconds
    Generation=[int]$vm.Generation
    Version=[string]$vm.Version
    IP=(@($net[$n] | Where-Object {$_} | Select-Object -Unique) -join ',')
    Switches=(@($sw[$n] | Where-Object {$_} | Select-Object -Unique) -join ',')
    VHDCount=@($vm.HardDrives).Count
    CheckpointCount=[int]$cp[$n]
    ReplState=[string]$vm.ReplicationState
    ReplHealth=[string]$vm.ReplicationHealth
  }
}
if($out){ ConvertTo-Json -InputObject @($out) -Depth 4 -Compress } else { '[]' }`

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
