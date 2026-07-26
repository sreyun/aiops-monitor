package main

import "testing"

func TestEmbeddedSkillPacksCore(t *testing.T) {
	list, err := listEmbeddedSkillPacks()
	if err != nil {
		t.Fatal(err)
	}
	need := map[string]bool{"incident-loop": false, "change-freeze": false, "mysql": false}
	for _, p := range list {
		if _, ok := need[p.ID]; ok {
			need[p.ID] = true
			if p.Count < 1 {
				t.Fatalf("pack %s has no skills", p.ID)
			}
		}
	}
	for id, ok := range need {
		if !ok {
			t.Fatalf("missing skill pack %s", id)
		}
	}
	pack, err := loadEmbeddedSkillPack("incident-loop")
	if err != nil || pack.ID != "incident-loop" {
		t.Fatalf("load incident-loop: %v %+v", err, pack)
	}
}
