//go:build !linux

package main

import (
	"os"
	"strings"
)

func linuxInteractiveShellInvocation(sh string, shArgs []string, dir string) (string, []string, bool) {
	return sh, shArgs, false
}

func linuxInteractiveShellInvocationPlain(sh string, shArgs []string) (string, []string, bool) {
	return sh, shArgs, false
}

func etcWritable() bool { return true }

// pathWritable reports whether the process can create files under path.
func pathWritable(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	if fi.IsDir() {
		f, err := os.CreateTemp(path, ".aiops-wtest-*")
		if err != nil {
			return false
		}
		name := f.Name()
		_ = f.Close()
		_ = os.Remove(name)
		return true
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func termPrivilegeDiag() string { return "" }
