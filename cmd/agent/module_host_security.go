package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// hostSecurityReport is the structured payload returned by host_security_scan.
type hostSecurityReport struct {
	CollectedAt int64                  `json:"collected_at"`
	Hostname    string                 `json:"hostname"`
	OS          string                 `json:"os"`
	Arch        string                 `json:"arch"`
	Kernel      string                 `json:"kernel,omitempty"`
	Distro      string                 `json:"distro,omitempty"`
	PkgMgr      string                 `json:"pkg_mgr,omitempty"`
	Packages    []hostSecPkg           `json:"packages"`
	Listeners   []string               `json:"listeners"`
	Processes   []string               `json:"processes"`
	Hardening   []hostSecFinding       `json:"hardening"`
	FileHashes  []hostSecHash          `json:"file_hashes"`
	Malware     hostSecMalware         `json:"malware"`
	Firewall    hostSecFirewall        `json:"firewall"`
	IOC         []hostSecFinding       `json:"ioc"`
	Meta        map[string]any         `json:"meta,omitempty"`
}

type hostSecPkg struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Ecosystem string `json:"ecosystem,omitempty"`
}

type hostSecFinding struct {
	Level   string `json:"level"` // crit|high|medium|low|info
	ID      string `json:"id"`
	Title   string `json:"title"`
	Detail  string `json:"detail,omitempty"`
	Suggest string `json:"suggest,omitempty"`
}

type hostSecHash struct {
	Path string `json:"path"`
	SHA256 string `json:"sha256"`
	Mode string `json:"mode,omitempty"`
	Size int64  `json:"size,omitempty"`
}

type hostSecMalware struct {
	ClamAV    string           `json:"clamav"` // available|unavailable|error
	Version   string           `json:"version,omitempty"`
	Scanned   int              `json:"scanned"`
	Infected  []string         `json:"infected,omitempty"`
	Findings  []hostSecFinding `json:"findings,omitempty"`
}

func moduleHostSecurityScan(args map[string]string) ([]byte, int) {
	ensureCommonBinPATH()
	rep := hostSecurityReport{
		CollectedAt: time.Now().Unix(),
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		Meta:        map[string]any{},
	}
	rep.Hostname, _ = os.Hostname()
	rep.Kernel = strings.TrimSpace(cmdOut(3, "uname", "-r"))
	rep.Distro, rep.PkgMgr = detectDistroPkgMgr()

	enableClam := true
	if v := strings.ToLower(strings.TrimSpace(args["clamav"])); v == "0" || v == "false" || v == "off" {
		enableClam = false
	}

	rep.Packages = collectHostPackages(rep.PkgMgr, rep.Distro, 800)
	rep.Listeners = collectListenLines(120)
	rep.Processes = collectProcessLines(40)
	rep.Firewall = collectFirewallStatus()
	rep.Hardening = collectHardeningFindings()
	rep.Hardening = append(rep.Hardening, firewallFindings(rep.Firewall)...)
	rep.FileHashes = collectSampleHashes(30)
	rep.IOC = collectIOCFindings(rep.Processes, rep.Listeners, rep.FileHashes)
	rep.Malware = runClamAVScan(enableClam, samplePathsForClam())

	raw, err := json.Marshal(rep)
	if err != nil {
		return []byte(`{"error":"marshal failed"}`), 1
	}
	// Cap output size (~1.5 MiB)
	if len(raw) > 1500<<10 {
		rep.Packages = rep.Packages[:min(200, len(rep.Packages))]
		rep.Meta["truncated"] = true
		raw, _ = json.Marshal(rep)
	}
	return raw, 0
}

func detectDistroPkgMgr() (distro, mgr string) {
	if runtime.GOOS == "darwin" {
		if have("brew") {
			return "macos", "brew"
		}
		return "macos", ""
	}
	if runtime.GOOS == "windows" {
		if have("winget") {
			return "windows", "winget"
		}
		return "windows", "wmic"
	}
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, ln := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(ln, "ID=") {
				distro = strings.Trim(strings.TrimPrefix(ln, "ID="), `"`)
			}
		}
	}
	switch {
	case have("dpkg"):
		mgr = "dpkg"
		if distro == "" {
			distro = "debian"
		}
	case have("rpm"):
		mgr = "rpm"
		if distro == "" {
			distro = "rhel"
		}
	case have("apk"):
		mgr = "apk"
		if distro == "" {
			distro = "alpine"
		}
	}
	return distro, mgr
}

func collectHostPackages(mgr, distro string, limit int) []hostSecPkg {
	out := []hostSecPkg{}
	eco := ecosystemFor(mgr, distro)
	switch mgr {
	case "dpkg":
		raw := cmdOut(20, "dpkg-query", "-W", "-f=${Package}\t${Version}\n")
		for _, ln := range strings.Split(raw, "\n") {
			f := strings.Split(ln, "\t")
			if len(f) < 2 || f[0] == "" {
				continue
			}
			out = append(out, hostSecPkg{Name: f[0], Version: f[1], Ecosystem: eco})
			if len(out) >= limit {
				break
			}
		}
	case "rpm":
		raw := cmdOut(20, "rpm", "-qa", "--qf", "%{NAME}\t%{VERSION}-%{RELEASE}\n")
		for _, ln := range strings.Split(raw, "\n") {
			f := strings.Split(ln, "\t")
			if len(f) < 2 || f[0] == "" {
				continue
			}
			out = append(out, hostSecPkg{Name: f[0], Version: f[1], Ecosystem: eco})
			if len(out) >= limit {
				break
			}
		}
	case "apk":
		raw := cmdOut(15, "apk", "info", "-v")
		for _, ln := range strings.Split(raw, "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			// name-version-rX
			i := strings.LastIndex(ln, "-")
			if i <= 0 {
				continue
			}
			j := strings.LastIndex(ln[:i], "-")
			if j <= 0 {
				continue
			}
			out = append(out, hostSecPkg{Name: ln[:j], Version: ln[j+1:], Ecosystem: "Alpine"})
			if len(out) >= limit {
				break
			}
		}
	case "brew":
		raw := cmdOut(25, "brew", "list", "--versions")
		for _, ln := range strings.Split(raw, "\n") {
			f := strings.Fields(ln)
			if len(f) < 2 {
				continue
			}
			out = append(out, hostSecPkg{Name: f[0], Version: f[1], Ecosystem: "Homebrew"})
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func ecosystemFor(mgr, distro string) string {
	d := strings.ToLower(distro)
	switch mgr {
	case "dpkg":
		if strings.Contains(d, "ubuntu") {
			return "Ubuntu"
		}
		return "Debian"
	case "apk":
		return "Alpine"
	case "rpm":
		if strings.Contains(d, "fedora") {
			return "Fedora"
		}
		if strings.Contains(d, "rocky") {
			return "Rocky Linux"
		}
		return "Red Hat"
	case "brew":
		return "Homebrew"
	default:
		return ""
	}
}

func collectListenLines(limit int) []string {
	raw, _ := moduleNetListen()
	out := []string{}
	for _, ln := range strings.Split(string(raw), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") || strings.HasPrefix(ln, "State") || strings.HasPrefix(ln, "Proto") {
			continue
		}
		out = append(out, ln)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func collectProcessLines(limit int) []string {
	raw, _ := moduleProcessTop()
	out := []string{}
	for i, ln := range strings.Split(string(raw), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || i == 0 && (strings.Contains(ln, "PID") || strings.Contains(ln, "Image Name")) {
			continue
		}
		out = append(out, ln)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// ensureCommonBinPATH prepends Homebrew / local bin dirs so LaunchAgent/systemd
// agents with a minimal PATH can still find clamscan, brew, etc.
func ensureCommonBinPATH() {
	cur := os.Getenv("PATH")
	var extras []string
	switch runtime.GOOS {
	case "darwin":
		extras = []string{"/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin", "/usr/local/sbin"}
	case "linux":
		extras = []string{"/usr/local/sbin", "/usr/local/bin", "/usr/sbin", "/sbin"}
	default:
		return
	}
	parts := filepath.SplitList(cur)
	have := map[string]bool{}
	for _, p := range parts {
		have[p] = true
	}
	var prefix []string
	for _, d := range extras {
		if d == "" || have[d] {
			continue
		}
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			prefix = append(prefix, d)
			have[d] = true
		}
	}
	if len(prefix) == 0 {
		return
	}
	_ = os.Setenv("PATH", strings.Join(append(prefix, parts...), string(os.PathListSeparator)))
}

func clamInstallSuggest() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS：执行 brew install clamav && sudo freshclam；若 Agent 以服务运行，重启 Agent 后再扫描"
	case "windows":
		return "Windows：安装 ClamAV 并将 clamscan.exe 加入 PATH 后重启 Agent"
	default:
		return "Linux：apt/yum/apk install clamav（及病毒库），执行 freshclam 后重试"
	}
}

func findClamAVBin() string {
	for _, name := range []string{"clamscan", "clamdscan"} {
		if p, err := exec.LookPath(name); err == nil && p != "" {
			return p
		}
	}
	var candidates []string
	if runtime.GOOS == "darwin" {
		if pref := strings.TrimSpace(cmdOut(4, "brew", "--prefix", "clamav")); pref != "" && !strings.Contains(strings.ToLower(pref), "error") {
			candidates = append(candidates,
				filepath.Join(pref, "bin", "clamscan"),
				filepath.Join(pref, "sbin", "clamscan"),
			)
		}
		candidates = append(candidates,
			"/opt/homebrew/bin/clamscan",
			"/usr/local/bin/clamscan",
			"/opt/local/bin/clamscan",
		)
	} else {
		candidates = append(candidates,
			"/usr/bin/clamscan",
			"/usr/local/bin/clamscan",
			"/bin/clamscan",
		)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

func collectHardeningFindings() []hostSecFinding {
	var fs []hostSecFinding
	if runtime.GOOS == "darwin" {
		return collectDarwinHardeningFindings()
	}
	if runtime.GOOS != "linux" {
		fs = append(fs, hostSecFinding{
			Level: "info", ID: "platform", Title: "非 Linux 平台",
			Detail: "当前 OS=" + runtime.GOOS + "，加固检查以 Linux/macOS 为主，请结合本机安全基线复核",
		})
		return fs
	}
	sshCfg := "/etc/ssh/sshd_config"
	if fileExists(sshCfg) {
		rootLogin := sshCfgValue(sshCfg, "PermitRootLogin", "yes")
		pwdAuth := sshCfgValue(sshCfg, "PasswordAuthentication", "yes")
		if strings.EqualFold(rootLogin, "yes") {
			fs = append(fs, hostSecFinding{
				Level: "high", ID: "ssh_root_login", Title: "SSH 允许 root 登录",
				Detail: "PermitRootLogin=" + rootLogin, Suggest: "设为 prohibit-password 或 no",
			})
		}
		if strings.EqualFold(pwdAuth, "yes") {
			fs = append(fs, hostSecFinding{
				Level: "medium", ID: "ssh_password_auth", Title: "SSH 启用密码认证",
				Detail: "PasswordAuthentication=yes", Suggest: "生产环境建议仅公钥认证",
			})
		}
	}
	if out := cmdOut(3, "getenforce"); out != "" {
		sel := strings.TrimSpace(out)
		if strings.EqualFold(sel, "Disabled") || strings.EqualFold(sel, "Permissive") {
			fs = append(fs, hostSecFinding{
				Level: "medium", ID: "selinux_weak", Title: "SELinux 未强制",
				Detail: sel, Suggest: "评估后设为 Enforcing",
			})
		}
	}
	// world-writable sensitive dirs
	for _, p := range []string{"/tmp", "/var/tmp"} {
		if fi, err := os.Stat(p); err == nil && fi.Mode()&0o002 != 0 {
			fs = append(fs, hostSecFinding{
				Level: "info", ID: "world_writable", Title: "世界可写目录",
				Detail: p + " mode=" + fi.Mode().String(), Suggest: "确认 sticky bit 与定期清理策略",
			})
		}
	}
	failed := 0
	if out := cmdOut(5, "journalctl", "-n", "200", "--no-pager", "-u", "sshd"); out != "" {
		low := strings.ToLower(out)
		failed = strings.Count(low, "failed") + strings.Count(out, "Invalid user")
	}
	if failed > 30 {
		fs = append(fs, hostSecFinding{
			Level: "high", ID: "ssh_bruteforce", Title: "近期 SSH 认证失败偏多",
			Detail: fmt.Sprintf("关键词计数≈%d", failed), Suggest: "启用 fail2ban / 限制来源 IP / 改密钥登录",
		})
	}
	return fs
}

func collectDarwinHardeningFindings() []hostSecFinding {
	var fs []hostSecFinding
	fs = append(fs, hostSecFinding{
		Level: "info", ID: "platform_darwin", Title: "macOS 主机",
		Detail: "已启用 macOS 加固抽检（FileVault / 防火墙 / SIP）与可选 ClamAV",
	})
	if out := strings.TrimSpace(cmdOut(5, "fdesetup", "status")); out != "" {
		low := strings.ToLower(out)
		if strings.Contains(low, "off") || strings.Contains(low, "disabled") {
			fs = append(fs, hostSecFinding{
				Level: "high", ID: "filevault_off", Title: "FileVault 未开启",
				Detail: out, Suggest: "系统设置 → 隐私与安全性 → 开启 FileVault 磁盘加密",
			})
		} else if strings.Contains(low, "on") || strings.Contains(low, "filevault is on") {
			fs = append(fs, hostSecFinding{
				Level: "info", ID: "filevault_on", Title: "FileVault 已开启",
				Detail: out,
			})
		}
	}
	// Firewall status is collected via collectFirewallStatus() + firewallFindings().
	if out := strings.TrimSpace(cmdOut(5, "csrutil", "status")); out != "" {
		low := strings.ToLower(out)
		if strings.Contains(low, "disabled") {
			fs = append(fs, hostSecFinding{
				Level: "high", ID: "sip_disabled", Title: "系统完整性保护（SIP）已关闭",
				Detail: out, Suggest: "除非有明确调试需要，否则应在恢复模式重新启用 SIP",
			})
		}
	}
	return fs
}

func collectSampleHashes(limit int) []hostSecHash {
	paths := []string{}
	// executables in /tmp
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if out := cmdOut(8, "find", "/tmp", "-maxdepth", "2", "-type", "f", "-perm", "-111"); out != "" {
			for _, ln := range strings.Split(out, "\n") {
				ln = strings.TrimSpace(ln)
				if ln != "" {
					paths = append(paths, ln)
				}
			}
		}
		home, _ := os.UserHomeDir()
		for _, p := range []string{
			filepath.Join(home, ".ssh", "authorized_keys"),
			"/etc/crontab",
			"/etc/rc.local",
		} {
			if fileExists(p) {
				paths = append(paths, p)
			}
		}
	}
	out := []hostSecHash{}
	for _, p := range paths {
		if len(out) >= limit {
			break
		}
		if h, ok := hashFileLimited(p, 2<<20); ok {
			out = append(out, h)
		}
	}
	return out
}

func hashFileLimited(path string, maxBytes int64) (hostSecHash, bool) {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return hostSecHash{}, false
	}
	if fi.Size() > maxBytes {
		return hostSecHash{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return hostSecHash{}, false
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, maxBytes)); err != nil {
		return hostSecHash{}, false
	}
	return hostSecHash{
		Path: path, SHA256: hex.EncodeToString(h.Sum(nil)),
		Mode: fi.Mode().String(), Size: fi.Size(),
	}, true
}

func collectIOCFindings(procs, listens []string, hashes []hostSecHash) []hostSecFinding {
	var fs []hostSecFinding
	suspicious := []string{"xmrig", "kdevtmpfsi", "kinsing", "minexmr", "cryptonight", "/dev/shm/", ".onion"}
	blob := strings.ToLower(strings.Join(procs, "\n") + "\n" + strings.Join(listens, "\n"))
	for _, s := range suspicious {
		if strings.Contains(blob, s) {
			fs = append(fs, hostSecFinding{
				Level: "crit", ID: "ioc_process", Title: "可疑进程/路径 IOC",
				Detail: "匹配: " + s, Suggest: "立即隔离主机并取证排查",
			})
		}
	}
	for _, h := range hashes {
		if strings.HasPrefix(h.Path, "/tmp/") && strings.Contains(h.Mode, "x") {
			fs = append(fs, hostSecFinding{
				Level: "medium", ID: "tmp_executable", Title: "/tmp 下存在可执行文件",
				Detail: h.Path, Suggest: "核查来源；清理非预期可执行文件",
			})
			break
		}
	}
	return fs
}

func samplePathsForClam() []string {
	paths := []string{"/tmp", "/var/tmp"}
	if runtime.GOOS == "darwin" {
		paths = append(paths, "/private/tmp")
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		paths = append(paths,
			filepath.Join(home, "Downloads"),
			filepath.Join(home, "Desktop"),
			filepath.Join(home, ".ssh"),
		)
	}
	// When Agent runs as root/LaunchDaemon, also sample common user homes.
	if runtime.GOOS == "darwin" {
		if ents, err := os.ReadDir("/Users"); err == nil {
			n := 0
			for _, e := range ents {
				if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == "Shared" {
					continue
				}
				paths = append(paths, filepath.Join("/Users", e.Name(), "Downloads"))
				n++
				if n >= 3 {
					break
				}
			}
		}
	}
	return paths
}

func runClamAVScan(enable bool, paths []string) hostSecMalware {
	m := hostSecMalware{ClamAV: "unavailable", Infected: []string{}, Findings: []hostSecFinding{}}
	if !enable {
		m.ClamAV = "disabled"
		return m
	}
	bin := findClamAVBin()
	if bin == "" {
		m.Findings = append(m.Findings, hostSecFinding{
			Level: "info", ID: "clamav_missing", Title: "未检测到 ClamAV",
			Detail: "未找到 clamscan（已检查 PATH 与常见安装路径）", Suggest: clamInstallSuggest(),
		})
		return m
	}
	ver := strings.TrimSpace(cmdOut(5, bin, "--version"))
	m.Version = ver
	lowVer := strings.ToLower(ver)
	if strings.Contains(lowVer, "can't open") || strings.Contains(lowVer, "no supported database") ||
		strings.Contains(lowVer, "error:") && !strings.Contains(lowVer, "clamav") {
		m.ClamAV = "error"
		m.Findings = append(m.Findings, hostSecFinding{
			Level: "medium", ID: "clamav_db_missing", Title: "ClamAV 病毒库未就绪",
			Detail: truncateStr(ver, 240), Suggest: "执行 sudo freshclam 更新病毒库后重新扫描",
		})
		return m
	}
	m.ClamAV = "available"
	args := []string{"--infected", "--no-summary", "--max-filesize=5M", "--max-scansize=20M"}
	exist := []string{}
	for _, p := range paths {
		if fileExists(p) {
			exist = append(exist, p)
		}
	}
	if len(exist) == 0 {
		m.Findings = append(m.Findings, hostSecFinding{
			Level: "info", ID: "clamav_ready", Title: "ClamAV 可用但无抽样路径",
			Detail: "引擎: " + bin, Suggest: "确认 Downloads/临时目录可访问，或稍后重试",
		})
		return m
	}
	args = append(args, exist...)
	ctxOut := cmdOut(120, bin, args...)
	m.Scanned = len(exist)
	lowOut := strings.ToLower(ctxOut)
	if strings.Contains(lowOut, "no supported database") || strings.Contains(lowOut, "can't open file or directory") && strings.Contains(lowOut, "daily") {
		m.ClamAV = "error"
		m.Findings = append(m.Findings, hostSecFinding{
			Level: "medium", ID: "clamav_db_missing", Title: "ClamAV 病毒库未就绪",
			Detail: truncateStr(ctxOut, 240), Suggest: "执行 sudo freshclam 更新病毒库后重新扫描",
		})
		return m
	}
	for _, ln := range strings.Split(ctxOut, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if strings.Contains(ln, "FOUND") {
			m.Infected = append(m.Infected, ln)
			m.Findings = append(m.Findings, hostSecFinding{
				Level: "crit", ID: "clamav_hit", Title: "ClamAV 检出恶意软件",
				Detail: ln, Suggest: "隔离文件、全盘复查、轮换凭据",
			})
		}
	}
	if len(m.Infected) == 0 {
		m.Findings = append(m.Findings, hostSecFinding{
			Level: "info", ID: "clamav_clean", Title: "ClamAV 抽样扫描完成",
			Detail: fmt.Sprintf("引擎 %s；已扫描 %d 个路径，未发现感染", bin, m.Scanned),
			Suggest: "定期执行 freshclam 更新病毒库；高敏主机可扩大扫描目录",
		})
	}
	return m
}

func truncateStr(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
