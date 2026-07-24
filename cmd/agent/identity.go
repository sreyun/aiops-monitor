package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"runtime"
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
func machineFingerprint() string {
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
	switch runtime.GOOS {
	case "linux":
		for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
			if b, err := os.ReadFile(p); err == nil {
				if s := strings.TrimSpace(string(b)); s != "" {
					return s
				}
			}
		}
	case "windows":
		out, _ := exec.Command("reg", "query", `HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid").Output()
		for _, ln := range strings.Split(string(out), "\n") {
			if strings.Contains(ln, "MachineGuid") {
				if f := strings.Fields(ln); len(f) >= 3 {
					return f[len(f)-1]
				}
			}
		}
	case "darwin":
		out, _ := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
		for _, ln := range strings.Split(string(out), "\n") {
			if strings.Contains(ln, "IOPlatformUUID") {
				if i := strings.Index(ln, `= "`); i >= 0 {
					rest := ln[i+3:]
					if j := strings.IndexByte(rest, '"'); j >= 0 {
						return rest[:j]
					}
				}
			}
		}
	}
	return ""
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

// primaryIP returns the host's primary non-loopback IPv4 address.
func primaryIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
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
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}
	return ""
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
