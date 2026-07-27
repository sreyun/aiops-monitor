package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newFeedTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	cs, err := NewConfigStore(filepath.Join(dir, "server.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{
		cfg:   cs,
		store: NewStore(),
		feeds: newFeedManager(filepath.Join(dir, "feeds")),
	}
}

// stopFeedJob cancels a run and waits for the goroutine to unwind, so the
// staging directory is gone before t.TempDir cleanup runs.
func stopFeedJob(t *testing.T, m *feedManager) {
	t.Helper()
	m.cancelJob()
	deadline := time.Now().Add(5 * time.Second)
	for m.jobRunning() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if m.jobRunning() {
		t.Error("update job did not stop after cancel")
	}
}

func decodeFeedStatus(t *testing.T, body []byte) feedStatusResponse {
	t.Helper()
	var out feedStatusResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode status: %v (%s)", err, truncateRun(string(body), 200))
	}
	return out
}

// TestFeedStatusListsWholeCatalog makes sure the panel can render every source,
// including ones that have never been downloaded — otherwise an operator cannot
// discover or enable them.
func TestFeedStatusListsWholeCatalog(t *testing.T) {
	s := newFeedTestServer(t)
	rec := httptest.NewRecorder()
	s.handleSecurityFeedStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/security/feeds", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	got := decodeFeedStatus(t, rec.Body.Bytes())
	if len(got.Sources) != len(feedCatalog) {
		t.Fatalf("returned %d sources, want the full catalog of %d", len(got.Sources), len(feedCatalog))
	}
	var nuclei *FeedSourceView
	for i := range got.Sources {
		if got.Sources[i].ID == "nuclei-templates" {
			nuclei = &got.Sources[i]
		}
	}
	if nuclei == nil {
		t.Fatal("nuclei-templates missing from the status response")
	}
	if !nuclei.Enabled {
		t.Error("nuclei-templates should be enabled on a fresh install")
	}
	if nuclei.Installed {
		t.Error("nothing is downloaded yet, Installed must be false")
	}
}

// TestSaveFeedConfigDistinguishesOmittedFromEmptySources is the bug this shape
// of handler exists to prevent: saving only a proxy must not be read as "the
// operator disabled every source".
func TestSaveFeedConfigDistinguishesOmittedFromEmptySources(t *testing.T) {
	s := newFeedTestServer(t)

	post := func(body string) feedStatusResponse {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/security/feeds/config", strings.NewReader(body))
		rec := httptest.NewRecorder()
		s.handleSetSecurityFeedConfig(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("save %s -> %d: %s", body, rec.Code, rec.Body.String())
		}
		return decodeFeedStatus(t, rec.Body.Bytes())
	}

	got := post(`{"proxy_url":"http://10.0.0.1:8080"}`)
	if got.Config.ProxyURL != "http://10.0.0.1:8080" {
		t.Errorf("proxy not saved: %q", got.Config.ProxyURL)
	}
	enabled := 0
	for _, src := range got.Sources {
		if src.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		t.Fatal("saving a proxy silently disabled every source")
	}

	got = post(`{"sources":[]}`)
	for _, src := range got.Sources {
		if src.Enabled {
			t.Fatalf("an explicit empty list must disable %s", src.ID)
		}
	}
	if got.Config.ProxyURL != "http://10.0.0.1:8080" {
		t.Error("unrelated settings were lost on a partial save")
	}

	got = post(`{"sources":["sqlmap-signatures"],"mirror_prefix":"https://ghfast.top","timeout_sec":99999}`)
	if got.Config.MirrorPrefix != "https://ghfast.top/" {
		t.Errorf("mirror prefix should be normalised with a trailing slash, got %q", got.Config.MirrorPrefix)
	}
	if got.Config.TimeoutSec != 7200 {
		t.Errorf("timeout not clamped: %d", got.Config.TimeoutSec)
	}
}

// TestFeedUpdateIsAsynchronous pins the contract the UI depends on: the request
// returns a job instead of blocking for the length of the download.
func TestFeedUpdateIsAsynchronous(t *testing.T) {
	s := newFeedTestServer(t)
	// Point the mirror at an unroutable address so the run fails fast in the
	// background; the handler must still answer immediately.
	if err := s.cfg.SetSecurityFeeds(SecurityFeedConfig{
		Sources:      []string{"sqlmap-signatures"},
		MirrorPrefix: "http://127.0.0.1:9/",
		TimeoutSec:   120,
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/security/feeds/update", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	s.handleSecurityFeedUpdate(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 Accepted: %s", rec.Code, rec.Body.String())
	}
	var job FeedJob
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatalf("decode job: %v", err)
	}
	if job.ID == "" || job.Total != 1 {
		t.Errorf("job not described for polling: %+v", job)
	}
	stopFeedJob(t, s.feeds)
}

// TestFeedUpdateRejectsSecondJob keeps a double-click from racing two runs into
// the same staging directory.
func TestFeedUpdateRejectsSecondJob(t *testing.T) {
	// The mirror hangs so the first job is guaranteed to still be running when
	// the second request arrives.
	blocked := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-blocked }))
	defer srv.Close()
	defer close(blocked)

	s := newFeedTestServer(t)
	if err := s.cfg.SetSecurityFeeds(SecurityFeedConfig{
		Sources: []string{"sqlmap-signatures"}, MirrorPrefix: srv.URL + "/", TimeoutSec: 300,
	}); err != nil {
		t.Fatal(err)
	}
	src, _ := feedSourceByID("sqlmap-signatures")
	if _, err := s.feeds.runUpdate(s.cfg.SecurityFeeds(), []FeedSource{src}, "tester"); err != nil {
		t.Fatal(err)
	}
	defer stopFeedJob(t, s.feeds)

	rec := httptest.NewRecorder()
	s.handleSecurityFeedUpdate(rec, httptest.NewRequest(http.MethodPost, "/api/v1/security/feeds/update", strings.NewReader("{}")))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 while a job is running: %s", rec.Code, rec.Body.String())
	}
}

// TestFeedCancelWithoutJobIsAConflict avoids reporting success for a cancel
// that did nothing.
func TestFeedCancelWithoutJobIsAConflict(t *testing.T) {
	s := newFeedTestServer(t)
	rec := httptest.NewRecorder()
	s.handleSecurityFeedCancel(rec, httptest.NewRequest(http.MethodPost, "/api/v1/security/feeds/cancel", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

// TestFeedTestRejectsBadProxyBeforeDialing gives the operator the error at the
// form instead of after a timeout.
func TestFeedTestRejectsBadProxyBeforeDialing(t *testing.T) {
	s := newFeedTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/security/feeds/test", strings.NewReader(`{"proxy_url":"ftp://nope:21"}`))
	rec := httptest.NewRecorder()
	s.handleSecurityFeedTest(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}
