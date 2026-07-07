//go:build !windows && !linux && !darwin

package main

// newPTY returns nil on platforms without a native PTY implementation, so the
// terminal falls back to piped stdio. Windows (ConPTY), Linux and macOS
// (openpty) each provide their own newPTY.
func newPTY(cols, rows int) termShell { return nil }
