//go:build !windows

package shared

import (
	"io"
	"os"
)

// SetupConsoleUTF8 is a no-op on non-Windows.
func SetupConsoleUTF8() {}

// NewConsoleAwareWriter returns f unchanged on non-Windows.
func NewConsoleAwareWriter(f *os.File) io.Writer {
	if f == nil {
		return io.Discard
	}
	return f
}
