package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// Ticket (work order) — a lightweight issue tracker for follow-up work.
//
// A ticket is an assignable, prioritized work item with a status flow and a
// comment thread. It can be spun off from an incident (for the fix / postmortem)
// or created standalone. Persisted via the DB snapshot so it survives restarts.
// ============================================================================

// TicketComment is one note on a ticket's thread.
type TicketComment struct {
	Ts     int64  `json:"ts"`
	Author string `json:"author"`
	Text   string `json:"text"`
}

// Ticket is a tracked unit of work.
type Ticket struct {
	ID          int64           `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Priority    string          `json:"priority"` // p1|p2|p3|p4
	Status      string          `json:"status"`   // open|in_progress|resolved|closed
	Assignee    string          `json:"assignee,omitempty"`
	Reporter    string          `json:"reporter,omitempty"`
	IncidentID  int64           `json:"incident_id,omitempty"`
	Comments    []TicketComment `json:"comments,omitempty"`
	CreatedAt   int64           `json:"created_at"`
	UpdatedAt   int64           `json:"updated_at"`
}

var ticketPriorities = map[string]bool{"p1": true, "p2": true, "p3": true, "p4": true}
var ticketStatuses = map[string]bool{"open": true, "in_progress": true, "resolved": true, "closed": true}

// ticketManager stores tickets in memory (persisted via the DB snapshot).
type ticketManager struct {
	mu      sync.Mutex
	tickets []Ticket
	nextID  int64
}

func newTicketManager() *ticketManager {
	return &ticketManager{nextID: 1}
}

func (m *ticketManager) find(id int64) *Ticket {
	for i := range m.tickets {
		if m.tickets[i].ID == id {
			return &m.tickets[i]
		}
	}
	return nil
}

// Create adds a new ticket. Priority/status default sensibly when blank/invalid.
func (m *ticketManager) Create(t Ticket, reporter string) (Ticket, error) {
	t.Title = strings.TrimSpace(t.Title)
	if t.Title == "" {
		return Ticket{}, fmt.Errorf("%s", Tz("ticket.title_required"))
	}
	if !ticketPriorities[t.Priority] {
		t.Priority = "p3"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	t.ID = m.nextID
	t.Status = "open"
	t.Reporter = reporter
	t.Comments = nil
	now := time.Now().Unix()
	t.CreatedAt, t.UpdatedAt = now, now
	m.tickets = append(m.tickets, t)
	return t, nil
}

// Update mutates editable fields (title/description/priority/status/assignee).
// A status transition and an assignment are journaled as system comments.
func (m *ticketManager) Update(id int64, in Ticket, actor string) (Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.find(id)
	if t == nil {
		return Ticket{}, fmt.Errorf("%s", Tz("ticket.not_found"))
	}
	if s := strings.TrimSpace(in.Title); s != "" {
		t.Title = s
	}
	t.Description = in.Description
	if ticketPriorities[in.Priority] {
		t.Priority = in.Priority
	}
	if in.Status != "" && ticketStatuses[in.Status] && in.Status != t.Status {
		t.Status = in.Status
		t.Comments = append(t.Comments, TicketComment{Ts: time.Now().Unix(), Author: actor,
			Text: Tz("ticket.evt_status", in.Status)})
	}
	if in.Assignee != t.Assignee {
		t.Assignee = in.Assignee
		who := in.Assignee
		if who == "" {
			who = Tz("ticket.unassigned")
		}
		t.Comments = append(t.Comments, TicketComment{Ts: time.Now().Unix(), Author: actor,
			Text: Tz("ticket.evt_assign", who)})
	}
	t.UpdatedAt = time.Now().Unix()
	return *t, nil
}

// Comment appends a note to a ticket.
func (m *ticketManager) Comment(id int64, author, text string) (Ticket, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Ticket{}, fmt.Errorf("%s", Tz("ticket.comment_required"))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.find(id)
	if t == nil {
		return Ticket{}, fmt.Errorf("%s", Tz("ticket.not_found"))
	}
	t.Comments = append(t.Comments, TicketComment{Ts: time.Now().Unix(), Author: author, Text: text})
	t.UpdatedAt = time.Now().Unix()
	return *t, nil
}

// Delete removes a ticket.
func (m *ticketManager) Delete(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.tickets {
		if m.tickets[i].ID == id {
			m.tickets = append(m.tickets[:i], m.tickets[i+1:]...)
			return
		}
	}
}

func (m *ticketManager) Get(id int64) (Ticket, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t := m.find(id); t != nil {
		return *t, true
	}
	return Ticket{}, false
}

// List returns tickets newest-first.
func (m *ticketManager) List() []Ticket {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Ticket, len(m.tickets))
	copy(out, m.tickets)
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// OpenCount returns tickets that are not resolved/closed (for nav badges).
func (m *ticketManager) OpenCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for i := range m.tickets {
		if m.tickets[i].Status == "open" || m.tickets[i].Status == "in_progress" {
			n++
		}
	}
	return n
}

// Export/Import bridge the manager to the DB snapshot.
func (m *ticketManager) Export() []Ticket {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Ticket, len(m.tickets))
	copy(out, m.tickets)
	return out
}

func (m *ticketManager) Import(list []Ticket) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tickets = make([]Ticket, len(list))
	copy(m.tickets, list)
	var maxID int64
	for _, t := range m.tickets {
		if t.ID > maxID {
			maxID = t.ID
		}
	}
	m.nextID = maxID
}
