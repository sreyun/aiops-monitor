package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExpandLogTargets 验证目录/文件自动识别：目录展开为其下日志文件、排除非日志、去重。
func TestExpandLogTargets(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x\n"), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("app.log")
	write("access.log.1") // 轮转文件
	write("service.out")
	write("readme.md") // 非日志
	single := write("single.log")

	got := expandLogTargets([]string{dir, single}) // 目录 + 显式文件（应去重）
	has := func(name string) bool {
		for _, g := range got {
			if filepath.Base(g) == name {
				return true
			}
		}
		return false
	}
	if !has("app.log") || !has("access.log.1") || !has("service.out") || !has("single.log") {
		t.Fatalf("目录下日志文件未全部采集: %v", got)
	}
	if has("readme.md") {
		t.Fatalf("非日志文件不应采集: %v", got)
	}
	count := 0
	for _, g := range got {
		if filepath.Base(g) == "single.log" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("single.log 应去重为 1 次，实际 %d", count)
	}
}

// TestSealLogAgentFormat 验证 agent 侧封装：密文不含明文、长度合理（nonce+密文）。
func TestSealLogAgentFormat(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	plain := []byte("2026-07-12 ERROR something secret happened")
	sealed, err := sealLogAgent(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	if len(sealed) <= 12 {
		t.Fatalf("密文过短: %d", len(sealed))
	}
	if string(sealed) == string(plain) {
		t.Fatal("未加密")
	}
	for _, w := range []string{"secret", "ERROR", "something"} {
		if containsBytes(sealed, w) {
			t.Fatalf("密文泄露明文片段 %q", w)
		}
	}
}

func containsBytes(hay []byte, needle string) bool {
	n := []byte(needle)
	for i := 0; i+len(n) <= len(hay); i++ {
		if string(hay[i:i+len(n)]) == needle {
			return true
		}
	}
	return false
}
