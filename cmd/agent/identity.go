package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// loadOrCreateHostID returns a stable per-machine id, persisting a freshly
// generated one so the agent keeps the same identity across restarts.
//
// Anti-clone: the state file also stores a machine fingerprint (OS machine-id +
// primary MAC). If a golden image / VM template bakes in agent_state.json, every
// clone would otherwise share one host_id and fight over a single host record on
// the server (data + online status flapping between the two machines). When the
// stored fingerprint no longer matches the current machine we detect the clone
// and regenerate the id, so different machines never collide — even with the same
// hostname and IP. Old state files without a fingerprint are honored unchanged.
//
// Atomicity: writes go to a temp file first, then os.Rename (atomic on
// Linux/macOS). On Windows, Rename is not atomic for existing targets, so
// we tolerate best-effort — the state file is tiny and corruption is
// recoverable by simply regenerating the ID.
func loadOrCreateHostID(path string) string {
	fp := machineFingerprint()
	migrateLegacyStateFile(path)
	if b, err := os.ReadFile(path); err == nil {
		var s struct {
			HostID string `json:"host_id"`
			FP     string `json:"fp"`
		}
		if json.Unmarshal(b, &s) == nil && s.HostID != "" {
			// Keep the id unless we can prove the file was cloned onto a different
			// machine (both fingerprints known and different).
			if fp == "" || s.FP == "" || s.FP == fp {
				return s.HostID
			}
		}
	}
	id := randomID()
	persistHostID(path, id, fp)
	return id
}

// migrateLegacyStateFile moves an identity file that a previous version wrote
// relative to the working directory into the anchored location.
//
// Windows services run with CWD=C:\Windows\System32, so agents installed before
// paths were anchored to the install dir left their agent_state.json there. A
// straight cutover would have generated a brand new host_id and split every
// host's history in two, so adopt the old file when the new one is absent.
func migrateLegacyStateFile(path string) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err == nil {
		return // already anchored
	}
	for _, legacy := range legacyStatePaths(path) {
		b, err := os.ReadFile(legacy)
		if err != nil || len(b) == 0 {
			continue
		}
		var s struct {
			HostID string `json:"host_id"`
		}
		if json.Unmarshal(b, &s) != nil || s.HostID == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(path, b, 0o600); err != nil {
			continue
		}
		slog.Info("已迁移旧身份文件到安装目录", "from", legacy, "to", path, "host_id", s.HostID)
		// Leave nothing behind that a downgraded binary could pick up and start
		// reporting under in parallel with the migrated identity. Deleting inside
		// System32 needs SYSTEM/admin, which the service has and a manual run may
		// not — the copy is already made either way, so only warn.
		if err := os.Remove(legacy); err != nil && !os.IsNotExist(err) {
			slog.Warn("旧身份文件删除失败（不影响运行，建议以管理员身份清理）", "path", legacy, "err", err)
		}
		return
	}
}

// legacyStatePaths lists working-directory-relative locations where older agents
// may have written the identity file.
func legacyStatePaths(path string) []string {
	base := filepath.Base(path)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	dirs := []string{cwd}
	if root := os.Getenv("SystemRoot"); root != "" {
		dirs = append(dirs, filepath.Join(root, "System32"))
	}
	var out []string
	for _, dir := range dirs {
		if dir == "" || dir == string(filepath.Separator) {
			continue
		}
		cand := filepath.Join(dir, base)
		if cand == path {
			continue
		}
		out = append(out, cand)
	}
	return out
}

// readHostIDFromState returns the host_id stored in the state file, or "" when
// the file is missing/unreadable/empty. Unlike loadOrCreateHostID it never
// generates a new id — used by the desktop worker to pick up an id the service
// may have just reconciled, without racing a fresh id into existence.
func readHostIDFromState(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var s struct {
		HostID string `json:"host_id"`
	}
	if json.Unmarshal(b, &s) != nil {
		return ""
	}
	return s.HostID
}

// persistHostID atomically writes the identity state file.
//
// Atomic write: temp file + rename to prevent partial writes on crash.
// On Windows, Rename is not atomic for existing targets, so we fall back to a
// direct write — the file is tiny and a corrupted one just regenerates the id.
func persistHostID(path, id, fp string) {
	b, err := json.Marshal(map[string]string{"host_id": id, "fp": fp})
	if err != nil {
		slog.Error("身份文件序列化失败", "path", path, "err", err)
		return
	}
	// Anchored paths can point at a directory the installer has not created yet
	// (or that an uninstall removed); without this the write fails and the agent
	// silently mints a new host_id on every single start.
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if e := os.MkdirAll(dir, 0o755); e != nil {
			slog.Error("身份文件目录创建失败", "dir", dir, "err", e)
		}
	}
	tmp := path + ".tmp"
	if e := os.WriteFile(tmp, b, 0o600); e == nil {
		if e2 := os.Rename(tmp, path); e2 == nil {
			return
		}
	}
	_ = os.WriteFile(path, b, 0o600) // fallback（host-id 身份文件，仅属主可读）
}

// machineFingerprint returns a stable, machine-unique fingerprint derived from
// the OS machine id and the primary MAC address, hashed. Returns "" when nothing
// machine-unique can be read (then clone detection is skipped, never a false
// positive). Zero third-party dependency.
//
// Containers: set AIOPS_MACHINE_ID to a stable string (compose/K8s) so the
// fingerprint does not change when the container MAC is reassigned on recreate.
func machineFingerprint() string {
	if v := strings.TrimSpace(os.Getenv("AIOPS_MACHINE_ID")); v != "" {
		sum := sha256.Sum256([]byte("env:" + v))
		return hex.EncodeToString(sum[:12])
	}
	parts := []string{machineID(), primaryMAC()}
	joined := strings.TrimSpace(strings.Trim(strings.Join(parts, "|"), "|"))
	if joined == "" || joined == "|" {
		return ""
	}
	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:12])
}

// machineID reads the OS-provided stable machine identifier.
func machineID() string {
	if v := strings.TrimSpace(os.Getenv("AIOPS_MACHINE_ID")); v != "" {
		return v
	}
	return machineIDFromOS()
}

// primaryMAC returns the hardware address of the first up, non-loopback
// interface — differs across machines (and most VM clones) even when hostname/IP
// coincide.
func primaryMAC() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		if mac := ifc.HardwareAddr.String(); mac != "" {
			return mac
		}
	}
	return ""
}

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "host-unknown"
	}
	return hex.EncodeToString(b)
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown-host"
	}
	return h
}

// primaryIP returns the host's most useful non-loopback IPv4 address.
// Prefer the kernel default-route source (UDP dial), then score remaining
// NIC addresses so APIPA/link-local 169.254/16 and virtual adapters lose to
// real LAN/WAN addresses (common on Windows Server + Hyper-V hosts).
func primaryIP() string {
	if ip := outboundIPv4(); ip != "" {
		return ip
	}
	ips := rankedLocalIPv4s()
	if len(ips) == 0 {
		return ""
	}
	return ips[0]
}

// outboundIPv4 returns the local IPv4 the OS would use for Internet egress.
// DialUDP does not send packets; it only consults the routing table.
func outboundIPv4() string {
	// Several destinations: some air-gapped hosts only have a private default route.
	for _, dst := range []string{"1.1.1.1:53", "8.8.8.8:53", "192.168.0.1:53"} {
		c, err := net.Dial("udp", dst)
		if err != nil {
			continue
		}
		la, ok := c.LocalAddr().(*net.UDPAddr)
		_ = c.Close()
		if !ok || la == nil {
			continue
		}
		ip4 := la.IP.To4()
		if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() || ip4.IsUnspecified() {
			continue
		}
		return ip4.String()
	}
	return ""
}

type rankedIPv4 struct {
	ip   string
	rank int
}

// rankedLocalIPv4s lists UP non-loopback IPv4s, best-first.
// Link-local (169.254/16) is kept at the end for completeness, never preferred.
func rankedLocalIPv4s() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var ranked []rankedIPv4
	seen := map[string]bool{}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() {
				continue
			}
			ip4 := ip.To4()
			if ip4 == nil {
				continue
			}
			s := ip4.String()
			if seen[s] {
				continue
			}
			seen[s] = true
			ranked = append(ranked, rankedIPv4{ip: s, rank: scoreIPv4(ip4, ifc.Name)})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].rank != ranked[j].rank {
			return ranked[i].rank > ranked[j].rank
		}
		return ranked[i].ip < ranked[j].ip
	})
	out := make([]string, len(ranked))
	for i := range ranked {
		out[i] = ranked[i].ip
	}
	return out
}

// scoreIPv4 ranks a candidate for "display / primary" use. Higher is better.
func scoreIPv4(ip net.IP, ifName string) int {
	ip4 := ip.To4()
	if ip4 == nil {
		return -10000
	}
	score := 100
	// APIPA / link-local — almost never the operational address.
	if ip4.IsLinkLocalUnicast() || (ip4[0] == 169 && ip4[1] == 254) {
		score -= 1000
	}
	switch {
	case ip4[0] == 10:
		score += 120 // RFC1918
	case ip4[0] == 192 && ip4[1] == 168:
		score += 120
	case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
		score += 110
	case ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127:
		score += 40 // CGNAT / some VPN overlays — usable but secondary
	case !ip4.IsPrivate() && !ip4.IsLinkLocalUnicast():
		score += 90 // public / other globally routable
	}
	n := strings.ToLower(ifName)
	switch {
	case strings.Contains(n, "docker"), strings.Contains(n, "br-"),
		strings.Contains(n, "veth"), strings.Contains(n, "virbr"),
		strings.Contains(n, "cni"), strings.Contains(n, "flannel"),
		strings.Contains(n, "calico"), strings.Contains(n, "kube"):
		score -= 250
	case strings.Contains(n, "vethernet"), strings.Contains(n, "hyper-v"),
		strings.Contains(n, "virtualbox"), strings.Contains(n, "vmware"),
		strings.Contains(n, "vbox"), strings.Contains(n, "virian"),
		strings.HasPrefix(n, "veth"), strings.Contains(n, "tap"),
		strings.Contains(n, "tun"), strings.Contains(n, "isatap"),
		strings.Contains(n, "teredo"), strings.Contains(n, "bluetooth"):
		score -= 200
	case strings.Contains(n, "ethernet"), strings.HasPrefix(n, "eth"),
		strings.HasPrefix(n, "en"), strings.HasPrefix(n, "em"),
		strings.HasPrefix(n, "bond"), strings.HasPrefix(n, "team"),
		strings.HasPrefix(n, "lan"), strings.Contains(n, "local area"):
		score += 40
	case strings.Contains(n, "wi-fi"), strings.Contains(n, "wifi"),
		strings.HasPrefix(n, "wl"), strings.Contains(n, "wlan"):
		score += 20
	}
	return score
}

// linuxPrettyName reads PRETTY_NAME from /etc/os-release (used by the Linux
// collector's osVersion; harmless no-op elsewhere).
func linuxPrettyName() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
		}
	}
	return ""
}
