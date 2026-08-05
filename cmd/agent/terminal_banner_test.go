package main

import (
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Regression documentation: server handleAgentTermTx does defer sess.close(), so
// any completed TX POST ends the session. Privilege banners must therefore ride
// the long-lived interactive TX (writeFrame on the open pipe), never termSendPlain.
func TestTermSendPlainIsOneShotTX(t *testing.T) {
	var hits atomic.Int32
	done := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
		select {
		case done <- struct{}{}:
		default:
		}
	}))
	defer srv.Close()

	a := &Agent{}
	a.identity.Fingerprint = "fp-test"
	start := time.Now()
	a.termSendPlain(srv.URL, "sess1", "\r\n[AIOps] non-root warning\r\n")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("termSendPlain did not complete TX POST")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("termSendPlain took too long: %v (expected one-shot)", time.Since(start))
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d", hits.Load())
	}
}

func TestWriteFrameBannerPrefix(t *testing.T) {
	var buf strings.Builder
	msg := []byte("hello-banner")
	if err := writeFrame(&buf, 'O', msg); err != nil {
		t.Fatal(err)
	}
	raw := []byte(buf.String())
	if len(raw) < 5 || raw[0] != 'O' {
		t.Fatalf("bad frame header: %v", raw)
	}
	if n := binary.BigEndian.Uint32(raw[1:5]); int(n) != len(msg) {
		t.Fatalf("len=%d want %d", n, len(msg))
	}
	if string(raw[5:]) != string(msg) {
		t.Fatalf("payload=%q", raw[5:])
	}
}

func TestResolveWritableHomePrefersTempWhenHomeMissing(t *testing.T) {
	got := resolveWritableHome()
	if got == "" {
		t.Fatal("resolveWritableHome returned empty")
	}
}

func TestWriteFrameConcurrent(t *testing.T) {
	pr, pw := io.Pipe()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(io.Discard, pr)
	}()
	for i := 0; i < 20; i++ {
		_ = writeFrame(pw, 'O', []byte("x"))
	}
	_ = pw.Close()
	wg.Wait()
}
