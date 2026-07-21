package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// —— 只读运维模块：禁止改配置/启停服务/写文件。异常时返回可读错误与非零退出码。——

func moduleDiskUsage() ([]byte, int) {
	switch runtime.GOOS {
	case "windows":
		return runModuleCmds([][]string{{"cmd", "/c", "wmic logicaldisk get Caption,FreeSpace,Size /format:list"}})
	default:
		return runModuleCmds([][]string{{"df", "-hT"}})
	}
}

func moduleMemInfo() ([]byte, int) {
	var b strings.Builder
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	fmt.Fprintf(&b, "go_alloc_mb=%.1f\n", float64(ms.Alloc)/1024/1024)
	fmt.Fprintf(&b, "go_sys_mb=%.1f\n", float64(ms.Sys)/1024/1024)
	switch runtime.GOOS {
	case "linux":
		if raw, err := os.ReadFile("/proc/meminfo"); err == nil {
			b.WriteString("--- /proc/meminfo (head) ---\n")
			lines := strings.Split(string(raw), "\n")
			for i, ln := range lines {
				if i >= 12 {
					break
				}
				b.WriteString(ln)
				b.WriteByte('\n')
			}
			return []byte(b.String()), 0
		}
	case "darwin":
		out, exit := runModuleCmds([][]string{{"vm_stat"}, {"sysctl", "-n", "hw.memsize"}})
		b.Write(out)
		return []byte(b.String()), exit
	case "windows":
		return runModuleCmds([][]string{{"cmd", "/c", "wmic OS get FreePhysicalMemory,TotalVisibleMemorySize /format:list"}})
	}
	return []byte(b.String()), 0
}

func moduleCPULoad() ([]byte, int) {
	var b strings.Builder
	fmt.Fprintf(&b, "cpus=%d\ngoos=%s\ngoarch=%s\n", runtime.NumCPU(), runtime.GOOS, runtime.GOARCH)
	switch runtime.GOOS {
	case "linux":
		if raw, err := os.ReadFile("/proc/loadavg"); err == nil {
			fmt.Fprintf(&b, "loadavg=%s", string(raw))
		}
		if raw, err := os.ReadFile("/proc/stat"); err == nil {
			lines := strings.Split(string(raw), "\n")
			if len(lines) > 0 {
				fmt.Fprintf(&b, "stat_cpu=%s\n", lines[0])
			}
		}
		return []byte(b.String()), 0
	case "darwin":
		out, exit := runModuleCmds([][]string{{"sysctl", "-n", "vm.loadavg"}, {"sysctl", "-n", "hw.ncpu"}})
		b.Write(out)
		return []byte(b.String()), exit
	case "windows":
		return runModuleCmds([][]string{{"cmd", "/c", "wmic cpu get LoadPercentage,NumberOfCores /format:list"}})
	}
	return []byte(b.String()), 0
}

func moduleProcessTop() ([]byte, int) {
	switch runtime.GOOS {
	case "windows":
		return runModuleCmds([][]string{{"cmd", "/c", "tasklist /FO LIST"}})
	case "darwin":
		return runModuleCmds([][]string{{"ps", "-axo", "pid,pcpu,pmem,rss,comm"}})
	default:
		return runModuleCmds([][]string{{"ps", "-eo", "pid,pcpu,pmem,rss,comm"}})
	}
}

func moduleUptimeInfo() ([]byte, int) {
	switch runtime.GOOS {
	case "windows":
		return runModuleCmds([][]string{{"cmd", "/c", "net statistics workstation"}})
	default:
		return runModuleCmds([][]string{{"uptime"}, {"who"}})
	}
}

func modulePkgList() ([]byte, int) {
	switch runtime.GOOS {
	case "linux":
		switch {
		case have("dpkg"):
			return runModuleCmds([][]string{{"dpkg", "-l"}})
		case have("rpm"):
			return runModuleCmds([][]string{{"rpm", "-qa"}})
		case have("apk"):
			return runModuleCmds([][]string{{"apk", "info"}})
		}
		return []byte("未找到 dpkg/rpm/apk"), 1
	case "darwin":
		if have("brew") {
			return runModuleCmds([][]string{{"brew", "list", "--versions"}})
		}
		return []byte("未找到 brew"), 1
	case "windows":
		if have("winget") {
			return runModuleCmds([][]string{{"winget", "list"}})
		}
		return runModuleCmds([][]string{{"cmd", "/c", "wmic product get name,version"}})
	}
	return []byte("不支持的系统"), 1
}

func moduleFileStat(args map[string]string) ([]byte, int) {
	path := strings.TrimSpace(args["path"])
	if path == "" {
		return []byte("file_stat 缺少 path"), 1
	}
	if agentDeniedPath(path) {
		return []byte("拒绝访问敏感路径"), 1
	}
	fi, err := os.Stat(path)
	if err != nil {
		return []byte("stat 失败: " + err.Error()), 1
	}
	abs, _ := filepath.Abs(path)
	var b strings.Builder
	fmt.Fprintf(&b, "path=%s\n", abs)
	fmt.Fprintf(&b, "name=%s\n", fi.Name())
	fmt.Fprintf(&b, "size=%d\n", fi.Size())
	fmt.Fprintf(&b, "mode=%s\n", fi.Mode().String())
	fmt.Fprintf(&b, "isdir=%v\n", fi.IsDir())
	fmt.Fprintf(&b, "mtime=%s\n", fi.ModTime().Format(time.RFC3339))
	return []byte(b.String()), 0
}

func moduleFileHead(args map[string]string) ([]byte, int) {
	path := strings.TrimSpace(args["path"])
	if path == "" {
		return []byte("file_head 缺少 path"), 1
	}
	if agentDeniedPath(path) {
		return []byte("拒绝访问敏感路径"), 1
	}
	n := 64 * 1024
	if v := strings.TrimSpace(args["bytes"]); v != "" {
		if x, err := strconv.Atoi(v); err == nil && x > 0 && x <= 256*1024 {
			n = x
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return []byte("打开失败: " + err.Error()), 1
	}
	defer f.Close()
	buf := make([]byte, n)
	nr, _ := f.Read(buf)
	return buf[:nr], 0
}

func agentDeniedPath(p string) bool {
	p = strings.ToLower(strings.TrimSpace(p))
	deny := []string{
		"/etc/shadow", "/etc/gshadow", "/etc/sudoers",
		".ssh/", ".gnupg/", ".aws/", ".kube/config",
		"/root/.bash_history",
	}
	for _, d := range deny {
		if strings.Contains(p, d) {
			return true
		}
	}
	return false
}

func moduleServiceStatus(args map[string]string) ([]byte, int) {
	name := strings.TrimSpace(args["name"])
	if name == "" {
		return []byte("service_status 缺少 name"), 1
	}
	switch runtime.GOOS {
	case "linux":
		return runModuleCmds([][]string{
			{"systemctl", "is-active", name},
			{"systemctl", "status", name, "--no-pager", "-l"},
		})
	case "windows":
		return runModuleCmds([][]string{{"sc", "query", name}})
	case "darwin":
		return runModuleCmds([][]string{{"brew", "services", "info", name}})
	}
	return []byte("不支持的系统"), 1
}

func moduleJournalRecent(args map[string]string) ([]byte, int) {
	n := "80"
	if v := strings.TrimSpace(args["lines"]); v != "" {
		n = v
	}
	switch runtime.GOOS {
	case "linux":
		if have("journalctl") {
			return runModuleCmds([][]string{{"journalctl", "-n", n, "--no-pager", "-o", "short-iso"}})
		}
		return runModuleCmds([][]string{{"tail", "-n", n, "/var/log/messages"}, {"tail", "-n", n, "/var/log/syslog"}})
	case "darwin":
		return runModuleCmds([][]string{{"log", "show", "--last", "30m", "--style", "compact"}})
	case "windows":
		return runModuleCmds([][]string{{"cmd", "/c", "wevtutil qe System /c:" + n + " /f:text"}})
	}
	return []byte("不支持的系统"), 1
}

func moduleDmesgRecent() ([]byte, int) {
	switch runtime.GOOS {
	case "linux", "darwin":
		return runModuleCmds([][]string{{"dmesg", "-T"}})
	default:
		return []byte("当前系统无 dmesg"), 1
	}
}

func moduleNetIfaces() ([]byte, int) {
	var b strings.Builder
	ifaces, err := net.Interfaces()
	if err != nil {
		return []byte(err.Error()), 1
	}
	for _, ifc := range ifaces {
		fmt.Fprintf(&b, "[%s] flags=%s mtu=%d\n", ifc.Name, ifc.Flags.String(), ifc.MTU)
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			fmt.Fprintf(&b, "  addr=%s\n", a.String())
		}
	}
	return []byte(b.String()), 0
}

func moduleNetListen() ([]byte, int) {
	switch runtime.GOOS {
	case "windows":
		return runModuleCmds([][]string{{"cmd", "/c", "netstat -ano"}})
	default:
		if have("ss") {
			return runModuleCmds([][]string{{"ss", "-lntup"}})
		}
		return runModuleCmds([][]string{{"netstat", "-lntp"}})
	}
}

func moduleNetRoutes() ([]byte, int) {
	switch runtime.GOOS {
	case "windows":
		return runModuleCmds([][]string{{"route", "print"}})
	case "darwin":
		return runModuleCmds([][]string{{"netstat", "-rn"}})
	default:
		if have("ip") {
			return runModuleCmds([][]string{{"ip", "route"}})
		}
		return runModuleCmds([][]string{{"route", "-n"}})
	}
}

func moduleNetSockets() ([]byte, int) {
	switch runtime.GOOS {
	case "windows":
		return runModuleCmds([][]string{{"cmd", "/c", "netstat -an"}})
	default:
		if have("ss") {
			return runModuleCmds([][]string{{"ss", "-s"}, {"ss", "-ant"}})
		}
		return runModuleCmds([][]string{{"netstat", "-an"}})
	}
}

func moduleDNSResolve(args map[string]string) ([]byte, int) {
	host := strings.TrimSpace(args["host"])
	if host == "" {
		return []byte("dns_resolve 缺少 host"), 1
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return []byte("解析失败: " + err.Error()), 1
	}
	var b strings.Builder
	fmt.Fprintf(&b, "host=%s\n", host)
	for _, ip := range ips {
		fmt.Fprintf(&b, "ip=%s\n", ip.String())
	}
	return []byte(b.String()), 0
}

func moduleDockerPS() ([]byte, int) {
	if !have("docker") {
		return []byte("未安装 docker 或不在 PATH"), 1
	}
	return runModuleCmds([][]string{{"docker", "ps", "-a", "--format", "table {{.ID}}\t{{.Names}}\t{{.Status}}\t{{.Image}}\t{{.Ports}}"}})
}

func moduleDockerStats() ([]byte, int) {
	if !have("docker") {
		return []byte("未安装 docker 或不在 PATH"), 1
	}
	return runModuleCmds([][]string{{"docker", "stats", "--no-stream", "--format", "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}"}})
}

func moduleKubeGet(args map[string]string) ([]byte, int) {
	if !have("kubectl") {
		return []byte("未安装 kubectl 或不在 PATH"), 1
	}
	res := strings.TrimSpace(args["resource"])
	if res == "" {
		res = "pods"
	}
	// 只允许只读 get 子资源名字符
	for _, c := range res {
		if !(c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return []byte("非法 resource 参数"), 1
		}
	}
	return runModuleCmds([][]string{{"kubectl", "get", res, "-A", "-o", "wide"}})
}

func moduleTimeSync() ([]byte, int) {
	var b strings.Builder
	fmt.Fprintf(&b, "now=%s\n", time.Now().Format(time.RFC3339Nano))
	fmt.Fprintf(&b, "unix=%d\n", time.Now().Unix())
	zone, off := time.Now().Zone()
	fmt.Fprintf(&b, "zone=%s offset_sec=%d\n", zone, off)
	switch runtime.GOOS {
	case "linux":
		if have("timedatectl") {
			out, _ := runModuleCmds([][]string{{"timedatectl", "status"}})
			b.Write(out)
		}
	}
	return []byte(b.String()), 0
}

func moduleUsersLogged() ([]byte, int) {
	switch runtime.GOOS {
	case "windows":
		return runModuleCmds([][]string{{"query", "user"}})
	default:
		return runModuleCmds([][]string{{"who"}, {"w"}})
	}
}

func moduleSecurityListen() ([]byte, int) {
	// 与 net_listen 相同数据源，附加说明头
	out, exit := moduleNetListen()
	head := []byte("# security_listen: 对外监听端口（只读）\n")
	return append(head, out...), exit
}

func moduleAuthFailures() ([]byte, int) {
	switch runtime.GOOS {
	case "linux":
		if have("journalctl") {
			return runModuleCmds([][]string{{"journalctl", "-n", "50", "--no-pager", "-u", "sshd", "-g", "Failed|Invalid|authentication failure"}})
		}
		return runModuleCmds([][]string{{"grep", "-E", "Failed|Invalid|authentication failure", "/var/log/auth.log"}, {"tail", "-n", "50", "/var/log/secure"}})
	case "darwin":
		return runModuleCmds([][]string{{"log", "show", "--last", "1h", "--predicate", "eventMessage CONTAINS \"Authentication\"", "--style", "compact"}})
	default:
		return []byte("当前系统暂无统一认证失败日志接口"), 1
	}
}

func moduleBigdataJPS() ([]byte, int) {
	if !have("jps") {
		return []byte("未找到 jps（需 JDK）"), 1
	}
	return runModuleCmds([][]string{{"jps", "-lvm"}})
}

func moduleBigdataPorts() ([]byte, int) {
	// 常见 Hadoop/Spark/Kafka/ES 端口探测（只看本机监听）
	ports := []string{"8020", "8088", "9000", "9870", "9864", "2181", "9092", "9200", "9300", "7077", "8080", "18080"}
	var b strings.Builder
	b.WriteString("# bigdata_ports: 检查本机是否监听常见大数据端口\n")
	listenOut, _ := moduleNetListen()
	listen := string(listenOut)
	for _, p := range ports {
		hit := strings.Contains(listen, ":"+p) || strings.Contains(listen, "."+p+" ")
		fmt.Fprintf(&b, "port=%s listening=%v\n", p, hit)
	}
	return []byte(b.String()), 0
}
