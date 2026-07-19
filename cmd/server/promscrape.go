package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"aiops-monitor/shared"
)

// ============================================================================
// 指标抓取（agentless）+ Prometheus 生态摄入
//
// 不手写 JVM/MySQL/Redis/中间件采集器，而是「吃 Prometheus exporter 生态」：服务端直接
// 抓取 exporter / 应用原生 /metrics（jmx_exporter、mysqld_exporter、Spring Actuator…），
// 解析 Prometheus 文本 → 带标签样本 → VM。一段代码解锁整个 exporter 生态，且 agentless
// （中间件多不宜装 agent）。复用拨测的 SSRF 守卫 + 加密凭据。remote_write 接收见 promscrape_api.go。
// ============================================================================

// ScrapeTarget 是一个指标抓取目标（一个 exporter / /metrics 端点）。
type ScrapeTarget struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`         // 作为默认 job 标签
	URL         string            `json:"url"`          // exporter /metrics 地址
	IntervalSec int               `json:"interval_sec"` // 抓取周期（秒，最小 5）
	TimeoutSec  int               `json:"timeout_sec,omitempty"`
	Enabled     bool              `json:"enabled"`
	Labels      map[string]string `json:"labels,omitempty"`  // 附加标签（instance/env/自定义），不覆盖样本自带标签
	Headers     map[string]string `json:"headers,omitempty"` // 抓取请求头（含 Authorization 等，静态加密存储）
	CreatedAt   int64             `json:"created_at"`
}

// ---- Prometheus 文本曝露格式解析 ----

// parsePromText 解析 Prometheus 文本曝露格式为带标签样本。忽略 #(HELP/TYPE)/空行/非法行与
// NaN；extra 标签合并进每条样本（不覆盖样本自带同名标签）。tsMs 为统一时间戳。
func parsePromText(body []byte, tsMs int64, extra map[string]string) []shared.LabeledSample {
	var out []shared.LabeledSample
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		name, labels, value, ok := parsePromLine(line)
		if !ok || math.IsNaN(value) {
			continue
		}
		for k, v := range extra {
			if _, exists := labels[k]; !exists {
				if labels == nil {
					labels = map[string]string{}
				}
				labels[k] = v
			}
		}
		out = append(out, shared.LabeledSample{Name: name, Labels: labels, Value: value, TsMs: tsMs})
	}
	return out
}

// parsePromLine 解析一行：name{labels} value [timestamp]。
func parsePromLine(line string) (name string, labels map[string]string, value float64, ok bool) {
	i := 0
	for i < len(line) && line[i] != '{' && line[i] != ' ' && line[i] != '\t' {
		i++
	}
	name = line[:i]
	if name == "" {
		return "", nil, 0, false
	}
	rest := strings.TrimLeft(line[i:], " \t")
	if strings.HasPrefix(rest, "{") {
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			return "", nil, 0, false
		}
		labels = parsePromLabels(rest[1:end])
		rest = rest[end+1:]
	}
	fields := strings.Fields(rest)
	if len(fields) < 1 {
		return "", nil, 0, false
	}
	v, err := parsePromValue(fields[0])
	if err != nil {
		return "", nil, 0, false
	}
	return name, labels, v, true
}

// parsePromLabels 解析 k1="v1",k2="v2"（值带引号，支持 \" \\ \n 转义）。
func parsePromLabels(s string) map[string]string {
	m := map[string]string{}
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == ',') {
			i++
		}
		if i >= len(s) {
			break
		}
		ks := i
		for i < len(s) && s[i] != '=' {
			i++
		}
		if i >= len(s) {
			break
		}
		key := strings.TrimSpace(s[ks:i])
		i++ // '='
		if i >= len(s) || s[i] != '"' {
			break
		}
		i++ // 开引号
		var val strings.Builder
		for i < len(s) {
			if s[i] == '\\' && i+1 < len(s) {
				switch s[i+1] {
				case 'n':
					val.WriteByte('\n')
				case '"':
					val.WriteByte('"')
				case '\\':
					val.WriteByte('\\')
				default:
					val.WriteByte(s[i+1])
				}
				i += 2
			} else if s[i] == '"' {
				i++ // 闭引号
				break
			} else {
				val.WriteByte(s[i])
				i++
			}
		}
		if key != "" {
			m[key] = val.String()
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func parsePromValue(s string) (float64, error) {
	switch s {
	case "+Inf":
		return math.Inf(1), nil
	case "-Inf":
		return math.Inf(-1), nil
	case "NaN":
		return math.NaN(), nil
	}
	return strconv.ParseFloat(s, 64)
}

// ---- ConfigStore：抓取目标 CRUD（与 APISystem 同机制，持久化到 PG/JSON） ----

func (cs *ConfigStore) ScrapeTargets() []ScrapeTarget {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make([]ScrapeTarget, len(cs.cfg.ScrapeTargets))
	copy(out, cs.cfg.ScrapeTargets)
	return out
}

func (cs *ConfigStore) UpsertScrapeTarget(t ScrapeTarget) (ScrapeTarget, error) {
	cs.mu.Lock()
	if t.ID == "" {
		t.ID = genToken()[:8]
		t.CreatedAt = time.Now().Unix()
		cs.cfg.ScrapeTargets = append(cs.cfg.ScrapeTargets, t)
	} else {
		found := false
		for i := range cs.cfg.ScrapeTargets {
			if cs.cfg.ScrapeTargets[i].ID == t.ID {
				t.CreatedAt = cs.cfg.ScrapeTargets[i].CreatedAt
				cs.cfg.ScrapeTargets[i] = t
				found = true
				break
			}
		}
		if !found {
			t.CreatedAt = time.Now().Unix()
			cs.cfg.ScrapeTargets = append(cs.cfg.ScrapeTargets, t)
		}
	}
	cs.mu.Unlock()
	return t, cs.save()
}

// SetPromWriteToken 设置 remote_write 接收令牌（空=禁用接收端点）。
func (cs *ConfigStore) SetPromWriteToken(token string) error {
	cs.mu.Lock()
	cs.cfg.PromWriteToken = token
	cs.mu.Unlock()
	return cs.save()
}

func (cs *ConfigStore) DeleteScrapeTarget(id string) error {
	cs.mu.Lock()
	kept := cs.cfg.ScrapeTargets[:0]
	for _, t := range cs.cfg.ScrapeTargets {
		if t.ID != id {
			kept = append(kept, t)
		}
	}
	cs.cfg.ScrapeTargets = kept
	cs.mu.Unlock()
	return cs.save()
}

// deepCopyScrapeTargets 深拷贝抓取目标（含 Labels/Headers map），供 save 在加密前隔离副本，
// 避免 encryptConfigSecrets 就地污染内存明文配置。
func deepCopyScrapeTargets(in []ScrapeTarget) []ScrapeTarget {
	out := make([]ScrapeTarget, len(in))
	for i, t := range in {
		out[i] = t
		if t.Labels != nil {
			m := make(map[string]string, len(t.Labels))
			for k, v := range t.Labels {
				m[k] = v
			}
			out[i].Labels = m
		}
		if t.Headers != nil {
			m := make(map[string]string, len(t.Headers))
			for k, v := range t.Headers {
				m[k] = v
			}
			out[i].Headers = m
		}
	}
	return out
}

// ---- 抓取管理器 ----

type scrapeStatus struct {
	OK        bool    `json:"ok"`
	Samples   int     `json:"samples"`
	LatencyMs float64 `json:"latency_ms"`
	Msg       string  `json:"msg"`
	CheckedAt int64   `json:"checked_at"`
}

type scrapeManager struct {
	cfg *ConfigStore
	vm  *vmWriter
	sem chan struct{} // 并发抓取限流

	mu      sync.Mutex
	status  map[string]scrapeStatus
	lastRun map[string]time.Time
}

func newScrapeManager(cfg *ConfigStore, vm *vmWriter) *scrapeManager {
	return &scrapeManager{
		cfg: cfg, vm: vm, sem: make(chan struct{}, 8),
		status: map[string]scrapeStatus{}, lastRun: map[string]time.Time{},
	}
}

func (m *scrapeManager) Run(tick time.Duration) {
	t := time.NewTicker(tick)
	defer t.Stop()
	m.sweep()
	for range t.C {
		m.sweep()
	}
}

func (m *scrapeManager) sweep() {
	now := time.Now()
	for _, tg := range m.cfg.ScrapeTargets() {
		if !tg.Enabled || strings.TrimSpace(tg.URL) == "" {
			continue
		}
		iv := tg.IntervalSec
		if iv < 5 {
			iv = 30
		}
		m.mu.Lock()
		last := m.lastRun[tg.ID]
		due := last.IsZero() || now.Sub(last) >= time.Duration(iv)*time.Second
		if due {
			m.lastRun[tg.ID] = now
		}
		m.mu.Unlock()
		if due {
			go m.scrapeLimited(tg)
		}
	}
	m.gc()
}

// gc 清理已删除目标的状态。
func (m *scrapeManager) gc() {
	live := map[string]bool{}
	for _, tg := range m.cfg.ScrapeTargets() {
		live[tg.ID] = true
	}
	m.mu.Lock()
	for id := range m.status {
		if !live[id] {
			delete(m.status, id)
			delete(m.lastRun, id)
		}
	}
	m.mu.Unlock()
}

func (m *scrapeManager) scrapeLimited(t ScrapeTarget) {
	m.sem <- struct{}{}
	defer func() { <-m.sem }()
	defer func() {
		if r := recover(); r != nil {
			slog.Error("scrape panic recovered", "target", t.Name, "err", r)
		}
	}()
	m.scrape(t)
}

// scrape 抓取一个目标：GET /metrics → 解析 → 附加标签(job/自定义) → 写 VM。
func (m *scrapeManager) scrape(t ScrapeTarget) {
	to := t.TimeoutSec
	if to <= 0 {
		to = 10
	}
	client := newGuardedHTTPClient(time.Duration(to) * time.Second) // SSRF 守卫：目标 URL 用户可配
	req, err := http.NewRequest(http.MethodGet, t.URL, nil)
	if err != nil {
		m.setStatus(t.ID, false, 0, 0, err.Error())
		return
	}
	for k, v := range t.Headers {
		if strings.TrimSpace(k) != "" {
			req.Header.Set(k, v)
		}
	}
	req.Header.Set("Accept", "text/plain;version=0.0.4")
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		m.setStatus(t.ID, false, 0, ms(time.Since(start)), "抓取失败："+err.Error())
		return
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20)) // 16MB 上限
	resp.Body.Close()
	latency := ms(time.Since(start))
	if resp.StatusCode >= 300 {
		m.setStatus(t.ID, false, 0, latency, fmt.Sprintf("HTTP %d", resp.StatusCode))
		return
	}
	extra := map[string]string{"job": t.Name}
	for k, v := range t.Labels {
		if strings.TrimSpace(k) != "" {
			extra[k] = v
		}
	}
	samples := parsePromText(body, time.Now().UnixMilli(), extra)
	if len(samples) == 0 {
		m.setStatus(t.ID, false, 0, latency, "未解析出任何指标（检查是否为 Prometheus 文本格式）")
		return
	}
	if m.vm != nil {
		m.vm.writeLabeled(samples)
	}
	m.setStatus(t.ID, true, len(samples), latency, "")
}

func (m *scrapeManager) setStatus(id string, ok bool, samples int, latency float64, msg string) {
	m.mu.Lock()
	m.status[id] = scrapeStatus{OK: ok, Samples: samples, LatencyMs: latency, Msg: msg, CheckedAt: time.Now().Unix()}
	m.mu.Unlock()
}

func (m *scrapeManager) snapshot() map[string]scrapeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]scrapeStatus, len(m.status))
	for k, v := range m.status {
		out[k] = v
	}
	return out
}

// runNow 立即抓取某目标（新增/编辑后触发）。
func (m *scrapeManager) runNow(id string) {
	for _, tg := range m.cfg.ScrapeTargets() {
		if tg.ID == id && tg.Enabled {
			m.mu.Lock()
			m.lastRun[tg.ID] = time.Now()
			m.mu.Unlock()
			go m.scrapeLimited(tg)
			return
		}
	}
}
