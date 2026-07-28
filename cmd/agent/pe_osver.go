package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// peWindowsOSVersion reads IMAGE_OPTIONAL_HEADER.Major/MinorOperatingSystemVersion
// from a PE file. Used to reject Go ≥1.21 binaries (OS version 10.0) on
// Windows Server 2012 / Win8 before they replace a working agent.
func peWindowsOSVersion(path string) (maj, min uint16, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	var dos [64]byte
	if _, err := f.ReadAt(dos[:], 0); err != nil {
		return 0, 0, err
	}
	if dos[0] != 'M' || dos[1] != 'Z' {
		return 0, 0, fmt.Errorf("not a PE (missing MZ)")
	}
	eLfanew := int64(binary.LittleEndian.Uint32(dos[0x3C:]))
	var peSig [4]byte
	if _, err := f.ReadAt(peSig[:], eLfanew); err != nil {
		return 0, 0, err
	}
	if peSig[0] != 'P' || peSig[1] != 'E' || peSig[2] != 0 || peSig[3] != 0 {
		return 0, 0, fmt.Errorf("not a PE (missing PE signature)")
	}
	// COFF header is 20 bytes; Optional Header starts immediately after.
	optOff := eLfanew + 4 + 20
	var magicBuf [2]byte
	if _, err := f.ReadAt(magicBuf[:], optOff); err != nil {
		return 0, 0, err
	}
	magic := binary.LittleEndian.Uint16(magicBuf[:])
	if magic != 0x10b && magic != 0x20b { // PE32 / PE32+
		return 0, 0, fmt.Errorf("unsupported optional header magic 0x%x", magic)
	}
	// MajorOperatingSystemVersion is at offset 40 within the optional header.
	var osVer [4]byte
	if _, err := f.ReadAt(osVer[:], optOff+40); err != nil {
		return 0, 0, err
	}
	maj = binary.LittleEndian.Uint16(osVer[0:2])
	min = binary.LittleEndian.Uint16(osVer[2:4])
	return maj, min, nil
}

// peCompatibleWithHost is false when a Windows PE requires a newer OS than
// this kernel can run (classic Go ≥1.21 → MajorOperatingSystemVersion=10).
func peCompatibleWithHost(path string) error {
	if !windowsNeedsLegacyAgentBuild() {
		return nil
	}
	maj, min, err := peWindowsOSVersion(path)
	if err != nil {
		// Non-PE or unreadable: let later --version probe decide.
		return nil
	}
	if maj >= 10 {
		return fmt.Errorf("PE requires Windows %d.%d (host needs win2012/Go1.20 build)", maj, min)
	}
	return nil
}
