package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestLoginAccountLockout verifies the per-account throttle locks an account after
// loginAccountMaxFail failures (independent of source IP) and clears on reset.
func TestLoginAccountLockout(t *testing.T) {
	a := NewAuth(newTestConfigStore(t))
	for i := 0; i < loginAccountMaxFail; i++ {
		if !a.loginAccountAllowed("bob") {
			t.Fatalf("account should be allowed before threshold (i=%d)", i)
		}
		a.loginAccountFailed("bob")
	}
	if a.loginAccountAllowed("bob") {
		t.Error("account should be throttled after reaching the failure threshold")
	}
	a.loginAccountReset("bob")
	if !a.loginAccountAllowed("bob") {
		t.Error("reset should clear the throttle")
	}
}

// TestTOTPSingleUse verifies a valid TOTP code is accepted once and its replay
// (same code/time-step) is rejected as totpReplay (not totpInvalid) within the skew window.
func TestTOTPSingleUse(t *testing.T) {
	a := NewAuth(newTestConfigStore(t))
	secret := genTOTPSecret()
	if secret == "" {
		t.Fatal("genTOTPSecret failed")
	}
	code := totpAt(secret, time.Now().Unix())
	if got := a.verifyAndConsumeTOTP("alice", secret, code); got != totpOK {
		t.Fatalf("first use of a valid TOTP code should succeed, got %v", got)
	}
	if got := a.verifyAndConsumeTOTP("alice", secret, code); got != totpReplay {
		t.Errorf("replay of the same TOTP code must be totpReplay, got %v", got)
	}
	if got := a.verifyAndConsumeTOTP("alice", secret, "000000"); got != totpInvalid {
		t.Errorf("wrong code must be totpInvalid, got %v", got)
	}
}

func TestNormalizeTOTPCode(t *testing.T) {
	if got := normalizeTOTPCode(" 123 456 "); got != "123456" {
		t.Errorf("normalize spaced code: got %q", got)
	}
	if got := normalizeTOTPCode("12-34-56"); got != "123456" {
		t.Errorf("normalize dashed code: got %q", got)
	}
	a := NewAuth(newTestConfigStore(t))
	secret := genTOTPSecret()
	code := totpAt(secret, time.Now().Unix())
	spaced := code[:3] + " " + code[3:]
	if got := a.verifyAndConsumeTOTP("bob", secret, spaced); got != totpOK {
		t.Fatalf("spaced autofill code should verify, got %v", got)
	}
}

// TestHandleHostsStripsFingerprint verifies the agent fingerprint — the sole
// credential authenticating the agent reverse channels (terminal rx/tx, report,
// logs, forward) — is never exposed to the browser via GET /api/v1/hosts. A leak
// would let any viewer hijack terminals or spoof host telemetry.
func TestHandleHostsStripsFingerprint(t *testing.T) {
	store := NewStore()
	store.RegisterHost("h1", "node-1", "fp-super-secret-123")
	cfg := newTestConfigStore(t)
	srv := &Server{store: store, cfg: cfg}

	req := httptest.NewRequest("GET", "/api/v1/hosts", nil)
	w := httptest.NewRecorder()
	srv.handleHosts(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleHosts status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "fp-super-secret-123") {
		t.Fatalf("fingerprint leaked in /api/v1/hosts response: %s", body)
	}
	var views []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &views); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("expected 1 host, got %d", len(views))
	}
	if v, ok := views[0]["fingerprint"]; ok && v != "" {
		t.Errorf("fingerprint field should be stripped, got %q", v)
	}
}

// TestHandleGetConfigMasksSecrets verifies GET /api/v1/config (readable by any
// viewer) masks the agent enrollment token, the relay shared secret, and custom
// webhook headers — none of which a low-privilege user should be able to read.
func TestHandleGetConfigMasksSecrets(t *testing.T) {
	cfg := newTestConfigStore(t)
	// Inject known secret values directly (Set() intentionally preserves these
	// fields from stored config, so they can't be set through the form).
	cfg.mu.Lock()
	cfg.cfg.InstallToken = "PLAINTOKEN0123456789"
	cfg.cfg.RelaySecret = "PLAINRELAYSECRET0123"
	cfg.cfg.CustomWebhook.Headers = "X-Token: plaintextsecret"
	cfg.mu.Unlock()

	srv := &Server{cfg: cfg}
	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	w := httptest.NewRecorder()
	srv.handleGetConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleGetConfig status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, secret := range []string{"PLAINTOKEN0123456789", "PLAINRELAYSECRET0123", "plaintextsecret"} {
		if strings.Contains(body, secret) {
			t.Errorf("secret %q leaked unmasked in /api/v1/config response", secret)
		}
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if tok, _ := got["install_token"].(string); !strings.Contains(tok, "****") {
		t.Errorf("install_token not masked: %q", tok)
	}
	if rs, _ := got["relay_secret"].(string); !strings.Contains(rs, "****") {
		t.Errorf("relay_secret not masked: %q", rs)
	}
}

// TestSecretEncryptionRoundTrip verifies AES-GCM at-rest encryption round-trips,
// passes plaintext through (migration), and is a no-op without a master key.
func TestSecretEncryptionRoundTrip(t *testing.T) {
	t.Setenv("AIOPS_SECRET_KEY", "test-master-key-123")
	plain := "s3cr3t-value!"
	enc := encryptSecret(plain)
	if enc == plain || !strings.HasPrefix(enc, secretEncPrefix) {
		t.Fatalf("expected encrypted output with prefix, got %q", enc)
	}
	if got := decryptSecret(enc); got != plain {
		t.Errorf("round-trip mismatch: got %q want %q", got, plain)
	}
	// A fresh encryption uses a random nonce, so ciphertext differs each time.
	if encryptSecret(plain) == enc {
		t.Error("expected a fresh nonce to produce different ciphertext")
	}
	// Legacy plaintext (no prefix) passes through decrypt unchanged.
	if got := decryptSecret("plainlegacy"); got != "plainlegacy" {
		t.Errorf("plaintext passthrough failed: %q", got)
	}
	if encryptSecret("") != "" {
		t.Error("empty value should stay empty")
	}
}

// TestSecretEncryptionDisabled verifies encryption is opt-in: with no master key,
// values are stored as-is (backward compatible).
func TestSecretEncryptionDisabled(t *testing.T) {
	t.Setenv("AIOPS_SECRET_KEY", "")
	if got := encryptSecret("abc"); got != "abc" {
		t.Errorf("without master key encryptSecret should passthrough, got %q", got)
	}
}

// TestConfigSecretsEncryptRoundTrip exercises the whole-config secret encryption
// path (top-level fields + per-user MFA seeds).
func TestConfigSecretsEncryptRoundTrip(t *testing.T) {
	t.Setenv("AIOPS_SECRET_KEY", "master-key-xyz")
	var c ServerConfig
	c.SMTP.Password = "smtp-pw"
	c.AI.APIKey = "sk-aikey"
	c.RelaySecret = "relay-abc"
	c.Account.MFASecret = "MFASEEDBASE32"
	c.Users = []AccountConfig{{Username: "u1", MFASecret: "USER1SEED"}}

	encryptConfigSecrets(&c)
	if !strings.HasPrefix(c.SMTP.Password, secretEncPrefix) || !strings.HasPrefix(c.Users[0].MFASecret, secretEncPrefix) {
		t.Fatalf("secrets not encrypted: %q / %q", c.SMTP.Password, c.Users[0].MFASecret)
	}
	decryptConfigSecrets(&c)
	if c.SMTP.Password != "smtp-pw" || c.AI.APIKey != "sk-aikey" || c.RelaySecret != "relay-abc" ||
		c.Account.MFASecret != "MFASEEDBASE32" || c.Users[0].MFASecret != "USER1SEED" {
		t.Errorf("config secret round-trip mismatch: %+v", c)
	}
}
