package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type k8sErrString string

func (e k8sErrString) Error() string { return string(e) }

func TestFriendlyK8sErrTimeout(t *testing.T) {
	msg := friendlyK8sErr(k8sErrString(`Get "https://192.168.10.81:6443/version": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`))
	if !strings.Contains(msg, "超时") {
		t.Fatalf("expected friendly timeout hint, got %q", msg)
	}
	if strings.Contains(msg, "context deadline exceeded") {
		t.Fatalf("raw error should be rewritten: %q", msg)
	}
}

func TestFriendlyK8sErrRefused(t *testing.T) {
	msg := friendlyK8sErr(k8sErrString("dial tcp 1.2.3.4:6443: connect: connection refused"))
	if !strings.Contains(msg, "拒绝") {
		t.Fatalf("%q", msg)
	}
}

func TestVersionWithTimeoutFailsFast(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		_ = json.NewEncoder(w).Encode(map[string]any{"gitVersion": "v1.29.0"})
	}))
	defer srv.Close()

	cli, err := newK8sRESTClient(K8sClusterConfig{
		APIServer: srv.URL,
		Token:     "t",
		Insecure:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = cli.versionWithTimeout(400 * time.Millisecond)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("probe not fail-fast: %v", elapsed)
	}
}

func TestK8sOverviewSoftFailShape(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
	}))
	defer srv.Close()

	cli, err := newK8sRESTClient(K8sClusterConfig{
		APIServer: srv.URL,
		Token:     "tok",
		Insecure:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = cli.versionWithTimeout(300 * time.Millisecond)
	if err == nil {
		t.Fatal("expected unreachable")
	}
	// Mirror handleK8sOverview soft-fail payload so UI can paint without HTTP 502.
	body := map[string]any{
		"reachable":   false,
		"error":       friendlyK8sErr(err),
		"version":     nil,
		"nodes":       map[string]any{"total": 0, "ready": 0},
		"pods":        map[string]any{"total": 0, "running": 0},
		"deployments": map[string]any{"total": 0},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["reachable"] != false {
		t.Fatalf("%v", decoded)
	}
	errMsg, _ := decoded["error"].(string)
	if !strings.Contains(errMsg, "超时") {
		t.Fatalf("error=%q", errMsg)
	}
}
