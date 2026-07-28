package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestPeWindowsOSVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.exe")
	// Minimal PE with OptionalHeader.MajorOperatingSystemVersion = 10.
	buf := make([]byte, 512)
	buf[0], buf[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(buf[0x3C:], 0x80) // e_lfanew
	copy(buf[0x80:], []byte{'P', 'E', 0, 0})
	// Optional header starts at 0x80+4+20 = 0x98
	opt := 0x98
	binary.LittleEndian.PutUint16(buf[opt:], 0x20b) // PE32+
	binary.LittleEndian.PutUint16(buf[opt+40:], 10) // Major OS
	binary.LittleEndian.PutUint16(buf[opt+42:], 0)  // Minor OS
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	maj, min, err := peWindowsOSVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if maj != 10 || min != 0 {
		t.Fatalf("got %d.%d", maj, min)
	}
}
