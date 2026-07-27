package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeTarGz builds a GitHub-style archive: every entry lives under a
// "<repo>-<ref>/" root that the extractor is expected to strip.
func makeTarGz(t *testing.T, root string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{
			Name:     root + "/" + name,
			Mode:     0o644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %s: %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write body %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestFeedSafeJoinRejectsTraversal is the security boundary of the whole
// updater: a poisoned mirror must not be able to write outside the feed dir.
func TestFeedSafeJoinRejectsTraversal(t *testing.T) {
	dst := t.TempDir()
	bad := []string{
		"../../etc/cron.d/pwn",
		"..\\..\\windows\\system32\\evil.dll",
		"a/../../../../outside.txt",
		"/etc/shadow",
		"",
		".",
	}
	for _, name := range bad {
		if p, ok := feedSafeJoin(dst, name); ok {
			rel, _ := filepath.Rel(dst, p)
			if strings.HasPrefix(rel, "..") {
				t.Errorf("feedSafeJoin(%q) escaped the destination: %s", name, p)
			}
		}
	}
	// A normal nested path must still be accepted.
	if _, ok := feedSafeJoin(dst, "http/cves/2024/CVE-2024-1.yaml"); !ok {
		t.Error("feedSafeJoin rejected a legitimate nested path")
	}
}

// TestExtractTarGzStripsRootAndFiltersIncludes covers the two transformations
// every source relies on: dropping GitHub's wrapper directory and keeping only
// the file types the source declares.
func TestExtractTarGzStripsRootAndFiltersIncludes(t *testing.T) {
	archive := makeTarGz(t, "nuclei-templates-9.9.9", map[string]string{
		"http/cves/CVE-2024-1.yaml": "id: a",
		"http/misc/thing.yml":       "id: b",
		"README.md":                 "# docs",
		"helpers/payload.bin":       "\x00\x01",
	})
	dst := t.TempDir()
	src := FeedSource{ID: "x", Include: []string{".yaml", ".yml"}, MaxFiles: 100, MaxBytes: 1 << 20}

	stats, err := extractTarGz(bytes.NewReader(archive), dst, src)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if stats.Files != 2 {
		t.Errorf("kept %d files, want 2 (yaml/yml only)", stats.Files)
	}
	if _, err := os.Stat(filepath.Join(dst, "http", "cves", "CVE-2024-1.yaml")); err != nil {
		t.Errorf("root wrapper not stripped: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "README.md")); err == nil {
		t.Error("README.md was kept despite the include filter")
	}
}

// TestExtractTarGzHonoursSubdir checks that a source pinned to a subdirectory
// (sqlmap keeps only data/xml) does not pull the whole repository.
func TestExtractTarGzHonoursSubdir(t *testing.T) {
	archive := makeTarGz(t, "sqlmap-master", map[string]string{
		"data/xml/errors.xml":   "<root/>",
		"data/xml/queries.xml":  "<root/>",
		"lib/core/settings.xml": "<root/>",
		"sqlmap.py":             "print()",
	})
	dst := t.TempDir()
	src := FeedSource{ID: "sqlmap", Subdir: "data/xml", Include: []string{".xml"}, MaxFiles: 100, MaxBytes: 1 << 20}

	stats, err := extractTarGz(bytes.NewReader(archive), dst, src)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if stats.Files != 2 {
		t.Errorf("kept %d files, want 2 from data/xml", stats.Files)
	}
	if _, err := os.Stat(filepath.Join(dst, "lib", "core", "settings.xml")); err == nil {
		t.Error("a file outside the configured subdir was extracted")
	}
}

// TestExtractTarGzDropsTraversalEntries proves a malicious archive is skipped
// rather than aborting the run or writing outside the tree.
func TestExtractTarGzDropsTraversalEntries(t *testing.T) {
	archive := makeTarGz(t, "repo-main", map[string]string{
		"../../evil.yaml": "id: pwn",
		"good.yaml":       "id: ok",
	})
	dst := t.TempDir()
	src := FeedSource{ID: "x", Include: []string{".yaml"}, MaxFiles: 100, MaxBytes: 1 << 20}

	stats, err := extractTarGz(bytes.NewReader(archive), dst, src)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if stats.Files != 1 {
		t.Errorf("kept %d files, want only the safe one", stats.Files)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dst), "evil.yaml")); err == nil {
		t.Fatal("traversal entry escaped the destination directory")
	}
}

// TestExtractTarGzEnforcesFileCap makes sure a runaway source cannot fill the
// disk: the walk stops and reports instead of extracting forever.
func TestExtractTarGzEnforcesFileCap(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 20; i++ {
		files[string(rune('a'+i))+".yaml"] = "id: x"
	}
	archive := makeTarGz(t, "repo-main", files)
	src := FeedSource{ID: "x", Include: []string{".yaml"}, MaxFiles: 5, MaxBytes: 1 << 20}

	_, err := extractTarGz(bytes.NewReader(archive), t.TempDir(), src)
	if err == nil || !strings.Contains(err.Error(), "文件数超过上限") {
		t.Fatalf("expected a file-count cap error, got %v", err)
	}
}

func TestFeedURLAppliesMirrorOnlyToGitHub(t *testing.T) {
	cfg := SecurityFeedConfig{MirrorPrefix: "https://ghfast.top"}.withDefaults()
	if got, want := cfg.MirrorPrefix, "https://ghfast.top/"; got != want {
		t.Errorf("mirror prefix not normalised: %q", got)
	}
	gh := "https://github.com/o/r/archive/refs/heads/main.tar.gz"
	if got := feedURL(cfg, gh); got != "https://ghfast.top/"+gh {
		t.Errorf("github URL not mirrored: %s", got)
	}
	other := "https://internal.example.com/x.tar.gz"
	if got := feedURL(cfg, other); got != other {
		t.Errorf("non-github URL should pass through unchanged, got %s", got)
	}
	none := SecurityFeedConfig{}.withDefaults()
	if got := feedURL(none, gh); got != gh {
		t.Errorf("no mirror configured should pass through, got %s", got)
	}
}

func TestFeedHTTPClientRejectsUnsupportedProxy(t *testing.T) {
	for _, bad := range []string{"ftp://10.0.0.1:21", "not a url", "://missing"} {
		if _, err := feedHTTPClient(SecurityFeedConfig{ProxyURL: bad}.withDefaults()); err == nil {
			t.Errorf("proxy %q was accepted", bad)
		}
	}
	for _, ok := range []string{"http://10.0.0.1:8080", "socks5://10.0.0.1:1080", "https://p.example.com:3128"} {
		if _, err := feedHTTPClient(SecurityFeedConfig{ProxyURL: ok}.withDefaults()); err != nil {
			t.Errorf("proxy %q rejected: %v", ok, err)
		}
	}
}

func TestRefPathSegment(t *testing.T) {
	cases := map[string]string{
		"v10.4.6": "tags/v10.4.6",
		"9.9.9":   "tags/9.9.9",
		"master":  "heads/master",
		"main":    "heads/main",
		"":        "heads/master",
	}
	for in, want := range cases {
		if got := refPathSegment(in); got != want {
			t.Errorf("refPathSegment(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestUpdateSourcePublishesAtomically drives a full update against a local
// server (reached through the mirror setting) and checks the live directory is
// only swapped in after a successful extraction.
func TestUpdateSourcePublishesAtomically(t *testing.T) {
	archive := makeTarGz(t, "repo-main", map[string]string{
		"data/xml/errors.xml": "<root><dbms value=\"MySQL\"/></root>",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".tar.gz") {
			w.Write(archive)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	m := newFeedManager(t.TempDir())
	cfg := SecurityFeedConfig{MirrorPrefix: srv.URL + "/", TimeoutSec: 60}.withDefaults()
	client, err := feedHTTPClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	src := FeedSource{ID: "sqlmap-signatures", Name: "sqlmap", Ref: "master",
		Subdir: "data/xml", Include: []string{".xml"}, MaxFiles: 10, MaxBytes: 1 << 20}

	st := m.updateSource(context.Background(), client, cfg, src)
	if st.Error != "" {
		t.Fatalf("update failed: %s", st.Error)
	}
	if st.Files != 1 {
		t.Errorf("files = %d, want 1", st.Files)
	}
	if st.UpdatedAt == 0 {
		t.Error("UpdatedAt not stamped on success")
	}
	live := filepath.Join(m.sourceDir(src.ID), "data", "xml", "errors.xml")
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("content not published: %v", err)
	}
	// No staging leftovers.
	if _, err := os.Stat(m.sourceDir(src.ID) + ".staging"); err == nil {
		t.Error("staging directory was left behind")
	}
}

// TestUpdateSourceKeepsPreviousTreeOnFailure is the availability guarantee: a
// failed refresh must never leave the scanner without a template library.
func TestUpdateSourceKeepsPreviousTreeOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gateway blew up", http.StatusBadGateway)
	}))
	defer srv.Close()

	m := newFeedManager(t.TempDir())
	src := FeedSource{ID: "nuclei-templates", Name: "tpl", Ref: "master", Include: []string{".yaml"}}

	// Seed a working tree.
	live := m.sourceDir(src.ID)
	if err := os.MkdirAll(filepath.Join(live, "http"), 0o750); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(live, "http", "existing.yaml")
	if err := os.WriteFile(marker, []byte("id: keep-me"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := SecurityFeedConfig{MirrorPrefix: srv.URL + "/", TimeoutSec: 30}.withDefaults()
	client, _ := feedHTTPClient(cfg)
	st := m.updateSource(context.Background(), client, cfg, src)

	if st.Error == "" {
		t.Fatal("expected the HTTP 502 to be reported as an error")
	}
	if b, err := os.ReadFile(marker); err != nil || string(b) != "id: keep-me" {
		t.Fatalf("previous template tree was destroyed by a failed update: %v", err)
	}
}

// TestFeedStatesRoundTrip guards the persistence that decides whether the UI
// shows "never updated" after a restart.
func TestFeedStatesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := newFeedManager(dir)
	m.mu.Lock()
	m.states["nuclei-templates"] = FeedState{ID: "nuclei-templates", Ref: "v1.2.3", Files: 42, UpdatedAt: 1700000000}
	m.saveStatesLocked()
	m.mu.Unlock()

	reloaded := newFeedManager(dir)
	got := reloaded.stateOf("nuclei-templates")
	if got.Ref != "v1.2.3" || got.Files != 42 || got.UpdatedAt != 1700000000 {
		t.Fatalf("state did not survive a restart: %+v", got)
	}
}

// TestEnabledSourcesFallsBackToCatalogDefaults distinguishes "never configured"
// (nil) from "explicitly disabled everything" (empty slice) — conflating them
// would silently switch every source off the first time someone saves a proxy.
func TestEnabledSourcesFallsBackToCatalogDefaults(t *testing.T) {
	unset := SecurityFeedConfig{}.withDefaults()
	if len(unset.enabledSources()) == 0 {
		t.Fatal("an unconfigured install should enable the default sources")
	}
	if !unset.sourceEnabled("nuclei-templates") {
		t.Error("nuclei-templates must be on by default")
	}
	explicit := SecurityFeedConfig{Sources: []string{}}.withDefaults()
	if len(explicit.enabledSources()) != 0 {
		t.Error("an explicitly empty list must disable everything")
	}
	picked := SecurityFeedConfig{Sources: []string{"vulhub", "bogus-id"}}.withDefaults()
	got := picked.enabledSources()
	if len(got) != 1 || got[0].ID != "vulhub" {
		t.Errorf("unknown IDs should be dropped, got %+v", got)
	}
}

func TestFeedConfigClampsTimeoutAndInterval(t *testing.T) {
	c := SecurityFeedConfig{TimeoutSec: 5, IntervalHours: 0}.withDefaults()
	if c.TimeoutSec != 120 {
		t.Errorf("TimeoutSec = %d, want clamp to 120", c.TimeoutSec)
	}
	if c.IntervalHours != 24 {
		t.Errorf("IntervalHours = %d, want default 24", c.IntervalHours)
	}
	c = SecurityFeedConfig{TimeoutSec: 999999, IntervalHours: 100000}.withDefaults()
	if c.TimeoutSec != 7200 || c.IntervalHours != 720 {
		t.Errorf("upper clamps not applied: %+v", c)
	}
}

// TestRunUpdateRejectsConcurrentJobs keeps a double-click from starting two
// downloads into the same staging directory.
func TestRunUpdateRejectsConcurrentJobs(t *testing.T) {
	blocked := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocked
	}))
	defer srv.Close()
	defer close(blocked)

	m := newFeedManager(t.TempDir())
	cfg := SecurityFeedConfig{MirrorPrefix: srv.URL + "/", TimeoutSec: 120}.withDefaults()
	src := FeedSource{ID: "vulhub", Name: "vulhub", Ref: "master", Include: []string{".md"}}

	if _, err := m.runUpdate(cfg, []FeedSource{src}, "tester"); err != nil {
		t.Fatalf("first run rejected: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !m.jobRunning() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := m.runUpdate(cfg, []FeedSource{src}, "tester"); err == nil {
		t.Fatal("a second concurrent update was allowed")
	}
	stopFeedJob(t, m)
}

func TestRunUpdateRequiresAtLeastOneSource(t *testing.T) {
	m := newFeedManager(t.TempDir())
	if _, err := m.runUpdate(SecurityFeedConfig{}.withDefaults(), nil, "tester"); err == nil {
		t.Fatal("empty source list should be rejected with a clear message")
	}
}

// TestRefCandidatesOrderAndDedupe pins the retry order. Repos disagree on
// whether the default branch is main or master, and a wrong first guess used to
// leave the whole template library empty.
func TestRefCandidatesOrderAndDedupe(t *testing.T) {
	src := FeedSource{RefFallback: "main"}
	got := src.refCandidates("v10.4.6")
	want := []string{"v10.4.6", "main", "master"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("refCandidates = %v, want %v", got, want)
	}
	pinned := FeedSource{Ref: "master"}
	got = pinned.refCandidates("")
	if strings.Join(got, ",") != "master,main" {
		t.Errorf("a pinned ref must come first and not repeat: %v", got)
	}
}

// TestUpdateSourceFallsBackWhenRefIsMissing covers the failure the pinned
// nuclei tag produced in the field: the release archive 404s and every scan
// then runs against no templates at all.
func TestUpdateSourceFallsBackWhenRefIsMissing(t *testing.T) {
	archive := makeTarGz(t, "nuclei-templates-main", map[string]string{
		"http/cves/CVE-2024-1.yaml": "id: a",
	})
	var tried []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tried = append(tried, r.URL.Path)
		if strings.Contains(r.URL.Path, "refs/heads/main.tar.gz") {
			w.Write(archive)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	m := newFeedManager(t.TempDir())
	cfg := SecurityFeedConfig{MirrorPrefix: srv.URL + "/", TimeoutSec: 60}.withDefaults()
	client, _ := feedHTTPClient(cfg)
	src := FeedSource{ID: "nuclei-templates", Name: "tpl", Ref: "v9.9.9", RefFallback: "main",
		Include: []string{".yaml"}, MaxFiles: 10, MaxBytes: 1 << 20}

	st := m.updateSource(context.Background(), client, cfg, src)
	if st.Error != "" {
		t.Fatalf("update should have recovered on the fallback ref: %s", st.Error)
	}
	if st.Ref != "main" {
		t.Errorf("recorded ref = %q, want the ref that actually worked", st.Ref)
	}
	if len(tried) < 2 {
		t.Errorf("expected a retry after the 404, only saw %v", tried)
	}
}

// TestUpdateSourceDoesNotRetryRefsOnTransportFailure keeps a broken proxy from
// multiplying into one failed request per candidate ref.
func TestUpdateSourceDoesNotRetryRefsOnTransportFailure(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer srv.Close()

	m := newFeedManager(t.TempDir())
	cfg := SecurityFeedConfig{MirrorPrefix: srv.URL + "/", TimeoutSec: 30}.withDefaults()
	client, _ := feedHTTPClient(cfg)
	src := FeedSource{ID: "vulhub", Name: "vulhub", Ref: "master", Include: []string{".md"}}

	st := m.updateSource(context.Background(), client, cfg, src)
	if st.Error == "" {
		t.Fatal("HTTP 502 must be surfaced")
	}
	if hits != 1 {
		t.Errorf("made %d requests, want exactly 1 — a 502 is not a missing ref", hits)
	}
}

// TestUpdateSourceFallsBackToZipball reproduces what acceleration mirrors do
// when they re-pack or interstitial the tarball: the body is not gzip, and the
// updater must recover from the .zip rather than reporting a corrupt archive.
func TestUpdateSourceFallsBackToZipball(t *testing.T) {
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	f, err := zw.Create("repo-master/data/xml/errors.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(`<root><dbms value="MySQL"/></root>`)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".tar.gz"):
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>please wait, redirecting…</html>"))
		case strings.HasSuffix(r.URL.Path, ".zip"):
			w.Write(zbuf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	m := newFeedManager(t.TempDir())
	cfg := SecurityFeedConfig{MirrorPrefix: srv.URL + "/", TimeoutSec: 60}.withDefaults()
	client, _ := feedHTTPClient(cfg)
	src := FeedSource{ID: "sqlmap-signatures", Name: "sqlmap", Ref: "master",
		Subdir: "data/xml", Include: []string{".xml"}, MaxFiles: 10, MaxBytes: 1 << 20}

	st := m.updateSource(context.Background(), client, cfg, src)
	if st.Error != "" {
		t.Fatalf("zip fallback did not kick in: %s", st.Error)
	}
	if !strings.HasSuffix(st.Source, ".zip") {
		t.Errorf("recorded source = %q, want the zip URL that actually worked", st.Source)
	}
	if _, err := os.Stat(filepath.Join(m.sourceDir(src.ID), "data", "xml", "errors.xml")); err != nil {
		t.Fatalf("zip content not published: %v", err)
	}
}

// TestFeedCatalogIsInternallyConsistent catches catalog typos that would only
// surface as a failed download in production.
func TestFeedCatalogIsInternallyConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range feedCatalog {
		if seen[s.ID] {
			t.Errorf("duplicate source ID %q", s.ID)
		}
		seen[s.ID] = true
		if s.Name == "" || s.Desc == "" {
			t.Errorf("%s: name/desc must be filled for the UI", s.ID)
		}
		if !strings.Contains(s.Repo, "/") || strings.Contains(s.Repo, " ") {
			t.Errorf("%s: repo %q is not owner/name", s.ID, s.Repo)
		}
		switch s.Kind {
		case FeedKindNuclei, FeedKindSignature, FeedKindKnowledge:
		default:
			t.Errorf("%s: unknown kind %q", s.ID, s.Kind)
		}
		if s.MaxBytes <= 0 || s.MaxFiles <= 0 {
			t.Errorf("%s: missing size caps — an unbounded source can fill the disk", s.ID)
		}
	}
}
