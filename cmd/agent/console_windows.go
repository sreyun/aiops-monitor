//go:build windows

package main

import "aiops-monitor/shared"

// On Windows the console defaults to a legacy code page (e.g. GBK / 936), which
// renders our UTF-8 log output as garbled text. Switch the console I/O code
// page to UTF-8 (65001) at startup. When there is no console (service / VBS /
// redirected output) these calls are harmless no-ops.
func init() {
	shared.SetupConsoleUTF8()
}
