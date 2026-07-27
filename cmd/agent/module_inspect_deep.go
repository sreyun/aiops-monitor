package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Linux 深度巡检段（对齐 linux_inspect.sh）；非 Linux 写入 status=skip。

func (b *inspectBuilder) skipSec(id, title, reason string) {
	b.rep.Sections = append(b.rep.Sections, inspectSection{
		ID: id, Title: title, Status: "skip", Summary: reason,
	})
}

func (b *inspectBuilder) collectInode() {
	if runtime.GOOS != "linux" {
		b.skipSec("inode", "Inode 使用", "仅 Linux 支持")
		return
	}
	st := "ok"
	items := []inspectItem{}
	alert := 0
	out := cmdOut(5, "df", "-iP")
	for i, ln := range strings.Split(out, "\n") {
		if i == 0 || strings.TrimSpace(ln) == "" {
			continue
		}
		f := strings.Fields(ln)
		if len(f) < 6 {
			continue
		}
		fs, usep, mount := f[0], f[4], f[5]
		if skipMount(mount, "") || strings.Contains(fs, "tmpfs") {
			continue
		}
		if usep == "-" {
			continue
		}
		pctStr := strings.TrimSuffix(usep, "%")
		pct, err := strconv.ParseFloat(pctStr, 64)
		if err != nil {
			continue
		}
		ist := "ok"
		if pct >= b.rep.Thresholds.InodeWarn+10 {
			ist, st, alert = "crit", b.worst(st, "crit"), alert+1
			b.addFinding("crit", "inode", fmt.Sprintf("%s Inode 使用率过高: %.0f%%", mount, pct))
		} else if pct >= b.rep.Thresholds.InodeWarn {
			ist, st, alert = "warn", b.worst(st, "warn"), alert+1
			b.addFinding("warn", "inode", fmt.Sprintf("%s Inode 使用率偏高: %.0f%%", mount, pct))
		}
		items = append(items, inspectItem{Label: mount, Value: usep + " · " + fs, Status: ist})
	}
	b.rep.Metrics.InodeAlertCnt = alert
	b.rep.Sections = append(b.rep.Sections, inspectSection{
		ID: "inode", Title: "Inode 使用", Status: st,
		Summary: fmt.Sprintf("%d 挂载点，%d 告警", len(items), alert), Items: items,
	})
}

func (b *inspectBuilder) collectFD() {
	if runtime.GOOS != "linux" {
		b.skipSec("fd", "文件描述符", "仅 Linux 支持")
		return
	}
	st := "ok"
	raw := readFileTrim("/proc/sys/fs/file-nr")
	f := strings.Fields(raw)
	cur, max := uint64(0), uint64(1)
	if len(f) >= 3 {
		cur, _ = strconv.ParseUint(f[0], 10, 64)
		max, _ = strconv.ParseUint(f[2], 10, 64)
	}
	pct := 0.0
	if max > 0 {
		pct = float64(cur) / float64(max) * 100
	}
	b.rep.Metrics.FDUsagePct = round1(pct)
	items := []inspectItem{
		{Label: "已分配 FD", Value: fmt.Sprintf("%d", cur)},
		{Label: "系统上限", Value: fmt.Sprintf("%d", max)},
		{Label: "使用率", Value: fmt.Sprintf("%.1f%%", pct)},
	}
	// 进程 FD TOP 5（抽样前 200 个 pid）
	type fdRow struct {
		name string
		pid  string
		n    int
	}
	var rows []fdRow
	ents, _ := os.ReadDir("/proc")
	checked := 0
	for _, e := range ents {
		if checked >= 200 {
			break
		}
		name := e.Name()
		if name == "" || name[0] < '0' || name[0] > '9' {
			continue
		}
		checked++
		fdDir := filepath.Join("/proc", name, "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		comm := readFileTrim(filepath.Join("/proc", name, "comm"))
		if comm == "" {
			comm = "unknown"
		}
		rows = append(rows, fdRow{name: comm, pid: name, n: len(fds)})
	}
	// 简单选择排序取前 5
	for i := 0; i < len(rows) && i < 5; i++ {
		maxI := i
		for j := i + 1; j < len(rows); j++ {
			if rows[j].n > rows[maxI].n {
				maxI = j
			}
		}
		rows[i], rows[maxI] = rows[maxI], rows[i]
		items = append(items, inspectItem{
			Label: fmt.Sprintf("FD TOP#%d", i+1),
			Value: fmt.Sprintf("%s pid=%s fd=%d", rows[i].name, rows[i].pid, rows[i].n),
		})
	}
	if pct >= b.rep.Thresholds.FDWarn+10 {
		st = "crit"
		b.addFinding("crit", "fd", fmt.Sprintf("文件描述符使用率过高: %.1f%%", pct))
	} else if pct >= b.rep.Thresholds.FDWarn {
		st = "warn"
		b.addFinding("warn", "fd", fmt.Sprintf("文件描述符使用率偏高: %.1f%%", pct))
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{ID: "fd", Title: "文件描述符", Status: st, Items: items})
}

func (b *inspectBuilder) collectDiskIO() {
	if runtime.GOOS != "linux" {
		b.skipSec("diskio", "磁盘 I/O", "仅 Linux 支持")
		return
	}
	st := "info"
	items := []inspectItem{}
	if out := cmdOut(6, "iostat", "-d", "-x", "1", "2"); out != "" {
		// 取第二轮样本行
		lines := strings.Split(out, "\n")
		seenHeader := 0
		for _, ln := range lines {
			if strings.HasPrefix(ln, "Device") {
				seenHeader++
				continue
			}
			if seenHeader < 2 {
				continue
			}
			f := strings.Fields(ln)
			if len(f) < 6 {
				continue
			}
			dev := f[0]
			if strings.HasPrefix(dev, "loop") || strings.HasPrefix(dev, "ram") {
				continue
			}
			// util% 通常在最后一列
			util := f[len(f)-1]
			items = append(items, inspectItem{Label: dev, Value: "util%=" + util + "  " + strings.Join(f[1:], " ")})
		}
	}
	if len(items) == 0 {
		// fallback /proc/diskstats 累计值
		raw, err := os.ReadFile("/proc/diskstats")
		if err == nil {
			for _, ln := range strings.Split(string(raw), "\n") {
				f := strings.Fields(ln)
				if len(f) < 14 {
					continue
				}
				dev := f[2]
				if strings.HasPrefix(dev, "loop") || strings.HasPrefix(dev, "ram") || strings.HasPrefix(dev, "dm-") {
					continue
				}
				rd, _ := strconv.ParseUint(f[3], 10, 64)
				wr, _ := strconv.ParseUint(f[7], 10, 64)
				if rd == 0 && wr == 0 {
					continue
				}
				items = append(items, inspectItem{
					Label: dev,
					Value: fmt.Sprintf("reads=%d writes=%d (累计)", rd, wr),
				})
				if len(items) >= 12 {
					break
				}
			}
		}
	}
	if len(items) == 0 {
		st = "skip"
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{ID: "diskio", Title: "磁盘 I/O", Status: st, Items: items})
}

func (b *inspectBuilder) collectContainers() {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		b.skipSec("docker", "容器运行时", "当前平台跳过")
		return
	}
	st := "info"
	items := []inspectItem{}
	ctr := ""
	for _, c := range []string{"docker", "podman"} {
		if cmdOut(3, c, "info") != "" {
			ctr = c
			break
		}
	}
	if ctr == "" {
		b.skipSec("docker", "容器运行时", "未检测到 Docker / Podman")
		return
	}
	ver := strings.TrimSpace(cmdOut(3, ctr, "version", "--format", "{{.Server.Version}}"))
	if ver == "" {
		ver = strings.TrimSpace(cmdOut(3, ctr, "--version"))
	}
	running, total := 0, 0
	var rows []inspectItem
	out := cmdOut(8, ctr, "ps", "-a", "--format", "{{.Names}}\t{{.Image}}\t{{.Status}}")
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		total++
		f := strings.Split(ln, "\t")
		name, image, status := ln, "", ""
		if len(f) >= 3 {
			name, image, status = f[0], f[1], f[2]
		}
		ist := "ok"
		low := strings.ToLower(status)
		if strings.Contains(low, "up") || strings.Contains(low, "running") {
			running++
		} else {
			ist = "warn"
		}
		if len(rows) < 20 {
			rows = append(rows, inspectItem{Label: name, Value: image + " · " + status, Status: ist})
		}
	}
	b.rep.Metrics.ContainerCount = total
	items = append(items,
		inspectItem{Label: "运行时", Value: ctr + " " + ver},
		inspectItem{Label: "容器统计", Value: fmt.Sprintf("运行 %d / 总计 %d", running, total)},
	)
	items = append(items, rows...)
	if df := cmdOut(6, ctr, "system", "df"); df != "" {
		items = append(items, inspectItem{Label: "磁盘占用", Value: compressLines(df, 6)})
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{
		ID: "docker", Title: "容器运行时", Status: st,
		Summary: fmt.Sprintf("%s · %d 容器", ctr, total), Items: items,
	})
}

func compressLines(s string, max int) string {
	var keep []string
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		keep = append(keep, ln)
		if len(keep) >= max {
			break
		}
	}
	return strings.Join(keep, " | ")
}

func (b *inspectBuilder) collectCron() {
	if runtime.GOOS != "linux" {
		b.skipSec("cron", "定时任务", "仅 Linux 支持")
		return
	}
	st := "info"
	items := []inspectItem{}
	addFile := func(user, src, path string) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return
		}
		n := 0
		for _, ln := range strings.Split(string(raw), "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" || strings.HasPrefix(ln, "#") || strings.Contains(ln, "=") && !strings.Contains(ln, " ") {
				// 跳过环境变量行（粗略）
				if strings.HasPrefix(ln, "SHELL=") || strings.HasPrefix(ln, "PATH=") || strings.HasPrefix(ln, "MAILTO=") {
					continue
				}
			}
			if ln == "" || strings.HasPrefix(ln, "#") {
				continue
			}
			n++
			if len(items) < 40 {
				val := ln
				if len(val) > 120 {
					val = val[:120] + "…"
				}
				items = append(items, inspectItem{Label: user + "@" + src, Value: val})
			}
		}
		if n > 0 && len(items) < 40 {
			// already added
			_ = n
		}
	}
	if fileExists("/etc/crontab") {
		addFile("system", "crontab", "/etc/crontab")
	}
	ents, _ := os.ReadDir("/etc/cron.d")
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		addFile("system", e.Name(), filepath.Join("/etc/cron.d", e.Name()))
	}
	for _, dir := range []string{"/var/spool/cron/crontabs", "/var/spool/cron"} {
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			addFile(e.Name(), "user", filepath.Join(dir, e.Name()))
		}
	}
	if len(items) == 0 {
		st = "ok"
		items = append(items, inspectItem{Label: "结果", Value: "未发现可解析的定时任务"})
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{
		ID: "cron", Title: "定时任务", Status: st,
		Summary: fmt.Sprintf("%d 条(截断展示)", len(items)), Items: items,
	})
}

func (b *inspectBuilder) collectKernel() {
	if runtime.GOOS != "linux" {
		b.skipSec("kernel", "内核参数", "仅 Linux 支持")
		return
	}
	st := "info"
	params := []struct{ key, desc, suggest string }{
		{"net.ipv4.tcp_syncookies", "TCP SYN Cookies", "1"},
		{"net.ipv4.ip_forward", "IP 转发", "视需求"},
		{"net.ipv4.tcp_max_syn_backlog", "SYN 队列", ">=1024"},
		{"net.core.somaxconn", "Socket 队列", ">=1024"},
		{"net.ipv4.tcp_tw_reuse", "TIME_WAIT 重用", "1"},
		{"net.ipv4.tcp_fin_timeout", "FIN 超时", "<=30"},
		{"net.ipv4.tcp_keepalive_time", "Keepalive", "<=600"},
		{"vm.swappiness", "Swap 倾向", "<=30"},
		{"fs.file-max", "系统 FD 上限", ">=65535"},
		{"net.ipv4.conf.all.rp_filter", "反向路径过滤", "1"},
		{"kernel.panic", "panic 重启", ">0"},
	}
	items := []inspectItem{}
	for _, p := range params {
		val := strings.TrimSpace(cmdOut(2, "sysctl", "-n", p.key))
		if val == "" {
			val = "N/A"
		}
		items = append(items, inspectItem{
			Label: p.key,
			Value: fmt.Sprintf("%s = %s（建议 %s）", p.desc, val, p.suggest),
		})
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{ID: "kernel", Title: "内核参数", Status: st, Items: items})
}

func (b *inspectBuilder) collectLogs() {
	if runtime.GOOS != "linux" {
		b.skipSec("logs", "系统日志 / OOM", "仅 Linux 支持")
		return
	}
	st := "ok"
	items := []inspectItem{}
	dmesg := cmdOut(6, "dmesg")
	if dmesg == "" {
		dmesg = cmdOut(6, "journalctl", "-k", "--no-pager", "-n", "2000")
	}
	oom := strings.Count(strings.ToLower(dmesg), "out of memory") + strings.Count(strings.ToLower(dmesg), "oom-killer") + strings.Count(strings.ToLower(dmesg), "oom_reaper")
	b.rep.Metrics.OOMCount = oom
	items = append(items, inspectItem{Label: "OOM 事件(估)", Value: fmt.Sprintf("%d", oom)})
	if oom > 0 {
		st = "warn"
		b.addFinding("warn", "logs", fmt.Sprintf("内核日志检测到约 %d 次 OOM 相关记录", oom))
	}
	hw := 0
	for _, kw := range []string{"hardware error", "machine check", "i/o error", "medium error"} {
		hw += strings.Count(strings.ToLower(dmesg), kw)
	}
	items = append(items, inspectItem{Label: "硬件错误关键词(估)", Value: fmt.Sprintf("%d", hw)})
	if hw > 0 {
		st = b.worst(st, "warn")
		b.addFinding("warn", "logs", "dmesg 中出现硬件/IO 错误关键词，请复核")
	}
	// 异常日志片段
	var errLog string
	for _, p := range []string{"/var/log/messages", "/var/log/syslog"} {
		if fileExists(p) {
			errLog = cmdOut(5, "grep", "-iE", "error|fail|critical|panic|oom", p)
			break
		}
	}
	if errLog == "" {
		errLog = cmdOut(5, "journalctl", "-p", "err", "--no-pager", "-n", "20")
	}
	if errLog != "" {
		items = append(items, inspectItem{Label: "异常日志(截断)", Value: compressLines(errLog, 12)})
	}
	tmp := strings.TrimSpace(cmdOut(4, "du", "-sh", "/tmp"))
	vlog := strings.TrimSpace(cmdOut(4, "du", "-sh", "/var/log"))
	if tmp != "" {
		items = append(items, inspectItem{Label: "/tmp 大小", Value: tmp})
	}
	if vlog != "" {
		items = append(items, inspectItem{Label: "/var/log 大小", Value: vlog})
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{ID: "logs", Title: "系统日志 / OOM", Status: st, Items: items})
}

func (b *inspectBuilder) collectSSL() {
	if runtime.GOOS != "linux" {
		b.skipSec("ssl", "SSL 证书", "仅 Linux 支持")
		return
	}
	if cmdOut(2, "openssl", "version") == "" {
		b.skipSec("ssl", "SSL 证书", "未安装 openssl")
		return
	}
	st := "ok"
	items := []inspectItem{}
	paths := []string{
		"/etc/letsencrypt/live", "/etc/ssl/certs", "/etc/pki/tls/certs",
		"/etc/nginx/ssl", "/etc/nginx/conf.d", "/etc/httpd/conf.d",
	}
	seen := map[string]bool{}
	total, expiring, expired := 0, 0, 0
	warnDays := b.rep.Thresholds.SSLDays
	if warnDays <= 0 {
		warnDays = 30
	}
	now := time.Now()
	for _, root := range paths {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			base := strings.ToLower(info.Name())
			if !(strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".crt") || base == "cert.pem" || strings.HasPrefix(base, "fullchain")) {
				return nil
			}
			if total >= 40 {
				return filepath.SkipDir
			}
			real := path
			if rp, err := filepath.EvalSymlinks(path); err == nil {
				real = rp
			}
			if seen[real] {
				return nil
			}
			seen[real] = true
			endOut := cmdOut(3, "openssl", "x509", "-in", path, "-noout", "-enddate")
			if !strings.HasPrefix(endOut, "notAfter=") {
				return nil
			}
			endStr := strings.Join(strings.Fields(strings.TrimSpace(strings.TrimPrefix(endOut, "notAfter="))), " ")
			endT, err := time.Parse("Jan 2 15:04:05 2006 MST", endStr)
			if err != nil {
				endT, err = time.Parse("Jan 2 15:04:05 2006 GMT", endStr)
			}
			if err != nil {
				return nil
			}
			total++
			days := int(endT.Sub(now).Hours() / 24)
			cn := strings.TrimSpace(cmdOut(3, "openssl", "x509", "-in", path, "-noout", "-subject"))
			if i := strings.Index(cn, "CN"); i >= 0 {
				cn = strings.TrimSpace(cn[i:])
				cn = strings.TrimPrefix(cn, "CN")
				cn = strings.TrimLeft(cn, " =")
				if j := strings.IndexAny(cn, "/,"); j >= 0 {
					cn = cn[:j]
				}
			}
			if cn == "" {
				cn = filepath.Base(path)
			}
			ist := "ok"
			badge := "正常"
			if days < 0 {
				ist, st, expired, badge = "crit", b.worst(st, "crit"), expired+1, "已过期"
				b.addFinding("crit", "ssl", fmt.Sprintf("证书已过期: %s (%s)", cn, path))
			} else if days < warnDays {
				ist, st, expiring, badge = "warn", b.worst(st, "warn"), expiring+1, "即将过期"
				b.addFinding("warn", "ssl", fmt.Sprintf("证书 %d 天后过期: %s", days, cn))
			}
			items = append(items, inspectItem{
				Label: cn, Value: fmt.Sprintf("%s · 剩余 %d 天 · %s · %s", badge, days, endStr, path), Status: ist,
			})
			return nil
		})
	}
	b.rep.Metrics.SSLExpiring = expiring
	b.rep.Metrics.SSLExpired = expired
	if total == 0 {
		b.skipSec("ssl", "SSL 证书", "未扫描到可读证书")
		return
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{
		ID: "ssl", Title: "SSL 证书", Status: st,
		Summary: fmt.Sprintf("共 %d · 将过期 %d · 已过期 %d", total, expiring, expired),
		Items:   items,
	})
}

func (b *inspectBuilder) collectLargeFiles() {
	if runtime.GOOS != "linux" {
		b.skipSec("large", "大文件", "仅 Linux 支持")
		return
	}
	st := "info"
	items := []inspectItem{}
	// 限路径 + head，避免全盘拖死
	out := cmdOut(25, "bash", "-c",
		`find /var /home /opt /usr/local -xdev -type f -size +100M 2>/dev/null | head -40 | xargs -r du -sh 2>/dev/null | sort -rh | head -10`)
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		items = append(items, inspectItem{Label: "大文件>100M", Value: ln})
	}
	recent := cmdOut(25, "bash", "-c",
		`find /var /home /opt /usr/local -xdev -type f -size +50M -mtime -7 2>/dev/null | head -40 | xargs -r du -sh 2>/dev/null | sort -rh | head -10`)
	for _, ln := range strings.Split(recent, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		items = append(items, inspectItem{Label: "近7天大文件>50M", Value: ln})
	}
	if len(items) == 0 {
		items = append(items, inspectItem{Label: "结果", Value: "未发现超阈值大文件（或权限不足）"})
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{ID: "large", Title: "大文件分析", Status: st, Items: items})
}

func (b *inspectBuilder) collectUpdates() {
	b.collectUpdatesInner(true)
}

func (b *inspectBuilder) collectUpdatesLight() {
	b.collectUpdatesInner(false)
}

func (b *inspectBuilder) collectUpdatesInner(network bool) {
	if runtime.GOOS != "linux" {
		b.skipSec("update", "系统更新", "仅 Linux 支持")
		return
	}
	st := "info"
	items := []inspectItem{}
	family := b.rep.Host.OSFamily
	pkgFam := b.rep.Host.PkgFamily
	if pkgFam == "" {
		switch family {
		case "debian", "uos":
			pkgFam = "deb"
		case "rhel":
			pkgFam = "rpm"
		case "kylin":
			// Kylin Desktop=deb, Server=rpm — never assume RPM-only.
			pkgFam = classifyLinuxPkg(strings.ToLower(b.rep.Host.OS+" "+b.rep.Host.DistroID), "kylin")
		case "suse":
			pkgFam = "zypper"
		case "alpine":
			pkgFam = "apk"
		case "arch":
			pkgFam = "pacman"
		}
	}
	info := "N/A"
	count := 0
	// Use cmdOutRaw: cmdOut → sanitizeInspectField collapses newlines to spaces,
	// which makes per-line apt/dnf/yum/zypper/pacman counts always 0/1.
	switch {
	case pkgFam == "deb" || family == "debian" || family == "uos" || (family == "kylin" && pkgFam == "deb"):
		if network {
			_ = cmdOut(30, "apt-get", "-qq", "update")
		}
		count = countAptUpgradable(string(cmdOutRaw(20, "apt", "list", "--upgradable")))
		info = fmt.Sprintf("apt: %d 个可升级", count)
	case pkgFam == "rpm" || family == "rhel" || (family == "kylin" && pkgFam != "deb"):
		// Rocky 9/10、RHEL clones、麒麟 V10/V11 Server（RPM）走 dnf/yum。
		if network {
			out := string(cmdOutRaw(40, "dnf", "check-update", "--quiet"))
			if strings.TrimSpace(out) == "" {
				out = string(cmdOutRaw(40, "yum", "check-update", "--quiet"))
			}
			count = countDnfCheckUpdate(out)
			info = fmt.Sprintf("dnf/yum: %d 个可用更新", count)
		} else {
			info = "已跳过联网检查（standard）；使用 deep 档位可执行完整检查"
		}
	case family == "suse" || pkgFam == "zypper":
		if network {
			count = countZypperUpdates(string(cmdOutRaw(40, "zypper", "list-updates")))
			info = fmt.Sprintf("zypper: %d 个可用更新", count)
		} else {
			info = "已跳过联网检查（standard）"
		}
	case family == "alpine" || pkgFam == "apk":
		if network {
			_ = cmdOut(20, "apk", "update")
			count = countNonEmptyLines(string(cmdOutRaw(20, "apk", "version", "-l", "<")))
			info = fmt.Sprintf("apk: %d 个可用更新", count)
		} else {
			info = "已跳过联网检查（standard）"
		}
	case family == "arch" || pkgFam == "pacman":
		count = countNonEmptyLines(string(cmdOutRaw(20, "pacman", "-Qu")))
		info = fmt.Sprintf("pacman: %d 个可用更新", count)
	default:
		info = "未知包管理器，请人工检查更新"
	}
	items = append(items, inspectItem{Label: "可用更新", Value: info})
	if count > 50 {
		st = "warn"
		b.addFinding("warn", "update", fmt.Sprintf("待安装更新较多: %d", count))
	} else if count > 0 && network {
		st = "info"
	}
	// 最近安装包（rpm/dpkg 尽力）
	if last := strings.TrimSpace(cmdOut(5, "rpm", "-qa", "--last")); last != "" {
		items = append(items, inspectItem{Label: "最近安装(rpm)", Value: compressLines(last, 1)})
	} else if last := strings.TrimSpace(cmdOut(5, "bash", "-c", `grep " install " /var/log/dpkg.log 2>/dev/null | tail -1`)); last != "" {
		items = append(items, inspectItem{Label: "最近安装(dpkg)", Value: last})
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{ID: "update", Title: "系统更新", Status: st, Items: items})
}

func (b *inspectBuilder) collectRecommend() {
	st := "info"
	short, mid, long := []string{}, []string{}, []string{}
	crit, warn := b.rep.Result.Critical, b.rep.Result.Warnings
	if crit > 0 {
		short = append(short, fmt.Sprintf("优先处理 %d 项严重告警", crit))
	}
	if b.rep.Metrics.DiskAlertCount > 0 {
		short = append(short, "清理告警磁盘上的大文件/日志，释放空间")
	}
	if b.rep.Metrics.OOMCount > 0 {
		short = append(short, fmt.Sprintf("排查约 %d 次 OOM，必要时扩容内存或限流", b.rep.Metrics.OOMCount))
	}
	if b.rep.Metrics.SSLExpired > 0 {
		short = append(short, fmt.Sprintf("立即续签 %d 张已过期证书", b.rep.Metrics.SSLExpired))
	}
	if b.rep.Metrics.ZombieCount > 0 {
		short = append(short, fmt.Sprintf("清理 %d 个僵尸进程并检查父进程", b.rep.Metrics.ZombieCount))
	}
	if len(short) == 0 {
		short = append(short, "当前无紧急问题", "保持每周例行巡检")
	}
	if warn > 0 {
		mid = append(mid, fmt.Sprintf("梳理 %d 项警告并制定整改计划", warn))
	}
	if b.rep.Metrics.SSLExpiring > 0 {
		mid = append(mid, fmt.Sprintf("提前续签 %d 张即将过期证书", b.rep.Metrics.SSLExpiring))
	}
	mid = append(mid, "检查关键服务备份策略与监控覆盖", "按业务负载复核内核参数与 FD/连接队列")
	long = append(long,
		"建立服务器健康基线，定期对比指标变化",
		"完善自动化巡检与告警推送",
		"规划容量增长与弹性扩展",
		"建立日志/审计归档与合规流程",
		"定期演练故障恢复与应急响应",
	)
	items := []inspectItem{
		{Label: "短期(1-3天)", Value: strings.Join(short, "；")},
		{Label: "中期(1-4周)", Value: strings.Join(mid, "；")},
		{Label: "长期(1-6月)", Value: strings.Join(long, "；")},
		{Label: "免责声明", Value: "本报告为巡检时刻瞬时只读采集结果，阈值仅供参考，请结合监控与业务场景综合判断。"},
	}
	b.rep.Sections = append(b.rep.Sections, inspectSection{
		ID: "recommend", Title: "总体建议", Status: st,
		Summary: fmt.Sprintf("警告 %d · 严重 %d · 档位 %s", warn, crit, b.profile),
		Items:   items,
	})
}

// countAptUpgradable counts package rows from `apt list --upgradable`.
// Ignores the "Listing..." header and warning banners.
func countAptUpgradable(out string) int {
	n := 0
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "Listing") || strings.HasPrefix(ln, "WARNING") {
			continue
		}
		if strings.Contains(ln, "[upgradable") || strings.Contains(ln, "upgradable from") {
			n++
		}
	}
	return n
}

// countDnfCheckUpdate counts package rows from `dnf/yum check-update`.
// Skips metadata headers and section titles like "Obsoleting Packages".
func countDnfCheckUpdate(out string) int {
	n := 0
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		lower := strings.ToLower(ln)
		if strings.HasPrefix(lower, "last ") || strings.HasPrefix(lower, "security:") ||
			strings.HasSuffix(ln, ":") || strings.HasPrefix(lower, "obsoleting") ||
			strings.HasPrefix(lower, "available") {
			continue
		}
		fields := strings.Fields(ln)
		if len(fields) < 2 {
			continue
		}
		pkg := fields[0]
		// Real rows look like "kernel.x86_64" / "bash.aarch64".
		if !strings.Contains(pkg, ".") {
			continue
		}
		n++
	}
	return n
}

func countZypperUpdates(out string) int {
	n := 0
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		lower := strings.ToLower(ln)
		// Header row: "v | S | Name | Type | ..."
		if strings.Contains(lower, "| name |") || strings.Contains(lower, "|name|") {
			continue
		}
		if strings.HasPrefix(ln, "v ") || strings.HasPrefix(ln, "v|") || strings.HasPrefix(ln, "v |") {
			n++
		}
	}
	return n
}

func countNonEmptyLines(out string) int {
	n := 0
	for _, ln := range strings.Split(out, "\n") {
		if strings.TrimSpace(ln) != "" {
			n++
		}
	}
	return n
}
