package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- TOTP/HOTP(RFC 6238/4226)标准强制用 HMAC-SHA1
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP (RFC 6238) — Google Authenticator-compatible one-time passwords, used as
// the optional second login factor. Zero third-party deps: HMAC-SHA1 over a 30s
// time step, 6 digits, RFC 4648 base32 secret.

const (
	totpDigits = 6
	totpPeriod = 30 // seconds per time step
	totpIssuer = "AIOps Monitor"
)

// base32 without padding — the encoding authenticator apps expect for the secret.
var totpB32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// genTOTPSecret returns a fresh base32 secret: 20 random bytes (160 bits) is the
// standard TOTP secret length. Returns "" only if the system RNG fails.
func genTOTPSecret() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return totpB32.EncodeToString(b)
}

// totpAt computes the 6-digit code for the time step containing unix, per RFC
// 6238 / 4226 (dynamic truncation). Returns "" if the secret can't be decoded.
func totpAt(secret string, unix int64) string {
	key, err := totpB32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(key) == 0 {
		return ""
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(unix/totpPeriod))
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[off]&0x7f) << 24) | (uint32(sum[off+1]) << 16) |
		(uint32(sum[off+2]) << 8) | uint32(sum[off+3])
	return fmt.Sprintf("%0*d", totpDigits, bin%1_000_000)
}

// totpVerify checks code for the current 30s step and the two adjacent steps
// (±1) to tolerate clock skew between server and phone. Constant-time compare.
func totpVerify(secret, code string) bool {
	code = strings.TrimSpace(code)
	if secret == "" || len(code) != totpDigits {
		return false
	}
	now := time.Now().Unix()
	ok := false
	for _, d := range []int64{0, -totpPeriod, totpPeriod} {
		if subtle.ConstantTimeCompare([]byte(totpAt(secret, now+d)), []byte(code)) == 1 {
			ok = true // keep looping so timing doesn't reveal which step matched
		}
	}
	return ok
}

// totpMatchStep verifies code across the ±1-step skew window and returns the
// matched time-step index (unix/period) so callers can enforce single-use
// (record the consumed step per user). ok is false if no step matched.
func totpMatchStep(secret, code string) (step int64, ok bool) {
	code = strings.TrimSpace(code)
	if secret == "" || len(code) != totpDigits {
		return 0, false
	}
	now := time.Now().Unix()
	for _, d := range []int64{0, -totpPeriod, totpPeriod} {
		if subtle.ConstantTimeCompare([]byte(totpAt(secret, now+d)), []byte(code)) == 1 {
			step = (now + d) / totpPeriod
			ok = true // keep looping so timing doesn't reveal which step matched
		}
	}
	return step, ok
}

// otpauthURL builds the provisioning URI ("otpauth://totp/…") that the enrollment
// QR encodes and that authenticator apps import.
//
// The label is "issuer:account" with each part percent-encoded separately so the
// colon delimiter stays literal (some authenticator apps don't decode %3A).
// Spaces in query parameters are encoded as %20 (not "+") for maximum
// compatibility — older Google Authenticator builds don't decode "+" as space.
func otpauthURL(account, secret string) string {
	label := url.PathEscape(totpIssuer) + ":" + url.PathEscape(account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", totpIssuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", totpDigits))
	q.Set("period", fmt.Sprintf("%d", totpPeriod))
	return strings.ReplaceAll("otpauth://totp/"+label+"?"+q.Encode(), "+", "%20")
}
