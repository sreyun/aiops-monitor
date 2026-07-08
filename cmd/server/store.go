package main

import (
	"crypto/subtle"
	"sync"
	"time"

	"aiops-monitor/shared"
)

const (
	maxSamples        = 240  // cap for the legacy /metrics endpoint (tail of raw history)
	maxEvents         = 300  // global ring of recent plugin events
	eventsPerAPI      = 100  // cap returned by the events endpoint
	deleteSuppressSec = 60   // ignore a host's re-reports for this long after a manual delete
	maxActivity       = 1000 // ring of recent activity-log entries (persisted)
	eventCooldownSec  = 300  // min gap between identical plugin events (noise suppression)

	// History storage constants (multi-tier downsampling)
	histRawMax     = 1200 // raw samples: ~1.5h at 5s interval
	hist1mMax      = 2880 // 1-min aggregates: 48h (2880 points)
	hist5mMax      = 2016 // 5-min aggregates: 7 days (2016 points)
	hist1mInterval = 60   // aggregate to 1-min every 60s
	hist5mInterval = 300  // aggregate to 5-min every 300s
)

// Host is the aggregate record the server keeps per agent.
type Host struct {
	ID          string             `json:"id"`
	Hostname    string             `json:"hostname"`
	OS          string             `json:"os"`
	Platform    string             `json:"platform"`
	Arch        string             `json:"arch"`
	IP          string             `json:"ip"`
	Kernel      string             `json:"kernel"`
	Category    string             `json:"category"`
	Fingerprint string             `json:"fingerprint,omitempty"` // machine fingerprint (machine-id+MAC), bound at registration
	FirstSeen   int64              `json:"first_seen"`
	LastSeen    int64              `json:"last_seen"`
	Latest      *shared.Sample     `json:"latest"`
	Custom      map[string]float64 `json:"custom,omitempty"` // latest custom gauges from plugins

	// Time-series history (multi-tier downsampling; persisted via the embedded DB)
	histRaw  []shared.Sample // raw samples (5s interval, ~1.5h)
	hist1m   []shared.Sample // 1-min aggregates (last 48h)
	hist5m   []shared.Sample // 5-min aggregates (last 7 days)
	last1mTs int64           // timestamp of last 1-min aggregation
	last5mTs int64           // timestamp of last 5-min aggregation
}

// storedEvent decorates a plugin event with the host it came from.
type storedEvent struct {
	shared.Event
	HostID   string `json:"host_id"`
	Hostname string `json:"hostname"`
}

// LogEntry is one line in the activity log. It unifies operator actions (操作),
// machine/system actions such as alert transitions and notifications (系统),
// and plugin findings (插件).
type LogEntry struct {
	Timestamp int64  `json:"timestamp"`
	Kind      string `json:"kind"`  // 操作 | 系统 | 插件
	Level     string `json:"level"` // info | warning | critical
	Actor     string `json:"actor"`
	Host      string `json:"host,omitempty"`
	Message   string `json:"message"`
}

// Store holds all host state and a ring of recent plugin events.
type Store struct {
	mu       sync.RWMutex
	hosts    map[string]*Host
	events   []storedEvent
	activity []LogEntry
	deleted   map[string]int64 // hostID -> unix time of manual deletion (re-add suppression)
	lastEvent map[string]int64 // dedup key -> last unix time (plugin-event noise suppression)
	dirty     bool             // set on every mutation; consumed by the embedded DB's autosave
}

func NewStore() *Store {
	return &Store{hosts: make(map[string]*Host), deleted: make(map[string]int64), lastEvent: make(map[string]int64)}
}

// GetHost returns a shallow copy of one host by id (for fingerprint verification).
func (s *Store) GetHost(id string) (*Host, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hosts[id]
	if !ok {
		return nil, false
	}
	cp := *h
	return &cp, true
}

// RegisterHost binds a machine fingerprint to a host record at registration time.
// It creates the host if absent, fills in a missing fingerprint, or updates the
// fingerprint when the machine hardware changed but the agent state file (and thus
// host_id) persisted. Token-based admission is checked by the caller beforehand.
func (s *Store) RegisterHost(hostID, hostname, fingerprint string) *Host {
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	if dt, ok := s.deleted[hostID]; ok {
		if now-dt < deleteSuppressSec {
			return &Host{ID: hostID} // recently deleted; suppress re-registration briefly
		}
		delete(s.deleted, hostID)
	}
	h, ok := s.hosts[hostID]
	if !ok {
		h = &Host{ID: hostID, Hostname: hostname, Fingerprint: fingerprint, FirstSeen: now, LastSeen: now}
		s.hosts[hostID] = h
		s.dirty = true
		return h
	}
	// Existing record: fill in or update the fingerprint.
	if h.Fingerprint != fingerprint {
		h.Fingerprint = fingerprint
		s.dirty = true
	}
	if hostname != "" {
		h.Hostname = hostname
	}
	s.dirty = true
	return h
}

// UpsertAuthenticated applies a report after verifying the agent's fingerprint
// against the one bound at registration. Returns (nil, false) when the host is
// unregistered, its fingerprint is not yet bound, or the fingerprint does not
// match — the caller must reject the report with 403. Verification and update
// happen under a single lock to avoid a TOCTOU window (host deleted between
// check and upsert) and the double-lock overhead of GetHost + Upsert on the hot
// report path.
func (s *Store) UpsertAuthenticated(r shared.Report, fingerprint string) (*Host, bool) {
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()

	if dt, ok := s.deleted[r.HostID]; ok {
		if now-dt < deleteSuppressSec {
			return nil, false // recently deleted by an operator; ignore re-report
		}
		delete(s.deleted, r.HostID) // suppression window elapsed
	}

	h, ok := s.hosts[r.HostID]
	if !ok {
		return nil, false // not registered
	}
	if h.Fingerprint == "" {
		return nil, false // fingerprint not bound (agent hasn't registered yet)
	}
	if subtle.ConstantTimeCompare([]byte(fingerprint), []byte(h.Fingerprint)) != 1 {
		return nil, false // fingerprint mismatch
	}
	h.Hostname = r.Hostname
	h.OS = r.OS
	h.Platform = r.Platform
	h.Arch = r.Arch
	h.IP = r.IP
	h.Kernel = r.Kernel
	h.Category = r.Category
	h.LastSeen = now

	sample := shared.Sample{Timestamp: now, Metrics: r.Metrics}
	sample.ProcessNames = nil // history never stores process lists (only Latest keeps them)

	// ---- Time-series history (multi-tier downsampling) ----
	// Tier 1: Raw samples (5s interval, ~1.5h)
	h.histRaw = append(h.histRaw, sample)
	if len(h.histRaw) > histRawMax {
		h.histRaw = h.histRaw[len(h.histRaw)-histRawMax:]
	}

	// Tier 2: 1-min aggregates (last 48h)
	if now-h.last1mTs >= hist1mInterval {
		agg := h.aggregateSamples(h.histRaw, h.last1mTs, now, hist1mInterval)
		if agg != nil {
			h.hist1m = append(h.hist1m, *agg)
			if len(h.hist1m) > hist1mMax {
				h.hist1m = h.hist1m[len(h.hist1m)-hist1mMax:]
			}
			h.last1mTs = now
		}
	}

	// Tier 3: 5-min aggregates (last 7 days)
	if now-h.last5mTs >= hist5mInterval {
		agg := h.aggregateSamples(h.hist1m, h.last5mTs, now, hist5mInterval)
		if agg != nil {
			h.hist5m = append(h.hist5m, *agg)
			if len(h.hist5m) > hist5mMax {
				h.hist5m = h.hist5m[len(h.hist5m)-hist5mMax:]
			}
			h.last5mTs = now
		}
	}

	latest := sample
	latest.ProcessNames = r.Metrics.ProcessNames // Latest alone carries the process list
	h.Latest = &latest
	if len(r.Custom) > 0 {
		h.Custom = r.Custom
	}

	for _, e := range r.Events {
		if e.Timestamp == 0 {
			e.Timestamp = now
		}
		// Noise suppression: an agent running a misconfigured probe (e.g. the old
		// example_service_check hitting 127.0.0.1:8529) would otherwise flood the
		// log every cycle. Record an identical event at most once per cooldown.
		key := h.ID + "|" + e.Source + "|" + e.Level + "|" + e.Message
		if last, ok := s.lastEvent[key]; ok && now-last < eventCooldownSec {
			continue
		}
		if len(s.lastEvent) > 2000 { // bound the dedup map (values-with-numbers make unique keys)
			for k, v := range s.lastEvent {
				if now-v >= eventCooldownSec {
					delete(s.lastEvent, k)
				}
			}
		}
		s.lastEvent[key] = now
		s.events = append(s.events, storedEvent{Event: e, HostID: h.ID, Hostname: h.Hostname})
		s.appendLog(LogEntry{Timestamp: e.Timestamp, Kind: "插件", Level: e.Level, Actor: e.Source, Host: h.Hostname, Message: e.Message})
	}
	if len(s.events) > maxEvents {
		s.events = s.events[len(s.events)-maxEvents:]
	}
	s.dirty = true
	return h, true
}

// hostMeta returns a shallow copy suitable for list APIs: the Latest sample is
// copied with its (potentially huge) process list stripped, so /hosts stays
// lean. Process names are served on demand via GetProcessNames.
func hostMeta(h *Host) *Host {
	cp := *h
	if h.Latest != nil {
		l := *h.Latest
		l.ProcessNames = nil
		cp.Latest = &l
	}
	return &cp
}

// aggregateSamples aggregates samples within [from, to] into a single sample.
// It computes the average of numeric metrics and takes the last value for counters.
func (h *Host) aggregateSamples(samples []shared.Sample, from, to, interval int64) *shared.Sample {
	if len(samples) == 0 {
		return nil
	}

	// Find samples in the aggregation window
	var window []shared.Sample
	for _, s := range samples {
		if s.Timestamp >= from && s.Timestamp < to {
			window = append(window, s)
		}
	}
	if len(window) == 0 {
		return nil
	}

	// Compute averages
	var agg shared.Sample
	agg.Timestamp = to
	n := float64(len(window))

	// CPU
	var cpuSum float64
	for _, s := range window {
		cpuSum += s.CPUPercent
	}
	agg.CPUPercent = cpuSum / n
	agg.CPUCores = window[len(window)-1].CPUCores // take last

	// Memory
	var memUsedSum, memTotalSum float64
	for _, s := range window {
		memUsedSum += float64(s.MemUsed)
		memTotalSum += float64(s.MemTotal)
	}
	avgMemUsed := uint64(memUsedSum / n)
	avgMemTotal := uint64(memTotalSum / n)
	agg.MemUsed = avgMemUsed
	agg.MemTotal = avgMemTotal
	if avgMemTotal > 0 {
		agg.MemPercent = float64(avgMemUsed) / float64(avgMemTotal) * 100
	}

	// Swap
	var swapUsedSum, swapTotalSum float64
	for _, s := range window {
		swapUsedSum += float64(s.SwapUsed)
		swapTotalSum += float64(s.SwapTotal)
	}
	avgSwapUsed := uint64(swapUsedSum / n)
	avgSwapTotal := uint64(swapTotalSum / n)
	agg.SwapUsed = avgSwapUsed
	agg.SwapTotal = avgSwapTotal
	if avgSwapTotal > 0 {
		agg.SwapPercent = float64(avgSwapUsed) / float64(avgSwapTotal) * 100
	}

	// Disk (root filesystem)
	var diskUsedSum, diskTotalSum float64
	for _, s := range window {
		diskUsedSum += float64(s.DiskUsed)
		diskTotalSum += float64(s.DiskTotal)
	}
	avgDiskUsed := uint64(diskUsedSum / n)
	avgDiskTotal := uint64(diskTotalSum / n)
	agg.DiskUsed = avgDiskUsed
	agg.DiskTotal = avgDiskTotal
	if avgDiskTotal > 0 {
		agg.DiskPercent = float64(avgDiskUsed) / float64(avgDiskTotal) * 100
	}

	// Per-disk info: aggregate each mount point
	if len(window) > 0 && len(window[0].Disks) > 0 {
		diskMap := make(map[string][]shared.DiskInfo)
		for _, s := range window {
			for _, d := range s.Disks {
				diskMap[d.Path] = append(diskMap[d.Path], d)
			}
		}
		for path, infos := range diskMap {
			var totalSum, usedSum float64
			for _, d := range infos {
				totalSum += float64(d.Total)
				usedSum += float64(d.Used)
			}
			avgTotal := uint64(totalSum / float64(len(infos)))
			avgUsed := uint64(usedSum / float64(len(infos)))
			percent := 0.0
			if avgTotal > 0 {
				percent = float64(avgUsed) / float64(avgTotal) * 100
			}
			agg.Disks = append(agg.Disks, shared.DiskInfo{
				Path:    path,
				Total:   avgTotal,
				Used:    avgUsed,
				Percent: percent,
			})
		}
	}

	// Per-GPU info: aggregate each GPU by name (average util / VRAM)
	if len(window) > 0 && len(window[0].GPUs) > 0 {
		type gacc struct {
			util, memUsed, memTotal, temp, n float64
		}
		order := []string{}
		gmap := map[string]*gacc{}
		for _, s := range window {
			for _, g := range s.GPUs {
				a := gmap[g.Name]
				if a == nil {
					a = &gacc{}
					gmap[g.Name] = a
					order = append(order, g.Name)
				}
				a.util += g.UtilPercent
				a.memUsed += float64(g.MemUsed)
				a.memTotal += float64(g.MemTotal)
				a.temp += g.Temp
				a.n++
			}
		}
		for _, name := range order {
			a := gmap[name]
			if a.n == 0 {
				continue
			}
			gi := shared.GPUInfo{
				Name:        name,
				UtilPercent: a.util / a.n,
				MemUsed:     uint64(a.memUsed / a.n),
				MemTotal:    uint64(a.memTotal / a.n),
				Temp:        a.temp / a.n,
			}
			if gi.MemTotal > 0 {
				gi.MemPercent = float64(gi.MemUsed) / float64(gi.MemTotal) * 100
			}
			agg.GPUs = append(agg.GPUs, gi)
		}
	}

	// Network rates (average)
	var netSentSum, netRecvSum float64
	for _, s := range window {
		netSentSum += s.NetSentRate
		netRecvSum += s.NetRecvRate
	}
	agg.NetSentRate = netSentSum / n
	agg.NetRecvRate = netRecvSum / n

	// Connections (average)
	var connsSum float64
	for _, s := range window {
		connsSum += float64(s.NetConns)
	}
	agg.NetConns = int(connsSum / n)

	// Load averages
	var l1Sum, l5Sum, l15Sum float64
	for _, s := range window {
		l1Sum += s.Load1
		l5Sum += s.Load5
		l15Sum += s.Load15
	}
	agg.Load1 = l1Sum / n
	agg.Load5 = l5Sum / n
	agg.Load15 = l15Sum / n

	// Process count (average)
	var procSum float64
	for _, s := range window {
		procSum += float64(s.ProcCount)
	}
	agg.ProcCount = int(procSum / n)

	// Uptime (take max, as it only increases)
	var maxUptime uint64
	for _, s := range window {
		if s.Uptime > maxUptime {
			maxUptime = s.Uptime
		}
	}
	agg.Uptime = maxUptime

	return &agg
}

// ListHosts returns metadata + latest sample + custom gauges for every host.
func (s *Store) ListHosts() []*Host {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Host, 0, len(s.hosts))
	for _, h := range s.hosts {
		out = append(out, hostMeta(h))
	}
	return out
}

// GetSamples returns the tail of the raw history for one host (legacy
// /metrics endpoint; the /history endpoint serves the tiered archive).
func (s *Store) GetSamples(id string) ([]shared.Sample, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hosts[id]
	if !ok {
		return nil, false
	}
	src := h.histRaw
	if len(src) > maxSamples {
		src = src[len(src)-maxSamples:]
	}
	cp := make([]shared.Sample, len(src))
	copy(cp, src)
	return cp, true
}

// GetProcessNames returns the latest reported process list for one host.
func (s *Store) GetProcessNames(id string) ([]string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hosts[id]
	if !ok || h.Latest == nil {
		return nil, false
	}
	return h.Latest.ProcessNames, true
}

// GetHistory returns time-series data for a host within [from, to] range.
// It automatically selects the appropriate tier based on the time span:
// - < 2h: raw samples (~3s interval)
// - < 48h: 1-min aggregates
// - >= 48h: 5-min aggregates
func (s *Store) GetHistory(id string, from, to int64) ([]shared.Sample, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hosts[id]
	if !ok {
		return nil, false
	}

	span := to - from
	var src []shared.Sample

	// Select tier based on time span
	if span < 7200 { // < 2h: use raw
		src = h.histRaw
	} else if span < 172800 { // < 48h: use 1-min
		src = h.hist1m
	} else { // >= 48h: use 5-min
		src = h.hist5m
	}

	// Filter by time range
	result := make([]shared.Sample, 0, len(src))
	for _, sample := range src {
		if sample.Timestamp >= from && sample.Timestamp <= to {
			result = append(result, sample)
		}
	}
	return result, true
}

// RecentEvents returns the most recent plugin events, newest first.
func (s *Store) RecentEvents() []storedEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := len(s.events)
	if n > eventsPerAPI {
		n = eventsPerAPI
	}
	out := make([]storedEvent, 0, n)
	for i := len(s.events) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, s.events[i])
	}
	return out
}

// DeleteHost removes a host and its events, and briefly suppresses re-adding it
// (so a still-running agent doesn't immediately resurrect a just-cleaned entry).
// Returns false if the host was not present.
func (s *Store) DeleteHost(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.hosts[id]; !ok {
		return false
	}
	delete(s.hosts, id)
	s.deleted[id] = time.Now().Unix()
	kept := s.events[:0]
	for _, e := range s.events {
		if e.HostID != id {
			kept = append(kept, e)
		}
	}
	s.events = kept
	s.dirty = true
	return true
}

// appendLog adds an activity-log entry; the caller must already hold s.mu.
func (s *Store) appendLog(e LogEntry) {
	if e.Timestamp == 0 {
		e.Timestamp = time.Now().Unix()
	}
	s.activity = append(s.activity, e)
	if len(s.activity) > maxActivity {
		s.activity = s.activity[len(s.activity)-maxActivity:]
	}
	s.dirty = true
}

// AddLog records an activity-log entry (locks internally).
func (s *Store) AddLog(e LogEntry) {
	s.mu.Lock()
	s.appendLog(e)
	s.mu.Unlock()
}

// RecentActivity returns activity-log entries, newest first.
func (s *Store) RecentActivity() []LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LogEntry, 0, len(s.activity))
	for i := len(s.activity) - 1; i >= 0; i-- {
		out = append(out, s.activity[i])
	}
	return out
}
