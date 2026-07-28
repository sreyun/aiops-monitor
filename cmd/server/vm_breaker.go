package main

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// vmCircuitBreaker trips after consecutive failures; half-opens after cool-down.
type vmCircuitBreaker struct {
	mu           sync.Mutex
	failures     int
	openUntil    time.Time
	threshold    int
	coolDown     time.Duration
	halfOpenProbe bool
}

func newVMCircuitBreaker() *vmCircuitBreaker {
	th := 5
	if v := strings.TrimSpace(os.Getenv("AIOPS_VM_BREAKER_THRESHOLD")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			th = n
		}
	}
	cd := 30 * time.Second
	if v := strings.TrimSpace(os.Getenv("AIOPS_VM_BREAKER_COOLDOWN_SEC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cd = time.Duration(n) * time.Second
		}
	}
	return &vmCircuitBreaker{threshold: th, coolDown: cd}
}

func (b *vmCircuitBreaker) allow() bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if now.Before(b.openUntil) {
		return false
	}
	if !b.openUntil.IsZero() && now.After(b.openUntil) {
		// half-open: allow one probe
		b.halfOpenProbe = true
		b.openUntil = time.Time{}
	}
	return true
}

func (b *vmCircuitBreaker) success() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.failures = 0
	b.halfOpenProbe = false
	b.openUntil = time.Time{}
	b.mu.Unlock()
}

func (b *vmCircuitBreaker) failure() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.failures++
	if b.failures >= b.threshold {
		b.openUntil = time.Now().Add(b.coolDown)
		b.failures = 0
	}
	b.halfOpenProbe = false
	b.mu.Unlock()
}

func vmQueryTimeout() time.Duration {
	sec := 15
	if v := strings.TrimSpace(os.Getenv("AIOPS_VM_QUERY_TIMEOUT_SEC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			sec = n
		}
	}
	return time.Duration(sec) * time.Second
}

func (v *vmWriter) doVMRequest(req *http.Request) (*http.Response, error) {
	if v == nil {
		return nil, context.Canceled
	}
	if v.breaker == nil {
		v.breaker = newVMCircuitBreaker()
	}
	if !v.breaker.allow() {
		return nil, errVMCircuitOpen
	}
	ctx, cancel := context.WithTimeout(req.Context(), vmQueryTimeout())
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := v.httpc.Do(req)
	if err != nil {
		v.breaker.failure()
		return nil, err
	}
	if resp.StatusCode >= 500 {
		v.breaker.failure()
		return resp, nil
	}
	v.breaker.success()
	return resp, nil
}

type vmCircuitOpenError struct{}

func (vmCircuitOpenError) Error() string { return "victoria metrics circuit open" }

var errVMCircuitOpen = vmCircuitOpenError{}
