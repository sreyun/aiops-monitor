//go:build windows

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// moduleHyperVPower starts/stops/restarts a Hyper-V guest on this host.
// Args: action=start|stop|restart|force_stop; vm_id (GUID) preferred; name fallback.
func moduleHyperVPower(args map[string]string) ([]byte, int) {
	action := strings.ToLower(strings.TrimSpace(args["action"]))
	vmID := strings.TrimSpace(args["vm_id"])
	name := strings.TrimSpace(args["name"])
	if action == "" {
		return []byte("hyperv_power 缺少 action（start|stop|restart|force_stop）"), 1
	}
	if vmID == "" && name == "" {
		return []byte("hyperv_power 缺少 vm_id 或 name"), 1
	}
	sel := hypervVMSelectPS(vmID, name)
	var ps string
	switch action {
	case "start":
		ps = sel + "; Start-VM -VM $vm -ErrorAction Stop; 'ok start ' + $vm.Name"
	case "stop":
		ps = sel + "; Stop-VM -VM $vm -ErrorAction Stop; 'ok stop ' + $vm.Name"
	case "force_stop":
		ps = sel + "; Stop-VM -VM $vm -Force -TurnOff -ErrorAction Stop; 'ok force_stop ' + $vm.Name"
	case "restart":
		ps = sel + "; Restart-VM -VM $vm -Force -ErrorAction Stop; 'ok restart ' + $vm.Name"
	default:
		return []byte("未知 action: " + action), 1
	}
	return runHyperVOpsPS(ps, 120*time.Second)
}

// moduleHyperVSet updates processor count and/or startup memory (MB).
func moduleHyperVSet(args map[string]string) ([]byte, int) {
	vmID := strings.TrimSpace(args["vm_id"])
	name := strings.TrimSpace(args["name"])
	if vmID == "" && name == "" {
		return []byte("hyperv_set 缺少 vm_id 或 name"), 1
	}
	cpuStr := strings.TrimSpace(args["processor_count"])
	memStr := strings.TrimSpace(args["memory_mb"])
	if cpuStr == "" && memStr == "" {
		return []byte("hyperv_set 需要 processor_count 或 memory_mb"), 1
	}
	sel := hypervVMSelectPS(vmID, name)
	var parts []string
	parts = append(parts, sel)
	if cpuStr != "" {
		n, err := strconv.Atoi(cpuStr)
		if err != nil || n < 1 || n > 256 {
			return []byte("processor_count 无效"), 1
		}
		parts = append(parts, fmt.Sprintf(
			`Set-VMProcessor -VM $vm -Count %d -ErrorAction Stop; 'ok cpu=' + [string]%d`, n, n))
	}
	if memStr != "" {
		mb, err := strconv.ParseInt(memStr, 10, 64)
		if err != nil || mb < 32 || mb > 1024*1024 {
			return []byte("memory_mb 无效"), 1
		}
		bytes := mb * 1024 * 1024
		parts = append(parts, fmt.Sprintf(
			`$bytes=%d; Set-VMMemory -VM $vm -StartupBytes $bytes -ErrorAction Stop; 'ok mem_mb=' + [string]%d`, bytes, mb))
	}
	parts = append(parts, `'ok config ' + $vm.Name`)
	return runHyperVOpsPS(strings.Join(parts, "; "), 120*time.Second)
}

func hypervVMSelectPS(vmID, name string) string {
	if vmID != "" {
		id := strings.ReplaceAll(vmID, "'", "''")
		return fmt.Sprintf(`$ErrorActionPreference='Stop'; $vm=Get-VM -Id '%s' -ErrorAction Stop`, id)
	}
	n := strings.ReplaceAll(name, "'", "''")
	return fmt.Sprintf(`$ErrorActionPreference='Stop'; $vm=Get-VM -Name '%s' -ErrorAction Stop`, n)
}

func runHyperVOpsPS(script string, timeout time.Duration) ([]byte, int) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return []byte("hyperv 操作超时"), 1
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return []byte(msg), 1
	}
	return out, 0
}
