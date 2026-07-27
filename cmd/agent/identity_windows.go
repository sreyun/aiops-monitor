//go:build windows

package main

import (
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

const (
	regKeyWOW64_64Key = 0x0100 // KEY_WOW64_64KEY — always read the native 64-bit hive
	regSz             = 1      // REG_SZ
)

// machineIDFromOS reads the Windows MachineGuid via the registry API.
//
// The previous implementation shelled out to reg.exe. AppLocker / WDAC / Software
// Restriction Policies on managed Windows 10/11 and Server fleets commonly deny
// reg.exe while still allowing the agent binary. That produced an empty
// fingerprint, the server rejected registration, and the host never appeared —
// with a Running service and no obvious console error.
func machineIDFromOS() string {
	if id := readMachineGuidRegistry(); id != "" {
		return id
	}
	return readMachineGuidRegExe()
}

func readMachineGuidRegistry() string {
	path, err := syscall.UTF16PtrFromString(`SOFTWARE\Microsoft\Cryptography`)
	if err != nil {
		return ""
	}
	var key syscall.Handle
	err = syscall.RegOpenKeyEx(syscall.HKEY_LOCAL_MACHINE, path, 0, syscall.KEY_READ|regKeyWOW64_64Key, &key)
	if err != nil {
		// Fall back without WOW64 flag (exotic / older kernels).
		err = syscall.RegOpenKeyEx(syscall.HKEY_LOCAL_MACHINE, path, 0, syscall.KEY_READ, &key)
		if err != nil {
			return ""
		}
	}
	defer syscall.RegCloseKey(key)

	name, err := syscall.UTF16PtrFromString("MachineGuid")
	if err != nil {
		return ""
	}
	var typ uint32
	var n uint32 = 256
	buf := make([]uint16, n/2)
	err = syscall.RegQueryValueEx(key, name, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &n)
	if err != nil || typ != regSz || n < 2 {
		return ""
	}
	// n is byte length including the trailing NUL.
	chars := int(n / 2)
	if chars > 0 && buf[chars-1] == 0 {
		chars--
	}
	return strings.TrimSpace(syscall.UTF16ToString(buf[:chars]))
}

func readMachineGuidRegExe() string {
	out, err := exec.Command("reg", "query", `HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid").Output()
	if err != nil {
		return ""
	}
	for _, ln := range strings.Split(string(out), "\n") {
		if strings.Contains(ln, "MachineGuid") {
			if f := strings.Fields(ln); len(f) >= 3 {
				return f[len(f)-1]
			}
		}
	}
	return ""
}
