package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
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

// vmCheckSample 是一次自定义拨测/接口探测结果，排队持久化到 VM（重启不丢，可查历史趋势）。
type vmCheckSample struct {
	checkID, name, checkType    string
	ts                          int64
	ok                          bool
	latencyMs                   float64
	statusCode                  int
	lossPct                     float64
	dnsMs, tcpMs, tlsMs, ttfbMs float64 // HTTP 高级模式分段计时（0=未测）
	certDays                    int     // 证书剩余天数（-1=非 HTTPS/未知）
	respBytes                   int64   // 响应体大小（0=未记）
}

type vmWriter struct {
	cfg     *ConfigStore
	ch      chan vmSample
	checkCh chan vmCheckSample
	httpc   *http.Client
}

func newVMWriter(cfg *ConfigStore) *vmWriter {
	return &vmWriter{cfg: cfg, ch: make(chan vmSample, 8192), checkCh: make(chan vmCheckSample, 4096), httpc: &http.Client{Timeout: 15 * time.Second}}
}

// enqueueCheck 排队一次拨测结果到 VM（VM 未启用或缓冲满时非阻塞丢弃）。
func (v *vmWriter) enqueueCheck(cs vmCheckSample) {
	if v == nil {
		return
	}
	c := v.cfg.VMConfig()
	if !c.Enabled || c.URL == "" {
		return
	}
	select {
	case v.checkCh <- cs:
	default:
	}
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
	cbuf := make([]vmCheckSample, 0, 256)
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	flush := func() {
		if len(buf) == 0 && len(cbuf) == 0 {
			return
		}
		if c := v.cfg.VMConfig(); c.Enabled && c.URL != "" {
			if len(buf) > 0 {
				v.push(c.URL, buf)
			}
			if len(cbuf) > 0 {
				v.pushChecks(c.URL, cbuf)
			}
		}
		buf = buf[:0]
		cbuf = cbuf[:0]
	}
	for {
		select {
		case s := <-v.ch:
			buf = append(buf, s)
			if len(buf) >= 512 {
				flush()
			}
		case cs := <-v.checkCh:
			cbuf = append(cbuf, cs)
			if len(cbuf) >= 256 {
				flush()
			}
		case <-t.C:
			flush()
		}
	}
}

// pushChecks 把拨测结果批量写入 VM（Prometheus 文本格式）。
// 指标：aiops_check_up(1/0) / _latency_ms / _status_code / _loss_pct，label 含 check_id/check_type/name。
func (v *vmWriter) pushChecks(url string, samples []vmCheckSample) {
	var b strings.Builder
	for _, s := range samples {
		lbl := fmt.Sprintf(`check_id="%s",check_type="%s",name="%s"`, lblEsc(s.checkID), lblEsc(s.checkType), lblEsc(s.name))
		ms := s.ts * 1000
		up := 0.0
		if s.ok {
			up = 1
		}
		fmt.Fprintf(&b, "aiops_check_up{%s} %g %d\n", lbl, up, ms)
		fmt.Fprintf(&b, "aiops_check_latency_ms{%s} %g %d\n", lbl, s.latencyMs, ms)
		if s.statusCode > 0 {
			fmt.Fprintf(&b, "aiops_check_status_code{%s} %d %d\n", lbl, s.statusCode, ms)
		}
		if s.lossPct >= 0 {
			fmt.Fprintf(&b, "aiops_check_loss_pct{%s} %g %d\n", lbl, s.lossPct, ms)
		}
		if s.dnsMs > 0 {
			fmt.Fprintf(&b, "aiops_check_dns_ms{%s} %g %d\n", lbl, s.dnsMs, ms)
		}
		if s.tcpMs > 0 {
			fmt.Fprintf(&b, "aiops_check_tcp_ms{%s} %g %d\n", lbl, s.tcpMs, ms)
		}
		if s.tlsMs > 0 {
			fmt.Fprintf(&b, "aiops_check_tls_ms{%s} %g %d\n", lbl, s.tlsMs, ms)
		}
		if s.ttfbMs > 0 {
			fmt.Fprintf(&b, "aiops_check_ttfb_ms{%s} %g %d\n", lbl, s.ttfbMs, ms)
		}
		if s.certDays >= 0 {
			fmt.Fprintf(&b, "aiops_check_cert_days{%s} %d %d\n", lbl, s.certDays, ms)
		}
		if s.respBytes > 0 {
			fmt.Fprintf(&b, "aiops_check_resp_bytes{%s} %d %d\n", lbl, s.respBytes, ms)
		}
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(url, "/")+"/api/v1/import/prometheus", strings.NewReader(b.String()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := v.httpc.Do(req)
	if err != nil {
		slog.Warn("VictoriaMetrics 写入拨测数据失败", "err", err)
		return
	}
	resp.Body.Close()
}

// queryCheckHistory 从 VM 读取某拨测在 [from,to] 的结果序列，重组为 []CheckPoint（重启后仍可查历史）。
func (v *vmWriter) queryCheckHistory(checkID string, from, to int64) []CheckPoint {
	c := v.cfg.VMConfig()
	if !c.Enabled || c.URL == "" {
		return nil
	}
	q := url.Values{
		"match[]": {fmt.Sprintf(`{check_id=%q,__name__=~"aiops_check_.*"}`, checkID)},
		"start":   {strconv.FormatInt(from, 10)},
		"end":     {strconv.FormatInt(to, 10)},
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(c.URL, "/")+"/api/v1/export?"+q.Encode(), nil)
	if err != nil {
		return nil
	}
	resp, err := v.httpc.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	return parseVMCheckExport(resp.Body)
}

// parseVMCheckExport 把 VM /export 的 NDJSON（每行一条 series）按时间戳重组为 []CheckPoint。
func parseVMCheckExport(r io.Reader) []CheckPoint {
	byTs := map[int64]*CheckPoint{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16<<20)
	for sc.Scan() {
		var line struct {
			Metric     map[string]string `json:"metric"`
			Values     []float64         `json:"values"`
			Timestamps []int64           `json:"timestamps"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		name := line.Metric["__name__"]
		for i := range line.Values {
			if i >= len(line.Timestamps) {
				break
			}
			ts := line.Timestamps[i] / 1000
			p := byTs[ts]
			if p == nil {
				p = &CheckPoint{Ts: ts, LossPct: -1}
				byTs[ts] = p
			}
			switch name {
			case "aiops_check_up":
				p.OK = line.Values[i] >= 0.5
			case "aiops_check_latency_ms":
				p.LatencyMs = line.Values[i]
			case "aiops_check_status_code":
				p.StatusCode = int(line.Values[i])
			case "aiops_check_loss_pct":
				p.LossPct = line.Values[i]
			}
		}
	}
	out := make([]CheckPoint, 0, len(byTs))
	for _, p := range byTs {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ts < out[j].Ts })
	return out
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
		w("cpu_cores", float64(s.m.CPUCores))
		w("mem_percent", s.m.MemPercent)
		w("mem_used_bytes", float64(s.m.MemUsed))
		w("mem_total_bytes", float64(s.m.MemTotal))
		w("swap_percent", s.m.SwapPercent)
		w("swap_used_bytes", float64(s.m.SwapUsed))
		w("swap_total_bytes", float64(s.m.SwapTotal))
		w("disk_percent", s.m.DiskPercent)
		w("disk_used_bytes", float64(s.m.DiskUsed))
		w("disk_total_bytes", float64(s.m.DiskTotal))
		w("uptime_seconds", float64(s.m.Uptime))
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

// enabled reports whether VM is the active time-series store.
func (v *vmWriter) enabled() bool {
	if v == nil {
		return false
	}
	c := v.cfg.VMConfig()
	return c.Enabled && c.URL != ""
}

// setSampleMetric writes one VM series value into the matching Sample field.
func setSampleMetric(s *shared.Sample, name string, val float64) {
	switch strings.TrimPrefix(name, "aiops_") {
	case "cpu_percent":
		s.CPUPercent = val
	case "cpu_cores":
		s.CPUCores = int(val)
	case "mem_percent":
		s.MemPercent = val
	case "mem_used_bytes":
		s.MemUsed = uint64(val)
	case "mem_total_bytes":
		s.MemTotal = uint64(val)
	case "swap_percent":
		s.SwapPercent = val
	case "swap_used_bytes":
		s.SwapUsed = uint64(val)
	case "swap_total_bytes":
		s.SwapTotal = uint64(val)
	case "disk_percent":
		s.DiskPercent = val
	case "disk_used_bytes":
		s.DiskUsed = uint64(val)
	case "disk_total_bytes":
		s.DiskTotal = uint64(val)
	case "uptime_seconds":
		s.Uptime = uint64(val)
	case "disk_io_util_percent":
		s.DiskIOUtilPercent = val
	case "disk_read_rate":
		s.DiskReadRate = val
	case "disk_write_rate":
		s.DiskWriteRate = val
	case "disk_read_iops":
		s.DiskReadIOPS = val
	case "disk_write_iops":
		s.DiskWriteIOPS = val
	case "net_sent_rate":
		s.NetSentRate = val
	case "net_recv_rate":
		s.NetRecvRate = val
	case "net_conns":
		s.NetConns = int(val)
	case "load1":
		s.Load1 = val
	case "load5":
		s.Load5 = val
	case "load15":
		s.Load15 = val
	case "proc_count":
		s.ProcCount = int(val)
	}
}

// queryHistory reads a host's series back from VM (the authoritative time-series
// store) over [from,to] and reassembles []shared.Sample keyed by timestamp.
func (v *vmWriter) queryHistory(hostID string, from, to int64) ([]shared.Sample, bool) {
	c := v.cfg.VMConfig()
	if !c.Enabled || c.URL == "" {
		return nil, false
	}
	q := url.Values{
		"match[]": {fmt.Sprintf(`{host=%q,__name__=~"aiops_.*"}`, hostID)},
		"start":   {strconv.FormatInt(from, 10)},
		"end":     {strconv.FormatInt(to, 10)},
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(c.URL, "/")+"/api/v1/export?"+q.Encode(), nil)
	if err != nil {
		return nil, false
	}
	resp, err := v.httpc.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	out := parseVMExport(resp.Body)
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// parseVMExport reassembles VM's /api/v1/export NDJSON (one line per series) into
// []shared.Sample joined by timestamp. Split out so it can be unit-tested without
// a live VM.
func parseVMExport(r io.Reader) []shared.Sample {
	byTs := map[int64]*shared.Sample{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16<<20)
	for sc.Scan() {
		var line struct {
			Metric     map[string]string `json:"metric"`
			Values     []float64         `json:"values"`
			Timestamps []int64           `json:"timestamps"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		name := line.Metric["__name__"]
		for i := range line.Values {
			if i >= len(line.Timestamps) {
				break
			}
			ts := line.Timestamps[i] / 1000
			s := byTs[ts]
			if s == nil {
				s = &shared.Sample{Timestamp: ts}
				byTs[ts] = s
			}
			setSampleMetric(s, name, line.Values[i])
		}
	}
	out := make([]shared.Sample, 0, len(byTs))
	for _, s := range byTs {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })
	return out
}
