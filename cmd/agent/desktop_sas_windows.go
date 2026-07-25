//go:build windows

package main

import (
	"fmt"
	"log/slog"
	"syscall"
	"time"
	"unsafe"
)

// Secure Attention Sequence (Ctrl+Alt+Del) injection.
//
// SendSAS is VOID and only reliable from a real SCM service (or uiAccess apps).
// Our desktop worker is CreateProcessAsUser(LocalSystem) inside the console
// session — NOT a service — so direct SendSAS often silently no-ops.
// Chromium remoting / UltraVNC solve this by:
//  1) enabling SoftwareSASGeneration policy
//  2) calling SendSAS from the Session-0 service after ImpersonateNamedPipeClient
//     so the SAS targets the worker's interactive session.

const (
	deskSASPipeName = `\\.\pipe\aiops-monitor-sas-v1`

	regOptionNonVolatile = 0
	keySetValue          = 0x0002
	keyQueryValue        = 0x0001
	keyCreateSubKey      = 0x0004
	regDWORD             = 4
	errorPipeBusy       = 231
	genericRead         = 0x80000000
	genericWrite        = 0x40000000
	openExisting        = 3
	fileAttributeNormal = 0x80
	pipeAccessDuplex    = 0x00000003
	pipeTypeByte        = 0x00000000
	pipeReadModeByte    = 0x00000000
	pipeWait            = 0x00000000
	pipeRejectRemote    = 0x00000008
	pipeUnlimitedInst   = 255
)

var (
	modAdvapi32SAS = syscall.NewLazyDLL("advapi32.dll")
	modKernel32SAS = syscall.NewLazyDLL("kernel32.dll")

	procRegCreateKeyExW = modAdvapi32SAS.NewProc("RegCreateKeyExW")
	procRegSetValueExW  = modAdvapi32SAS.NewProc("RegSetValueExW")
	procRegQueryValueExW = modAdvapi32SAS.NewProc("RegQueryValueExW")
	procRegCloseKey     = modAdvapi32SAS.NewProc("RegCloseKey")

	procCreateNamedPipeW             = modKernel32SAS.NewProc("CreateNamedPipeW")
	procConnectNamedPipe             = modKernel32SAS.NewProc("ConnectNamedPipe")
	procDisconnectNamedPipe          = modKernel32SAS.NewProc("DisconnectNamedPipe")
	procCreateFileW                  = modKernel32SAS.NewProc("CreateFileW")
	procWriteFileSAS                 = modKernel32SAS.NewProc("WriteFile")
	procReadFileSAS                  = modKernel32SAS.NewProc("ReadFile")
	procCloseHandleSAS               = modKernel32SAS.NewProc("CloseHandle")
	procWaitNamedPipeW               = modKernel32SAS.NewProc("WaitNamedPipeW")
	procImpersonateNamedPipeClient   = modAdvapi32SAS.NewProc("ImpersonateNamedPipeClient")
	procRevertToSelf                 = modAdvapi32SAS.NewProc("RevertToSelf")
	procGetNamedPipeClientSessionId  = modKernel32SAS.NewProc("GetNamedPipeClientSessionId")
)

// ensureSoftwareSASPolicy sets
// HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System\SoftwareSASGeneration
// so services (and ease-of-access apps) may call SendSAS. Without this, Winlogon
// ignores software SAS and Ctrl+Alt+Del buttons appear to do nothing.
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
	if r != 0 { // ERROR_SUCCESS == 0 for registry APIs
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
	// 0=None, 1=Services, 2=EaseOfAccess, 3=Both. Prefer 3; never downgrade.
	want := uint32(3)
	if qr == 0 && typ == regDWORD {
		switch cur {
		case 1, 3:
			want = cur // already allows services
		case 2:
			want = 3
		default:
			want = 1
		}
		if cur == want {
			return nil
		}
	}
	sr, _, se := procRegSetValueExW.Call(
		hKey, uintptr(unsafe.Pointer(name)), 0, regDWORD,
		uintptr(unsafe.Pointer(&want)), 4,
	)
	if sr != 0 {
		return fmt.Errorf("RegSetValueEx(SoftwareSASGeneration): win32=%d (%v)", sr, se)
	}
	slog.Info("已启用 SoftwareSASGeneration（允许服务模拟 Ctrl+Alt+Del）", "value", want)
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
	// SendSAS is VOID — ignore the syscall "return" value entirely.
	_, _, _ = procSendSAS.Call(flag)
}

// injectSecureAttentionSequence tries every reliable path to generate CAD.
func injectSecureAttentionSequence() error {
	if err := ensureSoftwareSASPolicy(); err != nil {
		slog.Warn("设置 SoftwareSASGeneration 失败（仍尝试 SendSAS）", "err", err)
	}
	if err := modSas.Load(); err != nil {
		return fmt.Errorf("加载 sas.dll 失败: %w", err)
	}

	// Preferred: Session-0 SCM service pipe (true service context + impersonation).
	if err := requestSASFromService(4 * time.Second); err == nil {
		slog.Info("已通过 Agent 服务注入 SAS (Ctrl+Alt+Del)", "session", currentSessionID())
		return nil
	} else {
		slog.Warn("服务管道 SAS 失败，回退本地 SendSAS", "err", err, "session", currentSessionID())
	}

	// Fallback: LocalSystem worker in the interactive session.
	callSendSAS(false)
	time.Sleep(80 * time.Millisecond)
	callSendSAS(true)
	slog.Info("已本地调用 SendSAS (Ctrl+Alt+Del)", "desktop", threadDesktopName(), "session", currentSessionID())
	return nil
}

func requestSASFromService(timeout time.Duration) error {
	namePtr, err := syscall.UTF16PtrFromString(deskSASPipeName)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for {
		h, _, e := procCreateFileW.Call(
			uintptr(unsafe.Pointer(namePtr)),
			uintptr(genericRead|genericWrite),
			0, 0, openExisting, fileAttributeNormal, 0,
		)
		if h != 0 && h != uintptr(syscall.InvalidHandle) {
			defer procCloseHandleSAS.Call(h)
			msg := []byte("SAS\n")
			var written uint32
			r, _, we := procWriteFileSAS.Call(h, uintptr(unsafe.Pointer(&msg[0])), uintptr(len(msg)),
				uintptr(unsafe.Pointer(&written)), 0)
			if r == 0 {
				return fmt.Errorf("WriteFile(SAS pipe): %v", we)
			}
			buf := make([]byte, 64)
			var n uint32
			r, _, re := procReadFileSAS.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
				uintptr(unsafe.Pointer(&n)), 0)
			if r == 0 {
				return fmt.Errorf("ReadFile(SAS pipe): %v", re)
			}
			resp := string(buf[:n])
			if len(resp) >= 2 && resp[:2] == "OK" {
				return nil
			}
			return fmt.Errorf("服务 SAS 响应: %q", resp)
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

// serveSASPipe runs in the Session-0 service. Each connection impersonates the
// desktop worker and calls SendSAS so Winlogon receives CAD in that session.
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

		// ConnectNamedPipe blocks; interrupt by closing handle on stop via race.
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
		// ERROR_PIPE_CONNECTED = 535 when client already connected.
		connected := r != 0
		if !connected {
			if errno, ok := ce.(syscall.Errno); ok && errno == 535 {
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
	buf := make([]byte, 32)
	var n uint32
	r, _, _ := procReadFileSAS.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
		uintptr(unsafe.Pointer(&n)), 0)
	if r == 0 || n < 3 {
		writePipeMsg(h, "ERR read\n")
		return
	}
	req := string(buf[:n])
	if len(req) < 3 || req[:3] != "SAS" {
		writePipeMsg(h, "ERR cmd\n")
		return
	}
	var sess uint32
	if r, _, _ := procGetNamedPipeClientSessionId.Call(h, uintptr(unsafe.Pointer(&sess))); r != 0 {
		if sess == 0 {
			writePipeMsg(h, "ERR session0\n")
			return
		}
	}

	if err := ensureSoftwareSASPolicy(); err != nil {
		slog.Warn("SAS 请求前策略设置失败", "err", err)
	}
	imp, _, ie := procImpersonateNamedPipeClient.Call(h)
	if imp == 0 {
		slog.Warn("ImpersonateNamedPipeClient 失败，仍以服务身份 SendSAS", "err", ie, "session", sess)
	}
	callSendSAS(false)
	time.Sleep(40 * time.Millisecond)
	callSendSAS(true)
	if imp != 0 {
		_, _, _ = procRevertToSelf.Call()
	}
	slog.Info("服务已注入 SAS", "client_session", sess)
	writePipeMsg(h, "OK\n")
}

func writePipeMsg(h uintptr, s string) {
	b := []byte(s)
	var written uint32
	_, _, _ = procWriteFileSAS.Call(h, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)),
		uintptr(unsafe.Pointer(&written)), 0)
}
