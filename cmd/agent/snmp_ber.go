package main

// SNMP 的 BER/ASN.1 编解码原语（手写）。
//
// 为什么不用标准库 encoding/asn1：asn1.Unmarshal 只认 UNIVERSAL 类标签并要求静态
// struct 映射，而 SNMP 用了一批 APPLICATION 类标签（Counter32=0x41 / Gauge32=0x42 /
// TimeTicks=0x43 / Counter64=0x46 / IpAddress=0x40），PDU 是 CONTEXT 构造类
// （GET=0xA0 … GetBulk=0xA5 … Trap=0xA7 … Report=0xA8），varbind 的 value 又是运行期
// 才定标签的异构 ANY；Counter64 需要完整 uint64（asn1 的 INTEGER 有符号会把它当负数）；
// v3 还要对报文做字节级重序列化再算 HMAC。这些 asn1 都表达不了，只能像 collector_netflow.go
// 用 encoding/binary 手撸 NetFlow 那样，手写 TLV。

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// BER 标签常量。
const (
	// Universal（primitive/constructed 已含在具体值里）
	tagInteger     = 0x02
	tagOctetString = 0x04
	tagNull        = 0x05
	tagOID         = 0x06
	tagSequence    = 0x30 // constructed

	// SNMP application-class（隐式 primitive）
	tagIPAddress = 0x40
	tagCounter32 = 0x41
	tagGauge32   = 0x42
	tagTimeTicks = 0x43
	tagOpaque    = 0x44
	tagCounter64 = 0x46

	// v2c 响应 varbind 的 value 例外（context primitive）
	tagNoSuchObject   = 0x80
	tagNoSuchInstance = 0x81
	tagEndOfMibView   = 0x82

	// PDU（context-specific constructed）
	pduGet     = 0xA0
	pduGetNext = 0xA1
	pduResponse = 0xA2
	pduSet     = 0xA3
	pduTrapV1  = 0xA4
	pduGetBulk = 0xA5
	pduInform  = 0xA6
	pduTrapV2  = 0xA7
	pduReport  = 0xA8
)

// ----------------------------------------------------------------------------
// 长度编解码
// ----------------------------------------------------------------------------

// encodeLength 编码 BER 长度：n≤127 用短式单字节；否则长式 0x80|字节数 + 大端最小长度。
func encodeLength(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n)}
	}
	var tmp []byte
	for n > 0 {
		tmp = append([]byte{byte(n & 0xff)}, tmp...)
		n >>= 8
	}
	return append([]byte{byte(0x80 | len(tmp))}, tmp...)
}

// decodeLength 解析 BER 长度，返回 (内容长度, 长度域自身占用的字节数, err)。
// 拒绝不定长（0x80 单独）——SNMP 只用定长。
func decodeLength(b []byte) (length, headerLen int, err error) {
	if len(b) == 0 {
		return 0, 0, errors.New("snmp ber: 缺少长度字节")
	}
	first := b[0]
	if first < 0x80 {
		return int(first), 1, nil
	}
	numBytes := int(first & 0x7f)
	if numBytes == 0 {
		return 0, 0, errors.New("snmp ber: 不支持不定长编码")
	}
	if numBytes > 4 || 1+numBytes > len(b) {
		return 0, 0, errors.New("snmp ber: 长度域过长")
	}
	n := 0
	for i := 0; i < numBytes; i++ {
		n = n<<8 | int(b[1+i])
	}
	if n < 0 {
		return 0, 0, errors.New("snmp ber: 长度溢出")
	}
	return n, 1 + numBytes, nil
}

// ----------------------------------------------------------------------------
// TLV 通用封装 / 解析
// ----------------------------------------------------------------------------

// encodeTLV 把 tag + 长度 + 内容拼成一个完整 TLV。
func encodeTLV(tag byte, content []byte) []byte {
	out := make([]byte, 0, 2+len(content))
	out = append(out, tag)
	out = append(out, encodeLength(len(content))...)
	out = append(out, content...)
	return out
}

// concat 顺序拼接多段已编码 TLV（用于组 SEQUENCE / PDU body）。
func concat(items ...[]byte) []byte {
	var out []byte
	for _, it := range items {
		out = append(out, it...)
	}
	return out
}

// readTLV 解一个 TLV：返回 (tag, 内容, 剩余字节, err)。校验内容长度不越界。
func readTLV(b []byte) (tag byte, content, rest []byte, err error) {
	if len(b) < 2 {
		return 0, nil, nil, errors.New("snmp ber: TLV 过短")
	}
	tag = b[0]
	length, hdr, err := decodeLength(b[1:])
	if err != nil {
		return 0, nil, nil, err
	}
	start := 1 + hdr
	end := start + length
	if end > len(b) {
		return 0, nil, nil, fmt.Errorf("snmp ber: TLV 长度 %d 超出缓冲 %d", length, len(b)-start)
	}
	return tag, b[start:end], b[end:], nil
}

// ----------------------------------------------------------------------------
// 整数（有符号）
// ----------------------------------------------------------------------------

// intContent 生成最小二进制补码内容字节（不含 tag/len）。
func intContent(v int64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(v))
	i := 0
	if v >= 0 {
		// 去前导 0x00，但保留一个使符号位为 0
		for i < 7 && buf[i] == 0x00 && buf[i+1]&0x80 == 0 {
			i++
		}
	} else {
		// 去前导 0xff，但保留一个使符号位为 1
		for i < 7 && buf[i] == 0xff && buf[i+1]&0x80 != 0 {
			i++
		}
	}
	return buf[i:]
}

// encodeInteger 编码有符号整数（request-id / error-status / error-index / version）。
func encodeInteger(v int64) []byte {
	return encodeTLV(tagInteger, intContent(v))
}

// decodeInteger 解码有符号整数（含符号扩展）。
func decodeInteger(content []byte) (int64, error) {
	if len(content) == 0 {
		return 0, errors.New("snmp ber: 空整数")
	}
	if len(content) > 8 {
		return 0, errors.New("snmp ber: 整数过长")
	}
	var v int64
	if content[0]&0x80 != 0 {
		v = -1 // 符号扩展
	}
	for _, b := range content {
		v = v<<8 | int64(b)
	}
	return v, nil
}

// ----------------------------------------------------------------------------
// 无符号（Counter32 / Gauge32 / TimeTicks / Counter64）
// ----------------------------------------------------------------------------

// uintContent 生成大端最小无符号内容；最高位为 1 时前置 0x00 保正。
func uintContent(v uint64) []byte {
	if v == 0 {
		return []byte{0x00}
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	i := 0
	for i < 7 && buf[i] == 0x00 {
		i++
	}
	content := buf[i:]
	if content[0]&0x80 != 0 {
		content = append([]byte{0x00}, content...)
	}
	return content
}

// encodeUnsigned 编码无符号值（Counter32/Gauge32/TimeTicks/Counter64，tag 由调用方给）。
func encodeUnsigned(tag byte, v uint64) []byte {
	return encodeTLV(tag, uintContent(v))
}

// decodeUnsigned 解码无符号值，忽略前导 0x00。最多接受 9 字节（8 字节 + 保正的前导 0）。
func decodeUnsigned(content []byte) (uint64, error) {
	if len(content) == 0 {
		return 0, nil
	}
	if len(content) > 9 {
		return 0, errors.New("snmp ber: 无符号过长")
	}
	var v uint64
	for _, b := range content {
		v = v<<8 | uint64(b)
	}
	return v, nil
}

// ----------------------------------------------------------------------------
// OctetString / Null
// ----------------------------------------------------------------------------

func encodeOctetString(b []byte) []byte { return encodeTLV(tagOctetString, b) }
func encodeNull() []byte                { return []byte{tagNull, 0x00} }

// ----------------------------------------------------------------------------
// OID
// ----------------------------------------------------------------------------

// encodeBase128 把一个 subid 编成 base-128（7bit/字节，除末字节外高位=1 续位）。
func encodeBase128(v uint32) []byte {
	if v == 0 {
		return []byte{0x00}
	}
	var tmp []byte
	for v > 0 {
		tmp = append([]byte{byte(v & 0x7f)}, tmp...)
		v >>= 7
	}
	for i := 0; i < len(tmp)-1; i++ {
		tmp[i] |= 0x80
	}
	return tmp
}

// encodeOIDValue 编码 OID 内容字节：首两弧合成 40*a+b（该合成值本身也可能 >127，须 base-128），
// 其余每个 subid 各自 base-128。
func encodeOIDValue(oid []uint32) []byte {
	if len(oid) < 2 {
		// 退化：非法 OID，尽量不 panic
		if len(oid) == 1 {
			return encodeBase128(40 * oid[0])
		}
		return []byte{0x00}
	}
	out := encodeBase128(40*oid[0] + oid[1])
	for _, sub := range oid[2:] {
		out = append(out, encodeBase128(sub)...)
	}
	return out
}

// encodeOID 编码完整 OID TLV。
func encodeOID(oid []uint32) []byte { return encodeTLV(tagOID, encodeOIDValue(oid)) }

// decodeOIDValue 解码 OID 内容字节为 []uint32，还原首两弧。
func decodeOIDValue(content []byte) ([]uint32, error) {
	if len(content) == 0 {
		return nil, errors.New("snmp ber: 空 OID")
	}
	var out []uint32
	var acc uint32
	started := false
	firstDone := false
	for _, b := range content {
		acc = acc<<7 | uint32(b&0x7f)
		started = true
		if b&0x80 == 0 { // 一个 subid 结束
			if !firstDone {
				a := acc / 40
				if a > 2 {
					a = 2
				}
				out = append(out, a, acc-40*a)
				firstDone = true
			} else {
				out = append(out, acc)
			}
			acc = 0
			started = false
		}
	}
	if started {
		return nil, errors.New("snmp ber: OID subid 截断")
	}
	return out, nil
}

// parseOID 把点分字符串（如 "1.3.6.1.2.1.1.1.0"）转成 []uint32。
func parseOID(s string) ([]uint32, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, ".")
	if s == "" {
		return nil, errors.New("snmp ber: 空 OID 字符串")
	}
	parts := strings.Split(s, ".")
	out := make([]uint32, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.ParseUint(p, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("snmp ber: OID 分量 %q 非法: %w", p, err)
		}
		out = append(out, uint32(n))
	}
	return out, nil
}

// oidToString 反向：[]uint32 → 点分字符串。
func oidToString(oid []uint32) string {
	if len(oid) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, v := range oid {
		if i > 0 {
			sb.WriteByte('.')
		}
		sb.WriteString(strconv.FormatUint(uint64(v), 10))
	}
	return sb.String()
}

// oidHasPrefix 判断 oid 是否以 prefix 开头（表遍历的列子树边界判断）。
func oidHasPrefix(oid, prefix []uint32) bool {
	if len(oid) < len(prefix) {
		return false
	}
	for i := range prefix {
		if oid[i] != prefix[i] {
			return false
		}
	}
	return true
}

// oidEqual 判断两个 OID 相等。
func oidEqual(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ----------------------------------------------------------------------------
// 值载体
// ----------------------------------------------------------------------------

// snmpValue 是一个解码后的 varbind value，按 Tag 取对应字段。
type snmpValue struct {
	Tag   byte
	Int   int64    // Integer
	Uint  uint64   // Counter32/Gauge32/TimeTicks/Counter64
	Bytes []byte   // OctetString/IpAddress/Opaque
	OID   []uint32 // OID value
	Exc   byte     // 0 或 tagNoSuchObject/tagNoSuchInstance/tagEndOfMibView
}

// decodeValue 按 tag 分派，把内容字节解成 snmpValue。
func decodeValue(tag byte, content []byte) (snmpValue, error) {
	v := snmpValue{Tag: tag}
	switch tag {
	case tagInteger:
		i, err := decodeInteger(content)
		if err != nil {
			return v, err
		}
		v.Int = i
	case tagCounter32, tagGauge32, tagTimeTicks, tagCounter64:
		u, err := decodeUnsigned(content)
		if err != nil {
			return v, err
		}
		v.Uint = u
	case tagOctetString, tagOpaque, tagIPAddress:
		v.Bytes = append([]byte(nil), content...)
	case tagOID:
		o, err := decodeOIDValue(content)
		if err != nil {
			return v, err
		}
		v.OID = o
	case tagNull:
		// 空值
	case tagNoSuchObject, tagNoSuchInstance, tagEndOfMibView:
		v.Exc = tag
	default:
		// 未知/未建模类型：保留原始字节，避免解析中断
		v.Bytes = append([]byte(nil), content...)
	}
	return v, nil
}

// Kind 返回值类型的人类可读名（trap varbind 的 Type 字段用）。
func (v snmpValue) Kind() string {
	switch v.Tag {
	case tagInteger:
		return "Integer"
	case tagCounter32:
		return "Counter32"
	case tagGauge32:
		return "Gauge32"
	case tagTimeTicks:
		return "TimeTicks"
	case tagCounter64:
		return "Counter64"
	case tagOctetString:
		return "OctetString"
	case tagIPAddress:
		return "IpAddress"
	case tagOpaque:
		return "Opaque"
	case tagOID:
		return "OID"
	case tagNull:
		return "Null"
	case tagNoSuchObject:
		return "noSuchObject"
	case tagNoSuchInstance:
		return "noSuchInstance"
	case tagEndOfMibView:
		return "endOfMibView"
	default:
		return fmt.Sprintf("tag_0x%02x", v.Tag)
	}
}

// String 返回值的人类可读表示（trap varbind 的 Value 字段 / 日志用）。
func (v snmpValue) String() string {
	switch v.Tag {
	case tagInteger:
		return strconv.FormatInt(v.Int, 10)
	case tagCounter32, tagGauge32, tagTimeTicks, tagCounter64:
		return strconv.FormatUint(v.Uint, 10)
	case tagIPAddress:
		if len(v.Bytes) == 4 {
			return fmt.Sprintf("%d.%d.%d.%d", v.Bytes[0], v.Bytes[1], v.Bytes[2], v.Bytes[3])
		}
		return fmt.Sprintf("%x", v.Bytes)
	case tagOctetString, tagOpaque:
		if isPrintable(v.Bytes) {
			return string(v.Bytes)
		}
		return "0x" + hexString(v.Bytes)
	case tagOID:
		return oidToString(v.OID)
	case tagNull:
		return ""
	case tagNoSuchObject, tagNoSuchInstance, tagEndOfMibView:
		return v.Kind()
	default:
		return "0x" + hexString(v.Bytes)
	}
}

// isPrintable 判断字节串是否可当作可读文本（含常见可打印 + 空白）。
func isPrintable(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	for _, c := range b {
		if c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

// hexString 把字节串转小写十六进制（不引入额外依赖）。
func hexString(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, hexdigits[c>>4], hexdigits[c&0x0f])
	}
	return string(out)
}
