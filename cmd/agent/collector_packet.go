package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"aiops-monitor/shared"
)

// PacketConfig is the agent config section for five-tuple packet capture.
type PacketConfig struct {
	Enabled          bool   `json:"enabled"`
	Interface        string `json:"interface"`
	BPFFilter        string `json:"bpf_filter"`
	SampleRate       int    `json:"sample_rate"`
	MaxPacketsPerMin int    `json:"max_packets_per_min"`
}

// packetCollector reads /proc/net/nf_conntrack periodically and computes
// incremental flow records by diffing against the previous snapshot.
// Linux-only; on other platforms this is a no-op.
type packetCollector struct {
	cfg    PacketConfig
	hostID string
	fp     string

	prevSnapshot map[string]conntrackEntry
}

type conntrackEntry struct {
	srcIP   string
	dstIP   string
	srcPort uint16
	dstPort uint16
	proto   uint8
	state   string
	bytes   uint64
	packets uint64
	lastSeen int64
}

func newPacketCollector(cfg PacketConfig, hostID, fp string) *packetCollector {
	return &packetCollector{
		cfg:          cfg,
		hostID:       hostID,
		fp:           fp,
		prevSnapshot: make(map[string]conntrackEntry),
	}
}

// run starts the periodic nf_conntrack polling loop.
func (pc *packetCollector) run(reporter func(shared.NetFlowReport)) {
	if runtime.GOOS != "linux" {
		slog.Info("五元组包采集器: 仅支持 Linux (nf_conntrack)，当前平台跳过", "os", runtime.GOOS)
		return
	}
	if !pc.cfg.Enabled {
		return
	}
	slog.Info("五元组包采集器启动 (nf_conntrack)", "interval", "30s")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		entries, err := pc.readConntrack()
		if err != nil {
			slog.Warn("读取 nf_conntrack 失败", "err", err)
			continue
		}

		flows := pc.diff(entries)
		if len(flows) == 0 {
			continue
		}

		// Rate limit: cap at MaxPacketsPerMin (default 6000)
		maxPkt := pc.cfg.MaxPacketsPerMin
		if maxPkt <= 0 {
			maxPkt = 6000
		}
		if len(flows) > maxPkt {
			flows = flows[:maxPkt]
		}

		var totalBytes, totalPackets uint64
		for _, f := range flows {
			totalBytes += f.Bytes
			totalPackets += f.Packets
		}

		reporter(shared.NetFlowReport{
			HostID:      pc.hostID,
			Fingerprint: pc.fp,
			Source:      "packet",
			Timestamp:   time.Now().Unix(),
			WindowSec:   30,
			Flows:       flows,
			Stats: shared.NetFlowStats{
				TotalFlows:   len(flows),
				TotalBytes:   totalBytes,
				TotalPackets: totalPackets,
			},
		})
	}
}

// readConntrack parses /proc/net/nf_conntrack and returns a map of current entries.
func (pc *packetCollector) readConntrack() (map[string]conntrackEntry, error) {
	f, err := os.Open("/proc/net/nf_conntrack")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	entries := make(map[string]conntrackEntry)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Text()
		entry, ok := parseConntrackLine(line)
		if !ok {
			continue
		}
		key := conntrackKey(entry)
		entries[key] = entry
	}

	return entries, scanner.Err()
}

// parseConntrackLine parses one line of /proc/net/nf_conntrack.
// Example: ipv4  2 tcp  6 431999 ESTABLISHED src=10.0.0.1 dst=10.0.0.2 sport=443 dport=52341 ...
func parseConntrackLine(line string) (conntrackEntry, bool) {
	var e conntrackEntry
	e.lastSeen = time.Now().Unix()

	fields := strings.Fields(line)
	if len(fields) < 6 {
		return e, false
	}

	// Detect protocol: ipv4/ipv6 + proto number
	protoStr := ""
	for _, f := range fields {
		switch f {
		case "tcp":
			e.proto = 6
			protoStr = "tcp"
		case "udp":
			e.proto = 17
			protoStr = "udp"
		case "icmp":
			e.proto = 1
			protoStr = "icmp"
		}
	}
	_ = protoStr

	// Extract key=value pairs
	for _, f := range fields {
		if !strings.Contains(f, "=") {
			// Check for state (ESTABLISHED, TIME_WAIT, etc.)
			switch f {
			case "ESTABLISHED", "TIME_WAIT", "CLOSE_WAIT", "SYN_SENT", "SYN_RECV",
				"FIN_WAIT", "CLOSE", "LAST_ACK", "LISTEN", "UNREPLIED", "ASSURED":
				e.state = f
			}
			continue
		}
		kv := strings.SplitN(f, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k, v := kv[0], kv[1]
		switch k {
		case "src":
			if e.srcIP == "" {
				e.srcIP = v
			}
		case "dst":
			if e.dstIP == "" {
				e.dstIP = v
			}
		case "sport":
			if e.srcPort == 0 {
				if p, err := strconv.ParseUint(v, 10, 16); err == nil {
					e.srcPort = uint16(p)
				}
			}
		case "dport":
			if e.dstPort == 0 {
				if p, err := strconv.ParseUint(v, 10, 16); err == nil {
					e.dstPort = uint16(p)
				}
			}
		case "bytes":
			// Not always present in conntrack; estimate from packets
		case "packets":
			// Same
		}
	}

	if e.srcIP == "" || e.dstIP == "" {
		return e, false
	}
	return e, true
}

func conntrackKey(e conntrackEntry) string {
	return fmt.Sprintf("%s:%d->%s:%d/%d", e.srcIP, e.srcPort, e.dstIP, e.dstPort, e.proto)
}

// diff computes incremental flow records between the current and previous snapshot.
func (pc *packetCollector) diff(current map[string]conntrackEntry) []shared.FlowRecord {
	var flows []shared.FlowRecord
	now := time.Now().Unix()

	for key, cur := range current {
		prev, existed := pc.prevSnapshot[key]
		if !existed {
			// New connection
			flows = append(flows, shared.FlowRecord{
				SrcIP:     cur.srcIP,
				DstIP:     cur.dstIP,
				SrcPort:   cur.srcPort,
				DstPort:   cur.dstPort,
				Protocol:  cur.proto,
				Bytes:     cur.bytes,
				Packets:   cur.packets,
				FirstSeen: now,
				LastSeen:  now,
			})
		} else {
			// Existing connection: compute delta
			var deltaBytes, deltaPackets uint64
			if cur.bytes > prev.bytes {
				deltaBytes = cur.bytes - prev.bytes
			}
			if cur.packets > prev.packets {
				deltaPackets = cur.packets - prev.packets
			}
			if deltaBytes > 0 || deltaPackets > 0 {
				flows = append(flows, shared.FlowRecord{
					SrcIP:     cur.srcIP,
					DstIP:     cur.dstIP,
					SrcPort:   cur.srcPort,
					DstPort:   cur.dstPort,
					Protocol:  cur.proto,
					Bytes:     deltaBytes,
					Packets:   deltaPackets,
					FirstSeen: prev.lastSeen,
					LastSeen:  now,
				})
			}
		}
	}

	// Update snapshot
	pc.prevSnapshot = current
	return flows
}

// ipToInt converts an IPv4 address string to uint32 for comparison.
func ipToInt(ip string) uint32 {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return 0
	}
	parsed = parsed.To4()
	if parsed == nil {
		return 0
	}
	return uint32(parsed[0])<<24 | uint32(parsed[1])<<16 | uint32(parsed[2])<<8 | uint32(parsed[3])
}
