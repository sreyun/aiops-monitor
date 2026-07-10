package main

import (
	"crypto/subtle"
	"fmt"
	"strings"
)

// ============================================================================
// Multi-user accounts + RBAC
//
// The dashboard supports multiple login accounts, each with a role. Roles are
// ranked; a request is allowed when the caller's rank meets the route's minimum.
//   admin    — full access, including user management
//   operator — every write/action except user management
//   viewer   — read-only (plus managing their own profile / password / MFA)
// ============================================================================

const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

// roleRank maps a role to a privilege level (higher = more). Unknown → 0.
func roleRank(role string) int {
	switch role {
	case RoleAdmin:
		return 3
	case RoleOperator:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

func validRole(role string) bool {
	return role == RoleAdmin || role == RoleOperator || role == RoleViewer
}

// migrateUsers upgrades a legacy single-account config to the Users list and
// enforces "at least one admin exists". Returns true if it changed anything.
func migrateUsers(c *ServerConfig) bool {
	changed := false
	if len(c.Users) == 0 {
		acc := c.Account
		if acc.Username == "" {
			acc = defaultAccount()
		}
		acc.Role = RoleAdmin
		c.Users = []AccountConfig{acc}
		changed = true
	}
	hasAdmin := false
	for i := range c.Users {
		if !validRole(c.Users[i].Role) {
			c.Users[i].Role = RoleViewer
			changed = true
		}
		if c.Users[i].Role == RoleAdmin {
			hasAdmin = true
		}
	}
	if !hasAdmin && len(c.Users) > 0 {
		c.Users[0].Role = RoleAdmin
		changed = true
	}
	// Drop the deprecated single-account mirror so credentials live in one place.
	if c.Account.Username != "" {
		c.Account = AccountConfig{}
		changed = true
	}
	return changed
}

// ---- per-user accessors on ConfigStore ----

// UsersList returns a copy of all users. Secret fields are included; any caller
// serializing to the browser MUST strip salt/hash/mfa_secret first.
func (cs *ConfigStore) UsersList() []AccountConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make([]AccountConfig, len(cs.cfg.Users))
	copy(out, cs.cfg.Users)
	return out
}

// UserByName returns the user with the exact username, and whether it was found.
func (cs *ConfigStore) UserByName(name string) (AccountConfig, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, u := range cs.cfg.Users {
		if u.Username == name {
			return u, true
		}
	}
	return AccountConfig{}, false
}

// UserByEmail returns the first user whose email matches (case-insensitive).
func (cs *ConfigStore) UserByEmail(email string) (AccountConfig, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, u := range cs.cfg.Users {
		if u.Email != "" && strings.EqualFold(u.Email, email) {
			return u, true
		}
	}
	return AccountConfig{}, false
}

// RoleOf returns a user's role, or "" if the user doesn't exist.
func (cs *ConfigStore) RoleOf(name string) string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, u := range cs.cfg.Users {
		if u.Username == name {
			return u.Role
		}
	}
	return ""
}

// callers below must hold cs.mu.
func (cs *ConfigStore) findLocked(name string) int {
	for i := range cs.cfg.Users {
		if cs.cfg.Users[i].Username == name {
			return i
		}
	}
	return -1
}
func (cs *ConfigStore) adminCountLocked() int {
	n := 0
	for _, u := range cs.cfg.Users {
		if u.Role == RoleAdmin {
			n++
		}
	}
	return n
}

// CreateUser adds a new user with the given password. Fails if the name exists.
func (cs *ConfigStore) CreateUser(username, password, displayName, email, role string) error {
	cs.mu.Lock()
	if cs.findLocked(username) >= 0 {
		cs.mu.Unlock()
		return fmt.Errorf("%s", Tz("user.username_exists"))
	}
	salt := genToken()[:16]
	cs.cfg.Users = append(cs.cfg.Users, AccountConfig{
		Username: username, DisplayName: displayName, Email: email,
		Salt: salt, Hash: hashPassword(password, salt), Role: role,
	})
	cs.mu.Unlock()
	return cs.save()
}

// UpdateUserMeta changes a user's display name, email and role (admin action).
// Refuses to demote the last remaining admin.
func (cs *ConfigStore) UpdateUserMeta(username, displayName, email, role string) error {
	cs.mu.Lock()
	i := cs.findLocked(username)
	if i < 0 {
		cs.mu.Unlock()
		return fmt.Errorf("%s", Tz("user.not_found"))
	}
	if cs.cfg.Users[i].Role == RoleAdmin && role != RoleAdmin && cs.adminCountLocked() <= 1 {
		cs.mu.Unlock()
		return fmt.Errorf("%s", Tz("user.keep_one_admin"))
	}
	cs.cfg.Users[i].DisplayName = displayName
	cs.cfg.Users[i].Email = email
	cs.cfg.Users[i].Role = role
	cs.mu.Unlock()
	return cs.save()
}

// SetUserPassword sets a user's password (self change-password or admin reset).
func (cs *ConfigStore) SetUserPassword(username, password string) error {
	cs.mu.Lock()
	i := cs.findLocked(username)
	if i < 0 {
		cs.mu.Unlock()
		return fmt.Errorf("%s", Tz("user.not_found"))
	}
	salt := genToken()[:16]
	cs.cfg.Users[i].Salt = salt
	cs.cfg.Users[i].Hash = hashPassword(password, salt)
	cs.mu.Unlock()
	return cs.save()
}

// SetUserProfile updates a user's own display name + email.
func (cs *ConfigStore) SetUserProfile(username, displayName, email string) error {
	cs.mu.Lock()
	i := cs.findLocked(username)
	if i < 0 {
		cs.mu.Unlock()
		return fmt.Errorf("%s", Tz("user.not_found"))
	}
	cs.cfg.Users[i].DisplayName = displayName
	cs.cfg.Users[i].Email = email
	cs.mu.Unlock()
	return cs.save()
}

// SetTerminalPassword sets (or changes) the terminal secondary password.
// v5.3.0: terminal password uses the same salted SHA-256 scheme as login password.
func (cs *ConfigStore) SetTerminalPassword(username, password string) error {
	cs.mu.Lock()
	i := cs.findLocked(username)
	if i < 0 {
		cs.mu.Unlock()
		return fmt.Errorf("%s", Tz("user.not_found"))
	}
	salt := genToken()[:16]
	cs.cfg.Users[i].TerminalPasswordSalt = salt
	cs.cfg.Users[i].TerminalPasswordHash = hashPassword(password, salt)
	cs.mu.Unlock()
	return cs.save()
}

// VerifyTerminalPassword checks the terminal secondary password.
// Returns true if the password matches.
func (cs *ConfigStore) VerifyTerminalPassword(username, password string) bool {
	cs.mu.Lock()
	i := cs.findLocked(username)
	if i < 0 {
		cs.mu.Unlock()
		return false
	}
	hash := cs.cfg.Users[i].TerminalPasswordHash
	salt := cs.cfg.Users[i].TerminalPasswordSalt
	cs.mu.Unlock()
	if hash == "" || salt == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hashPassword(password, salt)), []byte(hash)) == 1
}

// HasTerminalPassword reports whether the user has set a terminal password.
func (cs *ConfigStore) HasTerminalPassword(username string) bool {
	cs.mu.Lock()
	i := cs.findLocked(username)
	if i < 0 {
		cs.mu.Unlock()
		return false
	}
	has := cs.cfg.Users[i].TerminalPasswordHash != ""
	cs.mu.Unlock()
	return has
}

// RenameUser changes a user's login name. Fails if the new name is taken.
func (cs *ConfigStore) RenameUser(oldName, newName string) error {
	cs.mu.Lock()
	if oldName != newName && cs.findLocked(newName) >= 0 {
		cs.mu.Unlock()
		return fmt.Errorf("%s", Tz("user.username_exists"))
	}
	i := cs.findLocked(oldName)
	if i < 0 {
		cs.mu.Unlock()
		return fmt.Errorf("%s", Tz("user.not_found"))
	}
	cs.cfg.Users[i].Username = newName
	cs.mu.Unlock()
	return cs.save()
}

// SetUserMFA enables/disables a user's TOTP factor (disabling clears the secret).
func (cs *ConfigStore) SetUserMFA(username string, enabled bool, secret string) error {
	cs.mu.Lock()
	i := cs.findLocked(username)
	if i < 0 {
		cs.mu.Unlock()
		return fmt.Errorf("%s", Tz("user.not_found"))
	}
	cs.cfg.Users[i].MFAEnabled = enabled
	if enabled {
		cs.cfg.Users[i].MFASecret = secret
	} else {
		cs.cfg.Users[i].MFASecret = ""
	}
	cs.mu.Unlock()
	return cs.save()
}

// DeleteUser removes a user. Refuses to delete the last admin or the last user.
func (cs *ConfigStore) DeleteUser(username string) error {
	cs.mu.Lock()
	i := cs.findLocked(username)
	if i < 0 {
		cs.mu.Unlock()
		return fmt.Errorf("%s", Tz("user.not_found"))
	}
	if len(cs.cfg.Users) <= 1 {
		cs.mu.Unlock()
		return fmt.Errorf("%s", Tz("user.keep_one_user"))
	}
	if cs.cfg.Users[i].Role == RoleAdmin && cs.adminCountLocked() <= 1 {
		cs.mu.Unlock()
		return fmt.Errorf("%s", Tz("user.keep_one_admin"))
	}
	cs.cfg.Users = append(cs.cfg.Users[:i], cs.cfg.Users[i+1:]...)
	cs.mu.Unlock()
	return cs.save()
}

// AlertEmails returns the deduplicated non-empty emails of all users — the
// recipients for alert / test notifications.
func (cs *ConfigStore) AlertEmails() []string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	var out []string
	seen := map[string]bool{}
	for _, u := range cs.cfg.Users {
		e := strings.TrimSpace(u.Email)
		if e != "" && !seen[strings.ToLower(e)] {
			seen[strings.ToLower(e)] = true
			out = append(out, e)
		}
	}
	return out
}
