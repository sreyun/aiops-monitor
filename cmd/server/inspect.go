package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Host deep-inspection (agent module host_inspect) — batch run + web report store.

const (
	hostInspectCap       = 100
	hostInspectOutCap    = 2 << 20 // 2 MiB JSON reports
	hostInspectTimeout   = 180
	hostInspectConcLimit = 8
)

type hostInspectItem struct {
	HostID     string          `json:"host_id"`
	Hostname   string          `json:"hostname"`
	OS         string          `json:"os"`
	IP         string          `json:"ip"`
	Status     string          `json:"status"` // pending|running|ok|warn|crit|error
	Error      string          `json:"error,omitempty"`
	Warnings   int             `json:"warnings"`
	Critical   int             `json:"critical"`
	OSFamily   string          `json:"os_family,omitempty"`
	Report     json.RawMessage `json:"report,omitempty"`
	FinishedAt int64           `json:"finished_at,omitempty"`
	DurationMs int64           `json:"duration_ms,omitempty"`
}

type hostInspectBatch struct {
	ID         string            `json:"id"`
	Operator   string            `json:"operator"`
	Source     string            `json:"source,omitempty"` // e.g. playbook: nightly-inspect
	Status     string            `json:"status"`           // running|done
	StartedAt  int64             `json:"started_at"`
	FinishedAt int64             `json:"finished_at,omitempty"`
	HostCount  int               `json:"host_count"`
	DoneCount  int               `json:"done_count"`
	OKCount    int               `json:"ok_count"`
	WarnCount  int               `json:"warn_count"`
	CritCount  int               `json:"crit_count"`
	ErrCount   int               `json:"err_count"`
	Items      []hostInspectItem `json:"items"`
}

type hostInspectManager struct {
	mu      sync.RWMutex
	batches []*hostInspectBatch
	seq     atomic.Uint64
	persist func([]*hostInspectBatch) // optional PG persist hook
}

func newHostInspectManager() *hostInspectManager {
	return &hostInspectManager{}
}

func (m *hostInspectManager) setPersist(fn func([]*hostInspectBatch)) {
	m.mu.Lock()
	m.persist = fn
	m.mu.Unlock()
}

func (m *hostInspectManager) importBatches(list []*hostInspectBatch) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(list) == 0 {
		return
	}
	// Batches persisted as "running" cannot resume after process restart — mark them
	// failed so the UI does not show forever-stuck「巡检中」rows (same class of bug
	// as orphaned security scans).
	for _, b := range list {
		if b == nil {
			continue
		}
		if b.Status == "running" || b.Status == "pending" {
			b.Status = "done"
			for i := range b.Items {
				st := b.Items[i].Status
				if st == "running" || st == "pending" || st == "" {
					b.Items[i].Status = "error"
					if strings.TrimSpace(b.Items[i].Error) == "" {
						b.Items[i].Error = "服务重启，巡检中断"
					}
					b.ErrCount++
					b.DoneCount++
				}
			}
			if b.FinishedAt == 0 {
				b.FinishedAt = time.Now().Unix()
			}
		}
	}
	m.batches = list
	if len(m.batches) > hostInspectCap {
		m.batches = m.batches[:hostInspectCap]
	}
}

func (m *hostInspectManager) snapshot() []*hostInspectBatch {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*hostInspectBatch, len(m.batches))
	for i, b := range m.batches {
		out[i] = cloneInspectBatch(b)
	}
	return out
}

func (m *hostInspectManager) persistAsync() {
	m.mu.RLock()
	fn := m.persist
	m.mu.RUnlock()
	if fn == nil {
		return
	}
	go fn(m.snapshot())
}

func (m *hostInspectManager) list() []*hostInspectBatch {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*hostInspectBatch, len(m.batches))
	for i, b := range m.batches {
		out[i] = cloneInspectBatch(b)
	}
	return out
}

func (m *hostInspectManager) get(id string) (*hostInspectBatch, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, b := range m.batches {
		if b.ID == id {
			return cloneInspectBatch(b), true
		}
	}
	return nil, false
}

func (m *hostInspectManager) add(b *hostInspectBatch) {
	m.mu.Lock()
	m.batches = append([]*hostInspectBatch{b}, m.batches...)
	if len(m.batches) > hostInspectCap {
		m.batches = m.batches[:hostInspectCap]
	}
	m.mu.Unlock()
	m.persistAsync()
}

func (m *hostInspectManager) updateItem(batchID string, idx int, item hostInspectItem, bumpCounts bool) {
	m.mu.Lock()
	changed := false
	for _, b := range m.batches {
		if b.ID != batchID {
			continue
		}
		if idx < 0 || idx >= len(b.Items) {
			break
		}
		b.Items[idx] = item
		if bumpCounts {
			b.DoneCount++
			switch item.Status {
			case "ok":
				b.OKCount++
			case "warn":
				b.WarnCount++
			case "crit":
				b.CritCount++
			default:
				b.ErrCount++
			}
		}
		changed = true
		break
	}
	m.mu.Unlock()
	if changed {
		m.persistAsync()
	}
}

func (m *hostInspectManager) finish(batchID string) {
	m.mu.Lock()
	for _, b := range m.batches {
		if b.ID == batchID {
			b.Status = "done"
			b.FinishedAt = time.Now().Unix()
			break
		}
	}
	m.mu.Unlock()
	m.persistAsync()
}

func cloneInspectBatch(b *hostInspectBatch) *hostInspectBatch {
	if b == nil {
		return nil
	}
	cp := *b
	cp.Items = make([]hostInspectItem, len(b.Items))
	copy(cp.Items, b.Items)
	return &cp
}

func recalcInspectBatchCounts(b *hostInspectBatch) {
	if b == nil {
		return
	}
	b.OKCount, b.WarnCount, b.CritCount, b.ErrCount, b.DoneCount = 0, 0, 0, 0, 0
	for _, it := range b.Items {
		switch it.Status {
		case "pending", "running", "":
			continue
		case "ok":
			b.OKCount++
		case "warn":
			b.WarnCount++
		case "crit":
			b.CritCount++
		default:
			b.ErrCount++
		}
		b.DoneCount++
	}
}

func parseHostInspectOutput(host *Host, output string, durationMs int64) hostInspectItem {
	item := hostInspectItem{
		HostID: host.ID, Hostname: host.Hostname, OS: host.OS, IP: host.IP,
		DurationMs: durationMs, FinishedAt: time.Now().Unix(),
	}
	body := strings.TrimSpace(output)
	if i := strings.Index(body, "{"); i >= 0 {
		body = body[i:]
	}
	if j := strings.LastIndex(body, "}"); j >= 0 {
		body = body[:j+1]
	}
	var rep struct {
		Host struct {
			OSFamily string `json:"os_family"`
		} `json:"host"`
		Result struct {
			Warnings int `json:"warnings"`
			Critical int `json:"critical"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &rep); err != nil {
		item.Status = "error"
		item.Error = "巡检结果不是有效 JSON: " + truncateStr(body, 120)
		if err != nil {
			item.Error += " (" + err.Error() + ")"
		}
		return item
	}
	item.Report = json.RawMessage(body)
	item.Warnings = rep.Result.Warnings
	item.Critical = rep.Result.Critical
	item.OSFamily = rep.Host.OSFamily
	switch {
	case rep.Result.Critical > 0:
		item.Status = "crit"
	case rep.Result.Warnings > 0:
		item.Status = "warn"
	default:
		item.Status = "ok"
	}
	return item
}

func playbookInspectBatchID(execID int64) string {
	return fmt.Sprintf("insp-pb-%d", execID)
}

// ingestPlaybookHostInspect stores a successful playbook host_inspect step into the
// inspect batch store (one batch per playbook execution).
func (s *Server) ingestPlaybookHostInspect(playbookName string, execID int64, host *Host, operator, output string, durationMs int64) {
	if s.inspect == nil || host == nil {
		return
	}
	source := "playbook: " + strings.TrimSpace(playbookName)
	if source == "playbook:" {
		source = "playbook"
	}
	item := parseHostInspectOutput(host, output, durationMs)
	s.inspect.ingestPlaybookItem(playbookInspectBatchID(execID), source, operator, item)
}

func (m *hostInspectManager) ingestPlaybookItem(batchID, source, operator string, item hostInspectItem) {
	m.mu.Lock()
	var batch *hostInspectBatch
	for _, b := range m.batches {
		if b != nil && b.ID == batchID {
			batch = b
			break
		}
	}
	if batch == nil {
		batch = &hostInspectBatch{
			ID: batchID, Operator: operator, Source: source,
			Status: "running", StartedAt: time.Now().Unix(),
			Items: make([]hostInspectItem, 0, 4),
		}
		m.batches = append([]*hostInspectBatch{batch}, m.batches...)
		if len(m.batches) > hostInspectCap {
			m.batches = m.batches[:hostInspectCap]
		}
	} else if strings.TrimSpace(batch.Source) == "" && source != "" {
		batch.Source = source
	}
	found := false
	for i, it := range batch.Items {
		if it.HostID == item.HostID {
			batch.Items[i] = item
			found = true
			break
		}
	}
	if !found {
		batch.Items = append(batch.Items, item)
	}
	recalcInspectBatchCounts(batch)
	batch.HostCount = len(batch.Items)
	m.mu.Unlock()
	m.persistAsync()
}

func (m *hostInspectManager) finishPlaybookBatch(batchID string) {
	m.mu.Lock()
	changed := false
	for _, b := range m.batches {
		if b == nil || b.ID != batchID {
			continue
		}
		if b.Status != "done" {
			b.Status = "done"
			b.FinishedAt = time.Now().Unix()
		}
		b.HostCount = len(b.Items)
		recalcInspectBatchCounts(b)
		changed = true
		break
	}
	m.mu.Unlock()
	if changed {
		m.persistAsync()
	}
}

// ---- HTTP ----

func (s *Server) handleListHostInspect(w http.ResponseWriter, r *http.Request) {
	if s.inspect == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, s.inspect.list())
}

func (s *Server) handleGetHostInspect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	b, ok := s.inspect.get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "巡检批次不存在"})
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) handleRunHostInspect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HostIDs    []string `json:"host_ids"`
		TimeoutSec int      `json:"timeout_sec"`
		Profile    string   `json:"profile"` // quick | standard | deep
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	timeout := req.TimeoutSec
	if timeout < 30 {
		timeout = hostInspectTimeout
	}
	if timeout > 600 {
		timeout = 600
	}
	profile := strings.ToLower(strings.TrimSpace(req.Profile))
	switch profile {
	case "quick", "fast":
		profile = "quick"
	case "deep", "full":
		profile = "deep"
		if timeout < 240 {
			timeout = 240
		}
	default:
		profile = "standard"
	}

	offlineSec := int64(s.cfg.Thresholds().OfflineAfter.Seconds())
	if offlineSec <= 0 {
		offlineSec = 120
	}
	now := time.Now().Unix()
	all := s.store.ListHosts()
	var targets []*Host
	want := map[string]bool{}
	if len(req.HostIDs) == 0 {
		for _, h := range all {
			if h != nil && h.LastSeen > 0 && now-h.LastSeen <= offlineSec {
				targets = append(targets, h)
			}
		}
	} else {
		for _, id := range req.HostIDs {
			want[id] = true
		}
		for _, h := range all {
			if h != nil && want[h.ID] {
				targets = append(targets, h)
			}
		}
	}
	if u, ok := s.currentUser(r); ok && u.hostScopeRestricted() {
		filtered := make([]*Host, 0, len(targets))
		for _, h := range targets {
			if s.userCanAccessHost(u, h.ID) {
				filtered = append(filtered, h)
			}
		}
		targets = filtered
	}
	if len(targets) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "没有可体检的主机（请选择在线且已授权的主机）"})
		return
	}

	id := fmt.Sprintf("insp-%d-%d", time.Now().Unix(), s.inspect.seq.Add(1))
	items := make([]hostInspectItem, len(targets))
	for i, h := range targets {
		items[i] = hostInspectItem{
			HostID: h.ID, Hostname: h.Hostname, OS: h.OS, IP: h.IP, Status: "pending",
		}
	}
	batch := &hostInspectBatch{
		ID: id, Operator: s.actorName(r), Status: "running",
		StartedAt: time.Now().Unix(), HostCount: len(targets), Items: items,
	}

	s.inspect.add(batch)

	go s.runHostInspectBatch(batch.ID, targets, timeout, profile)
	writeJSON(w, http.StatusAccepted, batch)
}

// handleCompareHostInspect returns two finished batches for side-by-side history compare.
func (s *Server) handleCompareHostInspect(w http.ResponseWriter, r *http.Request) {
	aID := r.URL.Query().Get("a")
	bID := r.URL.Query().Get("b")
	if aID == "" || bID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "需要查询参数 a 与 b（两个批次 ID）"})
		return
	}
	a, okA := s.inspect.get(aID)
	b, okB := s.inspect.get(bID)
	if !okA || !okB {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "体检批次不存在"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"a": a, "b": b})
}

func (s *Server) runHostInspectBatch(batchID string, hosts []*Host, timeoutSec int, profile string) {
	sem := make(chan struct{}, hostInspectConcLimit)
	var wg sync.WaitGroup
	if profile == "" {
		profile = "standard"
	}
	cmd := buildModuleCommand("host_inspect", map[string]string{"profile": profile}, nil)
	for i, h := range hosts {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, host *Host) {
			defer wg.Done()
			defer func() { <-sem }()
			item := hostInspectItem{
				HostID: host.ID, Hostname: host.Hostname, OS: host.OS, IP: host.IP, Status: "running",
			}
			s.inspect.updateItem(batchID, idx, item, false)
			start := time.Now()
			out, kind, err := s.execCommandOnHostSized(host, cmd, timeoutSec, hostInspectOutCap)
			item.DurationMs = time.Since(start).Milliseconds()
			if err != nil && kind != execExit {
				item.Status = "error"
				item.Error = err.Error()
				if out != "" {
					item.Error += " | " + truncateStr(out, 200)
				}
				item.FinishedAt = time.Now().Unix()
				s.inspect.updateItem(batchID, idx, item, true)
				return
			}
			item = parseHostInspectOutput(host, out, item.DurationMs)
			s.inspect.updateItem(batchID, idx, item, true)
		}(i, h)
	}
	wg.Wait()
	s.inspect.finish(batchID)
	slog.Info("host inspect batch done", "id", batchID, "hosts", len(hosts))
}

// execCommandOnHostSized is like execCommandOnHost but allows a larger output buffer (for JSON reports).
func (s *Server) execCommandOnHostSized(h *Host, command string, timeoutSec, maxBytes int) (string, execKind, error) {
	if timeoutSec < 5 {
		timeoutSec = 30
	}
	if maxBytes < 64*1024 {
		maxBytes = 512 * 1024
	}
	sess := s.term.createExec(h.ID, h.Hostname, command)
	defer s.term.remove(sess.id)
	defer sess.close()
	s.term.notifyAgent(h.ID, sess.id)

	select {
	case <-sess.agentUp:
	case <-time.After(execPickupTimeout):
		return "", execNoPickup, fmt.Errorf("%s", Tz("playbook.no_pickup"))
	case <-sess.done:
		return "", execAbnormal, fmt.Errorf("%s", Tz("playbook.abnormal"))
	}

	var output []byte
	timer := time.NewTimer(time.Duration(timeoutSec) * time.Second)
	defer timer.Stop()
	for {
		select {
		case b := <-sess.toBrowser:
			output = append(output, b...)
			if len(output) > maxBytes {
				output = output[len(output)-maxBytes:]
			}
		case <-timer.C:
			out, kind, err := parseExecOutput(output, true)
			return out, kind, err
		case <-sess.done:
			draining := true
			for draining {
				select {
				case b := <-sess.toBrowser:
					output = append(output, b...)
				default:
					draining = false
				}
			}
			out, kind, err := parseExecOutput(output, false)
			return out, kind, err
		}
	}
}


