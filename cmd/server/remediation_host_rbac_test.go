package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Scoped operators must not propose or approve remediations for hosts outside
// their AllowedHostIDs. Diagnose already enforced requireHostAccess; propose /
// approve / loop actions previously skipped it and could run playbooks fleet-wide.
func TestRemediationProposeApproveRespectsHostScope(t *testing.T) {
	dir := t.TempDir()
	cfg, err := NewConfigStore(filepath.Join(dir, "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	_ = store.RegisterHost("host-a", "alpha", "fp-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	_ = store.RegisterHost("host-b", "beta", "fp-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	s := &Server{
		cfg:         cfg,
		store:       store,
		auth:        NewAuth(cfg),
		playbooks:   newPlaybookManager(cfg),
		incidents:   newIncidentManager(),
		remediation: newRemediationManager(cfg),
	}
	launched := 0
	s.remediation.getPlaybook = s.playbooks.Get
	s.remediation.resolveHost = s.hostByID
	s.remediation.trigger = func(pb Playbook, host *Host, op string, onDone func(ok bool)) int64 {
		launched++
		if onDone != nil {
			onDone(true)
		}
		return int64(launched)
	}

	salt := genToken()[:16]
	cfg.cfg.Users = append(cfg.cfg.Users, AccountConfig{
		Username: "scoped", DisplayName: "Scoped", Role: RoleOperator,
		Salt: salt, Hash: hashPassword("Passw0rd!", salt),
		AllowedHostIDs: []string{"host-a"},
	})
	_ = cfg.save()

	incB := s.incidents.CreateManual("disk full on beta", "critical", "host-b", "beta", "admin")
	s.incidents.AddEventWithCitations(incB.ID, "ai_diagnosis", "AI", "结论：磁盘满。置信度：高",
		[]RAGCitation{{Source: "metrics", Summary: "disk=99%"}})

	tok := s.auth.issueSession("scoped")
	withSession := func(req *http.Request) *http.Request {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
		return req
	}

	body := `{"playbook":{"name":"stop-db","steps":[{"name":"stop","command":"systemctl stop postgresql"}]}}`
	rr := httptest.NewRecorder()
	req := withSession(httptest.NewRequest(http.MethodPost,
		"/api/v1/incidents/"+strconv.FormatInt(incB.ID, 10)+"/remediation-propose",
		strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", strconv.FormatInt(incB.ID, 10))
	s.handleProposeRemediation(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("propose on out-of-scope host: want 403 got %d body=%s", rr.Code, rr.Body.String())
	}

	// Admin-created pending run on host-b must not be approvable by scoped op.
	pb := Playbook{ID: "pb-x", Name: "stop-db", Steps: []PlaybookStep{{Name: "stop", Command: "systemctl stop postgresql"}}}
	run, err := s.remediation.ProposeManual(pb, "host-b", "beta", incB.ID, "admin proposal", "admin")
	if err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	req = withSession(httptest.NewRequest(http.MethodPost,
		"/api/v1/remediation/runs/"+strconv.FormatInt(run.ID, 10)+"/approve", nil))
	req.SetPathValue("id", strconv.FormatInt(run.ID, 10))
	s.handleApproveRemediation(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("approve out-of-scope run: want 403 got %d body=%s", rr.Code, rr.Body.String())
	}
	if launched != 0 {
		t.Fatalf("playbook must not launch for out-of-scope approve, got %d", launched)
	}

	// Same incident via closed-loop action path.
	rr = httptest.NewRecorder()
	req = withSession(httptest.NewRequest(http.MethodPost,
		"/api/v1/incidents/"+strconv.FormatInt(incB.ID, 10)+"/loop/propose",
		strings.NewReader(`{"force":true}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", strconv.FormatInt(incB.ID, 10))
	req.SetPathValue("action", "propose")
	s.handleIncidentLoopAction(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("loop propose out-of-scope: want 403 got %d body=%s", rr.Code, rr.Body.String())
	}

	// In-scope host still works.
	incA := s.incidents.CreateManual("disk full on alpha", "critical", "host-a", "alpha", "admin")
	s.incidents.AddEventWithCitations(incA.ID, "ai_diagnosis", "AI", "结论：磁盘满。置信度：高",
		[]RAGCitation{{Source: "metrics", Summary: "disk=99%"}})
	rr = httptest.NewRecorder()
	req = withSession(httptest.NewRequest(http.MethodPost,
		"/api/v1/incidents/"+strconv.FormatInt(incA.ID, 10)+"/remediation-propose",
		strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", strconv.FormatInt(incA.ID, 10))
	s.handleProposeRemediation(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("propose on in-scope host: want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		OK  bool           `json:"ok"`
		Run RemediationRun `json:"run"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil || !resp.OK || resp.Run.ID == 0 {
		t.Fatalf("unexpected propose response: %s", rr.Body.String())
	}
	rr = httptest.NewRecorder()
	req = withSession(httptest.NewRequest(http.MethodPost,
		"/api/v1/remediation/runs/"+strconv.FormatInt(resp.Run.ID, 10)+"/approve", nil))
	req.SetPathValue("id", strconv.FormatInt(resp.Run.ID, 10))
	s.handleApproveRemediation(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("approve in-scope run: want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	if launched != 1 {
		t.Fatalf("expected one launch for in-scope approve, got %d", launched)
	}
}
