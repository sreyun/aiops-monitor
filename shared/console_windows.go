//go:build windows

package shared

import (
	"io"
	"os"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetStdHandle   = kernel32.NewProc("GetStdHandle")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procWriteConsoleW  = kernel32.NewProc("WriteConsoleW")
	procGetFileType    = kernel32.NewProc("GetFileType")
)

const (
	stdErrorHandle            = uintptr(0xfffffff4) // (HANDLE)-12
	fileTypeChar              = 2
	writeConsoleChunkMaxWChar = 8192
)

// SetupConsoleUTF8 used to force CP 65001 so UTF-8 slog looked right in CMD.
// That interacts badly with Go's WriteFile-based stderr: on CP 65001, Windows
// may report a character count instead of a byte count, causing writers to
// retry and visually duplicate every CJK rune (已已加加载载…). Prefer
// WriteConsoleW via NewConsoleAwareWriter instead; keep this as a no-op so
// older call sites stay harmless.
func SetupConsoleUTF8() {}

type consoleUTF16Writer struct {
	handle syscall.Handle
}

func (w *consoleUTF16Writer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	// Convert whole buffer to UTF-16 once; WriteConsoleW wants wchar units.
	u16 := utf16.Encode([]rune(string(p)))
	for len(u16) > 0 {
		n := len(u16)
		if n > writeConsoleChunkMaxWChar {
			n = writeConsoleChunkMaxWChar
		}
		var written uint32
		r1, _, err := procWriteConsoleW.Call(
			uintptr(w.handle),
			uintptr(unsafe.Pointer(&u16[0])),
			uintptr(n),
			uintptr(unsafe.Pointer(&written)),
			0,
		)
		if r1 == 0 {
			if err != nil {
				return 0, err
			}
			return 0, syscall.EINVAL
		}
		if int(written) >= n {
			u16 = u16[n:]
			continue
		}
		u16 = u16[written:]
	}
	// Must report original UTF-8 byte length (not wchar count) so MultiWriter /
	// slog do not retry remaining bytes.
	return len(p), nil
}

func isConsoleHandle(h syscall.Handle) bool {
	if h == 0 || h == syscall.InvalidHandle {
		return false
	}
	ft, _, _ := procGetFileType.Call(uintptr(h))
	if ft != fileTypeChar {
		return false
	}
	var mode uint32
	r1, _, _ := procGetConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode)))
	return r1 != 0
}

// NewConsoleAwareWriter returns a writer that uses WriteConsoleW when fd is an
// attached Windows console (avoids CP 65001 WriteFile duplication). Otherwise
// it returns the original file (service / redirected / pipe).
func NewConsoleAwareWriter(f *os.File) io.Writer {
	if f == nil {
		return io.Discard
	}
	h, _, _ := procGetStdHandle.Call(stdErrorHandle)
	handle := syscall.Handle(h)
	// Prefer the actual file's FD when available (tests / remapped stderr).
	if fd := syscall.Handle(f.Fd()); isConsoleHandle(fd) {
		return &consoleUTF16Writer{handle: fd}
	}
	if isConsoleHandle(handle) && f == os.Stderr {
		return &consoleUTF16Writer{handle: handle}
	}
	return f
}
