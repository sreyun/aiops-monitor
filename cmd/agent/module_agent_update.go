package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// moduleAgentUpdate downloads the matching platform binary from the server /dl/
// tree, verifies SHA-256, stages it beside the running executable, then asks the
// OS service manager to restart so the new binary is picked up.
//
// Args:
//
//	server   – base URL (required), e.g. http://mon.example:8529
//	force    – "1" to replace even when versions match
//	version  – expected target version (optional; skip when equal unless force)
//	rollback – "1" to restore aiops-agent(.exe).bak instead of downloading
func moduleAgentUpdate(args map[string]string, allowedBases []string) ([]byte, int) {
	if truthyArg(args["rollback"]) {
		return moduleAgentRollback()
	}
	server := strings.TrimRight(strings.TrimSpace(args["server"]), "/")
	if server == "" && len(allowedBases) > 0 {
		server = allowedBases[0]
	}
	if server == "" {
		return []byte("agent_update: missing server URL"), 1
	}
	if err := validateUpdateServerURL(server, allowedBases); err != nil {
		return []byte("agent_update: " + err.Error()), 1
	}
	force := truthyArg(args["force"])
	wantVer := strings.TrimSpace(args["version"])
	if !force && wantVer != "" && normalizeAgentVer(wantVer) == normalizeAgentVer(agentVersion()) {
		return []byte(fmt.Sprintf("agent_update: already at %s (skip)", agentVersion())), 0
	}

	binName, err := agentDistBinaryName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return []byte("agent_update: " + err.Error()), 1
	}
	exe, err := os.Executable()
	if err != nil {
		return []byte("agent_update: resolve executable: " + err.Error()), 1
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)
	staging := filepath.Join(dir, ".aiops-agent.new"+exeSuffix())
	bak := exe + ".bak"

	dlURL := server + "/dl/" + binName
	sumURL := dlURL + ".sha256"
	client := newUpdateHTTPClient(10 * time.Minute)
	if err := downloadFileHTTP(client, dlURL, staging); err != nil {
		_ = os.Remove(staging)
		return []byte("agent_update: download failed: " + err.Error()), 1
	}
	expected, err := fetchRemoteSHA256(client, sumURL)
	if err != nil {
		_ = os.Remove(staging)
		return []byte("agent_update: checksum fetch failed: " + err.Error()), 1
	}
	actual, err := fileSHA256Hex(staging)
	if err != nil {
		_ = os.Remove(staging)
		return []byte("agent_update: checksum compute failed: " + err.Error()), 1
	}
	if !strings.EqualFold(expected, actual) {
		_ = os.Remove(staging)
		return []byte(fmt.Sprintf("agent_update: SHA-256 mismatch (want %s got %s); refusing replace", expected, actual)), 1
	}
	_ = os.Chmod(staging, 0o755)

	// Backup current binary. On Windows the running PE is often locked for
	// reading — leave .bak to the restart helper and do not delete an existing
	// good backup before a successful copy.
	if runtime.GOOS != "windows" {
		tmpBak := bak + ".tmp"
		if err := copyFile(exe, tmpBak); err != nil {
			fmt.Fprintf(os.Stderr, "agent_update: backup warning: %v\n", err)
			_ = os.Remove(tmpBak)
		} else {
			_ = os.Remove(bak)
			if err := os.Rename(tmpBak, bak); err != nil {
				_ = copyFile(tmpBak, bak)
				_ = os.Remove(tmpBak)
			}
		}
	}

	cfgPath := resolveAgentConfigBesideExe(dir)
	if err := agentReplaceAndRestart(exe, staging, cfgPath); err != nil {
		return []byte("agent_update: " + err.Error()), 1
	}
	msg := fmt.Sprintf("agent_update: staged %s sha256=%s from=%s → restart scheduled (was %s",
		binName, actual[:12], server, agentVersion())
	if wantVer != "" {
		msg += fmt.Sprintf(", target %s", wantVer)
	}
	msg += ")"
	return []byte(msg), 0
}

func moduleAgentRollback() ([]byte, int) {
	exe, err := os.Executable()
	if err != nil {
		return []byte("agent_update rollback: " + err.Error()), 1
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	bak := exe + ".bak"
	if st, err := os.Stat(bak); err != nil || st.IsDir() {
		return []byte("agent_update rollback: no .bak beside current binary"), 1
	}
	staging := filepath.Join(filepath.Dir(exe), ".aiops-agent.rollback"+exeSuffix())
	if err := copyFile(bak, staging); err != nil {
		return []byte("agent_update rollback: " + err.Error()), 1
	}
	_ = os.Chmod(staging, 0o755)
	cfgPath := resolveAgentConfigBesideExe(filepath.Dir(exe))
	if err := agentReplaceAndRestart(exe, staging, cfgPath); err != nil {
		_ = os.Remove(staging)
		return []byte("agent_update rollback: " + err.Error()), 1
	}
	return []byte("agent_update: rollback to .bak scheduled"), 0
}

// resolveAgentConfigBesideExe returns an absolute config path next to the agent
// binary when present. Update restart helpers must pass this explicitly —
// Windows services start with CWD=System32, and a bare relaunch without
// --config falls back to localhost and breaks terminal/desktop.
func resolveAgentConfigBesideExe(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	for _, name := range []string{"config.yaml", "config.yml", "config.json"} {
		p := filepath.Join(dir, name)
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			continue
		}
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
		return p
	}
	return ""
}

func truthyArg(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func normalizeAgentVer(v string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "v")
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// agentDistBinaryName maps GOOS/GOARCH to the /dl/ artifact name served by the server.
func agentDistBinaryName(goos, goarch string) (string, error) {
	goos = strings.ToLower(strings.TrimSpace(goos))
	goarch = strings.ToLower(strings.TrimSpace(goarch))
	switch goarch {
	case "x86_64", "x64":
		goarch = "amd64"
	case "aarch64":
		goarch = "arm64"
	}
	switch goos {
	case "linux":
		switch goarch {
		case "amd64":
			return "aiops-agent-linux-amd64", nil
		case "arm64":
			return "aiops-agent-linux-arm64", nil
		}
	case "darwin":
		switch goarch {
		case "amd64":
			return "aiops-agent-darwin-amd64", nil
		case "arm64":
			return "aiops-agent-darwin-arm64", nil
		}
	case "windows":
		switch goarch {
		case "amd64":
			return "aiops-agent.exe", nil
		case "arm64":
			return "aiops-agent-windows-arm64.exe", nil
		}
	}
	return "", fmt.Errorf("unsupported platform %s/%s", goos, goarch)
}

func validateUpdateServerURL(server string, allowedBases []string) error {
	u, err := url.Parse(server)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid server URL")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("server URL scheme must be http or https")
	}
	if len(allowedBases) == 0 {
		// Standalone module tests / unexpected path: still block obvious SSRF targets.
		if host, _, err := net.SplitHostPort(u.Host); err == nil {
			if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
				// loopback is OK for local ops; link-local is not a typical monitor server
				if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
					return fmt.Errorf("server URL host not allowed")
				}
			}
		}
		return nil
	}
	want := strings.ToLower(strings.TrimRight(server, "/"))
	for _, b := range allowedBases {
		base := strings.ToLower(strings.TrimRight(strings.TrimSpace(b), "/"))
		if base == "" {
			continue
		}
		if want == base {
			return nil
		}
		// Allow same host with/without trailing path differences.
		bu, err := url.Parse(base)
		if err != nil {
			continue
		}
		if strings.EqualFold(u.Scheme, bu.Scheme) && strings.EqualFold(u.Host, bu.Host) {
			return nil
		}
	}
	return fmt.Errorf("server URL not in configured report targets")
}

func newUpdateHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: reportTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if len(via) == 0 {
				return nil
			}
			orig := via[0].URL
			if !strings.EqualFold(req.URL.Scheme, orig.Scheme) || !strings.EqualFold(req.URL.Host, orig.Host) {
				return fmt.Errorf("redirect to different host blocked")
			}
			return nil
		},
	}
}

func downloadFileHTTP(client *http.Client, rawURL, dest string) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}
	_ = os.Remove(dest)
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return f.Sync()
}

func fetchRemoteSHA256(client *http.Client, rawURL string) (string, error) {
	resp, err := client.Get(rawURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return "", fmt.Errorf("empty checksum body")
	}
	sum := strings.ToLower(fields[0])
	if len(sum) != 64 {
		return "", fmt.Errorf("invalid sha256 length")
	}
	for _, c := range sum {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", fmt.Errorf("invalid sha256 hex")
		}
	}
	return sum, nil
}

func fileSHA256Hex(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	_ = os.Remove(dst)
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
