package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	DiskIOWarn      float64 `json:"diskio_warn"`
	DiskIOCrit      float64 `json:"diskio_crit"`
	OfflineAfterSec int     `json:"offline_after_sec"`
}

func defaultThresholdConfig() ThresholdConfig {
	return ThresholdConfig{
		CPUWarn: 80, CPUCrit: 90,
		MemWarn: 80, MemCrit: 90,
		DiskWarn: 85, DiskCrit: 95,
		DiskIOWarn: 80, DiskIOCrit: 90,
		OfflineAfterSec: 30,
	}
}

func (t ThresholdConfig) toThresholds() Thresholds {
	return Thresholds{
		CPUWarn: t.CPUWarn, CPUCrit: t.CPUCrit,
		MemWarn: t.MemWarn, MemCrit: t.MemCrit,
		DiskWarn: t.DiskWarn, DiskCrit: t.DiskCrit,
		DiskIOWarn: t.DiskIOWarn, DiskIOCrit: t.DiskIOCrit,
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
	// Role is the RBAC role: admin | operator | viewer.
	Role string `json:"role,omitempty"`
}

func defaultAccount() AccountConfig {
	salt := genToken()[:16]
	return AccountConfig{
		Username:    "admin",
		DisplayName: Tz("user.default_display"),
		Salt:        salt,
		Hash:        hashPassword("admin", salt),
		Role:        RoleAdmin,
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

// HTTPProxyConfig is a saved HTTP proxy shortcut for quick access.
type HTTPProxyConfig struct {
	ID          string `json:"id"`           // Unique ID
	Name        string `json:"name"`         // Display name (e.g. "内部API服务")
	HostID      string `json:"host_id"`      // Target host ID
	Hostname    string `json:"hostname"`     // Target hostname (cached for display)
	TargetPort  int    `json:"target_port"`  // Target port
	DefaultPath string `json:"default_path"` // Default path prefix (e.g. "/api/v1")
	Operator    string `json:"operator"`     // Who created this
	CreatedAt   int64  `json:"created_at"`   // Creation timestamp
	Enabled     bool   `json:"enabled"`      // Whether this proxy is currently active
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
	// PrevInstallToken + PrevTokenExpiresAt keep a rotated-out token valid during a
	// grace period, so existing agents don't drop offline the instant the token is
	// rotated. Managed by ResetToken (rotate).
	PrevInstallToken   string `json:"prev_install_token,omitempty"`
	PrevTokenExpiresAt int64  `json:"prev_token_expires_at,omitempty"`
	RequireToken       bool   `json:"require_token"`
	Account       AccountConfig     `json:"account"`
	Checks        []CustomCheck     `json:"checks"`
	Playbooks     []Playbook        `json:"playbooks,omitempty"`
	// TerminalDisabled is an inverted flag so remote terminal defaults ON for
	// existing configs (zero value = enabled); set true to globally disable it.
	TerminalDisabled bool `json:"terminal_disabled"`
	// ForwardDisabled is an inverted flag so port forwarding defaults ON for
	// existing configs (zero value = enabled); set true to globally disable it.
	ForwardDisabled bool `json:"forward_disabled"`
	// ForwardListen is the bind address for TCP port forwarding listeners.
	// Default "0.0.0.0" binds all interfaces (reachable from other machines).
	// Set to "127.0.0.1" to restrict access to the local machine only.
	ForwardListen string `json:"forward_listen,omitempty"`
	// ForwardPortRange is the port range for TCP port forwarding ("min-max").
	// Default "10000-10099" for Docker deployments to expose a predictable range.
	// Set to "" or "0-0" to let the OS assign any available port.
	ForwardPortRange string `json:"forward_port_range,omitempty"`
	// HTTPProxies is the list of saved HTTP proxy shortcuts.
	// Each entry stores a target host+port+path for quick access.
	HTTPProxies []HTTPProxyConfig `json:"http_proxies,omitempty"`
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
	// MFARequired is the global MFA enforcement policy: when true, every user
	// without MFA enabled will be forced to enroll on their next login before
	// they can access the dashboard. Managed by admin via /api/v1/mfa/global.
	MFARequired bool `json:"mfa_required"`
	// Users is the multi-account list (RBAC). The legacy single Account above is
	// migrated into this list on load and then cleared.
	Users []AccountConfig `json:"users"`
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

// Validate checks the server config for obvious misconfiguration before it is
// applied or persisted. Returns nil when the config is sound.
func (c ServerConfig) Validate() error {
	t := c.Thresholds
	// Threshold percentages must be in [0, 100].
	for name, v := range map[string]float64{
		"cpu_warn": t.CPUWarn, "cpu_crit": t.CPUCrit,
		"mem_warn": t.MemWarn, "mem_crit": t.MemCrit,
		"disk_warn": t.DiskWarn, "disk_crit": t.DiskCrit,
	} {
		if v < 0 || v > 100 {
			return fmt.Errorf("%s", Tz("config.threshold_range", name, v))
		}
	}
	// OfflineAfter must be positive.
	if t.OfflineAfterSec <= 0 {
		return fmt.Errorf("%s", Tz("config.offline_positive", t.OfflineAfterSec))
	}
	// SMTP port must be valid when SMTP is enabled.
	if c.SMTP.Enabled {
		if c.SMTP.Port < 1 || c.SMTP.Port > 65535 {
			return fmt.Errorf("%s", Tz("config.smtp_port_range", c.SMTP.Port))
		}
		// SMTP password (if set) must be at least 4 characters.
		if c.SMTP.Password != "" && len(c.SMTP.Password) < 4 {
			return fmt.Errorf("%s", Tz("config.smtp_password_short"))
		}
	}
	return nil
}

// ConfigStore wraps ServerConfig with disk persistence and thread safety.
type ConfigStore struct {
	mu      sync.RWMutex
	path    string
	cfg     ServerConfig
	prev    ServerConfig // snapshot before the last Set(), for Revert()
	hasPrev bool         // whether prev holds a valid snapshot
}

func NewConfigStore(path string) (*ConfigStore, error) {
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
	// Migrate the legacy single Account into the multi-user Users list and ensure
	// at least one admin exists (creates the default admin/admin on first run).
	if migrateUsers(&cs.cfg) {
		dirty = true
	}
	// Validate the loaded config — refuse to start with an obviously broken one.
	if err := cs.cfg.Validate(); err != nil {
		return nil, err
	}
	if dirty {
		_ = cs.save()
	}
	return cs, nil
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

// ForwardEnabled reports whether the port forwarding feature is available
// (default true; disabled only when forward_disabled is set in config).
func (cs *ConfigStore) ForwardEnabled() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return !cs.cfg.ForwardDisabled
}

// ForwardListenAddr returns the configured bind address for TCP forwarding
// listeners. Defaults to "0.0.0.0" (all interfaces) so forwarded ports are
// reachable from other machines — essential for Docker deployments where
// 127.0.0.1 would only be reachable inside the container.
func (cs *ConfigStore) ForwardListenAddr() string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.cfg.ForwardListen == "" {
		return "0.0.0.0"
	}
	return cs.cfg.ForwardListen
}

// ForwardPortRangeBounds returns the min and max port for TCP forwarding.
// Defaults to 10000-10099 for predictable Docker port exposure.
// Returns (0, 0) to let the OS assign any port if not configured or "0-0".
func (cs *ConfigStore) ForwardPortRangeBounds() (minPort, maxPort int) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.cfg.ForwardPortRange == "" {
		return 10000, 10099 // default range for Docker (100 ports)
	}
	parts := strings.Split(cs.cfg.ForwardPortRange, "-")
	if len(parts) != 2 {
		return 10000, 10099
	}
	minPort = parseIntSafe(parts[0], 10000)
	maxPort = parseIntSafe(parts[1], 10099)
	if minPort <= 0 || maxPort <= 0 || minPort > maxPort {
		return 10000, 10099
	}
	return minPort, maxPort
}

func parseIntSafe(s string, def int) int {
	s = strings.TrimSpace(s)
	v := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			v = v*10 + int(c-'0')
		} else {
			return def
		}
	}
	if v == 0 {
		return def
	}
	return v
}

// TrustProxy reports whether to honor reverse-proxy client-IP headers
// (X-Real-IP / X-Forwarded-For). Off by default so a directly-exposed server
// can't be fooled by forged headers into miscounting login attempts.
func (cs *ConfigStore) TrustProxy() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.TrustProxy
}

// MFARequired reports whether the global MFA enforcement policy is active.
func (cs *ConfigStore) MFARequired() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.MFARequired
}

// SetMFARequired toggles the global MFA enforcement policy and persists it.
func (cs *ConfigStore) SetMFARequired(v bool) error {
	cs.mu.Lock()
	cs.cfg.MFARequired = v
	cs.mu.Unlock()
	return cs.save()
}

// tokenGracePeriod is how long a rotated-out token stays valid, so agents keep
// reporting after a rotation until their install command is updated.
const tokenGracePeriod = 7 * 24 * time.Hour

// ResetToken ROTATES the install token: the current token becomes the previous
// token (valid for tokenGracePeriod), then a fresh token is generated and
// returned. Existing agents keep working during the grace window — a rotation is
// no longer an instant "all agents offline" event.
func (cs *ConfigStore) ResetToken() string {
	cs.mu.Lock()
	if cs.cfg.InstallToken != "" {
		cs.cfg.PrevInstallToken = cs.cfg.InstallToken
		cs.cfg.PrevTokenExpiresAt = time.Now().Add(tokenGracePeriod).Unix()
	}
	cs.cfg.InstallToken = genToken()
	tok := cs.cfg.InstallToken
	cs.mu.Unlock()
	_ = cs.save()
	return tok
}

// PrevTokenValidUntil returns the unix expiry of the grace-period token, or 0 if
// none is active (used by the UI to show "old token valid until …").
func (cs *ConfigStore) PrevTokenValidUntil() int64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.cfg.PrevInstallToken == "" || time.Now().Unix() >= cs.cfg.PrevTokenExpiresAt {
		return 0
	}
	return cs.cfg.PrevTokenExpiresAt
}

// ValidInstallToken reports whether got matches the current token, or the
// previous token during its grace period. Constant-time.
func (cs *ConfigStore) ValidInstallToken(got string) bool {
	cs.mu.RLock()
	cur := cs.cfg.InstallToken
	prev := cs.cfg.PrevInstallToken
	prevExp := cs.cfg.PrevTokenExpiresAt
	cs.mu.RUnlock()
	if cur != "" && subtle.ConstantTimeCompare([]byte(got), []byte(cur)) == 1 {
		return true
	}
	if prev != "" && time.Now().Unix() < prevExp &&
		subtle.ConstantTimeCompare([]byte(got), []byte(prev)) == 1 {
		return true
	}
	return false
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
// persists to disk. The current config is snapshotted first so a bad change can
// be rolled back via Revert().
func (cs *ConfigStore) Set(c ServerConfig) error {
	if err := c.Validate(); err != nil {
		return err
	}
	cs.mu.Lock()
	cs.prev = cs.cfg // snapshot for potential rollback
	cs.hasPrev = true
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
	c.ForwardDisabled = cs.cfg.ForwardDisabled
	c.ForwardListen = cs.cfg.ForwardListen
	c.ForwardPortRange = cs.cfg.ForwardPortRange
	c.AllowAnonymousAgents = cs.cfg.AllowAnonymousAgents
	c.TrustProxy = cs.cfg.TrustProxy
	c.MFARequired = cs.cfg.MFARequired
	cs.cfg = c
	cs.mu.Unlock()
	return cs.save()
}

// Revert restores the config that was active before the most recent successful
// Set(), undoing a bad configuration change. Returns an error when there is no
// previous snapshot to revert to.
func (cs *ConfigStore) Revert() error {
	cs.mu.Lock()
	if !cs.hasPrev {
		cs.mu.Unlock()
		return fmt.Errorf("%s", Tz("config.no_revert"))
	}
	cs.cfg = cs.prev
	cs.prev = ServerConfig{}
	cs.hasPrev = false
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

// --- HTTP Proxy Config Management ---

// ListHTTPProxies returns all saved HTTP proxy configurations.
func (cs *ConfigStore) ListHTTPProxies() []HTTPProxyConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return append([]HTTPProxyConfig{}, cs.cfg.HTTPProxies...)
}

// AddHTTPProxy adds a new HTTP proxy configuration.
func (cs *ConfigStore) AddHTTPProxy(proxy HTTPProxyConfig) error {
	cs.mu.Lock()
	if proxy.ID == "" {
		proxy.ID = termID()[:8]
	}
	if proxy.CreatedAt == 0 {
		proxy.CreatedAt = time.Now().Unix()
	}
	cs.cfg.HTTPProxies = append(cs.cfg.HTTPProxies, proxy)
	cs.mu.Unlock()
	return cs.save()
}

// DeleteHTTPProxy removes an HTTP proxy configuration by ID.
func (cs *ConfigStore) DeleteHTTPProxy(id string) error {
	cs.mu.Lock()
	var kept []HTTPProxyConfig
	for _, p := range cs.cfg.HTTPProxies {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	cs.cfg.HTTPProxies = kept
	cs.mu.Unlock()
	return cs.save()
}

// ToggleHTTPProxy enables or disables an HTTP proxy configuration.
func (cs *ConfigStore) ToggleHTTPProxy(id string, enabled bool) error {
	cs.mu.Lock()
	found := false
	for i, p := range cs.cfg.HTTPProxies {
		if p.ID == id {
			cs.cfg.HTTPProxies[i].Enabled = enabled
			found = true
			break
		}
	}
	cs.mu.Unlock()
	if !found {
		return fmt.Errorf("proxy not found")
	}
	return cs.save()
}

// UpdateHTTPProxy updates an existing HTTP proxy configuration.
// The Enabled field is preserved from the existing config when the caller
// doesn't explicitly set it to true — use the toggle API for enable/disable.
func (cs *ConfigStore) UpdateHTTPProxy(id string, updated HTTPProxyConfig) error {
	cs.mu.Lock()
	found := false
	for i, p := range cs.cfg.HTTPProxies {
		if p.ID == id {
			// Preserve ID, CreatedAt, and Enabled (when not explicitly set)
			updated.ID = p.ID
			updated.CreatedAt = p.CreatedAt
			if !updated.Enabled {
				updated.Enabled = p.Enabled
			}
			cs.cfg.HTTPProxies[i] = updated
			found = true
			break
		}
	}
	cs.mu.Unlock()
	if !found {
		return fmt.Errorf("proxy not found")
	}
	return cs.save()
}

// CopyHTTPProxy duplicates an HTTP proxy configuration with a new ID.
func (cs *ConfigStore) CopyHTTPProxy(id string) (HTTPProxyConfig, error) {
	cs.mu.Lock()
	var original *HTTPProxyConfig
	for i, p := range cs.cfg.HTTPProxies {
		if p.ID == id {
			original = &cs.cfg.HTTPProxies[i]
			break
		}
	}
	if original == nil {
		cs.mu.Unlock()
		return HTTPProxyConfig{}, fmt.Errorf("proxy not found")
	}
	// Create a copy with new ID and timestamp
	newProxy := *original
	newProxy.ID = termID()[:8]
	newProxy.CreatedAt = time.Now().Unix()
	newProxy.Name = original.Name + " (副本)"
	cs.cfg.HTTPProxies = append(cs.cfg.HTTPProxies, newProxy)
	cs.mu.Unlock()
	if err := cs.save(); err != nil {
		return HTTPProxyConfig{}, err
	}
	return newProxy, nil
}
