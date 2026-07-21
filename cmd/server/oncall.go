package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// On-call schedules + escalation policies + runtime pages.
// ============================================================================

type OnCallLayer struct {
	Name      string   `json:"name"`
	Rotation  string   `json:"rotation"`             // weekly | daily
	HandoffAt string   `json:"handoff_at,omitempty"` // HH:MM local
	Members   []string `json:"members"`              // usernames
	StartAt   int64    `json:"start_at,omitempty"`   // rotation anchor unix
}

type OnCallOverride struct {
	Start  int64  `json:"start"`
	End    int64  `json:"end"`
	User   string `json:"user"`
	Reason string `json:"reason,omitempty"`
}

type OnCallSchedule struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Timezone  string           `json:"timezone,omitempty"` // default Asia/Shanghai
	Layers    []OnCallLayer    `json:"layers,omitempty"`
	Overrides []OnCallOverride `json:"overrides,omitempty"`
	UpdatedAt int64            `json:"updated_at,omitempty"`
}

type EscalationTarget struct {
	ScheduleID string   `json:"schedule_id,omitempty"`
	Layer      int      `json:"layer,omitempty"` // 0=primary
	Users      []string `json:"users,omitempty"`
}

type EscalationStep struct {
	AfterSec int              `json:"after_sec"` // delay from previous step (0 = immediate)
	Target   EscalationTarget `json:"target"`
	Channels []string         `json:"channels,omitempty"` // feishu|dingtalk|email|sms|voicecall|webhook
}

type EscalationPolicy struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Enabled   bool             `json:"enabled"`
	Steps     []EscalationStep `json:"steps,omitempty"`
	UpdatedAt int64            `json:"updated_at,omitempty"`
}

// OnCallPage is a live escalation for one incident.
type OnCallPage struct {
	ID             int64    `json:"id"`
	IncidentID     int64    `json:"incident_id"`
	PolicyID       string   `json:"policy_id,omitempty"`
	ScheduleID     string   `json:"schedule_id,omitempty"`
	Step           int      `json:"step"`
	Status         string   `json:"status"` // pending|acked|cancelled|exhausted
	Notified       []string `json:"notified,omitempty"`
	NextEscalateAt int64    `json:"next_escalate_at,omitempty"`
	CreatedAt      int64    `json:"created_at"`
	AckedAt        int64    `json:"acked_at,omitempty"`
	AckedBy        string   `json:"acked_by,omitempty"`
}

type onCallManager struct {
	mu     sync.Mutex
	pages  []OnCallPage
	nextID int64
}

func newOnCallManager() *onCallManager {
	return &onCallManager{nextID: 1}
}

func (m *onCallManager) Export() []OnCallPage {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]OnCallPage, len(m.pages))
	copy(out, m.pages)
	return out
}

func (m *onCallManager) Import(list []OnCallPage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pages = append([]OnCallPage(nil), list...)
	var maxID int64
	for _, p := range m.pages {
		if p.ID > maxID {
			maxID = p.ID
		}
	}
	if maxID >= m.nextID {
		m.nextID = maxID
	}
}

func (m *onCallManager) List(openOnly bool) []OnCallPage {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]OnCallPage, 0, len(m.pages))
	for _, p := range m.pages {
		if openOnly && p.Status != "pending" {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (m *onCallManager) AckByIncident(incidentID int64, actor string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().Unix()
	for i := range m.pages {
		if m.pages[i].IncidentID == incidentID && m.pages[i].Status == "pending" {
			m.pages[i].Status = "acked"
			m.pages[i].AckedAt = now
			m.pages[i].AckedBy = actor
			m.pages[i].NextEscalateAt = 0
		}
	}
}

func (m *onCallManager) CancelByIncident(incidentID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.pages {
		if m.pages[i].IncidentID == incidentID && m.pages[i].Status == "pending" {
			m.pages[i].Status = "cancelled"
			m.pages[i].NextEscalateAt = 0
		}
	}
}

func (m *onCallManager) Start(inc Incident, policy EscalationPolicy, scheduleID string, firstUsers []string) OnCallPage {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().Unix()
	m.nextID++
	delay := 0
	if len(policy.Steps) > 0 {
		delay = policy.Steps[0].AfterSec
	}
	p := OnCallPage{
		ID: m.nextID, IncidentID: inc.ID, PolicyID: policy.ID, ScheduleID: scheduleID,
		Step: 0, Status: "pending", Notified: append([]string(nil), firstUsers...),
		CreatedAt: now, NextEscalateAt: now + int64(delay),
	}
	if delay == 0 && len(policy.Steps) > 1 {
		p.NextEscalateAt = now + int64(policy.Steps[1].AfterSec)
		p.Step = 0
	}
	m.pages = append(m.pages, p)
	return p
}

func (m *onCallManager) DuePages(now int64) []OnCallPage {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []OnCallPage
	for _, p := range m.pages {
		if p.Status == "pending" && p.NextEscalateAt > 0 && p.NextEscalateAt <= now {
			out = append(out, p)
		}
	}
	return out
}

func (m *onCallManager) Advance(id int64, nextStep int, users []string, nextAt int64, exhausted bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.pages {
		if m.pages[i].ID != id {
			continue
		}
		if exhausted {
			m.pages[i].Status = "exhausted"
			m.pages[i].NextEscalateAt = 0
		} else {
			m.pages[i].Step = nextStep
			m.pages[i].Notified = append(m.pages[i].Notified, users...)
			m.pages[i].NextEscalateAt = nextAt
		}
		return
	}
}

// resolveOnCallUser picks the current primary (layer 0) member for a schedule at time t.
func resolveOnCallUser(sch OnCallSchedule, at time.Time) string {
	for _, o := range sch.Overrides {
		if at.Unix() >= o.Start && at.Unix() < o.End && o.User != "" {
			return o.User
		}
	}
	if len(sch.Layers) == 0 || len(sch.Layers[0].Members) == 0 {
		return ""
	}
	layer := sch.Layers[0]
	loc := time.Local
	if sch.Timezone != "" {
		if l, err := time.LoadLocation(sch.Timezone); err == nil {
			loc = l
		}
	}
	local := at.In(loc)
	anchor := layer.StartAt
	if anchor <= 0 {
		anchor = local.Unix() - 7*86400
	}
	var idx int
	switch strings.ToLower(layer.Rotation) {
	case "daily":
		days := int((local.Unix() - anchor) / 86400)
		if days < 0 {
			days = 0
		}
		idx = days % len(layer.Members)
	default: // weekly
		weeks := int((local.Unix() - anchor) / (7 * 86400))
		if weeks < 0 {
			weeks = 0
		}
		idx = weeks % len(layer.Members)
	}
	return layer.Members[idx]
}

func resolveLayerUsers(sch OnCallSchedule, layerIdx int, at time.Time) []string {
	if layerIdx < 0 || layerIdx >= len(sch.Layers) {
		return nil
	}
	// For simplicity escalate to the whole layer membership (pager duty style secondary),
	// with current rotation member first.
	layer := sch.Layers[layerIdx]
	cur := ""
	if layerIdx == 0 {
		cur = resolveOnCallUser(sch, at)
	} else if len(layer.Members) > 0 {
		cur = layer.Members[0]
	}
	out := []string{}
	if cur != "" {
		out = append(out, cur)
	}
	for _, u := range layer.Members {
		if u != cur {
			out = append(out, u)
		}
	}
	return out
}

func (cs *ConfigStore) OnCallSchedules() []OnCallSchedule {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make([]OnCallSchedule, len(cs.cfg.OnCallSchedules))
	copy(out, cs.cfg.OnCallSchedules)
	return out
}

func (cs *ConfigStore) UpsertOnCallSchedule(s OnCallSchedule) (OnCallSchedule, error) {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return OnCallSchedule{}, fmt.Errorf("排班名称不能为空")
	}
	if s.Timezone == "" {
		s.Timezone = "Asia/Shanghai"
	}
	s.UpdatedAt = time.Now().Unix()
	cs.mu.Lock()
	if s.ID == "" {
		s.ID = genToken()[:8]
		cs.cfg.OnCallSchedules = append(cs.cfg.OnCallSchedules, s)
	} else {
		found := false
		for i := range cs.cfg.OnCallSchedules {
			if cs.cfg.OnCallSchedules[i].ID == s.ID {
				cs.cfg.OnCallSchedules[i] = s
				found = true
				break
			}
		}
		if !found {
			cs.cfg.OnCallSchedules = append(cs.cfg.OnCallSchedules, s)
		}
	}
	cs.mu.Unlock()
	return s, cs.save()
}

func (cs *ConfigStore) DeleteOnCallSchedule(id string) error {
	cs.mu.Lock()
	kept := cs.cfg.OnCallSchedules[:0]
	for _, s := range cs.cfg.OnCallSchedules {
		if s.ID != id {
			kept = append(kept, s)
		}
	}
	cs.cfg.OnCallSchedules = kept
	cs.mu.Unlock()
	return cs.save()
}

func (cs *ConfigStore) EscalationPolicies() []EscalationPolicy {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make([]EscalationPolicy, len(cs.cfg.EscalationPolicies))
	copy(out, cs.cfg.EscalationPolicies)
	return out
}

func (cs *ConfigStore) UpsertEscalationPolicy(p EscalationPolicy) (EscalationPolicy, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return EscalationPolicy{}, fmt.Errorf("升级策略名称不能为空")
	}
	p.UpdatedAt = time.Now().Unix()
	cs.mu.Lock()
	if p.ID == "" {
		p.ID = genToken()[:8]
		cs.cfg.EscalationPolicies = append(cs.cfg.EscalationPolicies, p)
	} else {
		found := false
		for i := range cs.cfg.EscalationPolicies {
			if cs.cfg.EscalationPolicies[i].ID == p.ID {
				cs.cfg.EscalationPolicies[i] = p
				found = true
				break
			}
		}
		if !found {
			cs.cfg.EscalationPolicies = append(cs.cfg.EscalationPolicies, p)
		}
	}
	cs.mu.Unlock()
	return p, cs.save()
}

func (cs *ConfigStore) DeleteEscalationPolicy(id string) error {
	cs.mu.Lock()
	kept := cs.cfg.EscalationPolicies[:0]
	for _, p := range cs.cfg.EscalationPolicies {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	cs.cfg.EscalationPolicies = kept
	cs.mu.Unlock()
	return cs.save()
}

func (cs *ConfigStore) FindOnCallSchedule(id string) (OnCallSchedule, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, s := range cs.cfg.OnCallSchedules {
		if s.ID == id {
			return s, true
		}
	}
	return OnCallSchedule{}, false
}

func (cs *ConfigStore) FindEscalationPolicy(id string) (EscalationPolicy, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, p := range cs.cfg.EscalationPolicies {
		if p.ID == id {
			return p, true
		}
	}
	// default: first enabled
	for _, p := range cs.cfg.EscalationPolicies {
		if p.Enabled {
			return p, true
		}
	}
	return EscalationPolicy{}, false
}

func (cs *ConfigStore) DefaultEscalationPolicy() (EscalationPolicy, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, p := range cs.cfg.EscalationPolicies {
		if p.Enabled && len(p.Steps) > 0 {
			return p, true
		}
	}
	return EscalationPolicy{}, false
}
