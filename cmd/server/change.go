package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// Change windows + change records (ops change management).
// ============================================================================

type ChangeWindow struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Start      int64    `json:"start"`
	End        int64    `json:"end"`
	HostIDs    []string `json:"host_ids,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Freeze     bool     `json:"freeze"` // block unapproved auto-remediation
	Note       string   `json:"note,omitempty"`
	UpdatedAt  int64    `json:"updated_at,omitempty"`
}

type ChangeRecord struct {
	ID                int64    `json:"id"`
	Title             string   `json:"title"`
	Summary           string   `json:"summary,omitempty"`
	Kind              string   `json:"kind"`   // deploy|config|infra|emergency|other
	Status            string   `json:"status"` // planned|in_progress|completed|rolled_back|cancelled
	Risk              string   `json:"risk"`   // low|medium|high
	HostIDs           []string `json:"host_ids,omitempty"`
	Services          []string `json:"services,omitempty"`
	WindowID          string   `json:"window_id,omitempty"`
	StartedAt         int64    `json:"started_at"`
	EndedAt           int64    `json:"ended_at,omitempty"`
	Author            string   `json:"author,omitempty"`
	Approver          string   `json:"approver,omitempty"`
	ExternalRef       string   `json:"external_ref,omitempty"`
	LinkedIncidentIDs []int64  `json:"linked_incident_ids,omitempty"`
	CreatedAt         int64    `json:"created_at"`
	UpdatedAt         int64    `json:"updated_at"`
}

type changeManager struct {
	mu      sync.Mutex
	records []ChangeRecord
	nextID  int64
}

func newChangeManager() *changeManager {
	return &changeManager{nextID: 1}
}

func (m *changeManager) Export() []ChangeRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ChangeRecord, len(m.records))
	copy(out, m.records)
	return out
}

func (m *changeManager) Import(list []ChangeRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append([]ChangeRecord(nil), list...)
	var maxID int64
	for _, r := range m.records {
		if r.ID > maxID {
			maxID = r.ID
		}
	}
	if maxID >= m.nextID {
		m.nextID = maxID
	}
}

func (m *changeManager) List() []ChangeRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ChangeRecord, len(m.records))
	copy(out, m.records)
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt > out[j].StartedAt })
	return out
}

func (m *changeManager) Get(id int64) (ChangeRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.records {
		if r.ID == id {
			return r, true
		}
	}
	return ChangeRecord{}, false
}

func (m *changeManager) Upsert(in ChangeRecord, actor string) (ChangeRecord, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return ChangeRecord{}, fmt.Errorf("变更标题不能为空")
	}
	if in.Kind == "" {
		in.Kind = "other"
	}
	if in.Status == "" {
		in.Status = "planned"
	}
	if in.Risk == "" {
		in.Risk = "medium"
	}
	now := time.Now().Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	if in.ID == 0 {
		m.nextID++
		in.ID = m.nextID
		in.CreatedAt = now
		if in.Author == "" {
			in.Author = actor
		}
		if in.StartedAt == 0 {
			in.StartedAt = now
		}
		in.UpdatedAt = now
		m.records = append(m.records, in)
		return in, nil
	}
	for i := range m.records {
		if m.records[i].ID == in.ID {
			in.CreatedAt = m.records[i].CreatedAt
			if in.Author == "" {
				in.Author = m.records[i].Author
			}
			in.UpdatedAt = now
			m.records[i] = in
			return in, nil
		}
	}
	return ChangeRecord{}, fmt.Errorf("变更不存在")
}

func (m *changeManager) LinkIncident(changeID, incidentID int64) (ChangeRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.records {
		if m.records[i].ID != changeID {
			continue
		}
		for _, id := range m.records[i].LinkedIncidentIDs {
			if id == incidentID {
				return m.records[i], nil
			}
		}
		m.records[i].LinkedIncidentIDs = append(m.records[i].LinkedIncidentIDs, incidentID)
		m.records[i].UpdatedAt = time.Now().Unix()
		return m.records[i], nil
	}
	return ChangeRecord{}, fmt.Errorf("变更不存在")
}

func (m *changeManager) RelatedToHosts(hostIDs []string, since int64) []ChangeRecord {
	want := map[string]bool{}
	for _, h := range hostIDs {
		if h != "" {
			want[h] = true
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []ChangeRecord
	for _, r := range m.records {
		if r.StartedAt < since && (r.EndedAt == 0 || r.EndedAt < since) {
			continue
		}
		if len(want) == 0 {
			out = append(out, r)
			continue
		}
		hit := false
		for _, h := range r.HostIDs {
			if want[h] {
				hit = true
				break
			}
		}
		if hit {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt > out[j].StartedAt })
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

func (cs *ConfigStore) ChangeWindows() []ChangeWindow {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make([]ChangeWindow, len(cs.cfg.ChangeWindows))
	copy(out, cs.cfg.ChangeWindows)
	return out
}

func (cs *ConfigStore) UpsertChangeWindow(w ChangeWindow) (ChangeWindow, error) {
	w.Name = strings.TrimSpace(w.Name)
	if w.Name == "" {
		return ChangeWindow{}, fmt.Errorf("变更窗名称不能为空")
	}
	if w.End > 0 && w.End < w.Start {
		return ChangeWindow{}, fmt.Errorf("结束时间必须晚于开始时间")
	}
	w.UpdatedAt = time.Now().Unix()
	cs.mu.Lock()
	if w.ID == "" {
		w.ID = genToken()[:8]
		cs.cfg.ChangeWindows = append(cs.cfg.ChangeWindows, w)
	} else {
		found := false
		for i := range cs.cfg.ChangeWindows {
			if cs.cfg.ChangeWindows[i].ID == w.ID {
				cs.cfg.ChangeWindows[i] = w
				found = true
				break
			}
		}
		if !found {
			cs.cfg.ChangeWindows = append(cs.cfg.ChangeWindows, w)
		}
	}
	cs.mu.Unlock()
	return w, cs.save()
}

func (cs *ConfigStore) DeleteChangeWindow(id string) error {
	cs.mu.Lock()
	kept := cs.cfg.ChangeWindows[:0]
	for _, w := range cs.cfg.ChangeWindows {
		if w.ID != id {
			kept = append(kept, w)
		}
	}
	cs.cfg.ChangeWindows = kept
	cs.mu.Unlock()
	return cs.save()
}

// activeFreezeWindow reports whether auto-remediation should be frozen for host.
func (cs *ConfigStore) activeFreezeWindow(hostID, category string, now int64) (ChangeWindow, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, w := range cs.cfg.ChangeWindows {
		if !w.Freeze {
			continue
		}
		if now < w.Start || (w.End > 0 && now > w.End) {
			continue
		}
		if len(w.HostIDs) == 0 && len(w.Categories) == 0 {
			return w, true
		}
		for _, h := range w.HostIDs {
			if h == hostID {
				return w, true
			}
		}
		for _, c := range w.Categories {
			if c != "" && c == category {
				return w, true
			}
		}
	}
	return ChangeWindow{}, false
}
