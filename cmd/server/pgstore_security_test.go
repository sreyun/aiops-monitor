package main

import (
	"net/url"
	"strings"
	"testing"
)

// TestIsSafeFlowPartitionName locks in the DDL identifier whitelist: only
// flow_records_YYYYMM may ever be interpolated into CREATE TABLE.
func TestIsSafeFlowPartitionName(t *testing.T) {
	ok := []string{"flow_records_202607", "flow_records_209912"}
	for _, n := range ok {
		if !isSafeFlowPartitionName(n) {
			t.Errorf("%q should be accepted", n)
		}
	}
	bad := []string{
		"flow_records_2026",          // too short
		"flow_records_2026070",       // too long
		"flow_records_20260a",        // non-digit
		"flow_records_202607; DROP",  // injection attempt
		"flow_records_202607 OR 1=1", // injection attempt
		"other_202607",               // wrong prefix
		"",                           // empty
	}
	for _, n := range bad {
		if isSafeFlowPartitionName(n) {
			t.Errorf("%q should be rejected", n)
		}
	}
}

// TestApplyPGSafetyTimeouts verifies safe timeouts are injected for both DSN
// formats and that user-provided values are respected (not overwritten).
func TestApplyPGSafetyTimeouts(t *testing.T) {
	// keyword/value DSN
	kw := applyPGSafetyTimeouts("host=db port=5432 dbname=aiops")
	if !strings.Contains(kw, "lock_timeout=15000") {
		t.Errorf("keyword DSN missing lock_timeout: %q", kw)
	}
	if !strings.Contains(kw, "idle_in_transaction_session_timeout=60000") {
		t.Errorf("keyword DSN missing idle_in_transaction_session_timeout: %q", kw)
	}

	// URL DSN
	urlDSN := applyPGSafetyTimeouts("postgres://u:p@db:5432/aiops?sslmode=disable")
	u, err := url.Parse(urlDSN)
	if err != nil {
		t.Fatalf("result not a valid URL: %v", err)
	}
	q := u.Query()
	if q.Get("lock_timeout") != "15000" {
		t.Errorf("URL DSN missing lock_timeout: %q", urlDSN)
	}
	if q.Get("idle_in_transaction_session_timeout") != "60000" {
		t.Errorf("URL DSN missing idle_in_transaction_session_timeout: %q", urlDSN)
	}
	if q.Get("sslmode") != "disable" {
		t.Errorf("existing query param dropped: %q", urlDSN)
	}

	// user-set value must be respected
	kept := applyPGSafetyTimeouts("host=db lock_timeout=5000")
	if strings.Count(kept, "lock_timeout") != 1 {
		t.Errorf("user lock_timeout should not be duplicated/overwritten: %q", kept)
	}
}
