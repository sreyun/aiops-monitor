//go:build windows

package main

import (
	"fmt"
	"log/slog"
	"syscall"
	"unsafe"
)

// Spawning a SYSTEM helper into the active console session is how RDP-less
// remote-control tools (VNC/AnyDesk/MeshCentral) reach the *secure desktop*
// (lock screen, logon screen, UAC). A plain user-session process cannot open
// Winsta0\Winlogon; only a process running as SYSTEM *inside the interactive
// session* can OpenInputDesktop() it. The service (Session 0, SYSTEM) therefore
// duplicates its own token, retargets it to the active session and launches the
// desktop worker there.

var (
	modKernel32Svc = syscall.NewLazyDLL("kernel32.dll")
	modAdvapi32Svc = syscall.NewLazyDLL("advapi32.dll")
	modWtsapi32Svc = syscall.NewLazyDLL("wtsapi32.dll")

	procWTSGetActiveConsoleSessionId = modKernel32Svc.NewProc("WTSGetActiveConsoleSessionId")
	procGetCurrentProcessSvc         = modKernel32Svc.NewProc("GetCurrentProcess")
	procCloseHandleSvc               = modKernel32Svc.NewProc("CloseHandle")
	procTerminateProcessSvc          = modKernel32Svc.NewProc("TerminateProcess")
	procWaitForSingleObjectSvc       = modKernel32Svc.NewProc("WaitForSingleObject")

	procWTSEnumerateSessionsWSvc = modWtsapi32Svc.NewProc("WTSEnumerateSessionsW")
	procWTSFreeMemorySvc         = modWtsapi32Svc.NewProc("WTSFreeMemory")

	procOpenProcessTokenSvc     = modAdvapi32Svc.NewProc("OpenProcessToken")
	procDuplicateTokenExSvc     = modAdvapi32Svc.NewProc("DuplicateTokenEx")
	procSetTokenInformationSvc  = modAdvapi32Svc.NewProc("SetTokenInformation")
	procCreateProcessAsUserWSvc = modAdvapi32Svc.NewProc("CreateProcessAsUserW")
	procLookupPrivilegeValueW   = modAdvapi32Svc.NewProc("LookupPrivilegeValueW")
	procAdjustTokenPrivileges   = modAdvapi32Svc.NewProc("AdjustTokenPrivileges")
)

type luid struct {
	LowPart  uint32
	HighPart int32
}

type luidAndAttributes struct {
	Luid       luid
	Attributes uint32
}

type tokenPrivileges struct {
	PrivilegeCount uint32
	Privileges     [1]luidAndAttributes
}

const (
	tokenAdjustPrivileges = 0x0020
	sePrivilegeEnabled    = 0x00000002
)

// enableProcessPrivilege enables a named privilege on the current process token.
// LocalSystem holds SeAssignPrimaryTokenPrivilege / SeIncreaseQuotaPrivilege /
// SeTcbPrivilege but they may be disabled by default; CreateProcessAsUser into
// another session needs them enabled.
func enableProcessPrivilege(name string) error {
	curProc, _, _ := procGetCurrentProcessSvc.Call()
	var tok uintptr
	if r, _, e := procOpenProcessTokenSvc.Call(curProc, uintptr(tokenAdjustPrivileges|tokenQuery), uintptr(unsafe.Pointer(&tok))); r == 0 {
		return fmt.Errorf("OpenProcessToken(adjust): %v", e)
	}
	defer procCloseHandleSvc.Call(tok)

	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	var lu luid
	if r, _, e := procLookupPrivilegeValueW.Call(0, uintptr(unsafe.Pointer(namePtr)), uintptr(unsafe.Pointer(&lu))); r == 0 {
		return fmt.Errorf("LookupPrivilegeValue(%s): %v", name, e)
	}
	tp := tokenPrivileges{PrivilegeCount: 1}
	tp.Privileges[0] = luidAndAttributes{Luid: lu, Attributes: sePrivilegeEnabled}
	if r, _, e := procAdjustTokenPrivileges.Call(tok, 0, uintptr(unsafe.Pointer(&tp)), 0, 0, 0); r == 0 {
		return fmt.Errorf("AdjustTokenPrivileges(%s): %v", name, e)
	}
	return nil
}

const (
	tokenDuplicate      = 0x0002
	tokenQuery          = 0x0008
	tokenAssignPrimary  = 0x0001
	tokenAdjustDefault  = 0x0080
	tokenAdjustSessID   = 0x0100
	maximumAllowed      = 0x02000000
	securityImpersonate = 2 // SecurityImpersonation
	tokenPrimary        = 1 // TokenPrimary
	tokenSessionID      = 12 // TokenSessionId (TOKEN_INFORMATION_CLASS)

	createUnicodeEnv   = 0x00000400
	createNoWindow     = 0x08000000
	createNewConsole   = 0x00000010
	createBreakawayJob = 0x01000000

	invalidSession = 0xFFFFFFFF
	waitTimeout    = 0x00000102
)

type startupInfoW struct {
	Cb              uint32
	LpReserved      *uint16
	LpDesktop       *uint16
	LpTitle         *uint16
	DwX             uint32
	DwY             uint32
	DwXSize         uint32
	DwYSize         uint32
	DwXCountChars   uint32
	DwYCountChars   uint32
	DwFillAttribute uint32
	DwFlags         uint32
	WShowWindow     uint16
	CbReserved2     uint16
	LpReserved2     *byte
	HStdInput       uintptr
	HStdOutput      uintptr
	HStdError       uintptr
}

type processInformationW struct {
	HProcess    uintptr
	HThread     uintptr
	DwProcessID uint32
	DwThreadID  uint32
}

// activeConsoleSession returns the session id currently attached to the physical
// console (keyboard/monitor). Returns invalidSession when no one is attached.
func activeConsoleSession() uint32 {
	r, _, _ := procWTSGetActiveConsoleSessionId.Call()
	return uint32(r)
}

type wtsSessionInfoW struct {
	SessionID      uint32
	WinStationName *uint16
	State          uint32
}

const (
	wtsActiveState       = 0 // WTSActive — connected + interactive
	wtsConnectedState    = 1 // WTSConnected — connected, may still be at logon UI
	wtsDisconnectedState = 4 // WTSDisconnected — RDP closed; virtual desktop may still render
)

func wtsStationName(p *uint16) string {
	if p == nil {
		return ""
	}
	return syscall.UTF16ToString((*[256]uint16)(unsafe.Pointer(p))[:])
}

func isRDPStation(name string) bool {
	// RDP-Tcp#0, RDP-Tcp#1, … — anything else is Console / Services / etc.
	return len(name) >= 3 && (name[0] == 'R' || name[0] == 'r') &&
		(name[1] == 'D' || name[1] == 'd') && (name[2] == 'P' || name[2] == 'p')
}

// activeUserSession returns the session whose desktop is actually being rendered.
// Prefer RDP over Console: on headless/Hyper-V hosts the Console session is often
// "Active" but not compositing, while an RDP session holds the real desktop —
// capturing Console then yields solid black frames. Order:
//  1. Active/Connected RDP session
//  2. Active/Connected non-console (any)
//  3. Disconnected RDP (virtual desktop may still render)
//  4. Any disconnected session
//  5. Physical console / fallback
// Session 0 is always skipped.
func activeUserSession() uint32 {
	var pInfo unsafe.Pointer
	var count uint32
	r, _, _ := procWTSEnumerateSessionsWSvc.Call(0, 0, 1,
		uintptr(unsafe.Pointer(&pInfo)), uintptr(unsafe.Pointer(&count)))
	if r == 0 || pInfo == nil {
		return activeConsoleSession()
	}
	defer procWTSFreeMemorySvc.Call(uintptr(pInfo))

	size := unsafe.Sizeof(wtsSessionInfoW{})
	liveRDP, liveOther := uint32(invalidSession), uint32(invalidSession)
	discRDP, discOther := uint32(invalidSession), uint32(invalidSession)
	for i := uint32(0); i < count; i++ {
		si := (*wtsSessionInfoW)(unsafe.Add(pInfo, uintptr(i)*size))
		if si.SessionID == 0 {
			continue
		}
		name := wtsStationName(si.WinStationName)
		rdp := isRDPStation(name)
		switch si.State {
		case wtsActiveState, wtsConnectedState:
			if rdp {
				if liveRDP == invalidSession {
					liveRDP = si.SessionID
				}
			} else if liveOther == invalidSession {
				liveOther = si.SessionID
			}
		case wtsDisconnectedState:
			if rdp {
				if discRDP == invalidSession {
					discRDP = si.SessionID
				}
			} else if discOther == invalidSession {
				discOther = si.SessionID
			}
		}
	}
	for _, id := range []uint32{liveRDP, liveOther, discRDP, discOther} {
		if id != invalidSession {
			return id
		}
	}
	return activeConsoleSession()
}

// deskWorkerProc is a spawned worker process bound to a specific session.
type deskWorkerProc struct {
	handle  uintptr
	pid     uint32
	session uint32
}

func (p *deskWorkerProc) alive() bool {
	if p == nil || p.handle == 0 {
		return false
	}
	r, _, _ := procWaitForSingleObjectSvc.Call(p.handle, 0)
	return uint32(r) == waitTimeout // still running
}

func (p *deskWorkerProc) kill() {
	if p == nil || p.handle == 0 {
		return
	}
	_, _, _ = procTerminateProcessSvc.Call(p.handle, 1)
	// Wait for the process to actually exit before spawning a replacement —
	// otherwise two workers briefly race on deskWait and the dying one can
	// steal the session slot (black / "connected" with no usable frames).
	_, _, _ = procWaitForSingleObjectSvc.Call(p.handle, 3000)
	_, _, _ = procCloseHandleSvc.Call(p.handle)
	p.handle = 0
}

// spawnDesktopWorker launches "<exe> --desktop-worker --config <cfg>" as SYSTEM
// inside the given console session, attached to winsta0\default so it can then
// follow the input desktop (including the secure desktop). Must be called from
// the SYSTEM service.
func spawnDesktopWorker(exePath, cfgPath string, session uint32) (*deskWorkerProc, error) {
	if session == invalidSession {
		return nil, fmt.Errorf("无活动控制台会话")
	}

	// Ensure the privileges CreateProcessAsUser needs are enabled (best-effort).
	for _, p := range []string{"SeAssignPrimaryTokenPrivilege", "SeIncreaseQuotaPrivilege", "SeTcbPrivilege"} {
		if err := enableProcessPrivilege(p); err != nil {
			slog.Debug("启用特权失败(可能已足够)", "priv", p, "err", err)
		}
	}

	// 1) Open our own (SYSTEM) process token.
	curProc, _, _ := procGetCurrentProcessSvc.Call()
	var selfTok uintptr
	r, _, e := procOpenProcessTokenSvc.Call(
		curProc,
		uintptr(tokenDuplicate|tokenQuery|tokenAssignPrimary|tokenAdjustDefault|tokenAdjustSessID),
		uintptr(unsafe.Pointer(&selfTok)),
	)
	if r == 0 {
		return nil, fmt.Errorf("OpenProcessToken 失败: %v", e)
	}
	defer procCloseHandleSvc.Call(selfTok)

	// 2) Duplicate it into a primary token we can retarget.
	var dupTok uintptr
	r, _, e = procDuplicateTokenExSvc.Call(
		selfTok,
		uintptr(maximumAllowed),
		0,
		uintptr(securityImpersonate),
		uintptr(tokenPrimary),
		uintptr(unsafe.Pointer(&dupTok)),
	)
	if r == 0 {
		return nil, fmt.Errorf("DuplicateTokenEx 失败: %v", e)
	}
	defer procCloseHandleSvc.Call(dupTok)

	// 3) Retarget the token to the active console session.
	sess := session
	r, _, e = procSetTokenInformationSvc.Call(
		dupTok,
		uintptr(tokenSessionID),
		uintptr(unsafe.Pointer(&sess)),
		uintptr(unsafe.Sizeof(sess)),
	)
	if r == 0 {
		return nil, fmt.Errorf("SetTokenInformation(SessionId=%d) 失败: %v", session, e)
	}

	// 4) Build command line: "exe" --desktop-worker --config "cfg"
	cmdline := fmt.Sprintf(`"%s" --desktop-worker`, exePath)
	if cfgPath != "" {
		cmdline += fmt.Sprintf(` --config "%s"`, cfgPath)
	}
	appW, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return nil, err
	}
	cmdW, err := syscall.UTF16PtrFromString(cmdline)
	if err != nil {
		return nil, err
	}
	deskW, err := syscall.UTF16PtrFromString(`winsta0\default`)
	if err != nil {
		return nil, err
	}

	si := startupInfoW{}
	si.Cb = uint32(unsafe.Sizeof(si))
	si.LpDesktop = deskW
	var pi processInformationW

	r, _, e = procCreateProcessAsUserWSvc.Call(
		dupTok,
		uintptr(unsafe.Pointer(appW)),
		uintptr(unsafe.Pointer(cmdW)),
		0, 0, 0,
		uintptr(createUnicodeEnv|createNoWindow),
		0, 0,
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CreateProcessAsUser 失败(session=%d): %v", session, e)
	}
	_, _, _ = procCloseHandleSvc.Call(pi.HThread)
	slog.Info("已在活动会话派生远程桌面 worker", "session", session, "pid", pi.DwProcessID)
	return &deskWorkerProc{handle: pi.HProcess, pid: pi.DwProcessID, session: session}, nil
}
