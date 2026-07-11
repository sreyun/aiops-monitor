package main

import "testing"

func TestMessageHub(t *testing.T) {
	h := newMessageHub()
	h.push("incident", "critical", "T1", "b1", "sre", "1")
	h.push("ai", "info", "T2", "b2", "sre", "1")
	if got := h.unreadCount(); got != 2 {
		t.Fatalf("unread = %d, want 2", got)
	}
	list := h.list(10)
	if len(list) != 2 || list[0].Title != "T2" { // newest-first
		t.Fatalf("list order wrong: %+v", list)
	}
	h.markRead([]int64{list[0].ID})
	if got := h.unreadCount(); got != 1 {
		t.Fatalf("unread after markRead = %d, want 1", got)
	}
	h.markAllRead()
	if got := h.unreadCount(); got != 0 {
		t.Fatalf("unread after markAllRead = %d, want 0", got)
	}
	// export/import round-trip preserves messages + continues nextID
	h2 := newMessageHub()
	h2.importMsgs(h.export())
	if h2.unreadCount() != 0 || len(h2.list(10)) != 2 {
		t.Fatalf("import round-trip failed")
	}
	h2.push("system", "info", "T3", "", "", "")
	if h2.list(1)[0].ID != 3 { // nextID continued from imported max (2)
		t.Fatalf("nextID not continued after import, got %d", h2.list(1)[0].ID)
	}
}

// TestIncidentFiresMessageHook verifies the SRE→message wiring: raising an
// incident fires onChange(isNew=true) exactly once (dedup-aware), and a recovery
// fires onChange(isNew=false).
func TestIncidentFiresMessageHook(t *testing.T) {
	m := newIncidentManager()
	var raised []Incident
	m.onChange = func(inc Incident, isNew bool) {
		if isNew {
			raised = append(raised, inc)
		}
	}
	m.raise("cpu/h1", "CPU 95%", "critical", "alert", "h1", "web-01", "cpu")
	m.raise("cpu/h1", "CPU 95%", "critical", "alert", "h1", "web-01", "cpu") // dedup → no new
	if len(raised) != 1 {
		t.Fatalf("onChange(new) fired %d times, want 1 (dedup)", len(raised))
	}
	if raised[0].Title != "CPU 95%" || raised[0].Severity != "critical" {
		t.Fatalf("wrong incident passed to hook: %+v", raised[0])
	}

	var resolved int
	m.onChange = func(inc Incident, isNew bool) {
		if !isNew {
			resolved++
		}
	}
	m.resolveByKey("cpu/h1", "recovered")
	if resolved != 1 {
		t.Fatalf("resolveByKey fired onChange(resolved) %d times, want 1", resolved)
	}
}
