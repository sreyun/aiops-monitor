//go:build !windows

package main

// newPTY returns nil on platforms without a native PTY implementation yet, so
// the terminal falls back to piped stdio. (Windows ConPTY lives in
// pty_windows.go; Linux/macOS openpty will replace this stub.)
func newPTY(cols, rows int) termShell { return nil }
