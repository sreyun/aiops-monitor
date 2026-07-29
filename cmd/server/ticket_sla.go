package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// TicketSLAPolicy maps priority → response/resolve deadlines (minutes).
type TicketSLAPolicy struct {
	ResponseMin map[string]int `json:"response_min,omitempty"` // p1→15 …
	ResolveMin  map[string]int `json:"resolve_min,omitempty"`
	AutoAssign  bool           `json:"auto_assign,omitempty"`
}

func defaultTicketSLA() TicketSLAPolicy {
	return TicketSLAPolicy{
		ResponseMin: map[string]int{"p1": 15, "p2": 60, "p3": 240, "p4": 1440},
		ResolveMin:  map[string]int{"p1": 240, "p2": 1440, "p3": 4320, "p4": 10080},
		AutoAssign:  true,
	}
}

func (cs *ConfigStore) TicketSLA() TicketSLAPolicy {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	p := cs.cfg.TicketSLA
	d := defaultTicketSLA()
	if p.ResponseMin == nil {
		p.ResponseMin = d.ResponseMin
	}
	if p.ResolveMin == nil {
		p.ResolveMin = d.ResolveMin
	}
	return p
}

func (cs *ConfigStore) SetTicketSLA(p TicketSLAPolicy) error {
	d := defaultTicketSLA()
	if p.ResponseMin == nil {
		p.ResponseMin = d.ResponseMin
	}
	if p.ResolveMin == nil {
		p.ResolveMin = d.ResolveMin
	}
	cs.mu.Lock()
	cs.cfg.TicketSLA = p
	cs.mu.Unlock()
	return cs.save()
}

func (s *Server) handleGetTicketSLA(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.TicketSLA())
}

func (s *Server) handleSetTicketSLA(w http.ResponseWriter, r *http.Request) {
	var p TicketSLAPolicy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.cfg.SetTicketSLA(p); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "policy": s.cfg.TicketSLA()})
}

func applyTicketSLADeadlines(t *Ticket, policy TicketSLAPolicy) {
	if t == nil || t.CreatedAt <= 0 {
		return
	}
	pri := strings.ToLower(t.Priority)
	if t.DueAt <= 0 {
		if m := policy.ResolveMin[pri]; m > 0 {
			t.DueAt = t.CreatedAt + int64(m)*60
		}
	}
}

// finalizeNewTicket applies SLA deadlines + OnCall auto-assign after Create.
func (s *Server) finalizeNewTicket(tk Ticket) Ticket {
	s.tickets.ApplyPostCreate(tk.ID, func(t *Ticket) {
		applyTicketSLADeadlines(t, s.cfg.TicketSLA())
		s.autoAssignTicket(t)
	})
	if updated, ok := s.tickets.Get(tk.ID); ok {
		return updated
	}
	return tk
}

func (s *Server) autoAssignTicket(t *Ticket) {
	if t == nil || strings.TrimSpace(t.Assignee) != "" {
		return
	}
	policy := s.cfg.TicketSLA()
	if !policy.AutoAssign {
		return
	}
	at := time.Now()
	for _, sch := range s.cfg.OnCallSchedules() {
		if who := resolveOnCallUser(sch, at); who != "" {
			t.Assignee = who
			return
		}
	}
}

func (s *Server) ticketSLABreaches() []map[string]any {
	policy := s.cfg.TicketSLA()
	now := time.Now().Unix()
	var out []map[string]any
	for _, t := range s.tickets.List("") {
		st := strings.ToLower(t.Status)
		if st == "resolved" || st == "closed" {
			continue
		}
		pri := strings.ToLower(t.Priority)
		respMin := policy.ResponseMin[pri]
		resolveMin := policy.ResolveMin[pri]
		respDue := t.CreatedAt + int64(respMin)*60
		resolveDue := t.DueAt
		if resolveDue <= 0 && resolveMin > 0 {
			resolveDue = t.CreatedAt + int64(resolveMin)*60
		}
		responded := st == "in_progress" || t.Assignee != ""
		if respMin > 0 && !responded && now > respDue {
			out = append(out, map[string]any{
				"ticket_id": t.ID, "title": t.Title, "priority": t.Priority,
				"breach": "response", "due_at": respDue, "age_min": (now - t.CreatedAt) / 60,
			})
		}
		if resolveDue > 0 && now > resolveDue {
			out = append(out, map[string]any{
				"ticket_id": t.ID, "title": t.Title, "priority": t.Priority,
				"breach": "resolve", "due_at": resolveDue, "age_min": (now - t.CreatedAt) / 60,
			})
		}
	}
	return out
}

func (s *Server) handleTicketSLABreaches(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"breaches": s.ticketSLABreaches(), "policy": s.cfg.TicketSLA()})
}

func (s *Server) startTicketSLAWatcher() {
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			breaches := s.ticketSLABreaches()
			for _, b := range breaches {
				slog.Warn("ticket SLA breach", "ticket_id", b["ticket_id"], "breach", b["breach"], "title", b["title"])
				s.store.AddLog(LogEntry{
					Kind: KindSystem, Level: "warning", Actor: "sla-watcher",
					Message: fmt.Sprintf("工单 SLA 违约 #%v %v (%v)", b["ticket_id"], b["title"], b["breach"]),
				})
			}
		}
	}()
}
