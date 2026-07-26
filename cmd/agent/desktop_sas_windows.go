//go:build windows

package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// Secure Attention Sequence (Ctrl+Alt+Del) for Windows — especially Server + RDP.
//
// Root cause of "CAD click does nothing" on Windows Server with our remote
// desktop: capture prefers the active RDP session, but SendSAS(FALSE) (LocalSystem)
// targets the *physical console* session. Headless/Hyper-V hosts show the RDP
// lock screen while CAD fires elsewhere — silent no-op for the operator.
//
// Fix (MSDN + UltraVNC/MeshAgent patterns):
//  1) Force SoftwareSASGeneration=3 (64-bit registry view)
//  2) From the SCM service: impersonate the *target session* user/winlogon and
//     call SendSAS(TRUE) so SAS is delivered to that session
//  3) Also SendSAS(FALSE) for console-attached cases
//  4) Spawn --send-sas under winlogon token on winsta0\Winlogon and PostMessage
//     WM_HOTKEY (Ctrl+Alt+Del) to Winlogon windows
//  5) Dual channel: named pipe (with session id) + Global event (UltraVNC-style)

const (
	deskSASPipeName  = `\\.\pipe\aiops-monitor-sas-v1`
	deskSASEventName = `Global\aiops-monitor-sas-cad`

	regOptionNonVolatile = 0
	keySetValue          = 0x0002
	keyQueryValue        = 0x0001
	keyCreateSubKey      = 0x0004
	keyWOW64_64KEY       = 0x0100
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

	wmHotkey             = 0x0312
	modAlt               = 0x0001
	modControl           = 0x0002
	vkDelete             = 0x2E
	hwndBroadcast        = 0xFFFF
	processQueryInfo     = 0x0400
	processQueryLimited  = 0x1000
	processAllAccessSAS  = 0x1F0FFF
	loadLibrarySearchSys = 0x00000800
	eventModifyState     = 0x0002
	eventAllAccess       = 0x1F0003
	infiniteWait         = 0xFFFFFFFF
	waitObject0          = 0
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
	procPostMessageW                = modUser32SAS.NewProc("PostMessageW")
	procLoadLibraryExW              = modKernel32SAS.NewProc("LoadLibraryExW")
	procGetProcAddressSAS           = modKernel32SAS.NewProc("GetProcAddress")
	procFreeLibrarySAS              = modKernel32SAS.NewProc("FreeLibrary")
	procCreateEventW                = modKernel32SAS.NewProc("CreateEventW")
	procOpenEventW                  = modKernel32SAS.NewProc("OpenEventW")
	procSetEventSAS                 = modKernel32SAS.NewProc("SetEvent")
	procWaitForSingleObjectSAS      = modKernel32SAS.NewProc("WaitForSingleObject")
	procResetEventSAS               = modKernel32SAS.NewProc("ResetEvent")
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

// lastSASSession remembers the desktop worker's session for the Global event path.
var lastSASSession atomic.Uint32

// ensureSoftwareSASPolicy forces SoftwareSASGeneration=3 (Services + Ease of Access)
// in the native 64-bit registry view (KEY_WOW64_64KEY). Domain GPOs often leave
// this unset; RealVNC/MeshAgent rewrite it immediately before every CAD attempt.
func ensureSoftwareSASPolicy() error {
	if err := setPolicyDWORD(
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`,
		"SoftwareSASGeneration", 3,
	); err != nil {
		return err
	}
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
	access := uintptr(keySetValue | keyQueryValue | keyCreateSubKey | keyWOW64_64KEY)
	r, _, e := procRegCreateKeyExW.Call(
		uintptr(syscall.HKEY_LOCAL_MACHINE),
		uintptr(unsafe.Pointer(path)),
		0, 0, regOptionNonVolatile,
		access,
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

// callSendSAS loads sas.dll from System32 (MeshAgent/UltraVNC) and invokes SendSAS.
// asUser=TRUE requires a matching impersonation token for the *target* session.
func callSendSAS(asUser bool) error {
	name, err := syscall.UTF16PtrFromString(`sas.dll`)
	if err != nil {
		return err
	}
	h, _, e := procLoadLibraryExW.Call(uintptr(unsafe.Pointer(name)), 0, loadLibrarySearchSys)
	if h == 0 {
		// Server 2008 R2 / Win7 without KB2533623 reject LOAD_LIBRARY_SEARCH_SYSTEM32.
		h, _, e = procLoadLibraryExW.Call(uintptr(unsafe.Pointer(name)), 0, 0)
	}
	if h == 0 {
		// Fall back to Go lazy loader (PATH / already-loaded).
		if err := modSas.Load(); err != nil {
			return fmt.Errorf("加载 sas.dll 失败: %v / %w", e, err)
		}
		flag := uintptr(0)
		if asUser {
			flag = 1
		}
		_, _, _ = procSendSAS.Call(flag)
		return nil
	}
	defer procFreeLibrarySAS.Call(h)

	procName, _ := syscall.BytePtrFromString("SendSAS")
	addr, _, e2 := procGetProcAddressSAS.Call(h, uintptr(unsafe.Pointer(procName)))
	if addr == 0 {
		return fmt.Errorf("GetProcAddress(SendSAS): %v", e2)
	}
	flag := uintptr(0)
	if asUser {
		flag = 1
	}
	syscall.SyscallN(addr, flag)
	return nil
}

func fireSendSAS(asUser bool) {
	if err := callSendSAS(asUser); err != nil {
		slog.Warn("SendSAS 调用失败", "asUser", asUser, "err", err)
		return
	}
	time.Sleep(60 * time.Millisecond)
	_ = callSendSAS(asUser)
}

func cadHotkeyLParam() uintptr {
	return uintptr(modAlt|modControl) | (uintptr(vkDelete) << 16)
}

func postCADHotkey() {
	lparam := cadHotkeyLParam()
	_, _, _ = procPostMessageW.Call(hwndBroadcast, wmHotkey, 0, lparam)
	time.Sleep(30 * time.Millisecond)
	_, _, _ = procPostMessageW.Call(hwndBroadcast, wmHotkey, 0, lparam)
}

// postCADHotkeyToDesktop posts WM_HOTKEY to every top-level window on the
// current thread desktop (must already be attached to Winlogon), then broadcasts.
func postCADHotkeyToDesktop() {
	lparam := cadHotkeyLParam()
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		_, _, _ = procPostMessageW.Call(hwnd, wmHotkey, 0, lparam)
		return 1
	})
	desk, _, _ := procGetThreadDesktop.Call(uintptr(currentThreadID()))
	if desk != 0 {
		_, _, _ = procEnumDesktopWindows.Call(desk, cb, 0)
	}
	postCADHotkey()
}

// injectSecureAttentionSequence is called from the desktop worker input thread.
func injectSecureAttentionSequence() error {
	if err := ensureSoftwareSASPolicy(); err != nil {
		slog.Warn("设置 SoftwareSASGeneration 失败（仍尝试注入）", "err", err)
	}

	sid := currentSessionID()
	lastSASSession.Store(sid)
	signalSASEvent()

	var pipeErr error
	if err := requestSASFromService(sid, 8*time.Second); err == nil {
		slog.Info("服务管道 SAS 已受理", "session", sid)
	} else {
		pipeErr = err
		slog.Warn("服务管道 SAS 失败", "err", err, "session", sid)
	}

	// In-session Winlogon hotkey — Server RDP often needs this even after pipe OK.
	// Do NOT call SendSAS from the worker: only the SCM service is trusted on Server.
	if err := injectCADHotkeyOnWinlogonDesktop(); err != nil {
		slog.Warn("本机会话 Winlogon CAD 热键失败", "err", err)
		if pipeErr != nil {
			return fmt.Errorf("CAD 注入失败: pipe=%v; local=%v", pipeErr, err)
		}
	}
	slog.Info("已完成本地 Winlogon CAD 热键", "desktop", threadDesktopName(), "session", sid)
	return nil
}

func injectCADHotkeyOnWinlogonDesktop() error {
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
	postCADHotkeyToDesktop()
	time.Sleep(40 * time.Millisecond)
	postCADHotkeyToDesktop()
	return nil
}

// runSendSASOnce is spawned by the service with a winlogon token into winsta0\Winlogon.
func runSendSASOnce() error {
	runtime.LockOSThread()
	_ = ensureSoftwareSASPolicy()
	if err := injectCADHotkeyOnWinlogonDesktop(); err != nil {
		slog.Warn("send-sas Winlogon 热键失败", "err", err)
		postCADHotkey()
	}
	// Best-effort: if sas.dll accepts non-service callers on this build, try AsUser.
	_ = callSendSAS(true)
	_ = callSendSAS(false)
	slog.Info("send-sas helper 完成", "session", currentSessionID(), "desktop", threadDesktopName())
	return nil
}

func signalSASEvent() {
	namePtr, err := syscall.UTF16PtrFromString(deskSASEventName)
	if err != nil {
		return
	}
	h, _, _ := procOpenEventW.Call(eventModifyState, 0, uintptr(unsafe.Pointer(namePtr)))
	if h == 0 || h == uintptr(syscall.InvalidHandle) {
		return
	}
	_, _, _ = procSetEventSAS.Call(h)
	_, _, _ = procCloseHandleSAS.Call(h)
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
	go serveSASEvent(stop)

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

// serveSASEvent is the UltraVNC Global\SessionEventUltraCad equivalent: the
// session worker SetEvent's, and the SCM service thread calls SendSAS.
func serveSASEvent(stop <-chan struct{}) {
	namePtr, err := syscall.UTF16PtrFromString(deskSASEventName)
	if err != nil {
		return
	}
	h, _, e := procCreateEventW.Call(0, 0, 0, uintptr(unsafe.Pointer(namePtr))) // auto-reset
	if h == 0 || h == uintptr(syscall.InvalidHandle) {
		slog.Warn("CreateEvent(SAS) 失败", "err", e)
		return
	}
	defer procCloseHandleSAS.Call(h)
	slog.Info("SAS Global 事件已就绪", "name", deskSASEventName)

	for {
		// Wait in slices so we can observe stop.
		r, _, _ := procWaitForSingleObjectSAS.Call(h, 500)
		select {
		case <-stop:
			return
		default:
		}
		if r != waitObject0 {
			continue
		}
		sess := lastSASSession.Load()
		if sess == 0 || sess == invalidSession {
			sess = activeUserSession()
		}
		if sess == 0 || sess == invalidSession {
			slog.Warn("SAS 事件触发但无目标会话")
			continue
		}
		slog.Info("SAS Global 事件触发", "session", sess)
		injectSASFromService(sess)
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
	lastSASSession.Store(sess)

	paths := injectSASFromService(sess)
	if len(paths) == 0 {
		writePipeMsg(h, "ERR allpaths\n")
		slog.Warn("所有 SAS 注入路径均失败", "session", sess)
		return
	}
	slog.Info("服务已注入 SAS", "session", sess, "paths", strings.Join(paths, "+"))
	writePipeMsg(h, "OK\n")
}

// injectSASFromService runs every session-targeted SAS path from the SCM process.
func injectSASFromService(sess uint32) []string {
	if err := ensureSoftwareSASPolicy(); err != nil {
		slog.Warn("SAS 请求前策略设置失败", "err", err)
	}
	for _, p := range []string{"SeTcbPrivilege", "SeAssignPrimaryTokenPrivilege", "SeIncreaseQuotaPrivilege", "SeDebugPrivilege"} {
		_ = enableProcessPrivilege(p)
	}

	var paths []string

	// Path A (RDP / lock screen primary): impersonate interactive user → SendSAS(TRUE).
	// MSDN: AsUser=TRUE delivers SAS to the impersonated user's session.
	if revert, err := impersonateSessionUser(sess); err == nil {
		fireSendSAS(true)
		revert()
		paths = append(paths, "UserAsUser")
	} else {
		slog.Warn("会话用户模拟失败（锁屏/未登录时常见）", "session", sess, "err", err)
	}

	// Path B: impersonate that session's winlogon → SendSAS(TRUE).
	// Covers pre-logon and lock screens where WTSQueryUserToken fails or is stale.
	if revert, err := impersonateWinlogon(sess); err == nil {
		fireSendSAS(true)
		time.Sleep(40 * time.Millisecond)
		fireSendSAS(false) // some Server builds still want FALSE under winlogon token
		revert()
		paths = append(paths, "WinlogonAsUser")
	} else {
		slog.Warn("Winlogon 模拟失败", "session", sess, "err", err)
	}

	// Path C: LocalSystem token retargeted into the interactive session → TRUE.
	if revert, err := impersonateSystemInSession(sess); err == nil {
		fireSendSAS(true)
		revert()
		paths = append(paths, "SystemSessionAsUser")
	}

	// Path D: classic console SAS (physical console / MeshAgent UltraVNC style).
	fireSendSAS(false)
	paths = append(paths, "ServiceFalse")

	// Path E: winlogon-token helper on Winlogon desktop (WM_HOTKEY inside session).
	if err := spawnSendSASWithWinlogonToken(sess); err == nil {
		paths = append(paths, "WinlogonHelper")
		time.Sleep(500 * time.Millisecond)
	} else {
		slog.Warn("winlogon-token send-sas 失败，回退 SYSTEM token", "err", err)
		if err2 := spawnSendSASHelper(sess); err2 == nil {
			paths = append(paths, "SystemHelper")
			time.Sleep(500 * time.Millisecond)
		} else {
			slog.Warn("SYSTEM send-sas 也失败", "err", err2)
		}
	}

	return paths
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
		// Prefer worker-supplied session: pipe client session id is often 0 for SYSTEM.
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
	// UltraVNC uses PROCESS_ALL_ACCESS after SeDebugPrivilege / SeTcbPrivilege.
	hProc, _, e := procOpenProcessSAS.Call(uintptr(processAllAccessSAS), 0, uintptr(pid))
	if hProc == 0 {
		hProc, _, e = procOpenProcessSAS.Call(uintptr(processQueryInfo|processQueryLimited), 0, uintptr(pid))
	}
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
