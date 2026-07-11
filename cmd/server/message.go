package main

import (
	"sort"
	"sync"
	"time"
)

// ============================================================================
// Message hub — the unified notification center.
//
// A "message" is a user-facing notification that aggregates notable happenings
// across the platform: SRE incidents (raised / recovered), AI inspection
// findings and auto-diagnoses, remediation runs needing approval, SLO breaches,
// and system operations. Each message carries a deep-link (view + ref) so the
// bell/inbox in the topbar can navigate straight to the originating module —
// this is what "打通 SRE 并关联相关模块" means in practice.
//
// Persisted to PostgreSQL via the kv_state "messages" blob (like alert states
// and sessions), so the inbox + read state survive a restart.
// ============================================================================

// Message is one entry in the notification center.
type Message struct {
	ID    int64  `json:"id"`
	Ts    int64  `json:"ts"`
	Type  string `json:"type"`  // incident|alert|slo|remediation|ai|ticket|system
	Level string `json:"level"` // critical|warning|info|success
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	View  string `json:"view,omitempty"` // frontend view to open on click (sre/alerts/logs/...)
	Ref   string `json:"ref,omitempty"`  // optional id within the view (e.g. incident id)
	Read  bool   `json:"read"`
}

const messageCap = 500 // ring buffer: oldest messages beyond this are dropped

type messageHub struct {
	mu     sync.Mutex
	msgs   []Message
	nextID int64
}

func newMessageHub() *messageHub { return &messageHub{nextID: 0} }

// push appends a message (newest kept at the tail; list() returns newest-first),
// capping history. Safe to call from any module — it only takes the hub's own
// lock, never a caller's, so it can be invoked inside other managers' callbacks.
func (h *messageHub) push(typ, level, title, body, view, ref string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	h.msgs = append(h.msgs, Message{
		ID: h.nextID, Ts: time.Now().Unix(), Type: typ, Level: level,
		Title: title, Body: body, View: view, Ref: ref,
	})
	if len(h.msgs) > messageCap {
		h.msgs = h.msgs[len(h.msgs)-messageCap:]
	}
}

// list returns messages newest-first (optionally capped to limit).
func (h *messageHub) list(limit int) []Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Message, len(h.msgs))
	copy(out, h.msgs)
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (h *messageHub) unreadCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for i := range h.msgs {
		if !h.msgs[i].Read {
			n++
		}
	}
	return n
}

func (h *messageHub) markRead(ids []int64) {
	if len(ids) == 0 {
		return
	}
	set := make(map[int64]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.msgs {
		if set[h.msgs[i].ID] {
			h.msgs[i].Read = true
		}
	}
}

func (h *messageHub) markAllRead() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.msgs {
		h.msgs[i].Read = true
	}
}

// export / importMsgs bridge the hub to PostgreSQL (kv_state "messages").
func (h *messageHub) export() []Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Message, len(h.msgs))
	copy(out, h.msgs)
	return out
}

func (h *messageHub) importMsgs(list []Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.msgs = make([]Message, len(list))
	copy(h.msgs, list)
	var maxID int64
	for _, m := range h.msgs {
		if m.ID > maxID {
			maxID = m.ID
		}
	}
	h.nextID = maxID
}
