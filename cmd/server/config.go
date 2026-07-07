package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

// WebhookConfig holds one bot channel (Feishu or DingTalk).
type WebhookConfig struct {
	Enabled bool   `json:"enabled"`
	Webhook string `json:"webhook"`
	Secret  string `json:"secret,omitempty"` // DingTalk optional HMAC-SHA256 sign secret
}

// SMTPConfig holds the email (SMTP) notification channel. The password is
// stored in plaintext on disk (like the DingTalk secret) but masked when
// echoed to the browser via handleGetConfig.
type SMTPConfig struct {
	Enabled  bool   `json:"smtp_enabled"`
	Host     string `json:"smtp_host"`     // e.g. smtp.gmail.com
	Port     int    `json:"smtp_port"`     // 465 (implicit TLS) or 587 (STARTTLS)
	Username string `json:"smtp_username"` // sender email account
	Password string `json:"smtp_password,omitempty"`
	FromName string `json:"smtp_from_name"` // display name, default "AIOps Monitor"
	UseTLS   bool   `json:"smtp_use_tls"`  // true = implicit TLS (465), false = STARTTLS/plain
}

// ThresholdConfig is the JSON-friendly, operator-editable alert threshold set.
type ThresholdConfig struct {
	CPUWarn         float64 `json:"cpu_warn"`
	CPUCrit         float64 `json:"cpu_crit"`
	MemWarn         float64 `json:"mem_warn"`
	MemCrit         float64 `json:"mem_crit"`
	DiskWarn        float64 `json:"disk_warn"`
	DiskCrit        float64 `json:"disk_crit"`
	OfflineAfterSec int     `json:"offline_after_sec"`
}

func defaultThresholdConfig() ThresholdConfig {
	return ThresholdConfig{
		CPUWarn: 80, CPUCrit: 90,
		MemWarn: 80, MemCrit: 90,
		DiskWarn: 85, DiskCrit: 95,
		OfflineAfterSec: 30,
	}
}

func (t ThresholdConfig) toThresholds() Thresholds {
	return Thresholds{
		CPUWarn: t.CPUWarn, CPUCrit: t.CPUCrit,
		MemWarn: t.MemWarn, MemCrit: t.MemCrit,
		DiskWarn: t.DiskWarn, DiskCrit: t.DiskCrit,
		OfflineAfter: time.Duration(t.OfflineAfterSec) * time.Second,
	}
}

// AccountConfig is the dashboard login account + profile. The password is
// stored salted+hashed (never plaintext).
type AccountConfig struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Salt        string `json:"salt"`
	Hash        string `json:"hash"`
	// Optional TOTP (Google Authenticator) second factor. MFASecret is the base32
	// shared secret; it is never returned to the browser once enrollment completes.
	MFAEnabled bool   `json:"mfa_enabled"`
	MFASecret  string `json:"mfa_secret,omitempty"`
}

func defaultAccount() AccountConfig {
	salt := genToken()[:16]
	return AccountConfig{
		Username:    "admin",
		DisplayName: "管理员",
		Salt:        salt,
		Hash:        hashPassword("admin", salt),
	}
}

// CustomCheck is an operator-defined synthetic monitor run by the server:
// an HTTP(S) URL probe, a TCP host:port probe, or a process-existence check.
// A failing check raises an alert and pushes a notification.
type CustomCheck struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`   // http | tcp | process
	Target      string `json:"target"` // URL for http, host:port for tcp, hostID/procName for process
	IntervalSec int    `json:"interval_sec"`
	Level       string `json:"level"` // warning | critical
	Enabled     bool   `json:"enabled"`
}

// ServerConfig is the operator-editable server configuration persisted to disk.
// Categories holds manual per-host category overrides (host id -> category).
type ServerConfig struct {
	AlertsEnabled bool              `json:"alerts_enabled"`
	Feishu        WebhookConfig     `json:"feishu"`
	Dingtalk      WebhookConfig     `json:"dingtalk"`
	SMTP          SMTPConfig        `json:"smtp"`
	Thresholds    ThresholdConfig   `json:"thresholds"`
	Categories    map[string]string `json:"categories"`
	InstallToken  string            `json:"install_token"`
	RequireToken  bool              `json:"require_token"`
	Account       AccountConfig     `json:"account"`
	Checks        []CustomCheck     `json:"checks"`
	Playbooks     []Playbook        `json:"playbooks,omitempty"`
	// TerminalDisabled is an inverted flag so remote terminal defaults ON for
	// existing configs (zero value = enabled); set true to globally disable it.
	TerminalDisabled bool `json:"terminal_disabled"`
	// AllowAnonymousAgents is an inverted flag: by default (zero value = false)
	// every agent MUST present a valid install token to register/report. Set true
	// only to permit token-less agents (not recommended).
	AllowAnonymousAgents bool `json:"allow_anonymous_agents"`
	// TrustProxy tells the server it sits behind a trusted reverse proxy, so it
	// may believe the X-Real-IP / X-Forwarded-For headers for the real client
	// address (used by login rate-limiting and audit logs). Default false: when
	// the server is directly exposed these headers are attacker-forgeable, so
	// they are ignored and the raw connection address is used instead.
	TrustProxy bool `json:"trust_proxy"`
}

func defaultServerConfig() ServerConfig {
	return ServerConfig{
		AlertsEnabled: true,
		Thresholds:    defaultThresholdConfig(),
		Categories:    map[string]string{},
		SMTP: SMTPConfig{
			FromName: "AIOps Monitor",
		},
	}
}

// ConfigStore wraps ServerConfig with disk persistence and thread safety.
type ConfigStore struct {
	mu   sync.RWMutex
	path string
	cfg  ServerConfig
}

func NewConfigStore(path string) *ConfigStore {
	cs := &ConfigStore{path: path, cfg: defaultServerConfig()}
	if b, err := os.ReadFile(path); err == nil {
		var c ServerConfig
		if json.Unmarshal(b, &c) == nil {
			if c.Categories == nil {
				c.Categories = map[string]string{}
			}
			cs.cfg = c
		}
	}
	dirty := false
	if cs.cfg.InstallToken == "" {
		cs.cfg.InstallToken = genToken()
		dirty = true
	}
	if cs.cfg.Account.Username == "" {
		cs.cfg.Account = defaultAccount()
		dirty = true
	}
	if dirty {
		_ = cs.save()
	}
	return cs
}

func genToken() string {
	b := make([]byte, 16) // 32 hex characters
	if _, err := rand.Read(b); err != nil {
		return "aiops-token-fallback-0000000000000"
	}
	return hex.EncodeToString(b)
}

func (cs *ConfigStore) Get() ServerConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg
}

func (cs *ConfigStore) Thresholds() Thresholds {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.Thresholds.toThresholds()
}

func (cs *ConfigStore) InstallToken() string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.InstallToken
}

func (cs *ConfigStore) RequireToken() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.RequireToken
}

// AgentTokenRequired reports whether agents must present a valid install token.
// Enforced by default; only the explicit allow_anonymous_agents escape hatch
// disables it.
func (cs *ConfigStore) AgentTokenRequired() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return !cs.cfg.AllowAnonymousAgents
}

// TerminalEnabled reports whether the remote terminal feature is available
// (default true; disabled only when terminal_disabled is set in config).
func (cs *ConfigStore) TerminalEnabled() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return !cs.cfg.TerminalDisabled
}

// TrustProxy reports whether to honor reverse-proxy client-IP headers
// (X-Real-IP / X-Forwarded-For). Off by default so a directly-exposed server
// can't be fooled by forged headers into miscounting login attempts.
func (cs *ConfigStore) TrustProxy() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.TrustProxy
}

// ResetToken regenerates the install token and returns the new value.
func (cs *ConfigStore) ResetToken() string {
	cs.mu.Lock()
	cs.cfg.InstallToken = genToken()
	tok := cs.cfg.InstallToken
	cs.mu.Unlock()
	_ = cs.save()
	return tok
}

// ---- account ----

func (cs *ConfigStore) Account() AccountConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.Account
}

func (cs *ConfigStore) SetProfile(display, email string) error {
	cs.mu.Lock()
	cs.cfg.Account.DisplayName = display
	cs.cfg.Account.Email = email
	cs.mu.Unlock()
	return cs.save()
}

// SetUsername changes the login username (the account identifier).
func (cs *ConfigStore) SetUsername(name string) error {
	cs.mu.Lock()
	cs.cfg.Account.Username = name
	cs.mu.Unlock()
	return cs.save()
}

// SetMFA enables or disables the TOTP second factor. Disabling clears the secret
// so a stale secret can never linger in the config.
func (cs *ConfigStore) SetMFA(enabled bool, secret string) error {
	cs.mu.Lock()
	cs.cfg.Account.MFAEnabled = enabled
	if enabled {
		cs.cfg.Account.MFASecret = secret
	} else {
		cs.cfg.Account.MFASecret = ""
	}
	cs.mu.Unlock()
	return cs.save()
}

func (cs *ConfigStore) SetPassword(newPass string) error {
	cs.mu.Lock()
	salt := genToken()[:16]
	cs.cfg.Account.Salt = salt
	cs.cfg.Account.Hash = hashPassword(newPass, salt)
	cs.mu.Unlock()
	return cs.save()
}

// ---- custom checks ----

func (cs *ConfigStore) Checks() []CustomCheck {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make([]CustomCheck, len(cs.cfg.Checks))
	copy(out, cs.cfg.Checks)
	return out
}

// UpsertCheck adds a new check (assigning an id) or replaces one by id.
func (cs *ConfigStore) UpsertCheck(c CustomCheck) (CustomCheck, error) {
	cs.mu.Lock()
	if c.ID == "" {
		c.ID = genToken()[:8]
		cs.cfg.Checks = append(cs.cfg.Checks, c)
	} else {
		found := false
		for i := range cs.cfg.Checks {
			if cs.cfg.Checks[i].ID == c.ID {
				cs.cfg.Checks[i] = c
				found = true
				break
			}
		}
		if !found {
			cs.cfg.Checks = append(cs.cfg.Checks, c)
		}
	}
	cs.mu.Unlock()
	return c, cs.save()
}

func (cs *ConfigStore) DeleteCheck(id string) error {
	cs.mu.Lock()
	kept := cs.cfg.Checks[:0]
	for _, c := range cs.cfg.Checks {
		if c.ID != id {
			kept = append(kept, c)
		}
	}
	cs.cfg.Checks = kept
	cs.mu.Unlock()
	return cs.save()
}

// Set replaces the alert/threshold config, preserving category overrides, and
// persists to disk.
func (cs *ConfigStore) Set(c ServerConfig) error {
	cs.mu.Lock()
	c.Categories = cs.cfg.Categories     // categories managed via SetCategory
	c.InstallToken = cs.cfg.InstallToken // token managed via install endpoints
	c.Account = cs.cfg.Account           // account managed via auth endpoints
	c.Checks = cs.cfg.Checks             // checks managed via check endpoints
	c.Playbooks = cs.cfg.Playbooks       // playbooks managed via playbook endpoints
	// Preserve SMTP password when the incoming value is blank or masked (same
	// strategy as webhook secrets — the browser may submit without re-typing it).
	if c.SMTP.Password == "" || strings.Contains(c.SMTP.Password, "****") {
		c.SMTP.Password = cs.cfg.SMTP.Password
	}
	if c.SMTP.FromName == "" {
		c.SMTP.FromName = cs.cfg.SMTP.FromName
	}
	// Operational security flags are managed via the config file, not the alert
	// settings form — preserve them so a settings save can't silently flip them.
	c.RequireToken = cs.cfg.RequireToken
	c.TerminalDisabled = cs.cfg.TerminalDisabled
	c.AllowAnonymousAgents = cs.cfg.AllowAnonymousAgents
	c.TrustProxy = cs.cfg.TrustProxy
	cs.cfg = c
	cs.mu.Unlock()
	return cs.save()
}

// SetCategory records (or clears, when cat is empty) a manual category override.
func (cs *ConfigStore) SetCategory(hostID, cat string) error {
	cs.mu.Lock()
	if cs.cfg.Categories == nil {
		cs.cfg.Categories = map[string]string{}
	}
	if cat == "" {
		delete(cs.cfg.Categories, hostID)
	} else {
		cs.cfg.Categories[hostID] = cat
	}
	cs.mu.Unlock()
	return cs.save()
}

func (cs *ConfigStore) CategoryOverride(hostID string) (string, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	c, ok := cs.cfg.Categories[hostID]
	return c, ok
}

func (cs *ConfigStore) save() error {
	cs.mu.RLock()
	b, err := json.MarshalIndent(cs.cfg, "", "  ")
	cs.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(cs.path, b, 0o644)
}

// ---- playbooks ----

func (cs *ConfigStore) Playbooks() []Playbook {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make([]Playbook, len(cs.cfg.Playbooks))
	copy(out, cs.cfg.Playbooks)
	return out
}

func (cs *ConfigStore) UpsertPlaybook(p Playbook) (Playbook, error) {
	cs.mu.Lock()
	if p.ID == "" {
		cs.cfg.Playbooks = append(cs.cfg.Playbooks, p)
	} else {
		found := false
		for i := range cs.cfg.Playbooks {
			if cs.cfg.Playbooks[i].ID == p.ID {
				cs.cfg.Playbooks[i] = p
				found = true
				break
			}
		}
		if !found {
			cs.cfg.Playbooks = append(cs.cfg.Playbooks, p)
		}
	}
	cs.mu.Unlock()
	return p, cs.save()
}

func (cs *ConfigStore) DeletePlaybook(id string) error {
	cs.mu.Lock()
	kept := cs.cfg.Playbooks[:0]
	for _, p := range cs.cfg.Playbooks {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	cs.cfg.Playbooks = kept
	cs.mu.Unlock()
	return cs.save()
}
