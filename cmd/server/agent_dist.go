package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// agentDistArtifact describes one downloadable agent binary under dist/.
type agentDistArtifact struct {
	Name    string `json:"name"`
	GOOS    string `json:"goos"`
	GOARCH  string `json:"goarch"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
	Version string `json:"version"`
	Path    string `json:"-"`
}

var agentDistCatalog = []struct {
	Name   string
	GOOS   string
	GOARCH string
}{
	{"aiops-agent-linux-amd64", "linux", "amd64"},
	{"aiops-agent-linux-arm64", "linux", "arm64"},
	{"aiops-agent-darwin-amd64", "darwin", "amd64"},
	{"aiops-agent-darwin-arm64", "darwin", "arm64"},
	{"aiops-agent.exe", "windows", "amd64"},
	{"aiops-agent-windows-arm64.exe", "windows", "arm64"},
}

func agentDistBinaryName(goos, goarch string) (string, error) {
	goos = strings.ToLower(strings.TrimSpace(goos))
	goarch = strings.ToLower(strings.TrimSpace(goarch))
	// Normalize common agent/report arch aliases.
	switch goarch {
	case "x86_64", "x64":
		goarch = "amd64"
	case "aarch64":
		goarch = "arm64"
	}
	for _, a := range agentDistCatalog {
		if a.GOOS == goos && a.GOARCH == goarch {
			return a.Name, nil
		}
	}
	return "", fmt.Errorf("unsupported platform %s/%s", goos, goarch)
}

func (s *Server) listAgentDistManifest() []agentDistArtifact {
	out := make([]agentDistArtifact, 0, len(agentDistCatalog))
	if s.distDir == "" {
		return out
	}
	ver := strings.TrimSpace(appVersion)
	for _, a := range agentDistCatalog {
		p := filepath.Join(s.distDir, a.Name)
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			continue
		}
		sum, err := fileSHA256HexServer(p)
		if err != nil {
			continue
		}
		out = append(out, agentDistArtifact{
			Name: a.Name, GOOS: a.GOOS, GOARCH: a.GOARCH,
			Size: fi.Size(), SHA256: sum, Version: ver, Path: p,
		})
	}
	return out
}

func (s *Server) agentDistHas(goos, goarch string) bool {
	name, err := agentDistBinaryName(goos, goarch)
	if err != nil || s.distDir == "" {
		return false
	}
	_, err = os.Stat(filepath.Join(s.distDir, name))
	return err == nil
}

func fileSHA256HexServer(path string) (string, error) {
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

func normalizeAgentVer(v string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "v")
}

// isComparableAgentVer reports whether v looks like a release version we can order.
func isComparableAgentVer(v string) bool {
	v = normalizeAgentVer(v)
	if v == "" || v == "aiops" || v == "dev" {
		return false
	}
	return unicode.IsDigit(rune(v[0]))
}

// compareAgentVer returns -1 if a<b, 0 if equal, 1 if a>b (numeric dotted parts).
func compareAgentVer(a, b string) int {
	a, b = normalizeAgentVer(a), normalizeAgentVer(b)
	ap := strings.Split(a, ".")
	bp := strings.Split(b, ".")
	n := len(ap)
	if len(bp) > n {
		n = len(bp)
	}
	for i := 0; i < n; i++ {
		ai, bi := 0, 0
		if i < len(ap) {
			ai = verNumericPrefix(ap[i])
		}
		if i < len(bp) {
			bi = verNumericPrefix(bp[i])
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

func verNumericPrefix(s string) int {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0
	}
	n, _ := strconv.Atoi(s[:i])
	return n
}

// agentVersionBehind is true only when current is strictly older than target.
// Empty current (legacy agent) counts as behind. Uncomparable targets (AIOps/dev)
// never trigger updates. Newer agents are never treated as behind (no downgrade).
func agentVersionBehind(current, target string) bool {
	target = normalizeAgentVer(target)
	if !isComparableAgentVer(target) {
		return false
	}
	current = normalizeAgentVer(current)
	if current == "" {
		return true
	}
	if !isComparableAgentVer(current) {
		// Agent still reports "dev"/placeholder — push once to a real release.
		return true
	}
	return compareAgentVer(current, target) < 0
}

func hostSupportsAgentUpdateModule(h *Host) bool {
	// Agents that report agent_version were built with the self-update module.
	return h != nil && strings.TrimSpace(h.AgentVersion) != ""
}

// agentPublicBaseURL is the URL agents use to download /dl/. Prefer PublicURL;
// fall back to empty (caller must supply from HTTP request).
func (s *Server) agentPublicBaseURL() string {
	if s == nil || s.cfg == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(s.cfg.PublicURL()), "/")
}

func hostGOOSArch(h *Host) (goos, goarch string) {
	goos = strings.ToLower(strings.TrimSpace(h.OS))
	if goos == "macos" || goos == "osx" || goos == "mac" {
		goos = "darwin"
	}
	goarch = strings.ToLower(strings.TrimSpace(h.Arch))
	switch goarch {
	case "x86_64", "x64":
		goarch = "amd64"
	case "aarch64":
		goarch = "arm64"
	}
	return goos, goarch
}
