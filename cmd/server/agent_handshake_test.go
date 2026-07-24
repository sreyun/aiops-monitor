package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aiops-monitor/shared"
)

// TestLogEncryptHandshakeEndToEnd 验证日志加密全链路：注册下发 log_key → 用该密钥加密一批
// 日志 → 服务端 handleAgentLogs 按指纹重派生密钥解密解压并入库；且请求体不含明文。
func TestLogEncryptHandshakeEndToEnd(t *testing.T) {
	t.Setenv("AIOPS_SECRET_KEY", "test-master-key-e2e") // 隔离 + 自动还原

	srv, token := newTestServer(t)
	const hostID, fp = "h-log", "fp-log-0001"

	rr := postJSON(t, srv.handleRegister, "/api/v1/agent/register", map[string]string{
		"host_id": hostID, "hostname": "n", "token": token, "fingerprint": fp,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("注册失败: %d %s", rr.Code, rr.Body)
	}
	var reg struct {
		LogKey     string `json:"log_key"`
		LogEncrypt bool   `json:"log_encrypt"`
	}
	if json.Unmarshal(rr.Body.Bytes(), &reg) != nil || !reg.LogEncrypt || reg.LogKey == "" {
		t.Fatalf("注册响应应下发 log_key + log_encrypt: %s", rr.Body)
	}
	key, err := base64.StdEncoding.DecodeString(reg.LogKey)
	if err != nil || len(key) != 32 {
		t.Fatalf("log_key 非法: len=%d err=%v", len(key), err)
	}

	// 用下发密钥加密一批日志（sealLog 与 agent 的 sealLogAgent 同算法）→ 上报
	batch := shared.LogBatch{HostID: hostID, Lines: []shared.LogLine{{Ts: time.Now().Unix(), Source: "/var/log/x", Level: "error", Message: "secret-boom-42"}}}
	plain, _ := json.Marshal(batch)
	sealed, err := sealLog(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("secret-boom-42")) {
		t.Fatal("密文泄露明文")
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/logs", bytes.NewReader(sealed))
	req.Header.Set("X-Log-Enc", "aesgcm-gzip")
	req.Header.Set("X-Agent-Fingerprint", fp)
	rr2 := httptest.NewRecorder()
	srv.handleAgentLogs(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("加密日志入库失败: %d %s", rr2.Code, rr2.Body)
	}
}

// newTestServer builds a real Server backed by an in-memory Store and a throwaway
// ConfigStore (no PostgreSQL needed — persistence is orthogonal to the agent
// handshake). It exercises the actual handleRegister / handleReport handlers.
func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	store := NewStore()
	cfg := newTestConfigStore(t)
	notifier := NewNotifier(store, cfg)
	srv := NewServer(store, cfg, notifier, t.TempDir(), "127.0.0.1:0")
	return srv, cfg.InstallToken()
}

func postJSON(t *testing.T, h http.HandlerFunc, path string, v any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(v)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

// TestAgentHandshakeEndToEnd walks the full agent↔server admission handshake
// through the real HTTP handlers: token-gated registration, fingerprint-gated
// reporting, rejection of spoofed fingerprints, and token-less re-registration
// of a known host (server-restart recovery).
func TestAgentHandshakeEndToEnd(t *testing.T) {
	srv, token := newTestServer(t)
	const hostID = "host-abc"
	const fp = "fp-legit-0001"

	// 1. New agent registers with a VALID token + fingerprint → 200.
	rr := postJSON(t, srv.handleRegister, "/api/v1/agent/register", map[string]string{
		"host_id": hostID, "hostname": "node-1", "token": token, "fingerprint": fp,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("register with valid token: got %d, want 200 (body: %s)", rr.Code, rr.Body)
	}

	// 2. Authenticated report with the bound fingerprint → 200.
	rep := shared.Report{HostID: hostID, Hostname: "node-1", Fingerprint: fp}
	rr = postJSON(t, srv.handleReport, "/api/v1/agent/report", rep)
	if rr.Code != http.StatusOK {
		t.Fatalf("report with matching fingerprint: got %d, want 200 (body: %s)", rr.Code, rr.Body)
	}

	// 3. Spoofed report: correct host_id but WRONG fingerprint → 403 (this is the
	//    core anti-spoofing guarantee — the fingerprint is the report credential).
	spoof := shared.Report{HostID: hostID, Hostname: "node-1", Fingerprint: "fp-attacker"}
	rr = postJSON(t, srv.handleReport, "/api/v1/agent/report", spoof)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("report with spoofed fingerprint: got %d, want 403", rr.Code)
	}

	// 4. New/unknown host WITHOUT a valid token → 403 (admission is token-gated).
	rr = postJSON(t, srv.handleRegister, "/api/v1/agent/register", map[string]string{
		"host_id": "host-evil", "hostname": "evil", "token": "wrong-token", "fingerprint": "fp-x",
	})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("register unknown host with bad token: got %d, want 403", rr.Code)
	}

	// 5. KNOWN host re-registers WITHOUT a token but with its MATCHING fingerprint
	//    → 200. This is the server-restart / rotated-token recovery path.
	rr = postJSON(t, srv.handleRegister, "/api/v1/agent/register", map[string]string{
		"host_id": hostID, "hostname": "node-1", "token": "", "fingerprint": fp,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("known host token-less re-register: got %d, want 200 (body: %s)", rr.Code, rr.Body)
	}

	// 6. But a known host_id with a DIFFERENT fingerprint and no token → 403
	//    (an attacker who learned the host_id but not the fingerprint can't hijack).
	rr = postJSON(t, srv.handleRegister, "/api/v1/agent/register", map[string]string{
		"host_id": hostID, "hostname": "node-1", "token": "", "fingerprint": "fp-attacker",
	})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("known host wrong-fingerprint token-less re-register: got %d, want 403", rr.Code)
	}
}

// TestInstallScriptsRobustness renders the install/uninstall templates and
// asserts the cross-platform autostart + keepalive + clean-uninstall guarantees
// are present. When AIOPS_RENDER_DIR is set the rendered scripts are also dumped
// there for external shell/PowerShell syntax checking.
func TestInstallScriptsRobustness(t *testing.T) {
	server, token := "https://mon.example.com", "tok-123"
	shIn := renderScript(installShTemplate, server, token, "prod", "", "")
	ps1In := renderScript(installPs1Template, server, token, "prod", "", "")
	shUn := renderScript(uninstallShTemplate, server, token, "prod", "", "")
	ps1Un := renderScript(uninstallPs1Template, server, token, "prod", "", "")

	must := func(name, hay string, needles ...string) {
		for _, n := range needles {
			if !strings.Contains(hay, n) {
				t.Errorf("%s: missing %q", name, n)
			}
		}
	}
	// macOS now gets a real launchd job (autostart on boot + keepalive), and Linux
	// root keeps systemd auto-restart.
	must("install.sh", shIn,
		`elif [ "$OS" = "Darwin" ]`, "com.aiops.agent.plist",
		"<key>RunAtLoad</key><true/>", "<key>KeepAlive</key><true/>",
		"launchctl load", "Restart=always", "@reboot")
	// YAML is the default config format now: the script must write config.yaml,
	// point the service at it, and migrate away any stale config.json.
	must("install.sh (yaml)", shIn,
		"cat > config.yaml", "--config $DIR/config.yaml",
		"rm -f config.json")
	// Windows: supervisor VBS (no duplicates) + logon autostart + 5-min keepalive task.
	must("install.ps1", ps1In,
		"start-agent.vbs", "Win32_Process",
		`schtasks /Create /TN "AIOpsAgent"`, "/SC MINUTE /MO 5",
		`CurrentVersion\Run`)
	// PowerShell 5.1 defaults to a legacy console code page. The installer must
	// switch both console and native pipeline encodings to UTF-8, and must not
	// pipe Agent stderr through ForEach-Object (which caused Chinese mojibake).
	must("install.ps1 (utf8)", ps1In,
		`[Console]::InputEncoding = $Utf8NoBom`,
		`[Console]::OutputEncoding = $Utf8NoBom`,
		`$global:OutputEncoding = $Utf8NoBom`,
		`chcp.com 65001`)
	if strings.Contains(ps1In, `2>&1 | ForEach-Object`) {
		t.Error("install.ps1 (utf8): Agent output must not pass through the PowerShell 5.1 native-output decoding pipeline")
	}
	// Windows must also write config.yaml (hand-built, no JSON serializer) and
	// remove a stale config.json from a pre-YAML install.
	must("install.ps1 (yaml)", ps1In,
		`WriteAllText("$Dir\config.yaml"`, `$conf = "$Dir\config.yaml"`,
		`Remove-Item "$Dir\config.json"`)
	// Hyper-V auto-elevation: a non-elevated install on a Hyper-V host must relaunch
	// itself via UAC (RunAs + EncodedCommand) so Get-VM (VM collection) works. Host
	// detection uses the vmms service (instant, no slow module autoload race) rather
	// than Get-Command Get-VM.
	must("install.ps1 (uac)", ps1In,
		"Get-Service -Name vmms", "-Verb RunAs", "-EncodedCommand",
		"SecurityProtocol")
	// Elevated installs must register the real Windows service (boot autostart +
	// crash-recovery + interactive desktop worker), which is what makes Hyper-V
	// collection, reboot persistence, and lock-screen remote desktop all work.
	must("install.ps1 (service)", ps1In,
		"--install-service", "AiopsMonitorAgent")
	// Downloaded binaries carry MOTW; Application Control (WDAC/AppLocker) must
	// be detected early — Session-0 keepalive fallback cannot run a blocked exe.
	must("install.ps1 (app-control)", ps1In,
		"Unblock-File", "Test-AiopsAgentRunnable",
		"Application Control", "Zone.Identifier")
	// Uninstall must tear down every autostart mechanism it created.
	must("uninstall.sh", shUn,
		"LaunchDaemons/com.aiops.agent.plist", "launchctl unload", "crontab")
	must("uninstall.ps1", ps1Un,
		`schtasks /Delete /TN "AIOpsAgent"`, "AIOpsAgent", "AIOpsRelay")

	if dir := os.Getenv("AIOPS_RENDER_DIR"); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
		for name, body := range map[string]string{
			"install.sh": shIn, "install.ps1": ps1In,
			"uninstall.sh": shUn, "uninstall.ps1": ps1Un,
		} {
			_ = os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
		}
		t.Logf("rendered scripts written to %s", dir)
	}
}
