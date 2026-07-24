package main

// Shared keyboard mapping for remote desktop.
// Browser sends KeyboardEvent.key + KeyboardEvent.code; we normalize to a
// Windows-style virtual-key code that each OS backend translates to native input.
// Keeping one table avoids the previous drift where Linux/macOS/Windows each
// dropped different keys (punctuation, F-keys, Meta, numpad).

func deskKeyToVK(key, code string) int {
	if vk := deskCodeToVK(code); vk != 0 {
		return vk
	}
	if len(key) == 1 {
		r := key[0]
		if r >= 'a' && r <= 'z' {
			return int(r - 32)
		}
		if r >= 'A' && r <= 'Z' {
			return int(r)
		}
		if r >= '0' && r <= '9' {
			return int(r)
		}
	}
	return 0
}

func deskCodeToVK(code string) int {
	switch code {
	case "Backspace":
		return 0x08
	case "Tab":
		return 0x09
	case "Enter", "NumpadEnter":
		return 0x0D
	case "Escape":
		return 0x1B
	case "Space":
		return 0x20
	case "PageUp":
		return 0x21
	case "PageDown":
		return 0x22
	case "End":
		return 0x23
	case "Home":
		return 0x24
	case "ArrowLeft":
		return 0x25
	case "ArrowUp":
		return 0x26
	case "ArrowRight":
		return 0x27
	case "ArrowDown":
		return 0x28
	case "Insert":
		return 0x2D
	case "Delete":
		return 0x2E
	case "ShiftLeft":
		return 0x10 // VK_SHIFT (generic / left)
	case "ShiftRight":
		return 0xA1 // VK_RSHIFT
	case "ControlLeft":
		return 0x11 // VK_CONTROL
	case "ControlRight":
		return 0xA3 // VK_RCONTROL
	case "AltLeft":
		return 0x12 // VK_MENU
	case "AltRight":
		return 0xA5 // VK_RMENU
	case "MetaLeft", "OSLeft":
		return 0x5B // VK_LWIN / Super
	case "MetaRight", "OSRight":
		return 0x5C // VK_RWIN
	case "ContextMenu":
		return 0x5D
	case "CapsLock":
		return 0x14
	case "NumLock":
		return 0x90
	case "ScrollLock":
		return 0x91
	case "Pause":
		return 0x13
	case "PrintScreen":
		return 0x2C

	case "Minus":
		return 0xBD
	case "Equal":
		return 0xBB
	case "BracketLeft":
		return 0xDB
	case "BracketRight":
		return 0xDD
	case "Backslash":
		return 0xDC
	case "Semicolon":
		return 0xBA
	case "Quote":
		return 0xDE
	case "Backquote":
		return 0xC0
	case "Comma":
		return 0xBC
	case "Period":
		return 0xBE
	case "Slash":
		return 0xBF
	case "IntlBackslash":
		return 0xE2

	case "F1":
		return 0x70
	case "F2":
		return 0x71
	case "F3":
		return 0x72
	case "F4":
		return 0x73
	case "F5":
		return 0x74
	case "F6":
		return 0x75
	case "F7":
		return 0x76
	case "F8":
		return 0x77
	case "F9":
		return 0x78
	case "F10":
		return 0x79
	case "F11":
		return 0x7A
	case "F12":
		return 0x7B

	case "Numpad0":
		return 0x60
	case "Numpad1":
		return 0x61
	case "Numpad2":
		return 0x62
	case "Numpad3":
		return 0x63
	case "Numpad4":
		return 0x64
	case "Numpad5":
		return 0x65
	case "Numpad6":
		return 0x66
	case "Numpad7":
		return 0x67
	case "Numpad8":
		return 0x68
	case "Numpad9":
		return 0x69
	case "NumpadMultiply":
		return 0x6A
	case "NumpadAdd":
		return 0x6B
	case "NumpadSubtract":
		return 0x6D
	case "NumpadDecimal":
		return 0x6E
	case "NumpadDivide":
		return 0x6F
	}

	if len(code) == 4 && code[:3] == "Key" {
		c := code[3]
		if c >= 'A' && c <= 'Z' {
			return int(c)
		}
	}
	if len(code) == 6 && code[:5] == "Digit" {
		d := code[5]
		if d >= '0' && d <= '9' {
			return int(d)
		}
	}
	return 0
}

// deskVKExtended reports whether a VK needs KEYEVENTF_EXTENDEDKEY on Windows
// (and helps Linux/macOS pick the right keysym where relevant).
func deskVKExtended(vk int) bool {
	switch vk {
	case 0x21, 0x22, 0x23, 0x24, // PgUp PgDn End Home
		0x25, 0x26, 0x27, 0x28, // arrows
		0x2D, 0x2E, // Insert Delete
		0x5B, 0x5C, 0x5D, // Win / Apps
		0xA3, 0xA5, // RCONTROL / RMENU
		0x6F: // NumpadDivide
		return true
	}
	return false
}
