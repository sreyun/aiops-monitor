package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestClamavFreshnessGrading pins the thresholds that decide whether a "clean"
// ClamAV result can be trusted. ClamAV publishes signatures several times a
// day, so a week of silence means freshclam is broken or blocked.
func TestClamavFreshnessGrading(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name      string
		age       int
		updated   time.Time
		wantFind  bool
		wantLevel string
		wantID    string
	}{
		{"fresh today", 0, now, false, "", ""},
		{"six days is still acceptable", 6, now.AddDate(0, 0, -6), false, "", ""},
		{"one week warns", 7, now.AddDate(0, 0, -7), true, "medium", "clamav_db_stale"},
		{"a month is a real gap", 31, now.AddDate(0, 0, -31), true, "high", "clamav_db_stale"},
		{"no database found", 0, time.Time{}, true, "medium", "clamav_db_age_unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := clamavFreshnessFinding(tc.age, tc.updated)
			if ok != tc.wantFind {
				t.Fatalf("finding=%v, want %v (%+v)", ok, tc.wantFind, f)
			}
			if !ok {
				return
			}
			if f.Level != tc.wantLevel {
				t.Errorf("level = %q, want %q", f.Level, tc.wantLevel)
			}
			if f.ID != tc.wantID {
				t.Errorf("id = %q, want %q", f.ID, tc.wantID)
			}
			if f.Suggest == "" {
				t.Error("a stale-signature finding must tell the operator what to do")
			}
		})
	}
}

// TestClamavStaleFindingNamesTheDate: an operator triaging this needs the
// actual last-update timestamp, not just "it is old".
func TestClamavStaleFindingNamesTheDate(t *testing.T) {
	updated := time.Date(2026, 1, 2, 3, 4, 0, 0, time.Local)
	f, ok := clamavFreshnessFinding(45, updated)
	if !ok {
		t.Fatal("45-day-old database produced no finding")
	}
	if !strings.Contains(f.Detail, "2026-01-02 03:04") {
		t.Errorf("detail %q does not carry the last-update timestamp", f.Detail)
	}
	if !strings.Contains(f.Title, "45") {
		t.Errorf("title %q does not state the age", f.Title)
	}
}

// TestClamavDBFreshnessPicksNewestSignatureFile checks the directory scan
// itself: main.cvd is updated rarely, daily.cld constantly — the freshness of
// the set is the newest file, not the oldest.
func TestClamavDBFreshnessPicksNewestSignatureFile(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().AddDate(0, 0, -100)
	recent := time.Now().AddDate(0, 0, -1)

	write := func(name string, mod time.Time) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("main.cvd", old)
	write("daily.cld", recent)
	write("freshclam.log", time.Now()) // not a signature file — must be ignored

	newest := newestSignatureIn(dir)
	if newest.IsZero() {
		t.Fatal("no signature file found")
	}
	if diff := newest.Sub(recent); diff > time.Second || diff < -time.Second {
		t.Errorf("newest = %v, want the daily.cld mtime %v", newest, recent)
	}
}
