package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSMSOTPNotConsumedOnMFARequired(t *testing.T) {
	phone := "13800138000"
	smsCodeMu.Lock()
	smsCodes[phone] = smsCodeEntry{Code: "654321", ExpireAt: time.Now().Add(5 * time.Minute)}
	smsCodeMu.Unlock()
	defer consumeSMSLoginCode(phone)

	cfg := &ConfigStore{cfg: ServerConfig{}}
	s := &Server{cfg: cfg, auth: NewAuth(cfg), store: NewStore()}
	acc := AccountConfig{Username: "smsu", Phone: phone, Role: RoleOperator, MFAEnabled: true, MFASecret: "JBSWY3DPEHPK3PXP"}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/login", nil)
	s.completeLogin(w, r, acc, "", "", "127.0.0.1")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"mfa_required":true`) {
		t.Fatalf("want mfa_required, got %d %s", w.Code, w.Body.String())
	}
	smsCodeMu.Lock()
	_, still := smsCodes[phone]
	smsCodeMu.Unlock()
	if !still {
		t.Fatal("OTP must remain until MFA succeeds / session issued")
	}
}

func TestSMSOTPConsumedAfterSessionIssued(t *testing.T) {
	phone := "13900139000"
	smsCodeMu.Lock()
	smsCodes[phone] = smsCodeEntry{Code: "111222", ExpireAt: time.Now().Add(5 * time.Minute)}
	smsCodeMu.Unlock()

	cfg := &ConfigStore{cfg: ServerConfig{}}
	s := &Server{cfg: cfg, auth: NewAuth(cfg), store: NewStore()}
	acc := AccountConfig{Username: "smsok", Phone: phone, Role: RoleOperator}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/login", nil)
	s.completeLogin(w, r, acc, "", "", "127.0.0.1")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("want ok session, got %d %s", w.Code, w.Body.String())
	}
	smsCodeMu.Lock()
	_, still := smsCodes[phone]
	smsCodeMu.Unlock()
	if still {
		t.Fatal("OTP should be consumed after session issuance")
	}
}

func TestFinalizeAgentUpdateJobWhenVerifiedNoPending(t *testing.T) {
	s := &Server{agentUpdates: newAgentUpdateManager()}
	job := &agentUpdateJob{
		ID: "jf", Status: "running",
		Hosts: []*agentUpdateHostResult{{HostID: "h1", Status: "success"}},
	}
	s.agentUpdates.mu.Lock()
	s.agentUpdates.jobs[job.ID] = job
	s.agentUpdates.mu.Unlock()
	start := time.Now()
	s.finalizeAgentUpdateJobWhenVerified(job)
	if time.Since(start) > 2*time.Second {
		t.Fatal("finalize with zero pending_verify should return immediately")
	}
	if job.Status != "done" {
		t.Fatalf("status=%s", job.Status)
	}
	if job.FinishedAt == 0 {
		t.Fatal("FinishedAt unset")
	}
}

func TestLoopRunDemoSeedsAndAdvances(t *testing.T) {
	dir := t.TempDir()
	cfg, err := NewConfigStore(filepath.Join(dir, "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	salt := genToken()[:16]
	cfg.cfg.Users = append(cfg.cfg.Users, AccountConfig{
		Username: "admin", Role: RoleAdmin, Salt: salt, Hash: hashPassword("Passw0rd!", salt),
	})
	_ = cfg.save()

	im := newIncidentManager()
	inc := im.CreateManual("demo cpu", "warning", "h1", "web-01", "admin")
	auth := NewAuth(cfg)
	s := &Server{
		cfg: cfg, auth: auth, store: NewStore(),
		incidents: im, playbooks: newPlaybookManager(cfg),
		remediation: newRemediationManager(cfg),
	}
	s.remediation.onIncident = s.incidents.AddEvent

	tok := auth.issueSession("admin")
	r := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/1/loop/demo", strings.NewReader("{}"))
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
	w := httptest.NewRecorder()
	s.loopRunDemo(w, r, inc, "admin")
	if w.Code != 200 {
		t.Fatalf("demo status=%d body=%s", w.Code, w.Body.String())
	}
	got, _ := im.Get(inc.ID)
	gate := latestDiagnosisGate(got)
	if !gate.HasDiagnosis {
		t.Fatal("expected seeded diagnosis")
	}
	if got.Loop == nil || got.Loop.Stage == "" || got.Loop.Stage == "idle" {
		t.Fatalf("expected loop advanced, got %+v", got.Loop)
	}
}
