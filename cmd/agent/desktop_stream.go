package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Agent-side web desktop channel (screen stream + input + file xfer).
// Mirrors terminal reverse channel: wait → rx + tx.

const deskSessionIdle = 2 * time.Hour
const deskSessionHard = 8 * time.Hour

type deskCapture interface {
	Size() (w, h int)
	Capture() (image.Image, error)
	Close() error
	Monitors() []deskMonitorInfo
	SetMonitor(id int) error
}

// Optional: capture reports its current monitor origin in virtual-screen coords
// so input can convert image-local clicks to absolute SetCursorPos targets.
type deskOriginAware interface {
	Origin() (x, y int)
}
type deskOriginSink interface {
	SetOrigin(x, y int)
}

func syncDeskOrigin(cap deskCapture, inp deskInput) {
	g, okG := cap.(deskOriginAware)
	s, okS := inp.(deskOriginSink)
	if okG && okS {
		s.SetOrigin(g.Origin())
	}
}

type deskMonitorInfo struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Primary bool   `json:"primary"`
}

type deskInput interface {
	MouseMove(x, y int) error
	MouseButton(button int, down bool) error
	MouseWheel(delta int) error
	Key(vk int, down bool) error
	Close() error
}

type deskQuality struct {
	Scale   float64 `json:"scale"`   // 0.25–1.0
	Quality int     `json:"quality"` // JPEG 1–100
	FPS     int     `json:"fps"`     // 1–15
	Codec   string  `json:"codec"`   // jpeg | h264
	Monitor int     `json:"monitor"` // display id
}

func defaultDeskQuality() deskQuality {
	return deskQuality{Scale: 0.5, Quality: 55, FPS: 8, Codec: "jpeg"}
}

func (a *Agent) runDesktopChannelFor(t *serverTarget) {
	if a.identity.Fingerprint == "" {
		slog.Warn("远程桌面通道未启用：未采集到机器指纹", "server", t.server)
		return
	}
	slog.Info("远程桌面通道已就绪，等待服务端呼叫…", "server", t.server)
	backoff := newBackoffTimer(1*time.Second, 60*time.Second)
	for {
		// Desktop workers don't register; the service may reconcile the canonical
		// host id after we started. Re-read before every wait so we never sit on
		// a stale id for the process lifetime.
		if a.stateFile != "" {
			if id := readHostIDFromState(a.stateFile); id != "" && id != a.identity.HostID {
				slog.Info("桌面通道刷新 HostID", "old", short(a.identity.HostID), "new", short(id))
				a.identity.HostID = id
			}
		}
		sid, lang, ok := a.deskWait(t.server)
		if !ok {
			d := backoff.next()
			time.Sleep(d)
			continue
		}
		backoff.reset()
		if sid == "" {
			continue
		}
		go a.runDesktopSession(t.server, sid, lang)
	}
}

func (a *Agent) deskWait(server string) (sessionID, lang string, ok bool) {
	q := url.Values{"host": {a.identity.HostID}}
	resp, err := agentGet(termWaitHTTP, server+"/api/v1/agent/desktop/wait?"+q.Encode(), a.identity.Fingerprint)
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", false
	}
	var out struct {
		Session string `json:"session"`
		Lang    string `json:"lang"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Session, out.Lang, true
}

func deskTxFrame(typ byte, payload []byte) []byte {
	b := make([]byte, 5+len(payload))
	b[0] = typ
	binary.BigEndian.PutUint32(b[1:], uint32(len(payload)))
	copy(b[5:], payload)
	return b
}

// streamTxHTTP is a dedicated client for the *continuous* agent→server upload
// streams (desktop/terminal tx). The shared termHTTP buffers request-body writes
// in a 4KB bufio.Writer that only flushes when full — fine for report POSTs and
// for write-then-close streams (exec output, deskSendError), but fatal for a
// long-lived stream of small frames: the first meta ('S' ~500B) and low-detail
// JPEG/H264 frames sat in the buffer forever, so the browser reached "agent已接入"
// (tx headers arrived) but never got a single frame. WriteBufferSize=1 makes
// every frame (≥5B header) exceed the buffer and go straight to the socket.
var (
	streamTxOnce sync.Once
	streamTxHTTP *http.Client
)

func deskStreamClient() *http.Client {
	streamTxOnce.Do(func() {
		var tr *http.Transport
		if base, ok := termHTTP.Transport.(*http.Transport); ok && base != nil {
			tr = base.Clone() // inherit TLS/CA/proxy config applied at startup
		} else {
			tr = &http.Transport{}
		}
		tr.WriteBufferSize = 1
		streamTxHTTP = &http.Client{Transport: tr}
	})
	return streamTxHTTP
}

func (a *Agent) runDesktopSession(server, sid, lang string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("桌面会话异常已恢复", "session", sid, "panic", r)
		}
	}()

	cap, err := openDeskCapture()
	if err != nil {
		slog.Warn("桌面抓屏不可用", "session", sid, "err", err)
		a.deskSendError(server, sid, err.Error())
		return
	}
	defer cap.Close()

	inp, err := openDeskInput()
	viewOnly := false
	if err != nil {
		slog.Warn("桌面键鼠注入不可用，将以只读画面模式继续", "session", sid, "err", err)
		inp = &noopDeskInput{}
		viewOnly = true
	}
	defer inp.Close()

	slog.Info("远程桌面会话开始", "session", sid)
	var once sync.Once
	var stop atomic.Bool
	closeAll := func() {
		once.Do(func() {
			stop.Store(true)
			_ = cap.Close()
			_ = inp.Close()
		})
	}
	defer closeAll()

	q := defaultDeskQuality()
	var qMu sync.Mutex
	fileTxChan := make(chan []byte, 512)
	lastActive := time.Now()
	var actMu sync.Mutex
	touch := func() {
		actMu.Lock()
		lastActive = time.Now()
		actMu.Unlock()
	}

	hardTimer := time.AfterFunc(deskSessionHard, closeAll)
	defer hardTimer.Stop()
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for !stop.Load() {
			<-t.C
			actMu.Lock()
			idle := time.Since(lastActive)
			actMu.Unlock()
			if idle > deskSessionIdle {
				closeAll()
				return
			}
		}
	}()

	pr, pw := io.Pipe()
	var pwMu sync.Mutex
	writeTx := func(b []byte) error {
		pwMu.Lock()
		defer pwMu.Unlock()
		_, err := pw.Write(b)
		return err
	}
	reqDone := make(chan error, 1)
	go func() {
		req, err := http.NewRequest("POST", server+"/api/v1/agent/desktop/tx?session="+sid, pr)
		if err != nil {
			pw.Close()
			reqDone <- err
			return
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("X-Agent-Fingerprint", a.identity.Fingerprint)
		resp, doErr := deskStreamClient().Do(req)
		if doErr == nil {
			resp.Body.Close()
		}
		reqDone <- doErr
	}()

	sw, sh := cap.Size()
	mons := cap.Monitors()
	h264OK := deskH264Usable()
	codecs := []string{"jpeg"}
	if h264OK {
		codecs = append(codecs, "h264")
	}
	// prefer hints the browser to auto-select a codec. On macOS per-frame
	// screencapture is very slow, so continuous H.264 (avfoundation) is strongly
	// preferred when the screen-capture device is resolvable.
	prefer := deskPreferredCodec()
	meta, _ := json.Marshal(map[string]any{
		"w": sw, "h": sh, "os": runtimeGOOS(),
		"scale": q.Scale, "quality": q.Quality, "fps": q.FPS,
		"codec": q.Codec, "codecs": codecs, "prefer": prefer,
		"h264": h264OK, "clipboard": true, "monitors": mons,
		"view_only": viewOnly,
		"features": map[string]bool{"dnd": true, "clipboard": true, "monitors": true, "h264": h264OK, "input": !viewOnly},
	})
	if err := writeTx(deskTxFrame('S', meta)); err != nil {
		pw.Close()
		<-reqDone
		return
	}
	if viewOnly {
		warn, _ := json.Marshal(map[string]string{
			"error": "键鼠注入不可用，当前为只读画面（Linux 需安装 xdotool；macOS 需辅助功能权限）",
			"level": "warn",
		})
		_ = writeTx(deskTxFrame('E', warn))
	}

	var h264Mu sync.Mutex
	var h264 *h264Pipe
	stopH264 := func() {
		h264Mu.Lock()
		if h264 != nil {
			_ = h264.Close()
			h264 = nil
		}
		h264Mu.Unlock()
	}
	defer stopH264()

	// currentMon is read by the encoder goroutine and written by applyMonitor
	// (rx goroutine); guard it to avoid a data race on monitor switch.
	var monMu sync.Mutex
	currentMon := deskMonitorInfo{ID: 1, Width: sw, Height: sh, Primary: true}
	if len(mons) > 0 {
		currentMon = mons[0]
		for _, m := range mons {
			if m.Primary {
				currentMon = m
				break
			}
		}
	}

	// Encoder / capture loop (JPEG or H264)
	go func() {
		defer closeAll()
		defer pw.Close()
		// Desktop switches (lock↔unlock, UAC secure desktop, fast-user-switch)
		// make GDI BitBlt transiently fail for a frame or two while the worker
		// re-attaches to the new input desktop. Tearing the session down on the
		// first error would surface as a spurious "已断开". Tolerate a short burst
		// of consecutive failures (~4s) before giving up.
		capFails := 0
		const maxCapFails = 60
		// Blank-frame diagnostic: if capture SUCCEEDS but every frame is pure black
		// (a non-rendering target desktop — headless host, nobody logged in, or a
		// disconnected console), warn the operator ONCE with actionable guidance
		// instead of leaving them staring at an unexplained black screen.
		blankFrames := 0
		blankWarned := false
		const blankWarnAt = 40
		for !stop.Load() {
			qMu.Lock()
			cq := q
			qMu.Unlock()
			fps := cq.FPS
			if fps < 1 {
				fps = 1
			}
			if fps > 15 {
				fps = 15
			}
			interval := time.Second / time.Duration(fps)
			codec := cq.Codec
			if codec == "h264" && !h264OK {
				codec = "jpeg"
			}

			if codec == "h264" {
				h264Mu.Lock()
				needStart := h264 == nil
				h264Mu.Unlock()
				if needStart {
					stopH264()
					monMu.Lock()
					mon := currentMon
					monMu.Unlock()
					p, err := startH264Pipe(mon, cq.Scale, fps)
					if err != nil {
						codec = "jpeg"
					} else {
						h264Mu.Lock()
						h264 = p
						h264Mu.Unlock()
						// Each reader owns its buffer — a shared buffer raced when the
						// codec/monitor switched and two readers briefly overlapped.
						go func(pipe *h264Pipe) {
							rbuf := make([]byte, 64*1024)
							for !stop.Load() {
								n, err := pipe.Read(rbuf)
								if n > 0 {
									chunk := make([]byte, n)
									copy(chunk, rbuf[:n])
									if writeTx(deskTxFrame('H', chunk)) != nil {
										return
									}
								}
								if err != nil {
									// ffmpeg exited/crashed — clear the pipe so the next
									// loop iteration restarts it (or falls back to JPEG).
									stopH264()
									return
								}
							}
						}(p)
					}
				}
				if codec == "h264" {
					time.Sleep(interval)
					continue
				}
			} else {
				stopH264()
			}

			img, err := cap.Capture()
			if err != nil {
				capFails++
				if capFails < maxCapFails {
					// Likely a desktop switch in progress; the next Capture()
					// re-attaches to the input desktop. Back off briefly and retry
					// instead of dropping the whole session.
					time.Sleep(interval)
					continue
				}
				msg, _ := json.Marshal(map[string]string{"error": err.Error()})
				_ = writeTx(deskTxFrame('E', msg))
				return
			}
			capFails = 0
			// Keep mouse mapping in sync when RDP/DPI resizes the desktop mid-session.
			if nw, nh := cap.Size(); nw > 0 && nh > 0 && (nw != sw || nh != sh) {
				sw, sh = nw, nh
				js, _ := json.Marshal(map[string]any{"w": sw, "h": sh, "monitors": cap.Monitors()})
				_ = writeTx(deskTxFrame('S', js))
			}
			if !blankWarned {
				if isLikelyBlank(img) {
					if blankFrames++; blankFrames >= blankWarnAt {
						blankWarned = true
						msg, _ := json.Marshal(map[string]string{
							"error": "画面为纯黑：目标会话未渲染桌面（无显示器的服务器 / 无人登录 / 控制台会话已断开）。请通过 RDP 登录该主机后重试，或为无头服务器安装虚拟显示驱动。 (Captured desktop is all black: the target session isn't rendering — headless host, nobody logged in, or a disconnected console. Log in via RDP and retry, or install a virtual display driver.)",
							"level": "warn",
						})
						_ = writeTx(deskTxFrame('E', msg))
					}
				} else {
					blankFrames = 0
				}
			}
			scaled := scaleImage(img, cq.Scale)
			var jbuf bytes.Buffer
			qual := cq.Quality
			if qual < 20 {
				qual = 20
			}
			if qual > 95 {
				qual = 95
			}
			if err := jpeg.Encode(&jbuf, scaled, &jpeg.Options{Quality: qual}); err != nil {
				time.Sleep(interval)
				continue
			}
			jpegBytes := jbuf.Bytes()
			if len(jpegBytes) > 4<<20 {
				time.Sleep(interval)
				continue
			}
			if err := writeTx(deskTxFrame('K', jpegBytes)); err != nil {
				return
			}
			time.Sleep(interval)
		}
	}()

	go func() {
		for !stop.Load() {
			select {
			case fr := <-fileTxChan:
				if err := writeTx(fr); err != nil {
					return
				}
			case <-time.After(200 * time.Millisecond):
			}
		}
	}()

	// Periodic clipboard push (agent → browser), every 2s when text changes
	go func() {
		var last string
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for !stop.Load() {
			<-t.C
			txt, err := deskClipboardGet()
			if err != nil || txt == last || txt == "" {
				continue
			}
			last = txt
			js, _ := json.Marshal(map[string]string{"text": txt, "dir": "to_browser"})
			_ = writeTx(deskTxFrame('C', js))
		}
	}()

	applyMonitor := func(id int) {
		if id <= 0 {
			return
		}
		_ = cap.SetMonitor(id)
		syncDeskOrigin(cap, inp)
		for _, m := range cap.Monitors() {
			if m.ID == id {
				monMu.Lock()
				currentMon = m
				monMu.Unlock()
				sw, sh = m.Width, m.Height
				break
			}
		}
		stopH264()
		js, _ := json.Marshal(map[string]any{"w": sw, "h": sh, "monitors": cap.Monitors(), "monitor": id})
		_ = writeTx(deskTxFrame('S', js))
	}
	syncDeskOrigin(cap, inp)

	// rx: input + files + clipboard + monitor
	go func() {
		defer closeAll()
		resp, err := agentGet(termHTTP, server+"/api/v1/agent/desktop/rx?session="+sid, a.identity.Fingerprint)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		dr := newDeadlineReader(resp.Body, 90*time.Second)
		readDeskFrames(dr, inp, lang, &q, &qMu, touch, fileTxChan, &sw, &sh, applyMonitor)
	}()

	<-reqDone
	closeAll()
	slog.Info("远程桌面会话结束", "session", sid)
}

// isLikelyBlank reports whether a captured frame is entirely (near) black. On
// Windows a successful BitBlt of a non-rendering desktop returns pure #000000, so
// an all-black frame indicates a dead/headless target session rather than a
// legitimately dark screen (which almost always has a non-black taskbar/wallpaper).
// Samples a sparse grid so the check is cheap.
func isLikelyBlank(img image.Image) bool {
	b := img.Bounds()
	if b.Dx() < 8 || b.Dy() < 8 {
		return false
	}
	const steps = 32
	sx := b.Dx() / steps
	sy := b.Dy() / steps
	if sx < 1 {
		sx = 1
	}
	if sy < 1 {
		sy = 1
	}
	for y := b.Min.Y; y < b.Max.Y; y += sy {
		for x := b.Min.X; x < b.Max.X; x += sx {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r>>8 > 8 || g>>8 > 8 || bl>>8 > 8 {
				return false
			}
		}
	}
	return true
}

func (a *Agent) deskSendError(server, sid, msg string) {
	pr, pw := io.Pipe()
	go func() {
		js, _ := json.Marshal(map[string]string{"error": msg})
		_, _ = pw.Write(deskTxFrame('E', js))
		_ = pw.Close()
	}()
	req, err := http.NewRequest("POST", server+"/api/v1/agent/desktop/tx?session="+sid, pr)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Agent-Fingerprint", a.identity.Fingerprint)
	if resp, err := termHTTP.Do(req); err == nil {
		resp.Body.Close()
	}
}

func runtimeGOOS() string {
	return deskGOOS()
}

// noopDeskInput keeps the session streaming when OS input tools are missing.
type noopDeskInput struct{}

func (noopDeskInput) MouseMove(x, y int) error      { return nil }
func (noopDeskInput) MouseButton(button int, down bool) error { return nil }
func (noopDeskInput) MouseWheel(delta int) error    { return nil }
func (noopDeskInput) Key(vk int, down bool) error   { return nil }
func (noopDeskInput) Close() error                  { return nil }

func scaleImage(src image.Image, scale float64) image.Image {
	if scale <= 0 || scale >= 0.99 {
		return src
	}
	b := src.Bounds()
	nw := int(float64(b.Dx()) * scale)
	nh := int(float64(b.Dy()) * scale)
	if nw < 8 || nh < 8 {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := b.Min.Y + y*b.Dy()/nh
		for x := 0; x < nw; x++ {
			sx := b.Min.X + x*b.Dx()/nw
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func readDeskFrames(r io.Reader, inp deskInput, lang string, q *deskQuality, qMu *sync.Mutex, touch func(), fileTxChan chan<- []byte, screenW, screenH *int, applyMonitor func(int)) {
	var hdr [3]byte
	type fileUploadState struct {
		file     *os.File
		filename string
		size     int64
		received int64
	}
	var upload *fileUploadState

	sendFileInfo := func(typ string, meta map[string]interface{}) {
		meta["type"] = typ
		js, _ := json.Marshal(meta)
		frame := deskTxFrame('F', js)
		select {
		case fileTxChan <- frame:
		case <-time.After(5 * time.Second):
		}
	}

	for {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			if upload != nil {
				upload.file.Close()
				os.Remove(upload.file.Name())
			}
			return
		}
		n := int(binary.BigEndian.Uint16(hdr[1:]))
		payload := make([]byte, n)
		if n > 0 {
			if _, err := io.ReadFull(r, payload); err != nil {
				if upload != nil {
					upload.file.Close()
					os.Remove(upload.file.Name())
				}
				return
			}
		}
		touch()
		switch hdr[0] {
		case 'Q':
			var nq deskQuality
			if json.Unmarshal(payload, &nq) == nil {
				qMu.Lock()
				if nq.Scale > 0 {
					q.Scale = nq.Scale
				}
				if nq.Quality > 0 {
					q.Quality = nq.Quality
				}
				if nq.FPS > 0 {
					q.FPS = nq.FPS
				}
				if nq.Codec != "" {
					q.Codec = nq.Codec
				}
				mon := nq.Monitor
				qMu.Unlock()
				if mon > 0 && applyMonitor != nil {
					applyMonitor(mon)
				}
			}
		case 'N':
			var ev struct {
				ID int `json:"id"`
			}
			if json.Unmarshal(payload, &ev) == nil && applyMonitor != nil {
				applyMonitor(ev.ID)
			}
		case 'C':
			var ev struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(payload, &ev) == nil && ev.Text != "" {
				_ = deskClipboardSet(ev.Text)
			}
		case 'M':
			var ev struct {
				X      float64 `json:"x"`
				Y      float64 `json:"y"`
				Btn    int     `json:"btn"`
				Down   *bool   `json:"down"`
				Action string  `json:"action"`
			}
			if json.Unmarshal(payload, &ev) != nil {
				continue
			}
			sw, sh := 1920, 1080
			if screenW != nil && *screenW > 0 {
				sw = *screenW
			}
			if screenH != nil && *screenH > 0 {
				sh = *screenH
			}
			x := int(ev.X)
			y := int(ev.Y)
			if sw > 0 && ev.X <= 1 && ev.Y <= 1 {
				x = int(ev.X * float64(sw))
				y = int(ev.Y * float64(sh))
			}
			_ = inp.MouseMove(x, y)
			switch ev.Action {
			case "down":
				_ = inp.MouseButton(ev.Btn, true)
			case "up":
				_ = inp.MouseButton(ev.Btn, false)
			case "click":
				_ = inp.MouseButton(ev.Btn, true)
				_ = inp.MouseButton(ev.Btn, false)
			}
			if ev.Down != nil {
				_ = inp.MouseButton(ev.Btn, *ev.Down)
			}
		case 'W':
			var ev struct {
				Delta int `json:"delta"`
			}
			if json.Unmarshal(payload, &ev) == nil {
				_ = inp.MouseWheel(ev.Delta)
			}
		case 'B':
			var ev struct {
				Down bool   `json:"down"`
				VK   int    `json:"vk"`
				Key  string `json:"key"`
				Code string `json:"code"`
			}
			if json.Unmarshal(payload, &ev) != nil {
				continue
			}
			vk := ev.VK
			if vk == 0 {
				vk = deskKeyToVK(ev.Key, ev.Code)
			}
			if vk != 0 {
				_ = inp.Key(vk, ev.Down)
			}
		case 'f':
			var meta struct {
				Filename   string `json:"filename"`
				Size       int64  `json:"size"`
				TargetPath string `json:"target_path"`
			}
			if err := json.Unmarshal(payload, &meta); err != nil || meta.TargetPath == "" {
				continue
			}
			if meta.Size < 0 || meta.Size > 100<<20 {
				sendFileInfo("upload_ack", map[string]interface{}{
					"status": "error", "message": agentT(lang, "agent.file.upload_too_large"),
				})
				continue
			}
			target := filepath.Clean(meta.TargetPath)
			if !filepath.IsAbs(target) {
				target = filepath.Join(os.TempDir(), filepath.Base(target))
			}
			f, err := os.Create(target)
			if err != nil {
				sendFileInfo("upload_ack", map[string]interface{}{
					"status": "error", "message": agentT(lang, "agent.file.create_failed", err),
				})
				continue
			}
			upload = &fileUploadState{file: f, filename: meta.Filename, size: meta.Size}
		case 'u':
			if upload != nil {
				if upload.received+int64(len(payload)) > upload.size {
					upload.file.Close()
					os.Remove(upload.file.Name())
					sendFileInfo("upload_ack", map[string]interface{}{
						"status": "error", "message": agentT(lang, "agent.file.upload_oversize"),
					})
					upload = nil
					continue
				}
				if _, err := upload.file.Write(payload); err != nil {
					upload.file.Close()
					os.Remove(upload.file.Name())
					upload = nil
					continue
				}
				upload.received += int64(len(payload))
			}
		case 'e':
			if upload != nil {
				upload.file.Close()
				sendFileInfo("upload_ack", map[string]interface{}{
					"status": "ok", "filename": upload.filename, "size": upload.received,
				})
				upload = nil
			}
		case 'd':
			var meta struct {
				RemotePath string `json:"remote_path"`
			}
			if json.Unmarshal(payload, &meta) == nil && meta.RemotePath != "" {
				go handleFileDownload(meta.RemotePath, lang, fileTxChan)
			}
		}
	}
}
