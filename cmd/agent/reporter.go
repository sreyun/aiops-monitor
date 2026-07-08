package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"sync"
	"time"

	"aiops-monitor/shared"
)

// errForbidden signals that the server rejected a report with 403 (host not
// registered or fingerprint not bound). reportOnce reacts by re-registering.
var errForbidden = errors.New("forbidden")

// serverTarget is the runtime state for one backend server connection.
// Each target has its own HTTP client (connection pool isolation), its own
// token, and its own registration state — so one server being down or
// rejecting reports never affects the others.
type serverTarget struct {
	server string
	token  string
	httpc  *http.Client // isolated connection pool + 8s timeout

	regMu      sync.Mutex
	registered bool
}

// register sends the agent's identity (with this target's token) to the server.
// On success the target is marked registered; 403 or network errors return false
// but don't crash — the agent keeps retrying on subsequent report cycles.
func (t *serverTarget) register(base shared.Report) bool {
	body, _ := json.Marshal(map[string]string{
		"host_id":     base.HostID,
		"hostname":    base.Hostname,
		"token":       t.token,
		"fingerprint": base.Fingerprint,
	})
	resp, err := t.httpc.Post(t.server+"/api/v1/agent/register", "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Error("注册失败(将继续上报)", "server", t.server, "err", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		slog.Warn("注册被拒，可能 Token 已失效或指纹无效", "server", t.server, "status", resp.StatusCode)
		return false
	}
	t.regMu.Lock()
	t.registered = true
	t.regMu.Unlock()
	slog.Info("已向服务端注册", "server", t.server)
	return true
}

// isRegistered returns whether this target was successfully registered.
func (t *serverTarget) isRegistered() bool {
	t.regMu.Lock()
	defer t.regMu.Unlock()
	return t.registered
}

// send posts one report payload to this server. The report's Token field is
// set to this target's token before marshalling. Returns errForbidden on 403
// so the caller can re-register and retry.
func (t *serverTarget) send(rep shared.Report) error {
	rep.Token = t.token
	body, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	resp, err := t.httpc.Post(t.server+"/api/v1/agent/report", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return errForbidden
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("服务端返回状态码 %d", resp.StatusCode)
	}
	return nil
}

// Agent ties together the native collector (fast base metrics) and the plugin
// runner (slower custom/AI layer), then reports both to all configured backends.
// Metrics are collected exactly once per cycle and broadcast to every target —
// no duplicate collection regardless of how many servers are configured.
type Agent struct {
	targets        []*serverTarget
	reportInterval time.Duration
	pluginInterval time.Duration
	collector      Collector
	plugins        *PluginRunner
	identity       shared.Report // template with host fields pre-filled (Token is per-target)
	httpc          *http.Client  // used for non-report HTTP (e.g. plugin downloads)

	mu            sync.Mutex
	latestCustom  map[string]float64
	pendingEvents []shared.Event
	latestBase    *shared.Metrics // from a core plugin, used when native unsupported
}

func NewAgent(servers []ServerConfig, reportInterval, pluginInterval time.Duration,
	collector Collector, plugins *PluginRunner, hostID, category string) *Agent {
	targets := make([]*serverTarget, len(servers))
	for i, s := range servers {
		targets[i] = &serverTarget{
			server: s.Server,
			token:  s.Token,
			httpc:  &http.Client{Timeout: 8 * time.Second},
		}
	}
	return &Agent{
		targets:        targets,
		reportInterval: reportInterval,
		pluginInterval: pluginInterval,
		collector:      collector,
		plugins:        plugins,
		httpc:          &http.Client{Timeout: 8 * time.Second},
		latestCustom:   map[string]float64{},
		identity: shared.Report{
			HostID:      hostID,
			Hostname:    hostname(),
			OS:          runtime.GOOS,
			Platform:    osVersion(),
			Arch:        runtime.GOARCH,
			IP:          primaryIP(),
			Kernel:      kernelVersion(),
			Category:    category,
			Fingerprint: machineFingerprint(),
		},
	}
}

func (a *Agent) Run() {
	slog.Info("Agent 核心启动",
		"host", a.identity.Hostname,
		"os", a.identity.OS,
		"collector", a.collector.Name(),
		"id", short(a.identity.HostID),
		"servers", len(a.targets))
	for i, t := range a.targets {
		slog.Info("服务端", "index", i+1, "url", t.server)
	}
	if a.identity.Fingerprint != "" {
		slog.Info("机器指纹", "fingerprint", short(a.identity.Fingerprint))
	}
	if !a.collector.Supported() {
		slog.Info("提示: 当前平台无原生采集器，基础指标依赖 core 插件(plugins/core_metrics.py)")
	}

	// Register to all targets (best-effort, non-blocking on failures)
	for _, t := range a.targets {
		t.register(a.identity)
	}

	go a.pluginLoop() // Python layer, lower frequency

	// Start one terminal channel per target — each server independently
	// long-polls for pending terminal sessions.
	for _, t := range a.targets {
		go a.runTerminalChannelFor(t)
	}

	// Start one forward channel per target — each server independently
	// long-polls for pending port forwarding sessions.
	for _, t := range a.targets {
		go a.runForwardChannelFor(t)
	}

	// base-metric report loop, higher frequency
	ticker := time.NewTicker(a.reportInterval)
	defer ticker.Stop()
	a.reportOnce() // report immediately
	for range ticker.C {
		a.reportOnce()
	}
}

func (a *Agent) pluginLoop() {
	a.runPlugins() // run promptly on startup
	ticker := time.NewTicker(a.pluginInterval)
	defer ticker.Stop()
	for range ticker.C {
		a.runPlugins()
	}
}

func (a *Agent) runPlugins() {
	res := a.plugins.RunAll(func(format string, args ...any) {
		slog.Info(fmt.Sprintf(format, args...))
	})
	a.mu.Lock()
	if len(res.custom) > 0 {
		a.latestCustom = res.custom
	}
	if res.base != nil {
		a.latestBase = res.base
	}
	if len(res.events) > 0 {
		a.pendingEvents = append(a.pendingEvents, res.events...)
		slog.Info("插件产生事件", "count", len(res.events))
	}
	a.mu.Unlock()
}

// reportOnce collects metrics exactly once, then broadcasts the report to all
// configured server targets concurrently. Each target independently handles
// 403 (re-register + retry) and network errors — one server being down never
// blocks or affects the others. Events are re-queued only if ALL targets
// failed (at least one success means events were delivered).
func (a *Agent) reportOnce() {
	var base shared.Metrics
	if a.collector.Supported() {
		m, err := a.collector.Collect()
		if err != nil {
			slog.Error("原生采集失败", "err", err)
		}
		base = m
	}

	a.mu.Lock()
	if !a.collector.Supported() && a.latestBase != nil {
		base = *a.latestBase
	}
	custom := make(map[string]float64, len(a.latestCustom))
	for k, v := range a.latestCustom {
		custom[k] = v
	}
	events := a.pendingEvents
	a.pendingEvents = nil
	a.mu.Unlock()

	// Build the base report (Token is set per-target inside send()).
	rep := a.identity
	rep.Metrics = base
	if len(custom) > 0 {
		rep.Custom = custom
	}
	rep.Events = events

	// Broadcast to all targets concurrently — each gets its own goroutine so
	// a slow/unreachable server can't block the others (8s timeout isolation).
	var wg sync.WaitGroup
	results := make([]bool, len(a.targets)) // results[i] = true if target i succeeded

	for i, t := range a.targets {
		wg.Add(1)
		go func(idx int, tgt *serverTarget) {
			defer wg.Done()
			err := tgt.send(rep)
			if err == errForbidden {
				slog.Warn("上报被拒(指纹未绑定)，重新注册后重试", "server", tgt.server)
				if tgt.register(a.identity) {
					err = tgt.send(rep)
				} else {
					err = fmt.Errorf("注册失败，跳过本次上报重试")
				}
			}
			if err != nil {
				slog.Error("上报失败", "server", tgt.server, "err", err)
				results[idx] = false
				return
			}
			results[idx] = true
			slog.Info("上报成功",
				"server", tgt.server,
				"cpu", base.CPUPercent,
				"mem", base.MemPercent,
				"disk", base.DiskPercent,
				"custom", len(custom),
				"events", len(events))
		}(i, t)
	}
	wg.Wait()

	// Re-queue events only if ALL targets failed — at least one success means
	// the events were delivered (duplicates across servers are acceptable;
	// duplicates to the SAME server from re-queueing are not).
	allFailed := true
	for _, ok := range results {
		if ok {
			allFailed = false
			break
		}
	}
	if allFailed && len(events) > 0 {
		a.mu.Lock()
		a.pendingEvents = append(events, a.pendingEvents...)
		a.mu.Unlock()
	}
}

func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
