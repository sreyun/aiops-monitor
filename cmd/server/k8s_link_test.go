package main

import (
	"path/filepath"
	"testing"
)

func TestMatchHostForK8sNode(t *testing.T) {
	s := &Server{store: NewStore()}
	h := s.store.RegisterHost("h1", "node-a.example.com", "fp1")
	h.IP = "10.0.0.5"
	if got := s.matchHostForK8sNode("node-a", nil); got == nil || got.ID != "h1" {
		t.Fatalf("short name match: %+v", got)
	}
	if got := s.matchHostForK8sNode("other", []string{"10.0.0.5"}); got == nil || got.ID != "h1" {
		t.Fatalf("ip match: %+v", got)
	}
}

func TestNormalizeTopoRefResourceKinds(t *testing.T) {
	cases := map[string]string{
		"vm:h1/g1":           "vm:h1/g1",
		"container:h1/abc":   "container:h1/abc",
		"pod:c1/default/web": "pod:c1/default/web",
		"host:h1":            "host:h1",
	}
	for in, want := range cases {
		if got := normalizeTopoRef(in); got != want {
			t.Fatalf("%s → %s, want %s", in, got, want)
		}
	}
}

func TestLocateResourceHost(t *testing.T) {
	cfg, err := NewConfigStore(filepath.Join(t.TempDir(), "cfg.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: NewStore(), cfg: cfg}
	s.store.RegisterHost("h1", "web01", "fp1")
	res := s.locateResource("host:h1")
	if res.HostID != "h1" || res.Hostname != "web01" {
		t.Fatalf("%+v", res)
	}
	if res.Summary == "" {
		t.Fatal("empty summary")
	}
}
