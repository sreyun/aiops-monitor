package main

// Cross-platform packet-capture backend powered by Wireshark TShark.
// TShark delegates capture to libpcap/BPF on macOS and Npcap on Windows and
// emits only selected fields; the Agent never writes a pcap file to disk.

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"aiops-monitor/shared"
)

var tsharkFields = []string{
	"frame.time_epoch",
	"ip.src", "ipv6.src", "ip.dst", "ipv6.dst",
	"tcp.srcport", "tcp.dstport", "udp.srcport", "udp.dstport",
	"tcp.seq_raw", "tcp.flags", "tcp.payload", "udp.payload",
	"tls.handshake.extensions_server_name",
	"dns.qry.name", "dns.a", "dns.aaaa",
}

type tsharkCaptureRecord struct {
	info       l4Info
	sni        string
	dnsName    string
	dnsAddress string
}

func (sc *sniCollector) runTShark(ctx context.Context, reporter func(shared.DNSMapReport), contentReporter func(shared.ContentAuditReport)) error {
	binary, err := resolveTSharkPath(sc.cfg.TSharkPath)
	if err != nil {
		return err
	}
	args := []string{"-l", "-n"}
	if iface := strings.TrimSpace(sc.cfg.Interface); iface != "" {
		args = append(args, "-i", iface)
	}
	args = append(args, "-f", buildTSharkCaptureFilter(sc.cfg), "-T", "fields",
		"-E", "separator=\t", "-E", "quote=n", "-E", "occurrence=f", "-E", "escape=y")
	for _, field := range tsharkFields {
		args = append(args, "-e", field)
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("打开 tshark stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("打开 tshark stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 tshark: %w", err)
	}

	var lastStderr string
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 4096), 256<<10)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				lastStderr = line
			}
		}
	}()

	records := make(chan tsharkCaptureRecord, 2048)
	done := make(chan error, 1)
	go func() {
		defer close(records)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64<<10), 4<<20)
		for scanner.Scan() {
			rec, ok := parseTSharkCaptureLine(scanner.Text())
			if !ok {
				continue
			}
			select {
			case records <- rec:
			case <-ctx.Done():
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				done <- ctx.Err()
				return
			}
		}
		scanErr := scanner.Err()
		waitErr := cmd.Wait()
		if scanErr != nil {
			done <- fmt.Errorf("读取 tshark 输出: %w", scanErr)
		} else {
			done <- waitErr
		}
	}()

	ras := newReassembler(sc.cfg, sc.addContent)
	sweep := time.NewTicker(20 * time.Second)
	flush := time.NewTicker(30 * time.Second)
	defer sweep.Stop()
	defer flush.Stop()
	defer func() {
		sc.flush(reporter)
		sc.flushContent(contentReporter)
		sc.logContentDrops()
	}()

	slog.Info("TShark 内容审计后端已就绪",
		"path", binary, "interface", sc.cfg.Interface, "filter", buildTSharkCaptureFilter(sc.cfg))
	for {
		select {
		case <-ctx.Done():
			<-stderrDone
			return nil
		case rec, ok := <-records:
			if !ok {
				err := <-done
				<-stderrDone
				if err == nil {
					return fmt.Errorf("tshark 已退出%s", formatTSharkStderr(lastStderr))
				}
				return fmt.Errorf("tshark 退出: %w%s", err, formatTSharkStderr(lastStderr))
			}
			directSNI := rec.sni != "" && rec.info.dstIP != ""
			if directSNI {
				sc.observeSNI(rec.info, rec.sni)
			}
			if rec.dnsName != "" && rec.dnsAddress != "" {
				sc.add(ipDomain{ip: rec.dnsAddress, domain: rec.dnsName, source: "dns"})
			}
			if len(rec.info.payload) > 0 {
				if !directSNI {
					sc.handleL4(rec.info)
				}
				if sc.cfg.ContentAudit && rec.info.proto == 6 && !directSNI {
					ras.feed(rec.info)
				}
			}
		case <-sweep.C:
			if sc.cfg.ContentAudit {
				ras.sweepIdle(60)
			}
		case <-flush.C:
			sc.flush(reporter)
			sc.flushContent(contentReporter)
			sc.logContentDrops()
		}
	}
}

func formatTSharkStderr(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) > 512 {
		s = s[len(s)-512:]
	}
	return ": " + s
}

func resolveTSharkPath(configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if p, err := exec.LookPath(configured); err == nil {
			return p, nil
		}
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured, nil
		}
		return "", fmt.Errorf("未找到配置的 tshark: %s", configured)
	}
	if p, err := exec.LookPath("tshark"); err == nil {
		return p, nil
	}
	for _, candidate := range standardTSharkPaths() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("未找到 tshark；Windows 请安装 Wireshark+Npcap，macOS 请安装 Wireshark/ChmodBPF")
}

func standardTSharkPaths() []string {
	var paths []string
	switch runtime.GOOS {
	case "windows":
		for _, envName := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			if root := strings.TrimSpace(os.Getenv(envName)); root != "" {
				paths = append(paths, filepath.Join(root, "Wireshark", "tshark.exe"))
			}
		}
	case "darwin":
		paths = append(paths,
			"/Applications/Wireshark.app/Contents/MacOS/tshark",
			"/opt/homebrew/bin/tshark",
			"/usr/local/bin/tshark",
		)
	default:
		paths = append(paths, "/usr/bin/tshark", "/usr/local/bin/tshark")
	}
	return paths
}

func buildTSharkCaptureFilter(cfg SNIConfig) string {
	ports := append([]int(nil), cfg.TLSMetadataPorts...)
	if len(ports) == 0 {
		ports = []int{443, 8443, 9443}
	}
	if cfg.ContentAudit {
		ports = append(ports, cfg.ContentAuditPorts...)
	}
	sort.Ints(ports)
	seen := map[int]bool{}
	parts := []string{"udp port 53", "tcp port 53"}
	for _, p := range ports {
		if p < 1 || p > 65535 || seen[p] {
			continue
		}
		seen[p] = true
		parts = append(parts, "tcp port "+strconv.Itoa(p))
	}
	return "(" + strings.Join(parts, " or ") + ")"
}

func parseTSharkCaptureLine(line string) (tsharkCaptureRecord, bool) {
	var rec tsharkCaptureRecord
	fields := strings.Split(strings.TrimRight(line, "\r\n"), "\t")
	if len(fields) < len(tsharkFields) {
		fields = append(fields, make([]string, len(tsharkFields)-len(fields))...)
	}
	first := func(a, b string) string {
		if strings.TrimSpace(a) != "" {
			return strings.TrimSpace(a)
		}
		return strings.TrimSpace(b)
	}
	rec.info.srcIP = first(fields[1], fields[2])
	rec.info.dstIP = first(fields[3], fields[4])

	tcpSrc, _ := strconv.ParseUint(strings.TrimSpace(fields[5]), 10, 16)
	tcpDst, _ := strconv.ParseUint(strings.TrimSpace(fields[6]), 10, 16)
	udpSrc, _ := strconv.ParseUint(strings.TrimSpace(fields[7]), 10, 16)
	udpDst, _ := strconv.ParseUint(strings.TrimSpace(fields[8]), 10, 16)
	if tcpSrc > 0 || tcpDst > 0 {
		rec.info.proto = 6
		rec.info.srcPort, rec.info.dstPort = uint16(tcpSrc), uint16(tcpDst)
		seq, _ := strconv.ParseUint(strings.TrimSpace(fields[9]), 10, 32)
		rec.info.seq = uint32(seq)
		flags, _ := strconv.ParseUint(strings.TrimSpace(fields[10]), 0, 16)
		rec.info.tcpFlags = uint8(flags)
		rec.info.payload, _ = decodeTSharkHex(fields[11])
	} else if udpSrc > 0 || udpDst > 0 {
		rec.info.proto = 17
		rec.info.srcPort, rec.info.dstPort = uint16(udpSrc), uint16(udpDst)
		rec.info.payload, _ = decodeTSharkHex(fields[12])
	}
	rec.sni = strings.TrimSpace(fields[13])
	rec.dnsName = strings.TrimSuffix(strings.TrimSpace(fields[14]), ".")
	rec.dnsAddress = first(fields[15], fields[16])
	return rec, rec.info.proto != 0 || rec.sni != "" || (rec.dnsName != "" && rec.dnsAddress != "")
}

func decodeTSharkHex(s string) ([]byte, error) {
	s = strings.NewReplacer(":", "", " ", "", "\\", "").Replace(strings.TrimSpace(s))
	if s == "" {
		return nil, nil
	}
	if len(s)%2 != 0 {
		return nil, io.ErrUnexpectedEOF
	}
	return hex.DecodeString(s)
}
