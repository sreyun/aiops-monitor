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
		resp, doErr := termHTTP.Do(req)
		if doErr == nil {
			resp.Body.Close()
		}
		reqDone <- doErr
	}()

	sw, sh := cap.Size()
	mons := cap.Monitors()
	h264OK := ffmpegAvailable()
	codecs := []string{"jpeg"}
	if h264OK {
		codecs = append(codecs, "h264")
	}
	meta, _ := json.Marshal(map[string]any{
		"w": sw, "h": sh, "os": runtimeGOOS(),
		"scale": q.Scale, "quality": q.Quality, "fps": q.FPS,
		"codec": q.Codec, "codecs": codecs,
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
		buf := make([]byte, 64*1024)
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
					p, err := startH264Pipe(currentMon, cq.Scale, fps)
					if err != nil {
						codec = "jpeg"
					} else {
						h264Mu.Lock()
						h264 = p
						h264Mu.Unlock()
						go func(pipe *h264Pipe) {
							for !stop.Load() {
								n, err := pipe.Read(buf)
								if n > 0 {
									chunk := make([]byte, n)
									copy(chunk, buf[:n])
									if writeTx(deskTxFrame('H', chunk)) != nil {
										return
									}
								}
								if err != nil {
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
				msg, _ := json.Marshal(map[string]string{"error": err.Error()})
				_ = writeTx(deskTxFrame('E', msg))
				return
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
		for _, m := range cap.Monitors() {
			if m.ID == id {
				currentMon = m
				sw, sh = m.Width, m.Height
				break
			}
		}
		stopH264()
		js, _ := json.Marshal(map[string]any{"w": sw, "h": sh, "monitors": cap.Monitors(), "monitor": id})
		_ = writeTx(deskTxFrame('S', js))
	}

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
				X, Y   float64 `json:"x"`
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
