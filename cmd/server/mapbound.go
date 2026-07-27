package main

import "time"

// pruneExpiredUnixMap deletes entries whose unix expiry is before now.
// If still over max after prune, deletes arbitrary surplus keys (insertion order
// is undefined — acceptable for ephemeral OAuth/SMS state).
func pruneExpiredUnixMap[V any](m map[string]V, now int64, max int, expiry func(V) int64) {
	if m == nil {
		return
	}
	for k, v := range m {
		if expiry(v) > 0 && expiry(v) < now {
			delete(m, k)
		}
	}
	if max <= 0 || len(m) <= max {
		return
	}
	n := len(m) - max
	for k := range m {
		delete(m, k)
		n--
		if n <= 0 {
			break
		}
	}
}

// pruneExpiredTimeMap mirrors pruneExpiredUnixMap for time.Time expiries.
func pruneExpiredTimeMap[V any](m map[string]V, now time.Time, max int, expiry func(V) time.Time) {
	if m == nil {
		return
	}
	for k, v := range m {
		exp := expiry(v)
		if !exp.IsZero() && now.After(exp) {
			delete(m, k)
		}
	}
	if max <= 0 || len(m) <= max {
		return
	}
	n := len(m) - max
	for k := range m {
		delete(m, k)
		n--
		if n <= 0 {
			break
		}
	}
}

// capStringMap drops surplus keys when len exceeds max (no expiry semantics).
func capStringMap[V any](m map[string]V, max int) {
	if m == nil || max <= 0 || len(m) <= max {
		return
	}
	n := len(m) - max
	for k := range m {
		delete(m, k)
		n--
		if n <= 0 {
			break
		}
	}
}

const (
	maxOIDCStates   = 4096
	maxSSOStates    = 4096
	maxSMSCodes     = 2048
	maxSMSLast      = 2048
	maxTermAttempts = 4096
	maxTOTPUsed     = 8192
)
