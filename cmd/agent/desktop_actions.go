package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// deskAdvancedInput is optional: platforms that can unlock / type credentials
// implement it. Others fall back to chord synthesis via deskInput.Key.
type deskAdvancedInput interface {
	SendCAD() error
	TypeText(text string) error
	DeskInputMeta() deskInputMeta
}

type deskInputMeta struct {
	Desktop        string `json:"desktop,omitempty"`
	InputDesktopOK bool   `json:"input_desktop_ok"`
	CAD            bool   `json:"cad"`
	TypeText       bool   `json:"type_text"`
	SecureDesktop  bool   `json:"secure_desktop,omitempty"`
	LockHint       string `json:"lock_hint,omitempty"`
}

// deskActionRequest is the JSON body of frame type 'A'.
type deskActionRequest struct {
	Action  string `json:"action"`            // cad | chord | type_text | wake
	Chord   string `json:"chord,omitempty"`   // win_l | ctrl_shift_esc | esc | ctrl_alt_bksp | enter | tab
	Text    string `json:"text,omitempty"`    // for type_text
	Enter   bool   `json:"enter,omitempty"`   // append Enter after type_text
	ScreenW int    `json:"screen_w,omitempty"`
	ScreenH int    `json:"screen_h,omitempty"`
}

func deskFeaturesFromInput(inp deskInput, viewOnly bool) map[string]bool {
	meta := deskInputMetaFrom(inp)
	return map[string]bool{
		"dnd": true, "monitors": true,
		"input":     !viewOnly,
		"cad":       meta.CAD && !viewOnly,
		"type_text": meta.TypeText && !viewOnly,
		"chords":    !viewOnly,
		"wake":      !viewOnly,
	}
}

func deskInputMetaFrom(inp deskInput) deskInputMeta {
	if adv, ok := inp.(deskAdvancedInput); ok {
		return adv.DeskInputMeta()
	}
	return deskInputMeta{
		InputDesktopOK: true,
		CAD:            false,
		TypeText:       true, // best-effort via Key/osascript/xdotool path
		LockHint:       deskDefaultLockHint(),
	}
}

func deskDefaultLockHint() string {
	switch deskGOOS() {
	case "windows":
		return "锁屏时请先点「Ctrl+Alt+Del」，再输入密码；需以 Windows 服务+桌面 worker 运行 Agent。"
	case "darwin":
		return "macOS 锁屏可能拦截键鼠（Secure Input）；可尝试「唤醒」与凭据发送，不保证登录窗口可解锁。"
	case "linux":
		return "锁屏界面通常可键入；若无图形会话或 greeter，需先在本机登录。"
	default:
		return ""
	}
}

type deskDesktopNamer interface {
	CurrentDesktop() string
}

func deskCurrentDesktop(cap deskCapture) string {
	if n, ok := cap.(deskDesktopNamer); ok {
		return n.CurrentDesktop()
	}
	return ""
}

func deskIsSecureName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n == "winlogon" || strings.HasPrefix(n, "winlogon") || n == "screensaver"
}

func deskLockHintForDesktop(name string) string {
	if deskIsSecureName(name) {
		return "当前输入桌面: " + name + "。锁屏/注销请先点「Ctrl+Alt+Del」，再点「解锁」输入密码（或点「唤醒」后直接键入）。"
	}
	if name != "" {
		return "当前输入桌面: " + name
	}
	return deskDefaultLockHint()
}

func deskMetaExtras(inp deskInput, viewOnly bool) map[string]any {
	m := deskInputMetaFrom(inp)
	out := map[string]any{
		"desktop":          m.Desktop,
		"input_desktop_ok": m.InputDesktopOK && !viewOnly,
		"lock_hint":        m.LockHint,
		"features":         deskFeaturesFromInput(inp, viewOnly),
	}
	if m.SecureDesktop {
		out["secure_desktop"] = true
	}
	return out
}

func handleDeskAction(inp deskInput, payload []byte, screenW, screenH int, fileTxChan chan<- []byte) {
	var req deskActionRequest
	if json.Unmarshal(payload, &req) != nil || req.Action == "" {
		return
	}
	if req.ScreenW <= 0 {
		req.ScreenW = screenW
	}
	if req.ScreenH <= 0 {
		req.ScreenH = screenH
	}
	var err error
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "cad":
		err = deskDoCAD(inp)
	case "chord":
		err = deskPlayChord(inp, req.Chord)
	case "type_text":
		err = deskDoTypeText(inp, req.Text, req.Enter)
	case "wake":
		err = deskDoWake(inp, req.ScreenW, req.ScreenH)
	default:
		err = fmt.Errorf("unknown action %q", req.Action)
	}
	ack := map[string]any{
		"action_ack": true,
		"action":     req.Action,
		"ok":         err == nil,
	}
	if err != nil {
		ack["error"] = err.Error()
	}
	for k, v := range deskMetaExtras(inp, false) {
		ack[k] = v
	}
	js, _ := json.Marshal(ack)
	frame := deskTxFrame('S', js)
	select {
	case fileTxChan <- frame:
	case <-time.After(3 * time.Second):
		// Still log — operator must not see a silent no-op on CAD/unlock.
	}
}

func deskDoCAD(inp deskInput) error {
	if adv, ok := inp.(deskAdvancedInput); ok {
		return adv.SendCAD()
	}
	return fmt.Errorf("Ctrl+Alt+Del 在此平台不可用（仅 Windows 服务模式支持 SendSAS）")
}

func deskDoTypeText(inp deskInput, text string, enter bool) error {
	if text == "" && !enter {
		return fmt.Errorf("empty text")
	}
	const maxLen = 4096
	if len(text) > maxLen {
		text = text[:maxLen]
	}
	if adv, ok := inp.(deskAdvancedInput); ok {
		if err := adv.TypeText(text); err != nil {
			return err
		}
	} else if text != "" {
		if err := deskTypeTextViaKeys(inp, text); err != nil {
			return err
		}
	}
	if enter {
		_ = inp.Key(0x0D, true)
		_ = inp.Key(0x0D, false)
	}
	return nil
}

func deskDoWake(inp deskInput, w, h int) error {
	if w < 2 {
		w = 1920
	}
	if h < 2 {
		h = 1080
	}
	cx, cy := w/2, h*2/3 // password field is usually lower-center on lock screens
	_ = inp.MouseMove(cx, cy)
	_ = inp.MouseButton(1, true)
	_ = inp.MouseButton(1, false)
	time.Sleep(40 * time.Millisecond)
	_ = inp.Key(0x1B, true) // Esc wake
	_ = inp.Key(0x1B, false)
	time.Sleep(40 * time.Millisecond)
	_ = inp.MouseMove(cx, cy)
	_ = inp.MouseButton(1, true)
	_ = inp.MouseButton(1, false)
	return nil
}

func deskPlayChord(inp deskInput, name string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	var keys []int
	switch name {
	case "win_l", "lock":
		keys = []int{0x5B, 0x4C} // LWin + L
	case "ctrl_shift_esc", "taskmgr":
		keys = []int{0x11, 0x10, 0x1B} // Ctrl+Shift+Esc
	case "esc", "escape":
		keys = []int{0x1B}
	case "ctrl_alt_bksp":
		keys = []int{0x11, 0x12, 0x08}
	case "enter":
		keys = []int{0x0D}
	case "tab":
		keys = []int{0x09}
	case "win":
		keys = []int{0x5B}
	default:
		return fmt.Errorf("unknown chord %q", name)
	}
	return deskTapKeys(inp, keys)
}

func deskTapKeys(inp deskInput, keys []int) error {
	for _, vk := range keys {
		if err := inp.Key(vk, true); err != nil {
			return err
		}
		time.Sleep(15 * time.Millisecond)
	}
	for i := len(keys) - 1; i >= 0; i-- {
		_ = inp.Key(keys[i], false)
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

// deskTypeTextViaKeys types ASCII via VK; non-ASCII runes are skipped (platform TypeText preferred).
func deskTypeTextViaKeys(inp deskInput, text string) error {
	for _, r := range text {
		if r == '\n' || r == '\r' {
			_ = inp.Key(0x0D, true)
			_ = inp.Key(0x0D, false)
			continue
		}
		if r == '\t' {
			_ = inp.Key(0x09, true)
			_ = inp.Key(0x09, false)
			continue
		}
		if r > 0 && r < 0x7f {
			vk := int(r)
			if vk >= 'a' && vk <= 'z' {
				vk -= 32
			}
			needShift := false
			if r >= 'A' && r <= 'Z' {
				needShift = true
			}
			if needShift {
				_ = inp.Key(0x10, true)
			}
			_ = inp.Key(vk, true)
			_ = inp.Key(vk, false)
			if needShift {
				_ = inp.Key(0x10, false)
			}
			time.Sleep(8 * time.Millisecond)
			continue
		}
		// Non-ASCII: try as unicode via platform if available — already handled by TypeText.
		return fmt.Errorf("non-ASCII text requires platform TypeText support")
	}
	return nil
}

// chordVKSequence returns the VK list for tests / documentation.
func chordVKSequence(name string) []int {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "win_l", "lock":
		return []int{0x5B, 0x4C}
	case "ctrl_shift_esc", "taskmgr":
		return []int{0x11, 0x10, 0x1B}
	case "esc", "escape":
		return []int{0x1B}
	case "ctrl_alt_bksp":
		return []int{0x11, 0x12, 0x08}
	case "enter":
		return []int{0x0D}
	case "tab":
		return []int{0x09}
	case "win":
		return []int{0x5B}
	default:
		return nil
	}
}
