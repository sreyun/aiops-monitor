package main

import (
	"bytes"
	"testing"
)

// BER 编解码的成败标准：编码→解码必须无损往返，且长度/OID/整数三处易错点全覆盖。

func TestEncodeDecodeLength(t *testing.T) {
	cases := []int{0, 1, 127, 128, 255, 256, 300, 65535, 65536, 1 << 20}
	for _, n := range cases {
		enc := encodeLength(n)
		got, hdr, err := decodeLength(enc)
		if err != nil {
			t.Fatalf("decodeLength(%d) 编码=%x 出错: %v", n, enc, err)
		}
		if got != n {
			t.Errorf("长度往返 %d → %d", n, got)
		}
		if hdr != len(enc) {
			t.Errorf("长度 %d 头长 %d != 编码长 %d", n, hdr, len(enc))
		}
	}
	// 短式必须单字节
	if len(encodeLength(127)) != 1 {
		t.Error("127 应为短式单字节")
	}
	// 128 必须长式 0x81 0x80
	if enc := encodeLength(128); len(enc) != 2 || enc[0] != 0x81 || enc[1] != 0x80 {
		t.Errorf("128 长式编码错: %x", enc)
	}
}

func TestOIDRoundTrip(t *testing.T) {
	cases := []string{
		"1.3.6.1.2.1.1.1.0",              // sysDescr
		"1.3.6.1.2.1.31.1.1.1.6",         // ifHCInOctets
		"1.3.6.1.6.3.1.1.5.3",            // linkDown
		"1.3.6.1.4.1.9999.128.200.16384", // 含 >127 大 subid（多字节 base-128）
		"0.0",                            // 边界
		"2.999",                          // 首弧合成值 40*2+999=1079 >127，须 base-128
	}
	for _, s := range cases {
		oid, err := parseOID(s)
		if err != nil {
			t.Fatalf("parseOID(%q): %v", s, err)
		}
		enc := encodeOIDValue(oid)
		dec, err := decodeOIDValue(enc)
		if err != nil {
			t.Fatalf("decodeOIDValue(%q) 编码=%x: %v", s, enc, err)
		}
		if oidToString(dec) != s {
			t.Errorf("OID 往返 %q → %q (编码 %x)", s, oidToString(dec), enc)
		}
	}
}

func TestOIDEncodePrefixExample(t *testing.T) {
	// 1.3.6.1 → 0x2b 06 01（首两弧 40*1+3=43=0x2b）
	oid, _ := parseOID("1.3.6.1")
	if enc := encodeOIDValue(oid); !bytes.Equal(enc, []byte{0x2b, 0x06, 0x01}) {
		t.Errorf("1.3.6.1 编码错: %x", enc)
	}
}

func TestEncodeDecodeInteger(t *testing.T) {
	cases := []int64{0, 1, 127, 128, 200, 255, 256, -1, -128, 32767, -32768, 1 << 30, -(1 << 30)}
	for _, v := range cases {
		enc := encodeInteger(v)
		tag, content, rest, err := readTLV(enc)
		if err != nil || tag != tagInteger || len(rest) != 0 {
			t.Fatalf("readTLV(int %d) 失败: tag=%x err=%v", v, tag, err)
		}
		got, err := decodeInteger(content)
		if err != nil {
			t.Fatalf("decodeInteger(%d): %v", v, err)
		}
		if got != v {
			t.Errorf("整数往返 %d → %d (内容 %x)", v, got, content)
		}
	}
}

func TestEncodeDecodeUnsigned(t *testing.T) {
	cases := []uint64{0, 1, 127, 128, 255, 256, 0x7fffffff, 0x80000000, 0xffffffff,
		0x100000000, 0xffffffffffffffff} // 含 Counter64 极值
	for _, v := range cases {
		enc := encodeUnsigned(tagCounter64, v)
		tag, content, _, err := readTLV(enc)
		if err != nil || tag != tagCounter64 {
			t.Fatalf("readTLV(uint %d) 失败: %v", v, err)
		}
		got, err := decodeUnsigned(content)
		if err != nil {
			t.Fatalf("decodeUnsigned(%d): %v", v, err)
		}
		if got != v {
			t.Errorf("无符号往返 %d → %d (内容 %x)", v, got, content)
		}
	}
	// 最高位为 1 时必须前置 0x00 保正：0x80000000 内容应为 5 字节 00 80 00 00 00
	enc := uintContent(0x80000000)
	if !bytes.Equal(enc, []byte{0x00, 0x80, 0x00, 0x00, 0x00}) {
		t.Errorf("0x80000000 保正编码错: %x", enc)
	}
}

func TestReadTLVNested(t *testing.T) {
	// 组一个 SEQUENCE{ Integer(5), OctetString("hi") } 再解回来
	inner := concat(encodeInteger(5), encodeOctetString([]byte("hi")))
	seq := encodeTLV(tagSequence, inner)

	tag, content, rest, err := readTLV(seq)
	if err != nil || tag != tagSequence || len(rest) != 0 {
		t.Fatalf("读 SEQUENCE 失败: tag=%x err=%v", tag, err)
	}
	tag1, c1, r1, err := readTLV(content)
	if err != nil || tag1 != tagInteger {
		t.Fatalf("读第一元素失败: %v", err)
	}
	if i, _ := decodeInteger(c1); i != 5 {
		t.Errorf("第一元素 = %d, 期望 5", i)
	}
	tag2, c2, _, err := readTLV(r1)
	if err != nil || tag2 != tagOctetString {
		t.Fatalf("读第二元素失败: %v", err)
	}
	if string(c2) != "hi" {
		t.Errorf("第二元素 = %q, 期望 hi", string(c2))
	}
}

func TestDecodeValue(t *testing.T) {
	// Counter32
	_, c, _, _ := readTLV(encodeUnsigned(tagCounter32, 123456))
	if v, _ := decodeValue(tagCounter32, c); v.Uint != 123456 || v.Kind() != "Counter32" {
		t.Errorf("Counter32 解码错: %+v", v)
	}
	// TimeTicks 的 String
	_, c, _, _ = readTLV(encodeUnsigned(tagTimeTicks, 4200))
	if v, _ := decodeValue(tagTimeTicks, c); v.String() != "4200" {
		t.Errorf("TimeTicks String 错: %s", v.String())
	}
	// IpAddress
	v, _ := decodeValue(tagIPAddress, []byte{10, 0, 0, 1})
	if v.String() != "10.0.0.1" {
		t.Errorf("IpAddress String 错: %s", v.String())
	}
	// OctetString 可读
	v, _ = decodeValue(tagOctetString, []byte("GigabitEthernet0/1"))
	if v.String() != "GigabitEthernet0/1" {
		t.Errorf("OctetString String 错: %s", v.String())
	}
	// 不可读字节 → 0x 前缀
	v, _ = decodeValue(tagOctetString, []byte{0x00, 0x1b, 0xff})
	if v.String() != "0x001bff" {
		t.Errorf("二进制 OctetString String 错: %s", v.String())
	}
}
