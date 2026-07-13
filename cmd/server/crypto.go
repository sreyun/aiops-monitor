package main

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// v5.5.0: at-rest encryption for the reversible secrets in ServerConfig (MFA/TOTP
// seeds, SMTP/webhook/AI credentials, relay secret). Password *hashes* are already
// one-way (PBKDF2) and are never encrypted. Encryption is opt-in: set a master key
// in AIOPS_SECRET_KEY and every secret is AES-256-GCM sealed before it is written
// to PostgreSQL (or the bootstrap config file). Without the key the values are
// stored as before, so enabling/disabling is a transparent, backward-compatible
// migration — plaintext loads fine and gets encrypted on the next save.

const secretEncPrefix = "enc:v1:"

// loadSecretKey derives a 32-byte AES key from AIOPS_SECRET_KEY (SHA-256 of the
// operator-provided passphrase). Returns nil when the env var is unset. Derived
// per call (SHA-256 is cheap) so the key can change without a restart and stays
// test-friendly.
func loadSecretKey() []byte {
	raw := strings.TrimSpace(os.Getenv("AIOPS_SECRET_KEY"))
	if raw == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// secretEncryptionEnabled reports whether a master key is configured.
func secretEncryptionEnabled() bool { return loadSecretKey() != nil }

// encryptSecret seals a plaintext secret as "enc:v1:<base64(nonce|ciphertext)>"
// when a master key is set. Empty or already-encrypted input passes through; with
// no key it returns the plaintext (encryption disabled).
func encryptSecret(plain string) string {
	if plain == "" || strings.HasPrefix(plain, secretEncPrefix) {
		return plain
	}
	key := loadSecretKey()
	if key == nil {
		return plain
	}
	gcm, err := newGCM(key)
	if err != nil {
		slog.Error("配置密钥加密初始化失败", "err", err)
		return plain
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		slog.Error("配置密钥加密随机数失败", "err", err)
		return plain
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return secretEncPrefix + base64.StdEncoding.EncodeToString(sealed)
}

// decryptSecret reverses encryptSecret. Plaintext (no prefix) passes through for
// backward-compat/migration. An encrypted value that can't be opened (missing or
// wrong key, corrupt data) returns "" and logs — fail-safe, so ciphertext is never
// mistaken for a usable secret.
func decryptSecret(v string) string {
	if !strings.HasPrefix(v, secretEncPrefix) {
		return v
	}
	key := loadSecretKey()
	if key == nil {
		slog.Error("配置中存在加密字段，但未设置 AIOPS_SECRET_KEY，无法解密（相关凭据将不可用）")
		return ""
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(v, secretEncPrefix))
	if err != nil {
		slog.Error("配置密钥解密失败：base64 解码", "err", err)
		return ""
	}
	gcm, err := newGCM(key)
	if err != nil {
		slog.Error("配置密钥解密初始化失败", "err", err)
		return ""
	}
	if len(data) < gcm.NonceSize() {
		slog.Error("配置密钥解密失败：密文过短")
		return ""
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		slog.Error("配置密钥解密失败：密钥不匹配或数据损坏", "err", err)
		return ""
	}
	return string(pt)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// ---- 日志传输加密（gzip + AES-256-GCM）----
//
// 每个 agent 的日志密钥 = 派生自服务端主密钥 + agent 指纹：注册时服务端计算并把 key 一次性
// 下发给 agent；之后每批日志 agent 用它 gzip+AES-GCM 加密上报，服务端按上报头里的指纹重新
// 派生同一 key 解密——服务端无需存储 per-agent 密钥。未设置 AIOPS_SECRET_KEY 时返回 nil，
// 日志走明文（向后兼容 / 调试）。

func deriveLogKey(fingerprint string) []byte {
	master := loadSecretKey()
	if master == nil || strings.TrimSpace(fingerprint) == "" {
		return nil
	}
	buf := make([]byte, 0, len(master)+64)
	buf = append(buf, master...)
	buf = append(buf, []byte(":logenc:v1:"+fingerprint)...)
	sum := sha256.Sum256(buf)
	return sum[:]
}

// sealLog: gzip 压缩明文后 AES-256-GCM 加密，返回 nonce||ciphertext。
func sealLog(key, plaintext []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(plaintext); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, buf.Bytes(), nil), nil
}

// openLog: sealLog 的逆操作——AES-256-GCM 解密 + gzip 解压。
func openLog(key, data []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, fmt.Errorf("密文过短")
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	comp, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(comp))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(io.LimitReader(zr, 16<<20))
}

// encryptConfigSecrets seals every reversible secret in c in place. Operates on a
// COPY of the live config (see ConfigStore.save) — never on the in-memory config,
// which must keep plaintext for use.
func encryptConfigSecrets(c *ServerConfig) {
	c.SMTP.Password = encryptSecret(c.SMTP.Password)
	c.AI.APIKey = encryptSecret(c.AI.APIKey)
	c.AI.EmbedAPIKey = encryptSecret(c.AI.EmbedAPIKey)
	c.RelaySecret = encryptSecret(c.RelaySecret)
	c.Dingtalk.Secret = encryptSecret(c.Dingtalk.Secret)
	c.CustomWebhook.Headers = encryptSecret(c.CustomWebhook.Headers)
	c.Account.MFASecret = encryptSecret(c.Account.MFASecret)
	for i := range c.Users {
		c.Users[i].MFASecret = encryptSecret(c.Users[i].MFASecret)
	}
}

// decryptConfigSecrets reverses encryptConfigSecrets, restoring plaintext in the
// in-memory config after load.
func decryptConfigSecrets(c *ServerConfig) {
	c.SMTP.Password = decryptSecret(c.SMTP.Password)
	c.AI.APIKey = decryptSecret(c.AI.APIKey)
	c.AI.EmbedAPIKey = decryptSecret(c.AI.EmbedAPIKey)
	c.RelaySecret = decryptSecret(c.RelaySecret)
	c.Dingtalk.Secret = decryptSecret(c.Dingtalk.Secret)
	c.CustomWebhook.Headers = decryptSecret(c.CustomWebhook.Headers)
	c.Account.MFASecret = decryptSecret(c.Account.MFASecret)
	for i := range c.Users {
		c.Users[i].MFASecret = decryptSecret(c.Users[i].MFASecret)
	}
}
