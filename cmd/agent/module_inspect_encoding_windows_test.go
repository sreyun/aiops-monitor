//go:build windows

package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDecodeCmdOutGBKVersion(t *testing.T) {
	// GBK bytes for "版本" (common in `ver` on Chinese Windows).
	gbk := []byte{0xB0, 0xE6, 0xB1, 0xBE} // 版本
	got := decodeCmdOut(append([]byte("Microsoft Windows ["), append(gbk, []byte(" 10.0.26200]")...)...))
	if !utf8.ValidString(got) {
		t.Fatalf("invalid utf8 after decode: %q", got)
	}
	if !strings.Contains(got, "版本") {
		t.Fatalf("GBK 版本 not decoded: %q (hex=%x)", got, []byte(got))
	}
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("replacement char remains: %q", got)
	}
}
