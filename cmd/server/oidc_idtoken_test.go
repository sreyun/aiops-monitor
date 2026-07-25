package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateOIDCIDTokenRS256(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString([]byte{0x01, 0x00, 0x01})
	jwks := fmt.Sprintf(`{"keys":[{"kty":"RSA","kid":"k1","alg":"RS256","n":%q,"e":%q}]}`, n, e)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jwks))
	}))
	defer srv.Close()

	jwksCacheMu.Lock()
	delete(jwksCache, srv.URL)
	jwksCacheMu.Unlock()

	issuer := "https://idp.example.com/realms/ops"
	clientID := "aiops"
	nonce := "nonce-abc"
	now := time.Now().Unix()
	payload, _ := json.Marshal(map[string]any{
		"iss": issuer, "sub": "user-1", "aud": clientID,
		"exp": now + 300, "iat": now, "nonce": nonce,
		"email": "a@example.com", "name": "Alice",
	})
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"k1","typ":"JWT"}`))
	body := base64.RawURLEncoding.EncodeToString(payload)
	sum := sha256.Sum256([]byte(hdr + "." + body))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	raw := hdr + "." + body + "." + base64.RawURLEncoding.EncodeToString(sig)

	claims, err := validateOIDCIDToken(raw, issuer, clientID, nonce, srv.URL)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.Sub != "user-1" || claims.Email != "a@example.com" {
		t.Fatalf("claims=%+v", claims)
	}

	if _, err := validateOIDCIDToken(raw, issuer, "other-client", nonce, srv.URL); err == nil {
		t.Fatal("expected aud mismatch")
	}
	if _, err := validateOIDCIDToken(raw, issuer, clientID, "wrong-nonce", srv.URL); err == nil {
		t.Fatal("expected nonce mismatch")
	}
}

func TestOIDCAudContains(t *testing.T) {
	if !oidcAudContains(json.RawMessage(`"cli"`), "cli") {
		t.Fatal("string aud")
	}
	if !oidcAudContains(json.RawMessage(`["a","cli"]`), "cli") {
		t.Fatal("array aud")
	}
	if oidcAudContains(json.RawMessage(`["a"]`), "cli") {
		t.Fatal("should miss")
	}
}
