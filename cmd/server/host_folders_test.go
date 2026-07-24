package main

import (
	"path/filepath"
	"testing"
)

func TestValidateFolderTreeDepth(t *testing.T) {
	ok := []HostFolderNode{{
		ID: "a", Name: "L1", Children: []HostFolderNode{{
			ID: "b", Name: "L2", Children: []HostFolderNode{{
				ID: "c", Name: "L3", Children: []HostFolderNode{{
					ID: "d", Name: "L4",
				}},
			}},
		}},
	}}
	if err := validateFolderTree(ok); err != nil {
		t.Fatalf("depth 4 should be ok: %v", err)
	}
	bad := []HostFolderNode{{
		ID: "a", Name: "L1", Children: []HostFolderNode{{
			ID: "b", Name: "L2", Children: []HostFolderNode{{
				ID: "c", Name: "L3", Children: []HostFolderNode{{
					ID: "d", Name: "L4", Children: []HostFolderNode{{
						ID: "e", Name: "L5",
					}},
				}},
			}},
		}},
	}}
	if err := validateFolderTree(bad); err == nil {
		t.Fatal("depth 5 should fail")
	}
}

func testConfigStore(t *testing.T) *ConfigStore {
	t.Helper()
	cs, err := NewConfigStore(filepath.Join(t.TempDir(), "cfg.json"), nil)
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}
	return cs
}

func TestHostFolderMigrateAndCategoryL1(t *testing.T) {
	cs := testConfigStore(t)
	cs.cfg.Categories = map[string]string{"h1": "生产"}
	cs.cfg.HostFolders = nil
	cs.cfg.HostFolderAssign = nil
	hosts := []*Host{
		{ID: "h1", Hostname: "a", Category: "生产"},
		{ID: "h2", Hostname: "b", Category: "DB"},
		{ID: "h3", Hostname: "c"},
	}
	if !cs.ensureHostFoldersMigrated(hosts) {
		t.Fatal("expected migration")
	}
	if cs.cfg.HostFolders == nil {
		t.Fatal("HostFolders should be non-nil after migrate")
	}
	if len(cs.cfg.HostFolders) != 2 {
		t.Fatalf("want 2 L1 folders, got %d", len(cs.cfg.HostFolders))
	}
	if cs.cfg.HostFolderAssign["h1"] == "" || cs.cfg.HostFolderAssign["h2"] == "" {
		t.Fatal("h1/h2 should be assigned")
	}
	if _, ok := cs.cfg.HostFolderAssign["h3"]; ok {
		t.Fatal("h3 should stay ungrouped")
	}
	if cs.ensureHostFoldersMigrated(hosts) {
		t.Fatal("second migrate should be no-op")
	}

	if err := cs.setCategoryWithFolder("h3", "办公"); err != nil {
		t.Fatal(err)
	}
	if len(cs.cfg.HostFolders) != 3 {
		t.Fatalf("category should create L1, got %d", len(cs.cfg.HostFolders))
	}
	if cs.cfg.Categories["h3"] != "办公" {
		t.Fatalf("category sync: %q", cs.cfg.Categories["h3"])
	}
}

func TestDeleteHostFolderMovesUp(t *testing.T) {
	cs := testConfigStore(t)
	cs.cfg.HostFolders = []HostFolderNode{{
		ID: "p", Name: "Prod", Children: []HostFolderNode{{ID: "c", Name: "DB"}},
	}}
	cs.cfg.HostFolderAssign = map[string]string{"h1": "c"}
	cs.cfg.Categories = map[string]string{"h1": "DB"}
	if err := cs.deleteHostFolder("c"); err != nil {
		t.Fatal(err)
	}
	if cs.cfg.HostFolderAssign["h1"] != "p" {
		t.Fatalf("want parent p, got %q", cs.cfg.HostFolderAssign["h1"])
	}
	if cs.cfg.Categories["h1"] != "Prod" {
		t.Fatalf("category should be parent name, got %q", cs.cfg.Categories["h1"])
	}
	if err := cs.deleteHostFolder("p"); err != nil {
		t.Fatal(err)
	}
	if _, ok := cs.cfg.HostFolderAssign["h1"]; ok {
		t.Fatal("deleting L1 should ungroup hosts")
	}
}

func TestAddChildDepthLimit(t *testing.T) {
	cs := testConfigStore(t)
	cs.cfg.HostFolders = []HostFolderNode{}
	n1, err := cs.addHostFolder("", "L1")
	if err != nil {
		t.Fatal(err)
	}
	n2, err := cs.addHostFolder(n1.ID, "L2")
	if err != nil {
		t.Fatal(err)
	}
	n3, err := cs.addHostFolder(n2.ID, "L3")
	if err != nil {
		t.Fatal(err)
	}
	n4, err := cs.addHostFolder(n3.ID, "L4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cs.addHostFolder(n4.ID, "L5"); err == nil {
		t.Fatal("L5 should be rejected")
	}
}

func TestFolderPathMap(t *testing.T) {
	nodes := []HostFolderNode{{
		ID: "a", Name: "生产", Children: []HostFolderNode{{ID: "b", Name: "DB"}},
	}}
	paths := folderPathMap(nodes)
	if paths["b"] != "生产 / DB" {
		t.Fatalf("path=%q", paths["b"])
	}
}
