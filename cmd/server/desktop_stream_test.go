package main

import (
	"testing"
	"time"
)

func TestDeskFrameRoundTrip(t *testing.T) {
	f := deskFrame('M', []byte(`{"x":1}`))
	if f[0] != 'M' {
		t.Fatalf("type %v", f[0])
	}
	if len(f) != 3+len([]byte(`{"x":1}`)) {
		t.Fatalf("len %d", len(f))
	}
}

func TestDeskManagerNotifyPending(t *testing.T) {
	m := newDeskManager()
	s := m.create("h1", "host1", "op", "1.1.1.1", "zh")
	if !m.notifyAgent("h1", s.id) {
		t.Fatal("notify failed")
	}
	// no waiter → pending
	m.mu.Lock()
	n := len(m.pendingSessions["h1"])
	m.mu.Unlock()
	if n != 1 {
		t.Fatalf("pending=%d", n)
	}
	m.remove(s.id)
	time.Sleep(10 * time.Millisecond)
}
