package main

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// Incident — the hub of the SRE workflow.
//
// An incident is opened when an alert fires (or an SLO burns, or an operator
// creates one manually) and auto-resolves when the underlying alert recovers.
// It carries a timeline that records every state change and every automated
// remediation attempt, so the whole lifecycle of a problem lives in one place.
// ============================================================================

// IncidentEvent is one entry on an incident's timeline.
type IncidentEvent struct {
	Ts          int64        `json:"ts"`
	Kind        string       `json:"kind"` // created|fired|recovered|acked|resolved|remediation|comment|escalated|note
	Actor       string       `json:"actor,omitempty"`
	Text        string       `json:"text"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// Incident is a tracked problem with a lifecycle and timeline.
type Incident struct {
	ID         int64           `json:"id"`
	Key        string          `json:"key,omitempty"` // dedup key (alertKey / "slo/<id>"); empty for manual
	Title      string          `json:"title"`
	Severity   string          `json:"severity"` // critical|warning|info
	Status     string          `json:"status"`   // open|acknowledged|resolved
	Source     string          `json:"source"`   // alert|slo|manual
	HostID     string          `json:"host_id,omitempty"`
	Hostname   string          `json:"hostname,omitempty"`
	Type       string          `json:"type,omitempty"` // alert type (cpu/memory/...) for auto incidents
	Assignee   string          `json:"assignee,omitempty"`
	Timeline   []IncidentEvent `json:"timeline"`
	CreatedAt  int64           `json:"created_at"`
	AckedAt    int64           `json:"acked_at,omitempty"`
	ResolvedAt int64           `json:"resolved_at,omitempty"`
	TicketID   int64           `json:"ticket_id,omitempty"` // linked work order, if escalated
}

const incidentHistoryCap = 500 // resolved incidents beyond this are trimmed

// incidentManager stores incidents in memory (persisted via the DB snapshot) and
// keeps an index of open incidents by dedup key so a flapping alert reuses one
// incident instead of spawning a new one each cycle.
type incidentManager struct {
	mu        sync.Mutex
	incidents []Incident
	nextID    int64
	openByKey map[string]int64 // dedup key -> open incident ID
	// onChange is invoked (outside the lock) whenever an incident is newly
	// raised (isNew=true) or resolved (isNew=false). Wired in wireSRE to push
	// a notification-center message + trigger auto AI diagnosis. May be nil.
	onChange func(inc Incident, isNew bool)
}

func newIncidentManager() *incidentManager {
	return &incidentManager{nextID: 1, openByKey: map[string]int64{}}
}

// find returns a pointer to the incident with id (caller holds mu).
func (m *incidentManager) find(id int64) *Incident {
	for i := range m.incidents {
		if m.incidents[i].ID == id {
			return &m.incidents[i]
		}
	}
	return nil
}

// raise opens (or reuses) an incident for a dedup key. When an open incident with
// the same non-empty key already exists it is reused (so a flapping condition
// doesn't spawn duplicates). Returns the incident ID and whether it was newly
// created. It returns an ID (not a *Incident) on purpose: the slice can realloc
// on the next append, so a pointer must never escape the lock.
func (m *incidentManager) raise(key, title, severity, source, hostID, hostname, typ string) (int64, bool) {
	m.mu.Lock()
	if key != "" {
		if id, ok := m.openByKey[key]; ok {
			if inc := m.find(id); inc != nil {
				m.mu.Unlock()
				return inc.ID, false
			}
			delete(m.openByKey, key) // stale index entry; fall through to create
		}
	}
	m.nextID++
	inc := Incident{
		ID: m.nextID, Key: key, Title: title, Severity: severity, Status: "open",
		Source: source, HostID: hostID, Hostname: hostname, Type: typ,
		CreatedAt: time.Now().Unix(),
	}
	addEventLocked(&inc, "created", source, title)
	m.incidents = append(m.incidents, inc)
	if key != "" {
		m.openByKey[key] = inc.ID
	}
	m.trimLocked()
	newInc := inc // value copy — safe to hand to the callback after unlocking
	m.mu.Unlock()
	if m.onChange != nil {
		m.onChange(newInc, true)
	}
	return newInc.ID, true
}

// resolveByKey resolves the open incident bound to key (used when an alert
// recovers). Returns the resolved incident's ID, or 0 if none was open.
func (m *incidentManager) resolveByKey(key, note string) int64 {
	if key == "" {
		return 0
	}
	m.mu.Lock()
	id, ok := m.openByKey[key]
	if !ok {
		m.mu.Unlock()
		return 0
	}
	delete(m.openByKey, key)
	inc := m.find(id)
	if inc == nil || inc.Status == "resolved" {
		m.mu.Unlock()
		return 0
	}
	inc.Status = "resolved"
	inc.ResolvedAt = time.Now().Unix()
	addEventLocked(inc, "recovered", "alert-engine", note)
	resolved := *inc
	m.mu.Unlock()
	if m.onChange != nil {
		m.onChange(resolved, false)
	}
	return resolved.ID
}

// OnAlertTransition is the notifier hook: a firing alert opens/reuses an incident,
// a recovering alert resolves the matching one. Returns the affected incident ID
// (0 if none).
func (m *incidentManager) OnAlertTransition(a Alert, key string, firing bool) int64 {
	if firing {
		sev := a.Level
		if sev == "" {
			sev = "warning"
		}
		id, _ := m.raise(key, a.Message, sev, "alert", a.HostID, a.Hostname, a.Type)
		return id
	}
	return m.resolveByKey(key, a.Message)
}

// AddEvent appends a timeline entry to an incident by id (used by remediation).
func (m *incidentManager) AddEvent(id int64, kind, actor, text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if inc := m.find(id); inc != nil {
		addEventLocked(inc, kind, actor, text)
	}
}

// Ack marks an incident acknowledged.
func (m *incidentManager) Ack(id int64, actor string) (Incident, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inc := m.find(id)
	if inc == nil {
		return Incident{}, false
	}
	if inc.Status == "open" {
		inc.Status = "acknowledged"
		inc.AckedAt = time.Now().Unix()
		addEventLocked(inc, "acked", actor, "")
	}
	return *inc, true
}

// Resolve marks an incident resolved manually.
func (m *incidentManager) Resolve(id int64, actor string) (Incident, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inc := m.find(id)
	if inc == nil {
		return Incident{}, false
	}
	if inc.Status != "resolved" {
		inc.Status = "resolved"
		inc.ResolvedAt = time.Now().Unix()
		addEventLocked(inc, "resolved", actor, "")
		if inc.Key != "" {
			delete(m.openByKey, inc.Key)
		}
	}
	return *inc, true
}

// addEventLocked appends a timeline entry (caller holds mu).
func addEventLocked(inc *Incident, kind, actor, text string, atts ...[]Attachment) {
	ev := IncidentEvent{
		Ts: time.Now().Unix(), Kind: kind, Actor: actor, Text: text,
	}
	if len(atts) > 0 {
		ev.Attachments = sanitizeAttachments(atts[0])
	}
	inc.Timeline = append(inc.Timeline, ev)
	if len(inc.Timeline) > 200 {
		inc.Timeline = inc.Timeline[len(inc.Timeline)-200:]
	}
}

// Comment appends an operator note to the timeline（允许纯附件、无正文）。
func (m *incidentManager) Comment(id int64, actor, text string, atts []Attachment) (Incident, bool) {
	text = strings.TrimSpace(text)
	atts = sanitizeAttachments(atts)
	if text == "" && len(atts) == 0 {
		return Incident{}, false
	}
	if text == "" && len(atts) > 0 {
		text = Tz("ticket.evt_attach", len(atts))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	inc := m.find(id)
	if inc == nil {
		return Incident{}, false
	}
	addEventLocked(inc, "comment", actor, text, atts)
	return *inc, true
}

// SetTicket links an incident to a work order.
func (m *incidentManager) SetTicket(id, ticketID int64, actor string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if inc := m.find(id); inc != nil {
		inc.TicketID = ticketID
		addEventLocked(inc, "escalated", actor, "")
	}
}

// CreateManual opens an operator-declared incident.
func (m *incidentManager) CreateManual(title, severity, hostID, hostname, actor string) Incident {
	if severity == "" {
		severity = "warning"
	}
	id, _ := m.raise("", title, severity, "manual", hostID, hostname, "")
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.find(id)
	if p == nil {
		return Incident{}
	}
	if len(p.Timeline) > 0 {
		p.Timeline[len(p.Timeline)-1].Actor = actor
	}
	return *p
}

// Get returns one incident by id.
func (m *incidentManager) Get(id int64) (Incident, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if inc := m.find(id); inc != nil {
		return *inc, true
	}
	return Incident{}, false
}

// List returns incidents newest-first.
func (m *incidentManager) List() []Incident {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Incident, len(m.incidents))
	copy(out, m.incidents)
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// OpenCount returns how many incidents are not yet resolved (for nav badges).
func (m *incidentManager) OpenCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for i := range m.incidents {
		if m.incidents[i].Status != "resolved" {
			n++
		}
	}
	return n
}

// trimLocked drops the oldest resolved incidents beyond the cap (caller holds mu).
func (m *incidentManager) trimLocked() {
	if len(m.incidents) <= incidentHistoryCap {
		return
	}
	kept := m.incidents[:0]
	drop := len(m.incidents) - incidentHistoryCap
	for _, inc := range m.incidents {
		if drop > 0 && inc.Status == "resolved" {
			drop--
			continue
		}
		kept = append(kept, inc)
	}
	m.incidents = kept
}

// Export/Import bridge the manager to the DB snapshot.
func (m *incidentManager) Export() []Incident {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Incident, len(m.incidents))
	copy(out, m.incidents)
	return out
}

func (m *incidentManager) Import(list []Incident) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.incidents = make([]Incident, len(list))
	copy(m.incidents, list)
	m.openByKey = map[string]int64{}
	var maxID int64
	for _, inc := range m.incidents {
		if inc.ID > maxID {
			maxID = inc.ID
		}
		if inc.Key != "" && inc.Status != "resolved" {
			m.openByKey[inc.Key] = inc.ID
		}
	}
	m.nextID = maxID
}
