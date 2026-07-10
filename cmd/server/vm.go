package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"aiops-monitor/shared"
)

// ============================================================================
// VictoriaMetrics integration (optional, enabled via AIOPS_VM_URL).
//
// When a VM URL is configured, every host report is also pushed to VM in the
// Prometheus text exposition format via /api/v1/import/prometheus — stdlib HTTP
// only, no protobuf/snappy. This offloads long-term / large-scale time-series to
// a purpose-built TSDB (the same store Nightingale uses) while the embedded
// tiered store keeps serving the built-in dashboards. Pushes are batched and
// fire-and-forget so agent ingest never blocks on VM.
// ============================================================================

// VMConfig configures the optional VictoriaMetrics writer.
type VMConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"` // e.g. http://victoriametrics:8428
}

type vmSample struct {
	hostID, hostname, category string
	ts                         int64
	m                          shared.Metrics
}

type vmWriter struct {
	cfg   *ConfigStore
	ch    chan vmSample
	httpc *http.Client
}

func newVMWriter(cfg *ConfigStore) *vmWriter {
	return &vmWriter{cfg: cfg, ch: make(chan vmSample, 8192), httpc: &http.Client{Timeout: 15 * time.Second}}
}

// enqueue queues one sample for VM (no-op + non-blocking when VM is disabled or
// the buffer is full — VM must never slow down ingest).
func (v *vmWriter) enqueue(hostID, hostname, category string, ts int64, m shared.Metrics) {
	if v == nil {
		return
	}
	c := v.cfg.VMConfig()
	if !c.Enabled || c.URL == "" {
		return
	}
	select {
	case v.ch <- vmSample{hostID, hostname, category, ts, m}:
	default: // drop on overflow rather than block
	}
}

// run batches queued samples and pushes them to VM every few seconds.
func (v *vmWriter) run() {
	buf := make([]vmSample, 0, 512)
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	flush := func() {
		if len(buf) == 0 {
			return
		}
		if c := v.cfg.VMConfig(); c.Enabled && c.URL != "" {
			v.push(c.URL, buf)
		}
		buf = buf[:0]
	}
	for {
		select {
		case s := <-v.ch:
			buf = append(buf, s)
			if len(buf) >= 512 {
				flush()
			}
		case <-t.C:
			flush()
		}
	}
}

// lblEsc escapes a Prometheus label value.
func lblEsc(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", " ").Replace(s)
}

// push formats the samples as Prometheus text and imports them into VM.
func (v *vmWriter) push(url string, samples []vmSample) {
	var b strings.Builder
	for _, s := range samples {
		lbl := fmt.Sprintf(`host="%s",instance="%s"`, lblEsc(s.hostID), lblEsc(s.hostname))
		if s.category != "" {
			lbl += fmt.Sprintf(`,category="%s"`, lblEsc(s.category))
		}
		ms := s.ts * 1000
		w := func(name string, val float64) { fmt.Fprintf(&b, "aiops_%s{%s} %g %d\n", name, lbl, val, ms) }
		w("cpu_percent", s.m.CPUPercent)
		w("mem_percent", s.m.MemPercent)
		w("mem_used_bytes", float64(s.m.MemUsed))
		w("swap_percent", s.m.SwapPercent)
		w("disk_percent", s.m.DiskPercent)
		w("disk_io_util_percent", s.m.DiskIOUtilPercent)
		w("disk_read_rate", s.m.DiskReadRate)
		w("disk_write_rate", s.m.DiskWriteRate)
		w("disk_read_iops", s.m.DiskReadIOPS)
		w("disk_write_iops", s.m.DiskWriteIOPS)
		w("net_sent_rate", s.m.NetSentRate)
		w("net_recv_rate", s.m.NetRecvRate)
		w("net_conns", float64(s.m.NetConns))
		w("load1", s.m.Load1)
		w("load5", s.m.Load5)
		w("load15", s.m.Load15)
		w("proc_count", float64(s.m.ProcCount))
		for _, g := range s.m.GPUs {
			gl := lbl + fmt.Sprintf(`,gpu="%s"`, lblEsc(g.Name))
			fmt.Fprintf(&b, "aiops_gpu_util_percent{%s} %g %d\n", gl, g.UtilPercent, ms)
		}
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(url, "/")+"/api/v1/import/prometheus", strings.NewReader(b.String()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := v.httpc.Do(req)
	if err != nil {
		slog.Warn("VictoriaMetrics 写入失败", "err", err)
		return
	}
	resp.Body.Close()
}
