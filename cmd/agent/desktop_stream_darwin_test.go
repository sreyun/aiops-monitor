//go:build darwin

package main

import (
	"strings"
	"testing"
	"time"
)

func TestDarwinDenyCache(t *testing.T) {
	darwinCapCacheMu.Lock()
	darwinDenyErr = nil
	darwinDenyAt = time.Time{}
	darwinPermOK = false
	darwinCapCacheMu.Unlock()

	err := darwinCachedDeny()
	if err != nil {
		t.Fatalf("empty cache should allow: %v", err)
	}
	darwinRememberDeny(errString("desk_perm_denied: test"))
	if err := darwinCachedDeny(); err == nil || !strings.Contains(err.Error(), "desk_perm_denied") {
		t.Fatalf("expected cached deny, got %v", err)
	}
	darwinRememberPermOK()
	if err := darwinCachedDeny(); err != nil {
		t.Fatalf("perm OK should clear deny: %v", err)
	}
}

func errString(s string) error { return &simpleErr{s} }

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }

func TestDarwinMonitorsFromQuartzOrFallback(t *testing.T) {
	// May be empty in CI without GUI; ensure helper does not panic.
	_ = darwinMonitorsFromQuartz()
	c := &darwinCapture{}
	c.refreshMonitorsCached()
	if len(c.monitors) == 0 {
		t.Fatal("expected at least fallback monitor")
	}
	if c.monitor == 0 {
		t.Fatal("expected monitor selected")
	}
}

func TestDeskH264UsableDoesNotForceProbe(t *testing.T) {
	// Reset probe flag for this process test — only safe if Once has not run yet.
	if darwinScrIdxProbed {
		t.Skip("avfoundation already probed in this process")
	}
	if deskH264Usable() {
		t.Fatal("H264 should be false before explicit probe")
	}
}
