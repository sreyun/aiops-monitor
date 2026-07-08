//go:build windows

package shared

import "syscall"

// SetupConsoleUTF8 switches the Windows console I/O code page to UTF-8 (65001).
// On Windows the console defaults to a legacy code page (e.g. GBK / 936), which
// renders UTF-8 log output as garbled text. When there is no console (service /
// VBS / redirected output) these calls are harmless no-ops.
func SetupConsoleUTF8() {
	k := syscall.NewLazyDLL("kernel32.dll")
	k.NewProc("SetConsoleOutputCP").Call(65001)
	k.NewProc("SetConsoleCP").Call(65001)
}
