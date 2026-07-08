package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"sync"
	"time"

	"aiops-monitor/shared"
)

// errForbidden signals that the server rejected a report with 403 (host not
// registered or fingerprint not bound). reportOnce reacts by re-registering.
var errForbidden = errors.New("forbidden")

// Agent ties together the native collector (fast base metrics) and the plugin
// runner (slower custom/AI layer), then reports both to the backend.
type Agent struct {
	server         string
	reportInterval time.Duration
	pluginInterval time.Duration
	collector      Collector
	plugins        *PluginRunner
	identity       shared.Report // template with host fields pre-filled
	httpc          *http.Client

	mu            sync.Mutex
	latestCustom  map[string]float64
	pendingEvents []shared.Event
	latestBase    *shared.Metrics // from a core plugin, used when native unsupported
}

func NewAgent(server string, reportInterval, pluginInterval time.Duration,
	collector Collector, plugins *PluginRunner, hostID, category, token string) *Agent {
	return &Agent{
		server:         server,
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
			Token:       token,
			Fingerprint: machineFingerprint(),
		},
	}
}

func (a *Agent) Run() {
	log.Printf("Agent 核心启动 | host=%s | os=%s | 采集器=%s | id=%s",
		a.identity.Hostname, a.identity.OS, a.collector.Name(), short(a.identity.HostID))
	log.Printf("服务端=%s | 基础上报=%s | 插件周期=%s", a.server, a.reportInterval, a.pluginInterval)
	if !a.collector.Supported() {
		log.Printf("提示: 当前平台无原生采集器，基础指标依赖 core 插件(plugins/core_metrics.py)")
	}

	a.register()
	go a.pluginLoop()          // Python layer, lower frequency
	go a.runTerminalChannel()  // remote terminal reverse channel (免开入站端口)

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
	res := a.plugins.RunAll(log.Printf)
	a.mu.Lock()
	if len(res.custom) > 0 {
		a.latestCustom = res.custom
	}
	if res.base != nil {
		a.latestBase = res.base
	}
	if len(res.events) > 0 {
		a.pendingEvents = append(a.pendingEvents, res.events...)
		log.Printf("插件产生 %d 条事件", len(res.events))
	}
	a.mu.Unlock()
}

func (a *Agent) reportOnce() {
	var base shared.Metrics
	if a.collector.Supported() {
		m, err := a.collector.Collect()
		if err != nil {
			log.Printf("原生采集失败: %v", err)
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

	rep := a.identity
	rep.Metrics = base
	if len(custom) > 0 {
		rep.Custom = custom
	}
	rep.Events = events

	err := a.send(rep)
	if err == errForbidden {
		// Server rejected the report — host unregistered or fingerprint not bound
		// (e.g. first run after a server-side reset, or the host record predates
		// fingerprint binding). Re-register to bind the fingerprint, then retry.
		log.Printf("上报被拒(指纹未绑定)，重新注册后重试")
		if a.register() {
			err = a.send(rep)
		} else {
			err = fmt.Errorf("注册失败，跳过本次上报重试")
		}
	}
	if err != nil {
		log.Printf("上报失败: %v", err)
		if len(events) > 0 { // re-queue drained events so they aren't lost
			a.mu.Lock()
			a.pendingEvents = append(events, a.pendingEvents...)
			a.mu.Unlock()
		}
		return
	}
	log.Printf("上报成功  CPU %.1f%%  内存 %.1f%%  磁盘 %.1f%%  自定义指标 %d  事件 %d",
		base.CPUPercent, base.MemPercent, base.DiskPercent, len(custom), len(events))
}

func (a *Agent) register() bool {
	body, _ := json.Marshal(map[string]string{
		"host_id":     a.identity.HostID,
		"hostname":    a.identity.Hostname,
		"token":       a.identity.Token,
		"fingerprint": a.identity.Fingerprint,
	})
	resp, err := a.httpc.Post(a.server+"/api/v1/agent/register", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("注册失败(将继续上报): %v", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("注册被拒(状态码 %d)，可能 Token 已失效或指纹无效", resp.StatusCode)
		return false
	}
	log.Printf("已向服务端注册")
	return true
}

func (a *Agent) send(rep shared.Report) error {
	body, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	resp, err := a.httpc.Post(a.server+"/api/v1/agent/report", "application/json", bytes.NewReader(body))
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

func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
