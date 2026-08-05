//go:build !linux

package main

func linuxInteractiveShellInvocation(sh string, shArgs []string, dir string) (string, []string, bool) {
	return sh, shArgs, false
}

func linuxInteractiveShellInvocationPlain(sh string, shArgs []string) (string, []string, bool) {
	return sh, shArgs, false
}

func etcWritable() bool { return true }

func termPrivilegeDiag() string { return "" }
