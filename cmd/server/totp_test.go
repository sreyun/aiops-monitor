package main

import (
	"strings"
	"testing"
)

// TestTOTPVectors checks totpAt against RFC 6238 appendix B test vectors (SHA1,
// secret = ASCII "12345678901234567890" = base32 GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ),
// truncated to our 6 digits — proving the HMAC/dynamic-truncation is correct and
// thus that Google Authenticator codes will match.
func TestTOTPVectors(t *testing.T) {
	const sec = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	cases := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1234567890, "005924"},
		{2000000000, "279037"},
	}
	for _, c := range cases {
		if got := totpAt(sec, c.unix); got != c.want {
			t.Errorf("totpAt(T=%d) = %q, want %q", c.unix, got, c.want)
		}
	}
}

func TestSanitizeUsername(t *testing.T) {
	for _, s := range []string{"admin", "ops_wang", "a.b-c", "User123"} {
		if sanitizeUsername(s) == "" {
			t.Errorf("sanitizeUsername(%q) rejected, want accepted", s)
		}
	}
	bad := []string{"", "a", "has space", "quote\"x", "drop;rm", "x$y", strings.Repeat("a", 65)}
	for _, s := range bad {
		if sanitizeUsername(s) != "" {
			t.Errorf("sanitizeUsername(%q) accepted, want rejected", s)
		}
	}
}
