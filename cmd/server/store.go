package main

import (
	"sync"
	"time"

	"aiops-monitor/shared"
)

const (
	maxSamples        = 240 // ~20 min of base-metric history at 5s interval
	maxEvents         = 200 // global ring of recent plugin events
	eventsPerAPI      = 100 // cap returned by the events endpoint
	deleteSuppressSec = 60  // ignore a host's re-reports for this long after a manual delete
	maxActivity       = 300 // ring of recent activity-log entries
)

// Host is the aggregate record the server keeps per agent.
type Host struct {
	ID        string             `json:"id"`
	Hostname  string             `json:"hostname"`
	OS        string             `json:"os"`
	Platform  string             `json:"platform"`
	Arch      string             `json:"arch"`
	IP        string             `json:"ip"`
	Kernel    string             `json:"kernel"`
	Category  string             `json:"category"`
	FirstSeen int64              `json:"first_seen"`
	LastSeen  int64              `json:"last_seen"`
	Latest    *shared.Sample     `json:"latest"`
	Custom    map[string]float64 `json:"custom,omitempty"` // latest custom gauges from plugins
	Samples   []shared.Sample    `json:"-"`
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
	deleted  map[string]int64 // hostID -> unix time of manual deletion (re-add suppression)
}

func NewStore() *Store {
	return &Store{hosts: make(map[string]*Host), deleted: make(map[string]int64)}
}

// Upsert applies a report: base metrics -> sample history, custom gauges ->
// latest snapshot, and any plugin events -> the global ring.
func (s *Store) Upsert(r shared.Report) *Host {
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()

	if dt, ok := s.deleted[r.HostID]; ok {
		if now-dt < deleteSuppressSec {
			return &Host{ID: r.HostID} // recently deleted by an operator; ignore re-report
		}
		delete(s.deleted, r.HostID) // suppression window elapsed
	}

	h, ok := s.hosts[r.HostID]
	if !ok {
		h = &Host{ID: r.HostID, FirstSeen: now}
		s.hosts[r.HostID] = h
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
	h.Samples = append(h.Samples, sample)
	if len(h.Samples) > maxSamples {
		h.Samples = h.Samples[len(h.Samples)-maxSamples:]
	}
	latest := sample
	h.Latest = &latest
	if len(r.Custom) > 0 {
		h.Custom = r.Custom
	}

	for _, e := range r.Events {
		if e.Timestamp == 0 {
			e.Timestamp = now
		}
		s.events = append(s.events, storedEvent{Event: e, HostID: h.ID, Hostname: h.Hostname})
		s.appendLog(LogEntry{Timestamp: e.Timestamp, Kind: "插件", Level: e.Level, Actor: e.Source, Host: h.Hostname, Message: e.Message})
	}
	if len(s.events) > maxEvents {
		s.events = s.events[len(s.events)-maxEvents:]
	}
	return h
}

func hostMeta(h *Host) *Host {
	cp := *h
	cp.Samples = nil
	return &cp
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

// GetSamples returns a copy of the base-metric history for one host.
func (s *Store) GetSamples(id string) ([]shared.Sample, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hosts[id]
	if !ok {
		return nil, false
	}
	cp := make([]shared.Sample, len(h.Samples))
	copy(cp, h.Samples)
	return cp, true
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
