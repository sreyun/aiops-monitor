package main

import "testing"

func TestHostRosterSig(t *testing.T) {
	a := []*Host{{ID: "b"}, {ID: "a"}, {ID: ""}, nil}
	b := []*Host{{ID: "a"}, {ID: "b"}}
	if hostRosterSig(a) != hostRosterSig(b) {
		t.Fatalf("sig should ignore order/empty: %q vs %q", hostRosterSig(a), hostRosterSig(b))
	}
	c := []*Host{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	if hostRosterSig(a) == hostRosterSig(c) {
		t.Fatal("sig should change when a host joins")
	}
	if hostRosterSig(nil) != "" {
		t.Fatalf("empty roster sig=%q", hostRosterSig(nil))
	}
}
