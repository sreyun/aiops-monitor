package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"aiops-monitor/shared"
)

// pluginOutput is the JSON contract every plugin prints to stdout. All fields
// are optional:
//   - base:    base system metrics (used only when the native collector is
//              unsupported, e.g. on Windows/macOS via psutil)
//   - metrics: custom named gauges (mysql.connections, nginx.rps, ...)
//   - events:  discrete findings from the Python/AI/automation layer
type pluginOutput struct {
	Base    *shared.Metrics    `json:"base"`
	Metrics map[string]float64 `json:"metrics"`
	Events  []shared.Event     `json:"events"`
}

// pluginResult is the merged output of all plugins in one run.
type pluginResult struct {
	base   *shared.Metrics
	custom map[string]float64
	events []shared.Event
}

// PluginRunner discovers and executes plugins from a directory. Plugins run as
// isolated subprocesses so a crashing / hanging plugin can never take down the
// agent core — it is logged and skipped.
type PluginRunner struct {
	dir     string
	python  string
	timeout time.Duration
}

func NewPluginRunner(dir, python string, timeout time.Duration) *PluginRunner {
	return &PluginRunner{dir: dir, python: python, timeout: timeout}
}

// discover returns the plugin files to run, skipping the SDK helper, dotfiles
// and underscore-prefixed files.
func (p *PluginRunner) discover() []string {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		if name == "plugin_sdk.py" || name == "requirements.txt" {
			continue
		}
		// skip data / config files that live alongside plugins (e.g. a plugin's
		// own .json config) — only executable plugins should be run.
		switch strings.ToLower(filepath.Ext(name)) {
		case ".json", ".txt", ".md", ".yaml", ".yml", ".conf", ".ini", ".cfg", ".log":
			continue
		}
		out = append(out, filepath.Join(p.dir, name))
	}
	sort.Strings(out)
	return out
}

// RunAll executes every plugin concurrently and merges their outputs.
func (p *PluginRunner) RunAll(logf func(string, ...any)) pluginResult {
	files := p.discover()
	agg := pluginResult{custom: map[string]float64{}}
	if len(files) == 0 {
		return agg
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, f := range files {
		wg.Add(1)
		go func(file string) {
			defer wg.Done()
			name := pluginName(file)
			out, err := p.runOne(file)
			if err != nil {
				logf("插件 %s 执行失败: %v", name, err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if out.Base != nil {
				agg.base = out.Base
			}
			for k, v := range out.Metrics {
				agg.custom[k] = v
			}
			for _, ev := range out.Events {
				if ev.Source == "" {
					ev.Source = name
				}
				agg.events = append(agg.events, ev)
			}
		}(f)
	}
	wg.Wait()
	return agg
}

func (p *PluginRunner) runOne(file string) (pluginOutput, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	var cmd *exec.Cmd
	if strings.HasSuffix(file, ".py") {
		cmd = exec.CommandContext(ctx, p.python, file)
	} else {
		cmd = exec.CommandContext(ctx, file)
	}
	stdout, err := cmd.Output()
	if err != nil {
		return pluginOutput{}, err
	}
	stdout = []byte(strings.TrimSpace(string(stdout)))
	if len(stdout) == 0 {
		return pluginOutput{}, nil
	}
	var out pluginOutput
	if err := json.Unmarshal(stdout, &out); err != nil {
		return pluginOutput{}, err
	}
	return out, nil
}

func pluginName(file string) string {
	base := filepath.Base(file)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
