package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CheckStatus is the latest runtime result of a custom check.
type CheckStatus struct {
	OK        bool
	Message   string
	LatencyMs float64
	CheckedAt int64
}

// checkRunner executes operator-defined synthetic checks (HTTP / TCP) on their
// intervals and raises alerts + notifications on failure and recovery.
type checkRunner struct {
	cfg      *ConfigStore
	store    *Store
	notifier *Notifier
	httpc    *http.Client

	mu      sync.Mutex
	status  map[string]CheckStatus
	down    map[string]bool
	lastRun map[string]time.Time
}

func newCheckRunner(cfg *ConfigStore, store *Store, notifier *Notifier) *checkRunner {
	return &checkRunner{
		cfg: cfg, store: store, notifier: notifier,
		httpc: &http.Client{
			Timeout:       8 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		status:  map[string]CheckStatus{},
		down:    map[string]bool{},
		lastRun: map[string]time.Time{},
	}
}

func (cr *checkRunner) Run(tick time.Duration) {
	t := time.NewTicker(tick)
	defer t.Stop()
	cr.sweep()
	for range t.C {
		cr.sweep()
	}
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
		if !live[id] {
			delete(cr.status, id)
			delete(cr.down, id)
			delete(cr.lastRun, id)
		}
	}
	cr.mu.Unlock()
}

func (cr *checkRunner) runCheck(c CustomCheck) {
	start := time.Now()
	var ok bool
	var msg string
	switch c.Type {
	case "http":
		ok, msg = cr.probeHTTP(c.Target)
	case "tcp":
		ok, msg = cr.probeTCP(c.Target)
	default:
		ok, msg = false, "未知检查类型: "+c.Type
	}
	lat := float64(time.Since(start).Milliseconds())
	cr.mu.Lock()
	cr.status[c.ID] = CheckStatus{OK: ok, Message: msg, LatencyMs: lat, CheckedAt: time.Now().Unix()}
	wasDown := cr.down[c.ID]
	cr.down[c.ID] = !ok
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

func (cr *checkRunner) probeHTTP(target string) (bool, string) {
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}
	resp, err := cr.httpc.Get(target)
	if err != nil {
		return false, "请求失败: " + err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return false, "HTTP " + resp.Status
	}
	return true, "HTTP " + resp.Status
}

func (cr *checkRunner) probeTCP(target string) (bool, string) {
	conn, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		return false, "连接失败: " + err.Error()
	}
	conn.Close()
	return true, "连接正常"
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

// DownAlerts returns the currently-failing checks as alerts for the /alerts view.
func (cr *checkRunner) DownAlerts() []Alert {
	checks := cr.cfg.Checks()
	cr.mu.Lock()
	defer cr.mu.Unlock()
	var out []Alert
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
