package main

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

// Email system: SMTP sender + one-time verification codes / password-reset tokens.
// Zero third-party deps — uses net/smtp + crypto/tls from the standard library.

// -----------------------------------------------------------------------
// email sender
// -----------------------------------------------------------------------

// sendEmail delivers an HTML message via the configured SMTP server. It supports
// implicit TLS (port 465) and STARTTLS (port 587). Returns an error string that
// is safe to surface to the operator.
func sendEmail(cfg SMTPConfig, to, subject, htmlBody string) error {
	if !cfg.Enabled || cfg.Host == "" || cfg.Username == "" {
		return fmt.Errorf("邮件服务未配置或未启用")
	}
	if cfg.Port == 0 {
		cfg.Port = 465
	}
	fromName := cfg.FromName
	if fromName == "" {
		fromName = "AIOps Monitor"
	}
	from := cfg.Username
	headers := fmt.Sprintf("From: %s <%s>\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n",
		mimeEncode(fromName), from, to, mimeEncode(subject))
	msg := []byte(headers + htmlBody)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)

	if cfg.UseTLS {
		// Implicit TLS — dial a TLS connection then run SMTP over it (port 465).
		tlsCfg := &tls.Config{ServerName: cfg.Host}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("TLS 连接失败: %v", err)
		}
		defer conn.Close()
		c, err := smtp.NewClient(conn, cfg.Host)
		if err != nil {
			return fmt.Errorf("SMTP 握手失败: %v", err)
		}
		defer c.Close()
		if err = c.Auth(auth); err != nil {
			return fmt.Errorf("SMTP 认证失败: %v", err)
		}
		if err = c.Mail(from); err != nil {
			return fmt.Errorf("发件人错误: %v", err)
		}
		if err = c.Rcpt(to); err != nil {
			return fmt.Errorf("收件人错误: %v", err)
		}
		w, err := c.Data()
		if err != nil {
			return fmt.Errorf("数据写入失败: %v", err)
		}
		if _, err = w.Write(msg); err != nil {
			return fmt.Errorf("邮件写入失败: %v", err)
		}
		if err = w.Close(); err != nil {
			return fmt.Errorf("邮件发送失败: %v", err)
		}
		return nil
	}
	// STARTTLS or plain — use smtp.SendMail which handles STARTTLS upgrade.
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}

// mimeEncode encodes a header value as an RFC 2047 UTF-8 encoded-word when it
// contains non-ASCII characters. Keeps the header valid for Chinese text.
func mimeEncode(s string) string {
	for _, r := range s {
		if r > 127 {
			return fmt.Sprintf("=?UTF-8?B?%s?=", base64Str(s))
		}
	}
	return s
}

func base64Str(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// -----------------------------------------------------------------------
// verification code & reset-token manager (in-memory, with expiry)
// -----------------------------------------------------------------------

type emailCode struct {
	code      string
	expires   time.Time
	consumed  bool
	purpose   string // "mfa_unbind" | "reset_password" | "recover_username"
	email     string
}

type passwordResetToken struct {
	token    string
	username string
	email    string
	expires  time.Time
	used     bool
}

// emailManager tracks one-time verification codes and password-reset tokens.
// All entries are in-memory with TTLs; codes are single-use and consumed on
// first successful verification.
type emailManager struct {
	mu         sync.Mutex
	codes      map[string]emailCode       // key: lowercase email
	resetToken map[string]passwordResetToken // key: token
	lastSent   map[string]time.Time        // key: lowercase email — rate limit
}

func newEmailManager() *emailManager {
	return &emailManager{
		codes:      map[string]emailCode{},
		resetToken: map[string]passwordResetToken{},
		lastSent:   map[string]time.Time{},
	}
}

// genCode generates a 6-digit numeric verification code.
func genCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1_000_000)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// genResetToken generates a 32-char hex reset token.
func genResetToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}

// canSend checks the 60s rate limit for an email address.
func (em *emailManager) canSend(email string) bool {
	em.mu.Lock()
	defer em.mu.Unlock()
	key := strings.ToLower(email)
	if t, ok := em.lastSent[key]; ok {
		if time.Since(t) < 60*time.Second {
			return false
		}
	}
	return true
}

// markSent records the send time for rate limiting.
func (em *emailManager) markSent(email string) {
	em.mu.Lock()
	em.lastSent[strings.ToLower(email)] = time.Now()
	em.mu.Unlock()
}

// issueCode generates and stores a 6-digit code for the given email + purpose.
// TTL is 10 minutes. Returns the code and an error (if rate-limited).
func (em *emailManager) issueCode(email, purpose string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !em.canSend(email) {
		return "", fmt.Errorf("发送过于频繁，请 60 秒后再试")
	}
	code := genCode()
	em.mu.Lock()
	em.codes[email] = emailCode{
		code:    code,
		expires: time.Now().Add(10 * time.Minute),
		purpose: purpose,
		email:   email,
	}
	em.lastSent[email] = time.Now()
	em.mu.Unlock()
	return code, nil
}

// verifyCode checks and consumes a one-time code. Returns false if invalid,
// expired, wrong purpose, or already consumed.
func (em *emailManager) verifyCode(email, purpose, code string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	em.mu.Lock()
	defer em.mu.Unlock()
	entry, ok := em.codes[email]
	if !ok || entry.consumed || entry.purpose != purpose {
		return false
	}
	if time.Now().After(entry.expires) {
		delete(em.codes, email)
		return false
	}
	if !constantTimeEq(entry.code, code) {
		return false
	}
	entry.consumed = true
	em.codes[email] = entry
	delete(em.codes, email) // single-use: remove immediately
	return true
}

// issueResetToken creates a one-time password-reset token for username+email.
// TTL is 15 minutes.
func (em *emailManager) issueResetToken(username, email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	tok := genResetToken()
	em.mu.Lock()
	em.resetToken[tok] = passwordResetToken{
		token:    tok,
		username: username,
		email:    email,
		expires:  time.Now().Add(15 * time.Minute),
	}
	em.mu.Unlock()
	return tok
}

// consumeResetToken validates and consumes a reset token. Returns the username
// if valid, "" otherwise.
func (em *emailManager) consumeResetToken(tok string) (username, email string, ok bool) {
	em.mu.Lock()
	defer em.mu.Unlock()
	entry, exists := em.resetToken[tok]
	if !exists || entry.used || time.Now().After(entry.expires) {
		if exists {
			delete(em.resetToken, tok)
		}
		return "", "", false
	}
	entry.used = true
	delete(em.resetToken, tok) // single-use
	return entry.username, entry.email, true
}

// constantTimeEq compares two strings in constant time.
func constantTimeEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var r byte
	for i := 0; i < len(a); i++ {
		r |= a[i] ^ b[i]
	}
	return r == 0
}

// -----------------------------------------------------------------------
// email validation
// -----------------------------------------------------------------------

// validEmail performs a basic RFC-5321-ish format check: contains @, a local
// part, a domain with at least one dot. Good enough to reject garbage without
// pulling in a regex dependency.
func validEmail(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 5 || len(s) > 254 {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at < 1 || at == len(s)-1 {
		return false
	}
	domain := s[at+1:]
	dot := strings.IndexByte(domain, '.')
	return dot > 0 && dot < len(domain)-1
}

// lookupMX is an optional best-effort MX record check — used to give a friendlier
// error when the domain has no mail server. Returns nil if the check can't be done.
func lookupMX(domain string) bool {
	_, err := net.LookupMX(domain)
	return err == nil
}
