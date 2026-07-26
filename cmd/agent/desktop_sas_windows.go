//go:build windows

package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Secure Attention Sequence (Ctrl+Alt+Del) for Windows — especially Server + RDP.
//
// Win10/11 often accept SendSAS after ImpersonateNamedPipeClient. Windows Server
// frequently ignores SendSAS from a CreateProcessAsUser(SYSTEM) helper because it
// is not the SCM service process. UltraVNC / Veyon solve this with:
//  1) SoftwareSASGeneration forced on
//  2) Duplicate the session's winlogon.exe token and CreateProcessAsUser into
//     winsta0\Winlogon
//  3) From that Winlogon desktop thread: SendSAS(FALSE) AND PostMessage
//     HWND_BROADCAST WM_HOTKEY(Ctrl+Alt+Del) — the classic NT logon trap.
//
// The Session-0 Agent service also impersonates the winlogon token and calls
// SendSAS itself (true service context per MSDN).

const (
	deskSASPipeName = `\\.\pipe\aiops-monitor-sas-v1`

	regOptionNonVolatile = 0
	keySetValue          = 0x0002
	keyQueryValue        = 0x0001
	keyCreateSubKey      = 0x0004
	regDWORD             = 4
	errorPipeBusy        = 231
	genericRead          = 0x80000000
	genericWrite         = 0x40000000
	openExisting         = 3
	fileAttributeNormal  = 0x80
	pipeAccessDuplex     = 0x00000003
	pipeTypeByte         = 0x00000000
	pipeReadModeByte     = 0x00000000
	pipeWait             = 0x00000000
	pipeRejectRemote     = 0x00000008
	pipeUnlimitedInst    = 255
	pipeConnectedErr     = 535

	wmHotkey       = 0x0312
	modAlt         = 0x0001
	modControl     = 0x0002
	vkDelete       = 0x2E
	hwndBroadcast  = 0xFFFF
	processQueryInfo = 0x0400
	processQueryLimited = 0x1000
)

var (
	modAdvapi32SAS = syscall.NewLazyDLL("advapi32.dll")
	modKernel32SAS = syscall.NewLazyDLL("kernel32.dll")
	modWtsapi32SAS = syscall.NewLazyDLL("wtsapi32.dll")
	modUser32SAS   = syscall.NewLazyDLL("user32.dll")

	procRegCreateKeyExW  = modAdvapi32SAS.NewProc("RegCreateKeyExW")
	procRegSetValueExW   = modAdvapi32SAS.NewProc("RegSetValueExW")
	procRegQueryValueExW = modAdvapi32SAS.NewProc("RegQueryValueExW")
	procRegCloseKey      = modAdvapi32SAS.NewProc("RegCloseKey")

	procCreateNamedPipeW            = modKernel32SAS.NewProc("CreateNamedPipeW")
	procConnectNamedPipe            = modKernel32SAS.NewProc("ConnectNamedPipe")
	procDisconnectNamedPipe         = modKernel32SAS.NewProc("DisconnectNamedPipe")
	procCreateFileW                 = modKernel32SAS.NewProc("CreateFileW")
	procWriteFileSAS                = modKernel32SAS.NewProc("WriteFile")
	procReadFileSAS                 = modKernel32SAS.NewProc("ReadFile")
	procCloseHandleSAS              = modKernel32SAS.NewProc("CloseHandle")
	procWaitNamedPipeW              = modKernel32SAS.NewProc("WaitNamedPipeW")
	procImpersonateNamedPipeClient  = modAdvapi32SAS.NewProc("ImpersonateNamedPipeClient")
	procRevertToSelf                = modAdvapi32SAS.NewProc("RevertToSelf")
	procImpersonateLoggedOnUser     = modAdvapi32SAS.NewProc("ImpersonateLoggedOnUser")
	procGetNamedPipeClientSessionId = modKernel32SAS.NewProc("GetNamedPipeClientSessionId")
	procGetNamedPipeClientProcessId = modKernel32SAS.NewProc("GetNamedPipeClientProcessId")
	procOpenProcessTokenSAS         = modAdvapi32SAS.NewProc("OpenProcessToken")
	procDuplicateTokenExSAS         = modAdvapi32SAS.NewProc("DuplicateTokenEx")
	procSetTokenInformationSAS      = modAdvapi32SAS.NewProc("SetTokenInformation")
	procOpenProcessSAS              = modKernel32SAS.NewProc("OpenProcess")
	procWTSQueryUserToken           = modWtsapi32SAS.NewProc("WTSQueryUserToken")
	procWTSEnumerateProcessesW      = modWtsapi32SAS.NewProc("WTSEnumerateProcessesW")
	procWTSFreeMemorySAS            = modWtsapi32SAS.NewProc("WTSFreeMemory")
	procPostMessageW = modUser32SAS.NewProc("PostMessageW")
)

const (
	tokenQuerySAS         = 0x0008
	tokenDuplicateSAS     = 0x0002
	tokenAssignPrimarySAS = 0x0001
	tokenAdjustDefaultSAS = 0x0080
	tokenAdjustSessIDSAS  = 0x0100
	tokenAllAccessSAS     = 0xF01FF
	securityImpersonation = 2
	tokenPrimaryKind      = 1
	tokenSessionIDSAS     = 12
	maximumAllowedSAS     = 0x02000000
)

type wtsProcessInfoW struct {
	SessionID   uint32
	ProcessID   uint32
	ProcessName *uint16
	UserSid     uintptr
}

// ensureSoftwareSASPolicy forces SoftwareSASGeneration=3 (Services + Ease of Access).
// Domain GPOs on Server often leave this unset (default blocks service SAS).
// RealVNC-style: rewrite immediately before every CAD attempt.
func ensureSoftwareSASPolicy() error {
	if err := setPolicyDWORD(
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`,
		"SoftwareSASGeneration", 3,
	); err != nil {
		return err
	}
	// Server 2016/2019: without this, CAD can flash a blank secure desktop.
	_ = setPolicyDWORD(
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`,
		"EnableSecureCredentialPrompting", 1,
	)
	return nil
}

func setPolicyDWORD(subKey, valueName string, want uint32) error {
	path, err := syscall.UTF16PtrFromString(subKey)
	if err != nil {
		return err
	}
	var hKey uintptr
	var disp uint32
	r, _, e := procRegCreateKeyExW.Call(
		uintptr(syscall.HKEY_LOCAL_MACHINE),
		uintptr(unsafe.Pointer(path)),
		0, 0, regOptionNonVolatile,
		uintptr(keySetValue|keyQueryValue|keyCreateSubKey),
		0,
		uintptr(unsafe.Pointer(&hKey)),
		uintptr(unsafe.Pointer(&disp)),
	)
	if r != 0 {
		return fmt.Errorf("RegCreateKeyEx(%s): win32=%d (%v)", subKey, r, e)
	}
	defer procRegCloseKey.Call(hKey)

	name, _ := syscall.UTF16PtrFromString(valueName)
	var typ, dataLen, cur uint32
	dataLen = 4
	qr, _, _ := procRegQueryValueExW.Call(
		hKey, uintptr(unsafe.Pointer(name)), 0,
		uintptr(unsafe.Pointer(&typ)),
		uintptr(unsafe.Pointer(&cur)),
		uintptr(unsafe.Pointer(&dataLen)),
	)
	if qr == 0 && typ == regDWORD && cur == want {
		return nil
	}
	sr, _, se := procRegSetValueExW.Call(
		hKey, uintptr(unsafe.Pointer(name)), 0, regDWORD,
		uintptr(unsafe.Pointer(&want)), 4,
	)
	if sr != 0 {
		return fmt.Errorf("RegSetValueEx(%s): win32=%d (%v)", valueName, sr, se)
	}
	slog.Info("已写入策略 DWORD", "key", valueName, "value", want, "prev", cur)
	return nil
}

func callSendSAS(asUser bool) {
	if err := modSas.Load(); err != nil {
		slog.Warn("加载 sas.dll 失败", "err", err)
		return
	}
	flag := uintptr(0)
	if asUser {
		flag = 1
	}
	_, _, _ = procSendSAS.Call(flag)
}

func fireSendSASService() {
	// From a true SCM service (or while impersonating), AsUser=FALSE is correct.
	callSendSAS(false)
	time.Sleep(80 * time.Millisecond)
	callSendSAS(false)
}

func postCADHotkey() {
	// UltraVNC classic: Winlogon traps Ctrl+Alt+Del via WM_HOTKEY.
	lparam := uintptr(modAlt|modControl) | (uintptr(vkDelete) << 16)
	_, _, _ = procPostMessageW.Call(hwndBroadcast, wmHotkey, 0, lparam)
	time.Sleep(40 * time.Millisecond)
	_, _, _ = procPostMessageW.Call(hwndBroadcast, wmHotkey, 0, lparam)
}

// injectSecureAttentionSequence is called from the desktop worker input thread.
func injectSecureAttentionSequence() error {
	if err := ensureSoftwareSASPolicy(); err != nil {
		slog.Warn("设置 SoftwareSASGeneration 失败（仍尝试注入）", "err", err)
	}
	if err := modSas.Load(); err != nil {
		return fmt.Errorf("加载 sas.dll 失败: %w", err)
	}

	sid := currentSessionID()
	var pipeErr error
	if err := requestSASFromService(sid, 6*time.Second); err == nil {
		slog.Info("服务管道 SAS 已受理", "session", sid)
	} else {
		pipeErr = err
		slog.Warn("服务管道 SAS 失败", "err", err, "session", sid)
	}

	// Always also run the in-session Winlogon hotkey path from this worker —
	// Server often needs WM_HOTKEY even when the service reported OK.
	if err := injectCADOnWinlogonDesktop(); err != nil {
		slog.Warn("本机会话 Winlogon CAD 失败", "err", err)
		if pipeErr != nil {
			return fmt.Errorf("CAD 注入失败: pipe=%v; local=%v", pipeErr, err)
		}
	}
	slog.Info("已完成本地 Winlogon CAD 注入", "desktop", threadDesktopName(), "session", sid)
	return nil
}

// injectCADOnWinlogonDesktop switches this thread onto Winlogon and fires both
// SendSAS and WM_HOTKEY (UltraVNC-compatible).
func injectCADOnWinlogonDesktop() error {
	runtime.LockOSThread()
	var h uintptr
	var err error
	h, err = openNamedDesktop("Winlogon")
	if err != nil || h == 0 {
		h, err = openInputDesktop()
	}
	if err != nil || h == 0 {
		return fmt.Errorf("无法打开 Winlogon/输入桌面: %v", err)
	}
	defer procCloseDesktop.Call(h)
	if r, _, e := procSetThreadDesktop.Call(h); r == 0 {
		return fmt.Errorf("SetThreadDesktop(Winlogon) 失败: %v", e)
	}
	callSendSAS(false)
	time.Sleep(50 * time.Millisecond)
	postCADHotkey()
	time.Sleep(50 * time.Millisecond)
	callSendSAS(false)
	postCADHotkey()
	return nil
}

// runSendSASOnce is spawned by the service with a winlogon token into winsta0\Winlogon.
func runSendSASOnce() error {
	runtime.LockOSThread()
	_ = ensureSoftwareSASPolicy()
	if err := modSas.Load(); err != nil {
		return err
	}
	if err := injectCADOnWinlogonDesktop(); err != nil {
		slog.Warn("send-sas Winlogon 路径失败，尝试仅 SendSAS", "err", err)
		fireSendSASService()
		postCADHotkey()
	}
	slog.Info("send-sas helper 完成", "session", currentSessionID(), "desktop", threadDesktopName())
	return nil
}

func requestSASFromService(session uint32, timeout time.Duration) error {
	namePtr, err := syscall.UTF16PtrFromString(deskSASPipeName)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	msg := []byte(fmt.Sprintf("SAS %d\n", session))
	for {
		h, _, e := procCreateFileW.Call(
			uintptr(unsafe.Pointer(namePtr)),
			uintptr(genericRead|genericWrite),
			0, 0, openExisting, fileAttributeNormal, 0,
		)
		if h != 0 && h != uintptr(syscall.InvalidHandle) {
			defer procCloseHandleSAS.Call(h)
			var written uint32
			r, _, we := procWriteFileSAS.Call(h, uintptr(unsafe.Pointer(&msg[0])), uintptr(len(msg)),
				uintptr(unsafe.Pointer(&written)), 0)
			if r == 0 {
				return fmt.Errorf("WriteFile(SAS pipe): %v", we)
			}
			buf := make([]byte, 128)
			var n uint32
			r, _, re := procReadFileSAS.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
				uintptr(unsafe.Pointer(&n)), 0)
			if r == 0 {
				return fmt.Errorf("ReadFile(SAS pipe): %v", re)
			}
			resp := string(buf[:n])
			if strings.HasPrefix(resp, "OK") {
				return nil
			}
			return fmt.Errorf("服务 SAS 响应: %q", strings.TrimSpace(resp))
		}
		if errno, ok := e.(syscall.Errno); ok && errno == errorPipeBusy {
			_, _, _ = procWaitNamedPipeW.Call(uintptr(unsafe.Pointer(namePtr)), 1000)
		} else if time.Now().After(deadline) {
			return fmt.Errorf("连接 SAS 管道失败: %v（确认 Agent 以 Windows 服务运行）", e)
		} else {
			time.Sleep(120 * time.Millisecond)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("连接 SAS 管道超时（确认 Agent 以 Windows 服务运行）")
		}
	}
}

func serveSASPipe(stop <-chan struct{}) {
	if err := ensureSoftwareSASPolicy(); err != nil {
		slog.Warn("服务启动时启用 SoftwareSASGeneration 失败", "err", err)
	}
	namePtr, err := syscall.UTF16PtrFromString(deskSASPipeName)
	if err != nil {
		return
	}
	for {
		select {
		case <-stop:
			return
		default:
		}
		h, _, e := procCreateNamedPipeW.Call(
			uintptr(unsafe.Pointer(namePtr)),
			uintptr(pipeAccessDuplex|pipeRejectRemote),
			uintptr(pipeTypeByte|pipeReadModeByte|pipeWait),
			pipeUnlimitedInst,
			256, 256, 0, 0,
		)
		if h == 0 || h == uintptr(syscall.InvalidHandle) {
			slog.Warn("CreateNamedPipe(SAS) 失败", "err", e)
			select {
			case <-stop:
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}

		done := make(chan struct{})
		go func() {
			select {
			case <-stop:
				_, _, _ = procCloseHandleSAS.Call(h)
			case <-done:
			}
		}()
		r, _, ce := procConnectNamedPipe.Call(h, 0)
		close(done)
		connected := r != 0
		if !connected {
			if errno, ok := ce.(syscall.Errno); ok && errno == pipeConnectedErr {
				connected = true
			}
		}
		if !connected {
			_, _, _ = procCloseHandleSAS.Call(h)
			select {
			case <-stop:
				return
			default:
				continue
			}
		}

		handleSASPipeClient(h)
		_, _, _ = procDisconnectNamedPipe.Call(h)
		_, _, _ = procCloseHandleSAS.Call(h)
	}
}

func handleSASPipeClient(h uintptr) {
	buf := make([]byte, 64)
	var n uint32
	r, _, _ := procReadFileSAS.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
		uintptr(unsafe.Pointer(&n)), 0)
	if r == 0 || n < 3 {
		writePipeMsg(h, "ERR read\n")
		return
	}
	req := strings.TrimSpace(string(buf[:n]))
	if !strings.HasPrefix(req, "SAS") {
		writePipeMsg(h, "ERR cmd\n")
		return
	}

	sess := resolveSASSession(h, req)
	if sess == 0 || sess == invalidSession {
		writePipeMsg(h, "ERR nosession\n")
		return
	}

	if err := ensureSoftwareSASPolicy(); err != nil {
		slog.Warn("SAS 请求前策略设置失败", "err", err)
	}
	for _, p := range []string{"SeTcbPrivilege", "SeAssignPrimaryTokenPrivilege", "SeIncreaseQuotaPrivilege", "SeDebugPrivilege"} {
		_ = enableProcessPrivilege(p)
	}

	var paths []string

	// Path A (MSDN): true SCM LocalSystem Session-0 process calling SendSAS.
	// Impersonation is NOT required for this; SoftwareSASGeneration must be on.
	fireSendSASService()
	paths = append(paths, "ServiceDirect")

	// Path B (Server): impersonate winlogon.exe then SendSAS — some builds gate on token.
	if revert, err := impersonateWinlogon(sess); err == nil {
		fireSendSASService()
		revert()
		paths = append(paths, "WinlogonImpersonate")
	} else {
		slog.Warn("Winlogon 模拟失败", "session", sess, "err", err)
	}

	// Path C: logged-on user token (interactive RDP user present).
	if revert, err := impersonateSessionUser(sess); err == nil {
		fireSendSASService()
		revert()
		paths = append(paths, "WTSQueryUserToken")
	}

	// Path D: LocalSystem token retargeted into the interactive session.
	if revert, err := impersonateSystemInSession(sess); err == nil {
		fireSendSASService()
		revert()
		paths = append(paths, "SystemSessionToken")
	}

	// Path E: pipe-client impersonation (often enough on Win10/11).
	if imp, _, _ := procImpersonateNamedPipeClient.Call(h); imp != 0 {
		fireSendSASService()
		_, _, _ = procRevertToSelf.Call()
		paths = append(paths, "PipeClient")
	}

	// Path F (Server belt-and-suspenders): --send-sas under winlogon token on
	// winsta0\Winlogon, then SendSAS + WM_HOTKEY inside that desktop.
	if err := spawnSendSASWithWinlogonToken(sess); err == nil {
		paths = append(paths, "WinlogonHelper")
		time.Sleep(450 * time.Millisecond)
	} else {
		slog.Warn("winlogon-token send-sas 失败，回退 SYSTEM token", "err", err)
		if err2 := spawnSendSASHelper(sess); err2 == nil {
			paths = append(paths, "SystemHelper")
			time.Sleep(450 * time.Millisecond)
		} else {
			slog.Warn("SYSTEM send-sas 也失败", "err", err2)
		}
	}

	if len(paths) == 0 {
		writePipeMsg(h, "ERR allpaths\n")
		slog.Warn("所有 SAS 注入路径均失败", "session", sess)
		return
	}
	slog.Info("服务已注入 SAS", "session", sess, "paths", strings.Join(paths, "+"))
	writePipeMsg(h, "OK\n")
}

func resolveSASSession(h uintptr, req string) uint32 {
	sess := uint32(0)
	parts := strings.Fields(req)
	if len(parts) >= 2 {
		if v, err := strconv.ParseUint(parts[1], 10, 32); err == nil {
			sess = uint32(v)
		}
	}
	var pipeSess uint32
	if r, _, _ := procGetNamedPipeClientSessionId.Call(h, uintptr(unsafe.Pointer(&pipeSess))); r != 0 && pipeSess != 0 {
		if sess == 0 {
			sess = pipeSess
		}
	}
	if sess == 0 {
		var pid uint32
		if r, _, _ := procGetNamedPipeClientProcessId.Call(h, uintptr(unsafe.Pointer(&pid))); r != 0 && pid != 0 {
			var sid uint32
			if r, _, _ := procProcessIdToSessionId.Call(uintptr(pid), uintptr(unsafe.Pointer(&sid))); r != 0 && sid != 0 {
				sess = sid
			}
		}
	}
	if sess == 0 {
		sess = activeUserSession()
	}
	return sess
}

func writePipeMsg(h uintptr, s string) {
	b := []byte(s)
	var written uint32
	_, _, _ = procWriteFileSAS.Call(h, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)),
		uintptr(unsafe.Pointer(&written)), 0)
}

func impersonateSessionUser(session uint32) (func(), error) {
	var tok uintptr
	r, _, e := procWTSQueryUserToken.Call(uintptr(session), uintptr(unsafe.Pointer(&tok)))
	if r == 0 || tok == 0 {
		return nil, fmt.Errorf("WTSQueryUserToken(%d): %v", session, e)
	}
	ir, _, ie := procImpersonateLoggedOnUser.Call(tok)
	if ir == 0 {
		_, _, _ = procCloseHandleSAS.Call(tok)
		return nil, fmt.Errorf("ImpersonateLoggedOnUser: %v", ie)
	}
	return func() {
		_, _, _ = procRevertToSelf.Call()
		_, _, _ = procCloseHandleSAS.Call(tok)
	}, nil
}

func impersonateSystemInSession(session uint32) (func(), error) {
	tok, err := duplicateSystemTokenForSession(session)
	if err != nil {
		return nil, err
	}
	ir, _, ie := procImpersonateLoggedOnUser.Call(tok)
	if ir == 0 {
		_, _, _ = procCloseHandleSAS.Call(tok)
		return nil, fmt.Errorf("ImpersonateLoggedOnUser(system@%d): %v", session, ie)
	}
	return func() {
		_, _, _ = procRevertToSelf.Call()
		_, _, _ = procCloseHandleSAS.Call(tok)
	}, nil
}

func impersonateWinlogon(session uint32) (func(), error) {
	tok, err := duplicateWinlogonToken(session)
	if err != nil {
		return nil, err
	}
	ir, _, ie := procImpersonateLoggedOnUser.Call(tok)
	if ir == 0 {
		_, _, _ = procCloseHandleSAS.Call(tok)
		return nil, fmt.Errorf("ImpersonateLoggedOnUser(winlogon@%d): %v", session, ie)
	}
	return func() {
		_, _, _ = procRevertToSelf.Call()
		_, _, _ = procCloseHandleSAS.Call(tok)
	}, nil
}

func duplicateSystemTokenForSession(session uint32) (uintptr, error) {
	curProc, _, _ := procGetCurrentProcessSvc.Call()
	var selfTok uintptr
	access := uintptr(tokenDuplicateSAS | tokenQuerySAS | tokenAssignPrimarySAS | tokenAdjustDefaultSAS | tokenAdjustSessIDSAS)
	r, _, e := procOpenProcessTokenSAS.Call(curProc, access, uintptr(unsafe.Pointer(&selfTok)))
	if r == 0 {
		r, _, e = procOpenProcessTokenSAS.Call(curProc, uintptr(tokenDuplicateSAS|tokenQuerySAS),
			uintptr(unsafe.Pointer(&selfTok)))
	}
	if r == 0 || selfTok == 0 {
		return 0, fmt.Errorf("OpenProcessToken: %v", e)
	}
	defer procCloseHandleSAS.Call(selfTok)

	var dupTok uintptr
	r, _, e = procDuplicateTokenExSAS.Call(
		selfTok, uintptr(maximumAllowedSAS), 0,
		uintptr(securityImpersonation), uintptr(tokenPrimaryKind),
		uintptr(unsafe.Pointer(&dupTok)),
	)
	if r == 0 || dupTok == 0 {
		return 0, fmt.Errorf("DuplicateTokenEx: %v", e)
	}
	sess := session
	r, _, e = procSetTokenInformationSAS.Call(
		dupTok, uintptr(tokenSessionIDSAS),
		uintptr(unsafe.Pointer(&sess)), uintptr(unsafe.Sizeof(sess)),
	)
	if r == 0 {
		_, _, _ = procCloseHandleSAS.Call(dupTok)
		return 0, fmt.Errorf("SetTokenInformation(SessionId): %v", e)
	}
	return dupTok, nil
}

func findWinlogonPID(session uint32) (uint32, error) {
	if pid, err := findWinlogonPIDViaWTS(session); err == nil {
		return pid, nil
	} else {
		slog.Warn("WTS 枚举 winlogon 失败，尝试 Toolhelp", "session", session, "err", err)
	}
	return findWinlogonPIDViaToolhelp(session)
}

func findWinlogonPIDViaWTS(session uint32) (uint32, error) {
	var pInfo unsafe.Pointer
	var count uint32
	r, _, e := procWTSEnumerateProcessesW.Call(0, 0, 1,
		uintptr(unsafe.Pointer(&pInfo)), uintptr(unsafe.Pointer(&count)))
	if r == 0 || pInfo == nil {
		return 0, fmt.Errorf("WTSEnumerateProcessesW: %v", e)
	}
	defer procWTSFreeMemorySAS.Call(uintptr(pInfo))

	size := unsafe.Sizeof(wtsProcessInfoW{})
	for i := uint32(0); i < count; i++ {
		pi := (*wtsProcessInfoW)(unsafe.Add(pInfo, uintptr(i)*size))
		if pi.SessionID != session || pi.ProcessName == nil {
			continue
		}
		name := strings.ToLower(syscall.UTF16ToString((*[256]uint16)(unsafe.Pointer(pi.ProcessName))[:]))
		if name == "winlogon.exe" {
			return pi.ProcessID, nil
		}
	}
	return 0, fmt.Errorf("session %d 未找到 winlogon.exe", session)
}

func findWinlogonPIDViaToolhelp(session uint32) (uint32, error) {
	const (
		th32csSnapProcess = 0x00000002
		invalidHandleVal  = ^uintptr(0)
	)
	procCreateToolhelp32Snapshot := modKernel32SAS.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW := modKernel32SAS.NewProc("Process32FirstW")
	procProcess32NextW := modKernel32SAS.NewProc("Process32NextW")

	type processEntry32W struct {
		Size            uint32
		Usage           uint32
		ProcessID       uint32
		DefaultHeapID   uintptr
		ModuleID        uint32
		Threads         uint32
		ParentProcessID uint32
		PriClassBase    int32
		Flags           uint32
		ExeFile         [260]uint16
	}

	snap, _, e := procCreateToolhelp32Snapshot.Call(th32csSnapProcess, 0)
	if snap == 0 || snap == invalidHandleVal {
		return 0, fmt.Errorf("CreateToolhelp32Snapshot: %v", e)
	}
	defer procCloseHandleSAS.Call(snap)

	var pe processEntry32W
	pe.Size = uint32(unsafe.Sizeof(pe))
	r, _, e := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&pe)))
	if r == 0 {
		return 0, fmt.Errorf("Process32FirstW: %v", e)
	}
	for {
		name := strings.ToLower(syscall.UTF16ToString(pe.ExeFile[:]))
		if name == "winlogon.exe" {
			var sid uint32
			if rr, _, _ := procProcessIdToSessionId.Call(uintptr(pe.ProcessID), uintptr(unsafe.Pointer(&sid))); rr != 0 && sid == session {
				return pe.ProcessID, nil
			}
		}
		r, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&pe)))
		if r == 0 {
			break
		}
	}
	return 0, fmt.Errorf("session %d 未找到 winlogon.exe (toolhelp)", session)
}

func duplicateWinlogonToken(session uint32) (uintptr, error) {
	pid, err := findWinlogonPID(session)
	if err != nil {
		return 0, err
	}
	hProc, _, e := procOpenProcessSAS.Call(uintptr(processQueryInfo|processQueryLimited), 0, uintptr(pid))
	if hProc == 0 {
		hProc, _, e = procOpenProcessSAS.Call(uintptr(processQueryLimited), 0, uintptr(pid))
	}
	if hProc == 0 {
		return 0, fmt.Errorf("OpenProcess(winlogon pid=%d): %v", pid, e)
	}
	defer procCloseHandleSAS.Call(hProc)

	var procTok uintptr
	access := uintptr(tokenDuplicateSAS | tokenQuerySAS | tokenAssignPrimarySAS | tokenAdjustDefaultSAS | tokenAdjustSessIDSAS | tokenAllAccessSAS)
	r, _, e := procOpenProcessTokenSAS.Call(hProc, access, uintptr(unsafe.Pointer(&procTok)))
	if r == 0 {
		r, _, e = procOpenProcessTokenSAS.Call(hProc, uintptr(tokenDuplicateSAS|tokenQuerySAS|tokenAssignPrimarySAS),
			uintptr(unsafe.Pointer(&procTok)))
	}
	if r == 0 || procTok == 0 {
		return 0, fmt.Errorf("OpenProcessToken(winlogon): %v", e)
	}
	defer procCloseHandleSAS.Call(procTok)

	var dupTok uintptr
	r, _, e = procDuplicateTokenExSAS.Call(
		procTok, uintptr(maximumAllowedSAS), 0,
		uintptr(securityImpersonation), uintptr(tokenPrimaryKind),
		uintptr(unsafe.Pointer(&dupTok)),
	)
	if r == 0 || dupTok == 0 {
		return 0, fmt.Errorf("DuplicateTokenEx(winlogon): %v", e)
	}
	sess := session
	_, _, _ = procSetTokenInformationSAS.Call(
		dupTok, uintptr(tokenSessionIDSAS),
		uintptr(unsafe.Pointer(&sess)), uintptr(unsafe.Sizeof(sess)),
	)
	return dupTok, nil
}

func spawnSendSASHelper(session uint32) error {
	exe := svcExePath
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			return err
		}
	}
	return spawnSessionProcess(exe, `--send-sas`, session)
}

// spawnSendSASWithWinlogonToken launches --send-sas using the session's winlogon
// token on winsta0\Winlogon — the UltraVNC/Veyon pattern that works on Server.
func spawnSendSASWithWinlogonToken(session uint32) error {
	exe := svcExePath
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			return err
		}
	}
	tok, err := duplicateWinlogonToken(session)
	if err != nil {
		return err
	}
	defer procCloseHandleSAS.Call(tok)
	return createProcessAsUserOnDesktops(tok, exe, `--send-sas`, []string{`winsta0\Winlogon`, `winsta0\default`})
}

func createProcessAsUserOnDesktops(tok uintptr, exePath, args string, desktops []string) error {
	cmdline := fmt.Sprintf(`"%s" %s`, exePath, strings.TrimSpace(args))
	appW, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return err
	}
	cmdW, err := syscall.UTF16PtrFromString(cmdline)
	if err != nil {
		return err
	}
	var lastErr error
	for _, deskName := range desktops {
		deskW, err := syscall.UTF16PtrFromString(deskName)
		if err != nil {
			return err
		}
		si := startupInfoW{}
		si.Cb = uint32(unsafe.Sizeof(si))
		si.LpDesktop = deskW
		var pi processInformationW
		r, _, e := procCreateProcessAsUserWSvc.Call(
			tok,
			uintptr(unsafe.Pointer(appW)),
			uintptr(unsafe.Pointer(cmdW)),
			0, 0, 0,
			uintptr(createUnicodeEnv|createNoWindow|createBreakawayJob),
			0, 0,
			uintptr(unsafe.Pointer(&si)),
			uintptr(unsafe.Pointer(&pi)),
		)
		if r != 0 {
			_, _, _ = procCloseHandleSvc.Call(pi.HThread)
			_, _, _ = procCloseHandleSvc.Call(pi.HProcess)
			slog.Info("已用专用令牌派生 send-sas",
				"pid", pi.DwProcessID, "desktop", deskName)
			return nil
		}
		lastErr = e
		slog.Warn("CreateProcessAsUser(send-sas) 失败", "desktop", deskName, "err", e)
	}
	return fmt.Errorf("CreateProcessAsUser(send-sas): %v", lastErr)
}
