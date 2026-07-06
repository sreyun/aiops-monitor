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
	if cs.cfg.InstallToken == "" {
		cs.cfg.InstallToken = genToken()
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

// ResetToken regenerates the install token and returns the new value.
func (cs *ConfigStore) ResetToken() string {
	cs.mu.Lock()
	cs.cfg.InstallToken = genToken()
	tok := cs.cfg.InstallToken
	cs.mu.Unlock()
	_ = cs.save()
	return tok
}

// Set replaces the alert/threshold config, preserving category overrides, and
// persists to disk.
func (cs *ConfigStore) Set(c ServerConfig) error {
	cs.mu.Lock()
	c.Categories = cs.cfg.Categories     // categories managed via SetCategory
	c.InstallToken = cs.cfg.InstallToken // token managed via install endpoints
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
