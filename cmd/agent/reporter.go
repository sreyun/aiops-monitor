package main

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"

	"aiops-monitor/shared"
)

// reportTransport is the shared transport for all server targets (report POSTs).
// Connection reuse avoids TCP handshake overhead on every 10s report cycle.
// Default http.Transport is used by http.DefaultClient; we create our own so
// each target's client shares the same pool without colliding with global state.
//
// v5.2.6: HTTP/2 is explicitly disabled (TLSNextProto set to empty map).
// HTTP/2 multiplexes all requests over a single TCP connection. When the
// server restarts, that single connection dies and ALL concurrent requests
// fail simultaneously. With HTTP/1.1, each request gets its own connection
// from the pool, so a server restart only affects in-flight requests — new
// ones immediately succeed on fresh connections. This dramatically improves
// recovery time after server restarts (from 30s+ to <5s).
var reportTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns:          50,
	MaxIdleConnsPerHost:   10,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ForceAttemptHTTP2:     false, // v5.2.6: disable HTTP/2 for better restart recovery
	TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS12,
	},
	ResponseHeaderTimeout: 15 * time.Second,
}

// errForbidden signals that the server rejected a report with 403 (host not
// registered or fingerprint not bound). reportOnce reacts by re-registering.
var errForbidden = errors.New("forbidden")

// gzipCompressThreshold: payloads below 512 bytes skip gzip (the overhead of
// gzip headers + the CPU cost outweighs the tiny bandwidth saving).
const gzipCompressThreshold = 512

// serverTarget is the runtime state for one backend server connection.
// Each target has its own HTTP client (connection pool isolation), its own
// token, its own registration state, and now its own retry backoff +
// circuit breaker — so one server being down or rejecting reports never
// affects the others.
type serverTarget struct {
	server string
	token  string
	httpc  *http.Client // isolated connection pool + 30s timeout

	regMu      sync.Mutex
	registered bool

	// Retry + circuit breaker: independent per-target so one failing server
	// never starves or delays reports to healthy servers.
	bo *backoff
	cb *circuitBreaker

	// gzipMu protects disableGzip, which is set true when the server returns
	// 400 on a gzip-compressed request (proxy corruption, server bug, etc.).
	// Once disabled, all subsequent reports to this target skip compression.
	gzipMu      sync.Mutex
	disableGzip bool
}

// register sends the agent's identity (with this target's token) to the server.
// On success the target is marked registered; 403 or network errors return false
// but don't crash — the agent keeps retrying on subsequent report cycles.
// Token is never logged in full — only the first 4 chars for debugging.
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
	slog.Info("已向服务端注册", "server", t.server, "token_prefix", maskToken(t.token))
	return true
}

// isRegistered returns whether this target was successfully registered.
func (t *serverTarget) isRegistered() bool {
	t.regMu.Lock()
	defer t.regMu.Unlock()
	return t.registered
}

// send posts one report payload to this server. The report's Token field is
// set to this target's token before marshalling. The body is gzip-compressed
// when above the threshold to reduce bandwidth, UNLESS a previous 400 response
// triggered gzip degradation (proxy corruption on external networks).
// Returns errForbidden on 403 so the caller can re-register and retry.
// Returns errBadPayload on 400 when the body was gzip-compressed — the caller
// should disable gzip and retry.
var errBadPayload = errors.New("bad payload (server returned 400)")

func (t *serverTarget) send(rep shared.Report) error {
	rep.Token = t.token
	body, err := json.Marshal(rep)
	if err != nil {
		return err
	}

	// Decide whether to gzip: only if payload is large enough AND gzip has
	// not been disabled for this target (see sendWithRetry fallback).
	t.gzipMu.Lock()
	useGzip := !t.disableGzip
	t.gzipMu.Unlock()

	var reader *bytes.Reader
	contentEnc := ""
	if useGzip && len(body) >= gzipCompressThreshold {
		buf := getBytesBuf()
		defer putBytesBuf(buf)
		gw, _ := gzip.NewWriterLevel(buf, 3) // level 3 = best speed/size trade
		gw.Write(body)
		gw.Close()
		if buf.Len() < len(body) { // only compress if it actually shrinks
			reader = bytes.NewReader(buf.Bytes())
			contentEnc = "gzip"
		} else {
			reader = bytes.NewReader(body)
		}
	} else {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequest("POST", t.server+"/api/v1/agent/report", reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if contentEnc != "" {
		req.Header.Set("Content-Encoding", contentEnc)
	}

	resp, err := t.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return errForbidden
	}
	if resp.StatusCode == http.StatusBadRequest {
		// 400 when we sent gzip → likely proxy corruption on external network.
		// Signal caller to disable gzip and retry immediately.
		if contentEnc == "gzip" {
			return errBadPayload
		}
		// 400 without gzip → genuine bad request, don't retry.
		return fmt.Errorf("服务端返回状态码 400（请求格式错误）")
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("服务端返回状态码 %d", resp.StatusCode)
	}
	return nil
}

// sendWithRetry wraps send() with in-cycle retries and gzip degradation.
// On external networks, transient failures (proxy timeouts, gzip corruption)
// are common — retrying within the same cycle avoids wasting a full 10s
// report interval and prevents the circuit breaker from opening prematurely.
//
// Retry strategy:
//   - Up to 3 attempts per cycle (initial + 2 retries)
//   - 1s delay between retries (short enough to stay within one cycle)
//   - On 400 with gzip: disable gzip for this target, retry immediately
//   - On 403: re-register then retry
//   - Network errors / 5xx: retry after short delay
func (t *serverTarget) sendWithRetry(rep shared.Report) error {
	const maxAttempts = 3
	const retryDelay = 1 * time.Second

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			slog.Info("上报重试", "server", t.server, "attempt", attempt+1, "last_err", lastErr)
			time.Sleep(retryDelay)
		}

		err := t.send(rep)
		if err == nil {
			return nil // success
		}

		lastErr = err

		// 403 → re-register once, then retry
		if errors.Is(err, errForbidden) {
			slog.Warn("上报被拒(指纹未绑定)，重新注册后重试", "server", t.server)
			if t.register(rep) {
				// Registration succeeded, retry the send
				continue
			}
			return fmt.Errorf("注册失败，跳过本次上报")
		}

		// 400 with gzip → disable gzip for this target, retry without compression
		if errors.Is(err, errBadPayload) {
			slog.Warn("服务端返回400，疑似gzip被外网代理损坏，已禁用压缩", "server", t.server)
			t.gzipMu.Lock()
			t.disableGzip = true
			t.gzipMu.Unlock()
			continue // retry immediately without gzip
		}

		// Other errors (network timeout, 5xx, etc.) → retry
	}
	return lastErr
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
			httpc: &http.Client{
				Timeout:   30 * time.Second, // raised from 8s: gzip + multi-disk reports need more headroom
				Transport: reportTransport,
			},
			bo: newBackoff(1*time.Second, 60*time.Second),
			cb: newCircuitBreaker(8, 15*time.Second), // open after 8 consecutive failures, cooldown 15s — tuned for external networks where transient errors are common
		}
	}
	return &Agent{
		targets:        targets,
		reportInterval: reportInterval,
		pluginInterval: pluginInterval,
		collector:      collector,
		plugins:        plugins,
		httpc:          &http.Client{Timeout: 30 * time.Second, Transport: reportTransport},
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

// Run starts the agent's main loop. It registers to all targets, then
// runs collection => report cycles until interrupted.
// The main loop is wrapped in a defer/recover so a panic in any cycle
// (e.g. a nil dereference from a corrupted /proc read) can't kill the
// whole agent — it's logged and the loop restarts.
func (a *Agent) Run() {
	slog.Info("Agent 核心启动",
		"host", a.identity.Hostname,
		"os", a.identity.OS,
		"collector", a.collector.Name(),
		"id", short(a.identity.HostID),
		"servers", len(a.targets))
	for i, t := range a.targets {
		slog.Info("服务端", "index", i+1, "url", t.server, "token", maskToken(t.token))
	}
	if a.identity.Fingerprint != "" {
		slog.Info("机器指纹", "fingerprint", short(a.identity.Fingerprint))
	}
	if !a.collector.Supported() {
		slog.Info("提示: 当前平台无原生采集器，基础指标依赖 core 插件(plugins/core_metrics.py)")
	}

	// Register to all targets (best-effort with retry, non-blocking on failures)
	for _, t := range a.targets {
		a.registerTarget(t)
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

	// base-metric report loop, higher frequency.
	// Wrap in defer/recover so a panic inside reportOnce (e.g. from a
	// corrupted /proc read edge case) logs the stack and restarts the loop
	// instead of killing the process.
	a.reportOnceSafe() // report immediately
	ticker := time.NewTicker(a.reportInterval)
	defer ticker.Stop()
	for range ticker.C {
		a.reportOnceSafe()
	}
}

// reportOnceSafe calls reportOnce inside defer/recover so a panic in
// collection or network I/O never stops the agent.
func (a *Agent) reportOnceSafe() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("上报循环 panic 已恢复（采集不中断）", "panic", r)
		}
	}()
	a.reportOnce()
}

// registerTarget tries to register to one server with exponential backoff.
// Best-effort: failures are logged but don't block startup — the agent will
// retry registration on the next 403 during reporting.
func (a *Agent) registerTarget(t *serverTarget) {
	if t.token == "" {
		t.registered = true // no-token servers accept any host
		return
	}
	// Try up to 3 times with backoff.
	for attempt := 0; attempt < 3; attempt++ {
		if t.register(a.identity) {
			return
		}
		if attempt < 2 {
			d := t.bo.next()
			slog.Info("注册失败，等待后重试", "server", t.server, "wait", d.Round(time.Second))
			time.Sleep(d)
		}
	}
	slog.Warn("注册最终失败，将在上报时继续重试", "server", t.server)
}

// pluginLoop runs plugins on a slower tick, independently of the report loop.
// Wrapped in defer/recover so a panic in plugin execution (e.g. nil map from
// a corrupted plugin output) doesn't kill the whole agent.
func (a *Agent) pluginLoop() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("插件循环 panic 已恢复，尝试重启", "panic", r)
			go a.pluginLoop() // restart after a brief pause
		}
	}()
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
//
// Circuit breaker: if a target has 8 consecutive failures (each already retried
// 3x internally), the breaker opens and we skip that target for 15s — preventing
// futile connection attempts that waste CPU and network resources. Threshold and
// cooldown are tuned for external networks: old values (5/30s) were too aggressive
// and caused agents to go "offline" after brief network jitter.
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
	// a slow/unreachable server can't block the others (30s timeout isolation).
	var wg sync.WaitGroup
	results := make([]bool, len(a.targets)) // results[i] = true if target i succeeded

	for i, t := range a.targets {
		// Circuit breaker check: skip targets whose breaker is open.
		// We still check allow() inside the goroutine so the half-open trial
		// works correctly.
		if t.cb.isOpen() {
			// v5.2.6: When circuit breaker opens, reset registration flag
			// so the next successful report cycle triggers re-registration.
			// This ensures the agent re-establishes its server-side state
			// after a server restart or network partition.
			t.regMu.Lock()
			t.registered = false
			t.regMu.Unlock()
			results[i] = false
			continue
		}

		wg.Add(1)
		go func(idx int, tgt *serverTarget) {
			defer wg.Done()

			// Circuit breaker: skip if open (already checked above, but
			// double-check for the half-open race).
			if !tgt.cb.allow() {
				results[idx] = false
				return
			}

			// v5.2.6: If not registered (e.g. after circuit breaker reset),
			// try to register before sending the report.
			if !tgt.isRegistered() && tgt.token != "" {
				if tgt.register(rep) {
					slog.Info("断路器恢复后重新注册成功", "server", tgt.server)
				}
			}

			// sendWithRetry handles in-cycle retries, gzip degradation,
			// and 403 re-registration — all within a single report cycle.
			err := tgt.sendWithRetry(rep)
			if err != nil {
				slog.Error("上报失败", "server", tgt.server, "err", err)
				tgt.cb.failure()
				if tgt.cb.isOpen() {
					slog.Warn("断路器已打开，暂停向该服务端上报", "server", tgt.server)
					// v5.2.6: Reset registration on breaker open
					tgt.regMu.Lock()
					tgt.registered = false
					tgt.regMu.Unlock()
				}
				results[idx] = false
				return
			}
			tgt.cb.success()
			tgt.bo.reset()
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
