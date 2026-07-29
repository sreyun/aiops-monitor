package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestSignAWSv4SetsAuthorization(t *testing.T) {
	req, err := http.NewRequest(http.MethodPut, "https://minio.local/bucket/obj.dump", strings.NewReader("abc"))
	if err != nil {
		t.Fatal(err)
	}
	if err := signAWSv4(req, []byte("abc"), "AKIA", "secret", "us-east-1", "s3"); err != nil {
		t.Fatal(err)
	}
	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIA/") {
		t.Fatalf("bad auth: %s", auth)
	}
	if !strings.Contains(auth, "Signature=") {
		t.Fatalf("missing signature: %s", auth)
	}
	if req.Header.Get("X-Amz-Content-Sha256") == "" {
		t.Fatal("missing content sha")
	}
}

func TestS3HostAndPathCustomEndpoint(t *testing.T) {
	host, pathStyle := s3HostAndPath(BackupRemoteConfig{
		Endpoint: "https://oss-cn-hangzhou.aliyuncs.com",
		Bucket:   "b1",
	})
	if host != "oss-cn-hangzhou.aliyuncs.com" || !pathStyle {
		t.Fatalf("got host=%s pathStyle=%v", host, pathStyle)
	}
}

func TestPlaybookDiffSummary(t *testing.T) {
	a := Playbook{Name: "a", Description: "x", Steps: []PlaybookStep{{Name: "1"}}}
	b := Playbook{Name: "b", Description: "y", Steps: []PlaybookStep{{Name: "1"}, {Name: "2"}}}
	s := playbookDiffSummary(a, b)
	if s["name_changed"] != true || s["steps_b"].(int) != 2 {
		t.Fatalf("%v", s)
	}
}

func TestApplyTicketSLADeadlines(t *testing.T) {
	tk := &Ticket{Priority: "p1", CreatedAt: 1_000_000}
	applyTicketSLADeadlines(tk, defaultTicketSLA())
	if tk.DueAt != 1_000_000+240*60 {
		t.Fatalf("due=%d", tk.DueAt)
	}
}

func TestStatusPublicPath(t *testing.T) {
	for _, p := range []string{"/status", "/api/v1/public/status"} {
		if !isPublicPath(httptestGet(p)) {
			t.Fatalf("%s must be public", p)
		}
	}
}

func httptestGet(p string) *http.Request {
	r, _ := http.NewRequest(http.MethodGet, p, nil)
	return r
}

func TestAssignExperimentVariantStable(t *testing.T) {
	v1 := assignExperimentVariant("e1", "alice", map[string]int{"control": 50, "treatment": 50})
	v2 := assignExperimentVariant("e1", "alice", map[string]int{"control": 50, "treatment": 50})
	if v1 != v2 || v1 == "" {
		t.Fatalf("unstable variant %q/%q", v1, v2)
	}
}
