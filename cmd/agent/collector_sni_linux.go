//go:build linux

package main

import (
	"log/slog"
	"net"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"aiops-monitor/shared"
)

const ethPAll = 0x0003 // ETH_P_ALL

func htonsU16(v uint16) uint16 { return v<<8 | v>>8 }

// ---- 经典 BPF(cBPF)：内核侧只放行 IPv4 的 TCP/UDP（含 802.1Q VLAN），其余(ARP/IPv6/ICMP/
// STP…)在内核直接丢弃，userspace 根本收不到，大幅降 CPU 与 syscall/拷贝开销。这是高流量网关
// 的关键降载手段。过滤【保守】——只丢明确无关的，绝不丢 IPv4 TCP/UDP，避免误伤 SNI/DNS/HTTP。
// 端口级精细过滤(只留 53/443/http)留待现场压测后再收紧（错误的端口过滤会静默丢目标流量）。
type sockFilter struct {
	code uint16
	jt   uint8
	jf   uint8
	k    uint32
}
type sockFprog struct {
	length uint16
	_      [6]byte // 64 位对齐：len(2)+pad(6)+ptr(8)
	filter *sockFilter
}

const (
	solSocket      = 1
	soAttachFilter = 26
)

// ipv4TCPUDPFilter：accept(IPv4 && proto∈{TCP,UDP})，含 VLAN 内层；否则 drop。见文末逐条注释。
var ipv4TCPUDPFilter = []sockFilter{
	{0x28, 0, 0, 12},      // 0: ldh [12]  ethertype
	{0x15, 0, 3, 0x0800},  // 1: jeq 0x0800 → i2(非VLAN取proto) else i5(查VLAN)
	{0x30, 0, 0, 23},      // 2: ldb [23]  非VLAN IP proto
	{0x15, 7, 0, 6},       // 3: jeq TCP → ACCEPT(i11)
	{0x15, 6, 7, 17},      // 4: jeq UDP → ACCEPT(i11) else DROP(i12)
	{0x15, 0, 6, 0x8100},  // 5: jeq 0x8100(VLAN) → i6 else DROP(i12)
	{0x28, 0, 0, 16},      // 6: ldh [16]  VLAN 内层 ethertype
	{0x15, 0, 4, 0x0800},  // 7: jeq 0x0800 → i8 else DROP(i12)
	{0x30, 0, 0, 27},      // 8: ldb [27]  VLAN IP proto
	{0x15, 1, 0, 6},       // 9: jeq TCP → ACCEPT(i11)
	{0x15, 0, 1, 17},      // 10: jeq UDP → ACCEPT(i11) else DROP(i12)
	{0x06, 0, 0, 0xffff},  // 11: ret 65535 (accept)
	{0x06, 0, 0, 0},       // 12: ret 0     (drop)
}

// attachBPF 尽力给 AF_PACKET fd 挂 cBPF 过滤器。失败不致命——退回 ETH_P_ALL 全收 + userspace 过滤。
func attachBPF(fd int, filter []sockFilter) error {
	prog := sockFprog{length: uint16(len(filter)), filter: &filter[0]}
	_, _, errno := syscall.Syscall6(syscall.SYS_SETSOCKOPT, uintptr(fd),
		uintptr(solSocket), uintptr(soAttachFilter),
		uintptr(unsafe.Pointer(&prog)), unsafe.Sizeof(prog), 0)
	if errno != 0 {
		return errno
	}
	return nil
}

// run 开一个 AF_PACKET 原始套接字抓包，内核 BPF 过滤 + 读/解析解耦(worker 池)，解析 DNS/SNI，
// 周期 flush 上报。需 root/CAP_NET_RAW。opt-in（配置 enabled=true 才启动）。
func (sc *sniCollector) run(reporter func(shared.DNSMapReport), contentReporter func(shared.ContentAuditReport)) {
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(htonsU16(ethPAll)))
	if err != nil {
		slog.Error("SNI/DNS 抓取: 打开 AF_PACKET 失败（需 root/CAP_NET_RAW）", "err", err)
		return
	}
	defer syscall.Close(fd)

	// 内核 BPF 过滤（best-effort）。必须在 bind 之前挂：bind 前的短暂窗口即使漏几个包也无所谓，
	// 但挂上后能立刻少收无关包。
	if err := attachBPF(fd, ipv4TCPUDPFilter); err != nil {
		slog.Warn("SNI/DNS 抓取: BPF 过滤挂载失败，退回全收+用户态过滤（CPU 略高）", "err", err)
	} else {
		slog.Info("SNI/DNS 抓取: 已挂 BPF 内核过滤（仅 IPv4 TCP/UDP）")
	}

	ifaceName := "all"
	if sc.cfg.Interface != "" {
		if ifi, e := net.InterfaceByName(sc.cfg.Interface); e == nil {
			ll := &syscall.SockaddrLinklayer{Protocol: htonsU16(ethPAll), Ifindex: ifi.Index}
			if e := syscall.Bind(fd, ll); e != nil {
				slog.Warn("SNI/DNS 抓取: 绑定网卡失败，改为全部网卡", "iface", sc.cfg.Interface, "err", e)
			} else {
				ifaceName = sc.cfg.Interface
			}
		} else {
			slog.Warn("SNI/DNS 抓取: 网卡不存在，改为全部网卡", "iface", sc.cfg.Interface)
		}
	}

	// 读/解析解耦：读循环只 recvfrom+拷贝+入队；一组 worker 并发解析，避免解析阻塞抓包导致丢包。
	workers := runtime.NumCPU()
	if workers < 2 {
		workers = 2
	}
	if workers > 8 {
		workers = 8
	}
	frames := make(chan []byte, 4096)
	var dropped atomic.Uint64
	for i := 0; i < workers; i++ {
		go func() {
			for f := range frames {
				runSafe("sni-parse", func() { sc.handle(f) })
			}
		}()
	}
	slog.Info("SNI/DNS 抓取器启动", "iface", ifaceName, "workers", workers)

	// 周期 flush（含 recover）；顺带把丢包计数打出来，便于判断是否需要指定网卡/收紧过滤。
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			runSafe("sni-flush", func() { sc.flush(reporter) })
			runSafe("content-flush", func() { sc.flushContent(contentReporter) })
			if d := dropped.Swap(0); d > 0 {
				slog.Warn("SNI/DNS 抓取: 解析队列满导致丢包（考虑指定 interface 或收紧过滤）", "dropped_30s", d)
			}
		}
	}()

	buf := make([]byte, 65536)
	for {
		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			slog.Warn("SNI/DNS 抓取: 读取错误，继续", "err", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if n <= 0 {
			continue
		}
		// 必须拷贝：buf 会被下一次 recvfrom 复用，而 worker 是异步处理。
		frame := make([]byte, n)
		copy(frame, buf[:n])
		select {
		case frames <- frame:
		default:
			dropped.Add(1) // 队列满：丢弃而非阻塞抓包（丢包好过卡死整条采集）
		}
	}
}
