package main

import (
	"encoding/json"
	"strings"
)

// resolveModelForTask picks a model for an assist/chat task.
// Priority: TaskModelsJSON map → CheapModel for light tasks → cfg.Model.
// routed=true when the chosen model differs from the configured primary Model.
func resolveModelForTask(cfg AIConfig, task string) (model string, routed bool) {
	primary := strings.TrimSpace(cfg.Model)
	task = strings.ToLower(strings.TrimSpace(task))
	chosen := primary
	if m := taskModelFromJSON(cfg.TaskModelsJSON, task); m != "" {
		chosen = m
	} else if cheap := strings.TrimSpace(cfg.CheapModel); cheap != "" && isCheapAITask(task) {
		chosen = cheap
	}
	if chosen == "" {
		chosen = primary
	}
	return chosen, chosen != "" && chosen != primary
}

func taskModelFromJSON(raw, task string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || task == "" {
		return ""
	}
	var m map[string]string
	if json.Unmarshal([]byte(raw), &m) != nil {
		return ""
	}
	if v := strings.TrimSpace(m[task]); v != "" {
		return v
	}
	for k, v := range m {
		if strings.EqualFold(k, task) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func isCheapAITask(task string) bool {
	switch task {
	case "promql", "logql", "pgsql", "sqlql", "sql_beautify", "dashboard_prompt_optimize",
		"summarize", "summary", "translate", "legend", "title", "rename", "classify",
		"tag", "label", "format", "beautify":
		return true
	default:
		return strings.HasSuffix(task, "_suggest") || strings.HasPrefix(task, "fmt_")
	}
}

func applyRoutedModel(cfg AIConfig, task string) AIConfig {
	if m, _ := resolveModelForTask(cfg, task); m != "" {
		cfg.Model = m
	}
	return cfg
}
