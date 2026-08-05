package main

import (
	"net/http"
	"sort"
)

// listPromptNames 返回已注册的命名提示词模板清单（内嵌 + 部署覆盖）。
func listPromptNames() []string {
	entries, err := embeddedPromptsFS.ReadDir("prompts")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) > 3 && name[len(name)-3:] == ".md" {
			names = append(names, name[:len(name)-3])
		}
	}
	sort.Strings(names)
	return names
}

// handleAIPrompts GET /api/v1/ai/prompts (admin) — 列出当前生效的提示词模板与版本指纹，
// 便于排障「当前到底用的是哪版 prompt」。
func (s *Server) handleAIPrompts(w http.ResponseWriter, r *http.Request) {
	names := listPromptNames()
	type promptInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Source  string `json:"source"` // embedded | override
	}
	overrides := false
	if cfg := s.cfg.AIConfig(); cfg.PromptOverridesDir != "" {
		overrides = true
	}
	out := make([]promptInfo, 0, len(names))
	for _, n := range names {
		_, ver, err := defaultPromptStore.loadTemplate(n)
		if err != nil {
			continue
		}
		src := "embedded"
		if overrides {
			src = "override"
		}
		out = append(out, promptInfo{Name: n, Version: ver, Source: src})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"prompts":          out,
		"overrides_dir":    s.cfg.AIConfig().PromptOverridesDir,
		"override_enabled": overrides,
	})
}
