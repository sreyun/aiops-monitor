package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CheckStatus is the latest runtime result of a custom check.
type CheckStatus struct {
	OK        bool
	Message   string
	LatencyMs float64 // response/connect time; for ping this is the average round-trip time
	CheckedAt int64
	// Type-specific detail (0 / -1 when not applicable):
	StatusCode int     // HTTP: last status code
	CertDays   int     // HTTP: days until the TLS certificate expires (-1 = not HTTPS / unknown)
	LossPct    float64 // ping: packet-loss percentage (-1 = not a ping check)
}

const selfCheckID = "__self_health__"

// checkHistMax bounds the per-check time-series ring kept for the "history
// curve" view (≈24h at the default 30s interval; longer at bigger intervals).
const checkHistMax = 2880

// CheckPoint is one sampled result kept for a check's trend chart.
type CheckPoint struct {
	Ts         int64   `json:"timestamp"`
	OK         bool    `json:"ok"`
	LatencyMs  float64 `json:"latency_ms"`
	StatusCode int     `json:"status_code,omitempty"`
	LossPct    float64 `json:"loss_pct,omitempty"`
}

// checkRunner executes operator-defined synthetic checks (HTTP / TCP) on their
// intervals and raises alerts + notifications on failure and recovery.
type checkRunner struct {
	cfg      *ConfigStore
	store    *Store
	notifier *Notifier
	httpc    *http.Client
	selfAddr string // e.g. "127.0.0.1:8529" — used for the built-in self health-check

	mu        sync.Mutex
	status    map[string]CheckStatus
	down      map[string]bool
	downSince map[string]int64        // check id -> unix time it first went down
	lastRun   map[string]time.Time
	history   map[string][]CheckPoint // check id -> bounded time-series for the trend view
}

func newCheckRunner(cfg *ConfigStore, store *Store, notifier *Notifier, selfAddr string) *checkRunner {
	return &checkRunner{
		cfg: cfg, store: store, notifier: notifier, selfAddr: selfAddr,
		httpc: &http.Client{
			Timeout:       5 * time.Second, // reduced from 8s; retry logic provides resilience
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		status:    map[string]CheckStatus{},
		down:      map[string]bool{},
		downSince: map[string]int64{},
		lastRun:   map[string]time.Time{},
		history:   map[string][]CheckPoint{},
	}
}

// recordHistory appends a bounded time-series point; the caller must hold cr.mu.
func (cr *checkRunner) recordHistory(id string, p CheckPoint) {
	h := append(cr.history[id], p)
	if len(h) > checkHistMax {
		h = h[len(h)-checkHistMax:]
	}
	cr.history[id] = h
}

// HistoryOf returns a copy of a check's trend series (nil if none yet).
func (cr *checkRunner) HistoryOf(id string) []CheckPoint {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	src := cr.history[id]
	if len(src) == 0 {
		return nil
	}
	out := make([]CheckPoint, len(src))
	copy(out, src)
	return out
}

// markDown records the down/up transition time; caller holds cr.mu.
func (cr *checkRunner) markDown(id string, down bool) {
	if down {
		if _, ok := cr.downSince[id]; !ok {
			cr.downSince[id] = time.Now().Unix()
		}
	} else {
		delete(cr.downSince, id)
	}
}

func (cr *checkRunner) Run(tick time.Duration) {
	t := time.NewTicker(tick)
	defer t.Stop()
	cr.sweep()
	selfTick := time.NewTicker(30 * time.Second) // reduced from 10s to avoid excessive probing
	defer selfTick.Stop()
	if cr.selfAddr != "" {
		go cr.runSelfCheck() // run once immediately
	}
	for {
		select {
		case <-t.C:
			cr.sweep()
		case <-selfTick.C:
			if cr.selfAddr != "" {
				go cr.runSelfCheck()
			}
		}
	}
}

// runSelfCheck performs a lightweight HTTP check against the server's own
// /healthz endpoint. This replaces the old user-configured 127.0.0.1 check
// which fails inside Docker because 127.0.0.1 may not route correctly.
func (cr *checkRunner) runSelfCheck() {
	target := "http://127.0.0.1:" + portFromAddr(cr.selfAddr) + "/healthz"
	start := time.Now()
	resp, err := cr.httpc.Get(target)
	lat := float64(time.Since(start).Milliseconds())
	ok := false
	msg := ""
	code := 0
	if err != nil {
		msg = "请求失败: " + err.Error()
	} else {
		code = resp.StatusCode
		resp.Body.Close()
		msg = "HTTP " + resp.Status
		ok = resp.StatusCode < 400
	}
	nowUnix := time.Now().Unix()
	cr.mu.Lock()
	cr.status[selfCheckID] = CheckStatus{OK: ok, Message: msg, LatencyMs: lat, CheckedAt: nowUnix, StatusCode: code, CertDays: -1}
	cr.recordHistory(selfCheckID, CheckPoint{Ts: nowUnix, OK: ok, LatencyMs: lat, StatusCode: code})
	wasDown := cr.down[selfCheckID]
	cr.down[selfCheckID] = !ok
	cr.markDown(selfCheckID, !ok)
	cr.mu.Unlock()

	if !ok && !wasDown {
		// Self health-check failures should not clutter the activity log
		// They are still tracked in alerts and visible in the checks view
	} else if ok && wasDown {
		// Recovery also doesn't need an activity log entry for internal checks
	}
}

// portFromAddr extracts the port portion from an addr like ":8529" or "0.0.0.0:8529".
func portFromAddr(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i+1:]
	}
	return "8529"
}

func (cr *checkRunner) sweep() {
	now := time.Now()
	for _, c := range cr.cfg.Checks() {
		if !c.Enabled {
			continue
		}
		iv := c.IntervalSec
		if iv < 5 {
			iv = 30
		}
		cr.mu.Lock()
		last := cr.lastRun[c.ID]
		due := last.IsZero() || now.Sub(last) >= time.Duration(iv)*time.Second
		if due {
			cr.lastRun[c.ID] = now
		}
		cr.mu.Unlock()
		if due {
			cr.runCheck(c)
		}
	}
	cr.gc()
}

// gc drops runtime state for checks that no longer exist.
func (cr *checkRunner) gc() {
	live := map[string]bool{}
	for _, c := range cr.cfg.Checks() {
		live[c.ID] = true
	}
	cr.mu.Lock()
	for id := range cr.status {
		if id != selfCheckID && !live[id] {
			delete(cr.status, id)
			delete(cr.down, id)
			delete(cr.downSince, id)
			delete(cr.lastRun, id)
			delete(cr.history, id)
		}
	}
	cr.mu.Unlock()
}

func (cr *checkRunner) runCheck(c CustomCheck) {
	start := time.Now()
	var ok bool
	var msg string
	code, certDays := 0, -1
	lossPct, pingRTT := -1.0, -1.0
	switch c.Type {
	case "http":
		ok, msg, code, certDays = cr.probeHTTP(c.Target)
	case "tcp":
		ok, msg = cr.probeTCP(c.Target)
	case "ping":
		ok, msg, pingRTT, lossPct = cr.probePing(c.Target)
	case "process":
		ok, msg = cr.probeProcess(c.Target)
	default:
		ok, msg = false, "未知检查类型: "+c.Type
	}
	lat := float64(time.Since(start).Milliseconds())
	if c.Type == "ping" { // ping "latency" is the ICMP round-trip time, not the command duration
		if pingRTT >= 0 {
			lat = pingRTT
		} else {
			lat = 0
		}
	}
	nowUnix := time.Now().Unix()
	cr.mu.Lock()
	cr.status[c.ID] = CheckStatus{OK: ok, Message: msg, LatencyMs: lat, CheckedAt: nowUnix, StatusCode: code, CertDays: certDays, LossPct: lossPct}
	cr.recordHistory(c.ID, CheckPoint{Ts: nowUnix, OK: ok, LatencyMs: lat, StatusCode: code, LossPct: lossPct})
	wasDown := cr.down[c.ID]
	cr.down[c.ID] = !ok
	cr.markDown(c.ID, !ok)
	cr.mu.Unlock()

	if !ok && !wasDown {
		cr.transition(c, false, msg)
	} else if ok && wasDown {
		cr.transition(c, true, msg)
	}
}

func (cr *checkRunner) transition(c CustomCheck, up bool, msg string) {
	lvl := c.Level
	if lvl == "" {
		lvl = "critical"
	}
	a := Alert{Level: lvl, Type: "check", Scope: c.ID, Hostname: c.Name, Timestamp: time.Now().Unix()}
	if up {
		a.Level = "info"
		a.Message = "自定义监控恢复：" + c.Name
	} else {
		a.Message = "自定义监控异常：" + c.Name + "（" + msg + "）"
	}
	cr.store.AddLog(LogEntry{Kind: "系统", Level: a.Level, Actor: "自定义监控", Host: c.Name, Message: a.Message})
	if cfg := cr.cfg.Get(); cfg.AlertsEnabled {
		cr.notifier.pushChannels(cfg, a, !up)
	}
}

// probeHTTP returns (ok, message, statusCode, certDaysRemaining). certDays is -1
// for plain HTTP or when the certificate can't be read; for HTTPS it is the whole
// number of days until the leaf certificate's NotAfter.
func (cr *checkRunner) probeHTTP(target string) (bool, string, int, int) {
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}

	// Retry once on failure to avoid transient errors
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := cr.httpc.Get(target)
		if err == nil {
			certDays := certDaysRemaining(resp)
			code := resp.StatusCode
			resp.Body.Close()
			if code >= 400 {
				return false, "HTTP " + resp.Status, code, certDays
			}
			return true, "HTTP " + resp.Status, code, certDays
		}
		lastErr = err
		if attempt == 0 {
			time.Sleep(500 * time.Millisecond) // brief pause before retry
		}
	}
	return false, "请求失败: " + lastErr.Error(), 0, -1
}

// certDaysRemaining returns whole days until the served leaf TLS certificate
// expires, or -1 for non-HTTPS responses or when no peer certificate is present.
func certDaysRemaining(resp *http.Response) int {
	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		return -1
	}
	notAfter := resp.TLS.PeerCertificates[0].NotAfter
	d := time.Until(notAfter).Hours() / 24
	return int(d)
}

func (cr *checkRunner) probeTCP(target string) (bool, string) {
	conn, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		return false, "连接失败: " + err.Error()
	}
	conn.Close()
	return true, "连接正常"
}

// probePing runs the system `ping` (zero-dependency, no raw-socket privilege
// needed) against target and returns (ok, message, avgRTTms, lossPct). ICMP is
// unreachable/blocked → 100% loss → not ok. Reachable (any reply) → ok, with the
// loss percentage surfaced separately.
func (cr *checkRunner) probePing(target string) (bool, string, float64, float64) {
	target = strings.TrimSpace(target)
	// Guard against argument injection: target is passed as an argv element (no
	// shell), but a leading '-' or embedded whitespace could still be read as a
	// ping flag, so reject those outright.
	if target == "" || strings.HasPrefix(target, "-") || strings.ContainsAny(target, " \t\r\n") {
		return false, "无效的主机地址", -1, -1
	}
	const sent = 3
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "ping", "-n", strconv.Itoa(sent), "-w", "2000", target)
	case "darwin":
		cmd = exec.CommandContext(ctx, "ping", "-c", strconv.Itoa(sent), "-t", "6", target)
	default: // linux and other unix
		cmd = exec.CommandContext(ctx, "ping", "-c", strconv.Itoa(sent), "-W", "2", "-w", "6", target)
	}
	out, _ := cmd.CombinedOutput() // ping exits non-zero on 100% loss; parse output regardless
	received, avg := parsePingOutput(string(out))
	if received > sent {
		received = sent
	}
	loss := float64(sent-received) / float64(sent) * 100
	if received == 0 {
		return false, "不可达（100% 丢包）", -1, 100
	}
	return true, fmt.Sprintf("可达 · 平均 %.1f ms · 丢包 %.0f%%", avg, loss), avg, loss
}

// parsePingOutput counts successful replies and averages their round-trip times.
// It keys off the per-reply time marker, which every platform and locale prints:
// "time=" (Linux/macOS/EN-Windows), "time<" (Windows sub-1ms, e.g. "time<1ms"),
// and the Chinese-Windows "时间=" / "时间<". Keying off replies rather than the
// OS-specific summary line keeps the parser format-independent.
func parsePingOutput(out string) (received int, avgRTTms float64) {
	var sum float64
	for _, marker := range []string{"time=", "time<", "时间=", "时间<"} {
		idx := 0
		for {
			i := strings.Index(out[idx:], marker)
			if i < 0 {
				break
			}
			p := idx + i + len(marker)
			j := p
			for j < len(out) && (out[j] == '.' || (out[j] >= '0' && out[j] <= '9')) {
				j++
			}
			if v, err := strconv.ParseFloat(out[p:j], 64); err == nil {
				received++
				sum += v
			}
			idx = j + 1
		}
	}
	if received > 0 {
		avgRTTms = sum / float64(received)
	}
	return received, avgRTTms
}

// probeProcess checks whether a given process name is running on the target host.
// Target format: "hostID/processName" (e.g. "abc123/nginx").
func (cr *checkRunner) probeProcess(target string) (bool, string) {
	idx := -1
	for i := 0; i < len(target); i++ {
		if target[i] == '/' {
			idx = i
			break
		}
	}
	if idx <= 0 || idx >= len(target)-1 {
		return false, "目标格式错误，应为 hostID/进程名"
	}
	hostID := target[:idx]
	procName := target[idx+1:]

	procNames, ok := cr.store.GetProcessNames(hostID)
	if !ok || len(procNames) == 0 {
		return false, fmt.Sprintf("主机 %s 无进程数据或已离线", shortID(hostID))
	}
	for _, p := range procNames {
		// substring match, case-insensitive: "nginx" matches "nginx.exe"
		if strings.Contains(strings.ToLower(p), strings.ToLower(procName)) {
			return true, fmt.Sprintf("进程 %q 运行中（匹配 %s）", procName, p)
		}
	}
	return false, fmt.Sprintf("进程 %q 未运行（共上报 %d 个进程）", procName, len(procNames))
}

func (cr *checkRunner) snapshot() map[string]CheckStatus {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	out := make(map[string]CheckStatus, len(cr.status))
	for k, v := range cr.status {
		out[k] = v
	}
	return out
}

// SelfCheckName is the display name for the built-in self health-check.
const SelfCheckName = "服务端健康检查"

// DownAlerts returns the currently-failing checks as alerts for the /alerts view.
func (cr *checkRunner) DownAlerts() []Alert {
	var out []Alert

	// Self health-check
	cr.mu.Lock()
	if cr.down[selfCheckID] {
		st := cr.status[selfCheckID]
		out = append(out, Alert{
			Level: "critical", Type: "check", Scope: selfCheckID, Hostname: SelfCheckName,
			Since: cr.downSince[selfCheckID],
			Message: "服务端健康检查异常（" + st.Message + "）", Timestamp: st.CheckedAt,
		})
	}
	cr.mu.Unlock()

	checks := cr.cfg.Checks()
	cr.mu.Lock()
	defer cr.mu.Unlock()
	for _, c := range checks {
		if !c.Enabled || !cr.down[c.ID] {
			continue
		}
		lvl := c.Level
		if lvl == "" {
			lvl = "critical"
		}
		st := cr.status[c.ID]
		out = append(out, Alert{
			Level: lvl, Type: "check", Scope: c.ID, Hostname: c.Name,
			Since: cr.downSince[c.ID],
			Message: "自定义监控异常：" + c.Name + "（" + st.Message + "）", Timestamp: st.CheckedAt,
		})
	}
	return out
}

// runNow triggers an immediate check (used right after add / edit).
func (cr *checkRunner) runNow(id string) {
	for _, c := range cr.cfg.Checks() {
		if c.ID == id && c.Enabled {
			cr.mu.Lock()
			cr.lastRun[c.ID] = time.Now()
			cr.mu.Unlock()
			go cr.runCheck(c)
			return
		}
	}
}
