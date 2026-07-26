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

// Secure Attention Sequence (Ctrl+Alt+Del) injection.
//
// Win10/11 often succeed with ImpersonateNamedPipeClient + SendSAS. Windows
// Server (2012–2022) + RDP is stricter:
//   - GetNamedPipeClientSessionId may report 0 for LocalSystem clients → we
//     previously rejected the request (ERR session0) and fell back to a local
//     SendSAS that Server ignores.
//   - SendSAS from Session 0 without a proper interactive-session impersonation
//     targets the wrong session on RD Session Hosts.
// Fix: client sends the real session id; service tries WTSQueryUserToken /
// SYSTEM-token retarget / pipe impersonation / in-session --send-sas helper.

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
)

var (
	modAdvapi32SAS = syscall.NewLazyDLL("advapi32.dll")
	modKernel32SAS = syscall.NewLazyDLL("kernel32.dll")
	modWtsapi32SAS = syscall.NewLazyDLL("wtsapi32.dll")

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
	procWTSQueryUserToken           = modWtsapi32SAS.NewProc("WTSQueryUserToken")
)

const (
	tokenQuerySAS         = 0x0008
	tokenDuplicateSAS     = 0x0002
	securityImpersonation = 2
	tokenPrimaryKind      = 1
	tokenSessionIDSAS     = 12
	maximumAllowedSAS     = 0x02000000
)

// ensureSoftwareSASPolicy sets SoftwareSASGeneration so services may call SendSAS.
// Always refresh when missing/0 — domain GPOs on Server often leave it unset
// (defaults to Ease-of-Access only, which blocks our service injection).
func ensureSoftwareSASPolicy() error {
	path, err := syscall.UTF16PtrFromString(`SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`)
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
		return fmt.Errorf("RegCreateKeyEx(Policies\\System): win32=%d (%v)", r, e)
	}
	defer procRegCloseKey.Call(hKey)

	name, _ := syscall.UTF16PtrFromString("SoftwareSASGeneration")
	var typ, dataLen uint32
	var cur uint32
	dataLen = 4
	qr, _, _ := procRegQueryValueExW.Call(
		hKey, uintptr(unsafe.Pointer(name)), 0,
		uintptr(unsafe.Pointer(&typ)),
		uintptr(unsafe.Pointer(&cur)),
		uintptr(unsafe.Pointer(&dataLen)),
	)
	// 0=None, 1=Services, 2=EaseOfAccess, 3=Both.
	want := uint32(3)
	if qr == 0 && typ == regDWORD {
		switch cur {
		case 3:
			return nil
		case 1:
			want = 1 // already allows services; keep (don't fight admin choice of 1 vs 3)
		case 2:
			want = 3
		default:
			want = 3 // force services+EoA on Server when unset/None
		}
	}
	sr, _, se := procRegSetValueExW.Call(
		hKey, uintptr(unsafe.Pointer(name)), 0, regDWORD,
		uintptr(unsafe.Pointer(&want)), 4,
	)
	if sr != 0 {
		return fmt.Errorf("RegSetValueEx(SoftwareSASGeneration): win32=%d (%v)", sr, se)
	}
	slog.Info("已启用 SoftwareSASGeneration（允许服务模拟 Ctrl+Alt+Del）", "value", want, "prev", cur)
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

func fireSendSASBoth() {
	callSendSAS(false)
	time.Sleep(60 * time.Millisecond)
	callSendSAS(true)
	time.Sleep(60 * time.Millisecond)
	callSendSAS(false)
}

// injectSecureAttentionSequence tries every reliable path to generate CAD.
func injectSecureAttentionSequence() error {
	if err := ensureSoftwareSASPolicy(); err != nil {
		slog.Warn("设置 SoftwareSASGeneration 失败（仍尝试 SendSAS）", "err", err)
	}
	if err := modSas.Load(); err != nil {
		return fmt.Errorf("加载 sas.dll 失败: %w", err)
	}

	sid := currentSessionID()
	if err := requestSASFromService(sid, 5*time.Second); err == nil {
		slog.Info("已通过 Agent 服务注入 SAS (Ctrl+Alt+Del)", "session", sid)
		return nil
	} else {
		slog.Warn("服务管道 SAS 失败，回退本地 SendSAS", "err", err, "session", sid)
	}

	// Attach Winlogon before local SendSAS — matters on Server logon UI.
	if h, err := openInputDesktop(); err == nil && h != 0 {
		_, _, _ = procSetThreadDesktop.Call(h)
		// Keep handle for process lifetime of this call; close after.
		defer procCloseDesktop.Call(h)
	}
	fireSendSASBoth()
	slog.Info("已本地调用 SendSAS (Ctrl+Alt+Del)", "desktop", threadDesktopName(), "session", sid)
	return nil
}

// runSendSASOnce is the in-session helper spawned by the service on Server hosts
// where Session-0 impersonation alone is insufficient.
func runSendSASOnce() error {
	runtime.LockOSThread()
	_ = ensureSoftwareSASPolicy()
	if err := modSas.Load(); err != nil {
		return err
	}
	if h, err := openInputDesktop(); err == nil && h != 0 {
		if r, _, e := procSetThreadDesktop.Call(h); r == 0 {
			slog.Warn("send-sas: SetThreadDesktop 失败", "err", e, "desktop", desktopNameOf(h))
		} else {
			slog.Info("send-sas: 已附着输入桌面", "desktop", desktopNameOf(h), "session", currentSessionID())
		}
		defer procCloseDesktop.Call(h)
	} else if h2, err2 := openNamedDesktop("Winlogon"); err2 == nil && h2 != 0 {
		_, _, _ = procSetThreadDesktop.Call(h2)
		defer procCloseDesktop.Call(h2)
	}
	fireSendSASBoth()
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

// serveSASPipe runs in the Session-0 service.
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

	// Prefer session id from the desktop worker message (authoritative on Server).
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
		// Last resort: client process → ProcessIdToSessionId (SYSTEM-in-session
		// often reports pipe session 0 on Server; the PID usually does not).
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
	if sess == 0 || sess == invalidSession {
		writePipeMsg(h, "ERR nosession\n")
		return
	}

	if err := ensureSoftwareSASPolicy(); err != nil {
		slog.Warn("SAS 请求前策略设置失败", "err", err)
	}
	_ = enableProcessPrivilege("SeTcbPrivilege")

	var paths []string
	ok := false

	// Path A — impersonate logged-on user token for the target session (best on Server RDP).
	if revert, err := impersonateSessionUser(sess); err == nil {
		fireSendSASBoth()
		revert()
		paths = append(paths, "WTSQueryUserToken")
		ok = true
	} else {
		slog.Debug("WTSQueryUserToken 路径不可用", "session", sess, "err", err)
	}

	// Path B — retarget LocalSystem primary token to the session (works at logon UI).
	if revert, err := impersonateSystemInSession(sess); err == nil {
		fireSendSASBoth()
		revert()
		paths = append(paths, "SystemSessionToken")
		ok = true
	} else {
		slog.Debug("SystemSessionToken 路径失败", "session", sess, "err", err)
	}

	// Path C — classic pipe-client impersonation (works on Win10/11).
	if imp, _, ie := procImpersonateNamedPipeClient.Call(h); imp != 0 {
		fireSendSASBoth()
		_, _, _ = procRevertToSelf.Call()
		paths = append(paths, "PipeClient")
		ok = true
	} else {
		slog.Debug("ImpersonateNamedPipeClient 失败", "err", ie)
	}

	// Path D — spawn in-session helper on Winlogon (Server CAD reliability belt).
	if err := spawnSendSASHelper(sess); err == nil {
		paths = append(paths, "InSessionHelper")
		ok = true
		time.Sleep(200 * time.Millisecond)
	} else {
		slog.Warn("派生 send-sas helper 失败", "session", sess, "err", err)
	}

	if !ok {
		writePipeMsg(h, "ERR allpaths\n")
		slog.Warn("所有 SAS 注入路径均失败", "session", sess)
		return
	}
	slog.Info("服务已注入 SAS", "session", sess, "paths", strings.Join(paths, "+"))
	writePipeMsg(h, "OK\n")
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

// impersonateSystemInSession builds a LocalSystem primary token retargeted to
// the interactive session — required when nobody is logged on (Winlogon only).
func impersonateSystemInSession(session uint32) (func(), error) {
	curProc, _, _ := procGetCurrentProcessSvc.Call()
	var selfTok uintptr
	r, _, e := procOpenProcessTokenSAS.Call(
		curProc,
		uintptr(tokenDuplicateSAS|tokenQuerySAS|tokenAssignPrimary|tokenAdjustDefault|tokenAdjustSessID),
		uintptr(unsafe.Pointer(&selfTok)),
	)
	if r == 0 {
		// Fall back to narrower access mask.
		r, _, e = procOpenProcessTokenSAS.Call(curProc, uintptr(tokenDuplicateSAS|tokenQuerySAS),
			uintptr(unsafe.Pointer(&selfTok)))
	}
	if r == 0 || selfTok == 0 {
		return nil, fmt.Errorf("OpenProcessToken: %v", e)
	}
	defer procCloseHandleSAS.Call(selfTok)

	var dupTok uintptr
	r, _, e = procDuplicateTokenExSAS.Call(
		selfTok,
		uintptr(maximumAllowedSAS),
		0,
		uintptr(securityImpersonation),
		uintptr(tokenPrimaryKind),
		uintptr(unsafe.Pointer(&dupTok)),
	)
	if r == 0 || dupTok == 0 {
		return nil, fmt.Errorf("DuplicateTokenEx: %v", e)
	}
	sess := session
	r, _, e = procSetTokenInformationSAS.Call(
		dupTok, uintptr(tokenSessionIDSAS),
		uintptr(unsafe.Pointer(&sess)), uintptr(unsafe.Sizeof(sess)),
	)
	if r == 0 {
		_, _, _ = procCloseHandleSAS.Call(dupTok)
		return nil, fmt.Errorf("SetTokenInformation(SessionId): %v", e)
	}
	ir, _, ie := procImpersonateLoggedOnUser.Call(dupTok)
	if ir == 0 {
		_, _, _ = procCloseHandleSAS.Call(dupTok)
		return nil, fmt.Errorf("ImpersonateLoggedOnUser(system@%d): %v", session, ie)
	}
	return func() {
		_, _, _ = procRevertToSelf.Call()
		_, _, _ = procCloseHandleSAS.Call(dupTok)
	}, nil
}

// spawnSendSASHelper launches "<exe> --send-sas" inside the target session.
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
