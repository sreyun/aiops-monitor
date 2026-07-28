//go:build windows

package main

// windowsNeedsLegacyAgentBuild is true on kernels that cannot run Go ≥1.21
// binaries (Windows 8/8.1 and Server 2012/2012 R2 → major=6, minor=2/3).
func windowsNeedsLegacyAgentBuild() bool {
	maj, min, _, _ := winVersion()
	return maj == 6 && min >= 2 && min <= 3
}
