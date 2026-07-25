//go:build !windows

package main

// Stubs — Windows identity helpers live in module_inspect_windows.go.

func inspectWindowsFQDN(fallback string) string { return fallback }

func inspectWindowsOSIdentity() (pretty, kernel string) {
	return "Windows", "windows"
}
