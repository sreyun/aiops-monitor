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

// drainToBrowser collects everything currently queued for the browser.
func drainToBrowser(s *deskSession) [][]byte {
	var out [][]byte
	for {
		select {
		case b := <-s.toBrowser:
			out = append(out, b)
		default:
			return out
		}
	}
}

// A flood of video frames on a full queue must never evict a control/error frame.
// This guards the "一点开就已断开" regression where a racing error frame ('E')
// got dropped and the UI only saw a bare WebSocket close.
func TestDeskEnqueuePreservesControlFrames(t *testing.T) {
	s := &deskSession{toBrowser: make(chan []byte, 4), done: make(chan struct{})}

	// Fill the queue: one error frame first, then video frames.
	if !s.enqueueBrowser([]byte("E{\"error\":\"boom\"}")) {
		t.Fatal("enqueue error frame failed")
	}
	for i := 0; i < 3; i++ {
		if !s.enqueueBrowser([]byte("Kvideo")) {
			t.Fatal("enqueue video failed")
		}
	}
	// Queue is now full (cap 4). Flood with more video frames.
	for i := 0; i < 50; i++ {
		if !s.enqueueBrowser([]byte("Kmore")) {
			t.Fatal("flood enqueue returned done unexpectedly")
		}
	}

	frames := drainToBrowser(s)
	sawError := false
	for _, f := range frames {
		if len(f) > 0 && f[0] == 'E' {
			sawError = true
		}
	}
	if !sawError {
		t.Fatalf("error frame was evicted by video flood; queue=%d frames", len(frames))
	}
}

// Newest video frame should win when the queue overflows with video-only frames.
func TestDeskEnqueuePrefersNewestVideo(t *testing.T) {
	s := &deskSession{toBrowser: make(chan []byte, 2), done: make(chan struct{})}
	if !s.enqueueBrowser([]byte("Kold1")) || !s.enqueueBrowser([]byte("Kold2")) {
		t.Fatal("initial enqueue failed")
	}
	if !s.enqueueBrowser([]byte("Knew")) {
		t.Fatal("overflow enqueue failed")
	}
	frames := drainToBrowser(s)
	last := frames[len(frames)-1]
	if string(last) != "Knew" {
		t.Fatalf("newest video not preserved, got %q", last)
	}
}

// After close(), enqueue must report done so the relay loop can stop.
func TestDeskEnqueueStopsOnDone(t *testing.T) {
	s := &deskSession{toBrowser: make(chan []byte), done: make(chan struct{})}
	s.close()
	if s.enqueueBrowser([]byte("S{}")) {
		t.Fatal("expected enqueueBrowser to return false after close")
	}
	// close is idempotent
	s.close()
}
