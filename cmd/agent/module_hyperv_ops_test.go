//go:build !windows

package main

import "testing"

func TestHyperVOpsUnsupportedOnNonWindows(t *testing.T) {
	if _, code := moduleHyperVPower(map[string]string{"action": "start", "name": "x"}); code == 0 {
		t.Fatal("expected failure on non-windows")
	}
	if _, code := moduleHyperVSet(map[string]string{"name": "x", "processor_count": "2"}); code == 0 {
		t.Fatal("expected failure on non-windows")
	}
}
