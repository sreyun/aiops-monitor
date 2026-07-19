package main

import "testing"

func TestParseDNSResponseA(t *testing.T) {
	// example.com A → 93.184.216.34（带压缩指针的标准应答）
	dns := []byte{
		0x12, 0x34, 0x81, 0x80, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, // header
		0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 0x03, 'c', 'o', 'm', 0x00, // qname example.com
		0x00, 0x01, 0x00, 0x01, // qtype A, qclass IN
		0xC0, 0x0C, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x3C, 0x00, 0x04, 93, 184, 216, 34, // answer
	}
	got := parseDNSResponse(dns)
	if len(got) != 1 || got[0].ip != "93.184.216.34" || got[0].domain != "example.com" {
		t.Fatalf("DNS 解析错: %+v", got)
	}
	// 非应答报文（flags 无 QR 位）应返回 nil
	dns[2] = 0x01
	if r := parseDNSResponse(dns); r != nil {
		t.Errorf("非应答不应解析出记录: %+v", r)
	}
	// 截断报文不 panic
	if r := parseDNSResponse(dns[:8]); r != nil {
		t.Errorf("截断报文应返回 nil")
	}
}

// buildClientHelloSNI 构造一个带 SNI 的最小 TLS ClientHello。
func buildClientHelloSNI(sni string) []byte {
	name := []byte(sni)
	snl := []byte{0x00, byte(len(name) >> 8), byte(len(name))}
	snl = append(snl, name...)
	sniData := []byte{byte(len(snl) >> 8), byte(len(snl))}
	sniData = append(sniData, snl...)
	ext := []byte{0x00, 0x00, byte(len(sniData) >> 8), byte(len(sniData))}
	ext = append(ext, sniData...)

	body := []byte{0x03, 0x03}          // client_version
	body = append(body, make([]byte, 32)...) // random
	body = append(body, 0x00)                // session id len
	body = append(body, 0x00, 0x02, 0x13, 0x01) // cipher suites (len + 1 suite)
	body = append(body, 0x01, 0x00)             // compression methods (len + null)
	body = append(body, byte(len(ext)>>8), byte(len(ext)))
	body = append(body, ext...)

	hs := []byte{0x01, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	hs = append(hs, body...)

	rec := []byte{0x16, 0x03, 0x01, byte(len(hs) >> 8), byte(len(hs))}
	rec = append(rec, hs...)
	return rec
}

func TestParseTLSClientHelloSNI(t *testing.T) {
	if got := parseTLSClientHelloSNI(buildClientHelloSNI("api.openai.com")); got != "api.openai.com" {
		t.Errorf("SNI 解析错: %q", got)
	}
	if got := parseTLSClientHelloSNI(buildClientHelloSNI("chat.deepseek.com")); got != "chat.deepseek.com" {
		t.Errorf("SNI 解析错: %q", got)
	}
	// 非握手/截断输入不 panic、返回 ""
	if parseTLSClientHelloSNI([]byte{0x17, 0x03, 0x03, 0x00, 0x10}) != "" {
		t.Errorf("非握手记录应返回空")
	}
	full := buildClientHelloSNI("example.com")
	if parseTLSClientHelloSNI(full[:20]) != "" {
		t.Errorf("截断 ClientHello 应返回空")
	}
}

func TestParseEthIPv4UDP(t *testing.T) {
	// 构造 以太网(IPv4) + IPv4 + UDP(src=53) 载荷，验证四层解析。
	udpPayload := []byte{0xDE, 0xAD}
	udp := []byte{0x00, 0x35, 0x04, 0x00, 0x00, byte(8 + len(udpPayload)), 0x00, 0x00} // sport=53 dport=1024
	udp = append(udp, udpPayload...)
	ip := []byte{0x45, 0x00, 0x00, byte(20 + len(udp)), 0, 0, 0, 0, 64, 17, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2}
	ip = append(ip, udp...)
	eth := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x08, 0x00} // dst+src mac + type IPv4
	eth = append(eth, ip...)

	info, ok := parseEthIPv4(eth)
	if !ok || info.proto != 17 || info.srcPort != 53 || info.dstIP != "10.0.0.2" {
		t.Fatalf("以太网/IPv4/UDP 解析错: %+v ok=%v", info, ok)
	}
	if len(info.payload) != len(udpPayload) || info.payload[0] != 0xDE {
		t.Errorf("UDP 载荷错: %v", info.payload)
	}
}
