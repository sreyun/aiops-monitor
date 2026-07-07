package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// WebhookConfig holds one bot channel (Feishu or DingTalk).
type WebhookConfig struct {
	Enabled bool   `json:"enabled"`
	Webhook string `json:"webhook"`
	Secret  string `json:"secret,omitempty"` // DingTalk optional HMAC-SHA256 sign secret
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
	Thresholds    ThresholdConfig   `json:"thresholds"`
	Categories    map[string]string `json:"categories"`
	InstallToken  string            `json:"install_token"`
	RequireToken  bool              `json:"require_token"`
	Account       AccountConfig     `json:"account"`
	Checks        []CustomCheck     `json:"checks"`
	// TerminalDisabled is an inverted flag so remote terminal defaults ON for
	// existing configs (zero value = enabled); set true to globally disable it.
	TerminalDisabled bool `json:"terminal_disabled"`
}

func defaultServerConfig() ServerConfig {
	return ServerConfig{
		AlertsEnabled: true,
		Thresholds:    defaultThresholdConfig(),
		Categories:    map[string]string{},
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

// TerminalEnabled reports whether the remote terminal feature is available
// (default true; disabled only when terminal_disabled is set in config).
func (cs *ConfigStore) TerminalEnabled() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return !cs.cfg.TerminalDisabled
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
