package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

//go:embed skillpacks/*.json
var skillPackFS embed.FS

type skillPackFile struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Skills  []struct {
		Name    string `json:"name"`
		Trigger string `json:"trigger"`
		Steps   string `json:"steps"`
		Tags    string `json:"tags"`
	} `json:"skills"`
}

type skillPackInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Count   int    `json:"skill_count"`
}

func listEmbeddedSkillPacks() ([]skillPackInfo, error) {
	entries, err := skillPackFS.ReadDir("skillpacks")
	if err != nil {
		return nil, err
	}
	var out []skillPackInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := skillPackFS.ReadFile("skillpacks/" + e.Name())
		if err != nil {
			continue
		}
		var pack skillPackFile
		if json.Unmarshal(b, &pack) != nil || pack.ID == "" {
			continue
		}
		out = append(out, skillPackInfo{ID: pack.ID, Name: pack.Name, Version: pack.Version, Count: len(pack.Skills)})
	}
	return out, nil
}

func loadEmbeddedSkillPack(id string) (skillPackFile, error) {
	id = strings.TrimSpace(strings.ToLower(id))
	b, err := skillPackFS.ReadFile("skillpacks/" + id + ".json")
	if err != nil {
		return skillPackFile{}, fmt.Errorf("知识包不存在: %s", id)
	}
	var pack skillPackFile
	if err := json.Unmarshal(b, &pack); err != nil {
		return skillPackFile{}, err
	}
	if pack.ID == "" {
		pack.ID = id
	}
	return pack, nil
}

// importSkillPack upserts pack skills into ai_skills with source=pack:<id>.
func (s *Server) importSkillPack(packID string) (imported, skipped int, err error) {
	pack, err := loadEmbeddedSkillPack(packID)
	if err != nil {
		return 0, 0, err
	}
	if s.pg == nil {
		return 0, 0, fmt.Errorf("需要 PostgreSQL 以导入技能包")
	}
	cfg := s.cfg.AIConfig()
	source := "pack:" + pack.ID
	for _, sk := range pack.Skills {
		name := strings.TrimSpace(sk.Name)
		trigger := strings.TrimSpace(sk.Trigger)
		steps := strings.TrimSpace(sk.Steps)
		if name == "" || trigger == "" || steps == "" {
			skipped++
			continue
		}
		var emb []float64
		if embedReady(cfg) {
			emb = embedText(cfg, name+"\n"+trigger)
		}
		// Prefer update-by-name+source if exists.
		if id, ok := s.pg.findSkillByNameSource(name, source); ok {
			if len(emb) > 0 {
				_ = s.pg.updateSkill(id, name, trigger, steps, emb)
			} else {
				_ = s.pg.updateSkillText(id, name, trigger, steps)
			}
			imported++
			continue
		}
		if _, err := s.pg.insertSkill(name, trigger, steps, sk.Tags, source, emb); err != nil {
			// Retry without vector if provider rejects empty embedding cast.
			if len(emb) == 0 {
				if _, err2 := s.pg.insertSkillNoEmbed(name, trigger, steps, sk.Tags, source); err2 != nil {
					skipped++
					continue
				}
				imported++
				continue
			}
			skipped++
			continue
		}
		imported++
	}
	return imported, skipped, nil
}

// GET /api/v1/ai/skill-packs
func (s *Server) handleListSkillPacks(w http.ResponseWriter, r *http.Request) {
	list, err := listEmbeddedSkillPacks()
	if err != nil || list == nil {
		list = []skillPackInfo{}
	}
	writeJSON(w, http.StatusOK, list)
}

// POST /api/v1/ai/skill-packs/import  {id:"mysql"} or {ids:["mysql","postgres"]}
func (s *Server) handleImportSkillPacks(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var req struct {
		ID  string   `json:"id"`
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	ids := append([]string{}, req.IDs...)
	if strings.TrimSpace(req.ID) != "" {
		ids = append(ids, req.ID)
	}
	if len(ids) == 0 {
		// import all
		all, _ := listEmbeddedSkillPacks()
		for _, p := range all {
			ids = append(ids, p.ID)
		}
	}
	type packResult struct {
		ID       string `json:"id"`
		Imported int    `json:"imported"`
		Skipped  int    `json:"skipped"`
		Error    string `json:"error,omitempty"`
	}
	var results []packResult
	totalImp := 0
	for _, id := range ids {
		imp, skip, err := s.importSkillPack(id)
		pr := packResult{ID: id, Imported: imp, Skipped: skip}
		if err != nil {
			pr.Error = err.Error()
		}
		totalImp += imp
		results = append(results, pr)
	}
	actor, ip := s.actorIP(r)
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: actor, IP: ip,
		Message: fmt.Sprintf("导入行业知识包 %v，新增/更新 %d 条技能", ids, totalImp)})
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "imported_total": totalImp})
}
