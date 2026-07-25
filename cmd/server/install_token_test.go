package main

import (
	"testing"
	"time"
)

func TestInstallTokenRevokeAndMaxUses(t *testing.T) {
	cs := &ConfigStore{cfg: ServerConfig{InstallToken: "tok-abcdef0123456789"}}
	if !cs.ValidInstallToken("tok-abcdef0123456789") {
		t.Fatal("token should be valid initially")
	}
	cs.cfg.InstallTokenMaxUses = 1
	cs.cfg.InstallTokenUseCount = 1
	if cs.ValidInstallToken("tok-abcdef0123456789") {
		t.Fatal("token should be invalid after max uses")
	}
	cs.cfg.InstallTokenUseCount = 0
	cs.cfg.InstallTokenExpiresAt = time.Now().Add(-time.Hour).Unix()
	if cs.ValidInstallToken("tok-abcdef0123456789") {
		t.Fatal("token should be invalid when expired")
	}
	cs.cfg.InstallTokenExpiresAt = 0
	cs.cfg.InstallTokenRevoked = true
	if cs.ValidInstallToken("tok-abcdef0123456789") {
		t.Fatal("revoked token should be invalid")
	}
}

func TestMapOIDCGroupsToRole(t *testing.T) {
	c := OIDCConfig{
		GroupClaim:   "groups",
		GroupRoleMap: map[string]string{"ops-admins": RoleAdmin, "ops": RoleOperator},
		DefaultRole:  RoleViewer,
	}
	info := map[string]any{"groups": []any{"ops"}}
	if got := mapOIDCGroupsToRole(info, c); got != RoleOperator {
		t.Fatalf("got %q want operator", got)
	}
	info2 := map[string]any{"groups": []any{"other"}}
	if got := mapOIDCGroupsToRole(info2, c); got != RoleViewer {
		t.Fatalf("got %q want viewer default", got)
	}
}
