package main

import (
	_ "embed"
	"os"
	"path/filepath"

	"log/slog"
)

// configExampleJSON is the embedded copy of config.example.json.
// It is written to the agent's config directory on first startup so users
// always have a reference file next to their config.json.
//
//go:generate cp ../../config.example.json config_example.json
//go:embed config_example.json
var configExampleJSON []byte

// ensureConfigExample writes config.example.json next to the config file
// if it does not already exist. This guarantees every agent installation
// has a reference configuration regardless of install method.
func ensureConfigExample(cfgPath string) {
	dir := filepath.Dir(cfgPath)
	if dir == "" || dir == "." {
		// config in CWD — resolve absolute so we write to a real path
		if abs, err := filepath.Abs("."); err == nil {
			dir = abs
		}
	}
	dst := filepath.Join(dir, "config.example.json")
	if _, err := os.Stat(dst); err == nil {
		return // already exists — never overwrite user edits
	}
	if err := os.WriteFile(dst, configExampleJSON, 0644); err != nil {
		slog.Warn("写入 config.example.json 失败", "path", dst, "err", err)
	} else {
		slog.Info("已生成配置示例文件", "path", dst)
	}
}
