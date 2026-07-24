package main

// SNI + DNS 抓取的纯解析层（与原始套接字解耦，可单测）：从以太网帧里取 IPv4/IPv6 TCP/UDP，
// 再从 DNS 应答提取「A 记录 → 查询域名」、从 TLS ClientHello 提取 SNI 域名。
// 目的：把"某目的 IP 对应的真实域名"抓出来（用户实际请求的域名，比反向 DNS 更准），
// 不解密任何内容——SNI 与 DNS 本就是明文。全部手写字节解析，零依赖。

import (
	"encoding/binary"
	"net"
	"strings"
)

// ipDomain 是一条「IP ↔ 域名」观测。
type ipDomain struct {
	ip     string
	domain string
	source string // "dns" | "sni"
}

// l4Info 是解析出的四层信息。
type l4Info struct {
	proto    uint8
	srcIP    string
	dstIP    string
	srcPort  uint16
	dstPort  uint16
	seq      uint32 // TCP 序列号（流重组用）
	tcpFlags uint8  // TCP 标志位（FIN=0x01/RST=0x04… 用于连接结束判定）
	payload  []byte
}

// parseEthernetFrame 支持 IPv4/IPv6、802.1Q 与 QinQ。IPv6 扩展头被有界跳过；
// 非首分片没有 L4 头，安全丢弃。
func parseEthernetFrame(frame []byte) (l4Info, bool) {
	if len(frame) < 14 {
		return l4Info{}, false
	}
	etherType := binary.BigEndian.Uint16(frame[12:14])
	off := 14
	for tags := 0; (etherType == 0x8100 || etherType == 0x88a8) && tags < 2; tags++ {
		if len(frame) < off+4 {
			return l4Info{}, false
		}
		etherType = binary.BigEndian.Uint16(frame[off+2 : off+4])
		off += 4
	}
	switch etherType {
	case 0x0800:
		return parseIPv4(frame[off:])
	case 0x86dd:
		return parseIPv6(frame[off:])
	default:
		return l4Info{}, false
	}
}

// parseEthIPv4 保留给旧调用/测试；IPv6 请使用 parseEthernetFrame。
func parseEthIPv4(frame []byte) (l4Info, bool) {
	if len(frame) < 14 {
		return l4Info{}, false
	}
	etherType := binary.BigEndian.Uint16(frame[12:14])
	off := 14
	for tags := 0; (etherType == 0x8100 || etherType == 0x88a8) && tags < 2; tags++ {
		if len(frame) < off+4 {
			return l4Info{}, false
		}
		etherType = binary.BigEndian.Uint16(frame[off+2 : off+4])
		off += 4
	}
	if etherType != 0x0800 {
		return l4Info{}, false
	}
	return parseIPv4(frame[off:])
}

// parseIPv4 从 IP 头起解析 IPv4 + TCP/UDP。
func parseIPv4(pkt []byte) (l4Info, bool) {
	if len(pkt) < 20 || pkt[0]>>4 != 4 {
		return l4Info{}, false
	}
	ihl := int(pkt[0]&0x0f) * 4
	if ihl < 20 || len(pkt) < ihl {
		return l4Info{}, false
	}
	if total := int(binary.BigEndian.Uint16(pkt[2:4])); total >= ihl && total < len(pkt) {
		pkt = pkt[:total]
	}
	frag := binary.BigEndian.Uint16(pkt[6:8])
	if frag&0x1fff != 0 { // 非首分片不含 TCP/UDP 头
		return l4Info{}, false
	}
	info := l4Info{
		proto: pkt[9],
		srcIP: net.IP(pkt[12:16]).String(),
		dstIP: net.IP(pkt[16:20]).String(),
	}
	return parseTransport(info, pkt[ihl:])
}

// parseIPv6 跳过常见扩展头（Hop-by-Hop、Routing、Fragment、AH、Destination）。
func parseIPv6(pkt []byte) (l4Info, bool) {
	if len(pkt) < 40 || pkt[0]>>4 != 6 {
		return l4Info{}, false
	}
	if payloadLen := int(binary.BigEndian.Uint16(pkt[4:6])); payloadLen > 0 && 40+payloadLen < len(pkt) {
		pkt = pkt[:40+payloadLen]
	}
	next := pkt[6]
	off := 40
	for extensions := 0; extensions < 8; extensions++ {
		switch next {
		case 0, 43, 60: // Hop-by-Hop, Routing, Destination Options
			if len(pkt) < off+2 {
				return l4Info{}, false
			}
			hdrLen := (int(pkt[off+1]) + 1) * 8
			if hdrLen < 8 || len(pkt) < off+hdrLen {
				return l4Info{}, false
			}
			next, off = pkt[off], off+hdrLen
		case 44: // Fragment
			if len(pkt) < off+8 {
				return l4Info{}, false
			}
			fragment := binary.BigEndian.Uint16(pkt[off+2 : off+4])
			if fragment&0xfff8 != 0 {
				return l4Info{}, false // 非首分片
			}
			next, off = pkt[off], off+8
		case 51: // Authentication Header
			if len(pkt) < off+2 {
				return l4Info{}, false
			}
			hdrLen := (int(pkt[off+1]) + 2) * 4
			if hdrLen < 8 || len(pkt) < off+hdrLen {
				return l4Info{}, false
			}
			next, off = pkt[off], off+hdrLen
		default:
			info := l4Info{
				proto: next,
				srcIP: net.IP(pkt[8:24]).String(),
				dstIP: net.IP(pkt[24:40]).String(),
			}
			if off > len(pkt) {
				return l4Info{}, false
			}
			return parseTransport(info, pkt[off:])
		}
	}
	return l4Info{}, false
}

func parseTransport(info l4Info, l4 []byte) (l4Info, bool) {
	switch info.proto {
	case 6: // TCP
		if len(l4) < 20 {
			return l4Info{}, false
		}
		info.srcPort = binary.BigEndian.Uint16(l4[0:2])
		info.dstPort = binary.BigEndian.Uint16(l4[2:4])
		info.seq = binary.BigEndian.Uint32(l4[4:8])
		info.tcpFlags = l4[13]
		dataOff := int(l4[12]>>4) * 4
		if dataOff < 20 || len(l4) < dataOff {
			return l4Info{}, false
		}
		info.payload = l4[dataOff:]
	case 17: // UDP
		if len(l4) < 8 {
			return l4Info{}, false
		}
		info.srcPort = binary.BigEndian.Uint16(l4[0:2])
		info.dstPort = binary.BigEndian.Uint16(l4[2:4])
		info.payload = l4[8:]
	default:
		return l4Info{}, false
	}
	return info, true
}

// readDNSName 从 DNS 报文 off 处读一个域名（处理压缩指针 0xC0），返回域名、名字之后的偏移、ok。
func readDNSName(msg []byte, off int) (string, int, bool) {
	var labels []string
	jumped := false
	next := off
	guard := 0
	for {
		if off >= len(msg) || guard > 128 {
			return "", 0, false
		}
		guard++
		b := int(msg[off])
		if b == 0 { // 结束
			off++
			if !jumped {
				next = off
			}
			return strings.Join(labels, "."), next, true
		}
		if b&0xC0 == 0xC0 { // 压缩指针
			if off+1 >= len(msg) {
				return "", 0, false
			}
			ptr := (b&0x3F)<<8 | int(msg[off+1])
			if !jumped {
				next = off + 2
			}
			jumped = true
			off = ptr
			continue
		}
		// 普通标签
		if off+1+b > len(msg) {
			return "", 0, false
		}
		labels = append(labels, string(msg[off+1:off+1+b]))
		off += 1 + b
	}
}

// parseDNSResponse 解析 DNS 应答，把每条 A 记录的 IP 映射到【原始查询域名】(qname)——
// 用户想访问的是 qname，而不是 CNAME 链上的 CDN 名。返回 IP→域名 观测列表。
func parseDNSResponse(payload []byte) []ipDomain {
	if len(payload) < 12 {
		return nil
	}
	flags := binary.BigEndian.Uint16(payload[2:4])
	if flags&0x8000 == 0 { // 非应答
		return nil
	}
	qd := int(binary.BigEndian.Uint16(payload[4:6]))
	an := int(binary.BigEndian.Uint16(payload[6:8]))
	off := 12
	var qname string
	for i := 0; i < qd; i++ {
		name, no, ok := readDNSName(payload, off)
		if !ok || no+4 > len(payload) {
			return nil
		}
		if i == 0 {
			qname = name
		}
		off = no + 4 // 跳过 qtype(2)+qclass(2)
	}
	if qname == "" {
		return nil
	}
	var out []ipDomain
	for i := 0; i < an; i++ {
		_, no, ok := readDNSName(payload, off)
		if !ok || no+10 > len(payload) {
			break
		}
		rrType := binary.BigEndian.Uint16(payload[no : no+2])
		rdlen := int(binary.BigEndian.Uint16(payload[no+8 : no+10]))
		rdata := no + 10
		if rdata+rdlen > len(payload) {
			break
		}
		if rrType == 1 && rdlen == 4 { // A 记录
			out = append(out, ipDomain{ip: net.IP(payload[rdata : rdata+4]).String(), domain: qname, source: "dns"})
		} else if rrType == 28 && rdlen == 16 { // AAAA
			out = append(out, ipDomain{ip: net.IP(payload[rdata : rdata+16]).String(), domain: qname, source: "dns"})
		}
		off = rdata + rdlen
	}
	return out
}

// parseTLSClientHelloSNI 从可见的 TLS ClientHello 提取 SNI(server_name)。启用 ECH 时真实
// SNI 会被加密，因此空结果是合法边界，不应当被解释为“没有访问域名”。
func parseTLSClientHelloSNI(payload []byte) string {
	// TLS record: type(1)=0x16 handshake, version(2), length(2)
	if len(payload) < 6 || payload[0] != 0x16 {
		return ""
	}
	p := payload[5:]
	// handshake: msg_type(1)=0x01 ClientHello, length(3), version(2), random(32)
	if len(p) < 38 || p[0] != 0x01 {
		return ""
	}
	idx := 4 + 2 + 32 // 跳过 handshake头(4)+版本(2)+random(32)
	if idx >= len(p) {
		return ""
	}
	// session id
	sidLen := int(p[idx])
	idx += 1 + sidLen
	if idx+2 > len(p) {
		return ""
	}
	// cipher suites
	csLen := int(binary.BigEndian.Uint16(p[idx : idx+2]))
	idx += 2 + csLen
	if idx+1 > len(p) {
		return ""
	}
	// compression methods
	cmLen := int(p[idx])
	idx += 1 + cmLen
	if idx+2 > len(p) {
		return ""
	}
	// extensions
	extTotal := int(binary.BigEndian.Uint16(p[idx : idx+2]))
	idx += 2
	end := idx + extTotal
	if end > len(p) {
		end = len(p)
	}
	for idx+4 <= end {
		extType := binary.BigEndian.Uint16(p[idx : idx+2])
		extSize := int(binary.BigEndian.Uint16(p[idx+2 : idx+4]))
		idx += 4
		if idx+extSize > end {
			break
		}
		if extType == 0x0000 { // server_name
			sni := p[idx : idx+extSize]
			// server_name_list: list_len(2), name_type(1)=0, name_len(2), name...
			if len(sni) >= 5 && sni[2] == 0 {
				nameLen := int(binary.BigEndian.Uint16(sni[3:5]))
				if 5+nameLen <= len(sni) {
					return string(sni[5 : 5+nameLen])
				}
			}
			return ""
		}
		idx += extSize
	}
	return ""
}
