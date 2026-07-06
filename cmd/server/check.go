package main

import (
	"fmt"
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

const selfCheckID = "__self_health__"

// checkRunner executes operator-defined synthetic checks (HTTP / TCP) on their
// intervals and raises alerts + notifications on failure and recovery.
type checkRunner struct {
	cfg      *ConfigStore
	store    *Store
	notifier *Notifier
	httpc    *http.Client
	selfAddr string // e.g. "127.0.0.1:8080" — used for the built-in self health-check

	mu      sync.Mutex
	status  map[string]CheckStatus
	down    map[string]bool
	lastRun map[string]time.Time
}

func newCheckRunner(cfg *ConfigStore, store *Store, notifier *Notifier, selfAddr string) *checkRunner {
	return &checkRunner{
		cfg: cfg, store: store, notifier: notifier, selfAddr: selfAddr,
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
	selfTick := time.NewTicker(10 * time.Second)
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
	if err != nil {
		msg = "请求失败: " + err.Error()
	} else {
		resp.Body.Close()
		if resp.StatusCode < 400 {
			ok = true
			msg = "HTTP " + resp.Status
		} else {
			msg = "HTTP " + resp.Status
		}
	}
	cr.mu.Lock()
	cr.status[selfCheckID] = CheckStatus{OK: ok, Message: msg, LatencyMs: lat, CheckedAt: time.Now().Unix()}
	wasDown := cr.down[selfCheckID]
	cr.down[selfCheckID] = !ok
	cr.mu.Unlock()

	if !ok && !wasDown {
		cr.store.AddLog(LogEntry{Kind: "系统", Level: "critical", Actor: "自监控", Host: "服务端", Message: "服务端健康检查失败: " + msg})
	} else if ok && wasDown {
		cr.store.AddLog(LogEntry{Kind: "系统", Level: "info", Actor: "自监控", Host: "服务端", Message: "服务端健康检查恢复"})
	}
}

// portFromAddr extracts the port portion from an addr like ":8080" or "0.0.0.0:8080".
func portFromAddr(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i+1:]
	}
	return "8080"
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
	case "process":
		ok, msg = cr.probeProcess(c.Target)
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

	hosts := cr.store.ListHosts()
	var found bool
	var procNames []string
	for _, h := range hosts {
		if h.ID == hostID && h.Latest != nil {
			procNames = h.Latest.ProcessNames
			for _, p := range procNames {
				if strings.EqualFold(p, procName) {
					found = true
					break
				}
			}
			break
		}
	}
	if !found {
		if len(procNames) > 0 {
			return false, fmt.Sprintf("进程 %q 未运行（共 %d 个进程）", procName, len(procNames))
		}
		hid := hostID
		if len(hid) > 8 { hid = hid[:8] }
		return false, fmt.Sprintf("主机 %s 无进程数据或已离线", hid)
	}
	return true, fmt.Sprintf("进程 %q 运行中", procName)
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
		lvl := "critical"
		out = append(out, Alert{
			Level: lvl, Type: "check", Scope: selfCheckID, Hostname: SelfCheckName,
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
