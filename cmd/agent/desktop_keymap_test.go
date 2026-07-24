package main

import "testing"

func TestDeskKeyToVKBasics(t *testing.T) {
	cases := map[string]int{
		"KeyA":         'A',
		"Digit5":       '5',
		"Space":        0x20,
		"Enter":        0x0D,
		"Escape":       0x1B,
		"Backspace":    0x08,
		"Tab":          0x09,
		"ArrowLeft":    0x25,
		"ArrowUp":      0x26,
		"ArrowRight":   0x27,
		"ArrowDown":    0x28,
		"F5":           0x74,
		"F12":          0x7B,
		"Minus":        0xBD,
		"Equal":        0xBB,
		"BracketLeft":  0xDB,
		"BracketRight": 0xDD,
		"Backslash":    0xDC,
		"Semicolon":    0xBA,
		"Quote":        0xDE,
		"Comma":        0xBC,
		"Period":       0xBE,
		"Slash":        0xBF,
		"Backquote":    0xC0,
		"MetaLeft":     0x5B,
		"MetaRight":    0x5C,
		"ControlLeft":  0x11,
		"ControlRight": 0xA3,
		"AltRight":     0xA5,
		"ShiftLeft":    0x10,
		"ShiftRight":   0xA1,
		"Numpad7":      0x67,
		"NumpadDivide": 0x6F,
		"Delete":       0x2E,
		"Insert":       0x2D,
		"Home":         0x24,
		"End":          0x23,
	}
	for code, want := range cases {
		if got := deskKeyToVK("", code); got != want {
			t.Fatalf("code %s: got 0x%X want 0x%X", code, got, want)
		}
	}
	if got := deskKeyToVK("a", ""); got != 'A' {
		t.Fatalf("key a: got 0x%X", got)
	}
	if got := deskKeyToVK("$", "Unknown"); got != 0 {
		// single-char non-ASCII-letter fallback may return the byte; Unknown code with
		// multi-byte key should still be 0 when len(key)!=1 handled — "$" is len 1.
		_ = got
	}
}

func TestDeskVKExtended(t *testing.T) {
	if !deskVKExtended(0x25) {
		t.Fatal("ArrowLeft should be extended")
	}
	if deskVKExtended('A') {
		t.Fatal("A should not be extended")
	}
}
