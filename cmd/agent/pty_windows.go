//go:build windows

package main

import (
	"log"
	"os"
	"syscall"
	"unsafe"
)

// Windows pseudo-console (ConPTY, Windows 10 1809+) backing for the remote
// terminal — a real interactive TTY so colours, line editing, Ctrl+C and
// full-screen programs work. Pure syscall (no cgo, no third-party), matching the
// zero-dependency Win32 approach in collector_windows.go.

var (
	modkernel32c = syscall.NewLazyDLL("kernel32.dll")

	procCreatePseudoConsole               = modkernel32c.NewProc("CreatePseudoConsole")
	procResizePseudoConsole               = modkernel32c.NewProc("ResizePseudoConsole")
	procClosePseudoConsole                = modkernel32c.NewProc("ClosePseudoConsole")
	procCreatePipe                        = modkernel32c.NewProc("CreatePipe")
	procInitializeProcThreadAttributeList = modkernel32c.NewProc("InitializeProcThreadAttributeList")
	procUpdateProcThreadAttribute         = modkernel32c.NewProc("UpdateProcThreadAttribute")
	procDeleteProcThreadAttributeList     = modkernel32c.NewProc("DeleteProcThreadAttributeList")
	procCreateProcessW2                   = modkernel32c.NewProc("CreateProcessW")
	procWaitForSingleObjectT              = modkernel32c.NewProc("WaitForSingleObject")
	procTerminateProcessT                 = modkernel32c.NewProc("TerminateProcess")
	procCloseHandleT                      = modkernel32c.NewProc("CloseHandle")
)

const (
	procThreadAttrPseudoConsole = 0x00020016
	extendedStartupInfoPresent  = 0x00080000
	startfUseStdHandles         = 0x00000100
	infinite                    = 0xFFFFFFFF
)

// startupInfoExW mirrors STARTUPINFOEXW: STARTUPINFOW followed by the attribute
// list pointer.
type startupInfoExW struct {
	startupInfo   syscall.StartupInfo
	attributeList uintptr
}

// coordVal packs a COORD (two SHORTs) into the DWORD-by-value form the ConPTY
// APIs take (X in the low word, Y in the high word).
func coordVal(cols, rows int) uintptr {
	return uintptr(uint32(uint16(cols)) | uint32(uint16(rows))<<16)
}

type conptyShell struct {
	hpc     uintptr
	hProc   syscall.Handle
	hThread syscall.Handle
	inFile  *os.File // write keystrokes here → ConPTY input
	outFile *os.File // read shell output here ← ConPTY output
	attrBuf []byte   // keeps the attribute list memory alive
}

// newPTY starts the shell attached to a fresh pseudo console. Returns nil on any
// failure so the caller falls back to piped stdio.
func newPTY(cols, rows int) termShell {
	if err := procCreatePseudoConsole.Find(); err != nil { // ConPTY unavailable (< Win10 1809)
		log.Printf("ConPTY 不可用(将回退管道): %v", err)
		return nil
	}
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 30
	}

	var inR, inW, outR, outW syscall.Handle
	if r, _, _ := procCreatePipe.Call(uintptr(unsafe.Pointer(&inR)), uintptr(unsafe.Pointer(&inW)), 0, 0); r == 0 {
		return nil
	}
	if r, _, _ := procCreatePipe.Call(uintptr(unsafe.Pointer(&outR)), uintptr(unsafe.Pointer(&outW)), 0, 0); r == 0 {
		procCloseHandleT.Call(uintptr(inR))
		procCloseHandleT.Call(uintptr(inW))
		return nil
	}

	var hpc uintptr
	// HRESULT CreatePseudoConsole(COORD, hInput, hOutput, dwFlags, HPCON*)
	hr, _, _ := procCreatePseudoConsole.Call(coordVal(cols, rows), uintptr(inR), uintptr(outW), 0, uintptr(unsafe.Pointer(&hpc)))
	// The pseudo console dups the ends it needs; drop our copies of the pipe ends
	// it now owns so a shell exit propagates EOF to outR.
	procCloseHandleT.Call(uintptr(inR))
	procCloseHandleT.Call(uintptr(outW))
	if hr != 0 || hpc == 0 {
		log.Printf("ConPTY CreatePseudoConsole 失败 hr=0x%x hpc=%d", hr, hpc)
		procCloseHandleT.Call(uintptr(inW))
		procCloseHandleT.Call(uintptr(outR))
		return nil
	}

	// Build a process/thread attribute list carrying the pseudo console handle.
	var listSize uintptr
	procInitializeProcThreadAttributeList.Call(0, 1, 0, uintptr(unsafe.Pointer(&listSize)))
	if listSize == 0 {
		log.Printf("ConPTY InitializeProcThreadAttributeList(size) 返回 0")
		closeConPTY(hpc, inW, outR)
		return nil
	}
	attrBuf := make([]byte, listSize)
	attrList := uintptr(unsafe.Pointer(&attrBuf[0]))
	if r, _, e := procInitializeProcThreadAttributeList.Call(attrList, 1, 0, uintptr(unsafe.Pointer(&listSize))); r == 0 {
		log.Printf("ConPTY InitializeProcThreadAttributeList 失败: %v", e)
		closeConPTY(hpc, inW, outR)
		return nil
	}
	if r, _, e := procUpdateProcThreadAttribute.Call(attrList, 0, procThreadAttrPseudoConsole, hpc, unsafe.Sizeof(hpc), 0, 0); r == 0 {
		log.Printf("ConPTY UpdateProcThreadAttribute 失败: %v", e)
		procDeleteProcThreadAttributeList.Call(attrList)
		closeConPTY(hpc, inW, outR)
		return nil
	}

	var si startupInfoExW
	si.startupInfo.Cb = uint32(unsafe.Sizeof(si))
	si.attributeList = attrList
	// The agent's own stdio may be redirected to a log file; without this the
	// child would inherit a *copy* of that stdout and write its banner/prompt
	// there instead of into the pseudo console. Forcing STARTF_USESTDHANDLES with
	// NULL std handles makes cmd fall back to the pseudo console (CONIN$/CONOUT$).
	si.startupInfo.Flags = startfUseStdHandles

	cmdline, err := syscall.UTF16FromString(shellExe())
	if err != nil {
		procDeleteProcThreadAttributeList.Call(attrList)
		closeConPTY(hpc, inW, outR)
		return nil
	}
	var pi syscall.ProcessInformation
	r, _, _ := procCreateProcessW2.Call(
		0,                                    // application name
		uintptr(unsafe.Pointer(&cmdline[0])), // command line (mutable buffer)
		0, 0,                                 // process / thread security
		0,                                    // bInheritHandles = FALSE (ConPTY passes stdio via the attribute)
		extendedStartupInfoPresent,           // creation flags
		0, 0,                                 // environment / current dir
		uintptr(unsafe.Pointer(&si)),         // lpStartupInfo (STARTUPINFOEX)
		uintptr(unsafe.Pointer(&pi)),         // lpProcessInformation
	)
	procDeleteProcThreadAttributeList.Call(attrList)
	if r == 0 {
		log.Printf("ConPTY CreateProcess 失败")
		closeConPTY(hpc, inW, outR)
		return nil
	}

	log.Printf("ConPTY 已启动 %dx%d (pid=%d)", cols, rows, pi.ProcessId)
	return &conptyShell{
		hpc: hpc, hProc: pi.Process, hThread: pi.Thread,
		inFile:  os.NewFile(uintptr(inW), "conpty-in"),
		outFile: os.NewFile(uintptr(outR), "conpty-out"),
		attrBuf: attrBuf,
	}
}

func closeConPTY(hpc uintptr, inW, outR syscall.Handle) {
	procClosePseudoConsole.Call(hpc)
	procCloseHandleT.Call(uintptr(inW))
	procCloseHandleT.Call(uintptr(outR))
}

// shellExe returns the shell to launch (COMSPEC or cmd.exe).
func shellExe() string {
	if c := os.Getenv("COMSPEC"); c != "" {
		return c
	}
	return "cmd.exe"
}

func (c *conptyShell) Read(b []byte) (int, error)  { return c.outFile.Read(b) }
func (c *conptyShell) Write(b []byte) (int, error) { return c.inFile.Write(b) }
func (c *conptyShell) Resize(cols, rows int) error {
	procResizePseudoConsole.Call(c.hpc, coordVal(cols, rows))
	return nil
}
func (c *conptyShell) Wait() error {
	procWaitForSingleObjectT.Call(uintptr(c.hProc), infinite)
	return nil
}
func (c *conptyShell) Close() error {
	procClosePseudoConsole.Call(c.hpc) // signals the shell to exit and closes the pipes
	if c.hProc != 0 {
		procTerminateProcessT.Call(uintptr(c.hProc), 0)
	}
	_ = c.outFile.Close()
	_ = c.inFile.Close()
	if c.hThread != 0 {
		procCloseHandleT.Call(uintptr(c.hThread))
	}
	if c.hProc != 0 {
		procCloseHandleT.Call(uintptr(c.hProc))
	}
	return nil
}
