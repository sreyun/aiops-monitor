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

// vmAPISample 是一次 API 性能监控探测结果，排队持久化到 VM（aiops_api_* 指标族）。
type vmAPISample struct {
	apiID, system, endpoint     string
	ts                          int64
	ok                          bool
	latencyMs                   float64
	statusCode                  int
	dnsMs, tcpMs, tlsMs, ttfbMs float64
	certDays                    int
	respBytes                   int64
}

type vmWriter struct {
	cfg     *ConfigStore
	ch      chan vmSample
	checkCh chan vmCheckSample
	apiCh   chan vmAPISample
	httpc   *http.Client
}

func newVMWriter(cfg *ConfigStore) *vmWriter {
	return &vmWriter{cfg: cfg, ch: make(chan vmSample, 8192), checkCh: make(chan vmCheckSample, 4096), apiCh: make(chan vmAPISample, 4096), httpc: &http.Client{Timeout: 15 * time.Second}}
}

// enqueueAPI 排队一次 API 探测结果到 VM（VM 未启用或缓冲满时非阻塞丢弃）。
func (v *vmWriter) enqueueAPI(s vmAPISample) {
	if v == nil {
		return
	}
	c := v.cfg.VMConfig()
	if !c.Enabled || c.URL == "" {
		return
	}
	select {
	case v.apiCh <- s:
	default:
	}
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
	abuf := make([]vmAPISample, 0, 256)
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	flush := func() {
		if len(buf) == 0 && len(cbuf) == 0 && len(abuf) == 0 {
			return
		}
		if c := v.cfg.VMConfig(); c.Enabled && c.URL != "" {
			if len(buf) > 0 {
				v.push(c.URL, buf)
			}
			if len(cbuf) > 0 {
				v.pushChecks(c.URL, cbuf)
			}
			if len(abuf) > 0 {
				v.pushAPI(c.URL, abuf)
			}
		}
		buf = buf[:0]
		cbuf = cbuf[:0]
		abuf = abuf[:0]
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
		case as := <-v.apiCh:
			abuf = append(abuf, as)
			if len(abuf) >= 256 {
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

// pushAPI 把 API 性能监控探测结果批量写入 VM（Prometheus 文本格式）。
// 指标：aiops_api_up(1/0) / _latency_ms / _status_code / _dns_ms / _tcp_ms /
// _tls_ms / _ttfb_ms / _cert_days / _resp_bytes，label 含 api_id/system/endpoint。
func (v *vmWriter) pushAPI(url string, samples []vmAPISample) {
	var b strings.Builder
	for _, s := range samples {
		lbl := fmt.Sprintf(`api_id="%s",system="%s",endpoint="%s"`, lblEsc(s.apiID), lblEsc(s.system), lblEsc(s.endpoint))
		ms := s.ts * 1000
		up := 0.0
		if s.ok {
			up = 1
		}
		fmt.Fprintf(&b, "aiops_api_up{%s} %g %d\n", lbl, up, ms)
		fmt.Fprintf(&b, "aiops_api_latency_ms{%s} %g %d\n", lbl, s.latencyMs, ms)
		if s.statusCode > 0 {
			fmt.Fprintf(&b, "aiops_api_status_code{%s} %d %d\n", lbl, s.statusCode, ms)
		}
		if s.dnsMs > 0 {
			fmt.Fprintf(&b, "aiops_api_dns_ms{%s} %g %d\n", lbl, s.dnsMs, ms)
		}
		if s.tcpMs > 0 {
			fmt.Fprintf(&b, "aiops_api_tcp_ms{%s} %g %d\n", lbl, s.tcpMs, ms)
		}
		if s.tlsMs > 0 {
			fmt.Fprintf(&b, "aiops_api_tls_ms{%s} %g %d\n", lbl, s.tlsMs, ms)
		}
		if s.ttfbMs > 0 {
			fmt.Fprintf(&b, "aiops_api_ttfb_ms{%s} %g %d\n", lbl, s.ttfbMs, ms)
		}
		if s.certDays >= 0 {
			fmt.Fprintf(&b, "aiops_api_cert_days{%s} %d %d\n", lbl, s.certDays, ms)
		}
		if s.respBytes > 0 {
			fmt.Fprintf(&b, "aiops_api_resp_bytes{%s} %d %d\n", lbl, s.respBytes, ms)
		}
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(url, "/")+"/api/v1/import/prometheus", strings.NewReader(b.String()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := v.httpc.Do(req)
	if err != nil {
		slog.Warn("VictoriaMetrics 写入 API 监控数据失败", "err", err)
		return
	}
	resp.Body.Close()
}

// queryAPIHistory 从 VM 读取某接口在 [from,to] 的探测序列，重组为 []CheckPoint（历史曲线）。
func (v *vmWriter) queryAPIHistory(apiID string, from, to int64) []CheckPoint {
	c := v.cfg.VMConfig()
	if !c.Enabled || c.URL == "" {
		return nil
	}
	q := url.Values{
		"match[]": {fmt.Sprintf(`{api_id=%q,__name__=~"aiops_api_.*"}`, apiID)},
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
	return parseVMAPIExport(resp.Body)
}

// parseVMAPIExport 把 VM /export 的 NDJSON 按时间戳重组为 []CheckPoint（aiops_api_* 指标族）。
func parseVMAPIExport(r io.Reader) []CheckPoint {
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
			case "aiops_api_up":
				p.OK = line.Values[i] >= 0.5
			case "aiops_api_latency_ms":
				p.LatencyMs = line.Values[i]
			case "aiops_api_status_code":
				p.StatusCode = int(line.Values[i])
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

// apiAggregate 是一个接口由 VM 现算的性能聚合（平均/ P95 响应时间、1h/24h 可用率、1h 采样数）。
type apiAggregate struct {
	AvgMs     float64 `json:"avg_ms"`
	P95Ms     float64 `json:"p95_ms"`
	Avail1h   float64 `json:"avail_1h"`  // 百分比
	Avail24h  float64 `json:"avail_24h"` // 百分比
	Samples1h float64 `json:"samples_1h"`
}

// vmInstantByAPI 执行一次 PromQL 瞬时查询，返回 api_id -> 数值（VM 侧现算聚合）。
func (v *vmWriter) vmInstantByAPI(promql string) map[string]float64 {
	c := v.cfg.VMConfig()
	if !c.Enabled || c.URL == "" {
		return nil
	}
	q := url.Values{"query": {promql}}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(c.URL, "/")+"/api/v1/query?"+q.Encode(), nil)
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
	var out struct {
		Data struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Value  []any             `json:"value"` // [ts, "strval"]
			} `json:"result"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil {
		return nil
	}
	m := map[string]float64{}
	for _, r := range out.Data.Result {
		id := r.Metric["api_id"]
		if id == "" || len(r.Value) < 2 {
			continue
		}
		s, _ := r.Value[1].(string)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			m[id] = f
		}
	}
	return m
}

// queryAPIAggregate 用 5 次 PromQL 瞬时查询算出所有接口的聚合，按 api_id 归并返回。
// 一次查询即覆盖全部接口（VM 按 api_id label 返回多条结果），与接口数量无关。
func (v *vmWriter) queryAPIAggregate() map[string]apiAggregate {
	if !v.enabled() {
		return map[string]apiAggregate{}
	}
	avg := v.vmInstantByAPI(`avg_over_time(aiops_api_latency_ms[1h])`)
	p95 := v.vmInstantByAPI(`quantile_over_time(0.95, aiops_api_latency_ms[1h])`)
	a1 := v.vmInstantByAPI(`avg_over_time(aiops_api_up[1h]) * 100`)
	a24 := v.vmInstantByAPI(`avg_over_time(aiops_api_up[24h]) * 100`)
	cnt := v.vmInstantByAPI(`count_over_time(aiops_api_up[1h])`)
	out := map[string]apiAggregate{}
	get := func(m map[string]float64, id string) float64 {
		if m == nil {
			return 0
		}
		return m[id]
	}
	seen := map[string]bool{}
	for _, m := range []map[string]float64{avg, p95, a1, a24, cnt} {
		for id := range m {
			seen[id] = true
		}
	}
	for id := range seen {
		out[id] = apiAggregate{
			AvgMs: get(avg, id), P95Ms: get(p95, id),
			Avail1h: get(a1, id), Avail24h: get(a24, id), Samples1h: get(cnt, id),
		}
	}
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
		for _, d := range s.m.Disks {
			dl := lbl + fmt.Sprintf(`,path="%s"`, lblEsc(d.Path))
			fmt.Fprintf(&b, "aiops_disk_vol_percent{%s} %g %d\n", dl, d.Percent, ms)
			fmt.Fprintf(&b, "aiops_disk_vol_used_bytes{%s} %g %d\n", dl, float64(d.Used), ms)
			fmt.Fprintf(&b, "aiops_disk_vol_total_bytes{%s} %g %d\n", dl, float64(d.Total), ms)
		}
		for _, g := range s.m.GPUs {
			gl := lbl + fmt.Sprintf(`,gpu="%s"`, lblEsc(g.Name))
			fmt.Fprintf(&b, "aiops_gpu_util_percent{%s} %g %d\n", gl, g.UtilPercent, ms)
			fmt.Fprintf(&b, "aiops_gpu_temp_c{%s} %g %d\n", gl, g.Temp, ms)
			fmt.Fprintf(&b, "aiops_gpu_mem_percent{%s} %g %d\n", gl, g.MemPercent, ms)
			fmt.Fprintf(&b, "aiops_gpu_mem_used_bytes{%s} %g %d\n", gl, float64(g.MemUsed), ms)
			fmt.Fprintf(&b, "aiops_gpu_mem_free_bytes{%s} %g %d\n", gl, float64(g.MemFree), ms)
			fmt.Fprintf(&b, "aiops_gpu_mem_total_bytes{%s} %g %d\n", gl, float64(g.MemTotal), ms)
		}
		// 每 (协议,状态) 一条连接计数序列，支撑「连接数 / 会话状态」趋势图
		for _, c := range s.m.Conns {
			cl := lbl + fmt.Sprintf(`,proto="%s",state="%s"`, lblEsc(c.Proto), lblEsc(c.State))
			fmt.Fprintf(&b, "aiops_net_conn_count{%s} %g %d\n", cl, float64(c.Count), ms)
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

// setSampleGPU 把一条带 gpu 标签的 aiops_gpu_* 系列（按显卡名区分）并回该时间点样本的 GPUs
// 数组，按名重建每块显卡的 利用率/温度/显存 各字段。VM 里每块显卡是带 gpu 标签的独立系列，
// parseVMExport 必须按名重建，否则从 VM 读回的历史样本永远缺 gpus，前端趋势画不出 GPU 图。
func setSampleGPU(s *shared.Sample, gpuName, name string, val float64) {
	if gpuName == "" {
		gpuName = "GPU"
	}
	idx := -1
	for i := range s.GPUs {
		if s.GPUs[i].Name == gpuName {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.GPUs = append(s.GPUs, shared.GPUInfo{Name: gpuName})
		idx = len(s.GPUs) - 1
	}
	switch name {
	case "aiops_gpu_util_percent":
		s.GPUs[idx].UtilPercent = val
	case "aiops_gpu_temp_c":
		s.GPUs[idx].Temp = val
	case "aiops_gpu_mem_percent":
		s.GPUs[idx].MemPercent = val
	case "aiops_gpu_mem_used_bytes":
		s.GPUs[idx].MemUsed = uint64(val)
	case "aiops_gpu_mem_free_bytes":
		s.GPUs[idx].MemFree = uint64(val)
	case "aiops_gpu_mem_total_bytes":
		s.GPUs[idx].MemTotal = uint64(val)
	}
}

// setSampleConn 把一条带 proto+state 标签的 aiops_net_conn_count 系列并回样本的 Conns 数组，
// 按 (协议,状态) 重建，支撑「连接数 / 会话状态」趋势图。
func setSampleConn(s *shared.Sample, proto, state string, val float64) {
	if proto == "" {
		return
	}
	for i := range s.Conns {
		if s.Conns[i].Proto == proto && s.Conns[i].State == state {
			s.Conns[i].Count = int(val)
			return
		}
	}
	s.Conns = append(s.Conns, shared.ConnStat{Proto: proto, State: state, Count: int(val)})
}

// setSampleDisk 把一条带 path 标签的 aiops_disk_vol_* 系列并回该时间点样本的 Disks 数组，
// 按分区路径重建（每个分区的 percent/used/total）。VM 里多盘是带 path 标签的独立系列，不
// 重建则历史样本缺 disks，前端「近期趋势」只剩一条聚合根分区线（本次修复的 bug 点）。
func setSampleDisk(s *shared.Sample, path, name string, val float64) {
	if path == "" {
		return
	}
	idx := -1
	for i := range s.Disks {
		if s.Disks[i].Path == path {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.Disks = append(s.Disks, shared.DiskInfo{Path: path})
		idx = len(s.Disks) - 1
	}
	switch name {
	case "aiops_disk_vol_percent":
		s.Disks[idx].Percent = val
	case "aiops_disk_vol_used_bytes":
		s.Disks[idx].Used = uint64(val)
	case "aiops_disk_vol_total_bytes":
		s.Disks[idx].Total = uint64(val)
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
		gpuName := line.Metric["gpu"]     // GPU 系列带 gpu 标签（每块显卡一条），需按名重建 s.GPUs
		diskPath := line.Metric["path"]   // 磁盘分区系列带 path 标签（每个分区一条），需按路径重建 s.Disks
		connProto := line.Metric["proto"] // 连接计数系列带 proto+state 标签，需按 (协议,状态) 重建 s.Conns
		connState := line.Metric["state"]
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
			if strings.HasPrefix(name, "aiops_gpu_") {
				setSampleGPU(s, gpuName, name, line.Values[i])
			} else if strings.HasPrefix(name, "aiops_disk_vol_") {
				setSampleDisk(s, diskPath, name, line.Values[i])
			} else if name == "aiops_net_conn_count" {
				setSampleConn(s, connProto, connState, line.Values[i])
			} else {
				setSampleMetric(s, name, line.Values[i])
			}
		}
	}
	out := make([]shared.Sample, 0, len(byTs))
	for _, s := range byTs {
		if len(s.Disks) > 1 { // 分区按 path 排序，保证跨样本顺序稳定
			sort.Slice(s.Disks, func(a, b int) bool { return s.Disks[a].Path < s.Disks[b].Path })
		}
		if len(s.Conns) > 1 { // 连接按 proto+state 排序，保证跨样本顺序稳定
			sort.Slice(s.Conns, func(a, b int) bool {
				if s.Conns[a].Proto != s.Conns[b].Proto {
					return s.Conns[a].Proto < s.Conns[b].Proto
				}
				return s.Conns[a].State < s.Conns[b].State
			})
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })
	return out
}

// ============================================================================
// Hardware + NetFlow VM write helpers
// ============================================================================

// pushHardware writes one hardware metric to VM immediately (fire-and-forget).
// 标签值一律走 lblEsc：target / 传感器名等来自 Agent（乃至 BMC）上报，未转义时一个
// 形如 `a"} evil{x="` 的传感器名就能凭空造出/污染其它序列。
func (v *vmWriter) pushHardware(hostID, target string, ts int64, metric string, val float64) {
	v.pushRawLine(fmt.Sprintf(`%s{host="%s",target="%s"} %f %d`, metric, lblEsc(hostID), lblEsc(target), val, ts))
}

// pushHardwareLabeled writes one hardware metric with an extra label.
func (v *vmWriter) pushHardwareLabeled(hostID, target string, ts int64, metric string, val float64, extraKey, extraVal string) {
	v.pushRawLine(fmt.Sprintf(`%s{host="%s",target="%s",%s="%s"} %f %d`,
		metric, lblEsc(hostID), lblEsc(target), extraKey, lblEsc(extraVal), val, ts))
}

// pushRawLine writes one Prometheus text line directly to VM (fire-and-forget).
// Used by hardware/netflow metrics that don't fit the standard sample pipeline.
func (v *vmWriter) pushRawLine(line string) {
	if v == nil || !v.enabled() {
		return
	}
	c := v.cfg.VMConfig()
	go func() {
		body := line + "\n"
		req, err := http.NewRequest("POST", c.URL+"/api/v1/import/prometheus", strings.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "text/plain")
		resp, err := v.httpc.Do(req)
		if err != nil {
			return
		}
		resp.Body.Close()
	}()
}

// queryRawRange executes a range query against VM and returns raw results.
func (v *vmWriter) queryRawRange(promql string, from, to int64) []any {
	c := v.cfg.VMConfig()
	if !c.Enabled || c.URL == "" {
		return nil
	}
	u := fmt.Sprintf("%s/api/v1/query_range?query=%s&start=%d&end=%d&step=60",
		c.URL, url.QueryEscape(promql), from, to)
	resp, err := v.httpc.Get(u)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var result struct {
		Data struct {
			Result []any `json:"result"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&result) != nil {
		return nil
	}
	return result.Data.Result
}
