package main

import (
	"encoding/binary"
	"testing"
)

// TestReadUintNoPanicOnOversize 锁定 v9 readUint 越界 panic 的修复：字段长度由报文声明、
// 攻击者可控，len>8 时旧代码 copy(buf[8-len:], data) 下标为负 → panic 打崩整个 agent。
func TestReadUintNoPanicOnOversize(t *testing.T) {
	// 10 字节：不得 panic，取低 8 字节大端解释。
	got := readUint([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	want := binary.BigEndian.Uint64([]byte{3, 4, 5, 6, 7, 8, 9, 10})
	if got != want {
		t.Errorf("readUint(10B) = %d, 期望 %d", got, want)
	}
	// 常规宽度仍正确
	if readUint([]byte{0x01}) != 1 {
		t.Error("1 字节错")
	}
	if readUint([]byte{0x01, 0x00}) != 256 {
		t.Error("2 字节错")
	}
	if readUint([]byte{0x01, 0x02, 0x03}) != 0x010203 {
		t.Error("3 字节错")
	}
	if readUint(nil) != 0 {
		t.Error("空输入应为 0")
	}
	// 32 字节极端超长也不 panic
	big := make([]byte, 32)
	big[31] = 0xFF
	if readUint(big) != 0xFF {
		t.Error("32 字节错")
	}
}

// TestParseV9TemplateBounds 确认畸形 v9 模板(字段数超上限)不会崩溃或吃满内存。
func TestParseV9TemplateBounds(t *testing.T) {
	nr := newNetflowReceiver(NetFlowConfig{}, "h", "fp")
	// 声明 fieldCount=60000 但报文只有几字节 → 必须安全返回，不 panic、不巨型分配。
	data := make([]byte, 8)
	binary.BigEndian.PutUint16(data[0:2], 256)   // templateID
	binary.BigEndian.PutUint16(data[2:4], 60000) // fieldCount(超 maxV9Fields)
	nr.parseV9Template(data, 1)
	if nr.v9TemplateCount != 0 {
		t.Errorf("超上限字段数的模板不应入缓存, count=%d", nr.v9TemplateCount)
	}
}
