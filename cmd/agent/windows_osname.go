package main

import "fmt"

// Windows product types from OSVERSIONINFOEX.wProductType.
const (
	verNTWorkstation      = 1
	verNTDomainController = 2
	verNTServer           = 3
)

// formatWindowsOSName maps RtlGetVersion fields to a human-readable OS label.
// Distinguishes Server 2012/R2/2016/2019/2022 from Windows 10/11 — previously
// every maj==10 build was mislabeled "Windows 10".
func formatWindowsOSName(maj, min, build uint32, productType byte) string {
	server := productType == verNTServer || productType == verNTDomainController
	var name string
	switch {
	case maj == 6 && min == 1:
		if server {
			name = "Windows Server 2008 R2"
		} else {
			name = "Windows 7"
		}
	case maj == 6 && min == 2:
		if server {
			name = "Windows Server 2012"
		} else {
			name = "Windows 8"
		}
	case maj == 6 && min == 3:
		if server {
			name = "Windows Server 2012 R2"
		} else {
			name = "Windows 8.1"
		}
	case maj == 10 && server:
		switch {
		case build >= 26100:
			// Server 2025 LTSC shares the 26100 kernel line with Windows 11 24H2.
			name = "Windows Server 2025"
		case build >= 20348:
			name = "Windows Server 2022"
		case build >= 17763:
			name = "Windows Server 2019"
		default:
			name = "Windows Server 2016"
		}
	case maj == 10:
		if build >= 22000 {
			name = "Windows 11"
		} else {
			name = "Windows 10"
		}
	default:
		name = "Windows"
	}
	return fmt.Sprintf("%s (Build %d)", name, build)
}
