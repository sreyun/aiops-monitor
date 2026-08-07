package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aiops-monitor/shared"
)

func TestRequireHostAccessOnMetricsAndCategory(t *testing.T) {
	dir := t.TempDir()
	cfg, err := NewConfigStore(filepath.Join(dir, "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	s := &Server{cfg: cfg, store: store, auth: NewAuth(cfg)}

	_ = store.RegisterHost("host-a", "alpha", "fp-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	_ = store.RegisterHost("host-b", "beta", "fp-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	_, _ = store.UpsertAuthenticated(shared.Report{
		HostID: "host-a", Hostname: "alpha", Fingerprint: "fp-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Metrics: shared.Metrics{CPUPercent: 10},
	}, "fp-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	_, _ = store.UpsertAuthenticated(shared.Report{
		HostID: "host-b", Hostname: "beta", Fingerprint: "fp-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Metrics: shared.Metrics{CPUPercent: 20},
	}, "fp-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	salt := genToken()[:16]
	op := AccountConfig{
		Username: "scoped", DisplayName: "Scoped", Role: RoleOperator,
		Salt: salt, Hash: hashPassword("Passw0rd!", salt),
		AllowedHostIDs: []string{"host-a"},
	}
	cfg.cfg.Users = append(cfg.cfg.Users, op)
	_ = cfg.save()

	tok := s.auth.issueSession("scoped")
	withSession := func(req *http.Request) *http.Request {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
		return req
	}

	rr := httptest.NewRecorder()
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/v1/hosts/host-a/metrics", nil))
	req.SetPathValue("id", "host-a")
	s.handleHostMetrics(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("host-a metrics: want 200 got %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = withSession(httptest.NewRequest(http.MethodGet, "/api/v1/hosts/host-b/metrics", nil))
	req.SetPathValue("id", "host-b")
	s.handleHostMetrics(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("host-b metrics: want 403 got %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = withSession(httptest.NewRequest(http.MethodPost, "/api/v1/hosts/host-b/category", strings.NewReader(`{"category":"x"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "host-b")
	s.handleSetCategory(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("set category: want 403 got %d", rr.Code)
	}
}

func TestFilterAlertsForScopedUser(t *testing.T) {
	dir := t.TempDir()
	cfg, err := NewConfigStore(filepath.Join(dir, "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	s := &Server{cfg: cfg, store: store, auth: NewAuth(cfg)}

	salt := genToken()[:16]
	op := AccountConfig{
		Username: "viewer1", Role: RoleViewer,
		Salt: salt, Hash: hashPassword("Passw0rd!", salt),
		AllowedHostIDs: []string{"h1"},
	}
	cfg.cfg.Users = append(cfg.cfg.Users, op)

	tok := s.auth.issueSession("viewer1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})

	in := []Alert{
		{HostID: "h1", Type: "cpu", Level: "warning"},
		{HostID: "h2", Type: "cpu", Level: "critical"},
		{HostID: "", Type: "check", Level: "warning"},
	}
	out := s.filterAlertsForUser(req, in)
	if len(out) != 2 {
		t.Fatalf("want 2 alerts, got %d: %+v", len(out), out)
	}
	for _, a := range out {
		if a.HostID == "h2" {
			t.Fatal("h2 alert must be filtered")
		}
	}
}

func TestForbidSreyunToolHostAccess(t *testing.T) {
	dir := t.TempDir()
	cfg, err := NewConfigStore(filepath.Join(dir, "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	_ = store.RegisterHost("host-a", "alpha", "fp-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	_ = store.RegisterHost("host-b", "beta", "fp-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	s := &Server{cfg: cfg, store: store}
	h := &SreyunCore{s: s}

	salt := genToken()[:16]
	cfg.cfg.Users = append(cfg.cfg.Users, AccountConfig{
		Username: "scoped", Role: RoleOperator,
		Salt: salt, Hash: hashPassword("Passw0rd!", salt),
		AllowedHostIDs: []string{"host-a"},
	})

	if msg := h.forbidToolHostAccess("scoped", "query_metrics", map[string]any{"host_id": "host-b"}); msg == "" {
		t.Fatal("expected denial for host-b")
	}
	if msg := h.forbidToolHostAccess("scoped", "query_metrics", map[string]any{"host_id": "host-a"}); msg != "" {
		t.Fatalf("host-a should be allowed: %s", msg)
	}
}

func TestUpsertAuthenticatedReturnsSnapshot(t *testing.T) {
	store := NewStore()
	_ = store.RegisterHost("h1", "n1", "fp-cccccccccccccccccccccccccccccccc")
	h1, ok := store.UpsertAuthenticated(shared.Report{
		HostID: "h1", Hostname: "n1", Fingerprint: "fp-cccccccccccccccccccccccccccccccc",
		AgentVersion: "1.0.0",
		Metrics:      shared.Metrics{CPUPercent: 1},
	}, "fp-cccccccccccccccccccccccccccccccc")
	if !ok || h1 == nil {
		t.Fatal("upsert failed")
	}
	h1.AgentVersion = "mutated"
	h2, _ := store.GetHost("h1")
	if h2.AgentVersion == "mutated" {
		t.Fatal("mutating upsert result must not affect store")
	}
}

func TestAddObserverAfterClose(t *testing.T) {
	m := newTermManager()
	sess := m.create("h1", "host", "op")
	sess.close()
	time.Sleep(10 * time.Millisecond)
	if _, ok := m.addObserver(sess.id); ok {
		t.Fatal("addObserver on closed session must fail")
	}
}

func TestDefaultAccountMustChangePassword(t *testing.T) {
	acc := defaultAccount()
	if !acc.MustChangePassword {
		t.Fatal("default admin must require password change")
	}
}

func TestForwardCreateRuleRequiresWhitelistOnNonLoopback(t *testing.T) {
	dir := t.TempDir()
	cfg, err := NewConfigStore(filepath.Join(dir, "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg.cfg.ForwardListen = "0.0.0.0"
	m := newForwardManager(cfg)
	_, err = m.createRule("h1", "n1", 3306, 0, "0.0.0.0", "tcp", "", "op", "", false, nil)
	if err == nil {
		t.Fatal("expected whitelist required error")
	}
}

func scopedOperatorServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	cfg, err := NewConfigStore(filepath.Join(dir, "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	_ = store.RegisterHost("host-a", "alpha", "fp-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	_ = store.RegisterHost("host-b", "beta", "fp-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	salt := genToken()[:16]
	cfg.cfg.Users = append(cfg.cfg.Users, AccountConfig{
		Username: "scoped", Role: RoleOperator,
		Salt: salt, Hash: hashPassword("Passw0rd!", salt),
		AllowedHostIDs: []string{"host-a"},
	})
	_ = cfg.save()
	auth := NewAuth(cfg)
	s := &Server{
		cfg: cfg, store: store, auth: auth,
		incidents: newIncidentManager(),
		term:      newTermManager(),
		logs:      newLogStore(),
		oncall:    newOnCallManager(),
	}
	return s, auth.issueSession("scoped")
}

func TestIncidentHostScopeRBAC(t *testing.T) {
	s, tok := scopedOperatorServer(t)
	incA := s.incidents.CreateManual("a down", "warning", "host-a", "alpha", "admin")
	incB := s.incidents.CreateManual("b down", "critical", "host-b", "beta", "admin")

	withSession := func(req *http.Request) *http.Request {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
		return req
	}

	rr := httptest.NewRecorder()
	s.handleListIncidents(rr, withSession(httptest.NewRequest(http.MethodGet, "/api/v1/incidents", nil)))
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var listed []Incident
	if err := json.NewDecoder(rr.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != incA.ID {
		t.Fatalf("list should only include host-a incident, got %+v", listed)
	}

	rr = httptest.NewRecorder()
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/v1/incidents/"+itoa64(incB.ID), nil))
	req.SetPathValue("id", itoa64(incB.ID))
	s.handleGetIncident(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("get out-of-scope: want 403 got %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = withSession(httptest.NewRequest(http.MethodPost, "/api/v1/incidents/"+itoa64(incB.ID)+"/resolve", strings.NewReader(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", itoa64(incB.ID))
	s.handleResolveIncident(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("resolve out-of-scope: want 403 got %d", rr.Code)
	}
	if got, _ := s.incidents.Get(incB.ID); got.Status == "resolved" {
		t.Fatal("out-of-scope resolve must not mutate incident")
	}

	rr = httptest.NewRecorder()
	req = withSession(httptest.NewRequest(http.MethodGet, "/api/v1/incidents/"+itoa64(incA.ID), nil))
	req.SetPathValue("id", itoa64(incA.ID))
	s.handleGetIncident(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get in-scope: want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestLogSearchHostScopeRBAC(t *testing.T) {
	s, tok := scopedOperatorServer(t)
	s.logs.ingest("host-a", "alpha", []shared.LogLine{{Ts: time.Now().Unix(), Level: "error", Message: "secret-a"}})
	s.logs.ingest("host-b", "beta", []shared.LogLine{{Ts: time.Now().Unix(), Level: "error", Message: "secret-b-password"}})

	withSession := func(req *http.Request) *http.Request {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
		return req
	}

	rr := httptest.NewRecorder()
	s.handleSearchLogs(rr, withSession(httptest.NewRequest(http.MethodGet, "/api/v1/logs?level=error", nil)))
	if rr.Code != http.StatusOK {
		t.Fatalf("search status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Items []StoredLog `json:"items"`
		Total int         `json:"total"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Total != 1 || len(out.Items) != 1 || out.Items[0].HostID != "host-a" {
		t.Fatalf("scoped search leaked host-b logs: %+v", out)
	}

	rr = httptest.NewRecorder()
	s.handleSearchLogs(rr, withSession(httptest.NewRequest(http.MethodGet, "/api/v1/logs?host=host-b", nil)))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("explicit host-b search: want 403 got %d", rr.Code)
	}
}

func TestTerminalSessionHostScopeRBAC(t *testing.T) {
	s, tok := scopedOperatorServer(t)
	_ = s.term.create("host-a", "alpha", "admin")
	sessB := s.term.create("host-b", "beta", "admin")
	s.auth.markTerminalVerified(withCookie(httptest.NewRequest(http.MethodGet, "/", nil), tok))

	withSession := func(req *http.Request) *http.Request {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
		return req
	}
	// Ensure terminal secondary verification is marked on the live session.
	reqMark := withSession(httptest.NewRequest(http.MethodGet, "/", nil))
	s.auth.markTerminalVerified(reqMark)

	rr := httptest.NewRecorder()
	s.handleListTerminalSessions(rr, withSession(httptest.NewRequest(http.MethodGet, "/api/v1/terminal/sessions", nil)))
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d", rr.Code)
	}
	var sessions []termSessionInfo
	if err := json.NewDecoder(rr.Body).Decode(&sessions); err != nil {
		t.Fatal(err)
	}
	for _, sess := range sessions {
		if sess.HostID == "host-b" {
			t.Fatalf("list leaked host-b session: %+v", sess)
		}
	}

	rr = httptest.NewRecorder()
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/v1/terminal/sessions/"+sessB.id+"/replay", nil))
	req.SetPathValue("id", sessB.id)
	s.handleTerminalReplay(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("replay out-of-scope: want 403 got %d body=%s", rr.Code, rr.Body.String())
	}
}

func withCookie(r *http.Request, tok string) *http.Request {
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
	return r
}

func itoa64(v int64) string {
	return strconv.FormatInt(v, 10)
}
